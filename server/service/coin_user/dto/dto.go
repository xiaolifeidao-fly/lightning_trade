package dto

import (
	baseDTO "common/base/dto"
	"time"
)

type CoinUserDTO struct {
	baseDTO.BaseDTO
	Username      string    `json:"username"`
	Nickname      string    `json:"nickname"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	Country       string    `json:"country"`
	KycLevel      uint8     `json:"kycLevel"`
	KycStatus     string    `json:"kycStatus"`
	Status        string    `json:"status"`
	InviteCode    string    `json:"inviteCode"`
	InviterID     uint64    `json:"inviterId"`
	LastLoginIP   string    `json:"lastLoginIp"`
	LastLoginTime time.Time `json:"lastLoginTime"`
	TwoFAEnabled  uint8     `json:"twoFaEnabled"`
	Remark        string    `json:"remark"`
}

type CreateCoinUserDTO struct {
	Username   string `json:"username"`
	Nickname   string `json:"nickname"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	Password   string `json:"password"`
	Country    string `json:"country"`
	InviteCode string `json:"inviteCode"`
	InviterID  uint64 `json:"inviterId"`
	Remark     string `json:"remark"`
}

type UpdateCoinUserDTO struct {
	Nickname     *string `json:"nickname,omitempty"`
	Email        *string `json:"email,omitempty"`
	Phone        *string `json:"phone,omitempty"`
	Country      *string `json:"country,omitempty"`
	KycLevel     *uint8  `json:"kycLevel,omitempty"`
	KycStatus    *string `json:"kycStatus,omitempty"`
	Status       *string `json:"status,omitempty"`
	TwoFAEnabled *uint8  `json:"twoFaEnabled,omitempty"`
	Remark       *string `json:"remark,omitempty"`
}

type CoinUserQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Search    string `form:"search"`
	Username  string `form:"username"`
	Email     string `form:"email"`
	Phone     string `form:"phone"`
	Country   string `form:"country"`
	KycStatus string `form:"kycStatus"`
	Status    string `form:"status"`
	InviterID uint64 `form:"inviterId"`
}

type CoinUserAssetDTO struct {
	baseDTO.BaseDTO
	UserID         uint64  `json:"userId"`
	CoinID         uint64  `json:"coinId"`
	CoinCode       string  `json:"coinCode"`
	Available      float64 `json:"available"`
	Frozen         float64 `json:"frozen"`
	Total          float64 `json:"total"`
	Address        string  `json:"address"`
	WithdrawEnable uint8   `json:"withdrawEnable"`
}

type CreateCoinUserAssetDTO struct {
	UserID   uint64 `json:"userId"`
	CoinID   uint64 `json:"coinId"`
	CoinCode string `json:"coinCode"`
	Address  string `json:"address"`
}

type UpdateCoinUserAssetDTO struct {
	Available      *float64 `json:"available,omitempty"`
	Frozen         *float64 `json:"frozen,omitempty"`
	Total          *float64 `json:"total,omitempty"`
	Address        *string  `json:"address,omitempty"`
	WithdrawEnable *uint8   `json:"withdrawEnable,omitempty"`
}

type CoinUserAssetQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	UserID    uint64 `form:"userId"`
	CoinID    uint64 `form:"coinId"`
	CoinCode  string `form:"coinCode"`
}

type CoinUserLoginRecordDTO struct {
	baseDTO.BaseDTO
	UserID   uint64 `json:"userId"`
	IP       string `json:"ip"`
	Device   string `json:"device"`
	Location string `json:"location"`
	Success  uint8  `json:"success"`
}

type CreateCoinUserLoginRecordDTO struct {
	UserID   uint64 `json:"userId"`
	IP       string `json:"ip"`
	Device   string `json:"device"`
	Location string `json:"location"`
	Success  uint8  `json:"success"`
}

type CoinUserLoginRecordQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	UserID    uint64 `form:"userId"`
	IP        string `form:"ip"`
}

type CoinUserStatsDTO struct {
	TotalUsers      int `json:"totalUsers"`
	ActiveUsers     int `json:"activeUsers"`
	KycApprovedUsers int `json:"kycApprovedUsers"`
	NewUsersToday   int `json:"newUsersToday"`
}
