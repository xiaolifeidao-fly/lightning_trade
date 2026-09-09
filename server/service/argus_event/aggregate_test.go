package argus_event

import (
	"math"
	"testing"
	"time"

	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
)

// 时间轴聚合：桶与 K 线网格对齐，净持仓成阶梯，平仓后归零。
func TestBuildTimelineBucketsAlignsAndLadders(t *testing.T) {
	rows := []*repository.StrategyEventRow{
		// 10:00 桶：accountA 开到 3 张
		{Ts: "2026-08-18 10:00:12", Event: "open", AccountLabel: "A", Size: intp(3), OrderSize: intp(1), GapBp: fltp(6.1)},
		// 同桶内另一个账户被上限挡住，净仓 8 张
		{Ts: "2026-08-18 10:00:47", Event: "cap_skip", AccountLabel: "B", Size: intp(8), GapBp: fltp(-11.2)},
		// 10:02 桶：accountA 移动止盈平仓 → 该账户归零，只剩 B 的 8 张
		{Ts: "2026-08-18 10:02:05", Event: "trailing_close", AccountLabel: "A", Size: intp(3), Pnl: fltp(42.5)},
	}
	start, _ := parseEventTime("2026-08-18 10:00:00", false)
	end, _ := parseEventTime("2026-08-18 10:03:00", false)
	buckets := buildTimelineBuckets(rows, start, end, time.Minute)

	if len(buckets) != 4 {
		t.Fatalf("10:00–10:03 的 1m 桶应有 4 个, got %d", len(buckets))
	}
	if buckets[0].Total != 2 || buckets[0].Open != 1 || buckets[0].CapSkip != 1 {
		t.Errorf("首桶计数有误: %+v", buckets[0])
	}
	if buckets[0].MaxAbsGapBp == nil || *buckets[0].MaxAbsGapBp != 11.2 {
		t.Errorf("首桶最大 |gapBp| 有误: %v", buckets[0].MaxAbsGapBp)
	}
	if buckets[0].NetSizeEnd != 11 {
		t.Errorf("首桶净仓应为 3+8=11, got %d", buckets[0].NetSizeEnd)
	}
	// 无事件的桶沿用上一桶，形成阶梯
	if buckets[1].Total != 0 || buckets[1].NetSizeEnd != 11 {
		t.Errorf("空桶应沿用上一桶净仓: %+v", buckets[1])
	}
	if buckets[2].Exit != 1 || buckets[2].NetSizeEnd != 8 {
		t.Errorf("平仓桶应把该账户归零: %+v", buckets[2])
	}
	if buckets[2].RealizedPnl == nil || *buckets[2].RealizedPnl != 42.5 {
		t.Errorf("平仓桶已实现盈亏有误: %v", buckets[2].RealizedPnl)
	}
}

func TestKlineCoverage(t *testing.T) {
	start, _ := parseEventTime("2026-08-18 10:00:00", false)
	end, _ := parseEventTime("2026-08-18 10:09:00", false)
	c := klineCoverage(8, start, end, time.Minute)
	if c.Expected != 10 || c.Actual != 8 || c.Missing != 2 || c.CoveragePct != 80 {
		t.Errorf("覆盖率计算有误: %+v", c)
	}
}

// 拦截原因聚合：结果分布 + 门控三元组均值 + 强度分级开仓率。
func TestAggregateGateStats(t *testing.T) {
	rows := []*repository.StrategyEventRow{
		{Ts: "2026-08-18 10:00:01", Event: "open", GapBp: fltp(6.0)},
		{Ts: "2026-08-18 10:00:02", Event: "open", GapBp: fltp(9.5)},
		{Ts: "2026-08-18 10:01:00", Event: "gate_block", GapBp: fltp(6.2),
			GateKind: strp("reverse_gate_profit"), GateThreshold: fltp(8), GateActual: fltp(-100),
			Reason: strp("盈利不足 ROI=-100.0% < 8%")},
		{Ts: "2026-08-18 10:02:00", Event: "gate_block", GapBp: fltp(6.4),
			GateKind: strp("reverse_gate_profit"), GateThreshold: fltp(8), GateActual: fltp(-120)},
		{Ts: "2026-08-18 10:03:00", Event: "cap_skip", GapBp: fltp(12.0), GateKind: strp("cap")},
		// 老数据：拦截但没结构化出 gate_kind，必须进 unknown 桶而不是被丢掉
		{Ts: "2026-08-18 10:04:00", Event: "trend_skip"},
	}
	var out argusDTO.GateStatsDTO
	aggregateGateStats(rows, &out)

	if out.TotalTriggers != 6 || out.Opened != 2 || out.Blocked != 4 {
		t.Fatalf("总量有误: %+v", out)
	}
	if out.OpenRate != 0.3333 {
		t.Errorf("开仓率有误: %v", out.OpenRate)
	}
	if len(out.ByResult) != 4 {
		t.Errorf("结果分布桶数有误: %+v", out.ByResult)
	}
	gates := map[string]argusDTO.GateBucketDTO{}
	for _, g := range out.ByGate {
		gates[g.GateKind] = g
	}
	rg, ok := gates["reverse_gate_profit"]
	if !ok || rg.Count != 2 || rg.AvgThreshold == nil || *rg.AvgThreshold != 8 ||
		rg.AvgActual == nil || *rg.AvgActual != -110 {
		t.Errorf("反向门控聚合有误: %+v", rg)
	}
	if rg.Share != 0.5 {
		t.Errorf("拦截占比应按被拦截总数算: %v", rg.Share)
	}
	if rg.LastTs != "2026-08-18 10:02:00" {
		t.Errorf("最近一次拦截时刻有误: %q", rg.LastTs)
	}
	if rg.SampleReason == "" {
		t.Error("应保留一条原始 reason 供逐字对账")
	}
	if unknown, ok := gates["unknown"]; !ok || unknown.Count != 1 {
		t.Errorf("未结构化的拦截应进 unknown 桶: %+v", gates)
	}
	// 强度分级：三档恒定输出，方便前端画固定坐标
	if len(out.ByStrength) != 3 {
		t.Fatalf("强度分档应恒为 3 档: %+v", out.ByStrength)
	}
	weak := out.ByStrength[0]
	if weak.Level != StrengthWeak || weak.Count != 3 || weak.Opened != 1 {
		t.Errorf("弱档统计有误: %+v", weak)
	}
	strong := out.ByStrength[2]
	if strong.Count != 2 || strong.Opened != 1 || strong.OpenRate != 0.5 {
		t.Errorf("强档统计有误: %+v", strong)
	}
}

// 跨实例汇总：逐实例独立成行，服务端绝不做跨实例求和。
func TestAggregateInstanceSummaryKeepsInstancesApart(t *testing.T) {
	rows := []*repository.StrategyEventRow{
		{Ts: "2026-08-18 10:00:00", InstanceKey: "inst-a", AccountLabel: "account1", Event: "open",
			Variant: strp("champion/default"), ConfigVersion: 36},
		{Ts: "2026-08-18 10:01:00", InstanceKey: "inst-a", AccountLabel: "account1", Event: "cap_skip", ConfigVersion: 37},
		{Ts: "2026-08-18 10:02:00", InstanceKey: "inst-a", AccountLabel: "account1", Event: "trailing_close", Pnl: fltp(120.5)},
		{Ts: "2026-08-18 10:00:30", InstanceKey: "inst-b", AccountLabel: "account1", Event: "open",
			Variant: strp("champion/S400_cap246_gate8"), ConfigVersion: 12},
	}
	latest := []*repository.BalanceLatestRow{
		{InstanceKey: "inst-a", AccountLabel: "account1", Uid: "111", Ts: "2026-08-18 10:05:00",
			Balance: fltp(500), Equity: fltp(480), NetSize: intp(4), NetSizeKnown: 1},
		{InstanceKey: "inst-b", AccountLabel: "account1", Uid: "999", Ts: "2026-08-18 10:05:00",
			Balance: fltp(9000), Equity: fltp(8800), NetSize: intp(120), NetSizeKnown: 1},
	}
	registry := map[string]argusDTO.InstanceSummaryDTO{
		"inst-a": {InstanceKey: "inst-a", InstanceName: "实例1", Enabled: 1, Registered: true},
	}
	out := aggregateInstanceSummary(rows, latest, registry)
	if len(out) != 2 {
		t.Fatalf("应输出 2 个实例, got %d", len(out))
	}
	a, b := out[0], out[1]
	if a.InstanceKey != "inst-a" || a.InstanceName != "实例1" || !a.Registered {
		t.Errorf("注册表信息未挂上: %+v", a)
	}
	if a.Signals != 2 || a.Opened != 1 || a.CapSkip != 1 || a.Exits != 1 {
		t.Errorf("实例A 计数有误: %+v", a)
	}
	if a.ConfigVersion != 37 {
		t.Errorf("实例A 版本应取窗口内最大值: %d", a.ConfigVersion)
	}
	if a.RealizedPnl == nil || *a.RealizedPnl != 120.5 {
		t.Errorf("实例A 已实现盈亏有误: %v", a.RealizedPnl)
	}
	if a.EquityTotal == nil || *a.EquityTotal != 480 || a.NetSizeTotal == nil || *a.NetSizeTotal != 4 {
		t.Errorf("实例A 权益/净仓不得混入其他实例: %+v", a)
	}
	// 事件里出现但注册表没有的实例照样列出，并标 registered=false
	if b.InstanceKey != "inst-b" || b.Registered {
		t.Errorf("未注册实例应可见且 registered=false: %+v", b)
	}
	if b.Signals != 1 || b.EquityTotal == nil || *b.EquityTotal != 8800 {
		t.Errorf("实例B 汇总有误: %+v", b)
	}
	// 同名账户 account1 在两个实例下必须各归各的
	if len(a.Accounts) != 1 || len(b.Accounts) != 1 || a.Accounts[0].Uid == b.Accounts[0].Uid {
		t.Errorf("同名账户跨实例被合并: %+v / %+v", a.Accounts, b.Accounts)
	}
}

// 权益曲线：相对首个已知权益的变动百分比，account 序列按实例归集。
func TestBuildEquitySeriesComputesChangePct(t *testing.T) {
	rows := []*repository.BalanceBucketRow{
		{InstanceKey: "inst-a", AccountLabel: "A", BucketTs: "2026-08-18 10:00:00", Samples: 10, AvgBalance: fltp(1000), AvgEquity: fltp(1000)},
		{InstanceKey: "inst-a", AccountLabel: "A", BucketTs: "2026-08-18 10:10:00", Samples: 10, AvgBalance: fltp(1000), AvgEquity: fltp(900)},
		{InstanceKey: "inst-a", AccountLabel: "B", BucketTs: "2026-08-18 10:00:00", Samples: 10, AvgBalance: fltp(500)},
	}
	meta := map[string]*repository.BalanceLatestRow{
		"A": {AccountLabel: "A", Uid: "111", Variant: "champion"},
	}
	series := buildEquitySeries("inst-a", rows, meta)
	if len(series) != 2 {
		t.Fatalf("应输出 2 个账户序列, got %d", len(series))
	}
	a := series[0]
	if a.AccountLabel != "A" || a.Uid != "111" || a.Variant != "champion" {
		t.Errorf("账户元信息未挂上: %+v", a)
	}
	if a.ChangePct == nil || *a.ChangePct != -10 {
		t.Errorf("整段变动应为 -10%%: %v", a.ChangePct)
	}
	if a.Points[1].ChangePct == nil || *a.Points[1].ChangePct != -10 {
		t.Errorf("逐点变动有误: %v", a.Points[1].ChangePct)
	}
	// equity 全程未知（equity_known=0）的账户不算百分比，留空而不是补 0
	b := series[1]
	if b.ChangePct != nil || b.Points[0].ChangePct != nil {
		t.Errorf("权益未知时不得硬算百分比: %+v", b)
	}
}

// 秒级切片：按秒折叠、偏移量正确、同秒多账户只留一份点位。
func TestBuildSlicePoints(t *testing.T) {
	anchor, _ := parseEventTime("2026-08-18 10:15:03", false)
	rows := []*repository.StrategyEventRow{
		{Ts: "2026-08-18 10:14:58", Event: "open", SigLast: fltp(64000), SigMark: fltp(63990), GapBp: fltp(1.56)},
		{Ts: "2026-08-18 10:15:03", Event: "open", SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.52)},
		{Ts: "2026-08-18 10:15:03", Event: "gate_block", SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.52)},
	}
	points := buildSlicePoints(rows, anchor)
	if len(points) != 2 {
		t.Fatalf("同秒事件应折叠成一个点位, got %d", len(points))
	}
	if points[0].OffsetSec != -5 || points[0].IsTriggerTs {
		t.Errorf("前置点位有误: %+v", points[0])
	}
	if points[1].OffsetSec != 0 || !points[1].IsTriggerTs {
		t.Errorf("触发点位有误: %+v", points[1])
	}
	if len(points[1].Events) != 2 {
		t.Errorf("同秒的事件类型应全部列出: %+v", points[1].Events)
	}
	if points[1].DcLast == nil || *points[1].DcLast != 64080.6 {
		t.Errorf("报价点位有误: %v", points[1].DcLast)
	}
}

func TestIntervalDuration(t *testing.T) {
	if d, ok := intervalDuration("5m"); !ok || d != 5*time.Minute {
		t.Errorf("5m 解析有误: %v %v", d, ok)
	}
	// 1w 是唯一刻意拒绝的周期：Truncate 的整除点落在周四，与交易所周线对不上
	if _, ok := intervalDuration("1w"); ok {
		t.Error("1w 分桶会整体错位，应拒绝")
	}
}

// loss_alert 的 pnl 是**未实现浮亏**（account_monitor.go 取 pos.UnrealizedProfit），
// 且同一个持仓每过冷却就再报一次。把它累进「已实现盈亏」既是口径错误，也会把
// 同一笔浮亏重复计数多次。
//
// 真实数据：roc 实例 6 条 loss_alert 合计 -53.89，而真正实现的只有
// 减仓 +0.33 与移动止盈 +3.03，共 +3.36。旧写法把总额算成 -50.53，
// 符号反了、量级差 15 倍。
func TestRealizedPnlExcludesLossAlert(t *testing.T) {
	rows := []*repository.StrategyEventRow{
		{Ts: "2026-08-18 10:00:00", InstanceKey: "inst-a", AccountLabel: "A", Event: "open", Size: intp(5), OrderSize: intp(1)},
		// 同一个持仓的三次浮亏告警：金额相近且反复出现，正是重复计数的来源
		{Ts: "2026-08-18 10:01:00", InstanceKey: "inst-a", AccountLabel: "A", Event: "loss_alert", Size: intp(5), Pnl: fltp(-18.0), RoiPct: fltp(-160)},
		{Ts: "2026-08-18 10:06:00", InstanceKey: "inst-a", AccountLabel: "A", Event: "loss_alert", Size: intp(5), Pnl: fltp(-17.5), RoiPct: fltp(-155)},
		{Ts: "2026-08-18 10:11:00", InstanceKey: "inst-a", AccountLabel: "A", Event: "loss_alert", Size: intp(5), Pnl: fltp(-18.4), RoiPct: fltp(-162)},
		// 真正实现的两笔
		{Ts: "2026-08-18 10:20:00", InstanceKey: "inst-a", AccountLabel: "A", Event: "open", Size: intp(4), OrderSize: intp(1), Pnl: fltp(0.33)},
		{Ts: "2026-08-18 10:30:00", InstanceKey: "inst-a", AccountLabel: "A", Event: "trailing_close", Size: intp(4), Pnl: fltp(3.03)},
	}

	// 1) 实例汇总
	out := aggregateInstanceSummary(rows, nil, map[string]argusDTO.InstanceSummaryDTO{})
	if len(out) != 1 {
		t.Fatalf("应输出 1 个实例, got %d", len(out))
	}
	if out[0].RealizedPnl == nil {
		t.Fatal("已实现盈亏不应为 nil")
	}
	if got := *out[0].RealizedPnl; math.Abs(got-3.36) > 1e-9 {
		t.Errorf("已实现盈亏 = %.4f，期望 3.36（浮亏告警必须排除）", got)
	}

	// 2) 时间轴分桶：浮亏告警所在的桶不该冒出「已实现盈亏」
	start, _ := parseEventTime("2026-08-18 10:00:00", false)
	end, _ := parseEventTime("2026-08-18 10:30:00", false)
	buckets := buildTimelineBuckets(rows, start, end, 10*time.Minute)
	var total float64
	for _, b := range buckets {
		if b.RealizedPnl != nil {
			total += *b.RealizedPnl
		}
	}
	if math.Abs(total-3.36) > 1e-9 {
		t.Errorf("时间轴各桶已实现盈亏合计 = %.4f，期望 3.36", total)
	}
}
