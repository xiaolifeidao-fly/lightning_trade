package initialization

import (
	"context"
	"log"
	"time"

	"common/middleware/db"
	"common/middleware/vipper"
	tradesvc "service/trade"

	"oracle/pkg/backfill"
	"oracle/pkg/oraclecfg"
)

// RunBackfill 历史预测回填入口：配置 → DB → 建表 → 跑回填 → 退出（不进入常驻调度）。
// 由 cmd.go 在 -backfill 模式下调用。
func RunBackfill(opts backfill.Options) {
	log.Printf("[oracle][backfill] Initializing Config...")
	vipper.Init()

	log.Printf("[oracle][backfill] Initializing DB...")
	db.InitDB()
	if db.Db == nil {
		log.Fatalf("[oracle][backfill] 数据库未初始化，无法落库预测")
	}

	cfg := oraclecfg.Load()

	tradeService := tradesvc.NewTradeService()
	if err := tradeService.EnsureTable(); err != nil {
		log.Fatalf("[oracle][backfill] 建表失败: %v", err)
	}

	// 回填整体超时：按锚点规模放宽，单次 LLM 走 cfg.AI.Timeout。
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Hour)
	defer cancel()

	if err := backfill.Run(ctx, cfg, tradeService, opts); err != nil {
		log.Fatalf("[oracle][backfill] 回填失败: %v", err)
	}
	log.Printf("[oracle][backfill] 回填完成，退出")
}
