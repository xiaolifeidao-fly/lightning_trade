package dto

import (
	"time"
)

// InstanceDTO 是部署实例注册表的响应结构。
type InstanceDTO struct {
	ID           uint64 `json:"id"`
	InstanceKey  string `json:"instanceKey"`
	InstanceName string `json:"instanceName"`
	Description  string `json:"description,omitempty"`
	ConfigSource string `json:"configSource,omitempty"`
	Enabled      uint8  `json:"enabled"`
}

// SaveInstanceRequest 用于注册或更新一个部署实例，InstanceKey 全局唯一。
type SaveInstanceRequest struct {
	InstanceKey  string `json:"instanceKey" binding:"required"`
	InstanceName string `json:"instanceName"`
	Description  string `json:"description"`
	ConfigSource string `json:"configSource"`
	Enabled      *uint8 `json:"enabled"`
}

type ConfigVersionDTO struct {
	ID               uint64     `json:"id"`
	InstanceKey      string     `json:"instanceKey"`
	Version          uint64     `json:"version"`
	Status           string     `json:"status"`
	ReleaseNote      string     `json:"releaseNote"`
	PublishedBy      string     `json:"publishedBy"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
	SnapshotChecksum string     `json:"snapshotChecksum"`
}

type ConfigDTO struct {
	ID                          uint64  `json:"id"`
	ServerPort                  uint16  `json:"serverPort"`
	RequestPath                 string  `json:"requestPath"`
	LogDir                      string  `json:"logDir"`
	Enabled                     uint8   `json:"enabled"`
	TradeEnabled                uint8   `json:"tradeEnabled"`
	DefaultOrderSize            int     `json:"defaultOrderSize"`
	MonitorIntervalSecond       int     `json:"monitorIntervalSecond"`
	ProfitThreshold             float64 `json:"profitThreshold"`
	LossThreshold               float64 `json:"lossThreshold"`
	AICloseEnabled              uint8   `json:"aiCloseEnabled"`
	AICloseProvider             string  `json:"aiCloseProvider"`
	AICloseAPIURL               string  `json:"aiCloseApiUrl"`
	AICloseAPIKey               string  `json:"aiCloseApiKey,omitempty"`
	AICloseModel                string  `json:"aiCloseModel"`
	AICloseTimeoutSecond        int     `json:"aiCloseTimeoutSecond"`
	AICloseMaxTokens            int     `json:"aiCloseMaxTokens"`
	AICloseTemperature          float64 `json:"aiCloseTemperature"`
	AICloseIntervalMinute       int     `json:"aiCloseIntervalMinute"`
	AICloseMinInterval          int     `json:"aiCloseMinInterval"`
	AICloseMaxInterval          int     `json:"aiCloseMaxInterval"`
	AIOpenEnabled               uint8   `json:"aiOpenEnabled"`
	AIOpenAutoTrade             uint8   `json:"aiOpenAutoTrade"`
	AIOpenAPIURL                string  `json:"aiOpenApiUrl"`
	AIOpenAPIKey                string  `json:"aiOpenApiKey,omitempty"`
	AIOpenModel                 string  `json:"aiOpenModel"`
	AIOpenTimeoutSecond         int     `json:"aiOpenTimeoutSecond"`
	AIOpenMaxTokens             int     `json:"aiOpenMaxTokens"`
	AIOpenTemperature           float64 `json:"aiOpenTemperature"`
	AIOpenIntervalMinute        int     `json:"aiOpenIntervalMinute"`
	AIOpenMinInterval           int     `json:"aiOpenMinInterval"`
	AIOpenMaxInterval           int     `json:"aiOpenMaxInterval"`
	AIOpenMinLiqDistancePercent float64 `json:"aiOpenMinLiqDistancePercent"`
	AIOpenMinLiqDistanceUSD     float64 `json:"aiOpenMinLiqDistanceUsd"`
	AIOpenMaxBalancePercent     float64 `json:"aiOpenMaxBalancePercent"`
	AIOpenMinOrderContracts     int     `json:"aiOpenMinOrderContracts"`
	AIOpenMaxOrderContracts     int     `json:"aiOpenMaxOrderContracts"`
	AIOpenMaxTotalContracts     int     `json:"aiOpenMaxTotalContracts"`
	AIOpenCooldownMinute        int     `json:"aiOpenCooldownMinute"`
	AIOpenLiqSafetyFactor       float64 `json:"aiOpenLiqSafetyFactor"`
	LoginScheduledEnabled       uint8   `json:"loginScheduledEnabled"`
	LoginScheduledHour          uint8   `json:"loginScheduledHour"`
	LoginScheduledMinute        uint8   `json:"loginScheduledMinute"`
	SessionMaxAgeDay            int     `json:"sessionMaxAgeDay"`
	ExtraConfigJSON             string  `json:"extraConfigJson,omitempty"`
	// r5 收敛进来的全局策略参数，0 表示未配置（运行时回退 properties）。
	ContractFace            float64 `json:"contractFace"`
	SignalDelaySecond       int     `json:"signalDelaySecond"`
	SpreadMaxPriceAgeMs     int     `json:"spreadMaxPriceAgeMs"`
	TrendGateWindowHour     float64 `json:"trendGateWindowHour"`
	TrendGateThresholdPct   float64 `json:"trendGateThresholdPct"`
	ReverseGateMinProfitPct float64 `json:"reverseGateMinProfitPct"`
}

type AccountDTO struct {
	ID             uint64  `json:"id"`
	AccountName    string  `json:"accountName"`
	Platform       string  `json:"platform,omitempty"`
	URL            string  `json:"url"`
	UID            string  `json:"uid"`
	LoginType      string  `json:"loginType"`
	LoginHeadless  uint8   `json:"loginHeadless"`
	Username       string  `json:"username"`
	Password       string  `json:"password,omitempty"`
	GoogleAuthKey  string  `json:"googleAuthKey,omitempty"`
	APIKey         string  `json:"apiKey,omitempty"`
	SecretKey      string  `json:"secretKey,omitempty"`
	Passphrase     string  `json:"passphrase,omitempty"`
	ResourceID     string  `json:"resourceId"`
	PositionMode   string  `json:"positionMode"`
	PositionSide   string  `json:"positionSide"`
	CloseStrategy  string  `json:"closeStrategy"`
	InitialBalance float64 `json:"initialBalance"`
	Enabled        uint8   `json:"enabled"`
}

type AccountRiskDTO struct {
	ID                    uint64  `json:"id"`
	AccountID             uint64  `json:"accountId"`
	TakeProfitMode        string  `json:"takeProfitMode"`
	StopLossMode          string  `json:"stopLossMode"`
	TrailingStopTiersJSON string  `json:"trailingStopTiersJson"`
	RiskBudget            float64 `json:"riskBudget"`
	CatastrophicStopLoss  float64 `json:"catastrophicStopLoss"`
	ReverseGateEnabled    uint8   `json:"reverseGateEnabled"`
	MaxContracts          int     `json:"maxContracts"`
	ExtraRiskJSON         string  `json:"extraRiskJson,omitempty"`
	// r5 收敛进来的账户级策略参数，0 表示未配置（运行时回退 properties）。
	OrderSize               int     `json:"orderSize"`
	RiskEquity              float64 `json:"riskEquity"`
	ReverseGateMinProfitPct float64 `json:"reverseGateMinProfitPct"`
	TrendGateThresholdPct   float64 `json:"trendGateThresholdPct"`
}

type MonitorSymbolDTO struct {
	ID              uint64  `json:"id"`
	Symbol          string  `json:"symbol"`
	DeepInstrument  string  `json:"deepInstrument"`
	TradeInstrument string  `json:"tradeInstrument"`
	SpreadThreshold float64 `json:"spreadThreshold"`
	SignalThreshold float64 `json:"signalThreshold"`
	Enabled         uint8   `json:"enabled"`
}

type NotificationDTO struct {
	ID               uint64 `json:"id"`
	TelegramEnabled  uint8  `json:"telegramEnabled"`
	TelegramBotToken string `json:"telegramBotToken,omitempty"`
	TelegramChatID   string `json:"telegramChatId,omitempty"`
}

type RuntimeSessionDTO struct {
	ID              uint64 `json:"id"`
	AccountID       uint64 `json:"accountId"`
	Cookie          string `json:"cookie,omitempty"`
	Token           string `json:"token,omitempty"`
	OToken          string `json:"otoken,omitempty"`
	SentryRelease   string `json:"sentryRelease,omitempty"`
	SentryPublicKey string `json:"sentryPublicKey,omitempty"`
	Baggage         string `json:"baggage,omitempty"`
	// CookieLength / TokenLength 只回明文长度，不回明文本身。会话卡片是只读巡检，
	// 运维要判断的是「有没有、是不是被截断了」，长度足够，回显 950 字符的 cookie
	// 只会扩大泄露面。
	CookieLength     int        `json:"cookieLength"`
	TokenLength      int        `json:"tokenLength"`
	OTokenLength     int        `json:"otokenLength"`
	LoginURL         string     `json:"loginUrl"`
	FinalURL         string     `json:"finalUrl"`
	Valid            uint8      `json:"valid"`
	SessionUpdatedAt time.Time  `json:"sessionUpdatedAt"`
	ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
	LastError        string     `json:"lastError,omitempty"`
}

type ConfigSnapshotDTO struct {
	InstanceKey    string              `json:"instanceKey"`
	Version        ConfigVersionDTO    `json:"version"`
	Config         ConfigDTO           `json:"config"`
	Accounts       []AccountDTO        `json:"accounts"`
	AccountRisks   []AccountRiskDTO    `json:"accountRisks"`
	MonitorSymbols []MonitorSymbolDTO  `json:"monitorSymbols"`
	Notification   NotificationDTO     `json:"notification"`
	Sessions       []RuntimeSessionDTO `json:"sessions"`
}

type SaveConfigRequest struct {
	// InstanceKey 指定草稿归属的部署实例；为空时由服务端按默认实例解析。
	InstanceKey    string              `json:"instanceKey"`
	Config         ConfigDTO           `json:"config" binding:"required"`
	Accounts       []AccountDTO        `json:"accounts"`
	AccountRisks   []AccountRiskDTO    `json:"accountRisks"`
	MonitorSymbols []MonitorSymbolDTO  `json:"monitorSymbols"`
	Notification   NotificationDTO     `json:"notification"`
	Sessions       []RuntimeSessionDTO `json:"sessions"`
	ReleaseNote    string              `json:"releaseNote"`
}

type PublishConfigRequest struct {
	// InstanceKey 指定发布目标实例；为空时由服务端按默认实例解析。
	InstanceKey string `json:"instanceKey"`
	ReleaseNote string `json:"releaseNote"`
}

// RollbackConfigRequest 回滚到某个已归档版本。回滚不新建版本，沿用该历史版本
// 自身的版本号与不可变快照。
type RollbackConfigRequest struct {
	InstanceKey string `json:"instanceKey"`
	ReleaseNote string `json:"releaseNote"`
}

// InstanceRuntimeDTO 是一个部署实例的「注册信息 + 已发布版本 + 心跳生效状态」三合一
// 视图，供 Argus 总览页与实例对比页横向铺开。
//
// 版本漂移的口径只有一个：**同一个实例**的已发布版本 vs 该实例心跳回报的版本。
// 版本号是实例内自增的，跨实例比大小没有意义（实例1 的 v37 与实例3 的 v36 是两条
// 独立序列），所以这里不给「谁比谁新」这类字段。
type InstanceRuntimeDTO struct {
	InstanceKey  string `json:"instanceKey"`
	InstanceName string `json:"instanceName"`
	Description  string `json:"description,omitempty"`
	ConfigSource string `json:"configSource,omitempty"`
	Enabled      uint8  `json:"enabled"`

	// PublishedVersion 为 0 表示该实例还没有已发布版本（只有草稿或全空）。
	PublishedVersion  uint64     `json:"publishedVersion"`
	PublishedChecksum string     `json:"publishedChecksum"`
	PublishedAt       *time.Time `json:"publishedAt,omitempty"`
	PublishedBy       string     `json:"publishedBy,omitempty"`

	// Online 以心跳键是否还在（TTL 15s）为准，不看进程列表——实例可能部署在别的机器上。
	Online              bool       `json:"online"`
	HeartbeatAgeSeconds *int       `json:"heartbeatAgeSeconds,omitempty"`
	HeartbeatAt         *time.Time `json:"heartbeatAt,omitempty"`
	RunningVersion      uint64     `json:"runningVersion"`
	RunningChecksum     string     `json:"runningChecksum,omitempty"`
	Health              string     `json:"health,omitempty"`
	Pid                 int        `json:"pid"`
	BuildVersion        string     `json:"buildVersion,omitempty"`
	StartedAt           *time.Time `json:"startedAt,omitempty"`
	LastReloadAt        *time.Time `json:"lastReloadAt,omitempty"`
	LastReloadSuccess   *bool      `json:"lastReloadSuccess,omitempty"`
	LastReloadError     string     `json:"lastReloadError,omitempty"`

	// EffectState 与 r7 参数页前端同一套取值：effective / awaiting / drift / offline / unknown。
	EffectState string `json:"effectState"`
	// VersionDrift 心跳版本与已发布版本号不一致；ChecksumDrift 版本号一致但快照校验和不同
	// （同一版本号被重新发布过，进程还在跑旧快照）。老构建不上报 checksum 时后者恒为 false。
	VersionDrift  bool `json:"versionDrift"`
	ChecksumDrift bool `json:"checksumDrift"`
}

// InstanceOverviewDTO 实例总览返回体。
type InstanceOverviewDTO struct {
	Instances []InstanceRuntimeDTO `json:"instances"`
	// DuplicateInstanceKeys 是 argus_instance 里出现重复的实例键。历史库缺唯一索引时
	// 才可能非空；一旦非空，配置发布与心跳都可能串实例，页面必须显式告警而不是静默。
	DuplicateInstanceKeys []string `json:"duplicateInstanceKeys"`
	// DriftInstanceKeys 已发布但心跳尚未确认生效的实例键，供页面直接做角标。
	DriftInstanceKeys []string `json:"driftInstanceKeys"`
	// OfflineInstanceKeys 没有心跳的实例键。
	OfflineInstanceKeys []string `json:"offlineInstanceKeys"`
	Notice              string   `json:"notice"`
}
