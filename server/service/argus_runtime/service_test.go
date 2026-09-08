package argus_runtime

import (
	"context"
	"strings"
	"testing"
)

func TestReloadRequiresInstanceID(t *testing.T) {
	service := NewArgusRuntimeService()
	if _, err := service.Reload(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "instance id") {
		t.Fatalf("error = %v, want instance id validation", err)
	}
}

// Redis 未初始化时 Reload 必须报错而不是静默成功，否则页面会显示「已下发」
// 但实例其实什么都没收到。
func TestReloadReportsPublishFailure(t *testing.T) {
	service := NewArgusRuntimeService()
	if _, err := service.Reload(context.Background(), "argus-single-1"); err == nil {
		t.Fatal("expected publish failure without redis")
	}
}
