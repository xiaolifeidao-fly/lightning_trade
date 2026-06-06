package utils

import (
	"encoding/json"
	"testing"
)

// 测试配置 (请替换为您的实际配置)
const (
	testAPIKey     = "6f0298f5-7dda-4f71-b2ae-1196d4c20ec4"
	testSecretKey  = "30A1F70D920E95DAD49D5ED2E812F99C"
	testPassphrase = "Roc12345678#"
)

func TestNewDeepCoinClient(t *testing.T) {
	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)
	if client == nil {
		t.Fatal("创建DeepCoin客户端失败")
	}
}

func TestGetBalances(t *testing.T) {
	t.Skip("需要配置真实的API凭证")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 获取SWAP账户的USDT余额
	result, err := client.GetBalances("SWAP", "USDT")
	if err != nil {
		t.Fatalf("获取余额失败: %v", err)
	}

	t.Logf("余额结果: %+v", result)
}

func TestGetPositions(t *testing.T) {
	//t.Skip("需要配置真实的API凭证")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 获取SWAP持仓
	result, err := client.GetPositions("SWAP")
	if err != nil {
		t.Fatalf("获取持仓失败: %v", err)
	}

	t.Logf("持仓结果: %+v", result)
}

func TestPlaceOrder(t *testing.T) {
	t.Skip("需要配置真实的API凭证，谨慎测试交易接口")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 市价做多开仓示例
	orderReq := &OrderRequest{
		InstId:      "BTC-USDT-SWAP",
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "market",
		Sz:          "5",
		PosSide:     "long",
		MrgPosition: "merge",
	}

	result, err := client.PlaceOrder(orderReq)
	if err != nil {
		t.Fatalf("下单失败: %v", err)
	}

	t.Logf("下单结果: %+v", result)
}

func TestPlaceTriggerOrder(t *testing.T) {
	t.Skip("需要配置真实的API凭证，谨慎测试交易接口")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 条件单示例：价格达到95000时市价开多，带止盈止损
	triggerReq := &TriggerOrderRequest{
		InstId:          "BTC-USDT-SWAP",
		ProductGroup:    "Swap",
		Sz:              "1",
		Side:            "buy",
		PosSide:         "long",
		IsCrossMargin:   "1",
		OrderType:       "market",
		TriggerPrice:    "95000",
		TriggerPxType:   "last",
		MrgPosition:     "merge",
		TdMode:          "cross",
		TpTriggerPx:     100000, // 止盈触发价
		TpTriggerPxType: "last",
		TpOrdPx:         -1,    // 市价
		SlTriggerPx:     90000, // 止损触发价
		SlTriggerPxType: "last",
		SlOrdPx:         -1, // 市价
	}

	result, err := client.PlaceTriggerOrder(triggerReq)
	if err != nil {
		t.Fatalf("条件单下单失败: %v", err)
	}

	t.Logf("条件单下单结果: %+v", result)
}

func TestClosePositionsByIds(t *testing.T) {
	t.Skip("需要配置真实的API凭证，谨慎测试交易接口")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 平仓示例
	positionIds := []string{"1001063717138767", "1001063717138768"}
	result, err := client.ClosePositionsByIds("SwapU", "BTC-USDT-SWAP", positionIds)
	if err != nil {
		t.Fatalf("平仓失败: %v", err)
	}

	t.Logf("平仓结果: %+v", result)
}

func TestSetPositionSLTP(t *testing.T) {
	t.Skip("需要配置真实的API凭证，谨慎测试交易接口")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 设置止盈止损示例
	sltpReq := &SetPositionSLTPRequest{
		InstType:    "SWAP",
		InstId:      "BTC-USDT-SWAP",
		PosSide:     "long",
		MrgPosition: "merge",
		TdMode:      "cross",
		TpTriggerPx: "107000",
		SlTriggerPx: "102000",
	}

	result, err := client.SetPositionSLTP(sltpReq)
	if err != nil {
		t.Fatalf("设置止盈止损失败: %v", err)
	}

	t.Logf("设置止盈止损结果: %+v", result)
}

func TestCancelPositionSLTP(t *testing.T) {
	t.Skip("需要配置真实的API凭证，谨慎测试交易接口")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 取消止盈止损示例
	result, err := client.CancelPositionSLTP("SWAP", "BTC-USDT-SWAP", "1000762096073860")
	if err != nil {
		t.Fatalf("取消止盈止损失败: %v", err)
	}

	t.Logf("取消止盈止损结果: %+v", result)
}

func TestModifyPositionSLTP(t *testing.T) {
	t.Skip("需要配置真实的API凭证，谨慎测试交易接口")

	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)

	// 修改止盈止损示例
	modifyReq := &ModifyPositionSLTPRequest{
		InstType:    "SWAP",
		InstId:      "BTC-USDT-SWAP",
		OrdId:       "1000596069447069",
		PosSide:     "long",
		MrgPosition: "merge",
		TdMode:      "cross",
		TpTriggerPx: "110000",
		SlTriggerPx: "103000",
	}

	result, err := client.ModifyPositionSLTP(modifyReq)
	if err != nil {
		t.Fatalf("修改止盈止损失败: %v", err)
	}

	t.Logf("修改止盈止损结果: %+v", result)
}

// 测试签名生成
func TestGenerateSignature(t *testing.T) {
	client := NewDeepCoinClient(testAPIKey, testSecretKey, testPassphrase)
	isoTime := "2024-01-01T00:00:00Z"

	// 测试GET请求签名
	//params := map[string]interface{}{
	//	"instType": "SWAP",
	//	"ccy":      "USDT",
	//}
	sig := client.generateSignature("GET", "/deepcoin/account/balances", "", isoTime)
	if sig == "" {
		t.Fatal("签名生成失败")
	}
	t.Logf("GET签名: %s", sig)

	// sig3, err := client.doSign(isoTime, "GET", "/deepcoin/account/balances", "", testSecretKey)
	// if err != nil {
	// 	t.Fatal("签名生成失败: ", err)
	// }
	// if sig3 == "" {
	// 	t.Fatal("签名生成失败")
	// }
	// t.Logf("GET签名: %s", sig3)

	// assert.Equal(t, sig, sig3)

	// 测试POST请求签名
	orderParams := map[string]interface{}{
		"instId":  "BTC-USDT-SWAP",
		"tdMode":  "cross",
		"side":    "buy",
		"ordType": "market",
		"sz":      "5",
	}
	bodyBytes, _ := json.Marshal(orderParams)
	bodyStr := string(bodyBytes)
	sig2 := client.generateSignature("POST", "/deepcoin/trade/order", bodyStr, isoTime)
	if sig2 == "" {
		t.Fatal("签名生成失败")
	}
	t.Logf("POST签名: %s", sig2)
}
