package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
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

// GenerateAI returns a mock AI draft (description, scope of work, blueprint)
// for the given project context so the frontend can render/iterate on it.
func (h *Handler) GenerateAI(c *gin.Context) {
	var req dto.AIGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	result, err := h.svc.GenerateAIDraft(c.Request.Context(), &req)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "AI generation failed", err.Error())
		return
	}
	response.Success(c, http.StatusOK, "AI draft generated successfully", result)
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

func (h *Handler) GetArtifacts(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	artifacts, err := h.svc.GetArtifacts(c.Request.Context(), companyID, c.Param("id"))
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Artifacts retrieved successfully", artifacts)
}

// GetPublicQuotation returns a quotation revision addressed by its public token.
func (h *Handler) GetPublicQuotation(c *gin.Context) {
	result, err := h.svc.GetPublicQuotation(c.Request.Context(), c.Param("artifact_token"))
	if err != nil {
		h.handlePublicError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Public quotation retrieved successfully", result)
}

// SignPublicQuotation stores a client's drawn signature and marks the quotation signed.
func (h *Handler) SignPublicQuotation(c *gin.Context) {
	var req dto.SignPublicQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid signature payload", err.Error())
		return
	}

	result, err := h.svc.SignPublic(c.Request.Context(), c.Param("artifact_token"), req.SignatureImage, c.ClientIP())
	if err != nil {
		h.handlePublicError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Quotation signed successfully", result)
}

// GetPublicBlueprint returns a blueprint artifact addressed by its public token.
func (h *Handler) GetPublicBlueprint(c *gin.Context) {
	result, err := h.svc.GetPublicBlueprint(c.Request.Context(), c.Param("artifact_token"))
	if err != nil {
		h.handlePublicError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Public blueprint retrieved successfully", result)
}

// GetPublicDetail returns a detail artifact addressed by its public token.
func (h *Handler) GetPublicDetail(c *gin.Context) {
	result, err := h.svc.GetPublicDetail(c.Request.Context(), c.Param("artifact_token"))
	if err != nil {
		h.handlePublicError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Public detail retrieved successfully", result)
}

func (h *Handler) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
	case errors.Is(err, service.ErrInvalidClient):
		response.Error(c, http.StatusBadRequest, "Client not found or not active", err.Error())
	case errors.Is(err, service.ErrNoCompanyConte):
		response.Error(c, http.StatusForbidden, "Company context is required", "")
	case errors.Is(err, service.ErrConflict), errors.Is(err, service.ErrAlreadySigned):
		response.Error(c, http.StatusConflict, "Cannot mutate signed quotation", err.Error())
	default:
		response.Error(c, http.StatusInternalServerError, "Internal server error", err.Error())
	}
}

func (h *Handler) handlePublicError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrPublicNotFound):
		response.Error(c, http.StatusNotFound, "Public quotation not found", "")
	case errors.Is(err, service.ErrInvalidSignature):
		response.Error(c, http.StatusBadRequest, "Invalid signature image", "")
	case errors.Is(err, service.ErrAlreadySigned):
		response.Error(c, http.StatusConflict, "Quotation is already signed", "")
	default:
		response.Error(c, http.StatusInternalServerError, "Failed to process public quotation", "")
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
