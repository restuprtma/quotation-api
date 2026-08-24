package dto

import (
	"github.com/google/uuid"
	"time"
)

type CreateLegalTemplateRequest struct {
	TemplateCode string `json:"template_code" binding:"required"`
	TemplateName string `json:"template_name" binding:"required"`
	BodyContent  string `json:"body_content" binding:"required"`
}

type UpdateLegalTemplateRequest struct {
	TemplateName *string `json:"template_name"`
	BodyContent  *string `json:"body_content"`
	IsActive     *bool   `json:"is_active"`
}

type LegalTemplateResponse struct {
	ID            uuid.UUID `json:"id"`
	CompanyID     uuid.UUID `json:"company_id"`
	TemplateCode  string    `json:"template_code"`
	TemplateName  string    `json:"template_name"`
	VersionNumber int       `json:"version_number"`
	BodyContent   string    `json:"body_content"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
