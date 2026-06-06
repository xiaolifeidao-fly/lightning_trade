package monitor

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"argus_single/pkg/trade"
	"common/middleware/vipper"
	"common/utils"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

// AccountMonitor 账户监控器
type AccountMonitor struct {
	telegramClient     *utils.TelegramClient
	aiCloseDecider     AICloseDecider
	stopChan           chan struct{}
	wg                 sync.WaitGroup
	queryInterval      time.Duration        // 查询间隔，默认1分钟
	reportInterval     time.Duration        // 报告间隔，默认30分钟
	mu                 sync.RWMutex         // 保护余额状态
	lastBalances       []AccountBalanceInfo // 最新的余额信息
	hasInsufficientBal bool                 // 是否有账户余额不足（<30U，用于交易判断）
	hasLowBalance      bool                 // 是否有账户余额低于报告阈值（<100U，用于发送报告）
	minBalance         decimal.Decimal      // 最小余额阈值，默认30U（用于交易判断）
	reportThreshold    decimal.Decimal      // 报告阈值，默认100U（低于此值才发送报告）

	posQueryInterval   time.Duration        // 持仓查询间隔
	pnlProfitThreshold decimal.Decimal      // 盈利告警阈值（正值，如150表示150%）
	pnlLossThreshold   decimal.Decimal      // 亏损告警阈值（正值，如150表示-150%时告警）
	pnlAlertCooldown   time.Duration        // 同一持仓告警冷却时间
	pnlAlertMu         sync.RWMutex         // 保护告警记录
	lastPnlAlerts      map[string]time.Time // 上次告警时间，key: "账户名:instId:posSide"
	aiCheckInterval    time.Duration        // AI定时平仓检查间隔（AI未给建议时的回退值）
	aiCloseMinInterval time.Duration        // AI平仓建议巡检间隔下限
	aiCloseMaxInterval time.Duration        // AI平仓建议巡检间隔上限
	aiApprovalStore    *aiCloseApprovalStore

	aiOpenDecider       AIOpenDecider // AI加仓决策器
	aiOpenCheckInterval time.Duration // AI定时加仓巡检间隔（AI未给建议时的回退值）
	aiOpenMinInterval   time.Duration // AI建议巡检间隔下限
	aiOpenMaxInterval   time.Duration // AI建议巡检间隔上限
	aiOpenAutoTrade     bool          // AI加仓/开仓是否自动下单（false=仅告警）
}

// aiOpenTradeInst 是 AI 开仓/加仓实际下单使用的合约（当前仅支持 BTC）。
const aiOpenTradeInst = "BTCUSDT"

// NewAccountMonitor 创建账户监控器
func NewAccountMonitor() *AccountMonitor {
	intervalSec := vipper.GetInt("position.monitor.interval_seconds")
	if intervalSec <= 0 {
		intervalSec = 5
	}
	profitThreshold := vipper.GetFloat64("position.monitor.profit_threshold")
	if profitThreshold <= 0 {
		profitThreshold = 150
	}
	lossThreshold := vipper.GetFloat64("position.monitor.loss_threshold")
	if lossThreshold <= 0 {
		lossThreshold = 150
	}
	aiCheckMinutes := vipper.GetInt("position.ai_close.interval_minutes")
	if aiCheckMinutes <= 0 {
		aiCheckMinutes = 10
	}
	aiCloseMinMinutes := vipper.GetInt("position.ai_close.min_interval_minutes")
	if aiCloseMinMinutes <= 0 {
		aiCloseMinMinutes = 15
	}
	aiCloseMaxMinutes := vipper.GetInt("position.ai_close.max_interval_minutes")
	if aiCloseMaxMinutes <= 0 {
		aiCloseMaxMinutes = 240
	}
	if aiCloseMaxMinutes < aiCloseMinMinutes {
		aiCloseMaxMinutes = aiCloseMinMinutes
	}

	logrus.Infof("持仓监控配置: 间隔=%ds, 盈利阈值=%.2f%%, 亏损阈值=%.2f%%", intervalSec, profitThreshold, lossThreshold)

	aiCloseDecider := NewAICloseDeciderFromConfig()
	if aiCloseDecider == nil {
		logrus.Infof("AI平仓决策: 已禁用")
	} else {
		logrus.Infof("AI平仓决策: 已启用, provider=%s", vipper.GetString("position.ai_close.provider"))
	}

	aiOpenCheckMinutes := vipper.GetInt("position.ai_open.interval_minutes")
	if aiOpenCheckMinutes <= 0 {
		aiOpenCheckMinutes = 15
	}
	aiOpenMinMinutes := vipper.GetInt("position.ai_open.min_interval_minutes")
	if aiOpenMinMinutes <= 0 {
		aiOpenMinMinutes = 5
	}
	aiOpenMaxMinutes := vipper.GetInt("position.ai_open.max_interval_minutes")
	if aiOpenMaxMinutes <= 0 {
		aiOpenMaxMinutes = 15
	}
	if aiOpenMaxMinutes < aiOpenMinMinutes {
		aiOpenMaxMinutes = aiOpenMinMinutes
	}
	aiOpenAutoTrade := vipper.GetBool("position.ai_open.auto_trade")
	aiOpenDecider := NewAIOpenDeciderFromConfig()
	if aiOpenDecider == nil {
		logrus.Infof("AI加仓决策: 已禁用")
	} else {
		logrus.Infof("AI加仓决策: 已启用, 巡检间隔=%d分钟, 自动下单=%t", aiOpenCheckMinutes, aiOpenAutoTrade)
	}

	return &AccountMonitor{
		telegramClient:     utils.NewTelegramClientWithBotTokenAndChatID(vipper.GetString("telegram.bot_token"), vipper.GetString("telegram.chat_id")),
		aiCloseDecider:     aiCloseDecider,
		stopChan:           make(chan struct{}),
		queryInterval:      1 * time.Minute,  // 1分钟查询一次
		reportInterval:     30 * time.Minute, // 30分钟发送一次报告
		lastBalances:       make([]AccountBalanceInfo, 0),
		hasInsufficientBal: false,
		hasLowBalance:      false,
		minBalance:         decimal.NewFromInt(30), // 最小余额30U（用于交易判断）
		reportThreshold:    decimal.NewFromInt(50), // 报告阈值50U（低于此值才发送报告）
		posQueryInterval:   time.Duration(intervalSec) * time.Second,
		pnlProfitThreshold: decimal.NewFromFloat(profitThreshold),
		pnlLossThreshold:   decimal.NewFromFloat(lossThreshold),
		pnlAlertCooldown:   5 * time.Minute, // 同一持仓5分钟内不重复告警
		lastPnlAlerts:      make(map[string]time.Time),
		aiCheckInterval:    time.Duration(aiCheckMinutes) * time.Minute,
		aiCloseMinInterval: time.Duration(aiCloseMinMinutes) * time.Minute,
		aiCloseMaxInterval: time.Duration(aiCloseMaxMinutes) * time.Minute,
		aiApprovalStore:    newAICloseApprovalStore(defaultAICloseApprovalTimeout),

		aiOpenDecider:       aiOpenDecider,
		aiOpenCheckInterval: time.Duration(aiOpenCheckMinutes) * time.Minute,
		aiOpenMinInterval:   time.Duration(aiOpenMinMinutes) * time.Minute,
		aiOpenMaxInterval:   time.Duration(aiOpenMaxMinutes) * time.Minute,
		aiOpenAutoTrade:     aiOpenAutoTrade,
	}
}

// Start 启动账户监控
func (am *AccountMonitor) Start() {
	logrus.Info("启动账户监控...")

	// 程序启动后立即查询（如果余额低于100才发送报告）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.queryBalances(true) // 查询并检查是否需要发送报告
	}()

	// 启动定时查询任务（1分钟查询一次，但不发送报告）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startPeriodicQuery()
	}()

	// 启动定时报告任务（30分钟发送一次报告）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startPeriodicReport()
	}()

	// 启动持仓盈亏监控（5秒一次）
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startPositionMonitor()
	}()

	// 启动 AI 定时平仓巡检
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startAICloseMonitor()
	}()

	// 启动 AI 定时加仓巡检
	am.wg.Add(1)
	go func() {
		defer am.wg.Done()
		am.startAIOpenMonitor()
	}()

	logrus.Infof("✅ 账户监控已启动，每1分钟查询余额，每%v监控持仓盈亏（盈利>%s%%/亏损<-%s%%），每%v执行一次AI平仓巡检",
		am.posQueryInterval, am.pnlProfitThreshold.String(), am.pnlLossThreshold.String(), am.aiCheckInterval)
}

// Stop 停止账户监控
func (am *AccountMonitor) Stop() {
	close(am.stopChan)
	am.wg.Wait()
	logrus.Info("账户监控已停止")
}

// startPeriodicQuery 启动定时查询（1分钟一次，不发送报告）
func (am *AccountMonitor) startPeriodicQuery() {
	ticker := time.NewTicker(am.queryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.queryBalances(false) // 只查询，不发送报告
		}
	}
}

// startPeriodicReport 启动定时报告（30分钟一次）
func (am *AccountMonitor) startPeriodicReport() {
	ticker := time.NewTicker(am.reportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.sendReport()
		}
	}
}

// queryBalances 查询所有账户余额
// sendReport: 是否发送报告到Telegram
func (am *AccountMonitor) queryBalances(sendReport bool) {
	if !trade.IsInitialized() {
		logrus.Warnf("交易管理器未初始化，跳过账户余额查询")
		return
	}

	tm := trade.GetManager()
	if tm == nil {
		logrus.Warnf("无法获取交易管理器，跳过账户余额查询")
		return
	}

	// 获取账户配置
	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		logrus.Warnf("没有配置账户，跳过余额查询")
		return
	}

	logrus.Info("开始查询账户余额...")

	// 存储所有账户的余额信息
	accountBalances := make([]AccountBalanceInfo, 0, len(config.Accounts))

	// 遍历所有账户，查询余额
	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			logrus.Warnf("账户 %s 的客户端不存在，跳过", acc.Name)
			continue
		}

		// 查询余额
		logrus.Infof("[余额查询] 账户 %s (uid=%s) 开始请求 GetBalancesTyped, instType=SWAP, ccy=USDT", acc.Name, acc.UID)
		balResp, err := client.GetBalancesTyped(&utils.GetBalancesRequest{
			InstType: "SWAP",
			Ccy:      "USDT",
		})

		if err != nil {
			logrus.Errorf("[余额查询] 账户 %s 请求失败: %v", acc.Name, err)
			accountBalances = append(accountBalances, AccountBalanceInfo{
				AccountName:    acc.Name,
				UID:            acc.UID,
				InitialBalance: acc.InitialBalance,
				Error:          err.Error(),
			})
		} else {
			// 打印原始响应，方便排查字段映射问题
			if raw, jerr := json.Marshal(balResp); jerr == nil {
				logrus.Infof("[余额查询] 账户 %s 原始响应: %s", acc.Name, string(raw))
			}
			logrus.Infof("[余额查询] 账户 %s code=%s msg=%s Data条数=%d", acc.Name, balResp.Code, balResp.Msg, len(balResp.Data))

			// 获取USDT余额
			if usdtBal, found := balResp.GetBalance("USDT"); found {
				accountBalances = append(accountBalances, AccountBalanceInfo{
					AccountName:    acc.Name,
					UID:            acc.UID,
					InitialBalance: acc.InitialBalance,
					Ccy:            usdtBal.Ccy,
					Bal:            usdtBal.Bal,
					FrozenBal:      usdtBal.FrozenBal,
					AvailBal:       usdtBal.AvailBal,
				})
				logrus.Infof("[余额查询] 账户 %s 余额: 总=%s 可用=%s 冻结=%s",
					acc.Name, usdtBal.Bal, usdtBal.AvailBal, usdtBal.FrozenBal)
			} else {
				logrus.Warnf("[余额查询] 账户 %s Data中未找到 ccy=USDT 的条目（Data共%d条）", acc.Name, len(balResp.Data))
				for idx, b := range balResp.Data {
					logrus.Warnf("[余额查询]   Data[%d]: ccy=%s bal=%s availBal=%s", idx, b.Ccy, b.Bal, b.AvailBal)
				}
				accountBalances = append(accountBalances, AccountBalanceInfo{
					AccountName:    acc.Name,
					UID:            acc.UID,
					InitialBalance: acc.InitialBalance,
					Error:          "未找到USDT余额",
				})
			}
		}

		// 如果不是最后一个账户，sleep 1.2秒
		if i < len(config.Accounts)-1 {
			time.Sleep(1200 * time.Millisecond)
		}
	}

	// 更新余额状态并检查是否有余额不足的账户
	am.updateBalanceStatus(accountBalances)

	// 如果需要发送报告，检查是否有账户余额低于报告阈值
	if sendReport {
		am.mu.RLock()
		hasLowBal := am.hasLowBalance
		am.mu.RUnlock()

		if hasLowBal {
			am.sendReportWithBalances(accountBalances)
		} else {
			logrus.Debugf("所有账户余额正常（>=100U），跳过发送报告")
		}
	}
}

// updateBalanceStatus 更新余额状态并检查是否有余额不足的账户
func (am *AccountMonitor) updateBalanceStatus(balances []AccountBalanceInfo) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.lastBalances = balances
	hasInsufficient := false
	hasLow := false

	// 检查每个账户的可用余额
	for _, bal := range balances {
		if bal.Error != "" {
			continue // 跳过有错误的账户
		}

		if bal.AvailBal != "" {
			if availDec, err := decimal.NewFromString(bal.AvailBal); err == nil {
				// 检查是否低于交易阈值（30U）
				if availDec.LessThan(am.minBalance) {
					hasInsufficient = true
					logrus.Warnf("账户 %s 可用余额不足（交易阈值）: %s < %s", bal.AccountName, availDec.String(), am.minBalance.String())
				}
				// 检查是否低于报告阈值（100U）
				if availDec.LessThan(am.reportThreshold) {
					hasLow = true
					logrus.Warnf("账户 %s 可用余额低于报告阈值: %s < %s", bal.AccountName, availDec.String(), am.reportThreshold.String())
				}
			}
		}
	}

	am.hasInsufficientBal = hasInsufficient
	am.hasLowBalance = hasLow
}

// sendReport 发送报告（使用最新的余额信息）
// 只有当有账户余额低于报告阈值（100U）时才发送
func (am *AccountMonitor) sendReport() {
	am.mu.RLock()
	balances := make([]AccountBalanceInfo, len(am.lastBalances))
	copy(balances, am.lastBalances)
	hasLowBal := am.hasLowBalance
	am.mu.RUnlock()

	if len(balances) == 0 {
		return
	}

	// 只有当有账户余额低于报告阈值时才发送
	if hasLowBal {
		am.sendReportWithBalances(balances)
	} else {
		logrus.Debugf("所有账户余额正常（>=100U），跳过发送报告")
	}
}

// sendReportWithBalances 使用指定的余额信息发送报告
func (am *AccountMonitor) sendReportWithBalances(balances []AccountBalanceInfo) {
	// 生成消息并发送到Telegram
	message := am.formatBalanceMessage(balances)
	success, err := am.telegramClient.SendMessage(message)
	if err != nil {
		logrus.Errorf("发送Telegram消息失败: %v", err)
	} else if success {
		logrus.Info("账户余额信息已发送到Telegram")
	}
}

// HasSufficientBalance 检查是否有足够的余额（所有账户的可用余额都 >= 30U）
func (am *AccountMonitor) HasSufficientBalance() bool {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return !am.hasInsufficientBal
}

// GetInsufficientAccounts 获取余额不足的账户列表
func (am *AccountMonitor) GetInsufficientAccounts() []string {
	am.mu.RLock()
	defer am.mu.RUnlock()

	insufficientAccounts := make([]string, 0)
	for _, bal := range am.lastBalances {
		if bal.Error != "" {
			continue
		}

		if bal.AvailBal != "" {
			if availDec, err := decimal.NewFromString(bal.AvailBal); err == nil {
				if availDec.LessThan(am.minBalance) {
					insufficientAccounts = append(insufficientAccounts, bal.AccountName)
				}
			}
		}
	}

	return insufficientAccounts
}

// AccountBalanceInfo 账户余额信息
type AccountBalanceInfo struct {
	AccountName    string
	UID            string  // 用户ID
	InitialBalance float64 // 初始余额
	Ccy            string
	Bal            string // 总余额
	FrozenBal      string // 冻结余额
	AvailBal       string // 可用余额
	Error          string // 错误信息（如果有）
}

// formatBalanceMessage 格式化余额消息
func (am *AccountMonitor) formatBalanceMessage(balances []AccountBalanceInfo) string {
	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("💰 账户余额监控报告\n⏰ 时间: %s\n\n", now)

	if len(balances) == 0 {
		message += "⚠️ 未查询到任何账户余额信息"
		return message
	}

	// 计算总余额汇总和总盈亏
	var totalBal, totalAvailBal, totalFrozenBal, totalInitialBal, totalProfitLoss decimal.Decimal
	successCount := 0

	for i, bal := range balances {
		if bal.Error != "" {
			// 显示账户名称和UID
			accountDisplay := bal.AccountName
			if bal.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", bal.AccountName, bal.UID)
			}
			message += fmt.Sprintf("❌ 账户 %d: %s\n   错误: %s\n\n", i+1, accountDisplay, bal.Error)
		} else {
			// 显示账户名称和UID
			accountDisplay := bal.AccountName
			if bal.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", bal.AccountName, bal.UID)
			}
			message += fmt.Sprintf("✅ 账户 %d: %s\n", i+1, accountDisplay)
			message += fmt.Sprintf("   币种: %s\n", bal.Ccy)

			// 计算余额变动
			var currentBalDec, initialBalDec decimal.Decimal
			var balanceChange decimal.Decimal
			var changeEmoji string

			if bal.Bal != "" {
				if dec, err := decimal.NewFromString(bal.Bal); err == nil {
					currentBalDec = dec
					totalBal = totalBal.Add(dec)
				}
			}

			if bal.InitialBalance > 0 {
				initialBalDec = decimal.NewFromFloat(bal.InitialBalance)
				totalInitialBal = totalInitialBal.Add(initialBalDec)
				balanceChange = currentBalDec.Sub(initialBalDec)

				if balanceChange.IsPositive() {
					changeEmoji = "📈" // 绿色上升箭头
				} else if balanceChange.IsNegative() {
					changeEmoji = "📉" // 红色下降箭头
				} else {
					changeEmoji = "➡️" // 持平
				}
			}

			message += fmt.Sprintf("   初始余额: %s\n", initialBalDec.StringFixed(2))
			message += fmt.Sprintf("   总余额: %s\n", bal.Bal)
			if bal.InitialBalance > 0 {
				changeText := balanceChange.StringFixed(2)
				if balanceChange.IsPositive() {
					changeText = fmt.Sprintf("+%s", changeText)
				}
				message += fmt.Sprintf("   余额变动: %s %s\n", changeEmoji, changeText)
			}
			message += fmt.Sprintf("   可用余额: %s\n", bal.AvailBal)
			message += fmt.Sprintf("   冻结余额: %s\n\n", bal.FrozenBal)

			// 累加余额（只累加成功的账户）
			if bal.AvailBal != "" {
				if availDec, err := decimal.NewFromString(bal.AvailBal); err == nil {
					totalAvailBal = totalAvailBal.Add(availDec)
				}
			}
			if bal.FrozenBal != "" {
				if frozenDec, err := decimal.NewFromString(bal.FrozenBal); err == nil {
					totalFrozenBal = totalFrozenBal.Add(frozenDec)
				}
			}

			// 累加盈亏
			if bal.InitialBalance > 0 && bal.Bal != "" {
				if currentDec, err := decimal.NewFromString(bal.Bal); err == nil {
					profitLoss := currentDec.Sub(initialBalDec)
					totalProfitLoss = totalProfitLoss.Add(profitLoss)
				}
			}

			successCount++
		}
	}

	// 添加汇总信息
	if successCount > 0 {
		message += "━━━━━━━━━━━━━━━━━━━━\n"
		message += fmt.Sprintf("📊 汇总（%d个账户）:\n", successCount)
		message += fmt.Sprintf("   总余额: %s\n", totalBal.StringFixed(2))
		message += fmt.Sprintf("   可用余额: %s\n", totalAvailBal.StringFixed(2))
		message += fmt.Sprintf("   冻结余额: %s\n", totalFrozenBal.StringFixed(2))

		// 添加总盈亏信息
		if totalInitialBal.GreaterThan(decimal.Zero) {
			message += "\n━━━━━━━━━━━━━━━━━━━━\n"
			message += "💹 账户总盈亏:\n"
			message += fmt.Sprintf("   初始总余额: %s\n", totalInitialBal.StringFixed(2))
			message += fmt.Sprintf("   当前总余额: %s\n", totalBal.StringFixed(2))

			var totalProfitLossEmoji string
			if totalProfitLoss.IsPositive() {
				totalProfitLossEmoji = "📈"
			} else if totalProfitLoss.IsNegative() {
				totalProfitLossEmoji = "📉"
			} else {
				totalProfitLossEmoji = "➡️"
			}

			profitLossText := totalProfitLoss.StringFixed(2)
			if totalProfitLoss.IsPositive() {
				profitLossText = fmt.Sprintf("+%s", profitLossText)
			}
			message += fmt.Sprintf("   总盈亏: %s %s\n", totalProfitLossEmoji, profitLossText)

			// 计算盈亏百分比
			if totalInitialBal.GreaterThan(decimal.Zero) {
				profitLossPercent := totalProfitLoss.Div(totalInitialBal).Mul(decimal.NewFromInt(100))
				percentText := profitLossPercent.StringFixed(2)
				if profitLossPercent.IsPositive() {
					percentText = fmt.Sprintf("+%s%%", percentText)
				} else {
					percentText = fmt.Sprintf("%s%%", percentText)
				}
				message += fmt.Sprintf("   盈亏比例: %s\n", percentText)
			}
		}
	}

	return message
}

// GetBalanceReport 实时查询余额并返回报告（供 Telegram Bot 调用）
func (am *AccountMonitor) GetBalanceReport() string {
	// 实时触发一次查询，更新内部缓存
	am.queryBalances(false)

	am.mu.RLock()
	balances := make([]AccountBalanceInfo, len(am.lastBalances))
	copy(balances, am.lastBalances)
	am.mu.RUnlock()

	if len(balances) == 0 {
		return "⚠️ 暂无账户余额信息，请稍后再试"
	}

	return am.formatBalanceMessage(balances)
}

// GetPositionReport 实时查询所有账户持仓并返回报告（供 Telegram Bot 调用）
func (am *AccountMonitor) GetPositionReport() string {
	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法查询持仓"
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return "⚠️ 没有配置账户"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("📊 持仓查询报告\n⏰ 时间: %s\n\n", now)
	totalAccounts := len(config.Accounts)
	hasAnyPosition := false

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			message += fmt.Sprintf("❌ 账户%d %s: 客户端不存在\n\n", i+1, acc.Name)
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})

		accountDisplay := acc.Name
		if acc.UID != "" {
			accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
		}

		if err != nil {
			logrus.Errorf("[持仓查询] 账户 %s 失败: %v", acc.Name, err)
			message += fmt.Sprintf("❌ 账户%d %s\n   错误: %v\n\n", i+1, accountDisplay, err)
			continue
		}

		logrus.Infof("[持仓查询] 账户 %s 持仓条数: %d", acc.Name, len(posResp.Data))

		if len(posResp.Data) == 0 {
			message += fmt.Sprintf("✅ 账户%d %s\n   暂无持仓\n\n", i+1, accountDisplay)
			continue
		}

		hasAnyPosition = true
		message += fmt.Sprintf("✅ 账户%d %s (%d个持仓)\n", i+1, accountDisplay, len(posResp.Data))

		for j, pos := range posResp.Data {
			directionEmoji := "🔵"
			if pos.PosSide == "short" {
				directionEmoji = "🔴"
			}

			pnlDisplay := pos.UnrealizedProfit
			pnlEmoji := "➡️"
			pnlPercentDisplay := ""
			if pos.UnrealizedProfit != "" {
				if pnl, err := decimal.NewFromString(pos.UnrealizedProfit); err == nil {
					if pos.PosSide == "short" {
						pnl = pnl.Neg()
						pnlDisplay = pnl.String()
					}
					if pnl.IsPositive() {
						pnlEmoji = "📈"
					} else if pnl.IsNegative() {
						pnlEmoji = "📉"
					}
					if pos.UseMargin != "" {
						if margin, merr := decimal.NewFromString(pos.UseMargin); merr == nil && margin.IsPositive() {
							pct := pnl.Div(margin).Mul(decimal.NewFromInt(100))
							pctStr := pct.StringFixed(2)
							if pct.IsPositive() {
								pctStr = "+" + pctStr
							}
							pnlPercentDisplay = fmt.Sprintf("(%s%%)", pctStr)
						}
					}
				}
			}

			message += fmt.Sprintf(
				"  [%d] %s %s  方向:%s\n"+
					"      持仓:%s张  开仓均价:%s\n"+
					"      最新价:%s  强平价:%s\n"+
					"      占用保证金:%s  未实现盈亏:%s %s %s\n\n",
				j+1, directionEmoji, pos.InstId, pos.PosSide,
				pos.Pos, pos.AvgPx,
				pos.LastPx, pos.LiqPx,
				pos.UseMargin, pnlEmoji, pnlDisplay, pnlPercentDisplay,
			)
		}

		// 账户间加间隔
		if i < totalAccounts-1 {
			time.Sleep(1200 * time.Millisecond)
		}
	}

	if !hasAnyPosition {
		message += "📭 所有账户当前无持仓"
	}

	return message
}

// startPositionMonitor 启动持仓盈亏监控（5秒一次）
func (am *AccountMonitor) startPositionMonitor() {
	ticker := time.NewTicker(am.posQueryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-ticker.C:
			am.checkPositionPnl()
		}
	}
}

// startAICloseMonitor 启动 AI 平仓巡检：启动即执行一次，之后按 AI 建议的间隔动态调度。
func (am *AccountMonitor) startAICloseMonitor() {
	if am.aiCloseDecider == nil {
		return
	}

	// 启动后稍等片刻（让首次持仓/余额查询就绪）再立即执行一次。
	select {
	case <-am.stopChan:
		return
	case <-time.After(8 * time.Second):
	}

	next := am.runAICloseCheck()
	timer := time.NewTimer(next)
	defer timer.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-timer.C:
			next := am.runAICloseCheck()
			timer.Reset(next)
		}
	}
}

// startAIOpenMonitor 启动 AI 加仓巡检：启动即执行一次，之后按 AI 建议的间隔动态调度。
func (am *AccountMonitor) startAIOpenMonitor() {
	if am.aiOpenDecider == nil {
		return
	}

	// 启动后立即执行一次（稍等片刻让首次余额查询完成，新仓评估才有余额数据）。
	select {
	case <-am.stopChan:
		return
	case <-time.After(8 * time.Second):
	}

	next := am.runAIOpenCheck()
	timer := time.NewTimer(next)
	defer timer.Stop()

	for {
		select {
		case <-am.stopChan:
			return
		case <-timer.C:
			next := am.runAIOpenCheck()
			timer.Reset(next)
		}
	}
}

// runAIOpenCheck 执行一次巡检并返回下次执行的间隔（取各账户 AI 建议中最短的一个，无则用默认间隔）。
func (am *AccountMonitor) runAIOpenCheck() time.Duration {
	results, nextInterval := am.executeAIOpenStrategy("scheduled")

	if nextInterval <= 0 {
		nextInterval = am.aiOpenCheckInterval
	}
	if nextInterval <= 0 {
		nextInterval = 15 * time.Minute
	}

	if len(results) == 0 {
		logrus.Infof("[AI加仓] 策略执行完成：当前没有可评估的账户，下次 %v 后再巡检", nextInterval)
		return nextInterval
	}

	msg := fmt.Sprintf(
		"🤖 AI加仓策略定时执行（仅告警，未自动下单）\n⏰ %s\n下次巡检: %v 后\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		nextInterval,
		strings.Join(results, "\n\n---\n\n"),
	)
	if success, err := am.telegramClient.SendMessage(msg); err != nil {
		logrus.Errorf("[AI加仓] 发送策略执行结果失败: %v", err)
	} else if success {
		logrus.Infof("[AI加仓] 策略执行结果已发送，下次 %v 后再巡检", nextInterval)
	}

	for _, result := range results {
		logrus.Infof("[AI加仓] 策略执行结果: %s", strings.TrimSpace(result))
	}
	return nextInterval
}

// RunAIOpenStrategyNow 手动触发一次 AI 加仓巡检（供 Telegram Bot 调用）。
func (am *AccountMonitor) RunAIOpenStrategyNow() string {
	if am.aiOpenDecider == nil {
		return "⚠️ AI加仓: 已禁用"
	}
	results, nextInterval := am.executeAIOpenStrategy("manual")
	if len(results) == 0 {
		return "🤖 AI加仓策略已执行：当前没有可评估的账户"
	}
	if nextInterval <= 0 {
		nextInterval = am.aiOpenCheckInterval
	}
	return fmt.Sprintf(
		"🤖 AI加仓策略已执行（仅告警，未自动下单）\n⏰ %s\n下次巡检: %v 后\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		nextInterval,
		strings.Join(results, "\n\n---\n\n"),
	)
}

// executeAIOpenStrategy 遍历所有账户评估开仓/加仓（空仓评估开新仓，有仓评估加仓/减仓）。
// 返回告警文本列表，以及下次巡检间隔（取各账户 AI 建议 next_check_in 的最短者，无则 0）。
func (am *AccountMonitor) executeAIOpenStrategy(triggerType string) ([]string, time.Duration) {
	results := make([]string, 0)
	var nextInterval time.Duration // 0 表示无 AI 建议，由调用方回退默认
	if am.aiOpenDecider == nil || !trade.IsInitialized() {
		return results, nextInterval
	}

	tm := trade.GetManager()
	if tm == nil {
		return results, nextInterval
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return results, nextInterval
	}

	logrus.Infof("[AI加仓] 开始执行策略，trigger=%s, interval=%v", triggerType, am.aiOpenCheckInterval)

	var btcMarket *BTCAnalysisSnapshot
	if snapshot, err := GetBTCAnalysisSnapshot(); err != nil {
		logrus.Warnf("[AI加仓] 获取BTC市场快照失败: %v", err)
	} else {
		btcMarket = snapshot
	}

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			logrus.Warnf("[AI加仓] 账户 %s 客户端不存在，跳过", acc.Name)
			continue
		}

		var posResp *utils.GetPositionsResponse
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			posResp, err = client.GetPositionsTyped(&utils.GetPositionsRequest{InstType: "SWAP"})
			if err == nil {
				break
			}
			logrus.Warnf("[AI加仓] 账户 %s 查询持仓失败(第%d次): %v", acc.Name, attempt, err)
			if attempt < 3 {
				time.Sleep(2 * time.Second)
			}
		}
		if err != nil {
			logrus.Warnf("[AI加仓] 账户 %s 查询持仓失败，已重试3次，跳过: %v", acc.Name, err)
			continue
		}

		var availBal, totalBal string
		am.mu.RLock()
		for _, b := range am.lastBalances {
			if b.AccountName == acc.Name && b.Error == "" {
				availBal = b.AvailBal
				totalBal = b.Bal
				break
			}
		}
		am.mu.RUnlock()

		// 空仓账户：评估是否值得开一个新仓（全仓模式下仓位相对余额越小越安全）。
		if len(posResp.Data) == 0 {
			accountDisplay := acc.Name
			if acc.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
			}
			openResultMsg, next := am.evaluateAIOpenNoPosition(acc, btcMarket, availBal, totalBal)
			nextInterval = minPositiveDuration(nextInterval, next)
			results = append(results, fmt.Sprintf(
				"账户: %s\n当前状态: 空仓\n%s%s",
				accountDisplay, formatNoPositionCurrentPrice(btcMarket), openResultMsg))
			continue
		}

		for _, pos := range posResp.Data {
			// 当前 AI 加仓只用 BTC 行情快照，非 BTC 持仓跳过，避免用 BTC 指标误判其它币种。
			if !strings.Contains(strings.ToUpper(pos.InstId), "BTC") {
				logrus.Infof("[AI加仓] 账户 %s 持仓 %s 非BTC，AI加仓暂不支持，跳过", acc.Name, pos.InstId)
				continue
			}

			pct, pnl, ok, reason := calculatePositionPnLWithReason(pos)
			if !ok {
				logrus.Warnf("[AI加仓] 账户 %s 仓位不可评估，跳过: reason=%s instId=%s posSide=%s", acc.Name, reason, pos.InstId, pos.PosSide)
				continue
			}

			openResultMsg, next := am.evaluateAIOpen(acc, pos, pct, triggerType, btcMarket, availBal, totalBal)
			nextInterval = minPositiveDuration(nextInterval, next)

			accountDisplay := acc.Name
			if acc.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
			}
			directionEmoji := "🔵"
			if pos.PosSide == "short" {
				directionEmoji = "🔴"
			}
			pctStr := pct.StringFixed(2)
			if pct.IsPositive() {
				pctStr = "+" + pctStr
			}

			result := fmt.Sprintf(
				"账户: %s\n"+
					"%s %s  方向:%s\n"+
					"持仓:%s张  开仓均价:%s\n"+
					"最新价:%s  强平价:%s\n"+
					"占用保证金:%s  可用余额:%s\n"+
					"未实现盈亏: %s (%s%%)%s",
				accountDisplay,
				directionEmoji, pos.InstId, pos.PosSide,
				pos.Pos, pos.AvgPx,
				pos.LastPx, pos.LiqPx,
				pos.UseMargin, defaultString(availBal, "未知"),
				pnl.String(), pctStr,
				openResultMsg,
			)
			results = append(results, result)
		}

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return results, nextInterval
}

// minPositiveDuration 返回两个时长中较小的正值；忽略 <=0 的一方。
func minPositiveDuration(a, b time.Duration) time.Duration {
	if b <= 0 {
		return a
	}
	if a <= 0 || b < a {
		return b
	}
	return a
}

// evaluateAIOpen 对单个持仓调用 AI 加仓决策，第一期仅返回告警文本，不自动下单。
// 返回 (告警文本, AI 建议的下次巡检间隔)。
func (am *AccountMonitor) evaluateAIOpen(acc trade.AccountConfig, pos utils.PositionInfo, pct decimal.Decimal, triggerType string, btcMarket *BTCAnalysisSnapshot, availBal, totalBal string) (string, time.Duration) {
	if am.aiOpenDecider == nil {
		return "\n\n⚠️ AI加仓: 已禁用", 0
	}

	posDetails := CurrentPositionDetails{
		InstType:         pos.InstType,
		InstID:           pos.InstId,
		PositionID:       pos.PosId,
		PositionSide:     pos.PosSide,
		PositionSize:     pos.Pos,
		AvgPrice:         pos.AvgPx,
		LastPrice:        pos.LastPx,
		LiqPrice:         pos.LiqPx,
		UseMargin:        pos.UseMargin,
		UnrealizedProfit: pos.UnrealizedProfit,
		PnLPercent:       pct.StringFixed(2),
		Leverage:         pos.Lever,
		MarginMode:       pos.MgnMode,
		MarginPosition:   pos.MrgPosition,
		Currency:         pos.Ccy,
		CreateTime:       pos.CTime,
		UpdateTime:       pos.UTime,
	}

	decision, err := am.aiOpenDecider.Decide(PositionSnapshot{
		AccountName:        acc.Name,
		AccountUID:         acc.UID,
		HasPosition:        true,
		CurrentPosition:    posDetails,
		BTCMarket:          btcMarket,
		PositionSummary:    buildPositionSummary(posDetails),
		InstID:             pos.InstId,
		PositionID:         pos.PosId,
		PositionSide:       pos.PosSide,
		PositionSize:       pos.Pos,
		AvgPrice:           pos.AvgPx,
		LastPrice:          pos.LastPx,
		LiqPrice:           pos.LiqPx,
		UseMargin:          pos.UseMargin,
		UnrealizedProfit:   pos.UnrealizedProfit,
		PnLPercent:         pct,
		TriggerType:        triggerType,
		AvailBal:           availBal,
		TotalBal:           totalBal,
		LiqDistancePercent: computeLiqDistancePercent(pos.LastPx, pos.LiqPx, pos.PosSide),
	})
	if err != nil {
		logrus.Errorf("[AI加仓] 决策失败: account=%s inst=%s, err=%v", acc.Name, pos.InstId, err)
		return fmt.Sprintf("\n\n⚠️ AI加仓: 决策失败\n原因: %v", err), 0
	}
	if decision == nil {
		return "\n\n⚠️ AI加仓: 未返回结果", 0
	}

	next, _ := parseNextCheckInterval(decision.NextCheckIn, am.aiOpenMinInterval, am.aiOpenMaxInterval)

	wantsOpen := decision.FinalAction == "open_long" || decision.FinalAction == "open_short"
	if wantsOpen && decision.RiskPassed {
		logrus.Infof("[AI加仓] 建议操作: account=%s inst=%s mode=%s action=%s size=%s", acc.Name, pos.InstId, decision.Mode, decision.FinalAction, decision.SuggestedSize)
		execMsg := am.maybeAutoOpen(acc, decision, pos.LastPx)
		return fmt.Sprintf("\n\n🟢 AI决策: 建议开仓/加仓\n%s%s", formatAIOpenDecision(decision), execMsg), next
	}
	if wantsOpen && !decision.RiskPassed {
		logrus.Warnf("[AI加仓] 建议加仓但本地风控拦截: account=%s inst=%s reason=%s", acc.Name, pos.InstId, decision.RiskBlockReason)
		return fmt.Sprintf("\n\n⛔ AI决策: 想加仓但本地风控拦截（爆仓距离不足）\n%s", formatAIOpenDecision(decision)), next
	}
	logrus.Infof("[AI加仓] 暂不加仓: account=%s inst=%s action=%s", acc.Name, pos.InstId, decision.FinalAction)
	return fmt.Sprintf("\n\n🤖 AI决策: 暂不加仓\n%s", formatAIOpenDecision(decision)), next
}

// maybeAutoOpen 在自动下单开启且 AI 决策放行时执行市价开/加仓，并即时推送 Telegram。
// 返回追加到告警文本里的执行结果；未开启自动下单时返回"仅告警"提示。
func (am *AccountMonitor) maybeAutoOpen(acc trade.AccountConfig, decision *AIOpenDecision, tradePriceStr string) string {
	if !am.aiOpenAutoTrade {
		return "\n（自动下单已关闭，仅告警）"
	}
	if !trade.IsInitialized() {
		return "\n⚠️ 自动下单跳过: 交易管理器未初始化"
	}
	tm := trade.GetManager()
	if tm == nil {
		return "\n⚠️ 自动下单跳过: 交易管理器未就绪"
	}

	side := "long"
	if decision.FinalAction == "open_short" {
		side = "short"
	}
	size := int(parseContracts(decision.SuggestedSize).IntPart())
	if size <= 0 {
		return "\n⚠️ 自动下单跳过: 建议张数无效"
	}
	price := 0.0
	if p, err := decimal.NewFromString(strings.TrimSpace(tradePriceStr)); err == nil {
		price = p.InexactFloat64()
	}

	accountDisplay := acc.Name
	if acc.UID != "" {
		accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
	}

	resp, err := tm.OpenPositionByAI(acc.Name, aiOpenTradeInst, side, size, price)
	if err != nil {
		logrus.Errorf("[AI加仓] 自动下单失败: account=%s side=%s size=%d err=%v", acc.Name, side, size, err)
		tgMsg := fmt.Sprintf("🔴 AI自动下单失败\n账户: %s\n模式: %s  方向: %s  张数: %d\n合约: %s\n错误: %v\n时间: %s",
			accountDisplay, decision.Mode, side, size, aiOpenTradeInst, err, time.Now().Format("2006-01-02 15:04:05"))
		if _, serr := am.telegramClient.SendMessage(tgMsg); serr != nil {
			logrus.Errorf("[AI加仓] 自动下单失败告警发送失败: %v", serr)
		}
		return fmt.Sprintf("\n🔴 自动下单失败: %v", err)
	}

	logrus.Infof("[AI加仓] 自动下单成功: account=%s side=%s size=%d code=%d", acc.Name, side, size, resp.Code)
	tgMsg := fmt.Sprintf("🟢 AI自动下单成功\n账户: %s\n模式: %s  方向: %s  张数: %d\n合约: %s  参考价: %.2f\n止损: %s  止盈: %s\n时间: %s",
		accountDisplay, decision.Mode, side, size, aiOpenTradeInst, price,
		defaultString(decision.StopLossPrice, "未给"), defaultString(decision.TakeProfitPrice, "未给"),
		time.Now().Format("2006-01-02 15:04:05"))
	if _, serr := am.telegramClient.SendMessage(tgMsg); serr != nil {
		logrus.Errorf("[AI加仓] 自动下单成功告警发送失败: %v", serr)
	}
	return fmt.Sprintf("\n🟢 已自动下单: %s %d张 (合约%s)", side, size, aiOpenTradeInst)
}

// evaluateAIOpenNoPosition 对空仓账户评估是否值得开一个新仓（全仓模式，仅 BTC），第一期仅告警。
// 返回 (告警文本, AI 建议的下次巡检间隔)。
func (am *AccountMonitor) evaluateAIOpenNoPosition(acc trade.AccountConfig, btcMarket *BTCAnalysisSnapshot, availBal, totalBal string) (string, time.Duration) {
	if am.aiOpenDecider == nil {
		return "\n\n⚠️ AI开仓: 已禁用", 0
	}

	decision, err := am.aiOpenDecider.Decide(PositionSnapshot{
		AccountName:     acc.Name,
		AccountUID:      acc.UID,
		HasPosition:     false,
		BTCMarket:       btcMarket,
		PositionSummary: "no_open_position=true",
		InstID:          "BTC-USDT-SWAP",
		LastPrice:       currentBTCPrice(btcMarket),
		TriggerType:     "no_position",
		AvailBal:        availBal,
		TotalBal:        totalBal,
	})
	if err != nil {
		logrus.Errorf("[AI开仓] 空仓决策失败: account=%s, err=%v", acc.Name, err)
		return fmt.Sprintf("\n\n⚠️ AI开仓: 决策失败\n原因: %v", err), 0
	}
	if decision == nil {
		return "\n\n⚠️ AI开仓: 未返回结果", 0
	}

	next, _ := parseNextCheckInterval(decision.NextCheckIn, am.aiOpenMinInterval, am.aiOpenMaxInterval)

	wantsOpen := decision.FinalAction == "open_long" || decision.FinalAction == "open_short"
	if wantsOpen && decision.RiskPassed {
		logrus.Infof("[AI开仓] 建议开新仓: account=%s action=%s size=%s", acc.Name, decision.FinalAction, decision.SuggestedSize)
		execMsg := am.maybeAutoOpen(acc, decision, currentBTCPrice(btcMarket))
		return fmt.Sprintf("\n\n🟢 AI决策: 建议开新仓\n%s%s", formatAIOpenDecision(decision), execMsg), next
	}
	if wantsOpen && !decision.RiskPassed {
		logrus.Warnf("[AI开仓] 建议开新仓但本地风控拦截: account=%s reason=%s", acc.Name, decision.RiskBlockReason)
		return fmt.Sprintf("\n\n⛔ AI决策: 想开新仓但本地风控拦截\n%s", formatAIOpenDecision(decision)), next
	}
	logrus.Infof("[AI开仓] 暂不开仓: account=%s action=%s", acc.Name, decision.FinalAction)
	return fmt.Sprintf("\n\n🤖 AI决策: 暂不开仓\n%s", formatAIOpenDecision(decision)), next
}

// checkPositionPnl 检查所有账户持仓的盈亏比例，超过阈值则发送告警
func (am *AccountMonitor) checkPositionPnl() {
	if !trade.IsInitialized() {
		return
	}

	tm := trade.GetManager()
	if tm == nil {
		return
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return
	}

	profitThreshold := am.pnlProfitThreshold
	lossThreshold := am.pnlLossThreshold.Neg()

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})
		if err != nil {
			logrus.Debugf("[持仓监控] 账户 %s 查询失败: %v", acc.Name, err)
			continue
		}

		for _, pos := range posResp.Data {
			if pos.UnrealizedProfit == "" || pos.UseMargin == "" {
				continue
			}

			pnl, err := decimal.NewFromString(pos.UnrealizedProfit)
			if err != nil {
				continue
			}
			margin, err := decimal.NewFromString(pos.UseMargin)
			if err != nil || !margin.IsPositive() {
				continue
			}

			pct := pnl.Div(margin).Mul(decimal.NewFromInt(100))

			isProfit := pct.GreaterThan(profitThreshold)
			isLoss := pct.LessThan(lossThreshold)
			if !isProfit && !isLoss {
				continue
			}

			alertKey := fmt.Sprintf("%s:%s:%s", acc.Name, pos.InstId, pos.PosSide)

			am.pnlAlertMu.RLock()
			lastAlert, exists := am.lastPnlAlerts[alertKey]
			am.pnlAlertMu.RUnlock()

			if exists && time.Since(lastAlert) < am.pnlAlertCooldown {
				continue
			}

			am.pnlAlertMu.Lock()
			am.lastPnlAlerts[alertKey] = time.Now()
			am.pnlAlertMu.Unlock()

			accountDisplay := acc.Name
			if acc.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
			}

			directionEmoji := "🔵"
			if pos.PosSide == "short" {
				directionEmoji = "🔴"
			}

			pctStr := pct.StringFixed(2)
			if pct.IsPositive() {
				pctStr = "+" + pctStr
			}

			now := time.Now().Format("2006-01-02 15:04:05")
			logrus.Warnf("[持仓监控] %s 盈亏比例 %s%% 超过阈值(盈利>%s%%/亏损<-%s%%)", alertKey, pctStr, am.pnlProfitThreshold.String(), am.pnlLossThreshold.String())

			triggerType := "loss"
			if isProfit {
				triggerType = "profit"
			}

			thresholdActionMsg := am.formatThresholdAction(tm, acc, pos, alertKey, pct, triggerType)

			alertMsg := fmt.Sprintf(
				"🚨 持仓盈亏告警\n"+
					"⏰ %s\n\n"+
					"账户: %s\n"+
					"%s %s  方向:%s\n"+
					"持仓:%s张  开仓均价:%s\n"+
					"最新价:%s  强平价:%s\n"+
					"占用保证金:%s\n"+
					"未实现盈亏: %s (%s%%)%s\n",
				now,
				accountDisplay,
				directionEmoji, pos.InstId, pos.PosSide,
				pos.Pos, pos.AvgPx,
				pos.LastPx, pos.LiqPx,
				pos.UseMargin,
				pnl.String(), pctStr,
				thresholdActionMsg,
			)

			if success, err := am.telegramClient.SendMessage(alertMsg); err != nil {
				logrus.Errorf("[持仓监控] 发送告警消息失败: %v", err)
			} else if success {
				logrus.Infof("[持仓监控] 告警消息已发送: %s", alertKey)
			}
		}

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// runAICloseCheck 执行一次平仓巡检并返回下次执行间隔（取各持仓 AI 建议中最短者，无则回退默认间隔）。
func (am *AccountMonitor) runAICloseCheck() time.Duration {
	results, push, nextInterval := am.executeAICloseStrategy("scheduled")

	if nextInterval <= 0 {
		nextInterval = am.aiCheckInterval
	}
	if nextInterval <= 0 {
		nextInterval = 60 * time.Minute
	}

	for _, result := range results {
		logrus.Infof("[AI平仓] 策略执行结果: %s", strings.TrimSpace(result))
	}

	if len(results) == 0 {
		logrus.Infof("[AI平仓] 策略执行完成：当前没有可评估的持仓，下次 %v 后再巡检", nextInterval)
		return nextInterval
	}

	// 降噪：仅当有"建议平仓/高风险"时才推 Telegram，其余只记日志。
	if !push {
		logrus.Infof("[AI平仓] 本轮无建议平仓/高风险项，跳过 Telegram 推送，下次 %v 后再巡检", nextInterval)
		return nextInterval
	}

	msg := fmt.Sprintf(
		"🤖 AI平仓策略定时执行\n⏰ %s\n下次巡检: %v 后\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		nextInterval,
		strings.Join(results, "\n\n---\n\n"),
	)
	if success, err := am.telegramClient.SendMessage(msg); err != nil {
		logrus.Errorf("[AI平仓] 发送策略执行结果失败: %v", err)
	} else if success {
		logrus.Infof("[AI平仓] 策略执行结果已发送，下次 %v 后再巡检", nextInterval)
	}
	return nextInterval
}

func (am *AccountMonitor) RunAICloseStrategyNow() string {
	results, _, _ := am.executeAICloseStrategy("manual")
	if len(results) == 0 {
		return "🤖 AI平仓策略已执行：当前没有可评估的持仓"
	}

	return fmt.Sprintf(
		"🤖 AI平仓策略已执行\n⏰ %s\n\n%s",
		time.Now().Format("2006-01-02 15:04:05"),
		strings.Join(results, "\n\n---\n\n"),
	)
}

func (am *AccountMonitor) RunAICloseStrategyWithManualPosition(input manualAICloseInput) string {
	if am.aiCloseDecider == nil {
		return "⚠️ AI平仓: 已禁用"
	}
	if !input.AvgPrice.IsPositive() {
		return "⚠️ 手工均价无效，请发送例如: AI + 73210 + L"
	}

	var btcMarket *BTCAnalysisSnapshot
	if snapshot, err := GetBTCAnalysisSnapshot(); err != nil {
		logrus.Warnf("[AI手工均价] 获取BTC市场快照失败: %v", err)
	} else {
		btcMarket = snapshot
	}

	acc := trade.AccountConfig{Name: "manual-input", PositionSide: "unknown"}
	if config := trade.GetConfig(); config != nil && len(config.Accounts) > 0 {
		acc = config.Accounts[0]
		if strings.TrimSpace(acc.PositionSide) == "" {
			acc.PositionSide = "unknown"
		}
	}
	if normalizedSide := normalizeManualPositionSide(input.PositionSide); normalizedSide != "" {
		acc.PositionSide = normalizedSide
	}

	currentPrice := currentBTCPrice(btcMarket)
	metrics := calculateManualPositionMetrics(input, currentPrice, acc.PositionSide)
	useMargin := optionalDecimalString(metrics.InitialMargin)
	positionSize := optionalDecimalString(input.PositionSize)
	leverage := optionalDecimalString(input.Leverage)
	liqPrice := manualMetricString(metrics.LiqPrice, metrics.HasLiq)
	unrealizedProfit := manualMetricString(metrics.UnrealizedProfit, metrics.HasCurrent)
	decision, err := am.aiCloseDecider.Decide(PositionSnapshot{
		AccountName:        acc.Name,
		AccountUID:         acc.UID,
		HasPosition:        true,
		BTCMarket:          btcMarket,
		PositionSummary:    buildManualPositionSummary(acc, input, currentPrice, metrics),
		InstID:             "BTC-USDT-SWAP",
		PositionSide:       acc.PositionSide,
		PositionSize:       positionSize,
		AvgPrice:           input.AvgPrice.String(),
		LastPrice:          currentPrice,
		LiqPrice:           liqPrice,
		UseMargin:          useMargin,
		UnrealizedProfit:   unrealizedProfit,
		PnLPercent:         metrics.PnLPercent,
		TriggerType:        "manual_avg_price",
		AvailBal:           optionalDecimalString(input.Balance),
		TotalBal:           optionalDecimalString(input.Balance),
		LiqDistancePercent: computeLiqDistancePercent(currentPrice, liqPrice, acc.PositionSide),
		CurrentPosition: CurrentPositionDetails{
			InstType:         "SWAP",
			InstID:           "BTC-USDT-SWAP",
			PositionSide:     acc.PositionSide,
			PositionSize:     positionSize,
			AvgPrice:         input.AvgPrice.String(),
			LastPrice:        currentPrice,
			LiqPrice:         liqPrice,
			UseMargin:        useMargin,
			UnrealizedProfit: unrealizedProfit,
			PnLPercent:       metrics.PnLPercent.StringFixed(2),
			Leverage:         leverage,
		},
	})
	if err != nil {
		logrus.Errorf("[AI手工均价] AI建议失败: avg=%s, err=%v", input.AvgPrice.String(), err)
		return fmt.Sprintf("⚠️ AI手工均价建议失败\n手工均价: %s\n原因: %v", input.AvgPrice.String(), err)
	}
	if decision == nil {
		return fmt.Sprintf("⚠️ AI手工均价建议失败\n手工均价: %s\n原因: AI未返回结果", input.AvgPrice.String())
	}

	return fmt.Sprintf(
		"🤖 AI平仓策略已执行（手工均价）\n"+
			"⏰ %s\n\n"+
			"手工均价: %s\n"+
			"当前最新价: %s\n"+
			"方向: %s\n"+
			"余额: %s\n"+
			"张数: %s\n"+
			"仓位BTC: %s\n"+
			"计算保证金: %s\n"+
			"杠杆倍数: %s\n"+
			"估算爆仓价: %s\n"+
			"当前盈亏: %s\n"+
			"估算收益率: %s%%\n"+
			"说明: 未查询交易所当前仓位，仅按你发送的均价生成AI建议\n\n"+
			"%s",
		time.Now().Format("2006-01-02 15:04:05"),
		input.AvgPrice.String(),
		defaultString(currentPrice, "未获取到"),
		defaultString(acc.PositionSide, "unknown"),
		defaultString(optionalDecimalString(input.Balance), "未提供"),
		defaultString(positionSize, "未提供"),
		defaultString(optionalDecimalString(metrics.QuantityBTC), "未提供"),
		defaultString(useMargin, "未提供"),
		defaultString(leverage, "未提供"),
		defaultString(liqPrice, "未提供"),
		defaultString(unrealizedProfit, "未提供"),
		metrics.PnLPercent.StringFixed(2),
		formatAICloseDecision(decision),
	)
}

// executeAICloseStrategy 返回 (告警文本列表, 是否需要推送Telegram, 下次巡检间隔)。
// push=true 表示本轮存在「建议平仓或高风险」项，值得主动推送。
func (am *AccountMonitor) executeAICloseStrategy(triggerType string) ([]string, bool, time.Duration) {
	results := make([]string, 0)
	var nextInterval time.Duration
	push := false
	if am.aiCloseDecider == nil || !trade.IsInitialized() {
		return results, push, nextInterval
	}

	tm := trade.GetManager()
	if tm == nil {
		return results, push, nextInterval
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return results, push, nextInterval
	}

	logrus.Infof("[AI平仓] 开始执行策略，trigger=%s, interval=%v", triggerType, am.aiCheckInterval)

	var btcMarket *BTCAnalysisSnapshot
	if snapshot, err := GetBTCAnalysisSnapshot(); err != nil {
		logrus.Warnf("[AI平仓] 获取BTC市场快照失败: %v", err)
	} else {
		btcMarket = snapshot
	}

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			logrus.Warnf("[AI平仓] 账户 %s 客户端不存在，跳过策略执行", acc.Name)
			continue
		}

		var posResp *utils.GetPositionsResponse
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			posResp, err = client.GetPositionsTyped(&utils.GetPositionsRequest{
				InstType: "SWAP",
			})
			if err == nil {
				break
			}
			logrus.Warnf("[AI平仓] 账户 %s 查询持仓失败(第%d次): %v", acc.Name, attempt, err)
			if attempt < 3 {
				time.Sleep(2 * time.Second)
			}
		}
		if err != nil {
			logrus.Warnf("[AI平仓] 账户 %s 查询持仓失败，已重试3次，跳过: %v", acc.Name, err)
			continue
		}

		logrus.Infof("[AI平仓] 账户 %s 持仓条数: %d", acc.Name, len(posResp.Data))
		if len(posResp.Data) == 0 {
			logrus.Infof("[AI平仓] 账户 %s 暂无持仓，执行AI空仓建议", acc.Name)
			accountDisplay := acc.Name
			if acc.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
			}
			advice, next, p := am.evaluateNoPositionAdvice(acc, btcMarket)
			nextInterval = minPositiveDuration(nextInterval, next)
			push = push || p
			results = append(results, fmt.Sprintf(
				"账户: %s\n当前状态: 暂无持仓\n%s%s",
				accountDisplay,
				formatNoPositionCurrentPrice(btcMarket),
				advice,
			))
			continue
		}

		var availBal, totalBal string
		am.mu.RLock()
		for _, b := range am.lastBalances {
			if b.AccountName == acc.Name && b.Error == "" {
				availBal = b.AvailBal
				totalBal = b.Bal
				break
			}
		}
		am.mu.RUnlock()

		for _, pos := range posResp.Data {
			pct, pnl, ok, reason := calculatePositionPnLWithReason(pos)
			if !ok {
				logrus.Warnf(
					"[AI平仓] 账户 %s 仓位不可评估，跳过: reason=%s instId=%s posSide=%s pos=%s avgPx=%s lastPx=%s useMargin=%s unrealizedProfit=%s posId=%s",
					acc.Name,
					reason,
					pos.InstId,
					pos.PosSide,
					pos.Pos,
					pos.AvgPx,
					pos.LastPx,
					pos.UseMargin,
					pos.UnrealizedProfit,
					pos.PosId,
				)
				continue
			}

			alertKey := fmt.Sprintf("%s:%s:%s", acc.Name, pos.InstId, pos.PosSide)
			closeResultMsg, next, p := am.evaluateAndMaybeClosePosition(acc, pos, alertKey, pct, triggerType, btcMarket, availBal, totalBal)
			nextInterval = minPositiveDuration(nextInterval, next)
			push = push || p

			accountDisplay := acc.Name
			if acc.UID != "" {
				accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
			}

			directionEmoji := "🔵"
			if pos.PosSide == "short" {
				directionEmoji = "🔴"
			}

			pctStr := pct.StringFixed(2)
			if pct.IsPositive() {
				pctStr = "+" + pctStr
			}

			result := fmt.Sprintf(
				"账户: %s\n"+
					"%s %s  方向:%s\n"+
					"持仓:%s张  开仓均价:%s\n"+
					"最新价:%s  强平价:%s\n"+
					"占用保证金:%s\n"+
					"未实现盈亏: %s (%s%%)%s",
				accountDisplay,
				directionEmoji, pos.InstId, pos.PosSide,
				pos.Pos, pos.AvgPx,
				pos.LastPx, pos.LiqPx,
				pos.UseMargin,
				pnl.String(), pctStr,
				closeResultMsg,
			)
			results = append(results, result)
		}

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	return results, push, nextInterval
}

func formatNoPositionCurrentPrice(btcMarket *BTCAnalysisSnapshot) string {
	symbol := "BTCUSDT"
	price := currentBTCPrice(btcMarket)
	if btcMarket != nil {
		if strings.TrimSpace(btcMarket.Symbol) != "" {
			symbol = btcMarket.Symbol
		}
	}
	if price == "" {
		return fmt.Sprintf("当前最新价: %s 未获取到", symbol)
	}
	return fmt.Sprintf("当前最新价: %s %s", symbol, price)
}

func currentBTCPrice(btcMarket *BTCAnalysisSnapshot) string {
	if btcMarket == nil || btcMarket.TodayInfo == nil {
		return ""
	}
	return strings.TrimSpace(btcMarket.TodayInfo.CurrentPrice)
}

func manualAvgPricePnLPercent(avgPrice decimal.Decimal, currentPrice, posSide string, leverage decimal.Decimal) decimal.Decimal {
	current, err := decimal.NewFromString(strings.TrimSpace(currentPrice))
	if err != nil || !avgPrice.IsPositive() || !current.IsPositive() {
		return decimal.Zero
	}
	multiplier := decimal.NewFromInt(1)
	if leverage.IsPositive() {
		multiplier = leverage
	}
	var pct decimal.Decimal
	switch strings.ToLower(strings.TrimSpace(posSide)) {
	case "long":
		pct = current.Sub(avgPrice).Div(avgPrice).Mul(decimal.NewFromInt(100))
	case "short":
		pct = avgPrice.Sub(current).Div(avgPrice).Mul(decimal.NewFromInt(100))
	default:
		return decimal.Zero
	}
	return pct.Mul(multiplier)
}

type manualPositionMetrics struct {
	QuantityBTC      decimal.Decimal
	InitialMargin    decimal.Decimal
	LiqPrice         decimal.Decimal
	UnrealizedProfit decimal.Decimal
	PnLPercent       decimal.Decimal
	HasCurrent       bool
	HasLiq           bool
}

func calculateManualPositionMetrics(input manualAICloseInput, currentPrice, posSide string) manualPositionMetrics {
	qtyBTC := input.PositionSize.Mul(decimal.NewFromFloat(0.001))
	metrics := manualPositionMetrics{QuantityBTC: qtyBTC}
	if input.AvgPrice.IsPositive() && qtyBTC.IsPositive() && input.Leverage.IsPositive() {
		metrics.InitialMargin = input.AvgPrice.Mul(qtyBTC).Div(input.Leverage)
	}
	if input.AvgPrice.IsPositive() && qtyBTC.IsPositive() && input.Balance.IsPositive() {
		switch strings.ToLower(strings.TrimSpace(posSide)) {
		case "long":
			metrics.LiqPrice = input.AvgPrice.Sub(input.Balance.Div(qtyBTC))
			if metrics.LiqPrice.IsNegative() {
				metrics.LiqPrice = decimal.Zero
			}
			metrics.HasLiq = true
		case "short":
			metrics.LiqPrice = input.AvgPrice.Add(input.Balance.Div(qtyBTC))
			metrics.HasLiq = true
		}
	}

	current, err := decimal.NewFromString(strings.TrimSpace(currentPrice))
	if err != nil || !current.IsPositive() || !qtyBTC.IsPositive() {
		return metrics
	}
	metrics.HasCurrent = true
	switch strings.ToLower(strings.TrimSpace(posSide)) {
	case "long":
		metrics.UnrealizedProfit = current.Sub(input.AvgPrice).Mul(qtyBTC)
	case "short":
		metrics.UnrealizedProfit = input.AvgPrice.Sub(current).Mul(qtyBTC)
	default:
		return metrics
	}
	if metrics.InitialMargin.IsPositive() {
		metrics.PnLPercent = metrics.UnrealizedProfit.Div(metrics.InitialMargin).Mul(decimal.NewFromInt(100))
	}
	return metrics
}

func manualMetricString(value decimal.Decimal, available bool) string {
	if !available {
		return ""
	}
	return value.StringFixed(4)
}

func normalizeManualPositionSide(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "l", "long":
		return "long"
	case "s", "short":
		return "short"
	default:
		return ""
	}
}

func optionalDecimalString(value decimal.Decimal) string {
	if !value.IsPositive() {
		return ""
	}
	return value.String()
}

func buildManualPositionSummary(acc trade.AccountConfig, input manualAICloseInput, currentPrice string, metrics manualPositionMetrics) string {
	account := acc.Name
	if acc.UID != "" {
		account = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
	}
	return fmt.Sprintf(
		"manual_input_position=true, source=telegram_ai_plus_price, account=%s, inst=BTC-USDT-SWAP, side=%s, avg=%s, last=%s, unrealized_profit=%sUSDT, pnl=%s%%, balance=%sUSDT, size=%s张, qty_btc=%sBTC, contract_value=1张=0.001BTC, calculated_margin=%sUSDT, leverage=%sx, estimated_cross_liq=%s, liq_model=全仓简化估算_忽略维持保证金和手续费, instruction=用户手工提供当前仓位均价、方向、余额、张数和杠杆倍数；禁止假设已查询交易所仓位；请基于本地计算出的爆仓价、盈亏、收益率、当前最新价和BTC市场快照给出是否继续持有/减仓/平仓的建议",
		defaultString(account, "manual-input"),
		defaultString(acc.PositionSide, "unknown"),
		input.AvgPrice.String(),
		defaultString(currentPrice, "unknown"),
		defaultString(manualMetricString(metrics.UnrealizedProfit, metrics.HasCurrent), "unknown"),
		metrics.PnLPercent.StringFixed(2),
		defaultString(optionalDecimalString(input.Balance), "unknown"),
		defaultString(optionalDecimalString(input.PositionSize), "unknown"),
		defaultString(optionalDecimalString(metrics.QuantityBTC), "unknown"),
		defaultString(optionalDecimalString(metrics.InitialMargin), "unknown"),
		defaultString(optionalDecimalString(input.Leverage), "unknown"),
		defaultString(manualMetricString(metrics.LiqPrice, metrics.HasLiq), "unknown"),
	)
}

// evaluateNoPositionAdvice 返回 (建议文本, 下次巡检间隔, 是否需要推送)。空仓建议默认不推送。
func (am *AccountMonitor) evaluateNoPositionAdvice(acc trade.AccountConfig, btcMarket *BTCAnalysisSnapshot) (string, time.Duration, bool) {
	if am.aiCloseDecider == nil {
		return "\n\n⚠️ AI空仓建议: 已禁用", 0, false
	}

	decision, err := am.aiCloseDecider.Decide(PositionSnapshot{
		AccountName:     acc.Name,
		AccountUID:      acc.UID,
		HasPosition:     false,
		BTCMarket:       btcMarket,
		PositionSummary: "no_open_position=true",
		TriggerType:     "no_position",
	})
	if err != nil {
		logrus.Errorf("[AI空仓巡检] AI建议失败: account=%s, err=%v", acc.Name, err)
		return fmt.Sprintf("\n\n⚠️ AI空仓建议: 决策失败\n原因: %v", err), 0, false
	}
	if decision == nil {
		return "\n\n⚠️ AI空仓建议: 未返回结果", 0, false
	}

	decision.ShouldClose = false
	if decision.FinalAction == "" || decision.FinalAction == "hold" || decision.FinalAction == "close" {
		decision.FinalAction = "no_trade"
	}
	next, _ := parseNextCheckInterval(decision.NextCheckIn, am.aiCloseMinInterval, am.aiCloseMaxInterval)
	logrus.Infof("[AI空仓巡检] account=%s action=%s side=%s reason=%s", acc.Name, decision.FinalAction, decision.ContinueSide, decision.Reason)
	return fmt.Sprintf("\n\n🤖 AI空仓建议\n%s", formatAICloseDecision(decision)), next, false
}

func calculatePositionPnL(pos utils.PositionInfo) (decimal.Decimal, decimal.Decimal, bool) {
	pct, pnl, ok, _ := calculatePositionPnLWithReason(pos)
	return pct, pnl, ok
}

func calculatePositionPnLWithReason(pos utils.PositionInfo) (decimal.Decimal, decimal.Decimal, bool, string) {
	if pos.UnrealizedProfit == "" || pos.UseMargin == "" {
		return decimal.Zero, decimal.Zero, false, "missing_unrealized_profit_or_use_margin"
	}

	pnl, err := decimal.NewFromString(pos.UnrealizedProfit)
	if err != nil {
		return decimal.Zero, decimal.Zero, false, "invalid_unrealized_profit"
	}
	margin, err := decimal.NewFromString(pos.UseMargin)
	if err != nil || !margin.IsPositive() {
		if err != nil {
			return decimal.Zero, decimal.Zero, false, "invalid_use_margin"
		}
		return decimal.Zero, decimal.Zero, false, "non_positive_use_margin"
	}

	return pnl.Div(margin).Mul(decimal.NewFromInt(100)), pnl, true, ""
}

func (am *AccountMonitor) formatThresholdAction(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, alertKey string, pct decimal.Decimal, triggerType string) string {
	reason := fmt.Sprintf("threshold signal: %s, pnl=%s%%", triggerType, pct.StringFixed(2))
	if triggerType == "profit" {
		return am.closePositionBySignal(tm, acc, pos, alertKey, "threshold", reason)
	}
	return fmt.Sprintf("\n\n⚠️ 阈值警示: 仅发送告警，未自动平仓\n原因: %s\n提醒: 若仓位持续超过阈值，同仓位每5分钟提醒一次", reason)
}

// evaluateAndMaybeClosePosition 返回 (告警文本, 下次巡检间隔, 是否需要推送)。
// push=true 当 AI 建议平仓或风险等级 high。
func (am *AccountMonitor) evaluateAndMaybeClosePosition(acc trade.AccountConfig, pos utils.PositionInfo, alertKey string, pct decimal.Decimal, triggerType string, btcMarket *BTCAnalysisSnapshot, availBal, totalBal string) (string, time.Duration, bool) {
	if am.aiCloseDecider == nil {
		return "\n\n⚠️ AI平仓: 已禁用", 0, false
	}

	posDetails := CurrentPositionDetails{
		InstType:         pos.InstType,
		InstID:           pos.InstId,
		PositionID:       pos.PosId,
		PositionSide:     pos.PosSide,
		PositionSize:     pos.Pos,
		AvgPrice:         pos.AvgPx,
		LastPrice:        pos.LastPx,
		LiqPrice:         pos.LiqPx,
		UseMargin:        pos.UseMargin,
		UnrealizedProfit: pos.UnrealizedProfit,
		PnLPercent:       pct.StringFixed(2),
		Leverage:         pos.Lever,
		MarginMode:       pos.MgnMode,
		MarginPosition:   pos.MrgPosition,
		Currency:         pos.Ccy,
		CreateTime:       pos.CTime,
		UpdateTime:       pos.UTime,
	}

	decision, err := am.aiCloseDecider.Decide(PositionSnapshot{
		AccountName:        acc.Name,
		AccountUID:         acc.UID,
		HasPosition:        true,
		CurrentPosition:    posDetails,
		BTCMarket:          btcMarket,
		PositionSummary:    buildPositionSummary(posDetails),
		InstID:             pos.InstId,
		PositionID:         pos.PosId,
		PositionSide:       pos.PosSide,
		PositionSize:       pos.Pos,
		AvgPrice:           pos.AvgPx,
		LastPrice:          pos.LastPx,
		LiqPrice:           pos.LiqPx,
		UseMargin:          pos.UseMargin,
		UnrealizedProfit:   pos.UnrealizedProfit,
		PnLPercent:         pct,
		TriggerType:        triggerType,
		AvailBal:           availBal,
		TotalBal:           totalBal,
		LiqDistancePercent: computeLiqDistancePercent(pos.LastPx, pos.LiqPx, pos.PosSide),
	})
	if err != nil {
		logrus.Errorf("[持仓监控] AI平仓决策失败: %s, err=%v", alertKey, err)
		// 决策失败本身值得推送提醒
		return fmt.Sprintf("\n\n⚠️ AI平仓: 决策失败\n原因: %v", err), 0, true
	}

	if decision == nil {
		return "\n\n⚠️ AI平仓: 未返回结果", 0, true
	}

	next, _ := parseNextCheckInterval(decision.NextCheckIn, am.aiCloseMinInterval, am.aiCloseMaxInterval)
	riskHigh := strings.EqualFold(strings.TrimSpace(decision.RiskLevel), "high")

	if !decision.ShouldClose {
		decisionSummary := formatAICloseDecision(decision)
		logrus.Infof("[持仓监控] AI决定继续持有: %s, provider=%s, risk=%s, reason=%s", alertKey, decision.Provider, decision.RiskLevel, decision.Reason)
		// 继续持有时，仅高风险才推送；普通 hold 只记日志降噪。
		return fmt.Sprintf("\n\n🤖 AI决策: 继续观察\n%s", decisionSummary), next, riskHigh
	}

	logrus.Infof("[持仓监控] AI建议平仓但仅通知: %s, provider=%s, reason=%s", alertKey, decision.Provider, decision.Reason)
	return fmt.Sprintf("\n\n⚠️ AI决策: 建议平仓（仅通知，未创建审批，未执行平仓）\n%s", formatAICloseDecision(decision)), next, true
}

func (am *AccountMonitor) closePositionBySignal(tm *trade.TradeManager, acc trade.AccountConfig, pos utils.PositionInfo, alertKey, source, reason string) string {
	webClient := tm.GetWebClient(acc.Name)
	if webClient == nil {
		return fmt.Sprintf("\n\n⚠️ 平仓信号: 已产生但跳过\n来源: %s\n原因: %s\n详情: 无Web客户端配置", source, reason)
	}
	if pos.PosId == "" {
		return fmt.Sprintf("\n\n⚠️ 平仓信号: 已产生但跳过\n来源: %s\n原因: %s\n详情: 无PositionID", source, reason)
	}

	logrus.Infof("[持仓监控] 执行平仓信号: %s, source=%s, PositionID=%s", alertKey, source, pos.PosId)
	closeResp, closeErr := webClient.ClosePosition(pos.PosId)
	if closeErr != nil {
		logrus.Errorf("[持仓监控] 平仓信号执行失败: %s, source=%s, err=%v", alertKey, source, closeErr)
		return fmt.Sprintf("\n\n🔴 平仓信号执行: 失败\n来源: %s\n原因: %s\n错误: %v", source, reason, closeErr)
	}

	logrus.Infof("[持仓监控] 平仓信号执行成功: %s, source=%s, spend=%d", alertKey, source, closeResp.Data.Spend)
	return fmt.Sprintf("\n\n🟢 平仓信号执行: 成功 (耗时%dms)\n来源: %s\n原因: %s", closeResp.Data.Spend, source, reason)
}

// CloseAllPositions 一键平仓所有账户的所有持仓
func (am *AccountMonitor) CloseAllPositions() string {
	if !trade.IsInitialized() {
		return "⚠️ 交易管理器未初始化，无法执行平仓"
	}

	tm := trade.GetManager()
	if tm == nil {
		return "⚠️ 无法获取交易管理器"
	}

	config := trade.GetConfig()
	if config == nil || len(config.Accounts) == 0 {
		return "⚠️ 没有配置账户"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	message := fmt.Sprintf("🔄 一键平仓执行报告\n⏰ 时间: %s\n\n", now)
	totalClosed := 0
	totalFailed := 0
	totalSkipped := 0

	for i, acc := range config.Accounts {
		client := tm.GetClient(acc.Name)
		if client == nil {
			message += fmt.Sprintf("❌ 账户 %s: 客户端不存在\n\n", acc.Name)
			continue
		}

		posResp, err := client.GetPositionsTyped(&utils.GetPositionsRequest{
			InstType: "SWAP",
		})
		if err != nil {
			logrus.Errorf("[一键平仓] 账户 %s 查询持仓失败: %v", acc.Name, err)
			message += fmt.Sprintf("❌ 账户 %s: 查询持仓失败: %v\n\n", acc.Name, err)
			continue
		}

		if len(posResp.Data) == 0 {
			message += fmt.Sprintf("✅ 账户 %s: 暂无持仓\n\n", acc.Name)
			continue
		}

		accountDisplay := acc.Name
		if acc.UID != "" {
			accountDisplay = fmt.Sprintf("%s-%s", acc.Name, acc.UID)
		}
		message += fmt.Sprintf("📋 账户 %s (%d个持仓)\n", accountDisplay, len(posResp.Data))

		webClient := tm.GetWebClient(acc.Name)

		for _, pos := range posResp.Data {
			directionEmoji := "🔵"
			if pos.PosSide == "short" {
				directionEmoji = "🔴"
			}

			if webClient == nil {
				message += fmt.Sprintf("  %s %s %s: ⚠️ 无Web客户端，跳过\n", directionEmoji, pos.InstId, pos.PosSide)
				totalSkipped++
				continue
			}

			if pos.PosId == "" {
				message += fmt.Sprintf("  %s %s %s: ⚠️ 无PosId，跳过\n", directionEmoji, pos.InstId, pos.PosSide)
				totalSkipped++
				continue
			}

			logrus.Infof("[一键平仓] 执行市价全平: 账户=%s, %s %s, PosId=%s", acc.Name, pos.InstId, pos.PosSide, pos.PosId)
			closeResp, closeErr := webClient.ClosePosition(pos.PosId)
			if closeErr != nil {
				logrus.Errorf("[一键平仓] 平仓失败: 账户=%s, %s %s, err=%v", acc.Name, pos.InstId, pos.PosSide, closeErr)
				message += fmt.Sprintf("  %s %s %s %s张: 🔴 失败 (%v)\n", directionEmoji, pos.InstId, pos.PosSide, pos.Pos, closeErr)
				totalFailed++
			} else {
				logrus.Infof("[一键平仓] 平仓成功: 账户=%s, %s %s, spend=%d", acc.Name, pos.InstId, pos.PosSide, closeResp.Data.Spend)
				message += fmt.Sprintf("  %s %s %s %s张: 🟢 成功 (耗时%dms)\n", directionEmoji, pos.InstId, pos.PosSide, pos.Pos, closeResp.Data.Spend)
				totalClosed++
			}
		}
		message += "\n"

		if i < len(config.Accounts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	message += "━━━━━━━━━━━━━━━━━━━━\n"
	message += fmt.Sprintf("📊 汇总: 成功=%d, 失败=%d, 跳过=%d\n", totalClosed, totalFailed, totalSkipped)

	return message
}
