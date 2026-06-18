package news

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"oracle/pkg/oraclecfg"

	newssvc "service/news"
	newsDTO "service/news/dto"

	"github.com/sirupsen/logrus"
)

// systemPrompt 强制模型先联网搜索再作答。措辞反复强调「联网/最新/实时」，
// 以最大化触发上游代理的 web search 工具，避免凭训练期记忆编造旧闻。
const systemPrompt = `你是加密货币市场的消息面分析师。

【强制要求】你必须先调用联网搜索(web search)工具，实时获取该币种最近的最新新闻、链上事件、交易所/监管动态、宏观数据(如美联储、CPI、利率)与社交媒体情绪，再综合判断。务必使用联网拿到的最新数据，严禁凭训练期记忆或猜测作答；若无法联网，须如实在 source_freshness 中说明。

只能输出 JSON，禁止任何额外文字或 Markdown。字段如下：
{
  "sentiment": "bullish|bearish|neutral",   // 消息面整体方向
  "score": -1.0~1.0 的小数,                   // 偏多为正、偏空为负、0=中性；强度越大绝对值越大
  "key_events": ["最近的关键事件，每条<=40字，按重要性排序，最多5条"],
  "risk_flags": ["显著风险点，如监管/黑客/大额解锁/宏观利空，最多5条，无则空数组"],
  "as_of": "你引用的最新消息对应的日期或时间(尽量精确到日)",
  "source_freshness": "简述数据新鲜度，如『已联网获取，最新事件为X月X日』或『未能联网，以下为既有认知』",
  "summary": "中文综述消息面，<=150字，突出对短期价格的潜在影响"
}

要求：
- 优先采用最近24~72小时内的信息；越新权重越高。
- 区分事实与传闻，传闻需在 summary 中标注。
- score 与 sentiment 必须方向一致：bullish 为正、bearish 为负、neutral 为 0。`

// Sentiment 结构化的消息面结果。
type Sentiment struct {
	Sentiment string   `json:"sentiment"`
	Score     float64  `json:"score"`
	KeyEvents []string `json:"key_events"`
	RiskFlags []string `json:"risk_flags"`
	AsOf      string   `json:"as_of"`
	Freshness string   `json:"source_freshness"`
	Summary   string   `json:"summary"`
}

// cached 一次成功采集的缓存条目。
type cached struct {
	s         Sentiment
	fetchedAt time.Time
	raw       string
}

// Collector 按币种缓存消息面，独立于预测节奏慢速刷新；同时落库形成历史时间序列。
type Collector struct {
	ai      oraclecfg.AIConfig
	cfg     oraclecfg.NewsConfig
	client  *http.Client
	service *newssvc.NewsService

	mu    sync.RWMutex
	store map[string]cached
}

// New 创建消息面采集器。service 可为 nil（仅内存缓存、不落库）。
func New(ai oraclecfg.AIConfig, cfg oraclecfg.NewsConfig, service *newssvc.NewsService) *Collector {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = ai.Timeout
	}
	return &Collector{
		ai:      ai,
		cfg:     cfg,
		client:  &http.Client{Timeout: timeout},
		service: service,
		store:   map[string]cached{},
	}
}

// Refresh 联网拉取并解析指定币种的消息面，成功后写入缓存。
func (c *Collector) Refresh(ctx context.Context, coin string) error {
	coin = strings.ToUpper(strings.TrimSpace(coin))
	if strings.TrimSpace(c.ai.APIURL) == "" || strings.TrimSpace(c.ai.APIKey) == "" {
		return fmt.Errorf("AI 配置缺失(api_url/api_key)")
	}

	userPrompt := fmt.Sprintf(
		"请立即联网搜索 %s(加密货币)截至当前时间(%s)最近的最新消息面，获取实时新闻与事件后，按 system 要求只输出 JSON。务必使用联网得到的最新数据。",
		coin, time.Now().Format("2006-01-02 15:04:05"))

	raw, err := c.requestChatJSON(ctx, systemPrompt, userPrompt)
	if err != nil {
		return err
	}
	var s Sentiment
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return fmt.Errorf("解析消息面返回失败: %w", err)
	}
	normalize(&s)

	fetchedAt := time.Now()
	c.mu.Lock()
	c.store[coin] = cached{s: s, fetchedAt: fetchedAt, raw: raw}
	c.mu.Unlock()

	// 落库形成历史时间序列；失败不影响内存缓存与后续注入。
	if c.service != nil {
		if err := c.service.SaveSentiment(newsDTO.NewsSentimentSaveDTO{
			CoinCode:    coin,
			Sentiment:   s.Sentiment,
			Score:       s.Score,
			KeyEvents:   s.KeyEvents,
			RiskFlags:   s.RiskFlags,
			AsOf:        s.AsOf,
			Freshness:   s.Freshness,
			Summary:     s.Summary,
			Model:       c.modelName(),
			Provider:    c.ai.Provider,
			RawResponse: raw,
			FetchedTime: fetchedAt,
		}); err != nil {
			logrus.Warnf("[oracle][news] %s 落库失败: %v", coin, err)
		}
	}

	logrus.Infof("[oracle][news] %s 消息面刷新: sentiment=%s score=%.2f as_of=%s",
		coin, s.Sentiment, s.Score, s.AsOf)
	return nil
}

// modelName 返回实际使用的消息面模型名（专用模型优先，否则复用 AI 主模型）。
func (c *Collector) modelName() string {
	if strings.TrimSpace(c.cfg.Model) != "" {
		return c.cfg.Model
	}
	return c.ai.Model
}

// Summary 返回可拼进预测 prompt 的消息面摘要；无缓存或已过期返回 ""。
func (c *Collector) Summary(coin string) string {
	coin = strings.ToUpper(strings.TrimSpace(coin))
	c.mu.RLock()
	item, ok := c.store[coin]
	c.mu.RUnlock()
	if !ok {
		return ""
	}

	age := time.Since(item.fetchedAt)
	// 过期保护：超过刷新间隔的 3 倍仍未更新（刷新持续失败），视为不可信，不注入。
	if c.cfg.RefreshInterval > 0 && age > 3*c.cfg.RefreshInterval {
		return ""
	}

	s := item.s
	var b strings.Builder
	fmt.Fprintf(&b, "消息面(辅助参考，%.0f分钟前联网获取，对应时间≈%s):\n", age.Minutes(), s.AsOf)
	fmt.Fprintf(&b, "  方向=%s 强度评分=%.2f(-1偏空~1偏多)\n", s.Sentiment, s.Score)
	if len(s.KeyEvents) > 0 {
		fmt.Fprintf(&b, "  关键事件: %s\n", strings.Join(s.KeyEvents, "；"))
	}
	if len(s.RiskFlags) > 0 {
		fmt.Fprintf(&b, "  风险点: %s\n", strings.Join(s.RiskFlags, "；"))
	}
	if strings.TrimSpace(s.Summary) != "" {
		fmt.Fprintf(&b, "  综述: %s\n", s.Summary)
	}
	if strings.TrimSpace(s.Freshness) != "" {
		fmt.Fprintf(&b, "  数据新鲜度: %s\n", s.Freshness)
	}
	return b.String()
}

func normalize(s *Sentiment) {
	switch strings.ToLower(strings.TrimSpace(s.Sentiment)) {
	case "bullish", "bull", "long", "up", "positive":
		s.Sentiment = "bullish"
	case "bearish", "bear", "short", "down", "negative":
		s.Sentiment = "bearish"
	default:
		s.Sentiment = "neutral"
	}
	if s.Score > 1 {
		s.Score = 1
	} else if s.Score < -1 {
		s.Score = -1
	}
	// 方向与评分一致性校验，自相矛盾时以中性兜底。
	if (s.Sentiment == "bullish" && s.Score < 0) || (s.Sentiment == "bearish" && s.Score > 0) {
		s.Sentiment = "neutral"
		s.Score = 0
	}
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

func (c *Collector) requestChatJSON(ctx context.Context, systemMsg, userMsg string) (string, error) {
	maxTokens := c.cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = c.ai.MaxTokens
	}

	reqBody := chatRequest{
		Model: c.modelName(),
		Messages: []chatMessage{
			{Role: "system", Content: systemMsg},
			{Role: "user", Content: userMsg},
		},
		Temperature:         c.cfg.Temperature,
		MaxCompletionTokens: maxTokens,
		ResponseFormat:      map[string]string{"type": "json_object"},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(c.ai.APIURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.ai.APIKey)
	httpReq.Header.Set("x-codex-agent", "1")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("消息面请求失败 status=%d body=%s", resp.StatusCode, truncate(string(body), 512))
	}
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
		return "", fmt.Errorf("消息面返回缺少 choices")
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
