package monitor

import (
	"testing"
	"time"
)

// TrendTracker 是趋势闸（8/21 事故复盘）的动量数据源：分钟级采样 close，
// 提供"过去 window 内价格变动百分比"。语义与回测引擎 _px_hist 对齐：
// 参考价 = 窗口边界或更早的最近一个采样；覆盖不足一个完整窗口 → ok=false（放行）。

var trendT0 = time.Date(2026, 8, 20, 12, 0, 0, 0, time.Local)

func feedFlat(t *TrendTracker, start time.Time, minutes int, px float64) {
	for i := 0; i < minutes; i++ {
		t.Observe(start.Add(time.Duration(i)*time.Minute), px)
	}
}

func TestTrendTrackerMomentumAfterFullWindow(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	feedFlat(tr, trendT0, 60, 63000)                      // 前 1h 平盘
	feedFlat(tr, trendT0.Add(time.Hour), 61, 63000*1.035) // 后 1h+ 涨 3.5%
	mom, ok := tr.Momentum(trendT0.Add(2 * time.Hour))
	if !ok {
		t.Fatal("覆盖满窗后应可判定")
	}
	if mom < 3.4 || mom > 3.6 {
		t.Errorf("动量应 ≈ +3.5%%, got %.3f", mom)
	}
}

func TestTrendTrackerInsufficientHistory(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	feedFlat(tr, trendT0, 30, 63000) // 只有 30 分钟
	if _, ok := tr.Momentum(trendT0.Add(30 * time.Minute)); ok {
		t.Error("历史不足一个窗口时应返回 ok=false（放行语义）")
	}
}

func TestTrendTrackerEmptyPasses(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	if _, ok := tr.Momentum(trendT0); ok {
		t.Error("零样本应 ok=false")
	}
}

func TestTrendTrackerSameMinuteKeepsLatest(t *testing.T) {
	// close 语义：同一分钟内取最后一个价（tick 流 ~117/分钟，必须去重防内存爆炸）
	tr := NewTrendTracker(2 * time.Hour)
	feedFlat(tr, trendT0, 121, 63000)
	base := trendT0.Add(121 * time.Minute)
	tr.Observe(base, 70000)
	tr.Observe(base.Add(10*time.Second), 63000*1.02) // 同分钟覆盖
	mom, ok := tr.Momentum(base.Add(30 * time.Second))
	if !ok {
		t.Fatal("满窗应可判定")
	}
	if mom < 1.9 || mom > 2.1 {
		t.Errorf("同分钟应取最新价（+2%%), got %.3f", mom)
	}
}

func TestTrendTrackerNegativeMomentum(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	feedFlat(tr, trendT0, 60, 63000)
	feedFlat(tr, trendT0.Add(time.Hour), 61, 63000*0.96) // 跌 4%
	mom, ok := tr.Momentum(trendT0.Add(2 * time.Hour))
	if !ok || mom > -3.9 || mom < -4.1 {
		t.Errorf("动量应 ≈ -4%%, got %.3f ok=%v", mom, ok)
	}
}

func TestTrendTrackerPrunesOldPoints(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	feedFlat(tr, trendT0, 60*10, 63000) // 喂 10 小时
	if n := tr.Len(); n > 122 {
		t.Errorf("窗口 2h 只应保留 ~121 个点（边界前 1 个 + 窗口内），got %d", n)
	}
}

func TestTrendTrackerSeedHistory(t *testing.T) {
	// 重启回填：一次性灌入历史 K线 close，之后立即可判定
	tr := NewTrendTracker(2 * time.Hour)
	pts := make([]TrendPoint, 0, 121)
	for i := 0; i <= 120; i++ {
		pts = append(pts, TrendPoint{At: trendT0.Add(time.Duration(i) * time.Minute), Px: 63000})
	}
	tr.SeedHistory(pts)
	tr.Observe(trendT0.Add(121*time.Minute), 63000*1.05)
	mom, ok := tr.Momentum(trendT0.Add(121 * time.Minute))
	if !ok || mom < 4.9 || mom > 5.1 {
		t.Errorf("回填后应立即可判定 +5%%, got %.3f ok=%v", mom, ok)
	}
}

func TestTrendTrackerIgnoresNonPositivePrice(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	tr.Observe(trendT0, 0)
	tr.Observe(trendT0.Add(time.Minute), -1)
	if tr.Len() != 0 {
		t.Errorf("非正价格不应入样，got %d", tr.Len())
	}
}

// ---------- PriceMonitor 接线 ----------

// 每个 tick 喂 observeTrendLocked（signalMu 临界区内，与 devSampler 同约定）；
// fire 时用返回的动量填 SignalQuote。多 symbol 各自独立。
func TestObserveTrendLockedPerSymbol(t *testing.T) {
	pm := &PriceMonitor{trendTrackers: make(map[string]*TrendTracker), trendWindow: 2 * time.Hour}
	for i := 0; i <= 120; i++ {
		pm.observeTrendLocked(trendT0.Add(time.Duration(i)*time.Minute), "BTCUSDT", 63000)
	}
	mom, ok := pm.observeTrendLocked(trendT0.Add(121*time.Minute), "BTCUSDT", 63000*1.035)
	if !ok || mom < 3.4 || mom > 3.6 {
		t.Errorf("满窗后应返回 +3.5%%, got %.3f ok=%v", mom, ok)
	}
	if _, ok := pm.observeTrendLocked(trendT0.Add(121*time.Minute), "ETHUSDT", 3000); ok {
		t.Error("ETHUSDT 无历史，应 ok=false（symbol 间独立）")
	}
}

// ---------- 重启回填 ----------

// 回填 goroutine 可能晚于实时 Observe 到达：SeedHistory 必须能把更早的历史
// 合并到已有实时样本之前（简单 append 会乱序，Momentum 的 ref 就错了）。
func TestSeedHistoryMergesBeforeExistingObservations(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	tr.Observe(trendT0.Add(121*time.Minute), 63000*1.05) // 实时样本先到
	pts := make([]TrendPoint, 0, 121)
	for i := 0; i <= 120; i++ {
		pts = append(pts, TrendPoint{At: trendT0.Add(time.Duration(i) * time.Minute), Px: 63000})
	}
	tr.SeedHistory(pts) // 历史后到
	mom, ok := tr.Momentum(trendT0.Add(121 * time.Minute))
	if !ok || mom < 4.9 || mom > 5.1 {
		t.Errorf("乱序回填后动量应 +5%%, got %.3f ok=%v", mom, ok)
	}
}

// 同分钟冲突时保留实时样本（tick 比 K线 close 新鲜）。
func TestSeedHistoryKeepsLiveSampleOnSameMinute(t *testing.T) {
	tr := NewTrendTracker(2 * time.Hour)
	live := trendT0.Add(121 * time.Minute)
	tr.Observe(live, 66150)
	pts := []TrendPoint{{At: live, Px: 99999}}
	for i := 0; i <= 120; i++ {
		pts = append(pts, TrendPoint{At: trendT0.Add(time.Duration(i) * time.Minute), Px: 63000})
	}
	tr.SeedHistory(pts)
	mom, _ := tr.Momentum(live)
	if mom < 4.9 || mom > 5.1 {
		t.Errorf("同分钟应保留实时样本 66150 (+5%%), got %.3f", mom)
	}
}
