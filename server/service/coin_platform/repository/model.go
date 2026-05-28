package repository

import (
	"common/middleware/db"
	"time"
)

type CoinPlatform struct {
	db.BaseEntity
	Code            string `gorm:"column:code;type:varchar(32);uniqueIndex:idx_code" orm:"column(code);size(32);null" description:"平台代码 binance/okx/huobi"`
	Name            string `gorm:"column:name;type:varchar(64);index:idx_name" orm:"column(name);size(64);null" description:"平台名称"`
	FullName        string `gorm:"column:full_name;type:varchar(128)" orm:"column(full_name);size(128);null" description:"平台全称"`
	Icon            string `gorm:"column:icon;type:varchar(255)" orm:"column(icon);size(255);null" description:"平台图标"`
	Website         string `gorm:"column:website;type:varchar(255)" orm:"column(website);size(255);null" description:"官网地址"`
	Country         string `gorm:"column:country;type:varchar(64);index:idx_country" orm:"column(country);size(64);null" description:"所在国家"`
	ApiBaseURL      string `gorm:"column:api_base_url;type:varchar(255)" orm:"column(api_base_url);size(255);null" description:"API 基础地址"`
	WsBaseURL       string `gorm:"column:ws_base_url;type:varchar(255)" orm:"column(ws_base_url);size(255);null" description:"WebSocket 基础地址"`
	DocsURL         string `gorm:"column:docs_url;type:varchar(255)" orm:"column(docs_url);size(255);null" description:"接入文档地址"`
	SupportedTypes  string `gorm:"column:supported_types;type:varchar(255)" orm:"column(supported_types);size(255);null" description:"支持的业务类型 spot,futures,margin"`
	DefaultFeeRate  float64 `gorm:"column:default_fee_rate;type:decimal(10,6);default:0" orm:"column(default_fee_rate);null" description:"默认手续费率"`
	RateLimitPerSec uint32 `gorm:"column:rate_limit_per_sec;type:int unsigned;default:0" orm:"column(rate_limit_per_sec);null" description:"每秒限频"`
	Status          string `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"状态 online/offline/maintenance"`
	SortOrder       int    `gorm:"column:sort_order;type:int;default:0" orm:"column(sort_order);null" description:"排序"`
	Description     string `gorm:"column:description;type:varchar(500)" orm:"column(description);size(500);null" description:"描述"`
}

func (p *CoinPlatform) TableName() string {
	return "coin_platform"
}

type CoinPlatformCoin struct {
	db.BaseEntity
	PlatformID      uint64  `gorm:"column:platform_id;type:bigint unsigned;uniqueIndex:idx_platform_coin,priority:1" orm:"column(platform_id);null" description:"平台ID"`
	PlatformCode    string  `gorm:"column:platform_code;type:varchar(32);index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"平台代码"`
	CoinID          uint64  `gorm:"column:coin_id;type:bigint unsigned;uniqueIndex:idx_platform_coin,priority:2" orm:"column(coin_id);null" description:"币种ID"`
	CoinCode        string  `gorm:"column:coin_code;type:varchar(32);index:idx_coin_code" orm:"column(coin_code);size(32);null" description:"币种代码"`
	PlatformSymbol  string  `gorm:"column:platform_symbol;type:varchar(64)" orm:"column(platform_symbol);size(64);null" description:"平台侧符号"`
	ChainName       string  `gorm:"column:chain_name;type:varchar(32);index:idx_chain_name" orm:"column(chain_name);size(32);null" description:"主链"`
	ContractAddress string  `gorm:"column:contract_address;type:varchar(128)" orm:"column(contract_address);size(128);null" description:"合约地址"`
	DepositEnable   uint8   `gorm:"column:deposit_enable;type:tinyint unsigned;default:1" orm:"column(deposit_enable);null" description:"是否可充值"`
	WithdrawEnable  uint8   `gorm:"column:withdraw_enable;type:tinyint unsigned;default:1" orm:"column(withdraw_enable);null" description:"是否可提现"`
	TradeEnable     uint8   `gorm:"column:trade_enable;type:tinyint unsigned;default:1" orm:"column(trade_enable);null" description:"是否可交易"`
	MinWithdrawal   float64 `gorm:"column:min_withdrawal;type:decimal(36,18);default:0" orm:"column(min_withdrawal);null" description:"最小提现额"`
	WithdrawalFee   float64 `gorm:"column:withdrawal_fee;type:decimal(36,18);default:0" orm:"column(withdrawal_fee);null" description:"提现手续费"`
	Confirmations   uint32  `gorm:"column:confirmations;type:int unsigned;default:0" orm:"column(confirmations);null" description:"充值所需确认数"`
}

func (c *CoinPlatformCoin) TableName() string {
	return "coin_platform_coin"
}

type CoinPlatformAccount struct {
	db.BaseEntity
	PlatformID    uint64    `gorm:"column:platform_id;type:bigint unsigned;index:idx_platform_id" orm:"column(platform_id);null" description:"平台ID"`
	PlatformCode  string    `gorm:"column:platform_code;type:varchar(32);index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"平台代码"`
	AccountName   string    `gorm:"column:account_name;type:varchar(64);index:idx_account_name" orm:"column(account_name);size(64);null" description:"账户名称/标签"`
	AccountType   string    `gorm:"column:account_type;type:varchar(32)" orm:"column(account_type);size(32);null" description:"账户类型 master/sub/read_only"`
	ApiKey        string    `gorm:"column:api_key;type:varchar(255)" orm:"column(api_key);size(255);null" description:"API Key"`
	ApiSecret     string    `gorm:"column:api_secret;type:varchar(512)" orm:"column(api_secret);size(512);null" description:"API Secret(加密存储)"`
	Passphrase    string    `gorm:"column:passphrase;type:varchar(255)" orm:"column(passphrase);size(255);null" description:"OKX 等需要的口令"`
	IPWhitelist   string    `gorm:"column:ip_whitelist;type:varchar(500)" orm:"column(ip_whitelist);size(500);null" description:"IP白名单 逗号分隔"`
	Permissions   string    `gorm:"column:permissions;type:varchar(255)" orm:"column(permissions);size(255);null" description:"权限 read/trade/withdraw"`
	Status        string    `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"状态 active/disabled/expired"`
	LastUsedTime  time.Time `gorm:"column:last_used_time;type:datetime" orm:"column(last_used_time);null" description:"最近使用时间"`
	ExpireTime    time.Time `gorm:"column:expire_time;type:datetime" orm:"column(expire_time);null" description:"过期时间"`
	Remark        string    `gorm:"column:remark;type:varchar(255)" orm:"column(remark);size(255);null" description:"备注"`
}

func (a *CoinPlatformAccount) TableName() string {
	return "coin_platform_account"
}
