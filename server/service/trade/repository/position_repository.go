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
