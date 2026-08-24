package dto

import (
	"github.com/google/uuid"
	"time"
)

type CreateSkillRequest struct {
	Command      string  `json:"command" binding:"required"`
	Name         string  `json:"name" binding:"required"`
	Description  *string `json:"description"`
	SystemPrompt string  `json:"system_prompt" binding:"required"`
	IsActive     bool    `json:"is_active"`
}

type UpdateSkillRequest struct {
	Command      *string `json:"command"`
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	SystemPrompt *string `json:"system_prompt"`
	IsActive     *bool   `json:"is_active"`
}

type SkillResponse struct {
	ID           uuid.UUID `json:"id"`
	CompanyID    uuid.UUID `json:"company_id"`
	Command      string    `json:"command"`
	Name         string    `json:"name"`
	Description  *string   `json:"description,omitempty"`
	SystemPrompt string    `json:"system_prompt"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
