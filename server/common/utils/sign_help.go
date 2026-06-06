package utils

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// Eh 实现字符串哈希算法，对应 JavaScript 中的 eh 函数
// 使用公式: hash = (hash << 5) - hash + charCode，等价于 hash = hash * 31 + charCode
func GetExt(e string) int32 {
	// 如果是空字符串，返回 0
	if len(e) == 0 {
		return 0
	}

	var i int32 = 0
	// 遍历字符串的每个字符
	for _, char := range e {
		// JavaScript 的 charCodeAt 返回 UTF-16 编码单元
		// Go 的 rune 是 Unicode 码点，对于基本字符是相同的
		i = (i << 5) - i + int32(char)
		// JavaScript 中的 i &= i 是为了确保整数运算
		// Go 中 int32 已经保证是 32 位整数
	}

	return i
}

func ConvertToMessage(data interface{}) (string, error) {
	// 1. 将结构体转换为map
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("序列化数据失败: %w", err)
	}

	var dataMap map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &dataMap); err != nil {
		return "", fmt.Errorf("解析数据失败: %w", err)
	}

	// 2. 移除sign字段（如果存在）
	delete(dataMap, "sign")

	// 3. 提取所有键并排序
	keys := make([]string, 0, len(dataMap))
	for k := range dataMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 4. 按照 key=value 的格式拼接
	var builder strings.Builder
	for i, k := range keys {
		v := dataMap[k]

		// 跳过空值
		if v == nil || isZeroValue(v) {
			continue
		}

		if i > 0 {
			builder.WriteString("&")
		}

		builder.WriteString(k)
		builder.WriteString("=")
		builder.WriteString(formatValue(v))
	}
	message := builder.String()
	return message, nil
}

// CalculateSign 计算订单请求的签名
// data: 请求数据结构体
// 返回MD5签名（32位小写十六进制字符串）
func CalculateSign(message string) (string, error) {
	key := "61dd6c49529a05569900e71f49a0cd87"
	hmacThenMd5 := HmacSHA256ThenMD5String(key, message)
	return hmacThenMd5, nil
}

// formatValue 将所有类型的值转换为字符串
func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int8:
		return fmt.Sprintf("%d", val)
	case int16:
		return fmt.Sprintf("%d", val)
	case int32:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case uint:
		return fmt.Sprintf("%d", val)
	case uint8:
		return fmt.Sprintf("%d", val)
	case uint16:
		return fmt.Sprintf("%d", val)
	case uint32:
		return fmt.Sprintf("%d", val)
	case uint64:
		return fmt.Sprintf("%d", val)
	case float64:
		// 使用 'f' 格式避免科学计数法，-1 保留必要的小数位
		return strconv.FormatFloat(val, 'f', -1, 64)
	case float32:
		// 使用 'f' 格式避免科学计数法，-1 保留必要的小数位
		return strconv.FormatFloat(float64(val), 'f', -1, 32)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// isZeroValue 判断值是否为零值
func isZeroValue(v interface{}) bool {
	if v == nil {
		return true
	}

	val := reflect.ValueOf(v)
	switch val.Kind() {
	case reflect.String:
		return val.String() == ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return val.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return val.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return val.Float() == 0
	case reflect.Bool:
		return !val.Bool()
	case reflect.Slice, reflect.Map, reflect.Array:
		return val.Len() == 0
	default:
		return false
	}
}

// CalculateHMAC 生成32位随机十六进制字符串（用于请求头的hmac字段）
// data: 请求体的JSON字符串（此参数当前未使用，保留用于未来扩展）
// 返回32位小写十六进制字符串
func CalculateHMAC(data string) string {
	// 生成16字节随机数据，转为32位十六进制字符串
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		// 如果随机数生成失败，返回固定的后备值
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b)
}
