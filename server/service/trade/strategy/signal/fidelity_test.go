package signal

import (
	"math"
	"strings"
	"testing"
	"time"
)

// 改仓位/止盈/止损/门控/趋势闸 ⇒ 事件级；只有改 signal_threshold 才降级。
func TestClassifyEventLevelForNonThresholdKnobs(t *testing.T) {
	for name, mutate := range map[string]func(*Params){
		"仓位上限":   func(p *Params) { p.Ceiling = 26 },
		"风险预算":   func(p *Params) { p.BudgetPct = 13.3 },
		"兜底止损":   func(p *Params) { p.CatastropheStopPct = 400 },
		"移动止盈分档": func(p *Params) { p.SmallActivatePct = 120; p.SmallGiveback = 0.4 },
		"反向门控":   func(p *Params) { p.GateMinProfitPct = 8 },
		"趋势闸":    func(p *Params) { p.TrendGateWindowHours = 24; p.TrendGateThresholdPct = 5 },
		"下单张数":   func(p *Params) { p.OrderSize = 10 },
	} {
		p := DefaultParams()
		p.RiskEquity = 375.73
		p.BaselineThresholdBp = 3
		p.SignalThresholdBp = 3
		mutate(&p)
		f := Classify(p)
		if f.Level != FidelityEvent {
			t.Errorf("%s：精度 = %s, 期望 %s", name, f.Level, FidelityEvent)
		}
		if f.ThresholdChanged {
			t.Errorf("%s：不该标记阈值变更", name)
		}
	}
}

// 阈值改动一律降级为频率级，且必须带上"推不出时刻"的警示。
func TestClassifyFrequencyOnThresholdChange(t *testing.T) {
	p := DefaultParams()
	p.BaselineThresholdBp = 3
	p.SignalThresholdBp = 5
	f := Classify(p)
	if f.Level != FidelityFrequency || !f.ThresholdChanged {
		t.Fatalf("精度 = %s / changed=%v, 期望 frequency/true", f.Level, f.ThresholdChanged)
	}
	if !strings.Contains(f.Note(), "推不出每次触发时刻") {
		t.Errorf("警示缺少'推不出每次触发时刻': %s", f.Note())
	}
	if !strings.Contains(f.Note(), "门限筛近似") {
		t.Errorf("抬高阈值应说明是门限筛近似: %s", f.Note())
	}

	p.SignalThresholdBp = 1.5
	f = Classify(p)
	if !strings.Contains(f.Note(), "不产出 PnL") {
		t.Errorf("降低阈值应声明不产出 PnL: %s", f.Note())
	}
}

// 每一条 run 都必须带上保守假设的警示（不允许出现空 note）。
func TestClassifyAlwaysCarriesConservativeNotes(t *testing.T) {
	f := Classify(DefaultParams())
	for _, want := range []string{NoteNoMarkInKline, NoteMonitorInterval, NotePeakBias, NoteAdverseFirst} {
		if !strings.Contains(f.Note(), want) {
			t.Errorf("缺少必带警示: %s", want)
		}
	}
}

// 精度等级不同不可横向比较——r11/r16 依赖这条护栏。
func TestComparableOnlyWithinSameLevel(t *testing.T) {
	if !Comparable(Fidelity{Level: FidelityEvent}, Fidelity{Level: FidelityEvent}) {
		t.Error("同级应可比")
	}
	if Comparable(Fidelity{Level: FidelityEvent}, Fidelity{Level: FidelityFrequency}) {
		t.Error("事件级与频率级不得混排比较")
	}
}

// 降阈值时引擎必须拒绝产出 PnL，只给 λ。
func TestReplayRefusesPnlWhenThresholdLowered(t *testing.T) {
	p := baseParams()
	p.BaselineThresholdBp = 3
	p.SignalThresholdBp = 1
	sigs := []Signal{{Ts: ts(1), Side: "long", Event: EvOpen, OrderSize: 1, GapBp: 3.5}}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: flatBars(10, 60000),
		DevWindows: devWindowsFixture(60)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Fidelity.Level != FidelityFrequency {
		t.Errorf("精度 = %s, 期望 frequency", res.Fidelity.Level)
	}
	if res.SignalReplayed != 0 || len(res.Episodes) != 0 {
		t.Errorf("降阈值不得产出回放结果: replayed=%d episodes=%d", res.SignalReplayed, len(res.Episodes))
	}
	if res.Fidelity.Lambda == nil {
		t.Fatal("降阈值应给出 λ(θ)")
	}
}

// 抬阈值走 |gapBp| 门限筛，且被筛掉的数量要显式计数。
func TestReplayFiltersByGapWhenThresholdRaised(t *testing.T) {
	p := baseParams()
	p.BaselineThresholdBp = 3
	p.SignalThresholdBp = 5
	sigs := []Signal{
		{Ts: ts(1), Side: "long", Event: EvOpen, OrderSize: 1, GapBp: 3.5},  // 被筛
		{Ts: ts(2), Side: "long", Event: EvOpen, OrderSize: 1, GapBp: -6.0}, // 保留（取绝对值）
	}
	res, err := Replay(Input{Params: p, Signals: sigs, Bars: flatBars(10, 60000),
		DevWindows: devWindowsFixture(60)})
	if err != nil {
		t.Fatal(err)
	}
	if res.SignalFiltered != 1 || res.SignalReplayed != 1 {
		t.Errorf("筛掉/回放 = %d/%d, 期望 1/1", res.SignalFiltered, res.SignalReplayed)
	}
}

// ── λ(θ) ────────────────────────────────────────────────────────────────────

// devWindowsFixture 构造 n 个 1 分钟窗口：θ 越大穿越越少（指数衰减形态）。
func devWindowsFixture(n int) []DevWindow {
	out := make([]DevWindow, 0, n)
	base := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		out = append(out, DevWindow{
			At(base, i), 120,
			map[float64]int{1: 8, 3: 4, 5: 2, 10: 1},
		})
	}
	return out
}

// At 便于在字面量里写窗口时刻。
func At(base time.Time, i int) time.Time { return base.Add(time.Duration(i) * time.Minute) }

func TestLambdaPerDayAndRatio(t *testing.T) {
	// 60 个 1 分钟窗口 = 1 小时覆盖 = 1/24 天。
	// θ=3 总穿越 240 次 ⇒ λ = 240×24 = 5760 次/天；θ=5 ⇒ 120×24 = 2880。
	lam, err := Lambda(devWindowsFixture(60), 3, 5, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(lam.SpanHours-1) > 1e-9 {
		t.Errorf("覆盖时长 = %.6f 小时, 期望 1", lam.SpanHours)
	}
	if math.Abs(lam.BaselinePerDay-5760) > 1e-6 {
		t.Errorf("λ(3bp) = %.2f, 期望 5760", lam.BaselinePerDay)
	}
	if math.Abs(lam.TargetPerDay-2880) > 1e-6 {
		t.Errorf("λ(5bp) = %.2f, 期望 2880", lam.TargetPerDay)
	}
	if math.Abs(lam.Ratio-0.5) > 1e-9 {
		t.Errorf("λ 比值 = %.4f, 期望 0.5", lam.Ratio)
	}
	if lam.Interpolated {
		t.Error("命中候选阈值时不应标记为插值")
	}
}

// 目标阈值不在候选列表里 ⇒ 对数线性插值 + 明确警示。
func TestLambdaInterpolatesBetweenCandidates(t *testing.T) {
	lam, err := Lambda(devWindowsFixture(60), 3, 4, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !lam.Interpolated {
		t.Fatal("4bp 不在候选阈值内，应标记为插值")
	}
	// θ=3 → 5760，θ=5 → 2880；对数中点 = sqrt(5760×2880) ≈ 4072.4
	want := math.Sqrt(5760 * 2880)
	if math.Abs(lam.TargetPerDay-want) > 1 {
		t.Errorf("λ(4bp) = %.2f, 期望对数插值 %.2f", lam.TargetPerDay, want)
	}
	if lam.Warning == "" {
		t.Error("插值结果必须带警示")
	}
}

// 自检：θ0 的 λ 与真实事件密度差太多要显式报不可信。
func TestLambdaSelfCheckWarnsOnMismatch(t *testing.T) {
	bars := flatBars(1441, 60000) // 约 1 天
	lam, err := Lambda(devWindowsFixture(60), 3, 5, 10, bars)
	if err != nil {
		t.Fatal(err)
	}
	if lam.SelfCheckRatio <= 0 {
		t.Fatal("给了真实事件数就应算出自检比值")
	}
	if !strings.Contains(lam.Warning, "自检失败") {
		t.Errorf("λ(3bp)=5760/天 与真实 10/天 相差三个数量级，应报自检失败, got %q", lam.Warning)
	}
}

func TestLambdaRejectsEmptyInput(t *testing.T) {
	if _, err := Lambda(nil, 3, 5, 0, nil); err == nil {
		t.Fatal("没有 dev_sample 时应报错")
	}
	if _, err := Lambda(devWindowsFixture(2), 0, 5, 0, nil); err == nil {
		t.Fatal("阈值非正应报错")
	}
}
