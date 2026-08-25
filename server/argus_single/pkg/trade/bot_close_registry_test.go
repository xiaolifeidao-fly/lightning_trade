package trade

import (
	"testing"
	"time"
)

func TestBotCloseRegistryMarkConsume(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	r := newBotCloseRegistry(clock, 10*time.Minute)

	r.mark(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "p1"))
	if !r.consume(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "p1")) {
		t.Fatal("首次 consume 应命中")
	}
	if r.consume(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "p1")) {
		t.Fatal("二次 consume 不应命中（一次性）")
	}
}

func TestBotCloseRegistryInstIdNormalization(t *testing.T) {
	// trade 侧减仓到 0 用 trade_inst(BTCUSDT) 打标，monitor 对账用 swap inst 消费——必须同键
	r := newBotCloseRegistry(time.Now, 10*time.Minute)
	r.mark(botCloseKey("账户A", "BTCUSDT", "LONG", "p1"))
	if !r.consume(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "p1")) {
		t.Fatal("BTCUSDT 与 BTC-USDT-SWAP 应归一为同键（含大小写）")
	}
	// review fix#2: posId 不同 = 不同仓位实例, 不得互相消费
	r.mark(botCloseKey("账户A", "BTCUSDT", "long", "旧仓"))
	if r.consume(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "新仓")) {
		t.Fatal("旧仓标记不得被新仓消费")
	}
}

func TestBotCloseRegistryTTLAndSweep(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	r := newBotCloseRegistry(clock, 10*time.Minute)

	r.mark(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "p1"))
	now = now.Add(11 * time.Minute) // 过期
	if r.consume(botCloseKey("账户A", "BTC-USDT-SWAP", "long", "p1")) {
		t.Fatal("过期标记不应命中")
	}
	// 过期标记应被后续 mark 顺带清扫（防泄漏）
	r.mark(botCloseKey("账户A", "ETH-USDT-SWAP", "short", "p9"))
	if len(r.marks) != 1 {
		t.Fatalf("过期项应被清扫, marks=%v", r.marks)
	}
	if r.consume(botCloseKey("账户A", "BTC-USDT-SWAP", "short", "p1")) {
		t.Fatal("不同 posSide 不应命中")
	}
}

