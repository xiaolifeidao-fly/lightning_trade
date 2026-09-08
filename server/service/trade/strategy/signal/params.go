// Package signal 是盘口信号回测引擎：用 strategy_event 里的真实触发点驱动，
// 而不是喂 AI 预测（那条路是 ../engine.go，本包一行不动它）。
//
// 三条硬约束（改代码前先读）：
//
//  1. **信号侧零推导**。触发时刻、方向、gapBp、拦截原因全部来自 strategy_event
//     按 instance_key + account 过滤后的事件流，不从 K 线推。K 线里没有 mark
//     price，而信号源恰是 DeepCoin last 相对 mark 的偏离；且实测 28.7% 的信号在
//     同一分钟内触发 ≥2 次、承载 48.6% 的全部触发（单分钟最多 5 次），按 1m 推
//     导会丢掉约一半触发。1m K 线在本包里**只**用于持仓期间的路径回放。
//
//  2. **判定逻辑复用实盘函数**。移动止盈/兜底止损走 monitor.EvaluateExit，
//     反向门控走 trade.EvaluateReverseGate，趋势闸走 trade.EvaluateTrendGate，
//     仓位上限走 trade.ComputeMaxContracts，参数合法性走 trade.ValidateRiskParams。
//     回测与实盘共享同一份判定，不允许在本包里另写一套——那是"回测有效、实盘
//     不一致"的唯一来源。
//
//  3. **精度分层必须显式**。事件级（触发点精确到秒）与频率级（只能推 λ(θ)、
//     推不出每次时刻）的结果不得混排比较，见 fidelity.go。
//
// 金标准：docs/argus_single/backtest_dual_side.py。本包的 close/悲观两套 bar
// 内评估口径与它逐项对齐，parity_test.go 用仓库内真实数据钉住数值。
package signal

import (
	argusTrade "argus_single/pkg/trade"
)

// 持仓形态。实盘是净仓 + 反向门控（ModeNet）；ModeDual 是研究对照口径
// （双向各自累积、无门控），金标准脚本的 S1a/S1b 用它。
const (
	ModeNet  = "net"
	ModeDual = "dual"
)

// bar 内评估口径。实盘判定频率是 position.monitor.interval_seconds=5 秒，
// 回测只有 1m K 线，因此一根之内怎么取价是个显式选择，不能默认。
const (
	// EvalClose 用收盘价评估移动止盈：乐观（一根内的极值不参与判定）。
	EvalClose = "close"
	// EvalPessimistic 一根内先按不利极值结算移动止盈、兜底按极值成交：悲观。
	// 同一根内可能同时触及止盈与止损时按不利方向结算，宁可低估。
	EvalPessimistic = "pessimistic"
)

// 信号成交价口径。
const (
	// EntryBarClose 用信号落在的那根 1m 收盘价成交（金标准脚本口径）。
	EntryBarClose = "bar_close"
	// EntrySigLast 用事件自带的 DeepCoin last（触发瞬间真价）成交。
	// 更贴近触发时刻，但与实盘的 trade.signal.delay_seconds=5 秒延迟不同源。
	EntrySigLast = "sig_last"
)

// 默认值，全部取自 argus_single 的生产缺省（见 pkg/trade/account_params.go
// resolveCapParams 与 pkg/monitor 的 trail 缺省），不另立一套。
const (
	DefaultLeverage           = argusTrade.SignalLeverage // 125
	DefaultFaceValue          = 0.001                     // BTC 1 张 = 0.001
	DefaultTakerFee           = 0.0006                    // 0.06%/边，金标准 backtest_dual_side.py 口径
	DefaultBudgetPct          = 20.0
	DefaultCatastropheStopPct = 300.0
	DefaultCeiling            = 20
	DefaultGateMinProfitPct   = 20.0
	DefaultTierSmallRatio     = 0.30
	DefaultTierLargeRatio     = 0.65
	DefaultSmallActivatePct   = 150.0
	DefaultSmallGiveback      = 0.35
	DefaultMediumActivatePct  = 90.0
	DefaultMediumGiveback     = 0.28
	DefaultLargeActivatePct   = 40.0
	DefaultLargeGiveback      = 0.20

	// LiveMonitorIntervalSeconds 实盘平仓判定轮询周期；回测是 1m K 线 = 60 秒。
	// 两者不等是 peakPct 系统性偏高的根因，写进 fidelity 注记而不是悄悄忽略。
	LiveMonitorIntervalSeconds = 5
	BarMonitorIntervalSeconds  = 60
)

// Params 一组盘口信号回测参数。字段名后的括号是对应的生产配置键——
// 回测表单与 params_snapshot 都按这个口径展示，避免"回测参数"与"实盘参数"
// 各说一套。
type Params struct {
	Mode string `json:"mode"` // net / dual

	// 结构性常量（不参与寻优）
	Leverage   int     `json:"leverage"`   // 固定 125
	FaceValue  float64 `json:"faceValue"`  // position.risk.contract_face
	TakerFee   float64 `json:"takerFee"`   // 单边有效 taker 费率
	OrderSize  int     `json:"orderSize"`  // trade.accountN.order_size；0=沿用事件自带 orderSize
	RiskEquity float64 `json:"riskEquity"` // trade.accountN.risk_equity（cap 公式唯一本金输入）

	// 仓位上限：CapOverride>0 直接用固定上限（金标准脚本口径），
	// 否则走实盘公式 trade.ComputeMaxContracts(RiskEquity, 首个价, ...)。
	CapOverride        int     `json:"capOverride"`
	BudgetPct          float64 `json:"budgetPct"`          // position.risk.budget_pct（f，百分数）
	CatastropheStopPct float64 `json:"catastropheStopPct"` // position.monitor.catastrophe_stop_pct（S）
	Ceiling            int     `json:"ceiling"`            // position.risk.max_contracts_ceiling
	// CatastropheOvershootRoiPts 兜底成交过冲（ROI 点）：真实成交总比触发线更差。
	// 0=金标准 backtest_dual_side.py 口径；5=风险参数研究 backtest_capsf_study.py 口径。
	CatastropheOvershootRoiPts float64 `json:"catastropheOvershootRoiPts"`

	// 门控
	GateMinProfitPct      float64 `json:"gateMinProfitPct"`      // trade.accountN.reverse_gate_min_profit_pct
	TrendGateWindowHours  float64 `json:"trendGateWindowHours"`  // trade.trend_gate.window_hours；0=关闭
	TrendGateThresholdPct float64 `json:"trendGateThresholdPct"` // trade.trend_gate.threshold_pct；0=关闭

	// 移动止盈分档（position.monitor.trail.*）
	TierSmallRatio    float64 `json:"tierSmallRatio"`
	TierLargeRatio    float64 `json:"tierLargeRatio"`
	SmallActivatePct  float64 `json:"smallActivatePct"`
	SmallGiveback     float64 `json:"smallGiveback"`
	MediumActivatePct float64 `json:"mediumActivatePct"`
	MediumGiveback    float64 `json:"mediumGiveback"`
	LargeActivatePct  float64 `json:"largeActivatePct"`
	LargeGiveback     float64 `json:"largeGiveback"`

	// 信号阈值（monitor.symbols.<SYM>.signal_threshold，此处按 bp 表达）。
	// BaselineThresholdBp 是事件流的生产阈值 θ0；SignalThresholdBp 是本组要验的 θ。
	// 两者不等 ⇒ 精度降为频率级（见 fidelity.go），因为历史事件被 θ0 结构性截断。
	SignalThresholdBp   float64 `json:"signalThresholdBp"`
	BaselineThresholdBp float64 `json:"baselineThresholdBp"`

	// 回放口径
	EvalMode string `json:"evalMode"` // close / pessimistic
	EntryPx  string `json:"entryPx"`  // bar_close / sig_last
}

// DefaultParams 生产缺省组成的基线参数（净仓、close 评估、bar 收盘成交）。
// 回测表单的初值与 params_snapshot 的兜底都用它，避免零值参数悄悄跑出结果。
func DefaultParams() Params {
	return Params{
		Mode:                  ModeNet,
		Leverage:              DefaultLeverage,
		FaceValue:             DefaultFaceValue,
		TakerFee:              DefaultTakerFee,
		RiskEquity:            0,
		BudgetPct:             DefaultBudgetPct,
		CatastropheStopPct:    DefaultCatastropheStopPct,
		Ceiling:               DefaultCeiling,
		GateMinProfitPct:      DefaultGateMinProfitPct,
		TrendGateWindowHours:  0,
		TrendGateThresholdPct: 0,
		TierSmallRatio:        DefaultTierSmallRatio,
		TierLargeRatio:        DefaultTierLargeRatio,
		SmallActivatePct:      DefaultSmallActivatePct,
		SmallGiveback:         DefaultSmallGiveback,
		MediumActivatePct:     DefaultMediumActivatePct,
		MediumGiveback:        DefaultMediumGiveback,
		LargeActivatePct:      DefaultLargeActivatePct,
		LargeGiveback:         DefaultLargeGiveback,
		EvalMode:              EvalClose,
		EntryPx:               EntryBarClose,
	}
}

// Normalize 把零值补成生产缺省（不改已显式给出的值），并归一化枚举。
// 回测请求只传要改的旋钮，其余一律回落生产缺省——这是"改一个参数看差异"的前提。
func (p Params) Normalize() Params {
	d := DefaultParams()
	if p.Mode != ModeDual {
		p.Mode = ModeNet
	}
	if p.Leverage <= 0 {
		p.Leverage = d.Leverage
	}
	if p.FaceValue <= 0 {
		p.FaceValue = d.FaceValue
	}
	if p.TakerFee < 0 {
		p.TakerFee = d.TakerFee
	}
	if p.BudgetPct <= 0 {
		p.BudgetPct = d.BudgetPct
	}
	if p.CatastropheStopPct <= 0 {
		p.CatastropheStopPct = d.CatastropheStopPct
	}
	if p.Ceiling <= 0 {
		p.Ceiling = d.Ceiling
	}
	if p.GateMinProfitPct < 0 {
		p.GateMinProfitPct = 0 // 与 resolveReverseGateMinProfit 的负值钳零一致
	}
	if p.TierSmallRatio <= 0 {
		p.TierSmallRatio = d.TierSmallRatio
	}
	if p.TierLargeRatio <= 0 {
		p.TierLargeRatio = d.TierLargeRatio
	}
	if p.SmallActivatePct <= 0 {
		p.SmallActivatePct = d.SmallActivatePct
	}
	if p.SmallGiveback <= 0 {
		p.SmallGiveback = d.SmallGiveback
	}
	if p.MediumActivatePct <= 0 {
		p.MediumActivatePct = d.MediumActivatePct
	}
	if p.MediumGiveback <= 0 {
		p.MediumGiveback = d.MediumGiveback
	}
	if p.LargeActivatePct <= 0 {
		p.LargeActivatePct = d.LargeActivatePct
	}
	if p.LargeGiveback <= 0 {
		p.LargeGiveback = d.LargeGiveback
	}
	if p.EvalMode != EvalPessimistic {
		p.EvalMode = EvalClose
	}
	if p.EntryPx != EntrySigLast {
		p.EntryPx = EntryBarClose
	}
	if p.TrendGateWindowHours < 0 {
		p.TrendGateWindowHours = 0
	}
	if p.TrendGateThresholdPct < 0 {
		p.TrendGateThresholdPct = 0 // 与 resolveTrendGateThreshold 一致：负值=关闭
	}
	return p
}

// Validate 复用实盘的参数校验器 trade.ValidateRiskParams——回测拒绝的参数集
// 与实盘启动 fail-fast 拒绝的完全一致（含"S≥250：紧止损杀死均值回归 edge"
// 这类实证护栏），不在本包另立一套阈值。
//
// CapOverride>0（固定上限，金标准脚本口径）时 cap 公式的两个输入
// （risk_equity / budget_pct）不参与回放，用占位值过校验并把上限当天花板。
func (p Params) Validate() error {
	p = p.Normalize()
	view := argusTrade.RiskParamsView{
		RiskEquity: p.RiskEquity,
		BudgetPct:  p.BudgetPct,
		StopPct:    p.CatastropheStopPct,
		Ceiling:    p.Ceiling,
		OrderSize:  p.EffectiveOrderSize(1),
		GateMin:    p.GateMinProfitPct,
		SmallAct:   p.SmallActivatePct,
		SmallGb:    p.SmallGiveback,
		MedAct:     p.MediumActivatePct,
		MedGb:      p.MediumGiveback,
		LargeAct:   p.LargeActivatePct,
		LargeGb:    p.LargeGiveback,
		TierSmall:  p.TierSmallRatio,
		TierLarge:  p.TierLargeRatio,
	}
	if p.CapOverride > 0 {
		view.Ceiling = p.CapOverride
		view.RiskEquity, view.BudgetPct = 1, 1 // 占位：本组不走 cap 公式
	}
	if view.OrderSize > view.Ceiling {
		// 与 ValidateRiskParams 的口径一致，但先给出更具体的提示
		return errOrderSizeOverCeiling(view.OrderSize, view.Ceiling)
	}
	return argusTrade.ValidateRiskParams(view)
}

// EffectiveOrderSize 本次下单张数：Params.OrderSize>0 用它，否则沿用事件自带的
// orderSize（基线重放时不用配置就能复现真实下单量），再兜底 1。
func (p Params) EffectiveOrderSize(eventOrderSize int) int {
	if p.OrderSize > 0 {
		return p.OrderSize
	}
	if eventOrderSize > 0 {
		return eventOrderSize
	}
	return 1
}

// capParams 组装实盘的 cap 公式参数（含分档比例，供上限日志/分档边界复用）。
func (p Params) capParams() argusTrade.CapParams {
	return argusTrade.CapParams{
		Leverage:           p.Leverage,
		FaceValue:          p.FaceValue,
		RiskBudgetFraction: p.BudgetPct / 100,
		CatastropheStopPct: p.CatastropheStopPct,
		Ceiling:            p.Ceiling,
		TierSmallRatio:     p.TierSmallRatio,
		TierLargeRatio:     p.TierLargeRatio,
	}
}
