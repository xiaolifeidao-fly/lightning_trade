package signal

import "fmt"

// 精度等级。需求大纲 §3.3 的硬要求：两级结果**不得混排比较**，
// 所以它既落在 run 上也落在 metric 上，前端按它分组展示。
const (
	// FidelityEvent 事件级：信号序列不变，直接回放 strategy_event 的真实触发点，
	// 触发时刻精确到秒。改仓位上限/风险预算/止损/移动止盈分档/反向门控/趋势闸
	// 都属于这一级。
	FidelityEvent = "event"
	// FidelityFrequency 频率级：改了 signal_threshold。历史事件被生产阈值结构性
	// 截断（实测 min = 5.01bp），只能用 dev_sample.DevCross 推 λ(θ) 频率，
	// 推不出每次触发的时刻。
	FidelityFrequency = "frequency"
)

// Fidelity 一组回测结果的可信度描述。Notes 是必须随结果一起展示的警示，
// 不是可选说明——"结果可信"这条验收靠的就是它显式可见。
type Fidelity struct {
	Level            string        `json:"level"`
	ThresholdChanged bool          `json:"thresholdChanged"`
	Notes            []string      `json:"notes"`
	Lambda           *LambdaResult `json:"lambda,omitempty"`
}

// Note 把 Notes 拼成一行，供 varchar 列落库。
func (f Fidelity) Note() string {
	out := ""
	for i, n := range f.Notes {
		if i > 0 {
			out += " | "
		}
		out += n
	}
	return out
}

// 所有回放共有的保守假设与已知偏差。写死在这里而不是散在文档里，
// 是为了让每一条 run 都带着它落库。
const (
	NoteMonitorInterval = "出场判定粒度：回测 60 秒（1m K 线），实盘 position.monitor.interval_seconds=5 秒"
	NotePeakBias        = "peakPct 用 1m high 计算，相对实盘 5 秒轮询系统性偏高；移动止盈触发点因此偏早"
	NoteAdverseFirst    = "同一根内可能同时触及止盈与止损时按不利方向结算（先兜底止损），宁可低估"
	NoteNoMarkInKline   = "1m K 线只用于持仓期间的路径回放；K 线无 mark price，绝不用于推导信号"
	NoteDualMode        = "双向持仓不是实盘形态（实盘为净仓 + 反向门控），仅作研究对照，不可与净仓组混排"
	NoteEntrySigLast    = "成交价取事件的 sigLast（触发瞬间 DeepCoin last），与实盘 trade.signal.delay_seconds=5 秒后的成交价不同源"
	NoteEvalClose       = "移动止盈按收盘价评估：一根内的极值不参与判定，相对实盘偏乐观"
	NoteEvalPessimistic = "移动止盈按一根内不利极值评估、兜底按极值成交：相对实盘偏悲观"
	NoteTrendGateBar    = "趋势闸动量由 1m 收盘序列推出（实盘是 tick 流按分钟去重），判定时刻按根对齐，最多差一根"
)

// Classify 判定精度等级并生成必须随结果展示的警示清单。
func Classify(p Params) Fidelity {
	p = p.Normalize()
	f := Fidelity{Level: FidelityEvent}
	f.Notes = append(f.Notes, NoteNoMarkInKline, NoteMonitorInterval, NotePeakBias, NoteAdverseFirst)

	switch p.EvalMode {
	case EvalPessimistic:
		f.Notes = append(f.Notes, NoteEvalPessimistic)
	default:
		f.Notes = append(f.Notes, NoteEvalClose)
	}
	if p.EntryPx == EntrySigLast {
		f.Notes = append(f.Notes, NoteEntrySigLast)
	}
	if p.Mode == ModeDual {
		f.Notes = append(f.Notes, NoteDualMode)
	}
	if p.TrendGateThresholdPct > 0 && p.TrendGateWindowHours > 0 {
		f.Notes = append(f.Notes, NoteTrendGateBar)
	}

	// signal_threshold 是唯一一个会改变信号序列本身的旋钮。
	if p.SignalThresholdBp > 0 && p.BaselineThresholdBp > 0 &&
		!nearlyEqual(p.SignalThresholdBp, p.BaselineThresholdBp) {
		f.Level = FidelityFrequency
		f.ThresholdChanged = true
		f.Notes = append(f.Notes, fmt.Sprintf(
			"signal_threshold 由 %.2fbp 改为 %.2fbp：历史事件被生产阈值结构性截断，本组只能推 λ(θ) 频率，推不出每次触发时刻",
			p.BaselineThresholdBp, p.SignalThresholdBp))
		if p.SignalThresholdBp > p.BaselineThresholdBp {
			f.Notes = append(f.Notes,
				"抬高阈值的 PnL 由 |gapBp| ≥ θ 门限筛近似得到；触发时刻仍取自 θ0 流，edge-trigger 的 in-band re-arm 动态未重推")
		} else {
			f.Notes = append(f.Notes,
				"降低阈值无法从事件流补出新触发点：本组只输出 λ(θ)，不产出 PnL，不得与事件级结果并列排序")
		}
	}
	return f
}

// Comparable 两组结果是否可以横向比较：精度等级不同一律不可比。
// r11 的参数组对比、r16 的寻优都要先过这一关。
func Comparable(a, b Fidelity) bool { return a.Level == b.Level }

func nearlyEqual(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}
