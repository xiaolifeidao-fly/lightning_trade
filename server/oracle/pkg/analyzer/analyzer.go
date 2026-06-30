package analyzer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	argusTrade "argus_single/pkg/trade"
	tradeDTO "service/trade/dto"

	"oracle/pkg/calibration"
	"oracle/pkg/collector"
	"oracle/pkg/indicator"
	"oracle/pkg/oraclecfg"

	"github.com/sirupsen/logrus"
)

const systemPrompt = `你是一名严谨的加密货币合约趋势分析师。基于给定的多周期K线、技术指标、逐笔成交压力与资金费率，对「当前时刻起，经过指定时间间隔之后」给出可交易的判断。

核心交付是三件套：方向(trend) + 概率波动区间(expected_high_pct/expected_low_pct) + 失效价位(invalidation_pct)，并用 confidence 如实标定把握度。
不要追求精确点位：expected_move_pct 仅作为区间中枢/方向锚，价格本质是分布而非确定值，宁可把区间给对，也不要赌一个具体点。

只能输出 JSON，禁止任何额外文字或 Markdown。字段如下：
{
  "trend": "long|short|neutral",   // 方向判断(核心)
  "signal": "buy|sell|hold",       // 操作信号；与 trend 不一致时以谨慎为先
  "expected_high_pct": 数字,        // 预测期间内最高价相对当前价的涨幅(百分比，通常≥0)。区间上沿
  "expected_low_pct": 数字,         // 预测期间内最低价相对当前价的跌幅(百分比，通常≤0)。区间下沿
  "expected_move_pct": 数字,        // 区间中枢(方向锚，百分比)：long 取正、short 取负、neutral 取0。仅作锚点，非精确点位预测
  "invalidation_pct": 数字,         // 失效距离(百分比，正数)：方向被证伪的关键价位与当前价的距离。long 指下方支撑跌破位、short 指上方阻力突破位；0=不给
  "confidence": 0~1 的小数,         // 方向正确的主观概率，需如实标定
  "stop_loss_pct": 数字,            // 建议止损距离(百分比，正数；0=不给)。可与失效位不同：失效位是结构性证伪点，止损含风控缓冲
  "take_profit_pct": 数字,          // 建议止盈距离(百分比，正数；0=不给)
  "reason": "中文，简要说明依据(含失效逻辑：跌破/突破何处则方向作废)，<=200字"
}

判定与标定要求：
- 方向以高周期(高TF)趋势为主导，主周期均线排列、MACD、RSI、布林带位置共同验证；顺势优先，逆势需有明确反转证据。
- expected_high_pct/expected_low_pct 描述预测期间内的价格波动区间(这是最有价值的输出)，须满足 expected_high_pct ≥ expected_move_pct ≥ expected_low_pct 且 expected_high_pct ≥ 0 ≥ expected_low_pct，区间宽度结合 ATR 合理给出，禁止编造大幅跳变。
- 区间标定目标：把上下沿给到「真实最高/最低有约 80% 概率落在区间内」的宽度——不是越窄越好，宁可略宽以保证命中；系统性过窄(经常被真实波动击穿)会被校准判为失真。
- expected_move_pct 仅为区间中枢，不超过该周期波动率/ATR 所隐含的幅度；不要把它当成"必将到达的精确价"。
- invalidation_pct 是方向判断的证伪条件：基于近端支撑/阻力等结构位给出。long 时填"当前价到下方关键支撑"的距离，short 时填"当前价到上方关键阻力"的距离；该位被有效突破即说明方向判断失效。neutral 或无清晰结构位时填 0。
- confidence 标定：多周期共振且信号干净→0.6~0.85；信号一般→0.4~0.6；趋势纠缠、指标矛盾或关键数据为 N/A → ≤0.4 且优先 trend=neutral、signal=hold。
- 指标用法：RSI>70 超买/<30 超卖；价格触布林上轨偏阻力、触下轨偏支撑；MACD 柱由负转正偏多、由正转负偏空；成交量趋势>1 放量(趋势更可信)、<1 缩量(谨防假突破)，注意量价背离；主动买占比>60% 偏多、<40% 偏空；资金费率为正且偏高=多头拥挤(警惕回调)，为负且偏低=空头拥挤。
- trend 与 expected_move_pct 的符号必须一致：long 配正值、short 配负值、neutral 配 0，禁止自相矛盾。
- 凡标注 N/A 的指标表示数据不足，不得当作真实数值参与判断。
- 若提供压力面(上方做空压力位/下方做多压力位)，作为结构性参考：临近上方阻力时谨防上行受阻(long 的区间上沿/止盈宜保守、short 失效位更可信)，临近下方支撑时谨防下行获撑(short 的区间下沿宜保守、long 失效位更可信)；压力面带其计算时间，越旧权重越低，与最新K线结构冲突时以最新价格结构为准，勿单凭压力面反转结论。
- 止损止盈以百分比距离给出，结合 ATR 与近端支撑/阻力，使盈亏比合理(通常止盈距离≥止损距离)。`

// Decision AI 解析后的结构化预测。
// 模型只输出方向与百分比(expected_move_pct/stop_loss_pct/take_profit_pct)，
// 绝对价格(PredictPrice/StopLoss/TakeProfit)由系统据当前价换算，避免模型直接编造绝对价。
type Decision struct {
	Trend           string  `json:"trend"`
	Signal          string  `json:"signal"`
	ExpectedMovePct float64 `json:"expected_move_pct"`
	ExpectedHighPct float64 `json:"expected_high_pct"`
	ExpectedLowPct  float64 `json:"expected_low_pct"`
	InvalidationPct float64 `json:"invalidation_pct"` // 失效距离(百分比，正数)：方向被证伪的关键价位距当前价的距离
	Confidence      float64 `json:"confidence"`
	StopLossPct     float64 `json:"stop_loss_pct"`
	TakeProfitPct   float64 `json:"take_profit_pct"`
	Reason          string  `json:"reason"`

	// Efficiency 形态/趋势效率 = |中枢幅度| / 区间宽度：→0 震荡为主(宜区间回踩入场)，→1 趋势干净(宜顺势)。
	// 系统据已校验的区间派生，非模型输出；随 decision 落入 RawResponse 供策略层分流入场用，不单列建表。
	Efficiency float64 `json:"efficiency"`

	// 以下为系统换算后的绝对价（落库用），不来自模型。
	PredictPrice float64 `json:"predict_price"`
	PredictHigh  float64 `json:"predict_high"` // 预测期间最高价
	PredictLow   float64 `json:"predict_low"`  // 预测期间最低价
	Invalidation float64 `json:"invalidation"` // 失效价位(绝对价)：方向被证伪的关键价位
	StopLoss     float64 `json:"stop_loss"`
	TakeProfit   float64 `json:"take_profit"`
}

// Analyzer 调用 LLM 完成趋势预测。
type Analyzer struct {
	cfg    oraclecfg.AIConfig
	client *http.Client
	// calib 校准回环反馈器：把离线打分学到的修正作用回本次预测。
	// 默认 Noop(恒等)，未注入前预测行为不变——校准口子已留出但默认不生效。
	calib calibration.Calibrator
}

func New(cfg oraclecfg.AIConfig) *Analyzer {
	return &Analyzer{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
		calib:  calibration.Noop{},
	}
}

// UseCalibrator 注入校准回环反馈器；传 nil 视为 Noop。
// 这是「评估 → 反馈 → 预测」闭环接入预测层的唯一入口；在被显式调用前默认 Noop，预测不受影响。
func (a *Analyzer) UseCalibrator(c calibration.Calibrator) {
	if c == nil {
		c = calibration.Noop{}
	}
	a.calib = c
}

// Analyze 用快照 + 特征(+可选消息面/压力面)调用 LLM，返回结构化预测与原始返回。
// newsSummary/pressureSummary 为空时不注入对应内容；二者均为辅助修正项，技术面仍为主导。
// pressureDirectional=false 时压力面仅作纯价位参考(不附带方向倾向)，用于短周期避免持续给某一方向加权。
func (a *Analyzer) Analyze(ctx context.Context, snap *collector.Snapshot, f indicator.Features, newsSummary, pressureSummary string, pressureDirectional bool) (*Decision, string, error) {
	if strings.TrimSpace(a.cfg.APIURL) == "" || strings.TrimSpace(a.cfg.APIKey) == "" {
		return nil, "", fmt.Errorf("AI 配置缺失(api_url/api_key)")
	}

	newsBlock := ""
	if strings.TrimSpace(newsSummary) != "" {
		newsBlock = "\n" + newsSummary +
			"消息面仅作辅助：与技术面冲突时以技术面为主，可据消息面适度调整方向把握度(confidence)或风险，勿单凭消息面反转结论。\n"
	}

	pressureBlock := ""
	if strings.TrimSpace(pressureSummary) != "" {
		if pressureDirectional {
			pressureBlock = "\n" + pressureSummary +
				"压力面为结构性参考：临近上方阻力时对 long 的区间上沿/止盈需保守、short 失效位更可信；临近下方支撑时反之。注意其计算时间，越旧权重越低，与最新K线结构冲突时以最新价格为准。\n"
		} else {
			pressureBlock = "\n" + pressureSummary +
				"压力面仅作价位参考：以上为近端支撑/阻力位，仅用于校准波动区间上下沿、止盈与失效价位，不代表方向倾向；方向仍以多周期K线与指标为准，勿据压力位反推多空。注意其计算时间，越旧权重越低。\n"
		}
	}

	userPrompt := fmt.Sprintf(
		"交易对=%s 平台=%s\n当前价=%.4f 当前时间=%s 预测目标=当前时刻起经过 %s 之后\n\n%s%s%s\n请按 system 要求只输出 JSON。",
		snap.Symbol, snap.Platform, f.LastClose, time.Now().Format("2006-01-02 15:04:05"), snap.Interval,
		indicator.Summary(snap, f), newsBlock, pressureBlock)

	// 请求 + 解析最多重试一次，避免偶发的网络抖动或非法 JSON 直接丢弃整次预测。
	var (
		decision Decision
		raw      string
		lastErr  error
	)
	for attempt := 0; attempt < 2; attempt++ {
		raw, lastErr = a.requestChatJSON(ctx, systemPrompt, userPrompt)
		if lastErr != nil {
			continue
		}
		decision = Decision{}
		if e := json.Unmarshal([]byte(raw), &decision); e != nil {
			lastErr = fmt.Errorf("解析AI返回失败: %w", e)
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, raw, lastErr
	}

	finalizeDecision(&decision, f, pricePrecision(snap), a.calib)
	return &decision, raw, nil
}

// pricePrecision 从主周期最近一根收盘价字符串推断报价小数位（交易所原生精度），
// 用于把换算出的价格取整到可成交精度；无法推断时返回 -1（不取整）。
func pricePrecision(snap *collector.Snapshot) int {
	if snap == nil || len(snap.Primary) == 0 {
		return -1
	}
	return decimalsOf(snap.Primary[len(snap.Primary)-1].ClosePrice)
}

func decimalsOf(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return len(s) - i - 1
	}
	return 0
}

func roundTo(v float64, decimals int) float64 {
	if decimals < 0 {
		return v
	}
	p := math.Pow(10, float64(decimals))
	return math.Round(v*p) / p
}

// finalizeDecision 归一化方向/信号、标定置信度，并对幅度做合理性钳制后换算绝对价。
// 这是 oracle 侧唯一的本地校验关口：模型给出的离谱幅度会被夹回区间，避免脏数据落库。
func finalizeDecision(d *Decision, f indicator.Features, pricePrec int, calib calibration.Calibrator) {
	d.Trend = normalizeTrend(d.Trend)
	d.Signal = normalizeSignal(d.Signal)
	d.Confidence = clamp01(d.Confidence)

	// 校准回环反馈：把离线打分学到的修正(区间缩放 / 置信度平移)作用回本次预测，
	// 再走下方的一致性与幅度钳制重新收敛到合法区间。calib 默认 Noop 时此处恒等、不改任何字段。
	if calib != nil {
		adj := calib.Adjust(calibration.Prediction{
			MovePct:    d.ExpectedMovePct,
			HighPct:    d.ExpectedHighPct,
			LowPct:     d.ExpectedLowPct,
			Confidence: d.Confidence,
		})
		d.ExpectedMovePct = adj.MovePct
		d.ExpectedHighPct = adj.HighPct
		d.ExpectedLowPct = adj.LowPct
		d.Confidence = clamp01(adj.Confidence)
	}

	// 方向与幅度符号一致性校验：trend 与 expected_move_pct 自相矛盾时（如 long 配负幅度），
	// 视为模型不可信，保守归为 neutral/hold 并压低置信度。
	if (d.Trend == "long" && d.ExpectedMovePct < 0) || (d.Trend == "short" && d.ExpectedMovePct > 0) {
		d.Trend = "neutral"
		d.Signal = "hold"
		d.ExpectedMovePct = 0
		d.Confidence = math.Min(d.Confidence, 0.3)
	}
	if d.Trend == "neutral" {
		d.ExpectedMovePct = 0
	}

	ref := f.LastClose
	if ref <= 0 {
		d.PredictPrice = ref
		return
	}

	// 幅度上限：max(5%, 5×ATR%)。超出视为不可信，夹回上限并压低置信度。
	maxMovePct := 5.0
	if f.ATR14.Valid {
		if atrPct := f.ATR14.Value / ref * 100; 5*atrPct > maxMovePct {
			maxMovePct = 5 * atrPct
		}
	}
	if d.ExpectedMovePct > maxMovePct {
		d.ExpectedMovePct = maxMovePct
		d.Confidence = math.Min(d.Confidence, 0.4)
	} else if d.ExpectedMovePct < -maxMovePct {
		d.ExpectedMovePct = -maxMovePct
		d.Confidence = math.Min(d.Confidence, 0.4)
	}

	d.PredictPrice = roundTo(ref*(1+d.ExpectedMovePct/100), pricePrec)

	// 波动区间钳制：high ≥ 0 ≥ low，且须覆盖 expected_move_pct；同样受 maxMovePct 上限约束。
	if d.ExpectedHighPct < 0 {
		d.ExpectedHighPct = 0
	}
	if d.ExpectedLowPct > 0 {
		d.ExpectedLowPct = 0
	}
	if d.ExpectedHighPct < d.ExpectedMovePct {
		d.ExpectedHighPct = math.Max(d.ExpectedMovePct, 0)
	}
	if d.ExpectedLowPct > d.ExpectedMovePct {
		d.ExpectedLowPct = math.Min(d.ExpectedMovePct, 0)
	}
	if d.ExpectedHighPct > maxMovePct {
		d.ExpectedHighPct = maxMovePct
	}
	if d.ExpectedLowPct < -maxMovePct {
		d.ExpectedLowPct = -maxMovePct
	}
	d.PredictHigh = roundTo(ref*(1+d.ExpectedHighPct/100), pricePrec)
	d.PredictLow = roundTo(ref*(1+d.ExpectedLowPct/100), pricePrec)

	sl, tp := deriveSLTP(*d, ref)
	d.StopLoss = roundTo(sl, pricePrec)
	d.TakeProfit = roundTo(tp, pricePrec)

	d.Invalidation = roundTo(deriveInvalidation(*d, ref), pricePrec)

	// 形态(趋势效率)：中枢幅度占区间宽度的比例，由已钳制的区间派生。
	// 下游策略层据此在「趋势顺势」与「震荡区间回踩」两种入场方式间分流。
	if width := d.ExpectedHighPct - d.ExpectedLowPct; width > 0 {
		d.Efficiency = math.Abs(d.ExpectedMovePct) / width
	} else {
		d.Efficiency = 0
	}
}

// deriveInvalidation 把失效距离(百分比)按方向换算成绝对价：
// long 失效位在下方(支撑跌破)，short 失效位在上方(阻力突破)；neutral 或未给则为 0。
func deriveInvalidation(d Decision, ref float64) float64 {
	if ref <= 0 || d.InvalidationPct <= 0 {
		return 0
	}
	switch d.Trend {
	case "long":
		return ref * (1 - d.InvalidationPct/100)
	case "short":
		return ref * (1 + d.InvalidationPct/100)
	default:
		return 0
	}
}

// deriveSLTP 把百分比距离按方向换算成绝对止损/止盈价；neutral/未给百分比时返回 0。
func deriveSLTP(d Decision, ref float64) (sl, tp float64) {
	if ref <= 0 {
		return 0, 0
	}
	switch d.Trend {
	case "long":
		if d.StopLossPct > 0 {
			sl = ref * (1 - d.StopLossPct/100)
		}
		if d.TakeProfitPct > 0 {
			tp = ref * (1 + d.TakeProfitPct/100)
		}
	case "short":
		if d.StopLossPct > 0 {
			sl = ref * (1 + d.StopLossPct/100)
		}
		if d.TakeProfitPct > 0 {
			tp = ref * (1 - d.TakeProfitPct/100)
		}
	}
	return sl, tp
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

// ToSaveDTO 把预测映射成 service 落库 DTO。
// 滚动预测语义：predictTime = 执行时刻 + 预测间隔(snap.Interval)。
// 例如执行于 00:04、间隔 5m，则预测的是 00:09 这一时刻的价格；落库行的创建时间(CreatedTime)即为本次执行时刻。
// snap.Interval 在此表示「预测时间间隔(horizon)」，而非 K 线展示周期。
// openPrice 为 AI 分析完成后即时采集的实际盘价(实际开盘价)，costMs 为本次 AI 分析耗时(毫秒)。
func ToSaveDTO(snap *collector.Snapshot, f indicator.Features, d *Decision, provider, model, newsSummary, pressureSummary string, openPrice float64, costMs int64) tradeDTO.TradeAIPredictionSaveDTO {
	return ToSaveDTOAt(snap, f, d, provider, model, newsSummary, pressureSummary, openPrice, costMs, time.Now())
}

// ToSaveDTOAt 与 ToSaveDTO 相同，但以 baseTime 作为执行时刻锚点：predictTime = baseTime + 预测间隔。
// 实时预测传 time.Now()(即 ToSaveDTO)；历史回填传当时的发起时刻 T，使补跑的长周期预测带上历史 predict_time。
func ToSaveDTOAt(snap *collector.Snapshot, f indicator.Features, d *Decision, provider, model, newsSummary, pressureSummary string, openPrice float64, costMs int64, baseTime time.Time) tradeDTO.TradeAIPredictionSaveDTO {
	predictTime := baseTime.Add(IntervalDuration(snap.Interval)).Unix()
	rawResp, _ := json.Marshal(map[string]any{
		"decision":       d,
		"highTimeframes": keysOf(snap.HighTF),
		"features":       f,
		"news":           newsSummary,
		"pressure":       pressureSummary,
	})
	return tradeDTO.TradeAIPredictionSaveDTO{
		PlatformCode: snap.Platform,
		Symbol:       snap.Symbol,
		CoinCode:     snap.CoinCode,
		Interval:     snap.Interval,
		PredictTime:  predictTime,
		RefPrice:     f.LastClose,
		OpenPrice:    openPrice,
		CostMs:       costMs,
		PredictPrice: d.PredictPrice,
		PredictHigh:  d.PredictHigh,
		PredictLow:   d.PredictLow,
		Invalidation: d.Invalidation,
		Trend:        d.Trend,
		Signal:       d.Signal,
		Confidence:   d.Confidence,
		StopLoss:     d.StopLoss,
		TakeProfit:   d.TakeProfit,
		Reason:       d.Reason,
		RawResponse:  string(rawResp),
		Model:        model,
		Provider:     provider,
	}
}

// IntervalDuration 把预测间隔字符串(1m/5m/15m/1h/4h/1d 等)转成时间间隔，无法识别时回退到 5 分钟。
func IntervalDuration(interval string) time.Duration {
	switch strings.TrimSpace(interval) {
	case "1m":
		return time.Minute
	case "3m":
		return 3 * time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	case "1w":
		return 7 * 24 * time.Hour
	default:
		return 5 * time.Minute
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
		return "", fmt.Errorf("AI 请求失败 status=%d body=%s", resp.StatusCode, truncate(string(body), 512))
	}
	logrus.Debugf("[oracle] AI 原始返回: %s", truncate(string(body), 512))
	return parseChatContent(body)
}

// parseChatContent 从 OpenAI 兼容返回中取出 message.content。
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
		return "", fmt.Errorf("AI 返回缺少 choices")
	}
	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	// 去掉可能的 ```json 包裹
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content), nil
}

func normalizeTrend(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "long", "bull", "bullish", "up":
		return "long"
	case "short", "bear", "bearish", "down":
		return "short"
	default:
		return "neutral"
	}
}

func normalizeSignal(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "buy", "long", "open_long":
		return "buy"
	case "sell", "short", "open_short":
		return "sell"
	default:
		return "hold"
	}
}

func keysOf(m map[string][]argusTrade.MarketKline) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
