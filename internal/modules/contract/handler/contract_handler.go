package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/contract/domain"
	"venturo-skeleton-go/internal/modules/contract/dto"
	"venturo-skeleton-go/internal/modules/contract/service"
	"venturo-skeleton-go/internal/shared/response"
)

type ContractHandler struct {
	service service.ContractService
}

func NewContractHandler(service service.ContractService) *ContractHandler {
	return &ContractHandler{service: service}
}

func (h *ContractHandler) Create(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	userID := middleware.MustGetUserID(c)

	var req dto.CreateContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	contract, err := h.service.CreateContract(c.Request.Context(), companyID, req.QuotationID, userID)
	if err != nil {
		if err == service.ErrQuotationNotSigned || err == service.ErrContractExists {
			response.Error(c, http.StatusConflict, err.Error(), "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to create contract", err.Error())
		return
	}

	response.Success(c, http.StatusCreated, "Contract created successfully", mapToContractResponse(contract))
}

func (h *ContractHandler) List(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)

	contracts, err := h.service.ListContracts(c.Request.Context(), companyID)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to list contracts", err.Error())
		return
	}

	var res []dto.ContractResponse
	for _, c := range contracts {
		res = append(res, mapToContractResponse(&c))
	}

	response.Success(c, http.StatusOK, "Contracts retrieved successfully", res)
}

func (h *ContractHandler) Get(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id := c.Param("id")

	contract, err := h.service.GetContract(c.Request.Context(), id, companyID)
	if err != nil {
		response.Error(c, http.StatusNotFound, "Contract not found", "")
		return
	}

	response.Success(c, http.StatusOK, "Contract retrieved successfully", mapToContractResponse(contract))
}

func (h *ContractHandler) Update(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id := c.Param("id")

	var req dto.UpdateContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	param := domain.UpdateContractParam{
		ScopeData:        req.ScopeData,
		DeliverablesData: req.DeliverablesData,
		CommercialTerms:  req.CommercialTerms,
		PaymentTerms:     req.PaymentTerms,
		TimelineData:     req.TimelineData,
		AdditionalTerms:  req.AdditionalTerms,
	}

	err := h.service.UpdateContract(c.Request.Context(), id, companyID, param)
	if err != nil {
		if err == service.ErrInvalidStatus {
			response.Error(c, http.StatusConflict, err.Error(), "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to update contract", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Contract updated successfully", nil)
}

func (h *ContractHandler) Share(c *gin.Context) {
	companyID := middleware.GetCompanyID(c)
	id := c.Param("id")

	contract, err := h.service.ShareContract(c.Request.Context(), id, companyID)
	if err != nil {
		if err == service.ErrInvalidStatus {
			response.Error(c, http.StatusConflict, err.Error(), "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to share contract", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Contract shared successfully", mapToContractResponse(contract))
}

func (h *ContractHandler) GetPublic(c *gin.Context) {
	token := c.Param("token")

	contract, err := h.service.GetPublicContract(c.Request.Context(), token)
	if err != nil {
		fmt.Printf("GetPublicContract error: %v\n", err)
		response.Error(c, http.StatusNotFound, "Contract not found", "")
		return
	}

	res := dto.PublicContractResponse{
		QuotationID:      contract.QuotationID,
		RevisionID:       contract.RevisionID,
		ContractNumber:   contract.ContractNumber,
		Status:           contract.Status,
		ClientSnapshot:   contract.ClientSnapshot,
		ScopeData:        contract.ScopeData,
		DeliverablesData: contract.DeliverablesData,
		CommercialTerms:  contract.CommercialTerms,
		PaymentTerms:     contract.PaymentTerms,
		TimelineData:     contract.TimelineData,
		AdditionalTerms:  contract.AdditionalTerms,
		SignedAt:         contract.SignedAt,
		CreatedAt:        contract.CreatedAt,
	}

	response.Success(c, http.StatusOK, "Contract retrieved successfully", res)
}

func (h *ContractHandler) SignPublic(c *gin.Context) {
	token := c.Param("token")

	var req dto.SignContractRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}

	ip := c.ClientIP()

	err := h.service.SignContract(c.Request.Context(), token, req.SignatureDataURL, ip)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to sign contract", err.Error())
		return
	}

	response.Success(c, http.StatusOK, "Contract signed successfully", nil)
}

func mapToContractResponse(c *domain.Contract) dto.ContractResponse {
	return dto.ContractResponse{
		ID:               c.ID,
		QuotationID:      c.QuotationID,
		RevisionID:       c.RevisionID,
		ContractNumber:   c.ContractNumber,
		PublicToken:      c.PublicToken,
		Status:           c.Status,
		ClientSnapshot:   c.ClientSnapshot,
		ScopeData:        c.ScopeData,
		DeliverablesData: c.DeliverablesData,
		CommercialTerms:  c.CommercialTerms,
		PaymentTerms:     c.PaymentTerms,
		TimelineData:     c.TimelineData,
		AdditionalTerms:  c.AdditionalTerms,
		SignedAt:         c.SignedAt,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        c.UpdatedAt,
	}
}
