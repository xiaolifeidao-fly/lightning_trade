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

func TestLoadMainConfigImportAcceptsPerInstancePropertiesAndReadsInstanceKey(t *testing.T) {
	baseProperties := "server.port=8855\nposition.monitor.interval_seconds=5\ntrade.account_count=1\ntrade.account1.name=primary\ntrade.account1.url=https://example.test\ntrade.account1.uid=1\nmonitor.symbols.BTCUSDT.deep_inst=BTCUSDT\nmonitor.symbols.BTCUSDT.trade_inst=BTC-USDT\nmonitor.symbols.BTCUSDT.threshold=0.01\n"

	// 三份部署配置各自带 argus.instance.id，导入时按此归属实例。
	for filename, wantInstance := range map[string]string{
		"application.properties":   "argus-single-1",
		"application_1.properties": "argus-single-roc",
		"application_2.properties": "argus-single-ives",
	} {
		directory := t.TempDir()
		propertiesPath := filepath.Join(directory, filename)
		sessionPath := filepath.Join(directory, "session.json")
		if err := os.WriteFile(propertiesPath, []byte(baseProperties+"argus.instance.id="+wantInstance+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(sessionPath, []byte(`{"accounts":{}}`), 0o600); err != nil {
			t.Fatal(err)
		}
		request, summary, err := LoadMainConfigImport(propertiesPath, sessionPath)
		if err != nil {
			t.Fatalf("%s: %v", filename, err)
		}
		if summary.InstanceKey != wantInstance || request.InstanceKey != wantInstance {
			t.Fatalf("%s: instance key = %q/%q, want %q", filename, summary.InstanceKey, request.InstanceKey, wantInstance)
		}
		if summary.SourceFile != filename {
			t.Fatalf("%s: source file = %q", filename, summary.SourceFile)
		}
	}
}

func TestLoadMainConfigImportStillRejectsForeignPropertiesNames(t *testing.T) {
	directory := t.TempDir()
	sessionPath := filepath.Join(directory, "session.json")
	if err := os.WriteFile(sessionPath, []byte(`{"accounts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"other.properties", "application.txt", "app_application.properties"} {
		if _, _, err := LoadMainConfigImport(filepath.Join(directory, filename), sessionPath); err == nil {
			t.Fatalf("%s was accepted as an instance properties file", filename)
		}
	}
}

// TestResolveLoginTypeIgnoresStaleSessionMode 复现 2026-09-08 的线上故障。
//
// 部署用的 properties 从不写 login_type（运维靠 trade.accountN.cookie/token
// 维护静态凭证），而手上的 session.json 是几个月前的、里面写着
// loginType=password。旧实现 firstNonBlank(properties, session.LoginType,
// "config") 会取到 password，导入后账户被判成密码登录，argus_single 启动时
// 去调未部署的 pl-instance，盘口信号开仓全部失败。
func TestResolveLoginTypeIgnoresStaleSessionMode(t *testing.T) {
	properties := map[string]string{
		"trade.account1.cookie": "fresh-cookie",
		"trade.account1.token":  "fresh-token",
	}
	stale := importSession{LoginType: "password", Cookie: "old-cookie", Token: "old-token"}
	if got := resolveLoginType(properties, stale, "trade.account1."); got != "config" {
		t.Fatalf("有静态 cookie/token 时应判为 config，得到 %q（session.json 的 password 不该胜出）", got)
	}
}

func TestResolveLoginTypeHonoursExplicitAndInfersPassword(t *testing.T) {
	// properties 显式声明优先级最高
	explicit := map[string]string{
		"trade.account1.login_type": "password",
		"trade.account1.cookie":     "c",
		"trade.account1.token":      "t",
	}
	if got := resolveLoginType(explicit, importSession{}, "trade.account1."); got != "password" {
		t.Fatalf("properties 显式声明应优先，得到 %q", got)
	}
	// 没有静态凭证、只有账号密码时才推断为 password
	onlyLogin := map[string]string{
		"trade.account1.username": "u",
		"trade.account1.password": "p",
	}
	if got := resolveLoginType(onlyLogin, importSession{}, "trade.account1."); got != "password" {
		t.Fatalf("只有账号密码时应判为 password，得到 %q", got)
	}
	// 什么都没有时兜底 config，而不是去调 pl-instance
	if got := resolveLoginType(map[string]string{}, importSession{}, "trade.account1."); got != "config" {
		t.Fatalf("无凭证时应兜底 config，得到 %q", got)
	}
}

// TestImportRuntimeSessionPrefersPropertiesCredentials 守另一半故障：
// 运维每周换的是 properties 里的 cookie/token，session.json 只在无头登录成功后
// 才刷新。旧实现只读 session.json，等于把刚换的新凭证丢掉、把过期的写进库。
func TestImportRuntimeSessionPrefersPropertiesCredentials(t *testing.T) {
	properties := map[string]string{
		"trade.account1.cookie":        "fresh-cookie",
		"trade.account1.token":         "fresh-token",
		"trade.account1.sentryRelease": "fresh-release",
	}
	stale := importSession{
		Cookie: "stale-cookie", Token: "stale-token", OToken: "stale-otoken",
		SentryRelease: "stale-release",
	}
	got := importRuntimeSession(properties, stale, "trade.account1.", 1)
	if got.Cookie != "fresh-cookie" || got.Token != "fresh-token" {
		t.Fatalf("properties 的凭证应优先，得到 cookie=%q token=%q", got.Cookie, got.Token)
	}
	// properties 换了 token 就必须清掉 session.json 的旧 otoken——runtimeAccount
	// 先取 OToken 再回退 Token，留着旧 otoken 等于新 token 永远不生效。
	if got.OToken != "" {
		t.Fatalf("properties 提供 token 时旧 otoken 必须清空，得到 %q", got.OToken)
	}
	if got.SentryRelease != "fresh-release" {
		t.Fatalf("sentryRelease 也应优先取 properties，得到 %q", got.SentryRelease)
	}
	if got.Valid != 1 {
		t.Fatalf("cookie+token 齐备时 valid 应为 1，得到 %d", got.Valid)
	}
}

// TestImportRuntimeSessionFallsBackToSessionFile properties 没写时仍要用
// session.json，否则无头登录刷出来的凭证会丢。
func TestImportRuntimeSessionFallsBackToSessionFile(t *testing.T) {
	session := importSession{Cookie: "c-from-json", Token: "t-from-json", OToken: "o-from-json"}
	got := importRuntimeSession(map[string]string{}, session, "trade.account1.", 1)
	if got.Cookie != "c-from-json" || got.Token != "t-from-json" || got.OToken != "o-from-json" {
		t.Fatalf("properties 缺省时应回退 session.json，得到 %+v", got)
	}
}
