//go:build integration

package repository_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/testutil"
)

func extractionMigrationSQL(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	contents, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../database/migrations/quotation", name))
	require.NoError(t, err)
	return string(contents)
}

func ensureExtractionSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	var exists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.quotation_extraction_runs') IS NOT NULL`).Scan(&exists))
	if exists {
		return
	}
	_, err := pool.Exec(ctx, extractionMigrationSQL(t, "20260929010000_create_meeting_extraction_pipeline.up.sql"))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), extractionMigrationSQL(t, "20260929010000_create_meeting_extraction_pipeline.down.sql"))
	})
}

func extractionFixture(t *testing.T, pool *pgxpool.Pool) (repository.CreateExtractionRunParams, *repository.MeetingExtractionRepository) {
	t.Helper()
	fx := testutil.NewExtractionFixture(t, pool)
	clock := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	return fx.Params, repository.NewMeetingExtractionRepositoryWithClock(pool, func() time.Time { return clock })
}

func TestMeetingExtractionMigrationUpDownAndPrivacyColumns(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, extractionMigrationSQL(t, "20260929010000_create_meeting_extraction_pipeline.down.sql"))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, extractionMigrationSQL(t, "20260929010000_create_meeting_extraction_pipeline.up.sql"))
	require.NoError(t, err)

	for _, table := range []string{"quotation_extraction_runs", "quotation_extraction_phases", "quotation_extraction_provider_state"} {
		var exists bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists))
		assert.True(t, exists, table)
	}
	rows, err := tx.Query(ctx, `
		SELECT attname FROM pg_attribute
		WHERE attrelid IN ('quotation_extraction_runs'::regclass, 'quotation_extraction_phases'::regclass, 'quotation_extraction_provider_state'::regclass)
		  AND attnum > 0 AND NOT attisdropped
	`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		assert.NotEqual(t, "meeting_notes", strings.ToLower(column))
		assert.NotEqual(t, "raw_prompt", strings.ToLower(column))
		assert.NotEqual(t, "raw_provider_response", strings.ToLower(column))
	}
	require.NoError(t, rows.Err())
	_, err = tx.Exec(ctx, extractionMigrationSQL(t, "20260929010000_create_meeting_extraction_pipeline.down.sql"))
	require.NoError(t, err)
	var dropped bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT to_regclass('public.quotation_extraction_runs') IS NULL`).Scan(&dropped))
	assert.True(t, dropped)
	_, err = tx.Exec(ctx, extractionMigrationSQL(t, "20260929010000_create_meeting_extraction_pipeline.up.sql"))
	require.NoError(t, err)
	var restored bool
	require.NoError(t, tx.QueryRow(ctx, `SELECT to_regclass('public.quotation_extraction_runs') IS NOT NULL`).Scan(&restored))
	assert.True(t, restored)
}

func TestMeetingExtractionConcurrentClaimHasOneWinner(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)

	const workers = 20
	start := make(chan struct{})
	claims := make(chan *domain.ExtractionRun, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			claim, claimErr := repo.ClaimNext(ctx, "test-worker", 5*time.Minute)
			if claimErr != nil && !errors.Is(claimErr, repository.ErrNoExtractionRunAvailable) {
				errs <- claimErr
				return
			}
			if claim != nil {
				claims <- claim
			}
		}()
	}
	close(start)
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var won []*domain.ExtractionRun
	for claim := range claims {
		won = append(won, claim)
	}
	require.Len(t, won, 1)
	assert.Equal(t, created.Run.ID, won[0].ID)
	assert.Equal(t, int64(1), won[0].Fence)
}

func TestMeetingExtractionRejectsOldFenceAfterReclaim(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	first, err := repo.ClaimNext(ctx, "worker-a", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first)
	reclaimed, err := repo.ClaimNextAt(ctx, "worker-b", time.Minute, first.LeaseExpiresAt.Add(time.Second))
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	require.Equal(t, int64(2), reclaimed.Fence)
	err = repo.TransitionRun(ctx, created.Run.ID, first.Fence, domain.ExtractionRunSucceeded)
	assert.ErrorIs(t, err, repository.ErrExtractionFenceLost)
}

func TestMeetingExtractionReaperRestoresOnlyRunningPhase(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	claim, err := repo.ClaimNext(ctx, "worker-a", time.Minute)
	require.NoError(t, err)
	require.NoError(t, repo.TransitionPhase(ctx, claim.ID, "plan_metadata", claim.Fence, domain.ExtractionPhaseRunning))
	require.NoError(t, repo.TransitionPhase(ctx, claim.ID, "scope", claim.Fence, domain.ExtractionPhaseSkipped))
	reaped, err := repo.ReapExpired(ctx, claim.LeaseExpiresAt.Add(time.Second), 10)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	run, phases, err := repo.GetRun(ctx, created.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ExtractionRunQueued, run.Status)
	assert.Equal(t, domain.ExtractionPhasePending, phases[0].Status)
	assert.Equal(t, domain.ExtractionPhaseSkipped, phases[1].Status)
}

func TestMeetingExtractionConcurrentCreateReusesSingleActiveRun(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	const workers = 2
	start := make(chan struct{})
	results := make(chan *repository.CreateExtractionRunResult, workers)
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, err := repo.CreateOrReuseRun(ctx, params)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var ids []uuid.UUID
	for result := range results {
		ids = append(ids, result.Run.ID)
	}
	require.Len(t, ids, workers)
	assert.Equal(t, ids[0], ids[1])
	different := params
	different.NoteHash = strings.Repeat("b", 64)
	_, err := repo.CreateOrReuseRun(ctx, different)
	assert.ErrorIs(t, err, repository.ErrExtractionActiveRun)
}

func TestMeetingExtractionRejectsInvalidTransitionAndPersistsNoCanary(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	err = repo.TransitionRun(ctx, created.Run.ID, 0, domain.ExtractionRunSucceeded)
	assert.ErrorIs(t, err, domain.ErrInvalidExtractionTransition)
	claim, err := repo.ClaimNext(ctx, "worker-a", time.Minute)
	require.NoError(t, err)
	err = repo.TransitionPhase(ctx, claim.ID, "plan_metadata", claim.Fence, domain.ExtractionPhaseSucceeded)
	assert.ErrorIs(t, err, domain.ErrInvalidExtractionTransition)
	var row string
	require.NoError(t, pool.QueryRow(ctx, `SELECT row_to_json(r)::text FROM quotation_extraction_runs r WHERE id = $1`, created.Run.ID).Scan(&row))
	var phaseRow string
	require.NoError(t, pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM quotation_extraction_phases p WHERE p.run_id = $1 ORDER BY ordinal LIMIT 1`, created.Run.ID).Scan(&phaseRow))
	for _, canary := range []string{"CANARY_MEETING_NOTES", "CANARY_RAW_PROMPT", "CANARY_RAW_PROVIDER_RESPONSE"} {
		assert.NotContains(t, row, canary)
		assert.NotContains(t, phaseRow, canary)
	}
}

func TestMeetingExtractionPersistValidatedPhaseIsFencedAndSkipsIdenticalSuccess(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	_, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	run, err := repo.ClaimNext(ctx, "p1-writer", time.Minute)
	require.NoError(t, err)
	require.NoError(t, repo.TransitionPhase(ctx, run.ID, domain.ExtractionPhasePlanMetadata, run.Fence, domain.ExtractionPhaseRunning))

	fingerprint := strings.Repeat("b", 64)
	err = repo.PersistValidatedPhase(ctx, repository.PersistValidatedPhaseParams{
		RunID: run.ID, PhaseKey: domain.ExtractionPhasePlanMetadata, ExpectedFence: run.Fence,
		InputFingerprint: fingerprint, ResultJSON: []byte(`{"metadata":{"client_company":{"value":"Example"}}}`),
		EvidenceRefs: []string{"L0001"}, Warnings: []string{"SCHEME_REVIEW_REQUIRED"},
	})
	require.NoError(t, err)
	skip, err := repo.ShouldSkipSucceededPhase(ctx, run.ID, domain.ExtractionPhasePlanMetadata, fingerprint)
	require.NoError(t, err)
	require.True(t, skip)

	err = repo.PersistValidatedPhase(ctx, repository.PersistValidatedPhaseParams{RunID: run.ID, PhaseKey: domain.ExtractionPhasePlanMetadata, ExpectedFence: run.Fence - 1, InputFingerprint: fingerprint, ResultJSON: []byte(`{}`)})
	require.ErrorIs(t, err, repository.ErrExtractionFenceLost)

	var persisted string
	err = pool.QueryRow(ctx, `SELECT result_json::text FROM quotation_extraction_phases WHERE run_id=$1 AND phase_key=$2`, run.ID, domain.ExtractionPhasePlanMetadata).Scan(&persisted)
	require.NoError(t, err)
	assert.NotContains(t, persisted, "raw_response")
	assert.NotContains(t, persisted, "prompt")
}

func TestMeetingExtractionPersistValidatedPhaseNormalizesNilEvidenceAndWarnings(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	_, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	run, err := repo.ClaimNext(ctx, "nil-array-writer", time.Minute)
	require.NoError(t, err)
	require.NoError(t, repo.TransitionPhase(ctx, run.ID, domain.ExtractionPhasePlanMetadata, run.Fence, domain.ExtractionPhaseRunning))

	err = repo.PersistValidatedPhase(ctx, repository.PersistValidatedPhaseParams{
		RunID: run.ID, PhaseKey: domain.ExtractionPhasePlanMetadata, ExpectedFence: run.Fence,
		InputFingerprint: strings.Repeat("c", 64), ResultJSON: []byte(`{"metadata":{}}`),
	})
	require.NoError(t, err)

	var evidenceType, warningsType string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT jsonb_typeof(evidence_refs), jsonb_typeof(warnings)
		FROM quotation_extraction_phases WHERE run_id = $1 AND phase_key = $2
	`, run.ID, domain.ExtractionPhasePlanMetadata).Scan(&evidenceType, &warningsType))
	assert.Equal(t, "array", evidenceType)
	assert.Equal(t, "array", warningsType)
}

func TestCallReservationRespectsRunAndDailyLimits(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	reserve := repository.ExtractionCallReservationParams{RunID: created.Run.ID, QuotationID: params.QuotationID, Provider: "gemini", Model: "test", DailyProviderCallsMax: 2, DailyQuotationCallsMax: 2}
	require.NoError(t, repo.ReserveExtractionCall(ctx, reserve))
	require.NoError(t, repo.ReserveExtractionCall(ctx, reserve))
	assert.ErrorIs(t, repo.ReserveExtractionCall(ctx, reserve), repository.ErrExtractionBudgetExhausted)
	var calls int
	require.NoError(t, pool.QueryRow(ctx, `SELECT calls_used FROM quotation_extraction_runs WHERE id=$1`, created.Run.ID).Scan(&calls))
	assert.Equal(t, 2, calls)
}

func TestCallReservationConcurrentNeverExceedsCap(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	reserve := repository.ExtractionCallReservationParams{RunID: created.Run.ID, QuotationID: params.QuotationID, Provider: "gemini", Model: "concurrent", DailyProviderCallsMax: 3, DailyQuotationCallsMax: 3}
	start := make(chan struct{})
	var wg sync.WaitGroup
	successes := make(chan struct{}, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if repo.ReserveExtractionCall(ctx, reserve) == nil {
				successes <- struct{}{}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(successes)
	count := 0
	for range successes {
		count++
	}
	assert.Equal(t, 3, count)
	var calls int
	require.NoError(t, pool.QueryRow(ctx, `SELECT calls_used FROM quotation_extraction_runs WHERE id=$1`, created.Run.ID).Scan(&calls))
	assert.Equal(t, 3, calls)
}

func TestProviderCircuitBreakerTransitionsPerProviderModel(t *testing.T) {
	pool := getTestPool(t)
	params, repo := extractionFixture(t, pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	claimed, err := repo.ClaimNext(ctx, "circuit-worker", time.Minute)
	require.NoError(t, err)
	circuitModel := "circuit-model-" + uuid.NewString()
	reserve := repository.ExtractionCallReservationParams{RunID: created.Run.ID, QuotationID: params.QuotationID, Provider: "gemini", Model: circuitModel, DailyProviderCallsMax: 10, DailyQuotationCallsMax: 10}

	require.NoError(t, repo.ReserveExtractionCallFenced(ctx, reserve, claimed.Fence))
	require.NoError(t, repo.RecordExtractionProviderOutcome(ctx, repository.ExtractionProviderOutcomeParams{RunID: claimed.ID, Fence: claimed.Fence, Provider: "gemini", Model: circuitModel, Retryable: true, FailureThreshold: 2, Cooldown: time.Minute}))
	require.NoError(t, repo.ReserveExtractionCallFenced(ctx, reserve, claimed.Fence))
	require.NoError(t, repo.RecordExtractionProviderOutcome(ctx, repository.ExtractionProviderOutcomeParams{RunID: claimed.ID, Fence: claimed.Fence, Provider: "gemini", Model: circuitModel, Retryable: true, FailureThreshold: 2, Cooldown: time.Minute}))
	assert.ErrorIs(t, repo.ReserveExtractionCallFenced(ctx, reserve, claimed.Fence), repository.ErrExtractionCircuitOpen)
	otherModel := reserve
	otherModel.Model = "other-model"
	require.NoError(t, repo.ReserveExtractionCallFenced(ctx, otherModel, claimed.Fence))

	clock := time.Date(2026, 9, 29, 10, 1, 1, 0, time.UTC)
	repo = repository.NewMeetingExtractionRepositoryWithClock(pool, func() time.Time { return clock })
	require.NoError(t, repo.ReserveExtractionCallFenced(ctx, reserve, claimed.Fence)) // half-open probe
	require.NoError(t, repo.RecordExtractionProviderOutcome(ctx, repository.ExtractionProviderOutcomeParams{RunID: claimed.ID, Fence: claimed.Fence, Provider: "gemini", Model: circuitModel, FailureThreshold: 2, Cooldown: time.Minute}))
	require.NoError(t, repo.ReserveExtractionCallFenced(ctx, reserve, claimed.Fence)) // closed
}

func TestCallReservationAcrossRepositoryReplicasEnforcesGlobalAndQuotationDailyCaps(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	firstParams, firstRepo := extractionFixture(t, pool)
	secondParams, secondRepo := extractionFixture(t, pool)
	_, err := firstRepo.CreateOrReuseRun(ctx, firstParams)
	require.NoError(t, err)
	_, err = secondRepo.CreateOrReuseRun(ctx, secondParams)
	require.NoError(t, err)
	firstClaim, err := firstRepo.ClaimNext(ctx, "replica-a", time.Minute)
	require.NoError(t, err)
	secondClaim, err := secondRepo.ClaimNext(ctx, "replica-b", time.Minute)
	require.NoError(t, err)
	providerModel := "replica-cap-" + uuid.NewString()

	start := make(chan struct{})
	var wg sync.WaitGroup
	successes := make(chan struct{}, 12)
	for i := 0; i < 12; i++ {
		repo, run, quotationID := firstRepo, firstClaim, firstParams.QuotationID
		if i%2 == 1 {
			repo, run, quotationID = secondRepo, secondClaim, secondParams.QuotationID
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if repo.ReserveExtractionCallFenced(ctx, repository.ExtractionCallReservationParams{RunID: run.ID, QuotationID: quotationID, Provider: "gemini", Model: providerModel, DailyProviderCallsMax: 3, DailyQuotationCallsMax: 8}, run.Fence) == nil {
				successes <- struct{}{}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(successes)
	count := 0
	for range successes {
		count++
	}
	assert.Equal(t, 3, count, "global provider/model cap must cover both repository replicas")

	// A later run of the same quotation cannot bypass its daily cap.
	quotationParams, quotationRepo := extractionFixture(t, pool)
	quotationRun, err := quotationRepo.CreateOrReuseRun(ctx, quotationParams)
	require.NoError(t, err)
	quotationClaim, err := quotationRepo.ClaimNext(ctx, "quotation-cap-first", time.Minute)
	require.NoError(t, err)
	require.NoError(t, quotationRepo.ReserveExtractionCallFenced(ctx, repository.ExtractionCallReservationParams{RunID: quotationRun.Run.ID, QuotationID: quotationParams.QuotationID, Provider: "gemini", Model: "quotation-cap", DailyProviderCallsMax: 10, DailyQuotationCallsMax: 1}, quotationClaim.Fence))
	_, err = pool.Exec(ctx, `UPDATE quotation_extraction_runs SET status = 'failed' WHERE id = $1`, quotationRun.Run.ID)
	require.NoError(t, err)
	quotationParams.NoteHash = strings.Repeat("d", 64)
	later, err := quotationRepo.CreateOrReuseRun(ctx, quotationParams)
	require.NoError(t, err)
	laterClaim, err := quotationRepo.ClaimNext(ctx, "quotation-cap-later", time.Minute)
	require.NoError(t, err)
	assert.ErrorIs(t, quotationRepo.ReserveExtractionCallFenced(ctx, repository.ExtractionCallReservationParams{RunID: later.Run.ID, QuotationID: quotationParams.QuotationID, Provider: "gemini", Model: "quotation-cap", DailyProviderCallsMax: 10, DailyQuotationCallsMax: 1}, laterClaim.Fence), repository.ErrExtractionBudgetExhausted)
}

func TestProviderSlotHasExactlyOneHolder(t *testing.T) {
	pool := getTestPool(t)
	_, repo := extractionFixture(t, pool)
	ctx := context.Background()
	first, err := repo.TryAcquireProviderSlot(ctx, "gemini", "test")
	require.NoError(t, err)
	defer first.Release(ctx)
	second, err := repo.TryAcquireProviderSlot(ctx, "gemini", "test")
	assert.Nil(t, second)
	assert.ErrorIs(t, err, repository.ErrProviderSlotUnavailable)
}

func TestClaimNextQueuedHonorsRetryAfterAndCreatedOrder(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx, `UPDATE quotation_extraction_runs SET status='cancelled', lease_owner=NULL, lease_expires_at=NULL WHERE status IN ('queued','running','waiting_rate_limit','input_required')`)
	require.NoError(t, err)

	oldParams, repo := extractionFixture(t, pool)
	newParams, _ := extractionFixture(t, pool)
	futureParams, _ := extractionFixture(t, pool)
	oldRun, err := repo.CreateOrReuseRun(ctx, oldParams)
	require.NoError(t, err)
	newRun, err := repo.CreateOrReuseRun(ctx, newParams)
	require.NoError(t, err)
	futureRun, err := repo.CreateOrReuseRun(ctx, futureParams)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `UPDATE quotation_extraction_runs SET created_at=$1 WHERE id=$2`, base.Add(-3*time.Second), oldRun.Run.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE quotation_extraction_runs SET created_at=$1 WHERE id=$2`, base.Add(-2*time.Second), newRun.Run.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE quotation_extraction_runs SET created_at=$1, retry_after=$2 WHERE id=$3`, base.Add(-4*time.Second), base.Add(5*time.Second), futureRun.Run.ID)
	require.NoError(t, err)

	first, err := repo.ClaimNextAt(ctx, "created-order", time.Minute, base)
	require.NoError(t, err)
	assert.Equal(t, oldRun.Run.ID, first.ID)
	second, err := repo.ClaimNextAt(ctx, "created-order", time.Minute, base)
	require.NoError(t, err)
	assert.Equal(t, newRun.Run.ID, second.ID)
	_, err = repo.ClaimNextAt(ctx, "created-order", time.Minute, base.Add(4*time.Second))
	assert.ErrorIs(t, err, repository.ErrNoExtractionRunAvailable)
	third, err := repo.ClaimNextAt(ctx, "created-order", time.Minute, base.Add(5*time.Second))
	require.NoError(t, err)
	assert.Equal(t, futureRun.Run.ID, third.ID)
}
