package domain

import (
	"time"

	"github.com/google/uuid"
)

type Skill struct {
	ID                 uuid.UUID  `json:"id" db:"id"`
	CompanyID          uuid.UUID  `json:"company_id" db:"company_id"`
	Name               string     `json:"name" db:"name"`
	Command            string     `json:"command" db:"command"`
	ContentInstruction string     `json:"content_instruction" db:"content_instruction"`
	IsActive           bool       `json:"is_active" db:"is_active"`
	CreatedBy          *uuid.UUID `json:"created_by" db:"created_by"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at" db:"updated_at"`
	DeletedAt          *time.Time `json:"deleted_at" db:"deleted_at"`
}
