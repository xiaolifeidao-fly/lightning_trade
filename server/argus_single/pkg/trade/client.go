package trade

import (
	"time"

	"github.com/sirupsen/logrus"
)

var (
	globalManager *TradeManager
)

func InitTradeManager(config *TradingSystemConfig) {
	if globalManager != nil {
		logrus.Infof("交易管理器已初始化，跳过重复初始化")
		return
	}
	globalManager = NewTradeManager(config)
	logrus.Infof("✅ 交易管理器已初始化: %d个账户", len(config.Accounts))
}

func GetManager() *TradeManager {
	return globalManager
}

func IsInitialized() bool {
	return globalManager != nil
}

// EnsureSessionsReady 启动时主动检测所有账户 session，失效则触发无头登录。
func EnsureSessionsReady() {
	if globalManager == nil {
		logrus.Warn("交易管理器未初始化，跳过 session 检测")
		return
	}
	globalManager.EnsureSessionsReady()
}

// ============================= 套利交易 =============================

func ExecuteArbitrage(instId string, binPrice, deepPrice float64) error {
	if globalManager == nil {
		logrus.Warnf("交易管理器未初始化，跳过交易")
		return nil
	}
	return globalManager.ExecuteArbitrage_From_WEB(instId, binPrice, deepPrice)
}

// ============================= 账户状态 =============================

func GetAccountStatus() map[string]interface{} {
	if globalManager == nil {
		return map[string]interface{}{
			"error": "交易管理器未初始化",
		}
	}
	return globalManager.GetAccountStatus()
}

// ============================= 配置管理 =============================

func SetCooldown(seconds int) {
	if globalManager != nil {
		globalManager.SetCooldown(time.Duration(seconds) * time.Second)
		logrus.Infof("交易冷却时间已设置: %ds", seconds)
	}
}

func GetConfig() *TradingSystemConfig {
	if globalManager == nil {
		return nil
	}
	return globalManager.config
}
