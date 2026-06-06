package utils

// ============================= 通用响应 =============================

type AutoTradeResponse struct {
	Code string                 `json:"code"`
	Msg  string                 `json:"msg"`
	Data map[string]interface{} `json:"data"`
}

type CommonOrderResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data OrderResultData `json:"data"`
}

type OrderResultData struct {
	OrdId   string `json:"ordId"`
	ClOrdId string `json:"clOrdId"`
	Tag     string `json:"tag"`
	SCode   string `json:"sCode"`
	SMsg    string `json:"sMsg"`
}

// ============================= 账户查询 =============================

type GetBalancesRequest struct {
	InstType string `json:"instType"` // SPOT/SWAP
	Ccy      string `json:"ccy,omitempty"`
}

type GetBalancesResponse struct {
	Code string        `json:"code"`
	Msg  string        `json:"msg"`
	Data []BalanceInfo `json:"data"`
}

type BalanceInfo struct {
	Ccy       string `json:"ccy"`
	Bal       string `json:"bal"`       //余额
	FrozenBal string `json:"frozenBal"` //冻结(不可用)
	AvailBal  string `json:"availBal"`  //可用余额
}

type GetPositionsRequest struct {
	InstType string `json:"instType"` // SPOT/SWAP
	InstId   string `json:"instId,omitempty"`
}

type GetPositionsResponse struct {
	Code string         `json:"code"`
	Msg  string         `json:"msg"`
	Data []PositionInfo `json:"data"`
}

type PositionInfo struct {
	InstType         string `json:"instType"`         //产品类型 现货展示为：SPOT
	MgnMode          string `json:"mgnMode"`          // 保障模式: cross=全仓 现货展示为: cash
	InstId           string `json:"instId"`           //产品 ID
	PosId            string `json:"posId"`            // 订单 ID
	PosSide          string `json:"posSide"`          // 持仓方向 多: long   空: short 现货展示空
	Pos              string `json:"pos"`              // 持仓数量 单位: 张
	AvgPx            string `json:"avgPx"`            // 开仓均价 现货展示为：买入均价
	Lever            string `json:"lever"`            // 杠杆大小 现货展示空
	LiqPx            string `json:"liqPx"`            // 强平价 现货展示空
	UseMargin        string `json:"useMargin"`        // 占用保证金 现货展示空
	UnrealizedProfit string `json:"unrealizedProfit"` // 未实现盈亏
	MrgPosition      string `json:"mrgPosition"`      // 合约仓位类型 合仓: merge 分仓: split
	Ccy              string `json:"ccy"`              // 占用保证金币种
	LastPx           string `json:"lastPx"`           // 最新成交价
	UTime            string `json:"uTime"`            // 持仓创建时间，Unix 时间戳格式的毫秒数格式
	CTime            string `json:"cTime"`            // 最近一次持仓更新时间，Unix 时间戳格式的毫秒数格式
}

// ============================= 交易下单 =============================

type PlaceOrderResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data OrderResultData `json:"data"`
}

type PlaceTriggerOrderResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data OrderResultData `json:"data"`
}

// ============================= 平仓管理 =============================

type ClosePositionsRequest struct {
	ProductGroup string   `json:"productGroup"` // Spot/Swap/SwapU
	InstId       string   `json:"instId"`
	PositionIds  []string `json:"positionIds"`
}

type ClosePositionsResponse struct {
	Code string                  `json:"code"`
	Msg  string                  `json:"msg"`
	Data ClosePositionsErrorData `json:"data"`
}

type ClosePositionsErrorData struct {
	ErrorList []ClosePositionError `json:"errorList"`
}

type ClosePositionError struct {
	MemberId      string `json:"memberId"`
	AccountId     string `json:"accountId"`
	TradeUnitId   string `json:"tradeUnitId"`
	InstId        string `json:"instId"`
	PosiDirection string `json:"posiDirection"`
	ErrorCode     int    `json:"errorCode"`
	ErrorMsg      string `json:"errorMsg"`
}

// ============================= 止盈止损管理 =============================

type SetPositionSLTPResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data OrderResultData `json:"data"`
}

type ModifyPositionSLTPResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data OrderResultData `json:"data"`
}

type CancelPositionSLTPRequest struct {
	InstType string `json:"instType"`
	InstId   string `json:"instId"`
	OrdId    string `json:"ordId"`
}

type CancelPositionSLTPResponse struct {
	Code string          `json:"code"`
	Msg  string          `json:"msg"`
	Data OrderResultData `json:"data"`
}

// ============================= 快捷方法 =============================

type QuickOrderRequest struct {
	InstId string `json:"instId"`
	Size   string `json:"size"`
	Price  string `json:"price,omitempty"` // 市价单不需要
}

type ArbitrageTradeRequest struct {
	InstId  string `json:"instId"`
	Size    string `json:"size"`
	Price   string `json:"price"`
	BuyDeep bool   `json:"buyDeep"` // true=买入DeepCoin, false=卖出DeepCoin
}

type ArbitrageTradeResponse struct {
	Success  bool   `json:"success"`
	OrderId  string `json:"orderId"`
	Error    string `json:"error,omitempty"`
	CloseMsg string `json:"closeMsg,omitempty"`
}

// ============================= 辅助方法 =============================

func (r *OrderResultData) IsSuccess() bool {
	return r.SCode == "0"
}

func (r *OrderResultData) GetError() string {
	if r.IsSuccess() {
		return ""
	}
	return r.SMsg
}

func (r *GetBalancesResponse) GetBalance(ccy string) (*BalanceInfo, bool) {
	for _, bal := range r.Data {
		if bal.Ccy == ccy {
			return &bal, true
		}
	}
	return nil, false
}

func (r *GetPositionsResponse) GetPositionIds() []string {
	ids := make([]string, 0, len(r.Data))
	for _, pos := range r.Data {
		ids = append(ids, pos.PosId)
	}
	return ids
}

func (r *GetPositionsResponse) FilterByInstId(instId string) []PositionInfo {
	positions := make([]PositionInfo, 0)
	for _, pos := range r.Data {
		if pos.InstId == instId {
			positions = append(positions, pos)
		}
	}
	return positions
}

func (r *ClosePositionsResponse) HasErrors() bool {
	return len(r.Data.ErrorList) > 0
}

func (r *ClosePositionsResponse) IsSuccess() bool {
	return r.Code == "0" && !r.HasErrors()
}
