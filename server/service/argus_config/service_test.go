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

func TestSensitiveFieldsAreMaskedAndNeverSerialized(t *testing.T) {
	account := accountDTO(&repository.ArgusAccount{AccountName: "primary", Username: repository.NewEncryptedString("enc:v1:opaque"), APIKey: repository.NewEncryptedString("enc:v1:key")})
	encoded, err := json.Marshal(account)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "enc:v1") || strings.Contains(string(encoded), "opaque") {
		t.Fatalf("serialized DTO leaked encrypted material: %s", encoded)
	}
	if account.Username != "******" || account.APIKey != "******" {
		t.Fatalf("secrets were not masked: %+v", account)
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
