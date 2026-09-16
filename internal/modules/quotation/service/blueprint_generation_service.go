package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	// ErrBlueprintGenerationInvalidInput indicates an incomplete or unusable create request.
	ErrBlueprintGenerationInvalidInput = errors.New("invalid blueprint generation input")
	// ErrBlueprintGenerationTenantMismatch indicates that the authenticated tenant does not own the source identity.
	ErrBlueprintGenerationTenantMismatch = errors.New("blueprint generation tenant mismatch")
)

// BlueprintSourceLoader loads tenant-scoped quotation revisions for blueprint generation.
type BlueprintSourceLoader interface {
	LoadEligibleSource(ctx context.Context, companyID, quotationID, revisionID uuid.UUID) (repository.BlueprintSourceRecord, error)
}

// BlueprintSkillResolver resolves active skills by command within a tenant.
type BlueprintSkillResolver interface {
	GetActiveSkillByCommand(ctx context.Context, companyID, command string) (*domain.Skill, error)
}

// BlueprintContextBuilder normalizes a tenant-scoped source projection for persistence.
type BlueprintContextBuilder interface {
	Build(source BlueprintSourceProjection, request dto.CreateBlueprintGenerationRequest) (NormalizedBlueprintContext, error)
}

// BlueprintGenerationCreator persists the normalized Blueprint generation aggregate.
type BlueprintGenerationCreator interface {
	CreateBlueprintGeneration(ctx context.Context, params repository.CreateBlueprintGenerationParams) (*repository.CreateBlueprintGenerationResult, error)
}

// BlueprintGenerationService coordinates source load, skill resolution, normalization, and aggregate creation.
type BlueprintGenerationService struct {
	sourceLoader  BlueprintSourceLoader
	skillResolver BlueprintSkillResolver
	builder       BlueprintContextBuilder
	creator       BlueprintGenerationCreator
}

// NewBlueprintGenerationService constructs the service with its required dependencies.
func NewBlueprintGenerationService(
	sourceLoader BlueprintSourceLoader,
	skillResolver BlueprintSkillResolver,
	builder BlueprintContextBuilder,
	creator BlueprintGenerationCreator,
) *BlueprintGenerationService {
	return &BlueprintGenerationService{
		sourceLoader:  sourceLoader,
		skillResolver: skillResolver,
		builder:       builder,
		creator:       creator,
	}
}

// CreateBlueprint loads eligible source, resolves active /blueprint skill, builds immutable snapshots, normalizes, and persists the aggregate.
func (s *BlueprintGenerationService) CreateBlueprint(
	ctx context.Context,
	companyID, createdBy uuid.UUID,
	request dto.CreateBlueprintGenerationRequest,
) (*repository.CreateBlueprintGenerationResult, error) {
	if s == nil || s.sourceLoader == nil || s.skillResolver == nil || s.builder == nil || s.creator == nil {
		return nil, fmt.Errorf("%w: service dependencies are required", ErrBlueprintGenerationInvalidInput)
	}
	if companyID == uuid.Nil || createdBy == uuid.Nil {
		return nil, fmt.Errorf("%w: company and creator IDs are required", ErrBlueprintGenerationInvalidInput)
	}
	if request.QuotationID == uuid.Nil || request.SourceQuotationRevisionID == uuid.Nil || request.IdempotencyKey == uuid.Nil {
		return nil, fmt.Errorf("%w: quotation, revision, and idempotency key IDs are required", ErrBlueprintGenerationInvalidInput)
	}
	if strings.TrimSpace(request.Title) == "" {
		return nil, fmt.Errorf("%w: title is required", ErrBlueprintGenerationInvalidInput)
	}

	sourceRecord, err := s.sourceLoader.LoadEligibleSource(ctx, companyID, request.QuotationID, request.SourceQuotationRevisionID)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintSourceNotFound) {
			return nil, fmt.Errorf("%w: blueprint source not found", repository.ErrBlueprintSourceNotFound)
		}
		if errors.Is(err, repository.ErrBlueprintSourceIneligible) {
			return nil, fmt.Errorf("%w: blueprint source is not eligible", repository.ErrBlueprintSourceIneligible)
		}
		return nil, fmt.Errorf("%w: failed to load blueprint source", ErrBlueprintGenerationInvalidInput)
	}

	skill, err := s.skillResolver.GetActiveSkillByCommand(ctx, companyID.String(), BlueprintSkillCommand)
	if err != nil {
		if errors.Is(err, repository.ErrSkillNotFound) {
			return nil, fmt.Errorf("%w: active blueprint skill is not configured", repository.ErrSkillNotFound)
		}
		return nil, fmt.Errorf("%w: failed to resolve blueprint skill", ErrBlueprintGenerationInvalidInput)
	}
	if skill == nil || !skill.IsActive || skill.DeletedAt != nil {
		return nil, fmt.Errorf("%w: active blueprint skill is not configured", repository.ErrSkillNotFound)
	}

	skillSnapshot, err := BuildBlueprintSkillSnapshot(skill)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to build blueprint skill snapshot", ErrBlueprintGenerationInvalidInput)
	}

	sourceProjection := BlueprintSourceProjection{
		CompanyID:    sourceRecord.CompanyID,
		QuotationID:  sourceRecord.QuotationID,
		RevisionID:   sourceRecord.RevisionID,
		Subject:      sourceRecord.Subject,
		SolutionType: sourceRecord.SolutionType,
		ContentHTML:  sourceRecord.ContentHTML,
	}

	normalized, err := s.builder.Build(sourceProjection, request)
	if err != nil {
		return nil, fmt.Errorf("%w: unable to build blueprint source context", ErrBlueprintGenerationInvalidInput)
	}

	result, err := s.creator.CreateBlueprintGeneration(ctx, repository.CreateBlueprintGenerationParams{
		CompanyID:              companyID,
		QuotationID:            request.QuotationID,
		SourceRevisionID:       request.SourceQuotationRevisionID,
		Title:                  request.Title,
		IdempotencyKey:         request.IdempotencyKey,
		IdempotencyPayloadHash: normalized.IdempotencyPayloadHash,
		SourceContextSnapshot:  normalized.SourceContextSnapshot,
		ContextContent:         normalized.ContextContent,
		SkillSnapshot:          skillSnapshot,
		CreatedBy:              &createdBy,
	})
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintIdempotencyPayloadMismatch) {
			return nil, fmt.Errorf("%w: idempotency payload mismatch", repository.ErrBlueprintIdempotencyPayloadMismatch)
		}
		if errors.Is(err, repository.ErrBlueprintSourceIneligible) {
			return nil, fmt.Errorf("%w: blueprint source is not eligible", repository.ErrBlueprintSourceIneligible)
		}
		if errors.Is(err, repository.ErrBlueprintSourceNotFound) {
			return nil, fmt.Errorf("%w: blueprint source not found", repository.ErrBlueprintSourceNotFound)
		}
		return nil, fmt.Errorf("%w: failed to create blueprint generation", ErrBlueprintGenerationInvalidInput)
	}

	return result, nil
}
