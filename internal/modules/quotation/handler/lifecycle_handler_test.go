package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

// Mocks

type mockQuotationLifecycleService struct {
	publishQuotationFunc        func(ctx context.Context, companyID, quotationID string, req dto.PublishQuotationRequest) error
	createQuotationRevisionFunc func(ctx context.Context, companyID, quotationID string) (*dto.QuotationRevisionResponse, error)
	listRevisionsFunc           func(ctx context.Context, companyID, id string) ([]dto.QuotationRevisionResponse, error)
	getQuotationHistoryFunc     func(ctx context.Context, companyID, quotationID string) ([]dto.QuotationRevisionHistoryItem, error)
}

func (m *mockQuotationLifecycleService) PublishQuotation(ctx context.Context, companyID, quotationID string, req dto.PublishQuotationRequest) error {
	if m.publishQuotationFunc != nil {
		return m.publishQuotationFunc(ctx, companyID, quotationID, req)
	}
	return nil
}

func (m *mockQuotationLifecycleService) CreateQuotationRevision(ctx context.Context, companyID, quotationID string) (*dto.QuotationRevisionResponse, error) {
	if m.createQuotationRevisionFunc != nil {
		return m.createQuotationRevisionFunc(ctx, companyID, quotationID)
	}
	return &dto.QuotationRevisionResponse{
		ID:            uuid.New(),
		VersionNumber: 2,
		Status:        "draft",
	}, nil
}

func (m *mockQuotationLifecycleService) ListRevisions(ctx context.Context, companyID, id string) ([]dto.QuotationRevisionResponse, error) {
	if m.listRevisionsFunc != nil {
		return m.listRevisionsFunc(ctx, companyID, id)
	}
	return nil, nil
}

func (m *mockQuotationLifecycleService) GetQuotationHistory(ctx context.Context, companyID, quotationID string) ([]dto.QuotationRevisionHistoryItem, error) {
	if m.getQuotationHistoryFunc != nil {
		return m.getQuotationHistoryFunc(ctx, companyID, quotationID)
	}
	return nil, nil
}

type mockBlueprintLifecycleDraftService struct {
	publishBlueprintFunc        func(ctx context.Context, companyID, blueprintID string, req dto.PublishBlueprintRequest) error
	createBlueprintRevisionFunc func(ctx context.Context, companyID, blueprintID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error)
	deleteDraftFunc             func(ctx context.Context, companyID, blueprintID string) error
}

func (m *mockBlueprintLifecycleDraftService) PublishBlueprint(ctx context.Context, companyID, blueprintID string, req dto.PublishBlueprintRequest) error {
	if m.publishBlueprintFunc != nil {
		return m.publishBlueprintFunc(ctx, companyID, blueprintID, req)
	}
	return nil
}

func (m *mockBlueprintLifecycleDraftService) CreateBlueprintRevision(ctx context.Context, companyID, blueprintID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error) {
	if m.createBlueprintRevisionFunc != nil {
		return m.createBlueprintRevisionFunc(ctx, companyID, blueprintID, req)
	}
	return &dto.BlueprintRevisionResponse{
		ID:            uuid.New(),
		BlueprintID:   uuid.MustParse(blueprintID),
		VersionNumber: 2,
		Status:        "draft",
	}, nil
}

func (m *mockBlueprintLifecycleDraftService) DeleteDraft(ctx context.Context, companyID, blueprintID string) error {
	if m.deleteDraftFunc != nil {
		return m.deleteDraftFunc(ctx, companyID, blueprintID)
	}
	return nil
}

type mockBlueprintLifecycleReadService struct {
	listRevisionsFunc func(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintRevisionHistoryItem, error)
}

func (m *mockBlueprintLifecycleReadService) ListRevisions(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintRevisionHistoryItem, error) {
	if m.listRevisionsFunc != nil {
		return m.listRevisionsFunc(ctx, companyID, blueprintID)
	}
	return nil, nil
}

func setupLifecycleRouter(
	qSvc *mockQuotationLifecycleService,
	bpDraftSvc *mockBlueprintLifecycleDraftService,
	bpReadSvc *mockBlueprintLifecycleReadService,
	companyID string,
) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	hdlr := handler.NewLifecycleHandler(qSvc, bpDraftSvc, bpReadSvc)

	router.Use(func(c *gin.Context) {
		if companyID != "" {
			c.Set("user", &jwtpkg.Claims{
				CompanyID: companyID,
			})
		}
		c.Next()
	})

	v1 := router.Group("/quotation/v1")
	{
		quotations := v1.Group("/quotations")
		{
			quotations.POST("/:id/publish", hdlr.PublishQuotation)
			quotations.POST("/:id/revisions", hdlr.CreateQuotationRevision)
			quotations.GET("/:id/revisions", hdlr.ListQuotationRevisions)
			quotations.GET("/:id/revisions/:revision_id", hdlr.GetQuotationRevisionDetail)
		}

		blueprints := v1.Group("/blueprints")
		{
			blueprints.POST("/:id/publish", hdlr.PublishBlueprint)
			blueprints.POST("/:id/revisions", hdlr.CreateBlueprintRevision)
			blueprints.GET("/:id/revisions", hdlr.ListBlueprintRevisions)
			blueprints.GET("/:id/revisions/:revision_id", hdlr.GetBlueprintRevisionDetail)
			blueprints.DELETE("/:id/draft", hdlr.DeleteBlueprintDraft)
		}
	}

	return router
}

// Tests for Quotation Publish

func TestLifecycleHandler_QuotationPublish(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()
	revisionID := uuid.New().String()

	t.Run("missing company context returns 403", func(t *testing.T) {
		router := setupLifecycleRouter(&mockQuotationLifecycleService{}, nil, nil, "")
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid quotation ID returns 400", func(t *testing.T) {
		router := setupLifecycleRouter(&mockQuotationLifecycleService{}, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/invalid-uuid/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid payload returns 400", func(t *testing.T) {
		router := setupLifecycleRouter(&mockQuotationLifecycleService{}, nil, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", strings.NewReader("not-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("confirmation required returns 400", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			publishQuotationFunc: func(ctx context.Context, cID, qID string, req dto.PublishQuotationRequest) error {
				return service.ErrConfirmationRequired
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Confirmation required")
	})

	t.Run("draft not completed returns 400", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			publishQuotationFunc: func(ctx context.Context, cID, qID string, req dto.PublishQuotationRequest) error {
				return service.ErrDraftNotCompleted
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Draft generation must be completed")
	})

	t.Run("empty content returns 400", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			publishQuotationFunc: func(ctx context.Context, cID, qID string, req dto.PublishQuotationRequest) error {
				return service.ErrEmptyContent
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "cannot be empty")
	})

	t.Run("content version mismatch returns 409", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			publishQuotationFunc: func(ctx context.Context, cID, qID string, req dto.PublishQuotationRequest) error {
				return service.ErrContentVersionMismatch
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "Content version mismatch")
	})

	t.Run("not found returns 404", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			publishQuotationFunc: func(ctx context.Context, cID, qID string, req dto.PublishQuotationRequest) error {
				return service.ErrNotFound
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{RevisionID: revisionID, ContentVersion: 1})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("valid publish returns 200", func(t *testing.T) {
		var receivedReq dto.PublishQuotationRequest
		mockSvc := &mockQuotationLifecycleService{
			publishQuotationFunc: func(ctx context.Context, cID, qID string, req dto.PublishQuotationRequest) error {
				receivedReq = req
				return nil
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		body, _ := json.Marshal(dto.PublishQuotationRequest{
			RevisionID:                revisionID,
			ContentVersion:            2,
			ConfirmLegalModified:      true,
			ConfirmCommercialModified: true,
		})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, revisionID, receivedReq.RevisionID)
		assert.Equal(t, 2, receivedReq.ContentVersion)
		assert.True(t, receivedReq.ConfirmLegalModified)
		assert.True(t, receivedReq.ConfirmCommercialModified)
	})
}

// Tests for Quotation Create Revision

func TestLifecycleHandler_QuotationCreateRevision(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()

	t.Run("missing company context returns 403", func(t *testing.T) {
		router := setupLifecycleRouter(&mockQuotationLifecycleService{}, nil, nil, "")
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid quotation ID returns 400", func(t *testing.T) {
		router := setupLifecycleRouter(&mockQuotationLifecycleService{}, nil, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/invalid/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("active draft already exists returns 409", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			createQuotationRevisionFunc: func(ctx context.Context, cID, qID string) (*dto.QuotationRevisionResponse, error) {
				return nil, service.ErrActiveDraftAlreadyExists
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "Active draft revision already exists")
	})

	t.Run("no published revision found returns 400", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			createQuotationRevisionFunc: func(ctx context.Context, cID, qID string) (*dto.QuotationRevisionResponse, error) {
				return nil, service.ErrNoPublishedRevision
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "No published revision found")
	})

	t.Run("quotation not found returns 404", func(t *testing.T) {
		mockSvc := &mockQuotationLifecycleService{
			createQuotationRevisionFunc: func(ctx context.Context, cID, qID string) (*dto.QuotationRevisionResponse, error) {
				return nil, service.ErrNotFound
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("successful revision creation returns 201", func(t *testing.T) {
		newRevID := uuid.New()
		mockSvc := &mockQuotationLifecycleService{
			createQuotationRevisionFunc: func(ctx context.Context, cID, qID string) (*dto.QuotationRevisionResponse, error) {
				return &dto.QuotationRevisionResponse{
					ID:            newRevID,
					VersionNumber: 2,
					Status:        "draft",
				}, nil
			},
		}
		router := setupLifecycleRouter(mockSvc, nil, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), newRevID.String())
	})
}

// Tests for Quotation Revisions List and Detail

func TestLifecycleHandler_QuotationRevisions_ListAndDetail(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()
	rev1ID := uuid.New()
	secretPrompt := map[string]interface{}{"secret": "top-secret-prompt"}

	mockSvc := &mockQuotationLifecycleService{
		listRevisionsFunc: func(ctx context.Context, cID, qID string) ([]dto.QuotationRevisionResponse, error) {
			if qID != quotationID {
				return nil, service.ErrNotFound
			}
			return []dto.QuotationRevisionResponse{
				{
					ID:            rev1ID,
					VersionNumber: 1,
					Status:        "published",
					AIRawResponse: secretPrompt, // Handler should mask this
				},
			}, nil
		},
		getQuotationHistoryFunc: func(ctx context.Context, cID, qID string) ([]dto.QuotationRevisionHistoryItem, error) {
			return []dto.QuotationRevisionHistoryItem{
				{
					ID:                rev1ID,
					RevisionNumber:    1,
					PublicationStatus: "published",
				},
			}, nil
		},
	}

	router := setupLifecycleRouter(mockSvc, nil, nil, companyID)

	t.Run("list revisions returns 200 and masks sensitive fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/quotations/"+quotationID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "top-secret-prompt")
	})

	t.Run("list revisions with view=history returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/quotations/"+quotationID+"/revisions?view=history", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Quotation revision history retrieved successfully")
	})

	t.Run("get revision detail returns 200 and masks sensitive fields", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/quotations/"+quotationID+"/revisions/"+rev1ID.String(), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), rev1ID.String())
		assert.NotContains(t, w.Body.String(), "top-secret-prompt")
	})

	t.Run("get revision detail returns 404 for unknown revision", func(t *testing.T) {
		unknownID := uuid.New().String()
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/quotations/"+quotationID+"/revisions/"+unknownID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Tests for Blueprint Publish

func TestLifecycleHandler_BlueprintPublish(t *testing.T) {
	companyID := uuid.New().String()
	blueprintID := uuid.New().String()
	revisionID := uuid.New().String()

	t.Run("missing company context returns 403", func(t *testing.T) {
		router := setupLifecycleRouter(nil, &mockBlueprintLifecycleDraftService{}, nil, "")
		body, _ := json.Marshal(dto.PublishBlueprintRequest{RevisionID: revisionID})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid blueprint ID returns 400", func(t *testing.T) {
		router := setupLifecycleRouter(nil, &mockBlueprintLifecycleDraftService{}, nil, companyID)
		body, _ := json.Marshal(dto.PublishBlueprintRequest{RevisionID: revisionID})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/not-a-uuid/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("partial publish without confirmation returns 400", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			publishBlueprintFunc: func(ctx context.Context, cID, bID string, req dto.PublishBlueprintRequest) error {
				return service.ErrBlueprintConfirmationRequired
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		body, _ := json.Marshal(dto.PublishBlueprintRequest{RevisionID: revisionID, ConfirmPartial: false})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Confirmation required for partial blueprint publish")
	})

	t.Run("blueprint draft not found returns 404", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			publishBlueprintFunc: func(ctx context.Context, cID, bID string, req dto.PublishBlueprintRequest) error {
				return service.ErrBlueprintDraftNotFound
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		body, _ := json.Marshal(dto.PublishBlueprintRequest{RevisionID: revisionID})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("valid publish with confirm_partial returns 200", func(t *testing.T) {
		var receivedReq dto.PublishBlueprintRequest
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			publishBlueprintFunc: func(ctx context.Context, cID, bID string, req dto.PublishBlueprintRequest) error {
				receivedReq = req
				return nil
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		body, _ := json.Marshal(dto.PublishBlueprintRequest{RevisionID: revisionID, ConfirmPartial: true})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/publish", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, receivedReq.ConfirmPartial)
	})
}

// Tests for Blueprint Create Revision

func TestLifecycleHandler_BlueprintCreateRevision(t *testing.T) {
	companyID := uuid.New().String()
	blueprintID := uuid.New().String()

	t.Run("missing company context returns 403", func(t *testing.T) {
		router := setupLifecycleRouter(nil, &mockBlueprintLifecycleDraftService{}, nil, "")
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("draft already exists returns 409", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			createBlueprintRevisionFunc: func(ctx context.Context, cID, bID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error) {
				return nil, service.ErrBlueprintActiveDraftAlreadyExists
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusConflict, w.Code)
		assert.Contains(t, w.Body.String(), "Active draft revision already exists")
	})

	t.Run("no published revision returns 400", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			createBlueprintRevisionFunc: func(ctx context.Context, cID, bID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error) {
				return nil, service.ErrBlueprintNoPublishedRevision
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("blueprint not found returns 404", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			createBlueprintRevisionFunc: func(ctx context.Context, cID, bID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error) {
				return nil, service.ErrBlueprintNotFound
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("successful revision creation returns 201", func(t *testing.T) {
		newRevID := uuid.New()
		var receivedReq dto.CreateBlueprintRevisionRequest
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			createBlueprintRevisionFunc: func(ctx context.Context, cID, bID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error) {
				receivedReq = req
				return &dto.BlueprintRevisionResponse{
					ID:            newRevID,
					BlueprintID:   uuid.MustParse(blueprintID),
					VersionNumber: 2,
					Status:        "draft",
				}, nil
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		body, _ := json.Marshal(dto.CreateBlueprintRevisionRequest{Mode: "regenerate_from_source"})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/revisions", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "regenerate_from_source", receivedReq.Mode)
		assert.Contains(t, w.Body.String(), newRevID.String())
	})
}

// Tests for Blueprint Revisions List and Detail

func TestLifecycleHandler_BlueprintRevisions_ListAndDetail(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	rev1ID := uuid.New()

	mockBpRead := &mockBlueprintLifecycleReadService{
		listRevisionsFunc: func(ctx context.Context, cID, bID uuid.UUID) ([]dto.BlueprintRevisionHistoryItem, error) {
			if bID != blueprintID {
				return nil, service.ErrBlueprintNotFound
			}
			return []dto.BlueprintRevisionHistoryItem{
				{
					ID:                 rev1ID,
					RevisionNumber:     1,
					PublicationStatus:  "published",
					CompletenessStatus: "complete",
					CompletenessMatrix: map[string]string{
						"context": "validated",
						"journey": "completed",
					},
					CreatedAt: time.Now(),
				},
			}, nil
		},
	}

	router := setupLifecycleRouter(nil, nil, mockBpRead, companyID.String())

	t.Run("list revisions returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints/"+blueprintID.String()+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), rev1ID.String())
		assert.Contains(t, w.Body.String(), `"journey":"completed"`)
	})

	t.Run("get revision detail returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints/"+blueprintID.String()+"/revisions/"+rev1ID.String(), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), rev1ID.String())
	})

	t.Run("get revision detail returns 404 for unknown revision", func(t *testing.T) {
		unknownID := uuid.New().String()
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints/"+blueprintID.String()+"/revisions/"+unknownID, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("list revisions returns 404 for unknown blueprint", func(t *testing.T) {
		unknownBpID := uuid.New().String()
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints/"+unknownBpID+"/revisions", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Tests for Blueprint Delete Draft

func TestLifecycleHandler_BlueprintDeleteDraft(t *testing.T) {
	companyID := uuid.New().String()
	blueprintID := uuid.New().String()

	t.Run("missing company context returns 403", func(t *testing.T) {
		router := setupLifecycleRouter(nil, &mockBlueprintLifecycleDraftService{}, nil, "")
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/blueprints/"+blueprintID+"/draft", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("invalid blueprint ID returns 400", func(t *testing.T) {
		router := setupLifecycleRouter(nil, &mockBlueprintLifecycleDraftService{}, nil, companyID)
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/blueprints/not-a-uuid/draft", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("draft not found returns 404", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			deleteDraftFunc: func(ctx context.Context, cID, bID string) error {
				return service.ErrBlueprintDraftNotFound
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/blueprints/"+blueprintID+"/draft", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("repository ErrBlueprintDraftNotFound returns 404", func(t *testing.T) {
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			deleteDraftFunc: func(ctx context.Context, cID, bID string) error {
				return repository.ErrBlueprintDraftNotFound
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/blueprints/"+blueprintID+"/draft", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("successful deletion returns 200 with message", func(t *testing.T) {
		deleted := false
		mockBpDraft := &mockBlueprintLifecycleDraftService{
			deleteDraftFunc: func(ctx context.Context, cID, bID string) error {
				deleted = true
				return nil
			},
		}
		router := setupLifecycleRouter(nil, mockBpDraft, nil, companyID)
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/blueprints/"+blueprintID+"/draft", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, deleted)
		assert.Contains(t, w.Body.String(), "Draft deleted successfully")
	})
}

// Full Router Integration Wiring Test

func TestPublishRoute_Wiring(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	qSvc := &mockQuotationLifecycleService{
		publishQuotationFunc: func(ctx context.Context, companyID, quotationID string, req dto.PublishQuotationRequest) error {
			return nil
		},
	}
	bpDraft := &mockBlueprintLifecycleDraftService{
		publishBlueprintFunc: func(ctx context.Context, companyID, blueprintID string, req dto.PublishBlueprintRequest) error {
			return nil
		},
	}
	bpRead := &mockBlueprintLifecycleReadService{}

	lifecycleHandler := handler.NewLifecycleHandler(qSvc, bpDraft, bpRead)

	mod := &quotation.Module{
		LifecycleHandler: lifecycleHandler,
	}

	v1 := router.Group("/quotation/v1")
	mod.SetupRoutes(v1)

	companyID := uuid.New().String()
	token, err := jwtpkg.GenerateToken(
		uuid.New().String(), companyID, "Acme Corp", "client-1", "acme",
		"user@acme.com", "testuser", "Test User",
		false, []string{"admin"},
	)
	require.NoError(t, err)

	quotationID := uuid.New().String()
	blueprintID := uuid.New().String()

	t.Run("quotation publish route reaches handler", func(t *testing.T) {
		body, _ := json.Marshal(dto.PublishQuotationRequest{
			RevisionID:     uuid.New().String(),
			ContentVersion: 1,
		})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID+"/publish", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Quotation published successfully")
	})

	t.Run("blueprint publish route reaches handler", func(t *testing.T) {
		body, _ := json.Marshal(dto.PublishBlueprintRequest{
			RevisionID: uuid.New().String(),
		})
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/blueprints/"+blueprintID+"/publish", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Blueprint published successfully")
	})
}
