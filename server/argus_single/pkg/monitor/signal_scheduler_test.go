package monitor

import (
	"testing"
	"time"

	"argus_single/pkg/trade"
)

// A1: fire() 必须只在"确有账户开仓"时进入 POSITION；全跳过/全失败 → IDLE
func TestNextStateAfterFire(t *testing.T) {
	cases := []struct {
		opened bool
		dir    SignalDirection
		want   SignalState
	}{
		{true, SignalDirectionUp, SignalStateLongPosition},
		{true, SignalDirectionDown, SignalStateShortPosition},
		{false, SignalDirectionUp, SignalStateIdle}, // 全跳过 → IDLE（不漂移成 POSITION）
		{false, SignalDirectionDown, SignalStateIdle},
	}
	for _, c := range cases {
		if got := nextStateAfterFire(c.opened, c.dir); got != c.want {
			t.Errorf("nextStateAfterFire(%v,%s)=%s want %s", c.opened, c.dir, got, c.want)
		}
	}
}

// P7：信号报价随快照传递的语义。
// 关键点是按 symbol 隔离——延迟 5s 期间另一 symbol 的信号不得污染本 symbol 的报价。
func TestOnSignalCarriesQuote(t *testing.T) {
	s := NewSignalScheduler(time.Hour) // 长延迟：只观察状态，不触发 fire
	cfgA := SymbolConfig{TradeInst: "BTCUSDT", DeepInst: "BTC-USDT-SWAP"}
	cfgB := SymbolConfig{TradeInst: "ETHUSDT", DeepInst: "ETH-USDT-SWAP"}

	s.OnSignal("BTCUSDT", cfgA, SignalDirectionUp, 65032.5, "src", trade.NewSignalQuote(65032.5, 65000))
	if q := s.states["BTCUSDT"].Quote; q.GapBp < 4.99 || q.GapBp > 5.01 || q.Mark != 65000 {
		t.Fatalf("首个信号报价未记录: %+v", q)
	}

	// 另一 symbol 的信号不得污染
	s.OnSignal("ETHUSDT", cfgB, SignalDirectionDown, 3000, "src", trade.NewSignalQuote(3000, 3300))
	if q := s.states["BTCUSDT"].Quote; q.Mark != 65000 {
		t.Fatalf("跨 symbol 串扰: %+v", q)
	}

	// 同向叠加：与 Price 同语义，一并刷新为最新（fire 用的就是最后一个信号价）
	s.OnSignal("BTCUSDT", cfgA, SignalDirectionUp, 65130, "src", trade.NewSignalQuote(65130, 65000))
	st := s.states["BTCUSDT"]
	if st.SignalCount != 2 {
		t.Fatalf("应为同向叠加, count=%d", st.SignalCount)
	}
	if st.Price != 65130 || st.Quote.Last != 65130 {
		t.Fatalf("Quote 须与 Price 同步刷新: price=%v quote=%+v", st.Price, st.Quote)
	}
	if st.Quote.GapBp < 19.9 || st.Quote.GapBp > 20.1 {
		t.Fatalf("GapBp 应随之更新到 20bp: %+v", st.Quote)
	}

	// 显式信号无 mark → 零值快照，事件侧整体省略而非落 0
	s.OnSignal("BTCUSDT", cfgA, SignalDirectionDown, 64000, "explicit", trade.SignalQuote{})
	if s.states["BTCUSDT"].Quote.OK() {
		t.Fatalf("显式信号无 mark, 报价应不可用")
	}
}

