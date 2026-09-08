package initialization

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
	"argus_single/pkg/monitor"
	"argus_single/pkg/runtimeconfig"
	"argus_single/pkg/runtimehealth"
	"argus_single/pkg/trade"
	"argus_single/routers"
	"common/middleware/db"
	"common/middleware/redis"
	"common/middleware/vipper"

	"github.com/sirupsen/logrus"
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

	// 策略事件双写 MySQL（JSONL 仍是真源，保留至 eventstore.JSONLDualWriteUntil）。
	// 必须在任何交易/监控启动前注册 sink，否则早期事件只会落进 JSONL。
	// 建连接或建表失败都不算启动失败：进程退化成只写 JSONL，交易照常。
	if _, err := eventstore.Setup(context.Background(), eventstore.Options{
		DSN:         vipper.GetString("sqlconn"),
		InstanceKey: runtimeManager.InstanceID(),
	}); err != nil {
		if eventstore.IsDisabled(err) {
			log.Printf("Event store disabled (sqlconn empty), events are written to JSONL only")
		} else {
			logrus.Errorf("策略事件双写初始化失败，本次只写 JSONL（不影响交易）: %v", err)
		}
	} else {
		eventstore.SetConfigVersion(runtime.Version)
		eventstore.SetAccounts(accountIdentities(runtime.Trade))
	}

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
	heartbeat.SetConfigState(runtime.Version, runtime.Checksum)
	// 配置热加载后事件的 config_version 与 uid 映射都要跟着走：版本号错位会让
	// 「哪个参数版本产生了这批触发」的归因失真，账户变更后 uid 不更新会让新
	// 账户的事件永远缺 uid。apply() 是先写 current 再回调，所以这里读到的是新配置。
	runtimeManager.SetReloadObserver(func(version uint64, err error) {
		// apply() 先写 current 再回调，成功时 Current() 已是新快照；失败时它
		// 仍是仍在运行的旧快照，正好是心跳该上报的「程序实际读到什么」。
		heartbeat.RecordReload(version, runtimeManager.Current().Checksum, err)
		if err != nil || version == 0 {
			return
		}
		eventstore.SetConfigVersion(version)
		eventstore.SetAccounts(accountIdentities(runtimeManager.Current().Trade))
	})
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

// accountIdentities 从运行时配置抽出 account 标签 → uid 映射。
// JSONL 的 account 字段存的是人起的标签名（含邮箱），数字 uid 从来没进过事件，
// 只能从配置补；uid 为空时事件按 (instance_key, account_label) 唯一。
func accountIdentities(cfg *trade.TradingSystemConfig) []eventstore.Identity {
	if cfg == nil {
		return nil
	}
	identities := make([]eventstore.Identity, 0, len(cfg.Accounts))
	for _, acc := range cfg.Accounts {
		identities = append(identities, eventstore.Identity{Label: acc.Name, UID: acc.UID})
	}
	return identities
}
