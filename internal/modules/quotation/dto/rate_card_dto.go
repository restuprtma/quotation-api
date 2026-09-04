package dto

import (
	"time"

	"github.com/google/uuid"
)

// ── Request DTOs ────────────────────────────────────────────

type CreateRateCardRequest struct {
	Code                  string                      `json:"code" binding:"required"`
	Name                  string                      `json:"name" binding:"required"`
	Market                string                      `json:"market" binding:"required,oneof=national international"`
	ClientTier            string                      `json:"client_tier" binding:"required,oneof=commercial_national government_bumn international"`
	Currency              string                      `json:"currency" binding:"required"`
	Description           string                      `json:"description"`
	ShortTermSurchargePct *int                        `json:"short_term_surcharge_pct"`
	EnterpriseMarkupPct   *int                        `json:"enterprise_markup_pct"`
	EffectiveFrom         string                      `json:"effective_from" binding:"required"`
	EffectiveUntil        *string                     `json:"effective_until"`
	Items                 []CreateRateCardItemRequest `json:"items" binding:"required,min=1,dive"`
}

type UpdateRateCardRequest struct {
	Name                  *string `json:"name"`
	Market                *string `json:"market"`
	ClientTier            *string `json:"client_tier"`
	Currency              *string `json:"currency"`
	Description           *string `json:"description"`
	ShortTermSurchargePct *int    `json:"short_term_surcharge_pct"`
	EnterpriseMarkupPct   *int    `json:"enterprise_markup_pct"`
	IsActive              *bool   `json:"is_active"`
}

type CreateRateCardItemRequest struct {
	Role                  string `json:"role" binding:"required"`
	DisplayName           string `json:"display_name" binding:"required"`
	Seniority             string `json:"seniority" binding:"required,oneof=associate intermediate senior"`
	Discipline            string `json:"discipline" binding:"required,oneof=ENGINEERING QA PRODUCT DATA INFRASTRUCTURE SUPPORT"`
	TermType              string `json:"term_type" binding:"required,oneof=short_term long_term"`
	AmountPerMonth        int64  `json:"amount_per_month" binding:"required,min=1"`
	BaseSalary            int64  `json:"base_salary"`
	TechLeadFee           int64  `json:"tech_lead_fee"`
	AiRndFee              int64  `json:"ai_rnd_fee"`
	WorkspaceFee          int64  `json:"workspace_fee"`
	ManagementFee         int64  `json:"management_fee"`
	EducationRequirement  string `json:"education_requirement"`
	ExperienceRequirement string `json:"experience_requirement"`
	PublishedRate         int64  `json:"published_rate"`
	MaxDiscountPct        int    `json:"max_discount_pct"`
	FloorRate             int64  `json:"floor_rate"`
	KickoffFee            int64  `json:"kickoff_fee"`
	RecruitmentFee        int64  `json:"recruitment_fee"`
	OnboardingFee         int64  `json:"onboarding_fee"`
}

type UpdateRateCardItemRequest struct {
	AmountPerMonth *int64  `json:"amount_per_month"`
	BaseSalary     *int64  `json:"base_salary"`
	TechLeadFee    *int64  `json:"tech_lead_fee"`
	AiRndFee       *int64  `json:"ai_rnd_fee"`
	WorkspaceFee   *int64  `json:"workspace_fee"`
	ManagementFee  *int64  `json:"management_fee"`
	PublishedRate  *int64  `json:"published_rate"`
	MaxDiscountPct *int    `json:"max_discount_pct"`
	FloorRate      *int64  `json:"floor_rate"`
	KickoffFee     *int64  `json:"kickoff_fee"`
	RecruitmentFee *int64  `json:"recruitment_fee"`
	OnboardingFee  *int64  `json:"onboarding_fee"`
}

// ── Response DTOs ───────────────────────────────────────────

type RateCardSummaryResponse struct {
	ID         uuid.UUID `json:"id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
	Market     string    `json:"market"`
	ClientTier string    `json:"client_tier"`
	Currency   string    `json:"currency"`
	IsActive   bool      `json:"is_active"`
	ItemCount  int       `json:"item_count"`
	CreatedAt  time.Time `json:"created_at"`
}

type RateCardDetailResponse struct {
	ID                    uuid.UUID                       `json:"id"`
	CompanyID             uuid.UUID                       `json:"company_id"`
	Code                  string                          `json:"code"`
	Name                  string                          `json:"name"`
	Market                string                          `json:"market"`
	ClientTier            string                          `json:"client_tier"`
	Currency              string                          `json:"currency"`
	Description           string                          `json:"description"`
	ShortTermSurchargePct int                             `json:"short_term_surcharge_pct"`
	EnterpriseMarkupPct   int                             `json:"enterprise_markup_pct"`
	IsActive              bool                            `json:"is_active"`
	EffectiveFrom         time.Time                       `json:"effective_from"`
	EffectiveUntil        *time.Time                      `json:"effective_until"`
	Items                 []RateCardItemResponse          `json:"items"`
	OvertimeConfigs       []OvertimeConfigResponse        `json:"overtime_configs,omitempty"`
	InfraPackages         []InfrastructurePackageResponse `json:"infra_packages,omitempty"`
	CreatedAt             time.Time                       `json:"created_at"`
	UpdatedAt             time.Time                       `json:"updated_at"`
}

type RateCardItemResponse struct {
	ID                    uuid.UUID `json:"id"`
	RateCardID            uuid.UUID `json:"rate_card_id"`
	Role                  string    `json:"role"`
	DisplayName           string    `json:"display_name"`
	Seniority             string    `json:"seniority"`
	Discipline            string    `json:"discipline"`
	TermType              string    `json:"term_type"`
	AmountPerMonth        int64     `json:"amount_per_month"`
	BaseSalary            int64     `json:"base_salary"`
	TechLeadFee           int64     `json:"tech_lead_fee"`
	AiRndFee              int64     `json:"ai_rnd_fee"`
	WorkspaceFee          int64     `json:"workspace_fee"`
	ManagementFee         int64     `json:"management_fee"`
	DailyRate             int64     `json:"daily_rate"`
	HourlyRate            int64     `json:"hourly_rate"`
	EducationRequirement  string    `json:"education_requirement"`
	ExperienceRequirement string    `json:"experience_requirement"`
	PublishedRate         int64     `json:"published_rate"`
	MaxDiscountPct        int       `json:"max_discount_pct"`
	FloorRate             int64     `json:"floor_rate"`
	KickoffFee            int64     `json:"kickoff_fee"`
	RecruitmentFee        int64     `json:"recruitment_fee"`
	OnboardingFee         int64     `json:"onboarding_fee"`
	TotalHiringFee        int64     `json:"total_hiring_fee"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type OvertimeConfigResponse struct {
	ID                          uuid.UUID `json:"id"`
	Role                        string    `json:"role"`
	Seniority                   string    `json:"seniority"`
	TermType                    string    `json:"term_type"`
	BaseMonthlyRate             int64     `json:"base_monthly_rate"`
	HourlyRate                  int64     `json:"hourly_rate"`
	HourlyDivisor               int       `json:"hourly_divisor"`
	WorkdayFirstHourMultiplier  int       `json:"workday_first_hour_multiplier,omitempty"`
	WorkdaySubsequentMultiplier int       `json:"workday_subsequent_multiplier,omitempty"`
	HolidayBaseMultiplier       int       `json:"holiday_base_multiplier,omitempty"`
	Holiday9thHourMultiplier    int       `json:"holiday_9th_hour_multiplier,omitempty"`
	Holiday10PlusMultiplier     int       `json:"holiday_10plus_multiplier,omitempty"`
}

type InfrastructurePackageResponse struct {
	ID             uuid.UUID `json:"id"`
	Category       string    `json:"category"`
	PackageName    string    `json:"package_name"`
	VCPU           int       `json:"vcpu"`
	RAMGB          int       `json:"ram_gb"`
	StorageGB      int       `json:"storage_gb"`
	AmountPerMonth int64     `json:"amount_per_month"`
}

// ── Legacy compatibility alias ──────────────────────────────
type RateCardResponse = RateCardDetailResponse
