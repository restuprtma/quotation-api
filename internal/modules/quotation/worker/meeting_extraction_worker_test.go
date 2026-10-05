package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakeRepository struct {
	mu sync.Mutex

	claimCalls   int
	claims       []*domain.ExtractionRun
	reaped       int
	heartbeats   int
	transitions  []domain.ExtractionRunStatus
	heartbeatErr error
}

func (r *fakeRepository) ClaimNext(context.Context, string, time.Duration) (*domain.ExtractionRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.claimCalls++
	if len(r.claims) == 0 {
		return nil, repository.ErrNoExtractionRunAvailable
	}
	run := r.claims[0]
	r.claims = r.claims[1:]
	return run, nil
}

func (r *fakeRepository) Heartbeat(context.Context, uuid.UUID, int64, time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.heartbeats++
	return r.heartbeatErr
}

func (r *fakeRepository) ReapExpired(context.Context, time.Time, int) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reaped++
	return 1, nil
}

func (r *fakeRepository) TransitionRun(_ context.Context, _ uuid.UUID, _ int64, status domain.ExtractionRunStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.transitions = append(r.transitions, status)
	return nil
}

type fakeExecutor struct {
	started chan struct{}
	finish  chan struct{}
	err     error
}

func (e *fakeExecutor) Execute(ctx context.Context, _ *domain.ExtractionRun) error {
	close(e.started)
	if e.finish == nil {
		return e.err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.finish:
		return e.err
	}
}

func testRun() *domain.ExtractionRun {
	return &domain.ExtractionRun{ID: uuid.New(), Fence: 7}
}

func testOptions() Options {
	return Options{WorkerID: "test-worker", Lease: 50 * time.Millisecond, HeartbeatInterval: 5 * time.Millisecond, PollInterval: time.Millisecond, ReapLimit: 10}
}

func TestWorkerPollsClaimsAndRunsToCompletion(t *testing.T) {
	repo := &fakeRepository{claims: []*domain.ExtractionRun{testRun()}}
	executor := &fakeExecutor{started: make(chan struct{})}
	w := New(repo, executor, testOptions())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	<-executor.started
	waitForStatus(t, repo, domain.ExtractionRunSucceeded)
	cancel()
	w.Wait()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, 1, repo.reaped, "startup reaper must run before polling")
	require.GreaterOrEqual(t, repo.claimCalls, 1)
	require.Equal(t, []domain.ExtractionRunStatus{domain.ExtractionRunSucceeded}, repo.transitions)
}

func TestWorkerHeartbeatsLongRunningClaim(t *testing.T) {
	repo := &fakeRepository{claims: []*domain.ExtractionRun{testRun()}}
	executor := &fakeExecutor{started: make(chan struct{}), finish: make(chan struct{})}
	w := New(repo, executor, testOptions())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	<-executor.started
	time.Sleep(20 * time.Millisecond)
	cancel()
	w.Wait()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.GreaterOrEqual(t, repo.heartbeats, 1)
	require.Empty(t, repo.transitions, "shutdown must not write a terminal state")
}

func TestWorkerStaleFenceStopsExecutionWithoutOverwrite(t *testing.T) {
	repo := &fakeRepository{claims: []*domain.ExtractionRun{testRun()}, heartbeatErr: repository.ErrExtractionFenceLost}
	executor := &fakeExecutor{started: make(chan struct{}), finish: make(chan struct{})}
	w := New(repo, executor, testOptions())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	<-executor.started
	time.Sleep(20 * time.Millisecond)
	cancel()
	w.Wait()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Empty(t, repo.transitions, "stale fence must reject terminal overwrite")
}

func TestWorkerGracefulStopAndStartIsManaged(t *testing.T) {
	repo := &fakeRepository{}
	w := New(repo, &fakeExecutor{started: make(chan struct{})}, testOptions())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	require.ErrorIs(t, w.Start(ctx), ErrAlreadyStarted)
	cancel()
	w.Wait()

	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
}

func TestWorkerExecutorErrorMarksRunFailed(t *testing.T) {
	repo := &fakeRepository{claims: []*domain.ExtractionRun{testRun()}}
	executor := &fakeExecutor{started: make(chan struct{}), err: errors.New("executor failed")}
	w := New(repo, executor, testOptions())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	<-executor.started
	waitForStatus(t, repo, domain.ExtractionRunFailed)
	cancel()
	w.Wait()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Equal(t, []domain.ExtractionRunStatus{domain.ExtractionRunFailed}, repo.transitions)
}

func TestManagedWorkerFactoryNeverLeavesEnabledModeWithoutExecutor(t *testing.T) {
	for _, mode := range []string{"pipeline", "shadow"} {
		w := NewManagedWorker(&fakeRepository{}, mode, testOptions())
		require.True(t, w.HasExecutor(), mode)
		require.True(t, w.PollingEnabled(), mode)
	}

	legacy := NewManagedWorker(&fakeRepository{}, "legacy", testOptions())
	require.True(t, legacy.HasExecutor())
	require.False(t, legacy.PollingEnabled())
}

func TestProductionWorkerFactoryFailsFastForEnabledModesAndKeepsLegacyIdle(t *testing.T) {
	for _, mode := range []string{"pipeline", "shadow"} {
		_, err := NewManagedWorkerWithExecutor(&fakeRepository{}, mode, nil, testOptions())
		require.Error(t, err)
		require.Contains(t, err.Error(), "production executor")
	}
	legacy, err := NewManagedWorkerWithExecutor(&fakeRepository{}, "legacy", nil, testOptions())
	require.NoError(t, err)
	require.False(t, legacy.PollingEnabled())
}

func TestEnabledWorkerRejectsNilExecutorAtStartup(t *testing.T) {
	w := New(&fakeRepository{}, nil, testOptions())
	require.ErrorIs(t, w.Start(context.Background()), ErrExecutorRequired)
}

func TestManagedExecutorReturnsSafeUnavailableError(t *testing.T) {
	err := NewManagedExecutor("pipeline").Execute(context.Background(), testRun())
	require.ErrorIs(t, err, ErrManagedExecutionUnavailable)
}

func waitForStatus(t *testing.T, repo *fakeRepository, want domain.ExtractionRunStatus) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		repo.mu.Lock()
		for _, status := range repo.transitions {
			if status == want {
				repo.mu.Unlock()
				return
			}
		}
		repo.mu.Unlock()
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for status %q", want)
		case <-time.After(time.Millisecond):
		}
	}
}
