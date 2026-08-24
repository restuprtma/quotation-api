package service

import (
	"context"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type LegalTemplateService struct {
	repo *repository.LegalTemplateRepository
}

func NewLegalTemplateService(repo *repository.LegalTemplateRepository) *LegalTemplateService {
	return &LegalTemplateService{repo: repo}
}

func (s *LegalTemplateService) GetActiveTemplate(ctx context.Context, companyID, code string) (*domain.LegalTemplate, error) {
	return s.repo.GetActiveTemplate(ctx, companyID, code)
}
