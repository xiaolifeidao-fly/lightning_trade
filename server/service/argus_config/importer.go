package argus_config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	argusDTO "service/argus_config/dto"
)

const (
	mainSessionFilename = "session.json"
	// instanceIDPropertyKey 是 properties 里的实例键，三份部署配置各自不同。
	instanceIDPropertyKey = "argus.instance.id"
)

// instancePropertiesPattern 匹配三份部署实例的配置文件名，例如
// application.properties / application_1.properties / application_2.properties。
var instancePropertiesPattern = regexp.MustCompile(`^application(_[A-Za-z0-9]+)?\.properties$`)

type ImportSummary struct {
	// InstanceKey 取自 properties 的 argus.instance.id，标识本份配置属于哪个实例。
	InstanceKey    string
	SourceFile     string
	Accounts       int
	Sessions       int
	MonitorSymbols int
	TelegramSet    bool
}

// LoadMainConfigImport parses only BADelay's main application.properties and
// session.json. It returns DTOs suitable for SaveDraft without logging secrets.
func LoadMainConfigImport(propertiesPath, sessionPath string) (*argusDTO.SaveConfigRequest, ImportSummary, error) {
	if err := validateInstancePropertiesFile(propertiesPath); err != nil {
		return nil, ImportSummary{}, err
	}
	if err := validateImportFile(sessionPath, mainSessionFilename); err != nil {
		return nil, ImportSummary{}, err
	}

	properties, err := readProperties(propertiesPath)
	if err != nil {
		return nil, ImportSummary{}, err
	}
	sessions, err := readSessionFile(sessionPath)
	if err != nil {
		return nil, ImportSummary{}, err
	}
	request, err := buildImportRequest(properties, sessions)
	if err != nil {
		return nil, ImportSummary{}, err
	}
	request.InstanceKey = strings.TrimSpace(properties[instanceIDPropertyKey])
	return request, ImportSummary{
		InstanceKey:    request.InstanceKey,
		SourceFile:     filepath.Base(propertiesPath),
		Accounts:       len(request.Accounts),
		Sessions:       len(request.Sessions),
		MonitorSymbols: len(request.MonitorSymbols),
		TelegramSet:    request.Notification.TelegramEnabled == 1,
	}, nil
}

// AccountIdentity 一个账户的身份三元组，只含标签、数字 uid 与实验变体，
// 不含任何凭证。
type AccountIdentity struct {
	Name    string
	UID     string
	Variant string
}

// InstanceAccounts 一份部署 properties 里的实例键与账户身份清单。
type InstanceAccounts struct {
	InstanceKey string
	SourceFile  string
	Accounts    []AccountIdentity
}

// LoadInstanceAccounts 只读出实例键与 account 标签 → uid 映射，不碰
// session.json、不解析任何凭证字段。
//
// 历史事件回灌需要它：eventlog 的 account 字段存的是 acc.Name（人起的标签，
// 含邮箱），数字 uid 从来没进过事件（实测 9.9 万条历史事件里零出现），
// 任何字符串解析都不可能从标签恢复出 uid，只能从配置构建映射。
// 映射按实例隔离——不同实例各有一个「账户A-…」，指向不同的真实账户。
func LoadInstanceAccounts(propertiesPath string) (InstanceAccounts, error) {
	if err := validateInstancePropertiesFile(propertiesPath); err != nil {
		return InstanceAccounts{}, err
	}
	properties, err := readProperties(propertiesPath)
	if err != nil {
		return InstanceAccounts{}, err
	}
	accountCount, err := requiredPositiveInt(properties, "trade.account_count")
	if err != nil {
		return InstanceAccounts{}, err
	}
	result := InstanceAccounts{
		InstanceKey: strings.TrimSpace(properties[instanceIDPropertyKey]),
		SourceFile:  filepath.Base(propertiesPath),
	}
	for index := 1; index <= accountCount; index++ {
		prefix := fmt.Sprintf("trade.account%d.", index)
		name := strings.TrimSpace(properties[prefix+"name"])
		if name == "" {
			return InstanceAccounts{}, fmt.Errorf("%sname is required in %s", prefix, result.SourceFile)
		}
		result.Accounts = append(result.Accounts, AccountIdentity{
			Name:    name,
			UID:     strings.TrimSpace(properties[prefix+"uid"]),
			Variant: strings.TrimSpace(properties[prefix+"variant"]),
		})
	}
	return result, nil
}

type importSessionFile struct {
	Accounts map[string]importSession `json:"accounts"`
}

type importSession struct {
	AccountName     string `json:"accountName"`
	URL             string `json:"url"`
	UID             string `json:"uid"`
	LoginType       string `json:"loginType"`
	LoginHeadless   *bool  `json:"loginHeadless"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	GoogleAuthKey   string `json:"googleAuthKey"`
	APIKey          string `json:"apiKey"`
	SecretKey       string `json:"secretKey"`
	Passphrase      string `json:"passphrase"`
	ResourceID      string `json:"resourceId"`
	Cookie          string `json:"cookie"`
	Token           string `json:"token"`
	OToken          string `json:"otoken"`
	SentryRelease   string `json:"sentryRelease"`
	SentryPublicKey string `json:"sentryPublicKey"`
	Baggage         string `json:"baggage"`
	LoginURL        string `json:"loginURL"`
	FinalURL        string `json:"finalURL"`
	UpdatedAt       string `json:"updatedAt"`
}

// validateInstancePropertiesFile 允许三份实例配置文件名，仍拒绝任意路径。
func validateInstancePropertiesFile(path string) error {
	if !instancePropertiesPattern.MatchString(filepath.Base(path)) {
		return fmt.Errorf("import only accepts application[_suffix].properties, got %s", filepath.Base(path))
	}
	return validateImportFile(path, filepath.Base(path))
}

func validateImportFile(path, filename string) error {
	if filepath.Base(path) != filename {
		return fmt.Errorf("import only accepts %s", filename)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", filename, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", filename)
	}
	return nil
}

func readProperties(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read application.properties: %w", err)
	}
	result := make(map[string]string)
	for lineNumber, rawLine := range strings.Split(string(raw), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		separator := strings.IndexAny(line, "=:")
		if separator < 1 {
			return nil, fmt.Errorf("application.properties line %d has no key/value separator", lineNumber+1)
		}
		key := strings.TrimSpace(line[:separator])
		if key == "" {
			return nil, fmt.Errorf("application.properties line %d has an empty key", lineNumber+1)
		}
		result[key] = strings.TrimSpace(line[separator+1:])
	}
	return result, nil
}

func readSessionFile(path string) (map[string]importSession, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session.json: %w", err)
	}
	var file importSessionFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse session.json: %w", err)
	}
	if file.Accounts == nil {
		return map[string]importSession{}, nil
	}
	return file.Accounts, nil
}

func buildImportRequest(properties map[string]string, sessionByKey map[string]importSession) (*argusDTO.SaveConfigRequest, error) {
	accountCount, err := requiredPositiveInt(properties, "trade.account_count")
	if err != nil {
		return nil, err
	}
	request := &argusDTO.SaveConfigRequest{
		Config: argusDTO.ConfigDTO{
			ServerPort:            uint16(integer(properties, "server.port", 8855)),
			RequestPath:           stringOr(properties, "request.path", "/"),
			LogDir:                strings.TrimSpace(properties["log.dir"]),
			Enabled:               1,
			TradeEnabled:          1,
			DefaultOrderSize:      integer(properties, "trade.order_size", 1),
			MonitorIntervalSecond: integer(properties, "position.monitor.interval_seconds", 5),
			ProfitThreshold:       decimal(properties, "position.monitor.profit_threshold", 0),
			LossThreshold:         decimal(properties, "position.monitor.loss_threshold", 0),
			// r5：以下六项此前 DB 无列，只能留在 properties。缺省 0 = 未配置，
			// 运行时按「DB 优先 vipper 兜底」回退，不会把默认值当成显式配置。
			ContractFace:            decimal(properties, "position.risk.contract_face", 0),
			SignalDelaySecond:       integer(properties, "trade.signal.delay_seconds", 0),
			SpreadMaxPriceAgeMs:     integer(properties, "monitor.spread.max_price_age_ms", 0),
			TrendGateWindowHour:     decimal(properties, "trade.trend_gate.window_hours", 0),
			TrendGateThresholdPct:   decimal(properties, "trade.trend_gate.threshold_pct", 0),
			ReverseGateMinProfitPct: decimal(properties, "position.risk.reverse_gate_min_profit_pct", 0),
		},
	}
	if request.Config.ServerPort == 0 || request.Config.MonitorIntervalSecond <= 0 {
		return nil, fmt.Errorf("application.properties has an invalid server or monitor interval")
	}

	telegramToken := strings.TrimSpace(properties["telegram.bot_token"])
	telegramChatID := strings.TrimSpace(properties["telegram.chat_id"])
	request.Notification = argusDTO.NotificationDTO{TelegramBotToken: telegramToken, TelegramChatID: telegramChatID}
	if telegramToken != "" || telegramChatID != "" {
		if telegramToken == "" || telegramChatID == "" {
			return nil, fmt.Errorf("telegram configuration must include both bot token and chat id")
		}
		request.Notification.TelegramEnabled = 1
	}

	matchedSessions := make(map[string]bool, len(sessionByKey))
	for index := 1; index <= accountCount; index++ {
		prefix := fmt.Sprintf("trade.account%d.", index)
		account, session, sessionKey, err := importAccount(properties, sessionByKey, prefix, index)
		if err != nil {
			return nil, err
		}
		request.Accounts = append(request.Accounts, account)
		request.AccountRisks = append(request.AccountRisks, importRisk(properties, prefix, index))
		if sessionKey != "" {
			matchedSessions[sessionKey] = true
			request.Sessions = append(request.Sessions, importRuntimeSession(properties, session, prefix, index))
		}
	}
	for key := range sessionByKey {
		if !matchedSessions[key] {
			return nil, fmt.Errorf("session.json contains an account not declared by application.properties")
		}
	}

	symbols, err := importMonitorSymbols(properties)
	if err != nil {
		return nil, err
	}
	request.MonitorSymbols = symbols
	return request, nil
}

func importAccount(properties map[string]string, sessions map[string]importSession, prefix string, index int) (argusDTO.AccountDTO, importSession, string, error) {
	name := strings.TrimSpace(properties[prefix+"name"])
	if name == "" {
		return argusDTO.AccountDTO{}, importSession{}, "", fmt.Errorf("application.properties account %d has no name", index)
	}
	sessionKey, session := findSession(sessions, name, properties[prefix+"uid"], properties[prefix+"url"])
	value := argusDTO.AccountDTO{
		ID:             uint64(index),
		AccountName:    name,
		Platform:       "deepcoin",
		URL:            firstNonBlank(properties[prefix+"url"], session.URL),
		UID:            firstNonBlank(properties[prefix+"uid"], session.UID),
		LoginType:      resolveLoginType(properties, session, prefix),
		Username:       firstNonBlank(properties[prefix+"username"], session.Username),
		Password:       firstNonBlank(properties[prefix+"password"], session.Password),
		GoogleAuthKey:  firstNonBlank(properties[prefix+"google_auth_key"], session.GoogleAuthKey),
		APIKey:         firstNonBlank(properties[prefix+"api_key"], session.APIKey),
		SecretKey:      firstNonBlank(properties[prefix+"secret_key"], session.SecretKey),
		Passphrase:     firstNonBlank(properties[prefix+"passphrase"], session.Passphrase),
		ResourceID:     session.ResourceID,
		PositionMode:   normalizePositionMode(properties[prefix+"position_mode"]),
		PositionSide:   stringOr(properties, prefix+"position_side", "both"),
		CloseStrategy:  stringOr(properties, prefix+"close_strategy", "sltp"),
		InitialBalance: decimal(properties, prefix+"InitialBalance", 0),
		Enabled:        1,
	}
	if session.LoginHeadless != nil && *session.LoginHeadless {
		value.LoginHeadless = 1
	}
	if value.URL == "" {
		return argusDTO.AccountDTO{}, importSession{}, "", fmt.Errorf("application.properties account %d has no url", index)
	}
	return value, session, sessionKey, nil
}

func importRisk(properties map[string]string, prefix string, index int) argusDTO.AccountRiskDTO {
	trail := map[string]float64{}
	for key, value := range properties {
		if strings.HasPrefix(key, "position.monitor.trail.") {
			if number, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
				trail[strings.TrimPrefix(key, "position.monitor.trail.")] = number
			}
		}
	}
	trailJSON, _ := json.Marshal(trail)
	extra, _ := json.Marshal(map[string]string{
		"tradeDirection": stringOr(properties, prefix+"trade_direction", "forward"),
		"tradeLogic":     stringOr(properties, prefix+"trade_logic", "spread"),
		"variant":        strings.TrimSpace(properties[prefix+"variant"]),
	})
	reverseGate := uint8(0)
	if strings.EqualFold(strings.TrimSpace(properties[prefix+"reverse_gate"]), "on") {
		reverseGate = 1
	}
	return argusDTO.AccountRiskDTO{
		AccountID:             uint64(index),
		TakeProfitMode:        stringOr(properties, prefix+"tp_mode", "fixed"),
		StopLossMode:          "catastrophic",
		TrailingStopTiersJSON: string(trailJSON),
		// 账户级覆盖优先、全局兜底，与运行时 AccFloat 的解析顺序保持一致；
		// 原来只读全局键，application_1.properties 的 champion/challenger
		// 差异（budget_pct 13.3 / catastrophe 400）会在导入时被抹平。
		RiskBudget:           decimal(properties, prefix+"budget_pct", decimal(properties, "position.risk.budget_pct", 0)),
		CatastrophicStopLoss: decimal(properties, prefix+"catastrophe_stop_pct", decimal(properties, "position.monitor.catastrophe_stop_pct", 0)),
		ReverseGateEnabled:   reverseGate,
		MaxContracts:         integer(properties, prefix+"max_contracts_ceiling", integer(properties, "position.risk.max_contracts_ceiling", 0)),
		ExtraRiskJSON:        string(extra),
		// r5：账户级参数此前 DB 无列，champion/challenger 的差异只存在于 properties。
		OrderSize:               integer(properties, prefix+"order_size", 0),
		RiskEquity:              decimal(properties, prefix+"risk_equity", 0),
		ReverseGateMinProfitPct: decimal(properties, prefix+"reverse_gate_min_profit_pct", 0),
		TrendGateThresholdPct:   decimal(properties, prefix+"trend_gate_threshold_pct", 0),
	}
}

// resolveLoginType 决定账户走静态凭证还是密码登录。
//
// 原来是 firstNonBlank(properties[login_type], session.LoginType, "config")，
// 三个候选里 session.json 排在兜底之前。实测踩坑：部署用的 properties 从来
// 不写 login_type（运维靠 trade.accountN.cookie/token 维护静态凭证），而手上
// 那份 session.json 是几个月前的、里面写着 loginType=password —— 导入后账户
// 被判成密码登录模式，启动时去调 pl-instance 无头登录，而它并未部署，
// 于是盘口信号开仓全部失败（"获取 Web 用户凭证失败"）。
//
// 现在的顺序：properties 显式声明 > 按实际持有的凭证推断 > config。
// session.json 的 loginType 不再参与——它是运行态产物，不该决定模式。
func resolveLoginType(properties map[string]string, session importSession, prefix string) string {
	if explicit := strings.TrimSpace(properties[prefix+"login_type"]); explicit != "" {
		return explicit
	}
	// 有静态 cookie+token 就是 config 模式，与 argus_single 的
	// trade.BuildUserProvider / HasStaticWebCredentials 判定保持一致。
	if sessionCookie(properties, session, prefix) != "" && sessionToken(properties, session, prefix) != "" {
		return "config"
	}
	if strings.TrimSpace(firstNonBlank(properties[prefix+"username"], session.Username)) != "" &&
		strings.TrimSpace(firstNonBlank(properties[prefix+"password"], session.Password)) != "" {
		return "password"
	}
	return "config"
}

// sessionCookie / sessionToken 让 properties 里的值优先于 session.json。
//
// 运维每周更新的是 trade.accountN.cookie / trade.accountN.token，而
// session.json 只在 argus_single 无头登录成功后才被刷新。旧的 session.json
// 覆盖新的 properties，等于把刚换的凭证丢掉。
func sessionCookie(properties map[string]string, session importSession, prefix string) string {
	return firstNonBlank(properties[prefix+"cookie"], session.Cookie)
}

func sessionToken(properties map[string]string, session importSession, prefix string) string {
	return firstNonBlank(properties[prefix+"token"], session.Token)
}

func importRuntimeSession(properties map[string]string, session importSession, prefix string, accountID int) argusDTO.RuntimeSessionDTO {
	updatedAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, session.UpdatedAt); err == nil {
		updatedAt = parsed
	}
	cookie := sessionCookie(properties, session, prefix)
	token := sessionToken(properties, session, prefix)
	// otoken 只有 session.json 有；properties 里换了 token 却留着旧 otoken，
	// 会让 runtimeAccount 优先用旧 otoken（它先取 OToken 再回退 Token）。
	otoken := session.OToken
	if strings.TrimSpace(properties[prefix+"token"]) != "" {
		otoken = strings.TrimSpace(properties[prefix+"otoken"])
	}
	valid := uint8(0)
	if strings.TrimSpace(cookie) != "" && strings.TrimSpace(token) != "" {
		valid = 1
	}
	return argusDTO.RuntimeSessionDTO{AccountID: uint64(accountID), Cookie: cookie, Token: token, OToken: otoken, SentryRelease: firstNonBlank(properties[prefix+"sentryRelease"], session.SentryRelease), SentryPublicKey: firstNonBlank(properties[prefix+"SentryPublicKey"], session.SentryPublicKey), Baggage: firstNonBlank(properties[prefix+"baggage"], session.Baggage), LoginURL: session.LoginURL, FinalURL: session.FinalURL, Valid: valid, SessionUpdatedAt: updatedAt}
}

func importMonitorSymbols(properties map[string]string) ([]argusDTO.MonitorSymbolDTO, error) {
	prefixes := map[string]bool{}
	for key := range properties {
		if !strings.HasPrefix(key, "monitor.symbols.") {
			continue
		}
		remainder := strings.TrimPrefix(key, "monitor.symbols.")
		if separator := strings.Index(remainder, "."); separator > 0 {
			prefixes[remainder[:separator]] = true
		}
	}
	names := make([]string, 0, len(prefixes))
	for name := range prefixes {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, fmt.Errorf("application.properties has no monitor symbols")
	}
	result := make([]argusDTO.MonitorSymbolDTO, 0, len(names))
	for _, name := range names {
		prefix := "monitor.symbols." + name + "."
		value := argusDTO.MonitorSymbolDTO{Symbol: name, DeepInstrument: strings.TrimSpace(properties[prefix+"deep_inst"]), TradeInstrument: strings.TrimSpace(properties[prefix+"trade_inst"]), SpreadThreshold: decimal(properties, prefix+"threshold", 0), SignalThreshold: decimal(properties, prefix+"signal_threshold", 0), Enabled: 1}
		if value.DeepInstrument == "" || value.TradeInstrument == "" || value.SpreadThreshold <= 0 {
			return nil, fmt.Errorf("monitor symbol %s is incomplete", name)
		}
		result = append(result, value)
	}
	return result, nil
}

func findSession(sessions map[string]importSession, name, uid, url string) (string, importSession) {
	for key, value := range sessions {
		if value.AccountName == name || (uid != "" && value.UID == uid) || (url != "" && value.URL == url) {
			return key, value
		}
	}
	return "", importSession{}
}

func requiredPositiveInt(properties map[string]string, key string) (int, error) {
	value := integer(properties, key, 0)
	if value <= 0 {
		return 0, fmt.Errorf("application.properties %s must be positive", key)
	}
	return value, nil
}

func integer(properties map[string]string, key string, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(properties[key]))
	if err != nil {
		return fallback
	}
	return value
}

func decimal(properties map[string]string, key string, fallback float64) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(properties[key]), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return fallback
	}
	return value
}

func stringOr(properties map[string]string, key, fallback string) string {
	return firstNonBlank(properties[key], fallback)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizePositionMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "bidirectional", "hedge":
		return "hedge"
	case "net":
		return "net"
	default:
		return value
	}
}
