package runtimeconfig

import (
	"context"
	"testing"

	"argus_single/pkg/trade"
	"common/middleware/vipper"
	"service/argus_config/repository"
)

// signalSnapshot 是一份最小可用快照：一个启用账户 + 一个监控币种，
// 账户参数照 application_1.properties 的 challenger 口径填。
func signalSnapshot(t *testing.T) Snapshot {
	t.Helper()
	account := &repository.ArgusAccount{
		AccountName:  "账户B",
		URL:          "https://api.deepcoin.com",
		UID:          "10086",
		PositionMode: "hedge",
		PositionSide: "both",
		APIKey:       "api-key",
		SecretKey:    "secret-key",
		Passphrase:   "passphrase",
		Enabled:      1,
	}
	account.Id = 7
	risk := &repository.ArgusAccountRisk{
		AccountID:               7,
		TakeProfitMode:          "trailing",
		StopLossMode:            "catastrophic",
		TrailingStopTiersJSON:   `{"small_activate":150,"small_giveback":0.35,"medium_activate":90,"medium_giveback":0.28,"large_activate":40,"large_giveback":0.2,"tier_small_ratio":0.3,"tier_large_ratio":0.65}`,
		RiskBudget:              13.3,
		CatastrophicStopLoss:    400,
		ReverseGateEnabled:      1,
		MaxContracts:            8,
		ExtraRiskJSON:           `{"tradeDirection":"forward","tradeLogic":"signal","variant":"challenger/S400_cap8_gate8"}`,
		OrderSize:               3,
		RiskEquity:              1200,
		ReverseGateMinProfitPct: 8,
		TrendGateThresholdPct:   5,
	}
	return Snapshot{
		Version: repository.ArgusConfigVersion{InstanceKey: "argus-single-roc", Version: 11},
		Config: repository.ArgusConfig{
			ServerPort:              8855,
			RequestPath:             "/",
			LogDir:                  "./logs",
			DefaultOrderSize:        1,
			MonitorIntervalSecond:   5,
			ProfitThreshold:         150,
			LossThreshold:           150,
			ContractFace:            0.001,
			SignalDelaySecond:       5,
			SpreadMaxPriceAgeMs:     500,
			TrendGateWindowHour:     24,
			TrendGateThresholdPct:   5,
			ReverseGateMinProfitPct: 20,
		},
		Accounts:       []*repository.ArgusAccount{account},
		AccountRisks:   []*repository.ArgusAccountRisk{risk},
		MonitorSymbols: []*repository.ArgusMonitorSymbol{{Symbol: "BTCUSDT", DeepInstrument: "BTC-USDT-SWAP", TradeInstrument: "BTCUSDT", SpreadThreshold: 0.0012, SignalThreshold: 0.0003, Enabled: 1}},
	}
}

// 这是本任务修的功能性故障：DB 驱动下 trade_logic 读不出来 → 归一化成 spread →
// IsSignalLogic() 永远为假 → 整套盘口信号策略静默失效。
func TestRuntimeFromSnapshotConsumesExtraRisk(t *testing.T) {
	runtime, err := runtimeFromSnapshot(signalSnapshot(t), "checksum")
	if err != nil {
		t.Fatalf("runtimeFromSnapshot: %v", err)
	}
	account := runtime.Trade.Accounts[0]
	if !account.IsSignalLogic() {
		t.Fatalf("trade_logic = %q, want signal", account.TradeLogic)
	}
	if account.Variant != "challenger/S400_cap8_gate8" {
		t.Fatalf("variant = %q, want the challenger label", account.Variant)
	}
	if account.TradeDirection != trade.TradeDirectionForward {
		t.Fatalf("trade_direction = %q, want forward", account.TradeDirection)
	}
	if account.StopLossMode != trade.StopLossModeCatastrophic {
		t.Fatalf("stop_loss_mode = %q, want catastrophic", account.StopLossMode)
	}
	if account.OrderSize != 3 {
		t.Fatalf("order_size = %d, want 3", account.OrderSize)
	}
	// hedge 是 DB 的枚举拼写；信号逻辑账户归一化后必须落在 net。
	if account.PositionMode != "net" {
		t.Fatalf("position_mode = %q, want net", account.PositionMode)
	}
}

func TestRuntimeFromSnapshotRejectsMalformedExtraRisk(t *testing.T) {
	snapshot := signalSnapshot(t)
	snapshot.AccountRisks[0].ExtraRiskJSON = "{not json"
	if _, err := runtimeFromSnapshot(snapshot, "checksum"); err == nil {
		t.Fatal("want an error: silently defaulting trade_logic to spread is the failure mode being fixed")
	}
}

func TestRuntimeFromSnapshotBuildsParamOverrides(t *testing.T) {
	runtime, err := runtimeFromSnapshot(signalSnapshot(t), "checksum")
	if err != nil {
		t.Fatalf("runtimeFromSnapshot: %v", err)
	}
	account := runtime.Overrides.Accounts[1]
	for name, want := range map[string]float64{
		"small_activate":              150,
		"tier_large_ratio":            0.65,
		"budget_pct":                  13.3,
		"catastrophe_stop_pct":        400,
		"max_contracts_ceiling":       8,
		"risk_equity":                 1200,
		"reverse_gate_min_profit_pct": 8,
		"trend_gate_threshold_pct":    5,
	} {
		if got := account[name]; got != want {
			t.Fatalf("override %s = %v, want %v", name, got, want)
		}
	}
	if got := runtime.Overrides.Global["position.risk.reverse_gate_min_profit_pct"]; got != 20 {
		t.Fatalf("global reverse_gate_min_profit_pct = %v, want 20", got)
	}
}

func TestRuntimeFromSnapshotCarriesTuning(t *testing.T) {
	runtime, err := runtimeFromSnapshot(signalSnapshot(t), "checksum")
	if err != nil {
		t.Fatalf("runtimeFromSnapshot: %v", err)
	}
	want := RuntimeTuning{MonitorIntervalSecond: 5, ProfitThreshold: 150, LossThreshold: 150, ContractFace: 0.001, SignalDelaySecond: 5, SpreadMaxPriceAgeMs: 500, TrendGateWindowHour: 24}
	if runtime.Tuning != want {
		t.Fatalf("tuning = %#v, want %#v", runtime.Tuning, want)
	}
}

func TestSetTuningValueRestoresPropertiesWhenUnset(t *testing.T) {
	const key = "argus.test.tuning_fallback"
	vipper.Set(key, 7)
	t.Cleanup(func() { vipper.Set(key, nil) })

	setTuningValue(key, 21)
	if got := vipper.GetFloat64(key); got != 21 {
		t.Fatalf("value = %v, want the database value 21", got)
	}
	// DB 把这一项清空（0）时不能留着旧值，要退回 properties 原值。
	setTuningValue(key, 0)
	if got := vipper.GetFloat64(key); got != 7 {
		t.Fatalf("value = %v, want the properties fallback 7", got)
	}
}

func TestReconcileConfigSurvivesUnavailableDatabase(t *testing.T) {
	// 数据库不可用时，比对只能记一条告警后返回，绝不能 panic 或把运行中的
	// 配置清掉。配置改为 DB 唯一来源后这条更要紧：以前 Redis 命中还能兜一下，
	// 现在查库失败就是唯一的失败面，必须保持"什么都不做"。
	manager := &Manager{instanceID: "argus-single-roc", current: RuntimeConfig{Version: 4, Fingerprint: "running"}}
	manager.reconcileConfig(context.Background())
	if manager.Current().Version != 4 {
		t.Fatalf("version = %d, want the running config untouched", manager.Current().Version)
	}
	if manager.Current().Fingerprint != "running" {
		t.Fatalf("fingerprint = %q, want the running config untouched", manager.Current().Fingerprint)
	}
}


func TestHotAppliedReportsTuningChange(t *testing.T) {
	current := RuntimeConfig{Trade: &trade.TradingSystemConfig{}, Tuning: RuntimeTuning{MonitorIntervalSecond: 5}}
	next := RuntimeConfig{Trade: &trade.TradingSystemConfig{}, Tuning: RuntimeTuning{MonitorIntervalSecond: 10}}
	fields := hotApplied(next, current)
	if len(fields) != 1 || fields[0] != "tuning" {
		t.Fatalf("hot applied = %#v, want tuning only", fields)
	}
	// 巡检间隔属于热生效，不该被算进「需要重启」。
	if restart := restartRequired(next, current); len(restart) != 0 {
		t.Fatalf("restart required = %#v, want empty", restart)
	}
}

func TestApplyRuntimeConfigRejectsIllegalRiskParams(t *testing.T) {
	t.Cleanup(func() { trade.SetParamOverrides(trade.ParamOverrides{}) })
	snapshot := signalSnapshot(t)
	current, err := runtimeFromSnapshot(snapshot, "checksum")
	if err != nil {
		t.Fatalf("runtimeFromSnapshot: %v", err)
	}
	// 兜底止损被误改到 100%：紧止损会杀死均值回归 edge，ValidateRiskParams
	// 要求 ≥250。这份快照必须被拒绝，而不是让 NewTradeManager 打死进程。
	snapshot.AccountRisks[0].CatastrophicStopLoss = 100
	next, err := runtimeFromSnapshot(snapshot, "checksum-2")
	if err != nil {
		t.Fatalf("runtimeFromSnapshot: %v", err)
	}
	if err := applyRuntimeConfig(next, current); err == nil {
		t.Fatal("want the illegal snapshot to be rejected before the hot swap")
	}
	// 覆盖层必须回滚回当前配置，否则拒绝了快照却留下了它的参数。
	if got := trade.AccFloat(1, "catastrophe_stop_pct", "position.monitor.catastrophe_stop_pct", 300); got != 400 {
		t.Fatalf("catastrophe_stop_pct = %v, want the running 400 after rollback", got)
	}
}
