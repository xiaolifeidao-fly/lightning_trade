package eventstore

import (
	"testing"
	"time"

	"argus_single/pkg/eventlog"
)

func envelopeOf(e eventlog.Event) Envelope {
	return Envelope{Event: e, InstanceKey: "argus-single-roc", ConfigVersion: 42, UID: "9558450", Source: SourceLive}
}

func TestConvertOpenEvent(t *testing.T) {
	e := eventlog.Event{
		Ts: "2026-08-21 14:03:07", Account: "账户A-1394537246@qq.com", Variant: "champion/S400_cap26_gate8",
		InstId: "BTCUSDT", Event: eventlog.EvOpen, Side: "short", Size: 12, OrderSize: 1,
		SigLast: 113001.5, SigMark: 112945.2, GapBp: 5.01,
	}
	rows, kind := Convert(envelopeOf(e), time.Now())
	if kind != ErrNone || len(rows.Strategy) != 1 {
		t.Fatalf("kind=%v strategy=%d", kind, len(rows.Strategy))
	}
	row := rows.Strategy[0]
	if row.InstanceKey != "argus-single-roc" || row.ConfigVersion != 42 || row.UID != "9558450" {
		t.Fatalf("实例维度未落到行上: %+v", row)
	}
	if row.AccountLabel != e.Account {
		t.Fatalf("account_label 必须保留 JSONL 原串")
	}
	if row.Instrument != "BTCUSDT" || row.InstIdRaw == nil || *row.InstIdRaw != "BTCUSDT" {
		t.Fatalf("instrument 归一化或原值保留有误: %+v", row)
	}
	if row.Ts.Format(TsLayout) != e.Ts {
		t.Fatalf("ts 必须与 JSONL 逐字一致: %s vs %s", row.Ts.Format(TsLayout), e.Ts)
	}
	if row.Source != SourceLive || len(row.EventHash) != 16 {
		t.Fatalf("source/hash 有误: %+v", row)
	}
	if row.GateKind != nil {
		t.Fatalf("open 不是门控事件，gate_kind 必须为 NULL")
	}
	if row.Size == nil || *row.Size != 12 || row.OrderSize == nil || *row.OrderSize != 1 {
		t.Fatalf("张数字段丢失: %+v", row)
	}
}

// 持仓侧事件的 instId 是 BTC-USDT-SWAP（实测 100% 分裂），
// 入库要归一化，同时保留原值供对账。
func TestConvertCloseEventNormalizesInstrument(t *testing.T) {
	e := eventlog.Event{
		Ts: "2026-08-21 16:41:00", Account: "账户B-mortypeng@gmail.com", InstId: "BTC-USDT-SWAP",
		Event: eventlog.EvTrailingClose, Side: "short", Size: 8, AvgPx: 112000, LastPx: 111000,
		RoiPct: 111.5, Pnl: 34.2, PeakPct: 150.3, Reason: "移动止盈",
	}
	rows, kind := Convert(envelopeOf(e), time.Now())
	if kind != ErrNone {
		t.Fatalf("kind=%v", kind)
	}
	row := rows.Strategy[0]
	if row.Instrument != "BTCUSDT" {
		t.Fatalf("instrument=%q", row.Instrument)
	}
	if row.InstIdRaw == nil || *row.InstIdRaw != "BTC-USDT-SWAP" {
		t.Fatalf("inst_id_raw 必须保留原值，否则对账逐行报不匹配")
	}
	if row.PeakPct == nil || *row.PeakPct != 150.3 {
		t.Fatalf("peak_pct 丢失")
	}
}

func TestConvertGateBlockStructuresReason(t *testing.T) {
	e := eventlog.Event{
		Ts: "2026-08-21 14:05:11", Account: "账户A-1394537246@qq.com", InstId: "BTCUSDT",
		Event: eventlog.EvGateBlock, Side: "long", NetSide: "short", Size: 20,
		RoiPct: -12.3, OrderSize: 1, Reason: "盈利不足 ROI=-12.3% < 8%",
	}
	rows, _ := Convert(envelopeOf(e), time.Now())
	row := rows.Strategy[0]
	if row.GateKind == nil || *row.GateKind != GateKindReverseGateProfit {
		t.Fatalf("gate_kind 未结构化: %+v", row.GateKind)
	}
	if row.GateThreshold == nil || *row.GateThreshold != 8 || row.GateActual == nil || *row.GateActual != -12.3 {
		t.Fatalf("阈值/实际值未结构化: %+v %+v", row.GateThreshold, row.GateActual)
	}
	if row.Reason == nil || *row.Reason != e.Reason {
		t.Fatalf("原始 reason 必须保留，用于与 TG 消息逐字对账")
	}
}

func TestConvertBalanceKeepsEquityKnownAndKnownZeroNetSize(t *testing.T) {
	// equity=0（浮亏恰抵平余额）是最极端的回撤样本，不得被 omitempty 的零值判断丢掉。
	e := eventlog.Event{
		Ts: "2026-08-21 14:04:00", Account: "账户A-1394537246@qq.com", Event: eventlog.EvBalance,
		Balance: 500.5, Equity: 0, Upl: -500.5, EquityKnown: true, Size: 0,
	}
	rows, kind := Convert(envelopeOf(e), time.Now())
	if kind != ErrNone || len(rows.Balance) != 1 {
		t.Fatalf("kind=%v balance=%d", kind, len(rows.Balance))
	}
	row := rows.Balance[0]
	if row.EquityKnown != 1 {
		t.Fatalf("equity_known 必须为 1")
	}
	// equity=0 是已知样本，不能被"零值 ⇒ NULL"吃掉，否则 known=1 与 NULL 自相矛盾。
	if row.Equity == nil || *row.Equity != 0 {
		t.Fatalf("已知的零权益被写成 NULL: %v", row.Equity)
	}
	if row.Upl == nil || *row.Upl != -500.5 {
		t.Fatalf("upl 丢失: %v", row.Upl)
	}
	if row.Balance != 500.5 {
		t.Fatalf("balance=%v", row.Balance)
	}
	// 直写路径的 Size 是 Go 的 int 真值：0 就是已知空仓，不是"字段未上线"。
	if row.NetSize == nil || *row.NetSize != 0 || row.NetSizeKnown != 1 {
		t.Fatalf("已知空仓被当成未知: netSize=%v known=%d", row.NetSize, row.NetSizeKnown)
	}
}

func TestConvertBalanceEquityUnknownWhenAbsent(t *testing.T) {
	e := eventlog.Event{Ts: "2026-08-21 14:04:00", Account: "账户A", Event: eventlog.EvBalance, Balance: 500.5}
	rows, _ := Convert(envelopeOf(e), time.Now())
	row := rows.Balance[0]
	if row.EquityKnown != 0 || row.Equity != nil || row.Upl != nil {
		t.Fatalf("UPL 缓存陈旧时必须诚实地留 NULL: %+v", row)
	}
}

// 老日志（2026-07-21 之前）没有 equityKnown 字段，只能回退 equity>0 判断，
// 口径与 eventlog 聚合侧（report.go）一致。
func TestEquityKnownLegacyFallback(t *testing.T) {
	if !EquityKnown(eventlog.Event{Equity: 812.4}) {
		t.Fatalf("老日志的正权益应判为已知")
	}
	if EquityKnown(eventlog.Event{}) {
		t.Fatalf("无 equity 无标记应判为未知")
	}
}

func TestConvertDevSample(t *testing.T) {
	e := eventlog.Event{
		Ts: "2026-08-21 14:04:10", Event: eventlog.EvDevSample, InstId: "BTCUSDT", DevTicks: 97,
		DevCross: map[string]int{"3": 7, "5": 2}, DevOver: map[string]int{"3": 12},
		DevMaxBp: 9.31, DevMeanBp: 1.02,
	}
	rows, kind := Convert(envelopeOf(e), time.Now())
	if kind != ErrNone || len(rows.Dev) != 1 {
		t.Fatalf("kind=%v dev=%d", kind, len(rows.Dev))
	}
	row := rows.Dev[0]
	if row.DevTicks != 97 || row.DevCrossJson == nil || *row.DevCrossJson != `{"3":7,"5":2}` {
		t.Fatalf("dev 载荷有误: ticks=%d cross=%v", row.DevTicks, row.DevCrossJson)
	}
	if row.DevOverJson == nil || *row.DevOverJson != `{"3":12}` {
		t.Fatalf("dev_over 有误: %v", row.DevOverJson)
	}
	if row.InstanceKey != "argus-single-roc" || row.ConfigVersion != 42 {
		t.Fatalf("dev_sample 也必须带实例与版本维度: %+v", row)
	}
}

// dev_sample 与 balance 不能混进 strategy_event：分表的全部意义就在这。
func TestConvertRoutesEventsToOwnTables(t *testing.T) {
	routes := map[string]func(Rows) bool{
		eventlog.EvBalance:   func(r Rows) bool { return len(r.Balance) == 1 && len(r.Strategy) == 0 && len(r.Dev) == 0 },
		eventlog.EvDevSample: func(r Rows) bool { return len(r.Dev) == 1 && len(r.Strategy) == 0 && len(r.Balance) == 0 },
	}
	for event, check := range routes {
		rows, _ := Convert(envelopeOf(eventlog.Event{Ts: "2026-08-21 14:04:00", Event: event, Account: "账户A"}), time.Now())
		if !check(rows) {
			t.Fatalf("%s 落错表: %+v", event, rows)
		}
	}
	// 10 类业务事件全部进 strategy_event，一个不漏（12 类 = 10 + balance + dev_sample）。
	business := []string{
		eventlog.EvOpen, eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip,
		eventlog.EvTrailingClose, eventlog.EvCatastropheStop, eventlog.EvFixedClose,
		eventlog.EvManualClose, eventlog.EvExternalClose, eventlog.EvLossAlert,
	}
	for _, event := range business {
		rows, kind := Convert(envelopeOf(eventlog.Event{Ts: "2026-08-21 14:04:00", Event: event, Account: "账户A", InstId: "BTCUSDT"}), time.Now())
		if kind != ErrNone || len(rows.Strategy) != 1 {
			t.Fatalf("%s 未入 strategy_event: kind=%v", event, kind)
		}
	}
}

func TestConvertRejectsBadTsAndUnknownEvent(t *testing.T) {
	if _, kind := Convert(envelopeOf(eventlog.Event{Ts: "坏时间", Event: eventlog.EvOpen}), time.Now()); kind != ErrBadTs {
		t.Fatalf("坏时间戳应被识别为 ErrBadTs")
	}
	if _, kind := Convert(envelopeOf(eventlog.Event{Ts: "2026-08-21 14:04:00", Event: "brand_new_event"}), time.Now()); kind != ErrUnknownEvent {
		t.Fatalf("未知事件类型应被识别并计数，而不是静默落错表")
	}
}

// 空值口径：JSONL 全字段带 omitempty，零值与缺失编码相同，
// 所以可空列一律"零值 ⇒ NULL"，让 DB 与 JSONL 的可判定信息等价。
func TestConvertLeavesAbsentFieldsNull(t *testing.T) {
	e := eventlog.Event{Ts: "2026-08-21 14:04:00", Account: "账户A", Event: eventlog.EvLossAlert, InstId: "BTC-USDT-SWAP", Size: 20, RoiPct: -213.4}
	rows, _ := Convert(envelopeOf(e), time.Now())
	row := rows.Strategy[0]
	for name, isNil := range map[string]bool{
		"avg_px":        row.AvgPx == nil,
		"last_px":       row.LastPx == nil,
		"peak_pct":      row.PeakPct == nil,
		"sig_last":      row.SigLast == nil,
		"gap_bp":        row.GapBp == nil,
		"trend_mom_pct": row.TrendMomPct == nil,
		"order_size":    row.OrderSize == nil,
		"reason":        row.Reason == nil,
		"variant":       row.Variant == nil,
	} {
		if !isNil {
			t.Fatalf("%s 未落成 NULL", name)
		}
	}
}
