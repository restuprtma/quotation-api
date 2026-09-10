package service

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func validTalentOutsourcingGenerateRequest() dto.GenerateQuotationRequest {
	in := talentBPJSInput()
	in.DocumentNumber = "LANDA/BPJS/0626/001"
	return dto.GenerateQuotationRequest{Subject: "Penawaran Talent", CommercialScheme: domain.CommercialSchemeTalentOutsourcing, TalentOutsourcing: &in}
}

func TestValidateGenerateInputTalentOutsourcingExactUnion(t *testing.T) {
	req := validTalentOutsourcingGenerateRequest()
	require.NoError(t, ValidateGenerateInput(req))
	for _, tt := range []struct {
		name   string
		mutate func(*dto.GenerateQuotationRequest)
		want   string
	}{
		{"missing pointer", func(r *dto.GenerateQuotationRequest) { r.TalentOutsourcing = nil }, "exactly talent_outsourcing"},
		{"dedicated pointer", func(r *dto.GenerateQuotationRequest) { r.DedicatedSquad = &domain.DedicatedSquadInput{} }, "exactly talent_outsourcing"},
		{"kiosk pointer", func(r *dto.GenerateQuotationRequest) { r.KioskQueueSystem = &domain.KioskQueueSystemInput{} }, "exactly talent_outsourcing"},
		{"rate card", func(r *dto.GenerateQuotationRequest) { r.RateCardID = uuid.New() }, "rate_card_id"},
		{"budget", func(r *dto.GenerateQuotationRequest) { r.BudgetMaximum = 1 }, "legacy quotation fields"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validTalentOutsourcingGenerateRequest()
			tt.mutate(&candidate)
			var ge *GenerationError
			require.True(t, errors.As(ValidateGenerateInput(candidate), &ge))
			require.Contains(t, ge.Message(), tt.want)
		})
	}
}

func TestValidateGenerateInputLegacySchemesRejectTalentOutsourcing(t *testing.T) {
	req := validTalentOutsourcingGenerateRequest()
	req.CommercialScheme = domain.CommercialSchemeKioskQueueSystem
	req.KioskQueueSystem = &domain.KioskQueueSystemInput{}
	var ge *GenerationError
	require.True(t, errors.As(ValidateGenerateInput(req), &ge))
	require.Contains(t, ge.Message(), "kiosk_queue_system")
}
