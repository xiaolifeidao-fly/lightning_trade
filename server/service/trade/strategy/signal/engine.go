package signal

import (
	"math/rand"
	"strings"
	"time"

	argusMonitor "argus_single/pkg/monitor"
	argusTrade "argus_single/pkg/trade"
)

// 出场归因。前四类是引擎自己的判定，eod 是"回放窗口用尽仍未平"。
const (
	ExitTrailing    = "trailing"    // 移动止盈
	ExitCatastrophe = "catastrophe" // 兜底止损
	ExitReduce      = "reduce"      // 反向减仓把净仓削到 0（净仓模式特有）
	ExitEod         = "eod"         // 窗口结束仍持仓，按最后一根收盘标记
)

// Episode 一次持仓生命周期：某一侧的仓位从 0 张累积到平仓/削零为止。
// 它是逐笔明细的落库单位（trade_backtest_trade 一行 = 一个 episode）。
type Episode struct {
	Side         string
	OpenedAt     time.Time
	ClosedAt     time.Time
	AvgPx        float64 // 平仓时刻的累积均价
	ClosePx      float64 // 成交价（口径见 EvalMode）
	Contracts    int     // 平仓张数
	MaxContracts int     // 生命周期内的最大张数
	AddCount     int     // 加仓次数（含首次建仓）
	RoiPct       float64 // 平仓 ROI%（含杠杆）
	Pnl          float64 // 本次平仓的已实现盈亏（不含此前的减仓）
	ReducedPnl   float64 // 生命周期内减仓锁利累计
	Fee          float64 // 生命周期内累计手续费（开+减+平）
	PeakPct      float64 // trail 峰值；回测用 1m 极值，系统性偏高，见 fidelity
	TrailActive  bool
	Reason       string
	Open         bool // true = eod 未平
}

// EquityPoint 逐根 MTM 权益点（已实现 − 手续费 + 浮动）。
type EquityPoint struct {
	At  time.Time
	MTM float64
}

// book 单方向累积仓 + 移动止盈状态。TrailState 直接用实盘的结构体，
// 保证峰值/激活/加仓 rebase 三个语义不可能与实盘漂移。
type book struct {
	side  string
	size  int
	avg   float64
	trail argusMonitor.TrailState

	openedAt   time.Time
	addCount   int
	maxSize    int
	fee        float64
	reducedPnl float64
	// pendingExit 被扰动推迟的出场原因（见 Perturb.ExitLateProb）；空 = 无。
	// 下一根 bar 上、该根信号成交之后，按该根收盘平掉整个仓。削零 reset 时一并清掉。
	pendingExit string
}

func (b *book) roi(px float64, leverage int) float64 {
	// 口径同 pkg/trade/reduce_pnl.go netRoiPct：价格有利变动幅度 × 杠杆。
	if b.avg <= 0 {
		return 0
	}
	if b.side == "long" {
		return (px - b.avg) / b.avg * float64(leverage) * 100
	}
	return (b.avg - px) / b.avg * float64(leverage) * 100
}

func (b *book) pnlPerContract(px, face float64) float64 {
	// 口径同 pkg/trade/reduce_pnl.go estimateReducePnl。
	if b.side == "long" {
		return (px - b.avg) * face
	}
	return (b.avg - px) * face
}

func (b *book) add(px float64, n int, at time.Time) {
	if b.size == 0 {
		b.openedAt = at
		b.trail = argusMonitor.TrailState{}
		b.reducedPnl = 0
		b.fee = 0
		b.maxSize = 0
		b.addCount = 0
	}
	b.avg = (b.avg*float64(b.size) + px*float64(n)) / float64(b.size+n)
	b.size += n
	b.addCount++
	if b.size > b.maxSize {
		b.maxSize = b.size
	}
}

func (b *book) reset() {
	b.size = 0
	b.avg = 0
	b.trail = argusMonitor.TrailState{}
	b.reducedPnl = 0
	b.fee = 0
	b.addCount = 0
	b.maxSize = 0
	b.openedAt = time.Time{}
	b.pendingExit = ""
}

// Engine 事件驱动的盘口信号回测引擎。非并发安全：一个 run 一个实例。
type Engine struct {
	p       Params
	books   map[string]*book
	exitCfg argusMonitor.ExitConfig
	cap     int
	capOK   bool

	trend    *argusMonitor.TrendTracker
	trendNow time.Time // 趋势闸读动量的时刻：最后一个喂进 tracker 的样本时刻

	realized  float64
	fees      float64
	episodes  []Episode
	equity    []EquityPoint
	reduces   int
	skipCap   int
	skipGate  int
	skipTrend int
	// regimeLabels 每个自然日的**决策时可知**状态标签（取前一日，见 regime_scale.go）。
	// 整个窗口算一次，全路径共用——"8/19 前一日是不是单边"不该随路径变化。
	regimeLabels map[string]string
	// skipRegime 只被"路由压低后的上限"拦住的入场次数（原上限本可放行）。
	// 与 skipCap 分开计：否则区分不出"本来就顶格了"和"是路由拦的"，
	// 而后者才是这个机制到底有没有咬住的唯一证据。
	skipRegime int
	// trendStopCount 兜底线被收紧过多少次判定（不是平仓次数——每根 bar 的每次
	// 判定都计一次）。它是"这一格里机制到底有没有咬住"的唯一证据：
	// 扫参时某格与基线结果相同，可能是机制没生效、也可能是生效了但没改变结局，
	// 没有这个计数分不清。
	trendStopCount int
	maxStack       int
	barCount       int
	lastPx         float64
	firstBar       time.Time
	lastBar        time.Time

	// 扰动（Perturb.ExitLateProb）：exitRng==nil 即关闭，判定路径与从前逐字节一致。
	exitRng      *rand.Rand
	exitLateProb float64
	exitsDelayed int
}

// setExitPerturb 装上出场延迟扰动。Replay 按 Input.Perturb 调用；测试可直接调。
func (e *Engine) setExitPerturb(rng *rand.Rand, prob float64) {
	e.exitRng, e.exitLateProb = rng, prob
}

// closeOrDefer 出场判定成立时的落点：无扰动直接平；有扰动则按概率推迟到下一根
// （记在 book.pendingExit），同一笔只推迟一次——再推就不是"晚 60 秒"而是"不平"了。
func (e *Engine) closeOrDefer(bk *book, px float64, at time.Time, reason string) {
	if e.exitRng != nil && bk.pendingExit == "" && e.exitRng.Float64() < e.exitLateProb {
		bk.pendingExit = reason
		e.exitsDelayed++
		return
	}
	e.closeBook(bk, px, at, reason)
}

// NewEngine 构造引擎。p 应已 Normalize（Replay 会做）。
func NewEngine(p Params) *Engine {
	e := &Engine{
		p:     p,
		books: map[string]*book{"long": {side: "long"}, "short": {side: "short"}},
	}
	// 闸与止损共用同一个动量源（实盘也是共用 trade.trend_gate.window_hours）。
	// 条件是**任一**启用：只开止损、不开闸是一个要能单独研究的组合，
	// 写成"闸启用才建 tracker"会让那个组合静默失效。
	if p.TrendGateWindowHours > 0 && (p.TrendGateThresholdPct > 0 || (p.TrendStopTriggerPct > 0 && p.TrendStopPct > 0)) {
		e.trend = argusMonitor.NewTrendTracker(time.Duration(p.TrendGateWindowHours * float64(time.Hour)))
	}
	if p.CapOverride > 0 {
		e.setCap(p.CapOverride)
	}
	return e
}

// setCap 冻结仓位上限并据此算出分档退出配置（分档边界 = ceil/floor(ratio × N_max)）。
func (e *Engine) setCap(n int) {
	e.cap = n
	e.capOK = true
	e.exitCfg = argusMonitor.BuildExitConfig(n, argusMonitor.TrailParams{
		TierSmallRatio:     e.p.TierSmallRatio,
		TierLargeRatio:     e.p.TierLargeRatio,
		Small:              argusMonitor.Tier{ActivatePct: e.p.SmallActivatePct, GivebackFrac: e.p.SmallGiveback},
		Medium:             argusMonitor.Tier{ActivatePct: e.p.MediumActivatePct, GivebackFrac: e.p.MediumGiveback},
		Large:              argusMonitor.Tier{ActivatePct: e.p.LargeActivatePct, GivebackFrac: e.p.LargeGiveback},
		CatastropheStopPct: e.p.CatastropheStopPct,
	})
}

// ensureCap 懒冻结上限，语义照抄实盘 PositionCapGuard.EnsureInit：
// 第一个算得出来的 N_max 缓存到进程结束，之后价格再变也不重算。
// 回测里同样只在第一个可用价格上算一次——否则上限会随价格漂移，
// 与实盘行为不一致，也会让同一组参数在不同窗口给出不同 cap。
func (e *Engine) ensureCap(price float64) {
	if e.capOK {
		return
	}
	if n, ok := argusTrade.ComputeMaxContracts(e.p.RiskEquity, price, e.p.capParams()); ok {
		e.setCap(n)
	}
}

// Cap 已冻结的仓位上限（未冻结返回 0,false）。
func (e *Engine) Cap() (int, bool) { return e.cap, e.capOK }

// SeedRegimeLabels 灌入行情路由用的日标签。由 Replay 在拿到全窗口 K 线后调用；
// 不灌 = 路由关闭（resolveRegimeScale 查不到标签就返回系数 1）。
func (e *Engine) SeedRegimeLabels(bars []Bar) {
	if e.p.RegimeScaleLabels == "" {
		return
	}
	e.regimeLabels = PriorDayLabels(bars, e.p.regimeThresholds())
}

// entryCap 本次入场判定适用的上限：行情路由命中时按系数压低，否则就是 e.cap。
func (e *Engine) entryCap(at time.Time) int {
	sc := resolveRegimeScale(e.p, e.regimeLabels, at)
	if sc >= 1 {
		return e.cap
	}
	return scaledCap(e.cap, sc)
}

// admitEntry 本次入场（目标张数 want）是否放行，并把拦下的原因计到对应计数上。
func (e *Engine) admitEntry(want int, at time.Time) bool {
	if want <= e.entryCap(at) {
		return true
	}
	if want <= e.cap {
		e.skipRegime++
	}
	e.skipCap++
	return false
}

// Seed 灌入窗口起点的旧仓。必须在任何信号/K 线之前调用。
func (e *Engine) Seed(sp SeedPosition) {
	if !sp.OK() {
		return
	}
	b := e.books[strings.ToLower(sp.Side)]
	if b == nil {
		return
	}
	b.size, b.avg, b.openedAt = sp.Size, sp.AvgPx, sp.At
	b.maxSize, b.addCount = sp.Size, 1
	// last_size 直接设成种子张数：种子不是"本轮加仓"，不该触发 rebase。
	b.trail.LastSize = sp.Size
}

// ObserveTrend 喂一个价格样本给趋势闸的动量数据源。
// 数据源是 1m 收盘（实盘是 tick，TrendTracker 内部按分钟去重成 close 语义，
// 两者口径一致）；覆盖不足一个窗口时 Momentum 返回 ok=false，判定层放行，
// 与实盘重启后未回填的行为相同。
func (e *Engine) ObserveTrend(at time.Time, px float64) {
	if e.trend != nil {
		e.trend.Observe(at, px)
		e.trendNow = at
	}
}

func (e *Engine) fee(px float64, n int, b *book) {
	f := e.p.TakerFee * e.p.FaceValue * px * float64(n)
	e.fees += f
	if b != nil {
		b.fee += f
	}
}

func (e *Engine) netBook() *book {
	for _, side := range []string{"long", "short"} {
		if b := e.books[side]; b.size > 0 {
			return b
		}
	}
	return nil
}

// OnSignal 消费一次真实触发。px 是本次成交价（口径由 Replay 按 EntryPx 决定）。
//
// 判定顺序严格照抄 pkg/trade/manager.go executeSignalTrades_From_WEB：
// 反向减仓 → reverse_gate（趋势闸不拦减仓）；全新开仓/加仓 → 先趋势闸、再仓位上限。
func (e *Engine) OnSignal(s Signal, px float64) {
	if px <= 0 {
		return
	}
	e.ensureCap(px)
	if !e.capOK {
		// fail-closed，与 WouldExceedCap 无法初始化时跳过开仓一致。
		e.skipCap++
		return
	}
	orderSize := e.p.EffectiveOrderSize(s.OrderSize)
	side := strings.ToLower(s.Side)
	b := e.books[side]
	if b == nil {
		return
	}

	if e.p.Mode == ModeDual {
		// 双向形态：每侧各自累积、无反向门控（研究对照口径）。
		if !e.passTrendGate(side, s) {
			return
		}
		if !e.admitEntry(b.size+orderSize, s.Ts) {
			return
		}
		b.add(px, orderSize, s.Ts)
		e.fee(px, orderSize, b)
		return
	}

	nb := e.netBook()
	switch {
	case nb == nil:
		// 全新开仓
		if !e.passTrendGate(side, s) {
			return
		}
		if !e.admitEntry(orderSize, s.Ts) {
			return
		}
		b.add(px, orderSize, s.Ts)
		e.fee(px, orderSize, b)
	case nb.side == side:
		// 同向加仓
		if !e.passTrendGate(side, s) {
			return
		}
		if !e.admitEntry(nb.size+orderSize, s.Ts) {
			return
		}
		nb.add(px, orderSize, s.Ts)
		e.fee(px, orderSize, nb)
	default:
		// 反向减仓：走实盘门控（盈利足够且不翻转才放行）
		dec := argusTrade.EvaluateReverseGate(nb.side, nb.avg, px, e.p.Leverage, e.p.GateMinProfitPct, orderSize, nb.size)
		if !dec.Allow {
			e.skipGate++
			return
		}
		pnl := nb.pnlPerContract(px, e.p.FaceValue) * float64(orderSize)
		e.realized += pnl
		nb.reducedPnl += pnl
		e.fee(px, orderSize, nb)
		nb.size -= orderSize
		e.reduces++
		if nb.size == 0 {
			// 削零 = 一个 episode 的终点，归因为 reduce。本次平掉的张数是
			// orderSize，盈亏已全部计进 ReducedPnl（Pnl 留 0，避免重复计入）。
			e.recordEpisode(nb, px, s.Ts, ExitReduce, orderSize, 0, false)
			nb.reset()
		}
	}
}

// effectiveStop 本次判定生效的兜底线（ROI 正数）。
//
// 动量取当前 bar 的窗口动量，与趋势闸读的是同一个 tracker、同一个 trendNow——
// 两处若各读一次，闸与止损会在同一根 bar 上看到不同的动量。
// tracker 未构造（两者都关或没给窗口）或窗口未满时 ok=false，
// ResolveTrendStop 的 fail-safe 分支原样返回 S。
func (e *Engine) effectiveStop(side string) float64 {
	p := argusTrade.TrendStopParams{
		BaseStopPct:  e.p.CatastropheStopPct,
		TriggerPct:   e.p.TrendStopTriggerPct,
		TightStopPct: e.p.TrendStopPct,
	}
	if e.trend == nil {
		stop, _ := argusTrade.ResolveTrendStop(side, 0, false, p)
		return stop
	}
	mom, ok := e.trend.Momentum(e.trendNow)
	stop, tightened := argusTrade.ResolveTrendStop(side, mom, ok, p)
	if tightened {
		e.trendStopCount++
	}
	return stop
}

// passTrendGate 趋势闸：只拦全新开仓与同向加仓，不拦减仓（与 manager.go 分流一致）。
func (e *Engine) passTrendGate(side string, s Signal) bool {
	if e.p.TrendGateThresholdPct <= 0 || e.trend == nil {
		return true
	}
	// 动量读在"最后一个样本时刻"而不是信号 ts：tracker 的窗口裁剪以最新样本
	// 为基准（prune 只留窗口边界前最后一点），用落在上一根之内的信号 ts 去问
	// Momentum 会让参考点落进窗口内、恒返回 ok=false，趋势闸形同关闭。
	// 1m 数据下两者最多差一根，这是 60 秒粒度的固有对齐误差，已写进 fidelity 注记。
	mom, ok := e.trend.Momentum(e.trendNow)
	if dec := argusTrade.EvaluateTrendGate(side, mom, ok, e.p.TrendGateThresholdPct); dec.Block {
		e.skipTrend++
		return false
	}
	return true
}

// OnBar 推进一根 1m K 线：逐侧判定出场 → 记堆积 → 记 MTM 权益点。
//
// 出场判定分两步（顺序即"按不利方向结算"的落点）：
//
//	(1) 兜底止损：用一根内的极值判断是否穿越触发线。实盘每 5 秒判一次，
//	    盘中穿越必然被抓到；只看收盘会系统性漏掉插针，那是**高估**收益。
//	    成交价 = 触发线再过冲 CatastropheOvershootRoiPts 个 ROI 点；
//	    悲观口径下取"极值与过冲价里更差的那个"。
//	(2) 移动止盈：状态机一律走实盘的 monitor.EvaluateExit，只有喂进去的
//	    评估价按 EvalMode 不同：
//	      close 口径      → 收盘价（与金标准脚本 non-pess 分支逐行等价）；
//	      悲观口径        → 先用"本根不利极值 vs 上一根的峰值线"做一次触发前置
//	                        判定（成交在触发价），再用有利极值推进峰值/激活。
//	                        前置判定必要的原因：一根内的新高不该保护同一根内的
//	                        回撤，否则悲观口径失去意义。
func (e *Engine) OnBar(b Bar) {
	e.ensureCap(b.Close)
	e.barCount++
	if e.firstBar.IsZero() {
		e.firstBar = b.Ts
	}
	e.lastBar = b.Ts
	e.lastPx = b.Close

	for _, side := range []string{"long", "short"} {
		bk := e.books[side]
		if bk.size == 0 {
			continue
		}
		if !e.capOK {
			continue // 分档边界未知，无法判定；下一根自愈
		}
		if bk.pendingExit != "" {
			// 上一根被推迟的出场：本根信号已在 Replay 里先成交（加仓/减仓都算进去了），
			// 现在按本根收盘平掉整个仓。09-17 实盘就是这样把三次开空并进了一笔要平的空单。
			e.closeBook(bk, b.Close, b.Ts, bk.pendingExit)
			continue
		}
		e.evalBook(bk, b)
	}

	stack := 0
	for _, side := range []string{"long", "short"} {
		stack += e.books[side].size
	}
	if stack > e.maxStack {
		e.maxStack = stack
	}
	e.equity = append(e.equity, EquityPoint{At: b.Ts, MTM: e.realized - e.fees + e.floating(b.Close)})
}

func (e *Engine) evalBook(bk *book, bar Bar) {
	lev := float64(e.p.Leverage)
	pess := e.p.EvalMode == EvalPessimistic

	// (1) 兜底止损：一根内穿越即触发。
	// 生效线走 trade.ResolveTrendStop——与实盘 handleTrailingPnl 同一个纯函数，
	// 逆向动量 ≥ trigger 时收到 TrendStopPct，其余（含 warmup 动量不可算）沿用 S。
	stop := e.effectiveStop(bk.side)
	pen := stop + e.p.CatastropheOvershootRoiPts
	var trigPx, fillPx float64
	var crossed bool
	if bk.side == "long" {
		trigPx = bk.avg * (1 - stop/(lev*100))
		fillPx = bk.avg * (1 - pen/(lev*100))
		crossed = bar.Low <= trigPx
		if pess && crossed {
			fillPx = minf(bar.Low, fillPx)
		}
	} else {
		trigPx = bk.avg * (1 + stop/(lev*100))
		fillPx = bk.avg * (1 + pen/(lev*100))
		crossed = bar.High >= trigPx
		if pess && crossed {
			fillPx = maxf(bar.High, fillPx)
		}
	}
	if crossed {
		e.closeOrDefer(bk, fillPx, bar.Ts, ExitCatastrophe)
		return
	}

	tier := e.tierFor(bk.size)
	if pess {
		adverse, favorable := bar.Low, bar.High
		if bk.side == "short" {
			adverse, favorable = bar.High, bar.Low
		}
		// 加仓 rebase 用收盘价（均价变了、pct 跳变），与金标准 pess 分支同口径。
		// 提前做掉，EvaluateExit 里的 rebase 分支就不会再用有利极值重算一遍。
		if bk.size > bk.trail.LastSize {
			bk.trail.PeakPct = bk.roi(bar.Close, e.p.Leverage)
			bk.trail.Active = bk.trail.Active || bk.trail.PeakPct >= tier.ActivatePct
		}
		bk.trail.LastSize = bk.size
		// 不利极值是否已击穿上一根留下的移动止盈线
		if bk.trail.Active {
			exitRoi := bk.trail.PeakPct * (1 - tier.GivebackFrac)
			if bk.roi(adverse, e.p.Leverage) <= exitRoi {
				px := bk.avg * (1 + exitRoi/(lev*100))
				if bk.side == "short" {
					px = bk.avg * (1 - exitRoi/(lev*100))
				}
				e.closeOrDefer(bk, px, bar.Ts, ExitTrailing)
				return
			}
		}
		// 有利极值推进激活/峰值。前置判定已挡掉能触发的情形，
		// 因此这里的 EvaluateExit 只会返回 Hold（adverse ≤ favorable）。
		act, st := argusMonitor.EvaluateExit(bk.size, bk.roi(favorable, e.p.Leverage), e.exitCfg, bk.trail)
		bk.trail = st
		if act == argusMonitor.ActionTrailingClose {
			e.closeOrDefer(bk, favorable, bar.Ts, ExitTrailing)
		} else if act == argusMonitor.ActionCatastropheStop {
			e.closeOrDefer(bk, favorable, bar.Ts, ExitCatastrophe)
		}
		return
	}

	// close 口径：一次评估，状态机与决策全交给实盘函数。
	act, st := argusMonitor.EvaluateExit(bk.size, bk.roi(bar.Close, e.p.Leverage), e.exitCfg, bk.trail)
	bk.trail = st
	switch act {
	case argusMonitor.ActionCatastropheStop:
		e.closeOrDefer(bk, bar.Close, bar.Ts, ExitCatastrophe)
	case argusMonitor.ActionTrailingClose:
		e.closeOrDefer(bk, bar.Close, bar.Ts, ExitTrailing)
	}
}

// tierFor 该张数命中的档位。BuildExitConfig 只导出边界，档位选择在实盘是
// ExitConfig 的私有方法，这里按同一口径展开（小 ≤ floor(small×N)、大 ≥ ceil(large×N)）。
func (e *Engine) tierFor(size int) argusMonitor.Tier {
	if size <= e.exitCfg.SmallMaxContracts {
		return e.exitCfg.Small
	}
	if size >= e.exitCfg.LargeMinContracts {
		return e.exitCfg.Large
	}
	return e.exitCfg.Medium
}

func (e *Engine) closeBook(bk *book, px float64, at time.Time, reason string) {
	pnl := bk.pnlPerContract(px, e.p.FaceValue) * float64(bk.size)
	size := bk.size
	e.realized += pnl
	e.fee(px, bk.size, bk)
	e.recordEpisode(bk, px, at, reason, size, pnl, false)
	bk.reset()
}

// recordEpisode 落一条持仓生命周期记录。contracts/pnl 显式传入而不是从 bk 读，
// 因为削零场景下 bk.size 已经先减到 0（盈亏也已计进 ReducedPnl）。
func (e *Engine) recordEpisode(bk *book, px float64, at time.Time, reason string, contracts int, pnl float64, open bool) {
	e.episodes = append(e.episodes, Episode{
		Side:         bk.side,
		OpenedAt:     bk.openedAt,
		ClosedAt:     at,
		AvgPx:        bk.avg,
		ClosePx:      px,
		Contracts:    contracts,
		MaxContracts: bk.maxSize,
		AddCount:     bk.addCount,
		RoiPct:       bk.roi(px, e.p.Leverage),
		Pnl:          pnl,
		ReducedPnl:   bk.reducedPnl,
		Fee:          bk.fee,
		PeakPct:      bk.trail.PeakPct,
		TrailActive:  bk.trail.Active,
		Reason:       reason,
		Open:         open,
	})
}

func (e *Engine) floating(px float64) float64 {
	total := 0.0
	for _, side := range []string{"long", "short"} {
		if bk := e.books[side]; bk.size > 0 {
			total += bk.pnlPerContract(px, e.p.FaceValue) * float64(bk.size)
		}
	}
	return total
}

// Finish 收尾：把窗口结束仍持有的仓位记成 eod episode（不计已实现、不收平仓费），
// 让"期末浮动"与"未平持仓"在逐笔明细里可见而不是凭空消失。
func (e *Engine) Finish() {
	for _, side := range []string{"long", "short"} {
		if bk := e.books[side]; bk.size > 0 {
			e.recordEpisode(bk, e.lastPx, e.lastBar, ExitEod, bk.size,
				bk.pnlPerContract(e.lastPx, e.p.FaceValue)*float64(bk.size), true)
		}
	}
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
