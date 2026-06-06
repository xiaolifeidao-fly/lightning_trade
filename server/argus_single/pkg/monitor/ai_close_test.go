package monitor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"argus_single/pkg/trade"
	"common/utils"

	"github.com/shopspring/decimal"
)

// ─── RenderAIPrompt ───────────────────────────────────────────────────────────

func TestRenderAIPrompt(t *testing.T) {
	template := "你是{role}，规则:{rule}，输出:{response}，未知:{missing}"
	got := RenderAIPrompt(template, map[string]any{
		"role":     "风控员",
		"rule":     "保守",
		"response": "JSON",
	})
	want := "你是风控员，规则:保守，输出:JSON，未知:{missing}"
	if got != want {
		t.Fatalf("RenderAIPrompt() = %q, want %q", got, want)
	}
}

func TestRenderAIPrompt_EmptyVars(t *testing.T) {
	tpl := "hello {name} world"
	got := RenderAIPrompt(tpl, map[string]any{})
	if got != "hello {name} world" {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestRenderAIPrompt_AllReplaced(t *testing.T) {
	tpl := "{a}-{b}-{c}"
	got := RenderAIPrompt(tpl, map[string]any{"a": "1", "b": "2", "c": "3"})
	if got != "1-2-3" {
		t.Fatalf("unexpected: %q", got)
	}
}

// ─── extractJSONObject ────────────────────────────────────────────────────────

func TestExtractJSONObject_PlainJSON(t *testing.T) {
	input := `{"should_close":false}`
	got := extractJSONObject(input)
	if got != input {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestExtractJSONObject_EmbeddedInText(t *testing.T) {
	input := `Here is the answer: {"should_close":true,"reason":"test"} done.`
	got := extractJSONObject(input)
	want := `{"should_close":true,"reason":"test"}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractJSONObject_WithMarkdown(t *testing.T) {
	input := "```json\n{\"a\":1}\n```"
	got := extractJSONObject(input)
	want := `{"a":1}`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractJSONObject_NoJSON(t *testing.T) {
	input := "no json here"
	got := extractJSONObject(input)
	if got != input {
		t.Fatalf("expected original string, got %q", got)
	}
}

// ─── parseDecimalPercent ──────────────────────────────────────────────────────

func mustRaw(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

func TestParseDecimalPercent_Number(t *testing.T) {
	got := parseDecimalPercent(mustRaw(72.5))
	if !got.Equal(decimal.NewFromFloat(72.5)) {
		t.Fatalf("got %s, want 72.5", got)
	}
}

func TestParseDecimalPercent_StringNumber(t *testing.T) {
	got := parseDecimalPercent(mustRaw("65"))
	if got.StringFixed(0) != "65" {
		t.Fatalf("got %s, want 65", got)
	}
}

func TestParseDecimalPercent_StringWithPercent(t *testing.T) {
	got := parseDecimalPercent(mustRaw("80%"))
	if got.StringFixed(0) != "80" {
		t.Fatalf("got %s, want 80", got)
	}
}

func TestParseDecimalPercent_Null(t *testing.T) {
	got := parseDecimalPercent(json.RawMessage("null"))
	if !got.IsZero() {
		t.Fatalf("expected zero, got %s", got)
	}
}

func TestParseDecimalPercent_Empty(t *testing.T) {
	got := parseDecimalPercent(json.RawMessage(""))
	if !got.IsZero() {
		t.Fatalf("expected zero, got %s", got)
	}
}

// ─── parseAIChatContent ───────────────────────────────────────────────────────

func TestParseAIChatContent_Normal(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"{\"should_close\":true}"}}]}`
	content, err := parseAIChatContent([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != `{"should_close":true}` {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestParseAIChatContent_EmptyChoices(t *testing.T) {
	body := `{"choices":[]}`
	_, err := parseAIChatContent([]byte(body))
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestParseAIChatContent_EmptyContent(t *testing.T) {
	body := `{"choices":[{"message":{"role":"assistant","content":"   "}}]}`
	_, err := parseAIChatContent([]byte(body))
	if err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestParseAIChatContent_InvalidJSON(t *testing.T) {
	_, err := parseAIChatContent([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// ─── parseAICloseDecision ─────────────────────────────────────────────────────

func TestParseAICloseDecision(t *testing.T) {
	content := `{"should_close":true,"final_action":"close","continue_side":"short","long_win_rate":35,"short_win_rate":"65","confidence":72,"risk_level":"high","reason":"4H趋势转弱","long_suggested_hold":"2-4h","short_suggested_hold":"4-8h","next_check_in":"30m"}`
	decision, err := parseAICloseDecision(content)
	if err != nil {
		t.Fatalf("parseAICloseDecision() error = %v", err)
	}
	if !decision.ShouldClose {
		t.Fatalf("ShouldClose = false, want true")
	}
	if decision.ContinueSide != "short" {
		t.Fatalf("ContinueSide = %q, want short", decision.ContinueSide)
	}
	if decision.LongWinRate.StringFixed(0) != "35" || decision.ShortWinRate.StringFixed(0) != "65" {
		t.Fatalf("win rates = %s/%s, want 35/65", decision.LongWinRate, decision.ShortWinRate)
	}
	if decision.RiskLevel != "high" {
		t.Fatalf("RiskLevel = %q, want high", decision.RiskLevel)
	}
	if decision.FinalAction != "close" {
		t.Fatalf("FinalAction = %q, want close", decision.FinalAction)
	}
	if decision.LongSuggestedHold != "2-4h" {
		t.Fatalf("LongSuggestedHold = %q, want 2-4h", decision.LongSuggestedHold)
	}
	if decision.ShortSuggestedHold != "4-8h" {
		t.Fatalf("ShortSuggestedHold = %q, want 4-8h", decision.ShortSuggestedHold)
	}
	if decision.NextCheckIn != "30m" {
		t.Fatalf("NextCheckIn = %q, want 30m", decision.NextCheckIn)
	}
}

func TestParseAICloseDecision_Defaults(t *testing.T) {
	content := `{"should_close":false}`
	decision, err := parseAICloseDecision(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.ShouldClose {
		t.Fatal("ShouldClose should be false")
	}
	if decision.RiskLevel != "medium" {
		t.Fatalf("RiskLevel default = %q, want medium", decision.RiskLevel)
	}
	if decision.ContinueSide != "neutral" {
		t.Fatalf("ContinueSide default = %q, want neutral", decision.ContinueSide)
	}
	if decision.FinalAction != "hold" {
		t.Fatalf("FinalAction default = %q, want hold", decision.FinalAction)
	}
}

func TestParseAICloseDecision_NoPositionAction(t *testing.T) {
	content := `{"should_close":false,"final_action":"open_long","continue_side":"long","long_win_rate":62,"short_win_rate":38,"confidence":66,"risk_level":"medium","reason":"趋势回踩确认"}`
	decision, err := parseAICloseDecision(content)
	if err != nil {
		t.Fatalf("parseAICloseDecision() error = %v", err)
	}
	if decision.ShouldClose {
		t.Fatal("no-position decision should not close")
	}
	if decision.FinalAction != "open_long" || decision.ContinueSide != "long" {
		t.Fatalf("unexpected no-position decision: %+v", decision)
	}
}

func TestParseAICloseDecision_InvalidJSON(t *testing.T) {
	_, err := parseAICloseDecision("not json")
	if err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestParseAICloseAgentResult(t *testing.T) {
	content := `{"agent":"risk_budget","decision":"veto","veto":true,"score":20,"confidence":"80","risk_level":"high","continue_side":"neutral","reason":"强平距离过近"}`
	result, err := parseAICloseAgentResult("risk_budget", content)
	if err != nil {
		t.Fatalf("parseAICloseAgentResult() error = %v", err)
	}
	if result.Agent != "risk_budget" || result.Decision != "veto" || !result.Veto {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.RiskLevel != "high" || result.ContinueSide != "neutral" {
		t.Fatalf("unexpected risk/side: %+v", result)
	}
}

func TestFormatAICloseAgentReport(t *testing.T) {
	report, err := formatAICloseAgentReport([]aiCloseAgentResult{
		{Agent: "risk_budget", Decision: "veto", Veto: true, RiskLevel: "high", Reason: "强平距离过近"},
	})
	if err != nil {
		t.Fatalf("formatAICloseAgentReport() error = %v", err)
	}
	for _, want := range []string{"risk_budget", "veto", "强平距离过近"} {
		if !strings.Contains(report, want) {
			t.Fatalf("missing %q in report: %s", want, report)
		}
	}
}

// ─── defaultString ────────────────────────────────────────────────────────────

func TestDefaultString_UseValue(t *testing.T) {
	got := defaultString("hello", "fallback")
	if got != "hello" {
		t.Fatalf("got %q, want hello", got)
	}
}

func TestDefaultString_UseFallback(t *testing.T) {
	got := defaultString("  ", "fallback")
	if got != "fallback" {
		t.Fatalf("got %q, want fallback", got)
	}
}

func TestDefaultString_EmptyUseFallback(t *testing.T) {
	got := defaultString("", "fallback")
	if got != "fallback" {
		t.Fatalf("got %q, want fallback", got)
	}
}

// ─── buildAICloseTrigger ──────────────────────────────────────────────────────

func TestBuildAICloseTrigger_Profit(t *testing.T) {
	snap := PositionSnapshot{TriggerType: "profit", PnLPercent: decimal.NewFromFloat(12.5)}
	got := buildAICloseTrigger(snap)
	if got != "trigger=profit-threshold, has_position=false, pnl_percent=12.50%" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildAICloseTrigger_Loss(t *testing.T) {
	snap := PositionSnapshot{TriggerType: "loss", PnLPercent: decimal.NewFromFloat(-5.0)}
	got := buildAICloseTrigger(snap)
	if got != "trigger=loss-threshold, has_position=false, pnl_percent=-5.00%" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildAICloseTrigger_Scheduled(t *testing.T) {
	snap := PositionSnapshot{TriggerType: "scheduled", PnLPercent: decimal.NewFromFloat(0)}
	got := buildAICloseTrigger(snap)
	if got != "trigger=scheduled-check, has_position=false, pnl_percent=0.00%" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildAICloseTrigger_Unknown(t *testing.T) {
	snap := PositionSnapshot{TriggerType: "other", PnLPercent: decimal.NewFromFloat(1)}
	got := buildAICloseTrigger(snap)
	if got != "trigger=unknown, has_position=false, pnl_percent=1.00%" {
		t.Fatalf("got %q", got)
	}
}

// ─── buildPositionSummary ─────────────────────────────────────────────────────

func TestBuildPositionSummary(t *testing.T) {
	nowMs := time.Now().Add(-90 * time.Minute).UnixMilli()
	details := CurrentPositionDetails{
		InstID:           "BTC-USDT-SWAP",
		PositionSide:     "long",
		PositionSize:     "1",
		AvgPrice:         "90000",
		LastPrice:        "91000",
		LiqPrice:         "50000",
		UseMargin:        "1000",
		UnrealizedProfit: "100",
		PnLPercent:       "10",
		Leverage:         "10",
		MarginMode:       "cross",
		CreateTime:       fmt.Sprintf("%d", nowMs),
	}
	got := buildPositionSummary(details)
	for _, want := range []string{"inst=BTC-USDT-SWAP", "side=long", "avg=90000", "last=91000", "lever=10", "position_age=1h30m"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary missing %q: %s", want, got)
		}
	}
}

func TestParsePositionTimestamp_Milliseconds(t *testing.T) {
	want := time.Date(2026, 5, 25, 12, 0, 0, 0, time.Local)
	got, ok := parsePositionTimestamp(fmt.Sprintf("%d", want.UnixMilli()))
	if !ok {
		t.Fatal("expected timestamp to parse")
	}
	if !got.Equal(want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestFormatPositionAge(t *testing.T) {
	cases := map[time.Duration]string{
		45 * time.Minute:             "45m",
		90 * time.Minute:             "1h30m",
		(2*24 + 3) * time.Hour:       "2d3h",
		-1 * time.Second:             "unknown",
		59*time.Minute + time.Second: "59m",
	}
	for input, want := range cases {
		if got := formatPositionAge(input); got != want {
			t.Fatalf("formatPositionAge(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestBuildAIClosePositionPrompt_UsesSummary(t *testing.T) {
	snap := PositionSnapshot{PositionSummary: "custom-summary"}
	got := buildAIClosePositionPrompt(snap)
	if got != "custom-summary" {
		t.Fatalf("expected custom-summary, got %q", got)
	}
}

func TestBuildAIClosePositionPrompt_BuildsFromDetails(t *testing.T) {
	snap := PositionSnapshot{
		HasPosition:     true,
		PositionSummary: "",
		CurrentPosition: CurrentPositionDetails{InstID: "BTC-USDT-SWAP", PositionSide: "short"},
	}
	got := buildAIClosePositionPrompt(snap)
	if !strings.Contains(got, "inst=BTC-USDT-SWAP") {
		t.Fatalf("expected inst=BTC-USDT-SWAP in %q", got)
	}
}

func TestBuildAIClosePositionPrompt_NoPosition(t *testing.T) {
	snap := PositionSnapshot{
		AccountName:  "acc1",
		AccountUID:   "uid1",
		HasPosition:  false,
		TriggerType:  "no_position",
		PnLPercent:   decimal.Zero,
		BTCMarket:    nil,
		PositionSide: "",
	}
	got := buildAIClosePositionPrompt(snap)
	for _, want := range []string{"no_open_position=true", "acc1-uid1", "no_trade"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in no-position prompt: %s", want, got)
		}
	}
}

func TestBuildAIClosePreviousDecisionPrompt(t *testing.T) {
	record := &AICloseLastDecisionRecord{
		SavedAt:            "2026-06-03T10:00:00+08:00",
		AccountName:        "acc1",
		HasPosition:        true,
		InstID:             "BTC-USDT-SWAP",
		PositionSide:       "long",
		PnLPercent:         "3.25",
		LiqDistancePercent: "32.00",
		Decision: AICloseStoredDecision{
			ShouldClose:  false,
			FinalAction:  "hold",
			ContinueSide: "long",
			Confidence:   "68.00",
			RiskLevel:    "low",
			Reason:       "强平距离安全，趋势未破坏",
		},
	}
	got := buildAIClosePreviousDecisionPrompt(record)
	for _, want := range []string{"saved_at=2026-06-03T10:00:00+08:00", "final_action=hold", "should_close=false", "liq_distance=32.00%", "强平距离安全"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in previous decision prompt: %s", want, got)
		}
	}
}

func TestBuildAIClosePreviousDecisionPrompt_Nil(t *testing.T) {
	if got := buildAIClosePreviousDecisionPrompt(nil); got != "previous_ai_decision=nil" {
		t.Fatalf("got %q", got)
	}
}

func TestSaveAndLoadAICloseLastDecision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "ai_close_last_decision.json")
	now := time.Date(2026, 6, 3, 10, 30, 0, 0, time.FixedZone("CST", 8*3600))
	snap := PositionSnapshot{
		AccountName:        "acc1",
		AccountUID:         "uid1",
		HasPosition:        true,
		InstID:             "BTC-USDT-SWAP",
		PositionID:         "pos1",
		PositionSide:       "short",
		PnLPercent:         decimal.NewFromFloat(-2.5),
		LiqDistancePercent: "31.20",
		TriggerType:        "scheduled",
	}
	decision := &AICloseDecision{
		ShouldClose:        false,
		FinalAction:        "hold",
		ContinueSide:       "short",
		LongWinRate:        decimal.NewFromFloat(42),
		ShortWinRate:       decimal.NewFromFloat(58),
		LongSuggestedHold:  "不建议",
		ShortSuggestedHold: "2-4h",
		Confidence:         decimal.NewFromFloat(66),
		RiskLevel:          "low",
		Reason:             "距离爆仓价安全",
		Provider:           "tu2do",
		Model:              "gpt-test",
		RawResponse:        `{"judge":{"should_close":false}}`,
	}
	if err := saveAICloseLastDecision(path, snap, decision, now); err != nil {
		t.Fatalf("saveAICloseLastDecision() error = %v", err)
	}
	records, err := loadAICloseLastDecisions(path)
	if err != nil {
		t.Fatalf("loadAICloseLastDecisions() error = %v", err)
	}
	if len(records) == 0 {
		t.Fatal("expected record")
	}
	record := records[len(records)-1]
	if record.SavedAt != "2026-06-03T10:30:00+08:00" || record.Decision.FinalAction != "hold" || record.LiqDistancePercent != "31.20" {
		t.Fatalf("unexpected record: %+v", record)
	}
	if record.RawResponse == nil {
		t.Fatalf("expected raw response to be preserved")
	}
}

// ─── buildBTCMarketPrompt ─────────────────────────────────────────────────────

func TestBuildBTCMarketPrompt_Nil(t *testing.T) {
	got := buildBTCMarketPrompt(nil)
	if got != "btc_market=nil" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildBTCMarketPrompt_WithData(t *testing.T) {
	market := &BTCAnalysisSnapshot{
		Symbol: "BTCUSDT",
		TodayInfo: &BTCTodayInfo{
			CurrentPrice:       "90000",
			TodayChangePercent: "1.5",
			TodayHighPrice:     "92000",
			TodayLowPrice:      "88000",
			TodayVolume:        "10000",
			TodayQuoteVolume:   "900000000",
		},
		EMA1H20: &BTCMovingAverage{Value: "89500", Interval: "1h", Period: 20},
		RSI1H14: &BTCRSI{Value: "55.3", Period: 14},
	}
	got := buildBTCMarketPrompt(market)
	for _, want := range []string{"symbol=BTCUSDT", "ema_1h_20", "rsi_1h_14"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in market prompt: %s", want, got)
		}
	}
}

// ─── formatAICloseDecision ────────────────────────────────────────────────────

func TestFormatAICloseDecision_Nil(t *testing.T) {
	got := formatAICloseDecision(nil)
	if got != "AI未返回决策" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAICloseDecision_Full(t *testing.T) {
	d := &AICloseDecision{
		ShouldClose:        true,
		ContinueSide:       "short",
		LongWinRate:        decimal.NewFromFloat(35),
		ShortWinRate:       decimal.NewFromFloat(65),
		Confidence:         decimal.NewFromFloat(72),
		RiskLevel:          "high",
		Provider:           "tu2do",
		Model:              "gpt-4o-mini",
		Reason:             "趋势反转",
		LongSuggestedHold:  "2-4h",
		ShortSuggestedHold: "4-8h",
		NextCheckIn:        "30m",
	}
	got := formatAICloseDecision(d)
	for _, want := range []string{"true", "short", "35.00", "65.00", "72.00", "high", "tu2do", "趋势反转", "建议持仓 2-4h", "建议持仓 4-8h", "下次风险复检: 30m后"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatAICloseDecision_NoHoldFields(t *testing.T) {
	d := &AICloseDecision{
		ShouldClose:  false,
		ContinueSide: "long",
		LongWinRate:  decimal.NewFromFloat(60),
		ShortWinRate: decimal.NewFromFloat(40),
		Confidence:   decimal.NewFromFloat(55),
		RiskLevel:    "low",
	}
	got := formatAICloseDecision(d)
	if strings.Contains(got, "建议持仓") {
		t.Fatalf("should not contain 建议持仓 when fields are empty: %s", got)
	}
	if strings.Contains(got, "下次风险复检") {
		t.Fatalf("should not contain 下次风险复检 when NextCheckIn is empty: %s", got)
	}
}

func TestFormatHoldDescription(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"4-8h", "（建议持仓 4-8h）"},
		{"1-2d", "（建议持仓 1-2d）"},
		{"不建议", "（不建议持仓）"},
		{"", ""},
		{"  ", ""},
	}
	for _, c := range cases {
		got := formatHoldDescription(c.input)
		if got != c.want {
			t.Fatalf("formatHoldDescription(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// ─── BuildAIClosePrompt ───────────────────────────────────────────────────────

func TestBuildAIClosePrompt_UsesDefaultTemplate(t *testing.T) {
	snap := PositionSnapshot{
		TriggerType: "profit",
		PnLPercent:  decimal.NewFromFloat(5),
	}
	prompt, err := BuildAIClosePrompt("", snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "should_close") {
		t.Fatalf("prompt missing should_close field spec: %s", prompt)
	}
	if !strings.Contains(prompt, ">=30%") {
		t.Fatalf("prompt missing 30%% liquidation distance safety rule: %s", prompt)
	}
}

func TestBuildAIClosePrompt_CustomTemplate(t *testing.T) {
	tpl := "trigger={trigger} position={position}"
	snap := PositionSnapshot{
		HasPosition:     true,
		TriggerType:     "loss",
		PnLPercent:      decimal.NewFromFloat(-3),
		PositionSummary: "my-pos",
	}
	prompt, err := BuildAIClosePrompt(tpl, snap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "loss-threshold") {
		t.Fatalf("expected loss-threshold in %q", prompt)
	}
	if !strings.Contains(prompt, "my-pos") {
		t.Fatalf("expected my-pos in %q", prompt)
	}
}

func TestBuildAIClosePrompt_NoPosition(t *testing.T) {
	snap := PositionSnapshot{
		AccountName: "acc1",
		HasPosition: false,
		TriggerType: "no_position",
		PnLPercent:  decimal.Zero,
	}
	prompt, err := BuildAIClosePrompt("", snap)
	if err != nil {
		t.Fatalf("BuildAIClosePrompt() error = %v", err)
	}
	for _, want := range []string{"no-position-check", "has_position=false", "final_action", "open_long", "no_open_position=true"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt: %s", want, prompt)
		}
	}
}

func TestBuildAIClosePrompt_IncludesPreviousDecision(t *testing.T) {
	snap := PositionSnapshot{
		TriggerType: "scheduled",
		PnLPercent:  decimal.NewFromFloat(1),
		PreviousDecision: &AICloseLastDecisionRecord{
			SavedAt:     "2026-06-03T10:00:00+08:00",
			AccountName: "acc1",
			Decision: AICloseStoredDecision{
				FinalAction: "hold",
				Reason:      "上次建议继续观察",
			},
		},
	}
	prompt, err := BuildAIClosePrompt("", snap)
	if err != nil {
		t.Fatalf("BuildAIClosePrompt() error = %v", err)
	}
	for _, want := range []string{"上次 AI 建议", "2026-06-03T10:00:00+08:00", "上次建议继续观察"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt: %s", want, prompt)
		}
	}
}

func TestBuildAICloseAgentPrompt(t *testing.T) {
	spec := defaultAICloseAgentSpecs()[0]
	prompt := BuildAICloseAgentPrompt(spec, "base-context")
	for _, want := range []string{spec.DisplayName, "base-context", spec.Name} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt: %s", want, prompt)
		}
	}
}

func TestBuildAICloseJudgePrompt_IncludesAgentResults(t *testing.T) {
	snap := PositionSnapshot{
		TriggerType:     "scheduled",
		PnLPercent:      decimal.Zero,
		PositionSummary: "pos-summary",
	}
	prompt, err := BuildAICloseJudgePrompt("", snap, []aiCloseAgentResult{
		{Agent: "discipline", Decision: "veto", Veto: true, RiskLevel: "high", Reason: "纪律否决"},
	})
	if err != nil {
		t.Fatalf("BuildAICloseJudgePrompt() error = %v", err)
	}
	for _, want := range []string{"5 个专家机器人输出", "discipline", "纪律否决", "should_close"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt: %s", want, prompt)
		}
	}
}

func TestBuildAICloseJudgePrompt_AppendsAgentResultsForCustomTemplate(t *testing.T) {
	snap := PositionSnapshot{TriggerType: "scheduled", PnLPercent: decimal.Zero}
	prompt, err := BuildAICloseJudgePrompt("trigger={trigger}", snap, []aiCloseAgentResult{
		{Agent: "risk_budget", Decision: "veto", Veto: true, Reason: "风险否决"},
	})
	if err != nil {
		t.Fatalf("BuildAICloseJudgePrompt() error = %v", err)
	}
	for _, want := range []string{"trigger=scheduled-check", "risk_budget", "风险否决"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in prompt: %s", want, prompt)
		}
	}
}

// ─── format helpers ───────────────────────────────────────────────────────────

func TestFormatMovingAverage_Nil(t *testing.T) {
	if got := formatMovingAverage("ema_1h_20", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatMovingAverage_Valid(t *testing.T) {
	ma := &BTCMovingAverage{Value: "89500", Interval: "1h", Period: 20}
	got := formatMovingAverage("ema_1h_20", ma)
	if !strings.Contains(got, "ema_1h_20") || !strings.Contains(got, "89500") {
		t.Fatalf("unexpected: %q", got)
	}
}

func TestFormatMACD_Nil(t *testing.T) {
	if got := formatMACD("macd_1h", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatRSI_Nil(t *testing.T) {
	if got := formatRSI("rsi_1h_14", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatBollinger_Nil(t *testing.T) {
	if got := formatBollinger("boll_1h", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatVolume_Nil(t *testing.T) {
	if got := formatVolume("volume_1h", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatOpenInterest_Nil(t *testing.T) {
	if got := formatOpenInterest(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatFundingRate_Nil(t *testing.T) {
	if got := formatFundingRate(nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatLongShortRatio_Nil(t *testing.T) {
	if got := formatLongShortRatio("lsr_1h", nil); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatRecentKlines_Empty(t *testing.T) {
	if got := formatRecentKlines("recent_1h", nil, 3); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestFormatRecentKlines_LimitApplied(t *testing.T) {
	klines := []BTCKline{
		{OpenPrice: "1", HighPrice: "2", LowPrice: "0.5", ClosePrice: "1.5", Volume: "100"},
		{OpenPrice: "2", HighPrice: "3", LowPrice: "1.5", ClosePrice: "2.5", Volume: "200"},
		{OpenPrice: "3", HighPrice: "4", LowPrice: "2.5", ClosePrice: "3.5", Volume: "300"},
		{OpenPrice: "4", HighPrice: "5", LowPrice: "3.5", ClosePrice: "4.5", Volume: "400"},
	}
	got := formatRecentKlines("recent_1h", klines, 2)
	if strings.Contains(got, "open=1") || strings.Contains(got, "open=2") {
		t.Fatalf("should have trimmed to last 2, got: %s", got)
	}
	if !strings.Contains(got, "open=3") || !strings.Contains(got, "open=4") {
		t.Fatalf("should contain last 2 klines, got: %s", got)
	}
}

// ─── aiCloseApprovalStore ─────────────────────────────────────────────────────

func TestApprovalStore_CreateNew(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	req, created := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	if !created {
		t.Fatal("expected created=true for new entry")
	}
	if req.AlertKey != "key1" {
		t.Fatalf("AlertKey = %q, want key1", req.AlertKey)
	}
}

func TestApprovalStore_Deduplication(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	req1, created1 := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	req2, created2 := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason2", "profit", decimal.NewFromFloat(11),
	)
	if !created1 {
		t.Fatal("first call should create")
	}
	if created2 {
		t.Fatal("second call should return existing, not create")
	}
	if req1.ID != req2.ID {
		t.Fatalf("should return same request ID: %s vs %s", req1.ID, req2.ID)
	}
}

func TestApprovalStore_TakeValid(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	req, _ := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	taken, msg := store.take(req.ID)
	if taken == nil {
		t.Fatalf("take() returned nil, msg=%s", msg)
	}
	if taken.ID != req.ID {
		t.Fatalf("taken.ID = %q, want %q", taken.ID, req.ID)
	}
}

func TestApprovalStore_TakeNotFound(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	taken, msg := store.take("nonexistent-id")
	if taken != nil {
		t.Fatal("expected nil for nonexistent id")
	}
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestApprovalStore_TakeExpired(t *testing.T) {
	store := newAICloseApprovalStore(1 * time.Millisecond)
	req, _ := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	time.Sleep(5 * time.Millisecond)
	taken, msg := store.take(req.ID)
	if taken != nil {
		t.Fatal("expected nil for expired entry")
	}
	if !strings.Contains(msg, "过期") {
		t.Fatalf("expected expiry message, got %q", msg)
	}
}

func TestApprovalStore_TakeRemovesEntry(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	req, _ := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	store.take(req.ID)
	_, created := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	if !created {
		t.Fatal("after take(), same alertKey should be creatable again")
	}
}

func TestApprovalStore_Reject(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	req, _ := store.createOrGetExisting(
		dummyAccount(), dummyPosition(), "key1", "src", "reason", "profit", decimal.NewFromFloat(10),
	)
	rejected, msg := store.reject(req.ID)
	if rejected == nil {
		t.Fatalf("reject() returned nil, msg=%s", msg)
	}
}

func TestApprovalStore_ListEmpty(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	items := store.list()
	if len(items) != 0 {
		t.Fatalf("expected empty list, got %d", len(items))
	}
}

func TestApprovalStore_ListWithItems(t *testing.T) {
	store := newAICloseApprovalStore(30 * time.Minute)
	store.createOrGetExisting(dummyAccount(), dummyPosition(), "key1", "src", "r", "profit", decimal.NewFromFloat(1))
	store.createOrGetExisting(dummyAccount(), dummyPosition(), "key2", "src", "r", "loss", decimal.NewFromFloat(2))
	items := store.list()
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestApprovalStore_PruneExpired(t *testing.T) {
	store := newAICloseApprovalStore(1 * time.Millisecond)
	store.createOrGetExisting(dummyAccount(), dummyPosition(), "key1", "src", "r", "profit", decimal.NewFromFloat(1))
	time.Sleep(5 * time.Millisecond)
	items := store.list()
	if len(items) != 0 {
		t.Fatalf("expected 0 after pruning expired, got %d", len(items))
	}
}

func TestFormatNoPositionCurrentPrice(t *testing.T) {
	got := formatNoPositionCurrentPrice(&BTCAnalysisSnapshot{
		Symbol: "BTCUSDT",
		TodayInfo: &BTCTodayInfo{
			CurrentPrice: "69123.45",
		},
	})
	if got != "当前最新价: BTCUSDT 69123.45" {
		t.Fatalf("formatNoPositionCurrentPrice() = %q", got)
	}

	got = formatNoPositionCurrentPrice(nil)
	if got != "当前最新价: BTCUSDT 未获取到" {
		t.Fatalf("formatNoPositionCurrentPrice(nil) = %q", got)
	}
}

func TestExtractManualAICloseInput(t *testing.T) {
	input, ok := extractManualAICloseInput("@bot AI + 73210")
	if !ok || !input.AvgPrice.Equal(decimal.NewFromInt(73210)) || input.PositionSide != "" || !input.Leverage.Equal(decimal.NewFromInt(125)) {
		t.Fatalf("extractManualAICloseInput() = %+v, %t", input, ok)
	}

	input, ok = extractManualAICloseInput("@bot AI + 75000 + L")
	if !ok || !input.AvgPrice.Equal(decimal.NewFromInt(75000)) || input.PositionSide != "long" {
		t.Fatalf("extractManualAICloseInput(long) = %+v, %t", input, ok)
	}

	input, ok = extractManualAICloseInput("@bot AI + 75000 +S")
	if !ok || !input.AvgPrice.Equal(decimal.NewFromInt(75000)) || input.PositionSide != "short" {
		t.Fatalf("extractManualAICloseInput(short) = %+v, %t", input, ok)
	}

	input, ok = extractManualAICloseInput("@bot AI + 75000 + L + 100 + 20")
	if !ok ||
		!input.AvgPrice.Equal(decimal.NewFromInt(75000)) ||
		input.PositionSide != "long" ||
		!input.Balance.Equal(decimal.NewFromInt(100)) ||
		!input.PositionSize.Equal(decimal.NewFromInt(20)) ||
		!input.Leverage.Equal(decimal.NewFromInt(125)) {
		t.Fatalf("extractManualAICloseInput(default leverage) = %+v, %t", input, ok)
	}

	input, ok = extractManualAICloseInput("@bot AI + 75000 + L + 100 + 20 + 50")
	if !ok ||
		!input.AvgPrice.Equal(decimal.NewFromInt(75000)) ||
		input.PositionSide != "long" ||
		!input.Balance.Equal(decimal.NewFromInt(100)) ||
		!input.PositionSize.Equal(decimal.NewFromInt(20)) ||
		!input.Leverage.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("extractManualAICloseInput(full) = %+v, %t", input, ok)
	}

	_, ok = extractManualAICloseInput("@bot AI")
	if ok {
		t.Fatalf("plain AI should not be parsed as manual avg price")
	}
}

func TestManualAvgPricePnLPercent(t *testing.T) {
	avg := decimal.NewFromInt(100)

	got := manualAvgPricePnLPercent(avg, "110", "long", decimal.Zero)
	if !got.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("long pct = %s, want 10", got.String())
	}

	got = manualAvgPricePnLPercent(avg, "90", "short", decimal.Zero)
	if !got.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("short pct = %s, want 10", got.String())
	}

	got = manualAvgPricePnLPercent(avg, "110", "long", decimal.NewFromInt(20))
	if !got.Equal(decimal.NewFromInt(200)) {
		t.Fatalf("long pct with leverage = %s, want 200", got.String())
	}

	got = manualAvgPricePnLPercent(avg, "110", "unknown", decimal.Zero)
	if !got.IsZero() {
		t.Fatalf("unknown side pct = %s, want 0", got.String())
	}
}

func TestCalculateManualPositionMetrics(t *testing.T) {
	input := manualAICloseInput{
		AvgPrice:     decimal.NewFromInt(75000),
		PositionSide: "long",
		Balance:      decimal.NewFromInt(100),
		PositionSize: decimal.NewFromInt(30),
		Leverage:     decimal.NewFromInt(125),
	}

	got := calculateManualPositionMetrics(input, "76000", "long")
	if !got.QuantityBTC.Equal(decimal.NewFromFloat(0.03)) {
		t.Fatalf("qty = %s, want 0.03", got.QuantityBTC.String())
	}
	if !got.InitialMargin.Equal(decimal.NewFromInt(18)) {
		t.Fatalf("margin = %s, want 18", got.InitialMargin.String())
	}
	wantLiq, err := decimal.NewFromString("71666.6666666666666667")
	if err != nil {
		t.Fatal(err)
	}
	if !got.LiqPrice.Equal(wantLiq) {
		t.Fatalf("liq = %s, want 71666.6666666666666667", got.LiqPrice.String())
	}
	if !got.UnrealizedProfit.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("pnl = %s, want 30", got.UnrealizedProfit.String())
	}
	wantPct, err := decimal.NewFromString("166.66666666666667")
	if err != nil {
		t.Fatal(err)
	}
	if !got.PnLPercent.Equal(wantPct) {
		t.Fatalf("pnl percent = %s, want 166.6666666666666667", got.PnLPercent.String())
	}
}

func TestTelegramHelpMessage(t *testing.T) {
	msg := formatTelegramHelpMessage()
	for _, want := range []string{
		"help / 帮助",
		"余额 / balance",
		"AI + 75000 + L",
		"AI + 75000 + S",
		"AI + 75000 + L + 100 + 30 + 125",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("help message should contain %q, got:\n%s", want, msg)
		}
	}
	for _, removed := range []string{
		"确认平仓 AICLOSE-xxx",
		"拒绝平仓 AICLOSE-xxx",
		"待审批平仓",
	} {
		if strings.Contains(msg, removed) {
			t.Fatalf("help message should not contain removed command %q, got:\n%s", removed, msg)
		}
	}
}

func TestIsHelpCommandText(t *testing.T) {
	for _, text := range []string{"help", "/help", "帮助", " HELP "} {
		if !isHelpCommandText(text) {
			t.Fatalf("isHelpCommandText(%q) = false", text)
		}
	}
	if isHelpCommandText("help me") {
		t.Fatalf("help me should not be treated as bare help command")
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func dummyAccount() trade.AccountConfig {
	return trade.AccountConfig{Name: "test-account"}
}

func dummyPosition() utils.PositionInfo {
	return utils.PositionInfo{
		InstId:  "BTC-USDT-SWAP",
		PosSide: "long",
		PosId:   fmt.Sprintf("pos-%d", time.Now().UnixNano()),
	}
}
