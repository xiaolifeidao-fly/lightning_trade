package calibration

import "context"

// Source 是校准数据的来源口子(seam)：把「已结算预测」喂给打分器。
//
// ★ 这是校准回环目前唯一刻意留白处。真实实现应基于 service/trade 的已结算预测
//   (predict_time 已到、settleLoop 已回填真实极值/收盘价)构造 []Sample，
//   每条映射：RefPrice←open/ref、PredHigh/Low/Move←预测区间与中枢、
//   RealHigh/Low←窗口极值、RealClose←到期真实价、Trend/Confidence←预测当时值。
//
// 现仅留接口，未接任何仓储——因此校准回环「已落地但未启动」。
type Source interface {
	// FetchSettled 拉取指定币种/周期、最近 limit 条已结算预测样本。
	FetchSettled(ctx context.Context, coin, interval string, limit int) ([]Sample, error)
}

// Filter 限定一次打分的样本范围。
type Filter struct {
	Coin     string
	Interval string
	Limit    int
}

// Pipeline 把「取数(Source) → 打分(Score) → 固化反馈(FromReport)」串成一次完整校准回环，
// 返回打分报告与可注入预测层的 Calibrator。这是回环的装配点。
//
// src 为 nil(默认/未接线)时直接返回 Noop——保证在 Source 落地前调用方拿到的是恒等校准，
// 预测行为不变。当前没有任何调度调用本函数(未启动)。
func Pipeline(ctx context.Context, src Source, f Filter) (Report, Calibrator, error) {
	if src == nil {
		return Report{}, Noop{}, nil
	}
	samples, err := src.FetchSettled(ctx, f.Coin, f.Interval, f.Limit)
	if err != nil {
		return Report{}, Noop{}, err
	}
	rep := Score(samples)
	if rep.Samples == 0 {
		return rep, Noop{}, nil
	}
	return rep, FromReport(rep), nil
}
