package repository

import (
	"math"
	"service/trade/strategy"
	"time"
)

// ParamsFromStrategy 把一行策略配置(DB)映射成引擎参数(strategy.Params)，
// 是「策略定义表」与「共享引擎」之间的桥：引擎据此对每条预测做入场/出场判定。
// repository 依赖 strategy(叶子包)，strategy 不反向依赖，无 import 环。
func ParamsFromStrategy(s *TradeStrategy) strategy.Params {
	return strategy.Params{
		TrendFilter:        trendFilter(s.TrendFilter),
		MinConfidence:      s.MinConfidence,
		MinMovePct:         s.MinMovePct,
		EntryMode:          strategy.EntryMode(s.EntryMode),
		Alpha:              s.EntryAlpha,
		Gamma:              s.ExitGamma,
		EntryTTL:           time.Duration(s.EntryTTL) * time.Second,
		EfficiencyRoute:    s.EfficiencyRoute,
		TakeProfitSource:   s.TakeProfitSource,
		StopLossSource:     s.StopLossSource,
		TakeProfitPct:      s.TakeProfitPct,
		StopLossPct:        s.StopLossPct,
		PredictSLBufferPct: s.PredictSLBufferPct,
		PressureBufferPct:  s.PressureBufferPct,
		TakeProfitFloorPct: s.TakeProfitFloorPct,
		StopLossFloorPct:   s.StopLossFloorPct,
		HoldDuration:       time.Duration(s.HoldDuration) * time.Second,
		MaxHold:            time.Duration(s.MaxHoldDuration) * time.Second,
		Leverage:           s.Leverage,
		Contracts:          s.Contracts,
		MakerFee:           s.MakerFeeRate,
		TakerFee:           s.TakerFeeRate,
	}
}

// PredictionFromRow 把一条历史预测(DB)映射成引擎输入。
// efficiency 库里只存在 RawResponse JSON、无独立列，故按区间现算：|中枢-基准| / 区间宽。
func PredictionFromRow(p *TradeAIPrediction) strategy.Prediction {
	ref := p.OpenPrice
	if ref <= 0 {
		ref = p.RefPrice
	}
	eff := 0.0
	if w := p.PredictHigh - p.PredictLow; w > 0 {
		eff = math.Abs(p.PredictPrice-ref) / w
	}
	movePct := 0.0
	if ref > 0 {
		movePct = (p.PredictPrice - ref) / ref * 100
	}
	return strategy.Prediction{
		Trend:        strategy.Direction(p.Trend),
		RefPrice:     ref,
		High:         p.PredictHigh,
		Low:          p.PredictLow,
		Invalidation: p.Invalidation,
		Confidence:   p.Confidence,
		Efficiency:   eff,
		MovePct:      movePct,
	}
}

// BacktestTradeFromOrder 把一笔终态 Order + 结算 + 预测特征映射成回测逐笔行。
func BacktestTradeFromOrder(runID, predictionID int64, o *strategy.Order, st strategy.Settlement, pr strategy.Prediction) *TradeBacktestTrade {
	row := &TradeBacktestTrade{
		RunID:              runID,
		PredictionID:       predictionID,
		Direction:          string(o.Direction),
		EntryMode:          string(o.EntryMode),
		PlannedEntryPrice:  o.PlannedEntry,
		TakeProfitPrice:    o.TakeProfit,
		StopLossPrice:      o.StopLoss,
		Status:             string(o.State),
		OpenPrice:          o.OpenPrice,
		ClosePrice:         o.ClosePrice,
		CloseReason:        string(o.CloseReason),
		RequestedAt:        o.RequestedAt,
		Pnl:                st.Pnl,
		PnlRate:            st.PnlRate,
		Fee:                st.Fee,
		NetPnl:             st.NetPnl,
		Confidence:         pr.Confidence,
		PredictedMovePct:   pr.MovePct,
		Efficiency:         pr.Efficiency,
		PredHigh:           pr.High,
		PredLow:            pr.Low,
		PressureHigh:       pr.KeyResistance,
		PressureLow:        pr.KeySupport,
		MaxPriceDuringHold: o.MaxPrice,
		MinPriceDuringHold: o.MinPrice,
	}
	if !o.OpenedAt.IsZero() {
		t := o.OpenedAt
		row.OpenedAt = &t
	}
	if !o.ClosedAt.IsZero() {
		t := o.ClosedAt
		row.ClosedAt = &t
	}
	return row
}

// BacktestMetricFromAgg 把聚合指标映射成回测汇总行(按结算口径区分)。
func BacktestMetricFromAgg(runID int64, calcMode string, m strategy.Metric) *TradeBacktestMetric {
	return &TradeBacktestMetric{
		RunID:        runID,
		CalcMode:     calcMode,
		TradeCount:   m.TradeCount,
		FillCount:    m.FillCount,
		ExpiredCount: m.ExpiredCount,
		FillRate:     m.FillRate,
		WinCount:     m.WinCount,
		WinRate:      m.WinRate,
		GrossPnl:     m.GrossPnl,
		FeeTotal:     m.FeeTotal,
		NetPnl:       m.NetPnl,
		Expectancy:   m.Expectancy,
		ProfitFactor: m.ProfitFactor,
		MaxDrawdown:  m.MaxDrawdown,
		Sharpe:       m.Sharpe,
		AvgHoldSecs:  m.AvgHoldSecs,
		TpCount:      m.TpCount,
		SlCount:      m.SlCount,
		TimeoutCount: m.TimeoutCount,
	}
}

// trendFilter 把 trend_filter 文本归一成引擎方向；both/空 → "" 表示不过滤。
func trendFilter(v string) strategy.Direction {
	switch v {
	case "long":
		return strategy.Long
	case "short":
		return strategy.Short
	default:
		return ""
	}
}
