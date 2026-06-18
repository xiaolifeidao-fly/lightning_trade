package dto

import (
	baseDTO "common/base/dto"
	"time"
)

// NewsSentimentSaveDTO oracle 落库消息面的入参。
type NewsSentimentSaveDTO struct {
	CoinCode    string
	Sentiment   string
	Score       float64
	KeyEvents   []string
	RiskFlags   []string
	AsOf        string
	Freshness   string
	Summary     string
	Model       string
	Provider    string
	RawResponse string
	FetchedTime time.Time
}

// NewsSentimentDTO 对外输出的消息面记录。
type NewsSentimentDTO struct {
	baseDTO.BaseDTO
	CoinCode    string    `json:"coinCode"`
	Sentiment   string    `json:"sentiment"`
	Score       float64   `json:"score"`
	KeyEvents   []string  `json:"keyEvents"`
	RiskFlags   []string  `json:"riskFlags"`
	AsOf        string    `json:"asOf"`
	Freshness   string    `json:"freshness"`
	Summary     string    `json:"summary"`
	Model       string    `json:"model"`
	Provider    string    `json:"provider"`
	FetchedTime time.Time `json:"fetchedTime"`
}

// NewsSentimentQueryDTO 查询消息面历史的入参。
type NewsSentimentQueryDTO struct {
	CoinCode  string `json:"coinCode" form:"coinCode"`
	StartTime int64  `json:"startTime" form:"startTime"`
	EndTime   int64  `json:"endTime" form:"endTime"`
	Page      int    `json:"page" form:"page"`
	PageIndex int    `json:"pageIndex" form:"pageIndex"`
	PageSize  int    `json:"pageSize" form:"pageSize"`
}
