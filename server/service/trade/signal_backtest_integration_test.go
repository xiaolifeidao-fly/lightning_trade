package trade

import (
	"os"
	"testing"
	"time"

	commonDB "common/middleware/db"
	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"

	"argus_single/pkg/eventstore"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 真实 MySQL 集成校验。默认跳过（CI 与本地开发没有库），需要时显式给 DSN：
//
//	SIGNAL_BACKTEST_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/signal_bt_check?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test ./trade/ -run TestSignalBacktestIntegration -v
//
// 库会被反复建表并写入，请指向一个可丢弃的 schema，不要指向生产库。
// 它验证的是单测覆盖不到的三件事：
//  1. AutoMigrate 真的把新增列（engine_kind/fidelity/signal_count/...）建出来了；
//  2. 事件表按 instance_key + account_label 过滤确实只取到本实验体的触发流；
//  3. CreateSignalBacktestRun → RunSignalBacktest 的整条链路能把 run/metric/trade
//     三张表写成一致的状态。
//
// 实例键一律用 it- 前缀，绝不用真实部署的 argus.instance.id：用例会删自己
// 实例键下的行，用真实键等于给"DSN 指错库"配一把删除生产历史的枪。
const (
	itInstanceKey = "it-signal-backtest"
	itAccountA    = "it-账户A"
	itAccountB    = "it-账户B"
	itSymbol      = "BTCUSDT"
	itPlatform    = "deepcoin"
)

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("SIGNAL_BACKTEST_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 SIGNAL_BACKTEST_TEST_DSN，跳过真实 MySQL 集成校验")
	}
	g, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   logger.Default.LogMode(logger.Error),
	})
	if err != nil {
		t.Fatalf("连库失败: %v", err)
	}
	return g
}

// itSetup 把全局 db.Db 指到测试库并清掉本实例键下的残留。
// 注意 db.GetRepository 是全局单例 map：仓储实例只在首次取用时注入 Db，
// 所以必须先设 db.Db 再 NewTradeService。
func itSetup(t *testing.T) (*TradeService, *gorm.DB) {
	t.Helper()
	g := integrationDB(t)
	prev := commonDB.Db
	commonDB.Db = g
	t.Cleanup(func() { commonDB.Db = prev })

	svc := &TradeService{
		tradeKlineRepository:          newITRepo[tradeRepository.TradeKlineRepository](g),
		tradeBacktestRunRepository:    newITRepo[tradeRepository.TradeBacktestRunRepository](g),
		tradeBacktestTradeRepository:  newITRepo[tradeRepository.TradeBacktestTradeRepository](g),
		tradeBacktestMetricRepository: newITRepo[tradeRepository.TradeBacktestMetricRepository](g),
		tradeBacktestBatchRepository:  newITRepo[tradeRepository.TradeBacktestBatchRepository](g),
		tradeOptimizeStudyRepository:  newITRepo[tradeRepository.TradeOptimizeStudyRepository](g),
		tradeOptimizeCellRepository:   newITRepo[tradeRepository.TradeOptimizeCellRepository](g),
		strategyEventRepository:       newITRepo[tradeRepository.StrategyEventRepository](g),
		devSampleRepository:           newITRepo[tradeRepository.DevSampleRepository](g),
	}
	for _, m := range []interface{}{
		&tradeRepository.TradeKline{}, &tradeRepository.TradeBacktestRun{},
		&tradeRepository.TradeBacktestTrade{}, &tradeRepository.TradeBacktestMetric{},
		&tradeRepository.TradeBacktestBatch{},
		&tradeRepository.TradeOptimizeStudy{}, &tradeRepository.TradeOptimizeCell{},
		&eventstore.StrategyEvent{}, &eventstore.DevSample{},
	} {
		if err := g.AutoMigrate(m); err != nil {
			t.Fatalf("AutoMigrate %T: %v", m, err)
		}
	}
	g.Where("instance_key = ?", itInstanceKey).Delete(&eventstore.StrategyEvent{})
	g.Where("instance_key = ?", itInstanceKey).Delete(&eventstore.DevSample{})
	g.Where("platform_code = ? AND symbol = ?", itPlatform, itSymbol).Delete(&tradeRepository.TradeKline{})
	g.Where("instance_key = ?", itInstanceKey).Delete(&tradeRepository.TradeBacktestBatch{})
	g.Where("instance_key = ?", itInstanceKey).Delete(&tradeRepository.TradeBacktestRun{})
	g.Where("instance_key = ?", itInstanceKey).Delete(&tradeRepository.TradeOptimizeStudy{})
	g.Exec("DELETE FROM trade_optimize_cell WHERE study_id NOT IN (SELECT id FROM trade_optimize_study)")
	return svc, g
}

func newITRepo[R any](g *gorm.DB) *R {
	r := new(R)
	if setter, ok := any(r).(interface{ SetDb(*gorm.DB) }); ok {
		setter.SetDb(g)
	}
	return r
}

func itTime(min int) time.Time {
	return time.Date(2026, 8, 18, 0, 0, 0, 0, time.Local).Add(time.Duration(min) * time.Minute)
}

func itStrp(s string) *string   { return &s }
func itIntp(v int) *int         { return &v }
func itFltp(v float64) *float64 { return &v }

// TestSignalBacktestIntegrationNewColumnsExist 新增列必须真的被 AutoMigrate 建出来。
// gorm 的 tag 写错（例如 type:int 不生效、index 名冲突）在单测里完全看不出来。
func TestSignalBacktestIntegrationNewColumnsExist(t *testing.T) {
	_, g := itSetup(t)
	cases := []struct {
		model interface{}
		table string
		cols  []string
	}{
		{&tradeRepository.TradeBacktestRun{}, "trade_backtest_run",
			[]string{"engine_kind", "instance_key", "account_label", "signal_source",
				"fidelity", "fidelity_note", "signal_count"}},
		{&tradeRepository.TradeBacktestTrade{}, "trade_backtest_trade",
			[]string{"contracts", "max_contracts", "add_count", "peak_pct", "reduced_pnl"}},
		{&tradeRepository.TradeBacktestMetric{}, "trade_backtest_metric",
			[]string{"fidelity", "fidelity_note", "signal_count", "signal_dropped",
				"signal_filtered", "cap_skip_count", "gate_skip_count", "trend_skip_count", "reduce_count",
				"reduce_close_count", "eod_open_count", "max_stack", "cap_effective", "realized_pnl",
				"floating_pnl", "max_drawdown_pct", "lambda_per_day", "lambda_ratio", "lambda_self_test"}},
	}
	for _, c := range cases {
		for _, col := range c.cols {
			if !g.Migrator().HasColumn(c.model, col) {
				t.Errorf("%s 缺少列 %s", c.table, col)
			}
		}
	}
	// 既有的 trade_backtest_* 表在改造前就有数据，新增列必须是"加列"而不是重建表：
	// 旧列一个都不能少。
	for _, col := range []string{"prediction_interval", "prediction_variant", "kline_count"} {
		if !g.Migrator().HasColumn(&tradeRepository.TradeBacktestRun{}, col) {
			t.Errorf("trade_backtest_run 丢了既有列 %s", col)
		}
	}
}

// TestSignalBacktestIntegrationEndToEnd 整条链路：造事件 + K 线 → 建 run → 回放 → 查落库结果。
func TestSignalBacktestIntegrationEndToEnd(t *testing.T) {
	svc, g := itSetup(t)

	itSeedMarketAndEvents(t, g)

	// ③ 建 run 并同步回放（不走 CreateSignalBacktestRun 的异步 goroutine，
	//    否则测试要 sleep 等它）。
	runID, err := svc.CreateSignalBacktestRun(signalRunDTOForIT())
	if err != nil {
		t.Fatalf("建 run: %v", err)
	}
	// CreateSignalBacktestRun 已经异步起了一次回放，等它进终态再断言。
	// 不能在这里再手动跑一次 RunSignalBacktest——逐笔是 BatchCreate，会写两遍。
	waitRunDone(t, svc, runID)

	run, err := svc.tradeBacktestRunRepository.FindRunByID(runID)
	if err != nil {
		t.Fatalf("查 run: %v", err)
	}
	if run.Status != "done" {
		t.Fatalf("run 状态 = %s, 错误 = %s", run.Status, run.ErrorMsg)
	}
	if run.EngineKind != EngineKindSignal || run.SignalSource != SignalSourceStrategyEvent {
		t.Errorf("engine_kind/signal_source = %s/%s", run.EngineKind, run.SignalSource)
	}
	if run.Fidelity != signal.FidelityEvent {
		t.Errorf("精度等级 = %s, 期望 %s", run.Fidelity, signal.FidelityEvent)
	}
	if run.FidelityNote == "" {
		t.Error("fidelity_note 不得为空：保守假设必须随每条 run 落库")
	}
	// 零丢失零重复：只有账户A 的 3 次触发被回放（账户B 的那条被实例+账户过滤掉）。
	if run.SignalCount != 3 {
		t.Errorf("signal_count = %d, 期望 3（账户B 的触发不得混入）", run.SignalCount)
	}
	if run.KlineCount != 60 {
		t.Errorf("kline_count = %d, 期望 60", run.KlineCount)
	}

	metrics, err := svc.tradeBacktestMetricRepository.FindByRuns([]int64{runID})
	if err != nil || len(metrics) != 1 {
		t.Fatalf("查 metric: %v, rows=%d", err, len(metrics))
	}
	m := metrics[0]
	if m.CalcMode != CalcModeSignal || m.Fidelity != signal.FidelityEvent {
		t.Errorf("metric calc_mode/fidelity = %s/%s", m.CalcMode, m.Fidelity)
	}
	if m.SignalCount != 3 || m.MaxStack != 3 {
		t.Errorf("metric signal_count=%d max_stack=%d, 期望 3/3", m.SignalCount, m.MaxStack)
	}
	if m.CapEffective <= 0 {
		t.Errorf("cap_effective = %d, 应为回放生效的上限", m.CapEffective)
	}

	trades, err := svc.tradeBacktestTradeRepository.FindByRun(runID)
	if err != nil {
		t.Fatalf("查逐笔: %v", err)
	}
	if len(trades) != 1 {
		t.Fatalf("逐笔行数 = %d, 期望 1（一路上涨、未平仓 ⇒ 一条 eod）", len(trades))
	}
	tr := trades[0]
	if tr.CalcMode != CalcModeSignal || tr.CloseReason != signal.ExitEod || tr.Status != "open" {
		t.Errorf("逐笔口径有误: calc_mode=%s reason=%s status=%s", tr.CalcMode, tr.CloseReason, tr.Status)
	}
	if tr.Contracts != 3 || tr.AddCount != 3 {
		t.Errorf("张数/加仓次数 = %d/%d, 期望 3/3", tr.Contracts, tr.AddCount)
	}
}

// TestSignalBacktestIntegrationRejectsMissingEvents 区间内没有事件时必须明确报错，
// 而不是给出一个"0 笔、净盈亏 0"的假结果。
func TestSignalBacktestIntegrationRejectsMissingEvents(t *testing.T) {
	svc, _ := itSetup(t)
	dto := signalRunDTOForIT()
	dto.AccountLabel = "it-不存在的账户"
	runID, err := svc.CreateSignalBacktestRun(dto)
	if err != nil {
		t.Fatalf("建 run: %v", err)
	}
	waitRunTerminal(t, svc, runID)
	run, _ := svc.tradeBacktestRunRepository.FindRunByID(runID)
	if run.Status != "failed" || run.ErrorMsg == "" {
		t.Errorf("状态 = %s, 错误 = %q，期望 failed 且带原因", run.Status, run.ErrorMsg)
	}
}

// signalRunDTOForIT 集成用例的建 run 请求：只指定信号源与区间，
// 参数旋钮全部走生产缺省（cap 公式 + close 评估 + bar 收盘成交）。
func signalRunDTOForIT() tradeDTO.CreateSignalBacktestRunDTO {
	return tradeDTO.CreateSignalBacktestRunDTO{
		Name:         "it-signal-backtest",
		PlatformCode: itPlatform,
		CoinCode:     "BTC",
		Symbol:       itSymbol,
		StartTime:    itTime(0).Format("2006-01-02 15:04:05"),
		EndTime:      itTime(59).Format("2006-01-02 15:04:05"),
		InstanceKey:  itInstanceKey,
		AccountLabel: itAccountA,
		SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{
			RiskEquity: itFltp(375.73),
		},
	}
}

// itSeedMarketAndEvents 造集成用例的共享输入：60 根 1m K 线 + 4 条触发事件。
// 单次回测（r8）与批量扫描（r11）的集成用例共用同一份输入，好让两条路径的
// 断言数字（signal_count=3 / kline_count=60）可以直接互相对照。
//
//	K 线：价格从 60000 一路涨到 60590（+0.98%），不足以让 1 张小仓
//	（activate 150% ⇒ 1.2% 价格变动）被止盈带走 ⇒ 窗口末尾仍持仓（eod）。
//	事件：账户A 3 次 open（都在 cap 内）；账户B 1 次 open（必须被实例+账户
//	过滤掉，否则两本仓会被算成一本）。
func itSeedMarketAndEvents(t *testing.T, g *gorm.DB) {
	t.Helper()
	var klines []*tradeRepository.TradeKline
	for i := 0; i < 60; i++ {
		px := 60000 + float64(i)*10
		k := &tradeRepository.TradeKline{
			PlatformCode: itPlatform, Symbol: itSymbol, Interval: "1m",
			OpenTime: itTime(i), CloseTime: itTime(i).Add(time.Minute),
			OpenPrice: px, HighPrice: px, LowPrice: px, ClosePrice: px,
		}
		k.Init()
		klines = append(klines, k)
	}
	if err := g.CreateInBatches(klines, 100).Error; err != nil {
		t.Fatalf("写 K 线: %v", err)
	}

	var events []*eventstore.StrategyEvent
	add := func(account string, min int, side, ev string, gap float64) {
		events = append(events, &eventstore.StrategyEvent{
			Ts: itTime(min).Add(30 * time.Second), InstanceKey: itInstanceKey,
			AccountLabel: account, Event: ev, Instrument: itSymbol,
			Side: itStrp(side), OrderSize: itIntp(1), GapBp: itFltp(gap),
			SigLast: itFltp(60000), SigMark: itFltp(59980),
			Source: eventstore.SourceLive, IngestedAt: time.Now(),
		})
	}
	add(itAccountA, 1, "long", "open", 3.4)
	add(itAccountA, 2, "long", "open", 5.6)
	add(itAccountA, 3, "long", "open", 4.1)
	add(itAccountB, 1, "long", "open", 3.4)
	for i, e := range events {
		// event_hash 必须唯一：用序号打散，避免 16 字节截断后撞车。
		h := []byte("it-signal-hash--")
		h[15] = byte('0' + i)
		e.EventHash = h
	}
	if err := g.CreateInBatches(events, 50).Error; err != nil {
		t.Fatalf("写事件: %v", err)
	}
}

func waitRunDone(t *testing.T, svc *TradeService, runID int64) {
	t.Helper()
	waitRunTerminal(t, svc, runID)
	run, _ := svc.tradeBacktestRunRepository.FindRunByID(runID)
	if run.Status != "done" {
		t.Fatalf("run 未成功: status=%s err=%s", run.Status, run.ErrorMsg)
	}
}

func waitRunTerminal(t *testing.T, svc *TradeService, runID int64) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		run, err := svc.tradeBacktestRunRepository.FindRunByID(runID)
		if err == nil && (run.Status == "done" || run.Status == "failed") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("run=%d 20 秒内未进入终态", runID)
}
