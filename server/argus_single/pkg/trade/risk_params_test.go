package trade

import (
	"strings"
	"testing"

	"common/middleware/vipper"
)

func validView() RiskParamsView {
	return RiskParamsView{
		RiskEquity: 414, BudgetPct: 13.3, StopPct: 400, Ceiling: 26, OrderSize: 1,
		GateMin: 8, SmallAct: 150, SmallGb: 0.35, MedAct: 90, MedGb: 0.28,
		LargeAct: 40, LargeGb: 0.20, TierSmall: 0.30, TierLarge: 0.65,
	}
}

// fix#4 回归（codex review#2）：负 gate_min 配置必须原样到达校验层被 fail-fast 拒绝。
// 原绕过路径：resolveRiskParamsView 复用 resolveReverseGateMinProfit，后者把负值
// 钳成 0（运行时防御）→ 校验层永远看不到非法值。直接构造 view 的用例测不到这条链，
// 必须走 配置→resolveRiskParamsView→ValidateRiskParams 全程。
func TestResolveRiskParamsViewReadsRawGateMin(t *testing.T) {
	key := "trade.account7.reverse_gate_min_profit_pct"
	vipper.Set(key, "-1")
	defer vipper.Set(key, "") // pickFloat 视空串为未设置，等效还原

	v := resolveRiskParamsView(AccountConfig{Index: 7, InitialBalance: 100}, 1)
	if v.GateMin != -1 {
		t.Fatalf("解析层应保留原始配置值 -1, got %v（0 说明又复用了运行时钳制）", v.GateMin)
	}
	err := ValidateRiskParams(v)
	if err == nil {
		t.Fatalf("负 gate_min 应被 fail-fast 拒绝")
	}
	if !strings.Contains(err.Error(), "reverse_gate_min_profit_pct") {
		t.Fatalf("错误应指明键名, got: %v", err)
	}
}

// fix#4 契约的另一半：运行时路径保留负值钳 0 防御，与校验层读原始值并存、不可互替。
func TestResolveReverseGateMinProfitClampsNegative(t *testing.T) {
	key := "trade.account8.reverse_gate_min_profit_pct"
	vipper.Set(key, "-1")
	defer vipper.Set(key, "")

	if got := resolveReverseGateMinProfit(AccountConfig{Index: 8}); got != 0 {
		t.Fatalf("运行时钳制应把负值钳成 0, got %v", got)
	}
}

func TestValidateRiskParamsPassAndFail(t *testing.T) {
	if err := ValidateRiskParams(validView()); err != nil {
		t.Fatalf("合法参数不应报错: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(*RiskParamsView)
		expect string // 错误信息应含的键名
	}{
		{"budget_pct 超上限", func(v *RiskParamsView) { v.BudgetPct = 101 }, "budget_pct"},
		{"budget_pct 零", func(v *RiskParamsView) { v.BudgetPct = 0 }, "budget_pct"},
		{"S 过紧(违反松兜底护栏)", func(v *RiskParamsView) { v.StopPct = 200 }, "catastrophe_stop_pct"},
		{"ceiling 非正", func(v *RiskParamsView) { v.Ceiling = 0 }, "max_contracts_ceiling"},
		{"risk_equity 非正", func(v *RiskParamsView) { v.RiskEquity = 0 }, "risk_equity"},
		{"giveback 越界", func(v *RiskParamsView) { v.LargeGb = 1.0 }, "large_giveback"},
		{"giveback 零", func(v *RiskParamsView) { v.SmallGb = 0 }, "small_giveback"},
		{"分档比例不递增", func(v *RiskParamsView) { v.TierSmall = 0.7 }, "tier"},
		{"分档比例越界", func(v *RiskParamsView) { v.TierLarge = 1.0 }, "tier"},
		{"激活阈值非正", func(v *RiskParamsView) { v.MedAct = 0 }, "medium_activate"},
		{"order_size 超 ceiling", func(v *RiskParamsView) { v.OrderSize = 27 }, "order_size"},
		{"gate_min 负数", func(v *RiskParamsView) { v.GateMin = -1 }, "reverse_gate_min_profit_pct"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := validView()
			c.mutate(&v)
			err := ValidateRiskParams(v)
			if err == nil {
				t.Fatalf("应报错")
			}
			if !strings.Contains(err.Error(), c.expect) {
				t.Fatalf("错误应指明键名 %q, got: %v", c.expect, err)
			}
		})
	}
}

