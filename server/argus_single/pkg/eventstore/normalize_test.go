package eventstore

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNormalizeInstrument(t *testing.T) {
	cases := map[string]string{
		"BTC-USDT-SWAP": "BTCUSDT", // 持仓侧口径
		"BTCUSDT":       "BTCUSDT", // 信号侧口径
		"btc-usdt-swap": "BTCUSDT",
		" BTCUSDT ":     "BTCUSDT",
		"ETH-USDT-SWAP": "ETHUSDT",
		"":              "",
	}
	for in, want := range cases {
		if got := NormalizeInstrument(in); got != want {
			t.Fatalf("NormalizeInstrument(%q)=%q, want %q", in, got, want)
		}
	}
}

// 归一化必须与 pkg/trade 的 instrumentBase（归一化到 BTC）不同口径，
// 两者混用会让持仓匹配或前端展示之一必然出错。
func TestNormalizeInstrumentKeepsQuoteCurrency(t *testing.T) {
	if got := NormalizeInstrument("BTC-USDT-SWAP"); got == "BTC" {
		t.Fatalf("不得剥掉 USDT 后缀，那是持仓匹配的口径")
	}
}

func TestNormalizeDSNForcesTimezoneAndCharset(t *testing.T) {
	// 现网 properties 的形态：key 后带空格、charset=utf8、值前有空格。
	raw := " user:p@ss?word@tcp(db.example.com:3306)/lightning_trade?charset=utf8&parseTime=True&loc=Local"
	got, err := NormalizeDSN(raw)
	if err != nil {
		t.Fatalf("NormalizeDSN error: %v", err)
	}
	const wantBase = "user:p@ss?word@tcp(db.example.com:3306)/lightning_trade?"
	if !strings.HasPrefix(got, wantBase) {
		t.Fatalf("库名之前的部分被改写了: %s", got)
	}
	// 参数段的起点只能按"最后一个 / 之后的第一个 ?"定位——密码里那个 ? 不是分隔符。
	values, err := url.ParseQuery(strings.TrimPrefix(got, wantBase))
	if err != nil {
		t.Fatalf("parse normalized query: %v", err)
	}
	for key, want := range map[string]string{
		"loc":          "Local",
		"parseTime":    "true",
		"charset":      "utf8mb4",
		"timeout":      "5s",
		"readTimeout":  "15s",
		"writeTimeout": "15s",
	} {
		if got := values.Get(key); got != want {
			t.Fatalf("参数 %s=%q, want %q（全量: %s）", key, got, want, values.Encode())
		}
	}
}

func TestNormalizeDSNAddsParametersWhenAbsent(t *testing.T) {
	got, err := NormalizeDSN("user:pass@tcp(127.0.0.1:3306)/lightning_trade")
	if err != nil {
		t.Fatalf("NormalizeDSN error: %v", err)
	}
	if !strings.Contains(got, "loc=Local") || !strings.Contains(got, "parseTime=true") {
		t.Fatalf("缺少必须参数: %s", got)
	}
}

func TestNormalizeDSNKeepsExplicitTimeouts(t *testing.T) {
	got, err := NormalizeDSN("u:p@tcp(h:3306)/d?timeout=30s")
	if err != nil {
		t.Fatalf("NormalizeDSN error: %v", err)
	}
	if !strings.Contains(got, "timeout=30s") {
		t.Fatalf("已显式配置的超时被覆盖: %s", got)
	}
}

func TestNormalizeDSNEmpty(t *testing.T) {
	if _, err := NormalizeDSN("   "); err != ErrDSNEmpty {
		t.Fatalf("空 sqlconn 应返回 ErrDSNEmpty，实际 %v", err)
	}
	if !IsDisabled(ErrDSNEmpty) {
		t.Fatalf("IsDisabled 应识别 ErrDSNEmpty")
	}
}

func TestParseTsUsesLocalSecondPrecision(t *testing.T) {
	got, err := ParseTs("2026-08-21 14:03:07")
	if err != nil {
		t.Fatalf("ParseTs error: %v", err)
	}
	want := time.Date(2026, 8, 21, 14, 3, 7, 0, time.Local)
	if !got.Equal(want) {
		t.Fatalf("ParseTs=%v, want %v", got, want)
	}
	if got.Nanosecond() != 0 {
		t.Fatalf("ts 精度必须止于秒")
	}
	if _, err := ParseTs("not-a-time"); err == nil {
		t.Fatalf("坏时间戳必须报错，由调用方计数丢弃")
	}
}
