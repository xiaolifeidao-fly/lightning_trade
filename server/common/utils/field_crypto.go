package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

const encryptedFieldPrefix = "enc:"

func EncryptField(value, key, scope string) (string, error) {
	plainText := strings.TrimSpace(value)
	if plainText == "" || IsEncryptedField(plainText) {
		return plainText, nil
	}
	block, err := aes.NewCipher(normalizeFieldKey(key))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := deterministicNonce(key, scope, plainText, aead.NonceSize())
	cipherText := aead.Seal(nil, nonce, []byte(plainText), []byte(scope))
	payload := append(append([]byte{}, nonce...), cipherText...)
	return encryptedFieldPrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func DecryptField(value, key, scope string) (string, error) {
	cipherText := strings.TrimSpace(value)
	if cipherText == "" || !IsEncryptedField(cipherText) {
		return cipherText, nil
	}
	block, err := aes.NewCipher(normalizeFieldKey(key))
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(cipherText, encryptedFieldPrefix))
	if err != nil {
		return "", err
	}
	if len(payload) <= aead.NonceSize() {
		return "", errors.New("encrypted field payload is invalid")
	}
	nonce := payload[:aead.NonceSize()]
	encryptedPayload := payload[aead.NonceSize():]
	plainText, err := aead.Open(nil, nonce, encryptedPayload, []byte(scope))
	if err != nil {
		return "", err
	}
	return string(plainText), nil
}

func IsEncryptedField(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), encryptedFieldPrefix)
}

func DigestField(value, key, scope string) string {
	plainText := strings.TrimSpace(value)
	if plainText == "" {
		return ""
	}
	mac := hmac.New(sha256.New, normalizeFieldKey(key))
	mac.Write([]byte(scope))
	mac.Write([]byte{0})
	mac.Write([]byte(plainText))
	return hex.EncodeToString(mac.Sum(nil))
}

func normalizeFieldKey(key string) []byte {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return sum[:]
}

func deterministicNonce(key, scope, plainText string, size int) []byte {
	mac := hmac.New(sha256.New, normalizeFieldKey(key))
	mac.Write([]byte(scope))
	mac.Write([]byte{0})
	mac.Write([]byte(plainText))
	return mac.Sum(nil)[:size]
}
