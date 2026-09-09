package eventstore

import (
	"context"
	"os"
	"testing"
	"time"

	"argus_single/pkg/eventlog"
)

// 真实 MySQL 集成校验。默认跳过（CI 与本地开发没有库），需要时显式给 DSN：
//
//	EVENTSTORE_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/eventstore_check' \
//	  go test ./pkg/eventstore/ -run TestIntegration -v
//
// 库会被反复建表并写入，请指向一个可丢弃的 schema，不要指向生产库。
//
// 实例键一律用 it- 前缀的测试专用值，绝不用真实部署的 argus.instance.id：
// 这些用例会 DELETE 自己那个实例键下的全部行，用真实键等于给"DSN 不小心指错库"
// 配上一把删除生产历史的枪（r6 回灌的 16 万行历史事件就挂在真实实例键下）。
const (
	testInstanceKey    = "it-eventstore-main"
	testAltInstanceKey = "it-eventstore-alt"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("EVENTSTORE_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 EVENTSTORE_TEST_DSN，跳过真实 MySQL 集成校验")
	}
	store, err := Open(Options{DSN: dsn, InstanceKey: testInstanceKey, BatchSize: 50, FlushInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.EnsureTable(); err != nil {
		t.Fatalf("EnsureTable: %v", err)
	}
	t.Cleanup(func() { store.Close(3 * time.Second) })
	return store
}

// ts 必须是 DATETIME 而不是 TIMESTAMP：TIMESTAMP 会做时区转换，
// 与 JSONL 的无时区串就对不上了（差 8 小时，且极难发现）。
func TestIntegrationSchemaUsesDatetimeAndBinaryHash(t *testing.T) {
	store := integrationStore(t)
	type column struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
		DataType   string `gorm:"column:DATA_TYPE"`
		ColumnType string `gorm:"column:COLUMN_TYPE"`
	}
	for _, table := range []string{"strategy_event", "balance_sample", "dev_sample"} {
		var cols []column
		if err := store.db.Raw(`SELECT COLUMN_NAME, DATA_TYPE, COLUMN_TYPE FROM information_schema.columns
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&cols).Error; err != nil {
			t.Fatalf("read schema of %s: %v", table, err)
		}
		if len(cols) == 0 {
			t.Fatalf("表 %s 未建出来", table)
		}
		byName := map[string]column{}
		for _, c := range cols {
			byName[c.ColumnName] = c
		}
		if got := byName["ts"].DataType; got != "datetime" {
			t.Fatalf("%s.ts 类型为 %q，必须是 datetime", table, got)
		}
		if got := byName["event_hash"].ColumnType; got != "binary(16)" {
			t.Fatalf("%s.event_hash 类型为 %q，必须是 binary(16)", table, got)
		}
		if got := byName["instance_key"].DataType; got == "" {
			t.Fatalf("%s 缺 instance_key 列", table)
		}
		if got := byName["config_version"].DataType; got == "" {
			t.Fatalf("%s 缺 config_version 列", table)
		}
	}
}

// 端到端：eventlog.Log 一次 → JSONL + MySQL 双写；重放同一批事件不产生重复行；
// 读回的 ts 与 JSONL 字符串逐字一致（时区口径正确）。
func TestIntegrationDualWriteAndIdempotentReplay(t *testing.T) {
	store := integrationStore(t)
	if err := store.db.Exec("DELETE FROM strategy_event WHERE instance_key = ?", testInstanceKey).Error; err != nil {
		t.Fatalf("清理: %v", err)
	}
	if err := store.db.Exec("DELETE FROM balance_sample WHERE instance_key = ?", testInstanceKey).Error; err != nil {
		t.Fatalf("清理: %v", err)
	}
	if err := store.db.Exec("DELETE FROM dev_sample WHERE instance_key = ?", testInstanceKey).Error; err != nil {
		t.Fatalf("清理: %v", err)
	}

	eventlog.ResetSinks()
	defer eventlog.ResetSinks()
	eventlog.Init(t.TempDir())
	eventlog.RegisterSink(store)
	store.SetConfigVersion(17)
	store.SetAccounts([]Identity{{Label: "账户A-1394537246@qq.com", UID: "9558450"}})
	store.Start(context.Background())

	const ts = "2026-08-21 14:03:07"
	events := []eventlog.Event{
		{Ts: ts, Account: "账户A-1394537246@qq.com", Variant: "champion/S400_cap26_gate8", InstId: "BTCUSDT",
			Event: eventlog.EvOpen, Side: "short", Size: 12, OrderSize: 1, SigLast: 113001.5, SigMark: 112945.2, GapBp: 5.01},
		// 同一秒内的第二次触发必须是独立一行（实测 28.7% 的信号分钟内触发 ≥2 次）
		{Ts: ts, Account: "账户A-1394537246@qq.com", Variant: "champion/S400_cap26_gate8", InstId: "BTCUSDT",
			Event: eventlog.EvOpen, Side: "short", Size: 13, OrderSize: 1, SigLast: 113004.0, SigMark: 112946.0, GapBp: 5.09},
		{Ts: ts, Account: "账户A-1394537246@qq.com", InstId: "BTCUSDT", Event: eventlog.EvCapSkip,
			Side: "short", Size: 26, OrderSize: 1, Reason: "当前26+1>上限26"},
		{Ts: ts, Account: "账户A-1394537246@qq.com", InstId: "BTC-USDT-SWAP", Event: eventlog.EvTrailingClose,
			Side: "short", Size: 26, AvgPx: 112000, LastPx: 111000, RoiPct: 111.5, Pnl: 34.2, PeakPct: 150.3, Reason: "移动止盈"},
		{Ts: ts, Account: "账户A-1394537246@qq.com", Event: eventlog.EvBalance, Balance: 812.4, Equity: 0, Upl: -812.4, EquityKnown: true, Size: 0},
		{Ts: ts, Event: eventlog.EvDevSample, InstId: "BTCUSDT", DevTicks: 97, DevMaxBp: 9.31, DevMeanBp: 1.02,
			DevCross: map[string]int{"3": 7, "5": 2}, DevOver: map[string]int{"3": 12}},
	}
	// 写两轮：第二轮是"回灌重放"，必须被幂等哈希吃掉。
	for round := 0; round < 2; round++ {
		for _, e := range events {
			eventlog.Log(e)
		}
		waitDrain(t, store)
	}

	counts := map[string]int64{"strategy_event": 4, "balance_sample": 1, "dev_sample": 1}
	for table, want := range counts {
		var got int64
		if err := store.db.Raw("SELECT COUNT(*) FROM "+table+" WHERE instance_key = ?", testInstanceKey).Scan(&got).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s 行数=%d, want %d（重放去重或同秒多次触发有问题）", table, got, want)
		}
	}

	var row struct {
		TsText        string  `gorm:"column:ts_text"`
		UID           string  `gorm:"column:uid"`
		ConfigVersion uint64  `gorm:"column:config_version"`
		Instrument    string  `gorm:"column:instrument"`
		InstIdRaw     string  `gorm:"column:inst_id_raw"`
		GateKind      *string `gorm:"column:gate_kind"`
	}
	if err := store.db.Raw(`SELECT DATE_FORMAT(ts, '%Y-%m-%d %H:%i:%s') AS ts_text, uid, config_version,
		instrument, inst_id_raw, gate_kind FROM strategy_event WHERE event = ? AND instance_key = ?`,
		eventlog.EvTrailingClose, testInstanceKey).Scan(&row).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.TsText != ts {
		t.Fatalf("读回的 ts=%q，与 JSONL 的 %q 不一致（DSN loc 或列类型有问题）", row.TsText, ts)
	}
	if row.UID != "9558450" || row.ConfigVersion != 17 {
		t.Fatalf("实例维度未落库: uid=%q version=%d", row.UID, row.ConfigVersion)
	}
	if row.Instrument != "BTCUSDT" || row.InstIdRaw != "BTC-USDT-SWAP" {
		t.Fatalf("instId 归一化/原值保留有误: %q / %q", row.Instrument, row.InstIdRaw)
	}

	// 门控聚合：按 gate_kind 能直接答"今天最常被什么条件挡住"。
	var gate struct {
		GateKind      string  `gorm:"column:gate_kind"`
		GateThreshold float64 `gorm:"column:gate_threshold"`
		GateActual    float64 `gorm:"column:gate_actual"`
	}
	if err := store.db.Raw(`SELECT gate_kind, gate_threshold, gate_actual FROM strategy_event
		WHERE event = ? AND instance_key = ?`, eventlog.EvCapSkip, testInstanceKey).Scan(&gate).Error; err != nil {
		t.Fatalf("read gate: %v", err)
	}
	if gate.GateKind != GateKindCap || gate.GateThreshold != 26 || gate.GateActual != 27 {
		t.Fatalf("门控结构化未落库: %+v", gate)
	}

	// equity=0 是已知样本，net_size=0 是已知空仓，两者都不能被当成"缺失"。
	var bal struct {
		EquityKnown  int      `gorm:"column:equity_known"`
		Equity       *float64 `gorm:"column:equity"`
		NetSize      *int     `gorm:"column:net_size"`
		NetSizeKnown int      `gorm:"column:net_size_known"`
	}
	if err := store.db.Raw(`SELECT equity_known, equity, net_size, net_size_known FROM balance_sample
		WHERE instance_key = ?`, testInstanceKey).Scan(&bal).Error; err != nil {
		t.Fatalf("read balance: %v", err)
	}
	if bal.EquityKnown != 1 || bal.Equity == nil || *bal.Equity != 0 {
		t.Fatalf("equity 已知性丢失: %+v", bal)
	}
	if bal.NetSizeKnown != 1 || bal.NetSize == nil || *bal.NetSize != 0 {
		t.Fatalf("已知空仓被当成未知: %+v", bal)
	}

	var dev struct {
		DevTicks  int    `gorm:"column:dev_ticks"`
		CrossJson string `gorm:"column:dev_cross_json"`
	}
	if err := store.db.Raw(`SELECT dev_ticks, dev_cross_json FROM dev_sample WHERE instance_key = ?`, testInstanceKey).Scan(&dev).Error; err != nil {
		t.Fatalf("read dev: %v", err)
	}
	if dev.DevTicks != 97 || dev.CrossJson != `{"3": 7, "5": 2}` {
		t.Fatalf("dev_sample 载荷有误: ticks=%d cross=%s", dev.DevTicks, dev.CrossJson)
	}

	if stats := store.Stats(); stats.Dropped != 0 || stats.Failed != 0 || stats.BadTs != 0 || stats.UnknownEvent != 0 {
		t.Fatalf("有事件被丢弃: %+v", stats)
	}
}

// 实例1 与实例3 的 account1 是不同账户：同内容事件在两个实例下必须各占一行。
func TestIntegrationSameAccountLabelAcrossInstances(t *testing.T) {
	base := integrationStore(t)
	dsn := os.Getenv("EVENTSTORE_TEST_DSN")
	other, err := Open(Options{DSN: dsn, InstanceKey: testAltInstanceKey, FlushInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatalf("Open second instance: %v", err)
	}
	defer other.Close(3 * time.Second)
	if err := base.db.Exec("DELETE FROM strategy_event WHERE account_label = ? AND instance_key IN ?",
		"account1", []string{testInstanceKey, testAltInstanceKey}).Error; err != nil {
		t.Fatalf("清理: %v", err)
	}
	base.Start(context.Background())
	other.Start(context.Background())

	e := eventlog.Event{Ts: "2026-08-21 15:00:00", Account: "account1", InstId: "BTCUSDT",
		Event: eventlog.EvOpen, Side: "long", Size: 1, OrderSize: 1}
	base.Emit(e)
	other.Emit(e)
	waitDrain(t, base)
	waitDrain(t, other)

	var got int64
	if err := base.db.Raw("SELECT COUNT(*) FROM strategy_event WHERE account_label = ? AND instance_key IN ?",
		"account1", []string{testInstanceKey, testAltInstanceKey}).Scan(&got).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if got != 2 {
		t.Fatalf("同标签跨实例的事件行数=%d, want 2（instance_key 未进哈希）", got)
	}
}

func waitDrain(t *testing.T, store *Store) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stats := store.Stats()
		if len(store.queue) == 0 && stats.Flushes > 0 && stats.Inserted+stats.Failed >= stats.Accepted-uint64(len(store.queue)) {
			time.Sleep(200 * time.Millisecond)
			if len(store.queue) == 0 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("等待写入落库超时: %+v", store.Stats())
}

// 同一批 INSERT 内出现重复唯一键时，不能把整批带走。
//
// 幂等子句原来是 clause.OnConflict{DoNothing: true}，GORM 的 mysql 驱动把它
// 译成 ON DUPLICATE KEY UPDATE `id`=`id`，而 id 是自增列。跨批重复（前一批
// 已落库）没问题，但**同一条 INSERT 语句内部**撞键时 MySQL 8 会报
// Error 1869，insert 是整批提交，于是这一批全军覆没——包括同批里本来
// 没问题的行。线上表现是「Accepted 在涨、Inserted 不涨、Failed 在涨」，
// 而两条重复事件本身又不重要，很容易被当成噪声。
//
// strategy_event 是主表，两条完全相同的事件落在同一个 flush 窗口就会触发，
// 因此这条回归必须钉在主表上，而不是只钉在 signal_slice。
func TestIntegrationIntraBatchDuplicateKeepsRestOfBatch(t *testing.T) {
	store := integrationStore(t)
	if err := store.db.Exec("DELETE FROM strategy_event WHERE instance_key = ?", testInstanceKey).Error; err != nil {
		t.Fatalf("clean: %v", err)
	}
	store.Start(context.Background())

	dup := eventlog.Event{Ts: "2026-08-23 10:00:00", Account: "账户A", Event: eventlog.EvOpen,
		InstId: "BTC-USDT-SWAP", Side: "long", Size: 1, OrderSize: 1}
	other := eventlog.Event{Ts: "2026-08-23 10:00:01", Account: "账户B", Event: eventlog.EvOpen,
		InstId: "BTC-USDT-SWAP", Side: "short", Size: 2, OrderSize: 1}
	store.Emit(dup)
	store.Emit(dup)   // 同批重复：旧实现会让整批失败
	store.Emit(other) // 同批里的无辜行：必须活下来

	waitForFlush(t, store, 3)
	if s := store.Stats(); s.Failed != 0 {
		t.Errorf("同批重复不该造成写入失败: %+v", s)
	}
	var count int64
	if err := store.db.Table("strategy_event").Where("instance_key = ?", testInstanceKey).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("重复的去重成 1 行 + 另一条 = 2 行，实际 %d", count)
	}
}
