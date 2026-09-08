package signal

import (
	"math"
	"strings"
	"testing"
	"time"

	argusMonitor "argus_single/pkg/monitor"
)

func ts(min int) time.Time {
	return time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute)
}

func flatBars(n int, px float64) []Bar {
	out := make([]Bar, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, Bar{Ts: ts(i), Open: px, High: px, Low: px, Close: px})
	}
	return out
}

func baseParams() Params {
	p := DefaultParams()
	p.CapOverride = 15
	p.OrderSize = 1
	p.TakerFee = 0
	return p
}

// 上限：cap 张之后的同向触发一律记 cap_skip，且不改变持仓。
func TestCapSkipCountsAndDoesNotAdd(t *testing.T) {
	var sigs []Signal
	for i := 1; i <= 20; i++ {
		sigs = append(sigs, Signal{Ts: ts(i).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1})
	}
	res, err := Replay(Input{Params: baseParams(), Signals: sigs, Bars: flatBars(40, 60000)})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipCap != 5 {
		t.Errorf("cap 跳过 = %d, 期望 5（20 次触发 − 上限 15）", res.SkipCap)
	}
	if res.MaxStack != 15 {
		t.Errorf("最大堆积 = %d, 期望 15", res.MaxStack)
	}
	if res.SignalReplayed != 20 {
		t.Errorf("回放触发数 = %d, 期望 20（被拦截也算真实触发）", res.SignalReplayed)
	}
}

// 反向门控：净仓亏损时的反向触发被拦（gate_block），净仓不动。
func TestReverseGateBlocksWhenUnprofitable(t *testing.T) {
	p := baseParams()
	p.GateMinProfitPct = 20
	// 先在 60000 开 3 张多，价格跌到 59900（ROI 约 −208%），再来反向空信号。
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(3), Open: 59900, High: 59900, Low: 59900, Close: 59900},
		{Ts: ts(4), Open: 59900, High: 59900, Low: 59900, Close: 59900},
	}
	sigs := []Signal{
		{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(1).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(2).Add(time.Second), Side: "short", Event: EvGateBlock, OrderSize: 1},
	}
	p.CatastropheStopPct = 400 // 别让兜底先触发，本例只验门控
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipGate != 1 {
		t.Errorf("门控拦截 = %d, 期望 1", res.SkipGate)
	}
	if res.Reduces != 0 {
		t.Errorf("减仓次数 = %d, 期望 0", res.Reduces)
	}
}

// 反向门控：净仓盈利达阈值时放行减仓，已实现盈亏进账、净仓减少。
func TestReverseGateAllowsProfitableReduce(t *testing.T) {
	p := baseParams()
	p.GateMinProfitPct = 20
	p.SmallActivatePct = 1e9 // 关掉移动止盈，隔离门控行为
	p.MediumActivatePct = 1e9
	p.LargeActivatePct = 1e9
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 60100, High: 60100, Low: 60100, Close: 60100},
		{Ts: ts(3), Open: 60100, High: 60100, Low: 60100, Close: 60100},
	}
	sigs := []Signal{
		{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(1).Add(time.Second), Side: "short", Event: EvOpen, OrderSize: 1},
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.Reduces != 1 {
		t.Fatalf("减仓次数 = %d, 期望 1", res.Reduces)
	}
	// 1 张 60000→60100，面值 0.001 ⇒ 0.1U
	if math.Abs(res.Realized-0.1) > 1e-9 {
		t.Errorf("已实现 = %.6f, 期望 0.1", res.Realized)
	}
	// 削零 ⇒ 一个 reduce 归因的 episode
	if len(res.Episodes) != 1 || res.Episodes[0].Reason != ExitReduce {
		t.Fatalf("episode = %+v, 期望一条 reduce", res.Episodes)
	}
	if got := res.Episodes[0].ReducedPnl; math.Abs(got-0.1) > 1e-9 {
		t.Errorf("episode 减仓盈亏 = %.6f, 期望 0.1", got)
	}
}

// 趋势闸：24h 动量超阈时拦逆势开仓；覆盖不足一个窗口时放行（与实盘 warmup 一致）。
func TestTrendGateBlocksCounterTrendAndWarmupPasses(t *testing.T) {
	p := baseParams()
	p.TrendGateWindowHours = 2
	p.TrendGateThresholdPct = 5
	// 2 小时 = 120 根，一路匀速上涨。第 60 根（窗口未满 ⇒ 放行）与
	// 第 200 根（窗口已满、动量 ≈+9.4% ≥ 5% ⇒ 拦逆势开空）各来一个空信号。
	var bars []Bar
	for i := 0; i < 240; i++ {
		px := 60000.0 + float64(i)*50
		bars = append(bars, Bar{Ts: ts(i), Open: px, High: px, Low: px, Close: px})
	}
	sigs := []Signal{
		{Ts: ts(59).Add(time.Second), Side: "short", Event: EvOpen, OrderSize: 1},
		{Ts: ts(199).Add(time.Second), Side: "short", Event: EvOpen, OrderSize: 1},
	}
	p.CatastropheStopPct = 5000 // 隔离：别让上涨把空仓打到兜底
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipTrend != 1 {
		t.Errorf("趋势闸拦截 = %d, 期望 1（warmup 期那条应放行）", res.SkipTrend)
	}
}

// 趋势闸不拦减仓：与 manager.go 的分流一致（减仓走 reverse_gate）。
func TestTrendGateDoesNotBlockReduce(t *testing.T) {
	p := baseParams()
	p.TrendGateWindowHours = 1
	p.TrendGateThresholdPct = 1
	p.GateMinProfitPct = 0
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
	p.CatastropheStopPct = 5000
	var bars []Bar
	for i := 0; i < 180; i++ {
		px := 60000.0 + float64(i)*20 // 一路上涨，动量必然超 1%
		bars = append(bars, Bar{Ts: ts(i), Open: px, High: px, Low: px, Close: px})
	}
	sigs := []Signal{
		{Ts: ts(5).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},    // warmup 放行
		{Ts: ts(150).Add(time.Second), Side: "short", Event: EvOpen, OrderSize: 1}, // 反向减仓，趋势闸不应插手
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipTrend != 0 {
		t.Errorf("趋势闸拦截 = %d, 期望 0（减仓不过趋势闸）", res.SkipTrend)
	}
	if res.Reduces != 1 {
		t.Errorf("减仓次数 = %d, 期望 1", res.Reduces)
	}
}

// 兜底止损用一根内极值判定：只看收盘会漏掉插针（高估收益）。
func TestCatastropheUsesIntraBarExtreme(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 300 // 300% ROI @125x ⇒ 2.4% 价格逆行
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		// 插针到 58000（-3.33%，穿越 -300% 线），收盘回到 60000
		{Ts: ts(2), Open: 60000, High: 60000, Low: 58000, Close: 60000},
		{Ts: ts(3), Open: 60000, High: 60000, Low: 60000, Close: 60000},
	}
	sigs := []Signal{{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Episodes) != 1 || res.Episodes[0].Reason != ExitCatastrophe {
		t.Fatalf("episodes = %+v, 期望一条 catastrophe", res.Episodes)
	}
	// 无过冲、close 口径 ⇒ 按触发线成交，ROI 恰为 −300%
	if got := res.Episodes[0].RoiPct; math.Abs(got+300) > 1e-6 {
		t.Errorf("兜底 ROI = %.4f, 期望 -300", got)
	}
}

// 过冲：成交价比触发线更差 overshoot 个 ROI 点（研究脚本口径）。
func TestCatastropheOvershoot(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 300
	p.CatastropheOvershootRoiPts = 5
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 60000, High: 60000, Low: 58000, Close: 60000},
	}
	sigs := []Signal{{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Episodes[0].RoiPct; math.Abs(got+305) > 1e-6 {
		t.Errorf("兜底 ROI = %.4f, 期望 -305（含 5 点过冲）", got)
	}
}

// 移动止盈的状态机必须与实盘 monitor.EvaluateExit 完全一致：
// 单根内 O=H=L=C 时，引擎的决策序列应逐步等于直接调用 EvaluateExit。
func TestTrailStateMachineMatchesLiveEvaluateExit(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 1e6 // 关掉兜底，只看 trail
	cfg := argusMonitor.BuildExitConfig(p.CapOverride, argusMonitor.TrailParams{
		TierSmallRatio: p.TierSmallRatio, TierLargeRatio: p.TierLargeRatio,
		Small:              argusMonitor.Tier{ActivatePct: p.SmallActivatePct, GivebackFrac: p.SmallGiveback},
		Medium:             argusMonitor.Tier{ActivatePct: p.MediumActivatePct, GivebackFrac: p.MediumGiveback},
		Large:              argusMonitor.Tier{ActivatePct: p.LargeActivatePct, GivebackFrac: p.LargeGiveback},
		CatastropheStopPct: p.CatastropheStopPct,
	})

	// 1 张多仓 @60000（小仓：activate 150%，giveback 0.35 ⇒ 退出线 = peak×0.65）
	// 150% ROI @125x = 1.2% 价格变动：60800 激活、61000 抬峰、60600 回撤到
	// peak×0.65 以下触发。
	pxs := []float64{60000, 60800, 61000, 60700, 60600, 60000}
	bars := []Bar{{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000}}
	for i, px := range pxs {
		bars = append(bars, Bar{Ts: ts(2 + i), Open: px, High: px, Low: px, Close: px})
	}
	sigs := []Signal{{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}

	// 独立跑一遍纯实盘函数，找它在第几根给出 TrailingClose
	st := argusMonitor.TrailState{}
	wantIdx, wantRoi := -1, 0.0
	for i, px := range pxs {
		roi := (px - 60000) / 60000 * 125 * 100
		act, next := argusMonitor.EvaluateExit(1, roi, cfg, st)
		st = next
		if act == argusMonitor.ActionTrailingClose {
			wantIdx, wantRoi = i, roi
			break
		}
	}
	if wantIdx < 0 {
		t.Fatal("测试构造有误：实盘函数在该价格序列上没有触发移动止盈")
	}
	if len(res.Episodes) != 1 || res.Episodes[0].Reason != ExitTrailing {
		t.Fatalf("episodes = %+v, 期望一条 trailing", res.Episodes)
	}
	if got := res.Episodes[0].ClosedAt; !got.Equal(ts(2 + wantIdx)) {
		t.Errorf("触发时刻 = %s, 实盘函数口径应为 %s", got, ts(2+wantIdx))
	}
	if got := res.Episodes[0].RoiPct; math.Abs(got-wantRoi) > 1e-6 {
		t.Errorf("平仓 ROI = %.4f, 实盘函数口径 %.4f", got, wantRoi)
	}
}

// 种子仓：窗口起点的旧仓必须参与判定，且它之前的触发要被丢弃而不是重放。
func TestSeedPositionAndDroppedSignals(t *testing.T) {
	p := baseParams()
	seed := SeedPosition{At: ts(5), Side: "long", Size: 10, AvgPx: 60000}
	sigs := []Signal{
		{Ts: ts(1), Side: "long", Event: EvOpen, OrderSize: 1}, // 种子之前 → 丢弃
		{Ts: ts(5), Side: "long", Event: EvOpen, OrderSize: 1}, // 与种子同刻 → 丢弃
		{Ts: ts(6), Side: "long", Event: EvOpen, OrderSize: 1}, // 回放
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: flatBars(20, 60000), Seed: seed})
	if err != nil {
		t.Fatal(err)
	}
	if res.SignalDropped != 2 || res.SignalReplayed != 1 {
		t.Errorf("丢弃/回放 = %d/%d, 期望 2/1", res.SignalDropped, res.SignalReplayed)
	}
	if res.MaxStack != 11 {
		t.Errorf("最大堆积 = %d, 期望 11（种子 10 + 1）", res.MaxStack)
	}
}

// 仓位上限走实盘公式：CapOverride=0 时用 trade.ComputeMaxContracts，
// 且只在第一个可用价格上算一次（语义同 PositionCapGuard.EnsureInit）。
func TestCapFromLiveFormulaFrozenOnce(t *testing.T) {
	p := DefaultParams()
	p.OrderSize = 1
	p.TakerFee = 0
	p.RiskEquity = 375.73
	p.BudgetPct = 20
	p.CatastropheStopPct = 300
	p.Ceiling = 999
	// N = 0.20×375.73×125×100 / (0.001×60000×300) = 52.18 → 52
	bars := []Bar{{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000}}
	for i := 2; i < 10; i++ {
		bars = append(bars, Bar{Ts: ts(i), Open: 30000, High: 30000, Low: 30000, Close: 30000})
	}
	res, err := Replay(Input{Params: p, Signals: nil, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.Cap != 52 {
		t.Errorf("上限 = %d, 期望 52（价格腰斩后也不重算）", res.Cap)
	}
}

// 参数校验复用实盘的 trade.ValidateRiskParams：紧止损这类被实证否定的参数
// 在回测里也必须被拒绝，不允许"回测能跑、实盘起不来"。
func TestValidateRejectsTightStopLikeLive(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 150
	err := p.Validate()
	if err == nil {
		t.Fatal("S=150 应被拒绝（紧止损杀死均值回归 edge）")
	}
	if !strings.Contains(err.Error(), "catastrophe_stop_pct") {
		t.Errorf("错误信息 = %v, 应指出 catastrophe_stop_pct", err)
	}
	if _, err := Replay(Input{Params: p, Bars: flatBars(3, 60000)}); err == nil {
		t.Fatal("Replay 也应拒绝非法参数")
	}
}

// 无 K 线不产生结果：持仓路径无法回放时必须报错，不能悄悄给 0 笔。
func TestReplayFailsWithoutBars(t *testing.T) {
	if _, err := Replay(Input{Params: baseParams(), Signals: []Signal{{Ts: ts(1), Side: "long"}}}); err == nil {
		t.Fatal("没有 K 线时应报错")
	}
}

// 种子仓推导：快照出现在第一条 open 之后 ⇒ 那是回放自己建的仓，不得再灌种子。
func TestSeedFromSignalsIgnoresSnapshotAfterFirstOpen(t *testing.T) {
	sigs := []Signal{
		{Ts: ts(1), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(2), Side: "short", Event: EvGateBlock, NetSide: "long", NetSize: 3, AvgPx: 60000},
	}
	if seed := SeedFromSignals(sigs); seed.OK() {
		t.Errorf("不该推出种子仓: %+v", seed)
	}
}

// 窗口起点已有旧仓（第一条就是带快照的拦截事件）⇒ 灌种子。
func TestSeedFromSignalsAcceptsSnapshotBeforeAnyOpen(t *testing.T) {
	sigs := []Signal{
		{Ts: ts(1), Side: "short", Event: EvGateBlock, NetSide: "long", NetSize: 14, AvgPx: 60258.1},
		{Ts: ts(2), Side: "long", Event: EvOpen, OrderSize: 1},
	}
	seed := SeedFromSignals(sigs)
	if !seed.OK() || seed.Side != "long" || seed.Size != 14 {
		t.Fatalf("种子仓 = %+v", seed)
	}
}
