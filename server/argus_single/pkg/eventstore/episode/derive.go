package episode

import (
	"fmt"
	"sort"
	"time"

	"argus_single/pkg/eventlog"
	"argus_single/pkg/eventstore"
)

// closeEvents 五种"有平仓事件"的出场方式。第六种 reduce_to_zero 没有事件，
// 只能从仓位快照归零推出来（设计文档 §6.2）。
var closeEvents = map[string]bool{
	eventlog.EvTrailingClose:   true,
	eventlog.EvCatastropheStop: true,
	eventlog.EvFixedClose:      true,
	eventlog.EvExternalClose:   true,
	eventlog.EvManualClose:     true,
}

// strategyExit 出场是不是策略自己决定的。external_close（交易所侧/他人操作）
// 与 manual_close（TG 一键平仓）不是，按任务口径不计入策略胜率。
func strategyExit(kind string) bool {
	return kind != eventlog.EvExternalClose && kind != eventlog.EvManualClose
}

// Stats 一次派生的过程计数，全部要在报告里露出来——静默丢弃是这套派生最大的风险。
type Stats struct {
	Events           int            // 参与派生的事件条数
	Accounts         int            // (instance, account, instrument) 组数
	Episodes         int            //
	ByExit           map[string]int // 出场方式分布，key 含 "open"（仍持仓）
	PositionGaps     int            // |Δsize| != orderSize 的事件数
	StaleSnapshot    int            // Δsize = 0：持仓快照滞后（≤5s），读到的是旧仓位
	TruncatedHead    int            // 只看到出场、没看到建仓的 episode
	OrphanEvents     map[string]int // 无在场 episode 时收到的观测类事件
	OrphanSamples    []string       // 前几条孤儿观测的定位串
	MissingPnlEvents int            // 实现了但盈亏未知的次数（2026-07-24 前的减仓）
	UnattributedPnl  float64        // 落不到任何决策上的已实现盈亏（截断头）
	SideFlipRejected int            // 反向单反而把仓位做大的异常事件，只记不改仓位
	// SideFlipSamples 前几条异常的定位串（时刻 + 打码账户）。异常只报个数而不
	// 给定位信息，等于逼下一个人重跑一遍派生才能查——r6 的缺失哈希样例同理。
	SideFlipSamples []string
}

// sampleLimit 每类异常最多留几条定位样例。
const sampleLimit = 5

func newStats() Stats {
	return Stats{ByExit: map[string]int{}, OrphanEvents: map[string]int{}}
}

// Derive 把一批 strategy_event 归集成 episode + 决策行。纯函数：同样的输入
// （含顺序）必然产出同样的输出，不读时钟、不碰数据库。rebuiltAt 由调用方给。
//
// 事件按 (ts, id) 升序处理：同一秒内可能有多条（实测 28.7% 的信号分钟内触发
// ≥2 次），id 升序即 JSONL 落盘顺序，是同秒事件唯一可靠的先后依据。
func Derive(events []*eventstore.StrategyEvent, rebuiltAt time.Time) ([]*Episode, Stats) {
	stats := newStats()
	ordered := make([]*eventstore.StrategyEvent, 0, len(events))
	for _, e := range events {
		if e == nil {
			continue
		}
		ordered = append(ordered, e)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if !ordered[i].Ts.Equal(ordered[j].Ts) {
			return ordered[i].Ts.Before(ordered[j].Ts)
		}
		return ordered[i].Id < ordered[j].Id
	})
	stats.Events = len(ordered)

	type bookKey struct{ instance, account, instrument string }
	books := map[bookKey]*book{}
	order := make([]bookKey, 0, 8)
	for _, e := range ordered {
		key := bookKey{e.InstanceKey, e.AccountLabel, e.Instrument}
		b, ok := books[key]
		if !ok {
			b = &book{rebuiltAt: rebuiltAt, stats: &stats}
			books[key] = b
			order = append(order, key)
		}
		b.consume(e)
	}
	stats.Accounts = len(order)

	var episodes []*Episode
	for _, key := range order {
		b := books[key]
		b.finish()
		episodes = append(episodes, b.done...)
	}
	sort.SliceStable(episodes, func(i, j int) bool {
		if !episodes[i].FirstEventAt.Equal(episodes[j].FirstEventAt) {
			return episodes[i].FirstEventAt.Before(episodes[j].FirstEventAt)
		}
		if episodes[i].InstanceKey != episodes[j].InstanceKey {
			return episodes[i].InstanceKey < episodes[j].InstanceKey
		}
		return episodes[i].AccountLabel < episodes[j].AccountLabel
	})
	stats.Episodes = len(episodes)
	for _, ep := range episodes {
		if ep.ExitKind == nil {
			stats.ByExit["open"]++
			continue
		}
		stats.ByExit[*ep.ExitKind]++
	}
	return episodes, stats
}

// entryState 一笔建仓决策在派生过程中的可变部分。
type entryState struct {
	row *EpisodeEntry
	// remaining 仍在场的张数。等比例退场会摊出小数，见 EpisodeEntry 的注释。
	remaining float64
}

// book 一个 (instance, account, instrument) 上的持仓账本。
type book struct {
	rebuiltAt time.Time
	stats     *Stats

	ep      *Episode
	entries []*entryState
	// hidden 在场、但建仓决策不在数据窗口内的张数（截断头）。它必须进归集的
	// 分母：把整笔盈亏摊给看得见的那几笔决策，等于把看不见那几张的收益也算到
	// 它们头上，正是决策归集想消灭的那类假信号。
	hidden float64
	// started 这个账本上有没有开过 episode。只有第一条事件才能用"净仓大于下单量"
	// 反推出截断头——后面的时刻我们已经亲眼看着仓位归零，净仓是可信的。
	started bool
	net     int // 仓位快照口径的当前净仓张数，事件里的 size 是权威值
	done    []*Episode
}

// consume 处理一条事件，并把 last_event_at 顶到该事件时刻——仍持仓的 episode
// 没有 closed_at，深度保真度的窗口右端只能靠它。
func (b *book) consume(e *eventstore.StrategyEvent) {
	b.dispatch(e)
	if b.ep != nil {
		b.ep.LastEventAt = e.Ts
	}
}

func (b *book) dispatch(e *eventstore.StrategyEvent) {
	switch e.Event {
	case eventlog.EvOpen:
		b.onOpen(e)
	case eventlog.EvLossAlert:
		if !b.ensureEpisode(e) {
			return
		}
		b.ep.LossAlertCount++
		b.observeRoi(e.RoiPct)
	case eventlog.EvGateBlock:
		if !b.ensureEpisode(e) {
			return
		}
		b.ep.GateBlockCount++
		// gate_block 的 roi_pct 是拦截当下整个净仓的 ROI，与 loss_alert 同口径，
		// 拿来一起收深度能把 0 ~ −150% 这段空白补上一部分（仍然只是观测下界）。
		b.observeRoi(e.RoiPct)
	case eventlog.EvCapSkip:
		if !b.ensureEpisode(e) {
			return
		}
		b.ep.CapSkipCount++
	case eventlog.EvTrendSkip:
		if !b.ensureEpisode(e) {
			return
		}
		b.ep.TrendSkipCount++
	default:
		if closeEvents[e.Event] {
			b.onClose(e)
			return
		}
		b.orphan(e)
	}
}

// ensureEpisode 观测类事件（loss_alert / gate_block / cap_skip / trend_skip）
// 落在"我们以为空仓"的时刻时，用它自带的净仓快照开一个截断头 episode。
//
// 实测这不是罕见边界：roc 实例的日志窗口一开始，账户A 就已经扛着 14 张多单，
// 头 30 条观测事件（10 条 gate_block + 20 条 loss_alert，最深 −170%）全落在
// 建仓之前。丢掉它们等于把这段扛单过程从库里抹掉——而扛住浮亏等价格回来
// 正是这个策略赖以为生的部分。
//
// 净仓快照为 0（或缺失）时不开：那说明这条事件本来就不对应任何持仓。
func (b *book) ensureEpisode(e *eventstore.StrategyEvent) bool {
	if b.ep != nil {
		return true
	}
	net := intOr(e.Size, 0)
	side := strOr(e.NetSide)
	if side == "" {
		side = strOr(e.Side)
	}
	if net <= 0 || side == "" {
		b.orphan(e)
		return false
	}
	b.startTruncated(e, side, net)
	b.stats.TruncatedHead++
	return true
}

// orphan 无在场持仓时收到的观测事件：仓位快照滞后或数据窗口截断都会造成。
// 只计数并留定位样例，不据此凭空开一个 episode——观测事件里的 size 是净仓
// 快照，正是不可信的那个字段。
func (b *book) orphan(e *eventstore.StrategyEvent) {
	b.stats.OrphanEvents[e.Event]++
	if len(b.stats.OrphanSamples) < sampleLimit {
		b.stats.OrphanSamples = append(b.stats.OrphanSamples,
			fmt.Sprintf("%s %s size=%d", e.Ts.Format(eventstore.TsLayout), e.Event, intOr(e.Size, 0)))
	}
}

// onOpen 处理 open 事件：同方向是加仓，反方向是反向减仓（reverse_gate 放行的
// 盈利减仓）。两者在 JSONL 里是同一个事件类型，只能靠 side 与仓位快照区分。
//
// size 为 NULL 一律读成 0——JSONL 全字段带 omitempty，减到 0 的那一笔的 size
// 就被省掉了（实测 30 条）。加仓不可能落到 0，所以这个读法没有歧义。
func (b *book) onOpen(e *eventstore.StrategyEvent) {
	resulting := intOr(e.Size, 0)
	intended := intOr(e.OrderSize, 0)
	side := strOr(e.Side)

	// e.Side 是**下单方向**，不是净仓方向：给 short 减仓要买入，落库就是
	// side=long。manager.go 只在 isReduction 成立时才写 NetSide（见
	// pkg/trade/manager.go 的 openEv.NetSide 赋值），所以 NetSide 非空等价于
	// 「这一单在减仓」，且它才是真实净仓方向。
	//
	// 建账本必须用净仓方向。账本第一条恰好是减仓单时（实测 roc 账户A
	// 2026-09-08 22:18:50 首条即 side=long/net_side=short/size=5），按下单方向
	// 建账会把 short 记成 long，此后 18 笔真正的加空仓（side=short）全部撞进
	// 下面 delta<0 的分支被记成「反向单做大仓位」，net 从此冻结在 5，
	// 而 3 笔真减仓被当成加仓把峰值抬到 7——reverse_gate 其实一次都没漏。
	//
	// 加仓单没有 NetSide，此时下单方向就是净仓方向。
	posSide := side
	if netSide := strOr(e.NetSide); netSide != "" {
		posSide = netSide
	}

	if b.ep == nil {
		if resulting <= 0 {
			// 只看到"把仓位减到 0"的那一笔：建仓在数据窗口之前，opened_at 只能留 NULL。
			// 净仓方向优先读 NetSide；老事件没这个字段时退回「下单方向的反面」
			// ——减到 0 必然是减仓单，其下单方向与净仓方向相反。
			zeroSide := strOr(e.NetSide)
			if zeroSide == "" {
				zeroSide = opposite(side)
			}
			b.startTruncated(e, zeroSide, intOr(e.OrderSize, 0))
			b.realize(e.Pnl, true)
			b.ep.ReduceCount++
			b.closeEpisode(e, ExitReduceToZero)
			b.stats.TruncatedHead++
			return
		}
		if netSide := strOr(e.NetSide); netSide != "" {
			// 账本第一条就是减仓单（NetSide 非空即在减仓）。减仓单让仓位变小，
			// 它不是"决定开多少张"的时刻，不能记成建仓决策——旧写法会走下面的
			// 截断头分支再 applyAdd(orderSize)，凭空造出一笔建仓决策去领归因。
			//
			// 减仓前的仓位 = 减完的净仓 + 本单张数，这些张的建仓全在窗口之外，
			// 因此整段进 hidden：realize 会把这笔已实现盈亏全部记成不可归集，
			// retire 再按比例退掉减掉的那几张。
			prior := resulting + intended
			b.startTruncated(e, netSide, prior)
			b.stats.TruncatedHead++
			b.ep.ReduceCount++
			b.realize(e.Pnl, true)
			b.observeRoi(e.RoiPct)
			b.retire(float64(intended))
			b.net = resulting
			return
		}
		if !b.started && intended > 0 && resulting > intended {
			// 账本的第一条事件就报出比本次下单量更大的净仓 ⇒ 建仓在数据窗口之前。
			// 实测 roc 账户B 的首条事件是 open size=6 / orderSize=1，那 5 张的
			// 建仓决策根本没进过日志。把 6 张全记给这一笔会凭空造出一个"一次开 6 张"
			// 的决策，让它领走整段持仓的归因。
			//
			// 代价：§6.5 的"信号突发累加一次下单多张"（7/05 01:39 A 0→3，orderSize=1）
			// 若恰好发生在账本第一条，会被误判成截断头。两种误判里这个方向更安全——
			// 少归因是缺数据，多归因是假数据，而 opened_at=NULL 让下游能直接排除。
			b.startTruncated(e, posSide, resulting-intended)
			b.applyAdd(e, intended, intended)
			return
		}
		b.start(e, posSide)
		b.applyAdd(e, resulting, intended)
		return
	}

	if side == b.ep.Side {
		b.applyAdd(e, resulting-b.net, intended)
		if b.net <= 0 {
			// 加仓事件反而把仓位打到 0：只可能是仓位跳变（快照滞后 / 未记录的外部平仓）。
			b.ep.ReduceCount++
			b.closeEpisode(e, ExitReduceToZero)
		}
		return
	}

	// 反向减仓。先按当前全部在场张数摊掉这笔已实现盈亏，再退场——顺序不能反：
	// 减掉的 k 张与留下的 N−k 张在净额均价账本里是同一批浮盈，凭什么只算给 k 张。
	delta := b.net - resulting
	if delta != intended {
		b.markGap()
	}
	if delta == 0 {
		// 持仓快照滞后：实测 2026-07-07 05:10 两账户同一秒同时下单，≤5s 的持仓
		// 快照还没刷新，读到的是旧仓位（设计文档 §6.5）。按 D6 记标记不断开，
		// 也不凭 orderSize 猜真实仓位——猜错会让后面每一笔归因整体错位。
		b.stats.StaleSnapshot++
		return
	}
	if delta < 0 {
		// 反向单反而把仓位做大：reverse_gate 会拦掉翻转单，理论上不该出现。
		// 只记异常、不动仓位，宁可少改也不要凭空造出一个反方向的 episode。
		b.stats.SideFlipRejected++
		if len(b.stats.SideFlipSamples) < sampleLimit {
			b.stats.SideFlipSamples = append(b.stats.SideFlipSamples,
				fmt.Sprintf("%s side=%s size=%d net=%d", e.Ts.Format(eventstore.TsLayout), side, resulting, b.net))
		}
		return
	}
	b.ep.ReduceCount++
	b.realize(e.Pnl, true)
	b.observeRoi(e.RoiPct)
	b.retire(float64(delta))
	b.net -= delta
	if b.net <= 0 {
		b.closeEpisode(e, ExitReduceToZero)
	}
}

// applyAdd 按仓位快照实算加仓张数。事件里的 orderSize 只是"打算下几张"，
// 实测 0.8% 的 open 与快照对不上（信号突发累加一次下单多张、两账户同秒
// 导致快照滞后、未记录的外部平仓），一律以快照为准并记跳变标记（§6.5）。
func (b *book) applyAdd(e *eventstore.StrategyEvent, delta, intended int) {
	if delta != intended {
		b.markGap()
	}
	switch {
	case delta > 0:
		b.addEntry(e, delta)
		b.net += delta
	case delta < 0:
		// 同方向事件却让仓位变小：当减仓处理，但盈亏未知（事件里没有）。
		b.realize(nil, true)
		b.retire(float64(-delta))
		b.net += delta
	default:
		b.stats.StaleSnapshot++
	}
}

func (b *book) start(e *eventstore.StrategyEvent, side string) {
	opened := e.Ts
	b.ep = b.newEpisode(e, side)
	b.ep.OpenedAt = &opened
}

// startTruncated 开头被数据窗口截断：opened_at 留 NULL，first_event_at 记
// 实际看到的第一条。variant/config_version 只能取这条事件的，已非决策时刻，
// 因此这类 episode 不应进入按 variant 的归因统计（opened_at IS NULL 即判据）。
func (b *book) startTruncated(e *eventstore.StrategyEvent, side string, net int) {
	b.ep = b.newEpisode(e, side)
	b.ep.HasPositionGap = 1
	b.net = net
	b.hidden = float64(net)
	b.ep.MaxSize = net
}

func (b *book) newEpisode(e *eventstore.StrategyEvent, side string) *Episode {
	b.started = true
	return &Episode{
		InstanceKey:   e.InstanceKey,
		UID:           e.UID,
		AccountLabel:  e.AccountLabel,
		Instrument:    e.Instrument,
		Side:          side,
		FirstEventAt:  e.Ts,
		Variant:       e.Variant,
		ConfigVersion: e.ConfigVersion,
		LastEventAt:   e.Ts,
		DepthFidelity: DepthAlertSampled,
		RebuiltAt:     b.rebuiltAt,
	}
}

func (b *book) addEntry(e *eventstore.StrategyEvent, size int) {
	row := &EpisodeEntry{
		InstanceKey:   e.InstanceKey,
		UID:           e.UID,
		AccountLabel:  e.AccountLabel,
		Instrument:    e.Instrument,
		Side:          b.ep.Side,
		DecidedAt:     e.Ts,
		EventHash:     e.EventHash,
		AddedSize:     size,
		OrderSize:     e.OrderSize,
		Variant:       e.Variant,
		ConfigVersion: e.ConfigVersion,
		GapBp:         e.GapBp,
		AvgPx:         e.AvgPx,
		LastPx:        e.LastPx,
		RebuiltAt:     b.rebuiltAt,
	}
	b.entries = append(b.entries, &entryState{row: row, remaining: float64(size)})
	b.ep.Entries = append(b.ep.Entries, row)
	b.ep.AddCount++
	b.ep.EntrySizeTotal += size
	if b.net+size > b.ep.MaxSize {
		b.ep.MaxSize = b.net + size
	}
}

// realize 把一笔已实现盈亏按各笔决策的在场张数等比例摊回去（决策归集口径）。
//
// pnl 为 nil 表示"实现了但金额未知"——2026-07-24 之前的反向减仓事件只写 size
// 不写 pnl。这种情况**不能当 0**：0 是"不赚不亏"这个具体结论，未知是没有结论。
// 记进 missing_pnl_events，让分层统计能把这些决策整体剔除或单独标注。
func (b *book) realize(pnl *float64, strategy bool) {
	total := b.total()
	if total <= 0 {
		if pnl != nil {
			b.stats.UnattributedPnl += *pnl
			if b.ep != nil {
				b.ep.UnattributedPnl += *pnl
				b.ep.Pnl += *pnl
				if strategy {
					b.ep.PnlStrategy += *pnl
				}
				b.ep.RealizedEvents++
			}
		}
		return
	}
	if pnl == nil {
		b.stats.MissingPnlEvents++
		b.ep.MissingPnlEvents++
		for _, entry := range b.entries {
			if entry.remaining > 0 {
				entry.row.MissingPnlEvents++
			}
		}
		return
	}
	for _, entry := range b.entries {
		if entry.remaining <= 0 {
			continue
		}
		share := *pnl * entry.remaining / total
		entry.row.AttributedPnl += share
		if strategy {
			entry.row.AttributedPnlStrategy += share
		}
	}
	if b.hidden > 0 {
		orphaned := *pnl * b.hidden / total
		b.ep.UnattributedPnl += orphaned
		b.stats.UnattributedPnl += orphaned
	}
	b.ep.Pnl += *pnl
	if strategy {
		b.ep.PnlStrategy += *pnl
	}
	b.ep.RealizedEvents++
}

// retire 退场 size 张，按各笔决策的在场张数等比例摊（不是 FIFO，理由见
// EpisodeEntry 的注释）。整仓平掉时退化成"全部清零"。
func (b *book) retire(size float64) {
	total := b.total()
	if total <= 0 || size <= 0 {
		return
	}
	ratio := size / total
	if ratio > 1 {
		ratio = 1
	}
	b.hidden -= b.hidden * ratio
	for _, entry := range b.entries {
		if entry.remaining <= 0 {
			continue
		}
		cut := entry.remaining * ratio
		entry.remaining -= cut
		entry.row.ClosedSize += cut
	}
}

// total 在场总张数 = 可归集的 + 不可归集的。与 b.net（仓位快照口径）应当相等，
// 但权重一律用它：它跟着实际的加/退场走，不会被一次快照跳变带偏。
func (b *book) total() float64 {
	return b.openSize() + b.hidden
}

func (b *book) openSize() float64 {
	var total float64
	for _, entry := range b.entries {
		if entry.remaining > 0 {
			total += entry.remaining
		}
	}
	return total
}

func (b *book) onClose(e *eventstore.StrategyEvent) {
	if b.ep == nil {
		b.startTruncated(e, strOr(e.Side), intOr(e.Size, 0))
		b.stats.TruncatedHead++
	} else if size := intOr(e.Size, 0); size != b.net {
		// 平仓事件报的持仓张数与我们跟踪的净仓不一致：仓位跳变，标记但不断开。
		b.markGap()
	}
	strategy := strategyExit(e.Event)
	b.realize(e.Pnl, strategy)
	b.observeRoi(e.RoiPct)
	if e.PeakPct != nil {
		b.ep.PeakPct = e.PeakPct
	}
	b.retire(b.total())
	b.net = 0
	b.closeEpisode(e, e.Event)
}

func (b *book) closeEpisode(e *eventstore.StrategyEvent, kind string) {
	ep := b.ep
	closed := e.Ts
	exit := kind
	ep.ClosedAt = &closed
	ep.LastEventAt = closed
	ep.ExitKind = &exit
	ep.ExitEventHash = e.EventHash
	ep.ExitRoiPct = e.RoiPct
	if strategyExit(kind) {
		ep.StrategyAttributable = 1
	}
	if ep.OpenedAt != nil {
		seconds := int(closed.Sub(*ep.OpenedAt) / time.Second)
		ep.DurationSec = &seconds
	}
	b.seal()
}

// finish 收尾：流末仍持仓的 episode 保留 closed_at=NULL，并把在场张数落进
// open_size——"仍持仓"是结论，不是缺数据。
func (b *book) finish() {
	if b.ep == nil {
		return
	}
	b.seal()
}

func (b *book) seal() {
	ep := b.ep
	for _, entry := range b.entries {
		if entry.remaining > 0 {
			entry.row.OpenSize = entry.remaining
		}
		entry.row.ExitKind = ep.ExitKind
		entry.row.StrategyAttributable = ep.StrategyAttributable
		if entry.row.MissingPnlEvents == 0 && entry.row.OpenSize == 0 {
			entry.row.PnlKnown = 1
		}
	}
	ep.OpenSize = b.total()
	ep.HiddenSize = b.hidden
	b.done = append(b.done, ep)
	b.ep = nil
	b.entries = nil
	b.net = 0
	b.hidden = 0
}

func (b *book) markGap() {
	b.ep.HasPositionGap = 1
	b.ep.PositionGapCount++
	b.stats.PositionGaps++
}

// observeRoi 收一个 ROI 观测点。min/max 都只是**观测**极值：深度的数据源
// loss_alert 是双重过滤的采样（ROI < −150% 才发 + 5 分钟冷却），真实最深
// 必然更深（实测最深 −362.2%，而同一笔的真实最深是 −374.7%）。
func (b *book) observeRoi(roi *float64) {
	if roi == nil || b.ep == nil {
		return
	}
	value := *roi
	if b.ep.MinRoiPctObserved == nil || value < *b.ep.MinRoiPctObserved {
		v := value
		b.ep.MinRoiPctObserved = &v
	}
	if b.ep.MaxRoiPctObserved == nil || value > *b.ep.MaxRoiPctObserved {
		v := value
		b.ep.MaxRoiPctObserved = &v
	}
}

func opposite(side string) string {
	switch side {
	case "long":
		return "short"
	case "short":
		return "long"
	default:
		return side
	}
}

func intOr(v *int, fallback int) int {
	if v == nil {
		return fallback
	}
	return *v
}

func strOr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
