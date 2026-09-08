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

// 参数组批量扫描（r11）的真实 MySQL 集成校验。与 r8 的集成用例共用
// SIGNAL_BACKTEST_TEST_DSN 与 itSetup / itSeedMarketAndEvents，
// 断言的是单测覆盖不到的三件事：
//
//  1. trade_backtest_batch 与 run 上的批次三列真的被 AutoMigrate 建出来；
//  2. 一次提交 N 组 → 并发跑完 → 每组都有自己的 metric 与逐笔，且**共享同一份
//     输入**（各组的 signal_count / kline_count 必须逐一相等）；
//  3. 事件级与频率级混在同一批时，对比结果分成两组、跨组不给指标差异。
//
// 基线一律用 baselineParams 显式给出，不去读 argus_config 的已发布版本：
// 集成库里没有配置数据，而"基线从生产配置映射"这条逻辑已由 signal_batch_test.go
// 的 baselineFromSnapshot 用例覆盖。

// itBatchDTO 批量扫描的集成请求：信号源与窗口同 r8 用例，基线显式给出。
func itBatchDTO(groups ...tradeDTO.SignalBacktestGroupDTO) tradeDTO.CreateSignalBacktestBatchDTO {
	return tradeDTO.CreateSignalBacktestBatchDTO{
		Name:         "it-signal-batch",
		PlatformCode: itPlatform,
		CoinCode:     "BTC",
		Symbol:       itSymbol,
		StartTime:    itTime(0).Format("2006-01-02 15:04:05"),
		EndTime:      itTime(59).Format("2006-01-02 15:04:05"),
		InstanceKey:  itInstanceKey,
		AccountLabel: itAccountA,
		Concurrency:  3,
		BaselineParams: &tradeDTO.SignalBacktestParamsDTO{
			RiskEquity:          itFltp(375.73),
			BaselineThresholdBp: itFltp(5),
			SignalThresholdBp:   itFltp(5),
		},
		Groups: groups,
	}
}

// TestSignalBatchIntegrationNewColumnsExist 批次表与 run 上的批次三列必须真的建出来。
func TestSignalBatchIntegrationNewColumnsExist(t *testing.T) {
	_, g := itSetup(t)
	for _, col := range []string{
		"instance_key", "account_label", "platform_code", "symbol", "start_time", "end_time",
		"baseline_snapshot", "baseline_source", "baseline_note", "baseline_run_id",
		"concurrency", "group_count", "done_count", "failed_count", "status", "error_msg",
	} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeBacktestBatch{}, col) {
			t.Errorf("trade_backtest_batch 缺少列 %s", col)
		}
	}
	for _, col := range []string{"batch_id", "group_label", "is_baseline"} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeBacktestRun{}, col) {
			t.Errorf("trade_backtest_run 缺少批次列 %s", col)
		}
	}
	// 批次列是加列而不是重建表：r8 的列一个都不能少。
	for _, col := range []string{"engine_kind", "fidelity", "signal_count", "prediction_interval"} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeBacktestRun{}, col) {
			t.Errorf("trade_backtest_run 丢了既有列 %s", col)
		}
	}
}

// TestSignalBatchIntegrationEndToEnd 一次提交 3 组（基线 + cap15 + cap40）→ 并发跑完
// → 对比矩阵按精度分组、基线置顶、同精度组有指标差异。
func TestSignalBatchIntegrationEndToEnd(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	batchID, err := svc.CreateSignalBacktestBatch(context.Background(), itBatchDTO(
		tradeDTO.SignalBacktestGroupDTO{
			Label:                   "ceiling=15",
			SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{Ceiling: itIntp(15)},
		},
		tradeDTO.SignalBacktestGroupDTO{
			// 不给 label：服务端应按 diff 自动生成 "ceiling=40"。
			SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{Ceiling: itIntp(40)},
		},
	))
	if err != nil {
		t.Fatalf("建批次: %v", err)
	}
	batch := waitBatchTerminal(t, svc, batchID)
	if batch.Status != BatchStatusDone {
		t.Fatalf("批次状态 = %s, 错误 = %s", batch.Status, batch.ErrorMsg)
	}
	if batch.GroupCount != 3 || batch.DoneCount != 3 || batch.FailedCount != 0 {
		t.Errorf("进度计数有误: group=%d done=%d failed=%d", batch.GroupCount, batch.DoneCount, batch.FailedCount)
	}
	if batch.BaselineRunID == 0 {
		t.Fatal("baseline_run_id 未回填：没有基线就没有差异参照物")
	}
	if batch.BaselineSource != BaselineSourceRequest {
		t.Errorf("baseline_source = %q", batch.BaselineSource)
	}

	runs, err := svc.tradeBacktestRunRepository.FindRunsByBatch(batchID)
	if err != nil || len(runs) != 3 {
		t.Fatalf("查批次下的组: %v, rows=%d", err, len(runs))
	}
	// 共享数据的验收口径：三组的输入必须逐一相等。若实现改成各组各取一次，
	// 这里会在事件被并发写入时出现不等（也就是把数据差异算进了参数差异）。
	for _, run := range runs {
		if run.Status != "done" {
			t.Fatalf("组 %s 未成功: status=%s err=%s", run.GroupLabel, run.Status, run.ErrorMsg)
		}
		if run.SignalCount != 3 {
			t.Errorf("组 %s signal_count = %d, 期望 3（三组共享同一份触发流）", run.GroupLabel, run.SignalCount)
		}
		if run.KlineCount != 60 {
			t.Errorf("组 %s kline_count = %d, 期望 60", run.GroupLabel, run.KlineCount)
		}
		if run.EngineKind != EngineKindSignal || run.BatchID != batchID {
			t.Errorf("组 %s 归属有误: engine=%s batch=%d", run.GroupLabel, run.EngineKind, run.BatchID)
		}
		if run.Fidelity != signal.FidelityEvent {
			t.Errorf("组 %s 精度 = %s, 期望事件级", run.GroupLabel, run.Fidelity)
		}
		// 每组都要有自己的逐笔与汇总，不能只有一份。
		if trades, err := svc.tradeBacktestTradeRepository.FindByRun(int64(run.Id)); err != nil || len(trades) == 0 {
			t.Errorf("组 %s 没有逐笔明细: %v", run.GroupLabel, err)
		}
	}
	// 未给 label 的那组应被自动命名。
	labels := map[string]bool{}
	for _, run := range runs {
		labels[run.GroupLabel] = true
	}
	for _, want := range []string{"baseline", "ceiling=15", "ceiling=40"} {
		if !labels[want] {
			t.Errorf("缺少组标签 %q，实到 %v", want, labels)
		}
	}

	detail, err := svc.GetSignalBacktestBatchDetail(batchID)
	if err != nil {
		t.Fatalf("查批次详情: %v", err)
	}
	if len(detail.Groups) != 1 || detail.Groups[0].Fidelity != signal.FidelityEvent {
		t.Fatalf("全事件级应只有一个精度组: %+v", detail.Groups)
	}
	g0 := detail.Groups[0]
	if !g0.ComparableToBaseline || g0.SortedBy != "netPnl" {
		t.Errorf("可比标记/排序依据有误: comparable=%v sortedBy=%s", g0.ComparableToBaseline, g0.SortedBy)
	}
	if len(g0.Rows) != 3 || !g0.Rows[0].IsBaseline {
		t.Fatalf("基线行必须置顶: %+v", g0.Rows)
	}
	if len(g0.Notes) == 0 {
		t.Error("精度警示必须随对比结果返回：回测的保守假设不可隐藏")
	}
	for _, row := range g0.Rows[1:] {
		if row.Metric == nil {
			t.Errorf("组 %s 缺少汇总指标", row.GroupLabel)
			continue
		}
		if row.MetricDiff == nil {
			t.Errorf("组 %s 与基线同精度，应有指标差异（reason=%q）", row.GroupLabel, row.DiffBlockedReason)
		}
		if len(row.ParamDiff) != 1 || row.ParamDiff[0].Field != "ceiling" {
			t.Errorf("组 %s 的参数差异应只有 ceiling: %+v", row.GroupLabel, row.ParamDiff)
		}
	}
	if detail.Baseline.Params["riskEquity"] == nil {
		t.Error("批次详情应回显冻结的基线参数")
	}
}

// TestSignalBatchIntegrationDoesNotMixFidelity 事件级与频率级混在同一批时，
// 对比结果必须分成两组，且频率级组不给出与基线的指标差异。
func TestSignalBatchIntegrationDoesNotMixFidelity(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	batchID, err := svc.CreateSignalBacktestBatch(context.Background(), itBatchDTO(
		tradeDTO.SignalBacktestGroupDTO{
			Label:                   "ceiling=40",
			SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{Ceiling: itIntp(40)},
		},
		tradeDTO.SignalBacktestGroupDTO{
			// θ 由 5bp 抬到 8bp ⇒ 频率级。
			Label:                   "theta=8bp",
			SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{SignalThresholdBp: itFltp(8)},
		},
	))
	if err != nil {
		t.Fatalf("建批次: %v", err)
	}
	if batch := waitBatchTerminal(t, svc, batchID); batch.Status != BatchStatusDone {
		t.Fatalf("批次状态 = %s, 错误 = %s", batch.Status, batch.ErrorMsg)
	}

	detail, err := svc.GetSignalBacktestBatchDetail(batchID)
	if err != nil {
		t.Fatalf("查批次详情: %v", err)
	}
	if len(detail.Groups) != 2 {
		t.Fatalf("应分成事件级/频率级两组, got %d: %+v", len(detail.Groups), detail.Groups)
	}
	if detail.Groups[0].Fidelity != signal.FidelityEvent || detail.Groups[1].Fidelity != signal.FidelityFrequency {
		t.Fatalf("事件级必须在前: %s / %s", detail.Groups[0].Fidelity, detail.Groups[1].Fidelity)
	}
	if detail.Groups[1].ComparableToBaseline {
		t.Error("频率级组不得标为与基线可比")
	}
	for _, row := range detail.Groups[1].Rows {
		if row.MetricDiff != nil {
			t.Error("跨精度不得给出指标差异")
		}
		if !strings.Contains(row.DiffBlockedReason, "跨精度") {
			t.Errorf("拒绝理由应说明跨精度: %q", row.DiffBlockedReason)
		}
	}
	if !strings.Contains(strings.Join(detail.Warnings, " "), "不得比大小") {
		t.Errorf("多精度批次必须给出混排警示: %v", detail.Warnings)
	}
}

// TestSignalBatchIntegrationRejectsBadGroupBeforeCreating 任一组参数非法时整批拒绝，
// 且**不留下**半残的批次与 run —— 否则页面上会多出一个永远跑不完的批次。
func TestSignalBatchIntegrationRejectsBadGroupBeforeCreating(t *testing.T) {
	svc, g := itSetup(t)
	itSeedMarketAndEvents(t, g)

	var before int64
	g.Model(&tradeRepository.TradeBacktestBatch{}).Where("instance_key = ?", itInstanceKey).Count(&before)

	_, err := svc.CreateSignalBacktestBatch(context.Background(), itBatchDTO(
		tradeDTO.SignalBacktestGroupDTO{
			Label:                   "ok",
			SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{Ceiling: itIntp(40)},
		},
		tradeDTO.SignalBacktestGroupDTO{
			// S=100 违反实盘校验（catastrophe_stop_pct 必须 ≥250）。
			Label:                   "bad-S",
			SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{CatastropheStopPct: itFltp(100)},
		},
	))
	if err == nil {
		t.Fatal("含非法参数组的批次必须整批拒绝")
	}
	if !strings.Contains(err.Error(), "bad-S") || !strings.Contains(err.Error(), "catastrophe_stop_pct") {
		t.Errorf("错误应指出是哪一组、哪个参数: %v", err)
	}
	var after int64
	g.Model(&tradeRepository.TradeBacktestBatch{}).Where("instance_key = ?", itInstanceKey).Count(&after)
	if after != before {
		t.Errorf("拒绝的批次不得落库: before=%d after=%d", before, after)
	}
}

func waitBatchTerminal(t *testing.T, svc *TradeService, batchID int64) *tradeRepository.TradeBacktestBatch {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		batch, err := svc.tradeBacktestBatchRepository.FindBatchByID(batchID)
		if err != nil {
			t.Fatalf("查批次: %v", err)
		}
		switch batch.Status {
		case BatchStatusDone, BatchStatusPartial, BatchStatusFailed:
			return batch
		}
		if time.Now().After(deadline) {
			t.Fatalf("批次 %d 30 秒内未进终态，当前 status=%s done=%d failed=%d",
				batchID, batch.Status, batch.DoneCount, batch.FailedCount)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
