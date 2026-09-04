package domain

import (
	"time"

	"github.com/google/uuid"
)

// Discipline enum constants for rate card item categorization.
const (
	DisciplineEngineering    = "ENGINEERING"
	DisciplineQA             = "QA"
	DisciplineProduct        = "PRODUCT"
	DisciplineData           = "DATA"
	DisciplineInfrastructure = "INFRASTRUCTURE"
	DisciplineSupport        = "SUPPORT"
)

type RateCard struct {
	ID                    uuid.UUID  `json:"id" db:"id"`
	CompanyID             uuid.UUID  `json:"company_id" db:"company_id"`
	Code                  string     `json:"code" db:"code"`
	Name                  string     `json:"name" db:"name"`
	Market                string     `json:"market" db:"market"`
	ClientTier            string     `json:"client_tier" db:"client_tier"`
	Currency              string     `json:"currency" db:"currency"`
	Description           string     `json:"description" db:"description"`
	ShortTermSurchargePct int        `json:"short_term_surcharge_pct" db:"short_term_surcharge_pct"`
	EnterpriseMarkupPct   int        `json:"enterprise_markup_pct" db:"enterprise_markup_pct"`
	EffectiveFrom         time.Time  `json:"effective_from" db:"effective_from"`
	EffectiveUntil        *time.Time `json:"effective_until" db:"effective_until"`
	IsActive              bool       `json:"is_active" db:"is_active"`
	CreatedBy             *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt             time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at" db:"updated_at"`
}

type RateCardItem struct {
	ID             uuid.UUID `json:"id" db:"id"`
	CompanyID      uuid.UUID `json:"company_id" db:"company_id"`
	RateCardID     uuid.UUID `json:"rate_card_id" db:"rate_card_id"`
	Role           string    `json:"role" db:"role"`
	DisplayName    string    `json:"display_name" db:"display_name"`
	Seniority      string    `json:"seniority" db:"seniority"`
	Discipline     string    `json:"discipline" db:"discipline"`
	TermType       string    `json:"term_type" db:"term_type"`
	AmountPerMonth int64     `json:"amount_per_month" db:"amount_per_month"`

	// Cost breakdown (integer IDR)
	BaseSalary    int64 `json:"base_salary" db:"base_salary"`
	TechLeadFee   int64 `json:"tech_lead_fee" db:"tech_lead_fee"`
	AiRndFee      int64 `json:"ai_rnd_fee" db:"ai_rnd_fee"`
	WorkspaceFee  int64 `json:"workspace_fee" db:"workspace_fee"`
	ManagementFee int64 `json:"management_fee" db:"management_fee"`

	// Role metadata
	EducationRequirement  string `json:"education_requirement" db:"education_requirement"`
	ExperienceRequirement string `json:"experience_requirement" db:"experience_requirement"`

	// Negotiation parameters
	PublishedRate  int64 `json:"published_rate" db:"published_rate"`
	MaxDiscountPct int   `json:"max_discount_pct" db:"max_discount_pct"`
	FloorRate      int64 `json:"floor_rate" db:"floor_rate"`

	// Preparation fees (per-person, one-time)
	KickoffFee     int64 `json:"kickoff_fee" db:"kickoff_fee"`
	RecruitmentFee int64 `json:"recruitment_fee" db:"recruitment_fee"`
	OnboardingFee  int64 `json:"onboarding_fee" db:"onboarding_fee"`

	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// DailyRate computes the daily rate: shortTermMonthlyRate / 22 working days.
func (r *RateCardItem) DailyRate(shortTermMonthlyRate int64) int64 {
	if shortTermMonthlyRate <= 0 {
		return 0
	}
	return shortTermMonthlyRate / 22
}

// HourlyRate computes the base hourly rate: MonthlyRate / 173 hours.
func (r *RateCardItem) HourlyRate() int64 {
	if r.AmountPerMonth <= 0 {
		return 0
	}
	return r.AmountPerMonth / 173
}

// TotalHiringFee returns RecruitmentFee + OnboardingFee.
func (r *RateCardItem) TotalHiringFee() int64 {
	return r.RecruitmentFee + r.OnboardingFee
}
