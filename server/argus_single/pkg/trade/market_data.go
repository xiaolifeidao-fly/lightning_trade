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
)

// The services that consume Argus use these DTOs for public market data. They
// intentionally stay separate from the BADelay account and order execution
// implementation that replaced the previous Argus runtime.
const (
	PlatformDeepcoin = "deepcoin"
	PlatformBinance  = "binance"

	binanceFuturesMarketBaseURL = "https://fapi.binance.com"
	deepcoinMarketBaseURL       = "https://api.deepcoin.com"
)

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
	Limit  int    `json:"limit"`
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

type MarketTrade struct {
	Platform     string `json:"platform"`
	Symbol       string `json:"symbol"`
	InstID       string `json:"instId"`
	TradeID      string `json:"tradeId,omitempty"`
	Price        string `json:"price"`
	Qty          string `json:"qty"`
	QuoteQty     string `json:"quoteQty,omitempty"`
	Timestamp    int64  `json:"timestamp"`
	IsBuyerMaker bool   `json:"isBuyerMaker"`
	Source       string `json:"source"`
}

type FundingRatePoint struct {
	FundingRate string `json:"fundingRate"`
	FundingTime int64  `json:"fundingTime"`
}

type FundingRateSnapshot struct {
	Platform        string             `json:"platform"`
	Symbol          string             `json:"symbol"`
	InstID          string             `json:"instId"`
	LastRate        string             `json:"lastRate"`
	NextRate        string             `json:"nextRate,omitempty"`
	NextFundingTime int64              `json:"nextFundingTime,omitempty"`
	History         []FundingRatePoint `json:"history,omitempty"`
	Source          string             `json:"source"`
}

type marketDataClient struct {
	httpClient      *http.Client
	binanceBaseURL  string
	deepcoinBaseURL string
}

func newMarketDataClient(httpClient *http.Client, binanceBaseURL, deepcoinBaseURL string) *marketDataClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &marketDataClient{
		httpClient:      httpClient,
		binanceBaseURL:  strings.TrimRight(binanceBaseURL, "/"),
		deepcoinBaseURL: strings.TrimRight(deepcoinBaseURL, "/"),
	}
}

func defaultMarketDataClient() *marketDataClient {
	return newMarketDataClient(nil, binanceFuturesMarketBaseURL, deepcoinMarketBaseURL)
}

// GetKlinesByPlatform queries publicly available futures candles. It does not
// need, load, or expose an Argus account session.
func GetKlinesByPlatform(ctx context.Context, platform string, req MarketKlineRequest) ([]MarketKline, error) {
	return defaultMarketDataClient().getKlines(ctx, platform, req)
}

func GetTickerByPlatform(ctx context.Context, platform string, req MarketTickerRequest) (*MarketTicker, error) {
	return defaultMarketDataClient().getTicker(ctx, platform, req)
}

func GetRecentTradesByPlatform(ctx context.Context, platform string, req MarketTradeRequest) ([]MarketTrade, error) {
	return defaultMarketDataClient().getRecentTrades(ctx, platform, req)
}

func GetFundingRateByPlatform(ctx context.Context, platform string, req FundingRateRequest) (*FundingRateSnapshot, error) {
	return defaultMarketDataClient().getFundingRate(ctx, platform, req)
}

func (c *marketDataClient) getKlines(ctx context.Context, platform string, req MarketKlineRequest) ([]MarketKline, error) {
	switch marketDataPlatform(platform) {
	case PlatformBinance:
		return c.getBinanceKlines(ctx, req)
	case PlatformDeepcoin:
		return c.getDeepcoinKlines(ctx, req)
	default:
		return nil, fmt.Errorf("不支持的行情平台: %s", strings.TrimSpace(platform))
	}
}

func (c *marketDataClient) getTicker(ctx context.Context, platform string, req MarketTickerRequest) (*MarketTicker, error) {
	switch marketDataPlatform(platform) {
	case PlatformBinance:
		return c.getBinanceTicker(ctx, req)
	case PlatformDeepcoin:
		return c.getDeepcoinTicker(ctx, req)
	default:
		return nil, fmt.Errorf("不支持的行情平台: %s", strings.TrimSpace(platform))
	}
}

func (c *marketDataClient) getRecentTrades(ctx context.Context, platform string, req MarketTradeRequest) ([]MarketTrade, error) {
	if marketDataPlatform(platform) != PlatformBinance {
		return nil, fmt.Errorf("[%s] 暂未实现公开逐笔成交接口", strings.TrimSpace(platform))
	}

	symbol := normalizeBinanceMarketSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	limit := normalizeMarketLimit(req.Limit, 500, 1000)
	params := url.Values{"symbol": {symbol}, "limit": {strconv.Itoa(limit)}}
	var rows []struct {
		AggID   int64  `json:"a"`
		Price   string `json:"p"`
		Qty     string `json:"q"`
		Time    int64  `json:"T"`
		IsMaker bool   `json:"m"`
	}
	if err := c.getJSON(ctx, c.binanceURL("/fapi/v1/aggTrades", params), &rows); err != nil {
		return nil, err
	}

	trades := make([]MarketTrade, 0, len(rows))
	for _, row := range rows {
		trades = append(trades, MarketTrade{
			Platform: PlatformBinance, Symbol: symbol, InstID: symbol,
			TradeID: strconv.FormatInt(row.AggID, 10), Price: row.Price, Qty: row.Qty,
			Timestamp: row.Time, IsBuyerMaker: row.IsMaker, Source: "binance-futures:/fapi/v1/aggTrades",
		})
	}
	return trades, nil
}

func (c *marketDataClient) getFundingRate(ctx context.Context, platform string, req FundingRateRequest) (*FundingRateSnapshot, error) {
	if marketDataPlatform(platform) != PlatformBinance {
		return nil, fmt.Errorf("[%s] 暂未实现公开资金费率接口", strings.TrimSpace(platform))
	}

	symbol := normalizeBinanceMarketSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	limit := normalizeMarketLimit(req.Limit, 8, 100)
	params := url.Values{"symbol": {symbol}, "limit": {strconv.Itoa(limit)}}
	var historyRows []struct {
		FundingRate string `json:"fundingRate"`
		FundingTime int64  `json:"fundingTime"`
	}
	if err := c.getJSON(ctx, c.binanceURL("/fapi/v1/fundingRate", params), &historyRows); err != nil {
		return nil, err
	}

	snapshot := &FundingRateSnapshot{
		Platform: PlatformBinance, Symbol: symbol, InstID: symbol,
		History: make([]FundingRatePoint, 0, len(historyRows)),
		Source:  "binance-futures:/fapi/v1/fundingRate+premiumIndex",
	}
	for _, row := range historyRows {
		snapshot.History = append(snapshot.History, FundingRatePoint{FundingRate: row.FundingRate, FundingTime: row.FundingTime})
	}
	if n := len(snapshot.History); n > 0 {
		snapshot.LastRate = snapshot.History[n-1].FundingRate
	}

	var premium struct {
		LastFundingRate string `json:"lastFundingRate"`
		NextFundingTime int64  `json:"nextFundingTime"`
	}
	if err := c.getJSON(ctx, c.binanceURL("/fapi/v1/premiumIndex", url.Values{"symbol": {symbol}}), &premium); err == nil {
		snapshot.NextRate = premium.LastFundingRate
		snapshot.NextFundingTime = premium.NextFundingTime
	}
	return snapshot, nil
}

func (c *marketDataClient) getBinanceKlines(ctx context.Context, req MarketKlineRequest) ([]MarketKline, error) {
	symbol := normalizeBinanceMarketSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	interval := marketDataInterval(req.Interval)
	params := url.Values{
		"symbol":   {symbol},
		"interval": {interval},
		"limit":    {strconv.Itoa(normalizeMarketLimit(req.Limit, 100, 1500))},
	}
	var rows [][]any
	if err := c.getJSON(ctx, c.binanceURL("/fapi/v1/klines", params), &rows); err != nil {
		return nil, err
	}

	klines := make([]MarketKline, 0, len(rows))
	for _, row := range rows {
		kline, err := parseBinanceMarketKline(row, symbol, interval)
		if err != nil {
			return nil, err
		}
		klines = append(klines, kline)
	}
	return klines, nil
}

func (c *marketDataClient) getBinanceTicker(ctx context.Context, req MarketTickerRequest) (*MarketTicker, error) {
	symbol := normalizeBinanceMarketSymbol(req.Symbol, req.InstID)
	if symbol == "" {
		return nil, fmt.Errorf("[binance] symbol 不能为空")
	}
	var row struct {
		Symbol      string `json:"symbol"`
		LastPrice   string `json:"lastPrice"`
		BidPrice    string `json:"bidPrice"`
		AskPrice    string `json:"askPrice"`
		HighPrice   string `json:"highPrice"`
		LowPrice    string `json:"lowPrice"`
		Volume      string `json:"volume"`
		QuoteVolume string `json:"quoteVolume"`
		CloseTime   int64  `json:"closeTime"`
	}
	if err := c.getJSON(ctx, c.binanceURL("/fapi/v1/ticker/24hr", url.Values{"symbol": {symbol}}), &row); err != nil {
		return nil, err
	}
	return &MarketTicker{
		Platform: PlatformBinance, Symbol: row.Symbol, InstID: row.Symbol, Price: row.LastPrice,
		BidPrice: row.BidPrice, AskPrice: row.AskPrice, HighPrice: row.HighPrice, LowPrice: row.LowPrice,
		Volume: row.Volume, QuoteVolume: row.QuoteVolume, UpdateTime: row.CloseTime,
		Source: "binance-futures:/fapi/v1/ticker/24hr",
	}, nil
}

func (c *marketDataClient) getDeepcoinKlines(ctx context.Context, req MarketKlineRequest) ([]MarketKline, error) {
	instID := normalizeDeepcoinMarketInstID(req.InstID, req.Symbol)
	if instID == "" {
		return nil, fmt.Errorf("[deepcoin] instId 不能为空")
	}
	interval := marketDataInterval(req.Interval)
	// DeepCoin 的 bar 走 OKX 口径：分钟小写、小时/天/周大写，1h/1d 直接透传会被判为非法周期。
	params := url.Values{
		"instId": {instID},
		"bar":    {deepcoinMarketBar(interval)},
		"limit":  {strconv.Itoa(normalizeMarketLimit(req.Limit, 100, 1000))},
	}
	var payload struct {
		Code string            `json:"code"`
		Msg  string            `json:"msg"`
		Data []json.RawMessage `json:"data"`
	}
	if err := c.getJSON(ctx, c.deepcoinURL("/deepcoin/market/candles", params), &payload); err != nil {
		return nil, err
	}
	if payload.Code != "" && payload.Code != "0" {
		return nil, fmt.Errorf("[deepcoin] K线查询失败: code=%s msg=%s", payload.Code, payload.Msg)
	}

	klines := make([]MarketKline, 0, len(payload.Data))
	for _, raw := range payload.Data {
		kline, err := parseDeepcoinMarketKline(raw, instID, interval)
		if err != nil {
			return nil, err
		}
		klines = append(klines, kline)
	}
	return klines, nil
}

func (c *marketDataClient) getDeepcoinTicker(ctx context.Context, req MarketTickerRequest) (*MarketTicker, error) {
	instID := normalizeDeepcoinMarketInstID(req.InstID, req.Symbol)
	if instID == "" {
		return nil, fmt.Errorf("[deepcoin] instId 不能为空")
	}
	var payload struct {
		Code string           `json:"code"`
		Msg  string           `json:"msg"`
		Data []map[string]any `json:"data"`
	}
	if err := c.getJSON(ctx, c.deepcoinURL("/deepcoin/market/tickers", url.Values{"instType": {"SWAP"}}), &payload); err != nil {
		return nil, err
	}
	if payload.Code != "" && payload.Code != "0" {
		return nil, fmt.Errorf("[deepcoin] 实时价格查询失败: code=%s msg=%s", payload.Code, payload.Msg)
	}
	for _, row := range payload.Data {
		if !strings.EqualFold(marketDataString(row["instId"]), instID) {
			continue
		}
		return &MarketTicker{
			Platform: PlatformDeepcoin, Symbol: instID, InstID: instID,
			Price:       marketDataFirstString(row, "last", "lastPx", "close", "price"),
			BidPrice:    marketDataFirstString(row, "bidPx", "bidPrice", "bestBid"),
			AskPrice:    marketDataFirstString(row, "askPx", "askPrice", "bestAsk"),
			HighPrice:   marketDataFirstString(row, "high24h", "high"),
			LowPrice:    marketDataFirstString(row, "low24h", "low"),
			Volume:      marketDataFirstString(row, "vol24h", "vol"),
			QuoteVolume: marketDataFirstString(row, "volCcy24h", "quoteVolume"),
			UpdateTime:  marketDataFirstInt64(row, "ts", "uTime", "time"),
			Source:      "deepcoin:/deepcoin/market/tickers",
		}, nil
	}
	return nil, fmt.Errorf("[deepcoin] 未找到交易对行情: %s", instID)
}

func (c *marketDataClient) getJSON(ctx context.Context, rawURL string, dst any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("行情请求失败: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		return fmt.Errorf("解析行情响应失败: %w", err)
	}
	return nil
}

func (c *marketDataClient) binanceURL(path string, params url.Values) string {
	return c.binanceBaseURL + path + "?" + params.Encode()
}

func (c *marketDataClient) deepcoinURL(path string, params url.Values) string {
	return c.deepcoinBaseURL + path + "?" + params.Encode()
}

func marketDataPlatform(platform string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "", PlatformDeepcoin, "deepcorn":
		return PlatformDeepcoin
	case PlatformBinance:
		return PlatformBinance
	default:
		return strings.ToLower(strings.TrimSpace(platform))
	}
}

func marketDataInterval(interval string) string {
	if value := strings.TrimSpace(interval); value != "" {
		return value
	}
	return "1m"
}

// deepcoinMarketBar 把统一周期串转成 DeepCoin(OKX 口径)的 bar 参数。
// 分钟保持小写(1m=1分钟)，小时/天/周需大写(1H/1D/1W)；未知周期原样透传，交由服务端报错。
func deepcoinMarketBar(interval string) string {
	switch strings.TrimSpace(interval) {
	case "1h", "2h", "4h", "6h", "12h":
		return strings.ToUpper(strings.TrimSpace(interval))
	case "8h":
		return "8H"
	case "1d":
		return "1D"
	case "1w":
		return "1W"
	default:
		return strings.TrimSpace(interval)
	}
}

func normalizeMarketLimit(limit, defaultLimit, maxLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func normalizeBinanceMarketSymbol(symbol, instID string) string {
	value := strings.TrimSpace(symbol)
	if value == "" {
		value = strings.TrimSpace(instID)
	}
	value = strings.ToUpper(strings.ReplaceAll(value, "-", ""))
	return strings.TrimSuffix(value, "SWAP")
}

func normalizeDeepcoinMarketInstID(instID, symbol string) string {
	value := strings.ToUpper(strings.TrimSpace(instID))
	if value == "" {
		value = strings.ToUpper(strings.TrimSpace(symbol))
	}
	if value == "" {
		return ""
	}
	if strings.Contains(value, "-") {
		return value
	}
	if strings.HasSuffix(value, "USDT") {
		return strings.TrimSuffix(value, "USDT") + "-USDT-SWAP"
	}
	return value
}

func parseBinanceMarketKline(row []any, symbol, interval string) (MarketKline, error) {
	if len(row) < 9 {
		return MarketKline{}, fmt.Errorf("[binance] K线行长度异常: %d", len(row))
	}
	openTime, err := marketDataInt64(row[0])
	if err != nil {
		return MarketKline{}, fmt.Errorf("[binance] openTime 解析失败: %w", err)
	}
	closeTime, err := marketDataInt64(row[6])
	if err != nil {
		return MarketKline{}, fmt.Errorf("[binance] closeTime 解析失败: %w", err)
	}
	tradeCount, _ := marketDataInt64(row[8])
	return MarketKline{
		Platform: PlatformBinance, Symbol: symbol, InstID: symbol, Interval: interval,
		OpenTime: openTime, CloseTime: closeTime, OpenPrice: marketDataString(row[1]),
		HighPrice: marketDataString(row[2]), LowPrice: marketDataString(row[3]), ClosePrice: marketDataString(row[4]),
		Volume: marketDataString(row[5]), QuoteVolume: marketDataString(row[7]), TradeCount: tradeCount,
		Source: "binance-futures:/fapi/v1/klines",
	}, nil
}

func parseDeepcoinMarketKline(raw json.RawMessage, instID, interval string) (MarketKline, error) {
	var row []any
	if err := json.Unmarshal(raw, &row); err == nil && len(row) >= 6 {
		openTime, err := marketDataInt64(row[0])
		if err != nil {
			return MarketKline{}, fmt.Errorf("[deepcoin] openTime 解析失败: %w", err)
		}
		closeTime := int64(0)
		if len(row) > 6 {
			closeTime, _ = marketDataInt64(row[6])
		}
		return MarketKline{
			Platform: PlatformDeepcoin, Symbol: instID, InstID: instID, Interval: interval,
			OpenTime: openTime, CloseTime: closeTime, OpenPrice: marketDataString(row[1]),
			HighPrice: marketDataString(row[2]), LowPrice: marketDataString(row[3]), ClosePrice: marketDataString(row[4]),
			Volume: marketDataString(row[5]), QuoteVolume: marketDataOptionalRowString(row, 7),
			Source: "deepcoin:/deepcoin/market/candles",
		}, nil
	}
	var item map[string]any
	if err := json.Unmarshal(raw, &item); err != nil {
		return MarketKline{}, fmt.Errorf("[deepcoin] K线解析失败: %w", err)
	}
	return MarketKline{
		Platform: PlatformDeepcoin, Symbol: instID, InstID: instID, Interval: interval,
		OpenTime: marketDataFirstInt64(item, "ts", "openTime", "time"), CloseTime: marketDataFirstInt64(item, "closeTime"),
		OpenPrice:   marketDataFirstString(item, "open", "openPrice", "o"),
		HighPrice:   marketDataFirstString(item, "high", "highPrice", "h"),
		LowPrice:    marketDataFirstString(item, "low", "lowPrice", "l"),
		ClosePrice:  marketDataFirstString(item, "close", "closePrice", "c"),
		Volume:      marketDataFirstString(item, "vol", "volume", "v"),
		QuoteVolume: marketDataFirstString(item, "quoteVol", "quoteVolume", "q"),
		Source:      "deepcoin:/deepcoin/market/candles",
	}, nil
}

func marketDataInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	case int:
		return int64(typed), nil
	case json.Number:
		return typed.Int64()
	case string:
		return strconv.ParseInt(typed, 10, 64)
	default:
		return 0, fmt.Errorf("不支持的时间类型 %T", value)
	}
}

func marketDataString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func marketDataOptionalRowString(row []any, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return marketDataString(row[index])
}

func marketDataFirstString(item map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := marketDataString(item[key]); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func marketDataFirstInt64(item map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := item[key]; ok {
			if parsed, err := marketDataInt64(value); err == nil {
				return parsed
			}
		}
	}
	return 0
}
