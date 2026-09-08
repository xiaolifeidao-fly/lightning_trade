// argus-episode-rebuild 把事件库里的离散事件归集成 episode（一次完整持仓）
// 与 episode_entry（一次建仓决策 + 归给它的已实现盈亏）。
//
// 派生是纯函数：输入只有 strategy_event / balance_sample，输出的两张表可随时
// 整表 DROP 再跑一次，结果一致。因此本工具**总是整实例替换**，没有增量模式——
// episode 会跨天（实测最长 68.8 小时），按日期切片会把跨界持仓砍成假 episode。
//
// 用法照 cmd/argus-event-import 的先例：一次可跑多个实例，默认 dry run。
//
//	# 先看一眼派生结果，一行都不写库
//	go run ./cmd/argus-episode-rebuild --instance argus-single-roc \
//	  --report ../../doc/module/r10-7289d6a1b3/report/episode-argus-single-roc.md
//
//	# 确认后写库
//	go run ./cmd/argus-episode-rebuild --instance argus-single-roc --apply
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"common/middleware/vipper"
	argusConfig "service/argus_config"

	"argus_single/pkg/eventstore"
	"argus_single/pkg/eventstore/episode"

	"gorm.io/gorm"
)

// instanceList 支持重复传 --instance；每个实例各自整表替换，互不影响。
type instanceList []string

func (l *instanceList) String() string { return strings.Join(*l, ",") }

func (l *instanceList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("--instance must not be empty")
	}
	normalized, err := argusConfig.NormalizeInstanceKey(trimmed)
	if err != nil {
		return err
	}
	*l = append(*l, normalized)
	return nil
}

func main() {
	var instances instanceList
	flag.Var(&instances, "instance", "目标实例键，可重复；每个实例整表重建")
	dsn := flag.String("dsn", "", "事件库 DSN；缺省时读 configs/application.properties 的 sqlconn")
	apply := flag.Bool("apply", false, "真正写库；缺省只派生并出报告")
	reportDir := flag.String("report-dir", "", "报告输出目录；每个实例一份 episode-<实例键>.md")
	reportPath := flag.String("report", "", "单实例报告输出路径；与 --report-dir 二选一")
	batchSize := flag.Int("batch-size", 500, "单条 INSERT 的行数")
	flag.Parse()

	if len(instances) == 0 {
		log.Fatal("at least one --instance is required: rebuild replaces every episode row of that instance")
	}
	if *reportPath != "" && len(instances) > 1 {
		log.Fatal("--report only works with a single --instance; use --report-dir for several")
	}

	db := openEventDB(*dsn)
	if *apply {
		if err := episode.EnsureTables(db); err != nil {
			log.Fatalf("ensure episode tables: %v", err)
		}
	} else if !db.Migrator().HasTable(&episode.Episode{}) {
		log.Print("note: episode tables do not exist yet; this dry run only derives in memory")
	}

	// rebuiltAt 一次取好、所有实例共用：同一次重建的行带同一个时间戳，
	// 便于事后一眼看出哪些行是同一批产出的。
	rebuiltAt := time.Now()
	failed := false
	for _, instanceKey := range instances {
		assertInstanceRegistered(db, instanceKey)
		events, err := episode.LoadEvents(db, instanceKey)
		if err != nil {
			log.Fatalf("load events for %s: %v", instanceKey, err)
		}
		episodes, stats := episode.Derive(events, rebuiltAt)
		if err := episode.ApplyDepthFidelity(db, episodes); err != nil {
			log.Fatalf("classify depth fidelity for %s: %v", instanceKey, err)
		}

		report := &episode.Report{
			InstanceKey: instanceKey,
			GeneratedAt: rebuiltAt,
			Mode:        runMode(*apply),
			Episodes:    episodes,
			Stats:       stats,
		}
		if *apply {
			written, err := episode.Replace(db, instanceKey, episodes, *batchSize)
			if err != nil {
				log.Fatalf("write episodes for %s: %v", instanceKey, err)
			}
			report.Write = written
			log.Printf("rebuilt %s episodes=%d entries=%d (replaced %d/%d old rows)",
				instanceKey, written.Episodes, written.Entries, written.DeletedEpisodes, written.DeletedEntries)
		}
		log.Printf("derived %s events=%d episodes=%d gaps=%d truncatedHead=%d missingPnl=%d",
			instanceKey, stats.Events, stats.Episodes, stats.PositionGaps, stats.TruncatedHead, stats.MissingPnlEvents)
		emitReport(report, reportTarget(*reportPath, *reportDir, instanceKey))
		// 落不到任何决策上的盈亏 = 派生出的归因有缺口，需要人看一眼是不是窗口截断造成的。
		if stats.SideFlipRejected > 0 {
			log.Printf("warning: %s has %d reverse orders that grew the position; reverse_gate should have blocked those",
				instanceKey, stats.SideFlipRejected)
			failed = true
		}
	}
	if !*apply {
		log.Print("dry run complete; rerun with --apply to replace the derived tables")
	}
	if failed {
		os.Exit(2)
	}
}

// openEventDB 走 eventstore 自己的 DSN 口径（强制 loc=Local / parseTime /
// utf8mb4 / 超时），与 argus-event-import 一致：派生读回的 ts 必须与事件表
// 逐字一致，差 8 小时会让 episode 的窗口整体偏移。
func openEventDB(explicitDSN string) *gorm.DB {
	raw := strings.TrimSpace(explicitDSN)
	if raw == "" {
		vipper.Init()
		raw = strings.TrimSpace(vipper.GetString("sqlconn"))
	}
	if raw == "" {
		log.Fatal("event store DSN is empty: pass --dsn or run from server/manager-api with sqlconn configured")
	}
	db, err := eventstore.OpenDB(raw)
	if err != nil {
		log.Fatalf("open event store: %v", err)
	}
	return db
}

// assertInstanceRegistered 与 argus-event-import 同一口径：注册表不存在或为空
// 时放行（首次导入时它本来就是空的），登记过但查不到这个键才拦。
func assertInstanceRegistered(db *gorm.DB, instanceKey string) {
	if !db.Migrator().HasTable("argus_instance") {
		log.Printf("warning: argus_instance registry does not exist yet; skipping registration check for %s", instanceKey)
		return
	}
	var total int64
	if err := db.Table("argus_instance").Count(&total).Error; err != nil {
		log.Printf("warning: argus_instance registry is unavailable (%v); skipping registration check", err)
		return
	}
	if total == 0 {
		log.Printf("warning: argus_instance registry is empty; run argus-config-import to register %s", instanceKey)
		return
	}
	var matched int64
	if err := db.Table("argus_instance").Where("instance_key = ?", instanceKey).Count(&matched).Error; err != nil {
		log.Fatalf("check argus instance registry: %v", err)
	}
	if matched == 0 {
		log.Fatalf("instance %s is not registered in argus_instance; run argus-config-import first", instanceKey)
	}
}

func reportTarget(single, dir, instanceKey string) string {
	if strings.TrimSpace(single) != "" {
		return single
	}
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return strings.TrimRight(dir, "/") + "/episode-" + instanceKey + ".md"
}

func emitReport(report *episode.Report, path string) {
	if strings.TrimSpace(path) == "" {
		fmt.Print(report.Render())
		return
	}
	if err := report.WriteFile(path); err != nil {
		log.Fatalf("write report: %v", err)
	}
	log.Printf("episode report written to %s", path)
}

func runMode(apply bool) string {
	if apply {
		return "rebuild"
	}
	return "dry-run"
}
