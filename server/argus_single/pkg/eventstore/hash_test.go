package eventstore

import (
	"encoding/hex"
	"testing"

	"argus_single/pkg/eventlog"
)

func sampleOpen() eventlog.Event {
	return eventlog.Event{
		Ts: "2026-08-21 14:03:07", Account: "账户A-1394537246@qq.com", Variant: "champion/S400_cap26_gate8",
		InstId: "BTCUSDT", Event: eventlog.EvOpen, Side: "short", Size: 12, OrderSize: 1,
		SigLast: 113001.5, SigMark: 112945.2, GapBp: 5.01,
	}
}

func TestEventHashStableAndSized(t *testing.T) {
	h1, ok := EventHash("argus-single-1", sampleOpen())
	if !ok || len(h1) != 16 {
		t.Fatalf("哈希必须是 16 字节 BINARY(16)，得到 ok=%v len=%d", ok, len(h1))
	}
	h2, _ := EventHash("argus-single-1", sampleOpen())
	if hex.EncodeToString(h1) != hex.EncodeToString(h2) {
		t.Fatalf("同一事件两次计算必须一致")
	}
}

// 实例1 与实例3 各有一个 account1，是两个不同的真实账户：
// instanceKey 必须进哈希，否则同秒同内容的两条真事件会被判成重复行。
func TestEventHashSeparatesInstances(t *testing.T) {
	a, _ := EventHash("argus-single-1", sampleOpen())
	b, _ := EventHash("argus-single-ives", sampleOpen())
	if hex.EncodeToString(a) == hex.EncodeToString(b) {
		t.Fatalf("不同实例的同内容事件不得撞哈希")
	}
}

func TestEventHashSensitiveToEveryField(t *testing.T) {
	base, _ := EventHash("argus-single-1", sampleOpen())
	mutations := map[string]func(*eventlog.Event){
		"ts":        func(e *eventlog.Event) { e.Ts = "2026-08-21 14:03:08" },
		"account":   func(e *eventlog.Event) { e.Account = "账户B-mortypeng@gmail.com" },
		"variant":   func(e *eventlog.Event) { e.Variant = "challenger/S400_cap8_gate8" },
		"event":     func(e *eventlog.Event) { e.Event = eventlog.EvCapSkip },
		"size":      func(e *eventlog.Event) { e.Size = 13 },
		"orderSize": func(e *eventlog.Event) { e.OrderSize = 2 },
		"gapBp":     func(e *eventlog.Event) { e.GapBp = 5.02 },
		"reason":    func(e *eventlog.Event) { e.Reason = "减仓锁利(pnl为估算)" },
	}
	for name, mutate := range mutations {
		e := sampleOpen()
		mutate(&e)
		got, _ := EventHash("argus-single-1", e)
		if hex.EncodeToString(got) == hex.EncodeToString(base) {
			t.Fatalf("字段 %s 变化后哈希未变，会静默丢事件", name)
		}
	}
}

// 回灌路径拿到的是 ParseFile 反序列化出的同一个结构体：
// JSONL 行 → Event → 哈希，必须与直写路径逐字节一致。
func TestEventHashIdenticalAcrossLiveAndBackfill(t *testing.T) {
	live := sampleOpen()
	line := eventlog.Marshal(live)

	var parsed eventlog.Event
	if err := unmarshalLine(line, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	liveHash, _ := EventHash("argus-single-1", live)
	backfillHash, _ := EventHash("argus-single-1", parsed)
	if hex.EncodeToString(liveHash) != hex.EncodeToString(backfillHash) {
		t.Fatalf("直写与回灌哈希不一致，回灌会插出重复行")
	}
}

// devCross/devOver 是 map，哈希必须与遍历顺序无关（encoding/json 按 key 排序）。
func TestEventHashStableForMaps(t *testing.T) {
	e := eventlog.Event{
		Ts: "2026-08-21 14:03:07", Event: eventlog.EvDevSample, InstId: "BTCUSDT", DevTicks: 97,
		DevCross: map[string]int{"5": 2, "3": 7, "10": 1, "1": 31},
		DevOver:  map[string]int{"5": 4, "3": 12},
	}
	first, _ := EventHash("argus-single-1", e)
	for i := 0; i < 50; i++ {
		got, _ := EventHash("argus-single-1", e)
		if hex.EncodeToString(got) != hex.EncodeToString(first) {
			t.Fatalf("map 字段导致哈希不稳定（第 %d 次）", i)
		}
	}
}
