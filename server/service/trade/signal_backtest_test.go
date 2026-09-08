package trade

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"

	"argus_single/pkg/eventstore"
)

func strp(s string) *string   { return &s }
func intp(v int) *int         { return &v }
func fltp(v float64) *float64 { return &v }

// 事件行 → 引擎输入：逐字段原样透传，不推导。
func TestSignalsFromEventsCopiesFieldsVerbatim(t *testing.T) {
	at := time.Date(2026, 8, 18, 0, 22, 16, 0, time.UTC)
	rows := []*eventstore.StrategyEvent{
		{Ts: at, Event: "open", Side: strp("long"), OrderSize: intp(1), Size: intp(3),
			GapBp: fltp(3.528052702238369), SigLast: fltp(64080.6), SigMark: fltp(64058)},
		{Ts: at.Add(time.Minute), Event: "gate_block", Side: strp("short"), OrderSize: intp(1),
			NetSide: strp("long"), Size: intp(12), AvgPx: fltp(63665.0083333333), LastPx: fltp(64043.4),
			RoiPct: fltp(74.29), Reason: strp("盈利不足 ROI=1.2% < 8%"), GateKind: strp("reverse_gate_profit")},
		// side 缺失：无法回放，必须被跳过而不是当成 long
		{Ts: at.Add(2 * time.Minute), Event: "open"},
	}
	got := signalsFromEvents(rows)
	if len(got) != 2 {
		t.Fatalf("信号数 = %d, 期望 2（side 缺失的那条应跳过）", len(got))
	}
	if got[0].Side != "long" || got[0].OrderSize != 1 || math.Abs(got[0].GapBp-3.528052702238369) > 1e-12 {
		t.Errorf("首条透传有误: %+v", got[0])
	}
	if math.Abs(got[0].SigLast-64080.6) > 1e-9 || math.Abs(got[0].SigMark-64058) > 1e-9 {
		t.Errorf("报价快照透传有误: %+v", got[0])
	}
	if got[1].NetSide != "long" || got[1].NetSize != 12 || math.Abs(got[1].AvgPx-63665.0083333333) > 1e-9 {
		t.Errorf("净仓快照透传有误: %+v", got[1])
	}
	if got[1].GateKind != "reverse_gate_profit" {
		t.Errorf("gate_kind 透传有误: %q", got[1].GateKind)
	}
}

// 种子仓从事件流推出：窗口起点已有旧仓时，第一条带净仓快照的拦截事件即种子。
// （与真实数据一致：logs/argus_single/events-0702 里账户A 的第一条业务事件
// 就是 gate_block short net=long 14 @60258.105，窗口切在持仓中间。）
func TestSeedFromEventsUsesFirstSnapshot(t *testing.T) {
	at := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	rows := []*eventstore.StrategyEvent{
		{Ts: at, Event: "gate_block", Side: strp("short"),
			NetSide: strp("long"), Size: intp(14), AvgPx: fltp(60258.1)},
		{Ts: at.Add(time.Minute), Event: "open", Side: strp("long"), OrderSize: intp(1)},
	}
	seed := signal.SeedFromSignals(signalsFromEvents(rows))
	if !seed.OK() || seed.Side != "long" || seed.Size != 14 || math.Abs(seed.AvgPx-60258.1) > 1e-9 {
		t.Fatalf("种子仓 = %+v", seed)
	}
	if !seed.At.Equal(at) {
		t.Errorf("种子时刻 = %s, 期望 %s", seed.At, at)
	}
}

// 窗口起点是空仓（先出现 open）时不得灌种子，否则同一批仓位会被开两次。
func TestSeedFromEventsSkippedWhenWindowStartsFlat(t *testing.T) {
	at := time.Date(2026, 8, 18, 1, 0, 0, 0, time.UTC)
	rows := []*eventstore.StrategyEvent{
		{Ts: at, Event: "open", Side: strp("long"), OrderSize: intp(1)},
		{Ts: at.Add(time.Minute), Event: "gate_block", Side: strp("short"),
			NetSide: strp("long"), Size: intp(3), AvgPx: fltp(60000)},
	}
	if seed := signal.SeedFromSignals(signalsFromEvents(rows)); seed.OK() {
		t.Errorf("窗口起点空仓不该推出种子仓: %+v", seed)
	}
}

// dev_sample 行 → λ 输入：devCross 的字符串 key 解析成 bp 数值。
func TestDevWindowsFromSamplesParsesCrossJson(t *testing.T) {
	at := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	cross := `{"0.5":30,"1":18,"3":6,"5":3,"10":1}`
	empty := `{}`
	rows := []*eventstore.DevSample{
		{Ts: at, DevTicks: 120, DevCrossJson: &cross},
		{Ts: at.Add(time.Minute), DevTicks: 118, DevCrossJson: &empty}, // 空 map ⇒ 跳过
		{Ts: at.Add(2 * time.Minute), DevTicks: 0},                     // 无 JSON ⇒ 跳过
	}
	got := devWindowsFromSamples(rows)
	if len(got) != 1 {
		t.Fatalf("窗口数 = %d, 期望 1", len(got))
	}
	if got[0].Cross[0.5] != 30 || got[0].Cross[3] != 6 || got[0].Cross[10] != 1 {
		t.Errorf("穿越计数解析有误: %+v", got[0].Cross)
	}
	if got[0].Ticks != 120 {
		t.Errorf("ticks = %d, 期望 120", got[0].Ticks)
	}
}

// 参数快照必须能无损往返：同一个 runId 重跑要得到同一组参数。
func TestParamsSnapshotRoundTrip(t *testing.T) {
	dto := tradeDTO.CreateSignalBacktestRunDTO{
		SignalBacktestParamsDTO: tradeDTO.SignalBacktestParamsDTO{
			Mode: signal.ModeNet, EvalMode: signal.EvalPessimistic, EntryPx: signal.EntrySigLast,
			OrderSize: intp(10), RiskEquity: fltp(375.73), BudgetPct: fltp(13.3),
			CatastropheStopPct: fltp(400), Ceiling: intp(246), GateMinProfitPct: fltp(8),
			TrendGateWindowHours: fltp(24), TrendGateThresholdPct: fltp(5),
			SmallActivatePct: fltp(150), SmallGiveback: fltp(0.35),
			CatastropheOvershootRoiPts: fltp(5), TakerFee: fltp(0.00012),
		},
	}
	p := paramsFromSignalDTO(dto)
	if err := p.Validate(); err != nil {
		t.Fatalf("参数应合法: %v", err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	back, err := signalParamsFromSnapshot(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if back != p {
		t.Errorf("快照往返不一致:\n原 %+v\n回 %+v", p, back)
	}
}

// 没传的旋钮必须回落生产缺省，而不是零值。
func TestParamsFromDTOFallsBackToLiveDefaults(t *testing.T) {
	p := paramsFromSignalDTO(tradeDTO.CreateSignalBacktestRunDTO{})
	d := signal.DefaultParams()
	if p.Leverage != d.Leverage || p.FaceValue != d.FaceValue ||
		p.CatastropheStopPct != d.CatastropheStopPct || p.GateMinProfitPct != d.GateMinProfitPct ||
		p.SmallActivatePct != d.SmallActivatePct || p.TierLargeRatio != d.TierLargeRatio {
		t.Errorf("缺省回落有误: %+v", p)
	}
	if p.Mode != signal.ModeNet || p.EvalMode != signal.EvalClose || p.EntryPx != signal.EntryBarClose {
		t.Errorf("枚举缺省有误: mode=%s eval=%s entry=%s", p.Mode, p.EvalMode, p.EntryPx)
	}
}

// 空快照必须报错：参数没冻结的 run 不可复现，不能悄悄用缺省跑出个结果。
func TestSnapshotEmptyIsRejected(t *testing.T) {
	if _, err := signalParamsFromSnapshot("  "); err == nil {
		t.Fatal("空 params_snapshot 应报错")
	}
	if _, err := signalParamsFromSnapshot("{不是json"); err == nil {
		t.Fatal("非法 params_snapshot 应报错")
	}
}

// 逐笔实体映射：一个 episode 一行，calc_mode=signal，出场归因原样落库。
func TestSignalTradeEntitiesMapping(t *testing.T) {
	at := time.Date(2026, 8, 18, 3, 0, 0, 0, time.UTC)
	res := &signal.Result{
		LastPx: 64000,
		Episodes: []signal.Episode{
			{Side: "long", OpenedAt: at, ClosedAt: at.Add(2 * time.Hour), AvgPx: 63665, ClosePx: 64043.4,
				Contracts: 12, MaxContracts: 12, AddCount: 12, RoiPct: 74.29, Pnl: 4.54, Fee: 0.9,
				PeakPct: 104.92, Reason: signal.ExitTrailing},
			{Side: "short", OpenedAt: at, ClosedAt: at.Add(time.Hour), AvgPx: 64200, ClosePx: 64000,
				Contracts: 7, MaxContracts: 8, AddCount: 8, RoiPct: -85.2, Pnl: -1.4, Fee: 0.5,
				Reason: signal.ExitEod, Open: true},
		},
	}
	rows := signalTradeEntities(7, signal.DefaultParams(), res)
	if len(rows) != 2 {
		t.Fatalf("逐笔行数 = %d, 期望 2", len(rows))
	}
	for _, r := range rows {
		if r.RunID != 7 || r.CalcMode != CalcModeSignal {
			t.Errorf("run_id/calc_mode 有误: %+v", r)
		}
	}
	if rows[0].CloseReason != signal.ExitTrailing || rows[0].Status != "closed" || rows[0].Contracts != 12 {
		t.Errorf("首行映射有误: %+v", rows[0])
	}
	if math.Abs(rows[0].PeakPct-104.92) > 1e-9 {
		t.Errorf("peak_pct 未落库: %v", rows[0].PeakPct)
	}
	if rows[0].ClosedAt == nil {
		t.Error("已平仓行应有 closed_at")
	}
	if rows[1].Status != "open" || rows[1].ClosedAt != nil {
		t.Errorf("eod 行应为 open 且无 closed_at: %+v", rows[1])
	}
	if math.Abs(rows[1].NetPnl-(-1.9)) > 1e-9 {
		t.Errorf("eod 行净盈亏 = %v, 期望 -1.9（浮动 -1.4 − 费 0.5）", rows[1].NetPnl)
	}
}

// 汇总实体映射：精度等级必须同时落在 metric 上（横向对比按行分组）。
func TestSignalMetricEntityCarriesFidelity(t *testing.T) {
	m := signal.Metric{
		TradeCount: 3, WinCount: 2, NetPnl: 12.5, SignalCount: 334, CapSkipCount: 40,
		GateSkipCount: 132, ReduceCount: 18, MaxStack: 15, Fidelity: signal.FidelityFrequency,
		FidelityNote: "阈值改动", LambdaPerDay: 2880, LambdaRatio: 0.5, LambdaSelfTest: 0.98,
	}
	e := signalMetricEntity(9, m)
	if e.RunID != 9 || e.CalcMode != CalcModeSignal {
		t.Errorf("run_id/calc_mode 有误: %+v", e)
	}
	if e.Fidelity != signal.FidelityFrequency || e.FidelityNote != "阈值改动" {
		t.Errorf("精度信息未落库: %+v", e)
	}
	if e.SignalCount != 334 || e.CapSkipCount != 40 || e.GateSkipCount != 132 || e.MaxStack != 15 {
		t.Errorf("信号侧计数未落库: %+v", e)
	}
	if e.LambdaPerDay != 2880 || e.LambdaRatio != 0.5 || e.LambdaSelfTest != 0.98 {
		t.Errorf("λ 未落库: %+v", e)
	}
	// TradeCount 是生命周期数，不是信号数——两者口径不同不能混用。
	if e.TradeCount != 3 {
		t.Errorf("trade_count = %d, 期望 3（生命周期数）", e.TradeCount)
	}
}

// engine_kind 空串（列刚加上、既有行没值）读侧回落 prediction。
func TestNormalizeEngineKindDefaultsToPrediction(t *testing.T) {
	if got := normalizeEngineKind(""); got != EngineKindPrediction {
		t.Errorf("空 engine_kind = %q, 期望 %q", got, EngineKindPrediction)
	}
	if got := normalizeEngineKind(EngineKindSignal); got != EngineKindSignal {
		t.Errorf("engine_kind 被改写: %q", got)
	}
	run := &tradeRepository.TradeBacktestRun{}
	if backtestRunToDTO(run).EngineKind != EngineKindPrediction {
		t.Error("既有 run 的 DTO 应回落 prediction")
	}
}

// 频率级候选阈值必须与生产采样器同源，否则 λ 全靠插值。
func TestThresholdCandidatesMatchLiveSampler(t *testing.T) {
	got := SignalThresholdCandidatesBp()
	want := []float64{0.5, 1, 1.5, 2, 3, 4, 5, 7, 10}
	if len(got) != len(want) {
		t.Fatalf("候选阈值 = %v, 期望 %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("候选阈值 = %v, 期望 %v", got, want)
		}
	}
}

// 回测窗口按本地时区解析：事件 ts 是 JSONL 那串无时区文本按 time.Local 解出来的
// 瞬时，窗口若按 UTC 解析就会整段错 8 小时——数据全在、图能画、只是全错。
func TestSignalWindowParsedInLocalZone(t *testing.T) {
	got, err := parseSignalWindowTime("2026-08-18 00:22:16")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 18, 0, 22, 16, 0, time.Local)
	if !got.Equal(want) {
		t.Errorf("解析结果 = %s, 期望本地时区的 %s", got, want)
	}
	// 与预测驱动的 UTC 口径必须不同（除非本机就跑在 UTC）
	utc, err := parseTimeFlexible("2026-08-18 00:22:16")
	if err != nil {
		t.Fatal(err)
	}
	if _, off := want.Zone(); off != 0 && got.Equal(utc) {
		t.Error("信号回测窗口不应与预测回测共用 UTC 口径")
	}

	// 带偏移量的 RFC3339 按其自带偏移解析
	rfc, err := parseSignalWindowTime("2026-08-18T00:22:16+08:00")
	if err != nil {
		t.Fatal(err)
	}
	if !rfc.Equal(time.Date(2026, 8, 17, 16, 22, 16, 0, time.UTC)) {
		t.Errorf("RFC3339 解析 = %s", rfc.UTC())
	}

	// 只给日期也要能用（常见输入）
	if _, err := parseSignalWindowTime("2026-08-18"); err != nil {
		t.Errorf("纯日期应可解析: %v", err)
	}
	if _, err := parseSignalWindowTime("八月十八"); err == nil {
		t.Error("非法时间串应报错")
	}
}
