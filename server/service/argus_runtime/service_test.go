package argus_runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestControlRejectsUnsupportedAction(t *testing.T) {
	script := writeControlScript(t)
	service, err := NewArgusRuntimeService(script)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Control(context.Background(), "restart; rm -rf /"); err == nil {
		t.Fatal("expected unsupported action error")
	}
}

func TestControlExecutesOnlyFixedAction(t *testing.T) {
	script := writeControlScript(t)
	service, err := NewArgusRuntimeService(script)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Control(context.Background(), ActionRestart)
	if err != nil {
		t.Fatal(err)
	}
	if result.Action != ActionRestart || result.Output != "restart" {
		t.Fatalf("unexpected control result: %+v", result)
	}
}

func TestNewRequiresExecutableControlScript(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-control.sh")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := NewArgusRuntimeService(path)
	if err == nil || !strings.Contains(err.Error(), "control.sh") {
		t.Fatalf("error = %v, want control.sh validation", err)
	}
}

func writeControlScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.sh")
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\nprintf '%s\\n' \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
