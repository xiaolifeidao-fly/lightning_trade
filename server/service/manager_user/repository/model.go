package repository

import (
	"common/middleware/db"
	"time"
)

type User struct {
	db.BaseEntity
	Name           string    `gorm:"column:name;type:varchar(100);index:idx_name" orm:"column(name);size(100);null" description:"姓名"`
	Username       string    `gorm:"column:username;type:varchar(50);uniqueIndex:idx_username" orm:"column(username);size(50);null" description:"用户名"`
	Email          string    `gorm:"column:email;type:varchar(100);index:idx_email" orm:"column(email);size(100);null" description:"邮箱"`
	Phone          string    `gorm:"column:phone;type:varchar(32);index:idx_phone" orm:"column(phone);size(32);null" description:"手机号"`
	Department     string    `gorm:"column:department;type:varchar(100);index:idx_department" orm:"column(department);size(100);null" description:"部门"`
	Role           string    `gorm:"column:role;type:varchar(50);index:idx_role" orm:"column(role);size(50);null" description:"角色"`
	Password       string    `gorm:"column:password;type:varchar(50)" orm:"column(password);size(50);null" description:"密码"`
	OriginPassword string    `gorm:"column:origin_password;type:varchar(50)" orm:"column(origin_password);size(50);null" description:"原始密码"`
	Status         string    `gorm:"column:status;type:varchar(50)" orm:"column(status);size(50);null" description:"状态"`
	LastLoginTime  time.Time `gorm:"column:last_login_time;type:datetime" orm:"column(last_login_time);null" description:"最后登录时间"`
	SecretKey      string    `gorm:"column:secret_key;type:varchar(50);index:idx_secret_key" orm:"column(secret_key);size(50);null" description:"密钥"`
	Remark         string    `gorm:"column:remark;type:varchar(50)" orm:"column(remark);size(50);null" description:"备注"`
	PubToken       string    `gorm:"column:pub_token;type:varchar(100);uniqueIndex:pub_token" orm:"column(pub_token);size(100);null" description:"公钥token"`
	BanCount       uint32    `gorm:"column:ban_count;type:int unsigned;default:0" orm:"column(ban_count);null" description:"封禁次数"`
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
	Name           string    `gorm:"column:name"`
	Username       string    `gorm:"column:username"`
	Email          string    `gorm:"column:email"`
	Phone          string    `gorm:"column:phone"`
	Department     string    `gorm:"column:department"`
	Role           string    `gorm:"column:role"`
	Password       string    `gorm:"column:password"`
	OriginPassword string    `gorm:"column:origin_password"`
	Status         string    `gorm:"column:status"`
	LastLoginTime  time.Time `gorm:"column:last_login_time"`
	SecretKey      string    `gorm:"column:secret_key"`
	Remark         string    `gorm:"column:remark"`
	PubToken       string    `gorm:"column:pub_token"`
	BanCount       uint32    `gorm:"column:ban_count"`
}

type UserAccountRow struct {
	ID            int    `gorm:"column:id"`
	UserID        int    `gorm:"column:user_id"`
	AccountStatus string `gorm:"column:account_status"`
	BalanceAmount string `gorm:"column:balance_amount"`
}
