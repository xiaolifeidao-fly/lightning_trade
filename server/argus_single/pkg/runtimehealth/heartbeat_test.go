package runtimehealth

import (
	"testing"
	"time"
)

func TestNewNormalizesHeartbeatTimingAndDefaults(t *testing.T) {
	reporter := New("", "", 5*time.Second, 2*time.Second)
	if reporter.instanceID != "default" {
		t.Fatalf("instance id = %q, want default", reporter.instanceID)
	}
	if reporter.ttl != 15*time.Second {
		t.Fatalf("ttl = %s, want 15s", reporter.ttl)
	}
	if reporter.state.Health != "healthy" || reporter.state.PID <= 0 {
		t.Fatalf("unexpected initial state: %+v", reporter.state)
	}
}

func TestRecordReloadUpdatesHealthAndVersion(t *testing.T) {
	reporter := New("argus-1", "dev", time.Second, 3*time.Second)
	reporter.RecordReload(7, "checksum-7", nil)
	if reporter.state.Version != 7 || reporter.state.ConfigChecksum != "checksum-7" || reporter.state.Health != "healthy" || reporter.state.LastReloadSuccess == nil || !*reporter.state.LastReloadSuccess {
		t.Fatalf("unexpected successful reload state: %+v", reporter.state)
	}
	// 加载失败时上报的必须仍是「程序实际在跑的那份」，否则管理端会把失败的新
	// 版本显示成已生效。
	reporter.RecordReload(0, "checksum-8", assertError{})
	if reporter.state.Version != 7 || reporter.state.ConfigChecksum != "checksum-7" {
		t.Fatalf("failed reload must not advance running config: %+v", reporter.state)
	}
	if reporter.state.Health != "degraded" || reporter.state.LastReloadError == "" || reporter.state.LastReloadSuccess == nil || *reporter.state.LastReloadSuccess {
		t.Fatalf("unexpected failed reload state: %+v", reporter.state)
	}
}

func TestSetConfigStateRecordsVersionAndChecksum(t *testing.T) {
	reporter := New("argus-1", "dev", time.Second, 3*time.Second)
	reporter.SetConfigState(12, "boot-checksum")
	if reporter.state.Version != 12 || reporter.state.ConfigChecksum != "boot-checksum" {
		t.Fatalf("unexpected boot config state: %+v", reporter.state)
	}
}

type assertError struct{}

func (assertError) Error() string { return "reload failed" }
