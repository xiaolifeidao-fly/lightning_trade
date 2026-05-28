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

func (s *PermissionService) ListResources(query permissionDTO.ResourceQueryDTO) (*baseDTO.PageDTO[permissionDTO.ResourceDTO], error) {
	if s.resourceRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	pageIndex, pageSize := normalizePermissionPage(query.Page, query.PageIndex, query.PageSize)
	dbQuery := s.resourceRepository.Db.Model(&permissionRepository.Resource{}).Where("active = ?", 1)
	if value := strings.TrimSpace(query.Name); value != "" {
		dbQuery = dbQuery.Where("name LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(query.Code); value != "" {
		dbQuery = dbQuery.Where("code LIKE ?", "%"+value+"%")
	}
	if query.ParentID > 0 {
		dbQuery = dbQuery.Where("parent_id = ?", query.ParentID)
	}
	if value := strings.TrimSpace(query.ResourceType); value != "" {
		dbQuery = dbQuery.Where("resource_type = ?", value)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, err
	}
	var entities []*permissionRepository.Resource
	if err := dbQuery.Order("sort_id ASC, id DESC").Offset((pageIndex - 1) * pageSize).Limit(pageSize).Find(&entities).Error; err != nil {
		return nil, err
	}
	return baseDTO.BuildPage(int(total), db.ToDTOs[permissionDTO.ResourceDTO](entities)), nil
}

func (s *PermissionService) GetResourceByID(id uint) (*permissionDTO.ResourceDTO, error) {
	entity, err := s.resourceRepository.FindById(id)
	if err != nil {
		return nil, err
	}
	if entity.Active == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return db.ToDTO[permissionDTO.ResourceDTO](entity), nil
}

func (s *PermissionService) CreateResource(req *permissionDTO.CreateResourceDTO) (*permissionDTO.ResourceDTO, error) {
	if s.resourceRepository.Db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	created, err := s.resourceRepository.Create(&permissionRepository.Resource{
		Name:         strings.TrimSpace(req.Name),
		Code:         strings.TrimSpace(req.Code),
		ParentID:     req.ParentID,
		ResourceType: strings.TrimSpace(req.ResourceType),
		ResourceURL:  strings.TrimSpace(req.ResourceURL),
		PageURL:      strings.TrimSpace(req.PageURL),
		Component:    strings.TrimSpace(req.Component),
		Redirect:     strings.TrimSpace(req.Redirect),
		MenuName:     strings.TrimSpace(req.MenuName),
		Meta:         strings.TrimSpace(req.Meta),
		SortID:       req.SortID,
	})
	if err != nil {
		return nil, err
	}
	return db.ToDTO[permissionDTO.ResourceDTO](created), nil
}

func (s *PermissionService) UpdateResource(id uint, req *permissionDTO.UpdateResourceDTO) (*permissionDTO.ResourceDTO, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	entity, err := s.resourceRepository.FindById(id)
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
	if req.ParentID != nil {
		entity.ParentID = *req.ParentID
	}
	if req.ResourceType != nil {
		entity.ResourceType = strings.TrimSpace(*req.ResourceType)
	}
	if req.ResourceURL != nil {
		entity.ResourceURL = strings.TrimSpace(*req.ResourceURL)
	}
	if req.PageURL != nil {
		entity.PageURL = strings.TrimSpace(*req.PageURL)
	}
	if req.Component != nil {
		entity.Component = strings.TrimSpace(*req.Component)
	}
	if req.Redirect != nil {
		entity.Redirect = strings.TrimSpace(*req.Redirect)
	}
	if req.MenuName != nil {
		entity.MenuName = strings.TrimSpace(*req.MenuName)
	}
	if req.Meta != nil {
		entity.Meta = strings.TrimSpace(*req.Meta)
	}
	if req.SortID != nil {
		entity.SortID = *req.SortID
	}
	saved, err := s.resourceRepository.SaveOrUpdate(entity)
	if err != nil {
		return nil, err
	}
	return db.ToDTO[permissionDTO.ResourceDTO](saved), nil
}

func (s *PermissionService) DeleteResource(id uint) error {
	entity, err := s.resourceRepository.FindById(id)
	if err != nil {
		return err
	}
	if entity.Active == 0 {
		return gorm.ErrRecordNotFound
	}
	entity.Active = 0
	_, err = s.resourceRepository.SaveOrUpdate(entity)
	return err
}
