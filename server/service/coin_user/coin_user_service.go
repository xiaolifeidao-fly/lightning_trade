package coin_user

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	"net/mail"
	coinUserDTO "service/coin_user/dto"
	coinUserRepository "service/coin_user/repository"
	"strings"

	"gorm.io/gorm"
)

type CoinUserService struct {
	coinUserRepository                 *coinUserRepository.CoinUserRepository
	coinUserAssetRepository            *coinUserRepository.CoinUserAssetRepository
	coinUserLoginRecordRepository      *coinUserRepository.CoinUserLoginRecordRepository
	coinUserPositionRepository         *coinUserRepository.CoinUserPositionRepository
	coinUserPositionAnalysisRepository *coinUserRepository.CoinUserPositionAnalysisRepository
}

func NewCoinUserService() *CoinUserService {
	return &CoinUserService{
		coinUserRepository:                 db.GetRepository[coinUserRepository.CoinUserRepository](),
		coinUserAssetRepository:            db.GetRepository[coinUserRepository.CoinUserAssetRepository](),
		coinUserLoginRecordRepository:      db.GetRepository[coinUserRepository.CoinUserLoginRecordRepository](),
		coinUserPositionRepository:         db.GetRepository[coinUserRepository.CoinUserPositionRepository](),
		coinUserPositionAnalysisRepository: db.GetRepository[coinUserRepository.CoinUserPositionAnalysisRepository](),
	}
}

func (s *CoinUserService) EnsureTable() error {
	if err := s.coinUserRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.coinUserAssetRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.coinUserLoginRecordRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.coinUserPositionRepository.EnsureTable(); err != nil {
		return err
	}
	return s.coinUserPositionAnalysisRepository.EnsureTable()
}

func (s *CoinUserService) ListCoinUsers(query coinUserDTO.CoinUserQueryDTO) (*baseDTO.PageDTO[coinUserDTO.CoinUserDTO], error) {
	if s.coinUserRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeCoinUserPage(query.Page, query.PageIndex, query.PageSize)
	total, err := s.coinUserRepository.CountCoinUsersByQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.coinUserRepository.ListCoinUsersByQuery(query, pageIndex, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]*coinUserDTO.CoinUserDTO, 0, len(rows))
	for i := range rows {
		row := rows[i]
		list = append(list, &coinUserDTO.CoinUserDTO{
			BaseDTO: baseDTO.BaseDTO{
				Id:          row.Id,
				Active:      row.Active,
				CreatedTime: row.CreatedTime,
				CreatedBy:   row.CreatedBy,
				UpdatedTime: row.UpdatedTime,
				UpdatedBy:   row.UpdatedBy,
			},
			PlatformID:    row.PlatformID,
			PlatformCode:  row.PlatformCode,
			Account:       row.Account,
			Username:      row.Username,
			Nickname:      row.Nickname,
			Email:         row.Email,
			Phone:         row.Phone,
			Balance:       row.Balance,
			Country:       row.Country,
			KycLevel:      row.KycLevel,
			KycStatus:     row.KycStatus,
			Status:        row.Status,
			InviteCode:    row.InviteCode,
			InviterID:     row.InviterID,
			LastLoginIP:   row.LastLoginIP,
			LastLoginTime: row.LastLoginTime,
			TwoFAEnabled:  row.TwoFAEnabled,
			Remark:        row.Remark,
		})
	}
	return baseDTO.BuildPage(int(total), list), nil
}

func (s *CoinUserService) GetCoinUserByID(id uint) (*coinUserDTO.CoinUserDTO, error) {
	entity, err := s.coinUserRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[coinUserDTO.CoinUserDTO](entity), nil
}

func (s *CoinUserService) CreateCoinUser(req *coinUserDTO.CreateCoinUserDTO) (*coinUserDTO.CoinUserDTO, error) {
	if s.coinUserRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if err := validateCoinUserEmail(req.Email); err != nil {
		return nil, err
	}
	created, err := s.coinUserRepository.Create(&coinUserRepository.CoinUser{
		PlatformID:   req.PlatformID,
		PlatformCode: strings.ToLower(strings.TrimSpace(req.PlatformCode)),
		Account:      strings.TrimSpace(req.Account),
		Username:     username,
		Nickname:     strings.TrimSpace(req.Nickname),
		Email:        strings.TrimSpace(req.Email),
		Phone:        strings.TrimSpace(req.Phone),
		Password:     req.Password,
		SecretKey:    req.SecretKey,
		Balance:      req.Balance,
		Country:      strings.TrimSpace(req.Country),
		InviteCode:   strings.TrimSpace(req.InviteCode),
		InviterID:    req.InviterID,
		KycStatus:    "pending",
		Status:       "active",
		Remark:       strings.TrimSpace(req.Remark),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserDTO](created), nil
}

func (s *CoinUserService) UpdateCoinUser(id uint, req *coinUserDTO.UpdateCoinUserDTO) (*coinUserDTO.CoinUserDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinUserRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.PlatformID != nil {
		entity.PlatformID = *req.PlatformID
	}
	if req.PlatformCode != nil {
		entity.PlatformCode = strings.ToLower(strings.TrimSpace(*req.PlatformCode))
	}
	if req.Account != nil {
		entity.Account = strings.TrimSpace(*req.Account)
	}
	if req.SecretKey != nil {
		entity.SecretKey = *req.SecretKey
	}
	if req.Balance != nil {
		entity.Balance = *req.Balance
	}
	if req.Nickname != nil {
		entity.Nickname = strings.TrimSpace(*req.Nickname)
	}
	if req.Email != nil {
		if err := validateCoinUserEmail(*req.Email); err != nil {
			return nil, err
		}
		entity.Email = strings.TrimSpace(*req.Email)
	}
	if req.Phone != nil {
		entity.Phone = strings.TrimSpace(*req.Phone)
	}
	if req.Country != nil {
		entity.Country = strings.TrimSpace(*req.Country)
	}
	if req.KycLevel != nil {
		entity.KycLevel = *req.KycLevel
	}
	if req.KycStatus != nil {
		entity.KycStatus = normalizeKycStatus(*req.KycStatus)
	}
	if req.Status != nil {
		entity.Status = normalizeCoinUserStatus(*req.Status)
	}
	if req.TwoFAEnabled != nil {
		entity.TwoFAEnabled = *req.TwoFAEnabled
	}
	if req.Remark != nil {
		entity.Remark = strings.TrimSpace(*req.Remark)
	}
	saved, err := s.coinUserRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserDTO](saved), nil
}

func (s *CoinUserService) DeleteCoinUser(id uint) error {
	entity, err := s.coinUserRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.coinUserRepository.SaveOrUpdate(entity)
	return err
}

func (s *CoinUserService) ListUserAssets(userID uint64) ([]*coinUserDTO.CoinUserAssetDTO, error) {
	rows, err := s.coinUserAssetRepository.ListByUserID(userID)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinUserDTO.CoinUserAssetDTO](rows), nil
}

func (s *CoinUserService) UpsertUserAsset(req *coinUserDTO.CreateCoinUserAssetDTO) (*coinUserDTO.CoinUserAssetDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 || req.CoinID == 0 {
		return nil, fmt.Errorf("userId and coinId must be positive")
	}
	existed, err := s.coinUserAssetRepository.FindByUserAndCoin(req.UserID, req.CoinID)
	if err == nil && existed != nil && existed.Id > 0 {
		existed.Address = strings.TrimSpace(req.Address)
		saved, err := s.coinUserAssetRepository.SaveOrUpdate(existed)
		if err != nil {
			return nil, err
		}
		return db.ToDTO[coinUserDTO.CoinUserAssetDTO](saved), nil
	}
	created, err := s.coinUserAssetRepository.Create(&coinUserRepository.CoinUserAsset{
		UserID:         req.UserID,
		CoinID:         req.CoinID,
		CoinCode:       strings.TrimSpace(req.CoinCode),
		Address:        strings.TrimSpace(req.Address),
		WithdrawEnable: 1,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserAssetDTO](created), nil
}

func (s *CoinUserService) UpdateUserAsset(id uint, req *coinUserDTO.UpdateCoinUserAssetDTO) (*coinUserDTO.CoinUserAssetDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinUserAssetRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.Available != nil {
		entity.Available = *req.Available
	}
	if req.Frozen != nil {
		entity.Frozen = *req.Frozen
	}
	if req.Total != nil {
		entity.Total = *req.Total
	}
	if req.Address != nil {
		entity.Address = strings.TrimSpace(*req.Address)
	}
	if req.WithdrawEnable != nil {
		entity.WithdrawEnable = *req.WithdrawEnable
	}
	saved, err := s.coinUserAssetRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserAssetDTO](saved), nil
}

func (s *CoinUserService) RecordLogin(req *coinUserDTO.CreateCoinUserLoginRecordDTO) (*coinUserDTO.CoinUserLoginRecordDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("userId must be positive")
	}
	created, err := s.coinUserLoginRecordRepository.Create(&coinUserRepository.CoinUserLoginRecord{
		UserID:   req.UserID,
		IP:       strings.TrimSpace(req.IP),
		Device:   strings.TrimSpace(req.Device),
		Location: strings.TrimSpace(req.Location),
		Success:  req.Success,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserLoginRecordDTO](created), nil
}

func (s *CoinUserService) ListLoginRecords(userID uint64, limit int) ([]*coinUserDTO.CoinUserLoginRecordDTO, error) {
	rows, err := s.coinUserLoginRecordRepository.ListByUserID(userID, limit)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinUserDTO.CoinUserLoginRecordDTO](rows), nil
}

func (s *CoinUserService) ListUserPositions(userID uint64) ([]*coinUserDTO.CoinUserPositionDTO, error) {
	rows, err := s.coinUserPositionRepository.ListByUserID(userID)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinUserDTO.CoinUserPositionDTO](rows), nil
}

func (s *CoinUserService) ListOpenUserPositions(userID uint64) ([]*coinUserDTO.CoinUserPositionDTO, error) {
	rows, err := s.coinUserPositionRepository.ListOpenByUserID(userID)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinUserDTO.CoinUserPositionDTO](rows), nil
}

func (s *CoinUserService) UpsertUserPosition(req *coinUserDTO.CreateCoinUserPositionDTO) (*coinUserDTO.CoinUserPositionDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("userId must be positive")
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	existed, err := s.coinUserPositionRepository.FindByUserAndSymbol(req.UserID, req.Symbol)
	if err == nil && existed != nil && existed.Id > 0 {
		existed.Symbol = strings.TrimSpace(req.Symbol)
		existed.BaseCoinCode = strings.TrimSpace(req.BaseCoinCode)
		existed.QuoteCoinCode = strings.TrimSpace(req.QuoteCoinCode)
		existed.Amount = req.Amount
		existed.AvgCostPrice = req.AvgCostPrice
		existed.TotalCost = req.TotalCost
		if req.Amount <= 0 {
			existed.Status = "closed"
		} else {
			existed.Status = "open"
		}
		saved, err := s.coinUserPositionRepository.SaveOrUpdate(existed)
		if err != nil {
			return nil, err
		}
		return db.ToDTO[coinUserDTO.CoinUserPositionDTO](saved), nil
	}
	status := "open"
	if req.Amount <= 0 {
		status = "closed"
	}
	created, err := s.coinUserPositionRepository.Create(&coinUserRepository.CoinUserPosition{
		UserID:        req.UserID,
		Symbol:        strings.TrimSpace(req.Symbol),
		BaseCoinCode:  strings.TrimSpace(req.BaseCoinCode),
		QuoteCoinCode: strings.TrimSpace(req.QuoteCoinCode),
		Amount:        req.Amount,
		AvgCostPrice:  req.AvgCostPrice,
		TotalCost:     req.TotalCost,
		Status:        status,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserPositionDTO](created), nil
}

func (s *CoinUserService) UpdateUserPosition(id uint, req *coinUserDTO.UpdateCoinUserPositionDTO) (*coinUserDTO.CoinUserPositionDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinUserPositionRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.Amount != nil {
		entity.Amount = *req.Amount
	}
	if req.AvgCostPrice != nil {
		entity.AvgCostPrice = *req.AvgCostPrice
	}
	if req.TotalCost != nil {
		entity.TotalCost = *req.TotalCost
	}
	if req.Status != nil {
		entity.Status = *req.Status
	}
	saved, err := s.coinUserPositionRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserPositionDTO](saved), nil
}

func (s *CoinUserService) ListPositionAnalysisByUser(userID uint64) ([]*coinUserDTO.CoinUserPositionAnalysisDTO, error) {
	rows, err := s.coinUserPositionAnalysisRepository.ListByUserID(userID)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinUserDTO.CoinUserPositionAnalysisDTO](rows), nil
}

func (s *CoinUserService) ListPositionAnalysisByPosition(positionID uint64) ([]*coinUserDTO.CoinUserPositionAnalysisDTO, error) {
	rows, err := s.coinUserPositionAnalysisRepository.ListByPositionID(positionID)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[coinUserDTO.CoinUserPositionAnalysisDTO](rows), nil
}

func (s *CoinUserService) CreatePositionAnalysis(req *coinUserDTO.CreateCoinUserPositionAnalysisDTO) (*coinUserDTO.CoinUserPositionAnalysisDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("userId must be positive")
	}
	if strings.TrimSpace(req.Symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	created, err := s.coinUserPositionAnalysisRepository.Create(&coinUserRepository.CoinUserPositionAnalysis{
		UserID:           req.UserID,
		PositionID:       req.PositionID,
		Symbol:           strings.TrimSpace(req.Symbol),
		Side:             strings.ToLower(strings.TrimSpace(req.Side)),
		AvgPrice:         req.AvgPrice,
		LiquidationPrice: req.LiquidationPrice,
		Leverage:         req.Leverage,
		Contracts:        req.Contracts,
		Margin:           req.Margin,
		BalanceAtOpen:    req.BalanceAtOpen,
		AiAdvice:         strings.TrimSpace(req.AiAdvice),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserPositionAnalysisDTO](created), nil
}

func (s *CoinUserService) UpdatePositionAnalysis(id uint, req *coinUserDTO.UpdateCoinUserPositionAnalysisDTO) (*coinUserDTO.CoinUserPositionAnalysisDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.coinUserPositionAnalysisRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.AvgPrice != nil {
		entity.AvgPrice = *req.AvgPrice
	}
	if req.LiquidationPrice != nil {
		entity.LiquidationPrice = *req.LiquidationPrice
	}
	if req.Leverage != nil {
		entity.Leverage = *req.Leverage
	}
	if req.Contracts != nil {
		entity.Contracts = *req.Contracts
	}
	if req.Margin != nil {
		entity.Margin = *req.Margin
	}
	if req.BalanceAtOpen != nil {
		entity.BalanceAtOpen = *req.BalanceAtOpen
	}
	if req.AiAdvice != nil {
		entity.AiAdvice = strings.TrimSpace(*req.AiAdvice)
	}
	saved, err := s.coinUserPositionAnalysisRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[coinUserDTO.CoinUserPositionAnalysisDTO](saved), nil
}

func normalizeCoinUserPage(page, pageIndex, pageSize int) (int, int) {
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

func normalizeCoinUserStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "active":
		return "active"
	case "locked":
		return "locked"
	case "frozen":
		return "frozen"
	default:
		return "active"
	}
}

func normalizeKycStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "pending":
		return "pending"
	case "approved":
		return "approved"
	case "rejected":
		return "rejected"
	default:
		return "pending"
	}
}

func validateCoinUserEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("email format is invalid")
	}
	return nil
}
