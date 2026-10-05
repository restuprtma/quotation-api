package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type mockBlueprintTx struct {
	pgx.Tx
	commitCalled   bool
	rollbackCalled bool
	commitErr      error
	rollbackErr    error
}

func (m *mockBlueprintTx) Commit(ctx context.Context) error {
	m.commitCalled = true
	return m.commitErr
}

func (m *mockBlueprintTx) Rollback(ctx context.Context) error {
	m.rollbackCalled = true
	return m.rollbackErr
}

type mockBlueprintLifecycleRepo struct {
	beginFunc                           func(ctx context.Context) (pgx.Tx, error)
	lockBlueprintMasterFunc             func(ctx context.Context, tx pgx.Tx, blueprintID string) error
	deleteDraftFunc                     func(ctx context.Context, companyID, blueprintID string) error
	publishDraftBlueprintFunc           func(ctx context.Context, tx pgx.Tx, blueprintID, draftID string, isComplete bool) error
	markPublishedRevisionSupersededFunc func(ctx context.Context, tx pgx.Tx, blueprintID string) error
	getMaxRevisionNumberFunc            func(ctx context.Context, tx pgx.Tx, blueprintID string) (int, error)
	getDraftRevisionFunc                func(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error)
	getActivePublishedRevisionFunc      func(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error)
	createRevisionTxFunc                func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error
	updateBlueprintCurrentRevisionFunc  func(ctx context.Context, tx pgx.Tx, blueprintID string, currentRevision int) error
}

func (m *mockBlueprintLifecycleRepo) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginFunc != nil {
		return m.beginFunc(ctx)
	}
	return &mockBlueprintTx{}, nil
}

func (m *mockBlueprintLifecycleRepo) LockBlueprintMaster(ctx context.Context, tx pgx.Tx, blueprintID string) error {
	if m.lockBlueprintMasterFunc != nil {
		return m.lockBlueprintMasterFunc(ctx, tx, blueprintID)
	}
	return nil
}

func (m *mockBlueprintLifecycleRepo) DeleteDraft(ctx context.Context, companyID, blueprintID string) error {
	if m.deleteDraftFunc != nil {
		return m.deleteDraftFunc(ctx, companyID, blueprintID)
	}
	return nil
}

func (m *mockBlueprintLifecycleRepo) PublishDraftBlueprint(ctx context.Context, tx pgx.Tx, blueprintID, draftID string, isComplete bool) error {
	if m.publishDraftBlueprintFunc != nil {
		return m.publishDraftBlueprintFunc(ctx, tx, blueprintID, draftID, isComplete)
	}
	return nil
}

func (m *mockBlueprintLifecycleRepo) MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, blueprintID string) error {
	if m.markPublishedRevisionSupersededFunc != nil {
		return m.markPublishedRevisionSupersededFunc(ctx, tx, blueprintID)
	}
	return nil
}

func (m *mockBlueprintLifecycleRepo) GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, blueprintID string) (int, error) {
	if m.getMaxRevisionNumberFunc != nil {
		return m.getMaxRevisionNumberFunc(ctx, tx, blueprintID)
	}
	return 1, nil
}

func (m *mockBlueprintLifecycleRepo) GetDraftRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error) {
	if m.getDraftRevisionFunc != nil {
		return m.getDraftRevisionFunc(ctx, tx, blueprintID, companyID)
	}
	return nil, repository.ErrBlueprintDraftNotFound
}

func (m *mockBlueprintLifecycleRepo) GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error) {
	if m.getActivePublishedRevisionFunc != nil {
		return m.getActivePublishedRevisionFunc(ctx, tx, blueprintID, companyID)
	}
	return nil, repository.ErrBlueprintRevisionNotFound
}

func (m *mockBlueprintLifecycleRepo) CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
	if m.createRevisionTxFunc != nil {
		return m.createRevisionTxFunc(ctx, tx, rev)
	}
	return nil
}

func (m *mockBlueprintLifecycleRepo) UpdateBlueprintCurrentRevision(ctx context.Context, tx pgx.Tx, blueprintID string, currentRevision int) error {
	if m.updateBlueprintCurrentRevisionFunc != nil {
		return m.updateBlueprintCurrentRevisionFunc(ctx, tx, blueprintID, currentRevision)
	}
	return nil
}

func fullCompleteContentJSON() json.RawMessage {
	return json.RawMessage(`{
		"schema_version": "1.0",
		"title": "Complete System Blueprint",
		"summary": "Full blueprint summary",
		"journey": {
			"phases": [{"id": "p1", "label": "Onboarding", "order": 1}],
			"actors": [{"id": "a1", "label": "Customer", "order": 1}],
			"activities": [{"id": "act1", "phase_id": "p1", "actor_id": "a1", "label": "Register", "touchpoints": ["web"]}]
		},
		"architecture": {
			"groups": [{"id": "g1", "label": "App Tier", "order": 1}],
			"nodes": [{"id": "n1", "group_id": "g1", "type": "service", "label": "API", "layout": {"x": 10, "y": 10}}],
			"edges": []
		},
		"state_machine": {
			"entity": "Account",
			"initial_state": "s1",
			"states": [{"id": "s1", "label": "Active", "type": "normal", "layout": {"x": 10, "y": 10}}],
			"transitions": []
		},
		"workflows": [
			{
				"id": "wf1",
				"name": "User Signup",
				"description": "Signup flow",
				"lanes": [{"id": "l1", "label": "User", "order": 1}],
				"nodes": [{"id": "wn1", "lane_id": "l1", "type": "start", "label": "Start", "description": "start", "layout": {"x": 10, "y": 10}}],
				"edges": []
			}
		],
		"wbs": {
			"modules": [
				{
					"id": "m1",
					"name": "Auth Module",
					"description": "Authentication and authorization",
					"priority": "high",
					"references": ["architecture.node.n1"],
					"features": [{"id": "f1", "name": "Login", "description": "Login feature", "acceptance_criteria": ["Valid JWT"]}]
				}
			]
		}
	}`)
}

func partialContentJSON() json.RawMessage {
	return json.RawMessage(`{
		"schema_version": "1.0",
		"title": "Partial System Blueprint",
		"summary": "Partial blueprint",
		"journey": {
			"phases": [{"id": "p1", "label": "Onboarding", "order": 1}],
			"actors": [{"id": "a1", "label": "Customer", "order": 1}],
			"activities": [{"id": "act1", "phase_id": "p1", "actor_id": "a1", "label": "Register", "touchpoints": ["web"]}]
		}
	}`)
}

func TestBlueprintLifecycle_UnitContracts(t *testing.T) {
	assert.Equal(t, "confirmation required for partial blueprint publish", service.ErrBlueprintConfirmationRequired.Error())
	assert.Equal(t, "published blueprint is immutable", service.ErrBlueprintImmutable.Error())
	assert.Equal(t, "active draft revision already exists", service.ErrBlueprintActiveDraftAlreadyExists.Error())
	assert.Equal(t, "no published revision found to revise", service.ErrBlueprintNoPublishedRevision.Error())
	assert.Equal(t, "blueprint revision not found", service.ErrBlueprintRevisionNotFound.Error())
	assert.Equal(t, "blueprint not found", service.ErrBlueprintNotFound.Error())
	assert.Equal(t, "draft blueprint revision not found", service.ErrBlueprintDraftNotFound.Error())

	// Compile-time interface check
	var _ service.BlueprintLifecycleRepo = (*repository.BlueprintGenerationRepository)(nil)
	var _ service.BlueprintLifecycleRepo = (*mockBlueprintLifecycleRepo)(nil)
}

func TestBlueprintLifecycle_PublishBlueprint(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New().String()
	blueprintID := uuid.New().String()
	draftRevID := uuid.New()

	t.Run("MissingConfirmation_ForPartialDraft_ReturnsErrBlueprintConfirmationRequired", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                 draftRevID,
					CompanyID:          uuid.MustParse(companyID),
					BlueprintID:        uuid.MustParse(blueprintID),
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "partial",
					ContentJSON:        partialContentJSON(),
				}, nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: false,
		}

		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintConfirmationRequired)
	})

	t.Run("ConfirmPartial_True_AllowsPartialPublish", func(t *testing.T) {
		publishedIsComplete := true
		supersededCalled := false
		tx := &mockBlueprintTx{}

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                 draftRevID,
					CompanyID:          uuid.MustParse(companyID),
					BlueprintID:        uuid.MustParse(blueprintID),
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "partial",
					ContentJSON:        partialContentJSON(),
				}, nil
			},
			markPublishedRevisionSupersededFunc: func(ctx context.Context, tx pgx.Tx, bID string) error {
				supersededCalled = true
				return nil
			},
			publishDraftBlueprintFunc: func(ctx context.Context, tx pgx.Tx, bID, dID string, isComplete bool) error {
				assert.Equal(t, blueprintID, bID)
				assert.Equal(t, draftRevID.String(), dID)
				publishedIsComplete = isComplete
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: true,
		}

		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		assert.False(t, publishedIsComplete, "completeness status must be partial (isComplete=false)")
		assert.True(t, supersededCalled, "previous published revision must be marked superseded")
		assert.True(t, tx.commitCalled, "transaction must be committed")
	})

	t.Run("CompleteDraft_PublishesWithoutConfirmation", func(t *testing.T) {
		publishedIsComplete := false
		supersededCalled := false
		tx := &mockBlueprintTx{}

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                 draftRevID,
					CompanyID:          uuid.MustParse(companyID),
					BlueprintID:        uuid.MustParse(blueprintID),
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "complete",
					ContentJSON:        fullCompleteContentJSON(),
				}, nil
			},
			markPublishedRevisionSupersededFunc: func(ctx context.Context, tx pgx.Tx, bID string) error {
				supersededCalled = true
				return nil
			},
			publishDraftBlueprintFunc: func(ctx context.Context, tx pgx.Tx, bID, dID string, isComplete bool) error {
				publishedIsComplete = isComplete
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: false, // Not confirming partial
		}

		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		assert.True(t, publishedIsComplete, "completeness status must be complete (isComplete=true)")
		assert.True(t, supersededCalled)
		assert.True(t, tx.commitCalled)
	})

	t.Run("CompleteDraft_WithWarningCodes_RequiresConfirmation", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                 draftRevID,
					CompanyID:          uuid.MustParse(companyID),
					BlueprintID:        uuid.MustParse(blueprintID),
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "complete",
					ContentJSON:        fullCompleteContentJSON(),
				}, nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		// With warning codes but ConfirmPartial=false -> fails
		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: false,
			WarningCodes:   []string{"WARN_ARCH_NODE_UNREFERENCED_IN_WBS"},
		}
		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintConfirmationRequired)

		// With warning codes and ConfirmPartial=true -> succeeds as partial
		req.ConfirmPartial = true
		publishedIsComplete := true
		repo.publishDraftBlueprintFunc = func(ctx context.Context, tx pgx.Tx, bID, dID string, isComplete bool) error {
			publishedIsComplete = isComplete
			return nil
		}
		err = svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		assert.False(t, publishedIsComplete, "warning codes must publish as partial")
	})

	t.Run("CompleteDraft_WithCrossValidationWarnings_RequiresConfirmation", func(t *testing.T) {
		crossValJSON := json.RawMessage(`{
			"valid": true,
			"issues": [{"code": "WARN_EMPTY_SECTION", "severity": "warning", "blocking": false}]
		}`)

		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                    draftRevID,
					CompanyID:             uuid.MustParse(companyID),
					BlueprintID:           uuid.MustParse(blueprintID),
					RevisionNumber:        1,
					PublicationStatus:     "draft",
					CompletenessStatus:    "complete",
					ContentJSON:           fullCompleteContentJSON(),
					CrossValidationResult: crossValJSON,
				}, nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: false,
		}
		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintConfirmationRequired)

		req.ConfirmPartial = true
		err = svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
	})

	t.Run("RevisionIDMismatch_ReturnsErrNotFound", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                 draftRevID,
					CompanyID:          uuid.MustParse(companyID),
					BlueprintID:        uuid.MustParse(blueprintID),
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "complete",
					ContentJSON:        fullCompleteContentJSON(),
				}, nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		differentRevID := uuid.New().String()
		req := dto.PublishBlueprintRequest{
			RevisionID:     differentRevID,
			ConfirmPartial: true,
		}
		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)
	})

	t.Run("DraftNotFound_ReturnsErrNotFound", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: true,
		}
		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)
	})

	t.Run("TenantMismatch_WrongCompanyID_ReturnsErrNotFound", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				// Tenant isolation query returns ErrBlueprintDraftNotFound when company ID does not match
				return nil, repository.ErrBlueprintDraftNotFound
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		wrongCompanyID := uuid.New().String()
		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: true,
		}
		err := svc.PublishBlueprint(ctx, wrongCompanyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)
	})

	t.Run("InvalidUUIDs_ReturnsErrNotFound", func(t *testing.T) {
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(&mockBlueprintLifecycleRepo{})

		// Invalid companyID
		err := svc.PublishBlueprint(ctx, "invalid-uuid", blueprintID, dto.PublishBlueprintRequest{RevisionID: draftRevID.String()})
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)

		// Invalid blueprintID
		err = svc.PublishBlueprint(ctx, companyID, "invalid-uuid", dto.PublishBlueprintRequest{RevisionID: draftRevID.String()})
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)

		// Invalid revisionID
		err = svc.PublishBlueprint(ctx, companyID, blueprintID, dto.PublishBlueprintRequest{RevisionID: "invalid-uuid"})
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)
	})

	t.Run("LockBlueprintMaster_NotFound_ReturnsErrNotFound", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			lockBlueprintMasterFunc: func(ctx context.Context, tx pgx.Tx, bID string) error {
				return repository.ErrBlueprintMasterNotFound
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID:     draftRevID.String(),
			ConfirmPartial: true,
		}
		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, service.ErrBlueprintNotFound)
	})

	t.Run("TransactionRollback_OnPublishFailure", func(t *testing.T) {
		tx := &mockBlueprintTx{}
		publishErr := errors.New("database disk full")

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                 draftRevID,
					CompanyID:          uuid.MustParse(companyID),
					BlueprintID:        uuid.MustParse(blueprintID),
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "complete",
					ContentJSON:        fullCompleteContentJSON(),
				}, nil
			},
			publishDraftBlueprintFunc: func(ctx context.Context, tx pgx.Tx, bID, dID string, isComplete bool) error {
				return publishErr
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.PublishBlueprintRequest{
			RevisionID: draftRevID.String(),
		}
		err := svc.PublishBlueprint(ctx, companyID, blueprintID, req)
		assert.ErrorIs(t, err, publishErr)
		assert.True(t, tx.rollbackCalled, "rollback must be executed on error")
		assert.False(t, tx.commitCalled, "commit must not be called on error")
	})
}

func TestBlueprintLifecycle_CreateBlueprintRevision(t *testing.T) {
	ctx := context.Background()
	companyID := uuid.New().String()
	blueprintID := uuid.New().String()
	pubRevID := uuid.New()
	sourceQRevID := uuid.New()

	publishedRev := &domain.BlueprintRevision{
		ID:                        pubRevID,
		CompanyID:                 uuid.MustParse(companyID),
		BlueprintID:               uuid.MustParse(blueprintID),
		RevisionNumber:            1,
		SourceQuotationRevisionID: &sourceQRevID,
		SourceContextSnapshot:     json.RawMessage(`{"source": "quotation"}`),
		PublicationStatus:         "published",
		CompletenessStatus:        "complete",
		GenerationStatus:          "completed",
		ContentJSON:               fullCompleteContentJSON(),
		ContentVersion:            1,
		ValidationResult:          json.RawMessage(`{"valid": true}`),
		CrossValidationResult:     json.RawMessage(`{"valid": true}`),
		SkillSnapshot:             json.RawMessage(`{"skills": ["arch"]}`),
		SafetyEnvelopeVersion:     strPtr("v1.0"),
		AiProvider:                strPtr("gemini"),
		AiModel:                   strPtr("gemini-2.0-flash"),
	}

	t.Run("Fails_IfActiveDraftAlreadyExists", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return &domain.BlueprintRevision{
					ID:                uuid.New(),
					PublicationStatus: "draft",
				}, nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{Mode: "clone"}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrBlueprintActiveDraftAlreadyExists)
	})

	t.Run("Fails_IfNoPublishedRevisionExists", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintRevisionNotFound
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{Mode: "clone"}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrBlueprintNoPublishedRevision)
	})

	t.Run("ModeClone_CreatesNewRevision_WithClonedContent", func(t *testing.T) {
		tx := &mockBlueprintTx{}
		var createdRev *domain.BlueprintRevision
		var updatedCurrentRev int

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return publishedRev, nil
			},
			getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, bID string) (int, error) {
				return 1, nil
			},
			createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
				createdRev = rev
				rev.CreatedAt = time.Now()
				rev.UpdatedAt = time.Now()
				return nil
			},
			updateBlueprintCurrentRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID string, currentRevision int) error {
				updatedCurrentRev = currentRevision
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{Mode: "clone"}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Assert response mapping
		assert.Equal(t, createdRev.ID, resp.ID)
		assert.Equal(t, uuid.MustParse(blueprintID), resp.BlueprintID)
		assert.Equal(t, 2, resp.VersionNumber)
		assert.Equal(t, "draft", resp.Status)

		// Assert new revision field cloning
		require.NotNil(t, createdRev)
		assert.Equal(t, 2, createdRev.RevisionNumber)
		assert.Equal(t, "draft", createdRev.PublicationStatus)
		assert.Equal(t, "completed", createdRev.GenerationStatus)
		assert.Equal(t, "complete", createdRev.CompletenessStatus)
		assert.Equal(t, 1, createdRev.ContentVersion)
		assert.Nil(t, createdRev.PublishedAt)
		assert.Equal(t, publishedRev.ContentJSON, createdRev.ContentJSON)
		assert.Equal(t, publishedRev.SourceQuotationRevisionID, createdRev.SourceQuotationRevisionID)
		assert.Equal(t, publishedRev.SourceContextSnapshot, createdRev.SourceContextSnapshot)
		assert.Equal(t, publishedRev.SkillSnapshot, createdRev.SkillSnapshot)
		assert.Equal(t, publishedRev.AiProvider, createdRev.AiProvider)
		assert.Equal(t, publishedRev.AiModel, createdRev.AiModel)
		assert.Equal(t, publishedRev.ValidationResult, createdRev.ValidationResult)
		assert.Equal(t, publishedRev.CrossValidationResult, createdRev.CrossValidationResult)

		// Assert master update and commit
		assert.Equal(t, 2, updatedCurrentRev)
		assert.True(t, tx.commitCalled)
	})

	t.Run("ModeClone_DefaultWhenModeIsEmpty", func(t *testing.T) {
		tx := &mockBlueprintTx{}
		var createdRev *domain.BlueprintRevision

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return publishedRev, nil
			},
			getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, bID string) (int, error) {
				return 3, nil
			},
			createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
				createdRev = rev
				rev.CreatedAt = time.Now()
				rev.UpdatedAt = time.Now()
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{Mode: ""} // empty mode -> defaults to clone
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		assert.Equal(t, 4, resp.VersionNumber)
		assert.Equal(t, "completed", createdRev.GenerationStatus)
		assert.Equal(t, publishedRev.ContentJSON, createdRev.ContentJSON)
	})

	t.Run("ModeRegenerateFromSource_Success", func(t *testing.T) {
		tx := &mockBlueprintTx{}
		var createdRev *domain.BlueprintRevision
		var updatedCurrentRev int
		newSourceQRevID := uuid.New().String()

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return publishedRev, nil
			},
			getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, bID string) (int, error) {
				return 1, nil
			},
			createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
				createdRev = rev
				rev.CreatedAt = time.Now()
				rev.UpdatedAt = time.Now()
				return nil
			},
			updateBlueprintCurrentRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID string, currentRevision int) error {
				updatedCurrentRev = currentRevision
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{
			Mode:                 "regenerate_from_source",
			SourceQuotationRevID: newSourceQRevID,
		}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		assert.Equal(t, 2, resp.VersionNumber)
		assert.Equal(t, "draft", resp.Status)

		// Assert regenerate_from_source specific fields
		require.NotNil(t, createdRev)
		assert.Equal(t, 2, createdRev.RevisionNumber)
		assert.Equal(t, "draft", createdRev.PublicationStatus)
		assert.Equal(t, "pending", createdRev.GenerationStatus)
		assert.Equal(t, "partial", createdRev.CompletenessStatus)
		assert.Equal(t, json.RawMessage("{}"), createdRev.ContentJSON)
		assert.Equal(t, 1, createdRev.ContentVersion)
		assert.Nil(t, createdRev.PublishedAt)
		assert.Nil(t, createdRev.GeneratedAt)
		require.NotNil(t, createdRev.SourceQuotationRevisionID)
		assert.Equal(t, newSourceQRevID, createdRev.SourceQuotationRevisionID.String())

		assert.Equal(t, 2, updatedCurrentRev)
		assert.True(t, tx.commitCalled)
	})

	t.Run("ModeRegenerateFromSource_FallbackToPublishedSourceQuotationRevID", func(t *testing.T) {
		var createdRev *domain.BlueprintRevision

		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return publishedRev, nil
			},
			getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, bID string) (int, error) {
				return 1, nil
			},
			createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
				createdRev = rev
				rev.CreatedAt = time.Now()
				rev.UpdatedAt = time.Now()
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{
			Mode:                 "regenerate_from_source",
			SourceQuotationRevID: "", // empty -> fallback to publishedRev.SourceQuotationRevisionID
		}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		require.NoError(t, err)
		assert.Equal(t, 2, resp.VersionNumber)
		require.NotNil(t, createdRev)
		assert.Equal(t, publishedRev.SourceQuotationRevisionID, createdRev.SourceQuotationRevisionID)
	})

	t.Run("InvalidMode_ReturnsError", func(t *testing.T) {
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(&mockBlueprintLifecycleRepo{})
		req := dto.CreateBlueprintRevisionRequest{Mode: "unsupported_mode"}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		assert.Nil(t, resp)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid revision mode")
	})

	t.Run("TenantMismatch_WrongCompanyID_ReturnsErrNotFound", func(t *testing.T) {
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintRevisionNotFound
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		wrongCompanyID := uuid.New().String()
		req := dto.CreateBlueprintRevisionRequest{Mode: "clone"}
		resp, err := svc.CreateBlueprintRevision(ctx, wrongCompanyID, blueprintID, req)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, service.ErrBlueprintNoPublishedRevision)
	})

	t.Run("CreateBlueprintRevisionSimple_DelegatesCorrectly", func(t *testing.T) {
		var createdRev *domain.BlueprintRevision
		repo := &mockBlueprintLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return publishedRev, nil
			},
			getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, bID string) (int, error) {
				return 2, nil
			},
			createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
				createdRev = rev
				rev.CreatedAt = time.Now()
				rev.UpdatedAt = time.Now()
				return nil
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		resp, err := svc.CreateBlueprintRevisionSimple(ctx, companyID, blueprintID, "clone", "")
		require.NoError(t, err)
		assert.Equal(t, 3, resp.VersionNumber)
		assert.Equal(t, "completed", createdRev.GenerationStatus)
	})

	t.Run("TransactionRollback_OnInsertError", func(t *testing.T) {
		tx := &mockBlueprintTx{}
		insertErr := errors.New("unique constraint violation")

		repo := &mockBlueprintLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return nil, repository.ErrBlueprintDraftNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, bID, cID string) (*domain.BlueprintRevision, error) {
				return publishedRev, nil
			},
			getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, bID string) (int, error) {
				return 1, nil
			},
			createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
				return insertErr
			},
		}
		svc := service.NewBlueprintDraftService(nil, nil, nil, nil).WithLifecycleRepo(repo)

		req := dto.CreateBlueprintRevisionRequest{Mode: "clone"}
		resp, err := svc.CreateBlueprintRevision(ctx, companyID, blueprintID, req)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, insertErr)
		assert.True(t, tx.rollbackCalled)
		assert.False(t, tx.commitCalled)
	})
}
