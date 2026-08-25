package trade

import (
	"fmt"
	"strings"
)

// NetPosition 账户在某合约上的当前净持仓快照（净仓模式：每合约至多一个方向）。
type NetPosition struct {
	Side   string  // "long" / "short"（Size=0 时无意义）
	Size   int     // 净张数（绝对值）
	AvgPx  float64 // 开仓均价
	LastPx float64 // 最新价
	PosId  string  // 仓位ID（bot-close 注册表绑定用，review fix#2）
}

// GateDecision 反向减仓门控决策。
type GateDecision struct {
	Allow  bool
	RoiPct float64
	Reason string
}

// isReduction 判断"在 netSide 净仓上按 openSide 开仓"是否构成反向减仓（净仓模式）。
// 净仓为 0（全新开仓）或同向（加仓）均不是减仓。
func isReduction(openSide, netSide string, netSize int) bool {
	if netSize <= 0 {
		return false
	}
	return !strings.EqualFold(openSide, netSide)
}

// EvaluateReverseGate 反向减仓门控（仅对 isReduction=true 的单调用）：
// 仅当净仓 ROI >= minProfitPct 且 orderSize <= netSize（纯减仓、不翻转）才放行。
// ROI 口径与移动止盈一致：ROI% = 价格有利变动幅度 × 杠杆。
func EvaluateReverseGate(netSide string, avgPx, lastPx float64, leverage int, minProfitPct float64, orderSize, netSize int) GateDecision {
	roi := netRoiPct(netSide, avgPx, lastPx, leverage)
	if roi < minProfitPct {
		return GateDecision{Allow: false, RoiPct: roi, Reason: fmt.Sprintf("盈利不足 ROI=%.1f%% < %.0f%%", roi, minProfitPct)}
	}
	if orderSize > netSize {
		return GateDecision{Allow: false, RoiPct: roi, Reason: fmt.Sprintf("翻转单 order=%d>net=%d", orderSize, netSize)}
	}
	return GateDecision{Allow: true, RoiPct: roi, Reason: "锁利放行"}
}

