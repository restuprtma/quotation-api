package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

const (
	BlueprintProviderAttemptTimeout = 180 * time.Second
	BlueprintGeneratingStaleAfter   = 5 * time.Minute
	BlueprintTerminalWriteTimeout   = 5 * time.Second
)

var (
	ErrBlueprintPipelineInvalidInput      = errors.New("blueprint_invalid_input")
	ErrBlueprintPipelineNotFound          = errors.New("blueprint_not_found")
	ErrBlueprintPipelineBusy              = errors.New("pipeline_busy")
	ErrBlueprintPipelineCompleted         = errors.New("pipeline_completed")
	ErrBlueprintPipelineBlocked           = errors.New("pipeline_blocked")
	ErrBlueprintSectionRecoveryRequired   = errors.New("section_recovery_required")
	ErrBlueprintPipelineRecoveryRequired  = errors.New("recovery_required")
	ErrBlueprintRetryNotAllowed           = errors.New("retry_not_allowed")
	ErrBlueprintAttemptLimitReached       = errors.New("attempt_limit_reached")
	ErrBlueprintManualNotAllowed          = errors.New("manual_completion_not_allowed")
	ErrBlueprintExecutionConflict         = errors.New("execution_conflict")
	ErrBlueprintPromptConstruction        = errors.New("prompt_construction_failed")
	ErrBlueprintProviderInvalidCredential = errors.New("provider_invalid_credential")
	ErrBlueprintProviderConfiguration     = errors.New("provider_configuration")
	ErrBlueprintProviderRateLimited       = errors.New("provider_rate_limited")
	ErrBlueprintProviderTimeout           = errors.New("provider_timeout")
	ErrBlueprintProviderUnavailable       = errors.New("provider_unavailable")
	ErrBlueprintProviderRejected          = errors.New("provider_request_rejected")
	ErrBlueprintProviderPayloadSize       = errors.New("provider_payload_too_large")
	ErrBlueprintInvalidSchema             = errors.New("invalid_schema")
	ErrBlueprintSecurityFailure           = errors.New("security_failure")
	ErrBlueprintCrossValidationFailure    = errors.New("cross_validation_error")
	ErrBlueprintPersistenceFailure        = errors.New("persistence_failure")
)

type BlueprintPipelineRepository interface {
	ClaimNextBlueprintSection(context.Context, uuid.UUID, uuid.UUID, time.Time, time.Duration) (*repository.BlueprintSectionClaim, error)
	ClaimFailedBlueprintSection(context.Context, uuid.UUID, uuid.UUID, domain.BlueprintSectionType, time.Time, time.Duration) (*repository.BlueprintSectionClaim, error)
	CompleteBlueprintSection(context.Context, repository.CompleteBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error)
	FailBlueprintSection(context.Context, repository.FailBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error)
	LoadBlueprintManualSnapshot(context.Context, uuid.UUID, uuid.UUID, domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error)
	CompleteBlueprintSectionManually(context.Context, repository.CompleteBlueprintManualParams) (*repository.BlueprintSectionPersistenceResult, error)
}

type BlueprintStagePromptBuilder interface {
	Build(BlueprintPromptInput) (string, error)
}

type BlueprintStageValidator interface {
	Validate(domain.BlueprintSectionType, json.RawMessage) BlueprintValidationResult
}

type BlueprintBundleCrossValidator interface {
	Validate(BlueprintBundle) BlueprintCrossValidationResult
}

type BlueprintPipelineClock interface {
	Now() time.Time
}

type defaultPipelineClock struct{}

func (defaultPipelineClock) Now() time.Time {
	return time.Now()
}

type BlueprintPipelineResult struct {
	BlueprintID      uuid.UUID
	RevisionID       uuid.UUID
	Section          domain.BlueprintSectionType
	SectionStatus    domain.BlueprintSectionStatus
	GenerationStatus domain.BlueprintGenerationStatus
	ContentVersion   int
	Attempt          int
}

type BlueprintPipeline struct {
	repo            BlueprintPipelineRepository
	providers       ProviderSource
	prompts         BlueprintStagePromptBuilder
	validator       BlueprintStageValidator
	crossValidator  BlueprintBundleCrossValidator
	clock           BlueprintPipelineClock
	providerTimeout time.Duration
	recoveryWindow  time.Duration
}

type BlueprintPipelineOption func(*BlueprintPipeline)

func WithBlueprintPipelineClock(clock BlueprintPipelineClock) BlueprintPipelineOption {
	return func(p *BlueprintPipeline) {
		if clock != nil {
			p.clock = clock
		}
	}
}

func WithBlueprintProviderTimeout(d time.Duration) BlueprintPipelineOption {
	return func(p *BlueprintPipeline) {
		if d > 0 {
			p.providerTimeout = d
		}
	}
}

func WithBlueprintRecoveryWindow(d time.Duration) BlueprintPipelineOption {
	return func(p *BlueprintPipeline) {
		if d > 0 {
			p.recoveryWindow = d
		}
	}
}

func NewBlueprintPipeline(
	repo BlueprintPipelineRepository,
	providers ProviderSource,
	prompts BlueprintStagePromptBuilder,
	validator BlueprintStageValidator,
	crossValidator BlueprintBundleCrossValidator,
	opts ...BlueprintPipelineOption,
) *BlueprintPipeline {
	p := &BlueprintPipeline{
		repo:            repo,
		providers:       providers,
		prompts:         prompts,
		validator:       validator,
		crossValidator:  crossValidator,
		clock:           defaultPipelineClock{},
		providerTimeout: BlueprintProviderAttemptTimeout,
		recoveryWindow:  BlueprintGeneratingStaleAfter,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *BlueprintPipeline) Continue(ctx context.Context, companyID, blueprintID uuid.UUID) (*BlueprintPipelineResult, error) {
	if p == nil || p.repo == nil || p.providers == nil || p.prompts == nil || p.validator == nil || p.crossValidator == nil || p.clock == nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}

	claim, err := p.repo.ClaimNextBlueprintSection(ctx, companyID, blueprintID, p.clock.Now(), p.recoveryWindow)
	if err != nil {
		return nil, p.mapClaimError(err)
	}

	if claim.SectionType == domain.BlueprintSectionCrossValidation {
		return p.executeCrossValidationStage(ctx, claim)
	}

	return p.executeClaimedProviderStage(ctx, companyID, claim)
}

func (p *BlueprintPipeline) RetrySection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*BlueprintPipelineResult, error) {
	if p == nil || p.repo == nil || p.providers == nil || p.prompts == nil || p.validator == nil || p.crossValidator == nil || p.clock == nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if !isProviderSection(sectionType) {
		return nil, ErrBlueprintRetryNotAllowed
	}

	claim, err := p.repo.ClaimFailedBlueprintSection(ctx, companyID, blueprintID, sectionType, p.clock.Now(), p.recoveryWindow)
	if err != nil {
		return nil, p.mapClaimFailedError(err)
	}

	return p.executeClaimedProviderStage(ctx, companyID, claim)
}

// BlueprintManualValidationError provides typed validation error details for manual completion.
type BlueprintManualValidationError struct {
	BaseErr     error
	SafeOutcome string
	Issues      []BlueprintValidationIssue
}

func (e *BlueprintManualValidationError) Error() string {
	if e.BaseErr != nil {
		return e.BaseErr.Error()
	}
	return "blueprint manual validation error"
}

func (e *BlueprintManualValidationError) Unwrap() error {
	return e.BaseErr
}

func (p *BlueprintPipeline) ManualCompleteSection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, payload json.RawMessage) (*BlueprintPipelineResult, error) {
	return p.manualCompleteSection(ctx, companyID, blueprintID, sectionType, nil, nil, payload)
}

func (p *BlueprintPipeline) ManualCompleteSectionAtVersion(
	ctx context.Context,
	companyID, blueprintID uuid.UUID,
	sectionType domain.BlueprintSectionType,
	revisionID uuid.UUID,
	expectedVersion int,
	payload json.RawMessage,
) (*BlueprintPipelineResult, error) {
	if companyID == uuid.Nil || blueprintID == uuid.Nil || revisionID == uuid.Nil || expectedVersion < 1 {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	return p.manualCompleteSection(ctx, companyID, blueprintID, sectionType, &revisionID, &expectedVersion, payload)
}

func (p *BlueprintPipeline) manualCompleteSection(
	ctx context.Context,
	companyID, blueprintID uuid.UUID,
	sectionType domain.BlueprintSectionType,
	expectedRevisionID *uuid.UUID,
	expectedVersion *int,
	payload json.RawMessage,
) (*BlueprintPipelineResult, error) {
	if p == nil || p.repo == nil || p.providers == nil || p.prompts == nil || p.validator == nil || p.crossValidator == nil || p.clock == nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if !isProviderSection(sectionType) {
		return nil, ErrBlueprintManualNotAllowed
	}

	snapshot, err := p.repo.LoadBlueprintManualSnapshot(ctx, companyID, blueprintID, sectionType)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintPipelineNotFound
		}
		return nil, ErrBlueprintPersistenceFailure
	}

	if expectedRevisionID != nil && expectedVersion != nil {
		if snapshot.RevisionID != *expectedRevisionID || snapshot.ExpectedContentVersion != *expectedVersion {
			return nil, ErrBlueprintExecutionConflict
		}
	}

	validation := p.validator.Validate(sectionType, payload)
	if !validation.Valid {
		baseErr := ErrBlueprintInvalidSchema
		if validation.SafeOutcome == BlueprintValidationSecurityFailure {
			baseErr = ErrBlueprintSecurityFailure
		}
		return nil, &BlueprintManualValidationError{
			BaseErr:     baseErr,
			SafeOutcome: validation.SafeOutcome,
			Issues:      validation.Issues,
		}
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

	result, err := p.repo.CompleteBlueprintSectionManually(writeCtx, repository.CompleteBlueprintManualParams{
		Snapshot:            *snapshot,
		SectionContent:      validation.NormalizedContent,
		SectionValidation:   safeValidation,
		RevisionContent:     cleaned,
		InvalidatedSections: invalidated,
		CompletedAt:         p.clock.Now(),
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintManualConflict) ||
			errors.Is(err, repository.ErrBlueprintContentConflict) ||
			errors.Is(err, repository.ErrBlueprintClaimConflict) {
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

func (p *BlueprintPipeline) executeClaimedProviderStage(
	ctx context.Context,
	companyID uuid.UUID,
	claim *repository.BlueprintSectionClaim,
) (*BlueprintPipelineResult, error) {
	provider, providerName, model, err := p.providers.ProviderForCompany(ctx, companyID)
	if err != nil {
		safeCode, sentinel := mapProviderResolutionError(err)
		return nil, p.persistFailure(ctx, claim, safeCode, nil, sentinel)
	}
	if provider == nil {
		return nil, p.persistFailure(ctx, claim, "provider_configuration", nil, ErrBlueprintProviderConfiguration)
	}

	prompt, err := p.prompts.Build(BlueprintPromptInput{
		Stage:         claim.SectionType,
		SkillSnapshot: claim.SkillSnapshot,
		Context:       claim.SourceContextSnapshot,
		Dependencies:  claim.Dependencies,
	})
	if err != nil {
		var safeCode string
		var sentinel error
		switch {
		case errors.Is(err, ErrBlueprintPromptSecurity):
			safeCode, sentinel = "security_failure", ErrBlueprintSecurityFailure
		case errors.Is(err, ErrBlueprintPromptInvalidInput):
			safeCode, sentinel = "prompt_construction_failed", ErrBlueprintPromptConstruction
		default:
			safeCode, sentinel = "prompt_construction_failed", ErrBlueprintPromptConstruction
		}
		return nil, p.persistFailure(ctx, claim, safeCode, nil, sentinel)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, p.providerTimeout)
	generated, genErr := provider.Generate(attemptCtx, GenerateRequest{Prompt: prompt})
	cancel()
	if genErr != nil {
		safeCode, sentinel := mapProviderGenerateError(genErr, attemptCtx)
		return nil, p.persistFailure(ctx, claim, safeCode, nil, sentinel)
	}

	validation := p.validator.Validate(claim.SectionType, json.RawMessage(generated.Text))
	if !validation.Valid {
		safeCode := "invalid_schema"
		sentinel := ErrBlueprintInvalidSchema
		if validation.SafeOutcome == BlueprintValidationSecurityFailure {
			safeCode = "security_failure"
			sentinel = ErrBlueprintSecurityFailure
		}
		return nil, p.persistFailure(ctx, claim, safeCode, &validation, sentinel)
	}

	merged, err := MergeBlueprintSection(
		claim.RevisionContent, claim.BlueprintTitle, claim.ContextContent,
		claim.SectionType, validation.NormalizedContent,
	)
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

	result, err := p.repo.CompleteBlueprintSection(writeCtx, repository.CompleteBlueprintSectionParams{
		Claim:                 *claim,
		SectionContent:        validation.NormalizedContent,
		SectionValidation:     safeValidation,
		RevisionContent:       merged,
		CrossValidationResult: nil,
		ProviderName:          providerName,
		Model:                 model,
		CompletedAt:           p.clock.Now(),
		Final:                 false,
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintClaimConflict) || errors.Is(err, repository.ErrBlueprintContentConflict) {
			return nil, ErrBlueprintExecutionConflict
		}
		return nil, ErrBlueprintPersistenceFailure
	}

	return &BlueprintPipelineResult{
		BlueprintID:      claim.BlueprintID,
		RevisionID:       result.RevisionID,
		Section:          result.SectionType,
		SectionStatus:    result.SectionStatus,
		GenerationStatus: result.GenerationStatus,
		ContentVersion:   result.ContentVersion,
		Attempt:          result.Attempt,
	}, nil
}

func (p *BlueprintPipeline) executeCrossValidationStage(ctx context.Context, claim *repository.BlueprintSectionClaim) (*BlueprintPipelineResult, error) {
	bundle, err := DecodeBlueprintBundle(claim.RevisionContent)
	if err != nil {
		crossResult := BlueprintCrossValidationResult{
			Valid:       false,
			SafeOutcome: BlueprintValidationCrossValidationError,
			Issues: []BlueprintValidationIssue{
				{
					Code:     BlueprintIssueUnsupportedVersion,
					Severity: "CRITICAL",
					Section:  "cross_validation",
					Message:  "failed to decode blueprint bundle",
					Blocking: true,
				},
			},
		}
		crossJSON, _ := json.Marshal(crossResult)
		return nil, p.persistCrossValidationFailure(ctx, claim, crossJSON, ErrBlueprintCrossValidationFailure)
	}

	cross := p.crossValidator.Validate(bundle)
	crossJSON, err := json.Marshal(cross)
	if err != nil {
		return nil, ErrBlueprintPersistenceFailure
	}

	if !cross.Valid {
		return nil, p.persistCrossValidationFailure(ctx, claim, crossJSON, ErrBlueprintCrossValidationFailure)
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), BlueprintTerminalWriteTimeout)
	defer cancel()

	result, err := p.repo.CompleteBlueprintSection(writeCtx, repository.CompleteBlueprintSectionParams{
		Claim:                 *claim,
		SectionContent:        crossJSON,
		SectionValidation:     crossJSON,
		RevisionContent:       claim.RevisionContent,
		CrossValidationResult: crossJSON,
		CompletedAt:           p.clock.Now(),
		Final:                 true,
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintClaimConflict) || errors.Is(err, repository.ErrBlueprintContentConflict) {
			return nil, ErrBlueprintExecutionConflict
		}
		return nil, ErrBlueprintPersistenceFailure
	}

	return &BlueprintPipelineResult{
		BlueprintID:      claim.BlueprintID,
		RevisionID:       result.RevisionID,
		Section:          result.SectionType,
		SectionStatus:    result.SectionStatus,
		GenerationStatus: result.GenerationStatus,
		ContentVersion:   result.ContentVersion,
		Attempt:          result.Attempt,
	}, nil
}

func (p *BlueprintPipeline) persistCrossValidationFailure(
	ctx context.Context,
	claim *repository.BlueprintSectionClaim,
	crossJSON json.RawMessage,
	sentinel error,
) error {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), BlueprintTerminalWriteTimeout)
	defer cancel()

	_, err := p.repo.FailBlueprintSection(writeCtx, repository.FailBlueprintSectionParams{
		Claim:                 *claim,
		SafeErrorCode:         "cross_validation_error",
		SectionContent:        crossJSON,
		SectionValidation:     crossJSON,
		CrossValidationResult: crossJSON,
		FailedAt:              p.clock.Now(),
		Terminal:              false,
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintClaimConflict) || errors.Is(err, repository.ErrBlueprintContentConflict) {
			return ErrBlueprintExecutionConflict
		}
		return ErrBlueprintPersistenceFailure
	}

	return sentinel
}

func isProviderSection(s domain.BlueprintSectionType) bool {
	switch s {
	case domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS:
		return true
	default:
		return false
	}
}

func (p *BlueprintPipeline) mapClaimError(err error) error {
	if errors.Is(err, ErrBlueprintPipelineCompleted) {
		return ErrBlueprintPipelineCompleted
	}
	if errors.Is(err, ErrBlueprintSectionRecoveryRequired) {
		return ErrBlueprintSectionRecoveryRequired
	}
	if errors.Is(err, ErrBlueprintPipelineBlocked) {
		return ErrBlueprintPipelineBlocked
	}
	if errors.Is(err, repository.ErrBlueprintNotFound) {
		return ErrBlueprintPipelineNotFound
	}
	if errors.Is(err, repository.ErrBlueprintClaimBusy) {
		return ErrBlueprintPipelineBusy
	}
	if errors.Is(err, repository.ErrBlueprintRecoveryRequired) {
		return ErrBlueprintPipelineRecoveryRequired
	}
	if errors.Is(err, repository.ErrBlueprintNoEligibleSection) {
		return ErrBlueprintPipelineBlocked
	}
	return ErrBlueprintPersistenceFailure
}

func (p *BlueprintPipeline) mapClaimFailedError(err error) error {
	if errors.Is(err, repository.ErrBlueprintNotFound) {
		return ErrBlueprintPipelineNotFound
	}
	if errors.Is(err, repository.ErrBlueprintClaimBusy) {
		return ErrBlueprintPipelineBusy
	}
	if errors.Is(err, repository.ErrBlueprintRecoveryRequired) {
		return ErrBlueprintPipelineRecoveryRequired
	}
	if errors.Is(err, repository.ErrBlueprintAttemptLimit) {
		return ErrBlueprintAttemptLimitReached
	}
	if errors.Is(err, repository.ErrBlueprintSectionNotRetryable) {
		return ErrBlueprintRetryNotAllowed
	}
	if errors.Is(err, repository.ErrBlueprintNoEligibleSection) {
		return ErrBlueprintPipelineBlocked
	}
	return ErrBlueprintPersistenceFailure
}

func (p *BlueprintPipeline) persistFailure(
	ctx context.Context,
	claim *repository.BlueprintSectionClaim,
	safeCode string,
	validationResult *BlueprintValidationResult,
	sentinel error,
) error {
	isTerminal := safeCode == "security_failure" || claim.Attempt >= repository.BlueprintMaxSectionAttempts

	var sectionValidation json.RawMessage
	if validationResult != nil {
		sectionValidation, _ = json.Marshal(BlueprintValidationResult{
			Valid:       validationResult.Valid,
			SafeOutcome: validationResult.SafeOutcome,
			Issues:      validationResult.Issues,
		})
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), BlueprintTerminalWriteTimeout)
	defer cancel()

	_, err := p.repo.FailBlueprintSection(writeCtx, repository.FailBlueprintSectionParams{
		Claim:                 *claim,
		SafeErrorCode:         safeCode,
		SectionContent:        nil,
		SectionValidation:     sectionValidation,
		CrossValidationResult: nil,
		FailedAt:              p.clock.Now(),
		Terminal:              isTerminal,
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintClaimConflict) || errors.Is(err, repository.ErrBlueprintContentConflict) {
			return ErrBlueprintExecutionConflict
		}
		return ErrBlueprintPersistenceFailure
	}

	return sentinel
}

func mapProviderResolutionError(err error) (string, error) {
	var pErr *ProviderError
	if errors.As(err, &pErr) {
		switch pErr.Code {
		case ErrInvalidCredential, ErrCredentialNotConfigured:
			return "provider_invalid_credential", ErrBlueprintProviderInvalidCredential
		case ErrUnsupportedModel:
			return "provider_configuration", ErrBlueprintProviderConfiguration
		case ErrRateLimited:
			return "provider_rate_limited", ErrBlueprintProviderRateLimited
		case ErrTimeout:
			return "provider_timeout", ErrBlueprintProviderTimeout
		case ErrProviderUnavailable:
			return "provider_unavailable", ErrBlueprintProviderUnavailable
		default:
			return "provider_configuration", ErrBlueprintProviderConfiguration
		}
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "credential") || strings.Contains(errStr, "key") {
		return "provider_invalid_credential", ErrBlueprintProviderInvalidCredential
	}
	if strings.Contains(errStr, "model") || strings.Contains(errStr, "config") {
		return "provider_configuration", ErrBlueprintProviderConfiguration
	}
	if strings.Contains(errStr, "rate") {
		return "provider_rate_limited", ErrBlueprintProviderRateLimited
	}
	if strings.Contains(errStr, "timeout") || errors.Is(err, context.DeadlineExceeded) {
		return "provider_timeout", ErrBlueprintProviderTimeout
	}
	return "provider_unavailable", ErrBlueprintProviderUnavailable
}

func mapProviderGenerateError(err error, attemptCtx context.Context) (string, error) {
	if errors.Is(err, context.DeadlineExceeded) || (attemptCtx != nil && errors.Is(attemptCtx.Err(), context.DeadlineExceeded)) {
		return "provider_timeout", ErrBlueprintProviderTimeout
	}
	var pErr *ProviderError
	if errors.As(err, &pErr) {
		switch pErr.Code {
		case ErrTimeout:
			return "provider_timeout", ErrBlueprintProviderTimeout
		case ErrRateLimited:
			return "provider_rate_limited", ErrBlueprintProviderRateLimited
		case ErrInvalidCredential, ErrCredentialNotConfigured:
			return "provider_invalid_credential", ErrBlueprintProviderInvalidCredential
		case ErrUnsupportedModel:
			return "provider_configuration", ErrBlueprintProviderConfiguration
		case ErrProviderUnavailable:
			return "provider_unavailable", ErrBlueprintProviderUnavailable
		default:
			return "provider_unavailable", ErrBlueprintProviderUnavailable
		}
	}
	errStr := strings.ToLower(err.Error())
	if strings.Contains(errStr, "timeout") {
		return "provider_timeout", ErrBlueprintProviderTimeout
	}
	if strings.Contains(errStr, "rate") {
		return "provider_rate_limited", ErrBlueprintProviderRateLimited
	}
	if strings.Contains(errStr, "credential") || strings.Contains(errStr, "key") {
		return "provider_invalid_credential", ErrBlueprintProviderInvalidCredential
	}
	return "provider_unavailable", ErrBlueprintProviderUnavailable
}
