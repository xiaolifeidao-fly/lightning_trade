package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"common/utils"
)

const (
	binanceFuturesBaseURL = "https://fapi.binance.com"
	exchangeHTTPTimeout   = 10 * time.Second
)

// binanceExchangeClient 币安交易所实现。
type binanceExchangeClient struct {
	acc        AccountConfig
	httpClient *http.Client
}

func newBinanceExchangeClient(acc AccountConfig) ExchangeClient {
	return &binanceExchangeClient{
		acc:        acc,
		httpClient: &http.Client{Timeout: exchangeHTTPTimeout},
	}
}

func (c *binanceExchangeClient) Platform() string { return PlatformBinance }

func (c *binanceExchangeClient) GetKlines(ctx context.Context, req MarketKlineRequest) ([]MarketKline, error) {
	symbol := normalizeBinanceSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	interval := strings.TrimSpace(req.Interval)
	if interval == "" {
		interval = "1m"
	}

	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("interval", interval)
	params.Set("limit", strconv.Itoa(normalizeKlineLimit(req.Limit)))

	rawURL := fmt.Sprintf("%s/fapi/v1/klines?%s", binanceFuturesBaseURL, params.Encode())
	var rows [][]interface{}
	if err := c.getJSON(ctx, rawURL, &rows); err != nil {
		return nil, err
	}

	klines := make([]MarketKline, 0, len(rows))
	for _, row := range rows {
		kline, err := parseBinanceKlineRow(row, symbol, interval)
		if err != nil {
			return nil, err
		}
		klines = append(klines, kline)
	}
	return klines, nil
}

func (c *binanceExchangeClient) GetTicker(ctx context.Context, req MarketTickerRequest) (*MarketTicker, error) {
	symbol := normalizeBinanceSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}

	params := url.Values{}
	params.Set("symbol", symbol)

	rawURL := fmt.Sprintf("%s/fapi/v1/ticker/24hr?%s", binanceFuturesBaseURL, params.Encode())
	var ticker struct {
		Symbol             string `json:"symbol"`
		LastPrice          string `json:"lastPrice"`
		BidPrice           string `json:"bidPrice"`
		AskPrice           string `json:"askPrice"`
		HighPrice          string `json:"highPrice"`
		LowPrice           string `json:"lowPrice"`
		Volume             string `json:"volume"`
		QuoteVolume        string `json:"quoteVolume"`
		CloseTime          int64  `json:"closeTime"`
		PriceChange        string `json:"priceChange"`
		PriceChangePercent string `json:"priceChangePercent"`
	}
	if err := c.getJSON(ctx, rawURL, &ticker); err != nil {
		return nil, err
	}

	return &MarketTicker{
		Platform:    PlatformBinance,
		Symbol:      ticker.Symbol,
		InstID:      ticker.Symbol,
		Price:       ticker.LastPrice,
		BidPrice:    ticker.BidPrice,
		AskPrice:    ticker.AskPrice,
		HighPrice:   ticker.HighPrice,
		LowPrice:    ticker.LowPrice,
		Volume:      ticker.Volume,
		QuoteVolume: ticker.QuoteVolume,
		UpdateTime:  ticker.CloseTime,
		Source:      "binance-futures:/fapi/v1/ticker/24hr",
	}, nil
}

func (c *binanceExchangeClient) GetRecentTrades(ctx context.Context, req MarketTradeRequest) ([]MarketTrade, error) {
	symbol := normalizeBinanceSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}

	params := url.Values{}
	params.Set("symbol", symbol)
	params.Set("limit", strconv.Itoa(limit))

	rawURL := fmt.Sprintf("%s/fapi/v1/aggTrades?%s", binanceFuturesBaseURL, params.Encode())
	var rows []struct {
		AggID   int64  `json:"a"`
		Price   string `json:"p"`
		Qty     string `json:"q"`
		Time    int64  `json:"T"`
		IsMaker bool   `json:"m"`
	}
	if err := c.getJSON(ctx, rawURL, &rows); err != nil {
		return nil, err
	}

	trades := make([]MarketTrade, 0, len(rows))
	for _, r := range rows {
		trades = append(trades, MarketTrade{
			Platform:     PlatformBinance,
			Symbol:       symbol,
			InstID:       symbol,
			TradeID:      strconv.FormatInt(r.AggID, 10),
			Price:        r.Price,
			Qty:          r.Qty,
			Timestamp:    r.Time,
			IsBuyerMaker: r.IsMaker,
			Source:       "binance-futures:/fapi/v1/aggTrades",
		})
	}
	return trades, nil
}

func (c *binanceExchangeClient) GetFundingRate(ctx context.Context, req FundingRateRequest) (*FundingRateSnapshot, error) {
	symbol := normalizeBinanceSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 100 {
		limit = 100
	}

	// 历史资金费
	histParams := url.Values{}
	histParams.Set("symbol", symbol)
	histParams.Set("limit", strconv.Itoa(limit))
	histURL := fmt.Sprintf("%s/fapi/v1/fundingRate?%s", binanceFuturesBaseURL, histParams.Encode())
	var histRows []struct {
		FundingRate string `json:"fundingRate"`
		FundingTime int64  `json:"fundingTime"`
	}
	if err := c.getJSON(ctx, histURL, &histRows); err != nil {
		return nil, err
	}

	snapshot := &FundingRateSnapshot{
		Platform: PlatformBinance,
		Symbol:   symbol,
		InstID:   symbol,
		History:  make([]FundingRatePoint, 0, len(histRows)),
		Source:   "binance-futures:/fapi/v1/fundingRate+premiumIndex",
	}
	for _, r := range histRows {
		snapshot.History = append(snapshot.History, FundingRatePoint{
			FundingRate: r.FundingRate,
			FundingTime: r.FundingTime,
		})
	}
	if n := len(snapshot.History); n > 0 {
		snapshot.LastRate = snapshot.History[n-1].FundingRate
	}

	// 预测下一期资金费（premiumIndex 提供 lastFundingRate / nextFundingTime）
	pmParams := url.Values{}
	pmParams.Set("symbol", symbol)
	pmURL := fmt.Sprintf("%s/fapi/v1/premiumIndex?%s", binanceFuturesBaseURL, pmParams.Encode())
	var premium struct {
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
	}
	if err := c.getJSON(ctx, pmURL, &premium); err == nil {
		snapshot.NextRate = premium.LastFundingRate
		snapshot.NextFundingTime = premium.NextFundingTime
	}

	return snapshot, nil
}

func (c *binanceExchangeClient) GetBalances(ctx context.Context, req BalanceRequest) ([]Balance, error) {
	return nil, unsupportedPlatformFeature(PlatformBinance, "余额")
}

func (c *binanceExchangeClient) PlaceOrder(ctx context.Context, req ExchangeOrderRequest) (*ExchangeOrderResponse, error) {
	return nil, unsupportedPlatformFeature(PlatformBinance, "下单")
}

// OpenPosition 待对接币安 Futures OpenAPI（POST /fapi/v1/order）。
func (c *binanceExchangeClient) OpenPosition(instId, side string, size int, price float64) (*utils.WebOrderResponse, error) {
	return nil, fmt.Errorf("[binance] 账户 %s 建仓接口暂未实现（待对接 OpenAPI）", c.acc.Name)
}

// ClosePosition 待对接币安 Futures OpenAPI（平仓）。
func (c *binanceExchangeClient) ClosePosition(posId string) error {
	return fmt.Errorf("[binance] 账户 %s 平仓接口暂未实现（待对接 OpenAPI）", c.acc.Name)
}

func (c *binanceExchangeClient) getJSON(ctx context.Context, rawURL string, dst interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("[binance] status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

func normalizeBinanceSymbol(symbol, instID string) string {
	s := strings.TrimSpace(symbol)
	if s == "" {
		s = strings.TrimSpace(instID)
	}
	s = strings.ToUpper(strings.ReplaceAll(s, "-", ""))
	s = strings.TrimSuffix(s, "SWAP")
	return s
}

func parseBinanceKlineRow(row []interface{}, symbol, interval string) (MarketKline, error) {
	if len(row) < 9 {
		return MarketKline{}, fmt.Errorf("[binance] kline row 长度异常: %d", len(row))
	}

	openTime, err := interfaceInt64(row[0])
	if err != nil {
		return MarketKline{}, fmt.Errorf("[binance] openTime 解析失败: %w", err)
	}
	closeTime, err := interfaceInt64(row[6])
	if err != nil {
		return MarketKline{}, fmt.Errorf("[binance] closeTime 解析失败: %w", err)
	}
	tradeCount, _ := interfaceInt64(row[8])

	return MarketKline{
		Platform:    PlatformBinance,
		Symbol:      symbol,
		InstID:      symbol,
		Interval:    interval,
		OpenTime:    openTime,
		CloseTime:   closeTime,
		OpenPrice:   fmt.Sprint(row[1]),
		HighPrice:   fmt.Sprint(row[2]),
		LowPrice:    fmt.Sprint(row[3]),
		ClosePrice:  fmt.Sprint(row[4]),
		Volume:      fmt.Sprint(row[5]),
		QuoteVolume: fmt.Sprint(row[7]),
		TradeCount:  tradeCount,
		Source:      "binance-futures:/fapi/v1/klines",
	}, nil
}

func interfaceInt64(v interface{}) (int64, error) {
	switch x := v.(type) {
	case float64:
		return int64(x), nil
	case int64:
		return x, nil
	case int:
		return int64(x), nil
	case json.Number:
		return x.Int64()
	case string:
		return strconv.ParseInt(x, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported type %T", v)
	}
}
