package backfill

import (
	"os"
	"testing"

	"gorm.io/gorm"

	"argus_single/pkg/eventstore"
)

// 真实 MySQL 集成校验。默认跳过（CI 与本地开发没有库），需要时显式给 DSN：
//
//	EVENTSTORE_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/argus_event_check' \
//	  go test ./pkg/eventstore/backfill/ -run TestIntegration -v
//
// 库会被反复建表并写入，请指向一个可丢弃的 schema，不要指向生产库。
func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("EVENTSTORE_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 EVENTSTORE_TEST_DSN，跳过真实 MySQL 集成校验")
	}
	db, err := eventstore.OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := EnsureTables(db); err != nil {
		t.Fatalf("EnsureTables: %v", err)
	}
	return db
}

// cleanInstance 清掉本次用例的实例数据，避免历史残留干扰断言。
func cleanInstance(t *testing.T, db *gorm.DB, instanceKey string) {
	t.Helper()
	for _, table := range auditTables {
		if err := db.Exec("DELETE FROM "+table+" WHERE instance_key = ?", instanceKey).Error; err != nil {
			t.Fatalf("clean %s: %v", table, err)
		}
	}
}

// 回灌必须能反复重跑：第二遍提交同样多的行、一行都不新插入，
// 且库里总行数不变。这是"以后直接从 MySQL 拉历史"能成立的前提。
func TestIntegrationBackfillIsIdempotent(t *testing.T) {
	db := integrationDB(t)
	const instanceKey = "it-backfill-idempotent"
	cleanInstance(t, db, instanceKey)
	t.Cleanup(func() { cleanInstance(t, db, instanceKey) })

	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","instId":"BTCUSDT","event":"open","side":"long","size":3,"orderSize":1,"gapBp":5.4}
{"ts":"2026-08-18 00:00:02","account":"账户A-1394537246@qq.com","event":"balance","balance":100.5,"equity":0,"upl":-100.5,"equityKnown":true,"size":3}
{"ts":"2026-08-18 00:00:03","instId":"BTCUSDT","event":"dev_sample","devTicks":120,"devCross":{"3":4},"devMaxBp":7.5}
`)
	job := Job{InstanceKey: instanceKey, Sources: []string{dir}, Identities: []eventstore.Identity{accountA}}

	first := buildOne(t, job)
	stat, err := WriteDay(db, first.Days[0], 0)
	if err != nil {
		t.Fatalf("WriteDay: %v", err)
	}
	if stat.Submitted != 3 || stat.Inserted != 3 || stat.Existed != 0 {
		t.Fatalf("first pass should insert everything, got %+v", stat)
	}

	second := buildOne(t, job)
	stat, err = WriteDay(db, second.Days[0], 0)
	if err != nil {
		t.Fatalf("WriteDay rerun: %v", err)
	}
	if stat.Submitted != 3 || stat.Inserted != 0 || stat.Existed != 3 {
		t.Fatalf("rerun must be fully absorbed by event_hash, got %+v", stat)
	}

	audit, err := AuditDay(db, second.Days[0])
	if err != nil {
		t.Fatalf("AuditDay: %v", err)
	}
	if !audit.OK() {
		t.Fatalf("expected a clean audit, got missing=%d extra=%d", audit.MissingTotal, audit.ExtraTotal)
	}
	for _, table := range audit.Tables {
		if table.DBBackfill != table.DBRows {
			t.Fatalf("%s rows must all be source=2, got %d/%d", table.Table, table.DBBackfill, table.DBRows)
		}
	}
}

// ts 读回必须与 JSONL 的无时区串逐字一致。DSN 没设 loc 时会整体偏移 8 小时，
// 这个错误极隐蔽——数据全在、图能画、只是全错。
func TestIntegrationTimestampRoundTripsVerbatim(t *testing.T) {
	db := integrationDB(t)
	const instanceKey = "it-backfill-tz"
	cleanInstance(t, db, instanceKey)
	t.Cleanup(func() { cleanInstance(t, db, instanceKey) })

	const ts = "2026-08-21 09:47:33"
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-21", `{"ts":"`+ts+`","account":"账户A-1394537246@qq.com","instId":"BTC-USDT-SWAP","event":"catastrophe_stop","size":26,"roiPct":-448.32,"pnl":-67.76,"reason":"兜底止损"}`+"\n")
	result := buildOne(t, Job{InstanceKey: instanceKey, Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	if _, err := WriteDay(db, result.Days[0], 0); err != nil {
		t.Fatalf("WriteDay: %v", err)
	}

	var stored string
	err := db.Table("strategy_event").
		Select("DATE_FORMAT(ts, '%Y-%m-%d %H:%i:%s')").
		Where("instance_key = ?", instanceKey).
		Scan(&stored).Error
	if err != nil {
		t.Fatalf("read back ts: %v", err)
	}
	if stored != ts {
		t.Fatalf("ts must round-trip verbatim: JSONL %q vs DB %q", ts, stored)
	}
}

// 同一份 JSONL 归属两个不同实例时必须各占一行：实例1 与实例2 各有一个
// 「账户A-…」标签，是两个不同的真实账户，去重掉就丢了半边数据。
func TestIntegrationSameEventUnderTwoInstancesKeepsBothRows(t *testing.T) {
	db := integrationDB(t)
	const first, second = "it-backfill-inst-a", "it-backfill-inst-b"
	cleanInstance(t, db, first)
	cleanInstance(t, db, second)
	t.Cleanup(func() {
		cleanInstance(t, db, first)
		cleanInstance(t, db, second)
	})

	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100.5}`+"\n")
	for _, instanceKey := range []string{first, second} {
		result := buildOne(t, Job{InstanceKey: instanceKey, Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
		if _, err := WriteDay(db, result.Days[0], 0); err != nil {
			t.Fatalf("WriteDay %s: %v", instanceKey, err)
		}
	}
	var total int64
	if err := db.Table("balance_sample").Where("instance_key IN ?", []string{first, second}).Count(&total).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected 2 rows across two instances, got %d", total)
	}
}

// 缺行必须被巡检抓到：删掉一行后 AuditDay 要报 missing=1，并给出哈希样例。
func TestIntegrationAuditDetectsMissingRow(t *testing.T) {
	db := integrationDB(t)
	const instanceKey = "it-backfill-missing"
	cleanInstance(t, db, instanceKey)
	t.Cleanup(func() { cleanInstance(t, db, instanceKey) })

	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100.5}
{"ts":"2026-08-18 00:01:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100.6}
`)
	result := buildOne(t, Job{InstanceKey: instanceKey, Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	if _, err := WriteDay(db, result.Days[0], 0); err != nil {
		t.Fatalf("WriteDay: %v", err)
	}
	if err := db.Exec("DELETE FROM balance_sample WHERE instance_key = ? LIMIT 1", instanceKey).Error; err != nil {
		t.Fatalf("simulate a dropped event: %v", err)
	}
	audit, err := AuditDay(db, result.Days[0])
	if err != nil {
		t.Fatalf("AuditDay: %v", err)
	}
	if audit.MissingTotal != 1 {
		t.Fatalf("expected missing=1, got %d", audit.MissingTotal)
	}
	if len(audit.MissingHash) != 1 {
		t.Fatalf("expected one sample hash, got %v", audit.MissingHash)
	}
}

// 直写与回灌两条路径对同一条事件必须算出同一个 event_hash，
// 否则双写期的每一天都会平白多出一倍的行。
func TestIntegrationLiveWriteAndBackfillConverge(t *testing.T) {
	db := integrationDB(t)
	const instanceKey = "it-backfill-converge"
	cleanInstance(t, db, instanceKey)
	t.Cleanup(func() { cleanInstance(t, db, instanceKey) })

	const line = `{"ts":"2026-08-18 00:00:01","account":"账户A-1394537246@qq.com","event":"balance","balance":100.5,"size":3,"equity":100.5,"equityKnown":true}`
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", line+"\n")

	// 先按"回灌"写一行。
	backfilled := buildOne(t, Job{InstanceKey: instanceKey, Sources: []string{dir}, Identities: []eventstore.Identity{accountA}})
	if _, err := WriteDay(db, backfilled.Days[0], 0); err != nil {
		t.Fatalf("WriteDay: %v", err)
	}
	// 再模拟直写路径：同一条事件、source=1。
	scanned, _, err := ScanFile(backfilled.Files[0].Path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	rows, kind := eventstore.Convert(eventstore.Envelope{
		Event: scanned[0].Event, InstanceKey: instanceKey, UID: accountA.UID,
		ConfigVersion: 7, Source: eventstore.SourceLive,
	}, backfilled.Days[0].Rows.Balance[0].IngestedAt)
	if kind != eventstore.ErrNone {
		t.Fatalf("Convert: %v", kind)
	}
	live := &DayBatch{Date: "2026-08-18", InstanceKey: instanceKey, Rows: rows}
	stat, err := WriteDay(db, live, 0)
	if err != nil {
		t.Fatalf("WriteDay live: %v", err)
	}
	if stat.Inserted != 0 || stat.Existed != 1 {
		t.Fatalf("直写与回灌必须撞同一个 event_hash，得到 %+v", stat)
	}
}
