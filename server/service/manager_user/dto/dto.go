package dto

import (
	baseDTO "common/base/dto"
	"time"
)

type UserDTO struct {
	baseDTO.BaseDTO
	Name           string    `json:"name"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Department     string    `json:"department"`
	Role           string    `json:"role"`
	Password       string    `json:"password"`
	OriginPassword string    `json:"originPassword"`
	Status         string    `json:"status"`
	LastLoginTime  time.Time `json:"lastLoginTime"`
	SecretKey      string    `json:"secretKey"`
	Remark         string    `json:"remark"`
	PubToken       string    `json:"pubToken"`
	BanCount       uint32    `json:"banCount"`
	AccountID      int       `json:"accountId"`
	AccountStatus  string    `json:"accountStatus"`
	BalanceAmount  string    `json:"balanceAmount"`
}

type CreateUserDTO struct {
	Name           string    `json:"name"`
	Username       string    `json:"username"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Department     string    `json:"department"`
	Role           string    `json:"role"`
	Password       string    `json:"password"`
	OriginPassword string    `json:"originPassword"`
	Status         string    `json:"status"`
	LastLoginTime  time.Time `json:"lastLoginTime"`
	SecretKey      string    `json:"secretKey"`
	Remark         string    `json:"remark"`
	PubToken       string    `json:"pubToken"`
	BanCount       uint32    `json:"banCount"`
}

type UpdateUserDTO struct {
	Name           *string    `json:"name,omitempty"`
	Username       *string    `json:"username,omitempty"`
	Email          *string    `json:"email,omitempty"`
	Phone          *string    `json:"phone,omitempty"`
	Department     *string    `json:"department,omitempty"`
	Role           *string    `json:"role,omitempty"`
	Password       *string    `json:"password,omitempty"`
	OriginPassword *string    `json:"originPassword,omitempty"`
	Status         *string    `json:"status,omitempty"`
	LastLoginTime  *time.Time `json:"lastLoginTime,omitempty"`
	SecretKey      *string    `json:"secretKey,omitempty"`
	Remark         *string    `json:"remark,omitempty"`
	PubToken       *string    `json:"pubToken,omitempty"`
	BanCount       *uint32    `json:"banCount,omitempty"`
}

type UserQueryDTO struct {
	Page       int    `form:"page"`
	PageIndex  int    `form:"pageIndex"`
	PageSize   int    `form:"pageSize"`
	Search     string `form:"search"`
	Name       string `form:"name"`
	Username   string `form:"username"`
	Email      string `form:"email"`
	Phone      string `form:"phone"`
	Department string `form:"department"`
	Role       string `form:"role"`
	Status     string `form:"status"`
	SecretKey  string `form:"secretKey"`
	PubToken   string `form:"pubToken"`
}

type UserStatsDTO struct {
	VisibleUsers     int `json:"visibleUsers"`
	AccountCount     int `json:"accountCount"`
	PrivilegedUsers  int `json:"privilegedUsers"`
	RecentLoginUsers int `json:"recentLoginUsers"`
	ActiveUsers      int `json:"activeUsers"`
}

type UserLoginRecordDTO struct {
	baseDTO.BaseDTO
	IP     string `json:"ip"`
	UserID uint64 `json:"userId"`
}

type CreateUserLoginRecordDTO struct {
	IP     string `json:"ip"`
	UserID uint64 `json:"userId"`
}

type UpdateUserLoginRecordDTO struct {
	IP     *string `json:"ip,omitempty"`
	UserID *uint64 `json:"userId,omitempty"`
}

type UserLoginRecordQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	UserID    uint64 `form:"userId"`
	IP        string `form:"ip"`
}

type UserRoleDTO struct {
	baseDTO.BaseDTO
	UserID uint64 `json:"userId"`
	RoleID uint64 `json:"roleId"`
}

type CreateUserRoleDTO struct {
	UserID uint64 `json:"userId"`
	RoleID uint64 `json:"roleId"`
}

type UpdateUserRoleDTO struct {
	UserID *uint64 `json:"userId,omitempty"`
	RoleID *uint64 `json:"roleId,omitempty"`
}

type UserRoleQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	UserID    uint64 `form:"userId"`
	RoleID    uint64 `form:"roleId"`
}
