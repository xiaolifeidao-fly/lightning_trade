package utils

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	// DeepCoinAPIBaseURL DeepCoin API 基础URL
	DeepCoinAPIBaseURL = "https://api.deepcoin.com"
	// DefaultRequestTimeout 默认请求超时时间
	DefaultRequestTimeout = 10 * time.Second
)

// DeepCoinClient DeepCoin API客户端
type DeepCoinClient struct {
	apiKey     string
	secretKey  string
	passphrase string
	client     *http.Client
	apiBaseURL string
}

// NewDeepCoinClient 创建DeepCoin API客户端
// apiKey: API Key
// secretKey: Secret Key
// passphrase: API Passphrase
func NewDeepCoinClient(apiKey, secretKey, passphrase string) *DeepCoinClient {
	return &DeepCoinClient{
		apiKey:     apiKey,
		secretKey:  secretKey,
		passphrase: passphrase,
		apiBaseURL: DeepCoinAPIBaseURL,
		client: &http.Client{
			Timeout: DefaultRequestTimeout,
		},
	}
}

// ============================= 通用方法 =============================

// generateISOTime 生成ISO格式时间（包含毫秒）
func (dc *DeepCoinClient) generateISOTime() string {
	now := time.Now().UTC()
	return now.Format("2006-01-02T15:04:05.000Z")
}

// generateSignature 生成签名
// method: HTTP方法 (GET/POST)
// requestPath: 请求路径 (GET请求包含query参数，如 /deepcoin/account/balances?instType=SWAP&ccy=；POST请求不包含query参数)
// body: 请求体字符串 (GET请求为空字符串，POST请求为JSON字符串)
// isoTime: ISO格式时间戳
func (dc *DeepCoinClient) generateSignature(method, requestPath, body, isoTime string) string {
	// 构建签名消息：timestamp + method + requestPath + body
	message := fmt.Sprintf("%s%s%s", isoTime, method, requestPath)
	if body != "" {
		message = fmt.Sprintf("%s%s", message, body)
	}

	// HMAC SHA256签名
	h := hmac.New(sha256.New, []byte(dc.secretKey))
	h.Write([]byte(message))
	signature := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return signature
}

// doRequest 执行HTTP请求
func (dc *DeepCoinClient) doRequest(method, path string, params map[string]interface{}) (map[string]interface{}, error) {
	isoTime := dc.generateISOTime()

	var requestPath string // 用于签名的路径
	var bodyStr string     // 用于签名的body
	var fullURL string     // 完整的请求URL
	var req *http.Request
	var err error

	if method == "GET" {
		// GET请求：构建包含query参数的完整路径用于签名
		if len(params) > 0 {
			queryPairs := []string{}
			for k, v := range params {
				if v != nil {
					queryPairs = append(queryPairs, fmt.Sprintf("%s=%v", k, v))
				}
			}
			queryString := strings.Join(queryPairs, "&")
			requestPath = path + "?" + queryString
			fullURL = dc.apiBaseURL + requestPath
		} else {
			requestPath = path
			fullURL = dc.apiBaseURL + path
		}
		// GET请求body为空
		bodyStr = ""
		req, err = http.NewRequest("GET", fullURL, nil)
	} else {
		// POST请求：路径不包含query参数，body为JSON字符串
		requestPath = path
		fullURL = dc.apiBaseURL + path
		if len(params) > 0 {
			bodyBytes, _ := json.Marshal(params)
			bodyStr = string(bodyBytes)
			req, err = http.NewRequest("POST", fullURL, bytes.NewBuffer(bodyBytes))
		} else {
			bodyStr = ""
			req, err = http.NewRequest("POST", fullURL, nil)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 生成签名：GET请求使用包含query参数的完整路径，POST请求使用不包含query参数的路径+body
	signature := dc.generateSignature(method, requestPath, bodyStr, isoTime)

	// 设置请求头
	req.Header.Set("DC-ACCESS-KEY", dc.apiKey)
	req.Header.Set("DC-ACCESS-SIGN", signature)
	req.Header.Set("DC-ACCESS-TIMESTAMP", isoTime)
	req.Header.Set("DC-ACCESS-PASSPHRASE", dc.passphrase)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "DEEPCOIN_OPEN_API")
	req.Header.Set("Accept", "*/*")

	// 发送请求
	resp, err := dc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析JSON
	var result map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("解析响应JSON失败: %w, body: %s", err, string(bodyBytes))
	}

	// 检查响应状态
	if code, ok := result["code"].(string); ok && code != "0" {
		msg := result["msg"]
		return result, fmt.Errorf("API错误: code=%s, msg=%v", code, msg)
	}

	return result, nil
}

// ============================= 1. 获取资金账户余额 =============================

// GetBalances 获取资金账户余额
// instType: 产品类型 ("SPOT"=现货, "SWAP"=合约)
// ccy: 币种，如"USDT"，不传则查询所有资产
func (dc *DeepCoinClient) GetBalances(instType string, ccy ...string) (map[string]interface{}, error) {
	path := "/deepcoin/account/balances"
	params := map[string]interface{}{
		"instType": instType,
	}

	if len(ccy) > 0 && ccy[0] != "" {
		params["ccy"] = ccy[0]
	}

	logrus.Debugf("[DeepCoin] 获取余额: instType=%s, ccy=%v", instType, ccy)
	result, err := dc.doRequest("GET", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 获取余额失败: %v", err)
		return nil, err
	}

	logrus.Infof("[DeepCoin] 获取余额成功")
	return result, nil
}

// ============================= 2. 获取持仓列表 =============================

// GetPositions 获取持仓列表
// instType: 产品类型 ("SPOT"=现货, "SWAP"=合约)
// instId: 产品ID (可选)
func (dc *DeepCoinClient) GetPositions(instType string, instId ...string) (map[string]interface{}, error) {
	path := "/deepcoin/account/positions"
	params := map[string]interface{}{
		"instType": instType,
	}

	if len(instId) > 0 && instId[0] != "" {
		params["instId"] = instId[0]
	}

	logrus.Debugf("[DeepCoin] 获取持仓: instType=%s, instId=%v", instType, instId)
	result, err := dc.doRequest("GET", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 获取持仓失败: %v", err)
		return nil, err
	}

	logrus.Infof("[DeepCoin] 获取持仓成功")
	return result, nil
}

// ============================= 3. 下单 =============================

// OrderRequest 下单请求参数
type OrderRequest struct {
	InstId      string `json:"instId"`                // 产品ID
	TdMode      string `json:"tdMode"`                // 交易模式: cash/cross/isolated
	Side        string `json:"side"`                  // 订单方向: buy/sell
	OrdType     string `json:"ordType"`               // 订单类型: market/limit/post_only/ioc
	Sz          string `json:"sz"`                    // 委托数量
	Px          string `json:"px,omitempty"`          // 委托价格 (limit/post_only必填)
	PosSide     string `json:"posSide,omitempty"`     // 持仓方向: long/short (合约必填)
	MrgPosition string `json:"mrgPosition,omitempty"` // 合并仓位: merge/split (合约必填)
	Ccy         string `json:"ccy,omitempty"`         // 保证金币种
	ClosePosId  string `json:"closePosId,omitempty"`  // 平仓仓位ID (分仓模式必填)
	ReduceOnly  bool   `json:"reduceOnly,omitempty"`  // 是否只减仓
	TpTriggerPx string `json:"tpTriggerPx,omitempty"` // 止盈触发价
	SlTriggerPx string `json:"slTriggerPx,omitempty"` // 止损触发价
}

// PlaceOrder 下单
func (dc *DeepCoinClient) PlaceOrder(req *OrderRequest) (map[string]interface{}, error) {
	path := "/deepcoin/trade/order"

	// 转换为map
	params := map[string]interface{}{
		"instId":  req.InstId,
		"tdMode":  req.TdMode,
		"side":    req.Side,
		"ordType": req.OrdType,
		"sz":      req.Sz,
	}

	if req.Px != "" {
		params["px"] = req.Px
	}
	if req.PosSide != "" {
		params["posSide"] = req.PosSide
	}
	if req.MrgPosition != "" {
		params["mrgPosition"] = req.MrgPosition
	}
	if req.Ccy != "" {
		params["ccy"] = req.Ccy
	}
	if req.ClosePosId != "" {
		params["closePosId"] = req.ClosePosId
	}
	if req.ReduceOnly {
		params["reduceOnly"] = req.ReduceOnly
	}
	if req.TpTriggerPx != "" {
		params["tpTriggerPx"] = req.TpTriggerPx
	}
	if req.SlTriggerPx != "" {
		params["slTriggerPx"] = req.SlTriggerPx
	}

	logrus.Infof("[DeepCoin] 下单: instId=%s, side=%s, ordType=%s, sz=%s, px=%s",
		req.InstId, req.Side, req.OrdType, req.Sz, req.Px)

	result, err := dc.doRequest("POST", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 下单失败: %v", err)
		return nil, err
	}

	// 检查sCode
	if data, ok := result["data"].(map[string]interface{}); ok {
		if sCode, ok := data["sCode"].(string); ok && sCode == "0" {
			logrus.Infof("[DeepCoin] 下单成功: ordId=%v", data["ordId"])
		} else {
			logrus.Warnf("[DeepCoin] 下单部分成功: sCode=%v, sMsg=%v", data["sCode"], data["sMsg"])
		}
	}

	return result, nil
}

// ============================= 4. 条件单下单 =============================

// TriggerOrderRequest 条件单请求参数
type TriggerOrderRequest struct {
	InstId          string  `json:"instId"`                    // 产品ID
	ProductGroup    string  `json:"productGroup"`              // 交易类型: Spot/Swap
	Sz              string  `json:"sz"`                        // 委托数量
	Side            string  `json:"side"`                      // 订单方向: buy/sell
	OrderType       string  `json:"orderType"`                 // 订单价格类型: limit/market
	TriggerPrice    string  `json:"triggerPrice"`              // 触发价格
	TdMode          string  `json:"tdMode"`                    // 交易模式: cash/cross/isolated
	PosSide         string  `json:"posSide,omitempty"`         // 持仓方向: long/short (合约必填)
	Price           string  `json:"price,omitempty"`           // 限价单价格
	IsCrossMargin   string  `json:"isCrossMargin"`             // 是否全仓: 0/1
	TriggerPxType   string  `json:"triggerPxType,omitempty"`   // 触发价类型: last/index/mark
	MrgPosition     string  `json:"mrgPosition,omitempty"`     // 合并仓位: merge/split
	ClosePosId      string  `json:"closePosId,omitempty"`      // 平仓仓位ID
	TpTriggerPx     float64 `json:"tpTriggerPx,omitempty"`     // 止盈触发价
	TpTriggerPxType string  `json:"tpTriggerPxType,omitempty"` // 止盈触发价类型
	TpOrdPx         float64 `json:"tpOrdPx,omitempty"`         // 止盈委托价
	SlTriggerPx     float64 `json:"slTriggerPx,omitempty"`     // 止损触发价
	SlTriggerPxType string  `json:"slTriggerPxType,omitempty"` // 止损触发价类型
	SlOrdPx         float64 `json:"slOrdPx,omitempty"`         // 止损委托价
}

// PlaceTriggerOrder 条件单下单
func (dc *DeepCoinClient) PlaceTriggerOrder(req *TriggerOrderRequest) (map[string]interface{}, error) {
	path := "/deepcoin/trade/trigger-order"

	// 转换为map
	params := map[string]interface{}{
		"instId":        req.InstId,
		"productGroup":  req.ProductGroup,
		"sz":            req.Sz,
		"side":          req.Side,
		"orderType":     req.OrderType,
		"triggerPrice":  req.TriggerPrice,
		"tdMode":        req.TdMode,
		"isCrossMargin": req.IsCrossMargin,
	}

	if req.PosSide != "" {
		params["posSide"] = req.PosSide
	}
	if req.Price != "" {
		params["price"] = req.Price
	}
	if req.TriggerPxType != "" {
		params["triggerPxType"] = req.TriggerPxType
	}
	if req.MrgPosition != "" {
		params["mrgPosition"] = req.MrgPosition
	}
	if req.ClosePosId != "" {
		params["closePosId"] = req.ClosePosId
	}
	if req.TpTriggerPx != 0 {
		params["tpTriggerPx"] = req.TpTriggerPx
	}
	if req.TpTriggerPxType != "" {
		params["tpTriggerPxType"] = req.TpTriggerPxType
	}
	if req.TpOrdPx != 0 {
		params["tpOrdPx"] = req.TpOrdPx
	}
	if req.SlTriggerPx != 0 {
		params["slTriggerPx"] = req.SlTriggerPx
	}
	if req.SlTriggerPxType != "" {
		params["slTriggerPxType"] = req.SlTriggerPxType
	}
	if req.SlOrdPx != 0 {
		params["slOrdPx"] = req.SlOrdPx
	}

	logrus.Infof("[DeepCoin] 条件单下单: instId=%s, side=%s, triggerPrice=%s",
		req.InstId, req.Side, req.TriggerPrice)

	result, err := dc.doRequest("POST", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 条件单下单失败: %v", err)
		return nil, err
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if sCode, ok := data["sCode"].(string); ok && sCode == "0" {
			logrus.Infof("[DeepCoin] 条件单下单成功: ordId=%v", data["ordId"])
		}
	}

	return result, nil
}

// ============================= 5. 按仓位ID平仓 =============================

// ClosePositionsByIds 按仓位ID平仓
// productGroup: 产品组 (Spot/Swap/SwapU)
// instId: 产品ID
// positionIds: 仓位ID列表
func (dc *DeepCoinClient) ClosePositionsByIds(productGroup, instId string, positionIds []string) (map[string]interface{}, error) {
	path := "/deepcoin/trade/close-position-by-ids"
	params := map[string]interface{}{
		"productGroup": productGroup,
		"instId":       instId,
		"positionIds":  positionIds,
	}

	logrus.Infof("[DeepCoin] 平仓: productGroup=%s, instId=%s, posIds=%v",
		productGroup, instId, positionIds)

	result, err := dc.doRequest("POST", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 平仓失败: %v", err)
		return nil, err
	}

	// 检查错误列表
	if data, ok := result["data"].(map[string]interface{}); ok {
		if errorList, ok := data["errorList"].([]interface{}); ok && len(errorList) > 0 {
			logrus.Warnf("[DeepCoin] 平仓部分失败: errorList=%v", errorList)
		} else {
			logrus.Infof("[DeepCoin] 平仓成功")
		}
	}

	return result, nil
}

// ============================= 6. 设置持仓止盈止损 =============================

// SetPositionSLTPRequest 设置止盈止损请求参数
type SetPositionSLTPRequest struct {
	InstType        string `json:"instType"`                  // 产品类型: SPOT/SWAP
	InstId          string `json:"instId"`                    // 产品ID
	PosSide         string `json:"posSide,omitempty"`         // 持仓方向: long合仓/short分仓 (合约必填)
	MrgPosition     string `json:"mrgPosition,omitempty"`     // 保证金仓位模式: merge/split
	TdMode          string `json:"tdMode,omitempty"`          // 交易模式: cross全仓/isolated逐仓
	PosId           string `json:"posId,omitempty"`           // 仓位ID (分仓模式必填)
	TpTriggerPx     string `json:"tpTriggerPx,omitempty"`     // 止盈触发价
	TpTriggerPxType string `json:"tpTriggerPxType,omitempty"` // 止盈触发价类型: last/index/mark
	TpOrdPx         string `json:"tpOrdPx,omitempty"`         // 止盈委托价
	SlTriggerPx     string `json:"slTriggerPx,omitempty"`     // 止损触发价
	SlTriggerPxType string `json:"slTriggerPxType,omitempty"` // 止损触发价类型: last/index/mark
	SlOrdPx         string `json:"slOrdPx,omitempty"`         // 止损委托价
	Sz              string `json:"sz,omitempty"`              // 持仓数量 (部分止盈止损)
}

// SetPositionSLTP 设置持仓止盈止损
func (dc *DeepCoinClient) SetPositionSLTP(req *SetPositionSLTPRequest) (map[string]interface{}, error) {
	path := "/deepcoin/trade/set-position-sltp"

	// 转换为map
	params := map[string]interface{}{
		"instType": req.InstType,
		"instId":   req.InstId,
	}

	if req.PosSide != "" {
		params["posSide"] = req.PosSide
	}
	if req.MrgPosition != "" {
		params["mrgPosition"] = req.MrgPosition
	}
	if req.TdMode != "" {
		params["tdMode"] = req.TdMode
	}
	if req.PosId != "" {
		params["posId"] = req.PosId
	}
	if req.TpTriggerPx != "" {
		params["tpTriggerPx"] = req.TpTriggerPx
	}
	if req.TpTriggerPxType != "" {
		params["tpTriggerPxType"] = req.TpTriggerPxType
	}
	if req.TpOrdPx != "" {
		params["tpOrdPx"] = req.TpOrdPx
	}
	if req.SlTriggerPx != "" {
		params["slTriggerPx"] = req.SlTriggerPx
	}
	if req.SlTriggerPxType != "" {
		params["slTriggerPxType"] = req.SlTriggerPxType
	}
	if req.SlOrdPx != "" {
		params["slOrdPx"] = req.SlOrdPx
	}
	if req.Sz != "" {
		params["sz"] = req.Sz
	}

	logrus.Infof("[DeepCoin] 设置止盈止损: instId=%s, tp=%s, sl=%s",
		req.InstId, req.TpTriggerPx, req.SlTriggerPx)

	result, err := dc.doRequest("POST", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 设置止盈止损失败: %v", err)
		return nil, err
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if sCode, ok := data["sCode"].(string); ok && sCode == "0" {
			logrus.Infof("[DeepCoin] 设置止盈止损成功: ordId=%v", data["ordId"])
		}
	}

	return result, nil
}

// ============================= 7. 取消持仓止盈止损 =============================

// CancelPositionSLTP 取消持仓止盈止损
// instType: 产品类型 (SPOT/SWAP)
// instId: 产品ID
// ordId: 止盈止损订单ID
func (dc *DeepCoinClient) CancelPositionSLTP(instType, instId, ordId string) (map[string]interface{}, error) {
	path := "/deepcoin/trade/cancel-position-sltp"
	params := map[string]interface{}{
		"instType": instType,
		"instId":   instId,
		"ordId":    ordId,
	}

	logrus.Infof("[DeepCoin] 取消止盈止损: instId=%s, ordId=%s", instId, ordId)

	result, err := dc.doRequest("POST", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 取消止盈止损失败: %v", err)
		return nil, err
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if sCode, ok := data["sCode"].(string); ok && sCode == "0" {
			logrus.Infof("[DeepCoin] 取消止盈止损成功: ordId=%v", data["ordId"])
		}
	}

	return result, nil
}

// ============================= 8. 修改持仓止盈止损 =============================

// ModifyPositionSLTPRequest 修改止盈止损请求参数
type ModifyPositionSLTPRequest struct {
	InstType        string `json:"instType"`                  // 产品类型: SPOT/SWAP
	InstId          string `json:"instId"`                    // 产品ID
	OrdId           string `json:"ordId"`                     // 止盈止损订单ID (必填)
	PosSide         string `json:"posSide,omitempty"`         // 持仓方向: long/short (合约必填)
	MrgPosition     string `json:"mrgPosition,omitempty"`     // 保证金仓位模式: merge/split
	TdMode          string `json:"tdMode,omitempty"`          // 交易模式: cross/isolated
	PosId           string `json:"posId,omitempty"`           // 仓位ID (分仓模式必填)
	TpTriggerPx     string `json:"tpTriggerPx,omitempty"`     // 止盈触发价
	TpTriggerPxType string `json:"tpTriggerPxType,omitempty"` // 止盈触发价类型: last/index/mark
	TpOrdPx         string `json:"tpOrdPx,omitempty"`         // 止盈委托价
	SlTriggerPx     string `json:"slTriggerPx,omitempty"`     // 止损触发价
	SlTriggerPxType string `json:"slTriggerPxType,omitempty"` // 止损触发价类型: last/index/mark
	SlOrdPx         string `json:"slOrdPx,omitempty"`         // 止损委托价
	Sz              string `json:"sz,omitempty"`              // 持仓数量 (部分止盈止损)
}

// ModifyPositionSLTP 修改持仓止盈止损
func (dc *DeepCoinClient) ModifyPositionSLTP(req *ModifyPositionSLTPRequest) (map[string]interface{}, error) {
	path := "/deepcoin/trade/modify-position-sltp"

	// 转换为map
	params := map[string]interface{}{
		"instType": req.InstType,
		"instId":   req.InstId,
		"ordId":    req.OrdId,
	}

	if req.PosSide != "" {
		params["posSide"] = req.PosSide
	}
	if req.MrgPosition != "" {
		params["mrgPosition"] = req.MrgPosition
	}
	if req.TdMode != "" {
		params["tdMode"] = req.TdMode
	}
	if req.PosId != "" {
		params["posId"] = req.PosId
	}
	if req.TpTriggerPx != "" {
		params["tpTriggerPx"] = req.TpTriggerPx
	}
	if req.TpTriggerPxType != "" {
		params["tpTriggerPxType"] = req.TpTriggerPxType
	}
	if req.TpOrdPx != "" {
		params["tpOrdPx"] = req.TpOrdPx
	}
	if req.SlTriggerPx != "" {
		params["slTriggerPx"] = req.SlTriggerPx
	}
	if req.SlTriggerPxType != "" {
		params["slTriggerPxType"] = req.SlTriggerPxType
	}
	if req.SlOrdPx != "" {
		params["slOrdPx"] = req.SlOrdPx
	}
	if req.Sz != "" {
		params["sz"] = req.Sz
	}

	logrus.Infof("[DeepCoin] 修改止盈止损: instId=%s, ordId=%s, tp=%s, sl=%s",
		req.InstId, req.OrdId, req.TpTriggerPx, req.SlTriggerPx)

	result, err := dc.doRequest("POST", path, params)
	if err != nil {
		logrus.Errorf("[DeepCoin] 修改止盈止损失败: %v", err)
		return nil, err
	}

	if data, ok := result["data"].(map[string]interface{}); ok {
		if sCode, ok := data["sCode"].(string); ok && sCode == "0" {
			logrus.Infof("[DeepCoin] 修改止盈止损成功: ordId=%v", data["ordId"])
		}
	}

	return result, nil
}

// ============================= 类型化包装方法（供 argus_single 直接调用） =============================

// doRequestTyped 执行请求并将结果反序列化为目标类型
func (dc *DeepCoinClient) doRequestTyped(method, path string, params map[string]interface{}, dst interface{}) error {
	raw, err := dc.doRequest(method, path, params)
	if err != nil {
		return err
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("序列化响应失败: %w", err)
	}
	return json.Unmarshal(b, dst)
}

// GetBalancesTyped 获取余额（返回类型化响应）
func (dc *DeepCoinClient) GetBalancesTyped(req *GetBalancesRequest) (*GetBalancesResponse, error) {
	params := map[string]interface{}{
		"instType": req.InstType,
	}
	if req.Ccy != "" {
		params["ccy"] = req.Ccy
	}
	var resp GetBalancesResponse
	if err := dc.doRequestTyped("GET", "/deepcoin/account/balances", params, &resp); err != nil {
		return nil, err
	}
	if resp.Code != "0" {
		logrus.Errorf("[DeepCoin] 获取余额失败: code=%s, msg=%s", resp.Code, resp.Msg)
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}
	return &resp, nil
}

// GetPositionsTyped 获取持仓（返回类型化响应）
func (dc *DeepCoinClient) GetPositionsTyped(req *GetPositionsRequest) (*GetPositionsResponse, error) {
	params := map[string]interface{}{
		"instType": req.InstType,
	}
	if req.InstId != "" {
		params["instId"] = req.InstId
	}
	var resp GetPositionsResponse
	if err := dc.doRequestTyped("GET", "/deepcoin/account/positions", params, &resp); err != nil {
		return nil, err
	}
	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}
	return &resp, nil
}

// PlaceOrderTyped 下单（返回类型化响应）
func (dc *DeepCoinClient) PlaceOrderTyped(req *OrderRequest) (*PlaceOrderResponse, error) {
	params := map[string]interface{}{
		"instId":      req.InstId,
		"tdMode":      req.TdMode,
		"side":        req.Side,
		"ordType":     req.OrdType,
		"sz":          req.Sz,
		"posSide":     req.PosSide,
		"mrgPosition": req.MrgPosition,
	}
	if req.Px != "" {
		params["px"] = req.Px
	}
	var resp PlaceOrderResponse
	if err := dc.doRequestTyped("POST", "/deepcoin/trade/order", params, &resp); err != nil {
		return nil, err
	}
	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}
	if !resp.Data.IsSuccess() {
		return &resp, fmt.Errorf("下单失败: %s", resp.Data.GetError())
	}
	return &resp, nil
}

// SetPositionSLTPTyped 设置止盈止损（返回类型化响应）
func (dc *DeepCoinClient) SetPositionSLTPTyped(req *SetPositionSLTPRequest) (*SetPositionSLTPResponse, error) {
	params := map[string]interface{}{
		"instType":    req.InstType,
		"instId":      req.InstId,
		"posSide":     req.PosSide,
		"mrgPosition": req.MrgPosition,
		"tdMode":      req.TdMode,
		"tpTriggerPx": req.TpTriggerPx,
		"tpOrdPx":     req.TpOrdPx,
		"slTriggerPx": req.SlTriggerPx,
		"slOrdPx":     req.SlOrdPx,
		"sz":          req.Sz,
	}
	var resp SetPositionSLTPResponse
	if err := dc.doRequestTyped("POST", "/deepcoin/trade/set-position-sltp", params, &resp); err != nil {
		return nil, err
	}
	if resp.Code != "0" {
		return nil, fmt.Errorf("API错误: code=%s, msg=%s", resp.Code, resp.Msg)
	}
	return &resp, nil
}

// MarketBuyLong 市价做多开仓
func (dc *DeepCoinClient) MarketBuyLong(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	return dc.PlaceOrderTyped(&OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "market",
		Sz:          req.Size,
		PosSide:     "long",
		MrgPosition: "merge",
	})
}

// MarketSellShort 市价做空开仓
func (dc *DeepCoinClient) MarketSellShort(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	return dc.PlaceOrderTyped(&OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "sell",
		OrdType:     "market",
		Sz:          req.Size,
		PosSide:     "short",
		MrgPosition: "merge",
	})
}

// IOCBuyLong IOC限价做多开仓
func (dc *DeepCoinClient) IOCBuyLong(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	return dc.PlaceOrderTyped(&OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "buy",
		OrdType:     "ioc",
		Sz:          req.Size,
		Px:          req.Price,
		PosSide:     "long",
		MrgPosition: "merge",
	})
}

// IOCSellShort IOC限价做空开仓
func (dc *DeepCoinClient) IOCSellShort(req *QuickOrderRequest) (*PlaceOrderResponse, error) {
	return dc.PlaceOrderTyped(&OrderRequest{
		InstId:      req.InstId,
		TdMode:      "cross",
		Side:        "sell",
		OrdType:     "ioc",
		Sz:          req.Size,
		Px:          req.Price,
		PosSide:     "short",
		MrgPosition: "merge",
	})
}
