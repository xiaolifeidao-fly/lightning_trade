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
	values := make([]interface{}, 0, 10)

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
