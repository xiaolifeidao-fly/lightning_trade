package utils

import (
	"fmt"
	"math/rand"
	"testing"
)

// 测试配置常量，统一复用（AutoTradeClient专用）
const (
	testServerURL           = "http://localhost:8899"
	autoTradeTestAPIKey     = "8251da7c-ed24-4aad-bd03-484ffdf0af61"
	autoTradeTestSecretKey  = "956381CBFA3058A9D59E23BE8BB1A8FC"
	autoTradeTestPassphrase = "Aa111111@"
)

var testClient *AutoTradeClient

// 初始化测试客户端
func init() {
	testClient = NewAutoTradeClient(
		testServerURL,
		autoTradeTestAPIKey,
		autoTradeTestSecretKey,
		autoTradeTestPassphrase,
	)
}

func TestFunction(t *testing.T) {
	i := rand.Intn(2)
	fmt.Println(i)
}

// ============================= 业务方法测试 =============================

// TestAutoTradeClient_PlaceOrder 测试下单功能
func TestAutoTradeClient_PlaceOrder(t *testing.T) {
	req := &OrderRequest{
		InstId:      "BTC-USDT-SWAP",
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "market",
		Sz:          "1",
		PosSide:     "long",
		MrgPosition: "merge",
	}

	resp, err := testClient.PlaceOrder(req)
	t.Logf("PlaceOrder 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_PlaceTriggerOrder 测试条件单下单功能
func TestAutoTradeClient_PlaceTriggerOrder(t *testing.T) {
	req := &TriggerOrderRequest{
		InstId:       "BTC-USDT-SWAP",
		ProductGroup: "Swap",
		Sz:           "1",
		Side:         "buy",
		OrderType:    "limit",
		TriggerPrice: "95000",
		TdMode:       "cross",
		PosSide:      "long",
		Price:        "95000",
	}

	resp, err := testClient.PlaceTriggerOrder(req)
	t.Logf("PlaceTriggerOrder 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_GetBalances 测试获取余额功能
func TestAutoTradeClient_GetBalances(t *testing.T) {
	req := &GetBalancesRequest{
		InstType: "SWAP",
		Ccy:      "USDT",
	}

	resp, err := testClient.GetBalances(req)
	t.Logf("GetBalances 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_GetPositions 测试获取持仓功能
func TestAutoTradeClient_GetPositions(t *testing.T) {
	req := &GetPositionsRequest{
		InstType: "SWAP",
		InstId:   "BTC-USDT-SWAP",
	}
	// PosId:1001119889234250

	resp, err := testClient.GetPositions(req)
	t.Logf("GetPositions 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_ClosePositions 测试平仓功能
func TestAutoTradeClient_ClosePositions(t *testing.T) {
	req := &ClosePositionsRequest{
		ProductGroup: "SwapU",
		InstId:       "BTC-USDT-SWAP",
		PositionIds:  []string{"1001119889115929"},
	}

	resp, err := testClient.ClosePositions(req)
	t.Logf("ClosePositions 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_SetPositionSLTP 测试设置止盈止损功能
func TestAutoTradeClient_SetPositionSLTP(t *testing.T) {
	req := &SetPositionSLTPRequest{
		InstType:    "SWAP",
		InstId:      "BTC-USDT-SWAP",
		PosSide:     "long",
		MrgPosition: "merge",
		TdMode:      "cross",
		TpTriggerPx: "82000",
		TpOrdPx:     "82000",
		SlTriggerPx: "73000",
		SlOrdPx:     "73000",
		Sz:          "2",
	}

	resp, err := testClient.SetPositionSLTP(req)
	t.Logf("SetPositionSLTP 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_ModifyPositionSLTP 测试修改止盈止损功能 暂不测试
func TestAutoTradeClient_ModifyPositionSLTP(t *testing.T) {
	req := &ModifyPositionSLTPRequest{
		InstType:    "SWAP",
		InstId:      "BTC-USDT-SWAP",
		OrdId:       "sltp123",
		TpTriggerPx: "105000",
		SlTriggerPx: "95000",
	}

	resp, err := testClient.ModifyPositionSLTP(req)
	t.Logf("ModifyPositionSLTP 结果: resp=%+v, err=%v", resp, err)
}

// TestAutoTradeClient_CancelPositionSLTP 测试取消止盈止损功能 暂不测试
func TestAutoTradeClient_CancelPositionSLTP(t *testing.T) {
	req := &CancelPositionSLTPRequest{
		InstType: "SWAP",
		InstId:   "BTC-USDT-SWAP",
		OrdId:    "sltp123",
	}

	resp, err := testClient.CancelPositionSLTP(req)
	t.Logf("CancelPositionSLTP 结果: resp=%+v, err=%v", resp, err)
}

// TestMarketBuyLong 测试市价买入做多功能
func TestMarketBuyLong(t *testing.T) {
	req := &QuickOrderRequest{
		InstId: "BTC-USDT-SWAP",
		Size:   "2",
	}

	resp, err := testClient.MarketBuyLong(req)
	t.Logf("MarketBuyLong 结果: resp=%+v, err=%v", resp, err)
}

// TestMarketSellShort 测试市价卖出开空功能 暂不测试
func TestMarketSellShort(t *testing.T) {
	req := &QuickOrderRequest{
		InstId: "BTC-USDT-SWAP",
		Size:   "3",
	}

	resp, err := testClient.MarketSellShort(req)
	t.Logf("MarketSellShort 结果: resp=%+v, err=%v", resp, err)
}

// TestIOCBuyLong 测试IOC买入做多功能 暂不测试
func TestIOCBuyLong(t *testing.T) {
	req := &QuickOrderRequest{
		InstId: "BTC-USDT-SWAP",
		Size:   "1",
		Price:  "95000",
	}

	resp, err := testClient.IOCBuyLong(req)
	t.Logf("IOCBuyLong 结果: resp=%+v, err=%v", resp, err)
}

// TestIOCSellShort 测试IOC卖出做空功能 暂不测试
func TestIOCSellShort(t *testing.T) {
	req := &QuickOrderRequest{
		InstId: "BTC-USDT-SWAP",
		Size:   "1",
		Price:  "95000",
	}

	resp, err := testClient.IOCSellShort(req)
	t.Logf("IOCSellShort 结果: resp=%+v, err=%v", resp, err)
}

// TestGetPositionIdsByInstId 测试根据交易对获取持仓ID列表功能
func TestGetPositionIdsByInstId(t *testing.T) {
	posIds, err := testClient.GetPositionIdsByInstId("BTC-USDT-SWAP")
	t.Logf("GetPositionIdsByInstId 结果: posIds=%v, err=%v", posIds, err)
}

// TestCloseAllPositions 测试平仓所有持仓功能
func TestCloseAllPositions(t *testing.T) {
	resp, err := testClient.CloseAllPositions("BTC-USDT-SWAP")
	t.Logf("CloseAllPositions 结果: resp=%+v, err=%v", resp, err)
}

// TestArbitrageTrade 测试套利交易功能 待 review
func TestArbitrageTrade(t *testing.T) {
	req := &ArbitrageTradeRequest{
		InstId:  "BTC-USDT-SWAP",
		Size:    "1",
		Price:   "95000",
		BuyDeep: true,
	}

	resp, err := testClient.ArbitrageTrade(req)
	t.Logf("ArbitrageTrade 结果: resp=%+v, err=%v", resp, err)
}
