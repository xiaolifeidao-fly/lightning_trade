package utils

import (
	"encoding/base64"
	"errors"
)

// 解密WebSession数据
func DecryptContent(encryptData string) (string, error) {
	// 使用简单解混淆
	deobfuscatedData, err := simpleDeobfuscate(encryptData)
	if err != nil {
		return "", err
	}

	// Base64解码
	plaintext, err := base64.StdEncoding.DecodeString(deobfuscatedData)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// 简单解混淆
func simpleDeobfuscate(obfuscatedData string) (string, error) {
	if len(obfuscatedData) < 12 { // 至少需要: $标记 + 10个字符 + 2个随机字符
		return "", errors.New("数据格式错误: 长度不足")
	}

	// 检查是否有$标记
	if obfuscatedData[0] != '$' {
		return "", errors.New("数据格式错误: 缺少标记字符")
	}

	// 移除$标记
	chars := []rune(obfuscatedData[1:])

	// 第2位和第9位互换回来 (在原始数据中是第1位和第8位)
	chars[1], chars[9] = chars[9], chars[1]

	// 第1位和第5位互换回来 (在原始数据中是第0位和第4位)
	chars[0], chars[5] = chars[5], chars[0]

	// 移除第7位的随机字符 (在原始数据中是第6位)
	chars = append(chars[:7], chars[8:]...)

	// 移除第2位的随机字符 (在原始数据中是第2位)
	chars = append(chars[:2], chars[3:]...)

	return string(chars), nil
}
