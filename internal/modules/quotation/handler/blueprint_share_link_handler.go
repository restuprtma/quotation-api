package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

type ShareLinkManagementService interface {
	CreateShareLink(ctx context.Context, companyID, blueprintID uuid.UUID, userID *uuid.UUID, req dto.CreateBlueprintShareLinkRequest, baseURL string) (*dto.CreateBlueprintShareLinkResponse, error)
	ListShareLinks(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintShareLinkItemResponse, error)
	RevokeShareLink(ctx context.Context, companyID, blueprintID, linkID uuid.UUID) error
}

type BlueprintShareLinkHandler struct {
	svc     ShareLinkManagementService
	baseURL string
}

func NewBlueprintShareLinkHandler(svc ShareLinkManagementService, baseURL string) *BlueprintShareLinkHandler {
	return &BlueprintShareLinkHandler{
		svc:     svc,
		baseURL: baseURL,
	}
}

func resolveCompanyID(c *gin.Context) (uuid.UUID, error) {
	companyIDStr := middleware.GetCompanyID(c)
	if companyIDStr == "" {
		companyIDStr = c.GetString("company_id")
	}
	return uuid.Parse(companyIDStr)
}

func (h *BlueprintShareLinkHandler) Create(c *gin.Context) {
	companyID, err := resolveCompanyID(c)
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	bpIDStr := c.Param("id")
	blueprintID, err := uuid.Parse(bpIDStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	var req dto.CreateBlueprintShareLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	var userIDPtr *uuid.UUID
	if userIDStr, err := middleware.GetUserID(c); err == nil && userIDStr != "" {
		if uid, err := uuid.Parse(userIDStr); err == nil {
			userIDPtr = &uid
		}
	}

	targetBaseURL := h.baseURL
	if targetBaseURL == "" {
		scheme := "https"
		if c.Request.TLS == nil && c.Request.Header.Get("X-Forwarded-Proto") != "https" {
			scheme = "http"
		}
		targetBaseURL = scheme + "://" + c.Request.Host
	}

	res, err := h.svc.CreateShareLink(c.Request.Context(), companyID, blueprintID, userIDPtr, req, targetBaseURL)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrCannotShareDraftRevision), errors.Is(err, service.ErrInvalidShareLinkLabel):
			response.Error(c, http.StatusBadRequest, err.Error(), "")
		case errors.Is(err, service.ErrShareLinkBlueprintNotFound):
			response.Error(c, http.StatusNotFound, "Blueprint revision not found", "")
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to create share link", "")
		}
		return
	}

	response.Success(c, http.StatusCreated, "Share link created successfully", res)
}

func (h *BlueprintShareLinkHandler) List(c *gin.Context) {
	companyID, err := resolveCompanyID(c)
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	bpIDStr := c.Param("id")
	blueprintID, err := uuid.Parse(bpIDStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	items, err := h.svc.ListShareLinks(c.Request.Context(), companyID, blueprintID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve share links", "")
		return
	}

	response.Success(c, http.StatusOK, "Share links retrieved successfully", items)
}

func (h *BlueprintShareLinkHandler) Revoke(c *gin.Context) {
	companyID, err := resolveCompanyID(c)
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	bpIDStr := c.Param("id")
	blueprintID, err := uuid.Parse(bpIDStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint ID", "")
		return
	}

	linkIDStr := c.Param("link_id")
	linkID, err := uuid.Parse(linkIDStr)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid link ID", "")
		return
	}

	if err := h.svc.RevokeShareLink(c.Request.Context(), companyID, blueprintID, linkID); err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to revoke share link", "")
		return
	}

	response.Success(c, http.StatusOK, "Share link revoked successfully", gin.H{"revoked": true})
}
