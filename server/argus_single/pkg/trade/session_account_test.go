package trade

import (
	"context"
	"path/filepath"
	"testing"
)

// accountQueryParams 账户接口签名字段
type accountQueryParams struct {
	AppID       int    `json:"appid"`
	ConvertPOST int    `json:"convertPOST"`
	RandomStr   string `json:"randomstr"`
	Timestamp   int64  `json:"timestamp"`
}

// checkSessionValidViaWAPI 测试用封装，直接调用生产代码中的 CheckSessionValidViaWAPI
func checkSessionValidViaWAPI(entry SessionAccountData) (bool, error) {
	return CheckSessionValidViaWAPI(context.Background(), entry)
}


// TestSessionAccountValid 读取 session.json，调用 DeepCoin 账户接口，验证 session 是否有效
func TestSessionAccountValid(t *testing.T) {
	sessionPath := filepath.Clean("../../configs/session.json")
	store := NewSessionStore(sessionPath)
	if err := store.Load(); err != nil {
		t.Fatalf("加载 session.json 失败: %v", err)
	}
	if len(store.data.Accounts) == 0 {
		t.Fatalf("session.json 中没有账号")
	}

	var entry SessionAccountData
	var accountKey string
	for k, v := range store.data.Accounts {
		accountKey = k
		entry = v
		break
	}
	t.Logf("使用账号: %s (uid=%s, updatedAt=%s)", accountKey, entry.UID, entry.UpdatedAt)

	token := firstNonEmpty(entry.OToken, entry.Token)
	if token == "" {
		t.Fatal("session.json 中 token 为空，请先登录")
	}
	if entry.Cookie == "" {
		t.Fatal("session.json 中 cookie 为空，请先登录")
	}
	t.Logf("token (前30字符): %s...", token[:min(30, len(token))])
	t.Logf("cookie 长度: %d", len(entry.Cookie))

	valid, err := checkSessionValidViaWAPI(entry)
	if err != nil {
		t.Fatalf("session 检测失败: %v", err)
	}
	if valid {
		t.Logf("✅ session 有效！")
	} else {
		t.Logf("❌ session 已失效")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
