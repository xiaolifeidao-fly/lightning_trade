package main

import (
	"context"
	"encoding/base64"
	"errors"
	"flag"
	"log"
	"os"
	"path/filepath"
	"strings"

	"common/middleware/db"
	commonRedis "common/middleware/redis"
	"common/middleware/vipper"
	argusConfig "service/argus_config"
	argusDTO "service/argus_config/dto"

	"gorm.io/gorm"
)

func main() {
	configPath := flag.String("config", "", "path to one deployment instance application[_suffix].properties")
	sessionPath := flag.String("session", "", "path to BADelay session.json")
	instanceKey := flag.String("instance", "", "target instance key; defaults to argus.instance.id inside --config")
	instanceName := flag.String("instance-name", "", "instance display name; defaults to the instance key")
	apply := flag.Bool("apply", false, "write and publish a new Argus configuration version")
	releaseNote := flag.String("release-note", "import BADelay main configuration", "published version release note")
	actor := flag.String("actor", "argus-config-import", "audit actor")
	flag.Parse()

	if strings.TrimSpace(*configPath) == "" || strings.TrimSpace(*sessionPath) == "" {
		log.Fatal("both --config and --session are required")
	}
	request, summary, err := argusConfig.LoadMainConfigImport(filepath.Clean(*configPath), filepath.Clean(*sessionPath))
	if err != nil {
		log.Fatalf("validate source files: %v", err)
	}
	// 实例键优先取命令行，其次取 properties 里的 argus.instance.id。
	targetInstance := strings.TrimSpace(*instanceKey)
	if targetInstance == "" {
		targetInstance = summary.InstanceKey
	}
	if targetInstance == "" {
		log.Fatalf("instance key is required: pass --instance or set argus.instance.id in %s", summary.SourceFile)
	}
	targetInstance, err = argusConfig.NormalizeInstanceKey(targetInstance)
	if err != nil {
		log.Fatalf("invalid instance key: %v", err)
	}
	request.InstanceKey = targetInstance
	log.Printf("validated import: instance=%s source=%s accounts=%d sessions=%d monitorSymbols=%d telegramConfigured=%t", targetInstance, summary.SourceFile, summary.Accounts, summary.Sessions, summary.MonitorSymbols, summary.TelegramSet)
	if !*apply {
		log.Print("dry run complete; rerun from server/manager-api with --apply after setting ARGUS_CONFIG_ENCRYPTION_KEY")
		return
	}
	if !hasValidEncryptionKey(os.Getenv("ARGUS_CONFIG_ENCRYPTION_KEY")) {
		log.Fatal("ARGUS_CONFIG_ENCRYPTION_KEY must be a base64-encoded 32-byte key when --apply is used")
	}

	vipper.Init()
	db.InitDB()
	if db.Db == nil {
		log.Fatal("database initialization failed")
	}
	if err := commonRedis.InitRedisClient(vipper.GetString("redis.addr"), vipper.GetString("redis.password")); err != nil {
		log.Fatalf("redis initialization failed: %v", err)
	}

	service := argusConfig.NewArgusConfigService()
	if err := service.EnsureTable(); err != nil {
		log.Fatalf("ensure Argus configuration tables: %v", err)
	}
	// 先把实例登记进 argus_instance，实例键重复时会打印告警并复用原记录。
	instance, err := service.EnsureInstance(&argusDTO.SaveInstanceRequest{
		InstanceKey:  targetInstance,
		InstanceName: strings.TrimSpace(*instanceName),
		ConfigSource: summary.SourceFile,
	})
	if err != nil {
		log.Fatalf("register Argus instance %s: %v", targetInstance, err)
	}
	log.Printf("registered Argus instance id=%d instanceKey=%s configSource=%s", instance.ID, instance.InstanceKey, instance.ConfigSource)

	// 已发布检查按实例隔离，其他实例已有发布不影响本实例导入。
	if snapshot, err := service.GetPublished(context.Background(), targetInstance); err == nil && snapshot != nil {
		log.Fatalf("instance %s already has a published Argus configuration; refusing to overwrite it", targetInstance)
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatalf("check published Argus configuration: %v", err)
	}

	request.ReleaseNote = strings.TrimSpace(*releaseNote)
	draft, err := service.SaveDraft(targetInstance, request, strings.TrimSpace(*actor))
	if err != nil {
		log.Fatalf("save imported draft: %v", err)
	}
	published, err := service.Publish(context.Background(), targetInstance, draft.ID, &argusDTO.PublishConfigRequest{InstanceKey: targetInstance, ReleaseNote: request.ReleaseNote}, strings.TrimSpace(*actor))
	if err != nil {
		log.Fatalf("publish imported configuration: %v", err)
	}
	log.Printf("imported and published Argus configuration instance=%s version=%d checksum=%s", targetInstance, published.Version, published.SnapshotChecksum)
}

func hasValidEncryptionKey(encodedKey string) bool {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	return err == nil && len(key) == 32
}
