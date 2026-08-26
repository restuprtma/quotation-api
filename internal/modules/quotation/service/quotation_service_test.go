package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestService_TestSettingsConnection_NoCompany(t *testing.T) {
	s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil)

	req := &dto.CredentialRequest{
		ProviderType: "gemini",
		APIKey:       "",
		Model:        "gemini-1.5-flash",
	}

	err := s.TestSettingsConnection(context.Background(), "", req)
	assert.Equal(t, service.ErrNoCompanyContext, err)
}

func TestService_TestSettingsConnection_ErrorMasking(t *testing.T) {
	aiFactory := service.NewProviderFactory()
	s := service.NewService(nil, nil, nil, nil, aiFactory, nil, nil, nil)

	req := &dto.CredentialRequest{
		ProviderType: "invalid-provider",
		APIKey:       "dummy-key",
		Model:        "dummy-model",
	}

	err := s.TestSettingsConnection(context.Background(), "company-123", req)

	// Ensure error is masked
	assert.Equal(t, errors.New(service.ErrConnectionFailedMsg), err)
}
