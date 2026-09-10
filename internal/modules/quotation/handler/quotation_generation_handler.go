package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

// QuotationGenerationHandler exposes the AI-first quotation generation
// endpoints. It deliberately performs NO logging of request payloads, prompts
// or tokens; every failure surfaces as a safe error code produced by the
// service layer (GenerationError.Error() prints the code only).
type QuotationGenerationHandler struct {
	genSvc            *service.QuotationGenerationService
	genRepo           *repository.GenerationRepository
	previewCalculator dedicatedSquadPreviewCalculator
}

type dedicatedSquadPreviewCalculator interface {
	Calculate(ctx context.Context, companyID, rateCardID string, input domain.DedicatedSquadInput) (domain.DedicatedSquadCalculation, error)
}

func NewQuotationGenerationHandler(genSvc *service.QuotationGenerationService, genRepo *repository.GenerationRepository, previewCalculator ...dedicatedSquadPreviewCalculator) *QuotationGenerationHandler {
	h := &QuotationGenerationHandler{genSvc: genSvc, genRepo: genRepo}
	if len(previewCalculator) > 0 {
		h.previewCalculator = previewCalculator[0]
	}
	return h
}

// Preview computes server-authoritative Dedicated Squad figures without AI or persistence.
func (h *QuotationGenerationHandler) Preview(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	if req.DedicatedSquad == nil {
		response.Error(c, http.StatusUnprocessableEntity, "Dedicated squad input is required", service.ErrCodeAllocationInvalid)
		return
	}
	if req.CommercialScheme != "dedicated_squad" {
		response.Error(c, http.StatusUnprocessableEntity, "Dedicated squad commercial scheme is required", service.ErrCodeAllocationInvalid)
		return
	}
	if req.RateCardID == uuid.Nil {
		response.Error(c, http.StatusUnprocessableEntity, "Rate card is required", service.ErrCodeInvalidRateCard)
		return
	}
	if h.previewCalculator == nil {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	profile, err := service.ResolveDedicatedSquadProfile(*req.DedicatedSquad)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Dedicated squad allocation is invalid", service.ErrCodeAllocationInvalid)
		return
	}
	calculation, err := h.previewCalculator.Calculate(c.Request.Context(), companyID.String(), req.RateCardID.String(), *req.DedicatedSquad)
	if err != nil {
		if errors.Is(err, dto.ErrMissingRateItem) {
			response.Error(c, http.StatusUnprocessableEntity, "Rate item is unavailable", service.ErrCodeRateItemMissing)
			return
		}
		response.Error(c, http.StatusUnprocessableEntity, "Dedicated squad allocation is invalid", service.ErrCodeAllocationInvalid)
		return
	}
	rules, err := service.EvaluateDedicatedSquadRulesWithProfile(*req.DedicatedSquad, profile, calculation.DevelopmentSubtotal)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Dedicated squad allocation is invalid", service.ErrCodeAllocationInvalid)
		return
	}

	response.Success(c, http.StatusOK, "Dedicated squad preview calculated successfully", dto.DedicatedSquadPreviewResponse{
		Calculation: calculation, ResolvedProfile: profile, VisibleSections: service.BuildSectionPlan(rules),
		IncludeK6: rules.IncludeK6, IncludePentest: rules.IncludePentest,
	})
}

// PreviewSubscriptionSaaS computes server-authoritative Subscription SaaS figures without AI or persistence.
func (h *QuotationGenerationHandler) PreviewSubscriptionSaaS(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if _, ok := companyUUID(c); !ok {
		return
	}
	if req.CommercialScheme != domain.CommercialSchemeSubscriptionSaaS || req.SubscriptionSaaS == nil {
		response.Error(c, http.StatusUnprocessableEntity, "Subscription SaaS input is required", service.ErrCodeAllocationInvalid)
		return
	}
	calculation, err := service.CalculateSubscriptionSaaS(*req.SubscriptionSaaS)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Subscription SaaS input is invalid", service.ErrCodeAllocationInvalid)
		return
	}
	profile, err := service.ResolveSubscriptionSaaSProfile(*req.SubscriptionSaaS)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Subscription SaaS profile is invalid", service.ErrCodeAllocationInvalid)
		return
	}
	response.Success(c, http.StatusOK, "Subscription SaaS preview calculated successfully", dto.SubscriptionSaaSPreviewResponse{
		Calculation:     calculation,
		ResolvedProfile: profile,
		VisibleSections: service.BuildSubscriptionSectionPlan(profile),
	})
}

// PreviewWhiteLabel computes server-authoritative White Label figures without AI or persistence.
func (h *QuotationGenerationHandler) PreviewWhiteLabel(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request", err.Error())
		return
	}
	if _, ok := companyUUID(c); !ok {
		return
	}
	if req.CommercialScheme != domain.CommercialSchemeWhiteLabel || req.WhiteLabel == nil || req.DedicatedSquad != nil {
		response.Error(c, http.StatusBadRequest, "Invalid white label request", "")
		return
	}
	if err := req.WhiteLabel.Validate(); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid white label input", err.Error())
		return
	}
	calculation, err := service.CalculateWhiteLabel(*req.WhiteLabel)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "White label calculation mismatch", err.Error())
		return
	}
	profile, err := service.ResolveWhiteLabelProfile(*req.WhiteLabel)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid white label profile", err.Error())
		return
	}
	response.Success(c, http.StatusOK, "White label preview calculated successfully", dto.WhiteLabelPreviewResponse{Calculation: calculation, ResolvedProfile: profile})
}

// PreviewStandaloneMaintenance computes server-authoritative Standalone Maintenance figures without AI or persistence.
func (h *QuotationGenerationHandler) PreviewStandaloneMaintenance(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if _, ok := companyUUID(c); !ok {
		return
	}
	if req.CommercialScheme != domain.CommercialSchemeStandaloneMaintenance ||
		req.StandaloneMaintenance == nil ||
		req.DedicatedSquad != nil ||
		req.SubscriptionSaaS != nil ||
		req.WhiteLabel != nil {
		response.Error(c, http.StatusBadRequest, "Invalid standalone maintenance request", "")
		return
	}
	if req.RateCardID != uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Rate card is not allowed for standalone maintenance", "")
		return
	}
	if err := req.StandaloneMaintenance.Validate(); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid standalone maintenance input", "")
		return
	}
	calculation, err := service.CalculateStandaloneMaintenance(*req.StandaloneMaintenance)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Standalone maintenance calculation mismatch", "")
		return
	}
	profile := service.ResolveStandaloneMaintenanceProfile(*req.StandaloneMaintenance)
	response.Success(c, http.StatusOK, "Standalone maintenance preview calculated successfully", dto.StandaloneMaintenancePreviewResponse{
		Calculation:     calculation,
		ResolvedProfile: profile,
	})
}

// PreviewCustomDevelopment computes server-authoritative Custom Development figures without AI or persistence.
func (h *QuotationGenerationHandler) PreviewCustomDevelopment(c *gin.Context) {
	if _, ok := companyUUID(c); !ok {
		return
	}
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if req.CommercialScheme != domain.CommercialSchemeCustomDevelopment ||
		req.CustomDevelopment == nil ||
		req.DedicatedSquad != nil ||
		req.SubscriptionSaaS != nil ||
		req.WhiteLabel != nil ||
		req.StandaloneMaintenance != nil {
		response.Error(c, http.StatusBadRequest, "Invalid custom development request", "")
		return
	}
	if req.RateCardID != uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Rate card is not allowed for custom development", "")
		return
	}

	req.CustomDevelopment.SystemName = strings.TrimSpace(req.CustomDevelopment.SystemName)
	req.CustomDevelopment.DiscountLabel = strings.TrimSpace(req.CustomDevelopment.DiscountLabel)
	req.CustomDevelopment.ModuleGroups = service.NormalizeCustomDevelopmentModules(req.CustomDevelopment.ModuleGroups)
	req.CustomDevelopment.TimelineRows = service.NormalizeCustomDevelopmentTimeline(req.CustomDevelopment.TimelineRows)

	if err := req.CustomDevelopment.Validate(); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid custom development input", "")
		return
	}
	calculation, err := service.CalculateCustomDevelopment(*req.CustomDevelopment)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Custom development calculation mismatch", "")
		return
	}
	profile, err := service.ResolveCustomDevelopmentProfile(*req.CustomDevelopment)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid custom development profile", "")
		return
	}
	response.Success(c, http.StatusOK, "Custom development preview calculated successfully", dto.CustomDevelopmentPreviewResponse{
		Calculation:     calculation,
		ResolvedProfile: profile,
	})
}

// PreviewKioskQueueSystem computes server-authoritative Kiosk Queue System figures without AI or persistence.
func (h *QuotationGenerationHandler) PreviewKioskQueueSystem(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if _, ok := companyUUID(c); !ok {
		return
	}
	if req.CommercialScheme != domain.CommercialSchemeKioskQueueSystem || req.KioskQueueSystem == nil || hasNonKioskStructuredInput(req) {
		response.Error(c, http.StatusBadRequest, "Invalid kiosk queue request", "")
		return
	}
	if req.RateCardID != uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Rate card is not allowed for kiosk queue system", "")
		return
	}
	input := req.KioskQueueSystem.Normalize()
	if err := input.Validate(); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid kiosk queue input", "")
		return
	}
	calculation, err := service.CalculateKioskQueueSystem(input)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Kiosk queue calculation mismatch", "")
		return
	}
	profile, err := service.ResolveKioskQueueProfile(input)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid kiosk queue profile", "")
		return
	}
	response.Success(c, http.StatusOK, "Kiosk queue preview calculated successfully", dto.KioskQueuePreviewResponse{Calculation: calculation, ResolvedProfile: profile})
}

// PreviewTalentOutsourcing computes authoritative Talent Outsourcing figures
// without AI, rate-card access, or persistence.
func (h *QuotationGenerationHandler) PreviewTalentOutsourcing(c *gin.Context) {
	if _, ok := companyUUID(c); !ok {
		return
	}

	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if req.CommercialScheme != domain.CommercialSchemeTalentOutsourcing ||
		req.TalentOutsourcing == nil ||
		req.DedicatedSquad != nil ||
		req.SubscriptionSaaS != nil ||
		req.WhiteLabel != nil ||
		req.StandaloneMaintenance != nil ||
		req.CustomDevelopment != nil ||
		req.KioskQueueSystem != nil {
		response.Error(c, http.StatusBadRequest, "Invalid talent outsourcing request", "")
		return
	}
	if req.RateCardID != uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Rate card is not allowed for talent outsourcing", "")
		return
	}

	input := domain.NormalizeTalentOutsourcingInput(*req.TalentOutsourcing)
	if err := input.Validate(); err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid talent outsourcing input", "")
		return
	}
	calculation, err := service.CalculateTalentOutsourcing(input)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Talent outsourcing calculation mismatch", "")
		return
	}
	profile, err := service.ResolveTalentOutsourcingProfile(input)
	if err != nil {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid talent outsourcing profile", "")
		return
	}
	response.Success(c, http.StatusOK, "Talent outsourcing preview calculated successfully", dto.TalentOutsourcingPreviewResponse{
		Calculation:     calculation,
		ResolvedProfile: profile,
	})
}

func hasNonKioskStructuredInput(req dto.GenerateQuotationRequest) bool {
	return req.DedicatedSquad != nil ||
		req.SubscriptionSaaS != nil ||
		req.WhiteLabel != nil ||
		req.StandaloneMaintenance != nil ||
		req.CustomDevelopment != nil ||
		req.TalentOutsourcing != nil
}

// Generate starts the AI-first pipeline and returns 201 with the committed
// quotation ID. The draft is durable before any provider interaction, so a
// later failure remains retryable and idempotent replay (same
// idempotency_key) returns the same aggregate.
func (h *QuotationGenerationHandler) Generate(c *gin.Context) {
	var req dto.GenerateQuotationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", err.Error())
		return
	}

	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	var createdBy *uuid.UUID
	if rawID, uerr := middleware.GetUserID(c); uerr == nil {
		if uid, perr := uuid.Parse(rawID); perr == nil {
			createdBy = &uid
		}
	}

	quotationID, err := h.genSvc.Generate(c.Request.Context(), companyID, createdBy, req)
	if err != nil {
		h.mapGenerationError(c, err)
		return
	}
	response.Success(c, http.StatusCreated, "Quotation generated successfully", gin.H{
		"quotation_id": quotationID,
	})
}

// Regenerate re-runs the pipeline for an active draft. A successful response
// reports the refreshed generation state including the incremented
// content_version; failures never alter stored content.
//
// Contract note (deferred Session-4 decision): while a draft is pending its
// idempotent generate replays still return success with the same quotation
// ID — clients MUST consult GET /:id/generation-status for authoritative
// status instead of inferring it from generate/regenerate responses.
func (h *QuotationGenerationHandler) Regenerate(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	if err := h.genSvc.Regenerate(c.Request.Context(), companyID, quotationID); err != nil {
		h.mapRegenerateError(c, err)
		return
	}

	state, serr := h.genRepo.GetGenerationState(c.Request.Context(), companyID, quotationID)
	if serr != nil {
		response.Error(c, http.StatusInternalServerError, "Failed to load generation state", "")
		return
	}
	response.Success(c, http.StatusOK, "Quotation regenerated successfully", state)
}

// GetByID returns the AI-first detail view of a quotation: master identity
// plus the current revision's generation state (status, content_version,
// content_html). Budget figures and internal snapshots are excluded. A
// foreign or unknown aggregate answers 404 so existence is never leaked
// across tenants.
//
// NOTE: this replaces the previous route target, whose service method was an
// unconditional "not implemented" stub — no working behaviour changed hands.
func (h *QuotationGenerationHandler) GetByID(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	detail, derr := h.genRepo.GetGenerationDetail(c.Request.Context(), companyID, quotationID)
	if derr != nil {
		if errors.Is(derr, repository.ErrRevisionNotFound) {
			response.Error(c, http.StatusNotFound, "Quotation not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to load quotation", "")
		return
	}
	response.Success(c, http.StatusOK, "Quotation retrieved successfully", detail)
}

// GetGenerationStatus returns the safe generation view of a quotation's
// current revision: status, content_version, content_html and timestamps.
// Budget figures and internal snapshots are intentionally excluded.
func (h *QuotationGenerationHandler) GetGenerationStatus(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	state, serr := h.genRepo.GetGenerationState(c.Request.Context(), companyID, quotationID)
	if serr != nil {
		if errors.Is(serr, repository.ErrRevisionNotFound) {
			response.Error(c, http.StatusNotFound, "Quotation not found", "")
			return
		}
		response.Error(c, http.StatusInternalServerError, "Failed to load generation state", "")
		return
	}
	response.Success(c, http.StatusOK, "Generation status retrieved successfully", state)
}

// SaveDraft persists user edits to a quotation's draft revision with optimistic locking.
func (h *QuotationGenerationHandler) SaveDraft(c *gin.Context) {
	quotationID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid quotation id", "")
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}

	var req dto.SaveDraftRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "")
		return
	}
	if req.ContentVersion < 1 {
		response.Error(c, http.StatusBadRequest, "Invalid content version", "")
		return
	}

	var userID *uuid.UUID
	if rawID, uerr := middleware.GetUserID(c); uerr == nil {
		if uid, perr := uuid.Parse(rawID); perr == nil {
			userID = &uid
		}
	}

	res, err := h.genSvc.SaveDraft(c.Request.Context(), companyID, quotationID, userID, req)
	if err != nil {
		h.mapSaveDraftError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Draft saved successfully", res)
}

// mapSaveDraftError distinguishes tenant isolation 404, conflict 409, and DOM safety 422.
func (h *QuotationGenerationHandler) mapSaveDraftError(c *gin.Context, err error) {
	if errors.Is(err, repository.ErrRevisionNotFound) {
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
		return
	}
	if errors.Is(err, repository.ErrRevisionConflict) {
		response.Error(c, http.StatusConflict, "Revision conflict: stale content version", service.ErrCodeRevisionConflict)
		return
	}
	var ge *service.GenerationError
	if errors.As(err, &ge) {
		if ge.Code == service.ErrCodeRevisionConflict {
			response.Error(c, http.StatusConflict, "Revision conflict: stale content version", ge.Error())
			return
		}
		if ge.Code == service.ErrCodeUnsafeHTML {
			response.Error(c, http.StatusUnprocessableEntity, "Unsafe HTML or integrity validation failed", ge.Error())
			return
		}
	}
	h.mapGenerationError(c, err)
}

// mapRegenerateError distinguishes the tenant-isolation miss (404, no
// existence leak across companies) from service-level safe codes.
func (h *QuotationGenerationHandler) mapRegenerateError(c *gin.Context, err error) {
	if errors.Is(err, repository.ErrRevisionNotFound) {
		response.Error(c, http.StatusNotFound, "Quotation not found", "")
		return
	}
	h.mapGenerationError(c, err)
}

// mapGenerationError translates *service.GenerationError into HTTP responses.
// Every code maps to a static message and errors.detail carries ONLY the safe
// code itself (GenerationError.Error() == code), so no provider payload,
// stack trace or internal note reaches the client.
//
// SANCTIONED EXCEPTION: budget_unfeasible may carry a Summary (last allocation
// attempt: stable roles + rate-card-derived total + budget excess). These are
// figures the client already sees in the rendered document — explicitly
// permitted per service.GenerationError documentation.
func (h *QuotationGenerationHandler) mapGenerationError(c *gin.Context, err error) {
	var ge *service.GenerationError
	if !errors.As(err, &ge) {
		response.Error(c, http.StatusInternalServerError, "Internal server error", "")
		return
	}

	if ge.Code == service.ErrCodeBudgetUnfeasible && ge.Summary != "" {
		response.Error(c, http.StatusUnprocessableEntity, ge.Summary, ge.Error())
		return
	}

	switch ge.Code {
	case service.ErrCodeQuotationNumberConflict:
		response.Error(c, http.StatusConflict, "Quotation number already exists", ge.Error())
	case service.ErrCodeRegenerateRateLimited:
		response.Error(c, http.StatusTooManyRequests, "Regenerate limit reached", ge.Error())
	case service.ErrCodeRevisionConflict:
		response.Error(c, http.StatusConflict, "Revision is not regenerable", ge.Error())
	default:
		// settings_incomplete, invalid_rate_card, budget_unfeasible,
		// allocation_invalid, provider_*, unsafe_html, generation_failed:
		// business/configuration/provider conditions that the request body
		// cannot mechanically fix, hence 422 with the bare code.
		response.Error(c, http.StatusUnprocessableEntity, "Generation failed", ge.Error())
	}
}

// companyUUID resolves the tenant from JWT claims set by middleware.
func companyUUID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(middleware.GetCompanyID(c))
	if err != nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return uuid.Nil, false
	}
	return id, true
}
