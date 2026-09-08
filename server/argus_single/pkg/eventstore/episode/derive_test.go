package episode

import (
	"math"
	"reflect"
	"testing"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
)

const (
	testInstance = "it-episode"
	testAccount  = "账户A-测试"
	testSymbol   = "BTCUSDT"
)

var rebuiltAt = time.Date(2026, 9, 2, 10, 0, 0, 0, time.Local)

// stream 按顺序造事件，id 自增以模拟落库顺序（同秒事件的先后只能靠 id）。
type stream struct {
	rows []*eventstore.StrategyEvent
	id   uint64
}

func (s *stream) push(ts, event string, mods ...func(*eventstore.StrategyEvent)) *eventstore.StrategyEvent {
	parsed, err := eventstore.ParseTs(ts)
	if err != nil {
		panic(err)
	}
	s.id++
	row := &eventstore.StrategyEvent{
		Id:           s.id,
		EventHash:    []byte{byte(s.id), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		Ts:           parsed,
		InstanceKey:  testInstance,
		AccountLabel: testAccount,
		Instrument:   testSymbol,
		Event:        event,
		Variant:      strPtr("champion/default"),
	}
	for _, mod := range mods {
		mod(row)
	}
	s.rows = append(s.rows, row)
	return row
}

// open 一条加仓/减仓事件：side 是这一单的方向，size 是**结果净仓**快照。
func open(side string, size, orderSize int) func(*eventstore.StrategyEvent) {
	return func(row *eventstore.StrategyEvent) {
		row.Side = strPtr(side)
		row.OrderSize = intPtr(orderSize)
		if size != 0 {
			row.Size = intPtr(size)
		}
	}
}

func withPnl(roi, pnl float64) func(*eventstore.StrategyEvent) {
	return func(row *eventstore.StrategyEvent) {
		row.RoiPct = floatPtr(roi)
		row.Pnl = floatPtr(pnl)
	}
}

func closing(side string, size int) func(*eventstore.StrategyEvent) {
	return func(row *eventstore.StrategyEvent) {
		row.Side = strPtr(side)
		row.Size = intPtr(size)
	}
}

func withRoi(roi float64) func(*eventstore.StrategyEvent) {
	return func(row *eventstore.StrategyEvent) { row.RoiPct = floatPtr(roi) }
}

func withVariant(v string) func(*eventstore.StrategyEvent) {
	return func(row *eventstore.StrategyEvent) { row.Variant = strPtr(v) }
}

func strPtr(v string) *string     { return &v }
func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

func near(t *testing.T, got, want float64, label string) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %.10f, want %.10f", label, got, want)
	}
}

func single(t *testing.T, episodes []*Episode) *Episode {
	t.Helper()
	if len(episodes) != 1 {
		t.Fatalf("期望 1 个 episode，实际 %d", len(episodes))
	}
	return episodes[0]
}

// 加仓三次后移动止盈：最基础的一条完整持仓。
func TestDeriveTrailingClose(t *testing.T) {
	s := &stream{}
	s.push("2026-08-18 00:22:16", eventlog.EvOpen, open("long", 1, 1))
	s.push("2026-08-18 01:17:57", eventlog.EvOpen, open("long", 2, 1))
	s.push("2026-08-18 02:03:16", eventlog.EvOpen, open("long", 3, 1))
	s.push("2026-08-18 02:18:33", eventlog.EvTrailingClose, closing("long", 3), withPnl(32.05, 3.0),
		func(row *eventstore.StrategyEvent) { row.PeakPct = floatPtr(42.36) })

	episodes, stats := Derive(s.rows, rebuiltAt)
	ep := single(t, episodes)
	if ep.ExitKind == nil || *ep.ExitKind != ExitTrailingClose {
		t.Fatalf("exit_kind = %v，期望 trailing_close", ep.ExitKind)
	}
	if ep.AddCount != 3 || ep.EntrySizeTotal != 3 || ep.MaxSize != 3 {
		t.Fatalf("add=%d total=%d max=%d，期望 3/3/3", ep.AddCount, ep.EntrySizeTotal, ep.MaxSize)
	}
	if ep.StrategyAttributable != 1 {
		t.Fatal("移动止盈应计入策略胜率")
	}
	if ep.OpenedAt == nil || ep.ClosedAt == nil || ep.DurationSec == nil || *ep.DurationSec != 6977 {
		t.Fatalf("持仓时长 = %v，期望 6977 秒", ep.DurationSec)
	}
	near(t, ep.Pnl, 3.0, "episode.pnl")
	near(t, ep.PnlStrategy, 3.0, "episode.pnl_strategy")
	if ep.PeakPct == nil || *ep.PeakPct != 42.36 {
		t.Fatalf("peak_pct = %v", ep.PeakPct)
	}
	if ep.OpenSize != 0 {
		t.Fatalf("已出场的 episode open_size 应为 0，实际 %v", ep.OpenSize)
	}
	// 决策归集：整仓平掉时每张平分，与 vol_state_phase0.py 的 pnl/len(open_adds) 一致。
	if len(ep.Entries) != 3 {
		t.Fatalf("决策行 = %d，期望 3", len(ep.Entries))
	}
	for _, entry := range ep.Entries {
		near(t, entry.AttributedPnl, 1.0, "entry.attributed_pnl")
		near(t, entry.ClosedSize, 1.0, "entry.closed_size")
		if entry.PnlKnown != 1 {
			t.Fatal("完全出场且盈亏已知的决策行 pnl_known 应为 1")
		}
	}
	if stats.ByExit[ExitTrailingClose] != 1 || stats.PositionGaps != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

// reduce_to_zero：反向减仓磨到 0，全程没有任何平仓事件。
// 新格式（2026-07-24 起）带 pnl，size 因为 omitempty 被省成 NULL。
func TestDeriveReduceToZeroWithPnl(t *testing.T) {
	s := &stream{}
	s.push("2026-08-18 22:33:32", eventlog.EvOpen, open("short", 1, 1))
	s.push("2026-08-19 01:35:40", eventlog.EvOpen, open("long", 0, 1), withPnl(13.93, 0.0722))

	ep := single(t, mustDerive(t, s))
	if ep.ExitKind == nil || *ep.ExitKind != ExitReduceToZero {
		t.Fatalf("exit_kind = %v，期望 reduce_to_zero", ep.ExitKind)
	}
	if ep.Side != "short" || ep.ReduceCount != 1 {
		t.Fatalf("side=%s reduce=%d", ep.Side, ep.ReduceCount)
	}
	if ep.StrategyAttributable != 1 {
		t.Fatal("减仓磨到 0 是策略自己的决定，应计入胜率")
	}
	near(t, ep.Pnl, 0.0722, "episode.pnl")
	near(t, ep.Entries[0].AttributedPnl, 0.0722, "entry.attributed_pnl")
	if ep.MissingPnlEvents != 0 {
		t.Fatalf("missing_pnl_events = %d，期望 0", ep.MissingPnlEvents)
	}
}

// 老格式（2026-07-24 之前）的减仓只有 size 没有 pnl：实现盈亏是**未知**，
// 不能当 0——否则会凭空造出一个"这次减仓不赚不亏"的假样本。
func TestDeriveReduceToZeroWithoutPnl(t *testing.T) {
	s := &stream{}
	s.push("2026-06-30 22:03:17", eventlog.EvOpen, open("short", 1, 1))
	s.push("2026-06-30 22:06:09", eventlog.EvOpen, open("long", 0, 1))

	ep := single(t, mustDerive(t, s))
	if ep.ExitKind == nil || *ep.ExitKind != ExitReduceToZero {
		t.Fatalf("exit_kind = %v", ep.ExitKind)
	}
	if ep.MissingPnlEvents != 1 || ep.RealizedEvents != 0 {
		t.Fatalf("missing=%d realized=%d，期望 1/0", ep.MissingPnlEvents, ep.RealizedEvents)
	}
	near(t, ep.Pnl, 0, "episode.pnl")
	entry := ep.Entries[0]
	if entry.MissingPnlEvents != 1 || entry.PnlKnown != 0 {
		t.Fatalf("决策行应标成归因不完整：missing=%d known=%d", entry.MissingPnlEvents, entry.PnlKnown)
	}
}

// 部分减仓按在场张数等比例摊，不是 FIFO：两笔各 1 张时减掉 1 张，
// 两笔各退 0.5 张、各拿一半减仓盈亏，后续平仓再按剩余的一半各摊一半。
func TestDeriveProRataAcrossReduce(t *testing.T) {
	s := &stream{}
	s.push("2026-08-18 00:00:00", eventlog.EvOpen, open("long", 1, 1))
	s.push("2026-08-18 00:10:00", eventlog.EvOpen, open("long", 2, 1))
	s.push("2026-08-18 00:20:00", eventlog.EvOpen, open("short", 1, 1), withPnl(20, 2.0))
	s.push("2026-08-18 00:30:00", eventlog.EvTrailingClose, closing("long", 1), withPnl(40, 4.0))

	ep := single(t, mustDerive(t, s))
	near(t, ep.Pnl, 6.0, "episode.pnl")
	if len(ep.Entries) != 2 {
		t.Fatalf("决策行 = %d", len(ep.Entries))
	}
	for i, entry := range ep.Entries {
		near(t, entry.AttributedPnl, 3.0, "entry.attributed_pnl")
		near(t, entry.ClosedSize, 1.0, "entry.closed_size")
		if entry.OpenSize != 0 {
			t.Fatalf("entry[%d].open_size = %v，期望 0", i, entry.OpenSize)
		}
	}
}

// 决策归集与平仓归集必须给出不同答案，否则这套派生就没有存在意义
// （设计文档 §10.3：按平仓时刻归集会把"更容易平仓的状态"误读成"更赚钱的状态"）。
func TestDecisionAttributionDiffersFromExitAttribution(t *testing.T) {
	s := &stream{}
	s.push("2026-08-18 01:00:00", eventlog.EvOpen, open("long", 1, 1), withVariant("lo-vol"))
	s.push("2026-08-18 02:00:00", eventlog.EvOpen, open("long", 2, 1), withVariant("lo-vol"))
	s.push("2026-08-18 03:00:00", eventlog.EvOpen, open("long", 3, 1), withVariant("hi-vol"))
	s.push("2026-08-18 03:10:00", eventlog.EvTrailingClose, closing("long", 3), withPnl(60, 9.0), withVariant("hi-vol"))

	ep := single(t, mustDerive(t, s))
	// 平仓归集会把 9.0 全记在 hi-vol 名下（episode.variant 是开仓时刻，这里是 lo-vol）。
	if ep.Variant == nil || *ep.Variant != "lo-vol" {
		t.Fatalf("episode.variant = %v，按 §6.3 应取开仓时刻", ep.Variant)
	}
	byVariant := map[string]float64{}
	for _, entry := range ep.Entries {
		byVariant[*entry.Variant] += entry.AttributedPnl
	}
	near(t, byVariant["lo-vol"], 6.0, "lo-vol 决策归集")
	near(t, byVariant["hi-vol"], 3.0, "hi-vol 决策归集")
}

// 仓位跳变：|Δsize| != orderSize 时以快照为准、记标记，episode 不断开（§6.5 / D6）。
func TestDerivePositionGapKeepsEpisodeIntact(t *testing.T) {
	s := &stream{}
	s.push("2026-06-29 09:00:00", eventlog.EvOpen, open("long", 15, 1))
	s.push("2026-06-29 10:28:00", eventlog.EvOpen, open("long", 1, 1)) // 15 → 1，orderSize 仍写 1
	s.push("2026-06-29 11:00:00", eventlog.EvOpen, open("long", 2, 1))
	s.push("2026-06-29 12:00:00", eventlog.EvTrailingClose, closing("long", 2), withPnl(10, 1.0))

	episodes := mustDerive(t, s)
	ep := single(t, episodes)
	if ep.HasPositionGap != 1 || ep.PositionGapCount == 0 {
		t.Fatalf("应记仓位跳变：flag=%d count=%d", ep.HasPositionGap, ep.PositionGapCount)
	}
	if ep.MaxSize != 15 {
		t.Fatalf("max_size = %d，期望 15", ep.MaxSize)
	}
	if ep.ExitKind == nil || *ep.ExitKind != ExitTrailingClose {
		t.Fatalf("episode 不应被跳变断开，exit_kind = %v", ep.ExitKind)
	}
}

// 开头被数据窗口截断：只看到平仓，opened_at 必须留 NULL 而不是拿平仓时刻凑。
func TestDeriveTruncatedHead(t *testing.T) {
	s := &stream{}
	s.push("2026-06-28 23:54:41", eventlog.EvTrailingClose, closing("short", 13), withPnl(19.98, 1.2146))
	s.push("2026-06-28 23:55:00", eventlog.EvOpen, open("long", 1, 1))

	episodes, stats := Derive(s.rows, rebuiltAt)
	if len(episodes) != 2 {
		t.Fatalf("episode 数 = %d，期望 2", len(episodes))
	}
	head := episodes[0]
	if head.OpenedAt != nil || head.DurationSec != nil {
		t.Fatalf("截断头的 opened_at/duration 必须为 NULL，实际 %v/%v", head.OpenedAt, head.DurationSec)
	}
	if len(head.Entries) != 0 {
		t.Fatal("截断头没有可归集的建仓决策")
	}
	near(t, head.Pnl, 1.2146, "截断头 episode.pnl")
	near(t, stats.UnattributedPnl, 1.2146, "落不到决策上的盈亏")
	if stats.TruncatedHead != 1 {
		t.Fatalf("TruncatedHead = %d", stats.TruncatedHead)
	}
	if episodes[1].ExitKind != nil {
		t.Fatal("第二个 episode 仍持仓")
	}
}

// 流末仍持仓：closed_at / exit_kind 留 NULL，在场张数落进 open_size。
func TestDeriveStillOpen(t *testing.T) {
	s := &stream{}
	s.push("2026-08-21 10:00:00", eventlog.EvOpen, open("short", 1, 1))
	s.push("2026-08-21 10:30:00", eventlog.EvOpen, open("short", 2, 1))
	s.push("2026-08-21 10:40:00", eventlog.EvLossAlert, closing("short", 2), withRoi(-210.5))

	ep := single(t, mustDerive(t, s))
	if ep.ExitKind != nil || ep.ClosedAt != nil {
		t.Fatalf("仍持仓的 episode 不应有出场信息：%v / %v", ep.ExitKind, ep.ClosedAt)
	}
	if ep.StrategyAttributable != 0 {
		t.Fatal("结果未定的持仓不计入策略胜率")
	}
	near(t, ep.OpenSize, 2, "episode.open_size")
	if ep.LossAlertCount != 1 || ep.MinRoiPctObserved == nil || *ep.MinRoiPctObserved != -210.5 {
		t.Fatalf("深度观测未收集：count=%d min=%v", ep.LossAlertCount, ep.MinRoiPctObserved)
	}
	for _, entry := range ep.Entries {
		if entry.PnlKnown != 0 || entry.OpenSize != 1 {
			t.Fatalf("在场决策行不该标成归因完整：known=%d open=%v", entry.PnlKnown, entry.OpenSize)
		}
	}
}

// external_close / manual_close 是交易所侧或人工操作：盈亏照记，但不计入策略胜率。
func TestDeriveExternalAndManualCloseNotAttributable(t *testing.T) {
	for _, kind := range []string{eventlog.EvExternalClose, eventlog.EvManualClose} {
		s := &stream{}
		s.push("2026-07-23 20:00:00", eventlog.EvOpen, open("short", 1, 1))
		s.push("2026-07-23 22:06:45", kind, closing("short", 1), withPnl(56.58, 1.7716))

		ep := single(t, mustDerive(t, s))
		if *ep.ExitKind != kind {
			t.Fatalf("exit_kind = %s", *ep.ExitKind)
		}
		if ep.StrategyAttributable != 0 {
			t.Fatalf("%s 不得计入策略胜率", kind)
		}
		near(t, ep.Pnl, 1.7716, "episode.pnl")
		near(t, ep.PnlStrategy, 0, "episode.pnl_strategy")
		entry := ep.Entries[0]
		near(t, entry.AttributedPnl, 1.7716, "entry.attributed_pnl")
		near(t, entry.AttributedPnlStrategy, 0, "entry.attributed_pnl_strategy")
		if entry.StrategyAttributable != 0 {
			t.Fatal("决策行应继承 episode 的胜率口径")
		}
	}
}

// 兜底止损与固定止盈都是策略自己的出场方式，必须计入。
func TestDeriveCatastropheAndFixedClose(t *testing.T) {
	for _, kind := range []string{eventlog.EvCatastropheStop, eventlog.EvFixedClose} {
		s := &stream{}
		s.push("2026-08-20 20:00:00", eventlog.EvOpen, open("short", 8, 8))
		s.push("2026-08-20 23:32:01", kind, closing("short", 8), withPnl(-412.25, -18.57))

		ep := single(t, mustDerive(t, s))
		if *ep.ExitKind != kind || ep.StrategyAttributable != 1 {
			t.Fatalf("%s: exit=%s attributable=%d", kind, *ep.ExitKind, ep.StrategyAttributable)
		}
		near(t, ep.PnlStrategy, -18.57, "episode.pnl_strategy")
	}
}

// 持仓期内被挡掉的信号按类型分别计数——"今天最常被什么条件挡住"要能落到 episode 上。
func TestDeriveGateCounters(t *testing.T) {
	s := &stream{}
	s.push("2026-08-20 00:00:00", eventlog.EvOpen, open("short", 26, 26))
	s.push("2026-08-20 00:01:00", eventlog.EvCapSkip, closing("short", 26))
	s.push("2026-08-20 00:02:00", eventlog.EvGateBlock, closing("long", 26), withRoi(-98.6))
	s.push("2026-08-20 00:03:00", eventlog.EvTrendSkip, closing("short", 26))
	s.push("2026-08-20 00:04:00", eventlog.EvLossAlert, closing("short", 26), withRoi(-362.2))
	s.push("2026-08-20 00:05:00", eventlog.EvTrailingClose, closing("short", 26), withPnl(50, 5))

	ep := single(t, mustDerive(t, s))
	if ep.CapSkipCount != 1 || ep.GateBlockCount != 1 || ep.TrendSkipCount != 1 || ep.LossAlertCount != 1 {
		t.Fatalf("门控计数错误：%+v", []int{ep.CapSkipCount, ep.GateBlockCount, ep.TrendSkipCount, ep.LossAlertCount})
	}
	if *ep.MinRoiPctObserved != -362.2 || *ep.MaxRoiPctObserved != 50 {
		t.Fatalf("ROI 观测区间 = [%v, %v]", ep.MinRoiPctObserved, ep.MaxRoiPctObserved)
	}
}

// 同名账户标签在两个实例里指向不同真实账户，账本必须按 (instance, account) 隔离。
func TestDeriveIsolatesInstances(t *testing.T) {
	s := &stream{}
	s.push("2026-08-18 00:00:00", eventlog.EvOpen, open("long", 1, 1))
	other := s.push("2026-08-18 00:00:01", eventlog.EvOpen, open("short", 1, 1))
	other.InstanceKey = "it-episode-2"
	s.push("2026-08-18 01:00:00", eventlog.EvTrailingClose, closing("long", 1), withPnl(10, 1))

	episodes := mustDerive(t, s)
	if len(episodes) != 2 {
		t.Fatalf("episode 数 = %d，期望两个实例各一个", len(episodes))
	}
	for _, ep := range episodes {
		if ep.InstanceKey == "it-episode-2" && ep.ExitKind != nil {
			t.Fatal("实例2 的持仓被实例1 的平仓事件误关掉了")
		}
	}
}

// 派生必须是纯函数：同样的输入跑两次，产出逐字段一致。
func TestDeriveIsPureFunction(t *testing.T) {
	build := func() []*eventstore.StrategyEvent {
		s := &stream{}
		s.push("2026-08-18 00:00:00", eventlog.EvOpen, open("long", 1, 1))
		s.push("2026-08-18 00:10:00", eventlog.EvOpen, open("long", 2, 1))
		s.push("2026-08-18 00:20:00", eventlog.EvOpen, open("short", 1, 1), withPnl(20, 2.0))
		s.push("2026-08-18 00:30:00", eventlog.EvTrailingClose, closing("long", 1), withPnl(40, 4.0))
		return s.rows
	}
	first, statsA := Derive(build(), rebuiltAt)
	second, statsB := Derive(build(), rebuiltAt)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(statsA, statsB) {
		t.Fatal("同样的输入产出了不同的 episode，派生不是纯函数")
	}
}

// 乱序输入按 (ts, id) 归位：同秒事件的先后只有 id 能判定。
func TestDeriveSortsByTsThenId(t *testing.T) {
	s := &stream{}
	s.push("2026-08-18 00:00:00", eventlog.EvOpen, open("long", 1, 1))
	s.push("2026-08-18 00:00:00", eventlog.EvTrailingClose, closing("long", 1), withPnl(10, 1))
	shuffled := []*eventstore.StrategyEvent{s.rows[1], s.rows[0]}

	ep := single(t, mustDeriveRows(t, shuffled))
	if ep.ExitKind == nil || *ep.ExitKind != ExitTrailingClose || ep.AddCount != 1 {
		t.Fatalf("乱序输入未按 (ts,id) 归位：exit=%v add=%d", ep.ExitKind, ep.AddCount)
	}
}

func TestClassifyDepth(t *testing.T) {
	cases := []struct {
		total, withUpl int
		want           string
	}{
		{0, 0, DepthAlertSampled},   // 窗口内一条心跳都没有
		{120, 0, DepthAlertSampled}, // upl 字段还没上线
		{120, 120, DepthMinute},
		{120, 110, DepthMinute}, // 零星漏拍仍算分钟级
		{120, 60, DepthMixed},   // 跨越 upl 上线时点
	}
	for _, c := range cases {
		if got := classifyDepth(c.total, c.withUpl); got != c.want {
			t.Fatalf("classifyDepth(%d,%d) = %s，期望 %s", c.total, c.withUpl, got, c.want)
		}
	}
}

func mustDerive(t *testing.T, s *stream) []*Episode {
	t.Helper()
	return mustDeriveRows(t, s.rows)
}

func mustDeriveRows(t *testing.T, rows []*eventstore.StrategyEvent) []*Episode {
	t.Helper()
	episodes, _ := Derive(rows, rebuiltAt)
	return episodes
}

// 日志窗口一开始就扛着仓位：头几条只有 gate_block / loss_alert，没有任何 open。
// 这些观测必须挂进一个截断头 episode，而不是被当成孤儿丢掉——实测 roc 实例
// 开头就有 30 条这样的事件（账户A 扛着 14 张多单，最深 −170%）。
func TestDeriveStartsTruncatedFromObservation(t *testing.T) {
	s := &stream{}
	s.push("2026-06-28 22:37:18", eventlog.EvGateBlock, func(row *eventstore.StrategyEvent) {
		row.Side = strPtr("short")
		row.NetSide = strPtr("long")
		row.Size = intPtr(14)
		row.RoiPct = floatPtr(-31.35)
	})
	s.push("2026-06-29 01:12:45", eventlog.EvLossAlert, closing("long", 14), withRoi(-150.0))
	s.push("2026-06-29 09:00:00", eventlog.EvTrailingClose, closing("long", 14), withPnl(20, 2.0))

	episodes, stats := Derive(s.rows, rebuiltAt)
	ep := single(t, episodes)
	if ep.OpenedAt != nil {
		t.Fatalf("截断头的 opened_at 必须为 NULL，实际 %v", ep.OpenedAt)
	}
	if ep.Side != "long" {
		t.Fatalf("方向应取 gate_block 的 net_side，实际 %s", ep.Side)
	}
	if ep.MaxSize != 14 || ep.GateBlockCount != 1 || ep.LossAlertCount != 1 {
		t.Fatalf("观测未挂进 episode：max=%d gate=%d alert=%d", ep.MaxSize, ep.GateBlockCount, ep.LossAlertCount)
	}
	if ep.MinRoiPctObserved == nil || *ep.MinRoiPctObserved != -150.0 {
		t.Fatalf("深度观测 = %v", ep.MinRoiPctObserved)
	}
	if stats.TruncatedHead != 1 || len(stats.OrphanEvents) != 0 {
		t.Fatalf("stats = %+v", stats)
	}
	// 建仓决策全在窗口之外，这笔盈亏无处可归——如实记账，不硬摊给谁。
	near(t, stats.UnattributedPnl, 2.0, "落不到决策上的盈亏")
}

// 持仓快照滞后（Δsize = 0）：实测 2026-07-07 05:10 两账户同秒下单，
// 反向单发出后快照仍报旧仓位。记标记不断开，也不凭 orderSize 猜真实仓位。
func TestDeriveStaleSnapshotKeepsPosition(t *testing.T) {
	s := &stream{}
	s.push("2026-07-07 05:10:15", eventlog.EvOpen, open("short", 1, 1))
	s.push("2026-07-07 05:10:26", eventlog.EvOpen, open("long", 1, 1)) // 反向单，快照仍是 1
	s.push("2026-07-07 05:20:00", eventlog.EvTrailingClose, closing("short", 1), withPnl(10, 1.0))

	episodes, stats := Derive(s.rows, rebuiltAt)
	ep := single(t, episodes)
	if stats.StaleSnapshot != 1 || stats.SideFlipRejected != 0 {
		t.Fatalf("Δsize=0 应记成快照滞后而不是翻转单：%+v", stats)
	}
	if ep.HasPositionGap != 1 {
		t.Fatal("快照滞后必须留下仓位跳变标记")
	}
	if ep.ExitKind == nil || *ep.ExitKind != ExitTrailingClose {
		t.Fatalf("episode 不应被快照滞后断开：%v", ep.ExitKind)
	}
	near(t, ep.Pnl, 1.0, "episode.pnl")
}

// 报告不得把账户邮箱原样写进文档：logs/ 与 configs/ 含真实生产凭证，
// 任何落盘产物都要打码。
func TestMaskAccount(t *testing.T) {
	cases := map[string]string{
		"账户A-1394537246@qq.com":   "账户A-***@qq.com",
		"账户B-mortypeng@gmail.com": "账户B-***@gmail.com",
		"账户A-9564268":             "账户A-9564268",
		"":                        "(未标注)",
	}
	for in, want := range cases {
		if got := MaskAccount(in); got != want {
			t.Fatalf("MaskAccount(%q) = %q，期望 %q", in, got, want)
		}
	}
}

// 账本的第一条 open 就报出比下单量更大的净仓：那多出来的张数是窗口之前建的，
// 不能全记给这一笔决策。实测 roc 账户B 的首条事件正是 open size=6 / orderSize=1。
func TestDeriveFirstOpenLargerThanOrderIsTruncatedHead(t *testing.T) {
	s := &stream{}
	s.push("2026-06-28 22:37:18", eventlog.EvOpen, open("short", 6, 1))
	s.push("2026-06-28 22:37:49", eventlog.EvOpen, open("short", 7, 1))
	s.push("2026-06-28 23:54:41", eventlog.EvTrailingClose, closing("short", 7), withPnl(37.88, 7.0))

	ep := single(t, mustDerive(t, s))
	if ep.OpenedAt != nil {
		t.Fatalf("首条事件已带 5 张存量仓位，opened_at 应为 NULL，实际 %v", ep.OpenedAt)
	}
	if ep.AddCount != 2 || ep.EntrySizeTotal != 2 {
		t.Fatalf("只应归集看得见的 2 张：add=%d total=%d", ep.AddCount, ep.EntrySizeTotal)
	}
	if ep.MaxSize != 7 {
		t.Fatalf("峰值仍按真实净仓算，实际 %d", ep.MaxSize)
	}
	// 7 张的盈亏里只有 2 张的建仓决策可见：每张 1.0，其余 5.0 落在 unattributed_pnl。
	for _, entry := range ep.Entries {
		near(t, entry.AttributedPnl, 1.0, "entry.attributed_pnl")
	}
	near(t, ep.Pnl, 7.0, "episode.pnl")
	near(t, ep.UnattributedPnl, 5.0, "episode.unattributed_pnl")
}

// 但"净仓大于下单量"只有在账本第一条时才能这么推：仓位归零之后我们亲眼看着
// 它从 0 涨起来，此时的多张只可能是信号突发累加一次下单（§6.5 的 7/05 01:39）。
func TestDeriveBurstAddAfterFlatIsNotTruncated(t *testing.T) {
	s := &stream{}
	s.push("2026-07-05 01:00:00", eventlog.EvOpen, open("long", 1, 1))
	s.push("2026-07-05 01:20:00", eventlog.EvTrailingClose, closing("long", 1), withPnl(10, 1.0))
	s.push("2026-07-05 01:39:00", eventlog.EvOpen, open("long", 3, 1)) // 0 → 3，突发累加

	episodes := mustDeriveRows(t, s.rows)
	if len(episodes) != 2 {
		t.Fatalf("episode 数 = %d，期望 2", len(episodes))
	}
	burst := episodes[1]
	if burst.OpenedAt == nil {
		t.Fatal("空仓之后的突发累加是可见决策，不该判成截断头")
	}
	if burst.EntrySizeTotal != 3 || burst.HasPositionGap != 1 {
		t.Fatalf("应把 3 张全记给这一笔并标跳变：total=%d gap=%d", burst.EntrySizeTotal, burst.HasPositionGap)
	}
}
