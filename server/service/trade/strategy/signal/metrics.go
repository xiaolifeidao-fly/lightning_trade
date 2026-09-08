package signal

import (
	"math"
	"sort"
)

// DayMTM 一天的 MTM 权益变化。r16 的熊市月 bootstrap 直接吃这个序列，
// 所以由引擎产出而不是让下游从逐笔明细反推。
type DayMTM struct {
	Day   string  `json:"day"`
	Delta float64 `json:"delta"`
	Close float64 `json:"close"`
}

// Metric 一次信号回测的汇总指标。字段刻意与 strategy.Metric（预测驱动）
// 保持同名同义，好让两套结果落进同一张 trade_backtest_metric；
// 信号驱动独有的计数放在后半段。
type Metric struct {
	// 与预测驱动同名的通用指标
	TradeCount   int // 持仓生命周期数（含窗口结束仍未平的）
	WinCount     int
	WinRate      float64
	GrossPnl     float64 // 毛盈亏 = 已实现 + 期末浮动（未扣费）
	FeeTotal     float64
	NetPnl       float64 // 已实现 − 手续费 + 期末浮动
	Expectancy   float64 // 单个生命周期的期望净利
	ProfitFactor float64
	MaxDrawdown  float64 // MTM 口径最大回撤（USDT）
	Sharpe       float64 // 口径同预测驱动：逐笔净利序列的 均值/标准差
	AvgHoldSecs  float64
	TrailCount   int // 移动止盈平仓
	SlCount      int // 兜底止损（复用既有"止损笔数"列）

	// 信号驱动独有
	SignalCount      int // 回放消费的真实触发数（零丢失零重复的验收口径）
	SignalDropped    int // 落在种子之前 / 无 K 线覆盖而未回放的触发
	SignalFiltered   int // 抬高阈值后被 |gapBp| 门限筛掉的触发
	CapSkipCount     int // 仓位上限跳过
	GateSkipCount    int // 反向门控拦截
	TrendSkipCount   int // 趋势闸拦截
	ReduceCount      int // 盈利减仓次数
	ReduceCloseCount int // 被减仓削零结束的生命周期数
	EodOpenCount     int // 窗口结束仍持仓的生命周期数
	MaxStack         int // 最大堆积张数
	RealizedPnl      float64
	FloatingPnl      float64
	MaxDrawdownPct   float64 // 回撤占风险基数（risk_equity）的百分比；基数缺省为 0 时留 0
	CapEffective     int     // 本次回放实际生效的仓位上限

	// 精度分层
	Fidelity       string
	FidelityNote   string
	LambdaPerDay   float64 // 目标阈值下的 λ（次/天），仅频率级
	LambdaRatio    float64 // λ(θ)/λ(θ0)，仅频率级
	LambdaSelfTest float64 // θ0 的 λ 与真实事件密度之比，应 ≈1

	DailyMTM []DayMTM
}

// Aggregate 把回放结果聚合成汇总指标。
func Aggregate(res *Result) Metric {
	m := Metric{
		SignalCount:    res.SignalReplayed,
		SignalDropped:  res.SignalDropped,
		SignalFiltered: res.SignalFiltered,
		CapSkipCount:   res.SkipCap,
		GateSkipCount:  res.SkipGate,
		TrendSkipCount: res.SkipTrend,
		ReduceCount:    res.Reduces,
		MaxStack:       res.MaxStack,
		RealizedPnl:    res.Realized,
		FloatingPnl:    res.Floating,
		FeeTotal:       res.Fees,
		NetPnl:         res.Net,
		GrossPnl:       res.Realized + res.Floating,
		MaxDrawdown:    res.MaxDrawdown,
		CapEffective:   res.Cap,
		Fidelity:       res.Fidelity.Level,
		FidelityNote:   res.Fidelity.Note(),
		DailyMTM:       dailyMTM(res.Equity),
	}
	if lam := res.Fidelity.Lambda; lam != nil {
		m.LambdaPerDay = lam.TargetPerDay
		m.LambdaRatio = lam.Ratio
		m.LambdaSelfTest = lam.SelfCheckRatio
	}
	if res.Params.RiskEquity > 0 {
		m.MaxDrawdownPct = res.MaxDrawdown / res.Params.RiskEquity * 100
	}

	var grossWin, grossLoss, holdSum float64
	nets := make([]float64, 0, len(res.Episodes))
	for _, ep := range res.Episodes {
		m.TradeCount++
		switch ep.Reason {
		case ExitTrailing:
			m.TrailCount++
		case ExitCatastrophe:
			m.SlCount++
		case ExitReduce:
			m.ReduceCloseCount++
		case ExitEod:
			m.EodOpenCount++
		}
		net := ep.Pnl + ep.ReducedPnl - ep.Fee
		nets = append(nets, net)
		if net > 0 {
			m.WinCount++
			grossWin += net
		} else {
			grossLoss += -net
		}
		if !ep.OpenedAt.IsZero() && !ep.ClosedAt.IsZero() {
			holdSum += ep.ClosedAt.Sub(ep.OpenedAt).Seconds()
		}
	}
	if m.TradeCount > 0 {
		m.WinRate = float64(m.WinCount) / float64(m.TradeCount)
		m.Expectancy = m.NetPnl / float64(m.TradeCount)
		m.AvgHoldSecs = holdSum / float64(m.TradeCount)
	}
	if grossLoss > 0 {
		m.ProfitFactor = grossWin / grossLoss
	}
	m.Sharpe = sharpeOf(nets)
	return m
}

// dailyMTM 把逐根 MTM 曲线折成按日收盘 + 日增量（口径同 backtest_capsf_study.py
// 的 dmtm：首日增量以 0 为起点）。
func dailyMTM(points []EquityPoint) []DayMTM {
	if len(points) == 0 {
		return nil
	}
	closeOf := map[string]float64{}
	order := make([]string, 0, 8)
	for _, p := range points {
		day := p.At.Format("2006-01-02")
		if _, ok := closeOf[day]; !ok {
			order = append(order, day)
		}
		closeOf[day] = p.MTM
	}
	sort.Strings(order)
	out := make([]DayMTM, 0, len(order))
	prev := 0.0
	for _, day := range order {
		v := closeOf[day]
		out = append(out, DayMTM{Day: day, Delta: v - prev, Close: v})
		prev = v
	}
	return out
}

// sharpeOf 逐笔净利序列的夏普（均值/标准差）。口径与 strategy.sharpe 一致，
// 便于两套引擎的结果放在同一列里看。
func sharpeOf(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	varSum := 0.0
	for _, x := range xs {
		d := x - mean
		varSum += d * d
	}
	sd := math.Sqrt(varSum / float64(len(xs)-1))
	if sd == 0 {
		return 0
	}
	return mean / sd
}

// AnnualizedDays 回放覆盖的天数（按 1m 根数折算，用于把结果归一到"每 N 天"口径）。
func AnnualizedDays(res *Result) float64 {
	if res.BarCount <= 0 {
		return 0
	}
	return float64(res.BarCount) / 1440.0
}

// SpanHours 回放实际覆盖的小时数（首末根开盘时刻之差）。
func SpanHours(res *Result) float64 {
	if res.FirstBar.IsZero() || res.LastBar.IsZero() {
		return 0
	}
	return res.LastBar.Sub(res.FirstBar).Hours()
}
