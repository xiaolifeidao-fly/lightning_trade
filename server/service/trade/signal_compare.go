package trade

import (
	"fmt"
	"sort"
	"strings"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"
)

// 本文件是参数组横向对比（r11）的纯计算部分：参数 diff、指标 diff、按 fidelity
// 分组排序。全是纯函数，不碰 DB，便于把"不得混排"这条护栏用单测钉死。
//
// 两条硬规则：
//
//  1. **跨精度不比大小**。事件级（回放真实触发点）与频率级（只能推 λ(θ)）的
//     结果分在不同的 group 里，各自组内排序；接口从不返回一张跨精度的全局榜，
//     前端拿不到混排数据也就无从误排。
//  2. **不产 PnL 的组不参与净利排序**。降低阈值的频率级组只输出 λ(θ)
//     （见 signal.Replay 的早返回），把它的 netPnl=0 排进榜里会显示成"最差的一组"，
//     而事实是"这一组没有可比的净利"。

// 精度等级的中文标签。落在接口上而不是前端硬编码：这两个词的含义是需求口径
// （§3.3），不是展示文案，说法必须全站一致。
func fidelityLabel(level string) string {
	switch level {
	case signal.FidelityFrequency:
		return "频率级（只能推 λ(θ)，推不出每次触发时刻）"
	case signal.FidelityEvent:
		return "事件级（回放真实触发点，精确到秒）"
	default:
		return level
	}
}

// signalParamField 一个可对比的参数字段：生产配置键 + 回测字段名 + 取值器。
// 用显式表而不是反射：这张表同时是"批量扫描能改哪些旋钮"的清单，也是页面上
// 参数 diff 的列名来源，写死比反射出来的 Go 字段名更贴近运维的语言。
type signalParamField struct {
	Key     string // 生产配置键
	Field   string // 回测参数字段名（与 signal.Params 的 json tag 一致）
	Numeric bool
	Value   func(signal.Params) string
	Number  func(signal.Params) float64
}

func numField(key, field string, get func(signal.Params) float64, format string) signalParamField {
	return signalParamField{
		Key: key, Field: field, Numeric: true,
		Value:  func(p signal.Params) string { return fmt.Sprintf(format, get(p)) },
		Number: get,
	}
}

func enumField(key, field string, get func(signal.Params) string) signalParamField {
	return signalParamField{
		Key: key, Field: field, Numeric: false,
		Value:  get,
		Number: func(signal.Params) float64 { return 0 },
	}
}

// signalParamFields 参与 diff 的全部字段。顺序即页面上 diff 的展示顺序：
// 先形态、再仓位、再门控、再移动止盈、最后阈值与回放口径。
func signalParamFields() []signalParamField {
	return []signalParamField{
		enumField("(回测)mode", "mode", func(p signal.Params) string { return p.Mode }),
		numField("trade.accountN.order_size", "orderSize", func(p signal.Params) float64 { return float64(p.OrderSize) }, "%.0f"),
		numField("trade.accountN.risk_equity", "riskEquity", func(p signal.Params) float64 { return p.RiskEquity }, "%.2f"),
		numField("(回测)cap_override", "capOverride", func(p signal.Params) float64 { return float64(p.CapOverride) }, "%.0f"),
		numField("position.risk.budget_pct", "budgetPct", func(p signal.Params) float64 { return p.BudgetPct }, "%.2f"),
		numField("position.monitor.catastrophe_stop_pct", "catastropheStopPct", func(p signal.Params) float64 { return p.CatastropheStopPct }, "%.0f"),
		numField("position.risk.max_contracts_ceiling", "ceiling", func(p signal.Params) float64 { return float64(p.Ceiling) }, "%.0f"),
		numField("position.risk.contract_face", "faceValue", func(p signal.Params) float64 { return p.FaceValue }, "%.6f"),
		numField("(回测)catastrophe_overshoot_roi_pts", "catastropheOvershootRoiPts", func(p signal.Params) float64 { return p.CatastropheOvershootRoiPts }, "%.2f"),
		numField("trade.accountN.reverse_gate_min_profit_pct", "gateMinProfitPct", func(p signal.Params) float64 { return p.GateMinProfitPct }, "%.2f"),
		numField("trade.trend_gate.window_hours", "trendGateWindowHours", func(p signal.Params) float64 { return p.TrendGateWindowHours }, "%.2f"),
		numField("trade.trend_gate.threshold_pct", "trendGateThresholdPct", func(p signal.Params) float64 { return p.TrendGateThresholdPct }, "%.2f"),
		numField("position.monitor.trail.tier_small_ratio", "tierSmallRatio", func(p signal.Params) float64 { return p.TierSmallRatio }, "%.3f"),
		numField("position.monitor.trail.tier_large_ratio", "tierLargeRatio", func(p signal.Params) float64 { return p.TierLargeRatio }, "%.3f"),
		numField("position.monitor.trail.small_activate", "smallActivatePct", func(p signal.Params) float64 { return p.SmallActivatePct }, "%.2f"),
		numField("position.monitor.trail.small_giveback", "smallGiveback", func(p signal.Params) float64 { return p.SmallGiveback }, "%.3f"),
		numField("position.monitor.trail.medium_activate", "mediumActivatePct", func(p signal.Params) float64 { return p.MediumActivatePct }, "%.2f"),
		numField("position.monitor.trail.medium_giveback", "mediumGiveback", func(p signal.Params) float64 { return p.MediumGiveback }, "%.3f"),
		numField("position.monitor.trail.large_activate", "largeActivatePct", func(p signal.Params) float64 { return p.LargeActivatePct }, "%.2f"),
		numField("position.monitor.trail.large_giveback", "largeGiveback", func(p signal.Params) float64 { return p.LargeGiveback }, "%.3f"),
		numField("(回测)taker_fee", "takerFee", func(p signal.Params) float64 { return p.TakerFee }, "%.6f"),
		numField("monitor.symbols.<SYM>.signal_threshold(bp)", "signalThresholdBp", func(p signal.Params) float64 { return p.SignalThresholdBp }, "%.2f"),
		enumField("(回测)eval_mode", "evalMode", func(p signal.Params) string { return p.EvalMode }),
		enumField("(回测)entry_px", "entryPx", func(p signal.Params) string { return p.EntryPx }),
	}
}

// DiffSignalParams 逐字段比较两组参数，只返回有差异的字段。
// 两侧都先 Normalize：否则"没传"与"传了缺省值"会被算成差异。
func DiffSignalParams(baseline, target signal.Params) []tradeDTO.SignalBacktestParamDiffDTO {
	baseline, target = baseline.Normalize(), target.Normalize()
	out := make([]tradeDTO.SignalBacktestParamDiffDTO, 0, 4)
	for _, f := range signalParamFields() {
		bv, tv := f.Value(baseline), f.Value(target)
		if bv == tv {
			continue
		}
		row := tradeDTO.SignalBacktestParamDiffDTO{
			Key: f.Key, Field: f.Field, Baseline: bv, Value: tv, Numeric: f.Numeric,
		}
		if f.Numeric {
			row.Delta = f.Number(target) - f.Number(baseline)
		}
		out = append(out, row)
	}
	return out
}

// AutoGroupLabel 按与基线的差异自动生成组标签（用户没给 label 时）。
// 只取前两处差异，够长的组名反而看不清；全同基线时明说 same_as_baseline——
// 那通常是提交时漏填了旋钮，不该显示成一个匿名组。
func AutoGroupLabel(diff []tradeDTO.SignalBacktestParamDiffDTO) string {
	if len(diff) == 0 {
		return "same_as_baseline"
	}
	parts := make([]string, 0, 2)
	for i, d := range diff {
		if i >= 2 {
			parts = append(parts, fmt.Sprintf("+%d项", len(diff)-2))
			break
		}
		parts = append(parts, d.Field+"="+trimTrailingZeros(d.Value))
	}
	return strings.Join(parts, ",")
}

// trimTrailingZeros 去掉小数末尾的无意义 0（"0.350" → "0.35"，"400.00" → "400"）。
// 只对**带小数点**的串生效：整数串直接 TrimRight("0") 会把 40 剪成 4。
func trimTrailingZeros(v string) string {
	if !strings.Contains(v, ".") {
		return v
	}
	return strings.TrimSuffix(strings.TrimRight(strings.TrimRight(v, "0"), "."), ".")
}

// diffMetrics 指标差异。全部取自 trade_backtest_metric 既有列，不新造指标。
func diffMetrics(baseline, target *tradeRepository.TradeBacktestMetric) *tradeDTO.SignalBacktestMetricDiffDTO {
	d := &tradeDTO.SignalBacktestMetricDiffDTO{
		NetPnl:       target.NetPnl - baseline.NetPnl,
		WinRate:      target.WinRate - baseline.WinRate,
		ProfitFactor: target.ProfitFactor - baseline.ProfitFactor,
		MaxDrawdown:  target.MaxDrawdown - baseline.MaxDrawdown,
		Sharpe:       target.Sharpe - baseline.Sharpe,
		TradeCount:   target.TradeCount - baseline.TradeCount,
		TrailCount:   target.TrailCount - baseline.TrailCount,
		SlCount:      target.SlCount - baseline.SlCount,
		ReduceClose:  target.ReduceCloseCount - baseline.ReduceCloseCount,
		EodOpen:      target.EodOpenCount - baseline.EodOpenCount,
		MaxStack:     target.MaxStack - baseline.MaxStack,
	}
	// 基线净利为 0 或反号时百分比变化没有意义（−10 → +5 说成 "−150%" 只会误导），
	// 一律留 0 让前端只显示绝对差。
	if baseline.NetPnl > 0 && target.NetPnl > 0 {
		d.NetPnlPct = (target.NetPnl - baseline.NetPnl) / baseline.NetPnl * 100
	}
	return d
}

// pnlAvailable 本组是否产出了可比的 PnL。
//
// 降低阈值的频率级组在 signal.Replay 里直接早返回：SignalReplayed=0、
// 没有任何 episode，只有 λ(θ)。它的 netPnl=0 不是"不赚不亏"，是"没算"。
func pnlAvailable(m *tradeRepository.TradeBacktestMetric) bool {
	if m == nil {
		return false
	}
	return m.SignalCount > 0 || m.TradeCount > 0
}

// buildComparison 把一个批次的全部 run + metric 组装成按精度分组的对比矩阵。
//
// baselineParams 是批次冻结的基线快照（不是"现在再读一次生产配置"）；
// baselineRun/baselineMetric 为 nil 表示基线组没跑或没跑完，此时只出参数 diff。
func buildComparison(
	baselineParams signal.Params,
	runs []*tradeRepository.TradeBacktestRun,
	metricByRun map[int64]*tradeRepository.TradeBacktestMetric,
	baselineRunID int64,
) ([]tradeDTO.SignalBacktestFidelityGroupDTO, []string) {

	var warnings []string
	baselineMetric := metricByRun[baselineRunID]
	baselineFidelity := signal.Classify(baselineParams).Level
	for _, r := range runs {
		if int64(r.Id) == baselineRunID {
			if r.Fidelity != "" {
				baselineFidelity = r.Fidelity
			}
			if r.Status != "done" {
				warnings = append(warnings, fmt.Sprintf(
					"基线组 run=%d 状态为 %s：本批次只能给出参数差异，指标差异要等基线跑完", r.Id, r.Status))
			}
		}
	}
	if baselineRunID == 0 {
		warnings = append(warnings,
			"本批次没有基线组（includeBaselineRun=false）：只有参数差异，没有指标差异")
	} else if baselineMetric == nil {
		warnings = append(warnings, "基线组还没有汇总指标：指标差异暂不可用")
	}

	byFidelity := map[string][]tradeDTO.SignalBacktestComparisonRowDTO{}
	noteSet := map[string]map[string]bool{}
	for _, run := range runs {
		params, err := signalParamsFromSnapshot(run.ParamsSnapshot)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("run=%d 参数快照解析失败，该组只按状态展示: %v", run.Id, err))
			params = signal.DefaultParams()
		}
		level := run.Fidelity
		if level == "" {
			level = signal.Classify(params).Level
		}
		metric := metricByRun[int64(run.Id)]
		row := tradeDTO.SignalBacktestComparisonRowDTO{
			RunID:        int64(run.Id),
			GroupLabel:   run.GroupLabel,
			IsBaseline:   int64(run.Id) == baselineRunID,
			Status:       run.Status,
			ErrorMsg:     run.ErrorMsg,
			Fidelity:     level,
			ParamDiff:    DiffSignalParams(baselineParams, params),
			PnlAvailable: pnlAvailable(metric),
		}
		if row.GroupLabel == "" {
			row.GroupLabel = AutoGroupLabel(row.ParamDiff)
		}
		if metric != nil {
			dto := backtestMetricToDTO(metric)
			row.Metric = &dto
		}
		row.MetricDiff, row.DiffBlockedReason = metricDiffOrReason(
			run, level, baselineFidelity, baselineRunID, baselineMetric, metric)

		byFidelity[level] = append(byFidelity[level], row)
		if noteSet[level] == nil {
			noteSet[level] = map[string]bool{}
		}
		for _, n := range strings.Split(run.FidelityNote, " | ") {
			if n = strings.TrimSpace(n); n != "" {
				noteSet[level][n] = true
			}
		}
	}

	// 精度组的固定顺序：事件级在前（可信度高的先看），频率级在后，其余按字典序。
	levels := make([]string, 0, len(byFidelity))
	for level := range byFidelity {
		levels = append(levels, level)
	}
	sort.Slice(levels, func(i, j int) bool { return fidelityRank(levels[i]) < fidelityRank(levels[j]) })

	groups := make([]tradeDTO.SignalBacktestFidelityGroupDTO, 0, len(levels))
	for _, level := range levels {
		rows := byFidelity[level]
		sortedBy := sortComparisonRows(rows)
		notes := make([]string, 0, len(noteSet[level]))
		for n := range noteSet[level] {
			notes = append(notes, n)
		}
		sort.Strings(notes)
		groups = append(groups, tradeDTO.SignalBacktestFidelityGroupDTO{
			Fidelity:             level,
			FidelityLabel:        fidelityLabel(level),
			ComparableToBaseline: baselineRunID > 0 && level == baselineFidelity,
			SortedBy:             sortedBy,
			Notes:                notes,
			Rows:                 rows,
		})
	}
	if len(groups) > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"本批次含 %d 个精度等级的结果：不同精度组之间不得比大小，排序只在组内有效（需求大纲 §3.3）", len(groups)))
	}
	return groups, warnings
}

// metricDiffOrReason 算指标差异，或者说清为什么算不了。
// 顺序即优先级：本组没跑完 > 缺基线 > 精度不同 > 本组不产 PnL。
func metricDiffOrReason(
	run *tradeRepository.TradeBacktestRun,
	level, baselineFidelity string,
	baselineRunID int64,
	baselineMetric, metric *tradeRepository.TradeBacktestMetric,
) (*tradeDTO.SignalBacktestMetricDiffDTO, string) {

	if int64(run.Id) == baselineRunID {
		return nil, "本行即基线"
	}
	if run.Status != "done" || metric == nil {
		return nil, "本组尚未产出汇总指标（status=" + run.Status + "）"
	}
	if baselineRunID == 0 || baselineMetric == nil {
		return nil, "基线组无指标，无法计算差异"
	}
	if level != baselineFidelity {
		return nil, fmt.Sprintf("本组精度为 %s、基线为 %s：跨精度不得比大小，只能在同精度组内相互比较",
			level, baselineFidelity)
	}
	if !pnlAvailable(metric) {
		return nil, "本组只产出 λ(θ)、不产出 PnL（降低阈值无法从事件流补出新触发点），指标差异不成立"
	}
	return diffMetrics(baselineMetric, metric), ""
}

func fidelityRank(level string) int {
	switch level {
	case signal.FidelityEvent:
		return 0
	case signal.FidelityFrequency:
		return 1
	default:
		return 2
	}
}

// sortComparisonRows 组内排序，返回排序依据。
//
// 基线行永远置顶（它是参照物，不参与"谁更好"的排名）；其余按净利降序。
// 整组都没有 PnL（典型是降阈值的频率级组）时改按 λ(θ) 降序——那才是这类组
// 唯一有信息量的输出。未完成的行沉到末尾，避免空指标混在有效结果中间。
func sortComparisonRows(rows []tradeDTO.SignalBacktestComparisonRowDTO) string {
	anyPnl := false
	for _, r := range rows {
		if !r.IsBaseline && r.PnlAvailable {
			anyPnl = true
			break
		}
	}
	sortedBy := "netPnl"
	if !anyPnl {
		sortedBy = "lambdaPerDay"
	}
	key := func(r tradeDTO.SignalBacktestComparisonRowDTO) float64 {
		if r.Metric == nil {
			return 0
		}
		if sortedBy == "lambdaPerDay" {
			return r.Metric.LambdaPerDay
		}
		return r.Metric.NetPnl
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.IsBaseline != b.IsBaseline {
			return a.IsBaseline
		}
		aReady, bReady := a.Metric != nil && a.Status == "done", b.Metric != nil && b.Status == "done"
		if aReady != bReady {
			return aReady
		}
		if ka, kb := key(a), key(b); ka != kb {
			return ka > kb
		}
		return a.RunID < b.RunID
	})
	return sortedBy
}
