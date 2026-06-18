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

	"common/utils"
)

const deepcoinPublicBaseURL = "https://api.deepcoin.com"

// deepcoinExchangeClient 通过 Deepcoin Web/OpenAPI 执行账户和交易能力。
type deepcoinExchangeClient struct {
	acc        AccountConfig
	apiClient  *utils.DeepCoinClient
	webClient  *DirectWebClient
	httpClient *http.Client
}

func newDeepcoinExchangeClient(acc AccountConfig, webClient *DirectWebClient) ExchangeClient {
	return &deepcoinExchangeClient{
		acc:        acc,
		apiClient:  utils.NewDeepCoinClient(acc.APIKey, acc.SecretKey, acc.Passphrase),
		webClient:  webClient,
		httpClient: &http.Client{Timeout: exchangeHTTPTimeout},
	}
}

func (c *deepcoinExchangeClient) Platform() string { return PlatformDeepcoin }

func (c *deepcoinExchangeClient) GetKlines(ctx context.Context, req MarketKlineRequest) ([]MarketKline, error) {
	instID := normalizeDeepcoinInstID(req.InstID, req.Symbol)
	if instID == "" {
		return nil, fmt.Errorf("[deepcoin] instId 不能为空")
	}
	interval := strings.TrimSpace(req.Interval)
	if interval == "" {
		interval = "1m"
	}

	params := url.Values{}
	params.Set("instId", instID)
	params.Set("bar", interval)
	params.Set("limit", strconv.Itoa(normalizeKlineLimit(req.Limit)))

	rawURL := fmt.Sprintf("%s/deepcoin/market/candles?%s", deepcoinPublicBaseURL, params.Encode())
	var raw struct {
		Code string            `json:"code"`
		Msg  string            `json:"msg"`
		Data []json.RawMessage `json:"data"`
	}
	if err := c.getJSON(ctx, rawURL, &raw); err != nil {
		return nil, err
	}
	if raw.Code != "" && raw.Code != "0" {
		return nil, fmt.Errorf("[deepcoin] K线查询失败: code=%s msg=%s", raw.Code, raw.Msg)
	}

	klines := make([]MarketKline, 0, len(raw.Data))
	for _, item := range raw.Data {
		kline, err := parseDeepcoinKline(item, instID, interval)
		if err != nil {
			return nil, err
		}
		klines = append(klines, kline)
	}
	return klines, nil
}

func (c *deepcoinExchangeClient) GetTicker(ctx context.Context, req MarketTickerRequest) (*MarketTicker, error) {
	instID := normalizeDeepcoinInstID(req.InstID, req.Symbol)
	if instID == "" {
		return nil, fmt.Errorf("[deepcoin] instId 不能为空")
	}

	params := url.Values{}
	params.Set("instType", "SWAP")

	rawURL := fmt.Sprintf("%s/deepcoin/market/tickers?%s", deepcoinPublicBaseURL, params.Encode())
	var raw struct {
		Code string                   `json:"code"`
		Msg  string                   `json:"msg"`
		Data []map[string]interface{} `json:"data"`
	}
	if err := c.getJSON(ctx, rawURL, &raw); err != nil {
		return nil, err
	}
	if raw.Code != "" && raw.Code != "0" {
		return nil, fmt.Errorf("[deepcoin] 实时价格查询失败: code=%s msg=%s", raw.Code, raw.Msg)
	}

	for _, item := range raw.Data {
		if !strings.EqualFold(fmt.Sprint(item["instId"]), instID) {
			continue
		}
		return &MarketTicker{
			Platform:    PlatformDeepcoin,
			Symbol:      instID,
			InstID:      instID,
			Price:       firstNonEmptyString(item, "last", "lastPx", "close", "price"),
			BidPrice:    firstNonEmptyString(item, "bidPx", "bidPrice", "bestBid"),
			AskPrice:    firstNonEmptyString(item, "askPx", "askPrice", "bestAsk"),
			HighPrice:   firstNonEmptyString(item, "high24h", "high"),
			LowPrice:    firstNonEmptyString(item, "low24h", "low"),
			Volume:      firstNonEmptyString(item, "vol24h", "vol"),
			QuoteVolume: firstNonEmptyString(item, "volCcy24h", "quoteVolume"),
			UpdateTime:  firstInt64String(item, "ts", "uTime", "time"),
			Source:      "deepcoin:/deepcoin/market/tickers",
		}, nil
	}
	return nil, fmt.Errorf("[deepcoin] 未找到交易对行情: %s", instID)
}

func (c *deepcoinExchangeClient) GetRecentTrades(ctx context.Context, req MarketTradeRequest) ([]MarketTrade, error) {
	return nil, unsupportedPlatformFeature(PlatformDeepcoin, "逐笔成交")
}

func (c *deepcoinExchangeClient) GetFundingRate(ctx context.Context, req FundingRateRequest) (*FundingRateSnapshot, error) {
	return nil, unsupportedPlatformFeature(PlatformDeepcoin, "资金费率")
}

func (c *deepcoinExchangeClient) GetBalances(ctx context.Context, req BalanceRequest) ([]Balance, error) {
	instType := strings.TrimSpace(req.InstType)
	if instType == "" {
		instType = "SWAP"
	}
	resp, err := c.apiClient.GetBalancesTyped(&utils.GetBalancesRequest{
		InstType: instType,
		Ccy:      strings.TrimSpace(req.Ccy),
	})
	if err != nil {
		return nil, err
	}

	balances := make([]Balance, 0, len(resp.Data))
	for _, item := range resp.Data {
		balances = append(balances, Balance{
			Platform:  PlatformDeepcoin,
			Account:   c.acc.Name,
			Ccy:       item.Ccy,
			Total:     item.Bal,
			Available: item.AvailBal,
			Frozen:    item.FrozenBal,
			Raw:       item,
		})
	}
	return balances, nil
}

func (c *deepcoinExchangeClient) PlaceOrder(ctx context.Context, req ExchangeOrderRequest) (*ExchangeOrderResponse, error) {
	instID := normalizeDeepcoinInstID(req.InstID, req.Symbol)
	if instID == "" {
		return nil, fmt.Errorf("[deepcoin] instId 不能为空")
	}
	orderType := strings.TrimSpace(req.OrderType)
	if orderType == "" {
		orderType = "market"
	}
	tdMode := strings.TrimSpace(req.TdMode)
	if tdMode == "" {
		tdMode = "cross"
	}

	resp, err := c.apiClient.PlaceOrderTyped(&utils.OrderRequest{
		InstId:      instID,
		TdMode:      tdMode,
		Side:        req.Side,
		OrdType:     orderType,
		Sz:          req.Size,
		Px:          req.Price,
		PosSide:     req.PositionSide,
		MrgPosition: "merge",
		ReduceOnly:  req.ReduceOnly,
	})
	if err != nil {
		return nil, err
	}

	return &ExchangeOrderResponse{
		Platform: PlatformDeepcoin,
		OrderID:  resp.Data.OrdId,
		Code:     resp.Data.SCode,
		Message:  resp.Data.SMsg,
		Raw:      resp,
	}, nil
}

func (c *deepcoinExchangeClient) OpenPosition(instId, side string, size int, price float64) (*utils.WebOrderResponse, error) {
	if c.webClient == nil {
		return nil, fmt.Errorf("[deepcoin] 账户 %s 无 Web 客户端，无法开仓", c.acc.Name)
	}

	const (
		lever         = 125
		isCrossMargin = 1
	)

	switch side {
	case "long":
		return c.webClient.MarketBuyLongWithRisk(instId, size, lever, isCrossMargin, c.acc.UID, price)
	case "short":
		return c.webClient.MarketSellShortWithRisk(instId, size, lever, isCrossMargin, c.acc.UID, price)
	default:
		return nil, fmt.Errorf("[deepcoin] 未知开仓方向: %s", side)
	}
}

func (c *deepcoinExchangeClient) ClosePosition(posId string) error {
	if c.webClient == nil {
		return fmt.Errorf("[deepcoin] 账户 %s 无 Web 客户端，无法平仓", c.acc.Name)
	}
	_, err := c.webClient.ClosePosition(posId)
	return err
}

func (c *deepcoinExchangeClient) getJSON(ctx context.Context, rawURL string, dst interface{}) error {
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
		return fmt.Errorf("[deepcoin] status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

func normalizeDeepcoinInstID(instID, symbol string) string {
	s := strings.TrimSpace(instID)
	if s == "" {
		s = strings.TrimSpace(symbol)
	}
	s = strings.ToUpper(s)
	if s == "BTCUSDT" {
		return "BTC-USDT-SWAP"
	}
	if strings.Contains(s, "-") {
		return s
	}
	if strings.HasSuffix(s, "USDT") {
		base := strings.TrimSuffix(s, "USDT")
		return base + "-USDT-SWAP"
	}
	return s
}

func parseDeepcoinKline(raw json.RawMessage, instID, interval string) (MarketKline, error) {
	var row []interface{}
	if err := json.Unmarshal(raw, &row); err == nil && len(row) >= 6 {
		openTime, _ := interfaceInt64(row[0])
		closeTime := int64(0)
		if len(row) > 6 {
			closeTime, _ = interfaceInt64(row[6])
		}
		return MarketKline{
			Platform:    PlatformDeepcoin,
			Symbol:      instID,
			InstID:      instID,
			Interval:    interval,
			OpenTime:    openTime,
			CloseTime:   closeTime,
			OpenPrice:   fmt.Sprint(row[1]),
			HighPrice:   fmt.Sprint(row[2]),
			LowPrice:    fmt.Sprint(row[3]),
			ClosePrice:  fmt.Sprint(row[4]),
			Volume:      fmt.Sprint(row[5]),
			QuoteVolume: optionalRowString(row, 7),
			Source:      "deepcoin:/deepcoin/market/candles",
		}, nil
	}

	var item map[string]interface{}
	if err := json.Unmarshal(raw, &item); err != nil {
		return MarketKline{}, fmt.Errorf("[deepcoin] K线解析失败: %w", err)
	}
	return MarketKline{
		Platform:    PlatformDeepcoin,
		Symbol:      instID,
		InstID:      instID,
		Interval:    interval,
		OpenTime:    firstInt64String(item, "ts", "openTime", "time"),
		CloseTime:   firstInt64String(item, "closeTime"),
		OpenPrice:   firstNonEmptyString(item, "open", "openPrice", "o"),
		HighPrice:   firstNonEmptyString(item, "high", "highPrice", "h"),
		LowPrice:    firstNonEmptyString(item, "low", "lowPrice", "l"),
		ClosePrice:  firstNonEmptyString(item, "close", "closePrice", "c"),
		Volume:      firstNonEmptyString(item, "vol", "volume", "v"),
		QuoteVolume: firstNonEmptyString(item, "quoteVol", "quoteVolume", "q"),
		Source:      "deepcoin:/deepcoin/market/candles",
	}, nil
}

func optionalRowString(row []interface{}, idx int) string {
	if idx >= 0 && idx < len(row) {
		return fmt.Sprint(row[idx])
	}
	return ""
}

func firstNonEmptyString(item map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := item[key]; ok && fmt.Sprint(value) != "" && fmt.Sprint(value) != "<nil>" {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func firstInt64String(item map[string]interface{}, keys ...string) int64 {
	for _, key := range keys {
		value, ok := item[key]
		if !ok {
			continue
		}
		if n, err := interfaceInt64(value); err == nil {
			return n
		}
	}
	return 0
}
