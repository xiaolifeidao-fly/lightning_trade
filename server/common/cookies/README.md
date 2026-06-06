# Cookie 解析器

这是一个用于解析神策数据 cookie 的 Go 语言工具包。

## 功能特性

- ✅ URL 解码 cookie 字符串
- ✅ 解析神策数据 (sensorsdata2015jssdkcross) JSON 结构
- ✅ 自动解码 Base64 编码的身份信息 (identities)
- ✅ 提供便捷的辅助方法获取常用字段
- ✅ 完整的单元测试覆盖

## 数据结构

### CookieData
完整的 cookie 数据结构，包含：
- `SensorsData`: 神策数据
- `I18next`: 语言设置
- `Theme`: 主题设置

### SensorsCookie
神策数据结构，包含：
- `DistinctID`: 用户唯一标识
- `FirstID`: 首次访问 ID
- `Props`: 属性信息（流量来源、搜索关键词等）
- `Identities`: 身份信息结构体（自动从 Base64 解码）
- `HistoryLoginID`: 历史登录 ID
- `DeviceID`: 设备 ID

### IdentitiesData
身份信息结构体（`Identities` 字段类型），包含：
- `IdentityCookieID`: Cookie ID
- `IdentityLoginID`: 登录 ID
- `IdentityH5ID`: H5 ID

**注意**: `identities` 字段在原始 JSON 中是 Base64 编码的字符串，但通过自定义 JSON 解析器会自动解码为结构体。

## 使用方法

### 基本用法

```go
package main

import (
    "fmt"
    "badelay500w/common/cookies"
)

func main() {
    // 原始 URL 编码的 cookie 字符串
    rawCookie := `sensorsdata2015jssdkcross=%7B%22distinct_id%22%3A%229542187%22...`

    // 解析 cookie
    cookieData, err := cookies.ParseCookieString(rawCookie)
    if err != nil {
        panic(err)
    }

    // 使用便捷方法获取数据
    fmt.Println("用户 ID:", cookieData.GetDistinctID())
    fmt.Println("设备 ID:", cookieData.GetDeviceID())
    fmt.Println("登录 ID:", cookieData.GetLoginID())
    fmt.Println("语言:", cookieData.GetLanguage())
    fmt.Println("主题:", cookieData.GetTheme())
}
```

### 访问详细字段

```go
// 神策数据
sensors := cookieData.SensorsData
fmt.Println("首次访问 ID:", sensors.FirstID)
fmt.Println("流量来源:", sensors.Props.LatestTrafficSourceType)
fmt.Println("搜索关键词:", sensors.Props.LatestSearchKeyword)

// 身份信息（自动从 Base64 解码）
if sensors.Identities != nil {
    fmt.Println("Cookie ID:", sensors.Identities.IdentityCookieID)
    fmt.Println("登录 ID:", sensors.Identities.IdentityLoginID)
    fmt.Println("H5 ID:", sensors.Identities.IdentityH5ID)
}

// 历史登录信息
fmt.Println("历史登录名称:", sensors.HistoryLoginID.Name)
fmt.Println("历史登录值:", sensors.HistoryLoginID.Value)
```

## 运行测试

```bash
cd common/cookies
go test -v
```

## 示例输出

解析后的数据结构如下（`identities` 已自动从 Base64 解码）：

```json
{
  "sensorsdata2015jssdkcross": {
    "distinct_id": "9542187",
    "first_id": "19c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2",
    "props": {
      "$latest_traffic_source_type": "直接流量",
      "$latest_search_keyword": "未取到值_直接打开",
      "$latest_referrer": ""
    },
    "identities": {
      "$identity_cookie_id": "19c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2",
      "$identity_login_id": "9542187",
      "identity_h5_id": "h5-32e95a1da1f980d9e7f02d741e42c62b"
    },
    "history_login_id": {
      "name": "$identity_login_id",
      "value": "9542187"
    },
    "$device_id": "19c2135047fd6b-05c1362658ea4c4-1c525631-2073600-19c2135048038f2"
  },
  "i18next": "en",
  "theme": "light"
}
```

## 辅助方法

| 方法 | 说明 | 返回值 |
|------|------|--------|
| `GetDistinctID()` | 获取用户唯一标识 | string |
| `GetDeviceID()` | 获取设备 ID | string |
| `GetLoginID()` | 获取登录 ID | string |
| `GetLanguage()` | 获取语言设置 | string |
| `GetTheme()` | 获取主题设置 | string |

## 技术特性

### 自动 Base64 解码
`identities` 字段在原始 cookie 中是 Base64 编码的 JSON 字符串：
```
eyIkaWRlbnRpdHlfY29va2llX2lkIjoiMTljMjEzNTA0N2ZkNmItMDVjMTM2MjY1OGVhNGM0LTFjNTI1NjMxLTIwNzM2MDAtMTljMjEzNTA0ODAzOGYyIiwiJGlkZW50aXR5X2xvZ2luX2lkIjoiOTU0MjE4NyIsImlkZW50aXR5X2g1X2lkIjoiaDUtMzJlOTVhMWRhMWY5ODBkOWU3ZjAyZDc0MWU0MmM2MmIifQ==
```

解析时会自动：
1. 识别 Base64 编码的字符串
2. 进行 Base64 解码
3. 将解码后的 JSON 解析为 `IdentitiesData` 结构体

这样你就可以直接通过 `cookieData.SensorsData.Identities` 访问结构化的身份数据，无需手动解码。

## 注意事项

1. **URL 编码**: 函数会自动处理 URL 编码的 cookie 字符串
2. **Base64 自动解码**: `identities` 字段通过自定义 JSON 解析器自动从 Base64 解码为结构体
3. **错误处理**: 如果解析失败，函数会返回错误信息
4. **空值处理**: 如果某些字段为空，会返回空字符串或 nil

## 依赖

- Go 1.16+
- 标准库：`encoding/base64`, `encoding/json`, `net/url`, `strings`
