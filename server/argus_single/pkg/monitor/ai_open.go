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
	"strings"
	"time"

	"common/middleware/vipper"

	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
)

const (
	defaultAIOpenProvider              = "tu2do"
	defaultAIOpenTimeout               = 120 * time.Second
	defaultAIOpenLastDecisionFile      = "data/ai_open_last_decision.json"
	defaultAIOpenMinLiqDistancePercent = 30.0    // 加仓后爆仓距离百分比下限
	defaultAIOpenMinLiqDistanceUSD     = 10000.0 // 加仓后距爆仓价绝对美元差下限
	defaultAIOpenMaxBalancePercent     = 50.0    // 单次加仓最多占用可用余额百分比
	defaultAIOpenMaxTotalContracts     = 100.0   // 累计净仓位张数上限
	defaultAIOpenCooldownMinutes       = 30      // 同向加仓硬冷却（分钟）
	defaultAIOpenHistoryLimit          = 10      // 历史记录保留条数
	defaultAIOpenMinIntervalMinutes    = 5       // AI 建议巡检间隔下限（分钟）
	defaultAIOpenMaxIntervalMinutes    = 15      // AI 建议巡检间隔上限（分钟）
	defaultAIOpenMinOrderContracts     = 1       // 单次建议张数下限
	defaultAIOpenMaxOrderContracts     = 5       // 单次建议张数上限
	defaultAIOpenLiqSafetyFactor       = 0.9     // 爆仓距离安全系数（线性模型未计维持保证金/手续费，预留 10% 余量）
	btcContractSizeBTC                 = 0.001   // BTC 合约 1 张 = 0.001 BTC
)

// AIOpenDecision 是 AI 对「是否加仓」的激进逆向开仓决策。
type AIOpenDecision struct {
	FinalAction             string // open_long|open_short|no_trade|wait
	Mode                    string // contrarian(逆向)|trend(顺势)|none
	ContinueSide            string
	SentimentState          string // extreme_fear|fear|neutral|greed|extreme_greed
	SuggestedSize           string // AI 建议加仓张数，如 "2张"
	SuggestedBalancePercent string // AI 建议占用可用余额百分比，如 "30%"
	EstLiqPrice             string // AI 估算的加仓后爆仓价
	EstLiqDistanceUSD       decimal.Decimal
	EstLiqDistancePercent   decimal.Decimal
	StopLossPrice           string // 本次操作建议止损/失效价
	TakeProfitPrice         string // 本次操作建议止盈目标价
	LongEntryPrice          string // 做多入场参考价位（无论是否建议做多）
	LongStopLoss            string // 做多止损价
	LongTakeProfit          string // 做多止盈价
	ShortEntryPrice         string // 做空入场参考价位（无论是否建议做空）
	ShortStopLoss           string // 做空止损价
	ShortTakeProfit         string // 做空止盈价
	NextCheckIn             string // AI 建议的下次检测间隔，如 "15m"/"1h"/"4h"
	LongWinRate             decimal.Decimal
	ShortWinRate            decimal.Decimal
	Confidence              decimal.Decimal
	RiskLevel               string
	Reason                  string
	Provider                string
	Model                   string
	RawResponse             string

	// 本地风控复算结果（不信任 AI，二次校验）
	LocalLiqDistanceUSD     decimal.Decimal
	LocalLiqDistancePercent decimal.Decimal
	RequiredMargin          decimal.Decimal // 本次下单估算所需保证金（USDT）
	IsReduce                bool            // 是否为纯减仓/对冲（降低风险，免冷却）
	Flipped                 bool            // 操作后净仓方向是否反转
	RiskPassed              bool            // 是否通过本地硬门槛
	RiskBlockReason         string          // 未通过时的原因
}

// AIOpenDecider 定义 AI 加仓决策接口。
type AIOpenDecider interface {
	Decide(snapshot PositionSnapshot) (*AIOpenDecision, error)
}

// Tu2doOpenDecider 通过 OpenAI-compatible Chat Completions API 请求 AI 加仓建议。
type Tu2doOpenDecider struct {
	client                *http.Client
	apiURL                string
	apiKey                string
	model                 string
	temperature           float64
	maxTokens             int
	minLiqDistancePercent float64
	minLiqDistanceUSD     float64
	maxBalancePercent     float64
	maxTotalContracts     float64
	cooldownMinutes       int
	liqSafetyFactor       float64
	minOrderContracts     int
	maxOrderContracts     int
	storePath             string
}

// NewAIOpenDeciderFromConfig 根据配置构建 AI 加仓决策器；未启用或配置不全时返回 nil。
func NewAIOpenDeciderFromConfig() AIOpenDecider {
	if !vipper.GetBool("position.ai_open.enabled") {
		return nil
	}

	// api_url/key/model 未单独配置时，回退复用 AI 平仓的配置。
	apiURL := firstNonEmpty(vipper.GetString("position.ai_open.api_url"), vipper.GetString("position.ai_close.api_url"))
	apiKey := firstNonEmpty(vipper.GetString("position.ai_open.api_key"), vipper.GetString("position.ai_close.api_key"))
	model := firstNonEmpty(vipper.GetString("position.ai_open.model"), vipper.GetString("position.ai_close.model"))
	if strings.TrimSpace(apiURL) == "" || strings.TrimSpace(apiKey) == "" {
		logrus.Warnf("AI加仓决策: api_url 或 api_key 为空，已禁用")
		return nil
	}
	if strings.TrimSpace(model) == "" {
		model = "gpt-4o-mini"
	}

	timeoutSeconds := vipper.GetInt("position.ai_open.timeout_seconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = int(defaultAIOpenTimeout / time.Second)
	}
	maxTokens := vipper.GetInt("position.ai_open.max_tokens")
	if maxTokens <= 0 {
		maxTokens = 4000
	}
	temperature := vipper.GetFloat64("position.ai_open.temperature")
	if temperature <= 0 {
		temperature = 0.3
	}

	minLiqPct := vipper.GetFloat64("position.ai_open.min_liq_distance_percent")
	if minLiqPct <= 0 {
		minLiqPct = defaultAIOpenMinLiqDistancePercent
	}
	minLiqUSD := vipper.GetFloat64("position.ai_open.min_liq_distance_usd")
	if minLiqUSD <= 0 {
		minLiqUSD = defaultAIOpenMinLiqDistanceUSD
	}
	maxBalPct := vipper.GetFloat64("position.ai_open.max_balance_percent")
	if maxBalPct <= 0 {
		maxBalPct = defaultAIOpenMaxBalancePercent
	}
	maxTotalContracts := vipper.GetFloat64("position.ai_open.max_total_contracts")
	if maxTotalContracts <= 0 {
		maxTotalContracts = defaultAIOpenMaxTotalContracts
	}
	cooldownMinutes := vipper.GetInt("position.ai_open.cooldown_minutes")
	if cooldownMinutes <= 0 {
		cooldownMinutes = defaultAIOpenCooldownMinutes
	}
	liqSafetyFactor := vipper.GetFloat64("position.ai_open.liq_safety_factor")
	if liqSafetyFactor <= 0 || liqSafetyFactor > 1 {
		liqSafetyFactor = defaultAIOpenLiqSafetyFactor
	}
	minOrderContracts := vipper.GetInt("position.ai_open.min_order_contracts")
	if minOrderContracts <= 0 {
		minOrderContracts = defaultAIOpenMinOrderContracts
	}
	maxOrderContracts := vipper.GetInt("position.ai_open.max_order_contracts")
	if maxOrderContracts <= 0 {
		maxOrderContracts = defaultAIOpenMaxOrderContracts
	}
	if maxOrderContracts < minOrderContracts {
		maxOrderContracts = minOrderContracts
	}

	storePath := strings.TrimSpace(vipper.GetString("position.ai_open.last_decision_file"))
	if storePath == "" {
		storePath = defaultAIOpenLastDecisionFile
	}

	return &Tu2doOpenDecider{
		client:                &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		apiURL:                apiURL,
		apiKey:                apiKey,
		model:                 model,
		temperature:           temperature,
		maxTokens:             maxTokens,
		minLiqDistancePercent: minLiqPct,
		minLiqDistanceUSD:     minLiqUSD,
		maxBalancePercent:     maxBalPct,
		maxTotalContracts:     maxTotalContracts,
		cooldownMinutes:       cooldownMinutes,
		liqSafetyFactor:       liqSafetyFactor,
		minOrderContracts:     minOrderContracts,
		maxOrderContracts:     maxOrderContracts,
		storePath:             storePath,
	}
}

func (d *Tu2doOpenDecider) Decide(snapshot PositionSnapshot) (*AIOpenDecision, error) {
	if d == nil {
		return nil, fmt.Errorf("ai open decider is nil")
	}

	baseContext, err := BuildAICloseBaseContext(snapshot)
	if err != nil {
		return nil, err
	}

	// 注入单次张数约束（与本地夹取口径一致）。
	sizeConstraint := fmt.Sprintf("\n\n开仓张数约束：suggested_size 必须为 [%d, %d] 张之间的整数（含上下限）；超出会被系统强制夹到该区间。", d.minOrderContracts, d.maxOrderContracts)
	baseContext += sizeConstraint

	// 注入历史加仓建议（用于冷却判断、避免反复加仓、识别扛单）。
	var history []AIOpenLastDecisionRecord
	if strings.TrimSpace(d.storePath) != "" {
		if records, perr := loadAIOpenLastDecisions(d.storePath); perr != nil {
			logrus.Warnf("AI加仓决策: 读取历史建议失败，继续本次判断, file=%s err=%v", d.storePath, perr)
		} else {
			history = records
			baseContext += "\n\n最近AI加仓历史（用于冷却与扛单识别）:\n" + buildAIOpenHistoryPrompt(records)
		}
	}

	prompt := BuildAIOpenDirectPrompt(baseContext)
	content, err := requestAIChatJSON(d.client, d.apiURL, d.apiKey, d.model, d.temperature, d.maxTokens,
		"你是 BTC 杠杆「激进开仓」决策员。你会一次性完成情绪、趋势、关键价位、风险预算、入场时机和交易纪律分析，并只输出 JSON，不输出 Markdown。", prompt, "direct")
	if err != nil {
		return nil, err
	}
	decision, err := parseAIOpenDecision(content)
	if err != nil {
		return nil, err
	}
	decision.Provider = defaultString(decision.Provider, defaultAIOpenProvider)
	decision.Model = d.model
	decision.RawResponse = content

	// 本地硬风控：用交易所真实爆仓价重算「加仓后」距离，不信任 AI 的估算。
	d.applyLocalRiskGuard(snapshot, decision)

	// 硬冷却：与最近一次「已放行的同向加仓」间隔不足冷却时间时，强制改判 no_trade。
	now := time.Now()
	d.applyCooldownGuard(decision, history, now)

	// 落盘本次建议（追加进历史，最多保留最近 N 条），供下一轮巡检参考。
	if strings.TrimSpace(d.storePath) != "" {
		if err := saveAIOpenLastDecision(d.storePath, snapshot, decision, now); err != nil {
			logrus.Warnf("AI加仓决策: 保存本次建议失败, file=%s err=%v", d.storePath, err)
		}
	}
	return decision, nil
}

// applyCooldownGuard 对「已放行的同向加仓」施加硬冷却（仅限增大净仓的同向加仓；反向减仓不受限）。
func (d *Tu2doOpenDecider) applyCooldownGuard(decision *AIOpenDecision, history []AIOpenLastDecisionRecord, now time.Time) {
	if d.cooldownMinutes <= 0 || !decision.RiskPassed {
		return
	}
	wantsOpen := decision.FinalAction == "open_long" || decision.FinalAction == "open_short"
	if !wantsOpen {
		return
	}
	// #6 纯减仓/对冲是降风险动作，不受加仓冷却限制。
	if decision.IsReduce {
		return
	}

	window := time.Duration(d.cooldownMinutes) * time.Minute
	for i := len(history) - 1; i >= 0; i-- {
		rec := history[i]
		// #7 冷却只看「真正增大净仓的同向加仓」历史：排除减仓记录。
		// 一旦接入自动下单，应改为以 rec.Executed=true 为准（当前为告警期，按"已放行的加仓建议"计冷却以防刷屏）。
		if !rec.RiskPassed || rec.IsReduce || rec.FinalAction != decision.FinalAction {
			continue
		}
		savedAt, err := time.Parse(time.RFC3339, rec.SavedAt)
		if err != nil {
			continue
		}
		elapsed := now.Sub(savedAt)
		if elapsed < window {
			decision.RiskPassed = false
			decision.RiskBlockReason = fmt.Sprintf(
				"距上次同向加仓(%s)仅 %.0f 分钟，未满冷却 %d 分钟，本次跳过加仓",
				rec.FinalAction, elapsed.Minutes(), d.cooldownMinutes)
			decision.FinalAction = "no_trade"
		}
		return // 只看最近一次同向放行记录
	}
}

// applyLocalRiskGuard 用真实持仓数据复算加仓后爆仓距离，决定是否放行。
func (d *Tu2doOpenDecider) applyLocalRiskGuard(snapshot PositionSnapshot, decision *AIOpenDecision) {
	// 只有 AI 建议开仓方向才需要校验；no_trade/wait 直接视为通过（无风险动作）。
	wantsOpen := decision.FinalAction == "open_long" || decision.FinalAction == "open_short"
	if !wantsOpen {
		decision.RiskPassed = true
		return
	}

	// #2 AI 未给出可解析的下单张数 → 无法校验，拒绝。
	opIsLong := decision.FinalAction == "open_long"
	addContracts := parseContracts(decision.SuggestedSize)
	if addContracts.LessThanOrEqual(decimal.Zero) {
		decision.RiskPassed = false
		decision.RiskBlockReason = fmt.Sprintf("AI 未给出有效开仓张数(suggested_size=%q)，无法校验风险", decision.SuggestedSize)
		decision.FinalAction = "no_trade"
		return
	}

	// 单次张数夹取到 [min, max] 整数区间，并同步回 suggested_size（下游下单/展示一致）。
	addContracts = d.clampOrderContracts(addContracts, decision)

	// 空仓开新仓：无现有持仓/爆仓价，按"全仓余额承受能力"估算新仓爆仓距离。
	existing, errExist := decimal.NewFromString(strings.TrimSpace(snapshot.PositionSize))
	hasPosition := errExist == nil && existing.IsPositive() && strings.TrimSpace(snapshot.LiqPrice) != ""
	if !hasPosition {
		d.applyNewPositionRiskGuard(snapshot, addContracts, decision)
		return
	}

	distUSD, distPct, netQty, reduces, flipped, ok := estimatePostAddLiqDistance(
		snapshot.LastPrice, snapshot.LiqPrice, snapshot.PositionSize, snapshot.PositionSide, addContracts, opIsLong, d.liqSafetyFactor)
	if !ok {
		decision.RiskPassed = false
		decision.RiskBlockReason = "无法用真实持仓数据计算操作后爆仓距离（缺少最新价/爆仓价/张数）"
		decision.FinalAction = "no_trade"
		return
	}

	decision.LocalLiqDistanceUSD = distUSD
	decision.LocalLiqDistancePercent = distPct
	decision.Flipped = flipped

	// 纯减仓/对冲（同向缩小净仓、未反转）只会让爆仓距离变远，风险更低，直接放行且免冷却。
	if reduces && !flipped {
		decision.IsReduce = true
		decision.RiskPassed = true
		return
	}

	// #1 余额上限：本次下单所需保证金不得超过 可用余额 × maxBalancePercent。
	if !d.checkMarginBudget(snapshot, addContracts, decision) {
		return
	}

	// 累计仓位上限：操作后净仓张数不得超过配置上限。
	maxTotal := decimal.NewFromFloat(d.maxTotalContracts)
	if netQty.GreaterThan(maxTotal) {
		decision.RiskPassed = false
		decision.RiskBlockReason = fmt.Sprintf(
			"操作后累计净仓 %s 张超过上限 %.0f 张，禁止继续加仓",
			netQty.String(), d.maxTotalContracts)
		decision.FinalAction = "no_trade"
		return
	}

	// #3 方向反转后，爆仓距离已按新方向重算（见 estimatePostAddLiqDistance），同样要过安全线。
	minPct := decimal.NewFromFloat(d.minLiqDistancePercent)
	minUSD := decimal.NewFromFloat(d.minLiqDistanceUSD)
	if distPct.LessThan(minPct) || distUSD.LessThan(minUSD) {
		decision.RiskPassed = false
		flipNote := ""
		if flipped {
			flipNote = "（方向已反转，按新方向计算）"
		}
		decision.RiskBlockReason = fmt.Sprintf(
			"操作后爆仓距离 %.2f%%/%.0fU 低于安全线（需 ≥%.0f%% 且 ≥%.0fU）%s，风险极大，禁止操作",
			distPct.InexactFloat64(), distUSD.InexactFloat64(), d.minLiqDistancePercent, d.minLiqDistanceUSD, flipNote)
		decision.FinalAction = "no_trade"
		return
	}

	decision.RiskPassed = true
}

// checkMarginBudget 校验本次下单所需保证金是否在 可用余额 × maxBalancePercent 预算内。返回 true 表示通过。
func (d *Tu2doOpenDecider) checkMarginBudget(snapshot PositionSnapshot, addContracts decimal.Decimal, decision *AIOpenDecision) bool {
	last, errL := decimal.NewFromString(strings.TrimSpace(snapshot.LastPrice))
	lever, errLev := decimal.NewFromString(strings.TrimSpace(snapshot.CurrentPosition.Leverage))
	avail, errA := decimal.NewFromString(strings.TrimSpace(snapshot.AvailBal))
	if errL != nil || errLev != nil || !lever.IsPositive() || !last.IsPositive() {
		// 数据不全无法校验余额预算，保守放过余额校验但记录提示（爆仓距离仍会兜底）。
		logrus.Warnf("AI加仓决策: 余额预算无法校验(last=%q lever=%q avail=%q)，跳过该项", snapshot.LastPrice, snapshot.CurrentPosition.Leverage, snapshot.AvailBal)
		return true
	}

	// 所需保证金 = 张数 × 0.001BTC × 价格 ÷ 杠杆
	required := addContracts.Mul(decimal.NewFromFloat(btcContractSizeBTC)).Mul(last).Div(lever)
	decision.RequiredMargin = required

	if errA != nil || !avail.IsPositive() {
		decision.RiskPassed = false
		decision.RiskBlockReason = fmt.Sprintf("可用余额未知或为0，无法支撑本次约 %sU 保证金，禁止加仓", required.StringFixed(2))
		decision.FinalAction = "no_trade"
		return false
	}

	budget := avail.Mul(decimal.NewFromFloat(d.maxBalancePercent)).Div(decimal.NewFromInt(100))
	if required.GreaterThan(budget) {
		decision.RiskPassed = false
		decision.RiskBlockReason = fmt.Sprintf(
			"本次约需保证金 %sU，超过余额预算 %sU（可用 %sU × %.0f%%），禁止操作",
			required.StringFixed(2), budget.StringFixed(2), avail.StringFixed(2), d.maxBalancePercent)
		decision.FinalAction = "no_trade"
		return false
	}
	return true
}

// applyNewPositionRiskGuard 校验「空仓开新仓」的风险。
// 全仓模式下新仓爆仓距离 ≈ 账户权益 / 仓位名义量（价格维度），仓位相对余额越小越安全。
func (d *Tu2doOpenDecider) applyNewPositionRiskGuard(snapshot PositionSnapshot, addContracts decimal.Decimal, decision *AIOpenDecision) {
	// 新仓没有"减仓/反转"概念。
	decision.IsReduce = false
	decision.Flipped = false

	last, errL := decimal.NewFromString(strings.TrimSpace(snapshot.LastPrice))
	// 全仓承受能力以账户权益计：优先总余额，回退可用余额。
	balStr := firstNonEmpty(snapshot.TotalBal, snapshot.AvailBal)
	bal, errB := decimal.NewFromString(strings.TrimSpace(balStr))
	if errL != nil || errB != nil || !last.IsPositive() || !bal.IsPositive() {
		decision.RiskPassed = false
		decision.RiskBlockReason = "空仓开新仓：缺少最新价或账户余额，无法估算新仓爆仓距离"
		decision.FinalAction = "no_trade"
		return
	}

	distUSD, distPct, ok := estimateNewPositionLiqDistance(last, bal, addContracts, d.liqSafetyFactor)
	if !ok {
		decision.RiskPassed = false
		decision.RiskBlockReason = "空仓开新仓：无法估算新仓爆仓距离（张数无效）"
		decision.FinalAction = "no_trade"
		return
	}
	decision.LocalLiqDistanceUSD = distUSD
	decision.LocalLiqDistancePercent = distPct

	// #1 余额预算
	if !d.checkMarginBudget(snapshot, addContracts, decision) {
		return
	}

	// 累计仓位上限：新仓张数不得超过上限。
	if addContracts.GreaterThan(decimal.NewFromFloat(d.maxTotalContracts)) {
		decision.RiskPassed = false
		decision.RiskBlockReason = fmt.Sprintf("新仓 %s 张超过累计上限 %.0f 张，禁止开仓", addContracts.String(), d.maxTotalContracts)
		decision.FinalAction = "no_trade"
		return
	}

	// 爆仓安全线
	minPct := decimal.NewFromFloat(d.minLiqDistancePercent)
	minUSD := decimal.NewFromFloat(d.minLiqDistanceUSD)
	if distPct.LessThan(minPct) || distUSD.LessThan(minUSD) {
		decision.RiskPassed = false
		decision.RiskBlockReason = fmt.Sprintf(
			"新仓爆仓距离 %.2f%%/%.0fU 低于安全线（需 ≥%.0f%% 且 ≥%.0fU），风险极大，禁止开新仓",
			distPct.InexactFloat64(), distUSD.InexactFloat64(), d.minLiqDistancePercent, d.minLiqDistanceUSD)
		decision.FinalAction = "no_trade"
		return
	}

	decision.RiskPassed = true
}

// estimateNewPositionLiqDistance 估算空仓全仓开新仓后的爆仓距离。
// 承受能力 ≈ 账户权益 bal(USDT)，新仓名义量 = 张数×0.001BTC；价格维度可承受跌/涨幅 = bal / qtyBTC。
// distUSD = bal / qtyBTC × safetyFactor，distPct = distUSD / last × 100。
func estimateNewPositionLiqDistance(last, balance, addContracts decimal.Decimal, safetyFactor float64) (distUSD, distPct decimal.Decimal, ok bool) {
	qtyBTC := addContracts.Abs().Mul(decimal.NewFromFloat(btcContractSizeBTC))
	if !qtyBTC.IsPositive() || !last.IsPositive() || !balance.IsPositive() {
		return decimal.Zero, decimal.Zero, false
	}
	if safetyFactor <= 0 || safetyFactor > 1 {
		safetyFactor = defaultAIOpenLiqSafetyFactor
	}
	distUSD = balance.Div(qtyBTC).Mul(decimal.NewFromFloat(safetyFactor))
	distPct = distUSD.Div(last).Mul(decimal.NewFromInt(100))
	return distUSD, distPct, true
}

// estimatePostAddLiqDistance 估算全仓净仓模式下「操作后」距爆仓价的距离。
// 净仓签名：多仓为 +，空仓为 -；本次买入为 +，卖出为 -。
//   - 同向加仓 → 净仓张数变大 → 价格缓冲按比例缩小（风险升高）。
//   - 反向缩小（未反转）→ 净仓变小、缓冲变大（风险降低）→ reduces=true。
//   - 反向反超（净仓方向翻转）→ flipped=true：按翻转后的新方向、用守恒的亏损承受能力重算距离。
//
// 模型：全仓模式下当前 liqPx 已隐含账户权益，buffer×existing 即总亏损承受能力(USD·张)，近似守恒：
//
//	newBuffer = buffer * existing / |existing + op|
//
// safetyFactor∈(0,1] 预留维持保证金/手续费余量（线性模型未计），最终距离再乘以该系数（更保守）。
func estimatePostAddLiqDistance(lastPrice, liqPrice, existingContracts, posSide string, addContracts decimal.Decimal, opIsLong bool, safetyFactor float64) (distUSD, distPct, netQty decimal.Decimal, reduces, flipped, ok bool) {
	last, err1 := decimal.NewFromString(strings.TrimSpace(lastPrice))
	liq, err2 := decimal.NewFromString(strings.TrimSpace(liqPrice))
	existing, err3 := decimal.NewFromString(strings.TrimSpace(existingContracts))
	if err1 != nil || err2 != nil || err3 != nil || last.IsZero() || liq.IsZero() || !existing.IsPositive() {
		return decimal.Zero, decimal.Zero, decimal.Zero, false, false, false
	}

	buffer := last.Sub(liq).Abs()
	if !buffer.IsPositive() {
		return decimal.Zero, decimal.Zero, decimal.Zero, false, false, false
	}

	if safetyFactor <= 0 || safetyFactor > 1 {
		safetyFactor = defaultAIOpenLiqSafetyFactor
	}
	factor := decimal.NewFromFloat(safetyFactor)

	// 现有净仓签名张数
	existingSigned := existing
	if strings.EqualFold(strings.TrimSpace(posSide), "short") {
		existingSigned = existing.Neg()
	}
	// 本次操作签名张数
	op := addContracts.Abs()
	if !opIsLong {
		op = op.Neg()
	}

	newSigned := existingSigned.Add(op)
	netQty = newSigned.Abs()

	// 是否方向反转：操作前后签名乘积为负即翻转（任一为 0 不算翻转）。
	flipped = existingSigned.Mul(newSigned).IsNegative()
	// 纯减仓：净仓变小且未翻转。
	reduces = netQty.LessThan(existing) && !flipped

	// 净仓归零：无持仓即无爆仓风险，给一个极大的安全距离。
	if netQty.IsZero() {
		return last, decimal.NewFromInt(100), netQty, true, false, true
	}

	// 守恒模型对同向加仓、反向缩小、反向翻转都成立：距离幅度 = buffer×existing/newQty。
	// 翻转时 liq 落在价格另一侧，但我们只用距离幅度做 ≥30%/≥10000U 校验，故公式一致。
	newBuffer := buffer.Mul(existing).Div(netQty).Mul(factor)
	distUSD = newBuffer
	distPct = newBuffer.Div(last).Mul(decimal.NewFromInt(100))
	return distUSD, distPct, netQty, reduces, flipped, true
}

// clampOrderContracts 将建议张数向下取整并夹到 [minOrderContracts, maxOrderContracts]，
// 同步更新 decision.SuggestedSize，返回夹取后的整数张数（decimal）。
func (d *Tu2doOpenDecider) clampOrderContracts(n decimal.Decimal, decision *AIOpenDecision) decimal.Decimal {
	minC := decimal.NewFromInt(int64(d.minOrderContracts))
	maxC := decimal.NewFromInt(int64(d.maxOrderContracts))

	c := n.Floor() // 整数张
	if c.LessThan(minC) {
		c = minC
	}
	if c.GreaterThan(maxC) {
		c = maxC
	}
	if decision != nil {
		decision.SuggestedSize = c.String() + "张"
	}
	return c
}

// parseContracts 从 "2张"/"2"/"与当前仓位相同" 等文本中解析加仓张数；无法解析时返回 0。
func parseContracts(text string) decimal.Decimal {
	t := strings.TrimSpace(text)
	t = strings.ReplaceAll(t, "张", "")
	t = strings.TrimSpace(t)
	if t == "" {
		return decimal.Zero
	}
	if v, err := decimal.NewFromString(t); err == nil {
		return v
	}
	return decimal.Zero
}

// parseNextCheckInterval 解析 AI 给的下次巡检间隔（"15m"/"1h"/"4h"/"1-2h" 等）。
// 解析失败返回 ok=false；成功结果会被夹在 [minInterval, maxInterval] 内。
func parseNextCheckInterval(s string, minInterval, maxInterval time.Duration) (time.Duration, bool) {
	if minInterval <= 0 {
		minInterval = time.Duration(defaultAIOpenMinIntervalMinutes) * time.Minute
	}
	if maxInterval <= 0 {
		maxInterval = time.Duration(defaultAIOpenMaxIntervalMinutes) * time.Minute
	}
	if maxInterval < minInterval {
		maxInterval = minInterval
	}
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0, false
	}
	// 取第一个数字（兼容 "1-2h" 这类区间，取较短的一端）
	numStr := nextCheckNumberRe.FindString(s)
	if numStr == "" {
		return 0, false
	}
	n, err := decimal.NewFromString(numStr)
	if err != nil || !n.IsPositive() {
		return 0, false
	}

	var unit time.Duration
	switch {
	case strings.Contains(s, "d"): // day
		unit = 24 * time.Hour
	case strings.Contains(s, "h"): // hour
		unit = time.Hour
	default: // minute（m/min 或无单位默认按分钟）
		unit = time.Minute
	}

	d := time.Duration(n.InexactFloat64() * float64(unit))
	if d < minInterval {
		d = minInterval
	}
	if d > maxInterval {
		d = maxInterval
	}
	return d, true
}

var nextCheckNumberRe = regexp.MustCompile(`\d+(\.\d+)?`)

// aiOpenSizingGuide 公共的「可开张数估算」指引，让 AI 自行判断余额是否够用、给出不会爆仓的张数。
const aiOpenSizingGuide = `
可开张数估算（务必据此判断余额够不够、给出安全 suggested_size；1张=0.001BTC，P=最新价，杠杆=leverage）：
【空仓开新仓 / 全仓模式】账户权益≈total_bal(无则用avail_bal)：
- 余额预算约束：每张保证金 = 0.001×P÷杠杆；受预算限制的最多张数 N_margin = avail_bal×50% ÷ (0.001×P÷杠杆)。
- 爆仓距离约束（新仓爆仓距离 ≈ 账户权益 ÷ 仓位BTC量）：
  · 满足距爆仓价 ≥10000U：N_usd = 权益 ÷ (0.001×10000) = 权益 ÷ 10。
  · 满足爆仓距离 ≥30%：   N_pct = 权益 ÷ (0.001×0.30×P)。
- 安全可开张数上限 N_max ≈ min(N_margin, N_usd, N_pct)，且不超过累计上限 100 张。
- suggested_size 不得超过 N_max；若 N_max < 1，说明余额不足以安全开仓，应 no_trade 并在 reason 里说明"余额不足"。

【已有持仓加仓】用当前距爆仓价(USD)的缓冲 buffer 与现有张数 existing：
- 加仓后爆仓距离 ≈ buffer × existing ÷ (existing + 加仓张数)。
- 反推：要保持加仓后距离 ≥ max(10000U, 0.30×P)，加仓张数上限 = existing × (buffer ÷ 目标距离 − 1)。
- 同时受余额预算约束 N_margin（同上）。suggested_size 取两者较小值；不满足则 no_trade。
`

// BuildAIOpenAgentPrompt 构造单个开仓专家的 prompt。
func BuildAIOpenAgentPrompt(spec aiCloseAgentSpec, baseContext string) string {
	template := `
你是 BTC「激进开仓」委员会中的「{display_name}」。

本委员会有两种交易模式，由裁判按市场状态二选一（互斥）：
1) 逆向模式（别人恐惧我贪婪）：极端恐慌+超卖→抄底做多；极端贪婪+超买→抄顶做空。
   - 严禁在单边崩盘/逼空里接飞刀：只有出现止跌/见顶确认信号时才逆向进场。
2) 顺势模式（跟随趋势）：1D/4H/1H 趋势一致、动能确认的强趋势里顺势开仓——下跌趋势→做空，上涨趋势→做多。
   - 必须在回调/反抽进场（不追在极值），并确认趋势未衰竭。
原则：强趋势+健康回调 → 走顺势；趋势末端/极端情绪+关键位+有反转确认 → 走逆向；两者都不清晰 → 观望。
风险底线：BTC 历史单日涨跌幅极少超过 30% 且通常有反弹；只要操作后爆仓距离 ≥30% 且距爆仓价 ≥10000U，方向风险即视为极低，可激进。

合约面值：BTC 合约 1 张 = 0.001 BTC（计算名义价值、保证金、张数时统一按此换算）。

场景说明（重要）：
- 账户为 net 模式，最多只有一个净仓位；当前可能有持仓，也可能空仓。
- 空仓时：评估是否值得开一个新仓。全仓模式下新仓爆仓距离 ≈ 账户权益 / 仓位名义量，仓位相对余额越小爆仓距离越远；只要满足 ≥30% / ≥10000U 即可开，张数要小而稳。
- 有持仓时：允许同向加仓（如多 10 张→再买入）使净仓变大；也允许反向操作（如多 10 张→卖出 3 张，净仓变多 7 张）使净仓变小。
- 反向操作等于减仓/对冲，只会让爆仓距离变远、风险降低，可大胆建议。
- 同向加仓/开新仓会让爆仓距离逼近，必须严格守住 ≥30% / ≥10000U 安全线。
{sizing_guide}
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
		"sizing_guide": aiOpenSizingGuide,
		"base_context": baseContext,
		"response":     spec.Response,
	}
	return RenderAIPrompt(template, vars)
}

// BuildAIOpenJudgePrompt 构造裁判 prompt。
func BuildAIOpenJudgePrompt(snapshot PositionSnapshot, agentResults []aiCloseAgentResult) (string, error) {
	baseContext, err := BuildAICloseBaseContext(snapshot)
	if err != nil {
		return "", err
	}
	agentReport, err := formatAICloseAgentReport(agentResults)
	if err != nil {
		return "", err
	}
	vars := map[string]any{
		"base_context":  baseContext,
		"agent_results": agentReport,
		"sizing_guide":  aiOpenSizingGuide,
		"rule":          defaultAIOpenRules(),
		"response":      defaultAIOpenResponseRequirement(),
	}
	return RenderAIPrompt(defaultAIOpenJudgeTemplate, vars), nil
}

func BuildAIOpenDirectPrompt(baseContext string) string {
	vars := map[string]any{
		"base_context": baseContext,
		"sizing_guide": aiOpenSizingGuide,
		"rule":         defaultAIOpenRules(),
		"response":     defaultAIOpenResponseRequirement(),
	}
	return RenderAIPrompt(defaultAIOpenDirectTemplate, vars)
}

const defaultAIOpenJudgeTemplate = `
你是 BTC 杠杆「激进开仓」委员会的最终裁判。

核心交易哲学：别人恐惧我贪婪，别人贪婪我恐惧。市场越极端，逆向机会越好。
账户为 net 模式最多一个净仓位：空仓时评估是否开新仓（张数小而稳），有持仓时允许同向加仓或反向减仓/对冲。
全仓模式下爆仓距离取决于「仓位/余额」比例而非仅杠杆，仓位相对余额越小越安全。
合约面值：BTC 合约 1 张 = 0.001 BTC。
{sizing_guide}
共享输入：
{base_context}

6 个专家机器人输出：
{agent_results}

裁决规则：
{rule}

输出要求：
{response}
`

const defaultAIOpenDirectTemplate = `
你是 BTC 杠杆「激进开仓」决策员。

你只发送一次请求并直接给最终裁决，不再拆分专家与裁判。请在一次分析中同时完成以下工作：
1) 逆向情绪：判断 fear/greed/extreme_fear/extreme_greed，识别是否存在别人恐惧我贪婪、别人贪婪我恐惧的机会。
2) 顺势趋势：判断 1D/4H/1H 趋势是否一致，是否适合顺势回调/反抽进场。
3) 关键价位：结合布林带、EMA、近 20/60 根 K 线高低点、ATR，判断支撑/阻力与盈亏比。
4) 风险预算：结合可用余额、总余额、杠杆、当前仓位、爆仓价，估算操作后爆仓距离和保证金占用。
5) 入场时机：区分可立即入场、等待确认、禁止追单；逆向要确认止跌/见顶，顺势要避免追末端。
6) 交易纪律：拦截接飞刀、扛单加仓、过度交易、报复性加仓和黑天鹅高波动。

核心交易哲学：别人恐惧我贪婪，别人贪婪我恐惧。市场越极端，逆向机会越好，但单边崩盘/逼空且无反转确认时禁止接飞刀。
账户为 net 模式最多一个净仓位：空仓时评估是否开新仓，有持仓时允许同向加仓或反向减仓/对冲。
全仓模式下爆仓距离取决于「仓位/余额」比例而非仅杠杆，仓位相对余额越小越安全。
合约面值：BTC 合约 1 张 = 0.001 BTC。

{sizing_guide}

共享输入：
{base_context}

裁决规则：
{rule}

输出要求：
{response}
`

func defaultAIOpenRules() string {
	return strings.TrimSpace(`
模式分流（先定模式 mode，再定方向）：
- 先看 trend_follow 与 sentiment_contrarian 哪个信号更强、更干净：
  · 强趋势确认(trend_follow score 高) + 处在顺势回调/反抽进场点 → mode=trend，方向跟随趋势（下跌→open_short，上涨→open_long）。
  · 极端情绪(sentiment_contrarian score 高，extreme 级) + 贴关键支撑/阻力 + 有止跌/见顶反转确认 → mode=contrarian，方向逆向（恐慌→open_long，贪婪→open_short）。
  · 两者都不清晰、或互相矛盾且都不强 → mode=none，no_trade/wait。
- 同一时刻只能二选一；趋势中段优先顺势，趋势末端/极值反转才考虑逆向。

硬否决与强度：
- open_risk_budget 或 contrarian_discipline 任一 veto=true → final_action=no_trade（硬否决）。
- 本账户仓位 12 张以内均属安全范围，需要有主动承担方向风险的意愿，不要过度保守。
- 选定模式后，其主信号 score>=60 且 entry_timing 不反对 且 key_level 盈亏比尚可 → open，suggested_size 可激进（倾向给出上限附近张数）。
- 主信号 score 35-60 且其余无强烈反对 → 可操作，suggested_size 取中等值，不要轻易降为 no_trade。
- 主信号 score<35 或 entry_timing=不入场 → wait 或 no_trade。

方向与仓位关系：
- 空仓时：评估是否开新仓。情绪极端+位置好+爆仓安全才 open，张数要小（仓位相对余额越小爆仓越远）；机会一般则 no_trade/wait。
- 有持仓时：final_action 方向独立于持仓方向——与持仓同向=加仓（净仓变大，风险升高）；与持仓反向=减仓/对冲（净仓变小，风险降低）。
- 反向减仓/对冲只会让爆仓距离变远，风险更低，当情绪/位置支持反向时可大胆建议。
- 同向加仓/开新仓必须严格守住爆仓安全线（本地会用真实数据复算 ≥30% / ≥10000U，不达标会被强制改判 no_trade）。

浮亏加仓 vs 扛单（关键，二者必须区分）：
- 浮亏时同向加仓既可能是合理摊平/顺势补仓，也可能是扛单爆仓的主因。
- 只有同时满足以下全部条件，浮亏同向加仓才允许，否则一律视为扛单 → no_trade：
  (a) 加仓后爆仓距离仍 ≥30% 且距爆仓价 ≥10000U；
  (b) 有明确依据：mode=contrarian 时情绪达 extreme 级且出现反转确认；mode=trend 时趋势仍强且未衰竭、在回调点补仓；
  (c) 不是「逆向接飞刀」也不是「顺势追末端」——逆向时禁止在无止跌信号的单边崩盘里补；顺势时禁止在动能衰竭/趋势末端追；
  (d) 当前浮亏未过深（参考 pnl_percent，若已深度亏损说明前次判断大概率已错，不应继续加码）；
  (e) 距上次操作有足够冷却（见"最近AI加仓历史"），且未在短期内反复操作。
- 盈利时同向加仓（顺势补仓）风险更可控，但需 entry_timing 确认不是追在极值。
- 任何"越亏越加且不满足上述条件"的情形，contrarian_discipline 应 veto。

字段说明：
- suggested_size 是建议操作的合约张数（1张=0.001BTC），必须为整数且落在系统给定的张数区间内（见"开仓张数约束"），同时结合可用余额与杠杆，不得超出账户承受能力。
- suggested_balance_percent 是本次操作建议占用可用余额(avail_bal)的百分比，同向加仓不应超过 50%。
- est_liq_price / est_liq_distance_usd / est_liq_distance_percent 是你估算的「操作后」爆仓价及距离，供本地二次校验；本地会用交易所真实数据复算，估算偏差不影响最终风控。
- stop_loss_price 是止损/失效价：无论 final_action 是什么，都必须给出。建仓时为本次操作止损；no_trade/wait 时填写「如果此刻入场应设的止损参考价」，帮助判断何时值得入场。
- take_profit_price 是止盈目标价：无论 final_action 是什么，都必须给出。建仓时为本次目标价；no_trade/wait 时填写「如果此刻入场的止盈参考价」，结合反弹/回落空间、布林中轨/对侧轨。
- next_check_in 是建议多久后再次巡检本账户，范围 5m~15m（例如 "5m"/"8m"/"10m"/"15m"）；越不稳定/越临近关键位越短，越平静越接近 15m；必须给出，默认 15m。
`)
}

func defaultAIOpenResponseRequirement() string {
	return strings.TrimSpace(`
只输出一个 JSON 对象，字段固定如下：
{
  "final_action": "open_long|open_short|no_trade|wait",
  "mode": "contrarian|trend|none",
  "continue_side": "long|short|neutral",
  "sentiment_state": "extreme_fear|fear|neutral|greed|extreme_greed",
  "suggested_size": "2张",
  "suggested_balance_percent": "30%",
  "est_liq_price": "预估操作后爆仓价",
  "est_liq_distance_usd": 12500,
  "est_liq_distance_percent": 32.5,
  "stop_loss_price": "本次操作止损价（no_trade/wait 时填建议方向的参考止损）",
  "take_profit_price": "本次操作止盈价（no_trade/wait 时填建议方向的参考止盈）",
  "long_entry_price": "做多建仓参考价位区间，如 '103000-103500 附近 EMA 支撑处'（无论是否建议做多都必须给出）",
  "long_stop_loss": "做多方向止损价（必须给出）",
  "long_take_profit": "做多方向止盈目标价（必须给出）",
  "short_entry_price": "做空建仓参考价位区间，如 '107000-107500 附近阻力处'（无论是否建议做空都必须给出）",
  "short_stop_loss": "做空方向止损价（必须给出）",
  "short_take_profit": "做空方向止盈目标价（必须给出）",
  "next_check_in": "5m~15m 内取值，如 5m/8m/10m/15m",
  "long_win_rate": 65,
  "short_win_rate": 35,
  "confidence": 70,
  "risk_level": "low|medium|high",
  "reason": "中文，说明选哪个模式(逆向/顺势)及依据+趋势/情绪/关键价位+爆仓安全距离+做多/做空触发条件"
}
long_entry_price/long_stop_loss/long_take_profit 和 short_entry_price/short_stop_loss/short_take_profit 无论 final_action 是什么都必须给出，帮助人工判断何时介入、如何管理风险。
mode 必须与 final_action 一致：mode=trend 时方向跟随趋势，mode=contrarian 时方向逆向；no_trade/wait 时 mode=none。
next_check_in 必须在 5m~15m 区间内决策（行情越急/越临近关键位越接近 5m，越平静越接近 15m），不得给出超出该区间的值。
不要输出 Markdown，不要输出多余解释。
`)
}

func defaultAIOpenAgentSpecs() []aiCloseAgentSpec {
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
score 范围 0-100，表示本层面支持「本次开仓/加仓」的强度。veto=true 表示当前层面不允许开仓。
不要输出 Markdown，不要输出多余解释。
`)
	withName := func(name string) string {
		return strings.ReplaceAll(commonResponse, `"agent": "固定为你的英文 agent 名称"`, fmt.Sprintf(`"agent": "%s"`, name))
	}

	return []aiCloseAgentSpec{
		{
			Name:        "sentiment_contrarian",
			DisplayName: "逆向情绪机器人",
			System:      "你是 BTC 市场情绪逆向专家，核心信条是别人恐惧我贪婪、别人贪婪我恐惧。只量化情绪极端度并给出逆向方向，只输出 JSON。",
			Focus: strings.TrimSpace(`
- 量化市场情绪极端度，输出逆向方向（这是逆向模式的主信号源；顺势模式由 trend_follow 负责）。
- RSI(1H/4H/1D)：<30 恐慌→偏做多，<20 为极端；>70 贪婪→偏做空，>80 为极端；多周期共振才算强信号。
- 资金费率：< -0.01%/8h 偏空过度→逆向做多，> 0.05%/8h 偏多过度→逆向做空（数值越极端 score 越高）。
- 多空比：account/position 多空比 > 1.5 多方主导→逆向做空，< 0.67 空方主导→逆向做多。
- 24h 涨跌幅：单日跌幅 > 5% 为恐慌买点，涨幅 > 5% 为贪婪卖点，越大越极端。
- sentiment_state 判定：多个极端指标共振→extreme_fear/extreme_greed；仅个别偏离→fear/greed；否则 neutral。
- 必须区分「健康回调中的恐慌」（可逆向，score 高）与「趋势性崩盘/逼空中的恐慌」（1D 趋势明确反转+放量跌破/突破关键均线，不可接刀，veto）。
- 资金费率、多空比为 nil 时不要编造，按中性处理并降低 confidence。
- 情绪不极端时 score 低，宁可不操作。
`),
			Response: withName("sentiment_contrarian"),
		},
		{
			Name:        "trend_follow",
			DisplayName: "顺势跟随机器人",
			System:      "你是 BTC 趋势跟随专家，只判断是否存在可顺势进场的强趋势及方向，不做最终裁决。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 量化趋势强度与方向（这是顺势模式的主信号源）。
- 趋势确认：1D/4H/1H 均线多空排列是否一致、MACD 方向与柱体是否扩张、价格是否站稳/跌破关键均线。
- 下跌趋势（三线空头排列+MACD空头+放量下跌）→ continue_side=short；上涨趋势（三线多头+MACD多头）→ continue_side=long。
- 进场质量：必须是「顺势回调/反抽」——下跌趋势反抽到 EMA/前低附近做空、上涨趋势回踩 EMA/前高附近做多；追在极值(刚急跌/急涨末端)则降分。
- 趋势衰竭信号（动能背离、ATR 收敛、量能萎缩、价格反复触不破关键位）→ 降分或 veto，避免在趋势末端追单。
- 震荡/无明确趋势 → score 低，本模式不适用。
`),
			Response: withName("trend_follow"),
		},
		{
			Name:        "key_level",
			DisplayName: "关键价位机器人",
			System:      "你是 BTC 关键价位与盈亏比专家。只判断当前价格相对支撑/阻力的位置和操作盈亏比，不做最终裁决。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 判断当前价格相对关键价位的位置，给出本次操作（无论逆向或顺势）的盈亏比。
- 支撑位：布林下轨、EMA(1D/4H) 均线簇、近 1D 低点；阻力位：布林上轨、EMA 均线簇、近 1D 高点。
- 逆向做多看是否贴近强支撑、逆向做空看是否贴近强阻力；顺势进场看回调/反抽是否到达可依托的均线/前高前低。
- 价格处在区间中部、无明显支撑/阻力依托时盈亏比差，降分。
- 同向加仓：评估加仓后均价相对关键位是否更被动；反向减仓：评估是否在好位置兑现风险。
- 结合 ATR 判断目标空间是否足够；空间不足 veto。
`),
			Response: withName("key_level"),
		},
		{
			Name:        "open_risk_budget",
			DisplayName: "加仓风险预算机器人",
			System:      "你是 BTC 杠杆加仓风控专家。爆仓距离和余额占用优先于方向判断，你拥有一票否决权。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 评估「操作后」的风险，而非当前仓位。
- 空仓开新仓：新仓爆仓距离 ≈ 账户权益 / 仓位名义量，张数越小越安全。硬规则：≥30% 且 ≥10000USDT → 放行；否则 veto。
- 同向加仓：净仓变大、爆仓距离逼近。硬规则同上：≥30% 且 ≥10000USDT → 放行；否则 veto=true。
- 反向减仓/对冲：净仓变小、爆仓距离变远，风险降低，不需 veto（除非把仓位反转到更危险方向）。
- 用真实杠杆(leverage)、均价、爆仓价(liq_price)、账户余额(avail_bal/total_bal)重算操作后爆仓距离。
- 核对可用余额(avail_bal)：开新仓/同向加仓占用保证金不得超过可用余额的 50%。
- 杠杆越高、ATR 越大、已有亏损越深，越要压低张数或直接 veto。
`),
			Response: withName("open_risk_budget"),
		},
		{
			Name:        "entry_timing",
			DisplayName: "入场时机机器人",
			System:      "你是 BTC 入场时机专家，同时服务逆向与顺势两种模式，只判断当前是否到了可进场的时点，不做最终裁决。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 逆向模式时机：恐惧/贪婪是否已到极值并出现反转确认——做多看企稳（缩量、下影线/锤子线、触布林下轨后回升）；做空看见顶（放量滞涨、上影线、触布林上轨后回落）；仍在加速单边运行=未到，降分。
- 顺势模式时机：是否处在「顺势回调/反抽」的进场点——下跌趋势反抽到 EMA/前低附近、上涨趋势回踩 EMA/前高附近；追在极值（刚急跌/急涨）则降分。
- 结合近 3 根 1H/4H K 线形态。给出倾向：立即入场=score 高 / 等待确认=caution / 不入场=veto。
`),
			Response: withName("entry_timing"),
		},
		{
			Name:        "contrarian_discipline",
			DisplayName: "交易纪律机器人",
			System:      "你是 BTC 杠杆交易纪律专家，拦截两类典型错误：逆向接飞刀、顺势追末端，以及黑天鹅/过度交易/报复性加仓。你拥有一票否决权。只输出 JSON。",
			Focus: strings.TrimSpace(`
- 你的职责是拦截「错误的进场」，对逆向和顺势两种模式都适用；反向减仓/对冲不在拦截范围。
- 逆向风险：1D/4H/1H 趋势一致单边且无止跌/见顶确认时做逆向 = 接飞刀，veto（这是逆向模式专属红线，但不要因此否决顺势进场）。
- 顺势风险：趋势已衰竭（动能背离、ATR 收敛、量能萎缩、价格在关键位反复触不破）却追单 = 追末端，veto。
- 黑天鹅：成交量异常放大（超历史均值 3 倍以上）→ 波动失控，veto。
- 过度交易：结合「最近AI加仓历史」，距上次同向操作很近 / 短期反复操作 → veto。
- 报复性：浮亏时同向加仓必须满足（极端情绪或强趋势确认）+爆仓安全+浮亏不深+冷却足够，否则视为扛单，veto。
- 缺失数据（资金费率、多空比等为 nil）时不要编造，按中性处理并只降低 confidence。
`),
			Response: withName("contrarian_discipline"),
		},
	}
}

type aiOpenDecisionWire struct {
	FinalAction             string          `json:"final_action"`
	Mode                    string          `json:"mode"`
	ContinueSide            string          `json:"continue_side"`
	SentimentState          string          `json:"sentiment_state"`
	SuggestedSize           string          `json:"suggested_size"`
	SuggestedBalancePercent string          `json:"suggested_balance_percent"`
	EstLiqPrice             string          `json:"est_liq_price"`
	EstLiqDistanceUSD       json.RawMessage `json:"est_liq_distance_usd"`
	EstLiqDistancePercent   json.RawMessage `json:"est_liq_distance_percent"`
	StopLossPrice           string          `json:"stop_loss_price"`
	TakeProfitPrice         string          `json:"take_profit_price"`
	LongEntryPrice          string          `json:"long_entry_price"`
	LongStopLoss            string          `json:"long_stop_loss"`
	LongTakeProfit          string          `json:"long_take_profit"`
	ShortEntryPrice         string          `json:"short_entry_price"`
	ShortStopLoss           string          `json:"short_stop_loss"`
	ShortTakeProfit         string          `json:"short_take_profit"`
	NextCheckIn             string          `json:"next_check_in"`
	LongWinRate             json.RawMessage `json:"long_win_rate"`
	ShortWinRate            json.RawMessage `json:"short_win_rate"`
	Confidence              json.RawMessage `json:"confidence"`
	RiskLevel               string          `json:"risk_level"`
	Reason                  string          `json:"reason"`
	Provider                string          `json:"provider"`
}

func parseAIOpenDecision(content string) (*AIOpenDecision, error) {
	jsonText := extractJSONObject(content)
	var wire aiOpenDecisionWire
	if err := json.Unmarshal([]byte(jsonText), &wire); err != nil {
		return nil, fmt.Errorf("failed to parse ai open decision json: %w, content=%s", err, content)
	}
	return &AIOpenDecision{
		FinalAction:             defaultString(wire.FinalAction, "no_trade"),
		Mode:                    defaultString(wire.Mode, "none"),
		ContinueSide:            defaultString(wire.ContinueSide, "neutral"),
		SentimentState:          defaultString(wire.SentimentState, "neutral"),
		SuggestedSize:           wire.SuggestedSize,
		SuggestedBalancePercent: wire.SuggestedBalancePercent,
		EstLiqPrice:             wire.EstLiqPrice,
		EstLiqDistanceUSD:       parseDecimalPercent(wire.EstLiqDistanceUSD),
		EstLiqDistancePercent:   parseDecimalPercent(wire.EstLiqDistancePercent),
		StopLossPrice:           wire.StopLossPrice,
		TakeProfitPrice:         wire.TakeProfitPrice,
		LongEntryPrice:          wire.LongEntryPrice,
		LongStopLoss:            wire.LongStopLoss,
		LongTakeProfit:          wire.LongTakeProfit,
		ShortEntryPrice:         wire.ShortEntryPrice,
		ShortStopLoss:           wire.ShortStopLoss,
		ShortTakeProfit:         wire.ShortTakeProfit,
		NextCheckIn:             wire.NextCheckIn,
		LongWinRate:             parseDecimalPercent(wire.LongWinRate),
		ShortWinRate:            parseDecimalPercent(wire.ShortWinRate),
		Confidence:              parseDecimalPercent(wire.Confidence),
		RiskLevel:               defaultString(wire.RiskLevel, "medium"),
		Reason:                  wire.Reason,
		Provider:                wire.Provider,
	}, nil
}

// formatAIOpenDecision 把开仓决策格式化成 Telegram 文本。
func formatAIOpenDecision(decision *AIOpenDecision) string {
	if decision == nil {
		return "AI未返回开仓决策"
	}
	modeLabel := map[string]string{"contrarian": "逆向(抄底/抄顶)", "trend": "顺势(跟趋势)", "none": "无"}[decision.Mode]
	if modeLabel == "" {
		modeLabel = decision.Mode
	}
	// 是否真的要开/加仓（只有此时张数/止损/止盈/爆仓估算才有意义）。
	willTrade := decision.FinalAction == "open_long" || decision.FinalAction == "open_short"

	lines := []string{
		fmt.Sprintf("最终动作: %s", decision.FinalAction),
		fmt.Sprintf("交易模式: %s", modeLabel),
		fmt.Sprintf("方向: %s", decision.ContinueSide),
		fmt.Sprintf("情绪状态: %s", decision.SentimentState),
		fmt.Sprintf("做多胜率: %s%%", decision.LongWinRate.StringFixed(2)),
		fmt.Sprintf("做空胜率: %s%%", decision.ShortWinRate.StringFixed(2)),
		fmt.Sprintf("置信度: %s%%", decision.Confidence.StringFixed(2)),
		fmt.Sprintf("风险等级: %s", decision.RiskLevel),
	}

	// 建仓时展示张数/爆仓估算；止损止盈无论是否建仓都展示（no_trade/wait 时为入场参考价）。
	if willTrade {
		if strings.TrimSpace(decision.SuggestedSize) != "" {
			lines = append(lines, fmt.Sprintf("建议张数: %s (占用余额 %s)", decision.SuggestedSize, defaultString(decision.SuggestedBalancePercent, "未提供")))
		}
		if decision.Flipped {
			lines = append(lines, "⚠️ 该操作会反转净仓方向")
		}
		if decision.IsReduce {
			lines = append(lines, "性质: 反向减仓/对冲（降低风险，免冷却）")
		}
		if decision.RequiredMargin.IsPositive() {
			lines = append(lines, fmt.Sprintf("本地估算所需保证金: %sU", decision.RequiredMargin.StringFixed(2)))
		}
		if decision.LocalLiqDistancePercent.IsPositive() {
			lines = append(lines, fmt.Sprintf("本地复算操作后爆仓距离(权威): %s%% / %sU",
				decision.LocalLiqDistancePercent.StringFixed(2), decision.LocalLiqDistanceUSD.StringFixed(0)))
		}
		if decision.EstLiqPrice != "" {
			lines = append(lines, fmt.Sprintf("AI估算爆仓价(仅参考，以本地复算为准): %s", decision.EstLiqPrice))
		}
	}
	// 当前操作方向的止损止盈（建仓时为本次，no_trade/wait 时为建议方向参考）
	stopLabel := "建议止损/失效价"
	tpLabel := "建议止盈目标价"
	if !willTrade {
		stopLabel = "方向参考止损(当前未开仓)"
		tpLabel = "方向参考止盈(当前未开仓)"
	}
	if strings.TrimSpace(decision.StopLossPrice) != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", stopLabel, decision.StopLossPrice))
	}
	if strings.TrimSpace(decision.TakeProfitPrice) != "" {
		lines = append(lines, fmt.Sprintf("%s: %s", tpLabel, decision.TakeProfitPrice))
	}
	// 做多/做空方向场景（无论是否建仓都展示）
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

	if strings.TrimSpace(decision.NextCheckIn) != "" {
		lines = append(lines, fmt.Sprintf("下次巡检间隔: %s", decision.NextCheckIn))
	}
	if decision.RiskPassed {
		lines = append(lines, "本地风控: ✅ 通过安全线")
	} else if strings.TrimSpace(decision.RiskBlockReason) != "" {
		lines = append(lines, fmt.Sprintf("本地风控: ⛔ 拦截 - %s", decision.RiskBlockReason))
	}
	if decision.Model != "" {
		lines = append(lines, fmt.Sprintf("模型: %s", decision.Model))
	}
	if decision.Reason != "" {
		lines = append(lines, fmt.Sprintf("原因: %s", decision.Reason))
	}
	return strings.Join(lines, "\n")
}

// ============================= 加仓决策历史 =============================

// AIOpenLastDecisionRecord 落盘的上一次加仓建议，用于冷却判断与扛单识别。
type AIOpenLastDecisionRecord struct {
	SavedAt                 string `json:"saved_at"`
	AccountName             string `json:"account_name,omitempty"`
	AccountUID              string `json:"account_uid,omitempty"`
	InstID                  string `json:"inst_id,omitempty"`
	PositionSide            string `json:"position_side,omitempty"`
	PositionSize            string `json:"position_size,omitempty"`
	PnLPercent              string `json:"pnl_percent,omitempty"`
	FinalAction             string `json:"final_action,omitempty"`
	Mode                    string `json:"mode,omitempty"`
	ContinueSide            string `json:"continue_side,omitempty"`
	SentimentState          string `json:"sentiment_state,omitempty"`
	SuggestedSize           string `json:"suggested_size,omitempty"`
	SuggestedBalancePercent string `json:"suggested_balance_percent,omitempty"`
	IsReduce                bool   `json:"is_reduce"`
	Flipped                 bool   `json:"flipped,omitempty"`
	Executed                bool   `json:"executed"` // 是否真实下单（告警期恒为 false；接自动下单后据此计冷却）
	RiskPassed              bool   `json:"risk_passed"`
	RiskBlockReason         string `json:"risk_block_reason,omitempty"`
	Confidence              string `json:"confidence,omitempty"`
	RiskLevel               string `json:"risk_level,omitempty"`
	Reason                  string `json:"reason,omitempty"`
}

// loadAIOpenLastDecisions 读取历史记录列表（按时间从旧到新）。
// 兼容旧格式：文件若是单个对象则包装成单元素列表。
func loadAIOpenLastDecisions(path string) ([]AIOpenLastDecisionRecord, error) {
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
		var records []AIOpenLastDecisionRecord
		if err := json.Unmarshal(data, &records); err != nil {
			return nil, err
		}
		return records, nil
	}
	// 旧格式：单个对象
	var record AIOpenLastDecisionRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}
	return []AIOpenLastDecisionRecord{record}, nil
}

func saveAIOpenLastDecision(path string, snapshot PositionSnapshot, decision *AIOpenDecision, now time.Time) error {
	path = strings.TrimSpace(path)
	if path == "" || decision == nil {
		return nil
	}
	record := AIOpenLastDecisionRecord{
		SavedAt:                 now.Format(time.RFC3339),
		AccountName:             snapshot.AccountName,
		AccountUID:              snapshot.AccountUID,
		InstID:                  defaultString(snapshot.InstID, snapshot.CurrentPosition.InstID),
		PositionSide:            defaultString(snapshot.PositionSide, snapshot.CurrentPosition.PositionSide),
		PositionSize:            defaultString(snapshot.PositionSize, snapshot.CurrentPosition.PositionSize),
		PnLPercent:              snapshot.PnLPercent.StringFixed(2),
		FinalAction:             decision.FinalAction,
		Mode:                    decision.Mode,
		ContinueSide:            decision.ContinueSide,
		SentimentState:          decision.SentimentState,
		SuggestedSize:           decision.SuggestedSize,
		SuggestedBalancePercent: decision.SuggestedBalancePercent,
		IsReduce:                decision.IsReduce,
		Flipped:                 decision.Flipped,
		Executed:                false, // 告警期不下单；接自动下单后此处写入真实成交结果
		RiskPassed:              decision.RiskPassed,
		RiskBlockReason:         decision.RiskBlockReason,
		Confidence:              decision.Confidence.StringFixed(2),
		RiskLevel:               decision.RiskLevel,
		Reason:                  decision.Reason,
	}
	// 追加进历史，保留最近 defaultAIOpenHistoryLimit 条。
	history, err := loadAIOpenLastDecisions(path)
	if err != nil {
		logrus.Warnf("AI加仓决策: 读取历史用于追加失败，将以新列表覆盖, file=%s err=%v", path, err)
		history = nil
	}
	history = append(history, record)
	if len(history) > defaultAIOpenHistoryLimit {
		history = history[len(history)-defaultAIOpenHistoryLimit:]
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

// buildAIOpenHistoryPrompt 把最近若干条历史汇总成 prompt 文本。
func buildAIOpenHistoryPrompt(records []AIOpenLastDecisionRecord) string {
	if len(records) == 0 {
		return "ai_open_history=空（无历史，视为首次评估，无冷却约束）"
	}
	start := 0
	if len(records) > aiHistoryPromptLimit {
		start = len(records) - aiHistoryPromptLimit
	}
	lines := make([]string, 0, aiHistoryPromptLimit)
	for i := len(records) - 1; i >= start; i-- {
		rec := records[i]
		line := fmt.Sprintf("- saved_at=%s, action=%s, mode=%s, risk_passed=%t, side=%s, size=%s张, pnl=%s%%, sentiment=%s, suggested=%s",
			defaultString(rec.SavedAt, "unknown"),
			defaultString(rec.FinalAction, "unknown"),
			defaultString(rec.Mode, "none"),
			rec.RiskPassed,
			defaultString(rec.PositionSide, "unknown"),
			defaultString(rec.PositionSize, "unknown"),
			defaultString(rec.PnLPercent, "unknown"),
			defaultString(rec.SentimentState, "unknown"),
			defaultString(rec.SuggestedSize, "unknown"))
		if rec.RiskBlockReason != "" {
			line += fmt.Sprintf(", risk_block=%s", rec.RiskBlockReason)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "提示：若距上次同向 risk_passed 加仓时间很近，应警惕反复加仓/扛单，提高 veto 倾向（注：本地另有硬冷却兜底）")
	return strings.Join(lines, "\n")
}

// requestAIChatJSON 向 OpenAI-compatible Chat Completions 接口请求一次 JSON 输出。
func requestAIChatJSON(client *http.Client, apiURL, apiKey, model string, temperature float64, maxTokens int, systemPrompt, userPrompt, stage string) (string, error) {
	reqBody := aiChatCompletionRequest{
		Model: model,
		Messages: []aiChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature:         temperature,
		MaxCompletionTokens: maxTokens,
		ResponseFormat: map[string]string{
			"type": "json_object",
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	endpoint := strings.TrimRight(apiURL, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	logrus.Infof("AI加仓请求[%s]: url=%s model=%s key=%s...", stage, endpoint, model, apiKey[:min(12, len(apiKey))])

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("x-codex-agent", "1")

	resp, err := client.Do(httpReq)
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

	return parseAIChatContent(body)
}
