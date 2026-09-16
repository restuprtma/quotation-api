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
	ErrBlueprintProviderInvalidCredential = errors.New("provider_invalid_credential")
	ErrBlueprintProviderConfiguration     = errors.New("provider_configuration")
	ErrBlueprintProviderRateLimited       = errors.New("provider_rate_limited")
	ErrBlueprintProviderTimeout           = errors.New("provider_timeout")
	ErrBlueprintProviderUnavailable       = errors.New("provider_unavailable")
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
		safeCode := "provider_configuration"
		sentinel := ErrBlueprintProviderConfiguration
		if errors.Is(err, ErrBlueprintPromptSecurity) {
			safeCode = "security_failure"
			sentinel = ErrBlueprintSecurityFailure
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

	safeValidation, _ := json.Marshal(BlueprintValidationResult{
		Valid:       validation.Valid,
		SafeOutcome: validation.SafeOutcome,
		Issues:      validation.Issues,
	})

	writeCtx, writeCancel := context.WithTimeout(context.WithoutCancel(ctx), BlueprintTerminalWriteTimeout)
	defer writeCancel()

	result, err := p.repo.CompleteBlueprintSection(writeCtx, repository.CompleteBlueprintSectionParams{
		Claim:                 *claim,
		SectionContent:        validation.NormalizedContent,
		SectionValidation:     safeValidation,
		RevisionContent:       claim.RevisionContent,
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

func (p *BlueprintPipeline) RetrySection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*BlueprintPipelineResult, error) {
	if p == nil || p.repo == nil || p.providers == nil || p.prompts == nil || p.validator == nil || p.crossValidator == nil || p.clock == nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	return nil, ErrBlueprintRetryNotAllowed
}

func (p *BlueprintPipeline) ManualCompleteSection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, payload json.RawMessage) (*BlueprintPipelineResult, error) {
	if p == nil || p.repo == nil || p.providers == nil || p.prompts == nil || p.validator == nil || p.crossValidator == nil || p.clock == nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintPipelineInvalidInput
	}
	return nil, ErrBlueprintManualNotAllowed
}

func (p *BlueprintPipeline) mapClaimError(err error) error {
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
