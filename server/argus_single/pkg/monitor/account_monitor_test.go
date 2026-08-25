package monitor

import (
	"testing"

	"common/utils"
)

func TestNormalizePositionPnl(t *testing.T) {
	tests := []struct {
		name string
		pos  utils.PositionInfo
		want string
	}{
		{
			name: "short loses when last price is above average price",
			pos: utils.PositionInfo{
				PosSide:          "short",
				AvgPx:            "63459.2",
				LastPx:           "63722.6",
				UnrealizedProfit: "0.5268000000000029",
			},
			want: "-0.5268000000000029",
		},
		{
			name: "short profits when last price is below average price",
			pos: utils.PositionInfo{
				PosSide:          "short",
				AvgPx:            "63459.2",
				LastPx:           "63200",
				UnrealizedProfit: "-0.5268",
			},
			want: "0.5268",
		},
		{
			name: "long profits when last price is above average price",
			pos: utils.PositionInfo{
				PosSide:          "long",
				AvgPx:            "63459.2",
				LastPx:           "63722.6",
				UnrealizedProfit: "-0.5268",
			},
			want: "0.5268",
		},
		{
			name: "long loses when last price is below average price",
			pos: utils.PositionInfo{
				PosSide:          "long",
				AvgPx:            "63459.2",
				LastPx:           "63200",
				UnrealizedProfit: "0.5268",
			},
			want: "-0.5268",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizePositionPnl(tt.pos)
			if !ok {
				t.Fatal("normalizePositionPnl returned ok=false")
			}
			if got.String() != tt.want {
				t.Fatalf("normalizePositionPnl() = %s, want %s", got.String(), tt.want)
			}
		})
	}
}

