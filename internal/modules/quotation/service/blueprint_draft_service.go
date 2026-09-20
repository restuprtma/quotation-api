package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	// ErrBlueprintDraftNotImplemented represents the unimplemented stub state in Phase 09 S01.
	ErrBlueprintDraftNotImplemented = errors.New("blueprint_draft_not_implemented")
)

// BlueprintDraftValidator defines the validation methods required for saving blueprint section drafts.
type BlueprintDraftValidator interface {
	Validate(domain.BlueprintSectionType, json.RawMessage) BlueprintValidationResult
	ValidateDraft(domain.BlueprintSectionType, json.RawMessage) BlueprintValidationResult
}

// BlueprintDraftService handles saving draft sections with optimistic concurrency, shape validation, and downstream invalidation.
type BlueprintDraftService struct {
	repo           repository.BlueprintDraftRepository
	validator      BlueprintDraftValidator
	crossValidator BlueprintBundleCrossValidator
	providers      ProviderSource
	clock          BlueprintPipelineClock
}

// NewBlueprintDraftService constructs a new BlueprintDraftService instance.
func NewBlueprintDraftService(
	repo repository.BlueprintDraftRepository,
	validator BlueprintDraftValidator,
	crossValidator BlueprintBundleCrossValidator,
	providers ProviderSource,
	opts ...BlueprintPipelineOption,
) *BlueprintDraftService {
	s := &BlueprintDraftService{
		repo:           repo,
		validator:      validator,
		crossValidator: crossValidator,
		providers:      providers,
		clock:          defaultPipelineClock{},
	}
	for _, opt := range opts {
		if opt != nil {
			pipeline := &BlueprintPipeline{}
			opt(pipeline)
			if pipeline.clock != nil {
				s.clock = pipeline.clock
			}
		}
	}
	return s
}

// SaveDraftSectionAtVersion saves a section draft under optimistic concurrency control, setting run status to manual_edit.
func (s *BlueprintDraftService) SaveDraftSectionAtVersion(
	ctx context.Context,
	companyID, blueprintID uuid.UUID,
	sectionType domain.BlueprintSectionType,
	revisionID uuid.UUID,
	expectedVersion int,
	payload json.RawMessage,
) (*BlueprintPipelineResult, error) {
	if s == nil || s.repo == nil || s.validator == nil || s.clock == nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if companyID == uuid.Nil || blueprintID == uuid.Nil || revisionID == uuid.Nil || expectedVersion < 1 {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if !isProviderSection(sectionType) {
		return nil, ErrBlueprintInvalidSchema
	}

	snapshot, err := s.repo.LoadBlueprintDraftSnapshot(ctx, companyID, blueprintID, sectionType)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintPipelineNotFound
		}
		return nil, ErrBlueprintPersistenceFailure
	}

	if snapshot.RevisionID != revisionID || snapshot.ExpectedContentVersion != expectedVersion {
		return nil, ErrBlueprintExecutionConflict
	}

	if snapshot.SectionStatus == domain.BlueprintSectionGenerating {
		return nil, ErrBlueprintExecutionConflict
	}

	validation := s.validator.ValidateDraft(sectionType, payload)
	if validation.SafeOutcome != "" {
		if validation.SafeOutcome == BlueprintValidationSecurityFailure {
			return nil, ErrBlueprintSecurityFailure
		}
		return nil, ErrBlueprintInvalidSchema
	}

	merged, err := MergeBlueprintSection(
		snapshot.RevisionContent, snapshot.BlueprintTitle, snapshot.ContextContent,
		sectionType, validation.NormalizedContent,
	)
	if err != nil {
		return nil, ErrBlueprintInvalidSchema
	}

	cleaned, invalidated, err := InvalidateBlueprintDownstream(merged, sectionType)
	if err != nil {
		return nil, ErrBlueprintInvalidSchema
	}

	safeValidation, err := json.Marshal(BlueprintValidationResult{
		Valid:       validation.Valid,
		SafeOutcome: validation.SafeOutcome,
		Issues:      validation.Issues,
	})
	if err != nil {
		return nil, ErrBlueprintPersistenceFailure
	}

	writeCtx, writeCancel := context.WithTimeout(context.WithoutCancel(ctx), BlueprintTerminalWriteTimeout)
	defer writeCancel()

	result, err := s.repo.SaveBlueprintDraft(writeCtx, repository.SaveBlueprintDraftParams{
		Snapshot:            *snapshot,
		SectionContent:      validation.NormalizedContent,
		SectionValidation:   safeValidation,
		RevisionContent:     cleaned,
		InvalidatedSections: invalidated,
		SavedAt:             s.clock.Now(),
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintContentConflict) ||
			errors.Is(err, repository.ErrBlueprintClaimConflict) ||
			errors.Is(err, repository.ErrBlueprintManualConflict) {
			return nil, ErrBlueprintExecutionConflict
		}
		if errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintPipelineNotFound
		}
		return nil, ErrBlueprintPersistenceFailure
	}

	return &BlueprintPipelineResult{
		BlueprintID:      snapshot.BlueprintID,
		RevisionID:       result.RevisionID,
		Section:          result.SectionType,
		SectionStatus:    result.SectionStatus,
		GenerationStatus: result.GenerationStatus,
		ContentVersion:   result.ContentVersion,
		Attempt:          result.Attempt,
	}, nil
}
