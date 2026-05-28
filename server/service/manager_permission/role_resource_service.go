package permission

import (
	baseDTO "common/base/dto"
	"common/middleware/db"
	"fmt"
	permissionDTO "service/manager_permission/dto"
	permissionRepository "service/manager_permission/repository"

	"gorm.io/gorm"
)

func (s *PermissionService) ListRoleResources(query permissionDTO.RoleResourceQueryDTO) (*baseDTO.PageDTO[permissionDTO.RoleResourceDTO], error) {
	if s.roleResourceRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePermissionPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.roleResourceRepository.Db.Model(&permissionRepository.RoleResource{}).Where("active = ?", 1)
	if query.RoleID > 0 {
		dbQuery = dbQuery.Where("role_id = ?", query.RoleID)
	}
	if query.ResourceID > 0 {
		dbQuery = dbQuery.Where("resource_id = ?", query.ResourceID)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*permissionRepository.RoleResource
	if err := dbQuery.Order("id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[permissionDTO.RoleResourceDTO](entities)), nil
}

func (s *PermissionService) GetRoleResourceByID(id uint) (*permissionDTO.RoleResourceDTO, error) {
	entity, err := s.roleResourceRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[permissionDTO.RoleResourceDTO](entity), nil
}

func (s *PermissionService) CreateRoleResource(req *permissionDTO.CreateRoleResourceDTO) (*permissionDTO.RoleResourceDTO, error) {
	if s.roleResourceRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	created, err := s.roleResourceRepository.Create(&permissionRepository.RoleResource{
		RoleID:     req.RoleID,
		ResourceID: req.ResourceID,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[permissionDTO.RoleResourceDTO](created), nil
}

func (s *PermissionService) UpdateRoleResource(id uint, req *permissionDTO.UpdateRoleResourceDTO) (*permissionDTO.RoleResourceDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.roleResourceRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if req.RoleID != nil {
		entity.RoleID = *req.RoleID
	}
	if req.ResourceID != nil {
		entity.ResourceID = *req.ResourceID
	}
	saved, err := s.roleResourceRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[permissionDTO.RoleResourceDTO](saved), nil
}

func (s *PermissionService) DeleteRoleResource(id uint) error {
	entity, err := s.roleResourceRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.roleResourceRepository.SaveOrUpdate(entity)
	return err
}
