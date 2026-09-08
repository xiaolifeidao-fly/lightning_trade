package trade

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy/signal"

	"argus_single/pkg/eventstore"
	argusMonitor "argus_single/pkg/monitor"

	"github.com/sirupsen/logrus"
)

// 本文件是盘口信号回测（r8）的编排层：把 strategy_event 的真实触发流 +
// trade_kline 的 1m 路径喂给 strategy/signal 引擎，再把结果落进既有的
// trade_backtest_run / _trade / _metric 三张表（engine_kind/calc_mode=signal）。
//
// 与预测驱动回测（backtest.go）完全并行、互不影响：共用表、不共用代码路径。

const (
	// EngineKindPrediction / EngineKindSignal 落在 trade_backtest_run.engine_kind 上。
	// 既有行没有这一列，AutoMigrate 按 default 回填成 prediction。
	EngineKindPrediction = "prediction"
	EngineKindSignal     = "signal"

	// CalcModeSignal 信号驱动的结算口径标识（trade_backtest_trade/_metric.calc_mode）。
	CalcModeSignal = "signal"

	// SignalSourceStrategyEvent 信号来源：唯一合法值。信号侧零推导——
	// 不从 K 线、不从 Telegram 导出、不从任何派生表取触发点。
	SignalSourceStrategyEvent = "strategy_event"

	// DefaultSignalPlatform 1m 路径回放默认用 DeepCoin：信号源与下单都在
	// DeepCoin，用币安价回放会引入跨所基差。
	DefaultSignalPlatform = "deepcoin"
)

// CreateSignalBacktestRun 校验并创建一次盘口信号回测，落库后异步回放，立即返回 runId。
func (s *TradeService) CreateSignalBacktestRun(dto tradeDTO.CreateSignalBacktestRunDTO) (int64, error) {
	start, err := parseSignalWindowTime(dto.StartTime)
	if err != nil {
		return 0, fmt.Errorf("startTime: %w", err)
	}
	end, err := parseSignalWindowTime(dto.EndTime)
	if err != nil {
		return 0, fmt.Errorf("endTime: %w", err)
	}
	if !end.After(start) {
		return 0, fmt.Errorf("endTime 必须晚于 startTime")
	}
	if strings.TrimSpace(dto.InstanceKey) == "" {
		return 0, fmt.Errorf("instanceKey 必填：三个实例写同一张事件表，不带实例维度会把不同实验体的触发流混成一条")
	}
	if strings.TrimSpace(dto.AccountLabel) == "" {
		return 0, fmt.Errorf("accountLabel 必填：同一实例可能有多个账户（champion/challenger），少这一维会把两本仓算成一本")
	}

	params := paramsFromSignalDTO(dto)
	if err := params.Validate(); err != nil {
		// 校验器就是实盘的 trade.ValidateRiskParams：回测拒绝的参数集与实盘
		// 启动 fail-fast 拒绝的完全一致。
		return 0, fmt.Errorf("参数非法（口径同实盘启动校验）: %w", err)
	}
	// 冻结参数快照：结果可复现的唯一凭据。存 Normalize 后的完整参数集，
	// 而不是请求体——否则"没传的字段用了什么缺省"事后无从追溯。
	snapshot, err := json.Marshal(params.Normalize())
	if err != nil {
		return 0, err
	}
	fid := signal.Classify(params)

	platform := strings.TrimSpace(dto.PlatformCode)
	if platform == "" {
		platform = DefaultSignalPlatform
	}
	symbol := strings.ToUpper(strings.TrimSpace(dto.Symbol))
	if symbol == "" {
		symbol = "BTCUSDT"
	}

	run := &tradeRepository.TradeBacktestRun{
		Name:           dto.Name,
		PlatformCode:   platform,
		CoinCode:       strings.ToUpper(strings.TrimSpace(dto.CoinCode)),
		Symbol:         symbol,
		PriceInterval:  "1m",
		PriceSource:    platform,
		StartTime:      start,
		EndTime:        end,
		ParamsSnapshot: string(snapshot),
		Status:         "pending",
		EngineKind:     EngineKindSignal,
		InstanceKey:    strings.TrimSpace(dto.InstanceKey),
		AccountLabel:   strings.TrimSpace(dto.AccountLabel),
		SignalSource:   SignalSourceStrategyEvent,
		Fidelity:       fid.Level,
		FidelityNote:   truncate(fid.Note(), 1024),
	}
	if err := s.tradeBacktestRunRepository.CreateRun(run); err != nil {
		return 0, err
	}

	runID := int64(run.Id)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[signal-backtest] run=%d panic: %v", runID, r)
				_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", fmt.Sprintf("panic: %v", r))
			}
		}()
		if err := s.RunSignalBacktest(runID); err != nil {
			logrus.Warnf("[signal-backtest] run=%d 执行失败: %v", runID, err)
		}
	}()
	return runID, nil
}

// RunSignalBacktest 执行一次盘口信号回测：取真实触发流 + 1m 路径 → 回放 → 落库。
func (s *TradeService) RunSignalBacktest(runID int64) error {
	run, err := s.tradeBacktestRunRepository.FindRunByID(runID)
	if err != nil {
		return err
	}
	if run.EngineKind != EngineKindSignal {
		return fmt.Errorf("run=%d 不是盘口信号回测（engine_kind=%q），请走 RunBacktest", runID, run.EngineKind)
	}
	_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "running", "")

	res, trades, metric, err := s.computeSignalBacktest(run)
	if err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
		return err
	}
	_ = s.tradeBacktestRunRepository.UpdateRunKlineInfo(runID, run.KlineCount, run.KlineStart, run.KlineEnd)
	if err := s.tradeBacktestRunRepository.UpdateSignalRunResult(runID, res.Fidelity.Level,
		truncate(res.Fidelity.Note(), 1024), res.SignalReplayed); err != nil {
		logrus.Warnf("[signal-backtest] run=%d 回填精度信息失败: %v", runID, err)
	}
	if len(trades) > 0 {
		if err := s.tradeBacktestTradeRepository.BatchCreate(trades); err != nil {
			_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
			return err
		}
	}
	if err := s.tradeBacktestMetricRepository.UpsertMetrics([]*tradeRepository.TradeBacktestMetric{metric}); err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", truncate(err.Error(), 500))
		return err
	}
	return s.tradeBacktestRunRepository.UpdateRunStatus(runID, "done", "")
}

// signalWindow 一次取数所需的全部维度。单次回测从 run 上取，批量扫描（r11）
// 从 batch 上取——两条路径共用同一个取数函数，才能保证"批量里的某一组单独重跑
// 结果一致"。
type signalWindow struct {
	InstanceKey  string
	AccountLabel string
	Symbol       string
	PlatformCode string
	Start        time.Time
	End          time.Time
}

// signalDataset 一次回放的输入数据。它与参数无关，所以批量扫描只取一次、
// 全部参数组共享——既省掉 N−1 次全表扫，更重要的是让所有组吃到**逐字节同一份**
// 信号流：事件双写是持续在写的，分别取 N 次会让先跑完的组和后跑的组看到不同
// 的触发集，那样横向对比的差异里就混进了数据差异。
type signalDataset struct {
	Signals    []signal.Signal
	Bars       []signal.Bar
	Seed       signal.SeedPosition
	DevWindows []signal.DevWindow // 只有需要推 λ(θ) 时才非空

	KlineCount int
	KlineStart *time.Time
	KlineEnd   *time.Time
}

// loadSignalDataset 取一次回放所需的信号流与 1m 路径。needDevSample=true 时
// 顺带取 dev_sample（推 λ(θ) 用）。
func (s *TradeService) loadSignalDataset(w signalWindow, needDevSample bool) (*signalDataset, error) {
	// ① 信号流：按 instance_key + account_label 过滤，四类事件全取
	//    （被拦截的触发同样是真实触发，漏掉它们会让触发数对不上）。
	filter := tradeRepository.SignalEventFilter{
		InstanceKey:  w.InstanceKey,
		AccountLabel: w.AccountLabel,
		Instrument:   w.Symbol,
		Start:        w.Start,
		End:          w.End,
		Events:       signal.SignalEventTypes(),
	}
	rows, err := s.strategyEventRepository.ListEvents(filter)
	if err != nil {
		return nil, fmt.Errorf("取 strategy_event 失败: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("区间内没有 %s/%s 的触发事件：先确认 argus_single 的事件双写已上线并覆盖该区间",
			w.InstanceKey, w.AccountLabel)
	}
	// 零丢失零重复的第一道对账：仓储的 COUNT 与取回的行数必须一致。
	if total, err := s.strategyEventRepository.CountEvents(filter); err == nil && total != int64(len(rows)) {
		return nil, fmt.Errorf("事件数对账失败: COUNT=%d 但取回 %d 行", total, len(rows))
	}
	signals := signalsFromEvents(rows)

	// ② 1m 路径：只用于持仓期间回放。右侧不需要额外 pad——持仓在窗口末尾
	//    未平就记 eod 并按最后一根收盘标记浮动，不假装它已经平了。
	klines, err := s.tradeKlineRepository.ListBySymbolIntervalTimeRange(
		w.PlatformCode, w.Symbol, "1m", w.Start, w.End)
	if err != nil {
		return nil, fmt.Errorf("取 1m K 线失败: %w", err)
	}
	bars := barsFromKlines(klines)
	if len(bars) == 0 {
		return nil, fmt.Errorf("区间内 %s %s 没有 1m K 线：先跑 K 线回填（POST /klines/backfill）",
			w.PlatformCode, w.Symbol)
	}
	ds := &signalDataset{
		Signals:    signals,
		Bars:       bars,
		Seed:       signal.SeedFromSignals(signals),
		KlineCount: len(klines),
	}
	st, en := klines[0].OpenTime, klines[len(klines)-1].OpenTime
	ds.KlineStart, ds.KlineEnd = &st, &en

	// ③ 频率级路径才需要 dev_sample（推 λ(θ)）；事件级不查，省一次全表扫。
	if needDevSample {
		samples, err := s.devSampleRepository.ListSamples(w.InstanceKey, w.Symbol, w.Start, w.End)
		if err != nil {
			logrus.Warnf("[signal-backtest] %s/%s 取 dev_sample 失败，λ(θ) 将缺失: %v",
				w.InstanceKey, w.AccountLabel, err)
		} else {
			ds.DevWindows = devWindowsFromSamples(samples)
		}
	}
	return ds, nil
}

// replaySignalDataset 用一份已取好的数据跑一组参数。批量扫描的每一组都走这里，
// 单次回测也走这里——同一份代码，避免两条路径口径漂移。
func replaySignalDataset(ds *signalDataset, params signal.Params) (*signal.Result, error) {
	return signal.Replay(signal.Input{
		Params:     params,
		Signals:    ds.Signals,
		Bars:       ds.Bars,
		Seed:       ds.Seed,
		DevWindows: ds.DevWindows,
	})
}

// computeSignalBacktest 纯编排：拉数据 → 回放 → 转实体。不落库，便于单测与复用。
func (s *TradeService) computeSignalBacktest(run *tradeRepository.TradeBacktestRun) (
	*signal.Result, []*tradeRepository.TradeBacktestTrade, *tradeRepository.TradeBacktestMetric, error) {

	params, err := signalParamsFromSnapshot(run.ParamsSnapshot)
	if err != nil {
		return nil, nil, nil, err
	}
	ds, err := s.loadSignalDataset(signalWindow{
		InstanceKey:  run.InstanceKey,
		AccountLabel: run.AccountLabel,
		Symbol:       run.Symbol,
		PlatformCode: run.PlatformCode,
		Start:        run.StartTime,
		End:          run.EndTime,
	}, signal.Classify(params).ThresholdChanged)
	if err != nil {
		return nil, nil, nil, err
	}
	run.KlineCount, run.KlineStart, run.KlineEnd = ds.KlineCount, ds.KlineStart, ds.KlineEnd

	res, err := replaySignalDataset(ds, params)
	if err != nil {
		return nil, nil, nil, err
	}

	runID := int64(run.Id)
	metric := signalMetricEntity(runID, signal.Aggregate(res))
	trades := signalTradeEntities(runID, params, res)
	return res, trades, metric, nil
}

// ─── 转换 ────────────────────────────────────────────────────────────────────

// signalsFromEvents 事件行 → 引擎输入。**零推导**：每个字段都是事件上原样的值。
//
// 一个已知的口径事实：open 事件不带 avgPx/lastPx（只有反向减仓锁利那一支会带，
// 见 pkg/trade/manager.go 的 openEv 组装），所以成交均价不可能从 open 事件取。
// 引擎因此自己累积净仓均价，真实均价只出现在平仓类事件上（trailing_close /
// catastrophe_stop 的 avgPx），那是校准对照用的，不是回放输入。
func signalsFromEvents(rows []*eventstore.StrategyEvent) []signal.Signal {
	out := make([]signal.Signal, 0, len(rows))
	for _, r := range rows {
		s := signal.Signal{
			Ts:        r.Ts,
			Side:      strings.ToLower(strings.TrimSpace(derefStr(r.Side))),
			Event:     r.Event,
			OrderSize: derefInt(r.OrderSize),
			GapBp:     derefFloat(r.GapBp),
			SigLast:   derefFloat(r.SigLast),
			SigMark:   derefFloat(r.SigMark),
			NetSide:   strings.ToLower(strings.TrimSpace(derefStr(r.NetSide))),
			NetSize:   derefInt(r.Size),
			AvgPx:     derefFloat(r.AvgPx),
			LastPx:    derefFloat(r.LastPx),
			Reason:    derefStr(r.Reason),
			GateKind:  derefStr(r.GateKind),
		}
		if s.Side == "" || s.Ts.IsZero() {
			continue // side 缺失的行无法回放；计数差额会在 SignalDropped 里体现
		}
		out = append(out, s)
	}
	return out
}

func barsFromKlines(rows []*tradeRepository.TradeKline) []signal.Bar {
	out := make([]signal.Bar, 0, len(rows))
	for _, k := range rows {
		out = append(out, signal.Bar{
			Ts: k.OpenTime, Open: k.OpenPrice, High: k.HighPrice, Low: k.LowPrice, Close: k.ClosePrice,
		})
	}
	return out
}

// devWindowsFromSamples dev_sample 行 → λ 推导输入。devCross 的 key 是阈值(bp)
// 的字符串，跟 monitor.DevSampleThresholdsBp 一一对应。
func devWindowsFromSamples(rows []*eventstore.DevSample) []signal.DevWindow {
	out := make([]signal.DevWindow, 0, len(rows))
	for _, r := range rows {
		w := signal.DevWindow{Ts: r.Ts, Ticks: r.DevTicks}
		if r.DevCrossJson != nil && *r.DevCrossJson != "" {
			raw := map[string]int{}
			if err := json.Unmarshal([]byte(*r.DevCrossJson), &raw); err == nil {
				w.Cross = make(map[float64]int, len(raw))
				for k, v := range raw {
					var th float64
					if _, err := fmt.Sscanf(k, "%g", &th); err == nil && th > 0 {
						w.Cross[th] += v
					}
				}
			}
		}
		if len(w.Cross) == 0 {
			continue
		}
		out = append(out, w)
	}
	return out
}

// signalTradeEntities 把每个持仓生命周期落成一行逐笔明细。
func signalTradeEntities(runID int64, p signal.Params, res *signal.Result) []*tradeRepository.TradeBacktestTrade {
	out := make([]*tradeRepository.TradeBacktestTrade, 0, len(res.Episodes))
	for _, ep := range res.Episodes {
		opened := ep.OpenedAt
		closed := ep.ClosedAt
		status := "closed"
		if ep.Open {
			status = "open"
		}
		// eod（窗口结束仍持仓）行的口径：ClosePrice 是最后一根收盘的标记价，
		// Pnl 是按它算的浮动盈亏，status=open 标明它不是已实现。汇总指标的
		// NetPnl 同样含这部分浮动（Realized − Fees + Floating），两侧口径一致。
		row := &tradeRepository.TradeBacktestTrade{
			RunID:        runID,
			CalcMode:     CalcModeSignal,
			Direction:    ep.Side,
			EntryMode:    "market",
			Status:       status,
			OpenPrice:    ep.AvgPx,
			ClosePrice:   ep.ClosePx,
			CloseReason:  ep.Reason,
			RequestedAt:  opened,
			Pnl:          ep.Pnl + ep.ReducedPnl,
			PnlRate:      ep.RoiPct,
			Fee:          ep.Fee,
			NetPnl:       ep.Pnl + ep.ReducedPnl - ep.Fee,
			Leverage:     float64(p.Leverage),
			Contracts:    ep.Contracts,
			MaxContracts: ep.MaxContracts,
			AddCount:     ep.AddCount,
			PeakPct:      ep.PeakPct,
			ReducedPnl:   ep.ReducedPnl,
		}
		if !opened.IsZero() {
			row.OpenedAt = &opened
		}
		if !closed.IsZero() && !ep.Open {
			row.ClosedAt = &closed
		}
		out = append(out, row)
	}
	return out
}

func signalMetricEntity(runID int64, m signal.Metric) *tradeRepository.TradeBacktestMetric {
	return &tradeRepository.TradeBacktestMetric{
		RunID:    runID,
		CalcMode: CalcModeSignal,
		// 通用列：TradeCount 在信号口径下是"持仓生命周期数"，不是信号数
		// （信号数在 SignalCount 列，两者口径不同不能混用）。
		TradeCount:   m.TradeCount,
		FillCount:    m.TradeCount,
		WinCount:     m.WinCount,
		WinRate:      m.WinRate,
		GrossPnl:     m.GrossPnl,
		FeeTotal:     m.FeeTotal,
		NetPnl:       m.NetPnl,
		Expectancy:   m.Expectancy,
		ProfitFactor: m.ProfitFactor,
		MaxDrawdown:  m.MaxDrawdown,
		Sharpe:       m.Sharpe,
		AvgHoldSecs:  m.AvgHoldSecs,
		TrailCount:   m.TrailCount,
		SlCount:      m.SlCount,

		Fidelity:         m.Fidelity,
		FidelityNote:     truncate(m.FidelityNote, 1024),
		SignalCount:      m.SignalCount,
		SignalDropped:    m.SignalDropped,
		SignalFiltered:   m.SignalFiltered,
		CapSkipCount:     m.CapSkipCount,
		GateSkipCount:    m.GateSkipCount,
		TrendSkipCount:   m.TrendSkipCount,
		ReduceCount:      m.ReduceCount,
		ReduceCloseCount: m.ReduceCloseCount,
		EodOpenCount:     m.EodOpenCount,
		MaxStack:         m.MaxStack,
		CapEffective:     m.CapEffective,
		RealizedPnl:      m.RealizedPnl,
		FloatingPnl:      m.FloatingPnl,
		MaxDrawdownPct:   m.MaxDrawdownPct,
		LambdaPerDay:     m.LambdaPerDay,
		LambdaRatio:      m.LambdaRatio,
		LambdaSelfTest:   m.LambdaSelfTest,
	}
}

// applySignalParamKnobs 把请求体里显式给出的旋钮叠加到 base 上，未给出的沿用
// base。base 有两种来源：单次回测用生产缺省（signal.DefaultParams），批量扫描
// 用所选实例的当前生产参数（基线）——后者是"只改一个旋钮看差异"的前提，否则
// 每组都会在十几个字段上同时偏离生产，与基线的 diff 就毫无意义。
func applySignalParamKnobs(p signal.Params, dto tradeDTO.SignalBacktestParamsDTO) signal.Params {
	if dto.Mode != "" {
		p.Mode = dto.Mode
	}
	if dto.OrderSize != nil {
		p.OrderSize = *dto.OrderSize
	}
	if dto.RiskEquity != nil {
		p.RiskEquity = *dto.RiskEquity
	}
	if dto.CapOverride != nil {
		p.CapOverride = *dto.CapOverride
	}
	if dto.BudgetPct != nil {
		p.BudgetPct = *dto.BudgetPct
	}
	if dto.CatastropheStopPct != nil {
		p.CatastropheStopPct = *dto.CatastropheStopPct
	}
	if dto.Ceiling != nil {
		p.Ceiling = *dto.Ceiling
	}
	if dto.CatastropheOvershootRoiPts != nil {
		p.CatastropheOvershootRoiPts = *dto.CatastropheOvershootRoiPts
	}
	if dto.GateMinProfitPct != nil {
		p.GateMinProfitPct = *dto.GateMinProfitPct
	}
	if dto.TrendGateWindowHours != nil {
		p.TrendGateWindowHours = *dto.TrendGateWindowHours
	}
	if dto.TrendGateThresholdPct != nil {
		p.TrendGateThresholdPct = *dto.TrendGateThresholdPct
	}
	if dto.TierSmallRatio != nil {
		p.TierSmallRatio = *dto.TierSmallRatio
	}
	if dto.TierLargeRatio != nil {
		p.TierLargeRatio = *dto.TierLargeRatio
	}
	if dto.SmallActivatePct != nil {
		p.SmallActivatePct = *dto.SmallActivatePct
	}
	if dto.SmallGiveback != nil {
		p.SmallGiveback = *dto.SmallGiveback
	}
	if dto.MediumActivatePct != nil {
		p.MediumActivatePct = *dto.MediumActivatePct
	}
	if dto.MediumGiveback != nil {
		p.MediumGiveback = *dto.MediumGiveback
	}
	if dto.LargeActivatePct != nil {
		p.LargeActivatePct = *dto.LargeActivatePct
	}
	if dto.LargeGiveback != nil {
		p.LargeGiveback = *dto.LargeGiveback
	}
	if dto.TakerFee != nil {
		p.TakerFee = *dto.TakerFee
	}
	if dto.SignalThresholdBp != nil {
		p.SignalThresholdBp = *dto.SignalThresholdBp
	}
	if dto.BaselineThresholdBp != nil {
		p.BaselineThresholdBp = *dto.BaselineThresholdBp
	}
	if dto.EvalMode != "" {
		p.EvalMode = dto.EvalMode
	}
	if dto.EntryPx != "" {
		p.EntryPx = dto.EntryPx
	}
	return p.Normalize()
}

// paramsFromSignalDTO 单次回测的请求体 → 引擎参数：基线是生产缺省。
func paramsFromSignalDTO(dto tradeDTO.CreateSignalBacktestRunDTO) signal.Params {
	return applySignalParamKnobs(signal.DefaultParams(), dto.SignalBacktestParamsDTO)
}

// signalParamsFromSnapshot 从冻结快照还原参数。回放一律读快照而不读请求体，
// 保证同一个 runId 重跑得到同一个结果。
func signalParamsFromSnapshot(snapshot string) (signal.Params, error) {
	p := signal.DefaultParams()
	if strings.TrimSpace(snapshot) == "" {
		return p, fmt.Errorf("params_snapshot 为空：参数未冻结，结果不可复现")
	}
	if err := json.Unmarshal([]byte(snapshot), &p); err != nil {
		return p, fmt.Errorf("params_snapshot 解析失败: %w", err)
	}
	return p.Normalize(), nil
}

// SignalThresholdCandidatesBp 频率级回测可选的候选阈值：与生产采样器的候选
// 列表同源（monitor.DevSampleThresholdsBp），选到列表外的值只能靠插值。
func SignalThresholdCandidatesBp() []float64 {
	return append([]float64(nil), argusMonitor.DevSampleThresholdsBp...)
}

// ListSignalSources 枚举可回测的信号源（实例 × 账户 × 合约 + 覆盖区间）。
func (s *TradeService) ListSignalSources() ([]*tradeRepository.SignalSource, error) {
	return s.strategyEventRepository.ListSignalSources(signal.SignalEventTypes())
}

// parseSignalWindowTime 解析回测窗口时刻。
//
// **不裸时间串按本地时区解析**，与预测驱动回测的 parseTimeFlexible（按 UTC）
// 刻意不同。理由：本引擎的时间锚是 strategy_event.ts，而它是 JSONL 里那串
// 无时区文本按 time.Local 解析出来的瞬时（见 eventstore.ParseTs）——Telegram
// 消息、logs/ 下的全部历史文件、所有离线分析脚本都用这套本地墙钟口径。
// 让人为了查"8/18 00:00 那批触发"先把时间换算成 UTC，是必然出错的设计；
// 而错 8 小时正是需求大纲反复强调的那类极隐蔽错误（数据全在、图能画、
// 只是整段错位）。带偏移量的 RFC3339 仍按其自带偏移解析。
func parseSignalWindowTime(s string) (time.Time, error) {
	v := strings.TrimSpace(s)
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", v, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", v, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02", v, time.Local); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("时间格式应为 RFC3339 或 'YYYY-MM-DD[ HH:mm[:ss]]'（本地时区，与事件 ts 同口径）, got %q", s)
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func derefFloat(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
