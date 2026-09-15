package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func validTalentOutsourcingGenerateRequest() dto.GenerateQuotationRequest {
	in := talentBPJSInput()
	in.DocumentNumber = "LANDA/BPJS/0626/001"
	return dto.GenerateQuotationRequest{Subject: "Penawaran Talent", SolutionType: "talent_outsourcing", CommercialScheme: domain.CommercialSchemeTalentOutsourcing, TalentOutsourcing: &in}
}

func TestInspectTalentOutsourcingDraftClassification(t *testing.T) {
	req := validTalentOutsourcingGenerateRequest()
	calc, err := CalculateTalentOutsourcing(*req.TalentOutsourcing)
	require.NoError(t, err)
	profile, err := ResolveTalentOutsourcingProfile(*req.TalentOutsourcing)
	require.NoError(t, err)
	legal := `<section data-block-type="legal"><p>Ketentuan Venturo</p></section>`
	html, err := NewTalentOutsourcingRenderer().Render(TalentOutsourcingDocumentInput{Input: *req.TalentOutsourcing, Calculation: calc, Profile: profile, LegalHTML: legal})
	require.NoError(t, err)
	draft := &repository.DraftRevisionForSave{CommercialScheme: domain.CommercialSchemeTalentOutsourcing, GenerationInput: mustTalentJSON(t, req), CalculationSnapshot: mustTalentJSON(t, calc), ResolvedProfileSnapshot: mustTalentJSON(t, profile), LegalTemplateSnapshot: mustTalentJSON(t, legalSnapshotView{ContentHTML: legal})}
	svc := NewQuotationGenerationService(nil, nil)
	legalModified, commercialModified, total, err := svc.inspectTalentOutsourcingDraft(t.Context(), uuid.New(), draft, html)
	require.NoError(t, err)
	require.False(t, legalModified)
	require.False(t, commercialModified)
	require.Equal(t, calc.GrandTotal, total)

	legalModified, commercialModified, _, err = svc.inspectTalentOutsourcingDraft(t.Context(), uuid.New(), draft, strings.Replace(html, "Description", "Narasi scope yang diperbarui", 1))
	require.NoError(t, err)
	require.False(t, legalModified)
	require.False(t, commercialModified)

	legalModified, commercialModified, _, err = svc.inspectTalentOutsourcingDraft(t.Context(), uuid.New(), draft, strings.Replace(html, `data-value="44400000"`, `data-value="1"`, 1))
	require.NoError(t, err)
	require.False(t, legalModified)
	require.True(t, commercialModified)

	legalModified, commercialModified, _, err = svc.inspectTalentOutsourcingDraft(t.Context(), uuid.New(), draft, strings.Replace(html, "Ketentuan Venturo", "Ketentuan berubah", 1))
	require.NoError(t, err)
	require.True(t, legalModified)
	require.False(t, commercialModified)
}

func TestInspectTalentOutsourcingDraftFailsClosedOnCorruptSnapshots(t *testing.T) {
	req := validTalentOutsourcingGenerateRequest()
	calc, _ := CalculateTalentOutsourcing(*req.TalentOutsourcing)
	profile, _ := ResolveTalentOutsourcingProfile(*req.TalentOutsourcing)
	legal := `<section data-block-type="legal"><p>Ketentuan Venturo</p></section>`
	html, _ := NewTalentOutsourcingRenderer().Render(TalentOutsourcingDocumentInput{Input: *req.TalentOutsourcing, Calculation: calc, Profile: profile, LegalHTML: legal})
	draft := &repository.DraftRevisionForSave{CommercialScheme: domain.CommercialSchemeTalentOutsourcing, GenerationInput: []byte(`{`), CalculationSnapshot: mustTalentJSON(t, calc), ResolvedProfileSnapshot: mustTalentJSON(t, profile), LegalTemplateSnapshot: mustTalentJSON(t, legalSnapshotView{ContentHTML: legal})}
	_, _, _, err := NewQuotationGenerationService(nil, nil).inspectTalentOutsourcingDraft(t.Context(), uuid.New(), draft, html)
	var ge *GenerationError
	require.True(t, errors.As(err, &ge))
	require.Equal(t, ErrCodeUnsafeHTML, ge.Code)
}

func mustTalentJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
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
