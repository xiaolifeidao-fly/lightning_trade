package trade

import (
	"fmt"
	"strings"

	"common/middleware/vipper"
)

// pickFloat 账户键有值→账户值；否则全局键有值→全局值；否则默认。
// 用"原始字符串是否非空"判断键是否设置（vipper 无法区分未设置与 0）。
func pickFloat(accRaw string, accVal float64, globalRaw string, globalVal, def float64) float64 {
	if strings.TrimSpace(accRaw) != "" {
		return accVal
	}
	if strings.TrimSpace(globalRaw) != "" {
		return globalVal
	}
	return def
}

// AccFloat 解析账户级浮点参数：trade.account{index}.{name} 优先，否则 globalKey，否则 def。
func AccFloat(index int, name, globalKey string, def float64) float64 {
	accKey := fmt.Sprintf("trade.account%d.%s", index, name)
	return pickFloat(vipper.GetString(accKey), vipper.GetFloat64(accKey),
		vipper.GetString(globalKey), vipper.GetFloat64(globalKey), def)
}

// AccInt 账户级整型参数（同 AccFloat 取整）。
func AccInt(index int, name, globalKey string, def int) int {
	return int(AccFloat(index, name, globalKey, float64(def)))
}

// resolveCapParams 解析某账户的仓位上限参数（账户级覆盖 + 全局回退）。
func resolveCapParams(acc AccountConfig) CapParams {
	face := vipper.GetFloat64("position.risk.contract_face") // 结构性参数，仅全局
	if face <= 0 {
		face = 0.001
	}
	budgetPct := AccFloat(acc.Index, "budget_pct", "position.risk.budget_pct", 20)
	if acc.RiskBudget > 0 {
		budgetPct = acc.RiskBudget
	}
	stopPct := AccFloat(acc.Index, "catastrophe_stop_pct", "position.monitor.catastrophe_stop_pct", 300)
	if acc.CatastrophicStopLoss > 0 {
		stopPct = acc.CatastrophicStopLoss
	}
	ceiling := AccInt(acc.Index, "max_contracts_ceiling", "position.risk.max_contracts_ceiling", 20)
	if acc.MaxContracts > 0 {
		ceiling = acc.MaxContracts
	}
	return CapParams{
		Leverage:           SignalLeverage,
		FaceValue:          face,
		RiskBudgetFraction: budgetPct / 100,
		CatastropheStopPct: stopPct,
		Ceiling:            ceiling,
		TierSmallRatio:     AccFloat(acc.Index, "tier_small_ratio", "position.monitor.trail.tier_small_ratio", 0.30),
		TierLargeRatio:     AccFloat(acc.Index, "tier_large_ratio", "position.monitor.trail.tier_large_ratio", 0.65),
	}
}

// resolveReverseGateMinProfit 解析某账户的反向减仓最小盈利 ROI%（负值钳为 0）。
func resolveReverseGateMinProfit(acc AccountConfig) float64 {
	v := AccFloat(acc.Index, "reverse_gate_min_profit_pct", "position.risk.reverse_gate_min_profit_pct", 20)
	if v < 0 {
		v = 0
	}
	return v
}
