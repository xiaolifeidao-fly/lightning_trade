package initialization

import (
	"fmt"
	"log"

	"argus_single/pkg/monitor"
	"argus_single/pkg/trade"
	"common/middleware/vipper"
)

// Init 统一初始化入口
func Init() {
	// 初始化配置
	log.Printf("Initializing Config...")
	vipper.Init()
	log.Printf("Config initialized successfully")

	// 初始化路由
	log.Printf("Initializing Router...")
	// routers.Init()
	log.Printf("Router initialized successfully")

	// 初始化价格监控器
	log.Printf("Initializing Price Monitor...")
	symbolConfigs := loadSymbolConfigs()
	monitor.InitMonitor(symbolConfigs)
	log.Printf("Price Monitor initialized successfully")

	// 初始化交易管理器（必须在账户监控器启动前完成）
	log.Printf("Initializing Trade Manager...")
	trade.InitFromConfig()
	log.Printf("Trade Manager initialized successfully")

	// 启动时检测所有账户 session 有效性（net-wapi 接口），失效则无头模式重新登录
	log.Printf("Checking session validity for all accounts...")
	trade.EnsureSessionsReady()
	log.Printf("Session check completed")

	// 启动价格监控
	log.Printf("Starting Price Monitor...")
	go monitor.StartMonitor()
	log.Printf("Price Monitor started successfully")

	// 初始化账户监控器
	log.Printf("Initializing Account Monitor...")
	monitor.InitAccountMonitor()
	log.Printf("Account Monitor initialized successfully")

	// 启动账户监控
	log.Printf("Starting Account Monitor...")
	go monitor.StartAccountMonitor()
	log.Printf("Account Monitor started successfully")

	// 初始化Telegram Bot
	log.Printf("Initializing Telegram Bot...")
	monitor.InitTelegramBot()
	log.Printf("Telegram Bot initialized successfully")

	// 启动Telegram Bot
	log.Printf("Starting Telegram Bot...")
	go monitor.StartTelegramBot()
	log.Printf("Telegram Bot started successfully")
}

// loadSymbolConfigs 从配置文件读取监控币种配置
func loadSymbolConfigs() map[string]monitor.SymbolConfig {
	configs := make(map[string]monitor.SymbolConfig)
	// 枚举已知的币种 key，vipper 不支持动态枚举子key，所以逐个读取
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT"}
	for _, symbol := range symbols {
		deepInst := vipper.GetString(fmt.Sprintf("monitor.symbols.%s.deep_inst", symbol))
		threshold := vipper.GetFloat64(fmt.Sprintf("monitor.symbols.%s.threshold", symbol))
		if deepInst != "" && threshold > 0 {
			tradeInst := vipper.GetString(fmt.Sprintf("monitor.symbols.%s.trade_inst", symbol))
			if tradeInst == "" {
				tradeInst = symbol // 默认使用 symbol key，如 BTCUSDT
			}
			configs[symbol] = monitor.SymbolConfig{
				DeepInst:  deepInst,
				TradeInst: tradeInst,
				Threshold: threshold,
			}
		}
	}
	if len(configs) == 0 {
		log.Printf("警告: 未从配置文件读取到任何监控币种，使用默认配置 BTCUSDT")
		configs["BTCUSDT"] = monitor.SymbolConfig{
			DeepInst:  "BTC-USDT-SWAP",
			TradeInst: "BTCUSDT",
			Threshold: 0.0012,
		}
	}
	return configs
}
