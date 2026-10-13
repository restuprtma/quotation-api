package domain

import (
	"time"

	"github.com/google/uuid"
)

type SignatureSettings struct {
	ID                    uuid.UUID `json:"id" db:"id"`
	CompanyID             uuid.UUID `json:"company_id" db:"company_id"`
	DirectorName          string    `json:"director_name" db:"director_name"`
	DirectorTitle         string    `json:"director_title" db:"director_title"`
	DirectorSignatureData string    `json:"director_signature_data" db:"director_signature_data"`
	CompanyStampData      string    `json:"company_stamp_data" db:"company_stamp_data"`
	CreatedAt             time.Time `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time `json:"updated_at" db:"updated_at"`
}
