package dto

import (
	baseDTO "common/base/dto"
)

type CoinDTO struct {
	baseDTO.BaseDTO
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	FullName        string  `json:"fullName"`
	Icon            string  `json:"icon"`
	ChainName       string  `json:"chainName"`
	ContractAddress string  `json:"contractAddress"`
	Decimals        uint8   `json:"decimals"`
	PricePrecision  uint8   `json:"pricePrecision"`
	AmountPrecision uint8   `json:"amountPrecision"`
	MinWithdrawal   float64 `json:"minWithdrawal"`
	MaxWithdrawal   float64 `json:"maxWithdrawal"`
	WithdrawalFee   float64 `json:"withdrawalFee"`
	DepositEnable   uint8   `json:"depositEnable"`
	WithdrawEnable  uint8   `json:"withdrawEnable"`
	TradeEnable     uint8   `json:"tradeEnable"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sortOrder"`
	Description     string  `json:"description"`
}

type CreateCoinDTO struct {
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	FullName        string  `json:"fullName"`
	Icon            string  `json:"icon"`
	ChainName       string  `json:"chainName"`
	ContractAddress string  `json:"contractAddress"`
	Decimals        uint8   `json:"decimals"`
	PricePrecision  uint8   `json:"pricePrecision"`
	AmountPrecision uint8   `json:"amountPrecision"`
	MinWithdrawal   float64 `json:"minWithdrawal"`
	MaxWithdrawal   float64 `json:"maxWithdrawal"`
	WithdrawalFee   float64 `json:"withdrawalFee"`
	DepositEnable   uint8   `json:"depositEnable"`
	WithdrawEnable  uint8   `json:"withdrawEnable"`
	TradeEnable     uint8   `json:"tradeEnable"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sortOrder"`
	Description     string  `json:"description"`
}

type UpdateCoinDTO struct {
	Name            *string  `json:"name,omitempty"`
	FullName        *string  `json:"fullName,omitempty"`
	Icon            *string  `json:"icon,omitempty"`
	ChainName       *string  `json:"chainName,omitempty"`
	ContractAddress *string  `json:"contractAddress,omitempty"`
	Decimals        *uint8   `json:"decimals,omitempty"`
	PricePrecision  *uint8   `json:"pricePrecision,omitempty"`
	AmountPrecision *uint8   `json:"amountPrecision,omitempty"`
	MinWithdrawal   *float64 `json:"minWithdrawal,omitempty"`
	MaxWithdrawal   *float64 `json:"maxWithdrawal,omitempty"`
	WithdrawalFee   *float64 `json:"withdrawalFee,omitempty"`
	DepositEnable   *uint8   `json:"depositEnable,omitempty"`
	WithdrawEnable  *uint8   `json:"withdrawEnable,omitempty"`
	TradeEnable     *uint8   `json:"tradeEnable,omitempty"`
	Status          *string  `json:"status,omitempty"`
	SortOrder       *int     `json:"sortOrder,omitempty"`
	Description     *string  `json:"description,omitempty"`
}

type CoinQueryDTO struct {
	Page      int    `form:"page"`
	PageIndex int    `form:"pageIndex"`
	PageSize  int    `form:"pageSize"`
	Code      string `form:"code"`
	Name      string `form:"name"`
	ChainName string `form:"chainName"`
	Status    string `form:"status"`
}

type CoinPairDTO struct {
	baseDTO.BaseDTO
	Symbol          string  `json:"symbol"`
	BaseCoinID      uint64  `json:"baseCoinId"`
	BaseCoinCode    string  `json:"baseCoinCode"`
	QuoteCoinID     uint64  `json:"quoteCoinId"`
	QuoteCoinCode   string  `json:"quoteCoinCode"`
	PricePrecision  uint8   `json:"pricePrecision"`
	AmountPrecision uint8   `json:"amountPrecision"`
	MinAmount       float64 `json:"minAmount"`
	MaxAmount       float64 `json:"maxAmount"`
	MinTotal        float64 `json:"minTotal"`
	TakerFeeRate    float64 `json:"takerFeeRate"`
	MakerFeeRate    float64 `json:"makerFeeRate"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sortOrder"`
}

type CreateCoinPairDTO struct {
	Symbol          string  `json:"symbol"`
	BaseCoinID      uint64  `json:"baseCoinId"`
	BaseCoinCode    string  `json:"baseCoinCode"`
	QuoteCoinID     uint64  `json:"quoteCoinId"`
	QuoteCoinCode   string  `json:"quoteCoinCode"`
	PricePrecision  uint8   `json:"pricePrecision"`
	AmountPrecision uint8   `json:"amountPrecision"`
	MinAmount       float64 `json:"minAmount"`
	MaxAmount       float64 `json:"maxAmount"`
	MinTotal        float64 `json:"minTotal"`
	TakerFeeRate    float64 `json:"takerFeeRate"`
	MakerFeeRate    float64 `json:"makerFeeRate"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sortOrder"`
}

type UpdateCoinPairDTO struct {
	PricePrecision  *uint8   `json:"pricePrecision,omitempty"`
	AmountPrecision *uint8   `json:"amountPrecision,omitempty"`
	MinAmount       *float64 `json:"minAmount,omitempty"`
	MaxAmount       *float64 `json:"maxAmount,omitempty"`
	MinTotal        *float64 `json:"minTotal,omitempty"`
	TakerFeeRate    *float64 `json:"takerFeeRate,omitempty"`
	MakerFeeRate    *float64 `json:"makerFeeRate,omitempty"`
	Status          *string  `json:"status,omitempty"`
	SortOrder       *int     `json:"sortOrder,omitempty"`
}

type CoinPairQueryDTO struct {
	Page          int    `form:"page"`
	PageIndex     int    `form:"pageIndex"`
	PageSize      int    `form:"pageSize"`
	Symbol        string `form:"symbol"`
	BaseCoinCode  string `form:"baseCoinCode"`
	QuoteCoinCode string `form:"quoteCoinCode"`
	Status        string `form:"status"`
}

type CoinPriceDTO struct {
	baseDTO.BaseDTO
	CoinID    uint64  `json:"coinId"`
	CoinCode  string  `json:"coinCode"`
	QuoteCode string  `json:"quoteCode"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change24h"`
	Volume24h float64 `json:"volume24h"`
	High24h   float64 `json:"high24h"`
	Low24h    float64 `json:"low24h"`
	Source    string  `json:"source"`
}

type CreateCoinPriceDTO struct {
	CoinID    uint64  `json:"coinId"`
	CoinCode  string  `json:"coinCode"`
	QuoteCode string  `json:"quoteCode"`
	Price     float64 `json:"price"`
	Change24h float64 `json:"change24h"`
	Volume24h float64 `json:"volume24h"`
	High24h   float64 `json:"high24h"`
	Low24h    float64 `json:"low24h"`
	Source    string  `json:"source"`
}

type CoinPriceQueryDTO struct {
	CoinCode  string `form:"coinCode"`
	QuoteCode string `form:"quoteCode"`
}
