# Trade API 使用文档

## 概述

本包提供了 Deepcoin 交易平台的四个主要接口：
1. **交易风控接口** - `SendTradeRiskRequest`: 发送下单风控埋点数据
2. **交易接口** - `SendOrderInsert`: 发送实际的交易订单
3. **止盈止损接口** - `SendTriggerOrderInsert`: 为持仓设置止盈止损
4. **止盈止损风控接口** - `SendTPSLRiskRequest`: 发送设置止盈止损的风控埋点数据

## 安装依赖

```go
import (
    "argus/pkg/trade/user"
    "argus/pkg/trade/web"
)
```

## 1. 风控接口使用

### 功能说明
在实际下单前，需要先发送风控埋点数据，用于风险控制和数据分析。

### 使用示例

```go
// 创建用户对象
u := user.NewUser(
    "sensorsdata2015jssdkcross=...; i18next=zh; theme=dark",  // Cookie
    "",  // Token（风控接口不需要）
    "",  // SentryRelease
    "",  // SentryPublicKey
)

// 构造风控请求
req := &web.TradeRiskRequest{
    Username:              "test_user",
    LoginID:               "9533715",
    InstrumentIDName:      "BTCUSDT",
    InstrumentIDPerpetual: "USDT永续",
    OrderMode:             "开平仓模式",
    TradeType:             "买入",
    HoldType:              "开仓",
    TradeMode:             "限价",
    LeverageD:             125,
    LeverageK:             125,
    TradePrice:            76429.5,
    TradeVolume:           1,
    TPSLPrice:             []string{"-", "-"},
    TradeSource:           "限价",
    MarginModel:           "全仓合仓",
    IsReduceOnly:          false,
}

// 发送风控请求
err := web.SendTradeRiskRequest(u, req)
if err != nil {
    log.Printf("风控请求失败: %v", err)
}
```

## 2. 交易接口使用

### 功能说明
发送实际的交易订单到交易所。

### 用户对象创建

```go
u := user.NewUser(
    cookie,         // Cookie字符串
    token,          // Token（必需，从登录接口获取）
    sentryRelease,  // Sentry Release ID（可选）
    sentryPublicKey,// Sentry Public Key（可选）
)
```

### 订单方向说明

| Direction | 说明 | OffsetFlag |
|-----------|------|------------|
| "1" | 买入开多 | "0" (开仓) |
| "2" | 卖出开空 | "0" (开仓) |
| "3" | 卖出平多 | "1" (平仓) |
| "4" | 买入平空 | "1" (平仓) |

### 订单类型说明

| OrderPriceType | 说明 |
|----------------|------|
| "2" | 限价单 |
| "4" | 市价单 |

### 保证金模式

| IsCrossMargin | 说明 |
|---------------|------|
| 1 | 全仓模式 |
| 0 | 逐仓模式 |

### 示例1: 限价买入开多

```go
orderReq := &web.OrderRequest{
    InstrumentID:   "BTCUSDT",     // 交易对
    Volume:         1,              // 数量
    Direction:      "1",            // 买入开多
    OrderPriceType: "2",            // 限价单
    Price:          "71952.8",      // 限价价格
    OffsetFlag:     "0",            // 开仓
    IsCrossMargin:  1,              // 全仓模式
    Lever:          20,             // 20倍杠杆
}

resp, err := web.SendOrderInsert(u, orderReq)
if err != nil {
    log.Printf("下单失败: %v", err)
    return
}

log.Printf("下单成功，订单ID: %s", resp.Data.OrderID)
```

### 示例2: 市价买入开多

```go
orderReq := &web.OrderRequest{
    InstrumentID:   "BTCUSDT",
    Volume:         1,
    Direction:      "1",            // 买入开多
    OrderPriceType: "4",            // 市价单
    Price:          "",             // 市价单不需要价格
    OffsetFlag:     "0",            // 开仓
    IsCrossMargin:  1,
    Lever:          20,
}

resp, err := web.SendOrderInsert(u, orderReq)
```

### 示例3: 卖出开空

```go
orderReq := &web.OrderRequest{
    InstrumentID:   "BTCUSDT",
    Volume:         1,
    Direction:      "2",            // 卖出开空
    OrderPriceType: "2",            // 限价单
    Price:          "71000.0",      // 限价价格
    OffsetFlag:     "0",            // 开仓
    IsCrossMargin:  1,
    Lever:          20,
}

resp, err := web.SendOrderInsert(u, orderReq)
```

### 示例4: 卖出平多（平仓）

```go
orderReq := &web.OrderRequest{
    InstrumentID:   "BTCUSDT",
    Volume:         1,
    Direction:      "3",            // 卖出平多
    OrderPriceType: "4",            // 市价平仓
    Price:          "",
    OffsetFlag:     "1",            // 平仓
    IsCrossMargin:  1,
    Lever:          20,
}

resp, err := web.SendOrderInsert(u, orderReq)
```

### 示例5: 买入平空（平仓）

```go
orderReq := &web.OrderRequest{
    InstrumentID:   "BTCUSDT",
    Volume:         1,
    Direction:      "4",            // 买入平空
    OrderPriceType: "4",            // 市价平仓
    Price:          "",
    OffsetFlag:     "1",            // 平仓
    IsCrossMargin:  1,
    Lever:          20,
}

resp, err := web.SendOrderInsert(u, orderReq)
```

## 完整交易流程

```go
package main

import (
    "log"
    "argus/pkg/trade/user"
    "argus/pkg/trade/web"
)

func main() {
    // 1. 创建用户对象
    u := user.NewUser(
        "sensorsdata2015jssdkcross=...; i18next=en; theme=dark",
        "your-token-here",
        "ec3532632e2f7f732d67d8160eb8cf7069281616",
        "1e6f92c4133350179d7853f8bb1d8fb7",
    )

    // 2. 发送风控请求（可选，但建议发送）
    riskReq := &web.TradeRiskRequest{
        Username:              "user1",
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
    }
    
    if err := web.SendTradeRiskRequest(u, riskReq); err != nil {
        log.Printf("风控请求失败: %v", err)
    }

    // 3. 发送交易订单
    orderReq := &web.OrderRequest{
        InstrumentID:   "BTCUSDT",
        Volume:         1,
        Direction:      "1",
        OrderPriceType: "2",
        Price:          "71952.8",
        OffsetFlag:     "0",
        IsCrossMargin:  1,
        Lever:          20,
    }

    resp, err := web.SendOrderInsert(u, orderReq)
    if err != nil {
        log.Fatalf("下单失败: %v", err)
    }

    log.Printf("下单成功！订单ID: %s", resp.Data.OrderID)
}
```

## 响应结构

```go
type OrderResponse struct {
    Code    int    `json:"code"`      // 0表示成功，其他值表示失败
    Message string `json:"message"`   // 响应消息
    Data    struct {
        OrderID string `json:"order_id"` // 订单ID
    } `json:"data"`
}
```

## 错误处理

```go
resp, err := web.SendOrderInsert(u, orderReq)
if err != nil {
    // 网络错误或请求失败
    log.Printf("请求失败: %v", err)
    return
}

if resp.Code != 0 {
    // 业务逻辑错误（如余额不足、参数错误等）
    log.Printf("下单失败: %s (code: %d)", resp.Message, resp.Code)
    return
}

// 下单成功
log.Printf("订单ID: %s", resp.Data.OrderID)
```

## 3. 止盈止损接口使用

### 功能说明
为已有的持仓设置止盈止损价格，当市场价格触及设定价格时自动平仓。

### 持仓方向说明

| Direction | 说明 |
|-----------|------|
| "0" | 多仓持仓 |
| "1" | 空仓持仓 |

### 使用示例

#### 示例1: 为多仓设置止盈止损

```go
// 创建用户对象
u := user.NewUser(
    cookie,
    token,
    sentryRelease,
    sentryPublicKey,
)

// 为多仓设置止盈止损
triggerReq := &web.TriggerOrderRequest{
    InstrumentID:   "BTCUSDT",  // 交易对
    Direction:      "0",         // 0=多仓
    TPTriggerPrice: 66070.1,     // 止盈价格（多仓时低于当前价）
    SLTriggerPrice: 83305.8,     // 止损价格（多仓时高于当前价）
    IsCrossMargin:  1,           // 1=全仓
}

resp, err := web.SendTriggerOrderInsert(u, triggerReq)
if err != nil {
    log.Printf("设置止盈止损失败: %v", err)
    return
}

if len(resp.Data) > 0 {
    log.Printf("止盈止损设置成功，订单ID: %s", resp.Data[0].Data.OrderSysID)
    log.Printf("止盈价格: %.1f, 止损价格: %.1f", 
        resp.Data[0].Data.TPTriggerPrice,
        resp.Data[0].Data.SLTriggerPrice)
}
```

#### 示例2: 为空仓设置止盈止损

```go
triggerReq := &web.TriggerOrderRequest{
    InstrumentID:   "BTCUSDT",
    Direction:      "1",         // 1=空仓
    TPTriggerPrice: 80000.0,     // 止盈价格（空仓时高于当前价）
    SLTriggerPrice: 65000.0,     // 止损价格（空仓时低于当前价）
    IsCrossMargin:  1,
}

resp, err := web.SendTriggerOrderInsert(u, triggerReq)
```

#### 示例3: 只设置止盈（不设置止损）

```go
triggerReq := &web.TriggerOrderRequest{
    InstrumentID:   "BTCUSDT",
    Direction:      "0",
    TPTriggerPrice: 70000.0,  // 只设置止盈
    SLTriggerPrice: 0,         // 0表示不设置止损
    IsCrossMargin:  1,
}

resp, err := web.SendTriggerOrderInsert(u, triggerReq)
```

#### 示例4: 只设置止损（不设置止盈）

```go
triggerReq := &web.TriggerOrderRequest{
    InstrumentID:   "BTCUSDT",
    Direction:      "0",
    TPTriggerPrice: 0,         // 0表示不设置止盈
    SLTriggerPrice: 60000.0,   // 只设置止损
    IsCrossMargin:  1,
}

resp, err := web.SendTriggerOrderInsert(u, triggerReq)
```

### 止盈止损响应结构

```go
type TriggerOrderResponse struct {
    Code    int    `json:"code"`      // 0表示成功
    Message string `json:"msg"`       // 响应消息
    Data    []struct {
        Table string           `json:"table"`
        Data  TriggerOrderData `json:"data"`  // 订单详情
    } `json:"data"`
}

type TriggerOrderData struct {
    OrderSysID      string  `json:"OrderSysID"`      // 订单ID
    InstrumentID    string  `json:"InstrumentID"`    // 交易对
    Direction       string  `json:"Direction"`       // 持仓方向
    TPTriggerPrice  float64 `json:"TPTriggerPrice"`  // 止盈价格
    SLTriggerPrice  float64 `json:"SLTriggerPrice"`  // 止损价格
    TriggerStatus   string  `json:"TriggerStatus"`   // 触发状态
    // ... 其他字段
}
```

### 止盈止损注意事项

1. **价格方向**: 
   - 多仓：止盈价格 < 当前价格 < 止损价格
   - 空仓：止损价格 < 当前价格 < 止盈价格
2. **持仓要求**: 只能为已有持仓设置止盈止损
3. **同时设置**: 可以同时设置止盈和止损，也可以只设置其中一个
4. **修改订单**: 如需修改，需要先取消原订单再重新设置

## 4. 止盈止损风控接口使用

### 功能说明
在设置止盈止损时发送风控埋点数据，用于数据分析和风险控制。建议在调用 `SendTriggerOrderInsert` 后立即调用此接口。

### 使用示例

#### 示例1: 使用价格设置止盈止损的风控埋点

```go
// 创建用户对象（风控接口不需要token）
u := user.NewUser(
    "sensorsdata2015jssdkcross=...; i18next=zh; theme=dark",
    "",  // 风控接口不需要token
    "",
    "",
)

// 发送止盈止损风控请求
tpslRiskReq := &web.TPSLRiskRequest{
    Username:         "test_user",
    LoginID:          "9542187",
    InstrumentIDName: "BTCUSDT",
    Success:          "true",          // 设置是否成功
    TPSLVolumeType:   "全部",          // 数量类型：全部/部分
    TPSLTradeMode:    "市价",          // 交易模式：市价/限价
    TPActionType:     "收益率",        // 止盈触发类型：收益率/价格
    TPTriggerPercent: "",              // 止盈百分比（空表示使用价格）
    TPTriggerPrice:   "66070.1",       // 止盈价格
    SLActionType:     "收益率",        // 止损触发类型：收益率/价格
    SLTriggerPercent: "",              // 止损百分比（空表示使用价格）
    SLTriggerPrice:   "83305.8",       // 止损价格
    TPSlider:         false,           // 是否使用滑块设置止盈
    SLSlider:         false,           // 是否使用滑块设置止损
    VolumeSlider:     false,           // 是否使用滑块设置数量
}

err := web.SendTPSLRiskRequest(u, tpslRiskReq)
if err != nil {
    log.Printf("发送止盈止损风控请求失败: %v", err)
}
```

#### 示例2: 使用百分比设置止盈止损的风控埋点

```go
tpslRiskReq := &web.TPSLRiskRequest{
    Username:         "test_user",
    LoginID:          "9542187",
    InstrumentIDName: "BTCUSDT",
    Success:          "true",
    TPSLVolumeType:   "全部",
    TPSLTradeMode:    "市价",
    TPActionType:     "收益率",        // 使用百分比
    TPTriggerPercent: "10",            // 止盈10%
    TPTriggerPrice:   "",              // 使用百分比时价格为空
    SLActionType:     "收益率",
    SLTriggerPercent: "5",             // 止损5%
    SLTriggerPrice:   "",              // 使用百分比时价格为空
    TPSlider:         true,            // 使用滑块设置
    SLSlider:         true,
    VolumeSlider:     false,
}

err := web.SendTPSLRiskRequest(u, tpslRiskReq)
```

#### 示例3: 只设置止盈的风控埋点

```go
tpslRiskReq := &web.TPSLRiskRequest{
    Username:         "test_user",
    LoginID:          "9542187",
    InstrumentIDName: "BTCUSDT",
    Success:          "true",
    TPSLVolumeType:   "全部",
    TPSLTradeMode:    "市价",
    TPActionType:     "价格",
    TPTriggerPercent: "",
    TPTriggerPrice:   "70000.0",       // 只设置止盈
    SLActionType:     "收益率",
    SLTriggerPercent: "",
    SLTriggerPrice:   "",              // 不设置止损
    TPSlider:         false,
    SLSlider:         false,
    VolumeSlider:     false,
}

err := web.SendTPSLRiskRequest(u, tpslRiskReq)
```

### 完整流程示例（设置止盈止损 + 风控埋点）

```go
// 1. 创建用户对象
u := user.NewUser(cookie, token, sentryRelease, sentryPublicKey)

// 2. 发送止盈止损订单
triggerReq := &web.TriggerOrderRequest{
    InstrumentID:   "BTCUSDT",
    Direction:      "0",
    TPTriggerPrice: 66070.1,
    SLTriggerPrice: 83305.8,
    IsCrossMargin:  1,
}

resp, err := web.SendTriggerOrderInsert(u, triggerReq)
if err != nil {
    log.Printf("设置止盈止损失败: %v", err)
    return
}

// 3. 发送风控埋点（记录用户操作）
tpslRiskReq := &web.TPSLRiskRequest{
    Username:         "test_user",
    LoginID:          "9542187",
    InstrumentIDName: "BTCUSDT",
    Success:          "true",          // 设置成功
    TPSLVolumeType:   "全部",
    TPSLTradeMode:    "市价",
    TPActionType:     "收益率",
    TPTriggerPercent: "",
    TPTriggerPrice:   "66070.1",
    SLActionType:     "收益率",
    SLTriggerPercent: "",
    SLTriggerPrice:   "83305.8",
    TPSlider:         false,
    SLSlider:         false,
    VolumeSlider:     false,
}

err = web.SendTPSLRiskRequest(u, tpslRiskReq)
if err != nil {
    log.Printf("发送风控埋点失败: %v", err)
}
```

### 参数说明

| 参数 | 类型 | 说明 |
|------|------|------|
| InstrumentIDName | string | 交易对名称，如 "BTCUSDT" |
| Success | string | 设置是否成功，"true"/"false" |
| TPSLVolumeType | string | 数量类型："全部"/"部分" |
| TPSLTradeMode | string | 交易模式："市价"/"限价" |
| TPActionType | string | 止盈触发类型："收益率"/"价格" |
| TPTriggerPercent | string | 止盈触发百分比（使用百分比时填写，否则为空） |
| TPTriggerPrice | string | 止盈触发价格（使用价格时填写，否则为空） |
| SLActionType | string | 止损触发类型："收益率"/"价格" |
| SLTriggerPercent | string | 止损触发百分比（使用百分比时填写，否则为空） |
| SLTriggerPrice | string | 止损触发价格（使用价格时填写，否则为空） |
| TPSlider | bool | 是否使用滑块设置止盈 |
| SLSlider | bool | 是否使用滑块设置止损 |
| VolumeSlider | bool | 是否使用滑块设置数量 |

## 注意事项

1. **Token 必需**: 交易接口必须提供有效的 Token，可以从登录接口获取
2. **Cookie 必需**: 需要包含 `sensorsdata2015jssdkcross` 等完整的 cookie 信息
3. **签名计算**: 签名算法已内置，会自动计算请求的 sign 和 hmac
4. **杠杆设置**: 确保杠杆倍数符合交易所规则
5. **价格精度**: 不同交易对的价格精度可能不同，请注意价格格式
6. **风控建议**: 建议在下单前先调用风控接口，模拟真实用户行为

## 常见问题

### Q: 如何获取 Token？
A: Token 需要通过登录接口获取，通常在登录成功后会返回。

### Q: Cookie 从哪里获取？
A: Cookie 可以从浏览器的开发者工具中复制，或者通过登录接口的响应头获取。

### Q: 签名计算失败怎么办？
A: 签名算法已经内置实现，如果仍然失败，请检查：
- 请求参数是否完整
- 时间戳是否正确
- 是否缺少必需字段

### Q: 如何测试接口？
A: 可以运行测试文件：
```bash
go test -v argus/pkg/trade/web
```

## 更新日志

- 2026-02-05: 初始版本，支持交易风控、交易、止盈止损和止盈止损风控接口
