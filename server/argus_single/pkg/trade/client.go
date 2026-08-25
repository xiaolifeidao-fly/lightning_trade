package trade

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	globalManager   *TradeManager
	globalManagerMu sync.RWMutex
)

func InitTradeManager(config *TradingSystemConfig) {
	globalManagerMu.Lock()
	defer globalManagerMu.Unlock()
	if globalManager != nil {
		logrus.Infof("交易管理器已初始化，跳过重复初始化")
		return
	}
	globalManager = NewTradeManager(config)
	logrus.Infof("✅ 交易管理器已初始化: %d个账户", len(config.Accounts))
}

func GetManager() *TradeManager {
	globalManagerMu.RLock()
	defer globalManagerMu.RUnlock()
	return globalManager
}

func IsInitialized() bool {
	globalManagerMu.RLock()
	defer globalManagerMu.RUnlock()
	return globalManager != nil
}

// ReplaceManager builds the replacement before publishing it, so callers can
// safely keep using the old manager while the new configuration is prepared.
func ReplaceManager(config *TradingSystemConfig) {
	replacement := NewTradeManager(config)

	globalManagerMu.Lock()
	previous := globalManager
	globalManager = replacement
	globalManagerMu.Unlock()

	if previous != nil {
		previous.Stop()
	}
	logrus.Infof("✅ 交易管理器已热替换: %d个账户", len(config.Accounts))
}

// EnsureSessionsReady 启动时主动检测所有账户 session，失效则触发无头登录。
func EnsureSessionsReady() {
	manager := GetManager()
	if manager == nil {
		logrus.Warn("交易管理器未初始化，跳过 session 检测")
		return
	}
	manager.EnsureSessionsReady()
}

// ============================= 套利交易 =============================

func ExecuteArbitrage(instId string, binPrice, deepPrice float64) error {
	manager := GetManager()
	if manager == nil {
		logrus.Warnf("交易管理器未初始化，跳过交易")
		return nil
	}
	return manager.ExecuteArbitrage_From_WEB(instId, binPrice, deepPrice)
}

// ExecuteSignalTrade 返回 (opened, err)：opened=true 仅当至少一个账户真正开仓。
func ExecuteSignalTrade(instId string, price float64, direction string, q SignalQuote) (bool, error) {
	manager := GetManager()
	if manager == nil {
		logrus.Warnf("交易管理器未初始化，跳过盘口信号交易")
		return false, nil
	}
	return manager.ExecuteSignalTrade_From_WEB(instId, price, direction, q)
}

// ============================= 账户状态 =============================

func GetAccountStatus() map[string]interface{} {
	manager := GetManager()
	if manager == nil {
		return map[string]interface{}{
			"error": "交易管理器未初始化",
		}
	}
	return manager.GetAccountStatus()
}

// ============================= 配置管理 =============================

func SetCooldown(seconds int) {
	if manager := GetManager(); manager != nil {
		manager.SetCooldown(time.Duration(seconds) * time.Second)
		logrus.Infof("交易冷却时间已设置: %ds", seconds)
	}
}

func GetConfig() *TradingSystemConfig {
	if manager := GetManager(); manager != nil {
		return manager.Config()
	}
	return nil
}

func HasSpreadLogicAccounts() bool {
	manager := GetManager()
	return manager != nil && manager.HasSpreadLogicAccounts()
}

func HasSignalLogicAccounts() bool {
	manager := GetManager()
	return manager != nil && manager.HasSignalLogicAccounts()
}
