package signal

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// DevWindow 一条无条件偏离采样窗口（dev_sample 事件 / dev_sample 表一行）。
//
// 它是频率级回测唯一可信的数据源：GapBp 只在信号触发时落盘、被生产阈值
// 结构性截断（实测 min = 5.01bp），看不到分布主体；DevCross 是不经阈值过滤的
// 穿越计数，直接就是 λ(θ) 的测量值（见 pkg/monitor/dev_sampler.go）。
type DevWindow struct {
	Ts    time.Time
	Ticks int
	Cross map[float64]int // 候选阈值(bp) → 窗口内穿越次数
}

// LambdaPoint λ 曲线上的一格。
type LambdaPoint struct {
	ThresholdBp float64 `json:"thresholdBp"`
	CrossTotal  int     `json:"crossTotal"`
	PerDay      float64 `json:"perDay"`
}

// LambdaResult θ 变更后的频率推断结果。
type LambdaResult struct {
	Windows   int           `json:"windows"`
	Ticks     int           `json:"ticks"`
	SpanHours float64       `json:"spanHours"`
	Curve     []LambdaPoint `json:"curve"`

	BaselineBp     float64 `json:"baselineBp"`
	TargetBp       float64 `json:"targetBp"`
	BaselinePerDay float64 `json:"baselinePerDay"`
	TargetPerDay   float64 `json:"targetPerDay"`
	Ratio          float64 `json:"ratio"` // λ(θ)/λ(θ0)：触发频率的相对变化

	// 自检：θ0 下由 DevCross 推出的 λ 应当与真实事件流的触发密度一致
	// （dev_sampler 与生产判定共用 EvaluateDeviationSignal，口径不该漂移）。
	// SelfCheckRatio 明显偏离 1 就说明采样覆盖不全或阈值口径对不上，
	// 此时 Ratio 不可信——这正是"结果可信度显式可见"要暴露的东西。
	ObservedPerDay float64 `json:"observedPerDay"`
	SelfCheckRatio float64 `json:"selfCheckRatio"`

	// Interpolated 目标阈值不在候选阈值列表里，λ 由相邻两格对数线性插值得到。
	Interpolated bool   `json:"interpolated"`
	Warning      string `json:"warning"`
}

// Lambda 由 dev_sample 窗口推 λ(θ) 与 λ(θ)/λ(θ0)。
//
// observedSignals/bars 只用于自检（把真实事件流的触发密度算出来做对照），
// 不参与 λ 本身的计算。
func Lambda(windows []DevWindow, baselineBp, targetBp float64, observedSignals int, bars []Bar) (*LambdaResult, error) {
	if len(windows) == 0 {
		return nil, fmt.Errorf("区间内没有 dev_sample 采样窗口")
	}
	if baselineBp <= 0 || targetBp <= 0 {
		return nil, fmt.Errorf("baseline/target 阈值必须为正, got %.4f/%.4f", baselineBp, targetBp)
	}

	rows := append([]DevWindow(nil), windows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Ts.Before(rows[j].Ts) })

	// 覆盖时长 = 首末窗口间隔 + 一个窗口步长（窗口是右端时刻，首窗自身也占一个步长）。
	span := rows[len(rows)-1].Ts.Sub(rows[0].Ts) + windowStep(rows)
	spanHours := span.Hours()
	if spanHours <= 0 {
		return nil, fmt.Errorf("dev_sample 覆盖时长为 0，无法推频率")
	}

	totals := map[float64]int{}
	ticks := 0
	for _, w := range rows {
		ticks += w.Ticks
		for th, n := range w.Cross {
			totals[th] += n
		}
	}
	if len(totals) == 0 {
		return nil, fmt.Errorf("dev_sample 的 devCross 全为空：采样器未上线或窗口内零穿越")
	}

	res := &LambdaResult{
		Windows:    len(rows),
		Ticks:      ticks,
		SpanHours:  spanHours,
		BaselineBp: baselineBp,
		TargetBp:   targetBp,
	}
	days := spanHours / 24
	thresholds := make([]float64, 0, len(totals))
	for th := range totals {
		thresholds = append(thresholds, th)
	}
	sort.Float64s(thresholds)
	for _, th := range thresholds {
		res.Curve = append(res.Curve, LambdaPoint{ThresholdBp: th, CrossTotal: totals[th], PerDay: float64(totals[th]) / days})
	}

	var interpBase, interpTarget bool
	res.BaselinePerDay, interpBase = lambdaAt(res.Curve, baselineBp)
	res.TargetPerDay, interpTarget = lambdaAt(res.Curve, targetBp)
	res.Interpolated = interpBase || interpTarget
	if res.BaselinePerDay > 0 {
		res.Ratio = res.TargetPerDay / res.BaselinePerDay
	}

	if observedSignals > 0 {
		obsDays := days
		if len(bars) > 1 {
			// 事件流的覆盖窗口用 K 线区间更准（dev_sample 可能只覆盖其中一段）
			sorted := SortBars(bars)
			if d := sorted[len(sorted)-1].Ts.Sub(sorted[0].Ts).Hours() / 24; d > 0 {
				obsDays = d
			}
		}
		res.ObservedPerDay = float64(observedSignals) / obsDays
		if res.BaselinePerDay > 0 {
			res.SelfCheckRatio = res.ObservedPerDay / res.BaselinePerDay
		}
	}

	switch {
	case res.BaselinePerDay <= 0:
		res.Warning = fmt.Sprintf("θ0=%.2fbp 的穿越计数为 0，λ 比值不可用", baselineBp)
	case res.SelfCheckRatio > 0 && (res.SelfCheckRatio < 0.5 || res.SelfCheckRatio > 2):
		res.Warning = fmt.Sprintf("自检失败：θ0 的 λ=%.1f 次/天 与真实事件密度 %.1f 次/天 相差 %.2f 倍，λ 推断不可信",
			res.BaselinePerDay, res.ObservedPerDay, res.SelfCheckRatio)
	case res.Interpolated:
		res.Warning = "目标阈值不在 dev_sample 候选阈值内，λ 由相邻两格对数线性插值得到"
	}
	return res, nil
}

// windowStep 采样窗口步长：取相邻窗口间隔的中位数（r3 把粒度从 1min 提到 10s 后，
// 同一张表里可能并存两种步长，用中位数比用首个间隔稳）。
func windowStep(rows []DevWindow) time.Duration {
	if len(rows) < 2 {
		return time.Minute
	}
	gaps := make([]time.Duration, 0, len(rows)-1)
	for i := 1; i < len(rows); i++ {
		if g := rows[i].Ts.Sub(rows[i-1].Ts); g > 0 {
			gaps = append(gaps, g)
		}
	}
	if len(gaps) == 0 {
		return time.Minute
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i] < gaps[j] })
	return gaps[len(gaps)/2]
}

// lambdaAt 取某阈值处的 λ。命中候选阈值直接取；否则在相邻两格之间做
// 对数线性插值（λ(θ) 随 θ 近似指数衰减，线性插值会系统性高估）。
// 越界时钳到端点并标记为插值（结果偏保守，且会带 Warning）。
func lambdaAt(curve []LambdaPoint, th float64) (float64, bool) {
	if len(curve) == 0 {
		return 0, false
	}
	for _, p := range curve {
		if nearlyEqual(p.ThresholdBp, th) {
			return p.PerDay, false
		}
	}
	if th <= curve[0].ThresholdBp {
		return curve[0].PerDay, true
	}
	if th >= curve[len(curve)-1].ThresholdBp {
		return curve[len(curve)-1].PerDay, true
	}
	for i := 1; i < len(curve); i++ {
		lo, hi := curve[i-1], curve[i]
		if th > hi.ThresholdBp {
			continue
		}
		if lo.PerDay <= 0 || hi.PerDay <= 0 {
			// 有一端是 0，对数插值无定义，退化成线性
			w := (th - lo.ThresholdBp) / (hi.ThresholdBp - lo.ThresholdBp)
			return lo.PerDay + w*(hi.PerDay-lo.PerDay), true
		}
		w := (th - lo.ThresholdBp) / (hi.ThresholdBp - lo.ThresholdBp)
		return math.Exp(math.Log(lo.PerDay) + w*(math.Log(hi.PerDay)-math.Log(lo.PerDay))), true
	}
	return curve[len(curve)-1].PerDay, true
}
