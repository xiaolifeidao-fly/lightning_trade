package repository

import "common/middleware/db"

type Resource struct {
	db.BaseEntity
	Name         string `gorm:"column:name;type:varchar(50)" orm:"column(name);size(50);null" description:"资源名称"`
	Code         string `gorm:"column:code;type:varchar(50);index:idx_code" orm:"column(code);size(50);null" description:"资源编码"`
	ParentID     uint64 `gorm:"column:parent_id;type:bigint unsigned;index:idx_parent_id" orm:"column(parent_id);null" description:"父资源ID"`
	ResourceType string `gorm:"column:resource_type;type:varchar(50)" orm:"column(resource_type);size(50);null" description:"资源类型"`
	ResourceURL  string `gorm:"column:resource_url;type:varchar(200)" orm:"column(resource_url);size(200);null" description:"资源URL"`
	PageURL      string `gorm:"column:page_url;type:varchar(200)" orm:"column(page_url);size(200);null" description:"页面URL"`
	Component    string `gorm:"column:component;type:varchar(50)" orm:"column(component);size(50);null" description:"组件"`
	Redirect     string `gorm:"column:redirect;type:varchar(50)" orm:"column(redirect);size(50);null" description:"重定向"`
	MenuName     string `gorm:"column:menu_name;type:varchar(50)" orm:"column(menu_name);size(50);null" description:"菜单名称"`
	Meta         string `gorm:"column:meta;type:varchar(50)" orm:"column(meta);size(50);null" description:"元信息"`
	SortID       int    `gorm:"column:sort_id;type:int" orm:"column(sort_id);null" description:"排序"`
}

func (r *Resource) TableName() string {
	return "resource_new"
}

type Role struct {
	db.BaseEntity
	Name string `gorm:"column:name;type:varchar(50)" orm:"column(name);size(50);null" description:"角色名称"`
	Code string `gorm:"column:code;type:varchar(50);index:idx_code" orm:"column(code);size(50);null" description:"角色编码"`
}

func (r *Role) TableName() string {
	return "role"
}

type RoleResource struct {
	db.BaseEntity
	RoleID     uint64 `gorm:"column:role_id;type:bigint unsigned;index:idx_role_id" orm:"column(role_id);null" description:"角色ID"`
	ResourceID uint64 `gorm:"column:resource_id;type:bigint unsigned" orm:"column(resource_id);null" description:"资源ID"`
}

func (r *RoleResource) TableName() string {
	return "role_resource_new"
}
