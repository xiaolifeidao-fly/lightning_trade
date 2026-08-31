package monitor

import (
	"math"
	"testing"

	"argus_single/pkg/eventlog"
)

// mark 固定取 63000，用 bp 偏移构造 last，便于表述"第 N 个 tick 偏离 X bp"。
const devTestMark = 63000.0

func devTickAt(bp float64) float64 { return devTestMark * (1 + bp/10000) }

// observeBps 依次喂入一串偏离（bp）并 flush。
func observeBps(s *DevSampler, bps ...float64) (eventlog.Event, bool) {
	for _, bp := range bps {
		s.Observe(devTickAt(bp), devTestMark)
	}
	return s.Flush()
}

// 同一方向持续超阈只算一次穿越——镜像 handleOrderBookSignal 的 edge-trigger：
// lastDerivedSignal 相同则不再派发信号。
func TestDevSamplerCountsSameDirectionBurstOnce(t *testing.T) {
	s := NewDevSampler([]float64{5})

	ev, _ := observeBps(s, 6, 7, 8, 6)

	if got := ev.DevCross["5"]; got != 1 {
		t.Errorf("同向连续 4 个超阈 tick 应只算 1 次穿越，got %d", got)
	}
	if ev.DevTicks != 4 {
		t.Errorf("devTicks 应记满全部 tick，want 4 got %d", ev.DevTicks)
	}
}

// 回到带内会 re-arm（生产规则里 |dev|<threshold 时 lastDerivedSignal 被清空），
// 之后同方向再次超阈算新的一次穿越。这是 6/25 高信号密度的机制。
func TestDevSamplerReArmsAfterReturningInBand(t *testing.T) {
	s := NewDevSampler([]float64{5})

	ev, _ := observeBps(s, 6, 1, 6)

	if got := ev.DevCross["5"]; got != 2 {
		t.Errorf("超阈→带内→再超阈应算 2 次穿越，got %d", got)
	}
}

// 反向不需要先回到带内：生产规则只在方向与 lastDerivedSignal 相同时抑制，
// 所以 UP 直接翻到 DOWN 会立刻派发。用 bool armed 实现会把这里漏计成 1。
func TestDevSamplerCountsDirectionFlipWithoutReturningInBand(t *testing.T) {
	s := NewDevSampler([]float64{5})

	ev, _ := observeBps(s, 6, -6)

	if got := ev.DevCross["5"]; got != 2 {
		t.Errorf("UP 直接翻 DOWN（未回带内）应算 2 次穿越，got %d", got)
	}
}

// 每个候选阈值各自独立判定——这正是"阈值若取 θ，λ 会是多少"的测量。
func TestDevSamplerThresholdsAreIndependent(t *testing.T) {
	s := NewDevSampler([]float64{1, 2, 3, 4, 5})

	ev, _ := observeBps(s, 3)

	for _, key := range []string{"1", "2", "3"} {
		if got := ev.DevCross[key]; got != 1 {
			t.Errorf("3bp 偏离应穿越阈值 %s，want 1 got %d", key, got)
		}
	}
	for _, key := range []string{"4", "5"} {
		if got := ev.DevCross[key]; got != 0 {
			t.Errorf("3bp 偏离不应穿越阈值 %s，want 0 got %d", key, got)
		}
	}
}

// devOver 与 devCross 语义不同：前者是超阈 tick 数（→ 分布的生存函数
// P(|dev|>θ)），后者是穿越次数（→ λ(θ)）。同向 burst 里两者必然分叉。
func TestDevSamplerOverCountsEveryTickWhileCrossCountsTransitions(t *testing.T) {
	s := NewDevSampler([]float64{5})

	ev, _ := observeBps(s, 6, 7, 8, 6)

	if got := ev.DevOver["5"]; got != 4 {
		t.Errorf("devOver 应逐 tick 计数，want 4 got %d", got)
	}
	if got := ev.DevCross["5"]; got != 1 {
		t.Errorf("devCross 应只计穿越，want 1 got %d", got)
	}
}

// mark<=0 无法计算偏离，整条 tick 忽略（与 NewSignalQuote 的"不得落 0 冒充观测"同口径）。
func TestDevSamplerIgnoresTickWithNonPositiveMark(t *testing.T) {
	s := NewDevSampler([]float64{5})

	s.Observe(63000, 0)
	s.Observe(63000, -1)

	if s.Ticks() != 0 {
		t.Errorf("mark<=0 的 tick 不应计入，got %d", s.Ticks())
	}
	if _, ok := s.Flush(); ok {
		t.Error("无有效 tick 时不应产出事件")
	}
}

// 无 tick 不写空事件——否则日志里会出现无法与"真零穿越"区分的噪声行。
func TestDevSamplerFlushReportsNothingWithoutTicks(t *testing.T) {
	s := NewDevSampler([]float64{5})

	if _, ok := s.Flush(); ok {
		t.Error("零 tick 时 Flush 应返回 ok=false")
	}
}

// Flush 只清零计数，不清零 edge-trigger 状态。用反向的第二窗口来区分两者：
// 方向不同不受抑制，因此能干净地断言"计数没有跨窗口累积"。
func TestDevSamplerFlushResetsCounters(t *testing.T) {
	s := NewDevSampler([]float64{5})

	observeBps(s, 6, 7)
	ev, ok := observeBps(s, -6)

	if !ok {
		t.Fatal("第二个窗口应产出事件")
	}
	if ev.DevTicks != 1 {
		t.Errorf("第二个窗口应只含自己的 tick，want 1 got %d", ev.DevTicks)
	}
	if got := ev.DevCross["5"]; got != 1 {
		t.Errorf("穿越计数不应跨窗口累积，want 1 got %d", got)
	}
}

// edge-trigger 状态必须跨窗口保留：偏离是连续过程，一段持续同向的偏离在生产规则
// 下只触发一次信号。若窗口边界重置方向状态，它会在每个窗口各计一次穿越，λ 被
// 系统性夸大——这会直接毁掉 5bp 等价性自检。
func TestDevSamplerKeepsEdgeTriggerStateAcrossWindows(t *testing.T) {
	s := NewDevSampler([]float64{5})

	observeBps(s, 6)          // 窗口1：UP 穿越
	ev, _ := observeBps(s, 7) // 窗口2：仍是 UP，未回带内

	if got := ev.DevCross["5"]; got != 0 {
		t.Errorf("跨窗口的同向持续偏离不应再计穿越，want 0 got %d", got)
	}
	if ev.DevOver["5"] != 1 {
		t.Errorf("但超阈 tick 仍应计入，want 1 got %d", ev.DevOver["5"])
	}
}

// 幅度摘要用绝对值（方向已由 devCross 承载），是做市优化进展的最直接指标。
func TestDevSamplerReportsAbsoluteMagnitudeSummary(t *testing.T) {
	s := NewDevSampler([]float64{5})

	ev, _ := observeBps(s, 2, -8, 4, -2)

	if math.Abs(ev.DevMaxBp-8) > 1e-6 {
		t.Errorf("devMaxBp 应取 |dev| 极值，want 8 got %v", ev.DevMaxBp)
	}
	if math.Abs(ev.DevMeanBp-4) > 1e-6 { // (2+8+4+2)/4
		t.Errorf("devMeanBp 应取 |dev| 均值，want 4 got %v", ev.DevMeanBp)
	}
}
