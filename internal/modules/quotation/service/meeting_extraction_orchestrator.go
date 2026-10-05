package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	ErrMeetingExtractionOrchestrator = errors.New("meeting extraction orchestrator failed")
	ErrMeetingExtractionStaleFence   = errors.New("meeting extraction stale fence")
	ErrMeetingExtractionNotesChanged = errors.New("meeting extraction notes changed")
	ErrMeetingExtractionBudget       = errors.New("meeting extraction budget exhausted")
	ErrMeetingExtractionCircuitOpen  = errors.New("meeting extraction provider circuit open")
	ErrMeetingExtractionNotReady     = errors.New("meeting extraction is not ready")
)

// MeetingExtractionPhaseCheckpoint is the safe, resumable projection needed
// by the orchestrator. ResultJSON is already validated by the phase and must
// contain references only; it must never contain prompts, notes, or quotes.
type MeetingExtractionPhaseCheckpoint struct {
	Status           domain.ExtractionPhaseStatus
	InputFingerprint string
	ResultJSON       json.RawMessage
	TransientRetries int
	RepairRetries    int
}

type MeetingExtractionRunCheckpoint struct {
	Status     domain.ExtractionRunStatus
	RetryAfter time.Time
	CallsUsed  int
	CallsMax   int
}

// MeetingExtractionOrchestratorStore is the persistence seam for M8.1. A
// PostgreSQL implementation can delegate these operations to the existing
// repository's lease, fence, reservation, fingerprint, and safe persistence
// primitives. Every mutation carries the claimed fence.
type MeetingExtractionOrchestratorStore interface {
	LoadRun(context.Context, uuid.UUID, int64) (MeetingExtractionRunCheckpoint, error)
	LoadPhase(context.Context, uuid.UUID, PhaseKey, int64) (MeetingExtractionPhaseCheckpoint, error)
	ResumeRateLimited(context.Context, uuid.UUID, int64) error
	ReserveCall(context.Context, MeetingExtractionCallReservation) error
	StartPhase(context.Context, uuid.UUID, PhaseKey, int64) error
	CommitPhase(context.Context, MeetingExtractionPhaseCommit) error
	SkipPhase(context.Context, uuid.UUID, PhaseKey, int64, string) error
	WaitRateLimit(context.Context, MeetingExtractionRateLimitWait) error
	MarkNeedsReview(context.Context, uuid.UUID, int64, string) error
	MarkFailed(context.Context, MeetingExtractionFailure) error
	MarkSucceeded(context.Context, uuid.UUID, int64) error
}

type MeetingExtractionCallReservation struct {
	RunID     uuid.UUID
	Fence     int64
	PhaseKey  PhaseKey
	Provider  string
	Model     string
	CallsUsed int
	CallsMax  int
}

// MeetingExtractionProviderOutcome holds no upstream content: only the
// stable retry classification needed for a provider/model circuit breaker.
type MeetingExtractionProviderOutcome struct {
	RunID     uuid.UUID
	Fence     int64
	Provider  string
	Model     string
	Retryable bool
}

type meetingExtractionProviderOutcomeRecorder interface {
	RecordProviderOutcome(context.Context, MeetingExtractionProviderOutcome) error
}

type MeetingExtractionPhaseCommit struct {
	RunID            uuid.UUID
	PhaseKey         PhaseKey
	Fence            int64
	InputFingerprint string
	ResultJSON       []byte
	EvidenceRefs     []string
	Warnings         []string
	TransientRetries int
	RepairRetries    int
}

type MeetingExtractionRateLimitWait struct {
	RunID      uuid.UUID
	PhaseKey   PhaseKey
	Fence      int64
	RetryAfter time.Time
}

type MeetingExtractionFailure struct {
	RunID     uuid.UUID
	PhaseKey  PhaseKey
	Fence     int64
	ErrorCode string
	Retryable bool
}

type MeetingExtractionOrchestratorInput struct {
	RunID                uuid.UUID
	Fence                int64
	Document             *MeetingNoteDocument
	ExpectedNoteHash     string
	PipelineVersion      string
	PromptVersion        string
	SchemaVersion        string
	CatalogSnapshotHash  string
	RegistryEntryVersion string
	ManualScheme         string
	UserHints            PlanUserHints
	Provider             string
	Model                string
	CompanyID            uuid.UUID
	QuotationID          uuid.UUID
	RevisionID           uuid.UUID
	SolutionType         string
	Metadata             QuotationMetadata
	QuotationDate        time.Time
	LegalHTML            string
	RateCardID           string
	Catalog              []domain.RateCardItem
}

type MeetingExtractionOrchestratorResult struct {
	Status       domain.ExtractionRunStatus
	WaitingUntil time.Time
	Completed    []PhaseKey
	Skipped      []PhaseKey
	ErrorCode    string
	Retryable    bool
	NeedsInput   []string
}

type MeetingExtractionP5Input struct {
	RunID         uuid.UUID
	Fence         int64
	CompanyID     uuid.UUID
	QuotationID   uuid.UUID
	RevisionID    uuid.UUID
	Document      *MeetingNoteDocument
	Scheme        string
	SolutionType  string
	Metadata      QuotationMetadata
	QuotationDate time.Time
	LegalHTML     string
	RateCardID    string
	Catalog       []domain.RateCardItem
	Dependencies  map[string]json.RawMessage
	Validated     MeetingExtractionValidatedPhaseData
	Merge         MergeExtractionResult
}

type MeetingExtractionP5Func func(context.Context, MeetingExtractionP5Input) error

type MeetingExtractionPhaseSet struct {
	Plan     Phase
	Scope    Phase
	Staffing Phase
	Service  func(string) Phase
}

// NewDefaultMeetingExtractionPhaseSet wires the fixed P1-P4 graph. The
// orchestrator still owns order and control flow; this helper only constructs
// the already-validated phase implementations.
func NewDefaultMeetingExtractionPhaseSet(catalog []domain.RateCardItem) MeetingExtractionPhaseSet {
	return MeetingExtractionPhaseSet{
		Plan:     NewPlanMetadataPhase(""),
		Scope:    NewScopePhase(),
		Staffing: NewStaffingPhase(catalog),
		Service: func(scheme string) Phase {
			return NewServiceDetailsPhase(scheme)
		},
	}
}

type MeetingExtractionOrchestratorConfig struct {
	Now              func() time.Time
	TransientRetries int
	RepairRetries    int
	BackoffBase      time.Duration
	BackoffMaximum   time.Duration
	Jitter           func(time.Duration) time.Duration
}

type MeetingExtractionOrchestrator struct {
	provider ExtractionProvider
	store    MeetingExtractionOrchestratorStore
	phases   MeetingExtractionPhaseSet
	p5       MeetingExtractionP5Func
	config   MeetingExtractionOrchestratorConfig
}

func NewMeetingExtractionOrchestrator(
	provider ExtractionProvider,
	store MeetingExtractionOrchestratorStore,
	phases MeetingExtractionPhaseSet,
	p5 MeetingExtractionP5Func,
	config MeetingExtractionOrchestratorConfig,
) (*MeetingExtractionOrchestrator, error) {
	if provider == nil || store == nil || phases.Plan == nil || phases.Scope == nil || phases.Staffing == nil || phases.Service == nil || p5 == nil {
		return nil, fmt.Errorf("%w: incomplete dependencies", ErrMeetingExtractionOrchestrator)
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.TransientRetries < 0 {
		config.TransientRetries = 0
	}
	if config.RepairRetries < 0 {
		config.RepairRetries = 0
	}
	if config.TransientRetries == 0 {
		config.TransientRetries = 1
	}
	if config.RepairRetries == 0 {
		config.RepairRetries = 1
	}
	if config.BackoffBase <= 0 {
		config.BackoffBase = time.Second
	}
	if config.BackoffMaximum <= 0 {
		config.BackoffMaximum = 30 * time.Second
	}
	if config.Jitter == nil {
		config.Jitter = func(d time.Duration) time.Duration { return d }
	}
	return &MeetingExtractionOrchestrator{provider: provider, store: store, phases: phases, p5: p5, config: config}, nil
}

func (o *MeetingExtractionOrchestrator) Run(ctx context.Context, in MeetingExtractionOrchestratorInput) (MeetingExtractionOrchestratorResult, error) {
	if o == nil || in.RunID == uuid.Nil || in.Fence < 0 || in.Document == nil || len(in.ExpectedNoteHash) != 64 {
		return MeetingExtractionOrchestratorResult{}, fmt.Errorf("%w: invalid input", ErrMeetingExtractionOrchestrator)
	}
	if in.Document.Hash() != strings.ToLower(in.ExpectedNoteHash) {
		return MeetingExtractionOrchestratorResult{Status: domain.ExtractionRunFailed, ErrorCode: ErrExtractionNotesChanged}, fmt.Errorf("%w: %w", ErrMeetingExtractionOrchestrator, ErrMeetingExtractionNotesChanged)
	}

	run, err := o.store.LoadRun(ctx, in.RunID, in.Fence)
	if err != nil {
		return MeetingExtractionOrchestratorResult{}, o.storeError(err)
	}
	if run.Status == domain.ExtractionRunSucceeded || run.Status == domain.ExtractionRunNeedsReview || run.Status == domain.ExtractionRunCancelled {
		return MeetingExtractionOrchestratorResult{Status: run.Status}, nil
	}
	if run.Status == domain.ExtractionRunWaitingRateLimit {
		if run.RetryAfter.IsZero() || o.config.Now().Before(run.RetryAfter) {
			return MeetingExtractionOrchestratorResult{Status: run.Status, WaitingUntil: run.RetryAfter}, nil
		}
		if err := o.store.ResumeRateLimited(ctx, in.RunID, in.Fence); err != nil {
			return MeetingExtractionOrchestratorResult{}, o.storeError(err)
		}
	}
	if run.Status != domain.ExtractionRunRunning && run.Status != domain.ExtractionRunQueued && run.Status != domain.ExtractionRunWaitingRateLimit {
		return MeetingExtractionOrchestratorResult{}, fmt.Errorf("%w: run is not executable", ErrMeetingExtractionNotReady)
	}

	deps := make(map[string]json.RawMessage, len(domain.ExtractionPhaseOrder))
	result := MeetingExtractionOrchestratorResult{Status: domain.ExtractionRunRunning}
	for _, spec := range o.phaseSpecs(in, deps) {
		phaseResult, phaseStatus, err := o.runPhase(ctx, in, run, spec, deps)
		if err != nil {
			if wait, ok := err.(*meetingExtractionWaitingError); ok {
				result.Status = domain.ExtractionRunWaitingRateLimit
				result.WaitingUntil = wait.retryAfter
				return result, nil
			}
			return result, err
		}
		if len(phaseResult) > 0 {
			deps[string(spec.key)] = append(json.RawMessage(nil), phaseResult...)
		}
		if phaseStatus == domain.ExtractionPhaseSkipped {
			result.Skipped = append(result.Skipped, spec.key)
		} else {
			result.Completed = append(result.Completed, spec.key)
		}
		if spec.key == PhaseKeyPlanMetadata && (planNeedsReview(phaseResult) || !PipelineSchemeReady(planScheme(phaseResult))) {
			code := "SCHEME_REVIEW_REQUIRED"
			needsInput := []string(nil)
			if !PipelineSchemeReady(planScheme(phaseResult)) {
				code = "SCHEME_MAPPER_UNAVAILABLE"
				needsInput = []string{"commercial_scheme: choose a supported manual scheme or continue through legacy"}
			}
			if err := o.store.MarkNeedsReview(ctx, in.RunID, in.Fence, code); err != nil {
				return result, o.storeError(err)
			}
			result.Status = domain.ExtractionRunNeedsReview
			result.ErrorCode = code
			result.NeedsInput = needsInput
			return result, nil
		}
	}

	if err := o.store.MarkSucceeded(ctx, in.RunID, in.Fence); err != nil {
		return result, o.storeError(err)
	}
	result.Status = domain.ExtractionRunSucceeded
	return result, nil
}

type meetingExtractionPhaseSpec struct {
	key   PhaseKey
	phase Phase
	final bool
}

func (o *MeetingExtractionOrchestrator) phaseSpecs(in MeetingExtractionOrchestratorInput, deps map[string]json.RawMessage) []meetingExtractionPhaseSpec {
	_ = in
	_ = deps
	return []meetingExtractionPhaseSpec{
		{key: PhaseKeyPlanMetadata, phase: o.phases.Plan},
		{key: PhaseKeyScope, phase: o.phases.Scope},
		{key: PhaseKeyStaffing, phase: o.phases.Staffing},
		{key: PhaseKeyServiceDetails},
		{key: PhaseKey(domain.ExtractionPhaseFinalize), final: true},
	}
}

func (o *MeetingExtractionOrchestrator) runPhase(ctx context.Context, in MeetingExtractionOrchestratorInput, run MeetingExtractionRunCheckpoint, spec meetingExtractionPhaseSpec, deps map[string]json.RawMessage) ([]byte, domain.ExtractionPhaseStatus, error) {
	phase := spec.phase
	phaseInput := PhaseInput{
		Document:             in.Document,
		Dependencies:         cloneRawMap(deps),
		PipelineVersion:      in.PipelineVersion,
		PromptVersion:        in.PromptVersion,
		SchemaVersion:        in.SchemaVersion,
		CatalogSnapshotHash:  in.CatalogSnapshotHash,
		RegistryEntryVersion: in.RegistryEntryVersion,
		ManualScheme:         in.ManualScheme,
		UserHints:            in.UserHints,
	}
	if spec.key == PhaseKeyServiceDetails {
		scheme := planScheme(deps[string(PhaseKeyPlanMetadata)])
		phase = o.phases.Service(scheme)
		if phase == nil {
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionInternal, false)
		}
	}
	fingerprint := InputFingerprint(spec.key, phaseInput)
	checkpoint, err := o.store.LoadPhase(ctx, in.RunID, spec.key, in.Fence)
	if err != nil {
		return nil, domain.ExtractionPhaseFailed, o.storeError(err)
	}
	if checkpoint.Status == domain.ExtractionPhaseSucceeded && checkpoint.InputFingerprint == fingerprint && len(checkpoint.ResultJSON) > 0 {
		return append([]byte(nil), checkpoint.ResultJSON...), checkpoint.Status, nil
	}

	if spec.key == PhaseKeyScope {
		if skipper, ok := phase.(interface {
			ShouldSkip(PhaseInput) (bool, error)
		}); ok {
			skip, err := skipper.ShouldSkip(phaseInput)
			if err != nil {
				return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionSchemaInvalid, false)
			}
			if skip {
				if err := o.store.SkipPhase(ctx, in.RunID, spec.key, in.Fence, fingerprint); err != nil {
					return nil, domain.ExtractionPhaseFailed, o.storeError(err)
				}
				return nil, domain.ExtractionPhaseSkipped, nil
			}
		}
	}
	if spec.final {
		return o.runFinalize(ctx, in, phaseInput, fingerprint, deps)
	}

	request, err := phase.BuildPrompt(ctx, phaseInput)
	if err != nil {
		return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionInternal, false)
	}
	if err := o.store.StartPhase(ctx, in.RunID, spec.key, in.Fence); err != nil {
		return nil, domain.ExtractionPhaseFailed, o.storeError(err)
	}
	transientRetries, repairRetries := checkpoint.TransientRetries, checkpoint.RepairRetries
	for {
		if err := o.store.ReserveCall(ctx, MeetingExtractionCallReservation{RunID: in.RunID, Fence: in.Fence, PhaseKey: spec.key, Provider: in.Provider, Model: in.Model, CallsUsed: run.CallsUsed, CallsMax: run.CallsMax}); err != nil {
			if errors.Is(err, ErrMeetingExtractionCircuitOpen) {
				return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionProviderUnavailable, true)
			}
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionBudgetExhausted, false)
		}
		callCtx, cancel := ExtractionCallContext(ctx, 0)
		generated, providerErr := o.provider.GenerateExtraction(callCtx, ExtractionGenerateRequest{Prompt: request.Prompt, JSONSchema: request.JSONSchema, MaxOutputTokens: request.MaxOutputTokens, Thinking: ThinkingConfig{Mode: ThinkingDefault}})
		cancel()
		if recorder, ok := o.store.(meetingExtractionProviderOutcomeRecorder); ok {
			retryable := false
			if providerErr != nil {
				retryable = safeExtractionError(providerErr).Retryable
			}
			if err := recorder.RecordProviderOutcome(ctx, MeetingExtractionProviderOutcome{RunID: in.RunID, Fence: in.Fence, Provider: in.Provider, Model: in.Model, Retryable: retryable}); err != nil {
				return nil, domain.ExtractionPhaseFailed, o.storeError(err)
			}
		}
		if providerErr != nil {
			safe := safeExtractionError(providerErr)
			if safe.Code == ErrExtractionProviderRateLimited {
				retryAfter := safe.RetryAfter
				if retryAfter.IsZero() {
					retryAfter = o.config.Now().Add(ExtractionBackoff(transientRetries, o.config.BackoffBase, o.config.BackoffMaximum, o.config.Jitter))
				}
				if err := o.store.WaitRateLimit(ctx, MeetingExtractionRateLimitWait{RunID: in.RunID, PhaseKey: spec.key, Fence: in.Fence, RetryAfter: retryAfter}); err != nil {
					return nil, domain.ExtractionPhaseFailed, o.storeError(err)
				}
				return nil, domain.ExtractionPhasePending, &meetingExtractionWaitingError{retryAfter: retryAfter}
			}
			if safe.Retryable && transientRetries < o.config.TransientRetries {
				transientRetries++
				continue
			}
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, safe.Code, false)
		}
		if !json.Valid([]byte(generated.Text)) {
			if repairRetries < o.config.RepairRetries {
				repairRetries++
				continue
			}
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionInvalidJSON, false)
		}
		validated, validationErr := phase.Validate(ctx, phaseInput, json.RawMessage(generated.Text))
		if validationErr != nil {
			if repairRetries < o.config.RepairRetries {
				repairRetries++
				continue
			}
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionSchemaInvalid, false)
		}
		resultJSON, err := validated.SafeResultJSON()
		if err != nil || !json.Valid(resultJSON) {
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, spec.key, ErrExtractionInternal, false)
		}
		if err := o.store.CommitPhase(ctx, MeetingExtractionPhaseCommit{RunID: in.RunID, PhaseKey: spec.key, Fence: in.Fence, InputFingerprint: fingerprint, ResultJSON: resultJSON, EvidenceRefs: validated.EvidenceReferenceIDs(), Warnings: validated.WarningCodes(), TransientRetries: transientRetries, RepairRetries: repairRetries}); err != nil {
			return nil, domain.ExtractionPhaseFailed, o.storeError(err)
		}
		return resultJSON, domain.ExtractionPhaseSucceeded, nil
	}
}

func (o *MeetingExtractionOrchestrator) runFinalize(ctx context.Context, in MeetingExtractionOrchestratorInput, phaseInput PhaseInput, fingerprint string, deps map[string]json.RawMessage) ([]byte, domain.ExtractionPhaseStatus, error) {
	checkpoint, err := o.store.LoadPhase(ctx, in.RunID, PhaseKey(domain.ExtractionPhaseFinalize), in.Fence)
	if err != nil {
		return nil, domain.ExtractionPhaseFailed, o.storeError(err)
	}
	if checkpoint.Status == domain.ExtractionPhaseSucceeded && checkpoint.InputFingerprint == fingerprint {
		return append([]byte(nil), checkpoint.ResultJSON...), checkpoint.Status, nil
	}
	if err := o.store.StartPhase(ctx, in.RunID, PhaseKey(domain.ExtractionPhaseFinalize), in.Fence); err != nil {
		return nil, domain.ExtractionPhaseFailed, o.storeError(err)
	}
	scheme := planScheme(deps[string(PhaseKeyPlanMetadata)])
	validated := MeetingExtractionValidatedPhaseData{}
	if raw := deps[string(PhaseKeyPlanMetadata)]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &validated.Plan); err != nil {
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, PhaseKey(domain.ExtractionPhaseFinalize), ErrExtractionSchemaInvalid, false)
		}
	}
	if raw := deps[string(PhaseKeyScope)]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &validated.Scope); err != nil {
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, PhaseKey(domain.ExtractionPhaseFinalize), ErrExtractionSchemaInvalid, false)
		}
	}
	if raw := deps[string(PhaseKeyStaffing)]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &validated.Staffing); err != nil {
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, PhaseKey(domain.ExtractionPhaseFinalize), ErrExtractionSchemaInvalid, false)
		}
	}
	if raw := deps[string(PhaseKeyServiceDetails)]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &validated.Service); err != nil {
			return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, PhaseKey(domain.ExtractionPhaseFinalize), ErrExtractionSchemaInvalid, false)
		}
	}
	if err := o.p5(ctx, MeetingExtractionP5Input{
		RunID: in.RunID, Fence: in.Fence, CompanyID: in.CompanyID, QuotationID: in.QuotationID, RevisionID: in.RevisionID,
		Document: in.Document, Scheme: scheme, SolutionType: in.SolutionType, Metadata: in.Metadata,
		QuotationDate: in.QuotationDate, LegalHTML: in.LegalHTML, RateCardID: in.RateCardID,
		Catalog: append([]domain.RateCardItem(nil), in.Catalog...), Dependencies: cloneRawMap(deps), Validated: validated,
		Merge: buildMeetingExtractionMerge(validated),
	}); err != nil {
		return nil, domain.ExtractionPhaseFailed, o.fail(ctx, in, PhaseKey(domain.ExtractionPhaseFinalize), ErrExtractionInternal, false)
	}
	resultJSON := []byte(`{"finalized":true}`)
	if err := o.store.CommitPhase(ctx, MeetingExtractionPhaseCommit{RunID: in.RunID, PhaseKey: PhaseKey(domain.ExtractionPhaseFinalize), Fence: in.Fence, InputFingerprint: fingerprint, ResultJSON: resultJSON}); err != nil {
		return nil, domain.ExtractionPhaseFailed, o.storeError(err)
	}
	return resultJSON, domain.ExtractionPhaseSucceeded, nil
}

type meetingExtractionWaitingError struct{ retryAfter time.Time }

func (e *meetingExtractionWaitingError) Error() string {
	return "meeting extraction waiting for rate limit"
}

func (o *MeetingExtractionOrchestrator) fail(ctx context.Context, in MeetingExtractionOrchestratorInput, phase PhaseKey, code string, retryable bool) error {
	err := o.store.MarkFailed(ctx, MeetingExtractionFailure{RunID: in.RunID, PhaseKey: phase, Fence: in.Fence, ErrorCode: code, Retryable: retryable})
	if err != nil {
		return o.storeError(err)
	}
	return &MeetingExtractionOrchestratorError{Code: code, Retryable: retryable}
}

type MeetingExtractionOrchestratorError struct {
	Code      string
	Retryable bool
}

func (e *MeetingExtractionOrchestratorError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func (e *MeetingExtractionOrchestratorError) Is(target error) bool {
	switch target {
	case ErrMeetingExtractionNotesChanged:
		return e != nil && e.Code == ErrExtractionNotesChanged
	case ErrMeetingExtractionBudget:
		return e != nil && e.Code == ErrExtractionBudgetExhausted
	}
	return false
}

func (o *MeetingExtractionOrchestrator) storeError(err error) error {
	if errors.Is(err, ErrMeetingExtractionStaleFence) {
		return ErrMeetingExtractionStaleFence
	}
	return err
}

func safeExtractionError(err error) *ExtractionProviderError {
	var safe *ExtractionProviderError
	if errors.As(err, &safe) {
		return safe
	}
	return ExtractionErrorFromProvider(err)
}

func planNeedsReview(raw []byte) bool {
	var plan struct {
		Scheme     string   `json:"scheme"`
		Confidence *float64 `json:"confidence"`
	}
	if json.Unmarshal(raw, &plan) != nil {
		return true
	}
	return plan.Scheme == PlanSchemeUnknown || plan.Scheme == PlanSchemeMultiService || plan.Confidence != nil && *plan.Confidence < 0.70
}

func planScheme(raw []byte) string {
	var plan struct {
		Scheme string `json:"scheme"`
	}
	if json.Unmarshal(raw, &plan) != nil {
		return ""
	}
	return strings.TrimSpace(plan.Scheme)
}

func cloneRawMap(source map[string]json.RawMessage) map[string]json.RawMessage {
	clone := make(map[string]json.RawMessage, len(source))
	for key, value := range source {
		clone[key] = append(json.RawMessage(nil), value...)
	}
	return clone
}
