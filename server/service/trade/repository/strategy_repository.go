package repository

import (
	"common/middleware/db"
	"fmt"
	"strings"
)

type TradeStrategyRepository struct {
	db.Repository[*TradeStrategy]
}

func (r *TradeStrategyRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeStrategy{})
}

// FindActiveBySymbolInterval 查询指定 platform×symbol×interval 下所有启用的策略。
func (r *TradeStrategyRepository) FindActiveBySymbolInterval(platformCode, symbol, interval string) ([]*TradeStrategy, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeStrategy
	err := r.Db.Where(
		"active = 1 AND enabled = 1 AND platform_code = ? AND symbol = ? AND `interval` = ?",
		strings.ToLower(platformCode),
		strings.ToUpper(symbol),
		interval,
	).Find(&rows).Error
	return rows, err
}

// CountOpenPositions 统计指定策略当前持仓中（status=open）的仓位数，用于 max_open_positions 判断。
func (r *TradeStrategyRepository) CountOpenPositions(strategyID int64) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var count int64
	err := r.Db.Model(&TradeStrategyPosition{}).
		Where("active = 1 AND strategy_id = ? AND status = 'open'", strategyID).
		Count(&count).Error
	return count, err
}
