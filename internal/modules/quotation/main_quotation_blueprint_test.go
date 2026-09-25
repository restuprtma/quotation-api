package quotation_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

type mockCreator struct {
	createCalled bool
}

func (m *mockCreator) CreateBlueprint(ctx context.Context, companyID, createdBy uuid.UUID, req dto.CreateBlueprintGenerationRequest) (*repository.CreateBlueprintGenerationResult, error) {
	m.createCalled = true
	bpID := uuid.New()
	return &repository.CreateBlueprintGenerationResult{
		Blueprint: domain.Blueprint{
			ID:        bpID,
			CompanyID: companyID,
		},
		Revision: domain.BlueprintRevision{
			ID:          uuid.New(),
			BlueprintID: bpID,
		},
		IdempotentReplay: false,
	}, nil
}

type mockReader struct {
	statusCalled bool
	detailCalled bool
	listCalled   bool
}

func (m *mockReader) GetStatus(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
	m.statusCalled = true
	return &dto.BlueprintStatusResponse{
		BlueprintID:        blueprintID,
		RevisionID:         uuid.New(),
		Title:              "Mock Title",
		GenerationStatus:   "completed",
		PublicationStatus:  "draft",
		CompletenessStatus: "complete",
		ContentVersion:     1,
		Sections:           []dto.BlueprintSectionStatusResponse{},
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}, nil
}

func (m *mockReader) GetDetail(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintDetailResponse, error) {
	m.detailCalled = true
	return &dto.BlueprintDetailResponse{
		Status: dto.BlueprintStatusResponse{
			BlueprintID: blueprintID,
			RevisionID:  uuid.New(),
			Title:       "Mock Title",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		},
		QuotationID: uuid.New(),
	}, nil
}

func (m *mockReader) List(ctx context.Context, companyID uuid.UUID, params repository.BlueprintListParams) (*dto.BlueprintListResponse, error) {
	m.listCalled = true
	return &dto.BlueprintListResponse{
		Items: []dto.BlueprintListItemResponse{
			{
				BlueprintID: uuid.New(),
				QuotationID: uuid.New(),
				Title:       "Mock List Item",
				UpdatedAt:   time.Now(),
			},
		},
		Total:      1,
		Page:       1,
		Limit:      20,
		TotalPages: 1,
	}, nil
}

type mockPipeline struct {
	continueCalled bool
	retryCalled    bool
	manualCalled   bool
}

func (m *mockPipeline) Continue(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error) {
	m.continueCalled = true
	return &service.BlueprintPipelineResult{
		BlueprintID: blueprintID,
	}, nil
}

func (m *mockPipeline) RetrySection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*service.BlueprintPipelineResult, error) {
	m.retryCalled = true
	return &service.BlueprintPipelineResult{
		BlueprintID: blueprintID,
	}, nil
}

func (m *mockPipeline) ManualCompleteSectionAtVersion(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
	m.manualCalled = true
	return &service.BlueprintPipelineResult{
		BlueprintID: blueprintID,
	}, nil
}

type mockDraftSaver struct {
	saveDraftCalled bool
}

func (m *mockDraftSaver) SaveDraftSectionAtVersion(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error) {
	m.saveDraftCalled = true
	return &service.BlueprintPipelineResult{
		BlueprintID: blueprintID,
	}, nil
}

func setupTestQuotationModule() (*gin.Engine, *mockCreator, *mockReader, *mockPipeline) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	creator := &mockCreator{}
	reader := &mockReader{}
	pipeline := &mockPipeline{}
	draftSaver := &mockDraftSaver{}

	bpHdlr := handler.NewBlueprintGenerationHandler(creator, reader, pipeline, draftSaver)

	mod := &quotation.Module{
		BlueprintHandler: bpHdlr,
	}

	v1 := router.Group("/quotation/v1")
	mod.SetupRoutes(v1)

	return router, creator, reader, pipeline
}

func generateTestJWT(userID, companyID string) string {
	token, err := jwtpkg.GenerateToken(
		userID, companyID, "Acme Corp", "client-1", "acme",
		"user@acme.com", "testuser", "Test User",
		false, []string{"admin"},
	)
	if err != nil {
		panic(err)
	}
	return token
}

func TestMainQuotation_BlueprintRoutes_Registered(t *testing.T) {
	router, _, _, _ := setupTestQuotationModule()

	expectedRoutes := map[string]string{
		"POST /quotation/v1/blueprints/generate":                    "",
		"GET /quotation/v1/blueprints/:id/generation-status":        "",
		"GET /quotation/v1/blueprints/:id":                          "",
		"POST /quotation/v1/blueprints/:id/continue":                "",
		"POST /quotation/v1/blueprints/:id/sections/:section/retry": "",
		"PUT /quotation/v1/blueprints/:id/sections/:section/manual": "",
		"PUT /quotation/v1/blueprints/:id/sections/:section/draft":  "",
		"GET /quotation/v1/blueprints":                              "",
	}

	registeredRoutes := make(map[string]bool)
	for _, route := range router.Routes() {
		key := fmt.Sprintf("%s %s", route.Method, route.Path)
		registeredRoutes[key] = true
	}

	for expected := range expectedRoutes {
		assert.True(t, registeredRoutes[expected], "expected route %s to be registered", expected)
	}
}

func TestMainQuotation_BlueprintRoutes_Unauthorized_Returns401(t *testing.T) {
	router, _, _, _ := setupTestQuotationModule()
	bpID := uuid.New()

	endpoints := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/quotation/v1/blueprints/generate", []byte(`{}`)},
		{http.MethodGet, "/quotation/v1/blueprints/" + bpID.String() + "/generation-status", nil},
		{http.MethodGet, "/quotation/v1/blueprints/" + bpID.String(), nil},
		{http.MethodPost, "/quotation/v1/blueprints/" + bpID.String() + "/continue", nil},
		{http.MethodPost, "/quotation/v1/blueprints/" + bpID.String() + "/sections/journey/retry", nil},
		{http.MethodPut, "/quotation/v1/blueprints/" + bpID.String() + "/sections/journey/manual", []byte(`{}`)},
		{http.MethodPut, "/quotation/v1/blueprints/" + bpID.String() + "/sections/journey/draft", []byte(`{}`)},
		{http.MethodGet, "/quotation/v1/blueprints", nil},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("%s %s", ep.method, ep.path), func(t *testing.T) {
			var bodyReader *bytes.Reader
			if ep.body != nil {
				bodyReader = bytes.NewReader(ep.body)
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(ep.method, ep.path, bodyReader)
			if ep.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code, "expected 401 Unauthorized for unauthenticated request")
		})
	}
}

func TestMainQuotation_BlueprintRoutes_MissingCompanyContext_Returns403(t *testing.T) {
	router, _, _, _ := setupTestQuotationModule()
	bpID := uuid.New()

	// Token with empty company ID
	noCompanyToken := generateTestJWT(uuid.New().String(), "")

	endpoints := []struct {
		method string
		path   string
		body   []byte
	}{
		{http.MethodPost, "/quotation/v1/blueprints/generate", []byte(`{}`)},
		{http.MethodGet, "/quotation/v1/blueprints/" + bpID.String() + "/generation-status", nil},
		{http.MethodGet, "/quotation/v1/blueprints/" + bpID.String(), nil},
		{http.MethodPost, "/quotation/v1/blueprints/" + bpID.String() + "/continue", nil},
		{http.MethodPost, "/quotation/v1/blueprints/" + bpID.String() + "/sections/journey/retry", nil},
		{http.MethodPut, "/quotation/v1/blueprints/" + bpID.String() + "/sections/journey/manual", []byte(`{}`)},
		{http.MethodGet, "/quotation/v1/blueprints", nil},
	}

	for _, ep := range endpoints {
		t.Run(fmt.Sprintf("%s %s", ep.method, ep.path), func(t *testing.T) {
			var bodyReader *bytes.Reader
			if ep.body != nil {
				bodyReader = bytes.NewReader(ep.body)
			} else {
				bodyReader = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(ep.method, ep.path, bodyReader)
			req.Header.Set("Authorization", "Bearer "+noCompanyToken)
			if ep.body != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusForbidden, w.Code, "expected 403 Forbidden for missing company context")
		})
	}
}

func TestMainQuotation_BlueprintRoutes_NoCollision(t *testing.T) {
	router, _, reader, _ := setupTestQuotationModule()
	validToken := generateTestJWT(uuid.New().String(), uuid.New().String())
	bpID := uuid.New()

	// 1. GET /quotation/v1/blueprints -> hits List
	reqList := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints", nil)
	reqList.Header.Set("Authorization", "Bearer "+validToken)
	wList := httptest.NewRecorder()
	router.ServeHTTP(wList, reqList)
	require.Equal(t, http.StatusOK, wList.Code)
	assert.True(t, reader.listCalled)

	// 2. GET /quotation/v1/blueprints/:id -> hits GetDetail
	reqDetail := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints/"+bpID.String(), nil)
	reqDetail.Header.Set("Authorization", "Bearer "+validToken)
	wDetail := httptest.NewRecorder()
	router.ServeHTTP(wDetail, reqDetail)
	require.Equal(t, http.StatusOK, wDetail.Code)
	assert.True(t, reader.detailCalled)

	// 3. GET /quotation/v1/blueprints/:id/generation-status -> hits GetStatus
	reqStatus := httptest.NewRequest(http.MethodGet, "/quotation/v1/blueprints/"+bpID.String()+"/generation-status", nil)
	reqStatus.Header.Set("Authorization", "Bearer "+validToken)
	wStatus := httptest.NewRecorder()
	router.ServeHTTP(wStatus, reqStatus)
	require.Equal(t, http.StatusOK, wStatus.Code)
	assert.True(t, reader.statusCalled)
}
