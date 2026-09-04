package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/shared/response"
)

// CreateRateCard creates a new rate card with items.
func (h *Handler) CreateRateCard(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	userIDStr, err := middleware.GetUserID(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized", err.Error())
		return
	}

	var req dto.CreateRateCardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	var userUUIDPtr *uuid.UUID
	if parsedUUID, err := uuid.Parse(userIDStr); err == nil {
		userUUIDPtr = &parsedUUID
	}

	res, err := h.svc.CreateRateCard(c.Request.Context(), companyID, userUUIDPtr, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusCreated, "Rate card created successfully", res)
}

// GetRateCardDetail retrieves rate card details including items.
func (h *Handler) GetRateCardDetail(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	id := c.Param("id")
	res, err := h.svc.GetRateCardDetail(c.Request.Context(), companyID, id)
	if err != nil {
		if errors.Is(err, repository.ErrRateCardNotFound) {
			response.Error(c, http.StatusNotFound, "Rate card not found", "")
			return
		}
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Rate card details retrieved successfully", res)
}

// DeleteRateCard deletes a rate card and its associated items.
func (h *Handler) DeleteRateCard(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	id := c.Param("id")
	if err := h.svc.DeleteRateCard(c.Request.Context(), companyID, id); err != nil {
		if errors.Is(err, repository.ErrRateCardNotFound) {
			response.Error(c, http.StatusNotFound, "Rate card not found", "")
			return
		}
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Rate card deleted successfully", nil)
}
