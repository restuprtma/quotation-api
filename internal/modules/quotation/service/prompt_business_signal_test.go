package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestHasMeetingBusinessSignal(t *testing.T) {
	tests := []struct {
		name  string
		state domain.PromptQuotationState
		want  bool
	}{
		{"scope", domain.PromptQuotationState{Scope: []domain.Fact[string]{{Value: "Inventory"}}}, true},
		{"team", domain.PromptQuotationState{Team: []domain.PromptTeamLine{{RequestedRole: "backend_programmer"}}}, true},
		{"duration", domain.PromptQuotationState{DurationMonths: &domain.Fact[int]{Value: 3}}, true},
		{"scheme", domain.PromptQuotationState{CommercialScheme: &domain.Fact[string]{Value: domain.CommercialSchemeDedicatedSquad}}, true},
		{"payment", domain.PromptQuotationState{PaymentPattern: &domain.Fact[string]{Value: "50:50"}}, true},
		{"identity only", domain.PromptQuotationState{ProjectName: &domain.Fact[string]{Value: "Portal"}}, false},
		{"blank", domain.PromptQuotationState{Scope: []domain.Fact[string]{{Value: " "}}}, false},
		{"empty", domain.PromptQuotationState{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasMeetingBusinessSignal(tt.state))
		})
	}
}
