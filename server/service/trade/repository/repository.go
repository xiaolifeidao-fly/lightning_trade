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
