package coin_platform

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	coinPlatformDTO "service/coin_platform/dto"
	coinPlatformRepository "service/coin_platform/repository"
	"strings"

	"gorm.io/gorm"
)

type CoinPlatformService struct {
	coinPlatformRepository        *coinPlatformRepository.CoinPlatformRepository
	coinPlatformCoinRepository    *coinPlatformRepository.CoinPlatformCoinRepository
	coinPlatformAccountRepository *coinPlatformRepository.CoinPlatformAccountRepository
}

func NewCoinPlatformService() *CoinPlatformService {
	return &CoinPlatformService{
		coinPlatformRepository:        db.GetRepository[coinPlatformRepository.CoinPlatformRepository](),
		coinPlatformCoinRepository:    db.GetRepository[coinPlatformRepository.CoinPlatformCoinRepository](),
		coinPlatformAccountRepository: db.GetRepository[coinPlatformRepository.CoinPlatformAccountRepository](),
	}
}

func (s *CoinPlatformService) EnsureTable() error {
	if err := s.coinPlatformRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.coinPlatformCoinRepository.EnsureTable(); err != nil {
		return err
	}
	return s.coinPlatformAccountRepository.EnsureTable()
}

func (s *CoinPlatformService) ListPlatforms(query coinPlatformDTO.CoinPlatformQueryDTO) (*baseDTO.PageDTO[coinPlatformDTO.CoinPlatformDTO], error) {
	if s.coinPlatformRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePlatformPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.coinPlatformRepository.Db.Model(&coinPlatformRepository.CoinPlatform{}).Where("active = ?", 1)
	if value := strings.TrimSpace(query.Code); value != "" {
		dbQuery = dbQuery.Where("code LIKE ?", "%"+strings.ToLower(value)+"%")
	}
	if value := strings.TrimSpace(query.Name); value != "" {
		dbQuery = dbQuery.Where("name LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Country); value != "" {
		dbQuery = dbQuery.Where("country = ?", value)
	}
	if value := strings.TrimSpace(query.Status); value != "" {
		dbQuery = dbQuery.Where("status = ?", value)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*coinPlatformRepository.CoinPlatform
	if err := dbQuery.Order("sort_order DESC, id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[coinPlatformDTO.CoinPlatformDTO](entities)), nil
}

func (s *CoinPlatformService) GetPlatformByID(id uint) (*coinPlatformDTO.CoinPlatformDTO, error) {
	entity, err := s.coinPlatformRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformDTO](entity), nil
}

func (s *CoinPlatformService) GetPlatformByCode(code string) (*coinPlatformDTO.CoinPlatformDTO, error) {
	entity, err := s.coinPlatformRepository.FindByCode(strings.ToLower(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformDTO](entity), nil
}

func (s *CoinPlatformService) ListOnlinePlatforms() ([]*coinPlatformDTO.CoinPlatformDTO, error) {
	rows, err := s.coinPlatformRepository.ListOnline()
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinPlatformDTO.CoinPlatformDTO](rows), nil
}

func (s *CoinPlatformService) CreatePlatform(req *coinPlatformDTO.CreateCoinPlatformDTO) (*coinPlatformDTO.CoinPlatformDTO, error) {
	if s.coinPlatformRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	code := strings.ToLower(strings.TrimSpace(req.Code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	created, err := s.coinPlatformRepository.Create(&coinPlatformRepository.CoinPlatform{
		Code:            code,
		Name:            strings.TrimSpace(req.Name),
		FullName:        strings.TrimSpace(req.FullName),
		Icon:            strings.TrimSpace(req.Icon),
		Website:         strings.TrimSpace(req.Website),
		Country:         strings.TrimSpace(req.Country),
		ApiBaseURL:      strings.TrimSpace(req.ApiBaseURL),
		WsBaseURL:       strings.TrimSpace(req.WsBaseURL),
		DocsURL:         strings.TrimSpace(req.DocsURL),
		SupportedTypes:  strings.TrimSpace(req.SupportedTypes),
		DefaultFeeRate:  req.DefaultFeeRate,
		RateLimitPerSec: req.RateLimitPerSec,
		Status:          normalizePlatformStatus(req.Status),
		SortOrder:       req.SortOrder,
		Description:     strings.TrimSpace(req.Description),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformDTO](created), nil
}

func (s *CoinPlatformService) UpdatePlatform(id uint, req *coinPlatformDTO.UpdateCoinPlatformDTO) (*coinPlatformDTO.CoinPlatformDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinPlatformRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.Name != nil {
		entity.Name = strings.TrimSpace(*req.Name)
	}
	if req.FullName != nil {
		entity.FullName = strings.TrimSpace(*req.FullName)
	}
	if req.Icon != nil {
		entity.Icon = strings.TrimSpace(*req.Icon)
	}
	if req.Website != nil {
		entity.Website = strings.TrimSpace(*req.Website)
	}
	if req.Country != nil {
		entity.Country = strings.TrimSpace(*req.Country)
	}
	if req.ApiBaseURL != nil {
		entity.ApiBaseURL = strings.TrimSpace(*req.ApiBaseURL)
	}
	if req.WsBaseURL != nil {
		entity.WsBaseURL = strings.TrimSpace(*req.WsBaseURL)
	}
	if req.DocsURL != nil {
		entity.DocsURL = strings.TrimSpace(*req.DocsURL)
	}
	if req.SupportedTypes != nil {
		entity.SupportedTypes = strings.TrimSpace(*req.SupportedTypes)
	}
	if req.DefaultFeeRate != nil {
		entity.DefaultFeeRate = *req.DefaultFeeRate
	}
	if req.RateLimitPerSec != nil {
		entity.RateLimitPerSec = *req.RateLimitPerSec
	}
	if req.Status != nil {
		entity.Status = normalizePlatformStatus(*req.Status)
	}
	if req.SortOrder != nil {
		entity.SortOrder = *req.SortOrder
	}
	if req.Description != nil {
		entity.Description = strings.TrimSpace(*req.Description)
	}
	saved, err := s.coinPlatformRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformDTO](saved), nil
}

func (s *CoinPlatformService) DeletePlatform(id uint) error {
	entity, err := s.coinPlatformRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.coinPlatformRepository.SaveOrUpdate(entity)
	return err
}

func (s *CoinPlatformService) ListPlatformCoins(query coinPlatformDTO.CoinPlatformCoinQueryDTO) (*baseDTO.PageDTO[coinPlatformDTO.CoinPlatformCoinDTO], error) {
	if s.coinPlatformCoinRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePlatformPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.coinPlatformCoinRepository.Db.Model(&coinPlatformRepository.CoinPlatformCoin{}).Where("active = ?", 1)
	if query.PlatformID > 0 {
		dbQuery = dbQuery.Where("platform_id = ?", query.PlatformID)
	}
	if query.CoinID > 0 {
		dbQuery = dbQuery.Where("coin_id = ?", query.CoinID)
	}
	if value := strings.TrimSpace(query.CoinCode); value != "" {
		dbQuery = dbQuery.Where("coin_code LIKE ?", "%"+strings.ToUpper(value)+"%")
	}
	if value := strings.TrimSpace(query.ChainName); value != "" {
		dbQuery = dbQuery.Where("chain_name = ?", value)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*coinPlatformRepository.CoinPlatformCoin
	if err := dbQuery.Order("id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[coinPlatformDTO.CoinPlatformCoinDTO](rows)), nil
}

func (s *CoinPlatformService) ListPlatformsByCoinCode(coinCode string) ([]*coinPlatformDTO.CoinPlatformCoinDTO, error) {
	rows, err := s.coinPlatformCoinRepository.ListByCoinCode(strings.ToUpper(strings.TrimSpace(coinCode)))
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinPlatformDTO.CoinPlatformCoinDTO](rows), nil
}

func (s *CoinPlatformService) UpsertPlatformCoin(req *coinPlatformDTO.CreateCoinPlatformCoinDTO) (*coinPlatformDTO.CoinPlatformCoinDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.PlatformID == 0 || req.CoinID == 0 {
		return nil, fmt.Errorf("platformId and coinId are required")
	}
	existed, err := s.coinPlatformCoinRepository.FindByPlatformAndCoin(req.PlatformID, req.CoinID)
	if err == nil && existed != nil && existed.Id > 0 {
		existed.PlatformSymbol = strings.TrimSpace(req.PlatformSymbol)
		existed.ChainName = strings.TrimSpace(req.ChainName)
		existed.ContractAddress = strings.TrimSpace(req.ContractAddress)
		existed.DepositEnable = req.DepositEnable
		existed.WithdrawEnable = req.WithdrawEnable
		existed.TradeEnable = req.TradeEnable
		existed.MinWithdrawal = req.MinWithdrawal
		existed.WithdrawalFee = req.WithdrawalFee
		existed.Confirmations = req.Confirmations
		saved, err := s.coinPlatformCoinRepository.SaveOrUpdate(existed)
		if err != nil {
			return nil, err
		}
		return db.ToDTO[coinPlatformDTO.CoinPlatformCoinDTO](saved), nil
	}
	created, err := s.coinPlatformCoinRepository.Create(&coinPlatformRepository.CoinPlatformCoin{
		PlatformID:      req.PlatformID,
		PlatformCode:    strings.ToLower(strings.TrimSpace(req.PlatformCode)),
		CoinID:          req.CoinID,
		CoinCode:        strings.ToUpper(strings.TrimSpace(req.CoinCode)),
		PlatformSymbol:  strings.TrimSpace(req.PlatformSymbol),
		ChainName:       strings.TrimSpace(req.ChainName),
		ContractAddress: strings.TrimSpace(req.ContractAddress),
		DepositEnable:   req.DepositEnable,
		WithdrawEnable:  req.WithdrawEnable,
		TradeEnable:     req.TradeEnable,
		MinWithdrawal:   req.MinWithdrawal,
		WithdrawalFee:   req.WithdrawalFee,
		Confirmations:   req.Confirmations,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformCoinDTO](created), nil
}

func (s *CoinPlatformService) UpdatePlatformCoin(id uint, req *coinPlatformDTO.UpdateCoinPlatformCoinDTO) (*coinPlatformDTO.CoinPlatformCoinDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinPlatformCoinRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.PlatformSymbol != nil {
		entity.PlatformSymbol = strings.TrimSpace(*req.PlatformSymbol)
	}
	if req.ChainName != nil {
		entity.ChainName = strings.TrimSpace(*req.ChainName)
	}
	if req.ContractAddress != nil {
		entity.ContractAddress = strings.TrimSpace(*req.ContractAddress)
	}
	if req.DepositEnable != nil {
		entity.DepositEnable = *req.DepositEnable
	}
	if req.WithdrawEnable != nil {
		entity.WithdrawEnable = *req.WithdrawEnable
	}
	if req.TradeEnable != nil {
		entity.TradeEnable = *req.TradeEnable
	}
	if req.MinWithdrawal != nil {
		entity.MinWithdrawal = *req.MinWithdrawal
	}
	if req.WithdrawalFee != nil {
		entity.WithdrawalFee = *req.WithdrawalFee
	}
	if req.Confirmations != nil {
		entity.Confirmations = *req.Confirmations
	}
	saved, err := s.coinPlatformCoinRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformCoinDTO](saved), nil
}

func (s *CoinPlatformService) DeletePlatformCoin(id uint) error {
	entity, err := s.coinPlatformCoinRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.coinPlatformCoinRepository.SaveOrUpdate(entity)
	return err
}

func (s *CoinPlatformService) ListPlatformAccounts(query coinPlatformDTO.CoinPlatformAccountQueryDTO) (*baseDTO.PageDTO[coinPlatformDTO.CoinPlatformAccountDTO], error) {
	if s.coinPlatformAccountRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePlatformPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.coinPlatformAccountRepository.Db.Model(&coinPlatformRepository.CoinPlatformAccount{}).Where("active = ?", 1)
	if query.PlatformID > 0 {
		dbQuery = dbQuery.Where("platform_id = ?", query.PlatformID)
	}
	if value := strings.TrimSpace(query.AccountName); value != "" {
		dbQuery = dbQuery.Where("account_name LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Status); value != "" {
		dbQuery = dbQuery.Where("status = ?", value)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*coinPlatformRepository.CoinPlatformAccount
	if err := dbQuery.Order("id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[coinPlatformDTO.CoinPlatformAccountDTO](rows)), nil
}

func (s *CoinPlatformService) CreatePlatformAccount(req *coinPlatformDTO.CreateCoinPlatformAccountDTO) (*coinPlatformDTO.CoinPlatformAccountDTO, error) {
	if s.coinPlatformAccountRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.PlatformID == 0 {
		return nil, fmt.Errorf("platformId is required")
	}
	accountName := strings.TrimSpace(req.AccountName)
	if accountName == "" {
		return nil, fmt.Errorf("accountName is required")
	}
	created, err := s.coinPlatformAccountRepository.Create(&coinPlatformRepository.CoinPlatformAccount{
		PlatformID:   req.PlatformID,
		PlatformCode: strings.ToLower(strings.TrimSpace(req.PlatformCode)),
		AccountName:  accountName,
		AccountType:  normalizeAccountType(req.AccountType),
		ApiKey:       strings.TrimSpace(req.ApiKey),
		ApiSecret:    req.ApiSecret,
		Passphrase:   req.Passphrase,
		IPWhitelist:  strings.TrimSpace(req.IPWhitelist),
		Permissions:  strings.TrimSpace(req.Permissions),
		Status:       "active",
		ExpireTime:   req.ExpireTime,
		Remark:       strings.TrimSpace(req.Remark),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformAccountDTO](created), nil
}

func (s *CoinPlatformService) UpdatePlatformAccount(id uint, req *coinPlatformDTO.UpdateCoinPlatformAccountDTO) (*coinPlatformDTO.CoinPlatformAccountDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinPlatformAccountRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.AccountName != nil {
		entity.AccountName = strings.TrimSpace(*req.AccountName)
	}
	if req.AccountType != nil {
		entity.AccountType = normalizeAccountType(*req.AccountType)
	}
	if req.ApiKey != nil {
		entity.ApiKey = strings.TrimSpace(*req.ApiKey)
	}
	if req.ApiSecret != nil {
		entity.ApiSecret = *req.ApiSecret
	}
	if req.Passphrase != nil {
		entity.Passphrase = *req.Passphrase
	}
	if req.IPWhitelist != nil {
		entity.IPWhitelist = strings.TrimSpace(*req.IPWhitelist)
	}
	if req.Permissions != nil {
		entity.Permissions = strings.TrimSpace(*req.Permissions)
	}
	if req.Status != nil {
		entity.Status = normalizeAccountStatus(*req.Status)
	}
	if req.ExpireTime != nil {
		entity.ExpireTime = *req.ExpireTime
	}
	if req.Remark != nil {
		entity.Remark = strings.TrimSpace(*req.Remark)
	}
	saved, err := s.coinPlatformAccountRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinPlatformDTO.CoinPlatformAccountDTO](saved), nil
}

func (s *CoinPlatformService) DeletePlatformAccount(id uint) error {
	entity, err := s.coinPlatformAccountRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.coinPlatformAccountRepository.SaveOrUpdate(entity)
	return err
}

func normalizePlatformPage(page, pageIndex, pageSize int) (int, int) {
	if pageIndex <= 0 {
		pageIndex = page
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return pageIndex, pageSize
}

func normalizePlatformStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "online":
		return "online"
	case "offline":
		return "offline"
	case "maintenance":
		return "maintenance"
	default:
		return "online"
	}
}

func normalizeAccountType(accountType string) string {
	switch strings.ToLower(strings.TrimSpace(accountType)) {
	case "", "master":
		return "master"
	case "sub":
		return "sub"
	case "read_only", "readonly":
		return "read_only"
	default:
		return "master"
	}
}

func normalizeAccountStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active":
		return "active"
	case "disabled":
		return "disabled"
	case "expired":
		return "expired"
	default:
		return "active"
	}
}
