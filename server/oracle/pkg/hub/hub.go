// Package hub 提供进程内行情数据单例 MarketDataHub。
// 优先维护 Binance Futures WebSocket 连接（合并 miniTicker 流），
// 连接不可用时自动降级到 30s REST 轮询，业务层通过 GetPrice/Subscribe 无感切换。
package hub

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"

	"oracle/pkg/oraclecfg"
)

const (
	binanceFuturesWS   = "wss://fstream.binance.com/stream"
	binanceFuturesREST = "https://fapi.binance.com"
	pingInterval       = 30 * time.Second
	restPollInterval   = 30 * time.Second
	wsReconnectDelay   = 5 * time.Second
	wsMaxReconnectWait = 2 * time.Minute
	subBufSize         = 64
)

// Tick 是行情推送单元。
type Tick struct {
	Symbol string
	Price  float64
	At     time.Time
}

// MarketDataHub 维护行情最新价 + 订阅者广播，整个进程单例。
type MarketDataHub struct {
	mu     sync.RWMutex
	prices map[string]float64 // symbol(大写) → 最新价

	subMu sync.Mutex
	subs  []chan Tick // 订阅者列表，每个订阅者收所有 tick

	cfg     oraclecfg.Config
	symbols []string // e.g. ["BTCUSDT"]
	coins   []string // e.g. ["BTC"]

	httpClient *http.Client
	wsMode     atomic.Bool // true=WS 连通, false=REST 降级

	stop chan struct{}
	wg   sync.WaitGroup
}

// New 创建 hub，coins 列表来自 oracle 配置，与调度器保持一致。
func New(cfg oraclecfg.Config, coins []string) *MarketDataHub {
	h := &MarketDataHub{
		prices:  make(map[string]float64),
		cfg:     cfg,
		coins:   coins,
		symbols: coinsToSymbols(coins),
		httpClient: &http.Client{Timeout: 10 * time.Second},
		stop:    make(chan struct{}),
	}
	return h
}

// Start 启动行情采集主循环（非阻塞）。
func (h *MarketDataHub) Start() {
	h.wg.Add(1)
	go h.run()
	logrus.Infof("[hub] 启动: symbols=%v", h.symbols)
}

// Stop 优雅关闭，等待内部 goroutine 退出。
func (h *MarketDataHub) Stop() {
	close(h.stop)
	h.wg.Wait()
}

// GetPrice 同步读取最新价，ok=false 表示尚无数据。
func (h *MarketDataHub) GetPrice(symbol string) (float64, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.prices[strings.ToUpper(symbol)]
	return p, ok && p > 0
}

// Subscribe 注册订阅，返回 (<-chan Tick, cancel)。
// 订阅者收到的是所有 symbols 的 tick；chan 缓冲 subBufSize，满时丢弃（非阻塞）。
// 调用 cancel() 取消订阅并关闭 chan。
func (h *MarketDataHub) Subscribe() (<-chan Tick, func()) {
	ch := make(chan Tick, subBufSize)
	h.subMu.Lock()
	h.subs = append(h.subs, ch)
	h.subMu.Unlock()

	cancel := func() {
		h.subMu.Lock()
		defer h.subMu.Unlock()
		for i, s := range h.subs {
			if s == ch {
				h.subs = append(h.subs[:i], h.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, cancel
}

// IsWSMode 返回当前是否处于 WebSocket 模式。
func (h *MarketDataHub) IsWSMode() bool { return h.wsMode.Load() }

// ─── 内部主循环 ──────────────────────────────────────────────────────────────

func (h *MarketDataHub) run() {
	defer h.wg.Done()

	// 启动时先用 REST 拉一次现价，保证 GetPrice 立即可用
	h.fetchAllREST()

	reconnectWait := wsReconnectDelay
	for {
		select {
		case <-h.stop:
			return
		default:
		}

		// 尝试 WebSocket 连接
		if err := h.runWS(); err != nil {
			logrus.Warnf("[hub] WS 连接断开(%v)，%s 后重连，降级 REST 轮询", err, reconnectWait)
			h.wsMode.Store(false)
		}

		// WS 断开期间用 REST 降级轮询
		h.wg.Add(1)
		restDone := make(chan struct{})
		go func() {
			defer h.wg.Done()
			h.runRESTFallback(restDone)
		}()

		select {
		case <-h.stop:
			close(restDone)
			return
		case <-time.After(reconnectWait):
			close(restDone)
			// 指数退避，最长 wsMaxReconnectWait
			reconnectWait = time.Duration(math.Min(
				float64(reconnectWait*2),
				float64(wsMaxReconnectWait),
			))
			// 重连前用 REST 补一次价格
			h.fetchAllREST()
		}
	}
}

// runWS 建立并维护 WS 连接，直到连接断开或 stop 信号。
func (h *MarketDataHub) runWS() error {
	if len(h.symbols) == 0 {
		return fmt.Errorf("无 symbol 配置")
	}

	// 构造合并流 URL: /stream?streams=btcusdt@miniTicker/ethusdt@miniTicker
	streams := make([]string, len(h.symbols))
	for i, s := range h.symbols {
		streams[i] = strings.ToLower(s) + "@miniTicker"
	}
	u := fmt.Sprintf("%s?streams=%s", binanceFuturesWS, strings.Join(streams, "/"))

	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		return fmt.Errorf("dial %s: %w", u, err)
	}
	defer conn.Close()

	h.wsMode.Store(true)
	logrus.Infof("[hub] WS 已连接: %s", u)

	// 心跳：定时发 ping
	pingTicker := time.NewTicker(pingInterval)
	defer pingTicker.Stop()

	msgCh := make(chan []byte, 32)
	errCh := make(chan error, 1)

	// 读协程
	go func() {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				errCh <- err
				return
			}
			select {
			case msgCh <- msg:
			default: // 丢弃（msgCh 满）
			}
		}
	}()

	for {
		select {
		case <-h.stop:
			_ = conn.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return nil
		case err := <-errCh:
			return err
		case <-pingTicker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return fmt.Errorf("ping: %w", err)
			}
		case msg := <-msgCh:
			h.handleWSMessage(msg)
		}
	}
}

// Binance 合并流外层包装
type combinedMsg struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

// Binance miniTicker data
type miniTickerData struct {
	Symbol    string `json:"s"`
	LastPrice string `json:"c"` // 最新成交价
}

func (h *MarketDataHub) handleWSMessage(msg []byte) {
	var wrapper combinedMsg
	if err := json.Unmarshal(msg, &wrapper); err != nil {
		return
	}
	var ticker miniTickerData
	if err := json.Unmarshal(wrapper.Data, &ticker); err != nil {
		return
	}
	if ticker.Symbol == "" || ticker.LastPrice == "" {
		return
	}
	price, err := strconv.ParseFloat(strings.TrimSpace(ticker.LastPrice), 64)
	if err != nil || price <= 0 {
		return
	}
	h.setPrice(ticker.Symbol, price)
}

// runRESTFallback 每 restPollInterval 拉一次所有 symbols 现价，直到 done 关闭。
func (h *MarketDataHub) runRESTFallback(done <-chan struct{}) {
	ticker := time.NewTicker(restPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-h.stop:
			return
		case <-ticker.C:
			h.fetchAllREST()
		}
	}
}

func (h *MarketDataHub) fetchAllREST() {
	for i, symbol := range h.symbols {
		price, err := h.fetchRESTPrice(symbol)
		if err != nil {
			logrus.Warnf("[hub] REST 拉价失败 %s: %v", symbol, err)
			continue
		}
		h.setPrice(symbol, price)
		_ = i
	}
}

func (h *MarketDataHub) fetchRESTPrice(symbol string) (float64, error) {
	rawURL := fmt.Sprintf("%s/fapi/v1/ticker/price?symbol=%s",
		binanceFuturesREST, url.QueryEscape(strings.ToUpper(symbol)))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		Price string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	price, err := strconv.ParseFloat(strings.TrimSpace(result.Price), 64)
	if err != nil || price <= 0 {
		return 0, fmt.Errorf("无效价格 %q", result.Price)
	}
	return price, nil
}

// setPrice 更新价格并广播给所有订阅者（非阻塞投递）。
func (h *MarketDataHub) setPrice(symbol string, price float64) {
	sym := strings.ToUpper(symbol)
	h.mu.Lock()
	h.prices[sym] = price
	h.mu.Unlock()

	tick := Tick{Symbol: sym, Price: price, At: time.Now().UTC()}

	h.subMu.Lock()
	subs := make([]chan Tick, len(h.subs))
	copy(subs, h.subs)
	h.subMu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- tick:
		default: // 订阅者处理慢，丢弃，不阻塞发布者
		}
	}
}

func coinsToSymbols(coins []string) []string {
	out := make([]string, 0, len(coins))
	for _, c := range coins {
		c = strings.TrimSpace(strings.ToUpper(c))
		if c != "" {
			out = append(out, c+"USDT")
		}
	}
	return out
}
