package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type DBPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

type RateCardRepo interface {
	GetActiveRateCard(ctx context.Context, companyID, market string) (*domain.RateCard, error)
	Create(ctx context.Context, q repository.Execer, rc *domain.RateCard) error
}

type LegalTemplateRepo interface {
	GetActiveTemplate(ctx context.Context, companyID, code string) (*domain.LegalTemplate, error)
	Create(ctx context.Context, q repository.Execer, tmpl *domain.LegalTemplate) error
}

type SettingsService struct {
	db           DBPool
	skillRepo    SkillRepo
	rateCardRepo RateCardRepo
	legalRepo    LegalTemplateRepo
	skillService *SkillService
}

func NewSettingsService(
	db DBPool,
	skillRepo SkillRepo,
	rateCardRepo RateCardRepo,
	legalRepo LegalTemplateRepo,
	skillService *SkillService,
) *SettingsService {
	return &SettingsService{
		db:           db,
		skillRepo:    skillRepo,
		rateCardRepo: rateCardRepo,
		legalRepo:    legalRepo,
		skillService: skillService,
	}
}

func (s *SettingsService) EnsureDefaultAIAssets(ctx context.Context, companyID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		return err
	}

	// 1. Ensure /quotation skill
	_, err = s.skillRepo.GetActiveSkillByCommand(ctx, companyID, "/quotation")
	if errors.Is(err, repository.ErrSkillNotFound) {
		content := GetSafetyEnvelope("quotation_safety_v1.txt")
		skill := &domain.Skill{
			ID:                 uuid.New(),
			CompanyID:          companyUUID,
			Name:               "Quotation Generator",
			Command:            "/quotation",
			ContentInstruction: content,
			IsActive:           true,
		}
		if err := s.skillService.Create(ctx, tx, skill); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, repository.ErrSkillNotFound) {
		return err
	}

	// 2. Ensure /blueprint skill
	_, err = s.skillRepo.GetActiveSkillByCommand(ctx, companyID, "/blueprint")
	if errors.Is(err, repository.ErrSkillNotFound) {
		content := GetSafetyEnvelope("blueprint_safety_v1.txt")
		skill := &domain.Skill{
			ID:                 uuid.New(),
			CompanyID:          companyUUID,
			Name:               "Blueprint Generator",
			Command:            "/blueprint",
			ContentInstruction: content,
			IsActive:           true,
		}
		if err := s.skillService.Create(ctx, tx, skill); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, repository.ErrSkillNotFound) {
		return err
	}

	// 3. Ensure Default Rate Card
	_, err = s.rateCardRepo.GetActiveRateCard(ctx, companyID, "ID")
	if errors.Is(err, repository.ErrRateCardNotFound) {
		now := time.Now()
		rc := &domain.RateCard{
			ID:            uuid.New(),
			CompanyID:     companyUUID,
			Code:          "DEFAULT",
			Name:          "Default Rate Card",
			Market:        "ID",
			Currency:      "IDR",
			EffectiveFrom: now,
			IsActive:      true,
		}
		if err := s.rateCardRepo.Create(ctx, tx, rc); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, repository.ErrRateCardNotFound) {
		return err
	}

	// 4. Ensure Default Legal Template
	_, err = s.legalRepo.GetActiveTemplate(ctx, companyID, "QUOTATION_TERMS")
	if errors.Is(err, repository.ErrLegalTemplateNotFound) {
		tmpl := &domain.LegalTemplate{
			ID:          uuid.New(),
			CompanyID:   companyUUID,
			Code:        "QUOTATION_TERMS",
			Version:     1,
			ContentHTML: "<p>Standard Terms and Conditions.</p>",
			IsActive:    true,
		}
		if err := s.legalRepo.Create(ctx, tx, tmpl); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, repository.ErrLegalTemplateNotFound) {
		return err
	}

	return tx.Commit(ctx)
}
