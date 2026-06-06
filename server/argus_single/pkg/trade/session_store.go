package trade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultSessionFilePath = "./configs/session.json"

type SessionAccountData struct {
	AccountName     string   `json:"accountName"`
	URL             string   `json:"url,omitempty"`
	UID             string   `json:"uid,omitempty"`
	LoginType       string   `json:"loginType,omitempty"`
	LoginHeadless   *bool    `json:"loginHeadless,omitempty"`
	Username        string   `json:"username,omitempty"`
	Password        string   `json:"password,omitempty"`
	GoogleAuthKey   string   `json:"googleAuthKey,omitempty"`
	APIKey          string   `json:"apiKey,omitempty"`
	SecretKey       string   `json:"secretKey,omitempty"`
	Passphrase      string   `json:"passphrase,omitempty"`
	ResourceID      string   `json:"resourceId,omitempty"`
	Cookie          string   `json:"cookie,omitempty"`
	Token           string   `json:"token,omitempty"`
	OToken          string   `json:"otoken,omitempty"`
	SentryRelease   string   `json:"sentryRelease,omitempty"`
	SentryPublicKey string   `json:"sentryPublicKey,omitempty"`
	Baggage         string   `json:"baggage,omitempty"`
	LoginURL        string   `json:"loginURL,omitempty"`
	FinalURL        string   `json:"finalURL,omitempty"`
	InitialBalance  *float64 `json:"initialBalance,omitempty"`
	UpdatedAt       string   `json:"updatedAt,omitempty"`
}

type SessionFileData struct {
	Accounts map[string]SessionAccountData `json:"accounts"`
}

type SessionStore struct {
	path string

	mu   sync.RWMutex
	data SessionFileData
}

var (
	defaultSessionStore     *SessionStore
	defaultSessionStoreOnce sync.Once
)

func DefaultSessionStore() *SessionStore {
	defaultSessionStoreOnce.Do(func() {
		store := NewSessionStore(defaultSessionFilePath)
		if err := store.Load(); err != nil {
			store.data = SessionFileData{Accounts: map[string]SessionAccountData{}}
		}
		defaultSessionStore = store
	})
	return defaultSessionStore
}

func NewSessionStore(path string) *SessionStore {
	return &SessionStore{path: path}
}

func (s *SessionStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureFileLocked(); err != nil {
		return err
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("读取 session 文件失败: %w", err)
	}

	data := SessionFileData{Accounts: map[string]SessionAccountData{}}
	if len(strings.TrimSpace(string(raw))) != 0 {
		if err := json.Unmarshal(raw, &data); err != nil {
			return fmt.Errorf("解析 session 文件失败: %w", err)
		}
	}
	if data.Accounts == nil {
		data.Accounts = map[string]SessionAccountData{}
	}
	s.data = data
	return nil
}

func (s *SessionStore) Get(acc AccountConfig) (SessionAccountData, bool) {
	key := acc.SessionKey()

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.data.Accounts == nil {
		return SessionAccountData{}, false
	}
	if key == "" && len(s.data.Accounts) == 1 {
		for _, entry := range s.data.Accounts {
			return entry, true
		}
	}
	entry, ok := s.data.Accounts[key]
	return entry, ok
}

func (s *SessionStore) SaveFromLoginResult(acc AccountConfig, result *DeepCoinLoginResult) error {
	if result == nil {
		return fmt.Errorf("session 保存失败: 登录结果为空")
	}

	fmt.Printf("[session] SaveFromLoginResult account=%s cookieLen=%d token=%q oToken=%q sentryRelease=%q baggage=%q\n",
		acc.Name, len(result.Cookie), result.Token, result.OToken, result.SentryRelease, result.Baggage)

	entry := SessionAccountData{
		AccountName:     acc.Name,
		URL:             acc.URL,
		UID:             acc.UID,
		LoginType:       acc.LoginType,
		LoginHeadless:   &acc.LoginHeadless,
		Username:        acc.Username,
		Password:        acc.Password,
		GoogleAuthKey:   acc.GoogleAuthKey,
		APIKey:          acc.APIKey,
		SecretKey:       acc.SecretKey,
		Passphrase:      acc.Passphrase,
		ResourceID:      firstNonEmpty(result.ResourceID, acc.Username),
		Cookie:          result.Cookie,
		Token:           firstNonEmpty(result.OToken, result.Token),
		OToken:          firstNonEmpty(result.OToken, result.Token),
		SentryRelease:   result.SentryRelease,
		SentryPublicKey: result.SentryPublicKey,
		Baggage:         result.Baggage,
		LoginURL:        result.LoginURL,
		FinalURL:        result.FinalURL,
		UpdatedAt:       time.Now().Format(time.RFC3339),
	}

	return s.Save(acc, entry)
}

func (s *SessionStore) Save(acc AccountConfig, entry SessionAccountData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureFileLocked(); err != nil {
		return err
	}
	if s.data.Accounts == nil {
		s.data.Accounts = map[string]SessionAccountData{}
	}
	if entry.AccountName == "" {
		entry.AccountName = acc.Name
	}
	if entry.UID == "" {
		entry.UID = acc.UID
	}
	if entry.Username == "" {
		entry.Username = acc.Username
	}
	if entry.ResourceID == "" {
		entry.ResourceID = acc.Username
	}
	if entry.UpdatedAt == "" {
		entry.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	key := acc.SessionKey()
	if existing, ok := s.data.Accounts[key]; ok {
		entry = mergeSessionAccountData(existing, entry)
	}
	s.data.Accounts[key] = entry

	payload, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 session 文件失败: %w", err)
	}
	payload = append(payload, '\n')

	tempPath := s.path + ".tmp"
	if err := os.WriteFile(tempPath, payload, 0o644); err != nil {
		return fmt.Errorf("写入 session 临时文件失败: %w", err)
	}
	if err := os.Rename(tempPath, s.path); err != nil {
		return fmt.Errorf("替换 session 文件失败: %w", err)
	}
	return nil
}

func (s *SessionStore) ensureFileLocked() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建 session 目录失败: %w", err)
	}
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		initial := []byte("{\n  \"accounts\": {}\n}\n")
		if err := os.WriteFile(s.path, initial, 0o644); err != nil {
			return fmt.Errorf("初始化 session 文件失败: %w", err)
		}
	}
	return nil
}

func sessionDataToUser(acc AccountConfig, entry SessionAccountData) (*SessionAccountData, bool) {
	token := firstNonEmpty(entry.OToken, entry.Token)
	if strings.TrimSpace(entry.Cookie) == "" || strings.TrimSpace(token) == "" {
		return nil, false
	}

	entry.AccountName = firstNonEmpty(entry.AccountName, acc.Name)
	entry.UID = firstNonEmpty(entry.UID, acc.UID)
	entry.Username = firstNonEmpty(entry.Username, acc.Username)
	entry.ResourceID = firstNonEmpty(entry.ResourceID, acc.Username)
	entry.Token = token
	entry.OToken = token
	entry.SentryRelease = firstNonEmpty(entry.SentryRelease, acc.SentryRelease)
	entry.SentryPublicKey = firstNonEmpty(entry.SentryPublicKey, acc.SentryPublicKey)
	return &entry, true
}

func sessionNeedsRefreshByUpdatedAt(entry SessionAccountData, now time.Time, maxAgeDays int) (bool, string) {
	if maxAgeDays <= 0 {
		maxAgeDays = defaultSessionMaxAgeDays
	}

	updatedAtRaw := strings.TrimSpace(entry.UpdatedAt)
	if updatedAtRaw == "" {
		return true, "updatedAt 为空"
	}

	updatedAt, err := parseSessionUpdatedAt(updatedAtRaw)
	if err != nil {
		return true, fmt.Sprintf("updatedAt 解析失败: %v", err)
	}

	age := now.Sub(updatedAt)
	if age < 0 {
		return false, fmt.Sprintf("updatedAt 在未来 age=%s", age.Round(time.Second))
	}

	maxAge := time.Duration(maxAgeDays) * 24 * time.Hour
	if age > maxAge {
		return true, fmt.Sprintf("age=%s > maxAge=%s", age.Round(time.Second), maxAge)
	}
	return false, fmt.Sprintf("age=%s <= maxAge=%s", age.Round(time.Second), maxAge)
}

func parseSessionUpdatedAt(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	var lastErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed, nil
		}
		lastErr = err
	}
	return time.Time{}, lastErr
}

func applySessionAccountData(acc *AccountConfig, entry SessionAccountData) {
	acc.Name = firstNonEmpty(entry.AccountName, acc.Name)
	acc.URL = firstNonEmpty(entry.URL, acc.URL)
	acc.UID = firstNonEmpty(entry.UID, acc.UID)
	acc.LoginType = firstNonEmpty(entry.LoginType, acc.LoginType)
	if entry.LoginHeadless != nil {
		acc.LoginHeadless = *entry.LoginHeadless
	}
	acc.Username = firstNonEmpty(entry.Username, acc.Username)
	acc.Password = firstNonEmpty(entry.Password, acc.Password)
	// secretKey 作为 googleAuthKey 的兜底：用户在 session.json 中可能用 secretKey 字段存 TOTP 密钥
	acc.GoogleAuthKey = firstNonEmpty(entry.GoogleAuthKey, entry.SecretKey, acc.GoogleAuthKey)
	acc.APIKey = firstNonEmpty(entry.APIKey, acc.APIKey)
	acc.SecretKey = firstNonEmpty(entry.SecretKey, acc.SecretKey)
	acc.Passphrase = firstNonEmpty(entry.Passphrase, acc.Passphrase)
	acc.Cookie = firstNonEmpty(entry.Cookie, acc.Cookie)
	acc.Token = firstNonEmpty(firstNonEmpty(entry.OToken, entry.Token), acc.Token)
	acc.SentryRelease = firstNonEmpty(entry.SentryRelease, acc.SentryRelease)
	acc.SentryPublicKey = firstNonEmpty(entry.SentryPublicKey, acc.SentryPublicKey)
	acc.LoginURL = firstNonEmpty(entry.LoginURL, acc.LoginURL)
	if entry.InitialBalance != nil {
		acc.InitialBalance = *entry.InitialBalance
	}
}

func mergeSessionAccountData(existing, incoming SessionAccountData) SessionAccountData {
	merged := existing
	merged.AccountName = firstNonEmpty(incoming.AccountName, merged.AccountName)
	merged.URL = firstNonEmpty(incoming.URL, merged.URL)
	merged.UID = firstNonEmpty(incoming.UID, merged.UID)
	merged.LoginType = firstNonEmpty(incoming.LoginType, merged.LoginType)
	if incoming.LoginHeadless != nil {
		merged.LoginHeadless = incoming.LoginHeadless
	}
	merged.Username = firstNonEmpty(incoming.Username, merged.Username)
	merged.Password = firstNonEmpty(incoming.Password, merged.Password)
	merged.GoogleAuthKey = firstNonEmpty(incoming.GoogleAuthKey, merged.GoogleAuthKey)
	merged.APIKey = firstNonEmpty(incoming.APIKey, merged.APIKey)
	merged.SecretKey = firstNonEmpty(incoming.SecretKey, merged.SecretKey)
	merged.Passphrase = firstNonEmpty(incoming.Passphrase, merged.Passphrase)
	merged.ResourceID = firstNonEmpty(incoming.ResourceID, merged.ResourceID)
	merged.Cookie = firstNonEmpty(incoming.Cookie, merged.Cookie)
	merged.Token = firstNonEmpty(incoming.Token, merged.Token)
	merged.OToken = firstNonEmpty(incoming.OToken, merged.OToken)
	merged.SentryRelease = firstNonEmpty(incoming.SentryRelease, merged.SentryRelease)
	merged.SentryPublicKey = firstNonEmpty(incoming.SentryPublicKey, merged.SentryPublicKey)
	merged.Baggage = firstNonEmpty(incoming.Baggage, merged.Baggage)
	merged.LoginURL = firstNonEmpty(incoming.LoginURL, merged.LoginURL)
	merged.FinalURL = firstNonEmpty(incoming.FinalURL, merged.FinalURL)
	if incoming.InitialBalance != nil {
		merged.InitialBalance = incoming.InitialBalance
	}
	merged.UpdatedAt = firstNonEmpty(incoming.UpdatedAt, merged.UpdatedAt)
	return merged
}
