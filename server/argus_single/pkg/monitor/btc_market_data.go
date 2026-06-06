package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

const (
	BTCSymbol                 = "BTCUSDT"
	binanceRESTBaseURL        = "https://api.binance.com"
	binanceWebSocketBaseURL   = "wss://stream.binance.com:9443/stream"
	binanceHTTPTimeout        = 10 * time.Second
	binanceReconnectDelay     = 3 * time.Second
	btcMarketDefaultKlineSize = 240
)

var (
	btcMarketDataOnce sync.Once
	btcMarketDataFeed *BTCMarketDataFeed
)

// BTCKline 表示一根 BTC K 线。
type BTCKline struct {
	Symbol        string
	Interval      string
	OpenTime      string
	CloseTime     string
	OpenPrice     string
	HighPrice     string
	LowPrice      string
	ClosePrice    string
	Volume        string
	QuoteVolume   string
	TradeCount    int64
	Source        string
	RawPayloadRef string
}

// BTCTodayInfo 表示当天 BTC 的聚合信息。
type BTCTodayInfo struct {
	Symbol             string
	TradingDate        string
	CurrentPrice       string
	TodayOpenPrice     string
	TodayHighPrice     string
	TodayLowPrice      string
	TodayClosePrice    string
	TodayVolume        string
	TodayQuoteVolume   string
	TodayChangeValue   string
	TodayChangePercent string
	Source             string
}

// BTCMovingAverage 表示均线结果。
type BTCMovingAverage struct {
	Symbol    string
	Interval  string
	Period    int
	Value     string
	Source    string
	Reference string
}

// BTCMACD 表示 MACD 指标结果。
type BTCMACD struct {
	Symbol       string
	Interval     string
	FastPeriod   int
	SlowPeriod   int
	SignalPeriod int
	MACD         string
	Signal       string
	Histogram    string
	Source       string
}

// BTCRSI 表示 RSI 指标结果。
type BTCRSI struct {
	Symbol   string
	Interval string
	Period   int
	Value    string
	Source   string
}

// BTCATR 表示 ATR 指标结果。
type BTCATR struct {
	Symbol   string
	Interval string
	Period   int
	Value    string
	Source   string
}

// BTCBollingerBands 表示布林带结果。
type BTCBollingerBands struct {
	Symbol     string
	Interval   string
	Period     int
	StdDev     string
	MiddleBand string
	UpperBand  string
	LowerBand  string
	BandWidth  string
	Source     string
}

// BTCVolumeProfile 表示成交量相关快照。
type BTCVolumeProfile struct {
	Symbol        string
	Interval      string
	CurrentVolume string
	AverageVolume string
	QuoteVolume   string
	VolumeRatio   string
	Source        string
}

// BTCOpenInterest 表示未平仓量快照。
type BTCOpenInterest struct {
	Symbol        string
	Value         string
	Change24h     string
	ChangePercent string
	Source        string
}

// BTCFundingRate 表示资金费率快照。
type BTCFundingRate struct {
	Symbol        string
	CurrentRate   string
	NextFundingAt string
	Source        string
}

// BTCLongShortRatio 表示多空比快照。
type BTCLongShortRatio struct {
	Symbol     string
	Interval   string
	LongRatio  string
	ShortRatio string
	Ratio      string
	Source     string
}

// BTCAnalysisSnapshot 表示给 AI 平仓使用的 BTC 分析快照。
type BTCAnalysisSnapshot struct {
	Symbol           string
	Klines1H         []BTCKline
	Klines4H         []BTCKline
	Klines1D         []BTCKline
	TodayInfo        *BTCTodayInfo
	EMA1H20          *BTCMovingAverage
	EMA1H50          *BTCMovingAverage
	EMA4H20          *BTCMovingAverage
	EMA4H50          *BTCMovingAverage
	EMA1D20          *BTCMovingAverage
	EMA1D50          *BTCMovingAverage
	MA1D200          *BTCMovingAverage
	MACD1H           *BTCMACD
	MACD4H           *BTCMACD
	MACD1D           *BTCMACD
	RSI1H14          *BTCRSI
	RSI4H14          *BTCRSI
	RSI1D14          *BTCRSI
	ATR1H14          *BTCATR
	ATR4H14          *BTCATR
	ATR1D14          *BTCATR
	Bollinger1H      *BTCBollingerBands
	Bollinger4H      *BTCBollingerBands
	Bollinger1D      *BTCBollingerBands
	Volume1H         *BTCVolumeProfile
	Volume4H         *BTCVolumeProfile
	Volume1D         *BTCVolumeProfile
	OpenInterest     *BTCOpenInterest
	FundingRate      *BTCFundingRate
	LongShortRatio1H *BTCLongShortRatio
	LongShortRatio4H *BTCLongShortRatio
	LongShortRatio1D *BTCLongShortRatio
}

type binance24hrTicker struct {
	EventType          string `json:"e"`
	EventTime          int64  `json:"E"`
	Symbol             string `json:"s"`
	PriceChange        string `json:"p"`
	PriceChangePercent string `json:"P"`
	WeightedAvgPrice   string `json:"w"`
	PrevClosePrice     string `json:"x"`
	LastPrice          string `json:"c"`
	LastQty            string `json:"Q"`
	BestBidPrice       string `json:"b"`
	BestBidQty         string `json:"B"`
	BestAskPrice       string `json:"a"`
	BestAskQty         string `json:"A"`
	OpenPrice          string `json:"o"`
	HighPrice          string `json:"h"`
	LowPrice           string `json:"l"`
	Volume             string `json:"v"`
	QuoteVolume        string `json:"q"`
	OpenTime           int64  `json:"O"`
	CloseTime          int64  `json:"C"`
	FirstTradeID       int64  `json:"F"`
	LastTradeID        int64  `json:"L"`
	TradeCount         int64  `json:"n"`

	RESTSymbol             string `json:"symbol"`
	RESTPriceChange        string `json:"priceChange"`
	RESTPriceChangePercent string `json:"priceChangePercent"`
	RESTWeightedAvgPrice   string `json:"weightedAvgPrice"`
	RESTPrevClosePrice     string `json:"prevClosePrice"`
	RESTLastPrice          string `json:"lastPrice"`
	RESTLastQty            string `json:"lastQty"`
	RESTBestBidPrice       string `json:"bidPrice"`
	RESTBestBidQty         string `json:"bidQty"`
	RESTBestAskPrice       string `json:"askPrice"`
	RESTBestAskQty         string `json:"askQty"`
	RESTOpenPrice          string `json:"openPrice"`
	RESTHighPrice          string `json:"highPrice"`
	RESTLowPrice           string `json:"lowPrice"`
	RESTVolume             string `json:"volume"`
	RESTQuoteVolume        string `json:"quoteVolume"`
	RESTOpenTime           int64  `json:"openTime"`
	RESTCloseTime          int64  `json:"closeTime"`
	RESTFirstTradeID       int64  `json:"firstId"`
	RESTLastTradeID        int64  `json:"lastId"`
	RESTTradeCount         int64  `json:"count"`
}

type binanceRollingTicker struct {
	EventType          string `json:"e"`
	EventTime          int64  `json:"E"`
	Symbol             string `json:"s"`
	PriceChange        string `json:"p"`
	PriceChangePercent string `json:"P"`
	OpenPrice          string `json:"o"`
	HighPrice          string `json:"h"`
	LowPrice           string `json:"l"`
	LastPrice          string `json:"c"`
	WeightedAvgPrice   string `json:"w"`
	Volume             string `json:"v"`
	QuoteVolume        string `json:"q"`
	OpenTime           int64  `json:"O"`
	CloseTime          int64  `json:"C"`
	FirstTradeID       int64  `json:"F"`
	LastTradeID        int64  `json:"L"`
	TradeCount         int64  `json:"n"`
}

type binanceAvgPrice struct {
	EventType     string `json:"e"`
	EventTime     int64  `json:"E"`
	Symbol        string `json:"s"`
	Interval      string `json:"i"`
	WeightedPrice string `json:"w"`
	LastTradeTime int64  `json:"T"`
}

type binanceBookTicker struct {
	UpdateID   int64  `json:"u"`
	Symbol     string `json:"s"`
	BestBid    string `json:"b"`
	BestBidQty string `json:"B"`
	BestAsk    string `json:"a"`
	BestAskQty string `json:"A"`
}

type binanceKlineStreamEnvelope struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

type binanceKlineEvent struct {
	EventType string             `json:"e"`
	EventTime int64              `json:"E"`
	Symbol    string             `json:"s"`
	Kline     binanceKlineStream `json:"k"`
}

type binanceKlineStream struct {
	StartTime      int64  `json:"t"`
	CloseTime      int64  `json:"T"`
	Symbol         string `json:"s"`
	Interval       string `json:"i"`
	FirstTradeID   int64  `json:"f"`
	LastTradeID    int64  `json:"L"`
	OpenPrice      string `json:"o"`
	ClosePrice     string `json:"c"`
	HighPrice      string `json:"h"`
	LowPrice       string `json:"l"`
	Volume         string `json:"v"`
	TradeCount     int64  `json:"n"`
	IsClosed       bool   `json:"x"`
	QuoteVolume    string `json:"q"`
	TakerBuyVolume string `json:"V"`
	TakerBuyQuote  string `json:"Q"`
}

type binanceRESTAvgPrice struct {
	Mins      int    `json:"mins"`
	Price     string `json:"price"`
	CloseTime int64  `json:"closeTime"`
}

type BTCMarketDataFeed struct {
	httpClient *http.Client

	mu             sync.RWMutex
	klines         map[string][]BTCKline
	ticker24h      *binance24hrTicker
	rollingTickers map[string]*binanceRollingTicker
	avgPrice       *binanceAvgPrice
	bookTicker     *binanceBookTicker

	started bool
}

func StartBTCMarketDataFeed() {
	getOrCreateBTCMarketDataFeed().Start()
}

func getOrCreateBTCMarketDataFeed() *BTCMarketDataFeed {
	btcMarketDataOnce.Do(func() {
		btcMarketDataFeed = &BTCMarketDataFeed{
			httpClient: &http.Client{Timeout: binanceHTTPTimeout},
			klines: map[string][]BTCKline{
				"1h": make([]BTCKline, 0, btcMarketDefaultKlineSize),
				"4h": make([]BTCKline, 0, btcMarketDefaultKlineSize),
				"1d": make([]BTCKline, 0, btcMarketDefaultKlineSize),
			},
			rollingTickers: make(map[string]*binanceRollingTicker),
		}
	})
	return btcMarketDataFeed
}

func (f *BTCMarketDataFeed) Start() {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return
	}
	f.started = true
	f.mu.Unlock()

	if err := f.bootstrap(); err != nil {
		logrus.Warnf("[BTC行情] Binance bootstrap失败: %v", err)
	}

	go f.runWebSocketLoop()
	logrus.Infof("[BTC行情] Binance BTC 行情数据服务已启动")
}

func (f *BTCMarketDataFeed) bootstrap() error {
	if err := f.bootstrapKlines("1h", 200); err != nil {
		return err
	}
	if err := f.bootstrapKlines("4h", 200); err != nil {
		return err
	}
	if err := f.bootstrapKlines("1d", 200); err != nil {
		return err
	}
	if err := f.bootstrap24hrTicker(); err != nil {
		return err
	}
	if err := f.bootstrapAvgPrice(); err != nil {
		return err
	}
	return nil
}

func (f *BTCMarketDataFeed) bootstrapKlines(interval string, limit int) error {
	params := url.Values{}
	params.Set("symbol", BTCSymbol)
	params.Set("interval", interval)
	params.Set("limit", strconv.Itoa(limit))

	rawURL := fmt.Sprintf("%s/api/v3/klines?%s", binanceRESTBaseURL, params.Encode())
	var rows [][]interface{}
	if err := f.getJSON(rawURL, &rows); err != nil {
		return fmt.Errorf("bootstrap klines %s failed: %w", interval, err)
	}

	klines := make([]BTCKline, 0, len(rows))
	for _, row := range rows {
		kline, err := parseRESTKlineRow(row, interval)
		if err != nil {
			return fmt.Errorf("parse klines %s failed: %w", interval, err)
		}
		klines = append(klines, kline)
	}

	f.mu.Lock()
	f.klines[interval] = trimKlines(klines, btcMarketDefaultKlineSize)
	f.mu.Unlock()

	return nil
}

func (f *BTCMarketDataFeed) bootstrap24hrTicker() error {
	rawURL := fmt.Sprintf("%s/api/v3/ticker/24hr?symbol=%s", binanceRESTBaseURL, BTCSymbol)
	var ticker binance24hrTicker
	if err := f.getJSON(rawURL, &ticker); err != nil {
		return fmt.Errorf("bootstrap 24hr ticker failed: %w", err)
	}

	f.mu.Lock()
	ticker.EventType = "24hrTicker"
	f.ticker24h = &ticker
	f.mu.Unlock()
	return nil
}

func (f *BTCMarketDataFeed) bootstrapAvgPrice() error {
	rawURL := fmt.Sprintf("%s/api/v3/avgPrice?symbol=%s", binanceRESTBaseURL, BTCSymbol)
	var avg binanceRESTAvgPrice
	if err := f.getJSON(rawURL, &avg); err != nil {
		return fmt.Errorf("bootstrap avg price failed: %w", err)
	}

	f.mu.Lock()
	f.avgPrice = &binanceAvgPrice{
		EventType:     "avgPrice",
		Symbol:        BTCSymbol,
		Interval:      fmt.Sprintf("%dm", avg.Mins),
		WeightedPrice: avg.Price,
		LastTradeTime: avg.CloseTime,
	}
	f.mu.Unlock()
	return nil
}

func (f *BTCMarketDataFeed) getJSON(rawURL string, dst interface{}) error {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return json.NewDecoder(resp.Body).Decode(dst)
}

func (f *BTCMarketDataFeed) runWebSocketLoop() {
	streams := []string{
		"btcusdt@kline_1h",
		"btcusdt@kline_4h",
		"btcusdt@kline_1d",
		"btcusdt@ticker",
		"btcusdt@ticker_1h",
		"btcusdt@ticker_4h",
		"btcusdt@ticker_1d",
		"btcusdt@avgPrice",
		"btcusdt@bookTicker",
	}

	rawURL := fmt.Sprintf("%s?streams=%s", binanceWebSocketBaseURL, strings.Join(streams, "/"))

	for {
		conn, _, err := websocket.DefaultDialer.Dial(rawURL, nil)
		if err != nil {
			logrus.Errorf("[BTC行情] Binance WebSocket连接失败: %v", err)
			time.Sleep(binanceReconnectDelay)
			continue
		}

		logrus.Infof("[BTC行情] Binance WebSocket已连接: %s", rawURL)
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(70 * time.Second))
		})

		for {
			if err := conn.SetReadDeadline(time.Now().Add(70 * time.Second)); err != nil {
				logrus.Warnf("[BTC行情] 设置读取超时失败: %v", err)
			}

			_, payload, err := conn.ReadMessage()
			if err != nil {
				logrus.Errorf("[BTC行情] Binance WebSocket读取失败: %v", err)
				_ = conn.Close()
				break
			}

			if err := f.handleWebSocketMessage(payload); err != nil {
				logrus.Warnf("[BTC行情] 处理WebSocket消息失败: %v", err)
			}
		}

		time.Sleep(binanceReconnectDelay)
	}
}

func (f *BTCMarketDataFeed) handleWebSocketMessage(payload []byte) error {
	var envelope binanceKlineStreamEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return err
	}

	switch {
	case strings.Contains(envelope.Stream, "@kline_"):
		var event binanceKlineEvent
		if err := json.Unmarshal(envelope.Data, &event); err != nil {
			return err
		}
		f.upsertKline(convertWSKlineToBTCKline(event.Kline, envelope.Stream))
	case strings.HasSuffix(envelope.Stream, "@ticker") && !strings.Contains(envelope.Stream, "@ticker_"):
		var ticker binance24hrTicker
		if err := json.Unmarshal(envelope.Data, &ticker); err != nil {
			return err
		}
		f.mu.Lock()
		f.ticker24h = &ticker
		f.mu.Unlock()
	case strings.Contains(envelope.Stream, "@ticker_"):
		var ticker binanceRollingTicker
		if err := json.Unmarshal(envelope.Data, &ticker); err != nil {
			return err
		}
		interval := extractRollingInterval(envelope.Stream)
		f.mu.Lock()
		f.rollingTickers[interval] = &ticker
		f.mu.Unlock()
	case strings.HasSuffix(envelope.Stream, "@avgPrice"):
		var avg binanceAvgPrice
		if err := json.Unmarshal(envelope.Data, &avg); err != nil {
			return err
		}
		f.mu.Lock()
		f.avgPrice = &avg
		f.mu.Unlock()
	case strings.HasSuffix(envelope.Stream, "@bookTicker"):
		var ticker binanceBookTicker
		if err := json.Unmarshal(envelope.Data, &ticker); err != nil {
			return err
		}
		f.mu.Lock()
		f.bookTicker = &ticker
		f.mu.Unlock()
	}

	return nil
}

func (f *BTCMarketDataFeed) upsertKline(kline BTCKline) {
	f.mu.Lock()
	defer f.mu.Unlock()

	series := append([]BTCKline(nil), f.klines[kline.Interval]...)
	found := false
	for idx := range series {
		if series[idx].OpenTime == kline.OpenTime {
			series[idx] = kline
			found = true
			break
		}
	}
	if !found {
		series = append(series, kline)
	}
	f.klines[kline.Interval] = trimKlines(series, btcMarketDefaultKlineSize)
}

func (f *BTCMarketDataFeed) getKlines(interval string, limit int) ([]BTCKline, error) {
	f.Start()

	f.mu.RLock()
	series := append([]BTCKline(nil), f.klines[interval]...)
	f.mu.RUnlock()

	if len(series) == 0 {
		return nil, fmt.Errorf("no btc klines cached for interval=%s", interval)
	}

	if limit > 0 && len(series) > limit {
		series = series[len(series)-limit:]
	}
	return series, nil
}

func (f *BTCMarketDataFeed) getTodayInfo() (*BTCTodayInfo, error) {
	f.Start()

	f.mu.RLock()
	ticker := f.ticker24h
	f.mu.RUnlock()

	if ticker == nil {
		return nil, fmt.Errorf("no 24hr ticker cached")
	}

	tradingDate := ""
	closeTime := firstPositiveInt64(ticker.CloseTime, ticker.RESTCloseTime)
	if closeTime > 0 {
		tradingDate = time.UnixMilli(closeTime).UTC().Format("2006-01-02")
	}

	return &BTCTodayInfo{
		Symbol:             BTCSymbol,
		TradingDate:        tradingDate,
		CurrentPrice:       firstNonEmpty(ticker.LastPrice, ticker.RESTLastPrice),
		TodayOpenPrice:     firstNonEmpty(ticker.OpenPrice, ticker.RESTOpenPrice),
		TodayHighPrice:     firstNonEmpty(ticker.HighPrice, ticker.RESTHighPrice),
		TodayLowPrice:      firstNonEmpty(ticker.LowPrice, ticker.RESTLowPrice),
		TodayClosePrice:    firstNonEmpty(ticker.LastPrice, ticker.RESTLastPrice),
		TodayVolume:        firstNonEmpty(ticker.Volume, ticker.RESTVolume),
		TodayQuoteVolume:   firstNonEmpty(ticker.QuoteVolume, ticker.RESTQuoteVolume),
		TodayChangeValue:   firstNonEmpty(ticker.PriceChange, ticker.RESTPriceChange),
		TodayChangePercent: firstNonEmpty(ticker.PriceChangePercent, ticker.RESTPriceChangePercent),
		Source:             "binance-ws:btcusdt@ticker",
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

// GetBTC4HKlines 获取 BTC 4 小时 K 线数据。
func GetBTC4HKlines(limit int) ([]BTCKline, error) {
	return getOrCreateBTCMarketDataFeed().getKlines("4h", limit)
}

// GetBTC1HKlines 获取 BTC 1 小时 K 线数据。
func GetBTC1HKlines(limit int) ([]BTCKline, error) {
	return getOrCreateBTCMarketDataFeed().getKlines("1h", limit)
}

// GetBTCDailyKlines 获取 BTC 日线 K 线数据。
func GetBTCDailyKlines(limit int) ([]BTCKline, error) {
	return getOrCreateBTCMarketDataFeed().getKlines("1d", limit)
}

// GetBTCTodayInfo 获取当天 BTC 聚合信息。
func GetBTCTodayInfo() (*BTCTodayInfo, error) {
	return getOrCreateBTCMarketDataFeed().getTodayInfo()
}

// GetBTCEMA 获取 BTC EMA 指标。
func GetBTCEMA(interval string, period int) (*BTCMovingAverage, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	closes, err := klineCloses(series)
	if err != nil {
		return nil, err
	}
	value, err := calculateEMA(closes, period)
	if err != nil {
		return nil, err
	}
	return &BTCMovingAverage{
		Symbol:    BTCSymbol,
		Interval:  interval,
		Period:    period,
		Value:     formatFloat(value),
		Source:    "binance:derived",
		Reference: "close",
	}, nil
}

// GetBTCMA 获取 BTC MA 指标。
func GetBTCMA(interval string, period int) (*BTCMovingAverage, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	closes, err := klineCloses(series)
	if err != nil {
		return nil, err
	}
	value, err := calculateSMA(closes, period)
	if err != nil {
		return nil, err
	}
	return &BTCMovingAverage{
		Symbol:    BTCSymbol,
		Interval:  interval,
		Period:    period,
		Value:     formatFloat(value),
		Source:    "binance:derived",
		Reference: "close",
	}, nil
}

// GetBTCMACD 获取 BTC MACD 指标。
func GetBTCMACD(interval string, fastPeriod, slowPeriod, signalPeriod int) (*BTCMACD, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	closes, err := klineCloses(series)
	if err != nil {
		return nil, err
	}
	macd, signal, histogram, err := calculateMACD(closes, fastPeriod, slowPeriod, signalPeriod)
	if err != nil {
		return nil, err
	}
	return &BTCMACD{
		Symbol:       BTCSymbol,
		Interval:     interval,
		FastPeriod:   fastPeriod,
		SlowPeriod:   slowPeriod,
		SignalPeriod: signalPeriod,
		MACD:         formatFloat(macd),
		Signal:       formatFloat(signal),
		Histogram:    formatFloat(histogram),
		Source:       "binance:derived",
	}, nil
}

// GetBTCRSI 获取 BTC RSI 指标。
func GetBTCRSI(interval string, period int) (*BTCRSI, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	closes, err := klineCloses(series)
	if err != nil {
		return nil, err
	}
	value, err := calculateRSI(closes, period)
	if err != nil {
		return nil, err
	}
	return &BTCRSI{
		Symbol:   BTCSymbol,
		Interval: interval,
		Period:   period,
		Value:    formatFloat(value),
		Source:   "binance:derived",
	}, nil
}

// GetBTCATR 获取 BTC ATR 指标。
func GetBTCATR(interval string, period int) (*BTCATR, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	value, err := calculateATR(series, period)
	if err != nil {
		return nil, err
	}
	return &BTCATR{
		Symbol:   BTCSymbol,
		Interval: interval,
		Period:   period,
		Value:    formatFloat(value),
		Source:   "binance:derived",
	}, nil
}

// GetBTCBollingerBands 获取 BTC 布林带指标。
func GetBTCBollingerBands(interval string, period int, stdDev string) (*BTCBollingerBands, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	closes, err := klineCloses(series)
	if err != nil {
		return nil, err
	}
	stdDevValue, err := strconv.ParseFloat(stdDev, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid stdDev=%s: %w", stdDev, err)
	}
	middle, upper, lower, width, err := calculateBollingerBands(closes, period, stdDevValue)
	if err != nil {
		return nil, err
	}
	return &BTCBollingerBands{
		Symbol:     BTCSymbol,
		Interval:   interval,
		Period:     period,
		StdDev:     stdDev,
		MiddleBand: formatFloat(middle),
		UpperBand:  formatFloat(upper),
		LowerBand:  formatFloat(lower),
		BandWidth:  formatFloat(width),
		Source:     "binance:derived",
	}, nil
}

// GetBTCVolumeProfile 获取 BTC 成交量快照。
func GetBTCVolumeProfile(interval string, period int) (*BTCVolumeProfile, error) {
	series, err := getKlinesByInterval(interval)
	if err != nil {
		return nil, err
	}
	if len(series) < period {
		return nil, fmt.Errorf("not enough klines for volume profile interval=%s period=%d", interval, period)
	}

	window := series[len(series)-period:]
	currentVolume, err := strconv.ParseFloat(window[len(window)-1].Volume, 64)
	if err != nil {
		return nil, err
	}
	currentQuoteVolume, err := strconv.ParseFloat(window[len(window)-1].QuoteVolume, 64)
	if err != nil {
		return nil, err
	}

	var sum float64
	for _, item := range window {
		v, err := strconv.ParseFloat(item.Volume, 64)
		if err != nil {
			return nil, err
		}
		sum += v
	}

	avg := sum / float64(len(window))
	ratio := 0.0
	if avg > 0 {
		ratio = currentVolume / avg
	}

	return &BTCVolumeProfile{
		Symbol:        BTCSymbol,
		Interval:      interval,
		CurrentVolume: formatFloat(currentVolume),
		AverageVolume: formatFloat(avg),
		QuoteVolume:   formatFloat(currentQuoteVolume),
		VolumeRatio:   formatFloat(ratio),
		Source:        "binance:derived",
	}, nil
}

// GetBTCOpenInterest 获取 BTC 未平仓量。
func GetBTCOpenInterest() (*BTCOpenInterest, error) {
	return nil, nil
}

// GetBTCFundingRate 获取 BTC 资金费率。
func GetBTCFundingRate() (*BTCFundingRate, error) {
	return nil, nil
}

// GetBTCLongShortRatio 获取 BTC 多空比。
func GetBTCLongShortRatio(interval string) (*BTCLongShortRatio, error) {
	return nil, nil
}

// GetBTCAnalysisSnapshot 获取给 AI 平仓使用的 BTC 分析快照。
func GetBTCAnalysisSnapshot() (*BTCAnalysisSnapshot, error) {
	klines1H, err := GetBTC1HKlines(200)
	if err != nil {
		return nil, err
	}
	klines4H, err := GetBTC4HKlines(200)
	if err != nil {
		return nil, err
	}
	klines1D, err := GetBTCDailyKlines(200)
	if err != nil {
		return nil, err
	}

	todayInfo, err := GetBTCTodayInfo()
	if err != nil {
		return nil, err
	}

	closes1H, err := klineCloses(klines1H)
	if err != nil {
		return nil, err
	}
	closes4H, err := klineCloses(klines4H)
	if err != nil {
		return nil, err
	}
	closes1D, err := klineCloses(klines1D)
	if err != nil {
		return nil, err
	}

	snapshot := &BTCAnalysisSnapshot{
		Symbol:    BTCSymbol,
		Klines1H:  klines1H,
		Klines4H:  klines4H,
		Klines1D:  klines1D,
		TodayInfo: todayInfo,
	}

	snapshot.EMA1H20 = buildMovingAverage(BTCSymbol, "1h", 20, "close", closes1H, calculateEMA)
	snapshot.EMA1H50 = buildMovingAverage(BTCSymbol, "1h", 50, "close", closes1H, calculateEMA)
	snapshot.EMA4H20 = buildMovingAverage(BTCSymbol, "4h", 20, "close", closes4H, calculateEMA)
	snapshot.EMA4H50 = buildMovingAverage(BTCSymbol, "4h", 50, "close", closes4H, calculateEMA)
	snapshot.EMA1D20 = buildMovingAverage(BTCSymbol, "1d", 20, "close", closes1D, calculateEMA)
	snapshot.EMA1D50 = buildMovingAverage(BTCSymbol, "1d", 50, "close", closes1D, calculateEMA)
	snapshot.MA1D200 = buildMovingAverage(BTCSymbol, "1d", 200, "close", closes1D, calculateSMA)

	snapshot.MACD1H = buildMACD(BTCSymbol, "1h", closes1H, 12, 26, 9)
	snapshot.MACD4H = buildMACD(BTCSymbol, "4h", closes4H, 12, 26, 9)
	snapshot.MACD1D = buildMACD(BTCSymbol, "1d", closes1D, 12, 26, 9)

	snapshot.RSI1H14 = buildRSI(BTCSymbol, "1h", closes1H, 14)
	snapshot.RSI4H14 = buildRSI(BTCSymbol, "4h", closes4H, 14)
	snapshot.RSI1D14 = buildRSI(BTCSymbol, "1d", closes1D, 14)

	snapshot.ATR1H14 = buildATR(BTCSymbol, "1h", klines1H, 14)
	snapshot.ATR4H14 = buildATR(BTCSymbol, "4h", klines4H, 14)
	snapshot.ATR1D14 = buildATR(BTCSymbol, "1d", klines1D, 14)

	snapshot.Bollinger1H = buildBollingerBands(BTCSymbol, "1h", closes1H, 20, "2")
	snapshot.Bollinger4H = buildBollingerBands(BTCSymbol, "4h", closes4H, 20, "2")
	snapshot.Bollinger1D = buildBollingerBands(BTCSymbol, "1d", closes1D, 20, "2")

	snapshot.Volume1H = buildVolumeProfile(BTCSymbol, "1h", klines1H, 20)
	snapshot.Volume4H = buildVolumeProfile(BTCSymbol, "4h", klines4H, 20)
	snapshot.Volume1D = buildVolumeProfile(BTCSymbol, "1d", klines1D, 20)

	snapshot.OpenInterest, _ = GetBTCOpenInterest()
	snapshot.FundingRate, _ = GetBTCFundingRate()
	snapshot.LongShortRatio1H, _ = GetBTCLongShortRatio("1h")
	snapshot.LongShortRatio4H, _ = GetBTCLongShortRatio("4h")
	snapshot.LongShortRatio1D, _ = GetBTCLongShortRatio("1d")

	return snapshot, nil
}

func buildMovingAverage(symbol, interval string, period int, reference string, closes []float64, calc func([]float64, int) (float64, error)) *BTCMovingAverage {
	value, err := calc(closes, period)
	if err != nil {
		return nil
	}
	return &BTCMovingAverage{
		Symbol:    symbol,
		Interval:  interval,
		Period:    period,
		Value:     formatFloat(value),
		Source:    "binance:derived",
		Reference: reference,
	}
}

func buildMACD(symbol, interval string, closes []float64, fastPeriod, slowPeriod, signalPeriod int) *BTCMACD {
	macd, signal, histogram, err := calculateMACD(closes, fastPeriod, slowPeriod, signalPeriod)
	if err != nil {
		return nil
	}
	return &BTCMACD{
		Symbol:       symbol,
		Interval:     interval,
		FastPeriod:   fastPeriod,
		SlowPeriod:   slowPeriod,
		SignalPeriod: signalPeriod,
		MACD:         formatFloat(macd),
		Signal:       formatFloat(signal),
		Histogram:    formatFloat(histogram),
		Source:       "binance:derived",
	}
}

func buildRSI(symbol, interval string, closes []float64, period int) *BTCRSI {
	value, err := calculateRSI(closes, period)
	if err != nil {
		return nil
	}
	return &BTCRSI{
		Symbol:   symbol,
		Interval: interval,
		Period:   period,
		Value:    formatFloat(value),
		Source:   "binance:derived",
	}
}

func buildATR(symbol, interval string, series []BTCKline, period int) *BTCATR {
	value, err := calculateATR(series, period)
	if err != nil {
		return nil
	}
	return &BTCATR{
		Symbol:   symbol,
		Interval: interval,
		Period:   period,
		Value:    formatFloat(value),
		Source:   "binance:derived",
	}
}

func buildBollingerBands(symbol, interval string, closes []float64, period int, stdDev string) *BTCBollingerBands {
	stdDevValue, err := strconv.ParseFloat(stdDev, 64)
	if err != nil {
		return nil
	}
	middle, upper, lower, width, err := calculateBollingerBands(closes, period, stdDevValue)
	if err != nil {
		return nil
	}
	return &BTCBollingerBands{
		Symbol:     symbol,
		Interval:   interval,
		Period:     period,
		StdDev:     stdDev,
		MiddleBand: formatFloat(middle),
		UpperBand:  formatFloat(upper),
		LowerBand:  formatFloat(lower),
		BandWidth:  formatFloat(width),
		Source:     "binance:derived",
	}
}

func buildVolumeProfile(symbol, interval string, series []BTCKline, period int) *BTCVolumeProfile {
	if len(series) < period {
		return nil
	}

	window := series[len(series)-period:]
	currentVolume, err := strconv.ParseFloat(window[len(window)-1].Volume, 64)
	if err != nil {
		return nil
	}
	currentQuoteVolume, err := strconv.ParseFloat(window[len(window)-1].QuoteVolume, 64)
	if err != nil {
		return nil
	}

	var sum float64
	for _, item := range window {
		v, err := strconv.ParseFloat(item.Volume, 64)
		if err != nil {
			return nil
		}
		sum += v
	}

	avg := sum / float64(len(window))
	ratio := 0.0
	if avg > 0 {
		ratio = currentVolume / avg
	}

	return &BTCVolumeProfile{
		Symbol:        symbol,
		Interval:      interval,
		CurrentVolume: formatFloat(currentVolume),
		AverageVolume: formatFloat(avg),
		QuoteVolume:   formatFloat(currentQuoteVolume),
		VolumeRatio:   formatFloat(ratio),
		Source:        "binance:derived",
	}
}

func getKlinesByInterval(interval string) ([]BTCKline, error) {
	switch interval {
	case "1h":
		return GetBTC1HKlines(0)
	case "4h":
		return GetBTC4HKlines(0)
	case "1d":
		return GetBTCDailyKlines(0)
	default:
		return nil, fmt.Errorf("unsupported interval=%s", interval)
	}
}

func parseRESTKlineRow(row []interface{}, interval string) (BTCKline, error) {
	if len(row) < 11 {
		return BTCKline{}, fmt.Errorf("unexpected kline row length=%d", len(row))
	}

	openTime, err := toInt64(row[0])
	if err != nil {
		return BTCKline{}, err
	}
	closeTime, err := toInt64(row[6])
	if err != nil {
		return BTCKline{}, err
	}
	tradeCount, err := toInt64(row[8])
	if err != nil {
		return BTCKline{}, err
	}

	return BTCKline{
		Symbol:        BTCSymbol,
		Interval:      interval,
		OpenTime:      strconv.FormatInt(openTime, 10),
		CloseTime:     strconv.FormatInt(closeTime, 10),
		OpenPrice:     fmt.Sprint(row[1]),
		HighPrice:     fmt.Sprint(row[2]),
		LowPrice:      fmt.Sprint(row[3]),
		ClosePrice:    fmt.Sprint(row[4]),
		Volume:        fmt.Sprint(row[5]),
		QuoteVolume:   fmt.Sprint(row[7]),
		TradeCount:    tradeCount,
		Source:        "binance-rest:/api/v3/klines",
		RawPayloadRef: fmt.Sprintf("symbol=%s&interval=%s", BTCSymbol, interval),
	}, nil
}

func convertWSKlineToBTCKline(k binanceKlineStream, stream string) BTCKline {
	return BTCKline{
		Symbol:        k.Symbol,
		Interval:      k.Interval,
		OpenTime:      strconv.FormatInt(k.StartTime, 10),
		CloseTime:     strconv.FormatInt(k.CloseTime, 10),
		OpenPrice:     k.OpenPrice,
		HighPrice:     k.HighPrice,
		LowPrice:      k.LowPrice,
		ClosePrice:    k.ClosePrice,
		Volume:        k.Volume,
		QuoteVolume:   k.QuoteVolume,
		TradeCount:    k.TradeCount,
		Source:        "binance-ws:" + stream,
		RawPayloadRef: stream,
	}
}

func trimKlines(items []BTCKline, max int) []BTCKline {
	if len(items) <= max {
		return items
	}
	return items[len(items)-max:]
}

func klineCloses(series []BTCKline) ([]float64, error) {
	closes := make([]float64, 0, len(series))
	for _, item := range series {
		value, err := strconv.ParseFloat(item.ClosePrice, 64)
		if err != nil {
			return nil, err
		}
		closes = append(closes, value)
	}
	return closes, nil
}

func calculateSMA(values []float64, period int) (float64, error) {
	if len(values) < period {
		return 0, fmt.Errorf("not enough values for sma period=%d", period)
	}
	var sum float64
	for _, value := range values[len(values)-period:] {
		sum += value
	}
	return sum / float64(period), nil
}

func calculateEMA(values []float64, period int) (float64, error) {
	if len(values) < period {
		return 0, fmt.Errorf("not enough values for ema period=%d", period)
	}

	ema, err := calculateSMA(values[:period], period)
	if err != nil {
		return 0, err
	}

	multiplier := 2.0 / float64(period+1)
	for _, value := range values[period:] {
		ema = (value-ema)*multiplier + ema
	}
	return ema, nil
}

func calculateEMASequence(values []float64, period int) ([]float64, error) {
	if len(values) < period {
		return nil, fmt.Errorf("not enough values for ema sequence period=%d", period)
	}

	seed, err := calculateSMA(values[:period], period)
	if err != nil {
		return nil, err
	}

	multiplier := 2.0 / float64(period+1)
	series := []float64{seed}
	ema := seed
	for _, value := range values[period:] {
		ema = (value-ema)*multiplier + ema
		series = append(series, ema)
	}
	return series, nil
}

func calculateMACD(values []float64, fastPeriod, slowPeriod, signalPeriod int) (float64, float64, float64, error) {
	if len(values) < slowPeriod+signalPeriod {
		return 0, 0, 0, fmt.Errorf("not enough values for macd")
	}

	fastEMA, err := calculateEMASequence(values, fastPeriod)
	if err != nil {
		return 0, 0, 0, err
	}
	slowEMA, err := calculateEMASequence(values, slowPeriod)
	if err != nil {
		return 0, 0, 0, err
	}

	offset := slowPeriod - fastPeriod
	macdSeries := make([]float64, 0, len(slowEMA))
	for idx := range slowEMA {
		macdSeries = append(macdSeries, fastEMA[idx+offset]-slowEMA[idx])
	}

	signalSeries, err := calculateEMASequence(macdSeries, signalPeriod)
	if err != nil {
		return 0, 0, 0, err
	}

	macd := macdSeries[len(macdSeries)-1]
	signal := signalSeries[len(signalSeries)-1]
	return macd, signal, macd - signal, nil
}

func calculateRSI(values []float64, period int) (float64, error) {
	if len(values) < period+1 {
		return 0, fmt.Errorf("not enough values for rsi period=%d", period)
	}

	var gainSum float64
	var lossSum float64
	for idx := 1; idx <= period; idx++ {
		change := values[idx] - values[idx-1]
		if change > 0 {
			gainSum += change
		} else {
			lossSum -= change
		}
	}

	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)
	for idx := period + 1; idx < len(values); idx++ {
		change := values[idx] - values[idx-1]
		gain := math.Max(change, 0)
		loss := math.Max(-change, 0)
		avgGain = ((avgGain * float64(period-1)) + gain) / float64(period)
		avgLoss = ((avgLoss * float64(period-1)) + loss) / float64(period)
	}

	if avgLoss == 0 {
		return 100, nil
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs)), nil
}

func calculateATR(series []BTCKline, period int) (float64, error) {
	if len(series) < period+1 {
		return 0, fmt.Errorf("not enough klines for atr period=%d", period)
	}

	trueRanges := make([]float64, 0, len(series)-1)
	for idx := 1; idx < len(series); idx++ {
		high, err := strconv.ParseFloat(series[idx].HighPrice, 64)
		if err != nil {
			return 0, err
		}
		low, err := strconv.ParseFloat(series[idx].LowPrice, 64)
		if err != nil {
			return 0, err
		}
		prevClose, err := strconv.ParseFloat(series[idx-1].ClosePrice, 64)
		if err != nil {
			return 0, err
		}

		tr := math.Max(high-low, math.Max(math.Abs(high-prevClose), math.Abs(low-prevClose)))
		trueRanges = append(trueRanges, tr)
	}

	return calculateSMA(trueRanges, period)
}

func calculateBollingerBands(values []float64, period int, stdDev float64) (float64, float64, float64, float64, error) {
	if len(values) < period {
		return 0, 0, 0, 0, fmt.Errorf("not enough values for bollinger period=%d", period)
	}

	window := values[len(values)-period:]
	var sum float64
	for _, value := range window {
		sum += value
	}
	mean := sum / float64(period)

	var variance float64
	for _, value := range window {
		diff := value - mean
		variance += diff * diff
	}
	std := math.Sqrt(variance / float64(period))
	upper := mean + stdDev*std
	lower := mean - stdDev*std
	width := 0.0
	if mean != 0 {
		width = (upper - lower) / mean
	}
	return mean, upper, lower, width, nil
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func toInt64(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported numeric type %T", value)
	}
}

func extractRollingInterval(stream string) string {
	idx := strings.LastIndex(stream, "@ticker_")
	if idx == -1 {
		return ""
	}
	return stream[idx+len("@ticker_"):]
}
