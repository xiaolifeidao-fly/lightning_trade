package monitor

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestApplyParamsHotUpdatesMonitorTuning(t *testing.T) {
	am := &AccountMonitor{
		posQueryInterval:   5 * time.Second,
		pnlProfitThreshold: decimal.NewFromInt(150),
		pnlLossThreshold:   decimal.NewFromInt(150),
	}

	am.ApplyParams(10, 200, 120)

	if got := am.PositionInterval(); got != 10*time.Second {
		t.Fatalf("interval = %v, want 10s", got)
	}
	if got := am.profitThreshold().String(); got != "200" {
		t.Fatalf("profit threshold = %s, want 200", got)
	}
	if got := am.lossThreshold().String(); got != "120" {
		t.Fatalf("loss threshold = %s, want 120", got)
	}
}

func TestApplyParamsIgnoresUnsetValues(t *testing.T) {
	am := &AccountMonitor{
		posQueryInterval:   5 * time.Second,
		pnlProfitThreshold: decimal.NewFromInt(150),
		pnlLossThreshold:   decimal.NewFromInt(150),
	}

	// 0 表示配置里没填，此时保留现有取值，不能把巡检间隔归零把 ticker 打爆。
	am.ApplyParams(0, 0, 0)

	if got := am.PositionInterval(); got != 5*time.Second {
		t.Fatalf("interval = %v, want the previous 5s", got)
	}
	if got := am.profitThreshold().String(); got != "150" {
		t.Fatalf("profit threshold = %s, want the previous 150", got)
	}
}

func TestApplyAccountMonitorParamsWithoutMonitorIsNoop(t *testing.T) {
	previous := globalAccountMonitor
	globalAccountMonitor = nil
	t.Cleanup(func() { globalAccountMonitor = previous })

	ApplyAccountMonitorParams(10, 200, 120)
}
