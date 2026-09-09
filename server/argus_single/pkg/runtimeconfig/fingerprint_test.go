package runtimeconfig

import (
	"reflect"
	"testing"

	"argus_single/pkg/monitor"
	"argus_single/pkg/trade"
)

// baseRuntime 是一份最小但字段齐全的运行配置，用于逐字段改动后比对指纹。
func baseRuntime() RuntimeConfig {
	return RuntimeConfig{
		Version:  7,
		Checksum: "published-checksum",
		Trade: &trade.TradingSystemConfig{
			Trade: trade.TradeConfig{OrderSize: 1},
			Accounts: []trade.AccountConfig{{
				Name: "账户A", URL: "https://api.example.com", UID: "1",
				APIKey: "ak", SecretKey: "sk", Passphrase: "pp",
				Cookie: "cookie-v1", Token: "token-v1", Index: 0,
			}},
		},
		Symbols:      map[string]monitor.SymbolConfig{"BTCUSDT": {DeepInst: "BTC-USDT-SWAP"}},
		ServerPort:   8855,
		RequestPath:  "/",
		LogDir:       "./logs/",
		Notification: notificationConfig{enabled: true, token: "tg-token", chatID: "-100"},
		Tuning:       RuntimeTuning{MonitorIntervalSecond: 5, ContractFace: 0.001},
		Overrides:    trade.ParamOverrides{Global: map[string]float64{"g": 1}, Accounts: map[int]map[string]float64{0: {"budget_pct": 13.3}}},
	}
}

func fp(t *testing.T, c RuntimeConfig) string {
	t.Helper()
	v, err := configFingerprint(c)
	if err != nil {
		t.Fatalf("configFingerprint: %v", err)
	}
	return v
}

// TestFingerprintIsStable 同样内容必须算出同样指纹——否则每轮比对都判定"变了"，
// 会让进程每 60 秒把自己热替换一遍。map 字段（Symbols/Overrides）是主要风险点，
// 这里依赖 encoding/json 对 map 键排序的保证。
func TestFingerprintIsStable(t *testing.T) {
	for i := 0; i < 20; i++ {
		if a, b := fp(t, baseRuntime()), fp(t, baseRuntime()); a != b {
			t.Fatalf("同样内容算出不同指纹: %s vs %s", a, b)
		}
	}
}

// TestFingerprintIgnoresVersionAndChecksum 版本号与发布校验和不参与指纹。
//
// 这是本次改造的核心取舍：运维直接改库轮换 cookie/token 不会动这两个值，
// 若它们参与指纹反而会掩盖真实变更的判定语义。
func TestFingerprintIgnoresVersionAndChecksum(t *testing.T) {
	base := fp(t, baseRuntime())
	c := baseRuntime()
	c.Version = 999
	c.Checksum = "another-checksum"
	c.Fingerprint = "stale"
	if got := fp(t, c); got != base {
		t.Fatalf("Version/Checksum 不应影响指纹，base=%s got=%s", base, got)
	}
}

// TestFingerprintDetectsEveryBehaviouralChange 每一个影响行为的字段被改动后，
// 指纹都必须变化。漏掉任何一项都意味着那类配置改动永远不会被检测到。
func TestFingerprintDetectsEveryBehaviouralChange(t *testing.T) {
	base := fp(t, baseRuntime())
	cases := map[string]func(*RuntimeConfig){
		"账户 cookie（每周轮换，最关键）": func(c *RuntimeConfig) { c.Trade.Accounts[0].Cookie = "cookie-v2" },
		"账户 token":              func(c *RuntimeConfig) { c.Trade.Accounts[0].Token = "token-v2" },
		"账户 api_key":            func(c *RuntimeConfig) { c.Trade.Accounts[0].APIKey = "ak2" },
		"账户 secret_key":         func(c *RuntimeConfig) { c.Trade.Accounts[0].SecretKey = "sk2" },
		"账户 passphrase":         func(c *RuntimeConfig) { c.Trade.Accounts[0].Passphrase = "pp2" },
		"新增账户":                  func(c *RuntimeConfig) { c.Trade.Accounts = append(c.Trade.Accounts, trade.AccountConfig{Name: "账户B", Index: 1}) },
		"全局下单张数":                func(c *RuntimeConfig) { c.Trade.Trade.OrderSize = 2 },
		"监控币种":                  func(c *RuntimeConfig) { c.Symbols["ETHUSDT"] = monitor.SymbolConfig{} },
		"币种参数":                  func(c *RuntimeConfig) { c.Symbols["BTCUSDT"] = monitor.SymbolConfig{DeepInst: "OTHER"} },
		"server.port":           func(c *RuntimeConfig) { c.ServerPort = 9999 },
		"request.path":          func(c *RuntimeConfig) { c.RequestPath = "/api" },
		"log.dir":               func(c *RuntimeConfig) { c.LogDir = "/var/log/" },
		"Telegram 开关（未导出字段）":   func(c *RuntimeConfig) { c.Notification.enabled = false },
		"Telegram token（未导出字段）": func(c *RuntimeConfig) { c.Notification.token = "tg-token-2" },
		"Telegram chatID（未导出字段）": func(c *RuntimeConfig) { c.Notification.chatID = "-200" },
		"调优参数":                  func(c *RuntimeConfig) { c.Tuning.MonitorIntervalSecond = 10 },
		"全局覆盖层":                 func(c *RuntimeConfig) { c.Overrides.Global["g"] = 2 },
		"账户级覆盖层":                func(c *RuntimeConfig) { c.Overrides.Accounts[0]["budget_pct"] = 20 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := baseRuntime()
			mutate(&c)
			if got := fp(t, c); got == base {
				t.Fatalf("改动 %q 后指纹未变化（%s）——这类配置改动将永远检测不到", name, got)
			}
		})
	}
}

// TestFingerprintViewCoversRuntimeConfig 字段数守卫。
//
// configFingerprintView 是手写的镜像结构，新增 RuntimeConfig 字段时很容易忘记
// 纳入指纹，而那种遗漏是**静默**的：配置改了但检测不到，没有任何报错。
// 这个测试在字段数变化时失败，强制作者显式决定"新字段要不要参与指纹"。
func TestFingerprintViewCoversRuntimeConfig(t *testing.T) {
	const (
		runtimeFields = 11 // Version, Checksum, Fingerprint, Trade, Symbols, ServerPort, RequestPath, LogDir, Notification, Tuning, Overrides
		viewFields    = 10 // configFingerprintView 的字段数（Notification 摊平成 3 个）
	)
	got := reflect.TypeOf(RuntimeConfig{}).NumField()
	if got != runtimeFields {
		t.Fatalf("RuntimeConfig 字段数变为 %d（期望 %d）。新增字段请显式决定是否纳入 configFingerprintView："+
			"漏掉会导致该类配置改动永远无法被检测到，且不会有任何报错。确认后同步更新本测试的常量。", got, runtimeFields)
	}
	if v := reflect.TypeOf(configFingerprintView{}).NumField(); v != viewFields {
		t.Fatalf("configFingerprintView 字段数变为 %d（期望 %d），请同步更新本测试常量", v, viewFields)
	}
}
