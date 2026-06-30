package strategy

import (
	"context"
	"testing"
	"time"
)

// sliceFeed 是一个用切片回放的 PriceFeed，演示驱动器如何接入引擎(回测驱动器同此形态)。
type sliceFeed struct {
	quotes []Quote
	i      int
}

func (f *sliceFeed) Next(context.Context) (Quote, bool, error) {
	if f.i >= len(f.quotes) {
		return Quote{}, true, nil
	}
	q := f.quotes[f.i]
	f.i++
	return q, false, nil
}

func bar(t time.Time, low, high float64) Quote {
	return Quote{Time: t, Price: (low + high) / 2, High: high, Low: low}
}

// 偏多波段：开 61555 / 区间 61222~62233 / 失效 61100。
func bandPred() Prediction {
	return Prediction{
		Trend: Long, RefPrice: 61555, High: 62233, Low: 61222,
		Invalidation: 61100, Confidence: 0.7, Efficiency: 0.21, MovePct: 0.34,
	}
}

func baseParams() Params {
	return Params{
		MinConfidence: 0.5, MinMovePct: 0.1,
		Alpha: 0.15, Gamma: 0.1, EntryTTL: 30 * time.Minute,
		HoldDuration: time.Hour, Leverage: 10, Contracts: 1,
	}
}

func TestMarketEntryTakeProfit(t *testing.T) {
	s := baseParams()
	s.EntryMode = EntryMarket
	now := time.Unix(0, 0).UTC()
	o, ok := Plan(bandPred(), s, now)
	if !ok || o.State != StateOpen {
		t.Fatalf("market 应立即成交进 open, got ok=%v state=%s", ok, o.State)
	}
	if o.OpenPrice != 61555 {
		t.Fatalf("market 开仓价应=基准价 61555, got %v", o.OpenPrice)
	}
	// 价格冲到区间上沿附近触发止盈(γ=0.1 → 目标 62233-0.1*1011≈62131.9)。
	feed := &sliceFeed{quotes: []Quote{bar(now.Add(time.Minute), 61500, 62200)}}
	if err := Run(context.Background(), &o, feed, NoopSink{}); err != nil {
		t.Fatal(err)
	}
	if o.State != StateClosed || o.CloseReason != ReasonTP {
		t.Fatalf("应止盈平仓, got state=%s reason=%s", o.State, o.CloseReason)
	}
}

func TestPullbackFillThenTP(t *testing.T) {
	s := baseParams()
	s.EntryMode = EntryPullback // 入场 = 61222 + 0.15*1011 ≈ 61373.65
	now := time.Unix(0, 0).UTC()
	o, ok := Plan(bandPred(), s, now)
	if !ok || o.State != StatePending {
		t.Fatalf("pullback 应挂 pending, got ok=%v state=%s", ok, o.State)
	}
	feed := &sliceFeed{quotes: []Quote{
		bar(now.Add(time.Minute), 61350, 61560),   // 回踩触及限价 → 成交
		bar(now.Add(2*time.Minute), 61900, 62200), // 冲高 → 止盈
	}}
	if err := Run(context.Background(), &o, feed, NoopSink{}); err != nil {
		t.Fatal(err)
	}
	if o.State != StateClosed || o.CloseReason != ReasonTP {
		t.Fatalf("应成交后止盈, got state=%s reason=%s", o.State, o.CloseReason)
	}
	if o.OpenPrice <= 0 || o.OpenPrice > 61400 {
		t.Fatalf("成交价应≈限价 61373, got %v", o.OpenPrice)
	}
}

func TestPullbackExpired(t *testing.T) {
	s := baseParams()
	s.EntryMode = EntryPullback
	s.EntryTTL = 2 * time.Minute
	now := time.Unix(0, 0).UTC()
	o, _ := Plan(bandPred(), s, now)
	// 价格一直在限价上方徘徊，挂单超时未成交 → expired(未成交也是一种结果)。
	feed := &sliceFeed{quotes: []Quote{
		bar(now.Add(time.Minute), 61500, 61700),
		bar(now.Add(3*time.Minute), 61500, 61700), // 超过 deadline
	}}
	if err := Run(context.Background(), &o, feed, NoopSink{}); err != nil {
		t.Fatal(err)
	}
	if o.State != StateExpired || o.CloseReason != ReasonExpired {
		t.Fatalf("应超时未成交, got state=%s reason=%s", o.State, o.CloseReason)
	}
}

// 偏空波段：开 62000 / 区间 61222~62233 / 失效 62300。
func shortPred() Prediction {
	return Prediction{
		Trend: Short, RefPrice: 62000, High: 62233, Low: 61222,
		Invalidation: 62300, Confidence: 0.7, Efficiency: 0.21, MovePct: -0.34,
	}
}

func TestPressureStopLossShort(t *testing.T) {
	// 做空 + 压力面止损：止损 = 关键阻力 × (1+buffer%)，止盈 = 关键支撑。
	s := baseParams()
	s.EntryMode = EntryMarket
	s.StopLossSource = SourcePressure
	s.TakeProfitSource = SourcePressure
	s.PressureBufferPct = 0.5 // 突破关键阻力 0.5% 后止损
	p := shortPred()
	p.KeyResistance = 62500 // 上方关键阻力
	p.KeySupport = 61000    // 下方关键支撑
	now := time.Unix(0, 0).UTC()
	o, ok := Plan(p, s, now)
	if !ok || o.State != StateOpen {
		t.Fatalf("market 应立即成交进 open, got ok=%v state=%s", ok, o.State)
	}
	wantSL := 62500 * 1.005 // 关键阻力×(1+0.5%) = 62812.5
	if abs(o.StopLoss-wantSL) > 1e-6 {
		t.Fatalf("压力面止损应=阻力×(1+0.5%%)=%v, got %v", wantSL, o.StopLoss)
	}
	wantTP := 61000 * 1.005 // 关键支撑×(1+0.5%)=61305，空头止盈提前一点离支撑
	if abs(o.TakeProfit-wantTP) > 1e-6 {
		t.Fatalf("压力面止盈应=支撑×(1+0.5%%)=%v, got %v", wantTP, o.TakeProfit)
	}
	// 价格跌破止盈价(61305)触发止盈。
	feed := &sliceFeed{quotes: []Quote{bar(now.Add(time.Minute), 60900, 61800)}}
	if err := Run(context.Background(), &o, feed, NoopSink{}); err != nil {
		t.Fatal(err)
	}
	if o.State != StateClosed || o.CloseReason != ReasonTP {
		t.Fatalf("应到支撑止盈, got state=%s reason=%s", o.State, o.CloseReason)
	}
}

func TestPressureStopLossClampToBand(t *testing.T) {
	// 压力面止损若落在预测区间内，应夹到预测上/下沿，避免被预期内波动扫损。
	now := time.Unix(0, 0).UTC()

	// 空头：阻力 62000 + buffer0 = 62000，落在预测上沿 62233 下方 → 放宽到 62233。
	s := baseParams()
	s.EntryMode = EntryMarket
	s.StopLossSource = SourcePressure
	s.PressureBufferPct = 0
	ps := shortPred()
	ps.RefPrice = 61500 // 开仓价，确保止损在其上方
	ps.KeyResistance = 62000
	ps.KeySupport = 61000
	o, ok := Plan(ps, s, now)
	if !ok {
		t.Fatal("空头应能开仓")
	}
	if abs(o.StopLoss-ps.High) > 1e-6 {
		t.Fatalf("空头止损应夹到预测上沿 %v, got %v", ps.High, o.StopLoss)
	}

	// 多头：支撑 61500 - buffer0 = 61500，落在预测下沿 61222 上方 → 收紧到 61222。
	pl := bandPred()
	pl.KeySupport = 61500
	pl.KeyResistance = 62800
	o2, ok2 := Plan(pl, s, now)
	if !ok2 {
		t.Fatal("多头应能开仓")
	}
	if abs(o2.StopLoss-pl.Low) > 1e-6 {
		t.Fatalf("多头止损应夹到预测下沿 %v, got %v", pl.Low, o2.StopLoss)
	}
}

func TestStopLossFloor(t *testing.T) {
	// 兜底最小止损%：止损太近(亏损<floor)放宽到 floor；已达 floor 则不动。
	now := time.Unix(0, 0).UTC()
	s := baseParams()
	s.EntryMode = EntryMarket
	s.Leverage = 1 // floor 用含杠杆口径，杠杆=1 时 floorPct 即价格%
	s.StopLossSource = SourcePercent
	s.StopLossPct = 0.3 // 0.3% 止损，离入场很近

	// 空头 entry=62000，0.3% 止损=62186；floor=1% → 放宽到 62000×1.01=62620。
	s.StopLossFloorPct = 1.0
	o, ok := Plan(shortPred(), s, now)
	if !ok {
		t.Fatal("空头应能开仓")
	}
	if want := 62000 * 1.01; abs(o.StopLoss-want) > 1e-6 {
		t.Fatalf("空头止损应兜底到 %v, got %v", want, o.StopLoss)
	}

	// 多头 entry=61555，0.3% 止损=61370.3；floor=1% → 放宽到 61555×0.99=60939.45。
	o2, ok2 := Plan(bandPred(), s, now)
	if !ok2 {
		t.Fatal("多头应能开仓")
	}
	if want := 61555 * 0.99; abs(o2.StopLoss-want) > 1e-6 {
		t.Fatalf("多头止损应兜底到 %v, got %v", want, o2.StopLoss)
	}

	// floor 小于实际亏损时不约束：floor=0.1% < 0.3% → 止损保持 62186。
	s.StopLossFloorPct = 0.1
	o3, _ := Plan(shortPred(), s, now)
	if want := 62000 * 1.003; abs(o3.StopLoss-want) > 1e-6 {
		t.Fatalf("亏损已超兜底不应放宽, want %v got %v", want, o3.StopLoss)
	}

	// 含杠杆口径：100x + floor=150% → 价格幅度 1.5%；空头 0.3% 止损 → 放宽到 62000×1.015。
	s.Leverage = 100
	s.StopLossFloorPct = 150
	o4, _ := Plan(shortPred(), s, now)
	if want := 62000 * 1.015; abs(o4.StopLoss-want) > 1e-6 {
		t.Fatalf("含杠杆兜底止损应=62000×1.015, got %v", o4.StopLoss)
	}
}

func TestTakeProfitFloor(t *testing.T) {
	// 兜底锁盈%：止盈目标比 floor 更远时提前到 floor 锁盈；目标更近则不动。
	now := time.Unix(0, 0).UTC()
	s := baseParams()
	s.EntryMode = EntryMarket
	s.Leverage = 1                     // floor 含杠杆口径，杠杆=1 时 floorPct 即价格%
	s.TakeProfitSource = SourcePredict // 走预测区间 γ

	// 多头 entry=61555，预测 γ 止盈=62131.9(≈0.94%)；floor=0.5% → 提前到 61555×1.005。
	s.TakeProfitFloorPct = 0.5
	o, ok := Plan(bandPred(), s, now)
	if !ok {
		t.Fatal("多头应能开仓")
	}
	if want := 61555 * 1.005; abs(o.TakeProfit-want) > 1e-6 {
		t.Fatalf("多头止盈应兜底锁盈到 %v, got %v", want, o.TakeProfit)
	}

	// 空头 entry=62000，预测 γ 止盈=61323.1(≈1.09%)；floor=0.5% → 上移到 62000×0.995。
	o2, ok2 := Plan(shortPred(), s, now)
	if !ok2 {
		t.Fatal("空头应能开仓")
	}
	if want := 62000 * 0.995; abs(o2.TakeProfit-want) > 1e-6 {
		t.Fatalf("空头止盈应兜底锁盈到 %v, got %v", want, o2.TakeProfit)
	}

	// floor 比实际止盈目标还远时不动：多头 floor=2% > 0.94% → 保持 62131.9。
	s.TakeProfitFloorPct = 2.0
	o3, _ := Plan(bandPred(), s, now)
	if want := 62233 - 0.1*(62233-61222); abs(o3.TakeProfit-want) > 1e-6 {
		t.Fatalf("止盈目标更近不应改动, want %v got %v", want, o3.TakeProfit)
	}

	// 含杠杆口径：100x + floor=50% → 价格幅度 0.5%；空头预测目标更远 → 上移到 62000×0.995。
	s.Leverage = 100
	s.TakeProfitFloorPct = 50
	o4, _ := Plan(shortPred(), s, now)
	if want := 62000 * 0.995; abs(o4.TakeProfit-want) > 1e-6 {
		t.Fatalf("含杠杆兜底锁盈应=62000×0.995, got %v", o4.TakeProfit)
	}
}

func TestPressureStopLossFallback(t *testing.T) {
	// 选了压力面但当条预测无压力面数据 → 优雅回退到失效价/区间口径。
	s := baseParams()
	s.EntryMode = EntryMarket
	s.StopLossSource = SourcePressure
	s.TakeProfitSource = SourcePressure
	now := time.Unix(0, 0).UTC()
	o, ok := Plan(bandPred(), s, now) // bandPred 无 KeyResistance/KeySupport
	if !ok {
		t.Fatal("缺压力面应回退而非拒绝开仓")
	}
	// 多头回退：止损=失效价 61100，止盈=区间 γ 推导(>开仓价)。
	if o.StopLoss != 61100 {
		t.Fatalf("回退止损应=失效价 61100, got %v", o.StopLoss)
	}
	if o.TakeProfit <= o.OpenPrice {
		t.Fatalf("回退止盈应在开仓价上方, got tp=%v open=%v", o.TakeProfit, o.OpenPrice)
	}
}
