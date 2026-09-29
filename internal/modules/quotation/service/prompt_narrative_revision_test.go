package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func TestPromptNarrativeRevisionService_ReplacesOnlyRequestedSectionAndSanitizesOutput(t *testing.T) {
	repo := promptRevisionTestRepo(t)
	repo.record.ContentHTML = `<article><section data-quotation-section="scope" data-section-version="1"><p>BA edited scope</p></section><section data-quotation-section="timeline" data-section-version="1"><p>BA edited timeline</p></section><section data-quotation-section="commercial" data-section-version="1" data-managed="true"><p>commercial</p></section></article>`
	svc := NewPromptNarrativeRevisionService(repo, NewHTMLSanitizer())
	result, err := svc.Apply(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, PromptRevisionIntent{
		SchemaVersion: SchemaPromptRevisionV1, Intent: PromptRevisionIntentNarrative, TargetSection: "scope", ReplacementText: "Ringkas scope",
	}, promptRevisionRequest(repo, "Ringkas scope", "scope"))
	require.NoError(t, err)
	require.Contains(t, result.ContentHTML, "Ringkas scope")
	require.NotContains(t, result.ContentHTML, "<script")
	require.Contains(t, result.ContentHTML, "BA edited timeline")
	require.Contains(t, result.ContentHTML, "commercial")
	require.Len(t, result.CatalogRoles, 1)
	require.Equal(t, "frontend_programmer", result.CatalogRoles[0].Role)
}

type promptIntentClassifierStub struct {
	intent PromptRevisionIntent
	err    error
}

func (s promptIntentClassifierStub) Classify(context.Context, uuid.UUID, domain.PromptQuotationState, string, string, string) (PromptRevisionIntent, error) {
	return s.intent, s.err
}

func TestPromptSectionRevisionService_DelegatesCommercialAndBlocksUnpricedMaintenance(t *testing.T) {
	t.Run("discount delegates through the existing commercial patch path and preserves manual narrative", func(t *testing.T) {
		repo := promptRevisionTestRepo(t)
		commercial := NewPromptRevisionService(repo, NewHTMLSanitizer(), FixedDiscountPolicy{MaxBPS: 1_000, Floor: 1})
		classifier := promptIntentClassifierStub{intent: PromptRevisionIntent{SchemaVersion: SchemaPromptRevisionV1, Intent: PromptRevisionIntentCommercial, TargetSection: "commercial", CommercialPatch: []dto.PromptPatchOperation{{Type: dto.PromptPatchSetDiscountBPS, DiscountBPS: int32Ptr(500)}}}}
		svc := NewPromptSectionRevisionService(repo, classifier, NewPromptNarrativeRevisionService(repo, NewHTMLSanitizer()), commercial)
		result, err := svc.Revise(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, promptRevisionRequest(repo, "Berikan diskon lima persen.", ""))
		require.NoError(t, err)
		require.Contains(t, result.ContentHTML, "BA edited scope")
		require.Contains(t, result.ContentHTML, "BA edited timeline")
	})

	t.Run("maintenance leaves state and html untouched until a priced service is confirmed", func(t *testing.T) {
		repo := promptRevisionTestRepo(t)
		classifier := promptIntentClassifierStub{intent: PromptRevisionIntent{SchemaVersion: SchemaPromptRevisionV1, Intent: PromptRevisionIntentMaintenance, TargetSection: "commercial", CommercialPatch: []dto.PromptPatchOperation{{Type: "maintenance_clarification"}}}}
		svc := NewPromptSectionRevisionService(repo, classifier, NewPromptNarrativeRevisionService(repo, NewHTMLSanitizer()), NewPromptRevisionService(repo, NewHTMLSanitizer(), FixedDiscountPolicy{MaxBPS: 1_000, Floor: 1}))
		_, err := svc.Revise(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, promptRevisionRequest(repo, "Tambahkan maintenance tiga bulan.", ""))
		require.Equal(t, "maintenance_clarification_required", safePromptRevisionErrorCode(err))
		require.Empty(t, repo.params.ContentHTML)
		require.False(t, errors.Is(err, ErrPromptRevisionProvider))
	})
}

func promptRevisionRequest(repo *promptRevisionRepoStub, instruction, target string) dto.ReviseSectionRequest {
	return dto.ReviseSectionRequest{RevisionID: repo.record.RevisionID, ContentVersion: repo.record.ContentVersion, StateVersion: repo.record.StateVersion, Instruction: instruction, TargetSection: target}
}

func TestPromptNarrativeRevisionService_ProviderFailureDoesNotWrite(t *testing.T) {
	repo := promptRevisionTestRepo(t)
	classifier := promptIntentClassifierStub{err: ErrPromptRevisionProvider}
	svc := NewPromptSectionRevisionService(repo, classifier, NewPromptNarrativeRevisionService(repo, NewHTMLSanitizer()), NewPromptRevisionService(repo, NewHTMLSanitizer(), FixedDiscountPolicy{MaxBPS: 1_000, Floor: 1}))
	_, err := svc.Revise(context.Background(), repo.record.CompanyID, repo.record.QuotationID, nil, promptRevisionRequest(repo, "Ringkas scope", "scope"))
	require.Equal(t, "revision_provider_failed", safePromptRevisionErrorCode(err))
	require.Empty(t, repo.params.ContentHTML)
}
