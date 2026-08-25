package repository

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql/driver"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

const encryptedValuePrefix = "enc:v1:"

// EncryptedString encrypts plaintext before it is persisted by GORM. The
// stored value is intentionally kept opaque; callers must use Decrypt to read it.
type EncryptedString string

func NewEncryptedString(plaintext string) EncryptedString {
	return EncryptedString(plaintext)
}

func (value EncryptedString) IsEmpty() bool {
	return value == ""
}

func (value EncryptedString) Decrypt() (string, error) {
	if value == "" {
		return "", nil
	}
	if !strings.HasPrefix(string(value), encryptedValuePrefix) {
		return "", fmt.Errorf("encrypted value has an unexpected format")
	}

	ciphertext, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(string(value), encryptedValuePrefix))
	if err != nil {
		return "", fmt.Errorf("decode encrypted value: %w", err)
	}

	block, err := aes.NewCipher(argusEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("create AES-GCM: %w", err)
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", fmt.Errorf("encrypted value is shorter than the AES-GCM nonce")
	}

	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt encrypted value: %w", err)
	}
	return string(plaintext), nil
}

func (value EncryptedString) Value() (driver.Value, error) {
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(string(value), encryptedValuePrefix) {
		return string(value), nil
	}

	block, err := aes.NewCipher(argusEncryptionKey())
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate AES-GCM nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	return encryptedValuePrefix + base64.RawURLEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (value *EncryptedString) Scan(source any) error {
	if source == nil {
		*value = ""
		return nil
	}
	switch source := source.(type) {
	case string:
		*value = EncryptedString(source)
	case []byte:
		*value = EncryptedString(string(source))
	default:
		return fmt.Errorf("unsupported encrypted value type %T", source)
	}
	return nil
}

func argusEncryptionKey() []byte {
	encodedKey := strings.TrimSpace(os.Getenv("ARGUS_CONFIG_ENCRYPTION_KEY"))
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil
	}
	return key
}
