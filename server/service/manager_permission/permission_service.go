package permission

import (
	"common/middleware/db"
	permissionRepository "service/manager_permission/repository"
)

type PermissionService struct {
	resourceRepository     *permissionRepository.ResourceRepository
	roleRepository         *permissionRepository.RoleRepository
	roleResourceRepository *permissionRepository.RoleResourceRepository
}

func NewPermissionService() *PermissionService {
	return &PermissionService{
		resourceRepository:     db.GetRepository[permissionRepository.ResourceRepository](),
		roleRepository:         db.GetRepository[permissionRepository.RoleRepository](),
		roleResourceRepository: db.GetRepository[permissionRepository.RoleResourceRepository](),
	}
}

func (s *PermissionService) EnsureTable() error {
	if err := s.resourceRepository.EnsureTable(); err != nil {
		return err
	}
	if err := s.roleRepository.EnsureTable(); err != nil {
		return err
	}
	return s.roleResourceRepository.EnsureTable()
}

func normalizePermissionPage(page, pageIndex, pageSize int) (int, int) {
	if pageIndex <= 0 {
		pageIndex = page
	}
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return pageIndex, pageSize
}
