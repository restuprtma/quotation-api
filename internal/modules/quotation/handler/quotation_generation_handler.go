package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

// QuotationGenerationHandler exposes the AI-first quotation generation
// endpoints. It deliberately performs NO logging of request payloads, prompts
// or tokens; every failure surfaces as a safe error code produced by the
// service layer (GenerationError.Error() prints the code only).
type QuotationGenerationHandler struct {
	genSvc  *service.QuotationGenerationService
	genRepo *repository.GenerationRepository
}

func NewQuotationGenerationHandler(genSvc *service.QuotationGenerationService, genRepo *repository.GenerationRepository) *QuotationGenerationHandler {
	return &QuotationGenerationHandler{genSvc: genSvc, genRepo: genRepo}
}

// Generate starts the AI-first pipeline and returns 201 with the committed
// quotation ID. The draft is durable before any provider interaction, so a
// later failure remains retryable and idempotent replay (same
// idempotency_key) returns the same aggregate.
func (h *QuotationGenerationHandler) Generate(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	var createdBy *uuid.UUID
	if rawID, uerr := middleware.GetUserID(c); uerr == nil {
		if uid, perr := uuid.Parse(rawID); perr == nil {
			createdBy = &uid
		}
	}

	quotationID, err := h.genSvc.Generate(c.Request.Context(), companyID, createdBy, req)
	if err != nil {
		h.mapGenerationError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Quotation generated successfully", gin.H{
		"quotation_id": quotationID,
	})
}

// Regenerate re-runs the pipeline for an active draft. A successful response
// reports the refreshed generation state including the incremented
// content_version; failures never alter stored content.
//
// Contract note (deferred Session-4 decision): while a draft is pending its
// idempotent generate replays still return success with the same quotation
// ID — clients MUST consult GET /:id/generation-status for authoritative
// status instead of inferring it from generate/regenerate responses.
func (h *QuotationGenerationHandler) Regenerate(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	if err := h.genSvc.Regenerate(c.Request.Context(), companyID, quotationID); err != nil {
		h.mapRegenerateError(c, err)
		return
	}

	state, serr := h.genRepo.GetGenerationState(c.Request.Context(), companyID, quotationID)
	if serr != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to load generation state", "")
		return
	}
	response.Success(c, http.StatusOK, "Quotation regenerated successfully", state)
}

// GetByID returns the AI-first detail view of a quotation: master identity
// plus the current revision's generation state (status, content_version,
// content_html). Budget figures and internal snapshots are excluded. A
// foreign or unknown aggregate answers 404 so existence is never leaked
// across tenants.
//
// NOTE: this replaces the previous route target, whose service method was an
// unconditional "not implemented" stub — no working behaviour changed hands.
func (h *QuotationGenerationHandler) GetByID(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	detail, derr := h.genRepo.GetGenerationDetail(c.Request.Context(), companyID, quotationID)
	if derr != nil {
		if errors.Is(derr, repository.ErrRevisionNotFound) {
			response.Error(c, http.StatusNotFound, "Quotation not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to load quotation", "")
		return
	}
	response.Success(c, http.StatusOK, "Quotation retrieved successfully", detail)
}

// GetGenerationStatus returns the safe generation view of a quotation's
// current revision: status, content_version, content_html and timestamps.
// Budget figures and internal snapshots are intentionally excluded.
func (h *QuotationGenerationHandler) GetGenerationStatus(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	state, serr := h.genRepo.GetGenerationState(c.Request.Context(), companyID, quotationID)
	if serr != nil {
		if errors.Is(serr, repository.ErrRevisionNotFound) {
			response.Error(c, http.StatusNotFound, "Quotation not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to load generation state", "")
		return
	}
	response.Success(c, http.StatusOK, "Generation status retrieved successfully", state)
}

// SaveDraft persists user edits to a quotation's draft revision with optimistic locking.
func (h *QuotationGenerationHandler) SaveDraft(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	var req dto.SaveDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if req.ContentVersion < 1 {
		response.Error(c, http.StatusBadRequest, "Invalid content version", "")
		return
	}

	var userID *uuid.UUID
	if rawID, uerr := middleware.GetUserID(c); uerr == nil {
		if uid, perr := uuid.Parse(rawID); perr == nil {
			userID = &uid
		}
	}

	res, err := h.genSvc.SaveDraft(c.Request.Context(), companyID, quotationID, userID, req)
	if err != nil {
		h.mapSaveDraftError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Draft saved successfully", res)
}

// mapSaveDraftError distinguishes tenant isolation 404, conflict 409, and DOM safety 422.
func (h *QuotationGenerationHandler) mapSaveDraftError(c *gin.Context, err error) {
	if errors.Is(err, repository.ErrRevisionNotFound) {
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
		return
	}
	if errors.Is(err, repository.ErrRevisionConflict) {
		response.Error(c, http.StatusConflict, "Revision conflict: stale content version", service.ErrCodeRevisionConflict)
		return
	}
	var ge *service.GenerationError
	if errors.As(err, &ge) {
		if ge.Code == service.ErrCodeRevisionConflict {
			response.Error(c, http.StatusConflict, "Revision conflict: stale content version", ge.Error())
			return
		}
		if ge.Code == service.ErrCodeUnsafeHTML {
			response.Error(c, http.StatusUnprocessableEntity, "Unsafe HTML or integrity validation failed", ge.Error())
			return
		}
	}
	h.mapGenerationError(c, err)
}

// mapRegenerateError distinguishes the tenant-isolation miss (404, no
// existence leak across companies) from service-level safe codes.
func (h *QuotationGenerationHandler) mapRegenerateError(c *gin.Context, err error) {
	if errors.Is(err, repository.ErrRevisionNotFound) {
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
		return
	}
	h.mapGenerationError(c, err)
}

// mapGenerationError translates *service.GenerationError into HTTP responses.
// Every code maps to a static message; errors.detail carries ONLY the safe
// code itself (GenerationError.Error() == code), so no provider payload,
// stack trace, budget figure or internal note can ever reach the client.
func (h *QuotationGenerationHandler) mapGenerationError(c *gin.Context, err error) {
	var ge *service.GenerationError
	if !errors.As(err, &ge) {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	switch ge.Code {
	case service.ErrCodeRegenerateRateLimited:
		response.Error(c, http.StatusTooManyRequests, "Regenerate limit reached", ge.Error())
	case service.ErrCodeRevisionConflict:
		response.Error(c, http.StatusConflict, "Revision is not regenerable", ge.Error())
	default:
		// settings_incomplete, invalid_rate_card, budget_unfeasible,
		// allocation_invalid, provider_*, unsafe_html, generation_failed:
		// business/configuration/provider conditions that the request body
		// cannot mechanically fix, hence 422 with the bare code.
		response.Error(c, http.StatusUnprocessableEntity, "Generation failed", ge.Error())
	}
}

// companyUUID resolves the tenant from JWT claims set by middleware.
func companyUUID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(middleware.GetCompanyID(c))
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return uuid.Nil, false
	}
	return id, true
}
