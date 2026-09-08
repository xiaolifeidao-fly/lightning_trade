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
	// platform_code 并入唯一键：同一 symbol+周期 下 DeepCoin 与币安两条行情各存一份，互不覆盖。
	PlatformCode string    `gorm:"column:platform_code;type:varchar(32);not null;default:binance;uniqueIndex:idx_kline_platform_dim,priority:1;index:idx_platform_code" orm:"column(platform_code);size(32);null" description:"行情平台代码 binance/deepcoin"`
	Symbol       string    `gorm:"column:symbol;type:varchar(32);uniqueIndex:idx_kline_platform_dim,priority:2;index:idx_symbol" orm:"column(symbol);size(32);null" description:"交易对"`
	Interval     string    `gorm:"column:interval;type:varchar(8);uniqueIndex:idx_kline_platform_dim,priority:3" orm:"column(interval);size(8);null" description:"周期 1m/5m/15m/1h/4h/1d"`
	OpenTime     time.Time `gorm:"column:open_time;type:datetime;uniqueIndex:idx_kline_platform_dim,priority:4" orm:"column(open_time);null" description:"开始时间"`
	CloseTime    time.Time `gorm:"column:close_time;type:datetime" orm:"column(close_time);null" description:"结束时间"`
	OpenPrice    float64   `gorm:"column:open_price;type:decimal(36,18);default:0" orm:"column(open_price);null" description:"开盘价"`
	HighPrice    float64   `gorm:"column:high_price;type:decimal(36,18);default:0" orm:"column(high_price);null" description:"最高价"`
	LowPrice     float64   `gorm:"column:low_price;type:decimal(36,18);default:0" orm:"column(low_price);null" description:"最低价"`
	ClosePrice   float64   `gorm:"column:close_price;type:decimal(36,18);default:0" orm:"column(close_price);null" description:"收盘价"`
	Volume       float64   `gorm:"column:volume;type:decimal(36,18);default:0" orm:"column(volume);null" description:"成交量"`
	Turnover     float64   `gorm:"column:turnover;type:decimal(36,18);default:0" orm:"column(turnover);null" description:"成交额"`
	TradeCount   uint64    `gorm:"column:trade_count;type:bigint unsigned;default:0" orm:"column(trade_count);null" description:"成交笔数"`
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
	// 复合方向门槛：1=须信号时刻 4h/12h/1d 最近预测方向全部与本笔一致才建仓，否则忽略。0=不启用。
	RequireCompositeDir int8 `gorm:"column:require_composite_dir;type:tinyint;default:0" description:"复合方向门槛 1启用 0不启用"`
	// 持仓参数
	HoldDuration    int `gorm:"column:hold_duration;type:int;default:14400" description:"持仓时长(秒)，即预测周期口径的持仓上限，默认 4h=14400"`
	MaxHoldDuration int `gorm:"column:max_hold_duration;type:int;default:86400" description:"最长持仓硬上限(秒)，防极端行情挂单，默认 24h=86400"`
	// 交易周期(可选)：如 1d/1w。设了则回测额外按交易周期口径再算一套(持仓上限拉到该周期，TP/SL 用该周期预测区间)。空=只按预测周期。
	TradingPeriod string `gorm:"column:trading_period;type:varchar(8);default:''" description:"交易周期 1h/4h/12h/1d/1w，空=不启用交易周期口径"`
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
	// 移动止盈(峰值回撤 + 时间收敛)：0=不启用，退回静态止盈。含杠杆 ROI% 口径。
	TrailActivatePct float64 `gorm:"column:trail_activate_pct;type:decimal(12,4);default:0.0000" description:"移动止盈激活阈值：浮盈ROI%(含杠杆)达此值才启动，0=不启用"`
	TrailGiveback    float64 `gorm:"column:trail_giveback;type:decimal(6,4);default:0.0000" description:"峰值回撤比例r0(0~1)：从峰值ROI回撤此比例即平仓，退出线=peak×(1-r)"`
	TrailGivebackMin float64 `gorm:"column:trail_giveback_min;type:decimal(6,4);default:0.0000" description:"周期末回撤比例(时间收敛)：随持仓推进r从trail_giveback线性收敛到此值，<=0或≥trail_giveback=不收敛"`
	// 早段疲软离场：持仓过 early_cut_time_pct% 时，若峰值浮盈ROI%(含杠杆)仍<early_cut_min_profit_pct，按当时价市价平仓。0=不启用。
	EarlyCutTimePct      float64 `gorm:"column:early_cut_time_pct;type:decimal(6,2);default:0.00" description:"早段疲软触发时间点：持仓已过交易周期的百分比(0~100，0=不启用)"`
	EarlyCutMinProfitPct float64 `gorm:"column:early_cut_min_profit_pct;type:decimal(12,4);default:0.0000" description:"早段疲软利润门槛：该时点前峰值浮盈ROI%(含杠杆)低于此值则离场"`
	// 早段逆行离场(MAE 软止损)：本笔从未走出浮盈且逆行浮亏ROI%(含杠杆)达阈值时，先于硬止损减损离场。0=不启用。
	EarlyCutMaxAdversePct float64 `gorm:"column:early_cut_max_adverse_pct;type:decimal(12,4);default:0.0000" description:"早段逆行止损阈值：逆行浮亏ROI%(含杠杆)达此值即离场，0=不启用"`
	EarlyCutArmProfitPct  float64 `gorm:"column:early_cut_arm_profit_pct;type:decimal(12,4);default:0.0000" description:"逆行止损解除阈值：峰值浮盈ROI%(含杠杆)达此值则解除软止损放行扛单，<=0=始终武装"`
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

	// ─── 盘口信号回测（engine_kind=signal）专用 ────────────────────────────────
	// 两套引擎共用本表：预测驱动（engine.go）与信号驱动（strategy/signal）。
	// 既有行没有这一列，AutoMigrate 会按 default 回填成 prediction，读侧据此分流。
	EngineKind   string `gorm:"column:engine_kind;type:varchar(16);default:'prediction';index:idx_bt_run_engine" description:"引擎类型 prediction/signal"`
	InstanceKey  string `gorm:"column:instance_key;type:varchar(64)" description:"信号源实例键(argus_instance.instance_key)"`
	AccountLabel string `gorm:"column:account_label;type:varchar(128)" description:"信号源账户(strategy_event.account_label)"`
	SignalSource string `gorm:"column:signal_source;type:varchar(32)" description:"信号来源表，固定 strategy_event"`
	// Fidelity 精度等级：event=事件级(触发点精确到秒) / frequency=频率级(只能推λ(θ))。
	// 需求大纲 §3.3：两级结果不得混排比较，所以它必须落在 run 上而不只是算出来。
	Fidelity     string `gorm:"column:fidelity;type:varchar(16)" description:"精度等级 event/frequency"`
	FidelityNote string `gorm:"column:fidelity_note;type:varchar(1024)" description:"必须随结果展示的精度警示"`
	SignalCount  int    `gorm:"column:signal_count;type:int;default:0" description:"回放消费的真实触发数"`

	// ─── 参数组批量扫描（r11）────────────────────────────────────────────────────
	// 批量扫描不另建一张"批量逐组"表：一组参数就是一条完整的 run（自带
	// params_snapshot / fidelity / metric / 逐笔），只是多了个归属批次。这样
	// 批量里的任一组都能直接在既有 run 详情页打开，也能单独重跑。
	// batch_id=0 表示单跑的 run（既有行 AutoMigrate 后就是 0）。
	BatchID    int64  `gorm:"column:batch_id;type:bigint;default:0;index:idx_bt_run_batch" description:"所属批量扫描ID，0=单跑"`
	GroupLabel string `gorm:"column:group_label;type:varchar(128)" description:"批量扫描中的参数组标签"`
	IsBaseline uint8  `gorm:"column:is_baseline;type:tinyint;default:0" description:"是否本批次的基线组"`
}

func (r *TradeBacktestRun) TableName() string { return "trade_backtest_run" }

// TradeBacktestBatch 参数组批量扫描（r11）：一次批量 = 一行，下挂 N 条
// trade_backtest_run（每组参数一条）。
//
// 批次只承载"这一批共享什么、跑到哪一步、基线是谁"，指标一律不冗余在这里——
// 汇总指标的唯一来源是 trade_backtest_metric，横向对比页按 run_id 关联去取。
// 冗余一份就会出现"批次上的净利与组详情里的净利不一致"这类无法排查的问题。
type TradeBacktestBatch struct {
	db.BaseEntity
	Name string `gorm:"column:name;type:varchar(128)" description:"批次名"`
	// 共享的信号源与窗口：批内全部组必须完全一致，否则组间差异里混进数据差异。
	InstanceKey  string    `gorm:"column:instance_key;type:varchar(64);index:idx_bt_batch_inst" description:"信号源实例键"`
	AccountLabel string    `gorm:"column:account_label;type:varchar(128)" description:"信号源账户"`
	PlatformCode string    `gorm:"column:platform_code;type:varchar(32)" description:"1m 路径回放平台"`
	Symbol       string    `gorm:"column:symbol;type:varchar(32)" description:"交易对"`
	CoinCode     string    `gorm:"column:coin_code;type:varchar(32)" description:"基础币种"`
	StartTime    time.Time `gorm:"column:start_time;type:datetime" description:"扫描窗口起"`
	EndTime      time.Time `gorm:"column:end_time;type:datetime" description:"扫描窗口止"`
	// BaselineSnapshot 基线参数冻结快照（JSON，signal.Params）。与基线的 diff
	// 一律按它算，不按"重新读一次当前生产配置"——生产参数随时会被热更，
	// 事后再读会让同一个批次今天和明天算出不同的差异。
	BaselineSnapshot string `gorm:"column:baseline_snapshot;type:text" description:"基线参数冻结快照(JSON)"`
	BaselineSource   string `gorm:"column:baseline_source;type:varchar(32)" description:"基线来源 instance_published/request"`
	// BaselineNote 基线里哪些字段没能从 DB 取到、用了什么兜底（配置面收敛未完成时必然有）。
	BaselineNote  string `gorm:"column:baseline_note;type:varchar(1024)" description:"基线取值来源与兜底说明"`
	BaselineRunID int64  `gorm:"column:baseline_run_id;type:bigint;default:0" description:"基线组对应的 run_id，0=未跑基线"`

	Concurrency int    `gorm:"column:concurrency;type:int;default:1" description:"并发执行的组数上限"`
	GroupCount  int    `gorm:"column:group_count;type:int;default:0" description:"本批次的参数组数(含基线组)"`
	DoneCount   int    `gorm:"column:done_count;type:int;default:0" description:"已完成组数"`
	FailedCount int    `gorm:"column:failed_count;type:int;default:0" description:"失败组数"`
	Status      string `gorm:"column:status;type:varchar(16);default:'pending'" description:"状态 pending/running/done/partial/failed"`
	ErrorMsg    string `gorm:"column:error_msg;type:varchar(512)" description:"批次级失败原因(取数失败等)"`
}

func (b *TradeBacktestBatch) TableName() string { return "trade_backtest_batch" }

// TradeOptimizeStudy 后台自动参数寻优任务（r16）：一次扫描 = 一行，下挂 N 条
// trade_optimize_cell（一格一条，粗网格与精算格用 stage 区分）。
//
// 为什么**不复用** trade_backtest_run 逐路径落库：降噪协议每格 16 条路径，
// 40 格就是 640 条 run + 640 条 metric + 数万行逐笔，而其中任何一条单路径都
// **不是决策依据**（单路径混沌 ±70U）。真正要留档的是格级的中位数/IQR/一致率/
// 情景分位。要看某一格的逐笔，把该格的 params_snapshot 提交给
// POST /backtest/signal-runs 单跑一次即可——参数快照就是完整的 signal.Params。
//
// 阈值三件套（gate_snapshot / gate_locked_at / gate_note）是本表最关键的列：
// 预注册的意思就是"发起时写死、跑完不可改"，改阈值只能建新任务。
type TradeOptimizeStudy struct {
	db.BaseEntity
	Name string `gorm:"column:name;type:varchar(128)" description:"扫描任务名"`
	// 信号源与窗口：全部格子共享同一份触发流，否则格间差异里混进数据差异。
	InstanceKey  string    `gorm:"column:instance_key;type:varchar(64);index:idx_opt_study_inst" description:"信号源实例键"`
	AccountLabel string    `gorm:"column:account_label;type:varchar(128)" description:"信号源账户"`
	PlatformCode string    `gorm:"column:platform_code;type:varchar(32)" description:"1m 路径回放平台"`
	Symbol       string    `gorm:"column:symbol;type:varchar(32)" description:"交易对"`
	CoinCode     string    `gorm:"column:coin_code;type:varchar(32)" description:"基础币种"`
	StartTime    time.Time `gorm:"column:start_time;type:datetime" description:"扫描窗口起"`
	EndTime      time.Time `gorm:"column:end_time;type:datetime" description:"扫描窗口止"`

	// OOS 纪律：in_sample = 用于推参数的窗口（收益必然高估）；
	// out_of_sample = 以某次扫描为基准、继承其冻结阈值与精算格，只换数据窗口。
	SampleKind string `gorm:"column:sample_kind;type:varchar(16);default:'in_sample'" description:"样本口径 in_sample/out_of_sample"`
	SampleNote string `gorm:"column:sample_note;type:varchar(512)" description:"样本口径说明"`
	OosBaseID  int64  `gorm:"column:oos_base_id;type:bigint;default:0" description:"out_of_sample 任务继承的基准扫描 id，0=无"`

	// 冻结快照：结果可复算的唯一凭据。
	SpaceSnapshot     string    `gorm:"column:space_snapshot;type:text" description:"搜索空间快照(JSON signal.SearchSpace)"`
	ProtocolSnapshot  string    `gorm:"column:protocol_snapshot;type:text" description:"降噪协议快照(JSON signal.Protocol)"`
	ConvergeSnapshot  string    `gorm:"column:converge_snapshot;type:text" description:"粗→精收敛规则快照(JSON signal.Convergence)"`
	GateSnapshot      string    `gorm:"column:gate_snapshot;type:text" description:"预注册三关阈值快照(JSON signal.Gates)"`
	GateLockedAt      time.Time `gorm:"column:gate_locked_at;type:datetime" description:"阈值锁定时刻(发起时写死，跑完不可改)"`
	GateNote          string    `gorm:"column:gate_note;type:varchar(512)" description:"三关阈值的可读表述"`
	BaselineSnapshot  string    `gorm:"column:baseline_snapshot;type:text" description:"基线参数冻结快照(JSON signal.Params)"`
	BaselineSource    string    `gorm:"column:baseline_source;type:varchar(32)" description:"基线来源 instance_published/request"`
	BaselineNote      string    `gorm:"column:baseline_note;type:varchar(1024)" description:"基线取值来源与兜底说明"`
	IncumbentSnapshot string    `gorm:"column:incumbent_snapshot;type:varchar(512)" description:"现行配置格快照(JSON signal.CellSpec)，空=未解析出"`

	// 执行进度
	Stage           string `gorm:"column:stage;type:varchar(16);default:'coarse'" description:"阶段 coarse/fine/concluded"`
	Concurrency     int    `gorm:"column:concurrency;type:int;default:1" description:"并发执行的格数上限"`
	CoarseCellCount int    `gorm:"column:coarse_cell_count;type:int;default:0" description:"粗网格格数"`
	FineCellCount   int    `gorm:"column:fine_cell_count;type:int;default:0" description:"精算格数"`
	DoneCellCount   int    `gorm:"column:done_cell_count;type:int;default:0" description:"已完成格数"`
	FailedCellCount int    `gorm:"column:failed_cell_count;type:int;default:0" description:"失败格数"`
	SkipCellCount   int    `gorm:"column:skip_cell_count;type:int;default:0" description:"参数非法被跳过的格数"`
	ReplayCount     int    `gorm:"column:replay_count;type:int;default:0" description:"累计回放次数(格数×路径数)"`
	Status          string `gorm:"column:status;type:varchar(16);default:'pending'" description:"状态 pending/running/done/partial/failed"`
	ErrorMsg        string `gorm:"column:error_msg;type:varchar(512)" description:"任务级失败原因"`
	ConvergeNote    string `gorm:"column:converge_note;type:varchar(1024)" description:"精算格是怎么从粗网格收敛出来的"`

	// 数据侧事实
	SignalCount   int `gorm:"column:signal_count;type:int;default:0" description:"窗口内真实触发数"`
	KlineCount    int `gorm:"column:kline_count;type:int;default:0" description:"窗口内 1m 根数"`
	TrendDayCount int `gorm:"column:trend_day_count;type:int;default:0" description:"窗口内单边日天数(熊市月情景的样本基数)"`
	VolDayCount   int `gorm:"column:vol_day_count;type:int;default:0" description:"窗口内震荡日天数"`

	// 结论
	Verdict          string `gorm:"column:verdict;type:varchar(24)" description:"结论 candidate_found/no_solution，空=未出结论"`
	PassedCellCount  int    `gorm:"column:passed_cell_count;type:int;default:0" description:"通过三关的格数"`
	Conclusion       string `gorm:"column:conclusion;type:text" description:"结论正文(JSON signal.Conclusion)"`
	FrontierSnapshot string `gorm:"column:frontier_snapshot;type:text" description:"权衡前沿快照(JSON []signal.FrontierPoint)"`
	ScaleSnapshot    string `gorm:"column:scale_snapshot;type:text" description:"规模不变性核查快照(JSON []signal.ScaleInvarianceCheck)"`
}

func (s *TradeOptimizeStudy) TableName() string { return "trade_optimize_study" }

// TradeOptimizeCell 搜索空间里的一格 + 它的降噪产出与三关判定。
//
// 每个统计列都是 N 条路径的聚合，**没有任何一列是单路径点估计**——这是本表
// 与 trade_backtest_metric 的根本区别，也是它不能塞进那张表的原因。
type TradeOptimizeCell struct {
	db.BaseEntity
	StudyID int64   `gorm:"column:study_id;type:bigint;not null;index:idx_opt_cell_study" description:"关联寻优任务ID"`
	Stage   string  `gorm:"column:stage;type:varchar(16);index:idx_opt_cell_study" description:"阶段 coarse/fine"`
	CellKey string  `gorm:"column:cell_key;type:varchar(64)" description:"格子标识 如 net(26,400,g20)"`
	Mode    string  `gorm:"column:mode;type:varchar(8)" description:"持仓形态 net/dual"`
	Cap     int     `gorm:"column:cap_contracts;type:int;default:0" description:"仓位上限张数(dual 为每侧上限)；999=∞对照"`
	StopPct float64 `gorm:"column:stop_pct;type:decimal(18,6);default:0" description:"S 兜底止损 ROI%"`
	GatePct float64 `gorm:"column:gate_pct;type:decimal(18,6);default:0" description:"反向门控最低盈利%"`

	ParamsSnapshot string `gorm:"column:params_snapshot;type:text" description:"本格完整参数快照(JSON signal.Params)"`
	Fidelity       string `gorm:"column:fidelity;type:varchar(16);default:'event'" description:"精度等级；寻优只做事件级"`
	Status         string `gorm:"column:status;type:varchar(16);default:'pending'" description:"状态 pending/running/done/failed/skipped"`
	ErrorMsg       string `gorm:"column:error_msg;type:varchar(512)" description:"失败或跳过原因"`
	PathCount      int    `gorm:"column:path_count;type:int;default:0" description:"实际跑通的抖动路径数"`

	// 降噪产出
	MedPnl28  float64 `gorm:"column:med_pnl28;type:decimal(24,8);default:0" description:"中位净利(归一到 28 天)"`
	P25Pnl28  float64 `gorm:"column:p25_pnl28;type:decimal(24,8);default:0" description:"净利 p25"`
	P75Pnl28  float64 `gorm:"column:p75_pnl28;type:decimal(24,8);default:0" description:"净利 p75"`
	IqrPnl28  float64 `gorm:"column:iqr_pnl28;type:decimal(24,8);default:0" description:"净利 IQR"`
	MinPnl28  float64 `gorm:"column:min_pnl28;type:decimal(24,8);default:0" description:"净利最小值"`
	MaxPnl28  float64 `gorm:"column:max_pnl28;type:decimal(24,8);default:0" description:"净利最大值"`
	SignRatio float64 `gorm:"column:sign_ratio;type:decimal(18,6);default:0" description:"符号一致率(pnl>0 的路径占比)"`

	LambdaBear   float64 `gorm:"column:lambda_bear;type:decimal(18,6);default:0" description:"单边日口径兜底频率 次/月"`
	MeanStopLoss float64 `gorm:"column:mean_stop_loss;type:decimal(24,8);default:0" description:"单次兜底平均损失额"`
	StopBudget   float64 `gorm:"column:stop_budget;type:decimal(24,8);default:0" description:"月度兜底预算消耗 λ×损失"`
	StopCount    int     `gorm:"column:stop_count;type:int;default:0" description:"全路径兜底总次数"`

	P90MaxDrawdown float64 `gorm:"column:p90_max_drawdown;type:decimal(24,8);default:0" description:"p90 MTM 最大回撤"`
	MaxStack       int     `gorm:"column:max_stack;type:int;default:0" description:"最大堆积张数"`
	MedFee         float64 `gorm:"column:med_fee;type:decimal(24,8);default:0" description:"中位手续费"`
	MedDays        float64 `gorm:"column:med_days;type:decimal(18,6);default:0" description:"中位覆盖天数"`
	MedSignalRun   int     `gorm:"column:med_signal_run;type:int;default:0" description:"中位回放触发数"`

	// 情景 bootstrap
	BearPoolSize int     `gorm:"column:bear_pool_size;type:int;default:0" description:"熊市月重采样池大小"`
	BearP10      float64 `gorm:"column:bear_p10;type:decimal(24,8);default:0" description:"熊市月净 p10"`
	BearP50      float64 `gorm:"column:bear_p50;type:decimal(24,8);default:0" description:"熊市月净 p50"`
	BearP90      float64 `gorm:"column:bear_p90;type:decimal(24,8);default:0" description:"熊市月净 p90"`
	ChopP10      float64 `gorm:"column:chop_p10;type:decimal(24,8);default:0" description:"震荡月净 p10"`
	ChopP50      float64 `gorm:"column:chop_p50;type:decimal(24,8);default:0" description:"震荡月净 p50"`
	MixedP10     float64 `gorm:"column:mixed_p10;type:decimal(24,8);default:0" description:"混合月净 p10"`
	MixedP50     float64 `gorm:"column:mixed_p50;type:decimal(24,8);default:0" description:"混合月净 p50"`

	// 三关判定（阈值取自任务冻结的 gate_snapshot）
	OkSign      int    `gorm:"column:ok_sign;type:tinyint;default:0" description:"符号一致率关 1=过"`
	OkBear      int    `gorm:"column:ok_bear;type:tinyint;default:0" description:"熊市月净 p10 关 1=过"`
	OkDd        int    `gorm:"column:ok_dd;type:tinyint;default:0" description:"p90 回撤关 1=过"`
	OkBudget    int    `gorm:"column:ok_budget;type:tinyint;default:0" description:"参考项：频率预算 1=过(不计入三关)"`
	Passed      int    `gorm:"column:passed;type:tinyint;default:0" description:"三关全过 1=候选"`
	PassCount   int    `gorm:"column:pass_count;type:int;default:0" description:"通过的关数 0-3"`
	VerdictNote string `gorm:"column:verdict_note;type:varchar(1024)" description:"未过关的逐条原因"`

	// 支配关系与前沿
	DominatedBy    string `gorm:"column:dominated_by;type:varchar(512)" description:"全面支配本格的格子键，逗号分隔"`
	IsIncumbent    int    `gorm:"column:is_incumbent;type:tinyint;default:0" description:"1=现行线上配置对应的格"`
	OnDdFrontier   int    `gorm:"column:on_dd_frontier;type:tinyint;default:0" description:"1=在收益/回撤前沿上"`
	OnBearFrontier int    `gorm:"column:on_bear_frontier;type:tinyint;default:0" description:"1=在收益/熊市尾部前沿上"`

	PathsSnapshot string `gorm:"column:paths_snapshot;type:text" description:"逐路径产出快照(JSON []signal.PathOutcome)"`
	NoteSnapshot  string `gorm:"column:note_snapshot;type:text" description:"本格必须随结果展示的口径说明(JSON []string)"`
}

func (c *TradeOptimizeCell) TableName() string { return "trade_optimize_cell" }

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
	Pnl        float64 `gorm:"column:pnl;type:decimal(36,18);default:0" description:"盈亏金额 USDT"`
	PnlRate    float64 `gorm:"column:pnl_rate;type:decimal(18,8);default:0" description:"盈亏率%(含杠杆，未扣费)"`
	Fee        float64 `gorm:"column:fee;type:decimal(36,18);default:0" description:"往返手续费 USDT"`
	NetPnl     float64 `gorm:"column:net_pnl;type:decimal(36,18);default:0" description:"净盈亏 = pnl - fee"`
	NetPnlRate float64 `gorm:"column:net_pnl_rate;type:decimal(18,8);default:0" description:"净盈亏率%(含杠杆，已扣往返手续费)"`
	Leverage   float64 `gorm:"column:leverage;type:decimal(6,2);default:1" description:"杠杆倍数(展示含杠杆浮盈用)"`
	// 分时段峰值浮盈：JSON 数组[10]，第 i 项=前(i+1)×10%持仓时间内累积最高浮盈ROI%(含杠杆)，供"前X%时间内最大利润<Y%"筛选。
	FavPeakDeciles string `gorm:"column:fav_peak_deciles;type:varchar(255)" description:"分时段峰值浮盈十分位JSON[10](含杠杆ROI%)"`
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

	// ─── 盘口信号回测（calc_mode=signal）专用 ──────────────────────────────────
	// 信号驱动的一行 = 一个持仓生命周期（net 仓从 0 张累积到平仓/削零），
	// 不是一个预测信号，所以需要张数与加仓次数这两个净仓口径的字段。
	Contracts    int `gorm:"column:contracts;type:int;default:0" description:"平仓张数"`
	MaxContracts int `gorm:"column:max_contracts;type:int;default:0" description:"生命周期内最大张数"`
	AddCount     int `gorm:"column:add_count;type:int;default:0" description:"加仓次数(含首次建仓)"`
	// PeakPct 回测口径的 trail 峰值：用 1m high 计算，相对实盘 5 秒轮询系统性偏高。
	PeakPct    float64 `gorm:"column:peak_pct;type:decimal(14,6);default:0" description:"移动止盈峰值ROI%(回测口径，偏高)"`
	ReducedPnl float64 `gorm:"column:reduced_pnl;type:decimal(36,18);default:0" description:"生命周期内反向减仓锁利累计"`
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
	TpCount           int `gorm:"column:tp_count;type:int;default:0" description:"止盈笔数"`
	SlCount           int `gorm:"column:sl_count;type:int;default:0" description:"止损笔数"`
	TrailCount        int `gorm:"column:trail_count;type:int;default:0" description:"移动止盈平仓笔数"`
	EarlyCutCount     int `gorm:"column:early_cut_count;type:int;default:0" description:"早段疲软离场笔数"`
	EarlyAdverseCount int `gorm:"column:early_adverse_count;type:int;default:0" description:"早段逆行离场笔数"`
	TimeoutCount      int `gorm:"column:timeout_count;type:int;default:0" description:"超时平仓笔数"`

	// ─── 盘口信号回测（calc_mode=signal）专用 ──────────────────────────────────
	// Fidelity 冗余到 metric 而不只放 run 上：横向对比页是按 metric 行排序的，
	// 精度等级必须跟着每一行走，否则事件级与频率级会被排进同一张榜。
	Fidelity     string `gorm:"column:fidelity;type:varchar(16)" description:"精度等级 event/frequency"`
	FidelityNote string `gorm:"column:fidelity_note;type:varchar(1024)" description:"精度警示"`

	SignalCount      int     `gorm:"column:signal_count;type:int;default:0" description:"回放消费的真实触发数"`
	SignalDropped    int     `gorm:"column:signal_dropped;type:int;default:0" description:"未回放的触发数(种子之前/无K线覆盖)"`
	SignalFiltered   int     `gorm:"column:signal_filtered;type:int;default:0" description:"抬高阈值后被|gapBp|门限筛掉的触发数"`
	CapSkipCount     int     `gorm:"column:cap_skip_count;type:int;default:0" description:"仓位上限跳过次数"`
	GateSkipCount    int     `gorm:"column:gate_skip_count;type:int;default:0" description:"反向门控拦截次数"`
	TrendSkipCount   int     `gorm:"column:trend_skip_count;type:int;default:0" description:"趋势闸拦截次数"`
	ReduceCount      int     `gorm:"column:reduce_count;type:int;default:0" description:"盈利减仓次数"`
	ReduceCloseCount int     `gorm:"column:reduce_close_count;type:int;default:0" description:"被减仓削零结束的生命周期数"`
	EodOpenCount     int     `gorm:"column:eod_open_count;type:int;default:0" description:"窗口结束仍持仓的生命周期数"`
	MaxStack         int     `gorm:"column:max_stack;type:int;default:0" description:"最大堆积张数"`
	CapEffective     int     `gorm:"column:cap_effective;type:int;default:0" description:"本次回放生效的仓位上限"`
	RealizedPnl      float64 `gorm:"column:realized_pnl;type:decimal(36,18);default:0" description:"已实现盈亏(含减仓锁利)"`
	FloatingPnl      float64 `gorm:"column:floating_pnl;type:decimal(36,18);default:0" description:"期末浮动盈亏"`
	MaxDrawdownPct   float64 `gorm:"column:max_drawdown_pct;type:decimal(14,6);default:0" description:"最大回撤占风险基数%"`
	// λ(θ)：只有频率级(改了 signal_threshold)的组有值。LambdaSelfTest 是
	// θ0 的 λ 与真实事件密度之比，应≈1；偏离说明 λ 推断本身不可信。
	LambdaPerDay   float64 `gorm:"column:lambda_per_day;type:decimal(18,6);default:0" description:"目标阈值下的λ(次/天)"`
	LambdaRatio    float64 `gorm:"column:lambda_ratio;type:decimal(18,6);default:0" description:"λ(θ)/λ(θ0)"`
	LambdaSelfTest float64 `gorm:"column:lambda_self_test;type:decimal(18,6);default:0" description:"λ自检比值，应≈1"`
}

func (m *TradeBacktestMetric) TableName() string { return "trade_backtest_metric" }
