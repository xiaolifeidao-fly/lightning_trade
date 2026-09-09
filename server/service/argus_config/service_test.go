package argus_config

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	argusDTO "service/argus_config/dto"
	"service/argus_config/repository"
)

func validRequest() *argusDTO.SaveConfigRequest {
	return &argusDTO.SaveConfigRequest{
		InstanceKey:    "argus-single-1",
		Config:         argusDTO.ConfigDTO{ServerPort: 8855, MonitorIntervalSecond: 5},
		Accounts:       []argusDTO.AccountDTO{{AccountName: "primary", URL: "https://example.test"}},
		AccountRisks:   []argusDTO.AccountRiskDTO{{AccountID: 0}},
		MonitorSymbols: []argusDTO.MonitorSymbolDTO{{Symbol: "btc", TradeInstrument: "BTCUSDT"}},
	}
}

func TestValidateRejectsIncompleteRelease(t *testing.T) {
	request := validRequest()
	request.Accounts = nil
	if err := (&ArgusConfigService{}).Validate(request); err == nil || !strings.Contains(err.Error(), "account") {
		t.Fatalf("expected missing account validation error, got %v", err)
	}
}

func TestGetPublishedHonorsCanceledRequestContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := (&ArgusConfigService{}).GetPublished(ctx, "argus-single-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context error, got %v", err)
	}
}

// TestSensitiveFieldsAreMaskedAndNeverSerialized 守的是取消落库加密之后最要紧的
// 一条不变量：库里现在是明文，接口就绝不能把真值序列化出去。加密时代这个测试
// 只需证明密文不外泄，现在它必须证明**明文**不外泄——责任比以前更重。
func TestSensitiveFieldsAreMaskedAndNeverSerialized(t *testing.T) {
	const (
		plainUsername = "trader@example.com"
		plainAPIKey   = "ak-live-must-never-appear"
	)
	account := accountDTO(&repository.ArgusAccount{AccountName: "primary", Username: plainUsername, APIKey: plainAPIKey})
	encoded, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), plainUsername) || strings.Contains(string(encoded), plainAPIKey) {
		t.Fatalf("serialized DTO leaked plaintext credentials: %s", encoded)
	}
	if account.Username != "******" || account.APIKey != "******" {
		t.Fatalf("secrets were not masked: %+v", account)
	}
}

// TestPreserveSecretKeepsStoredValue 守另一条不变量：前端回填时敏感字段是
// ****** 或空，表示「此项未改」，必须沿用库里旧值。丢了它，一次保存就会把
// 所有凭证写成字面量 ******，实盘直接登不上。
func TestPreserveSecretKeepsStoredValue(t *testing.T) {
	const stored = "ak-live-stored"
	for _, incoming := range []string{"", "   ", "******"} {
		if got := preserveSecret(incoming, stored); got != stored {
			t.Fatalf("preserveSecret(%q, %q) = %q, 期望沿用旧值 %q", incoming, stored, got, stored)
		}
	}
	if got := preserveSecret("ak-live-new", stored); got != "ak-live-new" {
		t.Fatalf("显式传新值时应覆盖，得到 %q", got)
	}
	if got := preserveSecret("", ""); got != "" {
		t.Fatalf("旧值也为空时应返回空，得到 %q", got)
	}
}

func TestNormalizeInstanceKeyRejectsUnsafeKeys(t *testing.T) {
	for _, key := range []string{"", "   ", "argus:single", "argus single", strings.Repeat("a", 65)} {
		if _, err := NormalizeInstanceKey(key); err == nil {
			t.Fatalf("NormalizeInstanceKey(%q) accepted an unsafe instance key", key)
		}
	}
	for _, key := range []string{"argus-single-1", " argus-single-roc ", "argus_single.ives"} {
		if _, err := NormalizeInstanceKey(key); err != nil {
			t.Fatalf("NormalizeInstanceKey(%q) = %v, want accepted", key, err)
		}
	}
}

func TestInstanceScopedCallsRejectMissingInstanceKey(t *testing.T) {
	service := NewArgusConfigServiceWithRepository(&repository.ArgusConfigRepository{})
	if _, err := service.ResolveInstanceKey(""); err == nil {
		t.Fatal("ResolveInstanceKey without a configured default unexpectedly succeeded")
	}
	if _, err := service.ResolveInstanceKey("argus:single"); !errors.Is(err, ErrInstanceKeyInvalid) {
		t.Fatalf("ResolveInstanceKey with an unsafe key = %v, want ErrInstanceKeyInvalid", err)
	}
	if _, err := service.SaveDraft("", nil, "tester"); err == nil {
		t.Fatal("SaveDraft with a nil request unexpectedly succeeded")
	}
}

func TestVersionDTOCarriesInstanceKey(t *testing.T) {
	result := versionDTO(&repository.ArgusConfigVersion{InstanceKey: "argus-single-ives", Version: 4})
	if result.InstanceKey != "argus-single-ives" || result.Version != 4 {
		t.Fatalf("versionDTO = %+v, want instanceKey/version to be carried through", result)
	}
}
