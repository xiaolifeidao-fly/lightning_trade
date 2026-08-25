package argus_config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	argusDTO "service/argus_config/dto"
)

const (
	mainPropertiesFilename = "application.properties"
	mainSessionFilename    = "session.json"
)

type ImportSummary struct {
	Accounts       int
	Sessions       int
	MonitorSymbols int
	TelegramSet    bool
}

// LoadMainConfigImport parses only BADelay's main application.properties and
// session.json. It returns DTOs suitable for SaveDraft without logging secrets.
func LoadMainConfigImport(propertiesPath, sessionPath string) (*argusDTO.SaveConfigRequest, ImportSummary, error) {
	if err := validateImportFile(propertiesPath, mainPropertiesFilename); err != nil {
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
	return request, ImportSummary{
		Accounts:       len(request.Accounts),
		Sessions:       len(request.Sessions),
		MonitorSymbols: len(request.MonitorSymbols),
		TelegramSet:    request.Notification.TelegramEnabled == 1,
	}, nil
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
			request.Sessions = append(request.Sessions, importRuntimeSession(session, index))
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
		LoginType:      firstNonBlank(properties[prefix+"login_type"], session.LoginType, "config"),
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
		RiskBudget:            decimal(properties, "position.risk.budget_pct", 0),
		CatastrophicStopLoss:  decimal(properties, "position.monitor.catastrophe_stop_pct", 0),
		ReverseGateEnabled:    reverseGate,
		MaxContracts:          integer(properties, prefix+"max_contracts_ceiling", integer(properties, "position.risk.max_contracts_ceiling", 0)),
		ExtraRiskJSON:         string(extra),
	}
}

func importRuntimeSession(session importSession, accountID int) argusDTO.RuntimeSessionDTO {
	updatedAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, session.UpdatedAt); err == nil {
		updatedAt = parsed
	}
	valid := uint8(0)
	if strings.TrimSpace(session.Cookie) != "" && strings.TrimSpace(session.Token) != "" {
		valid = 1
	}
	return argusDTO.RuntimeSessionDTO{AccountID: uint64(accountID), Cookie: session.Cookie, Token: session.Token, OToken: session.OToken, SentryRelease: session.SentryRelease, SentryPublicKey: session.SentryPublicKey, Baggage: session.Baggage, LoginURL: session.LoginURL, FinalURL: session.FinalURL, Valid: valid, SessionUpdatedAt: updatedAt}
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
