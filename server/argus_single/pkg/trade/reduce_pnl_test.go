package trade

import (
	"math"
	"testing"
)

func almostEq(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestNetRoiPct(t *testing.T) {
	tests := []struct {
		name    string
		netSide string
		avgPx   float64
		lastPx  float64
		want    float64
	}{
		{"long 盈利", "long", 60000, 60480, 100.0}, // +0.8% × 125
		{"long 亏损", "long", 60000, 59520, -100.0},
		{"short 盈利", "short", 60000, 59520, 100.0},
		{"short 亏损", "short", 60000, 60480, -100.0},
		{"avgPx=0 防御", "long", 0, 60000, 0},
		{"大小写不敏感", "LONG", 60000, 60480, 100.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := netRoiPct(tt.netSide, tt.avgPx, tt.lastPx, 125); !almostEq(got, tt.want) {
				t.Fatalf("netRoiPct = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEstimateReducePnl(t *testing.T) {
	tests := []struct {
		name      string
		netSide   string
		avgPx     float64
		lastPx    float64
		face      float64
		orderSize int
		want      float64
	}{
		{"long 减1张锁利", "long", 63000, 63220, 0.001, 1, 0.22},
		{"short 减2张锁利", "short", 63459.2, 63200, 0.001, 2, 0.5184},
		{"long 减仓在亏损价(理论上被gate拦, 防御性负值)", "long", 63000, 62800, 0.001, 1, -0.2},
		{"avgPx=0 防御", "long", 0, 63000, 0.001, 1, 0},
		{"lastPx=0 防御", "long", 63000, 0, 0.001, 1, 0},
		{"orderSize=0 防御", "long", 63000, 63220, 0.001, 0, 0},
		{"face=0 防御", "long", 63000, 63220, 0, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateReducePnl(tt.netSide, tt.avgPx, tt.lastPx, tt.face, tt.orderSize)
			if !almostEq(got, tt.want) {
				t.Fatalf("estimateReducePnl = %v, want %v", got, tt.want)
			}
		})
	}
}

