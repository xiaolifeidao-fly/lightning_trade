package web

import (
	"encoding/json"
	"math"
	"math/rand"
	"testing"
	"time"

	"common/cookies"
)

// TestGenerateBrowserSignals_StablePerLoginID 同一账户多次调用结果完全一致
func TestGenerateBrowserSignals_StablePerLoginID(t *testing.T) {
	a := GenerateBrowserSignals("9558450")
	b := GenerateBrowserSignals("9558450")
	if a != b {
		t.Errorf("同一 LoginID 应生成相同的浏览器指纹，got %+v vs %+v", a, b)
	}
	if a.Webdriver || a.UAHeadless || a.PermissionsQueryAbnormal || a.IframeAccessAbnormal {
		t.Errorf("正常浏览器指纹不应出现 true 字段: %+v", a)
	}
	if a.PluginsLength < 3 || a.PluginsLength > 6 {
		t.Errorf("PluginsLength 超出合理范围: %d", a.PluginsLength)
	}
	if a.LanguagesLength < 1 || a.LanguagesLength > 2 {
		t.Errorf("LanguagesLength 超出合理范围: %d", a.LanguagesLength)
	}
}

// TestGenerateBrowserSignals_DifferentLoginIDDiffer 不同账户指纹应有分布差异
func TestGenerateBrowserSignals_DifferentLoginIDDiffer(t *testing.T) {
	seen := map[BrowserSignals]int{}
	for i := 1000000; i < 1000020; i++ {
		seen[GenerateBrowserSignals(itoa(i))]++
	}
	if len(seen) < 2 {
		t.Errorf("20 个 LoginID 应分布到至少 2 种浏览器指纹组合，got 1: %+v", seen)
	}
}

// TestGenerateViewportSignals_StablePerLoginID 视口指纹也应稳定
func TestGenerateViewportSignals_StablePerLoginID(t *testing.T) {
	a := GenerateViewportSignals("9558450")
	b := GenerateViewportSignals("9558450")
	if a != b {
		t.Errorf("同一 LoginID 应生成相同的视口指纹，got %+v vs %+v", a, b)
	}
	if a.ScreenWidth <= 0 || a.ScreenHeight <= 0 || a.ViewportWidth <= 0 || a.ViewportHeight <= 0 {
		t.Errorf("视口指纹包含非法零值: %+v", a)
	}
	if a.ViewportWidth > a.ScreenWidth || a.ViewportHeight > a.ScreenHeight {
		t.Errorf("视口尺寸不应大于屏幕尺寸: %+v", a)
	}
}

// TestGenerateBehaviorSnapshot_GeometricConsistency 行为字段必须自洽
func TestGenerateBehaviorSnapshot_GeometricConsistency(t *testing.T) {
	for i := 0; i < 200; i++ {
		// 用各种窗口长度跑：1s / 10s / 1min / 5min
		for _, w := range []float64{1000, 10000, 60000, 300000} {
			s := GenerateBehaviorSnapshot("acc-"+itoa(i), w)

			if s.MousePathPointCount < 0 {
				t.Fatalf("MousePathPointCount 不能为负: %+v", s)
			}
			if s.MousePathStraightnessScore < 0 || s.MousePathStraightnessScore > 1 {
				t.Fatalf("Straightness 必须在 [0,1]: %+v", s)
			}

			// < 5 个点：speed_std 与 direction_change 必须为 0
			if s.MousePathPointCount < 5 {
				if s.MousePathSpeedStd != 0 || s.MousePathDirectionChangeCount != 0 {
					t.Fatalf("点数 %d 时 speed_std/direction_change 必须为 0: %+v",
						s.MousePathPointCount, s)
				}
			}
			// >= 5 个点：方向变化数不能超过点数
			if s.MousePathPointCount >= 5 {
				if s.MousePathDirectionChangeCount > s.MousePathPointCount {
					t.Fatalf("方向变化数不应超过点数: %+v", s)
				}
				if s.MousePathSpeedStd <= 0 {
					t.Fatalf("点数 >= 5 时 speed_std 应 > 0: %+v", s)
				}
				if s.MousePathStraightnessScore > 0.5 {
					t.Fatalf("点数 >= 5 时 straightness 不应 > 0.5: %+v", s)
				}
			}
			// 3-4 个点：speed_std/direction_change=0，straightness 偏直
			if s.MousePathPointCount >= 3 && s.MousePathPointCount < 5 {
				if s.MousePathSpeedStd != 0 || s.MousePathDirectionChangeCount != 0 {
					t.Fatalf("3-4 点时 speed/direction 应为 0: %+v", s)
				}
				if s.MousePathStraightnessScore < 0.5 || s.MousePathStraightnessScore > 1 {
					t.Fatalf("3-4 点时 straightness 应在 [0.5, 1]: %+v", s)
				}
			}

			// 点击数 < 3 时 click_interval_std 必须为 0
			if s.ClickCount < 3 && s.ClickIntervalStdMs != 0 {
				t.Fatalf("点击数 %d 时 interval_std 必须为 0: %+v", s.ClickCount, s)
			}
			// 无点击时首次点击延迟必须为 0
			if s.ClickCount == 0 && s.FirstClickLatencyMs != 0 {
				t.Fatalf("ClickCount=0 时 FirstClickLatencyMs 必须为 0: %+v", s)
			}
			// 行为窗口必须 <= 5 分钟
			if s.RiskBehaviorWindowMs > maxBehaviorWindowMs+1 {
				t.Fatalf("窗口超过 5 分钟上限: %+v", s)
			}
			if s.RiskBehaviorSampleIntervalMs != defaultSampleIntervalMs {
				t.Fatalf("默认采样间隔应为 %d, got %d", defaultSampleIntervalMs, s.RiskBehaviorSampleIntervalMs)
			}
		}
	}
}

// TestGenerateBehaviorSnapshot_RandomBetweenCalls 同一 LoginID 多次调用应有差异（行为字段是随机的）
func TestGenerateBehaviorSnapshot_RandomBetweenCalls(t *testing.T) {
	results := map[string]int{}
	for i := 0; i < 30; i++ {
		s := GenerateBehaviorSnapshot("9558450", 60000)
		key := jsonString(s)
		results[key]++
		// 调用之间稍微 sleep 一下，避免 nano 时间戳完全相同
		time.Sleep(time.Millisecond)
	}
	if len(results) < 5 {
		t.Errorf("30 次调用应至少产生 5 种不同结果，got %d: %v", len(results), results)
	}
}

// TestBehaviorTracker_WindowSemantics 验证窗口跟踪与 prod 算法一致：
//
//	windowStart = max(now-5min, pageEnterTime, lastTradeAt)
//	windowMs = now - windowStart
func TestBehaviorTracker_WindowSemantics(t *testing.T) {
	tracker := &behaviorTracker{
		pageEnter: map[string]time.Time{},
		lastTrade: map[string]time.Time{},
	}
	r := rand.New(rand.NewSource(1))

	t0 := time.Date(2026, 5, 15, 23, 0, 0, 0, time.UTC)

	// 首次：windowStart = pageEnterTime（自动伪造为 t0 之前 5s~3min 的时刻）
	// 所以 windowMs ∈ [5s, 3min]
	s1 := tracker.consume("uid-1", t0, r)
	if s1.WindowMs < 5000 || s1.WindowMs > 3*60*1000+1000 {
		t.Errorf("首次窗口应在 [5s, 3min]，got %.2f", s1.WindowMs)
	}
	if !s1.IsFirstAfterPV {
		t.Errorf("首次调用应标记 IsFirstAfterPV=true")
	}
	// PageElapsedMs 应等于 WindowMs（首次时两者相同，因为 windowStart 就是 pageEnter）
	if math.Abs(s1.PageElapsedMs-s1.WindowMs) > 5 {
		t.Errorf("首次 PageElapsedMs 应≈WindowMs，got page=%.2f window=%.2f", s1.PageElapsedMs, s1.WindowMs)
	}

	// 第二次：windowStart 被 lastTradeAt(t0) 抬起，windowMs ≈ 15s
	t1 := t0.Add(15 * time.Second)
	s2 := tracker.consume("uid-1", t1, r)
	if s2.WindowMs < 14990 || s2.WindowMs > 15010 {
		t.Errorf("第二次窗口应≈15s，got %.2f", s2.WindowMs)
	}
	if s2.IsFirstAfterPV {
		t.Errorf("第二次不应再标记为首次")
	}
	// PageElapsed = 第一次伪造的 offset + 15s，应该明显 > WindowMs
	if s2.PageElapsedMs <= s2.WindowMs {
		t.Errorf("第二次 PageElapsedMs (%.2f) 应 > WindowMs (%.2f)", s2.PageElapsedMs, s2.WindowMs)
	}

	// 超过 5 分钟应被截断
	t2 := t1.Add(10 * time.Minute)
	s3 := tracker.consume("uid-1", t2, r)
	if s3.WindowMs > maxBehaviorWindowMs+5 {
		t.Errorf("窗口应被截断到 5 分钟，got %.2f", s3.WindowMs)
	}
}

// TestBuildTradeRiskData_AllNewFieldsPresent 端到端验证：payload 必须包含 16 个新字段 + production_version=国际版
func TestBuildTradeRiskData_AllNewFieldsPresent(t *testing.T) {
	cookieData := &cookies.CookieData{}
	cookieData.SensorsData.DistinctID = "9558450"
	cookieData.SensorsData.Identities = &cookies.IdentitiesData{
		IdentityCookieID: "test-cookie-id",
		IdentityLoginID:  "9558450",
		IdentityH5ID:     "pc-test",
	}

	req := &TradeRiskRequest{
		LoginID:               "9558450",
		InstrumentIDName:      "BTCUSDT",
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "净仓模式",
		TradeType:             "买入",
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             125,
		LeverageK:             125,
		TradePrice:            79251.9,
		TradeVolume:           1,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           "全仓合仓",
		IsReduceOnly:          false,
		PriceTrend:            "down",
	}

	data := buildTradeRiskData(cookieData, req)
	props := data.Properties

	// 16 个新字段 + production_version
	required := []string{
		"click_count", "click_interval_std_ms", "first_click_latency_ms",
		"keystroke_interval_std_ms",
		"mouse_path_speed_std", "mouse_path_point_count",
		"mouse_path_direction_change_count", "mouse_path_straightness_score",
		"risk_behavior_window_ms", "risk_behavior_sample_interval_ms",
		"browser_webdriver", "browser_plugins_length", "browser_languages_length",
		"browser_ua_headless", "browser_permissions_query_abnormal", "browser_iframe_access_abnormal",
		"production_version",
	}
	for _, key := range required {
		if _, ok := props[key]; !ok {
			t.Errorf("payload 缺失必填字段: %s", key)
		}
	}

	if v := props["production_version"]; v != "国际版" {
		t.Errorf("production_version 默认应为 '国际版'，got %v", v)
	}
	if v := props["browser_webdriver"]; v != false {
		t.Errorf("browser_webdriver 默认应为 false，got %v", v)
	}
	// 市价单时 de_trade_price 是字符串 "市价"
	if v, ok := props["de_trade_price"].(string); !ok || v != "市价" {
		t.Errorf("市价单 de_trade_price 应为 '市价'，got %v", props["de_trade_price"])
	}
	// $title 应包含 ▼ 和千分位价格
	title, _ := props["$title"].(string)
	if title == "" || !contains(title, "▼") || !contains(title, "79,251.9") {
		t.Errorf("$title 应包含 ▼ 与千分位价格，got %q", title)
	}
}

// TestBuildTradeRiskData_PriceTrendUp PriceTrend=up 时应是 ▲
func TestBuildTradeRiskData_PriceTrendUp(t *testing.T) {
	cookieData := &cookies.CookieData{}
	cookieData.SensorsData.Identities = &cookies.IdentitiesData{IdentityLoginID: "x"}

	req := &TradeRiskRequest{
		LoginID:          "x",
		InstrumentIDName: "BTCUSDT",
		TradeMode:        "市价",
		TradePrice:       1234.5,
		PriceTrend:       "up",
	}
	data := buildTradeRiskData(cookieData, req)
	title, _ := data.Properties["$title"].(string)
	if !contains(title, "▲") || !contains(title, "1,234.5") {
		t.Errorf("PriceTrend=up 时 $title 应是 ▲ 1,234.5，got %q", title)
	}
}

// TestFormatThousands1 千分位格式化
func TestFormatThousands1(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{79251.9, "79,251.9"},
		{1234.5, "1,234.5"},
		{999.0, "999.0"},
		{0, "0.0"},
		{1000000.0, "1,000,000.0"},
		{-1234.5, "-1,234.5"},
	}
	for _, c := range cases {
		got := formatThousands1(c.in)
		if got != c.want {
			t.Errorf("formatThousands1(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ------------- helpers -------------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
