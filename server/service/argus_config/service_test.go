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

	_, err := (&ArgusConfigService{}).GetPublished(ctx)
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
