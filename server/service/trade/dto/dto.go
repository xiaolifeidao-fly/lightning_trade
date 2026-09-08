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
	PlatformCode string    `json:"platformCode"`
	Symbol       string    `json:"symbol"`
	Interval     string    `json:"interval"`
	OpenTime     time.Time `json:"openTime"`
	CloseTime    time.Time `json:"closeTime"`
	OpenPrice    float64   `json:"openPrice"`
	HighPrice    float64   `json:"highPrice"`
	LowPrice     float64   `json:"lowPrice"`
	ClosePrice   float64   `json:"closePrice"`
	Volume       float64   `json:"volume"`
	Turnover     float64   `json:"turnover"`
	TradeCount   uint64    `json:"tradeCount"`
}

type TradeKlineQueryDTO struct {
	PlatformCode string `form:"platformCode"` // 行情平台 binance/deepcoin，空则按 binance
	Symbol       string `form:"symbol"`
	Interval     string `form:"interval"`
	Limit        int    `form:"limit"`
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
	ID                  int64   `json:"id"`
	PlatformCode        string  `json:"platformCode"`
	CoinCode            string  `json:"coinCode"`
	Symbol              string  `json:"symbol"`
	Interval            string  `json:"interval"`
	Enabled             int8    `json:"enabled"`
	MinConfidence       float64 `json:"minConfidence"`
	MinMovePct          float64 `json:"minMovePct"`
	TrendFilter         string  `json:"trendFilter"`
	MaxOpenPositions    int     `json:"maxOpenPositions"`
	RequireCompositeDir int8    `json:"requireCompositeDir"` // 复合方向门槛 1启用 0不启用
	HoldDuration        int     `json:"holdDuration"`
	MaxHoldDuration     int     `json:"maxHoldDuration"`
	TradingPeriod       string  `json:"tradingPeriod"` // 交易周期 1h/4h/12h/1d/1w，空=不启用
	TakeProfitPct       float64 `json:"takeProfitPct"`
	StopLossPct         float64 `json:"stopLossPct"`
	// 止盈止损来源(三选一)：percent/predict/pressure，及各来源对应的百分比
	TakeProfitSource   string  `json:"takeProfitSource"`
	StopLossSource     string  `json:"stopLossSource"`
	PredictSLBufferPct float64 `json:"predictSlBufferPct"`
	PressureBufferPct  float64 `json:"pressureBufferPct"`
	TakeProfitFloorPct float64 `json:"takeProfitFloorPct"`
	StopLossFloorPct   float64 `json:"stopLossFloorPct"`
	// 移动止盈(峰值回撤+时间收敛)：0=不启用，退回静态止盈
	TrailActivatePct float64 `json:"trailActivatePct"`
	TrailGiveback    float64 `json:"trailGiveback"`
	TrailGivebackMin float64 `json:"trailGivebackMin"`
	// 早段疲软离场
	EarlyCutTimePct      float64 `json:"earlyCutTimePct"`
	EarlyCutMinProfitPct float64 `json:"earlyCutMinProfitPct"`
	// 早段逆行离场(MAE 软止损)
	EarlyCutMaxAdversePct float64 `json:"earlyCutMaxAdversePct"`
	EarlyCutArmProfitPct  float64 `json:"earlyCutArmProfitPct"`
	Leverage              float64 `json:"leverage"`
	Contracts             int     `json:"contracts"`
	MakerFeeRate          float64 `json:"makerFeeRate"`
	TakerFeeRate          float64 `json:"takerFeeRate"`
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
	PlatformCode        string  `json:"platformCode" binding:"required"`
	CoinCode            string  `json:"coinCode" binding:"required"`
	Symbol              string  `json:"symbol" binding:"required"`
	Interval            string  `json:"interval" binding:"required"`
	Enabled             *int8   `json:"enabled"`
	MinConfidence       float64 `json:"minConfidence"`
	MinMovePct          float64 `json:"minMovePct"`
	TrendFilter         string  `json:"trendFilter"`
	MaxOpenPositions    int     `json:"maxOpenPositions"`
	RequireCompositeDir *int8   `json:"requireCompositeDir"` // 复合方向门槛 1启用 0不启用
	HoldDuration        string  `json:"holdDuration"`        // "4h"/"15m"/seconds
	MaxHoldDuration     string  `json:"maxHoldDuration"`     // "24h"/seconds
	TradingPeriod       string  `json:"tradingPeriod"`       // 交易周期 1h/4h/12h/1d/1w，空=不启用
	TakeProfitPct       float64 `json:"takeProfitPct"`
	StopLossPct         float64 `json:"stopLossPct"`
	// 止盈止损来源(三选一)：percent/predict/pressure，及各来源对应的百分比
	TakeProfitSource   string  `json:"takeProfitSource"`
	StopLossSource     string  `json:"stopLossSource"`
	PredictSLBufferPct float64 `json:"predictSlBufferPct"` // predict止损：失效价缓冲%
	PressureBufferPct  float64 `json:"pressureBufferPct"`  // pressure止盈/止损缓冲%
	TakeProfitFloorPct float64 `json:"takeProfitFloorPct"` // 兜底锁盈%，0=不约束
	StopLossFloorPct   float64 `json:"stopLossFloorPct"`   // 兜底最小止损%，0=不约束
	// 移动止盈(峰值回撤+时间收敛)：0=不启用
	TrailActivatePct float64 `json:"trailActivatePct"` // 激活阈值：浮盈ROI%(含杠杆)
	TrailGiveback    float64 `json:"trailGiveback"`    // 峰值回撤比例r0(0~1)
	TrailGivebackMin float64 `json:"trailGivebackMin"` // 周期末回撤比例(时间收敛)，<=0或≥r0=不收敛
	// 早段疲软离场：前X%时间内峰值浮盈<Y%(含杠杆)则市价平仓。0=不启用。
	EarlyCutTimePct      float64 `json:"earlyCutTimePct"`      // 触发时间点%(0~100)
	EarlyCutMinProfitPct float64 `json:"earlyCutMinProfitPct"` // 利润门槛ROI%(含杠杆)
	// 早段逆行离场(MAE 软止损)：从未走出浮盈且逆行浮亏达阈值时先于硬止损离场。0=不启用。
	EarlyCutMaxAdversePct float64 `json:"earlyCutMaxAdversePct"` // 逆行止损阈值ROI%(含杠杆)
	EarlyCutArmProfitPct  float64 `json:"earlyCutArmProfitPct"`  // 解除阈值ROI%(含杠杆)，<=0=始终武装
	Leverage              float64 `json:"leverage"`
	Contracts             int     `json:"contracts"`
	MakerFeeRate          float64 `json:"makerFeeRate"`
	TakerFeeRate          float64 `json:"takerFeeRate"`
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
	Enabled               *int8    `json:"enabled"`
	MinConfidence         *float64 `json:"minConfidence"`
	MinMovePct            *float64 `json:"minMovePct"`
	TrendFilter           *string  `json:"trendFilter"`
	MaxOpenPositions      *int     `json:"maxOpenPositions"`
	RequireCompositeDir   *int8    `json:"requireCompositeDir"`
	HoldDuration          *string  `json:"holdDuration"`
	MaxHoldDuration       *string  `json:"maxHoldDuration"`
	TradingPeriod         *string  `json:"tradingPeriod"`
	TakeProfitPct         *float64 `json:"takeProfitPct"`
	StopLossPct           *float64 `json:"stopLossPct"`
	TakeProfitSource      *string  `json:"takeProfitSource"`
	StopLossSource        *string  `json:"stopLossSource"`
	PredictSLBufferPct    *float64 `json:"predictSlBufferPct"`
	PressureBufferPct     *float64 `json:"pressureBufferPct"`
	TakeProfitFloorPct    *float64 `json:"takeProfitFloorPct"`
	StopLossFloorPct      *float64 `json:"stopLossFloorPct"`
	TrailActivatePct      *float64 `json:"trailActivatePct"`
	TrailGiveback         *float64 `json:"trailGiveback"`
	TrailGivebackMin      *float64 `json:"trailGivebackMin"`
	EarlyCutTimePct       *float64 `json:"earlyCutTimePct"`
	EarlyCutMinProfitPct  *float64 `json:"earlyCutMinProfitPct"`
	EarlyCutMaxAdversePct *float64 `json:"earlyCutMaxAdversePct"`
	EarlyCutArmProfitPct  *float64 `json:"earlyCutArmProfitPct"`
	Leverage              *float64 `json:"leverage"`
	Contracts             *int     `json:"contracts"`
	MakerFeeRate          *float64 `json:"makerFeeRate"`
	TakerFeeRate          *float64 `json:"takerFeeRate"`
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
	Page     int    `form:"page"`
	PageSize int    `form:"pageSize"`
	Symbol   string `form:"symbol"`
	// EngineKind 引擎类型 prediction/signal；空=不限（既有页面不传，行为不变）。
	EngineKind string `form:"engineKind"`
	StrategyID int64  `form:"strategyId"`
}

// CreateSignalBacktestRunDTO 新建一次盘口信号回测。
//
// 与预测驱动的 CreateBacktestRunDTO 分开，因为两者的必填项完全不同：
// 信号驱动不需要 strategyId / predictionInterval，但**必须**有
// instanceKey + accountLabel——三个部署实例写同一张事件表，且实例1 有两个
// 账户（champion/challenger），少任一维度都会把不同实验体的触发流混成一条。
//
// 参数旋钮全部用指针：区分"没传（回落生产缺省）"与"显式传了 0"。
// 字段后的括号是对应的生产配置键，回测与实盘共享同一套参数定义。
type CreateSignalBacktestRunDTO struct {
	Name         string `json:"name"`
	PlatformCode string `json:"platformCode"` // 1m 路径回放的行情平台，缺省 deepcoin
	CoinCode     string `json:"coinCode"`
	Symbol       string `json:"symbol"` // 缺省 BTCUSDT
	StartTime    string `json:"startTime" binding:"required"`
	EndTime      string `json:"endTime" binding:"required"`
	InstanceKey  string `json:"instanceKey" binding:"required"`  // argus_instance.instance_key
	AccountLabel string `json:"accountLabel" binding:"required"` // strategy_event.account_label

	// 参数旋钮内联（匿名嵌入，JSON 键与嵌入前完全一致）。批量扫描的每一组
	// 复用同一个结构，保证"批量里的某组单独重跑"参数口径一致。
	SignalBacktestParamsDTO
}

// SignalBacktestParamsDTO 一组盘口信号回测的参数旋钮。
//
// 单次回测（CreateSignalBacktestRunDTO）与批量扫描的每一组
// （SignalBacktestGroupDTO）共用它：两处若各写一份字段列表，加旋钮时必然漏改
// 一边，于是"同一组参数单跑和批量跑结果不同"。
//
// 全部用指针：区分"没传（回落基线/生产缺省）"与"显式传了 0"。
// 字段后的括号是对应的生产配置键，回测与实盘共享同一套参数定义。
type SignalBacktestParamsDTO struct {
	Mode     string `json:"mode"`     // net（实盘形态）/ dual（研究对照）
	EvalMode string `json:"evalMode"` // close / pessimistic
	EntryPx  string `json:"entryPx"`  // bar_close / sig_last

	OrderSize                  *int     `json:"orderSize"`                  // trade.accountN.order_size
	RiskEquity                 *float64 `json:"riskEquity"`                 // trade.accountN.risk_equity
	CapOverride                *int     `json:"capOverride"`                // >0 固定上限（金标准口径），0=走 cap 公式
	BudgetPct                  *float64 `json:"budgetPct"`                  // position.risk.budget_pct
	CatastropheStopPct         *float64 `json:"catastropheStopPct"`         // position.monitor.catastrophe_stop_pct
	Ceiling                    *int     `json:"ceiling"`                    // position.risk.max_contracts_ceiling
	CatastropheOvershootRoiPts *float64 `json:"catastropheOvershootRoiPts"` // 兜底成交过冲 ROI 点
	GateMinProfitPct           *float64 `json:"gateMinProfitPct"`           // trade.accountN.reverse_gate_min_profit_pct
	TrendGateWindowHours       *float64 `json:"trendGateWindowHours"`       // trade.trend_gate.window_hours
	TrendGateThresholdPct      *float64 `json:"trendGateThresholdPct"`      // trade.trend_gate.threshold_pct
	TierSmallRatio             *float64 `json:"tierSmallRatio"`             // position.monitor.trail.tier_small_ratio
	TierLargeRatio             *float64 `json:"tierLargeRatio"`             // position.monitor.trail.tier_large_ratio
	SmallActivatePct           *float64 `json:"smallActivatePct"`           // position.monitor.trail.small_activate
	SmallGiveback              *float64 `json:"smallGiveback"`              // position.monitor.trail.small_giveback
	MediumActivatePct          *float64 `json:"mediumActivatePct"`
	MediumGiveback             *float64 `json:"mediumGiveback"`
	LargeActivatePct           *float64 `json:"largeActivatePct"`
	LargeGiveback              *float64 `json:"largeGiveback"`
	TakerFee                   *float64 `json:"takerFee"`
	// SignalThresholdBp / BaselineThresholdBp 只要不相等，本组精度自动降为频率级。
	SignalThresholdBp   *float64 `json:"signalThresholdBp"`
	BaselineThresholdBp *float64 `json:"baselineThresholdBp"`
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
	// 盘口信号回测（engineKind=signal）专用
	EngineKind   string `json:"engineKind"`
	InstanceKey  string `json:"instanceKey"`
	AccountLabel string `json:"accountLabel"`
	SignalSource string `json:"signalSource"`
	Fidelity     string `json:"fidelity"`     // event / frequency，前端按它分组，不得混排
	FidelityNote string `json:"fidelityNote"` // 必须随结果一起展示的精度警示
	SignalCount  int    `json:"signalCount"`  // 回放消费的真实触发数
}

type BacktestRunListDTO struct {
	Total int64            `json:"total"`
	List  []BacktestRunDTO `json:"list"`
}

type BacktestTradeDTO struct {
	ID                 int64     `json:"id"`
	PredictionID       int64     `json:"predictionId"`
	CalcMode           string    `json:"calcMode"`
	PredictTime        string    `json:"predictTime"` // 预测目标时刻(关联预测 predict_time)，与 requestedAt 框定预测周期
	Direction          string    `json:"direction"`
	EntryMode          string    `json:"entryMode"`
	PlannedEntryPrice  float64   `json:"plannedEntryPrice"`
	TakeProfitPrice    float64   `json:"takeProfitPrice"`
	StopLossPrice      float64   `json:"stopLossPrice"`
	Status             string    `json:"status"`
	OpenPrice          float64   `json:"openPrice"`
	ClosePrice         float64   `json:"closePrice"`
	CloseReason        string    `json:"closeReason"`
	RequestedAt        string    `json:"requestedAt"`
	OpenedAt           string    `json:"openedAt"`
	ClosedAt           string    `json:"closedAt"`
	Pnl                float64   `json:"pnl"`
	PnlRate            float64   `json:"pnlRate"`    // 盈亏率%(含杠杆，未扣费) = 价差/开仓价×杠杆×100
	NetPnlRate         float64   `json:"netPnlRate"` // 净盈亏率%(含杠杆，已扣往返手续费)
	NetPnl             float64   `json:"netPnl"`
	Fee                float64   `json:"fee"`
	Confidence         float64   `json:"confidence"`
	Efficiency         float64   `json:"efficiency"`
	PredHigh           float64   `json:"predHigh"`           // 预测区间上沿
	PredLow            float64   `json:"predLow"`            // 预测区间下沿
	PredClose          float64   `json:"predClose"`          // 预测收盘价
	WindowOpen         float64   `json:"windowOpen"`         // 信号后窗口实际开盘价
	WindowClose        float64   `json:"windowClose"`        // 信号后窗口实际收盘价
	WindowLow          float64   `json:"windowLow"`          // 信号后窗口最低价
	WindowHigh         float64   `json:"windowHigh"`         // 信号后窗口最高价
	PressureHigh       float64   `json:"pressureHigh"`       // 压力面最高价(关键阻力)
	PressureLow        float64   `json:"pressureLow"`        // 压力面最低价(关键支撑)
	MaxPriceDuringHold float64   `json:"maxPriceDuringHold"` // 持仓期间最高价(算最高浮盈用)
	MinPriceDuringHold float64   `json:"minPriceDuringHold"` // 持仓期间最低价
	FavPeakDeciles     []float64 `json:"favPeakDeciles"`     // 分时段峰值浮盈[10]：第i项=前(i+1)×10%持仓时间内累积最高浮盈ROI%(含杠杆)
	Leverage           float64   `json:"leverage"`           // 杠杆倍数(算含杠杆浮盈用)
	// 持仓中(status=open)按当前最新价标记的浮动盈亏：回测窗口未走完生命周期的持仓，用最新一根 K 线收盘价标记。
	MarkPrice            float64 `json:"markPrice"`            // 当前最新价(标记价)，仅持仓中有值
	UnrealizedPnl        float64 `json:"unrealizedPnl"`        // 浮动毛盈亏 USDT
	UnrealizedPnlRate    float64 `json:"unrealizedPnlRate"`    // 浮动盈亏率%(含杠杆，未扣费)
	UnrealizedNetPnl     float64 `json:"unrealizedNetPnl"`     // 浮动净盈亏 = 浮动盈亏 - 预估手续费
	UnrealizedNetPnlRate float64 `json:"unrealizedNetPnlRate"` // 浮动净盈亏率%(含杠杆，已扣预估手续费)
	// 盘口信号回测（calcMode=signal）专用：一行 = 一个持仓生命周期
	Contracts    int     `json:"contracts"`    // 平仓张数
	MaxContracts int     `json:"maxContracts"` // 生命周期内最大张数
	AddCount     int     `json:"addCount"`     // 加仓次数(含首次建仓)
	PeakPct      float64 `json:"peakPct"`      // 移动止盈峰值ROI%(回测口径，用1m high 算，偏高)
	ReducedPnl   float64 `json:"reducedPnl"`   // 生命周期内反向减仓锁利累计
}

type BacktestMetricDTO struct {
	RunID             int64   `json:"runId"`
	CalcMode          string  `json:"calcMode"`
	TradeCount        int     `json:"tradeCount"`
	FillCount         int     `json:"fillCount"`
	ExpiredCount      int     `json:"expiredCount"`
	FillRate          float64 `json:"fillRate"`
	WinCount          int     `json:"winCount"`
	WinRate           float64 `json:"winRate"`
	GrossPnl          float64 `json:"grossPnl"`
	FeeTotal          float64 `json:"feeTotal"`
	NetPnl            float64 `json:"netPnl"`
	Expectancy        float64 `json:"expectancy"`
	ProfitFactor      float64 `json:"profitFactor"`
	MaxDrawdown       float64 `json:"maxDrawdown"`
	Sharpe            float64 `json:"sharpe"`
	AvgHoldSecs       float64 `json:"avgHoldSecs"`
	TpCount           int     `json:"tpCount"`
	SlCount           int     `json:"slCount"`
	TrailCount        int     `json:"trailCount"`
	EarlyCutCount     int     `json:"earlyCutCount"`
	EarlyAdverseCount int     `json:"earlyAdverseCount"`
	TimeoutCount      int     `json:"timeoutCount"`
	// 盘口信号回测（calcMode=signal）专用
	Fidelity         string  `json:"fidelity"`
	FidelityNote     string  `json:"fidelityNote"`
	SignalCount      int     `json:"signalCount"`
	SignalDropped    int     `json:"signalDropped"`
	SignalFiltered   int     `json:"signalFiltered"`
	CapSkipCount     int     `json:"capSkipCount"`
	GateSkipCount    int     `json:"gateSkipCount"`
	TrendSkipCount   int     `json:"trendSkipCount"`
	ReduceCount      int     `json:"reduceCount"`
	ReduceCloseCount int     `json:"reduceCloseCount"`
	EodOpenCount     int     `json:"eodOpenCount"`
	MaxStack         int     `json:"maxStack"`
	CapEffective     int     `json:"capEffective"`
	RealizedPnl      float64 `json:"realizedPnl"`
	FloatingPnl      float64 `json:"floatingPnl"`
	MaxDrawdownPct   float64 `json:"maxDrawdownPct"`
	LambdaPerDay     float64 `json:"lambdaPerDay"`
	LambdaRatio      float64 `json:"lambdaRatio"`
	LambdaSelfTest   float64 `json:"lambdaSelfTest"`
}

// BacktestRunDetailDTO 单次回测详情：任务 + 汇总指标(可能两种口径) + 逐笔。
type BacktestRunDetailDTO struct {
	Run     BacktestRunDTO      `json:"run"`
	Metrics []BacktestMetricDTO `json:"metrics"` // 按 calcMode 区分：prediction / trading
	Trades  []BacktestTradeDTO  `json:"trades"`
}

// ─── 参数组批量扫描与横向对比（r11）──────────────────────────────────────────

// SignalBacktestGroupDTO 批量扫描里的一组参数。
//
// 组参数是**相对基线的增量**：没给的旋钮沿用基线（= 所选实例当前生产参数），
// 不回落代码缺省。这样 "cap 15/26/40 三组" 的 diff 里就只有 cap 一行，而不是
// 十几个字段同时偏离生产。
type SignalBacktestGroupDTO struct {
	// Label 组标签，用于对比矩阵的行名。留空时服务端按 diff 自动生成
	// （如 "capOverride=26"），全同基线时生成 "same_as_baseline"。
	Label string `json:"label"`
	SignalBacktestParamsDTO
}

// CreateSignalBacktestBatchDTO 一次提交多组参数。
//
// 信号源与窗口是**批次级**的，不允许逐组指定：组间要能比大小的前提是吃同一份
// 触发流，允许每组换实例/换窗口就等于允许把不可比的东西排进同一张榜。
type CreateSignalBacktestBatchDTO struct {
	Name         string `json:"name"`
	PlatformCode string `json:"platformCode"` // 1m 路径回放平台，缺省 deepcoin
	CoinCode     string `json:"coinCode"`
	Symbol       string `json:"symbol"` // 缺省 BTCUSDT
	StartTime    string `json:"startTime" binding:"required"`
	EndTime      string `json:"endTime" binding:"required"`
	InstanceKey  string `json:"instanceKey" binding:"required"`
	AccountLabel string `json:"accountLabel" binding:"required"`

	// Concurrency 并发执行的组数上限；<=0 取缺省 3，上限 8。
	// 回放是纯 CPU 的，放太开只会把管理端进程的 CPU 吃满、拖慢在线接口。
	Concurrency int `json:"concurrency"`
	// BaselineParams 显式基线；为空时取所选实例当前生产参数（已发布配置版本）。
	// 配置还没导入 DB、或要拿某个历史参数包当基线时用它。
	BaselineParams *SignalBacktestParamsDTO `json:"baselineParams"`
	// IncludeBaselineRun 是否把基线本身也当一组跑（缺省 true）。
	// 关掉它就没有基线侧的指标，diff 只剩参数差异、没有指标差异。
	IncludeBaselineRun *bool `json:"includeBaselineRun"`

	Groups []SignalBacktestGroupDTO `json:"groups" binding:"required"`
}

// SignalBacktestBatchQueryDTO 批次列表查询。
type SignalBacktestBatchQueryDTO struct {
	Page         int    `form:"page"`
	PageSize     int    `form:"pageSize"`
	InstanceKey  string `form:"instanceKey"`
	AccountLabel string `form:"accountLabel"`
}

// SignalBaselineQueryDTO 基线预览查询：表单在提交前先看清基线是什么。
type SignalBaselineQueryDTO struct {
	InstanceKey  string `form:"instanceKey" binding:"required"`
	AccountLabel string `form:"accountLabel" binding:"required"`
	Symbol       string `form:"symbol"`
}

// SignalBaselineDTO 解析出来的基线：参数 + 每个字段从哪来。
// Notes 必须在页面上和参数一起显示——配置面收敛（r5）没做完之前，基线里有一部分
// 字段是按实盘缺省兜底的，不说清楚就会被当成生产事实读。
type SignalBaselineDTO struct {
	Source string `json:"source"` // instance_published / request
	// Params 是 signal.Params 的原样 JSON（键名即回测参数键），不再拆一层。
	Params map[string]interface{} `json:"params"`
	Notes  []string               `json:"notes"`
	FromDB []string               `json:"fromDb"` // 确实从 DB 取到值的配置键
}

// SignalBacktestParamDiffDTO 一个参数键与基线的差异。
type SignalBacktestParamDiffDTO struct {
	Key      string  `json:"key"`      // 生产配置键口径，如 position.risk.max_contracts_ceiling
	Field    string  `json:"field"`    // 回测参数字段名，如 ceiling
	Baseline string  `json:"baseline"` // 基线值（字符串，避免数值/枚举两套类型）
	Value    string  `json:"value"`    // 本组取值
	Delta    float64 `json:"delta"`    // 数值型的差值；枚举型为 0
	Numeric  bool    `json:"numeric"`  // Delta 是否有意义
}

// SignalBacktestMetricDiffDTO 本组指标与基线的差异（同精度等级才计算）。
// 字段全部取自 trade_backtest_metric 既有列，不新造指标。
type SignalBacktestMetricDiffDTO struct {
	NetPnl       float64 `json:"netPnl"`       // 净盈亏差（USDT）
	NetPnlPct    float64 `json:"netPnlPct"`    // 相对基线的百分比变化；基线为 0 时留 0
	WinRate      float64 `json:"winRate"`      // 胜率差（绝对值，0.01 = 1 个百分点）
	ProfitFactor float64 `json:"profitFactor"` // 盈亏比差
	MaxDrawdown  float64 `json:"maxDrawdown"`  // 最大回撤差（USDT，正=回撤更大）
	Sharpe       float64 `json:"sharpe"`       // 夏普差
	TradeCount   int     `json:"tradeCount"`   // 持仓生命周期数差
	TrailCount   int     `json:"trailCount"`   // 移动止盈平仓数差
	SlCount      int     `json:"slCount"`      // 兜底止损数差
	ReduceClose  int     `json:"reduceClose"`  // 减仓削零数差
	EodOpen      int     `json:"eodOpen"`      // 期末仍持仓数差
	MaxStack     int     `json:"maxStack"`     // 最大堆积张数差
}

// SignalBacktestComparisonRowDTO 对比矩阵的一行 = 一组参数。
type SignalBacktestComparisonRowDTO struct {
	RunID      int64  `json:"runId"`
	GroupLabel string `json:"groupLabel"`
	IsBaseline bool   `json:"isBaseline"`
	Status     string `json:"status"` // pending/running/done/failed
	ErrorMsg   string `json:"errorMsg"`
	Fidelity   string `json:"fidelity"`

	ParamDiff []SignalBacktestParamDiffDTO `json:"paramDiff"`
	Metric    *BacktestMetricDTO           `json:"metric"`
	// MetricDiff 仅在与基线**同精度等级**且两侧都有指标时非空。
	MetricDiff *SignalBacktestMetricDiffDTO `json:"metricDiff"`
	// DiffBlockedReason 说明为什么没有 MetricDiff（精度不同 / 基线缺失 / 本组未完成 /
	// 本组不产 PnL）。空串表示有 diff。
	DiffBlockedReason string `json:"diffBlockedReason"`
	// PnlAvailable 本组是否产出了 PnL。降低阈值的频率级组只输出 λ(θ)、不产 PnL，
	// 它们不能按净利排序，也不能与任何产 PnL 的组比大小。
	PnlAvailable bool `json:"pnlAvailable"`
}

// SignalBacktestFidelityGroupDTO 按精度等级分组后的一组对比行。
//
// 需求大纲 §3.3 与本任务的"不做"都明确：频率级与事件级结果不得放在同一排序里
// 比大小。所以排序只在组内做，接口层从不返回一张跨精度的全局榜——前端拿不到
// 混排数据，也就没法误排。
type SignalBacktestFidelityGroupDTO struct {
	Fidelity      string `json:"fidelity"`      // event / frequency
	FidelityLabel string `json:"fidelityLabel"` // 事件级 / 频率级
	// ComparableToBaseline 本精度组是否与基线同精度：false 时组内各行没有 metricDiff，
	// 只能组内相互比较。
	ComparableToBaseline bool `json:"comparableToBaseline"`
	// SortedBy 组内排序依据：netPnl（有 PnL）或 lambdaPerDay（只有 λ 的频率级组）。
	SortedBy string                           `json:"sortedBy"`
	Notes    []string                         `json:"notes"` // 本组内出现过的精度警示（去重）
	Rows     []SignalBacktestComparisonRowDTO `json:"rows"`
}

// SignalBacktestBatchDTO 批次头。
type SignalBacktestBatchDTO struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	InstanceKey    string `json:"instanceKey"`
	AccountLabel   string `json:"accountLabel"`
	PlatformCode   string `json:"platformCode"`
	CoinCode       string `json:"coinCode"`
	Symbol         string `json:"symbol"`
	StartTime      string `json:"startTime"`
	EndTime        string `json:"endTime"`
	Status         string `json:"status"`
	ErrorMsg       string `json:"errorMsg"`
	Concurrency    int    `json:"concurrency"`
	GroupCount     int    `json:"groupCount"`
	DoneCount      int    `json:"doneCount"`
	FailedCount    int    `json:"failedCount"`
	BaselineRunID  int64  `json:"baselineRunId"`
	BaselineSource string `json:"baselineSource"`
	CreatedTime    string `json:"createdTime"`
}

type SignalBacktestBatchListDTO struct {
	Total int64                    `json:"total"`
	List  []SignalBacktestBatchDTO `json:"list"`
}

// SignalBacktestBatchDetailDTO 批次详情 = 批次头 + 基线 + 按精度分组的对比矩阵。
type SignalBacktestBatchDetailDTO struct {
	Batch    SignalBacktestBatchDTO           `json:"batch"`
	Baseline SignalBaselineDTO                `json:"baseline"`
	Groups   []SignalBacktestFidelityGroupDTO `json:"groups"`
	// Warnings 整批级别的提醒：基线未跑完、跨精度组存在、样本量不足等。
	Warnings []string `json:"warnings"`
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

// KlineRangeQueryDTO 按平台+symbol+interval+时间区间拉取 K 线。
type KlineRangeQueryDTO struct {
	PlatformCode string `form:"platformCode"` // 行情平台 binance/deepcoin，空则按 binance
	Symbol       string `form:"symbol"`
	Interval     string `form:"interval"`
	Start        string `form:"start"`
	End          string `form:"end"`
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

// BackfillKlineDTO 触发 K 线回填的入参：拉某平台某币种某周期“最近 limit 根”入库。
type BackfillKlineDTO struct {
	PlatformCode string `json:"platformCode"` // 交易所 binance/deepcoin，默认 binance
	Symbol       string `json:"symbol" binding:"required"`
	Interval     string `json:"interval" binding:"required"` // 1m/5m/15m/1h/4h/1d ...
	Limit        int    `json:"limit" binding:"required"`    // 想要的最近根数
}

// BatchBackfillKlineDTO 批量回填入参：平台 × 币种 × 周期 三维笛卡尔积，逐组合独立回填。
// 复数字段为空时回落到对应的单数字段，兼容老的单组合请求体。
type BatchBackfillKlineDTO struct {
	PlatformCodes []string `json:"platformCodes"` // 空则取 platformCode，仍为空默认 [binance]
	PlatformCode  string   `json:"platformCode"`
	Symbols       []string `json:"symbols"` // 空则取 symbol
	Symbol        string   `json:"symbol"`
	Intervals     []string `json:"intervals"` // 空则取 interval，仍为空默认 [1m,5m,1h,1d]
	Interval      string   `json:"interval"`
	Limit         int      `json:"limit"` // 每个组合想要的最近根数
}

// BackfillKlineBatchResultDTO 批量回填汇总：逐组合结果 + 成功/失败计数。
type BackfillKlineBatchResultDTO struct {
	Total     int                      `json:"total"`     // 组合总数
	Succeeded int                      `json:"succeeded"` // 回填成功的组合数
	Failed    int                      `json:"failed"`    // 回填失败的组合数
	Items     []BackfillKlineResultDTO `json:"items"`     // 逐组合明细(失败项带 error)
}

// BackfillKlineResultDTO K 线回填结果：把“现有→需补→实拉→入库”的链路透明化。
type BackfillKlineResultDTO struct {
	PlatformCode string `json:"platformCode"` // 行情平台
	Symbol       string `json:"symbol"`
	Interval     string `json:"interval"`
	Requested    int    `json:"requested"`       // 请求的最近根数
	LatestBefore string `json:"latestBefore"`    // 回填前 DB 最新一根 open_time(空=原本无数据)
	NeedFetch    int    `json:"needFetch"`       // 据最新一条推算出需向交易所拉取的根数
	Fetched      int    `json:"fetched"`         // 实际从交易所拉到的根数
	Upserted     int64  `json:"upserted"`        // 幂等入库影响行数
	LatestAfter  string `json:"latestAfter"`     // 回填后 DB 最新一根 open_time
	Error        string `json:"error,omitempty"` // 该组合的失败原因(批量回填时单组合失败不影响其它组合)
}

// BackfillKlineRangeDTO 按【时间窗口覆盖率】回填 K 线的入参。
//
// 与 BackfillKlineDTO/BatchBackfillKlineDTO 的「最近 N 根增量」不同：那套以
// DB 最新一根为基准推算需补根数，窗口整段落在过去时会误判成「已是最新」而漏补
// 历史空洞（行情主视图 r12 的「回填缺口」正是这个场景）。这里改按窗口内实际
// 覆盖率判定，再按 (now - start)/周期 往回兜——交易所只提供「最近 N 根」。
//
// Start/End 建议传 RFC3339（带时区偏移）。传裸 "YYYY-MM-DD HH:mm:ss" 会按 UTC
// 解析，而 argus-event 的时间串是本地墙钟，两者混用会整体错开时区偏移。
type BackfillKlineRangeDTO struct {
	PlatformCodes []string `json:"platformCodes"` // 空则取 platformCode，仍为空默认 [binance]
	PlatformCode  string   `json:"platformCode"`
	Symbol        string   `json:"symbol" binding:"required"`
	Intervals     []string `json:"intervals"` // 空则取 interval，仍为空默认 [1m,5m,1h,1d]
	Interval      string   `json:"interval"`
	Start         string   `json:"start" binding:"required"`
	End           string   `json:"end"` // 空 = 现在
}

// BackfillKlineRangeItemDTO 单个「平台 × 周期」组合的窗口回填结果。
type BackfillKlineRangeItemDTO struct {
	PlatformCode string `json:"platformCode"`
	Symbol       string `json:"symbol"`
	Interval     string `json:"interval"`
	Expected     int    `json:"expected"`   // 窗口内理应有的根数
	HaveBefore   int    `json:"haveBefore"` // 回填前窗口内已有根数
	NeedFetch    int    `json:"needFetch"`  // 为兜到窗口左界需向交易所要的根数(已按单次上限截断)
	Fetched      int    `json:"fetched"`    // 实际拉到的根数
	Upserted     int64  `json:"upserted"`   // 幂等入库影响行数
	HaveAfter    int    `json:"haveAfter"`  // 回填后窗口内已有根数
	Skipped      bool   `json:"skipped"`    // 覆盖率已达标，未发起请求
	Capped       bool   `json:"capped"`     // 窗口太老，单次「最近 N 根」够不到左界
	Note         string `json:"note"`       // 人话说明(补不动时说清为什么)
	Error        string `json:"error,omitempty"`
}

// BackfillKlineRangeResultDTO 窗口回填汇总。
type BackfillKlineRangeResultDTO struct {
	Start     string                      `json:"start"` // 实际生效窗口(UTC 串)
	End       string                      `json:"end"`
	Total     int                         `json:"total"`
	Succeeded int                         `json:"succeeded"`
	Failed    int                         `json:"failed"`
	Items     []BackfillKlineRangeItemDTO `json:"items"`
}

// ─── 后台自动参数寻优（r16）──────────────────────────────────────────────────
//
// 这一段的请求/响应体**直接复用 strategy/signal 的领域结构**
// （SearchSpace / Protocol / Convergence / Gates / CellSpec / CellStats 系），
// 不在 dto 里手抄一份镜像。理由与 r11 的 paramsToMap 一致：搜索空间的轴、
// 降噪协议的抖动维度、三关阈值都会随研究推进增删，抄一份必然漂移，而这三样
// 一旦与引擎不一致，落库的"冻结快照"就不再等于实际跑的那份配置——那就把整个
// 可复现性给毁了。signal 包不依赖 dto，没有环。

// CreateSignalOptimizeStudyDTO 发起一次后台自动参数寻优。
//
// 关键约束：Gates（三关阈值）在**创建时锁定**，跑完不接受修改。要换阈值只能
// 建新任务。这是本任务唯一的防过拟合机械保障——跑完再定标准等于用同一份数据
// 既定标准又选参数。
type CreateSignalOptimizeStudyDTO struct {
	Name         string `json:"name"`
	PlatformCode string `json:"platformCode"` // 1m 路径回放平台，缺省 deepcoin
	CoinCode     string `json:"coinCode"`
	Symbol       string `json:"symbol"`    // 缺省 BTCUSDT
	StartTime    string `json:"startTime"` // OOS 任务可留空，继承基准任务之后的窗口需自行给出
	EndTime      string `json:"endTime"`
	InstanceKey  string `json:"instanceKey"`
	AccountLabel string `json:"accountLabel"`

	// Concurrency 并发执行的格数上限；<=0 取缺省 3，上限 8。
	// 单格内的 16 条路径是顺序跑的：并发放在格级已经足够吃满 CPU。
	Concurrency int `json:"concurrency"`

	// BaselineParams 显式基线；为空时取所选实例当前已发布的生产参数。
	// 基线提供的是"不参与寻优的那些旋钮"（trail 档位、面值、order_size、
	// risk_equity），四个搜索轴会覆盖在它之上。
	BaselineParams *SignalBacktestParamsDTO `json:"baselineParams"`

	// Space / Protocol / Converge / Gates 留空即取缺省（= 设计文档 §3.3 的搜索空间、
	// §3.2 的 16 路径降噪协议、§10.2 的三关阈值）。
	Space    *SignalOptimizeSpaceInput    `json:"space"`
	Protocol *SignalOptimizeProtocolInput `json:"protocol"`
	Converge *SignalOptimizeConvergeInput `json:"converge"`
	Gates    *SignalOptimizeGatesInput    `json:"gates"`

	// Incumbent 现行线上配置对应的格；留空时由基线参数推出。
	// 它会被强制纳入精算格——"现行配置是否被支配"必须有结论。
	Incumbent *SignalOptimizeCellInput `json:"incumbent"`
	// FineCells 显式指定精算格。给了就**跳过粗网格**，直接精算这些格子
	// （用于复现历史精算清单，如 study_fine.csv 的 10 格）。
	FineCells []SignalOptimizeCellInput `json:"fineCells"`

	// OosBaseStudyID 以某次已完成的扫描为基准建 out_of_sample 任务：
	// 继承它冻结的阈值、协议与精算格，只换数据窗口，不重新调参。
	// 窗口起点必须晚于基准任务的阈值锁定时刻，否则不是真正的样本外。
	OosBaseStudyID int64 `json:"oosBaseStudyId"`
}

// SignalOptimizeSpaceInput 搜索空间输入，字段与 signal.SearchSpace 同名同义。
type SignalOptimizeSpaceInput struct {
	NetCaps       []int     `json:"netCaps"`
	DualCaps      []int     `json:"dualCaps"`
	StopPcts      []float64 `json:"stopPcts"`
	GatePcts      []float64 `json:"gatePcts"`
	IncludeDual   *bool     `json:"includeDual"`
	CoarseGatePct float64   `json:"coarseGatePct"`
}

// SignalOptimizeProtocolInput 降噪协议输入。留空的字段取金标准缺省。
type SignalOptimizeProtocolInput struct {
	EvalModes       []string `json:"evalModes"`
	OffsetDays      []int    `json:"offsetDays"`
	DropSeeds       []*int64 `json:"dropSeeds"`
	DropRate        float64  `json:"dropRate"`
	NormalizeDays   float64  `json:"normalizeDays"`
	ScenarioDays    int      `json:"scenarioDays"`
	BootstrapMode   string   `json:"bootstrapMode"` // day_iid / episode_block
	BootstrapDraws  int      `json:"bootstrapDraws"`
	BootstrapSeed   int64    `json:"bootstrapSeed"`
	LambdaDenom     string   `json:"lambdaDenom"` // window / path
	LambdaMonthDays float64  `json:"lambdaMonthDays"`
	TrendAbsRetPct  float64  `json:"trendAbsRetPct"`
	VolRangePct     float64  `json:"volRangePct"`
	// CoarseOffsetDays / CoarseDropSeeds 粗网格阶段的抖动轴（缺省 4 条路径）。
	CoarseOffsetDays []int    `json:"coarseOffsetDays"`
	CoarseDropSeeds  []*int64 `json:"coarseDropSeeds"`
}

// SignalOptimizeConvergeInput 粗→精收敛规则输入。
type SignalOptimizeConvergeInput struct {
	TopK            int   `json:"topK"`
	KeepAllPositive *bool `json:"keepAllPositive"`
	ExpandGateAxis  *bool `json:"expandGateAxis"`
	MaxCells        int   `json:"maxCells"`
}

// SignalOptimizeGatesInput 预注册三关阈值输入。
//
// 优先给百分比（riskEquity + bearBudgetPct + ddMaxPct），绝对阈值由它们派生；
// 也可以直接给绝对阈值来复现历史标准（如 −56U / 94U）。
type SignalOptimizeGatesInput struct {
	RiskEquity     float64 `json:"riskEquity"`
	BearBudgetPct  float64 `json:"bearBudgetPct"`
	DdMaxPct       float64 `json:"ddMaxPct"`
	SignMin        float64 `json:"signMin"`
	BearNetP10Min  float64 `json:"bearNetP10Min"`
	MaxDrawdownMax float64 `json:"maxDrawdownMax"`
}

// SignalOptimizeCellInput 搜索空间里的一格。
type SignalOptimizeCellInput struct {
	Mode    string  `json:"mode"` // net / dual
	Cap     int     `json:"cap"`
	StopPct float64 `json:"stopPct"`
	GatePct float64 `json:"gatePct"`
}

// SignalOptimizeStudyDTO 寻优任务头。
type SignalOptimizeStudyDTO struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	InstanceKey  string `json:"instanceKey"`
	AccountLabel string `json:"accountLabel"`
	PlatformCode string `json:"platformCode"`
	CoinCode     string `json:"coinCode"`
	Symbol       string `json:"symbol"`
	StartTime    string `json:"startTime"`
	EndTime      string `json:"endTime"`

	SampleKind string `json:"sampleKind"` // in_sample / out_of_sample
	SampleNote string `json:"sampleNote"`
	OosBaseID  int64  `json:"oosBaseId"`

	Stage           string `json:"stage"` // coarse / fine / concluded
	Status          string `json:"status"`
	ErrorMsg        string `json:"errorMsg"`
	Concurrency     int    `json:"concurrency"`
	CoarseCellCount int    `json:"coarseCellCount"`
	FineCellCount   int    `json:"fineCellCount"`
	DoneCellCount   int    `json:"doneCellCount"`
	FailedCellCount int    `json:"failedCellCount"`
	SkipCellCount   int    `json:"skipCellCount"`
	ReplayCount     int    `json:"replayCount"`
	ConvergeNote    string `json:"convergeNote"`

	SignalCount   int `json:"signalCount"`
	KlineCount    int `json:"klineCount"`
	TrendDayCount int `json:"trendDayCount"`
	VolDayCount   int `json:"volDayCount"`

	// GateLockedAt 阈值锁定时刻。页面必须显示它：没有它，"预注册"就只是一句宣称。
	GateLockedAt    string `json:"gateLockedAt"`
	GateNote        string `json:"gateNote"`
	Verdict         string `json:"verdict"`
	PassedCellCount int    `json:"passedCellCount"`
	BaselineSource  string `json:"baselineSource"`
	CreatedTime     string `json:"createdTime"`
}

type SignalOptimizeStudyListDTO struct {
	Total int64                    `json:"total"`
	List  []SignalOptimizeStudyDTO `json:"list"`
}

// SignalOptimizeCellRowDTO 结果矩阵的一行 = 一格。
//
// 刻意**没有** netPnl 这种单值字段：一格的产出是一束路径上的分布，给一个
// "该格的 PnL" 就是在邀请人按点估计排序，而这正是本任务要消灭的误读
// （现行 champion 的符号一致率 0.56，按点估计排它并不难看）。
type SignalOptimizeCellRowDTO struct {
	ID       int64   `json:"id"`
	Stage    string  `json:"stage"`
	Key      string  `json:"key"`
	Mode     string  `json:"mode"`
	Cap      int     `json:"cap"`
	StopPct  float64 `json:"stopPct"`
	GatePct  float64 `json:"gatePct"`
	Fidelity string  `json:"fidelity"`
	Status   string  `json:"status"`
	ErrorMsg string  `json:"errorMsg"`

	PathCount int     `json:"pathCount"`
	MedPnl28  float64 `json:"medPnl28"`
	P25Pnl28  float64 `json:"p25Pnl28"`
	P75Pnl28  float64 `json:"p75Pnl28"`
	IqrPnl28  float64 `json:"iqrPnl28"`
	MinPnl28  float64 `json:"minPnl28"`
	MaxPnl28  float64 `json:"maxPnl28"`
	SignRatio float64 `json:"signRatio"`

	LambdaBear   float64 `json:"lambdaBear"`
	MeanStopLoss float64 `json:"meanStopLoss"`
	StopBudget   float64 `json:"stopBudget"`
	StopCount    int     `json:"stopCount"`

	P90MaxDrawdown float64 `json:"p90MaxDrawdown"`
	MaxStack       int     `json:"maxStack"`
	MedFee         float64 `json:"medFee"`
	MedDays        float64 `json:"medDays"`
	MedSignalRun   int     `json:"medSignalRun"`

	BearPoolSize int     `json:"bearPoolSize"`
	BearP10      float64 `json:"bearP10"`
	BearP50      float64 `json:"bearP50"`
	BearP90      float64 `json:"bearP90"`
	ChopP10      float64 `json:"chopP10"`
	ChopP50      float64 `json:"chopP50"`
	MixedP10     float64 `json:"mixedP10"`
	MixedP50     float64 `json:"mixedP50"`

	OkSign      bool     `json:"okSign"`
	OkBear      bool     `json:"okBear"`
	OkDd        bool     `json:"okDd"`
	OkBudget    bool     `json:"okBudget"`
	Passed      bool     `json:"passed"`
	PassCount   int      `json:"passCount"`
	VerdictNote string   `json:"verdictNote"`
	DominatedBy []string `json:"dominatedBy"`

	IsIncumbent    bool `json:"isIncumbent"`
	OnDdFrontier   bool `json:"onDdFrontier"`
	OnBearFrontier bool `json:"onBearFrontier"`

	// Notes 本格必须随结果展示的口径说明（bootstrap 偏差、λ 分母偏差、样本薄等）。
	Notes []string `json:"notes"`
	// Params 本格的完整参数快照，可直接提交给 POST /backtest/signal-runs 单跑看逐笔。
	Params map[string]interface{} `json:"params"`
	// Paths 逐路径产出（不含日 MTM 序列，那是中间量）。
	Paths []map[string]interface{} `json:"paths,omitempty"`
}

// SignalOptimizeStudyDetailDTO 寻优任务详情。
type SignalOptimizeStudyDetailDTO struct {
	Study SignalOptimizeStudyDTO `json:"study"`
	// Space / Protocol / Converge / Gates 是任务**冻结**的那份，不是当前缺省。
	Space     map[string]interface{} `json:"space"`
	Protocol  map[string]interface{} `json:"protocol"`
	Converge  map[string]interface{} `json:"converge"`
	Gates     map[string]interface{} `json:"gates"`
	Baseline  SignalBaselineDTO      `json:"baseline"`
	Incumbent map[string]interface{} `json:"incumbent"`

	Coarse []SignalOptimizeCellRowDTO `json:"coarse"`
	Fine   []SignalOptimizeCellRowDTO `json:"fine"`

	// Conclusion 结论（含无解分支的权衡前沿与被支配的现行配置）。未跑完为空。
	Conclusion  map[string]interface{}   `json:"conclusion"`
	Frontier    []map[string]interface{} `json:"frontier"`
	ScaleChecks []map[string]interface{} `json:"scaleChecks"`
	Warnings    []string                 `json:"warnings"`
}

// SignalOptimizeStudyQueryDTO 任务列表查询。
type SignalOptimizeStudyQueryDTO struct {
	Page         int    `form:"page"`
	PageSize     int    `form:"pageSize"`
	InstanceKey  string `form:"instanceKey"`
	AccountLabel string `form:"accountLabel"`
	SampleKind   string `form:"sampleKind"`
}

// SignalOptimizeDefaultsDTO 发起表单的缺省值预览：搜索空间会展开成多少格、
// 降噪协议是哪几条路径、三关阈值是多少。让人在提交前就能核对这三样。
type SignalOptimizeDefaultsDTO struct {
	Space           map[string]interface{} `json:"space"`
	CoarseCellCount int                    `json:"coarseCellCount"`
	CoarseCells     []string               `json:"coarseCells"`
	Protocol        map[string]interface{} `json:"protocol"`
	FinePathCount   int                    `json:"finePathCount"`
	FinePaths       []string               `json:"finePaths"`
	CoarsePathCount int                    `json:"coarsePathCount"`
	CoarsePaths     []string               `json:"coarsePaths"`
	Converge        map[string]interface{} `json:"converge"`
	Gates           map[string]interface{} `json:"gates"`
	GateNote        string                 `json:"gateNote"`
	// EstimatedReplays 预估总回放次数：粗网格格数×粗路径数 + 精算格数×精路径数。
	EstimatedReplays int      `json:"estimatedReplays"`
	Notes            []string `json:"notes"`
}
