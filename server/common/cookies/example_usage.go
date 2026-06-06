package cookies

import "fmt"

// ExampleUsage 展示如何使用 cookie 解析器
func ExampleUsage() {
	// 原始 URL 编码的 cookie 字符串
	rawCookie := `sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229542187%22%2C%22first_id%22%3A%2219c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2%22%2C%22props%22%3A%7B%22%24latest_traffic_source_type%22%3A%22%E7%9B%B4%E6%8E%A5%E6%B5%81%E9%87%8F%22%2C%22%24latest_search_keyword%22%3A%22%E6%9C%AA%E5%8F%96%E5%88%B0%E5%80%BC_%E7%9B%B4%E6%8E%A5%E6%89%93%E5%BC%80%22%2C%22%24latest_referrer%22%3A%22%22%7D%2C%22identities%22%3A%22eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTljMjEzNTA0N2ZkNmItMDVjMTM2MjY1OGVhNGM0LTFjNTI1NjMxLTIwNzM2MDAtMTljMjEzNTA0ODAzOGYyIiwiJGlkZW50aXR5X2xvZ2luX2lkIjoiOTU0MjE4NyIsImlkZW50aXR5X2g1X2lkIjoiaDUtMzJlOTVhMWRhMWY5ODBkOWU3ZjAyZDc0MWU0MmM2MmIifQ%3D%3D%22%2C%22history_login_id%22%3A%7B%22name%22%3A%22%24identity_login_id%22%2C%22value%22%3A%229542187%22%7D%2C%22%24device_id%22%3A%2219c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2%22%7D; i18next=en; theme=light`

	// 解析 cookie
	cookieData, err := ParseCookieString(rawCookie)
	if err != nil {
		fmt.Printf("解析失败: %v\n", err)
		return
	}

	// 使用解析后的数据
	fmt.Println("=== Cookie 解析结果 ===")
	fmt.Printf("用户 ID: %s\n", cookieData.GetDistinctID())
	fmt.Printf("设备 ID: %s\n", cookieData.GetDeviceID())
	fmt.Printf("登录 ID: %s\n", cookieData.GetLoginID())
	fmt.Printf("语言: %s\n", cookieData.GetLanguage())
	fmt.Printf("主题: %s\n", cookieData.GetTheme())

	fmt.Println("\n=== 神策数据详情 ===")
	fmt.Printf("首次访问 ID: %s\n", cookieData.SensorsData.FirstID)
	fmt.Printf("流量来源: %s\n", cookieData.SensorsData.Props.LatestTrafficSourceType)
	fmt.Printf("搜索关键词: %s\n", cookieData.SensorsData.Props.LatestSearchKeyword)
	fmt.Printf("引荐来源: %s\n", cookieData.SensorsData.Props.LatestReferrer)

	if cookieData.SensorsData.Identities != nil {
		fmt.Println("\n=== 身份信息（Base64 自动解码）===")
		fmt.Printf("Cookie ID: %s\n", cookieData.SensorsData.Identities.IdentityCookieID)
		fmt.Printf("登录 ID: %s\n", cookieData.SensorsData.Identities.IdentityLoginID)
		fmt.Printf("H5 ID: %s\n", cookieData.SensorsData.Identities.IdentityH5ID)
	}

	fmt.Println("\n=== 历史登录信息 ===")
	fmt.Printf("名称: %s\n", cookieData.SensorsData.HistoryLoginID.Name)
	fmt.Printf("值: %s\n", cookieData.SensorsData.HistoryLoginID.Value)
}
