package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateClientOrganizationRequest represents the payload for registering a new client organization.
type CreateClientOrganizationRequest struct {
	Name string `json:"name" binding:"required"`
}

// ClientOrganizationResponse represents the response data for a client organization.
type ClientOrganizationResponse struct {
	ID             uuid.UUID  `json:"id"`
	CompanyID      uuid.UUID  `json:"company_id"`
	Name           string     `json:"name"`
	NormalizedName string     `json:"normalized_name"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// ClientOrganizationListResponse wraps the list of client organizations.
type ClientOrganizationListResponse struct {
	Items []ClientOrganizationResponse `json:"items"`
}
