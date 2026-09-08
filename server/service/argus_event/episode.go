package argus_event

// 本文件是 episode（持仓生命周期）的读服务：列表、生命周期详情、出场归因分布。
//
// 派生表由 r10 的 `argus-episode-rebuild` CLI 离线整实例重建，本服务只读。
// 三条本文件特有的口径，改代码前先读：
//
//  1. **胜率的分母是 strategy_attributable = 1**。external_close 与 manual_close
//     是交易所侧或人工操作，盈亏如实计入 pnl，但不进胜率；仍持仓的结果未定，
//     同样不进。这是任务说明明确点名的「不做」项，页面上必须写出来而不是
//     悄悄过滤。
//
//  2. **窗口口径是显式入参**。同一批持仓按 opened / closed / overlap 数出来的
//     条数不同（r10 的派生报告实测 21 / 29 两种口径差 8 笔），服务端不能偷偷
//     定一个，必须让调用方选并随返回体回显。
//
//  3. **浮盈轨迹不换算成 ROI%**。心跳里有 upl（U）没有保证金，ROI 只在事件里
//     被离散地写下来。两者分两条序列返回，不做插值也不做换算——拟合出来的
//     连续 ROI 曲线看着精确，其实是假的。

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore/episode"
	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
)

const (
	// episodeDefaultSpan 未给窗口时的默认回溯跨度。持仓比信号稀疏得多
	// （实测三个月 164 笔），24 小时会经常是空页，这里放到 7 天。
	episodeDefaultSpan = 7 * 24 * time.Hour

	// maxEpisodePageSize 单页上限，与信号列表同量级。
	maxEpisodePageSize = 200

	// maxEpisodeTimelineRows 单条 episode 的事件条数上限。实测最长的一笔
	// 68.8 小时、上千条事件仍远低于此；命中时返回体挂 eventTruncated。
	maxEpisodeTimelineRows = 5000
	// maxEpisodeUplRows 单条 episode 的心跳条数上限。心跳约 30 秒一条，
	// 5000 条约合 41 小时，超出的部分会被截断并自曝。
	maxEpisodeUplRows = 5000
)

// ExitKindStillOpen 仍持仓在筛选与聚合里的取值。派生表里它是 NULL，
// 出参与入参统一用空串；筛选参数里额外接受 'open' 这个别名。
const ExitKindStillOpen = ""

// exitKindLabels 六种出场方式的中文名，与原型 signals.html 用词一致。
var exitKindLabels = map[string]string{
	episode.ExitTrailingClose:   "移动止盈",
	episode.ExitFixedClose:      "固定止盈",
	episode.ExitCatastropheStop: "兜底止损",
	episode.ExitExternalClose:   "交易所侧平仓",
	episode.ExitManualClose:     "人工平仓",
	episode.ExitReduceToZero:    "反向减仓归零",
}

// ExitKinds 六种出场方式，顺序即前端下拉与分布图的展示顺序。
func ExitKinds() []string {
	return []string{
		episode.ExitTrailingClose,
		episode.ExitReduceToZero,
		episode.ExitFixedClose,
		episode.ExitCatastropheStop,
		episode.ExitExternalClose,
		episode.ExitManualClose,
	}
}

// ExitLabel 出场方式的展示名；空串是仍持仓，未知值原样返回不吞掉。
func ExitLabel(kind string) string {
	if kind == ExitKindStillOpen {
		return "持仓中"
	}
	if label, ok := exitKindLabels[kind]; ok {
		return label
	}
	return kind
}

// CountedInWinRate 该出场方式是否计入策略胜率。
//
// 与派生表的 strategy_attributable 同口径，在读侧再定义一次是为了让筛选项
// 能直接标注"这一类不算胜率"——它是页面上必须显示的信息，不是内部细节。
func CountedInWinRate(kind string) bool {
	switch kind {
	case episode.ExitExternalClose, episode.ExitManualClose, ExitKindStillOpen:
		return false
	default:
		return true
	}
}

// exitAttributionReason 出场归因的口径说明。
func exitAttributionReason(kind string) string {
	switch kind {
	case episode.ExitExternalClose:
		return "交易所侧平仓（强平 / 交易所动作），不是策略自己的出场决策，盈亏如实计入但不计入策略胜率。"
	case episode.ExitManualClose:
		return "人工平仓，不是策略自己的出场决策，盈亏如实计入但不计入策略胜率。"
	case ExitKindStillOpen:
		return "仍持仓，结果未定，不计入策略胜率。"
	case episode.ExitReduceToZero:
		return "没有平仓事件：反向减仓一张一张磨到 0。只认 close 事件会把这类持仓显示成永远没结束。"
	case episode.ExitTrailingClose:
		return "浮盈回撤触及分档回撤线，由策略自身的移动止盈平掉，计入策略胜率。"
	case episode.ExitCatastropheStop:
		return "浮亏触及兜底止损线，由策略自身平掉，计入策略胜率。"
	case episode.ExitFixedClose:
		return "浮盈触及固定止盈阈值，由策略自身平掉，计入策略胜率。"
	default:
		return ""
	}
}

// depthFidelityLabels 扛单深度的三档保真度。
var depthFidelityLabels = map[string]string{
	episode.DepthMinute:       "分钟级（心跳带 upl，浮盈轨迹完整）",
	episode.DepthAlertSampled: "仅告警采样（0 ~ −150% 区间无观测）",
	episode.DepthMixed:        "混合（心跳有缺口或跨越 upl 上线时点）",
}

// DepthFidelityLabel 深度保真度的展示名。
func DepthFidelityLabel(kind string) string {
	if label, ok := depthFidelityLabels[kind]; ok {
		return label
	}
	return kind
}

// eventTones 时间轴轴点的色调，集中定义一次，不让前端各写一套映射。
var eventTones = map[string]string{
	eventlog.EvOpen:            "ok",
	eventlog.EvTrailingClose:   "ok",
	eventlog.EvFixedClose:      "ok",
	eventlog.EvCatastropheStop: "err",
	eventlog.EvLossAlert:       "err",
	eventlog.EvExternalClose:   "mute",
	eventlog.EvManualClose:     "mute",
	eventlog.EvGateBlock:       "warn",
	eventlog.EvCapSkip:         "warn",
	eventlog.EvTrendSkip:       "warn",
}

func eventTone(event string) string {
	if tone, ok := eventTones[event]; ok {
		return tone
	}
	return "mute"
}

// ─── 入参归一 ────────────────────────────────────────────────────────────────

// resolveEpisodeTimeField 归一窗口口径，并给出该口径会漏掉什么的说明。
func resolveEpisodeTimeField(raw string) (string, string, error) {
	switch strings.TrimSpace(raw) {
	case "", repository.EpisodeTimeOpened:
		return repository.EpisodeTimeOpened, "按**建仓时刻**收窗口（决策时刻口径，与决策归集一致）。建仓早于窗口左界的持仓不在列表里，包括开头被数据窗口截断的那几笔。", nil
	case repository.EpisodeTimeClosed:
		return repository.EpisodeTimeClosed, "按**出场时刻**收窗口。仍持仓的不在列表里；跨窗口建仓的会算在出场那一天，按状态分层做收益归因时不要用这个口径（会造出伪影）。", nil
	case repository.EpisodeTimeOverlap:
		return repository.EpisodeTimeOverlap, "只要与窗口有交集就入选，一笔不漏；但同一笔持仓会同时出现在相邻两段窗口里，计数与求和类指标不能用这个口径横向相加。", nil
	default:
		return "", "", fmt.Errorf("未知窗口口径: %s（可选 opened/closed/overlap）", raw)
	}
}

// resolveExitKinds 归一出场方式筛选。'open' 是仍持仓的入参别名，
// 它落到 status 而不是 exit_kind IN（NULL 不参与 IN 比较）。
func resolveExitKinds(raw string) (kinds []string, includeOpen bool, err error) {
	for _, v := range splitCSV(raw) {
		if v == "open" || v == "still_open" {
			includeOpen = true
			continue
		}
		if _, ok := exitKindLabels[v]; !ok {
			return nil, false, fmt.Errorf("未知出场方式: %s", v)
		}
		kinds = append(kinds, v)
	}
	return kinds, includeOpen, nil
}

// buildEpisodeFilter 把查询 DTO 翻译成仓储过滤条件。
func buildEpisodeFilter(q argusDTO.EpisodeQueryDTO) (repository.EpisodeFilter, string, error) {
	var f repository.EpisodeFilter

	keys, err := resolveInstanceKeys(q.InstanceKey, q.InstanceKeys)
	if err != nil {
		return f, "", err
	}
	f.InstanceKeys = keys

	timeField, notice, err := resolveEpisodeTimeField(q.TimeField)
	if err != nil {
		return f, "", err
	}
	f.TimeField = timeField

	if f.Start, err = normalizeBound(q.Start, false); err != nil {
		return f, "", fmt.Errorf("start: %w", err)
	}
	if f.End, err = normalizeBound(q.End, true); err != nil {
		return f, "", fmt.Errorf("end: %w", err)
	}

	f.Instruments = normalizeInstruments(q.Instrument, q.Instruments)
	f.AccountLabels = append(splitCSV(q.AccountLabel), splitCSV(q.AccountLabels)...)
	f.Uids = append(splitCSV(q.Uid), splitCSV(q.Uids)...)
	f.Sides = splitCSV(q.Side)
	f.Variants = splitCSV(q.Variants)
	if f.ConfigVersions, err = parseUint64List(q.ConfigVersions); err != nil {
		return f, "", err
	}

	kinds, includeOpen, err := resolveExitKinds(q.ExitKinds)
	if err != nil {
		return f, "", err
	}
	f.ExitKinds = kinds

	switch strings.TrimSpace(q.Status) {
	case "", repository.EpisodeStatusAll:
		f.Status = repository.EpisodeStatusAll
	case repository.EpisodeStatusOpen:
		f.Status = repository.EpisodeStatusOpen
	case repository.EpisodeStatusClosed:
		f.Status = repository.EpisodeStatusClosed
	default:
		return f, "", fmt.Errorf("未知持仓状态: %s（可选 all/open/closed）", q.Status)
	}
	// exitKinds 里只勾了"仍持仓"时等价于 status=open；与具体出场方式混选时
	// 无法用一条 where 表达（NULL 不进 IN），此时放宽 status 并在服务层不再收窄，
	// 由前端的勾选组合自己承担——实测这两类不会同时勾。
	if includeOpen && len(kinds) == 0 {
		f.Status = repository.EpisodeStatusOpen
		f.ExitKinds = nil
	}

	f.StrategyOnly = q.StrategyOnly == 1
	return f, notice, nil
}

// ─── 列表 ───────────────────────────────────────────────────────────────────

// ListEpisodes 持仓 episode 列表。
func (s *ArgusEventService) ListEpisodes(q argusDTO.EpisodeQueryDTO) (*argusDTO.EpisodeListDTO, error) {
	filter, timeNotice, err := buildEpisodeFilter(q)
	if err != nil {
		return nil, err
	}
	window, err := s.resolveEpisodeWindow(filter)
	if err != nil {
		return nil, err
	}
	filter.Start, filter.End = window.Start, window.End

	pageIndex, pageSize := q.PageIndex, q.PageSize
	if pageIndex <= 0 {
		pageIndex = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxEpisodePageSize {
		pageSize = maxEpisodePageSize
	}

	total, err := s.episodeRepository.CountEpisodes(filter)
	if err != nil {
		return nil, err
	}
	rows, err := s.episodeRepository.ListEpisodes(filter, (pageIndex-1)*pageSize, pageSize,
		strings.TrimSpace(q.Order) == "ts_asc")
	if err != nil {
		return nil, err
	}
	data := make([]*argusDTO.EpisodeDTO, 0, len(rows))
	for _, row := range rows {
		item := episodeDTO(row)
		data = append(data, &item)
	}
	rebuild, err := s.rebuildState(filter.InstanceKeys)
	if err != nil {
		return nil, err
	}
	result := argusDTO.EpisodeListDTO{
		Total:           int(total),
		Data:            data,
		Window:          window,
		TimeField:       filter.TimeField,
		TimeFieldNotice: timeNotice,
		CrossInstance:   len(filter.InstanceKeys) != 1,
		Rebuild:         rebuild,
	}
	if result.CrossInstance {
		result.Notice = "当前是跨实例视图：每行都标了实例归属，账户名会跨实例重号（实例1 的 account1 与实例3 的 account1 是两个账户），张数与盈亏不要跨实例相加。"
	}
	return &result, nil
}

// resolveEpisodeWindow 定出 episode 查询的实际窗口。
//
// 与信号侧同因：主力数据是回灌的历史，按服务器当前时间兜底只会给张白页。
// 锚点取**事件表**的最新时刻而不是 episode 表的——派生滞后时，按 episode 表
// 兜底会把窗口锚在上一次重建那一刻，看上去像"最近没有持仓"。
func (s *ArgusEventService) resolveEpisodeWindow(f repository.EpisodeFilter) (argusDTO.WindowDTO, error) {
	if f.Start != "" && f.End != "" {
		return argusDTO.WindowDTO{Start: f.Start, End: f.End, Resolved: "explicit"}, nil
	}
	return s.resolveWindow(repository.EventFilter{
		InstanceKeys:  f.InstanceKeys,
		Instruments:   f.Instruments,
		AccountLabels: f.AccountLabels,
	}, f.Start, f.End, episodeDefaultSpan)
}

// episodeDTO 行 → 出参。
func episodeDTO(row *repository.EpisodeRow) argusDTO.EpisodeDTO {
	status := repository.EpisodeStatusClosed
	statusLabel := ExitLabel(row.ExitKind)
	if row.ClosedAt == "" {
		status = repository.EpisodeStatusOpen
		statusLabel = "持仓中"
	}
	return argusDTO.EpisodeDTO{
		EpisodeID:            row.Id,
		InstanceKey:          row.InstanceKey,
		Uid:                  row.Uid,
		AccountLabel:         row.AccountLabel,
		Instrument:           row.Instrument,
		Side:                 row.Side,
		FirstEventAt:         row.FirstEventAt,
		OpenedAt:             row.OpenedAt,
		LastEventAt:          row.LastEventAt,
		ClosedAt:             row.ClosedAt,
		DurationSec:          row.DurationSec,
		ExitKind:             row.ExitKind,
		ExitLabel:            ExitLabel(row.ExitKind),
		Status:               status,
		StatusLabel:          statusLabel,
		StrategyAttributable: row.StrategyAttributable == 1,
		TruncatedHead:        row.OpenedAt == "",
		AddCount:             row.AddCount,
		ReduceCount:          row.ReduceCount,
		EntrySizeTotal:       row.EntrySizeTotal,
		MaxSize:              row.MaxSize,
		OpenSize:             row.OpenSize,
		HiddenSize:           row.HiddenSize,
		MinRoiPctObserved:    row.MinRoiPctObserved,
		MaxRoiPctObserved:    row.MaxRoiPctObserved,
		PeakPct:              row.PeakPct,
		ExitRoiPct:           row.ExitRoiPct,
		GivebackPct:          givebackPct(row.PeakPct, row.ExitRoiPct),
		Pnl:                  row.Pnl,
		PnlStrategy:          row.PnlStrategy,
		UnattributedPnl:      row.UnattributedPnl,
		PnlKnown:             row.MissingPnlEvents == 0,
		RealizedEvents:       row.RealizedEvents,
		MissingPnlEvents:     row.MissingPnlEvents,
		CapSkipCount:         row.CapSkipCount,
		GateBlockCount:       row.GateBlockCount,
		TrendSkipCount:       row.TrendSkipCount,
		LossAlertCount:       row.LossAlertCount,
		BalanceSampleCount:   row.BalanceSampleCount,
		UplSampleCount:       row.UplSampleCount,
		PositionGapCount:     row.PositionGapCount,
		HasPositionGap:       row.HasPositionGap == 1,
		Variant:              row.Variant,
		ConfigVersion:        row.ConfigVersion,
		DepthFidelity:        row.DepthFidelity,
		DepthFidelityLabel:   DepthFidelityLabel(row.DepthFidelity),
		RebuiltAt:            row.RebuiltAt,
	}
}

// givebackPct 回吐比例 (peak − exit) / peak × 100。
//
// peak 缺失（trail 从未激活）或 ≤ 0（峰值本身在水下）时返回 nil：拿一个非正
// 的分母去算比例，得到的数没有"回吐了多少"的含义，补 0 更糟——那会显示成
// "一点没回吐"。
func givebackPct(peak, exit *float64) *float64 {
	if peak == nil || exit == nil || *peak <= 0 {
		return nil
	}
	v := round4((*peak - *exit) / *peak * 100)
	return &v
}

// ─── 详情 ───────────────────────────────────────────────────────────────────

// GetEpisodeDetail 一条持仓的完整生命周期。
//
// 一次返回五段：episode 主体、出场归因、建仓决策行、事件时间轴、
// 两条互不换算的轨迹（ROI% 离散观测 + upl 连续心跳）。
func (s *ArgusEventService) GetEpisodeDetail(episodeID uint64) (*argusDTO.EpisodeDetailDTO, error) {
	row, err := s.episodeRepository.FindEpisodeByID(episodeID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrEpisodeNotFound
	}
	detail := argusDTO.EpisodeDetailDTO{Episode: episodeDTO(row)}
	detail.Exit = buildExitAttribution(row)

	entryRows, err := s.episodeEntryRepository.ListByEpisode(row.Id)
	if err != nil {
		return nil, err
	}
	decisionAt := make(map[string]struct{}, len(entryRows))
	for _, entry := range entryRows {
		detail.Entries = append(detail.Entries, episodeEntryDTO(entry))
		decisionAt[entry.DecidedAt] = struct{}{}
	}

	// 事件时间轴与轨迹都取 [first_event_at, 观测末刻] 这个闭区间：
	// 仍持仓时末刻是 last_event_at，已出场时是 closed_at。
	start, end := row.FirstEventAt, row.LastEventAt
	if row.ClosedAt != "" && row.ClosedAt > end {
		end = row.ClosedAt
	}
	eventRows, err := s.strategyEventRepository.ListEvents(repository.EventFilter{
		InstanceKeys:  []string{row.InstanceKey},
		AccountLabels: []string{row.AccountLabel},
		Instruments:   []string{row.Instrument},
		Events:        AllEvents(),
		Start:         start,
		End:           end,
	}, 0, maxEpisodeTimelineRows+1, true)
	if err != nil {
		return nil, err
	}
	if len(eventRows) > maxEpisodeTimelineRows {
		eventRows = eventRows[:maxEpisodeTimelineRows]
		detail.EventTruncated = true
	}
	detail.Timeline, detail.RoiTrack = buildEpisodeTimeline(eventRows, decisionAt)
	// 平仓事件带的 trail 峰值没有自己的时刻（它是持仓期内的极值），挂在出场
	// 时刻上只是为了同屏可见，因此单独标 kind=peak，前端不要把它连进观测线。
	if row.PeakPct != nil && row.ClosedAt != "" {
		detail.RoiTrack = append(detail.RoiTrack, argusDTO.EpisodeRoiPointDTO{
			Ts: row.ClosedAt, RoiPct: *row.PeakPct, Event: "trail_peak",
			Label: "trail 峰值（时刻未知）", Kind: "peak",
		})
	}
	detail.RoiTrackNotice = "ROI% 只在事件里被离散写下（门控拦截 / 浮亏告警 / 出场），中间没有观测；浮亏告警还是「ROI < −150% 才发 + 5 分钟冷却」的双重采样，最深值只是真实值的下界。"

	uplRows, err := s.balanceRepository.ListPoints(repository.BalanceFilter{
		InstanceKeys:  []string{row.InstanceKey},
		AccountLabels: []string{row.AccountLabel},
		Start:         start,
		End:           end,
	}, maxEpisodeUplRows+1)
	if err != nil {
		return nil, err
	}
	if len(uplRows) > maxEpisodeUplRows {
		uplRows = uplRows[:maxEpisodeUplRows]
		detail.UplTruncated = true
	}
	for _, point := range uplRows {
		detail.UplTrack = append(detail.UplTrack, argusDTO.EpisodeUplPointDTO{
			Ts: point.Ts, Upl: point.Upl, Equity: point.Equity, NetSize: point.NetSize,
		})
		if point.Upl != nil {
			detail.UplTrackAvailable = true
		}
	}
	if detail.UplTrackAvailable {
		detail.UplTrackNotice = "浮盈单位是 U（账户级未实现盈亏），不是 ROI%。ROI = 浮盈 / 保证金，而保证金没有落库，硬换算只能靠假定持仓期间保证金不变——那是拟合不是观测。"
	} else {
		detail.UplTrackNotice = "本段持仓的心跳不带 upl（2026-07-21 之前的历史数据），浮盈轨迹不存在；只能看下方的 ROI 离散观测点。"
	}
	return &detail, nil
}

// buildExitAttribution 出场归因。
func buildExitAttribution(row *repository.EpisodeRow) argusDTO.EpisodeExitAttributionDTO {
	out := argusDTO.EpisodeExitAttributionDTO{
		ExitKind:         row.ExitKind,
		ExitLabel:        ExitLabel(row.ExitKind),
		Attributable:     row.StrategyAttributable == 1,
		CountedInWinRate: row.StrategyAttributable == 1,
		Reason:           exitAttributionReason(row.ExitKind),
		ExitRoiPct:       row.ExitRoiPct,
		PeakPct:          row.PeakPct,
		GivebackPct:      givebackPct(row.PeakPct, row.ExitRoiPct),
	}
	switch {
	case row.PeakPct == nil:
		out.GivebackNote = "该笔持仓没有记录 trail 峰值（移动止盈从未激活），回吐比例无法计算。"
	case *row.PeakPct <= 0:
		out.GivebackNote = "峰值本身不为正，回吐比例没有意义，不做计算。"
	case row.ExitKind == episode.ExitReduceToZero:
		out.GivebackNote = "出场 ROI 是**减到 0 的那一笔**的收益率，不是整段持仓的收益率；回吐比例据此计算，读数时要带上这个前提。"
	}
	return out
}

// episodeEntryDTO 建仓决策行 → 出参。
func episodeEntryDTO(row *repository.EpisodeEntryRow) argusDTO.EpisodeEntryDTO {
	return argusDTO.EpisodeEntryDTO{
		EntryID:               row.Id,
		DecidedAt:             row.DecidedAt,
		Side:                  row.Side,
		AddedSize:             row.AddedSize,
		OrderSize:             row.OrderSize,
		ClosedSize:            row.ClosedSize,
		OpenSize:              row.OpenSize,
		AttributedPnl:         row.AttributedPnl,
		AttributedPnlStrategy: row.AttributedPnlStrategy,
		MissingPnlEvents:      row.MissingPnlEvents,
		PnlKnown:              row.PnlKnown == 1,
		Variant:               row.Variant,
		ConfigVersion:         row.ConfigVersion,
		GapBp:                 row.GapBp,
		StrengthLevel:         StrengthLevelOf(row.GapBp),
		AvgPx:                 row.AvgPx,
		LastPx:                row.LastPx,
	}
}

// buildEpisodeTimeline 把事件流铺成时间轴，并顺手抽出 ROI% 观测点。
//
// decisionAt 是 episode_entry 的决策时刻集合：命中的 open 事件标 isDecision，
// 让"这一次加仓真的记进了归因分母"在页面上可见——仓位跳变与快照滞后的
// open 事件不会产生决策行，那正是需要被看到的差异。
func buildEpisodeTimeline(rows []*repository.StrategyEventRow, decisionAt map[string]struct{}) ([]argusDTO.EpisodeTimelineItemDTO, []argusDTO.EpisodeRoiPointDTO) {
	timeline := make([]argusDTO.EpisodeTimelineItemDTO, 0, len(rows))
	roi := make([]argusDTO.EpisodeRoiPointDTO, 0, len(rows))
	for _, row := range rows {
		item := argusDTO.EpisodeTimelineItemDTO{
			Ts:        row.Ts,
			EventID:   row.Id,
			Event:     row.Event,
			Label:     EventLabel(row.Event),
			Tone:      eventTone(row.Event),
			Side:      strVal(row.Side),
			NetSize:   row.Size,
			OrderSize: row.OrderSize,
			AvgPx:     row.AvgPx,
			LastPx:    row.LastPx,
			RoiPct:    row.RoiPct,
			Pnl:       row.Pnl,
			PeakPct:   row.PeakPct,
			GapBp:     row.GapBp,
			GateKind:  strVal(row.GateKind),
			GateLabel: GateLabel(strVal(row.GateKind)),
			Reason:    strVal(row.Reason),
		}
		if row.Event == eventlog.EvOpen {
			_, item.IsDecision = decisionAt[row.Ts]
		}
		timeline = append(timeline, item)
		if row.RoiPct != nil {
			roi = append(roi, argusDTO.EpisodeRoiPointDTO{
				Ts: row.Ts, RoiPct: *row.RoiPct, Event: row.Event,
				Label: EventLabel(row.Event), Kind: "observed",
			})
		}
	}
	return timeline, roi
}

// ─── 出场归因分布 ────────────────────────────────────────────────────────────

// GetEpisodeStats 出场方式分布与胜率口径。
func (s *ArgusEventService) GetEpisodeStats(q argusDTO.EpisodeStatsQueryDTO) (*argusDTO.EpisodeStatsDTO, error) {
	filter, _, err := buildEpisodeFilter(argusDTO.EpisodeQueryDTO{
		InstanceKey:  q.InstanceKey,
		InstanceKeys: q.InstanceKeys,
		Instrument:   q.Instrument,
		AccountLabel: q.AccountLabel,
		Start:        q.Start,
		End:          q.End,
		TimeField:    q.TimeField,
	})
	if err != nil {
		return nil, err
	}
	window, err := s.resolveEpisodeWindow(filter)
	if err != nil {
		return nil, err
	}
	filter.Start, filter.End = window.Start, window.End

	rows, err := s.episodeRepository.AggregateEpisodes(filter, []string{"exit_kind"})
	if err != nil {
		return nil, err
	}
	result := argusDTO.EpisodeStatsDTO{
		Window:        window,
		TimeField:     filter.TimeField,
		InstanceKeys:  filter.InstanceKeys,
		CrossInstance: len(filter.InstanceKeys) != 1,
		ByExitKind:    []argusDTO.ExitKindBucketDTO{},
	}
	aggregateExitKinds(rows, &result)
	if result.CrossInstance {
		result.Notice = "跨实例视图：三个实例的阈值、仓位上限与下单张数都不同，胜率与盈亏跨实例相加没有意义，这里只做分布展示。"
	}
	if result.Excluded > 0 {
		result.Notice = strings.TrimSpace(result.Notice + " 交易所侧平仓 / 人工平仓共 " +
			fmt.Sprint(result.Excluded) + " 笔不计入策略胜率，其盈亏仍计入净盈亏。")
	}
	if result.Rebuild, err = s.rebuildState(filter.InstanceKeys); err != nil {
		return nil, err
	}
	return &result, nil
}

// aggregateExitKinds 把按 exit_kind 分组的聚合行铺成分布 + 总计。
//
// 胜率的分子分母都只取 strategy_attributable = 1 的行：不计入胜率的那两类
// 在返回体里仍然有 count 与 pnl，只是 winRate 为 nil——过滤掉它们会让"净盈亏"
// 对不上账，把它们算进胜率则会把非策略行为记成策略成绩。
func aggregateExitKinds(rows []*repository.EpisodeAggRow, out *argusDTO.EpisodeStatsDTO) {
	byKind := make(map[string]*repository.EpisodeAggRow, len(rows))
	for _, row := range rows {
		byKind[row.ExitKind] = row
		out.Total += row.Episodes
		out.Closed += row.Closed
		out.StillOpen += row.StillOpen
		out.Attributable += row.Strategy
		out.Wins += row.StrategyWins
		out.Pnl += f64Val(row.Pnl)
		out.PnlStrategy += f64Val(row.PnlStrategy)
		out.UnattributedPnl += f64Val(row.Unattributed)
		out.IncompletePnl += row.IncompletePnl
		out.TruncatedHead += row.TruncatedHead
	}
	out.Excluded = out.Total - out.Attributable
	if out.Attributable > 0 {
		rate := round4(float64(out.Wins) / float64(out.Attributable) * 100)
		out.WinRate = &rate
	}
	out.Pnl = round4(out.Pnl)
	out.PnlStrategy = round4(out.PnlStrategy)
	out.UnattributedPnl = round4(out.UnattributedPnl)

	// 固定按 ExitKinds() 的顺序输出，末尾补"仍持仓"；条数为 0 的档位也保留，
	// 分布图上的空档本身就是信息（"这段时间一次兜底止损都没有"）。
	kinds := append(ExitKinds(), ExitKindStillOpen)
	for _, kind := range kinds {
		bucket := argusDTO.ExitKindBucketDTO{
			ExitKind:  kind,
			Label:     ExitLabel(kind),
			CountedIn: CountedInWinRate(kind),
		}
		if row, ok := byKind[kind]; ok {
			bucket.Count = row.Episodes
			bucket.Pnl = round4(f64Val(row.Pnl))
			bucket.Wins = row.StrategyWins
			if bucket.CountedIn && row.Strategy > 0 {
				rate := round4(float64(row.StrategyWins) / float64(row.Strategy) * 100)
				bucket.WinRate = &rate
			}
		}
		bucket.Share = shareOf(bucket.Count, out.Total)
		out.ByExitKind = append(out.ByExitKind, bucket)
	}
}

func f64Val(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ─── 派生新鲜度 ──────────────────────────────────────────────────────────────

// rebuildState 派生表的新鲜度探针。
//
// episode 目前只能靠 CLI 手工整实例重建（r10 的遗留项），事件却在持续写入。
// 页面必须能说出"这批 episode 派生到哪一刻"，否则滞后会被读成"最近没有持仓"。
func (s *ArgusEventService) rebuildState(instanceKeys []string) (argusDTO.EpisodeRebuildDTO, error) {
	out := argusDTO.EpisodeRebuildDTO{}
	state, err := s.episodeRepository.RebuildState(instanceKeys)
	if err != nil {
		return out, err
	}
	out.EpisodeCount = state.EpisodeCount
	out.LastRebuiltAt = state.LastRebuiltAt
	out.DerivedThroughTs = state.LastEventAt

	rng, err := s.strategyEventRepository.EventTimeRange(repository.EventFilter{InstanceKeys: instanceKeys})
	if err != nil {
		return out, err
	}
	out.LatestEventTs = rng.LastTs

	if out.EpisodeCount == 0 {
		out.Stale = true
		out.Notice = "还没有派生过 episode。请先跑 argus-episode-rebuild（一次一个实例，--apply 才写库），否则持仓视角是空的。"
		return out, nil
	}
	if out.DerivedThroughTs != "" && out.LatestEventTs != "" {
		derived, derr := parseEventTime(out.DerivedThroughTs, false)
		latest, lerr := parseEventTime(out.LatestEventTs, false)
		if derr == nil && lerr == nil {
			lag := int64(latest.Sub(derived) / time.Second)
			if lag < 0 {
				lag = 0
			}
			out.LagSeconds = &lag
			// 一小时是拍的：重建是手工触发的批处理，分钟级的差是正常的写入
			// 时序，小时级的差才说明"该重建了"。
			if lag > 3600 {
				out.Stale = true
				out.Notice = fmt.Sprintf("派生结果只覆盖到 %s，事件表已经写到 %s（滞后 %s）。持仓视角看不到最近的持仓，跑一次 argus-episode-rebuild 再看。",
					out.DerivedThroughTs, out.LatestEventTs, humanizeLag(lag))
			}
		}
	}
	return out, nil
}

// humanizeLag 把秒数写成人能一眼判断严重程度的形式。
func humanizeLag(seconds int64) string {
	switch {
	case seconds >= 86400:
		return fmt.Sprintf("%.1f 天", float64(seconds)/86400)
	case seconds >= 3600:
		return fmt.Sprintf("%.1f 小时", float64(seconds)/3600)
	default:
		return fmt.Sprintf("%d 分钟", seconds/60)
	}
}

// ListExitKindOptions 出场方式筛选项，带派生表里的真实条数与胜率口径标注。
func (s *ArgusEventService) ListExitKindOptions(instanceKeys []string) ([]argusDTO.ExitKindOptionDTO, error) {
	rows, err := s.episodeRepository.ListExitKindOptions(instanceKeys)
	if err != nil {
		return nil, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.ExitKind] = row.Count
	}
	kinds := append(ExitKinds(), ExitKindStillOpen)
	out := make([]argusDTO.ExitKindOptionDTO, 0, len(kinds))
	for _, kind := range kinds {
		value := kind
		if kind == ExitKindStillOpen {
			// 入参里用 'open' 表达仍持仓：exit_kind IS NULL 进不了 IN 比较，
			// 空串在 query string 里也无法与"没传"区分。
			value = "open"
		}
		out = append(out, argusDTO.ExitKindOptionDTO{
			Value: value, Label: ExitLabel(kind), Count: counts[kind],
			CountedInWinRate: CountedInWinRate(kind),
		})
	}
	return out, nil
}

// sortedKeys 稳定输出 map 的键，避免返回体顺序抖动。
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// avgPtr 求平均；n = 0 时返回 nil 而不是 0——"没有样本"和"平均是 0"是两回事。
func avgPtr(sum float64, n int64) *float64 {
	if n <= 0 {
		return nil
	}
	v := round4(sum / float64(n))
	return &v
}
