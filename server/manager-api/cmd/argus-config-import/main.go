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
	configPath := flag.String("config", "", "path to BADelay application.properties")
	sessionPath := flag.String("session", "", "path to BADelay session.json")
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
	log.Printf("validated import: accounts=%d sessions=%d monitorSymbols=%d telegramConfigured=%t", summary.Accounts, summary.Sessions, summary.MonitorSymbols, summary.TelegramSet)
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
	if _, err := service.GetPublished(context.Background()); err == nil {
		log.Fatal("a published Argus configuration already exists; refusing to overwrite it")
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Fatalf("check published Argus configuration: %v", err)
	}

	request.ReleaseNote = strings.TrimSpace(*releaseNote)
	draft, err := service.SaveDraft(request, strings.TrimSpace(*actor))
	if err != nil {
		log.Fatalf("save imported draft: %v", err)
	}
	published, err := service.Publish(context.Background(), draft.ID, &argusDTO.PublishConfigRequest{ReleaseNote: request.ReleaseNote}, strings.TrimSpace(*actor))
	if err != nil {
		log.Fatalf("publish imported configuration: %v", err)
	}
	log.Printf("imported and published Argus configuration version=%d checksum=%s", published.Version, published.SnapshotChecksum)
}

func hasValidEncryptionKey(encodedKey string) bool {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	return err == nil && len(key) == 32
}
