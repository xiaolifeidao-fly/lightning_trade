package argus_config

import (
	"context"
	"os"
	"testing"
	"time"

	"common/middleware/db"
	"service/argus_config/repository"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 真实 MySQL 集成校验：planSessionRotate 是纯函数、已有单测覆盖，但真正动凭证的
// 那条 UPDATE（UpsertRuntimeSession）只有连上库才能验——列名写错、Save 与 Updates
// 语义差异、把不属于本次轮换的列顺手清掉，这些都是单测看不见的。
//
// 默认跳过，需要时显式给 DSN：
//
//	ARGUS_CONFIG_TEST_DSN='user:pass@tcp(127.0.0.1:3306)/argus_config_check' \
//	  go test ./argus_config/ -run TestIntegration -v
//
// 库会被反复建表写入，**必须指向可丢弃的 schema**。实例键一律 it- 前缀，
// 绝不用真实部署的键：用例会按实例键清理自己的行。
const itRotateInstance = "it-argus-config-rotate"

func rotateIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ARGUS_CONFIG_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 ARGUS_CONFIG_TEST_DSN，跳过真实 MySQL 集成校验")
	}
	database, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		t.Fatalf("连库失败: %v", err)
	}
	db.Db = database
	service := NewArgusConfigService()
	if err := service.EnsureTable(); err != nil {
		t.Fatalf("建表失败: %v", err)
	}
	return database
}

// seedRotateFixture 造一个"已发布 v1、两个账户、其中一个已有会话行"的实例。
// 返回两个账户的行 id。
func seedRotateFixture(t *testing.T, database *gorm.DB) (uint64, uint64) {
	t.Helper()
	cleanupRotateFixture(t, database)

	version := repository.ArgusConfigVersion{
		InstanceKey: itRotateInstance, Version: 1, SnapshotChecksum: "it-checksum",
	}
	// published_slot 是 *uint8：只有已发布那一行为 1，其余为 NULL，
	// 靠 (instance_key, published_slot) 的唯一索引保证每实例只有一个已发布版本。
	// 走 MarkPublished 而不是自己塞指针，口径与生产发布路径一致。
	version.MarkPublished(time.Date(2026, 9, 10, 16, 0, 0, 0, time.UTC))
	version.Active = 1
	if err := database.Create(&version).Error; err != nil {
		t.Fatalf("造版本行失败: %v", err)
	}

	accountA := repository.ArgusAccount{
		ConfigVersionID: uint64(version.Id), AccountName: "it-账户A", UID: "111",
		URL: "http://localhost:8899", LoginType: "config", Enabled: 1,
	}
	accountA.Active = 1
	accountB := repository.ArgusAccount{
		ConfigVersionID: uint64(version.Id), AccountName: "it-账户B", UID: "222",
		URL: "http://localhost:8899", LoginType: "config", Enabled: 1,
	}
	accountB.Active = 1
	if err := database.Create(&accountA).Error; err != nil {
		t.Fatalf("造账户 A 失败: %v", err)
	}
	if err := database.Create(&accountB).Error; err != nil {
		t.Fatalf("造账户 B 失败: %v", err)
	}

	// 只给 A 造会话行，且带上一个 last_error —— 轮换必须把它清掉，
	// 否则巡检页会一直挂着上一次的失败原因。
	sessionA := repository.ArgusRuntimeSession{
		AccountID: uint64(accountA.Id), Cookie: "old-cookie-a", Token: "old-token-a",
		OToken: "old-otoken-a", Valid: 1, LastError: "上次刷新失败：cookie 过期",
		SessionUpdatedAt: time.Date(2026, 8, 20, 18, 7, 18, 0, time.UTC),
	}
	sessionA.Active = 1
	if err := database.Create(&sessionA).Error; err != nil {
		t.Fatalf("造会话行失败: %v", err)
	}
	return uint64(accountA.Id), uint64(accountB.Id)
}

func cleanupRotateFixture(t *testing.T, database *gorm.DB) {
	t.Helper()
	var accountIDs []uint64
	database.Model(&repository.ArgusAccount{}).
		Where("config_version_id IN (?)", database.Model(&repository.ArgusConfigVersion{}).
			Select("id").Where("instance_key = ?", itRotateInstance)).
		Pluck("id", &accountIDs)
	if len(accountIDs) > 0 {
		database.Where("account_id IN ?", accountIDs).Delete(&repository.ArgusRuntimeSession{})
		database.Where("id IN ?", accountIDs).Delete(&repository.ArgusAccount{})
	}
	database.Where("instance_key = ?", itRotateInstance).Delete(&repository.ArgusConfigVersion{})
}

func TestIntegrationSessionRotateUpdatesAndInserts(t *testing.T) {
	database := rotateIntegrationDB(t)
	accountA, accountB := seedRotateFixture(t, database)
	t.Cleanup(func() { cleanupRotateFixture(t, database) })

	service := NewArgusConfigService()
	ctx := context.Background()

	plan, err := service.PlanSessionRotate(ctx, itRotateInstance, map[string]importSession{
		"a": {AccountName: "it-账户A", UID: "111", Cookie: "new-cookie-a", Token: "new-token-a",
			OToken: "new-otoken-a", SentryRelease: "ms26.09.10", LoginURL: "https://example.test/login",
			UpdatedAt: "2026-09-10T17:00:00Z"},
		"b": {AccountName: "it-账户B", UID: "222", Cookie: "new-cookie-b", Token: "new-token-b",
			UpdatedAt: "2026-09-10T17:00:00Z"},
	})
	if err != nil {
		t.Fatalf("生成计划失败: %v", err)
	}
	if plan.RotateCount() != 2 {
		t.Fatalf("应有 2 行待轮换（A 更新、B 插入），实际 %d：%+v", plan.RotateCount(), plan.Items)
	}

	written, err := service.ApplySessionRotate(ctx, plan, "it-rotate-actor")
	if err != nil {
		t.Fatalf("落库失败: %v", err)
	}
	if written != 2 {
		t.Fatalf("应写 2 行，实际 %d", written)
	}

	// A：走 UPDATE 分支。
	var rowA repository.ArgusRuntimeSession
	if err := database.Where("account_id = ? AND active = 1", accountA).First(&rowA).Error; err != nil {
		t.Fatalf("读回账户 A 的会话失败: %v", err)
	}
	if rowA.Cookie != "new-cookie-a" || rowA.Token != "new-token-a" || rowA.OToken != "new-otoken-a" {
		t.Errorf("账户 A 的凭证没换成新的：cookie=%q token=%q otoken=%q", rowA.Cookie, rowA.Token, rowA.OToken)
	}
	if rowA.SentryRelease != "ms26.09.10" || rowA.LoginURL != "https://example.test/login" {
		t.Errorf("附带字段没写进去：sentry=%q loginURL=%q", rowA.SentryRelease, rowA.LoginURL)
	}
	// 换了新凭证，上一次的失败原因必须清掉。
	if rowA.LastError != "" {
		t.Errorf("last_error 应被清空，实际 %q", rowA.LastError)
	}
	if rowA.Valid != 1 {
		t.Errorf("valid 应为 1，实际 %d", rowA.Valid)
	}
	if rowA.UpdatedBy != "it-rotate-actor" {
		t.Errorf("updated_by 应记下操作者，实际 %q", rowA.UpdatedBy)
	}
	if !rowA.SessionUpdatedAt.UTC().Equal(time.Date(2026, 9, 10, 17, 0, 0, 0, time.UTC)) {
		t.Errorf("session_updated_at 应取 session.json 的 updatedAt，实际 %v", rowA.SessionUpdatedAt)
	}

	// B：原来没有会话行，走 INSERT 分支。
	var rowB repository.ArgusRuntimeSession
	if err := database.Where("account_id = ? AND active = 1", accountB).First(&rowB).Error; err != nil {
		t.Fatalf("读回账户 B 的会话失败（插入分支没生效？）: %v", err)
	}
	if rowB.Cookie != "new-cookie-b" || rowB.Token != "new-token-b" {
		t.Errorf("账户 B 的凭证不对：cookie=%q token=%q", rowB.Cookie, rowB.Token)
	}
	if rowB.CreatedBy != "it-rotate-actor" {
		t.Errorf("插入分支应记 created_by，实际 %q", rowB.CreatedBy)
	}

	// 再跑一次同样的输入：应全部判为 unchanged，一行都不写。
	// 这条挡住"每次轮换都无脑重写"——重写会白白触发一次热加载，
	// 而热加载会停掉正在扛仓的交易管理器。
	plan2, err := service.PlanSessionRotate(ctx, itRotateInstance, map[string]importSession{
		"a": {AccountName: "it-账户A", UID: "111", Cookie: "new-cookie-a", Token: "new-token-a",
			OToken: "new-otoken-a", UpdatedAt: "2026-09-10T17:00:00Z"},
		"b": {AccountName: "it-账户B", UID: "222", Cookie: "new-cookie-b", Token: "new-token-b",
			UpdatedAt: "2026-09-10T17:00:00Z"},
	})
	if err != nil {
		t.Fatalf("第二次生成计划失败: %v", err)
	}
	if plan2.RotateCount() != 0 {
		t.Errorf("同样的凭证再跑一次应全部 unchanged，实际待写 %d 行：%+v", plan2.RotateCount(), plan2.Items)
	}
}

// 实例键大小写不同也能查到行（MySQL 默认排序规则不区分大小写），
// 但计划里的实例键必须回到**库里那一行自己的**写法——Redis 控制消息在进程侧是
// Go 字符串精确比较，大写形式发过去会被静默忽略，表现为"轮换成功却一直不生效"。
func TestIntegrationSessionRotateNormalizesInstanceKeyFromRow(t *testing.T) {
	database := rotateIntegrationDB(t)
	seedRotateFixture(t, database)
	t.Cleanup(func() { cleanupRotateFixture(t, database) })

	service := NewArgusConfigService()
	plan, err := service.PlanSessionRotate(context.Background(), "IT-ARGUS-CONFIG-ROTATE",
		map[string]importSession{
			"a": {AccountName: "it-账户A", UID: "111", Cookie: "c", Token: "t"},
			"b": {AccountName: "it-账户B", UID: "222", Cookie: "c", Token: "t"},
		})
	if err != nil {
		t.Fatalf("生成计划失败: %v", err)
	}
	if plan.InstanceKey != itRotateInstance {
		t.Errorf("计划里的实例键应取库里那一行的写法 %q，实际 %q", itRotateInstance, plan.InstanceKey)
	}
}

// 没有已发布配置时要给出能照着做的错误，而不是一句裸的 record not found。
func TestIntegrationSessionRotateMissingPublishedConfig(t *testing.T) {
	database := rotateIntegrationDB(t)
	cleanupRotateFixture(t, database)

	service := NewArgusConfigService()
	_, err := service.PlanSessionRotate(context.Background(), itRotateInstance,
		map[string]importSession{"a": {AccountName: "it-账户A", UID: "111", Cookie: "c", Token: "t"}})
	if err == nil {
		t.Fatal("没有已发布配置时必须报错")
	}
	if !contains(err.Error(), "argus-config-import") {
		t.Errorf("错误信息应指出该去导配置，实际：%v", err)
	}
}
