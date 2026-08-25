package monitor

import "testing"

// account2 档位（上限 12）：小 1-3 / 中 4-7 / 大 8-12
func testExitCfg() ExitConfig {
	return ExitConfig{
		SmallMaxContracts:  3,
		LargeMinContracts:  8,
		Small:              Tier{ActivatePct: 150, GivebackFrac: 0.35},
		Medium:             Tier{ActivatePct: 90, GivebackFrac: 0.28},
		Large:              Tier{ActivatePct: 40, GivebackFrac: 0.20},
		CatastropheStopPct: 300,
	}
}

func TestEvaluateExit(t *testing.T) {
	cfg := testExitCfg()
	cases := []struct {
		name       string
		size       int
		pnlPct     float64
		prev       TrailState
		wantAction ExitAction
		wantActive bool
		wantPeak   float64
		checkPeak  bool
	}{
		{"below activation holds (small tier A=150)", 2, 120, TrailState{LastSize: 2}, ActionHold, false, 0, false},
		{"large tier activates at +45 (below old +150)", 10, 45, TrailState{LastSize: 10}, ActionHold, true, 45, true},
		{"trailing close at peak*(1-r)", 10, 80, TrailState{PeakPct: 100, LastSize: 10, Active: true}, ActionTrailingClose, true, 0, false},
		{"trailing holds above the line", 10, 85, TrailState{PeakPct: 100, LastSize: 10, Active: true}, ActionHold, true, 100, true},
		{"peak updates upward", 10, 130, TrailState{PeakPct: 100, LastSize: 10, Active: true}, ActionHold, true, 130, true},
		{"catastrophe stop at -300", 5, -300, TrailState{LastSize: 5}, ActionCatastropheStop, false, 0, false},
		{"catastrophe stop beyond -300", 5, -310, TrailState{LastSize: 5}, ActionCatastropheStop, false, 0, false},
		{"no catastrophe at -299", 5, -299, TrailState{LastSize: 5}, ActionHold, false, 0, false},
		{"add rebase keeps active monotonic", 5, 80, TrailState{PeakPct: 180, LastSize: 2, Active: true}, ActionHold, true, 80, true},
		{"add rebase activates when pct>=A", 10, 50, TrailState{LastSize: 0}, ActionHold, true, 50, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			act, st := EvaluateExit(c.size, c.pnlPct, cfg, c.prev)
			if act != c.wantAction {
				t.Fatalf("action = %v, want %v", act, c.wantAction)
			}
			if act == ActionHold && st.Active != c.wantActive {
				t.Fatalf("active = %v, want %v", st.Active, c.wantActive)
			}
			if c.checkPeak && st.PeakPct != c.wantPeak {
				t.Fatalf("peak = %v, want %v", st.PeakPct, c.wantPeak)
			}
			if st.LastSize != c.size {
				t.Fatalf("lastSize = %d, want %d (must track current size)", st.LastSize, c.size)
			}
		})
	}
}

func TestEvaluateExitTierSelection(t *testing.T) {
	cfg := testExitCfg()
	// 小档 size=2 在 +120 不激活（A=150）；中档 size=5 在 +120 激活（A=90）；大档 size=10 在 +50 激活（A=40）
	if _, st := EvaluateExit(2, 120, cfg, TrailState{LastSize: 2}); st.Active {
		t.Error("size=2 small tier (A=150) should not activate at +120")
	}
	if _, st := EvaluateExit(5, 120, cfg, TrailState{LastSize: 5}); !st.Active {
		t.Error("size=5 medium tier (A=90) should activate at +120")
	}
	if _, st := EvaluateExit(10, 50, cfg, TrailState{LastSize: 10}); !st.Active {
		t.Error("size=10 large tier (A=40) should activate at +50")
	}
}

