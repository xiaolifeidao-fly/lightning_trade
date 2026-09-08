package backfill

import (
	"strings"
	"testing"

	"argus_single/pkg/eventstore"
)

var accountA = eventstore.Identity{Label: "账户A-1394537246@qq.com", UID: "9558450"}
var accountB = eventstore.Identity{Label: "账户B-mortypeng@gmail.com", UID: "9533715"}

func buildOne(t *testing.T, job Job) *BuildResult {
	t.Helper()
	result, err := Build(job)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return result
}

func dayOf(t *testing.T, result *BuildResult, date string) *DayBatch {
	t.Helper()
	for _, day := range result.Days {
		if day.Date == date {
			return day
		}
	}
	t.Fatalf("no day %s in result", date)
	return nil
}

// uid 从来没进过事件（实测 9.9 万条历史事件里零出现）。查不到映射就必须
// 报错停下——猜一个 uid 会让整段归因错位，而且错得静悄悄。
func TestBuildRejectsUnmappedAccountLabel(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户C-未登记","event":"balance","balance":100}
`)
	_, err := Build(Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	if err == nil {
		t.Fatal("expected Build to refuse an unmapped account label")
	}
	if !strings.Contains(err.Error(), "账户C-未登记") {
		t.Fatalf("error must name the offending label, got %v", err)
	}
}

// dev_sample 是市场侧事件、account 本就为空，不受 uid 映射约束。
func TestBuildAcceptsAccountlessDevSample(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","instId":"BTCUSDT","event":"dev_sample","devTicks":120,"devCross":{"3":4},"devMaxBp":7.5}
`)
	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}})
	day := dayOf(t, result, "2026-08-18")
	if len(day.Rows.Dev) != 1 {
		t.Fatalf("expected 1 dev_sample row, got %d", len(day.Rows.Dev))
	}
	if day.Rows.Dev[0].Instrument != "BTCUSDT" {
		t.Fatalf("instrument must be normalized, got %q", day.Rows.Dev[0].Instrument)
	}
}

// 来源目录的日期快照互相重叠（7.10-datas 与 7.17-datas 都有 07-10），
// 同一条事件必须靠 event_hash 收敛成一行。
func TestBuildDeduplicatesOverlappingSourceSnapshots(t *testing.T) {
	line := `{"ts":"2026-07-10 00:00:01","account":"账户A-1394537246@qq.com","variant":"champion/default","event":"balance","balance":100.5}` + "\n"
	first := t.TempDir()
	second := t.TempDir()
	writeJSONL(t, first, "2026-07-10", line)
	// 第二个快照是同一天的更完整版本：包含同一行 + 一条新行。
	writeJSONL(t, second, "2026-07-10", line+`{"ts":"2026-07-10 00:01:01","account":"账户A-1394537246@qq.com","variant":"champion/default","event":"balance","balance":100.6}`+"\n")

	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{first, second}, Identities: []eventstore.Identity{accountA}})
	day := dayOf(t, result, "2026-07-10")
	if day.RawEvents != 3 {
		t.Fatalf("expected 3 parsed events, got %d", day.RawEvents)
	}
	if len(day.Hashes) != 2 || day.Duplicates != 1 {
		t.Fatalf("expected 2 unique / 1 duplicate, got %d/%d", len(day.Hashes), day.Duplicates)
	}
	if len(day.Rows.Balance) != 2 {
		t.Fatalf("expected 2 balance rows, got %d", len(day.Rows.Balance))
	}
}

// 8.18-8.21剧烈上涨 与 -实例2 是两台机器上同名日期的文件。同内容同时刻的
// 两行在两个实例下是两条真事件，不能被去重掉——实例键必须进哈希。
func TestBuildKeepsIdenticalEventsUnderDifferentInstances(t *testing.T) {
	line := `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100.5}` + "\n"
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", line)

	roc := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	ives := buildOne(t, Job{InstanceKey: "argus-single-ives", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	rocHash := hexOf(dayOf(t, roc, "2026-08-18").Rows.Balance[0].EventHash)
	ivesHash := hexOf(dayOf(t, ives, "2026-08-18").Rows.Balance[0].EventHash)
	if rocHash == ivesHash {
		t.Fatal("同一条事件在两个实例下必须算出不同的 event_hash")
	}
}

// net_size 的已知性按「天 × 账户」自适应判定，不硬编码 7/21 这个上线日期。
func TestBuildNetSizeKnownIsAdaptivePerDayPerAccount(t *testing.T) {
	dir := t.TempDir()
	// 老日志那天：全天没有任何带 size 的 balance ⇒ 全天未知。
	writeJSONL(t, dir, "2026-07-20", `{"ts":"2026-07-20 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100}
{"ts":"2026-07-20 00:01:01","account":"账户A-1394537246@qq.com","event":"balance","balance":101}
`)
	// 字段上线后那天：A 出现过带 size 的行 ⇒ 同日缺 size 就是已知空仓；
	// B 一条都没有 ⇒ B 仍然全天未知（判定是逐账户的，不是逐天的）。
	writeJSONL(t, dir, "2026-07-21", `{"ts":"2026-07-21 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100,"size":15}
{"ts":"2026-07-21 00:01:01","account":"账户A-1394537246@qq.com","event":"balance","balance":101}
{"ts":"2026-07-21 00:01:02","account":"账户B-mortypeng@gmail.com","event":"balance","balance":50}
`)
	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA, accountB}})

	old := dayOf(t, result, "2026-07-20")
	for _, row := range old.Rows.Balance {
		if row.NetSizeKnown != 0 || row.NetSize != nil {
			t.Fatalf("字段未上线那天必须留 NULL/未知，得到 known=%d size=%v", row.NetSizeKnown, row.NetSize)
		}
	}
	if old.Coverage.NetSizeKnown != 0 {
		t.Fatalf("coverage should report 0 known, got %d", old.Coverage.NetSizeKnown)
	}

	live := dayOf(t, result, "2026-07-21")
	byLabel := map[string][]*eventstore.BalanceSample{}
	for _, row := range live.Rows.Balance {
		byLabel[row.AccountLabel] = append(byLabel[row.AccountLabel], row)
	}
	rowsA := byLabel[accountA.Label]
	if len(rowsA) != 2 {
		t.Fatalf("expected 2 rows for A, got %d", len(rowsA))
	}
	for _, row := range rowsA {
		if row.NetSizeKnown != 1 || row.NetSize == nil {
			t.Fatalf("A 当天字段已上线，缺 size 应记成已知空仓，得到 known=%d size=%v", row.NetSizeKnown, row.NetSize)
		}
	}
	if *rowsA[1].NetSize != 0 {
		t.Fatalf("A 第二条缺 size ⇒ 已知空仓 0，得到 %d", *rowsA[1].NetSize)
	}
	rowsB := byLabel[accountB.Label]
	if len(rowsB) != 1 || rowsB[0].NetSizeKnown != 0 || rowsB[0].NetSize != nil {
		t.Fatalf("B 当天没有任何带 size 的样本，必须留未知，得到 %+v", rowsB)
	}
}

// 老日志（2026-07-21 前）没有 equity/equityKnown 字段：equity 必须留 NULL、
// known=0，而不是写成 0——那会把"权益恰好为零"这个极端回撤样本伪造出来。
func TestBuildEquityKnownFallsBackForOldLogs(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-07-01", `{"ts":"2026-07-01 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100}
`)
	writeJSONL(t, dir, "2026-07-22", `{"ts":"2026-07-22 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100,"equity":0,"upl":-100,"equityKnown":true}
`)
	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})

	old := dayOf(t, result, "2026-07-01").Rows.Balance[0]
	if old.EquityKnown != 0 || old.Equity != nil {
		t.Fatalf("老日志无 equity 字段应留未知，得到 known=%d equity=%v", old.EquityKnown, old.Equity)
	}
	fresh := dayOf(t, result, "2026-07-22").Rows.Balance[0]
	if fresh.EquityKnown != 1 || fresh.Equity == nil || *fresh.Equity != 0 {
		t.Fatalf("显式 equityKnown 的零权益必须原值入库，得到 known=%d equity=%v", fresh.EquityKnown, fresh.Equity)
	}
}

// instId 100% 系统性分裂：信号侧 BTCUSDT / 持仓侧 BTC-USDT-SWAP。
// 入库归一化，同时保留原值，否则对账工具每行都报不匹配。
func TestBuildNormalizesSplitInstrumentAndKeepsRaw(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","instId":"BTCUSDT","event":"open","side":"long","size":3,"orderSize":1,"gapBp":5.4}
{"ts":"2026-08-18 00:00:02","account":"账户A-1394537246@qq.com","instId":"BTC-USDT-SWAP","event":"trailing_close","size":3,"roiPct":12.5,"pnl":1.2,"peakPct":30}
`)
	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	day := dayOf(t, result, "2026-08-18")
	if len(day.Rows.Strategy) != 2 {
		t.Fatalf("expected 2 strategy rows, got %d", len(day.Rows.Strategy))
	}
	for _, row := range day.Rows.Strategy {
		if row.Instrument != "BTCUSDT" {
			t.Fatalf("instrument must normalize to BTCUSDT, got %q", row.Instrument)
		}
		if row.InstIdRaw == nil {
			t.Fatal("inst_id_raw must keep the JSONL original")
		}
	}
	if raw := *day.Rows.Strategy[1].InstIdRaw; raw != "BTC-USDT-SWAP" {
		t.Fatalf("持仓侧原值应保留 BTC-USDT-SWAP，得到 %q", raw)
	}
	if day.Coverage.SignalRows != 1 || day.Coverage.GapBpPresent != 1 {
		t.Fatalf("gapBp coverage wrong: %+v", day.Coverage)
	}
	if day.Coverage.TrailingRows != 1 || day.Coverage.PeakPctPresent != 1 {
		t.Fatalf("peakPct coverage wrong: %+v", day.Coverage)
	}
}

// 回灌行必须带 source=2 与 config_version=0：历史事件没有可考的配置版本，
// 留 0 才能与直写的真实版本号区分开。
func TestBuildMarksBackfillProvenance(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100}
`)
	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	row := dayOf(t, result, "2026-08-18").Rows.Balance[0]
	if row.Source != eventstore.SourceBackfill {
		t.Fatalf("expected source=%d, got %d", eventstore.SourceBackfill, row.Source)
	}
	if row.ConfigVersion != 0 {
		t.Fatalf("expected config_version=0, got %d", row.ConfigVersion)
	}
	if row.UID != accountA.UID {
		t.Fatalf("expected uid %s, got %s", accountA.UID, row.UID)
	}
}

func TestBuildAppliesDateRange(t *testing.T) {
	dir := t.TempDir()
	for _, date := range []string{"2026-08-18", "2026-08-19", "2026-08-20"} {
		writeJSONL(t, dir, date, `{"ts":"`+date+` 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100}`+"\n")
	}
	result := buildOne(t, Job{
		InstanceKey: "argus-single-roc", Sources: []string{dir},
		Identities: []eventstore.Identity{accountA}, Since: "2026-08-19", Until: "2026-08-19",
	})
	if len(result.Days) != 1 || result.Days[0].Date != "2026-08-19" {
		t.Fatalf("expected only 2026-08-19, got %d days", len(result.Days))
	}
	if result.OutOfRange != 2 {
		t.Fatalf("expected 2 filtered events, got %d", result.OutOfRange)
	}
}

// 门控事件的阈值与实际值只存在于 reason 文案里。历史行同样要能结构化，
// 否则"按 gate_kind 聚合统计今天最常被什么条件挡住"只覆盖上线之后。
func TestBuildStructuresHistoricalGateReason(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","event":"cap_skip","side":"long","size":26,"reason":"当前26+1>上限26"}
`)
	result := buildOne(t, Job{InstanceKey: "argus-single-roc", Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	day := dayOf(t, result, "2026-08-18")
	row := day.Rows.Strategy[0]
	if row.GateKind == nil {
		t.Fatal("历史门控事件必须结构化出 gate_kind")
	}
	if day.Coverage.GateRows != 1 || day.Coverage.GateKindResolved != 1 {
		t.Fatalf("gate coverage wrong: %+v", day.Coverage)
	}
	if row.Reason == nil || *row.Reason != "当前26+1>上限26" {
		t.Fatal("原始 reason 必须保留，供与 TG 消息逐字对账")
	}
}
