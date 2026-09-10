package dto

import (
	baseDTO "common/base/dto"
	"time"
)

// UserDTO 是用户查询的**出参**。
//
// 这里刻意没有 Password / OriginPassword：口令只能写入、不能读出。
// 之前这两个字段挂在出参上，导致 GET /api/users 把口令哈希连同 origin_password
// 里的明文一起返回给任何登录态，列表页还专门渲染了一列「密码」把它显示出来。
// 设置口令走 CreateUserDTO / UpdateUserDTO 的入参，与出参分离。
type UserDTO struct {
	baseDTO.BaseDTO
	Name          string    `json:"name"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	Department    string    `json:"department"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	LastLoginTime time.Time `json:"lastLoginTime"`
	SecretKey     string    `json:"secretKey"`
	Remark        string    `json:"remark"`
	PubToken      string    `json:"pubToken"`
	BanCount      uint32    `json:"banCount"`
	AccountID     int       `json:"accountId"`
	AccountStatus string    `json:"accountStatus"`
	BalanceAmount string    `json:"balanceAmount"`
}

type CreateUserDTO struct {
	Name          string    `json:"name"`
	Username      string    `json:"username"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	Department    string    `json:"department"`
	Role          string    `json:"role"`
	Password      string    `json:"password"`
	Status        string    `json:"status"`
	LastLoginTime time.Time `json:"lastLoginTime"`
	SecretKey     string    `json:"secretKey"`
	Remark        string    `json:"remark"`
	PubToken      string    `json:"pubToken"`
	BanCount      uint32    `json:"banCount"`
}

type UpdateUserDTO struct {
	Name          *string    `json:"name,omitempty"`
	Username      *string    `json:"username,omitempty"`
	Email         *string    `json:"email,omitempty"`
	Phone         *string    `json:"phone,omitempty"`
	Department    *string    `json:"department,omitempty"`
	Role          *string    `json:"role,omitempty"`
	Password      *string    `json:"password,omitempty"`
	Status        *string    `json:"status,omitempty"`
	LastLoginTime *time.Time `json:"lastLoginTime,omitempty"`
	SecretKey     *string    `json:"secretKey,omitempty"`
	Remark        *string    `json:"remark,omitempty"`
	PubToken      *string    `json:"pubToken,omitempty"`
	BanCount      *uint32    `json:"banCount,omitempty"`
}

// AccountDTO 资金账户出参。余额按**字符串**回传：列是 decimal(38,8)，
// 走 float64 会在前端再丢一次精度，而用户列表里的余额本来就是按串展示的。
type AccountDTO struct {
	baseDTO.BaseDTO
	UserID        uint64 `json:"userId"`
	AccountStatus string `json:"accountStatus"`
	BalanceAmount string `json:"balanceAmount"`
}

// CreateAccountDTO 新建资金账户。前端在 record.accountId 为空时走这条
// （用户管理页的首次充值、首次冻结）。
type CreateAccountDTO struct {
	UserID        uint64 `json:"userId"`
	AccountStatus string `json:"accountStatus"`
	BalanceAmount string `json:"balanceAmount"`
}

// UpdateAccountDTO 局部更新：前端一次只改一个字段（充值只发 balanceAmount，
// 冻结只发 accountStatus），所以两个都是指针，nil = 本次不改。
type UpdateAccountDTO struct {
	AccountStatus *string `json:"accountStatus,omitempty"`
	BalanceAmount *string `json:"balanceAmount,omitempty"`
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
