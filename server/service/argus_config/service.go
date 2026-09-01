package argus_config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"common/middleware/db"
	commonRedis "common/middleware/redis"
	"common/middleware/vipper"
	argusDTO "service/argus_config/dto"
	"service/argus_config/repository"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	ErrConfigValidation = errors.New("argus config validation failed")
	ErrDraftNotFound    = errors.New("argus config draft not found")
)

type ArgusConfigService struct {
	repository *repository.ArgusConfigRepository
	redisTTL   time.Duration
}

func NewArgusConfigService() *ArgusConfigService {
	return &ArgusConfigService{repository: db.GetRepository[repository.ArgusConfigRepository](), redisTTL: 24 * time.Hour}
}

func NewArgusConfigServiceWithRepository(repo *repository.ArgusConfigRepository) *ArgusConfigService {
	return &ArgusConfigService{repository: repo, redisTTL: 24 * time.Hour}
}

func (s *ArgusConfigService) EnsureTable() error {
	if err := s.repository.EnsureTable(); err != nil {
		return err
	}
	// instance_key 是本次分域改造新增列，历史版本行先归到默认实例，
	// 否则实例内唯一索引会把它们全判成同一个 published 冲突。
	defaultKey := strings.TrimSpace(vipper.GetString("argus.instance.default_key"))
	if defaultKey == "" {
		defaultKey = strings.TrimSpace(vipper.GetString("argus.instance.id"))
	}
	if defaultKey == "" {
		logrus.Warn("未配置 argus.instance.default_key/argus.instance.id，跳过 instance_key 历史数据补齐")
		return nil
	}
	rows, err := s.repository.BackfillInstanceKey(defaultKey)
	if err != nil {
		return err
	}
	if rows > 0 {
		logrus.Infof("Argus 配置版本 instance_key 已补齐: instanceKey=%s rows=%d", defaultKey, rows)
	}
	return nil
}

func (s *ArgusConfigService) GetPublished(ctx context.Context, instanceKey string) (*argusDTO.ConfigSnapshotDTO, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resolvedKey, err := s.ResolveInstanceKey(instanceKey)
	if err != nil {
		return nil, err
	}
	version, err := s.repository.FindPublishedContext(ctx, resolvedKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.snapshotDTOContext(ctx, resolvedKey, uint64(version.Id))
}

// SaveDraft 把草稿写入指定实例；instanceKey 为空时回落到 req.InstanceKey，
// 再由 ResolveInstanceKey 解析默认实例。
func (s *ArgusConfigService) SaveDraft(instanceKey string, req *argusDTO.SaveConfigRequest, actor string) (*argusDTO.ConfigVersionDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if strings.TrimSpace(instanceKey) == "" {
		instanceKey = req.InstanceKey
	}
	resolvedKey, err := s.ResolveInstanceKey(instanceKey)
	if err != nil {
		return nil, err
	}
	if err := validateRequest(req); err != nil {
		return nil, err
	}
	if s.repository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if err := s.mergePublishedSecrets(resolvedKey, req); err != nil {
		return nil, err
	}
	versionNumber, err := s.repository.NextVersion(resolvedKey)
	if err != nil {
		return nil, err
	}
	version := &repository.ArgusConfigVersion{InstanceKey: resolvedKey, Version: versionNumber, Status: repository.ConfigVersionStatusDraft, ReleaseNote: strings.TrimSpace(req.ReleaseNote), PublishedBy: strings.TrimSpace(actor)}
	err = s.repository.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		return s.saveChildren(tx, version, req)
	})
	if err != nil {
		return nil, err
	}
	result := versionDTO(version)
	return &result, nil
}

func (s *ArgusConfigService) mergePublishedSecrets(instanceKey string, req *argusDTO.SaveConfigRequest) error {
	published, err := s.repository.FindPublished(instanceKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, config, accounts, _, _, notification, sessions, err := s.repository.LoadSnapshot(instanceKey, uint64(published.Id))
	if err != nil {
		return err
	}
	if config != nil {
		if value := secretOrEmpty(req.Config.AICloseAPIKey); value == "" {
			req.Config.AICloseAPIKey, err = config.AICloseAPIKey.Decrypt()
			if err != nil {
				return err
			}
		}
		if value := secretOrEmpty(req.Config.AIOpenAPIKey); value == "" {
			req.Config.AIOpenAPIKey, err = config.AIOpenAPIKey.Decrypt()
			if err != nil {
				return err
			}
		}
	}
	byName := make(map[string]*repository.ArgusAccount, len(accounts))
	for _, account := range accounts {
		byName[account.AccountName] = account
	}
	for index := range req.Accounts {
		old := byName[req.Accounts[index].AccountName]
		if old == nil {
			continue
		}
		if req.Accounts[index].Username = preserveSecret(req.Accounts[index].Username, old.Username, &err); err != nil {
			return err
		}
		if req.Accounts[index].Password = preserveSecret(req.Accounts[index].Password, old.Password, &err); err != nil {
			return err
		}
		if req.Accounts[index].GoogleAuthKey = preserveSecret(req.Accounts[index].GoogleAuthKey, old.GoogleAuthKey, &err); err != nil {
			return err
		}
		if req.Accounts[index].APIKey = preserveSecret(req.Accounts[index].APIKey, old.APIKey, &err); err != nil {
			return err
		}
		if req.Accounts[index].SecretKey = preserveSecret(req.Accounts[index].SecretKey, old.SecretKey, &err); err != nil {
			return err
		}
		if req.Accounts[index].Passphrase = preserveSecret(req.Accounts[index].Passphrase, old.Passphrase, &err); err != nil {
			return err
		}
	}
	if notification != nil {
		if req.Notification.TelegramBotToken = preserveSecret(req.Notification.TelegramBotToken, notification.TelegramBotToken, &err); err != nil {
			return err
		}
		if req.Notification.TelegramChatID = preserveSecret(req.Notification.TelegramChatID, notification.TelegramChatID, &err); err != nil {
			return err
		}
	}
	for index := range req.Sessions {
		if index >= len(sessions) {
			break
		}
		old := sessions[index]
		if req.Sessions[index].Cookie = preserveSecret(req.Sessions[index].Cookie, old.Cookie, &err); err != nil {
			return err
		}
		if req.Sessions[index].Token = preserveSecret(req.Sessions[index].Token, old.Token, &err); err != nil {
			return err
		}
		if req.Sessions[index].OToken = preserveSecret(req.Sessions[index].OToken, old.OToken, &err); err != nil {
			return err
		}
		if req.Sessions[index].SentryRelease = preserveSecret(req.Sessions[index].SentryRelease, old.SentryRelease, &err); err != nil {
			return err
		}
		if req.Sessions[index].SentryPublicKey = preserveSecret(req.Sessions[index].SentryPublicKey, old.SentryPublicKey, &err); err != nil {
			return err
		}
		if req.Sessions[index].Baggage = preserveSecret(req.Sessions[index].Baggage, old.Baggage, &err); err != nil {
			return err
		}
	}
	return nil
}

func secretOrEmpty(value string) string {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) == "******" {
		return ""
	}
	return value
}

func preserveSecret(value string, previous repository.EncryptedString, resultErr *error) string {
	if secretOrEmpty(value) != "" || previous == "" {
		return value
	}
	value, *resultErr = previous.Decrypt()
	return value
}

// Publish 只影响目标实例：归档、快照键、广播消息全部带实例键。
func (s *ArgusConfigService) Publish(ctx context.Context, instanceKey string, versionID uint64, req *argusDTO.PublishConfigRequest, actor string) (*argusDTO.ConfigVersionDTO, error) {
	if versionID == 0 {
		return nil, fmt.Errorf("version id is required")
	}
	if s.repository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if strings.TrimSpace(instanceKey) == "" && req != nil {
		instanceKey = req.InstanceKey
	}
	resolvedKey, err := s.ResolveInstanceKey(instanceKey)
	if err != nil {
		return nil, err
	}
	var draft repository.ArgusConfigVersion
	if err := s.repository.Db.Where("id = ? AND instance_key = ? AND status = ? AND active = 1", versionID, resolvedKey, repository.ConfigVersionStatusDraft).First(&draft).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDraftNotFound
		}
		return nil, err
	}
	if err := s.validateVersion(resolvedKey, versionID); err != nil {
		return nil, err
	}
	previousVersion, previousVersionErr := s.repository.FindPublished(resolvedKey)
	previousEnvelope, previousEnvelopeErr := commonRedis.ReadConfigSnapshot(ctx, resolvedKey)
	version, config, accounts, risks, symbols, notification, sessions, err := s.repository.LoadSnapshot(resolvedKey, versionID)
	if err != nil {
		return nil, err
	}
	version.PublishedBy = strings.TrimSpace(actor)
	if req != nil && strings.TrimSpace(req.ReleaseNote) != "" {
		version.ReleaseNote = strings.TrimSpace(req.ReleaseNote)
	}
	payload := repositorySnapshot{Version: *version, Config: *config, Accounts: accounts, AccountRisks: risks, MonitorSymbols: symbols, Notification: *notification, Sessions: sessions}
	envelope, err := commonRedis.WriteConfigSnapshot(ctx, resolvedKey, version.Version, payload, s.redisTTL)
	if err != nil {
		return nil, err
	}
	version.SnapshotChecksum = envelope.Checksum
	err = s.repository.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&repository.ArgusConfigVersion{}).Where("instance_key = ? AND status = ? AND published_slot = ?", resolvedKey, repository.ConfigVersionStatusPublished, 1).Updates(map[string]interface{}{"status": repository.ConfigVersionStatusArchived, "published_slot": nil, "published_at": nil}).Error; err != nil {
			return err
		}
		publishedAt := time.Now().UTC()
		version.MarkPublished(publishedAt)
		version.PublishedBy = strings.TrimSpace(actor)
		return tx.Save(&version).Error
	})
	if err != nil {
		_ = s.restoreRedisSnapshot(ctx, resolvedKey, previousEnvelope, previousEnvelopeErr)
		return nil, err
	}
	if err := commonRedis.PublishConfigVersion(ctx, resolvedKey, version.Version, envelope.Checksum); err != nil {
		_ = s.rollbackPublishedVersion(resolvedKey, previousVersion, previousVersionErr, version.Id)
		_ = s.restoreRedisSnapshot(ctx, resolvedKey, previousEnvelope, previousEnvelopeErr)
		return nil, err
	}
	result := versionDTO(version)
	return &result, nil
}

// rollbackPublishedVersion 只回滚目标实例的 published 槽位。
func (s *ArgusConfigService) rollbackPublishedVersion(instanceKey string, previous *repository.ArgusConfigVersion, previousErr error, publishedID int) error {
	if s.repository.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if strings.TrimSpace(instanceKey) == "" {
		return repository.ErrInstanceKeyRequired
	}
	return s.repository.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&repository.ArgusConfigVersion{}).Where("id = ? AND instance_key = ?", publishedID, instanceKey).Updates(map[string]interface{}{"status": repository.ConfigVersionStatusArchived, "published_slot": nil, "published_at": nil}).Error; err != nil {
			return err
		}
		if previousErr == nil && previous != nil {
			previous.MarkPublished(time.Now().UTC())
			return tx.Save(previous).Error
		}
		return nil
	})
}

func (s *ArgusConfigService) restoreRedisSnapshot(ctx context.Context, instanceKey string, envelope commonRedis.ConfigSnapshotEnvelope, envelopeErr error) error {
	if envelopeErr != nil {
		return commonRedis.DeleteConfigSnapshot(ctx, instanceKey)
	}
	_, err := commonRedis.WriteConfigSnapshot(ctx, instanceKey, envelope.Version, json.RawMessage(envelope.Payload), s.redisTTL)
	return err
}

func (s *ArgusConfigService) Validate(req *argusDTO.SaveConfigRequest) error {
	return validateRequest(req)
}

func (s *ArgusConfigService) validateVersion(instanceKey string, versionID uint64) error {
	_, config, accounts, risks, symbols, notification, _, err := s.repository.LoadSnapshot(instanceKey, versionID)
	if err != nil {
		return err
	}
	if config == nil || len(accounts) == 0 {
		return fmt.Errorf("%w: at least one account is required", ErrConfigValidation)
	}
	if len(symbols) == 0 {
		return fmt.Errorf("%w: at least one monitor symbol is required", ErrConfigValidation)
	}
	riskAccounts := make(map[uint64]bool, len(risks))
	for _, risk := range risks {
		riskAccounts[risk.AccountID] = true
	}
	for _, account := range accounts {
		if strings.TrimSpace(account.AccountName) == "" || strings.TrimSpace(account.URL) == "" {
			return fmt.Errorf("%w: account name and url are required", ErrConfigValidation)
		}
		if !riskAccounts[uint64(account.Id)] {
			return fmt.Errorf("%w: account %d risk config is required", ErrConfigValidation, account.Id)
		}
	}
	for _, symbol := range symbols {
		if strings.TrimSpace(symbol.Symbol) == "" || strings.TrimSpace(symbol.TradeInstrument) == "" {
			return fmt.Errorf("%w: symbol and trade instrument are required", ErrConfigValidation)
		}
	}
	if config.ServerPort == 0 || config.ServerPort > 65535 || config.MonitorIntervalSecond <= 0 {
		return fmt.Errorf("%w: invalid port or monitor interval", ErrConfigValidation)
	}
	if notification != nil && notification.TelegramEnabled == 1 && (notification.TelegramBotToken == "" || notification.TelegramChatID == "") {
		return fmt.Errorf("%w: telegram credentials are required", ErrConfigValidation)
	}
	return nil
}

type repositorySnapshot struct {
	Version        repository.ArgusConfigVersion     `json:"version"`
	Config         repository.ArgusConfig            `json:"config"`
	Accounts       []*repository.ArgusAccount        `json:"accounts"`
	AccountRisks   []*repository.ArgusAccountRisk    `json:"accountRisks"`
	MonitorSymbols []*repository.ArgusMonitorSymbol  `json:"monitorSymbols"`
	Notification   repository.ArgusNotification      `json:"notification"`
	Sessions       []*repository.ArgusRuntimeSession `json:"sessions"`
}

func (s *ArgusConfigService) snapshotDTO(instanceKey string, versionID uint64) (*argusDTO.ConfigSnapshotDTO, error) {
	return s.snapshotDTOContext(context.Background(), instanceKey, versionID)
}

func (s *ArgusConfigService) snapshotDTOContext(ctx context.Context, instanceKey string, versionID uint64) (*argusDTO.ConfigSnapshotDTO, error) {
	version, config, accounts, risks, symbols, notification, sessions, err := s.repository.LoadSnapshotContext(ctx, instanceKey, versionID)
	if err != nil {
		return nil, err
	}
	result := &argusDTO.ConfigSnapshotDTO{InstanceKey: version.InstanceKey, Version: versionDTO(version), Config: configDTO(config), Notification: notificationDTO(notification)}
	for _, account := range accounts {
		result.Accounts = append(result.Accounts, accountDTO(account))
	}
	for _, risk := range risks {
		result.AccountRisks = append(result.AccountRisks, riskDTO(risk))
	}
	for _, symbol := range symbols {
		result.MonitorSymbols = append(result.MonitorSymbols, symbolDTO(symbol))
	}
	for _, session := range sessions {
		result.Sessions = append(result.Sessions, sessionDTO(session))
	}
	return result, nil
}

func validateRequest(req *argusDTO.SaveConfigRequest) error {
	if req == nil {
		return fmt.Errorf("%w: request is nil", ErrConfigValidation)
	}
	if req.Config.ServerPort == 0 || req.Config.ServerPort > 65535 {
		return fmt.Errorf("%w: server port must be between 1 and 65535", ErrConfigValidation)
	}
	if req.Config.MonitorIntervalSecond <= 0 {
		return fmt.Errorf("%w: monitor interval must be positive", ErrConfigValidation)
	}
	if len(req.Accounts) == 0 {
		return fmt.Errorf("%w: at least one account is required", ErrConfigValidation)
	}
	if len(req.MonitorSymbols) == 0 {
		return fmt.Errorf("%w: at least one monitor symbol is required", ErrConfigValidation)
	}
	accountIDs := make(map[uint64]bool)
	for _, account := range req.Accounts {
		if strings.TrimSpace(account.AccountName) == "" || (strings.TrimSpace(account.Platform) == "" && strings.TrimSpace(account.URL) == "") {
			return fmt.Errorf("%w: account name and url are required", ErrConfigValidation)
		}
		if account.PositionMode != "" && account.PositionMode != "net" && account.PositionMode != "hedge" {
			return fmt.Errorf("%w: invalid position mode", ErrConfigValidation)
		}
		if account.PositionSide != "" && account.PositionSide != "long" && account.PositionSide != "short" && account.PositionSide != "both" {
			return fmt.Errorf("%w: invalid position side", ErrConfigValidation)
		}
		accountIDs[account.ID] = true
	}
	riskIDs := make(map[uint64]bool)
	for _, risk := range req.AccountRisks {
		if risk.AccountID == 0 || !accountIDs[risk.AccountID] {
			return fmt.Errorf("%w: risk references an unknown account", ErrConfigValidation)
		}
		riskIDs[risk.AccountID] = true
	}
	for _, account := range req.Accounts {
		if !riskIDs[account.ID] && account.ID != 0 {
			return fmt.Errorf("%w: account %d risk config is required", ErrConfigValidation, account.ID)
		}
	}
	seenSymbols := map[string]bool{}
	for _, symbol := range req.MonitorSymbols {
		key := strings.ToUpper(strings.TrimSpace(symbol.Symbol))
		if key == "" || strings.TrimSpace(symbol.TradeInstrument) == "" {
			return fmt.Errorf("%w: symbol and trade instrument are required", ErrConfigValidation)
		}
		if seenSymbols[key] {
			return fmt.Errorf("%w: duplicate symbol %s", ErrConfigValidation, key)
		}
		seenSymbols[key] = true
		if math.IsNaN(symbol.SpreadThreshold) || math.IsInf(symbol.SpreadThreshold, 0) {
			return fmt.Errorf("%w: invalid symbol threshold", ErrConfigValidation)
		}
	}
	if req.Notification.TelegramEnabled == 1 && strings.TrimSpace(req.Notification.TelegramBotToken) == "" && strings.TrimSpace(req.Notification.TelegramChatID) == "" {
		return fmt.Errorf("%w: telegram credentials are required", ErrConfigValidation)
	}
	return nil
}

func (s *ArgusConfigService) saveChildren(tx *gorm.DB, version *repository.ArgusConfigVersion, req *argusDTO.SaveConfigRequest) error {
	config := configEntity(version.Id, req.Config)
	if err := tx.Create(&config).Error; err != nil {
		return err
	}
	accountIDs := make(map[uint64]uint64, len(req.Accounts))
	for index, item := range req.Accounts {
		entity := accountEntity(version.Id, item)
		if err := tx.Create(&entity).Error; err != nil {
			return err
		}
		if item.ID != 0 {
			accountIDs[item.ID] = uint64(entity.Id)
		}
		accountIDs[uint64(index)+1] = uint64(entity.Id)
	}
	for _, item := range req.AccountRisks {
		if item.AccountID == 0 && len(req.Accounts) == 1 {
			item.AccountID = 1
		}
		if item.AccountID != 0 {
			item.AccountID = accountIDs[item.AccountID]
		}
		entity := riskEntity(version.Id, item)
		if err := tx.Create(&entity).Error; err != nil {
			return err
		}
	}
	for _, item := range req.MonitorSymbols {
		entity := symbolEntity(version.Id, item)
		if err := tx.Create(&entity).Error; err != nil {
			return err
		}
	}
	notification := notificationEntity(version.Id, req.Notification)
	if err := tx.Create(&notification).Error; err != nil {
		return err
	}
	for _, item := range req.Sessions {
		if item.AccountID == 0 && len(req.Accounts) == 1 {
			item.AccountID = 1
		}
		if item.AccountID != 0 {
			item.AccountID = accountIDs[item.AccountID]
		}
		entity := sessionEntity(item)
		if err := tx.Create(&entity).Error; err != nil {
			return err
		}
	}
	return nil
}

func configEntity(versionID int, value argusDTO.ConfigDTO) repository.ArgusConfig {
	return repository.ArgusConfig{ConfigVersionID: uint64(versionID), ServerPort: value.ServerPort, RequestPath: value.RequestPath, LogDir: value.LogDir, Enabled: value.Enabled, TradeEnabled: value.TradeEnabled, DefaultOrderSize: value.DefaultOrderSize, MonitorIntervalSecond: value.MonitorIntervalSecond, ProfitThreshold: value.ProfitThreshold, LossThreshold: value.LossThreshold, AICloseEnabled: value.AICloseEnabled, AICloseProvider: value.AICloseProvider, AICloseAPIURL: value.AICloseAPIURL, AICloseAPIKey: repository.NewEncryptedString(value.AICloseAPIKey), AICloseModel: value.AICloseModel, AICloseTimeoutSecond: value.AICloseTimeoutSecond, AICloseMaxTokens: value.AICloseMaxTokens, AICloseTemperature: value.AICloseTemperature, AICloseIntervalMinute: value.AICloseIntervalMinute, AICloseMinInterval: value.AICloseMinInterval, AICloseMaxInterval: value.AICloseMaxInterval, AIOpenEnabled: value.AIOpenEnabled, AIOpenAutoTrade: value.AIOpenAutoTrade, AIOpenAPIURL: value.AIOpenAPIURL, AIOpenAPIKey: repository.NewEncryptedString(value.AIOpenAPIKey), AIOpenModel: value.AIOpenModel, AIOpenTimeoutSecond: value.AIOpenTimeoutSecond, AIOpenMaxTokens: value.AIOpenMaxTokens, AIOpenTemperature: value.AIOpenTemperature, AIOpenIntervalMinute: value.AIOpenIntervalMinute, AIOpenMinInterval: value.AIOpenMinInterval, AIOpenMaxInterval: value.AIOpenMaxInterval, AIOpenMinLiqDistancePercent: value.AIOpenMinLiqDistancePercent, AIOpenMinLiqDistanceUSD: value.AIOpenMinLiqDistanceUSD, AIOpenMaxBalancePercent: value.AIOpenMaxBalancePercent, AIOpenMinOrderContracts: value.AIOpenMinOrderContracts, AIOpenMaxOrderContracts: value.AIOpenMaxOrderContracts, AIOpenMaxTotalContracts: value.AIOpenMaxTotalContracts, AIOpenCooldownMinute: value.AIOpenCooldownMinute, AIOpenLiqSafetyFactor: value.AIOpenLiqSafetyFactor, LoginScheduledEnabled: value.LoginScheduledEnabled, LoginScheduledHour: value.LoginScheduledHour, LoginScheduledMinute: value.LoginScheduledMinute, SessionMaxAgeDay: value.SessionMaxAgeDay, ExtraConfigJSON: value.ExtraConfigJSON}
}
func accountEntity(versionID int, value argusDTO.AccountDTO) repository.ArgusAccount {
	url := value.URL
	if url == "" {
		url = value.Platform
	}
	return repository.ArgusAccount{ConfigVersionID: uint64(versionID), AccountName: value.AccountName, URL: url, UID: value.UID, LoginType: value.LoginType, LoginHeadless: value.LoginHeadless, Username: repository.NewEncryptedString(value.Username), Password: repository.NewEncryptedString(value.Password), GoogleAuthKey: repository.NewEncryptedString(value.GoogleAuthKey), APIKey: repository.NewEncryptedString(value.APIKey), SecretKey: repository.NewEncryptedString(value.SecretKey), Passphrase: repository.NewEncryptedString(value.Passphrase), ResourceID: value.ResourceID, PositionMode: value.PositionMode, PositionSide: value.PositionSide, CloseStrategy: value.CloseStrategy, InitialBalance: value.InitialBalance, Enabled: value.Enabled}
}
func riskEntity(versionID int, value argusDTO.AccountRiskDTO) repository.ArgusAccountRisk {
	return repository.ArgusAccountRisk{ConfigVersionID: uint64(versionID), AccountID: value.AccountID, TakeProfitMode: value.TakeProfitMode, StopLossMode: value.StopLossMode, TrailingStopTiersJSON: value.TrailingStopTiersJSON, RiskBudget: value.RiskBudget, CatastrophicStopLoss: value.CatastrophicStopLoss, ReverseGateEnabled: value.ReverseGateEnabled, MaxContracts: value.MaxContracts, ExtraRiskJSON: value.ExtraRiskJSON}
}
func symbolEntity(versionID int, value argusDTO.MonitorSymbolDTO) repository.ArgusMonitorSymbol {
	return repository.ArgusMonitorSymbol{ConfigVersionID: uint64(versionID), Symbol: strings.ToUpper(strings.TrimSpace(value.Symbol)), DeepInstrument: value.DeepInstrument, TradeInstrument: value.TradeInstrument, SpreadThreshold: value.SpreadThreshold, SignalThreshold: value.SignalThreshold, Enabled: value.Enabled}
}
func notificationEntity(versionID int, value argusDTO.NotificationDTO) repository.ArgusNotification {
	return repository.ArgusNotification{ConfigVersionID: uint64(versionID), TelegramEnabled: value.TelegramEnabled, TelegramBotToken: repository.NewEncryptedString(value.TelegramBotToken), TelegramChatID: repository.NewEncryptedString(value.TelegramChatID)}
}
func sessionEntity(value argusDTO.RuntimeSessionDTO) repository.ArgusRuntimeSession {
	return repository.ArgusRuntimeSession{AccountID: value.AccountID, Cookie: repository.NewEncryptedString(value.Cookie), Token: repository.NewEncryptedString(value.Token), OToken: repository.NewEncryptedString(value.OToken), SentryRelease: repository.NewEncryptedString(value.SentryRelease), SentryPublicKey: repository.NewEncryptedString(value.SentryPublicKey), Baggage: repository.NewEncryptedString(value.Baggage), LoginURL: value.LoginURL, FinalURL: value.FinalURL, Valid: value.Valid, SessionUpdatedAt: value.SessionUpdatedAt, ExpiresAt: value.ExpiresAt, LastError: value.LastError}
}

func versionDTO(v *repository.ArgusConfigVersion) argusDTO.ConfigVersionDTO {
	return argusDTO.ConfigVersionDTO{ID: uint64(v.Id), InstanceKey: v.InstanceKey, Version: v.Version, Status: v.Status, ReleaseNote: v.ReleaseNote, PublishedBy: v.PublishedBy, PublishedAt: v.PublishedAt, SnapshotChecksum: v.SnapshotChecksum}
}
func configDTO(v *repository.ArgusConfig) argusDTO.ConfigDTO {
	return argusDTO.ConfigDTO{ID: uint64(v.Id), ServerPort: v.ServerPort, RequestPath: v.RequestPath, LogDir: v.LogDir, Enabled: v.Enabled, TradeEnabled: v.TradeEnabled, DefaultOrderSize: v.DefaultOrderSize, MonitorIntervalSecond: v.MonitorIntervalSecond, ProfitThreshold: v.ProfitThreshold, LossThreshold: v.LossThreshold, AICloseEnabled: v.AICloseEnabled, AICloseProvider: v.AICloseProvider, AICloseAPIURL: v.AICloseAPIURL, AICloseAPIKey: maskSecret(v.AICloseAPIKey), AICloseModel: v.AICloseModel, AICloseTimeoutSecond: v.AICloseTimeoutSecond, AICloseMaxTokens: v.AICloseMaxTokens, AICloseTemperature: v.AICloseTemperature, AICloseIntervalMinute: v.AICloseIntervalMinute, AICloseMinInterval: v.AICloseMinInterval, AICloseMaxInterval: v.AICloseMaxInterval, AIOpenEnabled: v.AIOpenEnabled, AIOpenAutoTrade: v.AIOpenAutoTrade, AIOpenAPIURL: v.AIOpenAPIURL, AIOpenAPIKey: maskSecret(v.AIOpenAPIKey), AIOpenModel: v.AIOpenModel, AIOpenTimeoutSecond: v.AIOpenTimeoutSecond, AIOpenMaxTokens: v.AIOpenMaxTokens, AIOpenTemperature: v.AIOpenTemperature, AIOpenIntervalMinute: v.AIOpenIntervalMinute, AIOpenMinInterval: v.AIOpenMinInterval, AIOpenMaxInterval: v.AIOpenMaxInterval, AIOpenMinLiqDistancePercent: v.AIOpenMinLiqDistancePercent, AIOpenMinLiqDistanceUSD: v.AIOpenMinLiqDistanceUSD, AIOpenMaxBalancePercent: v.AIOpenMaxBalancePercent, AIOpenMinOrderContracts: v.AIOpenMinOrderContracts, AIOpenMaxOrderContracts: v.AIOpenMaxOrderContracts, AIOpenMaxTotalContracts: v.AIOpenMaxTotalContracts, AIOpenCooldownMinute: v.AIOpenCooldownMinute, AIOpenLiqSafetyFactor: v.AIOpenLiqSafetyFactor, LoginScheduledEnabled: v.LoginScheduledEnabled, LoginScheduledHour: v.LoginScheduledHour, LoginScheduledMinute: v.LoginScheduledMinute, SessionMaxAgeDay: v.SessionMaxAgeDay, ExtraConfigJSON: v.ExtraConfigJSON}
}
func accountDTO(v *repository.ArgusAccount) argusDTO.AccountDTO {
	return argusDTO.AccountDTO{ID: uint64(v.Id), AccountName: v.AccountName, URL: v.URL, UID: v.UID, LoginType: v.LoginType, LoginHeadless: v.LoginHeadless, Username: maskSecret(v.Username), Password: maskSecret(v.Password), GoogleAuthKey: maskSecret(v.GoogleAuthKey), APIKey: maskSecret(v.APIKey), SecretKey: maskSecret(v.SecretKey), Passphrase: maskSecret(v.Passphrase), ResourceID: v.ResourceID, PositionMode: v.PositionMode, PositionSide: v.PositionSide, CloseStrategy: v.CloseStrategy, InitialBalance: v.InitialBalance, Enabled: v.Enabled}
}
func riskDTO(v *repository.ArgusAccountRisk) argusDTO.AccountRiskDTO {
	return argusDTO.AccountRiskDTO{ID: uint64(v.Id), AccountID: v.AccountID, TakeProfitMode: v.TakeProfitMode, StopLossMode: v.StopLossMode, TrailingStopTiersJSON: v.TrailingStopTiersJSON, RiskBudget: v.RiskBudget, CatastrophicStopLoss: v.CatastrophicStopLoss, ReverseGateEnabled: v.ReverseGateEnabled, MaxContracts: v.MaxContracts, ExtraRiskJSON: v.ExtraRiskJSON}
}
func symbolDTO(v *repository.ArgusMonitorSymbol) argusDTO.MonitorSymbolDTO {
	return argusDTO.MonitorSymbolDTO{ID: uint64(v.Id), Symbol: v.Symbol, DeepInstrument: v.DeepInstrument, TradeInstrument: v.TradeInstrument, SpreadThreshold: v.SpreadThreshold, SignalThreshold: v.SignalThreshold, Enabled: v.Enabled}
}
func notificationDTO(v *repository.ArgusNotification) argusDTO.NotificationDTO {
	return argusDTO.NotificationDTO{ID: uint64(v.Id), TelegramEnabled: v.TelegramEnabled, TelegramBotToken: maskSecret(v.TelegramBotToken), TelegramChatID: maskSecret(v.TelegramChatID)}
}
func sessionDTO(v *repository.ArgusRuntimeSession) argusDTO.RuntimeSessionDTO {
	return argusDTO.RuntimeSessionDTO{ID: uint64(v.Id), AccountID: v.AccountID, Cookie: maskSecret(v.Cookie), Token: maskSecret(v.Token), OToken: maskSecret(v.OToken), SentryRelease: maskSecret(v.SentryRelease), SentryPublicKey: maskSecret(v.SentryPublicKey), Baggage: maskSecret(v.Baggage), LoginURL: v.LoginURL, FinalURL: v.FinalURL, Valid: v.Valid, SessionUpdatedAt: v.SessionUpdatedAt, ExpiresAt: v.ExpiresAt, LastError: v.LastError}
}
func maskSecret(value repository.EncryptedString) string {
	if value == "" {
		return ""
	}
	return "******"
}
