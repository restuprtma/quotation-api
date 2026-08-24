package domain

import (
	"time"

	"github.com/google/uuid"
)

type RateCard struct {
	ID             uuid.UUID  `json:"id" db:"id"`
	CompanyID      uuid.UUID  `json:"company_id" db:"company_id"`
	Code           string     `json:"code" db:"code"`
	Name           string     `json:"name" db:"name"`
	Market         string     `json:"market" db:"market"`
	Currency       string     `json:"currency" db:"currency"`
	EffectiveFrom  time.Time  `json:"effective_from" db:"effective_from"`
	EffectiveUntil *time.Time `json:"effective_until" db:"effective_until"`
	IsActive       bool       `json:"is_active" db:"is_active"`
	CreatedBy      *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}

type RateCardItem struct {
	ID             uuid.UUID `json:"id" db:"id"`
	CompanyID      uuid.UUID `json:"company_id" db:"company_id"`
	RateCardID     uuid.UUID `json:"rate_card_id" db:"rate_card_id"`
	Role           string    `json:"role" db:"role"`
	Seniority      string    `json:"seniority" db:"seniority"`
	TermType       string    `json:"term_type" db:"term_type"`
	AmountPerMonth int64     `json:"amount_per_month" db:"amount_per_month"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
