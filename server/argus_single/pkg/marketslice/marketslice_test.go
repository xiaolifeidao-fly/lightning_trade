package marketslice

import (
	"testing"
	"time"
)

type recordingSink struct{ got []Slice }

func (r *recordingSink) EmitSlice(s Slice) { r.got = append(r.got, s) }

type panickingSink struct{}

func (panickingSink) EmitSlice(Slice) { panic("boom") }

// 半窗与点数是切片契约的核心常量：读侧按 offset+half 定位下标，改一个不改另一个
// 会让整条曲线错位。
func TestSeriesPointsMatchesHalfWindow(t *testing.T) {
	if SeriesPoints != 2*HalfWindowSeconds+1 {
		t.Fatalf("SeriesPoints(%d) 必须等于 2*HalfWindowSeconds+1(%d)", SeriesPoints, 2*HalfWindowSeconds+1)
	}
	if SeriesPoints != 121 {
		t.Fatalf("需求口径是 ±60 秒共 121 点, got %d", SeriesPoints)
	}
}

// 多 sink 按注册顺序投递；ResetSinks 之后不再投递。
func TestRegisterAndResetSinks(t *testing.T) {
	t.Cleanup(ResetSinks)
	ResetSinks()
	a, b := &recordingSink{}, &recordingSink{}
	RegisterSink(a)
	RegisterSink(nil) // nil 忽略，不能把它塞进列表后在投递时 panic
	RegisterSink(b)
	if SinkCount() != 2 {
		t.Fatalf("nil sink 不应入列, got %d", SinkCount())
	}
	Emit(Slice{Anchor: time.Unix(1, 0)})
	if len(a.got) != 1 || len(b.got) != 1 {
		t.Fatalf("两个 sink 都应收到, got %d/%d", len(a.got), len(b.got))
	}
	ResetSinks()
	Emit(Slice{Anchor: time.Unix(2, 0)})
	if len(a.got) != 1 {
		t.Error("Reset 之后不应再投递")
	}
}

// 单个 sink panic 不得带崩行情链路，也不得让后面的 sink 收不到。
func TestEmitIsolatesSinkPanic(t *testing.T) {
	t.Cleanup(ResetSinks)
	ResetSinks()
	RegisterSink(panickingSink{})
	good := &recordingSink{}
	RegisterSink(good)
	Emit(Slice{Anchor: time.Unix(3, 0)})
	if len(good.got) != 1 {
		t.Fatal("前一个 sink panic 后，后面的 sink 仍必须收到")
	}
}
