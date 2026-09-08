package monitor

import (
	"testing"
	"time"

	"argus_single/pkg/marketslice"
)

func fval(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

// 秒内多个 tick 取最后一个（秒收盘口径），且两条流各自落槽互不覆盖。
func TestSliceBufferAggregatesPerSecond(t *testing.T) {
	b := NewSliceBuffer(sliceBufferSeconds)
	b.ObserveQuote(1000, 100, 99)
	b.ObserveBinance(1000, 200)
	b.ObserveQuote(1000, 101, 99.5) // 同一秒后到的 tick 覆盖前一个
	b.ObserveBinance(1000, 201)

	dcLast, dcMark, binLast, dcPoints, binPoints := b.Snapshot(1000, 1)
	if got := fval(dcLast[1]); got != 101 {
		t.Errorf("dcLast 应取该秒最后一个 tick, want 101 got %v", got)
	}
	if got := fval(dcMark[1]); got != 99.5 {
		t.Errorf("dcMark want 99.5 got %v", got)
	}
	if got := fval(binLast[1]); got != 201 {
		t.Errorf("binLast want 201 got %v", got)
	}
	if dcPoints != 1 || binPoints != 1 {
		t.Errorf("覆盖率 want 1/1 got %d/%d", dcPoints, binPoints)
	}
	// 没有 tick 的秒必须是 nil，不能补零也不能插值。
	if dcLast[0] != nil || binLast[2] != nil {
		t.Error("无 tick 的秒应为 nil")
	}
}

// 一条流单独有数据时另一条留空：DC 断流的那几秒币安侧仍应可见。
func TestSliceBufferKeepsStreamsIndependent(t *testing.T) {
	b := NewSliceBuffer(sliceBufferSeconds)
	b.ObserveBinance(2000, 50)
	dcLast, _, binLast, dcPoints, binPoints := b.Snapshot(2000, 0)
	if dcLast[0] != nil || dcPoints != 0 {
		t.Error("只有币安 tick 时 DC 侧应为空")
	}
	if fval(binLast[0]) != 50 || binPoints != 1 {
		t.Errorf("币安侧应独立可见, got %v/%d", fval(binLast[0]), binPoints)
	}
}

// mark<=0 与 DevSampler.Observe 同口径忽略：拿不到标记价就算不出偏离。
func TestSliceBufferIgnoresBadQuote(t *testing.T) {
	b := NewSliceBuffer(sliceBufferSeconds)
	b.ObserveQuote(3000, 100, 0)
	b.ObserveQuote(3000, 0, 100)
	b.ObserveBinance(3000, 0)
	_, _, _, dcPoints, binPoints := b.Snapshot(3000, 0)
	if dcPoints != 0 || binPoints != 0 {
		t.Errorf("坏 tick 不应入槽, got %d/%d", dcPoints, binPoints)
	}
}

// 环形覆盖：容量之外的秒被顶掉，且不会串到别的秒上（取模碰撞必须被 sec 校验拦住）。
func TestSliceBufferRingOverwrites(t *testing.T) {
	b := NewSliceBuffer(marketslice.SeriesPoints) // 最小容量 121
	b.ObserveQuote(0, 100, 100)
	b.ObserveQuote(int64(marketslice.SeriesPoints), 200, 200) // 与第 0 秒同槽
	if s := b.at(0); s != nil {
		t.Error("被顶掉的秒应查不到，不能返回同槽的新数据")
	}
	if s := b.at(int64(marketslice.SeriesPoints)); s == nil || s.dcLast != 200 {
		t.Error("新写入的秒应可查")
	}
}

// 负 epoch 秒（1970 之前，理论值）不能把索引算成负数导致 panic。
func TestSliceBufferHandlesNegativeSeconds(t *testing.T) {
	b := NewSliceBuffer(sliceBufferSeconds)
	b.ObserveQuote(-5, 10, 10)
	if s := b.at(-5); s == nil || s.dcLast != 10 {
		t.Error("负 epoch 秒也应能正确定位")
	}
}

// Snapshot 是只读的：连续两次快照必须给出相同结果（at 不能像 slot 那样顺手清零）。
func TestSliceBufferSnapshotIsReadOnly(t *testing.T) {
	b := NewSliceBuffer(sliceBufferSeconds)
	b.ObserveQuote(4000, 100, 100)
	_, _, _, first, _ := b.Snapshot(4000, 60)
	_, _, _, second, _ := b.Snapshot(4000, 60)
	if first != 1 || second != 1 {
		t.Errorf("快照不应改动缓冲, got %d then %d", first, second)
	}
}

func newSlicePM() *PriceMonitor {
	return &PriceMonitor{
		sliceBuffers:  make(map[string]*SliceBuffer),
		slicePendings: make(map[string][]slicePending),
	}
}

// 到期才落盘：右半窗没凑满时不投递，且同一秒重复触发只登记一次。
func TestArmSliceFlushesOnlyWhenDue(t *testing.T) {
	pm := newSlicePM()
	t0 := time.Unix(1_700_000_000, 0)
	for i := -60; i <= 60; i++ {
		pm.observeSliceQuote(t0.Add(time.Duration(i)*time.Second), "BTCUSDT", 100+float64(i), 100)
		pm.observeSliceBinance(t0.Add(time.Duration(i)*time.Second), "BTCUSDT", 200+float64(i))
	}
	pm.armSlice(t0, "BTCUSDT", "BTCUSDT")
	pm.armSlice(t0.Add(300*time.Millisecond), "BTCUSDT", "BTCUSDT") // 同一秒的第二个账户

	if got := pm.takeDueSlices(t0.Add(59*time.Second), false); len(got) != 0 {
		t.Fatalf("右半窗未满不应落盘, got %d", len(got))
	}
	got := pm.takeDueSlices(t0.Add(61*time.Second), false)
	if len(got) != 1 {
		t.Fatalf("同一秒重复触发只应产出一条, got %d", len(got))
	}
	s := got[0]
	if len(s.DcLast) != marketslice.SeriesPoints || len(s.DcMark) != marketslice.SeriesPoints || len(s.BinLast) != marketslice.SeriesPoints {
		t.Fatalf("三条序列都应是 %d 点, got %d/%d/%d",
			marketslice.SeriesPoints, len(s.DcLast), len(s.DcMark), len(s.BinLast))
	}
	if s.DcPoints != marketslice.SeriesPoints || s.BinPoints != marketslice.SeriesPoints {
		t.Errorf("逐秒都有 tick 时覆盖率应满, got %d/%d", s.DcPoints, s.BinPoints)
	}
	if !s.Anchor.Equal(t0) || !s.StartAt.Equal(t0.Add(-60*time.Second)) || !s.EndAt.Equal(t0.Add(60*time.Second)) {
		t.Errorf("窗口边界不对: anchor=%v start=%v end=%v", s.Anchor, s.StartAt, s.EndAt)
	}
	// 第 0 点是 anchor-60 秒那一刻的 last=40
	if fval(s.DcLast[0]) != 40 || fval(s.DcLast[marketslice.SeriesPoints-1]) != 160 {
		t.Errorf("序列首末点位对不上: %v / %v", fval(s.DcLast[0]), fval(s.DcLast[marketslice.SeriesPoints-1]))
	}
	if len(pm.takeDueSlices(t0.Add(120*time.Second), false)) != 0 {
		t.Error("落盘后待办应清空")
	}
}

// 收尾（进程退出 / 配置热替换）时把未满窗的切片也交出去，覆盖率如实反映缺口。
func TestTakeDueSlicesForceEmitsPartial(t *testing.T) {
	pm := newSlicePM()
	t0 := time.Unix(1_700_000_500, 0)
	pm.observeSliceQuote(t0, "BTCUSDT", 100, 100)
	pm.armSlice(t0, "BTCUSDT", "BTCUSDT")

	got := pm.takeDueSlices(t0.Add(time.Second), true)
	if len(got) != 1 {
		t.Fatalf("force 应无视到期时间, got %d", len(got))
	}
	if got[0].DcPoints != 1 || got[0].BinPoints != 0 {
		t.Errorf("覆盖率应如实反映只有一秒有 tick, got %d/%d", got[0].DcPoints, got[0].BinPoints)
	}
}

// 多币种各自独立：一个币种到期不应带出另一个币种的切片。
func TestSlicePendingsPerSymbol(t *testing.T) {
	pm := newSlicePM()
	t0 := time.Unix(1_700_001_000, 0)
	pm.observeSliceQuote(t0, "BTCUSDT", 100, 100)
	pm.observeSliceQuote(t0.Add(30*time.Second), "ETHUSDT", 10, 10)
	pm.armSlice(t0, "BTCUSDT", "BTCUSDT")
	pm.armSlice(t0.Add(30*time.Second), "ETHUSDT", "ETHUSDT")

	got := pm.takeDueSlices(t0.Add(61*time.Second), false)
	if len(got) != 1 || got[0].InstIdRaw != "BTCUSDT" {
		t.Fatalf("只应落 BTCUSDT 那条, got %+v", got)
	}
	if len(pm.slicePendings["ETHUSDT"]) != 1 {
		t.Error("ETHUSDT 的待办应仍在")
	}
}

// 直接构造的 PriceMonitor（部分单测）没有 map，切片链路必须静默关闭而不是 panic。
func TestSliceWiringNoMapsIsSilent(t *testing.T) {
	pm := &PriceMonitor{}
	pm.observeSliceQuote(time.Now(), "BTCUSDT", 100, 100)
	pm.observeSliceBinance(time.Now(), "BTCUSDT", 100)
	pm.armSlice(time.Now(), "BTCUSDT", "BTCUSDT")
	if got := pm.takeDueSlices(time.Now(), true); len(got) != 0 {
		t.Errorf("无 map 时不应产出切片, got %d", len(got))
	}
}
