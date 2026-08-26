package service

import (
	"context"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type RateCardService struct {
	repo *repository.RateCardRepository
}

func NewRateCardService(repo *repository.RateCardRepository) *RateCardService {
	return &RateCardService{repo: repo}
}

func (s *RateCardService) GetActiveRateCard(ctx context.Context, companyID, market string) (*domain.RateCard, error) {
	return s.repo.GetActiveRateCard(ctx, companyID, market)
}

func (s *RateCardService) ListActiveRateCards(ctx context.Context, companyID string) ([]*domain.RateCard, error) {
	return s.repo.ListActiveRateCards(ctx, companyID)
}

func (s *RateCardService) GetRateCard(ctx context.Context, companyID, id string) (*domain.RateCard, error) {
	return s.repo.Get(ctx, id, companyID)
}
