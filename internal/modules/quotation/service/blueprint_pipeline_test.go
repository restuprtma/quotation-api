package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakePipelineRepo struct {
	claimNextCalls int
	claimNextFn    func(ctx context.Context, companyID, blueprintID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error)

	claimFailedCalls int
	claimFailedFn    func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error)

	completeCalls  int
	completeParams []repository.CompleteBlueprintSectionParams
	completeFn     func(ctx context.Context, params repository.CompleteBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error)

	failCalls  int
	failParams []repository.FailBlueprintSectionParams
	failFn     func(ctx context.Context, params repository.FailBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error)

	loadManualCalls int
	loadManualFn    func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error)

	completeManualCalls  int
	completeManualParams []repository.CompleteBlueprintManualParams
	completeManualFn     func(ctx context.Context, params repository.CompleteBlueprintManualParams) (*repository.BlueprintSectionPersistenceResult, error)
}

func (f *fakePipelineRepo) ClaimNextBlueprintSection(ctx context.Context, companyID, blueprintID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
	f.claimNextCalls++
	if f.claimNextFn != nil {
		return f.claimNextFn(ctx, companyID, blueprintID, now, staleAfter)
	}
	return nil, repository.ErrBlueprintNoEligibleSection
}

func (f *fakePipelineRepo) ClaimFailedBlueprintSection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
	f.claimFailedCalls++
	if f.claimFailedFn != nil {
		return f.claimFailedFn(ctx, companyID, blueprintID, sectionType, now, staleAfter)
	}
	return nil, repository.ErrBlueprintSectionNotRetryable
}

func (f *fakePipelineRepo) CompleteBlueprintSection(ctx context.Context, params repository.CompleteBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error) {
	f.completeCalls++
	f.completeParams = append(f.completeParams, params)
	if f.completeFn != nil {
		return f.completeFn(ctx, params)
	}
	genStatus := domain.BlueprintGenerationInProgress
	version := params.Claim.ExpectedContentVersion + 1
	if params.Final {
		genStatus = domain.BlueprintGenerationCompleted
		version = params.Claim.ExpectedContentVersion
	}
	return &repository.BlueprintSectionPersistenceResult{
		RevisionID:       params.Claim.RevisionID,
		SectionType:      params.Claim.SectionType,
		SectionStatus:    domain.BlueprintSectionCompleted,
		GenerationStatus: genStatus,
		ContentVersion:   version,
		Attempt:          params.Claim.Attempt,
	}, nil
}

func (f *fakePipelineRepo) FailBlueprintSection(ctx context.Context, params repository.FailBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error) {
	f.failCalls++
	f.failParams = append(f.failParams, params)
	if f.failFn != nil {
		return f.failFn(ctx, params)
	}
	genStatus := domain.BlueprintGenerationPartialFailed
	if params.Terminal {
		genStatus = domain.BlueprintGenerationFailed
	}
	return &repository.BlueprintSectionPersistenceResult{
		RevisionID:       params.Claim.RevisionID,
		SectionType:      params.Claim.SectionType,
		SectionStatus:    domain.BlueprintSectionFailed,
		GenerationStatus: genStatus,
		ContentVersion:   params.Claim.ExpectedContentVersion,
		Attempt:          params.Claim.Attempt,
	}, nil
}

func (f *fakePipelineRepo) LoadBlueprintManualSnapshot(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
	f.loadManualCalls++
	if f.loadManualFn != nil {
		return f.loadManualFn(ctx, companyID, blueprintID, sectionType)
	}
	return nil, repository.ErrBlueprintNotFound
}

func (f *fakePipelineRepo) CompleteBlueprintSectionManually(ctx context.Context, params repository.CompleteBlueprintManualParams) (*repository.BlueprintSectionPersistenceResult, error) {
	f.completeManualCalls++
	f.completeManualParams = append(f.completeManualParams, params)
	if f.completeManualFn != nil {
		return f.completeManualFn(ctx, params)
	}
	return &repository.BlueprintSectionPersistenceResult{
		RevisionID:       params.Snapshot.RevisionID,
		SectionType:      params.Snapshot.SectionType,
		SectionStatus:    domain.BlueprintSectionValidated,
		GenerationStatus: domain.BlueprintGenerationInProgress,
		ContentVersion:   params.Snapshot.ExpectedContentVersion + 1,
		Attempt:          1,
	}, nil
}

type fakeProviderSource struct {
	calls      int
	companyIDs []uuid.UUID
	providerFn func(ctx context.Context, companyID uuid.UUID) (AIProvider, string, string, error)
}

func (f *fakeProviderSource) ProviderForCompany(ctx context.Context, companyID uuid.UUID) (AIProvider, string, string, error) {
	f.calls++
	f.companyIDs = append(f.companyIDs, companyID)
	if f.providerFn != nil {
		return f.providerFn(ctx, companyID)
	}
	return &fakeAIProvider{}, "fake-provider", "fake-model", nil
}

type fakeAIProvider struct {
	calls      int
	requests   []GenerateRequest
	generateFn func(ctx context.Context, req GenerateRequest) (GenerateResult, error)
}

func (f *fakeAIProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
	f.calls++
	f.requests = append(f.requests, req)
	if f.generateFn != nil {
		return f.generateFn(ctx, req)
	}
	return GenerateResult{Text: validJourneyPayload()}, nil
}

type fakePromptBuilder struct {
	calls   int
	inputs  []BlueprintPromptInput
	buildFn func(input BlueprintPromptInput) (string, error)
}

func (f *fakePromptBuilder) Build(input BlueprintPromptInput) (string, error) {
	f.calls++
	f.inputs = append(f.inputs, input)
	if f.buildFn != nil {
		return f.buildFn(input)
	}
	return "prompt instructions for " + string(input.Stage), nil
}

type fixedClock struct {
	t time.Time
}

func (c fixedClock) Now() time.Time {
	return c.t
}

func validJourneyPayload() string {
	return `{
		"journey": {
			"phases": [{"id": "phase-1", "label": "Discovery", "order": 1}],
			"actors": [{"id": "actor-1", "label": "Client", "order": 1}],
			"activities": [{"id": "act-1", "phase_id": "phase-1", "actor_id": "actor-1", "label": "Initial Meeting", "description": "Discuss project scope"}]
		}
	}`
}

func makeBaseClaim(companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) *repository.BlueprintSectionClaim {
	return &repository.BlueprintSectionClaim{
		BlueprintID:            blueprintID,
		RevisionID:             uuid.New(),
		SectionRunID:           uuid.New(),
		CompanyID:              companyID,
		SectionType:            sectionType,
		Attempt:                1,
		ExpectedContentVersion: 1,
		BlueprintTitle:         "Test Blueprint",
		SourceContextSnapshot:  json.RawMessage(`{"quotation_id":"q-123","summary":"Banking Application"}`),
		SkillSnapshot:          json.RawMessage(`{"preset":"enterprise","tone":"formal"}`),
		RevisionContent:        json.RawMessage(`{}`),
		ContextContent:         json.RawMessage(`{"title":"Banking Application","summary":"Enterprise Core"}`),
		Dependencies: map[domain.BlueprintSectionType]json.RawMessage{
			domain.BlueprintSectionContext: json.RawMessage(`{"title":"Banking Application"}`),
		},
	}
}

func TestBlueprintPipelineContinueExecutesExactlyOneProviderStage(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
		},
	}
	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: validJourneyPayload()}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}
	prompts := &fakePromptBuilder{}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	pipeline := NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)

	res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, domain.BlueprintSectionJourney, res.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, res.SectionStatus)
	assert.Equal(t, 1, res.Attempt)
	assert.Equal(t, 2, res.ContentVersion)

	assert.Equal(t, 1, repo.claimNextCalls, "exactly one claim must be made")
	assert.Equal(t, 1, provSource.calls, "provider source must be resolved once")
	assert.Equal(t, 1, prov.calls, "exactly one generate call must be made")
	assert.Equal(t, 1, repo.completeCalls, "exactly one complete call must be made")
	assert.Equal(t, 0, repo.failCalls, "zero fail calls expected")
}

func TestBlueprintPipelineContinueResolvesProviderPerAction(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
		},
	}

	p1 := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: validJourneyPayload()}, nil
		},
	}
	p2 := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: validJourneyPayload()}, nil
		},
	}

	var instances []AIProvider
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			if len(instances) == 0 {
				instances = append(instances, p1)
				return p1, "provider-1", "model-1", nil
			}
			instances = append(instances, p2)
			return p2, "provider-2", "model-2", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	_, err1 := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.NoError(t, err1)

	_, err2 := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.NoError(t, err2)

	assert.Equal(t, 2, provSource.calls, "ProviderForCompany must be called once per Continue action")
	require.Len(t, instances, 2)
	assert.NotSame(t, instances[0], instances[1], "provider instances must be distinct and not reused/cached")
	assert.Equal(t, 1, p1.calls)
	assert.Equal(t, 1, p2.calls)
}

func TestBlueprintPipelineContinueNeverTrustsExternalCompanyOrStatus(t *testing.T) {
	repo := &fakePipelineRepo{}
	provSource := &fakeProviderSource{}
	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	// Nil company ID
	_, err := pipeline.Continue(context.Background(), uuid.Nil, uuid.New())
	assert.ErrorIs(t, err, ErrBlueprintPipelineInvalidInput)
	assert.Equal(t, 0, repo.claimNextCalls)

	// Nil blueprint ID
	_, err = pipeline.Continue(context.Background(), uuid.New(), uuid.Nil)
	assert.ErrorIs(t, err, ErrBlueprintPipelineInvalidInput)
	assert.Equal(t, 0, repo.claimNextCalls)

	// Both nil
	_, err = pipeline.Continue(context.Background(), uuid.Nil, uuid.Nil)
	assert.ErrorIs(t, err, ErrBlueprintPipelineInvalidInput)
	assert.Equal(t, 0, repo.claimNextCalls)

	// Nil pipeline instance
	var nilPipeline *BlueprintPipeline
	_, err = nilPipeline.Continue(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrBlueprintPipelineInvalidInput)

	// Pipeline with missing dependencies
	incompletePipeline := &BlueprintPipeline{}
	_, err = incompletePipeline.Continue(context.Background(), uuid.New(), uuid.New())
	assert.ErrorIs(t, err, ErrBlueprintPipelineInvalidInput)
}

func TestBlueprintPipelineContinueMapsUnknownAndForeignToNotFound(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	// Case 1: Unknown blueprint ID
	repo1 := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return nil, repository.ErrBlueprintNotFound
		},
	}
	pipeline1 := NewBlueprintPipeline(repo1, &fakeProviderSource{}, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	_, err1 := pipeline1.Continue(context.Background(), companyID, blueprintID)
	assert.True(t, errors.Is(err1, ErrBlueprintPipelineNotFound))

	// Case 2: Foreign company ID
	foreignCompanyID := uuid.New()
	repo2 := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return nil, repository.ErrBlueprintNotFound
		},
	}
	pipeline2 := NewBlueprintPipeline(repo2, &fakeProviderSource{}, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	_, err2 := pipeline2.Continue(context.Background(), foreignCompanyID, blueprintID)
	assert.True(t, errors.Is(err2, ErrBlueprintPipelineNotFound))
}

func TestBlueprintPipelineContinueUsesPersistedSnapshotsAndDependencies(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	expectedSkill := json.RawMessage(`{"preset":"fintech","rules":["strict_id"]}`)
	expectedContext := json.RawMessage(`{"quotation_id":"q-custom","summary":"Custom Fintech Spec"}`)
	expectedDeps := map[domain.BlueprintSectionType]json.RawMessage{
		domain.BlueprintSectionContext: json.RawMessage(`{"title":"Fintech Spec"}`),
	}

	claim := makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney)
	claim.SkillSnapshot = expectedSkill
	claim.SourceContextSnapshot = expectedContext
	claim.Dependencies = expectedDeps

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return claim, nil
		},
	}
	prompts := &fakePromptBuilder{}
	pipeline := NewBlueprintPipeline(repo, &fakeProviderSource{}, prompts, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.NoError(t, err)

	require.Len(t, prompts.inputs, 1)
	input := prompts.inputs[0]

	assert.Equal(t, domain.BlueprintSectionJourney, input.Stage)
	assert.JSONEq(t, string(expectedSkill), string(input.SkillSnapshot))
	assert.JSONEq(t, string(expectedContext), string(input.Context))
	require.Contains(t, input.Dependencies, domain.BlueprintSectionContext)
	assert.JSONEq(t, string(expectedDeps[domain.BlueprintSectionContext]), string(input.Dependencies[domain.BlueprintSectionContext]))
}

func TestBlueprintPipelineContinueTimesOutOneProviderAttempt(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	t.Run("provider timeout", func(t *testing.T) {
		repo := &fakePipelineRepo{
			claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
				return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
			},
		}
		prov := &fakeAIProvider{
			generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
				select {
				case <-ctx.Done():
					return GenerateResult{}, ctx.Err()
				case <-time.After(1 * time.Second):
					return GenerateResult{Text: validJourneyPayload()}, nil
				}
			},
		}
		provSource := &fakeProviderSource{
			providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
				return prov, "openai", "gpt-4o", nil
			},
		}

		pipeline := NewBlueprintPipeline(
			repo,
			provSource,
			&fakePromptBuilder{},
			NewBlueprintSectionValidator(),
			NewProductionBlueprintCrossValidator(),
			WithBlueprintProviderTimeout(20*time.Millisecond),
		)

		_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
		assert.ErrorIs(t, err, ErrBlueprintProviderTimeout)

		assert.Equal(t, 1, repo.failCalls, "failure must be persisted")
		require.Len(t, repo.failParams, 1)
		assert.Equal(t, "provider_timeout", repo.failParams[0].SafeErrorCode)
		assert.False(t, repo.failParams[0].Terminal, "timeout on attempt 1 is retryable, not terminal")
		assert.Nil(t, repo.failParams[0].SectionContent, "failed section content must be nil")
		assert.Equal(t, 0, repo.completeCalls, "complete must not be called on timeout")
	})

	t.Run("parent cancellation preserves terminal write context without leaking raw cancel error", func(t *testing.T) {
		repoCancel := &fakePipelineRepo{
			claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
				return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
			},
			failFn: func(ctx context.Context, params repository.FailBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error) {
				assert.NoError(t, ctx.Err(), "terminal write context must remain live despite parent cancellation")
				return &repository.BlueprintSectionPersistenceResult{
					RevisionID:       params.Claim.RevisionID,
					SectionType:      params.Claim.SectionType,
					SectionStatus:    domain.BlueprintSectionFailed,
					GenerationStatus: domain.BlueprintGenerationPartialFailed,
					ContentVersion:   params.Claim.ExpectedContentVersion,
					Attempt:          params.Claim.Attempt,
				}, nil
			},
		}
		prov := &fakeAIProvider{
			generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
				return GenerateResult{}, ctx.Err()
			},
		}
		provSource := &fakeProviderSource{
			providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
				return prov, "openai", "gpt-4o", nil
			},
		}

		canceledCtx, cancel := context.WithCancel(context.Background())
		cancel() // cancel parent context immediately

		pipelineCancel := NewBlueprintPipeline(
			repoCancel,
			provSource,
			&fakePromptBuilder{},
			NewBlueprintSectionValidator(),
			NewProductionBlueprintCrossValidator(),
		)

		_, cancelErr := pipelineCancel.Continue(canceledCtx, companyID, blueprintID)
		assert.ErrorIs(t, cancelErr, ErrBlueprintProviderUnavailable)
		assert.False(t, strings.Contains(cancelErr.Error(), "context canceled"), "raw cancellation error text must not leak")
		assert.Equal(t, 1, repoCancel.failCalls, "failure persistence must have completed")
	})
}

func TestBlueprintPipelineContinueRejectsNonPureJSONWithoutRepair(t *testing.T) {
	testCases := []struct {
		name        string
		rawResponse string
	}{
		{
			name:        "markdown code fence",
			rawResponse: "```json\n" + validJourneyPayload() + "\n```",
		},
		{
			name:        "prose before json",
			rawResponse: "Here is the valid journey JSON you requested:\n" + validJourneyPayload(),
		},
		{
			name:        "trailing json object",
			rawResponse: validJourneyPayload() + "\n{\"extra\":\"unwanted_second_object\"}",
		},
		{
			name:        "malformed syntax",
			rawResponse: "{\"schema_version\": \"1.0\", \"phases\": [",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			companyID := uuid.New()
			blueprintID := uuid.New()

			repo := &fakePipelineRepo{
				claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
					return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
				},
			}
			prov := &fakeAIProvider{
				generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
					return GenerateResult{Text: tc.rawResponse}, nil
				},
			}
			provSource := &fakeProviderSource{
				providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
					return prov, "openai", "gpt-4o", nil
				},
			}

			pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

			_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
			assert.ErrorIs(t, err, ErrBlueprintInvalidSchema, "must fail strictly with invalid_schema without repairing")

			assert.Equal(t, 1, repo.failCalls)
			require.Len(t, repo.failParams, 1)
			assert.Equal(t, "invalid_schema", repo.failParams[0].SafeErrorCode)
			assert.False(t, repo.failParams[0].Terminal)
			assert.Equal(t, 0, repo.completeCalls)
		})
	}
}

func TestBlueprintPipelineContinuePersistsSafeFailureWithoutRawOutput(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	sensitiveMarker := "SECRET_KEY_abc123_DO_NOT_EXPOSE"
	sensitiveResponse := `{"schema_version":"1.0","invalid_token":"` + sensitiveMarker + `"}`

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
		},
	}
	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: sensitiveResponse}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.Error(t, err)

	// Check returned error
	assert.False(t, strings.Contains(err.Error(), sensitiveMarker), "error must not leak raw output or secret marker")

	// Check persisted failure parameters
	require.Len(t, repo.failParams, 1)
	param := repo.failParams[0]
	assert.False(t, strings.Contains(param.SafeErrorCode, sensitiveMarker))
	assert.Nil(t, param.SectionContent, "section content must be nil on failure")

	if len(param.SectionValidation) > 0 {
		assert.False(t, strings.Contains(string(param.SectionValidation), sensitiveMarker), "section validation must not contain raw response text")
	}
}

func TestBlueprintPipelineContinueDoesNotRetrySecurityFailure(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	// Content with forbidden keys triggering security_failure in BlueprintSectionValidator
	forbiddenPayload := `{
		"schema_version": "1.0",
		"apiKey": "sk-secret-leak",
		"phases": [{"id": "phase-1", "label": "Discovery", "order": 1}],
		"actors": [{"id": "actor-1", "label": "Client", "order": 1}],
		"activities": [{"id": "act-1", "phase_id": "phase-1", "actor_id": "actor-1", "label": "Meeting", "description": "Scope"}]
	}`

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
		},
	}
	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: forbiddenPayload}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	assert.ErrorIs(t, err, ErrBlueprintSecurityFailure)

	assert.Equal(t, 1, prov.calls, "security failure must not be retried internally")
	assert.Equal(t, 1, repo.failCalls)
	require.Len(t, repo.failParams, 1)
	assert.Equal(t, "security_failure", repo.failParams[0].SafeErrorCode)
	assert.True(t, repo.failParams[0].Terminal, "security failure must be marked terminal")
}

func TestBlueprintPipelineContinueRejectsStaleTerminalWrite(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	t.Run("complete claim conflict", func(t *testing.T) {
		repo := &fakePipelineRepo{
			claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
				return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
			},
			completeFn: func(ctx context.Context, params repository.CompleteBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error) {
				return nil, repository.ErrBlueprintClaimConflict
			},
		}
		prov := &fakeAIProvider{
			generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
				return GenerateResult{Text: validJourneyPayload()}, nil
			},
		}
		provSource := &fakeProviderSource{
			providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
				return prov, "openai", "gpt-4o", nil
			},
		}

		pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
		_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
		assert.ErrorIs(t, err, ErrBlueprintExecutionConflict)
	})

	t.Run("complete version conflict", func(t *testing.T) {
		repo := &fakePipelineRepo{
			claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
				return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
			},
			completeFn: func(ctx context.Context, params repository.CompleteBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error) {
				return nil, repository.ErrBlueprintContentConflict
			},
		}
		prov := &fakeAIProvider{
			generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
				return GenerateResult{Text: validJourneyPayload()}, nil
			},
		}
		provSource := &fakeProviderSource{
			providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
				return prov, "openai", "gpt-4o", nil
			},
		}

		pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
		_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
		assert.ErrorIs(t, err, ErrBlueprintExecutionConflict)
	})

	t.Run("fail claim conflict", func(t *testing.T) {
		repo := &fakePipelineRepo{
			claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
				return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
			},
			failFn: func(ctx context.Context, params repository.FailBlueprintSectionParams) (*repository.BlueprintSectionPersistenceResult, error) {
				return nil, repository.ErrBlueprintClaimConflict
			},
		}
		prov := &fakeAIProvider{
			generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
				return GenerateResult{Text: "invalid json"}, nil
			},
		}
		provSource := &fakeProviderSource{
			providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
				return prov, "openai", "gpt-4o", nil
			},
		}

		pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
		_, err := pipeline.Continue(context.Background(), companyID, blueprintID)
		assert.ErrorIs(t, err, ErrBlueprintExecutionConflict)
	})
}

func validArchitecturePayload() string {
	return `{
		"architecture": {
			"groups": [{"id": "grp-1", "label": "Group 1", "order": 1}],
			"nodes": [{"id": "node-1", "group_id": "grp-1", "type": "service", "label": "Service 1", "description": "Desc", "layout": {"x": 100.5, "y": 200.5}}],
			"edges": [{"id": "edge-1", "source": "node-1", "target": "node-1", "label": "Loopback", "type": "sync"}]
		}
	}`
}

func makeBaseManualSnapshot(companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) *repository.BlueprintManualSnapshot {
	return &repository.BlueprintManualSnapshot{
		BlueprintID:            blueprintID,
		RevisionID:             uuid.New(),
		SectionRunID:           uuid.New(),
		CompanyID:              companyID,
		SectionType:            sectionType,
		SectionStatus:          domain.BlueprintSectionFailed,
		ExpectedContentVersion: 1,
		BlueprintTitle:         "Test Blueprint",
		RevisionContent:        json.RawMessage(`{}`),
		ContextContent:         json.RawMessage(`{"title":"Test Blueprint","summary":"Enterprise Core"}`),
	}
}

func TestBlueprintPipelineRetrySectionClaimsOnlyNamedFailedSection(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	var claimedSection domain.BlueprintSectionType
	repo := &fakePipelineRepo{
		claimFailedFn: func(ctx context.Context, cID, bID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			claimedSection = sectionType
			return makeBaseClaim(companyID, blueprintID, sectionType), nil
		},
	}
	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: validArchitecturePayload()}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	res, err := pipeline.RetrySection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, domain.BlueprintSectionArchitecture, claimedSection)
	assert.Equal(t, domain.BlueprintSectionArchitecture, res.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, res.SectionStatus)
	assert.Equal(t, 1, repo.claimFailedCalls)
	assert.Equal(t, 1, prov.calls)
	assert.Equal(t, 1, repo.completeCalls)
}

func TestBlueprintPipelineRetrySectionPreservesCompletedUpstream(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	journeyBundle := json.RawMessage(`{
		"schema_version": "1.0",
		"title": "Test Blueprint",
		"summary": "Enterprise Core",
		"journey": {
			"phases": [{"id": "phase-1", "label": "Discovery", "order": 1}],
			"actors": [{"id": "actor-1", "label": "Client", "order": 1}],
			"activities": [{"id": "act-1", "phase_id": "phase-1", "actor_id": "actor-1", "label": "Initial Meeting", "description": "Discuss project scope"}]
		},
		"assumptions": [],
		"open_questions": []
	}`)

	claim := makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionArchitecture)
	claim.RevisionContent = journeyBundle

	repo := &fakePipelineRepo{
		claimFailedFn: func(ctx context.Context, cID, bID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return claim, nil
		},
	}
	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: validArchitecturePayload()}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	_, err := pipeline.RetrySection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture)
	require.NoError(t, err)

	require.Len(t, repo.completeParams, 1)
	var rawMap map[string]any
	err = json.Unmarshal(repo.completeParams[0].RevisionContent, &rawMap)
	require.NoError(t, err)

	assert.NotNil(t, rawMap["journey"], "upstream journey must be preserved")
	assert.NotNil(t, rawMap["architecture"], "retried architecture must be added")
}

func TestBlueprintPipelineRetrySectionReplacesContentOnlyAfterValidResponse(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	claim := makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionArchitecture)
	repo := &fakePipelineRepo{
		claimFailedFn: func(ctx context.Context, cID, bID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return claim, nil
		},
	}
	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: `{"architecture":{"groups":[],"nodes":[{"id":"","group_id":"g"}],"edges":[]}}`}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	_, err := pipeline.RetrySection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture)
	assert.ErrorIs(t, err, ErrBlueprintInvalidSchema)

	assert.Equal(t, 0, repo.completeCalls, "must not call complete on invalid response")
	assert.Equal(t, 1, repo.failCalls, "must persist safe failure on invalid response")
}

func TestBlueprintPipelineRetrySectionEnforcesThreeAttemptLimit(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{
		claimFailedFn: func(ctx context.Context, cID, bID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return nil, repository.ErrBlueprintAttemptLimit
		},
	}
	prov := &fakeAIProvider{}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	_, err := pipeline.RetrySection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture)
	assert.ErrorIs(t, err, ErrBlueprintAttemptLimitReached)
	assert.Equal(t, 0, prov.calls, "provider must not be called when attempt limit reached")
}

func TestBlueprintPipelineRetrySectionRejectsSecurityFailure(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{
		claimFailedFn: func(ctx context.Context, cID, bID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return nil, repository.ErrBlueprintSectionNotRetryable
		},
	}
	prov := &fakeAIProvider{}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	_, err := pipeline.RetrySection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture)
	assert.ErrorIs(t, err, ErrBlueprintRetryNotAllowed)
	assert.Equal(t, 0, prov.calls, "provider must not be called for non-retryable failure")
}

func TestBlueprintPipelineManualCompleteSectionNeverCallsProvider(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)
	repo := &fakePipelineRepo{
		loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
			return snapshot, nil
		},
	}
	prov := &fakeAIProvider{}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	res, err := pipeline.ManualCompleteSection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture, json.RawMessage(validArchitecturePayload()))
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, domain.BlueprintSectionValidated, res.SectionStatus)
	assert.Equal(t, 0, provSource.calls, "manual completion must never resolve provider source")
	assert.Equal(t, 0, prov.calls, "manual completion must never call provider")
	assert.Equal(t, 1, repo.completeManualCalls)
}

func TestBlueprintPipelineManualCompleteSectionRejectsInvalidPayloadWithoutMutation(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)
	repo := &fakePipelineRepo{
		loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
			return snapshot, nil
		},
	}
	prov := &fakeAIProvider{}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	invalidPayload := json.RawMessage(`{"architecture":{"groups":[],"nodes":[{"id":"","group_id":"g"}],"edges":[]}}`)
	_, err := pipeline.ManualCompleteSection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture, invalidPayload)
	assert.ErrorIs(t, err, ErrBlueprintInvalidSchema)
	assert.Equal(t, 0, repo.completeManualCalls, "must not mutate database on invalid manual payload")
}

func TestBlueprintPipelineManualCompleteSectionValidatesAndUnblocksNextStage(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)
	snapshot.ExpectedContentVersion = 3
	repo := &fakePipelineRepo{
		loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
			return snapshot, nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, &fakeProviderSource{}, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())
	res, err := pipeline.ManualCompleteSection(context.Background(), companyID, blueprintID, domain.BlueprintSectionArchitecture, json.RawMessage(validArchitecturePayload()))
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, domain.BlueprintSectionValidated, res.SectionStatus)
	assert.Equal(t, 4, res.ContentVersion)
	require.Len(t, repo.completeManualParams, 1)

	expectedInvalidated := []domain.BlueprintSectionType{
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}
	assert.Equal(t, expectedInvalidated, repo.completeManualParams[0].InvalidatedSections)
}

func TestBlueprintPipelineManualCompleteSectionRejectsContextAndCrossValidation(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{}
	pipeline := NewBlueprintPipeline(repo, &fakeProviderSource{}, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	_, err := pipeline.ManualCompleteSection(context.Background(), companyID, blueprintID, domain.BlueprintSectionContext, json.RawMessage(`{}`))
	assert.ErrorIs(t, err, ErrBlueprintManualNotAllowed)

	_, err = pipeline.ManualCompleteSection(context.Background(), companyID, blueprintID, domain.BlueprintSectionCrossValidation, json.RawMessage(`{}`))
	assert.ErrorIs(t, err, ErrBlueprintManualNotAllowed)

	assert.Equal(t, 0, repo.loadManualCalls)
	assert.Equal(t, 0, repo.completeManualCalls)
}

func TestBlueprintPipelineManualCompleteSectionAtVersion(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	baseRevID := uuid.New()
	baseVersion := 3

	testCases := []struct {
		name                      string
		revisionID                uuid.UUID
		expectedVersion           int
		payload                   json.RawMessage
		expectErr                 error
		expectLoadManualCalls     int
		expectCompleteManualCalls int
		expectProviderCalls       int
		checkResult               func(t *testing.T, res *BlueprintPipelineResult)
	}{
		{
			name:                      "revision_mismatch",
			revisionID:                uuid.New(), // different from snapshot.RevisionID
			expectedVersion:           baseVersion,
			payload:                   json.RawMessage(validArchitecturePayload()),
			expectErr:                 ErrBlueprintExecutionConflict,
			expectLoadManualCalls:     1,
			expectCompleteManualCalls: 0,
			expectProviderCalls:       0,
		},
		{
			name:                      "version_mismatch",
			revisionID:                baseRevID,
			expectedVersion:           baseVersion + 1, // mismatch with snapshot.ExpectedContentVersion
			payload:                   json.RawMessage(validArchitecturePayload()),
			expectErr:                 ErrBlueprintExecutionConflict,
			expectLoadManualCalls:     1,
			expectCompleteManualCalls: 0,
			expectProviderCalls:       0,
		},
		{
			name:                      "invalid_version_zero",
			revisionID:                baseRevID,
			expectedVersion:           0,
			payload:                   json.RawMessage(validArchitecturePayload()),
			expectErr:                 ErrBlueprintPipelineInvalidInput,
			expectLoadManualCalls:     0,
			expectCompleteManualCalls: 0,
			expectProviderCalls:       0,
		},
		{
			name:                      "invalid_version_negative",
			revisionID:                baseRevID,
			expectedVersion:           -1,
			payload:                   json.RawMessage(validArchitecturePayload()),
			expectErr:                 ErrBlueprintPipelineInvalidInput,
			expectLoadManualCalls:     0,
			expectCompleteManualCalls: 0,
			expectProviderCalls:       0,
		},
		{
			name:                      "invalid_version_nil_revision",
			revisionID:                uuid.Nil,
			expectedVersion:           baseVersion,
			payload:                   json.RawMessage(validArchitecturePayload()),
			expectErr:                 ErrBlueprintPipelineInvalidInput,
			expectLoadManualCalls:     0,
			expectCompleteManualCalls: 0,
			expectProviderCalls:       0,
		},
		{
			name:                      "matching_version",
			revisionID:                baseRevID,
			expectedVersion:           baseVersion,
			payload:                   json.RawMessage(validArchitecturePayload()),
			expectErr:                 nil,
			expectLoadManualCalls:     1,
			expectCompleteManualCalls: 1,
			expectProviderCalls:       0,
			checkResult: func(t *testing.T, res *BlueprintPipelineResult) {
				require.NotNil(t, res)
				assert.Equal(t, baseVersion+1, res.ContentVersion)
				assert.Equal(t, domain.BlueprintSectionValidated, res.SectionStatus)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)
			snapshot.RevisionID = baseRevID
			snapshot.ExpectedContentVersion = baseVersion

			repo := &fakePipelineRepo{
				loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
					return snapshot, nil
				},
			}
			prov := &fakeAIProvider{}
			provSource := &fakeProviderSource{
				providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
					return prov, "openai", "gpt-4o", nil
				},
			}
			pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

			res, err := pipeline.ManualCompleteSectionAtVersion(
				context.Background(),
				companyID,
				blueprintID,
				domain.BlueprintSectionArchitecture,
				tc.revisionID,
				tc.expectedVersion,
				tc.payload,
			)

			if tc.expectErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectErr), "expected %v, got %v", tc.expectErr, err)
			} else {
				require.NoError(t, err)
				if tc.checkResult != nil {
					tc.checkResult(t, res)
				}
			}

			assert.Equal(t, tc.expectLoadManualCalls, repo.loadManualCalls, "loadManualCalls mismatch")
			assert.Equal(t, tc.expectCompleteManualCalls, repo.completeManualCalls, "completeManualCalls mismatch")
			assert.Equal(t, tc.expectProviderCalls, provSource.calls, "provider calls mismatch")
		})
	}
}

func TestBlueprintPipelineManualCompleteSectionTypedValidationError(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	baseRevID := uuid.New()
	baseVersion := 1

	canarySecret := "canary-secret-super-sensitive-token-12345"

	t.Run("schema_issue_returns_typed_validation_error", func(t *testing.T) {
		snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)
		snapshot.RevisionID = baseRevID
		snapshot.ExpectedContentVersion = baseVersion

		repo := &fakePipelineRepo{
			loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
				return snapshot, nil
			},
		}
		provSource := &fakeProviderSource{}
		pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

		invalidSchemaPayload := json.RawMessage(`{"architecture":{"groups":[],"nodes":[{"id":"","group_id":"g"}],"edges":[]}}`)
		res, err := pipeline.ManualCompleteSectionAtVersion(
			context.Background(),
			companyID,
			blueprintID,
			domain.BlueprintSectionArchitecture,
			baseRevID,
			baseVersion,
			invalidSchemaPayload,
		)
		require.Error(t, err)
		assert.Nil(t, res)

		assert.True(t, errors.Is(err, ErrBlueprintInvalidSchema), "must match ErrBlueprintInvalidSchema")
		assert.False(t, errors.Is(err, ErrBlueprintSecurityFailure), "must not match ErrBlueprintSecurityFailure")

		var valErr *BlueprintManualValidationError
		require.True(t, errors.As(err, &valErr), "must unwrap as BlueprintManualValidationError")
		assert.Equal(t, BlueprintValidationInvalidSchema, valErr.SafeOutcome)
		assert.NotEmpty(t, valErr.Issues, "must have validation issues")
		assert.Equal(t, ErrBlueprintInvalidSchema, valErr.BaseErr)
		assert.Equal(t, ErrBlueprintInvalidSchema.Error(), valErr.Error())
		assert.Equal(t, 0, repo.completeManualCalls)
	})

	t.Run("security_issue_returns_typed_validation_error_without_leaks", func(t *testing.T) {
		snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)
		snapshot.RevisionID = baseRevID
		snapshot.ExpectedContentVersion = baseVersion

		repo := &fakePipelineRepo{
			loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
				return snapshot, nil
			},
		}
		provSource := &fakeProviderSource{}
		pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

		// Payload with forbidden key "credential" and canary secret value
		payloadWithSecret := json.RawMessage(`{
			"architecture": {
				"groups": [],
				"nodes": [],
				"edges": [],
				"credential": "` + canarySecret + `"
			}
		}`)

		res, err := pipeline.ManualCompleteSectionAtVersion(
			context.Background(),
			companyID,
			blueprintID,
			domain.BlueprintSectionArchitecture,
			baseRevID,
			baseVersion,
			payloadWithSecret,
		)
		require.Error(t, err)
		assert.Nil(t, res)

		assert.True(t, errors.Is(err, ErrBlueprintSecurityFailure), "must match ErrBlueprintSecurityFailure")
		assert.False(t, errors.Is(err, ErrBlueprintInvalidSchema), "must not match ErrBlueprintInvalidSchema")

		var valErr *BlueprintManualValidationError
		require.True(t, errors.As(err, &valErr), "must unwrap as BlueprintManualValidationError")
		assert.Equal(t, BlueprintValidationSecurityFailure, valErr.SafeOutcome)
		assert.NotEmpty(t, valErr.Issues, "must have validation issues")
		assert.Equal(t, ErrBlueprintSecurityFailure, valErr.BaseErr)

		// Assert canary secret marker is NOT in the error message or issues
		errStr := valErr.Error()
		assert.NotContains(t, errStr, canarySecret, "error string must not leak canary secret")
		for _, issue := range valErr.Issues {
			assert.NotContains(t, issue.Message, canarySecret, "issue message must not leak canary secret")
			assert.NotContains(t, issue.Remediation, canarySecret, "issue remediation must not leak canary secret")
		}

		assert.Equal(t, 0, repo.completeManualCalls)
	})

	t.Run("unversioned_manual_complete_section_also_returns_typed_validation_error", func(t *testing.T) {
		snapshot := makeBaseManualSnapshot(companyID, blueprintID, domain.BlueprintSectionArchitecture)

		repo := &fakePipelineRepo{
			loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
				return snapshot, nil
			},
		}
		pipeline := NewBlueprintPipeline(repo, &fakeProviderSource{}, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

		invalidSchemaPayload := json.RawMessage(`{"architecture":{"groups":[],"nodes":[{"id":"","group_id":"g"}],"edges":[]}}`)
		_, err := pipeline.ManualCompleteSection(
			context.Background(),
			companyID,
			blueprintID,
			domain.BlueprintSectionArchitecture,
			invalidSchemaPayload,
		)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrBlueprintInvalidSchema))

		var valErr *BlueprintManualValidationError
		require.True(t, errors.As(err, &valErr))
		assert.Equal(t, BlueprintValidationInvalidSchema, valErr.SafeOutcome)
	})
}

func TestBlueprintPipelineConcurrentManualContinueRetryHasSingleWinner(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	var mu sync.Mutex
	claimed := false

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			mu.Lock()
			defer mu.Unlock()
			if claimed {
				return nil, repository.ErrBlueprintClaimBusy
			}
			claimed = true
			return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
		},
		claimFailedFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			mu.Lock()
			defer mu.Unlock()
			if claimed {
				return nil, repository.ErrBlueprintClaimBusy
			}
			claimed = true
			return makeBaseClaim(companyID, blueprintID, sType), nil
		},
		loadManualFn: func(ctx context.Context, cID, bID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintManualSnapshot, error) {
			return makeBaseManualSnapshot(companyID, blueprintID, sType), nil
		},
		completeManualFn: func(ctx context.Context, params repository.CompleteBlueprintManualParams) (*repository.BlueprintSectionPersistenceResult, error) {
			mu.Lock()
			defer mu.Unlock()
			if claimed {
				return nil, repository.ErrBlueprintManualConflict
			}
			claimed = true
			return &repository.BlueprintSectionPersistenceResult{
				RevisionID:       params.Snapshot.RevisionID,
				SectionType:      params.Snapshot.SectionType,
				SectionStatus:    domain.BlueprintSectionValidated,
				GenerationStatus: domain.BlueprintGenerationInProgress,
				ContentVersion:   params.Snapshot.ExpectedContentVersion + 1,
				Attempt:          1,
			}, nil
		},
	}

	prov := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			return GenerateResult{Text: validJourneyPayload()}, nil
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}

	pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

	const callers = 3
	start := make(chan struct{})
	type outcome struct {
		res *BlueprintPipelineResult
		err error
	}
	outcomes := make(chan outcome, callers)

	// Caller 1: Continue
	go func() {
		<-start
		res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
		outcomes <- outcome{res: res, err: err}
	}()

	// Caller 2: Retry
	go func() {
		<-start
		res, err := pipeline.RetrySection(context.Background(), companyID, blueprintID, domain.BlueprintSectionJourney)
		outcomes <- outcome{res: res, err: err}
	}()

	// Caller 3: ManualComplete
	go func() {
		<-start
		res, err := pipeline.ManualCompleteSection(context.Background(), companyID, blueprintID, domain.BlueprintSectionJourney, json.RawMessage(validJourneyPayload()))
		outcomes <- outcome{res: res, err: err}
	}()

	close(start)

	var successes int
	var failures int
	for i := 0; i < callers; i++ {
		out := <-outcomes
		if out.err == nil {
			successes++
			assert.NotNil(t, out.res)
		} else {
			failures++
			isExpectedConflict := errors.Is(out.err, ErrBlueprintPipelineBusy) ||
				errors.Is(out.err, ErrBlueprintExecutionConflict) ||
				errors.Is(out.err, ErrBlueprintManualNotAllowed)
			assert.True(t, isExpectedConflict, "unexpected error for losing caller: %v", out.err)
		}
	}

	assert.Equal(t, 1, successes, "exactly one caller must win")
	assert.Equal(t, callers-1, failures, "all losing callers must fail")
}

func TestBlueprintPipelineContinueRunsCrossValidationWithoutProvider(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	claim := makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionCrossValidation)
	claim.RevisionContent = testCompleteValidBundle()

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return claim, nil
		},
	}
	prov := &fakeAIProvider{}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return prov, "openai", "gpt-4o", nil
		},
	}
	prompts := &fakePromptBuilder{}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	pipeline := NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)

	res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, domain.BlueprintSectionCrossValidation, res.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, res.SectionStatus)
	assert.Equal(t, domain.BlueprintGenerationCompleted, res.GenerationStatus)
	assert.Equal(t, claim.ExpectedContentVersion, res.ContentVersion, "cross validation must not increment content version")

	assert.Equal(t, 1, repo.claimNextCalls)
	assert.Equal(t, 0, provSource.calls, "ProviderSource must not be resolved for cross validation")
	assert.Equal(t, 0, prov.calls, "provider Generate must not be called for cross validation")
	assert.Equal(t, 0, prompts.calls, "prompt builder must not be called for cross validation")
	assert.Equal(t, 1, repo.completeCalls)
	assert.Equal(t, 0, repo.failCalls)

	require.Len(t, repo.completeParams, 1)
	cp := repo.completeParams[0]
	assert.True(t, cp.Final)
	assert.Empty(t, cp.ProviderName)
	assert.Empty(t, cp.Model)
	assert.Equal(t, claim.RevisionContent, cp.RevisionContent)

	var crossResult BlueprintCrossValidationResult
	err = json.Unmarshal(cp.CrossValidationResult, &crossResult)
	require.NoError(t, err)
	assert.True(t, crossResult.Valid)
}

func TestBlueprintPipelineContinuePersistsCrossValidationWarningsAndCoverage(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	claim := makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionCrossValidation)
	claim.RevisionContent = testCompleteValidBundle()

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return claim, nil
		},
	}
	provSource := &fakeProviderSource{}
	prompts := &fakePromptBuilder{}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	pipeline := NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)

	res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, res)

	require.Len(t, repo.completeParams, 1)
	cp := repo.completeParams[0]
	assert.True(t, cp.Final)

	// SectionContent, SectionValidation, and CrossValidationResult must be identical
	assert.JSONEq(t, string(cp.CrossValidationResult), string(cp.SectionContent))
	assert.JSONEq(t, string(cp.CrossValidationResult), string(cp.SectionValidation))

	var crossResult BlueprintCrossValidationResult
	err = json.Unmarshal(cp.CrossValidationResult, &crossResult)
	require.NoError(t, err)
	assert.True(t, crossResult.Valid)
	assert.NotEmpty(t, crossResult.Issues, "expected non-blocking warnings in sample bundle")
	for _, issue := range crossResult.Issues {
		assert.False(t, issue.Blocking, "all issues in valid bundle must be non-blocking warnings")
	}
	assert.Greater(t, crossResult.Coverage.ArchitectureToWBS, 0.0)
}

func TestBlueprintPipelineContinueBlocksOnCrossValidationErrors(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	// Create bundle with dangling architecture reference in WBS module
	invalidBundle := strings.Replace(
		string(testCompleteValidBundle()),
		`"references": ["architecture.node.node-1"]`,
		`"references": ["architecture.node.nonexistent-node"]`,
		1,
	)

	claim := makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionCrossValidation)
	claim.RevisionContent = json.RawMessage(invalidBundle)

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return claim, nil
		},
	}
	provSource := &fakeProviderSource{}
	prompts := &fakePromptBuilder{}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	pipeline := NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)

	res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	assert.Nil(t, res)
	assert.True(t, errors.Is(err, ErrBlueprintCrossValidationFailure))

	assert.Equal(t, 0, provSource.calls)
	assert.Equal(t, 0, repo.completeCalls)
	assert.Equal(t, 1, repo.failCalls)

	require.Len(t, repo.failParams, 1)
	fp := repo.failParams[0]
	assert.Equal(t, "cross_validation_error", fp.SafeErrorCode)
	assert.False(t, fp.Terminal)

	// SectionContent and CrossValidationResult must be identical safe cross validation result
	require.NotEmpty(t, fp.SectionContent)
	require.NotEmpty(t, fp.CrossValidationResult)
	assert.JSONEq(t, string(fp.SectionContent), string(fp.CrossValidationResult))

	var crossResult BlueprintCrossValidationResult
	err = json.Unmarshal(fp.CrossValidationResult, &crossResult)
	require.NoError(t, err)
	assert.False(t, crossResult.Valid)
	assert.Equal(t, "cross_validation_error", crossResult.SafeOutcome)

	hasDangling := false
	for _, issue := range crossResult.Issues {
		if issue.Code == BlueprintIssueDanglingReference && issue.Blocking {
			hasDangling = true
			break
		}
	}
	assert.True(t, hasDangling, "expected blocking dangling reference issue")
}

func TestBlueprintPipelineContinueCompletedIsIdempotentNoOp(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return nil, ErrBlueprintPipelineCompleted
		},
	}
	provSource := &fakeProviderSource{}
	prompts := &fakePromptBuilder{}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	pipeline := NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)

	res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	assert.Nil(t, res)
	assert.True(t, errors.Is(err, ErrBlueprintPipelineCompleted))

	assert.Equal(t, 1, repo.claimNextCalls)
	assert.Equal(t, 0, provSource.calls)
	assert.Equal(t, 0, repo.completeCalls)
	assert.Equal(t, 0, repo.failCalls)
}

func TestBlueprintPipelineContinueBlocksWhenDependencyIsNotStable(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()

	repo := &fakePipelineRepo{
		claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
			return nil, repository.ErrBlueprintNoEligibleSection
		},
	}
	provSource := &fakeProviderSource{}
	prompts := &fakePromptBuilder{}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	pipeline := NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)

	res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
	assert.Nil(t, res)
	assert.True(t, errors.Is(err, ErrBlueprintPipelineBlocked))

	assert.Equal(t, 1, repo.claimNextCalls)
	assert.Equal(t, 0, provSource.calls)
	assert.Equal(t, 0, repo.completeCalls)
	assert.Equal(t, 0, repo.failCalls)
}

func TestBlueprintPipelinePromptClassification(t *testing.T) {
	testCases := []struct {
		name             string
		promptErr        error
		expectedErr      error
		expectedSafeCode string
		expectedTerminal bool
	}{
		{
			name:             "prompt invalid input fails with prompt_construction_failed",
			promptErr:        ErrBlueprintPromptInvalidInput,
			expectedErr:      ErrBlueprintPromptConstruction,
			expectedSafeCode: "prompt_construction_failed",
			expectedTerminal: false,
		},
		{
			name:             "prompt security violation fails with security_failure",
			promptErr:        ErrBlueprintPromptSecurity,
			expectedErr:      ErrBlueprintSecurityFailure,
			expectedSafeCode: "security_failure",
			expectedTerminal: true,
		},
		{
			name:             "arbitrary prompt error falls back to prompt_construction_failed",
			promptErr:        errors.New("arbitrary prompt formatting failure"),
			expectedErr:      ErrBlueprintPromptConstruction,
			expectedSafeCode: "prompt_construction_failed",
			expectedTerminal: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			companyID := uuid.New()
			blueprintID := uuid.New()

			repo := &fakePipelineRepo{
				claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
					return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
				},
			}
			prov := &fakeAIProvider{}
			provSource := &fakeProviderSource{
				providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
					return prov, "openai", "gpt-4o", nil
				},
			}
			prompts := &fakePromptBuilder{
				buildFn: func(input BlueprintPromptInput) (string, error) {
					return "", tc.promptErr
				},
			}

			pipeline := NewBlueprintPipeline(repo, provSource, prompts, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

			res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
			assert.Nil(t, res)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedErr)

			assert.Equal(t, 0, prov.calls, "fake provider must have zero calls")
			assert.Equal(t, 1, repo.failCalls, "failure must be persisted")
			require.Len(t, repo.failParams, 1)
			assert.Equal(t, tc.expectedSafeCode, repo.failParams[0].SafeErrorCode)
			assert.Equal(t, tc.expectedTerminal, repo.failParams[0].Terminal)
			assert.Nil(t, repo.failParams[0].SectionContent)
			assert.Equal(t, 0, repo.completeCalls)
		})
	}
}

func TestMapProviderResolutionError(t *testing.T) {
	testCases := []struct {
		name             string
		inputErr         error
		expectedSafeCode string
		expectedSentinel error
	}{
		{
			name:             "typed invalid credential",
			inputErr:         &ProviderError{Code: ErrInvalidCredential},
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "typed credential not configured",
			inputErr:         &ProviderError{Code: ErrCredentialNotConfigured},
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "typed unsupported model",
			inputErr:         &ProviderError{Code: ErrUnsupportedModel},
			expectedSafeCode: "provider_configuration",
			expectedSentinel: ErrBlueprintProviderConfiguration,
		},
		{
			name:             "typed rate limited",
			inputErr:         &ProviderError{Code: ErrRateLimited},
			expectedSafeCode: "provider_rate_limited",
			expectedSentinel: ErrBlueprintProviderRateLimited,
		},
		{
			name:             "typed timeout",
			inputErr:         &ProviderError{Code: ErrTimeout},
			expectedSafeCode: "provider_timeout",
			expectedSentinel: ErrBlueprintProviderTimeout,
		},
		{
			name:             "typed provider unavailable",
			inputErr:         &ProviderError{Code: ErrProviderUnavailable},
			expectedSafeCode: "provider_unavailable",
			expectedSentinel: ErrBlueprintProviderUnavailable,
		},
		{
			name:             "typed request rejected",
			inputErr:         &ProviderError{Code: ErrRequestRejected},
			expectedSafeCode: "provider_request_rejected",
			expectedSentinel: ErrBlueprintProviderRejected,
		},
		{
			name:             "typed payload too large",
			inputErr:         &ProviderError{Code: ErrPayloadTooLarge},
			expectedSafeCode: "provider_payload_too_large",
			expectedSentinel: ErrBlueprintProviderPayloadSize,
		},
		{
			name:             "typed unknown code defaults to provider_unavailable",
			inputErr:         &ProviderError{Code: "custom_unknown_code"},
			expectedSafeCode: "provider_unavailable",
			expectedSentinel: ErrBlueprintProviderUnavailable,
		},
		{
			name:             "untyped credential string",
			inputErr:         errors.New("missing API credential in settings"),
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "untyped api key string",
			inputErr:         errors.New("invalid key provided"),
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "untyped model string",
			inputErr:         errors.New("unsupported model claude-1"),
			expectedSafeCode: "provider_configuration",
			expectedSentinel: ErrBlueprintProviderConfiguration,
		},
		{
			name:             "untyped config string",
			inputErr:         errors.New("config is missing baseUrl"),
			expectedSafeCode: "provider_configuration",
			expectedSentinel: ErrBlueprintProviderConfiguration,
		},
		{
			name:             "untyped rate string",
			inputErr:         errors.New("rate limit quota exceeded"),
			expectedSafeCode: "provider_rate_limited",
			expectedSentinel: ErrBlueprintProviderRateLimited,
		},
		{
			name:             "untyped timeout string",
			inputErr:         errors.New("connection timeout to provider"),
			expectedSafeCode: "provider_timeout",
			expectedSentinel: ErrBlueprintProviderTimeout,
		},
		{
			name:             "untyped payload string",
			inputErr:         errors.New("payload exceeds maximum allowed limit"),
			expectedSafeCode: "provider_payload_too_large",
			expectedSentinel: ErrBlueprintProviderPayloadSize,
		},
		{
			name:             "untyped too large string",
			inputErr:         errors.New("request entity is too large"),
			expectedSafeCode: "provider_payload_too_large",
			expectedSentinel: ErrBlueprintProviderPayloadSize,
		},
		{
			name:             "untyped rejected string",
			inputErr:         errors.New("request rejected by remote policy"),
			expectedSafeCode: "provider_request_rejected",
			expectedSentinel: ErrBlueprintProviderRejected,
		},
		{
			name:             "untyped unknown defaults to provider_unavailable",
			inputErr:         errors.New("something totally random happened"),
			expectedSafeCode: "provider_unavailable",
			expectedSentinel: ErrBlueprintProviderUnavailable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			safeCode, sentinel := mapProviderResolutionError(tc.inputErr)
			assert.Equal(t, tc.expectedSafeCode, safeCode)
			assert.Equal(t, tc.expectedSentinel, sentinel)
		})
	}
}

func TestMapProviderGenerateError(t *testing.T) {
	testCases := []struct {
		name             string
		inputErr         error
		attemptCtx       context.Context
		expectedSafeCode string
		expectedSentinel error
	}{
		{
			name:             "context deadline exceeded",
			inputErr:         context.DeadlineExceeded,
			expectedSafeCode: "provider_timeout",
			expectedSentinel: ErrBlueprintProviderTimeout,
		},
		{
			name:             "typed timeout",
			inputErr:         &ProviderError{Code: ErrTimeout},
			expectedSafeCode: "provider_timeout",
			expectedSentinel: ErrBlueprintProviderTimeout,
		},
		{
			name:             "typed rate limited",
			inputErr:         &ProviderError{Code: ErrRateLimited},
			expectedSafeCode: "provider_rate_limited",
			expectedSentinel: ErrBlueprintProviderRateLimited,
		},
		{
			name:             "typed invalid credential",
			inputErr:         &ProviderError{Code: ErrInvalidCredential},
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "typed credential not configured",
			inputErr:         &ProviderError{Code: ErrCredentialNotConfigured},
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "typed unsupported model",
			inputErr:         &ProviderError{Code: ErrUnsupportedModel},
			expectedSafeCode: "provider_configuration",
			expectedSentinel: ErrBlueprintProviderConfiguration,
		},
		{
			name:             "typed provider unavailable",
			inputErr:         &ProviderError{Code: ErrProviderUnavailable},
			expectedSafeCode: "provider_unavailable",
			expectedSentinel: ErrBlueprintProviderUnavailable,
		},
		{
			name:             "typed request rejected",
			inputErr:         &ProviderError{Code: ErrRequestRejected},
			expectedSafeCode: "provider_request_rejected",
			expectedSentinel: ErrBlueprintProviderRejected,
		},
		{
			name:             "typed payload too large",
			inputErr:         &ProviderError{Code: ErrPayloadTooLarge},
			expectedSafeCode: "provider_payload_too_large",
			expectedSentinel: ErrBlueprintProviderPayloadSize,
		},
		{
			name:             "typed unknown code defaults to provider_unavailable",
			inputErr:         &ProviderError{Code: "unknown_generate_code"},
			expectedSafeCode: "provider_unavailable",
			expectedSentinel: ErrBlueprintProviderUnavailable,
		},
		{
			name:             "untyped timeout string",
			inputErr:         errors.New("upstream connection timeout"),
			expectedSafeCode: "provider_timeout",
			expectedSentinel: ErrBlueprintProviderTimeout,
		},
		{
			name:             "untyped rate string",
			inputErr:         errors.New("exceeded rate limit per minute"),
			expectedSafeCode: "provider_rate_limited",
			expectedSentinel: ErrBlueprintProviderRateLimited,
		},
		{
			name:             "untyped credential string",
			inputErr:         errors.New("bad credential error"),
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "untyped key string",
			inputErr:         errors.New("api key rejected"),
			expectedSafeCode: "provider_invalid_credential",
			expectedSentinel: ErrBlueprintProviderInvalidCredential,
		},
		{
			name:             "untyped payload string",
			inputErr:         errors.New("prompt payload is too big"),
			expectedSafeCode: "provider_payload_too_large",
			expectedSentinel: ErrBlueprintProviderPayloadSize,
		},
		{
			name:             "untyped too large string",
			inputErr:         errors.New("context length too large"),
			expectedSafeCode: "provider_payload_too_large",
			expectedSentinel: ErrBlueprintProviderPayloadSize,
		},
		{
			name:             "untyped rejected string",
			inputErr:         errors.New("prompt rejected by safety filters"),
			expectedSafeCode: "provider_request_rejected",
			expectedSentinel: ErrBlueprintProviderRejected,
		},
		{
			name:             "untyped unknown defaults to provider_unavailable",
			inputErr:         errors.New("some network socket drop"),
			expectedSafeCode: "provider_unavailable",
			expectedSentinel: ErrBlueprintProviderUnavailable,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			safeCode, sentinel := mapProviderGenerateError(tc.inputErr, tc.attemptCtx)
			assert.Equal(t, tc.expectedSafeCode, safeCode)
			assert.Equal(t, tc.expectedSentinel, sentinel)
		})
	}
}

func TestBlueprintPipelineContinueProviderErrorMapping(t *testing.T) {
	testCases := []struct {
		name             string
		resolutionErr    error
		generateErr      error
		expectedErr      error
		expectedSafeCode string
	}{
		{
			name:             "generate request rejected",
			generateErr:      &ProviderError{Code: ErrRequestRejected, Message: "content filter", UpstreamStatus: 400, RequestID: "req-rej-1"},
			expectedErr:      ErrBlueprintProviderRejected,
			expectedSafeCode: "provider_request_rejected",
		},
		{
			name:             "generate payload too large",
			generateErr:      &ProviderError{Code: ErrPayloadTooLarge, Message: "max tokens exceeded", UpstreamStatus: 413, RequestID: "req-large-1"},
			expectedErr:      ErrBlueprintProviderPayloadSize,
			expectedSafeCode: "provider_payload_too_large",
		},
		{
			name:             "resolution request rejected",
			resolutionErr:    &ProviderError{Code: ErrRequestRejected, Message: "account blocked", UpstreamStatus: 403, RequestID: "req-res-rej"},
			expectedErr:      ErrBlueprintProviderRejected,
			expectedSafeCode: "provider_request_rejected",
		},
		{
			name:             "resolution payload too large",
			resolutionErr:    &ProviderError{Code: ErrPayloadTooLarge, Message: "init config too large", UpstreamStatus: 413, RequestID: "req-res-large"},
			expectedErr:      ErrBlueprintProviderPayloadSize,
			expectedSafeCode: "provider_payload_too_large",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			companyID := uuid.New()
			blueprintID := uuid.New()

			repo := &fakePipelineRepo{
				claimNextFn: func(ctx context.Context, cID, bID uuid.UUID, now time.Time, staleAfter time.Duration) (*repository.BlueprintSectionClaim, error) {
					return makeBaseClaim(companyID, blueprintID, domain.BlueprintSectionJourney), nil
				},
			}
			prov := &fakeAIProvider{
				generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
					if tc.generateErr != nil {
						return GenerateResult{}, tc.generateErr
					}
					return GenerateResult{Text: validJourneyPayload()}, nil
				},
			}
			provSource := &fakeProviderSource{
				providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
					if tc.resolutionErr != nil {
						return nil, "mock-provider", "mock-model", tc.resolutionErr
					}
					return prov, "mock-provider", "mock-model", nil
				},
			}

			pipeline := NewBlueprintPipeline(repo, provSource, &fakePromptBuilder{}, NewBlueprintSectionValidator(), NewProductionBlueprintCrossValidator())

			res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
			assert.Nil(t, res)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.expectedErr)

			assert.Equal(t, 1, repo.failCalls)
			require.Len(t, repo.failParams, 1)
			assert.Equal(t, tc.expectedSafeCode, repo.failParams[0].SafeErrorCode)
		})
	}
}

func TestLogBlueprintProviderFailure(t *testing.T) {
	t.Run("logs all diagnostic fields safely without sensitive data", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		oldLogger := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldLogger)

		claim := makeBaseClaim(uuid.New(), uuid.New(), domain.BlueprintSectionArchitecture)
		claim.Attempt = 2
		provErr := &ProviderError{
			Code:           ErrRequestRejected,
			Message:        "sensitive message with key secret123",
			UpstreamStatus: 400,
			RequestID:      "req-diag-999",
			Retryable:      false,
		}

		logBlueprintProviderFailure(
			claim,
			"openai",
			"gpt-4o",
			"provider_request_rejected",
			1024,
			150*time.Millisecond,
			provErr,
		)

		logOutput := buf.String()
		assert.Contains(t, logOutput, claim.BlueprintID.String())
		assert.Contains(t, logOutput, claim.RevisionID.String())
		assert.Contains(t, logOutput, "architecture")
		assert.Contains(t, logOutput, claim.CompanyID.String())
		assert.Contains(t, logOutput, "openai")
		assert.Contains(t, logOutput, "gpt-4o")
		assert.Contains(t, logOutput, "provider_request_rejected")
		assert.Contains(t, logOutput, "req-diag-999")
		assert.Contains(t, logOutput, "400")

		// Assert no sensitive information or message body in log attributes
		assert.NotContains(t, logOutput, "secret123")
	})

	t.Run("handles nil claim and nil providerErr gracefully", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, nil))
		oldLogger := slog.Default()
		slog.SetDefault(logger)
		defer slog.SetDefault(oldLogger)

		assert.NotPanics(t, func() {
			logBlueprintProviderFailure(
				nil,
				"unknown",
				"unknown",
				"provider_unavailable",
				0,
				10*time.Millisecond,
				nil,
			)
		})

		logOutput := buf.String()
		assert.Contains(t, logOutput, "provider_unavailable")
	})
}
