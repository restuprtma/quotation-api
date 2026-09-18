package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

// MapBlueprintError translates domain, service, and repository errors into safe HTTP responses.
// Secrets, raw database errors, and stack traces are completely redacted.
func MapBlueprintError(c *gin.Context, err error) {
	if err == nil {
		return
	}

	// 1. Manual Validation Error
	var mve *service.BlueprintManualValidationError
	if errors.As(err, &mve) {
		if errors.Is(mve.BaseErr, service.ErrBlueprintSecurityFailure) || mve.SafeOutcome == "security_failure" {
			response.Error(c, http.StatusUnprocessableEntity, "Security validation failed", "security_failure")
			return
		}
		if errors.Is(mve.BaseErr, service.ErrBlueprintInvalidSchema) || mve.SafeOutcome == "invalid_schema" {
			response.Error(c, http.StatusUnprocessableEntity, "Invalid schema", "invalid_schema")
			return
		}
		outcome := mve.SafeOutcome
		if outcome == "" {
			outcome = "invalid_schema"
		}
		response.Error(c, http.StatusUnprocessableEntity, "Invalid manual completion payload", outcome)
		return
	}

	// 2. 400 Bad Request
	if errors.Is(err, service.ErrBlueprintGenerationInvalidInput) ||
		errors.Is(err, service.ErrBlueprintPipelineInvalidInput) ||
		errors.Is(err, repository.ErrBlueprintInvalidCreateParams) {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "blueprint_invalid_input")
		return
	}

	// 3. 404 Not Found
	if errors.Is(err, repository.ErrBlueprintSourceNotFound) ||
		errors.Is(err, service.ErrBlueprintGenerationTenantMismatch) {
		response.Error(c, http.StatusNotFound, "Source not found", "source_not_found")
		return
	}
	if errors.Is(err, repository.ErrBlueprintNotFound) ||
		errors.Is(err, service.ErrBlueprintNotFound) ||
		errors.Is(err, service.ErrBlueprintPipelineNotFound) {
		response.Error(c, http.StatusNotFound, "Blueprint not found", "blueprint_not_found")
		return
	}

	// 4. 409 Conflict
	if errors.Is(err, repository.ErrBlueprintAlreadyExists) {
		response.Error(c, http.StatusConflict, "Blueprint already exists", "blueprint_already_exists")
		return
	}
	if errors.Is(err, repository.ErrBlueprintIdempotencyPayloadMismatch) {
		response.Error(c, http.StatusConflict, "Idempotency payload mismatch", "idempotency_payload_mismatch")
		return
	}
	if errors.Is(err, service.ErrBlueprintPipelineBusy) ||
		errors.Is(err, repository.ErrBlueprintClaimBusy) {
		response.Error(c, http.StatusConflict, "Pipeline is busy", "pipeline_busy")
		return
	}
	if errors.Is(err, service.ErrBlueprintPipelineBlocked) ||
		errors.Is(err, repository.ErrBlueprintNoEligibleSection) {
		response.Error(c, http.StatusConflict, "Pipeline is blocked", "pipeline_blocked")
		return
	}
	if errors.Is(err, service.ErrBlueprintPipelineRecoveryRequired) ||
		errors.Is(err, repository.ErrBlueprintRecoveryRequired) {
		response.Error(c, http.StatusConflict, "Pipeline recovery required", "recovery_required")
		return
	}
	if errors.Is(err, service.ErrBlueprintSectionRecoveryRequired) {
		response.Error(c, http.StatusConflict, "Section recovery required", "section_recovery_required")
		return
	}
	if errors.Is(err, service.ErrBlueprintRetryNotAllowed) ||
		errors.Is(err, repository.ErrBlueprintSectionNotRetryable) {
		response.Error(c, http.StatusConflict, "Retry not allowed", "retry_not_allowed")
		return
	}
	if errors.Is(err, service.ErrBlueprintAttemptLimitReached) ||
		errors.Is(err, repository.ErrBlueprintAttemptLimit) {
		response.Error(c, http.StatusConflict, "Attempt limit reached", "attempt_limit_reached")
		return
	}
	if errors.Is(err, service.ErrBlueprintManualNotAllowed) {
		response.Error(c, http.StatusConflict, "Manual completion not allowed", "manual_completion_not_allowed")
		return
	}
	if errors.Is(err, service.ErrBlueprintExecutionConflict) ||
		errors.Is(err, repository.ErrBlueprintClaimConflict) ||
		errors.Is(err, repository.ErrBlueprintContentConflict) ||
		errors.Is(err, repository.ErrBlueprintManualConflict) {
		response.Error(c, http.StatusConflict, "Execution conflict", "execution_conflict")
		return
	}

	// 5. 422 Unprocessable Entity
	if errors.Is(err, repository.ErrBlueprintSourceIneligible) {
		response.Error(c, http.StatusUnprocessableEntity, "Blueprint source ineligible", "source_ineligible")
		return
	}
	if errors.Is(err, repository.ErrSkillNotFound) {
		response.Error(c, http.StatusUnprocessableEntity, "Active blueprint skill not configured", "skill_not_configured")
		return
	}
	if errors.Is(err, service.ErrBlueprintInvalidSchema) {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid schema", "invalid_schema")
		return
	}
	if errors.Is(err, service.ErrBlueprintSecurityFailure) {
		response.Error(c, http.StatusUnprocessableEntity, "Security failure", "security_failure")
		return
	}
	if errors.Is(err, service.ErrBlueprintCrossValidationFailure) {
		response.Error(c, http.StatusUnprocessableEntity, "Cross validation failure", "cross_validation_error")
		return
	}
	if errors.Is(err, service.ErrBlueprintProviderConfiguration) {
		response.Error(c, http.StatusUnprocessableEntity, "Provider configuration error", "provider_configuration")
		return
	}
	if errors.Is(err, service.ErrBlueprintProviderInvalidCredential) {
		response.Error(c, http.StatusUnprocessableEntity, "Invalid provider credential", "provider_invalid_credential")
		return
	}

	// 6. 429 Too Many Requests
	if errors.Is(err, service.ErrBlueprintProviderRateLimited) {
		response.Error(c, http.StatusTooManyRequests, "Provider rate limited", "provider_rate_limited")
		return
	}

	// 7. 503 Service Unavailable
	if errors.Is(err, service.ErrBlueprintProviderUnavailable) {
		response.Error(c, http.StatusServiceUnavailable, "Provider unavailable", "provider_unavailable")
		return
	}

	// 8. 504 Gateway Timeout
	if errors.Is(err, service.ErrBlueprintProviderTimeout) {
		response.Error(c, http.StatusGatewayTimeout, "Provider timeout", "provider_timeout")
		return
	}

	// 9. 500 Internal Server Error
	if errors.Is(err, service.ErrBlueprintPersistenceFailure) ||
		errors.Is(err, service.ErrCorruptBlueprintContent) {
		response.Error(c, http.StatusInternalServerError, "Persistence failure", "persistence_failure")
		return
	}

	// Default fallback: Redact all details and return generic 500
	response.Error(c, http.StatusInternalServerError, "Internal server error", "")
}
