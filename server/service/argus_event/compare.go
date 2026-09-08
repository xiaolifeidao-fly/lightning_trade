package argus_event

// 本文件实现「按 config_version 与实例切片对比」：把窗口内的信号与持仓
// 按 (实例, 参数版本) 切成行，横向摆在一起看"换了参数之后有没有变化"。
//
// 两条口径，改代码前先读：
//
//  1. **信号侧按事件自带的 config_version 归属，持仓侧按 episode 的
//     config_version 归属，后者是开仓时刻的版本**（r10 设计文档 §6.3）。
//     一条跨越发布点的持仓算在**做建仓决策**的那个版本上，不是平仓那个——
//     按平仓时刻归集会造出需求大纲 §3.5 记录的那类伪影。
//
//  2. **跨实例的行不能相加**。三个实例的阈值 5/3bp、上限 15/26+8/246、
//     下单 1/10 全不同，这里逐行独立，服务端不给合计行。

import (
	"strings"

	"argus_single/pkg/eventlog"
	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
)

// sliceKey 一个对比切片的身份。
type sliceKey struct {
	InstanceKey   string
	ConfigVersion uint64
}

// sliceAccumulator 信号侧的逐切片累加器。
type sliceAccumulator struct {
	signals   int64
	opened    int64
	capSkip   int64
	gateBlock int64
	trendSkip int64
	weak      int64
	medium    int64
	strong    int64
	gapSum    float64
	gapCount  int64
	firstTs   string
	lastTs    string
	variants  map[string]struct{}
}

// GetSliceCompare 按 (实例, 参数版本) 切片的横向对比。
func (s *ArgusEventService) GetSliceCompare(q argusDTO.SliceCompareQueryDTO) (*argusDTO.SliceCompareDTO, error) {
	keys, err := resolveInstanceKeys(q.InstanceKey, q.InstanceKeys)
	if err != nil {
		return nil, err
	}
	timeField, _, err := resolveEpisodeTimeField(q.TimeField)
	if err != nil {
		return nil, err
	}
	eventFilter := repository.EventFilter{
		InstanceKeys:  keys,
		Instruments:   normalizeInstruments(q.Instrument, ""),
		AccountLabels: splitCSV(q.AccountLabel),
		Events:        TriggerEvents(),
	}
	if eventFilter.Start, err = normalizeBound(q.Start, false); err != nil {
		return nil, err
	}
	if eventFilter.End, err = normalizeBound(q.End, true); err != nil {
		return nil, err
	}
	window, err := s.resolveWindow(eventFilter, eventFilter.Start, eventFilter.End, episodeDefaultSpan)
	if err != nil {
		return nil, err
	}
	eventFilter.Start, eventFilter.End = window.Start, window.End

	result := argusDTO.SliceCompareDTO{
		Window:        window,
		TimeField:     timeField,
		Rows:          []argusDTO.SliceCompareRowDTO{},
		CrossInstance: len(keys) != 1,
	}
	if result.CrossInstance {
		result.Notice = "每行是一个 (实例, 参数版本) 切片，逐行独立看。三个实例的阈值、仓位上限与下单张数都不同，开仓率、胜率、盈亏跨实例相加或直接比大小都没有意义。"
	} else {
		result.Notice = "每行是该实例的一个参数版本切片。版本之间的差异要结合窗口长度与信号条数看——单路径混沌 ±70U，条数少的切片点估计不可信。"
	}

	rows, err := s.strategyEventRepository.ListEvents(eventFilter, 0, maxAggregateRows+1, true)
	if err != nil {
		return nil, err
	}
	if len(rows) > maxAggregateRows {
		rows = rows[:maxAggregateRows]
		result.SignalTruncated = true
	}
	acc := accumulateSignalSlices(rows)

	episodeFilter := repository.EpisodeFilter{
		InstanceKeys:  keys,
		Instruments:   eventFilter.Instruments,
		AccountLabels: eventFilter.AccountLabels,
		TimeField:     timeField,
		Start:         window.Start,
		End:           window.End,
		Status:        repository.EpisodeStatusAll,
	}
	episodeRows, err := s.episodeRepository.AggregateEpisodes(episodeFilter, []string{"instance_key", "config_version"})
	if err != nil {
		return nil, err
	}
	episodeByKey := make(map[sliceKey]*repository.EpisodeAggRow, len(episodeRows))
	for _, row := range episodeRows {
		episodeByKey[sliceKey{row.InstanceKey, row.ConfigVersion}] = row
	}

	// 实例展示名：事件里出现过但注册表没有的实例照样列出来并标 registered=false，
	// 那正是"实例键撞名 / 漏注册"要被看见的场景。
	registry := map[string]string{}
	if instances, err := s.configService.ListInstances(false); err == nil {
		for _, item := range instances {
			registry[item.InstanceKey] = item.InstanceName
		}
	}

	// 切片集合取信号侧与持仓侧的并集：某个版本可能只有拦截没有持仓
	// （比如趋势闸把整段都挡住了），漏掉它等于把"这版一笔都没开出来"藏起来。
	seen := map[sliceKey]struct{}{}
	for key := range acc {
		seen[key] = struct{}{}
	}
	for key := range episodeByKey {
		seen[key] = struct{}{}
	}
	for _, key := range sortSliceKeys(seen) {
		row := argusDTO.SliceCompareRowDTO{
			InstanceKey:   key.InstanceKey,
			ConfigVersion: key.ConfigVersion,
			Variants:      []string{},
		}
		if name, ok := registry[key.InstanceKey]; ok {
			row.InstanceName, row.Registered = name, true
		} else {
			row.InstanceName = key.InstanceKey
		}
		if a, ok := acc[key]; ok {
			row.Signals = a.signals
			row.Opened = a.opened
			row.CapSkip = a.capSkip
			row.GateBlock = a.gateBlock
			row.TrendSkip = a.trendSkip
			row.OpenRate = shareOf(a.opened, a.signals)
			row.Weak, row.Medium, row.Strong = a.weak, a.medium, a.strong
			row.AvgGapBp = avgPtr(a.gapSum, a.gapCount)
			row.FirstTs, row.LastTs = a.firstTs, a.lastTs
			row.Variants = sortedKeys(a.variants)
		}
		if e, ok := episodeByKey[key]; ok {
			row.EpisodeAvailable = true
			row.Episodes = e.Episodes
			row.ClosedCount = e.Closed
			row.Attributable = e.Strategy
			row.Wins = e.StrategyWins
			if e.Strategy > 0 {
				rate := round4(float64(e.StrategyWins) / float64(e.Strategy) * 100)
				row.WinRate = &rate
			}
			row.Pnl = round4(f64Val(e.Pnl))
			row.PnlStrategy = round4(f64Val(e.PnlStrategy))
			row.AvgPeakPct = e.AvgPeakPct
			row.AvgExitRoiPct = e.AvgExitRoiPct
		}
		result.Rows = append(result.Rows, row)
	}
	if result.Rebuild, err = s.rebuildState(keys); err != nil {
		return nil, err
	}
	return &result, nil
}

// accumulateSignalSlices 把触发事件按 (实例, 参数版本) 累加。
//
// 聚合放在 Go 侧而不是下推成 GROUP BY：强度分档的边界只在 catalog.go 定义
// 一次，写进 SQL 的 CASE WHEN 会和它两处漂移，而这正是最容易被质疑的口径。
func accumulateSignalSlices(rows []*repository.StrategyEventRow) map[sliceKey]*sliceAccumulator {
	acc := make(map[sliceKey]*sliceAccumulator)
	for _, row := range rows {
		key := sliceKey{row.InstanceKey, row.ConfigVersion}
		a, ok := acc[key]
		if !ok {
			a = &sliceAccumulator{variants: map[string]struct{}{}, firstTs: row.Ts}
			acc[key] = a
		}
		a.signals++
		if row.Ts < a.firstTs || a.firstTs == "" {
			a.firstTs = row.Ts
		}
		if row.Ts > a.lastTs {
			a.lastTs = row.Ts
		}
		if v := strVal(row.Variant); v != "" {
			a.variants[v] = struct{}{}
		}
		switch row.Event {
		case eventlog.EvOpen:
			a.opened++
		case eventlog.EvCapSkip:
			a.capSkip++
		case eventlog.EvGateBlock:
			a.gateBlock++
		case eventlog.EvTrendSkip:
			a.trendSkip++
		}
		// gap_bp 为 NULL（报价不可算）不判档也不进均值，不当成 0 混进弱档。
		if row.GapBp != nil {
			abs := *row.GapBp
			if abs < 0 {
				abs = -abs
			}
			a.gapSum += abs
			a.gapCount++
			switch StrengthLevelOf(row.GapBp) {
			case StrengthWeak:
				a.weak++
			case StrengthMedium:
				a.medium++
			case StrengthStrong:
				a.strong++
			}
		}
	}
	return acc
}

// sortSliceKeys 稳定定序：实例键升序、版本号降序（新版本在前）。
func sortSliceKeys(set map[sliceKey]struct{}) []sliceKey {
	out := make([]sliceKey, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sortSlice(out, func(a, b sliceKey) bool {
		if a.InstanceKey != b.InstanceKey {
			return a.InstanceKey < b.InstanceKey
		}
		return a.ConfigVersion > b.ConfigVersion
	})
	return out
}

// sortSlice 是 sort.Slice 的具名包装，只为让上面的比较函数带类型。
func sortSlice[T any](list []T, less func(a, b T) bool) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

var _ = strings.TrimSpace
