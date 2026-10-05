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
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/shared/response"
)

type promptQuotationCreator interface {
	CreateMeetingDraft(context.Context, uuid.UUID, *uuid.UUID, dto.CreateFromMeetingRequest) (*dto.PendingMeetingDraftResponse, error)
	ProcessMeetingDraft(context.Context, uuid.UUID, uuid.UUID) (*dto.MeetingDraftResponse, error)
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

	result, err := h.creator.CreateMeetingDraft(c.Request.Context(), companyID, createdBy, req)
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

// ProcessMeeting retries or completes the persisted meeting draft identified
// by :id. The handler only exposes safe lifecycle codes, never provider data.
func (h *PromptQuotationHandler) ProcessMeeting(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "invalid_request")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	if h == nil || h.creator == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	result, err := h.creator.ProcessMeetingDraft(c.Request.Context(), companyID, quotationID)
	if err != nil {
		if errors.Is(err, repository.ErrRevisionNotFound) {
			response.Error(c, http.StatusNotFound, "Quotation not found", "generation_failed")
			return
		}
		code := safePromptErrorCode(err)
		switch code {
		case "provider_timeout":
			response.Error(c, http.StatusGatewayTimeout, "AI provider timed out", code)
		case "generation_in_progress":
			response.Error(c, http.StatusConflict, "Quotation generation is already in progress", code)
		default:
			response.Error(c, http.StatusUnprocessableEntity, "Unable to process quotation draft", code)
		}
		return
	}
	response.Success(c, http.StatusOK, "Quotation draft processed", result)
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
