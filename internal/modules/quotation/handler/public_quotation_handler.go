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
	// Implemented in Task 3, stub for now
}
