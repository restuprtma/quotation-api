package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type ClientOrganization struct {
	CompanyID      uuid.UUID
	ID             uuid.UUID
	Name           string
	NormalizedName string
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}

// NormalizeClientOrganizationName normalizes whitespace and lowercase for uniqueness checking.
// It returns display (single space separated, trimmed) and normalized (display lowercased).
func NormalizeClientOrganizationName(raw string) (display, normalized string) {
	display = strings.Join(strings.Fields(raw), " ")
	return display, strings.ToLower(display)
}
