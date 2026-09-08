package backfill

import (
	"os"
	"path/filepath"
	"testing"
)

// writeJSONL 在临时目录里造一个事件文件，返回目录。历史 JSONL 是只读真源，
// 全部测试都只在 t.TempDir() 里造数据，绝不碰 logs/。
func writeJSONL(t *testing.T, dir, date, content string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "events-"+date+".jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// ListEventFiles 只认 events-<date>.jsonl：来源目录里还躺着 ChatExport 导出、
// dev_sample_report.html、1.txt 之类的东西，扫进来会直接把报告污染掉。
func TestListEventFilesOnlyMatchesEventJSONL(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "2026-08-18", "")
	writeJSONL(t, dir, "2026-08-19", "")
	for _, noise := range []string{"dev_sample_report.html", "1.txt", "events-backup.jsonl", "events-2026-08-20.jsonl.bak"} {
		if err := os.WriteFile(filepath.Join(dir, noise), []byte("x"), 0o644); err != nil {
			t.Fatalf("write noise: %v", err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "ChatExport_2026-08-21"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	files, err := ListEventFiles(dir)
	if err != nil {
		t.Fatalf("ListEventFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 event files, got %v", files)
	}
}

// 坏行必须计数而不是静默跳过：eventlog.ParseFile 会静默丢掉它们，
// 回灌报告里"这个文件有几行、解析出几条、丢了几行"必须说得清。
func TestScanFileCountsBadAndBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONL(t, dir, "2026-08-18", `{"ts":"2026-08-18 00:00:01","account":"A","event":"balance","balance":100}
{"ts":"2026-08-18 00:01:01","account":"A","event":"balance","balance":101}

{not json}
{"ts":"2026-08-19 00:02:01","account":"A","event":"balance","balance":102}
`)
	events, stat, err := ScanFile(path)
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if stat.TotalLines != 4 || stat.BlankLines != 1 || stat.BadLines != 1 || stat.Events != 3 {
		t.Fatalf("unexpected stat: %+v", stat)
	}
	if stat.OffDate != 1 {
		t.Fatalf("expected 1 off-date event, got %d", stat.OffDate)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	// 归日按事件自身的 ts，不按文件名——按天的判定必须落在真实日期上。
	if events[2].Date != "2026-08-19" {
		t.Fatalf("expected event date 2026-08-19, got %s", events[2].Date)
	}
}
