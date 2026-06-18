package trade

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

var (
	globalManager *TradeManager
)

func InitTradeManager(config *TradingSystemConfig) {
	if globalManager != nil {
		logrus.Infof("交易管理器已初始化，跳过重复初始化")
		return
	}
	globalManager = NewTradeManager(config)
	logrus.Infof("✅ 交易管理器已初始化: %d个账户", len(config.Accounts))
}

func GetManager() *TradeManager {
	return globalManager
}

func IsInitialized() bool {
	return globalManager != nil
}

// EnsureSessionsReady 启动时主动检测所有账户 session，失效则触发无头登录。
func EnsureSessionsReady() {
	if globalManager == nil {
		logrus.Warn("交易管理器未初始化，跳过 session 检测")
		return
	}
	globalManager.EnsureSessionsReady()
}

// ============================= 套利交易 =============================

func ExecuteArbitrage(instId string, binPrice, deepPrice float64) error {
	if globalManager == nil {
		logrus.Warnf("交易管理器未初始化，跳过交易")
		return nil
	}
	return globalManager.ExecuteArbitrage_From_WEB(instId, binPrice, deepPrice)
}

// ============================= 账户状态 =============================

func GetAccountStatus() map[string]interface{} {
	if globalManager == nil {
		return map[string]interface{}{
			"error": "交易管理器未初始化",
		}
	}
	return globalManager.GetAccountStatus()
}

// GetKlinesByPlatform 按平台查询 K 线。示例：platform=binance/deepcoin，symbol=BTCUSDT 或 instId=BTC-USDT-SWAP。
func GetKlinesByPlatform(ctx context.Context, platform string, req MarketKlineRequest) ([]MarketKline, error) {
	return QueryKlinesByPlatform(ctx, platform, req)
}

// GetTickerByPlatform 按平台查询实时价格。
func GetTickerByPlatform(ctx context.Context, platform string, req MarketTickerRequest) (*MarketTicker, error) {
	return QueryTickerByPlatform(ctx, platform, req)
}

// GetRecentTradesByPlatform 按平台查询最近逐笔（聚合）成交。
func GetRecentTradesByPlatform(ctx context.Context, platform string, req MarketTradeRequest) ([]MarketTrade, error) {
	return QueryRecentTradesByPlatform(ctx, platform, req)
}

// GetFundingRateByPlatform 按平台查询合约资金费率快照。
func GetFundingRateByPlatform(ctx context.Context, platform string, req FundingRateRequest) (*FundingRateSnapshot, error) {
	return QueryFundingRateByPlatform(ctx, platform, req)
}

// GetBalancesByPlatform 按平台和账户查询余额。
func GetBalancesByPlatform(ctx context.Context, platform, accountName string, req BalanceRequest) ([]Balance, error) {
	if globalManager == nil {
		return nil, ErrTradeManagerNotInitialized()
	}
	return globalManager.GetBalancesByPlatform(ctx, platform, accountName, req)
}

// PlaceOrderByPlatform 按平台和账户路由统一下单。
func PlaceOrderByPlatform(ctx context.Context, platform, accountName string, req ExchangeOrderRequest) (*ExchangeOrderResponse, error) {
	if globalManager == nil {
		return nil, ErrTradeManagerNotInitialized()
	}
	return globalManager.PlaceOrderByPlatform(ctx, platform, accountName, req)
}

func ErrTradeManagerNotInitialized() error {
	return errTradeManagerNotInitialized{}
}

type errTradeManagerNotInitialized struct{}

func (errTradeManagerNotInitialized) Error() string {
	return "交易管理器未初始化"
}

// ============================= 配置管理 =============================

func SetCooldown(seconds int) {
	if globalManager != nil {
		globalManager.SetCooldown(time.Duration(seconds) * time.Second)
		logrus.Infof("交易冷却时间已设置: %ds", seconds)
	}
}

func GetConfig() *TradingSystemConfig {
	if globalManager == nil {
		return nil
	}
	return globalManager.config
}
