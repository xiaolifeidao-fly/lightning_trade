package trade

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

const (
	defaultTOTPPeriod = 30
	defaultTOTPDigits = 6
)

// GenerateGoogleAuthenticatorCode 根据 Google Authenticator 兼容的 TOTP 算法生成 6 位动态码。
func GenerateGoogleAuthenticatorCode(secret string, at time.Time) (string, error) {
	return generateTOTP(secret, at, defaultTOTPPeriod, defaultTOTPDigits)
}

func generateTOTP(secret string, at time.Time, periodSeconds, digits int) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", fmt.Errorf("google auth secret 不能为空")
	}
	if periodSeconds <= 0 {
		return "", fmt.Errorf("periodSeconds 必须大于 0")
	}
	if digits <= 0 {
		return "", fmt.Errorf("digits 必须大于 0")
	}

	normalized := normalizeBase32Secret(secret)
	decoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := decoder.DecodeString(normalized)
	if err != nil {
		return "", fmt.Errorf("解析 google auth secret 失败: %w", err)
	}

	counter := uint64(at.Unix() / int64(periodSeconds))
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)

	mac := hmac.New(sha1.New, key)
	if _, err := mac.Write(msg[:]); err != nil {
		return "", fmt.Errorf("计算 hmac 失败: %w", err)
	}
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	code := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)

	mod := 1
	for i := 0; i < digits; i++ {
		mod *= 10
	}

	value := code % mod
	return fmt.Sprintf("%0*d", digits, value), nil
}

func normalizeBase32Secret(secret string) string {
	normalized := strings.ToUpper(secret)
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "-", "")
	return normalized
}
