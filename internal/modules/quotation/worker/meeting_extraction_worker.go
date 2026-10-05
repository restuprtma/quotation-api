package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrAlreadyStarted              = errors.New("meeting extraction worker already started")
	ErrNilContext                  = errors.New("meeting extraction worker context is nil")
	ErrExecutorRequired            = errors.New("meeting extraction worker executor is required for enabled mode")
	ErrManagedExecutionUnavailable = errors.New("meeting extraction pipeline executor is unavailable")
)

// Repository is the lifecycle subset of the extraction repository. Keeping
// this seam small lets the worker be tested without changing the orchestrator
// or requiring a database in unit tests.
type Repository interface {
	ClaimNext(context.Context, string, time.Duration) (*domain.ExtractionRun, error)
	Heartbeat(context.Context, uuid.UUID, int64, time.Duration) error
	ReapExpired(context.Context, time.Time, int) (int, error)
	TransitionRun(context.Context, uuid.UUID, int64, domain.ExtractionRunStatus) error
}

// Executor owns the actual extraction/orchestration work for one claimed run.
// It must use the run fence for every durable write.
type Executor interface {
	Execute(context.Context, *domain.ExtractionRun) error
}

type Options struct {
	WorkerID          string
	Lease             time.Duration
	HeartbeatInterval time.Duration
	PollInterval      time.Duration
	ReapLimit         int
	Disabled          bool
	Now               func() time.Time
}

func DefaultOptions() Options {
	return Options{
		WorkerID:          "meeting-extraction-worker",
		Lease:             30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		PollInterval:      time.Second,
		ReapLimit:         100,
		Now:               time.Now,
	}
}

func (o Options) withDefaults() Options {
	d := DefaultOptions()
	if o.WorkerID == "" {
		o.WorkerID = d.WorkerID
	}
	if o.Lease <= 0 {
		o.Lease = d.Lease
	}
	if o.HeartbeatInterval <= 0 {
		o.HeartbeatInterval = d.HeartbeatInterval
	}
	if o.PollInterval <= 0 {
		o.PollInterval = d.PollInterval
	}
	if o.ReapLimit <= 0 {
		o.ReapLimit = d.ReapLimit
	}
	if o.Now == nil {
		o.Now = d.Now
	}
	return o
}

type Worker struct {
	repo     Repository
	executor Executor
	options  Options

	mu      sync.Mutex
	started bool
	done    chan struct{}
}

func New(repo Repository, executor Executor, options Options) *Worker {
	return &Worker{repo: repo, executor: executor, options: options.withDefaults()}
}

// NewManagedWorker is the production lifecycle factory. Legacy mode is an
// intentionally idle worker; pipeline and shadow modes poll only when their
// executor is present. The factory always supplies a non-nil managed executor
// so enabled modes cannot silently stall due to a nil dependency.
func NewManagedWorker(repo Repository, mode string, options Options) *Worker {
	options.Disabled = options.Disabled || (mode != "pipeline" && mode != "shadow")
	return New(repo, NewManagedExecutor(mode), options)
}

// NewManagedWorkerWithExecutor is the production composition entrypoint.
// Enabled modes must receive a fully constructed executor; there is no
// runtime fallback or no-op path.
func NewManagedWorkerWithExecutor(repo Repository, mode string, executor Executor, options Options) (*Worker, error) {
	enabled := mode == "pipeline" || mode == "shadow"
	if enabled && executor == nil {
		return nil, fmt.Errorf("meeting extraction %s startup dependency missing: production executor", mode)
	}
	options.Disabled = options.Disabled || !enabled
	return New(repo, executor, options), nil
}

// NewManagedExecutor provides an explicit safe seam until the PostgreSQL
// orchestrator store/input adapter is available. It returns a stable error;
// the worker then fenced-transitions the claimed run to failed.
func NewManagedExecutor(mode string) Executor {
	return managedExecutor{mode: mode}
}

type managedExecutor struct{ mode string }

func (e managedExecutor) Execute(context.Context, *domain.ExtractionRun) error {
	return fmt.Errorf("%w: mode=%s", ErrManagedExecutionUnavailable, e.mode)
}

func (w *Worker) HasExecutor() bool {
	return w != nil && w.executor != nil
}

func (w *Worker) PollingEnabled() bool {
	return w != nil && !w.options.Disabled
}

// Start starts exactly one managed worker goroutine. The goroutine exits when
// ctx is cancelled; callers must call Wait during process shutdown.
func (w *Worker) Start(ctx context.Context) error {
	if ctx == nil {
		return ErrNilContext
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return ErrAlreadyStarted
	}
	if !w.options.Disabled && w.executor == nil {
		return ErrExecutorRequired
	}
	w.started = true
	w.done = make(chan struct{})
	go func() {
		defer close(w.done)
		w.run(ctx)
	}()
	return nil
}

// Wait waits for the managed worker to stop. It is safe to call before Start
// and multiple times.
func (w *Worker) Wait() {
	w.mu.Lock()
	done := w.done
	w.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (w *Worker) run(ctx context.Context) {
	if w.repo == nil || w.options.Disabled || w.executor == nil {
		<-ctx.Done()
		return
	}

	// Reaping before the first poll makes recovery deterministic after startup.
	// ClaimNext also reclaims expired runs, so a failed reaper does not prevent
	// progress on the next poll.
	_, _ = w.repo.ReapExpired(ctx, w.options.Now().UTC(), w.options.ReapLimit)

	poll := time.NewTicker(w.options.PollInterval)
	defer poll.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		w.pollOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-poll.C:
		}
	}
}

func (w *Worker) pollOnce(ctx context.Context) {
	run, err := w.repo.ClaimNext(ctx, w.options.WorkerID, w.options.Lease)
	if err != nil {
		if errors.Is(err, repository.ErrNoExtractionRunAvailable) || ctx.Err() != nil {
			return
		}
		return
	}
	if run == nil {
		return
	}
	w.executeClaim(ctx, run)
}

func (w *Worker) executeClaim(parent context.Context, run *domain.ExtractionRun) {
	if run == nil || parent.Err() != nil {
		return
	}

	execCtx, cancel := context.WithCancel(parent)
	heartbeatErr := make(chan error, 1)
	heartbeatDone := make(chan struct{})
	go w.heartbeat(execCtx, run, cancel, heartbeatErr, heartbeatDone)

	executeErr := w.executor.Execute(execCtx, run)
	cancel()
	<-heartbeatDone

	if parent.Err() != nil || errors.Is(<-firstError(heartbeatErr), repository.ErrExtractionFenceLost) {
		return
	}
	if executeErr == nil {
		_ = w.repo.TransitionRun(parent, run.ID, run.Fence, domain.ExtractionRunSucceeded)
		return
	}
	if errors.Is(executeErr, repository.ErrExtractionFenceLost) {
		return
	}
	_ = w.repo.TransitionRun(parent, run.ID, run.Fence, domain.ExtractionRunFailed)
}

func (w *Worker) heartbeat(ctx context.Context, run *domain.ExtractionRun, cancel context.CancelFunc, errCh chan<- error, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(w.options.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			err := w.repo.Heartbeat(ctx, run.ID, run.Fence, w.options.Lease)
			if err != nil {
				errCh <- err
				cancel()
				return
			}
		}
	}
}

func firstError(errCh <-chan error) <-chan error {
	result := make(chan error, 1)
	select {
	case err := <-errCh:
		result <- err
	default:
		result <- nil
	}
	return result
}
