package redis

import (
	"context"
	"testing"
)

func TestArgusConfigRedisReturnsExplicitInitializationErrors(t *testing.T) {
	previous := Rdb
	Rdb = nil
	defer func() { Rdb = previous }()
	if err := SetContext(context.Background(), ArgusConfigVersionKey, 1, 0); err != ErrRedisNotInitialized {
		t.Fatalf("SetContext error = %v", err)
	}
	if _, err := ReadConfigSnapshot(context.Background()); err != ErrRedisNotInitialized {
		t.Fatalf("ReadConfigSnapshot error = %v", err)
	}
	if err := PublishConfigVersion(context.Background(), 1, "checksum"); err != ErrRedisNotInitialized {
		t.Fatalf("PublishConfigVersion error = %v", err)
	}
}
