package trade

import (
	"testing"
	"time"
)

func TestGenerateGoogleAuthenticatorCode(t *testing.T) {
	t.Parallel()

	// RFC 6238 SHA1 测试向量对应的 base32 secret。
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

	cases := []struct {
		name     string
		unixTime int64
		want     string
	}{
		{name: "t59", unixTime: 59, want: "287082"},
		{name: "t1111111109", unixTime: 1111111109, want: "081804"},
		{name: "t2000000000", unixTime: 2000000000, want: "279037"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := GenerateGoogleAuthenticatorCode(secret, time.Unix(tc.unixTime, 0))
			if err != nil {
				t.Fatalf("GenerateGoogleAuthenticatorCode returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("code mismatch: got %s want %s", got, tc.want)
			}
		})
	}
}

func TestGenerateGoogleAuthenticatorCodeInvalidSecret(t *testing.T) {
	t.Parallel()

	if _, err := GenerateGoogleAuthenticatorCode("%%%invalid%%%", time.Unix(59, 0)); err == nil {
		t.Fatalf("expected error for invalid secret")
	}
}
