package repository

// 本文件是 episode / episode_entry 两张**派生**表的只读仓储。
//
// 表由 r10（argus_single/pkg/eventstore/episode）拥有并离线重建，本包只读、
// 不写、不重建——重建是 `argus-episode-rebuild` 这条 CLI 的职责，页面上不该
// 存在任何能触发整实例先删后插的入口。
//
// 时间口径与本包其余部分一致：全程按**本地墙钟串**走，理由见包注释第 1 条。
// episode 的四个时间列（first_event_at / opened_at / closed_at / last_event_at）
// 由 argus_single 那条 loc=Local 的连接写入，manager-api 的连接 loc 未知，
// 让 time.Time 过一遍驱动就可能整段偏 8 小时。

import (
	"fmt"
	"strings"

	"argus_single/pkg/eventstore/episode"

	"gorm.io/gorm"
)

// EpisodeRow episode 表的读侧投影。四个时间列是串，理由见文件头。
type EpisodeRow struct {
	Id           uint64 `gorm:"column:id"`
	InstanceKey  string `gorm:"column:instance_key"`
	Uid          string `gorm:"column:uid"`
	AccountLabel string `gorm:"column:account_label"`
	Instrument   string `gorm:"column:instrument"`
	Side         string `gorm:"column:side"`

	FirstEventAt string `gorm:"column:first_event_at"`
	OpenedAt     string `gorm:"column:opened_at"` // 空串 = NULL = 开头被数据窗口截断
	LastEventAt  string `gorm:"column:last_event_at"`
	ClosedAt     string `gorm:"column:closed_at"` // 空串 = 仍持仓
	DurationSec  *int   `gorm:"column:duration_sec"`

	ExitKind             string `gorm:"column:exit_kind"`
	StrategyAttributable uint8  `gorm:"column:strategy_attributable"`

	AddCount       int     `gorm:"column:add_count"`
	ReduceCount    int     `gorm:"column:reduce_count"`
	EntrySizeTotal int     `gorm:"column:entry_size_total"`
	MaxSize        int     `gorm:"column:max_size"`
	OpenSize       float64 `gorm:"column:open_size"`
	HiddenSize     float64 `gorm:"column:hidden_size"`

	MinRoiPctObserved *float64 `gorm:"column:min_roi_pct_observed"`
	MaxRoiPctObserved *float64 `gorm:"column:max_roi_pct_observed"`
	PeakPct           *float64 `gorm:"column:peak_pct"`
	ExitRoiPct        *float64 `gorm:"column:exit_roi_pct"`

	Pnl              float64 `gorm:"column:pnl"`
	PnlStrategy      float64 `gorm:"column:pnl_strategy"`
	UnattributedPnl  float64 `gorm:"column:unattributed_pnl"`
	RealizedEvents   int     `gorm:"column:realized_events"`
	MissingPnlEvents int     `gorm:"column:missing_pnl_events"`

	CapSkipCount       int   `gorm:"column:cap_skip_count"`
	GateBlockCount     int   `gorm:"column:gate_block_count"`
	TrendSkipCount     int   `gorm:"column:trend_skip_count"`
	LossAlertCount     int   `gorm:"column:loss_alert_count"`
	BalanceSampleCount int   `gorm:"column:balance_sample_count"`
	UplSampleCount     int   `gorm:"column:upl_sample_count"`
	PositionGapCount   int   `gorm:"column:position_gap_count"`
	HasPositionGap     uint8 `gorm:"column:has_position_gap"`

	Variant       string `gorm:"column:variant"`
	ConfigVersion uint64 `gorm:"column:config_version"`
	DepthFidelity string `gorm:"column:depth_fidelity"`
	RebuiltAt     string `gorm:"column:rebuilt_at"`
}

// episodeColumns 显式列清单：四个 datetime 走 DATE_FORMAT，NULL 用 IFNULL 落成
// 空串——出参用空串表示"没有这个时刻"，与 opened_at IS NULL（截断头）
// / closed_at IS NULL（仍持仓）两种语义一一对应，前端不必再判 null 与 0 值。
const episodeColumns = "id, instance_key, uid, account_label, instrument, side, " +
	"DATE_FORMAT(first_event_at, '" + TsLayoutSQL + "') AS first_event_at, " +
	"IFNULL(DATE_FORMAT(opened_at, '" + TsLayoutSQL + "'), '') AS opened_at, " +
	"DATE_FORMAT(last_event_at, '" + TsLayoutSQL + "') AS last_event_at, " +
	"IFNULL(DATE_FORMAT(closed_at, '" + TsLayoutSQL + "'), '') AS closed_at, " +
	"duration_sec, IFNULL(exit_kind, '') AS exit_kind, strategy_attributable, " +
	"add_count, reduce_count, entry_size_total, max_size, open_size, hidden_size, " +
	"min_roi_pct_observed, max_roi_pct_observed, peak_pct, exit_roi_pct, " +
	"pnl, pnl_strategy, unattributed_pnl, realized_events, missing_pnl_events, " +
	"cap_skip_count, gate_block_count, trend_skip_count, loss_alert_count, " +
	"balance_sample_count, upl_sample_count, position_gap_count, has_position_gap, " +
	"IFNULL(variant, '') AS variant, config_version, depth_fidelity, " +
	"DATE_FORMAT(rebuilt_at, '" + TsLayoutSQL + "') AS rebuilt_at"

// episode 的三种时间口径。同一批 episode 按不同口径数出来的条数不一样，
// r10 的派生报告已经记过这个差（"只认平仓事件" 21 笔 vs "加上 reduce_to_zero" 29 笔），
// 所以口径必须是显式入参、并随返回体回显，不能在服务端偷偷定一个。
const (
	// EpisodeTimeOpened 按建仓时刻落在窗口内——**决策时刻口径**，与
	// episode_entry 的分层归因一致。截断头（opened_at IS NULL）落选。
	EpisodeTimeOpened = "opened"
	// EpisodeTimeClosed 按出场时刻落在窗口内。仍持仓的 episode 落选。
	EpisodeTimeClosed = "closed"
	// EpisodeTimeOverlap 与窗口有交集即入选，一条都不漏，但同一笔持仓会同时
	// 出现在相邻两天的统计里，计数类指标不能用它。
	EpisodeTimeOverlap = "overlap"
)

// episode 的持仓状态筛选。
const (
	EpisodeStatusAll    = "all"
	EpisodeStatusOpen   = "open"   // closed_at IS NULL
	EpisodeStatusClosed = "closed" // 已出场（六种出场方式之一）
)

// EpisodeFilter 持仓生命周期的筛选条件。复数字段语义都是 IN（空 = 不限）。
type EpisodeFilter struct {
	InstanceKeys   []string
	AccountLabels  []string
	Uids           []string
	Instruments    []string
	Sides          []string
	Variants       []string
	ConfigVersions []uint64
	ExitKinds      []string
	// Status 见 EpisodeStatus* 常量；空串等同 all。
	Status string
	// StrategyOnly 只看计入策略胜率的 episode（剔除 external_close / manual_close
	// 与仍持仓）。任务说明的"不计入策略胜率"就落在这个开关上。
	StrategyOnly bool
	// TimeField 见 EpisodeTime* 常量；空串等同 opened。
	TimeField  string
	Start, End string
}

// EpisodeRepository episode 只读仓储。
type EpisodeRepository struct {
	eventReader
}

// EnsureTable 建两张派生表。
//
// 表的所有者是 r10 的重建 CLI，这里建一次只是让页面在"还没跑过派生"时查出
// 空集而不是报未知表——空集配上返回体里的 rebuild 提示，比一个 500 更容易
// 让人知道该去跑 argus-episode-rebuild。本仓储永不写入这两张表。
func (r *EpisodeRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(episode.Models()...)
}

// scope 把 EpisodeFilter 翻译成 where 链。
func (r *EpisodeRepository) scope(f EpisodeFilter) *gorm.DB {
	q := r.Db.Table(episode.Episode{}.TableName())
	if len(f.InstanceKeys) > 0 {
		q = q.Where("instance_key IN ?", f.InstanceKeys)
	}
	if len(f.AccountLabels) > 0 {
		q = q.Where("account_label IN ?", f.AccountLabels)
	}
	if len(f.Uids) > 0 {
		q = q.Where("uid IN ?", f.Uids)
	}
	if len(f.Instruments) > 0 {
		q = q.Where("instrument IN ?", f.Instruments)
	}
	if len(f.Sides) > 0 {
		q = q.Where("side IN ?", f.Sides)
	}
	if len(f.Variants) > 0 {
		q = q.Where("variant IN ?", f.Variants)
	}
	if len(f.ConfigVersions) > 0 {
		q = q.Where("config_version IN ?", f.ConfigVersions)
	}
	if len(f.ExitKinds) > 0 {
		q = q.Where("exit_kind IN ?", f.ExitKinds)
	}
	switch f.Status {
	case EpisodeStatusOpen:
		q = q.Where("closed_at IS NULL")
	case EpisodeStatusClosed:
		q = q.Where("closed_at IS NOT NULL")
	}
	if f.StrategyOnly {
		q = q.Where("strategy_attributable = 1")
	}
	return r.applyWindow(q, f)
}

// applyWindow 按选定的时间口径收窗口。三种口径的行为差异见 EpisodeTime* 注释。
func (r *EpisodeRepository) applyWindow(q *gorm.DB, f EpisodeFilter) *gorm.DB {
	if f.Start == "" && f.End == "" {
		return q
	}
	switch f.TimeField {
	case EpisodeTimeClosed:
		q = q.Where("closed_at IS NOT NULL")
		if f.Start != "" {
			q = q.Where("closed_at >= ?", f.Start)
		}
		if f.End != "" {
			q = q.Where("closed_at <= ?", f.End)
		}
	case EpisodeTimeOverlap:
		// 有交集：本 episode 的观测起点不晚于窗口右界，且（仍持仓 或 出场不早于左界）。
		if f.End != "" {
			q = q.Where("first_event_at <= ?", f.End)
		}
		if f.Start != "" {
			q = q.Where("(closed_at IS NULL OR closed_at >= ?)", f.Start)
		}
	default: // EpisodeTimeOpened
		q = q.Where("opened_at IS NOT NULL")
		if f.Start != "" {
			q = q.Where("opened_at >= ?", f.Start)
		}
		if f.End != "" {
			q = q.Where("opened_at <= ?", f.End)
		}
	}
	return q
}

// ListEpisodes 按条件分页取 episode。
//
// 定序用 COALESCE(opened_at, first_event_at) 而不是 opened_at：截断头的
// opened_at 是 NULL，直接按它排会把这批 episode 全甩到一端，看上去像"最早的
// 几笔持仓不存在"。limit <= 0 表示不限行数。
func (r *EpisodeRepository) ListEpisodes(f EpisodeFilter, offset, limit int, asc bool) ([]*EpisodeRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	order := "anchor_at DESC, id DESC"
	if asc {
		order = "anchor_at ASC, id ASC"
	}
	q := r.scope(f).
		Select(episodeColumns + ", COALESCE(opened_at, first_event_at) AS anchor_at").
		Order(order)
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []*EpisodeRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountEpisodes 同条件下的总条数，与 ListEpisodes 共用一套 where。
func (r *EpisodeRepository) CountEpisodes(f EpisodeFilter) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var total int64
	err := r.scope(f).Count(&total).Error
	return total, err
}

// FindEpisodeByID 取单条 episode（生命周期抽屉的入口）。
func (r *EpisodeRepository) FindEpisodeByID(id uint64) (*EpisodeRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row EpisodeRow
	err := r.Db.Table(episode.Episode{}.TableName()).
		Select(episodeColumns).Where("id = ?", id).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// EpisodeRebuildRow 派生表的新鲜度探针。
type EpisodeRebuildRow struct {
	EpisodeCount  int64  `gorm:"column:episode_count"`
	LastRebuiltAt string `gorm:"column:last_rebuilt_at"`
	LastEventAt   string `gorm:"column:last_event_at"`
}

// RebuildState 取派生表的条数、最近一次重建时刻，以及派生到的最末事件时刻。
//
// episode 目前只能靠 CLI 手工重建（r10 的遗留项），事件却在持续写入。
// 页面必须能说出"这批 episode 是什么时候派生的、覆盖到哪一刻"，否则用户会把
// 滞后当成"最近没有持仓"。
func (r *EpisodeRepository) RebuildState(instanceKeys []string) (EpisodeRebuildRow, error) {
	var row EpisodeRebuildRow
	if r.Db == nil {
		return row, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Table(episode.Episode{}.TableName()).
		Select("COUNT(*) AS episode_count, " +
			"IFNULL(DATE_FORMAT(MAX(rebuilt_at), '" + TsLayoutSQL + "'), '') AS last_rebuilt_at, " +
			"IFNULL(DATE_FORMAT(MAX(last_event_at), '" + TsLayoutSQL + "'), '') AS last_event_at")
	if len(instanceKeys) > 0 {
		q = q.Where("instance_key IN ?", instanceKeys)
	}
	err := q.Scan(&row).Error
	return row, err
}

// ListExitKindOptions 枚举派生表里真实出现过的出场方式（含仍持仓的空值）。
type ExitKindOptionRow struct {
	ExitKind string `gorm:"column:exit_kind"`
	Count    int64  `gorm:"column:count"`
}

// ListExitKindOptions 出场方式筛选项，按条数降序。
func (r *EpisodeRepository) ListExitKindOptions(instanceKeys []string) ([]*ExitKindOptionRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Table(episode.Episode{}.TableName()).
		Select("IFNULL(exit_kind, '') AS exit_kind, COUNT(*) AS count").
		Group("exit_kind").Order("count DESC")
	if len(instanceKeys) > 0 {
		q = q.Where("instance_key IN ?", instanceKeys)
	}
	var rows []*ExitKindOptionRow
	err := q.Scan(&rows).Error
	return rows, err
}

// ─── episode_entry ──────────────────────────────────────────────────────────

// EpisodeEntryRow 一次建仓决策的读侧投影。
type EpisodeEntryRow struct {
	Id           uint64 `gorm:"column:id"`
	EpisodeId    uint64 `gorm:"column:episode_id"`
	InstanceKey  string `gorm:"column:instance_key"`
	AccountLabel string `gorm:"column:account_label"`
	Instrument   string `gorm:"column:instrument"`
	Side         string `gorm:"column:side"`
	DecidedAt    string `gorm:"column:decided_at"`

	AddedSize  int     `gorm:"column:added_size"`
	OrderSize  *int    `gorm:"column:order_size"`
	ClosedSize float64 `gorm:"column:closed_size"`
	OpenSize   float64 `gorm:"column:open_size"`

	AttributedPnl         float64 `gorm:"column:attributed_pnl"`
	AttributedPnlStrategy float64 `gorm:"column:attributed_pnl_strategy"`
	MissingPnlEvents      int     `gorm:"column:missing_pnl_events"`
	PnlKnown              uint8   `gorm:"column:pnl_known"`

	Variant       string   `gorm:"column:variant"`
	ConfigVersion uint64   `gorm:"column:config_version"`
	GapBp         *float64 `gorm:"column:gap_bp"`
	AvgPx         *float64 `gorm:"column:avg_px"`
	LastPx        *float64 `gorm:"column:last_px"`
	ExitKind      string   `gorm:"column:exit_kind"`
}

const episodeEntryColumns = "id, episode_id, instance_key, account_label, instrument, side, " +
	"DATE_FORMAT(decided_at, '" + TsLayoutSQL + "') AS decided_at, " +
	"added_size, order_size, closed_size, open_size, " +
	"attributed_pnl, attributed_pnl_strategy, missing_pnl_events, pnl_known, " +
	"IFNULL(variant, '') AS variant, config_version, gap_bp, avg_px, last_px, " +
	"IFNULL(exit_kind, '') AS exit_kind"

// EpisodeEntryRepository episode_entry 只读仓储。
type EpisodeEntryRepository struct {
	eventReader
}

// ListByEpisode 取某条 episode 的全部建仓决策，按决策时刻升序。
func (r *EpisodeEntryRepository) ListByEpisode(episodeID uint64) ([]*EpisodeEntryRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*EpisodeEntryRow
	err := r.Db.Table(episode.EpisodeEntry{}.TableName()).
		Select(episodeEntryColumns).
		Where("episode_id = ?", episodeID).
		Order("decided_at ASC, id ASC").
		Scan(&rows).Error
	return rows, err
}

// ─── 聚合 ───────────────────────────────────────────────────────────────────

// EpisodeAggRow 一个分组维度上的 episode 聚合。
//
// 胜率的分子分母都只在 strategy_attributable = 1 的行上取：external_close 与
// manual_close 是交易所侧/人工操作，盈亏如实计入 pnl，但不进胜率分母
// （任务说明的「不做」项）。仍持仓的 episode 结果未定，同样不进分母。
type EpisodeAggRow struct {
	InstanceKey   string   `gorm:"column:instance_key"`
	ConfigVersion uint64   `gorm:"column:config_version"`
	ExitKind      string   `gorm:"column:exit_kind"`
	AccountLabel  string   `gorm:"column:account_label"`
	Variant       string   `gorm:"column:variant"`
	Episodes      int64    `gorm:"column:episodes"`
	Closed        int64    `gorm:"column:closed"`
	StillOpen     int64    `gorm:"column:still_open"`
	Strategy      int64    `gorm:"column:strategy_count"`
	StrategyWins  int64    `gorm:"column:strategy_wins"`
	Pnl           *float64 `gorm:"column:pnl"`
	PnlStrategy   *float64 `gorm:"column:pnl_strategy"`
	Unattributed  *float64 `gorm:"column:unattributed"`
	IncompletePnl int64    `gorm:"column:incomplete_pnl"`
	TruncatedHead int64    `gorm:"column:truncated_head"`
	AvgPeakPct    *float64 `gorm:"column:avg_peak_pct"`
	AvgExitRoiPct *float64 `gorm:"column:avg_exit_roi_pct"`
	MaxSizeSum    int64    `gorm:"column:max_size_sum"`
	FirstAt       string   `gorm:"column:first_at"`
	LastAt        string   `gorm:"column:last_at"`
}

// episodeAggSelect 聚合表达式。分组列由调用方拼在前面，本串只含度量。
const episodeAggSelect = "COUNT(*) AS episodes, " +
	"SUM(CASE WHEN closed_at IS NOT NULL THEN 1 ELSE 0 END) AS closed, " +
	"SUM(CASE WHEN closed_at IS NULL THEN 1 ELSE 0 END) AS still_open, " +
	"SUM(CASE WHEN strategy_attributable = 1 THEN 1 ELSE 0 END) AS strategy_count, " +
	"SUM(CASE WHEN strategy_attributable = 1 AND pnl_strategy > 0 THEN 1 ELSE 0 END) AS strategy_wins, " +
	"SUM(pnl) AS pnl, SUM(pnl_strategy) AS pnl_strategy, SUM(unattributed_pnl) AS unattributed, " +
	"SUM(CASE WHEN missing_pnl_events > 0 THEN 1 ELSE 0 END) AS incomplete_pnl, " +
	"SUM(CASE WHEN opened_at IS NULL THEN 1 ELSE 0 END) AS truncated_head, " +
	"AVG(peak_pct) AS avg_peak_pct, AVG(exit_roi_pct) AS avg_exit_roi_pct, " +
	"SUM(max_size) AS max_size_sum, " +
	"IFNULL(DATE_FORMAT(MIN(COALESCE(opened_at, first_event_at)), '" + TsLayoutSQL + "'), '') AS first_at, " +
	"IFNULL(DATE_FORMAT(MAX(COALESCE(closed_at, last_event_at)), '" + TsLayoutSQL + "'), '') AS last_at"

// AggregateEpisodes 按指定维度聚合。dimensions 只接受本包白名单里的列名，
// 不接受外部拼接的表达式。
func (r *EpisodeRepository) AggregateEpisodes(f EpisodeFilter, dimensions []string) ([]*EpisodeAggRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	cols := make([]string, 0, len(dimensions))
	for _, d := range dimensions {
		switch d {
		case "instance_key", "config_version", "account_label":
			cols = append(cols, d)
		case "exit_kind":
			cols = append(cols, "IFNULL(exit_kind, '') AS exit_kind")
		case "variant":
			cols = append(cols, "IFNULL(variant, '') AS variant")
		default:
			return nil, fmt.Errorf("unsupported episode group dimension: %s", d)
		}
	}
	groupBy := make([]string, 0, len(dimensions))
	groupBy = append(groupBy, dimensions...)

	q := r.scope(f)
	selectExpr := episodeAggSelect
	if len(cols) > 0 {
		selectExpr = strings.Join(cols, ", ") + ", " + episodeAggSelect
		q = q.Group(strings.Join(groupBy, ", "))
	}
	var rows []*EpisodeAggRow
	err := q.Select(selectExpr).Scan(&rows).Error
	return rows, err
}
