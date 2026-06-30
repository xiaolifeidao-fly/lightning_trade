package trade

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"
	"service/trade/strategy"

	"github.com/sirupsen/logrus"
)

// RunBacktest 执行一次回测：读历史预测 + K 线 → 逐条过同一套引擎回放 → 落逐笔明细 + 汇总指标。
// 与实盘复用同一个 strategy 引擎，区别只在 PriceFeed 换成 K 线回放、Sink 写回测表。
// 落库失败/计算失败都会把 run 标记为 failed 并带上原因。
func (s *TradeService) RunBacktest(runID int64) error {
	run, err := s.tradeBacktestRunRepository.FindRunByID(runID)
	if err != nil {
		return err
	}
	_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "running", "")

	trades, metrics, err := s.computeBacktest(run)
	if err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", err.Error())
		return err
	}
	// 回填本次回放实际使用的 K 线覆盖(根数 + 时间区间)，供详情展示。
	_ = s.tradeBacktestRunRepository.UpdateRunKlineInfo(runID, run.KlineCount, run.KlineStart, run.KlineEnd)
	if err := s.tradeBacktestTradeRepository.BatchCreate(trades); err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", err.Error())
		return err
	}
	if err := s.tradeBacktestMetricRepository.UpsertMetrics(metrics); err != nil {
		_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", err.Error())
		return err
	}
	return s.tradeBacktestRunRepository.UpdateRunStatus(runID, "done", "")
}

// CalcModePrediction/Trading 是回测的两种结算口径标识。
const (
	CalcModePrediction = "prediction" // 按预测周期(现状：策略 HoldDuration/MaxHold)
	CalcModeTrading    = "trading"    // 按交易周期：把持仓上限拉长到交易周期，TP/SL 照常
)

// computeBacktest 是回测的纯编排：拉数据 → 逐预测 Plan + 回放 → 聚合，不直接落库(便于单测/复用)。
// 默认按预测周期(现状)算一套；若 run 选了交易周期，再用拉长的持仓上限算第二套(calc_mode=trading)。
func (s *TradeService) computeBacktest(run *tradeRepository.TradeBacktestRun) ([]*tradeRepository.TradeBacktestTrade, []*tradeRepository.TradeBacktestMetric, error) {
	st, err := s.tradeStrategyRepository.FindByID(run.StrategyID)
	if err != nil {
		return nil, nil, fmt.Errorf("策略不存在: %w", err)
	}
	params := tradeRepository.ParamsFromStrategy(st)

	preds, err := s.tradeAIPredictionRepository.ListByCoinIntervalTimeRange(
		run.PlatformCode, run.CoinCode, run.PredictionInterval, run.StartTime, run.EndTime)
	if err != nil {
		return nil, nil, err
	}
	// 两套口径里更长的持仓上限，决定 K 线窗口右侧要多取多久 + 自动补齐覆盖到哪。
	rightPad := backtestRightPad(run, params)
	// 回测前自动补齐窗口 K 线（DB 缺数据时从交易所拉取；best-effort，失败仅告警不中断）。
	s.ensureBacktestKlines(run, rightPad)

	// K 线窗口右侧多取 rightPad，保证临近结束时间的交易也有足够行情走完生命周期。
	klines, err := s.tradeKlineRepository.ListBySymbolIntervalTimeRange(
		run.Symbol, run.PriceInterval, run.StartTime, run.EndTime.Add(rightPad))
	if err != nil {
		return nil, nil, err
	}
	quotes := quotesFromKlines(klines)
	// 记录回放实际使用的 K 线覆盖：根数 + 真实时间区间(klines 已按 open_time 升序)。
	run.KlineCount = len(klines)
	if len(klines) > 0 {
		start := klines[0].OpenTime
		end := klines[len(klines)-1].OpenTime
		run.KlineStart = &start
		run.KlineEnd = &end
	}

	runID := int64(run.Id)

	// 预拉该币种历史压力面(信号时刻之前最近一条)做时间对齐：用于压力面止盈止损，
	// 同时落到每条逐笔(关键阻力/支撑)供回测详情与 K 线详情展示，故无论出场口径都加载。
	pressures := s.loadBacktestPressures(run)

	// 口径一：预测周期(现状)。入场/止盈止损均用预测周期区间，无 band 覆盖。
	predTrades, predOrders, err := s.replayMode(runID, CalcModePrediction, preds, params, quotes, klines, pressures, nil)
	if err != nil {
		return nil, nil, err
	}
	trades := predTrades
	metrics := []*tradeRepository.TradeBacktestMetric{
		tradeRepository.BacktestMetricFromAgg(runID, CalcModePrediction, strategy.Aggregate(predOrders, params)),
	}

	// 口径二：交易周期(可选)。入场仍按预测周期信号，持仓上限拉长到交易周期，
	// 且止盈止损改用交易周期那条预测的区间，使目标贴合持仓周期。
	if dur, ok := intervalDuration(run.TradingPeriod); ok && run.TradingPeriod != "" {
		tp := params
		tp.HoldDuration = dur
		tp.MaxHold = dur
		// 交易周期预测：左移一个交易周期，确保覆盖到每个信号时刻前最近一条。
		tpBand, err := s.tradeAIPredictionRepository.ListByCoinIntervalTimeRange(
			run.PlatformCode, run.CoinCode, run.TradingPeriod, run.StartTime.Add(-dur), run.EndTime)
		if err != nil {
			return nil, nil, err
		}
		tradeTrades, tradeOrders, err := s.replayMode(runID, CalcModeTrading, preds, tp, quotes, klines, pressures, tpBand)
		if err != nil {
			return nil, nil, err
		}
		trades = append(trades, tradeTrades...)
		metrics = append(metrics, tradeRepository.BacktestMetricFromAgg(runID, CalcModeTrading, strategy.Aggregate(tradeOrders, tp)))
	}

	return trades, metrics, nil
}

// replayMode 用给定 params 把所有预测回放一遍，产出该口径的逐笔与订单(供聚合)。
// pressures 非空时(策略按压力面出场)，按信号时刻给每条预测注入最近一次压力面关键位。
// tpBand 非空时(交易周期口径)，按信号时刻给每条预测注入交易周期那条预测的区间，用于止盈止损。
func (s *TradeService) replayMode(
	runID int64, mode string, preds []*tradeRepository.TradeAIPrediction,
	params strategy.Params, quotes []strategy.Quote, klines []*tradeRepository.TradeKline,
	pressures []pressurePoint, tpBand []*tradeRepository.TradeAIPrediction,
) ([]*tradeRepository.TradeBacktestTrade, []*strategy.Order, error) {
	var orders []*strategy.Order
	var trades []*tradeRepository.TradeBacktestTrade
	for _, p := range preds {
		pr := tradeRepository.PredictionFromRow(p)
		signalAt := p.CreatedTime // 信号时刻=预测发起时刻，挂单与回放都从这里起算
		if len(pressures) > 0 {
			pr.KeyResistance, pr.KeySupport = pressureAt(pressures, signalAt)
		}
		if b := nearestPredByCreated(tpBand, signalAt); b != nil {
			pr.TPBandHigh, pr.TPBandLow, pr.TPInvalidation = b.PredictHigh, b.PredictLow, b.Invalidation
		}
		o, ok := strategy.Plan(pr, params, signalAt)
		if !ok {
			continue // 未过开仓门槛，跳过(不计入交易)
		}
		window := quotesAfter(quotes, signalAt)
		feed := strategy.NewSliceFeed(window)
		if err := strategy.Run(context.Background(), &o, feed, strategy.NoopSink{}); err != nil {
			return nil, nil, err
		}
		orders = append(orders, &o)
		trade := tradeRepository.BacktestTradeFromOrder(runID, int64(p.Id), &o, o.Settle(params), pr)
		trade.CalcMode = mode
		trade.Leverage = params.Leverage // 冗余杠杆，供详情展示含杠杆浮盈
		// 冗余预测目标时刻：与信号时刻一起框定本笔对应预测覆盖的时间窗(预测周期)，供详情列表展示。
		if !p.PredictTime.IsZero() {
			pt := p.PredictTime
			trade.PredictTime = &pt
		}
		// 框定本笔实际经历的价格区间：从信号时刻到收尾时刻；未成交(pending/expired)则统计到数据末尾。
		trade.WindowLow, trade.WindowHigh = priceRange(window, o.ClosedAt)
		// 窗口实际开/收盘价(首根开盘、末根收盘) + 预测收盘价，供「预测 vs 实际」对照。
		trade.WindowOpen, trade.WindowClose = klineWindowOpenClose(klines, signalAt, o.ClosedAt)
		trade.PredClose = p.PredictPrice
		trades = append(trades, trade)
	}
	return trades, orders, nil
}

// pressurePoint 一次压力面分析里供出场使用的关键结构位 + 其分析时刻(用于时间对齐)。
type pressurePoint struct {
	at            time.Time
	keyResistance float64
	keySupport    float64
}

// loadBacktestPressures 拉本次回测币种×平台在结束时间前的全部压力面，按分析时刻升序，供逐预测时间对齐。
// best-effort：查询失败仅告警并返回空，出场逻辑会优雅回退到百分比/区间口径。
func (s *TradeService) loadBacktestPressures(run *tradeRepository.TradeBacktestRun) []pressurePoint {
	if s.pressureAnalysisRepository == nil {
		return nil
	}
	rows, err := s.pressureAnalysisRepository.ListByCoinPlatformBefore(run.PlatformCode, run.CoinCode, run.EndTime)
	if err != nil {
		logrus.Warnf("[backtest] run=%d 拉取压力面失败，止盈止损将回退: %v", run.Id, err)
		return nil
	}
	points := make([]pressurePoint, 0, len(rows))
	for _, r := range rows {
		points = append(points, pressurePoint{at: r.AnalyzedTime, keyResistance: r.KeyResistance, keySupport: r.KeySupport})
	}
	return points
}

// pressureAt 返回 signalAt 之前(含)最近一次压力面的关键位；points 按分析时刻升序。无则返回 0,0。
func pressureAt(points []pressurePoint, signalAt time.Time) (keyResistance, keySupport float64) {
	for i := len(points) - 1; i >= 0; i-- {
		if !points[i].at.After(signalAt) {
			return points[i].keyResistance, points[i].keySupport
		}
	}
	return 0, 0
}

// nearestPredByCreated 取 preds 里发起时刻最接近 signalAt(前后都算绝对值最近)的一条；空集返回 nil。
// 与 K 线详情复合方向的锚定口径一致：信号时刻对齐最近一条交易周期预测。
func nearestPredByCreated(preds []*tradeRepository.TradeAIPrediction, signalAt time.Time) *tradeRepository.TradeAIPrediction {
	var best *tradeRepository.TradeAIPrediction
	var bestDiff time.Duration
	for _, p := range preds {
		diff := p.CreatedTime.Sub(signalAt)
		if diff < 0 {
			diff = -diff
		}
		if best == nil || diff < bestDiff {
			best, bestDiff = p, diff
		}
	}
	return best
}

// backtestRightPad 返回两套口径里更长的持仓上限，用于决定 K 线窗口右界与回填覆盖范围。
func backtestRightPad(run *tradeRepository.TradeBacktestRun, params strategy.Params) time.Duration {
	pad := params.MaxHold
	if dur, ok := intervalDuration(run.TradingPeriod); ok && run.TradingPeriod != "" && dur > pad {
		pad = dur
	}
	return pad
}

// quotesFromKlines 把 K 线转成引擎行情：用 High/Low 做触价、Close 作收盘/超时平仓价。
func quotesFromKlines(ks []*tradeRepository.TradeKline) []strategy.Quote {
	out := make([]strategy.Quote, 0, len(ks))
	for _, k := range ks {
		out = append(out, strategy.Quote{
			Time:  k.OpenTime,
			Price: k.ClosePrice,
			High:  k.HighPrice,
			Low:   k.LowPrice,
		})
	}
	return out
}

// priceRange 统计行情切片的最低/最高价：用 K 线 Low/High 取真实区间。
// until 非零时只统计到该时刻(含)，用于把区间收敛到本笔交易实际经历的那一段；
// until 为零(未成交)则统计到切片末尾，反映挂单存活期间价格始终没触及限价。
func priceRange(qs []strategy.Quote, until time.Time) (low, high float64) {
	for _, q := range qs {
		if !until.IsZero() && q.Time.After(until) {
			break
		}
		if low == 0 || q.Low < low {
			low = q.Low
		}
		if q.High > high {
			high = q.High
		}
	}
	return
}

// klineWindowOpenClose 取窗口的实际开/收盘价：[from, until] 内首根 K 线的开盘价、末根的收盘价。
// until 为零(未成交)则统计到 K 线末尾。klines 按 open_time 升序。
func klineWindowOpenClose(klines []*tradeRepository.TradeKline, from, until time.Time) (open, close float64) {
	for _, k := range klines {
		if k.OpenTime.Before(from) {
			continue
		}
		if !until.IsZero() && k.OpenTime.After(until) {
			break
		}
		if open == 0 {
			open = k.OpenPrice // 窗口首根开盘
		}
		close = k.ClosePrice // 持续覆盖到窗口末根收盘
	}
	return
}

// quotesAfter 截取 from 时刻起(含)的行情切片；quotes 按时间升序。
func quotesAfter(quotes []strategy.Quote, from time.Time) []strategy.Quote {
	for i, q := range quotes {
		if !q.Time.Before(from) {
			return quotes[i:]
		}
	}
	return nil
}

// ─── HTTP 服务方法 ────────────────────────────────────────────────────────────

// CreateBacktestRun 校验并创建一次回测任务，落库后异步触发回放计算，立即返回 runId。
// 异步是因为回测可能跑较久；前端拿 runId 后轮询 run.status(pending→running→done/failed)。
func (s *TradeService) CreateBacktestRun(dto tradeDTO.CreateBacktestRunDTO) (int64, error) {
	start, err := parseTimeFlexible(dto.StartTime)
	if err != nil {
		return 0, fmt.Errorf("startTime: %w", err)
	}
	end, err := parseTimeFlexible(dto.EndTime)
	if err != nil {
		return 0, fmt.Errorf("endTime: %w", err)
	}
	if !end.After(start) {
		return 0, fmt.Errorf("endTime 必须晚于 startTime")
	}
	st, err := s.tradeStrategyRepository.FindByID(dto.StrategyID)
	if err != nil {
		return 0, fmt.Errorf("策略不存在: %w", err)
	}
	// 冻结策略参数快照，保证回测结果可复现。
	snapshot, _ := json.Marshal(tradeRepository.ParamsFromStrategy(st))

	priceInterval := dto.PriceInterval
	if priceInterval == "" {
		priceInterval = "1m"
	}
	variant := dto.PredictionVariant
	if variant == "" {
		variant = "raw"
	}
	tradingPeriod := strings.TrimSpace(dto.TradingPeriod)
	if tradingPeriod != "" {
		if _, ok := intervalDuration(tradingPeriod); !ok {
			return 0, fmt.Errorf("不支持的交易周期: %s", tradingPeriod)
		}
	}

	run := &tradeRepository.TradeBacktestRun{
		Name:               dto.Name,
		PlatformCode:       dto.PlatformCode,
		CoinCode:           dto.CoinCode,
		Symbol:             dto.Symbol,
		PredictionInterval: dto.PredictionInterval,
		PredictionVariant:  variant,
		PriceInterval:      priceInterval,
		PriceSource:        dto.PriceSource,
		TradingPeriod:      tradingPeriod,
		StartTime:          start,
		EndTime:            end,
		StrategyID:         dto.StrategyID,
		ParamsSnapshot:     string(snapshot),
		Status:             "pending",
	}
	if err := s.tradeBacktestRunRepository.CreateRun(run); err != nil {
		return 0, err
	}

	runID := int64(run.Id)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorf("[backtest] run=%d panic: %v", runID, r)
				_ = s.tradeBacktestRunRepository.UpdateRunStatus(runID, "failed", fmt.Sprintf("panic: %v", r))
			}
		}()
		if err := s.RunBacktest(runID); err != nil {
			logrus.Warnf("[backtest] run=%d 执行失败: %v", runID, err)
		}
	}()

	return runID, nil
}

// ListBacktestRuns 分页查询回测任务。
func (s *TradeService) ListBacktestRuns(dto tradeDTO.BacktestRunQueryDTO) (*tradeDTO.BacktestRunListDTO, error) {
	rows, total, err := s.tradeBacktestRunRepository.FindRuns(dto.Symbol, dto.StrategyID, dto.Page, dto.PageSize)
	if err != nil {
		return nil, err
	}
	list := make([]tradeDTO.BacktestRunDTO, 0, len(rows))
	for _, r := range rows {
		list = append(list, backtestRunToDTO(r))
	}
	return &tradeDTO.BacktestRunListDTO{Total: total, List: list}, nil
}

// GetBacktestRunDetail 返回单次回测的任务 + 汇总指标 + 逐笔明细。
func (s *TradeService) GetBacktestRunDetail(id int64) (*tradeDTO.BacktestRunDetailDTO, error) {
	run, err := s.tradeBacktestRunRepository.FindRunByID(id)
	if err != nil {
		return nil, err
	}
	detail := &tradeDTO.BacktestRunDetailDTO{Run: backtestRunToDTO(run)}

	if metrics, err := s.tradeBacktestMetricRepository.FindByRuns([]int64{id}); err == nil {
		detail.Metrics = make([]tradeDTO.BacktestMetricDTO, 0, len(metrics))
		for _, m := range metrics {
			detail.Metrics = append(detail.Metrics, backtestMetricToDTO(m))
		}
	}
	trades, err := s.tradeBacktestTradeRepository.FindByRun(id)
	if err != nil {
		return nil, err
	}
	detail.Trades = make([]tradeDTO.BacktestTradeDTO, 0, len(trades))
	for _, t := range trades {
		detail.Trades = append(detail.Trades, backtestTradeToDTO(t))
	}
	// 仍在持仓(open)的逐笔：回测窗口结束时未走完生命周期，用当前最新价标记浮动盈亏。
	s.markOpenBacktestTrades(run, detail.Trades)
	return detail, nil
}

// markOpenBacktestTrades 给仍在持仓(open)的逐笔补上「按当前最新价」的浮动盈亏。
// 这些持仓在回测 K 线窗口内没触发止盈/止损/超时(通常因数据到末尾仍未收尾)，结算口径下盈亏为 0；
// 这里取该 symbol+价格周期已入库的最新一根 K 线收盘价做标记价，算目前的浮动盈亏(含杠杆)。
// best-effort：缺最新价或参数快照解析失败则跳过，不影响其它字段。
func (s *TradeService) markOpenBacktestTrades(run *tradeRepository.TradeBacktestRun, trades []tradeDTO.BacktestTradeDTO) {
	hasOpen := false
	for i := range trades {
		if trades[i].Status == "open" && trades[i].OpenPrice > 0 {
			hasOpen = true
			break
		}
	}
	if !hasOpen {
		return
	}
	latest, err := s.tradeKlineRepository.LatestKline(run.Symbol, run.PriceInterval)
	if err != nil || latest == nil || latest.ClosePrice <= 0 {
		return
	}
	var params strategy.Params
	if err := json.Unmarshal([]byte(run.ParamsSnapshot), &params); err != nil {
		return
	}
	mark := latest.ClosePrice
	for i := range trades {
		t := &trades[i]
		if t.Status != "open" || t.OpenPrice <= 0 {
			continue
		}
		st := strategy.MarkToMarket(strategy.Direction(t.Direction), strategy.EntryMode(t.EntryMode), t.OpenPrice, mark, params)
		t.MarkPrice = mark
		t.UnrealizedPnl = st.Pnl
		t.UnrealizedPnlRate = st.PnlRate
		t.UnrealizedNetPnl = st.NetPnl
	}
}

// GetBacktestMetrics 拉取一批 run 的汇总指标，供前端横向对比「哪个策略有效」。
// 横向对比统一用预测周期口径(基准)，避免一个 run 出现两套指标导致重复行。
func (s *TradeService) GetBacktestMetrics(runIDs []int64) ([]tradeDTO.BacktestMetricDTO, error) {
	rows, err := s.tradeBacktestMetricRepository.FindByRuns(runIDs)
	if err != nil {
		return nil, err
	}
	out := make([]tradeDTO.BacktestMetricDTO, 0, len(rows))
	for _, m := range rows {
		if m.CalcMode != "" && m.CalcMode != CalcModePrediction {
			continue
		}
		out = append(out, backtestMetricToDTO(m))
	}
	return out, nil
}

// ─── K线详情：复合方向 + 预测周期 K 线 ─────────────────────────────────────────

// stdIntervalOrder 标准预测周期序列(由细到粗)，用于推导「大于本周期」的高周期集合。
var stdIntervalOrder = []string{"1h", "4h", "12h", "1d"}

// higherIntervalsOf 返回标准周期里时长大于 own 的那些(如 own=1h → [4h,12h,1d])。
func higherIntervalsOf(own string) []string {
	od, ok := intervalDuration(own)
	if !ok {
		return nil
	}
	var out []string
	for _, it := range stdIntervalOrder {
		if d, ok := intervalDuration(it); ok && d > od {
			out = append(out, it)
		}
	}
	return out
}

// candlesFromPreds 把预测行转成「预测 K 线」：开=参考价 收=预测价 高/低=预测极值。
func candlesFromPreds(rows []*tradeRepository.TradeAIPrediction) []tradeDTO.PredictionCandleDTO {
	out := make([]tradeDTO.PredictionCandleDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, tradeDTO.PredictionCandleDTO{
			OpenTime:   fmtTime(p.CreatedTime),
			CloseTime:  fmtTime(p.PredictTime),
			Open:       p.RefPrice,
			High:       p.PredictHigh,
			Low:        p.PredictLow,
			Close:      p.PredictPrice,
			Trend:      p.Trend,
			Confidence: p.Confidence,
		})
	}
	return out
}

// compositeProfit 按预测方向用预测极值算相对入场价的有利空间%：多看预测高、空看预测低；
// neutral 或极值缺失/落在不利侧 → 0。返回 (利润潜力%, 有利极值)。
func compositeProfit(p *tradeRepository.TradeAIPrediction, entry float64) (profitPct, favorable float64) {
	if entry <= 0 {
		return 0, 0
	}
	switch strings.ToLower(p.Trend) {
	case "long":
		favorable = p.PredictHigh
		if favorable > 0 {
			profitPct = (favorable - entry) / entry * 100
		}
	case "short":
		favorable = p.PredictLow
		if favorable > 0 {
			profitPct = (entry - favorable) / entry * 100
		}
	}
	if profitPct < 0 {
		profitPct = 0
	}
	return profitPct, favorable
}

// GetPredictionDetail 为回测「K线详情」装配预测增强：
//  1. 复合方向——对各高周期(>本周期)取 T 之前最近一条预测，按 利润潜力×置信度 选胜出周期定最终方向；
//  2. 自身周期预测 K 线——窗口 [start,end] 内本周期的预测序列；
//  3. 高周期预测 K 线——各高周期窗口内预测序列 + T 之前最近一条(预测开始之前的那根)。
func (s *TradeService) GetPredictionDetail(q tradeDTO.PredictionDetailQueryDTO) (*tradeDTO.PredictionDetailDTO, error) {
	platform := strings.ToLower(strings.TrimSpace(q.Platform))
	coin := strings.ToUpper(strings.TrimSpace(q.Coin))
	own := strings.TrimSpace(q.Interval)
	if platform == "" || coin == "" || own == "" {
		return nil, fmt.Errorf("platform/coin/interval 不能为空")
	}
	signalT, err := parseTimeFlexible(q.Signal)
	if err != nil {
		return nil, fmt.Errorf("signal: %w", err)
	}
	start, err := parseTimeFlexible(q.Start)
	if err != nil {
		start = signalT // 窗口起缺省回退到信号时刻
	}
	end, err := parseTimeFlexible(q.End)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	entry, _ := strconv.ParseFloat(strings.TrimSpace(q.Entry), 64)

	out := &tradeDTO.PredictionDetailDTO{}

	// 自身周期预测 K 线：created_time ∈ [start,end]。
	ownRows, err := s.tradeAIPredictionRepository.ListByCoinIntervalCreatedRange(platform, coin, own, start, end)
	if err != nil {
		return nil, err
	}
	out.OwnSeries = tradeDTO.PredictionSeriesDTO{Interval: own, Candles: candlesFromPreds(ownRows)}

	comp := tradeDTO.CompositeDirectionDTO{OwnInterval: own}
	// 自身方向：取 T 之前最近一条本周期预测；入场价缺省时也用它的参考价兜底。
	if ownAnchor, _ := s.tradeAIPredictionRepository.FindNearestBefore(platform, coin, own, signalT); ownAnchor != nil {
		comp.OwnDirection = ownAnchor.Trend
		if entry <= 0 {
			entry = ownAnchor.RefPrice
		}
	}
	comp.Entry = entry

	bestScore := -1.0
	for _, hi := range higherIntervalsOf(own) {
		rows, err := s.tradeAIPredictionRepository.ListByCoinIntervalCreatedRange(platform, coin, hi, start, end)
		if err != nil {
			return nil, err
		}

		// 预测 K 线图：前置「预测开始之前的那根」(T 之前最近一条) + 窗口内序列。
		beforeAnchor, _ := s.tradeAIPredictionRepository.FindNearestBefore(platform, coin, hi, signalT)
		series := make([]*tradeRepository.TradeAIPrediction, 0, len(rows)+1)
		if beforeAnchor != nil && (len(rows) == 0 || rows[0].Id != beforeAnchor.Id) {
			series = append(series, beforeAnchor)
		}
		series = append(series, rows...)
		out.HigherSeries = append(out.HigherSeries, tradeDTO.PredictionSeriesDTO{Interval: hi, Candles: candlesFromPreds(series)})

		// 复合方向行：取「时间上最接近 T(绝对值，前后都算)」的那条。
		compAnchor, _ := s.tradeAIPredictionRepository.FindNearestByCreated(platform, coin, hi, signalT)
		row := tradeDTO.CompositeRowDTO{Interval: hi}
		if compAnchor != nil {
			profit, fav := compositeProfit(compAnchor, entry)
			row.HasData = true
			row.Direction = compAnchor.Trend
			row.Confidence = compAnchor.Confidence
			row.PredictTime = fmtTime(compAnchor.PredictTime)
			row.PredLow = compAnchor.PredictLow
			row.PredHigh = compAnchor.PredictHigh
			row.FavorableExtreme = fav
			row.ProfitPct = profit
			row.Score = profit * compAnchor.Confidence
		}
		comp.Rows = append(comp.Rows, row)
	}

	// 利润×置信度最高者胜出定方向(neutral 不计入)。
	for i := range comp.Rows {
		r := comp.Rows[i]
		if r.HasData && strings.ToLower(r.Direction) != "neutral" && r.Score > bestScore {
			bestScore = r.Score
			comp.DominantInterval = r.Interval
			comp.RecommendedDirection = r.Direction
		}
	}
	for i := range comp.Rows {
		comp.Rows[i].Dominant = comp.DominantInterval != "" && comp.Rows[i].Interval == comp.DominantInterval
	}
	comp.Agree = comp.RecommendedDirection != "" && strings.EqualFold(comp.RecommendedDirection, comp.OwnDirection)
	out.Composite = comp

	return out, nil
}

// ─── 映射 ────────────────────────────────────────────────────────────────────

func backtestRunToDTO(r *tradeRepository.TradeBacktestRun) tradeDTO.BacktestRunDTO {
	return tradeDTO.BacktestRunDTO{
		ID:                 int64(r.Id),
		Name:               r.Name,
		PlatformCode:       r.PlatformCode,
		CoinCode:           r.CoinCode,
		Symbol:             r.Symbol,
		PredictionInterval: r.PredictionInterval,
		PredictionVariant:  r.PredictionVariant,
		PriceInterval:      r.PriceInterval,
		PriceSource:        r.PriceSource,
		TradingPeriod:      r.TradingPeriod,
		StartTime:          fmtTime(r.StartTime),
		EndTime:            fmtTime(r.EndTime),
		StrategyID:         r.StrategyID,
		ParamsSnapshot:     r.ParamsSnapshot,
		Status:             r.Status,
		ErrorMsg:           r.ErrorMsg,
		CreatedTime:        fmtTime(r.CreatedTime),
		KlineCount:         r.KlineCount,
		KlineStart:         fmtTimePtr(r.KlineStart),
		KlineEnd:           fmtTimePtr(r.KlineEnd),
	}
}

func backtestTradeToDTO(t *tradeRepository.TradeBacktestTrade) tradeDTO.BacktestTradeDTO {
	return tradeDTO.BacktestTradeDTO{
		ID:                 int64(t.Id),
		PredictionID:       t.PredictionID,
		CalcMode:           t.CalcMode,
		PredictTime:        fmtTimePtr(t.PredictTime),
		Direction:          t.Direction,
		EntryMode:          t.EntryMode,
		PlannedEntryPrice:  t.PlannedEntryPrice,
		TakeProfitPrice:    t.TakeProfitPrice,
		StopLossPrice:      t.StopLossPrice,
		Status:             t.Status,
		OpenPrice:          t.OpenPrice,
		ClosePrice:         t.ClosePrice,
		CloseReason:        t.CloseReason,
		RequestedAt:        fmtTime(t.RequestedAt),
		OpenedAt:           fmtTimePtr(t.OpenedAt),
		ClosedAt:           fmtTimePtr(t.ClosedAt),
		Pnl:                t.Pnl,
		PnlRate:            t.PnlRate,
		NetPnl:             t.NetPnl,
		Fee:                t.Fee,
		Confidence:         t.Confidence,
		Efficiency:         t.Efficiency,
		PredHigh:           t.PredHigh,
		PredLow:            t.PredLow,
		PredClose:          t.PredClose,
		WindowOpen:         t.WindowOpen,
		WindowClose:        t.WindowClose,
		WindowLow:          t.WindowLow,
		WindowHigh:         t.WindowHigh,
		PressureHigh:       t.PressureHigh,
		PressureLow:        t.PressureLow,
		MaxPriceDuringHold: t.MaxPriceDuringHold,
		MinPriceDuringHold: t.MinPriceDuringHold,
		Leverage:           t.Leverage,
	}
}

func backtestMetricToDTO(m *tradeRepository.TradeBacktestMetric) tradeDTO.BacktestMetricDTO {
	return tradeDTO.BacktestMetricDTO{
		RunID:        m.RunID,
		CalcMode:     m.CalcMode,
		TradeCount:   m.TradeCount,
		FillCount:    m.FillCount,
		ExpiredCount: m.ExpiredCount,
		FillRate:     m.FillRate,
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
		TpCount:      m.TpCount,
		SlCount:      m.SlCount,
		TimeoutCount: m.TimeoutCount,
	}
}

// parseTimeFlexible 解析前端时间：优先 RFC3339，回退 "2006-01-02 15:04:05"(按 UTC)。
func parseTimeFlexible(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("时间格式应为 RFC3339 或 'YYYY-MM-DD HH:mm:ss', got %q", s)
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmtTime(*t)
}
