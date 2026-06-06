package utils

import (
	"fmt"
	"strconv"
	"time"
)

func InterfaceToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

// FormatTimeWithMillisecond 格式化时间为字符串，包含毫秒
// 返回格式: 2006-01-02 15:04:05.000
func FormatTimeWithMillisecond(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000")
}

// GetCurrentTimeString 获取当前时间的格式化字符串，包含毫秒
// 返回格式: 2006-01-02 15:04:05.000
func GetCurrentTimeString() string {
	return FormatTimeWithMillisecond(time.Now())
}
