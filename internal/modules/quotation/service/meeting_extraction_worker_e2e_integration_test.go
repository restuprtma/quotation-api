//go:build integration

package service_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/modules/quotation/testutil"
	quotationWorker "venturo-skeleton-go/internal/modules/quotation/worker"
)

type reclaimProvider struct {
	base    *e2eProvider
	block   atomic.Bool
	blocked chan struct{}
	once    sync.Once
}

func (p *reclaimProvider) ExtractionCapabilities() service.ProviderCapabilities {
	return p.base.ExtractionCapabilities()
}

func (p *reclaimProvider) GenerateExtraction(ctx context.Context, request service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error) {
	if request.Prompt == "staffing" && p.block.CompareAndSwap(true, false) {
		p.base.record(request.Prompt)
		p.once.Do(func() { close(p.blocked) })
		<-ctx.Done()
		return service.ExtractionGenerateResult{}, ctx.Err()
	}
	return p.base.GenerateExtraction(ctx, request)
}

type reclaimExecutor struct {
	provider *reclaimProvider
	store    service.MeetingExtractionOrchestratorStore
	phases   service.MeetingExtractionPhaseSet
	p5Mu     sync.Mutex
	p5Calls  int
	errMu    sync.Mutex
	errs     []error
}

func (e *reclaimExecutor) Execute(ctx context.Context, run *domain.ExtractionRun) error {
	orchestrator, err := service.NewMeetingExtractionOrchestrator(
		e.provider,
		e.store,
		e.phases,
		func(context.Context, service.MeetingExtractionP5Input) error {
			e.p5Mu.Lock()
			e.p5Calls++
			e.p5Mu.Unlock()
			return nil
		},
		service.MeetingExtractionOrchestratorConfig{},
	)
	if err != nil {
		return err
	}
	// Keep the run identity in diagnostics while the fence-reclaim test is being stabilized.
	_ = run
	doc, err := service.NewMeetingNoteDocument("worker reclaim integration note", service.MeetingNoteDocumentConfig{})
	if err != nil {
		return err
	}
	_, err = orchestrator.Run(ctx, service.MeetingExtractionOrchestratorInput{
		RunID: run.ID, Fence: run.Fence, Document: doc, ExpectedNoteHash: run.NoteHash,
		PipelineVersion: "meeting-extraction-v1", PromptVersion: "p1", SchemaVersion: "s1",
		CatalogSnapshotHash: "catalog", RegistryEntryVersion: "registry", ManualScheme: domain.CommercialSchemeDedicatedSquad,
		Provider: "fake-reclaim", Model: "fake-reclaim-" + run.ID.String(), CompanyID: run.CompanyID,
		QuotationID: run.QuotationID, RevisionID: run.RevisionID,
	})
	if err != nil {
		e.errMu.Lock()
		e.errs = append(e.errs, fmt.Errorf("run=%s fence=%d: %w", run.ID, run.Fence, err))
		e.errMu.Unlock()
	}
	return err
}

func (e *reclaimExecutor) p5Count() int {
	e.p5Mu.Lock()
	defer e.p5Mu.Unlock()
	return e.p5Calls
}

func (e *reclaimExecutor) errors() []error {
	e.errMu.Lock()
	defer e.errMu.Unlock()
	return append([]error(nil), e.errs...)
}

func TestMeetingExtractionWorkerReclaimsCancelledPhaseWithNewFence(t *testing.T) {
	pool := testutil.Pool(t)
	_, err := pool.Exec(context.Background(), `DELETE FROM quotation_extraction_runs`)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `DELETE FROM quotation_extraction_provider_state`)
	require.NoError(t, err)
	fixture := testutil.NewExtractionFixture(t, pool)
	repo := repository.NewMeetingExtractionRepository(pool)
	doc, err := service.NewMeetingNoteDocument("worker reclaim integration note", service.MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	fixture.Params.NoteHash = doc.Hash()
	created, err := repo.CreateOrReuseRun(context.Background(), fixture.Params)
	require.NoError(t, err)

	base := &e2eProvider{responses: map[string][]e2eResponse{}, counts: map[string]int{}}
	for _, key := range []string{"plan_metadata", "scope", "staffing", "service_details"} {
		base.responses[key] = []e2eResponse{{text: `{}`}, {text: `{}`}}
	}
	provider := &reclaimProvider{base: base, blocked: make(chan struct{})}
	provider.block.Store(true)
	store, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{
		DailyProviderCallsMax: 20, DailyQuotationCallsMax: 20,
	})
	require.NoError(t, err)
	phases := service.MeetingExtractionPhaseSet{
		Plan:     e2ePhase{key: service.PhaseKeyPlanMetadata, result: []byte(`{"scheme":"dedicated_squad","confidence":0.95}`)},
		Scope:    e2ePhase{key: service.PhaseKey("scope"), result: []byte(`{"scope":[]}`)},
		Staffing: e2ePhase{key: service.PhaseKey("staffing"), result: []byte(`{"duration_months":null,"team":[]}`)},
		Service: func(value string) service.Phase {
			return e2ePhase{key: service.PhaseKey("service_details"), result: []byte(`{"scheme":"` + value + `","facts":[]}`)}
		},
	}
	executor := &reclaimExecutor{provider: provider, store: store, phases: phases}

	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstWorker := quotationWorker.New(repo, executor, quotationWorker.Options{
		WorkerID: "reclaim-worker-one", Lease: 100 * time.Millisecond,
		HeartbeatInterval: time.Hour, PollInterval: 5 * time.Millisecond,
	})
	require.NoError(t, firstWorker.Start(firstCtx))
	select {
	case <-provider.blocked:
	case <-time.After(5 * time.Second):
		t.Fatal("worker one did not reach the blocked staffing phase")
	}
	firstRun, phasesBefore, err := repo.GetRun(context.Background(), created.Run.ID)
	require.NoError(t, err)
	oldFence := firstRun.Fence
	require.Equal(t, domain.ExtractionPhaseRunning, phasesBefore[2].Status)
	firstCancel()
	firstWorker.Wait()

	ctx := context.Background()
	reaped, err := repo.ReapExpired(ctx, time.Now().UTC().Add(time.Second), 10)
	require.NoError(t, err)
	require.Equal(t, 1, reaped)
	_, _, err = repo.GetRun(ctx, created.Run.ID)
	require.NoError(t, err)

	secondCtx, secondCancel := context.WithCancel(ctx)
	secondWorker := quotationWorker.New(repo, executor, quotationWorker.Options{
		WorkerID: "reclaim-worker-two", Lease: time.Minute,
		HeartbeatInterval: time.Hour, PollInterval: 5 * time.Millisecond,
	})
	require.NoError(t, secondWorker.Start(secondCtx))
	deadline := time.Now().Add(5 * time.Second)
	var finalRun *domain.ExtractionRun
	var finalPhases []domain.ExtractionPhase
	for time.Now().Before(deadline) {
		finalRun, finalPhases, err = repo.GetRun(ctx, created.Run.ID)
		require.NoError(t, err)
		if finalRun.Status == domain.ExtractionRunSucceeded {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	secondCancel()
	secondWorker.Wait()
	require.NotNil(t, finalRun)
	require.Equalf(t, domain.ExtractionRunSucceeded, finalRun.Status, "worker_errors=%v", executor.errors())
	require.Equal(t, oldFence+1, finalRun.Fence)
	require.Equal(t, domain.ExtractionPhaseSucceeded, finalPhases[0].Status)
	require.Equal(t, domain.ExtractionPhaseSucceeded, finalPhases[1].Status)
	require.Equal(t, domain.ExtractionPhaseSucceeded, finalPhases[2].Status)
	require.Equal(t, domain.ExtractionPhaseSucceeded, finalPhases[3].Status)
	require.Equal(t, domain.ExtractionPhaseSucceeded, finalPhases[4].Status)

	var oldFenceRows int64
	result, err := pool.Exec(ctx, `
		UPDATE quotation_extraction_phases
		SET updated_at = updated_at
		WHERE run_id = $1 AND phase_key = 'staffing' AND fence = $2
	`, created.Run.ID, oldFence)
	require.NoError(t, err)
	oldFenceRows = result.RowsAffected()
	require.Zero(t, oldFenceRows)
	require.Equal(t, 1, base.count("plan_metadata"))
	require.Equal(t, 1, base.count("scope"))
	require.Equal(t, 2, base.count("staffing"))
	require.Equal(t, 1, base.count("service_details"))
	require.Equal(t, 1, executor.p5Count())

	var finalizedRows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM quotation_extraction_phases WHERE run_id = $1 AND phase_key = 'finalize' AND status = 'succeeded'`, created.Run.ID).Scan(&finalizedRows))
	require.Equal(t, 1, finalizedRows)
}
