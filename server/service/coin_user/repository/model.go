package repository

import (
	"common/middleware/db"
	"time"
)

type CoinUser struct {
	db.BaseEntity
	PlatformID    uint64    `gorm:"column:platform_id;type:bigint unsigned;default:0;index:idx_platform_id" orm:"column(platform_id);null" description:"所属平台ID"`
	PlatformCode  string    `gorm:"column:platform_code;type:varchar(32);index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"所属平台代码"`
	Account       string    `gorm:"column:account;type:varchar(128);index:idx_account" orm:"column(account);size(128);null" description:"账号(平台账户标识)"`
	Username      string    `gorm:"column:username;type:varchar(64);uniqueIndex:idx_username" orm:"column(username);size(64);null" description:"用户名"`
	Nickname      string    `gorm:"column:nickname;type:varchar(64)" orm:"column(nickname);size(64);null" description:"昵称"`
	Email         string    `gorm:"column:email;type:varchar(128);index:idx_email" orm:"column(email);size(128);null" description:"邮箱"`
	Phone         string    `gorm:"column:phone;type:varchar(32);index:idx_phone" orm:"column(phone);size(32);null" description:"手机号"`
	Password      string    `gorm:"column:password;type:varchar(128)" orm:"column(password);size(128);null" description:"密码"`
	SecretKey     string    `gorm:"column:secret_key;type:varchar(512)" orm:"column(secret_key);size(512);null" description:"秘钥(加密存储)"`
	Balance       float64   `gorm:"column:balance;type:decimal(36,18);default:0" orm:"column(balance);null" description:"账户总余额(USDT)"`
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

type CoinUserPosition struct {
	db.BaseEntity
	UserID        uint64  `gorm:"column:user_id;type:bigint unsigned;uniqueIndex:idx_user_symbol,priority:1;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	Symbol        string  `gorm:"column:symbol;type:varchar(32);uniqueIndex:idx_user_symbol,priority:2;index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对 BTC-USDT"`
	BaseCoinCode  string  `gorm:"column:base_coin_code;type:varchar(32)" orm:"column(base_coin_code);size(32);null" description:"基础币种"`
	QuoteCoinCode string  `gorm:"column:quote_coin_code;type:varchar(32)" orm:"column(quote_coin_code);size(32);null" description:"计价币种"`
	Amount        float64 `gorm:"column:amount;type:decimal(36,18);default:0" orm:"column(amount);null" description:"持仓数量"`
	AvgCostPrice  float64 `gorm:"column:avg_cost_price;type:decimal(36,18);default:0" orm:"column(avg_cost_price);null" description:"平均成本价"`
	TotalCost     float64 `gorm:"column:total_cost;type:decimal(36,18);default:0" orm:"column(total_cost);null" description:"持仓总成本"`
	Status        string  `gorm:"column:status;type:varchar(16);default:open;index:idx_status" orm:"column(status);size(16);null" description:"状态 open/closed"`
}

func (p *CoinUserPosition) TableName() string {
	return "coin_user_position"
}

type CoinUserPositionAnalysis struct {
	db.BaseEntity
	UserID           uint64  `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	PositionID       uint64  `gorm:"column:position_id;type:bigint unsigned;default:0;index:idx_position_id" orm:"column(position_id);null" description:"关联仓位ID"`
	Symbol           string  `gorm:"column:symbol;type:varchar(32);index:idx_symbol" orm:"column(symbol);size(32);null" description:"币种/交易对 BTC-USDT"`
	Side             string  `gorm:"column:side;type:varchar(8)" orm:"column(side);size(8);null" description:"开仓方向 long/short"`
	AvgPrice         float64 `gorm:"column:avg_price;type:decimal(36,18);default:0" orm:"column(avg_price);null" description:"仓位平均价格"`
	LiquidationPrice float64 `gorm:"column:liquidation_price;type:decimal(36,18);default:0" orm:"column(liquidation_price);null" description:"爆仓价格"`
	Leverage         float64 `gorm:"column:leverage;type:decimal(10,2);default:1" orm:"column(leverage);null" description:"开仓倍数"`
	Contracts        float64 `gorm:"column:contracts;type:decimal(36,18);default:0" orm:"column(contracts);null" description:"开仓张数"`
	Margin           float64 `gorm:"column:margin;type:decimal(36,18);default:0" orm:"column(margin);null" description:"保证金"`
	BalanceAtOpen    float64 `gorm:"column:balance_at_open;type:decimal(36,18);default:0" orm:"column(balance_at_open);null" description:"开仓时用户余额"`
	AiAdvice         string  `gorm:"column:ai_advice;type:text" orm:"column(ai_advice);null" description:"AI建议"`
}

func (a *CoinUserPositionAnalysis) TableName() string {
	return "coin_user_position_analysis"
}

type CoinUserListRow struct {
	db.BaseEntity
	PlatformID    uint64    `gorm:"column:platform_id"`
	PlatformCode  string    `gorm:"column:platform_code"`
	Account       string    `gorm:"column:account"`
	Username      string    `gorm:"column:username"`
	SecretKey     string    `gorm:"column:secret_key"`
	Balance       float64   `gorm:"column:balance"`
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
