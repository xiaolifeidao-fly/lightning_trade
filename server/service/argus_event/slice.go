package argus_event

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/sirupsen/logrus"

	argusDTO "service/argus_event/dto"
	"service/argus_event/repository"
	tradeRepository "service/trade/repository"
)

// 本文件实现「触发瞬间秒级切片」。
//
// 两条供数路径，必须让调用方看得见走的是哪条：
//
//  1. **signal_slice（r3 采集器）**：argus_single 在双 WS 流上挂秒级环形缓冲，
//     每次触发落一行 + 三条 121 点序列（DC last / DC mark / 币安 last）。
//     逐秒完整、含币安侧，是原型里那条连续曲线的真正数据源。
//     它是 90 天滚动的派生表，超期即删，所以永远要留降级路径。
//
//  2. **strategy_event（降级）**：用事件自带的 (sig_last, sig_mark, gap_bp)
//     秒级观测点供数。它是真实观测、不是插值，但只在"有触发发生的那一秒"
//     存在，窗口内其余秒没有点位，画不出连续曲线。
//
// 返回体用 tickSource / tickComplete / degradedReason 把这件事说清楚，前端据此
// 决定是画连续曲线还是打散点 + "数据待补齐"。
const (
	// TickSourceStrategyEvent 降级来源：只有触发那一秒有点。
	TickSourceStrategyEvent = "strategy_event"
	// TickSourceSignalSlice r3 的逐秒切片表。
	TickSourceSignalSlice = "signal_slice"

	sliceDegradedReason = "该触发没有逐秒切片可用（早于 r3 采集器上线、或已过 90 天滚动保留期），" +
		"当前只有触发时刻的秒级报价观测点；窗口内其余秒没有数据，不要把这些点连成连续曲线。"

	// sliceAnchorLookbackSec 由事件 ts 回找切片锚点的最大回看秒数。
	// 事件比锚点晚多少 = trade.signal.delay_seconds（默认 5 秒）+ 下单往返，
	// 与 signalGroupWindowSec 取同一个 30 秒量级，理由见那里的注释。
	sliceAnchorLookbackSec = 30
	// sliceAnchorGraceSec 容许切片锚点比事件 ts 略晚几秒：两者由不同 goroutine
	// 各自取 time.Now()，同一次触发在秒边界上可能反向差一秒。
	sliceAnchorGraceSec = 2
)

// GetSignalSlice 取一次触发前后 ±windowSeconds 的秒级切片。
func (s *ArgusEventService) GetSignalSlice(eventID uint64, q argusDTO.SliceQueryDTO) (*argusDTO.SignalSliceDTO, error) {
	anchor, err := s.strategyEventRepository.FindEventByID(eventID)
	if err != nil {
		return nil, err
	}
	if anchor == nil {
		return nil, ErrEventNotFound
	}
	window := q.WindowSeconds
	if window <= 0 {
		window = defaultSliceWindowSec
	}
	if window > maxSliceWindowSec {
		window = maxSliceWindowSec
	}
	eventTs, err := parseEventTime(anchor.Ts, false)
	if err != nil {
		return nil, err
	}

	result := argusDTO.SignalSliceDTO{
		SignalID:    anchor.Id,
		Ts:          anchor.Ts,
		InstanceKey: anchor.InstanceKey,
		Instrument:  anchor.Instrument,
	}

	// 逐秒切片优先。取失败只降级、不让整个抽屉打不开——切片是观测增强，
	// 事件本身的信息在 strategy_event 里一直都有。
	stored, err := s.signalSliceRepository.FindNearestAnchor(
		anchor.InstanceKey, anchor.Instrument, anchor.Ts, sliceAnchorLookbackSec, sliceAnchorGraceSec)
	if err != nil {
		logrus.Warnf("[argus-event] 取 signal_slice 失败，降级用 strategy_event 观测点: eventId=%d, %v", eventID, err)
		stored = nil
	}

	// 时间原点：有逐秒切片时用切片锚点（偏离穿越时刻），否则用事件 ts。
	originTs := eventTs
	if stored != nil {
		if ts, err := parseEventTime(stored.Ts, false); err == nil {
			originTs = ts
			result.SliceAnchorTs = stored.Ts
			result.AnchorLagSec = int(eventTs.Sub(ts) / time.Second)
			if half := stored.HalfWindowSec; half > 0 && window > half {
				window = half
				result.DegradedReason = fmt.Sprintf(
					"逐秒切片只采触发瞬间 ±%d 秒，请求的更宽窗口已收窄到 ±%d 秒；要看更长的过程请改用 1m K 线视图。", half, half)
			}
		} else {
			logrus.Warnf("[argus-event] signal_slice 锚点时间无法解析，降级用 strategy_event 观测点: ts=%q", stored.Ts)
			stored = nil
		}
	}
	startAt := originTs.Add(-time.Duration(window) * time.Second)
	endAt := originTs.Add(time.Duration(window) * time.Second)
	result.WindowSeconds = window
	result.Window = argusDTO.WindowDTO{
		Start:    formatEventTime(startAt),
		End:      formatEventTime(endAt),
		Resolved: "explicit",
	}

	// 事件行取的是"两个原点各自窗口的并集"：offsetSec 按 originTs 算，但被点开
	// 的那条事件（在 eventTs）必须一定在结果里，否则抽屉里看不到自己点的那条。
	evStart, evEnd := startAt, endAt
	if eventTs.Before(evStart) {
		evStart = eventTs
	}
	if eventTs.After(evEnd) {
		evEnd = eventTs
	}
	rows, err := s.strategyEventRepository.ListEvents(repository.EventFilter{
		InstanceKeys: []string{anchor.InstanceKey},
		Instruments:  []string{anchor.Instrument},
		Events:       AllEvents(),
		Start:        formatEventTime(evStart),
		End:          formatEventTime(evEnd),
	}, 0, 0, true)
	if err != nil {
		return nil, err
	}

	if stored != nil {
		result.TickSource = TickSourceSignalSlice
		result.TickComplete = true
		result.Window.Resolved = "signal-slice"
		result.DcPoints = stored.DcPoints
		result.BinPoints = stored.BinPoints
		result.Points = buildStoredSlicePoints(stored, originTs, window, eventTs, rows)
	} else {
		result.TickSource = TickSourceStrategyEvent
		result.TickComplete = false
		result.DegradedReason = sliceDegradedReason
		result.Points = buildSlicePoints(rows, originTs)
	}

	// dev_sample 是窗口落盘事件（r3 之后 10 秒一条，历史数据是 1 分钟一条），
	// 左右各外扩一分钟，保证覆盖触发时刻的那条采样一定落在区间内。
	devs, err := s.devSampleRepository.ListSamples(anchor.InstanceKey, anchor.Instrument,
		formatEventTime(startAt.Add(-time.Minute)), formatEventTime(endAt.Add(time.Minute)))
	if err != nil {
		return nil, err
	}
	for _, d := range devs {
		result.DevSamples = append(result.DevSamples, argusDTO.SliceDevSampleDTO{
			Ts: d.Ts, DevTicks: d.DevTicks, DevMaxBp: d.DevMaxBp, DevMeanBp: d.DevMeanBp,
		})
	}

	// 1m K 线作为上下文参照。左右各外扩一根，保证触发时刻那根一定被覆盖。
	platform := tradeRepository.NormalizeKlinePlatform("")
	symbol := anchor.Instrument
	klines, err := s.listKlines(platform, symbol, "1m", startAt.Add(-time.Minute), endAt.Add(time.Minute))
	if err != nil {
		return nil, err
	}
	result.Klines = klines
	result.KlineSource = fmt.Sprintf("%s/%s/1m", platform, symbol)
	return &result, nil
}

// buildStoredSlicePoints 把一行 signal_slice 展开成逐秒点位。
//
// 三条序列按秒对位：序列第 i 个元素对应 startAt(切片) + i 秒。请求窗口可能比
// 切片窗口窄（用户只想看 ±10 秒），窄的部分直接按下标裁；越界的秒给空点位而不是
// 丢掉那一秒——前端按 offsetSec 逐秒对位，长度不齐会让整条曲线错位。
func buildStoredSlicePoints(row *repository.SignalSliceRow, originTs time.Time, window int,
	eventTs time.Time, events []*repository.StrategyEventRow) []argusDTO.SlicePointDTO {
	dcLast := decodeSeries(row.DcLastJson)
	dcMark := decodeSeries(row.DcMarkJson)
	binLast := decodeSeries(row.BinLastJson)

	half := row.HalfWindowSec
	if half <= 0 {
		half = len(dcLast) / 2
	}
	eventsByTs := eventNamesByTs(events)
	eventTsKey := formatEventTime(eventTs)

	out := make([]argusDTO.SlicePointDTO, 0, 2*window+1)
	for offset := -window; offset <= window; offset++ {
		ts := originTs.Add(time.Duration(offset) * time.Second)
		key := formatEventTime(ts)
		point := argusDTO.SlicePointDTO{
			Ts:        key,
			OffsetSec: offset,
			// 被点开的那条事件所在的那一秒才是"触发行"，不是 offset==0：
			// 走切片时原点是偏离穿越时刻，事件晚 AnchorLagSec 秒才落地。
			IsTriggerTs: key == eventTsKey,
			Events:      eventsByTs[key],
		}
		if idx := offset + half; idx >= 0 && idx < len(dcLast) {
			point.DcLast = dcLast[idx]
			if idx < len(dcMark) {
				point.DcMark = dcMark[idx]
			}
			if idx < len(binLast) {
				point.BinLast = binLast[idx]
			}
			point.GapBp = gapBpOf(point.DcLast, point.DcMark)
		}
		out = append(out, point)
	}
	return out
}

// decodeSeries 解析一条逐秒序列。解析失败返回 nil——点位会整段留空并由
// dcPoints/binPoints 与图上的空洞暴露出来，不能拿 0 冒充价格。
func decodeSeries(raw string) []*float64 {
	if raw == "" {
		return nil
	}
	var series []*float64
	if err := json.Unmarshal([]byte(raw), &series); err != nil {
		logrus.Warnf("[argus-event] signal_slice 序列无法解析，该序列留空: %v", err)
		return nil
	}
	return series
}

// gapBpOf 由该秒的 last/mark 现算带符号偏离 bp，公式与 trade.NewSignalQuote
// 及 strategy_event.gap_bp 完全一致（(last-mark)/mark×10000）。
func gapBpOf(last, mark *float64) *float64 {
	if last == nil || mark == nil || *mark <= 0 {
		return nil
	}
	bp := (*last - *mark) / *mark * 10000
	return &bp
}

// eventNamesByTs 把窗口内的事件按秒折叠成事件类型清单。
// 同一秒可能有多个账户的事件（同一次触发），类型全部列出，好让前端打标。
func eventNamesByTs(rows []*repository.StrategyEventRow) map[string][]string {
	out := make(map[string][]string, len(rows))
	for _, row := range rows {
		out[row.Ts] = appendUnique(out[row.Ts], row.Event)
	}
	return out
}

// buildSlicePoints 降级路径：把窗口内的事件行折叠成按秒的观测点。
//
// 同一秒可能有多个账户的事件（同一次触发）——报价三元组相同，只留一份点位，
// 但把该秒发生的事件类型全部列出来，好让前端在切片图上打标。
func buildSlicePoints(rows []*repository.StrategyEventRow, anchorTs time.Time) []argusDTO.SlicePointDTO {
	byTs := make(map[string]*argusDTO.SlicePointDTO, len(rows))
	order := make([]string, 0, len(rows))
	for _, row := range rows {
		point, ok := byTs[row.Ts]
		if !ok {
			ts, err := parseEventTime(row.Ts, false)
			if err != nil {
				continue
			}
			point = &argusDTO.SlicePointDTO{
				Ts:          row.Ts,
				OffsetSec:   int(ts.Sub(anchorTs) / time.Second),
				IsTriggerTs: ts.Equal(anchorTs),
			}
			byTs[row.Ts] = point
			order = append(order, row.Ts)
		}
		if point.DcLast == nil && row.SigLast != nil {
			point.DcLast = row.SigLast
		}
		if point.DcMark == nil && row.SigMark != nil {
			point.DcMark = row.SigMark
		}
		if point.GapBp == nil && row.GapBp != nil {
			point.GapBp = row.GapBp
		}
		point.Events = appendUnique(point.Events, row.Event)
	}
	sort.Strings(order)
	out := make([]argusDTO.SlicePointDTO, 0, len(order))
	for _, ts := range order {
		out = append(out, *byTs[ts])
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, item := range list {
		if item == v {
			return list
		}
	}
	return append(list, v)
}

// listKlines 取一段 K 线并转成本地墙钟串。
//
// K 线的 open_time 由 manager-api 这条连接自己写入（time.UnixMilli），读回来
// 时驱动按同一个 loc 反解，得到的 time.Time 是正确的绝对时刻；再 In(Local)
// 落回本地墙钟，就与事件 ts 的墙钟口径对齐了。查询边界同理传 time.Time。
// 这也是本文件唯一使用 time.Time 与 DB 交互的地方——因为 trade_kline 的读写
// 两侧都是这条连接，不存在 argus_single 那种跨进程 loc 不一致的问题。
func (s *ArgusEventService) listKlines(platform, symbol, interval string, start, end time.Time) ([]argusDTO.SliceKlineDTO, error) {
	rows, err := s.klineRepository.ListBySymbolIntervalTimeRange(platform, symbol, interval, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]argusDTO.SliceKlineDTO, 0, len(rows))
	for _, k := range rows {
		out = append(out, argusDTO.SliceKlineDTO{
			Time:   k.OpenTime.In(time.Local).Format(eventTimeLayout),
			Open:   k.OpenPrice,
			High:   k.HighPrice,
			Low:    k.LowPrice,
			Close:  k.ClosePrice,
			Volume: k.Volume,
		})
	}
	return out, nil
}
