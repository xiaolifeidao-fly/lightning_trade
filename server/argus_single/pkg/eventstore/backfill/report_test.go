package backfill

import (
	"strings"
	"testing"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
)

// 矩阵是回灌时的校验器：trailing_close 缺 roiPct 是异常，open 缺 roiPct 是正常。
// 历史 JSONL 里没有任何字段出现过真零值，NULL 的语义只能由事件类型决定。
func TestMissingRequiredFollowsApplicabilityMatrix(t *testing.T) {
	full := eventlog.Event{Event: eventlog.EvTrailingClose, RoiPct: 12.5, Pnl: 1.2, Size: 3}
	if got := missingRequired(full); len(got) != 0 {
		t.Fatalf("完整的 trailing_close 不应有异常，得到 %v", got)
	}
	partial := eventlog.Event{Event: eventlog.EvTrailingClose, Size: 3}
	got := missingRequired(partial)
	if len(got) != 2 || got[0] != "roiPct" || got[1] != "pnl" {
		t.Fatalf("expected roiPct+pnl missing, got %v", got)
	}
	// open 不适用 roiPct/pnl/peakPct，不能被报成异常。
	open := eventlog.Event{Event: eventlog.EvOpen, Size: 3, OrderSize: 1, Side: "long"}
	if got := missingRequired(open); len(got) != 0 {
		t.Fatalf("open 缺 roiPct 是正常，不应报异常，得到 %v", got)
	}
}

func TestReportRenderCoversEverySection(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","instId":"BTCUSDT","event":"cap_skip","side":"long","size":26,"reason":"当前26+1>上限26"}
{"ts":"2026-08-18 00:00:02","account":"账户A-1394537246@qq.com","event":"balance","balance":100.5}
{"ts":"2026-08-18 00:00:03","account":"账户A-1394537246@qq.com","instId":"BTC-USDT-SWAP","event":"trailing_close","size":3}
`)
	built := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	report := &Report{
		InstanceKey: "argus-single-roc",
		GeneratedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local),
		Mode:        "import",
		Sources:     []string{dir},
		Build:       built,
		Writes:      map[string]WriteStat{"2026-08-18": {Date: "2026-08-18", Submitted: 3, Inserted: 3}},
		Audits:      []DayAudit{{Date: "2026-08-18", InstanceKey: "argus-single-roc", Tables: []TableAudit{{Table: "balance_sample", JSONLRows: 1, DBRows: 1, DBBackfill: 1}}}},
	}
	rendered := report.Render()
	for _, want := range []string{
		"## 1. 源文件读取", "## 2. 逐日回灌", "## 3. 字段可用性", "## 4. JSONL ↔ MySQL 一致性",
		"## 5. 字段适用性异常", "## 6. 口径备注",
		"全部日期 JSONL 与 MySQL 逐条一致",
		// trailing_close 缺 roiPct/pnl 必须出现在异常表里
		"`trailing_close` | `roiPct`",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("report is missing %q:\n%s", want, rendered)
		}
	}
	// 同一批数据两次渲染必须逐字一致，否则每日巡检报告没法 diff。
	if rendered != report.Render() {
		t.Fatal("report rendering must be deterministic")
	}
}

func TestReportFlagsMissingRows(t *testing.T) {
	report := &Report{
		InstanceKey: "argus-single-roc",
		GeneratedAt: time.Date(2026, 9, 1, 12, 0, 0, 0, time.Local),
		Mode:        "audit",
		Audits: []DayAudit{{
			Date: "2026-09-01", InstanceKey: "argus-single-roc",
			Tables:       []TableAudit{{Table: "strategy_event", JSONLRows: 10, DBRows: 8, DBLive: 8, Missing: 2}},
			MissingTotal: 2, MissingHash: []string{"strategy_event:abc"},
		}},
	}
	rendered := report.Render()
	if !strings.Contains(rendered, "缺失 2 行") {
		t.Fatalf("missing rows must be called out:\n%s", rendered)
	}
	if !strings.Contains(rendered, "strategy_event:abc") {
		t.Fatalf("missing hash sample must be listed:\n%s", rendered)
	}
}
