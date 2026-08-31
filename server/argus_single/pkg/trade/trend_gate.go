package trade

import (
	"fmt"
	"strings"

	"argus_single/pkg/eventlog"
)

// TrendGateDecision 趋势闸判定结果。
type TrendGateDecision struct {
	Block  bool
	Reason string
}

// EvaluateTrendGate 趋势闸（8/21 事故复盘补丁）：N 小时动量超阈时禁止逆势
// 方向的全新开仓与同向加仓；减仓不经过本函数（manager 分流层已把反向减仓
// 送进 reverse_gate 分支）。
//
// 机制依据：125x 下 -400% ROI 只等于 3.2% 的价格逆行（一个"死亡距离"），
// 而 reverse_gate 挡亏损减仓、不挡同向加仓，单边行情里逆势书是"只进不出"
// 的棘轮，必然被打到兜底。闸线 5% ≈ 1.6 个死亡距离。
// 回测（docs/backtest_aug1821_scan3.py，三窗口）：24h/5% 事故窗三账户合计
// 中位 -818U → +405U（兜底中位 2→0-1 次），6/09-7/02 混合窗 23 天仅触发
// 1 次（+1011.8 vs +1012.0），温和涨 OOS 零触发；敏感性平台 3-5.5%、悬崖 6%。
//
// momOK=false（历史不足一个窗口：重启后未回填）→ 放行，与回测 warmup 缺失
// 语义一致；thresholdPct ≤ 0 = 未启用（默认，部署安全）。
func EvaluateTrendGate(posSide string, momPct float64, momOK bool, thresholdPct float64) TrendGateDecision {
	if thresholdPct <= 0 || !momOK {
		return TrendGateDecision{}
	}
	if strings.EqualFold(posSide, "short") && momPct >= thresholdPct {
		return TrendGateDecision{Block: true,
			Reason: fmt.Sprintf("趋势闸: 动量%+.2f%% ≥ %.1f%%, 禁逆势开/加空", momPct, thresholdPct)}
	}
	if strings.EqualFold(posSide, "long") && momPct <= -thresholdPct {
		return TrendGateDecision{Block: true,
			Reason: fmt.Sprintf("趋势闸: 动量%+.2f%% ≤ -%.1f%%, 禁逆势开/加多", momPct, thresholdPct)}
	}
	return TrendGateDecision{}
}

// buildTrendSkipEvent 构造 trend_skip 事件（拦截即数据：被拦信号的 gap/动量
// 分布是后续"顺势 vs 逆势开仓结局"研究的原料，字段口径对齐 gate_block）。
func buildTrendSkipEvent(acc AccountConfig, instId, posSide string, net NetPosition,
	orderSize int, dec TrendGateDecision, q SignalQuote) eventlog.Event {
	ev := eventlog.Event{Account: acc.Name, Variant: acc.Variant, InstId: instId,
		Event: eventlog.EvTrendSkip, Side: posSide, NetSide: net.Side, Size: net.Size,
		OrderSize: orderSize, TrendMomPct: q.TrendMomPct, Reason: dec.Reason}
	return applySignalQuote(ev, q)
}

// resolveTrendGateThreshold 解析某账户的趋势闸阈值（%）。
// 账户级 trade.accountN.trend_gate_threshold_pct > 全局 trade.trend_gate.threshold_pct
// > 默认 0（=关闭：不配置绝不启用，负值同关闭）。
func resolveTrendGateThreshold(acc AccountConfig) float64 {
	v := AccFloat(acc.Index, "trend_gate_threshold_pct", "trade.trend_gate.threshold_pct", 0)
	if v < 0 {
		v = 0
	}
	return v
}
