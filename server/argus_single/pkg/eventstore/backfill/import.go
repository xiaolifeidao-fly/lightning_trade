package backfill

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"argus_single/pkg/eventstore"
)

// defaultBatchSize 单条 INSERT 的行数。回灌是离线批处理，可以比直写
// （200 行）大一档，但仍要让单条 SQL 停在几百 KB 量级。
const defaultBatchSize = 500

// WriteStat 一天的写库结果。
//
// Submitted 是提交给 MySQL 的行数，Existed 是被唯一键吃掉的行数——
// 重跑同一批 JSONL 时 Submitted 不变而 Existed 等于 Submitted，
// 这就是幂等的直接证据。
type WriteStat struct {
	Date      string
	Submitted int
	Inserted  int
	Existed   int
}

// WriteDay 把一天的行写进事件库。
//
// 用 ON DUPLICATE KEY UPDATE id=id：event_hash 撞键即静默跳过，
// 回灌、对账补录、双写期重跑可以任意反复执行。
func WriteDay(db *gorm.DB, day *DayBatch, batchSize int) (WriteStat, error) {
	stat := WriteStat{Date: day.Date}
	if db == nil {
		return stat, fmt.Errorf("backfill: database handle is nil")
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	writes := []struct {
		table string
		count int
		do    func(tx *gorm.DB) *gorm.DB
	}{
		{"strategy_event", len(day.Rows.Strategy), func(tx *gorm.DB) *gorm.DB { return tx.CreateInBatches(day.Rows.Strategy, batchSize) }},
		{"balance_sample", len(day.Rows.Balance), func(tx *gorm.DB) *gorm.DB { return tx.CreateInBatches(day.Rows.Balance, batchSize) }},
		{"dev_sample", len(day.Rows.Dev), func(tx *gorm.DB) *gorm.DB { return tx.CreateInBatches(day.Rows.Dev, batchSize) }},
	}
	for _, write := range writes {
		if write.count == 0 {
			continue
		}
		tx := write.do(db.Clauses(clause.OnConflict{DoNothing: true}))
		if tx.Error != nil {
			return stat, fmt.Errorf("backfill: insert %s for %s: %w", write.table, day.Date, tx.Error)
		}
		stat.Submitted += write.count
		stat.Inserted += int(tx.RowsAffected)
	}
	stat.Existed = stat.Submitted - stat.Inserted
	return stat, nil
}

// EnsureTables 建表/补列。三张表都是 append-only 的事实表，AutoMigrate
// 只会加列加索引，不会动已有数据。
func EnsureTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("backfill: database handle is nil")
	}
	return db.AutoMigrate(eventstore.Models()...)
}
