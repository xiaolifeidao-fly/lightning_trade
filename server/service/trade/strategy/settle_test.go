package strategy

import (
	"testing"
	"time"
)

func TestAggregateMetrics(t *testing.T) {
	p := baseParams()
	p.Leverage = 10
	p.Contracts = 1
	p.MakerFee = 0.0002
	p.TakerFee = 0.0005
	base := time.Unix(0, 0).UTC()

	// 一笔止盈、一笔止损、一笔未成交。
	win := &Order{
		Direction: Long, EntryMode: EntryMarket, State: StateClosed, CloseReason: ReasonTP,
		OpenPrice: 60000, ClosePrice: 60600,
		OpenedAt: base, ClosedAt: base.Add(30 * time.Minute),
	}
	loss := &Order{
		Direction: Long, EntryMode: EntryMarket, State: StateClosed, CloseReason: ReasonSL,
		OpenPrice: 60000, ClosePrice: 59700,
		OpenedAt: base, ClosedAt: base.Add(10 * time.Minute),
	}
	expired := &Order{Direction: Long, EntryMode: EntryPullback, State: StateExpired}

	m := Aggregate([]*Order{win, loss, expired}, p)

	if m.TradeCount != 3 || m.FillCount != 2 || m.ExpiredCount != 1 {
		t.Fatalf("计数错误: %+v", m)
	}
	if m.FillRate != 2.0/3.0 {
		t.Fatalf("成交率应=2/3, got %v", m.FillRate)
	}
	if m.WinCount != 1 || m.TpCount != 1 || m.SlCount != 1 {
		t.Fatalf("胜负/出口计数错误: %+v", m)
	}
	// win 毛利 = 0.6*0.001*... 价差600，qty=0.001 → pnl=0.6；loss 价差-300 → pnl=-0.3。
	if m.GrossPnl <= 0 {
		t.Fatalf("毛盈亏应为正, got %v", m.GrossPnl)
	}
	if m.NetPnl >= m.GrossPnl {
		t.Fatalf("净利应扣手续费后小于毛利, gross=%v net=%v", m.GrossPnl, m.NetPnl)
	}
	if m.MaxDrawdown <= 0 {
		t.Fatalf("应有回撤(止损那笔), got %v", m.MaxDrawdown)
	}
}
