package trade

import "argus_single/pkg/eventlog"

// SignalQuote 信号触发时刻的行情快照（P7 新 alpha 数据入口）。
//
// 系统的信号源就是 DeepCoin last 相对 mark 的偏离（|dev| ≥ signal_threshold=0.0005
// 即 5bp 触发），但偏离幅度此前从未落盘——只在 price_monitor 的日志里以字符串出现。
// 落盘 gap_bp 后才能开题"信号分级"：检验 gap 大小是否与信号后续表现相关。
//
// 必须随 SignalSnapshot 显式传递、不可用全局缓存：信号触发与实际下单之间有
// trade.signal.delay_seconds=5 的延迟，期间可能有新信号到达并覆盖缓存。
type SignalQuote struct {
	Last  float64 // 信号时刻 DeepCoin last
	Mark  float64 // 信号时刻 DeepCoin mark
	GapBp float64 // (last-mark)/mark × 10000，带符号：>0 对应 UP，<0 对应 DOWN
	// 趋势闸（8/21 事故补丁）：信号时刻的窗口动量。随快照显式传递的理由同上
	// ——5s 延迟期间动量可能刷新，判定必须用信号时刻的值。TrendOK=false 表示
	// 历史不足一个窗口（重启后未回填），判定层放行。零值安全（不启用闸）。
	TrendMomPct float64
	TrendOK     bool
}

// NewSignalQuote 由 last/mark 构造快照并算出 gap（基点）。
// mark ≤ 0 时无法计算，返回零值快照（OK()=false），调用方须整体省略而非落 0。
func NewSignalQuote(last, mark float64) SignalQuote {
	if mark <= 0 {
		return SignalQuote{}
	}
	return SignalQuote{Last: last, Mark: mark, GapBp: (last - mark) / mark * 10000}
}

// OK 报价是否可用（mark 有效）。
func (q SignalQuote) OK() bool { return q.Mark > 0 }

// applySignalQuote 给信号类事件（open/cap_skip/gate_block）附加报价字段。
// 三者共同构成信号流——被拦截信号的 gap 分布对分级研究同样有价值，故都要带。
func applySignalQuote(e eventlog.Event, q SignalQuote) eventlog.Event {
	if !q.OK() {
		return e // 不可算：整体省略，不得落 0 冒充真实观测
	}
	e.SigLast, e.SigMark, e.GapBp = q.Last, q.Mark, q.GapBp
	return e
}
