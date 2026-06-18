package repository

import (
	"common/middleware/db"
	"fmt"
	newsDTO "service/news/dto"
	"strings"
	"time"

	"gorm.io/gorm"
)

type NewsSentimentRepository struct {
	db.Repository[*NewsSentiment]
}

func (r *NewsSentimentRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&NewsSentiment{})
}

// FindLatestByCoin 取指定币种最近一条消息面（按拉取时间倒序）。无记录返回 (nil, nil)。
func (r *NewsSentimentRepository) FindLatestByCoin(coinCode string) (*NewsSentiment, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity NewsSentiment
	err := r.Db.Where("active = 1 AND coin_code = ?", strings.ToUpper(coinCode)).
		Order("fetched_time DESC, id DESC").First(&entity).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// CountByQuery / ListByQuery 支持按币种与时间范围分页查询历史消息面。
func (r *NewsSentimentRepository) CountByQuery(query newsDTO.NewsSentimentQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var total int64
	if err := r.buildWhere(query).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *NewsSentimentRepository) buildWhere(query newsDTO.NewsSentimentQueryDTO) *gorm.DB {
	dbq := r.Db.Model(&NewsSentiment{}).Where("active = 1")
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if query.StartTime > 0 {
		dbq = dbq.Where("fetched_time >= ?", time.Unix(query.StartTime, 0))
	}
	if query.EndTime > 0 {
		dbq = dbq.Where("fetched_time <= ?", time.Unix(query.EndTime, 0))
	}
	return dbq
}

func (r *NewsSentimentRepository) ListByQuery(query newsDTO.NewsSentimentQueryDTO, pageIndex, pageSize int) ([]*NewsSentiment, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*NewsSentiment
	if err := r.buildWhere(query).Order("fetched_time DESC, id DESC").
		Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
