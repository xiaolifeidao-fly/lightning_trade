package trade

import (
	"testing"

	"common/utils"
)

func TestParseNetPosition(t *testing.T) {
	mk := func(inst, side, pos, avg, last string) utils.PositionInfo {
		return utils.PositionInfo{InstId: inst, PosSide: side, Pos: pos, AvgPx: avg, LastPx: last}
	}

	t.Run("valid long", func(t *testing.T) {
		np, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "long", "5", "60000", "60500")}, "BTCUSDT")
		if !ok || np.Side != "long" || np.Size != 5 || np.AvgPx != 60000 || np.LastPx != 60500 {
			t.Fatalf("got %+v ok=%v", np, ok)
		}
	})
	t.Run("valid short negative pos -> abs", func(t *testing.T) {
		np, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "short", "-5", "60000", "59500")}, "BTCUSDT")
		if !ok || np.Side != "short" || np.Size != 5 {
			t.Fatalf("got %+v ok=%v", np, ok)
		}
	})
	t.Run("empty AvgPx -> fail-closed", func(t *testing.T) {
		if _, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "long", "5", "", "60500")}, "BTCUSDT"); ok {
			t.Fatal("empty AvgPx must return ok=false")
		}
	})
	t.Run("bad LastPx -> fail-closed", func(t *testing.T) {
		if _, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "long", "5", "60000", "abc")}, "BTCUSDT"); ok {
			t.Fatal("bad LastPx must return ok=false")
		}
	})
	t.Run("non-positive AvgPx -> fail-closed", func(t *testing.T) {
		if _, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "long", "5", "0", "60500")}, "BTCUSDT"); ok {
			t.Fatal("AvgPx<=0 must return ok=false")
		}
	})
	t.Run("bad Pos -> fail-closed", func(t *testing.T) {
		if _, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "long", "x", "60000", "60500")}, "BTCUSDT"); ok {
			t.Fatal("bad Pos must return ok=false")
		}
	})
	t.Run("no matching instId -> flat ok", func(t *testing.T) {
		np, ok := parseNetPosition([]utils.PositionInfo{mk("ETH-USDT-SWAP", "long", "5", "3000", "3010")}, "BTCUSDT")
		if !ok || np.Size != 0 {
			t.Fatalf("expected flat ok; got %+v ok=%v", np, ok)
		}
	})
	t.Run("zero-size line skipped -> flat", func(t *testing.T) {
		np, ok := parseNetPosition([]utils.PositionInfo{mk("BTC-USDT-SWAP", "long", "0", "60000", "60500")}, "BTCUSDT")
		if !ok || np.Size != 0 {
			t.Fatalf("expected flat ok; got %+v ok=%v", np, ok)
		}
	})
}

