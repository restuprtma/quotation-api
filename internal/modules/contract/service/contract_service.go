package service

import (
	"context"
	"errors"

	"venturo-skeleton-go/internal/modules/contract/domain"
	"venturo-skeleton-go/internal/modules/contract/repository"
	quotation_repo "venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrQuotationNotSigned = errors.New("contract can only be created from a signed quotation")
	ErrContractExists     = errors.New("contract already exists for this quotation")
	ErrContractNotFound   = errors.New("contract not found")
	ErrInvalidStatus      = errors.New("invalid contract status for this operation")
)

type ContractService interface {
	CreateContract(ctx context.Context, companyID, quotationID, createdBy string) (*domain.Contract, error)
	GetContract(ctx context.Context, id, companyID string) (*domain.Contract, error)
	GetPublicContract(ctx context.Context, token string) (*domain.Contract, error)
	ListContracts(ctx context.Context, companyID string) ([]domain.Contract, error)
	UpdateContract(ctx context.Context, id, companyID string, param domain.UpdateContractParam) error
	ShareContract(ctx context.Context, id, companyID string) (*domain.Contract, error)
	SignContract(ctx context.Context, token string, signatureURL string, ip string) error
}

type contractService struct {
	repo          repository.ContractRepository
	quotationRepo *quotation_repo.Repository
	revisionRepo  *quotation_repo.RevisionRepository
}

func NewContractService(repo repository.ContractRepository, qRepo *quotation_repo.Repository, rRepo *quotation_repo.RevisionRepository) ContractService {
	return &contractService{
		repo:          repo,
		quotationRepo: qRepo,
		revisionRepo:  rRepo,
	}
}

func (s *contractService) CreateContract(ctx context.Context, companyID, quotationID, createdBy string) (*domain.Contract, error) {
	// 1. Fetch quotation
	q, err := s.quotationRepo.GetQuotationByID(ctx, quotationID, companyID)
	if err != nil {
		return nil, err
	}

	// 2. Fetch signed revision
	rev, err := s.revisionRepo.GetSignedRevision(ctx, quotationID, companyID)
	if err != nil {
		if errors.Is(err, quotation_repo.ErrRevisionNotFound) {
			return nil, ErrQuotationNotSigned
		}
		return nil, err
	}

	// Verify exact signed indicators: signed_at, signature_data, and signed_content_hash
	if rev.SignedAt == nil || rev.SignatureData == nil || rev.SignedContentHash == nil || *rev.SignedContentHash == "" {
		return nil, ErrQuotationNotSigned
	}

	// 3. Check if contract already exists
	existing, err := s.repo.GetByQuotationID(ctx, quotationID, companyID)
	if err == nil && existing != nil {
		return nil, ErrContractExists
	}

	param := domain.CreateContractParam{
		CompanyID:      companyID,
		QuotationID:    quotationID,
		RevisionID:     rev.ID.String(),
		ContractNumber: "C-" + q.QuotationNumber,
		ClientSnapshot: nil, // will be handled if needed
		CreatedBy:      &createdBy,
	}

	return s.repo.Create(ctx, param)
}

func (s *contractService) GetContract(ctx context.Context, id, companyID string) (*domain.Contract, error) {
	return s.repo.GetByID(ctx, id, companyID)
}

func (s *contractService) GetPublicContract(ctx context.Context, token string) (*domain.Contract, error) {
	return s.repo.GetByPublicToken(ctx, token)
}

func (s *contractService) ListContracts(ctx context.Context, companyID string) ([]domain.Contract, error) {
	return s.repo.ListByCompany(ctx, companyID)
}

func (s *contractService) UpdateContract(ctx context.Context, id, companyID string, param domain.UpdateContractParam) error {
	c, err := s.repo.GetByID(ctx, id, companyID)
	if err != nil {
		return err
	}
	if c.Status != domain.ContractStatusDraft {
		return ErrInvalidStatus
	}

	return s.repo.Update(ctx, id, companyID, param)
}

func (s *contractService) ShareContract(ctx context.Context, id, companyID string) (*domain.Contract, error) {
	c, err := s.repo.GetByID(ctx, id, companyID)
	if err != nil {
		return nil, err
	}
	if c.Status != domain.ContractStatusDraft {
		return nil, ErrInvalidStatus
	}

	err = s.repo.UpdateStatus(ctx, id, companyID, domain.ContractStatusShared)
	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, id, companyID)
}

func (s *contractService) SignContract(ctx context.Context, token string, signatureURL string, ip string) error {
	return s.repo.Sign(ctx, token, signatureURL, ip)
}
