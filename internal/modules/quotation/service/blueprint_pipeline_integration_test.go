//go:build integration

package service_test

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
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type pipelineScriptedAIProvider struct {
	mu           sync.Mutex
	calls        int
	callsByStage map[domain.BlueprintSectionType]int
	generateFn   func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error)
}

func newPipelineScriptedAIProvider() *pipelineScriptedAIProvider {
	return &pipelineScriptedAIProvider{
		callsByStage: make(map[domain.BlueprintSectionType]int),
	}
}

func (s *pipelineScriptedAIProvider) Generate(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.generateFn != nil {
		return s.generateFn(ctx, req)
	}
	return service.GenerateResult{Text: "{}"}, nil
}

func detectStageFromPrompt(prompt string) domain.BlueprintSectionType {
	if strings.Contains(prompt, `stage="journey"`) {
		return domain.BlueprintSectionJourney
	} else if strings.Contains(prompt, `stage="architecture"`) {
		return domain.BlueprintSectionArchitecture
	} else if strings.Contains(prompt, `stage="state_machine"`) {
		return domain.BlueprintSectionStateMachine
	} else if strings.Contains(prompt, `stage="workflow"`) {
		return domain.BlueprintSectionWorkflow
	} else if strings.Contains(prompt, `stage="wbs"`) {
		return domain.BlueprintSectionWBS
	}
	return domain.BlueprintSectionContext
}


type pipelineScriptedProviderSource struct {
	mu         sync.Mutex
	calls      int
	provider   service.AIProvider
	providerFn func(ctx context.Context, companyID uuid.UUID) (service.AIProvider, string, string, error)
}

func (s *pipelineScriptedProviderSource) ProviderForCompany(ctx context.Context, companyID uuid.UUID) (service.AIProvider, string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.providerFn != nil {
		return s.providerFn(ctx, companyID)
	}
	return s.provider, "scripted-provider", "scripted-model-v1", nil
}

func setupIntegrationPipeline(t *testing.T, pool *pgxpool.Pool, provSource service.ProviderSource) (*service.BlueprintPipeline, *repository.BlueprintGenerationRepository, *service.BlueprintGenerationService) {
	t.Helper()
	repo := repository.NewBlueprintGenerationRepository(pool)
	inputRepo := repository.NewBlueprintGenerationInputRepository(pool)
	skillRepo := repository.NewSkillRepository(pool)
	contextBuilder := service.NewBlueprintSourceContextBuilder()

	createSvc := service.NewBlueprintGenerationService(inputRepo, skillRepo, contextBuilder, repo)
	prompts := service.NewBlueprintPromptBuilder()
	validator := service.NewBlueprintSectionValidator()
	crossValidator := service.NewProductionBlueprintCrossValidator()

	pipeline := service.NewBlueprintPipeline(repo, provSource, prompts, validator, crossValidator)
	return pipeline, repo, createSvc
}

func seedIntegrationAggregate(t *testing.T, pool *pgxpool.Pool, createSvc *service.BlueprintGenerationService, companyID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	qID, revID := seedQuotationAndRevision(
		t, pool, companyID, "draft", "completed",
		"<section><h2>Enterprise Cloud Solution</h2><p>Full architecture and implementation</p></section>",
	)
	seedActiveBlueprintSkill(
		t, pool, companyID, "Enterprise Blueprint Architect",
		"Produce high-level system architecture blueprint",
	)

	createdBy := uuid.New()
	req := validBlueprintRequest(qID, revID)
	req.Title = "Enterprise Cloud Blueprint"

	res, err := createSvc.CreateBlueprint(ctx, companyID, createdBy, req)
	require.NoError(t, err)
	require.NotNil(t, res)

	return res.Blueprint.ID, res.Revision.ID
}

func validJourneyIntegrationPayload() string {
	return `{
		"journey": {
			"phases": [{"id": "phase-1", "label": "Discovery", "order": 1}],
			"actors": [{"id": "actor-1", "label": "Client", "order": 1}],
			"activities": [{"id": "act-1", "phase_id": "phase-1", "actor_id": "actor-1", "label": "Initial Meeting", "description": "Discuss project scope"}]
		}
	}`
}

func validArchitectureIntegrationPayload() string {
	return `{
		"architecture": {
			"groups": [{"id": "grp-1", "label": "Core Services", "order": 1}],
			"nodes": [{"id": "node-1", "group_id": "grp-1", "type": "service", "label": "Auth Service", "description": "Authentication and authorization", "layout": {"x": 100.0, "y": 100.0}}],
			"edges": [{"id": "edge-1", "source": "node-1", "target": "node-1", "label": "Self", "type": "sync"}]
		}
	}`
}

func validStateMachineIntegrationPayload() string {
	return `{
		"state_machine": {
			"entity": "Quotation",
			"initial_state": "state-draft",
			"states": [
				{"id": "state-draft", "label": "Draft", "type": "normal", "layout": {"x": 0.0, "y": 0.0}},
				{"id": "state-published", "label": "Published", "type": "terminal", "layout": {"x": 100.0, "y": 100.0}}
			],
			"transitions": [
				{"id": "trans-publish", "source": "state-draft", "target": "state-published", "event": "PUBLISH", "actor": "admin", "guard": "valid", "actions": ["notify"]}
			]
		}
	}`
}

func validWorkflowIntegrationPayload() string {
	return `{
		"workflows": [{
			"id": "wf-main",
			"name": "Main Flow",
			"description": "Primary process workflow",
			"lanes": [{"id": "lane-1", "label": "Customer", "order": 1}],
			"nodes": [
				{"id": "node-start", "lane_id": "lane-1", "type": "start", "label": "Start", "description": "", "layout": {"x": 0.0, "y": 0.0}}
			],
			"edges": []
		}]
	}`
}

func validWBSIntegrationPayload() string {
	return `{
		"wbs": {
			"modules": [
				{
					"id": "mod-auth",
					"name": "Auth Module",
					"description": "Authentication and authorization",
					"priority": "high",
					"dependencies": [],
					"references": ["architecture.node.node-1"],
					"features": [
						{
							"id": "feat-login",
							"name": "User Login",
							"description": "Login with password",
							"acceptance_criteria": ["Successful login returns JWT"]
						}
					]
				}
			]
		},
		"cross_references": [
			{
				"id": "xref-1",
				"source_ref": "journey.activity.act-1",
				"target_ref": "wbs.module.mod-auth",
				"relation_type": "implements",
				"description": "Activity implements auth"
			}
		]
	}`
}

func TestBlueprintPipelineProductionPathFailureRetryAndRecovery(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	blueprintID, revisionID := seedIntegrationAggregate(t, pool, createSvc, companyID)

	ctx := context.Background()

	// Track stage call counts
	var callsJourney, callsArchitecture, callsStateMachine, callsWorkflow, callsWBS int

	// 1. Initial snapshot state check
	initialGen, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	require.NotNil(t, initialGen)
	assert.Equal(t, revisionID, initialGen.Revision.ID)
	initialContextSnapshot := string(initialGen.Revision.SourceContextSnapshot)
	initialSkillSnapshot := string(initialGen.Revision.SkillSnapshot)
	require.Equal(t, 1, initialGen.Revision.ContentVersion)
	require.Equal(t, string(domain.BlueprintGenerationPending), initialGen.Revision.GenerationStatus)

	// 2. Continue Journey (valid)
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		callsJourney++
		return service.GenerateResult{Text: validJourneyIntegrationPayload()}, nil
	}
	resJourney, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resJourney)
	assert.Equal(t, domain.BlueprintSectionJourney, resJourney.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resJourney.SectionStatus)
	assert.Equal(t, 2, resJourney.ContentVersion)
	assert.Equal(t, 1, callsJourney)

	// Verify Journey preserved in DB
	genAfterJourney, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	journeyContent := string(genAfterJourney.Revision.ContentJSON)
	assert.Contains(t, journeyContent, "Discovery")

	// 3. Continue Architecture -> invalid_schema
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		callsArchitecture++
		return service.GenerateResult{Text: `{"architecture": "invalid non-object format"}`}, nil
	}
	resArchFail, err := pipeline.Continue(ctx, companyID, blueprintID)
	assert.Nil(t, resArchFail)
	assert.True(t, errors.Is(err, service.ErrBlueprintInvalidSchema))
	assert.Equal(t, 1, callsArchitecture)

	// Reload: Architecture failed, safe_error_code = invalid_schema, Journey unchanged, content version unchanged
	genAfterArchFail, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	assert.Equal(t, 2, genAfterArchFail.Revision.ContentVersion, "failure must not increment content version")
	assert.Equal(t, string(domain.BlueprintGenerationPartialFailed), genAfterArchFail.Revision.GenerationStatus)
	assert.Equal(t, journeyContent, string(genAfterArchFail.Revision.ContentJSON), "failure must preserve Journey and revision content byte-for-byte")

	var archRunAfterFail *domain.BlueprintSectionRun
	for i := range genAfterArchFail.SectionRuns {
		if genAfterArchFail.SectionRuns[i].SectionType == string(domain.BlueprintSectionArchitecture) {
			archRunAfterFail = &genAfterArchFail.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, archRunAfterFail)
	assert.Equal(t, string(domain.BlueprintSectionFailed), archRunAfterFail.Status)
	assert.Equal(t, 1, archRunAfterFail.AttemptCount)
	require.NotNil(t, archRunAfterFail.SafeErrorCode)
	assert.Equal(t, "invalid_schema", *archRunAfterFail.SafeErrorCode)

	// 4. Retry Architecture -> completed
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		callsArchitecture++
		return service.GenerateResult{Text: validArchitectureIntegrationPayload()}, nil
	}
	resArchRetry, err := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionArchitecture)
	require.NoError(t, err)
	require.NotNil(t, resArchRetry)
	assert.Equal(t, domain.BlueprintSectionArchitecture, resArchRetry.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resArchRetry.SectionStatus)
	assert.Equal(t, 3, resArchRetry.ContentVersion)
	assert.Equal(t, 2, callsArchitecture)

	// 5. Continue State Machine -> completed
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		callsStateMachine++
		return service.GenerateResult{Text: validStateMachineIntegrationPayload()}, nil
	}
	resState, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resState)
	assert.Equal(t, domain.BlueprintSectionStateMachine, resState.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resState.SectionStatus)
	assert.Equal(t, 4, resState.ContentVersion)
	assert.Equal(t, 1, callsStateMachine)

	// 6. Continue Workflow -> completed
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		callsWorkflow++
		return service.GenerateResult{Text: validWorkflowIntegrationPayload()}, nil
	}
	resWorkflow, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resWorkflow)
	assert.Equal(t, domain.BlueprintSectionWorkflow, resWorkflow.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resWorkflow.SectionStatus)
	assert.Equal(t, 5, resWorkflow.ContentVersion)
	assert.Equal(t, 1, callsWorkflow)

	// 7. Continue WBS -> completed
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		callsWBS++
		return service.GenerateResult{Text: validWBSIntegrationPayload()}, nil
	}
	resWBS, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resWBS)
	assert.Equal(t, domain.BlueprintSectionWBS, resWBS.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resWBS.SectionStatus)
	assert.Equal(t, 6, resWBS.ContentVersion)
	assert.Equal(t, 1, callsWBS)

	// 8. Continue Cross Validation -> completed (deterministic, zero provider calls)
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		t.Fatalf("cross validation stage must never call AIProvider.Generate")
		return service.GenerateResult{}, nil
	}
	provSourceCallsBeforeCross := provSource.calls

	resCross, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resCross)
	assert.Equal(t, domain.BlueprintSectionCrossValidation, resCross.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resCross.SectionStatus)
	assert.Equal(t, domain.BlueprintGenerationCompleted, resCross.GenerationStatus)
	assert.Equal(t, 6, resCross.ContentVersion, "cross validation must not increment content version")
	assert.Equal(t, provSourceCallsBeforeCross, provSource.calls, "cross validation must not touch ProviderSource")

	// Assert exact provider call counts: Journey 1, Architecture 2, State Machine 1, Workflow 1, WBS 1, Cross 0, total 6
	assert.Equal(t, 1, callsJourney, "Journey calls must be exactly 1")
	assert.Equal(t, 2, callsArchitecture, "Architecture calls must be exactly 2 (1 failed + 1 retry)")
	assert.Equal(t, 1, callsStateMachine, "State Machine calls must be exactly 1")
	assert.Equal(t, 1, callsWorkflow, "Workflow calls must be exactly 1")
	assert.Equal(t, 1, callsWBS, "WBS calls must be exactly 1")
	assert.Equal(t, 6, provider.calls, "Total provider Generate calls must be exactly 6")

	// 9. Reload and assert final completed revision state
	finalGen, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	require.NotNil(t, finalGen)

	assert.Equal(t, string(domain.BlueprintGenerationCompleted), finalGen.Revision.GenerationStatus)
	assert.Equal(t, "complete", finalGen.Revision.CompletenessStatus)
	assert.Equal(t, 6, finalGen.Revision.ContentVersion)
	require.NotNil(t, finalGen.Revision.GeneratedAt)

	// Strict bundle decode
	bundle, err := service.DecodeBlueprintBundle(finalGen.Revision.ContentJSON)
	require.NoError(t, err)
	assert.Equal(t, "1.0", bundle.SchemaVersion)
	assert.NotEmpty(t, bundle.Journey.Activities)
	assert.NotEmpty(t, bundle.Architecture.Nodes)
	assert.NotEmpty(t, bundle.StateMachine.States)
	assert.NotEmpty(t, bundle.Workflows, "bundle must have plural workflows")
	assert.NotEmpty(t, bundle.WBS.Modules)
	assert.NotEmpty(t, bundle.CrossReferences, "bundle must have cross_references mapped from WBS")

	// Strict cross-validation on final bundle must be valid
	crossValidator := service.NewProductionBlueprintCrossValidator()
	crossValResult := crossValidator.Validate(bundle)
	assert.True(t, crossValResult.Valid, "final bundle must pass cross validation")

	// Snapshots must remain unchanged
	assert.Equal(t, initialContextSnapshot, string(finalGen.Revision.SourceContextSnapshot))
	assert.Equal(t, initialSkillSnapshot, string(finalGen.Revision.SkillSnapshot))

	// All 7 section runs must be completed or validated
	require.Len(t, finalGen.SectionRuns, 7)
	for _, run := range finalGen.SectionRuns {
		isStable := run.Status == string(domain.BlueprintSectionCompleted) || run.Status == string(domain.BlueprintSectionValidated)
		assert.True(t, isStable, "run %s status %s must be completed or validated", run.SectionType, run.Status)
	}
}

func TestBlueprintPipelineConcurrentContinuePreventsDuplicateProviderExecution(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	blueprintID, _ := seedIntegrationAggregate(t, pool, createSvc, companyID)

	ctx := context.Background()

	const callers = 8
	var failureWg sync.WaitGroup
	failureWg.Add(callers - 1)

	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		done := make(chan struct{})
		go func() {
			failureWg.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		return service.GenerateResult{Text: validJourneyIntegrationPayload()}, nil
	}

	start := make(chan struct{})
	type outcome struct {
		res *service.BlueprintPipelineResult
		err error
	}
	outcomes := make(chan outcome, callers)

	for i := 0; i < callers; i++ {
		go func() {
			<-start
			res, err := pipeline.Continue(context.Background(), companyID, blueprintID)
			if err != nil {
				failureWg.Done()
			}
			outcomes <- outcome{res: res, err: err}
		}()
	}

	close(start)

	var successes int
	var failures int
	for i := 0; i < callers; i++ {
		out := <-outcomes
		if out.err == nil {
			successes++
			assert.NotNil(t, out.res)
			assert.Equal(t, domain.BlueprintSectionJourney, out.res.Section)
			assert.Equal(t, domain.BlueprintSectionCompleted, out.res.SectionStatus)
		} else {
			failures++
			isExpectedConflict := errors.Is(out.err, service.ErrBlueprintPipelineBusy) ||
				errors.Is(out.err, service.ErrBlueprintExecutionConflict) ||
				errors.Is(out.err, service.ErrBlueprintPipelineBlocked)
			assert.True(t, isExpectedConflict, "unexpected error for loser: %v", out.err)
		}
	}

	assert.Equal(t, 1, successes, "exactly one caller must win the claim")
	assert.Equal(t, callers-1, failures, "all 7 losers must fail with busy/conflict")
	assert.Equal(t, 1, provider.calls, "exactly one provider Generate call must occur")

	// Verify in DB that only Journey was completed, no second section was claimed
	gen, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)

	for _, run := range gen.SectionRuns {
		if run.SectionType == string(domain.BlueprintSectionContext) {
			assert.Equal(t, string(domain.BlueprintSectionValidated), run.Status)
		} else if run.SectionType == string(domain.BlueprintSectionJourney) {
			assert.Equal(t, string(domain.BlueprintSectionCompleted), run.Status)
			assert.Equal(t, 1, run.AttemptCount, "Journey attempt count must be exactly 1")
		} else {
			assert.Equal(t, string(domain.BlueprintSectionPending), run.Status, "section %s must remain pending", run.SectionType)
			assert.Equal(t, 0, run.AttemptCount, "section %s must not have been attempted", run.SectionType)
		}
	}
}

func TestBlueprintPipelineDoesNotPersistOrReturnSensitiveData(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	blueprintID, revisionID := seedIntegrationAggregate(t, pool, createSvc, companyID)

	ctx := context.Background()

	secretAPIKey := "SECRET_API_KEY_xyz123_leak_check"
	secretEncryptedKey := "SECRET_ENCRYPTED_CIPHER_abc789_leak_check"
	secretToken := "SECRET_TOKEN_jkl012_leak_check"
	secretRawResponse := "SECRET_RAW_RESPONSE_ghi789_leak_check"

	// Mock provider returns malformed response containing secret markers
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		leakedText := fmt.Sprintf(`{"raw": "%s", "key": "%s", "cipher": "%s", "token": "%s", invalid_json}`,
			secretRawResponse, secretAPIKey, secretEncryptedKey, secretToken)
		return service.GenerateResult{Text: leakedText}, nil
	}

	res, err := pipeline.Continue(ctx, companyID, blueprintID)
	assert.Nil(t, res)
	require.Error(t, err)

	sensitiveMarkers := []string{
		secretAPIKey,
		secretEncryptedKey,
		secretToken,
		secretRawResponse,
		"SECRET_",
	}

	// 1. Assert returned error does not leak any sensitive marker
	errStr := err.Error()
	for _, marker := range sensitiveMarkers {
		assert.False(t, strings.Contains(errStr, marker), "returned error must not contain marker: %s", marker)
	}

	// 2. Query blueprint_revisions table columns
	var contentJSON, validationResult, crossResult, sourceContext, skillSnapshot, aiProvider, aiModel *string
	err = pool.QueryRow(ctx, `
		SELECT content_json::text, validation_result::text, cross_validation_result::text,
		       source_context_snapshot::text, skill_snapshot::text, ai_provider, ai_model
		FROM blueprint_revisions
		WHERE id = $1 AND company_id = $2
	`, revisionID, companyID).Scan(
		&contentJSON, &validationResult, &crossResult,
		&sourceContext, &skillSnapshot, &aiProvider, &aiModel,
	)
	require.NoError(t, err)

	revisionCols := []*string{contentJSON, validationResult, crossResult, sourceContext, skillSnapshot, aiProvider, aiModel}
	for _, col := range revisionCols {
		if col == nil {
			continue
		}
		for _, marker := range sensitiveMarkers {
			assert.False(t, strings.Contains(*col, marker), "blueprint_revisions column must not contain marker: %s", marker)
		}
	}

	// 3. Query blueprint_section_runs table columns
	rows, err := pool.Query(ctx, `
		SELECT content_json::text, validation_result::text, safe_error_code
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND company_id = $2
	`, revisionID, companyID)
	require.NoError(t, err)
	defer rows.Close()

	for rows.Next() {
		var runContent, runValidation, runErrorCode *string
		err := rows.Scan(&runContent, &runValidation, &runErrorCode)
		require.NoError(t, err)

		runCols := []*string{runContent, runValidation, runErrorCode}
		for _, col := range runCols {
			if col == nil {
				continue
			}
			for _, marker := range sensitiveMarkers {
				assert.False(t, strings.Contains(*col, marker), "blueprint_section_runs column must not contain marker: %s", marker)
			}
		}
	}
	require.NoError(t, rows.Err())

	// 4. Marshal public DTO and verify no secret leakage
	gen, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)

	dtoResponse := dto.BlueprintStatusResponse{
		BlueprintID:        gen.Blueprint.ID,
		RevisionID:         gen.Revision.ID,
		Title:              gen.Blueprint.Title,
		GenerationStatus:   gen.Revision.GenerationStatus,
		PublicationStatus:  gen.Revision.PublicationStatus,
		CompletenessStatus: gen.Revision.CompletenessStatus,
		ContentVersion:     gen.Revision.ContentVersion,
		CreatedAt:          gen.Blueprint.CreatedAt,
		UpdatedAt:          gen.Blueprint.UpdatedAt,
	}
	for _, run := range gen.SectionRuns {
		dtoResponse.Sections = append(dtoResponse.Sections, dto.BlueprintSectionStatusResponse{
			Type:        string(run.SectionType),
			Status:      string(run.Status),
			Attempts:    run.AttemptCount,
			ErrorCode:   run.SafeErrorCode,
			StartedAt:   run.StartedAt,
			CompletedAt: run.CompletedAt,
			UpdatedAt:   run.UpdatedAt,
		})
	}

	dtoJSON, err := json.Marshal(dtoResponse)
	require.NoError(t, err)
	for _, marker := range sensitiveMarkers {
		assert.False(t, strings.Contains(string(dtoJSON), marker), "public DTO must not contain marker: %s", marker)
	}
}

func TestBlueprintPipelineManualCompleteSectionAtVersionIntegration(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	blueprintID, revisionID := seedIntegrationAggregate(t, pool, createSvc, companyID)

	ctx := context.Background()

	// 1. First advance Journey via Continue
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{Text: validJourneyIntegrationPayload()}, nil
	}
	resJourney, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resJourney)
	assert.Equal(t, 2, resJourney.ContentVersion)

	// 2. Attempt ManualCompleteSectionAtVersion with mismatched revisionID -> conflict
	mismatchedRevID := uuid.New()
	_, err = pipeline.ManualCompleteSectionAtVersion(
		ctx,
		companyID,
		blueprintID,
		domain.BlueprintSectionArchitecture,
		mismatchedRevID,
		2,
		json.RawMessage(validArchitectureIntegrationPayload()),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintExecutionConflict), "expected ErrBlueprintExecutionConflict on revision mismatch")

	// 3. Attempt ManualCompleteSectionAtVersion with mismatched expectedVersion -> conflict
	_, err = pipeline.ManualCompleteSectionAtVersion(
		ctx,
		companyID,
		blueprintID,
		domain.BlueprintSectionArchitecture,
		revisionID,
		999,
		json.RawMessage(validArchitectureIntegrationPayload()),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintExecutionConflict), "expected ErrBlueprintExecutionConflict on version mismatch")

	// 4. Successful ManualCompleteSectionAtVersion with matching revisionID and expectedVersion (2)
	resArch, err := pipeline.ManualCompleteSectionAtVersion(
		ctx,
		companyID,
		blueprintID,
		domain.BlueprintSectionArchitecture,
		revisionID,
		2,
		json.RawMessage(validArchitectureIntegrationPayload()),
	)
	require.NoError(t, err)
	require.NotNil(t, resArch)
	assert.Equal(t, domain.BlueprintSectionArchitecture, resArch.Section)
	assert.Equal(t, domain.BlueprintSectionValidated, resArch.SectionStatus)
	assert.Equal(t, 3, resArch.ContentVersion)

	// 5. Reload from DB and verify updated version and validated status
	gen, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	assert.Equal(t, 3, gen.Revision.ContentVersion)

	var archRun *domain.BlueprintSectionRun
	for i := range gen.SectionRuns {
		if gen.SectionRuns[i].SectionType == string(domain.BlueprintSectionArchitecture) {
			archRun = &gen.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, archRun)
	assert.Equal(t, string(domain.BlueprintSectionValidated), archRun.Status)

	// 6. Calling again with the stale expectedVersion (2) must fail with conflict
	_, err = pipeline.ManualCompleteSectionAtVersion(
		ctx,
		companyID,
		blueprintID,
		domain.BlueprintSectionArchitecture,
		revisionID,
		2,
		json.RawMessage(validArchitectureIntegrationPayload()),
	)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintExecutionConflict), "expected conflict on stale version")
}

func seedIntegrationBlueprintWithManualJourneyAtAttempt3(
	t *testing.T,
	pool *pgxpool.Pool,
	pipeline *service.BlueprintPipeline,
	repo *repository.BlueprintGenerationRepository,
	createSvc *service.BlueprintGenerationService,
	provider *pipelineScriptedAIProvider,
	companyID uuid.UUID,
) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	blueprintID, revisionID := seedIntegrationAggregate(t, pool, createSvc, companyID)

	// Attempt 1: Journey fails via Continue with provider unavailable
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{}, &service.ProviderError{
			Code:           service.ErrProviderUnavailable,
			UpstreamStatus: 503,
			Message:        "upstream unavailable",
		}
	}
	_, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBlueprintProviderUnavailable))

	// Attempt 2: Journey fails via RetrySection with provider timeout
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{}, &service.ProviderError{
			Code:           service.ErrTimeout,
			UpstreamStatus: 504,
			Message:        "upstream timeout",
		}
	}
	_, err = pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionJourney)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBlueprintProviderTimeout))

	// Attempt 3: Journey fails via RetrySection with provider rate limit
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{}, &service.ProviderError{
			Code:           service.ErrRateLimited,
			UpstreamStatus: 429,
			Message:        "rate limited",
		}
	}
	_, err = pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionJourney)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBlueprintProviderRateLimited))

	// Attempt 4: Exceeding 3 provider attempts must be rejected
	_, err = pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionJourney)
	require.Error(t, err)
	require.True(t, errors.Is(err, service.ErrBlueprintAttemptLimitReached))

	// Manually complete Journey at expected version 1 with valid payload
	manualPayload := json.RawMessage(validJourneyIntegrationPayload())
	resManual, err := pipeline.ManualCompleteSectionAtVersion(
		ctx,
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		manualPayload,
	)
	require.NoError(t, err)
	require.NotNil(t, resManual)
	require.Equal(t, domain.BlueprintSectionJourney, resManual.Section)
	require.Equal(t, domain.BlueprintSectionValidated, resManual.SectionStatus)
	require.Equal(t, 2, resManual.ContentVersion)
	require.Equal(t, 3, resManual.Attempt)

	return blueprintID, revisionID
}

func TestBlueprintPipelineManualJourneyInvariantsIntegration(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	ctx := context.Background()

	blueprintID, revisionID := seedIntegrationAggregate(t, pool, createSvc, companyID)

	// 1. Initial snapshot check
	genInit, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	require.Equal(t, 1, genInit.Revision.ContentVersion)
	require.Equal(t, string(domain.BlueprintGenerationPending), genInit.Revision.GenerationStatus)

	// 2. Journey Attempt 1 -> fails with invalid credential
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{}, &service.ProviderError{
			Code:           service.ErrInvalidCredential,
			UpstreamStatus: 401,
			Message:        "invalid api key",
		}
	}
	res1, err := pipeline.Continue(ctx, companyID, blueprintID)
	assert.Nil(t, res1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintProviderInvalidCredential))

	// DB check after attempt 1: status failed, attempt 1, safe_error_code provider_invalid_credential
	gen1, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	var journeyRun1 *domain.BlueprintSectionRun
	for i := range gen1.SectionRuns {
		if gen1.SectionRuns[i].SectionType == string(domain.BlueprintSectionJourney) {
			journeyRun1 = &gen1.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, journeyRun1)
	assert.Equal(t, string(domain.BlueprintSectionFailed), journeyRun1.Status)
	assert.Equal(t, 1, journeyRun1.AttemptCount)
	require.NotNil(t, journeyRun1.SafeErrorCode)
	assert.Equal(t, "provider_invalid_credential", *journeyRun1.SafeErrorCode)
	assert.Equal(t, 1, gen1.Revision.ContentVersion)

	// 3. Journey Attempt 2 -> fails with configuration / unsupported model
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{}, &service.ProviderError{
			Code:           service.ErrUnsupportedModel,
			UpstreamStatus: 404,
			Message:        "model not found",
		}
	}
	res2, err := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionJourney)
	assert.Nil(t, res2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintProviderConfiguration))

	// DB check after attempt 2: attempt 2, safe_error_code provider_configuration
	gen2, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	var journeyRun2 *domain.BlueprintSectionRun
	for i := range gen2.SectionRuns {
		if gen2.SectionRuns[i].SectionType == string(domain.BlueprintSectionJourney) {
			journeyRun2 = &gen2.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, journeyRun2)
	assert.Equal(t, string(domain.BlueprintSectionFailed), journeyRun2.Status)
	assert.Equal(t, 2, journeyRun2.AttemptCount)
	require.NotNil(t, journeyRun2.SafeErrorCode)
	assert.Equal(t, "provider_configuration", *journeyRun2.SafeErrorCode)

	// 4. Journey Attempt 3 -> fails with provider unavailable
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{}, &service.ProviderError{
			Code:           service.ErrProviderUnavailable,
			UpstreamStatus: 503,
			Message:        "service unavailable",
		}
	}
	res3, err := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionJourney)
	assert.Nil(t, res3)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintProviderUnavailable))

	// DB check after attempt 3: attempt 3, safe_error_code provider_unavailable, revision status failed
	gen3, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	var journeyRun3 *domain.BlueprintSectionRun
	for i := range gen3.SectionRuns {
		if gen3.SectionRuns[i].SectionType == string(domain.BlueprintSectionJourney) {
			journeyRun3 = &gen3.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, journeyRun3)
	assert.Equal(t, string(domain.BlueprintSectionFailed), journeyRun3.Status)
	assert.Equal(t, 3, journeyRun3.AttemptCount)
	require.NotNil(t, journeyRun3.SafeErrorCode)
	assert.Equal(t, "provider_unavailable", *journeyRun3.SafeErrorCode)
	assert.Equal(t, string(domain.BlueprintGenerationFailed), gen3.Revision.GenerationStatus)

	// 5. Journey Attempt 4 -> blocked by attempt limit
	res4, err := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionJourney)
	assert.Nil(t, res4)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintAttemptLimitReached))

	// 6. Manually complete Journey with valid content at expectedVersion 1
	resManual, err := pipeline.ManualCompleteSectionAtVersion(
		ctx,
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyIntegrationPayload()),
	)
	require.NoError(t, err)
	require.NotNil(t, resManual)
	assert.Equal(t, domain.BlueprintSectionJourney, resManual.Section)
	assert.Equal(t, domain.BlueprintSectionValidated, resManual.SectionStatus)
	assert.Equal(t, 2, resManual.ContentVersion)
	assert.Equal(t, 3, resManual.Attempt, "Attempt count must remain 3 after manual completion")

	// 7. Assert manual completion invariants in DB
	genAfterManual, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	assert.Equal(t, 2, genAfterManual.Revision.ContentVersion)
	assert.Equal(t, string(domain.BlueprintGenerationInProgress), genAfterManual.Revision.GenerationStatus)
	assert.Contains(t, string(genAfterManual.Revision.ContentJSON), "Discovery")

	for _, run := range genAfterManual.SectionRuns {
		switch run.SectionType {
		case string(domain.BlueprintSectionContext):
			assert.Equal(t, string(domain.BlueprintSectionValidated), run.Status)
		case string(domain.BlueprintSectionJourney):
			assert.Equal(t, string(domain.BlueprintSectionValidated), run.Status, "Journey must be validated")
			assert.Nil(t, run.SafeErrorCode, "safe_error_code must be NULL/empty after manual completion")
			assert.Equal(t, 3, run.AttemptCount, "attempt count must remain exactly 3")
			assert.NotEmpty(t, run.ContentJSON, "content_json must be populated")
			assert.Contains(t, string(run.ContentJSON), "Discovery")
		default:
			// Downstream sections must be pending, 0 attempts, and empty content
			assert.Equal(t, string(domain.BlueprintSectionPending), run.Status, "Downstream %s must be pending", run.SectionType)
			assert.Equal(t, 0, run.AttemptCount, "Downstream %s attempt count must be 0", run.SectionType)
			assert.Nil(t, run.SafeErrorCode, "Downstream %s safe_error_code must be nil", run.SectionType)
			assert.True(t, len(run.ContentJSON) == 0 || string(run.ContentJSON) == "null" || string(run.ContentJSON) == "{}", "Downstream %s content must be empty", run.SectionType)
		}
	}
}

func TestBlueprintPipelineManualJourneyToArchitecturePromptConstructionIntegration(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	ctx := context.Background()

	blueprintID, _ := seedIntegrationBlueprintWithManualJourneyAtAttempt3(
		t, pool, pipeline, repo, createSvc, provider, companyID,
	)

	// Intercept provider generate request to verify Architecture prompt construction
	var capturedPrompt string
	var capturedStage domain.BlueprintSectionType
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		capturedPrompt = req.Prompt
		capturedStage = detectStageFromPrompt(req.Prompt)
		return service.GenerateResult{Text: validArchitectureIntegrationPayload()}, nil
	}

	resArch, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resArch)
	assert.Equal(t, domain.BlueprintSectionArchitecture, resArch.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resArch.SectionStatus)
	assert.Equal(t, 3, resArch.ContentVersion)
	assert.Equal(t, 1, resArch.Attempt)

	// Assert prompt construction invariants
	assert.NotEmpty(t, capturedPrompt, "Architecture prompt must not be empty")
	assert.True(t, strings.Contains(capturedPrompt, `stage="architecture"`), "Prompt must include architecture stage marker")
	assert.True(t, strings.Contains(capturedPrompt, "Discovery") || strings.Contains(capturedPrompt, "Initial Meeting"),
		"Architecture prompt must contain manually completed Journey dependency content")
	assert.Equal(t, domain.BlueprintSectionArchitecture, capturedStage)

	// Non-secret size and routing properties
	assert.Greater(t, len(capturedPrompt), 200, "Prompt must contain structured context and dependency data")
	assert.False(t, strings.Contains(capturedPrompt, "SECRET_"), "Prompt must not contain secret tokens")
}

func TestBlueprintPipelineManualJourneyToArchitectureProviderFailureCategoriesIntegration(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)

	tests := []struct {
		name                 string
		providerErr          error
		expectedSentinel     error
		expectedSafeCode     string
		isRetryable          bool
		expectedRetryOutcome string
	}{
		{
			name: "RequestRejected_NonRetryable",
			providerErr: &service.ProviderError{
				Code:           service.ErrRequestRejected,
				UpstreamStatus: 400,
				Message:        "upstream bad request / schema rejection",
				Retryable:      false,
			},
			expectedSentinel:     service.ErrBlueprintProviderRejected,
			expectedSafeCode:     "provider_request_rejected",
			isRetryable:          false,
			expectedRetryOutcome: "retry_not_allowed",
		},
		{
			name: "PayloadTooLarge_NonRetryable",
			providerErr: &service.ProviderError{
				Code:           service.ErrPayloadTooLarge,
				UpstreamStatus: 413,
				Message:        "upstream prompt too large",
				Retryable:      false,
			},
			expectedSentinel:     service.ErrBlueprintProviderPayloadSize,
			expectedSafeCode:     "provider_payload_too_large",
			isRetryable:          false,
			expectedRetryOutcome: "retry_not_allowed",
		},
		{
			name: "ProviderUnavailable_Retryable",
			providerErr: &service.ProviderError{
				Code:           service.ErrProviderUnavailable,
				UpstreamStatus: 503,
				Message:        "upstream service unavailable",
				Retryable:      true,
			},
			expectedSentinel:     service.ErrBlueprintProviderUnavailable,
			expectedSafeCode:     "provider_unavailable",
			isRetryable:          true,
			expectedRetryOutcome: "retry_allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			companyID := uuid.New()
			cleanupIntegrationTenant(t, pool, companyID)
			t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

			provider := newPipelineScriptedAIProvider()
			provSource := &pipelineScriptedProviderSource{provider: provider}

			pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
			ctx := context.Background()

			blueprintID, _ := seedIntegrationBlueprintWithManualJourneyAtAttempt3(
				t, pool, pipeline, repo, createSvc, provider, companyID,
			)

			// 1. Continue into Architecture -> fails with the specific provider error
			provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
				return service.GenerateResult{}, tt.providerErr
			}

			res, err := pipeline.Continue(ctx, companyID, blueprintID)
			assert.Nil(t, res)
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.expectedSentinel), "expected error sentinel %v, got %v", tt.expectedSentinel, err)

			// 2. Verify persisted safe error code in DB
			genAfterFail, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
			require.NoError(t, err)
			assert.Equal(t, 2, genAfterFail.Revision.ContentVersion, "failure must not increment content version")
			assert.Equal(t, string(domain.BlueprintGenerationPartialFailed), genAfterFail.Revision.GenerationStatus)

			var archRun *domain.BlueprintSectionRun
			for i := range genAfterFail.SectionRuns {
				if genAfterFail.SectionRuns[i].SectionType == string(domain.BlueprintSectionArchitecture) {
					archRun = &genAfterFail.SectionRuns[i]
					break
				}
			}
			require.NotNil(t, archRun)
			assert.Equal(t, string(domain.BlueprintSectionFailed), archRun.Status)
			assert.Equal(t, 1, archRun.AttemptCount)
			require.NotNil(t, archRun.SafeErrorCode)
			assert.Equal(t, tt.expectedSafeCode, *archRun.SafeErrorCode)

			// 3. Test retry eligibility
			if tt.isRetryable {
				// Retry should succeed to claim and execute attempt 2
				provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
					return service.GenerateResult{Text: validArchitectureIntegrationPayload()}, nil
				}
				retryRes, retryErr := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionArchitecture)
				require.NoError(t, retryErr)
				require.NotNil(t, retryRes)
				assert.Equal(t, domain.BlueprintSectionArchitecture, retryRes.Section)
				assert.Equal(t, domain.BlueprintSectionCompleted, retryRes.SectionStatus)
				assert.Equal(t, 2, retryRes.Attempt, "Attempt must increment to 2 on retry")
				assert.Equal(t, 3, retryRes.ContentVersion, "Content version must increment to 3 on completion")
			} else {
				// Non-retryable error: RetrySection must return ErrBlueprintRetryNotAllowed
				retryRes, retryErr := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionArchitecture)
				assert.Nil(t, retryRes)
				require.Error(t, retryErr)
				assert.True(t, errors.Is(retryErr, service.ErrBlueprintRetryNotAllowed),
					"expected ErrBlueprintRetryNotAllowed for %s, got: %v", tt.expectedSafeCode, retryErr)

				// DB check: attempt must still be 1, status still failed
				genAfterBlockedRetry, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
				require.NoError(t, err)
				var archRunBlocked *domain.BlueprintSectionRun
				for i := range genAfterBlockedRetry.SectionRuns {
					if genAfterBlockedRetry.SectionRuns[i].SectionType == string(domain.BlueprintSectionArchitecture) {
						archRunBlocked = &genAfterBlockedRetry.SectionRuns[i]
						break
					}
				}
				require.NotNil(t, archRunBlocked)
				assert.Equal(t, 1, archRunBlocked.AttemptCount, "Attempt count must not increment on blocked retry")
				assert.Equal(t, string(domain.BlueprintSectionFailed), archRunBlocked.Status)
			}
		})
	}
}

func TestBlueprintPipelineManualJourneyToArchitectureSuccessAndRetryGuardIntegration(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	ctx := context.Background()

	blueprintID, _ := seedIntegrationBlueprintWithManualJourneyAtAttempt3(
		t, pool, pipeline, repo, createSvc, provider, companyID,
	)

	// Continue into Architecture -> success
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{Text: validArchitectureIntegrationPayload()}, nil
	}

	resArch, err := pipeline.Continue(ctx, companyID, blueprintID)
	require.NoError(t, err)
	require.NotNil(t, resArch)
	assert.Equal(t, domain.BlueprintSectionArchitecture, resArch.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resArch.SectionStatus)
	assert.Equal(t, 3, resArch.ContentVersion)
	assert.Equal(t, 1, resArch.Attempt)

	// Attempting RetrySection on completed Architecture must fail with ErrBlueprintRetryNotAllowed
	resRetry, err := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionArchitecture)
	assert.Nil(t, resRetry)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintRetryNotAllowed))
}

func TestBlueprintPipelineManualJourneyToArchitectureStaleLeaseRecoveryIntegration(t *testing.T) {
	pool := getBlueprintIntegrationPool(t)
	companyID := uuid.New()
	cleanupIntegrationTenant(t, pool, companyID)
	t.Cleanup(func() { cleanupIntegrationTenant(t, pool, companyID) })

	provider := newPipelineScriptedAIProvider()
	provSource := &pipelineScriptedProviderSource{provider: provider}

	pipeline, repo, createSvc := setupIntegrationPipeline(t, pool, provSource)
	ctx := context.Background()

	blueprintID, revisionID := seedIntegrationBlueprintWithManualJourneyAtAttempt3(
		t, pool, pipeline, repo, createSvc, provider, companyID,
	)

	// 1. Simulate Architecture entering 'generating' state and becoming stale (> 5 minutes ago)
	staleTime := time.Now().Add(-10 * time.Minute)
	_, err := pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'generating', attempt_count = 1, started_at = $1, updated_at = $1
		WHERE blueprint_revision_id = $2 AND company_id = $3 AND section_type = 'architecture'
	`, staleTime, revisionID, companyID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE blueprint_revisions
		SET generation_status = 'in_progress', updated_at = $1
		WHERE id = $2 AND company_id = $3
	`, staleTime, revisionID, companyID)
	require.NoError(t, err)

	provCallsBeforeContinue := provider.calls

	// 2. Calling Continue once triggers stale lease reconciliation
	resRecov, err := pipeline.Continue(ctx, companyID, blueprintID)
	assert.Nil(t, resRecov)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintPipelineRecoveryRequired),
		"Expected ErrBlueprintPipelineRecoveryRequired on stale lease, got: %v", err)

	// Assert reconciliation did NOT invoke the AI provider
	assert.Equal(t, provCallsBeforeContinue, provider.calls, "Stale lease reconciliation must make ZERO provider calls")

	// 3. Check DB state after reconciliation: Architecture is failed, safe_error_code is interrupted, attempt remains 1
	genAfterRecov, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	assert.Equal(t, 2, genAfterRecov.Revision.ContentVersion, "Reconciliation must not increment content version")
	assert.Equal(t, string(domain.BlueprintGenerationPartialFailed), genAfterRecov.Revision.GenerationStatus)

	var archRunRecov *domain.BlueprintSectionRun
	for i := range genAfterRecov.SectionRuns {
		if genAfterRecov.SectionRuns[i].SectionType == string(domain.BlueprintSectionArchitecture) {
			archRunRecov = &genAfterRecov.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, archRunRecov)
	assert.Equal(t, string(domain.BlueprintSectionFailed), archRunRecov.Status)
	assert.Equal(t, 1, archRunRecov.AttemptCount, "Reconciliation must NOT increment attempt count")
	require.NotNil(t, archRunRecov.SafeErrorCode)
	assert.Equal(t, "interrupted", *archRunRecov.SafeErrorCode)

	// Calling Continue again while Architecture is failed should return ErrBlueprintPipelineBlocked (no eligible section)
	resBlocked, err := pipeline.Continue(ctx, companyID, blueprintID)
	assert.Nil(t, resBlocked)
	require.Error(t, err)
	assert.True(t, errors.Is(err, service.ErrBlueprintPipelineBlocked))

	// 4. Calling explicit RetrySection consumes attempt 2 and succeeds
	provider.generateFn = func(ctx context.Context, req service.GenerateRequest) (service.GenerateResult, error) {
		return service.GenerateResult{Text: validArchitectureIntegrationPayload()}, nil
	}

	resRetry, err := pipeline.RetrySection(ctx, companyID, blueprintID, domain.BlueprintSectionArchitecture)
	require.NoError(t, err)
	require.NotNil(t, resRetry)
	assert.Equal(t, domain.BlueprintSectionArchitecture, resRetry.Section)
	assert.Equal(t, domain.BlueprintSectionCompleted, resRetry.SectionStatus)
	assert.Equal(t, 2, resRetry.Attempt, "Attempt must increment to exactly 2 on explicit RetrySection")
	assert.Equal(t, 3, resRetry.ContentVersion, "Content version must increment to 3 on completion")

	// 5. Final DB state check
	finalGen, err := repo.GetBlueprintGeneration(ctx, blueprintID, companyID)
	require.NoError(t, err)
	assert.Equal(t, 3, finalGen.Revision.ContentVersion)

	var finalArchRun *domain.BlueprintSectionRun
	for i := range finalGen.SectionRuns {
		if finalGen.SectionRuns[i].SectionType == string(domain.BlueprintSectionArchitecture) {
			finalArchRun = &finalGen.SectionRuns[i]
			break
		}
	}
	require.NotNil(t, finalArchRun)
	assert.Equal(t, string(domain.BlueprintSectionCompleted), finalArchRun.Status)
	assert.Equal(t, 2, finalArchRun.AttemptCount)
	assert.Nil(t, finalArchRun.SafeErrorCode)
	assert.NotEmpty(t, finalArchRun.ContentJSON)
}

