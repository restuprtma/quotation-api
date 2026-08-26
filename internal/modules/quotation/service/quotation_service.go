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
	ErrNoCompanyContext = errors.New("company context is required")
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
	settingsSvc      *SettingsService
	rateCardSvc      *RateCardService
	legalTemplateSvc *LegalTemplateService
}

func NewService(
	repo *repository.Repository,
	credentialRepo *repository.CredentialRepository,
	skillRepo *repository.SkillRepository,
	credentialCipher *CredentialCipher,
	aiFactory *ProviderFactory,
	settingsSvc *SettingsService,
	rateCardSvc *RateCardService,
	legalTemplateSvc *LegalTemplateService,
) *Service {
	return &Service{
		repo:             repo,
		credentialRepo:   credentialRepo,
		skillRepo:        skillRepo,
		credentialCipher: credentialCipher,
		aiFactory:        aiFactory,
		settingsSvc:      settingsSvc,
		rateCardSvc:      rateCardSvc,
		legalTemplateSvc: legalTemplateSvc,
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
	_ = s.settingsSvc.EnsureDefaultAIAssets(ctx, companyID)
	if companyID == "" {
		return nil, ErrNoCompanyContext
	}

	if _, err := uuid.Parse(companyID); err != nil {
		return nil, errors.New("invalid company id")
	}

	cred, err := s.credentialRepo.GetActiveCredential(ctx, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrCredentialNotFound) {
			return &dto.CredentialResponse{}, nil
		}
		return nil, err
	}

	return &dto.CredentialResponse{
		ID:                   cred.ID,
		CompanyID:            cred.CompanyID,
		Provider:             cred.Provider,
		Model:                "", // Model not currently stored in cred entity
		KeyHint:              cred.KeyHint,
		IsActive:             cred.IsActive,
		CredentialConfigured: true,
		CreatedAt:            cred.CreatedAt,
		UpdatedAt:            cred.UpdatedAt,
	}, nil
}

func (s *Service) UpdateSettings(ctx context.Context, companyID string, req *dto.CredentialRequest) (*dto.CredentialResponse, error) {
	if companyID == "" {
		return nil, ErrNoCompanyContext
	}

	compUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, errors.New("invalid company id")
	}

	cred, err := s.credentialRepo.GetActiveCredential(ctx, companyID)
	isNew := false
	if err != nil {
		if errors.Is(err, repository.ErrCredentialNotFound) {
			isNew = true
			cred = &domain.Credential{
				ID:        uuid.New(),
				CompanyID: compUUID,
				IsActive:  true,
			}
		} else {
			return nil, err
		}
	}

	cred.Provider = req.ProviderType

	if req.APIKey != "" {
		encKey, err := s.credentialCipher.Encrypt(req.APIKey)
		if err != nil {
			return nil, errors.New("failed to encrypt api key")
		}
		cred.EncryptedAPIKey = encKey

		if len(req.APIKey) > 4 {
			cred.KeyHint = req.APIKey[len(req.APIKey)-4:]
		} else {
			cred.KeyHint = req.APIKey
		}
	}

	q := s.repo.Pool()
	if isNew {
		if err := s.credentialRepo.Create(ctx, q, cred); err != nil {
			return nil, err
		}
	} else {
		if err := s.credentialRepo.Update(ctx, q, cred); err != nil {
			return nil, err
		}
	}

	return &dto.CredentialResponse{
		ID:                   cred.ID,
		CompanyID:            cred.CompanyID,
		Provider:             cred.Provider,
		Model:                "", // Not mapped to cred yet
		KeyHint:              cred.KeyHint,
		IsActive:             cred.IsActive,
		CredentialConfigured: true,
		CreatedAt:            cred.CreatedAt,
		UpdatedAt:            cred.UpdatedAt,
	}, nil
}
const ErrConnectionFailedMsg = "Failed to connect to AI provider. Check your credentials or model name"

func (s *Service) TestSettingsConnection(ctx context.Context, companyID string, req *dto.CredentialRequest) error {
	apiKey := req.APIKey

	if apiKey == "" {
		if companyID == "" {
			return ErrNoCompanyContext
		}

		cred, err := s.credentialRepo.GetActiveCredential(ctx, companyID)
		if err != nil {
			return errors.New(ErrConnectionFailedMsg)
		}

		apiKey, err = s.credentialCipher.Decrypt(cred.EncryptedAPIKey)
		if err != nil {
			return errors.New(ErrConnectionFailedMsg)
		}
	}

	cfg := ProviderConfig{
		ProviderName:    req.ProviderType,
		DecryptedAPIKey: apiKey,
		Model:           req.Model,
	}

	provider, err := s.aiFactory.New(ctx, cfg)
	if err != nil {
		return errors.New(ErrConnectionFailedMsg)
	}

	_, err = provider.Generate(ctx, GenerateRequest{Prompt: "Ping. Reply with 'Pong'."})
	if err != nil {
		return errors.New(ErrConnectionFailedMsg)
	}

	return nil
}
func (s *Service) ListSkills(ctx context.Context, companyID string) (interface{}, error) {
	_ = s.settingsSvc.EnsureDefaultAIAssets(ctx, companyID)
	skills, err := s.skillRepo.ListActiveSkills(ctx, companyID)
	if err != nil {
		return nil, err
	}
	res := []dto.SkillResponse{}
	for _, skill := range skills {
		res = append(res, dto.SkillResponse{
			ID:           skill.ID,
			CompanyID:    skill.CompanyID,
			Command:      skill.Command,
			Name:         skill.Name,
			SystemPrompt: skill.ContentInstruction,
			IsActive:     skill.IsActive,
			CreatedAt:    skill.CreatedAt,
			UpdatedAt:    skill.UpdatedAt,
		})
	}
	return res, nil
}

func (s *Service) GetSkill(ctx context.Context, companyID, id string) (interface{}, error) {
	skill, err := s.skillRepo.Get(ctx, id, companyID)
	if err != nil {
		return nil, err
	}
	return dto.SkillResponse{
		ID:           skill.ID,
		CompanyID:    skill.CompanyID,
		Command:      skill.Command,
		Name:         skill.Name,
		SystemPrompt: skill.ContentInstruction,
		IsActive:     skill.IsActive,
		CreatedAt:    skill.CreatedAt,
		UpdatedAt:    skill.UpdatedAt,
	}, nil
}

func (s *Service) UpdateSkill(ctx context.Context, companyID, id string, req *dto.UpdateSkillRequest) (interface{}, error) {
	skill, err := s.skillRepo.Get(ctx, id, companyID)
	if err != nil {
		return nil, err
	}

	if req.SystemPrompt != nil {
		skill.ContentInstruction = *req.SystemPrompt
	}
	if req.IsActive != nil {
		skill.IsActive = *req.IsActive
	}

	q := s.repo.Pool()
	if err := s.skillRepo.Update(ctx, q, skill); err != nil {
		return nil, err
	}

	return dto.SkillResponse{
		ID:           skill.ID,
		CompanyID:    skill.CompanyID,
		Command:      skill.Command,
		Name:         skill.Name,
		SystemPrompt: skill.ContentInstruction,
		IsActive:     skill.IsActive,
		CreatedAt:    skill.CreatedAt,
		UpdatedAt:    skill.UpdatedAt,
	}, nil
}

func (s *Service) ResetSkillDefault(ctx context.Context, companyID, id string) error {
	skill, err := s.skillRepo.Get(ctx, id, companyID)
	if err != nil {
		return err
	}

	defaultPrompt := GetSafetyEnvelope(skill.Command + ".txt")
	if defaultPrompt == "" {
		return errors.New("default prompt not found")
	}
	skill.ContentInstruction = defaultPrompt

	q := s.repo.Pool()
	return s.skillRepo.Update(ctx, q, skill)
}
func (s *Service) ListRateCards(ctx context.Context, companyID string) (interface{}, error) {
	return s.rateCardSvc.ListActiveRateCards(ctx, companyID)
}

func (s *Service) GetRateCard(ctx context.Context, companyID, id string) (interface{}, error) {
	return s.rateCardSvc.GetRateCard(ctx, companyID, id)
}

func (s *Service) ListLegalTemplates(ctx context.Context, companyID string) (interface{}, error) {
	return s.legalTemplateSvc.ListActiveTemplates(ctx, companyID)
}

func (s *Service) GetActiveLegalTemplate(ctx context.Context, companyID, code string) (interface{}, error) {
	return s.legalTemplateSvc.GetActiveTemplate(ctx, companyID, code)
}
