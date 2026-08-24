package dto

import (
	"github.com/google/uuid"
	"time"
)

type CreateRateCardRequest struct {
	Name        string `json:"name" binding:"required"`
	Market      string `json:"market" binding:"required"`
	Currency    string `json:"currency" binding:"required"`
	Description string `json:"description"`
}

type UpdateRateCardRequest struct {
	Name        *string `json:"name"`
	Market      *string `json:"market"`
	Currency    *string `json:"currency"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"is_active"`
}

type CreateRateCardItemRequest struct {
	RoleName        string `json:"role_name" binding:"required"`
	UnitType        string `json:"unit_type" binding:"required"`
	DefaultUnitRate int64  `json:"default_unit_rate" binding:"required"`
}

type UpdateRateCardItemRequest struct {
	RoleName        *string `json:"role_name"`
	UnitType        *string `json:"unit_type"`
	DefaultUnitRate *int64  `json:"default_unit_rate"`
}

type RateCardItemResponse struct {
	ID              uuid.UUID `json:"id"`
	RateCardID      uuid.UUID `json:"rate_card_id"`
	RoleName        string    `json:"role_name"`
	UnitType        string    `json:"unit_type"`
	DefaultUnitRate int64     `json:"default_unit_rate"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RateCardResponse struct {
	ID          uuid.UUID              `json:"id"`
	CompanyID   uuid.UUID              `json:"company_id"`
	Name        string                 `json:"name"`
	Market      string                 `json:"market"`
	Currency    string                 `json:"currency"`
	Description string                 `json:"description,omitempty"`
	IsActive    bool                   `json:"is_active"`
	Items       []RateCardItemResponse `json:"items,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}
