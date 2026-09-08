package signal

import (
	"sort"
	"time"
)

// 日级行情状态标签。口径逐字取自 backtest_capsf_study.py 的 label_days：
// 先看日收益绝对值，再看日内振幅，都不到就是安静日。
//
// 注意 trend 在本研究里被当作"熊市月"的代理：均值回归累加器怕的是**单边行情**，
// 不分涨跌（8/21 那次剧烈上涨同样打爆了它），所以判据是 |日收益| 而不是负收益。
// 这一点必须随结果一起说明，否则"熊市月 p10"会被读成"下跌月 p10"。
const (
	RegimeTrend = "trend" // 单边日：|日收益| ≥ TrendAbsRetPct
	RegimeVol   = "vol"   // 震荡日：日内振幅 ≥ VolRangePct
	RegimeQuiet = "quiet" // 安静日
)

// 情景名。频率预算按熊市月情景验收（设计文档 §3.2），另外两个只作参照。
const (
	ScenarioBear  = "bear"  // 熊市月：单边日块
	ScenarioChop  = "chop"  // 震荡月：震荡日块
	ScenarioMixed = "mixed" // 混合月：全部块
)

// 情景 bootstrap 的两种口径。
const (
	// BootstrapDayIID 日级 iid 重抽：金标准 backtest_capsf_study.py 的实际口径，
	// study_fine.csv 就是它跑出来的，校准比对必须用它。
	// 已知偏差（设计文档 §10.3）：把稀有的兜底损失日按 iid 重抽会**高估尾部**，
	// 对低 λ 配置系统性偏悲观。
	BootstrapDayIID = "day_iid"
	// BootstrapEpisodeBlock 按持仓生命周期切块的分块重抽：设计文档 §3.2 原本
	// 要求的口径（"按仓位生命周期切块，避免切断持仓"）。块内保留损失日与其
	// 前后的扛单日的相关性，因此不像日级 iid 那样高估尾部。
	BootstrapEpisodeBlock = "episode_block"
)

// RegimeThresholds 日标签判据。缺省值即金标准脚本的写死值。
type RegimeThresholds struct {
	TrendAbsRetPct float64 `json:"trendAbsRetPct"` // |收盘−开盘|/开盘，百分数
	VolRangePct    float64 `json:"volRangePct"`    // (最高−最低)/开盘，百分数
}

// DefaultRegimeThresholds 金标准口径：1.5% / 2.5%。
func DefaultRegimeThresholds() RegimeThresholds {
	return RegimeThresholds{TrendAbsRetPct: 1.5, VolRangePct: 2.5}
}

func (t RegimeThresholds) normalize() RegimeThresholds {
	d := DefaultRegimeThresholds()
	if t.TrendAbsRetPct <= 0 {
		t.TrendAbsRetPct = d.TrendAbsRetPct
	}
	if t.VolRangePct <= 0 {
		t.VolRangePct = d.VolRangePct
	}
	return t
}

// DayLabels 按 1m K 线给每个自然日打状态标签，key 为本地日期 "2006-01-02"。
//
// 标签在**整个扫描窗口**上算一次，全部路径共用：起点偏移路径只是少看几天，
// 不该让"6/09 是不是单边日"这件事随路径变化。
func DayLabels(bars []Bar, th RegimeThresholds) map[string]string {
	th = th.normalize()
	type agg struct {
		open, close, high, low float64
		first, last            time.Time
	}
	byDay := map[string]*agg{}
	for _, b := range SortBars(bars) {
		day := b.Ts.Format("2006-01-02")
		a := byDay[day]
		if a == nil {
			a = &agg{open: b.Open, close: b.Close, high: b.High, low: b.Low, first: b.Ts, last: b.Ts}
			byDay[day] = a
			continue
		}
		if b.Ts.Before(a.first) {
			a.first, a.open = b.Ts, b.Open
		}
		if !b.Ts.Before(a.last) {
			a.last, a.close = b.Ts, b.Close
		}
		if b.High > a.high {
			a.high = b.High
		}
		if b.Low < a.low {
			a.low = b.Low
		}
	}
	out := make(map[string]string, len(byDay))
	for day, a := range byDay {
		if a.open <= 0 {
			out[day] = RegimeQuiet
			continue
		}
		ret := (a.close - a.open) / a.open * 100
		if ret < 0 {
			ret = -ret
		}
		rng := (a.high - a.low) / a.open * 100
		switch {
		case ret >= th.TrendAbsRetPct:
			out[day] = RegimeTrend
		case rng >= th.VolRangePct:
			out[day] = RegimeVol
		default:
			out[day] = RegimeQuiet
		}
	}
	return out
}

// CountLabel 窗口内某个标签的天数（λ 的分母、情景说明都要用）。
func CountLabel(labels map[string]string, label string) int {
	n := 0
	for _, v := range labels {
		if v == label {
			n++
		}
	}
	return n
}

// LabeledDays 某标签的日期列表（升序），供结果里说明"哪几天算单边日"。
func LabeledDays(labels map[string]string, label string) []string {
	out := make([]string, 0, 8)
	for d, v := range labels {
		if v == label {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}

// MtmBlock 一段**不切断持仓**的连续日块：情景 bootstrap 的重采样单元。
type MtmBlock struct {
	Days       []string `json:"days"`
	Delta      float64  `json:"delta"` // 块内 ΔMTM 合计
	TrendDays  int      `json:"trendDays"`
	VolDays    int      `json:"volDays"`
	QuietDays  int      `json:"quietDays"`
	Label      string   `json:"label"`      // 块的主导状态
	CutAtStart bool     `json:"cutAtStart"` // 块首是不是一个真实的空仓切点
}

// BuildMtmBlocks 把一条路径的日 ΔMTM 序列按"空仓时刻"切成块。
//
// 切点判据：日界（次日 00:00 本地）不落在任何一个持仓生命周期区间内部。
// 这正是设计文档 §3.2 "episode 对齐，避免切断持仓"的字面实现——把一段扛单
// 从中间切开，会让重采样把"深浮亏"和"回本"拆到两个不同的月里，凭空造出
// 现实中不存在的尾部。
//
// 未平仓（Open=true）的生命周期按"一直持有到序列末尾"处理：它确实没平。
// loc 是日界所处的时区：必须与 K 线/事件时间戳同源，否则"次日 00:00"会算错
// 8 小时，切点全错位（本仓库的时间锚是本地墙钟，见 parseSignalWindowTime）。
func BuildMtmBlocks(daily []DayMTM, episodes []Episode, labels map[string]string, loc *time.Location) []MtmBlock {
	if len(daily) == 0 {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	busy := mergeHoldingIntervals(episodes)
	blocks := make([]MtmBlock, 0, len(daily))
	cur := MtmBlock{CutAtStart: true}
	flush := func() {
		if len(cur.Days) == 0 {
			return
		}
		cur.Label = dominantLabel(cur)
		blocks = append(blocks, cur)
		cur = MtmBlock{CutAtStart: true}
	}
	for i, d := range daily {
		cur.Days = append(cur.Days, d.Day)
		cur.Delta += d.Delta
		switch labels[d.Day] {
		case RegimeTrend:
			cur.TrendDays++
		case RegimeVol:
			cur.VolDays++
		default:
			cur.QuietDays++
		}
		if i == len(daily)-1 {
			break // 末块无论是否空仓都收尾
		}
		if boundary, ok := dayBoundaryAfter(d.Day, loc); ok && !insideAny(busy, boundary) {
			flush()
		}
	}
	flush()
	return blocks
}

// dayBoundaryAfter 该自然日结束的瞬时（次日 00:00，按传入时区解析）。
func dayBoundaryAfter(day string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t.AddDate(0, 0, 1), true
}

type interval struct{ start, end time.Time }

// mergeHoldingIntervals 把全部持仓生命周期合并成互不重叠的"有仓区间"。
// 净仓模式下同一时刻只有一本仓，但双向模式两侧会重叠，必须先合并。
func mergeHoldingIntervals(episodes []Episode) []interval {
	raw := make([]interval, 0, len(episodes))
	for _, ep := range episodes {
		if ep.OpenedAt.IsZero() {
			continue
		}
		end := ep.ClosedAt
		if ep.Open || end.IsZero() {
			// 未平仓：视为持有到时间轴尽头，任何日界都不再是切点。
			end = time.Unix(1<<62, 0)
		}
		if !end.After(ep.OpenedAt) {
			continue
		}
		raw = append(raw, interval{ep.OpenedAt, end})
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].start.Before(raw[j].start) })
	out := make([]interval, 0, len(raw))
	for _, iv := range raw {
		if n := len(out); n > 0 && !iv.start.After(out[n-1].end) {
			if iv.end.After(out[n-1].end) {
				out[n-1].end = iv.end
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// insideAny 该时刻是否落在某个有仓区间**内部**（端点算空仓：刚平完就是切点）。
func insideAny(ivs []interval, t time.Time) bool {
	for _, iv := range ivs {
		if iv.start.Before(t) && t.Before(iv.end) {
			return true
		}
	}
	return false
}

func dominantLabel(b MtmBlock) string {
	// 平票按 trend > vol > quiet 取：块里只要有一半是单边日，就按最坏的算。
	if b.TrendDays >= b.VolDays && b.TrendDays >= b.QuietDays && b.TrendDays > 0 {
		return RegimeTrend
	}
	if b.VolDays >= b.QuietDays && b.VolDays > 0 {
		return RegimeVol
	}
	return RegimeQuiet
}

// ScenarioResult 一个情景（熊市月/震荡月/混合月）的 bootstrap 分位。
type ScenarioResult struct {
	Scenario string  `json:"scenario"`
	Mode     string  `json:"mode"`     // day_iid / episode_block
	PoolSize int     `json:"poolSize"` // 重采样池大小（日数或块数）
	Draws    int     `json:"draws"`
	P10      float64 `json:"p10"`
	P50      float64 `json:"p50"`
	P90      float64 `json:"p90"`
	// Note 池太小或为空时的说明；空池不给分位，也不假装给 0。
	Note string `json:"note,omitempty"`
}

// bootstrapDayIID 日级 iid 重抽：每次抽 scenarioDays 天求和，重复 draws 次。
// 逐行对齐金标准脚本（含 random.Random(seed) 与 sums[draws/10] / sums[draws/2]
// 的取分位方式），这样 study_fine.csv 才有可比性。
func bootstrapDayIID(pool []float64, scenarioDays, draws int, seed int64) (p10, p50, p90 float64, ok bool) {
	if len(pool) == 0 || scenarioDays <= 0 || draws <= 0 {
		return 0, 0, 0, false
	}
	rng := newPyRandom(seed)
	sums := make([]float64, 0, draws)
	for i := 0; i < draws; i++ {
		s := 0.0
		for j := 0; j < scenarioDays; j++ {
			s += pool[rng.ChoiceIndex(len(pool))]
		}
		sums = append(sums, s)
	}
	sort.Float64s(sums)
	return sums[draws/10], sums[draws/2], sums[draws*9/10], true
}

// bootstrapBlocks 分块重抽：按块抽到累计天数 ≥ scenarioDays，再把合计归一到
// scenarioDays 天（块长不整除，必须归一，否则长块占优的情景会系统性偏大）。
func bootstrapBlocks(pool []MtmBlock, scenarioDays, draws int, seed int64) (p10, p50, p90 float64, ok bool) {
	if len(pool) == 0 || scenarioDays <= 0 || draws <= 0 {
		return 0, 0, 0, false
	}
	rng := newPyRandom(seed)
	sums := make([]float64, 0, draws)
	for i := 0; i < draws; i++ {
		total, days := 0.0, 0
		for days < scenarioDays {
			b := pool[rng.ChoiceIndex(len(pool))]
			n := len(b.Days)
			if n == 0 {
				n = 1
			}
			total += b.Delta
			days += n
		}
		sums = append(sums, total*float64(scenarioDays)/float64(days))
	}
	sort.Float64s(sums)
	return sums[draws/10], sums[draws/2], sums[draws*9/10], true
}
