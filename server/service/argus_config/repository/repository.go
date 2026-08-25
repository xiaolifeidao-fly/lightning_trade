package repository

import (
	"common/middleware/db"
	"context"
	"fmt"
	"gorm.io/gorm"
)

type ArgusConfigRepository struct {
	db.Repository[*ArgusConfigVersion]
}

func (r *ArgusConfigRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(
		&ArgusConfigVersion{},
		&ArgusConfig{},
		&ArgusAccount{},
		&ArgusAccountRisk{},
		&ArgusMonitorSymbol{},
		&ArgusNotification{},
		&ArgusRuntimeSession{},
	)
}

func (r *ArgusConfigRepository) FindPublished() (*ArgusConfigVersion, error) {
	return r.FindPublishedContext(context.Background())
}

func (r *ArgusConfigRepository) FindPublishedContext(ctx context.Context) (*ArgusConfigVersion, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var version ArgusConfigVersion
	if err := r.Db.WithContext(ctx).Where("published_slot = ? AND active = 1", 1).First(&version).Error; err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *ArgusConfigRepository) FindByVersion(versionNumber uint64) (*ArgusConfigVersion, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var version ArgusConfigVersion
	if err := r.Db.Where("version = ? AND active = 1", versionNumber).First(&version).Error; err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *ArgusConfigRepository) NextVersion() (uint64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var version uint64
	if err := r.Db.Model(&ArgusConfigVersion{}).Select("COALESCE(MAX(version), 0) + 1").Scan(&version).Error; err != nil {
		return 0, err
	}
	if version == 0 {
		return 1, nil
	}
	return version, nil
}

func (r *ArgusConfigRepository) LoadSnapshot(versionID uint64) (*ArgusConfigVersion, *ArgusConfig, []*ArgusAccount, []*ArgusAccountRisk, []*ArgusMonitorSymbol, *ArgusNotification, []*ArgusRuntimeSession, error) {
	return r.LoadSnapshotContext(context.Background(), versionID)
}

func (r *ArgusConfigRepository) LoadSnapshotContext(ctx context.Context, versionID uint64) (*ArgusConfigVersion, *ArgusConfig, []*ArgusAccount, []*ArgusAccountRisk, []*ArgusMonitorSymbol, *ArgusNotification, []*ArgusRuntimeSession, error) {
	if r.Db == nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("database is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	database := r.Db.WithContext(ctx)
	var version ArgusConfigVersion
	if err := database.Where("id = ? AND active = 1", versionID).First(&version).Error; err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	var config ArgusConfig
	if err := database.Where("config_version_id = ? AND active = 1", versionID).First(&config).Error; err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	var accounts []*ArgusAccount
	if err := database.Where("config_version_id = ? AND active = 1", versionID).Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	var risks []*ArgusAccountRisk
	if err := database.Where("config_version_id = ? AND active = 1", versionID).Order("id ASC").Find(&risks).Error; err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	var symbols []*ArgusMonitorSymbol
	if err := database.Where("config_version_id = ? AND active = 1", versionID).Order("id ASC").Find(&symbols).Error; err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	var notification ArgusNotification
	if err := database.Where("config_version_id = ? AND active = 1", versionID).First(&notification).Error; err != nil && err != gorm.ErrRecordNotFound {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	var sessions []*ArgusRuntimeSession
	if err := database.Where("account_id IN (?) AND active = 1", database.Model(&ArgusAccount{}).Select("id").Where("config_version_id = ?", versionID)).Order("id ASC").Find(&sessions).Error; err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	return &version, &config, accounts, risks, symbols, &notification, sessions, nil
}
