package scheduler

import (
	"context"
	"sync"
	"time"

	newssvc "service/news"
	pressuresvc "service/pressure"
	tradesvc "service/trade"

	"oracle/pkg/analyzer"
	"oracle/pkg/collector"
	"oracle/pkg/indicator"
	"oracle/pkg/news"
	"oracle/pkg/oraclecfg"
	"oracle/pkg/pressure"

	"github.com/sirupsen/logrus"
)

// Scheduler 按 币种×主周期 定时跑 采集→指标→AI→落库。
type Scheduler struct {
	cfg      oraclecfg.Config
	analyzer *analyzer.Analyzer
	news     *news.Collector
	pressure *pressure.Analyzer
	service  *tradesvc.TradeService
	stop     chan struct{}
	wg       sync.WaitGroup
}

func New(cfg oraclecfg.Config, service *tradesvc.TradeService, newsService *newssvc.NewsService, pressureService *pressuresvc.PressureService) *Scheduler {
	s := &Scheduler{
		cfg:      cfg,
		analyzer: analyzer.New(cfg.AI),
		service:  service,
		stop:     make(chan struct{}),
	}
	if cfg.News.Enabled {
		s.news = news.New(cfg.AI, cfg.News, newsService)
	}
	if cfg.Pressure.Enabled {
		s.pressure = pressure.New(pressureAIConfig(cfg), pressureService)
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

	logrus.Infof("[oracle] 调度已启动: %d币种 × %d周期 消息面=%v 压力面=%v",
		len(s.cfg.Coins), len(s.cfg.Intervals), s.news != nil, s.pressure != nil)
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

	decision, _, err := s.analyzer.Analyze(ctx, snap, feats, newsSummary)
	if err != nil {
		logrus.Warnf("[oracle] %s/%s AI分析失败: %v", coin, interval, err)
		return
	}

	dto := analyzer.ToSaveDTO(snap, feats, decision, s.cfg.AI.Provider, s.cfg.AI.Model, newsSummary)
	if err := s.service.SaveAIPrediction(dto); err != nil {
		logrus.Errorf("[oracle] %s/%s 落库失败: %v", coin, interval, err)
		return
	}
	logrus.Infof("[oracle] %s/%s 预测落库: trend=%s signal=%s 预测价=%.4f 置信=%.2f",
		coin, interval, decision.Trend, decision.Signal, decision.PredictPrice, decision.Confidence)
}
