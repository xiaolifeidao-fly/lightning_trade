package monitor

import (
	"strings"
	"testing"
	"time"

	"argus_single/pkg/eventlog"
)

// 报告文案的权益诚实性（codex round-3 #1）：未知权益不得伪装成已知——
// 覆盖率不足时"含浮亏"标签要么降级、要么带下界+覆盖率标注；当前权益未知要明说。
func TestFormatComparisonReportEquityHonesty(t *testing.T) {
	since := time.Date(2026, 7, 20, 6, 0, 0, 0, time.UTC)
	until := since.Add(12 * time.Hour)

	full := &eventlog.AccountMetrics{Account: "A", Variant: "champion",
		FirstBalance: 400, LastBalance: 403, MaxDrawdownPct: 5.26, BalanceDrawdownPct: 0.2,
		LastEquity: 401, LastEquityKnown: true, BalanceEvents: 4, EquityKnownEvents: 4}
	partial := &eventlog.AccountMetrics{Account: "B", Variant: "probe",
		FirstBalance: 150, LastBalance: 150, MaxDrawdownPct: 3.1, BalanceDrawdownPct: 1.0,
		LastEquity: 141, LastEquityKnown: false, BalanceEvents: 4, EquityKnownEvents: 2}
	legacy := &eventlog.AccountMetrics{Account: "C", Variant: "old",
		FirstBalance: 100, LastBalance: 95, BalanceDrawdownPct: 8.33,
		BalanceEvents: 3, EquityKnownEvents: 0}

	msg := formatComparisonReport(map[string]*eventlog.AccountMetrics{
		"A": full, "B": partial, "C": legacy}, since, until)

	// 覆盖率 100%：现有口径不变
	if !strings.Contains(msg, "最大回撤(含浮亏): 5.26%") || !strings.Contains(msg, "当前权益: 401.00") {
		t.Fatalf("全覆盖账户口径变了:\n%s", msg)
	}
	// 覆盖率不足：回撤是下界, 须带"至少"+覆盖率; 最后一条无 equity → 当前权益未知
	if !strings.Contains(msg, "最大回撤(含浮亏): 至少3.10% (权益覆盖50%)") {
		t.Fatalf("部分覆盖应标注下界与覆盖率:\n%s", msg)
	}
	// 实盘 7/21 复现：721 条缺 1 条 = 99.86%，四舍五入会显示"覆盖100%"与"至少"自相矛盾。
	// 覆盖率与"至少"同为下界语义，一律向下取整。
	nearFull := &eventlog.AccountMetrics{Account: "D", Variant: "v",
		MaxDrawdownPct: 6.07, BalanceEvents: 723, EquityKnownEvents: 722,
		LastEquity: 152.52, LastEquityKnown: true}
	msg2 := formatComparisonReport(map[string]*eventlog.AccountMetrics{"D": nearFull}, since, until)
	if strings.Contains(msg2, "权益覆盖100%") {
		t.Fatalf("99.86%% 不得显示为覆盖100%%（与\"至少\"矛盾）:\n%s", msg2)
	}
	if !strings.Contains(msg2, "至少6.07% (权益覆盖99%)") {
		t.Fatalf("覆盖率应向下取整为 99%%:\n%s", msg2)
	}
	if !strings.Contains(msg, "当前权益: 未知") {
		t.Fatalf("最后样本权益未知应明说:\n%s", msg)
	}
	if strings.Contains(msg, "当前权益: 141") {
		t.Fatalf("过时权益值不得当作当前权益展示:\n%s", msg)
	}
	// 权益全未知（老日志/持续故障）：降级为已实现口径, 不得自称含浮亏
	if !strings.Contains(msg, "最大回撤(仅已实现): 8.33%") {
		t.Fatalf("零覆盖应降级为已实现口径:\n%s", msg)
	}
}

