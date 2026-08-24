package domain

import (
	"time"

	"github.com/google/uuid"
)

type LegalTemplate struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	CompanyID   uuid.UUID  `json:"company_id" db:"company_id"`
	Code        string     `json:"code" db:"code"`
	Version     int        `json:"version" db:"version"`
	ContentHTML string     `json:"content_html" db:"content_html"`
	IsActive    bool       `json:"is_active" db:"is_active"`
	ApprovedBy  *uuid.UUID `json:"approved_by" db:"approved_by"`
	ApprovedAt  *time.Time `json:"approved_at" db:"approved_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" db:"updated_at"`
}
