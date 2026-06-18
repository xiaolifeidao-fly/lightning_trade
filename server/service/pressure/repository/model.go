package repository

import (
	"time"

	"common/middleware/db"
)

// PressureAnalysis AI 压力面分析快照表，oracle 定时(默认10分钟)结合 K 线/指标与最新消息面分析后落库，
// 每次分析追加一行(时间序列)。维度为 平台×币种，记录上方做空压力位与下方做多压力位。
type PressureAnalysis struct {
	db.BaseEntity
	PlatformCode string  `gorm:"column:platform_code;type:varchar(32);index:idx_pressure_plat_coin_time,priority:1" orm:"column(platform_code);size(32);null" description:"行情平台 binance"`
	CoinCode     string  `gorm:"column:coin_code;type:varchar(32);index:idx_pressure_plat_coin_time,priority:2;index:idx_pressure_coin" orm:"column(coin_code);size(32);null" description:"基础币种 BTC"`
	Symbol       string  `gorm:"column:symbol;type:varchar(32)" orm:"column(symbol);size(32);null" description:"交易对 BTCUSDT"`
	Interval     string  `gorm:"column:interval;type:varchar(16)" orm:"column(interval);size(16);null" description:"分析所用主周期 15m"`
	RefPrice     float64 `gorm:"column:ref_price;type:decimal(24,8);default:0" orm:"column(ref_price);null" description:"分析时参考价(当前价)"`
	Bias         string  `gorm:"column:bias;type:varchar(16);index:idx_pressure_bias" orm:"column(bias);size(16);null" description:"压力面整体倾向 long/short/neutral"`
	// ShortPressureLevels 上方做空压力位(阻力)，LongPressureLevels 下方做多压力位(支撑)，均为 JSON 数组。
	ShortPressureLevels string    `gorm:"column:short_pressure_levels;type:text" orm:"column(short_pressure_levels);null" description:"做空压力位列表(上方阻力,JSON数组)"`
	LongPressureLevels  string    `gorm:"column:long_pressure_levels;type:text" orm:"column(long_pressure_levels);null" description:"做多压力位列表(下方支撑,JSON数组)"`
	KeyResistance       float64   `gorm:"column:key_resistance;type:decimal(24,8);default:0" orm:"column(key_resistance);null" description:"最关键上方压力位"`
	KeySupport          float64   `gorm:"column:key_support;type:decimal(24,8);default:0" orm:"column(key_support);null" description:"最关键下方支撑位"`
	Summary             string    `gorm:"column:summary;type:text" orm:"column(summary);null" description:"压力面中文综述"`
	NewsSummary         string    `gorm:"column:news_summary;type:text" orm:"column(news_summary);null" description:"分析时注入的消息面摘要"`
	Model               string    `gorm:"column:model;type:varchar(64)" orm:"column(model);size(64);null" description:"使用的模型"`
	Provider            string    `gorm:"column:provider;type:varchar(32)" orm:"column(provider);size(32);null" description:"AI服务商"`
	RawResponse         string    `gorm:"column:raw_response;type:text" orm:"column(raw_response);null" description:"LLM原始返回"`
	AnalyzedTime        time.Time `gorm:"column:analyzed_time;type:datetime;index:idx_pressure_plat_coin_time,priority:3;index:idx_pressure_analyzed" orm:"column(analyzed_time);null" description:"分析时间"`
}

func (p *PressureAnalysis) TableName() string {
	return "pressure_analysis"
}
