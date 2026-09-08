package signal

import (
	"fmt"
	"time"
)

// Input 一次回放的全部输入。信号与 K 线由调用方从 DB 取，本包不碰 IO。
type Input struct {
	Params  Params
	Signals []Signal // 真实触发流（strategy_event，按 instance_key + account 过滤）
	Bars    []Bar    // 1m K 线，只用于持仓期间的路径回放
	Seed    SeedPosition

	// DevWindows 无条件偏离采样窗口（dev_sample），只在改 signal_threshold 时用来推 λ(θ)。
	DevWindows []DevWindow
}

// Result 一次回放的产出。
type Result struct {
	Params   Params
	Fidelity Fidelity

	// 信号侧计数：Replayed 必须与 Total 减去"落在窗口/种子之前被丢弃的那些"完全相等，
	// 零丢失零重复是本引擎的第一条验收口径。
	SignalTotal    int
	SignalReplayed int
	SignalDropped  int // 丢弃原因：ts ≤ 种子时刻，或没有任何 K 线覆盖
	SignalFiltered int // 抬高阈值后被 |gapBp| 门限筛掉的（频率级路径）

	Episodes []Episode
	Equity   []EquityPoint

	Realized float64 // 已实现盈亏（含减仓锁利）
	Fees     float64
	Floating float64 // 期末浮动
	Net      float64 // Realized − Fees + Floating

	MaxDrawdown   float64
	Reduces       int
	SkipCap       int
	SkipGate      int
	SkipTrend     int
	MaxStack      int
	Cap           int
	BarCount      int
	FirstBar      time.Time
	LastBar       time.Time
	LastPx        float64
	OpenPositions []Episode // Episodes 里 Open=true 的那些
}

// Replay 跑一次事件驱动回放。
//
// 回放循环与金标准 backtest_dual_side.py 的 run() 逐行对齐：
//
//	按根推进 → 先把 ts ≤ 本根开盘时刻的待处理信号全部成交（成交价按 EntryPx，
//	默认取本根收盘）→ 再判定本根的出场 → 记 MTM。
//
// 为什么信号成交在"本根收盘"而不是"本根开盘"：信号 ts 落在上一根之内，
// 实盘还有 trade.signal.delay_seconds=5 秒的下单延迟，取本根收盘是金标准
// 已经用真实平仓序列校准过的口径，换成开盘会与全部历史研究结论不可比。
func Replay(in Input) (*Result, error) {
	p := in.Params.Normalize()
	if err := p.Validate(); err != nil {
		return nil, err
	}
	fid := Classify(p)

	signals := SortSignals(in.Signals)
	res := &Result{Params: p, SignalTotal: len(signals)}

	// 频率级路径：阈值被改动。抬高阈值可以在 θ0 流上做门限筛（近似）；
	// 降低阈值无法从事件流里补出新触发点，只能给 λ，不产 PnL。
	if fid.ThresholdChanged {
		lam, err := Lambda(in.DevWindows, p.BaselineThresholdBp, p.SignalThresholdBp, len(signals), in.Bars)
		if err == nil {
			fid.Lambda = lam
		} else {
			fid.Notes = append(fid.Notes, "λ(θ) 无法推导："+err.Error()+"（dev_sample 覆盖不足）")
		}
		if p.SignalThresholdBp < p.BaselineThresholdBp {
			res.Fidelity = fid
			res.SignalReplayed = 0
			res.SignalDropped = len(signals)
			return res, nil
		}
		before := len(signals)
		signals = FilterByGapBp(signals, p.SignalThresholdBp)
		res.SignalFiltered = before - len(signals)
	}
	res.Fidelity = fid

	bars := SortBars(in.Bars)
	if len(bars) == 0 {
		return nil, fmt.Errorf("回放区间内没有 1m K 线：持仓路径无法回放（先跑 K 线回填）")
	}

	eng := NewEngine(p)
	seed := in.Seed
	if seed.OK() {
		eng.Seed(seed)
	}

	// 丢掉种子时刻及更早的信号：那些触发的结果已经体现在种子仓里，
	// 再回放一遍等于把同一批仓位开两次。
	pending := make([]Signal, 0, len(signals))
	for _, s := range signals {
		if seed.OK() && !s.Ts.After(seed.At) {
			res.SignalDropped++
			continue
		}
		pending = append(pending, s)
	}

	idx := 0
	for _, bar := range bars {
		if seed.OK() && bar.Ts.Before(seed.At) {
			continue
		}
		eng.ObserveTrend(bar.Ts, bar.Close)
		for idx < len(pending) && !pending[idx].Ts.After(bar.Ts) {
			s := pending[idx]
			eng.OnSignal(s, entryPrice(p, s, bar))
			res.SignalReplayed++
			idx++
		}
		eng.OnBar(bar)
	}
	// 落在最后一根之后的信号没有任何 K 线覆盖，无法判定其后续路径。
	res.SignalDropped += len(pending) - idx

	eng.Finish()
	fill(res, eng)
	return res, nil
}

// entryPrice 本次触发的成交价。
func entryPrice(p Params, s Signal, bar Bar) float64 {
	if p.EntryPx == EntrySigLast && s.SigLast > 0 {
		return s.SigLast
	}
	return bar.Close
}

func fill(res *Result, e *Engine) {
	res.Episodes = e.episodes
	res.Equity = e.equity
	res.Realized = e.realized
	res.Fees = e.fees
	res.Floating = e.floating(e.lastPx)
	res.Net = res.Realized - res.Fees + res.Floating
	res.Reduces = e.reduces
	res.SkipCap = e.skipCap
	res.SkipGate = e.skipGate
	res.SkipTrend = e.skipTrend
	res.MaxStack = e.maxStack
	res.Cap = e.cap
	res.BarCount = e.barCount
	res.FirstBar = e.firstBar
	res.LastBar = e.lastBar
	res.LastPx = e.lastPx

	hi := 0.0
	first := true
	for _, pt := range e.equity {
		if first || pt.MTM > hi {
			hi = pt.MTM
			first = false
		}
		if dd := hi - pt.MTM; dd > res.MaxDrawdown {
			res.MaxDrawdown = dd
		}
	}
	for _, ep := range e.episodes {
		if ep.Open {
			res.OpenPositions = append(res.OpenPositions, ep)
		}
	}
}
