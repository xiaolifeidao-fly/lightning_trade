package dto

import (
	baseDTO "common/base/dto"
	"time"
)

type TradeOrderDTO struct {
	baseDTO.BaseDTO
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
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	UserID    uint64 `form:"userId"`
	Symbol    string `form:"symbol"`
	Side      string `form:"side"`
	OrderType string `form:"orderType"`
	Status    string `form:"status"`
	OrderNo   string `form:"orderNo"`
	StartTime int64  `form:"startTime"`
	EndTime   int64  `form:"endTime"`
}

type TradeMatchDTO struct {
	baseDTO.BaseDTO
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
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	UserID    uint64 `form:"userId"`
	Symbol    string `form:"symbol"`
	Limit     int    `form:"limit"`
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
