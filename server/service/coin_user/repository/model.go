package repository

import (
	"common/middleware/db"
	"time"
)

type CoinUser struct {
	db.BaseEntity
	Username      string    `gorm:"column:username;type:varchar(64);uniqueIndex:idx_username" orm:"column(username);size(64);null" description:"用户名"`
	Nickname      string    `gorm:"column:nickname;type:varchar(64)" orm:"column(nickname);size(64);null" description:"昵称"`
	Email         string    `gorm:"column:email;type:varchar(128);index:idx_email" orm:"column(email);size(128);null" description:"邮箱"`
	Phone         string    `gorm:"column:phone;type:varchar(32);index:idx_phone" orm:"column(phone);size(32);null" description:"手机号"`
	Password      string    `gorm:"column:password;type:varchar(128)" orm:"column(password);size(128);null" description:"密码"`
	Country       string    `gorm:"column:country;type:varchar(32)" orm:"column(country);size(32);null" description:"国家"`
	KycLevel      uint8     `gorm:"column:kyc_level;type:tinyint unsigned;default:0" orm:"column(kyc_level);null" description:"KYC等级(0未认证 1初级 2高级)"`
	KycStatus     string    `gorm:"column:kyc_status;type:varchar(32);index:idx_kyc_status" orm:"column(kyc_status);size(32);null" description:"KYC状态: pending/approved/rejected"`
	Status        string    `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"账户状态: active/locked/frozen"`
	InviteCode    string    `gorm:"column:invite_code;type:varchar(32);uniqueIndex:idx_invite_code" orm:"column(invite_code);size(32);null" description:"邀请码"`
	InviterID     uint64    `gorm:"column:inviter_id;type:bigint unsigned;default:0;index:idx_inviter_id" orm:"column(inviter_id);null" description:"邀请人ID"`
	LastLoginIP   string    `gorm:"column:last_login_ip;type:varchar(64)" orm:"column(last_login_ip);size(64);null" description:"最后登录IP"`
	LastLoginTime time.Time `gorm:"column:last_login_time;type:datetime" orm:"column(last_login_time);null" description:"最后登录时间"`
	GoogleAuthKey string    `gorm:"column:google_auth_key;type:varchar(64)" orm:"column(google_auth_key);size(64);null" description:"谷歌验证密钥"`
	TwoFAEnabled  uint8     `gorm:"column:two_fa_enabled;type:tinyint unsigned;default:0" orm:"column(two_fa_enabled);null" description:"是否开启二次验证"`
	Remark        string    `gorm:"column:remark;type:varchar(255)" orm:"column(remark);size(255);null" description:"备注"`
}

func (u *CoinUser) TableName() string {
	return "coin_user"
}

type CoinUserAsset struct {
	db.BaseEntity
	UserID         uint64  `gorm:"column:user_id;type:bigint unsigned;uniqueIndex:idx_user_coin,priority:1" orm:"column(user_id);null" description:"用户ID"`
	CoinID         uint64  `gorm:"column:coin_id;type:bigint unsigned;uniqueIndex:idx_user_coin,priority:2" orm:"column(coin_id);null" description:"币种ID"`
	CoinCode       string  `gorm:"column:coin_code;type:varchar(32);index:idx_coin_code" orm:"column(coin_code);size(32);null" description:"币种代码"`
	Available      float64 `gorm:"column:available;type:decimal(36,18);default:0" orm:"column(available);null" description:"可用余额"`
	Frozen         float64 `gorm:"column:frozen;type:decimal(36,18);default:0" orm:"column(frozen);null" description:"冻结余额"`
	Total          float64 `gorm:"column:total;type:decimal(36,18);default:0" orm:"column(total);null" description:"总余额"`
	Address        string  `gorm:"column:address;type:varchar(128)" orm:"column(address);size(128);null" description:"充值地址"`
	WithdrawEnable uint8   `gorm:"column:withdraw_enable;type:tinyint unsigned;default:1" orm:"column(withdraw_enable);null" description:"是否允许提现"`
}

func (a *CoinUserAsset) TableName() string {
	return "coin_user_asset"
}

type CoinUserLoginRecord struct {
	db.BaseEntity
	UserID   uint64 `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	IP       string `gorm:"column:ip;type:varchar(64)" orm:"column(ip);size(64);null" description:"登录IP"`
	Device   string `gorm:"column:device;type:varchar(128)" orm:"column(device);size(128);null" description:"设备信息"`
	Location string `gorm:"column:location;type:varchar(128)" orm:"column(location);size(128);null" description:"位置"`
	Success  uint8  `gorm:"column:success;type:tinyint unsigned;default:1" orm:"column(success);null" description:"是否成功"`
}

func (r *CoinUserLoginRecord) TableName() string {
	return "coin_user_login_record"
}

type CoinUserListRow struct {
	db.BaseEntity
	Username      string    `gorm:"column:username"`
	Nickname      string    `gorm:"column:nickname"`
	Email         string    `gorm:"column:email"`
	Phone         string    `gorm:"column:phone"`
	Country       string    `gorm:"column:country"`
	KycLevel      uint8     `gorm:"column:kyc_level"`
	KycStatus     string    `gorm:"column:kyc_status"`
	Status        string    `gorm:"column:status"`
	InviteCode    string    `gorm:"column:invite_code"`
	InviterID     uint64    `gorm:"column:inviter_id"`
	LastLoginIP   string    `gorm:"column:last_login_ip"`
	LastLoginTime time.Time `gorm:"column:last_login_time"`
	TwoFAEnabled  uint8     `gorm:"column:two_fa_enabled"`
	Remark        string    `gorm:"column:remark"`
}
