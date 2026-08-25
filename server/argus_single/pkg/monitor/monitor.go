package monitor

import (
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	globalMonitor        *PriceMonitor
	monitorMu            sync.RWMutex
	globalAccountMonitor *AccountMonitor
	accountMonitorOnce   sync.Once
	globalTelegramBot    *TelegramBot
	telegramBotMu        sync.RWMutex
)

// InitMonitor 初始化全局监控器
func InitMonitor(symbolConfigs map[string]SymbolConfig) {
	monitorMu.Lock()
	defer monitorMu.Unlock()
	if globalMonitor == nil {
		globalMonitor = NewPriceMonitor(symbolConfigs)
		logrus.Info("价格监控器已初始化")
	}
}

func ReloadMonitor(symbolConfigs map[string]SymbolConfig) {
	replacement := NewPriceMonitor(symbolConfigs)
	monitorMu.Lock()
	previous := globalMonitor
	globalMonitor = replacement
	monitorMu.Unlock()
	if previous != nil {
		previous.Stop()
	}
	SetupTradeIntegration(replacement)
	go replacement.Start()
	logrus.Infof("价格监控器已热替换: %d个币种", len(symbolConfigs))
}

// StartMonitor 启动全局监控器
func StartMonitor() {
	monitorMu.RLock()
	current := globalMonitor
	monitorMu.RUnlock()
	if current == nil {
		logrus.Error("监控器未初始化，请先调用 InitMonitor")
		return
	}

	// 设置交易集成
	SetupTradeIntegration(current)

	current.Start()
}

// StopMonitor 停止全局监控器
func StopMonitor() {
	if current := GetMonitor(); current != nil {
		current.Stop()
	}
}

// GetMonitor 获取全局监控器实例
func GetMonitor() *PriceMonitor {
	monitorMu.RLock()
	defer monitorMu.RUnlock()
	return globalMonitor
}

func GetSystemStatus() string {
	current := GetMonitor()
	if current == nil {
		return "⚠️ 价格监控器未初始化"
	}
	if current.signalScheduler == nil {
		return "⚠️ 盘口信号调度器未初始化"
	}
	return current.signalScheduler.StatusReport(hasSignalTradeAccounts())
}

func ResetSignalStateAfterClose(instId, posSide string) {
	current := GetMonitor()
	if current == nil || current.signalScheduler == nil {
		return
	}
	current.signalScheduler.ResetAfterClose(instId, posSide)
}

// InitAccountMonitor 初始化全局账户监控器
func InitAccountMonitor() {
	accountMonitorOnce.Do(func() {
		globalAccountMonitor = NewAccountMonitor()
		logrus.Info("账户监控器已初始化")
	})
}

// StartAccountMonitor 启动全局账户监控器
func StartAccountMonitor() {
	if globalAccountMonitor == nil {
		logrus.Error("账户监控器未初始化，请先调用 InitAccountMonitor")
		return
	}
	globalAccountMonitor.Start()
}

// StopAccountMonitor 停止全局账户监控器
func StopAccountMonitor() {
	if globalAccountMonitor != nil {
		globalAccountMonitor.Stop()
	}
}

// GetAccountMonitor 获取全局账户监控器实例
func GetAccountMonitor() *AccountMonitor {
	return globalAccountMonitor
}

// GetBalanceReport 获取余额报告（供外部调用）
func GetBalanceReport() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.GetBalanceReport()
}

// GetPositionReport 获取持仓报告（供外部调用）
func GetPositionReport() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.GetPositionReport()
}

// GetROIReport 获取收益率报告（供外部调用）
func GetROIReport() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.GetROIReport()
}

// CloseAllPositions 一键平仓所有账户所有持仓（供外部调用）
func CloseAllPositions() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.CloseAllPositions()
}

// ClosePositionsBySide 平仓指定方向持仓（供外部调用）
func ClosePositionsBySide(posSide string) string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.ClosePositionsBySide(posSide)
}

// InitTelegramBot 初始化全局Telegram Bot
func InitTelegramBot() {
	telegramBotMu.Lock()
	defer telegramBotMu.Unlock()
	if globalTelegramBot == nil {
		globalTelegramBot = NewTelegramBot()
		logrus.Info("Telegram Bot已初始化")
	}
}

func ReloadTelegramBot() {
	replacement := NewTelegramBot()
	telegramBotMu.Lock()
	previous := globalTelegramBot
	globalTelegramBot = replacement
	telegramBotMu.Unlock()
	if previous != nil {
		previous.Stop()
	}
	go replacement.Start()
	logrus.Info("Telegram Bot已热替换")
}

// StartTelegramBot 启动全局Telegram Bot
func StartTelegramBot() {
	telegramBotMu.RLock()
	current := globalTelegramBot
	telegramBotMu.RUnlock()
	if current == nil {
		logrus.Error("Telegram Bot未初始化，请先调用 InitTelegramBot")
		return
	}
	current.Start()
}

// StopTelegramBot 停止全局Telegram Bot
func StopTelegramBot() {
	if current := GetTelegramBot(); current != nil {
		current.Stop()
	}
}

// GetTelegramBot 获取全局Telegram Bot实例
func GetTelegramBot() *TelegramBot {
	telegramBotMu.RLock()
	defer telegramBotMu.RUnlock()
	return globalTelegramBot
}
