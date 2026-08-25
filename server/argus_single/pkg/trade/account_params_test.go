package trade

import "testing"

// pickFloat: 账户键有值→账户值；否则全局键有值→全局值；否则默认。
func TestPickFloat(t *testing.T) {
	cases := []struct {
		name                 string
		accRaw               string
		accVal               float64
		globalRaw            string
		globalVal, def, want float64
	}{
		{"account override", "0.15", 0.15, "0.20", 0.20, 0.99, 0.15},
		{"global fallback", "", 0, "0.20", 0.20, 0.99, 0.20},
		{"default when neither set", "", 0, "", 0, 0.99, 0.99},
		{"explicit account 0 honored", "0", 0, "0.20", 0.20, 0.99, 0},
		{"whitespace account treated as unset", "  ", 0, "0.20", 0.20, 0.99, 0.20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickFloat(c.accRaw, c.accVal, c.globalRaw, c.globalVal, c.def); got != c.want {
				t.Fatalf("pickFloat=%v want %v", got, c.want)
			}
		})
	}
}

