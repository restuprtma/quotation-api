package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

// --- Mock Implementations ---

type mockBlueprintCreator struct {
	createFunc func(ctx context.Context, companyID, createdBy uuid.UUID, req dto.CreateBlueprintGenerationRequest) (*repository.CreateBlueprintGenerationResult, error)
	calls      int
	lastReq    dto.CreateBlueprintGenerationRequest
	lastCompID uuid.UUID
	lastUser   uuid.UUID
}

func (m *mockBlueprintCreator) CreateBlueprint(ctx context.Context, companyID, createdBy uuid.UUID, req dto.CreateBlueprintGenerationRequest) (*repository.CreateBlueprintGenerationResult, error) {
	m.calls++
	m.lastCompID = companyID
	m.lastUser = createdBy
	m.lastReq = req
	if m.createFunc != nil {
		return m.createFunc(ctx, companyID, createdBy, req)
	}
	return nil, errors.New("unimplemented mock create")
}

type mockBlueprintReader struct {
	getStatusFunc func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error)
	getDetailFunc func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintDetailResponse, error)
	listFunc      func(ctx context.Context, companyID uuid.UUID, quotationIDFilter *uuid.UUID) (*dto.BlueprintListResponse, error)
	statusCalls   int
	detailCalls   int
	listCalls     int
	lastCompID    uuid.UUID
	lastBpID      uuid.UUID
	lastQFilter   *uuid.UUID
}

func (m *mockBlueprintReader) GetStatus(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
	m.statusCalls++
	m.lastCompID = companyID
	m.lastBpID = blueprintID
	if m.getStatusFunc != nil {
		return m.getStatusFunc(ctx, companyID, blueprintID)
	}
	return nil, errors.New("unimplemented mock get status")
}

func (m *mockBlueprintReader) GetDetail(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintDetailResponse, error) {
	m.detailCalls++
	m.lastCompID = companyID
	m.lastBpID = blueprintID
	if m.getDetailFunc != nil {
		return m.getDetailFunc(ctx, companyID, blueprintID)
	}
	return nil, errors.New("unimplemented mock get detail")
}

func (m *mockBlueprintReader) List(ctx context.Context, companyID uuid.UUID, quotationIDFilter *uuid.UUID) (*dto.BlueprintListResponse, error) {
	m.listCalls++
	m.lastCompID = companyID
	m.lastQFilter = quotationIDFilter
	if m.listFunc != nil {
		return m.listFunc(ctx, companyID, quotationIDFilter)
	}
	return nil, errors.New("unimplemented mock list")
}

type mockBlueprintPipelineRunner struct {
	continueFunc  func(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error)
	retryFunc     func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*service.BlueprintPipelineResult, error)
	manualFunc    func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error)
	continueCalls int
	retryCalls    int
	manualCalls   int
	lastCompID    uuid.UUID
	lastBpID      uuid.UUID
	lastSection   domain.BlueprintSectionType
	lastRevID     uuid.UUID
	lastVersion   int
	lastPayload   json.RawMessage
}

func (m *mockBlueprintPipelineRunner) Continue(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
	m.continueCalls++
	m.lastCompID = companyID
	m.lastBpID = blueprintID
	if m.continueFunc != nil {
		return m.continueFunc(ctx, companyID, blueprintID)
	}
	return nil, errors.New("unimplemented mock continue")
}

func (m *mockBlueprintPipelineRunner) RetrySection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*service.BlueprintPipelineResult, error) {
	m.retryCalls++
	m.lastCompID = companyID
	m.lastBpID = blueprintID
	m.lastSection = sectionType
	if m.retryFunc != nil {
		return m.retryFunc(ctx, companyID, blueprintID, sectionType)
	}
	return nil, errors.New("unimplemented mock retry")
}

func (m *mockBlueprintPipelineRunner) ManualCompleteSectionAtVersion(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
	m.manualCalls++
	m.lastCompID = companyID
	m.lastBpID = blueprintID
	m.lastSection = sectionType
	m.lastRevID = revisionID
	m.lastVersion = expectedVersion
	m.lastPayload = payload
	if m.manualFunc != nil {
		return m.manualFunc(ctx, companyID, blueprintID, sectionType, revisionID, expectedVersion, payload)
	}
	return nil, errors.New("unimplemented mock manual")
}

type mockBlueprintDraftSaver struct {
	saveDraftFunc  func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error)
	saveDraftCalls int
	lastCompID     uuid.UUID
	lastBpID       uuid.UUID
	lastSection    domain.BlueprintSectionType
	lastRevID      uuid.UUID
	lastVersion    int
	lastPayload    json.RawMessage
}

func (m *mockBlueprintDraftSaver) SaveDraftSectionAtVersion(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
	m.saveDraftCalls++
	m.lastCompID = companyID
	m.lastBpID = blueprintID
	m.lastSection = sectionType
	m.lastRevID = revisionID
	m.lastVersion = expectedVersion
	m.lastPayload = payload
	if m.saveDraftFunc != nil {
		return m.saveDraftFunc(ctx, companyID, blueprintID, sectionType, revisionID, expectedVersion, payload)
	}
	return nil, errors.New("unimplemented mock save draft")
}

// --- Router and Test Helpers ---

type blueprintHandlerTestRig struct {
	creator    *mockBlueprintCreator
	reader     *mockBlueprintReader
	pipeline   *mockBlueprintPipelineRunner
	draftSaver *mockBlueprintDraftSaver
	handler    *handler.BlueprintGenerationHandler
	router     *gin.Engine
	companyID  uuid.UUID
	userID     uuid.UUID
}

func setupBlueprintTestRig() *blueprintHandlerTestRig {
	gin.SetMode(gin.TestMode)
	rig := &blueprintHandlerTestRig{
		creator:    &mockBlueprintCreator{},
		reader:     &mockBlueprintReader{},
		pipeline:   &mockBlueprintPipelineRunner{},
		draftSaver: &mockBlueprintDraftSaver{},
		companyID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		userID:     uuid.MustParse("22222222-2222-2222-2222-222222222222"),
	}
	rig.handler = handler.NewBlueprintGenerationHandler(rig.creator, rig.reader, rig.pipeline, rig.draftSaver)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		comp := rig.companyID
		user := rig.userID
		if rawComp := c.GetHeader("X-Test-Company"); rawComp != "" {
			if id, err := uuid.Parse(rawComp); err == nil {
				comp = id
			} else if rawComp == "none" {
				comp = uuid.Nil
			}
		}
		if rawUser := c.GetHeader("X-Test-User"); rawUser != "" {
			if id, err := uuid.Parse(rawUser); err == nil {
				user = id
			} else if rawUser == "none" {
				user = uuid.Nil
			}
		}

		claims := &jwtpkg.Claims{}
		if comp != uuid.Nil {
			claims.CompanyID = comp.String()
		}
		if user != uuid.Nil {
			claims.UserID = user.String()
		}
		c.Set(middleware.UserContextKey, claims)
		c.Next()
	})

	blueprints := router.Group("/quotation/v1/blueprints")
	{
		blueprints.POST("/generate", rig.handler.Generate)
		blueprints.GET("/:id/generation-status", rig.handler.GetStatus)
		blueprints.GET("/:id", rig.handler.GetDetail)
		blueprints.POST("/:id/continue", rig.handler.Continue)
		blueprints.POST("/:id/sections/:section/retry", rig.handler.RetrySection)
		blueprints.PUT("/:id/sections/:section/manual", rig.handler.ManualCompleteSection)
		blueprints.PUT("/:id/sections/:section/draft", rig.handler.SaveDraftSection)
		blueprints.GET("", rig.handler.List)
	}
	rig.router = router
	return rig
}

func sampleValidCreateRequest() dto.CreateBlueprintGenerationRequest {
	return dto.CreateBlueprintGenerationRequest{
		IdempotencyKey:            uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"),
		QuotationID:               uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"),
		SourceQuotationRevisionID: uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc"),
		Title:                     "E-Commerce Blueprint",
		BusinessGoals:             []string{"Increase conversions by 20%"},
		Actors:                    []string{"Customer", "Admin"},
		Channels:                  []string{"Web", "Mobile App"},
		BusinessPhases:            []string{"Browse", "Checkout", "Fulfillment"},
		BusinessRules:             []string{"Orders over $50 get free shipping"},
		KnownStates:               []string{"cart_active", "paid", "shipped"},
		Integrations:              []string{"Stripe", "ShipStation"},
		TechnologyPreferences:     []string{"Go", "PostgreSQL"},
		OperationalConstraints:    []string{"99.9% uptime requirement"},
		AdditionalNotes:           "Deliver in 6 weeks",
	}
}

func sampleStatusResponse(bpID, revID uuid.UUID) *dto.BlueprintStatusResponse {
	return &dto.BlueprintStatusResponse{
		BlueprintID:        bpID,
		RevisionID:         revID,
		Title:              "E-Commerce Blueprint",
		GenerationStatus:   "pending",
		PublicationStatus:  "draft",
		CompletenessStatus: "partial",
		ContentVersion:     1,
		Sections: []dto.BlueprintSectionStatusResponse{
			{Type: "context", Status: "validated", Attempts: 0, UpdatedAt: time.Now()},
			{Type: "journey", Status: "pending", Attempts: 0, UpdatedAt: time.Now()},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

type handlerEnvelope struct {
	Data    json.RawMessage     `json:"data"`
	Message string              `json:"message"`
	Errors  map[string][]string `json:"errors"`
}

func parseEnvelope(t *testing.T, w *httptest.ResponseRecorder) handlerEnvelope {
	t.Helper()
	var env handlerEnvelope
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err, "failed to unmarshal envelope: %s", w.Body.String())
	return env
}

func getDetailCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	env := parseEnvelope(t, w)
	if len(env.Errors["detail"]) == 0 {
		return ""
	}
	return env.Errors["detail"][0]
}

func execRequest(router *gin.Engine, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// ---------------------------------------------------------------------------
// 2.01 & 2.02: Route Contracts, Mock Tests (201/200, Detail, Status, List, Continue, Retry, Manual)
// ---------------------------------------------------------------------------

func TestBlueprintHandler_Generate_NewCreate_Returns201Created(t *testing.T) {
	rig := setupBlueprintTestRig()
	reqDTO := sampleValidCreateRequest()
	bpID := uuid.New()
	revID := uuid.New()

	rig.creator.createFunc = func(ctx context.Context, companyID, createdBy uuid.UUID, req dto.CreateBlueprintGenerationRequest) (*repository.CreateBlueprintGenerationResult, error) {
		return &repository.CreateBlueprintGenerationResult{
			Blueprint: domain.Blueprint{
				ID:        bpID,
				CompanyID: companyID,
				Title:     req.Title,
			},
			Revision: domain.BlueprintRevision{
				ID:             revID,
				BlueprintID:    bpID,
				ContentVersion: 1,
			},
			IdempotentReplay: false,
		}, nil
	}
	rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
		return sampleStatusResponse(blueprintID, revID), nil
	}

	body, _ := json.Marshal(reqDTO)
	w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/generate", body, nil)

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, 1, rig.creator.calls)
	assert.Equal(t, 1, rig.reader.statusCalls)
	assert.Equal(t, rig.companyID, rig.creator.lastCompID)
	assert.Equal(t, rig.userID, rig.creator.lastUser)
	assert.Equal(t, reqDTO.Title, rig.creator.lastReq.Title)

	var status dto.BlueprintStatusResponse
	env := parseEnvelope(t, w)
	require.NoError(t, json.Unmarshal(env.Data, &status))
	assert.Equal(t, bpID, status.BlueprintID)
	assert.Equal(t, revID, status.RevisionID)
}

func TestBlueprintHandler_Generate_IdempotentReplay_Returns200OK(t *testing.T) {
	rig := setupBlueprintTestRig()
	reqDTO := sampleValidCreateRequest()
	bpID := uuid.New()
	revID := uuid.New()

	rig.creator.createFunc = func(ctx context.Context, companyID, createdBy uuid.UUID, req dto.CreateBlueprintGenerationRequest) (*repository.CreateBlueprintGenerationResult, error) {
		return &repository.CreateBlueprintGenerationResult{
			Blueprint: domain.Blueprint{
				ID:        bpID,
				CompanyID: companyID,
				Title:     req.Title,
			},
			Revision: domain.BlueprintRevision{
				ID:             revID,
				BlueprintID:    bpID,
				ContentVersion: 1,
			},
			IdempotentReplay: true,
		}, nil
	}
	rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
		return sampleStatusResponse(blueprintID, revID), nil
	}

	body, _ := json.Marshal(reqDTO)
	w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/generate", body, nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rig.creator.calls)
	assert.Equal(t, 1, rig.reader.statusCalls)
}

func TestBlueprintHandler_GetStatus_Returns200OK(t *testing.T) {
	rig := setupBlueprintTestRig()
	bpID := uuid.New()
	revID := uuid.New()

	rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
		return sampleStatusResponse(blueprintID, revID), nil
	}

	w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints/"+bpID.String()+"/generation-status", nil, nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rig.reader.statusCalls)
	assert.Equal(t, rig.companyID, rig.reader.lastCompID)
	assert.Equal(t, bpID, rig.reader.lastBpID)

	var status dto.BlueprintStatusResponse
	env := parseEnvelope(t, w)
	require.NoError(t, json.Unmarshal(env.Data, &status))
	assert.Equal(t, bpID, status.BlueprintID)
}

func TestBlueprintHandler_GetDetail_Returns200OK(t *testing.T) {
	rig := setupBlueprintTestRig()
	bpID := uuid.New()
	revID := uuid.New()
	qID := uuid.New()

	rig.reader.getDetailFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintDetailResponse, error) {
		return &dto.BlueprintDetailResponse{
			Status:                *sampleStatusResponse(blueprintID, revID),
			QuotationID:           qID,
			ContentJSON:           json.RawMessage(`{"journey":{}}`),
			ValidationResult:      json.RawMessage(`{"valid":true}`),
			CrossValidationResult: json.RawMessage(`{"valid":true}`),
		}, nil
	}

	w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints/"+bpID.String(), nil, nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rig.reader.detailCalls)
	assert.Equal(t, rig.companyID, rig.reader.lastCompID)
	assert.Equal(t, bpID, rig.reader.lastBpID)

	var detail dto.BlueprintDetailResponse
	env := parseEnvelope(t, w)
	require.NoError(t, json.Unmarshal(env.Data, &detail))
	assert.Equal(t, bpID, detail.Status.BlueprintID)
	assert.Equal(t, qID, detail.QuotationID)
	assert.JSONEq(t, `{"journey":{}}`, string(detail.ContentJSON))
}

func TestBlueprintHandler_Continue_Success_Returns200OK(t *testing.T) {
	rig := setupBlueprintTestRig()
	bpID := uuid.New()
	revID := uuid.New()

	rig.pipeline.continueFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
		return &service.BlueprintPipelineResult{
			BlueprintID:      blueprintID,
			RevisionID:       revID,
			Section:          domain.BlueprintSectionJourney,
			SectionStatus:    domain.BlueprintSectionCompleted,
			GenerationStatus: domain.BlueprintGenerationInProgress,
			ContentVersion:   1,
			Attempt:          1,
		}, nil
	}
	rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
		return sampleStatusResponse(blueprintID, revID), nil
	}

	w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/"+bpID.String()+"/continue", nil, nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rig.pipeline.continueCalls)
	assert.Equal(t, 1, rig.reader.statusCalls)
	assert.Equal(t, rig.companyID, rig.pipeline.lastCompID)
	assert.Equal(t, bpID, rig.pipeline.lastBpID)
}

func TestBlueprintHandler_Continue_CompletedSentinel_TreatedAsSuccess_Returns200OK(t *testing.T) {
	rig := setupBlueprintTestRig()
	bpID := uuid.New()
	revID := uuid.New()

	rig.pipeline.continueFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
		return nil, service.ErrBlueprintPipelineCompleted
	}
	rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
		st := sampleStatusResponse(blueprintID, revID)
		st.GenerationStatus = "completed"
		return st, nil
	}

	w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/"+bpID.String()+"/continue", nil, nil)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, rig.pipeline.continueCalls)
	assert.Equal(t, 1, rig.reader.statusCalls)
}

func TestBlueprintHandler_RetrySection_ValidProviderSections_Returns200OK(t *testing.T) {
	sections := []domain.BlueprintSectionType{
		domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
	}

	for _, sec := range sections {
		t.Run(string(sec), func(t *testing.T) {
			rig := setupBlueprintTestRig()
			bpID := uuid.New()
			revID := uuid.New()

			rig.pipeline.retryFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*service.BlueprintPipelineResult, error) {
				return &service.BlueprintPipelineResult{
					BlueprintID:      blueprintID,
					RevisionID:       revID,
					Section:          sectionType,
					SectionStatus:    domain.BlueprintSectionCompleted,
					GenerationStatus: domain.BlueprintGenerationInProgress,
					ContentVersion:   1,
					Attempt:          2,
				}, nil
			}
			rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
				return sampleStatusResponse(blueprintID, revID), nil
			}

			w := execRequest(rig.router, http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/%s/retry", bpID.String(), sec), nil, nil)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, 1, rig.pipeline.retryCalls)
			assert.Equal(t, sec, rig.pipeline.lastSection)
			assert.Equal(t, rig.companyID, rig.pipeline.lastCompID)
			assert.Equal(t, bpID, rig.pipeline.lastBpID)
		})
	}
}

func TestBlueprintHandler_ManualCompleteSection_Success_Returns200OK(t *testing.T) {
	sections := []domain.BlueprintSectionType{
		domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS,
	}

	for _, sec := range sections {
		t.Run(string(sec), func(t *testing.T) {
			rig := setupBlueprintTestRig()
			bpID := uuid.New()
			revID := uuid.New()

			manualReq := dto.ManualCompleteBlueprintSectionRequest{
				RevisionID:     revID,
				ContentVersion: 1,
				Content:        json.RawMessage(`{"manual_data":"valid_content"}`),
			}

			rig.pipeline.manualFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
				return &service.BlueprintPipelineResult{
					BlueprintID:      blueprintID,
					RevisionID:       revisionID,
					Section:          sectionType,
					SectionStatus:    domain.BlueprintSectionManualEdit,
					GenerationStatus: domain.BlueprintGenerationInProgress,
					ContentVersion:   expectedVersion + 1,
					Attempt:          1,
				}, nil
			}
			rig.reader.getStatusFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
				return sampleStatusResponse(blueprintID, revID), nil
			}

			body, _ := json.Marshal(manualReq)
			w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/%s/manual", bpID.String(), sec), body, nil)

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, 1, rig.pipeline.manualCalls)
			assert.Equal(t, sec, rig.pipeline.lastSection)
			assert.Equal(t, revID, rig.pipeline.lastRevID)
			assert.Equal(t, 1, rig.pipeline.lastVersion)
			assert.JSONEq(t, `{"manual_data":"valid_content"}`, string(rig.pipeline.lastPayload))
		})
	}
}

func TestBlueprintHandler_List_WithoutAndWithQuotationFilter(t *testing.T) {
	t.Run("list all without filter", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		qID := uuid.New()

		rig.reader.listFunc = func(ctx context.Context, companyID uuid.UUID, quotationIDFilter *uuid.UUID) (*dto.BlueprintListResponse, error) {
			assert.Nil(t, quotationIDFilter)
			return &dto.BlueprintListResponse{
				Items: []dto.BlueprintListItemResponse{
					{
						BlueprintID:      bpID,
						QuotationID:      qID,
						Title:            "Blueprint 1",
						GenerationStatus: "completed",
						UpdatedAt:        time.Now(),
					},
				},
			}, nil
		}

		w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints", nil, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, 1, rig.reader.listCalls)
		assert.Equal(t, rig.companyID, rig.reader.lastCompID)

		var listResp dto.BlueprintListResponse
		env := parseEnvelope(t, w)
		require.NoError(t, json.Unmarshal(env.Data, &listResp))
		assert.Len(t, listResp.Items, 1)
		assert.Equal(t, bpID, listResp.Items[0].BlueprintID)
	})

	t.Run("list with quotation_id filter", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		filterQID := uuid.New()
		bpID := uuid.New()

		rig.reader.listFunc = func(ctx context.Context, companyID uuid.UUID, quotationIDFilter *uuid.UUID) (*dto.BlueprintListResponse, error) {
			require.NotNil(t, quotationIDFilter)
			assert.Equal(t, filterQID, *quotationIDFilter)
			return &dto.BlueprintListResponse{
				Items: []dto.BlueprintListItemResponse{
					{
						BlueprintID:      bpID,
						QuotationID:      filterQID,
						Title:            "Filtered Blueprint",
						GenerationStatus: "pending",
						UpdatedAt:        time.Now(),
					},
				},
			}, nil
		}

		w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints?quotation_id="+filterQID.String(), nil, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, 1, rig.reader.listCalls)
		assert.NotNil(t, rig.reader.lastQFilter)
		assert.Equal(t, filterQID, *rig.reader.lastQFilter)
	})
}

// ---------------------------------------------------------------------------
// 2.03: Comprehensive Error Mapping Table Tests & Secret Redaction
// ---------------------------------------------------------------------------

func TestBlueprintHandler_ErrorMappingTable(t *testing.T) {
	testCases := []struct {
		name           string
		triggerErr     error
		expectedStatus int
		expectedDetail string
	}{
		// 400 Bad Request
		{"ErrBlueprintGenerationInvalidInput", service.ErrBlueprintGenerationInvalidInput, http.StatusBadRequest, "blueprint_invalid_input"},
		{"ErrBlueprintPipelineInvalidInput", service.ErrBlueprintPipelineInvalidInput, http.StatusBadRequest, "blueprint_invalid_input"},
		{"ErrBlueprintInvalidCreateParams", repository.ErrBlueprintInvalidCreateParams, http.StatusBadRequest, "blueprint_invalid_input"},

		// 404 Not Found
		{"ErrBlueprintSourceNotFound", repository.ErrBlueprintSourceNotFound, http.StatusNotFound, "source_not_found"},
		{"ErrBlueprintGenerationTenantMismatch", service.ErrBlueprintGenerationTenantMismatch, http.StatusNotFound, "source_not_found"},
		{"ErrBlueprintNotFound (repo)", repository.ErrBlueprintNotFound, http.StatusNotFound, "blueprint_not_found"},
		{"ErrBlueprintNotFound (service)", service.ErrBlueprintNotFound, http.StatusNotFound, "blueprint_not_found"},
		{"ErrBlueprintPipelineNotFound", service.ErrBlueprintPipelineNotFound, http.StatusNotFound, "blueprint_not_found"},

		// 409 Conflict
		{"ErrBlueprintAlreadyExists", repository.ErrBlueprintAlreadyExists, http.StatusConflict, "blueprint_already_exists"},
		{"ErrBlueprintIdempotencyPayloadMismatch", repository.ErrBlueprintIdempotencyPayloadMismatch, http.StatusConflict, "idempotency_payload_mismatch"},
		{"ErrBlueprintPipelineBusy", service.ErrBlueprintPipelineBusy, http.StatusConflict, "pipeline_busy"},
		{"ErrBlueprintClaimBusy", repository.ErrBlueprintClaimBusy, http.StatusConflict, "pipeline_busy"},
		{"ErrBlueprintPipelineBlocked", service.ErrBlueprintPipelineBlocked, http.StatusConflict, "pipeline_blocked"},
		{"ErrBlueprintNoEligibleSection", repository.ErrBlueprintNoEligibleSection, http.StatusConflict, "pipeline_blocked"},
		{"ErrBlueprintPipelineRecoveryRequired", service.ErrBlueprintPipelineRecoveryRequired, http.StatusConflict, "recovery_required"},
		{"ErrBlueprintRecoveryRequired", repository.ErrBlueprintRecoveryRequired, http.StatusConflict, "recovery_required"},
		{"ErrBlueprintSectionRecoveryRequired", service.ErrBlueprintSectionRecoveryRequired, http.StatusConflict, "section_recovery_required"},
		{"ErrBlueprintRetryNotAllowed", service.ErrBlueprintRetryNotAllowed, http.StatusConflict, "retry_not_allowed"},
		{"ErrBlueprintSectionNotRetryable", repository.ErrBlueprintSectionNotRetryable, http.StatusConflict, "retry_not_allowed"},
		{"ErrBlueprintAttemptLimitReached", service.ErrBlueprintAttemptLimitReached, http.StatusConflict, "attempt_limit_reached"},
		{"ErrBlueprintAttemptLimit", repository.ErrBlueprintAttemptLimit, http.StatusConflict, "attempt_limit_reached"},
		{"ErrBlueprintManualNotAllowed", service.ErrBlueprintManualNotAllowed, http.StatusConflict, "manual_completion_not_allowed"},
		{"ErrBlueprintExecutionConflict", service.ErrBlueprintExecutionConflict, http.StatusConflict, "execution_conflict"},
		{"ErrBlueprintClaimConflict", repository.ErrBlueprintClaimConflict, http.StatusConflict, "execution_conflict"},
		{"ErrBlueprintContentConflict", repository.ErrBlueprintContentConflict, http.StatusConflict, "execution_conflict"},
		{"ErrBlueprintManualConflict", repository.ErrBlueprintManualConflict, http.StatusConflict, "execution_conflict"},

		// 422 Unprocessable Entity
		{"ErrBlueprintSourceIneligible", repository.ErrBlueprintSourceIneligible, http.StatusUnprocessableEntity, "source_ineligible"},
		{"ErrSkillNotFound", repository.ErrSkillNotFound, http.StatusUnprocessableEntity, "skill_not_configured"},
		{"ErrBlueprintInvalidSchema", service.ErrBlueprintInvalidSchema, http.StatusUnprocessableEntity, "invalid_schema"},
		{"BlueprintManualValidationError invalid schema", &service.BlueprintManualValidationError{BaseErr: service.ErrBlueprintInvalidSchema, SafeOutcome: "invalid_schema"}, http.StatusUnprocessableEntity, "invalid_schema"},
		{"ErrBlueprintSecurityFailure", service.ErrBlueprintSecurityFailure, http.StatusUnprocessableEntity, "security_failure"},
		{"BlueprintManualValidationError security", &service.BlueprintManualValidationError{BaseErr: service.ErrBlueprintSecurityFailure, SafeOutcome: "security_failure"}, http.StatusUnprocessableEntity, "security_failure"},
		{"ErrBlueprintCrossValidationFailure", service.ErrBlueprintCrossValidationFailure, http.StatusUnprocessableEntity, "cross_validation_error"},
		{"ErrBlueprintProviderConfiguration", service.ErrBlueprintProviderConfiguration, http.StatusUnprocessableEntity, "provider_configuration"},
		{"ErrBlueprintProviderInvalidCredential", service.ErrBlueprintProviderInvalidCredential, http.StatusUnprocessableEntity, "provider_invalid_credential"},

		// 429 Too Many Requests
		{"ErrBlueprintProviderRateLimited", service.ErrBlueprintProviderRateLimited, http.StatusTooManyRequests, "provider_rate_limited"},

		// 503 Service Unavailable
		{"ErrBlueprintProviderUnavailable", service.ErrBlueprintProviderUnavailable, http.StatusServiceUnavailable, "provider_unavailable"},

		// 504 Gateway Timeout
		{"ErrBlueprintProviderTimeout", service.ErrBlueprintProviderTimeout, http.StatusGatewayTimeout, "provider_timeout"},

		// 500 Internal Server Error
		{"ErrBlueprintPersistenceFailure", service.ErrBlueprintPersistenceFailure, http.StatusInternalServerError, "persistence_failure"},
		{"ErrCorruptBlueprintContent", service.ErrCorruptBlueprintContent, http.StatusInternalServerError, "persistence_failure"},
		{"Unexpected / Unmapped Error", errors.New("unexpected database driver error"), http.StatusInternalServerError, ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rig := setupBlueprintTestRig()
			bpID := uuid.New()

			// Test error translation via Continue endpoint
			rig.pipeline.continueFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
				return nil, tc.triggerErr
			}

			w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/"+bpID.String()+"/continue", nil, nil)
			require.Equal(t, tc.expectedStatus, w.Code, "unexpected status code for %s: %s", tc.name, w.Body.String())

			detail := getDetailCode(t, w)
			assert.Equal(t, tc.expectedDetail, detail, "unexpected error detail for %s", tc.name)
		})
	}
}

func TestBlueprintHandler_SecretRedaction_NoLeakage(t *testing.T) {
	secretMarkers := []string{
		"SECRET_KEY_abc123",
		"API_TOKEN_XYZ987",
		"postgres://app_user:s3cr3t_p@ssw0rd@db.internal:5432/quotations",
		"goroutine 42 [running]:",
		"runtime/panic.go:838",
		"SELECT * FROM credentials WHERE secret_token =",
	}

	for _, secret := range secretMarkers {
		t.Run("redacts_"+secret[:min(15, len(secret))], func(t *testing.T) {
			rig := setupBlueprintTestRig()
			bpID := uuid.New()

			// Wrap a wrapped error with raw secret text
			rawErr := fmt.Errorf("fatal failure occurred: %s: %w", secret, service.ErrBlueprintPersistenceFailure)
			rig.pipeline.continueFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
				return nil, rawErr
			}

			w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/"+bpID.String()+"/continue", nil, nil)
			require.Equal(t, http.StatusInternalServerError, w.Code)

			body := w.Body.String()
			assert.NotContains(t, body, secret, "response MUST NEVER contain secret marker")
		})
	}

	t.Run("unmapped error redacts raw message", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		rawSecretErr := errors.New("CRITICAL_LEAK: SECRET_KEY_99999 connection to internal host 10.0.0.1 failed")

		rig.pipeline.continueFunc = func(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
			return nil, rawSecretErr
		}

		w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/"+bpID.String()+"/continue", nil, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)

		body := w.Body.String()
		assert.NotContains(t, body, "SECRET_KEY_99999")
		assert.NotContains(t, body, "10.0.0.1")
		assert.NotContains(t, body, "CRITICAL_LEAK")

		env := parseEnvelope(t, w)
		assert.Equal(t, "Internal server error", env.Message)
		assert.Empty(t, env.Errors["detail"])
	})
}

// ---------------------------------------------------------------------------
// 2.04: Invalid Input Tests (UUIDs, Section allowlist, Unknown JSON fields,
// Trailing tokens, Max 1MB, Version bounds, Empty Content, Non-empty empty bodies)
// ---------------------------------------------------------------------------

func TestBlueprintHandler_InvalidInputs_Returns400WithZeroServiceCalls(t *testing.T) {
	t.Run("malformed UUID in path for status", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints/not-a-valid-uuid/generation-status", nil, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.reader.statusCalls)
	})

	t.Run("malformed UUID in path for detail", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints/invalid-id", nil, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.reader.detailCalls)
	})

	t.Run("malformed UUID in query for list", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		w := execRequest(rig.router, http.MethodGet, "/quotation/v1/blueprints?quotation_id=bad-uuid-value", nil, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.reader.listCalls)
	})

	t.Run("invalid section param in retry", func(t *testing.T) {
		invalidSections := []string{"context", "cross_validation", "unknown_section", "journey123", "invalid_section"}
		for _, invalidSec := range invalidSections {
			t.Run(invalidSec, func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				w := execRequest(rig.router, http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/%s/retry", bpID.String(), invalidSec), nil, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.pipeline.retryCalls)
			})
		}
	})

	t.Run("invalid section param in manual", func(t *testing.T) {
		invalidSections := []string{"context", "cross_validation", "arbitrary_section"}
		for _, invalidSec := range invalidSections {
			t.Run(invalidSec, func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				body := []byte(`{"revision_id":"` + uuid.New().String() + `","content_version":1,"content":{"foo":"bar"}}`)
				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/%s/manual", bpID.String(), invalidSec), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.pipeline.manualCalls)
			})
		}
	})

	t.Run("unknown JSON fields in generate request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		payload := map[string]any{
			"idempotency_key":              uuid.New().String(),
			"quotation_id":                 uuid.New().String(),
			"source_quotation_revision_id": uuid.New().String(),
			"title":                        "Valid Title",
			"business_goals":               []string{"Goal 1"},
			"actors":                       []string{"Actor 1"},
			"business_phases":              []string{"Phase 1"},
			"unauthorized_extra_field":     "exploit",
		}
		body, _ := json.Marshal(payload)
		w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/generate", body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.creator.calls)
	})

	t.Run("unknown JSON fields in manual request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		payload := map[string]any{
			"revision_id":     uuid.New().String(),
			"content_version": 1,
			"content":         map[string]any{"data": "val"},
			"unexpected_key":  "bad",
		}
		body, _ := json.Marshal(payload)
		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.pipeline.manualCalls)
	})

	t.Run("trailing multiple JSON tokens in generate request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		reqDTO := sampleValidCreateRequest()
		firstJSON, _ := json.Marshal(reqDTO)
		twoJSONs := append(firstJSON, []byte(`{"second":"token"}`)...)

		w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/generate", twoJSONs, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.creator.calls)
	})

	t.Run("trailing multiple JSON tokens in manual request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		manualReq := dto.ManualCompleteBlueprintSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"a":1}`),
		}
		firstJSON, _ := json.Marshal(manualReq)
		twoJSONs := append(firstJSON, []byte(`{"extra":true}`)...)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID.String()), twoJSONs, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.pipeline.manualCalls)
	})

	t.Run("body exceeds 1MB in generate request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		largeNotes := strings.Repeat("A", (1<<20)+100) // > 1MB
		reqDTO := sampleValidCreateRequest()
		reqDTO.AdditionalNotes = largeNotes
		body, _ := json.Marshal(reqDTO)

		w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/generate", body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.creator.calls)
	})

	t.Run("body exceeds 1MB in manual request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		largeContent := `{"big":"` + strings.Repeat("x", (1<<20)+100) + `"}`
		manualReq := dto.ManualCompleteBlueprintSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(largeContent),
		}
		body, _ := json.Marshal(manualReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.pipeline.manualCalls)
	})

	t.Run("manual payload invalid content_version <= 0", func(t *testing.T) {
		versions := []int{0, -1, -99}
		for _, ver := range versions {
			t.Run(fmt.Sprintf("version_%d", ver), func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				manualReq := dto.ManualCompleteBlueprintSectionRequest{
					RevisionID:     uuid.New(),
					ContentVersion: ver,
					Content:        json.RawMessage(`{"data":"ok"}`),
				}
				body, _ := json.Marshal(manualReq)
				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID.String()), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.pipeline.manualCalls)
			})
		}
	})

	t.Run("manual payload nil revision_id", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		manualReq := dto.ManualCompleteBlueprintSectionRequest{
			RevisionID:     uuid.Nil,
			ContentVersion: 1,
			Content:        json.RawMessage(`{"data":"ok"}`),
		}
		body, _ := json.Marshal(manualReq)
		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.pipeline.manualCalls)
	})

	t.Run("manual payload empty or null content", func(t *testing.T) {
		contents := []string{"", "   ", "null"}
		for _, c := range contents {
			t.Run("content_"+c, func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				var raw json.RawMessage
				if c != "" {
					raw = json.RawMessage(c)
				}
				manualReq := dto.ManualCompleteBlueprintSectionRequest{
					RevisionID:     uuid.New(),
					ContentVersion: 1,
					Content:        raw,
				}
				body, _ := json.Marshal(manualReq)
				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/manual", bpID.String()), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.pipeline.manualCalls)
			})
		}
	})

	t.Run("non-empty body in continue request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		body := []byte(`{"unexpected":"body"}`)
		w := execRequest(rig.router, http.MethodPost, "/quotation/v1/blueprints/"+bpID.String()+"/continue", body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.pipeline.continueCalls)
	})

	t.Run("non-empty body in retry request", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		body := []byte(`{"unexpected":"body"}`)
		w := execRequest(rig.router, http.MethodPost, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/retry", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.pipeline.retryCalls)
	})
}

func TestBlueprintGenerationHandler_SaveDraftSection(t *testing.T) {
	t.Run("valid journey draft save returns 200 with canonical detail response", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		revID := uuid.New()
		journeyContent := `{"journey":{"phases":[{"id":"p1","name":"Discovery","order":1}],"actors":[{"id":"a1","name":"User","role":"End User"}],"activities":[{"id":"act1","phase_id":"p1","actor_id":"a1","name":"Browse","order":1}]}}`

		rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, sec domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
			assert.Equal(t, rig.companyID, compID)
			assert.Equal(t, bpID, bID)
			assert.Equal(t, domain.BlueprintSectionJourney, sec)
			assert.Equal(t, revID, rID)
			assert.Equal(t, 2, ver)
			assert.JSONEq(t, journeyContent, string(payload))
			return &service.BlueprintPipelineResult{
				BlueprintID:      bpID,
				RevisionID:       revID,
				Section:          domain.BlueprintSectionJourney,
				SectionStatus:    domain.BlueprintSectionManualEdit,
				GenerationStatus: domain.BlueprintGenerationInProgress,
				ContentVersion:   3,
			}, nil
		}

		expectedDetail := &dto.BlueprintDetailResponse{
			Status: dto.BlueprintStatusResponse{
				BlueprintID:        bpID,
				RevisionID:         revID,
				Title:              "E-Commerce System",
				GenerationStatus:   "in_progress",
				PublicationStatus:  "draft",
				CompletenessStatus: "partial",
				ContentVersion:     3,
				Sections: []dto.BlueprintSectionStatusResponse{
					{Type: "journey", Status: "manual_edit", Attempts: 1, UpdatedAt: time.Now()},
				},
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
			QuotationID: uuid.New(),
			ContentJSON: json.RawMessage(`{"journey":{"phases":[{"id":"p1","name":"Discovery","order":1}]}}`),
		}

		rig.reader.getDetailFunc = func(ctx context.Context, compID, bID uuid.UUID) (*dto.BlueprintDetailResponse, error) {
			assert.Equal(t, rig.companyID, compID)
			assert.Equal(t, bpID, bID)
			return expectedDetail, nil
		}

		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     revID,
			ContentVersion: 2,
			Content:        json.RawMessage(journeyContent),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 1, rig.draftSaver.saveDraftCalls)
		assert.Equal(t, 1, rig.reader.detailCalls)

		var resp map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Blueprint draft saved successfully", resp["message"])
		data := resp["data"].(map[string]interface{})
		status := data["status"].(map[string]interface{})
		assert.Equal(t, float64(3), status["content_version"])
		assert.Equal(t, "in_progress", status["generation_status"])
	})

	t.Run("all five provider sections accept draft saves", func(t *testing.T) {
		sections := []domain.BlueprintSectionType{
			domain.BlueprintSectionJourney,
			domain.BlueprintSectionArchitecture,
			domain.BlueprintSectionStateMachine,
			domain.BlueprintSectionWorkflow,
			domain.BlueprintSectionWBS,
		}

		for _, sec := range sections {
			t.Run(string(sec), func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				revID := uuid.New()

				rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, s domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
					assert.Equal(t, sec, s)
					return &service.BlueprintPipelineResult{
						BlueprintID:    bpID,
						RevisionID:     revID,
						Section:        sec,
						ContentVersion: ver + 1,
					}, nil
				}
				rig.reader.getDetailFunc = func(ctx context.Context, compID, bID uuid.UUID) (*dto.BlueprintDetailResponse, error) {
					return &dto.BlueprintDetailResponse{
						Status: dto.BlueprintStatusResponse{
							BlueprintID:    bpID,
							RevisionID:     revID,
							ContentVersion: 2,
						},
					}, nil
				}

				draftReq := dto.SaveBlueprintDraftSectionRequest{
					RevisionID:     revID,
					ContentVersion: 1,
					Content:        json.RawMessage(`{"` + string(sec) + `":{}}`),
				}
				body, _ := json.Marshal(draftReq)

				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/%s/draft", bpID.String(), sec), body, nil)
				require.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, 1, rig.draftSaver.saveDraftCalls)
			})
		}
	})

	t.Run("invalid blueprint ID returns 400", func(t *testing.T) {
		invalidIDs := []string{"not-a-uuid", "12345", uuid.Nil.String()}
		for _, idStr := range invalidIDs {
			t.Run(idStr, func(t *testing.T) {
				rig := setupBlueprintTestRig()
				draftReq := dto.SaveBlueprintDraftSectionRequest{
					RevisionID:     uuid.New(),
					ContentVersion: 1,
					Content:        json.RawMessage(`{"journey":{}}`),
				}
				body, _ := json.Marshal(draftReq)

				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", idStr), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
			})
		}
	})

	t.Run("invalid section parameter returns 400", func(t *testing.T) {
		invalidSections := []string{"context", "unknown", "invalid", "full_bundle", "phases"}
		for _, sec := range invalidSections {
			t.Run(sec, func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				draftReq := dto.SaveBlueprintDraftSectionRequest{
					RevisionID:     uuid.New(),
					ContentVersion: 1,
					Content:        json.RawMessage(`{"data":"val"}`),
				}
				body, _ := json.Marshal(draftReq)

				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/%s/draft", bpID.String(), sec), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
			})
		}
	})

	t.Run("strict JSON unknown top-level field returns 400", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		body := []byte(fmt.Sprintf(`{"revision_id":"%s","content_version":1,"content":{"journey":{}},"extra_field":"disallowed"}`, uuid.New().String()))

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
	})

	t.Run("multiple JSON tokens return 400", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		body := []byte(fmt.Sprintf(`{"revision_id":"%s","content_version":1,"content":{"journey":{}}}{"another":"token"}`, uuid.New().String()))

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
	})

	t.Run("oversized draft body > 1 MiB returns 400", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		largeContent := `{"big":"` + strings.Repeat("x", (1<<20)+100) + `"}`
		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(largeContent),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
	})

	t.Run("nil revision_id returns 400", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.Nil,
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{}}`),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
		assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
	})

	t.Run("content_version <= 0 returns 400", func(t *testing.T) {
		versions := []int{0, -1, -100}
		for _, ver := range versions {
			t.Run(fmt.Sprintf("version_%d", ver), func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				draftReq := dto.SaveBlueprintDraftSectionRequest{
					RevisionID:     uuid.New(),
					ContentVersion: ver,
					Content:        json.RawMessage(`{"journey":{}}`),
				}
				body, _ := json.Marshal(draftReq)

				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
			})
		}
	})

	t.Run("empty, null, or whitespace content returns 400", func(t *testing.T) {
		contents := []string{"", "   ", "null"}
		for _, c := range contents {
			t.Run("content_"+c, func(t *testing.T) {
				rig := setupBlueprintTestRig()
				bpID := uuid.New()
				var raw json.RawMessage
				if c != "" {
					raw = json.RawMessage(c)
				}
				draftReq := dto.SaveBlueprintDraftSectionRequest{
					RevisionID:     uuid.New(),
					ContentVersion: 1,
					Content:        raw,
				}
				body, _ := json.Marshal(draftReq)

				w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
				require.Equal(t, http.StatusBadRequest, w.Code)
				assert.Equal(t, "blueprint_invalid_input", getDetailCode(t, w))
				assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
			})
		}
	})

	t.Run("missing company context returns 403", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{}}`),
		}
		body, _ := json.Marshal(draftReq)

		headers := map[string]string{"X-Test-Company": "none"}
		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, headers)
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, 0, rig.draftSaver.saveDraftCalls)
	})

	t.Run("draft save conflict ErrBlueprintExecutionConflict returns 409", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, sec domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
			return nil, service.ErrBlueprintExecutionConflict
		}

		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{}}`),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusConflict, w.Code)
		assert.Equal(t, "execution_conflict", getDetailCode(t, w))
	})

	t.Run("draft save shape rejection ErrBlueprintInvalidSchema returns 422", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, sec domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
			return nil, service.ErrBlueprintInvalidSchema
		}

		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{}}`),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Equal(t, "invalid_schema", getDetailCode(t, w))
	})

	t.Run("draft save security failure ErrBlueprintSecurityFailure returns 422", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, sec domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
			return nil, service.ErrBlueprintSecurityFailure
		}

		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{"apiKey":"secret"}}`),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		assert.Equal(t, "security_failure", getDetailCode(t, w))
	})

	t.Run("draft save not found ErrBlueprintPipelineNotFound returns 404", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, sec domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
			return nil, service.ErrBlueprintPipelineNotFound
		}

		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{}}`),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, "blueprint_not_found", getDetailCode(t, w))
	})

	t.Run("draft save persistence error ErrBlueprintPersistenceFailure returns 500", func(t *testing.T) {
		rig := setupBlueprintTestRig()
		bpID := uuid.New()
		rig.draftSaver.saveDraftFunc = func(ctx context.Context, compID, bID uuid.UUID, sec domain.BlueprintSectionType, rID uuid.UUID, ver int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
			return nil, service.ErrBlueprintPersistenceFailure
		}

		draftReq := dto.SaveBlueprintDraftSectionRequest{
			RevisionID:     uuid.New(),
			ContentVersion: 1,
			Content:        json.RawMessage(`{"journey":{}}`),
		}
		body, _ := json.Marshal(draftReq)

		w := execRequest(rig.router, http.MethodPut, fmt.Sprintf("/quotation/v1/blueprints/%s/sections/journey/draft", bpID.String()), body, nil)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "persistence_failure", getDetailCode(t, w))
	})
}
