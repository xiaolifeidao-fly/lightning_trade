package dto

import (
	baseDTO "common/base/dto"
	"time"
)

type TradeOrderDTO struct {
	baseDTO.BaseDTO
	PlatformID     uint64    `json:"platformId"`
	PlatformCode   string    `json:"platformCode"`
	TradeCategory  string    `json:"tradeCategory"`
	TradeType      string    `json:"tradeType"`
	OrderNo        string    `json:"orderNo"`
	UserID         uint64    `json:"userId"`
	Symbol         string    `json:"symbol"`
	BaseCoinCode   string    `json:"baseCoinCode"`
	QuoteCoinCode  string    `json:"quoteCoinCode"`
	Side           string    `json:"side"`
	OrderType      string    `json:"orderType"`
	Price          float64   `json:"price"`
	Amount         float64   `json:"amount"`
	Total          float64   `json:"total"`
	StopPrice      float64   `json:"stopPrice"`
	FilledAmount   float64   `json:"filledAmount"`
	FilledTotal    float64   `json:"filledTotal"`
	AvgFilledPrice float64   `json:"avgFilledPrice"`
	FeeCoinCode    string    `json:"feeCoinCode"`
	FeeAmount      float64   `json:"feeAmount"`
	Status         string    `json:"status"`
	TimeInForce    string    `json:"timeInForce"`
	Source         string    `json:"source"`
	ClientOrderID  string    `json:"clientOrderId"`
	SubmittedTime  time.Time `json:"submittedTime"`
	FinishedTime   time.Time `json:"finishedTime"`
	CancelReason   string    `json:"cancelReason"`
}

type CreateTradeOrderDTO struct {
	PlatformID    uint64  `json:"platformId"`
	PlatformCode  string  `json:"platformCode"`
	TradeCategory string  `json:"tradeCategory"`
	TradeType     string  `json:"tradeType"`
	UserID        uint64  `json:"userId"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	OrderType     string  `json:"orderType"`
	Price         float64 `json:"price"`
	Amount        float64 `json:"amount"`
	StopPrice     float64 `json:"stopPrice"`
	TimeInForce   string  `json:"timeInForce"`
	Source        string  `json:"source"`
	ClientOrderID string  `json:"clientOrderId"`
}

type CancelTradeOrderDTO struct {
	OrderNo string `json:"orderNo"`
	Reason  string `json:"reason"`
}

type UpdateTradeOrderFillDTO struct {
	FilledAmount   *float64 `json:"filledAmount,omitempty"`
	FilledTotal    *float64 `json:"filledTotal,omitempty"`
	AvgFilledPrice *float64 `json:"avgFilledPrice,omitempty"`
	FeeAmount      *float64 `json:"feeAmount,omitempty"`
	Status         *string  `json:"status,omitempty"`
}

type TradeOrderQueryDTO struct {
	Page          int    `form:"page"`
	PageIndex     int    `form:"pageIndex"`
	PageSize      int    `form:"pageSize"`
	PlatformID    uint64 `form:"platformId"`
	PlatformCode  string `form:"platformCode"`
	TradeCategory string `form:"tradeCategory"`
	TradeType     string `form:"tradeType"`
	UserID        uint64 `form:"userId"`
	Symbol        string `form:"symbol"`
	Side          string `form:"side"`
	OrderType     string `form:"orderType"`
	Status        string `form:"status"`
	OrderNo       string `form:"orderNo"`
	StartTime     int64  `form:"startTime"`
	EndTime       int64  `form:"endTime"`
}

type TradeMatchDTO struct {
	baseDTO.BaseDTO
	PlatformID   uint64    `json:"platformId"`
	PlatformCode string    `json:"platformCode"`
	TradeNo      string    `json:"tradeNo"`
	Symbol       string    `json:"symbol"`
	TakerOrderNo string    `json:"takerOrderNo"`
	MakerOrderNo string    `json:"makerOrderNo"`
	TakerUserID  uint64    `json:"takerUserId"`
	MakerUserID  uint64    `json:"makerUserId"`
	Side         string    `json:"side"`
	Price        float64   `json:"price"`
	Amount       float64   `json:"amount"`
	Total        float64   `json:"total"`
	TakerFee     float64   `json:"takerFee"`
	MakerFee     float64   `json:"makerFee"`
	MatchedTime  time.Time `json:"matchedTime"`
}

type CreateTradeMatchDTO struct {
	PlatformID   uint64    `json:"platformId"`
	PlatformCode string    `json:"platformCode"`
	Symbol       string    `json:"symbol"`
	TakerOrderNo string    `json:"takerOrderNo"`
	MakerOrderNo string    `json:"makerOrderNo"`
	TakerUserID  uint64    `json:"takerUserId"`
	MakerUserID  uint64    `json:"makerUserId"`
	Side         string    `json:"side"`
	Price        float64   `json:"price"`
	Amount       float64   `json:"amount"`
	Total        float64   `json:"total"`
	TakerFee     float64   `json:"takerFee"`
	MakerFee     float64   `json:"makerFee"`
	MatchedTime  time.Time `json:"matchedTime"`
}

type TradeMatchQueryDTO struct {
	Page         int    `form:"page"`
	PageIndex    int    `form:"pageIndex"`
	PageSize     int    `form:"pageSize"`
	PlatformID   uint64 `form:"platformId"`
	PlatformCode string `form:"platformCode"`
	UserID       uint64 `form:"userId"`
	Symbol       string `form:"symbol"`
	Limit        int    `form:"limit"`
}

type TradeKlineDTO struct {
	baseDTO.BaseDTO
	Symbol     string    `json:"symbol"`
	Interval   string    `json:"interval"`
	OpenTime   time.Time `json:"openTime"`
	CloseTime  time.Time `json:"closeTime"`
	OpenPrice  float64   `json:"openPrice"`
	HighPrice  float64   `json:"highPrice"`
	LowPrice   float64   `json:"lowPrice"`
	ClosePrice float64   `json:"closePrice"`
	Volume     float64   `json:"volume"`
	Turnover   float64   `json:"turnover"`
	TradeCount uint64    `json:"tradeCount"`
}

type TradeKlineQueryDTO struct {
	Symbol   string `form:"symbol"`
	Interval string `form:"interval"`
	Limit    int    `form:"limit"`
}

type TradeStatsDTO struct {
	Symbol         string  `json:"symbol"`
	TotalOrders    int     `json:"totalOrders"`
	OpenOrders     int     `json:"openOrders"`
	FilledOrders   int     `json:"filledOrders"`
	CanceledOrders int     `json:"canceledOrders"`
	Volume24h      float64 `json:"volume24h"`
	Turnover24h    float64 `json:"turnover24h"`
}

// TradeDetail DTOs
type TradeDetailDTO struct {
	baseDTO.BaseDTO
	PlatformID       uint64    `json:"platformId"`
	PlatformCode     string    `json:"platformCode"`
	TradeCategory    string    `json:"tradeCategory"`
	TradeType        string    `json:"tradeType"`
	UserID           uint64    `json:"userId"`
	OrderNo          string    `json:"orderNo"`
	TradeNo          string    `json:"tradeNo"`
	Symbol           string    `json:"symbol"`
	CoinCode         string    `json:"coinCode"`
	Side             string    `json:"side"`
	OpenDirection    string    `json:"openDirection"`
	AvgOpenPrice     float64   `json:"avgOpenPrice"`
	LiquidationPrice float64   `json:"liquidationPrice"`
	Leverage         float64   `json:"leverage"`
	Margin           float64   `json:"margin"`
	UserBalanceOpen  float64   `json:"userBalanceOpen"`
	Price            float64   `json:"price"`
	Amount           float64   `json:"amount"`
	Total            float64   `json:"total"`
	Fee              float64   `json:"fee"`
	Pnl              float64   `json:"pnl"`
	PnlRate          float64   `json:"pnlRate"`
	TradeTime        time.Time `json:"tradeTime"`
}

type CreateTradeDetailDTO struct {
	PlatformID       uint64    `json:"platformId"`
	PlatformCode     string    `json:"platformCode"`
	TradeCategory    string    `json:"tradeCategory"`
	TradeType        string    `json:"tradeType"`
	UserID           uint64    `json:"userId"`
	OrderNo          string    `json:"orderNo"`
	TradeNo          string    `json:"tradeNo"`
	Symbol           string    `json:"symbol"`
	CoinCode         string    `json:"coinCode"`
	Side             string    `json:"side"`
	OpenDirection    string    `json:"openDirection"`
	AvgOpenPrice     float64   `json:"avgOpenPrice"`
	LiquidationPrice float64   `json:"liquidationPrice"`
	Leverage         float64   `json:"leverage"`
	Margin           float64   `json:"margin"`
	UserBalanceOpen  float64   `json:"userBalanceOpen"`
	Price            float64   `json:"price"`
	Amount           float64   `json:"amount"`
	Total            float64   `json:"total"`
	Fee              float64   `json:"fee"`
	Pnl              float64   `json:"pnl"`
	PnlRate          float64   `json:"pnlRate"`
	TradeTime        time.Time `json:"tradeTime"`
}

type TradeDetailQueryDTO struct {
	Page          int    `form:"page"`
	PageIndex     int    `form:"pageIndex"`
	PageSize      int    `form:"pageSize"`
	PlatformID    uint64 `form:"platformId"`
	PlatformCode  string `form:"platformCode"`
	TradeCategory string `form:"tradeCategory"`
	TradeType     string `form:"tradeType"`
	UserID        uint64 `form:"userId"`
	OrderNo       string `form:"orderNo"`
	Symbol        string `form:"symbol"`
	CoinCode      string `form:"coinCode"`
	StartTime     int64  `form:"startTime"`
	EndTime       int64  `form:"endTime"`
}

// TradeUserSummary DTOs
type TradeUserSummaryDTO struct {
	baseDTO.BaseDTO
	UserID        uint64  `json:"userId"`
	PlatformID    uint64  `json:"platformId"`
	PlatformCode  string  `json:"platformCode"`
	CoinCode      string  `json:"coinCode"`
	TradeCategory string  `json:"tradeCategory"`
	TradeDate     string  `json:"tradeDate"`
	TotalOrders   int64   `json:"totalOrders"`
	BuyOrders     int64   `json:"buyOrders"`
	SellOrders    int64   `json:"sellOrders"`
	BuyAmount     float64 `json:"buyAmount"`
	SellAmount    float64 `json:"sellAmount"`
	BuyTotal      float64 `json:"buyTotal"`
	SellTotal     float64 `json:"sellTotal"`
	TotalFee      float64 `json:"totalFee"`
	TotalVolume   float64 `json:"totalVolume"`
}

type TradeUserSummaryQueryDTO struct {
	Page          int    `form:"page"`
	PageIndex     int    `form:"pageIndex"`
	PageSize      int    `form:"pageSize"`
	UserID        uint64 `form:"userId"`
	PlatformID    uint64 `form:"platformId"`
	PlatformCode  string `form:"platformCode"`
	CoinCode      string `form:"coinCode"`
	TradeCategory string `form:"tradeCategory"`
	StartDate     string `form:"startDate"`
	EndDate       string `form:"endDate"`
}

// TradeUserPnl DTOs
type TradeUserPnlDTO struct {
	baseDTO.BaseDTO
	UserID         uint64  `json:"userId"`
	PlatformID     uint64  `json:"platformId"`
	PlatformCode   string  `json:"platformCode"`
	CoinCode       string  `json:"coinCode"`
	TradeCategory  string  `json:"tradeCategory"`
	TradeDate      string  `json:"tradeDate"`
	RealizedPnl    float64 `json:"realizedPnl"`
	UnrealizedPnl  float64 `json:"unrealizedPnl"`
	TotalPnl       float64 `json:"totalPnl"`
	PnlRate        float64 `json:"pnlRate"`
	PositionAmount float64 `json:"positionAmount"`
	PositionCost   float64 `json:"positionCost"`
	PositionValue  float64 `json:"positionValue"`
}

type TradeUserPnlQueryDTO struct {
	Page          int    `form:"page"`
	PageIndex     int    `form:"pageIndex"`
	PageSize      int    `form:"pageSize"`
	UserID        uint64 `form:"userId"`
	PlatformID    uint64 `form:"platformId"`
	PlatformCode  string `form:"platformCode"`
	CoinCode      string `form:"coinCode"`
	TradeCategory string `form:"tradeCategory"`
	StartDate     string `form:"startDate"`
	EndDate       string `form:"endDate"`
}

type TradeSimulationAnalysisQueryDTO struct {
	PlatformCode    string `form:"platformCode"`
	CoinCode        string `form:"coinCode"`
	Interval        string `form:"interval"`        // K线展示周期，默认 1m
	PredictInterval string `form:"predictInterval"` // 预测时间间隔(horizon)，默认 5m
	Limit           int    `form:"limit"`
}

// TradeAIPredictionSaveDTO oracle 落库一条 AI 预测时传入的数据。
type TradeAIPredictionSaveDTO struct {
	PlatformCode string  `json:"platformCode"`
	Symbol       string  `json:"symbol"`
	CoinCode     string  `json:"coinCode"`
	Interval     string  `json:"interval"`
	PredictTime  int64   `json:"predictTime"` // 预测对应K线时间（unix 秒）
	RefPrice     float64 `json:"refPrice"`    // AI参考开盘价：发起预测时 AI 参考的收盘价
	OpenPrice    float64 `json:"openPrice"`   // 实际开盘价：AI分析完成后即时采集的真实盘价
	CostMs       int64   `json:"costMs"`      // AI分析耗时(毫秒)：从发起到检测完成
	PredictPrice float64 `json:"predictPrice"`
	PredictHigh  float64 `json:"predictHigh"`  // 预测期间最高价
	PredictLow   float64 `json:"predictLow"`   // 预测期间最低价
	Invalidation float64 `json:"invalidation"` // 失效价位：方向被证伪的关键价位(0=未给)
	Trend        string  `json:"trend"`
	Signal       string  `json:"signal"`
	Confidence   float64 `json:"confidence"`
	StopLoss     float64 `json:"stopLoss"`
	TakeProfit   float64 `json:"takeProfit"`
	Reason       string  `json:"reason"`
	RawResponse  string  `json:"rawResponse"`
	Model        string  `json:"model"`
	Provider     string  `json:"provider"`
}

type TradeAnalysisOptionDTO struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type TradeSimulationKlinePointDTO struct {
	Time       string  `json:"time"`
	Timestamp  int64   `json:"timestamp"`
	OpenPrice  float64 `json:"openPrice"`
	HighPrice  float64 `json:"highPrice"`
	LowPrice   float64 `json:"lowPrice"`
	ClosePrice float64 `json:"closePrice"`
	Volume     float64 `json:"volume"`
}

type TradeSimulationAIPointDTO struct {
	Time         string  `json:"time"`        // 预测时间：被预测的那根未来 K 线时间
	Timestamp    int64   `json:"timestamp"`   // 预测时间(unix秒)
	CreatedTime  string  `json:"createdTime"` // 执行时间：本条预测落库(执行)的时间
	Price        float64 `json:"price"`
	PredictHigh  float64 `json:"predictHigh"`  // AI 预测的区间最高价(0=未给)
	PredictLow   float64 `json:"predictLow"`   // AI 预测的区间最低价(0=未给)
	Invalidation float64 `json:"invalidation"` // 失效价位：方向被证伪的关键价位(0=未给)
	Signal       string  `json:"signal"`
	Reason       string  `json:"reason"`
}

type TradeSimulationDiffPointDTO struct {
	Time        string `json:"time"`        // 预测时间：被预测的那根未来 K 线时间
	Timestamp   int64  `json:"timestamp"`   // 预测时间(unix秒)
	CreatedTime string `json:"createdTime"` // 执行时间：本条预测落库(执行)的时间
	// OpenTimestamp 开盘时间(unix秒)：即执行预测那一刻，用作「交易周期」窗口的起点。
	OpenTimestamp int64   `json:"openTimestamp"`
	Trend         string  `json:"trend"`      // AI 预测方向 long/short/neutral
	Confidence    float64 `json:"confidence"` // AI 置信度：方向正确的主观概率 0~1
	RefPrice      float64 `json:"refPrice"`   // AI参考开盘价：发起预测时 AI 看盘的收盘价(预测基准)
	OpenPrice     float64 `json:"openPrice"`  // 实际开盘价：AI分析完成后即时采集的真实盘价
	CostMs        int64   `json:"costMs"`     // AI分析耗时(毫秒)：看盘到检测完成的时间差
	RealPrice     float64 `json:"realPrice"`
	AIPrice       float64 `json:"aiPrice"`
	Diff          float64 `json:"diff"`
	DiffRate      float64 `json:"diffRate"`
	Matched       bool    `json:"matched"`
	// Touched 区间触达：从执行到预测时刻 [createdTime, predictTime] 之间，
	// 真实价格(最高/最低)是否曾覆盖过预测价。止盈/限价语义下表示「实盘能否在此价成交」。
	Touched bool `json:"touched"`
	// WindowHigh / WindowLow 该窗口 [createdTime, predictTime] 内真实价格的最高/最低点。
	WindowHigh float64 `json:"windowHigh"`
	WindowLow  float64 `json:"windowLow"`
	// PredictHigh / PredictLow AI 预测的区间最高/最低价(0=未给)。与 WindowHigh/WindowLow 对照衡量区间预测质量。
	PredictHigh float64 `json:"predictHigh"`
	PredictLow  float64 `json:"predictLow"`
	// Invalidation 失效价位：方向被证伪的关键价位(0=未给)。InvalidationHit 窗口内是否触及失效位 -1未给 0未触发 1已触发。
	Invalidation    float64 `json:"invalidation"`
	InvalidationHit int8    `json:"invalidationHit"`
	// BandContain AI 预测区间是否完整覆盖真实波动 [WindowLow, WindowHigh]。
	BandContain bool `json:"bandContain"`
	// BandUtil 区间利用率=真实波动宽度/预测区间宽度。完整覆盖且利用率≥阈值才算优质命中，否则为「过宽」。
	BandUtil float64 `json:"bandUtil"`
	Label    string  `json:"label"`
	Reason   string  `json:"reason"` // AI 文字理由，供界面悬浮/固定展示
}

// TradeSimulationSeriesDTO 单个预测周期(horizon)的一条预测线及其复核数据。
type TradeSimulationSeriesDTO struct {
	Interval    string                        `json:"interval"`    // 预测周期(horizon)，如 15m/1h/4h/1d
	Label       string                        `json:"label"`       // 显示名，如 "15分钟"
	LastRunTime string                        `json:"lastRunTime"` // 该周期最近一次预测执行时间
	MatchCount  int                           `json:"matchCount"`
	DiffCount   int                           `json:"diffCount"`
	TouchCount  int                           `json:"touchCount"` // 已到期点位中「区间触达」的数量
	AvgDiffRate float64                       `json:"avgDiffRate"`
	MaxDiffRate float64                       `json:"maxDiffRate"`
	AIPoints    []TradeSimulationAIPointDTO   `json:"aiPoints"`
	Markers     []TradeSimulationDiffPointDTO `json:"markers"`
}

type TradeSimulationAnalysisDTO struct {
	PlatformCode    string                         `json:"platformCode"`
	CoinCode        string                         `json:"coinCode"`
	Symbol          string                         `json:"symbol"`
	Interval        string                         `json:"interval"` // K线展示周期
	LastRunTime     string                         `json:"lastRunTime"`
	MatchCount      int                            `json:"matchCount"` // 各周期已到期点位汇总
	DiffCount       int                            `json:"diffCount"`
	TouchCount      int                            `json:"touchCount"` // 各周期已到期点位中「区间触达」的汇总
	AvgDiffRate     float64                        `json:"avgDiffRate"`
	MaxDiffRate     float64                        `json:"maxDiffRate"`
	PlatformOptions []TradeAnalysisOptionDTO       `json:"platformOptions"`
	CoinOptions     []TradeAnalysisOptionDTO       `json:"coinOptions"`
	RealKlines      []TradeSimulationKlinePointDTO `json:"realKlines"`
	// Series 每个预测周期一条预测线（图表叠加展示，图例可单独开关）。
	Series []TradeSimulationSeriesDTO `json:"series"`
}

// TradeStrategyBacktestQueryDTO 策略回测入参：按「方向 + 预测幅度阈值」筛选历史 AI 预测信号，
// 用其后续真实 K 线模拟不同止盈/止损组合的交易结果，输出期望矩阵。
type TradeStrategyBacktestQueryDTO struct {
	PlatformCode  string  `form:"platformCode"`
	CoinCode      string  `form:"coinCode"`
	Interval      string  `form:"interval"`
	Limit         int     `form:"limit"`         // 参与回测的最近 K 线根数上限
	HoldBars      int     `form:"holdBars"`      // 持仓窗口：开仓后向前看几根 K 线（默认 1 = 预测周期）
	MinConfidence float64 `form:"minConfidence"` // 置信度下限 0~1
	MinMovePct    float64 `form:"minMovePct"`    // 预测幅度阈值（百分比，如 3 表示 3%）
	TakerFeeRate  float64 `form:"takerFeeRate"`  // 单边吃单手续费率（百分比，如 0.05 = 0.05%）
	FundingRate   float64 `form:"fundingRate"`   // 每根持仓周期的资金费率（百分比）
	Leverage      float64 `form:"leverage"`      // 杠杆，仅用于把名义收益换算成保证金回报(ROE)展示
	TpList        string  `form:"tpList"`        // 止盈百分比列表，逗号分隔，如 "1,1.5,2,2.5,3"
	SlList        string  `form:"slList"`        // 止损百分比列表，逗号分隔，如 "0.5,1,1.5,2"
}

// TradeStrategyBacktestCellDTO 期望矩阵中一个「止盈×止损」组合的统计结果。
type TradeStrategyBacktestCellDTO struct {
	TakeProfitPct float64 `json:"takeProfitPct"` // 止盈幅度(%)
	StopLossPct   float64 `json:"stopLossPct"`   // 止损幅度(%)
	Samples       int     `json:"samples"`       // 参与样本数
	TpRate        float64 `json:"tpRate"`        // 触止盈占比(%)
	SlRate        float64 `json:"slRate"`        // 触止损占比(%)
	TimeoutRate   float64 `json:"timeoutRate"`   // 到期未触发占比(%)
	WinRate       float64 `json:"winRate"`       // 单笔净收益>0 的占比(%)
	AvgWin        float64 `json:"avgWin"`        // 平均盈利(名义%)
	AvgLoss       float64 `json:"avgLoss"`       // 平均亏损(名义%，正数)
	Payoff        float64 `json:"payoff"`        // 盈亏比 = 平均盈利 / 平均亏损
	Expectancy    float64 `json:"expectancy"`    // 单笔期望(名义%)，扣费后
	ExpectancyRoe float64 `json:"expectancyRoe"` // 单笔期望按杠杆换算的保证金回报(%)
	ProfitFactor  float64 `json:"profitFactor"`  // 盈利因子 = 总盈利 / 总亏损
	TotalReturn   float64 `json:"totalReturn"`   // 累计净收益(名义%，简单加总)
	MaxDrawdown   float64 `json:"maxDrawdown"`   // 最大回撤(名义%)
}

// ─── Strategy Management DTOs ─────────────────────────────────────────────────

type TradeStrategyDTO struct {
	ID               int64   `json:"id"`
	PlatformCode     string  `json:"platformCode"`
	CoinCode         string  `json:"coinCode"`
	Symbol           string  `json:"symbol"`
	Interval         string  `json:"interval"`
	Enabled          int8    `json:"enabled"`
	MinConfidence    float64 `json:"minConfidence"`
	MinMovePct       float64 `json:"minMovePct"`
	TrendFilter      string  `json:"trendFilter"`
	MaxOpenPositions int     `json:"maxOpenPositions"`
	HoldDuration     int     `json:"holdDuration"`
	MaxHoldDuration  int     `json:"maxHoldDuration"`
	TakeProfitPct    float64 `json:"takeProfitPct"`
	StopLossPct      float64 `json:"stopLossPct"`
	// 止盈止损来源(三选一)：percent/predict/pressure，及各来源对应的百分比
	TakeProfitSource   string  `json:"takeProfitSource"`
	StopLossSource     string  `json:"stopLossSource"`
	PredictSLBufferPct float64 `json:"predictSlBufferPct"`
	PressureBufferPct  float64 `json:"pressureBufferPct"`
	TakeProfitFloorPct float64 `json:"takeProfitFloorPct"`
	StopLossFloorPct   float64 `json:"stopLossFloorPct"`
	Leverage           float64 `json:"leverage"`
	Contracts          int     `json:"contracts"`
	MakerFeeRate       float64 `json:"makerFeeRate"`
	TakerFeeRate       float64 `json:"takerFeeRate"`
	// 入场策略(状态机)
	EntryMode         string  `json:"entryMode"`
	EntryAlpha        float64 `json:"entryAlpha"`
	ExitGamma         float64 `json:"exitGamma"`
	EntryTTL          int     `json:"entryTtl"`
	EfficiencyRoute   float64 `json:"efficiencyRoute"`
	PredictionVariant string  `json:"predictionVariant"`
	Remark            string  `json:"remark"`
	CreatedTime       string  `json:"createdTime"`
	UpdatedTime       string  `json:"updatedTime"`
}

type TradeStrategyListDTO struct {
	Total int64              `json:"total"`
	List  []TradeStrategyDTO `json:"list"`
}

type CreateTradeStrategyDTO struct {
	PlatformCode     string  `json:"platformCode" binding:"required"`
	CoinCode         string  `json:"coinCode" binding:"required"`
	Symbol           string  `json:"symbol" binding:"required"`
	Interval         string  `json:"interval" binding:"required"`
	Enabled          *int8   `json:"enabled"`
	MinConfidence    float64 `json:"minConfidence"`
	MinMovePct       float64 `json:"minMovePct"`
	TrendFilter      string  `json:"trendFilter"`
	MaxOpenPositions int     `json:"maxOpenPositions"`
	HoldDuration     string  `json:"holdDuration"`    // "4h"/"15m"/seconds
	MaxHoldDuration  string  `json:"maxHoldDuration"` // "24h"/seconds
	TakeProfitPct    float64 `json:"takeProfitPct"`
	StopLossPct      float64 `json:"stopLossPct"`
	// 止盈止损来源(三选一)：percent/predict/pressure，及各来源对应的百分比
	TakeProfitSource   string  `json:"takeProfitSource"`
	StopLossSource     string  `json:"stopLossSource"`
	PredictSLBufferPct float64 `json:"predictSlBufferPct"` // predict止损：失效价缓冲%
	PressureBufferPct  float64 `json:"pressureBufferPct"`  // pressure止盈/止损缓冲%
	TakeProfitFloorPct float64 `json:"takeProfitFloorPct"` // 兜底锁盈%，0=不约束
	StopLossFloorPct   float64 `json:"stopLossFloorPct"`   // 兜底最小止损%，0=不约束
	Leverage           float64 `json:"leverage"`
	Contracts          int     `json:"contracts"`
	MakerFeeRate       float64 `json:"makerFeeRate"`
	TakerFeeRate       float64 `json:"takerFeeRate"`
	// 入场策略(状态机)
	EntryMode         string  `json:"entryMode"`         // market/pullback
	EntryAlpha        float64 `json:"entryAlpha"`        // 入场分位 0~1
	ExitGamma         float64 `json:"exitGamma"`         // 止盈分位 0~1
	EntryTTL          int     `json:"entryTtl"`          // 挂单有效期(秒)
	EfficiencyRoute   float64 `json:"efficiencyRoute"`   // 效率路由阈值，0=不路由
	PredictionVariant string  `json:"predictionVariant"` // raw/calibrated
	Remark            string  `json:"remark"`
}

type UpdateTradeStrategyDTO struct {
	Enabled            *int8    `json:"enabled"`
	MinConfidence      *float64 `json:"minConfidence"`
	MinMovePct         *float64 `json:"minMovePct"`
	TrendFilter        *string  `json:"trendFilter"`
	MaxOpenPositions   *int     `json:"maxOpenPositions"`
	HoldDuration       *string  `json:"holdDuration"`
	MaxHoldDuration    *string  `json:"maxHoldDuration"`
	TakeProfitPct      *float64 `json:"takeProfitPct"`
	StopLossPct        *float64 `json:"stopLossPct"`
	TakeProfitSource   *string  `json:"takeProfitSource"`
	StopLossSource     *string  `json:"stopLossSource"`
	PredictSLBufferPct *float64 `json:"predictSlBufferPct"`
	PressureBufferPct  *float64 `json:"pressureBufferPct"`
	TakeProfitFloorPct *float64 `json:"takeProfitFloorPct"`
	StopLossFloorPct   *float64 `json:"stopLossFloorPct"`
	Leverage           *float64 `json:"leverage"`
	Contracts          *int     `json:"contracts"`
	MakerFeeRate       *float64 `json:"makerFeeRate"`
	TakerFeeRate       *float64 `json:"takerFeeRate"`
	// 入场策略(状态机)
	EntryMode         *string  `json:"entryMode"`
	EntryAlpha        *float64 `json:"entryAlpha"`
	ExitGamma         *float64 `json:"exitGamma"`
	EntryTTL          *int     `json:"entryTtl"`
	EfficiencyRoute   *float64 `json:"efficiencyRoute"`
	PredictionVariant *string  `json:"predictionVariant"`
	Remark            *string  `json:"remark"`
}

type TradeStrategyQueryDTO struct {
	Page         int    `form:"page"`
	PageSize     int    `form:"pageSize"`
	PlatformCode string `form:"platformCode"`
	CoinCode     string `form:"coinCode"`
	Symbol       string `form:"symbol"`
	Interval     string `form:"interval"`
	Enabled      string `form:"enabled"` // "0"/"1"/"" for all
}

// ─── Position Management DTOs ─────────────────────────────────────────────────

type TradeStrategyPositionDTO struct {
	ID                 int64   `json:"id"`
	StrategyID         int64   `json:"strategyId"`
	PredictionID       int64   `json:"predictionId"`
	PlatformCode       string  `json:"platformCode"`
	CoinCode           string  `json:"coinCode"`
	Symbol             string  `json:"symbol"`
	Interval           string  `json:"interval"`
	Direction          string  `json:"direction"`
	OpenPrice          float64 `json:"openPrice"`
	TakeProfitPrice    float64 `json:"takeProfitPrice"`
	StopLossPrice      float64 `json:"stopLossPrice"`
	Contracts          int     `json:"contracts"`
	Leverage           float64 `json:"leverage"`
	OpenedAt           string  `json:"openedAt"`
	HoldUntil          string  `json:"holdUntil"`
	Status             string  `json:"status"`
	ClosePrice         float64 `json:"closePrice"`
	CloseReason        string  `json:"closeReason"`
	ClosedAt           string  `json:"closedAt"`
	Pnl                float64 `json:"pnl"`
	PnlRate            float64 `json:"pnlRate"`
	Fee                float64 `json:"fee"`
	NetPnl             float64 `json:"netPnl"`
	Confidence         float64 `json:"confidence"`
	PredictedMovePct   float64 `json:"predictedMovePct"`
	MaxPriceDuringHold float64 `json:"maxPriceDuringHold"`
	MinPriceDuringHold float64 `json:"minPriceDuringHold"`
	CreatedTime        string  `json:"createdTime"`
}

type TradeStrategyPositionListDTO struct {
	Total int64                      `json:"total"`
	List  []TradeStrategyPositionDTO `json:"list"`
}

type TradeStrategyPositionQueryDTO struct {
	Page       int    `form:"page"`
	PageSize   int    `form:"pageSize"`
	StrategyID int64  `form:"strategyId"`
	Symbol     string `form:"symbol"`
	Status     string `form:"status"`
	StartTime  int64  `form:"startTime"` // unix seconds
	EndTime    int64  `form:"endTime"`
}

type TradeStrategyPositionSummaryQueryDTO struct {
	StrategyID int64  `form:"strategyId"`
	Symbol     string `form:"symbol"`
	StartTime  int64  `form:"startTime"`
	EndTime    int64  `form:"endTime"`
}

type TradeStrategyPositionSummaryDTO struct {
	TotalOpens       int64   `json:"totalOpens"`
	CurrentOpen      int64   `json:"currentOpen"`
	TotalClosed      int64   `json:"totalClosed"`
	CumulativeNetPnl float64 `json:"cumulativeNetPnl"`
	WinRate          float64 `json:"winRate"`        // % of closed positions with net_pnl > 0
	AvgHoldSeconds   float64 `json:"avgHoldSeconds"` // avg hold duration of closed positions
	MaxWin           float64 `json:"maxWin"`
	MaxLoss          float64 `json:"maxLoss"`
	TpCount          int64   `json:"tpCount"`
	SlCount          int64   `json:"slCount"`
	TimeoutCount     int64   `json:"timeoutCount"`
	ManualCount      int64   `json:"manualCount"`
	TpRate           float64 `json:"tpRate"`
	SlRate           float64 `json:"slRate"`
	TimeoutRate      float64 `json:"timeoutRate"`
	ManualRate       float64 `json:"manualRate"`
}

// TradeStrategyBacktestDTO 策略回测输出。
type TradeStrategyBacktestDTO struct {
	PlatformCode      string                         `json:"platformCode"`
	CoinCode          string                         `json:"coinCode"`
	Symbol            string                         `json:"symbol"`
	Interval          string                         `json:"interval"`
	HoldBars          int                            `json:"holdBars"`
	MinConfidence     float64                        `json:"minConfidence"`
	MinMovePct        float64                        `json:"minMovePct"`
	TakerFeeRate      float64                        `json:"takerFeeRate"`
	FundingRate       float64                        `json:"fundingRate"`
	Leverage          float64                        `json:"leverage"`
	CostPerTrade      float64                        `json:"costPerTrade"`      // 单笔总成本(名义%) = 2×手续费 + 资金费率×持仓根数
	RangeStart        string                         `json:"rangeStart"`        // 样本起始时间
	RangeEnd          string                         `json:"rangeEnd"`          // 样本结束时间
	TotalPredictions  int                            `json:"totalPredictions"`  // 区间内预测总数
	QualifiedSignals  int                            `json:"qualifiedSignals"`  // 满足开仓条件的信号数
	DirectionAccuracy float64                        `json:"directionAccuracy"` // 合格信号的方向正确率(%)，以持仓窗口末收盘价判定
	AvgPredictMovePct float64                        `json:"avgPredictMovePct"` // 合格信号的平均预测幅度(%)
	TpPercents        []float64                      `json:"tpPercents"`
	SlPercents        []float64                      `json:"slPercents"`
	Cells             []TradeStrategyBacktestCellDTO `json:"cells"`
	Best              *TradeStrategyBacktestCellDTO  `json:"best"` // 期望最高的组合
	PlatformOptions   []TradeAnalysisOptionDTO       `json:"platformOptions"`
	CoinOptions       []TradeAnalysisOptionDTO       `json:"coinOptions"`
}

// ─── Backtest DTOs (策略回测层) ────────────────────────────────────────────────

// CreateBacktestRunDTO 新建一次回测任务的请求体。时间支持 RFC3339 或 "2006-01-02 15:04:05"。
type CreateBacktestRunDTO struct {
	Name               string `json:"name"`
	PlatformCode       string `json:"platformCode" binding:"required"`
	CoinCode           string `json:"coinCode" binding:"required"`
	Symbol             string `json:"symbol" binding:"required"`
	PredictionInterval string `json:"predictionInterval" binding:"required"`
	PredictionVariant  string `json:"predictionVariant"`
	PriceInterval      string `json:"priceInterval"`
	PriceSource        string `json:"priceSource"`
	TradingPeriod      string `json:"tradingPeriod"` // 可选 1h/4h/8h/12h/1d；空=仅按预测周期(现状)
	StartTime          string `json:"startTime" binding:"required"`
	EndTime            string `json:"endTime" binding:"required"`
	StrategyID         int64  `json:"strategyId" binding:"required"`
}

// BacktestRunQueryDTO 回测任务列表查询。
type BacktestRunQueryDTO struct {
	Page       int    `form:"page"`
	PageSize   int    `form:"pageSize"`
	Symbol     string `form:"symbol"`
	StrategyID int64  `form:"strategyId"`
}

type BacktestRunDTO struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	PlatformCode       string `json:"platformCode"`
	CoinCode           string `json:"coinCode"`
	Symbol             string `json:"symbol"`
	PredictionInterval string `json:"predictionInterval"`
	PredictionVariant  string `json:"predictionVariant"`
	PriceInterval      string `json:"priceInterval"`
	PriceSource        string `json:"priceSource"`
	TradingPeriod      string `json:"tradingPeriod"`
	StartTime          string `json:"startTime"`
	EndTime            string `json:"endTime"`
	StrategyID         int64  `json:"strategyId"`
	ParamsSnapshot     string `json:"paramsSnapshot"`
	Status             string `json:"status"`
	ErrorMsg           string `json:"errorMsg"`
	CreatedTime        string `json:"createdTime"`
	KlineCount         int    `json:"klineCount"` // 回放使用的K线根数
	KlineStart         string `json:"klineStart"` // 实际K线起始时间
	KlineEnd           string `json:"klineEnd"`   // 实际K线结束时间
}

type BacktestRunListDTO struct {
	Total int64            `json:"total"`
	List  []BacktestRunDTO `json:"list"`
}

type BacktestTradeDTO struct {
	ID                 int64   `json:"id"`
	PredictionID       int64   `json:"predictionId"`
	CalcMode           string  `json:"calcMode"`
	PredictTime        string  `json:"predictTime"` // 预测目标时刻(关联预测 predict_time)，与 requestedAt 框定预测周期
	Direction          string  `json:"direction"`
	EntryMode          string  `json:"entryMode"`
	PlannedEntryPrice  float64 `json:"plannedEntryPrice"`
	TakeProfitPrice    float64 `json:"takeProfitPrice"`
	StopLossPrice      float64 `json:"stopLossPrice"`
	Status             string  `json:"status"`
	OpenPrice          float64 `json:"openPrice"`
	ClosePrice         float64 `json:"closePrice"`
	CloseReason        string  `json:"closeReason"`
	RequestedAt        string  `json:"requestedAt"`
	OpenedAt           string  `json:"openedAt"`
	ClosedAt           string  `json:"closedAt"`
	Pnl                float64 `json:"pnl"`
	PnlRate            float64 `json:"pnlRate"` // 盈亏率%(含杠杆) = 价差/开仓价×杠杆×100
	NetPnl             float64 `json:"netPnl"`
	Fee                float64 `json:"fee"`
	Confidence         float64 `json:"confidence"`
	Efficiency         float64 `json:"efficiency"`
	PredHigh           float64 `json:"predHigh"`           // 预测区间上沿
	PredLow            float64 `json:"predLow"`            // 预测区间下沿
	PredClose          float64 `json:"predClose"`          // 预测收盘价
	WindowOpen         float64 `json:"windowOpen"`         // 信号后窗口实际开盘价
	WindowClose        float64 `json:"windowClose"`        // 信号后窗口实际收盘价
	WindowLow          float64 `json:"windowLow"`          // 信号后窗口最低价
	WindowHigh         float64 `json:"windowHigh"`         // 信号后窗口最高价
	PressureHigh       float64 `json:"pressureHigh"`       // 压力面最高价(关键阻力)
	PressureLow        float64 `json:"pressureLow"`        // 压力面最低价(关键支撑)
	MaxPriceDuringHold float64 `json:"maxPriceDuringHold"` // 持仓期间最高价(算最高浮盈用)
	MinPriceDuringHold float64 `json:"minPriceDuringHold"` // 持仓期间最低价
	Leverage           float64 `json:"leverage"`           // 杠杆倍数(算含杠杆浮盈用)
	// 持仓中(status=open)按当前最新价标记的浮动盈亏：回测窗口未走完生命周期的持仓，用最新一根 K 线收盘价标记。
	MarkPrice         float64 `json:"markPrice"`         // 当前最新价(标记价)，仅持仓中有值
	UnrealizedPnl     float64 `json:"unrealizedPnl"`     // 浮动毛盈亏 USDT
	UnrealizedPnlRate float64 `json:"unrealizedPnlRate"` // 浮动盈亏率%(含杠杆)
	UnrealizedNetPnl  float64 `json:"unrealizedNetPnl"`  // 浮动净盈亏 = 浮动盈亏 - 预估手续费
}

type BacktestMetricDTO struct {
	RunID        int64   `json:"runId"`
	CalcMode     string  `json:"calcMode"`
	TradeCount   int     `json:"tradeCount"`
	FillCount    int     `json:"fillCount"`
	ExpiredCount int     `json:"expiredCount"`
	FillRate     float64 `json:"fillRate"`
	WinCount     int     `json:"winCount"`
	WinRate      float64 `json:"winRate"`
	GrossPnl     float64 `json:"grossPnl"`
	FeeTotal     float64 `json:"feeTotal"`
	NetPnl       float64 `json:"netPnl"`
	Expectancy   float64 `json:"expectancy"`
	ProfitFactor float64 `json:"profitFactor"`
	MaxDrawdown  float64 `json:"maxDrawdown"`
	Sharpe       float64 `json:"sharpe"`
	AvgHoldSecs  float64 `json:"avgHoldSecs"`
	TpCount      int     `json:"tpCount"`
	SlCount      int     `json:"slCount"`
	TimeoutCount int     `json:"timeoutCount"`
}

// BacktestRunDetailDTO 单次回测详情：任务 + 汇总指标(可能两种口径) + 逐笔。
type BacktestRunDetailDTO struct {
	Run     BacktestRunDTO      `json:"run"`
	Metrics []BacktestMetricDTO `json:"metrics"` // 按 calcMode 区分：prediction / trading
	Trades  []BacktestTradeDTO  `json:"trades"`
}

// KlinePointDTO 单根 K 线(供回测逐笔的“K线详情”弹窗展示)。
type KlinePointDTO struct {
	Time   string  `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// KlineRangeQueryDTO 按 symbol+interval+时间区间拉取 K 线。
type KlineRangeQueryDTO struct {
	Symbol   string `form:"symbol"`
	Interval string `form:"interval"`
	Start    string `form:"start"`
	End      string `form:"end"`
}

// ─── 回测「K线详情」预测增强：复合方向 + 预测周期 K 线 ───────────────────────────

// PredictionCandleDTO 一根「预测 K 线」：由一条 AI 预测构造(开=参考价 收=预测价 高/低=预测极值)。
type PredictionCandleDTO struct {
	OpenTime   string  `json:"openTime"`   // 发起时刻(=该预测周期开盘)
	CloseTime  string  `json:"closeTime"`  // 预测目标时刻(=该预测周期收盘)
	Open       float64 `json:"open"`       // ref_price 参考开盘
	High       float64 `json:"high"`       // predict_high 预测最高
	Low        float64 `json:"low"`        // predict_low 预测最低
	Close      float64 `json:"close"`      // predict_price 预测收盘
	Trend      string  `json:"trend"`      // long/short/neutral
	Confidence float64 `json:"confidence"` // 0~1
}

// PredictionSeriesDTO 某周期的一串预测 K 线。
type PredictionSeriesDTO struct {
	Interval string                `json:"interval"`
	Candles  []PredictionCandleDTO `json:"candles"`
}

// CompositeRowDTO 复合方向里某高周期一行：方向 + 置信度 + 据预测极值估的利润潜力 + 加权得分。
type CompositeRowDTO struct {
	Interval         string  `json:"interval"`
	Direction        string  `json:"direction"`
	Confidence       float64 `json:"confidence"`
	PredLow          float64 `json:"predLow"`          // 预测区间最低
	PredHigh         float64 `json:"predHigh"`         // 预测区间最高
	FavorableExtreme float64 `json:"favorableExtreme"` // 有利极值：多看预测高、空看预测低
	ProfitPct        float64 `json:"profitPct"`        // (有利极值相对入场价的有利空间%)
	Score            float64 `json:"score"`            // profitPct × confidence
	Dominant         bool    `json:"dominant"`         // 是否为胜出周期
	HasData          bool    `json:"hasData"`          // T 之前是否有该周期预测
	PredictTime      string  `json:"predictTime"`      // 锚定预测的目标时刻
}

// CompositeDirectionDTO 复合方向汇总：对各高周期算利润×置信度，胜出者定最终方向。
type CompositeDirectionDTO struct {
	Entry                float64           `json:"entry"`
	OwnInterval          string            `json:"ownInterval"`
	OwnDirection         string            `json:"ownDirection"`
	RecommendedDirection string            `json:"recommendedDirection"`
	DominantInterval     string            `json:"dominantInterval"`
	Agree                bool              `json:"agree"` // 复合方向是否与本笔自身方向一致
	Rows                 []CompositeRowDTO `json:"rows"`
}

// PredictionDetailDTO 回测「K线详情」的预测增强数据：复合方向 + 自身周期预测K线 + 高周期预测K线。
type PredictionDetailDTO struct {
	Composite    CompositeDirectionDTO `json:"composite"`
	OwnSeries    PredictionSeriesDTO   `json:"ownSeries"`
	HigherSeries []PredictionSeriesDTO `json:"higherSeries"`
}

// PredictionDetailQueryDTO 「K线详情」预测增强查询入参。
type PredictionDetailQueryDTO struct {
	Platform string `form:"platform"`
	Coin     string `form:"coin"`
	Interval string `form:"interval"` // 本笔预测周期(自身)
	Signal   string `form:"signal"`   // T = 本笔开仓/信号时刻(锚定高周期)
	Start    string `form:"start"`    // 窗口起(=开仓)
	End      string `form:"end"`      // 窗口止(=平仓/交易周期末)
	Entry    string `form:"entry"`    // 入场价基准(成交价/期望价)
}

// BackfillKlineDTO 触发 K 线回填的入参：拉某币种某周期“最近 limit 根”入库。
type BackfillKlineDTO struct {
	PlatformCode string `json:"platformCode"` // 交易所，默认 binance
	Symbol       string `json:"symbol" binding:"required"`
	Interval     string `json:"interval" binding:"required"` // 1m/5m/15m/1h/4h/1d ...
	Limit        int    `json:"limit" binding:"required"`    // 想要的最近根数
}

// BackfillKlineResultDTO K 线回填结果：把“现有→需补→实拉→入库”的链路透明化。
type BackfillKlineResultDTO struct {
	Symbol       string `json:"symbol"`
	Interval     string `json:"interval"`
	Requested    int    `json:"requested"`    // 请求的最近根数
	LatestBefore string `json:"latestBefore"` // 回填前 DB 最新一根 open_time(空=原本无数据)
	NeedFetch    int    `json:"needFetch"`    // 据最新一条推算出需向交易所拉取的根数
	Fetched      int    `json:"fetched"`      // 实际从交易所拉到的根数
	Upserted     int64  `json:"upserted"`     // 幂等入库影响行数
	LatestAfter  string `json:"latestAfter"`  // 回填后 DB 最新一根 open_time
}
