package repository

import (
	"common/middleware/db"
	"time"
)

const (
	ConfigVersionStatusDraft     = "draft"
	ConfigVersionStatusPublished = "published"
	ConfigVersionStatusArchived  = "archived"
)

// ArgusConfigVersion is the single-instance configuration version root. All
// static configuration children belong to one immutable version snapshot.
type ArgusConfigVersion struct {
	db.BaseEntity
	Version          uint64     `gorm:"column:version;type:bigint unsigned;uniqueIndex:idx_argus_config_version" description:"单实例配置版本号"`
	Status           string     `gorm:"column:status;type:varchar(16);index:idx_argus_config_status;check:chk_argus_config_version_status,status IN ('draft','published','archived')" description:"draft/published/archived"`
	PublishedSlot    *uint8     `gorm:"column:published_slot;type:tinyint unsigned;uniqueIndex:idx_argus_single_published" description:"仅已发布版本为 1，其余为 NULL"`
	ReleaseNote      string     `gorm:"column:release_note;type:varchar(500)" description:"发布说明"`
	PublishedBy      string     `gorm:"column:published_by;type:varchar(64)" description:"发布操作者"`
	PublishedAt      *time.Time `gorm:"column:published_at;type:datetime" description:"发布时间"`
	SnapshotChecksum string     `gorm:"column:snapshot_checksum;type:char(64)" description:"完整快照 SHA-256"`
}

func (v *ArgusConfigVersion) TableName() string { return "argus_config_version" }

func (v *ArgusConfigVersion) MarkPublished(publishedAt time.Time) {
	publishedSlot := uint8(1)
	v.Status = ConfigVersionStatusPublished
	v.PublishedSlot = &publishedSlot
	v.PublishedAt = &publishedAt
}

func (v *ArgusConfigVersion) MarkUnpublished(status string) {
	v.Status = status
	v.PublishedSlot = nil
	v.PublishedAt = nil
}

type ArgusConfig struct {
	db.BaseEntity
	ConfigVersionID             uint64          `gorm:"column:config_version_id;type:bigint unsigned;uniqueIndex:idx_argus_config_version_id" description:"配置版本 ID"`
	ServerPort                  uint16          `gorm:"column:server_port;type:smallint unsigned;default:8855" description:"服务端口"`
	RequestPath                 string          `gorm:"column:request_path;type:varchar(255);default:/" description:"请求路径"`
	LogDir                      string          `gorm:"column:log_dir;type:varchar(500)" description:"日志目录"`
	Enabled                     uint8           `gorm:"column:enabled;type:tinyint unsigned;default:1;check:chk_argus_config_enabled,enabled IN (0,1)" description:"配置是否启用"`
	TradeEnabled                uint8           `gorm:"column:trade_enabled;type:tinyint unsigned;default:0;check:chk_argus_trade_enabled,trade_enabled IN (0,1)" description:"价差开仓开关"`
	DefaultOrderSize            int             `gorm:"column:default_order_size;type:int unsigned;default:0" description:"默认下单张数"`
	MonitorIntervalSecond       int             `gorm:"column:monitor_interval_second;type:int unsigned;default:5" description:"仓位巡检秒数"`
	ProfitThreshold             float64         `gorm:"column:profit_threshold;type:decimal(20,8);default:0" description:"盈利告警阈值"`
	LossThreshold               float64         `gorm:"column:loss_threshold;type:decimal(20,8);default:0" description:"亏损告警阈值"`
	AICloseEnabled              uint8           `gorm:"column:ai_close_enabled;type:tinyint unsigned;default:0;check:chk_argus_ai_close_enabled,ai_close_enabled IN (0,1)" description:"AI 平仓开关"`
	AICloseProvider             string          `gorm:"column:ai_close_provider;type:varchar(64)" description:"AI 平仓服务商"`
	AICloseAPIURL               string          `gorm:"column:ai_close_api_url;type:varchar(500)" description:"AI 平仓接口地址"`
	AICloseAPIKey               EncryptedString `gorm:"column:ai_close_api_key;type:text" description:"加密的 AI 平仓密钥"`
	AICloseModel                string          `gorm:"column:ai_close_model;type:varchar(128)" description:"AI 平仓模型"`
	AICloseTimeoutSecond        int             `gorm:"column:ai_close_timeout_second;type:int unsigned;default:120" description:"AI 平仓超时"`
	AICloseMaxTokens            int             `gorm:"column:ai_close_max_tokens;type:int unsigned;default:0" description:"AI 平仓最大 Token"`
	AICloseTemperature          float64         `gorm:"column:ai_close_temperature;type:decimal(8,4);default:0" description:"AI 平仓温度"`
	AICloseIntervalMinute       int             `gorm:"column:ai_close_interval_minute;type:int unsigned;default:0" description:"AI 平仓巡检间隔"`
	AICloseMinInterval          int             `gorm:"column:ai_close_min_interval_minute;type:int unsigned;default:0" description:"AI 平仓巡检下限"`
	AICloseMaxInterval          int             `gorm:"column:ai_close_max_interval_minute;type:int unsigned;default:0" description:"AI 平仓巡检上限"`
	AIOpenEnabled               uint8           `gorm:"column:ai_open_enabled;type:tinyint unsigned;default:0;check:chk_argus_ai_open_enabled,ai_open_enabled IN (0,1)" description:"AI 加仓开关"`
	AIOpenAutoTrade             uint8           `gorm:"column:ai_open_auto_trade;type:tinyint unsigned;default:0;check:chk_argus_ai_open_auto_trade,ai_open_auto_trade IN (0,1)" description:"AI 加仓自动交易"`
	AIOpenAPIURL                string          `gorm:"column:ai_open_api_url;type:varchar(500)" description:"AI 加仓接口地址，为空时复用平仓"`
	AIOpenAPIKey                EncryptedString `gorm:"column:ai_open_api_key;type:text" description:"加密的 AI 加仓密钥"`
	AIOpenModel                 string          `gorm:"column:ai_open_model;type:varchar(128)" description:"AI 加仓模型"`
	AIOpenTimeoutSecond         int             `gorm:"column:ai_open_timeout_second;type:int unsigned;default:0" description:"AI 加仓超时"`
	AIOpenMaxTokens             int             `gorm:"column:ai_open_max_tokens;type:int unsigned;default:0" description:"AI 加仓最大 Token"`
	AIOpenTemperature           float64         `gorm:"column:ai_open_temperature;type:decimal(8,4);default:0" description:"AI 加仓温度"`
	AIOpenIntervalMinute        int             `gorm:"column:ai_open_interval_minute;type:int unsigned;default:0" description:"AI 加仓巡检间隔"`
	AIOpenMinInterval           int             `gorm:"column:ai_open_min_interval_minute;type:int unsigned;default:0" description:"AI 加仓巡检下限"`
	AIOpenMaxInterval           int             `gorm:"column:ai_open_max_interval_minute;type:int unsigned;default:0" description:"AI 加仓巡检上限"`
	AIOpenMinLiqDistancePercent float64         `gorm:"column:ai_open_min_liq_distance_percent;type:decimal(20,8);default:0" description:"AI 加仓爆仓距离百分比下限"`
	AIOpenMinLiqDistanceUSD     float64         `gorm:"column:ai_open_min_liq_distance_usd;type:decimal(20,8);default:0" description:"AI 加仓爆仓距离金额下限"`
	AIOpenMaxBalancePercent     float64         `gorm:"column:ai_open_max_balance_percent;type:decimal(20,8);default:0" description:"AI 加仓可用余额比例"`
	AIOpenMinOrderContracts     int             `gorm:"column:ai_open_min_order_contracts;type:int unsigned;default:0" description:"AI 加仓最小张数"`
	AIOpenMaxOrderContracts     int             `gorm:"column:ai_open_max_order_contracts;type:int unsigned;default:0" description:"AI 加仓最大张数"`
	AIOpenMaxTotalContracts     int             `gorm:"column:ai_open_max_total_contracts;type:int unsigned;default:0" description:"AI 加仓总张数上限"`
	AIOpenCooldownMinute        int             `gorm:"column:ai_open_cooldown_minute;type:int unsigned;default:0" description:"AI 加仓冷却时间"`
	AIOpenLiqSafetyFactor       float64         `gorm:"column:ai_open_liq_safety_factor;type:decimal(8,4);default:0" description:"AI 加仓爆仓安全系数"`
	LoginScheduledEnabled       uint8           `gorm:"column:login_scheduled_enabled;type:tinyint unsigned;default:0;check:chk_argus_login_scheduled_enabled,login_scheduled_enabled IN (0,1)" description:"定时登录开关"`
	LoginScheduledHour          uint8           `gorm:"column:login_scheduled_hour;type:tinyint unsigned;default:0;check:chk_argus_login_scheduled_hour,login_scheduled_hour <= 23" description:"定时登录小时"`
	LoginScheduledMinute        uint8           `gorm:"column:login_scheduled_minute;type:tinyint unsigned;default:0;check:chk_argus_login_scheduled_minute,login_scheduled_minute <= 59" description:"定时登录分钟"`
	SessionMaxAgeDay            int             `gorm:"column:session_max_age_day;type:int unsigned;default:0" description:"会话最长天数"`
	ExtraConfigJSON             string          `gorm:"column:extra_config_json;type:json" description:"未结构化但受支持的扩展配置"`
}

func (c *ArgusConfig) TableName() string { return "argus_config" }

type ArgusAccount struct {
	db.BaseEntity
	ConfigVersionID uint64          `gorm:"column:config_version_id;type:bigint unsigned;uniqueIndex:idx_argus_account_version_name,priority:1" description:"配置版本 ID"`
	AccountName     string          `gorm:"column:account_name;type:varchar(128);uniqueIndex:idx_argus_account_version_name,priority:2" description:"账户名称"`
	URL             string          `gorm:"column:url;type:varchar(500)" description:"交易站点地址"`
	UID             string          `gorm:"column:uid;type:varchar(128);index:idx_argus_account_uid" description:"平台用户 ID"`
	LoginType       string          `gorm:"column:login_type;type:varchar(32);default:password;check:chk_argus_account_login_type,login_type IN ('password','api_key','cookie')" description:"登录方式"`
	LoginHeadless   uint8           `gorm:"column:login_headless;type:tinyint unsigned;default:1;check:chk_argus_account_login_headless,login_headless IN (0,1)" description:"无头登录"`
	Username        EncryptedString `gorm:"column:username;type:text" description:"加密的登录名"`
	Password        EncryptedString `gorm:"column:password;type:text" description:"加密的密码"`
	GoogleAuthKey   EncryptedString `gorm:"column:google_auth_key;type:text" description:"加密的 Google 验证器密钥"`
	APIKey          EncryptedString `gorm:"column:api_key;type:text" description:"加密的 API Key"`
	SecretKey       EncryptedString `gorm:"column:secret_key;type:text" description:"加密的 API Secret"`
	Passphrase      EncryptedString `gorm:"column:passphrase;type:text" description:"加密的交易口令"`
	ResourceID      string          `gorm:"column:resource_id;type:varchar(128)" description:"平台资源 ID"`
	PositionMode    string          `gorm:"column:position_mode;type:varchar(32);default:net;check:chk_argus_account_position_mode,position_mode IN ('net','hedge')" description:"持仓模式"`
	PositionSide    string          `gorm:"column:position_side;type:varchar(16);default:long;check:chk_argus_account_position_side,position_side IN ('long','short','both')" description:"默认持仓方向"`
	CloseStrategy   string          `gorm:"column:close_strategy;type:varchar(32);default:sltp" description:"平仓策略"`
	InitialBalance  float64         `gorm:"column:initial_balance;type:decimal(20,8);default:0" description:"初始余额"`
	Enabled         uint8           `gorm:"column:enabled;type:tinyint unsigned;default:1;check:chk_argus_account_enabled,enabled IN (0,1)" description:"账户启用状态"`
}

func (a *ArgusAccount) TableName() string { return "argus_account" }

type ArgusAccountRisk struct {
	db.BaseEntity
	ConfigVersionID       uint64  `gorm:"column:config_version_id;type:bigint unsigned;index:idx_argus_account_risk_version" description:"配置版本 ID"`
	AccountID             uint64  `gorm:"column:account_id;type:bigint unsigned;uniqueIndex:idx_argus_account_risk_account" description:"Argus 账户 ID"`
	TakeProfitMode        string  `gorm:"column:take_profit_mode;type:varchar(32)" description:"止盈模式"`
	StopLossMode          string  `gorm:"column:stop_loss_mode;type:varchar(32)" description:"止损模式"`
	TrailingStopTiersJSON string  `gorm:"column:trailing_stop_tiers_json;type:json" description:"移动止盈分层"`
	RiskBudget            float64 `gorm:"column:risk_budget;type:decimal(20,8);default:0" description:"风险预算"`
	CatastrophicStopLoss  float64 `gorm:"column:catastrophic_stop_loss;type:decimal(20,8);default:0" description:"灾难止损"`
	ReverseGateEnabled    uint8   `gorm:"column:reverse_gate_enabled;type:tinyint unsigned;default:0;check:chk_argus_risk_reverse_gate_enabled,reverse_gate_enabled IN (0,1)" description:"反向开仓门禁"`
	MaxContracts          int     `gorm:"column:max_contracts;type:int unsigned;default:0" description:"最大合约数"`
	ExtraRiskJSON         string  `gorm:"column:extra_risk_json;type:json" description:"扩展风控参数"`
}

func (r *ArgusAccountRisk) TableName() string { return "argus_account_risk" }

type ArgusMonitorSymbol struct {
	db.BaseEntity
	ConfigVersionID uint64  `gorm:"column:config_version_id;type:bigint unsigned;uniqueIndex:idx_argus_symbol_version_code,priority:1" description:"配置版本 ID"`
	Symbol          string  `gorm:"column:symbol;type:varchar(64);uniqueIndex:idx_argus_symbol_version_code,priority:2" description:"交易符号"`
	DeepInstrument  string  `gorm:"column:deep_instrument;type:varchar(128)" description:"DeepCoin 行情标识"`
	TradeInstrument string  `gorm:"column:trade_instrument;type:varchar(128)" description:"下单交易标识"`
	SpreadThreshold float64 `gorm:"column:spread_threshold;type:decimal(20,8);default:0" description:"价差阈值"`
	SignalThreshold float64 `gorm:"column:signal_threshold;type:decimal(20,8);default:0" description:"信号阈值"`
	Enabled         uint8   `gorm:"column:enabled;type:tinyint unsigned;default:1;check:chk_argus_symbol_enabled,enabled IN (0,1)" description:"启用状态"`
}

func (s *ArgusMonitorSymbol) TableName() string { return "argus_monitor_symbol" }

type ArgusNotification struct {
	db.BaseEntity
	ConfigVersionID  uint64          `gorm:"column:config_version_id;type:bigint unsigned;uniqueIndex:idx_argus_notification_version" description:"配置版本 ID"`
	TelegramEnabled  uint8           `gorm:"column:telegram_enabled;type:tinyint unsigned;default:0;check:chk_argus_telegram_enabled,telegram_enabled IN (0,1)" description:"Telegram 通知开关"`
	TelegramBotToken EncryptedString `gorm:"column:telegram_bot_token;type:text" description:"加密的 Telegram Bot Token"`
	TelegramChatID   EncryptedString `gorm:"column:telegram_chat_id;type:text" description:"加密的 Telegram Chat ID"`
}

func (n *ArgusNotification) TableName() string { return "argus_notification" }

// ArgusRuntimeSession is intentionally separate from ArgusAccount. Argus may
// refresh it at runtime without changing static account configuration.
type ArgusRuntimeSession struct {
	db.BaseEntity
	AccountID        uint64          `gorm:"column:account_id;type:bigint unsigned;uniqueIndex:idx_argus_runtime_session_account" description:"Argus 账户 ID"`
	Cookie           EncryptedString `gorm:"column:cookie;type:text" description:"加密的 Cookie"`
	Token            EncryptedString `gorm:"column:token;type:text" description:"加密的 Token"`
	OToken           EncryptedString `gorm:"column:otoken;type:text" description:"加密的 OToken"`
	SentryRelease    EncryptedString `gorm:"column:sentry_release;type:text" description:"加密的 Sentry Release"`
	SentryPublicKey  EncryptedString `gorm:"column:sentry_public_key;type:text" description:"加密的 Sentry Public Key"`
	Baggage          EncryptedString `gorm:"column:baggage;type:text" description:"加密的 Sentry Baggage"`
	LoginURL         string          `gorm:"column:login_url;type:varchar(1000)" description:"登录回跳地址"`
	FinalURL         string          `gorm:"column:final_url;type:varchar(1000)" description:"登录完成地址"`
	Valid            uint8           `gorm:"column:valid;type:tinyint unsigned;default:0;check:chk_argus_session_valid,valid IN (0,1)" description:"会话有效状态"`
	SessionUpdatedAt time.Time       `gorm:"column:session_updated_at;type:datetime;index:idx_argus_session_updated" description:"Argus 最后刷新会话时间"`
	ExpiresAt        *time.Time      `gorm:"column:expires_at;type:datetime;index:idx_argus_session_expires" description:"会话到期时间"`
	LastError        string          `gorm:"column:last_error;type:varchar(1000)" description:"最近刷新失败原因"`
}

func (s *ArgusRuntimeSession) TableName() string { return "argus_runtime_session" }
