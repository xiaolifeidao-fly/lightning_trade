package permission

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	permissionDTO "service/manager_permission/dto"
	permissionRepository "service/manager_permission/repository"
	"strings"

	"gorm.io/gorm"
)

func (s *PermissionService) ListRoles(query permissionDTO.RoleQueryDTO) (*baseDTO.PageDTO[permissionDTO.RoleDTO], error) {
	if s.roleRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePermissionPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.roleRepository.Db.Model(&permissionRepository.Role{}).Where("active = ?", 1)
	if value := strings.TrimSpace(query.Name); value != "" {
		dbQuery = dbQuery.Where("name LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Code); value != "" {
		dbQuery = dbQuery.Where("code LIKE ?", "%"+value+"%")
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*permissionRepository.Role
	if err := dbQuery.Order("id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[permissionDTO.RoleDTO](entities)), nil
}

func (s *PermissionService) GetRoleByID(id uint) (*permissionDTO.RoleDTO, error) {
	entity, err := s.roleRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[permissionDTO.RoleDTO](entity), nil
}

func (s *PermissionService) CreateRole(req *permissionDTO.CreateRoleDTO) (*permissionDTO.RoleDTO, error) {
	if s.roleRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	created, err := s.roleRepository.Create(&permissionRepository.Role{
		Name: strings.TrimSpace(req.Name),
		Code: strings.TrimSpace(req.Code),
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[permissionDTO.RoleDTO](created), nil
}

func (s *PermissionService) UpdateRole(id uint, req *permissionDTO.UpdateRoleDTO) (*permissionDTO.RoleDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.roleRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.Name != nil {
		entity.Name = strings.TrimSpace(*req.Name)
	}
	if req.Code != nil {
		entity.Code = strings.TrimSpace(*req.Code)
	}
	saved, err := s.roleRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[permissionDTO.RoleDTO](saved), nil
}

func (s *PermissionService) DeleteRole(id uint) error {
	entity, err := s.roleRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.roleRepository.SaveOrUpdate(entity)
	return err
}
