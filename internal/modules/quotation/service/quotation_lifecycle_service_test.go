package service_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type mockTx struct {
	pgx.Tx
	commitCalled   bool
	rollbackCalled bool
	commitErr      error
	rollbackErr    error
}

func (m *mockTx) Commit(ctx context.Context) error {
	m.commitCalled = true
	return m.commitErr
}

func (m *mockTx) Rollback(ctx context.Context) error {
	m.rollbackCalled = true
	return m.rollbackErr
}

type mockLifecycleRepo struct {
	beginFunc                           func(ctx context.Context) (pgx.Tx, error)
	lockQuotationMasterFunc             func(ctx context.Context, tx pgx.Tx, quotationID string) error
	getQuotationByIDFunc                func(ctx context.Context, id, companyID string) (*domain.Quotation, error)
	getDraftRevisionFunc                func(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error)
	markPublishedRevisionSupersededFunc func(ctx context.Context, tx pgx.Tx, quotationID string) error
	publishDraftQuotationFunc           func(ctx context.Context, tx pgx.Tx, quotationID, draftID string) error
	getActivePublishedRevisionFunc      func(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error)
	getMaxRevisionNumberFunc            func(ctx context.Context, tx pgx.Tx, quotationID string) (int, error)
	createRevisionTxFunc                func(ctx context.Context, tx pgx.Tx, rev *domain.QuotationRevision) error
	updateQuotationCurrentRevisionFunc  func(ctx context.Context, tx pgx.Tx, quotationID string, currentRevision int) error
}

func (m *mockLifecycleRepo) Begin(ctx context.Context) (pgx.Tx, error) {
	if m.beginFunc != nil {
		return m.beginFunc(ctx)
	}
	return &mockTx{}, nil
}

func (m *mockLifecycleRepo) LockQuotationMaster(ctx context.Context, tx pgx.Tx, quotationID string) error {
	if m.lockQuotationMasterFunc != nil {
		return m.lockQuotationMasterFunc(ctx, tx, quotationID)
	}
	return nil
}

func (m *mockLifecycleRepo) GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
	if m.getQuotationByIDFunc != nil {
		return m.getQuotationByIDFunc(ctx, id, companyID)
	}
	return &domain.Quotation{
		ID:        uuid.MustParse(id),
		CompanyID: uuid.MustParse(companyID),
	}, nil
}

func (m *mockLifecycleRepo) GetDraftRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error) {
	if m.getDraftRevisionFunc != nil {
		return m.getDraftRevisionFunc(ctx, tx, quotationID, companyID)
	}
	return nil, repository.ErrRevisionNotFound
}

func (m *mockLifecycleRepo) MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, quotationID string) error {
	if m.markPublishedRevisionSupersededFunc != nil {
		return m.markPublishedRevisionSupersededFunc(ctx, tx, quotationID)
	}
	return nil
}

func (m *mockLifecycleRepo) PublishDraftQuotation(ctx context.Context, tx pgx.Tx, quotationID, draftID string) error {
	if m.publishDraftQuotationFunc != nil {
		return m.publishDraftQuotationFunc(ctx, tx, quotationID, draftID)
	}
	return nil
}

func (m *mockLifecycleRepo) GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error) {
	if m.getActivePublishedRevisionFunc != nil {
		return m.getActivePublishedRevisionFunc(ctx, tx, quotationID, companyID)
	}
	return nil, repository.ErrRevisionNotFound
}

func (m *mockLifecycleRepo) GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, quotationID string) (int, error) {
	if m.getMaxRevisionNumberFunc != nil {
		return m.getMaxRevisionNumberFunc(ctx, tx, quotationID)
	}
	return 1, nil
}

func (m *mockLifecycleRepo) CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.QuotationRevision) error {
	if m.createRevisionTxFunc != nil {
		return m.createRevisionTxFunc(ctx, tx, rev)
	}
	return nil
}

func (m *mockLifecycleRepo) UpdateQuotationCurrentRevision(ctx context.Context, tx pgx.Tx, quotationID string, currentRevision int) error {
	if m.updateQuotationCurrentRevisionFunc != nil {
		return m.updateQuotationCurrentRevisionFunc(ctx, tx, quotationID, currentRevision)
	}
	return nil
}

func TestQuotationLifecycle_ExportedErrors(t *testing.T) {
	assert.Equal(t, "confirmation required for modified content", service.ErrConfirmationRequired.Error())
	assert.Equal(t, "published quotation is immutable", service.ErrQuotationImmutable.Error())
	assert.Equal(t, "active draft revision already exists", service.ErrActiveDraftAlreadyExists.Error())
	assert.Equal(t, "no published revision found to revise", service.ErrNoPublishedRevision.Error())
	assert.Equal(t, "content version mismatch", service.ErrContentVersionMismatch.Error())
	assert.Equal(t, "draft generation must be completed before publishing", service.ErrDraftNotCompleted.Error())
	assert.Equal(t, "quotation content cannot be empty", service.ErrEmptyContent.Error())
	assert.Equal(t, "revision not found", service.ErrRevisionNotFound.Error())
}

func TestPublishQuotation_ValidationErrors(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()
	draftRevisionID := uuid.New().String()
	ctx := context.Background()

	t.Run("MissingCompanyID", func(t *testing.T) {
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(&mockLifecycleRepo{})

		err := s.PublishQuotation(ctx, "", quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNoCompanyContext)
	})

	t.Run("InvalidCompanyOrQuotationUUID", func(t *testing.T) {
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(&mockLifecycleRepo{})

		err := s.PublishQuotation(ctx, "invalid-uuid", quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNotFound)

		err = s.PublishQuotation(ctx, companyID, "invalid-uuid", dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("LockQuotationMaster_NotFound", func(t *testing.T) {
		mockRepo := &mockLifecycleRepo{
			lockQuotationMasterFunc: func(ctx context.Context, tx pgx.Tx, id string) error {
				return repository.ErrNotFound
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("QuotationMaster_TenantMismatch", func(t *testing.T) {
		mockRepo := &mockLifecycleRepo{
			getQuotationByIDFunc: func(ctx context.Context, id, cid string) (*domain.Quotation, error) {
				return nil, repository.ErrNotFound
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("DraftNotFound", func(t *testing.T) {
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return nil, repository.ErrRevisionNotFound
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("DraftRevisionIDMismatch", func(t *testing.T) {
		contentVer := "1"
		html := "<p>content</p>"
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:               uuid.New(), // different ID
					ContentVersion:   &contentVer,
					GenerationStatus: "completed",
					ContentHTML:      &html,
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("ContentVersionMismatch_NilOrDifferent", func(t *testing.T) {
		html := "<p>content</p>"
		// Case: draft ContentVersion is nil
		mockRepoNil := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:               uuid.MustParse(draftRevisionID),
					ContentVersion:   nil,
					GenerationStatus: "completed",
					ContentHTML:      &html,
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepoNil)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrContentVersionMismatch)

		// Case: draft ContentVersion is "2", req is 1
		ver2 := "2"
		mockRepoDiff := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:               uuid.MustParse(draftRevisionID),
					ContentVersion:   &ver2,
					GenerationStatus: "completed",
					ContentHTML:      &html,
				}, nil
			},
		}
		s = service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepoDiff)

		err = s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrContentVersionMismatch)
	})

	t.Run("DraftNotCompleted", func(t *testing.T) {
		contentVer := "1"
		html := "<p>content</p>"
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:               uuid.MustParse(draftRevisionID),
					ContentVersion:   &contentVer,
					GenerationStatus: "pending",
					ContentHTML:      &html,
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrDraftNotCompleted)
	})

	t.Run("EmptyContent_NilOrWhitespace", func(t *testing.T) {
		contentVer := "1"
		// Nil content
		mockRepoNil := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:               uuid.MustParse(draftRevisionID),
					ContentVersion:   &contentVer,
					GenerationStatus: "completed",
					ContentHTML:      nil,
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepoNil)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrEmptyContent)

		// Whitespace content
		emptyHtml := "   \n\t  "
		mockRepoEmpty := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:               uuid.MustParse(draftRevisionID),
					ContentVersion:   &contentVer,
					GenerationStatus: "completed",
					ContentHTML:      &emptyHtml,
				}, nil
			},
		}
		s = service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepoEmpty)

		err = s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		assert.ErrorIs(t, err, service.ErrEmptyContent)
	})

	t.Run("LegalConfirmationRequired", func(t *testing.T) {
		contentVer := "1"
		html := "<p>content</p>"
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:                   uuid.MustParse(draftRevisionID),
					ContentVersion:       &contentVer,
					GenerationStatus:     "completed",
					ContentHTML:          &html,
					LegalContentModified: true,
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:           draftRevisionID,
			ContentVersion:       1,
			ConfirmLegalModified: false,
		})
		assert.ErrorIs(t, err, service.ErrConfirmationRequired)
	})

	t.Run("CommercialConfirmationRequired", func(t *testing.T) {
		contentVer := "1"
		html := "<p>content</p>"
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:                        uuid.MustParse(draftRevisionID),
					ContentVersion:            &contentVer,
					GenerationStatus:          "completed",
					ContentHTML:               &html,
					CommercialContentModified: true,
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:                draftRevisionID,
			ContentVersion:            1,
			ConfirmCommercialModified: false,
		})
		assert.ErrorIs(t, err, service.ErrConfirmationRequired)
	})
}

func TestPublishQuotation_HappyPath(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()
	draftRevisionID := uuid.New().String()
	ctx := context.Background()

	t.Run("PublishUnmodifiedDraft_Success", func(t *testing.T) {
		contentVer := "1"
		html := "<h1>Quotation Proposal</h1>"
		supersededCalled := false
		publishedDraftID := ""
		tx := &mockTx{}

		mockRepo := &mockLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:                        uuid.MustParse(draftRevisionID),
					ContentVersion:            &contentVer,
					GenerationStatus:          "completed",
					ContentHTML:               &html,
					LegalContentModified:      false,
					CommercialContentModified: false,
				}, nil
			},
			markPublishedRevisionSupersededFunc: func(ctx context.Context, tx pgx.Tx, qid string) error {
				supersededCalled = true
				assert.Equal(t, quotationID, qid)
				return nil
			},
			publishDraftQuotationFunc: func(ctx context.Context, tx pgx.Tx, qid, did string) error {
				publishedDraftID = did
				assert.Equal(t, quotationID, qid)
				return nil
			},
		}

		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:     draftRevisionID,
			ContentVersion: 1,
		})
		require.NoError(t, err)
		assert.True(t, supersededCalled)
		assert.Equal(t, draftRevisionID, publishedDraftID)
		assert.True(t, tx.commitCalled)
	})

	t.Run("PublishModifiedDraft_WithConfirmations_Success", func(t *testing.T) {
		contentVer := "1"
		html := "<h1>Customized Proposal</h1>"
		tx := &mockTx{}

		mockRepo := &mockLifecycleRepo{
			beginFunc: func(ctx context.Context) (pgx.Tx, error) {
				return tx, nil
			},
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:                        uuid.MustParse(draftRevisionID),
					ContentVersion:            &contentVer,
					GenerationStatus:          "completed",
					ContentHTML:               &html,
					LegalContentModified:      true,
					CommercialContentModified: true,
				}, nil
			},
		}

		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		err := s.PublishQuotation(ctx, companyID, quotationID, dto.PublishQuotationRequest{
			RevisionID:                draftRevisionID,
			ContentVersion:            1,
			ConfirmLegalModified:      true,
			ConfirmCommercialModified: true,
		})
		require.NoError(t, err)
		assert.True(t, tx.commitCalled)
	})
}

func TestCreateQuotationRevision_ValidationAndErrors(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()
	ctx := context.Background()

	t.Run("MissingCompanyID", func(t *testing.T) {
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(&mockLifecycleRepo{})

		_, err := s.CreateQuotationRevision(ctx, "", quotationID)
		assert.ErrorIs(t, err, service.ErrNoCompanyContext)
	})

	t.Run("InvalidCompanyOrQuotationUUID", func(t *testing.T) {
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(&mockLifecycleRepo{})

		_, err := s.CreateQuotationRevision(ctx, "invalid-uuid", quotationID)
		assert.ErrorIs(t, err, service.ErrNotFound)

		_, err = s.CreateQuotationRevision(ctx, companyID, "invalid-uuid")
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("QuotationNotFound", func(t *testing.T) {
		mockRepo := &mockLifecycleRepo{
			lockQuotationMasterFunc: func(ctx context.Context, tx pgx.Tx, id string) error {
				return repository.ErrNotFound
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		_, err := s.CreateQuotationRevision(ctx, companyID, quotationID)
		assert.ErrorIs(t, err, service.ErrNotFound)
	})

	t.Run("ActiveDraftAlreadyExists", func(t *testing.T) {
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return &domain.QuotationRevision{
					ID:                uuid.New(),
					PublicationStatus: "draft",
				}, nil
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		_, err := s.CreateQuotationRevision(ctx, companyID, quotationID)
		assert.ErrorIs(t, err, service.ErrActiveDraftAlreadyExists)
	})

	t.Run("NoPublishedRevision", func(t *testing.T) {
		mockRepo := &mockLifecycleRepo{
			getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return nil, repository.ErrRevisionNotFound
			},
			getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
				return nil, repository.ErrRevisionNotFound
			},
		}
		s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
			WithLifecycleRepository(mockRepo)

		_, err := s.CreateQuotationRevision(ctx, companyID, quotationID)
		assert.ErrorIs(t, err, service.ErrNoPublishedRevision)
	})
}

func TestCreateQuotationRevision_HappyPath(t *testing.T) {
	companyID := uuid.New()
	quotationID := uuid.New()
	ctx := context.Background()

	html := "<h1>Contract Revision 1</h1>"
	hash := "sha256:abcd"
	aiProv := "openai"
	aiMod := "gpt-4o"
	safety := "v1"
	signer := "John Doe"
	signedIP := "192.168.1.1"
	sigData := "data:image/png;base64,xxxx"
	now := time.Now().UTC()
	budget := int64(150_000_000)
	version1 := "1"

	publishedRev := &domain.QuotationRevision{
		ID:                        uuid.New(),
		CompanyID:                 companyID,
		QuotationID:               quotationID,
		RevisionNumber:            1,
		PublicationStatus:         "published",
		GenerationStatus:          "completed",
		GenerationInput:           json.RawMessage(`{"scope":"full"}`),
		BudgetMaximum:             &budget,
		AllocationSnapshot:        json.RawMessage(`{"roles":[]}`),
		RateCardSnapshot:          json.RawMessage(`{"currency":"IDR"}`),
		LegalTemplateSnapshot:     json.RawMessage(`{"clauses":[]}`),
		ContentHTML:               &html,
		ContentHash:               &hash,
		AiProvider:                &aiProv,
		AiModel:                   &aiMod,
		SkillSnapshot:             json.RawMessage(`{"skills":[]}`),
		SafetyEnvelopeVersion:     &safety,
		PromptState:               json.RawMessage(`{"notes":"test"}`),
		PromptStateVersion:        2,
		PromptSectionHashes:       json.RawMessage(`{"section1":"hash1"}`),
		ContentVersion:            &version1,
		SignerName:                &signer,
		SignerEmail:               &signer,
		SignatureData:             &sigData,
		SignedContentHash:         &hash,
		SignedAt:                  &now,
		SignedIP:                  &signedIP,
		SignedUserAgent:           &signer,
		PublishedAt:               &now,
		LegalContentModified:      true,
		LegalModifiedBy:           &companyID,
		LegalModifiedAt:           &now,
		CommercialContentModified: true,
		CommercialModifiedBy:      &companyID,
		CommercialModifiedAt:      &now,
	}

	var createdRevision *domain.QuotationRevision
	var updatedCurrentRev int
	tx := &mockTx{}

	mockRepo := &mockLifecycleRepo{
		beginFunc: func(ctx context.Context) (pgx.Tx, error) {
			return tx, nil
		},
		getDraftRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
			return nil, repository.ErrRevisionNotFound
		},
		getActivePublishedRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid, cid string) (*domain.QuotationRevision, error) {
			return publishedRev, nil
		},
		getMaxRevisionNumberFunc: func(ctx context.Context, tx pgx.Tx, qid string) (int, error) {
			return 1, nil
		},
		createRevisionTxFunc: func(ctx context.Context, tx pgx.Tx, rev *domain.QuotationRevision) error {
			createdRevision = rev
			return nil
		},
		updateQuotationCurrentRevisionFunc: func(ctx context.Context, tx pgx.Tx, qid string, currentRevision int) error {
			updatedCurrentRev = currentRevision
			return nil
		},
	}

	s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
		WithLifecycleRepository(mockRepo)

	res, err := s.CreateQuotationRevision(ctx, companyID.String(), quotationID.String())
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.True(t, tx.commitCalled)

	// Verify revision number incremented
	assert.Equal(t, 2, updatedCurrentRev)
	assert.Equal(t, 2, res.VersionNumber)
	assert.Equal(t, "draft", res.Status)

	// Verify created revision fields
	require.NotNil(t, createdRevision)
	assert.Equal(t, 2, createdRevision.RevisionNumber)
	assert.Equal(t, "draft", createdRevision.PublicationStatus)
	assert.Equal(t, "completed", createdRevision.GenerationStatus)

	// Verify snapshots cloned
	assert.JSONEq(t, `{"scope":"full"}`, string(createdRevision.GenerationInput))
	assert.Equal(t, &budget, createdRevision.BudgetMaximum)
	assert.JSONEq(t, `{"roles":[]}`, string(createdRevision.AllocationSnapshot))
	assert.JSONEq(t, `{"currency":"IDR"}`, string(createdRevision.RateCardSnapshot))
	assert.JSONEq(t, `{"clauses":[]}`, string(createdRevision.LegalTemplateSnapshot))
	assert.Equal(t, &html, createdRevision.ContentHTML)
	assert.Equal(t, &hash, createdRevision.ContentHash)
	assert.Equal(t, &aiProv, createdRevision.AiProvider)
	assert.Equal(t, &aiMod, createdRevision.AiModel)
	assert.JSONEq(t, `{"skills":[]}`, string(createdRevision.SkillSnapshot))
	assert.Equal(t, &safety, createdRevision.SafetyEnvelopeVersion)
	assert.JSONEq(t, `{"notes":"test"}`, string(createdRevision.PromptState))
	assert.Equal(t, int64(2), createdRevision.PromptStateVersion)
	assert.JSONEq(t, `{"section1":"hash1"}`, string(createdRevision.PromptSectionHashes))

	// Verify reset fields
	require.NotNil(t, createdRevision.ContentVersion)
	assert.Equal(t, "1", *createdRevision.ContentVersion)
	assert.Nil(t, createdRevision.SignedAt)
	assert.Nil(t, createdRevision.SignerName)
	assert.Nil(t, createdRevision.SignerEmail)
	assert.Nil(t, createdRevision.SignatureData)
	assert.Nil(t, createdRevision.SignedContentHash)
	assert.Nil(t, createdRevision.SignedIP)
	assert.Nil(t, createdRevision.SignedUserAgent)
	assert.Nil(t, createdRevision.PublishedAt)
	assert.False(t, createdRevision.LegalContentModified)
	assert.Nil(t, createdRevision.LegalModifiedBy)
	assert.Nil(t, createdRevision.LegalModifiedAt)
	assert.False(t, createdRevision.CommercialContentModified)
	assert.Nil(t, createdRevision.CommercialModifiedBy)
	assert.Nil(t, createdRevision.CommercialModifiedAt)
}

func TestQuotationLifecycle_DatabaseIntegration(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("skipping lifecycle database test: TEST_DATABASE_URL not set")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping lifecycle database test: unable to connect: %v", err)
		return
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skipping lifecycle database test: database ping failed: %v", err)
		return
	}

	repo := repository.NewRepository(pool)
	s := service.NewService(repo, nil, nil, nil, nil, nil, nil, nil)

	companyID := uuid.New()
	clientID := uuid.New()
	quotationID := uuid.New()
	quotNumber := "QUO-" + uuid.New().String()[:8]

	// Seed client and quotation
	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
		VALUES ($1, $2, 'Integration Test Client', 'integration test client')
	`, clientID, companyID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
		VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'draft', 1)
	`, quotationID, companyID, clientID, quotNumber)
	require.NoError(t, err)

	// Clean up after test
	defer func() {
		_, _ = pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE quotation_id = $1`, quotationID)
		_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE id = $1`, quotationID)
		_, _ = pool.Exec(ctx, `DELETE FROM quotation_client_organizations WHERE id = $1`, clientID)
	}()

	// Insert initial draft revision
	rev1ID := uuid.New()
	contentVer := "1"
	html := "<p>Initial Draft</p>"
	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number, publication_status, generation_status,
			content_version, content_html, generation_input, budget_maximum
		) VALUES ($1, $2, $3, 1, 'draft', 'completed', $4, $5, '{}'::jsonb, 100000)
	`, rev1ID, companyID, quotationID, contentVer, html)
	require.NoError(t, err)

	// Test 1: Cannot create new revision while draft exists
	_, err = s.CreateQuotationRevision(ctx, companyID.String(), quotationID.String())
	assert.ErrorIs(t, err, service.ErrActiveDraftAlreadyExists)

	// Test 2: Publish draft revision 1
	err = s.PublishQuotation(ctx, companyID.String(), quotationID.String(), dto.PublishQuotationRequest{
		RevisionID:     rev1ID.String(),
		ContentVersion: 1,
	})
	require.NoError(t, err)

	// Verify quotation status is published and active_published_revision_id is rev1ID
	var status string
	var activePublishedRevID *uuid.UUID
	err = pool.QueryRow(ctx, `SELECT status, active_published_revision_id FROM quotations WHERE id = $1`, quotationID).
		Scan(&status, &activePublishedRevID)
	require.NoError(t, err)
	assert.Equal(t, "published", status)
	require.NotNil(t, activePublishedRevID)
	assert.Equal(t, rev1ID, *activePublishedRevID)

	// Test 3: Create new revision 2 from published revision 1
	rev2Resp, err := s.CreateQuotationRevision(ctx, companyID.String(), quotationID.String())
	require.NoError(t, err)
	require.NotNil(t, rev2Resp)
	assert.Equal(t, 2, rev2Resp.VersionNumber)
	assert.Equal(t, "draft", rev2Resp.Status)

	// Test 4: Cannot create revision 3 while draft revision 2 exists
	_, err = s.CreateQuotationRevision(ctx, companyID.String(), quotationID.String())
	assert.ErrorIs(t, err, service.ErrActiveDraftAlreadyExists)

	// Test 5: Publish revision 2 -> supersedes revision 1
	err = s.PublishQuotation(ctx, companyID.String(), quotationID.String(), dto.PublishQuotationRequest{
		RevisionID:     rev2Resp.ID.String(),
		ContentVersion: 1,
	})
	require.NoError(t, err)

	// Verify revision 1 is superseded, revision 2 is published
	var rev1PubStatus, rev2PubStatus string
	err = pool.QueryRow(ctx, `SELECT publication_status FROM quotation_revisions WHERE id = $1`, rev1ID).Scan(&rev1PubStatus)
	require.NoError(t, err)
	assert.Equal(t, "superseded", rev1PubStatus)

	err = pool.QueryRow(ctx, `SELECT publication_status FROM quotation_revisions WHERE id = $1`, rev2Resp.ID).Scan(&rev2PubStatus)
	require.NoError(t, err)
	assert.Equal(t, "published", rev2PubStatus)
}
