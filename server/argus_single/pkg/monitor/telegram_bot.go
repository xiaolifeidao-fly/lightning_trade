package monitor

import (
	"strings"
	"sync"

	"common/middleware/vipper"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

// CommandHandler 命令处理器接口
type CommandHandler interface {
	// Handle 处理命令，返回响应消息
	Handle(msg *tgbotapi.Message) string
	// Keywords 返回该处理器支持的关键词列表（不区分大小写）
	Keywords() []string
}

// TelegramBot Telegram Bot处理器
type TelegramBot struct {
	bot      *tgbotapi.BotAPI
	botName  string                    // Bot用户名（动态获取）
	handlers map[string]CommandHandler // 关键词到处理器的映射
	mu       sync.RWMutex              // 保护handlers的读写锁
	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewTelegramBot 创建Telegram Bot处理器
func NewTelegramBot() *TelegramBot {
	tb := &TelegramBot{
		stopChan: make(chan struct{}),
		handlers: make(map[string]CommandHandler),
	}
	// 注册默认命令处理器
	tb.registerDefaultHandlers()
	return tb
}

// BalanceCommandHandler 余额查询命令处理器
type BalanceCommandHandler struct{}

// Keywords 返回余额查询支持的关键词
func (h *BalanceCommandHandler) Keywords() []string {
	return []string{"余额", "balance", "yu e"}
}

// Handle 处理余额查询命令
func (h *BalanceCommandHandler) Handle(msg *tgbotapi.Message) string {
	return GetBalanceReport()
}

// PositionCommandHandler 持仓查询命令处理器
type PositionCommandHandler struct{}

// Keywords 返回持仓查询支持的关键词
func (h *PositionCommandHandler) Keywords() []string {
	return []string{"仓位", "持仓", "position"}
}

// Handle 处理持仓查询命令
func (h *PositionCommandHandler) Handle(msg *tgbotapi.Message) string {
	return GetPositionReport()
}

// ROICommandHandler 收益率查询命令处理器
type ROICommandHandler struct{}

func (h *ROICommandHandler) Keywords() []string {
	return []string{"roi", "收益率"}
}

func (h *ROICommandHandler) Handle(msg *tgbotapi.Message) string {
	return GetROIReport()
}

// StatusCommandHandler 系统状态查询命令处理器
type StatusCommandHandler struct{}

func (h *StatusCommandHandler) Keywords() []string {
	return []string{"status", "状态"}
}

func (h *StatusCommandHandler) Handle(msg *tgbotapi.Message) string {
	return GetSystemStatus()
}

// ClosePositionCommandHandler 一键平仓命令处理器
type ClosePositionCommandHandler struct{}

func (h *ClosePositionCommandHandler) Keywords() []string {
	return []string{"平仓", "close"}
}

func (h *ClosePositionCommandHandler) Handle(msg *tgbotapi.Message) string {
	text := strings.ToLower(msg.Text)
	if strings.Contains(text, "long") || strings.Contains(text, "多") {
		return ClosePositionsBySide("long")
	}
	if strings.Contains(text, "short") || strings.Contains(text, "空") {
		return ClosePositionsBySide("short")
	}
	return CloseAllPositions()
}

// RegisterCommand 注册命令处理器
func (tb *TelegramBot) RegisterCommand(handler CommandHandler) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for _, keyword := range handler.Keywords() {
		keywordLower := strings.ToLower(keyword)
		tb.handlers[keywordLower] = handler
		logrus.Infof("注册命令处理器: 关键词=%s", keyword)
	}
}

// registerDefaultHandlers 注册默认的命令处理器
func (tb *TelegramBot) registerDefaultHandlers() {
	// 注册余额查询处理器
	tb.RegisterCommand(&BalanceCommandHandler{})
	// 注册持仓查询处理器
	tb.RegisterCommand(&PositionCommandHandler{})
	// 注册收益率查询处理器
	tb.RegisterCommand(&ROICommandHandler{})
	// 注册系统状态查询处理器
	tb.RegisterCommand(&StatusCommandHandler{})
	// 注册一键平仓处理器
	tb.RegisterCommand(&ClosePositionCommandHandler{})
}

// Start 启动Telegram Bot消息监听
func (tb *TelegramBot) Start() {
	// 从配置文件读取Bot Token
	botToken := vipper.GetString("telegram.bot_token")
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		logrus.Errorf("创建Telegram Bot失败: %v", err)
		return
	}

	tb.bot = bot
	tb.botName = "@" + bot.Self.UserName // 动态获取Bot用户名
	logrus.Infof("Telegram Bot已启动，Bot用户名: %s", tb.botName)

	// 启动消息监听
	tb.wg.Add(1)
	go func() {
		defer tb.wg.Done()
		tb.startPolling()
	}()
}

// Stop 停止Telegram Bot
func (tb *TelegramBot) Stop() {
	tb.stopOnce.Do(func() {
		close(tb.stopChan)
		tb.wg.Wait()
		logrus.Info("Telegram Bot已停止")
	})
}

// startPolling 启动消息轮询（使用Long Polling）
func (tb *TelegramBot) startPolling() {
	// 创建更新配置，使用Long Polling
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60 // 60秒超时

	// 获取更新通道
	updates := tb.bot.GetUpdatesChan(u)

	for {
		select {
		case <-tb.stopChan:
			// 停止接收更新
			tb.bot.StopReceivingUpdates()
			return
		case update := <-updates:
			// 处理更新
			if update.Message != nil {
				tb.handleMessage(update.Message)
			}
		}
	}
}

// handleMessage 处理接收到的消息
func (tb *TelegramBot) handleMessage(msg *tgbotapi.Message) {
	if msg.Text == "" {
		return
	}

	text := msg.Text

	// 检查消息entities中的mention
	hasMention := false
	if len(msg.Entities) > 0 {
		for _, entity := range msg.Entities {
			if entity.Type == "mention" {
				if entity.Offset+entity.Length <= len(text) {
					mentionText := text[entity.Offset : entity.Offset+entity.Length]
					if mentionText == tb.botName {
						hasMention = true
						break
					}
				}
			}
		}
	}

	// 如果entities中没有找到，检查文本中是否包含@机器人
	if !hasMention {
		hasMention = strings.Contains(text, tb.botName)
	}

	if !hasMention {
		return
	}

	// 查找匹配的命令处理器
	textLower := strings.ToLower(text)
	var matchedHandler CommandHandler

	tb.mu.RLock()
	for keyword, handler := range tb.handlers {
		if strings.Contains(textLower, keyword) {
			matchedHandler = handler
			break
		}
	}
	tb.mu.RUnlock()

	if matchedHandler == nil {
		return
	}

	// 执行命令处理器
	response := matchedHandler.Handle(msg)

	// 创建回复消息
	replyMsg := tgbotapi.NewMessage(msg.Chat.ID, response)
	replyMsg.ReplyToMessageID = msg.MessageID // 回复原消息

	// 发送消息
	if _, err := tb.bot.Send(replyMsg); err != nil {
		logrus.Errorf("发送命令响应失败: %v", err)
	} else {
		logrus.Infof("命令响应已发送 (ChatID: %d)", msg.Chat.ID)
	}
}
