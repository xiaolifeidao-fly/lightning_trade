package backfill

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
)

// Job 一次回灌/巡检作业：一个实例 + 归属它的若干来源目录。
//
// 实例归属为什么按目录给而不是自动推断：8.18-8.21剧烈上涨 与
// 8.18-8.21剧烈上涨-实例2 是两台机器上同名日期的文件，账户标签
// （账户A-…）在两个实例里都存在但指向不同的真实账户。目录是唯一
// 可靠的归属依据，必须由调用方显式声明。
type Job struct {
	InstanceKey string
	Sources     []string              // 来源目录
	Identities  []eventstore.Identity // 该实例的 account 标签 → uid 映射
	Since       string                // 含端点，YYYY-MM-DD，空=不限
	Until       string                // 含端点，YYYY-MM-DD，空=不限
}

// Coverage 字段可用性（设计文档 §2.4："字段缺失是这个系统的常态"）。
// 报告里逐日打出来，下游查历史时才知道某天的某个口径能不能用。
type Coverage struct {
	BalanceRows      int
	EquityKnown      int // equity 已知（含 0 与负值）
	NetSizeKnown     int // 当天该账户 size 字段已上线
	SignalRows       int // open / cap_skip / gate_block / trend_skip
	GapBpPresent     int
	TrailingRows     int
	PeakPctPresent   int
	VariantMissing   int // variant 为空的业务事件，champion/challenger 归因会失灵
	GateKindResolved int // 门控事件成功结构化出 gate_kind 的条数
	GateRows         int
}

// DayBatch 一天（按事件自身 ts 归日）在一个实例上的全部回灌产物。
type DayBatch struct {
	Date        string
	InstanceKey string
	RawEvents   int // 去重前解析成功的条数
	Duplicates  int // 同 event_hash 的重复行（来源目录快照互相重叠）
	Rows        eventstore.Rows
	Hashes      map[string]struct{} // hex(event_hash)，供对账
	ByEvent     map[string]int      // 去重后按事件类型计数
	BadTs       int
	BadHash     int
	UnknownType map[string]int
	Anomalies   []FieldAnomaly
	Coverage    Coverage
}

// BuildResult 一次 Build 的全部结果。
type BuildResult struct {
	InstanceKey     string
	Days            []*DayBatch // 按日期升序
	Files           []FileStat
	UnknownAccounts map[string]int // 配置里查不到 uid 的账户标签
	OutOfRange      int            // 被 Since/Until 过滤掉的条数
	NoDate          int            // ts 无法归日的条数
}

// Build 扫描来源目录并把事件转成待入库的行。纯内存、不碰数据库、不写 JSONL。
//
// 未知账户标签一律**拒绝导入并报错**（设计文档 §9）：uid 从来没进过事件，
// 任何字符串解析都不可能从"账户A-1394537246@qq.com"恢复出 9558450，
// 猜错会让归因整段错位。dev_sample 是市场侧事件、account 本就为空，不受此限。
func Build(job Job) (*BuildResult, error) {
	instanceKey := strings.TrimSpace(job.InstanceKey)
	if instanceKey == "" {
		return nil, fmt.Errorf("backfill: instance key is required")
	}
	uidByLabel := make(map[string]string, len(job.Identities))
	for _, identity := range job.Identities {
		if identity.Label == "" {
			continue
		}
		uidByLabel[identity.Label] = identity.UID
	}

	result := &BuildResult{InstanceKey: instanceKey, UnknownAccounts: map[string]int{}}
	byDate := map[string][]ScannedEvent{}
	for _, dir := range job.Sources {
		files, err := ListEventFiles(dir)
		if err != nil {
			return nil, err
		}
		for _, path := range files {
			events, stat, err := ScanFile(path)
			if err != nil {
				return nil, err
			}
			result.Files = append(result.Files, stat)
			for _, scanned := range events {
				switch {
				case scanned.Date == "":
					result.NoDate++
				case !inDateRange(scanned.Date, job.Since, job.Until):
					result.OutOfRange++
				default:
					byDate[scanned.Date] = append(byDate[scanned.Date], scanned)
				}
			}
		}
	}

	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Strings(dates)

	ingestedAt := time.Now()
	for _, date := range dates {
		day, err := buildDay(instanceKey, date, byDate[date], uidByLabel, result.UnknownAccounts, ingestedAt)
		if err != nil {
			return nil, err
		}
		result.Days = append(result.Days, day)
	}
	if len(result.UnknownAccounts) > 0 {
		return result, fmt.Errorf("backfill: %d unmapped account label(s) under instance %s: %s (add trade.accountN.name/uid to the instance properties, uid must not be guessed)",
			len(result.UnknownAccounts), instanceKey, strings.Join(sortedKeys(result.UnknownAccounts), ", "))
	}
	return result, nil
}

func buildDay(instanceKey, date string, events []ScannedEvent, uidByLabel map[string]string, unknown map[string]int, ingestedAt time.Time) (*DayBatch, error) {
	day := &DayBatch{
		Date:        date,
		InstanceKey: instanceKey,
		RawEvents:   len(events),
		Hashes:      make(map[string]struct{}, len(events)),
		ByEvent:     map[string]int{},
		UnknownType: map[string]int{},
	}

	// net_size 的已知性按"天 × 账户"自适应判定（设计文档 §5.2）：
	// 某账户当天出现过至少一条带 size 的 balance ⇒ 该字段那天已上线，
	// 同日缺失即"已知空仓"；一条都没有 ⇒ 全天未知。不硬编码 7/21 这个上线日期。
	sizeLiveByAccount := map[string]bool{}
	for _, scanned := range events {
		if scanned.Event.Event == eventlog.EvBalance && scanned.Event.Size != 0 {
			sizeLiveByAccount[scanned.Event.Account] = true
		}
	}

	anomalyIndex := map[string]*FieldAnomaly{}
	for _, scanned := range events {
		event := scanned.Event
		uid := ""
		if event.Account != "" {
			mapped, ok := uidByLabel[event.Account]
			if !ok {
				unknown[event.Account]++
				continue
			}
			uid = mapped
		}
		rows, kind := eventstore.Convert(eventstore.Envelope{
			Event:       event,
			InstanceKey: instanceKey,
			UID:         uid,
			// 历史事件没有配置版本可考：回灌一律留 0，与直写的真实版本号区分开。
			ConfigVersion: 0,
			Source:        eventstore.SourceBackfill,
		}, ingestedAt)
		switch kind {
		case eventstore.ErrNone:
		case eventstore.ErrBadTs:
			day.BadTs++
			continue
		case eventstore.ErrBadHash:
			day.BadHash++
			continue
		case eventstore.ErrUnknownEvent:
			day.UnknownType[event.Event]++
			continue
		}

		hash := rowHash(rows)
		if hash == "" {
			day.BadHash++
			continue
		}
		if _, seen := day.Hashes[hash]; seen {
			day.Duplicates++
			continue
		}
		day.Hashes[hash] = struct{}{}
		day.ByEvent[event.Event]++

		for _, balance := range rows.Balance {
			applyNetSizeKnown(balance, event, sizeLiveByAccount[event.Account])
		}
		day.Rows.Strategy = append(day.Rows.Strategy, rows.Strategy...)
		day.Rows.Balance = append(day.Rows.Balance, rows.Balance...)
		day.Rows.Dev = append(day.Rows.Dev, rows.Dev...)

		accumulateCoverage(&day.Coverage, event, rows)
		for _, field := range missingRequired(event) {
			key := event.Event + "/" + field
			if entry, ok := anomalyIndex[key]; ok {
				entry.Count++
				continue
			}
			anomalyIndex[key] = &FieldAnomaly{Event: event.Event, Field: field, Count: 1, Example: event.Ts}
		}
	}

	for _, anomaly := range anomalyIndex {
		day.Anomalies = append(day.Anomalies, *anomaly)
	}
	sortAnomalies(day.Anomalies)
	return day, nil
}

// applyNetSizeKnown 覆盖 Convert 给回灌行留的保守默认值。
//
// Convert 对 source=backfill 一律置 net_size_known=0，因为它是纯函数、
// 看不到"当天这个账户有没有别的 balance 带 size"。这个上下文只有按天成批
// 处理时才有，所以判定落在这里。
func applyNetSizeKnown(row *eventstore.BalanceSample, event eventlog.Event, sizeLive bool) {
	if !sizeLive {
		row.NetSize = nil
		row.NetSizeKnown = 0
		return
	}
	size := event.Size
	row.NetSize = &size
	row.NetSizeKnown = 1
}

func accumulateCoverage(coverage *Coverage, event eventlog.Event, rows eventstore.Rows) {
	for _, balance := range rows.Balance {
		coverage.BalanceRows++
		if balance.EquityKnown == 1 {
			coverage.EquityKnown++
		}
		if balance.NetSizeKnown == 1 {
			coverage.NetSizeKnown++
		}
	}
	for _, strategy := range rows.Strategy {
		if strategy.Variant == nil {
			coverage.VariantMissing++
		}
		switch event.Event {
		case eventlog.EvOpen, eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip:
			coverage.SignalRows++
			if strategy.GapBp != nil {
				coverage.GapBpPresent++
			}
		case eventlog.EvTrailingClose:
			coverage.TrailingRows++
			if strategy.PeakPct != nil {
				coverage.PeakPctPresent++
			}
		}
		switch event.Event {
		case eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip:
			coverage.GateRows++
			if strategy.GateKind != nil {
				coverage.GateKindResolved++
			}
		}
	}
}

// rowHash 取这批行（Convert 一次只产出一行）的 event_hash 十六进制串。
func rowHash(rows eventstore.Rows) string {
	switch {
	case len(rows.Strategy) == 1:
		return hex.EncodeToString(rows.Strategy[0].EventHash)
	case len(rows.Balance) == 1:
		return hex.EncodeToString(rows.Balance[0].EventHash)
	case len(rows.Dev) == 1:
		return hex.EncodeToString(rows.Dev[0].EventHash)
	default:
		return ""
	}
}

func inDateRange(date, since, until string) bool {
	if since != "" && date < since {
		return false
	}
	if until != "" && date > until {
		return false
	}
	return true
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Totals 汇总一次 Build 的行数，供报告与日志。
func (r *BuildResult) Totals() (rawEvents, unique, duplicates, rows int) {
	for _, day := range r.Days {
		rawEvents += day.RawEvents
		unique += len(day.Hashes)
		duplicates += day.Duplicates
		rows += day.Rows.Len()
	}
	return
}
