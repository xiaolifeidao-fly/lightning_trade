package monitor

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strings"
	"sync"
	"time"

	"common/middleware/vipper"
	"common/utils"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
)

// SymbolConfig 币种配置
type SymbolConfig struct {
	DeepInst  string  // DeepCoin合约名称（用于行情订阅，如 BTC-USDT-SWAP）
	TradeInst string  // DeepCoin下单交易对（如 BTCUSDT）
	Threshold float64 // 价差阈值（如0.001表示0.1%）
}

// PriceMonitor 价格监控器
type PriceMonitor struct {
	symbolConfigs  map[string]SymbolConfig // 币种配置
	binancePrices  map[string]float64      // 币安价格
	deepcoinPrices map[string]float64      // DeepCoin价格
	telegramClient *utils.TelegramClient   // Telegram客户端
	mu             sync.RWMutex            // 价格读写锁
	stopChan       chan struct{}           // 停止信号
	wg             sync.WaitGroup          // 等待组
	lastAlertTime  map[string]time.Time    // 上次告警时间（防重复）
	alertMu        sync.Mutex              // 告警时间锁
	lastTradeTime  map[string]time.Time    // 上次交易时间（防重复），key: "symbol:long" 或 "symbol:short"
	tradeMu        sync.Mutex              // 交易时间锁
}

// NewPriceMonitor 创建价格监控器
func NewPriceMonitor(symbolConfigs map[string]SymbolConfig) *PriceMonitor {
	return &PriceMonitor{
		symbolConfigs:  symbolConfigs,
		binancePrices:  make(map[string]float64),
		deepcoinPrices: make(map[string]float64),
		telegramClient: utils.NewTelegramClientWithBotTokenAndChatID(vipper.GetString("telegram.bot_token"), vipper.GetString("telegram.chat_id")),
		stopChan:       make(chan struct{}),
		lastAlertTime:  make(map[string]time.Time),
		lastTradeTime:  make(map[string]time.Time),
	}
}

// Start 启动监控
func (pm *PriceMonitor) Start() {
	logrus.Info("启动价格监控...")

	for symbol := range pm.symbolConfigs {
		pm.binancePrices[symbol] = 0
		pm.deepcoinPrices[symbol] = 0

		// 启动币安WebSocket
		pm.wg.Add(1)
		go pm.subscribeBinance(symbol)

		// 启动DeepCoin WebSocket
		pm.wg.Add(1)
		go pm.subscribeDeepCoin(symbol)
		// go pm.subscribeDeepCoin_V2(symbol)
	}

	logrus.Infof("✅ 已启动多币种监听：%v", pm.getSymbolList())
}

// Stop 停止监控
func (pm *PriceMonitor) Stop() {
	close(pm.stopChan)
	pm.wg.Wait()
	logrus.Info("价格监控已停止")
}

// subscribeBinance 订阅币安价格
func (pm *PriceMonitor) subscribeBinance(symbol string) {
	defer pm.wg.Done()

	// url := "wss://fstream.binance.com/ws"
	url := "wss://stream.binance.com:9443/ws"

	for {
		select {
		case <-pm.stopChan:
			return
		default:
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				// 清除价格缓存
				pm.mu.Lock()
				pm.binancePrices[symbol] = 0
				pm.mu.Unlock()
				logrus.Errorf("%s Binance连接失败: %v, 已清除价格缓存，1秒后重试", symbol, err)
				time.Sleep(1 * time.Second)
				continue
			}

			logrus.Infof("%s Binance WebSocket已连接", symbol)

			// 发送订阅消息（币安需要小写symbol）
			stream := fmt.Sprintf("%s@aggTrade", strings.ToLower(symbol))
			subscribeMsg := map[string]interface{}{
				"method": "SUBSCRIBE",
				"params": []string{stream},
				"id":     1,
			}
			logrus.Debugf("%s Binance发送订阅消息: %v", symbol, subscribeMsg)
			if err := conn.WriteJSON(subscribeMsg); err != nil {
				logrus.Errorf("%s Binance发送订阅消息失败: %v", symbol, err)
				conn.Close()
				// 清除价格缓存
				pm.mu.Lock()
				pm.binancePrices[symbol] = 0
				pm.mu.Unlock()
				time.Sleep(1 * time.Second)
				continue
			}

			// 设置ping
			ticker := time.NewTicker(20 * time.Second)
			defer ticker.Stop()

			// 读取消息
			for {
				select {
				case <-pm.stopChan:
					conn.Close()
					return
				case <-ticker.C:
					if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
						logrus.Errorf("%s Binance ping失败: %v", symbol, err)
						conn.Close()
						goto reconnect
					}
				default:
					conn.SetReadDeadline(time.Now().Add(30 * time.Second))
					_, message, err := conn.ReadMessage()
					if err != nil {
						logrus.Errorf("%s Binance读取消息失败: %v", symbol, err)
						conn.Close()
						goto reconnect
					}

					pm.handleBinanceMessage(symbol, message)
				}
			}

		reconnect:
			// 清除内存中的币安价格，避免用旧价格比较
			pm.mu.Lock()
			pm.binancePrices[symbol] = 0
			pm.mu.Unlock()
			logrus.Warnf("%s Binance连接断开，已清除价格缓存，1秒后重连", symbol)
			time.Sleep(1 * time.Second)
		}
	}
}

// subscribeDeepCoin 订阅DeepCoin价格（老版本协议 非官网）
func (pm *PriceMonitor) subscribeDeepCoin(symbol string) {
	defer pm.wg.Done()

	// 使用与Python相同的WebSocket地址
	url := "wss://stream.deepcoin.com/public/ws"
	// url := "wss://stream.deepcoin.com/streamlet/trade/public/swap?platform=api"

	for {
		select {
		case <-pm.stopChan:
			return
		default:
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				// 清除价格缓存
				pm.mu.Lock()
				pm.deepcoinPrices[symbol] = 0
				pm.mu.Unlock()
				logrus.Errorf("%s DeepCoin连接失败: %v, 已清除价格缓存，3秒后重试", symbol, err)
				time.Sleep(3 * time.Second)
				continue
			}

			logrus.Infof("%s DeepCoin WebSocket已连接", symbol)

			// 配置WebSocket连接
			// 设置Ping处理器（当服务器发送WebSocket Ping帧时，自动回复Pong帧）
			conn.SetPingHandler(func(appData string) error {
				logrus.Debugf("%s [DeepCoin] ⬅️ 收到WebSocket Ping帧，自动回复Pong帧", symbol)
				err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
				if err != nil {
					logrus.Errorf("%s [DeepCoin] 回复Pong帧失败: %v", symbol, err)
					return err
				}
				conn.SetReadDeadline(time.Now().Add(40 * time.Second))
				return nil
			})

			// 发送订阅消息 - 使用与Python相同的参数
			subscribeMsg := map[string]interface{}{
				"SendTopicAction": map[string]interface{}{
					"Action":      "1",
					"FilterValue": fmt.Sprintf("DeepCoin_%s", symbol),
					"LocalNo":     6,
					"ResumeNo":    -1, // 与Python保持一致使用-1
					"TopicID":     "7",
				},
			}

			// 打印订阅消息
			subscribeMsgJSON, _ := json.Marshal(subscribeMsg)
			logrus.Infof("%s [DeepCoin] 发送订阅消息: %s", symbol, string(subscribeMsgJSON))

			if err := conn.WriteJSON(subscribeMsg); err != nil {
				logrus.Errorf("%s DeepCoin发送订阅消息失败: %v", symbol, err)
				conn.Close()
				// 清除价格缓存
				pm.mu.Lock()
				pm.deepcoinPrices[symbol] = 0
				pm.mu.Unlock()
				time.Sleep(3 * time.Second)
				continue
			}

			logrus.Infof("%s [DeepCoin] 订阅消息已发送，等待服务器响应...", symbol)

			// 心跳控制 - 发送文本消息"ping"（不是WebSocket Ping帧）
			pingTicker := time.NewTicker(20 * time.Second)
			stopHeartbeat := make(chan struct{})
			heartbeatDone := make(chan struct{})

			// 心跳goroutine - 发送文本消息"ping"
			go func() {
				defer close(heartbeatDone)
				for {
					select {
					case <-stopHeartbeat:
						return
					case <-pingTicker.C:
						// 发送文本消息"ping"（不是WebSocket协议的Ping帧）
						logrus.Debugf("%s [DeepCoin] ➡️ 发送文本心跳: ping", symbol)
						if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
							logrus.Errorf("%s [DeepCoin] ❌ 文本心跳发送失败: %v", symbol, err)
							return
						}
						logrus.Debugf("%s [DeepCoin] ✅ 文本ping发送成功", symbol)
					}
				}
			}()

			// 主读取循环
			connectionBroken := false
			// 设置读取超时：40秒（20秒心跳间隔 + 20秒容差）
			conn.SetReadDeadline(time.Now().Add(40 * time.Second))
			logrus.Debugf("%s [DeepCoin] 使用文本消息心跳机制（ping/pong）", symbol)

		readLoop:
			for {
				select {
				case <-pm.stopChan:
					close(stopHeartbeat)
					pingTicker.Stop()
					<-heartbeatDone // 等待心跳goroutine完全退出
					conn.Close()
					return
				default:
				}

				msgType, message, err := conn.ReadMessage()
				if err != nil {
					// 判断是否为超时错误
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						logrus.Errorf("%s DeepCoin读取超时 (40秒内未收到任何消息): %v", symbol, err)
					} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						logrus.Warnf("%s DeepCoin服务器正常关闭连接: %v", symbol, err)
					} else {
						logrus.Errorf("%s DeepCoin读取消息失败: %v", symbol, err)
					}
					connectionBroken = true
					break readLoop
				}

				// 每次成功读取消息后重置超时
				conn.SetReadDeadline(time.Now().Add(40 * time.Second))

				// 处理文本"pong"消息（心跳响应）
				if msgType == websocket.TextMessage && string(message) == "pong" {
					logrus.Debugf("%s [DeepCoin] ⬅️ 收到文本pong响应 ❤️", symbol)
					continue
				}

				// 记录收到的消息类型
				msgTypeStr := "unknown"
				switch msgType {
				case websocket.TextMessage:
					msgTypeStr = "text"
				case websocket.BinaryMessage:
					msgTypeStr = "binary"
				case websocket.PingMessage:
					msgTypeStr = "ping"
				case websocket.PongMessage:
					msgTypeStr = "pong"
				}

				logrus.Debugf("%s [DeepCoin] ⬅️ 收到%s消息，长度: %d", symbol, msgTypeStr, len(message))
				pm.handleDeepCoinMessage(symbol, message)
			}

			// 清理资源
			close(stopHeartbeat)
			pingTicker.Stop()
			conn.Close()
			// 等待心跳goroutine完全退出
			<-heartbeatDone

			// 清除内存中的DeepCoin价格，避免用旧价格比较
			pm.mu.Lock()
			pm.deepcoinPrices[symbol] = 0
			pm.mu.Unlock()

			if connectionBroken {
				logrus.Warnf("%s DeepCoin连接断开，已清除价格缓存，3秒后重连", symbol)
				time.Sleep(3 * time.Second)
			}
		}
	}
}

// subscribeDeepCoin_V2 订阅DeepCoin价格（V2版本协议）
func (pm *PriceMonitor) subscribeDeepCoin_V2(symbol string) {
	defer pm.wg.Done()

	// V2版本WebSocket地址
	url := "wss://stream.deepcoin.com/streamlet/trade/public/swap?platform=api&version=v2"

	for {
		select {
		case <-pm.stopChan:
			return
		default:
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				// 清除价格缓存
				pm.mu.Lock()
				pm.deepcoinPrices[symbol] = 0
				pm.mu.Unlock()
				logrus.Errorf("%s DeepCoin V2连接失败: %v, 已清除价格缓存，3秒后重试", symbol, err)
				time.Sleep(3 * time.Second)
				continue
			}

			logrus.Infof("%s DeepCoin V2 WebSocket已连接", symbol)

			// 配置WebSocket连接
			// 设置Ping处理器（当服务器发送WebSocket Ping帧时，自动回复Pong帧）
			conn.SetPingHandler(func(appData string) error {
				logrus.Debugf("%s [DeepCoin V2] ⬅️ 收到WebSocket Ping帧，自动回复Pong帧", symbol)
				err := conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(5*time.Second))
				if err != nil {
					logrus.Errorf("%s [DeepCoin V2] 回复Pong帧失败: %v", symbol, err)
					return err
				}
				conn.SetReadDeadline(time.Now().Add(40 * time.Second))
				return nil
			})

			// 发送订阅消息 - V2版本格式
			subscribeMsg := map[string]interface{}{
				"Action":   "1",
				"Symbol":   symbol, // 合约格式：BTCUSDT（不加斜杠）
				"LocalNo":  6,
				"ResumeNo": -1, // -1: 从服务端最新位置续传
				"Topic":    "market",
			}

			// 打印订阅消息
			subscribeMsgJSON, _ := json.Marshal(subscribeMsg)
			logrus.Infof("%s [DeepCoin V2] 发送订阅消息: %s", symbol, string(subscribeMsgJSON))

			if err := conn.WriteJSON(subscribeMsg); err != nil {
				logrus.Errorf("%s DeepCoin V2发送订阅消息失败: %v", symbol, err)
				conn.Close()
				// 清除价格缓存
				pm.mu.Lock()
				pm.deepcoinPrices[symbol] = 0
				pm.mu.Unlock()
				time.Sleep(3 * time.Second)
				continue
			}

			logrus.Infof("%s [DeepCoin V2] 订阅消息已发送，等待服务器响应...", symbol)

			// 心跳控制 - 发送文本消息"ping"（不是WebSocket Ping帧）
			pingTicker := time.NewTicker(20 * time.Second)
			stopHeartbeat := make(chan struct{})
			heartbeatDone := make(chan struct{})

			// 心跳goroutine - 发送文本消息"ping"
			go func() {
				defer close(heartbeatDone)
				for {
					select {
					case <-stopHeartbeat:
						return
					case <-pingTicker.C:
						// 发送文本消息"ping"（不是WebSocket协议的Ping帧）
						logrus.Debugf("%s [DeepCoin V2] ➡️ 发送文本心跳: ping", symbol)
						if err := conn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
							logrus.Errorf("%s [DeepCoin V2] ❌ 文本心跳发送失败: %v", symbol, err)
							return
						}
						logrus.Debugf("%s [DeepCoin V2] ✅ 文本ping发送成功", symbol)
					}
				}
			}()

			// 主读取循环
			connectionBroken := false
			// 设置读取超时：40秒（20秒心跳间隔 + 20秒容差）
			conn.SetReadDeadline(time.Now().Add(40 * time.Second))
			logrus.Debugf("%s [DeepCoin V2] 使用文本消息心跳机制（ping/pong）", symbol)

		readLoop:
			for {
				select {
				case <-pm.stopChan:
					close(stopHeartbeat)
					pingTicker.Stop()
					<-heartbeatDone // 等待心跳goroutine完全退出
					conn.Close()
					return
				default:
				}

				msgType, message, err := conn.ReadMessage()
				if err != nil {
					// 判断是否为超时错误
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						logrus.Errorf("%s DeepCoin V2读取超时 (40秒内未收到任何消息): %v", symbol, err)
					} else if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
						logrus.Warnf("%s DeepCoin V2服务器正常关闭连接: %v", symbol, err)
					} else {
						logrus.Errorf("%s DeepCoin V2读取消息失败: %v", symbol, err)
					}
					connectionBroken = true
					break readLoop
				}

				// 每次成功读取消息后重置超时
				conn.SetReadDeadline(time.Now().Add(40 * time.Second))

				// 处理文本"pong"消息（心跳响应）
				if msgType == websocket.TextMessage && string(message) == "pong" {
					logrus.Debugf("%s [DeepCoin V2] ⬅️ 收到文本pong响应 ❤️", symbol)
					continue
				}

				// 记录收到的消息类型
				msgTypeStr := "unknown"
				switch msgType {
				case websocket.TextMessage:
					msgTypeStr = "text"
				case websocket.BinaryMessage:
					msgTypeStr = "binary"
				case websocket.PingMessage:
					msgTypeStr = "ping"
				case websocket.PongMessage:
					msgTypeStr = "pong"
				}

				logrus.Debugf("%s [DeepCoin V2] ⬅️ 收到%s消息，长度: %d", symbol, msgTypeStr, len(message))
				pm.handleDeepCoinMessage_V2(symbol, message)
			}

			// 清理资源
			close(stopHeartbeat)
			pingTicker.Stop()
			conn.Close()
			// 等待心跳goroutine完全退出
			<-heartbeatDone

			// 清除内存中的DeepCoin价格，避免用旧价格比较
			pm.mu.Lock()
			pm.deepcoinPrices[symbol] = 0
			pm.mu.Unlock()

			if connectionBroken {
				logrus.Warnf("%s DeepCoin V2连接断开，已清除价格缓存，3秒后重连", symbol)
				time.Sleep(3 * time.Second)
			}
		}
	}
}

// subscribeDeepCoin_old_version 订阅DeepCoin价格（旧版本）
func (pm *PriceMonitor) subscribeDeepCoin_old_version(symbol string) {
	defer pm.wg.Done()

	url := "wss://stream.deepcoin.com/public/ws"

	for {
		select {
		case <-pm.stopChan:
			return
		default:
			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				// 清除价格缓存
				pm.mu.Lock()
				pm.deepcoinPrices[symbol] = 0
				pm.mu.Unlock()
				logrus.Errorf("%s DeepCoin连接失败: %v, 已清除价格缓存，1秒后重试", symbol, err)
				time.Sleep(1 * time.Second)
				continue
			}

			logrus.Infof("%s DeepCoin WebSocket已连接", symbol)

			// 发送订阅消息
			subscribeMsg := map[string]interface{}{
				"SendTopicAction": map[string]interface{}{
					"Action":      "1",
					"FilterValue": fmt.Sprintf("DeepCoin_%s", symbol),
					"LocalNo":     6,
					"ResumeNo":    -1,
					"TopicID":     "7",
				},
			}
			if err := conn.WriteJSON(subscribeMsg); err != nil {
				logrus.Errorf("%s DeepCoin发送订阅消息失败: %v", symbol, err)
				conn.Close()
				// 清除价格缓存
				pm.mu.Lock()
				pm.deepcoinPrices[symbol] = 0
				pm.mu.Unlock()
				time.Sleep(1 * time.Second)
				continue
			}

			// 设置ping
			ticker := time.NewTicker(20 * time.Second)
			defer ticker.Stop()

			// 读取消息
			for {
				select {
				case <-pm.stopChan:
					conn.Close()
					return
				case <-ticker.C:
					if err := conn.WriteMessage(websocket.PingMessage, []byte("ping")); err != nil {
						logrus.Errorf("%s DeepCoin ping失败: %v", symbol, err)
						conn.Close()
						goto reconnect
					}
				default:
					conn.SetReadDeadline(time.Now().Add(30 * time.Second))
					_, message, err := conn.ReadMessage()
					if err != nil {
						logrus.Errorf("%s DeepCoin读取消息失败: %v", symbol, err)
						conn.Close()
						goto reconnect
					}

					// 处理pong消息
					if string(message) == "pong" {
						continue
					}

					pm.handleDeepCoinMessage(symbol, message)
				}
			}

		reconnect:
			// 清除内存中的DeepCoin价格，避免用旧价格比较
			pm.mu.Lock()
			pm.deepcoinPrices[symbol] = 0
			pm.mu.Unlock()
			logrus.Warnf("%s DeepCoin连接断开，已清除价格缓存，1秒后重连", symbol)
			time.Sleep(1 * time.Second)
		}
	}
}

// handleBinanceMessage 处理币安消息
func (pm *PriceMonitor) handleBinanceMessage(symbol string, message []byte) {
	// 打印接收到的原始数据
	logrus.Debugf("%s [币安] 收到消息: %s", symbol, string(message))

	var data map[string]interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		logrus.Warnf("%s [币安] JSON解析失败: %v, 原始数据: %s", symbol, err, string(message))
		return
	}

	// 检查是否是订阅确认消息（包含result字段）
	if result, ok := data["result"]; ok {
		logrus.Debugf("%s [币安] 收到订阅确认消息: %v", symbol, result)
		return
	}

	// 检查是否是aggTrade事件
	if data["e"] != "aggTrade" {
		logrus.Debugf("%s [币安] 非aggTrade事件，跳过: %v", symbol, data["e"])
		return
	}

	// 获取价格
	priceStr, ok := data["p"].(string)
	if !ok {
		logrus.Warnf("%s [币安] 价格字段不存在或类型错误: %v", symbol, data)
		return
	}

	var price float64
	if _, err := fmt.Sscanf(priceStr, "%f", &price); err != nil {
		logrus.Warnf("%s [币安] 价格解析失败: %v, 价格字符串: %s", symbol, err, priceStr)
		return
	}

	logrus.Debugf("%s [币安] 价格更新: %.8f (原始数据: %s)", symbol, price, string(message))

	pm.mu.Lock()
	pm.binancePrices[symbol] = price
	deepPrice := pm.deepcoinPrices[symbol]
	pm.mu.Unlock()

	// 检查价差（在goroutine外检查，避免重复发送）
	if deepPrice > 0 {
		pm.checkPriceDiff(symbol, price, deepPrice)
	}
}

// handleDeepCoinMessageNew 处理DeepCoin消息（新协议）
func (pm *PriceMonitor) handleDeepCoinMessageNew(symbol string, message []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		logrus.Warnf("%s [DeepCoin-新] JSON解析失败: %v, 原始数据: %s", symbol, err, string(message))
		return
	}

	// 检查消息类型 "a"
	msgType, ok := data["a"].(string)
	if !ok {
		logrus.Debugf("%s [DeepCoin-新] 消息类型字段不存在，跳过", symbol)
		return
	}

	// 只处理 "PO" 类型（最新行情）
	if msgType != "PO" {
		// 记录订阅确认等其他消息
		if msgType == "RecvTopicAction" {
			logrus.Infof("%s [DeepCoin-新] ✅ 订阅确认成功", symbol)
		} else {
			logrus.Debugf("%s [DeepCoin-新] 非行情消息，类型: %s，跳过", symbol, msgType)
		}
		return
	}

	// 检查消息状态
	if status, ok := data["m"].(string); ok && status != "Success" {
		logrus.Warnf("%s [DeepCoin-新] 消息状态异常: %s", symbol, status)
		return
	}

	// 解析 "r" 数组
	resultArray, ok := data["r"].([]interface{})
	if !ok || len(resultArray) == 0 {
		logrus.Debugf("%s [DeepCoin-新] r字段不存在或为空: %v", symbol, data)
		return
	}

	// 获取第一个元素
	resultItem, ok := resultArray[0].(map[string]interface{})
	if !ok {
		logrus.Warnf("%s [DeepCoin-新] r[0]类型错误: %v", symbol, resultArray[0])
		return
	}

	// 获取 "d" 字段（数据对象）
	dataField, ok := resultItem["d"].(map[string]interface{})
	if !ok {
		logrus.Warnf("%s [DeepCoin-新] d字段不存在或类型错误: %v", symbol, resultItem)
		return
	}

	// 打印data字段内容
	logrus.Debugf("%s [DeepCoin-新] data字段内容: %+v", symbol, dataField)

	// 获取最新价 "N"
	var price float64
	var found bool

	// 尝试 float64 类型
	if n, ok := dataField["N"].(float64); ok {
		price = n
		found = true
	} else if nStr, ok := dataField["N"].(string); ok {
		// 尝试字符串类型
		if _, err := fmt.Sscanf(nStr, "%f", &price); err == nil {
			found = true
		}
	}

	if !found {
		logrus.Warnf("%s [DeepCoin-新] 未找到最新价字段N，可用字段: %v", symbol, getMapKeys(dataField))
		return
	}

	logrus.Debugf("%s [DeepCoin-新] 价格更新: %.2f", symbol, price)

	pm.mu.Lock()
	pm.deepcoinPrices[symbol] = price
	binPrice := pm.binancePrices[symbol]
	pm.mu.Unlock()

	// 检查价差（在goroutine外检查，避免重复发送）
	if binPrice > 0 {
		pm.checkPriceDiff(symbol, binPrice, price)
	}
}

// handleDeepCoinMessage 处理DeepCoin消息（旧协议）
func (pm *PriceMonitor) handleDeepCoinMessage(symbol string, message []byte) {
	// 打印接收到的原始数据
	logrus.Debugf("%s [DeepCoin] 收到消息: %s", symbol, string(message))

	var data map[string]interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		logrus.Warnf("%s [DeepCoin] JSON解析失败: %v, 原始数据: %s", symbol, err, string(message))
		return
	}
	// 检查是否是订阅确认消息（包含Action字段），跳过这些消息
	if _, ok := data["Action"]; ok {
		logrus.Debugf("%s [DeepCoin] 收到订阅确认消息，跳过: %s", symbol, string(message))
		return
	}

	// 解析result字段
	result, ok := data["result"].([]interface{})
	if !ok || len(result) == 0 {
		logrus.Debugf("%s [DeepCoin] result字段不存在或为空: %v", symbol, data)
		return
	}

	resultData, ok := result[0].(map[string]interface{})
	if !ok {
		logrus.Warnf("%s [DeepCoin] result[0]类型错误: %v", symbol, result[0])
		return
	}

	dataField, ok := resultData["data"].(map[string]interface{})
	if !ok {
		logrus.Warnf("%s [DeepCoin] data字段不存在或类型错误: %v", symbol, resultData)
		return
	}

	// 打印data字段内容
	logrus.Debugf("%s [DeepCoin] data字段内容: %+v", symbol, dataField)

	// 尝试多种字段名获取价格（参照Python代码：LastPrice, last, price）
	var price float64
	var found bool
	var priceField string

	if lastPrice, ok := dataField["LastPrice"].(float64); ok {
		price = lastPrice
		found = true
		priceField = "LastPrice"
	} else if lastPrice, ok := dataField["last"].(float64); ok {
		price = lastPrice
		found = true
		priceField = "last"
	} else if lastPrice, ok := dataField["price"].(float64); ok {
		price = lastPrice
		found = true
		priceField = "price"
	} else if priceStr, ok := dataField["LastPrice"].(string); ok {
		if _, err := fmt.Sscanf(priceStr, "%f", &price); err == nil {
			found = true
			priceField = "LastPrice(string)"
		}
	} else if priceStr, ok := dataField["last"].(string); ok {
		if _, err := fmt.Sscanf(priceStr, "%f", &price); err == nil {
			found = true
			priceField = "last(string)"
		}
	} else if priceStr, ok := dataField["price"].(string); ok {
		if _, err := fmt.Sscanf(priceStr, "%f", &price); err == nil {
			found = true
			priceField = "price(string)"
		}
	}

	if !found {
		logrus.Infof("dataField: %v", dataField)
		logrus.Warnf("%s [DeepCoin] 未找到价格字段，可用字段: %v", symbol, getMapKeys(dataField))
		return
	}

	logrus.Debugf("%s [DeepCoin] 价格更新: %.8f (字段: %s, 原始数据: %s)", symbol, price, priceField, string(message))

	pm.mu.Lock()
	pm.deepcoinPrices[symbol] = price
	binPrice := pm.binancePrices[symbol]
	pm.mu.Unlock()

	// 检查价差（在goroutine外检查，避免重复发送）
	if binPrice > 0 {
		pm.checkPriceDiff(symbol, binPrice, price)
	}
}

// handleDeepCoinMessage_V2 处理DeepCoin V2版本消息
func (pm *PriceMonitor) handleDeepCoinMessage_V2(symbol string, message []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		logrus.Warnf("%s [DeepCoin V2] JSON解析失败: %v, 原始数据: %s", symbol, err, string(message))
		return
	}
	logrus.Debugf("handleDeepCoinMessage_V2 data: %+v", data)

	// 检查消息类型 "a"
	msgType, ok := data["a"].(string)
	if !ok {
		logrus.Debugf("%s [DeepCoin V2] 消息类型字段不存在，跳过", symbol)
		return
	}

	// 只处理 "PO" 类型（最新行情）
	if msgType != "PO" {
		// 记录订阅确认等其他消息
		logrus.Debugf("%s [DeepCoin V2] 非行情消息，类型: %s，跳过", symbol, msgType)
		return
	}

	// 检查消息状态
	if status, ok := data["m"].(string); ok && status != "Success" {
		logrus.Warnf("%s [DeepCoin V2] 消息状态异常: %s", symbol, status)
		return
	}

	// 获取 "d" 字段（可能是数组或对象）
	var dataField map[string]interface{}
	dValue, ok := data["d"]
	if !ok {
		logrus.Warnf("%s [DeepCoin V2] d字段不存在: %v", symbol, data)
		return
	}

	// 检查 d 是否是数组
	if dArray, ok := dValue.([]interface{}); ok {
		// d 是数组，取第一个元素
		if len(dArray) == 0 {
			logrus.Warnf("%s [DeepCoin V2] d数组为空", symbol)
			return
		}
		dataField, ok = dArray[0].(map[string]interface{})
		if !ok {
			logrus.Warnf("%s [DeepCoin V2] d[0]类型错误: %v", symbol, dArray[0])
			return
		}
	} else if dMap, ok := dValue.(map[string]interface{}); ok {
		// d 直接是对象
		dataField = dMap
	} else {
		logrus.Warnf("%s [DeepCoin V2] d字段类型错误，期望数组或对象，实际: %T, 值: %v", symbol, dValue, dValue)
		return
	}

	// 打印data字段内容
	logrus.Debugf("%s [DeepCoin V2] data字段内容: %+v", symbol, dataField)

	// V2版本中，最新价字段是 "N"
	var price float64
	var found bool
	var priceField string

	if lastPrice, ok := dataField["N"].(float64); ok {
		price = lastPrice
		found = true
		priceField = "N"
	} else if priceStr, ok := dataField["N"].(string); ok {
		if _, err := fmt.Sscanf(priceStr, "%f", &price); err == nil {
			found = true
			priceField = "N(string)"
		}
	}

	if !found {
		logrus.Infof("dataField: %v", dataField)
		logrus.Warnf("%s [DeepCoin V2] 未找到价格字段N，可用字段: %v", symbol, getMapKeys(dataField))
		return
	}

	logrus.Debugf("%s [DeepCoin V2] 价格更新: %.8f (字段: %s, 原始数据: %s)", symbol, price, priceField, string(message))

	pm.mu.Lock()
	pm.deepcoinPrices[symbol] = price
	binPrice := pm.binancePrices[symbol]
	pm.mu.Unlock()

	// 检查价差（在goroutine外检查，避免重复发送）
	if binPrice > 0 {
		pm.checkPriceDiff(symbol, binPrice, price)
	}
}

// getMapKeys 获取map的所有key（用于调试）
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// checkPriceDiff 检查价差并发送通知
func (pm *PriceMonitor) checkPriceDiff(symbol string, binPrice, deepPrice float64) {
	if binPrice == 0 || deepPrice == 0 {
		return
	}

	config, ok := pm.symbolConfigs[symbol]
	if !ok {
		return
	}

	// 计算价差百分比（用中间价做分母，避免多/空方向不对称）
	diff := math.Abs(binPrice - deepPrice)
	midPrice := (binPrice + deepPrice) / 2
	diffPercent := diff / midPrice

	// 如果价差达到阈值
	if diffPercent >= config.Threshold {
		// 记录日志
		timestamp := utils.GetCurrentTimeString()
		logrus.Infof("[%s] %s 价差达到阈值: %.4f%%, 阈值: %.4f%%, Binance: %.8f, DeepCoin: %.8f",
			timestamp, symbol, diffPercent*100, config.Threshold*100, binPrice, deepPrice)

		// 检查账户余额是否充足
		accountMonitor := GetAccountMonitor()
		if accountMonitor != nil && !accountMonitor.HasSufficientBalance() {
			// 余额不足，发送余额不足告警
			insufficientAccounts := accountMonitor.GetInsufficientAccounts()
			go pm.sendInsufficientBalanceAlert(symbol, binPrice, deepPrice, diffPercent, config.Threshold, insufficientAccounts)
			logrus.Warnf("⚠️ %s 价差达到阈值但余额不足，已发送告警，未执行交易", symbol)
			return
		}

		// 发送Telegram告警
		go pm.sendTelegramAlert(symbol, binPrice, deepPrice, diffPercent, config.Threshold)

		// 执行套利交易（受开关控制）
		if vipper.GetBool("monitor.trade.enabled") {
			go pm.executeArbitrageTrade(symbol, config.TradeInst, binPrice, deepPrice)
		} else {
			logrus.Infof("🔒 %s 价差达到阈值但开仓开关已关闭，跳过下单", symbol)
		}
	} else {
		timestamp := utils.GetCurrentTimeString()
		logrus.Debugf("[%s] %s 价差: %.4f%%, 阈值: %.4f%%, Binance: %.8f, DeepCoin: %.8f",
			timestamp, symbol, diffPercent*100, config.Threshold*100, binPrice, deepPrice)
	}
}

// sendTelegramAlert 发送Telegram告警
func (pm *PriceMonitor) sendTelegramAlert(symbol string, binPrice, deepPrice, diffPercent, threshold float64) {
	// 告警冷却按方向区分，避免多单告警冷却吃掉空单告警
	alertDirection := "long"
	if deepPrice > binPrice {
		alertDirection = "short"
	}
	alertKey := symbol + ":" + alertDirection

	// 双重检查模式：先读后写，减少锁竞争
	// 1. 先用读锁快速检查（大部分goroutine会在这里被拦截）
	pm.alertMu.Lock()
	lastTime, exists := pm.lastAlertTime[alertKey]
	now := time.Now()

	if exists && now.Sub(lastTime) < 1*time.Second {
		pm.alertMu.Unlock()
		logrus.Debugf("%s[%s] 告警过于频繁，跳过发送 (距上次: %.1fs)", symbol, alertDirection, now.Sub(lastTime).Seconds())
		return
	}

	// 2. 通过检查，立即更新时间并释放锁（这样后续的goroutine会被拦截）
	pm.lastAlertTime[alertKey] = now
	pm.alertMu.Unlock()

	// 3. 发送消息（不持有锁，不影响其他检查）
	var direction string
	var action string

	if binPrice > deepPrice {
		direction = "币安 > DeepCoin"
		action = "BUY"
	} else {
		direction = "DeepCoin > 币安"
		action = "SELL"
	}

	message := fmt.Sprintf(
		"🚨 价差告警\n\n"+
			"币种: %s\n"+
			"方向: %s\n"+
			"操作: %s\n"+
			"币安价格: %.8f\n"+
			"DeepCoin价格: %.8f\n"+
			"价差: %.4f%%\n"+
			"阈值: %.4f%%\n"+
			"时间: %s",
		symbol,
		direction,
		action,
		binPrice,
		deepPrice,
		diffPercent*100,
		threshold*100,
		utils.GetCurrentTimeString(),
	)

	success, err := pm.telegramClient.SendMessage(message)
	if err != nil {
		logrus.Errorf("%s 发送Telegram消息失败: %v", symbol, err)
	} else if success {
		logrus.Infof("%s 价差告警已发送: %.4f%%", symbol, diffPercent*100)
	}
}

// sendInsufficientBalanceAlert 发送余额不足告警
func (pm *PriceMonitor) sendInsufficientBalanceAlert(symbol string, binPrice, deepPrice, diffPercent, threshold float64, insufficientAccounts []string) {
	// 余额不足告警也按方向区分冷却
	alertDirection := "long"
	if deepPrice > binPrice {
		alertDirection = "short"
	}
	alertKey := symbol + ":" + alertDirection + ":insuf"

	// 双重检查模式：先读后写，减少锁竞争
	pm.alertMu.Lock()
	lastTime, exists := pm.lastAlertTime[alertKey]
	now := time.Now()

	if exists && now.Sub(lastTime) < 1*time.Second {
		pm.alertMu.Unlock()
		logrus.Debugf("%s[%s] 余额不足告警过于频繁，跳过发送 (距上次: %.1fs)", symbol, alertDirection, now.Sub(lastTime).Seconds())
		return
	}

	// 通过检查，立即更新时间并释放锁
	pm.lastAlertTime[alertKey] = now
	pm.alertMu.Unlock()

	// 构建余额不足的账户列表
	accountList := ""
	if len(insufficientAccounts) > 0 {
		for i, acc := range insufficientAccounts {
			if i > 0 {
				accountList += ", "
			}
			accountList += acc
		}
	} else {
		accountList = "未知账户"
	}

	var direction string
	var action string

	if binPrice > deepPrice {
		direction = "币安 > DeepCoin"
		action = "BUY"
	} else {
		direction = "DeepCoin > 币安"
		action = "SELL"
	}

	message := fmt.Sprintf(
		"⚠️ 价差达到但余额不足\n\n"+
			"币种: %s\n"+
			"方向: %s\n"+
			"操作: %s\n"+
			"币安价格: %.8f\n"+
			"DeepCoin价格: %.8f\n"+
			"价差: %.4f%%\n"+
			"阈值: %.4f%%\n"+
			"余额不足账户: %s\n"+
			"最小余额要求: 30 USDT\n"+
			"时间: %s\n\n"+
			"❌ 未执行套利交易",
		symbol,
		direction,
		action,
		binPrice,
		deepPrice,
		diffPercent*100,
		threshold*100,
		accountList,
		utils.GetCurrentTimeString(),
	)

	success, err := pm.telegramClient.SendMessage(message)
	if err != nil {
		logrus.Errorf("%s 发送余额不足告警失败: %v", symbol, err)
	} else if success {
		logrus.Warnf("%s 余额不足告警已发送: 价差=%.4f%%, 余额不足账户=%v", symbol, diffPercent*100, insufficientAccounts)
	}
}

// executeArbitrageTrade 执行套利交易
func (pm *PriceMonitor) executeArbitrageTrade(symbol, instId string, binPrice, deepPrice float64) {
	// 确定本次交易方向（用于方向独立冷却）
	direction := "long"
	if deepPrice > binPrice {
		direction = "short"
	}
	tradeKey := symbol + ":" + direction

	// 双重检查模式：先读后写，减少锁竞争
	// 1. 先用锁快速检查（大部分goroutine会在这里被拦截）
	pm.tradeMu.Lock()
	lastTime, exists := pm.lastTradeTime[tradeKey]
	now := time.Now()

	if exists && now.Sub(lastTime) < 2*time.Second {
		pm.tradeMu.Unlock()
		logrus.Debugf("%s[%s] 套利交易过于频繁，跳过执行 (距上次: %.1fs)", symbol, direction, now.Sub(lastTime).Seconds())
		return
	}

	// 2. 通过检查，立即更新时间并释放锁（这样后续的goroutine会被拦截）
	pm.lastTradeTime[tradeKey] = now
	pm.tradeMu.Unlock()

	// 3. 执行交易（不持有锁，不影响其他检查）
	logrus.Infof("🎯 %s[%s] 触发套利交易: 币安=%.2f, DeepCoin=%.2f, 合约=%s",
		symbol, direction, binPrice, deepPrice, instId)

	executeArbitrageTradeInternal(instId, binPrice, deepPrice)
}

// getSymbolList 获取币种列表
func (pm *PriceMonitor) getSymbolList() []string {
	symbols := make([]string, 0, len(pm.symbolConfigs))
	for symbol := range pm.symbolConfigs {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// GetPrices 获取当前价格（用于调试）
func (pm *PriceMonitor) GetPrices(symbol string) (binPrice, deepPrice float64) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.binancePrices[symbol], pm.deepcoinPrices[symbol]
}
