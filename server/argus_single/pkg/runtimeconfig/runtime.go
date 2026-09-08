package runtimeconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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
	// Tuning 是直接被 vipper.GetX 读取的全局标量参数，Overrides 是 AccFloat
	// 的账户级/全局覆盖层。两者一起构成「DB 是唯一真源、properties 只兜底」。
	Tuning    RuntimeTuning
	Overrides trade.ParamOverrides
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
	pollInterval   time.Duration
	reloadObserver func(version uint64, err error)
}

// defaultVersionPollInterval 是配置版本兜底轮询周期。
// pub/sub 不保证送达：订阅断线重连的窗口内发布、Redis 主从切换、客户端缓冲区
// 溢出都会丢消息，结果是「发布成功、程序照跑旧参数、页面和日志都不报错」。
// 30 秒来自需求的生效口径——发布后 15 秒内应显示已生效，一个轮询周期内必须补上。
const defaultVersionPollInterval = 30 * time.Second

func Initialize(ctx context.Context) (*Manager, RuntimeConfig, error) {
	if db.Db == nil {
		return nil, RuntimeConfig{}, fmt.Errorf("argus configuration database is not initialized")
	}
	// 实例键决定所有配置读写的命名空间，必须在读 Redis / DB 之前确定。
	instanceID := strings.TrimSpace(vipper.GetString("argus.instance.id"))
	if instanceID == "" {
		return nil, RuntimeConfig{}, fmt.Errorf("argus.instance.id is required to resolve the instance configuration namespace")
	}
	if _, err := commonRedis.GetContext(ctx, commonRedis.ArgusConfigVersionKey(instanceID)); err != nil && !errors.Is(err, goRedis.Nil) {
		return nil, RuntimeConfig{}, fmt.Errorf("read argus redis configuration: %w", err)
	}

	runtime, err := loadCurrent(ctx, instanceID)
	if err != nil {
		return nil, RuntimeConfig{}, err
	}
	return &Manager{current: runtime, instanceID: instanceID, pollInterval: defaultVersionPollInterval}, runtime, nil
}

// InstanceID 返回本进程绑定的实例键。
func (m *Manager) InstanceID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.instanceID
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
	go m.pollPublishedVersion(childCtx)
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
	// 版本频道是三实例共用的，不属于本实例的消息直接丢弃；空实例键视为历史消息放行。
	if message.InstanceID != "" && message.InstanceID != m.instanceID {
		return
	}
	next, err := loadCurrent(ctx, m.instanceID)
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

// pollPublishedVersion 是 pub/sub 的兜底通道。
//
// argus:config:changed 是即发即弃的：订阅断线重连的窗口内发布、Redis 主从切换、
// 客户端缓冲区溢出，任何一种都会让消息静静丢掉，结果是发布成功、程序继续跑旧参数、
// 日志和页面都不报错——这是配置面最危险的失效方式，因为它看起来一切正常。
func (m *Manager) pollPublishedVersion(ctx context.Context) {
	interval := m.pollInterval
	if interval <= 0 {
		interval = defaultVersionPollInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.reconcilePublishedVersion(ctx)
		}
	}
}

// reconcilePublishedVersion 比对已发布版本与运行中版本，落后就补一次热加载。
func (m *Manager) reconcilePublishedVersion(ctx context.Context) {
	published, err := m.publishedVersion(ctx)
	if err != nil {
		logrus.Warnf("Argus 配置版本兜底轮询失败，等待下一轮: %v", err)
		return
	}
	current := m.Current()
	if published == 0 || published == current.Version {
		return
	}
	next, err := loadCurrent(ctx, m.InstanceID())
	if err != nil {
		logrus.Errorf("Argus 配置兜底轮询加载快照失败，继续使用当前配置: %v", err)
		m.notifyReload(0, err)
		return
	}
	if next.Version == current.Version && next.Checksum == current.Checksum {
		// 版本键已经往前走了但快照还是旧的（发布方写键与写快照之间的窗口）。
		// 这里不重载，避免每 30 秒空转一次 ReplaceManager。
		logrus.Warnf("Argus 已发布版本 %d 与可读快照版本 %d 不一致，本轮不重载", published, next.Version)
		return
	}
	logrus.Warnf("Argus 配置版本落后（运行中=%d 已发布=%d），pub/sub 可能漏消息，执行兜底热加载", current.Version, published)
	if err := m.apply(next, current); err != nil {
		logrus.Errorf("Argus 配置兜底热加载失败，继续使用当前配置: %v", err)
	}
}

// publishedVersion 先读 Redis 的实例版本键；Redis 不可用或键缺失时回落查库的
// published 版本，两条路都断才算失败——兜底通道本身不能依赖单点。
func (m *Manager) publishedVersion(ctx context.Context) (uint64, error) {
	instanceKey := m.InstanceID()
	if strings.TrimSpace(instanceKey) == "" {
		return 0, commonRedis.ErrInstanceKeyRequired
	}
	raw, err := commonRedis.GetContext(ctx, commonRedis.ArgusConfigVersionKey(instanceKey))
	switch {
	case err == nil:
		version, parseErr := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
		if parseErr == nil && version > 0 {
			return version, nil
		}
		logrus.Warnf("Argus 配置版本键内容非法(%q)，回落查库", raw)
	case errors.Is(err, goRedis.Nil):
		// 键过期或尚未写入，属于正常降级，直接查库。
	default:
		logrus.Warnf("Argus 配置版本键读取失败，回落查库: %v", err)
	}
	if db.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	repo := db.GetRepository[repository.ArgusConfigRepository]()
	version, err := repo.FindPublishedContext(ctx, instanceKey)
	if err != nil {
		return 0, err
	}
	return version.Version, nil
}

func (m *Manager) Reload(ctx context.Context) error {
	next, err := loadCurrent(ctx, m.instanceID)
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

func loadCurrent(ctx context.Context, instanceKey string) (RuntimeConfig, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return RuntimeConfig{}, commonRedis.ErrInstanceKeyRequired
	}
	envelope, err := commonRedis.ReadConfigSnapshot(ctx, instanceKey)
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

	snapshot, err := loadPublishedSnapshot(instanceKey)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("restore config snapshot from database: %w", err)
	}
	envelope, err = commonRedis.WriteConfigSnapshot(ctx, instanceKey, snapshot.Version.Version, snapshot, 24*time.Hour)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("restore redis config snapshot: %w", err)
	}
	return runtimeFromSnapshot(snapshot, envelope.Checksum)
}

func loadPublishedSnapshot(instanceKey string) (Snapshot, error) {
	repo := db.GetRepository[repository.ArgusConfigRepository]()
	version, err := repo.FindPublished(instanceKey)
	if err != nil {
		return Snapshot{}, err
	}
	loadedVersion, config, accounts, risks, symbols, notification, sessions, err := repo.LoadSnapshot(instanceKey, uint64(version.Id))
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
	overrides := trade.ParamOverrides{}
	globalParamOverrides(snapshot.Config, &overrides)
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
		accountParamOverrides(index+1, risk, account.AccountName, &overrides)
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
	return RuntimeConfig{Version: snapshot.Version.Version, Checksum: checksum, Trade: tradeConfig, Symbols: symbols, ServerPort: snapshot.Config.ServerPort, RequestPath: snapshot.Config.RequestPath, LogDir: snapshot.Config.LogDir, Notification: notification, Tuning: snapshotTuning(snapshot.Config), Overrides: overrides}, nil
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
	// extra_risk_json 里的三项决定走哪套策略与事件归因。解析失败必须报错而不是
	// 静默继续：TradeLogic 停在空串会被归一化成 spread，盘口信号策略整套不走，
	// 且没有任何错误可见。宁可拒绝这份快照、继续跑当前配置。
	extra, err := parseExtraRisk(risk.ExtraRiskJSON, account.AccountName)
	if err != nil {
		return trade.AccountConfig{}, fmt.Errorf("decode extra risk for %s: %w", account.AccountName, err)
	}
	result := trade.AccountConfig{Name: account.AccountName, URL: account.URL, UID: account.UID, LoginType: account.LoginType, LoginHeadless: account.LoginHeadless == 1, Username: username, Password: password, GoogleAuthKey: googleAuthKey, APIKey: apiKey, SecretKey: secretKey, Passphrase: passphrase, PositionMode: runtimePositionMode(account.PositionMode), PositionSide: account.PositionSide, CloseStrategy: account.CloseStrategy, InitialBalance: account.InitialBalance, Index: index, TPMode: risk.TakeProfitMode, StopLossMode: risk.StopLossMode, ReverseGate: "off", RiskBudget: risk.RiskBudget, CatastrophicStopLoss: risk.CatastrophicStopLoss, MaxContracts: risk.MaxContracts, OrderSize: risk.OrderSize, TradeDirection: extra.TradeDirection, TradeLogic: extra.TradeLogic, Variant: extra.Variant}
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
		if result.Baggage, err = decrypt(session.Baggage); err != nil {
			return trade.AccountConfig{}, err
		}
		result.LoginURL = session.LoginURL
	}
	if strings.TrimSpace(result.URL) == "" || strings.TrimSpace(result.APIKey) == "" || strings.TrimSpace(result.SecretKey) == "" || strings.TrimSpace(result.Passphrase) == "" {
		return trade.AccountConfig{}, fmt.Errorf("account %s has incomplete trade credentials", account.AccountName)
	}
	// 与 properties 路径共用同一套默认值与归一化，避免两条加载路径行为分叉。
	trade.NormalizeAccountConfig(&result)
	return result, nil
}

// runtimePositionMode 把 DB 的持仓模式枚举翻回运行时口径。
// argus_account.position_mode 的 CHECK 约束只收 net/hedge，而 properties 与
// pkg/trade 用的是 bidirectional；不翻译的话双向持仓账户会被当成非 net，
// 走进「信号逻辑强制改 net」的告警分支。
func runtimePositionMode(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "hedge") {
		return "bidirectional"
	}
	return value
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

// hotApplied 列出本次热加载真正改变了运行时行为的面。它与 restartRequired 是
// 一对：能热生效的一律立刻生效，只有 serverPort/requestPath/logDir 三项做不到
// （端口和路由在 gin 启动时就绑死，日志目录在 eventlog.Init 时就打开了文件）。
func hotApplied(next, current RuntimeConfig) []string {
	fields := make([]string, 0, 4)
	if current.Trade == nil || len(next.Trade.Accounts) != len(current.Trade.Accounts) {
		fields = append(fields, "accounts")
	}
	if next.Tuning != current.Tuning {
		fields = append(fields, "tuning")
	}
	if next.Notification != current.Notification {
		fields = append(fields, "notification")
	}
	if len(next.Symbols) != len(current.Symbols) {
		fields = append(fields, "symbols")
	}
	return fields
}

func applyRuntimeConfig(next, current RuntimeConfig) error {
	if next.Trade == nil || len(next.Symbols) == 0 {
		return fmt.Errorf("runtime configuration is incomplete")
	}
	// 顺序有强约束：覆盖层与全局标量必须先落地，NewTradeManager 与
	// NewPriceMonitor 都在构造时把参数读走，晚一步就是「换了配置跑旧参数」。
	trade.SetParamOverrides(next.Overrides)
	applyTuning(next.Tuning)
	// 预校验必须在覆盖层装好之后、ReplaceManager 之前：NewTradeManager 校验不过
	// 是 logrus.Fatalf，一次误发布会打死正在扛仓的进程。这里先用可返回错误的
	// 校验挡一道，失败就把覆盖层与全局标量回滚回当前配置，继续跑旧参数。
	if err := trade.ValidateAccounts(next.Trade); err != nil {
		trade.SetParamOverrides(current.Overrides)
		applyTuning(current.Tuning)
		return fmt.Errorf("reject config version %d: %w", next.Version, err)
	}
	trade.ReplaceManager(next.Trade)
	monitor.ReloadMonitor(next.Symbols)
	// 账户监控器是 sync.Once 单例，热替换会丢掉 trail 峰值与告警冷却，只能就地改参数。
	monitor.ApplyAccountMonitorParams(next.Tuning.MonitorIntervalSecond, next.Tuning.ProfitThreshold, next.Tuning.LossThreshold)
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
	if fields := hotApplied(next, current); len(fields) > 0 {
		logrus.Infof("Argus 配置已热生效: %s", strings.Join(fields, ", "))
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
	// 必须早于 InitTradeManager / InitMonitor / InitAccountMonitor：三者都在
	// 构造时读参数。setTuningValue 也在这里第一次抓 properties 兜底值，
	// 之后任何 vipper.Set 都不会污染它。
	trade.SetParamOverrides(runtime.Overrides)
	applyTuning(runtime.Tuning)
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

// InstallSessionWriteBack 把会话回写钉在本实例上，避免刷新到别的实例的版本。
func (m *Manager) InstallSessionWriteBack() {
	instanceKey := m.InstanceID()
	trade.SetSessionSaveHook(func(account trade.AccountConfig, session trade.SessionAccountData) error {
		return persistSession(context.Background(), instanceKey, account, session)
	})
}

func persistSession(ctx context.Context, instanceKey string, account trade.AccountConfig, session trade.SessionAccountData) error {
	if strings.TrimSpace(instanceKey) == "" {
		return commonRedis.ErrInstanceKeyRequired
	}
	if db.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	var version repository.ArgusConfigVersion
	if err := db.Db.Where("instance_key = ? AND published_slot = ? AND active = ?", instanceKey, 1, 1).First(&version).Error; err != nil {
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
	snapshot, err := loadPublishedSnapshot(instanceKey)
	if err != nil {
		return err
	}
	envelope, err := commonRedis.WriteConfigSnapshot(ctx, instanceKey, snapshot.Version.Version, snapshot, 24*time.Hour)
	if err != nil {
		return err
	}
	return commonRedis.PublishConfigVersion(ctx, instanceKey, envelope.Version, envelope.Checksum)
}
