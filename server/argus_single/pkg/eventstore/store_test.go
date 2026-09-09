package eventstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"argus_single/pkg/eventlog"
)

// dryRunDB 一个不真正连库的 gorm 句柄：SkipInitializeWithVersion 不发版本查询、
// DisableAutomaticPing 不建连接、DryRun 让语句只生成 SQL 不执行。
func dryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "u:p@tcp(127.0.0.1:3306)/lightning_trade?parseTime=true&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true, // 不建连接
		// 与 Open 保持一致：默认事务会让 gorm 在 DryRun 前先真的 BeginTx。
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open dry-run db: %v", err)
	}
	return gdb
}

// 幂等落点：撞唯一键必须译成 ON DUPLICATE KEY UPDATE id=id，
// 这样回灌与补录可以反复重放而不插重复行。
func TestInsertUsesOnDuplicateKeyUpdate(t *testing.T) {
	gdb := dryRunDB(t)
	rows, kind := Convert(Envelope{
		Event:       eventlog.Event{Ts: "2026-08-21 14:03:07", Account: "账户A", Event: eventlog.EvOpen, InstId: "BTCUSDT", Size: 3},
		InstanceKey: "argus-single-1",
	}, time.Now())
	if kind != ErrNone {
		t.Fatalf("convert kind=%v", kind)
	}
	// 断言的是**生产用的那个子句**，不是测试自己另写一份——旧写法在这里
	// 内联 clause.OnConflict{DoNothing: true}，与 insert() 实际用的东西脱钩，
	// 所以 id=id 撞自增列（Error 1869）这件事一直没被测出来。
	tx := gdb.Session(&gorm.Session{DryRun: true}).
		Clauses(idempotentInsertClause()).
		Create(rows.Strategy)
	if tx.Error != nil {
		t.Fatalf("dry-run create: %v", tx.Error)
	}
	sql := tx.Statement.SQL.String()
	if !strings.Contains(sql, "ON DUPLICATE KEY UPDATE") {
		t.Fatalf("缺少幂等子句: %s", sql)
	}
	// 绝不能给自增列赋值：同一条 INSERT 内出现重复键时 MySQL 8 会报 1869，
	// 整批写入全部失败（含同批里没问题的行）。
	if strings.Contains(sql, "`id`") {
		t.Fatalf("幂等子句不得触碰自增列 id: %s", sql)
	}
	if !strings.Contains(sql, "`instance_key`=`instance_key`") {
		t.Fatalf("幂等子句应把 instance_key 赋回自身（真空操作且绕开自增列）: %s", sql)
	}
	if !strings.Contains(sql, "`strategy_event`") {
		t.Fatalf("表名不对: %s", sql)
	}
}

// 交易路径绝不阻塞：队列满时直接丢弃并计数，Emit 必须立刻返回。
func TestEmitDropsInsteadOfBlockingWhenQueueFull(t *testing.T) {
	s := newStore(dryRunDB(t), Options{InstanceKey: "argus-single-1", QueueSize: 1})
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5; i++ {
			s.Emit(eventlog.Event{Ts: "2026-08-21 14:03:07", Account: "账户A", Event: eventlog.EvOpen})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Emit 阻塞了——交易路径会被写库拖住")
	}
	stats := s.Stats()
	if stats.Accepted != 1 || stats.Dropped != 4 {
		t.Fatalf("accepted=%d dropped=%d, want 1/4", stats.Accepted, stats.Dropped)
	}
}

func TestEmitStampsInstanceVersionAndUID(t *testing.T) {
	s := newStore(dryRunDB(t), Options{InstanceKey: "argus-single-ives", QueueSize: 4})
	s.SetConfigVersion(7)
	s.SetAccounts([]Identity{{Label: "账户A-1394537246@qq.com", UID: "9558450"}})
	s.Emit(eventlog.Event{Ts: "2026-08-21 14:03:07", Account: "账户A-1394537246@qq.com", Event: eventlog.EvOpen})
	env := <-s.queue
	if env.InstanceKey != "argus-single-ives" || env.ConfigVersion != 7 || env.UID != "9558450" || env.Source != SourceLive {
		t.Fatalf("实例维度未在入队时刻钉住: %+v", env)
	}
}

// 配置热更新只改版本号，账户映射不能被顺带清空（反之亦然）。
func TestSetConfigVersionKeepsAccountMapping(t *testing.T) {
	s := newStore(dryRunDB(t), Options{InstanceKey: "argus-single-1", QueueSize: 4})
	s.SetAccounts([]Identity{{Label: "账户A", UID: "9558450"}})
	s.SetConfigVersion(9)
	s.Emit(eventlog.Event{Ts: "2026-08-21 14:03:07", Account: "账户A", Event: eventlog.EvOpen})
	env := <-s.queue
	if env.UID != "9558450" || env.ConfigVersion != 9 {
		t.Fatalf("版本更新把 uid 映射冲掉了: %+v", env)
	}
}

// 未配置账户的事件不该被丢：uid 留空，唯一性回退 (instance_key, account_label)。
func TestEmitKeepsEventWhenUIDUnknown(t *testing.T) {
	s := newStore(dryRunDB(t), Options{InstanceKey: "argus-single-1", QueueSize: 4})
	s.Emit(eventlog.Event{Ts: "2026-08-21 14:03:07", Account: "未登记的账户", Event: eventlog.EvOpen})
	env := <-s.queue
	if env.UID != "" || env.Event.Account != "未登记的账户" {
		t.Fatalf("uid 未知时应留空并保留标签: %+v", env)
	}
}

// nil Store 上的全部方法都必须安全：Setup 失败时调用方仍会调 SetConfigVersion。
func TestNilStoreIsSafe(t *testing.T) {
	var s *Store
	s.Emit(eventlog.Event{})
	s.SetConfigVersion(1)
	s.SetAccounts([]Identity{{Label: "账户A"}})
	s.Start(nil)
	s.Close(time.Second)
	if got := s.Stats(); got != (Stats{}) {
		t.Fatalf("nil store 的 Stats 应为零值: %+v", got)
	}
}

// 双写链路：eventlog.Log 一次调用要同时落 JSONL 与投递 sink。
func TestEventlogDualWriteReachesSink(t *testing.T) {
	eventlog.ResetSinks()
	defer eventlog.ResetSinks()

	dir := t.TempDir()
	eventlog.Init(dir)
	s := newStore(dryRunDB(t), Options{InstanceKey: "argus-single-1", QueueSize: 8})
	eventlog.RegisterSink(s)

	eventlog.Log(eventlog.Event{Account: "账户A", Event: eventlog.EvOpen, InstId: "BTCUSDT", Side: "short", Size: 3})

	// JSONL 侧：真源必须照旧落盘。
	files, err := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("JSONL 未落盘: files=%v err=%v", files, err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil || !strings.Contains(string(body), `"event":"open"`) {
		t.Fatalf("JSONL 内容有误: %s err=%v", body, err)
	}

	// sink 侧：同一条事件、Ts 已补齐。
	if got := s.Stats().Accepted; got != 1 {
		t.Fatalf("sink 未收到事件: accepted=%d", got)
	}
	env := <-s.queue
	if env.Event.Ts == "" {
		t.Fatalf("sink 收到的事件缺 Ts，哈希会与 JSONL 不一致")
	}
	if _, err := ParseTs(env.Event.Ts); err != nil {
		t.Fatalf("sink 收到的 Ts 不可解析: %v", err)
	}
}

// sink panic 不得影响 JSONL 落盘与交易路径。
type panicSink struct{}

func (panicSink) Emit(eventlog.Event) { panic("boom") }

func TestSinkPanicDoesNotBreakJSONL(t *testing.T) {
	eventlog.ResetSinks()
	defer eventlog.ResetSinks()
	dir := t.TempDir()
	eventlog.Init(dir)
	eventlog.RegisterSink(panicSink{})
	eventlog.Log(eventlog.Event{Account: "账户A", Event: eventlog.EvOpen})
	files, _ := filepath.Glob(filepath.Join(dir, "events-*.jsonl"))
	if len(files) != 1 {
		t.Fatalf("sink panic 影响了 JSONL 落盘")
	}
}

// DB 中途不可用时：flush 失败只计数 + 告警，writer 继续活着，Emit 照常返回。
// 这是"写库失败只告警、绝不阻断交易路径"的兜底路径。
func TestFlushFailureIsCountedAndNeverPanics(t *testing.T) {
	gdb, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "u:p@tcp(127.0.0.1:1)/nowhere?parseTime=true&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open unreachable db: %v", err)
	}
	s := newStore(gdb, Options{InstanceKey: "argus-single-1", QueueSize: 8, FlushInterval: 50 * time.Millisecond})
	s.Start(context.Background())
	defer s.Close(2 * time.Second)

	s.Emit(eventlog.Event{Ts: "2026-08-21 14:03:07", Account: "账户A", Event: eventlog.EvOpen, InstId: "BTCUSDT", Size: 3})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if s.Stats().Failed > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("写库失败未被计数: %+v", s.Stats())
}
