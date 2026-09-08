package argus_event

import (
	"fmt"
	"math"
	"sort"
	"time"

	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
)

// 本文件实现三个聚合读接口：拦截原因聚合、跨实例汇总对比、筛选项枚举。
//
// 聚合一律在 Go 侧做，不下推成 SQL 的 GROUP BY：
//   - 口径（结果大类、强度分档、净仓推导）在 catalog.go 里只定义一次，
//     下推就得在 SQL 里再写一遍 CASE WHEN，两处必然漂移；
//   - 这些口径是本任务最容易被质疑的地方，纯函数能被单测钉死；
//   - 量级撑得住：全库实测 16.6 万条事件，其中业务事件仅 9%，
//     正常窗口下扫描量在万级。超过 maxAggregateRows 时挂 truncated 标记，
//     不静默截断。

const crossInstanceNotice = "三个实例的参数不同（信号阈值 5/3 bp、仓位上限 15 / 26+8 / 246、单次下单 1/10 张），" +
	"胜率、盈亏这类指标跨实例相加没有意义；要对比请逐实例看，要归因请先选定实例。"

// GetGateStats 拦截原因聚合 + 结果分布 + 信号强度分级。
//
// instanceKey 可以为空（全部实例）：触发次数这类计数跨实例相加是有意义的，
// 但比率不可比，因此返回体会挂 crossInstance 标记与提示。
func (s *ArgusEventService) GetGateStats(q argusDTO.GateStatsQueryDTO) (*argusDTO.GateStatsDTO, error) {
	keys, err := resolveInstanceKeys(q.InstanceKey, q.InstanceKeys)
	if err != nil {
		return nil, err
	}
	filter := repository.EventFilter{
		InstanceKeys: keys,
		Events:       TriggerEvents(),
	}
	if inst := normalizeInstrument(q.Instrument); inst != "" {
		filter.Instruments = []string{inst}
	}
	if labels := splitCSV(q.AccountLabel); len(labels) > 0 {
		filter.AccountLabels = labels
	}
	start, err := normalizeBound(q.Start, false)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	end, err := normalizeBound(q.End, true)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	window, err := s.resolveWindow(filter, start, end, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	filter.Start, filter.End = window.Start, window.End

	result := argusDTO.GateStatsDTO{
		Window:        window,
		InstanceKeys:  keys,
		CrossInstance: len(keys) != 1,
		ByResult:      []argusDTO.ResultBucketDTO{},
		ByGate:        []argusDTO.GateBucketDTO{},
		ByStrength:    []argusDTO.StrengthBucketDTO{},
	}
	if result.CrossInstance {
		result.Notice = crossInstanceNotice
	}
	if window.Start == "" || window.End == "" {
		return &result, nil
	}
	rows, err := s.strategyEventRepository.ListEvents(filter, 0, maxAggregateRows, true)
	if err != nil {
		return nil, err
	}
	result.Truncated = len(rows) >= maxAggregateRows
	aggregateGateStats(rows, &result)
	return &result, nil
}

// aggregateGateStats 纯聚合，无 IO。
func aggregateGateStats(rows []*repository.StrategyEventRow, out *argusDTO.GateStatsDTO) {
	type gateAcc struct {
		count          int64
		thresholdSum   float64
		thresholdCount int64
		actualSum      float64
		actualCount    int64
		lastTs         string
		sampleReason   string
	}
	byEvent := map[string]int64{}
	byGate := map[string]*gateAcc{}
	type strengthAcc struct{ count, opened int64 }
	byStrength := map[string]*strengthAcc{}

	for _, row := range rows {
		out.TotalTriggers++
		byEvent[row.Event]++
		switch ResultKindOf(row.Event) {
		case ResultKindOpen:
			out.Opened++
		case ResultKindBlocked:
			out.Blocked++
		}
		// gate_kind 为空的拦截事件也要计数：reason 文案漂移时 ParseGate 会给
		// 兜底 kind，真为空说明是老数据，归到 unknown 桶而不是丢掉。
		if ResultKindOf(row.Event) == ResultKindBlocked {
			kind := strVal(row.GateKind)
			if kind == "" {
				kind = "unknown"
			}
			acc := byGate[kind]
			if acc == nil {
				acc = &gateAcc{}
				byGate[kind] = acc
			}
			acc.count++
			if row.GateThreshold != nil {
				acc.thresholdSum += *row.GateThreshold
				acc.thresholdCount++
			}
			if row.GateActual != nil {
				acc.actualSum += *row.GateActual
				acc.actualCount++
			}
			if row.Ts > acc.lastTs {
				acc.lastTs = row.Ts
			}
			if acc.sampleReason == "" {
				acc.sampleReason = strVal(row.Reason)
			}
		}
		if level := StrengthLevelOf(row.GapBp); level != "" {
			acc := byStrength[level]
			if acc == nil {
				acc = &strengthAcc{}
				byStrength[level] = acc
			}
			acc.count++
			if ResultKindOf(row.Event) == ResultKindOpen {
				acc.opened++
			}
		}
	}
	if out.TotalTriggers > 0 {
		out.OpenRate = round4(float64(out.Opened) / float64(out.TotalTriggers))
	}

	for _, event := range TriggerEvents() {
		count := byEvent[event]
		if count == 0 {
			continue
		}
		out.ByResult = append(out.ByResult, argusDTO.ResultBucketDTO{
			Event: event, Label: EventLabel(event), Count: count,
			Share: shareOf(count, out.TotalTriggers),
		})
	}
	kinds := append(GateKinds(), "unknown")
	for _, kind := range kinds {
		acc := byGate[kind]
		if acc == nil {
			continue
		}
		bucket := argusDTO.GateBucketDTO{
			GateKind: kind, Label: GateLabel(kind), Count: acc.count,
			Share: shareOf(acc.count, out.Blocked), LastTs: acc.lastTs, SampleReason: acc.sampleReason,
		}
		if kind == "unknown" {
			bucket.Label = "未结构化（老数据）"
		}
		if acc.thresholdCount > 0 {
			v := round4(acc.thresholdSum / float64(acc.thresholdCount))
			bucket.AvgThreshold = &v
		}
		if acc.actualCount > 0 {
			v := round4(acc.actualSum / float64(acc.actualCount))
			bucket.AvgActual = &v
		}
		out.ByGate = append(out.ByGate, bucket)
	}
	sort.SliceStable(out.ByGate, func(i, j int) bool { return out.ByGate[i].Count > out.ByGate[j].Count })

	for _, band := range StrengthOptions() {
		acc := byStrength[band.Value]
		bucket := argusDTO.StrengthBucketDTO{
			Level: band.Value, Label: band.Label, MinAbsBp: band.MinAbsBp, MaxAbsBp: band.MaxAbsBp,
		}
		if acc != nil {
			bucket.Count = acc.count
			bucket.Opened = acc.opened
			if acc.count > 0 {
				bucket.OpenRate = round4(float64(acc.opened) / float64(acc.count))
			}
		}
		out.ByStrength = append(out.ByStrength, bucket)
	}
}

func shareOf(count, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return round4(float64(count) / float64(total))
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

// ─── 跨实例汇总对比 ──────────────────────────────────────────────────────────

// GetInstanceSummary 跨实例汇总对比。
//
// 返回体里每个实例都是独立一行，服务端**不做任何跨实例求和**——那正是需求
// 大纲反复强调不能干的事。前端要显示"合计"只能在明确标注不可比的前提下自己加。
func (s *ArgusEventService) GetInstanceSummary(q argusDTO.InstanceSummaryQueryDTO) (*argusDTO.InstanceSummaryResultDTO, error) {
	filter := repository.EventFilter{Events: AllEvents()}
	if inst := normalizeInstrument(q.Instrument); inst != "" {
		filter.Instruments = []string{inst}
	}
	start, err := normalizeBound(q.Start, false)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	end, err := normalizeBound(q.End, true)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	window, err := s.resolveWindow(filter, start, end, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	filter.Start, filter.End = window.Start, window.End

	result := argusDTO.InstanceSummaryResultDTO{
		Window:    window,
		Instances: []argusDTO.InstanceSummaryDTO{},
		Notice:    crossInstanceNotice,
	}
	if window.Start == "" || window.End == "" {
		return &result, nil
	}
	rows, err := s.strategyEventRepository.ListEvents(filter, 0, maxAggregateRows, true)
	if err != nil {
		return nil, err
	}
	result.Truncated = len(rows) >= maxAggregateRows

	latest, err := s.balanceRepository.ListLatestByAccount(repository.BalanceFilter{
		Start: window.Start, End: window.End,
	})
	if err != nil {
		return nil, err
	}
	// 注册表只用来补展示名与启用状态；事件里出现过但没注册的实例照样列出来，
	// 并标 registered=false——那正是"实例键撞名/漏注册"要被看见的场景。
	registry := map[string]argusDTO.InstanceSummaryDTO{}
	if instances, err := s.configService.ListInstances(false); err == nil {
		for _, item := range instances {
			registry[item.InstanceKey] = argusDTO.InstanceSummaryDTO{
				InstanceKey: item.InstanceKey, InstanceName: item.InstanceName,
				Enabled: item.Enabled, Registered: true,
			}
		}
	}
	result.Instances = aggregateInstanceSummary(rows, latest, registry)
	return &result, nil
}

// aggregateInstanceSummary 纯聚合，无 IO。
func aggregateInstanceSummary(rows []*repository.StrategyEventRow, latest []*repository.BalanceLatestRow,
	registry map[string]argusDTO.InstanceSummaryDTO) []argusDTO.InstanceSummaryDTO {

	type instAcc struct {
		summary  argusDTO.InstanceSummaryDTO
		variants map[string]struct{}
		pnl      *float64
	}
	byInstance := map[string]*instAcc{}
	ensure := func(key string) *instAcc {
		acc := byInstance[key]
		if acc == nil {
			acc = &instAcc{variants: map[string]struct{}{}}
			if base, ok := registry[key]; ok {
				acc.summary = base
			} else {
				acc.summary = argusDTO.InstanceSummaryDTO{InstanceKey: key, InstanceName: key}
			}
			byInstance[key] = acc
		}
		return acc
	}

	for _, row := range rows {
		acc := ensure(row.InstanceKey)
		s := &acc.summary
		switch row.Event {
		case eventlog.EvOpen:
			s.Signals++
			s.Opened++
		case eventlog.EvCapSkip:
			s.Signals++
			s.CapSkip++
		case eventlog.EvGateBlock:
			s.Signals++
			s.GateBlock++
		case eventlog.EvTrendSkip:
			s.Signals++
			s.TrendSkip++
		default:
			if ResultKindOf(row.Event) == ResultKindExit {
				s.Exits++
			}
		}
		if v := strVal(row.Variant); v != "" {
			acc.variants[v] = struct{}{}
		}
		if row.ConfigVersion > s.ConfigVersion {
			s.ConfigVersion = row.ConfigVersion
		}
		if s.FirstTs == "" || row.Ts < s.FirstTs {
			s.FirstTs = row.Ts
		}
		if row.Ts > s.LastTs {
			s.LastTs = row.Ts
		}
		if row.Pnl != nil {
			sum := *row.Pnl
			if acc.pnl != nil {
				sum += *acc.pnl
			}
			acc.pnl = &sum
		}
	}

	for _, row := range latest {
		acc := ensure(row.InstanceKey)
		account := argusDTO.InstanceAccountDTO{
			AccountLabel: row.AccountLabel, Uid: row.Uid, Variant: row.Variant,
			LastEquity: row.Equity, LastBalance: row.Balance, LastSampleTs: row.Ts,
		}
		if row.NetSizeKnown == 1 {
			account.LastNetSize = row.NetSize
		}
		acc.summary.Accounts = append(acc.summary.Accounts, account)
		if row.Equity != nil {
			total := *row.Equity
			if acc.summary.EquityTotal != nil {
				total += *acc.summary.EquityTotal
			}
			acc.summary.EquityTotal = &total
		}
		if account.LastNetSize != nil {
			total := *account.LastNetSize
			if acc.summary.NetSizeTotal != nil {
				total += *acc.summary.NetSizeTotal
			}
			acc.summary.NetSizeTotal = &total
		}
	}

	out := make([]argusDTO.InstanceSummaryDTO, 0, len(byInstance))
	for _, acc := range byInstance {
		s := acc.summary
		if s.Signals > 0 {
			s.OpenRate = round4(float64(s.Opened) / float64(s.Signals))
		}
		s.RealizedPnl = acc.pnl
		s.Variants = sortedStrings(acc.variants)
		sort.Slice(s.Accounts, func(i, j int) bool { return s.Accounts[i].AccountLabel < s.Accounts[j].AccountLabel })
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceKey < out[j].InstanceKey })
	return out
}

// ─── 筛选项 ──────────────────────────────────────────────────────────────────

// GetFilterOptions 一次拉齐四个页面要用的全部筛选项。
//
// 所有维度都从**事件表里真实出现过的值**枚举，而不是让前端手打 instanceKey
// 或猜账户名；账户项一律带 instance_key，因为账户名会跨实例重号。
func (s *ArgusEventService) GetFilterOptions(instanceKey, instanceKeys string) (*argusDTO.FilterOptionsDTO, error) {
	keys, err := resolveInstanceKeys(instanceKey, instanceKeys)
	if err != nil {
		return nil, err
	}
	result := argusDTO.FilterOptionsDTO{
		Instances:      []argusDTO.InstanceOptionDTO{},
		Accounts:       []argusDTO.AccountOptionDTO{},
		Instruments:    []string{},
		Variants:       []string{},
		ConfigVersions: []argusDTO.ConfigVersionOptionDTO{},
		Strengths:      StrengthOptions(),
		Sources: []argusDTO.OptionDTO{
			{Value: fmt.Sprint(eventstore.SourceLive), Label: SourceLabel(eventstore.SourceLive)},
			{Value: fmt.Sprint(eventstore.SourceBackfill), Label: SourceLabel(eventstore.SourceBackfill)},
		},
	}
	for _, event := range AllEvents() {
		result.EventKinds = append(result.EventKinds, argusDTO.OptionDTO{Value: event, Label: EventLabel(event)})
	}
	for _, kind := range GateKinds() {
		result.GateKinds = append(result.GateKinds, argusDTO.OptionDTO{Value: kind, Label: GateLabel(kind)})
	}

	registry := map[string]argusDTO.InstanceOptionDTO{}
	if instances, err := s.configService.ListInstances(false); err == nil {
		for _, item := range instances {
			registry[item.InstanceKey] = argusDTO.InstanceOptionDTO{
				InstanceKey: item.InstanceKey, InstanceName: item.InstanceName,
				Enabled: item.Enabled, Registered: true,
			}
		}
	}
	instanceRows, err := s.strategyEventRepository.ListInstanceOptions()
	if err != nil {
		return nil, err
	}
	for _, row := range instanceRows {
		option, ok := registry[row.InstanceKey]
		if !ok {
			option = argusDTO.InstanceOptionDTO{InstanceKey: row.InstanceKey, InstanceName: row.InstanceKey}
		}
		option.EventCount = row.EventCount
		option.FirstTs = row.FirstTs
		option.LastTs = row.LastTs
		result.Instances = append(result.Instances, option)
	}

	accountRows, err := s.strategyEventRepository.ListAccountOptions(keys)
	if err != nil {
		return nil, err
	}
	for _, row := range accountRows {
		result.Accounts = append(result.Accounts, argusDTO.AccountOptionDTO{
			InstanceKey: row.InstanceKey, AccountLabel: row.AccountLabel, Uid: row.Uid,
			Variant: row.Variant, Instrument: row.Instrument, EventCount: row.EventCount,
			FirstTs: row.FirstTs, LastTs: row.LastTs,
		})
	}
	if result.Instruments, err = s.strategyEventRepository.ListDistinct("instrument", keys); err != nil {
		return nil, err
	}
	if result.Variants, err = s.strategyEventRepository.ListDistinct("variant", keys); err != nil {
		return nil, err
	}
	versionRows, err := s.strategyEventRepository.ListConfigVersionOptions(keys)
	if err != nil {
		return nil, err
	}
	for _, row := range versionRows {
		result.ConfigVersions = append(result.ConfigVersions, argusDTO.ConfigVersionOptionDTO{
			InstanceKey: row.InstanceKey, ConfigVersion: row.ConfigVersion,
			EventCount: row.EventCount, FirstTs: row.FirstTs, LastTs: row.LastTs,
		})
	}
	rng, err := s.strategyEventRepository.EventTimeRange(repository.EventFilter{InstanceKeys: keys})
	if err != nil {
		return nil, err
	}
	result.DataRange = argusDTO.WindowDTO{Start: rng.FirstTs, End: rng.LastTs, Resolved: "latest-data"}
	return &result, nil
}
