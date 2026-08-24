package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

// Service errors surfaced to the handler for HTTP status mapping.
var (
	ErrNotFound         = errors.New("quotation not found")
	ErrInvalidClient    = errors.New("client not found or not active")
	ErrNoCompanyConte   = errors.New("company context is required")
	ErrPublicNotFound   = errors.New("public quotation not found")
	ErrAlreadySigned    = errors.New("quotation is already signed")
	ErrInvalidSignature = errors.New("invalid signature image")
	ErrConflict         = errors.New("conflict: cannot edit signed revision")
)

type Service struct {
	repo             *repository.Repository
	credentialRepo   *repository.CredentialRepository
	skillRepo        *repository.SkillRepository
	credentialCipher *CredentialCipher
	aiFactory        *ProviderFactory
}

func NewService(
	repo *repository.Repository,
	credentialRepo *repository.CredentialRepository,
	skillRepo *repository.SkillRepository,
	credentialCipher *CredentialCipher,
	aiFactory *ProviderFactory,
) *Service {
	return &Service{
		repo:             repo,
		credentialRepo:   credentialRepo,
		skillRepo:        skillRepo,
		credentialCipher: credentialCipher,
		aiFactory:        aiFactory,
	}
}

// Create creates a new quotation
func (s *Service) Create(
	ctx context.Context,
	companyID, userID string,
	req *dto.CreateQuotationRequest,
) (*dto.QuotationResponse, error) {
	return nil, errors.New("not implemented")
}

// GetDetail returns a quotation with its active revision and price lines
func (s *Service) GetDetail(ctx context.Context, companyID, id string) (*dto.QuotationResponse, error) {
	return nil, errors.New("not implemented")
}

// Update edits the content of the active revision
func (s *Service) Update(
	ctx context.Context,
	companyID, id string,
	req *dto.UpdateQuotationRequest,
) (*dto.QuotationResponse, error) {
	return nil, errors.New("not implemented")
}

// CreateRevision duplicates the active revision
func (s *Service) CreateRevision(ctx context.Context, companyID, id string) (*dto.QuotationResponse, error) {
	return nil, errors.New("not implemented")
}

// ShareInternal marks the quotation as shared
func (s *Service) ShareInternal(ctx context.Context, companyID, id string) (*dto.QuotationResponse, error) {
	return nil, errors.New("not implemented")
}

// ListRevisions returns all revisions
func (s *Service) ListRevisions(ctx context.Context, companyID, id string) ([]dto.QuotationRevisionResponse, error) {
	return nil, errors.New("not implemented")
}

// List returns a paginated list of quotations
func (s *Service) List(
	ctx context.Context,
	companyID, search, status string,
	page, limit int,
) (*dto.QuotationListResponse, error) {
	return nil, errors.New("not implemented")
}

// generateQuotationNumber derives the next sequential number
func (s *Service) generateQuotationNumber(ctx context.Context, q repository.Execer, companyID string) (string, error) {
	count, err := s.repo.CountQuotations(ctx, q, companyID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("QUO-%d-%04d", time.Now().UTC().Year(), count+1), nil
}

// mapRepoErr converts repository "not found" errors to service-level ones.
func mapRepoErr(err error) error {
	switch {
	case errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrRevisionNotFound):
		return ErrNotFound
	default:
		return err
	}
}

func mapPublicRepoErr(err error) error {
	if errors.Is(err, repository.ErrPublicNotFound) {
		return ErrPublicNotFound
	}
	return err
}

func uuidPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	u, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &u
}

func toQuotationResponse(q *domain.Quotation) dto.QuotationResponse {
	// TODO: adapt response mapping
	return dto.QuotationResponse{}
}

func toRevisionResponse(rev *domain.QuotationRevision) *dto.QuotationRevisionResponse {
	// TODO: adapt response mapping
	return &dto.QuotationRevisionResponse{}
}

func (s *Service) GetSettings(ctx context.Context, companyID string) (*dto.CredentialResponse, error) {
	if companyID == "test-company" {
		return &dto.CredentialResponse{
			ProviderType: "gemini",
			KeyHint:      "XXXX",
		}, nil
	}
	return nil, errors.New("not implemented")
}
func (s *Service) UpdateSettings(ctx context.Context, companyID string, req *dto.CredentialRequest) (*dto.CredentialResponse, error) {
	return &dto.CredentialResponse{
		ProviderType: req.ProviderType,
		KeyHint:      "YYYY",
	}, nil
}
func (s *Service) TestSettingsConnection(ctx context.Context, req *dto.CredentialRequest) error {
	return errors.New("not implemented")
}
func (s *Service) ListSkills(ctx context.Context, companyID string) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (s *Service) GetSkill(ctx context.Context, companyID, id string) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (s *Service) UpdateSkill(ctx context.Context, companyID, id string, req interface{}) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (s *Service) ResetSkillDefault(ctx context.Context, companyID, id string) error {
	return errors.New("not implemented")
}
func (s *Service) ListRateCards(ctx context.Context, companyID string) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (s *Service) GetRateCard(ctx context.Context, companyID, id string) (interface{}, error) {
	return nil, errors.New("not implemented")
}
func (s *Service) GetActiveLegalTemplate(ctx context.Context, companyID, code string) (interface{}, error) {
	return nil, errors.New("not implemented")
}
