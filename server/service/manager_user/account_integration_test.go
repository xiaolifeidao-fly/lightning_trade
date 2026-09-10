package user

import (
	"os"
	"testing"
	"time"

	"common/middleware/db"
	userDTO "service/manager_user/dto"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 真实 MySQL 下验资金账户的写路径。
//
// 为什么必须是集成测试：/accounts 这两个接口以前**根本不存在**（resource_new 里
// 有权限行，路由却没实现），用户管理页的充值与冻结按钮点下去是 404。补完之后
// 要证明的不是"函数返回了对象"，而是"值真的按 decimal(38,8) 落进了 account 表
// 且没丢精度"——这件事只有连真库才验得了。
//
// account 表是手工建的（id 是 signed bigint，与 user/user_role 的 unsigned 不同），
// 不参与 AutoMigrate，所以这里自己建。
func accountIntegrationService(t *testing.T) (*UserService, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("ARGUS_EVENT_TEST_DSN")
	if dsn == "" {
		t.Skip("未设置 ARGUS_EVENT_TEST_DSN，跳过真实 MySQL 集成校验")
	}
	gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	if err != nil {
		t.Fatalf("打开连接失败: %v", err)
	}
	// 与生产同构：id signed bigint、user_id unsigned、balance decimal(38,8) NOT NULL。
	if err := gdb.Exec(`CREATE TABLE IF NOT EXISTS account (
		id bigint NOT NULL AUTO_INCREMENT,
		active tinyint DEFAULT 1,
		created_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_time timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		created_by longtext, updated_by longtext,
		user_id bigint unsigned DEFAULT NULL,
		account_status varchar(32) DEFAULT NULL,
		balance_amount decimal(38,8) NOT NULL DEFAULT '0.00000000',
		PRIMARY KEY (id), KEY idx_user_id (user_id)
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatalf("建 account 表失败: %v", err)
	}

	db.Db = gdb
	svc := NewUserService()
	// 仓储是全局单例，前面的用例可能已经拿到过旧连接，强制指向本次连接。
	svc.userRepository.SetDb(gdb)
	svc.accountRepository.SetDb(gdb)
	if err := svc.userRepository.EnsureTable(); err != nil {
		t.Fatalf("建 user 表失败: %v", err)
	}
	// 每个用例都从干净状态起：只清 account 不够，seedUser 造的用户会留下，
	// 下一次 CreateUser 直接撞 "username already exists"。
	gdb.Exec("DELETE FROM account")
	gdb.Exec("DELETE FROM user WHERE username LIKE 'acct-it-%'")
	return svc, gdb
}

// seedUser 造一个生效用户，返回其 id。CreateAccount 会校验用户存在。
//
// LastLoginTime 必须显式给：CreateUser 收到零值 time.Time 会往
// last_login_time 写 '0000-00-00'，生产库的 sql_mode 不含 NO_ZERO_DATE 所以
// 能存进去，而 MySQL 8 默认（也就是 CI 容器）是严格的，直接 Error 1292。
// 这是 CreateUser 自身的可移植性问题，与资金账户无关，这里绕开它以免噪声
// 盖住本用例真正要验的东西。
func seedUser(t *testing.T, svc *UserService, username string) uint64 {
	t.Helper()
	created, err := svc.CreateUser(&userDTO.CreateUserDTO{
		Name: "集成测试用户", Username: username, Role: "admin", Status: "active",
		LastLoginTime: time.Now(),
		// pub_token 上有唯一索引，而字段是非指针 string，不给就写空串。
		// MySQL 里空串是一个值（不像 NULL），所以第二个不带 pub_token 的用户
		// 必然撞 Duplicate entry '' for key 'user.pub_token'。这同样是
		// CreateUser 自身的问题（生产只有 1 个用户所以没撞上），这里给唯一值绕开。
		PubToken: "tok-" + username,
	})
	if err != nil {
		t.Fatalf("造用户失败: %v", err)
	}
	return uint64(created.Id)
}

func TestIntegrationAccountCreateThenUpdateBalance(t *testing.T) {
	svc, gdb := accountIntegrationService(t)
	uid := seedUser(t, svc, "acct-it-balance")

	created, err := svc.CreateAccount(&userDTO.CreateAccountDTO{
		UserID: uid, AccountStatus: "normal", BalanceAmount: "100.00",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if created.Id == 0 {
		t.Fatal("新建账户应回传 id —— 前端 createAccount 取的就是 data.id")
	}
	if created.BalanceAmount != "100.00000000" {
		t.Errorf("余额 = %s，期望 100.00000000", created.BalanceAmount)
	}

	// 充值：前端算好「当前 + 充值额」再发过来
	updated, err := svc.UpdateAccount(created.Id, &userDTO.UpdateAccountDTO{
		BalanceAmount: strPtr("100.30"),
	})
	if err != nil {
		t.Fatalf("UpdateAccount: %v", err)
	}
	if updated.BalanceAmount != "100.30000000" {
		t.Errorf("充值后余额 = %s，期望 100.30000000", updated.BalanceAmount)
	}
	// 只发了 balanceAmount，状态不能被顺手清掉
	if updated.AccountStatus != "normal" {
		t.Errorf("只改余额时状态不该变，实际 %q", updated.AccountStatus)
	}

	// 直接查库确认落的是 decimal 而不是被截断/四舍五入过的值
	var raw string
	if err := gdb.Raw("SELECT balance_amount FROM account WHERE id = ?", created.Id).Scan(&raw).Error; err != nil {
		t.Fatalf("查库: %v", err)
	}
	if raw != "100.30000000" {
		t.Errorf("库里的 balance_amount = %s，期望 100.30000000", raw)
	}
}

func TestIntegrationAccountToggleStatus(t *testing.T) {
	svc, _ := accountIntegrationService(t)
	uid := seedUser(t, svc, "acct-it-status")

	created, err := svc.CreateAccount(&userDTO.CreateAccountDTO{
		UserID: uid, AccountStatus: "normal", BalanceAmount: "8.25",
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	frozen, err := svc.UpdateAccount(created.Id, &userDTO.UpdateAccountDTO{AccountStatus: strPtr("frozen")})
	if err != nil {
		t.Fatalf("冻结: %v", err)
	}
	if frozen.AccountStatus != "frozen" {
		t.Errorf("状态 = %q，期望 frozen", frozen.AccountStatus)
	}
	// 只发了 accountStatus，余额不能被清零
	if frozen.BalanceAmount != "8.25000000" {
		t.Errorf("只改状态时余额不该变，实际 %s", frozen.BalanceAmount)
	}

	back, err := svc.UpdateAccount(created.Id, &userDTO.UpdateAccountDTO{AccountStatus: strPtr("normal")})
	if err != nil {
		t.Fatalf("解冻: %v", err)
	}
	if back.AccountStatus != "normal" {
		t.Errorf("解冻后状态 = %q", back.AccountStatus)
	}
}

func TestIntegrationAccountRejectsDuplicateAndBadInput(t *testing.T) {
	svc, _ := accountIntegrationService(t)
	uid := seedUser(t, svc, "acct-it-dup")

	if _, err := svc.CreateAccount(&userDTO.CreateAccountDTO{
		UserID: uid, AccountStatus: "normal", BalanceAmount: "1",
	}); err != nil {
		t.Fatalf("首次建账户: %v", err)
	}
	// 一个用户只允许一个生效账户：前端按 record.accountId 为空才新建来写的，
	// 真有两条，用户列表的余额就会取到不确定的那一条。
	if _, err := svc.CreateAccount(&userDTO.CreateAccountDTO{
		UserID: uid, AccountStatus: "normal", BalanceAmount: "2",
	}); err == nil {
		t.Error("同一用户重复建账户应被拒绝")
	}
	// 用户不存在
	if _, err := svc.CreateAccount(&userDTO.CreateAccountDTO{
		UserID: 999999, AccountStatus: "normal", BalanceAmount: "1",
	}); err == nil {
		t.Error("用户不存在时应拒绝，否则会造出挂不到人身上的账户")
	}
	// 更新不存在的账户
	if _, err := svc.UpdateAccount(999999, &userDTO.UpdateAccountDTO{AccountStatus: strPtr("frozen")}); err == nil {
		t.Error("更新不存在的账户应报错")
	}
	// 两个字段都不给
	if _, err := svc.UpdateAccount(1, &userDTO.UpdateAccountDTO{}); err == nil {
		t.Error("什么都不改的更新应报错，而不是静默成功")
	}
}

func strPtr(v string) *string { return &v }
