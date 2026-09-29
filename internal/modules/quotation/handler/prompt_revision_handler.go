package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/shared/response"
)

type promptRevisionPatcher interface {
	ApplyCommercialPatch(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, dto.PromptPatchRequest) (*dto.PromptPatchResponse, error)
}

type promptSectionReviser interface {
	Revise(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, dto.ReviseSectionRequest) (*dto.PromptPatchResponse, error)
}

// PromptRevisionHandler exposes only the additive, typed commercial PATCH.
// It never delegates to whole-document regeneration.
type PromptRevisionHandler struct {
	patcher promptRevisionPatcher
	reviser promptSectionReviser
}

func NewPromptRevisionHandler(patcher promptRevisionPatcher, reviser ...promptSectionReviser) *PromptRevisionHandler {
	h := &PromptRevisionHandler{patcher: patcher}
	if len(reviser) > 0 {
		h.reviser = reviser[0]
	}
	return h
}

func (h *PromptRevisionHandler) ReviseSection(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation ID", "invalid_request")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	var req dto.ReviseSectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "invalid_request")
		return
	}
	if h == nil || h.reviser == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}
	var userID *uuid.UUID
	if raw, err := middleware.GetUserID(c); err == nil {
		if parsed, parseErr := uuid.Parse(raw); parseErr == nil {
			userID = &parsed
		}
	}
	result, err := h.reviser.Revise(c.Request.Context(), companyID, quotationID, userID, req)
	if err != nil {
		code := safePromptErrorCode(err)
		response.Error(c, promptRevisionHTTPStatus(code), "Unable to revise quotation draft", code)
		return
	}
	response.Success(c, http.StatusOK, "Quotation draft revised", result)
}

func (h *PromptRevisionHandler) PatchPromptState(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation ID", "invalid_request")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	var req dto.PromptPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "invalid_request")
		return
	}
	if h == nil || h.patcher == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}
	var userID *uuid.UUID
	if raw, err := middleware.GetUserID(c); err == nil {
		if parsed, parseErr := uuid.Parse(raw); parseErr == nil {
			userID = &parsed
		}
	}
	result, err := h.patcher.ApplyCommercialPatch(c.Request.Context(), companyID, quotationID, userID, req)
	if err != nil {
		response.Error(c, promptRevisionHTTPStatus(safePromptErrorCode(err)), "Unable to update quotation draft", safePromptErrorCode(err))
		return
	}
	response.Success(c, http.StatusOK, "Quotation draft updated", result)
}

func promptRevisionHTTPStatus(code string) int {
	switch code {
	case "invalid_patch", "invalid_request":
		return http.StatusBadRequest
	case "invalid_revision":
		return http.StatusUnprocessableEntity
	case "revision_not_found":
		return http.StatusNotFound
	case "revision_conflict":
		return http.StatusConflict
	case "discount_policy_unverified":
		return http.StatusConflict
	default:
		return http.StatusUnprocessableEntity
	}
}
