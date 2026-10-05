//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/modules/quotation/testutil"
	appLogger "venturo-skeleton-go/pkg/logger"
)

type e2eProvider struct {
	mu        sync.Mutex
	responses map[string][]e2eResponse
	counts    map[string]int
}

type e2eResponse struct {
	text string
	err  error
}

func (p *e2eProvider) ExtractionCapabilities() service.ProviderCapabilities {
	return service.ProviderCapabilities{JSONSchema: true}
}

func (p *e2eProvider) GenerateExtraction(_ context.Context, request service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := request.Prompt
	p.counts[key]++
	items := p.responses[key]
	index := p.counts[key] - 1
	if index >= len(items) {
		return service.ExtractionGenerateResult{Text: `{}`}, nil
	}
	return service.ExtractionGenerateResult{Text: items[index].text}, items[index].err
}

func (p *e2eProvider) count(key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.counts[key]
}

func (p *e2eProvider) record(key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.counts[key]++
}

type e2ePhase struct {
	key     service.PhaseKey
	result  []byte
	onBuild func(service.PhaseInput)
}

func (p e2ePhase) Key() service.PhaseKey     { return p.key }
func (p e2ePhase) Owns() []service.FieldPath { return []service.FieldPath{service.FieldPath(p.key)} }
func (p e2ePhase) BuildPrompt(_ context.Context, input service.PhaseInput) (service.ProviderRequest, error) {
	// The fake records only this phase key, never the document or prompt body.
	if p.onBuild != nil {
		p.onBuild(input)
	}
	return service.ProviderRequest{Prompt: string(p.key), JSONSchema: json.RawMessage(`{}`), MaxOutputTokens: 32}, nil
}
func (p e2ePhase) Schema(service.PhaseInput) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (p e2ePhase) Validate(context.Context, service.PhaseInput, json.RawMessage) (service.PhaseResult, error) {
	return e2eResult{key: p.key, result: p.result}, nil
}

type e2eResult struct {
	key    service.PhaseKey
	result []byte
}

func (r e2eResult) PhaseKey() service.PhaseKey      { return r.key }
func (r e2eResult) SafeResultJSON() ([]byte, error) { return append([]byte(nil), r.result...), nil }
func (r e2eResult) EvidenceReferenceIDs() []string  { return nil }
func (r e2eResult) WarningCodes() []string          { return nil }

func TestMeetingExtractionE2EManualAndAutoDedicatedSquad(t *testing.T) {
	for _, manual := range []bool{true, false} {
		t.Run(map[bool]string{true: "manual", false: "auto"}[manual], func(t *testing.T) {
			provider, result := runE2E(t, domain.CommercialSchemeDedicatedSquad, manual, nil)
			require.NoError(t, result.err)
			require.Equal(t, domain.ExtractionRunSucceeded, result.status)
			for _, key := range []string{"plan_metadata", "scope", "staffing", "service_details"} {
				require.Equal(t, 1, provider.count(key))
			}
			require.Equal(t, 1, result.finalizeCalls)
		})
	}
}

func TestMeetingExtractionE2EP3TimeoutRetriesOnlyP3(t *testing.T) {
	provider, result := runE2E(t, domain.CommercialSchemeDedicatedSquad, false, map[string][]e2eResponse{
		"staffing": {{err: &service.ExtractionProviderError{Code: service.ErrExtractionProviderTimeout, Retryable: true}}, {text: `{}`}},
	})
	require.NoError(t, result.err)
	require.Equal(t, domain.ExtractionRunSucceeded, result.status)
	require.Equal(t, 1, provider.count("plan_metadata"))
	require.Equal(t, 1, provider.count("scope"))
	require.Equal(t, 2, provider.count("staffing"))
	require.Equal(t, 1, provider.count("service_details"))
	require.Equal(t, 1, result.finalizeCalls)
}

func TestMeetingExtractionE2EAutoUnmappedSchemeStopsAfterP1(t *testing.T) {
	provider, result := runE2E(t, "subscription_saas", false, nil)
	require.NoError(t, result.err)
	require.Equal(t, domain.ExtractionRunNeedsReview, result.status)
	require.Equal(t, "SCHEME_MAPPER_UNAVAILABLE", result.errorCode)
	require.Equal(t, 1, provider.count("plan_metadata"))
	require.Zero(t, provider.count("scope"))
	require.Zero(t, provider.count("staffing"))
	require.Zero(t, provider.count("service_details"))
	require.Zero(t, result.finalizeCalls)
}

type privacyCanaryPhase struct {
	key    service.PhaseKey
	result []byte
	prompt string
}

func (p privacyCanaryPhase) Key() service.PhaseKey { return p.key }
func (p privacyCanaryPhase) Owns() []service.FieldPath {
	return []service.FieldPath{service.FieldPath(p.key)}
}
func (p privacyCanaryPhase) BuildPrompt(context.Context, service.PhaseInput) (service.ProviderRequest, error) {
	return service.ProviderRequest{Prompt: p.prompt, JSONSchema: json.RawMessage(`{}`), MaxOutputTokens: 32}, nil
}
func (p privacyCanaryPhase) Schema(service.PhaseInput) (json.RawMessage, error) {
	return json.RawMessage(`{}`), nil
}
func (p privacyCanaryPhase) Validate(context.Context, service.PhaseInput, json.RawMessage) (service.PhaseResult, error) {
	return e2eResult{key: p.key, result: p.result}, nil
}

type privacyCanaryProvider struct {
	base    *e2eProvider
	rawBody string
}

func (p *privacyCanaryProvider) ExtractionCapabilities() service.ProviderCapabilities {
	return p.base.ExtractionCapabilities()
}
func (p *privacyCanaryProvider) GenerateExtraction(ctx context.Context, request service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error) {
	return p.base.GenerateExtraction(ctx, request)
}

func TestMeetingExtractionPrivacyCanaryZapObserver(t *testing.T) {
	const (
		notesCanary  = "CANARY_M8_NOTES_7f1d"
		promptCanary = "CANARY_M8_PROMPT_91ac"
		keyCanary    = "CANARY_M8_API_KEY_4b22"
		authCanary   = "CANARY_M8_AUTHORIZATION_0d7e"
		bodyCanary   = "CANARY_M8_RAW_PROVIDER_BODY_53e9"
	)
	core, logs := observer.New(zapcore.DebugLevel)
	previousLogger := appLogger.Log
	appLogger.Log = zap.New(core)
	defer func() { appLogger.Log = previousLogger }()

	pool := testutil.Pool(t)
	fixture := testutil.NewExtractionFixture(t, pool)
	repo := repository.NewMeetingExtractionRepository(pool)
	doc, err := service.NewMeetingNoteDocument(notesCanary, service.MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	fixture.Params.NoteHash = doc.Hash()
	created, err := repo.CreateOrReuseRun(context.Background(), fixture.Params)
	require.NoError(t, err)
	claimed, err := repo.ClaimNext(context.Background(), "privacy-canary-worker", time.Minute)
	require.NoError(t, err)
	store, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{
		DailyProviderCallsMax: 20, DailyQuotationCallsMax: 20,
	})
	require.NoError(t, err)

	base := &e2eProvider{responses: map[string][]e2eResponse{}, counts: map[string]int{}}
	provider := &privacyCanaryProvider{base: base, rawBody: bodyCanary}
	for _, key := range []string{"plan_metadata", "scope", "staffing", "service_details"} {
		base.responses[key] = []e2eResponse{{text: `{"provider_body":"` + bodyCanary + `"}`}}
	}
	canaryPrompt := strings.Join([]string{promptCanary, keyCanary, authCanary, bodyCanary}, " ")
	phases := service.MeetingExtractionPhaseSet{
		Plan:     privacyCanaryPhase{key: service.PhaseKeyPlanMetadata, result: []byte(`{"scheme":"dedicated_squad","confidence":0.95}`), prompt: canaryPrompt},
		Scope:    privacyCanaryPhase{key: service.PhaseKey("scope"), result: []byte(`{"scope":[]}`), prompt: canaryPrompt},
		Staffing: privacyCanaryPhase{key: service.PhaseKey("staffing"), result: []byte(`{"duration_months":null,"team":[]}`), prompt: canaryPrompt},
		Service: func(value string) service.Phase {
			return privacyCanaryPhase{key: service.PhaseKey("service_details"), result: []byte(`{"scheme":"` + value + `","facts":[]}`), prompt: canaryPrompt}
		},
	}
	orchestrator, err := service.NewMeetingExtractionOrchestrator(provider, store, phases, func(context.Context, service.MeetingExtractionP5Input) error { return nil }, service.MeetingExtractionOrchestratorConfig{})
	require.NoError(t, err)
	result, err := orchestrator.Run(context.Background(), service.MeetingExtractionOrchestratorInput{
		RunID: claimed.ID, Fence: claimed.Fence, Document: doc, ExpectedNoteHash: doc.Hash(),
		PipelineVersion: "meeting-extraction-v1", PromptVersion: "p1", SchemaVersion: "s1",
		CatalogSnapshotHash: "catalog", RegistryEntryVersion: "registry", ManualScheme: domain.CommercialSchemeDedicatedSquad,
		Provider: "fake-privacy", Model: "fake-privacy-" + created.Run.ID.String(), CompanyID: created.Run.CompanyID,
		QuotationID: created.Run.QuotationID, RevisionID: created.Run.RevisionID,
	})
	require.NoError(t, err)
	require.Equal(t, domain.ExtractionRunSucceeded, result.Status)

	for _, entry := range logs.All() {
		serialized := entry.Message + " " + fmt.Sprint(entry.Context)
		for _, canary := range []string{notesCanary, promptCanary, keyCanary, authCanary, bodyCanary} {
			require.NotContains(t, serialized, canary)
		}
	}
}

type e2eRunResult struct {
	status        domain.ExtractionRunStatus
	errorCode     string
	err           error
	finalizeCalls int
}

func runE2E(t *testing.T, scheme string, manual bool, overrides map[string][]e2eResponse) (*e2eProvider, e2eRunResult) {
	t.Helper()
	pool := testutil.Pool(t)
	fx := testutil.NewExtractionFixture(t, pool)
	repo := repository.NewMeetingExtractionRepository(pool)
	doc, err := service.NewMeetingNoteDocument("integration note", service.MeetingNoteDocumentConfig{})
	require.NoError(t, err)
	params := fx.Params
	params.NoteHash = doc.Hash()
	created, err := repo.CreateOrReuseRun(context.Background(), params)
	require.NoError(t, err)
	claimed, err := repo.ClaimNext(context.Background(), "e2e-worker", time.Minute)
	require.NoError(t, err)
	store, err := service.NewPostgreSQLMeetingExtractionOrchestratorStore(repo, service.MeetingExtractionStoreOptions{DailyProviderCallsMax: 8, DailyQuotationCallsMax: 8})
	require.NoError(t, err)

	responses := map[string][]e2eResponse{}
	for _, key := range []string{"plan_metadata", "scope", "staffing", "service_details"} {
		responses[key] = []e2eResponse{{text: `{}`}}
	}
	for key, values := range overrides {
		responses[key] = values
	}
	provider := &e2eProvider{responses: responses, counts: map[string]int{}}
	phases := service.MeetingExtractionPhaseSet{
		Plan:     e2ePhase{key: service.PhaseKeyPlanMetadata, result: []byte(`{"scheme":"` + scheme + `","confidence":0.95}`)},
		Scope:    e2ePhase{key: service.PhaseKey("scope"), result: []byte(`{"scope":[]}`)},
		Staffing: e2ePhase{key: service.PhaseKey("staffing"), result: []byte(`{"duration_months":null,"team":[]}`)},
		Service: func(value string) service.Phase {
			return e2ePhase{key: service.PhaseKey("service_details"), result: []byte(`{"scheme":"` + value + `","facts":[]}`)}
		},
	}
	finalizeCalls := 0
	orchestrator, err := service.NewMeetingExtractionOrchestrator(provider, store, phases, func(context.Context, service.MeetingExtractionP5Input) error {
		finalizeCalls++
		return nil
	}, service.MeetingExtractionOrchestratorConfig{})
	require.NoError(t, err)
	manualScheme := ""
	if manual {
		manualScheme = domain.CommercialSchemeDedicatedSquad
	}
	got, runErr := orchestrator.Run(context.Background(), service.MeetingExtractionOrchestratorInput{
		RunID: claimed.ID, Fence: claimed.Fence, Document: doc, ExpectedNoteHash: doc.Hash(),
		PipelineVersion: "meeting-extraction-v1", PromptVersion: "p1", SchemaVersion: "s1",
		CatalogSnapshotHash: "catalog", RegistryEntryVersion: "registry", ManualScheme: manualScheme,
		Provider: "fake", Model: "fake-" + created.Run.ID.String(), CompanyID: created.Run.CompanyID, QuotationID: created.Run.QuotationID, RevisionID: created.Run.RevisionID,
	})
	return provider, e2eRunResult{status: got.Status, errorCode: got.ErrorCode, err: runErr, finalizeCalls: finalizeCalls}
}
