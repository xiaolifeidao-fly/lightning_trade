// argus-event-import 把 logs/argus_single 下的历史事件 JSONL 回灌进事件库，
// 并在 JSONL/MySQL 双写期（至 2026-12-01）产出每日一致性巡检报告。
//
// 用法照 cmd/argus-config-import 的先例：一次跑一个实例，默认 dry run，
// 加 --apply 才真写库。
//
//	# 实例1（application_1.properties，两个账户），回灌全部历史目录
//	go run ./cmd/argus-event-import \
//	  --instance argus-single-roc \
//	  --config ../argus_single/configs/application_1.properties \
//	  --source ../../logs/argus_single/events-0702 \
//	  --source ../../logs/argus_single/8.18-8.21剧烈上涨 \
//	  --report ../../doc/module/r6-cf8beef011/report/argus-single-roc.md \
//	  --apply
//
//	# 双写期每日巡检：只比对不写库
//	go run ./cmd/argus-event-import --instance argus-single-roc \
//	  --config ... --source ... --since 2026-09-01 --until 2026-09-01 --audit
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"common/middleware/vipper"
	argusConfig "service/argus_config"

	"argus_single/pkg/eventstore"
	"argus_single/pkg/eventstore/backfill"

	"gorm.io/gorm"
)

// sourceList 支持重复传 --source，一个实例可以有多个历史快照目录。
type sourceList []string

func (s *sourceList) String() string { return strings.Join(*s, ",") }

func (s *sourceList) Set(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("--source must not be empty")
	}
	*s = append(*s, filepath.Clean(trimmed))
	return nil
}

func main() {
	var sources sourceList
	flag.Var(&sources, "source", "历史 JSONL 来源目录，可重复；该目录下的事件全部归属 --instance")
	configPath := flag.String("config", "", "该实例的 application[_suffix].properties，用于构建 account 标签 → uid 映射")
	instanceKey := flag.String("instance", "", "目标实例键；缺省时取 --config 里的 argus.instance.id")
	dsn := flag.String("dsn", "", "事件库 DSN；缺省时读 configs/application.properties 的 sqlconn")
	since := flag.String("since", "", "起始日期（含），YYYY-MM-DD")
	until := flag.String("until", "", "结束日期（含），YYYY-MM-DD")
	apply := flag.Bool("apply", false, "真正写库；缺省只做 dry run")
	audit := flag.Bool("audit", false, "只做 JSONL ↔ MySQL 比对，不写任何行")
	reportPath := flag.String("report", "", "一致性巡检报告输出路径（Markdown）")
	batchSize := flag.Int("batch-size", 500, "单条 INSERT 的行数")
	flag.Parse()

	if len(sources) == 0 {
		log.Fatal("at least one --source directory is required")
	}
	if strings.TrimSpace(*configPath) == "" {
		log.Fatal("--config is required: account label → uid mapping can only come from the instance properties")
	}
	identities, sourceFile, propertiesInstance := loadIdentities(filepath.Clean(*configPath))

	targetInstance := strings.TrimSpace(*instanceKey)
	if targetInstance == "" {
		targetInstance = propertiesInstance
	}
	if targetInstance == "" {
		log.Fatalf("instance key is required: pass --instance or set argus.instance.id in %s", sourceFile)
	}
	targetInstance, err := argusConfig.NormalizeInstanceKey(targetInstance)
	if err != nil {
		log.Fatalf("invalid instance key: %v", err)
	}
	// 实例键与 properties 不一致时只告警不拦：历史目录可能来自改名之前的部署，
	// 归属由人按目录判断，工具不替他决定。
	if propertiesInstance != "" && propertiesInstance != targetInstance {
		log.Printf("warning: --instance %s differs from argus.instance.id %s in %s; source directories are attributed to %s",
			targetInstance, propertiesInstance, sourceFile, targetInstance)
	}

	job := backfill.Job{
		InstanceKey: targetInstance,
		Sources:     sources,
		Identities:  identities,
		Since:       strings.TrimSpace(*since),
		Until:       strings.TrimSpace(*until),
	}
	built, err := backfill.Build(job)
	if err != nil {
		log.Fatalf("scan source directories: %v", err)
	}
	rawEvents, unique, duplicates, rows := built.Totals()
	log.Printf("scanned instance=%s files=%d days=%d events=%d unique=%d duplicates=%d rows=%d",
		targetInstance, len(built.Files), len(built.Days), rawEvents, unique, duplicates, rows)

	report := &backfill.Report{
		InstanceKey: targetInstance,
		GeneratedAt: time.Now(),
		Mode:        runMode(*apply, *audit),
		Sources:     sources,
		Since:       job.Since,
		Until:       job.Until,
		Build:       built,
		Writes:      map[string]backfill.WriteStat{},
	}

	if !*apply && !*audit {
		log.Print("dry run complete; rerun with --apply to write, or --audit to compare against MySQL only")
		emitReport(report, *reportPath)
		return
	}

	db := openEventDB(*dsn)
	assertInstanceRegistered(db, targetInstance)
	if *apply {
		if err := backfill.EnsureTables(db); err != nil {
			log.Fatalf("ensure event tables: %v", err)
		}
		for _, day := range built.Days {
			stat, err := backfill.WriteDay(db, day, *batchSize)
			if err != nil {
				log.Fatalf("write %s: %v", day.Date, err)
			}
			report.Writes[day.Date] = stat
			log.Printf("imported %s submitted=%d inserted=%d existed=%d", day.Date, stat.Submitted, stat.Inserted, stat.Existed)
		}
	}

	missing, extra := 0, 0
	jsonlDates := make(map[string]struct{}, len(built.Days))
	for _, day := range built.Days {
		jsonlDates[day.Date] = struct{}{}
		result, err := backfill.AuditDay(db, day)
		if err != nil {
			log.Fatalf("audit %s: %v", day.Date, err)
		}
		report.Audits = append(report.Audits, result)
		missing += result.MissingTotal
		extra += result.ExtraTotal
	}
	orphans, err := backfill.FindOrphanDays(db, targetInstance, jsonlDates, job.Since, job.Until)
	if err != nil {
		log.Fatalf("scan for days missing from JSONL: %v", err)
	}
	report.Orphans = orphans
	log.Printf("consistency check complete: days=%d missing=%d extra=%d jsonlMissingDays=%d",
		len(report.Audits), missing, extra, len(orphans))
	emitReport(report, *reportPath)
	if missing > 0 || len(orphans) > 0 {
		// 缺口是需要人处置的结果，用非零退出码让定时巡检能直接告警。
		os.Exit(2)
	}
}

func loadIdentities(configPath string) ([]eventstore.Identity, string, string) {
	loaded, err := argusConfig.LoadInstanceAccounts(configPath)
	if err != nil {
		log.Fatalf("read instance accounts: %v", err)
	}
	identities := make([]eventstore.Identity, 0, len(loaded.Accounts))
	for _, account := range loaded.Accounts {
		if account.UID == "" {
			log.Fatalf("trade.account uid is empty for %q in %s; uid must not be guessed from the label", account.Name, loaded.SourceFile)
		}
		identities = append(identities, eventstore.Identity{Label: account.Name, UID: account.UID})
	}
	log.Printf("loaded %d account identities from %s (instance=%s)", len(identities), loaded.SourceFile, loaded.InstanceKey)
	return identities, loaded.SourceFile, loaded.InstanceKey
}

// openEventDB 走 eventstore 自己的 DSN 口径（强制 loc=Local / parseTime /
// utf8mb4 / 超时），不复用 common/middleware/db 的全局连接——回灌读回的 ts
// 必须与 JSONL 逐字一致，差 8 小时会让每一行都报不一致。
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

// assertInstanceRegistered 校验实例已登记进 argus_instance（r1 的注册表）。
// 注册表不存在或为空时放行，与 argus_config 的自举口径一致：首次导入时
// 注册表本来就是空的，不能因此把回灌卡死。
func assertInstanceRegistered(db *gorm.DB, instanceKey string) {
	if !db.Migrator().HasTable("argus_instance") {
		log.Printf("warning: argus_instance registry does not exist yet; skipping instance registration check for %s", instanceKey)
		return
	}
	var total int64
	if err := db.Table("argus_instance").Count(&total).Error; err != nil {
		log.Printf("warning: argus_instance registry is unavailable (%v); skipping instance registration check", err)
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
		log.Fatalf("instance %s is not registered in argus_instance; run argus-config-import first so events are attributed to a known instance", instanceKey)
	}
}

func emitReport(report *backfill.Report, path string) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		fmt.Print(report.Render())
		return
	}
	if err := report.Write(trimmed); err != nil {
		log.Fatalf("write report: %v", err)
	}
	log.Printf("consistency report written to %s", trimmed)
}

func runMode(apply, audit bool) string {
	switch {
	case apply && audit:
		return "import + audit"
	case apply:
		return "import"
	case audit:
		return "audit"
	default:
		return "dry-run"
	}
}
