package initialization

import (
	"log"

	"common/middleware/db"
	"common/middleware/vipper"
	newssvc "service/news"
	pressuresvc "service/pressure"
	tradesvc "service/trade"

	"oracle/pkg/oraclecfg"
	"oracle/pkg/scheduler"
)

// Init 统一初始化入口：配置 → DB → 建表 → 启动调度。
func Init() {
	log.Printf("[oracle] Initializing Config...")
	vipper.Init()

	log.Printf("[oracle] Initializing DB...")
	db.InitDB()
	if db.Db == nil {
		log.Fatalf("[oracle] 数据库未初始化，无法落库预测")
	}

	cfg := oraclecfg.Load()

	tradeService := tradesvc.NewTradeService()
	if err := tradeService.EnsureTable(); err != nil {
		log.Fatalf("[oracle] 建表失败: %v", err)
	}

	newsService := newssvc.NewNewsService()
	if err := newsService.EnsureTable(); err != nil {
		log.Fatalf("[oracle] 消息面建表失败: %v", err)
	}

	pressureService := pressuresvc.NewPressureService()
	if err := pressureService.EnsureTable(); err != nil {
		log.Fatalf("[oracle] 压力面建表失败: %v", err)
	}

	sched := scheduler.New(cfg, tradeService, newsService, pressureService)
	sched.Start()
	log.Printf("[oracle] 启动完成，进入运行态")

	// 常驻
	select {}
}
