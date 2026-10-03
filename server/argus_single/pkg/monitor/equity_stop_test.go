package monitor

import (
	"testing"

	"argus_single/pkg/trade"

	"github.com/shopspring/decimal"
)

func TestEquityStopLineFromOverrides(t *testing.T) {
	trade.SetParamOverrides(trade.ParamOverrides{Accounts: map[int]map[string]float64{
		2: {"equity_stop_pct": 20, "risk_equity": 125},
	}})
	defer trade.SetParamOverrides(trade.ParamOverrides{})
	line, on := resolveEquityStopLine(trade.AccountConfig{Index: 2, Name: "B", InitialBalance: 98.7})
	if !on || line != 25 {
		t.Fatalf("权益线 = %.2f/%v, 期望 25/true", line, on)
	}
	if _, on := resolveEquityStopLine(trade.AccountConfig{Index: 1, Name: "A", InitialBalance: 414}); on {
		t.Fatal("未配置账户不应启用权益线")
	}
}

func TestEquityStopHitAndRoiStopDisabled(t *testing.T) {
	if !equityStopHit(decimal.NewFromFloat(-25.0), 25) || equityStopHit(decimal.NewFromFloat(-24.9), 25) {
		t.Fatal("权益线触发边界错误")
	}
	if equityStopHit(decimal.NewFromFloat(-100), 0) {
		t.Fatal("线为 0 不应触发")
	}
	tp := applyEquityStop(TrailParams{TierSmallRatio: 0.3, TierLargeRatio: 0.65,
		Small: Tier{150, 0.35}, Medium: Tier{90, 0.28}, Large: Tier{40, 0.20}, CatastropheStopPct: 400})
	cfg := BuildExitConfig(8, tp)
	// ROI −900% 在权益线模式下不触发 ROI 兜底（由权益线接管）；移动止盈档位不变。
	if action, _ := EvaluateExit(8, -900, cfg, TrailState{}); action == ActionCatastropheStop {
		t.Fatal("权益线模式下 ROI 兜底应永不触发")
	}
	if cfg.Large.ActivatePct != 40 || cfg.SmallMaxContracts != 2 || cfg.LargeMinContracts != 6 {
		t.Fatalf("分档应原样保留: %+v", cfg)
	}
	if r := equityStopReason(25); r != "兜底止损(权益线 25.0U)" {
		t.Fatalf("理由串 = %q", r)
	}
}
