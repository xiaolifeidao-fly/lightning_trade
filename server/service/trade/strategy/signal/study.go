package signal

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// 本文件是后台自动参数寻优（r16）的方法学内核：**降噪协议**。
//
// 为什么需要它：单路径混沌达 ±70U（设计文档 §8.3），点估计不可信。实测反例——
// net(15,400) 的 16 路径中位是 +79.9U，看着不错，但符号一致率只有 0.69；现行
// champion net(15,300) 的一致率是 0.56，等于掷硬币。**按单次回放的 PnL 排序，
// 排在前面的会是纯噪声格子。** 所以一格的产出不是"一个 PnL"，而是一束路径上的
// 中位数 + IQR + 符号一致率 + 情景分位。
//
// 协议本体（设计文档 §3.2，脚本原型 docs/argus_single/backtest_capsf_study.py）：
//
//	每格 N 条路径 = {bar 模式 close / 悲观} × {起点偏移 0/1/2/3 天}
//	              × {信号 5% 随机丢弃：不丢 / 丢(种子1)}   = 2×4×2 = 16
//
// 三个抖动轴各自打掉一类脆弱性：bar 模式打掉"出场判定粒度"的乐观偏差，起点偏移
// 打掉"恰好从某个有利时点起步"，信号丢弃打掉"结论依赖某几次特定触发"。
//
// 本文件是纯计算，不碰 IO、不碰 DB：输入是一份已取好的信号流 + 1m 路径，
// 输出是一格的统计量。编排在 service/trade/signal_optimize.go。
type Protocol struct {
	// EvalModes bar 内评估口径的抖动轴，缺省 [close, pessimistic]。
	EvalModes []string `json:"evalModes"`
	// OffsetDays 起点偏移天数的抖动轴，缺省 [0,1,2,3]。
	OffsetDays []int `json:"offsetDays"`
	// DropSeeds 信号随机丢弃的种子轴，缺省 [null, 1]：null = 不丢弃。
	DropSeeds []*int64 `json:"dropSeeds"`
	// DropRate 丢弃比例，缺省 0.05。
	DropRate float64 `json:"dropRate"`

	// NormalizeDays 收益归一到多少天，缺省 28（金标准口径："中位 PnL/28天"）。
	NormalizeDays float64 `json:"normalizeDays"`
	// ScenarioDays 情景月长度，缺省 30 天。
	ScenarioDays int `json:"scenarioDays"`
	// BootstrapMode day_iid（金标准，可与 study_fine.csv 比对）/ episode_block（设计原意）。
	BootstrapMode string `json:"bootstrapMode"`
	// BootstrapDraws 重采样次数，缺省 2000（金标准口径）。
	BootstrapDraws int `json:"bootstrapDraws"`
	// BootstrapSeed 重采样种子，缺省 42（金标准口径）。固定种子让同一份数据
	// 每次跑出同一个分位——分位本身是蒙特卡洛估计，不固定种子会让"三关判定"
	// 每次刷新都可能翻面。
	BootstrapSeed int64 `json:"bootstrapSeed"`

	// LambdaDenom λ_bear 的分母口径：
	//   window = 整个扫描窗口的单边日数（金标准脚本口径，见 CellStats.Notes 的偏差说明）
	//   path   = 该路径自己覆盖到的单边日数（无偏，但与 study_fine.csv 不可比）
	LambdaDenom string `json:"lambdaDenom"`
	// LambdaMonthDays λ 折算到多少天算"一个月"，缺省 30。
	LambdaMonthDays float64 `json:"lambdaMonthDays"`

	// Regime 日状态判据。
	Regime RegimeThresholds `json:"regime"`
}

// λ_bear 分母口径。
const (
	LambdaDenomWindow = "window"
	LambdaDenomPath   = "path"
)

// FineProtocol 精算协议：16 条路径。决策只看它。
func FineProtocol() Protocol {
	one := int64(1)
	return Protocol{
		EvalModes:       []string{EvalClose, EvalPessimistic},
		OffsetDays:      []int{0, 1, 2, 3},
		DropSeeds:       []*int64{nil, &one},
		DropRate:        0.05,
		NormalizeDays:   28,
		ScenarioDays:    30,
		BootstrapMode:   BootstrapDayIID,
		BootstrapDraws:  2000,
		BootstrapSeed:   42,
		LambdaDenom:     LambdaDenomWindow,
		LambdaMonthDays: 30,
		Regime:          DefaultRegimeThresholds(),
	}
}

// CoarseProtocol 粗网格协议：4 条路径（close/悲观 × 偏移 0/2，不丢信号）。
//
// 它**只用于筛掉明显没戏的格子**，不作任何判定：4 条路径的符号一致率只有
// 0/0.25/0.5/0.75/1 五档，分辨不出 0.69 与 0.80 的差别，而那正是决策线所在。
func CoarseProtocol() Protocol {
	p := FineProtocol()
	p.EvalModes = []string{EvalClose, EvalPessimistic}
	p.OffsetDays = []int{0, 2}
	p.DropSeeds = []*int64{nil}
	return p
}

// Normalize 补齐零值。刻意不做"看起来合理"的纠正（比如把 draws 拉到 10000）：
// 协议是随任务冻结落库的，改缺省会让历史扫描无法复现。
func (p Protocol) Normalize() Protocol {
	d := FineProtocol()
	if len(p.EvalModes) == 0 {
		p.EvalModes = d.EvalModes
	}
	if len(p.OffsetDays) == 0 {
		p.OffsetDays = d.OffsetDays
	}
	if len(p.DropSeeds) == 0 {
		p.DropSeeds = d.DropSeeds
	}
	if p.DropRate <= 0 {
		p.DropRate = d.DropRate
	}
	if p.NormalizeDays <= 0 {
		p.NormalizeDays = d.NormalizeDays
	}
	if p.ScenarioDays <= 0 {
		p.ScenarioDays = d.ScenarioDays
	}
	if p.BootstrapMode != BootstrapEpisodeBlock {
		p.BootstrapMode = BootstrapDayIID
	}
	if p.BootstrapDraws <= 0 {
		p.BootstrapDraws = d.BootstrapDraws
	}
	if p.BootstrapSeed == 0 {
		p.BootstrapSeed = d.BootstrapSeed
	}
	if p.LambdaDenom != LambdaDenomPath {
		p.LambdaDenom = LambdaDenomWindow
	}
	if p.LambdaMonthDays <= 0 {
		p.LambdaMonthDays = d.LambdaMonthDays
	}
	p.Regime = p.Regime.normalize()
	return p
}

// PathSpec 一条抖动路径。
type PathSpec struct {
	Index      int     `json:"index"`
	EvalMode   string  `json:"evalMode"`
	OffsetDays int     `json:"offsetDays"`
	DropSeed   *int64  `json:"dropSeed"` // nil = 不丢弃信号
	DropRate   float64 `json:"dropRate"`
}

// Label 路径的可读标签，落进结果供人核对"这 16 条到底是哪 16 条"。
func (s PathSpec) Label() string {
	drop := "nodrop"
	if s.DropSeed != nil {
		drop = fmt.Sprintf("drop%.0f%%s%d", s.DropRate*100, *s.DropSeed)
	}
	return fmt.Sprintf("%s/off%dd/%s", s.EvalMode, s.OffsetDays, drop)
}

// Paths 展开全部抖动路径。**顺序固定**为 evalMode → offset → dropSeed，
// 与金标准脚本的 FINE_PATHS 逐项一致：bootstrap 的重采样池按路径顺序拼接，
// 顺序一变分位就变，校准就对不上了。
func (p Protocol) Paths() []PathSpec {
	p = p.Normalize()
	out := make([]PathSpec, 0, len(p.EvalModes)*len(p.OffsetDays)*len(p.DropSeeds))
	for _, mode := range p.EvalModes {
		for _, off := range p.OffsetDays {
			for _, seed := range p.DropSeeds {
				out = append(out, PathSpec{
					Index: len(out), EvalMode: mode, OffsetDays: off, DropSeed: seed, DropRate: p.DropRate,
				})
			}
		}
	}
	return out
}

// StopEvent 一次兜底止损。单次损失额的分布是频率预算约束的另一半
// （预算 = λ × E[单次损失]），所以逐次记下来而不是只记个数。
type StopEvent struct {
	At  time.Time `json:"at"`
	Day string    `json:"day"`
	Pnl float64   `json:"pnl"` // 负值，口径同金标准：本次平仓的毛盈亏，不含手续费
}

// PathOutcome 一条路径的产出。
type PathOutcome struct {
	Spec      PathSpec `json:"spec"`
	Pnl28     float64  `json:"pnl28"` // 归一到 NormalizeDays 天的净利
	Net       float64  `json:"net"`
	Fees      float64  `json:"fees"`
	MaxDD     float64  `json:"maxDd"`
	Days      float64  `json:"days"`
	BarCount  int      `json:"barCount"`
	MaxStack  int      `json:"maxStack"`
	SignalIn  int      `json:"signalIn"`  // 本路径喂进引擎的触发数（偏移+丢弃之后）
	SignalRun int      `json:"signalRun"` // 引擎实际回放的触发数
	TradeCnt  int      `json:"tradeCount"`

	Stops    []StopEvent `json:"stops"`
	DailyMTM []DayMTM    `json:"-"`
	Blocks   []MtmBlock  `json:"-"`
}

// RunPath 跑一条抖动路径。
//
// windowStart 是**扫描窗口的起点**，不是第一根 K 线的时刻：起点偏移按窗口起点
// 加天数算，这样同一批格子的第 k 条路径看的是同一段时间。
func RunPath(base Input, spec PathSpec, windowStart time.Time, labels map[string]string, proto Protocol) (*PathOutcome, error) {
	proto = proto.Normalize()
	p := base.Params.Normalize()
	p.EvalMode = spec.EvalMode

	start := windowStart.AddDate(0, 0, spec.OffsetDays)
	signals := make([]Signal, 0, len(base.Signals))
	for _, s := range SortSignals(base.Signals) {
		if s.Ts.Before(start) {
			continue
		}
		signals = append(signals, s)
	}
	if spec.DropSeed != nil && spec.DropRate > 0 {
		rng := newPyRandom(*spec.DropSeed)
		kept := signals[:0:0]
		for _, s := range signals {
			// 口径同脚本：先抽随机数、再判 >= dropRate，逐条消费 RNG 流。
			if rng.Float64() >= spec.DropRate {
				kept = append(kept, s)
			}
		}
		signals = kept
	}
	bars := make([]Bar, 0, len(base.Bars))
	for _, b := range SortBars(base.Bars) {
		if b.Ts.Before(start) {
			continue
		}
		bars = append(bars, b)
	}
	if len(bars) == 0 {
		return nil, fmt.Errorf("路径 %s 偏移后没有任何 1m K 线：窗口太短或偏移过大", spec.Label())
	}

	in := Input{Params: p, Signals: signals, Bars: bars, DevWindows: base.DevWindows}
	// 种子仓只在偏移 0 的路径上灌：它是"窗口起点那一刻的旧仓"，把它原样搬到
	// 偏移三天之后，等于凭空造出一个当时可能早已平掉的仓位。
	if spec.OffsetDays == 0 {
		in.Seed = base.Seed
	}
	res, err := Replay(in)
	if err != nil {
		return nil, fmt.Errorf("路径 %s 回放失败: %w", spec.Label(), err)
	}

	days := float64(res.BarCount) / 1440.0
	out := &PathOutcome{
		Spec: spec, Net: res.Net, Fees: res.Fees, MaxDD: res.MaxDrawdown,
		Days: days, BarCount: res.BarCount, MaxStack: res.MaxStack,
		SignalIn: len(signals), SignalRun: res.SignalReplayed, TradeCnt: len(res.Episodes),
		DailyMTM: dailyMTM(res.Equity),
	}
	if days > 0 {
		out.Pnl28 = res.Net * proto.NormalizeDays / days
	}
	for _, ep := range res.Episodes {
		if ep.Reason != ExitCatastrophe {
			continue
		}
		out.Stops = append(out.Stops, StopEvent{At: ep.ClosedAt, Day: ep.ClosedAt.Format("2006-01-02"), Pnl: ep.Pnl})
	}
	out.Blocks = BuildMtmBlocks(out.DailyMTM, res.Episodes, labels, bars[0].Ts.Location())
	return out, nil
}

// CellSpec 搜索空间里的一格。trail 档位固定为 champion 值（防组合爆炸，
// 设计文档 §3.3），所以一格只由这四个旋钮定义。
type CellSpec struct {
	Mode    string  `json:"mode"`    // net / dual
	Cap     int     `json:"cap"`     // 净仓上限；dual 下是每侧上限
	StopPct float64 `json:"stopPct"` // S = position.monitor.catastrophe_stop_pct
	GatePct float64 `json:"gatePct"` // trade.accountN.reverse_gate_min_profit_pct
}

// Key 格子的稳定标识，同时用作展示标签。
func (c CellSpec) Key() string {
	return fmt.Sprintf("%s(%d,%.0f,g%.0f)", c.Mode, c.Cap, c.StopPct, c.GatePct)
}

// Params 把一格叠加到基线参数上。只动这四个旋钮，其余（trail 档位、费率、
// 过冲惩罚、order_size、risk_equity）沿用基线——那是"一格只代表这四个旋钮的
// 效应"的前提。
func (c CellSpec) Params(base Params) Params {
	p := base
	if c.Mode == ModeDual {
		p.Mode = ModeDual
	} else {
		p.Mode = ModeNet
	}
	p.CapOverride = c.Cap
	p.CatastropheStopPct = c.StopPct
	p.GateMinProfitPct = c.GatePct
	return p.Normalize()
}

// CellStats 一格的降噪产出。**没有任何一个字段是单路径点估计**。
type CellStats struct {
	Spec      CellSpec `json:"spec"`
	Key       string   `json:"key"`
	PathCount int      `json:"pathCount"`

	MedPnl28  float64 `json:"medPnl28"`
	P25Pnl28  float64 `json:"p25Pnl28"`
	P75Pnl28  float64 `json:"p75Pnl28"`
	IQRPnl28  float64 `json:"iqrPnl28"`
	MinPnl28  float64 `json:"minPnl28"`
	MaxPnl28  float64 `json:"maxPnl28"`
	SignRatio float64 `json:"signRatio"` // 符号一致率：pnl28 > 0 的路径占比

	LambdaBearPerMonth float64 `json:"lambdaBearPerMonth"` // 单边日口径的兜底频率，次/月
	MeanStopLoss       float64 `json:"meanStopLoss"`       // 单次兜底平均损失额（正数）
	StopBudget         float64 `json:"stopBudget"`         // λ_bear × 单次损失 = 月度兜底预算消耗
	StopCount          int     `json:"stopCount"`          // 全路径兜底总次数

	P90MaxDrawdown float64 `json:"p90MaxDrawdown"`
	MaxStack       int     `json:"maxStack"`
	MedFee         float64 `json:"medFee"`
	MedDays        float64 `json:"medDays"`
	MedSignalRun   int     `json:"medSignalRun"`
	Fidelity       string  `json:"fidelity"`

	Scenarios []ScenarioResult `json:"scenarios"`
	Paths     []PathOutcome    `json:"paths"`
	// Notes 本格必须随结果展示的口径说明与已知偏差。
	Notes []string `json:"notes"`
}

// Scenario 取某个情景的分位，取不到返回 nil。
func (c *CellStats) Scenario(name string) *ScenarioResult {
	for i := range c.Scenarios {
		if c.Scenarios[i].Scenario == name {
			return &c.Scenarios[i]
		}
	}
	return nil
}

// AggregateCell 把一格的 N 条路径聚合成统计量。
//
// labels 是整个窗口的日状态标签（全格共用），trendDayCount 是窗口内的单边日数
// （λ_bear 的 window 口径分母）。
func AggregateCell(spec CellSpec, outcomes []PathOutcome, labels map[string]string, proto Protocol) CellStats {
	proto = proto.Normalize()
	st := CellStats{Spec: spec, Key: spec.Key(), PathCount: len(outcomes),
		Paths: outcomes, Fidelity: FidelityEvent}
	if len(outcomes) == 0 {
		st.Notes = append(st.Notes, "本格没有任何有效路径，全部统计量无意义")
		return st
	}

	pnls := make([]float64, 0, len(outcomes))
	dds := make([]float64, 0, len(outcomes))
	fees := make([]float64, 0, len(outcomes))
	daysArr := make([]float64, 0, len(outcomes))
	runs := make([]float64, 0, len(outcomes))
	positive := 0
	for _, o := range outcomes {
		pnls = append(pnls, o.Pnl28)
		dds = append(dds, o.MaxDD)
		fees = append(fees, o.Fees)
		daysArr = append(daysArr, o.Days)
		runs = append(runs, float64(o.SignalRun))
		if o.Pnl28 > 0 {
			positive++
		}
		if o.MaxStack > st.MaxStack {
			st.MaxStack = o.MaxStack
		}
	}
	st.MedPnl28 = medianOf(pnls)
	st.P25Pnl28 = quantileByIndex(pnls, 0.25)
	st.P75Pnl28 = quantileByIndex(pnls, 0.75)
	st.IQRPnl28 = st.P75Pnl28 - st.P25Pnl28
	st.MinPnl28, st.MaxPnl28 = minMax(pnls)
	st.SignRatio = float64(positive) / float64(len(outcomes))
	st.P90MaxDrawdown = quantileByIndex(dds, 0.9)
	st.MedFee = medianOf(fees)
	st.MedDays = medianOf(daysArr)
	st.MedSignalRun = int(math.Round(medianOf(runs)))

	// 单次兜底损失：全路径合池取均值（口径同金标准脚本的 all_cats）。
	lossSum, lossN := 0.0, 0
	for _, o := range outcomes {
		for _, s := range o.Stops {
			lossSum += s.Pnl
			lossN++
		}
	}
	st.StopCount = lossN
	if lossN > 0 {
		st.MeanStopLoss = math.Abs(lossSum / float64(lossN))
	}

	// λ_bear：逐路径算"单边日上的兜底次数 / 单边日数 × 30"，再取中位数。
	trendDaysWindow := CountLabel(labels, RegimeTrend)
	lams := make([]float64, 0, len(outcomes))
	for _, o := range outcomes {
		denom := trendDaysWindow
		if proto.LambdaDenom == LambdaDenomPath {
			denom = countPathTrendDays(o, labels)
		}
		if denom < 1 {
			denom = 1
		}
		hits := 0
		for _, s := range o.Stops {
			if labels[s.Day] == RegimeTrend {
				hits++
			}
		}
		lams = append(lams, float64(hits)/float64(denom)*proto.LambdaMonthDays)
	}
	st.LambdaBearPerMonth = medianOf(lams)
	st.StopBudget = st.LambdaBearPerMonth * st.MeanStopLoss

	st.Scenarios = buildScenarios(outcomes, labels, proto)
	st.Notes = append(st.Notes, protocolNotes(proto, trendDaysWindow, len(outcomes))...)
	return st
}

// countPathTrendDays 该路径自己覆盖到的单边日数。
func countPathTrendDays(o PathOutcome, labels map[string]string) int {
	n := 0
	for _, d := range o.DailyMTM {
		if labels[d.Day] == RegimeTrend {
			n++
		}
	}
	return n
}

// buildScenarios 三种情景的月度净利分布。
//
// day_iid 口径下重采样池是**全路径的日 ΔMTM 按路径序拼接**（金标准口径）；
// episode_block 口径下是全路径的日块按路径序拼接。每个情景各起一条新的
// RNG 流（种子相同），这样"熊市月"的抽样序列与只算熊市月时逐字一致。
func buildScenarios(outcomes []PathOutcome, labels map[string]string, proto Protocol) []ScenarioResult {
	names := []string{ScenarioBear, ScenarioChop, ScenarioMixed}
	out := make([]ScenarioResult, 0, len(names))
	for _, name := range names {
		r := ScenarioResult{Scenario: name, Mode: proto.BootstrapMode, Draws: proto.BootstrapDraws}
		if proto.BootstrapMode == BootstrapEpisodeBlock {
			pool := make([]MtmBlock, 0, 64)
			for _, o := range outcomes {
				for _, b := range o.Blocks {
					if blockInScenario(b, name) {
						pool = append(pool, b)
					}
				}
			}
			r.PoolSize = len(pool)
			if p10, p50, p90, ok := bootstrapBlocks(pool, proto.ScenarioDays, proto.BootstrapDraws, proto.BootstrapSeed); ok {
				r.P10, r.P50, r.P90 = p10, p50, p90
			} else {
				r.Note = "该情景没有任何不切断持仓的日块可供重采样：窗口内缺少这类行情，分位留空而不是当 0 用"
			}
			out = append(out, r)
			continue
		}
		pool := make([]float64, 0, 256)
		for _, o := range outcomes {
			for _, d := range o.DailyMTM {
				if dayInScenario(labels[d.Day], name) {
					pool = append(pool, d.Delta)
				}
			}
		}
		r.PoolSize = len(pool)
		if p10, p50, p90, ok := bootstrapDayIID(pool, proto.ScenarioDays, proto.BootstrapDraws, proto.BootstrapSeed); ok {
			r.P10, r.P50, r.P90 = p10, p50, p90
		} else {
			r.Note = "该情景在窗口内没有对应状态的自然日：分位留空而不是当 0 用"
		}
		out = append(out, r)
	}
	return out
}

func dayInScenario(label, scenario string) bool {
	switch scenario {
	case ScenarioBear:
		return label == RegimeTrend
	case ScenarioChop:
		return label == RegimeVol
	default:
		return true
	}
}

func blockInScenario(b MtmBlock, scenario string) bool {
	switch scenario {
	case ScenarioBear:
		return b.Label == RegimeTrend
	case ScenarioChop:
		return b.Label == RegimeVol
	default:
		return true
	}
}

// protocolNotes 必须随每一格结果展示的口径说明。写在结果里而不是文档里，
// 是因为读结果的人不一定读过设计文档，而这几条直接决定数字怎么解释。
func protocolNotes(proto Protocol, trendDays, pathCount int) []string {
	notes := []string{
		fmt.Sprintf("降噪协议：每格 %d 条路径（bar 模式 %v × 起点偏移 %v 天 × 信号丢弃 %.0f%% 种子 %s），报告中位数 + IQR + 符号一致率，不报单路径点估计",
			pathCount, proto.EvalModes, proto.OffsetDays, proto.DropRate*100, dropSeedLabel(proto.DropSeeds)),
		fmt.Sprintf("「熊市月」= 单边日（|日收益| ≥ %.1f%%）情景，不分涨跌：均值回归累加器怕的是单边行情本身（8/21 剧烈上涨同样打爆它），不是下跌",
			proto.Regime.TrendAbsRetPct),
		fmt.Sprintf("窗口内单边日共 %d 天，λ_bear 与熊市月分位都建立在这 %d 天上，样本很薄，结论按方向读、不按数值读", trendDays, trendDays),
		fmt.Sprintf("情景分位是 %d 次重采样的蒙特卡洛估计（种子 %d 固定），不是解析分位", proto.BootstrapDraws, proto.BootstrapSeed),
	}
	switch proto.BootstrapMode {
	case BootstrapEpisodeBlock:
		notes = append(notes, "情景 bootstrap 口径：episode_block（按持仓生命周期切块，不切断扛单）。它比 day_iid 更贴近设计原意，但与 study_fine.csv 的历史数值不可直接比对")
	default:
		notes = append(notes, "情景 bootstrap 口径：day_iid（金标准 backtest_capsf_study.py 口径，可与 study_fine.csv 比对）。已知偏差：把稀有的兜底损失日按 iid 重抽会高估尾部，对低 λ 配置系统性偏悲观（设计文档 §10.3）")
	}
	if proto.LambdaDenom == LambdaDenomWindow {
		notes = append(notes, "λ_bear 分母用整窗口单边日数（金标准口径）。已知偏差：起点偏移路径只覆盖窗口的一部分，分子少了分母没少，λ 偏低——同一口径下各格可比，跨口径不可比")
	} else {
		notes = append(notes, "λ_bear 分母用各路径自己覆盖到的单边日数（无偏口径），与 study_fine.csv 的历史数值不可直接比对")
	}
	return notes
}

func dropSeedLabel(seeds []*int64) string {
	out := ""
	for i, s := range seeds {
		if i > 0 {
			out += "/"
		}
		if s == nil {
			out += "不丢"
		} else {
			out += fmt.Sprintf("%d", *s)
		}
	}
	return out
}

// medianOf 口径同 Python statistics.median：偶数个取中间两个的平均。
func medianOf(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// quantileByIndex 口径同金标准脚本的 p90 取法：排序后取 min(n-1, int(n*q)) 位。
// 刻意不用插值分位——16 条路径下插值只会制造一个不存在的精度感，而金标准的
// p90DD 正是按这个取法算的，换算法就与 study_fine.csv 对不上。
func quantileByIndex(xs []float64, q float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	idx := int(float64(len(s)) * q)
	if idx > len(s)-1 {
		idx = len(s) - 1
	}
	if idx < 0 {
		idx = 0
	}
	return s[idx]
}

func minMax(xs []float64) (float64, float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	lo, hi := xs[0], xs[0]
	for _, v := range xs {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return lo, hi
}
