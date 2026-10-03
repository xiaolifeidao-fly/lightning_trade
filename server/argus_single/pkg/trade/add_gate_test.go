package trade

import "testing"

func TestEvaluateAddGate(t *testing.T) {
	cases := []struct {
		name              string
		netSide, openSide string
		netSize           int
		avgPx, lastPx     float64
		minRoi            float64
		wantBlock         bool
	}{
		{"关闭(0)不拦", "long", "long", 8, 86000, 83000, 0, false},
		{"多单 ROI −436 < −200 拦", "long", "long", 8, 86000, 83000, -200, true},
		{"多单 ROI −145 > −200 放", "long", "long", 8, 86000, 85000, -200, false},
		{"空单 ROI −436 < −200 拦", "short", "short", 8, 83000, 85900, -200, true},
		{"全新开仓不拦", "", "long", 0, 0, 83000, -200, false},
		{"反向(减仓)不拦", "long", "short", 8, 86000, 83000, -200, false},
		{"正阈值视为关闭", "long", "long", 8, 86000, 83000, 50, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dec := EvaluateAddGate(c.netSide, c.openSide, c.netSize, c.avgPx, c.lastPx, SignalLeverage, c.minRoi)
			if dec.Block != c.wantBlock {
				t.Fatalf("Block=%v 期望 %v (roi=%.1f reason=%s)", dec.Block, c.wantBlock, dec.RoiPct, dec.Reason)
			}
		})
	}
}

func TestResolveAddGateKeysFromOverrides(t *testing.T) {
	SetParamOverrides(ParamOverrides{Accounts: map[int]map[string]float64{
		2: {"equity_stop_pct": 20, "add_min_roi_pct": -200, "risk_equity": 125},
	}})
	defer SetParamOverrides(ParamOverrides{})
	acc := AccountConfig{Index: 2, Name: "B", InitialBalance: 98.7}
	if got := ResolveEquityStopPct(acc); got != 20 {
		t.Errorf("equity_stop_pct = %v, 期望 20", got)
	}
	if got := resolveAddMinRoiPct(acc); got != -200 {
		t.Errorf("add_min_roi_pct = %v, 期望 -200", got)
	}
	if got := ResolveRiskEquity(acc); got != 125 {
		t.Errorf("risk_equity = %v, 期望 125", got)
	}
	other := AccountConfig{Index: 1, Name: "A", InitialBalance: 414}
	if got := ResolveEquityStopPct(other); got != 0 {
		t.Errorf("未配置账户 equity_stop_pct 应为 0, got %v", got)
	}
}
