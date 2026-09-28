package service

import (
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func hasMeetingBusinessSignal(state domain.PromptQuotationState) bool {
	for _, item := range state.Scope {
		if strings.TrimSpace(item.Value) != "" {
			return true
		}
	}
	for _, line := range state.Team {
		if strings.TrimSpace(line.RequestedRole) != "" {
			return true
		}
	}
	if state.DurationMonths != nil && state.DurationMonths.Value > 0 {
		return true
	}
	if state.CommercialScheme != nil && strings.TrimSpace(state.CommercialScheme.Value) != "" {
		return true
	}
	return state.PaymentPattern != nil && strings.TrimSpace(state.PaymentPattern.Value) != ""
}
