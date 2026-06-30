package trade

import (
	"context"
	"fmt"
	"strings"
	"time"

	tradeDTO "service/trade/dto"
	tradeRepository "service/trade/repository"

	argusTrade "argus_single/pkg/trade"

	"github.com/sirupsen/logrus"
)

// klineFetchMax 单次向交易所拉取 K 线的上限(主流交易所如 Binance 期货单次返回上限)。
const klineFetchMax = 1500

// BackfillKlines 把“某币种 + 某周期 + 最近 N 根”K 线回填进 trade_kline。
//
// 增量策略(避免重复拉取)：
//  1. 查 DB 里该 symbol+interval 距今最新的一条；
//  2. 用 (now - 最新 open_time) / 周期 推算自那以后新增了多少根 → 只补这部分；
//     DB 原本没有数据时则按请求的 N 根全量拉取；
//  3. 实际拉取数 = min(需补, N, 交易所单次上限)。
//
// 入库按唯一键 (symbol, interval, open_time) 幂等，重复时只刷新行情值。
func (s *TradeService) BackfillKlines(ctx context.Context, dto tradeDTO.BackfillKlineDTO) (*tradeDTO.BackfillKlineResultDTO, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	symbol := strings.ToUpper(strings.TrimSpace(dto.Symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol 不能为空")
	}
	interval := strings.TrimSpace(dto.Interval)
	if interval == "" {
		interval = "1m"
	}
	platform := strings.TrimSpace(dto.PlatformCode)
	if platform == "" {
		platform = "binance"
	}
	want := dto.Limit
	if want <= 0 {
		return nil, fmt.Errorf("limit 必须大于 0")
	}
	dur, ok := intervalDuration(interval)
	if !ok {
		return nil, fmt.Errorf("不支持的周期: %s", interval)
	}

	// 1) 查现有最新一条，推算需要补多少根。
	latest, err := s.tradeKlineRepository.LatestKline(symbol, interval)
	if err != nil {
		return nil, err
	}
	result := &tradeDTO.BackfillKlineResultDTO{Symbol: symbol, Interval: interval, Requested: want}
	need := want
	if latest != nil {
		result.LatestBefore = fmtTime(latest.OpenTime)
		// 自最新一根以来理应新增的根数；为负(时钟/时区抖动)按 0 处理。
		missing := int(time.Now().UTC().Sub(latest.OpenTime.UTC()) / dur)
		if missing < 0 {
			missing = 0
		}
		if missing < want {
			need = missing // 已有大部分，只补缺口
		}
	}
	result.NeedFetch = need
	if need <= 0 {
		result.LatestAfter = result.LatestBefore
		return result, nil // 已是最新，无需拉取
	}

	// 2) 拉取 + 幂等入库。
	fetched, affected, err := s.fetchAndStoreRecentKlines(ctx, platform, symbol, interval, need)
	if err != nil {
		return nil, err
	}
	result.Fetched = fetched
	result.Upserted = affected

	if after, err := s.tradeKlineRepository.LatestKline(symbol, interval); err == nil && after != nil {
		result.LatestAfter = fmtTime(after.OpenTime)
	}
	return result, nil
}

// fetchAndStoreRecentKlines 从交易所拉取“最近 limit 根”并幂等入库，返回(实拉根数, 入库影响行数)。
// limit 超过交易所单次上限时按上限截断。供主动回填与回测前自动补齐共用。
func (s *TradeService) fetchAndStoreRecentKlines(ctx context.Context, platform, symbol, interval string, limit int) (int, int64, error) {
	if limit <= 0 {
		return 0, 0, nil
	}
	if limit > klineFetchMax {
		limit = klineFetchMax
	}
	rows, err := argusTrade.GetKlinesByPlatform(ctx, platform, argusTrade.MarketKlineRequest{
		Symbol:   symbol,
		Interval: interval,
		Limit:    limit,
	})
	if err != nil {
		return 0, 0, fmt.Errorf("拉取%s行情失败: %w", platform, err)
	}
	models, err := klinesToModels(symbol, interval, rows)
	if err != nil {
		return 0, 0, err
	}
	affected, err := s.tradeKlineRepository.UpsertKlines(models)
	if err != nil {
		return len(rows), 0, err
	}
	return len(rows), affected, nil
}

// ensureBacktestKlines 回测前自动补齐窗口 K 线（best-effort，失败只告警不中断回测）。
//
// 与主动回填(BackfillKlines)的“最新基准增量”不同：这里按【窗口覆盖率】判定——
// 因为回测窗口可能整段落在过去，仅看最新一条会误判“已是最新”而漏补历史段。
// 交易所只返回“最近 N 根”，故按 (now - StartTime)/周期 推算需要的根数往回兜。
func (s *TradeService) ensureBacktestKlines(run *tradeRepository.TradeBacktestRun, rightPad time.Duration) {
	dur, ok := intervalDuration(run.PriceInterval)
	if !ok {
		logrus.Warnf("[backtest] run=%d 不支持的价格周期 %q，跳过自动回填", run.Id, run.PriceInterval)
		return
	}
	now := time.Now().UTC()
	// 需覆盖 [StartTime, EndTime+rightPad]；但 K 线最多到“现在”，故有效右界取 min(窗口右界, now)。
	effEnd := run.EndTime.Add(rightPad)
	if effEnd.After(now) {
		effEnd = now
	}
	if !effEnd.After(run.StartTime) {
		return // 窗口在未来或为空，无可补
	}
	expected := int(effEnd.Sub(run.StartTime) / dur)
	have, err := s.tradeKlineRepository.CountBySymbolIntervalRange(run.Symbol, run.PriceInterval, run.StartTime, effEnd)
	if err != nil {
		logrus.Warnf("[backtest] run=%d 统计已有K线失败(继续): %v", run.Id, err)
		return
	}
	// 已基本覆盖(>=95%)则跳过，避免每次回测都重复拉取。
	if expected > 0 && have >= int64(expected)*95/100 {
		return
	}
	// 要回溯到窗口左界 StartTime，需要“最近 (now-StartTime)/周期 根”。
	want := int(now.Sub(run.StartTime)/dur) + 2 // +2 边界缓冲
	fetched, upserted, err := s.fetchAndStoreRecentKlines(context.Background(), run.PlatformCode, run.Symbol, run.PriceInterval, want)
	if err != nil {
		logrus.Warnf("[backtest] run=%d 自动回填K线失败(继续用现有数据): %v", run.Id, err)
		return
	}
	logrus.Infof("[backtest] run=%d 自动回填K线: 期望=%d 已有=%d 实拉=%d 入库=%d", run.Id, expected, have, fetched, upserted)
}

// ListKlinesInRange 拉取某 symbol+interval 在 [start,end] 内的 K 线，供回测逐笔的“K线详情”弹窗。
func (s *TradeService) ListKlinesInRange(symbol, interval, startStr, endStr string) ([]tradeDTO.KlinePointDTO, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol 不能为空")
	}
	if strings.TrimSpace(interval) == "" {
		interval = "1m"
	}
	start, err := parseTimeFlexible(startStr)
	if err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	end, err := parseTimeFlexible(endStr)
	if err != nil {
		return nil, fmt.Errorf("end: %w", err)
	}
	rows, err := s.tradeKlineRepository.ListBySymbolIntervalTimeRange(symbol, interval, start, end)
	if err != nil {
		return nil, err
	}
	out := make([]tradeDTO.KlinePointDTO, 0, len(rows))
	for _, k := range rows {
		out = append(out, tradeDTO.KlinePointDTO{
			Time:   fmtTime(k.OpenTime),
			Open:   k.OpenPrice,
			High:   k.HighPrice,
			Low:    k.LowPrice,
			Close:  k.ClosePrice,
			Volume: k.Volume,
		})
	}
	return out, nil
}

// intervalDuration 把周期字符串转成时长；不支持的周期返回 ok=false。
func intervalDuration(interval string) (time.Duration, bool) {
	switch interval {
	case "1m":
		return time.Minute, true
	case "3m":
		return 3 * time.Minute, true
	case "5m":
		return 5 * time.Minute, true
	case "15m":
		return 15 * time.Minute, true
	case "30m":
		return 30 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "2h":
		return 2 * time.Hour, true
	case "4h":
		return 4 * time.Hour, true
	case "6h":
		return 6 * time.Hour, true
	case "8h":
		return 8 * time.Hour, true
	case "12h":
		return 12 * time.Hour, true
	case "1d":
		return 24 * time.Hour, true
	default:
		return 0, false
	}
}

// klinesToModels 把交易所返回的 K 线(字符串价 + 毫秒时间)映射成入库模型。
// 时间用 time.UnixMilli(本地时区)，与模拟分析的取数口径(fetchSimulationKlines)保持一致。
func klinesToModels(symbol, interval string, rows []argusTrade.MarketKline) ([]*tradeRepository.TradeKline, error) {
	out := make([]*tradeRepository.TradeKline, 0, len(rows))
	for _, r := range rows {
		open, err := parseMarketFloat(r.OpenPrice)
		if err != nil {
			return nil, fmt.Errorf("解析开盘价失败: %w", err)
		}
		high, err := parseMarketFloat(r.HighPrice)
		if err != nil {
			return nil, fmt.Errorf("解析最高价失败: %w", err)
		}
		low, err := parseMarketFloat(r.LowPrice)
		if err != nil {
			return nil, fmt.Errorf("解析最低价失败: %w", err)
		}
		closePrice, err := parseMarketFloat(r.ClosePrice)
		if err != nil {
			return nil, fmt.Errorf("解析收盘价失败: %w", err)
		}
		volume, _ := parseMarketFloat(r.Volume)
		turnover, _ := parseMarketFloat(r.QuoteVolume)
		var tradeCount uint64
		if r.TradeCount > 0 {
			tradeCount = uint64(r.TradeCount)
		}
		out = append(out, &tradeRepository.TradeKline{
			Symbol:     symbol,
			Interval:   interval,
			OpenTime:   time.UnixMilli(r.OpenTime),
			CloseTime:  time.UnixMilli(r.CloseTime),
			OpenPrice:  open,
			HighPrice:  high,
			LowPrice:   low,
			ClosePrice: closePrice,
			Volume:     volume,
			Turnover:   turnover,
			TradeCount: tradeCount,
		})
	}
	return out, nil
}
