// Package backfill 历史预测回填：以源周期(1h)在最近一段时间的发起时刻为锚点，
// 在每个时刻用「截至该时刻」的历史 K 线补跑长周期(4h/12h/1d)预测并落库。
//
// 用途：1h 一直在每小时滚动产出，而 4h/12h/1d 此前因边界对齐/部署原因缺数据，
// 用本包把这些时刻的长周期预测补齐，使各周期覆盖一致。
//
// 精度说明：交易所 K 线接口只能取「最近 N 根」，本包一次性拉足够长的历史再按时刻切片重建快照；
// 但逐笔成交(trades)与资金费(funding)只有“最近值”、历史时点无法重建，回填快照中置空(指标按 N/A 处理)。
// 因此回填预测与当时真·实时预测会有差异，且不注入消息面/压力面(同样无法重建历史)。
package backfill

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	argusTrade "argus_single/pkg/trade"
	tradesvc "service/trade"

	"oracle/pkg/analyzer"
	"oracle/pkg/collector"
	"oracle/pkg/indicator"
	"oracle/pkg/oraclecfg"

	"github.com/sirupsen/logrus"
)

// exchangeKlineMax 单次向交易所拉取 K 线的保守上限（避免触发平台单次返回限制）。
const exchangeKlineMax = 1000

// minPrimaryBars 主周期切片后至少要有的根数，低于此值指标不可靠，跳过该时刻。
const minPrimaryBars = 30

// Options 回填参数。
type Options struct {
	Coin           string        // 回填币种；空则取 cfg.Coins[0]。
	Intervals      []string      // 要补跑的预测周期；空则默认 4h,12h,1d。
	SourceInterval string        // 时间点来源周期；空则默认 1h。
	Lookback       time.Duration // 回看时长；<=0 则默认 72h。
	DelayPerCall   time.Duration // 每次 LLM 调用之间的间隔，缓解限流；<=0 不停顿。
	DryRun         bool          // 只切片+打印，不调用 LLM、不落库。
}

func (o *Options) withDefaults(cfg oraclecfg.Config) {
	if strings.TrimSpace(o.Coin) == "" {
		if len(cfg.Coins) > 0 {
			o.Coin = cfg.Coins[0]
		} else {
			o.Coin = "BTC"
		}
	}
	o.Coin = strings.ToUpper(strings.TrimSpace(o.Coin))
	if len(o.Intervals) == 0 {
		o.Intervals = []string{"4h", "12h", "1d"}
	}
	if strings.TrimSpace(o.SourceInterval) == "" {
		o.SourceInterval = "1h"
	}
	if o.Lookback <= 0 {
		o.Lookback = 72 * time.Hour
	}
}

// Run 执行回填。返回处理过程中遇到的首个致命错误（取时间点失败等）；
// 单个时刻/周期的失败只记日志、不中断整体。
func Run(ctx context.Context, cfg oraclecfg.Config, service *tradesvc.TradeService, opts Options) error {
	opts.withDefaults(cfg)

	end := time.Now()
	start := end.Add(-opts.Lookback)

	// 1) 取源周期发起时刻作为锚点。
	anchors, err := loadAnchors(cfg, service, opts, start, end)
	if err != nil {
		return err
	}
	if len(anchors) == 0 {
		return fmt.Errorf("最近 %s 内未找到 %s/%s 的预测时间点，无可回填锚点",
			opts.Lookback, opts.Coin, opts.SourceInterval)
	}
	logrus.Infof("[backfill] %s 锚点=%d 个 区间=[%s ~ %s] 目标周期=%v dryRun=%v",
		opts.Coin, len(anchors), anchors[0].Format("01-02 15:04"), anchors[len(anchors)-1].Format("01-02 15:04"),
		opts.Intervals, opts.DryRun)

	an := analyzer.New(cfg.AI)

	// 2) 逐周期回填。
	for _, interval := range opts.Intervals {
		runInterval(ctx, cfg, service, an, opts, interval, anchors)
	}
	return nil
}

// loadAnchors 取源周期在 [start,end] 的发起时刻，去重升序。
func loadAnchors(cfg oraclecfg.Config, service *tradesvc.TradeService, opts Options, start, end time.Time) ([]time.Time, error) {
	rows, err := service.ListPredictionsByCreatedRange(cfg.Platform, opts.Coin, opts.SourceInterval, start, end)
	if err != nil {
		return nil, fmt.Errorf("查询源周期预测失败: %w", err)
	}
	seen := make(map[int64]struct{}, len(rows))
	var anchors []time.Time
	for _, r := range rows {
		t := r.CreatedTime
		key := t.Unix()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		anchors = append(anchors, t)
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].Before(anchors[j]) })
	return anchors, nil
}

// runInterval 为单个目标周期预取历史 K 线，逐锚点切片→分析→落库。
func runInterval(ctx context.Context, cfg oraclecfg.Config, service *tradesvc.TradeService, an *analyzer.Analyzer,
	opts Options, interval string, anchors []time.Time) {

	symbol := opts.Coin + "USDT"

	// 预取主周期历史：足够覆盖最早锚点往前 KlineLimit 根。
	primaryAll, err := fetchHistory(ctx, cfg, symbol, interval, neededBars(interval, opts.Lookback, cfg.KlineLimit))
	if err != nil {
		logrus.Errorf("[backfill] %s/%s 拉取主周期历史失败: %v", opts.Coin, interval, err)
		return
	}

	// 预取高周期历史（失败不致命，缺哪个少哪个）。
	htfAll := map[string][]argusTrade.MarketKline{}
	for _, htf := range cfg.HighTimeframes[interval] {
		rows, herr := fetchHistory(ctx, cfg, symbol, htf, neededBars(htf, opts.Lookback, cfg.HighKlineLimit))
		if herr != nil || len(rows) == 0 {
			logrus.Warnf("[backfill] %s/%s 高周期 %s 历史拉取失败/为空: %v", opts.Coin, interval, htf, herr)
			continue
		}
		htfAll[htf] = rows
	}

	var saved, skipped, failed int
	for _, t := range anchors {
		snap := buildSnapshot(cfg, opts.Coin, interval, t, primaryAll, htfAll)
		if snap == nil {
			skipped++
			continue
		}
		feats := indicator.Compute(snap)

		if opts.DryRun {
			logrus.Infof("[backfill][dry] %s/%s @%s 主周期=%d根 refClose=%.4f → 预测时刻=%s",
				opts.Coin, interval, t.Format("01-02 15:04"), len(snap.Primary), feats.LastClose,
				t.Add(analyzer.IntervalDuration(interval)).Format("01-02 15:04"))
			saved++
			continue
		}

		// 历史回填不注入消息面/压力面（无法重建历史快照）。
		aiStart := time.Now()
		decision, _, aerr := an.Analyze(ctx, snap, feats, "", "", false)
		if aerr != nil {
			logrus.Warnf("[backfill] %s/%s @%s AI分析失败: %v", opts.Coin, interval, t.Format("01-02 15:04"), aerr)
			failed++
			continue
		}
		costMs := time.Since(aiStart).Milliseconds()

		// 历史时点无独立盘价：以参考收盘价(LastClose)兼作 open_price。
		dto := analyzer.ToSaveDTOAt(snap, feats, decision, cfg.AI.Provider, cfg.AI.Model, "", "", feats.LastClose, costMs, t)
		if err := service.SaveAIPredictionAt(dto, t); err != nil {
			logrus.Errorf("[backfill] %s/%s @%s 落库失败: %v", opts.Coin, interval, t.Format("01-02 15:04"), err)
			failed++
			continue
		}
		saved++
		logrus.Infof("[backfill] %s/%s @%s 落库: trend=%s signal=%s 预测价=%.4f 置信=%.2f",
			opts.Coin, interval, t.Format("01-02 15:04"), decision.Trend, decision.Signal, decision.PredictPrice, decision.Confidence)

		if opts.DelayPerCall > 0 {
			select {
			case <-ctx.Done():
				logrus.Warnf("[backfill] %s/%s 被取消，提前结束", opts.Coin, interval)
				return
			case <-time.After(opts.DelayPerCall):
			}
		}
	}
	logrus.Infof("[backfill] %s/%s 完成: 落库=%d 跳过=%d 失败=%d (共%d锚点)",
		opts.Coin, interval, saved, skipped, failed, len(anchors))
}

// buildSnapshot 把预取的历史 K 线按锚点时刻 t 切片，重建一份「截至 t」的快照。
// 主周期不足 minPrimaryBars 时返回 nil（指标不可靠，跳过该时刻）。
func buildSnapshot(cfg oraclecfg.Config, coin, interval string, t time.Time,
	primaryAll []argusTrade.MarketKline, htfAll map[string][]argusTrade.MarketKline) *collector.Snapshot {

	primary := sliceAsOf(primaryAll, t, cfg.KlineLimit)
	if len(primary) < minPrimaryBars {
		return nil
	}
	snap := &collector.Snapshot{
		Platform: cfg.Platform,
		CoinCode: coin,
		Symbol:   coin + "USDT",
		Interval: interval,
		Primary:  primary,
		HighTF:   map[string][]argusTrade.MarketKline{},
	}
	for htf, rows := range htfAll {
		if s := sliceAsOf(rows, t, cfg.HighKlineLimit); len(s) > 0 {
			snap.HighTF[htf] = s
		}
	}
	// Trades/Funding 历史不可重建，置空：指标层按 N/A 处理。
	return snap
}

// sliceAsOf 从升序 K 线中取 OpenTime ≤ t 的最后 n 根（含 t 所在那根未收盘的当前 K 线，贴合实时采集语义）。
func sliceAsOf(all []argusTrade.MarketKline, t time.Time, n int) []argusTrade.MarketKline {
	if len(all) == 0 || n <= 0 {
		return nil
	}
	tms := t.UnixMilli()
	endIdx := 0
	for i := range all {
		if all[i].OpenTime <= tms {
			endIdx = i + 1
		} else {
			break
		}
	}
	if endIdx == 0 {
		return nil
	}
	startIdx := endIdx - n
	if startIdx < 0 {
		startIdx = 0
	}
	out := make([]argusTrade.MarketKline, endIdx-startIdx)
	copy(out, all[startIdx:endIdx])
	return out
}

// neededBars 估算要预取多少根：覆盖回看时长 + 锚点往前 baseN 根 + 余量，并钳到交易所单次上限。
func neededBars(interval string, lookback time.Duration, baseN int) int {
	dur := analyzer.IntervalDuration(interval)
	if dur <= 0 {
		dur = time.Hour
	}
	lookbackBars := int(math.Ceil(float64(lookback) / float64(dur)))
	want := baseN + lookbackBars + 5
	if want > exchangeKlineMax {
		want = exchangeKlineMax
	}
	if want < baseN {
		want = baseN
	}
	return want
}

// fetchHistory 拉取指定周期最近 limit 根 K 线（升序），供后续按锚点切片。
func fetchHistory(ctx context.Context, cfg oraclecfg.Config, symbol, interval string, limit int) ([]argusTrade.MarketKline, error) {
	rows, err := argusTrade.GetKlinesByPlatform(ctx, cfg.Platform, argusTrade.MarketKlineRequest{
		Symbol:   symbol,
		Interval: interval,
		Limit:    limit,
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("K线为空(%s %s)", symbol, interval)
	}
	return rows, nil
}
