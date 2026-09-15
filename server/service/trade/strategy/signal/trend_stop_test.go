package signal

import (
	"math"
	"testing"
	"time"
)

// 趋势条件止损在回测侧的口径必须与实盘完全一致（实盘见
// argus_single/pkg/trade/trend_stop.go 的 ResolveTrendStop）：逆向窗口动量
// ≥ TrendStopTriggerPct 时，本次判定的兜底线改用 TrendStopPct，其余时间沿用 S。
//
// 没有这一块，80 天窗口的扫参就只能扫"不带该机制"的参数集——而这个机制恰好是
// 为那 16 笔兜底止损做的，扫不到它等于回答不了"它到底省了多少"。
//
// 窗口复用趋势闸的 TrendGateWindowHours：实盘也是共用
// trade.trend_gate.window_hours（见 logStaticRiskParams 打的那行）。

// 逆势下跌里的多仓：兜底线应从 400% 收到 250%，在 -250% 就平掉，
// 而不是一路走到 -400%。
func TestTrendStopTightensCatastropheInAdverseTrend(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.TrendGateWindowHours = 2
	p.TrendStopTriggerPct = 3
	p.TrendStopPct = 250
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9

	bars := adverseDownBars()
	// 第 121 根开多；第 122 根探到 58500：
	//   -250% 线 = 60000*(1-250/12500) = 58800  → 穿越
	//   -400% 线 = 60000*(1-400/12500) = 58080  → 未穿越
	// 所以"收紧生效"与"未生效"在结果上完全可分。
	sigs := []Signal{{Ts: ts(121).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Episodes) != 1 || res.Episodes[0].Reason != ExitCatastrophe {
		t.Fatalf("期望一条 catastrophe，实际 %+v", res.Episodes)
	}
	if got := res.Episodes[0].RoiPct; math.Abs(got+250) > 1e-6 {
		t.Errorf("兜底 ROI = %.4f，期望 -250（收紧后的线），得到 -400 说明没生效", got)
	}
}

// 同样的价格路径，只是把机制关掉：应该**不平仓**（58500 没到 -400% 线）。
// 这一条是上一条的对照组——没有它，上面那条无法排除"本来就会平"的可能。
func TestTrendStopDisabledRidesToBaseLine(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.TrendGateWindowHours = 2
	p.TrendStopTriggerPct = 0 // 关闭
	p.TrendStopPct = 0
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9

	sigs := []Signal{{Ts: ts(121).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: adverseDownBars()})
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range res.Episodes {
		if ep.Reason == ExitCatastrophe {
			t.Fatalf("机制关闭时 58500 不该触发兜底（-400%% 线在 58080），实际 %+v", ep)
		}
	}
}

// 顺势（价格一路上涨的多仓）不该被收紧：趋势在帮它。
func TestTrendStopKeepsBaseLineWithTrend(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.TrendGateWindowHours = 2
	p.TrendStopTriggerPct = 3
	p.TrendStopPct = 250
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9

	// 前 121 根从 58000 涨到 60000（+3.4%，对多仓是顺势），第 122 根探到 58500。
	var bars []Bar
	for i := 0; i <= 121; i++ {
		px := 58000.0 + float64(i)*(2000.0/121.0)
		bars = append(bars, Bar{Ts: ts(i), Open: px, High: px, Low: px, Close: px})
	}
	bars = append(bars, Bar{Ts: ts(122), Open: 60000, High: 60000, Low: 58500, Close: 60000})
	sigs := []Signal{{Ts: ts(121).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: bars})
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range res.Episodes {
		if ep.Reason == ExitCatastrophe {
			t.Fatalf("顺势不该收紧，实际 %+v", ep)
		}
	}
}

// warmup 期（窗口未满、动量不可算）必须 fail-safe 沿用 S：
// 拿不到证据就不动别人的兜底线，与实盘 ResolveTrendStop 的 momOK=false 分支一致。
func TestTrendStopFailsSafeDuringWarmup(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.TrendGateWindowHours = 24 // 窗口远大于 bar 数 ⇒ 全程 warmup
	p.TrendStopTriggerPct = 3
	p.TrendStopPct = 250
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9

	sigs := []Signal{{Ts: ts(121).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: adverseDownBars()})
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range res.Episodes {
		if ep.Reason == ExitCatastrophe {
			t.Fatalf("warmup 期不得收紧，实际 %+v", ep)
		}
	}
}

// 机制启用时趋势闸可以是关的：两者必须能独立研究，否则扫不出"只加止损"的效应。
func TestTrendStopWorksWithTrendGateOff(t *testing.T) {
	p := baseParams()
	p.CatastropheStopPct = 400
	p.TrendGateWindowHours = 2
	p.TrendGateThresholdPct = 0 // 闸关着
	p.TrendStopTriggerPct = 3
	p.TrendStopPct = 250
	p.SmallActivatePct, p.MediumActivatePct, p.LargeActivatePct = 1e9, 1e9, 1e9

	sigs := []Signal{{Ts: ts(121).Add(time.Second), Side: "long", Event: EvOpen, OrderSize: 1}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: adverseDownBars()})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Episodes) != 1 || res.Episodes[0].Reason != ExitCatastrophe {
		t.Fatalf("闸关着也应能收紧（tracker 需按「闸或止损任一启用」构造），实际 %+v", res.Episodes)
	}
	if res.SkipTrend != 0 {
		t.Errorf("闸关着不该有 trend_skip，实际 %d", res.SkipTrend)
	}
}

// 参数校验：回测拒绝的参数集必须与实盘启动 fail-fast 拒绝的完全一致。
func TestTrendStopParamsValidation(t *testing.T) {
	base := func() Params {
		p := baseParams()
		p.CatastropheStopPct = 400
		p.TrendGateWindowHours = 2
		p.TrendStopTriggerPct = 3
		p.TrendStopPct = 250
		return p
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("稳妥版 3/250 应通过，得到 %v", err)
	}
	p := base()
	p.TrendStopPct = 150
	if err := p.Validate(); err == nil {
		t.Error("trend_stop_pct=150 低于 250 护栏，回测也必须拒绝（与实盘同口径）")
	}
	p = base()
	p.TrendStopPct = 400
	if err := p.Validate(); err == nil {
		t.Error("Y >= S 不是收紧，必须拒绝")
	}
	p = base()
	p.TrendStopPct = 0
	if err := p.Validate(); err == nil {
		t.Error("只配一半必须拒绝")
	}
	p = base()
	p.TrendGateWindowHours = 0
	if err := p.Validate(); err == nil {
		t.Error("启用了止损但没有窗口 ⇒ 动量永远不可算，必须拒绝而不是静默失效")
	}
}

// adverseDownBars 前 122 根从 62000 匀速跌到 60000（2h 动量 ≈ -3.2%，
// 对多仓是逆势），最后一根探到 58500。
func adverseDownBars() []Bar {
	var bars []Bar
	for i := 0; i <= 121; i++ {
		px := 62000.0 - float64(i)*(2000.0/121.0)
		bars = append(bars, Bar{Ts: ts(i), Open: px, High: px, Low: px, Close: px})
	}
	bars = append(bars, Bar{Ts: ts(122), Open: 60000, High: 60000, Low: 58500, Close: 60000})
	return bars
}
