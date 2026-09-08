package signal

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

func errOrderSizeOverCeiling(orderSize, ceiling int) error {
	return fmt.Errorf("order_size=%d 超过仓位上限 %d：本组一张也开不进去，参数无意义", orderSize, ceiling)
}

// 信号事件类型：一次触发在每个账户上恰好落一条（manager.go 的四个早返回分支）。
// 四类合起来才是完整信号流——被拦截的触发同样是真实触发，漏掉它们会让
// "触发次数与真实触发数一致"的验收口径失真。
const (
	EvOpen      = "open"
	EvCapSkip   = "cap_skip"
	EvGateBlock = "gate_block"
	EvTrendSkip = "trend_skip"
)

// SignalEventTypes 构成信号流的四类事件（供仓储层 IN 查询与计数校验共用）。
func SignalEventTypes() []string { return []string{EvOpen, EvCapSkip, EvGateBlock, EvTrendSkip} }

// Signal 一次真实触发。全部字段来自 strategy_event，本包不推导任何一项。
type Signal struct {
	Ts    time.Time // 触发时刻（秒精度，与 JSONL 逐字一致）
	Side  string    // 下单方向 long/short（UP 偏离→long，DOWN→short）
	Event string    // 该触发在生产参数下的实际结局：open/cap_skip/gate_block/trend_skip

	OrderSize int     // 事件记录的本次下单张数
	GapBp     float64 // 带符号偏离 bp（>0=UP）；用于阈值抬高时的门限过滤
	SigLast   float64 // 触发时刻 DeepCoin last
	SigMark   float64 // 触发时刻 mark

	// 生产参数下的净仓快照（gate_block 事件才完整），仅用于种子仓推导与对账，
	// 不参与回放判定——回放的净仓由引擎自己累积。
	NetSide  string
	NetSize  int
	AvgPx    float64
	LastPx   float64
	Reason   string
	GateKind string
}

// Bar 一根 1m K 线。Ts 是开盘时刻（本地时区口径，与事件 ts 同源）。
type Bar struct {
	Ts    time.Time
	Open  float64
	High  float64
	Low   float64
	Close float64
}

// SeedPosition 回放窗口起点已有的旧仓。窗口左端几乎总是切在持仓中间，
// 不灌种子仓会让第一批信号在空仓上重新起步，前若干笔的分档与门控全错。
type SeedPosition struct {
	At    time.Time
	Side  string
	Size  int
	AvgPx float64
}

// OK 种子仓是否可用。
func (s SeedPosition) OK() bool {
	return s.Size > 0 && s.AvgPx > 0 && (strings.EqualFold(s.Side, "long") || strings.EqualFold(s.Side, "short"))
}

// SeedFromSignals 用窗口内第一条同时带 avgPx + size + netSide 的事件推种子仓，
// 口径同金标准 backtest_dual_side.py 的 extract_seed：拦截类事件（gate_block）
// 携带净仓快照，是窗口起点唯一可信的旧仓来源——open 事件不带 avgPx
// （见 pkg/trade/manager.go 的 openEv 组装，只有反向减仓锁利那一支才补）。
//
// 比脚本多一道护栏：**只认出现在第一条 open 之前的快照**。脚本的窗口是人挑的、
// 必然切在持仓中间；这里的窗口由使用者任选，如果窗口起点本来就是空仓，
// 第一条带快照的 gate_block 可能出现在若干次 open 之后——那时的持仓是回放
// 自己建起来的，再灌一次种子等于把同一批仓位开两次。
func SeedFromSignals(signals []Signal) SeedPosition {
	for _, s := range signals {
		if s.Event == EvOpen {
			return SeedPosition{} // 窗口起点是空仓：持仓由回放自己建立
		}
		if s.AvgPx > 0 && s.NetSize > 0 && s.NetSide != "" {
			return SeedPosition{At: s.Ts, Side: strings.ToLower(s.NetSide), Size: abs(s.NetSize), AvgPx: s.AvgPx}
		}
	}
	return SeedPosition{}
}

// FilterByGapBp 抬高信号阈值时的近似过滤：只留 |gapBp| ≥ θ 的触发。
//
// 这**不是**精确重推。生产的信号规则是 edge-trigger + in-band re-arm
// （见 monitor.EvaluateDeviationSignal）：阈值一变，"同向持续只算一次"的
// 抑制边界随之变化，某些在 θ0 下被抑制的偏离在更高的 θ 下会成为新的首次触发。
// 因此本函数只是"θ0 流上的门限筛"，结果一律标频率级精度，见 fidelity.go。
func FilterByGapBp(signals []Signal, thresholdBp float64) []Signal {
	if thresholdBp <= 0 {
		return signals
	}
	out := make([]Signal, 0, len(signals))
	for _, s := range signals {
		if math.Abs(s.GapBp) >= thresholdBp {
			out = append(out, s)
		}
	}
	return out
}

// SortSignals 按 (ts, 原顺序) 稳定排序。同秒多条触发是常态（实测单分钟最多 5 次），
// 必须保序回放，不能去重。
func SortSignals(signals []Signal) []Signal {
	out := append([]Signal(nil), signals...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	return out
}

// SortBars 按开盘时刻升序排序并丢掉重复/非法根。
func SortBars(bars []Bar) []Bar {
	out := make([]Bar, 0, len(bars))
	for _, b := range bars {
		if b.Close > 0 && b.High > 0 && b.Low > 0 {
			out = append(out, b)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Ts.Before(out[j].Ts) })
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
