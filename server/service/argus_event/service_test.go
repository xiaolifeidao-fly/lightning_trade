package argus_event

import (
	"errors"
	"testing"

	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
)

func strp(s string) *string   { return &s }
func intp(v int) *int         { return &v }
func fltp(v float64) *float64 { return &v }

// 时间口径：本地墙钟串进、本地墙钟串出，不做任何时区换算——错 8 小时是
// 本需求反复点名的隐蔽故障（数据全在、图能画、只是整段错位）。
func TestParseEventTimeKeepsWallClock(t *testing.T) {
	cases := []struct {
		in       string
		endOfDay bool
		want     string
	}{
		{"2026-08-18 00:22:16", false, "2026-08-18 00:22:16"},
		{"2026-08-18 00:22", false, "2026-08-18 00:22:00"},
		{"2026-08-18", false, "2026-08-18 00:00:00"},
		{"2026-08-18", true, "2026-08-18 23:59:59"},
		{"2026-08-18 09:30", true, "2026-08-18 09:30:59"},
	}
	for _, c := range cases {
		got, err := parseEventTime(c.in, c.endOfDay)
		if err != nil {
			t.Fatalf("parseEventTime(%q) 报错: %v", c.in, err)
		}
		if formatEventTime(got) != c.want {
			t.Errorf("parseEventTime(%q, endOfDay=%v) = %q, 期望 %q", c.in, c.endOfDay, formatEventTime(got), c.want)
		}
	}
	if _, err := parseEventTime("18/08/2026", false); err == nil {
		t.Error("非法时间格式应报错")
	}
}

func TestSplitCSVTrimsAndDedupes(t *testing.T) {
	got := splitCSV(" a , b ,, a ,c")
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("splitCSV = %v, 期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("splitCSV = %v, 期望 %v", got, want)
		}
	}
	if splitCSV("  ") != nil {
		t.Error("全空白应返回 nil")
	}
}

func TestResolveEventsByCategory(t *testing.T) {
	trigger, err := resolveEvents("", "")
	if err != nil || len(trigger) != 4 {
		t.Fatalf("默认应为四类触发事件, got %v err=%v", trigger, err)
	}
	exit, err := resolveEvents(CategoryExit, "")
	if err != nil || len(exit) != 6 {
		t.Fatalf("exit 应为六类, got %v err=%v", exit, err)
	}
	all, err := resolveEvents(CategoryAll, "")
	if err != nil || len(all) != 10 {
		t.Fatalf("all 应为十类, got %v err=%v", all, err)
	}
	// 显式 events 覆盖 category
	explicit, err := resolveEvents(CategoryExit, "open")
	if err != nil || len(explicit) != 1 || explicit[0] != "open" {
		t.Fatalf("显式事件应覆盖 category, got %v err=%v", explicit, err)
	}
	if _, err := resolveEvents("", "not_an_event"); err == nil {
		t.Error("未知事件类型应报错")
	}
	if _, err := resolveEvents("nope", ""); err == nil {
		t.Error("未知事件大类应报错")
	}
}

func TestApplyResultFilter(t *testing.T) {
	events, gates, err := applyResultFilter(ResultFilterOpen, TriggerEvents())
	if err != nil || len(events) != 1 || events[0] != "open" || gates != nil {
		t.Fatalf("open 结果筛选有误: %v %v %v", events, gates, err)
	}
	events, gates, err = applyResultFilter(ResultFilterBlocked, TriggerEvents())
	if err != nil || len(events) != 3 || gates != nil {
		t.Fatalf("blocked 结果筛选有误: %v %v %v", events, gates, err)
	}
	events, gates, err = applyResultFilter("cap", TriggerEvents())
	if err != nil || len(events) != 3 || len(gates) != 1 || gates[0] != "cap" {
		t.Fatalf("gate_kind 结果筛选有误: %v %v %v", events, gates, err)
	}
	if _, _, err := applyResultFilter("whatever", TriggerEvents()); err == nil {
		t.Error("未知结果筛选应报错")
	}
	// 结果与事件类型取交集为空时，列表必须报冲突而不是悄悄返回全量
	if _, err := buildEventFilter(argusDTO.SignalQueryDTO{Category: CategoryExit, Result: ResultFilterOpen}); !errors.Is(err, ErrFilterConflict) {
		t.Errorf("冲突筛选应返回 ErrFilterConflict, got %v", err)
	}
}

func TestStrengthLevelBoundaries(t *testing.T) {
	cases := []struct {
		gap  *float64
		want string
	}{
		{nil, ""},
		{fltp(5.01), StrengthWeak},
		{fltp(6.999), StrengthWeak},
		{fltp(7), StrengthMedium},
		{fltp(-8.5), StrengthMedium}, // 负号只表示 DOWN，分档看绝对值
		{fltp(9), StrengthStrong},
		{fltp(-42), StrengthStrong},
	}
	for _, c := range cases {
		if got := StrengthLevelOf(c.gap); got != c.want {
			t.Errorf("StrengthLevelOf(%v) = %q, 期望 %q", c.gap, got, c.want)
		}
	}
}

func TestBuildEventFilterCarriesInstanceAndStrength(t *testing.T) {
	f, err := buildEventFilter(argusDTO.SignalQueryDTO{
		InstanceKey:  "argus-single-roc",
		InstanceKeys: "argus-single-ives,argus-single-roc",
		Instrument:   "BTC-USDT-SWAP",
		Strength:     StrengthMedium,
		Start:        "2026-08-18",
		End:          "2026-08-21",
	})
	if err != nil {
		t.Fatalf("buildEventFilter 报错: %v", err)
	}
	if len(f.InstanceKeys) != 2 || f.InstanceKeys[0] != "argus-single-roc" {
		t.Errorf("实例键合并去重有误: %v", f.InstanceKeys)
	}
	if len(f.Instruments) != 1 || f.Instruments[0] != "BTCUSDT" {
		t.Errorf("合约归一化有误: %v", f.Instruments)
	}
	if f.GapBpMinAbs == nil || *f.GapBpMinAbs != 7 || f.GapBpMaxAbs == nil || *f.GapBpMaxAbs != 9 {
		t.Errorf("强度区间有误: %v %v", f.GapBpMinAbs, f.GapBpMaxAbs)
	}
	// 右界只给日期时必须补到当天最后一秒，否则"查 8/18–8/21"会丢掉 8/21 全天
	if f.End != "2026-08-21 23:59:59" {
		t.Errorf("右界补齐有误: %q", f.End)
	}
	// 非法实例键必须挡在查询之前
	if _, err := buildEventFilter(argusDTO.SignalQueryDTO{InstanceKey: "bad key:1"}); err == nil {
		t.Error("非法实例键应报错")
	}
}

// ─── 信号归组：本任务两条硬约束的回归 ────────────────────────────────────────

// 同一次触发的各账户事件靠报价快照三元组归组，成交事件比拦截事件晚数秒也算同一次。
func TestGroupSignalRowsGroupsByQuoteNotTimestamp(t *testing.T) {
	anchor := &repository.StrategyEventRow{
		Id: 100, Ts: "2026-08-18 10:15:03", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountA", Event: "gate_block", Side: strp("long"),
		SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.5280527),
	}
	// 同一次触发的成交事件：等下单往返，晚 4 秒，报价三元组完全一致
	openRow := &repository.StrategyEventRow{
		Id: 101, Ts: "2026-08-18 10:15:07", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountB", Event: "open", Side: strp("long"), OrderSize: intp(1),
		SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.5280527),
	}
	group := groupSignalRows(anchor, []*repository.StrategyEventRow{anchor, openRow})
	if len(group) != 2 {
		t.Fatalf("同一次触发应归为 2 条判定, got %d", len(group))
	}
}

// 同一分钟内的第二次触发必须是独立信号——实测单分钟最多 5 次，
// 合并会直接丢掉约一半触发（需求大纲 §3.3）。
func TestGroupSignalRowsSeparatesSecondTriggerInSameMinute(t *testing.T) {
	first := &repository.StrategyEventRow{
		Id: 1, Ts: "2026-08-18 10:15:03", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountA", Event: "open", Side: strp("long"),
		SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.52),
	}
	second := &repository.StrategyEventRow{
		Id: 2, Ts: "2026-08-18 10:15:41", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountA", Event: "open", Side: strp("long"),
		SigLast: fltp(64091.2), SigMark: fltp(64060), GapBp: fltp(4.87),
	}
	group := groupSignalRows(first, []*repository.StrategyEventRow{first, second})
	if len(group) != 1 || group[0].Id != 1 {
		t.Fatalf("同分钟第二次触发不得并入第一次: %+v", group)
	}
	group = groupSignalRows(second, []*repository.StrategyEventRow{first, second})
	if len(group) != 1 || group[0].Id != 2 {
		t.Fatalf("第二次触发应能独立定位到秒: %+v", group)
	}
}

// 按实例筛选不得串数据：同秒、同报价的另一个实例事件不得混进来。
func TestGroupSignalRowsNeverCrossesInstances(t *testing.T) {
	anchor := &repository.StrategyEventRow{
		Id: 10, Ts: "2026-08-18 10:15:03", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "account1", Event: "open", Side: strp("long"),
		SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.52),
	}
	other := &repository.StrategyEventRow{
		Id: 11, Ts: "2026-08-18 10:15:03", InstanceKey: "inst-b", Instrument: "BTCUSDT",
		AccountLabel: "account1", Event: "open", Side: strp("long"),
		SigLast: fltp(64080.6), SigMark: fltp(64058), GapBp: fltp(3.52),
	}
	group := groupSignalRows(anchor, []*repository.StrategyEventRow{anchor, other})
	if len(group) != 1 || group[0].InstanceKey != "inst-a" {
		t.Fatalf("跨实例事件被误并入: %+v", group)
	}
}

// 老事件没有报价快照时退回"同一秒 + 同方向"。
func TestGroupSignalRowsFallsBackToExactSecond(t *testing.T) {
	anchor := &repository.StrategyEventRow{
		Id: 20, Ts: "2026-07-02 08:00:00", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountA", Event: "cap_skip", Side: strp("short"),
	}
	sameSecond := &repository.StrategyEventRow{
		Id: 21, Ts: "2026-07-02 08:00:00", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountB", Event: "gate_block", Side: strp("short"),
	}
	later := &repository.StrategyEventRow{
		Id: 22, Ts: "2026-07-02 08:00:05", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountC", Event: "gate_block", Side: strp("short"),
	}
	group := groupSignalRows(anchor, []*repository.StrategyEventRow{anchor, sameSecond, later})
	if len(group) != 2 {
		t.Fatalf("无报价快照时应只并同一秒的事件, got %d", len(group))
	}
}

// 详情：逐账户判定 + TG 消息 [n] / [跳过n] 明细还原。
func TestBuildSignalDetailRestoresTelegramLines(t *testing.T) {
	open := &repository.StrategyEventRow{
		Id: 101, Ts: "2026-08-31 17:13:48", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "1394537246@qq.com", Event: "open", Side: strp("long"), OrderSize: intp(1),
		Size: intp(4), Variant: strp("challenger/S400_cap8"), ConfigVersion: 37,
		SigLast: fltp(78608.5), SigMark: fltp(78558), GapBp: fltp(6.42),
	}
	blocked := &repository.StrategyEventRow{
		Id: 102, Ts: "2026-08-31 17:13:48", InstanceKey: "inst-a", Instrument: "BTCUSDT",
		AccountLabel: "accountA", Event: "gate_block", Side: strp("long"), ConfigVersion: 37,
		Variant:  strp("champion/S400_cap26_gate8"),
		GateKind: strp("reverse_gate_profit"), GateThreshold: fltp(8), GateActual: fltp(-108.7),
		Reason:  strp("盈利不足 ROI=-108.7% < 8%"),
		SigLast: fltp(78608.5), SigMark: fltp(78558), GapBp: fltp(6.42),
	}
	detail := buildSignalDetail(open, []*repository.StrategyEventRow{open, blocked})

	if detail.SignalID != 101 {
		t.Errorf("signalId 应取组内最小 id, got %d", detail.SignalID)
	}
	if detail.Direction != "UP" {
		t.Errorf("gapBp>0 应还原成 UP, got %q", detail.Direction)
	}
	if detail.StrengthLevel != StrengthWeak {
		t.Errorf("6.42bp 应落在弱档, got %q", detail.StrengthLevel)
	}
	if detail.AccountCount != 2 || detail.OpenedCount != 1 || detail.BlockedCount != 1 {
		t.Errorf("逐账户计数有误: %+v", detail)
	}
	if detail.TotalOrderQty != 1 {
		t.Errorf("总张数有误: %d", detail.TotalOrderQty)
	}
	if len(detail.TelegramLines) != 2 {
		t.Fatalf("TG 明细行数有误: %v", detail.TelegramLines)
	}
	if detail.TelegramLines[0] != "[1] 🔵 1394537246@qq.com [long 1张]  均价:—  成交:—" {
		t.Errorf("成交行还原有误: %q", detail.TelegramLines[0])
	}
	if detail.TelegramLines[1] != "[跳过1] 🚦门控 accountA 盈利不足 ROI=-108.7% < 8%" {
		t.Errorf("跳过行还原有误: %q", detail.TelegramLines[1])
	}
	// 门控跳过行的 ROI 与阈值必须结构化可读（需求大纲 §6.1 验收 1）
	gate := detail.Accounts[1]
	if gate.GateKind != "reverse_gate_profit" || gate.GateThreshold == nil || *gate.GateThreshold != 8 ||
		gate.GateActual == nil || *gate.GateActual != -108.7 {
		t.Errorf("门控三元组丢失: %+v", gate)
	}
	if detail.FillPriceAvailable {
		t.Error("成交价埋点未落库，fillPriceAvailable 必须为 false")
	}
	if len(detail.ConfigVersions) != 1 || detail.ConfigVersions[0] != 37 {
		t.Errorf("参数版本汇总有误: %v", detail.ConfigVersions)
	}
	if len(detail.Variants) != 2 {
		t.Errorf("变体汇总有误: %v", detail.Variants)
	}
}

func TestSignalEventDTODerivations(t *testing.T) {
	row := &repository.StrategyEventRow{
		Id: 7, Ts: "2026-08-18 10:15:03", Event: "cap_skip", GapBp: fltp(-9.4),
		GateKind: strp("cap"), Source: 2,
	}
	got := signalEventDTO(row)
	if got.ResultKind != ResultKindBlocked {
		t.Errorf("cap_skip 应归 blocked, got %q", got.ResultKind)
	}
	if got.Direction != "DOWN" {
		t.Errorf("gapBp<0 应还原成 DOWN, got %q", got.Direction)
	}
	if got.StrengthLevel != StrengthStrong {
		t.Errorf("|-9.4| 应落在强档, got %q", got.StrengthLevel)
	}
	if got.GateLabel != "仓位上限" || got.EventLabel != "上限跳过" {
		t.Errorf("展示名有误: %+v", got)
	}
	if got.SourceLabel != "回灌" {
		t.Errorf("来源标记有误: %q", got.SourceLabel)
	}
}
