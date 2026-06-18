package oraclecfg

import (
	"strings"
	"time"

	"common/middleware/vipper"
)

// AIConfig AI 服务商配置（OpenAI 兼容）。
type AIConfig struct {
	Provider    string
	APIURL      string
	APIKey      string
	Model       string
	Timeout     time.Duration
	MaxTokens   int
	Temperature float64
}

// NewsConfig 消息面采集配置（复用 AIConfig 的 endpoint/key）。
type NewsConfig struct {
	Enabled bool
	// RefreshInterval 消息面刷新间隔（独立于预测节奏，慢速即可）。
	RefreshInterval time.Duration
	// Model 留空时复用 AIConfig.Model。
	Model       string
	Timeout     time.Duration
	MaxTokens   int
	Temperature float64
}

// PressureConfig 压力面分析配置（复用 AIConfig 的 endpoint/key）。
type PressureConfig struct {
	Enabled bool
	// Interval 压力面分析所用的主周期（拉取该周期 K 线 + 指标）。
	Interval string
	// AnalyzeInterval 分析节奏（默认 10 分钟）。
	AnalyzeInterval time.Duration
	// Model 留空时复用 AIConfig.Model。
	Model       string
	Timeout     time.Duration
	MaxTokens   int
	Temperature float64
}

// Config oracle 运行配置。
type Config struct {
	Platform       string
	Coins          []string
	Intervals      []string
	KlineLimit     int
	TradeLimit     int
	FundingLimit   int
	HighKlineLimit int
	// HighTimeframes 主周期 -> 高周期列表
	HighTimeframes map[string][]string
	// ScanInterval 主周期 -> 跑批间隔
	ScanInterval map[string]time.Duration
	DefaultScan  time.Duration
	// SettleInterval 到期预测回填真实价的轮询间隔；SettleBatch 单轮最多结算条数。
	SettleInterval time.Duration
	SettleBatch    int
	AI             AIConfig
	News           NewsConfig
	Pressure       PressureConfig
}

// Load 从 vipper（已 Init）读取配置。
func Load() Config {
	cfg := Config{
		Platform:       firstNonEmpty(vipper.GetString("oracle.platform"), "binance"),
		Coins:          splitCSV(vipper.GetString("oracle.coins"), []string{"BTC"}),
		Intervals:      splitCSV(vipper.GetString("oracle.intervals"), []string{"15m"}),
		KlineLimit:     intOr(vipper.GetInt("oracle.kline_limit"), 200),
		TradeLimit:     intOr(vipper.GetInt("oracle.trade_limit"), 500),
		FundingLimit:   intOr(vipper.GetInt("oracle.funding_limit"), 8),
		HighKlineLimit: intOr(vipper.GetInt("oracle.high_kline_limit"), 60),
		HighTimeframes: map[string][]string{},
		ScanInterval:   map[string]time.Duration{},
		DefaultScan:    time.Duration(intOr(vipper.GetInt("oracle.default_scan_seconds"), 900)) * time.Second,
		SettleInterval: time.Duration(intOr(vipper.GetInt("oracle.settle_seconds"), 60)) * time.Second,
		SettleBatch:    intOr(vipper.GetInt("oracle.settle_batch"), 500),
	}

	for _, itv := range cfg.Intervals {
		cfg.HighTimeframes[itv] = splitCSV(vipper.GetString("oracle.high_timeframes."+itv), nil)
		if sec := vipper.GetInt("oracle.scan_seconds." + itv); sec > 0 {
			cfg.ScanInterval[itv] = time.Duration(sec) * time.Second
		} else {
			cfg.ScanInterval[itv] = cfg.DefaultScan
		}
	}

	cfg.AI = AIConfig{
		Provider:    firstNonEmpty(vipper.GetString("oracle.ai.provider"), "tu2do"),
		APIURL:      firstNonEmpty(vipper.GetString("oracle.ai.api_url"), vipper.GetString("position.ai_open.api_url"), vipper.GetString("position.ai_close.api_url")),
		APIKey:      firstNonEmpty(vipper.GetString("oracle.ai.api_key"), vipper.GetString("position.ai_open.api_key"), vipper.GetString("position.ai_close.api_key")),
		Model:       firstNonEmpty(vipper.GetString("oracle.ai.model"), vipper.GetString("position.ai_open.model"), vipper.GetString("position.ai_close.model"), "gpt-4o-mini"),
		Timeout:     time.Duration(intOr(vipper.GetInt("oracle.ai.timeout_seconds"), 120)) * time.Second,
		MaxTokens:   intOr(vipper.GetInt("oracle.ai.max_tokens"), 2000),
		Temperature: floatOr(vipper.GetFloat64("oracle.ai.temperature"), 0.3),
	}

	cfg.News = NewsConfig{
		Enabled:         vipper.GetBool("oracle.news.enabled"),
		RefreshInterval: time.Duration(intOr(vipper.GetInt("oracle.news.refresh_seconds"), 1800)) * time.Second,
		Model:           vipper.GetString("oracle.news.model"),
		Timeout:         time.Duration(intOr(vipper.GetInt("oracle.news.timeout_seconds"), 0)) * time.Second,
		MaxTokens:       intOr(vipper.GetInt("oracle.news.max_tokens"), 1500),
		Temperature:     floatOr(vipper.GetFloat64("oracle.news.temperature"), 0.4),
	}

	cfg.Pressure = PressureConfig{
		Enabled:         vipper.GetBool("oracle.pressure.enabled"),
		Interval:        firstNonEmpty(vipper.GetString("oracle.pressure.interval"), "15m"),
		AnalyzeInterval: time.Duration(intOr(vipper.GetInt("oracle.pressure.analyze_seconds"), 600)) * time.Second,
		Model:           vipper.GetString("oracle.pressure.model"),
		Timeout:         time.Duration(intOr(vipper.GetInt("oracle.pressure.timeout_seconds"), 0)) * time.Second,
		MaxTokens:       intOr(vipper.GetInt("oracle.pressure.max_tokens"), 2500),
		Temperature:     floatOr(vipper.GetFloat64("oracle.pressure.temperature"), 0.3),
	}

	return cfg
}

func splitCSV(value string, fallback []string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func intOr(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func floatOr(v, fallback float64) float64 {
	if v <= 0 {
		return fallback
	}
	return v
}
