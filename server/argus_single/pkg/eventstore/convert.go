package eventstore

import (
	"encoding/json"
	"strings"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/marketslice"
)

// Envelope 一条待入库的事件 + 投递时刻的实例上下文。
//
// uid 与 configVersion 在 Emit 时刻（交易 goroutine 内、一次原子读）就取好，
// 不留到 writer goroutine——否则一次配置热更新会把队列里积压的旧事件
// 标成新版本号，归因口径立刻失真。
type Envelope struct {
	Event         eventlog.Event
	InstanceKey   string
	ConfigVersion uint64
	UID           string
	Source        int8
	// Slice 非 nil 时本信封承载的是一条秒级切片（r3）而不是 eventlog 事件。
	// 复用同一条队列而不另开一条：切片与事件共享"非阻塞入队 + 批量 INSERT +
	// 失败只计数"这套背压策略，另开一套只会多一份需要同步维护的丢弃口径。
	// 频率上也不需要——切片是每次触发一条（实测 613 条/4 天）。
	Slice *marketslice.Slice
}

// Rows 一批信封按目标表分好的行。
type Rows struct {
	Strategy []*StrategyEvent
	Balance  []*BalanceSample
	Dev      []*DevSample
	Slice    []*SignalSlice
}

// Len 四张表的总行数。
func (r Rows) Len() int {
	return len(r.Strategy) + len(r.Balance) + len(r.Dev) + len(r.Slice)
}

// ErrKind 转换失败的原因分类，供丢弃计数分组。
type ErrKind int

const (
	ErrNone ErrKind = iota
	ErrBadTs
	ErrBadHash
	ErrUnknownEvent
)

// Convert 把一条 Envelope 转成对应表的行。纯函数，直写与回灌共用。
//
// 空值口径：JSONL 侧全部业务字段都带 omitempty，零值与缺失在 JSON 里编码相同
// （实测业务事件里没有任何字段出现过真零值）。因此这里对可空列一律"零值 ⇒ NULL"，
// 让 DB 与 JSONL 的可判定信息完全等价——否则 r6 的一致性校验会逐行报差异。
// 需要区分"已知零"的两个字段（equity / net_size）走独立的 known 标记列。
func Convert(env Envelope, ingestedAt time.Time) (Rows, ErrKind) {
	if env.Slice != nil {
		row, ok := ConvertSlice(env, ingestedAt)
		if !ok {
			return Rows{}, ErrBadTs
		}
		return Rows{Slice: []*SignalSlice{row}}, ErrNone
	}

	e := env.Event
	ts, err := ParseTs(e.Ts)
	if err != nil {
		return Rows{}, ErrBadTs
	}
	hash, ok := EventHash(env.InstanceKey, e)
	if !ok {
		return Rows{}, ErrBadHash
	}
	source := env.Source
	if source == 0 {
		source = SourceLive
	}

	switch e.Event {
	case eventlog.EvBalance:
		// equity 已知时必须原值入库，包含 0 与负值——那恰恰是最极端的回撤样本
		// （浮亏恰抵平余额 / 权益打穿）。按"零值 ⇒ NULL"处理会把它们连同
		// equity_known=1 一起写成矛盾的行。upl 同理（空仓时真的是 0）。
		equityKnown := EquityKnown(e)
		row := &BalanceSample{
			EventHash:     hash,
			Ts:            ts,
			InstanceKey:   env.InstanceKey,
			ConfigVersion: env.ConfigVersion,
			UID:           env.UID,
			AccountLabel:  e.Account,
			Variant:       nonEmptyPtr(e.Variant),
			Balance:       e.Balance,
			Equity:        floatPtrWhen(equityKnown, e.Equity),
			Upl:           floatPtrWhen(equityKnown, e.Upl),
			EquityKnown:   boolToTinyint(equityKnown),
			NetSize:       intPtr(e.Size),
			// 直写路径的 Size 是 Go 的 int 真值（0 就是已知空仓），恒为已知；
			// 回灌路径按天自适应判定（r6），因此这里由调用方通过 Source 区分。
			NetSizeKnown: boolToTinyint(source == SourceLive),
			Source:       source,
			IngestedAt:   ingestedAt,
		}
		return Rows{Balance: []*BalanceSample{row}}, ErrNone

	case eventlog.EvDevSample:
		row := &DevSample{
			EventHash:     hash,
			Ts:            ts,
			InstanceKey:   env.InstanceKey,
			ConfigVersion: env.ConfigVersion,
			Instrument:    NormalizeInstrument(e.InstId),
			InstIdRaw:     nonEmptyPtr(e.InstId),
			DevTicks:      e.DevTicks,
			DevMaxBp:      nonZeroPtr(e.DevMaxBp),
			DevMeanBp:     nonZeroPtr(e.DevMeanBp),
			DevCrossJson:  marshalCounts(e.DevCross),
			DevOverJson:   marshalCounts(e.DevOver),
			Source:        source,
			IngestedAt:    ingestedAt,
		}
		return Rows{Dev: []*DevSample{row}}, ErrNone

	case eventlog.EvOpen, eventlog.EvCapSkip, eventlog.EvGateBlock, eventlog.EvTrendSkip,
		eventlog.EvTrailingClose, eventlog.EvCatastropheStop, eventlog.EvFixedClose,
		eventlog.EvManualClose, eventlog.EvExternalClose, eventlog.EvLossAlert:
		row := &StrategyEvent{
			EventHash:     hash,
			Ts:            ts,
			InstanceKey:   env.InstanceKey,
			ConfigVersion: env.ConfigVersion,
			UID:           env.UID,
			AccountLabel:  e.Account,
			Variant:       nonEmptyPtr(e.Variant),
			Event:         e.Event,
			Instrument:    NormalizeInstrument(e.InstId),
			InstIdRaw:     nonEmptyPtr(e.InstId),
			Side:          nonEmptyPtr(e.Side),
			NetSide:       nonEmptyPtr(e.NetSide),
			Size:          nonZeroIntPtr(e.Size),
			OrderSize:     nonZeroIntPtr(e.OrderSize),
			AvgPx:         nonZeroPtr(e.AvgPx),
			LastPx:        nonZeroPtr(e.LastPx),
			RoiPct:        nonZeroPtr(e.RoiPct),
			Pnl:           nonZeroPtr(e.Pnl),
			PeakPct:       nonZeroPtr(e.PeakPct),
			SigLast:       nonZeroPtr(e.SigLast),
			SigMark:       nonZeroPtr(e.SigMark),
			GapBp:         nonZeroPtr(e.GapBp),
			TrendMomPct:   nonZeroPtr(e.TrendMomPct),
			Reason:        nonEmptyPtr(e.Reason),
			Source:        source,
			IngestedAt:    ingestedAt,
		}
		if gate, ok := ParseGate(e.Event, e.Reason, e); ok {
			row.GateKind = nonEmptyPtr(gate.Kind)
			row.GateThreshold = gate.Threshold
			row.GateActual = gate.Actual
		}
		return Rows{Strategy: []*StrategyEvent{row}}, ErrNone

	default:
		return Rows{}, ErrUnknownEvent
	}
}

// EquityKnown equity 是否为已知样本。口径与 eventlog 聚合侧一致
// （report.go: known := e.EquityKnown || e.Equity > 0）：老日志没有
// equityKnown 字段，只能回退正数判断；新日志一律读显式标记，这样
// equity=0（浮亏恰抵平余额）与负权益——恰是最极端的回撤样本——不会丢。
func EquityKnown(e eventlog.Event) bool {
	return e.EquityKnown || e.Equity > 0
}

func marshalCounts(m map[string]int) *string {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	s := string(b)
	return &s
}

func nonEmptyPtr(s string) *string {
	v := strings.TrimSpace(s)
	if v == "" {
		return nil
	}
	return &v
}

func nonZeroPtr(f float64) *float64 {
	if f == 0 {
		return nil
	}
	v := f
	return &v
}

func nonZeroIntPtr(i int) *int {
	if i == 0 {
		return nil
	}
	v := i
	return &v
}

// floatPtrWhen 已知时保留真值（含 0 与负值），未知时留 NULL。
func floatPtrWhen(known bool, f float64) *float64 {
	if !known {
		return nil
	}
	v := f
	return &v
}

// intPtr 保留真零（balance 的 net_size：0=已知空仓，由 net_size_known 标记可信度）。
func intPtr(i int) *int {
	v := i
	return &v
}

func boolToTinyint(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// ConvertSlice 把一条秒级切片信封转成 signal_slice 行。纯函数，与事件转换同规矩。
//
// 三条序列固定序列化成等长 JSON 数组（含 null 元素），不做压缩也不裁掉空洞：
// 前端要按 offsetSec 逐秒对位画图，长度一变就得在读侧重建索引；null 元素本身
// 就是"这一秒没有行情"这条信息，压掉它等于把断流抹平。
func ConvertSlice(env Envelope, ingestedAt time.Time) (*SignalSlice, bool) {
	sl := env.Slice
	if sl == nil || sl.Anchor.IsZero() {
		return nil, false
	}
	source := env.Source
	if source == 0 {
		source = SourceLive
	}
	// 锚点按秒截断：strategy_event.ts 是秒精度，切片必须能按 (instance, instrument, ts)
	// 与那条触发事件对上，留着纳秒会让唯一键与关联查询同时失效。
	anchor := sl.Anchor.Truncate(time.Second)
	half := sl.HalfWindow
	if half <= 0 {
		half = marketslice.HalfWindowSeconds
	}
	return &SignalSlice{
		Ts:            anchor,
		InstanceKey:   env.InstanceKey,
		ConfigVersion: env.ConfigVersion,
		Instrument:    NormalizeInstrument(sl.InstIdRaw),
		InstIdRaw:     nonEmptyPtr(sl.InstIdRaw),
		StartAt:       anchor.Add(-time.Duration(half) * time.Second),
		EndAt:         anchor.Add(time.Duration(half) * time.Second),
		HalfWindowSec: half,
		SeriesPoints:  len(sl.DcLast),
		DcPoints:      sl.DcPoints,
		BinPoints:     sl.BinPoints,
		DcLastJson:    marshalSeries(sl.DcLast),
		DcMarkJson:    marshalSeries(sl.DcMark),
		BinLastJson:   marshalSeries(sl.BinLast),
		Source:        source,
		IngestedAt:    ingestedAt,
	}, true
}

// marshalSeries 序列化一条逐秒序列。序列化失败（实践中不可达：只有 *float64）
// 回落成空数组而不是空串——json 列拒绝非法 JSON，整行会插不进去。
func marshalSeries(series []*float64) string {
	if len(series) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(series)
	if err != nil {
		return "[]"
	}
	return string(raw)
}
