package repository

import (
	"time"

	"common/middleware/db"
)

// NewsSentiment 消息面快照表，oracle 定时联网拉取后落库，每次刷新追加一行(时间序列)。
// 维度为币种(CoinCode)，消息面多为币种/宏观级别，不区分交易平台。
type NewsSentiment struct {
	db.BaseEntity
	CoinCode    string    `gorm:"column:coin_code;type:varchar(32);index:idx_news_coin_fetched,priority:1;index:idx_coin_code" orm:"column(coin_code);size(32);null" description:"基础币种 BTC"`
	Sentiment   string    `gorm:"column:sentiment;type:varchar(16);index:idx_sentiment" orm:"column(sentiment);size(16);null" description:"消息面方向 bullish/bearish/neutral"`
	Score       float64   `gorm:"column:score;type:decimal(10,4);default:0" orm:"column(score);null" description:"情绪强度评分 -1~1"`
	KeyEvents   string    `gorm:"column:key_events;type:text" orm:"column(key_events);null" description:"关键事件列表(JSON数组)"`
	RiskFlags   string    `gorm:"column:risk_flags;type:text" orm:"column(risk_flags);null" description:"风险点列表(JSON数组)"`
	AsOf        string    `gorm:"column:as_of;type:varchar(64)" orm:"column(as_of);size(64);null" description:"模型引用的最新消息对应时间"`
	Freshness   string    `gorm:"column:freshness;type:varchar(255)" orm:"column(freshness);size(255);null" description:"数据新鲜度自评"`
	Summary     string    `gorm:"column:summary;type:text" orm:"column(summary);null" description:"消息面中文综述"`
	Model       string    `gorm:"column:model;type:varchar(64)" orm:"column(model);size(64);null" description:"使用的模型"`
	Provider    string    `gorm:"column:provider;type:varchar(32)" orm:"column(provider);size(32);null" description:"AI服务商"`
	RawResponse string    `gorm:"column:raw_response;type:text" orm:"column(raw_response);null" description:"LLM原始返回"`
	FetchedTime time.Time `gorm:"column:fetched_time;type:datetime;index:idx_news_coin_fetched,priority:2;index:idx_fetched_time" orm:"column(fetched_time);null" description:"联网拉取时间"`
}

func (n *NewsSentiment) TableName() string {
	return "news_sentiment"
}
