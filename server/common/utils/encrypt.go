package utils

import (
	"encoding/base64"
)

// 加密WebSession数据
func EncryptContent(value string) (string, error) {

	// Base64编码
	base64Data := base64.StdEncoding.EncodeToString([]byte(value))
	// 使用简单混淆
	obfuscatedData := simpleObfuscate(base64Data)

	return obfuscatedData, nil
}

// 简单混淆
func simpleObfuscate(data string) string {
	chars := []rune(data)

	// 在第2位插入一个随机字符
	randomChar1 := rune('A' + (len(data) % 26))
	chars = append(chars[:2], append([]rune{randomChar1}, chars[2:]...)...)

	// 在第7位插入一个随机字符
	randomChar2 := rune('a' + (len(data) % 26))
	chars = append(chars[:7], append([]rune{randomChar2}, chars[7:]...)...)

	// 第1位和第5位互换
	chars[0], chars[5] = chars[5], chars[0]

	// 第2位和第9位互换
	chars[1], chars[9] = chars[9], chars[1]

	// 添加$标记
	return "$" + string(chars)
}
