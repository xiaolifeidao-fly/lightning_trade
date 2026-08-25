package argus_config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMainConfigImportMapsMainFilesWithoutSecretOutput(t *testing.T) {
	directory := t.TempDir()
	propertiesPath := filepath.Join(directory, "application.properties")
	sessionPath := filepath.Join(directory, "session.json")
	const secret = "do-not-log-this-secret"
	properties := strings.Join([]string{
		"server.port=8855",
		"request.path=/",
		"log.dir=logs",
		"trade.account_count=2",
		"trade.order_size=3",
		"position.monitor.interval_seconds=6",
		"position.monitor.profit_threshold=0.2",
		"position.monitor.loss_threshold=-0.1",
		"position.monitor.catastrophe_stop_pct=-0.3",
		"position.risk.budget_pct=0.5",
		"position.risk.max_contracts_ceiling=12",
		"position.monitor.trail.small_activate=0.1",
		"monitor.symbols.BTCUSDT.deep_inst=BTCUSDT",
		"monitor.symbols.BTCUSDT.trade_inst=BTC-USDT-SWAP",
		"monitor.symbols.BTCUSDT.threshold=0.01",
		"monitor.symbols.BTCUSDT.signal_threshold=0.02",
		"telegram.bot_token=" + secret,
		"telegram.chat_id=123",
		"trade.account1.name=primary",
		"trade.account1.url=https://example.test",
		"trade.account1.uid=1",
		"trade.account1.api_key=" + secret,
		"trade.account1.secret_key=secret-key",
		"trade.account1.passphrase=passphrase",
		"trade.account1.position_mode=bidirectional",
		"trade.account1.position_side=long",
		"trade.account1.tp_mode=trailing",
		"trade.account1.max_contracts_ceiling=5",
		"trade.account2.name=secondary",
		"trade.account2.url=https://example.test/two",
		"trade.account2.uid=2",
		"trade.account2.api_key=key-two",
		"trade.account2.secret_key=secret-two",
		"trade.account2.passphrase=passphrase-two",
	}, "\n")
	session := `{"accounts":{"primary":{"accountName":"primary","uid":"1","cookie":"` + secret + `","token":"token-one","updatedAt":"2026-08-20T00:00:00Z"},"secondary":{"accountName":"secondary","uid":"2","cookie":"cookie-two","token":"token-two"}}}`
	if err := os.WriteFile(propertiesPath, []byte(properties), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, []byte(session), 0o600); err != nil {
		t.Fatal(err)
	}

	request, summary, err := LoadMainConfigImport(propertiesPath, sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Accounts != 2 || summary.Sessions != 2 || summary.MonitorSymbols != 1 || !summary.TelegramSet {
		t.Fatalf("unexpected import summary: %+v", summary)
	}
	if request.Config.DefaultOrderSize != 3 || request.Config.MonitorIntervalSecond != 6 || request.Accounts[0].PositionMode != "hedge" {
		t.Fatalf("main settings not mapped: %+v %+v", request.Config, request.Accounts[0])
	}
	if request.AccountRisks[0].AccountID != 1 || request.AccountRisks[1].AccountID != 2 || request.Sessions[0].AccountID != 1 || request.Sessions[1].AccountID != 2 {
		t.Fatalf("account references are not stable: risks=%+v sessions=%+v", request.AccountRisks, request.Sessions)
	}
	if request.Sessions[0].Valid != 1 || request.Notification.TelegramBotToken != secret {
		t.Fatal("session or notification was not mapped")
	}
}

func TestLoadMainConfigImportRejectsUnexpectedFilesAndOrphanSessions(t *testing.T) {
	directory := t.TempDir()
	propertiesPath := filepath.Join(directory, "application.properties")
	sessionPath := filepath.Join(directory, "session.json")
	properties := "server.port=8855\ntrade.account_count=1\ntrade.account1.name=primary\ntrade.account1.url=https://example.test\nmonitor.symbols.BTCUSDT.deep_inst=BTCUSDT\nmonitor.symbols.BTCUSDT.trade_inst=BTC-USDT\nmonitor.symbols.BTCUSDT.threshold=0.01\n"
	if err := os.WriteFile(propertiesPath, []byte(properties), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath, []byte(`{"accounts":{"orphan":{"accountName":"orphan"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadMainConfigImport(filepath.Join(directory, "other.properties"), sessionPath); err == nil {
		t.Fatal("expected filename rejection")
	}
	if _, _, err := LoadMainConfigImport(propertiesPath, sessionPath); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("expected orphan-session rejection, got %v", err)
	}
}
