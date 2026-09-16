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
