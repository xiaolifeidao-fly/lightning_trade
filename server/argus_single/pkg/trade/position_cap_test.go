package trade

import "testing"

func testCapParams() CapParams {
	return CapParams{
		Leverage:           125,
		FaceValue:          0.001,
		RiskBudgetFraction: 0.20,
		CatastropheStopPct: 300,
		Ceiling:            20,
	}
}

func TestComputeMaxContracts(t *testing.T) {
	p := testCapParams()
	cases := []struct {
		name           string
		initialBalance float64
		price          float64
		ceiling        int
		wantN          int
		wantOK         bool
	}{
		{"account2 98.7U -> 12 (floor of 12.65, not 13)", 98.7, 65000, 20, 12, true},
		{"account1 414U formula 53 capped to ceiling 20", 414, 65000, 20, 20, true},
		{"account1 414U no ceiling -> 53", 414, 65000, 0, 53, true},
		{"invalid price 0 -> fail", 98.7, 0, 20, 0, false},
		{"invalid balance 0 -> fail", 0, 65000, 20, 0, false},
		{"tiny balance floors below 1 -> fail", 0.01, 65000, 20, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pp := p
			pp.Ceiling = c.ceiling
			n, ok := ComputeMaxContracts(c.initialBalance, c.price, pp)
			if n != c.wantN || ok != c.wantOK {
				t.Fatalf("ComputeMaxContracts(%v,%v) = (%d,%v), want (%d,%v)",
					c.initialBalance, c.price, n, ok, c.wantN, c.wantOK)
			}
		})
	}
}

func TestPositionCapGuard_EnsureInitAndMaxContracts(t *testing.T) {
	g := NewPositionCapGuard(testCapParams(), nil)
	const acc = "acc2"

	if _, ok := g.MaxContracts(acc); ok {
		t.Fatal("MaxContracts before EnsureInit should be ok=false")
	}

	g.EnsureInit(acc, 98.7, 65000)
	n, ok := g.MaxContracts(acc)
	if n != 12 || !ok {
		t.Fatalf("after init: got (%d,%v), want (12,true)", n, ok)
	}

	// idempotent: first successful init wins, later price does not recompute
	g.EnsureInit(acc, 98.7, 80000)
	if n, _ := g.MaxContracts(acc); n != 12 {
		t.Fatalf("idempotent init: got %d, want 12 (first price wins)", n)
	}
}

func TestPositionCapGuard_FailedInitNotCachedThenHeals(t *testing.T) {
	g := NewPositionCapGuard(testCapParams(), nil)
	const acc = "a"

	g.EnsureInit(acc, 98.7, 0) // bad price -> must not cache
	if _, ok := g.MaxContracts(acc); ok {
		t.Fatal("failed init must not cache (ok should stay false)")
	}

	g.EnsureInit(acc, 98.7, 65000) // good price -> heals
	if n, ok := g.MaxContracts(acc); n != 12 || !ok {
		t.Fatalf("after good price: got (%d,%v), want (12,true)", n, ok)
	}
}

func TestPositionCapGuard_WouldExceedCap(t *testing.T) {
	const acc = "a"
	cases := []struct {
		name        string
		currentSize int
		orderSize   int
		price       float64
		want        bool
	}{
		{"5 + 10 = 15 > 12 -> skip", 5, 10, 65000, true},
		{"2 + 10 = 12 not > 12 -> allow", 2, 10, 65000, false},
		{"short -5: abs(5)+10=15 > 12 -> skip", -5, 10, 65000, true},
		{"fresh 0 + 1 -> allow", 0, 1, 65000, false},
		{"cannot init (price 0) -> fail-closed skip", 0, 1, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := NewPositionCapGuard(testCapParams(), nil)
			got := g.WouldExceedCap(acc, 98.7, c.currentSize, c.orderSize, c.price)
			if got != c.want {
				t.Fatalf("WouldExceedCap(cur=%d,order=%d,price=%v) = %v, want %v",
					c.currentSize, c.orderSize, c.price, got, c.want)
			}
		})
	}
}

