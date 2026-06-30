// Package calibration 实现「校准回环」：用已结算的历史预测，离线评估预测层到底准不准，
// 并把学到的偏差作为反馈作用回下一次预测(区间缩放 / 置信度平移)。
//
// 设计上本包是叶子包——只依赖标准库，不反向依赖 analyzer——因此 analyzer 可安全依赖它而不成环。
// 校准对象用裸字段的 Prediction/Sample 表达，而非 analyzer.Decision。
//
// 当前状态：架子已落地但「未启动」。Source(取数口子)无真实实现、调度不触发，
// 预测默认装配 Noop 校准器，行为与无校准完全一致。详见 source.go / calibrator.go。
package calibration

import "math"

// Sample 是一条「已结算」预测：预测要素 + 真实兑现结果，是校准打分的最小输入。
// 价格统一用绝对价 + 基准价(RefPrice)，打分时内部换算成相对百分比，避免口径分歧。
type Sample struct {
	Trend      string  // 预测方向 long/short/neutral
	Confidence float64 // 预测时标定的方向把握度 0~1
	RefPrice   float64 // 预测基准价(开仓价/参考价)
	PredHigh   float64 // 预测区间上沿(绝对价)
	PredLow    float64 // 预测区间下沿(绝对价)
	PredMove   float64 // 预测中枢/方向锚(绝对价)
	RealHigh   float64 // 预测窗口内真实最高价
	RealLow    float64 // 预测窗口内真实最低价
	RealClose  float64 // 预测窗口到期的真实收盘价
}

// ReliabilityBucket 是一个置信度分桶的可靠性点：桶内预测把握度均值 vs 实际方向命中率。
// 校准良好时 MeanConf ≈ HitRate；MeanConf 系统性高于 HitRate 即「过度自信」。
type ReliabilityBucket struct {
	Lower    float64 // 桶下界(含)
	Upper    float64 // 桶上界(不含；末桶含)
	Count    int     // 桶内样本数
	MeanConf float64 // 桶内预测置信度均值
	HitRate  float64 // 桶内实际方向命中率
}

// Report 是一次校准打分的聚合结果：三组指标 + 给反馈器的建议。
type Report struct {
	Samples int // 参与打分的样本总数

	// ① 置信度可靠性
	Reliability []ReliabilityBucket // 分桶可靠性曲线
	HitRate     float64             // 总体方向命中率(仅 long/short 计入)
	MeanConf    float64             // 总体置信度均值
	ConfBias    float64             // MeanConf - HitRate：>0 过度自信，<0 过度保守

	// ② 区间命中
	ContainRate float64 // 真实[low,high]完全落在预测区间内的比例
	BandUtil    float64 // 区间利用率均值(真实宽/预测宽)：≪1 区间报太宽，>1 报太窄

	// ③ 漂移偏差(相对基准价%)
	DriftBias float64 // mean(真实兑现幅度 - 预测中枢幅度)：>0 实际更涨(预测偏保守)
	DriftMAE  float64 // 平均绝对误差

	// 反馈建议(喂给 StaticCalibrator)
	SuggestBandScale float64 // 区间缩放系数(>1 放宽 / <1 收紧 / 1 不动)
	SuggestConfShift float64 // 置信度平移量(= HitRate - MeanConf)
}

// containTarget 是区间命中率的目标：真实极值约有此比例应落在预测区间内。
// 与 prompt 中「区间标定目标」一致；命中率显著低于它说明区间报得过窄。
const containTarget = 0.8

// Score 对一批已结算样本打分，产出可靠性 / 区间命中 / 漂移偏差三组指标与反馈建议。
// 纯函数、无 IO，便于单测；空样本返回 Samples=0 的零值 Report。
func Score(samples []Sample) Report {
	r := Report{Samples: len(samples)}
	if len(samples) == 0 {
		return r
	}

	const buckets = 10
	bkCount := make([]int, buckets)
	bkConf := make([]float64, buckets)
	bkHit := make([]int, buckets)

	var dirN, dirHit int
	var confSum float64
	var containN, containYes int
	var utilSum float64
	var driftSum, driftAbsSum float64
	var driftN int

	for _, s := range samples {
		// ① 方向命中：仅 long/short 计入(neutral 无方向主张)。
		if s.Trend == "long" || s.Trend == "short" {
			dirN++
			confSum += s.Confidence
			hit := (s.Trend == "long" && s.RealClose > s.RefPrice) ||
				(s.Trend == "short" && s.RealClose < s.RefPrice)
			bi := int(s.Confidence * buckets)
			if bi < 0 {
				bi = 0
			}
			if bi >= buckets {
				bi = buckets - 1
			}
			bkCount[bi]++
			bkConf[bi] += s.Confidence
			if hit {
				dirHit++
				bkHit[bi]++
			}
		}

		// ② 区间命中 + 利用率。
		if s.PredHigh > s.PredLow && s.RealHigh > 0 && s.RealLow > 0 {
			containN++
			if s.RealHigh <= s.PredHigh && s.RealLow >= s.PredLow {
				containYes++
			}
			utilSum += (s.RealHigh - s.RealLow) / (s.PredHigh - s.PredLow)
		}

		// ③ 漂移偏差(相对基准价%)。
		if s.RefPrice > 0 {
			realMove := (s.RealClose - s.RefPrice) / s.RefPrice * 100
			predMove := (s.PredMove - s.RefPrice) / s.RefPrice * 100
			d := realMove - predMove
			driftSum += d
			driftAbsSum += math.Abs(d)
			driftN++
		}
	}

	r.Reliability = make([]ReliabilityBucket, 0, buckets)
	for i := 0; i < buckets; i++ {
		if bkCount[i] == 0 {
			continue
		}
		r.Reliability = append(r.Reliability, ReliabilityBucket{
			Lower:    float64(i) / buckets,
			Upper:    float64(i+1) / buckets,
			Count:    bkCount[i],
			MeanConf: bkConf[i] / float64(bkCount[i]),
			HitRate:  float64(bkHit[i]) / float64(bkCount[i]),
		})
	}

	if dirN > 0 {
		r.HitRate = float64(dirHit) / float64(dirN)
		r.MeanConf = confSum / float64(dirN)
		r.ConfBias = r.MeanConf - r.HitRate
		r.SuggestConfShift = r.HitRate - r.MeanConf
	}
	if containN > 0 {
		r.ContainRate = float64(containYes) / float64(containN)
		r.BandUtil = utilSum / float64(containN)
	}
	if driftN > 0 {
		r.DriftBias = driftSum / float64(driftN)
		r.DriftMAE = driftAbsSum / float64(driftN)
	}

	r.SuggestBandScale = suggestBandScale(r.ContainRate, r.BandUtil)
	return r
}

// suggestBandScale 依据区间命中率与利用率给出区间缩放建议：
// 命中率低于目标 → 放宽(>1)；命中率达标但利用率过低(区间报太宽) → 适度收紧(<1)。
// 夹在 [0.5, 2.0]，避免单次小样本给出极端缩放。
func suggestBandScale(containRate, bandUtil float64) float64 {
	scale := 1.0
	switch {
	case containRate < containTarget:
		scale = 1 + (containTarget - containRate) // 命中越差放得越宽
	case bandUtil > 0 && bandUtil < 0.5:
		scale = 0.5 + bandUtil // 命中达标却太宽，收紧到更贴合真实波动
	}
	return clampRange(scale, 0.5, 2.0)
}

func clamp01(v float64) float64 { return clampRange(v, 0, 1) }

func clampRange(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
