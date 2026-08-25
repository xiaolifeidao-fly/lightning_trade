package trade

import (
	"math"
	"testing"
)

func TestIsReduction(t *testing.T) {
	cases := []struct {
		name     string
		openSide string
		netSide  string
		netSize  int
		want     bool
	}{
		{"open long vs net short -> reduction", "long", "short", 5, true},
		{"open short vs net long -> reduction", "short", "long", 5, true},
		{"open long vs net long -> accumulation", "long", "long", 5, false},
		{"open short vs net short -> accumulation", "short", "short", 5, false},
		{"open long vs flat -> fresh open", "long", "long", 0, false},
		{"open long vs flat(short side label) -> fresh", "long", "short", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isReduction(c.openSide, c.netSide, c.netSize); got != c.want {
				t.Fatalf("isReduction(%s,%s,%d)=%v want %v", c.openSide, c.netSide, c.netSize, got, c.want)
			}
		})
	}
}

func TestEvaluateReverseGate(t *testing.T) {
	const L = 125
	const minProfit = 20.0
	cases := []struct {
		name      string
		netSide   string
		avgPx     float64
		lastPx    float64
		orderSize int
		netSize   int
		wantAllow bool
	}{
		{"long deep profit, pure reduction -> allow", "long", 60000, 61000, 1, 5, true},         // ROI +208%
		{"long underwater -> block", "long", 60000, 59500, 1, 5, false},                         // ROI -104%
		{"long tiny profit below fee threshold -> block", "long", 60000, 60080, 1, 5, false},    // ROI +16.7% < 20
		{"long just over threshold -> allow", "long", 60000, 60100, 1, 5, true},                 // ROI +20.8%
		{"short deep profit -> allow", "short", 60000, 59000, 1, 5, true},                       // ROI +208%
		{"short underwater -> block", "short", 60000, 61000, 1, 5, false},                       // ROI -208%
		{"profitable but flip-through (order>net) -> block", "long", 60000, 62000, 3, 1, false}, // +417% but 3>1
		{"profitable and order==net -> allow", "long", 60000, 62000, 5, 5, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := EvaluateReverseGate(c.netSide, c.avgPx, c.lastPx, L, minProfit, c.orderSize, c.netSize)
			if d.Allow != c.wantAllow {
				t.Fatalf("Allow=%v want %v (roi=%.2f reason=%q)", d.Allow, c.wantAllow, d.RoiPct, d.Reason)
			}
		})
	}
}

func TestIsReverseGate(t *testing.T) {
	cases := []struct {
		rg, tp string
		want   bool
	}{
		{"on", "trailing", true},
		{"on", "fixed", false}, // P1 coupling: gate needs trailing's cap+stop
		{"off", "trailing", false},
		{"", "trailing", false},
		{"on", "", false}, // empty tp -> fixed -> gate disabled
		{"true", "trailing", true},
	}
	for _, c := range cases {
		acc := &AccountConfig{ReverseGate: c.rg, TPMode: c.tp}
		if got := acc.IsReverseGate(); got != c.want {
			t.Errorf("IsReverseGate(rg=%q,tp=%q)=%v want %v", c.rg, c.tp, got, c.want)
		}
	}
}

func TestEvaluateReverseGate_ROIFormula(t *testing.T) {
	// long, +0.16% price move @ 125x = +20.00% ROI exactly (threshold boundary -> allow)
	d := EvaluateReverseGate("long", 60000, 60096, 125, 20, 1, 5)
	if math.Abs(d.RoiPct-20.0) > 0.01 {
		t.Fatalf("RoiPct=%.4f want 20.00", d.RoiPct)
	}
	if !d.Allow {
		t.Fatalf("roi==threshold should allow (>=), got block: %q", d.Reason)
	}
}

