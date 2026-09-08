package argus_event

import (
	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
	argusDTO "service/argus_event/dto"
)

// 本文件是读侧的口径字典：事件类型、结果大类、门控种类、信号强度分级。
//
// 全部枚举值都从写侧的常量引出（eventlog.Ev* / eventstore.GateKind*），
// 不在这里重打一遍字符串——写侧改名会直接编译报错，而不是让前端筛选静默失配。

// 事件大类。触发类是"一次盘口信号在某账户上的判定结果"，出场类是持仓的终结。
const (
	CategoryTrigger = "trigger"
	CategoryExit    = "exit"
	CategoryAll     = "all"
)

// 结果大类（列表里的 resultKind）。
const (
	ResultKindOpen    = "open"    // 成交开仓/加仓/减仓
	ResultKindBlocked = "blocked" // 被门控 / 上限 / 趋势闸挡住
	ResultKindExit    = "exit"    // 出场
	ResultKindAlert   = "alert"   // 浮亏告警，不改变仓位
)

// 结果筛选的两个聚合值（Result 参数还接受任意具体 gate_kind）。
const (
	ResultFilterOpen    = "open"
	ResultFilterBlocked = "blocked"
)

// TriggerEvents 四类触发事件：一次盘口信号在每个账户上要么成交、要么被三种
// 条件之一挡住。信号列表默认只看这四类。
func TriggerEvents() []string {
	return []string{eventlog.EvOpen, eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip}
}

// BlockedEvents 三类拦截事件。
func BlockedEvents() []string {
	return []string{eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip}
}

// ExitEvents 出场与告警类事件。loss_alert 不是出场，但它与出场同属"持仓期间
// 发生的事"，放同一大类方便复盘页一次拉全。
func ExitEvents() []string {
	return []string{eventlog.EvTrailingClose, eventlog.EvCatastropheStop, eventlog.EvFixedClose,
		eventlog.EvManualClose, eventlog.EvExternalClose, eventlog.EvLossAlert}
}

// AllEvents strategy_event 里的全部 10 类事件（balance / dev_sample 在另外两张表）。
func AllEvents() []string {
	return append(TriggerEvents(), ExitEvents()...)
}

// eventLabels 事件类型的中文名，与 Telegram 消息和原型页面用词保持一致。
var eventLabels = map[string]string{
	eventlog.EvOpen:            "成交开仓",
	eventlog.EvCapSkip:         "上限跳过",
	eventlog.EvGateBlock:       "门控拦截",
	eventlog.EvTrendSkip:       "趋势闸拦截",
	eventlog.EvTrailingClose:   "移动止盈",
	eventlog.EvCatastropheStop: "兜底止损",
	eventlog.EvFixedClose:      "固定止盈",
	eventlog.EvManualClose:     "人工平仓",
	eventlog.EvExternalClose:   "交易所侧平仓",
	eventlog.EvLossAlert:       "浮亏告警",
}

// EventLabel 事件类型的展示名；未知类型原样返回，不吞掉新事件。
func EventLabel(event string) string {
	if label, ok := eventLabels[event]; ok {
		return label
	}
	return event
}

// gateLabels 门控种类的中文名。
var gateLabels = map[string]string{
	eventstore.GateKindCap:               "仓位上限",
	eventstore.GateKindReverseGateProfit: "反向门控 · 盈利不足",
	eventstore.GateKindReverseGateFlip:   "反向门控 · 会翻转方向",
	eventstore.GateKindReverseGate:       "反向门控",
	eventstore.GateKindTrendGateShort:    "趋势闸 · 禁逆势加空",
	eventstore.GateKindTrendGateLong:     "趋势闸 · 禁逆势加多",
	eventstore.GateKindTrendGate:         "趋势闸",
}

// GateLabel 门控种类的展示名；reason 文案漂移导致的兜底 kind 也有名字。
func GateLabel(kind string) string {
	if kind == "" {
		return ""
	}
	if label, ok := gateLabels[kind]; ok {
		return label
	}
	return kind
}

// GateKinds 全部门控种类，供前端下拉框。
func GateKinds() []string {
	return []string{
		eventstore.GateKindCap,
		eventstore.GateKindReverseGateProfit,
		eventstore.GateKindReverseGateFlip,
		eventstore.GateKindReverseGate,
		eventstore.GateKindTrendGateShort,
		eventstore.GateKindTrendGateLong,
		eventstore.GateKindTrendGate,
	}
}

// ResultKindOf 事件类型 → 结果大类。
func ResultKindOf(event string) string {
	switch event {
	case eventlog.EvOpen:
		return ResultKindOpen
	case eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip:
		return ResultKindBlocked
	case eventlog.EvLossAlert:
		return ResultKindAlert
	default:
		return ResultKindExit
	}
}

// 信号强度分级。档位与原型 signals.html 一致，并对齐生产阈值的实测下界：
// 生产 signal_threshold 是 5bp（实测 gap_bp 最小 5.01bp），所以 5bp 以下
// 在 strategy_event 里结构性不存在——分布主体要看 dev_sample，不是这里。
const (
	StrengthWeak   = "weak"
	StrengthMedium = "medium"
	StrengthStrong = "strong"
)

type strengthBand struct {
	Level string
	Label string
	Min   *float64 // 含
	Max   *float64 // 不含；nil = 无上界
}

func f64p(v float64) *float64 { return &v }

// strengthBands 分档定义。唯一的定义处：筛选、聚合、前端下拉都从这里取，
// 免得三处各写一遍 7 和 9。
var strengthBands = []strengthBand{
	{Level: StrengthWeak, Label: "弱 5–7 bp", Min: nil, Max: f64p(7)},
	{Level: StrengthMedium, Label: "中 7–9 bp", Min: f64p(7), Max: f64p(9)},
	{Level: StrengthStrong, Label: "强 ≥ 9 bp", Min: f64p(9), Max: nil},
}

// StrengthLevelOf 按 |gap_bp| 判档；gap_bp 为空（报价不可算）时返回空串，
// 不硬塞进"弱"档——那会让"弱信号占比"凭空变大。
func StrengthLevelOf(gapBp *float64) string {
	if gapBp == nil {
		return ""
	}
	abs := *gapBp
	if abs < 0 {
		abs = -abs
	}
	for _, band := range strengthBands {
		if band.Min != nil && abs < *band.Min {
			continue
		}
		if band.Max != nil && abs >= *band.Max {
			continue
		}
		return band.Level
	}
	return ""
}

// StrengthBoundsOf 返回某一档的 [min, max) 绝对值边界，供查询条件使用。
func StrengthBoundsOf(level string) (min, max *float64, ok bool) {
	for _, band := range strengthBands {
		if band.Level == level {
			return band.Min, band.Max, true
		}
	}
	return nil, nil, false
}

// StrengthOptions 分档定义的对外形态。
func StrengthOptions() []argusDTO.StrengthOptionDTO {
	out := make([]argusDTO.StrengthOptionDTO, 0, len(strengthBands))
	for _, band := range strengthBands {
		out = append(out, argusDTO.StrengthOptionDTO{
			Value: band.Level, Label: band.Label, MinAbsBp: band.Min, MaxAbsBp: band.Max,
		})
	}
	return out
}

// SourceLabel 数据来源标记的展示名。
func SourceLabel(source int8) string {
	switch source {
	case eventstore.SourceLive:
		return "直写"
	case eventstore.SourceBackfill:
		return "回灌"
	default:
		return ""
	}
}
