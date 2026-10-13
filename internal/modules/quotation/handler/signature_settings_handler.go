package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

type SignatureSettingsService interface {
	GetSignatureSettings(ctx context.Context, companyID string) (*dto.SignatureSettingsResponse, error)
	UpdateSignatureSettings(ctx context.Context, companyID string, req dto.UpdateSignatureSettingsRequest) (*dto.SignatureSettingsResponse, error)
}

type SignatureSettingsHandler struct {
	svc SignatureSettingsService
}

func NewSignatureSettingsHandler(svc SignatureSettingsService) *SignatureSettingsHandler {
	return &SignatureSettingsHandler{svc: svc}
}

func (h *SignatureSettingsHandler) getCompanyID(c *gin.Context) string {
	if companyID := c.GetString("company_id"); companyID != "" {
		return companyID
	}
	return middleware.GetCompanyID(c)
}

func (h *SignatureSettingsHandler) GetSignatureSettings(c *gin.Context) {
	companyID := h.getCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusUnauthorized, "Missing company context", "")
		return
	}

	data, err := h.svc.GetSignatureSettings(c.Request.Context(), companyID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve signature settings", "")
		return
	}

	response.Success(c, http.StatusOK, "Signature settings retrieved", data)
}

func (h *SignatureSettingsHandler) UpdateSignatureSettings(c *gin.Context) {
	companyID := h.getCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusUnauthorized, "Missing company context", "")
		return
	}

	var req dto.UpdateSignatureSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	data, err := h.svc.UpdateSignatureSettings(c.Request.Context(), companyID, req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidPNGFormat) {
			response.Error(c, http.StatusBadRequest, "Failed to update signature settings", err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to update signature settings", "")
		return
	}

	response.Success(c, http.StatusOK, "Signature settings updated", data)
}
