package monitor

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"common/middleware/vipper"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/shopspring/decimal"
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

// ClosePositionCommandHandler 一键平仓命令处理器
type ClosePositionCommandHandler struct{}

func (h *ClosePositionCommandHandler) Keywords() []string {
	return []string{"平仓", "close"}
}

func (h *ClosePositionCommandHandler) Handle(msg *tgbotapi.Message) string {
	return CloseAllPositions()
}

type HelpCommandHandler struct{}

func (h *HelpCommandHandler) Keywords() []string {
	return []string{"help", "/help", "帮助"}
}

func (h *HelpCommandHandler) Handle(msg *tgbotapi.Message) string {
	return formatTelegramHelpMessage()
}

type ApproveAICloseCommandHandler struct{}

func (h *ApproveAICloseCommandHandler) Keywords() []string {
	return []string{"确认平仓", "批准平仓", "approve"}
}

func (h *ApproveAICloseCommandHandler) Handle(msg *tgbotapi.Message) string {
	id := extractAICloseRequestID(msg.Text)
	if id == "" {
		return fmt.Sprintf("⚠️ 请带上请求ID，例如: %s 确认平仓 AICLOSE-123", botMentionOrFallback())
	}
	return ApprovePendingAIClose(id)
}

type RejectAICloseCommandHandler struct{}

func (h *RejectAICloseCommandHandler) Keywords() []string {
	return []string{"拒绝平仓", "取消平仓", "reject"}
}

func (h *RejectAICloseCommandHandler) Handle(msg *tgbotapi.Message) string {
	id := extractAICloseRequestID(msg.Text)
	if id == "" {
		return fmt.Sprintf("⚠️ 请带上请求ID，例如: %s 拒绝平仓 AICLOSE-123", botMentionOrFallback())
	}
	return RejectPendingAIClose(id)
}

type ListAICloseCommandHandler struct{}

func (h *ListAICloseCommandHandler) Keywords() []string {
	return []string{"待审批平仓", "审批列表", "pending"}
}

func (h *ListAICloseCommandHandler) Handle(msg *tgbotapi.Message) string {
	return ListPendingAICloseRequests()
}

type RunAICloseCommandHandler struct{}

func (h *RunAICloseCommandHandler) Keywords() []string {
	return []string{"AI", "ai", "AI平仓", "ai平仓"}
}

func (h *RunAICloseCommandHandler) Handle(msg *tgbotapi.Message) string {
	if manualInput, ok := extractManualAICloseInput(msg.Text); ok {
		return RunAICloseStrategyWithManualPosition(manualInput)
	}
	return RunAICloseStrategyNow()
}

type RunAIOpenCommandHandler struct{}

func (h *RunAIOpenCommandHandler) Keywords() []string {
	return []string{"AI加仓", "ai加仓", "加仓"}
}

func (h *RunAIOpenCommandHandler) Handle(msg *tgbotapi.Message) string {
	return RunAIOpenStrategyNow()
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
	tb.RegisterCommand(&HelpCommandHandler{})
	// 注册余额查询处理器
	tb.RegisterCommand(&BalanceCommandHandler{})
	// 注册持仓查询处理器
	tb.RegisterCommand(&PositionCommandHandler{})
	// 注册一键平仓处理器
	tb.RegisterCommand(&ClosePositionCommandHandler{})
	tb.RegisterCommand(&RunAICloseCommandHandler{})
	tb.RegisterCommand(&RunAIOpenCommandHandler{})
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
	close(tb.stopChan)
	tb.wg.Wait()
	logrus.Info("Telegram Bot已停止")
}

func (tb *TelegramBot) BotName() string {
	return tb.botName
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

	if !hasMention && !isHelpCommandText(text) {
		return
	}

	// 查找匹配的命令处理器
	textLower := strings.ToLower(text)
	var matchedHandler CommandHandler
	longestKeyword := 0

	tb.mu.RLock()
	for keyword, handler := range tb.handlers {
		if strings.Contains(textLower, keyword) && len(keyword) > longestKeyword {
			matchedHandler = handler
			longestKeyword = len(keyword)
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

func extractAICloseRequestID(text string) string {
	for _, token := range strings.Fields(text) {
		normalized := strings.Trim(token, " ,，。:：;；()[]{}")
		if strings.HasPrefix(strings.ToUpper(normalized), "AICLOSE-") {
			return normalized
		}
	}
	return ""
}

func isHelpCommandText(text string) bool {
	normalized := strings.ToLower(strings.TrimSpace(text))
	return normalized == "help" || normalized == "/help" || normalized == "帮助"
}

func formatTelegramHelpMessage() string {
	return strings.Join([]string{
		"📖 支持的命令",
		"",
		"help / 帮助: 查看这份命令列表",
		"余额 / balance: 查询账户余额",
		"仓位 / 持仓 / position: 查询当前持仓",
		"平仓 / close: 一键平仓所有账户持仓",
		"AI: 按当前真实仓位执行 AI 策略建议",
		"AI + 75000: 用手工均价执行 AI 策略建议，不查询当前仓位",
		"AI + 75000 + L: 用手工均价按做多仓位执行 AI 策略建议",
		"AI + 75000 + S: 用手工均价按做空仓位执行 AI 策略建议",
		"AI + 75000 + L + 100 + 30 + 125: 用手工均价、方向、余额、张数、杠杆倍数执行 AI 策略建议（1张=0.001BTC，倍数默认125，全仓估算）",
	}, "\n")
}

type manualAICloseInput struct {
	AvgPrice     decimal.Decimal
	PositionSide string
	Balance      decimal.Decimal
	PositionSize decimal.Decimal
	Leverage     decimal.Decimal
}

var manualAICloseInputPattern = regexp.MustCompile(`(?i)(?:^|\s)ai(?:平仓)?\s*\+\s*([0-9]+(?:\.[0-9]+)?)(?:\s*\+\s*([ls]))?(?:\s*\+\s*([0-9]+(?:\.[0-9]+)?))?(?:\s*\+\s*([0-9]+(?:\.[0-9]+)?))?(?:\s*\+\s*([0-9]+(?:\.[0-9]+)?))?`)

func extractManualAICloseInput(text string) (manualAICloseInput, bool) {
	match := manualAICloseInputPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		return manualAICloseInput{}, false
	}
	price, err := decimal.NewFromString(match[1])
	if err != nil || !price.IsPositive() {
		return manualAICloseInput{}, false
	}

	input := manualAICloseInput{AvgPrice: price}
	if len(match) >= 3 {
		switch strings.ToUpper(strings.TrimSpace(match[2])) {
		case "L":
			input.PositionSide = "long"
		case "S":
			input.PositionSide = "short"
		}
	}
	if len(match) >= 4 && strings.TrimSpace(match[3]) != "" {
		balance, err := decimal.NewFromString(match[3])
		if err == nil && balance.IsPositive() {
			input.Balance = balance
		}
	}
	if len(match) >= 5 && strings.TrimSpace(match[4]) != "" {
		size, err := decimal.NewFromString(match[4])
		if err == nil && size.IsPositive() {
			input.PositionSize = size
		}
	}
	if len(match) >= 6 && strings.TrimSpace(match[5]) != "" {
		leverage, err := decimal.NewFromString(match[5])
		if err == nil && leverage.IsPositive() {
			input.Leverage = leverage
		}
	}
	if !input.Leverage.IsPositive() {
		input.Leverage = decimal.NewFromInt(125)
	}
	return input, true
}

func botMentionOrFallback() string {
	if mention := GetTelegramBotMention(); mention != "" {
		return mention
	}
	return "@你的Bot"
}
