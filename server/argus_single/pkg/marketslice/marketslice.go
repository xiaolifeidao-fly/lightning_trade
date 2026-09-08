// Package marketslice 承载「触发瞬间 ±1min 秒级行情切片」的载荷与旁路投递口。
//
// 为什么单独成一个 leaf 包（与 pkg/eventlog 同因）：切片由 pkg/monitor 在双 WS
// 流上聚合产出、由 pkg/eventstore 落库，两边都不该反向依赖对方——monitor 挂上
// gorm 会把 DB 驱动拖进交易 goroutine 所在的包，eventstore 反向 import monitor
// 又会把整条 trade 栈拖进写库包。因此这里只放"数据结构 + 接口"，实现分居两侧，
// 由 initialization 组装时注册。
//
// 两条硬约束（与 eventlog.Sink 一致，改代码前先读）：
//  1. EmitSlice 必须非阻塞、自行吞掉全部错误——写库问题绝不能回灌到行情链路；
//  2. 切片是**派生观测品，不是策略事件**：它不进 JSONL。理由见 Slice 的注释。
package marketslice

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// HalfWindowSeconds 切片半窗：触发时刻前后各 60 秒（需求大纲 §3.1 的 ±1min）。
const HalfWindowSeconds = 60

// SeriesPoints 单条序列的点数：[-60, +60] 闭区间，含触发那一秒 ⇒ 121 点。
const SeriesPoints = 2*HalfWindowSeconds + 1

// Slice 一次触发对应的秒级切片载荷：一条记录 + 三条等长序列。
//
// 为什么不落 JSONL（与 dev_sample 的处理刻意不同）：
//  1. 它在触发那一刻还不完整——右半窗要再等 60 秒，写不进"触发时刻"那一行；
//  2. 单条载荷约 4KB（3×121 个浮点），JSONL 是回测平台与全部历史分析脚本的
//     输入格式，塞进去会让每日日志膨胀且没有任何脚本读它；
//  3. 它有 90 天滚动保留的生命周期，JSONL 没有这个概念。
//
// 三条序列等长且按秒对齐：第 i 个点对应 Anchor-HalfWindow+i 秒。
// 该秒没有 tick 时元素为 nil——nil 表示"这一秒没有行情"，不是 0，也不许插值：
// 切片的用途就是看清触发瞬间到底发生了什么，补出来的点会直接骗人。
type Slice struct {
	Anchor     time.Time // 触发时刻（秒精度，本地墙钟口径，与 strategy_event.ts 同源）
	InstIdRaw  string    // 原始 instId（信号侧口径，如 BTCUSDT），归一化交给写侧
	StartAt    time.Time // Anchor - HalfWindow 秒
	EndAt      time.Time // Anchor + HalfWindow 秒
	HalfWindow int       // 半窗秒数，恒为 HalfWindowSeconds；显式带上以便将来调窗不歧义

	DcLast  []*float64 // DeepCoin 最新价序列
	DcMark  []*float64 // DeepCoin 标记价序列
	BinLast []*float64 // 币安最新价序列

	DcPoints  int // DcLast/DcMark 中的非 nil 点数（覆盖率，判断切片是否可用）
	BinPoints int // BinLast 中的非 nil 点数
}

// Sink 切片旁路投递口。当前唯一实现是 pkg/eventstore 的 signal_slice writer。
type Sink interface {
	EmitSlice(s Slice)
}

var (
	sinkMu sync.RWMutex
	sinks  []Sink
)

// RegisterSink 注册一个 sink，可重复调用（按注册顺序投递）。
func RegisterSink(s Sink) {
	if s == nil {
		return
	}
	sinkMu.Lock()
	sinks = append(sinks, s)
	sinkMu.Unlock()
}

// ResetSinks 清空已注册 sink（进程收尾与测试隔离用）。
func ResetSinks() {
	sinkMu.Lock()
	sinks = nil
	sinkMu.Unlock()
}

// SinkCount 已注册的 sink 数量（自检与测试用）。
func SinkCount() int {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	return len(sinks)
}

// Emit 把切片投递给全部 sink。单个 sink 的 panic 只记 error。
func Emit(s Slice) {
	sinkMu.RLock()
	list := sinks
	sinkMu.RUnlock()
	for _, sink := range list {
		emitOne(sink, s)
	}
}

func emitOne(sink Sink, s Slice) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[marketslice] sink 投递 panic（不影响行情与交易）: %v", r)
		}
	}()
	sink.EmitSlice(s)
}
