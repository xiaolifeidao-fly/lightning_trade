// Package repository 是 argus_event 的读侧数据访问层。
//
// 它读的是 argus_single 双写进来的三张 append-only 事实表
// （strategy_event / balance_sample / dev_sample，模型见
// argus_single/pkg/eventstore/model.go），表结构由写侧（r4）拥有；
// 外加 r3 的 signal_slice——那张表是 90 天滚动的派生切片，不是真源，
// 查不到切片是正常态（超过保留期或采集器还没上线），读侧必须能降级。
//
// 三条本包特有的约定，改代码前先读：
//
//  1. **时间一律按串走**。事件 ts 是 DATETIME（秒精度），argus_single 侧的
//     连接强制 `loc=Local`（eventstore.NormalizeDSN），而 manager-api 的
//     `sqlconn` 由部署方给，仓库里看不到、也不保证带 loc=Local——go-sql-driver
//     默认是 UTC。若把 time.Time 当绑定参数发出去、再把 DATETIME 读回
//     time.Time，两侧 loc 不一致就会整体偏移 8 小时：数据全在、图能画、
//     只是整段错位，正是需求大纲反复点名的那类隐蔽错误。
//     因此本包的时间**入参是本地墙钟串、出参是 DATE_FORMAT 出来的串**，
//     全程不经过 time.Time，读到的字符与 JSONL / Telegram 消息逐字一致。
//
//  2. **不带 active = 1**。事实表不嵌 db.BaseEntity，没有 active 列。
//
//  3. **不做分钟级去重或聚合**。实测 28.7% 的信号分钟内触发 ≥2 次、单分钟
//     最多 5 次，任何"同分钟折叠"都会丢掉约一半触发。排序统一
//     (ts, id)，同秒多条靠 id 定序保证分页可复现。
package repository

import (
	"fmt"
	"strings"

	"argus_single/pkg/eventstore"

	"gorm.io/gorm"
)

// TsLayoutSQL DATE_FORMAT 的格式串，产出与 eventlog.TsLayout 逐字一致的秒精度串。
const TsLayoutSQL = "%Y-%m-%d %H:%i:%s"

// eventReader 只提供 Db 与 SetDb。事实表不实现 db.Entity 的 Init()，
// 套不进 db.Repository[T] 泛型基类（与 service/trade 读侧同因）。
type eventReader struct {
	Db *gorm.DB
}

// SetDb 供 db.GetRepository 注入全局连接。
func (r *eventReader) SetDb(d *gorm.DB) { r.Db = d }

// ─── strategy_event ─────────────────────────────────────────────────────────

// StrategyEventRow strategy_event 的读侧投影。ts 是串，理由见包注释。
type StrategyEventRow struct {
	Id            uint64   `gorm:"column:id"`
	Ts            string   `gorm:"column:ts"`
	InstanceKey   string   `gorm:"column:instance_key"`
	ConfigVersion uint64   `gorm:"column:config_version"`
	Uid           string   `gorm:"column:uid"`
	AccountLabel  string   `gorm:"column:account_label"`
	Variant       *string  `gorm:"column:variant"`
	Event         string   `gorm:"column:event"`
	Instrument    string   `gorm:"column:instrument"`
	InstIdRaw     *string  `gorm:"column:inst_id_raw"`
	Side          *string  `gorm:"column:side"`
	NetSide       *string  `gorm:"column:net_side"`
	Size          *int     `gorm:"column:size"`
	OrderSize     *int     `gorm:"column:order_size"`
	AvgPx         *float64 `gorm:"column:avg_px"`
	LastPx        *float64 `gorm:"column:last_px"`
	RoiPct        *float64 `gorm:"column:roi_pct"`
	Pnl           *float64 `gorm:"column:pnl"`
	PeakPct       *float64 `gorm:"column:peak_pct"`
	SigLast       *float64 `gorm:"column:sig_last"`
	SigMark       *float64 `gorm:"column:sig_mark"`
	GapBp         *float64 `gorm:"column:gap_bp"`
	TrendMomPct   *float64 `gorm:"column:trend_mom_pct"`
	GateKind      *string  `gorm:"column:gate_kind"`
	GateThreshold *float64 `gorm:"column:gate_threshold"`
	GateActual    *float64 `gorm:"column:gate_actual"`
	Reason        *string  `gorm:"column:reason"`
	Source        int8     `gorm:"column:source"`
}

// strategyEventColumns 显式列清单：ts 走 DATE_FORMAT，其余原样。
const strategyEventColumns = "id, DATE_FORMAT(ts, '" + TsLayoutSQL + "') AS ts, instance_key, config_version, uid, " +
	"account_label, variant, event, instrument, inst_id_raw, side, net_side, size, order_size, " +
	"avg_px, last_px, roi_pct, pnl, peak_pct, sig_last, sig_mark, gap_bp, trend_mom_pct, " +
	"gate_kind, gate_threshold, gate_actual, reason, source"

// EventFilter 信号流的多维筛选条件。
//
// 所有复数字段语义都是 IN（空 = 不限）；Start/End 是闭区间的本地墙钟串。
// InstanceKeys 为空表示"全部实例"——允许，但调用方必须把 instance_key 带回
// 前端，且不得把不同实例的同名账户合并（实例1 的 account1 与实例3 的
// account1 是两个人）。
type EventFilter struct {
	InstanceKeys   []string
	Start          string
	End            string
	Instruments    []string
	AccountLabels  []string
	Uids           []string
	Events         []string
	GateKinds      []string
	Variants       []string
	ConfigVersions []uint64
	Sides          []string
	Sources        []int8
	// GapBpMinAbs / GapBpMaxAbs 按 |gap_bp| 过滤（信号强度分级）。
	GapBpMinAbs *float64
	GapBpMaxAbs *float64
}

// StrategyEventRepository strategy_event 只读仓储。
type StrategyEventRepository struct {
	eventReader
}

// EnsureTable 建表。写侧（argus_single 进程）拥有并 AutoMigrate 同一组表；
// 管理端先起来时这里也建一次，好让四个页面在事件还没进来前查出空集，
// 而不是报未知表。balance_sample 此前没有任何一侧在管理端建过（回测只用了
// strategy_event / dev_sample），权益曲线依赖它，所以这里一起建。
// eventstore.Models() 已含 r3 的 signal_slice，清单只有写侧一份，不在这里重复枚举。
func (r *StrategyEventRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(eventstore.Models()...)
}

// scope 把 EventFilter 翻译成 where 链。
func (r *StrategyEventRepository) scope(f EventFilter) *gorm.DB {
	q := r.Db.Table(eventstore.StrategyEvent{}.TableName())
	if len(f.InstanceKeys) > 0 {
		q = q.Where("instance_key IN ?", f.InstanceKeys)
	}
	if f.Start != "" {
		q = q.Where("ts >= ?", f.Start)
	}
	if f.End != "" {
		q = q.Where("ts <= ?", f.End)
	}
	if len(f.Instruments) > 0 {
		q = q.Where("instrument IN ?", f.Instruments)
	}
	if len(f.AccountLabels) > 0 {
		q = q.Where("account_label IN ?", f.AccountLabels)
	}
	if len(f.Uids) > 0 {
		q = q.Where("uid IN ?", f.Uids)
	}
	if len(f.Events) > 0 {
		q = q.Where("event IN ?", f.Events)
	}
	if len(f.GateKinds) > 0 {
		q = q.Where("gate_kind IN ?", f.GateKinds)
	}
	if len(f.Variants) > 0 {
		q = q.Where("variant IN ?", f.Variants)
	}
	if len(f.ConfigVersions) > 0 {
		q = q.Where("config_version IN ?", f.ConfigVersions)
	}
	if len(f.Sides) > 0 {
		q = q.Where("side IN ?", f.Sides)
	}
	if len(f.Sources) > 0 {
		q = q.Where("source IN ?", f.Sources)
	}
	// gap_bp 带符号（>0=UP），强度看绝对值；NULL（报价不可算）在给了强度
	// 条件时一律排除，不能当成 0 混进"弱"档。
	if f.GapBpMinAbs != nil {
		q = q.Where("gap_bp IS NOT NULL AND ABS(gap_bp) >= ?", *f.GapBpMinAbs)
	}
	if f.GapBpMaxAbs != nil {
		q = q.Where("gap_bp IS NOT NULL AND ABS(gap_bp) < ?", *f.GapBpMaxAbs)
	}
	return q
}

// ListEvents 按条件分页取事件，(ts, id) 定序。limit <= 0 表示不限行数。
func (r *StrategyEventRepository) ListEvents(f EventFilter, offset, limit int, asc bool) ([]*StrategyEventRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	order := "ts DESC, id DESC"
	if asc {
		order = "ts ASC, id ASC"
	}
	q := r.scope(f).Select(strategyEventColumns).Order(order)
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []*StrategyEventRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountEvents 同条件下的总行数，与 ListEvents 共用一套 where。
func (r *StrategyEventRepository) CountEvents(f EventFilter) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var total int64
	err := r.scope(f).Count(&total).Error
	return total, err
}

// FindEventByID 取单条事件（信号详情的锚点）。
func (r *StrategyEventRepository) FindEventByID(id uint64) (*StrategyEventRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row StrategyEventRow
	err := r.Db.Table(eventstore.StrategyEvent{}.TableName()).
		Select(strategyEventColumns).Where("id = ?", id).Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// TimeRangeRow 一次 MIN/MAX 扫描的结果。
type TimeRangeRow struct {
	FirstTs string `gorm:"column:first_ts"`
	LastTs  string `gorm:"column:last_ts"`
}

// EventTimeRange 取同条件下事件的时间覆盖区间。
// 窗口未显式给出时，服务用它把默认窗口锚在**已入库数据的最新时刻**上——
// 历史回灌库里 now-24h 永远是空的，按服务器当前时间兜底等于给张白页。
func (r *StrategyEventRepository) EventTimeRange(f EventFilter) (TimeRangeRow, error) {
	var row TimeRangeRow
	if r.Db == nil {
		return row, fmt.Errorf("database is not initialized")
	}
	err := r.scope(f).Select(
		"DATE_FORMAT(MIN(ts), '" + TsLayoutSQL + "') AS first_ts, " +
			"DATE_FORMAT(MAX(ts), '" + TsLayoutSQL + "') AS last_ts").Scan(&row).Error
	return row, err
}

// InstanceOptionRow 有事件的实例及覆盖区间。
type InstanceOptionRow struct {
	InstanceKey string `gorm:"column:instance_key"`
	EventCount  int64  `gorm:"column:event_count"`
	FirstTs     string `gorm:"column:first_ts"`
	LastTs      string `gorm:"column:last_ts"`
}

// ListInstanceOptions 枚举 strategy_event 里真实出现过的实例。
func (r *StrategyEventRepository) ListInstanceOptions() ([]*InstanceOptionRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*InstanceOptionRow
	err := r.Db.Table(eventstore.StrategyEvent{}.TableName()).
		Select("instance_key, COUNT(*) AS event_count, " +
			"DATE_FORMAT(MIN(ts), '" + TsLayoutSQL + "') AS first_ts, " +
			"DATE_FORMAT(MAX(ts), '" + TsLayoutSQL + "') AS last_ts").
		Group("instance_key").Order("instance_key ASC").Scan(&rows).Error
	return rows, err
}

// AccountOptionRow 账户维度的筛选项。归集键是 (instance_key, account_label)——
// 只按账户名会串实例。
type AccountOptionRow struct {
	InstanceKey  string `gorm:"column:instance_key"`
	AccountLabel string `gorm:"column:account_label"`
	Uid          string `gorm:"column:uid"`
	Variant      string `gorm:"column:variant"`
	Instrument   string `gorm:"column:instrument"`
	EventCount   int64  `gorm:"column:event_count"`
	FirstTs      string `gorm:"column:first_ts"`
	LastTs       string `gorm:"column:last_ts"`
}

// ListAccountOptions 枚举实例 × 账户 × 合约。variant 取窗口内最大值（同一账户
// 换过变体时只做展示提示，筛选仍走独立的 variants 维度）。
func (r *StrategyEventRepository) ListAccountOptions(instanceKeys []string) ([]*AccountOptionRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Table(eventstore.StrategyEvent{}.TableName()).
		Select("instance_key, account_label, MAX(uid) AS uid, MAX(variant) AS variant, instrument, " +
			"COUNT(*) AS event_count, " +
			"DATE_FORMAT(MIN(ts), '" + TsLayoutSQL + "') AS first_ts, " +
			"DATE_FORMAT(MAX(ts), '" + TsLayoutSQL + "') AS last_ts").
		Group("instance_key, account_label, instrument").
		Order("instance_key ASC, account_label ASC")
	if len(instanceKeys) > 0 {
		q = q.Where("instance_key IN ?", instanceKeys)
	}
	var rows []*AccountOptionRow
	err := q.Scan(&rows).Error
	return rows, err
}

// ConfigVersionOptionRow 参数版本筛选项。
type ConfigVersionOptionRow struct {
	InstanceKey   string `gorm:"column:instance_key"`
	ConfigVersion uint64 `gorm:"column:config_version"`
	EventCount    int64  `gorm:"column:event_count"`
	FirstTs       string `gorm:"column:first_ts"`
	LastTs        string `gorm:"column:last_ts"`
}

// ListConfigVersionOptions 枚举实例 × 参数版本。
func (r *StrategyEventRepository) ListConfigVersionOptions(instanceKeys []string) ([]*ConfigVersionOptionRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Table(eventstore.StrategyEvent{}.TableName()).
		Select("instance_key, config_version, COUNT(*) AS event_count, " +
			"DATE_FORMAT(MIN(ts), '" + TsLayoutSQL + "') AS first_ts, " +
			"DATE_FORMAT(MAX(ts), '" + TsLayoutSQL + "') AS last_ts").
		Group("instance_key, config_version").
		Order("instance_key ASC, config_version DESC")
	if len(instanceKeys) > 0 {
		q = q.Where("instance_key IN ?", instanceKeys)
	}
	var rows []*ConfigVersionOptionRow
	err := q.Scan(&rows).Error
	return rows, err
}

// ListDistinct 取某一列的去重值（instrument / variant）。列名由本包内部给定，
// 不接受外部拼接。
func (r *StrategyEventRepository) ListDistinct(column string, instanceKeys []string) ([]string, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	switch column {
	case "instrument", "variant":
	default:
		return nil, fmt.Errorf("unsupported distinct column: %s", column)
	}
	q := r.Db.Table(eventstore.StrategyEvent{}.TableName()).
		Distinct(column).Where(column + " IS NOT NULL AND " + column + " <> ''").Order(column + " ASC")
	if len(instanceKeys) > 0 {
		q = q.Where("instance_key IN ?", instanceKeys)
	}
	var values []string
	err := q.Pluck(column, &values).Error
	return values, err
}

// ─── balance_sample ─────────────────────────────────────────────────────────

// BalanceFilter 权益曲线的查询条件。
type BalanceFilter struct {
	InstanceKeys  []string
	AccountLabels []string
	Start         string
	End           string
}

// BalanceBucketRow 一个降采样桶内的权益聚合。
type BalanceBucketRow struct {
	InstanceKey  string   `gorm:"column:instance_key"`
	AccountLabel string   `gorm:"column:account_label"`
	BucketTs     string   `gorm:"column:bucket_ts"`
	Samples      int      `gorm:"column:samples"`
	AvgBalance   *float64 `gorm:"column:avg_balance"`
	AvgEquity    *float64 `gorm:"column:avg_equity"`
	MinEquity    *float64 `gorm:"column:min_equity"`
	MaxEquity    *float64 `gorm:"column:max_equity"`
	AvgUpl       *float64 `gorm:"column:avg_upl"`
}

// BalanceSampleRepository balance_sample 只读仓储。
type BalanceSampleRepository struct {
	eventReader
}

func (r *BalanceSampleRepository) scope(f BalanceFilter) *gorm.DB {
	q := r.Db.Table(eventstore.BalanceSample{}.TableName())
	if len(f.InstanceKeys) > 0 {
		q = q.Where("instance_key IN ?", f.InstanceKeys)
	}
	if len(f.AccountLabels) > 0 {
		q = q.Where("account_label IN ?", f.AccountLabels)
	}
	if f.Start != "" {
		q = q.Where("ts >= ?", f.Start)
	}
	if f.End != "" {
		q = q.Where("ts <= ?", f.End)
	}
	return q
}

// ListBuckets 按 bucketSeconds 降采样取权益曲线。
//
// 桶键用 FROM_UNIXTIME(FLOOR(UNIX_TIMESTAMP(ts)/N)*N)：两个函数走同一个会话
// 时区，来回抵消，桶边界落在**存储的墙钟**网格上，与连接的 loc 设置无关。
// equity 只在 equity_known = 1 的样本上聚合——0 与负权益是真实的极端回撤样本，
// 不能靠 "> 0" 过滤掉，也不能把未知当 0 拉低均值。
func (r *BalanceSampleRepository) ListBuckets(f BalanceFilter, bucketSeconds int) ([]*BalanceBucketRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if bucketSeconds <= 0 {
		bucketSeconds = 600
	}
	bucketExpr := fmt.Sprintf("FROM_UNIXTIME(FLOOR(UNIX_TIMESTAMP(ts)/%d)*%d, '%s')", bucketSeconds, bucketSeconds, TsLayoutSQL)
	var rows []*BalanceBucketRow
	err := r.scope(f).
		Select(bucketExpr + " AS bucket_ts, instance_key, account_label, COUNT(*) AS samples, " +
			"AVG(balance) AS avg_balance, " +
			"AVG(CASE WHEN equity_known = 1 THEN equity END) AS avg_equity, " +
			"MIN(CASE WHEN equity_known = 1 THEN equity END) AS min_equity, " +
			"MAX(CASE WHEN equity_known = 1 THEN equity END) AS max_equity, " +
			"AVG(CASE WHEN equity_known = 1 THEN upl END) AS avg_upl").
		Group("instance_key, account_label, bucket_ts").
		Order("instance_key ASC, account_label ASC, bucket_ts ASC").
		Scan(&rows).Error
	return rows, err
}

// BalancePointRow 一条原始心跳（不降采样）。episode 生命周期抽屉用它画浮盈
// 轨迹：单条持仓的窗口只有小时量级、心跳实测约 30 秒一条，直接取原始点比
// 降采样更能看清扛单过程，也不会把回撤的谷底平均掉。
type BalancePointRow struct {
	Ts           string   `gorm:"column:ts"`
	Balance      *float64 `gorm:"column:balance"`
	Equity       *float64 `gorm:"column:equity"`
	Upl          *float64 `gorm:"column:upl"`
	EquityKnown  uint8    `gorm:"column:equity_known"`
	NetSize      *int     `gorm:"column:net_size"`
	NetSizeKnown uint8    `gorm:"column:net_size_known"`
}

// ListPoints 取窗口内的原始心跳，按 ts 升序。limit <= 0 表示不限；
// 命中 limit 时调用方要自曝截断，不能把半截曲线当成完整轨迹画出去。
func (r *BalanceSampleRepository) ListPoints(f BalanceFilter, limit int) ([]*BalancePointRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.scope(f).
		Select("DATE_FORMAT(ts, '" + TsLayoutSQL + "') AS ts, balance, " +
			"CASE WHEN equity_known = 1 THEN equity END AS equity, " +
			"CASE WHEN equity_known = 1 THEN upl END AS upl, " +
			"equity_known, " +
			"CASE WHEN net_size_known = 1 THEN net_size END AS net_size, net_size_known").
		Order("ts ASC, id ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []*BalancePointRow
	err := q.Scan(&rows).Error
	return rows, err
}

// BalanceLatestRow 某账户在窗口内的最后一条心跳。
type BalanceLatestRow struct {
	InstanceKey  string   `gorm:"column:instance_key"`
	AccountLabel string   `gorm:"column:account_label"`
	Uid          string   `gorm:"column:uid"`
	Variant      string   `gorm:"column:variant"`
	Ts           string   `gorm:"column:ts"`
	Balance      *float64 `gorm:"column:balance"`
	Equity       *float64 `gorm:"column:equity"`
	Upl          *float64 `gorm:"column:upl"`
	EquityKnown  uint8    `gorm:"column:equity_known"`
	NetSize      *int     `gorm:"column:net_size"`
	NetSizeKnown uint8    `gorm:"column:net_size_known"`
}

// ListLatestByAccount 取每个 (instance_key, account_label) 在窗口内的最后一条心跳，
// 供总览与跨实例对比展示"当前权益 / 当前净仓"。
func (r *BalanceSampleRepository) ListLatestByAccount(f BalanceFilter) ([]*BalanceLatestRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	// 先在同一套 where 上求每个账户的 MAX(ts)，再回表取那一行；同秒多行时
	// 用 MAX(id) 收口，保证结果唯一。
	sub := r.scope(f).Select("instance_key, account_label, MAX(ts) AS max_ts").
		Group("instance_key, account_label")
	var rows []*BalanceLatestRow
	err := r.Db.Table(eventstore.BalanceSample{}.TableName()+" AS b").
		Joins("JOIN (?) AS m ON m.instance_key = b.instance_key AND m.account_label = b.account_label AND m.max_ts = b.ts", sub).
		Select("b.instance_key, b.account_label, MAX(b.uid) AS uid, MAX(b.variant) AS variant, " +
			"DATE_FORMAT(MAX(b.ts), '" + TsLayoutSQL + "') AS ts, " +
			"MAX(b.balance) AS balance, MAX(CASE WHEN b.equity_known = 1 THEN b.equity END) AS equity, " +
			"MAX(CASE WHEN b.equity_known = 1 THEN b.upl END) AS upl, " +
			"MAX(b.equity_known) AS equity_known, " +
			"MAX(CASE WHEN b.net_size_known = 1 THEN b.net_size END) AS net_size, " +
			"MAX(b.net_size_known) AS net_size_known").
		Group("b.instance_key, b.account_label").
		Order("b.instance_key ASC, b.account_label ASC").
		Scan(&rows).Error
	return rows, err
}

// ─── dev_sample ─────────────────────────────────────────────────────────────

// DevSampleRow 无条件偏离采样窗口的读侧投影。
type DevSampleRow struct {
	Ts        string   `gorm:"column:ts"`
	DevTicks  int      `gorm:"column:dev_ticks"`
	DevMaxBp  *float64 `gorm:"column:dev_max_bp"`
	DevMeanBp *float64 `gorm:"column:dev_mean_bp"`
}

// DevSampleRepository dev_sample 只读仓储。
type DevSampleRepository struct {
	eventReader
}

// ListSamples 取区间内的采样窗口，按 ts 升序。切片抽屉用它给出"这一分钟里
// 偏离到底有多大"的无条件参照（不受生产阈值截断）。
func (r *DevSampleRepository) ListSamples(instanceKey, instrument, start, end string) ([]*DevSampleRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Table(eventstore.DevSample{}.TableName()).
		Select("DATE_FORMAT(ts, '" + TsLayoutSQL + "') AS ts, dev_ticks, dev_max_bp, dev_mean_bp").
		Order("ts ASC, id ASC")
	if key := strings.TrimSpace(instanceKey); key != "" {
		q = q.Where("instance_key = ?", key)
	}
	if inst := strings.TrimSpace(instrument); inst != "" {
		q = q.Where("instrument = ?", strings.ToUpper(inst))
	}
	if start != "" {
		q = q.Where("ts >= ?", start)
	}
	if end != "" {
		q = q.Where("ts <= ?", end)
	}
	var rows []*DevSampleRow
	err := q.Scan(&rows).Error
	return rows, err
}

// ─── signal_slice ───────────────────────────────────────────────────────────

// SignalSliceRow 触发瞬间秒级切片的读侧投影。三条序列原样带出 JSON 串，
// 解析放在 service 层——本包不认识 *float64 序列的业务语义。
type SignalSliceRow struct {
	Ts            string `gorm:"column:ts"`
	InstanceKey   string `gorm:"column:instance_key"`
	ConfigVersion uint64 `gorm:"column:config_version"`
	Instrument    string `gorm:"column:instrument"`
	StartAt       string `gorm:"column:start_at"`
	EndAt         string `gorm:"column:end_at"`
	HalfWindowSec int    `gorm:"column:half_window_sec"`
	SeriesPoints  int    `gorm:"column:series_points"`
	DcPoints      int    `gorm:"column:dc_points"`
	BinPoints     int    `gorm:"column:bin_points"`
	DcLastJson    string `gorm:"column:dc_last_json"`
	DcMarkJson    string `gorm:"column:dc_mark_json"`
	BinLastJson   string `gorm:"column:bin_last_json"`
}

const signalSliceColumns = "DATE_FORMAT(ts, '" + TsLayoutSQL + "') AS ts, instance_key, config_version, instrument, " +
	"DATE_FORMAT(start_at, '" + TsLayoutSQL + "') AS start_at, DATE_FORMAT(end_at, '" + TsLayoutSQL + "') AS end_at, " +
	"half_window_sec, series_points, dc_points, bin_points, dc_last_json, dc_mark_json, bin_last_json"

// SignalSliceRepository signal_slice 只读仓储（写侧是 argus_single 的 r3 采集器）。
type SignalSliceRepository struct {
	eventReader
}

// FindNearestAnchor 取一条最贴近某个事件时刻的切片。
//
// 为什么不能按 ts 精确相等去取：切片锚在**偏离穿越那一秒**，而事件 ts 是
// 判定/成交落地的时刻——中间隔着 trade.signal.delay_seconds（默认 5 秒）的
// 延迟调度，成交类还要再加一次下单往返。实测拦截类比锚点晚数秒、成交类更晚
// （service.go 的 signalGroupWindowSec 就是为同一现象设的 30 秒归组窗口）。
//
// 因此这里取"锚点不晚于 eventTs+grace 的最近一条，且回看不超过 lookback 秒"。
// 已知取舍：同一簇里 20 秒内连着两次触发时，晚到的成交事件可能被归到后一个
// 锚点上。这在按 ts 单值关联下无法根除（事件行里没有锚点字段），但两条切片
// 的窗口高度重叠，图形结论不受影响；真要精确关联得等埋点带上锚点。
func (r *SignalSliceRepository) FindNearestAnchor(instanceKey, instrument, eventTs string, lookbackSec, graceSec int) (*SignalSliceRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	key := strings.TrimSpace(instanceKey)
	if key == "" || strings.TrimSpace(eventTs) == "" {
		return nil, nil
	}
	q := r.Db.Table(eventstore.SignalSlice{}.TableName()).
		Select(signalSliceColumns).
		Where("instance_key = ?", key).
		Where("ts >= DATE_SUB(?, INTERVAL ? SECOND)", eventTs, lookbackSec).
		Where("ts <= DATE_ADD(?, INTERVAL ? SECOND)", eventTs, graceSec).
		Order("ts DESC").
		Limit(1)
	if inst := strings.TrimSpace(instrument); inst != "" {
		q = q.Where("instrument = ?", strings.ToUpper(inst))
	}
	var rows []*SignalSliceRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}
