package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

// MeetingExtractionStoreOptions contains only policy values needed by the
// repository adapter. Secrets, notes, prompts, and provider responses never
// cross this boundary.
type MeetingExtractionStoreOptions struct {
	DailyProviderCallsMax  int64
	DailyQuotationCallsMax int64
	// Circuit settings apply independently to each provider/model pair.
	// Defaults are conservative and do not persist payloads or error text.
	CircuitFailureThreshold int
	CircuitCooldown         time.Duration
	// Now and RandomIntn make retry scheduling deterministic in integration
	// tests. Production defaults remain wall clock and a uniform 1-5 seconds.
	Now        func() time.Time
	RandomIntn func(int) int
}

// PostgreSQLMeetingExtractionOrchestratorStore adapts repository primitives
// to the service orchestrator without importing service from repository. A
// provider advisory slot is retained from ReserveCall until the phase reaches
// a terminal persistence path, so the provider call is made while the slot is
// held.
type PostgreSQLMeetingExtractionOrchestratorStore struct {
	repo    *repository.MeetingExtractionRepository
	options MeetingExtractionStoreOptions

	mu    sync.Mutex
	slots map[storeSlotKey]*repository.ProviderSlot
}

type storeSlotKey struct {
	runID    uuid.UUID
	fence    int64
	phaseKey PhaseKey
}

func NewPostgreSQLMeetingExtractionOrchestratorStore(repo *repository.MeetingExtractionRepository, options MeetingExtractionStoreOptions) (*PostgreSQLMeetingExtractionOrchestratorStore, error) {
	if repo == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrMeetingExtractionOrchestrator)
	}
	if options.DailyProviderCallsMax <= 0 || options.DailyQuotationCallsMax <= 0 {
		return nil, fmt.Errorf("%w: positive daily call limits are required", ErrMeetingExtractionOrchestrator)
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.RandomIntn == nil {
		options.RandomIntn = rand.Intn
	}
	if options.CircuitFailureThreshold <= 0 {
		options.CircuitFailureThreshold = 3
	}
	if options.CircuitCooldown <= 0 {
		options.CircuitCooldown = time.Minute
	}
	return &PostgreSQLMeetingExtractionOrchestratorStore{repo: repo, options: options, slots: make(map[storeSlotKey]*repository.ProviderSlot)}, nil
}

// RecordProviderOutcome updates only the safe circuit aggregate after an
// upstream attempt. It intentionally has no field for provider text/body.
func (s *PostgreSQLMeetingExtractionOrchestratorStore) RecordProviderOutcome(ctx context.Context, in MeetingExtractionProviderOutcome) error {
	err := s.repo.RecordExtractionProviderOutcome(ctx, repository.ExtractionProviderOutcomeParams{
		RunID: in.RunID, Fence: in.Fence, Provider: in.Provider, Model: in.Model,
		Retryable: in.Retryable, FailureThreshold: s.options.CircuitFailureThreshold, Cooldown: s.options.CircuitCooldown,
	})
	// A cancelled execution context can reject the outcome write before the
	// normal failure path releases its session advisory slot. Never leave that
	// slot held: the reclaimed worker must be able to make the next attempt.
	if err != nil {
		s.releaseSlotsForRun(context.Background(), in.RunID, in.Fence)
	}
	return s.mapError(err)
}

var _ MeetingExtractionOrchestratorStore = (*PostgreSQLMeetingExtractionOrchestratorStore)(nil)

func (s *PostgreSQLMeetingExtractionOrchestratorStore) LoadRun(ctx context.Context, runID uuid.UUID, fence int64) (MeetingExtractionRunCheckpoint, error) {
	checkpoint, err := s.repo.LoadOrchestratorRun(ctx, runID, fence)
	if err != nil {
		return MeetingExtractionRunCheckpoint{}, s.mapError(err)
	}
	return MeetingExtractionRunCheckpoint{
		Status: checkpoint.Status, RetryAfter: checkpoint.RetryAfter,
		CallsUsed: checkpoint.CallsUsed, CallsMax: checkpoint.CallsMax,
	}, nil
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) LoadPhase(ctx context.Context, runID uuid.UUID, phaseKey PhaseKey, fence int64) (MeetingExtractionPhaseCheckpoint, error) {
	checkpoint, err := s.repo.LoadOrchestratorPhase(ctx, runID, string(phaseKey), fence)
	if err != nil {
		return MeetingExtractionPhaseCheckpoint{}, s.mapError(err)
	}
	return MeetingExtractionPhaseCheckpoint{
		Status: checkpoint.Status, InputFingerprint: checkpoint.InputFingerprint,
		ResultJSON:       append([]byte(nil), checkpoint.ResultJSON...),
		TransientRetries: checkpoint.TransientRetries, RepairRetries: checkpoint.RepairRetries,
	}, nil
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) ResumeRateLimited(ctx context.Context, runID uuid.UUID, fence int64) error {
	return s.mapError(s.repo.ResumeRateLimited(ctx, runID, fence))
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) ReserveCall(ctx context.Context, in MeetingExtractionCallReservation) error {
	if in.RunID == uuid.Nil || in.Fence < 0 || in.PhaseKey == "" || in.Provider == "" || in.Model == "" || in.CallsMax <= 0 {
		return ErrMeetingExtractionBudget
	}
	key := storeSlotKey{runID: in.RunID, fence: in.Fence, phaseKey: in.PhaseKey}

	s.mu.Lock()
	_, alreadyHeld := s.slots[key]
	s.mu.Unlock()

	var slot *repository.ProviderSlot
	if !alreadyHeld {
		var err error
		slot, err = s.repo.TryAcquireProviderSlot(ctx, in.Provider, in.Model)
		if err != nil {
			if errors.Is(err, repository.ErrProviderSlotUnavailable) {
				// Do not keep a lease while another worker owns this provider slot.
				// The next claim observes retry_after, which prevents a hot loop.
				jitter := s.options.RandomIntn(5)
				if jitter < 0 || jitter > 4 {
					jitter = 0
				}
				retryAfter := s.options.Now().UTC().Add(time.Duration(jitter+1) * time.Second)
				if requeueErr := s.repo.RequeueWithRetryAfter(ctx, in.RunID, in.Fence, retryAfter); requeueErr != nil {
					return s.mapError(requeueErr)
				}
				return ErrMeetingExtractionStaleFence
			}
			return s.mapError(err)
		}
	}

	err := s.repo.ReserveExtractionCallFenced(ctx, repository.ExtractionCallReservationParams{
		RunID: in.RunID, Provider: in.Provider, Model: in.Model,
		DailyProviderCallsMax:  s.options.DailyProviderCallsMax,
		DailyQuotationCallsMax: s.options.DailyQuotationCallsMax,
	}, in.Fence)
	if err != nil {
		if slot != nil {
			slot.Release(ctx)
		}
		return s.mapError(err)
	}
	if slot != nil {
		s.mu.Lock()
		s.slots[key] = slot
		s.mu.Unlock()
	}
	return nil
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) StartPhase(ctx context.Context, runID uuid.UUID, phaseKey PhaseKey, fence int64) error {
	err := s.repo.TransitionPhase(ctx, runID, string(phaseKey), fence, domain.ExtractionPhaseRunning)
	if err != nil {
		s.releaseSlot(ctx, storeSlotKey{runID: runID, fence: fence, phaseKey: phaseKey})
	}
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) CommitPhase(ctx context.Context, in MeetingExtractionPhaseCommit) error {
	err := s.repo.PersistValidatedPhase(ctx, repository.PersistValidatedPhaseParams{
		RunID: in.RunID, PhaseKey: string(in.PhaseKey), ExpectedFence: in.Fence,
		InputFingerprint: in.InputFingerprint, ResultJSON: append([]byte(nil), in.ResultJSON...),
		EvidenceRefs: append([]string(nil), in.EvidenceRefs...), Warnings: append([]string(nil), in.Warnings...),
		TransientRetries: in.TransientRetries, RepairRetries: in.RepairRetries,
	})
	s.releaseSlot(ctx, storeSlotKey{runID: in.RunID, fence: in.Fence, phaseKey: in.PhaseKey})
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) SkipPhase(ctx context.Context, runID uuid.UUID, phaseKey PhaseKey, fence int64, fingerprint string) error {
	err := s.repo.SkipPhaseWithFence(ctx, runID, string(phaseKey), fence, fingerprint)
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) WaitRateLimit(ctx context.Context, in MeetingExtractionRateLimitWait) error {
	err := s.repo.WaitRateLimit(ctx, in.RunID, string(in.PhaseKey), in.Fence, in.RetryAfter)
	s.releaseSlot(ctx, storeSlotKey{runID: in.RunID, fence: in.Fence, phaseKey: in.PhaseKey})
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) MarkNeedsReview(ctx context.Context, runID uuid.UUID, fence int64, code string) error {
	err := s.repo.MarkExtractionNeedsReview(ctx, runID, fence, code)
	s.releaseSlotsForRun(ctx, runID, fence)
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) MarkFailed(ctx context.Context, in MeetingExtractionFailure) error {
	err := s.repo.MarkExtractionFailed(ctx, in.RunID, string(in.PhaseKey), in.Fence, in.ErrorCode, in.Retryable)
	s.releaseSlotsForRun(ctx, in.RunID, in.Fence)
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) MarkSucceeded(ctx context.Context, runID uuid.UUID, fence int64) error {
	err := s.repo.TransitionRun(ctx, runID, fence, domain.ExtractionRunSucceeded)
	s.releaseSlotsForRun(ctx, runID, fence)
	return s.mapError(err)
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) releaseSlot(ctx context.Context, key storeSlotKey) {
	s.mu.Lock()
	slot := s.slots[key]
	delete(s.slots, key)
	s.mu.Unlock()
	if slot != nil {
		slot.Release(ctx)
	}
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) releaseSlotsForRun(ctx context.Context, runID uuid.UUID, fence int64) {
	s.mu.Lock()
	keys := make([]storeSlotKey, 0)
	for key := range s.slots {
		if key.runID == runID && key.fence == fence {
			keys = append(keys, key)
		}
	}
	s.mu.Unlock()
	for _, key := range keys {
		s.releaseSlot(ctx, key)
	}
}

func (s *PostgreSQLMeetingExtractionOrchestratorStore) mapError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repository.ErrExtractionFenceLost):
		return ErrMeetingExtractionStaleFence
	case errors.Is(err, repository.ErrExtractionBudgetExhausted):
		return ErrMeetingExtractionBudget
	case errors.Is(err, repository.ErrExtractionCircuitOpen):
		return ErrMeetingExtractionCircuitOpen
	default:
		return err
	}
}
