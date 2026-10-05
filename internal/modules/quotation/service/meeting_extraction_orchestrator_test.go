package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type orchestratorFakePhase struct {
	key    PhaseKey
	result []byte
}

func (p orchestratorFakePhase) Key() PhaseKey     { return p.key }
func (p orchestratorFakePhase) Owns() []FieldPath { return []FieldPath{FieldPath(string(p.key))} }
func (p orchestratorFakePhase) BuildPrompt(context.Context, PhaseInput) (ProviderRequest, error) {
	return ProviderRequest{Prompt: string(p.key), JSONSchema: json.RawMessage(`{}`), MaxOutputTokens: 32}, nil
}
func (p orchestratorFakePhase) Schema(PhaseInput) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (p orchestratorFakePhase) Validate(context.Context, PhaseInput, json.RawMessage) (PhaseResult, error) {
	return orchestratorFakeResult{key: p.key, result: p.result}, nil
}

type orchestratorFakeResult struct {
	key    PhaseKey
	result []byte
}

func (r orchestratorFakeResult) PhaseKey() PhaseKey { return r.key }
func (r orchestratorFakeResult) SafeResultJSON() ([]byte, error) {
	return append([]byte(nil), r.result...), nil
}
func (orchestratorFakeResult) EvidenceReferenceIDs() []string { return nil }
func (orchestratorFakeResult) WarningCodes() []string         { return nil }

type orchestratorFakeProvider struct {
	mu        sync.Mutex
	counts    map[string]int
	responses map[string][]orchestratorProviderResponse
}

type orchestratorProviderResponse struct {
	text string
	err  error
}

func (p *orchestratorFakeProvider) ExtractionCapabilities() ProviderCapabilities {
	return ProviderCapabilities{JSONSchema: true}
}
func (p *orchestratorFakeProvider) GenerateExtraction(_ context.Context, request ExtractionGenerateRequest) (ExtractionGenerateResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.counts[request.Prompt]++
	responses := p.responses[request.Prompt]
	index := p.counts[request.Prompt] - 1
	if index >= len(responses) {
		return ExtractionGenerateResult{}, fmt.Errorf("missing fake response for %s", request.Prompt)
	}
	response := responses[index]
	return ExtractionGenerateResult{Text: response.text}, response.err
}
func (p *orchestratorFakeProvider) count(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts[key]
}

type orchestratorFakeStore struct {
	mu            sync.Mutex
	run           MeetingExtractionRunCheckpoint
	phases        map[PhaseKey]MeetingExtractionPhaseCheckpoint
	started       map[PhaseKey]bool
	staleOnCommit bool
	failed        MeetingExtractionFailure
	reviewCode    string
}

func (s *orchestratorFakeStore) LoadRun(context.Context, uuid.UUID, int64) (MeetingExtractionRunCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run, nil
}
func (s *orchestratorFakeStore) LoadPhase(_ context.Context, _ uuid.UUID, key PhaseKey, _ int64) (MeetingExtractionPhaseCheckpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phases[key], nil
}
func (s *orchestratorFakeStore) ResumeRateLimited(context.Context, uuid.UUID, int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.Status = domain.ExtractionRunRunning
	s.run.RetryAfter = time.Time{}
	return nil
}
func (s *orchestratorFakeStore) ReserveCall(_ context.Context, p MeetingExtractionCallReservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.run.CallsUsed >= s.run.CallsMax {
		return ErrMeetingExtractionBudget
	}
	s.run.CallsUsed++
	return nil
}

func (s *orchestratorFakeStore) StartPhase(_ context.Context, _ uuid.UUID, key PhaseKey, _ int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started == nil {
		s.started = map[PhaseKey]bool{}
	}
	if s.started[key] {
		return domain.ErrInvalidExtractionTransition
	}
	s.started[key] = true
	return nil
}
func (s *orchestratorFakeStore) CommitPhase(_ context.Context, p MeetingExtractionPhaseCommit) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.staleOnCommit {
		return ErrMeetingExtractionStaleFence
	}
	if !s.started[p.PhaseKey] {
		return ErrMeetingExtractionStaleFence
	}
	s.phases[p.PhaseKey] = MeetingExtractionPhaseCheckpoint{Status: domain.ExtractionPhaseSucceeded, InputFingerprint: p.InputFingerprint, ResultJSON: append([]byte(nil), p.ResultJSON...), TransientRetries: p.TransientRetries, RepairRetries: p.RepairRetries}
	return nil
}
func (s *orchestratorFakeStore) SkipPhase(_ context.Context, _ uuid.UUID, key PhaseKey, _ int64, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phases[key] = MeetingExtractionPhaseCheckpoint{Status: domain.ExtractionPhaseSkipped, InputFingerprint: fingerprint}
	return nil
}
func (s *orchestratorFakeStore) WaitRateLimit(_ context.Context, p MeetingExtractionRateLimitWait) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.Status = domain.ExtractionRunWaitingRateLimit
	s.run.RetryAfter = p.RetryAfter
	s.phases[p.PhaseKey] = MeetingExtractionPhaseCheckpoint{Status: domain.ExtractionPhasePending}
	delete(s.started, p.PhaseKey)
	return nil
}
func (s *orchestratorFakeStore) MarkNeedsReview(_ context.Context, _ uuid.UUID, _ int64, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.Status = domain.ExtractionRunNeedsReview
	s.reviewCode = code
	return nil
}
func (s *orchestratorFakeStore) MarkFailed(_ context.Context, failure MeetingExtractionFailure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.Status = domain.ExtractionRunFailed
	s.failed = failure
	return nil
}
func (s *orchestratorFakeStore) MarkSucceeded(context.Context, uuid.UUID, int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.run.Status = domain.ExtractionRunSucceeded
	return nil
}

func newOrchestratorTest(t *testing.T, provider *orchestratorFakeProvider, store *orchestratorFakeStore, now *time.Time) *MeetingExtractionOrchestrator {
	t.Helper()
	phases := MeetingExtractionPhaseSet{
		Plan:     orchestratorFakePhase{key: PhaseKeyPlanMetadata, result: []byte(`{"scheme":"dedicated_squad","confidence":0.95,"available_sections":[{"section":"scope","refs":["L0001"]}],"metadata":{}}`)},
		Scope:    orchestratorFakePhase{key: PhaseKeyScope, result: []byte(`{"scope":[]}`)},
		Staffing: orchestratorFakePhase{key: PhaseKeyStaffing, result: []byte(`{"duration_months":null,"team":[]}`)},
		Service: func(scheme string) Phase {
			return orchestratorFakePhase{key: PhaseKeyServiceDetails, result: []byte(`{"scheme":"` + scheme + `","facts":[]}`)}
		},
	}
	orchestrator, err := NewMeetingExtractionOrchestrator(provider, store, phases, func(context.Context, MeetingExtractionP5Input) error { return nil }, MeetingExtractionOrchestratorConfig{Now: func() time.Time { return *now }, Jitter: func(d time.Duration) time.Duration { return d }})
	require.NoError(t, err)
	return orchestrator
}

func TestNewMeetingExtractionOrchestratorRejectsNilP5(t *testing.T) {
	now := time.Unix(100, 0)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	phases := MeetingExtractionPhaseSet{
		Plan:     orchestratorFakePhase{key: PhaseKeyPlanMetadata, result: []byte(`{}`)},
		Scope:    orchestratorFakePhase{key: PhaseKeyScope, result: []byte(`{}`)},
		Staffing: orchestratorFakePhase{key: PhaseKeyStaffing, result: []byte(`{}`)},
		Service:  func(string) Phase { return orchestratorFakePhase{key: PhaseKeyServiceDetails, result: []byte(`{}`)} },
	}
	_, err := NewMeetingExtractionOrchestrator(provider, store, phases, nil, MeetingExtractionOrchestratorConfig{Now: func() time.Time { return now }})
	require.ErrorIs(t, err, ErrMeetingExtractionOrchestrator)
}

func newOrchestratorInput(t *testing.T, notes string) (MeetingExtractionOrchestratorInput, *MeetingNoteDocument) {
	t.Helper()
	doc, err := NewMeetingNoteDocument(notes, MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	return MeetingExtractionOrchestratorInput{RunID: uuid.New(), Fence: 1, Document: doc, ExpectedNoteHash: doc.Hash(), PipelineVersion: "pipeline-v1", PromptVersion: "p1", SchemaVersion: "s1", CatalogSnapshotHash: "catalog", RegistryEntryVersion: "registry", Provider: "gemini", Model: "test-model"}, doc
}

func TestMeetingExtractionOrchestratorP3TimeoutRetriesOnlyFailedPhase(t *testing.T) {
	now := time.Unix(100, 0)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{
		string(PhaseKeyPlanMetadata): {{text: `{}`}}, string(PhaseKeyScope): {{text: `{}`}}, string(PhaseKeyStaffing): {{err: &ExtractionProviderError{Code: ErrExtractionProviderTimeout, Retryable: true}}, {text: `{}`}}, string(PhaseKeyServiceDetails): {{text: `{}`}},
	}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	orchestrator := newOrchestratorTest(t, provider, store, &now)
	in, _ := newOrchestratorInput(t, "scope")
	result, err := orchestrator.Run(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunSucceeded, result.Status)
	require.Equal(t, 1, provider.count(string(PhaseKeyPlanMetadata)))
	require.Equal(t, 1, provider.count(string(PhaseKeyScope)))
	require.Equal(t, 2, provider.count(string(PhaseKeyStaffing)))
	require.Equal(t, 1, provider.count(string(PhaseKeyServiceDetails)))
	require.Equal(t, 5, store.run.CallsUsed)
}

func TestMeetingExtractionOrchestratorAutoUnmappedSchemeStopsAfterP1(t *testing.T) {
	now := time.Unix(100, 0)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{
		string(PhaseKeyPlanMetadata): {{text: `{}`}},
	}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	phases := MeetingExtractionPhaseSet{
		Plan:     orchestratorFakePhase{key: PhaseKeyPlanMetadata, result: []byte(`{"scheme":"subscription_saas","confidence":0.95}`)},
		Scope:    orchestratorFakePhase{key: PhaseKeyScope, result: []byte(`{"scope":[]}`)},
		Staffing: orchestratorFakePhase{key: PhaseKeyStaffing, result: []byte(`{"team":[]}`)},
		Service: func(scheme string) Phase {
			return orchestratorFakePhase{key: PhaseKeyServiceDetails, result: []byte(`{"scheme":"` + scheme + `"}`)}
		},
	}
	orchestrator, err := NewMeetingExtractionOrchestrator(provider, store, phases, func(context.Context, MeetingExtractionP5Input) error {
		t.Fatal("P5 must not be called")
		return nil
	}, MeetingExtractionOrchestratorConfig{Now: func() time.Time { return now }})
	require.NoError(t, err)
	in, _ := newOrchestratorInput(t, "scope")
	result, err := orchestrator.Run(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunNeedsReview, result.Status)
	require.Equal(t, "SCHEME_MAPPER_UNAVAILABLE", result.ErrorCode)
	require.Equal(t, []string{"commercial_scheme: choose a supported manual scheme or continue through legacy"}, result.NeedsInput)
	require.Equal(t, "SCHEME_MAPPER_UNAVAILABLE", store.reviewCode)
	require.Equal(t, 1, provider.count(string(PhaseKeyPlanMetadata)))
	require.Zero(t, provider.count(string(PhaseKeyScope)))
	require.Zero(t, provider.count(string(PhaseKeyStaffing)))
}

func TestMeetingExtractionOrchestratorRateLimitWaitsAndResumes(t *testing.T) {
	now := time.Unix(100, 0)
	retryAt := now.Add(10 * time.Second)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{
		string(PhaseKeyPlanMetadata): {{err: &ExtractionProviderError{Code: ErrExtractionProviderRateLimited, Retryable: true, RetryAfter: retryAt}}, {text: `{}`}}, string(PhaseKeyScope): {{text: `{}`}}, string(PhaseKeyStaffing): {{text: `{}`}}, string(PhaseKeyServiceDetails): {{text: `{}`}},
	}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	orchestrator := newOrchestratorTest(t, provider, store, &now)
	in, _ := newOrchestratorInput(t, "scope")
	result, err := orchestrator.Run(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunWaitingRateLimit, result.Status)
	require.Equal(t, retryAt, result.WaitingUntil)
	require.Equal(t, 1, provider.count(string(PhaseKeyPlanMetadata)))
	now = now.Add(9 * time.Second)
	result, err = orchestrator.Run(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunWaitingRateLimit, result.Status)
	require.Equal(t, 1, provider.count(string(PhaseKeyPlanMetadata)))
	now = retryAt
	result, err = orchestrator.Run(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunSucceeded, result.Status)
	require.Equal(t, 2, provider.count(string(PhaseKeyPlanMetadata)))
}

func TestMeetingExtractionOrchestratorExhaustedBudgetStopsBeforeRetry(t *testing.T) {
	now := time.Unix(100, 0)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{string(PhaseKeyPlanMetadata): {{err: &ExtractionProviderError{Code: ErrExtractionProviderTimeout, Retryable: true}}}}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 1}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	orchestrator := newOrchestratorTest(t, provider, store, &now)
	in, _ := newOrchestratorInput(t, "scope")
	_, err := orchestrator.Run(context.Background(), in)
	var orchestratorErr *MeetingExtractionOrchestratorError
	require.ErrorAs(t, err, &orchestratorErr)
	require.ErrorIs(t, err, ErrMeetingExtractionBudget)
	require.Equal(t, ErrExtractionBudgetExhausted, store.failed.ErrorCode)
	require.Equal(t, 1, provider.count(string(PhaseKeyPlanMetadata)))
}

func TestMeetingExtractionOrchestratorChangedHashMakesNoCall(t *testing.T) {
	now := time.Unix(100, 0)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	orchestrator := newOrchestratorTest(t, provider, store, &now)
	in, doc := newOrchestratorInput(t, "scope")
	in.ExpectedNoteHash = strings.Repeat("0", len(doc.Hash()))
	_, err := orchestrator.Run(context.Background(), in)
	require.ErrorIs(t, err, ErrMeetingExtractionNotesChanged)
	require.Zero(t, provider.count(string(PhaseKeyPlanMetadata)))
}

func TestMeetingExtractionOrchestratorFingerprintSkipUsesPersistedResults(t *testing.T) {
	now := time.Unix(100, 0)
	firstProvider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{string(PhaseKeyPlanMetadata): {{text: `{}`}}, string(PhaseKeyScope): {{text: `{}`}}, string(PhaseKeyStaffing): {{text: `{}`}}, string(PhaseKeyServiceDetails): {{text: `{}`}}}}
	firstStore := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}}
	first := newOrchestratorTest(t, firstProvider, firstStore, &now)
	in, _ := newOrchestratorInput(t, "scope")
	_, err := first.Run(context.Background(), in)
	require.NoError(t, err)

	secondPhases := make(map[PhaseKey]MeetingExtractionPhaseCheckpoint, len(firstStore.phases))
	for key, checkpoint := range firstStore.phases {
		secondPhases[key] = checkpoint
	}
	secondStore := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: secondPhases}
	secondProvider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{}}
	second := newOrchestratorTest(t, secondProvider, secondStore, &now)
	result, err := second.Run(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunSucceeded, result.Status)
	require.Equal(t, 0, secondProvider.count(string(PhaseKeyPlanMetadata)))
	require.Equal(t, 0, secondProvider.count(string(PhaseKeyScope)))
	require.Equal(t, 0, secondProvider.count(string(PhaseKeyStaffing)))
	require.Equal(t, 0, secondProvider.count(string(PhaseKeyServiceDetails)))
}

func TestMeetingExtractionOrchestratorStopsOnStaleFence(t *testing.T) {
	now := time.Unix(100, 0)
	provider := &orchestratorFakeProvider{counts: map[string]int{}, responses: map[string][]orchestratorProviderResponse{string(PhaseKeyPlanMetadata): {{text: `{}`}}}}
	store := &orchestratorFakeStore{run: MeetingExtractionRunCheckpoint{Status: domain.ExtractionRunRunning, CallsMax: 8}, phases: map[PhaseKey]MeetingExtractionPhaseCheckpoint{}, staleOnCommit: true}
	orchestrator := newOrchestratorTest(t, provider, store, &now)
	in, _ := newOrchestratorInput(t, "scope")
	_, err := orchestrator.Run(context.Background(), in)
	require.ErrorIs(t, err, ErrMeetingExtractionStaleFence)
	require.Equal(t, 1, provider.count(string(PhaseKeyPlanMetadata)))
	require.Empty(t, store.phases)
}

func TestMeetingExtractionOrchestratorSafeErrorDoesNotExposeProviderDetail(t *testing.T) {
	sentinel := "secret-provider-body"
	safe := safeExtractionError(&ProviderError{Code: ErrRateLimited, Message: sentinel, Retryable: true})
	require.NotContains(t, safe.Error(), sentinel)
	require.True(t, errors.Is(&MeetingExtractionOrchestratorError{Code: ErrExtractionBudgetExhausted}, ErrMeetingExtractionBudget))
}
