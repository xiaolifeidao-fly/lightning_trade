package monitor

import (
	"argus_single/pkg/trade"

	"github.com/sirupsen/logrus"
)

// SetupTradeIntegration 设置交易集成
func SetupTradeIntegration(pm *PriceMonitor) {
	// 初始化交易管理器
	trade.InitFromConfig()

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
