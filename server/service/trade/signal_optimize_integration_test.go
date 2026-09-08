package trade

import (
	"context"
	"strings"
	"testing"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"
)

// 自动参数寻优（r16）的真实 MySQL 集成校验。与 r8/r11 的集成用例共用
// SIGNAL_BACKTEST_TEST_DSN 与 itSetup / itSeedMarketAndEvents，断言的是单测
// 覆盖不到的四件事：
//
//  1. trade_optimize_study / trade_optimize_cell 真的被 AutoMigrate 建出来，
//     且既有回测三张表的列一个不少（新表是加表，不是重建）；
//  2. 端到端：显式精算格 → 多路径回放 → 落库 → 用**创建时锁定的**阈值判定 →
//     出结论；gate_locked_at 在执行前后不变；
//  3. 无解分支真的走通：没有格子过三关时 verdict=no_solution，且结论里给出
//     权衡前沿与被支配的现行配置，不出现"最优参数"；
//  4. 参数非法的格子建成 skipped 而不是 failed，且不阻断其余格子。
//
// 集成库里没有 argus_config 数据，所以基线一律用 baselineParams 显式给出；
// "基线从生产配置映射"由 signal_batch_test.go 的单测覆盖。
//
// 窗口只有 60 根 1m，所以协议把起点偏移压到 [0]、只留 close/悲观两条路径
// —— 抖动轴的展开顺序与 16 路径口径已由 strategy/signal 的单测与金标准校准钉住，
// 这里要验的是编排与落库。

func itOptimizeProtocol() *tradeDTO.SignalOptimizeProtocolInput {
	return &tradeDTO.SignalOptimizeProtocolInput{
		OffsetDays:       []int{0},
		DropSeeds:        []*int64{nil},
		CoarseOffsetDays: []int{0},
		CoarseDropSeeds:  []*int64{nil},
		ScenarioDays:     1, // 窗口只有 1 个自然日，情景月按 1 天合成
		BootstrapDraws:   200,
	}
}

func itOptimizeDTO(fine ...tradeDTO.SignalOptimizeCellInput) tradeDTO.CreateSignalOptimizeStudyDTO {
	return tradeDTO.CreateSignalOptimizeStudyDTO{
		Name:         "it-signal-optimize",
		PlatformCode: itPlatform,
		CoinCode:     "BTC",
		Symbol:       itSymbol,
		StartTime:    itTime(0).Format("2006-01-02 15:04:05"),
		EndTime:      itTime(59).Format("2006-01-02 15:04:05"),
		InstanceKey:  itInstanceKey,
		AccountLabel: itAccountA,
		Concurrency:  2,
		BaselineParams: &tradeDTO.SignalBacktestParamsDTO{
			RiskEquity: itFltp(375.73),
			OrderSize:  itIntp(1),
			Ceiling:    itIntp(15),
		},
		Protocol:  itOptimizeProtocol(),
		Incumbent: &tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 15, StopPct: 300, GatePct: 20},
		FineCells: fine,
	}
}

// TestSignalOptimizeIntegrationNewTablesExist 两张新表的列必须真的建出来，
// 且既有回测表的列一个不少。
func TestSignalOptimizeIntegrationNewTablesExist(t *testing.T) {
	_, g := itSetup(t)
	for _, col := range []string{
		"instance_key", "account_label", "platform_code", "symbol", "start_time", "end_time",
		"sample_kind", "sample_note", "oos_base_id",
		"space_snapshot", "protocol_snapshot", "converge_snapshot",
		"gate_snapshot", "gate_locked_at", "gate_note",
		"baseline_snapshot", "baseline_source", "baseline_note", "incumbent_snapshot",
		"stage", "concurrency", "coarse_cell_count", "fine_cell_count",
		"done_cell_count", "failed_cell_count", "skip_cell_count", "replay_count",
		"status", "error_msg", "converge_note",
		"signal_count", "kline_count", "trend_day_count", "vol_day_count",
		"verdict", "passed_cell_count", "conclusion", "frontier_snapshot", "scale_snapshot",
	} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeOptimizeStudy{}, col) {
			t.Errorf("trade_optimize_study 缺少列 %s", col)
		}
	}
	for _, col := range []string{
		"study_id", "stage", "cell_key", "mode", "cap_contracts", "stop_pct", "gate_pct",
		"params_snapshot", "fidelity", "status", "error_msg", "path_count",
		"med_pnl28", "p25_pnl28", "p75_pnl28", "iqr_pnl28", "min_pnl28", "max_pnl28", "sign_ratio",
		"lambda_bear", "mean_stop_loss", "stop_budget", "stop_count",
		"p90_max_drawdown", "max_stack", "med_fee", "med_days", "med_signal_run",
		"bear_pool_size", "bear_p10", "bear_p50", "bear_p90", "chop_p10", "chop_p50",
		"mixed_p10", "mixed_p50",
		"ok_sign", "ok_bear", "ok_dd", "ok_budget", "passed", "pass_count", "verdict_note",
		"dominated_by", "is_incumbent", "on_dd_frontier", "on_bear_frontier",
		"paths_snapshot", "note_snapshot",
	} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeOptimizeCell{}, col) {
			t.Errorf("trade_optimize_cell 缺少列 %s", col)
		}
	}
	// 新增两张表不该影响 r8/r11 的既有表。
	for _, col := range []string{"engine_kind", "fidelity", "signal_count", "batch_id", "group_label"} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeBacktestRun{}, col) {
			t.Errorf("trade_backtest_run 丢了既有列 %s", col)
		}
	}
}

// TestSignalOptimizeIntegrationEndToEndNoSolution 端到端跑通并走无解分支。
func TestSignalOptimizeIntegrationEndToEndNoSolution(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	studyID, err := svc.CreateSignalOptimizeStudy(context.Background(), itOptimizeDTO(
		tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 15, StopPct: 300, GatePct: 20},
		tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 26, StopPct: 400, GatePct: 20},
	))
	if err != nil {
		t.Fatalf("建寻优任务: %v", err)
	}
	created, err := svc.tradeOptimizeStudyRepository.FindStudyByID(studyID)
	if err != nil {
		t.Fatalf("读任务: %v", err)
	}
	lockedAt := created.GateLockedAt
	if lockedAt.IsZero() {
		t.Fatal("阈值锁定时刻必须在创建时写死")
	}
	if created.Stage != OptimizeStageFine || created.FineCellCount != 2 || created.CoarseCellCount != 0 {
		t.Errorf("显式精算格应跳过粗网格: stage=%s fine=%d coarse=%d",
			created.Stage, created.FineCellCount, created.CoarseCellCount)
	}
	if created.SampleKind != SampleKindInSample {
		t.Errorf("样本口径应为 in_sample，得到 %s", created.SampleKind)
	}

	study := waitOptimizeTerminal(t, svc, studyID)
	if study.Status != OptimizeStatusDone {
		t.Fatalf("任务未成功: status=%s err=%s", study.Status, study.ErrorMsg)
	}
	// 阈值预注册：执行期任何路径都不许改它。
	if !study.GateLockedAt.Equal(lockedAt) {
		t.Errorf("阈值锁定时刻被执行期改动了: %v → %v", lockedAt, study.GateLockedAt)
	}
	if study.Stage != OptimizeStageConcluded {
		t.Errorf("跑完应进入 concluded，得到 %s", study.Stage)
	}
	if study.DoneCellCount != 2 || study.FailedCellCount != 0 {
		t.Errorf("2 格应全部跑通: done=%d failed=%d", study.DoneCellCount, study.FailedCellCount)
	}
	// 2 格 × 2 条路径 = 4 次回放。
	if study.ReplayCount != 4 {
		t.Errorf("回放次数应为 4，得到 %d", study.ReplayCount)
	}
	// 数据侧事实：3 条账户A触发 + 60 根 1m（与 r8/r11 用例同一份输入）。
	if study.SignalCount != 3 || study.KlineCount != 60 {
		t.Errorf("数据事实不对: signal=%d kline=%d", study.SignalCount, study.KlineCount)
	}
	if study.Verdict != signal.VerdictNoSolution || study.PassedCellCount != 0 {
		t.Errorf("这份 1 小时窗口不可能过三关，应走无解分支: verdict=%s passed=%d",
			study.Verdict, study.PassedCellCount)
	}

	detail, err := svc.GetSignalOptimizeStudyDetail(studyID, true)
	if err != nil {
		t.Fatalf("取详情: %v", err)
	}
	if len(detail.Fine) != 2 || len(detail.Coarse) != 0 {
		t.Fatalf("详情应只有 2 个精算格: fine=%d coarse=%d", len(detail.Fine), len(detail.Coarse))
	}
	incumbentSeen := false
	for _, row := range detail.Fine {
		if row.Status != OptimizeCellDone {
			t.Errorf("%s 未跑通: %s", row.Key, row.ErrorMsg)
			continue
		}
		if row.PathCount != 2 {
			t.Errorf("%s 应跑 2 条路径，得到 %d", row.Key, row.PathCount)
		}
		if row.Fidelity != signal.FidelityEvent {
			t.Errorf("%s 应为事件级，得到 %s", row.Key, row.Fidelity)
		}
		if row.Params == nil || row.Params["capOverride"] == nil {
			t.Errorf("%s 缺参数快照（它是「丢给单次回测看逐笔」的入口）", row.Key)
		}
		if len(row.Notes) == 0 {
			t.Errorf("%s 缺口径说明", row.Key)
		}
		if len(row.Paths) != 2 {
			t.Errorf("%s 的逐路径快照应有 2 条，得到 %d", row.Key, len(row.Paths))
		}
		// 熊市月情景在这份窗口里估计不出来（1 个安静日，没有单边日块）
		// ⇒ 该关必须按不通过计，"不可判定"不等于"通过"。
		if row.OkBear {
			t.Errorf("%s 熊市不可估计却放行了", row.Key)
		}
		if row.Passed {
			t.Errorf("%s 不该通过三关", row.Key)
		}
		if row.VerdictNote == "" {
			t.Errorf("%s 未过关必须给出逐条原因", row.Key)
		}
		if row.IsIncumbent {
			incumbentSeen = true
			if row.Key != "net(15,300,g20)" {
				t.Errorf("现行配置格标错了: %s", row.Key)
			}
		}
	}
	if !incumbentSeen {
		t.Error("现行配置格必须被标出来：「现行配置是否被支配」是本任务必须回答的问题")
	}

	if detail.Conclusion == nil {
		t.Fatal("详情缺结论")
	}
	if detail.Conclusion["verdict"] != signal.VerdictNoSolution {
		t.Errorf("结论 verdict 不对: %v", detail.Conclusion["verdict"])
	}
	if len(detail.Frontier) != 2 {
		t.Errorf("权衡前沿应含 2 格，得到 %d", len(detail.Frontier))
	}
	if len(detail.ScaleChecks) != 2 {
		t.Errorf("规模不变性核查应含 2 格，得到 %d", len(detail.ScaleChecks))
	}
	// 冻结快照必须随详情返回：没有它们，结果无从复算。
	for name, m := range map[string]map[string]interface{}{
		"gates": detail.Gates, "protocol": detail.Protocol, "space": detail.Space, "converge": detail.Converge,
	} {
		if len(m) == 0 {
			t.Errorf("详情缺 %s 冻结快照", name)
		}
	}
	if detail.Study.GateLockedAt == "" || detail.Study.GateNote == "" {
		t.Error("详情必须回显阈值锁定时刻与三关表述")
	}
	warn := strings.Join(detail.Warnings, " || ")
	for _, want := range []string{"不输出「最优参数」", "in_sample", "预注册阈值", "无解分支"} {
		if !strings.Contains(warn, want) {
			t.Errorf("提醒里缺少 %q：%s", want, warn)
		}
	}
	// 结论正文里绝不能出现"最优参数"。
	lines, _ := detail.Conclusion["lines"].([]interface{})
	all, _ := detail.Conclusion["headline"].(string)
	for _, l := range lines {
		if s, ok := l.(string); ok {
			all += " " + s
		}
	}
	if strings.Contains(all, "最优参数") {
		t.Errorf("结论里出现了「最优参数」：%s", all)
	}
	if !strings.Contains(all, "权衡前沿") {
		t.Errorf("无解分支必须给出权衡前沿：%s", all)
	}
}

// TestSignalOptimizeIntegrationInfeasibleCellSkipped 参数非法的格子建成 skipped，
// 不阻断其余格子，也不计进"失败"。
func TestSignalOptimizeIntegrationInfeasibleCellSkipped(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	dto := itOptimizeDTO(
		tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 20, StopPct: 400, GatePct: 20},
		// S=200 触发松兜底护栏（口径同实盘启动校验）→ 本格跳过。
		tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 20, StopPct: 200, GatePct: 20},
	)
	studyID, err := svc.CreateSignalOptimizeStudy(context.Background(), dto)
	if err != nil {
		t.Fatalf("建寻优任务: %v", err)
	}
	study := waitOptimizeTerminal(t, svc, studyID)
	if study.Status != OptimizeStatusDone {
		t.Fatalf("一个格子非法不该让整个任务失败: status=%s err=%s", study.Status, study.ErrorMsg)
	}
	if study.SkipCellCount != 1 || study.FailedCellCount != 0 || study.DoneCellCount != 1 {
		t.Errorf("计数不对: skip=%d failed=%d done=%d",
			study.SkipCellCount, study.FailedCellCount, study.DoneCellCount)
	}
	if !strings.Contains(study.ErrorMsg, "不是失败") {
		t.Errorf("跳过说明应出现在任务级消息里: %s", study.ErrorMsg)
	}
	rows, err := svc.tradeOptimizeCellRepository.FindCellsByStudy(studyID, OptimizeStageFine)
	if err != nil {
		t.Fatalf("取格子: %v", err)
	}
	skipped := 0
	for _, r := range rows {
		if r.Status == OptimizeCellSkipped {
			skipped++
			if !strings.Contains(r.ErrorMsg, "250") {
				t.Errorf("跳过原因应引用实盘校验的护栏文案: %s", r.ErrorMsg)
			}
		}
	}
	if skipped != 1 {
		t.Errorf("应有 1 格 skipped，得到 %d", skipped)
	}
}

// TestSignalOptimizeIntegrationOosRejectsRetuning OOS 任务不允许换阈值或重新选格，
// 也不允许把定参数时见过的窗口当样本外。
func TestSignalOptimizeIntegrationOosRejectsRetuning(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	baseID, err := svc.CreateSignalOptimizeStudy(context.Background(), itOptimizeDTO(
		tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 20, StopPct: 400, GatePct: 20},
	))
	if err != nil {
		t.Fatalf("建基准任务: %v", err)
	}
	base := waitOptimizeTerminal(t, svc, baseID)
	if base.Stage != OptimizeStageConcluded {
		t.Fatalf("基准任务未出结论: stage=%s err=%s", base.Stage, base.ErrorMsg)
	}

	// ① 窗口起点早于阈值锁定时刻 → 不是样本外。
	early := itOptimizeDTO()
	early.OosBaseStudyID = baseID
	early.Protocol = nil
	if _, err := svc.CreateSignalOptimizeStudy(context.Background(), early); err == nil ||
		!strings.Contains(err.Error(), "不是样本外") {
		t.Errorf("应拒绝定参数时见过的窗口，得到 err=%v", err)
	}

	// ② 给了新阈值 → 直接拒绝（改阈值就不是 OOS 验证了）。
	future := itOptimizeDTO()
	future.OosBaseStudyID = baseID
	future.Protocol = nil
	future.StartTime = base.GateLockedAt.Add(time.Hour).Format("2006-01-02 15:04:05")
	future.EndTime = base.GateLockedAt.Add(2 * time.Hour).Format("2006-01-02 15:04:05")
	withGates := future
	withGates.Gates = &tradeDTO.SignalOptimizeGatesInput{SignMin: 0.5}
	if _, err := svc.CreateSignalOptimizeStudy(context.Background(), withGates); err == nil ||
		!strings.Contains(err.Error(), "不允许给 gates") {
		t.Errorf("OOS 不该允许换阈值，得到 err=%v", err)
	}

	// ③ 重新选格同样拒绝。
	withCells := future
	withCells.FineCells = []tradeDTO.SignalOptimizeCellInput{{Mode: "net", Cap: 26, StopPct: 400, GatePct: 20}}
	if _, err := svc.CreateSignalOptimizeStudy(context.Background(), withCells); err == nil ||
		!strings.Contains(err.Error(), "不允许给 space/fineCells") {
		t.Errorf("OOS 不该允许重新选格，得到 err=%v", err)
	}

	// ④ 合法的 OOS 任务：继承阈值与精算格，只换窗口。该窗口没有事件，
	//    取数失败是预期的——这里验的是"继承与标注"，不是"跑出结果"。
	oosID, err := svc.CreateSignalOptimizeStudy(context.Background(), future)
	if err != nil {
		t.Fatalf("建 OOS 任务: %v", err)
	}
	oos := waitOptimizeTerminal(t, svc, oosID)
	if oos.SampleKind != SampleKindOutSample || oos.OosBaseID != baseID {
		t.Errorf("样本口径标注不对: kind=%s base=%d", oos.SampleKind, oos.OosBaseID)
	}
	if !oos.GateLockedAt.Equal(base.GateLockedAt) {
		t.Errorf("OOS 应沿用基准任务的阈值锁定时刻: %v vs %v", oos.GateLockedAt, base.GateLockedAt)
	}
	if oos.GateSnapshot != base.GateSnapshot {
		t.Error("OOS 的三关阈值快照必须与基准任务逐字相同")
	}
	if oos.FineCellCount != 1 {
		t.Errorf("应继承 1 个精算格，得到 %d", oos.FineCellCount)
	}
	if !strings.Contains(oos.SampleNote, "不重新调参") {
		t.Errorf("OOS 说明不对: %s", oos.SampleNote)
	}
}

// TestSignalOptimizeIntegrationCoarseToFineConverges 粗网格 → 自动收敛精算格的
// 完整两阶段流程。这是任务说明里"粗网格→自动收敛精算格"那一句的落地验证：
// 收敛不是判定，它只按粗网格中位 PnL 筛格 + 补上全路径为正的格 + 强制纳入现行
// 配置格，随后 gate 副轴展开成精算清单。
func TestSignalOptimizeIntegrationCoarseToFineConverges(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	dto := itOptimizeDTO() // 不给 fineCells ⇒ 走粗网格
	dto.Space = &tradeDTO.SignalOptimizeSpaceInput{
		NetCaps:       []int{15, 20},
		StopPcts:      []float64{300, 400},
		GatePcts:      []float64{8, 20},
		IncludeDual:   itBoolp(false),
		CoarseGatePct: 20,
	}
	dto.Converge = &tradeDTO.SignalOptimizeConvergeInput{
		TopK: 2, KeepAllPositive: itBoolp(false), ExpandGateAxis: itBoolp(true), MaxCells: 10,
	}
	// 现行配置格刻意选一个不在搜索空间里的格，验证它确实被强制纳入。
	dto.Incumbent = &tradeDTO.SignalOptimizeCellInput{Mode: "net", Cap: 12, StopPct: 350, GatePct: 20}

	studyID, err := svc.CreateSignalOptimizeStudy(context.Background(), dto)
	if err != nil {
		t.Fatalf("建寻优任务: %v", err)
	}
	created, err := svc.tradeOptimizeStudyRepository.FindStudyByID(studyID)
	if err != nil {
		t.Fatalf("读任务: %v", err)
	}
	// 2 cap × 2 S，gate 固定 20 ⇒ 粗网格 4 格
	if created.Stage != OptimizeStageCoarse || created.CoarseCellCount != 4 {
		t.Fatalf("粗网格应为 4 格: stage=%s coarse=%d", created.Stage, created.CoarseCellCount)
	}

	study := waitOptimizeTerminal(t, svc, studyID)
	if study.Status != OptimizeStatusDone {
		t.Fatalf("任务未成功: status=%s err=%s", study.Status, study.ErrorMsg)
	}
	if study.Stage != OptimizeStageConcluded {
		t.Errorf("跑完应进入 concluded，得到 %s", study.Stage)
	}
	// 前 2 名 + 强制纳入的现行配置格 = 3 格，× gate 副轴 2 档 = 6 格精算
	if study.FineCellCount != 6 {
		t.Errorf("精算格应为 6 格（(top2 + 现行) × gate 2 档），得到 %d", study.FineCellCount)
	}
	// 4 格粗网格 × 2 路径 + 6 格精算 × 2 路径 = 20 次回放
	if study.ReplayCount != 20 {
		t.Errorf("回放次数应为 20，得到 %d", study.ReplayCount)
	}
	if study.ConvergeNote == "" {
		t.Fatal("收敛必须交代「为什么选这些格」")
	}
	for _, want := range []string{"按中位 PnL 取前", "强制纳入现行配置", "gate 副轴展开"} {
		if !strings.Contains(study.ConvergeNote, want) {
			t.Errorf("收敛说明里缺少 %q：%s", want, study.ConvergeNote)
		}
	}

	detail, err := svc.GetSignalOptimizeStudyDetail(studyID, false)
	if err != nil {
		t.Fatalf("取详情: %v", err)
	}
	if len(detail.Coarse) != 4 || len(detail.Fine) != 6 {
		t.Fatalf("详情分段不对: coarse=%d fine=%d", len(detail.Coarse), len(detail.Fine))
	}
	fineKeys := map[string]bool{}
	for _, r := range detail.Fine {
		fineKeys[r.Key] = true
	}
	// 现行配置格的两个 gate 变体都应在精算清单里，且原始 gate 那一格被标为现行。
	for _, want := range []string{"net(12,350,g20)", "net(12,350,g8)"} {
		if !fineKeys[want] {
			t.Errorf("精算清单缺少 %s：%v", want, fineKeys)
		}
	}
	incumbent := 0
	for _, r := range detail.Fine {
		if r.IsIncumbent {
			incumbent++
			if r.Key != "net(12,350,g20)" {
				t.Errorf("现行配置格标错: %s", r.Key)
			}
		}
	}
	if incumbent != 1 {
		t.Errorf("应恰好 1 格被标为现行配置，得到 %d", incumbent)
	}
	// 粗网格的行不带判定：4 条路径分辨不出 0.69 与 0.80，不允许在这一层下结论。
	for _, r := range detail.Coarse {
		if r.Passed || r.PassCount != 0 {
			t.Errorf("粗网格格 %s 不该带三关判定: passed=%v cnt=%d", r.Key, r.Passed, r.PassCount)
		}
	}
}

func itBoolp(v bool) *bool { return &v }

func waitOptimizeTerminal(t *testing.T, svc *TradeService, studyID int64) *tradeRepository.TradeOptimizeStudy {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		study, err := svc.tradeOptimizeStudyRepository.FindStudyByID(studyID)
		if err == nil {
			switch study.Status {
			case OptimizeStatusDone, OptimizeStatusPartial, OptimizeStatusFailed:
				return study
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	study, _ := svc.tradeOptimizeStudyRepository.FindStudyByID(studyID)
	t.Fatalf("寻优任务 %d 60 秒未进入终态: status=%s err=%s", studyID, study.Status, study.ErrorMsg)
	return nil
}
