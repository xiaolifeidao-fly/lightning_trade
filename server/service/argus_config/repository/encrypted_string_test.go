package repository

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestEncryptedStringValueDoesNotPersistPlaintext(t *testing.T) {
	t.Setenv("ARGUS_CONFIG_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")))

	secret := "super-secret-token"
	stored, err := NewEncryptedString(secret).Value()
	if err != nil {
		t.Fatalf("encrypt value: %v", err)
	}
	storedText := stored.(string)
	if !strings.HasPrefix(storedText, encryptedValuePrefix) {
		t.Fatalf("stored value does not have encrypted prefix: %q", storedText)
	}
	if strings.Contains(storedText, secret) {
		t.Fatalf("stored value contains plaintext secret")
	}

	var loaded EncryptedString
	if err := loaded.Scan(storedText); err != nil {
		t.Fatalf("scan encrypted value: %v", err)
	}
	plaintext, err := loaded.Decrypt()
	if err != nil {
		t.Fatalf("decrypt value: %v", err)
	}
	if plaintext != secret {
		t.Fatalf("plaintext = %q, want %q", plaintext, secret)
	}
}

func TestEncryptedStringRejectsMissingOrInvalidKey(t *testing.T) {
	t.Setenv("ARGUS_CONFIG_ENCRYPTION_KEY", "")
	if _, err := NewEncryptedString("secret").Value(); err == nil {
		t.Fatal("expected encryption to fail without ARGUS_CONFIG_ENCRYPTION_KEY")
	}
}
