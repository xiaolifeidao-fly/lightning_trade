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

// 加仓闸：净仓 ROI 低于阈值时同向加仓被拦（计 SkipAddRoi），高于阈值照常加；
// 全新开仓不受影响；0=关闭时行为与从前一致。
func TestAddGateBlocksAddsBelowRoiThreshold(t *testing.T) {
	p := baseParams()
	p.AddMinRoiPct = -100
	p.CatastropheStopPct = 400
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
	// 60000 开 1 张多；59700（ROI −62.5%）加仓应放行；59000（ROI −208%）加仓应被拦。
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 59700, High: 59700, Low: 59700, Close: 59700},
		{Ts: ts(3), Open: 59000, High: 59000, Low: 59000, Close: 59000},
		{Ts: ts(4), Open: 59000, High: 59000, Low: 59000, Close: 59000},
	}
	sigs := []Signal{
		{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(1).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}, // 按 ts(2) 成交，ROI 对 60000 是 −62.5%
		{Ts: ts(2).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}, // 按 ts(3) 成交，均价 59850 → ROI −177%
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipAddRoi != 1 {
		t.Errorf("加仓闸拦截 = %d, 期望 1", res.SkipAddRoi)
	}
	if res.MaxStack != 2 {
		t.Errorf("最大堆积 = %d, 期望 2（第三张被拦）", res.MaxStack)
	}
	p.AddMinRoiPct = 0
	off, _ := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if off.SkipAddRoi != 0 || off.MaxStack != 3 {
		t.Errorf("关闭时应不拦：skip=%d stack=%d", off.SkipAddRoi, off.MaxStack)
	}
}

func TestValidateAddGateRange(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.AddMinRoiPct = 50
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "addMinRoiPct") {
		t.Errorf("正值应被拒, got %v", err)
	}
	p.AddMinRoiPct = -401
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "addMinRoiPct") {
		t.Errorf("低于兜底线应被拒, got %v", err)
	}
	p.AddMinRoiPct = -400
	if err := p.Validate(); err != nil {
		t.Errorf("边界值 −兜底线 应放行, got %v", err)
	}
}

// 本金回撤兜底：按 USDT 亏损触发、与均价/ROI 无关；启用时 ROI 兜底不再生效。
// 2 张多 @60000，riskEquity 100、10% ⇒ 亏 10U 触发；每张面值 0.001 ⇒ 2 张每跌 1 元亏 0.002U
// ⇒ 触发线 60000 − 10/0.002 = 55000。57000（ROI −625%，ROI 兜底 400 早该触发）不触发；54900 触发。
func TestEquityStopFiresOnUsdtLossNotRoi(t *testing.T) {
	p := baseParams()
	p.RiskEquity = 100
	p.EquityStopPct = 10
	p.CatastropheStopPct = 400
	p.CatastropheOvershootRoiPts = 0
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 57000, High: 57000, Low: 57000, Close: 57000},
		{Ts: ts(3), Open: 54900, High: 54900, Low: 54900, Close: 54900},
		{Ts: ts(4), Open: 54900, High: 54900, Low: 54900, Close: 54900},
	}
	sigs := []Signal{
		{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
		{Ts: ts(0).Add(2 * time.Second), Side: "long", Event: EvOpen, OrderSize: 1},
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Episodes) != 1 || res.Episodes[0].Reason != ExitCatastrophe {
		t.Fatalf("应有一笔兜底: %+v", res.Episodes)
	}
	ep := res.Episodes[0]
	if !ep.ClosedAt.Equal(ts(3)) {
		t.Errorf("应在 ts(3)（54900 穿 55000）触发而非 ts(2)（ROI 口径会在 57000 触发）, got %v", ep.ClosedAt)
	}
	if math.Abs(ep.ClosePx-55000) > 1e-6 {
		t.Errorf("成交价应为触发线 55000（无过冲）, got %.2f", ep.ClosePx)
	}
	if math.Abs(ep.Pnl-(-10)) > 1e-6 {
		t.Errorf("亏损应恰为 10U, got %.4f", ep.Pnl)
	}
}

// 本金回撤兜底 13.3% 在"满仓 = 公式上限"时与 ROI −400% 同点：这是两种口径的校准锚。
// cap 公式：N = f×E×lev×100/(face×px×S)。取 E=100、f=13.3%、px=60000、S=400 ⇒ N=6.93→6。
// 6 张时 USDT 线 = 13.3/(0.001×6)=2216.7 元；ROI 线 = 60000×400/12500=1920 元——上限取整后 USDT 口径略松，方向正确。
func TestEquityStopIsLooserThanRoiBelowFullStack(t *testing.T) {
	p := baseParams()
	p.RiskEquity = 100
	p.CatastropheStopPct = 400
	p.CatastropheOvershootRoiPts = 0
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
	bars := []Bar{
		{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000},
		{Ts: ts(2), Open: 58000, High: 58000, Low: 58000, Close: 58000}, // −2000：ROI 兜底 −417% 触发；USDT 亏 2U < 13.3U 不触发
		{Ts: ts(3), Open: 58000, High: 58000, Low: 58000, Close: 58000},
	}
	sigs := []Signal{{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	roi, _ := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	p.EquityStopPct = 13.3
	eq, _ := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if len(roi.Episodes) != 1 || roi.Episodes[0].Reason != ExitCatastrophe {
		t.Fatalf("ROI 口径 1 张跌 2000 应兜底: %+v", roi.Episodes)
	}
	if len(eq.Episodes) != 1 || !eq.Episodes[0].Open {
		t.Fatalf("USDT 口径 1 张只亏 2U，不该兜底，应 eod 未平: %+v", eq.Episodes)
	}
}

func TestValidateEquityStopRange(t *testing.T) {
	p := baseParams()
	p.RiskEquity = 100
	p.EquityStopPct = 150
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "equityStopPct") {
		t.Errorf(">100 应被拒, got %v", err)
	}
	p.EquityStopPct = 10
	p.RiskEquity = 0
	if err := p.Validate(); err == nil || !strings.Contains(err.Error(), "riskEquity") {
		t.Errorf("无 riskEquity 应被拒, got %v", err)
	}
}

// 开仓波动闸：高波动时拦全新开仓、不拦加仓；样本不足 30 根放行；关闭时不拦。
func TestOpenVolGateBlocksFreshOpenOnly(t *testing.T) {
	p := baseParams()
	p.OpenMaxVolBpm = 5
	// 前 40 根每根来回 0.1%（10 bp/min），远超阈值 5
	bars := []Bar{}
	px := 60000.0
	for i := 0; i < 44; i++ {
		if i%2 == 0 {
			px = 60000
		} else {
			px = 60060
		}
		bars = append(bars, Bar{Ts: ts(i), Open: px, High: px, Low: px, Close: px})
	}
	sigs := []Signal{
		{Ts: ts(10).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}, // 样本只有 10 根 → warmup 放行
		{Ts: ts(41).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}, // 加仓：不受闸约束
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipVolGate != 0 || res.MaxStack != 2 {
		t.Fatalf("warmup 放行 + 加仓不拦：skip=%d stack=%d", res.SkipVolGate, res.MaxStack)
	}
	// 全新开仓落在样本充足的高波动段 → 拦
	p2 := p
	sigs2 := []Signal{{Ts: ts(41).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res2, _ := Replay(Input{Params: p2, Signals: sigs2, Bars: bars})
	if res2.SkipVolGate != 1 || res2.MaxStack != 0 {
		t.Fatalf("高波动全新开仓应被拦：skip=%d stack=%d", res2.SkipVolGate, res2.MaxStack)
	}
	p2.OpenMaxVolBpm = 0
	res3, _ := Replay(Input{Params: p2, Signals: sigs2, Bars: bars})
	if res3.SkipVolGate != 0 || res3.MaxStack != 1 {
		t.Fatalf("关闭时不拦：skip=%d stack=%d", res3.SkipVolGate, res3.MaxStack)
	}
}

// 开仓密度闸：前 60 分钟信号 ≥ k 拦全新开仓（含被其它闸拦下的信号），加仓不拦。
func TestOpenDensityGateCountsPriorSignals(t *testing.T) {
	p := baseParams()
	p.OpenMaxSignals60 = 3
	bars := flatBars(80, 60000)
	// 4 条空信号（均因 cap=15 放行开仓/加仓…改为让它们本身是有效开仓会改变净仓；用相反方向被反向闸拦下的来堆密度）
	sigs := []Signal{
		{Ts: ts(1).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},       // 全新开仓：前 60m 0 条 → 放行
		{Ts: ts(2).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},       // 加仓：不受闸约束
		{Ts: ts(3).Add(time.Second), Side: "short", Event: EvGateBlock, OrderSize: 1}, // 反向被反向闸拦，但计入密度
		{Ts: ts(4).Add(time.Second), Side: "short", Event: EvGateBlock, OrderSize: 1},
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipDens != 0 || res.MaxStack != 2 {
		t.Fatalf("加仓不受密度闸约束：skipDens=%d stack=%d", res.SkipDens, res.MaxStack)
	}
	// 空仓判定直接打在引擎上：前 60 分钟 4 条 ≥ 3 → 拦；61 分钟前的被裁掉后 2 条 <3 → 放行。
	e := NewEngine(p.Normalize())
	for k := 0; k < 4; k++ {
		e.noteSignal(ts(k))
	}
	e.noteSignal(ts(5))
	if e.passOpenGate(ts(5)) || e.skipDens != 1 {
		t.Fatalf("前 60 分钟 4 条应拦：skipDens=%d", e.skipDens)
	}
	e.noteSignal(ts(70)) // 距 ts(0..5) 均 >60 分钟，全部裁掉，只剩本条
	e.noteSignal(ts(71))
	if !e.passOpenGate(ts(71)) {
		t.Fatalf("裁掉旧信号后只剩 1 条先前信号，应放行")
	}
}

// 兜底冷静期：兜底后 N 分钟内全新开仓被拦（计 SkipCooldown），过期后放行。
func TestCatastropheCooldownBlocksFreshOpen(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.CatastropheCooldownMin = 10
	bars := []Bar{{Ts: ts(1), Open: 60000, High: 60000, Low: 60000, Close: 60000}}
	for i := 2; i < 30; i++ {
		bars = append(bars, Bar{Ts: ts(i), Open: 57000, High: 57000, Low: 57000, Close: 57000}) // 穿越兜底
	}
	sigs := []Signal{
		{Ts: ts(0).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1},   // 开多，ts(2) 兜底
		{Ts: ts(5).Add(time.Second), Side: "short", Event: EvOpen, OrderSize: 1},  // 兜底后 3 分钟 → 拦
		{Ts: ts(20).Add(time.Second), Side: "short", Event: EvOpen, OrderSize: 1}, // 兜底后 18 分钟 → 放行
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipCooldown != 1 {
		t.Errorf("冷静期内应拦 1 次, got %d", res.SkipCooldown)
	}
	if len(res.Episodes) != 2 || res.Episodes[0].Reason != ExitCatastrophe || !res.Episodes[1].Open {
		t.Errorf("应为 1 笔兜底 + 1 笔冷静期后的新开仓(eod): %+v", res.Episodes)
	}
}

// 日亏熔断：当日 MTM 回撤 ≥ X%×E 后到次日 00:00 不开新仓；跨日恢复；加仓不受影响。
func TestDailyLossHaltBlocksFreshOpenUntilNextDay(t *testing.T) {
	p := baseParams()
	p.RiskEquity = 100
	p.DailyLossHaltPct = 9 // 亏 9U 熔断（兜底在触发线 58080 成交：5 张各亏 1.92U = 9.6U）
	p.CatastropheStopPct = 400
	p.CatastropheOvershootRoiPts = 0
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
	// 5 张多 @60000，跌到 57900：ROI −437% 触发兜底、按触发线 58080 成交，合计亏 9.6U ≥ 9U → 同一根熔断。
	day0 := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	mk := func(min int, px float64) Bar {
		t0 := day0.Add(time.Duration(min) * time.Minute)
		return Bar{Ts: t0, Open: px, High: px, Low: px, Close: px}
	}
	bars := []Bar{mk(1, 60000), mk(2, 60000), mk(3, 57900), mk(4, 57900), mk(5, 57900)}
	for m := 6; m <= 1439; m += 60 {
		bars = append(bars, mk(m, 57900)) // 当日其余时间
	}
	bars = append(bars, mk(1440, 57900), mk(1441, 57900)) // 次日 00:00、00:01
	sigs := []Signal{}
	for k := 0; k < 5; k++ {
		sigs = append(sigs, Signal{Ts: day0.Add(time.Duration(k+1) * time.Second), Side: "long", Event: EvOpen, OrderSize: 1})
	}
	sigs = append(sigs,
		Signal{Ts: day0.Add(10*time.Minute + time.Second), Side: "short", Event: EvOpen, OrderSize: 1},   // 当日：熔断中 → 拦
		Signal{Ts: day0.Add(12*time.Hour + time.Second), Side: "short", Event: EvOpen, OrderSize: 1},     // 当日：仍拦
		Signal{Ts: day0.Add(1440*time.Minute + time.Second), Side: "short", Event: EvOpen, OrderSize: 1}, // 次日 00:00:01 → 放行
	)
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if res.SkipHalt != 2 {
		t.Errorf("当日两次开仓应被熔断拦下, got %d（episodes=%+v）", res.SkipHalt, res.Episodes)
	}
	if len(res.Episodes) != 2 || !res.Episodes[1].Open || res.Episodes[1].Side != "short" {
		t.Errorf("次日应放行开空(eod): %+v", res.Episodes)
	}
}
