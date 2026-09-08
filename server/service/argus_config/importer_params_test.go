package argus_config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 配置面收敛的验收口径是「DB 解析出来的值与 properties 原值逐项一致」，
// 所以导入必须把此前无列的 9 项一起搬进来，并且账户级覆盖不能被全局值抹平。
func TestLoadMainConfigImportCarriesNewlyConvergedParams(t *testing.T) {
	directory := t.TempDir()
	propertiesPath := filepath.Join(directory, "application_1.properties")
	sessionPath := filepath.Join(directory, "session.json")
	properties := strings.Join([]string{
		"argus.instance.id=argus-single-roc",
		"server.port=8855",
		"request.path=/",
		"trade.account_count=2",
		"trade.order_size=1",
		"position.monitor.interval_seconds=5",
		"position.risk.contract_face=0.001",
		"position.risk.budget_pct=20",
		"position.risk.reverse_gate_min_profit_pct=20",
		"position.monitor.catastrophe_stop_pct=300",
		"trade.signal.delay_seconds=5",
		"monitor.spread.max_price_age_ms=500",
		"trade.trend_gate.window_hours=24",
		"trade.trend_gate.threshold_pct=5",
		"monitor.symbols.BTCUSDT.deep_inst=BTC-USDT-SWAP",
		"monitor.symbols.BTCUSDT.trade_inst=BTCUSDT",
		"monitor.symbols.BTCUSDT.threshold=0.0012",
		"monitor.symbols.BTCUSDT.signal_threshold=0.0003",
		"trade.account1.name=账户A",
		"trade.account1.url=https://example.test",
		"trade.account1.api_key=key-one",
		"trade.account1.secret_key=secret-one",
		"trade.account1.passphrase=passphrase-one",
		"trade.account1.order_size=1",
		"trade.account1.risk_equity=1000",
		"trade.account1.budget_pct=13.3",
		"trade.account1.catastrophe_stop_pct=400",
		"trade.account1.max_contracts_ceiling=26",
		"trade.account1.reverse_gate_min_profit_pct=8",
		"trade.account1.trend_gate_threshold_pct=5",
		"trade.account2.name=账户B",
		"trade.account2.url=https://example.test/two",
		"trade.account2.api_key=key-two",
		"trade.account2.secret_key=secret-two",
		"trade.account2.passphrase=passphrase-two",
		"trade.account2.max_contracts_ceiling=8",
	}, "\n")
	if err := os.WriteFile(propertiesPath, []byte(properties), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, []byte(`{"accounts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	request, _, err := LoadMainConfigImport(propertiesPath, sessionPath)
	if err != nil {
		t.Fatalf("LoadMainConfigImport: %v", err)
	}

	config := request.Config
	if config.ContractFace != 0.001 || config.SignalDelaySecond != 5 || config.SpreadMaxPriceAgeMs != 500 {
		t.Fatalf("global params = %+v, want contract face/signal delay/price age carried over", config)
	}
	if config.TrendGateWindowHour != 24 || config.TrendGateThresholdPct != 5 || config.ReverseGateMinProfitPct != 20 {
		t.Fatalf("gate params = %+v, want trend gate and reverse gate carried over", config)
	}

	first := request.AccountRisks[0]
	if first.OrderSize != 1 || first.RiskEquity != 1000 || first.ReverseGateMinProfitPct != 8 || first.TrendGateThresholdPct != 5 {
		t.Fatalf("account 1 risk = %+v, want the account-level params carried over", first)
	}
	// 账户级 budget_pct / catastrophe_stop_pct 曾被全局值覆盖，
	// champion/challenger 的差异会在导入时被抹平。
	if first.RiskBudget != 13.3 || first.CatastrophicStopLoss != 400 {
		t.Fatalf("account 1 budget/stop = %v/%v, want the account-level 13.3/400", first.RiskBudget, first.CatastrophicStopLoss)
	}
	second := request.AccountRisks[1]
	if second.RiskBudget != 20 || second.CatastrophicStopLoss != 300 {
		t.Fatalf("account 2 budget/stop = %v/%v, want the global 20/300 fallback", second.RiskBudget, second.CatastrophicStopLoss)
	}
	if second.OrderSize != 0 || second.RiskEquity != 0 {
		t.Fatalf("account 2 = %+v, want zero for params it never configured", second)
	}
}
