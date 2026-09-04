package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type RateCardService struct {
	db   *pgxpool.Pool
	repo *repository.RateCardRepository
}

func NewRateCardService(db *pgxpool.Pool, repo *repository.RateCardRepository) *RateCardService {
	return &RateCardService{db: db, repo: repo}
}

func (s *RateCardService) Create(ctx context.Context, companyID string, userID *uuid.UUID, req *dto.CreateRateCardRequest) (*dto.RateCardDetailResponse, error) {
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, fmt.Errorf("invalid company id: %w", err)
	}

	effectiveFrom, err := time.Parse(time.RFC3339, req.EffectiveFrom)
	if err != nil {
		effectiveFrom, err = time.Parse("2006-01-02", req.EffectiveFrom)
		if err != nil {
			return nil, fmt.Errorf("invalid effective_from: %w", err)
		}
	}

	var effectiveUntil *time.Time
	if req.EffectiveUntil != nil && *req.EffectiveUntil != "" {
		t, err := time.Parse(time.RFC3339, *req.EffectiveUntil)
		if err != nil {
			t, err = time.Parse("2006-01-02", *req.EffectiveUntil)
			if err != nil {
				return nil, fmt.Errorf("invalid effective_until: %w", err)
			}
		}
		effectiveUntil = &t
	}

	shortTermSurcharge := 40
	if req.ShortTermSurchargePct != nil {
		shortTermSurcharge = *req.ShortTermSurchargePct
	}

	enterpriseMarkup := 35
	if req.EnterpriseMarkupPct != nil {
		enterpriseMarkup = *req.EnterpriseMarkupPct
	}

	rc := &domain.RateCard{
		ID:                    uuid.New(),
		CompanyID:             companyUUID,
		Code:                  req.Code,
		Name:                  req.Name,
		Market:                req.Market,
		ClientTier:            req.ClientTier,
		Currency:              req.Currency,
		Description:           req.Description,
		ShortTermSurchargePct: shortTermSurcharge,
		EnterpriseMarkupPct:   enterpriseMarkup,
		EffectiveFrom:         effectiveFrom,
		EffectiveUntil:        effectiveUntil,
		IsActive:              true,
		CreatedBy:             userID,
	}

	items := make([]*domain.RateCardItem, len(req.Items))
	for i, itemReq := range req.Items {
		item := &domain.RateCardItem{
			ID:                    uuid.New(),
			CompanyID:             companyUUID,
			RateCardID:            rc.ID,
			Role:                  resolveRole(itemReq.Role),
			DisplayName:           itemReq.DisplayName,
			Seniority:             itemReq.Seniority,
			Discipline:            itemReq.Discipline,
			TermType:              itemReq.TermType,
			AmountPerMonth:        itemReq.AmountPerMonth,
			BaseSalary:            itemReq.BaseSalary,
			TechLeadFee:           itemReq.TechLeadFee,
			AiRndFee:              itemReq.AiRndFee,
			WorkspaceFee:          itemReq.WorkspaceFee,
			ManagementFee:         itemReq.ManagementFee,
			EducationRequirement:  itemReq.EducationRequirement,
			ExperienceRequirement: itemReq.ExperienceRequirement,
			PublishedRate:         itemReq.PublishedRate,
			MaxDiscountPct:        itemReq.MaxDiscountPct,
			FloorRate:             itemReq.FloorRate,
			KickoffFee:            itemReq.KickoffFee,
			RecruitmentFee:        itemReq.RecruitmentFee,
			OnboardingFee:         itemReq.OnboardingFee,
		}
		items[i] = item
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.repo.CreateWithItems(ctx, tx, rc, items); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return mapToDetailResponse(rc, items), nil
}

func (s *RateCardService) GetDetail(ctx context.Context, companyID, id string) (*dto.RateCardDetailResponse, error) {
	rc, items, err := s.repo.GetWithDetails(ctx, id, companyID)
	if err != nil {
		return nil, err
	}
	overtimes, _ := s.repo.GetOvertimeConfigs(ctx, id, companyID)
	infras, _ := s.repo.GetInfrastructurePackages(ctx, id, companyID)
	return mapToDetailResponseWithExtras(rc, items, overtimes, infras), nil
}

func (s *RateCardService) ListAll(ctx context.Context, companyID string, activeOnly bool) ([]*dto.RateCardSummaryResponse, error) {
	cards, err := s.repo.ListAll(ctx, companyID, activeOnly)
	if err != nil {
		return nil, err
	}

	counts := make(map[uuid.UUID]int)
	countQuery := `SELECT rate_card_id, COUNT(*) FROM rate_card_items WHERE company_id = $1 GROUP BY rate_card_id`
	rows, countErr := s.db.Query(ctx, countQuery, companyID)
	if countErr == nil {
		defer rows.Close()
		for rows.Next() {
			var rcID uuid.UUID
			var c int
			if err := rows.Scan(&rcID, &c); err == nil {
				counts[rcID] = c
			}
		}
	}

	res := make([]*dto.RateCardSummaryResponse, len(cards))
	for i, c := range cards {
		res[i] = &dto.RateCardSummaryResponse{
			ID:         c.ID,
			Code:       c.Code,
			Name:       c.Name,
			Market:     c.Market,
			ClientTier: c.ClientTier,
			Currency:   c.Currency,
			IsActive:   c.IsActive,
			ItemCount:  counts[c.ID],
			CreatedAt:  c.CreatedAt,
		}
	}
	return res, nil
}

func (s *RateCardService) Delete(ctx context.Context, companyID, id string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.repo.DeleteItemsByRateCard(ctx, tx, id, companyID); err != nil {
		return fmt.Errorf("delete items: %w", err)
	}

	if err := s.repo.Delete(ctx, tx, id, companyID); err != nil {
		return fmt.Errorf("delete rate card: %w", err)
	}

	return tx.Commit(ctx)
}

func (s *RateCardService) GetActiveRateCard(ctx context.Context, companyID, market string) (*domain.RateCard, error) {
	return s.repo.GetActiveRateCard(ctx, companyID, market)
}

func (s *RateCardService) ListActiveRateCards(ctx context.Context, companyID string) ([]*domain.RateCard, error) {
	return s.repo.ListActiveRateCards(ctx, companyID)
}

// ListGeneratorCards returns only official, active master cards available to
// the tenant. Admin listing remains available through ListAll.
func (s *RateCardService) ListGeneratorCards(ctx context.Context, companyID string) ([]*domain.RateCard, error) {
	return s.repo.ListGeneratorCards(ctx, companyID)
}

func (s *RateCardService) ResolveItem(ctx context.Context, companyID, rateCardID, role, seniority, termType string) (*domain.RateCardItem, error) {
	return s.repo.ResolveItem(ctx, companyID, rateCardID, role, seniority, termType)
}

func (s *RateCardService) GetRateCard(ctx context.Context, companyID, id string) (*domain.RateCard, error) {
	return s.repo.Get(ctx, id, companyID)
}

func mapToDetailResponse(rc *domain.RateCard, items []*domain.RateCardItem) *dto.RateCardDetailResponse {
	return mapToDetailResponseWithExtras(rc, items, nil, nil)
}

func mapToDetailResponseWithExtras(
	rc *domain.RateCard,
	items []*domain.RateCardItem,
	overtimes []*domain.OvertimeRateConfig,
	infras []*domain.InfrastructurePackage,
) *dto.RateCardDetailResponse {
	itemResponses := make([]dto.RateCardItemResponse, len(items))
	for i, item := range items {
		itemResponses[i] = dto.RateCardItemResponse{
			ID:                    item.ID,
			RateCardID:            item.RateCardID,
			Role:                  item.Role,
			DisplayName:           item.DisplayName,
			Seniority:             item.Seniority,
			Discipline:            item.Discipline,
			TermType:              item.TermType,
			AmountPerMonth:        item.AmountPerMonth,
			BaseSalary:            item.BaseSalary,
			TechLeadFee:           item.TechLeadFee,
			AiRndFee:              item.AiRndFee,
			WorkspaceFee:          item.WorkspaceFee,
			ManagementFee:         item.ManagementFee,
			DailyRate:             item.DailyRate(item.AmountPerMonth),
			HourlyRate:            item.HourlyRate(),
			EducationRequirement:  item.EducationRequirement,
			ExperienceRequirement: item.ExperienceRequirement,
			PublishedRate:         item.PublishedRate,
			MaxDiscountPct:        item.MaxDiscountPct,
			FloorRate:             item.FloorRate,
			KickoffFee:            item.KickoffFee,
			RecruitmentFee:        item.RecruitmentFee,
			OnboardingFee:         item.OnboardingFee,
			TotalHiringFee:        item.TotalHiringFee(),
			CreatedAt:             item.CreatedAt,
			UpdatedAt:             item.UpdatedAt,
		}
	}

	var otResponses []dto.OvertimeConfigResponse
	if len(overtimes) > 0 {
		otResponses = make([]dto.OvertimeConfigResponse, len(overtimes))
		for i, ot := range overtimes {
			otResponses[i] = dto.OvertimeConfigResponse{
				ID:                          ot.ID,
				Role:                        ot.Role,
				Seniority:                   ot.Seniority,
				TermType:                    ot.TermType,
				BaseMonthlyRate:             ot.BaseMonthlyRate,
				HourlyRate:                  ot.HourlyRate(),
				HourlyDivisor:               ot.HourlyDivisor,
				WorkdayFirstHourMultiplier:  ot.WorkdayFirstHourMultiplier,
				WorkdaySubsequentMultiplier: ot.WorkdaySubsequentMultiplier,
				HolidayBaseMultiplier:       ot.HolidayBaseMultiplier,
				Holiday9thHourMultiplier:    ot.Holiday9thHourMultiplier,
				Holiday10PlusMultiplier:     ot.Holiday10PlusMultiplier,
			}
		}
	}

	var infraResponses []dto.InfrastructurePackageResponse
	if len(infras) > 0 {
		infraResponses = make([]dto.InfrastructurePackageResponse, len(infras))
		for i, inf := range infras {
			infraResponses[i] = dto.InfrastructurePackageResponse{
				ID:             inf.ID,
				Category:       inf.Category,
				PackageName:    inf.PackageName,
				VCPU:           inf.VCPU,
				RAMGB:          inf.RAMGB,
				StorageGB:      inf.StorageGB,
				AmountPerMonth: inf.AmountPerMonth,
			}
		}
	}

	return &dto.RateCardDetailResponse{
		ID:                    rc.ID,
		CompanyID:             rc.CompanyID,
		Code:                  rc.Code,
		Name:                  rc.Name,
		Market:                rc.Market,
		ClientTier:            rc.ClientTier,
		Currency:              rc.Currency,
		Description:           rc.Description,
		ShortTermSurchargePct: rc.ShortTermSurchargePct,
		EnterpriseMarkupPct:   rc.EnterpriseMarkupPct,
		IsActive:              rc.IsActive,
		EffectiveFrom:         rc.EffectiveFrom,
		EffectiveUntil:        rc.EffectiveUntil,
		Items:                 itemResponses,
		OvertimeConfigs:       otResponses,
		InfraPackages:         infraResponses,
		CreatedAt:             rc.CreatedAt,
		UpdatedAt:             rc.UpdatedAt,
	}
}
