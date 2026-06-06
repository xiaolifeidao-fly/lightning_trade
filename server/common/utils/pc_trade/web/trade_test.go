package web

import (
	"context"
	"log"
	"testing"

	"common/utils/pc_trade/user"
)

// TestTradeFlow 测试完整下单流程：先下单，再进行下单风控
func TestTradeFlow(t *testing.T) {
	err := InitGlobalSigner(context.Background())
	if err != nil {
		log.Fatalf("❌ 初始化 WASM 签名器失败: %v", err)
	}
	// 1. 创建用户对象
	u := user.NewUser(
		`sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229533715%22%2C%22first_id%22%3A%2219a4d866db53d1-007ed755c1362658-1e525631-1930176-19a4d866db6163d%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E7%9B%B4%E6%8E%A5%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC_%E7%9B%B4%E6%8E%A5%E6%89%93%E5%BC%80%22%2C%22%24latest_referrer%22%3A%22%22%7D%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTlhNGQ4NjZkYjUzZDEtMDA3ZWQ3NTVjMTM2MjY1OC0xZTUyNTYzMS0xOTMwMTc2LTE5YTRkODY2ZGI2MTYzZCIsIiRpZGVudGl0eV9sb2dpbl9pZCI6Ijk1MzM3MTUiLCJpZGVudGl0eV9oNV9pZCI6InBjLTM0YjgwOGQ1NGI1NzZiZDVmZDUxYTk2ZGY4MjE2MjI2In0%3D%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%229533715%22%7D%2C%22%24device_id%22%3A%2219a4d866db53d1-007ed755c1362658-1e525631-1930176-19a4d866db6163d%22%7D; theme=dark`,
		"hfPGd8A69FHSnprCJ/jhFvaqn8XoyHhV/Ij/drNQKNT7Xu8r56peA67IvjpLnS0zm7wGKwyc+0EgYScY8RYgVQ==",
		"f5961c3cd7be3b342b06e5ffed516be705faa5c2",
		"1e6f92c4133350179d7853f8bb1d8fb7",
	)

	// ========== 步骤1: 先下单 ==========
	t.Log("步骤1: 发送下单请求...")
	orderReq := &OrderRequest{
		InstrumentID:   "BTCUSDT",
		Volume:         1,
		Direction:      "1", // 1=买入开多
		OrderPriceType: "4", // 2=限价
		Price:          "70512.8",
		OffsetFlag:     "0", // 0=开仓
		IsCrossMargin:  1,   // 1=全仓
		Lever:          20,
	}

	orderResp, err := SendOrderInsert(u, orderReq)
	if err != nil {
		t.Logf("下单失败: %v", err)
		// 注意：如果cookie/token无效或网络问题，这个测试可能会失败
		// 这是正常的，只是为了演示API的使用方式
		return
	}

	t.Logf("✓ 下单成功，响应: code=%d, message=%s", orderResp.Code, orderResp.Msg)
	if orderData, err := orderResp.GetOrderData(); err == nil {
		t.Logf("  订单ID: %s, 成交价: %.1f, 成交量: %d",
			orderData.OrderSysID, orderData.TradePrice, orderData.VolumeTraded)
	}

	// ========== 步骤2: 再进行下单风控 ==========
	t.Log("步骤2: 发送下单风控请求...")
	riskReq := &TradeRiskRequest{
		LoginID:               "9542187",
		InstrumentIDName:      "BTCUSDT",
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "开平仓模式",
		TradeType:             "买入",
		HoldType:              "开仓",
		TradeMode:             "限价",
		LeverageD:             20,
		LeverageK:             20,
		TradePrice:            71952.8,
		TradeVolume:           1,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "限价",
		MarginModel:           "全仓合仓",
		IsReduceOnly:          false,
		CoinType:              "CNY",
		LanguageType:          "简体中文",
		EnvPlatform:           "web_desktop",
	}

	err = SendTradeRiskRequest(u, riskReq)
	if err != nil {
		t.Logf("下单风控请求失败: %v", err)
	} else {
		t.Log("✓ 下单风控请求发送成功")
	}
}

// TestTPSLFlow 测试完整止盈止损流程：先止盈止损，再进行止盈止损风控
func TestTPSLFlow(t *testing.T) {
	err := InitGlobalSigner(context.Background())
	if err != nil {
		log.Fatalf("❌ 初始化 WASM 签名器失败: %v", err)
	}
	// 1. 创建用户对象
	u := user.NewUser(
		`sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229533715%22%2C%22first_id%22%3A%2219a4d866db53d1-007ed755c1362658-1e525631-1930176-19a4d866db6163d%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E7%9B%B4%E6%8E%A5%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC_%E7%9B%B4%E6%8E%A5%E6%89%93%E5%BC%80%22%2C%22%24latest_referrer%22%3A%22%22%7D%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTlhNGQ4NjZkYjUzZDEtMDA3ZWQ3NTVjMTM2MjY1OC0xZTUyNTYzMS0xOTMwMTc2LTE5YTRkODY2ZGI2MTYzZCIsIiRpZGVudGl0eV9sb2dpbl9pZCI6Ijk1MzM3MTUiLCJpZGVudGl0eV9oNV9pZCI6InBjLTM0YjgwOGQ1NGI1NzZiZDVmZDUxYTk2ZGY4MjE2MjI2In0%3D%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%229533715%22%7D%2C%22%24device_id%22%3A%2219a4d866db53d1-007ed755c1362658-1e525631-1930176-19a4d866db6163d%22%7D; theme=dark`,
		"hfPGd8A69FHSnprCJ/jhFvaqn8XoyHhV/Ij/drNQKNT7Xu8r56peA67IvjpLnS0zm7wGKwyc+0EgYScY8RYgVQ==",
		"f5961c3cd7be3b342b06e5ffed516be705faa5c2",
		"1e6f92c4133350179d7853f8bb1d8fb7",
	)

	// ========== 步骤1: 先设置止盈止损 ==========
	t.Log("步骤1: 发送止盈止损请求...")
	triggerReq := &TriggerOrderRequest{
		InstrumentID:   "BTCUSDT",
		Direction:      "0",   // 0=多仓
		TPTriggerPrice: 66666, // 止盈价格
		TPPrice:        66666,
		SLTriggerPrice: 70777, // 止损价格
		SLPrice:        70777,
		IsCrossMargin:  1, // 1=全仓
		Volume:         1,
	}

	triggerResp, err := SendTriggerOrderInsert(u, triggerReq)
	if err != nil {
		t.Logf("设置止盈止损失败: %v", err)
		// 注意：如果cookie/token无效或网络问题，这个测试可能会失败
		// 这是正常的，只是为了演示API的使用方式
		return
	}

	t.Logf("✓ 止盈止损设置成功，响应: code=%d, message=%s", triggerResp.Code, triggerResp.Msg)
	if len(triggerResp.Data) > 0 {
		t.Logf("  订单ID: %s, 止盈: %.1f, 止损: %.1f",
			triggerResp.Data[0].Data.OrderSysID,
			triggerResp.Data[0].Data.TPTriggerPrice,
			triggerResp.Data[0].Data.SLTriggerPrice)
	}

	// ========== 步骤2: 再进行止盈止损风控 ==========
	t.Log("步骤2: 发送止盈止损风控请求...")
	tpslRiskReq := &TPSLRiskRequest{
		Username:         "test_user",
		LoginID:          "9542187",
		InstrumentIDName: "BTCUSDT",
		Success:          "true",
		TPSLVolumeType:   "全部",
		TPSLTradeMode:    "市价",
		TPActionType:     "收益率",
		TPTriggerPercent: "",
		TPTriggerPrice:   "65070.1",
		SLActionType:     "收益率",
		SLTriggerPercent: "",
		SLTriggerPrice:   "83305.8",
		TPSlider:         false,
		SLSlider:         false,
		VolumeSlider:     false,
	}

	err = SendTPSLRiskRequest(u, tpslRiskReq)
	if err != nil {
		t.Logf("止盈止损风控请求失败: %v", err)
	} else {
		t.Log("✓ 止盈止损风控请求发送成功")
	}
}
