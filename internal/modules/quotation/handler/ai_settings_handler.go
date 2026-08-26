package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/shared/response"
)

// GetSettings retrieves the AI credential settings for the current company
func (h *Handler) GetSettings(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.GetSettings(c.Request.Context(), companyID)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Settings retrieved successfully", result)
}

// UpdateSettings updates the AI credential settings for the current company
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req dto.CredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	// Validate strict logic: If provider changed and APIKey is empty, throw error.
	if req.APIKey == "" {
		oldSettings, err := h.svc.GetSettings(c.Request.Context(), companyID)
		// We only enforce this if there are existing settings
		if err == nil && oldSettings != nil {
			if oldSettings.Provider != req.ProviderType {
				response.Error(c, http.StatusBadRequest, "API Key is required when changing provider", "")
				return
			}
		}
	}

	result, err := h.svc.UpdateSettings(c.Request.Context(), companyID, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Settings updated successfully", result)
}

// TestSettingsConnection tests the AI credential connection
func (h *Handler) TestSettingsConnection(c *gin.Context) {
	var req dto.CredentialRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	err := h.svc.TestSettingsConnection(c.Request.Context(), companyID, &req)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Connection test failed", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Connection successful", nil)
}

func (h *Handler) ListSkills(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.ListSkills(c.Request.Context(), companyID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Skills retrieved successfully", result)
}

func (h *Handler) GetSkill(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	id := c.Param("id")

	result, err := h.svc.GetSkill(c.Request.Context(), companyID, id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Skill retrieved successfully", result)
}

func (h *Handler) UpdateSkill(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	id := c.Param("id")

	var req dto.UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	result, err := h.svc.UpdateSkill(c.Request.Context(), companyID, id, &req)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Skill updated successfully", result)
}

func (h *Handler) ResetSkillDefault(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	id := c.Param("id")

	err := h.svc.ResetSkillDefault(c.Request.Context(), companyID, id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Skill reset successfully", nil)
}

func (h *Handler) ListRateCards(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.ListRateCards(c.Request.Context(), companyID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Rate cards retrieved successfully", result)
}

func (h *Handler) GetRateCard(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}
	id := c.Param("id")

	result, err := h.svc.GetRateCard(c.Request.Context(), companyID, id)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Rate card retrieved successfully", result)
}

func (h *Handler) ListLegalTemplates(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	result, err := h.svc.ListLegalTemplates(c.Request.Context(), companyID)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Legal templates retrieved successfully", result)
}

func (h *Handler) GetActiveLegalTemplate(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	if companyID == "" {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return
	}

	code := c.Param("code")
	result, err := h.svc.GetActiveLegalTemplate(c.Request.Context(), companyID, code)
	if err != nil {
		h.handleError(c, err)
		return
	}
	response.Success(c, http.StatusOK, "Active legal template retrieved successfully", result)
}
