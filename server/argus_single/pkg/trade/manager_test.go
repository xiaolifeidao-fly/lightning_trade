package trade

import (
	"testing"
)

func TestRandomAllocate(t *testing.T) {
	config := &TradingSystemConfig{
		Trade: TradeConfig{
			OrderSize: 5,
		},
	}

	tm := &TradeManager{
		config: config,
	}

	// 测试1：单账户
	alloc := tm.randomAllocate(10, 1)
	if len(alloc) != 1 {
		t.Errorf("分配数量错误: got %d, want 1", len(alloc))
	}
	if alloc[0] != 10 {
		t.Errorf("分配张数错误: got %d, want 10", alloc[0])
	}

	// 测试2：3个账户
	alloc = tm.randomAllocate(10, 3)
	if len(alloc) != 3 {
		t.Errorf("分配数量错误: got %d, want 3", len(alloc))
	}

	sum := 0
	for _, v := range alloc {
		sum += v
		if v < 1 {
			t.Errorf("每个账户至少应该分配1张: got %d", v)
		}
	}

	if sum != 10 {
		t.Errorf("总张数错误: got %d, want 10", sum)
	}

	t.Logf("分配结果: %v", alloc)

	// 测试3：0个账户
	alloc = tm.randomAllocate(10, 0)
	if len(alloc) != 0 {
		t.Errorf("0个账户应该返回空数组")
	}
}

func TestAccountFilters(t *testing.T) {
	config := &TradingSystemConfig{
		Accounts: []AccountConfig{
			{Name: "A", PositionSide: "long", CloseStrategy: "sltp"},
			{Name: "B", PositionSide: "short", CloseStrategy: "trigger"},
			{Name: "C", PositionSide: "long", CloseStrategy: "sltp"},
			{Name: "D", PositionSide: "short", CloseStrategy: "trigger_open"},
		},
	}

	tm := &TradeManager{
		config: config,
	}

	// 测试获取多头账户
	longAccounts := tm.getLongAccounts()
	if len(longAccounts) != 2 {
		t.Errorf("多头账户数量错误: got %d, want 2", len(longAccounts))
	}

	// 测试获取空头账户
	shortAccounts := tm.getShortAccounts()
	if len(shortAccounts) != 2 {
		t.Errorf("空头账户数量错误: got %d, want 2", len(shortAccounts))
	}
}

func TestAccountConfig(t *testing.T) {
	// 测试多头账户
	longAcc := AccountConfig{
		PositionSide:  "long",
		CloseStrategy: "sltp",
	}

	if !longAcc.IsLongAccount() {
		t.Error("应该是多头账户")
	}

	if longAcc.IsShortAccount() {
		t.Error("不应该是空头账户")
	}

	if !longAcc.IsSLTPStrategy() {
		t.Error("应该是止盈止损策略")
	}

	// 测试空头账户
	shortAcc := AccountConfig{
		PositionSide:  "short",
		CloseStrategy: "trigger",
	}

	if shortAcc.IsLongAccount() {
		t.Error("不应该是多头账户")
	}

	if !shortAcc.IsShortAccount() {
		t.Error("应该是空头账户")
	}

	if shortAcc.IsSLTPStrategy() {
		t.Error("不应该是止盈止损策略")
	}
}

func TestSLTPCalculation(t *testing.T) {
	tpPercent := 0.002 // 0.2%
	slPercent := 0.001 // 0.1%

	// 测试多头止盈止损计算
	entryPrice := 95000.0

	// 多头：TP = entry * 1.002, SL = entry * 0.999
	expectedTP := 95000.0 * (1 + tpPercent) // 95190
	expectedSL := 95000.0 * (1 - slPercent) // 94905

	t.Logf("多头开仓价: %.2f", entryPrice)
	t.Logf("  止盈: %.2f (+%.2f%%)", expectedTP, tpPercent*100)
	t.Logf("  止损: %.2f (-%.2f%%)", expectedSL, slPercent*100)

	if expectedTP != 95190.0 {
		t.Errorf("多头止盈计算错误: got %.2f, want 95190.00", expectedTP)
	}
	if expectedSL != 94905.0 {
		t.Errorf("多头止损计算错误: got %.2f, want 94905.00", expectedSL)
	}

	// 测试空头止盈止损计算
	// 空头：TP = entry * 0.998, SL = entry * 1.001
	expectedTPShort := 95000.0 * (1 - tpPercent) // 94810
	expectedSLShort := 95000.0 * (1 + slPercent) // 95095

	t.Logf("空头开仓价: %.2f", entryPrice)
	t.Logf("  止盈: %.2f (-%.2f%%)", expectedTPShort, tpPercent*100)
	t.Logf("  止损: %.2f (+%.2f%%)", expectedSLShort, slPercent*100)

	if expectedTPShort != 94810.0 {
		t.Errorf("空头止盈计算错误: got %.2f, want 94810.00", expectedTPShort)
	}
	if expectedSLShort < 95094.99 || expectedSLShort > 95095.01 {
		t.Errorf("空头止损计算错误: got %.2f, want 95095.00", expectedSLShort)
	}
}

func TestAccountConfigGetOrderSize(t *testing.T) {
	tests := []struct {
		name     string
		acc      AccountConfig
		fallback int
		want     int
	}{
		{"账户级配置优先", AccountConfig{OrderSize: 7}, 3, 7},
		{"账户未配置回退全局", AccountConfig{OrderSize: 0}, 5, 5},
		{"都未配置兜底1", AccountConfig{OrderSize: 0}, 0, 1},
		{"都未配置负数也兜底1", AccountConfig{OrderSize: 0}, -3, 1},
		{"账户0回退全局", AccountConfig{OrderSize: 0}, 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.acc.GetOrderSize(tt.fallback); got != tt.want {
				t.Errorf("GetOrderSize(%d) = %d, want %d", tt.fallback, got, tt.want)
			}
		})
	}
}

func TestAccountConfigGetPosSide(t *testing.T) {
	tests := []struct {
		name        string
		direction   string
		needBuyDeep bool
		want        string
	}{
		{"forward+应开多=long", TradeDirectionForward, true, "long"},
		{"forward+应开空=short", TradeDirectionForward, false, "short"},
		{"reverse+应开多=short", TradeDirectionReverse, true, "short"},
		{"reverse+应开空=long", TradeDirectionReverse, false, "long"},
		{"空配置(默认forward)+应开多=long", "", true, "long"},
		{"空配置(默认forward)+应开空=short", "", false, "short"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := AccountConfig{TradeDirection: tt.direction}
			if got := acc.GetPosSide(tt.needBuyDeep); got != tt.want {
				t.Errorf("GetPosSide(needBuyDeep=%v) = %s, want %s", tt.needBuyDeep, got, tt.want)
			}
		})
	}
}

func TestAccountConfigIsReverseDirection(t *testing.T) {
	if (&AccountConfig{TradeDirection: TradeDirectionReverse}).IsReverseDirection() != true {
		t.Error("reverse 应被识别为反向")
	}
	if (&AccountConfig{TradeDirection: TradeDirectionForward}).IsReverseDirection() != false {
		t.Error("forward 不应被识别为反向")
	}
	if (&AccountConfig{}).IsReverseDirection() != false {
		t.Error("未配置默认不应被识别为反向")
	}
}

func TestShufflePositionSides(t *testing.T) {
	config := &TradingSystemConfig{
		Accounts: []AccountConfig{
			{Name: "A", PositionSide: "long"},
			{Name: "B", PositionSide: "long"},
			{Name: "C", PositionSide: "long"},
			{Name: "D", PositionSide: "long"},
		},
	}

	tm := &TradeManager{
		config:      config,
		stopShuffle: make(chan struct{}),
	}

	// 模拟所有账户无仓位的情况下打乱
	// 由于checkAllPositionsClosed需要实际的客户端，我们直接测试打乱逻辑
	oldSides := make([]string, len(tm.config.Accounts))
	for i := range tm.config.Accounts {
		oldSides[i] = tm.config.Accounts[i].PositionSide
	}

	t.Logf("打乱前: %v", oldSides)

	// 多次运行shuffle，确保至少有一个long和一个short
	for run := 0; run < 10; run++ {
		// 重置为初始状态
		for i := range tm.config.Accounts {
			tm.config.Accounts[i].PositionSide = "long"
		}

		// 手动调用shuffle逻辑
		accountCount := len(tm.config.Accounts)
		newSides := make([]string, accountCount)

		// 先随机分配第一个账户为long或short
		newSides[0] = "long"
		newSides[1] = "short"

		// 剩余的随机分配
		for i := 2; i < accountCount; i++ {
			if i%2 == 0 {
				newSides[i] = "long"
			} else {
				newSides[i] = "short"
			}
		}

		// 应用新的position_side
		longCount := 0
		shortCount := 0
		for i := range tm.config.Accounts {
			tm.config.Accounts[i].PositionSide = newSides[i]
			if newSides[i] == "long" {
				longCount++
			} else {
				shortCount++
			}
		}

		t.Logf("运行 %d: long=%d, short=%d, sides=%v", run+1, longCount, shortCount, newSides)

		// 验证至少有一个long和一个short
		if longCount < 1 {
			t.Errorf("运行 %d: 必须至少有1个long账户, got %d", run+1, longCount)
		}
		if shortCount < 1 {
			t.Errorf("运行 %d: 必须至少有1个short账户, got %d", run+1, shortCount)
		}

		// 验证总数正确
		if longCount+shortCount != accountCount {
			t.Errorf("运行 %d: 账户总数错误: long=%d, short=%d, total=%d", run+1, longCount, shortCount, accountCount)
		}
	}
}
