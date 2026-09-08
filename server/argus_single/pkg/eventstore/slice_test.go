package eventstore

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"argus_single/pkg/marketslice"
)

func sampleSlice(anchor time.Time) marketslice.Slice {
	n := marketslice.SeriesPoints
	dcLast := make([]*float64, n)
	dcMark := make([]*float64, n)
	binLast := make([]*float64, n)
	for i := 0; i < n; i++ {
		if i == 3 {
			continue // 留一个空洞：断流的那一秒必须序列化成 null
		}
		last, mark, bin := 100+float64(i), 100.0, 200+float64(i)
		dcLast[i], dcMark[i], binLast[i] = &last, &mark, &bin
	}
	return marketslice.Slice{
		Anchor:     anchor,
		InstIdRaw:  "BTCUSDT",
		StartAt:    anchor.Add(-marketslice.HalfWindowSeconds * time.Second),
		EndAt:      anchor.Add(marketslice.HalfWindowSeconds * time.Second),
		HalfWindow: marketslice.HalfWindowSeconds,
		DcLast:     dcLast,
		DcMark:     dcMark,
		BinLast:    binLast,
		DcPoints:   n - 1,
		BinPoints:  n - 1,
	}
}

// 一条切片 = 一行 + 三条等长 JSON 序列（各 121 点），实例维度在入队时刻钉住。
func TestConvertSliceProducesOneRowWithThreeSeries(t *testing.T) {
	anchor := time.Date(2026, 8, 21, 14, 3, 7, 0, time.Local)
	ingestedAt := time.Date(2026, 8, 21, 14, 4, 8, 0, time.Local)
	rows, kind := Convert(Envelope{
		InstanceKey:   "argus-single-1",
		ConfigVersion: 12,
		Slice:         ptrSlice(sampleSlice(anchor)),
	}, ingestedAt)
	if kind != ErrNone {
		t.Fatalf("转换失败: kind=%v", kind)
	}
	if rows.Len() != 1 || len(rows.Slice) != 1 {
		t.Fatalf("应只产出一行 signal_slice, got %+v", rows)
	}
	row := rows.Slice[0]
	if row.InstanceKey != "argus-single-1" || row.ConfigVersion != 12 || row.Source != SourceLive {
		t.Errorf("实例维度没落上: %+v", row)
	}
	if row.Instrument != "BTCUSDT" || row.InstIdRaw == nil || *row.InstIdRaw != "BTCUSDT" {
		t.Errorf("合约口径不对: instrument=%q raw=%v", row.Instrument, row.InstIdRaw)
	}
	if !row.Ts.Equal(anchor) || !row.StartAt.Equal(anchor.Add(-60*time.Second)) || !row.EndAt.Equal(anchor.Add(60*time.Second)) {
		t.Errorf("窗口边界不对: ts=%v start=%v end=%v", row.Ts, row.StartAt, row.EndAt)
	}
	if row.HalfWindowSec != 60 || row.SeriesPoints != marketslice.SeriesPoints {
		t.Errorf("窗口元数据不对: half=%d points=%d", row.HalfWindowSec, row.SeriesPoints)
	}
	if row.DcPoints != marketslice.SeriesPoints-1 || row.BinPoints != marketslice.SeriesPoints-1 {
		t.Errorf("覆盖率不对: %d/%d", row.DcPoints, row.BinPoints)
	}
	for name, raw := range map[string]string{"dc_last": row.DcLastJson, "dc_mark": row.DcMarkJson, "bin_last": row.BinLastJson} {
		var series []*float64
		if err := json.Unmarshal([]byte(raw), &series); err != nil {
			t.Fatalf("%s 不是合法 JSON: %v", name, err)
		}
		if len(series) != marketslice.SeriesPoints {
			t.Errorf("%s 应有 %d 点, got %d", name, marketslice.SeriesPoints, len(series))
		}
		if series[3] != nil {
			t.Errorf("%s 断流的那一秒应是 null，不能补零", name)
		}
	}
	if !strings.Contains(row.DcLastJson, "null") {
		t.Error("空洞必须序列化成 null，让读侧看得见断流")
	}
}

// 锚点按秒截断：切片要能与秒精度的 strategy_event.ts 对上。
func TestConvertSliceTruncatesAnchorToSecond(t *testing.T) {
	anchor := time.Date(2026, 8, 21, 14, 3, 7, 987_000_000, time.Local)
	row, ok := ConvertSlice(Envelope{InstanceKey: "argus-single-1", Slice: ptrSlice(sampleSlice(anchor))}, time.Now())
	if !ok {
		t.Fatal("转换应成功")
	}
	if row.Ts.Nanosecond() != 0 {
		t.Errorf("锚点应截断到秒, got %v", row.Ts)
	}
}

// 零值锚点是坏数据，必须拒掉而不是插一行 ts=0 的垃圾。
func TestConvertSliceRejectsZeroAnchor(t *testing.T) {
	if _, kind := Convert(Envelope{InstanceKey: "argus-single-1", Slice: &marketslice.Slice{}}, time.Now()); kind != ErrBadTs {
		t.Errorf("零值锚点应判为 ErrBadTs, got %v", kind)
	}
}

// EmitSlice 与 Emit 同规矩：队列满时立刻丢弃并计数，绝不阻塞行情 goroutine。
func TestEmitSliceDropsInsteadOfBlocking(t *testing.T) {
	s := newStore(dryRunDB(t), Options{InstanceKey: "argus-single-1", QueueSize: 1})
	s.SetConfigVersion(5)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 4; i++ {
			s.EmitSlice(sampleSlice(time.Now()))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("EmitSlice 阻塞了——行情链路会被写库拖住")
	}
	stats := s.Stats()
	if stats.Accepted != 1 || stats.Dropped != 3 {
		t.Fatalf("accepted=%d dropped=%d, want 1/3", stats.Accepted, stats.Dropped)
	}
	env := <-s.queue
	if env.Slice == nil || env.ConfigVersion != 5 || env.InstanceKey != "argus-single-1" {
		t.Fatalf("入队时刻未钉住实例维度: %+v", env)
	}
}

// nil Store 上 EmitSlice 必须安全：Setup 失败时采集器仍在跑并投递。
func TestNilStoreEmitSliceIsSafe(t *testing.T) {
	var s *Store
	s.EmitSlice(sampleSlice(time.Now()))
	if _, err := s.PurgeExpiredSlices(time.Now()); err != nil {
		t.Errorf("nil Store 上清理应静默返回, got %v", err)
	}
	s.StartSlicePurge(nil)
}

// signal_slice 必须进 Models()，否则两侧 AutoMigrate 都建不出这张表。
func TestModelsIncludesSignalSlice(t *testing.T) {
	for _, m := range Models() {
		if _, ok := m.(*SignalSlice); ok {
			return
		}
	}
	t.Fatal("Models() 里没有 SignalSlice")
}

func ptrSlice(s marketslice.Slice) *marketslice.Slice { return &s }
