package monitor

import (
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	globalAccountMonitor *AccountMonitor
	accountMonitorOnce   sync.Once
	globalTelegramBot    *TelegramBot
	telegramBotOnce      sync.Once
)

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

// CloseAllPositions 一键平仓所有账户所有持仓（供外部调用）
func CloseAllPositions() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.CloseAllPositions()
}

func ApprovePendingAIClose(id string) string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.ApprovePendingAIClose(id)
}

func RejectPendingAIClose(id string) string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.RejectPendingAIClose(id)
}

func ListPendingAICloseRequests() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.ListPendingAICloseRequests()
}

func RunAICloseStrategyNow() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.RunAICloseStrategyNow()
}

func RunAIOpenStrategyNow() string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.RunAIOpenStrategyNow()
}

func RunAICloseStrategyWithManualPosition(input manualAICloseInput) string {
	if globalAccountMonitor == nil {
		return "⚠️ 账户监控器未初始化"
	}
	return globalAccountMonitor.RunAICloseStrategyWithManualPosition(input)
}

// InitTelegramBot 初始化全局Telegram Bot
func InitTelegramBot() {
	telegramBotOnce.Do(func() {
		globalTelegramBot = NewTelegramBot()
		logrus.Info("Telegram Bot已初始化")
	})
}

// StartTelegramBot 启动全局Telegram Bot
func StartTelegramBot() {
	if globalTelegramBot == nil {
		logrus.Error("Telegram Bot未初始化，请先调用 InitTelegramBot")
		return
	}
	globalTelegramBot.Start()
}

// StopTelegramBot 停止全局Telegram Bot
func StopTelegramBot() {
	if globalTelegramBot != nil {
		globalTelegramBot.Stop()
	}
}

// GetTelegramBot 获取全局Telegram Bot实例
func GetTelegramBot() *TelegramBot {
	return globalTelegramBot
}

func GetTelegramBotMention() string {
	if globalTelegramBot == nil {
		return ""
	}
	return globalTelegramBot.BotName()
}
