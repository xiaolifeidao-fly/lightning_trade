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
	if err := r.EnsureInstanceScopedIndexes(); err != nil {
		return err
	}
	return r.EnsureLoginTypeConstraint()
}

// LoginTypeConstraintName 是 argus_account.login_type 的取值约束名。
const LoginTypeConstraintName = "chk_argus_account_login_type"

// loginTypeAllowedValues 必须与 argus_single 的 trade.BuildUserProvider 支持的
// 取值完全一致：config = 静态 cookie/token，password = pl-instance 无头登录。
var loginTypeAllowedValues = []string{"config", "password"}

// EnsureLoginTypeConstraint 把历史库里遗留的 login_type 约束重建成与代码一致。
//
// 为什么需要显式重建：AutoMigrate 不会修改已存在的 CHECK 约束（与索引同一类
// 问题）。历史约束写的是 IN ('password','api_key','cookie')，与代码只在
// password 上重叠——想把账户改成 config（静态凭证）会被 ERROR 3819 拒绝，
// 结果是配置入库后只能走 password 模式去调 pl-instance；pl-instance 未部署时
// 盘口信号开仓全部失败，且现场只看得到"获取 Web 用户凭证失败"，看不到根因。
//
// 顺带把不再受支持的存量取值收敛到 config：'cookie' 与 'api_key' 在代码里都会
// 被 BuildUserProvider 判成"不支持"而直接建不出 Web 客户端。
func (r *ArgusConfigRepository) EnsureLoginTypeConstraint() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	table := (&ArgusAccount{}).TableName()
	var clause string
	if err := r.Db.Raw(
		"SELECT check_clause FROM information_schema.check_constraints WHERE constraint_schema = DATABASE() AND constraint_name = ?",
		LoginTypeConstraintName,
	).Scan(&clause).Error; err != nil {
		return fmt.Errorf("inspect constraint %s: %w", LoginTypeConstraintName, err)
	}
	// 已经只允许 config/password 就不动它，避免每次启动都 DDL。
	if strings.Contains(clause, "'config'") && !strings.Contains(clause, "'cookie'") && !strings.Contains(clause, "'api_key'") {
		return nil
	}
	// 先把存量非法值收敛，否则新约束建不上。
	if err := r.Db.Exec(fmt.Sprintf(
		"UPDATE `%s` SET login_type = 'config' WHERE login_type IS NULL OR login_type NOT IN ('config','password')", table,
	)).Error; err != nil {
		return fmt.Errorf("normalize legacy login_type: %w", err)
	}
	if strings.TrimSpace(clause) != "" {
		if err := r.Db.Exec(fmt.Sprintf("ALTER TABLE `%s` DROP CHECK `%s`", table, LoginTypeConstraintName)).Error; err != nil {
			return fmt.Errorf("drop legacy constraint %s: %w", LoginTypeConstraintName, err)
		}
	}
	values := "'" + strings.Join(loginTypeAllowedValues, "','") + "'"
	if err := r.Db.Exec(fmt.Sprintf(
		"ALTER TABLE `%s` ADD CONSTRAINT `%s` CHECK (login_type IN (%s))", table, LoginTypeConstraintName, values,
	)).Error; err != nil {
		return fmt.Errorf("create constraint %s: %w", LoginTypeConstraintName, err)
	}
	return nil
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

// CountMissingInstanceKey 统计还没有归属实例的历史版本行。
//
// 用途只有一个：区分「全新部署，本来就没有存量行」与「升级部署，存量行等着
// 归属实例」。前者跳过回填是对的，后者跳过会让所有实例都查不到自己的已发布
// 版本——同一段代码，两种截然不同的后果，只能靠这个计数分开。
func (r *ArgusConfigRepository) CountMissingInstanceKey() (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	var count int64
	if err := r.Db.Model(&ArgusConfigVersion{}).
		Where("instance_key IS NULL OR instance_key = ''").
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count config versions missing instance key: %w", err)
	}
	return count, nil
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

// ListVersions 返回某个实例的版本历史，按版本号倒序。参数页的回滚入口只认这
// 一个来源，跨实例的版本不会互相出现在对方的历史里。
func (r *ArgusConfigRepository) ListVersions(instanceKey string, limit int) ([]*ArgusConfigVersion, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return nil, ErrInstanceKeyRequired
	}
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var versions []*ArgusConfigVersion
	if err := r.Db.Where("instance_key = ? AND active = 1", strings.TrimSpace(instanceKey)).
		Order("version DESC").Limit(limit).Find(&versions).Error; err != nil {
		return nil, err
	}
	return versions, nil
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

// LoadAccountsWithSessions 只读某个配置版本的账户与其会话行。
//
// 为什么不复用 LoadSnapshotContext：那个函数要求 argus_config 行必须存在
// （缺了直接返回 record not found），而凭证轮换只依赖账户与会话两张表。
// 用整份快照的加载器等于让轮换被一堆无关行的存在性绑住，出错信息还会指向
// argus_config 这种和轮换毫无关系的地方。
func (r *ArgusConfigRepository) LoadAccountsWithSessions(ctx context.Context, versionID uint64) ([]*ArgusAccount, []*ArgusRuntimeSession, error) {
	if r.Db == nil {
		return nil, nil, fmt.Errorf("database is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	database := r.Db.WithContext(ctx)
	var accounts []*ArgusAccount
	if err := database.Where("config_version_id = ? AND active = 1", versionID).Order("id ASC").Find(&accounts).Error; err != nil {
		return nil, nil, err
	}
	if len(accounts) == 0 {
		return accounts, nil, nil
	}
	ids := make([]uint64, 0, len(accounts))
	for _, account := range accounts {
		ids = append(ids, uint64(account.Id))
	}
	var sessions []*ArgusRuntimeSession
	if err := database.Where("account_id IN ? AND active = 1", ids).Order("id ASC").Find(&sessions).Error; err != nil {
		return nil, nil, err
	}
	return accounts, sessions, nil
}

// UpsertRuntimeSession 只改会话列，不碰配置版本与参数行。
//
// 轮换 cookie/token 是每周的常规运维，它不是"改参数"：新建一个配置版本会让
// 版本历史里全是凭证轮换的噪音，也会让"这版改了什么参数"变得没法看。
// Id 为 0 时插入（新导入的实例可能还没有会话行），否则按主键更新。
//
// 显式列出要写的列而不是 Save 整个结构体：Save 会把零值也写进去，
// expires_at / last_error 这些不属于本次轮换的列会被顺手清掉。
func (r *ArgusConfigRepository) UpsertRuntimeSession(ctx context.Context, row *ArgusRuntimeSession, actor string) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if row == nil || row.AccountID == 0 {
		return fmt.Errorf("runtime session requires an account id")
	}
	database := r.Db.WithContext(ctx)
	if row.Id == 0 {
		row.Active = 1
		row.CreatedBy = actor
		row.UpdatedBy = actor
		return database.Create(row).Error
	}
	return database.Model(&ArgusRuntimeSession{}).Where("id = ?", row.Id).Updates(map[string]interface{}{
		"cookie":             row.Cookie,
		"token":              row.Token,
		"otoken":             row.OToken,
		"sentry_release":     row.SentryRelease,
		"sentry_public_key":  row.SentryPublicKey,
		"baggage":            row.Baggage,
		"login_url":          row.LoginURL,
		"final_url":          row.FinalURL,
		"valid":              row.Valid,
		"session_updated_at": row.SessionUpdatedAt,
		// 换了新凭证，上一次的失败原因就过期了；留着会让巡检页一直挂着旧告警。
		"last_error": "",
		"active":     1,
		"updated_by": actor,
	}).Error
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
