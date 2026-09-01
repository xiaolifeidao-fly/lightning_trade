package redis

import (
	"context"
	"errors"
	"testing"
)

func TestArgusConfigRedisReturnsExplicitInitializationErrors(t *testing.T) {
	previous := Rdb
	Rdb = nil
	defer func() { Rdb = previous }()
	if err := SetContext(context.Background(), ArgusConfigVersionKey("argus-single-1"), 1, 0); err != ErrRedisNotInitialized {
		t.Fatalf("SetContext error = %v", err)
	}
	if _, err := ReadConfigSnapshot(context.Background(), "argus-single-1"); err != ErrRedisNotInitialized {
		t.Fatalf("ReadConfigSnapshot error = %v", err)
	}
	if err := PublishConfigVersion(context.Background(), "argus-single-1", 1, "checksum"); err != ErrRedisNotInitialized {
		t.Fatalf("PublishConfigVersion error = %v", err)
	}
}

func TestArgusConfigKeysAreNamespacedPerInstance(t *testing.T) {
	first, second := "argus-single-roc", "argus-single-ives"
	if ArgusConfigSnapshotKey(first) == ArgusConfigSnapshotKey(second) {
		t.Fatal("snapshot keys collide across instances")
	}
	if ArgusConfigVersionKey(first) == ArgusConfigVersionKey(second) {
		t.Fatal("version keys collide across instances")
	}
	if ArgusConfigSnapshotVersionKey(first, 3) == ArgusConfigSnapshotVersionKey(second, 3) {
		t.Fatal("versioned snapshot keys collide across instances")
	}
	if got, want := ArgusConfigSnapshotKey(first), "argus:config:argus-single-roc:snapshot"; got != want {
		t.Fatalf("snapshot key = %q, want %q", got, want)
	}
	if got, want := ArgusConfigSnapshotVersionKey(first, 7), "argus:config:argus-single-roc:snapshot:7"; got != want {
		t.Fatalf("versioned snapshot key = %q, want %q", got, want)
	}
}

func TestArgusConfigRejectsMissingInstanceKey(t *testing.T) {
	for name, err := range map[string]error{
		"write":   mustWriteErr(),
		"read":    mustReadErr(),
		"publish": PublishConfigVersion(context.Background(), "  ", 1, "checksum"),
		"delete":  DeleteConfigSnapshot(context.Background(), ""),
	} {
		if !errors.Is(err, ErrInstanceKeyRequired) {
			t.Fatalf("%s error = %v, want ErrInstanceKeyRequired", name, err)
		}
	}
}

func mustWriteErr() error {
	_, err := WriteConfigSnapshot(context.Background(), "", 1, struct{}{}, 0)
	return err
}

func mustReadErr() error {
	_, err := ReadConfigSnapshot(context.Background(), "")
	return err
}
