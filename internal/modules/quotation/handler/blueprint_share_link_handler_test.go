package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type MockShareLinkDataRepoForHandler struct {
	mock.Mock
}

func (m *MockShareLinkDataRepoForHandler) Create(ctx context.Context, link *domain.ShareLink) error {
	return m.Called(ctx, link).Error(0)
}
func (m *MockShareLinkDataRepoForHandler) Revoke(ctx context.Context, companyID, blueprintID, linkID uuid.UUID) error {
	return m.Called(ctx, companyID, blueprintID, linkID).Error(0)
}
func (m *MockShareLinkDataRepoForHandler) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*domain.ShareLink, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.ShareLink), args.Error(1)
}
func (m *MockShareLinkDataRepoForHandler) TouchLastAccessed(ctx context.Context, linkID uuid.UUID) error {
	return m.Called(ctx, linkID).Error(0)
}
func (m *MockShareLinkDataRepoForHandler) ListByBlueprint(ctx context.Context, companyID, blueprintID uuid.UUID) ([]repository.ShareLinkWithRevision, error) {
	args := m.Called(ctx, companyID, blueprintID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]repository.ShareLinkWithRevision), args.Error(1)
}

type MockBlueprintSharingLookupForHandler struct {
	mock.Mock
}

func (m *MockBlueprintSharingLookupForHandler) GetRevisionForSharing(ctx context.Context, companyID, blueprintID, revisionID uuid.UUID) (*domain.BlueprintRevision, error) {
	args := m.Called(ctx, companyID, blueprintID, revisionID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.BlueprintRevision), args.Error(1)
}

func TestBlueprintShareLinkHandler_Create_DraftReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := new(MockShareLinkDataRepoForHandler)
	lookup := new(MockBlueprintSharingLookupForHandler)
	svc := service.NewBlueprintShareLinkService(repo, lookup)
	h := handler.NewBlueprintShareLinkHandler(svc, "https://app.tuai.id")

	companyID := uuid.New()
	blueprintID := uuid.New()
	draftRevID := uuid.New()

	lookup.On("GetRevisionForSharing", mock.Anything, companyID, blueprintID, draftRevID).Return(&domain.BlueprintRevision{
		ID:                draftRevID,
		CompanyID:         companyID,
		BlueprintID:       blueprintID,
		RevisionNumber:    1,
		PublicationStatus: "draft",
		PublishedAt:       nil,
	}, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", companyID.String())
		c.Next()
	})
	r.POST("/blueprints/:id/share-links", h.Create)

	body, _ := json.Marshal(dto.CreateBlueprintShareLinkRequest{
		BlueprintRevisionID: draftRevID,
		Label:               "Test Draft",
	})
	req := httptest.NewRequest(http.MethodPost, "/blueprints/"+blueprintID.String()+"/share-links", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestBlueprintShareLinkHandler_ListAndRevoke(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := new(MockShareLinkDataRepoForHandler)
	lookup := new(MockBlueprintSharingLookupForHandler)
	svc := service.NewBlueprintShareLinkService(repo, lookup)
	h := handler.NewBlueprintShareLinkHandler(svc, "https://app.tuai.id")

	companyID := uuid.New()
	blueprintID := uuid.New()
	linkID := uuid.New()

	repo.On("ListByBlueprint", mock.Anything, companyID, blueprintID).Return([]repository.ShareLinkWithRevision{
		{
			ShareLink: domain.ShareLink{
				ID:          linkID,
				CompanyID:   companyID,
				BlueprintID: blueprintID,
				Label:       "Programmer Link",
				CreatedAt:   time.Now(),
			},
			RevisionNumber: 2,
		},
	}, nil)
	repo.On("Revoke", mock.Anything, companyID, blueprintID, linkID).Return(nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("company_id", companyID.String())
		c.Next()
	})
	r.GET("/blueprints/:id/share-links", h.List)
	r.DELETE("/blueprints/:id/share-links/:link_id", h.Revoke)

	// Test List
	reqList := httptest.NewRequest(http.MethodGet, "/blueprints/"+blueprintID.String()+"/share-links", nil)
	wList := httptest.NewRecorder()
	r.ServeHTTP(wList, reqList)
	assert.Equal(t, http.StatusOK, wList.Code)

	// Test Revoke
	reqRevoke := httptest.NewRequest(http.MethodDelete, "/blueprints/"+blueprintID.String()+"/share-links/"+linkID.String(), nil)
	wRevoke := httptest.NewRecorder()
	r.ServeHTTP(wRevoke, reqRevoke)
	assert.Equal(t, http.StatusOK, wRevoke.Code)
}
