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
	RefPrice     float64   `gorm:"column:ref_price;type:decimal(36,18);default:0" orm:"column(ref_price);null" description:"AI参考开盘价：发起预测时 AI 参考的收盘价(预测基准)"`
	OpenPrice    float64   `gorm:"column:open_price;type:decimal(36,18);default:0" orm:"column(open_price);null" description:"实际开盘价：AI分析完成后即时采集的真实盘价"`
	CostMs       int64     `gorm:"column:cost_ms;type:bigint;default:0" orm:"column(cost_ms);null" description:"AI分析耗时(毫秒)：从发起到检测完成"`
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
	ActualPrice  float64 `gorm:"column:actual_price;type:decimal(36,18);default:0" orm:"column(actual_price);null" description:"到期真实价(predict_time 时刻1m收盘价)"`
	ErrorPct     float64 `gorm:"column:error_pct;type:decimal(20,8);default:0" orm:"column(error_pct);null" description:"有符号误差% (predict-actual)/actual*100"`
	AbsErrorPct  float64 `gorm:"column:abs_error_pct;type:decimal(20,8);default:0" orm:"column(abs_error_pct);null" description:"绝对误差%"`
	DirectionHit int8    `gorm:"column:direction_hit;type:tinyint;default:0" orm:"column(direction_hit);null" description:"方向是否命中 1命中 0未命中"`
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
	InvalidationHit int8       `gorm:"column:invalidation_hit;type:tinyint;default:0" orm:"column(invalidation_hit);null" description:"失效位是否触达 -1未给 0未触发 1已触发"`
	Settled         int8       `gorm:"column:settled;type:tinyint;default:0;index:idx_settled" orm:"column(settled);null" description:"是否已结算回填 1是 0否"`
	SettledTime     *time.Time `gorm:"column:settled_time;type:datetime;null" orm:"column(settled_time);null" description:"结算回填时间"`
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

// TradeStrategy 策略配置表：定义 AI 信号触发开仓的条件与持仓参数。
// 一个 symbol×interval 可配置多条策略，每条独立开仓，受 max_open_positions 约束。
type TradeStrategy struct {
	db.BaseEntity
	PlatformCode string `gorm:"column:platform_code;type:varchar(32);not null;index:idx_strategy_dim,priority:1" description:"平台代码"`
	CoinCode     string `gorm:"column:coin_code;type:varchar(32);not null;index:idx_strategy_dim,priority:2" description:"基础币种 BTC"`
	Symbol       string `gorm:"column:symbol;type:varchar(32);not null;index:idx_strategy_dim,priority:3" description:"交易对 BTCUSDT"`
	Interval     string `gorm:"column:interval;type:varchar(8);not null;index:idx_strategy_dim,priority:4" description:"触发预测周期 1m/15m/1h/4h"`
	Enabled      int8   `gorm:"column:enabled;type:tinyint;default:1;index:idx_enabled" description:"是否启用 1是 0否"`
	// 开仓条件
	MinConfidence    float64 `gorm:"column:min_confidence;type:decimal(6,4);default:0.6000" description:"最低置信度 0~1"`
	MinMovePct       float64 `gorm:"column:min_move_pct;type:decimal(10,4);default:0.5000" description:"最低预测幅度百分比，如 0.5 表示 0.5%"`
	TrendFilter      string  `gorm:"column:trend_filter;type:varchar(8);default:'both'" description:"方向过滤 long/short/both"`
	MaxOpenPositions int     `gorm:"column:max_open_positions;type:int;default:1" description:"同一策略最多同时持仓数"`
	// 持仓参数
	HoldDuration    int `gorm:"column:hold_duration;type:int;default:14400" description:"持仓时长(秒)，即交易周期，默认 4h=14400"`
	MaxHoldDuration int `gorm:"column:max_hold_duration;type:int;default:86400" description:"最长持仓硬上限(秒)，防极端行情挂单，默认 24h=86400"`
	// 止盈止损：0 = 使用 AI 给的建议价，非 0 = 使用此百分比覆盖
	TakeProfitPct float64 `gorm:"column:take_profit_pct;type:decimal(10,4);default:0.0000" description:"止盈幅度百分比，0=跟 AI 建议"`
	StopLossPct   float64 `gorm:"column:stop_loss_pct;type:decimal(10,4);default:0.0000" description:"止损幅度百分比，0=跟 AI 建议"`
	// 止盈止损来源(三选一)：percent(离入场价%)/predict(跟AI预测)/pressure(跟AI压力面)
	TakeProfitSource   string  `gorm:"column:take_profit_source;type:varchar(16);default:'predict'" description:"止盈来源 percent/predict/pressure"`
	StopLossSource     string  `gorm:"column:stop_loss_source;type:varchar(16);default:'predict'" description:"止损来源 percent/predict/pressure"`
	PredictSLBufferPct float64 `gorm:"column:predict_sl_buffer_pct;type:decimal(10,4);default:0.0000" description:"predict止损：突破失效价该%后止损"`
	PressureBufferPct  float64 `gorm:"column:pressure_buffer_pct;type:decimal(10,4);default:0.0000" description:"pressure止盈/止损：离关键结构位缓冲%"`
	TakeProfitFloorPct float64 `gorm:"column:take_profit_floor_pct;type:decimal(10,4);default:0.0000" description:"兜底锁盈%：止盈目标比该值更远时提前到该%锁盈，0=不约束"`
	StopLossFloorPct   float64 `gorm:"column:stop_loss_floor_pct;type:decimal(10,4);default:0.0000" description:"兜底最小止损%：止损隐含亏损<该值则放宽到该值，0=不约束"`
	// 仓位参数
	Leverage     float64 `gorm:"column:leverage;type:decimal(6,2);default:10.00" description:"杠杆倍数"`
	Contracts    int     `gorm:"column:contracts;type:int;default:1" description:"开仓张数，1张=0.001BTC"`
	MakerFeeRate float64 `gorm:"column:maker_fee_rate;type:decimal(10,6);default:0.000200" description:"Maker 手续费率"`
	TakerFeeRate float64 `gorm:"column:taker_fee_rate;type:decimal(10,6);default:0.000500" description:"Taker 手续费率"`
	// 入场策略(状态机)：决定怎么入场，而非只在开盘价市价开仓。与 strategy.Params 一一对应。
	EntryMode         string  `gorm:"column:entry_mode;type:varchar(16);default:'market'" description:"入场方式 market(市价)/pullback(区间回踩限价)"`
	EntryAlpha        float64 `gorm:"column:entry_alpha;type:decimal(6,4);default:0.1500" description:"入场分位α：限价离区间下沿(多)/上沿(空)的比例"`
	ExitGamma         float64 `gorm:"column:exit_gamma;type:decimal(6,4);default:0.1000" description:"止盈分位γ：止盈价离区间对沿的比例"`
	EntryTTL          int     `gorm:"column:entry_ttl;type:int;default:1800" description:"挂单有效期(秒)，未成交则放弃，默认30分钟"`
	EfficiencyRoute   float64 `gorm:"column:efficiency_route;type:decimal(6,4);default:0.0000" description:"趋势效率阈值：低于→pullback、高于→market；0=不路由固定用 entry_mode"`
	PredictionVariant string  `gorm:"column:prediction_variant;type:varchar(16);default:'raw'" description:"预测变体 raw(原始)/calibrated(校准后)，用于对比校准价值"`
	Remark            string  `gorm:"column:remark;type:varchar(255)" description:"备注"`
}

func (s *TradeStrategy) TableName() string { return "trade_strategy" }

// TradeStrategyPosition 持仓生命周期表：记录从挂单/开仓到平仓的完整过程。
// 状态机：pending → open → closed（close_reason: tp/sl/timeout/manual），或 pending → expired（挂单超时未成交）。
// 与 strategy.Order 对应：market 模式直接落 open，pullback 模式先落 pending 待成交。
type TradeStrategyPosition struct {
	db.BaseEntity
	StrategyID   int64  `gorm:"column:strategy_id;type:bigint;not null;index:idx_strategy_id" description:"关联策略ID"`
	PredictionID int64  `gorm:"column:prediction_id;type:bigint;not null;index:idx_prediction_id" description:"触发信号的 AI 预测ID"`
	PlatformCode string `gorm:"column:platform_code;type:varchar(32);not null" description:"平台代码"`
	CoinCode     string `gorm:"column:coin_code;type:varchar(32);not null" description:"基础币种"`
	Symbol       string `gorm:"column:symbol;type:varchar(32);not null;index:idx_pos_symbol" description:"交易对 BTCUSDT"`
	Interval     string `gorm:"column:interval;type:varchar(8);not null" description:"触发预测周期"`
	Direction    string `gorm:"column:direction;type:varchar(8);not null" description:"开仓方向 long/short"`
	// 入场意图（pullback 限价单用；market 模式下 planned=open）
	EntryMode         string     `gorm:"column:entry_mode;type:varchar(16);default:'market'" description:"入场方式 market/pullback"`
	PlannedEntryPrice float64    `gorm:"column:planned_entry_price;type:decimal(36,18);default:0" description:"计划入场价（pullback 限价）"`
	RequestedAt       *time.Time `gorm:"column:requested_at;type:datetime" description:"信号/挂单时刻（区别于成交时刻 opened_at）"`
	EntryDeadline     *time.Time `gorm:"column:entry_deadline;type:datetime;index:idx_entry_deadline_status" description:"挂单失效时间 = requested_at + entry_ttl"`
	// 开仓信息（成交后回填；pending 时为 0/空）
	OpenPrice       float64   `gorm:"column:open_price;type:decimal(36,18);default:0" description:"成交价（market=即时行情价 / pullback=限价成交价）"`
	TakeProfitPrice float64   `gorm:"column:take_profit_price;type:decimal(36,18);default:0" description:"止盈价（绝对价位）"`
	StopLossPrice   float64   `gorm:"column:stop_loss_price;type:decimal(36,18);default:0" description:"止损价（绝对价位）"`
	Contracts       int       `gorm:"column:contracts;type:int;default:1" description:"张数"`
	Leverage        float64   `gorm:"column:leverage;type:decimal(6,2);default:1.00" description:"杠杆"`
	OpenedAt        time.Time `gorm:"column:opened_at;type:datetime;not null;index:idx_opened_at" description:"开仓时间(UTC)"`
	HoldUntil       time.Time `gorm:"column:hold_until;type:datetime;not null;index:idx_hold_until_status" description:"持仓截止时间(UTC)=opened_at+hold_duration"`
	// 状态机
	Status      string     `gorm:"column:status;type:varchar(16);not null;default:'open';index:idx_hold_until_status" description:"状态 pending/open/closed/expired"`
	ClosePrice  float64    `gorm:"column:close_price;type:decimal(36,18);default:0" description:"平仓价"`
	CloseReason string     `gorm:"column:close_reason;type:varchar(16)" description:"收尾原因 tp/sl/timeout/manual/expired"`
	ClosedAt    *time.Time `gorm:"column:closed_at;type:datetime" description:"平仓时间(UTC)"`
	// 结算字段（平仓后回填）
	Pnl     float64 `gorm:"column:pnl;type:decimal(36,18);default:0" description:"盈亏金额 USDT（名义）"`
	PnlRate float64 `gorm:"column:pnl_rate;type:decimal(18,8);default:0" description:"盈亏率%（含杠杆）"`
	Fee     float64 `gorm:"column:fee;type:decimal(36,18);default:0" description:"往返手续费 USDT"`
	NetPnl  float64 `gorm:"column:net_pnl;type:decimal(36,18);default:0" description:"净盈亏 = pnl - fee"`
	// 辅助字段（冗余，方便查询/分析）
	Confidence       float64 `gorm:"column:confidence;type:decimal(6,4);default:0" description:"开仓时 AI 置信度"`
	PredictedMovePct float64 `gorm:"column:predicted_move_pct;type:decimal(10,4);default:0" description:"触发开仓的 AI 预测幅度%"`
	// 实时追踪（监测循环持续更新）
	MaxPriceDuringHold float64 `gorm:"column:max_price_during_hold;type:decimal(36,18);default:0" description:"持仓期间最高价"`
	MinPriceDuringHold float64 `gorm:"column:min_price_during_hold;type:decimal(36,18);default:0" description:"持仓期间最低价"`
}

func (p *TradeStrategyPosition) TableName() string { return "trade_strategy_position" }

// TradeBacktestRun 回测任务表：一次回测 = 一行，记录「用了什么数据、什么策略、什么参数」。
// 是回测层的顶层，下挂 trade_backtest_trade(逐笔)与 trade_backtest_metric(汇总)。
type TradeBacktestRun struct {
	db.BaseEntity
	Name         string `gorm:"column:name;type:varchar(128)" description:"任务名(便于对比)"`
	PlatformCode string `gorm:"column:platform_code;type:varchar(32);index:idx_bt_run_dim,priority:1" description:"平台代码"`
	CoinCode     string `gorm:"column:coin_code;type:varchar(32)" description:"基础币种"`
	Symbol       string `gorm:"column:symbol;type:varchar(32);index:idx_bt_run_dim,priority:2" description:"交易对 BTCUSDT"`
	// 输入选择（前端的选择项落地）
	PredictionInterval string    `gorm:"column:prediction_interval;type:varchar(8)" description:"用哪个预测周期的区间 15m/1h"`
	PredictionVariant  string    `gorm:"column:prediction_variant;type:varchar(16);default:'raw'" description:"预测变体 raw/calibrated"`
	PriceInterval      string    `gorm:"column:price_interval;type:varchar(8);default:'1m'" description:"实际价回放周期 1m/5m"`
	PriceSource        string    `gorm:"column:price_source;type:varchar(32)" description:"价格来源(交易所/数据集)"`
	TradingPeriod      string    `gorm:"column:trading_period;type:varchar(8)" description:"可选交易周期 1h/4h/8h/12h/1d；空=仅按预测周期(现状)"`
	StartTime          time.Time `gorm:"column:start_time;type:datetime;index:idx_bt_run_time" description:"回测起始时间(UTC)"`
	EndTime            time.Time `gorm:"column:end_time;type:datetime" description:"回测结束时间(UTC)"`
	// 策略与冻结参数
	StrategyID     int64  `gorm:"column:strategy_id;type:bigint;index:idx_bt_run_strategy" description:"关联策略ID"`
	ParamsSnapshot string `gorm:"column:params_snapshot;type:text" description:"策略参数冻结快照(JSON)，保证结果可复现"`
	// 执行状态
	Status   string `gorm:"column:status;type:varchar(16);default:'pending'" description:"状态 pending/running/done/failed"`
	ErrorMsg string `gorm:"column:error_msg;type:varchar(512)" description:"失败原因"`
	// 回放实际使用的 K 线覆盖情况（回测结束后回填，供详情展示“数据够不够”）。
	// Start/End 用指针：创建任务时尚未回填，存 NULL，避免零值写成 '0000-00-00' 被严格模式拒绝。
	KlineCount int        `gorm:"column:kline_count;type:int;default:0" description:"回放使用的K线根数"`
	KlineStart *time.Time `gorm:"column:kline_start;type:datetime" description:"实际K线起始时间"`
	KlineEnd   *time.Time `gorm:"column:kline_end;type:datetime" description:"实际K线结束时间"`
}

func (r *TradeBacktestRun) TableName() string { return "trade_backtest_run" }

// TradeBacktestTrade 回测逐笔明细：一次回测里的每一笔模拟交易，结构镜像持仓但归属某个 run、不进实盘监控。
// 与 strategy.Order 对应；status=expired 表示挂单未成交(成交率统计必需)。
type TradeBacktestTrade struct {
	db.BaseEntity
	RunID        int64  `gorm:"column:run_id;type:bigint;not null;index:idx_bt_trade_run" description:"关联回测任务ID"`
	PredictionID int64  `gorm:"column:prediction_id;type:bigint;index:idx_bt_trade_pred" description:"对应历史预测ID"`
	CalcMode     string `gorm:"column:calc_mode;type:varchar(16);default:'prediction'" description:"结算口径 prediction(预测周期)/trading(交易周期)"`
	// 预测周期：该笔关联预测的预测目标时刻(predict_time)，与 requested_at 一起框定预测覆盖的时间窗。
	PredictTime *time.Time `gorm:"column:predict_time;type:datetime" description:"预测目标时刻(关联预测的 predict_time)"`
	Direction   string     `gorm:"column:direction;type:varchar(8)" description:"方向 long/short"`
	// 入场意图
	EntryMode         string  `gorm:"column:entry_mode;type:varchar(16)" description:"入场方式 market/pullback"`
	PlannedEntryPrice float64 `gorm:"column:planned_entry_price;type:decimal(36,18);default:0" description:"计划入场价"`
	TakeProfitPrice   float64 `gorm:"column:take_profit_price;type:decimal(36,18);default:0" description:"止盈价"`
	StopLossPrice     float64 `gorm:"column:stop_loss_price;type:decimal(36,18);default:0" description:"止损价"`
	// 生命周期
	Status      string     `gorm:"column:status;type:varchar(16)" description:"状态 open/closed/expired"`
	OpenPrice   float64    `gorm:"column:open_price;type:decimal(36,18);default:0" description:"成交价"`
	ClosePrice  float64    `gorm:"column:close_price;type:decimal(36,18);default:0" description:"平仓价"`
	CloseReason string     `gorm:"column:close_reason;type:varchar(16)" description:"收尾原因 tp/sl/timeout/expired"`
	RequestedAt time.Time  `gorm:"column:requested_at;type:datetime" description:"挂单时刻(仿真)"`
	OpenedAt    *time.Time `gorm:"column:opened_at;type:datetime" description:"成交时刻(仿真)"`
	ClosedAt    *time.Time `gorm:"column:closed_at;type:datetime" description:"收尾时刻(仿真)"`
	// 结算
	Pnl      float64 `gorm:"column:pnl;type:decimal(36,18);default:0" description:"盈亏金额 USDT"`
	PnlRate  float64 `gorm:"column:pnl_rate;type:decimal(18,8);default:0" description:"盈亏率%(含杠杆)"`
	Fee      float64 `gorm:"column:fee;type:decimal(36,18);default:0" description:"往返手续费 USDT"`
	NetPnl   float64 `gorm:"column:net_pnl;type:decimal(36,18);default:0" description:"净盈亏 = pnl - fee"`
	Leverage float64 `gorm:"column:leverage;type:decimal(6,2);default:1" description:"杠杆倍数(展示含杠杆浮盈用)"`
	// 预测特征冗余（便于事后切片分析：按置信度/效率分组看哪类预测赚钱）
	Confidence       float64 `gorm:"column:confidence;type:decimal(6,4);default:0" description:"预测置信度"`
	PredictedMovePct float64 `gorm:"column:predicted_move_pct;type:decimal(10,4);default:0" description:"预测幅度%"`
	Efficiency       float64 `gorm:"column:efficiency;type:decimal(10,4);default:0" description:"趋势效率"`
	// 关联预测的价格区间上下沿(期望价即由它推导，冗余出来供详情展示)
	PredHigh  float64 `gorm:"column:pred_high;type:decimal(36,18);default:0" description:"预测区间上沿"`
	PredLow   float64 `gorm:"column:pred_low;type:decimal(36,18);default:0" description:"预测区间下沿"`
	PredClose float64 `gorm:"column:pred_close;type:decimal(36,18);default:0" description:"预测收盘价(AI预测价)"`
	// 信号后窗口的实际开/收盘价(取窗口首根开盘、末根收盘，与预测收盘对照)
	WindowOpen         float64 `gorm:"column:window_open;type:decimal(36,18);default:0" description:"信号后窗口实际开盘价"`
	WindowClose        float64 `gorm:"column:window_close;type:decimal(36,18);default:0" description:"信号后窗口实际收盘价"`
	MaxPriceDuringHold float64 `gorm:"column:max_price_during_hold;type:decimal(36,18);default:0" description:"持仓期间最高价"`
	MinPriceDuringHold float64 `gorm:"column:min_price_during_hold;type:decimal(36,18);default:0" description:"持仓期间最低价"`
	// 信号后窗口实际行情区间（与「持仓期间」不同：覆盖从信号时刻起到本笔收尾/数据用尽的整段，
	// 即使未成交也能看到价格区间——用于判断回踩限价为何没被触及）。
	WindowLow  float64 `gorm:"column:window_low;type:decimal(36,18);default:0" description:"信号后窗口最低价"`
	WindowHigh float64 `gorm:"column:window_high;type:decimal(36,18);default:0" description:"信号后窗口最高价"`
	// 本笔所属压力面(信号时刻最近一次分析)的关键结构位：最高=关键阻力、最低=关键支撑(0=无)。
	PressureHigh float64 `gorm:"column:pressure_high;type:decimal(36,18);default:0" description:"压力面最高价(关键阻力)"`
	PressureLow  float64 `gorm:"column:pressure_low;type:decimal(36,18);default:0" description:"压力面最低价(关键支撑)"`
}

func (t *TradeBacktestTrade) TableName() string { return "trade_backtest_trade" }

// TradeBacktestMetric 回测汇总指标：一次回测一行，是横向对比「哪个策略有效」的最终依据。
type TradeBacktestMetric struct {
	db.BaseEntity
	RunID    int64  `gorm:"column:run_id;type:bigint;not null;uniqueIndex:idx_bt_metric_run,priority:1" description:"关联回测任务ID"`
	CalcMode string `gorm:"column:calc_mode;type:varchar(16);default:'prediction';uniqueIndex:idx_bt_metric_run,priority:2" description:"结算口径 prediction/trading"`
	// 笔数与成交
	TradeCount   int     `gorm:"column:trade_count;type:int;default:0" description:"信号总数(含未成交)"`
	FillCount    int     `gorm:"column:fill_count;type:int;default:0" description:"成交笔数"`
	ExpiredCount int     `gorm:"column:expired_count;type:int;default:0" description:"未成交笔数"`
	FillRate     float64 `gorm:"column:fill_rate;type:decimal(6,4);default:0" description:"成交率 = fill/(fill+expired)"`
	// 胜负与盈亏
	WinCount int     `gorm:"column:win_count;type:int;default:0" description:"盈利笔数"`
	WinRate  float64 `gorm:"column:win_rate;type:decimal(6,4);default:0" description:"胜率"`
	GrossPnl float64 `gorm:"column:gross_pnl;type:decimal(36,18);default:0" description:"毛盈亏 USDT"`
	FeeTotal float64 `gorm:"column:fee_total;type:decimal(36,18);default:0" description:"总手续费 USDT"`
	NetPnl   float64 `gorm:"column:net_pnl;type:decimal(36,18);default:0" description:"净盈亏 USDT"`
	// 风险调整
	Expectancy   float64 `gorm:"column:expectancy;type:decimal(36,18);default:0" description:"单笔期望净利"`
	ProfitFactor float64 `gorm:"column:profit_factor;type:decimal(18,8);default:0" description:"盈亏比 总盈/总亏"`
	MaxDrawdown  float64 `gorm:"column:max_drawdown;type:decimal(36,18);default:0" description:"最大回撤 USDT"`
	Sharpe       float64 `gorm:"column:sharpe;type:decimal(18,8);default:0" description:"夏普比率"`
	AvgHoldSecs  float64 `gorm:"column:avg_hold_secs;type:decimal(18,4);default:0" description:"平均持仓秒数"`
	// 出口分布
	TpCount      int `gorm:"column:tp_count;type:int;default:0" description:"止盈笔数"`
	SlCount      int `gorm:"column:sl_count;type:int;default:0" description:"止损笔数"`
	TimeoutCount int `gorm:"column:timeout_count;type:int;default:0" description:"超时平仓笔数"`
}

func (m *TradeBacktestMetric) TableName() string { return "trade_backtest_metric" }
