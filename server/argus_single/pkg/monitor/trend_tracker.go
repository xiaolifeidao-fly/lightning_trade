package monitor

import (
	"sort"
	"time"
)

// TrendPoint 一个分钟级价格采样。
type TrendPoint struct {
	At time.Time
	Px float64
}

// TrendTracker 趋势闸（8/21 事故复盘）的动量数据源：分钟级采样 close，
// 回答"过去 window 内价格变动了百分之几"。
//
// 语义与回测引擎的 _px_hist 完全对齐（docs/backtest_aug1821_scan3.py 的
// 24h/5% 结论建立在该语义上）：
//   - 参考价 = 窗口边界或更早的最近一个采样（裁剪时保留边界前最后一点）；
//   - 历史覆盖不足一个完整窗口 → ok=false，调用方放行（重启后未回填即此态）。
//
// 非并发安全：调用方（PriceMonitor）在 signalMu 临界区内使用，与
// devSamplers 同一约定，不引入新锁序。
type TrendTracker struct {
	window time.Duration
	points []TrendPoint // 按时间升序；同分钟只保留最新（close 语义）
}

// NewTrendTracker 按窗口构造。
func NewTrendTracker(window time.Duration) *TrendTracker {
	return &TrendTracker{window: window}
}

// Observe 记录一个价格样本。同一分钟内后到覆盖先到（close 语义——tick 流
// ~百次/分钟，不去重会让 24h 窗口膨胀到十万级样本）。非正价格忽略。
func (t *TrendTracker) Observe(now time.Time, px float64) {
	if px <= 0 {
		return
	}
	minute := now.Truncate(time.Minute)
	if n := len(t.points); n > 0 && t.points[n-1].At.Equal(minute) {
		t.points[n-1].Px = px
	} else {
		t.points = append(t.points, TrendPoint{At: minute, Px: px})
	}
	t.prune(minute)
}

// SeedHistory 灌入历史采样（重启回填 K线 close 用）并与既有实时样本合并排序。
// 回填 goroutine 可能晚于首批实时 Observe 到达，因此不能简单 append（会乱序，
// Momentum 的参考价就错了）；同分钟冲突保留实时样本（tick 比 K线 close 新鲜）。
func (t *TrendTracker) SeedHistory(points []TrendPoint) {
	live := make(map[time.Time]bool, len(t.points))
	for _, p := range t.points {
		live[p.At] = true
	}
	merged := make([]TrendPoint, 0, len(points)+len(t.points))
	for _, p := range points {
		m := p.At.Truncate(time.Minute)
		if p.Px <= 0 || live[m] {
			continue
		}
		merged = append(merged, TrendPoint{At: m, Px: p.Px})
	}
	merged = append(merged, t.points...)
	sort.Slice(merged, func(i, j int) bool { return merged[i].At.Before(merged[j].At) })
	t.points = merged
	if n := len(t.points); n > 0 {
		t.prune(t.points[n-1].At)
	}
}

// Momentum 过去一个窗口的价格变动（百分比，+5.0 = +5%）。
// 覆盖不足一个完整窗口时 ok=false（调用方放行）。
func (t *TrendTracker) Momentum(now time.Time) (pct float64, ok bool) {
	if len(t.points) == 0 {
		return 0, false
	}
	ref := t.points[0]
	if ref.At.After(now.Add(-t.window)) {
		return 0, false // 最老样本仍在窗口内 → 覆盖不满
	}
	last := t.points[len(t.points)-1]
	return (last.Px - ref.Px) / ref.Px * 100.0, true
}

// Len 当前样本数（测试与容量观测用）。
func (t *TrendTracker) Len() int { return len(t.points) }

// prune 裁掉窗口边界前多余的样本，只留边界前最后一点作参考价。
func (t *TrendTracker) prune(now time.Time) {
	cut := now.Add(-t.window)
	i := 0
	for i+1 < len(t.points) && !t.points[i+1].At.After(cut) {
		i++
	}
	if i > 0 {
		t.points = append(t.points[:0], t.points[i:]...)
	}
}

// observeTrendLocked 记录一个 tick 的 last 价并返回当前窗口动量。
// 每个 tick 都调（分钟去重在 tracker 内部）；fire 时用返回值填 SignalQuote。
// 调用方须持有 signalMu（读写 trendTrackers），与 devSamplers 同一约定。
func (pm *PriceMonitor) observeTrendLocked(now time.Time, symbol string, last float64) (float64, bool) {
	tr := pm.trendTrackers[symbol]
	if tr == nil {
		tr = NewTrendTracker(pm.trendWindow)
		pm.trendTrackers[symbol] = tr
	}
	tr.Observe(now, last)
	return tr.Momentum(now)
}
