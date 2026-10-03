package eventstore

import (
	"sync"
	"testing"
	"time"

	"gorm.io/gorm/schema"
)

func TestNextBackoffDoublesAndCaps(t *testing.T) {
	seq := []time.Duration{nextBackoff(0)}
	for i := 0; i < 6; i++ {
		seq = append(seq, nextBackoff(seq[len(seq)-1]))
	}
	want := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 10 * time.Minute, 10 * time.Minute, 10 * time.Minute}
	for i := range want {
		if seq[i] != want[i] {
			t.Fatalf("第 %d 次间隔 = %s, 期望 %s（序列 %v）", i, seq[i], want[i], seq)
		}
	}
}

// 10-03 回归：event_hash 用 uniqueIndex 且没写 size，gorm AutoMigrate 比 unique/长度永远不等，
// 每次启动都发 MODIFY COLUMN。三张表的哈希列必须 size=16 且 field.Unique=true。
func TestEventHashColumnsDeclareSizeMatchingType(t *testing.T) {
	checked := 0
	for _, model := range Models() {
		sch, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatal(err)
		}
		f := sch.LookUpField("EventHash")
		if f == nil {
			continue // signal_slice 等没有幂等哈希列的表
		}
		checked++
		if f.Size != 16 || string(f.DataType) != "binary(16)" || !f.Unique {
			t.Errorf("%s.event_hash size=%d type=%s unique=%v, 期望 16 / binary(16) / true", sch.Table, f.Size, f.DataType, f.Unique)
		}
	}
	if checked != 3 {
		t.Fatalf("应检查 3 张带 event_hash 的表，实际 %d", checked)
	}
}
