package trade

import (
	"context"
	"fmt"
	"strings"

	"common/utils"
)

const (
	PlatformDeepcoin = "deepcoin"
	PlatformBinance  = "binance"
)

// ExchangeClient 定义交易所统一操作接口。
// 行情、账户、交易能力都从这里进入，平台差异留在各自实现里。
type ExchangeClient interface {
	// GetKlines 查询 K 线数据。公开行情接口不强依赖账户配置。
	GetKlines(ctx context.Context, req MarketKlineRequest) ([]MarketKline, error)

	// GetTicker 查询实时价格/盘口快照。公开行情接口不强依赖账户配置。
	GetTicker(ctx context.Context, req MarketTickerRequest) (*MarketTicker, error)

	// GetRecentTrades 查询最近逐笔成交（聚合成交）。公开行情接口不强依赖账户配置。
	GetRecentTrades(ctx context.Context, req MarketTradeRequest) ([]MarketTrade, error)

	// GetFundingRate 查询合约资金费率（最新一条 + 最近若干历史）。公开行情接口不强依赖账户配置。
	GetFundingRate(ctx context.Context, req FundingRateRequest) (*FundingRateSnapshot, error)

	// GetBalances 查询账户余额。需要平台对应账户 API 凭证。
	GetBalances(ctx context.Context, req BalanceRequest) ([]Balance, error)

	// PlaceOrder 统一下单入口。side/positionSide/orderType 由调用方按业务传入。
	PlaceOrder(ctx context.Context, req ExchangeOrderRequest) (*ExchangeOrderResponse, error)

	// OpenPosition 市价开仓。side: "long" 或 "short"，size 为合约张数，price 仅用于风控埋点。
	OpenPosition(instId, side string, size int, price float64) (*utils.WebOrderResponse, error)

	// ClosePosition 市价全平指定持仓（posId 为持仓 ID）。
	ClosePosition(posId string) error

	// Platform 返回平台标识，便于日志与告警区分。
	Platform() string
}

type MarketKlineRequest struct {
	Symbol   string `json:"symbol"`
	InstID   string `json:"instId"`
	Interval string `json:"interval"`
	Limit    int    `json:"limit"`
}

type MarketTickerRequest struct {
	Symbol string `json:"symbol"`
	InstID string `json:"instId"`
}

type MarketTradeRequest struct {
	Symbol string `json:"symbol"`
	InstID string `json:"instId"`
	Limit  int    `json:"limit"`
}

type FundingRateRequest struct {
	Symbol string `json:"symbol"`
	InstID string `json:"instId"`
	// Limit 控制返回的历史资金费条数（含最新）。<=0 时由实现给默认值。
	Limit int `json:"limit"`
}

type BalanceRequest struct {
	AccountName string `json:"accountName"`
	InstType    string `json:"instType"`
	Ccy         string `json:"ccy"`
}

type ExchangeOrderRequest struct {
	AccountName  string `json:"accountName"`
	InstID       string `json:"instId"`
	Symbol       string `json:"symbol"`
	Side         string `json:"side"`
	PositionSide string `json:"positionSide"`
	OrderType    string `json:"orderType"`
	Size         string `json:"size"`
	Price        string `json:"price,omitempty"`
	TdMode       string `json:"tdMode,omitempty"`
	ReduceOnly   bool   `json:"reduceOnly,omitempty"`
}

type MarketKline struct {
	Platform    string `json:"platform"`
	Symbol      string `json:"symbol"`
	InstID      string `json:"instId"`
	Interval    string `json:"interval"`
	OpenTime    int64  `json:"openTime"`
	CloseTime   int64  `json:"closeTime"`
	OpenPrice   string `json:"openPrice"`
	HighPrice   string `json:"highPrice"`
	LowPrice    string `json:"lowPrice"`
	ClosePrice  string `json:"closePrice"`
	Volume      string `json:"volume"`
	QuoteVolume string `json:"quoteVolume"`
	TradeCount  int64  `json:"tradeCount,omitempty"`
	Source      string `json:"source"`
}

type MarketTicker struct {
	Platform    string `json:"platform"`
	Symbol      string `json:"symbol"`
	InstID      string `json:"instId"`
	Price       string `json:"price"`
	BidPrice    string `json:"bidPrice,omitempty"`
	AskPrice    string `json:"askPrice,omitempty"`
	HighPrice   string `json:"highPrice,omitempty"`
	LowPrice    string `json:"lowPrice,omitempty"`
	Volume      string `json:"volume,omitempty"`
	QuoteVolume string `json:"quoteVolume,omitempty"`
	UpdateTime  int64  `json:"updateTime,omitempty"`
	Source      string `json:"source"`
}

// MarketTrade 表示一笔（聚合）逐笔成交。
type MarketTrade struct {
	Platform     string `json:"platform"`
	Symbol       string `json:"symbol"`
	InstID       string `json:"instId"`
	TradeID      string `json:"tradeId,omitempty"`
	Price        string `json:"price"`
	Qty          string `json:"qty"`
	QuoteQty     string `json:"quoteQty,omitempty"`
	Timestamp    int64  `json:"timestamp"`
	IsBuyerMaker bool   `json:"isBuyerMaker"` // true=买方为挂单方（即本笔为主动卖出）
	Source       string `json:"source"`
}

// FundingRatePoint 表示一条资金费率历史。
type FundingRatePoint struct {
	FundingRate string `json:"fundingRate"`
	FundingTime int64  `json:"fundingTime"`
}

// FundingRateSnapshot 表示资金费率快照：最新一条 + 最近若干历史。
type FundingRateSnapshot struct {
	Platform        string             `json:"platform"`
	Symbol          string             `json:"symbol"`
	InstID          string             `json:"instId"`
	LastRate        string             `json:"lastRate"`           // 最新一期已结算资金费率
	NextRate        string             `json:"nextRate,omitempty"` // 预测下一期资金费率（如平台提供）
	NextFundingTime int64              `json:"nextFundingTime,omitempty"`
	History         []FundingRatePoint `json:"history,omitempty"`
	Source          string             `json:"source"`
}

type Balance struct {
	Platform  string `json:"platform"`
	Account   string `json:"account,omitempty"`
	Ccy       string `json:"ccy"`
	Total     string `json:"total"`
	Available string `json:"available"`
	Frozen    string `json:"frozen,omitempty"`
	Raw       any    `json:"raw,omitempty"`
}

type ExchangeOrderResponse struct {
	Platform string `json:"platform"`
	OrderID  string `json:"orderId,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
	Raw      any    `json:"raw,omitempty"`
}

// NewExchangeClient 工厂方法：根据 AccountConfig.Platform 返回对应实现。
// 调用方持有 ExchangeClient 接口，无需关心底层是 Deepcoin 还是 Binance。
func NewExchangeClient(acc AccountConfig, webClient *DirectWebClient) ExchangeClient {
	switch NormalizePlatform(acc.Platform) {
	case PlatformBinance:
		return newBinanceExchangeClient(acc)
	default:
		// 默认 deepcoin（兼容旧配置中未填 platform 字段的账户）
		return newDeepcoinExchangeClient(acc, webClient)
	}
}

// NewPlatformExchangeClient 按平台创建无账户客户端，适合 K 线/价格等公开行情查询。
func NewPlatformExchangeClient(platform string) (ExchangeClient, error) {
	acc := AccountConfig{Platform: NormalizePlatform(platform)}
	switch acc.Platform {
	case PlatformBinance:
		return newBinanceExchangeClient(acc), nil
	case PlatformDeepcoin:
		return newDeepcoinExchangeClient(acc, nil), nil
	default:
		return nil, fmt.Errorf("不支持的平台: %s", platform)
	}
}

func NormalizePlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "", "deepcoin", "deepcorn":
		return PlatformDeepcoin
	case "binance":
		return PlatformBinance
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func normalizeKlineLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func unsupportedPlatformFeature(platform, feature string) error {
	return fmt.Errorf("[%s] 暂未实现%s接口", platform, feature)
}
