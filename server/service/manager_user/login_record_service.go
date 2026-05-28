package user

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	userDTO "service/manager_user/dto"
	userRepository "service/manager_user/repository"
	"strings"

	"gorm.io/gorm"
)

func (s *UserService) ListUserLoginRecords(query userDTO.UserLoginRecordQueryDTO) (*baseDTO.PageDTO[userDTO.UserLoginRecordDTO], error) {
	if s.userLoginRecordRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizeUserPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.userLoginRecordRepository.Db.Model(&userRepository.UserLoginRecord{}).Where("active = ?", 1)
	if query.UserID > 0 {
		dbQuery = dbQuery.Where("user_id = ?", query.UserID)
	}
	if value := strings.TrimSpace(query.IP); value != "" {
		dbQuery = dbQuery.Where("ip LIKE ?", "%"+value+"%")
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*userRepository.UserLoginRecord
	if err := dbQuery.Order("id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[userDTO.UserLoginRecordDTO](entities)), nil
}

func (s *UserService) GetUserLoginRecordByID(id uint) (*userDTO.UserLoginRecordDTO, error) {
	if s.userLoginRecordRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	entity, err := s.userLoginRecordRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[userDTO.UserLoginRecordDTO](entity), nil
}

func (s *UserService) CreateUserLoginRecord(req *userDTO.CreateUserLoginRecordDTO) (*userDTO.UserLoginRecordDTO, error) {
	if s.userLoginRecordRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if err := ensureUserExists(s.userRepository, req.UserID); err != nil {
		return nil, err
	}
	created, err := s.userLoginRecordRepository.Create(&userRepository.UserLoginRecord{
		IP:     strings.TrimSpace(req.IP),
		UserID: req.UserID,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[userDTO.UserLoginRecordDTO](created), nil
}

func (s *UserService) UpdateUserLoginRecord(id uint, req *userDTO.UpdateUserLoginRecordDTO) (*userDTO.UserLoginRecordDTO, error) {
	if s.userLoginRecordRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.userLoginRecordRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.IP != nil {
		entity.IP = strings.TrimSpace(*req.IP)
	}
	if req.UserID != nil {
		if err := ensureUserExists(s.userRepository, *req.UserID); err != nil {
			return nil, err
		}
		entity.UserID = *req.UserID
	}
	saved, err := s.userLoginRecordRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[userDTO.UserLoginRecordDTO](saved), nil
}

func (s *UserService) DeleteUserLoginRecord(id uint) error {
	if s.userLoginRecordRepository.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	entity, err := s.userLoginRecordRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.userLoginRecordRepository.SaveOrUpdate(entity)
	return err
}
