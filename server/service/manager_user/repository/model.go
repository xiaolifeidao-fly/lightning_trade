package repository

import (
	"common/middleware/db"
	"github.com/shopspring/decimal"
	"time"
)

type User struct {
	db.BaseEntity
	Name       string `gorm:"column:name;type:varchar(100);index:idx_name" orm:"column(name);size(100);null" description:"姓名"`
	Username   string `gorm:"column:username;type:varchar(50);uniqueIndex:idx_username" orm:"column(username);size(50);null" description:"用户名"`
	Email      string `gorm:"column:email;type:varchar(100);index:idx_email" orm:"column(email);size(100);null" description:"邮箱"`
	Phone      string `gorm:"column:phone;type:varchar(32);index:idx_phone" orm:"column(phone);size(32);null" description:"手机号"`
	Department string `gorm:"column:department;type:varchar(100);index:idx_department" orm:"column(department);size(100);null" description:"部门"`
	Role       string `gorm:"column:role;type:varchar(50);index:idx_role" orm:"column(role);size(50);null" description:"角色"`
	Password   string `gorm:"column:password;type:varchar(50)" orm:"column(password);size(50);null" description:"密码（encryptPassword 后的摘要，登录校验用）"`
	// origin_password（明文口令）字段已从实体移除：登录只校验 Password 摘要，
	// 明文历来只用于列表页展示，属纯风险项。列还留在库里但代码不再读写，
	// 存量明文需要另跑一条 UPDATE 清空。
	Status string `gorm:"column:status;type:varchar(50)" orm:"column(status);size(50);null" description:"状态"`
	// 指针类型：从没登录过就该是 NULL，不是 '0000-00-00'。
	// 非指针 time.Time 的零值会被写成 '0000-00-00 00:00:00'——生产库的 sql_mode
	// 不含 NO_ZERO_DATE 所以能存进去，而 MySQL 8 默认（含 CI 容器）直接
	// Error 1292 Incorrect datetime value。前端新建用户表单根本不发这个字段
	// （UserPayload 里都没有），所以走界面建的每个用户都在写这个脏日期。
	// repository.go 的统计查询里那句 "last_login_time > '1970-01-02'"
	// 就是当初为了绕开它加的。
	LastLoginTime *time.Time `gorm:"column:last_login_time;type:datetime" orm:"column(last_login_time);null" description:"最后登录时间"`
	SecretKey     string     `gorm:"column:secret_key;type:varchar(50);index:idx_secret_key" orm:"column(secret_key);size(50);null" description:"密钥"`
	Remark        string     `gorm:"column:remark;type:varchar(50)" orm:"column(remark);size(50);null" description:"备注"`
	// 指针类型是必须的，不是风格问题：pub_token 上有唯一索引，而 MySQL 的唯一
	// 索引**不约束 NULL**、却把空串当成一个值。用非指针 string 时未设置就写 ''，
	// 于是第二个不带 token 的用户必然撞
	// Duplicate entry '' for key 'user.pub_token' —— 生产只有 1 个用户所以一直没暴露。
	// 改成 *string 后：没有 token 就存 NULL（可以有任意多个），真有 token 时唯一性照旧生效。
	PubToken *string `gorm:"column:pub_token;type:varchar(100);uniqueIndex:pub_token" orm:"column(pub_token);size(100);null" description:"公钥token"`
	BanCount uint32  `gorm:"column:ban_count;type:int unsigned;default:0" orm:"column(ban_count);null" description:"封禁次数"`
}

func (u *User) TableName() string {
	return "user"
}

type UserLoginRecord struct {
	db.BaseEntity
	IP     string `gorm:"column:ip;type:varchar(50)" orm:"column(ip);size(50);null" description:"登录IP"`
	UserID uint64 `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
}

func (u *UserLoginRecord) TableName() string {
	return "user_login_record"
}

type UserRole struct {
	db.BaseEntity
	UserID uint64 `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	RoleID uint64 `gorm:"column:role_id;type:bigint unsigned" orm:"column(role_id);null" description:"角色ID"`
}

func (u *UserRole) TableName() string {
	return "user_role"
}

type UserListRow struct {
	db.BaseEntity
	Name          string     `gorm:"column:name"`
	Username      string     `gorm:"column:username"`
	Email         string     `gorm:"column:email"`
	Phone         string     `gorm:"column:phone"`
	Department    string     `gorm:"column:department"`
	Role          string     `gorm:"column:role"`
	Status        string     `gorm:"column:status"`
	LastLoginTime *time.Time `gorm:"column:last_login_time"`
	SecretKey     string     `gorm:"column:secret_key"`
	Remark        string     `gorm:"column:remark"`
	PubToken      string     `gorm:"column:pub_token"`
	BanCount      uint32     `gorm:"column:ban_count"`
}

// Account 用户资金账户。用户列表里的「资金账户 / 钱包总额」以及用户管理页的
// 充值、冻结/解冻都落在这张表上。
//
// 注意这张表是**手工建的**，不是 AutoMigrate 建的：它的 id 是 signed bigint，
// 而 user / user_role 都是 bigint unsigned。因此本模型只用于读写，不要挂进
// AutoMigrate——让 GORM 去"纠正"主键列类型没有任何收益，只有风险。
type Account struct {
	db.BaseEntity
	UserID        uint64          `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" description:"所属用户"`
	AccountStatus string          `gorm:"column:account_status;type:varchar(32)" description:"账户状态：normal / frozen"`
	BalanceAmount decimal.Decimal `gorm:"column:balance_amount;type:decimal(38,8);not null;default:0" description:"余额，decimal 存储不用浮点"`
}

func (a *Account) TableName() string {
	return "account"
}

type UserAccountRow struct {
	ID            int    `gorm:"column:id"`
	UserID        int    `gorm:"column:user_id"`
	AccountStatus string `gorm:"column:account_status"`
	BalanceAmount string `gorm:"column:balance_amount"`
}
