package repository_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestClientOrganization_Normalization(t *testing.T) {
	tests := []struct {
		name               string
		raw                string
		expectedDisplay    string
		expectedNormalized string
	}{
		{
			name:               "simple name",
			raw:                "PT Venturo",
			expectedDisplay:    "PT Venturo",
			expectedNormalized: "pt venturo",
		},
		{
			name:               "excessive whitespace and tabs",
			raw:                "   PT   \t  Venturo   Indonesia  \n ",
			expectedDisplay:    "PT Venturo Indonesia",
			expectedNormalized: "pt venturo indonesia",
		},
		{
			name:               "mixed case",
			raw:                "Acme Corp GLOBAL",
			expectedDisplay:    "Acme Corp GLOBAL",
			expectedNormalized: "acme corp global",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			display, normalized := domain.NormalizeClientOrganizationName(tt.raw)
			assert.Equal(t, tt.expectedDisplay, display)
			assert.Equal(t, tt.expectedNormalized, normalized)
		})
	}
}

func TestClientOrganization_Errors(t *testing.T) {
	assert.Equal(t, "client organization already exists", repository.ErrClientOrganizationDuplicate.Error())
	assert.Equal(t, "client organization not found", repository.ErrClientOrganizationNotFound.Error())
}
