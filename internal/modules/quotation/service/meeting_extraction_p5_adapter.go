package service

import (
	"context"
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// NewCatalogBackedMeetingExtractionP5 creates the P5 dispatcher per attempt,
// binding dedicated-squad calculation to the immutable catalog snapshot. The
// existing calculators/renderers remain the only pricing/rendering authority.
func NewCatalogBackedMeetingExtractionP5(repository P5FinalizationRepository, registry *PromptServiceAdapterRegistry) (MeetingExtractionP5Func, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrP5Finalization)
	}
	return func(ctx context.Context, in MeetingExtractionP5Input) error {
		finalizers := append([]P5SchemeFinalizer{
			NewP5DedicatedFinalizer(NewDedicatedSquadCalculator(snapshotDedicatedRateLookup{items: append([]domain.RateCardItem(nil), in.Catalog...)}), NewDedicatedSquadHTMLRenderer()),
		}, NewP5ExistingAdapterFinalizers(registry)...)
		finalizer, err := NewP5Finalizer(repository, finalizers...)
		if err != nil {
			return err
		}
		adapter, err := NewMeetingExtractionP5Adapter(finalizer)
		if err != nil {
			return err
		}
		return adapter(ctx, in)
	}, nil
}

// NewMeetingExtractionP5Adapter bridges the resumable orchestrator to the
// existing deterministic P5 dispatcher. It deliberately exposes only schemes
// with a production mapper; startup must reject the others instead of silently
// falling back to a no-op.
func NewMeetingExtractionP5Adapter(finalizer *P5Finalizer) (MeetingExtractionP5Func, error) {
	if finalizer == nil {
		return nil, fmt.Errorf("%w: finalizer is required", ErrP5Finalization)
	}
	if err := ValidatePipelineSchemeRegistry(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrP5Finalization, err)
	}
	return func(ctx context.Context, in MeetingExtractionP5Input) error {
		mapper, ready := PipelineSchemeMapper(in.Scheme)
		if !ready {
			if in.Scheme == PlanSchemeUnknown || in.Scheme == PlanSchemeMultiService {
				return nil
			}
			return fmt.Errorf("%w: mapper unavailable for scheme", ErrP5Finalization)
		}
		mapped, err := mapper(MeetingExtractionP5MapperInput{
			Scheme: in.Scheme, Merge: in.Merge, Phases: in.Validated,
			Catalog: append([]domain.RateCardItem(nil), in.Catalog...),
		})
		if err != nil {
			return err
		}
		if mapped.NeedsReview {
			return nil
		}
		if mapped.NeedsInput {
			return fmt.Errorf("%w: required prompt input is missing", ErrP5Finalization)
		}
		result, err := finalizer.Finalize(ctx, P5FinalizeInput{
			CompanyID: in.CompanyID, QuotationID: in.QuotationID, RevisionID: in.RevisionID,
			Scheme: in.Scheme, SolutionType: in.SolutionType, State: mapped.State,
			Metadata: in.Metadata, QuotationDate: in.QuotationDate, LegalHTML: in.LegalHTML,
			RateCardID: in.RateCardID, Catalog: append([]domain.RateCardItem(nil), in.Catalog...),
		})
		if err != nil {
			return err
		}
		if result.NeedsReview || !result.Completed {
			return fmt.Errorf("%w: finalizer did not complete", ErrP5Finalization)
		}
		return nil
	}, nil
}
