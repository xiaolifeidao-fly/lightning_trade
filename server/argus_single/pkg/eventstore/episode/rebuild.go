package episode

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
)

// defaultBatchSize 单条 INSERT 的行数，与 backfill 保持一档。
const defaultBatchSize = 500

// minuteCoverage 判定 depth_fidelity=minute 的下限：窗口内的余额心跳里
// 至少这个比例带 upl。留 10% 余量是因为进程重启、心跳漏拍都会造成零星缺口，
// 卡死 100% 会让绝大多数 2026-07-21 之后的 episode 被误标成 mixed。
const minuteCoverage = 0.9

// derivedEvents 参与 episode 派生的事件类型。balance / dev_sample 不在其中：
// balance.size 只有 2026-07-21 之后才有，且"空仓"与"字段缺失"编码相同（§6.1），
// 拿它切 episode 会在上线日之前整段失效；open 事件的 size 则条条都在。
var derivedEvents = []string{
	eventlog.EvOpen,
	eventlog.EvCapSkip,
	eventlog.EvGateBlock,
	eventlog.EvTrendSkip,
	eventlog.EvLossAlert,
	eventlog.EvTrailingClose,
	eventlog.EvCatastropheStop,
	eventlog.EvFixedClose,
	eventlog.EvExternalClose,
	eventlog.EvManualClose,
}

// WriteStat 一次重建的写库结果。
type WriteStat struct {
	DeletedEpisodes int64
	DeletedEntries  int64
	Episodes        int
	Entries         int
}

// EnsureTables 建表/补列。两张都是派生表，可随时整表重建。
func EnsureTables(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("episode: database handle is nil")
	}
	return db.AutoMigrate(Models()...)
}

// LoadEvents 读一个实例的全部持仓相关事件，按 (ts, id) 升序。
//
// 为什么一律整实例重建、不支持按日期切片：episode 会跨天（实测最长 68.8 小时），
// 按日期截一段会把跨界的持仓拦腰砍成两个假 episode。事件量级也不构成理由——
// 实测一个实例三个月的持仓相关事件不到 8 千条。
func LoadEvents(db *gorm.DB, instanceKey string) ([]*eventstore.StrategyEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("episode: database handle is nil")
	}
	var rows []*eventstore.StrategyEvent
	err := db.Where("instance_key = ? AND event IN ?", instanceKey, derivedEvents).
		Order("ts ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("episode: load strategy events for %s: %w", instanceKey, err)
	}
	return rows, nil
}

// Rebuild 重建一个实例的 episode 与决策行：读事实表 → 纯函数派生 → 整实例替换。
//
// rebuiltAt 由调用方传入而不是这里取 time.Now()，是为了让"同样的库跑两次
// 产出同样的行"这条性质可以被集成测试直接断言。
func Rebuild(db *gorm.DB, instanceKey string, rebuiltAt time.Time, batchSize int) ([]*Episode, Stats, WriteStat, error) {
	var stat WriteStat
	events, err := LoadEvents(db, instanceKey)
	if err != nil {
		return nil, Stats{}, stat, err
	}
	episodes, stats := Derive(events, rebuiltAt)
	if err := ApplyDepthFidelity(db, episodes); err != nil {
		return nil, stats, stat, err
	}
	stat, err = Replace(db, instanceKey, episodes, batchSize)
	if err != nil {
		return nil, stats, stat, err
	}
	return episodes, stats, stat, nil
}

// ApplyDepthFidelity 按窗口内余额心跳的 upl 覆盖率给每个 episode 定深度保真度。
//
// 判据用"带 upl 的心跳 / 全部心跳"这个比值而不是绝对条数：心跳周期改过
// （实测约 30 秒一条，不是设计文档写的一分钟），拿绝对条数当分母，改一次
// 采样周期就会让全部历史 episode 的档位集体漂移。
func ApplyDepthFidelity(db *gorm.DB, episodes []*Episode) error {
	if db == nil {
		return fmt.Errorf("episode: database handle is nil")
	}
	type counts struct {
		Total   int `gorm:"column:total"`
		WithUpl int `gorm:"column:with_upl"`
	}
	for _, ep := range episodes {
		var c counts
		err := db.Table("balance_sample").
			Select("COUNT(*) AS total, SUM(CASE WHEN upl IS NULL THEN 0 ELSE 1 END) AS with_upl").
			Where("instance_key = ? AND account_label = ? AND ts BETWEEN ? AND ?",
				ep.InstanceKey, ep.AccountLabel, ep.FirstEventAt, ep.LastEventAt).
			Scan(&c).Error
		if err != nil {
			return fmt.Errorf("episode: count balance samples for %s@%s: %w", ep.AccountLabel, ep.InstanceKey, err)
		}
		ep.BalanceSampleCount = c.Total
		ep.UplSampleCount = c.WithUpl
		ep.DepthFidelity = classifyDepth(c.Total, c.WithUpl)
	}
	return nil
}

// classifyDepth 三档判定（设计文档 §6.4）。没有任何带 upl 的心跳时，水下曲线
// 只能靠 loss_alert 采样，而它 0 ~ −150% 之间完全空白、深水区 5 分钟一采且是
// 下界——必须让前端看得出来，否则拼在一起会给人虚假的精确感。
func classifyDepth(total, withUpl int) string {
	switch {
	case total <= 0 || withUpl <= 0:
		return DepthAlertSampled
	case float64(withUpl) >= float64(total)*minuteCoverage:
		return DepthMinute
	default:
		return DepthMixed
	}
}

// Replace 整实例替换派生行。先删后插、包在一个事务里，因为这两张表是派生物：
// 半旧半新的中间态没有任何解释，比整段缺失更危险。
func Replace(db *gorm.DB, instanceKey string, episodes []*Episode, batchSize int) (WriteStat, error) {
	stat := WriteStat{}
	if db == nil {
		return stat, fmt.Errorf("episode: database handle is nil")
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		deleted := tx.Where("instance_key = ?", instanceKey).Delete(&EpisodeEntry{})
		if deleted.Error != nil {
			return fmt.Errorf("episode: clear episode_entry for %s: %w", instanceKey, deleted.Error)
		}
		stat.DeletedEntries = deleted.RowsAffected
		deleted = tx.Where("instance_key = ?", instanceKey).Delete(&Episode{})
		if deleted.Error != nil {
			return fmt.Errorf("episode: clear episode for %s: %w", instanceKey, deleted.Error)
		}
		stat.DeletedEpisodes = deleted.RowsAffected
		if len(episodes) == 0 {
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).
			CreateInBatches(episodes, batchSize).Error; err != nil {
			return fmt.Errorf("episode: insert episode rows for %s: %w", instanceKey, err)
		}
		stat.Episodes = len(episodes)

		var entries []*EpisodeEntry
		for _, ep := range episodes {
			for _, entry := range ep.Entries {
				entry.EpisodeId = ep.Id
				entries = append(entries, entry)
			}
		}
		if len(entries) == 0 {
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).
			CreateInBatches(entries, batchSize).Error; err != nil {
			return fmt.Errorf("episode: insert episode_entry rows for %s: %w", instanceKey, err)
		}
		stat.Entries = len(entries)
		return nil
	})
	return stat, err
}
