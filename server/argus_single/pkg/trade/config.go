package trade

import (
	"common/middleware/vipper"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// 账户交易方向模式
const (
	TradeDirectionForward = "forward" // 正向：跟随行情（币安价高→开多, 币安价低→开空）
	TradeDirectionReverse = "reverse" // 反向：对冲行情（币安价高→开空, 币安价低→开多）
)

// 账户交易逻辑模式
const (
	TradeLogicSpread = "spread" // 老逻辑：根据 Binance 与 DeepCoin 价差决定开仓方向
	TradeLogicSignal = "signal" // 新逻辑：根据盘口 UP/DOWN 信号延迟调度下单
)

// 账户止盈模式
const (
	TPModeFixed    = "fixed"    // 现状：到 profit_threshold 整仓平、亏损只告警、无仓位上限
	TPModeTrailing = "trailing" // 新策略：分级移动止盈 + 仓位上限 + 兜底止损
)

// 盘口信号方向
const (
	OrderBookSignalUp   = "UP"
	OrderBookSignalDown = "DOWN"
)

type AccountConfig struct {
	Name                 string  `json:"name"`
	URL                  string  `json:"url"`
	APIKey               string  `json:"apiKey"`
	SecretKey            string  `json:"secretKey"`
	Passphrase           string  `json:"passphrase"`
	PositionMode         string  `json:"positionMode"`    // bidirectional=双向持仓, net=净仓模式
	PositionSide         string  `json:"positionSide"`    // long=多, short=空
	CloseStrategy        string  `json:"closeStrategy"`   // sltp=止盈止损, trigger=条件单平仓, trigger_open=条件单开仓
	UID                  string  `json:"uid"`             // 用户ID
	LoginType            string  `json:"loginType"`       // web凭证来源: config=配置直读(默认), password=账号密码登录
	LoginURL             string  `json:"loginURL"`        // DeepCoin 登录页地址
	Username             string  `json:"username"`        // DeepCoin 登录账号
	Password             string  `json:"password"`        // DeepCoin 登录密码
	GoogleAuthKey        string  `json:"googleAuthKey"`   // 谷歌验证码秘钥，用于后续生成动态验证码
	LoginHeadless        bool    `json:"loginHeadless"`   // Playwright 是否无头模式
	Cookie               string  `json:"cookie"`          // 浏览器Cookie
	Token                string  `json:"token"`           // 用户Token
	SentryRelease        string  `json:"sentryRelease"`   // Sentry Release
	SentryPublicKey      string  `json:"sentryPublicKey"` // Sentry Public Key
	InitialBalance       float64 `json:"initialBalance"`  // 初始余额
	OrderSize            int     `json:"orderSize"`       // 每次开仓张数（账户级，未配置时回退到 TradeConfig.OrderSize）
	TradeDirection       string  `json:"tradeDirection"`  // 交易方向: forward=正向(默认), reverse=反向
	TradeLogic           string  `json:"tradeLogic"`      // 交易逻辑: spread=老价差逻辑(默认), signal=盘口信号延迟逻辑
	TPMode               string  `json:"tpMode"`          // 止盈模式: fixed=现状(默认), trailing=移动止盈+上限+兜底止损
	ReverseGate          string  `json:"reverseGate"`     // 反向减仓门控: on/off(默认off); 仅 tp_mode=trailing 时生效
	Variant              string  `json:"variant"`         // 策略变体标签（JSONL 事件用），如 champion/default
	RiskBudget           float64 `json:"riskBudget"`
	CatastrophicStopLoss float64 `json:"catastrophicStopLoss"`
	MaxContracts         int     `json:"maxContracts"`
	Index                int     `json:"index"` // 账户序号(1-based)，账户级参数解析用
}

type TradeConfig struct {
	OrderSize int `json:"orderSize"` // 全局默认开仓张数（当账户未配置 order_size 时回退使用）
}

type TradingSystemConfig struct {
	Accounts []AccountConfig `json:"accounts"`
	Trade    TradeConfig     `json:"trade"`
}

func LoadConfigFromProperties() (*TradingSystemConfig, error) {
	config := &TradingSystemConfig{}
	sessionStore := DefaultSessionStore()

	// 加载全局默认交易配置（当账户未配置 order_size 时回退使用）
	config.Trade.OrderSize = vipper.GetInt("trade.order_size")

	// 加载账户配置
	accountCount := vipper.GetInt("trade.account_count")
	if accountCount == 0 {
		return nil, fmt.Errorf("未配置交易账户")
	}

	for i := 1; i <= accountCount; i++ {
		prefix := fmt.Sprintf("trade.account%d", i)

		account := AccountConfig{
			Name:            vipper.GetString(fmt.Sprintf("%s.name", prefix)),
			URL:             vipper.GetString(fmt.Sprintf("%s.url", prefix)),
			APIKey:          vipper.GetString(fmt.Sprintf("%s.api_key", prefix)),
			SecretKey:       vipper.GetString(fmt.Sprintf("%s.secret_key", prefix)),
			Passphrase:      vipper.GetString(fmt.Sprintf("%s.passphrase", prefix)),
			PositionMode:    vipper.GetString(fmt.Sprintf("%s.position_mode", prefix)),
			PositionSide:    vipper.GetString(fmt.Sprintf("%s.position_side", prefix)),
			CloseStrategy:   vipper.GetString(fmt.Sprintf("%s.close_strategy", prefix)),
			UID:             vipper.GetString(fmt.Sprintf("%s.uid", prefix)),
			LoginType:       vipper.GetString(fmt.Sprintf("%s.login_type", prefix)),
			LoginURL:        vipper.GetString(fmt.Sprintf("%s.login_url", prefix)),
			Username:        vipper.GetString(fmt.Sprintf("%s.username", prefix)),
			Password:        vipper.GetString(fmt.Sprintf("%s.password", prefix)),
			GoogleAuthKey:   vipper.GetString(fmt.Sprintf("%s.google_auth_key", prefix)),
			LoginHeadless:   vipper.GetBool(fmt.Sprintf("%s.login_headless", prefix)),
			Cookie:          vipper.GetString(fmt.Sprintf("%s.cookie", prefix)),
			Token:           vipper.GetString(fmt.Sprintf("%s.token", prefix)),
			SentryRelease:   vipper.GetString(fmt.Sprintf("%s.sentryRelease", prefix)),
			SentryPublicKey: vipper.GetString(fmt.Sprintf("%s.SentryPublicKey", prefix)),
			InitialBalance:  vipper.GetFloat64(fmt.Sprintf("%s.InitialBalance", prefix)),
			OrderSize:       vipper.GetInt(fmt.Sprintf("%s.order_size", prefix)),
			TradeDirection:  vipper.GetString(fmt.Sprintf("%s.trade_direction", prefix)),
			TradeLogic:      vipper.GetString(fmt.Sprintf("%s.trade_logic", prefix)),
			TPMode:          vipper.GetString(fmt.Sprintf("%s.tp_mode", prefix)),
			ReverseGate:     vipper.GetString(fmt.Sprintf("%s.reverse_gate", prefix)),
			Variant:         vipper.GetString(fmt.Sprintf("%s.variant", prefix)),
			Index:           i,
		}

		// 用 session.json 中保存的最新凭证（cookie/token/apiKey 等）覆盖配置文件中的种子值
		if sessionEntry, ok := sessionStore.Get(account); ok {
			applySessionAccountData(&account, sessionEntry)
		}

		// 验证必填字段
		if account.URL == "" || account.APIKey == "" || account.SecretKey == "" || account.Passphrase == "" {
			logrus.Warnf("账户%d配置不完整，跳过", i)
			continue
		}

		// 设置默认值
		if account.PositionMode == "" {
			account.PositionMode = "bidirectional"
		}
		if account.CloseStrategy == "" {
			account.CloseStrategy = "sltp"
		}
		if account.LoginType == "" {
			if account.HasLoginCredentials() {
				account.LoginType = WebCredentialModePassword
			} else {
				account.LoginType = WebCredentialModeConfig
			}
		}
		if account.LoginURL == "" {
			account.LoginURL = DefaultDeepCoinLoginURL
		}
		if account.TradeDirection == "" {
			account.TradeDirection = TradeDirectionForward
		} else if account.TradeDirection != TradeDirectionForward && account.TradeDirection != TradeDirectionReverse {
			logrus.Warnf("账户%d trade_direction 非法(%s)，回退为 %s", i, account.TradeDirection, TradeDirectionForward)
			account.TradeDirection = TradeDirectionForward
		}
		account.TradeLogic = normalizeTradeLogic(account.TradeLogic)
		account.TPMode = normalizeTPMode(account.TPMode)
		if normalizeOnOff(account.ReverseGate) && account.TPMode != TPModeTrailing {
			logrus.Warnf("账户%d reverse_gate=on 但 tp_mode=%s（非 trailing），缺少 cap+兜底止损保护，已强制关闭 reverse_gate", i, account.TPMode)
		}
		if account.IsSignalLogic() && account.PositionMode != "net" {
			logrus.Warnf("账户%d 使用盘口信号逻辑时要求净仓模式，position_mode 从 %s 调整为 net", i, account.PositionMode)
			account.PositionMode = "net"
		}

		config.Accounts = append(config.Accounts, account)
	}

	if len(config.Accounts) == 0 {
		return nil, fmt.Errorf("没有有效的交易账户配置")
	}

	// 设置默认交易参数（回退默认值）
	if config.Trade.OrderSize == 0 {
		config.Trade.OrderSize = 1
	}

	logrus.Infof("✅ 交易配置加载成功: %d个账户, 全局默认开仓张数: %d", len(config.Accounts), config.Trade.OrderSize)
	for i, acc := range config.Accounts {
		hasWebConfig := acc.UID != "" && acc.Cookie != "" && acc.Token != ""
		webStatus := "无Web配置"
		if hasWebConfig {
			webStatus = fmt.Sprintf("UID=%s", acc.UID)
		}
		logrus.Infof("  账户%d: %s, 持仓方向=%s, 模式=%s, 策略=%s, 交易方向=%s, 交易逻辑=%s, 止盈模式=%s, 反向门控=%v, 张数=%d, %s",
			i+1, acc.Name, acc.PositionSide, acc.PositionMode, acc.CloseStrategy,
			acc.TradeDirection, acc.TradeLogic, acc.TPMode, acc.IsReverseGate(), acc.GetOrderSize(config.Trade.OrderSize), webStatus)
	}

	return config, nil
}

func normalizeTradeLogic(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", TradeLogicSpread, "old", "legacy", "diff", "arbitrage":
		return TradeLogicSpread
	case TradeLogicSignal, "new", "orderbook", "book":
		return TradeLogicSignal
	default:
		logrus.Warnf("trade_logic 非法(%s)，回退为 %s", value, TradeLogicSpread)
		return TradeLogicSpread
	}
}

func normalizeTPMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TPModeTrailing:
		return TPModeTrailing
	case "", TPModeFixed:
		return TPModeFixed
	default:
		logrus.Warnf("tp_mode 非法(%s)，回退为 %s", value, TPModeFixed)
		return TPModeFixed
	}
}

// IsTrailingTP 账户是否使用移动止盈模式（否则为 fixed 现状逻辑）
func (acc *AccountConfig) IsTrailingTP() bool {
	return normalizeTPMode(acc.TPMode) == TPModeTrailing
}

func normalizeOnOff(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "on", "true", "1", "yes", "enable", "enabled":
		return true
	default:
		return false
	}
}

// IsReverseGate 反向减仓门控是否生效：reverse_gate=on 且 tp_mode=trailing（安全耦合，缺一不可）。
func (acc *AccountConfig) IsReverseGate() bool {
	return normalizeOnOff(acc.ReverseGate) && acc.IsTrailingTP()
}

func (acc *AccountConfig) IsLongAccount() bool {
	return acc.PositionSide == "long"
}

func (acc *AccountConfig) IsShortAccount() bool {
	return acc.PositionSide == "short"
}

func (acc *AccountConfig) IsSLTPStrategy() bool {
	return acc.CloseStrategy == "sltp"
}

func (acc *AccountConfig) HasStaticWebCredentials() bool {
	return acc.Cookie != "" && acc.Token != ""
}

func (acc *AccountConfig) HasLoginCredentials() bool {
	return acc.Username != "" && acc.Password != ""
}

func (acc *AccountConfig) HasWebCredentialSeed() bool {
	switch acc.LoginType {
	case WebCredentialModePassword:
		return acc.HasLoginCredentials()
	default:
		return acc.HasStaticWebCredentials()
	}
}

func (acc *AccountConfig) SessionKey() string {
	switch {
	case acc.Name != "":
		return acc.Name
	case acc.UID != "":
		return acc.UID
	default:
		return acc.Username
	}
}

// GetOrderSize 返回账户的开仓张数。
// 优先使用账户级 OrderSize，未配置时回退到 fallback（通常为全局 trade.order_size），再兜底为 1。
func (acc *AccountConfig) GetOrderSize(fallback int) int {
	if acc.OrderSize > 0 {
		return acc.OrderSize
	}
	if fallback > 0 {
		return fallback
	}
	return 1
}

// IsReverseDirection 账户是否配置为反向开仓
func (acc *AccountConfig) IsReverseDirection() bool {
	return acc.TradeDirection == TradeDirectionReverse
}

func (acc *AccountConfig) IsSpreadLogic() bool {
	return normalizeTradeLogic(acc.TradeLogic) == TradeLogicSpread
}

func (acc *AccountConfig) IsSignalLogic() bool {
	return normalizeTradeLogic(acc.TradeLogic) == TradeLogicSignal
}

// GetPosSide 根据行情方向与账户 trade_direction 配置，返回本次应开的持仓方向。
// needBuyDeep=true 表示行情显示应开多（币安价 > DeepCoin 价）。
// forward（默认）跟随行情；reverse 反向对冲。
func (acc *AccountConfig) GetPosSide(needBuyDeep bool) string {
	posSide := "long"
	if !needBuyDeep {
		posSide = "short"
	}
	if acc.IsReverseDirection() {
		if posSide == "long" {
			return "short"
		}
		return "long"
	}
	return posSide
}
