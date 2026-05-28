package repository

import (
	"common/middleware/db"
	"fmt"
)

type ResourceRepository struct {
	db.Repository[*Resource]
}

func (r *ResourceRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&Resource{})
}

type RoleRepository struct {
	db.Repository[*Role]
}

func (r *RoleRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&Role{})
}

type RoleResourceRepository struct {
	db.Repository[*RoleResource]
}

func (r *RoleResourceRepository) EnsureTable() error {
	if r.Db == nil {
		return fmt.Errorf("database is not initialized")
	}
	return r.Db.AutoMigrate(&RoleResource{})
}
