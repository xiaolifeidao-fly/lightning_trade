package eventstore

import (
	"context"
	"testing"
	"time"
)

// 真实 MySQL 下验 signal_slice：建表口径、幂等重放、90 天滚动清理边界。
// 默认跳过，用法与 store_integration_test.go 一致（EVENTSTORE_TEST_DSN）。
func TestIntegrationSignalSliceWriteAndReplay(t *testing.T) {
	store := integrationStore(t)
	if err := store.db.Exec("DELETE FROM signal_slice WHERE instance_key = ?", testInstanceKey).Error; err != nil {
		t.Fatalf("clean: %v", err)
	}
	store.Start(context.Background())
	anchor := time.Date(2026, 8, 21, 14, 3, 7, 0, time.Local)

	// 同一锚点投两次：唯一键 (instance_key, instrument, ts) 必须去重成一行。
	store.EmitSlice(sampleSlice(anchor))
	store.EmitSlice(sampleSlice(anchor))
	store.EmitSlice(sampleSlice(anchor.Add(time.Minute)))
	waitForFlush(t, store, 3)

	var count int64
	if err := store.db.Table("signal_slice").Where("instance_key = ?", testInstanceKey).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("同一锚点应去重成一行（两个锚点共 2 行）, got %d", count)
	}

	// 读回来的 ts 必须与写入的墙钟逐字一致（DATETIME + loc=Local 的口径自检）。
	var ts string
	if err := store.db.Table("signal_slice").
		Where("instance_key = ? AND instrument = ?", testInstanceKey, "BTCUSDT").
		Select("DATE_FORMAT(ts, '%Y-%m-%d %H:%i:%s')").Order("ts ASC").Limit(1).Scan(&ts).Error; err != nil {
		t.Fatalf("read ts: %v", err)
	}
	if ts != "2026-08-21 14:03:07" {
		t.Fatalf("ts 口径漂了（loc 不是 Local？）: got %q", ts)
	}
}

// 滚动清理只删过期切片，且绝不碰三张 append-only 事件表。
func TestIntegrationSlicePurgeRespectsRetention(t *testing.T) {
	store := integrationStore(t)
	if err := store.db.Exec("DELETE FROM signal_slice WHERE instance_key = ?", testInstanceKey).Error; err != nil {
		t.Fatalf("clean: %v", err)
	}
	store.Start(context.Background())
	now := time.Now().Truncate(time.Second)
	expired := now.Add(-SliceRetention - time.Hour)
	fresh := now.Add(-SliceRetention + time.Hour)
	store.EmitSlice(sampleSlice(expired))
	store.EmitSlice(sampleSlice(fresh))
	waitForFlush(t, store, 2)

	deleted, err := store.PurgeExpiredSlices(now)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("只应删掉过期的那一条, got %d", deleted)
	}
	var left []string
	if err := store.db.Table("signal_slice").Where("instance_key = ?", testInstanceKey).
		Select("DATE_FORMAT(ts, '%Y-%m-%d %H:%i:%s')").Scan(&left).Error; err != nil {
		t.Fatalf("read left: %v", err)
	}
	if len(left) != 1 || left[0] != fresh.Format("2006-01-02 15:04:05") {
		t.Fatalf("保留期内的切片被删了: %v", left)
	}
}

// waitForFlush 等 writer 把 n 行提交出去。
func waitForFlush(t *testing.T, store *Store, want uint64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if store.Stats().Inserted >= want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("等待落库超时: %+v", store.Stats())
}
