package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"time"

	"common/cookies"
	"common/utils"
	"common/utils/pc_trade/user"
)

// TradeRiskRequest 下单风控请求参数
type TradeRiskRequest struct {
	// 用户信息
	LoginID string // 登录ID

	// 交易信息
	InstrumentIDName      string   // 交易对名称，如 "BTCUSDT"
	InstrumentIDPerpetual string   // 永续合约类型，如 "USDT永续"
	OrderMode             string   // 订单模式，如 "开平仓模式"/"净仓模式"
	TradeType             string   // 交易类型，如 "买入"/"卖出"
	HoldType              string   // 持仓类型，如 "开仓"/"平仓"
	TradeMode             string   // 交易模式，如 "限价"/"市价"
	LeverageD             int      // 杠杆倍数（多）
	LeverageK             int      // 杠杆倍数（空）
	TradePrice            float64  // 交易价格（市价单时仅用于 $title 显示）
	TradeVolume           int      // 交易数量
	TPSLPrice             []string // 止盈止损价格，如 ["-", "-"]
	TradeSource           string   // 交易来源，如 "限价"/"市价"
	MarginModel           string   // 保证金模式，如 "全仓合仓"/"逐仓"
	IsReduceOnly          bool     // 是否只减仓

	// 可选参数
	CoinType     string // 币种类型，默认 "CNY"
	LanguageType string // 语言类型，默认 "简体中文"
	EnvPlatform  string // 环境平台，默认 "web_desktop"

	// ---- 新版风控字段（缺省自动生成）----

	// 产品版本，默认 "国际版"（www.deepcoin.com）；国内站请显式传 "国内版"
	ProductionVersion string
	// 价格趋势，"up"=▲ "down"=▼，决定 $title 箭头方向，默认 "down"
	PriceTrend string

	// 行为采集摘要：nil 时自动按 LoginID 当前窗口生成（推荐）
	Behavior *RiskBehaviorSnapshot
	// 浏览器自动化指纹：nil 时按 LoginID 派生稳定指纹（推荐）
	Browser *BrowserSignals
	// 屏幕/视口指纹：nil 时按 LoginID 派生稳定指纹（推荐）
	Viewport *ViewportSignals
}

// TPSLRiskRequest 止盈止损风控请求参数
type TPSLRiskRequest struct {
	// 用户信息
	Username string // 配置文件中的用户名（用于获取cookie）
	LoginID  string // 登录ID

	// 止盈止损信息
	InstrumentIDName string // 交易对名称，如 "BTCUSDT"
	Success          string // 是否成功，"true"/"false"
	TPSLVolumeType   string // 数量类型，如 "全部"
	TPSLTradeMode    string // 交易模式，如 "市价"/"限价"
	TPActionType     string // 止盈触发类型，如 "收益率"/"价格"
	TPTriggerPercent string // 止盈触发百分比
	TPTriggerPrice   string // 止盈触发价格
	SLActionType     string // 止损触发类型，如 "收益率"/"价格"
	SLTriggerPercent string // 止损触发百分比
	SLTriggerPrice   string // 止损触发价格
	TPSlider         bool   // 止盈是否使用滑块
	SLSlider         bool   // 止损是否使用滑块
	VolumeSlider     bool   // 数量是否使用滑块

	// 可选参数
	CoinType     string // 币种类型，默认 "CNY"
	LanguageType string // 语言类型，默认 "简体中文"
	EnvPlatform  string // 环境平台，默认 "web_desktop"
}

// tradeRiskData 内部请求数据结构（对应base64编码前的JSON）
type tradeRiskData struct {
	Identities  map[string]string `json:"identities"`
	DistinctID  string            `json:"distinct_id"`
	Lib         map[string]string `json:"lib"`
	Properties  map[string]any    `json:"properties"`
	LoginID     string            `json:"login_id"`
	AnonymousID string            `json:"anonymous_id"`
	Type        string            `json:"type"`
	Event       string            `json:"event"`
	Time        int64             `json:"time"`
	TrackID     int64             `json:"_track_id"`
	FlushTime   int64             `json:"_flush_time"`
}

// SendTradeRiskRequest 发送下单风控请求
// user: 用户信息（包含cookie等）
// req: 请求参数
func SendTradeRiskRequest(u *user.User, req *TradeRiskRequest) error {
	// 1. 从user中获取cookie
	rawCookie := u.Cookie

	// 2. 解析cookie
	cookieData, err := cookies.ParseCookieString(rawCookie)
	if err != nil {
		return fmt.Errorf("解析cookie失败: %w", err)
	}

	// 3. 构造请求数据
	data := buildTradeRiskData(cookieData, req)

	// 4. 将数据转为JSON并base64编码
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}
	encodedData := base64.StdEncoding.EncodeToString(jsonData)

	// 5. 计算ext参数
	ext := utils.GetExt(encodedData)

	// 6. 构造完整URL
	baseURL := "https://ubt.deepcoin.pro/save.gif"
	params := url.Values{}
	params.Add("project", "production")
	params.Add("data", encodedData)
	params.Add("ext", fmt.Sprintf("crc=%d", ext))
	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 7. 发送HTTP请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	httpReq, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 8. 设置请求头
	httpReq.Header.Set("accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	httpReq.Header.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8")
	httpReq.Header.Set("loginuser", req.LoginID)
	httpReq.Header.Set("referer", "https://www.deepcoin.com/")
	httpReq.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	httpReq.Header.Set("sec-ch-ua-mobile", "?0")
	httpReq.Header.Set("sec-ch-ua-platform", `"macOS"`)
	httpReq.Header.Set("sec-fetch-dest", "image")
	httpReq.Header.Set("sec-fetch-mode", "no-cors")
	httpReq.Header.Set("sec-fetch-site", "cross-site")
	httpReq.Header.Set("sec-fetch-storage-access", "active")
	httpReq.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	httpReq.Header.Set("x-forwarded-for", "4.2.2.2")

	// 9. 发送请求
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 10. 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 11. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// buildTradeRiskData 构造风控数据
func buildTradeRiskData(cookieData *cookies.CookieData, req *TradeRiskRequest) *tradeRiskData {
	// 设置默认值
	coinType := req.CoinType
	if coinType == "" {
		coinType = "CNY"
	}
	languageType := req.LanguageType
	if languageType == "" {
		languageType = "简体中文"
	}
	envPlatform := req.EnvPlatform
	if envPlatform == "" {
		envPlatform = "web_desktop"
	}
	productionVersion := req.ProductionVersion
	if productionVersion == "" {
		productionVersion = "国际版"
	}

	// 获取cookie中的身份信息
	identities := map[string]string{}
	if cookieData.SensorsData.Identities != nil {
		identities["$identity_cookie_id"] = cookieData.SensorsData.Identities.IdentityCookieID
		identities["$identity_login_id"] = cookieData.SensorsData.Identities.IdentityLoginID
		identities["identity_h5_id"] = cookieData.SensorsData.Identities.IdentityH5ID
	}

	// 当前时间戳（毫秒）
	now := time.Now().UnixNano() / int64(time.Millisecond)

	// 取设备/视口/浏览器/行为四组指纹（缺省按 LoginID 自动生成）
	loginID := req.LoginID
	if loginID == "" {
		loginID = cookieData.GetLoginID()
	}
	viewport := req.Viewport
	if viewport == nil {
		v := GenerateViewportSignals(loginID)
		viewport = &v
	}
	browser := req.Browser
	if browser == nil {
		b := GenerateBrowserSignals(loginID)
		browser = &b
	}
	behavior := req.Behavior
	if behavior == nil {
		bs := GenerateBehaviorSnapshot(loginID, 0)
		behavior = &bs
	}

	// 市价单的 de_trade_price 抓包里是 "市价"（字符串），不是价格数字
	var deTradePrice any
	if req.TradeMode == "市价" {
		deTradePrice = "市价"
	} else {
		deTradePrice = req.TradePrice
	}

	// $title 箭头方向 + 千分位
	arrow := "▼"
	if req.PriceTrend == "up" {
		arrow = "▲"
	}
	title := fmt.Sprintf("%s %s | %s - Deepcoin", arrow, formatThousands1(req.TradePrice), req.InstrumentIDName)

	// 构造数据
	data := &tradeRiskData{
		Identities:  identities,
		DistinctID:  cookieData.GetDistinctID(),
		LoginID:     req.LoginID,
		AnonymousID: cookieData.GetDeviceID(),
		Type:        "track",
		Event:       "DESubTradeInfo",
		Time:        now,
		TrackID:     now % 1000000000, // 简单生成trackID
		FlushTime:   now,
		Lib: map[string]string{
			"$lib":         "js",
			"$lib_method":  "code",
			"$lib_version": "1.26.4",
		},
		Properties: map[string]any{
			"$timezone_offset":            viewport.TimezoneOffset,
			"$screen_height":              viewport.ScreenHeight,
			"$screen_width":               viewport.ScreenWidth,
			"$viewport_height":            viewport.ViewportHeight,
			"$viewport_width":             viewport.ViewportWidth,
			"$lib":                        "js",
			"$lib_version":                "1.26.4",
			"$latest_traffic_source_type": cookieData.SensorsData.Props.LatestTrafficSourceType,
			"$latest_search_keyword":      cookieData.SensorsData.Props.LatestSearchKeyword,
			"$latest_referrer":            cookieData.SensorsData.Props.LatestReferrer,
			"platform_type":               "WEB-ONE",
			"production_version":          productionVersion,
			"language_type":               languageType,
			"coin_type":                   coinType,
			"is_login":                    true,
			"env_platform":                envPlatform,
			"de_instrument_id_name":       req.InstrumentIDName,
			"de_instrument_id_perpetual":  req.InstrumentIDPerpetual,
			"de_order_mode":               req.OrderMode,
			"de_trade_type":               req.TradeType,
			"de_hold_type":                req.HoldType,
			"de_trade_mode":               req.TradeMode,
			"de_leverage_d":               req.LeverageD,
			"de_leverage_k":               req.LeverageK,
			"de_trade_price":              deTradePrice,
			"de_trade_volume":             req.TradeVolume,
			"de_tpsl_price":               req.TPSLPrice,
			"de_trade_source":             req.TradeSource,
			"margin_model":                req.MarginModel,
			"is_reduce_only":              req.IsReduceOnly,

			// ---- 行为采集摘要 ----
			"click_count":                       behavior.ClickCount,
			"click_interval_std_ms":             behavior.ClickIntervalStdMs,
			"first_click_latency_ms":            behavior.FirstClickLatencyMs,
			"keystroke_interval_std_ms":         behavior.KeystrokeIntervalStdMs,
			"mouse_path_speed_std":              behavior.MousePathSpeedStd,
			"mouse_path_point_count":            behavior.MousePathPointCount,
			"mouse_path_direction_change_count": behavior.MousePathDirectionChangeCount,
			"mouse_path_straightness_score":     behavior.MousePathStraightnessScore,
			"risk_behavior_window_ms":           behavior.RiskBehaviorWindowMs,
			"risk_behavior_sample_interval_ms":  behavior.RiskBehaviorSampleIntervalMs,

			// ---- 浏览器自动化指纹 ----
			"browser_webdriver":                  browser.Webdriver,
			"browser_plugins_length":             browser.PluginsLength,
			"browser_languages_length":           browser.LanguagesLength,
			"browser_ua_headless":                browser.UAHeadless,
			"browser_permissions_query_abnormal": browser.PermissionsQueryAbnormal,
			"browser_iframe_access_abnormal":     browser.IframeAccessAbnormal,

			"$is_first_day": false,
			"$url":          fmt.Sprintf("https://www.deepcoin.com/turbo/zh/swap/%s", req.InstrumentIDName),
			"$title":        title,
		},
	}

	return data
}

// formatThousands1 把价格格式化成 "79,251.9" 这种带千分位的字符串
func formatThousands1(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	// 拆分整数和小数
	dot := -1
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 {
		dot = len(s)
	}
	intPart := s[:dot]
	frac := s[dot:]

	neg := false
	if len(intPart) > 0 && intPart[0] == '-' {
		neg = true
		intPart = intPart[1:]
	}

	// 给整数部分加千分位
	n := len(intPart)
	if n <= 3 {
		if neg {
			return "-" + intPart + frac
		}
		return intPart + frac
	}

	out := make([]byte, 0, n+n/3+1)
	first := n % 3
	if first > 0 {
		out = append(out, intPart[:first]...)
		if first < n {
			out = append(out, ',')
		}
	}
	for i := first; i < n; i += 3 {
		out = append(out, intPart[i:i+3]...)
		if i+3 < n {
			out = append(out, ',')
		}
	}
	if neg {
		return "-" + string(out) + frac
	}
	return string(out) + frac
}

// SendTPSLRiskRequest 发送止盈止损风控请求
// u: 用户信息（包含cookie等）
// req: 请求参数
func SendTPSLRiskRequest(u *user.User, req *TPSLRiskRequest) error {
	// 1. 从user中获取cookie
	rawCookie := u.Cookie

	// 2. 解析cookie
	cookieData, err := cookies.ParseCookieString(rawCookie)
	if err != nil {
		return fmt.Errorf("解析cookie失败: %w", err)
	}

	// 3. 构造请求数据
	data := buildTPSLRiskData(cookieData, req)

	// 4. 将数据转为JSON并base64编码
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据失败: %w", err)
	}
	encodedData := base64.StdEncoding.EncodeToString(jsonData)

	// 5. 计算ext参数
	ext := utils.GetExt(encodedData)

	// 6. 构造完整URL
	baseURL := "https://ubt.deepcoin.pro/save.gif"
	params := url.Values{}
	params.Add("project", "production")
	params.Add("data", encodedData)
	params.Add("ext", fmt.Sprintf("crc=%d", ext))
	fullURL := fmt.Sprintf("%s?%s", baseURL, params.Encode())

	// 7. 发送HTTP请求
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	httpReq, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}

	// 8. 设置请求头
	httpReq.Header.Set("accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	httpReq.Header.Set("accept-language", "zh-CN,zh;q=0.9")
	httpReq.Header.Set("referer", "https://www.deepcoin.com/")
	httpReq.Header.Set("sec-ch-ua", `"Not(A:Brand";v="8", "Chromium";v="144", "Google Chrome";v="144"`)
	httpReq.Header.Set("sec-ch-ua-mobile", "?0")
	httpReq.Header.Set("sec-ch-ua-platform", `"macOS"`)
	httpReq.Header.Set("sec-fetch-dest", "image")
	httpReq.Header.Set("sec-fetch-mode", "no-cors")
	httpReq.Header.Set("sec-fetch-site", "cross-site")
	httpReq.Header.Set("sec-fetch-storage-access", "active")
	httpReq.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	// 9. 发送请求
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 10. 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}

	// 11. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	return nil
}

// buildTPSLRiskData 构造止盈止损风控数据
func buildTPSLRiskData(cookieData *cookies.CookieData, req *TPSLRiskRequest) *tradeRiskData {
	// 设置默认值
	coinType := req.CoinType
	if coinType == "" {
		coinType = "CNY"
	}
	languageType := req.LanguageType
	if languageType == "" {
		languageType = "简体中文"
	}
	envPlatform := req.EnvPlatform
	if envPlatform == "" {
		envPlatform = "web_desktop"
	}

	// 获取cookie中的身份信息
	identities := map[string]string{}
	if cookieData.SensorsData.Identities != nil {
		identities["$identity_cookie_id"] = cookieData.SensorsData.Identities.IdentityCookieID
		identities["$identity_login_id"] = cookieData.SensorsData.Identities.IdentityLoginID
		identities["identity_h5_id"] = cookieData.SensorsData.Identities.IdentityH5ID
	}

	// 当前时间戳（毫秒）
	now := time.Now().UnixNano() / int64(time.Millisecond)

	// 构造数据
	data := &tradeRiskData{
		Identities:  identities,
		DistinctID:  cookieData.GetDistinctID(),
		LoginID:     req.LoginID,
		AnonymousID: cookieData.GetDeviceID(),
		Type:        "track",
		Event:       "PositionTPSL", // 止盈止损事件
		Time:        now,
		TrackID:     now % 1000000000, // 简单生成trackID
		FlushTime:   now,
		Lib: map[string]string{
			"$lib":         "js",
			"$lib_method":  "code",
			"$lib_version": "1.26.4",
		},
		Properties: map[string]any{
			"$timezone_offset":            -480,
			"$screen_height":              1080,
			"$screen_width":               1920,
			"$viewport_height":            875,
			"$viewport_width":             1920,
			"$lib":                        "js",
			"$lib_version":                "1.26.4",
			"$latest_traffic_source_type": cookieData.SensorsData.Props.LatestTrafficSourceType,
			"$latest_search_keyword":      cookieData.SensorsData.Props.LatestSearchKeyword,
			"$latest_referrer":            cookieData.SensorsData.Props.LatestReferrer,
			"platform_type":               "WEB-ONE",
			"production_version":          "国内版",
			"language_type":               languageType,
			"coin_type":                   coinType,
			"is_login":                    true,
			"env_platform":                envPlatform,
			"success":                     req.Success,
			"tpsl_volume_type":            req.TPSLVolumeType,
			"tpsl_trade_mode":             req.TPSLTradeMode,
			"tp_action_type":              req.TPActionType,
			"tp_trigger_percent":          req.TPTriggerPercent,
			"tp_trigger_price":            req.TPTriggerPrice,
			"sl_action_type":              req.SLActionType,
			"sl_trigger_percent":          req.SLTriggerPercent,
			"sl_trigger_price":            req.SLTriggerPrice,
			"instrument_id_name":          req.InstrumentIDName,
			"tp_slider":                   req.TPSlider,
			"sl_slider":                   req.SLSlider,
			"volume_slider":               req.VolumeSlider,
			"$is_first_day":               false,
			"$url":                        fmt.Sprintf("https://www.deepcoin.com/turbo/zh/swap/%s", req.InstrumentIDName),
			"$title":                      fmt.Sprintf("▼ 70,647.3 | %s - Deepcoin", req.InstrumentIDName),
		},
	}

	return data
}

// OrderRequest 下单请求参数
type OrderRequest struct {
	InstrumentID   string // 交易对，如 "BTCUSDT"
	Volume         int    // 交易数量
	Direction      string // 方向: "0"=买入, "1"=卖出  Direction-OffsetFlag 00开多 10开空 11平多 01平空
	OrderPriceType string // 订单类型: "0"=限价, "4"=市价
	Price          string // 价格（市价时可以为空）
	OffsetFlag     string // 开平标志: "0"=开仓, "1"=平仓
	IsCrossMargin  int    // 保证金模式: 1=全仓, 0=逐仓
	Lever          int    // 杠杆倍数

	// 可选参数
	ExchangeID  string // 交易所ID，默认 "DeepCoin"
	AppID       int    // 应用ID，默认 547798
	ConvertPOST int    // 固定值 1
}

// orderRequestInternal 内部请求结构（用于JSON序列化）
type orderRequestInternal struct {
	ExchangeID     string `json:"ExchangeID"`
	MemberID       string `json:"MemberID"`
	InstrumentID   string `json:"InstrumentID"`
	Volume         int    `json:"Volume"`
	Direction      string `json:"Direction"`
	AccountID      string `json:"AccountID"`
	IsCrossMargin  int    `json:"IsCrossMargin"`
	UserID         string `json:"UserID"`
	OrderPriceType string `json:"OrderPriceType"`
	PcPrice        string `json:"pcPrice,omitempty"`
	OffsetFlag     string `json:"OffsetFlag"`
	TradeUnitID    string `json:"TradeUnitID"`
	Lever          int    `json:"lever"`
	AppID          int    `json:"appid"`
	RandomStr      string `json:"randomstr"`
	Timestamp      int64  `json:"timestamp"`
	ConvertPOST    int    `json:"convertPOST"`
	Sign           string `json:"sign"`
}

// OrderData 订单详细数据
type OrderData struct {
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

// PositionData 持仓详细数据
type PositionData struct {
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
	SLTriggerPrice    float64 `json:"SLTriggerPrice"`
	SettlementGroup   string  `json:"SettlementGroup"`
	ShortFrozen       int     `json:"ShortFrozen"`
	ShortFrozenMargin float64 `json:"ShortFrozenMargin"`
	TPTriggerPrice    float64 `json:"TPTriggerPrice"`
	TotalCloseProfit  float64 `json:"TotalCloseProfit"`
	TotalPositionCost float64 `json:"TotalPositionCost"`
	TradeFee          float64 `json:"TradeFee"`
	TradeUnitID       string  `json:"TradeUnitID"`
	UpdateTime        int64   `json:"UpdateTime"`
	UseMargin         float64 `json:"UseMargin"`
	UserID            string  `json:"UserID"`
}

// OrderResponseData 响应数据项
type OrderResponseData struct {
	Table string      `json:"table"`
	Data  interface{} `json:"data"` // 可以是 OrderData 或 PositionData
}

// OrderResponse 下单响应
type OrderResponse struct {
	Code int                 `json:"code"`
	Msg  string              `json:"msg"`
	Data []OrderResponseData `json:"data"`
}

// GetOrderData 从响应中获取订单数据
func (r *OrderResponse) GetOrderData() (*OrderData, error) {
	for _, item := range r.Data {
		if item.Table == "Order" {
			// 将 interface{} 转换为 OrderData
			jsonData, err := json.Marshal(item.Data)
			if err != nil {
				return nil, fmt.Errorf("序列化订单数据失败: %w", err)
			}
			var orderData OrderData
			if err := json.Unmarshal(jsonData, &orderData); err != nil {
				return nil, fmt.Errorf("反序列化订单数据失败: %w", err)
			}
			return &orderData, nil
		}
	}
	return nil, fmt.Errorf("响应中未找到订单数据")
}

// GetPositionData 从响应中获取持仓数据
func (r *OrderResponse) GetPositionData() (*PositionData, error) {
	for _, item := range r.Data {
		if item.Table == "Position" {
			// 将 interface{} 转换为 PositionData
			jsonData, err := json.Marshal(item.Data)
			if err != nil {
				return nil, fmt.Errorf("序列化持仓数据失败: %w", err)
			}
			var positionData PositionData
			if err := json.Unmarshal(jsonData, &positionData); err != nil {
				return nil, fmt.Errorf("反序列化持仓数据失败: %w", err)
			}
			return &positionData, nil
		}
	}
	return nil, fmt.Errorf("响应中未找到持仓数据")
}

// SendOrderInsert 发送下单请求
// u: 用户信息（包含cookie、token等）
// req: 下单请求参数
func SendOrderInsert(u *user.User, req *OrderRequest) (*OrderResponse, error) {
	startTime := time.Now()
	log.Printf("[SendOrderInsert] ========== 开始处理下单请求 ==========")
	log.Printf("[SendOrderInsert] InstrumentID=%s, Volume=%d, Direction=%s", req.InstrumentID, req.Volume, req.Direction)

	// 参数校验
	if u == nil {
		return nil, fmt.Errorf("用户对象为空")
	}
	if req == nil {
		return nil, fmt.Errorf("请求对象为空")
	}

	// 1. 解析cookie获取用户信息
	step1Start := time.Now()
	cookieData, err := cookies.ParseCookieString(u.Cookie)
	if err != nil {
		return nil, fmt.Errorf("解析cookie失败: %w", err)
	}
	step1Duration := time.Since(step1Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤1-解析Cookie: %v", step1Duration)

	// 2. 获取用户ID（distinct_id）
	step2Start := time.Now()
	userID := cookieData.GetDistinctID()
	if userID == "" {
		return nil, fmt.Errorf("无法从cookie中获取用户ID")
	}
	deviceID := cookieData.GetDeviceID()
	step2Duration := time.Since(step2Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤2-获取用户信息: %v (UserID=%s)", step2Duration, userID)

	// 3. 设置默认值
	exchangeID := req.ExchangeID
	if exchangeID == "" {
		exchangeID = "DeepCoin"
	}
	appID := req.AppID
	if appID == 0 {
		appID = 547798
	}

	// 4. 生成随机字符串和时间戳
	randomStr := generateRandomString(6)
	timestamp := time.Now().UnixMilli()

	// 5. 构造内部请求结构
	step5Start := time.Now()
	internalReq := &orderRequestInternal{
		ExchangeID:     exchangeID,
		MemberID:       userID,
		InstrumentID:   req.InstrumentID,
		Volume:         req.Volume,
		Direction:      req.Direction,
		AccountID:      userID,
		IsCrossMargin:  req.IsCrossMargin,
		UserID:         userID,
		OrderPriceType: req.OrderPriceType,
		PcPrice:        req.Price,
		OffsetFlag:     req.OffsetFlag,
		TradeUnitID:    userID,
		Lever:          req.Lever,
		AppID:          appID,
		RandomStr:      randomStr,
		Timestamp:      timestamp,
		ConvertPOST:    1,
	}
	step5Duration := time.Since(step5Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤5-构造请求结构: %v", step5Duration)

	// 6. 计算签名
	step6Start := time.Now()
	message, err := utils.ConvertToMessage(internalReq)
	if err != nil {
		return nil, fmt.Errorf("转换为消息失败: %w", err)
	}
	convertDuration := time.Since(step6Start)

	signStart := time.Now()
	sign, err := utils.CalculateSign(message)
	if err != nil {
		return nil, fmt.Errorf("计算签名失败: %w", err)
	}
	internalReq.Sign = sign
	signDuration := time.Since(signStart)
	step6Duration := time.Since(step6Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤6-计算签名: %v (转换消息: %v, 计算sign: %v)", step6Duration, convertDuration, signDuration)

	// 7. 序列化为JSON
	step7Start := time.Now()
	jsonData, err := json.Marshal(internalReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}
	step7Duration := time.Since(step7Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤7-JSON序列化: %v (大小: %d 字节)", step7Duration, len(jsonData))

	// 8. 获取签名器并计算HMAC
	step8Start := time.Now()
	signer, err := GetGlobalSigner()
	if err != nil {
		return nil, fmt.Errorf("获取签名器失败: %w", err)
	}
	if signer == nil {
		return nil, fmt.Errorf("签名器为空，请确保已调用InitGlobalSigner")
	}
	getSignerDuration := time.Since(step8Start)

	hmacStart := time.Now()
	hmac, err := signer.SignParams(message)
	if err != nil {
		return nil, fmt.Errorf("计算HMAC签名失败: %w", err)
	}
	hmacDuration := time.Since(hmacStart)
	step8Duration := time.Since(step8Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤8-HMAC签名: %v (获取签名器: %v, 计算HMAC: %v)", step8Duration, getSignerDuration, hmacDuration)

	// 9. 生成requestid
	requestID := generateRequestID()

	// 10. 创建HTTP请求并设置请求头
	step10Start := time.Now()
	// apiURL := "https://www.deepcoin.wang/v2/public/swap/SendOrderInsert"
	// apiURL := "http://43.159.108.73/v2/public/swap/SendOrderInsert"
	apiURL := "https://www.deepcoin.com/v2/public/swap/SendOrderInsert"
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	step10Duration := time.Since(step10Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤10-创建HTTP请求: %v", step10Duration)

	// 11. 设置请求头
	step11Start := time.Now()
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	httpReq.Header.Set("Content-Type", "application/json;charset=UTF-8")
	httpReq.Header.Set("Origin", "https://www.deepcoin.com")
	httpReq.Header.Set("Referer", fmt.Sprintf("https://www.deepcoin.com/turbo/zh/swap/%s", req.InstrumentID))
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	// 业务相关请求头
	httpReq.Header.Set("appid", fmt.Sprintf("%d", appID))
	httpReq.Header.Set("device", deviceID)
	httpReq.Header.Set("hmac", hmac)
	httpReq.Header.Set("lang", "zh")
	httpReq.Header.Set("platform", "pc")
	httpReq.Header.Set("requestid", requestID)
	httpReq.Header.Set("timestamp", fmt.Sprintf("%d", timestamp))
	httpReq.Header.Set("token", u.Token)
	httpReq.Header.Set("Otoken", u.Token)
	httpReq.Header.Set("Version", "t26.02.07")
	httpReq.Header.Set("uid", userID)

	// Sentry相关请求头
	if u.SentryRelease != "" {
		httpReq.Header.Set("baggage", fmt.Sprintf(
			"sentry-environment=production,sentry-release=%s,sentry-public_key=%s,sentry-trace_id=%s,sentry-sampled=false,sentry-sample_rand=%.16f,sentry-sample_rate=0",
			u.SentryRelease,
			u.SentryPublicKey,
			generateTraceID(),
			rand.Float64(),
		))
		httpReq.Header.Set("sentry-trace", fmt.Sprintf("%s-%s-0", generateTraceID(), generateSpanID()))
	}

	// 其他安全请求头
	httpReq.Header.Set("sec-ch-ua", `"Not(A:Brand";v="8", "Chromium";v="144", "Google Chrome";v="144"`)
	httpReq.Header.Set("sec-ch-ua-mobile", "?0")
	httpReq.Header.Set("sec-ch-ua-platform", `"macOS"`)
	httpReq.Header.Set("Sec-Fetch-Dest", "empty")
	httpReq.Header.Set("Sec-Fetch-Mode", "cors")
	httpReq.Header.Set("Sec-Fetch-Site", "same-origin")
	httpReq.Header.Set("x-requested-with", "XMLHttpRequest")
	httpReq.Header.Set("Cookie", u.Cookie)
	step11Duration := time.Since(step11Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤11-设置请求头: %v", step11Duration)

	// 12. 发送HTTP请求
	step12Start := time.Now()
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()
	step12Duration := time.Since(step12Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤12-HTTP请求: %v (状态码: %d)", step12Duration, resp.StatusCode)

	// 13. 读取响应
	step13Start := time.Now()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	step13Duration := time.Since(step13Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤13-读取响应: %v (大小: %d 字节)", step13Duration, len(body))

	// 14. 解析响应
	step14Start := time.Now()
	var orderResp OrderResponse
	if err := json.Unmarshal(body, &orderResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应内容: %s", err, string(body))
	}
	step14Duration := time.Since(step14Start)
	log.Printf("[SendOrderInsert] ⏱️  步骤14-解析响应: %v (code=%d, msg=%s)", step14Duration, orderResp.Code, orderResp.Msg)

	// 15. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return &orderResp, fmt.Errorf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	if orderResp.Code != 0 {
		log.Printf("[SendOrderInsert] 下单失败: code=%d, msg=%s", orderResp.Code, orderResp.Msg)
		return &orderResp, fmt.Errorf("下单失败: %s", orderResp.Msg)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[SendOrderInsert] ========== 下单成功完成 总耗时: %v ==========", totalDuration)
	log.Printf("[SendOrderInsert] 📊 耗时分析:")
	log.Printf("  - Cookie解析: %v", step1Duration)
	log.Printf("  - 获取用户信息: %v", step2Duration)
	log.Printf("  - 构造请求: %v", step5Duration)
	log.Printf("  - 计算签名: %v (转换: %v, sign: %v)", step6Duration, convertDuration, signDuration)
	log.Printf("  - JSON序列化: %v", step7Duration)
	log.Printf("  - HMAC签名: %v (获取签名器: %v, 计算: %v)", step8Duration, getSignerDuration, hmacDuration)
	log.Printf("  - 创建请求: %v", step10Duration)
	log.Printf("  - 设置Header: %v", step11Duration)
	log.Printf("  - HTTP请求: %v ⭐", step12Duration)
	log.Printf("  - 读取响应: %v", step13Duration)
	log.Printf("  - 解析响应: %v", step14Duration)

	return &orderResp, nil
}

// ============================= 市价全平接口 =============================

// ClosePosRequest 市价全平请求参数
type ClosePosRequest struct {
	PositionID string // 持仓ID
	// 可选参数
	ExchangeID   string // 交易所ID，默认 "DeepCoin"
	ProductGroup string // 产品组，默认 "SwapU"
	AppID        int    // 应用ID，默认 547798
}

// closePosSignInternal 用于签名计算（IsCopyTrade 用 string 避免 bool false 被 isZeroValue 跳过）
type closePosSignInternal struct {
	AccountID    string `json:"AccountID"`
	ExchangeID   string `json:"ExchangeID"`
	IsCopyTrade  string `json:"IsCopyTrade"`
	MemberID     string `json:"MemberID"`
	PositionID   string `json:"PositionID"`
	ProductGroup string `json:"ProductGroup"`
	AppID        int    `json:"appid"`
	RandomStr    string `json:"randomstr"`
	Timestamp    int64  `json:"timestamp"`
	ConvertPOST  int    `json:"convertPOST"`
	Sign         string `json:"sign"`
}

// closePosRequestBody 完整请求体（IsCopyTrade 为 bool，JSON 序列化为 false）
type closePosRequestBody struct {
	AccountID    string `json:"AccountID"`
	ExchangeID   string `json:"ExchangeID"`
	IsCopyTrade  bool   `json:"IsCopyTrade"`
	MemberID     string `json:"MemberID"`
	PositionID   string `json:"PositionID"`
	ProductGroup string `json:"ProductGroup"`
	AppID        int    `json:"appid"`
	RandomStr    string `json:"randomstr"`
	Timestamp    int64  `json:"timestamp"`
	ConvertPOST  int    `json:"convertPOST"`
	Sign         string `json:"sign"`
}

// ClosePosResponse 市价全平响应
type ClosePosResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		ErrorList []interface{} `json:"errorList"`
		Spend     int           `json:"spend"`
	} `json:"data"`
}

// SendClosePos 发送市价全平请求
func SendClosePos(u *user.User, req *ClosePosRequest) (*ClosePosResponse, error) {
	log.Printf("[SendClosePos] 开始处理市价全平请求, PositionID=%s", req.PositionID)

	if u == nil {
		return nil, fmt.Errorf("用户对象为空")
	}
	if req == nil || req.PositionID == "" {
		return nil, fmt.Errorf("请求对象为空或PositionID为空")
	}

	cookieData, err := cookies.ParseCookieString(u.Cookie)
	if err != nil {
		return nil, fmt.Errorf("解析cookie失败: %w", err)
	}

	userID := cookieData.GetDistinctID()
	if userID == "" {
		return nil, fmt.Errorf("无法从cookie中获取用户ID")
	}
	deviceID := cookieData.GetDeviceID()

	exchangeID := req.ExchangeID
	if exchangeID == "" {
		exchangeID = "DeepCoin"
	}
	productGroup := req.ProductGroup
	if productGroup == "" {
		productGroup = "SwapU"
	}
	appID := req.AppID
	if appID == 0 {
		appID = 547798
	}

	randomStr := generateRandomString(6)
	timestamp := time.Now().UnixMilli()

	signReq := &closePosSignInternal{
		AccountID:    userID,
		ExchangeID:   exchangeID,
		IsCopyTrade:  "false",
		MemberID:     userID,
		PositionID:   req.PositionID,
		ProductGroup: productGroup,
		AppID:        appID,
		RandomStr:    randomStr,
		Timestamp:    timestamp,
		ConvertPOST:  1,
	}

	message, err := utils.ConvertToMessage(signReq)
	if err != nil {
		return nil, fmt.Errorf("转换为消息失败: %w", err)
	}
	sign, err := utils.CalculateSign(message)
	if err != nil {
		return nil, fmt.Errorf("计算签名失败: %w", err)
	}

	bodyReq := &closePosRequestBody{
		AccountID:    userID,
		ExchangeID:   exchangeID,
		IsCopyTrade:  false,
		MemberID:     userID,
		PositionID:   req.PositionID,
		ProductGroup: productGroup,
		AppID:        appID,
		RandomStr:    randomStr,
		Timestamp:    timestamp,
		ConvertPOST:  1,
		Sign:         sign,
	}

	jsonData, err := json.Marshal(bodyReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	signer, err := GetGlobalSigner()
	if err != nil {
		return nil, fmt.Errorf("获取签名器失败: %w", err)
	}
	hmac, err := signer.SignParams(message)
	if err != nil {
		return nil, fmt.Errorf("计算HMAC签名失败: %w", err)
	}

	requestID := generateRequestID()

	// apiURL := "https://www.deepcoin.wang/v2/public/swap/ClosePos"
	apiURL := "https://www.deepcoin.com/v2/public/swap/ClosePos"
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	httpReq.Header.Set("Content-Type", "application/json;charset=UTF-8")
	httpReq.Header.Set("Origin", "https://www.deepcoin.com")
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")

	httpReq.Header.Set("appid", fmt.Sprintf("%d", appID))
	httpReq.Header.Set("device", deviceID)
	httpReq.Header.Set("hmac", hmac)
	httpReq.Header.Set("lang", "zh")
	httpReq.Header.Set("platform", "pc")
	httpReq.Header.Set("requestid", requestID)
	httpReq.Header.Set("timestamp", fmt.Sprintf("%d", timestamp))
	httpReq.Header.Set("token", u.Token)
	httpReq.Header.Set("Otoken", u.Token)
	httpReq.Header.Set("uid", userID)

	if u.SentryRelease != "" {
		httpReq.Header.Set("baggage", fmt.Sprintf(
			"sentry-environment=production,sentry-release=%s,sentry-public_key=%s,sentry-trace_id=%s,sentry-sampled=false,sentry-sample_rand=%.16f,sentry-sample_rate=0",
			u.SentryRelease,
			u.SentryPublicKey,
			generateTraceID(),
			rand.Float64(),
		))
		httpReq.Header.Set("sentry-trace", fmt.Sprintf("%s-%s-0", generateTraceID(), generateSpanID()))
	}

	httpReq.Header.Set("sec-ch-ua", `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`)
	httpReq.Header.Set("sec-ch-ua-mobile", "?0")
	httpReq.Header.Set("sec-ch-ua-platform", `"macOS"`)
	httpReq.Header.Set("Sec-Fetch-Dest", "empty")
	httpReq.Header.Set("Sec-Fetch-Mode", "cors")
	httpReq.Header.Set("Sec-Fetch-Site", "same-origin")
	httpReq.Header.Set("x-requested-with", "XMLHttpRequest")
	httpReq.Header.Set("Cookie", u.Cookie)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var closeResp ClosePosResponse
	if err := json.Unmarshal(body, &closeResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应内容: %s", err, string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return &closeResp, fmt.Errorf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	if closeResp.Code != 0 {
		log.Printf("[SendClosePos] 市价全平失败: code=%d, msg=%s", closeResp.Code, closeResp.Msg)
		return &closeResp, fmt.Errorf("市价全平失败: %s", closeResp.Msg)
	}

	log.Printf("[SendClosePos] 市价全平成功, PositionID=%s, spend=%d", req.PositionID, closeResp.Data.Spend)
	return &closeResp, nil
}

// generateRandomString 生成指定长度的随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// generateRequestID 生成请求ID（32位十六进制字符串）
func generateRequestID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func CalculateHMAC() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// generateTraceID 生成trace ID（32位十六进制字符串）
func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// generateSpanID 生成span ID（16位十六进制字符串）
func generateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// TriggerOrderRequest 止盈止损订单请求参数
type TriggerOrderRequest struct {
	InstrumentID   string  // 交易对，如 "BTCUSDT"
	Direction      string  // 持仓方向: "0"=买入, "1"=卖出 平多仓传1 平空仓传0
	TPTriggerPrice float64 // 止盈触发价格（0表示不设置止盈）
	SLTriggerPrice float64 // 止损触发价格（0表示不设置止损）
	IsCrossMargin  int     // 保证金模式: 1=全仓, 0=逐仓
	Volume         int     // 交易数量
	TPPrice        float64 // 止盈委托价格
	SLPrice        float64 // 止损委托价格

	// 可选参数
	ExchangeID     string // 交易所ID，默认 "DeepCoin"
	OrderPriceType string // 订单类型，默认 "0"（限价）
	OffsetFlag     string // 开平标志，默认 "8"（限价单止盈止损）
	Source         string // 来源，默认 "position"
	Remark         string // 备注，默认 "2"
	BusinessType   string // 业务类型，默认 "X"
	AppID          int    // 应用ID，默认 547798
	ConvertPOST    int    // 固定值 1
}

// triggerOrderRequestInternal 内部请求结构（用于JSON序列化）
type triggerOrderRequestInternal struct {
	ExchangeID     string  `json:"ExchangeID"`
	MemberID       string  `json:"MemberID"`
	InstrumentID   string  `json:"InstrumentID"`
	Direction      string  `json:"Direction"`
	AccountID      string  `json:"AccountID"`
	IsCrossMargin  int     `json:"IsCrossMargin"`
	UserID         string  `json:"UserID"`
	OrderPriceType string  `json:"OrderPriceType"`
	OffsetFlag     string  `json:"OffsetFlag"`
	TradeUnitID    string  `json:"TradeUnitID"`
	Source         string  `json:"Source"`
	Remark         string  `json:"Remark"`
	TPTriggerPrice float64 `json:"TPTriggerPrice"`
	SLTriggerPrice float64 `json:"SLTriggerPrice"`
	Volume         int     `json:"Volume"`
	TPPrice        float64 `json:"TPPrice"`
	SLPrice        float64 `json:"SLPrice"`
	BusinessType   string  `json:"BusinessType"`
	AppID          int     `json:"appid"`
	RandomStr      string  `json:"randomstr"`
	Timestamp      int64   `json:"timestamp"`
	ConvertPOST    int     `json:"convertPOST"`
	Sign           string  `json:"sign"`
}

// TriggerOrderData 触发订单数据
type TriggerOrderData struct {
	APPID            string  `json:"APPID"`
	AccountID        string  `json:"AccountID"`
	BusinessType     string  `json:"BusinessType"`
	Direction        string  `json:"Direction"`
	ExchangeID       string  `json:"ExchangeID"`
	InsertTime       int64   `json:"InsertTime"`
	InstrumentID     string  `json:"InstrumentID"`
	IsCrossMargin    int     `json:"IsCrossMargin"`
	Leverage         int     `json:"Leverage"`
	MemberID         string  `json:"MemberID"`
	OffsetFlag       string  `json:"OffsetFlag"`
	OrderPriceType   string  `json:"OrderPriceType"`
	OrderSysID       string  `json:"OrderSysID"`
	OrderType        string  `json:"OrderType"`
	PosiDirection    string  `json:"PosiDirection"`
	PositionID       string  `json:"PositionID"`
	Remark           string  `json:"Remark"`
	SLTriggerPrice   float64 `json:"SLTriggerPrice"`
	TPTriggerPrice   float64 `json:"TPTriggerPrice"`
	TradeUnitID      string  `json:"TradeUnitID"`
	TriggerOrderType string  `json:"TriggerOrderType"`
	TriggerStatus    string  `json:"TriggerStatus"`
	UpdateTime       int64   `json:"UpdateTime"`
	// 其他字段可根据需要添加
}

// TriggerOrderResponse 止盈止损订单响应
type TriggerOrderResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data []struct {
		Table string           `json:"table"`
		Data  TriggerOrderData `json:"data"`
	} `json:"data"`
}

// SendTriggerOrderInsert 发送止盈止损订单请求
// u: 用户信息（包含cookie、token等）
// req: 止盈止损请求参数
func SendTriggerOrderInsert(u *user.User, req *TriggerOrderRequest) (*TriggerOrderResponse, error) {
	// 1. 解析cookie获取用户信息
	cookieData, err := cookies.ParseCookieString(u.Cookie)
	if err != nil {
		return nil, fmt.Errorf("解析cookie失败: %w", err)
	}

	// 2. 获取用户ID（distinct_id）
	userID := cookieData.GetDistinctID()
	if userID == "" {
		return nil, fmt.Errorf("无法从cookie中获取用户ID")
	}

	// 3. 获取设备ID
	deviceID := cookieData.GetDeviceID()
	//if deviceID == "" {
	//	return nil, fmt.Errorf("无法从cookie中获取设备ID")
	//}

	// 4. 设置默认值
	exchangeID := req.ExchangeID
	if exchangeID == "" {
		exchangeID = "DeepCoin"
	}
	orderPriceType := req.OrderPriceType
	if orderPriceType == "" {
		orderPriceType = "0" // 限价
	}
	offsetFlag := req.OffsetFlag
	if offsetFlag == "" {
		offsetFlag = "8" // 止盈止损
	}
	source := req.Source
	if source == "" {
		source = "position"
	}
	remark := req.Remark
	if remark == "" {
		remark = "2"
	}
	businessType := req.BusinessType
	if businessType == "" {
		businessType = "X"
	}
	appID := req.AppID
	if appID == 0 {
		appID = 547798
	}

	// 5. 生成随机字符串
	randomStr := generateRandomString(6)

	// 6. 获取当前时间戳（毫秒）
	timestamp := time.Now().UnixMilli()

	// 7. 构造内部请求结构
	internalReq := &triggerOrderRequestInternal{
		ExchangeID:     exchangeID,
		MemberID:       userID,
		InstrumentID:   req.InstrumentID,
		Direction:      req.Direction,
		AccountID:      userID,
		IsCrossMargin:  req.IsCrossMargin,
		UserID:         userID, // 通常为空
		OrderPriceType: orderPriceType,
		OffsetFlag:     offsetFlag,
		TradeUnitID:    userID,
		Source:         source,
		Remark:         remark,
		TPTriggerPrice: req.TPTriggerPrice,
		SLTriggerPrice: req.SLTriggerPrice,
		Volume:         req.Volume,
		TPPrice:        req.TPPrice,
		SLPrice:        req.SLPrice,
		BusinessType:   businessType,
		AppID:          appID,
		RandomStr:      randomStr,
		Timestamp:      timestamp,
		ConvertPOST:    1,
	}

	// 8. 计算签名
	message, err := utils.ConvertToMessage(internalReq)
	if err != nil {
		return nil, fmt.Errorf("转换为消息失败: %w", err)
	}
	sign, err := utils.CalculateSign(message)
	if err != nil {
		return nil, fmt.Errorf("计算签名失败: %w", err)
	}
	internalReq.Sign = sign

	// 9. 序列化为JSON
	jsonData, err := json.Marshal(internalReq)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 10. 创建HTTP请求
	apiURL := "https://www.deepcoin.com/v2/public/swap/SendTriggerOrderInsert"
	httpReq, err := http.NewRequest("POST", apiURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 11. 计算请求头中的hmac（生成32位随机字符串）
	signer, err := GetGlobalSigner()
	if err != nil {
		return nil, fmt.Errorf("获取签名器失败: %w", err)
	}
	hmac, err := signer.SignParams(message)
	if err != nil {
		return nil, fmt.Errorf("计算签名失败: %w", err)
	}

	// 12. 生成requestid
	requestID := generateRequestID()

	// 13. 设置请求头
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	httpReq.Header.Set("Content-Type", "application/json;charset=UTF-8")
	httpReq.Header.Set("Origin", "https://www.deepcoin.com")
	httpReq.Header.Set("Referer", fmt.Sprintf("https://www.deepcoin.com/turbo/zh/swap/%s", req.InstrumentID))
	httpReq.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36")

	// 业务相关请求头
	httpReq.Header.Set("appid", fmt.Sprintf("%d", appID))
	httpReq.Header.Set("device", deviceID)
	httpReq.Header.Set("hmac", hmac)
	httpReq.Header.Set("lang", "zh")
	httpReq.Header.Set("platform", "pc")
	httpReq.Header.Set("requestid", requestID)
	httpReq.Header.Set("timestamp", fmt.Sprintf("%d", timestamp))
	httpReq.Header.Set("token", u.Token)
	httpReq.Header.Set("Otoken", u.Token)
	httpReq.Header.Set("uid", userID)
	httpReq.Header.Set("Version", "t26.02.07")

	// Sentry相关请求头
	if u.SentryRelease != "" {
		httpReq.Header.Set("baggage", fmt.Sprintf(
			"sentry-environment=production,sentry-release=%s,sentry-public_key=%s,sentry-trace_id=%s,sentry-sampled=false,sentry-sample_rand=%.16f,sentry-sample_rate=0",
			u.SentryRelease,
			u.SentryPublicKey,
			generateTraceID(),
			rand.Float64(),
		))
		httpReq.Header.Set("sentry-trace", fmt.Sprintf("%s-%s-0", generateTraceID(), generateSpanID()))
	}

	// 其他安全请求头
	httpReq.Header.Set("sec-ch-ua", `"Not(A:Brand";v="8", "Chromium";v="144", "Google Chrome";v="144"`)
	httpReq.Header.Set("sec-ch-ua-mobile", "?0")
	httpReq.Header.Set("sec-ch-ua-platform", `"macOS"`)
	httpReq.Header.Set("Sec-Fetch-Dest", "empty")
	httpReq.Header.Set("Sec-Fetch-Mode", "cors")
	httpReq.Header.Set("Sec-Fetch-Site", "same-origin")
	httpReq.Header.Set("x-requested-with", "XMLHttpRequest")

	// 14. 设置Cookie
	httpReq.Header.Set("Cookie", u.Cookie)

	// 15. 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 16. 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 17. 解析响应
	var triggerResp TriggerOrderResponse
	if err := json.Unmarshal(body, &triggerResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w, 响应内容: %s", err, string(body))
	}

	// 18. 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return &triggerResp, fmt.Errorf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	if triggerResp.Code != 0 {
		return &triggerResp, fmt.Errorf("设置止盈止损失败: %s", triggerResp.Msg)
	}

	return &triggerResp, nil
}

// ExampleTriggerOrderUsage 止盈止损接口使用示例
func ExampleTriggerOrderUsage() {
	// 1. 创建用户对象
	u := user.NewUser(
		`sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229542187%22...%7D; i18next=en; theme=dark`,
		"DUw4954/pISWPVvIEvgV6OFUqYRUMsDhXEmvz3eFl9TQka+CeekG0PgDbwx/Lailr6Tb+k1hp1yXhSUgbFETBA==",
		"ec3532632e2f7f732d67d8160eb8cf7069281616",
		"1e6f92c4133350179d7853f8bb1d8fb7",
	)

	// 2. 为多仓设置止盈止损
	triggerReq := &TriggerOrderRequest{
		InstrumentID:   "BTCUSDT", // 交易对
		Direction:      "0",       // 0=多仓
		TPTriggerPrice: 66070.1,   // 止盈价格
		SLTriggerPrice: 83305.8,   // 止损价格
		IsCrossMargin:  1,         // 1=全仓
	}

	// 3. 发送止盈止损请求
	resp, err := SendTriggerOrderInsert(u, triggerReq)
	if err != nil {
		fmt.Printf("设置止盈止损失败: %v\n", err)
		return
	}

	if len(resp.Data) > 0 {
		fmt.Printf("止盈止损设置成功，订单ID: %s\n", resp.Data[0].Data.OrderSysID)
		fmt.Printf("止盈价格: %.1f, 止损价格: %.1f\n",
			resp.Data[0].Data.TPTriggerPrice,
			resp.Data[0].Data.SLTriggerPrice)
	}

	// 示例2: 只设置止盈（不设置止损）
	tpOnlyReq := &TriggerOrderRequest{
		InstrumentID:   "BTCUSDT",
		Direction:      "0",
		TPTriggerPrice: 70000.0, // 只设置止盈
		SLTriggerPrice: 0,       // 0表示不设置止损
		IsCrossMargin:  1,
	}

	resp2, err := SendTriggerOrderInsert(u, tpOnlyReq)
	if err != nil {
		fmt.Printf("设置止盈失败: %v\n", err)
		return
	}

	if len(resp2.Data) > 0 {
		fmt.Printf("止盈设置成功，订单ID: %s\n", resp2.Data[0].Data.OrderSysID)
	}

	// 示例3: 只设置止损（不设置止盈）
	slOnlyReq := &TriggerOrderRequest{
		InstrumentID:   "BTCUSDT",
		Direction:      "0",
		TPTriggerPrice: 0,       // 0表示不设置止盈
		SLTriggerPrice: 60000.0, // 只设置止损
		IsCrossMargin:  1,
	}

	resp3, err := SendTriggerOrderInsert(u, slOnlyReq)
	if err != nil {
		fmt.Printf("设置止损失败: %v\n", err)
		return
	}

	if len(resp3.Data) > 0 {
		fmt.Printf("止损设置成功，订单ID: %s\n", resp3.Data[0].Data.OrderSysID)
	}
}

// ExampleTPSLRiskUsage 止盈止损风控接口使用示例
func ExampleTPSLRiskUsage() {
	// 1. 创建用户对象（风控接口不需要token）
	u := user.NewUser(
		`sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229542187%22...%7D; i18next=en; theme=dark`,
		"", // 风控接口不需要token
		"",
		"",
	)

	// 2. 构造止盈止损风控请求（同时设置止盈和止损）
	tpslRiskReq := &TPSLRiskRequest{
		Username:         "test_user",
		LoginID:          "9542187",
		InstrumentIDName: "BTCUSDT",
		Success:          "true",    // 设置成功
		TPSLVolumeType:   "全部",      // 数量类型：全部/部分
		TPSLTradeMode:    "市价",      // 交易模式：市价/限价
		TPActionType:     "收益率",     // 止盈触发类型：收益率/价格
		TPTriggerPercent: "",        // 止盈百分比（空表示使用价格）
		TPTriggerPrice:   "66070.1", // 止盈价格
		SLActionType:     "收益率",     // 止损触发类型：收益率/价格
		SLTriggerPercent: "",        // 止损百分比（空表示使用价格）
		SLTriggerPrice:   "83305.8", // 止损价格
		TPSlider:         false,     // 是否使用滑块设置止盈
		SLSlider:         false,     // 是否使用滑块设置止损
		VolumeSlider:     false,     // 是否使用滑块设置数量
	}

	// 3. 发送风控请求
	err := SendTPSLRiskRequest(u, tpslRiskReq)
	if err != nil {
		fmt.Printf("发送止盈止损风控请求失败: %v\n", err)
		return
	}

	fmt.Println("止盈止损风控请求发送成功")

	// 示例2: 只设置止盈的风控埋点
	tpOnlyRiskReq := &TPSLRiskRequest{
		Username:         "test_user",
		LoginID:          "9542187",
		InstrumentIDName: "BTCUSDT",
		Success:          "true",
		TPSLVolumeType:   "全部",
		TPSLTradeMode:    "市价",
		TPActionType:     "价格", // 使用价格触发
		TPTriggerPercent: "",
		TPTriggerPrice:   "70000.0", // 只设置止盈价格
		SLActionType:     "收益率",
		SLTriggerPercent: "",
		SLTriggerPrice:   "", // 不设置止损
		TPSlider:         false,
		SLSlider:         false,
		VolumeSlider:     false,
	}

	err = SendTPSLRiskRequest(u, tpOnlyRiskReq)
	if err != nil {
		fmt.Printf("发送止盈风控请求失败: %v\n", err)
		return
	}

	fmt.Println("止盈风控请求发送成功")

	// 示例3: 使用百分比设置止盈止损的风控埋点
	percentRiskReq := &TPSLRiskRequest{
		Username:         "test_user",
		LoginID:          "9542187",
		InstrumentIDName: "BTCUSDT",
		Success:          "true",
		TPSLVolumeType:   "全部",
		TPSLTradeMode:    "市价",
		TPActionType:     "收益率",
		TPTriggerPercent: "10", // 止盈10%
		TPTriggerPrice:   "",   // 使用百分比时价格为空
		SLActionType:     "收益率",
		SLTriggerPercent: "5",  // 止损5%
		SLTriggerPrice:   "",   // 使用百分比时价格为空
		TPSlider:         true, // 使用滑块设置
		SLSlider:         true,
		VolumeSlider:     false,
	}

	err = SendTPSLRiskRequest(u, percentRiskReq)
	if err != nil {
		fmt.Printf("发送百分比止盈止损风控请求失败: %v\n", err)
		return
	}

	fmt.Println("百分比止盈止损风控请求发送成功")
}
