package user

import (
	"os"
	"testing"

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
	gdb.Exec("DELETE FROM user WHERE username LIKE 'acct-it-notok-%' OR username LIKE 'acct-it-tok-%' OR username = 'acct-it-nologin'")
	return svc, gdb
}

// seedUser 造一个生效用户，返回其 id。CreateAccount 会校验用户存在。
//
// 刻意**不传** LastLoginTime，走的就是前端新建用户表单那条路径
// （UserPayload 里根本没有这个字段）。以前不传会往 last_login_time 写
// '0000-00-00'，在严格 sql_mode 下直接 Error 1292；现在零值存 NULL。
func seedUser(t *testing.T, svc *UserService, username string) uint64 {
	t.Helper()
	created, err := svc.CreateUser(&userDTO.CreateUserDTO{
		Name: "集成测试用户", Username: username, Role: "admin", Status: "active",
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

// 两个都不带 pub_token 的用户必须都能建出来。
//
// 这是本仓库真实存在过的缺陷：User.PubToken 原本是非指针 string，未设置就写空串，
// 而 pub_token 上有唯一索引。MySQL 的唯一索引不约束 NULL，却把空串当成一个值，
// 于是**第二个**不带 token 的用户必然撞
//
//	Duplicate entry '' for key 'user.pub_token'
//
// 生产库当时只有 1 个用户，所以这个"新建用户只能用一次"的问题一直没暴露。
// 前端的新建用户表单根本没有 pub_token 输入项，后端也不自动生成，
// 所以走界面建的每个用户都会踩。
func TestIntegrationCreateMultipleUsersWithoutPubToken(t *testing.T) {
	svc, gdb := accountIntegrationService(t)

	mk := func(username string) error {
		_, err := svc.CreateUser(&userDTO.CreateUserDTO{
			Name: "无 token 用户", Username: username, Role: "admin", Status: "active",
		})
		return err
	}
	if err := mk("acct-it-notok-1"); err != nil {
		t.Fatalf("第一个用户就建不出来: %v", err)
	}
	if err := mk("acct-it-notok-2"); err != nil {
		t.Fatalf("第二个不带 pub_token 的用户也必须能建出来（原缺陷正是在这一步炸）: %v", err)
	}

	// 空 token 必须落成 NULL 而不是空串，否则唯一索引照样会撞。
	var nulls int64
	if err := gdb.Raw("SELECT COUNT(*) FROM user WHERE username LIKE 'acct-it-notok-%' AND pub_token IS NULL").Scan(&nulls).Error; err != nil {
		t.Fatalf("查库: %v", err)
	}
	if nulls != 2 {
		var empties int64
		gdb.Raw("SELECT COUNT(*) FROM user WHERE username LIKE 'acct-it-notok-%' AND pub_token = ''").Scan(&empties)
		t.Fatalf("空 token 应存 NULL，实际 NULL=%d 空串=%d", nulls, empties)
	}

	// 真有 token 时唯一性必须照旧生效——修复不能把约束一起弄丢了。
	tok := "dup-token-shared"
	if _, err := svc.CreateUser(&userDTO.CreateUserDTO{
		Name: "带 token", Username: "acct-it-tok-1", Role: "admin", Status: "active",
		PubToken: tok,
	}); err != nil {
		t.Fatalf("带 token 的用户应能建出来: %v", err)
	}
	if _, err := svc.CreateUser(&userDTO.CreateUserDTO{
		Name: "带同样的 token", Username: "acct-it-tok-2", Role: "admin", Status: "active",
		PubToken: tok,
	}); err == nil {
		t.Error("重复的 pub_token 必须被唯一索引挡住，否则这次修复等于把约束删了")
	}

	// 列表查询必须能扫过 NULL：UserListRow.PubToken 是 string，
	// 直接 SELECT 会报 converting NULL to string is unsupported，所以 SQL 里用了 IFNULL。
	rows, err := svc.ListUsers(userDTO.UserQueryDTO{PageIndex: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("列表查询在有 NULL token 时炸了: %v", err)
	}
	// 3 个而不是 4 个：acct-it-tok-2 用了重复 token，本来就该被唯一索引挡住。
	if rows == nil {
		t.Fatal("列表返回 nil")
	}
	if rows.Total < 3 {
		t.Errorf("列表应至少列出成功建出的 3 个用户，实际 total=%d", rows.Total)
	}
}

// 不传 LastLoginTime 时必须存 NULL，而不是 '0000-00-00'。
//
// 前端新建用户表单**根本不发**这个字段（UserPayload 里都没有），所以走界面
// 建的每个用户都会命中这条路径。原来 CreateUser 写的是零值 time.Time，
// 落库成 '0000-00-00 00:00:00'：
//   - 生产库 sql_mode 不含 NO_ZERO_DATE，能存进去，但那是个脏日期
//   - MySQL 8 默认（含本 CI 容器）严格，直接 Error 1292 Incorrect datetime value
//
// repository.go 统计查询里那句 last_login_time > '1970-01-02' 就是当初为了
// 绕开这个脏值加的。
func TestIntegrationCreateUserWithoutLoginTimeStoresNull(t *testing.T) {
	svc, gdb := accountIntegrationService(t)

	created, err := svc.CreateUser(&userDTO.CreateUserDTO{
		Name: "没登录过的用户", Username: "acct-it-nologin", Role: "admin", Status: "active",
	})
	if err != nil {
		t.Fatalf("不传 LastLoginTime 应能建出用户（严格 sql_mode 下原实现会 Error 1292）: %v", err)
	}

	var isNull bool
	if err := gdb.Raw("SELECT last_login_time IS NULL FROM user WHERE id = ?", created.Id).Scan(&isNull).Error; err != nil {
		t.Fatalf("查库: %v", err)
	}
	if !isNull {
		var raw string
		gdb.Raw("SELECT CAST(last_login_time AS CHAR) FROM user WHERE id = ?", created.Id).Scan(&raw)
		t.Errorf("没登录过应存 NULL，实际存了 %q", raw)
	}
	// 出参里也不该冒出假日期
	if created.LastLoginTime != nil {
		t.Errorf("没登录过时出参应为 null，实际 %v", *created.LastLoginTime)
	}

	// 列表要能扫过 NULL：UserListRow.LastLoginTime 也已改成指针
	rows, err := svc.ListUsers(userDTO.UserQueryDTO{PageIndex: 1, PageSize: 50})
	if err != nil {
		t.Fatalf("列表在有 NULL 登录时间时炸了: %v", err)
	}
	if rows == nil || rows.Total < 1 {
		t.Error("列表应能列出刚建的用户")
	}
}
