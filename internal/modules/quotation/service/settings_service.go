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
	CreateItem(ctx context.Context, q repository.Execer, item *domain.RateCardItem) error
	CreateItemExpanded(ctx context.Context, q repository.Execer, item *domain.RateCardItem) error
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
	_, err = s.rateCardRepo.GetActiveRateCard(ctx, companyID, "DEFAULT")
	if errors.Is(err, repository.ErrRateCardNotFound) {
		now := time.Now()
		rc := &domain.RateCard{
			ID:                    uuid.New(),
			CompanyID:             companyUUID,
			Code:                  "DEFAULT",
			Name:                  "Default Rate Card",
			Market:                "national",
			ClientTier:            "commercial_national",
			Currency:              "IDR",
			ShortTermSurchargePct: 40,
			EnterpriseMarkupPct:   35,
			EffectiveFrom:         now,
			IsActive:              true,
		}
		if err := s.rateCardRepo.Create(ctx, tx, rc); err != nil {
			return err
		}

		// Seed 14 Venturo standard roles
		type seedSen struct {
			seniority     string
			baseSalary    int64
			techLeadFee   int64
			aiRndFee      int64
			workspaceFee  int64
			managementFee int64
			longTermRate  int64
			shortTermRate int64
			maxDiscount   int
		}
		type seedRoleDef struct {
			role        string
			displayName string
			discipline  string
			education   string
			experience  string
			seniorities []seedSen
		}

		engSeniorities := []seedSen{
			{seniority: "senior", baseSalary: 6800000, techLeadFee: 1030000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 2720000, longTermRate: 11550000, shortTermRate: 16170000, maxDiscount: 5},
			{seniority: "intermediate", baseSalary: 5500000, techLeadFee: 1675000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 1375000, longTermRate: 9550000, shortTermRate: 13370000, maxDiscount: 5},
			{seniority: "associate", baseSalary: 4500000, techLeadFee: 1200000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 1000000, longTermRate: 7700000, shortTermRate: 10780000, maxDiscount: 5},
		}

		analysisSeniorities := []seedSen{
			{seniority: "senior", baseSalary: 10000000, techLeadFee: 1500000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 4200000, longTermRate: 16700000, shortTermRate: 23380000, maxDiscount: 5},
			{seniority: "intermediate", baseSalary: 8000000, techLeadFee: 1200000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 3300000, longTermRate: 13500000, shortTermRate: 18900000, maxDiscount: 5},
			{seniority: "associate", baseSalary: 5500000, techLeadFee: 1000000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 2500000, longTermRate: 10000000, shortTermRate: 14000000, maxDiscount: 5},
		}

		supportSeniorities := []seedSen{
			{seniority: "senior", baseSalary: 6000000, techLeadFee: 1200000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 2000000, longTermRate: 10200000, shortTermRate: 14500000, maxDiscount: 10},
			{seniority: "intermediate", baseSalary: 5000000, techLeadFee: 1000000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 1500000, longTermRate: 8500000, shortTermRate: 12500000, maxDiscount: 10},
			{seniority: "associate", baseSalary: 4000000, techLeadFee: 800000, aiRndFee: 500000, workspaceFee: 500000, managementFee: 1200000, longTermRate: 7000000, shortTermRate: 9800000, maxDiscount: 10},
		}

		rolesDefs := []seedRoleDef{
			{role: "business_analyst", displayName: "Business Analyst", discipline: domain.DisciplineProduct, education: "S1", experience: "> 2 th", seniorities: analysisSeniorities},
			{role: "system_analyst", displayName: "System Analyst", discipline: domain.DisciplineEngineering, education: "S1", experience: "> 2 th", seniorities: analysisSeniorities},
			{role: "frontend_programmer", displayName: "Front End Programmer", discipline: domain.DisciplineEngineering, education: "S1/D4", experience: "> 2 th", seniorities: engSeniorities},
			{role: "backend_programmer", displayName: "Back End Programmer", discipline: domain.DisciplineEngineering, education: "S1/D4", experience: "> 2 th", seniorities: engSeniorities},
			{role: "fullstack_developer", displayName: "Full Stack Developer", discipline: domain.DisciplineEngineering, education: "S1/D4", experience: "> 2 th", seniorities: engSeniorities},
			{role: "mobile_developer", displayName: "Mobile Developer", discipline: domain.DisciplineEngineering, education: "S1/D4", experience: "> 2 th", seniorities: engSeniorities},
			{role: "devops", displayName: "DevOps Engineer", discipline: domain.DisciplineInfrastructure, education: "S1/D4", experience: "> 2 th", seniorities: engSeniorities},
			{role: "data_engineer", displayName: "Data Engineer", discipline: domain.DisciplineData, education: "S1/D4", experience: "> 2 th", seniorities: analysisSeniorities},
			{role: "data_analyst", displayName: "Data Analyst", discipline: domain.DisciplineData, education: "S1/D4", experience: "> 2 th", seniorities: analysisSeniorities},
			{role: "ui_ux_designer", displayName: "UI/UX Designer", discipline: domain.DisciplineProduct, education: "D3/S1", experience: "> 2 th", seniorities: engSeniorities},
			{role: "automated_tester", displayName: "Automated Tester (QA Automation)", discipline: domain.DisciplineQA, education: "D3/S1", experience: "< 2 th", seniorities: engSeniorities},
			{role: "functional_tester", displayName: "Functional Tester (Manual QA)", discipline: domain.DisciplineQA, education: "D3", experience: "< 2 th", seniorities: supportSeniorities},
			{role: "technical_writer", displayName: "Technical Writer", discipline: domain.DisciplineSupport, education: "D3", experience: "> 2 th", seniorities: supportSeniorities},
			{role: "pentester", displayName: "Penetration Tester", discipline: domain.DisciplineInfrastructure, education: "S1/Sertifikasi", experience: "> 2 th", seniorities: engSeniorities},
		}

		for _, rDef := range rolesDefs {
			for _, sen := range rDef.seniorities {
				// Long term item
				ltFloor := sen.longTermRate - (sen.longTermRate * int64(sen.maxDiscount) / 100)
				ltItem := &domain.RateCardItem{
					ID:                    uuid.New(),
					CompanyID:             companyUUID,
					RateCardID:            rc.ID,
					Role:                  rDef.role,
					DisplayName:           rDef.displayName,
					Seniority:             sen.seniority,
					Discipline:            rDef.discipline,
					TermType:              "long_term",
					AmountPerMonth:        sen.longTermRate,
					BaseSalary:            sen.baseSalary,
					TechLeadFee:           sen.techLeadFee,
					AiRndFee:              sen.aiRndFee,
					WorkspaceFee:          sen.workspaceFee,
					ManagementFee:         sen.managementFee,
					EducationRequirement:  rDef.education,
					ExperienceRequirement: rDef.experience,
					PublishedRate:         sen.longTermRate,
					MaxDiscountPct:        sen.maxDiscount,
					FloorRate:             ltFloor,
				}
				if err := s.rateCardRepo.CreateItemExpanded(ctx, tx, ltItem); err != nil {
					return err
				}

				// Short term item
				stFloor := sen.shortTermRate - (sen.shortTermRate * int64(sen.maxDiscount) / 100)
				stItem := &domain.RateCardItem{
					ID:                    uuid.New(),
					CompanyID:             companyUUID,
					RateCardID:            rc.ID,
					Role:                  rDef.role,
					DisplayName:           rDef.displayName,
					Seniority:             sen.seniority,
					Discipline:            rDef.discipline,
					TermType:              "short_term",
					AmountPerMonth:        sen.shortTermRate,
					BaseSalary:            sen.baseSalary,
					TechLeadFee:           sen.techLeadFee,
					AiRndFee:              sen.aiRndFee,
					WorkspaceFee:          sen.workspaceFee,
					ManagementFee:         sen.managementFee,
					EducationRequirement:  rDef.education,
					ExperienceRequirement: rDef.experience,
					PublishedRate:         sen.shortTermRate,
					MaxDiscountPct:        sen.maxDiscount,
					FloorRate:             stFloor,
				}
				if err := s.rateCardRepo.CreateItemExpanded(ctx, tx, stItem); err != nil {
					return err
				}
			}
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
