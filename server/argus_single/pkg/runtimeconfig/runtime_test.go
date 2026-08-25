package runtimeconfig

import (
	"strings"
	"testing"

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
