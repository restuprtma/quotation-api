package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	ErrNoExtractionRunAvailable  = errors.New("no extraction run available")
	ErrExtractionFenceLost       = errors.New("extraction lease fence lost")
	ErrExtractionActiveRun       = errors.New("different active extraction run exists")
	ErrExtractionNotFound        = errors.New("extraction run not found")
	ErrExtractionBudgetExhausted = errors.New("extraction call budget exhausted")
	ErrExtractionCircuitOpen     = errors.New("extraction provider circuit open")
	ErrProviderSlotUnavailable   = errors.New("extraction provider slot unavailable")
)

type ExtractionCallReservationParams struct {
	RunID                  uuid.UUID
	QuotationID            uuid.UUID
	Provider               string
	Model                  string
	DailyProviderCallsMax  int64
	DailyQuotationCallsMax int64
}

// ExtractionProviderOutcomeParams deliberately accepts only a retryability
// classification. Provider bodies and upstream error text must never reach
// persistence while updating the circuit breaker.
type ExtractionProviderOutcomeParams struct {
	RunID            uuid.UUID
	Fence            int64
	Provider         string
	Model            string
	Retryable        bool
	FailureThreshold int
	Cooldown         time.Duration
}

// ReserveExtractionCallFenced is the worker-facing form of call reservation.
// It verifies the claimed run fence before consuming any run/provider/day
// quota, so a reclaimed worker cannot reserve a call for the new owner.
func (r *MeetingExtractionRepository) ReserveExtractionCallFenced(ctx context.Context, p ExtractionCallReservationParams, expectedFence int64) error {
	if p.RunID == uuid.Nil || strings.TrimSpace(p.Provider) == "" || strings.TrimSpace(p.Model) == "" || p.DailyProviderCallsMax <= 0 || p.DailyQuotationCallsMax <= 0 || expectedFence < 0 {
		return ErrExtractionBudgetExhausted
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var quotationID uuid.UUID
	if err = tx.QueryRow(ctx, `
		SELECT quotation_id
		FROM quotation_extraction_runs
		WHERE id = $1 AND status = 'running' AND fence = $2
		FOR UPDATE
	`, p.RunID, expectedFence).Scan(&quotationID); errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionFenceLost
	} else if err != nil {
		return err
	}

	now := r.now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	// Lock/create the provider-model daily state before consuming any quota.
	// The provider advisory slot held by the caller serializes the half-open
	// probe across replicas; this row lock makes the open check atomic too.
	var openUntil *time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO quotation_extraction_provider_state (provider, model, usage_date, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (provider, model, usage_date) DO UPDATE
		SET updated_at = EXCLUDED.updated_at
		RETURNING circuit_open_until
	`, p.Provider, p.Model, dayStart, now).Scan(&openUntil)
	if err != nil {
		return err
	}
	if openUntil != nil && now.Before(openUntil.UTC()) {
		return ErrExtractionCircuitOpen
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "meeting-extraction-quotation:"+quotationID.String()); err != nil {
		return err
	}
	var quotationUsed int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(calls_used), 0) FROM quotation_extraction_runs WHERE quotation_id = $1 AND created_at >= $2 AND created_at < $3`, quotationID, dayStart, dayStart.AddDate(0, 0, 1)).Scan(&quotationUsed); err != nil {
		return err
	}
	if quotationUsed >= p.DailyQuotationCallsMax {
		return ErrExtractionBudgetExhausted
	}
	var callsUsed int16
	err = tx.QueryRow(ctx, `
		UPDATE quotation_extraction_runs
		SET calls_used = calls_used + 1, updated_at = $1
		WHERE id = $2 AND status = 'running' AND fence = $3 AND calls_used < calls_max AND calls_used < 10
		RETURNING calls_used
	`, now, p.RunID, expectedFence).Scan(&callsUsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionBudgetExhausted
	}
	if err != nil {
		return err
	}
	var providerCalls int64
	err = tx.QueryRow(ctx, `
		UPDATE quotation_extraction_provider_state
		SET calls_used = calls_used + 1, updated_at = $1
		WHERE provider = $2 AND model = $3 AND usage_date = $4 AND calls_used < $5
		RETURNING calls_used
	`, now, p.Provider, p.Model, dayStart, p.DailyProviderCallsMax).Scan(&providerCalls)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionBudgetExhausted
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordExtractionProviderOutcome advances the provider/model circuit using
// only safe aggregate state. A success closes the half-open circuit; a
// retryable provider failure increments the consecutive failure counter and
// opens it once the configured threshold is met.
func (r *MeetingExtractionRepository) RecordExtractionProviderOutcome(ctx context.Context, p ExtractionProviderOutcomeParams) error {
	if p.RunID == uuid.Nil || p.Fence < 0 || strings.TrimSpace(p.Provider) == "" || strings.TrimSpace(p.Model) == "" || p.FailureThreshold <= 0 || p.Cooldown <= 0 {
		return ErrExtractionFenceLost
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM quotation_extraction_runs WHERE id = $1 AND status = 'running' AND fence = $2)`, p.RunID, p.Fence).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrExtractionFenceLost
	}
	now := r.now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_extraction_provider_state (provider, model, usage_date, consecutive_transient_failures, circuit_open_until, updated_at)
		VALUES ($1, $2, $3, CASE WHEN $4 THEN 1 ELSE 0 END,
			CASE WHEN $4 AND $5 <= 1 THEN $6::timestamptz ELSE NULL END, $7)
		ON CONFLICT (provider, model, usage_date) DO UPDATE SET
			consecutive_transient_failures = CASE WHEN $4 THEN quotation_extraction_provider_state.consecutive_transient_failures + 1 ELSE 0 END,
			circuit_open_until = CASE
				WHEN NOT $4 THEN NULL
				WHEN quotation_extraction_provider_state.consecutive_transient_failures + 1 >= $5 THEN $6::timestamptz
				ELSE NULL
			END,
			updated_at = $7
	`, p.Provider, p.Model, dayStart, p.Retryable, p.FailureThreshold, now.Add(p.Cooldown), now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReserveExtractionCall conservatively consumes the run, provider-day and
// quotation-day budgets. A crash after this transaction still consumes quota.
func (r *MeetingExtractionRepository) ReserveExtractionCall(ctx context.Context, p ExtractionCallReservationParams) error {
	if p.RunID == uuid.Nil || p.QuotationID == uuid.Nil || strings.TrimSpace(p.Provider) == "" || strings.TrimSpace(p.Model) == "" || p.DailyProviderCallsMax <= 0 || p.DailyQuotationCallsMax <= 0 {
		return ErrExtractionBudgetExhausted
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	now := r.now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "meeting-extraction-quotation:"+p.QuotationID.String()); err != nil {
		return err
	}
	var quotationUsed int64
	if err = tx.QueryRow(ctx, `SELECT COALESCE(SUM(calls_used), 0) FROM quotation_extraction_runs WHERE quotation_id = $1 AND created_at >= $2 AND created_at < $3`, p.QuotationID, dayStart, dayStart.AddDate(0, 0, 1)).Scan(&quotationUsed); err != nil {
		return err
	}
	if quotationUsed >= p.DailyQuotationCallsMax {
		return ErrExtractionBudgetExhausted
	}
	var callsUsed int16
	err = tx.QueryRow(ctx, `UPDATE quotation_extraction_runs SET calls_used = calls_used + 1, updated_at = $1 WHERE id = $2 AND calls_used < calls_max AND calls_used < 10 RETURNING calls_used`, now, p.RunID).Scan(&callsUsed)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionBudgetExhausted
	}
	if err != nil {
		return err
	}
	var providerCalls int64
	err = tx.QueryRow(ctx, `INSERT INTO quotation_extraction_provider_state (provider, model, usage_date, calls_used, updated_at) VALUES ($1,$2,$3,1,$4) ON CONFLICT (provider,model,usage_date) DO UPDATE SET calls_used = quotation_extraction_provider_state.calls_used + 1, updated_at = EXCLUDED.updated_at WHERE quotation_extraction_provider_state.calls_used < $5 RETURNING calls_used`, p.Provider, p.Model, dayStart, now, p.DailyProviderCallsMax).Scan(&providerCalls)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionBudgetExhausted
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ProviderSlot owns a dedicated session, retaining the advisory lock while a
// provider call runs across replicas.
type ProviderSlot struct {
	conn *pgxpool.Conn
	key  string
}

func (s *ProviderSlot) Release(ctx context.Context) {
	if s == nil || s.conn == nil {
		return
	}
	// Cancellation paths may pass an already-canceled execution context. The
	// session-scoped advisory lock must still be released before pooling it.
	releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _ = s.conn.Exec(releaseCtx, `SELECT pg_advisory_unlock(hashtext($1))`, s.key)
	s.conn.Release()
	s.conn = nil
}
func (r *MeetingExtractionRepository) TryAcquireProviderSlot(ctx context.Context, provider, model string) (*ProviderSlot, error) {
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return nil, ErrProviderSlotUnavailable
	}
	conn, err := r.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	key := "meeting-extraction-provider:" + provider + ":" + model
	var held bool
	err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtext($1))`, key).Scan(&held)
	if err != nil {
		conn.Release()
		return nil, err
	}
	if !held {
		conn.Release()
		return nil, ErrProviderSlotUnavailable
	}
	return &ProviderSlot{conn: conn, key: key}, nil
}

type MeetingExtractionRepository struct {
	db  *pgxpool.Pool
	now func() time.Time
}

// PersistValidatedPhase stores only a phase DTO that has already passed the
// phase validator. The fence predicate rejects stale workers before any result
// can overwrite a reclaimed run.
type PersistValidatedPhaseParams struct {
	RunID            uuid.UUID
	PhaseKey         string
	ExpectedFence    int64
	InputFingerprint string
	ResultJSON       []byte
	EvidenceRefs     []string
	Warnings         []string
	TransientRetries int
	RepairRetries    int
}

func (r *MeetingExtractionRepository) PersistValidatedPhase(ctx context.Context, p PersistValidatedPhaseParams) error {
	if p.RunID == uuid.Nil || strings.TrimSpace(p.PhaseKey) == "" || p.ExpectedFence < 0 || len(p.InputFingerprint) != 64 || len(p.ResultJSON) == 0 || len(p.ResultJSON) > 65536 || len(p.EvidenceRefs) > 128 || len(p.Warnings) > 128 || !json.Valid(p.ResultJSON) {
		return ErrExtractionFenceLost
	}
	if hasUnsafePersistedKey(p.ResultJSON) {
		return ErrExtractionFenceLost
	}
	evidenceRefs := p.EvidenceRefs
	if evidenceRefs == nil {
		evidenceRefs = []string{}
	}
	refs, err := json.Marshal(evidenceRefs)
	if err != nil {
		return err
	}
	warningsList := p.Warnings
	if warningsList == nil {
		warningsList = []string{}
	}
	warnings, err := json.Marshal(warningsList)
	if err != nil {
		return err
	}
	now := r.now().UTC()
	if p.TransientRetries < 0 || p.RepairRetries < 0 || p.TransientRetries > 32767 || p.RepairRetries > 32767 {
		return ErrExtractionFenceLost
	}
	result, err := r.db.Exec(ctx, `
		UPDATE quotation_extraction_phases phase
		SET status = 'succeeded', input_fingerprint = $1, result_json = $2::jsonb,
			evidence_refs = $3::jsonb, warnings = $4::jsonb, fence = $5,
			transient_retries = $6, repair_retries = $7,
			completed_at = $8, updated_at = $8
		FROM quotation_extraction_runs run
		WHERE phase.run_id = $9 AND phase.phase_key = $10 AND phase.status = 'running'
			AND run.id = phase.run_id AND run.status = 'running' AND run.fence = $5
	`, p.InputFingerprint, string(p.ResultJSON), string(refs), string(warnings), p.ExpectedFence, p.TransientRetries, p.RepairRetries, now, p.RunID, p.PhaseKey)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return nil
}

func (r *MeetingExtractionRepository) ShouldSkipSucceededPhase(ctx context.Context, runID uuid.UUID, phaseKey, fingerprint string) (bool, error) {
	if runID == uuid.Nil || strings.TrimSpace(phaseKey) == "" || len(fingerprint) != 64 {
		return false, nil
	}
	var matches bool
	err := r.db.QueryRow(ctx, `SELECT status = 'succeeded' AND input_fingerprint = $3 AND result_json IS NOT NULL FROM quotation_extraction_phases WHERE run_id = $1 AND phase_key = $2`, runID, phaseKey, fingerprint).Scan(&matches)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrExtractionNotFound
	}
	return matches, err
}

func hasUnsafePersistedKey(raw []byte) bool {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return true
	}
	var unsafe bool
	var inspect func(any)
	inspect = func(v any) {
		switch typed := v.(type) {
		case map[string]any:
			for key, child := range typed {
				normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
				if normalized == "prompt" || normalized == "rawresponse" || normalized == "providerresponse" || normalized == "quote" {
					unsafe = true
					return
				}
				inspect(child)
			}
		case []any:
			for _, child := range typed {
				inspect(child)
			}
		}
	}
	inspect(value)
	return unsafe
}

func NewMeetingExtractionRepository(db *pgxpool.Pool) *MeetingExtractionRepository {
	return NewMeetingExtractionRepositoryWithClock(db, time.Now)
}

func NewMeetingExtractionRepositoryWithClock(db *pgxpool.Pool, clock func() time.Time) *MeetingExtractionRepository {
	if clock == nil {
		clock = time.Now
	}
	return &MeetingExtractionRepository{db: db, now: clock}
}

type CreateExtractionRunParams struct {
	CompanyID       uuid.UUID
	QuotationID     uuid.UUID
	RevisionID      uuid.UUID
	NoteHash        string
	PipelineVersion string
}

type CreateExtractionRunResult struct {
	Run    *domain.ExtractionRun
	Reused bool
}

func (r *MeetingExtractionRepository) CreateOrReuseRun(ctx context.Context, params CreateExtractionRunParams) (*CreateExtractionRunResult, error) {
	if params.CompanyID == uuid.Nil || params.QuotationID == uuid.Nil || params.RevisionID == uuid.Nil ||
		len(params.NoteHash) != 64 || strings.TrimSpace(params.PipelineVersion) == "" {
		return nil, ErrExtractionNotFound
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var quotationID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM quotations WHERE id = $1 AND company_id = $2 AND deleted_at IS NULL FOR UPDATE`, params.QuotationID, params.CompanyID).Scan(&quotationID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExtractionNotFound
	}
	if err != nil {
		return nil, err
	}

	existing, err := loadActiveExtractionRun(ctx, tx, params.QuotationID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.NoteHash != params.NoteHash || existing.PipelineVersion != params.PipelineVersion {
			return nil, ErrExtractionActiveRun
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return &CreateExtractionRunResult{Run: existing, Reused: true}, nil
	}

	run := &domain.ExtractionRun{}
	now := r.now().UTC()
	err = tx.QueryRow(ctx, `
		INSERT INTO quotation_extraction_runs (company_id, quotation_id, revision_id, status, note_hash, pipeline_version, created_at, updated_at)
		SELECT $1, $2, $3, 'queued', $4, $5, $6, $6
		WHERE EXISTS (SELECT 1 FROM quotation_revisions WHERE id = $3 AND quotation_id = $2 AND company_id = $1)
		RETURNING id, company_id, quotation_id, revision_id, status, note_hash, pipeline_version, lease_owner, lease_expires_at, fence, created_at, updated_at
	`, params.CompanyID, params.QuotationID, params.RevisionID, params.NoteHash, params.PipelineVersion, now).Scan(runFields(run)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExtractionNotFound
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrExtractionActiveRun
		}
		return nil, err
	}
	for index, key := range domain.ExtractionPhaseOrder {
		_, err = tx.Exec(ctx, `INSERT INTO quotation_extraction_phases (run_id, phase_key, ordinal, status, created_at, updated_at) VALUES ($1, $2, $3, 'pending', $4, $4)`, run.ID, key, index+1, now)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &CreateExtractionRunResult{Run: run}, nil
}

func (r *MeetingExtractionRepository) ClaimNext(ctx context.Context, worker string, lease time.Duration) (*domain.ExtractionRun, error) {
	return r.ClaimNextAt(ctx, worker, lease, r.now())
}

func (r *MeetingExtractionRepository) ClaimNextAt(ctx context.Context, worker string, lease time.Duration, now time.Time) (*domain.ExtractionRun, error) {
	if strings.TrimSpace(worker) == "" || lease <= 0 {
		return nil, ErrNoExtractionRunAvailable
	}
	leaseExpires := now.Add(lease).UTC()
	run := &domain.ExtractionRun{}
	err := r.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id FROM quotation_extraction_runs
			WHERE (status = 'queued' AND (retry_after IS NULL OR retry_after <= $1))
			   OR (status = 'running' AND lease_expires_at < $1)
			   OR (status = 'waiting_rate_limit' AND retry_after <= $1)
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE quotation_extraction_runs run
		SET status = 'running', lease_owner = $2, lease_expires_at = $3,
			fence = run.fence + 1, started_at = COALESCE(run.started_at, $1), updated_at = $1
		FROM candidate
		WHERE run.id = candidate.id
		RETURNING run.id, run.company_id, run.quotation_id, run.revision_id, run.status, run.note_hash, run.pipeline_version, run.lease_owner, run.lease_expires_at, run.fence, run.created_at, run.updated_at
	`, now.UTC(), worker, leaseExpires).Scan(runFields(run)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoExtractionRunAvailable
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (r *MeetingExtractionRepository) Heartbeat(ctx context.Context, runID uuid.UUID, expectedFence int64, lease time.Duration) error {
	if lease <= 0 {
		return ErrExtractionFenceLost
	}
	now := r.now().UTC()
	result, err := r.db.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET lease_expires_at = $1, updated_at = $2
		WHERE id = $3 AND status = 'running' AND fence = $4
	`, now.Add(lease), now, runID, expectedFence)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return nil
}

func (r *MeetingExtractionRepository) TransitionRun(ctx context.Context, runID uuid.UUID, expectedFence int64, next domain.ExtractionRunStatus) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status domain.ExtractionRunStatus
	var fence int64
	err = tx.QueryRow(ctx, `SELECT status, fence FROM quotation_extraction_runs WHERE id = $1 FOR UPDATE`, runID).Scan(&status, &fence)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionNotFound
	}
	if err != nil {
		return err
	}
	if fence != expectedFence {
		return ErrExtractionFenceLost
	}
	if !status.CanTransitionTo(next) {
		return domain.ErrInvalidExtractionTransition
	}
	now := r.now().UTC()
	result, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_runs
		SET status = $1::varchar, lease_owner = CASE WHEN $1::varchar = 'running' THEN lease_owner ELSE NULL END,
			lease_expires_at = CASE WHEN $1::varchar = 'running' THEN lease_expires_at ELSE NULL END,
			completed_at = CASE WHEN $1::varchar IN ('needs_review','succeeded','cancelled') THEN $2 ELSE completed_at END,
			updated_at = $2
		WHERE id = $3 AND fence = $4
	`, next, now, runID, expectedFence)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return tx.Commit(ctx)
}

func (r *MeetingExtractionRepository) TransitionPhase(ctx context.Context, runID uuid.UUID, phaseKey string, expectedFence int64, next domain.ExtractionPhaseStatus) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status domain.ExtractionPhaseStatus
	err = tx.QueryRow(ctx, `
		SELECT phase.status
		FROM quotation_extraction_phases phase
		JOIN quotation_extraction_runs run ON run.id = phase.run_id
		WHERE phase.run_id = $1 AND phase.phase_key = $2 AND run.fence = $3 AND run.status = 'running'
		FOR UPDATE OF phase
	`, runID, phaseKey, expectedFence).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrExtractionFenceLost
	}
	if err != nil {
		return err
	}
	if !status.CanTransitionTo(next) {
		return domain.ErrInvalidExtractionTransition
	}
	now := r.now().UTC()
	result, err := tx.Exec(ctx, `
		UPDATE quotation_extraction_phases
		SET status = $1::varchar, fence = $2::bigint,
			started_at = CASE WHEN $1::varchar = 'running' THEN $3::timestamptz ELSE started_at END,
			completed_at = CASE WHEN $1::varchar IN ('succeeded','failed','skipped') THEN $3::timestamptz ELSE NULL END,
			updated_at = $3::timestamptz
		WHERE run_id = $4 AND phase_key = $5 AND fence <= $2::bigint
	`, next, expectedFence, now, runID, phaseKey)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrExtractionFenceLost
	}
	return tx.Commit(ctx)
}

func (r *MeetingExtractionRepository) ReapExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `
		SELECT id, fence FROM quotation_extraction_runs
		WHERE status = 'running' AND lease_expires_at < $1::timestamptz
		ORDER BY lease_expires_at
		FOR UPDATE SKIP LOCKED LIMIT $2
	`, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	type expiredRun struct {
		id    uuid.UUID
		fence int64
	}
	expired := make([]expiredRun, 0, limit)
	for rows.Next() {
		var item expiredRun
		if err := rows.Scan(&item.id, &item.fence); err != nil {
			rows.Close()
			return 0, err
		}
		expired = append(expired, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, item := range expired {
		_, err = tx.Exec(ctx, `
			UPDATE quotation_extraction_phases SET status = 'pending', started_at = NULL, completed_at = NULL, updated_at = $1::timestamptz
			WHERE run_id = $2 AND status = 'running' AND fence = $3 AND result_json IS NULL AND input_fingerprint IS NULL
		`, now.UTC(), item.id, item.fence)
		if err != nil {
			return 0, err
		}
		result, err := tx.Exec(ctx, `
			UPDATE quotation_extraction_runs
			SET status = 'queued', lease_owner = NULL, lease_expires_at = NULL, updated_at = $1::timestamptz
			WHERE id = $2 AND status = 'running' AND fence = $3 AND lease_expires_at < $1::timestamptz
		`, now.UTC(), item.id, item.fence)
		if err != nil {
			return 0, err
		}
		if result.RowsAffected() != 1 {
			return 0, ErrExtractionFenceLost
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(expired), nil
}

func (r *MeetingExtractionRepository) GetRun(ctx context.Context, runID uuid.UUID) (*domain.ExtractionRun, []domain.ExtractionPhase, error) {
	run := &domain.ExtractionRun{}
	err := r.db.QueryRow(ctx, `SELECT id, company_id, quotation_id, revision_id, status, note_hash, pipeline_version, lease_owner, lease_expires_at, fence, created_at, updated_at FROM quotation_extraction_runs WHERE id = $1`, runID).Scan(runFields(run)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrExtractionNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	rows, err := r.db.Query(ctx, `SELECT id, run_id, phase_key, ordinal, status, fence, created_at, updated_at FROM quotation_extraction_phases WHERE run_id = $1 ORDER BY ordinal`, runID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	phases := make([]domain.ExtractionPhase, 0, len(domain.ExtractionPhaseOrder))
	for rows.Next() {
		var phase domain.ExtractionPhase
		if err := rows.Scan(&phase.ID, &phase.RunID, &phase.PhaseKey, &phase.Ordinal, &phase.Status, &phase.Fence, &phase.CreatedAt, &phase.UpdatedAt); err != nil {
			return nil, nil, err
		}
		phases = append(phases, phase)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return run, phases, nil
}

func loadActiveExtractionRun(ctx context.Context, q pgx.Tx, quotationID uuid.UUID) (*domain.ExtractionRun, error) {
	run := &domain.ExtractionRun{}
	err := q.QueryRow(ctx, `
		SELECT id, company_id, quotation_id, revision_id, status, note_hash, pipeline_version, lease_owner, lease_expires_at, fence, created_at, updated_at
		FROM quotation_extraction_runs
		WHERE quotation_id = $1 AND status IN ('queued','running','waiting_rate_limit','input_required')
		FOR UPDATE
	`, quotationID).Scan(runFields(run)...)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

func runFields(run *domain.ExtractionRun) []any {
	return []any{&run.ID, &run.CompanyID, &run.QuotationID, &run.RevisionID, &run.Status, &run.NoteHash, &run.PipelineVersion, &run.LeaseOwner, &run.LeaseExpiresAt, &run.Fence, &run.CreatedAt, &run.UpdatedAt}
}
