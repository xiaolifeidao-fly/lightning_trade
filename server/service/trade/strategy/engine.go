// Package strategy 是实盘与回测共享的策略引擎(状态机)。
//
// 它不知道自己跑在实盘还是回测——价格从哪来由 PriceFeed 决定、结果去哪由 Sink 决定(见 feed.go)。
// 这是「一个引擎、两个驱动器」的核心：同一套入场/出场判定，实盘用实时 tick 喂、回测用历史 K 线回放喂，
// 从而保证回测里验证有效的策略，在实盘行为完全一致。
//
// 本包是纯领域逻辑、零 IO / 零 DB 依赖，便于单测；落库/取价交由调用方实现 PriceFeed/Sink。
package strategy

import "time"

// Direction 开仓方向。
type Direction string

const (
	Long    Direction = "long"
	Short   Direction = "short"
	Neutral Direction = "neutral"
)

// EntryMode 入场方式。
type EntryMode string

const (
	EntryMarket   EntryMode = "market"   // 即时市价(开盘价)开仓——现状口径
	EntryPullback EntryMode = "pullback" // 区间回踩限价开仓——等价格回到区间分位再成交
)

// 止盈止损来源(三选一)：每种来源带各自的「百分比」参数；缺数据时按 percent←predict 链回退。
const (
	SourcePercent  = "percent"  // 离入场价固定百分比：止盈/止损 = 入场价 ×(1±X%)
	SourcePredict  = "predict"  // 跟 AI 预测：止盈按区间分位γ、止损按失效价(可带缓冲%)
	SourcePressure = "pressure" // 跟 AI 压力面：止盈到对侧关键结构位、止损突破关键压力位缓冲%后
)

// State 单笔交易的生命周期状态：pending → open → closed，或 pending → expired。
type State string

const (
	StatePending State = "pending" // 已挂限价，待成交
	StateOpen    State = "open"    // 已成交，持仓中
	StateClosed  State = "closed"  // 已平仓
	StateExpired State = "expired" // 挂单超时未成交，放弃(未成交也是一种要记录的结果)
)

// CloseReason 平仓/收尾原因。
type CloseReason string

const (
	ReasonTP       CloseReason = "tp"
	ReasonSL       CloseReason = "sl"
	ReasonTrail    CloseReason = "trail"     // 移动止盈(峰值回撤)触发平仓
	ReasonEarlyCut CloseReason = "early_cut" // 早段疲软离场：前X%时间内峰值浮盈未达Y%，按当时价市价平仓
	// 早段逆行离场：本笔从未走出浮盈、逆行已达阈值(方向可能真做错)，在触及硬止损前先减损离场
	ReasonEarlyAdverse CloseReason = "early_adverse"
	ReasonTimeout      CloseReason = "timeout"
	ReasonExpired      CloseReason = "expired"
	ReasonManual       CloseReason = "manual"
)

// Prediction 引擎消费的预测输入(来自 oracle 预测层，统一绝对价口径)。
type Prediction struct {
	Trend        Direction
	RefPrice     float64 // 基准价(开仓参考/市价口径)
	High         float64 // 区间上沿(绝对价)
	Low          float64 // 区间下沿(绝对价)
	Invalidation float64 // 失效价(绝对价)，0=未给
	Confidence   float64
	Efficiency   float64 // 趋势效率 |中枢|/区间宽：用于 market/pullback 路由
	MovePct      float64 // 预测中枢幅度%(用于幅度门槛过滤)

	// 压力面(结构性价位，由信号时刻最近一次压力面分析注入；0=未给)。
	// 做空在 KeyResistance 上方止损、KeySupport 处止盈；做多反之。
	KeyResistance float64 // 关键阻力位(上方做空压力位)
	KeySupport    float64 // 关键支撑位(下方做多压力位)

	// 止盈止损区间覆盖(可选)：交易周期那条预测的上下沿/失效价。给了则按持仓周期口径设 TP/SL，
	// 入场/门槛仍用上面 High/Low。0=未给则回退到 High/Low/Invalidation。
	TPBandHigh     float64 // 交易周期预测上沿
	TPBandLow      float64 // 交易周期预测下沿
	TPInvalidation float64 // 交易周期预测失效价

	// 复合方向门槛(可选)：信号时刻各更高周期(4h/12h/1d)最近一条预测的方向。
	// 启用 Params.RequireCompositeDir 时，须全部与本笔方向一致才放行，否则忽略不建仓。
	HigherTrends []Direction
}

// Params 一种策略的参数集(对应 trade_strategy 一行 = 一种计算方式)。
type Params struct {
	// 开仓门槛
	TrendFilter   Direction // long/short/""=both
	MinConfidence float64
	MinMovePct    float64
	// 复合方向门槛：启用后须信号时刻 4h/12h/1d 的方向全部与本笔方向一致才建仓，否则忽略。
	RequireCompositeDir bool

	// 入场策略
	EntryMode       EntryMode
	Alpha           float64       // 入场分位 0~1：限价离区间下沿(多)/上沿(空)的比例
	Gamma           float64       // 止盈分位 0~1：止盈价离区间对沿的比例
	EntryTTL        time.Duration // 挂单有效期：未成交则放弃
	EfficiencyRoute float64       // >0 时按效率路由 market/pullback；0=固定用 EntryMode

	// 出场来源(percent/predict/pressure)，止盈止损各自独立。
	TakeProfitSource string // 止盈来源
	StopLossSource   string // 止损来源
	// 各来源对应的「百分比」参数：
	TakeProfitPct      float64       // percent 止盈：离入场价%
	StopLossPct        float64       // percent 止损：离入场价%
	PredictSLBufferPct float64       // predict 止损：突破失效价该%后止损(0=贴着失效价)
	PressureBufferPct  float64       // pressure 止盈/止损：离关键结构位的缓冲%
	TakeProfitFloorPct float64       // 兜底锁盈%：止盈目标比该值更远时，提前到该%锁盈(0=不约束)
	StopLossFloorPct   float64       // 兜底最小止损%：止损隐含亏损<该值则放宽到该值(0=不约束)
	HoldDuration       time.Duration // 持仓时长(交易周期)
	MaxHold            time.Duration // 持仓硬上限

	// 移动止盈(峰值回撤 + 时间收敛)，借鉴自 BADelay500W 的分级移动止盈，叠加本项目特有的交易周期时间轴。
	// 锚在「已实现的峰值浮盈」而非预设绝对目标：激活后浮盈每创新高就抬高峰值，回撤掉一定比例即落袋。
	// TrailActivatePct<=0 时整套关闭，退回静态止盈，不影响既有策略。
	TrailActivatePct float64 // 浮盈 ROI%(含杠杆)达此值才激活移动止盈
	TrailGiveback    float64 // 从峰值 ROI 回撤此比例即平仓(退出线=peak×(1-r))；周期初始值 r0(0~1)
	TrailGivebackMin float64 // 周期末的回撤比例(④时间收敛)：随持仓推进 r 从 TrailGiveback 线性收敛到此值；<=0 或 ≥TrailGiveback 时不收敛(固定用 TrailGiveback)

	// 早段疲软离场：持仓推进到交易周期的 EarlyCutTimePct% 时，若这段时间内的峰值浮盈 ROI%(含杠杆)
	// 仍未达到 EarlyCutMinProfitPct，则判定该笔"迟迟没走出利润"，按当时价格市价平仓止损离场(不再等止盈/超时)。
	// EarlyCutTimePct<=0 时整套关闭，不影响既有策略。
	EarlyCutTimePct      float64 // 触发时间点：持仓已过交易周期的百分比(0~100，0=关闭)
	EarlyCutMinProfitPct float64 // 该时点前的峰值浮盈 ROI%(含杠杆)门槛：低于则离场

	// 早段逆行离场(MAE 软止损)：与「早段疲软」互补——疲软盯"利润迟迟没来"(时间闸门)，这条盯"逆行来了"(逐根检查)。
	// 逆行浮亏 ROI%(含杠杆)达到 EarlyCutMaxAdversePct 时，在触及硬止损前先市价减损离场，
	// 专治"方向真做错、发现时已亏不少"。仅当本笔从未走出有意义浮盈时武装(见 EarlyCutArmProfitPct)。
	// EarlyCutMaxAdversePct<=0 时整套关闭，不影响既有策略。
	EarlyCutMaxAdversePct float64 // 逆行浮亏 ROI%(含杠杆)达此值即离场(正数表回撤幅度)
	EarlyCutArmProfitPct  float64 // 解除闸门：峰值有利浮盈 ROI% 达此值(这笔"工作过")则永久解除软止损，放行扛单/移动止盈；<=0=不设闸门，始终武装

	// 仓位与费率
	Leverage  float64
	Contracts int
	MakerFee  float64 // Maker 费率(限价/止盈)
	TakerFee  float64 // Taker 费率(市价/止损/超时)
}

// Order 单笔交易的运行态：实盘=一行持仓，回测=一行回测逐笔。
type Order struct {
	Direction    Direction
	EntryMode    EntryMode
	PlannedEntry float64
	TakeProfit   float64
	StopLoss     float64
	HoldDuration time.Duration

	// 移动止盈配置(开仓时从 Params 冻结，0=不启用)；Leverage 同时用于把价格幅度换算成含杠杆 ROI%。
	Leverage         float64
	TrailActivatePct float64
	TrailGiveback    float64
	TrailGivebackMin float64

	// 早段疲软离场配置(开仓时从 Params 冻结，0=不启用)。
	EarlyCutTimePct      float64
	EarlyCutMinProfitPct float64

	// 早段逆行离场(MAE 软止损)配置(开仓时从 Params 冻结，0=不启用)。
	EarlyCutMaxAdversePct float64
	EarlyCutArmProfitPct  float64

	State       State
	RequestedAt time.Time
	Deadline    time.Time // 挂单失效时间 = RequestedAt + EntryTTL

	OpenPrice float64
	OpenedAt  time.Time
	HoldUntil time.Time

	ClosePrice  float64
	CloseReason CloseReason
	ClosedAt    time.Time

	MaxPrice float64 // 持仓期间最高价
	MinPrice float64 // 持仓期间最低价

	// 移动止盈运行态：激活后持续记录峰值 ROI%，回撤到退出线即平仓。
	TrailActive bool
	PeakPnlPct  float64

	// 分时段峰值浮盈(供「前 X% 持仓时间内最大利润」分析)：按持仓已过比例(elapsed/HoldDuration)分十桶，
	// 记录各桶内最高含杠杆浮盈 ROI%。FavPeakDeciles() 再前向填充成"前(i+1)×10%时间内累积峰值"。
	favBucket  [10]float64
	favTouched [10]bool
}

// Quote 一次价格观测：实盘=一个 tick(High=Low=Price)，回测=一根 K 线(含 High/Low)。
// 触价判定统一用 High/Low，使两种驱动共用同一套出入场检测。
type Quote struct {
	Time  time.Time
	Price float64 // 收盘/最新价
	High  float64
	Low   float64
}

// Plan 把一条预测 + 一种策略参数，转成一笔待执行 Order(或拒绝)。
// 即「开仓门槛过滤 + 入场/出场价位推导」，纯函数，实盘与回测完全一致。
// 市价模式直接成交进 open；回踩模式返回 pending，等 Step 触价成交。
func Plan(p Prediction, s Params, now time.Time) (Order, bool) {
	dir := p.Trend
	if dir != Long && dir != Short {
		return Order{}, false
	}
	if s.TrendFilter != "" && s.TrendFilter != dir {
		return Order{}, false
	}
	if p.Confidence < s.MinConfidence || abs(p.MovePct) < s.MinMovePct {
		return Order{}, false
	}
	// 复合方向门槛：更高周期(4h/12h/1d)方向须全部与本笔一致，否则忽略不建仓。
	if s.RequireCompositeDir && !compositeAligned(dir, p.HigherTrends) {
		return Order{}, false
	}

	mode := routeMode(s, p.Efficiency)
	entry := entryPrice(dir, mode, p, s.Alpha)
	if entry <= 0 {
		return Order{}, false
	}
	// 入场用信号(预测周期)区间；止盈止损按交易周期区间(若给了覆盖)，使目标贴合持仓周期。
	band := tpBandPred(p)
	tp := takeProfit(dir, band, s, entry)
	sl := stopLoss(dir, band, s, entry)
	if tp <= 0 || sl <= 0 {
		return Order{}, false
	}
	// 止盈须在盈利一侧、止损须在亏损一侧，否则本笔价位彼此矛盾(常见于交易周期区间落到
	// 入场价另一侧，见 tpBandPred)：开仓当根就会立刻触及止盈价，被误记为"止盈"却实为亏损。
	// 源头拒单，不入场、不计入交易。
	if !profitSide(dir, entry, tp) || !lossSide(dir, entry, sl) {
		return Order{}, false
	}

	hold := s.HoldDuration
	if s.MaxHold > 0 && hold > s.MaxHold {
		hold = s.MaxHold
	}

	o := Order{
		Direction:             dir,
		EntryMode:             mode,
		PlannedEntry:          entry,
		TakeProfit:            tp,
		StopLoss:              sl,
		HoldDuration:          hold,
		Leverage:              s.Leverage,
		TrailActivatePct:      s.TrailActivatePct,
		TrailGiveback:         s.TrailGiveback,
		TrailGivebackMin:      s.TrailGivebackMin,
		EarlyCutTimePct:       s.EarlyCutTimePct,
		EarlyCutMinProfitPct:  s.EarlyCutMinProfitPct,
		EarlyCutMaxAdversePct: s.EarlyCutMaxAdversePct,
		EarlyCutArmProfitPct:  s.EarlyCutArmProfitPct,
		RequestedAt:           now,
		MaxPrice:              entry,
		MinPrice:              entry,
	}
	if mode == EntryMarket {
		o.fill(entry, now) // 市价：立即成交进 open
	} else {
		o.State = StatePending
		o.Deadline = now.Add(s.EntryTTL)
	}
	return o, true
}

// Step 用一次价格观测推进状态机；返回是否发生状态变化(供 Sink 落库)。
// 出口按「首触优先」：谁先被触及谁生效——实盘逐 tick 天然有序；
// 回测逐 K 线时见 exit() 的同根 K 线歧义约定。
func (o *Order) Step(q Quote) bool {
	switch o.State {
	case StatePending:
		if touched(o.Direction, o.PlannedEntry, q) {
			o.fill(o.PlannedEntry, q.Time)
			return true
		}
		if !q.Time.Before(o.Deadline) {
			o.State = StateExpired
			o.CloseReason = ReasonExpired
			o.ClosedAt = q.Time
			return true
		}
		return false
	case StateOpen:
		o.track(q)
		// 出口判定用「本根 K 线之前」的峰值(exit 内部读 PeakPnlPct)，再用本根行情更新峰值；
		// 保守约定：同一根 K 线内创出的新高不会反过来抬高本根的移动止盈退出线(避免回测高估)。
		if reason, price, ok := o.exit(q); ok {
			o.State = StateClosed
			o.ClosePrice = price
			o.CloseReason = reason
			o.ClosedAt = q.Time
			return true
		}
		o.updateTrail(q)
		return false
	default:
		return false // closed/expired 为终态
	}
}

// Terminal 报告 Order 是否已到终态(closed/expired)。
func (o *Order) Terminal() bool { return o.State == StateClosed || o.State == StateExpired }

func (o *Order) fill(price float64, at time.Time) {
	o.State = StateOpen
	o.OpenPrice = price
	o.OpenedAt = at
	o.HoldUntil = at.Add(o.HoldDuration)
	o.MaxPrice = price
	o.MinPrice = price
}

func (o *Order) track(q Quote) {
	if q.High > o.MaxPrice {
		o.MaxPrice = q.High
	}
	if o.MinPrice == 0 || q.Low < o.MinPrice {
		o.MinPrice = q.Low
	}
	o.trackFavDecile(q)
}

// trackFavDecile 把本根行情的有利极值(多看 High、空看 Low)换算成含杠杆浮盈 ROI%，
// 按持仓已过比例落入十桶之一，只保留桶内最高值。持仓时长缺失时不记录。
func (o *Order) trackFavDecile(q Quote) {
	if o.HoldDuration <= 0 || o.OpenedAt.IsZero() {
		return
	}
	var favPx float64
	if o.Direction == Long {
		favPx = q.High
	} else {
		favPx = q.Low
	}
	roi := o.roiAt(favPx)
	frac := q.Time.Sub(o.OpenedAt).Seconds() / o.HoldDuration.Seconds()
	if frac < 0 {
		frac = 0
	}
	idx := int(frac * 10)
	if idx < 0 {
		idx = 0
	}
	if idx > 9 {
		idx = 9
	}
	if !o.favTouched[idx] || roi > o.favBucket[idx] {
		o.favBucket[idx] = roi
		o.favTouched[idx] = true
	}
}

// FavTracked 报告是否记录过分时段峰值浮盈(至少一个观测)。
// 用于区分「有观测但全程零浮盈」(需落库，正是早段疲软要找的单)与「未成交/无持仓时长」(无数据)。
func (o *Order) FavTracked() bool {
	for _, touched := range o.favTouched {
		if touched {
			return true
		}
	}
	return false
}

// FavPeakDeciles 返回「前 (i+1)×10% 持仓时间内的累积最高浮盈 ROI%」：对分桶取前缀最大并前向填充。
// 无任何观测(未成交)时返回全 0。索引 i 对应"前 (i+1)×10% 时间"这一时段。
func (o *Order) FavPeakDeciles() [10]float64 {
	var out [10]float64
	var cur float64
	have := false
	for i := 0; i < 10; i++ {
		if o.favTouched[i] && (!have || o.favBucket[i] > cur) {
			cur = o.favBucket[i]
		}
		if o.favTouched[i] {
			have = true
		}
		if have {
			out[i] = cur // 前向填充：无观测的时段沿用上一时段的累积峰值
		}
	}
	return out
}

// leverage 返回有效杠杆(<=0 兜底为 1)，用于价格幅度↔含杠杆 ROI% 互换。
func (o *Order) leverage() float64 {
	if o.Leverage <= 0 {
		return 1
	}
	return o.Leverage
}

// roiAt 给定价格的含杠杆浮盈 ROI%(口径与 Settle/MarkToMarket 一致)。
func (o *Order) roiAt(price float64) float64 {
	if o.OpenPrice <= 0 {
		return 0
	}
	var diff float64
	if o.Direction == Long {
		diff = price - o.OpenPrice
	} else {
		diff = o.OpenPrice - price
	}
	return diff / o.OpenPrice * o.leverage() * 100
}

// priceAtROI 把一个目标 ROI%(含杠杆)换算回绝对价位(roiAt 的逆运算)。
func (o *Order) priceAtROI(roi float64) float64 {
	frac := roi / 100 / o.leverage()
	if o.Direction == Long {
		return o.OpenPrice * (1 + frac)
	}
	return o.OpenPrice * (1 - frac)
}

// givebackFrac 当前允许的峰值回撤比例 r：基线 TrailGiveback，按持仓进度 τ 线性收敛到 TrailGivebackMin。
// ④ 时间收敛——越接近交易周期末，r 越小(越不容忍回撤)，把临近超时还没到目标的浮盈尽量锁住。
// TrailGivebackMin<=0 或 ≥基线时不收敛，固定用基线。
func (o *Order) givebackFrac(now time.Time) float64 {
	r := o.TrailGiveback
	if r <= 0 {
		return 0
	}
	if o.TrailGivebackMin > 0 && o.TrailGivebackMin < r && o.HoldDuration > 0 {
		tau := now.Sub(o.OpenedAt).Seconds() / o.HoldDuration.Seconds()
		if tau < 0 {
			tau = 0
		}
		if tau > 1 {
			tau = 1
		}
		r -= (o.TrailGiveback - o.TrailGivebackMin) * tau
	}
	return r
}

// trailExitPrice 返回当前移动止盈退出价及其是否生效：退出 ROI = 峰值×(1-r(τ))，再换算成价位。
// 未激活(浮盈还没达 TrailActivatePct)时不生效。
func (o *Order) trailExitPrice(now time.Time) (float64, bool) {
	if !o.TrailActive {
		return 0, false
	}
	exitROI := o.PeakPnlPct * (1 - o.givebackFrac(now))
	return o.priceAtROI(exitROI), true
}

// updateTrail 用本根行情的有利极值(多看 High、空看 Low)推进移动止盈：达激活阈值则激活并置峰值，
// 已激活则只在创新高时抬高峰值(单调，永不下调)。TrailActivatePct<=0 时整套关闭。
func (o *Order) updateTrail(q Quote) {
	if o.TrailActivatePct <= 0 {
		return
	}
	var favROI float64
	if o.Direction == Long {
		favROI = o.roiAt(q.High)
	} else {
		favROI = o.roiAt(q.Low)
	}
	if !o.TrailActive {
		if favROI >= o.TrailActivatePct {
			o.TrailActive = true
			o.PeakPnlPct = favROI
		}
		return
	}
	if favROI > o.PeakPnlPct {
		o.PeakPnlPct = favROI
	}
}

// exit 判定 open 持仓是否触及出口。
// 同根 K 线内若止损与止盈同时被触及，保守优先止损(先假设逆行)，避免回测高估收益；实盘逐 tick 无此歧义。
func (o *Order) exit(q Quote) (CloseReason, float64, bool) {
	// 出口优先级(保守)：早段逆行软止损 > 止损 > 移动止盈 > 静态止盈 > 超时。
	// 软止损退出线在入场价与硬止损之间(更近)，逆行时先被触及——武装态下同根 K 线两者都触及时取软止损(减损)。
	// 移动止盈退出线在入场价与峰值之间，比静态止盈更近——同根 K 线两者都触及时取更保守的移动止盈(落袋更少)。
	trailPx, trailOn := o.trailExitPrice(q.Time)
	advPx, advOn := o.earlyAdverseStop()
	if o.Direction == Long {
		if advOn && q.Low <= advPx {
			return ReasonEarlyAdverse, advPx, true
		}
		if q.Low <= o.StopLoss {
			return ReasonSL, o.StopLoss, true
		}
		if trailOn && q.Low <= trailPx {
			return ReasonTrail, trailPx, true
		}
		if q.High >= o.TakeProfit {
			return ReasonTP, o.TakeProfit, true
		}
	} else {
		if advOn && q.High >= advPx {
			return ReasonEarlyAdverse, advPx, true
		}
		if q.High >= o.StopLoss {
			return ReasonSL, o.StopLoss, true
		}
		if trailOn && q.High >= trailPx {
			return ReasonTrail, trailPx, true
		}
		if q.Low <= o.TakeProfit {
			return ReasonTP, o.TakeProfit, true
		}
	}
	// 早段疲软离场：到达 X% 时间点、前段峰值浮盈仍未达 Y% → 按当时价市价平仓。放在超时之前(它是更早的时间条件)。
	if o.earlyCutTriggered(q) {
		return ReasonEarlyCut, q.Price, true
	}
	if !q.Time.Before(o.HoldUntil) {
		return ReasonTimeout, q.Price, true
	}
	return "", 0, false
}

// earlyCutTriggered 判定是否触发早段疲软离场：持仓已过交易周期的 EarlyCutTimePct%，
// 且至此的峰值浮盈 ROI%(含杠杆)仍 < EarlyCutMinProfitPct。
// 因峰值单调不降，一旦达标就不会再触发；未达标则在跨过时间点的第一根行情上离场。
func (o *Order) earlyCutTriggered(q Quote) bool {
	if o.EarlyCutTimePct <= 0 || o.HoldDuration <= 0 || o.OpenedAt.IsZero() {
		return false
	}
	frac := q.Time.Sub(o.OpenedAt).Seconds() / o.HoldDuration.Seconds()
	if frac*100 < o.EarlyCutTimePct {
		return false // 还没到 X% 时间点
	}
	return o.peakFavROI() < o.EarlyCutMinProfitPct
}

// peakFavROI 持仓至今的峰值有利浮盈 ROI%(含杠杆)：多看持仓最高价 MaxPrice、空看最低价 MinPrice。
// 峰值单调不降(极值只扩不缩)，故此值随持仓推进只增不减。
func (o *Order) peakFavROI() float64 {
	if o.Direction == Long {
		return o.roiAt(o.MaxPrice)
	}
	return o.roiAt(o.MinPrice)
}

// earlyAdverseStop 返回早段逆行软止损价及是否生效(武装态)。
// 武装条件：EarlyCutMaxAdversePct>0，且本笔尚未走出有意义浮盈——峰值有利 ROI% < EarlyCutArmProfitPct。
// 一旦峰值浮盈达到 arm 阈值(这笔"工作过")即永久解除(峰值单调不降)，把出场交回硬止损/移动止盈，保住扛单空间。
// EarlyCutArmProfitPct<=0 时不设解除闸门，只要启用就始终武装。软止损价 = 逆行 EarlyCutMaxAdversePct 处的价位。
func (o *Order) earlyAdverseStop() (float64, bool) {
	if o.EarlyCutMaxAdversePct <= 0 {
		return 0, false
	}
	if o.EarlyCutArmProfitPct > 0 && o.peakFavROI() >= o.EarlyCutArmProfitPct {
		return 0, false
	}
	return o.priceAtROI(-o.EarlyCutMaxAdversePct), true
}

// touched 判定限价是否被触及：long 在下方挂单(价格跌到 entry 即成交)，short 在上方挂单。
func touched(dir Direction, limit float64, q Quote) bool {
	if dir == Long {
		return q.Low <= limit
	}
	return q.High >= limit
}

// routeMode 决定入场方式：配了效率阈值则按趋势效率分流(干净趋势→市价顺势，震荡→回踩)，否则用固定模式。
func routeMode(s Params, efficiency float64) EntryMode {
	if s.EfficiencyRoute > 0 {
		if efficiency >= s.EfficiencyRoute {
			return EntryMarket
		}
		return EntryPullback
	}
	if s.EntryMode == "" {
		return EntryMarket
	}
	return s.EntryMode
}

// entryPrice 推导入场价：市价取基准价；回踩按区间分位 α 给限价。区间无效时退回市价基准。
func entryPrice(dir Direction, mode EntryMode, p Prediction, alpha float64) float64 {
	if mode == EntryMarket {
		return p.RefPrice
	}
	w := p.High - p.Low
	if w <= 0 {
		return p.RefPrice
	}
	if dir == Long {
		return p.Low + alpha*w // 回踩到下沿附近买
	}
	return p.High - alpha*w // 反弹到上沿附近卖
}

// takeProfit 按来源推导止盈价，缺数据时回退：pressure/percent → 预测区间γ。
// 最后统一套用兜底锁盈%。返回 0=无法定价(拒单)。
func takeProfit(dir Direction, p Prediction, s Params, entry float64) float64 {
	var tp float64
	switch s.TakeProfitSource {
	case SourcePressure:
		tp = pressureTakeProfit(dir, p, s, entry)
	case SourcePercent:
		tp = percentTakeProfit(dir, s, entry)
	}
	if tp <= 0 {
		// SourcePredict 或上面来源缺数据 → 预测区间分位 γ(不贪满，离对沿留 γ 比例)。
		tp = bandTakeProfit(dir, p, s)
	}
	return floorTakeProfit(dir, entry, tp, s.TakeProfitFloorPct, s.Leverage)
}

// floorTakeProfit 兜底锁盈：盈利%(含杠杆)达 floorPct 即提前锁盈，不死等更远的 AI 目标回吐。
// floorPct 是杠杆后收益率，换算成价格幅度 = floorPct/杠杆。目标比该幅度更近时不动；floorPct<=0 不约束。
func floorTakeProfit(dir Direction, entry, tp, floorPct, leverage float64) float64 {
	if floorPct <= 0 || tp <= 0 || entry <= 0 {
		return tp
	}
	if leverage <= 0 {
		leverage = 1
	}
	floor := floorPct / 100 / leverage // 含杠杆%→价格幅度
	if dir == Long {
		if lock := entry * (1 + floor); tp > lock { // 止盈目标更远 → 提前到兜底%
			return lock
		}
		return tp
	}
	if lock := entry * (1 - floor); tp < lock { // 空头同理 → 上移到兜底%
		return lock
	}
	return tp
}

// stopLoss 按来源推导止损价，缺数据时回退：pressure/percent → 失效价(带缓冲) → 区间外沿。
// 最后统一套用兜底最小止损%。返回 0=拒单。
func stopLoss(dir Direction, p Prediction, s Params, entry float64) float64 {
	var sl float64
	switch s.StopLossSource {
	case SourcePressure:
		sl = pressureStopLoss(dir, p, s, entry)
	case SourcePercent:
		sl = percentStopLoss(dir, s, entry)
	}
	if sl <= 0 {
		// SourcePredict 或上面来源缺数据 → 失效价(可带缓冲)，再退回区间外沿。
		sl = predictStopLoss(dir, p, s)
	}
	return floorStopLoss(dir, entry, sl, s.StopLossFloorPct, s.Leverage)
}

// floorStopLoss 兜底最小止损：亏损%(含杠杆)若低于 floorPct，放宽到正好 floorPct 处——
// 只有亏损达到兜底% 才允许止损，过滤噪音级微亏扫损。floorPct 同为杠杆后口径：价格幅度=floorPct/杠杆。
// floorPct<=0 时不约束。
func floorStopLoss(dir Direction, entry, sl, floorPct, leverage float64) float64 {
	if floorPct <= 0 || sl <= 0 || entry <= 0 {
		return sl
	}
	if leverage <= 0 {
		leverage = 1
	}
	floor := floorPct / 100 / leverage // 含杠杆%→价格幅度
	if dir == Long {
		if minSL := entry * (1 - floor); sl > minSL { // 止损离入场不足 floor% → 放宽下移
			return minSL
		}
		return sl
	}
	if maxSL := entry * (1 + floor); sl < maxSL { // 空头同理 → 放宽上移
		return maxSL
	}
	return sl
}

// ─ percent 来源：离入场价固定百分比 ─

func percentTakeProfit(dir Direction, s Params, entry float64) float64 {
	if s.TakeProfitPct <= 0 {
		return 0
	}
	if dir == Long {
		return entry * (1 + s.TakeProfitPct/100)
	}
	return entry * (1 - s.TakeProfitPct/100)
}

func percentStopLoss(dir Direction, s Params, entry float64) float64 {
	if s.StopLossPct <= 0 {
		return 0
	}
	if dir == Long {
		return entry * (1 - s.StopLossPct/100)
	}
	return entry * (1 + s.StopLossPct/100)
}

// ─ predict 来源：跟 AI 预测(止盈区间分位γ、止损失效价带缓冲) ─

// tpBandPred 给了交易周期区间覆盖时，返回换掉 High/Low/Invalidation 的副本供止盈止损用；
// 入场/门槛仍用原预测，故只替换区间相关字段(压力面/置信度等保持不变)。无覆盖则原样返回。
func tpBandPred(p Prediction) Prediction {
	if p.TPBandHigh <= 0 || p.TPBandLow <= 0 {
		return p
	}
	p.High = p.TPBandHigh
	p.Low = p.TPBandLow
	if p.TPInvalidation > 0 {
		p.Invalidation = p.TPInvalidation
	}
	return p
}

// bandTakeProfit 预测区间分位止盈：多→上沿-γ×宽、空→下沿+γ×宽；区间无效返回 0。
func bandTakeProfit(dir Direction, p Prediction, s Params) float64 {
	w := p.High - p.Low
	if w <= 0 {
		return 0
	}
	if dir == Long {
		return p.High - s.Gamma*w
	}
	return p.Low + s.Gamma*w
}

// predictStopLoss 失效价止损：有失效价则突破其 buffer% 后止损(0=贴着失效价)，否则退回区间外沿。
func predictStopLoss(dir Direction, p Prediction, s Params) float64 {
	buf := s.PredictSLBufferPct / 100
	if buf < 0 {
		buf = 0
	}
	if p.Invalidation > 0 {
		if dir == Long {
			return p.Invalidation * (1 - buf)
		}
		return p.Invalidation * (1 + buf)
	}
	if dir == Long {
		return p.Low
	}
	return p.High
}

// ─ pressure 来源：跟 AI 压力面结构位 ─

// pressureTakeProfit 压力面止盈：到对侧关键结构位(多→关键阻力、空→关键支撑)，可留缓冲%提前止盈。
// 价位须落在盈利一侧(>0=有效)，否则返回 0 触发回退。
func pressureTakeProfit(dir Direction, p Prediction, s Params, entry float64) float64 {
	buf := s.PressureBufferPct / 100
	if buf < 0 {
		buf = 0
	}
	if dir == Long {
		if p.KeyResistance <= 0 {
			return 0
		}
		tp := p.KeyResistance * (1 - buf) // 提前一点止盈，更易成交
		if tp > entry {
			return tp
		}
		return 0
	}
	if p.KeySupport <= 0 {
		return 0
	}
	tp := p.KeySupport * (1 + buf)
	if tp < entry {
		return tp
	}
	return 0
}

// pressureStopLoss 压力面止损：突破关键压力位 buffer% 后止损。
// 空头在关键阻力上方止损 = KeyResistance×(1+buffer)；多头在关键支撑下方止损 = KeySupport×(1-buffer)。
// buffer 为 0 即贴着结构位；价位须落在亏损一侧(>0=有效)，否则返回 0 触发回退。
// 夹逼：压力面止损若落在预测区间内(还没到预测上/下沿)，以预测边界为准——
// 否则会被 AI 预期内的波动提前扫损。多头不高于预测下沿，空头不低于预测上沿。
func pressureStopLoss(dir Direction, p Prediction, s Params, entry float64) float64 {
	buf := s.PressureBufferPct / 100
	if buf < 0 {
		buf = 0
	}
	if dir == Long {
		if p.KeySupport <= 0 {
			return 0
		}
		sl := p.KeySupport * (1 - buf)
		if p.Low > 0 && sl > p.Low { // 止损落在区间内 → 放宽到预测下沿
			sl = p.Low
		}
		if sl < entry {
			return sl
		}
		return 0
	}
	if p.KeyResistance <= 0 {
		return 0
	}
	sl := p.KeyResistance * (1 + buf)
	if p.High > 0 && sl < p.High { // 止损落在区间内 → 放宽到预测上沿
		sl = p.High
	}
	if sl > entry {
		return sl
	}
	return 0
}

// profitSide 报告止盈价是否落在盈利一侧(多头高于入场、空头低于入场)。
func profitSide(dir Direction, entry, tp float64) bool {
	if dir == Long {
		return tp > entry
	}
	return tp < entry
}

// lossSide 报告止损价是否落在亏损一侧(多头低于入场、空头高于入场)。
func lossSide(dir Direction, entry, sl float64) bool {
	if dir == Long {
		return sl < entry
	}
	return sl > entry
}

// compositeAligned 复合方向是否全部与 dir 一致：要求 higher 非空、且每一项都等于 dir。
// 任一周期缺失(空方向)或相反都视为不通过——保守，宁可忽略也不逆势建仓。
func compositeAligned(dir Direction, higher []Direction) bool {
	if len(higher) == 0 {
		return false
	}
	for _, h := range higher {
		if h != dir {
			return false
		}
	}
	return true
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
