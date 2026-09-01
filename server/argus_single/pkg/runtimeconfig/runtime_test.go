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

	// 属于本实例的消息会进入加载分支；此处无 Redis/DB，只要求它不再被过滤掉。
	manager.handleVersionMessage(context.Background(), `{"instanceId":"argus-single-roc","version":9,"checksum":"mine"}`)
	if len(observed) == 0 {
		t.Fatal("reload observer did not fire for this instance")
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
