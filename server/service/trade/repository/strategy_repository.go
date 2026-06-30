package repository

import (
	"common/middleware/db"
	"fmt"
	"strings"
	"time"
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

// FindAll 分页查询策略列表，支持按 platformCode/coinCode/symbol/interval/enabled 过滤。
func (r *TradeStrategyRepository) FindAll(
	platformCode, coinCode, symbol, interval string,
	enabled *int8,
	page, pageSize int,
) ([]*TradeStrategy, int64, error) {
	if r.Db == nil {
		return nil, 0, fmt.Errorf("database is not initialized")
	}
	q := r.Db.Model(&TradeStrategy{}).Where("active = 1")
	if platformCode != "" {
		q = q.Where("platform_code = ?", platformCode)
	}
	if coinCode != "" {
		q = q.Where("coin_code = ?", coinCode)
	}
	if symbol != "" {
		q = q.Where("symbol = ?", strings.ToUpper(symbol))
	}
	if interval != "" {
		q = q.Where("`interval` = ?", interval)
	}
	if enabled != nil {
		q = q.Where("enabled = ?", *enabled)
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
	var rows []*TradeStrategy
	err := q.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error
	return rows, total, err
}

// FindByID 按主键查询，active=1。
func (r *TradeStrategyRepository) FindByID(id int64) (*TradeStrategy, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var row TradeStrategy
	err := r.Db.Where("id = ? AND active = 1", id).First(&row).Error
	return &row, err
}

// CreateStrategy 写入一条策略配置。
func (r *TradeStrategyRepository) CreateStrategy(s *TradeStrategy) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	s.Init()
	return r.Db.Create(s).Error
}

// UpdateStrategy 按 map 更新指定策略的可变字段。
func (r *TradeStrategyRepository) UpdateStrategy(id int64, updates map[string]interface{}) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	updates["updated_time"] = time.Now().UTC()
	return r.Db.Model(&TradeStrategy{}).Where("id = ? AND active = 1", id).Updates(updates).Error
}

// SoftDelete 软删除：将 active 置 0。
func (r *TradeStrategyRepository) SoftDelete(id int64) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Model(&TradeStrategy{}).Where("id = ? AND active = 1", id).Updates(map[string]interface{}{
		"active":       0,
		"enabled":      0,
		"updated_time": time.Now().UTC(),
	}).Error
}
