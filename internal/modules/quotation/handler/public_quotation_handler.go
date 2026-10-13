package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

type PublicQuotationService interface {
	GetPublicQuotation(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error)
	SignPublicQuotation(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error)
}

type PublicQuotationHandler struct {
	svc PublicQuotationService
}

func NewPublicQuotationHandler(svc PublicQuotationService) *PublicQuotationHandler {
	return &PublicQuotationHandler{svc: svc}
}

func (h *PublicQuotationHandler) GetPublicQuotation(c *gin.Context) {
	tokenStr := c.Param("public_token")
	if _, err := uuid.Parse(tokenStr); err != nil {
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
		return
	}

	data, err := h.svc.GetPublicQuotation(c.Request.Context(), tokenStr)
	if err != nil {
		if errors.Is(err, service.ErrPublicQuotationNotFound) {
			response.Error(c, http.StatusNotFound, "Quotation not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve quotation", "")
		return
	}

	response.Success(c, http.StatusOK, "Quotation retrieved successfully", data)
}

func (h *PublicQuotationHandler) SignPublicQuotation(c *gin.Context) {
	tokenStr := c.Param("public_token")
	if _, err := uuid.Parse(tokenStr); err != nil {
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
		return
	}

	var req dto.SignPublicQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	clientIP := c.ClientIP()
	userAgent := c.Request.UserAgent()

	data, err := h.svc.SignPublicQuotation(c.Request.Context(), tokenStr, req, clientIP, userAgent)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrPublicQuotationNotFound):
			response.Error(c, http.StatusNotFound, "Quotation not found", "")
		case errors.Is(err, service.ErrQuotationAlreadySigned):
			response.Error(c, http.StatusConflict, "Quotation has already been signed", "")
		case errors.Is(err, service.ErrConsentRequired),
			errors.Is(err, service.ErrInvalidSignerName),
			errors.Is(err, service.ErrInvalidSignerEmail),
			errors.Is(err, service.ErrInvalidSignaturePNG),
			errors.Is(err, service.ErrInvalidPNGFormat):
			response.Error(c, http.StatusBadRequest, err.Error(), "")
		default:
			response.Error(c, http.StatusInternalServerError, "Failed to sign quotation", "")
		}
		return
	}

	response.Success(c, http.StatusOK, "Quotation signed successfully", data)
}
