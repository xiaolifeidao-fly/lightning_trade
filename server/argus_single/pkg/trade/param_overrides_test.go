package trade

import (
	"testing"

	"common/middleware/vipper"
)

func TestAccFloatPrefersDatabaseOverrideOverProperties(t *testing.T) {
	t.Cleanup(func() { SetParamOverrides(ParamOverrides{}) })
	vipper.Set("trade.account1.small_activate", 150)
	t.Cleanup(func() { vipper.Set("trade.account1.small_activate", nil) })

	overrides := ParamOverrides{}
	overrides.SetAccount(1, "small_activate", 210)
	SetParamOverrides(overrides)

	if got := AccFloat(1, "small_activate", "position.monitor.trail.small_activate", 0); got != 210 {
		t.Fatalf("AccFloat = %v, want 210 from the database override", got)
	}
	// 覆盖层只覆盖被显式配置的账户，其余账户仍走 properties。
	if got := AccFloat(2, "small_activate", "position.monitor.trail.small_activate", 88); got != 88 {
		t.Fatalf("AccFloat(account 2) = %v, want the 88 default", got)
	}
}

func TestAccFloatFallsBackToPropertiesWhenNotConfigured(t *testing.T) {
	t.Cleanup(func() { SetParamOverrides(ParamOverrides{}) })
	vipper.Set("trade.account1.large_activate", 40)
	t.Cleanup(func() { vipper.Set("trade.account1.large_activate", nil) })

	overrides := ParamOverrides{}
	overrides.SetAccount(1, "small_activate", 210)
	SetParamOverrides(overrides)

	if got := AccFloat(1, "large_activate", "position.monitor.trail.large_activate", 0); got != 40 {
		t.Fatalf("AccFloat = %v, want the 40 configured in properties", got)
	}
}

func TestAccFloatUsesGlobalOverrideWhenAccountHasNone(t *testing.T) {
	t.Cleanup(func() { SetParamOverrides(ParamOverrides{}) })
	overrides := ParamOverrides{}
	overrides.SetGlobal("trade.trend_gate.threshold_pct", 5)
	SetParamOverrides(overrides)

	if got := AccFloat(3, "trend_gate_threshold_pct", "trade.trend_gate.threshold_pct", 0); got != 5 {
		t.Fatalf("AccFloat = %v, want 5 from the global override", got)
	}
}

func TestSetParamOverridesReplacesPreviousLayerEntirely(t *testing.T) {
	t.Cleanup(func() { SetParamOverrides(ParamOverrides{}) })
	first := ParamOverrides{}
	first.SetAccount(1, "budget_pct", 13.3)
	SetParamOverrides(first)

	// 每次热加载整体重建覆盖层：上一版配置过、这一版取消了的键必须彻底消失，
	// 否则「删掉一个参数」永远不生效。
	SetParamOverrides(ParamOverrides{})
	if got := AccFloat(1, "budget_pct", "position.risk.budget_pct", 20); got != 20 {
		t.Fatalf("AccFloat = %v, want the 20 default after the override was cleared", got)
	}
}

func TestSetParamOverridesCopiesInput(t *testing.T) {
	t.Cleanup(func() { SetParamOverrides(ParamOverrides{}) })
	overrides := ParamOverrides{}
	overrides.SetAccount(1, "max_contracts_ceiling", 26)
	SetParamOverrides(overrides)
	overrides.SetAccount(1, "max_contracts_ceiling", 999)

	if got := AccInt(1, "max_contracts_ceiling", "position.risk.max_contracts_ceiling", 20); got != 26 {
		t.Fatalf("AccInt = %d, want 26 (installed snapshot must not alias the caller's map)", got)
	}
}
