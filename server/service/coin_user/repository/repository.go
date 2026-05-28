package repository

import (
	"common/middleware/db"
	"fmt"
	coinUserDTO "service/coin_user/dto"
	"strings"

	"gorm.io/gorm"
)

type CoinUserRepository struct {
	db.Repository[*CoinUser]
}

func (r *CoinUserRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinUser{})
}

func (r *CoinUserRepository) FindByUsername(username string) (*CoinUser, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinUser
	if err := r.Db.Where("username = ? AND active = 1", username).First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *CoinUserRepository) FindByEmail(email string) (*CoinUser, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinUser
	if err := r.Db.Where("email = ? AND active = 1", email).First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *CoinUserRepository) FindByInviteCode(code string) (*CoinUser, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinUser
	if err := r.Db.Where("invite_code = ? AND active = 1", code).First(&entity).Error; err != nil {
		return nil, err
	}
	if entity.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &entity, nil
}

func (r *CoinUserRepository) CountCoinUsersByQuery(query coinUserDTO.CoinUserQueryDTO) (int64, error) {
	if r.Db == nil {
		return 0, fmt.Errorf("database is not initialized")
	}
	whereSQL, values := buildCoinUserWhere(query)
	sql := "SELECT id FROM coin_user " + whereSQL
	return r.CountBySQL(sql, values...)
}

func (r *CoinUserRepository) ListCoinUsersByQuery(query coinUserDTO.CoinUserQueryDTO, pageIndex, pageSize int) ([]CoinUserListRow, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	whereSQL, values := buildCoinUserWhere(query)
	sql := `SELECT id, active, created_time, updated_time, created_by, updated_by,
		platform_id, platform_code,
		username, nickname, email, phone, country, kyc_level, kyc_status, status,
		invite_code, inviter_id, last_login_ip, last_login_time, two_fa_enabled, remark
		FROM coin_user ` + whereSQL + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	values = append(values, pageSize, (pageIndex-1)*pageSize)
	var rows []CoinUserListRow
	if err := r.QueryBySQL(&rows, sql, values...); err != nil {
		return nil, err
	}
	return rows, nil
}

func buildCoinUserWhere(query coinUserDTO.CoinUserQueryDTO) (string, []interface{}) {
	clauses := []string{"WHERE active = 1"}
	values := make([]interface{}, 0, 14)

	if query.PlatformID > 0 {
		clauses = append(clauses, "platform_id = ?")
		values = append(values, query.PlatformID)
	}
	if value := strings.TrimSpace(query.PlatformCode); value != "" {
		clauses = append(clauses, "platform_code = ?")
		values = append(values, strings.ToLower(value))
	}
	if value := strings.TrimSpace(query.Search); value != "" {
		like := "%" + value + "%"
		clauses = append(clauses, "(username LIKE ? OR nickname LIKE ? OR email LIKE ? OR phone LIKE ? OR invite_code LIKE ?)")
		values = append(values, like, like, like, like, like)
	}
	if value := strings.TrimSpace(query.Username); value != "" {
		clauses = append(clauses, "username LIKE ?")
		values = append(values, "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Email); value != "" {
		clauses = append(clauses, "email LIKE ?")
		values = append(values, "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Phone); value != "" {
		clauses = append(clauses, "phone LIKE ?")
		values = append(values, "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Country); value != "" {
		clauses = append(clauses, "country = ?")
		values = append(values, value)
	}
	if value := strings.TrimSpace(query.KycStatus); value != "" {
		clauses = append(clauses, "kyc_status = ?")
		values = append(values, value)
	}
	if value := strings.TrimSpace(query.Status); value != "" {
		clauses = append(clauses, "status = ?")
		values = append(values, value)
	}
	if query.InviterID > 0 {
		clauses = append(clauses, "inviter_id = ?")
		values = append(values, query.InviterID)
	}

	return strings.Join(clauses, " AND "), values
}

type CoinUserAssetRepository struct {
	db.Repository[*CoinUserAsset]
}

func (r *CoinUserAssetRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinUserAsset{})
}

func (r *CoinUserAssetRepository) FindByUserAndCoin(userID, coinID uint64) (*CoinUserAsset, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinUserAsset
	if err := r.Db.Where("user_id = ? AND coin_id = ? AND active = 1", userID, coinID).First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *CoinUserAssetRepository) ListByUserID(userID uint64) ([]*CoinUserAsset, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinUserAsset
	if err := r.Db.Where("user_id = ? AND active = 1", userID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinUserPositionRepository struct {
	db.Repository[*CoinUserPosition]
}

func (r *CoinUserPositionRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinUserPosition{})
}

func (r *CoinUserPositionRepository) FindByUserAndSymbol(userID uint64, symbol string) (*CoinUserPosition, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var entity CoinUserPosition
	if err := r.Db.Where("user_id = ? AND symbol = ? AND active = 1", userID, symbol).First(&entity).Error; err != nil {
		return nil, err
	}
	return &entity, nil
}

func (r *CoinUserPositionRepository) ListByUserID(userID uint64) ([]*CoinUserPosition, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinUserPosition
	if err := r.Db.Where("user_id = ? AND active = 1", userID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CoinUserPositionRepository) ListOpenByUserID(userID uint64) ([]*CoinUserPosition, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinUserPosition
	if err := r.Db.Where("user_id = ? AND status = 'open' AND active = 1", userID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinUserPositionAnalysisRepository struct {
	db.Repository[*CoinUserPositionAnalysis]
}

func (r *CoinUserPositionAnalysisRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinUserPositionAnalysis{})
}

func (r *CoinUserPositionAnalysisRepository) ListByUserID(userID uint64) ([]*CoinUserPositionAnalysis, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinUserPositionAnalysis
	if err := r.Db.Where("user_id = ? AND active = 1", userID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *CoinUserPositionAnalysisRepository) ListByPositionID(positionID uint64) ([]*CoinUserPositionAnalysis, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	var rows []*CoinUserPositionAnalysis
	if err := r.Db.Where("position_id = ? AND active = 1", positionID).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

type CoinUserLoginRecordRepository struct {
	db.Repository[*CoinUserLoginRecord]
}

func (r *CoinUserLoginRecordRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&CoinUserLoginRecord{})
}

func (r *CoinUserLoginRecordRepository) ListByUserID(userID uint64, limit int) ([]*CoinUserLoginRecord, error) {
	if r.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if limit <= 0 {
		limit = 20
	}
	var rows []*CoinUserLoginRecord
	if err := r.Db.Where("user_id = ? AND active = 1", userID).Order("id DESC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}
