package repository

import (
	"common/middleware/db"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TradeStrategyPositionRepository struct {
	db.Repository[*TradeStrategyPosition]
}

func (r *TradeStrategyPositionRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeStrategyPosition{})
}

// CreatePosition 写入一条新持仓记录（status=open）。
func (r *TradeStrategyPositionRepository) CreatePosition(pos *TradeStrategyPosition) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	pos.Init()
	return r.Db.Create(pos).Error
}

// FindOpenPositions 返回所有 status=open 的持仓，供监测循环轮询。
func (r *TradeStrategyPositionRepository) FindOpenPositions() ([]*TradeStrategyPosition, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeStrategyPosition
	err := r.Db.Where("active = 1 AND status = 'open'").Order("id ASC").Find(&rows).Error
	return rows, err
}

// ClosePositionTx 事务平仓：用 SELECT FOR UPDATE 锁行，确保并发时只有一次平仓成功。
// 若行已不是 open 状态，返回 ErrRecordNotFound，调用方忽略即可（幂等）。
func (r *TradeStrategyPositionRepository) ClosePositionTx(
	id int,
	closePrice float64,
	closeReason string,
	closedAt time.Time,
	pnl, pnlRate, fee, netPnl float64,
) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Transaction(func(tx *gorm.DB) error {
		var pos TradeStrategyPosition
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND status = 'open' AND active = 1", id).
			First(&pos).Error; err != nil {
			return err // ErrRecordNotFound = 已平仓或不存在，调用方忽略
		}
		return tx.Model(&TradeStrategyPosition{}).Where("id = ?", id).Updates(map[string]interface{}{
			"status":        "closed",
			"close_price":   closePrice,
			"close_reason":  closeReason,
			"closed_at":     closedAt,
			"pnl":           pnl,
			"pnl_rate":      pnlRate,
			"fee":           fee,
			"net_pnl":       netPnl,
			"updated_time":  closedAt,
		}).Error
	})
}

// FindByID 按主键查询持仓记录。
func (r *TradeStrategyPositionRepository) FindByID(id int64) (*TradeStrategyPosition, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row TradeStrategyPosition
	err := r.Db.Where("id = ? AND active = 1", id).First(&row).Error
	return &row, err
}

// FindByQuery 分页查询持仓列表，支持 strategyID/symbol/status/时间范围过滤，按 opened_at DESC。
func (r *TradeStrategyPositionRepository) FindByQuery(
	strategyID int64,
	symbol, status string,
	startTime, endTime *time.Time,
	page, pageSize int,
) ([]*TradeStrategyPosition, int64, error) {
	if r.Db == nil {
		return nil, 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&TradeStrategyPosition{}).Where("active = 1")
	if strategyID > 0 {
		q = q.Where("strategy_id = ?", strategyID)
	}
	if symbol != "" {
		q = q.Where("symbol = ?", symbol)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if startTime != nil {
		q = q.Where("opened_at >= ?", *startTime)
	}
	if endTime != nil {
		q = q.Where("opened_at <= ?", *endTime)
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
	var rows []*TradeStrategyPosition
	err := q.Order("opened_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

// PositionSummaryResult SQL 聚合扫描目标。
type PositionSummaryResult struct {
	TotalOpens      int64    `gorm:"column:total_opens"`
	CurrentOpen     int64    `gorm:"column:current_open"`
	TotalClosed     int64    `gorm:"column:total_closed"`
	CumNetPnl       *float64 `gorm:"column:cum_net_pnl"`
	WinCount        int64    `gorm:"column:win_count"`
	AvgHoldSeconds  *float64 `gorm:"column:avg_hold_seconds"`
	MaxWin          *float64 `gorm:"column:max_win"`
	MaxLoss         *float64 `gorm:"column:max_loss"`
	TpCount         int64    `gorm:"column:tp_count"`
	SlCount         int64    `gorm:"column:sl_count"`
	TimeoutCount    int64    `gorm:"column:timeout_count"`
	ManualCount     int64    `gorm:"column:manual_count"`
}

// GetSummary 用单条聚合 SQL 返回持仓汇总统计。
func (r *TradeStrategyPositionRepository) GetSummary(
	strategyID int64,
	symbol string,
	startTime, endTime *time.Time,
) (*PositionSummaryResult, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}

	where := "active = 1"
	args := []interface{}{}
	if strategyID > 0 {
		where += " AND strategy_id = ?"
		args = append(args, strategyID)
	}
	if symbol != "" {
		where += " AND symbol = ?"
		args = append(args, symbol)
	}
	if startTime != nil {
		where += " AND opened_at >= ?"
		args = append(args, *startTime)
	}
	if endTime != nil {
		where += " AND opened_at <= ?"
		args = append(args, *endTime)
	}

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) AS total_opens,
			SUM(CASE WHEN status='open' THEN 1 ELSE 0 END) AS current_open,
			SUM(CASE WHEN status='closed' THEN 1 ELSE 0 END) AS total_closed,
			SUM(CASE WHEN status='closed' THEN net_pnl ELSE 0 END) AS cum_net_pnl,
			SUM(CASE WHEN status='closed' AND net_pnl > 0 THEN 1 ELSE 0 END) AS win_count,
			AVG(CASE WHEN status='closed' AND closed_at IS NOT NULL THEN TIMESTAMPDIFF(SECOND, opened_at, closed_at) ELSE NULL END) AS avg_hold_seconds,
			MAX(CASE WHEN status='closed' THEN net_pnl ELSE NULL END) AS max_win,
			MIN(CASE WHEN status='closed' THEN net_pnl ELSE NULL END) AS max_loss,
			SUM(CASE WHEN close_reason='tp' THEN 1 ELSE 0 END) AS tp_count,
			SUM(CASE WHEN close_reason='sl' THEN 1 ELSE 0 END) AS sl_count,
			SUM(CASE WHEN close_reason='timeout' THEN 1 ELSE 0 END) AS timeout_count,
			SUM(CASE WHEN close_reason='manual' THEN 1 ELSE 0 END) AS manual_count
		FROM trade_strategy_position
		WHERE %s`, where)

	var result PositionSummaryResult
	if err := r.Db.Raw(query, args...).Scan(&result).Error; err != nil {
		return nil, err
	}
	return &result, nil
}

// UpdateMinMaxPrice 实时更新持仓期间的最高/最低价（非事务，允许覆盖竞争）。
func (r *TradeStrategyPositionRepository) UpdateMinMaxPrice(id int, currentPrice float64, current *TradeStrategyPosition) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	newMax := math.Max(current.MaxPriceDuringHold, currentPrice)
	newMin := current.MinPriceDuringHold
	if newMin == 0 || currentPrice < newMin {
		newMin = currentPrice
	}
	return r.Db.Model(&TradeStrategyPosition{}).Where("id = ?", id).Updates(map[string]interface{}{
		"max_price_during_hold": newMax,
		"min_price_during_hold": newMin,
		"updated_time":          time.Now().UTC(),
	}).Error
}
