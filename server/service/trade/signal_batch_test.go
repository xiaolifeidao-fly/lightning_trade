package trade

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	argusDTO "service/argus_config/dto"
	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"
)

// ─── 基线：实例当前生产参数 ────────────────────────────────────────────────────

// 已发布快照 → 基线参数：有 DB 列的逐项取到，signal_threshold 按 bp 换算。
// 数据照实例2 champion（账户A cap26 / S400 / budget 13.3 / 3bp）填。
func TestBaselineFromSnapshotMapsPublishedConfig(t *testing.T) {
	snapshot := &argusDTO.ConfigSnapshotDTO{
		InstanceKey: "argus-2",
		Version:     argusDTO.ConfigVersionDTO{Version: 7},
		Config:      argusDTO.ConfigDTO{DefaultOrderSize: 1},
		Accounts: []argusDTO.AccountDTO{
			{ID: 1, AccountName: "账户A", InitialBalance: 375.73, Enabled: 1},
			{ID: 2, AccountName: "账户B", InitialBalance: 120, Enabled: 1},
		},
		AccountRisks: []argusDTO.AccountRiskDTO{
			{AccountID: 1, RiskBudget: 13.3, CatastrophicStopLoss: 400, MaxContracts: 26,
				ReverseGateEnabled: 1, TrailingStopTiersJSON: `{"small_activate":150,"small_giveback":0.35,` +
					`"medium_activate":90,"medium_giveback":0.28,"large_activate":40,"large_giveback":0.2,` +
					`"tier_small_ratio":0.3,"tier_large_ratio":0.65}`},
			{AccountID: 2, RiskBudget: 13.3, CatastrophicStopLoss: 400, MaxContracts: 8, ReverseGateEnabled: 1},
		},
		MonitorSymbols: []argusDTO.MonitorSymbolDTO{
			{Symbol: "BTCUSDT", DeepInstrument: "BTC-USDT-SWAP", TradeInstrument: "BTCUSDT",
				SpreadThreshold: 0.001, SignalThreshold: 0.0003, Enabled: 1},
		},
	}
	b, err := baselineFromSnapshot(snapshot, "账户A", "BTCUSDT")
	if err != nil {
		t.Fatalf("基线解析失败: %v", err)
	}
	if b.Source != BaselineSourceInstancePublished {
		t.Errorf("source = %q", b.Source)
	}
	if b.Params.Ceiling != 26 || math.Abs(b.Params.BudgetPct-13.3) > 1e-9 ||
		math.Abs(b.Params.CatastropheStopPct-400) > 1e-9 {
		t.Errorf("仓位/风险参数映射有误: %+v", b.Params)
	}
	// 0.0003 → 3bp。基线自身不改阈值，θ 与 θ0 必须相等，否则精度会被误判成频率级。
	if math.Abs(b.Params.BaselineThresholdBp-3) > 1e-9 || math.Abs(b.Params.SignalThresholdBp-3) > 1e-9 {
		t.Errorf("signal_threshold 换算有误: θ0=%v θ=%v", b.Params.BaselineThresholdBp, b.Params.SignalThresholdBp)
	}
	if signal.Classify(b.Params).Level != signal.FidelityEvent {
		t.Errorf("基线必须是事件级精度，得到 %s", signal.Classify(b.Params).Level)
	}
	if math.Abs(b.Params.RiskEquity-375.73) > 1e-9 {
		t.Errorf("risk_equity 应回落 initial_balance，得到 %v", b.Params.RiskEquity)
	}
	if math.Abs(b.Params.MediumActivatePct-90) > 1e-9 || math.Abs(b.Params.LargeGiveback-0.2) > 1e-9 {
		t.Errorf("trail 分档映射有误: %+v", b.Params)
	}
	if err := b.Params.Validate(); err != nil {
		t.Errorf("从生产配置映射出来的基线应当通过实盘校验: %v", err)
	}
}

// 配置面收敛（r5）未完成的字段必须在 Notes 里说清是兜底，不能被当成生产事实。
// 这条护栏针对的是"基线趋势闸=关闭"这种看起来像事实、实际是缺列的读数。
func TestBaselineNotesDeclareMissingColumns(t *testing.T) {
	snapshot := &argusDTO.ConfigSnapshotDTO{
		InstanceKey:  "argus-2",
		Version:      argusDTO.ConfigVersionDTO{Version: 3},
		Config:       argusDTO.ConfigDTO{DefaultOrderSize: 1},
		Accounts:     []argusDTO.AccountDTO{{ID: 1, AccountName: "账户A", InitialBalance: 375.73}},
		AccountRisks: []argusDTO.AccountRiskDTO{{AccountID: 1, RiskBudget: 13.3, CatastrophicStopLoss: 400, MaxContracts: 26}},
	}
	b, err := baselineFromSnapshot(snapshot, "账户A", "BTCUSDT")
	if err != nil {
		t.Fatalf("基线解析失败: %v", err)
	}
	note := b.Note()
	for _, want := range []string{"risk_equity", "trend_gate", "reverse_gate_min_profit_pct", "contract_face", "order_size"} {
		if !strings.Contains(note, want) {
			t.Errorf("基线说明缺少 %s 的兜底交代: %s", want, note)
		}
	}
	// 趋势闸缺列 ⇒ 基线按关闭处理，但必须说出来（上面已断言）。
	if b.Params.TrendGateThresholdPct != 0 {
		t.Errorf("趋势闸无 DB 列时基线应按关闭处理，得到 %v", b.Params.TrendGateThresholdPct)
	}
	if !strings.Contains(note, "v3") {
		t.Errorf("基线说明应标明取自哪个版本: %s", note)
	}
}

// accountLabel 命中不了就报错并列出候选，绝不退到第一个账户：
// 实例2 有 champion(cap26) 与 challenger(cap8) 两本仓，猜错等于整批基线错一倍仓位。
func TestBaselineRefusesUnknownAccount(t *testing.T) {
	snapshot := &argusDTO.ConfigSnapshotDTO{
		Accounts: []argusDTO.AccountDTO{
			{ID: 1, AccountName: "账户A"}, {ID: 2, AccountName: "账户B"},
		},
		AccountRisks: []argusDTO.AccountRiskDTO{{AccountID: 1}, {AccountID: 2}},
	}
	_, err := baselineFromSnapshot(snapshot, "账户C", "BTCUSDT")
	if err == nil {
		t.Fatal("未命中的账户必须报错，不能退到第一个账户")
	}
	if !strings.Contains(err.Error(), "账户A") || !strings.Contains(err.Error(), "账户B") {
		t.Errorf("错误里应列出可选账户: %v", err)
	}
}

// ─── 参数 diff ───────────────────────────────────────────────────────────────

// 组参数叠加在基线上：没给的旋钮沿用基线而不是回落代码缺省，
// 于是"只改 cap"的组 diff 里只有 cap 一行。
func TestGroupParamsOverlayBaselineNotDefaults(t *testing.T) {
	baseline := signal.DefaultParams()
	baseline.RiskEquity = 375.73
	baseline.Ceiling = 26
	baseline.BudgetPct = 13.3
	baseline.CatastropheStopPct = 400
	baseline.GateMinProfitPct = 8
	baseline = baseline.Normalize()

	group := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(40)})
	if group.Ceiling != 40 {
		t.Fatalf("组参数未生效: ceiling=%d", group.Ceiling)
	}
	// 未给的旋钮必须还是基线值，而不是 DefaultParams 的 300/20/20。
	if math.Abs(group.CatastropheStopPct-400) > 1e-9 || math.Abs(group.BudgetPct-13.3) > 1e-9 ||
		math.Abs(group.GateMinProfitPct-8) > 1e-9 || math.Abs(group.RiskEquity-375.73) > 1e-9 {
		t.Fatalf("未给的旋钮应沿用基线: %+v", group)
	}
	diff := DiffSignalParams(baseline, group)
	if len(diff) != 1 || diff[0].Field != "ceiling" {
		t.Fatalf("diff 应只有 ceiling 一行, got %+v", diff)
	}
	if diff[0].Baseline != "26" || diff[0].Value != "40" || diff[0].Delta != 14 {
		t.Errorf("diff 取值有误: %+v", diff[0])
	}
}

// 显式传了与基线相同的值不算差异（否则表单一把全量回填就会显示"改了 20 项"）。
func TestDiffSignalParamsIgnoresSameValues(t *testing.T) {
	baseline := signal.DefaultParams().Normalize()
	same := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{
		Ceiling:            intp(baseline.Ceiling),
		CatastropheStopPct: fltp(baseline.CatastropheStopPct),
		Mode:               baseline.Mode,
	})
	if diff := DiffSignalParams(baseline, same); len(diff) != 0 {
		t.Fatalf("值相同不应产生 diff: %+v", diff)
	}
}

func TestAutoGroupLabel(t *testing.T) {
	baseline := signal.DefaultParams().Normalize()
	baseline.RiskEquity = 375.73
	if got := AutoGroupLabel(DiffSignalParams(baseline, baseline)); got != "same_as_baseline" {
		t.Errorf("全同基线的组标签 = %q", got)
	}
	one := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(40)})
	if got := AutoGroupLabel(DiffSignalParams(baseline, one)); got != "ceiling=40" {
		t.Errorf("单差异组标签 = %q", got)
	}
	many := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{
		Ceiling: intp(40), CatastropheStopPct: fltp(350), BudgetPct: fltp(15), GateMinProfitPct: fltp(8),
	})
	got := AutoGroupLabel(DiffSignalParams(baseline, many))
	if !strings.Contains(got, "+2项") {
		t.Errorf("多差异组标签应折叠尾部: %q", got)
	}
}

func TestNormalizeBatchConcurrency(t *testing.T) {
	for _, c := range []struct{ in, want int }{
		{0, DefaultSignalBatchConcurrency}, {-1, DefaultSignalBatchConcurrency},
		{1, 1}, {4, 4}, {99, MaxSignalBatchConcurrency},
	} {
		if got := normalizeBatchConcurrency(c.in); got != c.want {
			t.Errorf("normalizeBatchConcurrency(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ─── 横向对比：按 fidelity 分组，不得混排 ────────────────────────────────────

func batchRun(id int64, label string, params signal.Params, isBaseline bool) *tradeRepository.TradeBacktestRun {
	raw, _ := json.Marshal(params.Normalize())
	fid := signal.Classify(params)
	run := &tradeRepository.TradeBacktestRun{
		GroupLabel: label, Status: "done", EngineKind: EngineKindSignal,
		ParamsSnapshot: string(raw), Fidelity: fid.Level, FidelityNote: fid.Note(),
	}
	run.Id = int(id)
	if isBaseline {
		run.IsBaseline = 1
	}
	return run
}

func batchMetric(runID int64, fidelity string, netPnl float64, signalCount int) *tradeRepository.TradeBacktestMetric {
	return &tradeRepository.TradeBacktestMetric{
		RunID: runID, CalcMode: CalcModeSignal, Fidelity: fidelity,
		NetPnl: netPnl, SignalCount: signalCount, TradeCount: 12, WinCount: 8, WinRate: 8.0 / 12,
		ProfitFactor: 1.8, MaxDrawdown: 40, Sharpe: 0.3, TrailCount: 7, SlCount: 2,
	}
}

// 核心护栏：事件级与频率级分在不同的 group 里，接口从不给出一张跨精度的全局榜。
func TestBuildComparisonSplitsFidelityLevels(t *testing.T) {
	baseline := signal.DefaultParams()
	baseline.RiskEquity = 375.73
	baseline.BaselineThresholdBp, baseline.SignalThresholdBp = 3, 3
	baseline = baseline.Normalize()

	// 频率级组：把阈值从 3bp 抬到 8bp。
	freq := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{SignalThresholdBp: fltp(8)})
	// 事件级组：只改 cap。
	evt := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(40)})

	runs := []*tradeRepository.TradeBacktestRun{
		batchRun(1, "baseline", baseline, true),
		batchRun(2, "ceiling=40", evt, false),
		batchRun(3, "theta=8bp", freq, false),
	}
	metrics := map[int64]*tradeRepository.TradeBacktestMetric{
		1: batchMetric(1, signal.FidelityEvent, 100, 613),
		2: batchMetric(2, signal.FidelityEvent, 168, 613),
		// 频率级组的净利刻意设成全场最高：如果实现把它排进同一张榜，
		// 它会冒到第一名——这正是需求大纲 §3.3 禁止的那种误读。
		3: batchMetric(3, signal.FidelityFrequency, 900, 210),
	}
	groups, warnings := buildComparison(baseline, runs, metrics, 1)
	if len(groups) != 2 {
		t.Fatalf("应分成事件级/频率级两组, got %d", len(groups))
	}
	if groups[0].Fidelity != signal.FidelityEvent || groups[1].Fidelity != signal.FidelityFrequency {
		t.Fatalf("事件级必须排在前面: %s / %s", groups[0].Fidelity, groups[1].Fidelity)
	}
	if !groups[0].ComparableToBaseline || groups[1].ComparableToBaseline {
		t.Errorf("可比标记有误: event=%v frequency=%v", groups[0].ComparableToBaseline, groups[1].ComparableToBaseline)
	}
	// 频率级组的 900 不能出现在事件级组里。
	for _, row := range groups[0].Rows {
		if row.Metric != nil && row.Metric.NetPnl == 900 {
			t.Fatal("频率级结果被排进了事件级榜：跨精度混排")
		}
	}
	// 跨精度组不给 metricDiff，只给拒绝理由。
	for _, row := range groups[1].Rows {
		if row.MetricDiff != nil {
			t.Error("跨精度组不应有与基线的指标差异")
		}
		if !strings.Contains(row.DiffBlockedReason, "跨精度") {
			t.Errorf("拒绝理由应说明跨精度: %q", row.DiffBlockedReason)
		}
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, " "), "不得比大小") {
		t.Errorf("多精度批次必须给出混排警示: %v", warnings)
	}
}

// 基线行置顶、其余按净利降序；同精度组内才有 metricDiff。
func TestBuildComparisonSortsWithinGroupAndDiffsAgainstBaseline(t *testing.T) {
	baseline := signal.DefaultParams()
	baseline.RiskEquity = 375.73
	baseline = baseline.Normalize()
	lo := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(15)})
	hi := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(40)})

	runs := []*tradeRepository.TradeBacktestRun{
		batchRun(1, "baseline", baseline, true),
		batchRun(2, "ceiling=15", lo, false),
		batchRun(3, "ceiling=40", hi, false),
	}
	metrics := map[int64]*tradeRepository.TradeBacktestMetric{
		1: batchMetric(1, signal.FidelityEvent, 100, 613),
		2: batchMetric(2, signal.FidelityEvent, 60, 613),
		3: batchMetric(3, signal.FidelityEvent, 168, 613),
	}
	groups, _ := buildComparison(baseline, runs, metrics, 1)
	if len(groups) != 1 {
		t.Fatalf("全事件级应只有一组, got %d", len(groups))
	}
	g := groups[0]
	if g.SortedBy != "netPnl" {
		t.Errorf("排序依据 = %q", g.SortedBy)
	}
	if len(g.Rows) != 3 || !g.Rows[0].IsBaseline {
		t.Fatalf("基线行必须置顶: %+v", g.Rows)
	}
	if g.Rows[1].RunID != 3 || g.Rows[2].RunID != 2 {
		t.Errorf("组内应按净利降序: %d, %d", g.Rows[1].RunID, g.Rows[2].RunID)
	}
	if g.Rows[0].MetricDiff != nil || g.Rows[0].DiffBlockedReason != "本行即基线" {
		t.Errorf("基线行不该有自比差异: %+v", g.Rows[0])
	}
	best := g.Rows[1]
	if best.MetricDiff == nil {
		t.Fatalf("同精度组应有指标差异: %+v", best)
	}
	if math.Abs(best.MetricDiff.NetPnl-68) > 1e-9 {
		t.Errorf("净利差 = %v, want 68", best.MetricDiff.NetPnl)
	}
	if math.Abs(best.MetricDiff.NetPnlPct-68) > 1e-9 {
		t.Errorf("净利变化%% = %v, want 68", best.MetricDiff.NetPnlPct)
	}
	if len(best.ParamDiff) != 1 || best.ParamDiff[0].Field != "ceiling" {
		t.Errorf("参数差异应只有 ceiling: %+v", best.ParamDiff)
	}
}

// 降阈值的频率级组只产 λ(θ)、不产 PnL：不能按净利排序，也不能给出指标差异。
func TestBuildComparisonHandlesLambdaOnlyGroups(t *testing.T) {
	baseline := signal.DefaultParams()
	baseline.RiskEquity = 375.73
	baseline.BaselineThresholdBp, baseline.SignalThresholdBp = 5, 5
	baseline = baseline.Normalize()
	down := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{SignalThresholdBp: fltp(3)})

	runs := []*tradeRepository.TradeBacktestRun{
		batchRun(1, "baseline", baseline, true),
		batchRun(2, "theta=3bp", down, false),
	}
	// 降阈值组：signalCount=0 / tradeCount=0，只有 λ。
	lambdaOnly := &tradeRepository.TradeBacktestMetric{
		RunID: 2, CalcMode: CalcModeSignal, Fidelity: signal.FidelityFrequency, LambdaPerDay: 41.2,
	}
	metrics := map[int64]*tradeRepository.TradeBacktestMetric{
		1: batchMetric(1, signal.FidelityEvent, 100, 613),
		2: lambdaOnly,
	}
	groups, _ := buildComparison(baseline, runs, metrics, 1)
	var freq *tradeDTO.SignalBacktestFidelityGroupDTO
	for i := range groups {
		if groups[i].Fidelity == signal.FidelityFrequency {
			freq = &groups[i]
		}
	}
	if freq == nil {
		t.Fatal("缺少频率级组")
	}
	if freq.SortedBy != "lambdaPerDay" {
		t.Errorf("整组无 PnL 时应按 λ 排序, got %q", freq.SortedBy)
	}
	row := freq.Rows[0]
	if row.PnlAvailable {
		t.Error("降阈值组不产 PnL，pnlAvailable 应为 false")
	}
	if row.MetricDiff != nil {
		t.Error("不产 PnL 的组不应有指标差异")
	}
}

// 基线组还没跑完时只出参数 diff，并明确警示，不能把空指标当成 0 去比。
func TestBuildComparisonWarnsWhenBaselineNotReady(t *testing.T) {
	baseline := signal.DefaultParams()
	baseline.RiskEquity = 375.73
	baseline = baseline.Normalize()
	other := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(40)})

	baseRun := batchRun(1, "baseline", baseline, true)
	baseRun.Status = "running"
	runs := []*tradeRepository.TradeBacktestRun{baseRun, batchRun(2, "ceiling=40", other, false)}
	metrics := map[int64]*tradeRepository.TradeBacktestMetric{
		2: batchMetric(2, signal.FidelityEvent, 168, 613),
	}
	groups, warnings := buildComparison(baseline, runs, metrics, 1)
	joined := strings.Join(warnings, " ")
	if !strings.Contains(joined, "基线") {
		t.Errorf("基线未就绪必须警示: %v", warnings)
	}
	for _, row := range groups[0].Rows {
		if row.IsBaseline {
			continue
		}
		if row.MetricDiff != nil {
			t.Error("基线无指标时不应给出差异")
		}
		if row.DiffBlockedReason == "" {
			t.Error("必须说明差异为何算不了")
		}
		if len(row.ParamDiff) == 0 {
			t.Error("参数差异与基线是否跑完无关，应始终可用")
		}
	}
}

// 失败/未完成的组沉到组内末尾，不与有效结果混在中间。
func TestBuildComparisonPushesUnfinishedRowsToTail(t *testing.T) {
	baseline := signal.DefaultParams()
	baseline.RiskEquity = 375.73
	baseline = baseline.Normalize()
	ok := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(40)})
	bad := applySignalParamKnobs(baseline, tradeDTO.SignalBacktestParamsDTO{Ceiling: intp(15)})

	failed := batchRun(3, "ceiling=15", bad, false)
	failed.Status, failed.ErrorMsg = "failed", "区间内没有 1m K 线"
	runs := []*tradeRepository.TradeBacktestRun{
		batchRun(1, "baseline", baseline, true), failed, batchRun(2, "ceiling=40", ok, false),
	}
	metrics := map[int64]*tradeRepository.TradeBacktestMetric{
		1: batchMetric(1, signal.FidelityEvent, 100, 613),
		2: batchMetric(2, signal.FidelityEvent, 168, 613),
	}
	groups, _ := buildComparison(baseline, runs, metrics, 1)
	rows := groups[0].Rows
	if rows[len(rows)-1].RunID != 3 || rows[len(rows)-1].Status != "failed" {
		t.Fatalf("失败组应沉到末尾: %+v", rows)
	}
	if rows[len(rows)-1].ErrorMsg == "" {
		t.Error("失败组必须带上失败原因")
	}
}

// 组标签里的数值只剪小数末尾的 0：整数串一律原样，40 不能被剪成 4。
func TestTrimTrailingZeros(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"40", "40"}, {"400", "400"}, {"0.350", "0.35"}, {"400.00", "400"}, {"3", "3"}, {"0.000600", "0.0006"},
	} {
		if got := trimTrailingZeros(c.in); got != c.want {
			t.Errorf("trimTrailingZeros(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
