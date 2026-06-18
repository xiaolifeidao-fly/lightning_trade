package repository

import (
	"common/middleware/db"
	"time"
)

type TradeOrder struct {
	db.BaseEntity
	PlatformID     uint64    `gorm:"column:platform_id;type:bigint unsigned;default:0;index:idx_platform_id" orm:"column(platform_id);null" description:"所属平台ID"`
	PlatformCode   string    `gorm:"column:platform_code;type:varchar(32);index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"所属平台代码"`
	TradeCategory  string    `gorm:"column:trade_category;type:varchar(32);index:idx_trade_category" orm:"column(trade_category);size(32);null" description:"交易类别 spot/futures/margin"`
	TradeType      string    `gorm:"column:trade_type;type:varchar(16);index:idx_trade_type" orm:"column(trade_type);size(16);null" description:"交易类型 simulation/real"`
	OrderNo        string    `gorm:"column:order_no;type:varchar(64);uniqueIndex:idx_order_no" orm:"column(order_no);size(64);null" description:"订单号"`
	UserID         uint64    `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	Symbol         string    `gorm:"column:symbol;type:varchar(32);index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对 BTC-USDT"`
	BaseCoinCode   string    `gorm:"column:base_coin_code;type:varchar(32)" orm:"column(base_coin_code);size(32);null" description:"基础币种"`
	QuoteCoinCode  string    `gorm:"column:quote_coin_code;type:varchar(32)" orm:"column(quote_coin_code);size(32);null" description:"计价币种"`
	Side           string    `gorm:"column:side;type:varchar(8);index:idx_side" orm:"column(side);size(8);null" description:"方向 buy/sell"`
	OrderType      string    `gorm:"column:order_type;type:varchar(16);index:idx_order_type" orm:"column(order_type);size(16);null" description:"类型 limit/market/stop_limit"`
	Price          float64   `gorm:"column:price;type:decimal(36,18);default:0" orm:"column(price);null" description:"委托价格"`
	Amount         float64   `gorm:"column:amount;type:decimal(36,18);default:0" orm:"column(amount);null" description:"委托数量"`
	Total          float64   `gorm:"column:total;type:decimal(36,18);default:0" orm:"column(total);null" description:"委托总额"`
	StopPrice      float64   `gorm:"column:stop_price;type:decimal(36,18);default:0" orm:"column(stop_price);null" description:"触发价"`
	FilledAmount   float64   `gorm:"column:filled_amount;type:decimal(36,18);default:0" orm:"column(filled_amount);null" description:"已成交数量"`
	FilledTotal    float64   `gorm:"column:filled_total;type:decimal(36,18);default:0" orm:"column(filled_total);null" description:"已成交总额"`
	AvgFilledPrice float64   `gorm:"column:avg_filled_price;type:decimal(36,18);default:0" orm:"column(avg_filled_price);null" description:"平均成交价"`
	FeeCoinCode    string    `gorm:"column:fee_coin_code;type:varchar(32)" orm:"column(fee_coin_code);size(32);null" description:"手续费币种"`
	FeeAmount      float64   `gorm:"column:fee_amount;type:decimal(36,18);default:0" orm:"column(fee_amount);null" description:"手续费"`
	Status         string    `gorm:"column:status;type:varchar(32);index:idx_status" orm:"column(status);size(32);null" description:"状态 pending/partial/filled/canceled/rejected"`
	TimeInForce    string    `gorm:"column:time_in_force;type:varchar(8);default:GTC" orm:"column(time_in_force);size(8);null" description:"GTC/IOC/FOK"`
	Source         string    `gorm:"column:source;type:varchar(32)" orm:"column(source);size(32);null" description:"下单来源 web/app/api"`
	ClientOrderID  string    `gorm:"column:client_order_id;type:varchar(64);index:idx_client_order_id" orm:"column(client_order_id);size(64);null" description:"客户端自定义订单ID"`
	SubmittedTime  time.Time `gorm:"column:submitted_time;type:datetime" orm:"column(submitted_time);null" description:"提交时间"`
	FinishedTime   time.Time `gorm:"column:finished_time;type:datetime" orm:"column(finished_time);null" description:"完结时间"`
	CancelReason   string    `gorm:"column:cancel_reason;type:varchar(255)" orm:"column(cancel_reason);size(255);null" description:"取消原因"`
}

func (o *TradeOrder) TableName() string {
	return "trade_order"
}

type TradeMatch struct {
	db.BaseEntity
	PlatformID   uint64    `gorm:"column:platform_id;type:bigint unsigned;default:0;index:idx_platform_id" orm:"column(platform_id);null" description:"所属平台ID"`
	PlatformCode string    `gorm:"column:platform_code;type:varchar(32);index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"所属平台代码"`
	TradeNo      string    `gorm:"column:trade_no;type:varchar(64);uniqueIndex:idx_trade_no" orm:"column(trade_no);size(64);null" description:"成交单号"`
	Symbol       string    `gorm:"column:symbol;type:varchar(32);index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对"`
	TakerOrderNo string    `gorm:"column:taker_order_no;type:varchar(64);index:idx_taker_order_no" orm:"column(taker_order_no);size(64);null" description:"吃单订单号"`
	MakerOrderNo string    `gorm:"column:maker_order_no;type:varchar(64);index:idx_maker_order_no" orm:"column(maker_order_no);size(64);null" description:"挂单订单号"`
	TakerUserID  uint64    `gorm:"column:taker_user_id;type:bigint unsigned;index:idx_taker_user_id" orm:"column(taker_user_id);null" description:"吃单用户ID"`
	MakerUserID  uint64    `gorm:"column:maker_user_id;type:bigint unsigned;index:idx_maker_user_id" orm:"column(maker_user_id);null" description:"挂单用户ID"`
	Side         string    `gorm:"column:side;type:varchar(8)" orm:"column(side);size(8);null" description:"吃单方向 buy/sell"`
	Price        float64   `gorm:"column:price;type:decimal(36,18);default:0" orm:"column(price);null" description:"成交价"`
	Amount       float64   `gorm:"column:amount;type:decimal(36,18);default:0" orm:"column(amount);null" description:"成交数量"`
	Total        float64   `gorm:"column:total;type:decimal(36,18);default:0" orm:"column(total);null" description:"成交金额"`
	TakerFee     float64   `gorm:"column:taker_fee;type:decimal(36,18);default:0" orm:"column(taker_fee);null" description:"吃单手续费"`
	MakerFee     float64   `gorm:"column:maker_fee;type:decimal(36,18);default:0" orm:"column(maker_fee);null" description:"挂单手续费"`
	MatchedTime  time.Time `gorm:"column:matched_time;type:datetime;index:idx_matched_time" orm:"column(matched_time);null" description:"撮合时间"`
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

// TradeAIPrediction AI 模拟盘预测表，oracle 定时分析后落库，每个 币种×周期×K线时间 一条
type TradeAIPrediction struct {
	db.BaseEntity
	PlatformCode string    `gorm:"column:platform_code;type:varchar(32);uniqueIndex:idx_ai_pred_dim,priority:1;index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"平台代码"`
	Symbol       string    `gorm:"column:symbol;type:varchar(32);uniqueIndex:idx_ai_pred_dim,priority:2;index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对 BTCUSDT"`
	CoinCode     string    `gorm:"column:coin_code;type:varchar(32);index:idx_coin_code" orm:"column(coin_code);size(32);null" description:"基础币种 BTC"`
	Interval     string    `gorm:"column:interval;type:varchar(8);uniqueIndex:idx_ai_pred_dim,priority:3" orm:"column(interval);size(8);null" description:"主周期 1m/5m/15m/1h/4h/1d"`
	PredictTime  time.Time `gorm:"column:predict_time;type:datetime;uniqueIndex:idx_ai_pred_dim,priority:4;index:idx_predict_time" orm:"column(predict_time);null" description:"预测对应的K线时间"`
	RefPrice     float64   `gorm:"column:ref_price;type:decimal(36,18);default:0" orm:"column(ref_price);null" description:"预测时真实参考价(收盘价)"`
	PredictPrice float64   `gorm:"column:predict_price;type:decimal(36,18);default:0" orm:"column(predict_price);null" description:"AI预测价"`
	PredictHigh  float64   `gorm:"column:predict_high;type:decimal(36,18);default:0" orm:"column(predict_high);null" description:"预测期间最高价"`
	PredictLow   float64   `gorm:"column:predict_low;type:decimal(36,18);default:0" orm:"column(predict_low);null" description:"预测期间最低价"`
	Invalidation float64   `gorm:"column:invalidation;type:decimal(36,18);default:0" orm:"column(invalidation);null" description:"失效价位：方向被证伪的关键价位(0=未给)"`
	Trend        string    `gorm:"column:trend;type:varchar(16);index:idx_trend" orm:"column(trend);size(16);null" description:"趋势 long/short/neutral"`
	Signal       string    `gorm:"column:signal;type:varchar(32)" orm:"column(signal);size(32);null" description:"信号 buy/sell/hold 等"`
	Confidence   float64   `gorm:"column:confidence;type:decimal(10,4);default:0" orm:"column(confidence);null" description:"置信度 0~1"`
	StopLoss     float64   `gorm:"column:stop_loss;type:decimal(36,18);default:0" orm:"column(stop_loss);null" description:"建议止损价"`
	TakeProfit   float64   `gorm:"column:take_profit;type:decimal(36,18);default:0" orm:"column(take_profit);null" description:"建议止盈价"`
	Reason       string    `gorm:"column:reason;type:text" orm:"column(reason);null" description:"AI文字理由"`
	RawResponse  string    `gorm:"column:raw_response;type:text" orm:"column(raw_response);null" description:"LLM原始返回"`
	Model        string    `gorm:"column:model;type:varchar(64)" orm:"column(model);size(64);null" description:"使用的模型"`
	Provider     string    `gorm:"column:provider;type:varchar(32)" orm:"column(provider);size(32);null" description:"AI服务商"`
	// 以下为到期回填结算字段：predict_time 到期后由 oracle 取真实价回填，用于命中率/误差统计。
	ActualPrice  float64    `gorm:"column:actual_price;type:decimal(36,18);default:0" orm:"column(actual_price);null" description:"到期真实价(predict_time 时刻1m收盘价)"`
	ErrorPct     float64    `gorm:"column:error_pct;type:decimal(20,8);default:0" orm:"column(error_pct);null" description:"有符号误差% (predict-actual)/actual*100"`
	AbsErrorPct  float64    `gorm:"column:abs_error_pct;type:decimal(20,8);default:0" orm:"column(abs_error_pct);null" description:"绝对误差%"`
	DirectionHit int8       `gorm:"column:direction_hit;type:tinyint;default:0" orm:"column(direction_hit);null" description:"方向是否命中 1命中 0未命中"`
	// 以下为区间触达结算字段：遍历 [created_time, predict_time] 区间内的 1m K线，衡量信号可交易性。
	MaxFavorablePct float64 `gorm:"column:max_favorable_pct;type:decimal(20,8);default:0" orm:"column(max_favorable_pct);null" description:"区间内沿预测方向最大有利偏移%(MFE，相对ref_price)"`
	MaxAdversePct   float64 `gorm:"column:max_adverse_pct;type:decimal(20,8);default:0" orm:"column(max_adverse_pct);null" description:"区间内逆预测方向最大不利偏移%(MAE，正数表回撤幅度)"`
	FirstHit        string  `gorm:"column:first_hit;type:varchar(8);default:''" orm:"column(first_hit);size(8);null" description:"区间内先触达：tp先触止盈/sl先触止损/none都未触"`
	// 以下为预测波动区间(predict_high/predict_low)的结算字段：与区间内真实最高/最低价比对，衡量区间预测质量。
	ActualHigh   float64 `gorm:"column:actual_high;type:decimal(36,18);default:0" orm:"column(actual_high);null" description:"区间内真实最高价"`
	ActualLow    float64 `gorm:"column:actual_low;type:decimal(36,18);default:0" orm:"column(actual_low);null" description:"区间内真实最低价"`
	HighErrorPct float64 `gorm:"column:high_error_pct;type:decimal(20,8);default:0" orm:"column(high_error_pct);null" description:"预测最高价有符号误差% (predict_high-actual_high)/actual_high*100"`
	LowErrorPct  float64 `gorm:"column:low_error_pct;type:decimal(20,8);default:0" orm:"column(low_error_pct);null" description:"预测最低价有符号误差% (predict_low-actual_low)/actual_low*100"`
	BandContain  int8    `gorm:"column:band_contain;type:tinyint;default:0" orm:"column(band_contain);null" description:"预测区间是否完整覆盖真实波动 1是 0否"`
	// 失效位结算：窗口内真实价是否触及失效价位(invalidation)。long 看最低价跌破、short 看最高价突破。-1=未给失效位 0=未触发(方向未被证伪) 1=已触发(方向被证伪)
	InvalidationHit int8 `gorm:"column:invalidation_hit;type:tinyint;default:0" orm:"column(invalidation_hit);null" description:"失效位是否触达 -1未给 0未触发 1已触发"`
	Settled         int8    `gorm:"column:settled;type:tinyint;default:0;index:idx_settled" orm:"column(settled);null" description:"是否已结算回填 1是 0否"`
	SettledTime  *time.Time `gorm:"column:settled_time;type:datetime;null" orm:"column(settled_time);null" description:"结算回填时间"`
}

func (p *TradeAIPrediction) TableName() string {
	return "trade_ai_prediction"
}

type TradeOrderListRow struct {
	db.BaseEntity
	PlatformID     uint64    `gorm:"column:platform_id"`
	PlatformCode   string    `gorm:"column:platform_code"`
	TradeCategory  string    `gorm:"column:trade_category"`
	TradeType      string    `gorm:"column:trade_type"`
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

// TradeDetail 交易明细盈亏表，每笔成交对应一条记录
type TradeDetail struct {
	db.BaseEntity
	PlatformID       uint64    `gorm:"column:platform_id;type:bigint unsigned;default:0;index:idx_platform_id" orm:"column(platform_id);null" description:"平台ID"`
	PlatformCode     string    `gorm:"column:platform_code;type:varchar(32);index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"平台代码"`
	TradeCategory    string    `gorm:"column:trade_category;type:varchar(32);index:idx_trade_category" orm:"column(trade_category);size(32);null" description:"交易类别 spot/futures/margin"`
	TradeType        string    `gorm:"column:trade_type;type:varchar(16);index:idx_trade_type" orm:"column(trade_type);size(16);null" description:"交易类型 simulation/real"`
	UserID           uint64    `gorm:"column:user_id;type:bigint unsigned;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	OrderNo          string    `gorm:"column:order_no;type:varchar(64);index:idx_order_no" orm:"column(order_no);size(64);null" description:"关联订单号"`
	TradeNo          string    `gorm:"column:trade_no;type:varchar(64);uniqueIndex:idx_trade_no" orm:"column(trade_no);size(64);null" description:"成交单号"`
	Symbol           string    `gorm:"column:symbol;type:varchar(32);index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对"`
	CoinCode         string    `gorm:"column:coin_code;type:varchar(32);index:idx_coin_code" orm:"column(coin_code);size(32);null" description:"基础币种"`
	Side             string    `gorm:"column:side;type:varchar(8)" orm:"column(side);size(8);null" description:"成交方向 buy/sell"`
	OpenDirection    string    `gorm:"column:open_direction;type:varchar(8);index:idx_open_direction" orm:"column(open_direction);size(8);null" description:"开仓方向 long/short"`
	AvgOpenPrice     float64   `gorm:"column:avg_open_price;type:decimal(36,18);default:0" orm:"column(avg_open_price);null" description:"开仓平均价格"`
	LiquidationPrice float64   `gorm:"column:liquidation_price;type:decimal(36,18);default:0" orm:"column(liquidation_price);null" description:"爆仓价格"`
	Leverage         float64   `gorm:"column:leverage;type:decimal(10,2);default:1" orm:"column(leverage);null" description:"开仓倍数(杠杆)"`
	Margin           float64   `gorm:"column:margin;type:decimal(36,18);default:0" orm:"column(margin);null" description:"保证金"`
	UserBalanceOpen  float64   `gorm:"column:user_balance_open;type:decimal(36,18);default:0" orm:"column(user_balance_open);null" description:"开仓时用户余额"`
	Price            float64   `gorm:"column:price;type:decimal(36,18);default:0" orm:"column(price);null" description:"成交价"`
	Amount           float64   `gorm:"column:amount;type:decimal(36,18);default:0" orm:"column(amount);null" description:"成交数量"`
	Total            float64   `gorm:"column:total;type:decimal(36,18);default:0" orm:"column(total);null" description:"成交金额"`
	Fee              float64   `gorm:"column:fee;type:decimal(36,18);default:0" orm:"column(fee);null" description:"手续费"`
	Pnl              float64   `gorm:"column:pnl;type:decimal(36,18);default:0" orm:"column(pnl);null" description:"盈亏金额"`
	PnlRate          float64   `gorm:"column:pnl_rate;type:decimal(18,8);default:0" orm:"column(pnl_rate);null" description:"盈亏比率"`
	TradeTime        time.Time `gorm:"column:trade_time;type:datetime;index:idx_trade_time" orm:"column(trade_time);null" description:"成交时间"`
}

func (d *TradeDetail) TableName() string {
	return "trade_detail"
}

// TradeUserSummary 用户交易汇总表（按天聚合）
type TradeUserSummary struct {
	db.BaseEntity
	UserID        uint64  `gorm:"column:user_id;type:bigint unsigned;uniqueIndex:idx_summary_dim,priority:1;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	PlatformID    uint64  `gorm:"column:platform_id;type:bigint unsigned;uniqueIndex:idx_summary_dim,priority:2" orm:"column(platform_id);null" description:"平台ID"`
	PlatformCode  string  `gorm:"column:platform_code;type:varchar(32)" orm:"column(platform_code);size(32);null" description:"平台代码"`
	CoinCode      string  `gorm:"column:coin_code;type:varchar(32);uniqueIndex:idx_summary_dim,priority:3" orm:"column(coin_code);size(32);null" description:"币种代码"`
	TradeCategory string  `gorm:"column:trade_category;type:varchar(32);uniqueIndex:idx_summary_dim,priority:4" orm:"column(trade_category);size(32);null" description:"交易类别"`
	TradeDate     string  `gorm:"column:trade_date;type:varchar(10);uniqueIndex:idx_summary_dim,priority:5;index:idx_trade_date" orm:"column(trade_date);size(10);null" description:"交易日期 yyyy-MM-dd"`
	TotalOrders   int64   `gorm:"column:total_orders;type:bigint;default:0" orm:"column(total_orders);null" description:"总订单数"`
	BuyOrders     int64   `gorm:"column:buy_orders;type:bigint;default:0" orm:"column(buy_orders);null" description:"买入订单数"`
	SellOrders    int64   `gorm:"column:sell_orders;type:bigint;default:0" orm:"column(sell_orders);null" description:"卖出订单数"`
	BuyAmount     float64 `gorm:"column:buy_amount;type:decimal(36,18);default:0" orm:"column(buy_amount);null" description:"买入数量"`
	SellAmount    float64 `gorm:"column:sell_amount;type:decimal(36,18);default:0" orm:"column(sell_amount);null" description:"卖出数量"`
	BuyTotal      float64 `gorm:"column:buy_total;type:decimal(36,18);default:0" orm:"column(buy_total);null" description:"买入金额"`
	SellTotal     float64 `gorm:"column:sell_total;type:decimal(36,18);default:0" orm:"column(sell_total);null" description:"卖出金额"`
	TotalFee      float64 `gorm:"column:total_fee;type:decimal(36,18);default:0" orm:"column(total_fee);null" description:"总手续费"`
	TotalVolume   float64 `gorm:"column:total_volume;type:decimal(36,18);default:0" orm:"column(total_volume);null" description:"总成交额"`
}

func (s *TradeUserSummary) TableName() string {
	return "trade_user_summary"
}

// TradeUserPnl 用户交易盈亏表（按天聚合）
type TradeUserPnl struct {
	db.BaseEntity
	UserID         uint64  `gorm:"column:user_id;type:bigint unsigned;uniqueIndex:idx_pnl_dim,priority:1;index:idx_user_id" orm:"column(user_id);null" description:"用户ID"`
	PlatformID     uint64  `gorm:"column:platform_id;type:bigint unsigned;uniqueIndex:idx_pnl_dim,priority:2" orm:"column(platform_id);null" description:"平台ID"`
	PlatformCode   string  `gorm:"column:platform_code;type:varchar(32)" orm:"column(platform_code);size(32);null" description:"平台代码"`
	CoinCode       string  `gorm:"column:coin_code;type:varchar(32);uniqueIndex:idx_pnl_dim,priority:3" orm:"column(coin_code);size(32);null" description:"币种代码"`
	TradeCategory  string  `gorm:"column:trade_category;type:varchar(32);uniqueIndex:idx_pnl_dim,priority:4" orm:"column(trade_category);size(32);null" description:"交易类别"`
	TradeDate      string  `gorm:"column:trade_date;type:varchar(10);uniqueIndex:idx_pnl_dim,priority:5;index:idx_trade_date" orm:"column(trade_date);size(10);null" description:"交易日期 yyyy-MM-dd"`
	RealizedPnl    float64 `gorm:"column:realized_pnl;type:decimal(36,18);default:0" orm:"column(realized_pnl);null" description:"已实现盈亏"`
	UnrealizedPnl  float64 `gorm:"column:unrealized_pnl;type:decimal(36,18);default:0" orm:"column(unrealized_pnl);null" description:"未实现盈亏"`
	TotalPnl       float64 `gorm:"column:total_pnl;type:decimal(36,18);default:0" orm:"column(total_pnl);null" description:"总盈亏"`
	PnlRate        float64 `gorm:"column:pnl_rate;type:decimal(18,8);default:0" orm:"column(pnl_rate);null" description:"盈亏比率"`
	PositionAmount float64 `gorm:"column:position_amount;type:decimal(36,18);default:0" orm:"column(position_amount);null" description:"持仓数量"`
	PositionCost   float64 `gorm:"column:position_cost;type:decimal(36,18);default:0" orm:"column(position_cost);null" description:"持仓成本"`
	PositionValue  float64 `gorm:"column:position_value;type:decimal(36,18);default:0" orm:"column(position_value);null" description:"持仓市值"`
}

func (p *TradeUserPnl) TableName() string {
	return "trade_user_pnl"
}
