package scheduler

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	newssvc "service/news"
	pressuresvc "service/pressure"
	tradesvc "service/trade"
	tradeRepository "service/trade/repository"

	"oracle/pkg/analyzer"
	"oracle/pkg/collector"
	"oracle/pkg/hub"
	"oracle/pkg/indicator"
	"oracle/pkg/news"
	"oracle/pkg/oraclecfg"
	"oracle/pkg/pressure"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// Scheduler 按 币种×主周期 定时跑 采集→指标→AI→落库。
type Scheduler struct {
	cfg      oraclecfg.Config
	analyzer *analyzer.Analyzer
	news     *news.Collector
	pressure *pressure.Analyzer
	service  *tradesvc.TradeService
	hub      *hub.MarketDataHub
	stop     chan struct{}
	wg       sync.WaitGroup
}

func New(cfg oraclecfg.Config, service *tradesvc.TradeService, newsService *newssvc.NewsService, pressureService *pressuresvc.PressureService) *Scheduler {
	s := &Scheduler{
		cfg:      cfg,
		analyzer: analyzer.New(cfg.AI),
		service:  service,
		hub:      hub.New(cfg, cfg.Coins),
		stop:     make(chan struct{}),
	}
	if cfg.News.Enabled {
		s.news = news.New(cfg.AI, cfg.News, newsService)
	}
	if cfg.Pressure.Enabled {
		s.pressure = pressure.New(pressureAIConfig(cfg), cfg.Pressure, pressureService)
	}
	return s
}

// pressureAIConfig 以 AI 主配置为基准，套用压力面专属覆盖项(模型/超时/token/温度)。
func pressureAIConfig(cfg oraclecfg.Config) oraclecfg.AIConfig {
	ai := cfg.AI
	if v := cfg.Pressure.Model; v != "" {
		ai.Model = v
	}
	if cfg.Pressure.Timeout > 0 {
		ai.Timeout = cfg.Pressure.Timeout
	}
	if cfg.Pressure.MaxTokens > 0 {
		ai.MaxTokens = cfg.Pressure.MaxTokens
	}
	if cfg.Pressure.Temperature > 0 {
		ai.Temperature = cfg.Pressure.Temperature
	}
	return ai
}

// Start 为每个 币种×主周期 起一个独立节奏的 goroutine。
func (s *Scheduler) Start() {
	// 行情数据 hub：先启动，确保预测落库后能立即拿到价格
	s.hub.Start()

	// 消息面：预测前先把各币种消息面拉到缓存，再起慢节奏刷新循环，
	// 让首批预测就能拿到消息面；刷新独立于预测节奏，避免逐次重复联网。
	if s.news != nil {
		s.warmNews()
		for _, coin := range s.cfg.Coins {
			s.wg.Add(1)
			go s.newsLoop(coin)
		}
	}

	for _, coin := range s.cfg.Coins {
		for _, interval := range s.cfg.Intervals {
			s.wg.Add(1)
			go s.runLoop(coin, interval)
		}
	}
	// 结算回填：到期预测取真实价回填误差/方向命中。
	s.wg.Add(1)
	go s.settleLoop()

	// 压力面：按独立节奏(默认10分钟)结合 K 线/指标与消息面分析上下方压力位。
	if s.pressure != nil {
		for _, coin := range s.cfg.Coins {
			s.wg.Add(1)
			go s.pressureLoop(coin)
		}
	}

	// 持仓监测：订阅 hub tick，实时检测 TP/SL/超时，驱动平仓。
	s.wg.Add(1)
	go s.positionMonitorLoop()

	logrus.Infof("[oracle] 调度已启动: %d币种 × %d周期 消息面=%v 压力面=%v hub=ws:%v",
		len(s.cfg.Coins), len(s.cfg.Intervals), s.news != nil, s.pressure != nil, s.hub.IsWSMode())
}

// warmNews 启动时并发预热各币种消息面缓存，阻塞至全部完成（失败不致命）。
func (s *Scheduler) warmNews() {
	var wg sync.WaitGroup
	for _, coin := range s.cfg.Coins {
		wg.Add(1)
		go func(coin string) {
			defer wg.Done()
			s.refreshNews(coin)
		}(coin)
	}
	wg.Wait()
}

// newsLoop 按 News.RefreshInterval 慢节奏刷新指定币种消息面（预热已在 Start 完成）。
func (s *Scheduler) newsLoop(coin string) {
	defer s.wg.Done()

	every := s.cfg.News.RefreshInterval
	if every <= 0 {
		every = 30 * time.Minute
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.refreshNews(coin)
		}
	}
}

// refreshNews 刷新单个币种消息面，自带超时与 panic 防护。
func (s *Scheduler) refreshNews(coin string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[oracle][news] %s panic: %v", coin, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.AI.Timeout+30*time.Second)
	defer cancel()

	if err := s.news.Refresh(ctx, coin); err != nil {
		logrus.Warnf("[oracle][news] %s 刷新失败: %v", coin, err)
	}
}

func (s *Scheduler) Stop() {
	close(s.stop)
	s.wg.Wait()
	s.hub.Stop()
}

func (s *Scheduler) runLoop(coin, interval string) {
	defer s.wg.Done()

	every := s.cfg.ScanInterval[interval]
	if every <= 0 {
		every = s.cfg.DefaultScan
	}
	logrus.Infof("[oracle] %s/%s 启动，间隔=%s", coin, interval, every)

	// 启动即跑一次，随后按节奏。
	s.runOnce(coin, interval)

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.runOnce(coin, interval)
		}
	}
}

// settleLoop 定时把已到期的预测用真实价回填（误差/方向命中）。
func (s *Scheduler) settleLoop() {
	defer s.wg.Done()

	every := s.cfg.SettleInterval
	if every <= 0 {
		every = time.Minute
	}
	logrus.Infof("[oracle] 结算回填启动，间隔=%s", every)

	s.settleOnce()

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.settleOnce()
		}
	}
}

func (s *Scheduler) settleOnce() {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[oracle] 结算回填 panic: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	n, err := s.service.SettleDuePredictions(ctx, s.cfg.SettleBatch)
	if err != nil {
		logrus.Warnf("[oracle] 结算回填失败: %v", err)
		return
	}
	if n > 0 {
		logrus.Infof("[oracle] 结算回填完成: %d 条", n)
	}
}

// pressureLoop 按 Pressure.AnalyzeInterval 节奏分析指定币种压力面，启动即跑一次。
func (s *Scheduler) pressureLoop(coin string) {
	defer s.wg.Done()

	every := s.cfg.Pressure.AnalyzeInterval
	if every <= 0 {
		every = 10 * time.Minute
	}
	logrus.Infof("[oracle][pressure] %s 启动，间隔=%s 周期=%s", coin, every, s.cfg.Pressure.Interval)

	s.pressureOnce(coin)

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.pressureOnce(coin)
		}
	}
}

// pressureOnce 采集主周期行情→算指标→拼消息面→AI 压力面分析→落库，自带超时与 panic 防护。
func (s *Scheduler) pressureOnce(coin string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[oracle][pressure] %s panic: %v", coin, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.AI.Timeout+30*time.Second)
	defer cancel()

	interval := s.cfg.Pressure.Interval
	snap, err := collector.Collect(ctx, s.cfg, coin, interval)
	if err != nil {
		logrus.Warnf("[oracle][pressure] %s 采集失败: %v", coin, err)
		return
	}

	feats := indicator.Compute(snap)

	newsSummary := ""
	if s.news != nil {
		newsSummary = s.news.Summary(coin)
	}

	result, err := s.pressure.Analyze(ctx, snap, feats, newsSummary)
	if err != nil {
		logrus.Warnf("[oracle][pressure] %s 分析失败: %v", coin, err)
		return
	}
	logrus.Infof("[oracle][pressure] %s 压力面落库: bias=%s 阻力%d个 支撑%d个 关键阻力=%.4f 关键支撑=%.4f",
		coin, result.Bias, len(result.ShortPressureLevels), len(result.LongPressureLevels), result.KeyResistance, result.KeySupport)
}

func (s *Scheduler) runOnce(coin, interval string) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[oracle] %s/%s panic: %v", coin, interval, r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.AI.Timeout+30*time.Second)
	defer cancel()

	snap, err := collector.Collect(ctx, s.cfg, coin, interval)
	if err != nil {
		logrus.Warnf("[oracle] %s/%s 采集失败: %v", coin, interval, err)
		return
	}

	feats := indicator.Compute(snap)

	newsSummary := ""
	if s.news != nil {
		newsSummary = s.news.Summary(coin)
	}

	// 压力面摘要(含其计算时间)作为结构性参考注入预测；开关关闭或暂无缓存时为空。
	pressureSummary := ""
	if s.pressure != nil && s.cfg.Pressure.InjectToPrediction {
		pressureSummary = s.pressure.Summary(coin)
	}

	// 记录 AI 检测耗时：从发起到返回。交易瞬变，这段耗时决定预测落地时盘价已偏移多少。
	aiStart := time.Now()
	decision, _, err := s.analyzer.Analyze(ctx, snap, feats, newsSummary, pressureSummary)
	if err != nil {
		logrus.Warnf("[oracle] %s/%s AI分析失败: %v", coin, interval, err)
		return
	}
	costMs := time.Since(aiStart).Milliseconds()

	// AI 检测完成后即时采集一份实际盘价(实际开盘价)，与发起时 AI 参考的收盘价(ref_price)区分；
	// 失败不致命，记 0 并告警。
	openPrice, perr := collector.CurrentPrice(ctx, s.cfg, coin)
	if perr != nil {
		logrus.Warnf("[oracle] %s/%s 采集实际开盘价失败: %v", coin, interval, perr)
	}

	dto := analyzer.ToSaveDTO(snap, feats, decision, s.cfg.AI.Provider, s.cfg.AI.Model, newsSummary, pressureSummary, openPrice, costMs)
	savedID, err := s.service.SaveAIPredictionWithID(dto)
	if err != nil {
		logrus.Errorf("[oracle] %s/%s 落库失败: %v", coin, interval, err)
		return
	}
	logrus.Infof("[oracle] %s/%s 预测落库: trend=%s signal=%s 预测价=%.4f 置信=%.2f 耗时=%dms 实际开盘价=%.4f",
		coin, interval, decision.Trend, decision.Signal, decision.PredictPrice, decision.Confidence, costMs, openPrice)

	// P2：策略检测门——预测落库后，检测是否命中已配置的策略条件，命中则开仓。
	// 使用 openPrice（预测完成后的即时行情价）作为开仓基准价；openPrice=0 时跳过，避免无效开仓。
	if openPrice > 0 && savedID > 0 && decision.Signal != "hold" {
		s.strategyGate(coin, interval, savedID, openPrice, decision)
	}
}

// ─── P2：策略检测门 ──────────────────────────────────────────────────────────

// strategyGate 在预测落库后检测是否命中策略条件，命中则开仓写入持仓表。
// 不侵入预测主流程：任何错误只记日志，不影响预测结果。
func (s *Scheduler) strategyGate(coin, interval string, predictionID int64, openPrice float64, decision *analyzer.Decision) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[strategy] %s/%s gate panic: %v", coin, interval, r)
		}
	}()

	symbol := strings.ToUpper(coin) + "USDT"
	strategies, err := s.service.GetActiveStrategies(s.cfg.Platform, symbol, interval)
	if err != nil {
		logrus.Warnf("[strategy] 拉取策略失败 %s/%s: %v", coin, interval, err)
		return
	}
	if len(strategies) == 0 {
		return
	}

	// AI 预测方向
	direction := strings.ToLower(decision.Trend) // long/short/neutral
	if direction == "neutral" {
		return
	}

	// 预测幅度（绝对值百分比）
	movePct := math.Abs(decision.ExpectedMovePct)

	now := time.Now().UTC()

	for _, strategy := range strategies {
		// 1. 方向过滤
		filter := strings.ToLower(strategy.TrendFilter)
		if filter != "both" && filter != direction {
			continue
		}

		// 2. 置信度 + 幅度阈值
		if decision.Confidence < strategy.MinConfidence {
			logrus.Debugf("[strategy] %s/%s id=%d 置信度%.2f < %.2f 跳过",
				coin, interval, strategy.Id, decision.Confidence, strategy.MinConfidence)
			continue
		}
		if movePct < strategy.MinMovePct {
			logrus.Debugf("[strategy] %s/%s id=%d 幅度%.4f%% < %.4f%% 跳过",
				coin, interval, strategy.Id, movePct, strategy.MinMovePct)
			continue
		}

		// 3. max_open_positions 检查
		openCount, err := s.service.CountOpenPositions(int64(strategy.Id))
		if err != nil {
			logrus.Warnf("[strategy] 统计持仓数失败 strategy=%d: %v", strategy.Id, err)
			continue
		}
		if openCount >= int64(strategy.MaxOpenPositions) {
			logrus.Debugf("[strategy] strategy=%d 已持仓%d/%d，跳过开仓", strategy.Id, openCount, strategy.MaxOpenPositions)
			continue
		}

		// 4. 计算止盈止损价（策略配置优先，0 则用 AI 给的）
		tpPrice, slPrice := resolveSLTP(strategy, decision, openPrice, direction)
		if slPrice <= 0 || tpPrice <= 0 {
			logrus.Warnf("[strategy] strategy=%d %s 无法确定 TP/SL，跳过开仓 (tp=%.4f sl=%.4f)",
				strategy.Id, symbol, tpPrice, slPrice)
			continue
		}

		// 5. 写入持仓记录
		holdSec := strategy.HoldDuration
		if holdSec <= 0 {
			holdSec = 14400 // 默认 4h
		}
		maxSec := strategy.MaxHoldDuration
		if maxSec <= 0 {
			maxSec = 86400
		}
		if holdSec > maxSec {
			holdSec = maxSec
		}

		pos := &tradeRepository.TradeStrategyPosition{
			StrategyID:         int64(strategy.Id),
			PredictionID:       predictionID,
			PlatformCode:       strings.ToLower(s.cfg.Platform),
			CoinCode:           strings.ToUpper(coin),
			Symbol:             symbol,
			Interval:           interval,
			Direction:          direction,
			OpenPrice:          openPrice,
			TakeProfitPrice:    tpPrice,
			StopLossPrice:      slPrice,
			Contracts:          strategy.Contracts,
			Leverage:           strategy.Leverage,
			OpenedAt:           now,
			HoldUntil:          now.Add(time.Duration(holdSec) * time.Second),
			Status:             "open",
			Confidence:         decision.Confidence,
			PredictedMovePct:   movePct,
			MaxPriceDuringHold: openPrice,
			MinPriceDuringHold: openPrice,
		}

		if err := s.service.OpenPosition(pos); err != nil {
			logrus.Errorf("[strategy] 开仓写库失败 strategy=%d %s: %v", strategy.Id, symbol, err)
			continue
		}
		logrus.Infof("[strategy] 开仓 strategy=%d %s/%s direction=%s openPrice=%.4f tp=%.4f sl=%.4f holdUntil=%s",
			strategy.Id, coin, interval, direction, openPrice, tpPrice, slPrice, pos.HoldUntil.Format("01-02 15:04:05"))
	}
}

// resolveSLTP 确定止盈止损绝对价：策略配置非 0 时按配置百分比算，否则用 AI 建议价。
func resolveSLTP(strategy *tradeRepository.TradeStrategy, decision *analyzer.Decision, openPrice float64, direction string) (tp, sl float64) {
	if strategy.TakeProfitPct > 0 {
		if direction == "long" {
			tp = openPrice * (1 + strategy.TakeProfitPct/100)
		} else {
			tp = openPrice * (1 - strategy.TakeProfitPct/100)
		}
	} else {
		tp = decision.TakeProfit
	}

	if strategy.StopLossPct > 0 {
		if direction == "long" {
			sl = openPrice * (1 - strategy.StopLossPct/100)
		} else {
			sl = openPrice * (1 + strategy.StopLossPct/100)
		}
	} else {
		sl = decision.StopLoss
	}
	return tp, sl
}

// ─── P3：持仓监测循环 ─────────────────────────────────────────────────────────

// positionMonitorLoop 订阅 hub tick，每收到一次 tick 就检测该 symbol 下的所有持仓。
// 同时维护一个 5s 兜底 ticker，防止 hub 静默期（WS/REST 均无推送）时持仓长时间不被检测。
func (s *Scheduler) positionMonitorLoop() {
	defer s.wg.Done()

	tickCh, cancelSub := s.hub.Subscribe()
	defer cancelSub()

	// 兜底：即使无 tick 推送，每 5s 也全量检测一次（防静默）
	fallback := time.NewTicker(5 * time.Second)
	defer fallback.Stop()

	// 按 symbol 做去抖：同一 symbol 的 tick 在 200ms 内只触发一次检测
	debounce := make(map[string]time.Time)
	const debounceWindow = 200 * time.Millisecond

	checkSymbol := func(symbol string) {
		now := time.Now()
		if last, ok := debounce[symbol]; ok && now.Sub(last) < debounceWindow {
			return
		}
		debounce[symbol] = now
		price, ok := s.hub.GetPrice(symbol)
		if !ok {
			return
		}
		s.checkPositionsForSymbol(symbol, price, now.UTC())
	}

	for {
		select {
		case <-s.stop:
			return
		case tick := <-tickCh:
			checkSymbol(tick.Symbol)
		case <-fallback.C:
			s.monitorAllPositions()
		}
	}
}

// checkPositionsForSymbol 检测指定 symbol 下所有 open 持仓。
func (s *Scheduler) checkPositionsForSymbol(symbol string, price float64, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[monitor] %s panic: %v", symbol, r)
		}
	}()

	positions, err := s.service.GetOpenPositions()
	if err != nil {
		return
	}
	for _, pos := range positions {
		if pos.Symbol != symbol {
			continue
		}
		s.evaluatePosition(pos, price, now)
	}
}

// monitorAllPositions 兜底全量检测，针对所有 symbol 用 hub 取最新价。
func (s *Scheduler) monitorAllPositions() {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("[monitor] 全量检测 panic: %v", r)
		}
	}()

	positions, err := s.service.GetOpenPositions()
	if err != nil || len(positions) == 0 {
		return
	}
	now := time.Now().UTC()
	for _, pos := range positions {
		price, ok := s.hub.GetPrice(pos.Symbol)
		if !ok {
			continue
		}
		s.evaluatePosition(pos, price, now)
	}
}

// evaluatePosition 对单条持仓执行 TP/SL/超时检测，并更新 max/min 价格追踪。
func (s *Scheduler) evaluatePosition(pos *tradeRepository.TradeStrategyPosition, price float64, now time.Time) {
	// 更新最高/最低价（忽略错误，不影响平仓判断）
	_ = s.service.UpdatePositionMinMax(pos, price)

	var closeReason string
	switch {
	case pos.Direction == "long" && price >= pos.TakeProfitPrice:
		closeReason = "tp"
	case pos.Direction == "short" && price <= pos.TakeProfitPrice:
		closeReason = "tp"
	case pos.Direction == "long" && price <= pos.StopLossPrice:
		closeReason = "sl"
	case pos.Direction == "short" && price >= pos.StopLossPrice:
		closeReason = "sl"
	case now.After(pos.HoldUntil):
		closeReason = "timeout"
	}

	if closeReason == "" {
		return
	}

	// 查找所属策略的手续费配置
	strategies, _ := s.service.GetActiveStrategies(pos.PlatformCode, pos.Symbol, pos.Interval)
	makerFeeRate := 0.0002
	takerFeeRate := 0.0005
	for _, st := range strategies {
		if int64(st.Id) == pos.StrategyID {
			makerFeeRate = st.MakerFeeRate
			takerFeeRate = st.TakerFeeRate
			break
		}
	}

	err := s.service.ClosePosition(pos, price, closeReason, now, makerFeeRate, takerFeeRate)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return // 已被其他并发检测平仓，幂等忽略
		}
		logrus.Errorf("[monitor] 平仓失败 pos=%d reason=%s: %v", pos.Id, closeReason, err)
		return
	}
	logrus.Infof("[monitor] 平仓 pos=%d %s direction=%s price=%.4f reason=%s",
		pos.Id, pos.Symbol, pos.Direction, price, closeReason)
}
