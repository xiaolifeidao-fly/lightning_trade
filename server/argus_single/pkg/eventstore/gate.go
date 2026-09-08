package eventstore

import (
	"regexp"
	"strconv"
	"strings"

	"argus_single/pkg/eventlog"
)

// 门控种类（gate_kind）。需求大纲 §6.1 要求"可按 gate_kind 聚合统计今天最常被
// 什么条件挡住"，所以拦截原因必须是可分组的枚举，不能只留一句中文 reason。
const (
	GateKindCap               = "cap"                 // 仓位上限跳过
	GateKindReverseGateProfit = "reverse_gate_profit" // 反向减仓：盈利不足
	GateKindReverseGateFlip   = "reverse_gate_flip"   // 反向减仓：会翻转方向
	GateKindReverseGate       = "reverse_gate"        // 反向减仓：reason 文本无法结构化时的兜底
	GateKindTrendGateShort    = "trend_gate_short"    // 趋势闸：禁逆势开/加空
	GateKindTrendGateLong     = "trend_gate_long"     // 趋势闸：禁逆势开/加多
	GateKindTrendGate         = "trend_gate"          // 趋势闸兜底
)

// GateInfo 门控拦截的结构化口径：种类 + 阈值 + 实际值。
type GateInfo struct {
	Kind      string
	Threshold *float64
	Actual    *float64
}

// 三类拦截 reason 的产出点（改这些格式串要同步改这里的正则与测试）：
//   - pkg/trade/reverse_gate.go EvaluateReverseGate
//   - pkg/trade/manager.go 仓位上限分支
//   - pkg/trade/trend_gate.go EvaluateTrendGate
var (
	reReverseProfit = regexp.MustCompile(`盈利不足\s*ROI=(-?[0-9.]+)%\s*<\s*(-?[0-9.]+)%`)
	reReverseFlip   = regexp.MustCompile(`翻转单\s*order=(-?[0-9]+)>net=(-?[0-9]+)`)
	reCapSkip       = regexp.MustCompile(`当前(-?[0-9]+)\+(-?[0-9]+)>上限(-?[0-9]+)`)
	reTrendGate     = regexp.MustCompile(`动量([+-]?[0-9.]+)%\s*[≥≤]\s*(-?[0-9.]+)%`)
)

// ParseGate 从事件类型 + reason 文本结构化出门控三元组。
//
// 为什么解析文本而不是在埋点处新增结构化字段：r6 要把 logs/ 下 44 MB 历史
// JSONL 回灌进同一张表，历史行里只有 reason 串。若靠新字段，历史数据的
// gate_kind 全为空、聚合统计只能覆盖上线之后——那正是本任务要消灭的断层。
// 解析器是纯函数、直写与回灌共用一条代码路径，改埋点格式会被单测拦住。
//
// reason 文本漂移时仍按事件类型给出兜底 kind（阈值/实际值留空），保证
// "按 gate_kind 聚合"这个能力不会因为一句文案改动整段失效。
func ParseGate(event, reason string, e eventlog.Event) (GateInfo, bool) {
	switch event {
	case eventlog.EvGateBlock:
		if m := reReverseProfit.FindStringSubmatch(reason); m != nil {
			return GateInfo{Kind: GateKindReverseGateProfit, Actual: parseFloatPtr(m[1]), Threshold: parseFloatPtr(m[2])}, true
		}
		if m := reReverseFlip.FindStringSubmatch(reason); m != nil {
			return GateInfo{Kind: GateKindReverseGateFlip, Actual: parseFloatPtr(m[1]), Threshold: parseFloatPtr(m[2])}, true
		}
		// 兜底：ROI 在事件字段里也有一份，至少让"实际值"不丢。
		return GateInfo{Kind: GateKindReverseGate, Actual: nonZeroPtr(e.RoiPct)}, true
	case eventlog.EvCapSkip:
		if m := reCapSkip.FindStringSubmatch(reason); m != nil {
			net, netOK := parseFloat(m[1])
			order, orderOK := parseFloat(m[2])
			info := GateInfo{Kind: GateKindCap, Threshold: parseFloatPtr(m[3])}
			if netOK && orderOK {
				total := net + order
				info.Actual = &total
			}
			return info, true
		}
		return GateInfo{Kind: GateKindCap}, true
	case eventlog.EvTrendSkip:
		kind := GateKindTrendGate
		switch {
		case containsAny(reason, "禁逆势开/加空", "加空"):
			kind = GateKindTrendGateShort
		case containsAny(reason, "禁逆势开/加多", "加多"):
			kind = GateKindTrendGateLong
		}
		info := GateInfo{Kind: kind, Actual: nonZeroPtr(e.TrendMomPct)}
		if m := reTrendGate.FindStringSubmatch(reason); m != nil {
			if actual := parseFloatPtr(m[1]); actual != nil {
				info.Actual = actual
			}
			if th, ok := parseFloat(m[2]); ok {
				// 做多方向的闸线是负值（reason 里印成 "≤ -5.0%"，正则已带上负号）。
				if kind == GateKindTrendGateLong && th > 0 {
					th = -th
				}
				info.Threshold = &th
			}
		}
		return info, true
	default:
		return GateInfo{}, false
	}
}

func parseFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseFloatPtr(s string) *float64 {
	if v, ok := parseFloat(s); ok {
		return &v
	}
	return nil
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if sub != "" && strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
