package argus_event

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
	tradeRepository "service/trade/repository"

	"argus_single/pkg/eventlog"
)

// 本文件实现「与 K 线对齐的时间轴聚合」：把触发点按 K 线网格分桶，
// 让行情主图（r12）能把蜡烛、触发点标记、净持仓阶梯画在同一条时间轴上。
//
// 网格对齐用 time.Truncate（自 Unix 纪元起算，等价 UTC 对齐），与交易所的
// K 线开盘时刻同一套网格——不能用"本地午夜对齐"，1d 周期会整体错开 8 小时。
// 桶键最后按本地墙钟输出，与事件 ts 同口径。

// intervalDuration 周期字符串 → 时长，口径对齐 service/trade 的同名函数。
//
// 唯一刻意不支持的是 1w：time.Truncate 按 Unix 纪元起算，一周的整除点落在
// 周四，与交易所的周线开盘（周一）对不上，分桶会整体错位。真要周线聚合得先
// 从 K 线自身的 open_time 取网格，本任务用不到，先明确拒绝而不是给个错的桶。
func intervalDuration(interval string) (time.Duration, bool) {
	switch strings.TrimSpace(interval) {
	case "1m":
		return time.Minute, true
	case "3m":
		return 3 * time.Minute, true
	case "5m":
		return 5 * time.Minute, true
	case "15m":
		return 15 * time.Minute, true
	case "30m":
		return 30 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "2h":
		return 2 * time.Hour, true
	case "4h":
		return 4 * time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "8h":
		return 8 * time.Hour, true
	case "12h":
		return 12 * time.Hour, true
	case "1d":
		return 24 * time.Hour, true
	default:
		return 0, false
	}
}

// GetTimeline 与 K 线对齐的时间轴聚合。
//
// instanceKey 必填：净持仓阶梯与开仓率是实例内的量，跨实例相加没有意义
// （三实例的仓位上限分别是 15 / 26+8 / 246）。
func (s *ArgusEventService) GetTimeline(q argusDTO.TimelineQueryDTO) (*argusDTO.TimelineDTO, error) {
	instanceKey, err := requireSingleInstance(q.InstanceKey)
	if err != nil {
		return nil, err
	}
	interval := strings.TrimSpace(q.Interval)
	if interval == "" {
		interval = "1m"
	}
	dur, ok := intervalDuration(interval)
	if !ok {
		return nil, fmt.Errorf("不支持的周期: %s（可选 1m/5m/15m/30m/1h/4h/1d）", q.Interval)
	}
	instrument := normalizeInstrument(q.Instrument)
	platform := tradeRepository.NormalizeKlinePlatform(q.PlatformCode)

	filter := repository.EventFilter{
		InstanceKeys: []string{instanceKey},
		Events:       AllEvents(),
	}
	if instrument != "" {
		filter.Instruments = []string{instrument}
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
	// 默认窗口 24 小时，锚在已入库数据的最新时刻上。
	window, err := s.resolveWindow(filter, start, end, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	filter.Start, filter.End = window.Start, window.End

	result := argusDTO.TimelineDTO{
		InstanceKey:   instanceKey,
		Instrument:    instrument,
		Symbol:        instrument,
		Interval:      interval,
		PlatformCode:  platform,
		Window:        window,
		Klines:        []argusDTO.SliceKlineDTO{},
		Buckets:       []argusDTO.TimelineBucketDTO{},
		CompareKlines: []argusDTO.SliceKlineDTO{},
	}
	// 双源对比：第二条 K 线序列走同一个窗口与同一个格式化口径，
	// 与主序列共用已解析的 startAt/endAt，不重复扫事件表。
	comparePlatform := ""
	if strings.TrimSpace(q.ComparePlatformCode) != "" {
		if normalized := tradeRepository.NormalizeKlinePlatform(q.ComparePlatformCode); normalized != platform {
			comparePlatform = normalized
			result.ComparePlatformCode = normalized
		}
	}
	if window.Start == "" || window.End == "" {
		return &result, nil
	}
	startAt, err := parseEventTime(window.Start, false)
	if err != nil {
		return nil, err
	}
	endAt, err := parseEventTime(window.End, true)
	if err != nil {
		return nil, err
	}

	rows, err := s.strategyEventRepository.ListEvents(filter, 0, maxAggregateRows, true)
	if err != nil {
		return nil, err
	}
	result.EventTotal = len(rows)
	result.Truncated = len(rows) >= maxAggregateRows

	if instrument != "" {
		if klines, err := s.listKlines(platform, instrument, interval, startAt, endAt); err == nil {
			result.Klines = klines
		} else {
			return nil, err
		}
		if comparePlatform != "" {
			if klines, err := s.listKlines(comparePlatform, instrument, interval, startAt, endAt); err == nil {
				result.CompareKlines = klines
			} else {
				return nil, err
			}
			result.CompareCoverage = klineCoverage(len(result.CompareKlines), startAt, endAt, dur)
		}
	}
	result.Buckets = buildTimelineBuckets(rows, startAt, endAt, dur)
	result.Coverage = klineCoverage(len(result.Klines), startAt, endAt, dur)
	return &result, nil
}

// buildTimelineBuckets 把事件按 K 线网格分桶。
//
// 净持仓阶梯的推导口径：逐条事件维护每个账户的最近一次净仓快照——
// 开仓/加仓/减仓与三类拦截事件的 size 就是当时的净仓张数（open 记的是
// 成交后的净仓，见 pkg/trade/manager.go 的 postNet），五类平仓事件之后
// 该账户归零。桶末值 = 各账户当前值之和；桶内无事件时沿用上一桶，形成阶梯。
//
// 这是**派生量而不是实测量**：部分平仓、进程重启期间的仓位变化都追不到。
// episode 级的精确生命周期是 r10 的事，这里只求图上那条阶梯不失真到误导。
func buildTimelineBuckets(rows []*repository.StrategyEventRow, start, end time.Time, dur time.Duration) []argusDTO.TimelineBucketDTO {
	type acc struct {
		total, open, capSkip, gateBlock, trendSkip, exit int
		maxAbsGap                                        *float64
		pnl                                              *float64
		configVersion                                    uint64
	}
	buckets := map[time.Time]*acc{}
	netByAccount := map[string]int{}
	netAtBucket := map[time.Time]int{}

	bucketOf := func(t time.Time) time.Time { return t.Truncate(dur) }

	for _, row := range rows {
		ts, err := parseEventTime(row.Ts, false)
		if err != nil {
			continue
		}
		key := bucketOf(ts)
		b := buckets[key]
		if b == nil {
			b = &acc{}
			buckets[key] = b
		}
		b.total++
		switch row.Event {
		case eventlog.EvOpen:
			b.open++
		case eventlog.EvCapSkip:
			b.capSkip++
		case eventlog.EvGateBlock:
			b.gateBlock++
		case eventlog.EvTrendSkip:
			b.trendSkip++
		default:
			if ResultKindOf(row.Event) == ResultKindExit {
				b.exit++
			}
		}
		if row.GapBp != nil {
			abs := math.Abs(*row.GapBp)
			if b.maxAbsGap == nil || abs > *b.maxAbsGap {
				v := abs
				b.maxAbsGap = &v
			}
		}
		// 只累加真正实现了盈亏的事件。loss_alert 的 pnl 是**未实现浮亏**
		// （account_monitor.go 取 pos.UnrealizedProfit），而且同一个持仓每过
		// 告警冷却就再报一次——累进「已实现盈亏」既错了口径，也把同一笔浮亏
		// 重复计数。实测 roc 实例 6 条 loss_alert 合计 -53.89，把真实的 +3.36
		// 算成了 -50.53。
		if row.Pnl != nil && ResultKindOf(row.Event) != ResultKindAlert {
			sum := *row.Pnl
			if b.pnl != nil {
				sum += *b.pnl
			}
			b.pnl = &sum
		}
		if row.ConfigVersion > b.configVersion {
			b.configVersion = row.ConfigVersion
		}

		switch {
		case ResultKindOf(row.Event) == ResultKindExit && row.Event != eventlog.EvLossAlert:
			netByAccount[row.AccountLabel] = 0
		case row.Size != nil:
			netByAccount[row.AccountLabel] = *row.Size
		}
		total := 0
		for _, v := range netByAccount {
			total += v
		}
		netAtBucket[key] = total
	}

	out := make([]argusDTO.TimelineBucketDTO, 0, len(buckets))
	lastNet := 0
	for t := bucketOf(start); !t.After(end); t = t.Add(dur) {
		item := argusDTO.TimelineBucketDTO{Time: t.In(time.Local).Format(eventTimeLayout)}
		if b, ok := buckets[t]; ok {
			item.Total = b.total
			item.Open = b.open
			item.CapSkip = b.capSkip
			item.GateBlock = b.gateBlock
			item.TrendSkip = b.trendSkip
			item.Exit = b.exit
			item.MaxAbsGapBp = b.maxAbsGap
			item.RealizedPnl = b.pnl
			item.ConfigVersion = b.configVersion
		}
		if net, ok := netAtBucket[t]; ok {
			lastNet = net
		}
		item.NetSizeEnd = lastNet
		out = append(out, item)
	}
	return out
}

// klineCoverage K 线覆盖率，供前端的「缺口回填」空状态。
func klineCoverage(actual int, start, end time.Time, dur time.Duration) argusDTO.TimelineCoverageDTO {
	expected := int(end.Sub(start)/dur) + 1
	if expected < 0 {
		expected = 0
	}
	coverage := argusDTO.TimelineCoverageDTO{Expected: expected, Actual: actual}
	if expected > 0 {
		coverage.CoveragePct = math.Round(float64(actual)/float64(expected)*10000) / 100
		coverage.Missing = expected - actual
		if coverage.Missing < 0 {
			coverage.Missing = 0
		}
	}
	return coverage
}

// ─── 权益曲线 ────────────────────────────────────────────────────────────────

// GetEquityCurve 按账户返回降采样后的权益曲线。
//
// 数据源是 balance_sample（每账户每分钟一条心跳，占全部事件 61%），
// 默认按 10 分钟降采样——直接返回逐分钟点位会让 30 天窗口吐出 4 万多个点，
// 前端图表扛不住也看不清。
//
// equity 只在 equity_known = 1 的样本上聚合：0 与负权益是真实的极端回撤样本
// （浮亏恰抵平余额 / 权益打穿），不能当成"未知"丢掉。
func (s *ArgusEventService) GetEquityCurve(q argusDTO.EquityQueryDTO) (*argusDTO.EquityCurveDTO, error) {
	instanceKey, err := requireSingleInstance(q.InstanceKey)
	if err != nil {
		return nil, err
	}
	bucketSeconds := q.BucketSeconds
	if bucketSeconds <= 0 {
		bucketSeconds = 600
	}
	if bucketSeconds < 60 {
		// 心跳本身就是分钟级，比 60 秒更细的桶只会造出一堆单样本点。
		bucketSeconds = 60
	}
	start, err := normalizeBound(q.Start, false)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	end, err := normalizeBound(q.End, true)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	window, err := s.resolveWindow(repository.EventFilter{InstanceKeys: []string{instanceKey}}, start, end, 24*time.Hour)
	if err != nil {
		return nil, err
	}
	result := argusDTO.EquityCurveDTO{
		InstanceKey:   instanceKey,
		Window:        window,
		BucketSeconds: bucketSeconds,
		Series:        []argusDTO.EquitySeriesDTO{},
		Notice:        "125x 杠杆下绝对权益摆动很大，跨账户对比请看 changePct（相对本序列首个已知权益的变动百分比）。",
	}
	filter := repository.BalanceFilter{
		InstanceKeys:  []string{instanceKey},
		AccountLabels: splitCSV(q.AccountLabels),
		Start:         window.Start,
		End:           window.End,
	}
	rows, err := s.balanceRepository.ListBuckets(filter, bucketSeconds)
	if err != nil {
		return nil, err
	}
	latest, err := s.balanceRepository.ListLatestByAccount(filter)
	if err != nil {
		return nil, err
	}
	meta := make(map[string]*repository.BalanceLatestRow, len(latest))
	for _, row := range latest {
		meta[row.AccountLabel] = row
	}
	result.Series = buildEquitySeries(instanceKey, rows, meta)
	return &result, nil
}

// buildEquitySeries 把降采样桶按账户拼成序列，并算出相对首个已知权益的变动。
func buildEquitySeries(instanceKey string, rows []*repository.BalanceBucketRow, meta map[string]*repository.BalanceLatestRow) []argusDTO.EquitySeriesDTO {
	byAccount := map[string]*argusDTO.EquitySeriesDTO{}
	order := make([]string, 0)
	for _, row := range rows {
		series, ok := byAccount[row.AccountLabel]
		if !ok {
			series = &argusDTO.EquitySeriesDTO{
				InstanceKey:  instanceKey,
				AccountLabel: row.AccountLabel,
			}
			if m, ok := meta[row.AccountLabel]; ok {
				series.Uid = m.Uid
				series.Variant = m.Variant
			}
			byAccount[row.AccountLabel] = series
			order = append(order, row.AccountLabel)
		}
		point := argusDTO.EquityPointDTO{
			Time:      row.BucketTs,
			Balance:   row.AvgBalance,
			Equity:    row.AvgEquity,
			Upl:       row.AvgUpl,
			MinEquity: row.MinEquity,
			MaxEquity: row.MaxEquity,
			Samples:   row.Samples,
		}
		if row.AvgEquity != nil {
			if series.FirstEquity == nil {
				series.FirstEquity = row.AvgEquity
			}
			series.LastEquity = row.AvgEquity
		}
		series.Points = append(series.Points, point)
	}
	sort.Strings(order)
	out := make([]argusDTO.EquitySeriesDTO, 0, len(order))
	for _, label := range order {
		series := byAccount[label]
		// 基准是本序列首个已知权益；基准为 0 时百分比无定义，留空不硬算。
		if series.FirstEquity != nil && *series.FirstEquity != 0 {
			base := *series.FirstEquity
			for i := range series.Points {
				if series.Points[i].Equity == nil {
					continue
				}
				pct := (*series.Points[i].Equity - base) / base * 100
				series.Points[i].ChangePct = &pct
			}
			if series.LastEquity != nil {
				pct := (*series.LastEquity - base) / base * 100
				series.ChangePct = &pct
			}
		}
		out = append(out, *series)
	}
	return out
}
