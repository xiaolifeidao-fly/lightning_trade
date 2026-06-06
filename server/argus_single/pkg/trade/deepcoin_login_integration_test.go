package trade

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestDeepCoinLoginAndSaveSession(t *testing.T) {
	initManualLoginTestConfig(t)

	sessionPath := filepath.Clean("../../configs/session.json")
	store := NewSessionStore(sessionPath)
	if err := store.Load(); err != nil {
		t.Fatalf("加载 session.json 失败: %v", err)
	}

	acc := accountConfigForManualLoginTest(t, store)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	provider := NewLoginUserProvider(acc, NewDeepCoinLoginService(), store)
	t.Logf("登录生产流程配置：login.session_max_age_days=%d", sessionMaxAgeDays())

	u, err := provider.GetUser(ctx)
	if err != nil {
		t.Fatalf("DeepCoin 登录生产流程失败: %v", err)
	}

	entry, _ := store.Get(acc)
	t.Logf("DeepCoin 登录生产流程完成，session=%s, account=%s, updatedAt=%s, cookieLen=%d, tokenLen=%d",
		sessionPath, acc.Name, entry.UpdatedAt, len(u.Cookie), len(u.Token))
}

func initManualLoginTestConfig(t *testing.T) {
	t.Helper()

	configPath := filepath.Clean("../../configs/application.properties")
	viper.SetConfigFile(configPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("加载配置文件失败 %s: %v", configPath, err)
	}
}

func accountConfigForManualLoginTest(t *testing.T, store *SessionStore) AccountConfig {
	t.Helper()

	if len(store.data.Accounts) == 0 {
		t.Fatalf("session.json 中没有账号，请先在 session.json.accounts 中写入账号信息")
	}

	var accountName string
	var entry SessionAccountData
	for key, candidate := range store.data.Accounts {
		candidateName := firstNonEmpty(candidate.AccountName, key)
		candidateUsername := firstNonEmpty(candidate.Username, candidateName)
		if strings.TrimSpace(candidateUsername) != "" && strings.TrimSpace(candidate.Password) != "" {
			accountName = key
			entry = candidate
			break
		}
	}
	if accountName == "" {
		t.Fatalf("session.json 中没有可登录账号，请确认目标账号包含 username/password 字段")
	}

	acc := AccountConfig{
		Name:          firstNonEmpty(entry.AccountName, accountName),
		UID:           entry.UID,
		Username:      firstNonEmpty(entry.Username, firstNonEmpty(entry.AccountName, accountName)),
		Password:      entry.Password,
		GoogleAuthKey: firstNonEmpty(entry.GoogleAuthKey, entry.SecretKey),
		APIKey:        entry.APIKey,
		SecretKey:     entry.SecretKey,
		Passphrase:    entry.Passphrase,
		LoginType:     WebCredentialModePassword,
		LoginURL:      firstNonEmpty(entry.LoginURL, DefaultDeepCoinLoginURL),
		LoginHeadless: true,
	}
	return acc
}
