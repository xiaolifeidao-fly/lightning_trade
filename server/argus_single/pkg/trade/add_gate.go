package trade

import (
	"fmt"
	"strings"
)

// 加仓闸与本金回撤兜底的账户级参数（DB 覆盖层键名 = AccFloat 参数名；也可用
// properties 的 trade.accountN.<name> / 全局键兜底）。两者缺省 0 = 关闭，老部署零行为变化。
//
// 研究依据：doc/module/收益诊断-2026-09-15/诊断报告.md 的 09-19 下午追补与 10-03 复核——
// 账户B 的 equityStopPct=20 × addMinRoiPct=−200 格在 N=32 集合评估里训练/测试/留出
// 三段同向 24~32/32、兜底不劣 32/32，留出段（09-15→10-03）参照兜底 5~6 次 → 1 次。

// ResolveEquityStopPct 本金回撤兜底线（占 risk_equity 的 %）。>0 时持仓监控改按
// 未实现亏损 USDT ≥ equity_stop_pct% × risk_equity 触发兜底，ROI 兜底不再生效（替换，不叠加）。
func ResolveEquityStopPct(acc AccountConfig) float64 {
	return AccFloat(acc.Index, "equity_stop_pct", "position.monitor.equity_stop_pct", 0)
}

// resolveAddMinRoiPct 加仓闸：净仓 ROI% 低于它时不再同向加仓（负值；全新开仓与反向减仓不受影响）。
func resolveAddMinRoiPct(acc AccountConfig) float64 {
	return AccFloat(acc.Index, "add_min_roi_pct", "position.risk.add_min_roi_pct", 0)
}

// AddGateDecision 加仓闸决策。
type AddGateDecision struct {
	Block  bool
	RoiPct float64
	Reason string
}

// EvaluateAddGate 加仓闸（纯函数）：同向加仓时净仓 ROI% 低于 minRoiPct（负值）就不再摊平。
// minRoiPct >= 0 视为关闭；净仓为 0（全新开仓）或方向不同（减仓，由调用方先判 isReduction）一律放行。
// ROI 口径与反向减仓门控一致：价格有利变动幅度 × 杠杆。
func EvaluateAddGate(netSide, openSide string, netSize int, avgPx, lastPx float64, leverage int, minRoiPct float64) AddGateDecision {
	if minRoiPct >= 0 || netSize <= 0 || !strings.EqualFold(netSide, openSide) || avgPx <= 0 || lastPx <= 0 {
		return AddGateDecision{}
	}
	roi := netRoiPct(netSide, avgPx, lastPx, leverage)
	if roi < minRoiPct {
		return AddGateDecision{Block: true, RoiPct: roi,
			Reason: fmt.Sprintf("加仓闸: ROI %.1f%% < %.0f%%，不再摊平（当前%d张）", roi, minRoiPct, netSize)}
	}
	return AddGateDecision{RoiPct: roi}
}
