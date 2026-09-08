package eventstore

import (
	"fmt"
	"testing"

	"argus_single/pkg/eventlog"
)

func ptrVal(t *testing.T, p *float64, want float64, field string) {
	t.Helper()
	if p == nil {
		t.Fatalf("%s 应被结构化出来，实际为 NULL", field)
	}
	if *p != want {
		t.Fatalf("%s=%v, want %v", field, *p, want)
	}
}

// reason 文本由 pkg/trade 的三个格式串产出，这里用同样的 Sprintf 生成，
// 埋点侧改格式会直接把本测试打红。
func TestParseGateReverseGateProfit(t *testing.T) {
	reason := fmt.Sprintf("盈利不足 ROI=%.1f%% < %.0f%%", -12.3, 8.0)
	got, ok := ParseGate(eventlog.EvGateBlock, reason, eventlog.Event{RoiPct: -12.3})
	if !ok || got.Kind != GateKindReverseGateProfit {
		t.Fatalf("kind=%q ok=%v", got.Kind, ok)
	}
	ptrVal(t, got.Actual, -12.3, "actual")
	ptrVal(t, got.Threshold, 8, "threshold")
}

func TestParseGateReverseGateFlip(t *testing.T) {
	reason := fmt.Sprintf("翻转单 order=%d>net=%d", 10, 4)
	got, ok := ParseGate(eventlog.EvGateBlock, reason, eventlog.Event{})
	if !ok || got.Kind != GateKindReverseGateFlip {
		t.Fatalf("kind=%q ok=%v", got.Kind, ok)
	}
	ptrVal(t, got.Actual, 10, "actual")
	ptrVal(t, got.Threshold, 4, "threshold")
}

func TestParseGateCapSkip(t *testing.T) {
	reason := fmt.Sprintf("当前%d+%d>上限%d", 26, 1, 26)
	got, ok := ParseGate(eventlog.EvCapSkip, reason, eventlog.Event{})
	if !ok || got.Kind != GateKindCap {
		t.Fatalf("kind=%q ok=%v", got.Kind, ok)
	}
	ptrVal(t, got.Actual, 27, "actual") // 实际值 = 当前张数 + 本次下单张数
	ptrVal(t, got.Threshold, 26, "threshold")
}

func TestParseGateTrendGate(t *testing.T) {
	short := fmt.Sprintf("趋势闸: 动量%+.2f%% ≥ %.1f%%, 禁逆势开/加空", 6.31, 5.0)
	got, ok := ParseGate(eventlog.EvTrendSkip, short, eventlog.Event{TrendMomPct: 6.31})
	if !ok || got.Kind != GateKindTrendGateShort {
		t.Fatalf("空头方向 kind=%q ok=%v", got.Kind, ok)
	}
	ptrVal(t, got.Actual, 6.31, "actual")
	ptrVal(t, got.Threshold, 5, "threshold")

	long := fmt.Sprintf("趋势闸: 动量%+.2f%% ≤ -%.1f%%, 禁逆势开/加多", -7.02, 5.0)
	got, ok = ParseGate(eventlog.EvTrendSkip, long, eventlog.Event{TrendMomPct: -7.02})
	if !ok || got.Kind != GateKindTrendGateLong {
		t.Fatalf("多头方向 kind=%q ok=%v", got.Kind, ok)
	}
	ptrVal(t, got.Actual, -7.02, "actual")
	// 多头闸线是负值，否则"实际值跌破阈值"在同一个符号系里读不通。
	ptrVal(t, got.Threshold, -5, "threshold")
}

// reason 文案漂移时仍要给出可聚合的 kind，否则一句文案改动会让
// "今天最常被什么条件挡住"这个统计整段失效。
func TestParseGateFallsBackToEventKind(t *testing.T) {
	cases := map[string]string{
		eventlog.EvGateBlock: GateKindReverseGate,
		eventlog.EvCapSkip:   GateKindCap,
		eventlog.EvTrendSkip: GateKindTrendGate,
	}
	for event, wantKind := range cases {
		got, ok := ParseGate(event, "门控拦截（文案已改）", eventlog.Event{})
		if !ok || got.Kind != wantKind {
			t.Fatalf("event=%s kind=%q ok=%v, want %q", event, got.Kind, ok, wantKind)
		}
	}
}

func TestParseGateSkipsNonGateEvents(t *testing.T) {
	for _, event := range []string{eventlog.EvOpen, eventlog.EvBalance, eventlog.EvTrailingClose, eventlog.EvLossAlert} {
		if _, ok := ParseGate(event, "移动止盈", eventlog.Event{}); ok {
			t.Fatalf("%s 不是门控事件，不应有 gate_kind", event)
		}
	}
}
