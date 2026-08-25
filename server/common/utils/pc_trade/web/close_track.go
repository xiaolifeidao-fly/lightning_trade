package web

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"common/cookies"
	"common/utils"
	"common/utils/pc_trade/user"

	"github.com/sirupsen/logrus"
)

// ---- 埋点失败通知 ----
//
// web 是底层库，不直接依赖 TG——上层用 SetTrackFailureHandler 注入通知方式。
// 动机：埋点失败此前只写 logrus，淹没在日志里无人察觉（上报域名下线导致
// 静默失败长达数周就是这么发生的），必须能主动推送出来。
var trackFailureHandler atomic.Value // func(stage string, err error)

// SetTrackFailureHandler 注册埋点失败通知钩子；传 nil 可注销。
// 钩子内应自行节流——埋点失败往往连续发生。
func SetTrackFailureHandler(h func(stage string, err error)) {
	if h == nil {
		trackFailureHandler.Store((func(string, error))(nil))
		return
	}
	trackFailureHandler.Store(h)
}

// notifyTrackFailure 触发通知；未注册钩子或 err 为 nil 时静默返回。
func notifyTrackFailure(stage string, err error) {
	if err == nil {
		return
	}
	h, _ := trackFailureHandler.Load().(func(string, error))
	if h == nil {
		return
	}
	h(stage, err)
}

// postTrack 发送单条埋点。请求头与网页一致（缺了会让请求在服务端侧显得可疑）。
func postTrack(data *tradeRiskData, loginID string) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化埋点失败: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(jsonData)

	params := url.Values{}
	params.Add("project", "production")
	params.Add("data", encoded)
	params.Add("ext", fmt.Sprintf("crc=%d", utils.GetExt(encoded)))

	httpReq, err := http.NewRequest("GET", trackEndpoint+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("构造埋点请求失败: %w", err)
	}
	setTrackHeaders(httpReq, loginID)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(httpReq)
	if err != nil {
		return fmt.Errorf("发送埋点失败: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("埋点响应状态码 %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// setTrackHeaders 埋点请求头（与网页抓包一致）。
func setTrackHeaders(r *http.Request, loginID string) {
	r.Header.Set("accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	r.Header.Set("accept-language", "zh-CN,zh;q=0.9,en;q=0.8")
	if loginID != "" {
		r.Header.Set("loginuser", loginID)
	}
	r.Header.Set("referer", "https://www.deepcoin.com/")
	r.Header.Set("sec-ch-ua", `"Chromium";v="142", "Google Chrome";v="142", "Not_A Brand";v="99"`)
	r.Header.Set("sec-ch-ua-mobile", "?0")
	r.Header.Set("sec-ch-ua-platform", `"macOS"`)
	r.Header.Set("sec-fetch-dest", "image")
	r.Header.Set("sec-fetch-mode", "no-cors")
	r.Header.Set("sec-fetch-site", "cross-site")
	r.Header.Set("sec-fetch-storage-access", "active")
	r.Header.Set("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	r.Header.Set("x-forwarded-for", "4.2.2.2")
}

// ClosePosTrackRequest 平仓埋点入参。
// 模板来自 2026-07-31 浏览器抓包（网页「市价全平」单仓平仓），字段与之严格对齐。
type ClosePosTrackRequest struct {
	PositionID string  // → de_order_id / local_info.oid
	TradePair  string  // → de_tradepair_name，埋点口径（BTCUSDT），用 TradePairName 转换
	TradeQuote float64 // → trade_quote / local_info.new_price；平仓侧取内存快照 LastPx（≤5s 旧，非成交价）
	DealSize   int     // → de_deal_size，张数
	PriceTrend string  // "up"/"down"，仅决定 $title 的箭头方向；缺省 down
}

// TradePairName 把持仓 instId 转成埋点口径的交易对名：BTC-USDT-SWAP → BTCUSDT。
// 与 trade 包的 instrumentBase 不同——后者会把 USDT 也去掉（得到 "BTC"），
// 那是给 bot-close 注册表做基础币归一用的，两者不可混用。
func TradePairName(instId string) string {
	s := strings.ToUpper(strings.TrimSpace(instId))
	s = strings.ReplaceAll(s, "-", "")
	return strings.TrimSuffix(s, "SWAP")
}

// buildClosePosTrackData 构造平仓埋点数据。
// mark 为空 → 动作事件 TradeCloseAllPosition（带 local_info）；
// mark 非空 → 结果事件 TradeCloseAllPositionResult（带 mark，无 local_info）。
// 两条事件的差异完全照抓包实测，勿凭直觉增删字段。
func buildClosePosTrackData(cookieData *cookies.CookieData, req *ClosePosTrackRequest, mark string) *tradeRiskData {
	identities := map[string]string{}
	if cookieData.SensorsData.Identities != nil {
		identities["$identity_cookie_id"] = cookieData.SensorsData.Identities.IdentityCookieID
		identities["$identity_login_id"] = cookieData.SensorsData.Identities.IdentityLoginID
		identities["identity_h5_id"] = cookieData.SensorsData.Identities.IdentityH5ID
	}

	loginID := cookieData.GetLoginID()
	viewport := GenerateViewportSignals(loginID)
	now := time.Now().UnixNano() / int64(time.Millisecond)

	arrow := "▼"
	if req.PriceTrend == "up" {
		arrow = "▲"
	}

	event := "TradeCloseAllPosition"
	if mark != "" {
		event = "TradeCloseAllPositionResult"
	}

	props := map[string]any{
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
		"production_version":          "国际版",
		"language_type":               "简体中文",
		"coin_type":                   "CNY",
		"is_login":                    true,
		"env_platform":                "web_desktop",

		"button_name":       "市价全平二次确认",
		"de_order_id":       req.PositionID,
		"de_tradepair_name": req.TradePair,
		"trade_quote":       req.TradeQuote,
		"de_deal_size":      req.DealSize,
		"product_group":     "SwapU",
		"message_count":     1,

		"$is_first_day": false,
		"$url":          fmt.Sprintf("https://www.deepcoin.com/swap/zh/%s", req.TradePair),
		"$title":        fmt.Sprintf("%s %s | %s - Deepcoin", arrow, formatThousands1(req.TradeQuote), req.TradePair),
	}

	if mark != "" {
		props["mark"] = mark
	} else {
		// local_info 是 JSON 字符串而非对象（抓包如此）
		li, _ := json.Marshal([]map[string]any{{
			"instrument_id_name": req.TradePair,
			"oid":                req.PositionID,
			"new_price":          req.TradeQuote,
		}})
		props["local_info"] = string(li)
	}

	return &tradeRiskData{
		Identities:  identities,
		DistinctID:  cookieData.GetDistinctID(),
		LoginID:     loginID,
		AnonymousID: cookieData.GetDeviceID(),
		Type:        "track",
		Event:       event,
		Time:        now,
		TrackID:     now % 1000000000,
		FlushTime:   now,
		Lib: map[string]string{
			"$lib":         "js",
			"$lib_method":  "code",
			"$lib_version": "1.26.4",
		},
		Properties: props,
	}
}

// SendClosePosTrack 发送平仓的两条埋点（动作 + 结果），对齐网页行为。
// resultMark 传接口返回的原始 JSON 字符串。
// 埋点失败绝不影响平仓结果——调用方已完成平仓，此处只记日志。
func SendClosePosTrack(u *user.User, req *ClosePosTrackRequest, resultMark string) {
	if u == nil || req == nil || req.PositionID == "" {
		return
	}
	cookieData, err := cookies.ParseCookieString(u.Cookie)
	if err != nil {
		logrus.Warnf("[平仓埋点] 解析 cookie 失败（不影响平仓）: %v", err)
		return
	}
	loginID := cookieData.GetLoginID()
	if err := postTrack(buildClosePosTrackData(cookieData, req, ""), loginID); err != nil {
		logrus.Warnf("[平仓埋点] 动作事件发送失败（不影响平仓）: %v", err)
		notifyTrackFailure("平仓埋点/动作", err)
	}
	if err := postTrack(buildClosePosTrackData(cookieData, req, resultMark), loginID); err != nil {
		logrus.Warnf("[平仓埋点] 结果事件发送失败（不影响平仓）: %v", err)
		notifyTrackFailure("平仓埋点/结果", err)
	}
}

