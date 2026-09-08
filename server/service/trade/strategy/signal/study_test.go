package signal

import (
	"fmt"
	"testing"
	"time"
)

// ─── 降噪协议 ────────────────────────────────────────────────────────────────

// 路径展开的**顺序**是校准的一部分：bootstrap 的重采样池按路径序拼接，
// 顺序一变分位就变。所以这里逐条钉死标签。
func TestFineProtocolExpandsSixteenPathsInGoldenOrder(t *testing.T) {
	paths := FineProtocol().Paths()
	if len(paths) != 16 {
		t.Fatalf("精算协议应展开 16 条路径，得到 %d", len(paths))
	}
	want := []string{
		"close/off0d/nodrop", "close/off0d/drop5%s1",
		"close/off1d/nodrop", "close/off1d/drop5%s1",
		"close/off2d/nodrop", "close/off2d/drop5%s1",
		"close/off3d/nodrop", "close/off3d/drop5%s1",
		"pessimistic/off0d/nodrop", "pessimistic/off0d/drop5%s1",
		"pessimistic/off1d/nodrop", "pessimistic/off1d/drop5%s1",
		"pessimistic/off2d/nodrop", "pessimistic/off2d/drop5%s1",
		"pessimistic/off3d/nodrop", "pessimistic/off3d/drop5%s1",
	}
	for i, w := range want {
		if got := paths[i].Label(); got != w {
			t.Errorf("第 %d 条路径: got %s want %s", i, got, w)
		}
		if paths[i].Index != i {
			t.Errorf("第 %d 条路径的 Index=%d", i, paths[i].Index)
		}
	}
}

func TestCoarseProtocolExpandsFourPaths(t *testing.T) {
	paths := CoarseProtocol().Paths()
	if len(paths) != 4 {
		t.Fatalf("粗网格协议应展开 4 条路径，得到 %d", len(paths))
	}
	for _, p := range paths {
		if p.DropSeed != nil {
			t.Errorf("粗网格不该丢信号，路径 %s 带了种子", p.Label())
		}
	}
}

// 零值协议必须补成金标准缺省：协议是随任务冻结落库的，补错等于历史任务不可复现。
func TestProtocolNormalizeFillsGoldenDefaults(t *testing.T) {
	p := Protocol{}.Normalize()
	if p.DropRate != 0.05 || p.NormalizeDays != 28 || p.ScenarioDays != 30 {
		t.Errorf("抖动/归一缺省不对: %+v", p)
	}
	if p.BootstrapMode != BootstrapDayIID || p.BootstrapDraws != 2000 || p.BootstrapSeed != 42 {
		t.Errorf("bootstrap 缺省应为金标准口径 day_iid/2000/42: %+v", p)
	}
	if p.LambdaDenom != LambdaDenomWindow || p.LambdaMonthDays != 30 {
		t.Errorf("λ 口径缺省不对: %+v", p)
	}
	if p.Regime.TrendAbsRetPct != 1.5 || p.Regime.VolRangePct != 2.5 {
		t.Errorf("日标签判据缺省不对: %+v", p.Regime)
	}
	// 非法的 bootstrap 口径要落回金标准，而不是原样接受一个引擎不认识的值。
	if got := (Protocol{BootstrapMode: "monte-carlo-magic"}).Normalize().BootstrapMode; got != BootstrapDayIID {
		t.Errorf("未知 bootstrapMode 应落回 day_iid，得到 %s", got)
	}
}

// ─── 日状态标签 ──────────────────────────────────────────────────────────────

func synthBars(day string, o, hi, lo, c float64) []Bar {
	base, _ := time.ParseInLocation("2006-01-02", day, time.UTC)
	return []Bar{
		{Ts: base.Add(time.Minute), Open: o, High: o, Low: o, Close: o},
		{Ts: base.Add(2 * time.Minute), Open: o, High: hi, Low: lo, Close: (hi + lo) / 2},
		{Ts: base.Add(3 * time.Minute), Open: o, High: c, Low: c, Close: c},
	}
}

func TestDayLabelsMatchesGoldenRule(t *testing.T) {
	bars := append(synthBars("2026-06-09", 100, 101, 99, 102), synthBars("2026-06-10", 100, 103, 100, 100.5)...)
	bars = append(bars, synthBars("2026-06-11", 100, 100.5, 99.8, 100.2)...)
	labels := DayLabels(bars, RegimeThresholds{})
	// 收益 +2% ≥ 1.5% → trend（不分涨跌）
	if labels["2026-06-09"] != RegimeTrend {
		t.Errorf("6/09 应为 trend，得到 %s", labels["2026-06-09"])
	}
	// 收益 0.5% 不到线，但振幅 3% ≥ 2.5% → vol
	if labels["2026-06-10"] != RegimeVol {
		t.Errorf("6/10 应为 vol，得到 %s", labels["2026-06-10"])
	}
	if labels["2026-06-11"] != RegimeQuiet {
		t.Errorf("6/11 应为 quiet，得到 %s", labels["2026-06-11"])
	}
	if CountLabel(labels, RegimeTrend) != 1 || CountLabel(labels, RegimeVol) != 1 {
		t.Errorf("标签计数不对: %v", labels)
	}
	if days := LabeledDays(labels, RegimeTrend); len(days) != 1 || days[0] != "2026-06-09" {
		t.Errorf("单边日清单不对: %v", days)
	}
}

// ─── episode 对齐的分块 ──────────────────────────────────────────────────────

func day(s string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", s, time.UTC)
	return t
}

// 一段扛单跨过日界时**不允许**在那里切块：把深浮亏和回本拆到两个月里，
// 会凭空造出现实中不存在的尾部（这正是 day_iid 高估尾部的机制）。
func TestBuildMtmBlocksNeverCutsThroughAnOpenPosition(t *testing.T) {
	daily := []DayMTM{
		{Day: "2026-06-09", Delta: -10},
		{Day: "2026-06-10", Delta: 30},
		{Day: "2026-06-11", Delta: 5},
	}
	labels := map[string]string{"2026-06-09": RegimeTrend, "2026-06-10": RegimeTrend, "2026-06-11": RegimeQuiet}
	// 仓位从 6/09 中午扛到 6/10 中午：6/10 00:00 这个日界落在持仓内部。
	eps := []Episode{{OpenedAt: day("2026-06-09").Add(12 * time.Hour), ClosedAt: day("2026-06-10").Add(12 * time.Hour)}}
	blocks := BuildMtmBlocks(daily, eps, labels, time.UTC)
	if len(blocks) != 2 {
		t.Fatalf("应切成 2 块（[6/09,6/10] 与 [6/11]），得到 %d 块: %+v", len(blocks), blocks)
	}
	if len(blocks[0].Days) != 2 || blocks[0].Delta != 20 {
		t.Errorf("第一块应含 2 天、ΔMTM=20，得到 %+v", blocks[0])
	}
	if blocks[0].Label != RegimeTrend {
		t.Errorf("第一块两天都是单边日，主导标签应为 trend，得到 %s", blocks[0].Label)
	}
	if len(blocks[1].Days) != 1 || blocks[1].Label != RegimeQuiet {
		t.Errorf("第二块不对: %+v", blocks[1])
	}
}

func TestBuildMtmBlocksCutsAtFlatBoundaries(t *testing.T) {
	daily := []DayMTM{{Day: "2026-06-09", Delta: 1}, {Day: "2026-06-10", Delta: 2}}
	labels := map[string]string{"2026-06-09": RegimeQuiet, "2026-06-10": RegimeQuiet}
	// 仓位当天开当天平 → 6/10 00:00 是空仓时刻 → 可切。
	eps := []Episode{{OpenedAt: day("2026-06-09").Add(2 * time.Hour), ClosedAt: day("2026-06-09").Add(5 * time.Hour)}}
	if got := len(BuildMtmBlocks(daily, eps, labels, time.UTC)); got != 2 {
		t.Errorf("空仓日界应切块，得到 %d 块", got)
	}
	// 期末未平仓 → 之后任何日界都不是切点。
	epsOpen := []Episode{{OpenedAt: day("2026-06-09").Add(2 * time.Hour), Open: true}}
	if got := len(BuildMtmBlocks(daily, epsOpen, labels, time.UTC)); got != 1 {
		t.Errorf("未平仓不该切块，得到 %d 块", got)
	}
}

// ─── 三关阈值 ────────────────────────────────────────────────────────────────

// 派生口径必须给出设计文档 §10.2 那两个数（−56U / 94U），否则历史结论无法复算。
func TestGatesDeriveHistoricalThresholds(t *testing.T) {
	g := DefaultGates().Normalize()
	if !g.Derived {
		t.Error("缺省阈值应标记为派生")
	}
	if diff := g.BearNetP10Min - (-56.3595); diff > 1e-4 || diff < -1e-4 {
		t.Errorf("熊市 p10 下限 %.4f，期望 −56.3595", g.BearNetP10Min)
	}
	if diff := g.MaxDrawdownMax - 93.9325; diff > 1e-4 || diff < -1e-4 {
		t.Errorf("p90 回撤上限 %.4f，期望 93.9325", g.MaxDrawdownMax)
	}
	if diff := g.StopBudgetMax - 56.3595; diff > 1e-4 || diff < -1e-4 {
		t.Errorf("月度预算 %.4f，期望 56.3595", g.StopBudgetMax)
	}
	// 显式给绝对阈值时不再派生（用于复现历史标准）。
	e := Gates{RiskEquity: 100, BearNetP10Min: -20, MaxDrawdownMax: 30}.Normalize()
	if e.Derived || e.BearNetP10Min != -20 || e.MaxDrawdownMax != 30 {
		t.Errorf("显式阈值被覆盖了: %+v", e)
	}
}

// Normalize 必须幂等：阈值冻结成 JSON 落库后会被读回来再 Normalize 一次
// （OOS 任务继承阈值走的就是这条路）。第二次调用若把派生阈值误判成显式给定，
// Describe() 里那句"由 E/B_month/DD_max 派生"就成了假话，而它正是预注册阈值的
// 审计说明。
func TestGatesNormalizeIsIdempotent(t *testing.T) {
	for _, in := range []Gates{
		DefaultGates(),
		{RiskEquity: 414, BearBudgetPct: 15, DdMaxPct: 25, SignMin: 0.8},
		{RiskEquity: 100, BearNetP10Min: -20, MaxDrawdownMax: 30},
	} {
		once := in.Normalize()
		twice := once.Normalize()
		if once != twice {
			t.Errorf("Normalize 不幂等:\n once=%+v\ntwice=%+v", once, twice)
		}
		if once.Describe() != twice.Describe() {
			t.Errorf("阈值表述在二次归一后变了:\n once=%s\ntwice=%s", once.Describe(), twice.Describe())
		}
	}
}

func cellWithBear(key string, med, sign, dd, p10, p50 float64) CellStats {
	spec := CellSpec{Mode: ModeNet, Cap: 26, StopPct: 400, GatePct: 20}
	return CellStats{Spec: spec, Key: key, MedPnl28: med, SignRatio: sign, P90MaxDrawdown: dd,
		Scenarios: []ScenarioResult{{Scenario: ScenarioBear, PoolSize: 176, P10: p10, P50: p50}}}
}

func TestJudgeThreeGates(t *testing.T) {
	g := Gates{RiskEquity: 375.73, BearBudgetPct: 15, DdMaxPct: 25, SignMin: 0.8}.Normalize()

	// 三关全过。
	v := Judge(cellWithBear("ok", 120, 1.0, 50, -30, 20), g)
	if !v.Passed || v.PassCnt != 3 {
		t.Errorf("应三关全过: %+v", v)
	}

	// 中位数很高但一致率 0.69 —— 正是必须被否掉的噪声格（net(15,400) 的真实形态）。
	v = Judge(cellWithBear("noisy", 79.9, 0.69, 89.2, -276, -148), g)
	if v.OkSign {
		t.Error("一致率 0.69 不该过一致率关")
	}
	if v.Passed {
		t.Error("噪声格不该通过三关")
	}
	if len(v.Reasons) == 0 {
		t.Error("未过关必须给出逐条原因")
	}

	// 熊市池为空 → 不可判定，按**不通过**计。
	empty := CellStats{Key: "nobear", SignRatio: 1, P90MaxDrawdown: 10,
		Scenarios: []ScenarioResult{{Scenario: ScenarioBear, PoolSize: 0}}}
	v = Judge(empty, g)
	if v.OkBear || v.BearAvailable || v.Passed {
		t.Errorf("熊市不可估计时不得放行: %+v", v)
	}

	// 参考项（频率预算）不计入三关。
	st := cellWithBear("budget", 120, 1.0, 50, -30, 20)
	st.LambdaBearPerMonth, st.MeanStopLoss = 10, 40
	st.StopBudget = 400
	v = Judge(st, g)
	if v.OkStopBudget {
		t.Error("预算 400U 远超 56U，参考项应为 false")
	}
	if !v.Passed {
		t.Error("参考项不通过不该影响三关结论")
	}
}

// ─── 搜索空间与收敛 ──────────────────────────────────────────────────────────

func TestDefaultSearchSpaceExpandsFiftyFiveCoarseCells(t *testing.T) {
	cells := DefaultSearchSpace().CoarseCells()
	if len(cells) != 55 {
		t.Fatalf("缺省粗网格应为 55 格（净仓 7×5 + 双向 4×5），得到 %d", len(cells))
	}
	seen := map[string]bool{}
	for _, c := range cells {
		if seen[c.Key()] {
			t.Errorf("格子重复: %s", c.Key())
		}
		seen[c.Key()] = true
		if c.GatePct != 20 {
			t.Errorf("粗网格的 gate 应固定 20，%s 是 %.0f", c.Key(), c.GatePct)
		}
	}
	if !seen["net(999,400,g20)"] {
		t.Error("缺少 cap=∞ 对照格")
	}
}

func TestConvergePicksTopKPlusStableAndIncumbent(t *testing.T) {
	mk := func(cap int, med, sign float64) CellStats {
		spec := CellSpec{Mode: ModeNet, Cap: cap, StopPct: 400, GatePct: 20}
		return CellStats{Spec: spec, Key: spec.Key(), MedPnl28: med, SignRatio: sign}
	}
	coarse := []CellStats{
		mk(26, 140, 1.0), mk(20, 100, 0.5), mk(15, 80, 0.5),
		mk(10, 5, 1.0), // 低幅但 4 路径全正：必须被 KeepAllPositive 捞上来
		mk(32, 3, 0.25),
	}
	incumbent := &CellSpec{Mode: ModeNet, Cap: 12, StopPct: 300, GatePct: 20}
	space := DefaultSearchSpace().Normalize()
	res := Converge(coarse, space, Convergence{TopK: 2, KeepAllPositive: true, ExpandGateAxis: false, MaxCells: 40}, incumbent)

	keys := map[string]bool{}
	for _, c := range res.Cells {
		keys[c.Key()] = true
	}
	for _, want := range []string{"net(26,400,g20)", "net(20,400,g20)", "net(10,400,g20)", "net(12,300,g20)"} {
		if !keys[want] {
			t.Errorf("精算格缺少 %s（选中的是 %v）", want, keys)
		}
	}
	if keys["net(32,400,g20)"] {
		t.Error("排名靠后且一致率不足的格子不该入选")
	}
	if res.Note == "" {
		t.Error("收敛必须给出「为什么这么选」")
	}
}

func TestConvergeExpandsGateAxisAndTruncatesLoudly(t *testing.T) {
	mk := func(cap int, med float64) CellStats {
		spec := CellSpec{Mode: ModeNet, Cap: cap, StopPct: 400, GatePct: 20}
		return CellStats{Spec: spec, Key: spec.Key(), MedPnl28: med}
	}
	coarse := []CellStats{mk(26, 140), mk(20, 100), mk(15, 80)}
	space := DefaultSearchSpace().Normalize() // GatePcts = [8,20]
	res := Converge(coarse, space, Convergence{TopK: 3, ExpandGateAxis: true, MaxCells: 40}, nil)
	if len(res.Cells) != 6 {
		t.Fatalf("3 格 × gate 2 档应得 6 格，得到 %d: %v", len(res.Cells), res.Cells)
	}
	// 同一格的两个 gate 必须相邻，方便逐对读副轴效应。
	if res.Cells[0].Cap != res.Cells[1].Cap {
		t.Errorf("同格的两个 gate 应相邻: %v", res.Cells[:2])
	}

	// 截断必须发声：静默截断会让"覆盖了整个空间"这句话变成假的。
	res = Converge(coarse, space, Convergence{TopK: 3, ExpandGateAxis: true, MaxCells: 4}, nil)
	if len(res.Cells) != 4 || len(res.Dropped) != 2 {
		t.Errorf("应截断到 4 格并列出 2 个被丢掉的: cells=%d dropped=%v", len(res.Cells), res.Dropped)
	}
}

// ─── 支配关系与结论 ──────────────────────────────────────────────────────────

// 用 study_fine.csv 的真实数字验"现行 champion 被全面支配"（§10.3 第 2 条）。
func TestConcludeNoSolutionAndDominatedIncumbent(t *testing.T) {
	mk := func(mode string, cap int, s, gate, med, sign, lam, dd, p10, p50 float64) CellStats {
		spec := CellSpec{Mode: mode, Cap: cap, StopPct: s, GatePct: gate}
		return CellStats{Spec: spec, Key: spec.Key(), MedPnl28: med, SignRatio: sign,
			LambdaBearPerMonth: lam, P90MaxDrawdown: dd,
			Scenarios: []ScenarioResult{{Scenario: ScenarioBear, PoolSize: 176, P10: p10, P50: p50}}}
	}
	cells := []CellStats{
		mk("net", 26, 400, 20, 142.66, 1.00, 2.73, 88.84, -113.40, 24.11),
		mk("net", 20, 400, 20, 101.95, 1.00, 4.09, 61.45, -189.20, -59.87),
		mk("net", 15, 300, 20, 6.88, 0.5625, 10.91, 97.47, -220.90, -125.70), // 现行 champion
	}
	gates := DefaultGates().Normalize()
	verdicts := map[string]CellVerdict{}
	for _, st := range cells {
		verdicts[st.Key] = Judge(st, gates)
	}
	incumbent := &CellSpec{Mode: "net", Cap: 15, StopPct: 300, GatePct: 20}
	c := Conclude(cells, verdicts, gates, incumbent)

	if c.Verdict != VerdictNoSolution {
		t.Fatalf("三关无一通过时应走无解分支，得到 %s", c.Verdict)
	}
	if len(c.PassedCells) != 0 {
		t.Errorf("不该有通过的格子: %v", c.PassedCells)
	}
	if !c.IncumbentDominated {
		t.Errorf("现行 net(15,300) 应被全面支配，支配者=%v", c.DominatorKeys)
	}
	if len(c.DominatorKeys) != 2 || c.DominatorKeys[0] != "net(20,400,g20)" || c.DominatorKeys[1] != "net(26,400,g20)" {
		// 脊线上两格在六个维度上都不差于现行配置 —— §10.3 那句"脊线上任一格在所有
		// 指标上同时优于它"的字面验证。清单按键名排序，便于稳定断言。
		t.Errorf("支配者清单不对: %v", c.DominatorKeys)
	}
	// 结论正文里绝不能出现"最优参数"。
	all := c.Headline
	for _, l := range c.Lines {
		all += " " + l
	}
	for _, banned := range []string{"最优参数", "最佳参数"} {
		if contains(all, banned) {
			t.Errorf("结论里出现了被禁止的措辞 %q", banned)
		}
	}
	if !contains(all, "权衡前沿") || !contains(all, "OOS") {
		t.Errorf("无解分支必须给出权衡前沿与 OOS 纪律，实际：%s", all)
	}
	// 前沿：(26,400) 中位最高，必在收益/回撤前沿上；(20,400) 回撤最小，也在。
	onDd := map[string]bool{}
	for _, p := range c.Frontier {
		if p.OnDdFrontier {
			onDd[p.Key] = true
		}
	}
	if !onDd["net(26,400,g20)"] || !onDd["net(20,400,g20)"] {
		t.Errorf("收益/回撤前沿不对: %v", onDd)
	}
	if onDd["net(15,300,g20)"] {
		t.Error("被全面支配的现行配置不可能在前沿上")
	}
}

func TestConcludeCandidateFound(t *testing.T) {
	spec := CellSpec{Mode: ModeNet, Cap: 26, StopPct: 400, GatePct: 8}
	st := CellStats{Spec: spec, Key: spec.Key(), MedPnl28: 168.7, SignRatio: 1.0, P90MaxDrawdown: 40,
		Scenarios: []ScenarioResult{{Scenario: ScenarioBear, PoolSize: 176, P10: -20, P50: 60}}}
	gates := DefaultGates().Normalize()
	c := Conclude([]CellStats{st}, map[string]CellVerdict{st.Key: Judge(st, gates)}, gates, nil)
	if c.Verdict != VerdictCandidateFound || len(c.PassedCells) != 1 {
		t.Fatalf("应给出候选: %+v", c)
	}
	joined := c.Headline
	for _, l := range c.Lines {
		joined += " " + l
	}
	if !contains(joined, "没被三关否掉") {
		t.Errorf("候选必须明确说明它只是「没被否掉」，实际：%s", joined)
	}
	if !contains(joined, "不自动下发") {
		t.Errorf("候选必须声明不自动改线上参数，实际：%s", joined)
	}
}

// ─── 规模不变性 ──────────────────────────────────────────────────────────────

// §10.3 第 5 条的可计算版本：小额账户按公式只能实例化 cap≈8，装不下 cap=26。
func TestCheckScaleInvarianceFlagsSmallAccount(t *testing.T) {
	base := DefaultParams()
	base.BudgetPct = 13.3
	base.RiskEquity = 120
	spec := CellSpec{Mode: ModeNet, Cap: 26, StopPct: 400, GatePct: 8}
	cells := []CellStats{{Spec: spec, Key: spec.Key()}}

	small := CheckScaleInvariance(cells, base, 120, 63000)
	if len(small) != 1 || small[0].Feasible {
		t.Fatalf("120U 本金装不下 cap=26，应标破缺: %+v", small)
	}
	if small[0].FormulaCap >= 26 || small[0].FormulaCap < 1 {
		t.Errorf("公式上限应远小于 26，得到 %d", small[0].FormulaCap)
	}
	if !contains(small[0].Note, "规模不变性破缺") {
		t.Errorf("提示文案不对: %s", small[0].Note)
	}

	big := CheckScaleInvariance(cells, base, 414, 63000)
	if !big[0].Feasible {
		t.Errorf("414U 本金下 cap=26 应可实例化（公式 %d 张）: %s", big[0].FormulaCap, big[0].Note)
	}
}

// ─── 统计口径 ────────────────────────────────────────────────────────────────

// 中位数走 Python statistics.median 口径（偶数取中间两个的平均），
// 分位走金标准的"排序取 int(n*q) 位"口径——两者都不能换，换了就与 study_fine.csv 对不上。
func TestMedianAndQuantileKeepGoldenSemantics(t *testing.T) {
	if got := medianOf([]float64{1, 2, 3, 4}); got != 2.5 {
		t.Errorf("偶数个中位数应为 2.5，得到 %v", got)
	}
	if got := medianOf([]float64{3, 1, 2}); got != 2 {
		t.Errorf("奇数个中位数应为 2，得到 %v", got)
	}
	xs := make([]float64, 16)
	for i := range xs {
		xs[i] = float64(i)
	}
	if got := quantileByIndex(xs, 0.9); got != 14 {
		t.Errorf("16 个样本的 p90 应取第 14 位（int(16*0.9)=14），得到 %v", got)
	}
	if got := quantileByIndex(xs, 0.25); got != 4 {
		t.Errorf("p25 应取第 4 位，得到 %v", got)
	}
	if got := quantileByIndex([]float64{7}, 0.9); got != 7 {
		t.Errorf("单样本分位应为它自己，得到 %v", got)
	}
}

// ─── 路径构造 ────────────────────────────────────────────────────────────────

// 5% 丢弃必须可复现（同种子同结果），且偏移路径不得沿用窗口起点的种子仓。
func TestRunPathDropIsDeterministicAndOffsetDropsSeed(t *testing.T) {
	start := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	bars := make([]Bar, 0, 4320)
	signals := make([]Signal, 0, 400)
	px := 60000.0
	for i := 0; i < 4320; i++ { // 3 天
		ts := start.Add(time.Duration(i) * time.Minute)
		bars = append(bars, Bar{Ts: ts, Open: px, High: px, Low: px, Close: px})
		if i%10 == 0 {
			side := "long"
			if (i/10)%2 == 1 {
				side = "short"
			}
			signals = append(signals, Signal{Ts: ts.Add(time.Second), Side: side, Event: EvOpen, OrderSize: 1})
		}
	}
	p := DefaultParams()
	p.OrderSize = 1
	p.CapOverride = 20
	p.RiskEquity = 375.73
	in := Input{Params: p, Signals: signals, Bars: bars,
		Seed: SeedPosition{At: start, Side: "long", Size: 3, AvgPx: px}}
	proto := FineProtocol()
	labels := DayLabels(bars, proto.Regime)
	one := int64(1)

	drop := PathSpec{EvalMode: EvalClose, OffsetDays: 0, DropSeed: &one, DropRate: 0.05}
	a, err := RunPath(in, drop, start, labels, proto)
	if err != nil {
		t.Fatalf("丢弃路径: %v", err)
	}
	b, err := RunPath(in, drop, start, labels, proto)
	if err != nil {
		t.Fatalf("丢弃路径重跑: %v", err)
	}
	if a.SignalIn != b.SignalIn || a.Net != b.Net {
		t.Errorf("同种子必须给同结果: %d/%v vs %d/%v", a.SignalIn, a.Net, b.SignalIn, b.Net)
	}
	full, err := RunPath(in, PathSpec{EvalMode: EvalClose}, start, labels, proto)
	if err != nil {
		t.Fatalf("不丢弃路径: %v", err)
	}
	if a.SignalIn >= full.SignalIn {
		t.Errorf("5%% 丢弃后触发数应变少: %d vs %d", a.SignalIn, full.SignalIn)
	}
	if lost := full.SignalIn - a.SignalIn; lost > full.SignalIn/5 {
		t.Errorf("丢弃比例明显偏离 5%%：丢了 %d/%d", lost, full.SignalIn)
	}

	// 偏移 1 天：窗口起点的旧仓不该被搬过去（它当时可能早已平掉）。
	off, err := RunPath(in, PathSpec{EvalMode: EvalClose, OffsetDays: 1}, start, labels, proto)
	if err != nil {
		t.Fatalf("偏移路径: %v", err)
	}
	if off.BarCount >= full.BarCount {
		t.Errorf("偏移 1 天应少 1440 根: %d vs %d", off.BarCount, full.BarCount)
	}
	if len(off.DailyMTM) == 0 {
		t.Error("偏移路径应仍有日 MTM 序列")
	}
	// 偏移过大导致没有 K 线时必须报错，而不是静默给一格空结果。
	if _, err := RunPath(in, PathSpec{EvalMode: EvalClose, OffsetDays: 30}, start, labels, proto); err == nil {
		t.Error("偏移超出窗口应报错")
	}
}

// AggregateCell 必须把口径说明与已知偏差一起带出来：读结果的人不一定读过设计文档。
func TestAggregateCellCarriesProtocolNotes(t *testing.T) {
	spec := CellSpec{Mode: ModeNet, Cap: 26, StopPct: 400, GatePct: 20}
	proto := FineProtocol().Normalize()
	labels := map[string]string{"2026-06-09": RegimeTrend}
	outcomes := []PathOutcome{
		{Pnl28: 10, MaxDD: 5, Days: 1, DailyMTM: []DayMTM{{Day: "2026-06-09", Delta: 10}}},
		{Pnl28: -2, MaxDD: 8, Days: 1, DailyMTM: []DayMTM{{Day: "2026-06-09", Delta: -2}}},
	}
	st := AggregateCell(spec, outcomes, labels, proto)
	if st.SignRatio != 0.5 {
		t.Errorf("2 条路径 1 正应为 0.5，得到 %v", st.SignRatio)
	}
	if st.MedPnl28 != 4 {
		t.Errorf("中位应为 4，得到 %v", st.MedPnl28)
	}
	if st.Fidelity != FidelityEvent {
		t.Errorf("寻优格恒为事件级，得到 %s", st.Fidelity)
	}
	joined := ""
	for _, n := range st.Notes {
		joined += n + " "
	}
	for _, want := range []string{"降噪协议", "熊市月", "day_iid", "λ_bear 分母"} {
		if !contains(joined, want) {
			t.Errorf("口径说明里缺少 %q：%s", want, joined)
		}
	}
	if bear := st.Scenario(ScenarioBear); bear == nil || bear.PoolSize != 2 {
		t.Errorf("熊市池应含两条路径各自的单边日: %+v", bear)
	}
	if st.Scenario("nope") != nil {
		t.Error("不存在的情景应返回 nil")
	}
	// 空格子不能崩，但必须说清统计量无意义。
	empty := AggregateCell(spec, nil, labels, proto)
	if len(empty.Notes) == 0 || !contains(empty.Notes[0], "无意义") {
		t.Errorf("空格子的说明不对: %v", empty.Notes)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ = fmt.Sprintf
