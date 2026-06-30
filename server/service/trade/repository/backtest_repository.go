package repository

import (
	"common/middleware/db"
	"fmt"
	"time"

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

// FindRunByID 按主键查询任务。
func (r *TradeBacktestRunRepository) FindRunByID(id int64) (*TradeBacktestRun, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row TradeBacktestRun
	err := r.Db.Where("id = ? AND active = 1", id).First(&row).Error
	return &row, err
}

// FindRuns 分页查询任务列表，支持按 symbol/strategyID 过滤，按 id 倒序。
func (r *TradeBacktestRunRepository) FindRuns(symbol string, strategyID int64, page, pageSize int) ([]*TradeBacktestRun, int64, error) {
	if r.Db == nil {
		return nil, 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&TradeBacktestRun{}).Where("active = 1")
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
