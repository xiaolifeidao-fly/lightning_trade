package repository

import (
	"common/middleware/db"
	"fmt"

	"gorm.io/gorm"
)

type CoinPlatformRepository struct {
	db.Repository[*CoinPlatform]
}

func (r *CoinPlatformRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinPlatform{})
}

func (r *CoinPlatformRepository) FindByCode(code string) (*CoinPlatform, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinPlatform
	if err := r.Db.Where("code = ? AND active = 1", code).First(&entity).Error; err != nil {
		return nil, err
	}
	if entity.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &entity, nil
}

func (r *CoinPlatformRepository) ListOnline() ([]*CoinPlatform, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinPlatform
	if err := r.Db.Where("active = 1 AND status = ?", "online").
		Order("sort_order DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinPlatformCoinRepository struct {
	db.Repository[*CoinPlatformCoin]
}

func (r *CoinPlatformCoinRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinPlatformCoin{})
}

func (r *CoinPlatformCoinRepository) FindByPlatformAndCoin(platformID, coinID uint64) (*CoinPlatformCoin, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinPlatformCoin
	if err := r.Db.Where("platform_id = ? AND coin_id = ? AND active = 1", platformID, coinID).
		First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *CoinPlatformCoinRepository) ListByPlatformID(platformID uint64) ([]*CoinPlatformCoin, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinPlatformCoin
	if err := r.Db.Where("platform_id = ? AND active = 1", platformID).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CoinPlatformCoinRepository) ListByCoinCode(coinCode string) ([]*CoinPlatformCoin, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinPlatformCoin
	if err := r.Db.Where("coin_code = ? AND active = 1", coinCode).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinPlatformAccountRepository struct {
	db.Repository[*CoinPlatformAccount]
}

func (r *CoinPlatformAccountRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinPlatformAccount{})
}

func (r *CoinPlatformAccountRepository) ListByPlatformID(platformID uint64) ([]*CoinPlatformAccount, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinPlatformAccount
	if err := r.Db.Where("platform_id = ? AND active = 1", platformID).
		Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CoinPlatformAccountRepository) FindActiveByName(platformID uint64, accountName string) (*CoinPlatformAccount, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinPlatformAccount
	if err := r.Db.Where("platform_id = ? AND account_name = ? AND active = 1 AND status = ?",
		platformID, accountName, "active").First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}
