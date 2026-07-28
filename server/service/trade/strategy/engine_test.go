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

func TestTrailingTakeProfit(t *testing.T) {
	// 移动止盈(峰值回撤)：浮盈冲高后回撤掉 giveback 比例即落袋，而非死等更远的静态止盈/超时。
	s := baseParams()
	s.EntryMode = EntryMarket
	s.Leverage = 1 // 杠杆=1，ROI% 即价格幅度%，便于核对
	s.TakeProfitSource = SourcePercent
	s.TakeProfitPct = 3.0 // 静态止盈挂很远(3%)，确保本笔由移动止盈而非静态止盈收尾
	s.StopLossPct = 5.0
	s.StopLossSource = SourcePercent
	s.TrailActivatePct = 1.0  // 浮盈达 1% 激活
	s.TrailGiveback = 0.5     // 从峰值回撤 50% 平仓
	s.HoldDuration = time.Hour
	now := time.Unix(0, 0).UTC()

	o, ok := Plan(bandPred(), s, now) // 多头 entry=61555
	if !ok || o.State != StateOpen {
		t.Fatalf("应市价开仓, got ok=%v state=%s", ok, o.State)
	}
	// bar1 冲高到 +2%(61555×1.02=62786.1) → 激活并记峰值 2%；未触静态止盈(3%)。
	// bar2 回落：峰值 2%、giveback 0.5 → 退出 ROI=1%，退出价=61555×1.01=62170.55；
	//      本根 Low=62000(<62170.55) → 移动止盈触发，平在退出价。
	feed := &sliceFeed{quotes: []Quote{
		bar(now.Add(time.Minute), 61600, 62786.1),
		bar(now.Add(2*time.Minute), 62000, 62500),
	}}
	if err := Run(context.Background(), &o, feed, NoopSink{}); err != nil {
		t.Fatal(err)
	}
	if o.State != StateClosed || o.CloseReason != ReasonTrail {
		t.Fatalf("应移动止盈平仓, got state=%s reason=%s", o.State, o.CloseReason)
	}
	if want := 61555 * 1.01; abs(o.ClosePrice-want) > 1e-6 {
		t.Fatalf("移动止盈退出价应=峰值2%%回撤50%%→1%%处=%v, got %v", want, o.ClosePrice)
	}
}

func TestTrailingTimeConvergence(t *testing.T) {
	// ④时间收敛：同样的峰值，临近交易周期末 r 收敛变小 → 更早触发移动止盈。
	mk := func() Order {
		return Order{
			Direction: Long, OpenPrice: 100, Leverage: 1, HoldDuration: time.Hour,
			OpenedAt: time.Unix(0, 0).UTC(), State: StateOpen,
			TrailActivatePct: 1, TrailGiveback: 0.5, TrailGivebackMin: 0.1,
			TrailActive: true, PeakPnlPct: 10, // 峰值 10%
		}
	}
	// τ=0：r=0.5 → 退出 ROI=10×0.5=5% → 退出价=105。
	o := mk()
	if px, _ := o.trailExitPrice(o.OpenedAt); abs(px-105) > 1e-6 {
		t.Fatalf("周期初退出价应=105, got %v", px)
	}
	// τ=1(周期末)：r 收敛到 0.1 → 退出 ROI=10×0.9=9% → 退出价=109(更贴近峰值，锁得更紧)。
	end := o.OpenedAt.Add(time.Hour)
	if px, _ := o.trailExitPrice(end); abs(px-109) > 1e-6 {
		t.Fatalf("周期末退出价应=109, got %v", px)
	}
}

func TestFavPeakDeciles(t *testing.T) {
	// 分时段峰值浮盈：多头开 100、杠杆 1、持仓 100s。
	// 5s(前10%)最高 101→+1%；15s(前20%)最高 103→+3%；55s(前60%)最高 102→+2%(不创新高)。
	o := Order{
		Direction: Long, OpenPrice: 100, Leverage: 1, HoldDuration: 100 * time.Second,
		OpenedAt: time.Unix(0, 0).UTC(), State: StateOpen,
	}
	base := o.OpenedAt
	o.track(bar(base.Add(5*time.Second), 100, 101))
	o.track(bar(base.Add(15*time.Second), 100, 103))
	o.track(bar(base.Add(55*time.Second), 100, 102))

	d := o.FavPeakDeciles()
	// 前10%=+1；前20%起累积峰值=+3，并前向填充到末段(第60%的+2不创新高)。
	want := [10]float64{1, 3, 3, 3, 3, 3, 3, 3, 3, 3}
	for i := range want {
		if abs(d[i]-want[i]) > 1e-9 {
			t.Fatalf("decile[%d] 期望 %v, got %v (全部=%v)", i, want[i], d[i], d)
		}
	}
}

func TestFavPeakDecilesNeverFavorable(t *testing.T) {
	// 空头全程逆行：价格从没低于成交价 → 各时段峰值浮盈全为 0，但 FavTracked 为真(有观测)，需落库。
	o := Order{
		Direction: Short, OpenPrice: 100, Leverage: 100, HoldDuration: 100 * time.Second,
		OpenedAt: time.Unix(0, 0).UTC(), State: StateOpen,
	}
	base := o.OpenedAt
	o.track(bar(base.Add(10*time.Second), 100, 103)) // 最低=成交价，最高逆行
	o.track(bar(base.Add(60*time.Second), 101, 104))
	if !o.FavTracked() {
		t.Fatalf("有观测应 FavTracked=true")
	}
	if d := o.FavPeakDeciles(); d != [10]float64{} {
		t.Fatalf("全程零浮盈应返回全 0 十分位, got %v", d)
	}
}

func TestEarlyCutTriggered(t *testing.T) {
	// 多头开 100、100x、持仓 100s；前 50% 时间内浮盈须达 15% ROI(=0.15%价格)，否则离场。
	base := time.Unix(0, 0).UTC()
	o := Order{
		Direction: Long, Leverage: 100, HoldDuration: 100 * time.Second,
		EarlyCutTimePct: 50, EarlyCutMinProfitPct: 15,
		TakeProfit: 200, StopLoss: 1, // 远端，确保不被 TP/SL 抢先
	}
	o.fill(100, base)
	// 10s(前10%)：还没到时间点，不离场。
	if o.Step(bar(base.Add(10*time.Second), 99.9, 100.05)); o.State != StateOpen {
		t.Fatalf("10s 时不应离场, state=%v", o.State)
	}
	// 60s(前60%)：峰值浮盈=roiAt(100.10)=10% < 15% → 早段疲软离场，按当时价平仓。
	q := bar(base.Add(60*time.Second), 99.95, 100.10)
	o.Step(q)
	if o.State != StateClosed || o.CloseReason != ReasonEarlyCut {
		t.Fatalf("应早段疲软离场, state=%v reason=%v", o.State, o.CloseReason)
	}
	if o.ClosePrice != q.Price {
		t.Fatalf("应按当时价(%v)平仓, got %v", q.Price, o.ClosePrice)
	}
}

func TestEarlyCutSkippedWhenProfitReached(t *testing.T) {
	// 前 50% 内浮盈已达标(≥15%)：不触发离场，放行到超时。
	base := time.Unix(0, 0).UTC()
	o := Order{
		Direction: Long, Leverage: 100, HoldDuration: 100 * time.Second,
		EarlyCutTimePct: 50, EarlyCutMinProfitPct: 15,
		TakeProfit: 200, StopLoss: 1,
	}
	o.fill(100, base)
	o.Step(bar(base.Add(20*time.Second), 100, 100.20)) // 浮盈达 20% ≥ 15%
	o.Step(bar(base.Add(60*time.Second), 100, 100.10)) // 过时间点但已达标 → 不离场
	if o.State != StateOpen {
		t.Fatalf("已达利润门槛不应离场, state=%v reason=%v", o.State, o.CloseReason)
	}
	o.Step(bar(base.Add(100*time.Second), 100, 100.10)) // 到期 → 超时
	if o.State != StateClosed || o.CloseReason != ReasonTimeout {
		t.Fatalf("应超时平仓, state=%v reason=%v", o.State, o.CloseReason)
	}
}

func TestEarlyAdverseTriggered(t *testing.T) {
	// 多头开 100、100x；逆行浮亏达 30% ROI(=0.30%价格=99.70)即软止损离场，按软止损价平仓。
	// 无解除闸门(arm=0)、硬止损设远端(1)：证明逆行时软止损先于硬止损触发。
	base := time.Unix(0, 0).UTC()
	o := Order{
		Direction: Long, Leverage: 100, HoldDuration: 100 * time.Second,
		EarlyCutMaxAdversePct: 30,
		TakeProfit:            200, StopLoss: 1,
	}
	o.fill(100, base)
	// 本笔从未走出浮盈，逆行下探到 99.65 ≤ 99.70 → 早段逆行离场，成交价=软止损价 99.70。
	o.Step(bar(base.Add(30*time.Second), 99.65, 99.95))
	if o.State != StateClosed || o.CloseReason != ReasonEarlyAdverse {
		t.Fatalf("应早段逆行离场, state=%v reason=%v", o.State, o.CloseReason)
	}
	if o.ClosePrice != 99.70 {
		t.Fatalf("应按软止损价 99.70 平仓, got %v", o.ClosePrice)
	}
}

func TestEarlyAdverseBeforeHardStop(t *testing.T) {
	// 同根 K 线软止损(99.50)与硬止损(99)都被触及：武装态取更近的软止损，减损离场。
	base := time.Unix(0, 0).UTC()
	o := Order{
		Direction: Long, Leverage: 100, HoldDuration: 100 * time.Second,
		EarlyCutMaxAdversePct: 50,
		TakeProfit:            200, StopLoss: 99, // 硬止损 -100% ROI
	}
	o.fill(100, base)
	o.Step(bar(base.Add(30*time.Second), 98.5, 100)) // 大阴线击穿两者
	if o.State != StateClosed || o.CloseReason != ReasonEarlyAdverse {
		t.Fatalf("应软止损先于硬止损离场, state=%v reason=%v", o.State, o.CloseReason)
	}
	if o.ClosePrice != 99.50 {
		t.Fatalf("应按软止损价 99.50 平仓(减损), got %v", o.ClosePrice)
	}
}

func TestEarlyAdverseDisarmedAfterProfit(t *testing.T) {
	// 曾走出浮盈(峰值≥arm 20%)后解除软止损：即便逆行到阈值也不再软止损，放行扛单直到超时。
	base := time.Unix(0, 0).UTC()
	o := Order{
		Direction: Long, Leverage: 100, HoldDuration: 100 * time.Second,
		EarlyCutMaxAdversePct: 30, EarlyCutArmProfitPct: 20,
		TakeProfit: 200, StopLoss: 1,
	}
	o.fill(100, base)
	o.Step(bar(base.Add(20*time.Second), 100, 100.25)) // 峰值浮盈 25% ≥ 20% → 解除
	o.Step(bar(base.Add(60*time.Second), 99.60, 99.95)) // 逆行到软止损位下方，但已解除 → 不离场
	if o.State != StateOpen {
		t.Fatalf("已解除软止损不应离场, state=%v reason=%v", o.State, o.CloseReason)
	}
	o.Step(bar(base.Add(100*time.Second), 99.80, 100)) // 到期 → 超时
	if o.State != StateClosed || o.CloseReason != ReasonTimeout {
		t.Fatalf("应超时平仓, state=%v reason=%v", o.State, o.CloseReason)
	}
}

func TestCompositeDirGate(t *testing.T) {
	s := baseParams()
	s.EntryMode = EntryMarket
	s.RequireCompositeDir = true
	now := time.Unix(0, 0).UTC()
	p := bandPred() // 偏多波段
	if p.Trend != Long {
		t.Fatalf("前置：bandPred 应为 long")
	}

	// 4h/12h/1d 全部 long → 放行。
	p.HigherTrends = []Direction{Long, Long, Long}
	if _, ok := Plan(p, s, now); !ok {
		t.Fatalf("复合方向全部一致应放行")
	}
	// 有一个 short → 忽略不建仓。
	p.HigherTrends = []Direction{Long, Short, Long}
	if _, ok := Plan(p, s, now); ok {
		t.Fatalf("复合方向不一致应拒单")
	}
	// 有一个缺失(空) → 忽略。
	p.HigherTrends = []Direction{Long, "", Long}
	if _, ok := Plan(p, s, now); ok {
		t.Fatalf("复合方向缺失应拒单")
	}
	// 关闭门槛 → 不受复合方向影响，放行。
	s.RequireCompositeDir = false
	p.HigherTrends = []Direction{Long, Short, Long}
	if _, ok := Plan(p, s, now); !ok {
		t.Fatalf("未启用复合门槛应放行")
	}
}

func TestFavPeakDecilesEmptyWhenNoHold(t *testing.T) {
	// 未成交/无持仓时长：不记录，返回全 0。
	o := Order{Direction: Long, OpenPrice: 100, Leverage: 1, OpenedAt: time.Unix(0, 0).UTC()}
	o.track(bar(o.OpenedAt.Add(time.Second), 100, 105))
	if d := o.FavPeakDeciles(); d != [10]float64{} {
		t.Fatalf("无 HoldDuration 应返回全 0, got %v", d)
	}
}

func TestTrailingDisabledByDefault(t *testing.T) {
	// 不配 TrailActivatePct 时移动止盈整套关闭，行为与现状静态止盈一致。
	s := baseParams()
	s.EntryMode = EntryMarket
	now := time.Unix(0, 0).UTC()
	o, _ := Plan(bandPred(), s, now)
	feed := &sliceFeed{quotes: []Quote{
		bar(now.Add(time.Minute), 61600, 62100), // 冲高
		bar(now.Add(2*time.Minute), 61300, 61700), // 回落
	}}
	if err := Run(context.Background(), &o, feed, NoopSink{}); err != nil {
		t.Fatal(err)
	}
	if o.TrailActive {
		t.Fatalf("未配置移动止盈不应激活")
	}
	if o.CloseReason == ReasonTrail {
		t.Fatalf("未配置移动止盈不应以 trail 收尾")
	}
}

func TestRejectTakeProfitOnWrongSide(t *testing.T) {
	// 交易周期区间整体落在入场价下方(TPBandHigh/Low < 入场价)：多头止盈会被算到入场价下方，
	// 开仓当根即触及 → 曾被误记为"止盈"却实为亏损。应源头拒单。
	s := baseParams()
	s.EntryMode = EntryMarket
	s.TakeProfitSource = SourcePredict
	s.StopLossSource = SourcePredict
	now := time.Unix(0, 0).UTC()

	p := bandPred() // 多头，入场=RefPrice=61555
	// 交易周期覆盖区间整体在入场价下方 → bandTakeProfit(High-γ宽) < 入场价。
	p.TPBandHigh, p.TPBandLow, p.TPInvalidation = 60500, 59500, 60600
	if _, ok := Plan(p, s, now); ok {
		t.Fatal("止盈价落在入场价下方(矛盾单)应拒单，而非伪止盈")
	}

	// 空头镜像：交易周期区间整体在入场价上方 → 止盈被算到入场价上方，应拒单。
	sp := shortPred() // 空头，入场=RefPrice=62000
	sp.TPBandHigh, sp.TPBandLow, sp.TPInvalidation = 63500, 62500, 63400
	if _, ok := Plan(sp, s, now); ok {
		t.Fatal("空头止盈价落在入场价上方(矛盾单)应拒单")
	}

	// 对照：交易周期区间正常套住入场价 → 应正常开仓。
	ok3 := p
	ok3.TPBandHigh, ok3.TPBandLow, ok3.TPInvalidation = 63000, 61000, 60800
	if o, ok := Plan(ok3, s, now); !ok || o.TakeProfit <= o.OpenPrice {
		t.Fatalf("区间正常套住入场价应开仓且止盈在上方, ok=%v tp=%v open=%v", ok, o.TakeProfit, o.OpenPrice)
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
