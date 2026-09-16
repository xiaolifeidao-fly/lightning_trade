package signal

import (
	"testing"
	"time"
)

// 行情路由减仓：前一日被判为危险状态时，把本日的仓位上限按系数压低。
//
// 为什么路由的是**入场侧的上限**而不是止损线：趋势条件止损那次的教训是，
// 出场侧 + 在阈值附近抖动的指标 = 成交档位随机（名义 250% 线实际砍在 −299%），
// 而且把本可回归的浮亏砍成了实亏。上限在**承担风险之前**就定了，判断错只少赚。
//
// 为什么必须用【前一日】而不是 DayLabels：DayLabels 用当日 OHLC 给整日打标签，
// 是事后情景标注（bootstrap 用），拿它做入场路由等于偷看当天的收盘和振幅。
// 这一条是本文件最重要的性质，单独钉一个测试。

func dayStart(d int) time.Time {
	return time.Date(2026, 8, int(d), 0, 0, 0, 0, time.UTC)
}

// oneDayBars 造出某一天的 1m K 线：用首尾与高低点凑出想要的 |日收益| 与振幅。
func oneDayBars(day int, open, close, high, low float64) []Bar {
	var out []Bar
	base := dayStart(day)
	for i := 0; i < 3; i++ {
		b := Bar{Ts: base.Add(time.Duration(i) * time.Minute), Open: open, Close: open, High: open, Low: open}
		switch i {
		case 1:
			b.High, b.Low = high, low
		case 2:
			b.Close = close
		}
		out = append(out, b)
	}
	return out
}

// 最重要的一条：D 日的路由标签必须来自 D−1 日，与 D 日自身的 OHLC 无关。
// 如果实现误用了当日数据，把 D 日改成剧烈单边、D−1 保持安静，标签就会变——
// 这个测试就是用来抓这件事的。
func TestPriorDayLabelsDoNotPeekAtToday(t *testing.T) {
	th := RegimeThresholds{TrendAbsRetPct: 1.5, VolRangePct: 2.5}
	// 8/18 安静（收益 0%、振幅 0.2%），8/19 剧烈单边（收益 +5%）
	bars := append(oneDayBars(18, 60000, 60000, 60060, 59940),
		oneDayBars(19, 60000, 63000, 63000, 60000)...)

	got := PriorDayLabels(bars, th)
	if got["2026-08-19"] != RegimeQuiet {
		t.Errorf("8/19 的路由标签应取 8/18（安静），得到 %q——实现偷看了当日 OHLC", got["2026-08-19"])
	}
	// 反向确认：事后标注确实把 8/19 判成单边，两者本就该不同。
	if post := DayLabels(bars, th); post["2026-08-19"] != RegimeTrend {
		t.Fatalf("前置条件不成立：DayLabels 应把 8/19 判为 trend，得到 %q", post["2026-08-19"])
	}
}

// 前一日是单边日 → 本日拿到 trend 标签。
func TestPriorDayLabelsCarriesTrendForward(t *testing.T) {
	th := RegimeThresholds{TrendAbsRetPct: 1.5, VolRangePct: 2.5}
	bars := append(oneDayBars(18, 60000, 63000, 63000, 60000), // 8/18 +5% 单边
		oneDayBars(19, 60000, 60000, 60060, 59940)...) // 8/19 安静
	got := PriorDayLabels(bars, th)
	if got["2026-08-19"] != RegimeTrend {
		t.Errorf("8/19 应继承 8/18 的 trend，得到 %q", got["2026-08-19"])
	}
}

// 窗口第一天没有前一日 → 不给标签（fail-safe 不减仓），与趋势闸 warmup 同口径：
// 拿不到证据就不动别人的仓位。
func TestPriorDayLabelsFirstDayHasNoLabel(t *testing.T) {
	th := DefaultRegimeThresholds()
	bars := oneDayBars(18, 60000, 63000, 63000, 60000)
	got := PriorDayLabels(bars, th)
	if l, ok := got["2026-08-18"]; ok {
		t.Errorf("首日不该有路由标签，得到 %q", l)
	}
}

// 日期不相邻（中间缺整天）→ 不给标签。用 10 天前的状态给今天路由是错的，
// 而 K 线确实会缺天（首次回补 80 天时就丢了连续 10 天）。
func TestPriorDayLabelsRequiresAdjacentDay(t *testing.T) {
	th := DefaultRegimeThresholds()
	bars := append(oneDayBars(18, 60000, 63000, 63000, 60000), // 8/18 单边
		oneDayBars(25, 60000, 60000, 60060, 59940)...) // 8/25，中间缺 6 天
	got := PriorDayLabels(bars, th)
	if l, ok := got["2026-08-25"]; ok {
		t.Errorf("与前一日不相邻时不该给标签，得到 %q", l)
	}
}

// 参数校验：系数必须真的是减仓，标签必须是已知的三个之一。
func TestRegimeScaleValidation(t *testing.T) {
	base := func() Params {
		p := baseParams()
		p.RegimeScaleLabels = "trend"
		p.RegimeScaleFactor = 0.5
		return p
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("trend/0.5 应通过，得到 %v", err)
	}
	p := base()
	p.RegimeScaleFactor = 1.0
	if err := p.Validate(); err == nil {
		t.Error("系数 1.0 不是减仓，必须拒绝")
	}
	p = base()
	p.RegimeScaleFactor = 1.5
	if err := p.Validate(); err == nil {
		t.Error("系数 >1 是加仓，必须拒绝")
	}
	p = base()
	p.RegimeScaleFactor = -0.1
	if err := p.Validate(); err == nil {
		t.Error("负系数必须拒绝")
	}
	p = base()
	p.RegimeScaleLabels = "bull"
	if err := p.Validate(); err == nil {
		t.Error("未知标签必须拒绝，否则拼错字就静默失效")
	}
	// 只给系数不给标签 = 没启用，应通过（视为关闭）
	p = baseParams()
	p.RegimeScaleFactor = 0.5
	if err := p.Validate(); err != nil {
		t.Errorf("只给系数不给标签视为关闭，应通过，得到 %v", err)
	}
}

// 关闭时（标签为空）路由不得改变任何行为。
func TestRegimeScaleDisabledIsNoOp(t *testing.T) {
	th := DefaultRegimeThresholds()
	bars := append(oneDayBars(18, 60000, 63000, 63000, 60000), oneDayBars(19, 60000, 60000, 60060, 59940)...)
	p := baseParams()
	p.RegimeScaleLabels = ""
	p.RegimeScaleFactor = 0.25
	if got := resolveRegimeScale(p, PriorDayLabels(bars, th), dayStart(19)); got != 1 {
		t.Errorf("关闭时系数应为 1，得到 %v", got)
	}
}

// 命中标签 → 返回配置的系数；未命中 → 1。
func TestResolveRegimeScaleHitAndMiss(t *testing.T) {
	th := RegimeThresholds{TrendAbsRetPct: 1.5, VolRangePct: 2.5}
	bars := append(oneDayBars(18, 60000, 63000, 63000, 60000), // 8/18 单边 ⇒ 8/19 命中
		oneDayBars(19, 60000, 60000, 60060, 59940)...) // 8/19 安静 ⇒ 8/20 未命中
	bars = append(bars, oneDayBars(20, 60000, 60000, 60060, 59940)...)
	labels := PriorDayLabels(bars, th)

	p := baseParams()
	p.RegimeScaleLabels = "trend"
	p.RegimeScaleFactor = 0.5
	if got := resolveRegimeScale(p, labels, dayStart(19).Add(10*time.Hour)); got != 0.5 {
		t.Errorf("8/19 前一日是单边，应减仓到 0.5，得到 %v", got)
	}
	if got := resolveRegimeScale(p, labels, dayStart(20).Add(10*time.Hour)); got != 1 {
		t.Errorf("8/20 前一日安静，不应减仓，得到 %v", got)
	}
	// 多标签
	p.RegimeScaleLabels = "trend,quiet"
	if got := resolveRegimeScale(p, labels, dayStart(20).Add(10*time.Hour)); got != 0.5 {
		t.Errorf("标签集含 quiet 时 8/20 应命中，得到 %v", got)
	}
}

// 系数落到仓位上限上：cap 15 × 0.5 → 7（向下取整，宁可小不可大）。
// 0 系数意味着当日完全不开新仓，这是"跳过"档，必须允许。
func TestScaledCapFloorsDown(t *testing.T) {
	cases := []struct {
		cap   int
		scale float64
		want  int
	}{
		{15, 1, 15},
		{15, 0.5, 7},
		{8, 0.5, 4},
		{22, 0.25, 5},
		{15, 0, 0},
		{1, 0.5, 0},
	}
	for _, c := range cases {
		if got := scaledCap(c.cap, c.scale); got != c.want {
			t.Errorf("scaledCap(%d, %v) = %d, 期望 %d", c.cap, c.scale, got, c.want)
		}
	}
}

// 行为测试：纯逻辑测试通过不代表引擎真的用上了。这一条走完整 Replay，
// 证明命中路由的那天持仓确实被压在压低后的上限内。
func TestRegimeScaleCapsPositionInReplay(t *testing.T) {
	th := RegimeThresholds{TrendAbsRetPct: 1.5, VolRangePct: 2.5}
	// 8/18 单边日（+5%）⇒ 8/19 命中 trend 标签。
	bars := oneDayBars(18, 60000, 63000, 63000, 60000)
	// 8/19 价格走平，整天 1 分钟一根，够信号落点。
	base := dayStart(19)
	for i := 0; i < 600; i++ {
		bars = append(bars, Bar{Ts: base.Add(time.Duration(i) * time.Minute),
			Open: 63000, High: 63000, Low: 63000, Close: 63000})
	}
	// 8/19 连开 15 次 long，足以顶到上限 15。
	var sigs []Signal
	for i := 0; i < 15; i++ {
		sigs = append(sigs, Signal{Ts: base.Add(time.Duration(i*10+1) * time.Minute),
			Side: "long", Event: EvOpen, OrderSize: 1})
	}

	run := func(labels string, factor float64) *Result {
		p := baseParams()
		p.CapOverride = 15
		p.RegimeScaleLabels = labels
		p.RegimeScaleFactor = factor
		p.RegimeTrendAbsRetPct, p.RegimeVolRangePct = th.TrendAbsRetPct, th.VolRangePct
		// 关掉移动止盈，避免中途平仓干扰"最大堆积"的观察
		p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9
		res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	off := run("", 0)
	if off.MaxStack != 15 {
		t.Fatalf("关闭路由时应顶到上限 15，实际 %d（前置条件不成立）", off.MaxStack)
	}
	on := run("trend", 0.5)
	if on.MaxStack != 7 {
		t.Errorf("命中 trend 时上限应压到 7（15×0.5 向下取整），实际 %d", on.MaxStack)
	}
	if on.SkipRegime == 0 {
		t.Error("被路由拦下的入场次数应 >0，否则无法证明机制咬住了")
	}
	// 系数 0 = 当日完全不开新仓
	zero := run("trend", 0)
	if zero.MaxStack != 0 {
		t.Errorf("系数 0 应当日不开新仓，实际最大堆积 %d", zero.MaxStack)
	}
}
