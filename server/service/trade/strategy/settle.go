package strategy

import "math"

// ContractSizeBTC 合约面值：1 张 = 0.001 BTC。开仓数量(BTC) = 张数 × 此值。
const ContractSizeBTC = 0.001

// Settlement 单笔交易的结算结果(USDT 口径)。
type Settlement struct {
	Pnl     float64 // 毛盈亏
	Fee     float64 // 往返手续费
	NetPnl  float64 // 净盈亏 = Pnl - Fee
	PnlRate float64 // 盈亏率%(含杠杆)
}

// Settle 计算一笔终态 Order 的盈亏与手续费。未成交(expired)或未平仓返回零结算。
// 费率口径与实盘一致：开仓市价吃单(Taker)/回踩限价(Maker)，平仓止盈记 Maker、止损与超时记 Taker。
func (o *Order) Settle(p Params) Settlement {
	if o.State != StateClosed || o.OpenPrice <= 0 {
		return Settlement{}
	}
	qty := float64(p.Contracts) * ContractSizeBTC
	var diff float64
	if o.Direction == Long {
		diff = o.ClosePrice - o.OpenPrice
	} else {
		diff = o.OpenPrice - o.ClosePrice
	}

	entryRate := p.TakerFee
	if o.EntryMode == EntryPullback {
		entryRate = p.MakerFee
	}
	closeRate := p.TakerFee
	if o.CloseReason == ReasonTP {
		closeRate = p.MakerFee
	}
	feePerUnit := o.OpenPrice*entryRate + o.ClosePrice*closeRate

	pnl := diff * qty
	fee := feePerUnit * qty
	rate := 0.0
	if o.OpenPrice > 0 {
		rate = diff / o.OpenPrice * p.Leverage * 100
	}
	return Settlement{Pnl: pnl, Fee: fee, NetPnl: pnl - fee, PnlRate: rate}
}

// MarkToMarket 用标记价(最新价) mark 估算一笔仍在持仓(open)的浮动盈亏。
// 用于回测窗口结束时还没触发止盈/止损/超时的持仓——按当前最新价算目前盈亏。
// 口径与 Settle 一致：入场费按实际入场方式(市价 Taker / 回踩 Maker)，平仓费按市价 Taker 估算(出场原因未知)。
func MarkToMarket(dir Direction, entryMode EntryMode, openPrice, mark float64, p Params) Settlement {
	if openPrice <= 0 || mark <= 0 {
		return Settlement{}
	}
	qty := float64(p.Contracts) * ContractSizeBTC
	var diff float64
	if dir == Long {
		diff = mark - openPrice
	} else {
		diff = openPrice - mark
	}

	entryRate := p.TakerFee
	if entryMode == EntryPullback {
		entryRate = p.MakerFee
	}
	feePerUnit := openPrice*entryRate + mark*p.TakerFee

	pnl := diff * qty
	fee := feePerUnit * qty
	rate := diff / openPrice * p.Leverage * 100
	return Settlement{Pnl: pnl, Fee: fee, NetPnl: pnl - fee, PnlRate: rate}
}

// Metric 一组回测交易的汇总指标(评估层)，是横向对比「哪个策略有效」的依据。
type Metric struct {
	TradeCount   int
	FillCount    int
	ExpiredCount int
	FillRate     float64
	WinCount     int
	WinRate      float64
	GrossPnl     float64
	FeeTotal     float64
	NetPnl       float64
	Expectancy   float64
	ProfitFactor float64
	MaxDrawdown  float64
	Sharpe       float64
	AvgHoldSecs  float64
	TpCount      int
	SlCount      int
	TimeoutCount int
}

// Aggregate 把一批终态 Order 聚合成评估指标。
// orders 应按收尾时间有序，以正确计算回撤(回撤基于累计净值曲线)。
func Aggregate(orders []*Order, p Params) Metric {
	m := Metric{TradeCount: len(orders)}
	var grossWin, grossLoss float64
	var nets []float64
	var holdSum float64
	var equity, peak, maxDD float64

	for _, o := range orders {
		switch o.State {
		case StateExpired:
			m.ExpiredCount++
			continue
		case StateClosed:
			m.FillCount++
		default:
			continue
		}

		st := o.Settle(p)
		m.GrossPnl += st.Pnl
		m.FeeTotal += st.Fee
		m.NetPnl += st.NetPnl
		nets = append(nets, st.NetPnl)
		if st.NetPnl > 0 {
			m.WinCount++
			grossWin += st.NetPnl
		} else {
			grossLoss += -st.NetPnl
		}
		switch o.CloseReason {
		case ReasonTP:
			m.TpCount++
		case ReasonSL:
			m.SlCount++
		case ReasonTimeout:
			m.TimeoutCount++
		}
		if !o.ClosedAt.IsZero() && !o.OpenedAt.IsZero() {
			holdSum += o.ClosedAt.Sub(o.OpenedAt).Seconds()
		}
		equity += st.NetPnl
		if equity > peak {
			peak = equity
		}
		if dd := peak - equity; dd > maxDD {
			maxDD = dd
		}
	}

	if denom := m.FillCount + m.ExpiredCount; denom > 0 {
		m.FillRate = float64(m.FillCount) / float64(denom)
	}
	if m.FillCount > 0 {
		m.WinRate = float64(m.WinCount) / float64(m.FillCount)
		m.Expectancy = m.NetPnl / float64(m.FillCount)
		m.AvgHoldSecs = holdSum / float64(m.FillCount)
	}
	if grossLoss > 0 {
		m.ProfitFactor = grossWin / grossLoss
	}
	m.MaxDrawdown = maxDD
	m.Sharpe = sharpe(nets)
	return m
}

// sharpe 计算单笔净利序列的夏普(均值/标准差)，样本不足或无波动返回 0。
func sharpe(xs []float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	mean := sum / float64(len(xs))
	var varSum float64
	for _, x := range xs {
		d := x - mean
		varSum += d * d
	}
	std := math.Sqrt(varSum / float64(len(xs)-1))
	if std == 0 {
		return 0
	}
	return mean / std
}
