package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

type Handler struct {
	svc *service.Service
}

func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}

// List returns a paginated list of quotations for the caller's company.
func (h *Handler) List(c *gin.Context) {
	page, limit := parsePagination(c)
	search := c.Query("search")
	status := c.Query("status")

	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.List(c.Request.Context(), companyID, search, status, page, limit)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to list quotations", err.Error())
		return
	}
	response.SuccessWithPagination(c, http.StatusOK, "Quotations retrieved successfully", result.Items, page, limit, result.Total)
}

// Create creates a new quotation (draft) with its first revision.
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	userID, err := middleware.GetUserID(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized", err.Error())
		return
	}

	result, err := h.svc.Create(c.Request.Context(), companyID, userID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Quotation created successfully", result)
}

// GetByID returns one quotation with its active revision.
func (h *Handler) GetByID(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.GetDetail(c.Request.Context(), companyID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Quotation retrieved successfully", result)
}

// Update edits the active revision content of a quotation.
func (h *Handler) Update(c *gin.Context) {
	var req dto.UpdateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.Update(c.Request.Context(), companyID, c.Param("id"), &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Quotation updated successfully", result)
}

func (h *Handler) CreateRevision(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.CreateRevision(c.Request.Context(), companyID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Revision created successfully", result)
}

func (h *Handler) ListRevisions(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.ListRevisions(c.Request.Context(), companyID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Revisions retrieved successfully", result)
}

func (h *Handler) GetRevisionDetail(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	revisionID := c.Param("revision_id")

	revs, err := h.svc.ListRevisions(c.Request.Context(), companyID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}

	for _, r := range revs {
		if r.ID.String() == revisionID {
			r.BlueprintSourceMetadata = nil
			response.Success(c, http.StatusOK, "Revision detail retrieved successfully", r)
			return
		}
	}
	response.Error(c, http.StatusNotFound, "Revision not found", "")
}

func (h *Handler) ShareQuotation(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.ShareInternal(c.Request.Context(), companyID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Quotation shared successfully", result)
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrRateCardNotFound):
		response.Error(c, http.StatusNotFound, "Rate card not found", "")
	case errors.Is(err, repository.ErrRateCardItemNotFound):
		response.Error(c, http.StatusNotFound, "Rate card item not found", "")
	case errors.Is(err, service.ErrNotFound):
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
	case errors.Is(err, service.ErrInvalidClient):
		response.Error(c, http.StatusBadRequest, "Client not found or not active", err.Error())
	case errors.Is(err, service.ErrNoCompanyContext):
		response.Error(c, http.StatusForbidden, "Company context is required", "")
	case errors.Is(err, service.ErrConflict), errors.Is(err, service.ErrAlreadySigned):
		response.Error(c, http.StatusConflict, "Cannot mutate signed quotation", err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error", err.Error())
	}
}

func parsePagination(c *gin.Context) (page, limit int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "25"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 25
	}
	return page, limit
}
