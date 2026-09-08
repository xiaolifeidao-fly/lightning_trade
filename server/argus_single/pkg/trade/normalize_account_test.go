package trade

import "testing"

func TestNormalizeAccountConfigKeepsSignalLogic(t *testing.T) {
	account := AccountConfig{Index: 1, TradeLogic: "signal", PositionMode: "bidirectional"}
	NormalizeAccountConfig(&account)

	if !account.IsSignalLogic() {
		t.Fatalf("trade_logic = %q, want the signal branch to stay enabled", account.TradeLogic)
	}
	if account.PositionMode != "net" {
		t.Fatalf("position_mode = %q, want net for signal logic", account.PositionMode)
	}
}

// 空 trade_logic 会被归一化成 spread —— 这正是 DB 驱动下盘口信号策略静默失效的
// 机理。这条用例把它钉住，避免有人以为空值等于 signal。
func TestNormalizeAccountConfigTreatsEmptyTradeLogicAsSpread(t *testing.T) {
	account := AccountConfig{Index: 1}
	NormalizeAccountConfig(&account)

	if !account.IsSpreadLogic() {
		t.Fatalf("trade_logic = %q, want spread", account.TradeLogic)
	}
}

func TestNormalizeAccountConfigDefaultsStopLossMode(t *testing.T) {
	account := AccountConfig{Index: 1}
	NormalizeAccountConfig(&account)
	if account.StopLossMode != StopLossModeCatastrophic {
		t.Fatalf("stop_loss_mode = %q, want %q", account.StopLossMode, StopLossModeCatastrophic)
	}

	unsupported := AccountConfig{Index: 1, StopLossMode: "tight"}
	NormalizeAccountConfig(&unsupported)
	if unsupported.StopLossMode != StopLossModeCatastrophic {
		t.Fatalf("stop_loss_mode = %q, want the unsupported value to fall back", unsupported.StopLossMode)
	}
}

func TestNormalizeAccountConfigFallsBackOnIllegalTradeDirection(t *testing.T) {
	account := AccountConfig{Index: 1, TradeDirection: "sideways"}
	NormalizeAccountConfig(&account)
	if account.TradeDirection != TradeDirectionForward {
		t.Fatalf("trade_direction = %q, want %q", account.TradeDirection, TradeDirectionForward)
	}
}
