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
	RefPrice     float64 `json:"refPrice"`
	PredictPrice float64 `json:"predictPrice"`
	PredictHigh  float64 `json:"predictHigh"` // 预测期间最高价
	PredictLow   float64 `json:"predictLow"`  // 预测期间最低价
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
	Time        string  `json:"time"`        // 预测时间：被预测的那根未来 K 线时间
	Timestamp   int64   `json:"timestamp"`   // 预测时间(unix秒)
	CreatedTime string  `json:"createdTime"` // 执行时间：本条预测落库(执行)的时间
	Price        float64 `json:"price"`
	PredictHigh  float64 `json:"predictHigh"`  // AI 预测的区间最高价(0=未给)
	PredictLow   float64 `json:"predictLow"`   // AI 预测的区间最低价(0=未给)
	Invalidation float64 `json:"invalidation"` // 失效价位：方向被证伪的关键价位(0=未给)
	Signal       string  `json:"signal"`
	Reason       string  `json:"reason"`
}

type TradeSimulationDiffPointDTO struct {
	Time        string  `json:"time"`        // 预测时间：被预测的那根未来 K 线时间
	Timestamp   int64   `json:"timestamp"`   // 预测时间(unix秒)
	CreatedTime string  `json:"createdTime"` // 执行时间：本条预测落库(执行)的时间
	Trend       string  `json:"trend"`       // AI 预测方向 long/short/neutral
	RefPrice    float64 `json:"refPrice"`    // 执行时的真实盘价格(最近收盘价)，本次预测的基准价
	RealPrice   float64 `json:"realPrice"`
	AIPrice     float64 `json:"aiPrice"`
	Diff        float64 `json:"diff"`
	DiffRate    float64 `json:"diffRate"`
	Matched     bool    `json:"matched"`
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
	Label       string `json:"label"`
	Reason     string  `json:"reason"` // AI 文字理由，供界面悬浮/固定展示
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
	PlatformCode  string `form:"platformCode"`
	CoinCode      string `form:"coinCode"`
	Interval      string `form:"interval"`
	Limit         int    `form:"limit"`         // 参与回测的最近 K 线根数上限
	HoldBars      int    `form:"holdBars"`      // 持仓窗口：开仓后向前看几根 K 线（默认 1 = 预测周期）
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
