package eventlog

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInWindow(t *testing.T) {
	since := time.Date(2026, 6, 27, 12, 0, 0, 0, time.Local)
	until := time.Date(2026, 6, 28, 12, 0, 0, 0, time.Local)
	cases := []struct {
		ts   string
		want bool
	}{
		{"2026-06-27 13:00:00", true},
		{"2026-06-27 11:00:00", false}, // before since
		{"2026-06-28 13:00:00", false}, // after until
		{"2026-06-27 12:00:00", true},  // == since, inclusive
		{"garbage", false},
	}
	for _, c := range cases {
		if got := inWindow(c.ts, since, until); got != c.want {
			t.Errorf("inWindow(%q)=%v want %v", c.ts, got, c.want)
		}
	}
}

func TestLoadWindow(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, evs ...Event) {
		var b strings.Builder
		for _, e := range evs {
			b.WriteString(Marshal(e))
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("events-2026-06-27.jsonl",
		Event{Ts: "2026-06-27 11:00:00", Account: "B", Event: EvOpen}, // before window
		Event{Ts: "2026-06-27 13:00:00", Account: "B", Event: EvOpen}) // in
	write("events-2026-06-28.jsonl",
		Event{Ts: "2026-06-28 10:00:00", Account: "B", Event: EvOpen}, // in
		Event{Ts: "2026-06-28 13:00:00", Account: "B", Event: EvOpen}) // after window
	since := time.Date(2026, 6, 27, 12, 0, 0, 0, time.Local)
	until := time.Date(2026, 6, 28, 12, 0, 0, 0, time.Local)
	if got := LoadWindow(dir, since, until); len(got) != 2 {
		t.Fatalf("LoadWindow got %d events want 2", len(got))
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	e := Event{Ts: "2026-06-28 13:00:00", Account: "账户B", Variant: "challenger", Event: EvGateBlock, NetSide: "long", RoiPct: -104.2, Reason: "x"}
	line := Marshal(e)
	if line == "" || line[len(line)-1] != '\n' {
		t.Fatalf("Marshal should end with newline: %q", line)
	}
	var back Event
	if err := json.Unmarshal([]byte(line[:len(line)-1]), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Account != "账户B" || back.Event != EvGateBlock || back.RoiPct != -104.2 || back.Variant != "challenger" {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
}

func TestLoggerWriteParseAggregate(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	l.Log(Event{Account: "B", Variant: "challenger", Event: EvOpen, Size: 1})
	l.Log(Event{Account: "B", Event: EvTrailingClose, RoiPct: 50, Pnl: 2})

	files, _ := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("expected 1 jsonl file, got %d", len(files))
	}
	events, err := ParseFile(files[0])
	if err != nil || len(events) != 2 {
		t.Fatalf("ParseFile: got %d events err=%v", len(events), err)
	}
	m := Aggregate(events)
	if m["B"] == nil || m["B"].Opens != 1 || m["B"].TrailingCloses != 1 {
		t.Fatalf("aggregate after round-trip wrong: %+v", m["B"])
	}
}

func TestAggregate(t *testing.T) {
	events := []Event{
		{Account: "B", Variant: "challenger", Event: EvBalance, Balance: 100},
		{Account: "B", Event: EvOpen, Size: 1},
		{Account: "B", Event: EvGateBlock, RoiPct: -50},
		{Account: "B", Event: EvCapSkip},
		{Account: "B", Event: EvTrailingClose, RoiPct: 60, Pnl: 3, Size: 8},
		{Account: "B", Event: EvTrailingClose, RoiPct: 40, Pnl: 2},
		{Account: "B", Event: EvBalance, Balance: 120},
		{Account: "B", Event: EvBalance, Balance: 110}, // drawdown from peak 120 = 8.33%
	}
	m := Aggregate(events)
	b := m["B"]
	if b == nil {
		t.Fatal("missing account B")
	}
	if b.Variant != "challenger" || b.Opens != 1 || b.GateBlocks != 1 || b.CapSkips != 1 || b.TrailingCloses != 2 {
		t.Fatalf("counts wrong: %+v", b)
	}
	if b.CloseCount != 2 || b.CloseWins != 2 || b.CloseRoiSum != 100 || b.CloseRoiMin != 40 || b.CloseRoiMax != 60 || b.ClosePnlSum != 5 {
		t.Fatalf("close stats wrong: %+v", b)
	}
	if b.MaxSize != 8 || b.FirstBalance != 100 || b.LastBalance != 110 {
		t.Fatalf("size/balance wrong: %+v", b)
	}
	// 老日志无 equity：已实现回撤走 BalanceDrawdownPct，权益口径不得伪造
	if math.Abs(b.BalanceDrawdownPct-8.3333) > 0.01 {
		t.Fatalf("balance drawdown=%.4f want ~8.33", b.BalanceDrawdownPct)
	}
	if b.MaxDrawdownPct != 0 || b.EquityKnownEvents != 0 || b.BalanceEvents != 3 || b.LastEquityKnown {
		t.Fatalf("无 equity 样本不得进权益口径: %+v", b)
	}
}

func TestPeakPctRoundTripAndOmitempty(t *testing.T) {
	// 带 peakPct 的平仓事件 round-trip
	e := Event{Ts: "2026-07-17 12:00:00", Account: "账户A", Event: EvTrailingClose,
		Side: "long", Size: 17, AvgPx: 63000.5, LastPx: 63220.1, RoiPct: 44.5, PeakPct: 61.2}
	line := Marshal(e)
	var back Event
	if err := json.Unmarshal([]byte(line[:len(line)-1]), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.PeakPct != 61.2 || back.AvgPx != 63000.5 || back.LastPx != 63220.1 {
		t.Fatalf("round-trip mismatch: %+v", back)
	}
	// 零值省略：未激活的兜底止损没有 peak
	noPeak := Marshal(Event{Ts: "t", Account: "a", Event: EvCatastropheStop, RoiPct: -405})
	if strings.Contains(noPeak, "peakPct") {
		t.Fatalf("零值 peakPct 应省略: %s", noPeak)
	}
	// 老 JSONL 行（无新字段）解析兼容
	old := `{"ts":"2026-06-30 20:59:50","account":"账户A","event":"catastrophe_stop","side":"long","size":13,"roiPct":-303.29,"pnl":-18.87,"reason":"兜底止损"}`
	var oldEv Event
	if err := json.Unmarshal([]byte(old), &oldEv); err != nil {
		t.Fatalf("老行解析失败: %v", err)
	}
	if oldEv.PeakPct != 0 || oldEv.RoiPct != -303.29 {
		t.Fatalf("老行兼容性: %+v", oldEv)
	}
}

func TestEquityUplRoundTripAndOmitempty(t *testing.T) {
	e := Event{Ts: "t", Account: "a", Event: EvBalance, Balance: 420.43, Equity: 401.5, Upl: -18.93}
	line := Marshal(e)
	var back Event
	if err := json.Unmarshal([]byte(line[:len(line)-1]), &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back.Equity != 401.5 || back.Upl != -18.93 {
		t.Fatalf("round-trip: %+v", back)
	}
	noEq := Marshal(Event{Ts: "t", Account: "a", Event: EvBalance, Balance: 420.43})
	if strings.Contains(noEq, "equity") || strings.Contains(noEq, "upl") {
		t.Fatalf("无缓存时 equity/upl 应省略: %s", noEq)
	}
	old := `{"ts":"2026-07-10 18:20:50","account":"账户A","event":"balance","balance":405.05}`
	var oldEv Event
	if err := json.Unmarshal([]byte(old), &oldEv); err != nil || oldEv.Equity != 0 {
		t.Fatalf("老行兼容: %v %+v", err, oldEv)
	}
}

func TestAggregateEquityDrawdownAndExternalManual(t *testing.T) {
	events := []Event{
		// 老格式(无 equity) → 回退 balance
		{Account: "A", Event: EvBalance, Balance: 400},
		// 新格式: balance 平稳但 equity 深水 → 回撤必须用 equity 口径
		{Account: "A", Event: EvBalance, Balance: 402, Equity: 380, Upl: -22},
		{Account: "A", Event: EvBalance, Balance: 402, Equity: 360, Upl: -42},
		{Account: "A", Event: EvBalance, Balance: 403, Equity: 401, Upl: -2},
		// external/manual: 计数但不进胜率/ROI/CloseCount
		{Account: "A", Event: EvExternalClose, RoiPct: -240, Pnl: -16.76},
		{Account: "A", Event: EvManualClose, RoiPct: 12.0, Pnl: 0.5},
		// 策略平仓: 进胜率
		{Account: "A", Event: EvTrailingClose, RoiPct: 44.5, Pnl: 3.88},
	}
	m := Aggregate(events)["A"]
	if m.ExternalCloses != 1 || m.ManualCloses != 1 {
		t.Fatalf("external/manual 计数: %+v", m)
	}
	if m.CloseCount != 1 || m.CloseWins != 1 || m.CloseRoiSum != 44.5 {
		t.Fatalf("external/manual 不得进策略平仓统计: %+v", m)
	}
	// 权益口径回撤只用权益已知样本: 峰 380 → 谷 360 = 5.26%。
	// 老行(无 equity)不得回退 balance 冒充权益——balance 假峰会虚增回撤,
	// 接口故障期又会隐藏深水浮亏, 双向失真(codex round-3 #1)。
	if m.MaxDrawdownPct < 5.25 || m.MaxDrawdownPct > 5.28 {
		t.Fatalf("回撤应仅用已知 equity (380→360=5.26%%), got %.2f", m.MaxDrawdownPct)
	}
	if m.BalanceEvents != 4 || m.EquityKnownEvents != 3 {
		t.Fatalf("权益覆盖率计数: %+v", m)
	}
	if m.LastEquity != 401 || !m.LastEquityKnown {
		t.Fatalf("LastEquity: %+v", m)
	}
	// 收益率仍按 balance 首尾（本金口径不变）
	if p := m.PnLPct(); p < 0.74 || p > 0.76 {
		t.Fatalf("收益率仍用 balance 首尾 400→403=0.75%%, got %.2f", p)
	}
}

// equityKnown 显式有效标记（codex round-4）：equity 带 omitempty, 0 值会被
// JSON 省略; 若聚合只认 Equity>0, balance=100/upl=-100 这种已知 100% 回撤
// （恰是最极端的风险样本）会被误判未知丢弃, 负权益同理。
func TestAggregateEquityKnownZeroAndNegative(t *testing.T) {
	// 序列化: equity=0 省略但 equityKnown 保留, 解析侧不丢已知性
	line := Marshal(Event{Ts: "t", Account: "a", Event: EvBalance, Balance: 100, Upl: -100, EquityKnown: true})
	if strings.Contains(line, "\"equity\":") {
		t.Fatalf("equity=0 应被 omitempty 省略: %s", line)
	}
	if !strings.Contains(line, "equityKnown") {
		t.Fatalf("equityKnown 应保留: %s", line)
	}
	var back Event
	if err := json.Unmarshal([]byte(line[:len(line)-1]), &back); err != nil || !back.EquityKnown || back.Equity != 0 {
		t.Fatalf("round-trip: %v %+v", err, back)
	}

	// 零权益: 峰 400 → 0 = 已知的 100% 回撤, 必须计入
	m := Aggregate([]Event{
		{Account: "Z", Event: EvBalance, Balance: 400, Equity: 400, Upl: 0.0001, EquityKnown: true},
		{Account: "Z", Event: EvBalance, Balance: 100, Upl: -100, EquityKnown: true}, // equity=0
	})["Z"]
	if m.EquityKnownEvents != 2 || !m.LastEquityKnown || m.LastEquity != 0 {
		t.Fatalf("零权益应为已知样本: %+v", m)
	}
	if m.MaxDrawdownPct < 99.99 || m.MaxDrawdownPct > 100.01 {
		t.Fatalf("零权益回撤应 100%%, got %.2f", m.MaxDrawdownPct)
	}

	// 负权益: 峰 400 → -10 = 102.5%
	m = Aggregate([]Event{
		{Account: "N", Event: EvBalance, Balance: 400, Equity: 400, EquityKnown: true},
		{Account: "N", Event: EvBalance, Balance: 100, Equity: -10, Upl: -110, EquityKnown: true},
	})["N"]
	if m.MaxDrawdownPct < 102.49 || m.MaxDrawdownPct > 102.51 {
		t.Fatalf("负权益回撤应 102.5%%, got %.2f", m.MaxDrawdownPct)
	}

	// 病态首样本 ≤0: 无正峰, 回撤无定义 → 0, 不得 NaN/Inf
	m = Aggregate([]Event{
		{Account: "P", Event: EvBalance, Balance: 100, Upl: -100, EquityKnown: true},
	})["P"]
	if math.IsNaN(m.MaxDrawdownPct) || math.IsInf(m.MaxDrawdownPct, 0) || m.MaxDrawdownPct != 0 {
		t.Fatalf("无正峰不得除零: %v", m.MaxDrawdownPct)
	}

	// 老日志兼容: 无 equityKnown 字段, Equity>0 仍判已知
	m = Aggregate([]Event{{Account: "O", Event: EvBalance, Balance: 400, Equity: 390}})["O"]
	if m.EquityKnownEvents != 1 || !m.LastEquityKnown {
		t.Fatalf("老日志 Equity>0 回退失效: %+v", m)
	}
}

// 权益未知的诚实语义（codex round-3 #1）：无 equity 字段 = 该时点权益未知
// （老日志/启动首查/UPL 陈旧统一处理），不得用 balance 顶替进权益序列。
func TestAggregateEquityUnknownTail(t *testing.T) {
	events := []Event{
		{Account: "M", Event: EvBalance, Balance: 400, Equity: 390, Upl: -10},
		{Account: "M", Event: EvBalance, Balance: 400}, // UPL 陈旧被省略 → 未知
	}
	m := Aggregate(events)["M"]
	if m.LastEquityKnown {
		t.Fatalf("最后一条无 equity, 当前权益应为未知: %+v", m)
	}
	if m.LastEquity != 390 {
		t.Fatalf("最近一次已知权益应保留 390: %+v", m)
	}
	if m.BalanceEvents != 2 || m.EquityKnownEvents != 1 {
		t.Fatalf("覆盖率 1/2: %+v", m)
	}
	if c := m.EquityCoveragePct(); c < 49.9 || c > 50.1 {
		t.Fatalf("覆盖率应 50%%, got %.1f", c)
	}
	// 权益峰谷不受未知样本影响: 单点 390 无回撤
	if m.MaxDrawdownPct != 0 {
		t.Fatalf("未知样本不得参与权益峰谷: %+v", m)
	}
}

