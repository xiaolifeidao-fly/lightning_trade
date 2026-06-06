package trade

import (
	"github.com/sirupsen/logrus"
)

func InitFromConfig() {
	config, err := LoadConfigFromProperties()
	if err != nil {
		logrus.Warnf("⚠️  加载交易配置失败: %v，跳过交易功能初始化", err)
		return
	}

	InitTradeManager(config)
}

func ExampleUsage() {
	// 执行套利交易
	binPrice := 95100.0
	deepPrice := 95000.0

	err := ExecuteArbitrage("BTC-USDT-SWAP", binPrice, deepPrice)
	if err != nil {
		logrus.Errorf("套利交易失败: %v", err)
		return
	}

	// 获取账户状态
	status := GetAccountStatus()
	logrus.Infof("账户状态: %+v", status)
}
