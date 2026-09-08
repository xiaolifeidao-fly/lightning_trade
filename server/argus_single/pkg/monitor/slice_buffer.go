package monitor

import (
	"time"

	"argus_single/pkg/marketslice"

	"github.com/sirupsen/logrus"
)

// sliceBufferSeconds 环形缓冲容量（秒）。下界是 121（一个完整切片），这里取
// 240 留一倍余量：右半窗要等 60 秒才落盘，期间新 tick 会继续覆写环上的槽位，
// 容量恰好 121 时最老的那一秒会被 anchor+61 秒的写入顶掉。240 秒 ≈ 12KB/币种，
// 换来"落盘延迟到 2 分钟也不丢左半窗"的裕度。
const sliceBufferSeconds = 240

// sliceFlushTick 到期检查节奏。切片的右半窗由独立时钟推进，不挂在行情 tick 上
// ——行情断流时已经登记的切片也必须能落盘（那正是最该看的一种切片）。
const sliceFlushTick = time.Second

// sliceSlot 一秒的聚合结果。同一秒内多个 tick 取最后一个（秒收盘口径）：
// 秒级切片要回答的是"这一秒行情走到哪"，不是"这一秒成交了几笔"，
// 均值会把瞬时极值抹平，恰好抹掉触发瞬间最该看的东西。
type sliceSlot struct {
	sec     int64
	dcLast  float64
	dcMark  float64
	binLast float64
	hasDc   bool
	hasBin  bool
}

// SliceBuffer 秒级行情环形缓冲：按 epoch 秒取模定位槽位，写入 O(1)、无分配。
//
// 与 DevSampler 同一模式——纯累加器，不持有时间、不持有锁：窗口节奏与并发保护
// 都由调用方掌握，于是全部行为都能在测试里确定性复现。区别只在 DevSampler 是
// 标量累加、这里是按秒覆写的定长环。
type SliceBuffer struct {
	slots []sliceSlot
}

// NewSliceBuffer 按容量（秒）构造。低于一个完整切片的容量会被抬到 121。
func NewSliceBuffer(capacitySeconds int) *SliceBuffer {
	if capacitySeconds < marketslice.SeriesPoints {
		capacitySeconds = marketslice.SeriesPoints
	}
	return &SliceBuffer{slots: make([]sliceSlot, capacitySeconds)}
}

// slot 取写槽位。槽位上是别的秒的旧数据时先整体清零——这就是"环形覆盖"，
// 不需要任何显式淘汰逻辑。
func (b *SliceBuffer) slot(sec int64) *sliceSlot {
	s := &b.slots[b.index(sec)]
	if s.sec != sec {
		*s = sliceSlot{sec: sec}
	}
	return s
}

// at 只读定位。槽位上不是这一秒的数据就返回 nil——绝不像 slot 那样顺手清零，
// 否则一次快照会把缓冲擦掉一半。
func (b *SliceBuffer) at(sec int64) *sliceSlot {
	s := &b.slots[b.index(sec)]
	if s.sec != sec {
		return nil
	}
	return s
}

func (b *SliceBuffer) index(sec int64) int {
	n := int64(len(b.slots))
	return int(((sec % n) + n) % n)
}

// ObserveQuote 记录一个 DeepCoin 报价 tick。last/mark 同源同刻，一起写。
// mark<=0 无法判定偏离，与 DevSampler.Observe 同口径忽略。
func (b *SliceBuffer) ObserveQuote(sec int64, last, mark float64) {
	if last <= 0 || mark <= 0 {
		return
	}
	s := b.slot(sec)
	s.dcLast, s.dcMark, s.hasDc = last, mark, true
}

// ObserveBinance 记录一个币安 last tick。币安流与 DeepCoin 流各自到达，
// 因此两条流分别落槽——只在 DC tick 上顺手读币安缓存的话，DC 断流的那几秒
// 币安侧也会跟着变空，而"一边断流"恰恰是要看清的场景。
func (b *SliceBuffer) ObserveBinance(sec int64, last float64) {
	if last <= 0 {
		return
	}
	s := b.slot(sec)
	s.binLast, s.hasBin = last, true
}

// Snapshot 取 [anchorSec-half, anchorSec+half] 的三条等长序列。
// 该秒无 tick 时元素为 nil（含义见 marketslice.Slice 注释：不插值、不补零）。
func (b *SliceBuffer) Snapshot(anchorSec int64, half int) (dcLast, dcMark, binLast []*float64, dcPoints, binPoints int) {
	n := 2*half + 1
	dcLast = make([]*float64, n)
	dcMark = make([]*float64, n)
	binLast = make([]*float64, n)
	for i := 0; i < n; i++ {
		s := b.at(anchorSec - int64(half) + int64(i))
		if s == nil {
			continue
		}
		if s.hasDc {
			last, mark := s.dcLast, s.dcMark
			dcLast[i], dcMark[i] = &last, &mark
			dcPoints++
		}
		if s.hasBin {
			bin := s.binLast
			binLast[i] = &bin
			binPoints++
		}
	}
	return dcLast, dcMark, binLast, dcPoints, binPoints
}

// slicePending 一次已触发、等右半窗凑满的切片。
type slicePending struct {
	anchorSec int64
	dueAt     time.Time
	instIdRaw string
}

// observeSliceQuote 把一个 DeepCoin 报价 tick 记进秒级缓冲。
//
// sliceMu 是**叶子锁**：本文件是它唯一的获取点，且获取时绝不持有 mu 或
// signalMu，因此不引入任何新的锁序。调用方必须在锁外调用（见 handleOrderBookSignal）。
func (pm *PriceMonitor) observeSliceQuote(now time.Time, symbol string, last, mark float64) {
	pm.sliceMu.Lock()
	defer pm.sliceMu.Unlock()
	if b := pm.sliceBufferLocked(symbol); b != nil {
		b.ObserveQuote(now.Unix(), last, mark)
	}
}

// observeSliceBinance 把一个币安 last tick 记进秒级缓冲。
func (pm *PriceMonitor) observeSliceBinance(now time.Time, symbol string, last float64) {
	pm.sliceMu.Lock()
	defer pm.sliceMu.Unlock()
	if b := pm.sliceBufferLocked(symbol); b != nil {
		b.ObserveBinance(now.Unix(), last)
	}
}

// sliceBufferLocked 取（必要时建）某币种的缓冲。map 为 nil 说明本实例是直接
// 构造出来的（部分单测），此时整条切片链路静默关闭，不影响被测行为。
func (pm *PriceMonitor) sliceBufferLocked(symbol string) *SliceBuffer {
	if pm.sliceBuffers == nil {
		return nil
	}
	b := pm.sliceBuffers[symbol]
	if b == nil {
		b = NewSliceBuffer(sliceBufferSeconds)
		pm.sliceBuffers[symbol] = b
	}
	return b
}

// armSlice 登记一次触发，等 now+半窗 到期后再落盘。
//
// 同一秒重复触发只登记一次：切片按 (instance_key, instrument, ts) 唯一，
// 同秒多个账户各自判定会调进来多次，重复登记只会产出完全相同的行。
func (pm *PriceMonitor) armSlice(now time.Time, symbol string, instIdRaw string) {
	anchorSec := now.Unix()
	pm.sliceMu.Lock()
	defer pm.sliceMu.Unlock()
	if pm.slicePendings == nil {
		return
	}
	for _, p := range pm.slicePendings[symbol] {
		if p.anchorSec == anchorSec {
			return
		}
	}
	pm.slicePendings[symbol] = append(pm.slicePendings[symbol], slicePending{
		anchorSec: anchorSec,
		dueAt:     now.Add(marketslice.HalfWindowSeconds * time.Second),
		instIdRaw: instIdRaw,
	})
}

// takeDueSlices 取出全部已到期的切片并从待办里摘掉。
// 锁内只做内存快照，投递（可能写库）在锁外——与 dev_sample 的落盘时机同规矩。
// force=true 时不看到期时间，全部取出：进程收尾/热替换时把手上的切片交出去，
// 覆盖率字段会如实反映右半窗没凑满。
func (pm *PriceMonitor) takeDueSlices(now time.Time, force bool) []marketslice.Slice {
	pm.sliceMu.Lock()
	defer pm.sliceMu.Unlock()
	if len(pm.slicePendings) == 0 {
		return nil
	}
	var out []marketslice.Slice
	for symbol, pendings := range pm.slicePendings {
		kept := pendings[:0]
		for _, p := range pendings {
			if !force && now.Before(p.dueAt) {
				kept = append(kept, p)
				continue
			}
			buf := pm.sliceBuffers[symbol]
			if buf == nil {
				continue
			}
			dcLast, dcMark, binLast, dcPoints, binPoints := buf.Snapshot(p.anchorSec, marketslice.HalfWindowSeconds)
			anchor := time.Unix(p.anchorSec, 0)
			out = append(out, marketslice.Slice{
				Anchor:     anchor,
				InstIdRaw:  p.instIdRaw,
				StartAt:    anchor.Add(-marketslice.HalfWindowSeconds * time.Second),
				EndAt:      anchor.Add(marketslice.HalfWindowSeconds * time.Second),
				HalfWindow: marketslice.HalfWindowSeconds,
				DcLast:     dcLast,
				DcMark:     dcMark,
				BinLast:    binLast,
				DcPoints:   dcPoints,
				BinPoints:  binPoints,
			})
		}
		if len(kept) == 0 {
			delete(pm.slicePendings, symbol)
			continue
		}
		pm.slicePendings[symbol] = kept
	}
	return out
}

// runSliceFlusher 秒级切片的落盘时钟。
func (pm *PriceMonitor) runSliceFlusher() {
	defer pm.wg.Done()
	ticker := time.NewTicker(sliceFlushTick)
	defer ticker.Stop()
	for {
		select {
		case <-pm.stopChan:
			// 收尾：把手上未到期的切片也交出去，别让热替换吃掉一整批触发。
			if rest := pm.takeDueSlices(time.Now(), true); len(rest) > 0 {
				logrus.Infof("[slice] 收尾投递 %d 条未满窗切片（覆盖率字段会如实反映）", len(rest))
				emitSlices(rest)
			}
			return
		case now := <-ticker.C:
			emitSlices(pm.takeDueSlices(now, false))
		}
	}
}

func emitSlices(list []marketslice.Slice) {
	for _, s := range list {
		marketslice.Emit(s)
	}
}
