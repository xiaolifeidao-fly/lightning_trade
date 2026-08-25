package monitor

import "testing"

func TestBuildExitConfig(t *testing.T) {
	p := TrailParams{
		TierSmallRatio:     0.30,
		TierLargeRatio:     0.65,
		Small:              Tier{ActivatePct: 150, GivebackFrac: 0.35},
		Medium:             Tier{ActivatePct: 90, GivebackFrac: 0.28},
		Large:              Tier{ActivatePct: 40, GivebackFrac: 0.20},
		CatastropheStopPct: 300,
	}

	// N_max=12 -> small=floor(3.6)=3, large=ceil(7.8)=8  => 小1-3/中4-7/大8-12
	c := BuildExitConfig(12, p)
	if c.SmallMaxContracts != 3 || c.LargeMinContracts != 8 {
		t.Fatalf("N=12: got small=%d large=%d, want 3/8", c.SmallMaxContracts, c.LargeMinContracts)
	}
	// N_max=20 -> small=floor(6)=6, large=ceil(13)=13  => 小1-6/中7-12/大13-20
	c2 := BuildExitConfig(20, p)
	if c2.SmallMaxContracts != 6 || c2.LargeMinContracts != 13 {
		t.Fatalf("N=20: got small=%d large=%d, want 6/13", c2.SmallMaxContracts, c2.LargeMinContracts)
	}
	// tiers + catastrophe carried through unchanged
	if c.Small != p.Small || c.Medium != p.Medium || c.Large != p.Large || c.CatastropheStopPct != 300 {
		t.Fatal("tiers / catastrophe not carried into ExitConfig")
	}
}

