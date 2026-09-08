package episode

import (
	"os"
	"reflect"
	"testing"
	"time"

	"gorm.io/gorm"

	"argus_single/pkg/eventstore"
)

// 真实事件库上的派生校验。默认跳过（CI 与本地开发没有库），需要时显式给 DSN：
//
//	EVENTSTORE_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/argus_event_check' \
//	  go test ./pkg/eventstore/episode/ -run TestIntegration -v
//
// 只读用例（普查/纯函数）对任何库都安全；写库用例只碰 it- 前缀的测试实例键，
// 绝不用真实部署的 argus.instance.id——Replace 会整实例删行，指错库等于把
// r6 回灌的历史 episode 清掉。
const (
	censusInstance      = "argus-single-roc"
	writeTestInstance   = "it-episode-rebuild"
	censusWindowEndText = "2026-07-27 23:59:59"
)

func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("EVENTSTORE_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 EVENTSTORE_TEST_DSN，跳过真实事件库校验")
	}
	db, err := eventstore.OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	return db
}

// 设计文档 §6.2 的实测普查（30 天窗口，2026-06-28 ~ 2026-07-27）逐格复现。
// 这是整套状态机唯一的外部真值：口径改错时，reduce_to_zero 与 external_close
// 这两类最容易被吃掉的出场方式会第一个对不上。
func TestIntegrationMatchesDesignDocCensus(t *testing.T) {
	db := integrationDB(t)
	all, err := LoadEvents(db, censusInstance)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(all) == 0 {
		t.Skipf("实例 %s 在该库里没有事件，跳过普查", censusInstance)
	}
	end, err := eventstore.ParseTs(censusWindowEndText)
	if err != nil {
		t.Fatal(err)
	}
	var window []*eventstore.StrategyEvent
	for _, e := range all {
		if !e.Ts.After(end) {
			window = append(window, e)
		}
	}
	episodes, _ := Derive(window, time.Now())

	// 设计文档 §6.2 的表：账户A / 账户B 各自的出场方式分布。
	want := map[string]map[string]int{
		"A": {ExitTrailingClose: 37, ExitReduceToZero: 6, ExitCatastropheStop: 1, ExitExternalClose: 1, "open": 1},
		"B": {ExitTrailingClose: 45, ExitReduceToZero: 11, ExitCatastropheStop: 1, ExitExternalClose: 1, "open": 1},
	}
	got := map[string]map[string]int{"A": {}, "B": {}}
	for _, ep := range episodes {
		slot := ""
		switch {
		case len(ep.AccountLabel) >= 4 && ep.AccountLabel[:len("账户A")] == "账户A":
			slot = "A"
		case len(ep.AccountLabel) >= 4 && ep.AccountLabel[:len("账户B")] == "账户B":
			slot = "B"
		default:
			continue
		}
		got[slot][exitLabel(ep)]++
	}
	for slot, expected := range want {
		for kind, count := range expected {
			if got[slot][kind] != count {
				t.Errorf("账户%s 的 %s = %d，设计文档 §6.2 实测为 %d（全部：%v）",
					slot, kind, got[slot][kind], count, got[slot])
			}
		}
	}
}

// 派生对同一个库跑两次必须产出完全一样的行——episode 可随时 DROP 重建，
// 这条性质不成立的话，"重建"就变成了"再猜一次"。
func TestIntegrationDeriveIsReproducible(t *testing.T) {
	db := integrationDB(t)
	events, err := LoadEvents(db, censusInstance)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) == 0 {
		t.Skipf("实例 %s 在该库里没有事件", censusInstance)
	}
	stamp := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)
	first, statsA := Derive(events, stamp)
	second, statsB := Derive(events, stamp)
	if !reflect.DeepEqual(statsA, statsB) {
		t.Fatalf("两次派生的过程计数不同：%+v vs %+v", statsA, statsB)
	}
	if len(first) != len(second) {
		t.Fatalf("两次派生的 episode 数不同：%d vs %d", len(first), len(second))
	}
	for i := range first {
		if !reflect.DeepEqual(first[i], second[i]) {
			t.Fatalf("第 %d 个 episode 两次派生结果不同", i)
		}
	}
}

// 整实例替换的幂等性：连跑两次 Rebuild，第二次删掉的行数必须等于第一次写入的
// 行数，且最终结果一致。半旧半新的中间态没有任何解释，所以先删后插包在事务里。
func TestIntegrationRebuildReplacesWholeInstance(t *testing.T) {
	db := integrationDB(t)
	if err := EnsureTables(db); err != nil {
		t.Fatalf("EnsureTables: %v", err)
	}
	seedInstanceEvents(t, db)
	stamp := time.Date(2026, 9, 2, 0, 0, 0, 0, time.Local)

	_, _, firstWrite, err := Rebuild(db, writeTestInstance, stamp, 100)
	if err != nil {
		t.Fatalf("first Rebuild: %v", err)
	}
	if firstWrite.Episodes != 2 || firstWrite.Entries != 3 {
		t.Fatalf("首次重建写入 episodes=%d entries=%d，期望 2/3", firstWrite.Episodes, firstWrite.Entries)
	}
	_, _, secondWrite, err := Rebuild(db, writeTestInstance, stamp, 100)
	if err != nil {
		t.Fatalf("second Rebuild: %v", err)
	}
	if secondWrite.DeletedEpisodes != int64(firstWrite.Episodes) || secondWrite.DeletedEntries != int64(firstWrite.Entries) {
		t.Fatalf("第二次没有把上一轮的行清干净：删 %d/%d，上轮写 %d/%d",
			secondWrite.DeletedEpisodes, secondWrite.DeletedEntries, firstWrite.Episodes, firstWrite.Entries)
	}
	var episodes []*Episode
	if err := db.Where("instance_key = ?", writeTestInstance).Order("first_event_at").Find(&episodes).Error; err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("库里 episode = %d，期望 2", len(episodes))
	}
	if episodes[0].ExitKind == nil || *episodes[0].ExitKind != ExitTrailingClose {
		t.Fatalf("第一个 episode 的出场方式 = %v", episodes[0].ExitKind)
	}
	if episodes[1].ExitKind != nil {
		t.Fatal("第二个 episode 应仍持仓")
	}
	var entries []*EpisodeEntry
	if err := db.Where("instance_key = ?", writeTestInstance).Order("decided_at").Find(&entries).Error; err != nil {
		t.Fatalf("read entries: %v", err)
	}
	var total float64
	for _, entry := range entries {
		total += entry.AttributedPnl
	}
	if len(entries) != 3 {
		t.Fatalf("决策行 = %d，期望 3", len(entries))
	}
	near(t, total, 6.0, "归集回来的盈亏合计")

	t.Cleanup(func() {
		db.Where("instance_key = ?", writeTestInstance).Delete(&EpisodeEntry{})
		db.Where("instance_key = ?", writeTestInstance).Delete(&Episode{})
		db.Where("instance_key = ?", writeTestInstance).Delete(&eventstore.StrategyEvent{})
	})
}

// seedInstanceEvents 往测试实例键下灌一小段事件流：两笔加仓 + 移动止盈，
// 再补一笔仍持仓的开仓。写前先清干净，保证反复跑结果一致。
func seedInstanceEvents(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Where("instance_key = ?", writeTestInstance).Delete(&eventstore.StrategyEvent{}).Error; err != nil {
		t.Fatalf("clear seed events: %v", err)
	}
	s := &stream{}
	s.push("2026-08-18 00:00:00", "open", open("long", 1, 1))
	s.push("2026-08-18 00:10:00", "open", open("long", 2, 1))
	s.push("2026-08-18 00:30:00", "trailing_close", closing("long", 2), withPnl(40, 6.0))
	s.push("2026-08-18 01:00:00", "open", open("short", 1, 1))
	for i, row := range s.rows {
		row.InstanceKey = writeTestInstance
		row.Source = eventstore.SourceLive
		row.IngestedAt = time.Now()
		// id 交给自增；event_hash 换个测试专用前缀，避免与库里真实事件撞唯一键。
		row.Id = 0
		row.EventHash[0] = 0xEE
		row.EventHash[1] = byte(i)
	}
	if err := db.Create(s.rows).Error; err != nil {
		t.Fatalf("seed events: %v", err)
	}
}
