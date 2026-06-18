package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"common/middleware/vipper"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

const (
	defaultAICloseProvider         = "tu2do"
	defaultAICloseTimeout          = 20 * time.Second
	defaultAICloseLastDecisionFile = "data/ai_close_last_decision.json"
)

// PositionSnapshot 提供给 AI 决策器的持仓快照。
type PositionSnapshot struct {
	AccountName        string
	AccountUID         string
	HasPosition        bool
	CurrentPosition    CurrentPositionDetails
	BTCMarket          *BTCAnalysisSnapshot
	PositionSummary    string
	InstID             string
	PositionID         string
	PositionSide       string
	PositionSize       string
	AvgPrice           string
	LastPrice          string
	LiqPrice           string
	UseMargin          string
	UnrealizedProfit   string
	PnLPercent         decimal.Decimal
	TriggerType        string
	AvailBal           string // 账户可用余额（USDT）
	TotalBal           string // 账户总余额（USDT）
	LiqDistancePercent string // 强平距离百分比（正值，距强平越近越小）
	PreviousDecision   *AICloseLastDecisionRecord
}

// CurrentPositionDetails 是给 AI 使用的结构化当前仓位信息。
type CurrentPositionDetails struct {
	InstType         string
	InstID           string
	PositionID       string
	PositionSide     string
	PositionSize     string
	AvgPrice         string
	LastPrice        string
	LiqPrice         string
	UseMargin        string
	UnrealizedProfit string
	PnLPercent       string
	Leverage         string
	MarginMode       string
	MarginPosition   string
	Currency         string
	CreateTime       string
	UpdateTime       string
	PositionAge      string
	PositionAgeHours decimal.Decimal
}

// AICloseDecision 是 AI 对当前持仓的平仓决策。
type AICloseDecision struct {
	ShouldClose        bool
	FinalAction        string
	Reason             string
	Provider           string
	Model              string
	Confidence         decimal.Decimal
	RiskLevel          string
	LongWinRate        decimal.Decimal
	ShortWinRate       decimal.Decimal
	ContinueSide       string
	LongSuggestedHold  string
	ShortSuggestedHold string
	LongSuggestedSize  string
	ShortSuggestedSize string
	StopLossPrice      string // 当前仓位建议止损/失效价
	TakeProfitPrice    string // 当前仓位建议止盈目标价
	LongEntryPrice     string // 做多入场参考价位（无论当前是否有仓）
	LongStopLoss       string // 做多止损价
	LongTakeProfit     string // 做多止盈价
	ShortEntryPrice    string // 做空入场参考价位
	ShortStopLoss      string // 做空止损价
	ShortTakeProfit    string // 做空止盈价
	NextCheckIn        string
	RawResponse        string
}

// AICloseLastDecisionRecord 是落盘给下一轮 AI 决策参考的最近一次建议。
type AICloseLastDecisionRecord struct {
	SavedAt            string                 `json:"saved_at"`
	AccountName        string                 `json:"account_name,omitempty"`
	AccountUID         string                 `json:"account_uid,omitempty"`
	HasPosition        bool                   `json:"has_position"`
	InstID             string                 `json:"inst_id,omitempty"`
	PositionID         string                 `json:"position_id,omitempty"`
	PositionSide       string                 `json:"position_side,omitempty"`
	PnLPercent         string                 `json:"pnl_percent,omitempty"`
	LiqDistancePercent string                 `json:"liq_distance_percent,omitempty"`
	TriggerType        string                 `json:"trigger_type,omitempty"`
	Decision           AICloseStoredDecision  `json:"decision"`
	RawResponse        map[string]interface{} `json:"raw_response,omitempty"`
}

type AICloseStoredDecision struct {
	ShouldClose        bool   `json:"should_close"`
	FinalAction        string `json:"final_action,omitempty"`
	ContinueSide       string `json:"continue_side,omitempty"`
	LongWinRate        string `json:"long_win_rate,omitempty"`
	ShortWinRate       string `json:"short_win_rate,omitempty"`
	LongSuggestedHold  string `json:"long_suggested_hold,omitempty"`
	ShortSuggestedHold string `json:"short_suggested_hold,omitempty"`
	LongSuggestedSize  string `json:"long_suggested_size,omitempty"`
	ShortSuggestedSize string `json:"short_suggested_size,omitempty"`
	StopLossPrice      string `json:"stop_loss_price,omitempty"`
	TakeProfitPrice    string `json:"take_profit_price,omitempty"`
	LongEntryPrice     string `json:"long_entry_price,omitempty"`
	LongStopLoss       string `json:"long_stop_loss,omitempty"`
	LongTakeProfit     string `json:"long_take_profit,omitempty"`
	ShortEntryPrice    string `json:"short_entry_price,omitempty"`
	ShortStopLoss      string `json:"short_stop_loss,omitempty"`
	ShortTakeProfit    string `json:"short_take_profit,omitempty"`
	NextCheckIn        string `json:"next_check_in,omitempty"`
	Confidence         string `json:"confidence,omitempty"`
	RiskLevel          string `json:"risk_level,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Provider           string `json:"provider,omitempty"`
	Model              string `json:"model,omitempty"`
}

type aiCloseAgentSpec struct {
	Name        string
	DisplayName string
	System      string
	Focus       string
	Response    string
}

type aiCloseAgentResult struct {
	Agent        string          `json:"agent"`
	Decision     string          `json:"decision,omitempty"`
	Veto         bool            `json:"veto,omitempty"`
	Score        json.RawMessage `json:"score,omitempty"`
	Confidence   json.RawMessage `json:"confidence,omitempty"`
	RiskLevel    string          `json:"risk_level,omitempty"`
	ContinueSide string          `json:"continue_side,omitempty"`
	LongWinRate  json.RawMessage `json:"long_win_rate,omitempty"`
	ShortWinRate json.RawMessage `json:"short_win_rate,omitempty"`
	Reason       string          `json:"reason,omitempty"`
	RawResponse  string          `json:"raw_response,omitempty"`
}

// AICloseDecider 定义 AI 平仓决策接口。
type AICloseDecider interface {
	Decide(snapshot PositionSnapshot) (*AICloseDecision, error)
}

// Tu2doCloseDecider 通过 OpenAI-compatible Chat Completions API 请求 AI 平仓建议。
type Tu2doCloseDecider struct {
	client      *http.Client
	apiURL      string
	apiKey      string
	model       string
	temperature float64
	maxTokens   int
	promptTpl   string
	storePath   string
}

func NewAICloseDeciderFromConfig() AICloseDecider {
	enabled := vipper.GetBool("position.ai_close.enabled")
	provider := vipper.GetString("position.ai_close.provider")
	if provider == "" {
		provider = defaultAICloseProvider
	}

	// 默认开启 AI 平仓，以便新逻辑直接生效。
	if !enabled && vipper.GetString("position.ai_close.provider") == "" {
		enabled = true
	}

	if !enabled {
		return nil
	}

	switch provider {
	case "tu2do", "openai", "openai_compatible":
		decider, err := newTu2doCloseDeciderFromConfig(provider)
		if err != nil {
			logrus.Warnf("AI平仓决策: 配置不完整，已禁用, err=%v", err)
			return nil
		}
		return decider
	default:
		decider, err := newTu2doCloseDeciderFromConfig(provider)
		if err != nil {
			logrus.Warnf("AI平仓决策: provider=%s 配置不完整，已禁用, err=%v", provider, err)
			return nil
		}
		return decider
	}
}

func (d *Tu2doCloseDecider) Decide(snapshot PositionSnapshot) (*AICloseDecision, error) {
	if d == nil {
		return nil, fmt.Errorf("ai close decider is nil")
	}

	var history []AICloseLastDecisionRecord
	if strings.TrimSpace(d.storePath) != "" {
		records, err := loadAICloseLastDecisions(d.storePath)
		if err != nil {
			logrus.Warnf("AI平仓决策: 读取历史建议失败，继续本次判断, file=%s err=%v", d.storePath, err)
		} else if len(records) > 0 {
			history = records
			last := records[len(records)-1]
			snapshot.PreviousDecision = &last
		}
	}

	baseContext, err := BuildAICloseBaseContext(snapshot)
	if err != nil {
		return nil, err
	}
	if len(history) > 0 {
		baseContext += "\n\n最近AI平仓历史（用于识别扛单/反复摇摆）:\n" + buildAICloseHistoryPrompt(history)
	}

	prompt := BuildAICloseDirectPrompt(d.promptTpl, snapshot, baseContext)
	content, err := d.requestAIJSON("你是 BTC 杠杆交易风控裁判。你会一次性完成市场、风险、纪律和执行计划分析，并只输出 JSON，不输出 Markdown。", prompt, "direct")
	if err != nil {
		return nil, err
	}
	decision, err := parseAICloseDecision(content)
	if err != nil {
		return nil, err
	}
	decision.Provider = defaultString(decision.Provider, defaultAICloseProvider)
	decision.Model = d.model
	decision.RawResponse = content

	if strings.TrimSpace(d.storePath) != "" {
		if err := saveAICloseLastDecision(d.storePath, snapshot, decision, time.Now()); err != nil {
			logrus.Warnf("AI平仓决策: 保存本次建议失败, file=%s err=%v", d.storePath, err)
		}
	}
	return decision, nil
}

func (d *Tu2doCloseDecider) requestAIJSON(systemPrompt, userPrompt, stage string) (string, error) {
	reqBody := aiChatCompletionRequest{
		Model: d.model,
		Messages: []aiChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:         d.temperature,
		MaxCompletionTokens: d.maxTokens,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(d.apiURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	logrus.Infof("AI平仓请求[%s]: url=%s model=%s key=%s...", stage, endpoint, d.model, d.apiKey[:min(12, len(d.apiKey))])

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+d.apiKey)
	httpReq.Header.Set("x-codex-agent", "1")

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("ai request failed: stage=%s status=%d body=%s", stage, resp.StatusCode, string(body))
	}

	content, err := parseAIChatContent(body)
	if err != nil {
		return "", err
	}
	return content, nil
}

func buildPositionSummary(details CurrentPositionDetails) string {
	details = enrichPositionAge(details, time.Now())
	parts := []string{
		fmt.Sprintf("inst=%s", details.InstID),
		fmt.Sprintf("side=%s", details.PositionSide),
		fmt.Sprintf("size=%s", details.PositionSize),
		fmt.Sprintf("avg=%s", details.AvgPrice),
		fmt.Sprintf("last=%s", details.LastPrice),
		fmt.Sprintf("liq=%s", details.LiqPrice),
		fmt.Sprintf("margin=%s", details.UseMargin),
		fmt.Sprintf("upl=%s", details.UnrealizedProfit),
		fmt.Sprintf("pnl=%s%%", details.PnLPercent),
		fmt.Sprintf("lever=%s", details.Leverage),
		fmt.Sprintf("mode=%s", details.MarginMode),
		fmt.Sprintf("create_time=%s", defaultString(details.CreateTime, "unknown")),
		fmt.Sprintf("update_time=%s", defaultString(details.UpdateTime, "unknown")),
		fmt.Sprintf("position_age=%s", defaultString(details.PositionAge, "unknown")),
	}
	return strings.Join(parts, ", ")
}

func enrichPositionAge(details CurrentPositionDetails, now time.Time) CurrentPositionDetails {
	if strings.TrimSpace(details.PositionAge) != "" {
		return details
	}
	openedAt, ok := parsePositionTimestamp(details.CreateTime)
	if !ok {
		openedAt, ok = parsePositionTimestamp(details.UpdateTime)
	}
	if !ok || openedAt.After(now) {
		details.PositionAge = "unknown"
		return details
	}

	age := now.Sub(openedAt)
	details.PositionAgeHours = decimal.NewFromFloat(age.Hours())
	details.PositionAge = formatPositionAge(age)
	return details
}

func parsePositionTimestamp(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch {
		case n > 1_000_000_000_000:
			return time.UnixMilli(n), true
		case n > 1_000_000_000:
			return time.Unix(n, 0), true
		}
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
	} {
		if ts, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return ts, true
		}
	}
	return time.Time{}, false
}

func formatPositionAge(age time.Duration) string {
	if age < 0 {
		return "unknown"
	}
	minutes := int(age.Minutes())
	hours := int(age.Hours())
	days := hours / 24
	switch {
	case minutes < 60:
		return fmt.Sprintf("%dm", minutes)
	case hours < 48:
		return fmt.Sprintf("%dh%dm", hours, minutes%60)
	default:
		return fmt.Sprintf("%dd%dh", days, hours%24)
	}
}

func newTu2doCloseDeciderFromConfig(provider string) (*Tu2doCloseDecider, error) {
	apiURL := strings.TrimSpace(vipper.GetString("position.ai_close.api_url"))
	apiKey := strings.TrimSpace(vipper.GetString("position.ai_close.api_key"))
	model := strings.TrimSpace(vipper.GetString("position.ai_close.model"))
	if apiURL == "" {
		return nil, fmt.Errorf("position.ai_close.api_url is empty")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("position.ai_close.api_key is empty")
	}
	if model == "" {
		model = "gpt-4o-mini"
	}

	timeoutSeconds := vipper.GetInt("position.ai_close.timeout_seconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = int(defaultAICloseTimeout / time.Second)
	}
	maxTokens := vipper.GetInt("position.ai_close.max_tokens")
	if maxTokens <= 0 {
		maxTokens = 900
	}
	temperature := vipper.GetFloat64("position.ai_close.temperature")
	if temperature <= 0 {
		temperature = 0.2
	}

	promptTpl := strings.TrimSpace(vipper.GetString("position.ai_close.prompt_template"))
	if promptTpl == "" {
		promptTpl = defaultAIClosePromptTemplate
	}
	storePath := strings.TrimSpace(vipper.GetString("position.ai_close.last_decision_file"))
	if storePath == "" {
		storePath = defaultAICloseLastDecisionFile
	}

	return &Tu2doCloseDecider{
		client: &http.Client{
			Timeout: time.Duration(timeoutSeconds) * time.Second,
		},
		apiURL:      apiURL,
		apiKey:      apiKey,
		model:       model,
		temperature: temperature,
		maxTokens:   maxTokens,
		promptTpl:   promptTpl,
		storePath:   storePath,
	}, nil
}

const defaultAIClosePromptTemplate = `
你是一个谨慎、偏风控的 BTC 合约交易辅助决策员。

你的任务不是替用户直接交易，而是帮助人工判断当前仓位是否应该平仓，以及继续做多/做空的相对胜率。
你是最终裁判，必须综合 5 个专家机器人的 JSON 结果；风控和纪律专家拥有一票否决权。

{data_semantics}

角色要求：
{role}

风控规则：
{rule}

当前触发来源：
{trigger}

当前仓位信息：
{position}

上次 AI 建议（如有）：
{previous_decision}

BTC 当前市场与技术指标：
{btc_market}

5 个专家机器人输出：
{agent_results}

请综合仓位盈亏、强平距离、1H/4H/1D 趋势、均线、MACD、RSI、ATR、布林带、成交量，可用的资金费率/多空比/未平仓量信息，以及 5 个专家机器人的 veto / score / confidence。
如果 risk 或 discipline 专家 veto=true，除非其他证据极强且理由充分，否则 should_close 应偏保守；对于已有持仓，veto=true 通常表示不应加风险，应优先观望、减仓或平仓。

输出要求：
{response}
`

const defaultAICloseDirectPromptTemplate = `
你是一个谨慎、偏风控的 BTC 合约交易辅助决策员。

你的任务不是替用户承诺盈利，而是在一次分析中完成以下判断：
1) 市场环境：趋势、震荡、高波动、关键支撑阻力、量能和动能是否支持当前仓位。
2) 机会质量：当前持仓方向是否仍有优势，是否已经错过最佳退出/加仓/持有窗口。
3) 风险预算：强平距离、杠杆、账户余额、未实现盈亏、ATR 放大风险是否允许继续持仓。
4) 执行计划：继续持有、减仓、平仓或等待时，必须给出清晰止损/止盈与下次巡检间隔。
5) 交易纪律：识别扛单、盈利回撤不保护、过度交易、信号冲突和报复性操作。

{data_semantics}

角色要求：
{role}

风控规则：
{rule}

共享输入：
{base_context}

输出要求：
{response}
`

func BuildAIClosePrompt(template string, snapshot PositionSnapshot) (string, error) {
	if strings.TrimSpace(template) == "" {
		template = defaultAIClosePromptTemplate
	}
	baseContext, err := BuildAICloseBaseContext(snapshot)
	if err != nil {
		return "", err
	}
	vars := defaultAIClosePromptVars(snapshot, baseContext, "")
	return RenderAIPrompt(template, vars), nil
}

func BuildAICloseDirectPrompt(template string, snapshot PositionSnapshot, baseContext string) string {
	if strings.TrimSpace(template) == "" || strings.Contains(template, "{agent_results}") {
		template = defaultAICloseDirectPromptTemplate
	}
	vars := defaultAIClosePromptVars(snapshot, baseContext, "")
	return RenderAIPrompt(template, vars)
}

// defaultAICloseDataSemantics 全仓125x模式下的数据口径说明，平仓/加仓的专家与裁判共用。
func defaultAICloseDataSemantics() string {
	return strings.TrimSpace(`
账户为【全仓(cross) + 125x 杠杆】模式，数据口径如下（重要）:
- 杠杆固定 125x；1张=0.001BTC。仓位占用保证金 use_margin ≈ 开仓价 × 张数 × 0.001 ÷ 125，这只是名义初始保证金，仅用于计算收益率，不是你真正的本金缓冲。
- pnl_percent（仓位盈亏比）= 未实现盈亏 ÷ use_margin，是按【已开张数】计算的当前收益率（杠杆放大后的值），不代表加仓/减仓后的整体收益率。
- 真正的保证金/抗亏损能力是账户余额：avail_bal(可用余额) / total_bal(总权益)，这才是真实本金。全仓模式下整个余额都为仓位兜底。
- 爆仓价由账户余额决定（不是由 use_margin 决定）：余额越大，爆仓价离现价越远、越抗跌/抗涨；仓位名义量相对余额越小越安全。判断爆仓风险要看【余额 vs 仓位名义量】，liq_distance 即据此而来。
- range_1h/4h/1d 给出各周期近20/60根K线的最高/最低价（前高前低参照）；recent_1h/4h/1d 给出最近6根K线的开高低收量。判断关键支撑/阻力、前高前低、止损止盈位时优先参考这些真实高低点。
`)
}

func BuildAICloseBaseContext(snapshot PositionSnapshot) (string, error) {
	parts := []string{
		defaultAICloseDataSemantics(),
		"",
		"当前触发来源:",
		buildAICloseTrigger(snapshot),
		"",
		"当前仓位信息:",
		buildAIClosePositionPrompt(snapshot),
		"",
		"上次 AI 建议（如有）:",
		buildAIClosePreviousDecisionPrompt(snapshot.PreviousDecision),
		"",
		"BTC 当前市场与技术指标:",
		buildBTCMarketPrompt(snapshot.BTCMarket),
	}
	return strings.Join(parts, "\n"), nil
}

func BuildAICloseAgentPrompt(spec aiCloseAgentSpec, baseContext string) string {
	template := `
你是 AI 平仓委员会中的「{display_name}」。

你的职责边界：
{focus}

共享输入：
{base_context}

输出要求：
{response}
`
	vars := map[string]any{
		"display_name": spec.DisplayName,
		"focus":        spec.Focus,
		"base_context": baseContext,
		"response":     spec.Response,
	}
	return RenderAIPrompt(template, vars)
}

func BuildAICloseJudgePrompt(template string, snapshot PositionSnapshot, agentResults []aiCloseAgentResult) (string, error) {
	if strings.TrimSpace(template) == "" {
		template = defaultAIClosePromptTemplate
	}
	if !strings.Contains(template, "{agent_results}") {
		template += "\n\n5 个专家机器人输出：\n{agent_results}\n"
	}
	baseContext, err := BuildAICloseBaseContext(snapshot)
	if err != nil {
		return "", err
	}
	agentReport, err := formatAICloseAgentReport(agentResults)
	if err != nil {
		return "", err
	}
	vars := defaultAIClosePromptVars(snapshot, baseContext, agentReport)
	return RenderAIPrompt(template, vars), nil
}

func defaultAIClosePromptVars(snapshot PositionSnapshot, baseContext, agentReport string) map[string]any {
	if strings.TrimSpace(agentReport) == "" {
		agentReport = "agent_results=[]"
	}
	return map[string]any{
		"data_semantics":    defaultAICloseDataSemantics(),
		"role":              defaultAICloseRole(),
		"rule":              defaultAICloseRules(),
		"trigger":           buildAICloseTrigger(snapshot),
		"position":          buildAIClosePositionPrompt(snapshot),
		"previous_decision": buildAIClosePreviousDecisionPrompt(snapshot.PreviousDecision),
		"btc_market":        buildBTCMarketPrompt(snapshot.BTCMarket),
		"base_context":      baseContext,
		"agent_results":     agentReport,
		"response":          defaultAICloseResponseRequirement(),
	}
}

// RenderAIPrompt 将模板中的 {name} 占位符替换为 vars 中的真实值。
func RenderAIPrompt(template string, vars map[string]any) string {
	re := regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)
	return re.ReplaceAllStringFunc(template, func(token string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(token, "{"), "}")
		value, ok := vars[key]
		if !ok {
			return token
		}
		return fmt.Sprint(value)
	})
}

func defaultAICloseRole() string {
	return strings.TrimSpace(`
- 你只做风险评估和概率判断，不承诺盈利。
- 你的决策风格是：偏激进，敢于持仓和开仓。当信号较明确、风险可控时，坚定持有或开仓，不要因为小波动就轻易建议平仓；只有信号明确反转或强平风险迫近时才考虑退出。
- 本账户仓位 12 张以内均属安全范围，需要有主动承担方向风险的意愿，不要过度保守。
- 高杠杆、强平距离过近（liq_distance < 5%）、趋势明确反转、异常放量时仍必须优先保护本金。
- 当 liq_distance >= 20% 时，强平风险可控；>= 30% 时非常安全，此时绝不因强平风险建议平仓，重点看趋势和动能。
- 当数据不足或信号严重冲突时，降低 confidence，但倾向继续持仓而非保守平仓。
- 开仓建议必须结合当前账户可用余额（avail_bal）和当前仓位占用保证金（use_margin），给出合理的开仓张数（1张=0.001BTC），不能超出账户风险承受能力。
- 平仓或开仓建议必须解释核心原因，避免只给结论。
`)
}

func defaultAICloseRules() string {
	return strings.TrimSpace(`
- should_close=true 表示建议人工优先考虑平仓或减仓；false 表示暂不建议因 AI 信号平仓。
- final_action 表示最终建议：有仓位时用 hold|reduce|close|wait；无仓位时用 no_trade|open_long|open_short|wait。
- 当当前没有仓位时，should_close 必须为 false，重点判断是否值得新开仓；没有高质量机会时 final_action=no_trade 或 wait。
- 如果当前仓位方向与 1H、4H 趋势同时冲突，且亏损扩大或反弹/回撤失败，应提高平仓倾向。
- 如果当前盈利较高但短周期动能衰减、RSI 极端、价格触及布林带外侧后回落，应提高止盈平仓倾向。
- 如果持仓时间较长但盈利没有兑现，机会质量应下降；亏损仓持有越久且趋势未修复，越应提高减仓/平仓倾向。
- 短时间持仓的普通噪音不要过度解读，但高杠杆、强平距离近或 ATR 扩大时除外。
- 如果强平价接近、ATR 扩大、成交量异常放大，应提高风险等级。
- long_win_rate 与 short_win_rate 是接下来一个巡检周期到数小时内的方向胜率估计，范围 0-100。
- long_suggested_hold 是当前若做多建议持仓的时间窗口，例如 "1-2h"、"4-8h"、"1-2d"；若市场不支持做多或方向不明，填 "不建议"。
- short_suggested_hold 是当前若做空建议持仓的时间窗口，例如 "1-2h"、"4-8h"、"1-2d"；若市场不支持做空或方向不明，填 "不建议"。
- long_suggested_size 是结合账户可用余额（avail_bal）、当前仓位占用保证金（use_margin）、杠杆、ATR、强平距离（liq_distance）和风险等级，建议若做多应开仓的具体合约张数，例如 "1张"、"2张"、"与当前仓位相同"；若不支持做多，填 "不建议"。注意：BTC合约 1张 = 0.001 BTC，开仓张数需与账户可用余额和杠杆匹配，不要超出账户承受能力。
- short_suggested_size 是结合账户可用余额（avail_bal）、当前仓位占用保证金（use_margin）、杠杆、ATR、强平距离（liq_distance）和风险等级，建议若做空应开仓的具体合约张数，例如 "1张"、"2张"、"与当前仓位相同"；若不支持做空，填 "不建议"。注意：BTC合约 1张 = 0.001 BTC，开仓张数需与账户可用余额和杠杆匹配，不要超出账户承受能力。
- liq_distance 是当前价格距强平价的百分比距离（正值越小越危险）：liq_distance < 3% 时必须优先平仓或减仓；3%-8% 时需谨慎；8%-30% 时风险相对可控；>=30% 时距离爆仓价很远，强平风险很安全。
- stop_loss_price 是止损/失效价：有持仓时，价格跌破/突破该价说明持仓逻辑已坏，应止损离场；无持仓时，填写「如果此时入场应设的止损价」作为参考（多方向在现价下方、空方向在现价上方）。必须给出，结合 ATR、关键支撑阻力给出。
- take_profit_price 是止盈目标价：有持仓时，到达即可考虑兑现/减仓；无持仓时，填写「如果此时入场的止盈目标价」作为参考。必须给出，结合反弹回落空间、布林轨、前高前低给出。
- 当前无仓位时，stop_loss_price 和 take_profit_price 必须以 continue_side 方向为基准给出「如果此刻入场」的参考价位；continue_side=neutral 时给出多空两方向参考，格式如 "多方止损=xxx/空方止损=yyy"。
- next_check_in 是建议多久后再次检测当前仓位风险，范围 15m~4h（如 "15m"/"30m"/"1h"/"4h"）；临近强平/止盈止损或波动剧烈→偏 15m，平静盈利仓→偏 4h；系统会据此安排下次巡检，必须给出。
- confidence 是你对本次判断的置信度，范围 0-100。
`)
}

func defaultAICloseResponseRequirement() string {
	return strings.TrimSpace(`
只输出一个 JSON 对象，字段固定如下：
{
  "should_close": false,
  "final_action": "hold|reduce|close|no_trade|open_long|open_short|wait",
  "continue_side": "long|short|neutral",
  "long_win_rate": 50,
  "short_win_rate": 50,
  "long_suggested_hold": "4-8h",
  "short_suggested_hold": "2-4h",
  "long_suggested_size": "1张",
  "short_suggested_size": "不建议",
  "stop_loss_price": "当前持仓止损/失效价（无持仓时填 continue_side 方向的参考止损）",
  "take_profit_price": "当前持仓止盈目标价（无持仓时填 continue_side 方向的参考止盈）",
  "long_entry_price": "做多建仓参考价位区间，如 '103000-103500 附近 EMA 支撑处'（无论是否建议做多都必须给出）",
  "long_stop_loss": "做多方向止损价，如 '102000'（必须给出）",
  "long_take_profit": "做多方向止盈目标价，如 '106000'（必须给出）",
  "short_entry_price": "做空建仓参考价位区间，如 '107000-107500 附近阻力处'（无论是否建议做空都必须给出）",
  "short_stop_loss": "做空方向止损价，如 '108500'（必须给出）",
  "short_take_profit": "做空方向止盈目标价，如 '103000'（必须给出）",
  "next_check_in": "1h",
  "confidence": 60,
  "risk_level": "low|medium|high",
  "reason": "用中文给出简洁但具体的依据，包含趋势、动能、波动、仓位风险，以及做多/做空的触发条件"
}
long_entry_price/long_stop_loss/long_take_profit 和 short_entry_price/short_stop_loss/short_take_profit 无论 final_action 是什么都必须给出，帮助人工判断何时介入、如何管理风险。
不要输出 Markdown，不要输出多余解释。
`)
}

func defaultAICloseAgentSpecs() []aiCloseAgentSpec {
	commonResponse := strings.TrimSpace(`
只输出一个 JSON 对象，字段固定如下：
{
  "agent": "固定为你的英文 agent 名称",
  "decision": "pass|caution|veto",
  "veto": false,
  "score": 0,
  "confidence": 60,
  "risk_level": "low|medium|high",
  "continue_side": "long|short|neutral",
  "long_win_rate": 50,
  "short_win_rate": 50,
  "reason": "中文，最多 80 字，只写核心依据"
}
score 范围 0-100。veto=true 表示当前层面不允许增加风险，或建议优先平仓/减仓/观望。
不要输出 Markdown，不要输出多余解释。
`)
	return []aiCloseAgentSpec{
		{
			Name:        "market_regime",
			DisplayName: "市场环境分析机器人",
			System:      "你是 BTC 市场状态识别专家。只判断市场环境，不直接给最终交易裁决。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 判断当前是趋势、震荡、高波动、插针、消息驱动、流动性差还是信号冲突。
- 重点看 1H/4H/1D 趋势一致性、ATR、布林带宽度、成交量和近期 K 线。
- 只回答当前市场是否适合继续持有或承担方向风险，不负责仓位和纪律。
- 如果当前无仓位，只判断市场是否适合新开方向风险；不清晰时应 caution 或 veto。
`),
			Response: strings.ReplaceAll(commonResponse, `"agent": "固定为你的英文 agent 名称"`, `"agent": "market_regime"`),
		},
		{
			Name:        "opportunity_quality",
			DisplayName: "机会质量评估机器人",
			System:      "你是 BTC 杠杆机会筛选专家。只评估当前方向机会质量，不直接给最终交易裁决。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 判断当前持仓方向的机会质量：位置、趋势延续、反转风险、盈亏比、是否追单或错过最佳点。
- 如果当前无仓位，判断是否存在值得新开仓的高质量机会；方向不清、位置不好或盈亏比不足时 veto=true。
- 结合 position_age 判断机会是否已经过期：持仓越久但方向没有兑现，score 越应下降。
- 重点看价格相对均线、布林带、MACD、RSI、最近 1H/4H K 线形态。
- 如果方向可能对但入场位置差，也要降低 score。
`),
			Response: strings.ReplaceAll(commonResponse, `"agent": "固定为你的英文 agent 名称"`, `"agent": "opportunity_quality"`),
		},
		{
			Name:        "risk_budget",
			DisplayName: "风险预算机器人",
			System:      "你是 BTC 杠杆仓位风控专家。强平距离、杠杆和亏损暴露优先于方向判断。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 只判断当前风险是否允许继续持仓或增加风险。
- 如果当前无仓位，默认风险暴露为 0；只有当市场波动和强平风险可控、止损空间清晰时才放行新开仓。
- 重点看杠杆、保证金、未实现盈亏、PnL 百分比、持仓时长、强平价与现价距离、ATR 放大风险。
- 你拥有一票否决权：强平距离过近、亏损扩大、高波动叠加高杠杆时必须 veto=true。
`),
			Response: strings.ReplaceAll(commonResponse, `"agent": "固定为你的英文 agent 名称"`, `"agent": "risk_budget"`),
		},
		{
			Name:        "execution_plan",
			DisplayName: "执行计划机器人",
			System:      "你是 BTC 持仓执行计划专家。只给执行层面的可行性和退出计划，不直接给最终交易裁决。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 判断如果继续持仓，是否存在清晰的失效点、止盈/止损、减仓条件。
- 如果当前无仓位，判断是否能形成清晰开仓计划：方向、入场区间、失效点、止损和止盈；没有计划则 veto=true。
- 如果无法从数据中形成清晰计划，decision 应为 caution 或 veto。
- 对已有持仓，重点判断 position_age 是否已经超过交易逻辑应兑现的时间窗口，是否应该继续观察、减仓、移动止损或直接退出。
`),
			Response: strings.ReplaceAll(commonResponse, `"agent": "固定为你的英文 agent 名称"`, `"agent": "execution_plan"`),
		},
		{
			Name:        "discipline",
			DisplayName: "交易纪律机器人",
			System:      "你是 BTC 杠杆交易纪律审查专家。你专门识别过度交易和情绪风险。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 只判断这次 AI 信号是否应该被纪律层面放行。
- 如果当前无仓位，重点避免因为空仓焦虑而追单；没有高质量机会时应建议继续等待。
- 由于当前输入可能缺少交易历史，缺失数据时不要编造，只降低 confidence。
- 如果持仓时间很长但交易逻辑没有兑现，要识别扛单、不愿认错、盈利回撤不保护等纪律风险。
- 如果信号冲突、触发频繁、仓位风险和行情噪音都高，应 veto=true，避免报复交易和过度操作。
`),
			Response: strings.ReplaceAll(commonResponse, `"agent": "固定为你的英文 agent 名称"`, `"agent": "discipline"`),
		},
	}
}

func buildAICloseTrigger(snapshot PositionSnapshot) string {
	triggerLabel := "unknown"
	switch snapshot.TriggerType {
	case "profit":
		triggerLabel = "profit-threshold"
	case "loss":
		triggerLabel = "loss-threshold"
	case "scheduled":
		triggerLabel = "scheduled-check"
	case "no_position":
		triggerLabel = "no-position-check"
	case "manual_avg_price":
		triggerLabel = "manual-avg-price"
	}
	return fmt.Sprintf("trigger=%s, has_position=%t, pnl_percent=%s%%", triggerLabel, snapshotHasPosition(snapshot), snapshot.PnLPercent.StringFixed(2))
}

func buildAIClosePositionPrompt(snapshot PositionSnapshot) string {
	if !snapshotHasPosition(snapshot) {
		account := snapshot.AccountName
		if snapshot.AccountUID != "" {
			account = fmt.Sprintf("%s-%s", snapshot.AccountName, snapshot.AccountUID)
		}
		balPart := ""
		if snapshot.AvailBal != "" {
			balPart = fmt.Sprintf(", avail_bal=%sUSDT, total_bal=%sUSDT", snapshot.AvailBal, snapshot.TotalBal)
		}
		return fmt.Sprintf("no_open_position=true, account=%s%s, instruction=当前无仓位，请只评估是否值得新开 BTC 合约仓位；没有高质量机会时必须建议 no_trade 或 wait", defaultString(account, "unknown"), balPart)
	}

	base := snapshot.PositionSummary
	if base == "" {
		base = buildPositionSummary(snapshot.CurrentPosition)
	}

	extras := make([]string, 0, 3)
	if snapshot.LiqDistancePercent != "" {
		extras = append(extras, fmt.Sprintf("liq_distance=%s%%", snapshot.LiqDistancePercent))
	}
	if snapshot.AvailBal != "" {
		extras = append(extras, fmt.Sprintf("avail_bal=%sUSDT", snapshot.AvailBal))
		extras = append(extras, fmt.Sprintf("total_bal=%sUSDT", snapshot.TotalBal))
	}
	if len(extras) == 0 {
		return base
	}
	return base + ", " + strings.Join(extras, ", ")
}

func buildAIClosePreviousDecisionPrompt(record *AICloseLastDecisionRecord) string {
	if record == nil || strings.TrimSpace(record.SavedAt) == "" {
		return "previous_ai_decision=nil"
	}
	parts := []string{
		fmt.Sprintf("saved_at=%s", record.SavedAt),
		fmt.Sprintf("account=%s", defaultString(record.AccountName, "unknown")),
		fmt.Sprintf("has_position=%t", record.HasPosition),
	}
	if record.InstID != "" {
		parts = append(parts, fmt.Sprintf("inst=%s", record.InstID))
	}
	if record.PositionSide != "" {
		parts = append(parts, fmt.Sprintf("side=%s", record.PositionSide))
	}
	if record.PnLPercent != "" {
		parts = append(parts, fmt.Sprintf("pnl=%s%%", record.PnLPercent))
	}
	if record.LiqDistancePercent != "" {
		parts = append(parts, fmt.Sprintf("liq_distance=%s%%", record.LiqDistancePercent))
	}

	decision := record.Decision
	parts = append(parts,
		fmt.Sprintf("final_action=%s", defaultString(decision.FinalAction, "unknown")),
		fmt.Sprintf("should_close=%t", decision.ShouldClose),
		fmt.Sprintf("continue_side=%s", defaultString(decision.ContinueSide, "neutral")),
		fmt.Sprintf("long_win_rate=%s%%", defaultString(decision.LongWinRate, "unknown")),
		fmt.Sprintf("short_win_rate=%s%%", defaultString(decision.ShortWinRate, "unknown")),
		fmt.Sprintf("confidence=%s%%", defaultString(decision.Confidence, "unknown")),
		fmt.Sprintf("risk_level=%s", defaultString(decision.RiskLevel, "unknown")),
	)
	if decision.LongSuggestedHold != "" {
		parts = append(parts, fmt.Sprintf("long_suggested_hold=%s", decision.LongSuggestedHold))
	}
	if decision.ShortSuggestedHold != "" {
		parts = append(parts, fmt.Sprintf("short_suggested_hold=%s", decision.ShortSuggestedHold))
	}
	if decision.LongSuggestedSize != "" {
		parts = append(parts, fmt.Sprintf("long_suggested_size=%s", decision.LongSuggestedSize))
	}
	if decision.ShortSuggestedSize != "" {
		parts = append(parts, fmt.Sprintf("short_suggested_size=%s", decision.ShortSuggestedSize))
	}
	if decision.NextCheckIn != "" {
		parts = append(parts, fmt.Sprintf("next_check_in=%s", decision.NextCheckIn))
	}
	if decision.Reason != "" {
		parts = append(parts, fmt.Sprintf("reason=%s", decision.Reason))
	}
	return strings.Join(parts, ", ")
}

const defaultAICloseHistoryLimit = 10

// loadAICloseLastDecisions 读取历史决策列表（按时间从旧到新）。兼容旧的单对象格式。
func loadAICloseLastDecisions(path string) ([]AICloseLastDecisionRecord, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}
	if data[0] == '[' {
		var records []AICloseLastDecisionRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, err
		}
		return records, nil
	}
	// 旧格式：单个对象
	var record AICloseLastDecisionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return []AICloseLastDecisionRecord{record}, nil
}

func saveAICloseLastDecision(path string, snapshot PositionSnapshot, decision *AICloseDecision, now time.Time) error {
	path = strings.TrimSpace(path)
	if path == "" || decision == nil {
		return nil
	}
	record := buildAICloseLastDecisionRecord(snapshot, decision, now)

	history, err := loadAICloseLastDecisions(path)
	if err != nil {
		logrus.Warnf("AI平仓决策: 读取历史用于追加失败，将以新列表覆盖, file=%s err=%v", path, err)
		history = nil
	}
	history = append(history, record)
	if len(history) > defaultAICloseHistoryLimit {
		history = history[len(history)-defaultAICloseHistoryLimit:]
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

// aiHistoryPromptLimit 注入 prompt 的历史条数上限（仅影响展示，落盘仍保留更多）。
const aiHistoryPromptLimit = 5

// buildAICloseHistoryPrompt 把最近若干条平仓决策历史汇总成 prompt 文本（最多取最近 aiHistoryPromptLimit 条）。
func buildAICloseHistoryPrompt(records []AICloseLastDecisionRecord) string {
	if len(records) == 0 {
		return "ai_close_history=空"
	}
	start := 0
	if len(records) > aiHistoryPromptLimit {
		start = len(records) - aiHistoryPromptLimit
	}
	lines := make([]string, 0, aiHistoryPromptLimit)
	for i := len(records) - 1; i >= start; i-- {
		rec := records[i]
		line := fmt.Sprintf("- saved_at=%s, action=%s, should_close=%t, side=%s, pnl=%s%%, liq_distance=%s%%, continue_side=%s",
			defaultString(rec.SavedAt, "unknown"),
			defaultString(rec.Decision.FinalAction, "unknown"),
			rec.Decision.ShouldClose,
			defaultString(rec.PositionSide, "unknown"),
			defaultString(rec.PnLPercent, "unknown"),
			defaultString(rec.LiqDistancePercent, "unknown"),
			defaultString(rec.Decision.ContinueSide, "neutral"))
		if rec.Decision.Reason != "" {
			line += fmt.Sprintf(", reason=%s", rec.Decision.Reason)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "提示：若历史里反复 hold 但 pnl 持续恶化 = 扛单倾向；反复在 close/hold 间摇摆 = 信号不稳，应提高谨慎度")
	return strings.Join(lines, "\n")
}

func buildAICloseLastDecisionRecord(snapshot PositionSnapshot, decision *AICloseDecision, now time.Time) AICloseLastDecisionRecord {
	raw := map[string]interface{}(nil)
	if strings.TrimSpace(decision.RawResponse) != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(decision.RawResponse), &parsed); err == nil {
			raw = parsed
		}
	}
	return AICloseLastDecisionRecord{
		SavedAt:            now.Format(time.RFC3339),
		AccountName:        snapshot.AccountName,
		AccountUID:         snapshot.AccountUID,
		HasPosition:        snapshotHasPosition(snapshot),
		InstID:             defaultString(snapshot.InstID, snapshot.CurrentPosition.InstID),
		PositionID:         defaultString(snapshot.PositionID, snapshot.CurrentPosition.PositionID),
		PositionSide:       defaultString(snapshot.PositionSide, snapshot.CurrentPosition.PositionSide),
		PnLPercent:         snapshot.PnLPercent.StringFixed(2),
		LiqDistancePercent: snapshot.LiqDistancePercent,
		TriggerType:        snapshot.TriggerType,
		Decision: AICloseStoredDecision{
			ShouldClose:        decision.ShouldClose,
			FinalAction:        decision.FinalAction,
			ContinueSide:       decision.ContinueSide,
			LongWinRate:        decision.LongWinRate.StringFixed(2),
			ShortWinRate:       decision.ShortWinRate.StringFixed(2),
			LongSuggestedHold:  decision.LongSuggestedHold,
			ShortSuggestedHold: decision.ShortSuggestedHold,
			LongSuggestedSize:  decision.LongSuggestedSize,
			ShortSuggestedSize: decision.ShortSuggestedSize,
			StopLossPrice:      decision.StopLossPrice,
			TakeProfitPrice:    decision.TakeProfitPrice,
			LongEntryPrice:     decision.LongEntryPrice,
			LongStopLoss:       decision.LongStopLoss,
			LongTakeProfit:     decision.LongTakeProfit,
			ShortEntryPrice:    decision.ShortEntryPrice,
			ShortStopLoss:      decision.ShortStopLoss,
			ShortTakeProfit:    decision.ShortTakeProfit,
			NextCheckIn:        decision.NextCheckIn,
			Confidence:         decision.Confidence.StringFixed(2),
			RiskLevel:          decision.RiskLevel,
			Reason:             decision.Reason,
			Provider:           decision.Provider,
			Model:              decision.Model,
		},
		RawResponse: raw,
	}
}

// computeLiqDistancePercent 计算当前价格距强平价的百分比距离（正值，越小越危险）。
func computeLiqDistancePercent(lastPrice, liqPrice, posSide string) string {
	last, err1 := decimal.NewFromString(strings.TrimSpace(lastPrice))
	liq, err2 := decimal.NewFromString(strings.TrimSpace(liqPrice))
	if err1 != nil || err2 != nil || last.IsZero() || liq.IsZero() {
		return ""
	}
	var dist decimal.Decimal
	switch posSide {
	case "long":
		dist = last.Sub(liq).Div(last).Mul(decimal.NewFromInt(100))
	case "short":
		dist = liq.Sub(last).Div(last).Mul(decimal.NewFromInt(100))
	default:
		return ""
	}
	if dist.IsNegative() {
		return "已触及强平"
	}
	return dist.StringFixed(2)
}

func snapshotHasPosition(snapshot PositionSnapshot) bool {
	if snapshot.TriggerType == "no_position" {
		return false
	}
	return snapshot.HasPosition || strings.TrimSpace(snapshot.PositionSummary) != "" || strings.TrimSpace(snapshot.CurrentPosition.InstID) != "" || strings.TrimSpace(snapshot.PositionID) != ""
}

func buildBTCMarketPrompt(market *BTCAnalysisSnapshot) string {
	if market == nil {
		return "btc_market=nil"
	}
	parts := []string{
		fmt.Sprintf("symbol=%s", market.Symbol),
		formatBTCTodayInfo(market.TodayInfo),
		formatMovingAverage("ema_1h_20", market.EMA1H20),
		formatMovingAverage("ema_1h_50", market.EMA1H50),
		formatMovingAverage("ema_4h_20", market.EMA4H20),
		formatMovingAverage("ema_4h_50", market.EMA4H50),
		formatMovingAverage("ema_1d_20", market.EMA1D20),
		formatMovingAverage("ema_1d_50", market.EMA1D50),
		formatMovingAverage("ma_1d_200", market.MA1D200),
		formatMACD("macd_1h", market.MACD1H),
		formatMACD("macd_4h", market.MACD4H),
		formatMACD("macd_1d", market.MACD1D),
		formatRSI("rsi_1h_14", market.RSI1H14),
		formatRSI("rsi_4h_14", market.RSI4H14),
		formatRSI("rsi_1d_14", market.RSI1D14),
		formatATR("atr_1h_14", market.ATR1H14),
		formatATR("atr_4h_14", market.ATR4H14),
		formatATR("atr_1d_14", market.ATR1D14),
		formatBollinger("boll_1h", market.Bollinger1H),
		formatBollinger("boll_4h", market.Bollinger4H),
		formatBollinger("boll_1d", market.Bollinger1D),
		formatVolume("volume_1h", market.Volume1H),
		formatVolume("volume_4h", market.Volume4H),
		formatVolume("volume_1d", market.Volume1D),
		formatOpenInterest(market.OpenInterest),
		formatFundingRate(market.FundingRate),
		formatLongShortRatio("lsr_1h", market.LongShortRatio1H),
		formatLongShortRatio("lsr_4h", market.LongShortRatio4H),
		formatLongShortRatio("lsr_1d", market.LongShortRatio1D),
		// 区间高低点（前高/前低参照，用于支撑阻力、止损止盈定价）
		formatKlineHighLowRange("range_1h", market.Klines1H, 20, 60),
		formatKlineHighLowRange("range_4h", market.Klines4H, 20, 60),
		formatKlineHighLowRange("range_1d", market.Klines1D, 20, 60),
		formatRecentKlines("recent_1h", market.Klines1H, 6),
		formatRecentKlines("recent_4h", market.Klines4H, 6),
		formatRecentKlines("recent_1d", market.Klines1D, 6),
	}

	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			clean = append(clean, part)
		}
	}
	return strings.Join(clean, "\n")
}

type aiChatCompletionRequest struct {
	Model               string            `json:"model"`
	Messages            []aiChatMessage   `json:"messages"`
	Temperature         float64           `json:"temperature,omitempty"`
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"`
	ResponseFormat      map[string]string `json:"response_format,omitempty"`
}

type aiChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiChatCompletionResponse struct {
	Choices []struct {
		Message aiChatMessage `json:"message"`
	} `json:"choices"`
}

type aiCloseDecisionWire struct {
	ShouldClose        bool            `json:"should_close"`
	FinalAction        string          `json:"final_action"`
	ContinueSide       string          `json:"continue_side"`
	LongWinRate        json.RawMessage `json:"long_win_rate"`
	ShortWinRate       json.RawMessage `json:"short_win_rate"`
	Confidence         json.RawMessage `json:"confidence"`
	RiskLevel          string          `json:"risk_level"`
	Reason             string          `json:"reason"`
	Provider           string          `json:"provider"`
	LongSuggestedHold  string          `json:"long_suggested_hold"`
	ShortSuggestedHold string          `json:"short_suggested_hold"`
	LongSuggestedSize  string          `json:"long_suggested_size"`
	ShortSuggestedSize string          `json:"short_suggested_size"`
	StopLossPrice      string          `json:"stop_loss_price"`
	TakeProfitPrice    string          `json:"take_profit_price"`
	LongEntryPrice     string          `json:"long_entry_price"`
	LongStopLoss       string          `json:"long_stop_loss"`
	LongTakeProfit     string          `json:"long_take_profit"`
	ShortEntryPrice    string          `json:"short_entry_price"`
	ShortStopLoss      string          `json:"short_stop_loss"`
	ShortTakeProfit    string          `json:"short_take_profit"`
	NextCheckIn        string          `json:"next_check_in"`
}

func parseAIChatContent(body []byte) (string, error) {
	var resp aiChatCompletionResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to unmarshal ai response: %w", err)
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("ai response has no message content")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func parseAICloseDecision(content string) (*AICloseDecision, error) {
	jsonText := extractJSONObject(content)
	var wire aiCloseDecisionWire
	if err := json.Unmarshal([]byte(jsonText), &wire); err != nil {
		return nil, fmt.Errorf("failed to parse ai decision json: %w, content=%s", err, content)
	}
	return &AICloseDecision{
		ShouldClose:        wire.ShouldClose,
		FinalAction:        defaultString(wire.FinalAction, defaultAICloseFinalAction(wire.ShouldClose)),
		Reason:             wire.Reason,
		Provider:           wire.Provider,
		Confidence:         parseDecimalPercent(wire.Confidence),
		RiskLevel:          defaultString(wire.RiskLevel, "medium"),
		LongWinRate:        parseDecimalPercent(wire.LongWinRate),
		ShortWinRate:       parseDecimalPercent(wire.ShortWinRate),
		ContinueSide:       defaultString(wire.ContinueSide, "neutral"),
		LongSuggestedHold:  wire.LongSuggestedHold,
		ShortSuggestedHold: wire.ShortSuggestedHold,
		LongSuggestedSize:  wire.LongSuggestedSize,
		ShortSuggestedSize: wire.ShortSuggestedSize,
		StopLossPrice:      wire.StopLossPrice,
		TakeProfitPrice:    wire.TakeProfitPrice,
		LongEntryPrice:     wire.LongEntryPrice,
		LongStopLoss:       wire.LongStopLoss,
		LongTakeProfit:     wire.LongTakeProfit,
		ShortEntryPrice:    wire.ShortEntryPrice,
		ShortStopLoss:      wire.ShortStopLoss,
		ShortTakeProfit:    wire.ShortTakeProfit,
		NextCheckIn:        wire.NextCheckIn,
	}, nil
}

func defaultAICloseFinalAction(shouldClose bool) string {
	if shouldClose {
		return "close"
	}
	return "hold"
}

func parseAICloseAgentResult(agentName, content string) (aiCloseAgentResult, error) {
	jsonText := extractJSONObject(content)
	var result aiCloseAgentResult
	if err := json.Unmarshal([]byte(jsonText), &result); err != nil {
		return aiCloseAgentResult{}, fmt.Errorf("failed to parse ai agent json: agent=%s err=%w content=%s", agentName, err, content)
	}
	result.Agent = defaultString(result.Agent, agentName)
	result.Decision = defaultString(result.Decision, "caution")
	result.RiskLevel = defaultString(result.RiskLevel, "medium")
	result.ContinueSide = defaultString(result.ContinueSide, "neutral")
	result.RawResponse = jsonText
	return result, nil
}

func formatAICloseAgentReport(results []aiCloseAgentResult) (string, error) {
	if len(results) == 0 {
		return "[]", nil
	}
	report := make([]map[string]any, 0, len(results))
	for _, result := range results {
		report = append(report, map[string]any{
			"agent":          result.Agent,
			"decision":       result.Decision,
			"veto":           result.Veto,
			"score":          rawJSONValue(result.Score),
			"confidence":     rawJSONValue(result.Confidence),
			"risk_level":     result.RiskLevel,
			"continue_side":  result.ContinueSide,
			"long_win_rate":  rawJSONValue(result.LongWinRate),
			"short_win_rate": rawJSONValue(result.ShortWinRate),
			"reason":         result.Reason,
		})
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func rawJSONValue(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return string(raw)
	}
	return value
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)

	// 剥离 <think>...</think> 推理块（兼容截断情况）
	if start := strings.Index(content, "<think>"); start >= 0 {
		if end := strings.Index(content, "</think>"); end > start {
			content = strings.TrimSpace(content[end+len("</think>"):])
		} else {
			// think 块未关闭，说明响应被截断，无法提取 JSON
			content = ""
		}
	}

	// 剥离 markdown 代码块
	if strings.HasPrefix(content, "```") {
		if end := strings.LastIndex(content, "```"); end > 0 {
			inner := content[3:end]
			if nl := strings.Index(inner, "\n"); nl >= 0 {
				inner = inner[nl+1:]
			}
			content = strings.TrimSpace(inner)
		}
	}

	if strings.HasPrefix(content, "{") && strings.HasSuffix(content, "}") {
		return content
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start >= 0 && end > start {
		return content[start : end+1]
	}
	return content
}

func parseDecimalPercent(raw json.RawMessage) decimal.Decimal {
	if len(raw) == 0 || string(raw) == "null" {
		return decimal.Zero
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return decimal.NewFromFloat(f)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		s = strings.TrimSuffix(strings.TrimSpace(s), "%")
		if v, err := decimal.NewFromString(s); err == nil {
			return v
		}
	}
	return decimal.Zero
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatBTCTodayInfo(info *BTCTodayInfo) string {
	if info == nil {
		return ""
	}
	return fmt.Sprintf("today: price=%s, change_24h=%s%%, high=%s, low=%s, volume=%s, quote_volume=%s",
		info.CurrentPrice, info.TodayChangePercent, info.TodayHighPrice, info.TodayLowPrice, info.TodayVolume, info.TodayQuoteVolume)
}

func formatMovingAverage(name string, ma *BTCMovingAverage) string {
	if ma == nil {
		return ""
	}
	return fmt.Sprintf("%s: value=%s, interval=%s, period=%d", name, ma.Value, ma.Interval, ma.Period)
}

func formatMACD(name string, macd *BTCMACD) string {
	if macd == nil {
		return ""
	}
	return fmt.Sprintf("%s: macd=%s, signal=%s, histogram=%s", name, macd.MACD, macd.Signal, macd.Histogram)
}

func formatRSI(name string, rsi *BTCRSI) string {
	if rsi == nil {
		return ""
	}
	return fmt.Sprintf("%s: value=%s, period=%d", name, rsi.Value, rsi.Period)
}

func formatATR(name string, atr *BTCATR) string {
	if atr == nil {
		return ""
	}
	return fmt.Sprintf("%s: value=%s, period=%d", name, atr.Value, atr.Period)
}

func formatBollinger(name string, bands *BTCBollingerBands) string {
	if bands == nil {
		return ""
	}
	return fmt.Sprintf("%s: middle=%s, upper=%s, lower=%s, width=%s", name, bands.MiddleBand, bands.UpperBand, bands.LowerBand, bands.BandWidth)
}

func formatVolume(name string, volume *BTCVolumeProfile) string {
	if volume == nil {
		return ""
	}
	return fmt.Sprintf("%s: current=%s, average=%s, ratio=%s, quote=%s", name, volume.CurrentVolume, volume.AverageVolume, volume.VolumeRatio, volume.QuoteVolume)
}

func formatOpenInterest(oi *BTCOpenInterest) string {
	if oi == nil {
		return ""
	}
	return fmt.Sprintf("open_interest: value=%s, change_24h=%s, change_percent=%s", oi.Value, oi.Change24h, oi.ChangePercent)
}

func formatFundingRate(rate *BTCFundingRate) string {
	if rate == nil {
		return ""
	}
	return fmt.Sprintf("funding_rate: current=%s, next_at=%s", rate.CurrentRate, rate.NextFundingAt)
}

func formatLongShortRatio(name string, ratio *BTCLongShortRatio) string {
	if ratio == nil {
		return ""
	}
	return fmt.Sprintf("%s: long=%s, short=%s, ratio=%s", name, ratio.LongRatio, ratio.ShortRatio, ratio.Ratio)
}

func formatSuggestedSize(suggestedSize string) string {
	size := strings.TrimSpace(suggestedSize)
	if size == "" || size == "不建议" {
		if size == "不建议" {
			return "（不建议开仓）"
		}
		return ""
	}
	return fmt.Sprintf("（建议开仓 %s）", size)
}

func formatHoldDescription(suggestedHold string) string {
	hold := strings.TrimSpace(suggestedHold)
	if hold == "" || hold == "不建议" {
		if hold == "不建议" {
			return "（不建议持仓）"
		}
		return ""
	}
	return fmt.Sprintf("（建议持仓 %s）", hold)
}

// formatKlineHighLowRange 输出最近 N 根 K 线的区间最高/最低价（支持多个窗口，如 20、60 根）。
// 用于给 AI 前高/前低、关键支撑阻力、止损止盈的参照。
func formatKlineHighLowRange(name string, klines []BTCKline, windows ...int) string {
	if len(klines) == 0 || len(windows) == 0 {
		return ""
	}
	segs := make([]string, 0, len(windows))
	for _, w := range windows {
		if w <= 0 {
			continue
		}
		n := w
		if n > len(klines) {
			n = len(klines)
		}
		recent := klines[len(klines)-n:]
		hi, lo, ok := klineHighLow(recent)
		if !ok {
			continue
		}
		segs = append(segs, fmt.Sprintf("近%d根(high=%s, low=%s)", n, hi, lo))
	}
	if len(segs) == 0 {
		return ""
	}
	return fmt.Sprintf("%s: %s", name, strings.Join(segs, ", "))
}

// klineHighLow 返回一段 K 线里的最高价与最低价（字符串原值，避免精度损失）。
func klineHighLow(klines []BTCKline) (high, low string, ok bool) {
	var hiV, loV float64
	for i, k := range klines {
		h, errH := strconv.ParseFloat(strings.TrimSpace(k.HighPrice), 64)
		l, errL := strconv.ParseFloat(strings.TrimSpace(k.LowPrice), 64)
		if errH != nil || errL != nil {
			continue
		}
		if !ok {
			hiV, loV, high, low, ok = h, l, k.HighPrice, k.LowPrice, true
			continue
		}
		if h > hiV {
			hiV, high = h, k.HighPrice
		}
		if l < loV {
			loV, low = l, k.LowPrice
		}
		_ = i
	}
	return high, low, ok
}

func formatRecentKlines(name string, klines []BTCKline, limit int) string {
	if len(klines) == 0 {
		return ""
	}
	if limit <= 0 || limit > len(klines) {
		limit = len(klines)
	}
	recent := klines[len(klines)-limit:]
	rows := make([]string, 0, len(recent))
	for _, k := range recent {
		rows = append(rows, fmt.Sprintf("{open=%s, high=%s, low=%s, close=%s, volume=%s}", k.OpenPrice, k.HighPrice, k.LowPrice, k.ClosePrice, k.Volume))
	}
	return fmt.Sprintf("%s: %s", name, strings.Join(rows, ", "))
}

func formatAICloseDecision(decision *AICloseDecision) string {
	if decision == nil {
		return "AI未返回决策"
	}

	longHoldDesc := formatHoldDescription(decision.LongSuggestedHold)
	shortHoldDesc := formatHoldDescription(decision.ShortSuggestedHold)

	longSizeDesc := formatSuggestedSize(decision.LongSuggestedSize)
	shortSizeDesc := formatSuggestedSize(decision.ShortSuggestedSize)

	lines := []string{
		fmt.Sprintf("最终动作: %s", decision.FinalAction),
		fmt.Sprintf("是否建议平仓: %t", decision.ShouldClose),
		fmt.Sprintf("继续方向: %s", decision.ContinueSide),
		fmt.Sprintf("做多胜率: %s%%%s%s", decision.LongWinRate.StringFixed(2), longHoldDesc, longSizeDesc),
		fmt.Sprintf("做空胜率: %s%%%s%s", decision.ShortWinRate.StringFixed(2), shortHoldDesc, shortSizeDesc),
		fmt.Sprintf("置信度: %s%%", decision.Confidence.StringFixed(2)),
		fmt.Sprintf("风险等级: %s", decision.RiskLevel),
	}
	if strings.TrimSpace(decision.StopLossPrice) != "" {
		lines = append(lines, fmt.Sprintf("建议止损/失效价: %s", decision.StopLossPrice))
	}
	if strings.TrimSpace(decision.TakeProfitPrice) != "" {
		lines = append(lines, fmt.Sprintf("建议止盈目标价: %s", decision.TakeProfitPrice))
	}
	// 做多/做空方向场景（无论是否建仓都展示，供人工参考）
	if strings.TrimSpace(decision.LongEntryPrice) != "" || strings.TrimSpace(decision.LongStopLoss) != "" {
		lines = append(lines, "--- 做多场景 ---")
		if strings.TrimSpace(decision.LongEntryPrice) != "" {
			lines = append(lines, fmt.Sprintf("  入场价位: %s", decision.LongEntryPrice))
		}
		if strings.TrimSpace(decision.LongStopLoss) != "" {
			lines = append(lines, fmt.Sprintf("  止损价: %s", decision.LongStopLoss))
		}
		if strings.TrimSpace(decision.LongTakeProfit) != "" {
			lines = append(lines, fmt.Sprintf("  止盈价: %s", decision.LongTakeProfit))
		}
	}
	if strings.TrimSpace(decision.ShortEntryPrice) != "" || strings.TrimSpace(decision.ShortStopLoss) != "" {
		lines = append(lines, "--- 做空场景 ---")
		if strings.TrimSpace(decision.ShortEntryPrice) != "" {
			lines = append(lines, fmt.Sprintf("  入场价位: %s", decision.ShortEntryPrice))
		}
		if strings.TrimSpace(decision.ShortStopLoss) != "" {
			lines = append(lines, fmt.Sprintf("  止损价: %s", decision.ShortStopLoss))
		}
		if strings.TrimSpace(decision.ShortTakeProfit) != "" {
			lines = append(lines, fmt.Sprintf("  止盈价: %s", decision.ShortTakeProfit))
		}
	}
	if decision.NextCheckIn != "" {
		lines = append(lines, fmt.Sprintf("下次风险复检: %s后", decision.NextCheckIn))
	}
	if decision.Provider != "" {
		lines = append(lines, fmt.Sprintf("Provider: %s", decision.Provider))
	}
	if decision.Model != "" {
		lines = append(lines, fmt.Sprintf("模型: %s", decision.Model))
	}
	if decision.Reason != "" {
		lines = append(lines, fmt.Sprintf("原因: %s", decision.Reason))
	}
	return strings.Join(lines, "\n")
}
