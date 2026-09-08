package repository

import (
	"common/middleware/db"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ─── 回测任务 run ────────────────────────────────────────────────────────────

type TradeBacktestRunRepository struct {
	db.Repository[*TradeBacktestRun]
}

func (r *TradeBacktestRunRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeBacktestRun{})
}

// CreateRun 写入一条回测任务（status 初始 pending）。
func (r *TradeBacktestRunRepository) CreateRun(run *TradeBacktestRun) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	run.Init()
	return r.Db.Create(run).Error
}

// UpdateRunStatus 更新任务执行状态（running/done/failed）及失败原因。
func (r *TradeBacktestRunRepository) UpdateRunStatus(id int64, status, errMsg string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeBacktestRun{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"status":       status,
		"error_msg":    errMsg,
		"updated_time": time.Now().UTC(),
	}).Error
}

// UpdateRunKlineInfo 回填本次回放实际使用的 K 线覆盖(根数 + 时间区间)。
func (r *TradeBacktestRunRepository) UpdateRunKlineInfo(id int64, count int, start, end *time.Time) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	updates := map[string]interface{}{
		"kline_count":  count,
		"updated_time": time.Now().UTC(),
	}
	if start != nil && !start.IsZero() {
		updates["kline_start"] = *start
	}
	if end != nil && !end.IsZero() {
		updates["kline_end"] = *end
	}
	return r.Db.Model(&TradeBacktestRun{}).Where("id = ? AND active = 1", id).Updates(updates).Error
}

// UpdateSignalRunResult 回填盘口信号回测的精度信息与真实触发数。
// 精度等级在创建时就按参数算过一遍，这里用回放实测结果覆盖——λ 推导失败、
// dev_sample 覆盖不足这类只有跑完才知道的警示必须落到 run 上。
func (r *TradeBacktestRunRepository) UpdateSignalRunResult(id int64, fidelity, note string, signalCount int) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeBacktestRun{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"fidelity":      fidelity,
		"fidelity_note": note,
		"signal_count":  signalCount,
		"updated_time":  time.Now().UTC(),
	}).Error
}

// FindRunByID 按主键查询任务。
func (r *TradeBacktestRunRepository) FindRunByID(id int64) (*TradeBacktestRun, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row TradeBacktestRun
	err := r.Db.Where("id = ? AND active = 1", id).First(&row).Error
	return &row, err
}

// FindRuns 分页查询任务列表，支持按 symbol/strategyID/engineKind 过滤，按 id 倒序。
// engineKind 为空表示不限（既有页面不传，行为与改造前一致）。
func (r *TradeBacktestRunRepository) FindRuns(symbol string, strategyID int64, engineKind string, page, pageSize int) ([]*TradeBacktestRun, int64, error) {
	if r.Db == nil {
		return nil, 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&TradeBacktestRun{}).Where("active = 1")
	if engineKind != "" {
		q = q.Where("engine_kind = ?", engineKind)
	}
	if symbol != "" {
		q = q.Where("symbol = ?", symbol)
	}
	if strategyID > 0 {
		q = q.Where("strategy_id = ?", strategyID)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	var rows []*TradeBacktestRun
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

// FindRunsByBatch 拉取某次批量扫描下的全部参数组，按 id 升序（= 提交顺序）。
// 排序刻意不按净利：横向对比的排序必须在服务层按 fidelity 分组后再做，
// 仓储层直接给出"哪些组、按提交顺序"的原始集合。
func (r *TradeBacktestRunRepository) FindRunsByBatch(batchID int64) ([]*TradeBacktestRun, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeBacktestRun
	err := r.Db.Where("active = 1 AND batch_id = ?", batchID).Order("id ASC").Find(&rows).Error
	return rows, err
}

// ─── 回测逐笔 trade ──────────────────────────────────────────────────────────

type TradeBacktestTradeRepository struct {
	db.Repository[*TradeBacktestTrade]
}

func (r *TradeBacktestTradeRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeBacktestTrade{})
}

// BatchCreate 批量写入逐笔（回测产出量大，分批落库）。
func (r *TradeBacktestTradeRepository) BatchCreate(rows []*TradeBacktestTrade) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if len(rows) == 0 {
		return nil
	}
	for _, row := range rows {
		row.Init()
	}
	return r.Db.CreateInBatches(rows, 200).Error
}

// FindByRun 拉取某次回测的全部逐笔，按成交/挂单时间排序。
func (r *TradeBacktestTradeRepository) FindByRun(runID int64) ([]*TradeBacktestTrade, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeBacktestTrade
	err := r.Db.Where("active = 1 AND run_id = ?", runID).Order("requested_at ASC").Find(&rows).Error
	return rows, err
}

// ─── 回测汇总 metric ─────────────────────────────────────────────────────────

type TradeBacktestMetricRepository struct {
	db.Repository[*TradeBacktestMetric]
}

func (r *TradeBacktestMetricRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeBacktestMetric{})
}

// UpsertMetrics 按 (run_id, calc_mode) 写入或覆盖汇总指标（一次回测每种口径一行，重跑可覆盖）。
func (r *TradeBacktestMetricRepository) UpsertMetrics(metrics []*TradeBacktestMetric) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if len(metrics) == 0 {
		return nil
	}
	for _, m := range metrics {
		if m.CalcMode == "" {
			m.CalcMode = "prediction"
		}
		m.Init()
	}
	return r.Db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}, {Name: "calc_mode"}},
		UpdateAll: true,
	}).Create(metrics).Error
}

// FindByRuns 拉取一批 run 的汇总指标，供前端横向对比多个策略。
func (r *TradeBacktestMetricRepository) FindByRuns(runIDs []int64) ([]*TradeBacktestMetric, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if len(runIDs) == 0 {
		return nil, nil
	}
	var rows []*TradeBacktestMetric
	err := r.Db.Where("active = 1 AND run_id IN ?", runIDs).Find(&rows).Error
	return rows, err
}

// ─── 参数组批量扫描 batch（r11）───────────────────────────────────────────────

type TradeBacktestBatchRepository struct {
	db.Repository[*TradeBacktestBatch]
}

func (r *TradeBacktestBatchRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeBacktestBatch{})
}

// CreateBatch 写入一条批量扫描（status 初始 pending）。
func (r *TradeBacktestBatchRepository) CreateBatch(batch *TradeBacktestBatch) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	batch.Init()
	return r.Db.Create(batch).Error
}

// FindBatchByID 按主键查询批次。
func (r *TradeBacktestBatchRepository) FindBatchByID(id int64) (*TradeBacktestBatch, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row TradeBacktestBatch
	err := r.Db.Where("id = ? AND active = 1", id).First(&row).Error
	return &row, err
}

// FindBatches 分页查询批次列表，支持按实例/账户过滤，按 id 倒序。
func (r *TradeBacktestBatchRepository) FindBatches(instanceKey, accountLabel string, page, pageSize int) ([]*TradeBacktestBatch, int64, error) {
	if r.Db == nil {
		return nil, 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&TradeBacktestBatch{}).Where("active = 1")
	if instanceKey != "" {
		q = q.Where("instance_key = ?", instanceKey)
	}
	if accountLabel != "" {
		q = q.Where("account_label = ?", accountLabel)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	var rows []*TradeBacktestBatch
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

// UpdateBatchBaselineRun 回填基线组对应的 run_id。
func (r *TradeBacktestBatchRepository) UpdateBatchBaselineRun(id, runID int64) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeBacktestBatch{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"baseline_run_id": runID,
		"updated_time":    time.Now().UTC(),
	}).Error
}

// BumpBatchProgress 原子推进批次进度：done/failed 各 +1（并发组回报用，
// 不读改写，避免多组同时完成时互相覆盖计数）。
func (r *TradeBacktestBatchRepository) BumpBatchProgress(id int64, doneDelta, failedDelta int) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	updates := map[string]interface{}{"updated_time": time.Now().UTC()}
	if doneDelta != 0 {
		updates["done_count"] = gorm.Expr("done_count + ?", doneDelta)
	}
	if failedDelta != 0 {
		updates["failed_count"] = gorm.Expr("failed_count + ?", failedDelta)
	}
	return r.Db.Model(&TradeBacktestBatch{}).Where("id = ? AND active = 1", id).Updates(updates).Error
}

// UpdateBatchStatus 更新批次状态与失败原因。
func (r *TradeBacktestBatchRepository) UpdateBatchStatus(id int64, status, errMsg string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeBacktestBatch{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"status":       status,
		"error_msg":    errMsg,
		"updated_time": time.Now().UTC(),
	}).Error
}
