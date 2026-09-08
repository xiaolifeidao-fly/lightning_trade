package backfill

import (
	"fmt"
	"sort"

	"gorm.io/gorm"
)

// dbRow DB 侧一行的对账投影：只取哈希与来源，不拉业务列。
type dbRow struct {
	Hash   string `gorm:"column:hash"`
	Source int8   `gorm:"column:source"`
}

// TableAudit 单张表在某一天的比对结果。
type TableAudit struct {
	Table      string
	JSONLRows  int
	DBRows     int
	DBLive     int // source=1，进程内直写
	DBBackfill int // source=2，回灌
	Missing    int // JSONL 有、DB 无 —— 双写期真正要盯的那个数
	Extra      int // DB 有、JSONL 无
}

// DayAudit 一天在一个实例上的一致性结论。
type DayAudit struct {
	Date         string
	InstanceKey  string
	Tables       []TableAudit
	MissingTotal int
	ExtraTotal   int
	MissingHash  []string // 缺失哈希样例（最多 SampleLimit 条），供人工回原始 JSONL 定位
}

// SampleLimit 报告里每天最多列几条缺失哈希样例。
const SampleLimit = 5

// OK 当天 JSONL 与 DB 完全对齐。
func (a DayAudit) OK() bool { return a.MissingTotal == 0 && a.ExtraTotal == 0 }

// auditTables 三张事实表，比对顺序固定以保证报告可 diff。
var auditTables = []string{"strategy_event", "balance_sample", "dev_sample"}

// AuditDay 按 event_hash 比对一天的 JSONL 与 DB。
//
// 这是"3 个月双写期比对"的核心：JSONL 是真源，DB 少了什么就是直写路径
// 丢了什么（队列满、DB 短暂不可用、进程被 SIGKILL）。缺口可以直接用
// 同一份代码回灌补上，因为两条路径的 event_hash 同源。
func AuditDay(db *gorm.DB, day *DayBatch) (DayAudit, error) {
	audit := DayAudit{Date: day.Date, InstanceKey: day.InstanceKey}
	if db == nil {
		return audit, fmt.Errorf("backfill: database handle is nil")
	}
	// JSONL 侧按表分组的哈希集合。
	jsonlByTable := map[string]map[string]struct{}{
		"strategy_event": {},
		"balance_sample": {},
		"dev_sample":     {},
	}
	for _, row := range day.Rows.Strategy {
		jsonlByTable["strategy_event"][hexOf(row.EventHash)] = struct{}{}
	}
	for _, row := range day.Rows.Balance {
		jsonlByTable["balance_sample"][hexOf(row.EventHash)] = struct{}{}
	}
	for _, row := range day.Rows.Dev {
		jsonlByTable["dev_sample"][hexOf(row.EventHash)] = struct{}{}
	}

	for _, table := range auditTables {
		expected := jsonlByTable[table]
		rows, err := loadDayHashes(db, table, day.InstanceKey, day.Date)
		if err != nil {
			return audit, err
		}
		result := TableAudit{Table: table, JSONLRows: len(expected), DBRows: len(rows)}
		actual := make(map[string]struct{}, len(rows))
		for _, row := range rows {
			actual[row.Hash] = struct{}{}
			switch row.Source {
			case 1:
				result.DBLive++
			case 2:
				result.DBBackfill++
			}
		}
		for hash := range expected {
			if _, ok := actual[hash]; !ok {
				result.Missing++
				if len(audit.MissingHash) < SampleLimit {
					audit.MissingHash = append(audit.MissingHash, table+":"+hash)
				}
			}
		}
		for hash := range actual {
			if _, ok := expected[hash]; !ok {
				result.Extra++
			}
		}
		audit.MissingTotal += result.Missing
		audit.ExtraTotal += result.Extra
		audit.Tables = append(audit.Tables, result)
	}
	sort.Strings(audit.MissingHash)
	return audit, nil
}

// loadDayHashes 拉一天一实例一张表的全部哈希。
//
// 按 ts 的自然日过滤而不是 ingested_at：JSONL 按事件时刻分文件，
// 只有同一个口径才能逐行对上。
func loadDayHashes(db *gorm.DB, table, instanceKey, date string) ([]dbRow, error) {
	var rows []dbRow
	err := db.Table(table).
		Select("LOWER(HEX(event_hash)) AS hash, source").
		Where("instance_key = ? AND ts >= ? AND ts < DATE_ADD(?, INTERVAL 1 DAY)", instanceKey, date, date).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("backfill: load %s hashes for %s/%s: %w", table, instanceKey, date, err)
	}
	return rows, nil
}

// OrphanDay 库里有整天数据、但本次来源目录里没有对应 JSONL 的日期。
//
// 逐日比对只能覆盖"JSONL 有的天"，覆盖不到"JSONL 整天都没有"这种情况。
// 双写期里这意味着 JSONL 落盘失败或该天的日志没被归档进来——不报出来
// 就会被当成"这几天没交易"。
type OrphanDay struct {
	Date  string
	Table string
	Rows  int
}

// FindOrphanDays 找出 [since, until] 内库里有行、JSONL 却没有的日期。
// since/until 为空时按库里的实际范围查。
func FindOrphanDays(db *gorm.DB, instanceKey string, jsonlDates map[string]struct{}, since, until string) ([]OrphanDay, error) {
	if db == nil {
		return nil, fmt.Errorf("backfill: database handle is nil")
	}
	var orphans []OrphanDay
	for _, table := range auditTables {
		query := db.Table(table).
			Select("DATE_FORMAT(ts, '%Y-%m-%d') AS date, COUNT(*) AS rows_count").
			Where("instance_key = ?", instanceKey).
			Group("date")
		if since != "" {
			query = query.Where("ts >= ?", since)
		}
		if until != "" {
			query = query.Where("ts < DATE_ADD(?, INTERVAL 1 DAY)", until)
		}
		var found []struct {
			Date      string `gorm:"column:date"`
			RowsCount int    `gorm:"column:rows_count"`
		}
		if err := query.Scan(&found).Error; err != nil {
			return nil, fmt.Errorf("backfill: scan %s dates for %s: %w", table, instanceKey, err)
		}
		for _, row := range found {
			if _, ok := jsonlDates[row.Date]; ok {
				continue
			}
			orphans = append(orphans, OrphanDay{Date: row.Date, Table: table, Rows: row.RowsCount})
		}
	}
	sort.Slice(orphans, func(i, j int) bool {
		if orphans[i].Date != orphans[j].Date {
			return orphans[i].Date < orphans[j].Date
		}
		return orphans[i].Table < orphans[j].Table
	})
	return orphans, nil
}

func hexOf(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}
