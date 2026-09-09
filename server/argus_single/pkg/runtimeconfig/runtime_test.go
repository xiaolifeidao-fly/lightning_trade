package runtimeconfig

import (
	"context"
	"errors"
	"strings"
	"testing"

	"argus_single/pkg/trade"
	commonRedis "common/middleware/redis"
	"service/argus_config/repository"
)

func TestRuntimeFromSnapshotRejectsIncompleteSnapshot(t *testing.T) {
	_, err := runtimeFromSnapshot(Snapshot{Version: repository.ArgusConfigVersion{Version: 1}}, "checksum")
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("error = %v, want incomplete snapshot validation error", err)
	}
}

func TestRestartRequiredOnlyReportsChangedFields(t *testing.T) {
	current := RuntimeConfig{ServerPort: 8855, RequestPath: "/", LogDir: "./logs"}
	next := RuntimeConfig{ServerPort: 8855, RequestPath: "/v2", LogDir: "./logs"}
	fields := restartRequired(next, current)
	if len(fields) != 1 || fields[0] != "requestPath" {
		t.Fatalf("restart fields = %#v, want requestPath only", fields)
	}
}

func TestHandleVersionMessageIgnoresOtherInstances(t *testing.T) {
	manager := &Manager{instanceID: "argus-single-roc"}
	var observed []uint64
	manager.SetReloadObserver(func(version uint64, err error) {
		observed = append(observed, version)
	})

	// 其他实例的发布消息必须被静默丢弃，不触发加载也不上报 reload。
	manager.handleVersionMessage(context.Background(), `{"instanceId":"argus-single-ives","version":9,"checksum":"other"}`)
	if len(observed) != 0 {
		t.Fatalf("reload observer fired for another instance: %v", observed)
	}

	// 属于本实例的消息不被过滤（会进入比对分支）。这里直接断言过滤判定本身，
	// 而不是观察 reload 回调——配置改为 DB 唯一来源后，比对在读库失败时会静默
	// 返回（避免 DB 抖动每 60 秒往心跳刷一条"热加载失败"），回调不再是可靠信号。
	if !manager.ownsMessage("argus-single-roc") {
		t.Fatal("本实例的消息被误过滤")
	}
	if manager.ownsMessage("argus-single-ives") {
		t.Fatal("其他实例的消息未被过滤")
	}
	// 分域改造之前发布的消息没有 instanceId，按历史消息放行。
	if !manager.ownsMessage("") {
		t.Fatal("空实例键应按历史消息放行")
	}
	// 走一遍完整入口，确认无 DB 时不 panic、不动运行配置。
	manager.handleVersionMessage(context.Background(), `{"instanceId":"argus-single-roc","version":9,"checksum":"mine"}`)
	if manager.Current().Version != 0 {
		t.Fatalf("running config changed without a usable database: %d", manager.Current().Version)
	}
}

func TestLoadCurrentRequiresInstanceKey(t *testing.T) {
	if _, err := loadCurrent(context.Background(), "  "); !errors.Is(err, commonRedis.ErrInstanceKeyRequired) {
		t.Fatalf("loadCurrent error = %v, want ErrInstanceKeyRequired", err)
	}
}

func TestPersistSessionRequiresInstanceKey(t *testing.T) {
	err := persistSession(context.Background(), "", trade.AccountConfig{Name: "primary"}, trade.SessionAccountData{})
	if !errors.Is(err, commonRedis.ErrInstanceKeyRequired) {
		t.Fatalf("persistSession error = %v, want ErrInstanceKeyRequired", err)
	}
}
