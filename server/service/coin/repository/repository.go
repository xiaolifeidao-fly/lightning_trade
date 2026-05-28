package repository

import (
	"common/middleware/db"
	"fmt"

	"gorm.io/gorm"
)

type CoinRepository struct {
	db.Repository[*Coin]
}

func (r *CoinRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&Coin{})
}

func (r *CoinRepository) FindByCode(code string) (*Coin, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity Coin
	if err := r.Db.Where("code = ? AND active = 1", code).First(&entity).Error; err != nil {
		return nil, err
	}
	if entity.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &entity, nil
}

func (r *CoinRepository) ListOnlineCoins() ([]*Coin, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*Coin
	if err := r.Db.Where("active = 1 AND status = ?", "online").Order("sort_order DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinPairRepository struct {
	db.Repository[*CoinPair]
}

func (r *CoinPairRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinPair{})
}

func (r *CoinPairRepository) FindBySymbol(symbol string) (*CoinPair, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinPair
	if err := r.Db.Where("symbol = ? AND active = 1", symbol).First(&entity).Error; err != nil {
		return nil, err
	}
	if entity.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &entity, nil
}

func (r *CoinPairRepository) ListOnlinePairs() ([]*CoinPair, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinPair
	if err := r.Db.Where("active = 1 AND status = ?", "online").Order("sort_order DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinPriceRepository struct {
	db.Repository[*CoinPrice]
}

func (r *CoinPriceRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinPrice{})
}

func (r *CoinPriceRepository) FindLatestByCode(coinCode, quoteCode string) (*CoinPrice, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinPrice
	if err := r.Db.Where("coin_code = ? AND quote_code = ? AND active = 1", coinCode, quoteCode).
		Order("id DESC").First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}
