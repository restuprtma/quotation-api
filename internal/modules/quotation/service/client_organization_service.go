package service

import (
	"context"
	"errors"
	"unicode/utf8"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	// ErrInvalidClientOrgName indicates the provided name is empty or exceeds the maximum length.
	ErrInvalidClientOrgName = errors.New("client organization name is required and must not exceed 255 characters")
	// ErrClientOrganizationDuplicate indicates an organization with the same normalized name already exists.
	ErrClientOrganizationDuplicate = repository.ErrClientOrganizationDuplicate
)

// ClientOrganizationRepository defines the database interactions for client organizations.
type ClientOrganizationRepository interface {
	Create(ctx context.Context, org *domain.ClientOrganization) error
	List(ctx context.Context, companyID uuid.UUID) ([]*domain.ClientOrganization, error)
	FindByNormalizedName(ctx context.Context, companyID uuid.UUID, normalizedName string) (*domain.ClientOrganization, error)
}

// ClientOrganizationService handles business logic for company-scoped client organizations.
type ClientOrganizationService struct {
	repo ClientOrganizationRepository
}

func NewClientOrganizationService(repo ClientOrganizationRepository) *ClientOrganizationService {
	return &ClientOrganizationService{repo: repo}
}

// Create validates, normalizes, checks duplicates, and creates a company-scoped client organization.
func (s *ClientOrganizationService) Create(
	ctx context.Context,
	companyID uuid.UUID,
	createdBy *uuid.UUID,
	req dto.CreateClientOrganizationRequest,
) (*dto.ClientOrganizationResponse, error) {
	displayName, normalizedName := domain.NormalizeClientOrganizationName(req.Name)
	if displayName == "" || utf8.RuneCountInString(displayName) > 255 {
		return nil, ErrInvalidClientOrgName
	}

	existing, err := s.repo.FindByNormalizedName(ctx, companyID, normalizedName)
	if err == nil && existing != nil {
		return nil, ErrClientOrganizationDuplicate
	}
	if err != nil && !errors.Is(err, repository.ErrClientOrganizationNotFound) {
		return nil, err
	}

	org := &domain.ClientOrganization{
		CompanyID:      companyID,
		ID:             uuid.New(),
		Name:           displayName,
		NormalizedName: normalizedName,
		CreatedBy:      createdBy,
	}

	if err := s.repo.Create(ctx, org); err != nil {
		return nil, err
	}

	return toClientOrganizationResponse(org), nil
}

// List returns all active client organizations for the caller's company.
func (s *ClientOrganizationService) List(ctx context.Context, companyID uuid.UUID) (*dto.ClientOrganizationListResponse, error) {
	orgs, err := s.repo.List(ctx, companyID)
	if err != nil {
		return nil, err
	}

	items := make([]dto.ClientOrganizationResponse, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, *toClientOrganizationResponse(org))
	}

	return &dto.ClientOrganizationListResponse{Items: items}, nil
}

func toClientOrganizationResponse(org *domain.ClientOrganization) *dto.ClientOrganizationResponse {
	return &dto.ClientOrganizationResponse{
		ID:             org.ID,
		CompanyID:      org.CompanyID,
		Name:           org.Name,
		NormalizedName: org.NormalizedName,
		CreatedBy:      org.CreatedBy,
		CreatedAt:      org.CreatedAt,
		UpdatedAt:      org.UpdatedAt,
	}
}
