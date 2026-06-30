package calibration

// Prediction 是校准要作用的预测最小要素：中枢幅度、区间上下沿(相对幅度%)与置信度。
// 用裸字段而非 analyzer.Decision，使本包保持叶子包(被 analyzer 依赖而不反向依赖)。
type Prediction struct {
	MovePct    float64 // 中枢幅度%(long>0 / short<0 / neutral=0)
	HighPct    float64 // 区间上沿%，≥0
	LowPct     float64 // 区间下沿%，≤0
	Confidence float64 // 方向把握度 0~1
}

// Calibrator 是「校准回环」反馈进预测的口子：把离线打分学到的修正，作用回单次预测。
// 默认实现是 Noop(恒等)，因此在反馈被显式注入前，预测行为完全不变。
type Calibrator interface {
	Adjust(p Prediction) Prediction
}

// Noop 恒等校准：不改动任何预测要素。默认装配，保证「已落地但未启用」。
type Noop struct{}

// Adjust 原样返回。
func (Noop) Adjust(p Prediction) Prediction { return p }

// Static 用一份固化的校准参数(通常来自一次 Score 的反馈建议)修正预测：
// 区间按 BandScale 缩放、置信度按 ConfShift 平移。零值项不动。
type Static struct {
	BandScale float64 // 区间缩放(0 或 1 = 不动)
	ConfShift float64 // 置信度平移(0 = 不动)
}

// Adjust 按固化参数缩放区间、平移置信度；缩放只动幅度大小不动方向(符号保持)。
func (s Static) Adjust(p Prediction) Prediction {
	if s.BandScale > 0 && s.BandScale != 1 {
		p.HighPct *= s.BandScale
		p.LowPct *= s.BandScale
	}
	if s.ConfShift != 0 {
		p.Confidence = clamp01(p.Confidence + s.ConfShift)
	}
	return p
}

// FromReport 把一次打分的反馈建议固化成 Static 校准器，是「评估 → 反馈」的衔接点。
func FromReport(r Report) Static {
	return Static{BandScale: r.SuggestBandScale, ConfShift: r.SuggestConfShift}
}
