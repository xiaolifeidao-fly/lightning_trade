package trade

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"context"
	"fmt"
	"math"
	"math/rand"
	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"sort"
	"strconv"
	"strings"
	"time"

	argusTrade "argus_single/pkg/trade"

	"gorm.io/gorm"
)

type TradeService struct {
	tradeOrderRepository            *tradeRepository.TradeOrderRepository
	tradeMatchRepository            *tradeRepository.TradeMatchRepository
	tradeKlineRepository            *tradeRepository.TradeKlineRepository
	tradeDetailRepository           *tradeRepository.TradeDetailRepository
	tradeUserSummaryRepository      *tradeRepository.TradeUserSummaryRepository
	tradeUserPnlRepository          *tradeRepository.TradeUserPnlRepository
	tradeAIPredictionRepository     *tradeRepository.TradeAIPredictionRepository
	tradeStrategyRepository         *tradeRepository.TradeStrategyRepository
	tradeStrategyPositionRepository *tradeRepository.TradeStrategyPositionRepository
}

func NewTradeService() *TradeService {
	return &TradeService{
		tradeOrderRepository:            db.GetRepository[tradeRepository.TradeOrderRepository](),
		tradeMatchRepository:            db.GetRepository[tradeRepository.TradeMatchRepository](),
		tradeKlineRepository:            db.GetRepository[tradeRepository.TradeKlineRepository](),
		tradeDetailRepository:           db.GetRepository[tradeRepository.TradeDetailRepository](),
		tradeUserSummaryRepository:      db.GetRepository[tradeRepository.TradeUserSummaryRepository](),
		tradeUserPnlRepository:          db.GetRepository[tradeRepository.TradeUserPnlRepository](),
		tradeAIPredictionRepository:     db.GetRepository[tradeRepository.TradeAIPredictionRepository](),
		tradeStrategyRepository:         db.GetRepository[tradeRepository.TradeStrategyRepository](),
		tradeStrategyPositionRepository: db.GetRepository[tradeRepository.TradeStrategyPositionRepository](),
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
	if err := s.tradeUserPnlRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.tradeAIPredictionRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.tradeStrategyRepository.EnsureTable(); err != nil {
		return err
	}
	return s.tradeStrategyPositionRepository.EnsureTable()
}

// SaveAIPrediction 落库一条 AI 模拟盘预测（oracle 调用入口，按维度幂等 upsert）。
func (s *TradeService) SaveAIPrediction(dto tradeDTO.TradeAIPredictionSaveDTO) error {
	if s.tradeAIPredictionRepository.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	platformCode := strings.ToLower(strings.TrimSpace(dto.PlatformCode))
	if platformCode == "" {
		platformCode = argusTrade.PlatformBinance
	}
	symbol := strings.ToUpper(strings.TrimSpace(dto.Symbol))
	if symbol == "" {
		return fmt.Errorf("symbol 不能为空")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(dto.CoinCode))
	if coinCode == "" {
		coinCode = strings.TrimSuffix(symbol, "USDT")
	}
	interval := strings.TrimSpace(dto.Interval)
	if interval == "" {
		return fmt.Errorf("interval 不能为空")
	}
	if dto.PredictTime <= 0 {
		return fmt.Errorf("predictTime 不能为空")
	}

	entity := &tradeRepository.TradeAIPrediction{
		PlatformCode: platformCode,
		Symbol:       symbol,
		CoinCode:     coinCode,
		Interval:     interval,
		PredictTime:  time.Unix(dto.PredictTime, 0),
		RefPrice:     dto.RefPrice,
		OpenPrice:    dto.OpenPrice,
		CostMs:       dto.CostMs,
		PredictPrice: dto.PredictPrice,
		PredictHigh:  dto.PredictHigh,
		PredictLow:   dto.PredictLow,
		Invalidation: dto.Invalidation,
		Trend:        strings.TrimSpace(dto.Trend),
		Signal:       strings.TrimSpace(dto.Signal),
		Confidence:   dto.Confidence,
		StopLoss:     dto.StopLoss,
		TakeProfit:   dto.TakeProfit,
		Reason:       dto.Reason,
		RawResponse:  dto.RawResponse,
		Model:        strings.TrimSpace(dto.Model),
		Provider:     strings.TrimSpace(dto.Provider),
	}
	return s.tradeAIPredictionRepository.Upsert(entity)
}

// SaveAIPredictionWithID 落库预测并返回记录 ID，供策略检测门关联 prediction_id。
func (s *TradeService) SaveAIPredictionWithID(dto tradeDTO.TradeAIPredictionSaveDTO) (int64, error) {
	if s.tradeAIPredictionRepository.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	platformCode := strings.ToLower(strings.TrimSpace(dto.PlatformCode))
	if platformCode == "" {
		platformCode = argusTrade.PlatformBinance
	}
	symbol := strings.ToUpper(strings.TrimSpace(dto.Symbol))
	if symbol == "" {
		return 0, fmt.Errorf("symbol 不能为空")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(dto.CoinCode))
	if coinCode == "" {
		coinCode = strings.TrimSuffix(symbol, "USDT")
	}
	interval := strings.TrimSpace(dto.Interval)
	if interval == "" {
		return 0, fmt.Errorf("interval 不能为空")
	}
	if dto.PredictTime <= 0 {
		return 0, fmt.Errorf("predictTime 不能为空")
	}
	entity := &tradeRepository.TradeAIPrediction{
		PlatformCode: platformCode,
		Symbol:       symbol,
		CoinCode:     coinCode,
		Interval:     interval,
		PredictTime:  time.Unix(dto.PredictTime, 0),
		RefPrice:     dto.RefPrice,
		OpenPrice:    dto.OpenPrice,
		CostMs:       dto.CostMs,
		PredictPrice: dto.PredictPrice,
		PredictHigh:  dto.PredictHigh,
		PredictLow:   dto.PredictLow,
		Invalidation: dto.Invalidation,
		Trend:        strings.TrimSpace(dto.Trend),
		Signal:       strings.TrimSpace(dto.Signal),
		Confidence:   dto.Confidence,
		StopLoss:     dto.StopLoss,
		TakeProfit:   dto.TakeProfit,
		Reason:       dto.Reason,
		RawResponse:  dto.RawResponse,
		Model:        strings.TrimSpace(dto.Model),
		Provider:     strings.TrimSpace(dto.Provider),
	}
	if err := s.tradeAIPredictionRepository.Upsert(entity); err != nil {
		return 0, err
	}
	return int64(entity.Id), nil
}

// SettleDuePredictions 把已到期(predict_time<=now)但尚未结算的预测回填两类指标：
//   - 端点指标：用 predict_time 时刻的真实价算误差(error_pct/abs_error_pct)与方向命中(direction_hit)，衡量数值精度。
//   - 区间触达指标：遍历 [created_time, predict_time] 的 1m K线，算 MFE/MAE 与先触止盈/止损(first_hit)，衡量信号可交易性。
//
// 滚动预测语义：predict_time 不落在 K 线边界上，故用 1m K 线就近取价。返回本次结算条数。
func (s *TradeService) SettleDuePredictions(ctx context.Context, batchLimit int) (int, error) {
	if s.tradeAIPredictionRepository.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	if batchLimit <= 0 {
		batchLimit = 200
	}
	rows, err := s.tradeAIPredictionRepository.ListUnsettled(time.Now(), batchLimit)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	// 同 平台×交易对 只拉一次 1m K线，按 predict_time 就近取价并评估区间触达。
	type klineKey struct{ platform, symbol string }
	klineCache := make(map[klineKey][]argusTrade.MarketKline)

	// 预计算每个交易对在本批次中最早的 created_time，用于动态决定 1m K线拉取条数：
	// 区间触达需覆盖 [created_time, predict_time]，长周期(如 1d)窗口可达 24h，固定 1000 根(≈16.6h)会丢早段。
	earliest := make(map[klineKey]time.Time)
	for _, pred := range rows {
		key := klineKey{platform: pred.PlatformCode, symbol: pred.Symbol}
		if t, ok := earliest[key]; !ok || pred.CreatedTime.Before(t) {
			earliest[key] = pred.CreatedTime
		}
	}

	settled := 0
	for _, pred := range rows {
		key := klineKey{platform: pred.PlatformCode, symbol: pred.Symbol}
		klines, cached := klineCache[key]
		if !cached {
			fetched, ferr := argusTrade.GetKlinesByPlatform(ctx, pred.PlatformCode, argusTrade.MarketKlineRequest{
				Symbol:   pred.Symbol,
				Interval: "1m",
				Limit:    settleKlineLimit(earliest[key]),
			})
			if ferr != nil {
				// 该交易对拉取失败：本轮跳过，留待下次结算。
				klineCache[key] = nil
				continue
			}
			klines = fetched
			klineCache[key] = klines
		}
		if len(klines) == 0 {
			continue
		}

		actual, ok := closePriceAt(klines, pred.PredictTime)
		if !ok {
			// 真实价尚不可得（1m K线还没覆盖到该时刻）：留待下次。
			continue
		}

		st := tradeRepository.PredictionSettlement{ActualPrice: actual, FirstHit: "none"}
		// 端点指标：误差%。
		if actual != 0 {
			st.ErrorPct = (pred.PredictPrice - actual) / actual * 100
			st.AbsErrorPct = math.Abs(st.ErrorPct)
		}
		// 方向命中——直接以预测方向(trend)与真实涨跌方向比对，不再从点位符号反推：
		//   long→真实价高于 ref 即命中，short→低于 ref 即命中，neutral 不计方向命中。
		actualDir := signOf(actual - pred.RefPrice)
		if (pred.Trend == "long" && actualDir > 0) || (pred.Trend == "short" && actualDir < 0) {
			st.DirectionHit = 1
		}
		// 区间触达指标：遍历 [created_time, predict_time] 的 1m K线，算 MFE/MAE 与先触达，并取区间真实最高/最低价。
		st.MaxFavorablePct, st.MaxAdversePct, st.FirstHit, st.ActualHigh, st.ActualLow = evalPredictionWindow(
			klines, pred.CreatedTime, pred.PredictTime, pred.RefPrice, pred.PredictPrice, pred.TakeProfit, pred.StopLoss)

		// 区间预测质量：预测最高/最低价 vs 区间真实最高/最低价。
		//   - high/low_error_pct：有符号误差%(预测-真实)/真实，正=高估、负=低估。
		//   - band_contain：真实波动 [actual_low, actual_high] 是否被预测区间 [predict_low, predict_high] 完整覆盖。
		if st.ActualHigh > 0 && pred.PredictHigh > 0 {
			st.HighErrorPct = (pred.PredictHigh - st.ActualHigh) / st.ActualHigh * 100
		}
		if st.ActualLow > 0 && pred.PredictLow > 0 {
			st.LowErrorPct = (pred.PredictLow - st.ActualLow) / st.ActualLow * 100
		}
		if pred.PredictHigh > 0 && pred.PredictLow > 0 && st.ActualHigh > 0 && st.ActualLow > 0 &&
			pred.PredictHigh >= st.ActualHigh && pred.PredictLow <= st.ActualLow {
			st.BandContain = 1
		}

		// 失效位结算：窗口内真实价是否触及失效价位，衡量"方向判断是否被证伪"。
		//   long 失效位在下方→最低价跌破即失效；short 失效位在上方→最高价突破即失效。
		//   未给失效位记 -1(区别于"给了但未触发"的 0)。
		st.InvalidationHit = -1
		if pred.Invalidation > 0 {
			st.InvalidationHit = 0
			if (pred.Trend == "long" && st.ActualLow > 0 && st.ActualLow <= pred.Invalidation) ||
				(pred.Trend == "short" && st.ActualHigh > 0 && st.ActualHigh >= pred.Invalidation) {
				st.InvalidationHit = 1
			}
		}

		if err := s.tradeAIPredictionRepository.SettlePrediction(pred.Id, st); err != nil {
			return settled, err
		}
		settled++
	}
	return settled, nil
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

func (s *TradeService) GetSimulationAnalysis(ctx context.Context, query tradeDTO.TradeSimulationAnalysisQueryDTO) (*tradeDTO.TradeSimulationAnalysisDTO, error) {
	platformCode := strings.ToLower(strings.TrimSpace(query.PlatformCode))
	if platformCode == "" {
		platformCode = argusTrade.PlatformBinance
	}
	if platformCode != argusTrade.PlatformBinance {
		return nil, fmt.Errorf("AI模拟盘暂时只支持币安")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(query.CoinCode))
	if coinCode == "" {
		coinCode = "BTC"
	}
	// interval = K线展示周期（默认 1m）；predictInterval = 预测时间间隔/horizon（默认 5m）。
	interval := strings.TrimSpace(query.Interval)
	if interval == "" {
		interval = "1m"
	}
	limit := query.Limit
	if limit <= 0 {
		limit = 64
	}
	if limit > 160 {
		limit = 160
	}

	symbol := coinCode + "USDT"

	realKlines, err := s.fetchSimulationKlines(ctx, platformCode, symbol, interval, limit)
	if err != nil {
		realKlines, err = s.listSimulationKlinesFromDB(symbol, interval, limit)
		if err != nil {
			return nil, err
		}
	}

	// 每个预测周期(horizon)各画一条预测线：分别按周期拉取预测、与真实 K 线对齐，
	// 已到期点位算拟合度并入复核表，未到期点位作为图表右侧的「未来预测」延伸展示。
	displayDurSec := int64(intervalToDuration(interval) / time.Second)
	series := make([]tradeDTO.TradeSimulationSeriesDTO, 0, len(simulationHorizons))
	totalMatch := 0
	totalDiff := 0
	totalTouch := 0
	aggTotalDiffRate := 0.0
	aggMaxDiffRate := 0.0
	var aggLastRun time.Time

	if len(realKlines) > 0 {
		startTime := time.Unix(realKlines[0].Timestamp, 0)
		lastRealTs := realKlines[len(realKlines)-1].Timestamp
		for _, h := range simulationHorizons {
			// 终点放宽到「现在 + 一个预测周期」，把尚未到期(predict_time>now)的未来预测点也取出来。
			endTime := time.Now().Add(intervalToDuration(h.value)).Add(intervalToDuration(interval))
			predRows, err := s.tradeAIPredictionRepository.ListByCoinIntervalTimeRange(platformCode, coinCode, h.value, startTime, endTime)
			if err != nil {
				return nil, err
			}
			one := buildSimulationSeries(realKlines, predRows, displayDurSec, lastRealTs, h)
			series = append(series, one.TradeSimulationSeriesDTO)
			totalMatch += one.MatchCount
			totalDiff += one.DiffCount
			totalTouch += one.TouchCount
			aggTotalDiffRate += one.sumDiffRate
			if one.MaxDiffRate > aggMaxDiffRate {
				aggMaxDiffRate = one.MaxDiffRate
			}
			if one.lastRun.After(aggLastRun) {
				aggLastRun = one.lastRun
			}
		}
	}

	avgDiffRate := 0.0
	// 拟合度汇总只统计各周期已到期、有真实价对比的点位。
	if n := totalMatch + totalDiff; n > 0 {
		avgDiffRate = math.Round(aggTotalDiffRate/float64(n)*10000) / 100
	}
	lastRunTime := ""
	if !aggLastRun.IsZero() {
		lastRunTime = aggLastRun.Format("2006-01-02 15:04:05")
	}

	return &tradeDTO.TradeSimulationAnalysisDTO{
		PlatformCode: platformCode,
		CoinCode:     coinCode,
		Symbol:       symbol,
		Interval:     interval,
		LastRunTime:  lastRunTime,
		MatchCount:   totalMatch,
		DiffCount:    totalDiff,
		TouchCount:   totalTouch,
		AvgDiffRate:  avgDiffRate,
		MaxDiffRate:  math.Round(aggMaxDiffRate*100) / 100,
		PlatformOptions: []tradeDTO.TradeAnalysisOptionDTO{
			{Label: "Binance", Value: "binance"},
		},
		// AI 预测目前只跑 BTC（oracle.coins=BTC），其它币种无预测数据，因此只暴露 BTC。
		CoinOptions: []tradeDTO.TradeAnalysisOptionDTO{
			{Label: "BTC / USDT", Value: "BTC"},
		},
		RealKlines: realKlines,
		Series:     series,
	}, nil
}

// simulationHorizon 描述一个预测周期(horizon)及其中文显示名。
type simulationHorizon struct {
	value string
	label string
}

// simulationHorizons 与 oracle.intervals 保持一致：oracle 实际生成的预测周期。
var simulationHorizons = []simulationHorizon{
	{value: "15m", label: "15分钟"},
	{value: "1h", label: "1小时"},
	{value: "4h", label: "4小时"},
	{value: "1d", label: "1日"},
}

// simulationSeriesResult 在 DTO 基础上多带几个聚合用中间值，供上层汇总。
type simulationSeriesResult struct {
	tradeDTO.TradeSimulationSeriesDTO
	sumDiffRate float64
	lastRun     time.Time
}

// buildSimulationSeries 把单个预测周期的预测行与真实 K 线对齐，产出一条预测线的
// 已到期复核点(markers/aiPoints) 与未到期的未来预测点。
func buildSimulationSeries(
	realKlines []tradeDTO.TradeSimulationKlinePointDTO,
	predRows []*tradeRepository.TradeAIPrediction,
	displayDurSec int64,
	lastRealTs int64,
	h simulationHorizon,
) simulationSeriesResult {
	predByTs := make(map[int64]*tradeRepository.TradeAIPrediction, len(predRows))
	for _, pred := range predRows {
		ts := floorUnixToInterval(pred.PredictTime.Unix(), displayDurSec)
		// 同一根 K 线若命中多条预测，保留执行时间最新的一条。
		if existing, ok := predByTs[ts]; !ok || pred.CreatedTime.After(existing.CreatedTime) {
			predByTs[ts] = pred
		}
	}

	aiPoints := make([]tradeDTO.TradeSimulationAIPointDTO, 0, len(predByTs))
	markers := make([]tradeDTO.TradeSimulationDiffPointDTO, 0, len(predByTs))
	totalDiffRate := 0.0
	maxDiffRate := 0.0
	matchCount := 0
	diffCount := 0
	touchCount := 0
	var lastRun time.Time

	predPrice := func(pred *tradeRepository.TradeAIPrediction) float64 {
		if pred.PredictPrice != 0 {
			return pred.PredictPrice
		}
		return pred.RefPrice
	}

	// touchedInWindow 判断从执行到预测时刻之间，真实价格是否「触达」过预测价：
	// 取 [执行时刻, 预测时刻] 区间内所有真实 K 线的最高/最低价，计算预测价到该
	// [最低, 最高] 区间的相对距离——落在区间内距离为 0；落在区间外则取到最近极值
	// (最低或最高)的距离。该距离 <= 0.2% 即视为触达(止盈/限价语义)。
	// touchedInWindow 判断 [执行,预测] 窗口内真实价格区间 [最低,最高] 是否覆盖预测价，
	// 同时返回窗口内真实最低/最高价(无数据时为 0)，供复核表展示。
	touchedInWindow := func(execUnix, predictTs, aiPrice float64) (touched bool, winLow, winHigh float64) {
		startTs := floorUnixToInterval(int64(execUnix), displayDurSec)
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, k := range realKlines {
			if k.Timestamp < startTs || k.Timestamp > int64(predictTs) {
				continue
			}
			if k.LowPrice > 0 && k.LowPrice < lo {
				lo = k.LowPrice
			}
			if k.HighPrice > hi {
				hi = k.HighPrice
			}
		}
		if math.IsInf(lo, 1) || math.IsInf(hi, -1) {
			return false, 0, 0
		}
		if aiPrice == 0 {
			return false, lo, hi
		}
		return aiPrice >= lo && aiPrice <= hi, lo, hi
	}

	for _, realKline := range realKlines {
		pred, ok := predByTs[realKline.Timestamp]
		if !ok {
			continue
		}
		aiPrice := predPrice(pred)
		if aiPrice == 0 {
			continue
		}
		// 已结算的预测用落库时冻结的真实收盘价(actual_price)，避免每次刷新按最新 1m K线重算导致利润浮动。
		closePrice := realKline.ClosePrice
		if pred.Settled == 1 && pred.ActualPrice > 0 {
			closePrice = pred.ActualPrice
		}
		diff := aiPrice - closePrice
		diffRate := 0.0
		if closePrice != 0 {
			diffRate = math.Abs(diff) / closePrice
		}
		totalDiffRate += diffRate
		if diffRate > maxDiffRate {
			maxDiffRate = diffRate
		}
		matched := diffRate <= 0.006
		if matched {
			matchCount++
		} else {
			diffCount++
		}
		touched, winLow, winHigh := touchedInWindow(float64(pred.CreatedTime.Unix()), float64(realKline.Timestamp), aiPrice)
		// 已结算：用落库冻结的区间真实最高/最低价(actual_high/actual_low)，避免刷新按最新K线重算导致区间/触达/利润浮动。
		if pred.Settled == 1 && pred.ActualHigh > 0 && pred.ActualLow > 0 {
			winLow, winHigh = pred.ActualLow, pred.ActualHigh
			touched = aiPrice >= winLow && aiPrice <= winHigh
		}
		if touched {
			touchCount++
		}
		if pred.UpdatedTime.After(lastRun) {
			lastRun = pred.UpdatedTime
		}

		signal := strings.TrimSpace(pred.Signal)
		if signal == "" {
			signal = strings.TrimSpace(pred.Trend)
		}
		createdTime := ""
		if !pred.CreatedTime.IsZero() {
			createdTime = pred.CreatedTime.Format("01-02 15:04")
		}
		aiPoints = append(aiPoints, tradeDTO.TradeSimulationAIPointDTO{
			Time:         realKline.Time,
			Timestamp:    realKline.Timestamp,
			CreatedTime:  createdTime,
			Price:        roundPrice(aiPrice),
			PredictHigh:  roundPrice(pred.PredictHigh),
			PredictLow:   roundPrice(pred.PredictLow),
			Invalidation: roundPrice(pred.Invalidation),
			Signal:       signal,
			Reason:       pred.Reason,
		})

		label := "一致"
		if !matched {
			label = "偏离"
		}
		// 区间命中：AI 预测区间 [PredictLow, PredictHigh] 是否完整覆盖窗口真实波动 [winLow, winHigh]。
		bandContain := pred.PredictHigh > 0 && pred.PredictLow > 0 && winHigh > 0 && winLow > 0 &&
			pred.PredictHigh >= winHigh && pred.PredictLow <= winLow
		// 区间利用率：真实波动宽度 / 预测区间宽度。越接近 1 越紧致；过低说明预测区间报得太宽，覆盖得名不副实。
		bandUtil := 0.0
		if predWidth := pred.PredictHigh - pred.PredictLow; predWidth > 0 && winHigh > 0 && winLow > 0 {
			bandUtil = (winHigh - winLow) / predWidth
		}
		markers = append(markers, tradeDTO.TradeSimulationDiffPointDTO{
			Time:        realKline.Time,
			Timestamp:   realKline.Timestamp,
			CreatedTime: createdTime,
			Trend:       strings.TrimSpace(pred.Trend),
			RefPrice:    roundPrice(pred.RefPrice),
			OpenPrice:   roundPrice(pred.OpenPrice),
			CostMs:      pred.CostMs,
			RealPrice:   roundPrice(closePrice),
			AIPrice:     roundPrice(aiPrice),
			Diff:        roundPrice(diff),
			DiffRate:    math.Round(diffRate*10000) / 100,
			Matched:     matched,
			Touched:     touched,
			WindowHigh:      roundPrice(winHigh),
			WindowLow:       roundPrice(winLow),
			PredictHigh:     roundPrice(pred.PredictHigh),
			PredictLow:      roundPrice(pred.PredictLow),
			Invalidation:    roundPrice(pred.Invalidation),
			InvalidationHit: pred.InvalidationHit,
			BandContain:     bandContain,
			BandUtil:        math.Round(bandUtil*1000) / 1000,
			Label:           label,
			Reason:          pred.Reason,
		})
	}

	// 未到期的未来预测点：predict_time 落在最后一根真实 K 线之后，尚无真实价对比。
	futureTs := make([]int64, 0, len(predByTs))
	for ts := range predByTs {
		if ts > lastRealTs {
			futureTs = append(futureTs, ts)
		}
	}
	sort.Slice(futureTs, func(i, j int) bool { return futureTs[i] < futureTs[j] })
	for _, ts := range futureTs {
		pred := predByTs[ts]
		aiPrice := predPrice(pred)
		if aiPrice == 0 {
			continue
		}
		signal := strings.TrimSpace(pred.Signal)
		if signal == "" {
			signal = strings.TrimSpace(pred.Trend)
		}
		createdTime := ""
		if !pred.CreatedTime.IsZero() {
			createdTime = pred.CreatedTime.Format("01-02 15:04")
		}
		if pred.UpdatedTime.After(lastRun) {
			lastRun = pred.UpdatedTime
		}
		aiPoints = append(aiPoints, tradeDTO.TradeSimulationAIPointDTO{
			Time:         time.Unix(ts, 0).Format("01-02 15:04"),
			Timestamp:    ts,
			CreatedTime:  createdTime,
			Price:        roundPrice(aiPrice),
			PredictHigh:  roundPrice(pred.PredictHigh),
			PredictLow:   roundPrice(pred.PredictLow),
			Invalidation: roundPrice(pred.Invalidation),
			Signal:       signal,
			Reason:       pred.Reason,
		})
	}

	avgDiffRate := 0.0
	if n := matchCount + diffCount; n > 0 {
		avgDiffRate = math.Round(totalDiffRate/float64(n)*10000) / 100
	}
	lastRunTime := ""
	if !lastRun.IsZero() {
		lastRunTime = lastRun.Format("2006-01-02 15:04:05")
	}

	return simulationSeriesResult{
		TradeSimulationSeriesDTO: tradeDTO.TradeSimulationSeriesDTO{
			Interval:    h.value,
			Label:       h.label,
			LastRunTime: lastRunTime,
			MatchCount:  matchCount,
			DiffCount:   diffCount,
			TouchCount:  touchCount,
			AvgDiffRate: avgDiffRate,
			MaxDiffRate: math.Round(maxDiffRate*10000) / 100,
			AIPoints:    aiPoints,
			Markers:     markers,
		},
		sumDiffRate: totalDiffRate,
		lastRun:     lastRun,
	}
}

func (s *TradeService) fetchSimulationKlines(ctx context.Context, platformCode, symbol, interval string, limit int) ([]tradeDTO.TradeSimulationKlinePointDTO, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	rows, err := argusTrade.GetKlinesByPlatform(ctx, platformCode, argusTrade.MarketKlineRequest{
		Symbol:   symbol,
		Interval: interval,
		Limit:    limit,
	})
	if err != nil {
		return nil, fmt.Errorf("获取%s行情失败: %w", platformCode, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("获取%s行情失败: K线为空", platformCode)
	}

	points := make([]tradeDTO.TradeSimulationKlinePointDTO, 0, len(rows))
	for _, row := range rows {
		openPrice, err := parseMarketFloat(row.OpenPrice)
		if err != nil {
			return nil, fmt.Errorf("解析开盘价失败: %w", err)
		}
		highPrice, err := parseMarketFloat(row.HighPrice)
		if err != nil {
			return nil, fmt.Errorf("解析最高价失败: %w", err)
		}
		lowPrice, err := parseMarketFloat(row.LowPrice)
		if err != nil {
			return nil, fmt.Errorf("解析最低价失败: %w", err)
		}
		closePrice, err := parseMarketFloat(row.ClosePrice)
		if err != nil {
			return nil, fmt.Errorf("解析收盘价失败: %w", err)
		}
		volume, err := parseMarketFloat(row.Volume)
		if err != nil {
			return nil, fmt.Errorf("解析成交量失败: %w", err)
		}
		pointTime := time.UnixMilli(row.OpenTime)
		points = append(points, tradeDTO.TradeSimulationKlinePointDTO{
			Time:       pointTime.Format("01-02 15:04"),
			Timestamp:  pointTime.Unix(),
			OpenPrice:  roundPrice(openPrice),
			HighPrice:  roundPrice(highPrice),
			LowPrice:   roundPrice(lowPrice),
			ClosePrice: roundPrice(closePrice),
			Volume:     roundPrice(volume),
		})
	}
	return points, nil
}

func (s *TradeService) listSimulationKlinesFromDB(symbol, interval string, limit int) ([]tradeDTO.TradeSimulationKlinePointDTO, error) {
	klineRows, err := s.tradeKlineRepository.ListBySymbolInterval(symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(klineRows)-1; i < j; i, j = i+1, j-1 {
		klineRows[i], klineRows[j] = klineRows[j], klineRows[i]
	}
	points := make([]tradeDTO.TradeSimulationKlinePointDTO, 0, len(klineRows))
	for _, kline := range klineRows {
		points = append(points, buildSimulationKlinePoint(kline))
	}
	return points, nil
}

func buildSimulationKlinePoint(kline *tradeRepository.TradeKline) tradeDTO.TradeSimulationKlinePointDTO {
	return tradeDTO.TradeSimulationKlinePointDTO{
		Time:       kline.OpenTime.Format("01-02 15:04"),
		Timestamp:  kline.OpenTime.Unix(),
		OpenPrice:  roundPrice(kline.OpenPrice),
		HighPrice:  roundPrice(kline.HighPrice),
		LowPrice:   roundPrice(kline.LowPrice),
		ClosePrice: roundPrice(kline.ClosePrice),
		Volume:     roundPrice(kline.Volume),
	}
}

func parseMarketFloat(value string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(value), 64)
}

// signOf 返回数值符号：正 1 / 负 -1 / 零 0。
func signOf(v float64) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	default:
		return 0
	}
}

// closePriceAt 在 1m K线序列中取包含目标时刻 t 的那根的收盘价；
// 若无完全包含的（如 t 落在缝隙），则取 openTime 不晚于 t 且最接近的一根。
// K线 OpenTime/CloseTime 单位为毫秒。取不到有效价时返回 ok=false。
func closePriceAt(klines []argusTrade.MarketKline, t time.Time) (float64, bool) {
	targetMs := t.UnixMilli()
	var best *argusTrade.MarketKline
	for i := range klines {
		k := &klines[i]
		if k.OpenTime <= targetMs && targetMs <= k.CloseTime {
			if v, err := parseMarketFloat(k.ClosePrice); err == nil && v > 0 {
				return v, true
			}
			return 0, false
		}
		if k.OpenTime <= targetMs && (best == nil || k.OpenTime > best.OpenTime) {
			best = k
		}
	}
	if best == nil {
		return 0, false
	}
	if v, err := parseMarketFloat(best.ClosePrice); err == nil && v > 0 {
		return v, true
	}
	return 0, false
}

const (
	// settleKlineMinLimit/MaxLimit：结算拉取 1m K线条数的下限与上限。
	// 上限 1500 为主流交易所(Binance 期货等)单次返回上限；下限保证短周期也有足够上下文。
	settleKlineMinLimit = 200
	settleKlineMaxLimit = 1500
)

// settleKlineLimit 根据本批次最早的 created_time 动态计算需要的 1m K线条数：
// 覆盖 [earliestCreated, now] 全窗口并留出余量，夹在 [min, max] 之间。
// earliestCreated 为零值(无数据)时退回上限，最大化覆盖。
func settleKlineLimit(earliestCreated time.Time) int {
	if earliestCreated.IsZero() {
		return settleKlineMaxLimit
	}
	minutes := int(time.Since(earliestCreated).Minutes()) + 5 // +5 余量，吸收边界/时钟误差
	if minutes < settleKlineMinLimit {
		return settleKlineMinLimit
	}
	if minutes > settleKlineMaxLimit {
		return settleKlineMaxLimit
	}
	return minutes
}

// evalPredictionWindow 遍历 [from, to] 区间内的 1m K线，计算区间触达指标：
//   - mfe: 沿预测方向的最大有利偏移%(相对 ref)；mae: 逆方向的最大不利偏移%(正数表回撤)。
//   - firstHit: 按时间先后，先触止盈("tp") / 先触止损("sl") / 都未触("none")。
//     同一根 K 线内止盈止损都被触及时，无法判先后，保守按"先触止损"处理（与 simulateTrade 一致）。
//
// dir 由 predict-ref 的符号决定；neutral(dir=0) 不判触达，mfe/mae 取双向最大波动。
// tp/sl 为 0 表示未给该价位，对应方向永不触发。
func evalPredictionWindow(klines []argusTrade.MarketKline, from, to time.Time, ref, predict, tp, sl float64) (mfe, mae float64, firstHit string, actualHigh, actualLow float64) {
	firstHit = "none"
	if ref <= 0 {
		return 0, 0, firstHit, 0, 0
	}
	fromMs, toMs := from.UnixMilli(), to.UnixMilli()

	// 过滤出窗口内的 K 线并按开盘时间升序，保证 firstHit 的时间先后判定正确。
	window := make([]argusTrade.MarketKline, 0, len(klines))
	for i := range klines {
		if k := klines[i]; k.OpenTime >= fromMs && k.OpenTime <= toMs {
			window = append(window, k)
		}
	}
	sort.Slice(window, func(i, j int) bool { return window[i].OpenTime < window[j].OpenTime })

	dir := signOf(predict - ref)
	for i := range window {
		high, herr := parseMarketFloat(window[i].HighPrice)
		low, lerr := parseMarketFloat(window[i].LowPrice)
		if herr != nil || lerr != nil || high <= 0 || low <= 0 {
			continue
		}
		if actualHigh == 0 || high > actualHigh {
			actualHigh = high
		}
		if actualLow == 0 || low < actualLow {
			actualLow = low
		}
		upPct := (high - ref) / ref * 100
		downPct := (ref - low) / ref * 100
		switch {
		case dir > 0:
			mfe = math.Max(mfe, upPct)
			mae = math.Max(mae, downPct)
		case dir < 0:
			mfe = math.Max(mfe, downPct)
			mae = math.Max(mae, upPct)
		default:
			mfe = math.Max(mfe, math.Max(upPct, downPct))
		}

		if firstHit == "none" && dir != 0 {
			var hitTp, hitSl bool
			if dir > 0 {
				hitTp = tp > 0 && high >= tp
				hitSl = sl > 0 && low <= sl
			} else {
				hitTp = tp > 0 && low <= tp
				hitSl = sl > 0 && high >= sl
			}
			if hitSl { // 同根内保守：止损优先
				firstHit = "sl"
			} else if hitTp {
				firstHit = "tp"
			}
		}
	}
	return mfe, mae, firstHit, actualHigh, actualLow
}

func roundPrice(value float64) float64 {
	return math.Round(value*10000) / 10000
}

// floorUnixToInterval 把 unix 秒向下取整到指定周期(秒)的网格。
// unix 纪元对齐到 UTC 0 点，因此对 1m/1h/4h/1d 等周期取模即可得到与交易所 K 线一致的开盘时间。
func floorUnixToInterval(ts int64, durSec int64) int64 {
	if durSec <= 0 {
		return ts
	}
	return ts - ((ts%durSec)+durSec)%durSec
}

// intervalToDuration 把 K 线周期字符串（5m/15m/1h/4h/1d 等）转成时间间隔，无法识别时回退到 15 分钟。
func intervalToDuration(interval string) time.Duration {
	switch strings.TrimSpace(interval) {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return 15 * time.Minute
	}
}

func round2(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return math.Round(value*100) / 100
}

// parsePercentList 解析逗号分隔的百分比列表，去重并升序；为空时回退到 fallback。
func parsePercentList(raw string, fallback []float64) []float64 {
	seen := make(map[float64]struct{})
	out := make([]float64, 0, 8)
	for _, part := range strings.Split(raw, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || v <= 0 {
			continue
		}
		v = round2(v)
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	if len(out) == 0 {
		out = append(out, fallback...)
	}
	sort.Float64s(out)
	return out
}

// backtestSignal 一条满足开仓条件的历史信号及其持仓窗口内的真实 K 线。
type backtestSignal struct {
	dirSign float64 // +1 做多 / -1 做空
	entry   float64 // 开仓价（预测时收盘价）
	window  []tradeDTO.TradeSimulationKlinePointDTO
}

// simulateTrade 在单条信号上模拟「现价开仓 + 固定止盈止损 + 持仓窗口到期平仓」的名义收益率(%)。
// 同一根 K 线内若止盈/止损价同时被触及，无法判定先后，保守按「先触止损」处理。
// 返回 grossPct(名义收益%) 与 outcome("tp"/"sl"/"timeout")。
func simulateTrade(sig backtestSignal, tpPct, slPct float64) (float64, string) {
	long := sig.dirSign > 0
	var tpPrice, slPrice float64
	if long {
		tpPrice = sig.entry * (1 + tpPct/100)
		slPrice = sig.entry * (1 - slPct/100)
	} else {
		tpPrice = sig.entry * (1 - tpPct/100)
		slPrice = sig.entry * (1 + slPct/100)
	}
	for _, k := range sig.window {
		var hitTp, hitSl bool
		if long {
			hitTp = k.HighPrice >= tpPrice
			hitSl = k.LowPrice <= slPrice
		} else {
			hitTp = k.LowPrice <= tpPrice
			hitSl = k.HighPrice >= slPrice
		}
		if hitSl { // 保守：同一根内止损优先
			return -slPct, "sl"
		}
		if hitTp {
			return tpPct, "tp"
		}
	}
	// 到期未触发：按窗口末根收盘价平仓
	last := sig.window[len(sig.window)-1]
	gross := (last.ClosePrice - sig.entry) / sig.entry * 100
	if !long {
		gross = -gross
	}
	return gross, "timeout"
}

// GetStrategyBacktest 按「方向 + 预测幅度阈值 + 置信度」筛选历史 AI 预测信号，
// 用其后续真实 K 线回测不同「止盈×止损」组合的扣费后期望，输出期望矩阵。
func (s *TradeService) GetStrategyBacktest(ctx context.Context, query tradeDTO.TradeStrategyBacktestQueryDTO) (*tradeDTO.TradeStrategyBacktestDTO, error) {
	platformCode := strings.ToLower(strings.TrimSpace(query.PlatformCode))
	if platformCode == "" {
		platformCode = argusTrade.PlatformBinance
	}
	if platformCode != argusTrade.PlatformBinance {
		return nil, fmt.Errorf("AI模拟盘暂时只支持币安")
	}
	coinCode := strings.ToUpper(strings.TrimSpace(query.CoinCode))
	if coinCode == "" {
		coinCode = "BTC"
	}
	interval := strings.TrimSpace(query.Interval)
	if interval == "" {
		interval = "1h"
	}

	limit := query.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}
	holdBars := query.HoldBars
	if holdBars <= 0 {
		holdBars = 1
	}
	if holdBars > 24 {
		holdBars = 24
	}
	minConfidence := query.MinConfidence
	if minConfidence < 0 {
		minConfidence = 0
	}
	minMovePct := query.MinMovePct
	if minMovePct < 0 {
		minMovePct = 0
	}
	takerFeeRate := query.TakerFeeRate
	if takerFeeRate <= 0 {
		takerFeeRate = 0.05 // 币安合约吃单 0.05%
	}
	fundingRate := query.FundingRate
	if fundingRate < 0 {
		fundingRate = 0
	}
	leverage := query.Leverage
	if leverage <= 0 {
		leverage = 1
	}
	tpPercents := parsePercentList(query.TpList, []float64{1, 1.5, 2, 2.5, 3})
	slPercents := parsePercentList(query.SlList, []float64{0.5, 1, 1.5, 2})

	symbol := coinCode + "USDT"

	klines, err := s.listSimulationKlinesFromDB(symbol, interval, limit)
	if err != nil {
		return nil, err
	}
	idxByTs := make(map[int64]int, len(klines))
	for i, k := range klines {
		idxByTs[k.Timestamp] = i
	}

	result := &tradeDTO.TradeStrategyBacktestDTO{
		PlatformCode:  platformCode,
		CoinCode:      coinCode,
		Symbol:        symbol,
		Interval:      interval,
		HoldBars:      holdBars,
		MinConfidence: minConfidence,
		MinMovePct:    minMovePct,
		TakerFeeRate:  takerFeeRate,
		FundingRate:   fundingRate,
		Leverage:      leverage,
		TpPercents:    tpPercents,
		SlPercents:    slPercents,
		Cells:         []tradeDTO.TradeStrategyBacktestCellDTO{},
		PlatformOptions: []tradeDTO.TradeAnalysisOptionDTO{
			{Label: "Binance", Value: "binance"},
		},
		// AI 预测目前只跑 BTC（oracle.coins=BTC），其它币种无预测数据，因此只暴露 BTC。
		CoinOptions: []tradeDTO.TradeAnalysisOptionDTO{
			{Label: "BTC / USDT", Value: "BTC"},
		},
	}
	// 单笔总成本(名义%)：开+平两次手续费 + 每根持仓周期的资金费率
	costPerTrade := round2(2*takerFeeRate + fundingRate*float64(holdBars))
	result.CostPerTrade = costPerTrade

	if len(klines) == 0 {
		return result, nil
	}
	result.RangeStart = klines[0].Time
	result.RangeEnd = klines[len(klines)-1].Time

	startTime := time.Unix(klines[0].Timestamp, 0)
	endTime := time.Unix(klines[len(klines)-1].Timestamp, 0)
	predRows, err := s.tradeAIPredictionRepository.ListByCoinIntervalTimeRange(platformCode, coinCode, interval, startTime, endTime)
	if err != nil {
		return nil, err
	}
	result.TotalPredictions = len(predRows)

	// 1) 信号筛选：方向有效 + 置信度达标 + 预测幅度达标 + 持仓窗口有完整真实 K 线。
	signals := make([]backtestSignal, 0, len(predRows))
	dirCorrect := 0
	moveSum := 0.0
	for _, pred := range predRows {
		dir := strings.ToLower(strings.TrimSpace(pred.Trend))
		var dirSign float64
		switch dir {
		case "long":
			dirSign = 1
		case "short":
			dirSign = -1
		default:
			continue // neutral 不开仓
		}
		if pred.Confidence < minConfidence {
			continue
		}
		entry := pred.RefPrice
		// 滚动预测的 predict_time 不在 K 线边界上，向下取整到 K 线网格后定位落点，持仓窗口从这根开始往后取 holdBars 根。
		i, ok := idxByTs[floorUnixToInterval(pred.PredictTime.Unix(), int64(intervalToDuration(interval)/time.Second))]
		if !ok || entry <= 0 {
			continue
		}
		if entry == 0 {
			entry = klines[i].ClosePrice
		}
		movePct := math.Abs(pred.PredictPrice-entry) / entry * 100
		if movePct < minMovePct {
			continue
		}
		if i+holdBars > len(klines) {
			continue // 持仓窗口超出已知 K 线，无法回测
		}
		window := klines[i : i+holdBars]
		sig := backtestSignal{dirSign: dirSign, entry: entry, window: window}
		signals = append(signals, sig)
		moveSum += movePct
		// 方向正确率：以持仓窗口末收盘价相对开仓价的方向判定
		last := window[len(window)-1]
		if (last.ClosePrice-entry)*dirSign > 0 {
			dirCorrect++
		}
	}

	n := len(signals)
	result.QualifiedSignals = n
	if n == 0 {
		return result, nil
	}
	result.DirectionAccuracy = round2(float64(dirCorrect) / float64(n) * 100)
	result.AvgPredictMovePct = round2(moveSum / float64(n))

	// 2) 遍历止盈×止损组合，逐信号模拟并聚合。
	var best *tradeDTO.TradeStrategyBacktestCellDTO
	for _, sl := range slPercents {
		for _, tp := range tpPercents {
			var tpCnt, slCnt, toCnt, winCnt, lossCnt int
			var sumNet, sumWin, sumLossAbs float64
			var cum, peak, maxDD float64
			for _, sig := range signals {
				gross, outcome := simulateTrade(sig, tp, sl)
				net := gross - costPerTrade
				switch outcome {
				case "tp":
					tpCnt++
				case "sl":
					slCnt++
				default:
					toCnt++
				}
				if net > 0 {
					winCnt++
					sumWin += net
				} else if net < 0 {
					lossCnt++
					sumLossAbs += -net
				}
				sumNet += net
				cum += net
				if cum > peak {
					peak = cum
				}
				if dd := peak - cum; dd > maxDD {
					maxDD = dd
				}
			}
			cell := tradeDTO.TradeStrategyBacktestCellDTO{
				TakeProfitPct: tp,
				StopLossPct:   sl,
				Samples:       n,
				TpRate:        round2(float64(tpCnt) / float64(n) * 100),
				SlRate:        round2(float64(slCnt) / float64(n) * 100),
				TimeoutRate:   round2(float64(toCnt) / float64(n) * 100),
				WinRate:       round2(float64(winCnt) / float64(n) * 100),
				Expectancy:    round2(sumNet / float64(n)),
				ExpectancyRoe: round2(sumNet / float64(n) * leverage),
				TotalReturn:   round2(sumNet),
				MaxDrawdown:   round2(maxDD),
			}
			if winCnt > 0 {
				cell.AvgWin = round2(sumWin / float64(winCnt))
			}
			if lossCnt > 0 {
				cell.AvgLoss = round2(sumLossAbs / float64(lossCnt))
			}
			if cell.AvgLoss > 0 {
				cell.Payoff = round2(cell.AvgWin / cell.AvgLoss)
			}
			if sumLossAbs > 0 {
				cell.ProfitFactor = round2(sumWin / sumLossAbs)
			} else if sumWin > 0 {
				cell.ProfitFactor = 999 // 无亏损样本
			}
			result.Cells = append(result.Cells, cell)
			if best == nil || cell.Expectancy > best.Expectancy {
				c := cell
				best = &c
			}
		}
	}
	result.Best = best
	return result, nil
}

// ── 策略 & 持仓方法 ──────────────────────────────────────────────────────────

// GetActiveStrategies 返回指定 platform×symbol×interval 下所有启用的策略。
func (s *TradeService) GetActiveStrategies(platformCode, symbol, interval string) ([]*tradeRepository.TradeStrategy, error) {
	return s.tradeStrategyRepository.FindActiveBySymbolInterval(platformCode, symbol, interval)
}

// CountOpenPositions 返回指定策略当前未平仓数，供 max_open_positions 检查。
func (s *TradeService) CountOpenPositions(strategyID int64) (int64, error) {
	return s.tradeStrategyRepository.CountOpenPositions(strategyID)
}

// OpenPosition 写入一条 status=open 的持仓记录。
func (s *TradeService) OpenPosition(pos *tradeRepository.TradeStrategyPosition) error {
	return s.tradeStrategyPositionRepository.CreatePosition(pos)
}

// GetOpenPositions 返回所有未平仓持仓，供监测循环使用。
func (s *TradeService) GetOpenPositions() ([]*tradeRepository.TradeStrategyPosition, error) {
	return s.tradeStrategyPositionRepository.FindOpenPositions()
}

// ClosePosition 事务平仓，计算盈亏并写入结算字段。
// 若持仓已不是 open 状态（重复触发），返回 gorm.ErrRecordNotFound，调用方可安全忽略。
func (s *TradeService) ClosePosition(
	pos *tradeRepository.TradeStrategyPosition,
	closePrice float64,
	closeReason string,
	closedAt time.Time,
	makerFeeRate, takerFeeRate float64,
) error {
	contractSize := 0.001 // 1 张 = 0.001 BTC
	qty := float64(pos.Contracts) * contractSize

	// 盈亏方向
	var rawPnl float64
	switch pos.Direction {
	case "long":
		rawPnl = (closePrice - pos.OpenPrice) * qty
	case "short":
		rawPnl = (pos.OpenPrice - closePrice) * qty
	default:
		rawPnl = 0
	}

	// 手续费：开仓 Taker，平仓止盈用 Maker，其余用 Taker
	closeFeeRate := takerFeeRate
	if closeReason == "tp" {
		closeFeeRate = makerFeeRate
	}
	fee := pos.OpenPrice*qty*takerFeeRate + closePrice*qty*closeFeeRate

	netPnl := rawPnl - fee
	pnlRate := 0.0
	if pos.OpenPrice > 0 {
		pnlRate = (rawPnl / (pos.OpenPrice * qty / pos.Leverage)) * 100
	}

	return s.tradeStrategyPositionRepository.ClosePositionTx(
		pos.Id, closePrice, closeReason, closedAt,
		rawPnl, pnlRate, fee, netPnl,
	)
}

// UpdatePositionMinMax 更新持仓期间的最高/最低价追踪。
func (s *TradeService) UpdatePositionMinMax(pos *tradeRepository.TradeStrategyPosition, currentPrice float64) error {
	return s.tradeStrategyPositionRepository.UpdateMinMaxPrice(pos.Id, currentPrice, pos)
}
