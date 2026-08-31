package monitor

import (
	"math"
	"strconv"
	"time"

	"argus_single/pkg/eventlog"
)

// DevSampleThresholdsBp 候选阈值（bp）。下探到 0.5bp ≈ 秒级价格漂移量级（由实测
// rv 标定：141bp/日 ⇒ 5m 8.3bp ⇒ 1s 0.48bp），才能看清物理地板在哪；5bp 是当前
// 生产阈值，它的穿越数应与真实信号数一致，是采样口径的自检锚点；7/10bp 用于观察
// 尾部是否仍在变厚。
var DevSampleThresholdsBp = []float64{0.5, 1, 1.5, 2, 3, 4, 5, 7, 10}

// devSampleInterval dev_sample 落盘间隔。与 balance 事件同频（实测每分钟一条），
// 约 1440 条/天；离线可把窗口计数求和成任意粒度。
const devSampleInterval = time.Minute

// DevSampler 无条件偏离采样器：在每个 ticker tick 上累加 DeepCoin last-vs-mark
// 偏离，按窗口 flush 成一条 dev_sample 事件。
//
// 存在理由：P7 的 gapBp 只在信号触发时落盘，被 signal_threshold=5bp 结构性截断
// （实测 360 样本 min=5.01bp），因此看不到分布主体，无法回答"阈值降到 θ 后 λ
// 会是多少"——而实测显示信号频率正以每周 −16%~−22% 衰减（详见
// docs/2026-08-08-信号源衰减与阈值自适应设计.md）。
//
// 关键设计：对每个候选阈值各自维护一份 edge-trigger 状态，精确镜像
// handleOrderBookSignal 的判定链，于是各阈值的穿越次数就是"若阈值取该值，λ 会是
// 多少"的直接测量。λ 取决于穿越次数而非幅度分位数——同一幅度分布，成簇与分散的
// 时间结构会给出完全不同的 λ，所以只能实跑规则、不能由分位数推算。
//
// 不持有时间：窗口节奏由调用方掌握（用 Ticks() 判断有无样本）。这样采样器是纯
// 累加器，全部行为可在测试里确定性复现。并发保护同样由调用方负责——接线处已在
// signalMu 临界区内，不引入新锁序。
type DevSampler struct {
	ratios []float64 // 候选阈值（比例，= bp/10000），与 EvaluateDeviationSignal 同单位
	keys   []string  // 预格式化的 map 键（bp），避免每次 flush 重复格式化
	// lastDerived 每个阈值各自的 edge-trigger 状态，语义与 PriceMonitor 的
	// lastDerivedSignal 完全一致（""=未派发或已回带内）。
	lastDerived []SignalDirection
	cross       []int // 穿越次数 → λ(θ)
	over        []int // 超阈 tick 数 → P(|dev|>θ)
	ticks       int
	sumAbs      float64
	maxAbs      float64
}

// NewDevSampler 按候选阈值（单位 bp）构造采样器。
func NewDevSampler(thresholdsBp []float64) *DevSampler {
	keys := make([]string, len(thresholdsBp))
	ratios := make([]float64, len(thresholdsBp))
	for i, th := range thresholdsBp {
		keys[i] = strconv.FormatFloat(th, 'g', -1, 64)
		ratios[i] = th / 10000
	}
	return &DevSampler{
		ratios:      ratios,
		keys:        keys,
		lastDerived: make([]SignalDirection, len(thresholdsBp)),
		cross:       make([]int, len(thresholdsBp)),
		over:        make([]int, len(thresholdsBp)),
	}
}

// Observe 记录一个 tick。mark<=0 无法计算偏离，直接忽略（与 NewSignalQuote 同口径）。
func (s *DevSampler) Observe(last, mark float64) {
	if mark <= 0 {
		return
	}
	deviation := (last - mark) / mark
	abs := math.Abs(deviation)
	s.ticks++
	s.sumAbs += abs * 10000
	if bp := abs * 10000; bp > s.maxAbs {
		s.maxAbs = bp
	}
	for i, ratio := range s.ratios {
		// 穿越判定走生产同一函数，口径不可能漂移。
		next, fire := EvaluateDeviationSignal(deviation, ratio, s.lastDerived[i])
		s.lastDerived[i] = next
		if fire {
			s.cross[i]++
		}
		// 超阈计数是"分布"而非"频率"，纯函数不区分带内与同向抑制，故这里直接
		// 比较。候选阈值恒为显式正值，不会触发纯函数里的默认值回落，两者一致。
		if abs >= ratio {
			s.over[i]++
		}
	}
}

// Ticks 当前窗口内的有效 tick 数（调用方据此判断是否值得 flush）。
func (s *DevSampler) Ticks() int { return s.ticks }

// Flush 产出一条 dev_sample 事件并清零窗口。无有效 tick 时返回 ok=false——
// 不写空事件，否则日志里会出现无法与"真零穿越"区分的噪声行。
//
// 注意 lastDerived 跨窗口保留：edge-trigger 状态属于行情的连续过程，
// 若随窗口清零，跨窗口边界的同向持续偏离会被重复计成新穿越。
func (s *DevSampler) Flush() (eventlog.Event, bool) {
	if s.ticks == 0 {
		return eventlog.Event{}, false
	}
	ev := eventlog.Event{
		Event:     eventlog.EvDevSample,
		DevTicks:  s.ticks,
		DevMaxBp:  s.maxAbs,
		DevMeanBp: s.sumAbs / float64(s.ticks),
	}
	cross := make(map[string]int)
	over := make(map[string]int)
	for i := range s.ratios {
		if s.cross[i] > 0 {
			cross[s.keys[i]] = s.cross[i]
		}
		if s.over[i] > 0 {
			over[s.keys[i]] = s.over[i]
		}
		s.cross[i], s.over[i] = 0, 0
	}
	if len(cross) > 0 {
		ev.DevCross = cross
	}
	if len(over) > 0 {
		ev.DevOver = over
	}
	s.ticks, s.sumAbs, s.maxAbs = 0, 0, 0
	return ev, true
}

// observeDeviationLocked 记录一个 tick 的偏离，并在窗口到期时产出 dev_sample 事件。
//
// 调用方须持有 signalMu（本方法读写 devSamplers/lastDevFlush）；产出的事件由调用方
// 在锁外落盘——锁内不做 I/O。返回 ok=false 表示本次无需落盘。
func (pm *PriceMonitor) observeDeviationLocked(now time.Time, symbol string, config SymbolConfig, last, mark float64) (eventlog.Event, bool) {
	s := pm.devSamplers[symbol]
	if s == nil {
		s = NewDevSampler(DevSampleThresholdsBp)
		pm.devSamplers[symbol] = s
	}
	s.Observe(last, mark)

	prev := pm.lastDevFlush[symbol]
	if prev.IsZero() {
		pm.lastDevFlush[symbol] = now // 首个 tick 只起窗口，不落单样本事件
		return eventlog.Event{}, false
	}
	if now.Sub(prev) < devSampleInterval {
		return eventlog.Event{}, false
	}
	ev, ok := s.Flush()
	if !ok {
		return eventlog.Event{}, false // 窗口内全是 mark<=0 的坏 tick，不推进窗口
	}
	pm.lastDevFlush[symbol] = now
	ev.InstId = config.TradeInst // 与信号侧事件同口径（BTCUSDT），避免 instId 分裂
	return ev, true
}
