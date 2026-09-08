package trade

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// RiskParamsView 某账户解析后的全部风险参数快照（P5）。
// 用于启动校验与静态参数打印；解析层薄（vipper），校验层纯（可测）。
type RiskParamsView struct {
	RiskEquity           float64 // 风险计算基数（cap 公式唯一输入；缺省=InitialBalance）
	BudgetPct            float64 // f（%）
	StopPct              float64 // S
	Ceiling              int
	OrderSize            int
	GateMin              float64
	SmallAct, SmallGb    float64
	MedAct, MedGb        float64
	LargeAct, LargeGb    float64
	TierSmall, TierLarge float64
}

// ResolveRiskEquity 风险计算基数：argus_account_risk.risk_equity 显式配置优先，
// 其次 properties 的 trade.accountN.risk_equity，缺省=InitialBalance（零行为变化）。
// 制度性护栏（合并设计 P5）：risk_equity 只能来自配置，禁止任何"运行时余额→cap"
// 的自动路径——cap 变更一律走 challenger/OOS 审批后改配置。
//
// r5 之前这里有一条 `RiskBudget > 0 即回 InitialBalance` 的短路，用来防止 DB 驱动
// 时 properties 的 risk_equity 漏进来；risk_equity 有了自己的列之后，
// AccFloat 的「DB 优先 vipper 兜底」已经表达了同一件事，短路反而会把 DB 里
// 显式配置的 risk_equity 挡掉，故移除。
func ResolveRiskEquity(acc AccountConfig) float64 {
	return AccFloat(acc.Index, "risk_equity", "", acc.InitialBalance)
}

// resolveRiskParamsView 解析账户全部风险参数（薄 vipper 层）。
func resolveRiskParamsView(acc AccountConfig, globalOrderSize int) RiskParamsView {
	cp := resolveCapParams(acc)
	f := func(name, globalKey string, def float64) float64 {
		return AccFloat(acc.Index, name, globalKey, def)
	}
	return RiskParamsView{
		RiskEquity: ResolveRiskEquity(acc),
		BudgetPct:  cp.RiskBudgetFraction * 100,
		StopPct:    cp.CatastropheStopPct,
		Ceiling:    cp.Ceiling,
		OrderSize:  acc.GetOrderSize(globalOrderSize),
		// review fix#4：校验必须看原始配置值——resolveReverseGateMinProfit 会把负数
		// 钳成 0（运行时防御），若在此复用，配置 -1 将绕过 fail-fast
		GateMin:   f("reverse_gate_min_profit_pct", "position.risk.reverse_gate_min_profit_pct", 20),
		SmallAct:  f("small_activate", "position.monitor.trail.small_activate", 150),
		SmallGb:   f("small_giveback", "position.monitor.trail.small_giveback", 0.35),
		MedAct:    f("medium_activate", "position.monitor.trail.medium_activate", 90),
		MedGb:     f("medium_giveback", "position.monitor.trail.medium_giveback", 0.28),
		LargeAct:  f("large_activate", "position.monitor.trail.large_activate", 40),
		LargeGb:   f("large_giveback", "position.monitor.trail.large_giveback", 0.20),
		TierSmall: f("tier_small_ratio", "position.monitor.trail.tier_small_ratio", 0.30),
		TierLarge: f("tier_large_ratio", "position.monitor.trail.tier_large_ratio", 0.65),
	}
}

// ValidateRiskParams 风险参数合法性校验（纯函数，P5/R5 口径）。
// 违反即返回含配置键名的错误；启动路径据此 fail-fast 拒绝启动。
func ValidateRiskParams(v RiskParamsView) error {
	if v.RiskEquity <= 0 {
		return fmt.Errorf("risk_equity/InitialBalance 必须 >0, got %.2f", v.RiskEquity)
	}
	if v.BudgetPct <= 0 || v.BudgetPct > 100 {
		return fmt.Errorf("budget_pct 必须在 (0,100], got %.2f", v.BudgetPct)
	}
	if v.StopPct < 250 {
		return fmt.Errorf("catastrophe_stop_pct 必须 ≥250（松兜底护栏：紧止损杀死均值回归 edge）, got %.0f", v.StopPct)
	}
	if v.Ceiling < 1 {
		return fmt.Errorf("max_contracts_ceiling 必须 ≥1, got %d", v.Ceiling)
	}
	if v.OrderSize < 1 || v.OrderSize > v.Ceiling {
		return fmt.Errorf("order_size 必须在 [1, max_contracts_ceiling=%d], got %d", v.Ceiling, v.OrderSize)
	}
	if v.GateMin < 0 {
		return fmt.Errorf("reverse_gate_min_profit_pct 不得为负, got %.2f", v.GateMin)
	}
	type gb struct {
		name string
		val  float64
	}
	for _, g := range []gb{{"small_giveback", v.SmallGb}, {"medium_giveback", v.MedGb}, {"large_giveback", v.LargeGb}} {
		if g.val <= 0 || g.val >= 1 {
			return fmt.Errorf("%s 必须在 (0,1), got %.3f", g.name, g.val)
		}
	}
	for _, a := range []gb{{"small_activate", v.SmallAct}, {"medium_activate", v.MedAct}, {"large_activate", v.LargeAct}} {
		if a.val <= 0 {
			return fmt.Errorf("%s 必须 >0, got %.2f", a.name, a.val)
		}
	}
	if !(0 < v.TierSmall && v.TierSmall < v.TierLarge && v.TierLarge < 1) {
		return fmt.Errorf("tier_small_ratio/tier_large_ratio 必须满足 0<small<large<1, got %.2f/%.2f", v.TierSmall, v.TierLarge)
	}
	return nil
}

// ValidateAccounts 用当前已安装的参数覆盖层校验一份候选账户配置。
//
// 存在的理由：移动止盈档位等参数收敛到 DB 之后就变成了可热更新项，而
// NewTradeManager 校验不过是 logrus.Fatalf——一次误发布会直接打死正在扛仓的
// 进程，比不生效危险得多。热加载路径先用它做一次可返回错误的预校验，
// 不合法就拒绝这份快照、继续跑旧配置。启动路径仍保留 Fatalf 的 fail-fast。
func ValidateAccounts(config *TradingSystemConfig) error {
	if config == nil {
		return fmt.Errorf("trading system config is nil")
	}
	for _, acc := range config.Accounts {
		if !acc.IsTrailingTP() {
			continue
		}
		if err := ValidateRiskParams(resolveRiskParamsView(acc, config.Trade.OrderSize)); err != nil {
			return fmt.Errorf("账户 %s 风险参数非法: %w", acc.Name, err)
		}
	}
	return nil
}

// logStaticRiskParams 启动时打印静态参数行（P5 两段式打印之一；
// 动态行 P/N_formula/N_effective/档位 在 cap guard 首次成功初始化时打印）。
func logStaticRiskParams(acc AccountConfig, v RiskParamsView) {
	logrus.Infof("[风险参数] %s risk_equity=%.2f f=%.1f%% S=%.0f ceiling=%d order_size=%d gate_min=%.1f "+
		"trail=[小%.0f/%.2f 中%.0f/%.2f 大%.0f/%.2f] tier=[%.2f,%.2f] logic=%s variant=%s sl_mode=%s",
		acc.Name, v.RiskEquity, v.BudgetPct, v.StopPct, v.Ceiling, v.OrderSize, v.GateMin,
		v.SmallAct, v.SmallGb, v.MedAct, v.MedGb, v.LargeAct, v.LargeGb, v.TierSmall, v.TierLarge,
		acc.TradeLogic, acc.Variant, acc.StopLossMode)
}
