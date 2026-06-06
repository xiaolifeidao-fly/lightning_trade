package trade

import (
	"github.com/sirupsen/logrus"
)

// IntegratePriceMonitor 示例：在价格监控中集成套利交易
func IntegratePriceMonitor(symbol, deepInst string, binPrice, deepPrice, threshold float64) {
	diff := (binPrice - deepPrice) / deepPrice

	if diff >= threshold || -diff >= threshold {
		logrus.Infof("触发套利: 价差=%.4f%%", diff*100)

		err := ExecuteArbitrage(deepInst, binPrice, deepPrice)
		if err != nil {
			logrus.Errorf("套利失败: %v", err)
			return
		}

		logrus.Infof("✅ 套利执行完成")
	}
}

// CheckAccountStatus 示例：查询所有账户状态
func CheckAccountStatus() {
	status := GetAccountStatus()

	for name, info := range status {
		logrus.Infof("账户 %s: %+v", name, info)
	}
}
