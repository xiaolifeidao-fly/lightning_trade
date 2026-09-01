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
	ID               uint64     `json:"id"`
	AccountID        uint64     `json:"accountId"`
	Cookie           string     `json:"cookie,omitempty"`
	Token            string     `json:"token,omitempty"`
	OToken           string     `json:"otoken,omitempty"`
	SentryRelease    string     `json:"sentryRelease,omitempty"`
	SentryPublicKey  string     `json:"sentryPublicKey,omitempty"`
	Baggage          string     `json:"baggage,omitempty"`
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
