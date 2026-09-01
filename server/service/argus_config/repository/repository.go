package repository

import (
	"common/middleware/db"
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// PublishedSlotIndexName 是每实例唯一已发布版本的索引名。它从单列
// (published_slot) 迁移到 (instance_key, published_slot)，AutoMigrate 不会重建
// 同名索引，所以 EnsureTable 里要显式修正。
const (
	PublishedSlotIndexName   = "idx_argus_single_published"
	VersionSequenceIndexName = "idx_argus_config_version"
)

// ErrInstanceKeyRequired 表示调用方没有给出实例键，配置读写不允许跨实例兜底。
var ErrInstanceKeyRequired = fmt.Errorf("argus instance key is required")

type ArgusConfigRepository struct {
	db.Repository[*ArgusConfigVersion]
}

func (r *ArgusConfigRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if err := r.Db.AutoMigrate(
		&ArgusInstance{},
		&ArgusConfigVersion{},
		&ArgusConfig{},
		&ArgusAccount{},
		&ArgusAccountRisk{},
		&ArgusMonitorSymbol{},
		&ArgusNotification{},
		&ArgusRuntimeSession{},
	); err != nil {
		return err
	}
	return r.EnsureInstanceScopedIndexes()
}

// EnsureInstanceScopedIndexes 把历史库里遗留的单列唯一索引重建成实例内唯一。
func (r *ArgusConfigRepository) EnsureInstanceScopedIndexes() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	table := (&ArgusConfigVersion{}).TableName()
	if err := r.rebuildUniqueIndex(table, PublishedSlotIndexName, []string{"instance_key", "published_slot"}); err != nil {
		return err
	}
	return r.rebuildUniqueIndex(table, VersionSequenceIndexName, []string{"instance_key", "version"})
}

func (r *ArgusConfigRepository) rebuildUniqueIndex(table, indexName string, wanted []string) error {
	var columns []string
	if err := r.Db.Raw(
		"SELECT COLUMN_NAME FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ? ORDER BY seq_in_index",
		table, indexName,
	).Scan(&columns).Error; err != nil {
		return fmt.Errorf("inspect index %s: %w", indexName, err)
	}
	if strings.EqualFold(strings.Join(columns, ","), strings.Join(wanted, ",")) {
		return nil
	}
	if len(columns) > 0 {
		if err := r.Db.Exec(fmt.Sprintf("DROP INDEX `%s` ON `%s`", indexName, table)).Error; err != nil {
			return fmt.Errorf("drop legacy index %s: %w", indexName, err)
		}
	}
	statement := fmt.Sprintf("CREATE UNIQUE INDEX `%s` ON `%s` (`%s`)", indexName, table, strings.Join(wanted, "`, `"))
	if err := r.Db.Exec(statement).Error; err != nil {
		return fmt.Errorf("create index %s: %w", indexName, err)
	}
	return nil
}

// BackfillInstanceKey 把新增 instance_key 之前的历史版本行归到默认实例，
// 返回补齐的行数，便于调用方打印迁移日志。
func (r *ArgusConfigRepository) BackfillInstanceKey(defaultInstanceKey string) (int64, error) {
	if strings.TrimSpace(defaultInstanceKey) == "" {
		return 0, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	result := r.Db.Model(&ArgusConfigVersion{}).
		Where("instance_key IS NULL OR instance_key = ''").
		Update("instance_key", strings.TrimSpace(defaultInstanceKey))
	return result.RowsAffected, result.Error
}

func (r *ArgusConfigRepository) FindPublished(instanceKey string) (*ArgusConfigVersion, error) {
	return r.FindPublishedContext(context.Background(), instanceKey)
}

func (r *ArgusConfigRepository) FindPublishedContext(ctx context.Context, instanceKey string) (*ArgusConfigVersion, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return nil, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var version ArgusConfigVersion
	if err := r.Db.WithContext(ctx).Where("instance_key = ? AND published_slot = ? AND active = 1", strings.TrimSpace(instanceKey), 1).First(&version).Error; err != nil {
		return nil, err
	}
	return &version, nil
}

func (r *ArgusConfigRepository) FindByVersion(instanceKey string, versionNumber uint64) (*ArgusConfigVersion, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return nil, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var version ArgusConfigVersion
	if err := r.Db.Where("instance_key = ? AND version = ? AND active = 1", strings.TrimSpace(instanceKey), versionNumber).First(&version).Error; err != nil {
		return nil, err
	}
	return &version, nil
}

// NextVersion 按实例独立递增，三个实例的版本序列互不干扰。
func (r *ArgusConfigRepository) NextVersion(instanceKey string) (uint64, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return 0, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var version uint64
	if err := r.Db.Model(&ArgusConfigVersion{}).Where("instance_key = ?", strings.TrimSpace(instanceKey)).Select("COALESCE(MAX(version), 0) + 1").Scan(&version).Error; err != nil {
		return 0, err
	}
	if version == 0 {
		return 1, nil
	}
	return version, nil
}

// ListInstances 返回全部有效实例，按实例键排序保证输出稳定。
func (r *ArgusConfigRepository) ListInstances(onlyEnabled bool) ([]*ArgusInstance, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	query := r.Db.Model(&ArgusInstance{}).Where("active = 1")
	if onlyEnabled {
		query = query.Where("enabled = 1")
	}
	var instances []*ArgusInstance
	if err := query.Order("instance_key ASC").Find(&instances).Error; err != nil {
		return nil, err
	}
	return instances, nil
}

func (r *ArgusConfigRepository) FindInstanceByKey(instanceKey string) (*ArgusInstance, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return nil, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var instance ArgusInstance
	if err := r.Db.Where("instance_key = ? AND active = 1", strings.TrimSpace(instanceKey)).First(&instance).Error; err != nil {
		return nil, err
	}
	return &instance, nil
}

func (r *ArgusConfigRepository) CreateInstance(instance *ArgusInstance) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Create(instance).Error
}

func (r *ArgusConfigRepository) SaveInstance(instance *ArgusInstance) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.Save(instance).Error
}

// DuplicateInstanceKeys 找出同一实例键的多条有效记录。唯一索引正常时结果必为空，
// 历史库缺索引时用它触发重复告警。
func (r *ArgusConfigRepository) DuplicateInstanceKeys() ([]string, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []struct {
		InstanceKey string `gorm:"column:instance_key"`
	}
	if err := r.Db.Model(&ArgusInstance{}).
		Select("instance_key").
		Where("active = 1").
		Group("instance_key").
		Having("COUNT(1) > 1").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		keys = append(keys, row.InstanceKey)
	}
	return keys, nil
}

func (r *ArgusConfigRepository) LoadSnapshot(instanceKey string, versionID uint64) (*ArgusConfigVersion, *ArgusConfig, []*ArgusAccount, []*ArgusAccountRisk, []*ArgusMonitorSymbol, *ArgusNotification, []*ArgusRuntimeSession, error) {
	return r.LoadSnapshotContext(context.Background(), instanceKey, versionID)
}

// LoadSnapshotContext 用实例键作为版本归属校验，防止跨实例读到别人的版本。
func (r *ArgusConfigRepository) LoadSnapshotContext(ctx context.Context, instanceKey string, versionID uint64) (*ArgusConfigVersion, *ArgusConfig, []*ArgusAccount, []*ArgusAccountRisk, []*ArgusMonitorSymbol, *ArgusNotification, []*ArgusRuntimeSession, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return nil, nil, nil, nil, nil, nil, nil, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("database is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, err
	}
	database := r.Db.WithContext(ctx)
	var version ArgusConfigVersion
	if err := database.Where("id = ? AND instance_key = ? AND active = 1", versionID, strings.TrimSpace(instanceKey)).First(&version).Error; err != nil {
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
