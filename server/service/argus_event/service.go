// Package argus_event 是 Argus 四个前端页面（总览 / 实例对比、行情触发点、
// 信号复盘、回测与寻优）的统一读服务。
//
// 它只读 argus_single 双写进来的三张事实表，不写任何数据、不触碰实盘链路。
//
// 两条贯穿全包的口径，改代码前先读：
//
//   - **时间是本地墙钟串**。入参与出参都用 `2006-01-02 15:04:05`，与 JSONL、
//     Telegram 消息、strategy_event.ts 逐字一致，不做时区换算。原因见
//     repository 包注释：manager-api 的 sqlconn 不保证带 loc=Local，
//     一旦让 time.Time 过一遍驱动就可能整体偏 8 小时。
//
//   - **实例是硬隔离维度**。账户唯一性是 (instance_key, account_label)，
//     实例1 的 account1 与实例3 的 account1 是两个人；净仓、权益、开仓率这些
//     指标跨实例相加没有意义。因此：净持仓与权益类接口强制单实例，
//     列表与聚合类接口允许"全部实例"但必须把 instance_key 带回前端，
//     并在返回体上挂不可比提示。
package argus_event

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"common/base/dto"
	"common/middleware/db"
	argusConfig "service/argus_config"
	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
	tradeRepository "service/trade/repository"

	"argus_single/pkg/trade"
)

var (
	// ErrInstanceKeyRequired 净仓 / 权益类接口必须指定实例，见包注释。
	ErrInstanceKeyRequired = errors.New("instanceKey is required for per-instance metrics")
	// ErrFilterConflict 结果筛选与事件类型筛选取交集后为空。
	ErrFilterConflict = errors.New("result filter conflicts with event filter")
	// ErrEventNotFound 锚点事件不存在。
	ErrEventNotFound = errors.New("strategy event not found")
	// ErrEpisodeNotFound 持仓 episode 不存在。派生表是离线重建的，
	// "查不到"既可能是 id 错了，也可能是这条持仓还没被派生出来。
	ErrEpisodeNotFound = errors.New("episode not found")
)

const (
	// eventTimeLayout 与 eventlog.TsLayout 同格式；本地墙钟，秒精度。
	eventTimeLayout = "2006-01-02 15:04:05"

	defaultPageSize = 20
	maxPageSize     = 200

	// maxAggregateRows 聚合类接口一次最多扫的行数。全库实测 16.6 万条事件
	// （其中业务事件仅 9%），这个上限在正常窗口下够用；命中时返回体会挂
	// truncated 标记，不静默截断。
	maxAggregateRows = 200000

	// signalGroupWindowSec 同一次触发的判定聚合窗口。
	// 各账户在独立 goroutine 里判定，拦截类事件在毫秒内落地，而成交类要等
	// 下单往返，实测可比拦截晚数秒，所以不能按"ts 完全相等"归组；
	// 又因为相邻触发的间隔中位只有 61 秒，窗口也不能开大。30 秒是折中，
	// 真正的归组依据是报价快照三元组（见 sameSignalQuote）。
	signalGroupWindowSec = 30

	// defaultSliceWindowSec 触发瞬间秒级切片的默认半窗（需求大纲 §3.1 的 ±1min）。
	defaultSliceWindowSec = 60
	maxSliceWindowSec     = 300
)

// ArgusEventService Argus 事件读服务。
type ArgusEventService struct {
	strategyEventRepository *repository.StrategyEventRepository
	balanceRepository       *repository.BalanceSampleRepository
	devSampleRepository     *repository.DevSampleRepository
	// signalSliceRepository r3 的逐秒切片。它是 90 天滚动的派生表，
	// 查不到是正常态，读侧必须能降级回 strategy_event 观测点（见 slice.go）。
	signalSliceRepository *repository.SignalSliceRepository
	// klineRepository 复用 trade 域的 K 线仓储：trade_kline 由 r2 拥有，
	// 在本域再定义一遍实体必然漂移。只读，不建表、不回填。
	klineRepository *tradeRepository.TradeKlineRepository
	// episodeRepository / episodeEntryRepository 读 r10 派生出来的两张表。
	// 只读：派生表的所有者是 argus-episode-rebuild CLI，本服务不写一个字节。
	episodeRepository      *repository.EpisodeRepository
	episodeEntryRepository *repository.EpisodeEntryRepository
	// configService 只用来取 argus_instance 的展示名与启用状态。
	configService *argusConfig.ArgusConfigService
}

func NewArgusEventService() *ArgusEventService {
	return &ArgusEventService{
		strategyEventRepository: db.GetRepository[repository.StrategyEventRepository](),
		balanceRepository:       db.GetRepository[repository.BalanceSampleRepository](),
		devSampleRepository:     db.GetRepository[repository.DevSampleRepository](),
		signalSliceRepository:   db.GetRepository[repository.SignalSliceRepository](),
		klineRepository:         db.GetRepository[tradeRepository.TradeKlineRepository](),
		episodeRepository:       db.GetRepository[repository.EpisodeRepository](),
		episodeEntryRepository:  db.GetRepository[repository.EpisodeEntryRepository](),
		configService:           argusConfig.NewArgusConfigService(),
	}
}

// EnsureTable 见 repository.StrategyEventRepository.EnsureTable 的说明。
//
// episode 两张派生表一并建：复盘页的 episode 视角在没跑过重建时应该看到
// 一张带"还没派生"提示的空表，而不是一个未知表的 500。
func (s *ArgusEventService) EnsureTable() error {
	if err := s.strategyEventRepository.EnsureTable(); err != nil {
		return err
	}
	return s.episodeRepository.EnsureTable()
}

// ─── 时间口径 ────────────────────────────────────────────────────────────────

// parseEventTime 解析一个本地墙钟串。
//
// 与 service/trade 的 parseTimeFlexible（按 UTC）刻意不同：本服务的时间锚是
// strategy_event.ts，它是 JSONL 里那串无时区文本按 time.Local 解析出来的。
// 让人为了查"8/18 00:00 那批触发"先换算成 UTC，是必然出错的设计。
// 带偏移量的 RFC3339 按其自带偏移解析后落回本地墙钟。
func parseEventTime(raw string, endOfDay bool) (time.Time, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return time.Time{}, fmt.Errorf("时间不能为空")
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.In(time.Local), nil
	}
	if t, err := time.ParseInLocation(eventTimeLayout, v, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", v, time.Local); err == nil {
		if endOfDay {
			return t.Add(59 * time.Second), nil
		}
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		if endOfDay {
			// 只给日期时，右界补到当天最后一秒——否则"查 8/18"会只命中 00:00:00 一秒。
			return t.Add(24*time.Hour - time.Second), nil
		}
		return t, nil
	}
	return time.Time{}, fmt.Errorf("时间格式应为 RFC3339 或 'YYYY-MM-DD[ HH:mm[:ss]]'（本地时区，与事件 ts 同口径）, got %q", raw)
}

func formatEventTime(t time.Time) string { return t.Format(eventTimeLayout) }

// normalizeBound 把入参时间归一成查询用的墙钟串；空串原样返回。
func normalizeBound(raw string, endOfDay bool) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	t, err := parseEventTime(raw, endOfDay)
	if err != nil {
		return "", err
	}
	return formatEventTime(t), nil
}

// resolveWindow 定出一次聚合查询实际使用的窗口。
//
// 两端都没给时，窗口锚在**已入库数据的最新事件时刻**上回推 defaultSpan，
// 而不是服务器当前时间：本库的主力数据是 6–8 月的回灌历史，按 now-24h
// 兜底只会给出一张空白页。
func (s *ArgusEventService) resolveWindow(f repository.EventFilter, start, end string, defaultSpan time.Duration) (argusDTO.WindowDTO, error) {
	if start != "" && end != "" {
		return argusDTO.WindowDTO{Start: start, End: end, Resolved: "explicit"}, nil
	}
	probe := f
	probe.Start, probe.End = "", ""
	rng, err := s.strategyEventRepository.EventTimeRange(probe)
	if err != nil {
		return argusDTO.WindowDTO{}, err
	}
	if rng.LastTs == "" {
		return argusDTO.WindowDTO{Start: start, End: end, Resolved: "empty"}, nil
	}
	last, err := parseEventTime(rng.LastTs, false)
	if err != nil {
		return argusDTO.WindowDTO{}, err
	}
	window := argusDTO.WindowDTO{Start: start, End: end, Resolved: "latest-data"}
	if window.End == "" {
		window.End = formatEventTime(last)
	}
	if window.Start == "" {
		endAt, err := parseEventTime(window.End, true)
		if err != nil {
			return argusDTO.WindowDTO{}, err
		}
		first := endAt.Add(-defaultSpan)
		if rng.FirstTs != "" {
			if firstTs, err := parseEventTime(rng.FirstTs, false); err == nil && firstTs.After(first) {
				first = firstTs
			}
		}
		window.Start = formatEventTime(first)
	}
	return window, nil
}

// ─── 入参归一 ────────────────────────────────────────────────────────────────

// splitCSV 解析逗号分隔的多值参数：去空白、丢空项、去重，保持原顺序。
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// resolveInstanceKeys 归一实例键集合：单值 + 多值合并，逐个校验格式。
// 返回空切片表示"全部实例"。
func resolveInstanceKeys(single, multi string) ([]string, error) {
	raw := splitCSV(multi)
	if v := strings.TrimSpace(single); v != "" {
		raw = append([]string{v}, raw...)
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, v := range raw {
		key, err := argusConfig.NormalizeInstanceKey(v)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}

// requireSingleInstance 净仓 / 权益类接口的实例校验：必须且只能一个。
func requireSingleInstance(raw string) (string, error) {
	keys, err := resolveInstanceKeys(raw, "")
	if err != nil {
		return "", err
	}
	if len(keys) != 1 {
		return "", ErrInstanceKeyRequired
	}
	return keys[0], nil
}

// normalizeInstruments 合约统一大写并按 eventstore 的口径去掉 -SWAP 后缀，
// 让前端传 BTC-USDT-SWAP 或 BTCUSDT 都能命中。
func normalizeInstruments(single, multi string) []string {
	raw := splitCSV(multi)
	if v := strings.TrimSpace(single); v != "" {
		raw = append([]string{v}, raw...)
	}
	out := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, v := range raw {
		inst := normalizeInstrument(v)
		if inst == "" {
			continue
		}
		if _, ok := seen[inst]; ok {
			continue
		}
		seen[inst] = struct{}{}
		out = append(out, inst)
	}
	return out
}

// normalizeInstrument 与写侧 eventstore.NormalizeInstrument 同口径。
func normalizeInstrument(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "-", "")
	s = strings.TrimSuffix(s, "SWAP")
	s = strings.TrimSuffix(s, "PERP")
	return s
}

// resolveEvents 定出要查的事件类型集合。
// events 显式给出时按它（并校验合法性）；否则按 category 展开。
func resolveEvents(category, events string) ([]string, error) {
	if explicit := splitCSV(events); len(explicit) > 0 {
		valid := make(map[string]struct{}, 10)
		for _, e := range AllEvents() {
			valid[e] = struct{}{}
		}
		for _, e := range explicit {
			if _, ok := valid[e]; !ok {
				return nil, fmt.Errorf("未知事件类型: %s", e)
			}
		}
		return explicit, nil
	}
	switch strings.TrimSpace(category) {
	case "", CategoryTrigger:
		return TriggerEvents(), nil
	case CategoryExit:
		return ExitEvents(), nil
	case CategoryAll:
		return AllEvents(), nil
	default:
		return nil, fmt.Errorf("未知事件大类: %s（可选 trigger/exit/all）", category)
	}
}

// applyResultFilter 把 result 参数收进事件类型 / 门控种类两个维度。
// result 是具体 gate_kind 时同时收窄事件集合到三类拦截事件。
func applyResultFilter(result string, events []string) (outEvents []string, gateKinds []string, err error) {
	v := strings.TrimSpace(result)
	if v == "" {
		return events, nil, nil
	}
	switch v {
	case ResultFilterOpen:
		return intersect(events, []string{"open"}), nil, nil
	case ResultFilterBlocked:
		return intersect(events, BlockedEvents()), nil, nil
	}
	for _, kind := range GateKinds() {
		if kind == v {
			return intersect(events, BlockedEvents()), []string{v}, nil
		}
	}
	return nil, nil, fmt.Errorf("未知结果筛选: %s（可选 open/blocked 或具体 gate_kind）", result)
}

func intersect(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(a))
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

// resolveStrength 把强度档位 / 显式区间翻译成 |gap_bp| 的上下界。
func resolveStrength(level string, min, max *float64) (*float64, *float64, error) {
	if min != nil || max != nil {
		return min, max, nil
	}
	v := strings.TrimSpace(level)
	if v == "" || v == "all" {
		return nil, nil, nil
	}
	lo, hi, ok := StrengthBoundsOf(v)
	if !ok {
		return nil, nil, fmt.Errorf("未知信号强度档位: %s", level)
	}
	return lo, hi, nil
}

func parseUint64List(raw string) ([]uint64, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]uint64, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("参数版本号非法: %s", p)
		}
		out = append(out, v)
	}
	return out, nil
}

func parseInt8List(raw string) ([]int8, error) {
	parts := splitCSV(raw)
	if len(parts) == 0 {
		return nil, nil
	}
	out := make([]int8, 0, len(parts))
	for _, p := range parts {
		v, err := strconv.ParseInt(p, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("数据来源标记非法: %s", p)
		}
		out = append(out, int8(v))
	}
	return out, nil
}

// buildEventFilter 把查询 DTO 翻译成仓储过滤条件。
func buildEventFilter(q argusDTO.SignalQueryDTO) (repository.EventFilter, error) {
	var f repository.EventFilter

	keys, err := resolveInstanceKeys(q.InstanceKey, q.InstanceKeys)
	if err != nil {
		return f, err
	}
	f.InstanceKeys = keys

	if f.Start, err = normalizeBound(q.Start, false); err != nil {
		return f, fmt.Errorf("start: %w", err)
	}
	if f.End, err = normalizeBound(q.End, true); err != nil {
		return f, fmt.Errorf("end: %w", err)
	}

	f.Instruments = normalizeInstruments(q.Instrument, q.Instruments)
	f.AccountLabels = append(splitCSV(q.AccountLabel), splitCSV(q.AccountLabels)...)
	f.Uids = append(splitCSV(q.Uid), splitCSV(q.Uids)...)
	f.Variants = splitCSV(q.Variants)
	f.Sides = splitCSV(q.Side)

	events, err := resolveEvents(q.Category, q.Events)
	if err != nil {
		return f, err
	}
	events, gateKinds, err := applyResultFilter(q.Result, events)
	if err != nil {
		return f, err
	}
	if len(events) == 0 {
		return f, ErrFilterConflict
	}
	f.Events = events
	f.GateKinds = gateKinds

	if f.GapBpMinAbs, f.GapBpMaxAbs, err = resolveStrength(q.Strength, q.GapBpMin, q.GapBpMax); err != nil {
		return f, err
	}
	if f.ConfigVersions, err = parseUint64List(q.ConfigVersions); err != nil {
		return f, err
	}
	if f.Sources, err = parseInt8List(q.Source); err != nil {
		return f, err
	}
	return f, nil
}

// ─── 信号列表 ────────────────────────────────────────────────────────────────

// ListSignals 多维筛选的信号列表。
//
// 分页排序固定 (ts, id)：同一秒内多条触发是常态，只按 ts 排会让翻页结果抖动；
// 也**不做任何分钟级折叠**——那会丢掉约一半触发（需求大纲 §3.3）。
func (s *ArgusEventService) ListSignals(q argusDTO.SignalQueryDTO) (*dto.PageDTO[argusDTO.SignalEventDTO], error) {
	filter, err := buildEventFilter(q)
	if err != nil {
		return nil, err
	}
	pageIndex, pageSize := q.PageIndex, q.PageSize
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	total, err := s.strategyEventRepository.CountEvents(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.strategyEventRepository.ListEvents(filter, (pageIndex-1)*pageSize, pageSize,
		strings.TrimSpace(q.Order) == "ts_asc")
	if err != nil {
		return nil, err
	}
	data := make([]*argusDTO.SignalEventDTO, 0, len(rows))
	for _, row := range rows {
		item := signalEventDTO(row)
		data = append(data, &item)
	}
	return dto.BuildPage(int(total), data), nil
}

// signalEventDTO 行 → 出参。字段一律原样透传，派生项只有四个：
// eventLabel / resultKind / direction / strengthLevel。
func signalEventDTO(row *repository.StrategyEventRow) argusDTO.SignalEventDTO {
	return argusDTO.SignalEventDTO{
		EventID:       row.Id,
		Ts:            row.Ts,
		InstanceKey:   row.InstanceKey,
		ConfigVersion: row.ConfigVersion,
		Uid:           row.Uid,
		AccountLabel:  row.AccountLabel,
		Variant:       strVal(row.Variant),
		Event:         row.Event,
		EventLabel:    EventLabel(row.Event),
		Instrument:    row.Instrument,
		InstIdRaw:     strVal(row.InstIdRaw),
		ResultKind:    ResultKindOf(row.Event),
		Side:          strVal(row.Side),
		NetSide:       strVal(row.NetSide),
		Direction:     directionOf(row.GapBp),
		Size:          row.Size,
		OrderSize:     row.OrderSize,
		AvgPx:         row.AvgPx,
		LastPx:        row.LastPx,
		RoiPct:        row.RoiPct,
		Pnl:           row.Pnl,
		PeakPct:       row.PeakPct,
		SigLast:       row.SigLast,
		SigMark:       row.SigMark,
		GapBp:         row.GapBp,
		TrendMomPct:   row.TrendMomPct,
		StrengthLevel: StrengthLevelOf(row.GapBp),
		GateKind:      strVal(row.GateKind),
		GateLabel:     GateLabel(strVal(row.GateKind)),
		GateThreshold: row.GateThreshold,
		GateActual:    row.GateActual,
		Reason:        strVal(row.Reason),
		Source:        row.Source,
		SourceLabel:   SourceLabel(row.Source),
	}
}

// directionOf 由 gap_bp 的符号还原信号方向（>0 = UP，对应开多）。
// gap_bp 为空说明信号时刻报价不可算，此时方向无法还原，返回空串而不是猜。
func directionOf(gapBp *float64) string {
	if gapBp == nil || *gapBp == 0 {
		return ""
	}
	if *gapBp > 0 {
		return trade.OrderBookSignalUp
	}
	return trade.OrderBookSignalDown
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ─── 信号详情 ────────────────────────────────────────────────────────────────

// GetSignalDetail 按锚点事件 id 还原一次触发的逐账户判定。
//
// 归组依据是**报价快照三元组** (sig_last, sig_mark, gap_bp) + side：同一次
// 触发把同一个 SignalQuote 复制给每个账户的事件（pkg/trade/signal_quote.go
// applySignalQuote），所以三元组相等即同源；ts 只用来限定搜索范围，不能当
// 归组键——成交事件要等下单往返，实测会比同一次触发的拦截事件晚数秒。
// 报价不可算（三元组为空）的老事件回退到"ts 完全相等 + side 相同"。
func (s *ArgusEventService) GetSignalDetail(eventID uint64) (*argusDTO.SignalDetailDTO, error) {
	anchor, err := s.strategyEventRepository.FindEventByID(eventID)
	if err != nil {
		return nil, err
	}
	if anchor == nil {
		return nil, ErrEventNotFound
	}
	anchorTs, err := parseEventTime(anchor.Ts, false)
	if err != nil {
		return nil, err
	}
	siblings, err := s.strategyEventRepository.ListEvents(repository.EventFilter{
		InstanceKeys: []string{anchor.InstanceKey},
		Instruments:  []string{anchor.Instrument},
		Events:       TriggerEvents(),
		Start:        formatEventTime(anchorTs.Add(-signalGroupWindowSec * time.Second)),
		End:          formatEventTime(anchorTs.Add(signalGroupWindowSec * time.Second)),
	}, 0, 0, true)
	if err != nil {
		return nil, err
	}
	group := groupSignalRows(anchor, siblings)
	detail := buildSignalDetail(anchor, group)
	return &detail, nil
}

// groupSignalRows 从候选行里挑出与锚点属于同一次触发的行，并按账户去重
// （同账户多行时取离锚点最近的一条）。返回按 (ts, id) 升序。
func groupSignalRows(anchor *repository.StrategyEventRow, candidates []*repository.StrategyEventRow) []*repository.StrategyEventRow {
	anchorTs, err := parseEventTime(anchor.Ts, false)
	if err != nil {
		return []*repository.StrategyEventRow{anchor}
	}
	best := make(map[string]*repository.StrategyEventRow)
	bestGap := make(map[string]time.Duration)
	consider := func(row *repository.StrategyEventRow) {
		if !sameSignal(anchor, row) {
			return
		}
		ts, err := parseEventTime(row.Ts, false)
		if err != nil {
			return
		}
		gap := ts.Sub(anchorTs)
		if gap < 0 {
			gap = -gap
		}
		key := row.AccountLabel
		if prev, ok := best[key]; ok {
			if gap > bestGap[key] || (gap == bestGap[key] && row.Id > prev.Id) {
				return
			}
		}
		best[key] = row
		bestGap[key] = gap
	}
	for _, row := range candidates {
		consider(row)
	}
	// 锚点自己必须在结果里，哪怕候选集因为并发写入还没覆盖到它。
	if _, ok := best[anchor.AccountLabel]; !ok {
		best[anchor.AccountLabel] = anchor
	}
	out := make([]*repository.StrategyEventRow, 0, len(best))
	for _, row := range best {
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ts != out[j].Ts {
			return out[i].Ts < out[j].Ts
		}
		return out[i].Id < out[j].Id
	})
	return out
}

// sameSignal 判断两行是否来自同一次触发。
func sameSignal(anchor, row *repository.StrategyEventRow) bool {
	if anchor.InstanceKey != row.InstanceKey || anchor.Instrument != row.Instrument {
		return false
	}
	if strVal(anchor.Side) != strVal(row.Side) {
		return false
	}
	if hasQuote(anchor) || hasQuote(row) {
		return sameSignalQuote(anchor, row)
	}
	// 报价不可算的老事件：只能退回"同一秒 + 同方向"。
	return anchor.Ts == row.Ts
}

func hasQuote(row *repository.StrategyEventRow) bool {
	return row.SigLast != nil || row.SigMark != nil || row.GapBp != nil
}

// sameSignalQuote 报价快照三元组是否同源。
// 三个字段是同一个 SignalQuote 复制出来的，落库前后都不会被单独改写，
// 因此按值比较即可；浮点比较仍留一个极小的相对容差防 DECIMAL 转换抖动。
func sameSignalQuote(a, b *repository.StrategyEventRow) bool {
	return floatPtrEqual(a.SigLast, b.SigLast) &&
		floatPtrEqual(a.SigMark, b.SigMark) &&
		floatPtrEqual(a.GapBp, b.GapBp)
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	diff := math.Abs(*a - *b)
	scale := math.Max(math.Abs(*a), math.Abs(*b))
	return diff <= 1e-9*math.Max(1, scale)
}

// buildSignalDetail 把同一次触发的行组装成详情。
func buildSignalDetail(anchor *repository.StrategyEventRow, group []*repository.StrategyEventRow) argusDTO.SignalDetailDTO {
	detail := argusDTO.SignalDetailDTO{
		SignalID:       anchor.Id,
		Ts:             anchor.Ts,
		InstanceKey:    anchor.InstanceKey,
		Instrument:     anchor.Instrument,
		InstIdRaw:      strVal(anchor.InstIdRaw),
		Side:           strVal(anchor.Side),
		SigLast:        anchor.SigLast,
		SigMark:        anchor.SigMark,
		GapBp:          anchor.GapBp,
		TrendMomPct:    anchor.TrendMomPct,
		GroupWindowSec: signalGroupWindowSec,
	}
	if len(group) > 0 {
		// signalId 取组内最小 id，点哪一行进来拿到的都是同一个信号标识。
		detail.SignalID = group[0].Id
		detail.Ts = group[0].Ts
		for _, row := range group {
			if row.Id < detail.SignalID {
				detail.SignalID = row.Id
			}
		}
	}
	detail.Direction = directionOf(detail.GapBp)
	detail.StrengthLevel = StrengthLevelOf(detail.GapBp)

	anchorTs, tsErr := parseEventTime(detail.Ts, false)
	versionSeen := map[uint64]struct{}{}
	variantSeen := map[string]struct{}{}
	openIdx, skipIdx := 0, 0
	for _, row := range group {
		decision := argusDTO.AccountDecisionDTO{
			EventID:       row.Id,
			Ts:            row.Ts,
			AccountLabel:  row.AccountLabel,
			Uid:           row.Uid,
			Variant:       strVal(row.Variant),
			Result:        row.Event,
			ResultLabel:   EventLabel(row.Event),
			ResultKind:    ResultKindOf(row.Event),
			Side:          strVal(row.Side),
			NetSide:       strVal(row.NetSide),
			OrderSize:     row.OrderSize,
			NetSize:       row.Size,
			AvgPx:         row.AvgPx,
			LastPx:        row.LastPx,
			RoiPct:        row.RoiPct,
			Pnl:           row.Pnl,
			GateKind:      strVal(row.GateKind),
			GateLabel:     GateLabel(strVal(row.GateKind)),
			GateThreshold: row.GateThreshold,
			GateActual:    row.GateActual,
			Reason:        strVal(row.Reason),
			ConfigVersion: row.ConfigVersion,
		}
		if tsErr == nil {
			if ts, err := parseEventTime(row.Ts, false); err == nil {
				decision.TsOffsetSec = int(ts.Sub(anchorTs) / time.Second)
			}
		}
		detail.Accounts = append(detail.Accounts, decision)

		versionSeen[row.ConfigVersion] = struct{}{}
		if v := strVal(row.Variant); v != "" {
			variantSeen[v] = struct{}{}
		}
		if decision.ResultKind == ResultKindOpen {
			detail.OpenedCount++
			openIdx++
			if row.OrderSize != nil {
				detail.TotalOrderQty += *row.OrderSize
			}
			detail.TelegramLines = append(detail.TelegramLines, telegramOpenLine(openIdx, detail.Direction, row))
		} else {
			detail.BlockedCount++
			skipIdx++
			detail.TelegramLines = append(detail.TelegramLines, telegramSkipLine(skipIdx, row))
		}
	}
	detail.AccountCount = len(detail.Accounts)
	detail.ConfigVersions = sortedUint64(versionSeen)
	detail.Variants = sortedStrings(variantSeen)
	detail.FillPriceAvailable = false
	detail.Notice = "均价 / 成交价来自交易所下单回执，埋点未落库，本页显示为 —；其余字段与 Telegram 消息同源。"
	return detail
}

// telegramOpenLine 还原 TG 消息里的 `[n]` 成交行。
// 格式对齐 pkg/trade/manager.go 的 sendSignalOpenSummaryToTelegram：
// 均价 / 成交来自下单回执，事件表里没有，用 — 占位而不是补 0。
func telegramOpenLine(index int, direction string, row *repository.StrategyEventRow) string {
	emoji := "🔴"
	if direction == trade.OrderBookSignalUp {
		emoji = "🔵"
	}
	size := 0
	if row.OrderSize != nil {
		size = *row.OrderSize
	}
	return fmt.Sprintf("[%d] %s %s [%s %d张]  均价:—  成交:—", index, emoji, row.AccountLabel, strVal(row.Side), size)
}

// telegramSkipLine 还原 TG 消息里的 `[跳过n]` 行，前缀与运行时一致
// （🚦门控 / 🌊趋势闸 / ⛔上限），正文用落库的 reason 原文。
func telegramSkipLine(index int, row *repository.StrategyEventRow) string {
	prefix := ""
	switch row.Event {
	case "gate_block":
		prefix = "🚦门控"
	case "trend_skip":
		prefix = "🌊趋势闸"
	case "cap_skip":
		prefix = "⛔上限"
	default:
		prefix = EventLabel(row.Event)
	}
	reason := strVal(row.Reason)
	if reason == "" {
		reason = EventLabel(row.Event)
	}
	return fmt.Sprintf("[跳过%d] %s %s %s", index, prefix, row.AccountLabel, reason)
}

func sortedUint64(set map[uint64]struct{}) []uint64 {
	out := make([]uint64, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedStrings(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
