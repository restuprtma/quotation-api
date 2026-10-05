package handler

import (
	"context"
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

// QuotationLifecycleService defines operations for quotation publishing and revision management.
type QuotationLifecycleService interface {
	PublishQuotation(ctx context.Context, companyID, quotationID string, req dto.PublishQuotationRequest) error
	CreateQuotationRevision(ctx context.Context, companyID, quotationID string) (*dto.QuotationRevisionResponse, error)
	ListRevisions(ctx context.Context, companyID, id string) ([]dto.QuotationRevisionResponse, error)
	GetQuotationHistory(ctx context.Context, companyID, quotationID string) ([]dto.QuotationRevisionHistoryItem, error)
}

// BlueprintLifecycleDraftService defines draft, revision, and publication operations for blueprints.
type BlueprintLifecycleDraftService interface {
	PublishBlueprint(ctx context.Context, companyID, blueprintID string, req dto.PublishBlueprintRequest) error
	CreateBlueprintRevision(ctx context.Context, companyID, blueprintID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error)
	DeleteDraft(ctx context.Context, companyID, blueprintID string) error
}

// BlueprintLifecycleReadService defines read operations for blueprint revisions.
type BlueprintLifecycleReadService interface {
	ListRevisions(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintRevisionHistoryItem, error)
}

// LifecycleHandler manages HTTP endpoints for Quotation and Blueprint revision lifecycles.
type LifecycleHandler struct {
	quotationSvc QuotationLifecycleService
	bpDraftSvc   BlueprintLifecycleDraftService
	bpReadSvc    BlueprintLifecycleReadService
}

// NewLifecycleHandler constructs a new LifecycleHandler.
func NewLifecycleHandler(
	quotationSvc QuotationLifecycleService,
	bpDraftSvc BlueprintLifecycleDraftService,
	bpReadSvc BlueprintLifecycleReadService,
) *LifecycleHandler {
	return &LifecycleHandler{
		quotationSvc: quotationSvc,
		bpDraftSvc:   bpDraftSvc,
		bpReadSvc:    bpReadSvc,
	}
}

// PublishQuotation transitions a draft quotation revision to published.
func (h *LifecycleHandler) PublishQuotation(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	quotationID := c.Param("id")
	if _, err := uuid.Parse(quotationID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation ID", "")
		return
	}

	var req dto.PublishQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	err := h.quotationSvc.PublishQuotation(c.Request.Context(), companyID, quotationID, req)
	if err != nil {
		h.handleQuotationError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Quotation published successfully", gin.H{"status": "published"})
}

// CreateQuotationRevision creates a new draft revision cloned from the active published revision.
func (h *LifecycleHandler) CreateQuotationRevision(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	quotationID := c.Param("id")
	if _, err := uuid.Parse(quotationID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation ID", "")
		return
	}

	res, err := h.quotationSvc.CreateQuotationRevision(c.Request.Context(), companyID, quotationID)
	if err != nil {
		h.handleQuotationError(c, err)
		return
	}

	response.Success(c, http.StatusCreated, "Quotation revision created successfully", res)
}

// ListQuotationRevisions returns the revision history for a quotation.
func (h *LifecycleHandler) ListQuotationRevisions(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	quotationID := c.Param("id")
	if _, err := uuid.Parse(quotationID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation ID", "")
		return
	}

	if c.Query("view") == "history" {
		history, err := h.quotationSvc.GetQuotationHistory(c.Request.Context(), companyID, quotationID)
		if err != nil {
			h.handleQuotationError(c, err)
			return
		}
		response.Success(c, http.StatusOK, "Quotation revision history retrieved successfully", history)
		return
	}

	result, err := h.quotationSvc.ListRevisions(c.Request.Context(), companyID, quotationID)
	if err != nil {
		h.handleQuotationError(c, err)
		return
	}

	for i := range result {
		result[i].AIRawResponse = nil
	}

	response.Success(c, http.StatusOK, "Revisions retrieved successfully", result)
}

// GetQuotationRevisionDetail returns detail for a specified quotation revision.
func (h *LifecycleHandler) GetQuotationRevisionDetail(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	quotationID := c.Param("id")
	if _, err := uuid.Parse(quotationID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation ID", "")
		return
	}

	revisionID := c.Param("revision_id")
	if _, err := uuid.Parse(revisionID); err != nil {
		response.Error(c, http.StatusNotFound, "Revision not found", "")
		return
	}

	revs, err := h.quotationSvc.ListRevisions(c.Request.Context(), companyID, quotationID)
	if err != nil {
		h.handleQuotationError(c, err)
		return
	}

	for _, r := range revs {
		if r.ID.String() == revisionID {
			r.BlueprintSourceMetadata = nil
			r.AIRawResponse = nil
			response.Success(c, http.StatusOK, "Revision detail retrieved successfully", r)
			return
		}
	}

	response.Error(c, http.StatusNotFound, "Revision not found", "")
}

// PublishBlueprint publishes a blueprint draft revision.
func (h *LifecycleHandler) PublishBlueprint(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	blueprintID := c.Param("id")
	if _, err := uuid.Parse(blueprintID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	var req dto.PublishBlueprintRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	err := h.bpDraftSvc.PublishBlueprint(c.Request.Context(), companyID, blueprintID, req)
	if err != nil {
		h.handleBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint published successfully", gin.H{"status": "published"})
}

// CreateBlueprintRevision creates a new draft revision for a published blueprint.
func (h *LifecycleHandler) CreateBlueprintRevision(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	blueprintID := c.Param("id")
	if _, err := uuid.Parse(blueprintID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	var req dto.CreateBlueprintRevisionRequest
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
			return
		}
	}
	if req.Mode == "" {
		req.Mode = "clone"
	}

	res, err := h.bpDraftSvc.CreateBlueprintRevision(c.Request.Context(), companyID, blueprintID, req)
	if err != nil {
		h.handleBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusCreated, "Blueprint revision created successfully", res)
}

// ListBlueprintRevisions returns the revision history for a blueprint.
func (h *LifecycleHandler) ListBlueprintRevisions(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	blueprintID := c.Param("id")
	blueprintUUID, err := uuid.Parse(blueprintID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	items, err := h.bpReadSvc.ListRevisions(c.Request.Context(), companyUUID, blueprintUUID)
	if err != nil {
		h.handleBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint revisions retrieved successfully", items)
}

// GetBlueprintRevisionDetail returns detail for a specified blueprint revision.
func (h *LifecycleHandler) GetBlueprintRevisionDetail(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	blueprintID := c.Param("id")
	blueprintUUID, err := uuid.Parse(blueprintID)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	revisionID := c.Param("revision_id")
	revisionUUID, err := uuid.Parse(revisionID)
	if err != nil {
		response.Error(c, http.StatusNotFound, "Blueprint revision not found", "")
		return
	}

	items, err := h.bpReadSvc.ListRevisions(c.Request.Context(), companyUUID, blueprintUUID)
	if err != nil {
		h.handleBlueprintError(c, err)
		return
	}

	for _, item := range items {
		if item.ID == revisionUUID {
			response.Success(c, http.StatusOK, "Blueprint revision detail retrieved successfully", item)
			return
		}
	}

	response.Error(c, http.StatusNotFound, "Blueprint revision not found", "")
}

// DeleteBlueprintDraft deletes the active draft revision for a blueprint.
func (h *LifecycleHandler) DeleteBlueprintDraft(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	blueprintID := c.Param("id")
	if _, err := uuid.Parse(blueprintID); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	err := h.bpDraftSvc.DeleteDraft(c.Request.Context(), companyID, blueprintID)
	if err != nil {
		h.handleBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Draft deleted successfully", nil)
}

func (h *LifecycleHandler) handleQuotationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrConfirmationRequired):
		response.Error(c, http.StatusBadRequest, "Confirmation required for modified content", err.Error())
	case errors.Is(err, service.ErrDraftNotCompleted):
		response.Error(c, http.StatusBadRequest, "Draft generation must be completed before publishing", err.Error())
	case errors.Is(err, service.ErrEmptyContent):
		response.Error(c, http.StatusBadRequest, "Quotation content cannot be empty", err.Error())
	case errors.Is(err, service.ErrContentVersionMismatch):
		response.Error(c, http.StatusConflict, "Content version mismatch", err.Error())
	case errors.Is(err, service.ErrActiveDraftAlreadyExists):
		response.Error(c, http.StatusConflict, "Active draft revision already exists", err.Error())
	case errors.Is(err, service.ErrNoPublishedRevision):
		response.Error(c, http.StatusBadRequest, "No published revision found to revise", err.Error())
	case errors.Is(err, service.ErrNotFound), errors.Is(err, repository.ErrNotFound), errors.Is(err, repository.ErrRevisionNotFound):
		response.Error(c, http.StatusNotFound, "Quotation or revision not found", "")
	case errors.Is(err, service.ErrNoCompanyContext):
		response.Error(c, http.StatusForbidden, "Company context is required", "")
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error", err.Error())
	}
}

func (h *LifecycleHandler) handleBlueprintError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrBlueprintConfirmationRequired):
		response.Error(c, http.StatusBadRequest, "Confirmation required for partial blueprint publish", err.Error())
	case errors.Is(err, service.ErrBlueprintActiveDraftAlreadyExists):
		response.Error(c, http.StatusConflict, "Active draft revision already exists", err.Error())
	case errors.Is(err, service.ErrBlueprintNoPublishedRevision):
		response.Error(c, http.StatusBadRequest, "No published revision found to revise", err.Error())
	case errors.Is(err, service.ErrBlueprintDraftNotFound), errors.Is(err, repository.ErrBlueprintDraftNotFound):
		response.Error(c, http.StatusNotFound, "Draft blueprint revision not found", "")
	case errors.Is(err, service.ErrBlueprintRevisionNotFound), errors.Is(err, repository.ErrBlueprintRevisionNotFound):
		response.Error(c, http.StatusNotFound, "Blueprint revision not found", "")
	case errors.Is(err, service.ErrBlueprintNotFound), errors.Is(err, repository.ErrBlueprintNotFound), errors.Is(err, repository.ErrBlueprintMasterNotFound):
		response.Error(c, http.StatusNotFound, "Blueprint not found", "")
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error", err.Error())
	}
}
