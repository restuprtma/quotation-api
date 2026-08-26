package service_test

import (
	"testing"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestService_TestSettingsConnection_APIKeyFallback(t *testing.T) {
	t.Skip("Requires DB mock or testcontainer to test API key fallback logic")

	_ = service.NewService(nil, nil, nil, nil, nil, nil, nil, nil)
	_ = &dto.CredentialRequest{
		ProviderType: "gemini",
		APIKey:       "",
		Model:        "gemini-1.5-flash",
	}

	// s.TestSettingsConnection(context.Background(), "company-123", req)
}
