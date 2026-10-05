package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidExtractionTransition = errors.New("invalid extraction state transition")

type ExtractionRunStatus string

const (
	ExtractionRunQueued           ExtractionRunStatus = "queued"
	ExtractionRunRunning          ExtractionRunStatus = "running"
	ExtractionRunWaitingRateLimit ExtractionRunStatus = "waiting_rate_limit"
	ExtractionRunInputRequired    ExtractionRunStatus = "input_required"
	ExtractionRunNeedsReview      ExtractionRunStatus = "needs_review"
	ExtractionRunFailed           ExtractionRunStatus = "failed"
	ExtractionRunSucceeded        ExtractionRunStatus = "succeeded"
	ExtractionRunCancelled        ExtractionRunStatus = "cancelled"
)

type ExtractionPhaseStatus string

const (
	ExtractionPhasePending   ExtractionPhaseStatus = "pending"
	ExtractionPhaseRunning   ExtractionPhaseStatus = "running"
	ExtractionPhaseSucceeded ExtractionPhaseStatus = "succeeded"
	ExtractionPhaseFailed    ExtractionPhaseStatus = "failed"
	ExtractionPhaseSkipped   ExtractionPhaseStatus = "skipped"
)

const (
	ExtractionPhasePlanMetadata = "plan_metadata"
	ExtractionPhaseScope        = "scope"
	ExtractionPhaseStaffing     = "staffing"
	ExtractionPhaseService      = "service_details"
	ExtractionPhaseFinalize     = "finalize"
)

var ExtractionPhaseOrder = []string{
	ExtractionPhasePlanMetadata,
	ExtractionPhaseScope,
	ExtractionPhaseStaffing,
	ExtractionPhaseService,
	ExtractionPhaseFinalize,
}

// CommercialSchemes is the single, ordered allowlist used by extraction
// validation. It intentionally excludes planner-only unknown and multi_service
// outcomes, neither of which is a valid quotation scheme.
func CommercialSchemes() []string {
	return []string{
		CommercialSchemeDedicatedSquad,
		CommercialSchemeSubscriptionSaaS,
		CommercialSchemeWhiteLabel,
		CommercialSchemeStandaloneMaintenance,
		CommercialSchemeCustomDevelopment,
		CommercialSchemeKioskQueueSystem,
		CommercialSchemeTalentOutsourcing,
		CommercialSchemeCloudInfrastructure,
	}
}

func IsCommercialScheme(value string) bool {
	for _, scheme := range CommercialSchemes() {
		if value == scheme {
			return true
		}
	}
	return false
}

type ExtractionRun struct {
	ID              uuid.UUID
	CompanyID       uuid.UUID
	QuotationID     uuid.UUID
	RevisionID      uuid.UUID
	Status          ExtractionRunStatus
	NoteHash        string
	PipelineVersion string
	LeaseOwner      *string
	LeaseExpiresAt  *time.Time
	Fence           int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type ExtractionPhase struct {
	ID        uuid.UUID
	RunID     uuid.UUID
	PhaseKey  string
	Ordinal   int16
	Status    ExtractionPhaseStatus
	Fence     int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (from ExtractionRunStatus) CanTransitionTo(to ExtractionRunStatus) bool {
	switch from {
	case ExtractionRunQueued:
		return to == ExtractionRunRunning || to == ExtractionRunCancelled
	case ExtractionRunRunning:
		return to == ExtractionRunWaitingRateLimit || to == ExtractionRunInputRequired || to == ExtractionRunNeedsReview || to == ExtractionRunFailed || to == ExtractionRunSucceeded || to == ExtractionRunCancelled || to == ExtractionRunQueued
	case ExtractionRunWaitingRateLimit, ExtractionRunInputRequired:
		return to == ExtractionRunQueued || to == ExtractionRunCancelled
	case ExtractionRunFailed:
		return to == ExtractionRunQueued
	default:
		return false
	}
}

func (from ExtractionPhaseStatus) CanTransitionTo(to ExtractionPhaseStatus) bool {
	switch from {
	case ExtractionPhasePending:
		return to == ExtractionPhaseRunning || to == ExtractionPhaseSkipped
	case ExtractionPhaseRunning:
		return to == ExtractionPhaseSucceeded || to == ExtractionPhaseFailed || to == ExtractionPhasePending
	case ExtractionPhaseFailed:
		return to == ExtractionPhaseRunning
	default:
		return false
	}
}
