package initialization

import (
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

	// 初始化交易管理器（必须在账户监控器启动前完成）
	log.Printf("Initializing Trade Manager...")
	trade.InitFromConfig()
	log.Printf("Trade Manager initialized successfully")

	// 启动时检测所有账户 session 有效性（net-wapi 接口），失效则无头模式重新登录
	log.Printf("Checking session validity for all accounts...")
	trade.EnsureSessionsReady()
	log.Printf("Session check completed")

	// 启动 BTC 行情数据服务（AI 检测点依赖）
	log.Printf("Starting BTC Market Data Feed...")
	go monitor.StartBTCMarketDataFeed()
	log.Printf("BTC Market Data Feed started successfully")

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
