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
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

type clientOrganizationService interface {
	Create(ctx context.Context, companyID uuid.UUID, createdBy *uuid.UUID, req dto.CreateClientOrganizationRequest) (*dto.ClientOrganizationResponse, error)
	List(ctx context.Context, companyID uuid.UUID) (*dto.ClientOrganizationListResponse, error)
}

// ClientOrganizationHandler handles company-scoped client organization HTTP endpoints.
type ClientOrganizationHandler struct {
	svc clientOrganizationService
}

func NewClientOrganizationHandler(svc clientOrganizationService) *ClientOrganizationHandler {
	return &ClientOrganizationHandler{svc: svc}
}

// List returns all active client organizations for the caller's company.
func (h *ClientOrganizationHandler) List(c *gin.Context) {
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	if h == nil || h.svc == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	res, err := h.svc.List(c.Request.Context(), companyID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to list client organizations", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Client organizations retrieved successfully", res)
}

// Create registers a new company-scoped client organization.
func (h *ClientOrganizationHandler) Create(c *gin.Context) {
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	var req dto.CreateClientOrganizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "invalid_request")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		response.Error(c, http.StatusBadRequest, "Client organization name is required", "name_required")
		return
	}

	var createdBy *uuid.UUID
	if rawID, err := middleware.GetUserID(c); err == nil {
		if userID, parseErr := uuid.Parse(rawID); parseErr == nil {
			createdBy = &userID
		}
	}

	if h == nil || h.svc == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	res, err := h.svc.Create(c.Request.Context(), companyID, createdBy, req)
	if err != nil {
		if errors.Is(err, repository.ErrClientOrganizationDuplicate) || errors.Is(err, service.ErrClientOrganizationDuplicate) {
			response.Error(c, http.StatusConflict, "Client organization already exists", "client_organization_exists")
			return
		}
		if errors.Is(err, service.ErrInvalidClientOrgName) {
			response.Error(c, http.StatusBadRequest, "Invalid client organization name", "invalid_name")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to create client organization", err.Error())
		return
	}

	response.Success(c, http.StatusCreated, "Client organization created successfully", res)
}
