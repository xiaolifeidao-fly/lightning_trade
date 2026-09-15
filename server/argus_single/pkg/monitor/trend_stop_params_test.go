package monitor

import (
	"testing"

	"common/middleware/vipper"

	"argus_single/pkg/trade"
)

// 趋势条件止损的两个旋钮要和其余 trail 参数走同一条解析路径
// （账户级 trade.accountN.* > 全局 > 缺省 0=未启用），否则热更新覆盖层
// 只盖住一半参数，线上会出现"改了不生效"。

func TestTrailParamsCarriesTrendStopKnobs(t *testing.T) {
	p := TrailParams{
		CatastropheStopPct:  400,
		TrendStopTriggerPct: 3,
		TrendStopPct:        250,
	}
	got := p.TrendStop()
	want := trade.TrendStopParams{BaseStopPct: 400, TriggerPct: 3, TightStopPct: 250}
	if got != want {
		t.Fatalf("TrendStop() = %+v, want %+v", got, want)
	}
}

func TestTrailParamsTrendStopBaseFollowsCatastrophe(t *testing.T) {
	// 基线必须跟着 CatastropheStopPct 走：这两个值分别配置时，
	// ResolveTrendStop 的「Y ≥ S 就不收紧」护栏才判得对。
	p := TrailParams{CatastropheStopPct: 300, TrendStopTriggerPct: 3, TrendStopPct: 250}
	if got := p.TrendStop().BaseStopPct; got != 300 {
		t.Fatalf("BaseStopPct 应取 CatastropheStopPct=300，得到 %v", got)
	}
}

func TestResolveTrailParamsReadsTrendStopFromAccountKeys(t *testing.T) {
	const idx = 91 // 用不存在的账户号，避免与其他测试共享的 vipper 键互相污染
	vipper.Set("trade.account91.trend_stop_trigger_pct", 3.5)
	vipper.Set("trade.account91.trend_stop_pct", 250.0)
	defer func() {
		vipper.Set("trade.account91.trend_stop_trigger_pct", 0.0)
		vipper.Set("trade.account91.trend_stop_pct", 0.0)
	}()

	p := resolveTrailParamsForAccount(idx)
	if p.TrendStopTriggerPct != 3.5 {
		t.Errorf("TrendStopTriggerPct = %v, want 3.5", p.TrendStopTriggerPct)
	}
	if p.TrendStopPct != 250 {
		t.Errorf("TrendStopPct = %v, want 250", p.TrendStopPct)
	}
}

func TestResolveTrailParamsTrendStopDefaultsOff(t *testing.T) {
	// 缺省必须是 0/0：不配置绝不启用，与趋势闸同一部署安全语义。
	p := resolveTrailParamsForAccount(92)
	if p.TrendStopTriggerPct != 0 || p.TrendStopPct != 0 {
		t.Fatalf("未配置时应为 0/0（未启用），得到 %v/%v", p.TrendStopTriggerPct, p.TrendStopPct)
	}
	// 未启用时 ResolveTrendStop 必须原样返回 S。
	if got, tightened := trade.ResolveTrendStop("short", 99, true, p.TrendStop()); tightened {
		t.Fatalf("未启用却收紧到 %v", got)
	}
}

// withTrendStop 是"把趋势条件止损叠加到 trail 参数上"的纯函数。
// 它只许改兜底线一项——分档比例与三档阈值被顺手改掉的话，收紧止损会连带
// 改变移动止盈的触发点，事后从结果上根本分不清是哪个改动造成的。

func TestWithTrendStopOnlyChangesCatastropheLine(t *testing.T) {
	base := TrailParams{
		TierSmallRatio: 0.30, TierLargeRatio: 0.65,
		Small:              Tier{ActivatePct: 150, GivebackFrac: 0.35},
		Medium:             Tier{ActivatePct: 90, GivebackFrac: 0.28},
		Large:              Tier{ActivatePct: 40, GivebackFrac: 0.20},
		CatastropheStopPct: 400, TrendStopTriggerPct: 3, TrendStopPct: 250,
	}
	got, tightened := withTrendStop(base, "short", 4.09, true)
	if !tightened {
		t.Fatal("逆向动量 4.09% ≥ 3% 应收紧")
	}
	if got.CatastropheStopPct != 250 {
		t.Errorf("兜底线应为 250，得到 %v", got.CatastropheStopPct)
	}
	want := base
	want.CatastropheStopPct = 250
	if got != want {
		t.Errorf("除兜底线外不得改动任何字段\n got: %+v\nwant: %+v", got, want)
	}
}

func TestWithTrendStopLeavesParamsAloneWhenNotTightened(t *testing.T) {
	base := TrailParams{CatastropheStopPct: 400, TrendStopTriggerPct: 3, TrendStopPct: 250}
	got, tightened := withTrendStop(base, "short", 1.0, true)
	if tightened || got != base {
		t.Fatalf("动量不到阈值应原样返回，得到 %+v tightened=%v", got, tightened)
	}
}

func TestWithTrendStopFailsSafeWhenMomentumUnknown(t *testing.T) {
	base := TrailParams{CatastropheStopPct: 400, TrendStopTriggerPct: 3, TrendStopPct: 250}
	got, tightened := withTrendStop(base, "short", 9.9, false)
	if tightened || got.CatastropheStopPct != 400 {
		t.Fatalf("动量不可算必须沿用 400，得到 %v tightened=%v", got.CatastropheStopPct, tightened)
	}
}

// 启用配方的端到端组合守卫：全局键 → resolveTrailParamsForAccount → withTrendStop。
// AccFloat 的"账户级 > 全局 > 缺省"回退本身早有测试，这条守的是**本次新接的线**：
// 两个键名拼对了、TrendStop() 的基线取自 CatastropheStopPct、叠加只改兜底线。
// 线上就是按这个配方开的，配方错了比函数错了更难发现。
func TestTrendStopEnableRecipeViaGlobalKeys(t *testing.T) {
	vipper.Set("position.monitor.trend_stop.trigger_pct", 3.0)
	vipper.Set("position.monitor.trend_stop.stop_pct", 250.0)
	vipper.Set("trade.account93.catastrophe_stop_pct", 400.0)
	defer func() {
		vipper.Set("position.monitor.trend_stop.trigger_pct", 0.0)
		vipper.Set("position.monitor.trend_stop.stop_pct", 0.0)
		vipper.Set("trade.account93.catastrophe_stop_pct", 0.0)
	}()

	tp := resolveTrailParamsForAccount(93)
	if tp.CatastropheStopPct != 400 {
		t.Fatalf("前置：S 应为 400，得到 %v", tp.CatastropheStopPct)
	}

	// 趋势顶着空仓 +4.09% → 收紧到 250
	if got, tightened := withTrendStop(tp, "short", 4.09, true); !tightened || got.CatastropheStopPct != 250 {
		t.Errorf("逆向趋势应收紧到 250，得到 %v tightened=%v", got.CatastropheStopPct, tightened)
	}
	// 顺势 → 仍是 400
	if got, tightened := withTrendStop(tp, "short", -4.09, true); tightened || got.CatastropheStopPct != 400 {
		t.Errorf("顺势应沿用 400，得到 %v tightened=%v", got.CatastropheStopPct, tightened)
	}
	// 动量不可算 → 仍是 400
	if got, tightened := withTrendStop(tp, "short", 4.09, false); tightened || got.CatastropheStopPct != 400 {
		t.Errorf("动量不可算应沿用 400，得到 %v tightened=%v", got.CatastropheStopPct, tightened)
	}
}

// withTrendNote 把收紧说明拼进平仓理由。收紧后的平仓在 ROI 上看是一笔 -250%
// 而不是 -400%，理由里不写清"为什么是 250"，事后没人能判断它是有人改了配置
// 还是趋势条件止损生效了。

func TestWithTrendNoteAppendsReason(t *testing.T) {
	got := withTrendNote("兜底止损", "趋势条件止损: 逆向动量+4.09% ≥ 3.0%, 兜底线 400% → 250%")
	if !strings2Contains(got, "兜底止损") {
		t.Errorf("原理由必须保留，得到 %q", got)
	}
	if !strings2Contains(got, "400% → 250%") {
		t.Errorf("收紧说明必须出现，得到 %q", got)
	}
}

func TestWithTrendNoteUnchangedWhenNotTightened(t *testing.T) {
	// 没收紧时必须**原样**返回：平时的平仓理由不该被多加一对空括号。
	if got := withTrendNote("兜底止损", ""); got != "兜底止损" {
		t.Fatalf("未收紧应原样返回，得到 %q", got)
	}
	if got := withTrendNote("移动止盈", ""); got != "移动止盈" {
		t.Fatalf("未收紧应原样返回，得到 %q", got)
	}
}

func strings2Contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
