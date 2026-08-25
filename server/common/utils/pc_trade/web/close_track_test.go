package web

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"common/cookies"
)

func trackTestCookie(t *testing.T) *cookies.CookieData {
	t.Helper()
	raw := "sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229533715%22%2C%22first_id%22%3A%22" +
		"19bffb198426f-0d8f6a8b33c3098-1d525631-1930176-19bffb198431d0b%22%2C%22props%22%3A%7B%7D%2C" +
		"%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTliZmZiMTk4NDI2Zi0wZDhmNmE4YjMzYzMwOTgtMWQ1MjU2MzEtMTkzMDE3Ni0xOWJmZmIxOTg0MzFkMGIiLCIkaWRlbnRpdHlfbG9naW5faWQiOiI5NTMzNzE1IiwiaWRlbnRpdHlfaDVfaWQiOiJwYy0yNzg1YjFlMjgxODVkMjQzNDdkM2RjZTY0MTY2MDFkZiJ9%22%2C" +
		"%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%229533715%22%7D%7D"
	cd, err := cookies.ParseCookieString(raw)
	if err != nil {
		t.Fatalf("解析测试 cookie 失败: %v", err)
	}
	return cd
}

// 平仓埋点 payload 结构（2026-07-31 浏览器抓包实测模板：网页「市价全平」）。
// 我们的四条平仓路径此前完全不发埋点，而网页每次平仓都发两条事件；
// "有平仓订单却无对应埋点" 本身即异常模式。字段严格照抓包对齐。
func TestBuildClosePosTrackData(t *testing.T) {
	cd := trackTestCookie(t)
	req := &ClosePosTrackRequest{
		PositionID: "1001124499792827",
		TradePair:  "BTCUSDT",
		TradeQuote: 63943.5,
		DealSize:   1,
	}

	t.Run("动作事件", func(t *testing.T) {
		d := buildClosePosTrackData(cd, req, "")
		p := d.Properties
		if d.Event != "TradeCloseAllPosition" {
			t.Fatalf("event = %q", d.Event)
		}
		checks := map[string]any{
			"button_name":       "市价全平二次确认",
			"de_order_id":       "1001124499792827",
			"de_tradepair_name": "BTCUSDT",
			"trade_quote":       63943.5,
			"de_deal_size":      1,
			"product_group":     "SwapU",
			"message_count":     1,
		}
		for k, want := range checks {
			if got, ok := p[k]; !ok {
				t.Fatalf("缺字段 %s", k)
			} else if !equalLoose(got, want) {
				t.Fatalf("%s = %v(%T), want %v", k, got, got, want)
			}
		}
		// local_info 是 JSON 字符串（不是对象），内含 oid 与 new_price
		li, _ := p["local_info"].(string)
		var arr []map[string]any
		if err := json.Unmarshal([]byte(li), &arr); err != nil {
			t.Fatalf("local_info 应为 JSON 字符串: %q err=%v", li, err)
		}
		if len(arr) != 1 || arr[0]["oid"] != "1001124499792827" || arr[0]["instrument_id_name"] != "BTCUSDT" {
			t.Fatalf("local_info 内容: %v", arr)
		}
		// 动作事件不带 mark
		if _, ok := p["mark"]; ok {
			t.Fatalf("动作事件不应有 mark")
		}
		// 网页改版后的 URL 形态（旧代码里的 /turbo/zh/swap/ 已过时）
		if u, _ := p["$url"].(string); u != "https://www.deepcoin.com/swap/zh/BTCUSDT" {
			t.Fatalf("$url = %q", u)
		}
		if ttl, _ := p["$title"].(string); !strings.Contains(ttl, "63,943.5") || !strings.Contains(ttl, "BTCUSDT - Deepcoin") {
			t.Fatalf("$title = %q", ttl)
		}
	})

	t.Run("结果事件带 mark 且无 local_info", func(t *testing.T) {
		mark := `{"code":0,"msg":"OK","data":{"errorList":[],"spend":2}}`
		d := buildClosePosTrackData(cd, req, mark)
		if d.Event != "TradeCloseAllPositionResult" {
			t.Fatalf("event = %q", d.Event)
		}
		if got, _ := d.Properties["mark"].(string); got != mark {
			t.Fatalf("mark = %q", got)
		}
		// 抓包实测：结果事件不带 local_info
		if _, ok := d.Properties["local_info"]; ok {
			t.Fatalf("结果事件不应有 local_info")
		}
	})

	t.Run("身份字段取自 cookie", func(t *testing.T) {
		d := buildClosePosTrackData(cd, req, "")
		if d.DistinctID != "9533715" || d.Type != "track" {
			t.Fatalf("身份: distinct=%q type=%q", d.DistinctID, d.Type)
		}
		if d.Identities["$identity_login_id"] != "9533715" {
			t.Fatalf("identities: %v", d.Identities)
		}
	})
}

// tradePairName：持仓 instId(BTC-USDT-SWAP) → 埋点口径(BTCUSDT)。
// 注意与 instrumentBase 不同——后者会连 USDT 一起去掉，得到 "BTC"。
func TestTradePairName(t *testing.T) {
	cases := map[string]string{
		"BTC-USDT-SWAP":   "BTCUSDT",
		"BTCUSDT":         "BTCUSDT", // 幂等
		"eth-usdt-swap":   "ETHUSDT",
		" BTC-USDT-SWAP ": "BTCUSDT",
	}
	for in, want := range cases {
		if got := TradePairName(in); got != want {
			t.Fatalf("TradePairName(%q) = %q, want %q", in, got, want)
		}
	}
}

func equalLoose(got, want any) bool {
	switch w := want.(type) {
	case int:
		switch g := got.(type) {
		case int:
			return g == w
		case float64:
			return g == float64(w)
		}
	case float64:
		if g, ok := got.(float64); ok {
			return g == w
		}
	case string:
		if g, ok := got.(string); ok {
			return g == w
		}
	}
	return false
}

// 埋点失败通知钩子：web 是底层库，不直接依赖 TG——由上层注入通知方式。
// 动机：埋点失败此前只写 logrus，淹没在日志里无人察觉（域名下线导致
// 上报静默失败长达数周就是这么发生的），必须能主动推到 TG。
func TestTrackFailureHandler(t *testing.T) {
	t.Cleanup(func() { SetTrackFailureHandler(nil) })

	type call struct {
		stage string
		err   error
	}
	var got []call
	SetTrackFailureHandler(func(stage string, err error) {
		got = append(got, call{stage, err})
	})

	notifyTrackFailure("平仓埋点/动作", errNoRoute)
	if len(got) != 1 || got[0].stage != "平仓埋点/动作" {
		t.Fatalf("钩子未按预期收到通知: %+v", got)
	}
	if got[0].err != errNoRoute {
		t.Fatalf("错误须原样透传: %v", got[0].err)
	}

	// 未注册钩子时不得 panic（common 被多个二进制复用，未必都注入）
	SetTrackFailureHandler(nil)
	notifyTrackFailure("x", errNoRoute)

	// nil error 不触发
	SetTrackFailureHandler(func(stage string, err error) { got = append(got, call{stage, err}) })
	before := len(got)
	notifyTrackFailure("y", nil)
	if len(got) != before {
		t.Fatalf("nil error 不应触发通知")
	}
}

var errNoRoute = errors.New("dial tcp: no such host")

