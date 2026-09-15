package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	// ErrBlueprintGenerationInvalidInput indicates an incomplete or unusable create request.
	ErrBlueprintGenerationInvalidInput = errors.New("invalid blueprint generation input")
	// ErrBlueprintGenerationTenantMismatch indicates that the authenticated tenant does not own the source identity.
	ErrBlueprintGenerationTenantMismatch = errors.New("blueprint generation tenant mismatch")
)

// BlueprintContextBuilder normalizes a tenant-scoped source projection for persistence.
type BlueprintContextBuilder interface {
	Build(source BlueprintSourceProjection, request dto.CreateBlueprintGenerationRequest) (NormalizedBlueprintContext, error)
}

// BlueprintGenerationCreator persists the normalized Blueprint generation aggregate.
type BlueprintGenerationCreator interface {
	CreateBlueprintGeneration(ctx context.Context, params repository.CreateBlueprintGenerationParams) (*repository.CreateBlueprintGenerationResult, error)
}

// BlueprintGenerationService coordinates source-context normalization and aggregate creation.
type BlueprintGenerationService struct {
	builder BlueprintContextBuilder
	creator BlueprintGenerationCreator
}

// NewBlueprintGenerationService constructs the service with its narrow dependencies.
func NewBlueprintGenerationService(builder BlueprintContextBuilder, creator BlueprintGenerationCreator) *BlueprintGenerationService {
	return &BlueprintGenerationService{builder: builder, creator: creator}
}

// CreateBlueprint normalizes an authenticated source projection and persists one generation aggregate.
func (s *BlueprintGenerationService) CreateBlueprint(
	ctx context.Context,
	companyID, createdBy uuid.UUID,
	source BlueprintSourceProjection,
	request dto.CreateBlueprintGenerationRequest,
) (*repository.CreateBlueprintGenerationResult, error) {
	if s == nil || s.builder == nil || s.creator == nil {
		return nil, fmt.Errorf("%w: service dependencies are required", ErrBlueprintGenerationInvalidInput)
	}
	if companyID == uuid.Nil || createdBy == uuid.Nil {
		return nil, fmt.Errorf("%w: company and creator IDs are required", ErrBlueprintGenerationInvalidInput)
	}
	if source.CompanyID != companyID {
		return nil, fmt.Errorf("%w: source company does not match authenticated company", ErrBlueprintGenerationTenantMismatch)
	}
	if source.QuotationID == uuid.Nil || source.RevisionID == uuid.Nil || request.QuotationID == uuid.Nil || request.SourceQuotationRevisionID == uuid.Nil {
		return nil, fmt.Errorf("%w: quotation and revision IDs are required", ErrBlueprintGenerationInvalidInput)
	}
	if request.QuotationID != source.QuotationID || request.SourceQuotationRevisionID != source.RevisionID {
		return nil, fmt.Errorf("%w: request source identity does not match authenticated source", ErrBlueprintGenerationTenantMismatch)
	}

	normalized, err := s.builder.Build(source, request)
	if err != nil {
		return nil, fmt.Errorf("%w: unable to build blueprint source context", ErrBlueprintGenerationInvalidInput)
	}

	return s.creator.CreateBlueprintGeneration(ctx, repository.CreateBlueprintGenerationParams{
		CompanyID:              companyID,
		QuotationID:            request.QuotationID,
		SourceRevisionID:       request.SourceQuotationRevisionID,
		Title:                  request.Title,
		IdempotencyKey:         request.IdempotencyKey,
		IdempotencyPayloadHash: normalized.IdempotencyPayloadHash,
		SourceContextSnapshot:  normalized.SourceContextSnapshot,
		ContextContent:         normalized.ContextContent,
		SkillSnapshot:          json.RawMessage("{}"),
		CreatedBy:              &createdBy,
	})
}
