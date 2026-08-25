package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"argus_single/pkg/monitor"
	"argus_single/pkg/trade"
	"common/middleware/db"
	commonRedis "common/middleware/redis"
	"common/middleware/vipper"
	"service/argus_config/repository"

	goRedis "github.com/go-redis/redis"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Snapshot is the complete, encrypted-at-rest Argus configuration payload.
// It is only read by Argus and never exposed through management APIs.
type Snapshot struct {
	Version        repository.ArgusConfigVersion     `json:"version"`
	Config         repository.ArgusConfig            `json:"config"`
	Accounts       []*repository.ArgusAccount        `json:"accounts"`
	AccountRisks   []*repository.ArgusAccountRisk    `json:"accountRisks"`
	MonitorSymbols []*repository.ArgusMonitorSymbol  `json:"monitorSymbols"`
	Notification   repository.ArgusNotification      `json:"notification"`
	Sessions       []*repository.ArgusRuntimeSession `json:"sessions"`
}

type RuntimeConfig struct {
	Version      uint64
	Checksum     string
	Trade        *trade.TradingSystemConfig
	Symbols      map[string]monitor.SymbolConfig
	ServerPort   uint16
	RequestPath  string
	LogDir       string
	Notification notificationConfig
}

type notificationConfig struct {
	enabled bool
	token   string
	chatID  string
}

type Manager struct {
	mu             sync.Mutex
	current        RuntimeConfig
	cancel         context.CancelFunc
	started        bool
	instanceID     string
	reloadObserver func(version uint64, err error)
}

func Initialize(ctx context.Context) (*Manager, RuntimeConfig, error) {
	if db.Db == nil {
		return nil, RuntimeConfig{}, fmt.Errorf("argus configuration database is not initialized")
	}
	if _, err := commonRedis.GetContext(ctx, commonRedis.ArgusConfigVersionKey); err != nil && !errors.Is(err, goRedis.Nil) {
		return nil, RuntimeConfig{}, fmt.Errorf("read argus redis configuration: %w", err)
	}

	runtime, err := loadCurrent(ctx)
	if err != nil {
		return nil, RuntimeConfig{}, err
	}
	instanceID := vipper.GetString("argus.instance.id")
	if instanceID == "" {
		instanceID = "default"
	}
	return &Manager{current: runtime, instanceID: instanceID}, runtime, nil
}

func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.started {
		return nil
	}
	childCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.started = true
	go m.subscribe(childCtx)
	return nil
}

func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	m.started = false
}

func (m *Manager) Current() RuntimeConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.current
}

func (m *Manager) SetReloadObserver(observer func(version uint64, err error)) {
	m.mu.Lock()
	m.reloadObserver = observer
	m.mu.Unlock()
}

func (m *Manager) subscribe(ctx context.Context) {
	pubsub, err := commonRedis.SubscribeContext(ctx, commonRedis.ArgusConfigChannel, commonRedis.ArgusControlChannel)
	if err != nil {
		logrus.Errorf("Argus 配置订阅启动失败: %v", err)
		return
	}
	defer pubsub.Close()
	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-channel:
			if !ok {
				return
			}
			switch message.Channel {
			case commonRedis.ArgusConfigChannel:
				m.handleVersionMessage(ctx, message.Payload)
			case commonRedis.ArgusControlChannel:
				m.handleControlMessage(ctx, message.Payload)
			}
		}
	}
}

func (m *Manager) handleVersionMessage(ctx context.Context, payload string) {
	var message commonRedis.ConfigVersionMessage
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		logrus.Errorf("忽略无效 Argus 配置版本消息: %v", err)
		m.notifyReload(0, err)
		return
	}
	next, err := loadCurrent(ctx)
	if err != nil {
		logrus.Errorf("Argus 新配置校验失败，继续使用当前配置: %v", err)
		m.notifyReload(0, err)
		return
	}
	if next.Version != message.Version || next.Checksum != message.Checksum {
		err := fmt.Errorf("config version message does not match snapshot: message=%d/%s snapshot=%d/%s", message.Version, message.Checksum, next.Version, next.Checksum)
		logrus.Error(err)
		m.notifyReload(next.Version, err)
		return
	}

	m.mu.Lock()
	current := m.current
	if current.Version == next.Version && current.Checksum == next.Checksum {
		m.mu.Unlock()
		m.notifyReload(next.Version, nil)
		return
	}
	m.mu.Unlock()

	if err := m.apply(next, current); err != nil {
		logrus.Errorf("Argus 配置热加载失败，继续使用当前配置: %v", err)
		return
	}
}

func (m *Manager) handleControlMessage(ctx context.Context, payload string) {
	var message commonRedis.ArgusControlMessage
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		logrus.Errorf("忽略无效 Argus 控制消息: %v", err)
		m.notifyReload(0, err)
		return
	}
	if message.Action != "reload" {
		logrus.Warnf("忽略不支持的 Argus 控制动作: %s", message.Action)
		return
	}
	if message.InstanceID != "" && message.InstanceID != m.instanceID {
		return
	}
	if err := m.Reload(ctx); err != nil {
		logrus.Errorf("Argus 主动热加载失败，继续使用当前配置: %v", err)
	}
}

func (m *Manager) Reload(ctx context.Context) error {
	next, err := loadCurrent(ctx)
	if err != nil {
		m.notifyReload(0, err)
		return err
	}
	m.mu.Lock()
	current := m.current
	m.mu.Unlock()
	return m.apply(next, current)
}

func (m *Manager) apply(next, current RuntimeConfig) error {
	if err := applyRuntimeConfig(next, current); err != nil {
		m.notifyReload(next.Version, err)
		return err
	}
	m.mu.Lock()
	m.current = next
	m.mu.Unlock()
	m.notifyReload(next.Version, nil)
	logrus.Infof("Argus 配置已热加载: version=%d", next.Version)
	return nil
}

func (m *Manager) notifyReload(version uint64, err error) {
	m.mu.Lock()
	observer := m.reloadObserver
	m.mu.Unlock()
	if observer != nil {
		observer(version, err)
	}
}

func loadCurrent(ctx context.Context) (RuntimeConfig, error) {
	envelope, err := commonRedis.ReadConfigSnapshot(ctx)
	if err == nil {
		runtime, err := runtimeFromEnvelope(envelope)
		if err != nil {
			return RuntimeConfig{}, err
		}
		return runtime, nil
	}
	if !errors.Is(err, goRedis.Nil) {
		return RuntimeConfig{}, fmt.Errorf("read redis config snapshot: %w", err)
	}

	snapshot, err := loadPublishedSnapshot()
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("restore config snapshot from database: %w", err)
	}
	envelope, err = commonRedis.WriteConfigSnapshot(ctx, snapshot.Version.Version, snapshot, 24*time.Hour)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("restore redis config snapshot: %w", err)
	}
	return runtimeFromSnapshot(snapshot, envelope.Checksum)
}

func loadPublishedSnapshot() (Snapshot, error) {
	repo := db.GetRepository[repository.ArgusConfigRepository]()
	version, err := repo.FindPublished()
	if err != nil {
		return Snapshot{}, err
	}
	loadedVersion, config, accounts, risks, symbols, notification, sessions, err := repo.LoadSnapshot(uint64(version.Id))
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Version: *loadedVersion, Config: *config, Accounts: accounts, AccountRisks: risks, MonitorSymbols: symbols, Notification: *notification, Sessions: sessions}, nil
}

func runtimeFromEnvelope(envelope commonRedis.ConfigSnapshotEnvelope) (RuntimeConfig, error) {
	var snapshot Snapshot
	if err := json.Unmarshal(envelope.Payload, &snapshot); err != nil {
		return RuntimeConfig{}, fmt.Errorf("decode config snapshot payload: %w", err)
	}
	if snapshot.Version.Version != envelope.Version {
		return RuntimeConfig{}, fmt.Errorf("config snapshot version mismatch")
	}
	return runtimeFromSnapshot(snapshot, envelope.Checksum)
}

func runtimeFromSnapshot(snapshot Snapshot, checksum string) (RuntimeConfig, error) {
	if snapshot.Version.Version == 0 || snapshot.Config.MonitorIntervalSecond <= 0 || len(snapshot.Accounts) == 0 || len(snapshot.MonitorSymbols) == 0 {
		return RuntimeConfig{}, fmt.Errorf("config snapshot is incomplete")
	}

	riskByAccount := make(map[uint64]*repository.ArgusAccountRisk, len(snapshot.AccountRisks))
	for _, risk := range snapshot.AccountRisks {
		riskByAccount[risk.AccountID] = risk
	}
	sessionByAccount := make(map[uint64]*repository.ArgusRuntimeSession, len(snapshot.Sessions))
	for _, session := range snapshot.Sessions {
		sessionByAccount[session.AccountID] = session
	}

	tradeConfig := &trade.TradingSystemConfig{Trade: trade.TradeConfig{OrderSize: snapshot.Config.DefaultOrderSize}}
	if tradeConfig.Trade.OrderSize <= 0 {
		tradeConfig.Trade.OrderSize = 1
	}
	for index, account := range snapshot.Accounts {
		if account.Enabled == 0 {
			continue
		}
		risk := riskByAccount[uint64(account.Id)]
		if risk == nil {
			return RuntimeConfig{}, fmt.Errorf("account %s has no risk configuration", account.AccountName)
		}
		converted, err := runtimeAccount(account, risk, sessionByAccount[uint64(account.Id)], index+1)
		if err != nil {
			return RuntimeConfig{}, err
		}
		tradeConfig.Accounts = append(tradeConfig.Accounts, converted)
	}
	if len(tradeConfig.Accounts) == 0 {
		return RuntimeConfig{}, fmt.Errorf("config snapshot has no enabled accounts")
	}

	symbols := make(map[string]monitor.SymbolConfig, len(snapshot.MonitorSymbols))
	for _, symbol := range snapshot.MonitorSymbols {
		if symbol.Enabled == 0 || strings.TrimSpace(symbol.Symbol) == "" || strings.TrimSpace(symbol.DeepInstrument) == "" || strings.TrimSpace(symbol.TradeInstrument) == "" || symbol.SpreadThreshold <= 0 {
			continue
		}
		signalThreshold := symbol.SignalThreshold
		if signalThreshold <= 0 {
			signalThreshold = 0.0005
		}
		symbols[symbol.Symbol] = monitor.SymbolConfig{DeepInst: symbol.DeepInstrument, TradeInst: symbol.TradeInstrument, Threshold: symbol.SpreadThreshold, SignalThreshold: signalThreshold}
	}
	if len(symbols) == 0 {
		return RuntimeConfig{}, fmt.Errorf("config snapshot has no enabled monitor symbols")
	}

	notification, err := decryptNotification(snapshot.Notification)
	if err != nil {
		return RuntimeConfig{}, err
	}
	return RuntimeConfig{Version: snapshot.Version.Version, Checksum: checksum, Trade: tradeConfig, Symbols: symbols, ServerPort: snapshot.Config.ServerPort, RequestPath: snapshot.Config.RequestPath, LogDir: snapshot.Config.LogDir, Notification: notification}, nil
}

func runtimeAccount(account *repository.ArgusAccount, risk *repository.ArgusAccountRisk, session *repository.ArgusRuntimeSession, index int) (trade.AccountConfig, error) {
	apiKey, err := decrypt(account.APIKey)
	if err != nil {
		return trade.AccountConfig{}, fmt.Errorf("decrypt api key for %s: %w", account.AccountName, err)
	}
	secretKey, err := decrypt(account.SecretKey)
	if err != nil {
		return trade.AccountConfig{}, fmt.Errorf("decrypt secret key for %s: %w", account.AccountName, err)
	}
	passphrase, err := decrypt(account.Passphrase)
	if err != nil {
		return trade.AccountConfig{}, fmt.Errorf("decrypt passphrase for %s: %w", account.AccountName, err)
	}
	username, err := decrypt(account.Username)
	if err != nil {
		return trade.AccountConfig{}, err
	}
	password, err := decrypt(account.Password)
	if err != nil {
		return trade.AccountConfig{}, err
	}
	googleAuthKey, err := decrypt(account.GoogleAuthKey)
	if err != nil {
		return trade.AccountConfig{}, err
	}
	result := trade.AccountConfig{Name: account.AccountName, URL: account.URL, UID: account.UID, LoginType: account.LoginType, LoginHeadless: account.LoginHeadless == 1, Username: username, Password: password, GoogleAuthKey: googleAuthKey, APIKey: apiKey, SecretKey: secretKey, Passphrase: passphrase, PositionMode: account.PositionMode, PositionSide: account.PositionSide, CloseStrategy: account.CloseStrategy, InitialBalance: account.InitialBalance, Index: index, TPMode: risk.TakeProfitMode, ReverseGate: "off", RiskBudget: risk.RiskBudget, CatastrophicStopLoss: risk.CatastrophicStopLoss, MaxContracts: risk.MaxContracts}
	if risk.ReverseGateEnabled == 1 {
		result.ReverseGate = "on"
	}
	if session != nil {
		if result.Cookie, err = decrypt(session.Cookie); err != nil {
			return trade.AccountConfig{}, err
		}
		if result.Token, err = decrypt(session.OToken); err != nil {
			return trade.AccountConfig{}, err
		}
		if result.Token == "" {
			if result.Token, err = decrypt(session.Token); err != nil {
				return trade.AccountConfig{}, err
			}
		}
		if result.SentryRelease, err = decrypt(session.SentryRelease); err != nil {
			return trade.AccountConfig{}, err
		}
		if result.SentryPublicKey, err = decrypt(session.SentryPublicKey); err != nil {
			return trade.AccountConfig{}, err
		}
		result.LoginURL = session.LoginURL
	}
	if strings.TrimSpace(result.URL) == "" || strings.TrimSpace(result.APIKey) == "" || strings.TrimSpace(result.SecretKey) == "" || strings.TrimSpace(result.Passphrase) == "" {
		return trade.AccountConfig{}, fmt.Errorf("account %s has incomplete trade credentials", account.AccountName)
	}
	return result, nil
}

func decrypt(value repository.EncryptedString) (string, error) { return value.Decrypt() }

func decryptNotification(notification repository.ArgusNotification) (notificationConfig, error) {
	if notification.TelegramEnabled == 0 {
		return notificationConfig{}, nil
	}
	token, err := decrypt(notification.TelegramBotToken)
	if err != nil {
		return notificationConfig{}, err
	}
	chatID, err := decrypt(notification.TelegramChatID)
	if err != nil {
		return notificationConfig{}, err
	}
	if token == "" || chatID == "" {
		return notificationConfig{}, fmt.Errorf("telegram notification is enabled without credentials")
	}
	return notificationConfig{enabled: true, token: token, chatID: chatID}, nil
}

func restartRequired(next, current RuntimeConfig) []string {
	fields := make([]string, 0, 3)
	if next.ServerPort != current.ServerPort {
		fields = append(fields, "serverPort")
	}
	if next.RequestPath != current.RequestPath {
		fields = append(fields, "requestPath")
	}
	if next.LogDir != current.LogDir {
		fields = append(fields, "logDir")
	}
	return fields
}

func applyRuntimeConfig(next, current RuntimeConfig) error {
	if next.Trade == nil || len(next.Symbols) == 0 {
		return fmt.Errorf("runtime configuration is incomplete")
	}
	trade.ReplaceManager(next.Trade)
	monitor.ReloadMonitor(next.Symbols)
	if next.Notification.enabled {
		vipper.Set("telegram.bot_token", next.Notification.token)
		vipper.Set("telegram.chat_id", next.Notification.chatID)
	} else {
		vipper.Set("telegram.bot_token", "")
		vipper.Set("telegram.chat_id", "")
	}
	if next.Notification != current.Notification {
		monitor.ReloadTelegramBot()
	}
	if fields := restartRequired(next, current); len(fields) > 0 {
		logrus.Warnf("Argus 配置字段需要重启后生效: %s", strings.Join(fields, ", "))
	}
	return nil
}

func ApplyInitial(runtime RuntimeConfig) error {
	if runtime.Trade == nil {
		return fmt.Errorf("runtime trade configuration is nil")
	}
	if runtime.Notification.enabled {
		vipper.Set("telegram.bot_token", runtime.Notification.token)
		vipper.Set("telegram.chat_id", runtime.Notification.chatID)
	} else {
		vipper.Set("telegram.bot_token", "")
		vipper.Set("telegram.chat_id", "")
	}
	if runtime.ServerPort != 0 {
		vipper.Set("server.port", runtime.ServerPort)
	}
	if strings.TrimSpace(runtime.RequestPath) != "" {
		vipper.Set("request.path", runtime.RequestPath)
	}
	if strings.TrimSpace(runtime.LogDir) != "" {
		vipper.Set("log.dir", runtime.LogDir)
	}
	trade.InitTradeManager(runtime.Trade)
	return nil
}

func InstallSessionWriteBack() {
	trade.SetSessionSaveHook(func(account trade.AccountConfig, session trade.SessionAccountData) error {
		return persistSession(context.Background(), account, session)
	})
}

func persistSession(ctx context.Context, account trade.AccountConfig, session trade.SessionAccountData) error {
	if db.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	var version repository.ArgusConfigVersion
	if err := db.Db.Where("published_slot = ? AND active = ?", 1, 1).First(&version).Error; err != nil {
		return err
	}
	var storedAccount repository.ArgusAccount
	if err := db.Db.Where("config_version_id = ? AND account_name = ? AND active = ?", version.Id, account.Name, 1).First(&storedAccount).Error; err != nil {
		return err
	}
	updatedAt, err := time.Parse(time.RFC3339, session.UpdatedAt)
	if err != nil {
		updatedAt = time.Now().UTC()
	}
	value := repository.ArgusRuntimeSession{AccountID: uint64(storedAccount.Id), Cookie: repository.NewEncryptedString(session.Cookie), Token: repository.NewEncryptedString(session.Token), OToken: repository.NewEncryptedString(session.OToken), SentryRelease: repository.NewEncryptedString(session.SentryRelease), SentryPublicKey: repository.NewEncryptedString(session.SentryPublicKey), Baggage: repository.NewEncryptedString(session.Baggage), LoginURL: session.LoginURL, FinalURL: session.FinalURL, Valid: 1, SessionUpdatedAt: updatedAt}
	if err := db.Db.Transaction(func(tx *gorm.DB) error {
		var existing repository.ArgusRuntimeSession
		err := tx.Where("account_id = ? AND active = ?", storedAccount.Id, 1).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&value).Error
		}
		if err != nil {
			return err
		}
		value.Id = existing.Id
		return tx.Save(&value).Error
	}); err != nil {
		return err
	}
	snapshot, err := loadPublishedSnapshot()
	if err != nil {
		return err
	}
	envelope, err := commonRedis.WriteConfigSnapshot(ctx, snapshot.Version.Version, snapshot, 24*time.Hour)
	if err != nil {
		return err
	}
	return commonRedis.PublishConfigVersion(ctx, envelope.Version, envelope.Checksum)
}
