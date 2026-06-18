package trade

import (
	"context"
	"fmt"
)

// QueryKlinesByPlatform 通过 platform 路由到对应交易所 K 线实现。
func QueryKlinesByPlatform(ctx context.Context, platform string, req MarketKlineRequest) ([]MarketKline, error) {
	client, err := NewPlatformExchangeClient(platform)
	if err != nil {
		return nil, err
	}
	return client.GetKlines(ctx, req)
}

// QueryTickerByPlatform 通过 platform 路由到对应交易所实时价格实现。
func QueryTickerByPlatform(ctx context.Context, platform string, req MarketTickerRequest) (*MarketTicker, error) {
	client, err := NewPlatformExchangeClient(platform)
	if err != nil {
		return nil, err
	}
	return client.GetTicker(ctx, req)
}

// QueryRecentTradesByPlatform 通过 platform 路由到对应交易所逐笔成交实现。
func QueryRecentTradesByPlatform(ctx context.Context, platform string, req MarketTradeRequest) ([]MarketTrade, error) {
	client, err := NewPlatformExchangeClient(platform)
	if err != nil {
		return nil, err
	}
	return client.GetRecentTrades(ctx, req)
}

// QueryFundingRateByPlatform 通过 platform 路由到对应交易所资金费率实现。
func QueryFundingRateByPlatform(ctx context.Context, platform string, req FundingRateRequest) (*FundingRateSnapshot, error) {
	client, err := NewPlatformExchangeClient(platform)
	if err != nil {
		return nil, err
	}
	return client.GetFundingRate(ctx, req)
}

func (tm *TradeManager) GetBalancesByPlatform(ctx context.Context, platform, accountName string, req BalanceRequest) ([]Balance, error) {
	client, err := tm.exchangeClientFor(platform, accountName)
	if err != nil {
		return nil, err
	}
	return client.GetBalances(ctx, req)
}

func (tm *TradeManager) PlaceOrderByPlatform(ctx context.Context, platform, accountName string, req ExchangeOrderRequest) (*ExchangeOrderResponse, error) {
	client, err := tm.exchangeClientFor(platform, accountName)
	if err != nil {
		return nil, err
	}
	return client.PlaceOrder(ctx, req)
}

func (tm *TradeManager) exchangeClientFor(platform, accountName string) (ExchangeClient, error) {
	if tm == nil {
		return nil, fmt.Errorf("交易管理器未初始化")
	}

	normalizedPlatform := NormalizePlatform(platform)
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	if accountName != "" {
		client := tm.exchangeClients[accountName]
		if client == nil {
			return nil, fmt.Errorf("账户 %s 未找到交易所客户端", accountName)
		}
		if normalizedPlatform != "" && client.Platform() != normalizedPlatform {
			return nil, fmt.Errorf("账户 %s 的平台是 %s，不是 %s", accountName, client.Platform(), normalizedPlatform)
		}
		return client, nil
	}

	for _, client := range tm.exchangeClients {
		if client != nil && client.Platform() == normalizedPlatform {
			return client, nil
		}
	}
	return nil, fmt.Errorf("未找到平台 %s 的账户客户端", normalizedPlatform)
}
