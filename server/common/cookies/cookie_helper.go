package cookies

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
)

// SensorsCookie 神策数据 cookie 结构
type SensorsCookie struct {
	DistinctID     string          `json:"distinct_id"`      // 用户唯一标识
	FirstID        string          `json:"first_id"`         // 首次访问 ID
	Props          Props           `json:"props"`            // 属性信息
	Identities     *IdentitiesData `json:"identities"`       // 身份信息（自动从 Base64 解码）
	HistoryLoginID HistoryLoginID  `json:"history_login_id"` // 历史登录 ID
	DeviceID       string          `json:"$device_id"`       // 设备 ID
}

// Props 属性信息
type Props struct {
	LatestTrafficSourceType string `json:"$latest_traffic_source_type"` // 最新流量来源类型
	LatestSearchKeyword     string `json:"$latest_search_keyword"`      // 最新搜索关键词
	LatestReferrer          string `json:"$latest_referrer"`            // 最新引荐来源
}

// HistoryLoginID 历史登录 ID
type HistoryLoginID struct {
	Name  string `json:"name"`  // 名称
	Value string `json:"value"` // 值
}

// IdentitiesData 身份信息（identities 字段解码后的数据）
type IdentitiesData struct {
	IdentityCookieID string `json:"$identity_cookie_id"` // Cookie ID
	IdentityLoginID  string `json:"$identity_login_id"`  // 登录 ID
	IdentityH5ID     string `json:"identity_h5_id"`      // H5 ID
}

// UnmarshalJSON 自定义 JSON 解析，自动处理 Base64 编码的字符串
func (i *IdentitiesData) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串（Base64 编码的情况）
	var base64Str string
	if err := json.Unmarshal(data, &base64Str); err == nil && base64Str != "" {
		// Base64 解码
		decoded, err := base64.StdEncoding.DecodeString(base64Str)
		if err != nil {
			return err
		}

		// 解析解码后的 JSON
		type Alias IdentitiesData
		var temp Alias
		if err := json.Unmarshal(decoded, &temp); err != nil {
			return err
		}
		*i = IdentitiesData(temp)
		return nil
	}

	// 如果不是字符串，尝试直接解析为对象
	type Alias IdentitiesData
	var temp Alias
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	*i = IdentitiesData(temp)
	return nil
}

// CookieData 完整的 cookie 数据
type CookieData struct {
	SensorsData SensorsCookie `json:"sensorsdata2015jssdkcross"` // 神策数据
	I18next     string        `json:"i18next"`                   // 语言设置
	Theme       string        `json:"theme"`                     // 主题设置
}

// ParseCookieString 解析 cookie 字符串
// 支持 URL 编码的 cookie 字符串
func ParseCookieString(cookieStr string) (*CookieData, error) {
	// URL 解码
	decodedStr, err := url.QueryUnescape(cookieStr)
	if err != nil {
		return nil, err
	}

	// 分割 cookie 键值对
	cookies := strings.Split(decodedStr, "; ")
	result := &CookieData{}

	for _, cookie := range cookies {
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "sensorsdata2015jssdkcross":
			// 解析神策数据 JSON（identities 字段会自动从 Base64 解码）
			if err := json.Unmarshal([]byte(value), &result.SensorsData); err != nil {
				return nil, err
			}

		case "i18next":
			result.I18next = value

		case "theme":
			result.Theme = value
		}
	}

	return result, nil
}

// GetDistinctID 获取用户唯一标识
func (c *CookieData) GetDistinctID() string {
	return c.SensorsData.DistinctID
}

// GetDeviceID 获取设备 ID
func (c *CookieData) GetDeviceID() string {
	return c.SensorsData.Identities.IdentityH5ID
}

// GetLoginID 获取登录 ID
func (c *CookieData) GetLoginID() string {
	if c.SensorsData.Identities != nil {
		return c.SensorsData.Identities.IdentityLoginID
	}
	return ""
}

// GetLanguage 获取语言设置
func (c *CookieData) GetLanguage() string {
	return c.I18next
}

// GetTheme 获取主题设置
func (c *CookieData) GetTheme() string {
	return c.Theme
}
