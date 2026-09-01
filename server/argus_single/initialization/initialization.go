package initialization

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/monitor"
	"argus_single/pkg/runtimeconfig"
	"argus_single/pkg/runtimehealth"
	"argus_single/pkg/trade"
	"argus_single/routers"
	"common/middleware/db"
	"common/middleware/redis"
	"common/middleware/vipper"
)

// Init 统一初始化入口
func Init() error {
	// 初始化配置
	log.Printf("Initializing Config...")
	vipper.Init()
	log.Printf("Config initialized successfully")
	db.InitDB()
	if db.Db == nil {
		return fmt.Errorf("Argus configuration database is unavailable")
	}
	if err := redis.InitRedisClient(vipper.GetString("redis.addr"), vipper.GetString("redis.password")); err != nil {
		return fmt.Errorf("initialize Argus Redis client: %w", err)
	}
	runtimeManager, runtime, err := runtimeconfig.Initialize(context.Background())
	if err != nil {
		return fmt.Errorf("load Argus runtime configuration: %w", err)
	}
	if err := runtimeconfig.ApplyInitial(runtime); err != nil {
		return err
	}
	runtimeManager.InstallSessionWriteBack()

	// 初始化结构化事件日志（须在任何交易/监控启动前，避免早期信号事件被静默丢弃）
	eventLogDir := vipper.GetString("log.dir")
	if strings.TrimSpace(eventLogDir) == "" {
		eventLogDir = "./logs"
	}
	eventlog.Init(eventLogDir)
	log.Printf("Event log initialized at %s", eventLogDir)

	// 初始化路由
	log.Printf("Initializing Router...")
	routers.Init()
	log.Printf("Router initialized successfully")

	// 初始化价格监控器
	log.Printf("Initializing Price Monitor...")
	symbolConfigs := runtime.Symbols
	monitor.InitMonitor(symbolConfigs)
	log.Printf("Price Monitor initialized successfully")

	// 初始化交易管理器（必须在账户监控器启动前完成）
	log.Printf("Initializing Trade Manager...")
	// 交易管理器已经由运行时快照安全初始化。
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

	if err := runtimeManager.Start(context.Background()); err != nil {
		return fmt.Errorf("start Argus config subscription: %w", err)
	}
	heartbeat := runtimehealth.New(
		runtimeManager.InstanceID(),
		vipper.GetString("argus.build.version"),
		time.Duration(vipper.GetInt("argus.heartbeat.interval_seconds"))*time.Second,
		time.Duration(vipper.GetInt("argus.heartbeat.ttl_seconds"))*time.Second,
	)
	heartbeat.SetVersion(runtime.Version)
	runtimeManager.SetReloadObserver(heartbeat.RecordReload)
	heartbeat.Start(context.Background())
	runtimehealth.SetDefaultReporter(heartbeat)
	return nil
}

// loadSymbolConfigs 从配置文件读取监控币种配置
func loadSymbolConfigs() map[string]monitor.SymbolConfig {
	configs := make(map[string]monitor.SymbolConfig)
	// 枚举已知的币种 key，vipper 不支持动态枚举子key，所以逐个读取
	symbols := []string{"BTCUSDT", "ETHUSDT", "BNBUSDT", "SOLUSDT"}
	for _, symbol := range symbols {
		deepInst := vipper.GetString(fmt.Sprintf("monitor.symbols.%s.deep_inst", symbol))
		threshold := vipper.GetFloat64(fmt.Sprintf("monitor.symbols.%s.threshold", symbol))
		signalThreshold := vipper.GetFloat64(fmt.Sprintf("monitor.symbols.%s.signal_threshold", symbol))
		if deepInst != "" && threshold > 0 {
			tradeInst := vipper.GetString(fmt.Sprintf("monitor.symbols.%s.trade_inst", symbol))
			if tradeInst == "" {
				tradeInst = symbol // 默认使用 symbol key，如 BTCUSDT
			}
			if signalThreshold <= 0 {
				signalThreshold = 0.0005
			}
			configs[symbol] = monitor.SymbolConfig{
				DeepInst:        deepInst,
				TradeInst:       tradeInst,
				Threshold:       threshold,
				SignalThreshold: signalThreshold,
			}
		}
	}
	if len(configs) == 0 {
		log.Printf("警告: 未从配置文件读取到任何监控币种，使用默认配置 BTCUSDT")
		configs["BTCUSDT"] = monitor.SymbolConfig{
			DeepInst:        "BTC-USDT-SWAP",
			TradeInst:       "BTCUSDT",
			Threshold:       0.0012,
			SignalThreshold: 0.0005,
		}
	}
	return configs
}
