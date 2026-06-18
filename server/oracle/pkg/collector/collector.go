package collector

import (
	"context"
	"fmt"
	"strings"

	argusTrade "argus_single/pkg/trade"
	"oracle/pkg/oraclecfg"
)

// Snapshot 一次采集得到的多周期行情快照。
type Snapshot struct {
	Platform string
	CoinCode string
	Symbol   string
	Interval string

	// Primary 主周期 K 线（时间升序）。
	Primary []argusTrade.MarketKline
	// HighTF 高周期 K 线，key=周期。
	HighTF map[string][]argusTrade.MarketKline
	// Trades 最近逐笔成交。
	Trades []argusTrade.MarketTrade
	// Funding 资金费率快照（可能为 nil）。
	Funding *argusTrade.FundingRateSnapshot
}

// Collect 采集指定 币种×主周期 的多周期行情 + 逐笔 + 资金费。
func Collect(ctx context.Context, cfg oraclecfg.Config, coin, interval string) (*Snapshot, error) {
	coin = strings.ToUpper(strings.TrimSpace(coin))
	symbol := coin + "USDT"

	primary, err := argusTrade.GetKlinesByPlatform(ctx, cfg.Platform, argusTrade.MarketKlineRequest{
		Symbol:   symbol,
		Interval: interval,
		Limit:    cfg.KlineLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("拉取主周期K线失败(%s %s): %w", symbol, interval, err)
	}
	if len(primary) == 0 {
		return nil, fmt.Errorf("主周期K线为空(%s %s)", symbol, interval)
	}

	snap := &Snapshot{
		Platform: cfg.Platform,
		CoinCode: coin,
		Symbol:   symbol,
		Interval: interval,
		Primary:  primary,
		HighTF:   map[string][]argusTrade.MarketKline{},
	}

	// 高周期：失败不致命，仅记空。
	for _, htf := range cfg.HighTimeframes[interval] {
		rows, herr := argusTrade.GetKlinesByPlatform(ctx, cfg.Platform, argusTrade.MarketKlineRequest{
			Symbol:   symbol,
			Interval: htf,
			Limit:    cfg.HighKlineLimit,
		})
		if herr == nil && len(rows) > 0 {
			snap.HighTF[htf] = rows
		}
	}

	// 逐笔成交：失败不致命。
	if trades, terr := argusTrade.GetRecentTradesByPlatform(ctx, cfg.Platform, argusTrade.MarketTradeRequest{
		Symbol: symbol,
		Limit:  cfg.TradeLimit,
	}); terr == nil {
		snap.Trades = trades
	}

	// 资金费：失败不致命。
	if funding, ferr := argusTrade.GetFundingRateByPlatform(ctx, cfg.Platform, argusTrade.FundingRateRequest{
		Symbol: symbol,
		Limit:  cfg.FundingLimit,
	}); ferr == nil {
		snap.Funding = funding
	}

	return snap, nil
}
