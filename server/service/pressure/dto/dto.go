package dto

import (
	baseDTO "common/base/dto"
	"time"
)

// PressureLevel 单个压力/支撑位：价格 + 强度(0~1) + 原因。
type PressureLevel struct {
	Price    float64 `json:"price"`
	Strength float64 `json:"strength"`
	Reason   string  `json:"reason"`
}

// PressureAnalysisSaveDTO oracle 落库压力面分析的入参。
type PressureAnalysisSaveDTO struct {
	PlatformCode string
	CoinCode     string
	Symbol       string
	Interval     string
	RefPrice     float64
	Bias         string
	// ShortPressureLevels 做空压力位(上方阻力)，LongPressureLevels 做多压力位(下方支撑)。
	ShortPressureLevels []PressureLevel
	LongPressureLevels  []PressureLevel
	KeyResistance       float64
	KeySupport          float64
	Summary             string
	NewsSummary         string
	Model               string
	Provider            string
	RawResponse         string
	AnalyzedTime        time.Time
}

// PressureAnalysisDTO 对外输出的压力面分析记录。
type PressureAnalysisDTO struct {
	baseDTO.BaseDTO
	PlatformCode        string          `json:"platformCode"`
	CoinCode            string          `json:"coinCode"`
	Symbol              string          `json:"symbol"`
	Interval            string          `json:"interval"`
	RefPrice            float64         `json:"refPrice"`
	Bias                string          `json:"bias"`
	ShortPressureLevels []PressureLevel `json:"shortPressureLevels"`
	LongPressureLevels  []PressureLevel `json:"longPressureLevels"`
	KeyResistance       float64         `json:"keyResistance"`
	KeySupport          float64         `json:"keySupport"`
	Summary             string          `json:"summary"`
	NewsSummary         string          `json:"newsSummary"`
	Model               string          `json:"model"`
	Provider            string          `json:"provider"`
	AnalyzedTime        time.Time       `json:"analyzedTime"`
}

// PressureAnalysisQueryDTO 查询压力面分析历史的入参。
type PressureAnalysisQueryDTO struct {
	PlatformCode string `json:"platformCode" form:"platformCode"`
	CoinCode     string `json:"coinCode" form:"coinCode"`
	StartTime    int64  `json:"startTime" form:"startTime"`
	EndTime      int64  `json:"endTime" form:"endTime"`
	Page         int    `json:"page" form:"page"`
	PageIndex    int    `json:"pageIndex" form:"pageIndex"`
	PageSize     int    `json:"pageSize" form:"pageSize"`
}
