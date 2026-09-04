package domain

import (
	"time"

	"github.com/google/uuid"
)

type InfrastructurePackage struct {
	ID             uuid.UUID `json:"id" db:"id"`
	CompanyID      uuid.UUID `json:"company_id" db:"company_id"`
	RateCardID     uuid.UUID `json:"rate_card_id" db:"rate_card_id"`
	Category       string    `json:"category" db:"category"`
	PackageName    string    `json:"package_name" db:"package_name"`
	VCPU           int       `json:"vcpu" db:"vcpu"`
	RAMGB          int       `json:"ram_gb" db:"ram_gb"`
	StorageGB      int       `json:"storage_gb" db:"storage_gb"`
	AmountPerMonth int64     `json:"amount_per_month" db:"amount_per_month"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
