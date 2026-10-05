package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ExtractionOrchestratorRunCheckpoint is a repository-neutral projection for
// the service orchestrator. It intentionally excludes notes, prompts, keys,
// provider bodies, and other sensitive inputs.
type ExtractionOrchestratorRunCheckpoint struct {
	Status     domain.ExtractionRunStatus
	RetryAfter time.Time
	CallsUsed  int
	CallsMax   int
}

type ExtractionOrchestratorPhaseCheckpoint struct {
	Status           domain.ExtractionPhaseStatus
	InputFingerprint string
	ResultJSON       []byte
	TransientRetries int
	RepairRetries    int
}

func (r *MeetingExtractionRepository) LoadOrchestratorRun(ctx context.Context, runID uuid.UUID, expectedFence int64) (ExtractionOrchestratorRunCheckpoint, error) {
	var checkpoint ExtractionOrchestratorRunCheckpoint
	var retryAfter *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT status, retry_after, calls_used, calls_max
		FROM quotation_extraction_runs
		WHERE id = $1 AND fence = $2
	`, runID, expectedFence).Scan(&checkpoint.Status, &retryAfter, &checkpoint.CallsUsed, &checkpoint.CallsMax)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExtractionOrchestratorRunCheckpoint{}, ErrExtractionFenceLost
	}
	if err != nil {
		return ExtractionOrchestratorRunCheckpoint{}, err
	}
	if retryAfter != nil {
		checkpoint.RetryAfter = retryAfter.UTC()
	}
	return checkpoint, nil
}

func (r *MeetingExtractionRepository) LoadOrchestratorPhase(ctx context.Context, runID uuid.UUID, phaseKey string, expectedFence int64) (ExtractionOrchestratorPhaseCheckpoint, error) {
	var checkpoint ExtractionOrchestratorPhaseCheckpoint
	var inputFingerprint *string
	err := r.db.QueryRow(ctx, `
		SELECT phase.status, phase.input_fingerprint, phase.result_json,
		       phase.transient_retries, phase.repair_retries
		FROM quotation_extraction_phases phase
		JOIN quotation_extraction_runs run ON run.id = phase.run_id
		WHERE phase.run_id = $1 AND phase.phase_key = $2
		  AND run.status IN ('queued','running','waiting_rate_limit')
		  AND run.fence = $3
	`, runID, phaseKey, expectedFence).Scan(&checkpoint.Status, &inputFingerprint, &checkpoint.ResultJSON, &checkpoint.TransientRetries, &checkpoint.RepairRetries)
	if errors.Is(err, pgx.ErrNoRows) {
		return ExtractionOrchestratorPhaseCheckpoint{}, ErrExtractionFenceLost
	}
	if err != nil {
		return ExtractionOrchestratorPhaseCheckpoint{}, err
	}
	if inputFingerprint != nil {
		checkpoint.InputFingerprint = *inputFingerprint
	}
	return checkpoint, nil
}

func (r *MeetingExtractionRepository) ResumeRateLimited(ctx context.Context, runID uuid.UUID, expectedFence int64) error {
	result, err := r.db.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET status = 'queued', retry_after = NULL, lease_owner = NULL,
		    lease_expires_at = NULL, error_code = NULL, retryable = false, updated_at = $1
		WHERE id = $2 AND status = 'waiting_rate_limit' AND fence = $3
	`, r.now().UTC(), runID, expectedFence)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return nil
}

// RequeueWithRetryAfter releases a claimed worker lease after a transient
// local contention (for example an occupied provider advisory slot). The
// fence predicate means a reclaimed worker can never delay the new owner.
func (r *MeetingExtractionRepository) RequeueWithRetryAfter(ctx context.Context, runID uuid.UUID, expectedFence int64, retryAfter time.Time) error {
	if retryAfter.IsZero() {
		return ErrExtractionFenceLost
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := r.now().UTC()
	result, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET status = 'queued', retry_after = $1, lease_owner = NULL,
		    lease_expires_at = NULL, error_code = NULL, retryable = true, updated_at = $2
		WHERE id = $3 AND status = 'running' AND fence = $4
	`, retryAfter.UTC(), now, runID, expectedFence)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	// Provider-slot contention occurs after the phase has entered running.
	// Return an uncommitted phase to pending with the same fence so the next
	// worker can safely begin it after claiming a new fence.
	_, err = tx.Exec(ctx, `
		UPDATE quotation_extraction_phases
		SET status = 'pending', started_at = NULL, completed_at = NULL, updated_at = $1
		WHERE run_id = $2 AND status = 'running' AND fence = $3
		  AND result_json IS NULL AND input_fingerprint IS NULL
	`, now, runID, expectedFence)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *MeetingExtractionRepository) SkipPhaseWithFence(ctx context.Context, runID uuid.UUID, phaseKey string, expectedFence int64, fingerprint string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE quotation_extraction_phases phase
		SET status = 'skipped', input_fingerprint = $1, fence = $2,
		    started_at = NULL, completed_at = $3, updated_at = $3
		FROM quotation_extraction_runs run
		WHERE phase.run_id = $4 AND phase.phase_key = $5
		  AND phase.status = 'pending' AND run.id = phase.run_id
		  AND run.status = 'running' AND run.fence = $2
	`, fingerprint, expectedFence, r.now().UTC(), runID, phaseKey)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return nil
}

func (r *MeetingExtractionRepository) WaitRateLimit(ctx context.Context, runID uuid.UUID, phaseKey string, expectedFence int64, retryAfter time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := r.now().UTC()
	phaseResult, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_phases phase
		SET status = 'pending', fence = $1, started_at = NULL, completed_at = NULL,
		    error_code = NULL, retryable = false, updated_at = $2
		FROM quotation_extraction_runs run
		WHERE phase.run_id = $3 AND phase.phase_key = $4 AND phase.status = 'running'
		  AND run.id = phase.run_id AND run.status = 'running' AND run.fence = $1
	`, expectedFence, now, runID, phaseKey)
	if err != nil {
		return err
	}
	if phaseResult.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	runResult, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET status = 'waiting_rate_limit', retry_after = $1, error_code = 'PROVIDER_RATE_LIMITED',
		    retryable = true, lease_owner = NULL, lease_expires_at = NULL, updated_at = $2
		WHERE id = $3 AND status = 'running' AND fence = $4
	`, retryAfter.UTC(), now, runID, expectedFence)
	if err != nil {
		return err
	}
	if runResult.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return tx.Commit(ctx)
}

func (r *MeetingExtractionRepository) MarkExtractionNeedsReview(ctx context.Context, runID uuid.UUID, expectedFence int64, code string) error {
	if code != "SCHEME_REVIEW_REQUIRED" && code != "SCHEME_MAPPER_UNAVAILABLE" {
		code = "SCHEME_REVIEW_REQUIRED"
	}
	result, err := r.db.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET status = 'needs_review', error_code = $1, retryable = false,
		    lease_owner = NULL, lease_expires_at = NULL, completed_at = $2, updated_at = $2
		WHERE id = $3 AND status = 'running' AND fence = $4
	`, code, r.now().UTC(), runID, expectedFence)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return nil
}

func (r *MeetingExtractionRepository) MarkExtractionFailed(ctx context.Context, runID uuid.UUID, phaseKey string, expectedFence int64, code string, retryable bool) error {
	if !safeExtractionErrorCode(code) {
		code = "INTERNAL_ERROR"
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := r.now().UTC()
	phaseResult, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_phases phase
		SET status = 'failed', error_code = $1, retryable = $2, fence = $3,
		    completed_at = $4, updated_at = $4
		FROM quotation_extraction_runs run
		WHERE phase.run_id = $5 AND phase.phase_key = $6
		  AND phase.status IN ('pending','running') AND run.id = phase.run_id
		  AND run.status = 'running' AND run.fence = $3
	`, code, retryable, expectedFence, now, runID, phaseKey)
	if err != nil {
		return err
	}
	if phaseResult.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	runResult, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET status = 'failed', error_code = $1, retryable = $2,
		    lease_owner = NULL, lease_expires_at = NULL, completed_at = $3, updated_at = $3
		WHERE id = $4 AND status = 'running' AND fence = $5
	`, code, retryable, now, runID, expectedFence)
	if err != nil {
		return err
	}
	if runResult.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return tx.Commit(ctx)
}

func safeExtractionErrorCode(code string) bool {
	switch code {
	case "PROVIDER_TIMEOUT", "PROVIDER_RATE_LIMITED", "PROVIDER_UNAVAILABLE", "INVALID_JSON", "SCHEMA_VALIDATION_FAILED", "EVIDENCE_MISMATCH", "AI_BUDGET_EXHAUSTED", "MEETING_NOTES_CHANGED", "SCHEME_MAPPER_UNAVAILABLE", "INTERNAL_ERROR":
		return true
	default:
		return false
	}
}
