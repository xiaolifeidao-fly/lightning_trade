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
	Symbol       string `form:"symbol"`
	Side         string `form:"side"`
	OrderType    string `form:"orderType"`
	Status       string `form:"status"`
	OrderNo      string `form:"orderNo"`
	StartTime    int64  `form:"startTime"`
	EndTime      int64  `form:"endTime"`
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
	UserID          uint64  `json:"userId"`
	PlatformID      uint64  `json:"platformId"`
	PlatformCode    string  `json:"platformCode"`
	CoinCode        string  `json:"coinCode"`
	TradeCategory   string  `json:"tradeCategory"`
	TradeDate       string  `json:"tradeDate"`
	RealizedPnl     float64 `json:"realizedPnl"`
	UnrealizedPnl   float64 `json:"unrealizedPnl"`
	TotalPnl        float64 `json:"totalPnl"`
	PnlRate         float64 `json:"pnlRate"`
	PositionAmount  float64 `json:"positionAmount"`
	PositionCost    float64 `json:"positionCost"`
	PositionValue   float64 `json:"positionValue"`
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
