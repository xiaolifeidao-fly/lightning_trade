package repository

import (
	"common/middleware/db"
	"time"
)

type TradeOrder struct {
	db.BaseEntity
	OrderNo         string    `gorm:"column:order_no;type:varchar(64);uniqueIndex:idx_order_no" orm:"column(order_no);size(64);null" description:"订单号"`
	UserID          uint64    `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	Symbol          string    `gorm:"column:symbol;type:varchar(32);index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对 BTC-USDT"`
	BaseCoinCode    string    `gorm:"column:base_coin_code;type:varchar(32)" orm:"column(base_coin_code);size(32);null" description:"基础币种"`
	QuoteCoinCode   string    `gorm:"column:quote_coin_code;type:varchar(32)" orm:"column(quote_coin_code);size(32);null" description:"计价币种"`
	Side            string    `gorm:"column:side;type:varchar(8);index:idx_side" orm:"column(side);size(8);null" description:"方向 buy/sell"`
	OrderType       string    `gorm:"column:order_type;type:varchar(16);index:idx_order_type" orm:"column(order_type);size(16);null" description:"类型 limit/market/stop_limit"`
	Price           float64   `gorm:"column:price;type:decimal(36,18);default:0" orm:"column(price);null" description:"委托价格"`
	Amount          float64   `gorm:"column:amount;type:decimal(36,18);default:0" orm:"column(amount);null" description:"委托数量"`
	Total           float64   `gorm:"column:total;type:decimal(36,18);default:0" orm:"column(total);null" description:"委托总额"`
	StopPrice       float64   `gorm:"column:stop_price;type:decimal(36,18);default:0" orm:"column(stop_price);null" description:"触发价"`
	FilledAmount    float64   `gorm:"column:filled_amount;type:decimal(36,18);default:0" orm:"column(filled_amount);null" description:"已成交数量"`
	FilledTotal     float64   `gorm:"column:filled_total;type:decimal(36,18);default:0" orm:"column(filled_total);null" description:"已成交总额"`
	AvgFilledPrice  float64   `gorm:"column:avg_filled_price;type:decimal(36,18);default:0" orm:"column(avg_filled_price);null" description:"平均成交价"`
	FeeCoinCode     string    `gorm:"column:fee_coin_code;type:varchar(32)" orm:"column(fee_coin_code);size(32);null" description:"手续费币种"`
	FeeAmount       float64   `gorm:"column:fee_amount;type:decimal(36,18);default:0" orm:"column(fee_amount);null" description:"手续费"`
	Status          string    `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"状态 pending/partial/filled/canceled/rejected"`
	TimeInForce     string    `gorm:"column:time_in_force;type:varchar(8);default:GTC" orm:"column(time_in_force);size(8);null" description:"GTC/IOC/FOK"`
	Source          string    `gorm:"column:source;type:varchar(32)" orm:"column(source);size(32);null" description:"下单来源 web/app/api"`
	ClientOrderID   string    `gorm:"column:client_order_id;type:varchar(64);index:idx_client_order_id" orm:"column(client_order_id);size(64);null" description:"客户端自定义订单ID"`
	SubmittedTime   time.Time `gorm:"column:submitted_time;type:datetime" orm:"column(submitted_time);null" description:"提交时间"`
	FinishedTime    time.Time `gorm:"column:finished_time;type:datetime" orm:"column(finished_time);null" description:"完结时间"`
	CancelReason    string    `gorm:"column:cancel_reason;type:varchar(255)" orm:"column(cancel_reason);size(255);null" description:"取消原因"`
}

func (o *TradeOrder) TableName() string {
	return "trade_order"
}

type TradeMatch struct {
	db.BaseEntity
	TradeNo         string    `gorm:"column:trade_no;type:varchar(64);uniqueIndex:idx_trade_no" orm:"column(trade_no);size(64);null" description:"成交单号"`
	Symbol          string    `gorm:"column:symbol;type:varchar(32);index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对"`
	TakerOrderNo    string    `gorm:"column:taker_order_no;type:varchar(64);index:idx_taker_order_no" orm:"column(taker_order_no);size(64);null" description:"吃单订单号"`
	MakerOrderNo    string    `gorm:"column:maker_order_no;type:varchar(64);index:idx_maker_order_no" orm:"column(maker_order_no);size(64);null" description:"挂单订单号"`
	TakerUserID     uint64    `gorm:"column:taker_user_id;type:bigint unsigned;index:idx_taker_user_id" orm:"column(taker_user_id);null" description:"吃单用户ID"`
	MakerUserID     uint64    `gorm:"column:maker_user_id;type:bigint unsigned;index:idx_maker_user_id" orm:"column(maker_user_id);null" description:"挂单用户ID"`
	Side            string    `gorm:"column:side;type:varchar(8)" orm:"column(side);size(8);null" description:"吃单方向 buy/sell"`
	Price           float64   `gorm:"column:price;type:decimal(36,18);default:0" orm:"column(price);null" description:"成交价"`
	Amount          float64   `gorm:"column:amount;type:decimal(36,18);default:0" orm:"column(amount);null" description:"成交数量"`
	Total           float64   `gorm:"column:total;type:decimal(36,18);default:0" orm:"column(total);null" description:"成交金额"`
	TakerFee        float64   `gorm:"column:taker_fee;type:decimal(36,18);default:0" orm:"column(taker_fee);null" description:"吃单手续费"`
	MakerFee        float64   `gorm:"column:maker_fee;type:decimal(36,18);default:0" orm:"column(maker_fee);null" description:"挂单手续费"`
	MatchedTime     time.Time `gorm:"column:matched_time;type:datetime;index:idx_matched_time" orm:"column(matched_time);null" description:"撮合时间"`
}

func (m *TradeMatch) TableName() string {
	return "trade_match"
}

type TradeKline struct {
	db.BaseEntity
	Symbol     string    `gorm:"column:symbol;type:varchar(32);uniqueIndex:idx_symbol_interval_open,priority:1" orm:"column(symbol);size(32);null" description:"交易对"`
	Interval   string    `gorm:"column:interval;type:varchar(8);uniqueIndex:idx_symbol_interval_open,priority:2" orm:"column(interval);size(8);null" description:"周期 1m/5m/15m/1h/4h/1d"`
	OpenTime   time.Time `gorm:"column:open_time;type:datetime;uniqueIndex:idx_symbol_interval_open,priority:3" orm:"column(open_time);null" description:"开始时间"`
	CloseTime  time.Time `gorm:"column:close_time;type:datetime" orm:"column(close_time);null" description:"结束时间"`
	OpenPrice  float64   `gorm:"column:open_price;type:decimal(36,18);default:0" orm:"column(open_price);null" description:"开盘价"`
	HighPrice  float64   `gorm:"column:high_price;type:decimal(36,18);default:0" orm:"column(high_price);null" description:"最高价"`
	LowPrice   float64   `gorm:"column:low_price;type:decimal(36,18);default:0" orm:"column(low_price);null" description:"最低价"`
	ClosePrice float64   `gorm:"column:close_price;type:decimal(36,18);default:0" orm:"column(close_price);null" description:"收盘价"`
	Volume     float64   `gorm:"column:volume;type:decimal(36,18);default:0" orm:"column(volume);null" description:"成交量"`
	Turnover   float64   `gorm:"column:turnover;type:decimal(36,18);default:0" orm:"column(turnover);null" description:"成交额"`
	TradeCount uint64    `gorm:"column:trade_count;type:bigint unsigned;default:0" orm:"column(trade_count);null" description:"成交笔数"`
}

func (k *TradeKline) TableName() string {
	return "trade_kline"
}

type TradeOrderListRow struct {
	db.BaseEntity
	OrderNo        string    `gorm:"column:order_no"`
	UserID         uint64    `gorm:"column:user_id"`
	Symbol         string    `gorm:"column:symbol"`
	BaseCoinCode   string    `gorm:"column:base_coin_code"`
	QuoteCoinCode  string    `gorm:"column:quote_coin_code"`
	Side           string    `gorm:"column:side"`
	OrderType      string    `gorm:"column:order_type"`
	Price          float64   `gorm:"column:price"`
	Amount         float64   `gorm:"column:amount"`
	Total          float64   `gorm:"column:total"`
	FilledAmount   float64   `gorm:"column:filled_amount"`
	FilledTotal    float64   `gorm:"column:filled_total"`
	AvgFilledPrice float64   `gorm:"column:avg_filled_price"`
	FeeAmount      float64   `gorm:"column:fee_amount"`
	Status         string    `gorm:"column:status"`
	SubmittedTime  time.Time `gorm:"column:submitted_time"`
	FinishedTime   time.Time `gorm:"column:finished_time"`
}
