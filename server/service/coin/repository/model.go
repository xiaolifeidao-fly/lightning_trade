package repository

import (
	"common/middleware/db"
)

type Coin struct {
	db.BaseEntity
	Code             string  `gorm:"column:code;type:varchar(32);uniqueIndex:idx_code" orm:"column(code);size(32);null" description:"币种代码 BTC/ETH/USDT"`
	Name             string  `gorm:"column:name;type:varchar(64);index:idx_name" orm:"column(name);size(64);null" description:"币种名称"`
	FullName         string  `gorm:"column:full_name;type:varchar(128)" orm:"column(full_name);size(128);null" description:"币种全称"`
	Icon             string  `gorm:"column:icon;type:varchar(255)" orm:"column(icon);size(255);null" description:"图标URL"`
	ChainName        string  `gorm:"column:chain_name;type:varchar(32);index:idx_chain_name" orm:"column(chain_name);size(32);null" description:"主链名称 BTC/ETH/TRC20"`
	ContractAddress  string  `gorm:"column:contract_address;type:varchar(128)" orm:"column(contract_address);size(128);null" description:"合约地址"`
	Decimals         uint8   `gorm:"column:decimals;type:tinyint unsigned;default:8" orm:"column(decimals);null" description:"小数位精度"`
	PricePrecision   uint8   `gorm:"column:price_precision;type:tinyint unsigned;default:8" orm:"column(price_precision);null" description:"价格精度"`
	AmountPrecision  uint8   `gorm:"column:amount_precision;type:tinyint unsigned;default:8" orm:"column(amount_precision);null" description:"数量精度"`
	MinWithdrawal    float64 `gorm:"column:min_withdrawal;type:decimal(36,18);default:0" orm:"column(min_withdrawal);null" description:"最小提现额度"`
	MaxWithdrawal    float64 `gorm:"column:max_withdrawal;type:decimal(36,18);default:0" orm:"column(max_withdrawal);null" description:"最大提现额度"`
	WithdrawalFee    float64 `gorm:"column:withdrawal_fee;type:decimal(36,18);default:0" orm:"column(withdrawal_fee);null" description:"提现手续费"`
	DepositEnable    uint8   `gorm:"column:deposit_enable;type:tinyint unsigned;default:1" orm:"column(deposit_enable);null" description:"是否允许充值"`
	WithdrawEnable   uint8   `gorm:"column:withdraw_enable;type:tinyint unsigned;default:1" orm:"column(withdraw_enable);null" description:"是否允许提现"`
	TradeEnable      uint8   `gorm:"column:trade_enable;type:tinyint unsigned;default:1" orm:"column(trade_enable);null" description:"是否允许交易"`
	Status           string  `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"状态: online/offline/maintenance"`
	SortOrder        int     `gorm:"column:sort_order;type:int;default:0" orm:"column(sort_order);null" description:"排序"`
	Description      string  `gorm:"column:description;type:varchar(500)" orm:"column(description);size(500);null" description:"描述"`
}

func (c *Coin) TableName() string {
	return "coin"
}

type CoinPair struct {
	db.BaseEntity
	Symbol           string  `gorm:"column:symbol;type:varchar(32);uniqueIndex:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对符号 BTC-USDT"`
	BaseCoinID       uint64  `gorm:"column:base_coin_id;type:bigint unsigned;index:idx_base_coin_id" orm:"column(base_coin_id);null" description:"基础币种ID"`
	BaseCoinCode     string  `gorm:"column:base_coin_code;type:varchar(32);index:idx_base_coin_code" orm:"column(base_coin_code);size(32);null" description:"基础币种代码"`
	QuoteCoinID      uint64  `gorm:"column:quote_coin_id;type:bigint unsigned;index:idx_quote_coin_id" orm:"column(quote_coin_id);null" description:"计价币种ID"`
	QuoteCoinCode    string  `gorm:"column:quote_coin_code;type:varchar(32);index:idx_quote_coin_code" orm:"column(quote_coin_code);size(32);null" description:"计价币种代码"`
	PricePrecision   uint8   `gorm:"column:price_precision;type:tinyint unsigned;default:8" orm:"column(price_precision);null" description:"价格精度"`
	AmountPrecision  uint8   `gorm:"column:amount_precision;type:tinyint unsigned;default:8" orm:"column(amount_precision);null" description:"数量精度"`
	MinAmount        float64 `gorm:"column:min_amount;type:decimal(36,18);default:0" orm:"column(min_amount);null" description:"最小下单量"`
	MaxAmount        float64 `gorm:"column:max_amount;type:decimal(36,18);default:0" orm:"column(max_amount);null" description:"最大下单量"`
	MinTotal         float64 `gorm:"column:min_total;type:decimal(36,18);default:0" orm:"column(min_total);null" description:"最小成交额"`
	TakerFeeRate     float64 `gorm:"column:taker_fee_rate;type:decimal(10,6);default:0" orm:"column(taker_fee_rate);null" description:"吃单手续费率"`
	MakerFeeRate     float64 `gorm:"column:maker_fee_rate;type:decimal(10,6);default:0" orm:"column(maker_fee_rate);null" description:"挂单手续费率"`
	Status           string  `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"状态: online/offline/halt"`
	SortOrder        int     `gorm:"column:sort_order;type:int;default:0" orm:"column(sort_order);null" description:"排序"`
}

func (p *CoinPair) TableName() string {
	return "coin_pair"
}

type CoinPrice struct {
	db.BaseEntity
	CoinID     uint64  `gorm:"column:coin_id;type:bigint unsigned;index:idx_coin_id" orm:"column(coin_id);null" description:"币种ID"`
	CoinCode   string  `gorm:"column:coin_code;type:varchar(32);index:idx_coin_code" orm:"column(coin_code);size(32);null" description:"币种代码"`
	QuoteCode  string  `gorm:"column:quote_code;type:varchar(32);index:idx_quote_code" orm:"column(quote_code);size(32);null" description:"计价币种代码"`
	Price      float64 `gorm:"column:price;type:decimal(36,18);default:0" orm:"column(price);null" description:"最新价格"`
	Change24h  float64 `gorm:"column:change_24h;type:decimal(10,6);default:0" orm:"column(change_24h);null" description:"24小时涨跌幅"`
	Volume24h  float64 `gorm:"column:volume_24h;type:decimal(36,18);default:0" orm:"column(volume_24h);null" description:"24小时成交量"`
	High24h    float64 `gorm:"column:high_24h;type:decimal(36,18);default:0" orm:"column(high_24h);null" description:"24小时最高价"`
	Low24h     float64 `gorm:"column:low_24h;type:decimal(36,18);default:0" orm:"column(low_24h);null" description:"24小时最低价"`
	Source     string  `gorm:"column:source;type:varchar(32)" orm:"column(source);size(32);null" description:"价格来源 binance/okx"`
}

func (p *CoinPrice) TableName() string {
	return "coin_price"
}
