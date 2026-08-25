package monitor

import (
	"argus_single/pkg/trade"

	"github.com/sirupsen/logrus"
)

// SetupTradeIntegration 设置交易集成
func SetupTradeIntegration(pm *PriceMonitor) {
	// 运行时配置已在启动/热加载路径中创建交易管理器；仅为旧的属性文件启动方式保留兜底。
	if !trade.IsInitialized() {
		trade.InitFromConfig()
	}

	if !trade.IsInitialized() {
		logrus.Warnf("⚠️  交易功能未启用")
		return
	}

	logrus.Infof("✅ 价格监控已集成交易功能")
}

// executeArbitrageTrade 执行套利交易（由price_monitor调用）
func executeArbitrageTradeInternal(instId string, binPrice, deepPrice float64) {
	if !trade.IsInitialized() {
		return
	}

	err := trade.ExecuteArbitrage(instId, binPrice, deepPrice)
	if err != nil {
		logrus.Errorf("套利交易执行失败: %v", err)
	}
}

// executeSignalTradeInternal 返回 opened：是否确有账户开仓（供调度器判定 POSITION/IDLE）。
func executeSignalTradeInternal(instId string, price float64, direction string, q trade.SignalQuote) bool {
	if !trade.IsInitialized() {
		return false
	}

	opened, err := trade.ExecuteSignalTrade(instId, price, direction, q)
	if err != nil {
		logrus.Errorf("盘口信号交易执行失败: %v", err)
	}
	return opened
}

func hasSpreadTradeAccounts() bool {
	return trade.HasSpreadLogicAccounts()
}

func hasSignalTradeAccounts() bool {
	return trade.HasSignalLogicAccounts()
}
