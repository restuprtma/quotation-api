package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakeDraftRepo struct {
	mu sync.Mutex

	loadSnapshotCalls int
	loadSnapshotFn    func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error)

	saveDraftCalls  int
	saveDraftParams []repository.SaveBlueprintDraftParams
	saveDraftFn     func(ctx context.Context, params repository.SaveBlueprintDraftParams) (*repository.BlueprintSectionPersistenceResult, error)
}

func (f *fakeDraftRepo) LoadBlueprintDraftSnapshot(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loadSnapshotCalls++
	if f.loadSnapshotFn != nil {
		return f.loadSnapshotFn(ctx, companyID, blueprintID, sectionType)
	}
	return nil, repository.ErrBlueprintNotFound
}

func (f *fakeDraftRepo) SaveBlueprintDraft(ctx context.Context, params repository.SaveBlueprintDraftParams) (*repository.BlueprintSectionPersistenceResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saveDraftCalls++
	f.saveDraftParams = append(f.saveDraftParams, params)
	if f.saveDraftFn != nil {
		return f.saveDraftFn(ctx, params)
	}
	return &repository.BlueprintSectionPersistenceResult{
		RevisionID:       params.Snapshot.RevisionID,
		SectionType:      params.Snapshot.SectionType,
		SectionStatus:    domain.BlueprintSectionManualEdit,
		GenerationStatus: domain.BlueprintGenerationInProgress,
		ContentVersion:   params.Snapshot.ExpectedContentVersion + 1,
		Attempt:          1,
	}, nil
}

func defaultDraftSnapshot(companyID, blueprintID, revisionID uuid.UUID) *repository.BlueprintDraftSnapshot {
	return &repository.BlueprintDraftSnapshot{
		BlueprintID:            blueprintID,
		RevisionID:             revisionID,
		SectionRunID:           uuid.New(),
		CompanyID:              companyID,
		SectionType:            domain.BlueprintSectionJourney,
		SectionStatus:          domain.BlueprintSectionPending,
		ExpectedContentVersion: 1,
		BlueprintTitle:         "Queue Management System Blueprint",
		RevisionContent:        json.RawMessage(`{"schema_version":"1.0","title":"Queue Management System Blueprint","summary":"Queue Blueprint"}`),
		ContextContent:         json.RawMessage(`{"summary":"Queue Blueprint Context"}`),
	}
}

func validJourneyDraftPayload() string {
	return `{
		"journey": {
			"phases": [
				{"id": "phase-1", "label": "Registration & Booking", "order": 1},
				{"id": "phase-2", "label": "Onsite Check-in", "order": 2}
			],
			"actors": [
				{"id": "actor-1", "label": "Citizen / Applicant", "order": 1},
				{"id": "actor-2", "label": "Counter Officer", "order": 2}
			],
			"activities": [
				{
					"id": "act-1",
					"phase_id": "phase-1",
					"actor_id": "actor-1",
					"label": "Online Booking",
					"description": "Applicant books appointment via portal",
					"touchpoints": ["Web Portal"],
					"references": []
				},
				{
					"id": "act-2",
					"phase_id": "phase-2",
					"actor_id": "actor-1",
					"label": "Kiosk Check-in",
					"description": "Applicant scans QR code at kiosk",
					"touchpoints": ["Kiosk Machine"],
					"references": []
				}
			]
		}
	}`
}

func incompleteJourneyDraftPayload() string {
	return `{
		"journey": {
			"phases": [
				{"id": "phase-1", "label": "Registration & Booking", "order": 1}
			],
			"actors": [
				{"id": "actor-1", "label": "Citizen", "order": 1}
			],
			"activities": [
				{
					"id": "act-1",
					"phase_id": "phase-missing-99",
					"actor_id": "actor-missing-99",
					"label": "Unlinked Activity",
					"description": "Draft in progress with unlinked actor and phase",
					"touchpoints": [],
					"references": []
				}
			]
		}
	}`
}

// 1. Valid Journey Draft Test
func TestBlueprintDraftService_SaveDraft_Journey_Valid(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			assert.Equal(t, companyID, cID)
			assert.Equal(t, blueprintID, bpID)
			assert.Equal(t, domain.BlueprintSectionJourney, sType)
			return defaultDraftSnapshot(companyID, blueprintID, revisionID), nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	res, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, blueprintID, res.BlueprintID)
	assert.Equal(t, revisionID, res.RevisionID)
	assert.Equal(t, domain.BlueprintSectionJourney, res.Section)
	assert.Equal(t, domain.BlueprintSectionManualEdit, res.SectionStatus, "Draft save must set status to manual_edit, not validated")
	assert.Equal(t, domain.BlueprintGenerationInProgress, res.GenerationStatus)
	assert.Equal(t, 2, res.ContentVersion, "Content version must be incremented exactly once")

	require.Equal(t, 1, repo.saveDraftCalls)
	saved := repo.saveDraftParams[0]
	assert.Contains(t, string(saved.RevisionContent), "phase-1")
	assert.Contains(t, string(saved.RevisionContent), "Online Booking")
}

// 2. Incomplete Journey Draft with issues is saved as manual_edit
func TestBlueprintDraftService_SaveDraft_Journey_IncompleteWithIssues(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			return defaultDraftSnapshot(companyID, blueprintID, revisionID), nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	res, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(incompleteJourneyDraftPayload()),
	)

	require.NoError(t, err, "Incomplete draft must be saved successfully without blocking BA work")
	require.NotNil(t, res)
	assert.Equal(t, domain.BlueprintSectionManualEdit, res.SectionStatus, "Incomplete draft must be manual_edit")
	assert.Equal(t, 2, res.ContentVersion)

	require.Equal(t, 1, repo.saveDraftCalls)
	saved := repo.saveDraftParams[0]
	var validationRes BlueprintValidationResult
	err = json.Unmarshal(saved.SectionValidation, &validationRes)
	require.NoError(t, err)
	assert.False(t, validationRes.Valid, "Validation result for incomplete draft must indicate valid: false")
	assert.NotEmpty(t, validationRes.Issues, "Validation result must record structural/reference issues")
}

// 3. 409 Conflict: Version and Revision ID mismatch
func TestBlueprintDraftService_SaveDraft_Conflict_VersionAndRevisionMismatch(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	actualRevisionID := uuid.New()
	staleRevisionID := uuid.New()

	snapshot := defaultDraftSnapshot(companyID, blueprintID, actualRevisionID)
	snapshot.ExpectedContentVersion = 3

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			return snapshot, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	// (a) Stale version (expected 2 while server is at 3)
	_, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		actualRevisionID,
		2,
		json.RawMessage(validJourneyDraftPayload()),
	)
	assert.ErrorIs(t, err, ErrBlueprintExecutionConflict, "Stale content_version must return 409 execution_conflict")
	assert.Equal(t, 0, repo.saveDraftCalls, "Zero DB writes on 409 conflict")

	// (b) Mismatched revision ID
	_, err = service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		staleRevisionID,
		3,
		json.RawMessage(validJourneyDraftPayload()),
	)
	assert.ErrorIs(t, err, ErrBlueprintExecutionConflict, "Wrong revision_id must return 409 execution_conflict")
	assert.Equal(t, 0, repo.saveDraftCalls, "Zero DB writes on 409 conflict")
}

// 4. Foreign tenant isolation: returns 404 Not Found
func TestBlueprintDraftService_SaveDraft_ForeignTenant_ReturnsNotFound(t *testing.T) {
	companyA := uuid.New()
	foreignCompanyB := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			if cID != companyA {
				return nil, repository.ErrBlueprintNotFound
			}
			return defaultDraftSnapshot(companyA, blueprintID, revisionID), nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	_, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		foreignCompanyB,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)

	assert.ErrorIs(t, err, ErrBlueprintPipelineNotFound, "Accessing foreign company draft must return 404 without leaking existence")
	assert.Equal(t, 0, repo.saveDraftCalls, "Zero writes on foreign tenant rejection")
}

// 5. 422 Shape and Security Rejections
func TestBlueprintDraftService_SaveDraft_422ShapeAndSecurity_Rejection(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			return defaultDraftSnapshot(companyID, blueprintID, revisionID), nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	// (a) Invalid root shape (non-journey payload)
	malformedShape := json.RawMessage(`{"unknown_section":{"phases":[]}}`)
	_, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		malformedShape,
	)
	assert.ErrorIs(t, err, ErrBlueprintInvalidSchema, "Malformed/unsupported section shape must return 422 invalid_schema")
	assert.Equal(t, 0, repo.saveDraftCalls, "Zero writes on shape rejection")

	// (b) Forbidden security keys
	securityPayload := json.RawMessage(`{
		"journey": {
			"phases": [{"id": "phase-1", "label": "Start", "order": 1}],
			"actors": [{"id": "actor-1", "label": "User", "order": 1}],
			"activities": []
		},
		"apiKey": "super-secret-leak",
		"budget": 50000000
	}`)
	_, err = service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		securityPayload,
	)
	assert.ErrorIs(t, err, ErrBlueprintSecurityFailure, "Forbidden keys must trigger 422 security_failure")
	assert.Equal(t, 0, repo.saveDraftCalls, "Zero writes on security rejection")
}

// 6. 409 Conflict: Run is currently generating
func TestBlueprintDraftService_SaveDraft_Conflict_GeneratingRun(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	generatingSnapshot := defaultDraftSnapshot(companyID, blueprintID, revisionID)
	generatingSnapshot.SectionStatus = domain.BlueprintSectionGenerating

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			return generatingSnapshot, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	_, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)

	assert.ErrorIs(t, err, ErrBlueprintExecutionConflict, "Attempting to save draft during section generation must return 409 execution_conflict")
	assert.Equal(t, 0, repo.saveDraftCalls, "Zero writes on generating conflict")
}

// 7. Two concurrent writers: Optimistic Concurrency Control (CAS)
func TestBlueprintDraftService_SaveDraft_TwoConcurrentWriters_CAS(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	currentVersion := 1
	var repoMu sync.Mutex

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			repoMu.Lock()
			v := currentVersion
			repoMu.Unlock()
			snap := defaultDraftSnapshot(companyID, blueprintID, revisionID)
			snap.ExpectedContentVersion = v
			return snap, nil
		},
		saveDraftFn: func(ctx context.Context, params repository.SaveBlueprintDraftParams) (*repository.BlueprintSectionPersistenceResult, error) {
			repoMu.Lock()
			defer repoMu.Unlock()
			if params.Snapshot.ExpectedContentVersion != currentVersion {
				return nil, repository.ErrBlueprintContentConflict
			}
			currentVersion++
			return &repository.BlueprintSectionPersistenceResult{
				RevisionID:       revisionID,
				SectionType:      domain.BlueprintSectionJourney,
				SectionStatus:    domain.BlueprintSectionManualEdit,
				GenerationStatus: domain.BlueprintGenerationInProgress,
				ContentVersion:   currentVersion,
				Attempt:          1,
			}, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	// Writer 1 starts at version 1 and saves
	res1, err1 := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)
	require.NoError(t, err1)
	assert.Equal(t, 2, res1.ContentVersion)

	// Writer 2 was holding version 1 and attempts to save -> 409 Conflict
	_, err2 := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)
	assert.ErrorIs(t, err2, ErrBlueprintExecutionConflict, "Concurrent writer with stale version must receive 409 execution_conflict")
}

// 8. Downstream Invalidation on Journey draft save
func TestBlueprintDraftService_SaveDraft_DownstreamInvalidation(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			return defaultDraftSnapshot(companyID, blueprintID, revisionID), nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	_, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)

	require.NoError(t, err)
	require.Equal(t, 1, repo.saveDraftCalls)

	saved := repo.saveDraftParams[0]
	assert.ElementsMatch(t, []domain.BlueprintSectionType{
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}, saved.InvalidatedSections, "Saving Journey draft must invalidate all downstream sections")
}

// 9. No provider / AI LLM call during Save Draft
func TestBlueprintDraftService_SaveDraft_NoProviderCall(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	providerMock := &fakeAIProvider{
		generateFn: func(ctx context.Context, req GenerateRequest) (GenerateResult, error) {
			t.Fatalf("Provider Generate must NOT be called during Save Draft")
			return GenerateResult{}, errors.New("unexpected provider call")
		},
	}
	provSource := &fakeProviderSource{
		providerFn: func(ctx context.Context, cID uuid.UUID) (AIProvider, string, string, error) {
			return providerMock, "fake-provider", "fake-model", nil
		},
	}

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			return defaultDraftSnapshot(companyID, blueprintID, revisionID), nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	_, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionJourney,
		revisionID,
		1,
		json.RawMessage(validJourneyDraftPayload()),
	)

	require.NoError(t, err)
	assert.Equal(t, 0, providerMock.calls, "AI provider generate calls must be 0")
}

// 10. Architecture Draft Save
func TestBlueprintDraftService_SaveDraft_Architecture_IncompleteWithIssues(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			snap := defaultDraftSnapshot(companyID, blueprintID, revisionID)
			snap.SectionType = domain.BlueprintSectionArchitecture
			return snap, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	archPayload := `{
		"architecture": {
			"groups": [{"id": "grp-1", "label": "Tier 1", "order": 1}],
			"nodes": [{"id": "node-1", "group_id": "grp-missing", "type": "service", "label": "Service 1", "description": "", "layout": {"x": 0, "y": 0}}],
			"edges": [{"id": "edge-1", "source": "node-1", "target": "node-missing", "label": "", "type": "sync"}]
		}
	}`

	res, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionArchitecture,
		revisionID,
		1,
		json.RawMessage(archPayload),
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, domain.BlueprintSectionManualEdit, res.SectionStatus)
	assert.Equal(t, 2, res.ContentVersion)

	require.Equal(t, 1, repo.saveDraftCalls)
	saved := repo.saveDraftParams[0]
	assert.ElementsMatch(t, []domain.BlueprintSectionType{
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}, saved.InvalidatedSections)
}

// 11. State Machine Draft Save
func TestBlueprintDraftService_SaveDraft_StateMachine_IncompleteWithIssues(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			snap := defaultDraftSnapshot(companyID, blueprintID, revisionID)
			snap.SectionType = domain.BlueprintSectionStateMachine
			return snap, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	smPayload := `{
		"state_machine": {
			"entity": "Document",
			"initial_state": "state-missing",
			"states": [{"id": "state-draft", "label": "Draft", "type": "normal", "layout": {"x": 0, "y": 0}}],
			"transitions": [{"id": "trans-1", "source": "state-draft", "target": "state-missing", "event": "SUBMIT", "actor": "", "guard": "", "actions": []}]
		}
	}`

	res, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionStateMachine,
		revisionID,
		1,
		json.RawMessage(smPayload),
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, domain.BlueprintSectionManualEdit, res.SectionStatus)
	assert.Equal(t, 2, res.ContentVersion)

	require.Equal(t, 1, repo.saveDraftCalls)
	saved := repo.saveDraftParams[0]
	assert.ElementsMatch(t, []domain.BlueprintSectionType{
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}, saved.InvalidatedSections)
}

// 12. Workflow Draft Save
func TestBlueprintDraftService_SaveDraft_Workflow_IncompleteWithIssues(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			snap := defaultDraftSnapshot(companyID, blueprintID, revisionID)
			snap.SectionType = domain.BlueprintSectionWorkflow
			return snap, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	wfPayload := `{
		"workflows": [{
			"id": "wf-1",
			"name": "Process Flow",
			"description": "",
			"lanes": [{"id": "lane-1", "label": "Lane 1", "order": 1}],
			"nodes": [
				{"id": "node-dec", "lane_id": "lane-1", "type": "decision", "label": "Decide", "description": "", "layout": {"x": 0, "y": 0}},
				{"id": "node-act", "lane_id": "lane-1", "type": "action", "label": "Action", "description": "", "layout": {"x": 50, "y": 0}}
			],
			"edges": [
				{"id": "edge-1", "source": "node-dec", "target": "node-act", "label": "Branch 1"}
			]
		}]
	}`

	res, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionWorkflow,
		revisionID,
		1,
		json.RawMessage(wfPayload),
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, domain.BlueprintSectionManualEdit, res.SectionStatus)
	assert.Equal(t, 2, res.ContentVersion)

	require.Equal(t, 1, repo.saveDraftCalls)
	saved := repo.saveDraftParams[0]
	assert.ElementsMatch(t, []domain.BlueprintSectionType{
		domain.BlueprintSectionWBS,
		domain.BlueprintSectionCrossValidation,
	}, saved.InvalidatedSections)
}

// 13. WBS Draft Save
func TestBlueprintDraftService_SaveDraft_WBS_CycleWithIssues(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	revisionID := uuid.New()

	repo := &fakeDraftRepo{
		loadSnapshotFn: func(ctx context.Context, cID, bpID uuid.UUID, sType domain.BlueprintSectionType) (*repository.BlueprintDraftSnapshot, error) {
			snap := defaultDraftSnapshot(companyID, blueprintID, revisionID)
			snap.SectionType = domain.BlueprintSectionWBS
			return snap, nil
		},
	}
	validator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	provSource := &fakeProviderSource{}

	service := NewBlueprintDraftService(repo, validator, crossValidator, provSource)

	wbsPayload := `{
		"wbs": {
			"modules": [
				{"id": "mod-a", "name": "Module A", "description": "", "priority": "high", "dependencies": ["mod-b"], "references": [], "features": []},
				{"id": "mod-b", "name": "Module B", "description": "", "priority": "high", "dependencies": ["mod-a"], "references": [], "features": []}
			]
		},
		"cross_references": []
	}`

	res, err := service.SaveDraftSectionAtVersion(
		context.Background(),
		companyID,
		blueprintID,
		domain.BlueprintSectionWBS,
		revisionID,
		1,
		json.RawMessage(wbsPayload),
	)

	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, domain.BlueprintSectionManualEdit, res.SectionStatus)
	assert.Equal(t, 2, res.ContentVersion)

	require.Equal(t, 1, repo.saveDraftCalls)
	saved := repo.saveDraftParams[0]
	assert.ElementsMatch(t, []domain.BlueprintSectionType{
		domain.BlueprintSectionCrossValidation,
	}, saved.InvalidatedSections)
}
