package utils

import (
	"testing"
)

// 测试配置常量（AutoTradeWebClient专用）
const (
	testWebServerURL       = "http://localhost:8899"
	testWebCookie          = "sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229367817%22%2C%22first_id%22%3A%2219a33f550c9ed1-04efb4b5c3758-1e525631-2073600-19a33f550ca1cd4%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E7%9B%B4%E6%8E%A5%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC_%E7%9B%B4%E6%8E%A5%E6%89%93%E5%BC%80%22%2C%22%24latest_referrer%22%3A%22%22%7D%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTlhMzNmNTUwYzllZDEtMDRlZmI0YjVjMzc1OC0xZTUyNTYzMS0yMDczNjAwLTE5YTMzZjU1MGNhMWNkNCIsIiRpZGVudGl0eV9sb2dpbl9pZCI6IjkzNjc4MTciLCJpZGVudGl0eV9oNV9pZCI6InBjLWQ4MTc0YzY3ZWE2OGQ3NzY3NmFiODY5NzUwNTc0OTI5In0%3D%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%229367817%22%7D%2C%22%24device_id%22%3A%2219a33f550c9ed1-04efb4b5c3758-1e525631-2073600-19a33f550ca1cd4%22%7D"
	testWebToken           = "j4JsIpfm/XB/FWpRzptOFNy5GHla2WYEStdTg1BgpAvVa3xgJvuQhE0lkdnM1vCaVJXvFuwMKAo2oaoqhkw+cw=="
	testWebSentryRelease   = "f5961c3cd7be3b342b06e5ffed516be705faa5c2"
	testWebSentryPublicKey = "1e6f92c4133350179d7853f8bb1d8fb7"
	testWebLoginID         = "9367817"
)

var testWebClient *AutoTradeWebClient

// 初始化测试Web客户端
func init() {
	testWebClient = NewAutoTradeWebClient(
		testWebServerURL,
		testWebCookie,
		testWebToken,
		testWebSentryRelease,
		testWebSentryPublicKey,
	)
}

// ============================= 基础功能测试 =============================

// TestNewAutoTradeWebClient 测试创建客户端
func TestNewAutoTradeWebClient(t *testing.T) {
	client := NewAutoTradeWebClient(
		testWebServerURL,
		testWebCookie,
		testWebToken,
		testWebSentryRelease,
		testWebSentryPublicKey,
	)

	if client == nil {
		t.Fatal("创建AutoTradeWebClient失败")
	}

	if client.baseURL != testWebServerURL {
		t.Errorf("baseURL错误: got=%s, want=%s", client.baseURL, testWebServerURL)
	}

	if client.cookie != testWebCookie {
		t.Errorf("cookie未正确设置")
	}
}

// ============================= 接口1: 原样的下单 =============================

// TestAutoTradeWebClient_SendOrderInsert 测试原样下单接口
func TestAutoTradeWebClient_SendOrderInsert(t *testing.T) {
	req := &WebOrderRequest{
		InstrumentID:   "BTCUSDT",
		Volume:         1,
		Direction:      "0", // 买入
		OrderPriceType: "0", // 限价
		Price:          "70512.8",
		OffsetFlag:     "0", // 开仓
		IsCrossMargin:  1,   // 全仓
		Lever:          20,
	}

	resp, err := testWebClient.SendOrderInsert(req)
	if err != nil {
		t.Logf("下单失败（可能是服务未启动或凭证无效）: %v", err)
		return
	}

	t.Logf("✅ 下单成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)
	t.Logf("  Data条数: %d", len(resp.Data))

	// 尝试获取订单数据
	if orderData, err := resp.GetOrderData(); err == nil {
		t.Logf("  订单ID: %s", orderData.OrderSysID)
		t.Logf("  成交价: %.2f", orderData.TradePrice)
		t.Logf("  成交量: %d", orderData.VolumeTraded)
		t.Logf("  方向: %s", orderData.Direction)
	}

	// 尝试获取持仓数据
	if positionData, err := resp.GetPositionData(); err == nil {
		t.Logf("  持仓ID: %s", positionData.PositionID)
		t.Logf("  持仓数量: %d", positionData.Position)
		t.Logf("  开仓均价: %.2f", positionData.OpenPrice)
		t.Logf("  占用保证金: %.4f", positionData.UseMargin)
	}
}

// TestAutoTradeWebClient_SendOrderInsert_Market 测试市价下单
func TestAutoTradeWebClient_SendOrderInsert_Market(t *testing.T) {
	req := &WebOrderRequest{
		InstrumentID:   "BTCUSDT",
		Volume:         1,
		Direction:      "0", // 买入
		OrderPriceType: "4", // 市价
		Price:          "",  // 市价不需要价格
		OffsetFlag:     "0", // 开仓
		IsCrossMargin:  1,   // 全仓
		Lever:          20,
	}

	resp, err := testWebClient.SendOrderInsert(req)
	if err != nil {
		t.Logf("市价下单失败: %v", err)
		return
	}

	t.Logf("✅ 市价下单成功: Code=%d, Msg=%s", resp.Code, resp.Msg)
}

// ============================= 接口2: MarketBuyLong =============================

// TestAutoTradeWebClient_MarketBuyLong 测试市价做多
func TestAutoTradeWebClient_MarketBuyLong(t *testing.T) {
	resp, err := testWebClient.MarketBuyLong("BTCUSDT", 1, 125, 1)
	if err != nil {
		t.Logf("市价做多失败: %v", err)
		return
	}

	t.Logf("✅ 市价做多成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)
	t.Logf("  Data条数: %d", len(resp.Data))

	// 获取订单详情
	if orderData, err := resp.GetOrderData(); err == nil {
		t.Logf("  订单ID: %s", orderData.OrderSysID)
		t.Logf("  成交价: %.2f", orderData.TradePrice)
		t.Logf("  成交量: %d", orderData.VolumeTraded)
		t.Logf("  杠杆: %d", orderData.Leverage)
	}

	// 获取持仓详情
	if positionData, err := resp.GetPositionData(); err == nil {
		t.Logf("  持仓ID: %s", positionData.PositionID)
		t.Logf("  持仓量: %d", positionData.Position)
		t.Logf("  均价: %.2f", positionData.OpenPrice)
		t.Logf("  保证金: %.4f", positionData.UseMargin)
	}
}

// TestAutoTradeWebClient_MarketBuyLong_IsolatedMargin 测试逐仓市价做多
func TestAutoTradeWebClient_MarketBuyLong_IsolatedMargin(t *testing.T) {
	resp, err := testWebClient.MarketBuyLong("BTCUSDT", 1, 10, 0) // 0=逐仓
	if err != nil {
		t.Logf("逐仓市价做多失败: %v", err)
		return
	}

	t.Logf("✅ 逐仓市价做多成功: Code=%d", resp.Code)
}

// ============================= 接口3: MarketSellShort =============================

// TestAutoTradeWebClient_MarketSellShort 测试市价做空
func TestAutoTradeWebClient_MarketSellShort(t *testing.T) {
	resp, err := testWebClient.MarketSellShort("BTCUSDT", 1, 20, 1)
	if err != nil {
		t.Logf("市价做空失败: %v", err)
		return
	}

	t.Logf("✅ 市价做空成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)
	t.Logf("  Data条数: %d", len(resp.Data))

	// 获取订单详情
	if orderData, err := resp.GetOrderData(); err == nil {
		t.Logf("  订单ID: %s", orderData.OrderSysID)
		t.Logf("  成交价: %.2f", orderData.TradePrice)
		t.Logf("  成交量: %d", orderData.VolumeTraded)
	}

	// 获取持仓详情
	if positionData, err := resp.GetPositionData(); err == nil {
		t.Logf("  持仓ID: %s", positionData.PositionID)
		t.Logf("  持仓量: %d", positionData.Position)
		t.Logf("  均价: %.2f", positionData.OpenPrice)
	}
}

// TestAutoTradeWebClient_MarketSellShort_IsolatedMargin 测试逐仓市价做空
func TestAutoTradeWebClient_MarketSellShort_IsolatedMargin(t *testing.T) {
	resp, err := testWebClient.MarketSellShort("BTCUSDT", 1, 10, 0) // 0=逐仓
	if err != nil {
		t.Logf("逐仓市价做空失败: %v", err)
		return
	}

	t.Logf("✅ 逐仓市价做空成功: Code=%d", resp.Code)
}

// ============================= 接口4: SendTradeRiskRequest =============================

// TestAutoTradeWebClient_SendTradeRiskRequest 测试下单风控请求
func TestAutoTradeWebClient_SendTradeRiskRequest(t *testing.T) {
	req := &WebTradeRiskRequest{
		LoginID:               testWebLoginID,
		InstrumentIDName:      "BTCUSDT",
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "开平仓模式",
		TradeType:             "买入",
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             125,
		LeverageK:             125,
		TradePrice:            71952.8,
		TradeVolume:           1,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           "全仓合仓",
		IsReduceOnly:          false,
		CoinType:              "CNY",
		LanguageType:          "简体中文",
		EnvPlatform:           "web_desktop",
	}

	err := testWebClient.SendTradeRiskRequest(req)
	if err != nil {
		t.Logf("风控请求失败: %v", err)
		return
	}

	t.Logf("✅ 风控请求发送成功")
}

// TestAutoTradeWebClient_SendTradeRiskRequest_Short 测试做空风控请求
func TestAutoTradeWebClient_SendTradeRiskRequest_Short(t *testing.T) {
	req := &WebTradeRiskRequest{
		LoginID:               testWebLoginID,
		InstrumentIDName:      "BTCUSDT",
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "开平仓模式",
		TradeType:             "卖出",
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             20,
		LeverageK:             20,
		TradePrice:            71000.0,
		TradeVolume:           1,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           "全仓合仓",
		IsReduceOnly:          false,
	}

	err := testWebClient.SendTradeRiskRequest(req)
	if err != nil {
		t.Logf("做空风控请求失败: %v", err)
		return
	}

	t.Logf("✅ 做空风控请求发送成功")
}

// ============================= 组合方法测试 =============================

// TestAutoTradeWebClient_MarketBuyLongWithRisk 测试市价做多+风控组合
func TestAutoTradeWebClient_MarketBuyLongWithRisk(t *testing.T) {
	resp, err := testWebClient.MarketBuyLongWithRisk(
		"BTCUSDT",
		1,   // volume
		125, // lever
		1,   // isCrossMargin (全仓)
		testWebLoginID,
		71952.8, // tradePrice
	)

	if err != nil {
		t.Logf("市价做多+风控失败: %v", err)
		return
	}

	t.Logf("✅ 市价做多+风控成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)
}

// TestAutoTradeWebClient_MarketSellShortWithRisk 测试市价做空+风控组合
func TestAutoTradeWebClient_MarketSellShortWithRisk(t *testing.T) {
	resp, err := testWebClient.MarketSellShortWithRisk(
		"BTCUSDT",
		4,   // volume
		125, // lever
		1,   // isCrossMargin (全仓)
		testWebLoginID,
		71000.0, // tradePrice
	)

	if err != nil {
		t.Logf("市价做空+风控失败: %v", err)
		return
	}

	t.Logf("✅ 市价做空+风控成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)
}

// ============================= 完整流程测试 =============================

// TestAutoTradeWebClient_FullTradeFlow 测试完整交易流程：下单 -> 风控
func TestAutoTradeWebClient_FullTradeFlow(t *testing.T) {
	t.Log("========== 步骤1: 市价做多开仓 ==========")

	// 1. 下单
	orderResp, err := testWebClient.MarketBuyLong("BTCUSDT", 1, 20, 1)
	if err != nil {
		t.Logf("下单失败: %v", err)
		t.Skip("跳过后续测试")
		return
	}

	t.Logf("✅ 下单成功: Code=%d", orderResp.Code)

	// 2. 发送风控
	t.Log("========== 步骤2: 发送下单风控 ==========")

	riskReq := &WebTradeRiskRequest{
		LoginID:               testWebLoginID,
		InstrumentIDName:      "BTCUSDT",
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "开平仓模式",
		TradeType:             "买入",
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             20,
		LeverageK:             20,
		TradePrice:            71952.8,
		TradeVolume:           1,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           "全仓合仓",
		IsReduceOnly:          false,
	}

	err = testWebClient.SendTradeRiskRequest(riskReq)
	if err != nil {
		t.Logf("风控请求失败（不影响测试）: %v", err)
	} else {
		t.Log("✅ 风控请求成功")
	}

	t.Log("========== 完整流程测试完成 ==========")
}

// ============================= 接口5: SendTriggerOrderInsert =============================

// TestAutoTradeWebClient_SendTriggerOrderInsert 测试设置止盈止损（直接调用）
func TestAutoTradeWebClient_SendTriggerOrderInsert(t *testing.T) {
	req := &WebTriggerOrderRequest{
		InstrumentID:   "BTCUSDT",
		Direction:      "0",   // 多仓
		TPTriggerPrice: 75000, // 止盈触发价
		SLTriggerPrice: 68000, // 止损触发价
		Volume:         1,     // 数量
		TPPrice:        75000, // 止盈委托价
		SLPrice:        68000, // 止损委托价
		IsCrossMargin:  1,     // 全仓
		BusinessType:   "X",
	}

	resp, err := testWebClient.SendTriggerOrderInsert(req)
	if err != nil {
		t.Logf("设置止盈止损失败: %v", err)
		return
	}

	t.Logf("✅ 设置止盈止损成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)

	if len(resp.Data) > 0 {
		t.Logf("  订单ID: %s", resp.Data[0].Data.OrderSysID)
		t.Logf("  止盈触发价: %.1f", resp.Data[0].Data.TPTriggerPrice)
		t.Logf("  止损触发价: %.1f", resp.Data[0].Data.SLTriggerPrice)
	}
}

// ============================= 接口6: SendTPSLRiskRequest =============================

// TestAutoTradeWebClient_SendTPSLRiskRequest 测试止盈止损风控请求（直接调用）
func TestAutoTradeWebClient_SendTPSLRiskRequest(t *testing.T) {
	req := &WebTPSLRiskRequest{
		LoginID:          testWebLoginID,
		InstrumentIDName: "BTCUSDT",
		Success:          "true",
		TPSLVolumeType:   "全部",
		TPSLTradeMode:    "市价",
		TPActionType:     "价格",
		TPTriggerPercent: "",
		TPTriggerPrice:   "75000.0",
		SLActionType:     "价格",
		SLTriggerPercent: "",
		SLTriggerPrice:   "68000.0",
		TPSlider:         false,
		SLSlider:         false,
		VolumeSlider:     false,
		CoinType:         "CNY",
		LanguageType:     "简体中文",
		EnvPlatform:      "web_desktop",
	}

	err := testWebClient.SendTPSLRiskRequest(req)
	if err != nil {
		t.Logf("止盈止损风控请求失败: %v", err)
		return
	}

	t.Logf("✅ 止盈止损风控请求发送成功")
}

// ============================= 组合方法测试: 止盈止损+风控 =============================

// TestAutoTradeWebClient_SendTriggerOrderInsertWithRisk 测试止盈止损+风控组合
func TestAutoTradeWebClient_SendTriggerOrderInsertWithRisk(t *testing.T) {
	resp, err := testWebClient.SendTriggerOrderInsertWithRisk(
		"BTCUSDT",
		"0",     // direction: 多仓
		75000.0, // tpTriggerPrice
		68000.0, // slTriggerPrice
		1,       // volume
		75000.0, // tpPrice
		68000.0, // slPrice
		1,       // isCrossMargin: 全仓
		testWebLoginID,
	)

	if err != nil {
		t.Logf("设置止盈止损+风控失败: %v", err)
		return
	}

	t.Logf("✅ 设置止盈止损+风控成功:")
	t.Logf("  Code: %d", resp.Code)
	t.Logf("  Msg: %s", resp.Msg)

	if len(resp.Data) > 0 {
		t.Logf("  订单ID: %s", resp.Data[0].Data.OrderSysID)
		t.Logf("  止盈: %.1f", resp.Data[0].Data.TPTriggerPrice)
		t.Logf("  止损: %.1f", resp.Data[0].Data.SLTriggerPrice)
	}
}

// ============================= 完整TPSL流程测试 =============================

// TestAutoTradeWebClient_FullTPSLFlow 测试完整止盈止损流程：设置 -> 风控
func TestAutoTradeWebClient_FullTPSLFlow(t *testing.T) {
	t.Log("========== 步骤1: 设置止盈止损 ==========")

	// 1. 设置止盈止损
	triggerResp, err := testWebClient.SendTriggerOrderInsert(&WebTriggerOrderRequest{
		InstrumentID:   "BTCUSDT",
		Direction:      "0",
		TPTriggerPrice: 65000,
		SLTriggerPrice: 71000,
		Volume:         1,
		TPPrice:        68000,
		SLPrice:        71000,
		IsCrossMargin:  1,
		BusinessType:   "X",
	})

	if err != nil {
		t.Logf("设置止盈止损失败: %v", err)
		t.Skip("跳过后续测试")
		return
	}

	t.Logf("✅ 设置止盈止损成功: Code=%d", triggerResp.Code)

	// 2. 发送风控
	t.Log("========== 步骤2: 发送止盈止损风控 ==========")

	tpslRiskReq := &WebTPSLRiskRequest{
		LoginID:          testWebLoginID,
		InstrumentIDName: "BTCUSDT",
		Success:          "true",
		TPSLVolumeType:   "全部",
		TPSLTradeMode:    "市价",
		TPActionType:     "价格",
		TPTriggerPercent: "",
		TPTriggerPrice:   "75000.0",
		SLActionType:     "价格",
		SLTriggerPercent: "",
		SLTriggerPrice:   "68000.0",
		TPSlider:         false,
		SLSlider:         false,
		VolumeSlider:     false,
	}

	err = testWebClient.SendTPSLRiskRequest(tpslRiskReq)
	if err != nil {
		t.Logf("止盈止损风控请求失败（不影响测试）: %v", err)
	} else {
		t.Log("✅ 止盈止损风控请求成功")
	}

	t.Log("========== 完整止盈止损流程测试完成 ==========")
}
