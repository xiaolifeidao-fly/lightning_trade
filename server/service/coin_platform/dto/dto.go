package dto

import (
	baseDTO "common/base/dto"
	"time"
)

type CoinPlatformDTO struct {
	baseDTO.BaseDTO
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	FullName        string  `json:"fullName"`
	Icon            string  `json:"icon"`
	Website         string  `json:"website"`
	Country         string  `json:"country"`
	ApiBaseURL      string  `json:"apiBaseUrl"`
	WsBaseURL       string  `json:"wsBaseUrl"`
	DocsURL         string  `json:"docsUrl"`
	SupportedTypes  string  `json:"supportedTypes"`
	DefaultFeeRate  float64 `json:"defaultFeeRate"`
	RateLimitPerSec uint32  `json:"rateLimitPerSec"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sortOrder"`
	Description     string  `json:"description"`
}

type CreateCoinPlatformDTO struct {
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	FullName        string  `json:"fullName"`
	Icon            string  `json:"icon"`
	Website         string  `json:"website"`
	Country         string  `json:"country"`
	ApiBaseURL      string  `json:"apiBaseUrl"`
	WsBaseURL       string  `json:"wsBaseUrl"`
	DocsURL         string  `json:"docsUrl"`
	SupportedTypes  string  `json:"supportedTypes"`
	DefaultFeeRate  float64 `json:"defaultFeeRate"`
	RateLimitPerSec uint32  `json:"rateLimitPerSec"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sortOrder"`
	Description     string  `json:"description"`
}

type UpdateCoinPlatformDTO struct {
	Name            *string  `json:"name,omitempty"`
	FullName        *string  `json:"fullName,omitempty"`
	Icon            *string  `json:"icon,omitempty"`
	Website         *string  `json:"website,omitempty"`
	Country         *string  `json:"country,omitempty"`
	ApiBaseURL      *string  `json:"apiBaseUrl,omitempty"`
	WsBaseURL       *string  `json:"wsBaseUrl,omitempty"`
	DocsURL         *string  `json:"docsUrl,omitempty"`
	SupportedTypes  *string  `json:"supportedTypes,omitempty"`
	DefaultFeeRate  *float64 `json:"defaultFeeRate,omitempty"`
	RateLimitPerSec *uint32  `json:"rateLimitPerSec,omitempty"`
	Status          *string  `json:"status,omitempty"`
	SortOrder       *int     `json:"sortOrder,omitempty"`
	Description     *string  `json:"description,omitempty"`
}

type CoinPlatformQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Code      string `form:"code"`
	Name      string `form:"name"`
	Country   string `form:"country"`
	Status    string `form:"status"`
}

type CoinPlatformCoinDTO struct {
	baseDTO.BaseDTO
	PlatformID      uint64  `json:"platformId"`
	PlatformCode    string  `json:"platformCode"`
	CoinID          uint64  `json:"coinId"`
	CoinCode        string  `json:"coinCode"`
	PlatformSymbol  string  `json:"platformSymbol"`
	ChainName       string  `json:"chainName"`
	ContractAddress string  `json:"contractAddress"`
	DepositEnable   uint8   `json:"depositEnable"`
	WithdrawEnable  uint8   `json:"withdrawEnable"`
	TradeEnable     uint8   `json:"tradeEnable"`
	MinWithdrawal   float64 `json:"minWithdrawal"`
	WithdrawalFee   float64 `json:"withdrawalFee"`
	Confirmations   uint32  `json:"confirmations"`
}

type CreateCoinPlatformCoinDTO struct {
	PlatformID      uint64  `json:"platformId"`
	PlatformCode    string  `json:"platformCode"`
	CoinID          uint64  `json:"coinId"`
	CoinCode        string  `json:"coinCode"`
	PlatformSymbol  string  `json:"platformSymbol"`
	ChainName       string  `json:"chainName"`
	ContractAddress string  `json:"contractAddress"`
	DepositEnable   uint8   `json:"depositEnable"`
	WithdrawEnable  uint8   `json:"withdrawEnable"`
	TradeEnable     uint8   `json:"tradeEnable"`
	MinWithdrawal   float64 `json:"minWithdrawal"`
	WithdrawalFee   float64 `json:"withdrawalFee"`
	Confirmations   uint32  `json:"confirmations"`
}

type UpdateCoinPlatformCoinDTO struct {
	PlatformSymbol  *string  `json:"platformSymbol,omitempty"`
	ChainName       *string  `json:"chainName,omitempty"`
	ContractAddress *string  `json:"contractAddress,omitempty"`
	DepositEnable   *uint8   `json:"depositEnable,omitempty"`
	WithdrawEnable  *uint8   `json:"withdrawEnable,omitempty"`
	TradeEnable     *uint8   `json:"tradeEnable,omitempty"`
	MinWithdrawal   *float64 `json:"minWithdrawal,omitempty"`
	WithdrawalFee   *float64 `json:"withdrawalFee,omitempty"`
	Confirmations   *uint32  `json:"confirmations,omitempty"`
}

type CoinPlatformCoinQueryDTO struct {
	Page       int    `form:"page"`
	PageIndex  int    `form:"pageIndex"`
	PageSize   int    `form:"pageSize"`
	PlatformID uint64 `form:"platformId"`
	CoinID     uint64 `form:"coinId"`
	CoinCode   string `form:"coinCode"`
	ChainName  string `form:"chainName"`
}

type CoinPlatformAccountDTO struct {
	baseDTO.BaseDTO
	PlatformID   uint64    `json:"platformId"`
	PlatformCode string    `json:"platformCode"`
	AccountName  string    `json:"accountName"`
	AccountType  string    `json:"accountType"`
	ApiKey       string    `json:"apiKey"`
	IPWhitelist  string    `json:"ipWhitelist"`
	Permissions  string    `json:"permissions"`
	Status       string    `json:"status"`
	LastUsedTime time.Time `json:"lastUsedTime"`
	ExpireTime   time.Time `json:"expireTime"`
	Remark       string    `json:"remark"`
}

type CreateCoinPlatformAccountDTO struct {
	PlatformID   uint64    `json:"platformId"`
	PlatformCode string    `json:"platformCode"`
	AccountName  string    `json:"accountName"`
	AccountType  string    `json:"accountType"`
	ApiKey       string    `json:"apiKey"`
	ApiSecret    string    `json:"apiSecret"`
	Passphrase   string    `json:"passphrase"`
	IPWhitelist  string    `json:"ipWhitelist"`
	Permissions  string    `json:"permissions"`
	ExpireTime   time.Time `json:"expireTime"`
	Remark       string    `json:"remark"`
}

type UpdateCoinPlatformAccountDTO struct {
	AccountName *string    `json:"accountName,omitempty"`
	AccountType *string    `json:"accountType,omitempty"`
	ApiKey      *string    `json:"apiKey,omitempty"`
	ApiSecret   *string    `json:"apiSecret,omitempty"`
	Passphrase  *string    `json:"passphrase,omitempty"`
	IPWhitelist *string    `json:"ipWhitelist,omitempty"`
	Permissions *string    `json:"permissions,omitempty"`
	Status      *string    `json:"status,omitempty"`
	ExpireTime  *time.Time `json:"expireTime,omitempty"`
	Remark      *string    `json:"remark,omitempty"`
}

type CoinPlatformAccountQueryDTO struct {
	Page        int    `form:"page"`
	PageIndex   int    `form:"pageIndex"`
	PageSize    int    `form:"pageSize"`
	PlatformID  uint64 `form:"platformId"`
	AccountName string `form:"accountName"`
	Status      string `form:"status"`
}
