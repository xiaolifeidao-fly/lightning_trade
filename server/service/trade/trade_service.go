package trade

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	"math/rand"
	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"strings"
	"time"

	"gorm.io/gorm"
)

type TradeService struct {
	tradeOrderRepository       *tradeRepository.TradeOrderRepository
	tradeMatchRepository       *tradeRepository.TradeMatchRepository
	tradeKlineRepository       *tradeRepository.TradeKlineRepository
	tradeDetailRepository      *tradeRepository.TradeDetailRepository
	tradeUserSummaryRepository *tradeRepository.TradeUserSummaryRepository
	tradeUserPnlRepository     *tradeRepository.TradeUserPnlRepository
}

func NewTradeService() *TradeService {
	return &TradeService{
		tradeOrderRepository:       db.GetRepository[tradeRepository.TradeOrderRepository](),
		tradeMatchRepository:       db.GetRepository[tradeRepository.TradeMatchRepository](),
		tradeKlineRepository:       db.GetRepository[tradeRepository.TradeKlineRepository](),
		tradeDetailRepository:      db.GetRepository[tradeRepository.TradeDetailRepository](),
		tradeUserSummaryRepository: db.GetRepository[tradeRepository.TradeUserSummaryRepository](),
		tradeUserPnlRepository:     db.GetRepository[tradeRepository.TradeUserPnlRepository](),
	}
}

func (s *TradeService) EnsureTable() error {
	if err := s.tradeOrderRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.tradeMatchRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.tradeKlineRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.tradeDetailRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.tradeUserSummaryRepository.EnsureTable(); err != nil {
		return err
	}
	return s.tradeUserPnlRepository.EnsureTable()
}

func (s *TradeService) ListOrders(query tradeDTO.TradeOrderQueryDTO) (*baseDTO.PageDTO[tradeDTO.TradeOrderDTO], error) {
	if s.tradeOrderRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeTradePage(query.Page, query.PageIndex, query.PageSize)
	total, err := s.tradeOrderRepository.CountOrdersByQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.tradeOrderRepository.ListOrdersByQuery(query, pageIndex, pageSize)
	if err != nil {
		return nil, err
	}
	list := make([]*tradeDTO.TradeOrderDTO, 0, len(rows))
	for i := range rows {
		row := rows[i]
		list = append(list, &tradeDTO.TradeOrderDTO{
			BaseDTO: baseDTO.BaseDTO{
				Id:          row.Id,
				Active:      row.Active,
				CreatedTime: row.CreatedTime,
				CreatedBy:   row.CreatedBy,
				UpdatedTime: row.UpdatedTime,
				UpdatedBy:   row.UpdatedBy,
			},
			PlatformID:     row.PlatformID,
			PlatformCode:   row.PlatformCode,
			TradeCategory:  row.TradeCategory,
			TradeType:      row.TradeType,
			OrderNo:        row.OrderNo,
			UserID:         row.UserID,
			Symbol:         row.Symbol,
			BaseCoinCode:   row.BaseCoinCode,
			QuoteCoinCode:  row.QuoteCoinCode,
			Side:           row.Side,
			OrderType:      row.OrderType,
			Price:          row.Price,
			Amount:         row.Amount,
			Total:          row.Total,
			FilledAmount:   row.FilledAmount,
			FilledTotal:    row.FilledTotal,
			AvgFilledPrice: row.AvgFilledPrice,
			FeeAmount:      row.FeeAmount,
			Status:         row.Status,
			SubmittedTime:  row.SubmittedTime,
			FinishedTime:   row.FinishedTime,
		})
	}
	return baseDTO.BuildPage(int(total), list), nil
}

func (s *TradeService) GetOrderByOrderNo(orderNo string) (*tradeDTO.TradeOrderDTO, error) {
	entity, err := s.tradeOrderRepository.FindByOrderNo(strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	return db.ToDTO[tradeDTO.TradeOrderDTO](entity), nil
}

func (s *TradeService) ListOpenOrdersByUser(userID uint64, symbol string) ([]*tradeDTO.TradeOrderDTO, error) {
	rows, err := s.tradeOrderRepository.ListOpenOrdersByUser(userID, symbol)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[tradeDTO.TradeOrderDTO](rows), nil
}

func (s *TradeService) PlaceOrder(req *tradeDTO.CreateTradeOrderDTO) (*tradeDTO.TradeOrderDTO, error) {
	if s.tradeOrderRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("userId is required")
	}
	symbol := strings.ToUpper(strings.TrimSpace(req.Symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	side := normalizeOrderSide(req.Side)
	if side == "" {
		return nil, fmt.Errorf("invalid side")
	}
	orderType := normalizeOrderType(req.OrderType)
	if orderType == "" {
		return nil, fmt.Errorf("invalid orderType")
	}
	if req.Amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if orderType == "limit" && req.Price <= 0 {
		return nil, fmt.Errorf("price must be positive for limit order")
	}

	base, quote := splitSymbol(symbol)
	total := req.Price * req.Amount

	created, err := s.tradeOrderRepository.Create(&tradeRepository.TradeOrder{
		PlatformID:    req.PlatformID,
		PlatformCode:  strings.ToLower(strings.TrimSpace(req.PlatformCode)),
		TradeCategory: normalizeTradeCategory(req.TradeCategory),
		TradeType:     normalizeTradeType(req.TradeType),
		OrderNo:       generateOrderNo(),
		UserID:        req.UserID,
		Symbol:        symbol,
		BaseCoinCode:  base,
		QuoteCoinCode: quote,
		Side:          side,
		OrderType:     orderType,
		Price:         req.Price,
		Amount:        req.Amount,
		Total:         total,
		StopPrice:     req.StopPrice,
		Status:        "pending",
		TimeInForce:   normalizeTimeInForce(req.TimeInForce),
		Source:        strings.TrimSpace(req.Source),
		ClientOrderID: strings.TrimSpace(req.ClientOrderID),
		FeeCoinCode:   quote,
		SubmittedTime: time.Now(),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[tradeDTO.TradeOrderDTO](created), nil
}

func (s *TradeService) CancelOrder(req *tradeDTO.CancelTradeOrderDTO) (*tradeDTO.TradeOrderDTO, error) {
	if req == nil || strings.TrimSpace(req.OrderNo) == "" {
		return nil, fmt.Errorf("orderNo is required")
	}
	entity, err := s.tradeOrderRepository.FindByOrderNo(strings.TrimSpace(req.OrderNo))
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if entity.Status != "pending" && entity.Status != "partial" {
		return nil, fmt.Errorf("order can not be canceled in status %s", entity.Status)
	}
	entity.Status = "canceled"
	entity.CancelReason = strings.TrimSpace(req.Reason)
	entity.FinishedTime = time.Now()
	saved, err := s.tradeOrderRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[tradeDTO.TradeOrderDTO](saved), nil
}

func (s *TradeService) UpdateOrderFill(orderNo string, req *tradeDTO.UpdateTradeOrderFillDTO) (*tradeDTO.TradeOrderDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.tradeOrderRepository.FindByOrderNo(strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.FilledAmount != nil {
		entity.FilledAmount = *req.FilledAmount
	}
	if req.FilledTotal != nil {
		entity.FilledTotal = *req.FilledTotal
	}
	if req.AvgFilledPrice != nil {
		entity.AvgFilledPrice = *req.AvgFilledPrice
	}
	if req.FeeAmount != nil {
		entity.FeeAmount = *req.FeeAmount
	}
	if req.Status != nil {
		entity.Status = normalizeOrderStatus(*req.Status)
		if entity.Status == "filled" || entity.Status == "canceled" || entity.Status == "rejected" {
			entity.FinishedTime = time.Now()
		}
	}
	saved, err := s.tradeOrderRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[tradeDTO.TradeOrderDTO](saved), nil
}

func (s *TradeService) RecordMatch(req *tradeDTO.CreateTradeMatchDTO) (*tradeDTO.TradeMatchDTO, error) {
	if s.tradeMatchRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	matchedTime := req.MatchedTime
	if matchedTime.IsZero() {
		matchedTime = time.Now()
	}
	created, err := s.tradeMatchRepository.Create(&tradeRepository.TradeMatch{
		PlatformID:   req.PlatformID,
		PlatformCode: strings.ToLower(strings.TrimSpace(req.PlatformCode)),
		TradeNo:      generateTradeNo(),
		Symbol:       strings.ToUpper(strings.TrimSpace(req.Symbol)),
		TakerOrderNo: strings.TrimSpace(req.TakerOrderNo),
		MakerOrderNo: strings.TrimSpace(req.MakerOrderNo),
		TakerUserID:  req.TakerUserID,
		MakerUserID:  req.MakerUserID,
		Side:         normalizeOrderSide(req.Side),
		Price:        req.Price,
		Amount:       req.Amount,
		Total:        req.Total,
		TakerFee:     req.TakerFee,
		MakerFee:     req.MakerFee,
		MatchedTime:  matchedTime,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[tradeDTO.TradeMatchDTO](created), nil
}

func (s *TradeService) ListRecentMatches(symbol string, limit int) ([]*tradeDTO.TradeMatchDTO, error) {
	rows, err := s.tradeMatchRepository.ListBySymbol(symbol, limit)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[tradeDTO.TradeMatchDTO](rows), nil
}

func (s *TradeService) ListUserMatches(userID uint64, symbol string, limit int) ([]*tradeDTO.TradeMatchDTO, error) {
	rows, err := s.tradeMatchRepository.ListByUserID(userID, symbol, limit)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[tradeDTO.TradeMatchDTO](rows), nil
}

func (s *TradeService) ListKlines(query tradeDTO.TradeKlineQueryDTO) ([]*tradeDTO.TradeKlineDTO, error) {
	if strings.TrimSpace(query.Symbol) == "" {
		return nil, fmt.Errorf("symbol is required")
	}
	interval := strings.TrimSpace(query.Interval)
	if interval == "" {
		interval = "1m"
	}
	rows, err := s.tradeKlineRepository.ListBySymbolInterval(query.Symbol, interval, query.Limit)
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[tradeDTO.TradeKlineDTO](rows), nil
}

func normalizeTradePage(page, pageIndex, pageSize int) (int, int) {
	if pageIndex <= 0 {
		pageIndex = page
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 500 {
		pageSize = 500
	}
	return pageIndex, pageSize
}

func normalizeOrderSide(side string) string {
	switch strings.ToLower(strings.TrimSpace(side)) {
	case "buy", "bid":
		return "buy"
	case "sell", "ask":
		return "sell"
	default:
		return ""
	}
}

func normalizeOrderType(orderType string) string {
	switch strings.ToLower(strings.TrimSpace(orderType)) {
	case "", "limit":
		return "limit"
	case "market":
		return "market"
	case "stop_limit", "stoplimit":
		return "stop_limit"
	default:
		return ""
	}
}

func normalizeOrderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pending":
		return "pending"
	case "partial":
		return "partial"
	case "filled":
		return "filled"
	case "canceled", "cancelled":
		return "canceled"
	case "rejected":
		return "rejected"
	default:
		return "pending"
	}
}

func normalizeTimeInForce(tif string) string {
	switch strings.ToUpper(strings.TrimSpace(tif)) {
	case "", "GTC":
		return "GTC"
	case "IOC":
		return "IOC"
	case "FOK":
		return "FOK"
	default:
		return "GTC"
	}
}

func splitSymbol(symbol string) (string, string) {
	if idx := strings.Index(symbol, "-"); idx > 0 {
		return symbol[:idx], symbol[idx+1:]
	}
	if idx := strings.Index(symbol, "/"); idx > 0 {
		return symbol[:idx], symbol[idx+1:]
	}
	return symbol, ""
}

func generateOrderNo() string {
	return fmt.Sprintf("O%d%04d", time.Now().UnixNano()/1e6, rand.Intn(10000))
}

func generateTradeNo() string {
	return fmt.Sprintf("T%d%04d", time.Now().UnixNano()/1e6, rand.Intn(10000))
}

func normalizeTradeCategory(category string) string {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "spot":
		return "spot"
	case "futures":
		return "futures"
	case "margin":
		return "margin"
	default:
		return "spot"
	}
}

func normalizeOpenDirection(dir string) string {
	switch strings.ToLower(strings.TrimSpace(dir)) {
	case "long", "buy":
		return "long"
	case "short", "sell":
		return "short"
	default:
		return "long"
	}
}

func normalizeLeverage(leverage float64) float64 {
	if leverage <= 0 {
		return 1
	}
	return leverage
}

func normalizeTradeType(tradeType string) string {
	switch strings.ToLower(strings.TrimSpace(tradeType)) {
	case "simulation", "sim", "demo":
		return "simulation"
	case "", "real", "live":
		return "real"
	default:
		return "real"
	}
}

// TradeDetail 相关方法

func (s *TradeService) CreateTradeDetail(req *tradeDTO.CreateTradeDetailDTO) (*tradeDTO.TradeDetailDTO, error) {
	if s.tradeDetailRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.UserID == 0 {
		return nil, fmt.Errorf("userId is required")
	}
	tradeTime := req.TradeTime
	if tradeTime.IsZero() {
		tradeTime = time.Now()
	}
	created, err := s.tradeDetailRepository.Create(&tradeRepository.TradeDetail{
		PlatformID:       req.PlatformID,
		PlatformCode:     strings.ToLower(strings.TrimSpace(req.PlatformCode)),
		TradeCategory:    normalizeTradeCategory(req.TradeCategory),
		TradeType:        normalizeTradeType(req.TradeType),
		UserID:           req.UserID,
		OrderNo:          strings.TrimSpace(req.OrderNo),
		TradeNo:          strings.TrimSpace(req.TradeNo),
		Symbol:           strings.ToUpper(strings.TrimSpace(req.Symbol)),
		CoinCode:         strings.ToUpper(strings.TrimSpace(req.CoinCode)),
		Side:             normalizeOrderSide(req.Side),
		OpenDirection:    normalizeOpenDirection(req.OpenDirection),
		AvgOpenPrice:     req.AvgOpenPrice,
		LiquidationPrice: req.LiquidationPrice,
		Leverage:         normalizeLeverage(req.Leverage),
		Margin:           req.Margin,
		UserBalanceOpen:  req.UserBalanceOpen,
		Price:            req.Price,
		Amount:           req.Amount,
		Total:            req.Total,
		Fee:              req.Fee,
		Pnl:              req.Pnl,
		PnlRate:          req.PnlRate,
		TradeTime:        tradeTime,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[tradeDTO.TradeDetailDTO](created), nil
}

func (s *TradeService) ListTradeDetails(query tradeDTO.TradeDetailQueryDTO) (*baseDTO.PageDTO[tradeDTO.TradeDetailDTO], error) {
	if s.tradeDetailRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeTradePage(query.Page, query.PageIndex, query.PageSize)
	total, err := s.tradeDetailRepository.CountByQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.tradeDetailRepository.ListByQuery(query, pageIndex, pageSize)
	if err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[tradeDTO.TradeDetailDTO](rows)), nil
}

func (s *TradeService) ListDetailsByOrderNo(orderNo string) ([]*tradeDTO.TradeDetailDTO, error) {
	rows, err := s.tradeDetailRepository.ListByOrderNo(strings.TrimSpace(orderNo))
	if err != nil {
		return nil, err
	}
	return db.ToDTOs[tradeDTO.TradeDetailDTO](rows), nil
}

// TradeUserSummary 相关方法

func (s *TradeService) ListUserSummary(query tradeDTO.TradeUserSummaryQueryDTO) (*baseDTO.PageDTO[tradeDTO.TradeUserSummaryDTO], error) {
	if s.tradeUserSummaryRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeTradePage(query.Page, query.PageIndex, query.PageSize)
	total, err := s.tradeUserSummaryRepository.CountByQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.tradeUserSummaryRepository.ListByQuery(query, pageIndex, pageSize)
	if err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[tradeDTO.TradeUserSummaryDTO](rows)), nil
}

// TradeUserPnl 相关方法

func (s *TradeService) ListUserPnl(query tradeDTO.TradeUserPnlQueryDTO) (*baseDTO.PageDTO[tradeDTO.TradeUserPnlDTO], error) {
	if s.tradeUserPnlRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeTradePage(query.Page, query.PageIndex, query.PageSize)
	total, err := s.tradeUserPnlRepository.CountByQuery(query)
	if err != nil {
		return nil, err
	}
	rows, err := s.tradeUserPnlRepository.ListByQuery(query, pageIndex, pageSize)
	if err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[tradeDTO.TradeUserPnlDTO](rows)), nil
}
