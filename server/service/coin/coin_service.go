package coin

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	coinDTO "service/coin/dto"
	coinRepository "service/coin/repository"
	"strings"

	"gorm.io/gorm"
)

type CoinService struct {
	coinRepository      *coinRepository.CoinRepository
	coinPairRepository  *coinRepository.CoinPairRepository
	coinPriceRepository *coinRepository.CoinPriceRepository
}

func NewCoinService() *CoinService {
	return &CoinService{
		coinRepository:      db.GetRepository[coinRepository.CoinRepository](),
		coinPairRepository:  db.GetRepository[coinRepository.CoinPairRepository](),
		coinPriceRepository: db.GetRepository[coinRepository.CoinPriceRepository](),
	}
}

func (s *CoinService) EnsureTable() error {
	if err := s.coinRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.coinPairRepository.EnsureTable(); err != nil {
		return err
	}
	return s.coinPriceRepository.EnsureTable()
}

func (s *CoinService) ListCoins(query coinDTO.CoinQueryDTO) (*baseDTO.PageDTO[coinDTO.CoinDTO], error) {
	if s.coinRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeCoinPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.coinRepository.Db.Model(&coinRepository.Coin{}).Where("active = ?", 1)
	if value := strings.TrimSpace(query.Code); value != "" {
		dbQuery = dbQuery.Where("code LIKE ?", "%"+strings.ToUpper(value)+"%")
	}
	if value := strings.TrimSpace(query.Name); value != "" {
		dbQuery = dbQuery.Where("name LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(query.ChainName); value != "" {
		dbQuery = dbQuery.Where("chain_name = ?", value)
	}
	if value := strings.TrimSpace(query.Status); value != "" {
		dbQuery = dbQuery.Where("status = ?", value)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*coinRepository.Coin
	if err := dbQuery.Order("sort_order DESC, id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[coinDTO.CoinDTO](entities)), nil
}

func (s *CoinService) GetCoinByID(id uint) (*coinDTO.CoinDTO, error) {
	entity, err := s.coinRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[coinDTO.CoinDTO](entity), nil
}

func (s *CoinService) GetCoinByCode(code string) (*coinDTO.CoinDTO, error) {
	entity, err := s.coinRepository.FindByCode(strings.ToUpper(strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinDTO](entity), nil
}

func (s *CoinService) ListOnlineCoins() ([]*coinDTO.CoinDTO, error) {
	rows, err := s.coinRepository.ListOnlineCoins()
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinDTO.CoinDTO](rows), nil
}

func (s *CoinService) CreateCoin(req *coinDTO.CreateCoinDTO) (*coinDTO.CoinDTO, error) {
	if s.coinRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	code := strings.ToUpper(strings.TrimSpace(req.Code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}
	created, err := s.coinRepository.Create(&coinRepository.Coin{
		Code:            code,
		Name:            strings.TrimSpace(req.Name),
		FullName:        strings.TrimSpace(req.FullName),
		Icon:            strings.TrimSpace(req.Icon),
		ChainName:       strings.TrimSpace(req.ChainName),
		ContractAddress: strings.TrimSpace(req.ContractAddress),
		Decimals:        req.Decimals,
		PricePrecision:  req.PricePrecision,
		AmountPrecision: req.AmountPrecision,
		MinWithdrawal:   req.MinWithdrawal,
		MaxWithdrawal:   req.MaxWithdrawal,
		WithdrawalFee:   req.WithdrawalFee,
		DepositEnable:   req.DepositEnable,
		WithdrawEnable:  req.WithdrawEnable,
		TradeEnable:     req.TradeEnable,
		Status:          normalizeCoinStatus(req.Status),
		SortOrder:       req.SortOrder,
		Description:     strings.TrimSpace(req.Description),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinDTO](created), nil
}

func (s *CoinService) UpdateCoin(id uint, req *coinDTO.UpdateCoinDTO) (*coinDTO.CoinDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinRepository.FindById(id)
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
	if req.ChainName != nil {
		entity.ChainName = strings.TrimSpace(*req.ChainName)
	}
	if req.ContractAddress != nil {
		entity.ContractAddress = strings.TrimSpace(*req.ContractAddress)
	}
	if req.Decimals != nil {
		entity.Decimals = *req.Decimals
	}
	if req.PricePrecision != nil {
		entity.PricePrecision = *req.PricePrecision
	}
	if req.AmountPrecision != nil {
		entity.AmountPrecision = *req.AmountPrecision
	}
	if req.MinWithdrawal != nil {
		entity.MinWithdrawal = *req.MinWithdrawal
	}
	if req.MaxWithdrawal != nil {
		entity.MaxWithdrawal = *req.MaxWithdrawal
	}
	if req.WithdrawalFee != nil {
		entity.WithdrawalFee = *req.WithdrawalFee
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
	if req.Status != nil {
		entity.Status = normalizeCoinStatus(*req.Status)
	}
	if req.SortOrder != nil {
		entity.SortOrder = *req.SortOrder
	}
	if req.Description != nil {
		entity.Description = strings.TrimSpace(*req.Description)
	}
	saved, err := s.coinRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinDTO](saved), nil
}

func (s *CoinService) DeleteCoin(id uint) error {
	entity, err := s.coinRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.coinRepository.SaveOrUpdate(entity)
	return err
}

func (s *CoinService) ListCoinPairs(query coinDTO.CoinPairQueryDTO) (*baseDTO.PageDTO[coinDTO.CoinPairDTO], error) {
	if s.coinPairRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeCoinPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.coinPairRepository.Db.Model(&coinRepository.CoinPair{}).Where("active = ?", 1)
	if value := strings.TrimSpace(query.Symbol); value != "" {
		dbQuery = dbQuery.Where("symbol LIKE ?", "%"+strings.ToUpper(value)+"%")
	}
	if value := strings.TrimSpace(query.BaseCoinCode); value != "" {
		dbQuery = dbQuery.Where("base_coin_code = ?", strings.ToUpper(value))
	}
	if value := strings.TrimSpace(query.QuoteCoinCode); value != "" {
		dbQuery = dbQuery.Where("quote_coin_code = ?", strings.ToUpper(value))
	}
	if value := strings.TrimSpace(query.Status); value != "" {
		dbQuery = dbQuery.Where("status = ?", value)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*coinRepository.CoinPair
	if err := dbQuery.Order("sort_order DESC, id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[coinDTO.CoinPairDTO](entities)), nil
}

func (s *CoinService) CreateCoinPair(req *coinDTO.CreateCoinPairDTO) (*coinDTO.CoinPairDTO, error) {
	if s.coinPairRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	created, err := s.coinPairRepository.Create(&coinRepository.CoinPair{
		Symbol:          symbol,
		BaseCoinID:      req.BaseCoinID,
		BaseCoinCode:    strings.ToUpper(strings.TrimSpace(req.BaseCoinCode)),
		QuoteCoinID:     req.QuoteCoinID,
		QuoteCoinCode:   strings.ToUpper(strings.TrimSpace(req.QuoteCoinCode)),
		PricePrecision:  req.PricePrecision,
		AmountPrecision: req.AmountPrecision,
		MinAmount:       req.MinAmount,
		MaxAmount:       req.MaxAmount,
		MinTotal:        req.MinTotal,
		TakerFeeRate:    req.TakerFeeRate,
		MakerFeeRate:    req.MakerFeeRate,
		Status:          normalizePairStatus(req.Status),
		SortOrder:       req.SortOrder,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinPairDTO](created), nil
}

func (s *CoinService) UpdateCoinPair(id uint, req *coinDTO.UpdateCoinPairDTO) (*coinDTO.CoinPairDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinPairRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.PricePrecision != nil {
		entity.PricePrecision = *req.PricePrecision
	}
	if req.AmountPrecision != nil {
		entity.AmountPrecision = *req.AmountPrecision
	}
	if req.MinAmount != nil {
		entity.MinAmount = *req.MinAmount
	}
	if req.MaxAmount != nil {
		entity.MaxAmount = *req.MaxAmount
	}
	if req.MinTotal != nil {
		entity.MinTotal = *req.MinTotal
	}
	if req.TakerFeeRate != nil {
		entity.TakerFeeRate = *req.TakerFeeRate
	}
	if req.MakerFeeRate != nil {
		entity.MakerFeeRate = *req.MakerFeeRate
	}
	if req.Status != nil {
		entity.Status = normalizePairStatus(*req.Status)
	}
	if req.SortOrder != nil {
		entity.SortOrder = *req.SortOrder
	}
	saved, err := s.coinPairRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinPairDTO](saved), nil
}

func (s *CoinService) DeleteCoinPair(id uint) error {
	entity, err := s.coinPairRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.coinPairRepository.SaveOrUpdate(entity)
	return err
}

func (s *CoinService) ListOnlinePairs() ([]*coinDTO.CoinPairDTO, error) {
	rows, err := s.coinPairRepository.ListOnlinePairs()
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinDTO.CoinPairDTO](rows), nil
}

func (s *CoinService) UpsertPrice(req *coinDTO.CreateCoinPriceDTO) (*coinDTO.CoinPriceDTO, error) {
	if s.coinPriceRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	created, err := s.coinPriceRepository.Create(&coinRepository.CoinPrice{
		CoinID:    req.CoinID,
		CoinCode:  strings.ToUpper(strings.TrimSpace(req.CoinCode)),
		QuoteCode: strings.ToUpper(strings.TrimSpace(req.QuoteCode)),
		Price:     req.Price,
		Change24h: req.Change24h,
		Volume24h: req.Volume24h,
		High24h:   req.High24h,
		Low24h:    req.Low24h,
		Source:    strings.TrimSpace(req.Source),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinPriceDTO](created), nil
}

func (s *CoinService) GetLatestPrice(coinCode, quoteCode string) (*coinDTO.CoinPriceDTO, error) {
	entity, err := s.coinPriceRepository.FindLatestByCode(
		strings.ToUpper(strings.TrimSpace(coinCode)),
		strings.ToUpper(strings.TrimSpace(quoteCode)),
	)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinDTO.CoinPriceDTO](entity), nil
}

func normalizeCoinPage(page, pageIndex, pageSize int) (int, int) {
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

func normalizeCoinStatus(status string) string {
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

func normalizePairStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "online":
		return "online"
	case "offline":
		return "offline"
	case "halt":
		return "halt"
	default:
		return "online"
	}
}
