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
	DefaultAutoTradeURL     = "http://localhost:8899"
	DefaultAutoTradeTimeout = 10 * time.Second
)

type AutoTradeClient struct {
	baseURL    string
	apiKey     string
	secretKey  string
	passphrase string
	client     *http.Client
}

func NewAutoTradeClient(baseURL, apiKey, secretKey, passphrase string) *AutoTradeClient {
	if baseURL == "" {
		baseURL = DefaultAutoTradeURL
	}

	return &AutoTradeClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		secretKey:  secretKey,
		passphrase: passphrase,
		client: &http.Client{
			Timeout: DefaultAutoTradeTimeout,
		},
	}
}

func (c *AutoTradeClient) doRequest(method, path string, reqBody interface{}, respBody interface{}) error {
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

	req.Header.Set("X-DeepCoin-API-Key", c.apiKey)
	req.Header.Set("X-DeepCoin-Secret-Key", c.secretKey)
	req.Header.Set("X-DeepCoin-Passphrase", c.passphrase)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 先检查响应中的code字段，如果code不是"0"，返回错误
	var errorCheck struct {
		Code interface{} `json:"code"` // 使用interface{}支持字符串和数字类型
		Msg  string      `json:"msg"`
	}
	if err := json.Unmarshal(bodyBytes, &errorCheck); err == nil {
		// 检查code字段
		if errorCheck.Code != nil {
			var codeStr string
			// 处理code可能是字符串或数字的情况
			switch v := errorCheck.Code.(type) {
			case string:
				codeStr = v
			case float64:
				codeStr = fmt.Sprintf("%.0f", v)
			case int:
				codeStr = fmt.Sprintf("%d", v)
			default:
				codeStr = fmt.Sprintf("%v", v)
			}
			// 如果code不是"0"，返回错误
			if codeStr != "0" {
				if errorCheck.Msg != "" {
					return fmt.Errorf("%s", errorCheck.Msg)
				}
				return fmt.Errorf("API错误: code=%s", codeStr)
			}
		}
	}

	// code为"0"或不存在code字段，正常解析到respBody
	if err := json.Unmarshal(bodyBytes, respBody); err != nil {
		return fmt.Errorf("解析响应JSON失败: %w, body: %s", err, string(bodyBytes))
	}

	return nil
}

// ============================= 账户查询 =============================

func (c *AutoTradeClient) GetBalances(req *GetBalancesRequest) (*GetBalancesResponse, error) {
	logrus.Debugf("[AutoTrade] 获取余额: instType=%s, ccy=%s", req.InstType, req.Ccy)

	params := map[string]string{
		"instType": req.InstType,
	}
	if req.Ccy != "" {
		params["ccy"] = req.Ccy
	}

	var resp GetBalancesResponse
	err := c.doRequest("GET", "/trade/balances", params, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 获取余额失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	logrus.Infof("[AutoTrade] 获取余额成功: %d个币种", len(resp.Data))
	return &resp, nil
}

func (c *AutoTradeClient) GetPositions(req *GetPositionsRequest) (*GetPositionsResponse, error) {
	logrus.Debugf("[AutoTrade] 获取持仓: instType=%s, instId=%s", req.InstType, req.InstId)

	params := map[string]string{
		"instType": req.InstType,
	}
	if req.InstId != "" {
		params["instId"] = req.InstId
	}

	var resp GetPositionsResponse
	err := c.doRequest("GET", "/trade/positions", params, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 获取持仓失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	logrus.Infof("[AutoTrade] 获取持仓成功: %d个持仓", len(resp.Data))
	return &resp, nil
}

// ============================= 交易下单 =============================

func (c *AutoTradeClient) PlaceOrder(req *OrderRequest) (*PlaceOrderResponse, error) {
	logrus.Infof("[AutoTrade] 下单: instId=%s, side=%s, ordType=%s, sz=%s, px=%s",
		req.InstId, req.Side, req.OrdType, req.Sz, req.Px)

	var resp PlaceOrderResponse
	err := c.doRequest("POST", "/trade/order", req, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 下单失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	if !resp.Data.IsSuccess() {
		logrus.Warnf("[AutoTrade] 下单失败: %s", resp.Data.GetError())
		return &resp, fmt.Errorf("下单失败: %s", resp.Data.GetError())
	}

	logrus.Infof("[AutoTrade] 下单成功: ordId=%s", resp.Data.OrdId)
	return &resp, nil
}

func (c *AutoTradeClient) PlaceTriggerOrder(req *TriggerOrderRequest) (*PlaceTriggerOrderResponse, error) {
	logrus.Infof("[AutoTrade] 条件单下单: instId=%s, triggerPrice=%s", req.InstId, req.TriggerPrice)

	var resp PlaceTriggerOrderResponse
	err := c.doRequest("POST", "/trade/trigger-order", req, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 条件单下单失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	if !resp.Data.IsSuccess() {
		return &resp, fmt.Errorf("条件单下单失败: %s", resp.Data.GetError())
	}

	logrus.Infof("[AutoTrade] 条件单下单成功: ordId=%s", resp.Data.OrdId)
	return &resp, nil
}

// ============================= 平仓管理 =============================

func (c *AutoTradeClient) ClosePositions(req *ClosePositionsRequest) (*ClosePositionsResponse, error) {
	logrus.Infof("[AutoTrade] 平仓: productGroup=%s, instId=%s, posIds=%v",
		req.ProductGroup, req.InstId, req.PositionIds)

	var resp ClosePositionsResponse
	err := c.doRequest("POST", "/trade/close-positions", req, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 平仓失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	if resp.HasErrors() {
		logrus.Warnf("[AutoTrade] 平仓部分失败: %d个错误", len(resp.Data.ErrorList))
	} else {
		logrus.Infof("[AutoTrade] 平仓成功")
	}

	return &resp, nil
}

// ============================= 止盈止损管理 =============================

func (c *AutoTradeClient) SetPositionSLTP(req *SetPositionSLTPRequest) (*SetPositionSLTPResponse, error) {
	logrus.Infof("[AutoTrade] 设置止盈止损: instId=%s, tp=%s, sl=%s",
		req.InstId, req.TpTriggerPx, req.SlTriggerPx)

	var resp SetPositionSLTPResponse
	err := c.doRequest("POST", "/trade/set-sltp", req, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 设置止盈止损失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	if !resp.Data.IsSuccess() {
		return &resp, fmt.Errorf("设置止盈止损失败: %s", resp.Data.GetError())
	}

	logrus.Infof("[AutoTrade] 设置止盈止损成功: ordId=%s", resp.Data.OrdId)
	return &resp, nil
}

func (c *AutoTradeClient) ModifyPositionSLTP(req *ModifyPositionSLTPRequest) (*ModifyPositionSLTPResponse, error) {
	logrus.Infof("[AutoTrade] 修改止盈止损: instId=%s, ordId=%s", req.InstId, req.OrdId)

	var resp ModifyPositionSLTPResponse
	err := c.doRequest("POST", "/trade/modify-sltp", req, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 修改止盈止损失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	if !resp.Data.IsSuccess() {
		return &resp, fmt.Errorf("修改止盈止损失败: %s", resp.Data.GetError())
	}

	logrus.Infof("[AutoTrade] 修改止盈止损成功: ordId=%s", resp.Data.OrdId)
	return &resp, nil
}

func (c *AutoTradeClient) CancelPositionSLTP(req *CancelPositionSLTPRequest) (*CancelPositionSLTPResponse, error) {
	logrus.Infof("[AutoTrade] 取消止盈止损: instId=%s, ordId=%s", req.InstId, req.OrdId)

	var resp CancelPositionSLTPResponse
	err := c.doRequest("POST", "/trade/cancel-sltp", req, &resp)
	if err != nil {
		logrus.Errorf("[AutoTrade] 取消止盈止损失败: %v", err)
		return nil, err
	}

	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}

	if !resp.Data.IsSuccess() {
		return &resp, fmt.Errorf("取消止盈止损失败: %s", resp.Data.GetError())
	}

	logrus.Infof("[AutoTrade] 取消止盈止损成功: ordId=%s", resp.Data.OrdId)
	return &resp, nil
}

// ============================= 快捷方法 =============================

func (c *AutoTradeClient) MarketBuyLong(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	orderReq := &OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "market",
		Sz:          req.Size,
		PosSide:     "long",
		MrgPosition: "merge",
	}
	return c.PlaceOrder(orderReq)
}

func (c *AutoTradeClient) MarketSellShort(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	orderReq := &OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "sell",
		OrdType:     "market",
		Sz:          req.Size,
		PosSide:     "short",
		MrgPosition: "merge",
	}
	return c.PlaceOrder(orderReq)
}

func (c *AutoTradeClient) IOCBuyLong(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	orderReq := &OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "ioc",
		Sz:          req.Size,
		Px:          req.Price,
		PosSide:     "long",
		MrgPosition: "merge",
	}
	return c.PlaceOrder(orderReq)
}

func (c *AutoTradeClient) IOCSellShort(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	orderReq := &OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "sell",
		OrdType:     "ioc",
		Sz:          req.Size,
		Px:          req.Price,
		PosSide:     "short",
		MrgPosition: "merge",
	}
	return c.PlaceOrder(orderReq)
}

func (c *AutoTradeClient) GetPositionIdsByInstId(instId string) ([]string, error) {
	req := &GetPositionsRequest{
		InstType: "SWAP",
		InstId:   instId,
	}

	resp, err := c.GetPositions(req)
	if err != nil {
		return nil, err
	}

	return resp.GetPositionIds(), nil
}

func (c *AutoTradeClient) CloseAllPositions(instId string) (*ClosePositionsResponse, error) {
	posIds, err := c.GetPositionIdsByInstId(instId)
	if err != nil {
		return nil, err
	}

	if len(posIds) == 0 {
		logrus.Infof("没有需要平仓的持仓: %s", instId)
		return &ClosePositionsResponse{
			Code: "0",
			Msg:  "没有持仓",
			Data: ClosePositionsErrorData{
				ErrorList: []ClosePositionError{},
			},
		}, nil
	}

	closeReq := &ClosePositionsRequest{
		ProductGroup: "SwapU",
		InstId:       instId,
		PositionIds:  posIds,
	}

	return c.ClosePositions(closeReq)
}

// ============================= 套利交易 =============================

func (c *AutoTradeClient) ArbitrageTrade(req *ArbitrageTradeRequest) (*ArbitrageTradeResponse, error) {
	var orderResp *PlaceOrderResponse
	var err error

	quickReq := &QuickOrderRequest{
		InstId: req.InstId,
		Size:   req.Size,
		Price:  req.Price,
	}

	if req.BuyDeep {
		logrus.Infof("套利交易: 买入 DeepCoin %s, 价格=%s, 数量=%s", req.InstId, req.Price, req.Size)
		orderResp, err = c.IOCBuyLong(quickReq)
	} else {
		logrus.Infof("套利交易: 卖出 DeepCoin %s, 价格=%s, 数量=%s", req.InstId, req.Price, req.Size)
		orderResp, err = c.IOCSellShort(quickReq)
	}

	if err != nil {
		return &ArbitrageTradeResponse{
			Success: false,
			Error:   err.Error(),
		}, err
	}

	if !orderResp.Data.IsSuccess() {
		return &ArbitrageTradeResponse{
			Success: false,
			OrderId: orderResp.Data.OrdId,
			Error:   orderResp.Data.GetError(),
		}, fmt.Errorf("下单失败: %s", orderResp.Data.GetError())
	}

	logrus.Infof("套利下单成功, ordId=%s", orderResp.Data.OrdId)

	time.Sleep(500 * time.Millisecond)

	closeResp, err := c.CloseAllPositions(req.InstId)
	closeMsg := "平仓成功"
	if err != nil {
		closeMsg = fmt.Sprintf("平仓失败: %v", err)
		logrus.Errorf(closeMsg)
	} else if !closeResp.IsSuccess() {
		closeMsg = fmt.Sprintf("平仓部分失败: %d个错误", len(closeResp.Data.ErrorList))
		logrus.Warnf(closeMsg)
	} else {
		logrus.Infof("平仓成功")
	}

	return &ArbitrageTradeResponse{
		Success:  true,
		OrderId:  orderResp.Data.OrdId,
		CloseMsg: closeMsg,
	}, nil
}
