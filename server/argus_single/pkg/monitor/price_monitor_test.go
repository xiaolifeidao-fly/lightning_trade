package monitor

import "testing"

func TestIsSpreadThresholdCross(t *testing.T) {
	threshold := 0.001

	tests := []struct {
		name    string
		prev    float64
		hasPrev bool
		current float64
		want    bool
	}{
		{
			name:    "first sample only initializes baseline",
			current: 0.0012,
			want:    false,
		},
		{
			name:    "crosses upward threshold",
			prev:    0.0008,
			hasPrev: true,
			current: 0.0011,
			want:    true,
		},
		{
			name:    "stays above upward threshold",
			prev:    0.0012,
			hasPrev: true,
			current: 0.0013,
			want:    false,
		},
		{
			name:    "crosses downward threshold",
			prev:    -0.0008,
			hasPrev: true,
			current: -0.0011,
			want:    true,
		},
		{
			name:    "stays below downward threshold",
			prev:    -0.0012,
			hasPrev: true,
			current: -0.0013,
			want:    false,
		},
		{
			name:    "inside threshold",
			prev:    0.0002,
			hasPrev: true,
			current: -0.0009,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSpreadThresholdCross(tt.prev, tt.hasPrev, tt.current, threshold)
			if got != tt.want {
				t.Fatalf("isSpreadThresholdCross() = %v, want %v", got, tt.want)
			}
		})
	}
}

