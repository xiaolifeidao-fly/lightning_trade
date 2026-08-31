package monitor

import (
	"testing"
	"time"

	"argus_single/pkg/eventlog"
)

// 采样按窗口节流：若每个 tick 都产出事件，日志会从 1440 条/天涨到数十万条。
// 直接构造 PriceMonitor（不走 NewPriceMonitor）以避开配置与调度器依赖——
// observeDeviationLocked 只用到这两个 map。
func TestObserveDeviationFlushesOncePerInterval(t *testing.T) {
	pm := &PriceMonitor{
		devSamplers:  make(map[string]*DevSampler),
		lastDevFlush: make(map[string]time.Time),
	}
	cfg := SymbolConfig{TradeInst: "BTCUSDT", SignalThreshold: 0.0005}
	t0 := time.Date(2026, 8, 8, 10, 0, 0, 0, time.Local)

	if _, ok := pm.observeDeviationLocked(t0, "BTCUSDT", cfg, devTickAt(6), devTestMark); ok {
		t.Error("首个 tick 只起窗口，不应产出事件")
	}
	if _, ok := pm.observeDeviationLocked(t0.Add(30*time.Second), "BTCUSDT", cfg, devTickAt(1), devTestMark); ok {
		t.Error("窗口未满不应产出事件")
	}

	ev, ok := pm.observeDeviationLocked(t0.Add(61*time.Second), "BTCUSDT", cfg, devTickAt(7), devTestMark)
	if !ok {
		t.Fatal("窗口满应产出事件")
	}
	if ev.Event != eventlog.EvDevSample {
		t.Errorf("事件类型 want %q got %q", eventlog.EvDevSample, ev.Event)
	}
	if ev.InstId != "BTCUSDT" {
		t.Errorf("instId 应用信号侧口径 BTCUSDT（避免与持仓侧 BTC-USDT-SWAP 分裂），got %q", ev.InstId)
	}
	if ev.DevTicks != 3 {
		t.Errorf("窗口内三个 tick 都应计入，want 3 got %d", ev.DevTicks)
	}
	// 6bp→1bp→7bp：回带内后再超阈，两次穿越
	if got := ev.DevCross["5"]; got != 2 {
		t.Errorf("穿越计数 want 2 got %d", got)
	}

	if _, ok := pm.observeDeviationLocked(t0.Add(62*time.Second), "BTCUSDT", cfg, devTickAt(7), devTestMark); ok {
		t.Error("flush 后应开启新窗口，不应立即再产出")
	}
}

// 多币种各自独立计窗口与状态（当前线上只有 BTCUSDT，但 symbolConfigs 是 map）。
func TestObserveDeviationKeepsSymbolsIndependent(t *testing.T) {
	pm := &PriceMonitor{
		devSamplers:  make(map[string]*DevSampler),
		lastDevFlush: make(map[string]time.Time),
	}
	btc := SymbolConfig{TradeInst: "BTCUSDT", SignalThreshold: 0.0005}
	eth := SymbolConfig{TradeInst: "ETHUSDT", SignalThreshold: 0.0005}
	t0 := time.Date(2026, 8, 8, 10, 0, 0, 0, time.Local)

	pm.observeDeviationLocked(t0, "BTCUSDT", btc, devTickAt(6), devTestMark)
	pm.observeDeviationLocked(t0, "ETHUSDT", eth, devTickAt(6), devTestMark)
	pm.observeDeviationLocked(t0.Add(61*time.Second), "BTCUSDT", btc, devTickAt(6), devTestMark)

	ev, ok := pm.observeDeviationLocked(t0.Add(61*time.Second), "ETHUSDT", eth, devTickAt(6), devTestMark)
	if !ok {
		t.Fatal("ETHUSDT 自己的窗口也应到期")
	}
	if ev.InstId != "ETHUSDT" {
		t.Errorf("instId want ETHUSDT got %q", ev.InstId)
	}
	if ev.DevTicks != 2 {
		t.Errorf("ETHUSDT 应只含自己的 tick，want 2 got %d", ev.DevTicks)
	}
}
