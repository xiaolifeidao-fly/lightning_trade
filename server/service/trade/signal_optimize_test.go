package trade

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"
)

// 本文件覆盖寻优编排层里不碰 DB 的那部分：基线口径覆盖、格子建行与跳过、
// 配置继承（OOS）、排序口径、任务级提醒。方法学内核的用例在
// strategy/signal/{study_test,calibration_test}.go。

func optBaseParams() signal.Params {
	p := signal.DefaultParams()
	p.OrderSize = 1
	p.RiskEquity = 375.73
	p.CapOverride = 0
	p.Ceiling = 15
	p.CatastropheStopPct = 300
	p.GateMinProfitPct = 20
	return p.Normalize()
}

// 寻优的费用与执行模型是研究口径（0.012%/边 + 兜底过冲 5 点），不是回测缺省
// （0.06%/边 + 无过冲）。费率错了，门控与兜底的重校准结论全错。
func TestResolveOptimizeBaselineAppliesStudyFeeModel(t *testing.T) {
	s := &TradeService{}
	dto := tradeDTO.CreateSignalOptimizeStudyDTO{
		BaselineParams: &tradeDTO.SignalBacktestParamsDTO{
			RiskEquity: optFltp(375.73),
			OrderSize:  optIntp(1),
			Ceiling:    optIntp(15),
		},
	}
	b, err := s.resolveOptimizeBaseline(context.Background(), dto, nil, "inst", "acc", "BTCUSDT")
	if err != nil {
		t.Fatalf("定基线: %v", err)
	}
	if b.Params.TakerFee != OptimizeEffectiveTakerFee {
		t.Errorf("taker 费率应被盖成 %.5f，得到 %.5f", OptimizeEffectiveTakerFee, b.Params.TakerFee)
	}
	if b.Params.CatastropheOvershootRoiPts != OptimizeOvershootRoiPts {
		t.Errorf("兜底过冲应被盖成 %.0f 点，得到 %.2f", OptimizeOvershootRoiPts, b.Params.CatastropheOvershootRoiPts)
	}
	note := strings.Join(b.Notes, " | ")
	if !strings.Contains(note, "0.012") || !strings.Contains(note, "过冲") {
		t.Errorf("口径覆盖必须在说明里交代清楚，实际：%s", note)
	}

	// 显式给了就尊重请求，不再强盖。
	dto.BaselineParams.TakerFee = optFltp(0.0006)
	dto.BaselineParams.CatastropheOvershootRoiPts = optFltp(0)
	b, err = s.resolveOptimizeBaseline(context.Background(), dto, nil, "inst", "acc", "BTCUSDT")
	if err != nil {
		t.Fatalf("定基线(显式): %v", err)
	}
	if b.Params.TakerFee != 0.0006 || b.Params.CatastropheOvershootRoiPts != 0 {
		t.Errorf("显式费用口径被覆盖了: taker=%.5f overshoot=%.2f", b.Params.TakerFee, b.Params.CatastropheOvershootRoiPts)
	}
}

// 参数在实盘校验下非法的格子建成 skipped 而不是丢掉：搜索空间的某个角落装不进
// 这个账户，是必须被看见的事实（实例3 的 order_size=10 让 cap 5/7 物理上开不进）。
func TestBuildOptimizeCellRowsMarksInfeasibleAsSkipped(t *testing.T) {
	base := optBaseParams()
	base.OrderSize = 10
	cells := []signal.CellSpec{
		{Mode: signal.ModeNet, Cap: 26, StopPct: 400, GatePct: 20},
		{Mode: signal.ModeDual, Cap: 5, StopPct: 400, GatePct: 20}, // order_size 10 > 上限 5
		{Mode: signal.ModeNet, Cap: 20, StopPct: 200, GatePct: 20}, // S<250 触发松兜底护栏
	}
	rows, skipped := buildOptimizeCellRows(7, OptimizeStageCoarse, cells, base)
	if len(rows) != 3 || skipped != 2 {
		t.Fatalf("应建 3 行、跳过 2 行，得到 %d/%d", len(rows), skipped)
	}
	if rows[0].Status != OptimizeCellPending || rows[0].CellKey != "net(26,400,g20)" {
		t.Errorf("可行格不该被跳过: %+v", rows[0])
	}
	for _, i := range []int{1, 2} {
		if rows[i].Status != OptimizeCellSkipped {
			t.Errorf("第 %d 行应为 skipped，得到 %s", i, rows[i].Status)
		}
		if !strings.Contains(rows[i].ErrorMsg, "不是失败") {
			t.Errorf("跳过原因必须说明它不是失败: %s", rows[i].ErrorMsg)
		}
	}
	// 参数快照必须是完整的 signal.Params：它就是"把这一格丢给单次回测看逐笔"的入口。
	p, err := signalParamsFromSnapshot(rows[0].ParamsSnapshot)
	if err != nil {
		t.Fatalf("参数快照不可用: %v", err)
	}
	if p.CapOverride != 26 || p.CatastropheStopPct != 400 || p.GateMinProfitPct != 20 {
		t.Errorf("四个搜索轴没落到快照上: %+v", p)
	}
	if p.TakerFee != base.TakerFee || p.SmallActivatePct != base.SmallActivatePct {
		t.Errorf("非搜索轴的旋钮应沿用基线: %+v", p)
	}
	// 寻优不动 signal_threshold ⇒ 每格恒为事件级。
	if rows[0].Fidelity != signal.FidelityEvent {
		t.Errorf("寻优格应恒为事件级，得到 %s", rows[0].Fidelity)
	}
}

func TestResolveIncumbentCellFallsBackToBaseline(t *testing.T) {
	base := optBaseParams() // CapOverride=0 / Ceiling=15 / S=300 / gate=20
	got := resolveIncumbentCell(tradeDTO.CreateSignalOptimizeStudyDTO{}, nil, base)
	if got == nil || got.Key() != "net(15,300,g20)" {
		t.Fatalf("现行配置格应由基线推出 net(15,300,g20)，得到 %v", got)
	}
	// 显式给的优先。
	got = resolveIncumbentCell(tradeDTO.CreateSignalOptimizeStudyDTO{
		Incumbent: &tradeDTO.SignalOptimizeCellInput{Cap: 26, StopPct: 400, GatePct: 8},
	}, nil, base)
	if got == nil || got.Key() != "net(26,400,g8)" {
		t.Fatalf("显式现行配置格没生效: %v", got)
	}
	// OOS 任务继承基准任务的现行配置格。
	snapshot, _ := json.Marshal(signal.CellSpec{Mode: "net", Cap: 12, StopPct: 350, GatePct: 20})
	got = resolveIncumbentCell(tradeDTO.CreateSignalOptimizeStudyDTO{},
		&tradeRepository.TradeOptimizeStudy{IncumbentSnapshot: string(snapshot)}, base)
	if got == nil || got.Key() != "net(12,350,g20)" {
		t.Fatalf("OOS 应继承基准任务的现行配置格: %v", got)
	}
	// 基线连上限都没有 → 不硬造一个格。
	bad := base
	bad.Ceiling, bad.CapOverride = 0, 0
	if got := resolveIncumbentCell(tradeDTO.CreateSignalOptimizeStudyDTO{}, nil, bad); got != nil {
		t.Errorf("无上限可推时不该造格: %v", got)
	}
}

// 阈值没给时，本金取基线的 risk_equity 而不是研究里那个 375.73：两个绝对阈值
// 都是"占本金百分比"，用别人的本金算出来的 U 值毫无意义。
func TestResolveOptimizeConfigDerivesGatesFromBaselineEquity(t *testing.T) {
	s := &TradeService{}
	base := optBaseParams()
	base.RiskEquity = 120
	_, protocols, converge, gates, err := s.resolveOptimizeConfig(tradeDTO.CreateSignalOptimizeStudyDTO{}, nil, base)
	if err != nil {
		t.Fatalf("组装配置: %v", err)
	}
	if gates.RiskEquity != 120 {
		t.Errorf("三关本金应取基线的 120U，得到 %.2f", gates.RiskEquity)
	}
	if diff := gates.BearNetP10Min - (-18); diff > 1e-6 || diff < -1e-6 {
		t.Errorf("120U × 15%% 应派生出 −18U，得到 %.4f", gates.BearNetP10Min)
	}
	if len(protocols.Fine.Paths()) != 16 || len(protocols.Coarse.Paths()) != 4 {
		t.Errorf("协议缺省应是 16/4 条路径，得到 %d/%d", len(protocols.Fine.Paths()), len(protocols.Coarse.Paths()))
	}
	if converge.TopK != 10 || !converge.ExpandGateAxis {
		t.Errorf("收敛规则缺省不对: %+v", converge)
	}

	// 粗网格必须沿用精算的费用/情景/λ 口径，只换抖动轴：否则"粗筛掉的格子"
	// 与"精算留下的格子"不是同一个量在比较。
	_, protocols, _, _, err = s.resolveOptimizeConfig(tradeDTO.CreateSignalOptimizeStudyDTO{
		Protocol: &tradeDTO.SignalOptimizeProtocolInput{BootstrapDraws: 500, ScenarioDays: 20},
	}, nil, base)
	if err != nil {
		t.Fatalf("组装配置(自定义协议): %v", err)
	}
	if protocols.Coarse.BootstrapDraws != 500 || protocols.Coarse.ScenarioDays != 20 {
		t.Errorf("粗网格没沿用精算口径: %+v", protocols.Coarse)
	}
	if len(protocols.Coarse.OffsetDays) != 2 {
		t.Errorf("粗网格抖动轴应仍是 2 个偏移，得到 %v", protocols.Coarse.OffsetDays)
	}
}

// OOS 任务的配置一律继承基准任务的冻结值，一个字节都不重新算。
func TestResolveOptimizeConfigInheritsFrozenSnapshotsForOos(t *testing.T) {
	s := &TradeService{}
	space := signal.SearchSpace{NetCaps: []int{26}, StopPcts: []float64{400}, GatePcts: []float64{8}, CoarseGatePct: 8}
	protocols := optimizeProtocolPair{Fine: signal.FineProtocol(), Coarse: signal.CoarseProtocol()}
	protocols.Fine.BootstrapDraws = 777
	gates := signal.Gates{RiskEquity: 414, BearBudgetPct: 15, DdMaxPct: 25, SignMin: 0.9}.Normalize()
	conv := signal.Convergence{TopK: 4, MaxCells: 8}.Normalize()

	spaceJSON, _ := json.Marshal(space)
	protoJSON, _ := json.Marshal(protocols)
	gateJSON, _ := json.Marshal(gates)
	convJSON, _ := json.Marshal(conv)
	base := &tradeRepository.TradeOptimizeStudy{
		SpaceSnapshot: string(spaceJSON), ProtocolSnapshot: string(protoJSON),
		GateSnapshot: string(gateJSON), ConvergeSnapshot: string(convJSON),
	}
	gotSpace, gotProto, gotConv, gotGates, err := s.resolveOptimizeConfig(
		tradeDTO.CreateSignalOptimizeStudyDTO{
			// 请求里的这些值必须被忽略：OOS 不允许重新调参。
			Converge: &tradeDTO.SignalOptimizeConvergeInput{TopK: 99},
			Protocol: &tradeDTO.SignalOptimizeProtocolInput{BootstrapDraws: 1},
		}, base, optBaseParams())
	if err != nil {
		t.Fatalf("继承配置: %v", err)
	}
	if gotGates.SignMin != 0.9 || gotGates.RiskEquity != 414 {
		t.Errorf("三关阈值没继承: %+v", gotGates)
	}
	if gotProto.Fine.BootstrapDraws != 777 {
		t.Errorf("降噪协议没继承（请求里的 1 不该生效）: %d", gotProto.Fine.BootstrapDraws)
	}
	if gotConv.TopK != 4 {
		t.Errorf("收敛规则没继承（请求里的 99 不该生效）: %d", gotConv.TopK)
	}
	if len(gotSpace.NetCaps) != 1 || gotSpace.NetCaps[0] != 26 {
		t.Errorf("搜索空间没继承: %+v", gotSpace)
	}
}

func TestNormalizeOptimizeConcurrency(t *testing.T) {
	cases := map[int]int{0: DefaultOptimizeConcurrency, -3: DefaultOptimizeConcurrency,
		1: 1, 5: 5, 99: MaxOptimizeConcurrency}
	for in, want := range cases {
		if got := normalizeOptimizeConcurrency(in); got != want {
			t.Errorf("concurrency %d → %d，期望 %d", in, got, want)
		}
	}
}

// 排序必须**判定优先于收益**：只按中位 PnL 排，排在前面的就会是噪声格
// （现行 champion 一致率 0.56，按点估计排它并不难看）。
func TestSortOptimizeRowsPutsVerdictBeforePnl(t *testing.T) {
	rows := []tradeDTO.SignalOptimizeCellRowDTO{
		{Key: "noisy", Status: OptimizeCellDone, PassCount: 1, MedPnl28: 500},
		{Key: "failed", Status: OptimizeCellFailed, PassCount: 0, MedPnl28: 999},
		{Key: "solid", Status: OptimizeCellDone, PassCount: 3, MedPnl28: 80},
		{Key: "mid", Status: OptimizeCellDone, PassCount: 1, MedPnl28: 600},
	}
	sortOptimizeRows(rows)
	want := []string{"solid", "mid", "noisy", "failed"}
	for i, w := range want {
		if rows[i].Key != w {
			t.Fatalf("排序结果 %v，期望 %v", []string{rows[0].Key, rows[1].Key, rows[2].Key, rows[3].Key}, want)
		}
	}
}

// 落库列 → 统计量的往返必须无损：收敛与判定读的都是库里的行（断点续跑时那是唯一真值）。
func TestCellStatsFieldsAndRowRoundTrip(t *testing.T) {
	spec := signal.CellSpec{Mode: signal.ModeNet, Cap: 26, StopPct: 400, GatePct: 20}
	st := signal.CellStats{
		Spec: spec, Key: spec.Key(), PathCount: 16, Fidelity: signal.FidelityEvent,
		MedPnl28: 142.66269677, P25Pnl28: 90, P75Pnl28: 180, IQRPnl28: 90,
		MinPnl28: 10, MaxPnl28: 260, SignRatio: 1,
		LambdaBearPerMonth: 2.727273, MeanStopLoss: 41.66692948, StopBudget: 113.63708041,
		StopCount: 6, P90MaxDrawdown: 88.84061897, MaxStack: 26, MedFee: 9.27707476, MedDays: 23,
		MedSignalRun: 1459,
		Scenarios: []signal.ScenarioResult{
			{Scenario: signal.ScenarioBear, PoolSize: 176, P10: -113.40381354, P50: 24.10704861, P90: 150},
			{Scenario: signal.ScenarioChop, P10: -20, P50: 5},
			{Scenario: signal.ScenarioMixed, P10: -50, P50: 30},
		},
	}
	f := cellStatsFields(st)
	if f["bear_p10"] != signal.RoundTo(-113.40381354, 8) || f["bear_pool_size"] != 176 {
		t.Errorf("熊市情景没摊到列上: %v", f)
	}
	if f["chop_p50"] != 5.0 || f["mixed_p10"] != -50.0 {
		t.Errorf("震荡/混合情景没摊到列上: %v", f)
	}

	row := &tradeRepository.TradeOptimizeCell{
		Mode: spec.Mode, Cap: spec.Cap, StopPct: spec.StopPct, GatePct: spec.GatePct,
		CellKey: st.Key, Status: OptimizeCellDone, PathCount: 16,
		MedPnl28: f["med_pnl28"].(float64), SignRatio: f["sign_ratio"].(float64),
		LambdaBear: f["lambda_bear"].(float64), MeanStopLoss: f["mean_stop_loss"].(float64),
		StopBudget: f["stop_budget"].(float64), P90MaxDrawdown: f["p90_max_drawdown"].(float64),
		BearPoolSize: 176, BearP10: f["bear_p10"].(float64), BearP50: f["bear_p50"].(float64),
		Fidelity: signal.FidelityEvent,
	}
	back := cellStatsFromRow(row)
	if back.Key != st.Key || back.Spec != st.Spec {
		t.Errorf("格子标识往返失真: %+v", back.Spec)
	}
	if bear := back.Scenario(signal.ScenarioBear); bear == nil || bear.P10 != row.BearP10 {
		t.Errorf("熊市分位往返失真: %+v", bear)
	}
	// 往返后必须仍能被判定（这是收敛与结论的前提）。
	if v := signal.Judge(back, signal.DefaultGates()); v.OkBear {
		t.Errorf("p10=−113 不该过熊市关: %+v", v)
	}
}

// 任务级提醒不是装饰：其中任意一条被忽略，读到的数字就会被当成更强的证据。
func TestOptimizeWarningsCoverTheNonNegotiables(t *testing.T) {
	study := &tradeRepository.TradeOptimizeStudy{
		SampleKind:    SampleKindInSample,
		GateNote:      "三关：① 一致率 ≥ 0.80 ...",
		GateLockedAt:  time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local),
		Status:        OptimizeStatusPartial,
		SkipCellCount: 3,
		TrendDayCount: 4,
		Verdict:       signal.VerdictNoSolution,
		BaselineNote:  "risk_equity 无列，按 initial_balance 兜底",
	}
	w := strings.Join(optimizeWarnings(study, nil), " || ")
	for _, want := range []string{
		"不输出「最优参数」", "in_sample", "预注册阈值", "部分失败",
		"因参数在实盘校验下非法而跳过", "样本极薄", "无解分支", "基线取值说明",
	} {
		if !strings.Contains(w, want) {
			t.Errorf("提醒里缺少 %q：%s", want, w)
		}
	}

	oos := &tradeRepository.TradeOptimizeStudy{SampleKind: SampleKindOutSample, OosBaseID: 12, Status: OptimizeStatusDone}
	w = strings.Join(optimizeWarnings(oos, nil), " || ")
	if !strings.Contains(w, "out_of_sample") || !strings.Contains(w, "#12") {
		t.Errorf("OOS 提醒不对: %s", w)
	}

	// 频率级格子混进来时必须点名：寻优不该动 signal_threshold。
	w = strings.Join(optimizeWarnings(&tradeRepository.TradeOptimizeStudy{SampleKind: SampleKindInSample},
		[]tradeDTO.SignalOptimizeCellRowDTO{{Fidelity: signal.FidelityFrequency}}), " || ")
	if !strings.Contains(w, "频率级") {
		t.Errorf("频率级格子应被点名: %s", w)
	}
}

func TestScaleInvarianceLines(t *testing.T) {
	ok := scaleInvarianceLines([]signal.ScaleInvarianceCheck{{Key: "net(15,300,g20)", Feasible: true}}, "本金 414U")
	if len(ok) != 1 || !strings.Contains(ok[0], "都能按公式实例化") {
		t.Errorf("全可行时的文案不对: %v", ok)
	}
	broken := scaleInvarianceLines([]signal.ScaleInvarianceCheck{
		{Key: "net(26,400,g8)", CellCap: 26, FormulaCap: 8, Feasible: false},
		{Key: "net(15,300,g20)", Feasible: true},
	}, "本金 120U")
	if len(broken) != 1 || !strings.Contains(broken[0], "规模不变性破缺") ||
		!strings.Contains(broken[0], "需26张/公式8张") {
		t.Errorf("破缺文案不对: %v", broken)
	}
	if !strings.Contains(broken[0], "方向性") {
		t.Errorf("破缺文案必须说明只能验方向性: %s", broken[0])
	}
}

// 缺省预览要让人在提交前就能核对空间/协议/阈值三样，并知道大概要跑多少次回放。
func TestGetSignalOptimizeDefaults(t *testing.T) {
	s := &TradeService{}
	d := s.GetSignalOptimizeDefaults()
	if d.CoarseCellCount != 55 || len(d.CoarseCells) != 55 {
		t.Errorf("缺省粗网格应为 55 格，得到 %d", d.CoarseCellCount)
	}
	if d.FinePathCount != 16 || d.CoarsePathCount != 4 {
		t.Errorf("路径数应为 16/4，得到 %d/%d", d.FinePathCount, d.CoarsePathCount)
	}
	// 55×4 + min(10×2, 40)×16 = 220 + 320 = 540
	if d.EstimatedReplays != 540 {
		t.Errorf("预估回放次数应为 540，得到 %d", d.EstimatedReplays)
	}
	if d.GateNote == "" || d.Gates == nil || d.Space == nil || d.Protocol == nil {
		t.Errorf("缺省预览字段缺失: %+v", d)
	}
	notes := strings.Join(d.Notes, " || ")
	for _, want := range []string{"§3.3", "signal_threshold 不在搜索空间", "跑完不接受修改", "0.012"} {
		if !strings.Contains(notes, want) {
			t.Errorf("缺省说明里缺少 %q：%s", want, notes)
		}
	}
}

func optIntp(v int) *int         { return &v }
func optFltp(v float64) *float64 { return &v }
