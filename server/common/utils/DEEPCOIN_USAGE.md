# DeepCoin API 客户端使用指南

## 简介

DeepCoin API 客户端封装了 DeepCoin 交易所的 8 个核心接口，提供了完整的账户查询和交易功能。

## 快速开始

### 1. 创建客户端

```go
import "badelay500w/common/utils"

// 创建DeepCoin客户端
client := utils.NewDeepCoinClient(
    "your-api-key",
    "your-secret-key",
    "your-passphrase",
)
```

### 2. 获取账户余额

```go
// 获取合约账户的USDT余额
result, err := client.GetBalances("SWAP", "USDT")
if err != nil {
    log.Fatal(err)
}

// 获取现货账户的所有资产
result, err := client.GetBalances("SPOT")
```

### 3. 获取持仓列表

```go
// 获取所有合约持仓
result, err := client.GetPositions("SWAP")

// 获取指定产品的持仓
result, err := client.GetPositions("SWAP", "BTC-USDT-SWAP")
```

## 交易接口

### 1. 普通下单

#### 市价做多（买入开多）

```go
orderReq := &utils.OrderRequest{
    InstId:      "BTC-USDT-SWAP",
    TdMode:      "cross",        // 全仓
    Side:        "buy",          // 买入
    OrdType:     "market",       // 市价
    Sz:          "5",            // 数量
    PosSide:     "long",         // 做多
    MrgPosition: "merge",        // 合仓
}

result, err := client.PlaceOrder(orderReq)
```

#### 市价平多（卖出平多）

```go
orderReq := &utils.OrderRequest{
    InstId:      "BTC-USDT-SWAP",
    TdMode:      "cross",
    Side:        "sell",         // 卖出
    OrdType:     "market",
    Sz:          "5",
    PosSide:     "long",         // 平多
    MrgPosition: "merge",
}

result, err := client.PlaceOrder(orderReq)
```

#### 限价做空（卖出开空）

```go
orderReq := &utils.OrderRequest{
    InstId:      "BTC-USDT-SWAP",
    TdMode:      "cross",
    Side:        "sell",
    OrdType:     "limit",        // 限价
    Sz:          "1",
    Px:          "35000",        // 价格
    PosSide:     "short",        // 做空
    MrgPosition: "merge",
}

result, err := client.PlaceOrder(orderReq)
```

#### IOC订单（立即成交或撤销）

```go
orderReq := &utils.OrderRequest{
    InstId:      "BTC-USDT-SWAP",
    TdMode:      "cross",
    Side:        "buy",
    OrdType:     "ioc",          // IOC订单
    Sz:          "1",
    Px:          "95000",
    PosSide:     "long",
    MrgPosition: "merge",
}

result, err := client.PlaceOrder(orderReq)
```

### 2. 条件单下单

#### 基础条件单

```go
triggerReq := &utils.TriggerOrderRequest{
    InstId:        "BTC-USDT-SWAP",
    ProductGroup:  "Swap",
    Sz:            "1",
    Side:          "buy",
    PosSide:       "long",
    IsCrossMargin: "1",          // 全仓
    OrderType:     "market",     // 市价
    TriggerPrice:  "95000",      // 触发价
    TriggerPxType: "last",       // 最新价触发
    MrgPosition:   "merge",
    TdMode:        "cross",
}

result, err := client.PlaceTriggerOrder(triggerReq)
```

#### 带止盈止损的条件单

```go
triggerReq := &utils.TriggerOrderRequest{
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
    TpTriggerPx:     100000,     // 止盈触发价
    TpTriggerPxType: "last",
    TpOrdPx:         -1,         // -1表示市价
    SlTriggerPx:     90000,      // 止损触发价
    SlTriggerPxType: "last",
    SlOrdPx:         -1,
}

result, err := client.PlaceTriggerOrder(triggerReq)
```

### 3. 平仓操作

#### 按仓位ID平仓

```go
positionIds := []string{
    "1001063717138767",
    "1001063717138768",
}

result, err := client.ClosePositionsByIds(
    "SwapU",            // U本位合约
    "BTC-USDT-SWAP",
    positionIds,
)
```

## 止盈止损管理

### 1. 设置止盈止损

#### 现货止盈止损

```go
sltpReq := &utils.SetPositionSLTPRequest{
    InstType:    "SPOT",
    InstId:      "BTC-USDT",
    TpTriggerPx: "107000",       // 止盈价
    SlTriggerPx: "102000",       // 止损价
}

result, err := client.SetPositionSLTP(sltpReq)
```

#### 合约止盈止损（合仓模式）

```go
sltpReq := &utils.SetPositionSLTPRequest{
    InstType:        "SWAP",
    InstId:          "BTC-USDT-SWAP",
    PosSide:         "long",
    MrgPosition:     "merge",
    TdMode:          "cross",
    TpTriggerPx:     "107000",
    TpTriggerPxType: "mark",     // 标记价触发
    TpOrdPx:         "-1",       // 市价
    SlTriggerPx:     "102000",
    SlTriggerPxType: "mark",
    SlOrdPx:         "-1",
}

result, err := client.SetPositionSLTP(sltpReq)
```

#### 仅设置止盈

```go
sltpReq := &utils.SetPositionSLTPRequest{
    InstType:    "SPOT",
    InstId:      "BTC-USDT",
    TpTriggerPx: "107000",       // 只设置止盈
}

result, err := client.SetPositionSLTP(sltpReq)
```

### 2. 修改止盈止损

```go
modifyReq := &utils.ModifyPositionSLTPRequest{
    InstType:    "SWAP",
    InstId:      "BTC-USDT-SWAP",
    OrdId:       "1000596069447069",  // 订单ID（从设置接口获取）
    PosSide:     "long",
    MrgPosition: "merge",
    TdMode:      "cross",
    TpTriggerPx: "110000",            // 新的止盈价
    SlTriggerPx: "103000",            // 新的止损价
}

result, err := client.ModifyPositionSLTP(modifyReq)
```

### 3. 取消止盈止损

```go
result, err := client.CancelPositionSLTP(
    "SWAP",                    // 产品类型
    "BTC-USDT-SWAP",          // 产品ID
    "1000762096073860",       // 订单ID
)
```

## 完整使用示例

```go
package main

import (
    "log"
    "badelay500w/common/utils"
)

func main() {
    // 1. 创建客户端
    client := utils.NewDeepCoinClient(
        "your-api-key",
        "your-secret-key",
        "your-passphrase",
    )

    // 2. 查询余额
    balance, err := client.GetBalances("SWAP", "USDT")
    if err != nil {
        log.Printf("查询余额失败: %v", err)
        return
    }
    log.Printf("余额: %+v", balance)

    // 3. 查询持仓
    positions, err := client.GetPositions("SWAP")
    if err != nil {
        log.Printf("查询持仓失败: %v", err)
        return
    }
    log.Printf("持仓: %+v", positions)

    // 4. 下单示例（谨慎使用）
    orderReq := &utils.OrderRequest{
        InstId:      "BTC-USDT-SWAP",
        TdMode:      "cross",
        Side:        "buy",
        OrdType:     "ioc",
        Sz:          "1",
        Px:          "95000",
        PosSide:     "long",
        MrgPosition: "merge",
    }

    orderResult, err := client.PlaceOrder(orderReq)
    if err != nil {
        log.Printf("下单失败: %v", err)
        return
    }
    log.Printf("下单成功: %+v", orderResult)

    // 5. 设置止盈止损
    sltpReq := &utils.SetPositionSLTPRequest{
        InstType:    "SWAP",
        InstId:      "BTC-USDT-SWAP",
        PosSide:     "long",
        MrgPosition: "merge",
        TdMode:      "cross",
        TpTriggerPx: "100000",
        SlTriggerPx: "90000",
    }

    sltpResult, err := client.SetPositionSLTP(sltpReq)
    if err != nil {
        log.Printf("设置止盈止损失败: %v", err)
        return
    }
    log.Printf("止盈止损设置成功: %+v", sltpResult)
}
```

## 参数说明

### 产品类型 (InstType/ProductGroup)
- `SPOT`: 现货
- `SWAP`: 合约

### 交易模式 (TdMode)
- `cash`: 非保证金（现货）
- `cross`: 全仓
- `isolated`: 逐仓

### 订单类型 (OrdType)
- `market`: 市价单
- `limit`: 限价单
- `post_only`: 只做maker单
- `ioc`: 立即成交或撤销单

### 持仓方向 (PosSide)
- `long`: 多头
- `short`: 空头

### 仓位模式 (MrgPosition)
- `merge`: 合仓
- `split`: 分仓

### 触发价类型 (TriggerPxType)
- `last`: 最新价
- `index`: 指数价
- `mark`: 标记价

## 错误处理

所有接口都返回 `(map[string]interface{}, error)`，建议检查：

```go
result, err := client.GetBalances("SWAP", "USDT")
if err != nil {
    log.Printf("API调用失败: %v", err)
    return
}

// 检查响应code
if code, ok := result["code"].(string); ok && code != "0" {
    log.Printf("API返回错误: %v", result["msg"])
    return
}

// 检查data中的sCode
if data, ok := result["data"].(map[string]interface{}); ok {
    if sCode, ok := data["sCode"].(string); ok && sCode != "0" {
        log.Printf("操作失败: %v", data["sMsg"])
        return
    }
}
```

## 注意事项

1. **API凭证安全**：请妥善保管API Key、Secret Key和Passphrase
2. **交易谨慎**：所有交易接口都会产生实际交易，测试时请使用小额资金
3. **限频规则**：大部分接口限频为每秒1次，请注意控制调用频率
4. **参数验证**：下单前请确保参数正确，特别是价格精度和数量精度
5. **止盈止损**：设置止盈止损后请保存返回的ordId，用于后续修改或取消

## 相关文档

- [DeepCoin API官方文档](https://docs.deepcoin.com)
- Python参考实现：`python/main_script (1).py`
