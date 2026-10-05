//go:build integration

package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/modules/quotation/testutil"
)

var _ service.MeetingExtractionOrchestratorStore = (*service.PostgreSQLMeetingExtractionOrchestratorStore)(nil)

func TestPostgreSQLMeetingExtractionOrchestratorStorePersistsOnlySafePhaseData(t *testing.T) {
	pool := testutil.Pool(t)
	fx := testutil.NewExtractionFixture(t, pool)
	params, repo := fx.Params, repository.NewMeetingExtractionRepository(pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	claimed, err := repo.ClaimNext(ctx, "adapter-test", time.Minute)
	require.NoError(t, err)

	store, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{DailyProviderCallsMax: 4, DailyQuotationCallsMax: 4})
	require.NoError(t, err)
	run, err := store.LoadRun(ctx, claimed.ID, claimed.Fence)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunRunning, run.Status)
	phase, err := store.LoadPhase(ctx, claimed.ID, service.PhaseKey(domain.ExtractionPhasePlanMetadata), claimed.Fence)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionPhasePending, phase.Status)

	require.NoError(t, store.StartPhase(ctx, claimed.ID, service.PhaseKey(domain.ExtractionPhasePlanMetadata), claimed.Fence))
	require.NoError(t, store.ReserveCall(ctx, service.MeetingExtractionCallReservation{RunID: claimed.ID, Fence: claimed.Fence, PhaseKey: service.PhaseKey(domain.ExtractionPhasePlanMetadata), Provider: "gemini", Model: "test", CallsMax: 8}))
	require.NoError(t, store.CommitPhase(ctx, service.MeetingExtractionPhaseCommit{RunID: claimed.ID, PhaseKey: service.PhaseKey(domain.ExtractionPhasePlanMetadata), Fence: claimed.Fence, InputFingerprint: strings.Repeat("a", 64), ResultJSON: []byte(`{"metadata":{"client_company":{"value":"Example"}}}`), EvidenceRefs: []string{"L0001"}, Warnings: []string{"NONE"}}))

	var persisted string
	require.NoError(t, pool.QueryRow(ctx, `SELECT result_json::text FROM quotation_extraction_phases WHERE run_id=$1 AND phase_key=$2`, created.Run.ID, domain.ExtractionPhasePlanMetadata).Scan(&persisted))
	require.NotContains(t, persisted, "CANARY_MEETING_NOTES")
	require.NotContains(t, persisted, "CANARY_RAW_PROMPT")
	require.NotContains(t, persisted, "CANARY_RAW_PROVIDER_RESPONSE")
}

func TestPostgreSQLMeetingExtractionOrchestratorStoreFencesMutationsAndRateLimit(t *testing.T) {
	pool := testutil.Pool(t)
	fx := testutil.NewExtractionFixture(t, pool)
	params, repo := fx.Params, repository.NewMeetingExtractionRepository(pool)
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, params)
	require.NoError(t, err)
	claimed, err := repo.ClaimNext(ctx, "adapter-fence-test", time.Minute)
	require.NoError(t, err)
	store, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{DailyProviderCallsMax: 4, DailyQuotationCallsMax: 4})
	require.NoError(t, err)

	require.ErrorIs(t, store.StartPhase(ctx, claimed.ID, service.PhaseKey(domain.ExtractionPhasePlanMetadata), claimed.Fence-1), service.ErrMeetingExtractionStaleFence)
	retryAt := time.Now().UTC().Add(time.Minute)
	require.NoError(t, store.StartPhase(ctx, claimed.ID, service.PhaseKey(domain.ExtractionPhasePlanMetadata), claimed.Fence))
	require.NoError(t, store.WaitRateLimit(ctx, service.MeetingExtractionRateLimitWait{RunID: claimed.ID, PhaseKey: service.PhaseKey(domain.ExtractionPhasePlanMetadata), Fence: claimed.Fence, RetryAfter: retryAt}))
	run, err := store.LoadRun(ctx, claimed.ID, claimed.Fence)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunWaitingRateLimit, run.Status)
	phase, err := store.LoadPhase(ctx, claimed.ID, service.PhaseKey(domain.ExtractionPhasePlanMetadata), claimed.Fence)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionPhasePending, phase.Status)
	require.NoError(t, store.ResumeRateLimited(ctx, claimed.ID, claimed.Fence))
	run, err = store.LoadRun(ctx, claimed.ID, claimed.Fence)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunQueued, run.Status)
	_, err = repo.ClaimNext(ctx, "adapter-fence-reclaim", time.Minute)
	require.NoError(t, err)
	require.ErrorIs(t, store.MarkSucceeded(ctx, created.Run.ID, claimed.Fence), service.ErrMeetingExtractionStaleFence)
}

func TestNewPostgreSQLMeetingExtractionOrchestratorStoreRejectsInvalidOptions(t *testing.T) {
	pool := testutil.Pool(t)
	_, repo := testutil.NewExtractionFixture(t, pool), repository.NewMeetingExtractionRepository(pool)
	_, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{})
	require.Error(t, err)
}

func TestPostgreSQLMeetingExtractionOrchestratorStoreRequeuesProviderSlotWithJitter(t *testing.T) {
	pool := testutil.Pool(t)
	fx := testutil.NewExtractionFixture(t, pool)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	repo := repository.NewMeetingExtractionRepositoryWithClock(pool, func() time.Time { return now })
	ctx := context.Background()
	created, err := repo.CreateOrReuseRun(ctx, fx.Params)
	require.NoError(t, err)
	claimed, err := repo.ClaimNextAt(ctx, "slot-jitter-worker", time.Minute, now)
	require.NoError(t, err)

	holder, err := repo.TryAcquireProviderSlot(ctx, "gemini", "test")
	require.NoError(t, err)
	t.Cleanup(func() { holder.Release(context.Background()) })
	store, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{
		DailyProviderCallsMax: 4, DailyQuotationCallsMax: 4,
		Now: func() time.Time { return now }, RandomIntn: func(int) int { return 4 },
	})
	require.NoError(t, err)
	err = store.ReserveCall(ctx, service.MeetingExtractionCallReservation{RunID: claimed.ID, Fence: claimed.Fence, PhaseKey: service.PhaseKey(domain.ExtractionPhasePlanMetadata), Provider: "gemini", Model: "test", CallsMax: 8})
	require.ErrorIs(t, err, service.ErrMeetingExtractionStaleFence)

	var status domain.ExtractionRunStatus
	var retryAfter time.Time
	var leaseOwner *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT status, retry_after, lease_owner FROM quotation_extraction_runs WHERE id=$1`, created.Run.ID).Scan(&status, &retryAfter, &leaseOwner))
	assert.Equal(t, domain.ExtractionRunQueued, status)
	assert.True(t, retryAfter.UTC().Equal(now.Add(5*time.Second)), "retry_after must preserve the jitter instant")
	assert.Nil(t, leaseOwner)
	_, err = repo.ClaimNextAt(ctx, "before-jitter", time.Minute, now.Add(4*time.Second))
	assert.ErrorIs(t, err, repository.ErrNoExtractionRunAvailable)
	reclaimed, err := repo.ClaimNextAt(ctx, "after-jitter", time.Minute, now.Add(5*time.Second))
	require.NoError(t, err)
	assert.Equal(t, claimed.Fence+1, reclaimed.Fence)
}
