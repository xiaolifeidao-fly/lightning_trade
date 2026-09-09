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
	Version uint64
	// Checksum 是 argus_config_version.snapshot_checksum，即"发布那一刻的快照
	// 校验和"。它回答的是「我在跑哪一个已发布版本」，经心跳上报，管理端
	// overview.go 用它与库里的值比对判漂移——语义不能改。
	Checksum string
	// Fingerprint 是"派生后运行配置"的内容指纹，只用于变更检测，不上报。
	// 与 Checksum 的分工：运维直接改库（每周轮换 cookie/token）不会动
	// Checksum 与 Version，只有 Fingerprint 会变。见 fingerprint.go。
	Fingerprint  string
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

// defaultVersionPollInterval 是配置比对周期。
//
// 它不再是"兜底"而是**主要准据**：配置只从数据库读，每轮重新派生运行配置并比对
// 内容指纹，不同就热加载。Redis 通知只是"提前触发一次比对"的加速信号。
// 60 秒的含义是"运维直接改库后最坏 60 秒生效"；一轮约 7 条查询，三实例
// 合计约 21 次/分钟，对 RDS 可忽略。
// pub/sub 不保证送达：订阅断线重连的窗口内发布、Redis 主从切换、客户端缓冲区
// 溢出都会丢消息，结果是「发布成功、程序照跑旧参数、页面和日志都不报错」。
// 30 秒来自需求的生效口径——发布后 15 秒内应显示已生效，一个轮询周期内必须补上。
const defaultVersionPollInterval = 60 * time.Second

func Initialize(ctx context.Context) (*Manager, RuntimeConfig, error) {
	if db.Db == nil {
		return nil, RuntimeConfig{}, fmt.Errorf("argus configuration database is not initialized")
	}
	// 实例键决定所有配置读写的命名空间，必须在读 Redis / DB 之前确定。
	instanceID := strings.TrimSpace(vipper.GetString("argus.instance.id"))
	if instanceID == "" {
		return nil, RuntimeConfig{}, fmt.Errorf("argus.instance.id is required to resolve the instance configuration namespace")
	}
	// 这里不再预检 Redis 的配置键：配置只从数据库读（见需求大纲 §决策记录：
	// 配置不经 Redis 缓存）。Redis 仍用于心跳与控制/通知通道，其可用性由
	// InitRedisClient 在更早的初始化阶段负责，不可用时那些能力自行降级。
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
	go m.pollConfig(childCtx)
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

// ownsMessage 判断一条广播是否属于本实例。
//
// 版本频道是三实例共用的：不做过滤会让一次发布把三个实例一起触发，而它们的
// 参数本来就不同（r1 分域改造的起点）。空实例键视为历史消息放行——分域之前
// 发布的消息没有这个字段。
//
// 提成独立方法是为了可直接断言：这条不变量以前只能靠"观察 reload 回调有没有
// 被触发"间接测，而回调是否触发依赖 DB 可用性，与实例隔离本身无关。
func (m *Manager) ownsMessage(instanceID string) bool {
	return instanceID == "" || instanceID == m.instanceID
}

func (m *Manager) handleVersionMessage(ctx context.Context, payload string) {
	var message commonRedis.ConfigVersionMessage
	if err := json.Unmarshal([]byte(payload), &message); err != nil {
		logrus.Errorf("忽略无效 Argus 配置版本消息: %v", err)
		m.notifyReload(0, err)
		return
	}
	if !m.ownsMessage(message.InstanceID) {
		return
	}
	// 通知只是"现在去查一次库"的信号，不携带正确性责任：消息里的
	// version/checksum 不再用于校验（数据库才是唯一来源，消息可能过期或丢失），
	// 走与定时轮询完全相同的一条路径，避免两条链路行为分叉。
	logrus.Infof("收到 Argus 配置变更通知（version=%d），立即比对数据库", message.Version)
	m.reconcileConfig(ctx)
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
func (m *Manager) pollConfig(ctx context.Context) {
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
			m.reconcileConfig(ctx)
		}
	}
}

// reconcileConfig 从数据库重新派生运行配置，与当前生效配置比对内容指纹，
// 不同则热加载。定时轮询与 Redis 通知都走这一条路径。
//
// 为什么比指纹而不是比版本号：
//   - 运维直接改库（每周轮换 cookie/token 是常规操作）不会改动 version，
//     只比版本号等于这条通道形同虚设；
//   - 回滚场景下同一版本号会被重新发布，版本号相同而内容不同。
//
// 为什么指纹对"派生后的运行配置"求取而不是对数据库原始行：原始行含
// updated_time / session_updated_at 等审计列，而本进程自己的会话回写
// （InstallSessionWriteBack）就会改动它们。对原始行求指纹会让进程每次回写会话
// 都把自己热替换一遍——ReplaceManager + ReloadMonitor 会停掉正在扛仓的交易
// 管理器与监控器，比参数不生效危险得多。见 fingerprint.go。
func (m *Manager) reconcileConfig(ctx context.Context) {
	next, err := loadCurrent(ctx, m.InstanceID())
	if err != nil {
		// 查库失败不改变现状：继续跑当前配置，等下一轮。不上报 reload 失败，
		// 否则一次网络抖动会在管理端留下"热加载失败"的假告警。
		logrus.Warnf("Argus 配置比对读库失败，继续使用当前配置，等待下一轮: %v", err)
		return
	}
	current := m.Current()
	if next.Fingerprint == current.Fingerprint {
		return
	}
	logrus.Warnf("Argus 检测到配置变更（运行中 version=%d fingerprint=%s… → 数据库 version=%d fingerprint=%s…），执行热加载",
		current.Version, shortFingerprint(current.Fingerprint), next.Version, shortFingerprint(next.Fingerprint))
	if err := m.apply(next, current); err != nil {
		logrus.Errorf("Argus 配置热加载失败，继续使用当前配置: %v", err)
	}
}

// shortFingerprint 只截前 12 位用于日志，完整 sha256 在日志里没有可读性。
func shortFingerprint(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:12]
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

// loadCurrent 从数据库读该实例的已发布配置并派生运行配置。
//
// 这里没有缓存：配置读取一天只有数次（启动 + 每次比对发现变更），而缓存换来的
// 是"改了库、没清缓存"这种静默失效——实例继续跑旧配置，日志照样打"已热加载"。
// 2026-09-08 的会话凭证故障就是这个形状。详见需求大纲 §决策记录。
//
// Checksum 取库里存的 snapshot_checksum（发布时算的），不重算：它要回答的是
// "我在跑哪一个已发布版本"，重算会让手改库之后与管理端的比对永远显示漂移。
func loadCurrent(ctx context.Context, instanceKey string) (RuntimeConfig, error) {
	if strings.TrimSpace(instanceKey) == "" {
		return RuntimeConfig{}, commonRedis.ErrInstanceKeyRequired
	}
	if err := ctx.Err(); err != nil {
		return RuntimeConfig{}, err
	}
	snapshot, err := loadPublishedSnapshot(instanceKey)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("load published config from database: %w", err)
	}
	return runtimeFromSnapshot(snapshot, snapshot.Version.SnapshotChecksum)
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

	notification, err := notificationFromSnapshot(snapshot.Notification)
	if err != nil {
		return RuntimeConfig{}, err
	}
	result := RuntimeConfig{Version: snapshot.Version.Version, Checksum: checksum, Trade: tradeConfig, Symbols: symbols, ServerPort: snapshot.Config.ServerPort, RequestPath: snapshot.Config.RequestPath, LogDir: snapshot.Config.LogDir, Notification: notification, Tuning: snapshotTuning(snapshot.Config), Overrides: overrides}
	// 指纹在这里一次算好：下游的比对全部只读 Fingerprint，不重复计算，
	// 避免"两处算法漂移导致每轮都判定为变更"这种自触发热替换。
	fingerprint, err := configFingerprint(result)
	if err != nil {
		return RuntimeConfig{}, err
	}
	result.Fingerprint = fingerprint
	return result, nil
}

func runtimeAccount(account *repository.ArgusAccount, risk *repository.ArgusAccountRisk, session *repository.ArgusRuntimeSession, index int) (trade.AccountConfig, error) {
	// 凭证明文存储（见需求大纲 §决策记录），这里直接取值。下面对
	// URL/APIKey/SecretKey/Passphrase 的非空校验仍在，缺项照样拒绝这份快照。
	apiKey := account.APIKey
	secretKey := account.SecretKey
	passphrase := account.Passphrase
	username := account.Username
	password := account.Password
	googleAuthKey := account.GoogleAuthKey
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
		result.Cookie = session.Cookie
		// OToken 优先，空则回退 Token——两者语义相同，历史上换过字段名。
		result.Token = session.OToken
		if result.Token == "" {
			result.Token = session.Token
		}
		result.SentryRelease = session.SentryRelease
		result.SentryPublicKey = session.SentryPublicKey
		result.Baggage = session.Baggage
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

// notificationFromSnapshot 取 Telegram 凭证。开了通知但凭证缺项仍然报错——
// 静默降级会让告警整条链路失效而没人知道。
func notificationFromSnapshot(notification repository.ArgusNotification) (notificationConfig, error) {
	if notification.TelegramEnabled == 0 {
		return notificationConfig{}, nil
	}
	token := notification.TelegramBotToken
	chatID := notification.TelegramChatID
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
	value := repository.ArgusRuntimeSession{AccountID: uint64(storedAccount.Id), Cookie: session.Cookie, Token: session.Token, OToken: session.OToken, SentryRelease: session.SentryRelease, SentryPublicKey: session.SentryPublicKey, Baggage: session.Baggage, LoginURL: session.LoginURL, FinalURL: session.FinalURL, Valid: 1, SessionUpdatedAt: updatedAt}
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
	// 写完库就结束：不再写 Redis 快照、也不再广播。
	//
	// 原来这里会 WriteConfigSnapshot + PublishConfigVersion，有两个问题：
	//  1. 管理端发布与本进程会话回写共享同一份 Redis 快照，两个写入方谁后写谁赢；
	//  2. 广播的是自己刚写的数据，其他实例并不关心，本实例收到后还要再走一遍
	//     加载流程。配置改为数据库唯一来源后，本进程下一轮指纹比对会自然发现
	//     这次会话变更（cookie/token 参与指纹），不需要任何额外通知。
	return nil
}
