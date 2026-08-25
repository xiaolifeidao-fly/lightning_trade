package monitor

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"common/utils"
)

// 身份对账（设计 v3）：只认 posId。探针已证实 posId 在加/减仓间稳定、平仓重开必换，
// 是唯一可靠的强身份；时间字段一律禁用（CTime 在净仓 long→short 切换时不重置，
// 会把旧仓峰值错套到新仓 → 新仓开出来就被误平，正是 R1 要防的失效模式）。
// 一切存疑不恢复：错误恢复比不恢复更糟。
func TestReconcileTrailStates(t *testing.T) {
	const kLong = "账户A:BTC-USDT-SWAP:long"
	const kShort = "账户A:BTC-USDT-SWAP:short"

	tests := []struct {
		name        string
		persisted   map[string]persistedTrail
		live        map[string]string // key → 实盘当前 posId
		wantRestore map[string]TrailState
		wantDropped []string
	}{
		{
			name:        "posId 一致: 恢复峰值",
			persisted:   map[string]persistedTrail{kLong: {PosId: "p1", PeakPct: 78.5, LastSize: 6, Active: true}},
			live:        map[string]string{kLong: "p1"},
			wantRestore: map[string]TrailState{kLong: {PeakPct: 78.5, LastSize: 6, Active: true}},
		},
		{
			name:        "posId 不同: 丢弃(平仓后重开的新仓)",
			persisted:   map[string]persistedTrail{kLong: {PosId: "p1", PeakPct: 78.5, Active: true}},
			live:        map[string]string{kLong: "p2"},
			wantRestore: map[string]TrailState{},
			wantDropped: []string{kLong},
		},
		{
			name:        "实盘无此仓: 丢弃",
			persisted:   map[string]persistedTrail{kLong: {PosId: "p1", PeakPct: 78.5, Active: true}},
			live:        map[string]string{},
			wantRestore: map[string]TrailState{},
			wantDropped: []string{kLong},
		},
		{
			name:        "落盘 posId 为空: 无强身份, 丢弃",
			persisted:   map[string]persistedTrail{kLong: {PosId: "", PeakPct: 78.5, Active: true}},
			live:        map[string]string{kLong: "p1"},
			wantRestore: map[string]TrailState{},
			wantDropped: []string{kLong},
		},
		{
			name:        "实盘 posId 为空: 无法验证, 丢弃",
			persisted:   map[string]persistedTrail{kLong: {PosId: "p1", PeakPct: 78.5, Active: true}},
			live:        map[string]string{kLong: ""},
			wantRestore: map[string]TrailState{},
			wantDropped: []string{kLong},
		},
		{
			name: "多仓混合: 逐条独立判定",
			persisted: map[string]persistedTrail{
				kLong:  {PosId: "p1", PeakPct: 78.5, LastSize: 6, Active: true},
				kShort: {PosId: "p9", PeakPct: 40.0, LastSize: 3, Active: true},
			},
			live:        map[string]string{kLong: "p1", kShort: "pX"},
			wantRestore: map[string]TrailState{kLong: {PeakPct: 78.5, LastSize: 6, Active: true}},
			wantDropped: []string{kShort},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dropped := reconcileTrailStates(tt.persisted, tt.live)
			if len(got) != len(tt.wantRestore) {
				t.Fatalf("恢复条数 %d, want %d (got=%+v)", len(got), len(tt.wantRestore), got)
			}
			for k, want := range tt.wantRestore {
				if got[k] != want {
					t.Fatalf("key %s: got %+v, want %+v", k, got[k], want)
				}
			}
			sort.Strings(dropped)
			sort.Strings(tt.wantDropped)
			if len(dropped) != len(tt.wantDropped) {
				t.Fatalf("丢弃 %v, want %v", dropped, tt.wantDropped)
			}
			for i := range dropped {
				if dropped[i] != tt.wantDropped[i] {
					t.Fatalf("丢弃 %v, want %v", dropped, tt.wantDropped)
				}
			}
		})
	}
}

// 落盘数据由 trailStates(峰值) 与 snapshots(posId) 拼装；缺 posId 的不落盘
// ——落了也恢复不了(对账必然丢弃)，徒增噪音。
func TestBuildPersistedTrails(t *testing.T) {
	am := &AccountMonitor{
		trailStates: map[string]TrailState{
			"账户A:BTC-USDT-SWAP:long":  {PeakPct: 78.5, LastSize: 10, Active: true},
			"账户B:BTC-USDT-SWAP:short": {PeakPct: 40.0, LastSize: 6, Active: true},
			"账户C:BTC-USDT-SWAP:long":  {PeakPct: 12.0, LastSize: 1, Active: false},
		},
		snapshots: map[string]posSnapshot{
			"账户A:BTC-USDT-SWAP:long":  {PosId: "p-A", Size: 10},
			"账户B:BTC-USDT-SWAP:short": {PosId: "", Size: 6}, // 无强身份
			// 账户C 无快照
		},
	}
	got := am.buildPersistedTrails()
	if len(got) != 1 {
		t.Fatalf("仅账户A 有强身份, got %d 条: %+v", len(got), got)
	}
	a := got["账户A:BTC-USDT-SWAP:long"]
	if a.PosId != "p-A" || a.PeakPct != 78.5 || a.LastSize != 10 || !a.Active {
		t.Fatalf("账户A 落盘内容: %+v", a)
	}
}

// 恢复对账只接受 live 仓位的 posId：Pos=0 是确已平掉的仓，其 posId 不得参与对账
// （否则交易所在平仓后仍回带旧 posId 时会误判"身份一致"而复活死仓的峰值）。
func TestLivePosIds(t *testing.T) {
	got := livePosIds("账户A", []utils.PositionInfo{
		{InstId: "BTC-USDT-SWAP", PosSide: "long", Pos: "10", PosId: "p-live"},
		{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "0", PosId: "p-dead"},  // 已平
		{InstId: "ETH-USDT-SWAP", PosSide: "long", Pos: "", PosId: "p-unknown"}, // 瞬时脏数据
		{InstId: "", PosSide: "long", Pos: "3", PosId: "p-nokey"},               // 无法构成 key
		{InstId: "SOL-USDT-SWAP", PosSide: "long", Pos: "2", PosId: ""},         // 无 posId
	})
	if got["账户A:BTC-USDT-SWAP:long"] != "p-live" {
		t.Fatalf("live 仓应收录: %+v", got)
	}
	if _, ok := got["账户A:BTC-USDT-SWAP:short"]; ok {
		t.Fatalf("Pos=0 的死仓不得参与对账: %+v", got)
	}
	// unknown（Pos 空）保守收录：与 buildLiveKeys 的三态口径一致，宁可多留一轮
	if got["账户A:ETH-USDT-SWAP:long"] != "p-unknown" {
		t.Fatalf("unknown 态应与 buildLiveKeys 口径一致(保守保留): %+v", got)
	}
	if len(got) != 3 { // live + unknown + 空posId 那条（值为空，对账时自会丢弃）
		t.Fatalf("收录条数: %+v", got)
	}
}

// 恢复按账户在该账户首轮持仓轮询时进行（此时实盘 posId 已在手，无需额外 API 调用）；
// 消费后不得重复恢复——否则 GC 掉的状态会被反复复活。
func TestRestoreAccountTrailsConsumesOnce(t *testing.T) {
	am := &AccountMonitor{
		trailStates: map[string]TrailState{},
		pendingRestore: map[string]persistedTrail{
			"账户A:BTC-USDT-SWAP:long":  {PosId: "p-A", PeakPct: 78.5, LastSize: 10, Active: true},
			"账户A:BTC-USDT-SWAP:short": {PosId: "旧仓", PeakPct: 55.0, Active: true},
			"账户B:BTC-USDT-SWAP:long":  {PosId: "p-B", PeakPct: 30.0, LastSize: 2, Active: true},
		},
	}
	// 账户A 首轮：long 的 posId 对上、short 已换仓
	am.restoreAccountTrails("账户A", map[string]string{
		"账户A:BTC-USDT-SWAP:long":  "p-A",
		"账户A:BTC-USDT-SWAP:short": "新仓",
	})
	if st := am.trailStates["账户A:BTC-USDT-SWAP:long"]; st.PeakPct != 78.5 || !st.Active {
		t.Fatalf("posId 一致应恢复: %+v", st)
	}
	if _, ok := am.trailStates["账户A:BTC-USDT-SWAP:short"]; ok {
		t.Fatalf("posId 不同不得恢复（旧峰值套新仓会立即误平）")
	}
	// 账户B 的待恢复项不受影响
	if _, ok := am.pendingRestore["账户B:BTC-USDT-SWAP:long"]; !ok {
		t.Fatalf("不得消费其他账户的待恢复项")
	}
	// 账户A 的项已消费
	for k := range am.pendingRestore {
		if strings.HasPrefix(k, "账户A:") {
			t.Fatalf("账户A 待恢复项应已消费, 残留 %s", k)
		}
	}
	// 二次调用不得复活已被 GC 的状态
	delete(am.trailStates, "账户A:BTC-USDT-SWAP:long")
	am.restoreAccountTrails("账户A", map[string]string{"账户A:BTC-USDT-SWAP:long": "p-A"})
	if _, ok := am.trailStates["账户A:BTC-USDT-SWAP:long"]; ok {
		t.Fatalf("已消费的恢复项不得二次复活")
	}
}

func TestTrailStateFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "trail_state.json") // 目录不存在也应能落盘
	states := map[string]persistedTrail{
		"账户A:BTC-USDT-SWAP:long": {PosId: "1001124331810473", PeakPct: 78.54, LastSize: 10, Active: true},
	}
	if err := saveTrailStateFile(path, states); err != nil {
		t.Fatalf("保存失败: %v", err)
	}
	back, err := loadTrailStateFile(path)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if len(back) != 1 || back["账户A:BTC-USDT-SWAP:long"] != states["账户A:BTC-USDT-SWAP:long"] {
		t.Fatalf("round-trip 不一致: %+v", back)
	}
}

// 损坏/缺失/版本不符一律冷启动（= 现行为），不得让进程起不来。
func TestTrailStateFileDegradesToColdStart(t *testing.T) {
	dir := t.TempDir()

	if got, err := loadTrailStateFile(filepath.Join(dir, "不存在.json")); err != nil || got != nil {
		t.Fatalf("文件不存在应冷启动: got=%+v err=%v", got, err)
	}

	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{不是合法json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := loadTrailStateFile(broken); got != nil {
		t.Fatalf("文件损坏应冷启动, got=%+v", got)
	}

	future := filepath.Join(dir, "future.json")
	if err := os.WriteFile(future, []byte(`{"schemaVersion":999,"states":{"k":{"posId":"p1","peakPct":50}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadTrailStateFile(future)
	if got != nil {
		t.Fatalf("schemaVersion 不匹配应拒载, got=%+v", got)
	}
	if err == nil {
		t.Fatalf("版本不符应返回可打日志的原因")
	}
}

// 端到端：flush（Stop 时同步落盘）→ 重启载入 → 首轮 posId 对账恢复。
// 覆盖设计里"SIGTERM 同步 flush 后重启零丢失"的核心链路（不含信号管道本身）。
func TestFlushThenRestartRestoresPeak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trail_state.json")
	const key = "账户A:BTC-USDT-SWAP:short"

	// 旧进程：持有一个已激活、峰值 78.54% 的仓
	old := &AccountMonitor{
		trailStatePath: path,
		trailStates:    map[string]TrailState{key: {PeakPct: 78.54, LastSize: 6, Active: true}},
		snapshots:      map[string]posSnapshot{key: {PosId: "1001124331810473", Size: 6}},
	}
	old.flushTrailStates(true) // Stop() 里做的事

	// 新进程：载入 + 首轮轮询对账
	fresh := &AccountMonitor{
		trailStatePath: path,
		trailStates:    map[string]TrailState{},
	}
	states, err := loadTrailStateFile(path)
	if err != nil || len(states) != 1 {
		t.Fatalf("重启载入失败: states=%+v err=%v", states, err)
	}
	fresh.pendingRestore = states
	fresh.restoreAccountTrails("账户A", livePosIds("账户A", []utils.PositionInfo{
		{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "-6", PosId: "1001124331810473"},
	}))

	got := fresh.trailStates[key]
	if got.PeakPct != 78.54 || got.LastSize != 6 || !got.Active {
		t.Fatalf("重启后峰值保护应完整恢复, got %+v", got)
	}

	// 同一份快照, 若实盘已换仓（posId 变了）则必须拒绝恢复
	other := &AccountMonitor{trailStatePath: path, trailStates: map[string]TrailState{}, pendingRestore: states}
	other.restoreAccountTrails("账户A", livePosIds("账户A", []utils.PositionInfo{
		{InstId: "BTC-USDT-SWAP", PosSide: "short", Pos: "-6", PosId: "换仓后的新ID"},
	}))
	if _, ok := other.trailStates[key]; ok {
		t.Fatalf("换仓后不得恢复旧峰值（会让新仓开出来就被误平）")
	}
}

// 原子写：临时文件 + rename，中途崩溃不得留下半截文件覆盖旧快照。
func TestSaveTrailStateFileIsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trail_state.json")
	if err := saveTrailStateFile(path, map[string]persistedTrail{"k1": {PosId: "p1", PeakPct: 10}}); err != nil {
		t.Fatal(err)
	}
	if err := saveTrailStateFile(path, map[string]persistedTrail{"k2": {PosId: "p2", PeakPct: 20}}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "trail_state.json" {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("不得残留临时文件, 目录内容: %v", names)
	}
	back, _ := loadTrailStateFile(path)
	if len(back) != 1 || back["k2"].PosId != "p2" {
		t.Fatalf("第二次写入应完整覆盖: %+v", back)
	}
}

