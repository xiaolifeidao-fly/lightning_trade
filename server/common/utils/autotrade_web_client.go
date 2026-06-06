package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	DefaultAutoTradeWebURL     = "http://localhost:8899"
	DefaultAutoTradeWebTimeout = 30 * time.Second
)

// ============================= Web请求/响应类型 =============================

// WebOrderRequest Web端下单请求
type WebOrderRequest struct {
	InstrumentID   string `json:"InstrumentID"`
	Volume         int    `json:"Volume"`
	Direction      string `json:"Direction"`      // "0"=买入, "1"=卖出
	OrderPriceType string `json:"OrderPriceType"` // "0"=限价, "4"=市价
	Price          string `json:"Price"`
	OffsetFlag     string `json:"OffsetFlag"`    // "0"=开仓, "1"=平仓
	IsCrossMargin  int    `json:"IsCrossMargin"` // 1=全仓, 0=逐仓
	Lever          int    `json:"Lever"`
	ExchangeID     string `json:"ExchangeID,omitempty"`
	AppID          int    `json:"AppID,omitempty"`
	ConvertPOST    int    `json:"ConvertPOST,omitempty"`
}

// WebOrderResponse Web端下单响应
type WebOrderResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data []WebOrderResponseData `json:"data"`
}

// WebOrderResponseData 响应数据项
type WebOrderResponseData struct {
	Table string      `json:"table"`
	Data  interface{} `json:"data"`
}

// WebOrderData 订单详细数据
type WebOrderData struct {
	APPID                   string  `json:"APPID"`
	AccountID               string  `json:"AccountID"`
	AskPrice1ByInsert       float64 `json:"AskPrice1ByInsert"`
	Available               int     `json:"Available"`
	BidPrice1ByInsert       float64 `json:"BidPrice1ByInsert"`
	BusinessNo              int64   `json:"BusinessNo"`
	BusinessResult          string  `json:"BusinessResult"`
	BusinessType            string  `json:"BusinessType"`
	BusinessValue           string  `json:"BusinessValue"`
	CFDGrade                string  `json:"CFDGrade"`
	CFDPrice                float64 `json:"CFDPrice"`
	CloseOrderID            string  `json:"CloseOrderID"`
	CloseProfit             float64 `json:"CloseProfit"`
	CopyMemberID            string  `json:"CopyMemberID"`
	CopyOrderID             string  `json:"CopyOrderID"`
	CopyProfit              float64 `json:"CopyProfit"`
	CostMode                string  `json:"CostMode"`
	Currency                string  `json:"Currency"`
	DeriveDetail            string  `json:"DeriveDetail"`
	DeriveSource            string  `json:"DeriveSource"`
	Direction               string  `json:"Direction"`
	ExchangeID              string  `json:"ExchangeID"`
	Fee                     float64 `json:"Fee"`
	FrontNo                 int     `json:"FrontNo"`
	FrozenFee               float64 `json:"FrozenFee"`
	FrozenMargin            float64 `json:"FrozenMargin"`
	FrozenMoney             float64 `json:"FrozenMoney"`
	InsertTime              int64   `json:"InsertTime"`
	InstrumentID            string  `json:"InstrumentID"`
	IsCrossMargin           int     `json:"IsCrossMargin"`
	LastPriceByInsert       float64 `json:"LastPriceByInsert"`
	Leverage                int     `json:"Leverage"`
	LocalID                 string  `json:"LocalID"`
	MemberID                string  `json:"MemberID"`
	MinVolume               int     `json:"MinVolume"`
	OffsetFlag              string  `json:"OffsetFlag"`
	OpenPrice               float64 `json:"OpenPrice"`
	OrderPriceType          string  `json:"OrderPriceType"`
	OrderRemark             string  `json:"OrderRemark"`
	OrderStatus             string  `json:"OrderStatus"`
	OrderSysID              string  `json:"OrderSysID"`
	OrderType               string  `json:"OrderType"`
	PosiDirection           string  `json:"PosiDirection"`
	Position                int     `json:"Position"`
	PositionID              string  `json:"PositionID"`
	Price                   float64 `json:"Price"`
	ProductGroup            string  `json:"ProductGroup"`
	RelatedOrderSysID       string  `json:"RelatedOrderSysID"`
	Remark                  string  `json:"Remark"`
	SessionNo               int     `json:"SessionNo"`
	TheoryAskPrice1ByInsert float64 `json:"TheoryAskPrice1ByInsert"`
	TheoryBidPrice1ByInsert float64 `json:"TheoryBidPrice1ByInsert"`
	TheoryPriceByInsert     float64 `json:"TheoryPriceByInsert"`
	TimeCondition           string  `json:"TimeCondition"`
	TradePrice              float64 `json:"TradePrice"`
	TradeUnitID             string  `json:"TradeUnitID"`
	TriggerOrderID          string  `json:"TriggerOrderID"`
	Turnover                float64 `json:"Turnover"`
	UpdateMilliTime         int64   `json:"UpdateMilliTime"`
	UpdateTime              int64   `json:"UpdateTime"`
	UserID                  string  `json:"UserID"`
	Volume                  int     `json:"Volume"`
	VolumeCancled           int     `json:"VolumeCancled"`
	VolumeMode              string  `json:"VolumeMode"`
	VolumeRemain            int     `json:"VolumeRemain"`
	VolumeTraded            int     `json:"VolumeTraded"`
}

// WebPositionData 持仓详细数据
type WebPositionData struct {
	AccountID         string  `json:"AccountID"`
	BeginTime         int64   `json:"BeginTime"`
	BusinessNo        int64   `json:"BusinessNo"`
	BusinessType      string  `json:"BusinessType"`
	BusinessValue     string  `json:"BusinessValue"`
	ClearCurrency     string  `json:"ClearCurrency"`
	CloseOrderID      string  `json:"CloseOrderID"`
	CloseOrderSysID   string  `json:"CloseOrderSysID"`
	ClosePosition     int     `json:"ClosePosition"`
	CloseProfit       float64 `json:"CloseProfit"`
	CopyMemberID      string  `json:"CopyMemberID"`
	CopyProfit        float64 `json:"CopyProfit"`
	CostPrice         float64 `json:"CostPrice"`
	CreateTime        string  `json:"CreateTime"`
	Currency          string  `json:"Currency"`
	ExchangeID        string  `json:"ExchangeID"`
	FirstTradeID      string  `json:"FirstTradeID"`
	Frequency         int     `json:"Frequency"`
	FrozenMargin      float64 `json:"FrozenMargin"`
	HighestPosition   int     `json:"HighestPosition"`
	InsertTime        int64   `json:"InsertTime"`
	InstrumentID      string  `json:"InstrumentID"`
	IsCrossMargin     int     `json:"IsCrossMargin"`
	LastTradeID       string  `json:"LastTradeID"`
	Leverage          int     `json:"Leverage"`
	LongFrozen        int     `json:"LongFrozen"`
	LongFrozenMargin  float64 `json:"LongFrozenMargin"`
	MemberID          string  `json:"MemberID"`
	OpenPrice         float64 `json:"OpenPrice"`
	PosiDirection     string  `json:"PosiDirection"`
	Position          int     `json:"Position"`
	PositionCost      float64 `json:"PositionCost"`
	PositionFee       float64 `json:"PositionFee"`
	PositionID        string  `json:"PositionID"`
	PreLongFrozen     int     `json:"PreLongFrozen"`
	PrePosition       int     `json:"PrePosition"`
	PreShortFrozen    int     `json:"PreShortFrozen"`
	PriceCurrency     string  `json:"PriceCurrency"`
	ProductGroup      string  `json:"ProductGroup"`
	ProductID         string  `json:"ProductID"`
	Remark            string  `json:"Remark"`
	SettlementGroup   string  `json:"SettlementGroup"`
	ShortFrozen       int     `json:"ShortFrozen"`
	ShortFrozenMargin float64 `json:"ShortFrozenMargin"`
	TotalCloseProfit  float64 `json:"TotalCloseProfit"`
	TotalPositionCost float64 `json:"TotalPositionCost"`
	TradeFee          float64 `json:"TradeFee"`
	TradeUnitID       string  `json:"TradeUnitID"`
	UpdateTime        int64   `json:"UpdateTime"`
	UseMargin         float64 `json:"UseMargin"`
	UserID            string  `json:"UserID"`
}

// GetOrderData 从响应中获取订单数据
func (r *WebOrderResponse) GetOrderData() (*WebOrderData, error) {
	for _, item := range r.Data {
		if item.Table == "Order" {
			// 将 interface{} 转换为 WebOrderData
			jsonData, err := json.Marshal(item.Data)
			if err != nil {
				return nil, fmt.Errorf("序列化订单数据失败: %w", err)
			}
			var orderData WebOrderData
			if err := json.Unmarshal(jsonData, &orderData); err != nil {
				return nil, fmt.Errorf("反序列化订单数据失败: %w", err)
			}
			return &orderData, nil
		}
	}
	return nil, fmt.Errorf("响应中未找到订单数据")
}

// GetPositionData 从响应中获取持仓数据
func (r *WebOrderResponse) GetPositionData() (*WebPositionData, error) {
	for _, item := range r.Data {
		if item.Table == "Position" {
			// 将 interface{} 转换为 WebPositionData
			jsonData, err := json.Marshal(item.Data)
			if err != nil {
				return nil, fmt.Errorf("序列化持仓数据失败: %w", err)
			}
			var positionData WebPositionData
			if err := json.Unmarshal(jsonData, &positionData); err != nil {
				return nil, fmt.Errorf("反序列化持仓数据失败: %w", err)
			}
			return &positionData, nil
		}
	}
	return nil, fmt.Errorf("响应中未找到持仓数据")
}

// WebTradeRiskRequest Web端下单风控请求
type WebTradeRiskRequest struct {
	LoginID               string   `json:"LoginID"`
	InstrumentIDName      string   `json:"InstrumentIDName"`
	InstrumentIDPerpetual string   `json:"InstrumentIDPerpetual"`
	OrderMode             string   `json:"OrderMode"`
	TradeType             string   `json:"TradeType"`
	HoldType              string   `json:"HoldType"`
	TradeMode             string   `json:"TradeMode"`
	LeverageD             int      `json:"LeverageD"`
	LeverageK             int      `json:"LeverageK"`
	TradePrice            float64  `json:"TradePrice"`
	TradeVolume           int      `json:"TradeVolume"`
	TPSLPrice             []string `json:"TPSLPrice"`
	TradeSource           string   `json:"TradeSource"`
	MarginModel           string   `json:"MarginModel"`
	IsReduceOnly          bool     `json:"IsReduceOnly"`
	CoinType              string   `json:"CoinType,omitempty"`
	LanguageType          string   `json:"LanguageType,omitempty"`
	EnvPlatform           string   `json:"EnvPlatform,omitempty"`
}

// WebTradeRiskResponse Web端风控响应
type WebTradeRiskResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Message string `json:"message"`
}

// WebTriggerOrderRequest Web端止盈止损订单请求
type WebTriggerOrderRequest struct {
	InstrumentID   string  `json:"InstrumentID"`
	Direction      string  `json:"Direction"`      // 持仓方向: "0"=买入, "1"=卖出 平多仓传1 平空仓传0
	TPTriggerPrice float64 `json:"TPTriggerPrice"` // 止盈触发价格
	SLTriggerPrice float64 `json:"SLTriggerPrice"` // 止损触发价格
	Volume         int     `json:"Volume"`         // 交易数量
	TPPrice        float64 `json:"TPPrice"`        // 止盈委托价格
	SLPrice        float64 `json:"SLPrice"`        // 止损委托价格
	IsCrossMargin  int     `json:"IsCrossMargin"`  // 1=全仓, 0=逐仓
	ExchangeID     string  `json:"ExchangeID,omitempty"`
	OrderPriceType string  `json:"OrderPriceType,omitempty"`
	OffsetFlag     string  `json:"OffsetFlag,omitempty"`
	Source         string  `json:"Source,omitempty"`
	Remark         string  `json:"Remark,omitempty"`
	BusinessType   string  `json:"BusinessType,omitempty"`
	AppID          int     `json:"AppID,omitempty"`
	ConvertPOST    int     `json:"ConvertPOST,omitempty"`
}

// WebTriggerOrderResponse Web端止盈止损订单响应
type WebTriggerOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Table string              `json:"table"`
		Data  WebTriggerOrderData `json:"data"`
	} `json:"data"`
}

// WebTriggerOrderData 触发订单数据
type WebTriggerOrderData struct {
	APPID           string  `json:"APPID"`
	AccountID       string  `json:"AccountID"`
	BusinessType    string  `json:"BusinessType"`
	Direction       string  `json:"Direction"`
	ExchangeID      string  `json:"ExchangeID"`
	InsertTime      int64   `json:"InsertTime"`
	InstrumentID    string  `json:"InstrumentID"`
	IsCrossMargin   int     `json:"IsCrossMargin"`
	MemberID        string  `json:"MemberID"`
	OffsetFlag      string  `json:"OffsetFlag"`
	OrderPriceType  string  `json:"OrderPriceType"`
	OrderSysID      string  `json:"OrderSysID"`
	Remark          string  `json:"Remark"`
	SLTriggerPrice  float64 `json:"SLTriggerPrice"`
	Source          string  `json:"Source"`
	TPTriggerPrice  float64 `json:"TPTriggerPrice"`
	TradeUnitID     string  `json:"TradeUnitID"`
	UpdateMilliTime int64   `json:"UpdateMilliTime"`
	UpdateTime      int64   `json:"UpdateTime"`
	UserID          string  `json:"UserID"`
}

// WebTPSLRiskRequest Web端止盈止损风控请求
type WebTPSLRiskRequest struct {
	LoginID          string `json:"LoginID"`
	InstrumentIDName string `json:"InstrumentIDName"`
	Success          string `json:"Success"`
	TPSLVolumeType   string `json:"TPSLVolumeType"`
	TPSLTradeMode    string `json:"TPSLTradeMode"`
	TPActionType     string `json:"TPActionType"`
	TPTriggerPercent string `json:"TPTriggerPercent"`
	TPTriggerPrice   string `json:"TPTriggerPrice"`
	SLActionType     string `json:"SLActionType"`
	SLTriggerPercent string `json:"SLTriggerPercent"`
	SLTriggerPrice   string `json:"SLTriggerPrice"`
	TPSlider         bool   `json:"TPSlider"`
	SLSlider         bool   `json:"SLSlider"`
	VolumeSlider     bool   `json:"VolumeSlider"`
	CoinType         string `json:"CoinType,omitempty"`
	LanguageType     string `json:"LanguageType,omitempty"`
	EnvPlatform      string `json:"EnvPlatform,omitempty"`
}

// WebTPSLRiskResponse Web端止盈止损风控响应
type WebTPSLRiskResponse struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Message string `json:"message"`
}

// AutoTradeWebClient PC Web端交易客户端
type AutoTradeWebClient struct {
	baseURL         string
	cookie          string
	token           string
	sentryRelease   string
	sentryPublicKey string
	client          *http.Client
}

// NewAutoTradeWebClient 创建AutoTrade Web客户端
// baseURL: AutoTrade服务地址，如 "http://localhost:8899"
// cookie: 用户Cookie
// token: 用户Token
// sentryRelease: Sentry Release
// sentryPublicKey: Sentry Public Key
func NewAutoTradeWebClient(baseURL, cookie, token, sentryRelease, sentryPublicKey string) *AutoTradeWebClient {
	if baseURL == "" {
		baseURL = DefaultAutoTradeWebURL
	}

	return &AutoTradeWebClient{
		baseURL:         baseURL,
		cookie:          cookie,
		token:           token,
		sentryRelease:   sentryRelease,
		sentryPublicKey: sentryPublicKey,
		client: &http.Client{
			Timeout: DefaultAutoTradeWebTimeout,
		},
	}
}

// doRequest 执行HTTP请求
func (c *AutoTradeWebClient) doRequest(method, path string, reqBody interface{}, respBody interface{}) error {
	url := c.baseURL + path
	var req *http.Request
	var err error

	if method == "GET" {
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return fmt.Errorf("创建请求失败: %w", err)
		}

		if reqBody != nil {
			if params, ok := reqBody.(map[string]string); ok {
				q := req.URL.Query()
				for k, v := range params {
					if v != "" {
						q.Add(k, v)
					}
				}
				req.URL.RawQuery = q.Encode()
			}
		}
	} else {
		var bodyBytes []byte
		if reqBody != nil {
			bodyBytes, _ = json.Marshal(reqBody)
		}
		req, err = http.NewRequest(method, url, bytes.NewBuffer(bodyBytes))
		if err != nil {
			return fmt.Errorf("创建请求失败: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
	}

	// 设置用户凭证到Header
	req.Header.Set("X-User-Cookie", c.cookie)
	req.Header.Set("X-User-Token", c.token)
	req.Header.Set("X-User-Sentry-Release", c.sentryRelease)
	req.Header.Set("X-User-Sentry-Public-Key", c.sentryPublicKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 先检查响应中的code字段
	var errorCheck struct {
		Code interface{} `json:"code"`
		Msg  string      `json:"msg"`
	}
	if err := json.Unmarshal(bodyBytes, &errorCheck); err == nil {
		if errorCheck.Code != nil {
			var codeStr string
			switch v := errorCheck.Code.(type) {
			case string:
				codeStr = v
			case float64:
				codeStr = fmt.Sprintf("%.0f", v)
			case int:
				codeStr = fmt.Sprintf("%d", v)
			}

			// code不是"200"或"0"时返回错误
			if codeStr != "200" && codeStr != "0" {
				logrus.Errorf("请求失败: code=%s, msg=%s", codeStr, errorCheck.Msg)
				return fmt.Errorf("请求失败: code=%s, msg=%s", codeStr, errorCheck.Msg)
			}
		}
	}

	// 解析响应体
	if respBody != nil {
		if err := json.Unmarshal(bodyBytes, respBody); err != nil {
			logrus.Errorf("解析响应失败: %v, body=%s", err, string(bodyBytes))
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}

	return nil
}

// ============================= Web交易接口 =============================

// SendOrderInsert 接口1: 发送下单请求（原样的）
// req: 下单请求参数
func (c *AutoTradeWebClient) SendOrderInsert(req *WebOrderRequest) (*WebOrderResponse, error) {
	var resp WebOrderResponse
	err := c.doRequest("POST", "/web/SendOrderInsert", req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// MarketBuyLong 接口2: 市价做多开仓
// instrumentID: 交易对，如 "BTCUSDT"
// volume: 交易数量
// lever: 杠杆倍数
// isCrossMargin: 1=全仓, 0=逐仓
func (c *AutoTradeWebClient) MarketBuyLong(instrumentID string, volume, lever, isCrossMargin int) (*WebOrderResponse, error) {
	req := &WebOrderRequest{
		InstrumentID:   instrumentID,
		Volume:         volume,
		Direction:      "0", // 买入
		OrderPriceType: "4", // 市价
		Price:          "",  // 市价不需要价格
		OffsetFlag:     "0", // 开仓
		IsCrossMargin:  isCrossMargin,
		Lever:          lever,
	}
	return c.SendOrderInsert(req)
}

// MarketSellShort 接口3: 市价做空开仓
// instrumentID: 交易对，如 "BTCUSDT"
// volume: 交易数量
// lever: 杠杆倍数
// isCrossMargin: 1=全仓, 0=逐仓
func (c *AutoTradeWebClient) MarketSellShort(instrumentID string, volume, lever, isCrossMargin int) (*WebOrderResponse, error) {
	req := &WebOrderRequest{
		InstrumentID:   instrumentID,
		Volume:         volume,
		Direction:      "1", // 卖出
		OrderPriceType: "4", // 市价
		Price:          "",  // 市价不需要价格
		OffsetFlag:     "0", // 开仓
		IsCrossMargin:  isCrossMargin,
		Lever:          lever,
	}
	return c.SendOrderInsert(req)
}

// SendTradeRiskRequest 接口4: 发送下单风控请求
// req: 风控请求参数
func (c *AutoTradeWebClient) SendTradeRiskRequest(req *WebTradeRiskRequest) error {
	var resp WebTradeRiskResponse
	err := c.doRequest("POST", "/web/SendTradeRisk", req, &resp)
	if err != nil {
		return err
	}
	return nil
}

// SendTriggerOrderInsert 接口5: 发送止盈止损订单请求
// req: 止盈止损订单请求参数
func (c *AutoTradeWebClient) SendTriggerOrderInsert(req *WebTriggerOrderRequest) (*WebTriggerOrderResponse, error) {
	var resp WebTriggerOrderResponse
	err := c.doRequest("POST", "/web/SendTriggerOrderInsert", req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// SendTPSLRiskRequest 接口6: 发送止盈止损风控请求
// req: 止盈止损风控请求参数
func (c *AutoTradeWebClient) SendTPSLRiskRequest(req *WebTPSLRiskRequest) error {
	var resp WebTPSLRiskResponse
	err := c.doRequest("POST", "/web/SendTPSLRisk", req, &resp)
	if err != nil {
		return err
	}
	return nil
}

// ============================= 便捷组合方法 =============================

// MarketBuyLongWithRisk 市价做多开仓并发送风控（组合方法）
// instrumentID: 交易对，如 "BTCUSDT"
// volume: 交易数量
// lever: 杠杆倍数
// isCrossMargin: 1=全仓, 0=逐仓
// loginID: 登录ID（用于风控）
// tradePrice: 交易价格（用于风控）
func (c *AutoTradeWebClient) MarketBuyLongWithRisk(
	instrumentID string,
	volume, lever, isCrossMargin int,
	loginID string,
	tradePrice float64,
) (*WebOrderResponse, error) {
	// 1. 先下单
	orderResp, err := c.MarketBuyLong(instrumentID, volume, lever, isCrossMargin)
	if err != nil {
		return nil, fmt.Errorf("下单失败: %w", err)
	}

	// 2. 发送风控
	marginModel := "全仓合仓"
	if isCrossMargin == 0 {
		marginModel = "逐仓"
	}

	riskReq := &WebTradeRiskRequest{
		LoginID:               loginID,
		InstrumentIDName:      instrumentID,
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "开平仓模式",
		TradeType:             "买入",
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             lever,
		LeverageK:             lever,
		TradePrice:            tradePrice,
		TradeVolume:           volume,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           marginModel,
		IsReduceOnly:          false,
		CoinType:              "CNY",
		LanguageType:          "简体中文",
		EnvPlatform:           "web_desktop",
	}

	// 异步发送风控请求，不等待返回结果
	go func() {
		if err := c.SendTradeRiskRequest(riskReq); err != nil {
			logrus.Warnf("风控请求失败（不影响下单）: %v", err)
		}
	}()

	return orderResp, nil
}

// MarketSellShortWithRisk 市价做空开仓并发送风控（组合方法）
// instrumentID: 交易对，如 "BTCUSDT"
// volume: 交易数量
// lever: 杠杆倍数
// isCrossMargin: 1=全仓, 0=逐仓
// loginID: 登录ID（用于风控）
// tradePrice: 交易价格（用于风控）
func (c *AutoTradeWebClient) MarketSellShortWithRisk(
	instrumentID string,
	volume, lever, isCrossMargin int,
	loginID string,
	tradePrice float64,
) (*WebOrderResponse, error) {
	// 1. 先下单
	orderResp, err := c.MarketSellShort(instrumentID, volume, lever, isCrossMargin)
	if err != nil {
		return nil, fmt.Errorf("下单失败: %w", err)
	}

	// 2. 发送风控
	marginModel := "全仓合仓"
	if isCrossMargin == 0 {
		marginModel = "逐仓"
	}

	riskReq := &WebTradeRiskRequest{
		LoginID:               loginID,
		InstrumentIDName:      instrumentID,
		InstrumentIDPerpetual: "USDT永续",
		OrderMode:             "开平仓模式",
		TradeType:             "卖出",
		HoldType:              "开仓",
		TradeMode:             "市价",
		LeverageD:             lever,
		LeverageK:             lever,
		TradePrice:            tradePrice,
		TradeVolume:           volume,
		TPSLPrice:             []string{"-", "-"},
		TradeSource:           "市价",
		MarginModel:           marginModel,
		IsReduceOnly:          false,
		CoinType:              "CNY",
		LanguageType:          "简体中文",
		EnvPlatform:           "web_desktop",
	}

	// 异步发送风控请求，不等待返回结果
	go func() {
		if err := c.SendTradeRiskRequest(riskReq); err != nil {
			logrus.Warnf("风控请求失败（不影响下单）: %v", err)
		}
	}()

	return orderResp, nil
}

// SendTriggerOrderInsertWithRisk 设置止盈止损并发送风控（组合方法）
// instrumentID: 交易对，如 "BTCUSDT"
// direction: 持仓方向: "0"=买入, "1"=卖出 平多仓传1 平空仓传0
// tpTriggerPrice: 止盈触发价格
// slTriggerPrice: 止损触发价格
// volume: 交易数量
// tpPrice: 止盈委托价格
// slPrice: 止损委托价格
// isCrossMargin: 1=全仓, 0=逐仓
// loginID: 登录ID（用于风控）
func (c *AutoTradeWebClient) SendTriggerOrderInsertWithRisk(
	instrumentID, direction string,
	tpTriggerPrice, slTriggerPrice float64,
	volume int,
	tpPrice, slPrice float64,
	isCrossMargin int,
	loginID string,
) (*WebTriggerOrderResponse, error) {
	// 1. 先设置止盈止损
	triggerReq := &WebTriggerOrderRequest{
		InstrumentID:   instrumentID,
		Direction:      direction,
		TPTriggerPrice: tpTriggerPrice,
		SLTriggerPrice: slTriggerPrice,
		Volume:         volume,
		TPPrice:        tpPrice,
		SLPrice:        slPrice,
		IsCrossMargin:  isCrossMargin,
		BusinessType:   "X",
	}

	triggerResp, err := c.SendTriggerOrderInsert(triggerReq)
	if err != nil {
		return nil, fmt.Errorf("设置止盈止损失败: %w", err)
	}

	// 2. 发送止盈止损风控
	tpslRiskReq := &WebTPSLRiskRequest{
		LoginID:          loginID,
		InstrumentIDName: instrumentID,
		Success:          "true",
		TPSLVolumeType:   "全部",
		TPSLTradeMode:    "市价",
		TPActionType:     "价格",
		TPTriggerPercent: "",
		TPTriggerPrice:   fmt.Sprintf("%.1f", tpTriggerPrice),
		SLActionType:     "价格",
		SLTriggerPercent: "",
		SLTriggerPrice:   fmt.Sprintf("%.1f", slTriggerPrice),
		TPSlider:         false,
		SLSlider:         false,
		VolumeSlider:     false,
		CoinType:         "CNY",
		LanguageType:     "简体中文",
		EnvPlatform:      "web_desktop",
	}

	if err := c.SendTPSLRiskRequest(tpslRiskReq); err != nil {
		logrus.Warnf("止盈止损风控请求失败（不影响设置）: %v", err)
	}

	return triggerResp, nil
}
