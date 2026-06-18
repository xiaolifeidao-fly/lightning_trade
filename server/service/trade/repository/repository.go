package repository

import (
	"common/middleware/db"
	"fmt"
	tradeDTO "service/trade/dto"
	"strings"
	"time"

	"gorm.io/gorm"
)

type TradeOrderRepository struct {
	db.Repository[*TradeOrder]
}

func (r *TradeOrderRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeOrder{})
}

func (r *TradeOrderRepository) FindByOrderNo(orderNo string) (*TradeOrder, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity TradeOrder
	if err := r.Db.Where("order_no = ? AND active = 1", orderNo).First(&entity).Error; err != nil {
		return nil, err
	}
	if entity.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &entity, nil
}

func (r *TradeOrderRepository) ListOpenOrdersByUser(userID uint64, symbol string) ([]*TradeOrder, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Where("active = 1 AND user_id = ? AND status IN ?", userID, []string{"pending", "partial"})
	if symbol = strings.TrimSpace(symbol); symbol != "" {
		dbq = dbq.Where("symbol = ?", strings.ToUpper(symbol))
	}
	var rows []*TradeOrder
	if err := dbq.Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeOrderRepository) CountOrdersByQuery(query tradeDTO.TradeOrderQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	whereSQL, values := buildTradeOrderWhere(query)
	sql := "SELECT id FROM trade_order " + whereSQL
	return r.CountBySQL(sql, values...)
}

func (r *TradeOrderRepository) ListOrdersByQuery(query tradeDTO.TradeOrderQueryDTO, pageIndex, pageSize int) ([]TradeOrderListRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	whereSQL, values := buildTradeOrderWhere(query)
	sql := `SELECT id, active, created_time, updated_time, created_by, updated_by,
		platform_id, platform_code, trade_category, trade_type,
		order_no, user_id, symbol, base_coin_code, quote_coin_code, side, order_type,
		price, amount, total, filled_amount, filled_total, avg_filled_price, fee_amount,
		status, submitted_time, finished_time
		FROM trade_order ` + whereSQL + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	values = append(values, pageSize, (pageIndex-1)*pageSize)
	var rows []TradeOrderListRow
	if err := r.QueryBySQL(&rows, sql, values...); err != nil {
		return nil, err
	}
	return rows, nil
}

func buildTradeOrderWhere(query tradeDTO.TradeOrderQueryDTO) (string, []interface{}) {
	clauses := []string{"WHERE active = 1"}
	values := make([]interface{}, 0, 12)

	if query.PlatformID > 0 {
		clauses = append(clauses, "platform_id = ?")
		values = append(values, query.PlatformID)
	}
	if value := strings.TrimSpace(query.PlatformCode); value != "" {
		clauses = append(clauses, "platform_code = ?")
		values = append(values, strings.ToLower(value))
	}
	if value := strings.TrimSpace(query.TradeCategory); value != "" {
		clauses = append(clauses, "trade_category = ?")
		values = append(values, value)
	}
	if value := strings.TrimSpace(query.TradeType); value != "" {
		clauses = append(clauses, "trade_type = ?")
		values = append(values, value)
	}
	if query.UserID > 0 {
		clauses = append(clauses, "user_id = ?")
		values = append(values, query.UserID)
	}
	if value := strings.TrimSpace(query.Symbol); value != "" {
		clauses = append(clauses, "symbol = ?")
		values = append(values, strings.ToUpper(value))
	}
	if value := strings.TrimSpace(query.Side); value != "" {
		clauses = append(clauses, "side = ?")
		values = append(values, value)
	}
	if value := strings.TrimSpace(query.OrderType); value != "" {
		clauses = append(clauses, "order_type = ?")
		values = append(values, value)
	}
	if value := strings.TrimSpace(query.Status); value != "" {
		clauses = append(clauses, "status = ?")
		values = append(values, value)
	}
	if value := strings.TrimSpace(query.OrderNo); value != "" {
		clauses = append(clauses, "order_no LIKE ?")
		values = append(values, "%"+value+"%")
	}
	if query.StartTime > 0 {
		clauses = append(clauses, "submitted_time >= ?")
		values = append(values, time.Unix(query.StartTime, 0))
	}
	if query.EndTime > 0 {
		clauses = append(clauses, "submitted_time <= ?")
		values = append(values, time.Unix(query.EndTime, 0))
	}

	return strings.Join(clauses, " AND "), values
}

type TradeMatchRepository struct {
	db.Repository[*TradeMatch]
}

func (r *TradeMatchRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeMatch{})
}

func (r *TradeMatchRepository) ListBySymbol(symbol string, limit int) ([]*TradeMatch, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	var rows []*TradeMatch
	if err := r.Db.Where("symbol = ? AND active = 1", strings.ToUpper(symbol)).
		Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeMatchRepository) ListByUserID(userID uint64, symbol string, limit int) ([]*TradeMatch, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 100
	}
	dbq := r.Db.Where("active = 1 AND (taker_user_id = ? OR maker_user_id = ?)", userID, userID)
	if symbol = strings.TrimSpace(symbol); symbol != "" {
		dbq = dbq.Where("symbol = ?", strings.ToUpper(symbol))
	}
	var rows []*TradeMatch
	if err := dbq.Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type TradeKlineRepository struct {
	db.Repository[*TradeKline]
}

func (r *TradeKlineRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeKline{})
}

func (r *TradeKlineRepository) ListBySymbolInterval(symbol, interval string, limit int) ([]*TradeKline, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 200
	}
	var rows []*TradeKline
	if err := r.Db.Where("active = 1 AND symbol = ? AND `interval` = ?", strings.ToUpper(symbol), interval).
		Order("open_time DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeKlineRepository) ListBySymbolIntervalTimeRange(symbol, interval string, startTime, endTime time.Time) ([]*TradeKline, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeKline
	if err := r.Db.Where("active = 1 AND symbol = ? AND `interval` = ? AND open_time >= ? AND open_time <= ?",
		strings.ToUpper(symbol), interval, startTime, endTime).
		Order("open_time ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type TradeDetailRepository struct {
	db.Repository[*TradeDetail]
}

func (r *TradeDetailRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeDetail{})
}

func (r *TradeDetailRepository) ListByOrderNo(orderNo string) ([]*TradeDetail, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeDetail
	if err := r.Db.Where("order_no = ? AND active = 1", orderNo).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeDetailRepository) CountByQuery(query tradeDTO.TradeDetailQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	whereSQL, values := buildTradeDetailWhere(query)
	sql := "SELECT id FROM trade_detail " + whereSQL
	return r.CountBySQL(sql, values...)
}

func (r *TradeDetailRepository) ListByQuery(query tradeDTO.TradeDetailQueryDTO, pageIndex, pageSize int) ([]*TradeDetail, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Model(&TradeDetail{}).Where("active = 1")
	if query.PlatformID > 0 {
		dbq = dbq.Where("platform_id = ?", query.PlatformID)
	}
	if v := strings.TrimSpace(query.PlatformCode); v != "" {
		dbq = dbq.Where("platform_code = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(query.TradeCategory); v != "" {
		dbq = dbq.Where("trade_category = ?", v)
	}
	if v := strings.TrimSpace(query.TradeType); v != "" {
		dbq = dbq.Where("trade_type = ?", v)
	}
	if query.UserID > 0 {
		dbq = dbq.Where("user_id = ?", query.UserID)
	}
	if v := strings.TrimSpace(query.OrderNo); v != "" {
		dbq = dbq.Where("order_no = ?", v)
	}
	if v := strings.TrimSpace(query.Symbol); v != "" {
		dbq = dbq.Where("symbol = ?", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if query.StartTime > 0 {
		dbq = dbq.Where("trade_time >= ?", time.Unix(query.StartTime, 0))
	}
	if query.EndTime > 0 {
		dbq = dbq.Where("trade_time <= ?", time.Unix(query.EndTime, 0))
	}
	var rows []*TradeDetail
	if err := dbq.Order("id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func buildTradeDetailWhere(query tradeDTO.TradeDetailQueryDTO) (string, []interface{}) {
	clauses := []string{"WHERE active = 1"}
	values := make([]interface{}, 0, 10)
	if query.PlatformID > 0 {
		clauses = append(clauses, "platform_id = ?")
		values = append(values, query.PlatformID)
	}
	if v := strings.TrimSpace(query.PlatformCode); v != "" {
		clauses = append(clauses, "platform_code = ?")
		values = append(values, strings.ToLower(v))
	}
	if v := strings.TrimSpace(query.TradeCategory); v != "" {
		clauses = append(clauses, "trade_category = ?")
		values = append(values, v)
	}
	if v := strings.TrimSpace(query.TradeType); v != "" {
		clauses = append(clauses, "trade_type = ?")
		values = append(values, v)
	}
	if query.UserID > 0 {
		clauses = append(clauses, "user_id = ?")
		values = append(values, query.UserID)
	}
	if v := strings.TrimSpace(query.OrderNo); v != "" {
		clauses = append(clauses, "order_no = ?")
		values = append(values, v)
	}
	if v := strings.TrimSpace(query.Symbol); v != "" {
		clauses = append(clauses, "symbol = ?")
		values = append(values, strings.ToUpper(v))
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		clauses = append(clauses, "coin_code = ?")
		values = append(values, strings.ToUpper(v))
	}
	if query.StartTime > 0 {
		clauses = append(clauses, "trade_time >= ?")
		values = append(values, time.Unix(query.StartTime, 0))
	}
	if query.EndTime > 0 {
		clauses = append(clauses, "trade_time <= ?")
		values = append(values, time.Unix(query.EndTime, 0))
	}
	return strings.Join(clauses, " AND "), values
}

type TradeAIPredictionRepository struct {
	db.Repository[*TradeAIPrediction]
}

func (r *TradeAIPredictionRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeAIPrediction{})
}

// Upsert 按 平台×交易对×周期×K线时间 维度写入或更新一条预测。
func (r *TradeAIPredictionRepository) Upsert(entity *TradeAIPrediction) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	entity.Symbol = strings.ToUpper(strings.TrimSpace(entity.Symbol))
	entity.CoinCode = strings.ToUpper(strings.TrimSpace(entity.CoinCode))
	entity.PlatformCode = strings.ToLower(strings.TrimSpace(entity.PlatformCode))
	entity.Init()

	var existing TradeAIPrediction
	err := r.Db.Where("platform_code = ? AND symbol = ? AND `interval` = ? AND predict_time = ?",
		entity.PlatformCode, entity.Symbol, entity.Interval, entity.PredictTime).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return r.Db.Create(entity).Error
	}
	if err != nil {
		return err
	}
	entity.Id = existing.Id
	entity.CreatedTime = existing.CreatedTime
	return r.Db.Save(entity).Error
}

// ListByCoinInterval 按 平台×币种×周期 取最近 limit 条预测（按 K线时间倒序）。
func (r *TradeAIPredictionRepository) ListByCoinInterval(platformCode, coinCode, interval string, limit int) ([]*TradeAIPrediction, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 200
	}
	var rows []*TradeAIPrediction
	if err := r.Db.Where("active = 1 AND platform_code = ? AND coin_code = ? AND `interval` = ?",
		strings.ToLower(platformCode), strings.ToUpper(coinCode), interval).
		Order("predict_time DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeAIPredictionRepository) ListByCoinIntervalTimeRange(platformCode, coinCode, interval string, startTime, endTime time.Time) ([]*TradeAIPrediction, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*TradeAIPrediction
	if err := r.Db.Where("active = 1 AND platform_code = ? AND coin_code = ? AND `interval` = ? AND predict_time >= ? AND predict_time <= ?",
		strings.ToLower(platformCode), strings.ToUpper(coinCode), interval, startTime, endTime).
		Order("predict_time ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// ListUnsettled 取 predict_time 已到期(<=before)但尚未结算回填的预测，按 predict_time 升序，最多 limit 条。
func (r *TradeAIPredictionRepository) ListUnsettled(before time.Time, limit int) ([]*TradeAIPrediction, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 200
	}
	var rows []*TradeAIPrediction
	if err := r.Db.Where("active = 1 AND settled = 0 AND predict_time <= ?", before).
		Order("predict_time ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// PredictionSettlement 一条预测的结算结果：端点(误差/方向命中) + 区间触达(MFE/MAE/先触达)。
type PredictionSettlement struct {
	ActualPrice     float64
	ErrorPct        float64
	AbsErrorPct     float64
	DirectionHit    int8
	MaxFavorablePct float64
	MaxAdversePct   float64
	FirstHit        string
	ActualHigh      float64
	ActualLow       float64
	HighErrorPct    float64
	LowErrorPct     float64
	BandContain     int8
	InvalidationHit int8
}

// SettlePrediction 回填一条预测的端点误差/方向命中与区间触达指标，并标记为已结算。
func (r *TradeAIPredictionRepository) SettlePrediction(id int, st PredictionSettlement) error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	now := time.Now()
	return r.Db.Model(&TradeAIPrediction{}).Where("id = ?", id).Updates(map[string]interface{}{
		"actual_price":      st.ActualPrice,
		"error_pct":         st.ErrorPct,
		"abs_error_pct":     st.AbsErrorPct,
		"direction_hit":     st.DirectionHit,
		"max_favorable_pct": st.MaxFavorablePct,
		"max_adverse_pct":   st.MaxAdversePct,
		"first_hit":         st.FirstHit,
		"actual_high":       st.ActualHigh,
		"actual_low":        st.ActualLow,
		"high_error_pct":    st.HighErrorPct,
		"low_error_pct":     st.LowErrorPct,
		"band_contain":      st.BandContain,
		"invalidation_hit":  st.InvalidationHit,
		"settled":           1,
		"settled_time":      now,
		"updated_time":      now,
	}).Error
}

type TradeUserSummaryRepository struct {
	db.Repository[*TradeUserSummary]
}

func (r *TradeUserSummaryRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeUserSummary{})
}

func (r *TradeUserSummaryRepository) ListByQuery(query tradeDTO.TradeUserSummaryQueryDTO, pageIndex, pageSize int) ([]*TradeUserSummary, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Model(&TradeUserSummary{}).Where("active = 1")
	if query.UserID > 0 {
		dbq = dbq.Where("user_id = ?", query.UserID)
	}
	if query.PlatformID > 0 {
		dbq = dbq.Where("platform_id = ?", query.PlatformID)
	}
	if v := strings.TrimSpace(query.PlatformCode); v != "" {
		dbq = dbq.Where("platform_code = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(query.TradeCategory); v != "" {
		dbq = dbq.Where("trade_category = ?", v)
	}
	if v := strings.TrimSpace(query.StartDate); v != "" {
		dbq = dbq.Where("trade_date >= ?", v)
	}
	if v := strings.TrimSpace(query.EndDate); v != "" {
		dbq = dbq.Where("trade_date <= ?", v)
	}
	var rows []*TradeUserSummary
	if err := dbq.Order("trade_date DESC, id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeUserSummaryRepository) CountByQuery(query tradeDTO.TradeUserSummaryQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Model(&TradeUserSummary{}).Where("active = 1")
	if query.UserID > 0 {
		dbq = dbq.Where("user_id = ?", query.UserID)
	}
	if query.PlatformID > 0 {
		dbq = dbq.Where("platform_id = ?", query.PlatformID)
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(query.TradeCategory); v != "" {
		dbq = dbq.Where("trade_category = ?", v)
	}
	if v := strings.TrimSpace(query.StartDate); v != "" {
		dbq = dbq.Where("trade_date >= ?", v)
	}
	if v := strings.TrimSpace(query.EndDate); v != "" {
		dbq = dbq.Where("trade_date <= ?", v)
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

type TradeUserPnlRepository struct {
	db.Repository[*TradeUserPnl]
}

func (r *TradeUserPnlRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&TradeUserPnl{})
}

func (r *TradeUserPnlRepository) ListByQuery(query tradeDTO.TradeUserPnlQueryDTO, pageIndex, pageSize int) ([]*TradeUserPnl, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Model(&TradeUserPnl{}).Where("active = 1")
	if query.UserID > 0 {
		dbq = dbq.Where("user_id = ?", query.UserID)
	}
	if query.PlatformID > 0 {
		dbq = dbq.Where("platform_id = ?", query.PlatformID)
	}
	if v := strings.TrimSpace(query.PlatformCode); v != "" {
		dbq = dbq.Where("platform_code = ?", strings.ToLower(v))
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(query.TradeCategory); v != "" {
		dbq = dbq.Where("trade_category = ?", v)
	}
	if v := strings.TrimSpace(query.StartDate); v != "" {
		dbq = dbq.Where("trade_date >= ?", v)
	}
	if v := strings.TrimSpace(query.EndDate); v != "" {
		dbq = dbq.Where("trade_date <= ?", v)
	}
	var rows []*TradeUserPnl
	if err := dbq.Order("trade_date DESC, id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *TradeUserPnlRepository) CountByQuery(query tradeDTO.TradeUserPnlQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	dbq := r.Db.Model(&TradeUserPnl{}).Where("active = 1")
	if query.UserID > 0 {
		dbq = dbq.Where("user_id = ?", query.UserID)
	}
	if query.PlatformID > 0 {
		dbq = dbq.Where("platform_id = ?", query.PlatformID)
	}
	if v := strings.TrimSpace(query.CoinCode); v != "" {
		dbq = dbq.Where("coin_code = ?", strings.ToUpper(v))
	}
	if v := strings.TrimSpace(query.TradeCategory); v != "" {
		dbq = dbq.Where("trade_category = ?", v)
	}
	if v := strings.TrimSpace(query.StartDate); v != "" {
		dbq = dbq.Where("trade_date >= ?", v)
	}
	if v := strings.TrimSpace(query.EndDate); v != "" {
		dbq = dbq.Where("trade_date <= ?", v)
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}
