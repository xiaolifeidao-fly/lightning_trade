package monitor

import (
	"testing"
	"time"
)

// 趋势条件止损的动量来源：持仓监控要按 pos.InstId（"BTC-USDT-SWAP"）读到
// 价格监控里那个按 symbol（"BTCUSDT"）建的 TrendTracker。两边的键不同名，
// 映射走 SymbolConfig.DeepInst。
//
// 只读是硬要求：持仓轮询**不得**往趋势窗口里喂价。喂了就等于让 1 分钟一次的
// 持仓轮询污染 tick 级的动量序列，闸门与止损看到的会是两条不同的曲线。

var wireT0 = time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)

func seededMonitor(t *testing.T) *PriceMonitor {
	t.Helper()
	pm := &PriceMonitor{
		symbolConfigs: map[string]SymbolConfig{
			"BTCUSDT": {DeepInst: "BTC-USDT-SWAP", TradeInst: "BTCUSDT"},
		},
		trendTrackers: make(map[string]*TrendTracker),
		trendWindow:   2 * time.Hour,
	}
	for i := 0; i <= 120; i++ {
		pm.observeTrendLocked(wireT0.Add(time.Duration(i)*time.Minute), "BTCUSDT", 77000)
	}
	pm.observeTrendLocked(wireT0.Add(121*time.Minute), "BTCUSDT", 77000*1.04)
	return pm
}

func TestTrendMomentumForInstMatchesByDeepInst(t *testing.T) {
	pm := seededMonitor(t)
	mom, ok := pm.TrendMomentumForInst("BTC-USDT-SWAP", wireT0.Add(121*time.Minute))
	if !ok {
		t.Fatal("已满窗且 DeepInst 对得上，应能取到动量")
	}
	if mom < 3.9 || mom > 4.1 {
		t.Fatalf("动量应约 +4%%，得到 %.3f", mom)
	}
}

func TestTrendMomentumForInstIsCaseInsensitive(t *testing.T) {
	pm := seededMonitor(t)
	if _, ok := pm.TrendMomentumForInst("btc-usdt-swap", wireT0.Add(121*time.Minute)); !ok {
		t.Fatal("交易所回的 instId 大小写不保证，应按 EqualFold 匹配")
	}
}

func TestTrendMomentumForInstUnknownInst(t *testing.T) {
	pm := seededMonitor(t)
	if _, ok := pm.TrendMomentumForInst("ETH-USDT-SWAP", wireT0.Add(121*time.Minute)); ok {
		t.Fatal("未配置的 instId 必须返回 ok=false（fail-safe，不收紧）")
	}
}

func TestTrendMomentumForInstDoesNotCreateTracker(t *testing.T) {
	// 配了 symbol 但 tracker 还没建（刚热替换、回填未完成）：只读，绝不建。
	pm := &PriceMonitor{
		symbolConfigs: map[string]SymbolConfig{"BTCUSDT": {DeepInst: "BTC-USDT-SWAP"}},
		trendTrackers: make(map[string]*TrendTracker),
		trendWindow:   2 * time.Hour,
	}
	if _, ok := pm.TrendMomentumForInst("BTC-USDT-SWAP", wireT0); ok {
		t.Fatal("tracker 不存在应返回 ok=false")
	}
	if len(pm.trendTrackers) != 0 {
		t.Fatalf("只读访问不得创建 tracker，现在有 %d 个", len(pm.trendTrackers))
	}
}

func TestTrendMomentumForInstDoesNotFeedPrices(t *testing.T) {
	pm := seededMonitor(t)
	before := pm.trendTrackers["BTCUSDT"].Len()
	for i := 0; i < 5; i++ {
		pm.TrendMomentumForInst("BTC-USDT-SWAP", wireT0.Add(time.Duration(200+i)*time.Minute))
	}
	// 只读不喂价：点数只可能因为 prune 变少，绝不会变多。
	if after := pm.trendTrackers["BTCUSDT"].Len(); after > before {
		t.Fatalf("只读访问却把点数从 %d 涨到了 %d——持仓轮询污染了动量序列", before, after)
	}
}

func TestCurrentTrendMomentumFailsSafeWithoutMonitor(t *testing.T) {
	// 全局监控器还没建 / 正在热替换的空窗：拿不到动量就别收紧。
	saved := globalMonitor
	globalMonitor = nil
	defer func() { globalMonitor = saved }()
	if _, ok := currentTrendMomentum("BTC-USDT-SWAP", wireT0); ok {
		t.Fatal("无全局监控器时必须 ok=false")
	}
}
