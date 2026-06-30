package pressure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"oracle/pkg/collector"
	"oracle/pkg/indicator"
	"oracle/pkg/oraclecfg"

	pressuresvc "service/pressure"
	pressureDTO "service/pressure/dto"

	"github.com/sirupsen/logrus"
)

// systemPrompt 要求模型基于 K 线/指标与最新消息面，给出当前的压力面：
// 上方做空压力位(阻力)与下方做多压力位(支撑)，每个价位带强度与原因。
const systemPrompt = `你是一名严谨的加密货币合约盘口与压力面分析师。基于给定的多周期K线、技术指标、逐笔成交压力、资金费率与最新消息面，刻画「当前时刻」该交易对的压力面结构。

核心交付：上方做空压力位(阻力，价格上行时承压、利于做空的关键位)与下方做多压力位(支撑，价格下行时获撑、利于做多的关键位)，每个价位给出强度与依据。

只能输出 JSON，禁止任何额外文字或 Markdown。字段如下：
{
  "bias": "long|short|neutral",        // 压力面整体倾向：上方更易突破偏 long、下方更易跌破偏 short、均衡为 neutral
  "short_pressure_levels": [           // 上方做空压力位(阻力)，按价格从近到远排序，最多5个；价格须>当前价
    {"price": 数字, "strength": 0~1的小数, "reason": "中文，<=60字，说明该价位为何构成阻力(如布林上轨/前高/密集成交/整数关口/消息面)"}
  ],
  "long_pressure_levels": [            // 下方做多压力位(支撑)，按价格从近到远排序，最多5个；价格须<当前价
    {"price": 数字, "strength": 0~1的小数, "reason": "中文，<=60字，说明该价位为何构成支撑"}
  ],
  "key_resistance": 数字,               // 最关键的上方压力位(若无填0)
  "key_support": 数字,                  // 最关键的下方支撑位(若无填0)
  "summary": "中文综述压力面与多空争夺逻辑，<=200字，可结合消息面"
}

要求：
- 价位须落在合理区间：以近端高低点、布林带上下轨、整数关口、密集成交区、前高前低为主要依据，禁止凭空编造远离当前价的价位。
- short_pressure_levels 的 price 必须严格大于当前价并按从近到远升序；long_pressure_levels 的 price 必须严格小于当前价并按从近到远降序。
- strength 标定该价位的有效性/重要性：多重依据共振→0.7~1.0；单一依据→0.4~0.7；较弱→<0.4。
- key_resistance/key_support 从对应列表中挑最关键的一个价位；无清晰价位时填 0。
- 消息面仅作辅助：与技术面冲突时以技术面结构为主，可据消息面微调强度或在 summary 中点明风险，勿单凭消息面臆造价位。
- 凡标注 N/A 的指标表示数据不足，不得当作真实数值参与判断。`

// Result AI 解析后的压力面结构。
type Result struct {
	Bias                string                      `json:"bias"`
	ShortPressureLevels []pressureDTO.PressureLevel `json:"short_pressure_levels"`
	LongPressureLevels  []pressureDTO.PressureLevel `json:"long_pressure_levels"`
	KeyResistance       float64                     `json:"key_resistance"`
	KeySupport          float64                     `json:"key_support"`
	Summary             string                      `json:"summary"`
}

// cachedPressure 一次成功分析的缓存条目，供预测侧注入。
type cachedPressure struct {
	result     Result
	interval   string
	refPrice   float64
	analyzedAt time.Time
}

// Analyzer 调用 LLM 完成压力面分析并落库，同时按币种缓存最新结果供预测注入。
type Analyzer struct {
	cfg     oraclecfg.AIConfig
	pcfg    oraclecfg.PressureConfig
	client  *http.Client
	service *pressuresvc.PressureService

	mu    sync.RWMutex
	store map[string]cachedPressure
}

// New 创建压力面分析器。service 可为 nil（仅分析不落库）。
func New(cfg oraclecfg.AIConfig, pcfg oraclecfg.PressureConfig, service *pressuresvc.PressureService) *Analyzer {
	return &Analyzer{
		cfg:     cfg,
		pcfg:    pcfg,
		client:  &http.Client{Timeout: cfg.Timeout},
		service: service,
		store:   map[string]cachedPressure{},
	}
}

// Analyze 用快照 + 特征(+可选消息面)调用 LLM，得到压力面并落库。
func (a *Analyzer) Analyze(ctx context.Context, snap *collector.Snapshot, f indicator.Features, newsSummary string) (*Result, error) {
	if strings.TrimSpace(a.cfg.APIURL) == "" || strings.TrimSpace(a.cfg.APIKey) == "" {
		return nil, fmt.Errorf("AI 配置缺失(api_url/api_key)")
	}

	newsBlock := ""
	if strings.TrimSpace(newsSummary) != "" {
		newsBlock = "\n" + newsSummary +
			"消息面仅作辅助：与技术面冲突时以技术面结构为主，可据消息面微调价位强度或风险，勿单凭消息面臆造价位。\n"
	}

	userPrompt := fmt.Sprintf(
		"交易对=%s 平台=%s\n当前价=%.4f 当前时间=%s\n\n%s%s\n请按 system 要求只输出 JSON 压力面。",
		snap.Symbol, snap.Platform, f.LastClose, time.Now().Format("2006-01-02 15:04:05"),
		indicator.Summary(snap, f), newsBlock)

	var (
		result  Result
		raw     string
		lastErr error
	)
	for attempt := 0; attempt < 2; attempt++ {
		raw, lastErr = a.requestChatJSON(ctx, systemPrompt, userPrompt)
		if lastErr != nil {
			continue
		}
		result = Result{}
		if e := json.Unmarshal([]byte(raw), &result); e != nil {
			lastErr = fmt.Errorf("解析压力面返回失败: %w", e)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, lastErr
	}

	finalize(&result, f.LastClose)

	// 缓存最新结果供预测侧注入（落库失败也不影响内存缓存与注入）。
	analyzedAt := time.Now()
	a.mu.Lock()
	a.store[strings.ToUpper(snap.CoinCode)] = cachedPressure{
		result:     result,
		interval:   snap.Interval,
		refPrice:   f.LastClose,
		analyzedAt: analyzedAt,
	}
	a.mu.Unlock()

	if a.service != nil {
		if err := a.service.SavePressure(pressureDTO.PressureAnalysisSaveDTO{
			PlatformCode:        snap.Platform,
			CoinCode:            snap.CoinCode,
			Symbol:              snap.Symbol,
			Interval:            snap.Interval,
			RefPrice:            f.LastClose,
			Bias:                result.Bias,
			ShortPressureLevels: result.ShortPressureLevels,
			LongPressureLevels:  result.LongPressureLevels,
			KeyResistance:       result.KeyResistance,
			KeySupport:          result.KeySupport,
			Summary:             result.Summary,
			NewsSummary:         newsSummary,
			Model:               a.cfg.Model,
			Provider:            a.cfg.Provider,
			RawResponse:         raw,
			AnalyzedTime:        analyzedAt,
		}); err != nil {
			return &result, fmt.Errorf("压力面落库失败: %w", err)
		}
	}
	return &result, nil
}

// Summary 返回可拼进预测 prompt 的压力面摘要（含压力面的计算时间）；无缓存或已过期返回 ""。
func (a *Analyzer) Summary(coin string) string {
	coin = strings.ToUpper(strings.TrimSpace(coin))
	a.mu.RLock()
	item, ok := a.store[coin]
	a.mu.RUnlock()
	if !ok {
		return ""
	}

	age := time.Since(item.analyzedAt)
	// 过期保护：超过分析间隔的 3 倍仍未更新（分析持续失败），视为不可信，不注入。
	if a.pcfg.AnalyzeInterval > 0 && age > 3*a.pcfg.AnalyzeInterval {
		return ""
	}

	r := item.result
	var b strings.Builder
	fmt.Fprintf(&b, "压力面(辅助参考，基于%s周期，于 %s 计算，约%.0f分钟前):\n",
		item.interval, item.analyzedAt.Format("2006-01-02 15:04:05"), age.Minutes())
	fmt.Fprintf(&b, "  整体倾向=%s 计算时参考价=%.4f\n", r.Bias, item.refPrice)
	if r.KeyResistance > 0 {
		fmt.Fprintf(&b, "  关键阻力(做空压力位)=%.4f\n", r.KeyResistance)
	}
	if r.KeySupport > 0 {
		fmt.Fprintf(&b, "  关键支撑(做多压力位)=%.4f\n", r.KeySupport)
	}
	if s := formatLevels(r.ShortPressureLevels); s != "" {
		fmt.Fprintf(&b, "  上方阻力(做空压力位): %s\n", s)
	}
	if s := formatLevels(r.LongPressureLevels); s != "" {
		fmt.Fprintf(&b, "  下方支撑(做多压力位): %s\n", s)
	}
	if strings.TrimSpace(r.Summary) != "" {
		fmt.Fprintf(&b, "  综述: %s\n", r.Summary)
	}
	return b.String()
}

// formatLevels 取最多前 4 个价位（已按从近到远排序），格式化为「价格(强度)」串，控制注入 token。
func formatLevels(levels []pressureDTO.PressureLevel) string {
	const maxLevels = 4
	parts := make([]string, 0, maxLevels)
	for i, lv := range levels {
		if i >= maxLevels {
			break
		}
		parts = append(parts, fmt.Sprintf("%.4f(强度%.2f)", lv.Price, lv.Strength))
	}
	return strings.Join(parts, "、")
}

// finalize 归一化倾向并对价位做方向/排序校验：
// 做空压力位须在当前价上方(从近到远升序)，做多压力位须在当前价下方(从近到远降序)，过滤越界价位。
func finalize(r *Result, ref float64) {
	r.Bias = normalizeBias(r.Bias)

	if ref > 0 {
		r.ShortPressureLevels = filterLevels(r.ShortPressureLevels, ref, true)
		r.LongPressureLevels = filterLevels(r.LongPressureLevels, ref, false)
		// 上方阻力按价从近到远升序；下方支撑按价从近到远降序。
		sort.SliceStable(r.ShortPressureLevels, func(i, j int) bool {
			return r.ShortPressureLevels[i].Price < r.ShortPressureLevels[j].Price
		})
		sort.SliceStable(r.LongPressureLevels, func(i, j int) bool {
			return r.LongPressureLevels[i].Price > r.LongPressureLevels[j].Price
		})
		if r.KeyResistance > 0 && r.KeyResistance <= ref {
			r.KeyResistance = 0
		}
		if r.KeySupport > 0 && r.KeySupport >= ref {
			r.KeySupport = 0
		}
	}
	if r.ShortPressureLevels == nil {
		r.ShortPressureLevels = []pressureDTO.PressureLevel{}
	}
	if r.LongPressureLevels == nil {
		r.LongPressureLevels = []pressureDTO.PressureLevel{}
	}
}

// filterLevels 过滤价位：above=true 仅保留价>ref(阻力)，否则仅保留 0<价<ref(支撑)，并钳制 strength 到 [0,1]。
func filterLevels(levels []pressureDTO.PressureLevel, ref float64, above bool) []pressureDTO.PressureLevel {
	out := make([]pressureDTO.PressureLevel, 0, len(levels))
	for _, lv := range levels {
		if lv.Price <= 0 {
			continue
		}
		if above && lv.Price <= ref {
			continue
		}
		if !above && lv.Price >= ref {
			continue
		}
		lv.Strength = clamp01(lv.Strength)
		out = append(out, lv)
	}
	return out
}

func normalizeBias(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "long", "bull", "bullish", "up":
		return "long"
	case "short", "bear", "bearish", "down":
		return "short"
	default:
		return "neutral"
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

type chatRequest struct {
	Model               string            `json:"model"`
	Messages            []chatMessage     `json:"messages"`
	Temperature         float64           `json:"temperature"`
	MaxCompletionTokens int               `json:"max_completion_tokens"`
	ResponseFormat      map[string]string `json:"response_format"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (a *Analyzer) requestChatJSON(ctx context.Context, systemMsg, userMsg string) (string, error) {
	reqBody := chatRequest{
		Model: a.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		Temperature:         a.cfg.Temperature,
		MaxCompletionTokens: a.cfg.MaxTokens,
		ResponseFormat:      map[string]string{"type": "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(a.cfg.APIURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	httpReq.Header.Set("x-codex-agent", "1")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("压力面请求失败 status=%d body=%s", resp.StatusCode, truncate(string(body), 512))
	}
	logrus.Debugf("[oracle][pressure] AI 原始返回: %s", truncate(string(body), 512))
	return parseChatContent(body)
}

func parseChatContent(body []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("压力面返回缺少 choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
