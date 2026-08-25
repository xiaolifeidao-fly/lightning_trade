package trade

import "testing"

func TestNormalizeTPMode(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", TPModeFixed},
		{"fixed", TPModeFixed},
		{"FIXED", TPModeFixed},
		{" trailing ", TPModeTrailing},
		{"trailing", TPModeTrailing},
		{"TRAILING", TPModeTrailing},
		{"garbage", TPModeFixed},
	}
	for _, c := range cases {
		if got := normalizeTPMode(c.in); got != c.want {
			t.Errorf("normalizeTPMode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsTrailingTP(t *testing.T) {
	if !(&AccountConfig{TPMode: "trailing"}).IsTrailingTP() {
		t.Error("tp_mode=trailing should report IsTrailingTP() = true")
	}
	if (&AccountConfig{TPMode: "fixed"}).IsTrailingTP() {
		t.Error("tp_mode=fixed should report IsTrailingTP() = false")
	}
	if (&AccountConfig{TPMode: ""}).IsTrailingTP() {
		t.Error("empty tp_mode should default to fixed, not trailing")
	}
}

