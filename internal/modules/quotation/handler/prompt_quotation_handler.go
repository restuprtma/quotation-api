package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/shared/response"
)

type promptQuotationCreator interface {
	CreateFromMeeting(context.Context, uuid.UUID, *uuid.UUID, dto.CreateFromMeetingRequest) (*dto.MeetingDraftResponse, error)
}

// PromptQuotationHandler exposes the additive prompt-first endpoint. It does
// not inspect or transform provider output; the service owns extraction,
// validation, pricing, rendering, and persistence.
type PromptQuotationHandler struct {
	creator promptQuotationCreator
}

func NewPromptQuotationHandler(creator promptQuotationCreator) *PromptQuotationHandler {
	return &PromptQuotationHandler{creator: creator}
}

func (h *PromptQuotationHandler) CreateFromMeeting(c *gin.Context) {
	var req dto.CreateFromMeetingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "invalid_request")
		return
	}
	if strings.TrimSpace(req.MeetingNotes) == "" {
		response.Error(c, http.StatusBadRequest, "Meeting notes are required", "meeting_notes_required")
		return
	}

	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	var createdBy *uuid.UUID
	if rawID, err := middleware.GetUserID(c); err == nil {
		if userID, parseErr := uuid.Parse(rawID); parseErr == nil {
			createdBy = &userID
		}
	}
	if h == nil || h.creator == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	result, err := h.creator.CreateFromMeeting(c.Request.Context(), companyID, createdBy, req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		code := safePromptErrorCode(err)
		if code == "invalid_request" {
			status = http.StatusBadRequest
		}
		response.Error(c, status, "Unable to create quotation draft", code)
		return
	}

	status := http.StatusCreated
	if result != nil && result.IdempotentReplay {
		status = http.StatusOK
	}
	response.Success(c, status, "Quotation draft created", result)
}

type safePromptError interface {
	SafeCode() string
}

func safePromptErrorCode(err error) string {
	var safe safePromptError
	if errors.As(err, &safe) && strings.TrimSpace(safe.SafeCode()) != "" {
		return safe.SafeCode()
	}
	return "generation_failed"
}
