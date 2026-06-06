package utils

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
)

// HmacSHA256 计算 HMAC-SHA256，返回字节数组
// key: 密钥
// message: 要签名的消息
func HmacSHA256(key []byte, message []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(message)
	return h.Sum(nil)
}

// HmacSHA256Hex 计算 HMAC-SHA256，返回十六进制字符串（小写）
func HmacSHA256Hex(key []byte, message []byte) string {
	hash := HmacSHA256(key, message)
	return hex.EncodeToString(hash)
}

// HmacSHA256HexUpper 计算 HMAC-SHA256，返回十六进制字符串（大写）
func HmacSHA256HexUpper(key []byte, message []byte) string {
	hash := HmacSHA256(key, message)
	return hex.EncodeToString(hash)
}

// HmacSHA256Base64 计算 HMAC-SHA256，返回 Base64 编码字符串
func HmacSHA256Base64(key []byte, message []byte) string {
	hash := HmacSHA256(key, message)
	return base64.StdEncoding.EncodeToString(hash)
}

// HmacSHA256Base64URL 计算 HMAC-SHA256，返回 Base64 URL 安全编码字符串
func HmacSHA256Base64URL(key []byte, message []byte) string {
	hash := HmacSHA256(key, message)
	return base64.URLEncoding.EncodeToString(hash)
}

// HmacSHA256String 便捷方法：使用字符串参数计算 HMAC-SHA256，返回十六进制字符串
func HmacSHA256String(key string, message string) string {
	return HmacSHA256Hex([]byte(key), []byte(message))
}

// VerifyHmacSHA256 验证 HMAC-SHA256 签名是否正确
// key: 密钥
// message: 消息
// signature: 要验证的签名（字节数组）
func VerifyHmacSHA256(key []byte, message []byte, signature []byte) bool {
	expectedMAC := HmacSHA256(key, message)
	return hmac.Equal(signature, expectedMAC)
}

// VerifyHmacSHA256Hex 验证 HMAC-SHA256 十六进制签名是否正确
func VerifyHmacSHA256Hex(key []byte, message []byte, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return VerifyHmacSHA256(key, message, signature)
}

// VerifyHmacSHA256Base64 验证 HMAC-SHA256 Base64 签名是否正确
func VerifyHmacSHA256Base64(key []byte, message []byte, signatureBase64 string) bool {
	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false
	}
	return VerifyHmacSHA256(key, message, signature)
}

// HmacSHA256ThenMD5 先计算 HMAC-SHA256（十六进制），再对结果进行 MD5，返回小写十六进制字符串
// key: 密钥
// message: 要签名的消息
func HmacSHA256ThenMD5(key []byte, message []byte) string {
	// 第一步：计算 HMAC-SHA256 并转为十六进制字符串
	hmacHex := HmacSHA256Hex(key, message)

	// 第二步：对十六进制字符串进行 MD5
	md5Hash := md5.Sum([]byte(hmacHex))

	// 返回小写十六进制字符串
	return hex.EncodeToString(md5Hash[:])
}

// HmacSHA256ThenMD5String 便捷方法：使用字符串参数，先计算 HMAC-SHA256 再 MD5
func HmacSHA256ThenMD5String(key string, message string) string {
	return HmacSHA256ThenMD5([]byte(key), []byte(message))
}

// GenerateRandomString 生成指定长度的随机字符串
// 字符集不包含容易混淆的字符（如 I, L, O, 0, 1, 9 等）
// length: 字符串长度，如果 <= 0 则默认为 32
func GenerateRandomString(length int) string {
	// 默认长度为 32
	if length <= 0 {
		length = 32
	}

	// 字符集：不包含容易混淆的字符
	charset := "ABCDEFGHJKMNPQRSTWXYZabcdefhijkmnprstwxyz2345678"
	charsetLen := big.NewInt(int64(len(charset)))

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		// 使用 crypto/rand 生成安全的随机数
		randomIndex, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			// 如果发生错误，使用第一个字符作为后备
			result[i] = charset[0]
			continue
		}
		result[i] = charset[randomIndex.Int64()]
	}

	return string(result)
}
