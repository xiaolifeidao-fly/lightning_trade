package argus_event

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"service/argus_event/repository"
)

func seriesJSON(t *testing.T, base float64, holes ...int) string {
	t.Helper()
	skip := map[int]bool{}
	for _, h := range holes {
		skip[h] = true
	}
	series := make([]*float64, 121)
	for i := range series {
		if skip[i] {
			continue
		}
		v := base + float64(i)
		series[i] = &v
	}
	raw, err := json.Marshal(series)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func sliceRowFixture(t *testing.T, anchor string) *repository.SignalSliceRow {
	t.Helper()
	return &repository.SignalSliceRow{
		Ts:            anchor,
		InstanceKey:   "argus-single-1",
		Instrument:    "BTCUSDT",
		HalfWindowSec: 60,
		SeriesPoints:  121,
		DcPoints:      120,
		BinPoints:     121,
		DcLastJson:    seriesJSON(t, 100, 3),
		DcMarkJson:    seriesJSON(t, 100, 3),
		BinLastJson:   seriesJSON(t, 200),
	}
}

// 逐秒切片展开成 121 点，offsetSec 连续、按秒对位，空洞留 null 不补零。
func TestBuildStoredSlicePointsExpandsFullWindow(t *testing.T) {
	anchor := "2026-08-21 14:03:07"
	originTs, err := parseEventTime(anchor, false)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// 事件比锚点晚 5 秒（trade.signal.delay_seconds 默认值）。
	eventTs := originTs.Add(5 * time.Second)
	events := []*repository.StrategyEventRow{
		{Ts: formatEventTime(eventTs), Event: "open"},
		{Ts: formatEventTime(eventTs), Event: "cap_skip"},
	}

	points := buildStoredSlicePoints(sliceRowFixture(t, anchor), originTs, 60, eventTs, events)
	if len(points) != 121 {
		t.Fatalf("应有 121 点, got %d", len(points))
	}
	if points[0].OffsetSec != -60 || points[120].OffsetSec != 60 {
		t.Errorf("offsetSec 首末应是 -60/60, got %d/%d", points[0].OffsetSec, points[120].OffsetSec)
	}
	if points[0].DcLast == nil || *points[0].DcLast != 100 {
		t.Errorf("序列首点应对位到窗口左界, got %v", points[0].DcLast)
	}
	if points[0].BinLast == nil || *points[0].BinLast != 200 {
		t.Errorf("币安序列应一起带出, got %v", points[0].BinLast)
	}
	if points[3].DcLast != nil || points[3].GapBp != nil {
		t.Error("断流的那一秒必须留空，不能补零也不能插值")
	}
	if points[3].BinLast == nil {
		t.Error("DC 断流不应连带抹掉币安侧点位")
	}
	// gapBp 由该秒的 last/mark 现算：base 相同 ⇒ 偏离 0。
	if points[0].GapBp == nil || *points[0].GapBp != 0 {
		t.Errorf("gapBp 应由该秒两个价位现算, got %v", points[0].GapBp)
	}
	// 触发行是事件所在的那一秒（offset=+5），不是 offset==0。
	trigger := -1
	for i, p := range points {
		if p.IsTriggerTs {
			if trigger >= 0 {
				t.Fatal("只应有一个触发行")
			}
			trigger = i
		}
	}
	if trigger != 65 {
		t.Fatalf("触发行应落在 offset=+5（第 65 点）, got %d", trigger)
	}
	if len(points[65].Events) != 2 {
		t.Errorf("该秒的全部事件类型都应列出, got %v", points[65].Events)
	}
}

// 请求窗口比切片窄时按下标裁，不能整段错位。
func TestBuildStoredSlicePointsHonorsNarrowWindow(t *testing.T) {
	anchor := "2026-08-21 14:03:07"
	originTs, _ := parseEventTime(anchor, false)
	points := buildStoredSlicePoints(sliceRowFixture(t, anchor), originTs, 5, originTs, nil)
	if len(points) != 11 {
		t.Fatalf("±5 秒应有 11 点, got %d", len(points))
	}
	// 窄窗第 0 点对应 offset=-5 ⇒ 序列下标 55 ⇒ 值 155。
	if points[0].DcLast == nil || *points[0].DcLast != 155 {
		t.Errorf("窄窗对位错了, got %v", points[0].DcLast)
	}
	if points[5].OffsetSec != 0 || points[5].DcLast == nil || *points[5].DcLast != 160 {
		t.Errorf("窄窗中心应是锚点那一秒, got %+v", points[5])
	}
}

// 序列 JSON 损坏时该序列整段留空，绝不用 0 冒充价格，也不能让接口报错。
func TestDecodeSeriesTolerantToBadJSON(t *testing.T) {
	if got := decodeSeries("not-json"); got != nil {
		t.Errorf("坏 JSON 应返回 nil, got %v", got)
	}
	if got := decodeSeries(""); got != nil {
		t.Errorf("空串应返回 nil, got %v", got)
	}
}

// mark<=0 时不算偏离（与 trade.NewSignalQuote 同口径）。
func TestGapBpOfIgnoresBadMark(t *testing.T) {
	last, zero, mark := 100.0, 0.0, 99.0
	if gapBpOf(&last, &zero) != nil {
		t.Error("mark=0 时不应给出 gapBp")
	}
	if gapBpOf(nil, &mark) != nil {
		t.Error("last 缺失时不应给出 gapBp")
	}
	got := gapBpOf(&last, &mark)
	if got == nil {
		t.Fatal("正常值应算出 gapBp")
	}
	// 用容差比：编译期常量折叠是高精度求值，与运行时浮点算出的位不一定相同。
	if want := 101.0101; math.Abs(*got-want) > 1e-4 {
		t.Errorf("gapBp want ~%.4f got %.6f", want, *got)
	}
}
