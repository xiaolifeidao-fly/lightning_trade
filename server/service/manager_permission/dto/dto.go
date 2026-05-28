package dto

import baseDTO "common/base/dto"

type ResourceDTO struct {
	baseDTO.BaseDTO
	Name         string `json:"name"`
	Code         string `json:"code"`
	ParentID     uint64 `json:"parentId"`
	ResourceType string `json:"resourceType"`
	ResourceURL  string `json:"resourceUrl"`
	PageURL      string `json:"pageUrl"`
	Component    string `json:"component"`
	Redirect     string `json:"redirect"`
	MenuName     string `json:"menuName"`
	Meta         string `json:"meta"`
	SortID       int    `json:"sortId"`
}

type CreateResourceDTO struct {
	Name         string `json:"name"`
	Code         string `json:"code"`
	ParentID     uint64 `json:"parentId"`
	ResourceType string `json:"resourceType"`
	ResourceURL  string `json:"resourceUrl"`
	PageURL      string `json:"pageUrl"`
	Component    string `json:"component"`
	Redirect     string `json:"redirect"`
	MenuName     string `json:"menuName"`
	Meta         string `json:"meta"`
	SortID       int    `json:"sortId"`
}

type UpdateResourceDTO struct {
	Name         *string `json:"name,omitempty"`
	Code         *string `json:"code,omitempty"`
	ParentID     *uint64 `json:"parentId,omitempty"`
	ResourceType *string `json:"resourceType,omitempty"`
	ResourceURL  *string `json:"resourceUrl,omitempty"`
	PageURL      *string `json:"pageUrl,omitempty"`
	Component    *string `json:"component,omitempty"`
	Redirect     *string `json:"redirect,omitempty"`
	MenuName     *string `json:"menuName,omitempty"`
	Meta         *string `json:"meta,omitempty"`
	SortID       *int    `json:"sortId,omitempty"`
}

type ResourceQueryDTO struct {
	Page         int    `form:"page"`
	PageIndex    int    `form:"pageIndex"`
	PageSize     int    `form:"pageSize"`
	Name         string `form:"name"`
	Code         string `form:"code"`
	ParentID     uint64 `form:"parentId"`
	ResourceType string `form:"resourceType"`
}

type RoleDTO struct {
	baseDTO.BaseDTO
	Name string `json:"name"`
	Code string `json:"code"`
}

type CreateRoleDTO struct {
	Name string `json:"name"`
	Code string `json:"code"`
}

type UpdateRoleDTO struct {
	Name *string `json:"name,omitempty"`
	Code *string `json:"code,omitempty"`
}

type RoleQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Name      string `form:"name"`
	Code      string `form:"code"`
}

type RoleResourceDTO struct {
	baseDTO.BaseDTO
	RoleID     uint64 `json:"roleId"`
	ResourceID uint64 `json:"resourceId"`
}

type CreateRoleResourceDTO struct {
	RoleID     uint64 `json:"roleId"`
	ResourceID uint64 `json:"resourceId"`
}

type UpdateRoleResourceDTO struct {
	RoleID     *uint64 `json:"roleId,omitempty"`
	ResourceID *uint64 `json:"resourceId,omitempty"`
}

type RoleResourceQueryDTO struct {
	Page       int    `form:"page"`
	PageIndex  int    `form:"pageIndex"`
	PageSize   int    `form:"pageSize"`
	RoleID     uint64 `form:"roleId"`
	ResourceID uint64 `form:"resourceId"`
}
