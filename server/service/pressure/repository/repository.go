package repository

import (
	"common/middleware/db"
	"fmt"
	pressureDTO "service/pressure/dto"
	"strings"
	"time"

	"gorm.io/gorm"
)

type PressureAnalysisRepository struct {
	db.Repository[*PressureAnalysis]
}

func (r *PressureAnalysisRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&PressureAnalysis{})
}

// FindLatestByCoin 取指定币种最近一条压力面分析（按分析时间倒序）。无记录返回 (nil, nil)。
func (r *PressureAnalysisRepository) FindLatestByCoin(coinCode string) (*PressureAnalysis, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity PressureAnalysis
	err := r.Db.Where("active = 1 AND coin_code = ?", strings.ToUpper(coinCode)).
		Order("analyzed_time DESC, id DESC").First(&entity).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entity, nil
}

// ListByCoinPlatformBefore 取指定平台×币种、analyzed_time ≤ end 的历史压力面分析，按 analyzed_time 升序返回。
// 供回测「时间对齐」用：每条预测取信号时刻之前最近一次压力面。platform 为空则不按平台过滤。
func (r *PressureAnalysisRepository) ListByCoinPlatformBefore(platform, coin string, end time.Time) ([]*PressureAnalysis, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Model(&PressureAnalysis{}).
		Where("active = 1 AND coin_code = ?", strings.ToUpper(coin)).
		Where("analyzed_time <= ?", end)
	if v := strings.TrimSpace(platform); v != "" {
		dbq = dbq.Where("platform_code = ?", v)
	}
	var rows []*PressureAnalysis
	if err := dbq.Order("analyzed_time ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountByQuery / ListByQuery 支持按平台、币种与时间范围分页查询历史压力面分析。
func (r *PressureAnalysisRepository) CountByQuery(query pressureDTO.PressureAnalysisQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var total int64
	if err := r.buildWhere(query).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func (r *PressureAnalysisRepository) buildWhere(query pressureDTO.PressureAnalysisQueryDTO) *gorm.DB {
	dbq := r.Db.Model(&PressureAnalysis{}).Where("active = 1")
	if v := strings.TrimSpace(query.PlatformCode); v != "" {
		dbq = dbq.Where("platform_code = ?", v)
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if query.StartTime > 0 {
		dbq = dbq.Where("analyzed_time >= ?", time.Unix(query.StartTime, 0))
	}
	if query.EndTime > 0 {
		dbq = dbq.Where("analyzed_time <= ?", time.Unix(query.EndTime, 0))
	}
	return dbq
}

func (r *PressureAnalysisRepository) ListByQuery(query pressureDTO.PressureAnalysisQueryDTO, pageIndex, pageSize int) ([]*PressureAnalysis, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*PressureAnalysis
	if err := r.buildWhere(query).Order("analyzed_time DESC, id DESC").
		Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
