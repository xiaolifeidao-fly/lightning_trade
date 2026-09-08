// Package backfill 把 logs/argus_single 下的历史 JSONL 回灌进事件库，
// 并在 3 个月双写期内做 JSONL ↔ MySQL 的一致性巡检。
//
// 三条与写入侧共享的口径（改这里之前先读 pkg/eventstore 的包头）：
//  1. 行 → 行的转换只走 eventstore.Convert，回灌与直写共用同一份纯函数，
//     否则同一条事件在两条路径上会算出两个 event_hash、插出两行；
//  2. 历史 JSONL 是只读真源，本包只读不写、不改名、不移动；
//  3. 实例归属由**来源目录**决定（8.18-8.21剧烈上涨 与 -实例2 是两个不同
//     实例的同名日期文件），不能靠文件名或事件内容推断。
package backfill

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"argus_single/pkg/eventlog"
)

// eventFilePattern 事件文件名，与 eventlog.Logger.filePath 的 events-<date>.jsonl 一致。
var eventFilePattern = regexp.MustCompile(`^events-(\d{4}-\d{2}-\d{2})\.jsonl$`)

// FileStat 单个 JSONL 文件的行级统计。
//
// 为什么不直接用 eventlog.ParseFile：它静默跳过坏行，回灌报告需要说清
// "这个文件有几行、解析出几条、丢了几行"，静默跳过等于把缺口藏起来。
type FileStat struct {
	Path       string
	FileDate   string // 文件名里的日期
	TotalLines int    // 非空行总数
	BlankLines int
	BadLines   int // JSON 解析失败
	Events     int // 成功解析
	OffDate    int // ts 日期与文件名日期不一致的条数
}

// ScannedEvent 一条事件 + 它的来源，供报告定位问题行。
type ScannedEvent struct {
	Event    eventlog.Event
	Date     string // 按事件自身 ts 归日，不按文件名——按天的判定必须落在真实日期上
	FilePath string
	LineNo   int
}

// ListEventFiles 列出一个来源目录下的 events-<date>.jsonl（不递归；
// ChatExport / dc_files 之类的同级目录不是事件文件，天然被过滤掉）。
func ListEventFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("backfill: read source dir %s: %w", dir, err)
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !eventFilePattern.MatchString(entry.Name()) {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

// ScanFile 逐行读一个 JSONL，返回事件与行级统计。
//
// 解析用的是与 eventlog.ParseFile 相同的 json.Unmarshal 到同一个结构体，
// 因此后续 eventlog.Marshal 出来的规范行与直写路径字节级同源。
func ScanFile(path string) ([]ScannedEvent, FileStat, error) {
	stat := FileStat{Path: path}
	if m := eventFilePattern.FindStringSubmatch(filepath.Base(path)); m != nil {
		stat.FileDate = m[1]
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, stat, fmt.Errorf("backfill: open %s: %w", path, err)
	}
	defer file.Close()

	var events []ScannedEvent
	scanner := bufio.NewScanner(file)
	// 与 eventlog.ParseFile 同档：dev_sample 带两个阈值 map，单行可达数 KB。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			stat.BlankLines++
			continue
		}
		stat.TotalLines++
		var event eventlog.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			stat.BadLines++
			continue
		}
		stat.Events++
		date := eventDate(event.Ts)
		if stat.FileDate != "" && date != stat.FileDate {
			stat.OffDate++
		}
		events = append(events, ScannedEvent{Event: event, Date: date, FilePath: path, LineNo: lineNo})
	}
	if err := scanner.Err(); err != nil {
		return events, stat, fmt.Errorf("backfill: scan %s: %w", path, err)
	}
	return events, stat, nil
}

// eventDate 取 ts 的日期部分；ts 形态不对时返回空串，由上层记成坏数据。
func eventDate(ts string) string {
	trimmed := strings.TrimSpace(ts)
	if len(trimmed) < 10 {
		return ""
	}
	return trimmed[:10]
}
