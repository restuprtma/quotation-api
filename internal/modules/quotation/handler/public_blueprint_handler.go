package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

type PublicBlueprintResolver interface {
	GetPublicBlueprint(ctx context.Context, rawToken string) (*dto.ProgrammerBlueprintDTO, error)
}

type PublicBlueprintHandler struct {
	svc PublicBlueprintResolver
}

func NewPublicBlueprintHandler(svc PublicBlueprintResolver) *PublicBlueprintHandler {
	return &PublicBlueprintHandler{svc: svc}
}

func (h *PublicBlueprintHandler) GetPublicBlueprint(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		response.Error(c, http.StatusNotFound, "Blueprint not found", "")
		return
	}

	data, err := h.svc.GetPublicBlueprint(c.Request.Context(), token)
	if err != nil {
		if errors.Is(err, service.ErrPublicBlueprintNotFound) {
			response.Error(c, http.StatusNotFound, "Blueprint not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to retrieve blueprint", "")
		return
	}

	response.Success(c, http.StatusOK, "Blueprint retrieved successfully", data)
}
