package cookies

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestParseCookieString(t *testing.T) {
	// 原始 URL 编码的 cookie 字符串
	rawCookie := `sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229542187%22%2C%22first_id%22%3A%2219c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E7%9B%B4%E6%8E%A5%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC_%E7%9B%B4%E6%8E%A5%E6%89%93%E5%BC%80%22%2C%22%24latest_referrer%22%3A%22%22%7D%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTljMjEzNTA0N2ZkNmItMDVjMTM2MjY1OGVhNGM0LTFjNTI1NjMxLTIwNzM2MDAtMTljMjEzNTA0ODAzOGYyIiwiJGlkZW50aXR5X2xvZ2luX2lkIjoiOTU0MjE4NyIsImlkZW50aXR5X2g1X2lkIjoiaDUtMzJlOTVhMWRhMWY5ODBkOWU3ZjAyZDc0MWU0MmM2MmIifQ%3D%3D%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%229542187%22%7D%2C%22%24device_id%22%3A%2219c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2%22%7D; i18next=en; theme=light`

	// 解析 cookie
	cookieData, err := ParseCookieString(rawCookie)
	if err != nil {
		t.Fatalf("解析 cookie 失败: %v", err)
	}

	// 验证神策数据
	if cookieData.SensorsData.DistinctID != "9542187" {
		t.Errorf("distinct_id 不匹配，期望: 9542187, 实际: %s", cookieData.SensorsData.DistinctID)
	}

	if cookieData.SensorsData.FirstID != "19c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2" {
		t.Errorf("first_id 不匹配")
	}

	if cookieData.SensorsData.DeviceID != "19c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2" {
		t.Errorf("device_id 不匹配")
	}

	// 验证属性
	if cookieData.SensorsData.Props.LatestTrafficSourceType != "直接流量" {
		t.Errorf("latest_traffic_source_type 不匹配，期望: 直接流量, 实际: %s", cookieData.SensorsData.Props.LatestTrafficSourceType)
	}

	if cookieData.SensorsData.Props.LatestSearchKeyword != "未取到值_直接打开" {
		t.Errorf("latest_search_keyword 不匹配")
	}

	// 验证历史登录 ID
	if cookieData.SensorsData.HistoryLoginID.Name != "$identity_login_id" {
		t.Errorf("history_login_id.name 不匹配")
	}

	if cookieData.SensorsData.HistoryLoginID.Value != "9542187" {
		t.Errorf("history_login_id.value 不匹配")
	}

	// 验证身份信息（Base64 自动解码后的数据）
	if cookieData.SensorsData.Identities == nil {
		t.Fatal("identities 数据解析失败")
	}

	if cookieData.SensorsData.Identities.IdentityLoginID != "9542187" {
		t.Errorf("identity_login_id 不匹配")
	}

	if cookieData.SensorsData.Identities.IdentityCookieID != "19c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2" {
		t.Errorf("identity_cookie_id 不匹配")
	}

	if cookieData.SensorsData.Identities.IdentityH5ID != "h5-32e95a1da1f980d9e7f02d741e42c62b" {
		t.Errorf("identity_h5_id 不匹配")
	}

	// 验证其他 cookie
	if cookieData.I18next != "en" {
		t.Errorf("i18next 不匹配，期望: en, 实际: %s", cookieData.I18next)
	}

	if cookieData.Theme != "light" {
		t.Errorf("theme 不匹配，期望: light, 实际: %s", cookieData.Theme)
	}

	// 测试辅助方法
	if cookieData.GetDistinctID() != "9542187" {
		t.Errorf("GetDistinctID() 不匹配")
	}

	if cookieData.GetLoginID() != "9542187" {
		t.Errorf("GetLoginID() 不匹配")
	}

	if cookieData.GetLanguage() != "en" {
		t.Errorf("GetLanguage() 不匹配")
	}

	if cookieData.GetTheme() != "light" {
		t.Errorf("GetTheme() 不匹配")
	}

	// 打印 JSON 格式的结果（用于调试）
	jsonBytes, _ := json.MarshalIndent(cookieData, "", "  ")
	fmt.Println("解析结果:\n", string(jsonBytes))
}

func TestParseCookieString_InvalidCookie(t *testing.T) {
	// 测试无效的 cookie
	_, err := ParseCookieString("invalid_cookie_string")
	if err != nil {
		t.Logf("预期的错误: %v", err)
	}
}

func TestParseCookieString_EmptyCookie(t *testing.T) {
	// 测试空 cookie
	cookieData, err := ParseCookieString("")
	if err != nil {
		t.Fatalf("解析空 cookie 失败: %v", err)
	}

	if cookieData.SensorsData.DistinctID != "" {
		t.Errorf("空 cookie 应该返回空的结构体")
	}
}
