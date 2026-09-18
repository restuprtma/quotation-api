package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/service"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

type mockRevisionRepo struct {
	listRevisionsFunc func(ctx context.Context, quotationID, companyID string) ([]domain.QuotationRevision, error)
}

func (m *mockRevisionRepo) ListRevisionsByQuotationID(ctx context.Context, quotationID, companyID string) ([]domain.QuotationRevision, error) {
	if m.listRevisionsFunc != nil {
		return m.listRevisionsFunc(ctx, quotationID, companyID)
	}
	return nil, nil
}

type mockQuotationRepo struct {
	getQuotationFunc func(ctx context.Context, id, companyID string) (*domain.Quotation, error)
}

func (m *mockQuotationRepo) GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
	if m.getQuotationFunc != nil {
		return m.getQuotationFunc(ctx, id, companyID)
	}
	return &domain.Quotation{ID: uuid.MustParse(id)}, nil
}

func setupRevisionTestRouter(svc *service.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.Default()

	router.Use(func(c *gin.Context) {
		c.Set("user", &jwtpkg.Claims{
			CompanyID: "00000000-0000-0000-0000-000000000001",
		})
		c.Next()
	})

	hdlr := handler.NewHandler(svc)
	router.GET("/quotations/:id/revisions", hdlr.ListRevisions)
	router.GET("/quotations/:id/revisions/:revision_id", hdlr.GetRevisionDetail)

	return router
}

func TestHandler_ListRevisions_And_GetRevisionDetail_MetadataContract(t *testing.T) {
	quotationID := uuid.New()
	revisionID := uuid.New()
	htmlContent := "<p>Signed & approved content</p>"

	mockQ := &mockQuotationRepo{
		getQuotationFunc: func(ctx context.Context, id, compID string) (*domain.Quotation, error) {
			return &domain.Quotation{ID: quotationID}, nil
		},
	}

	mockR := &mockRevisionRepo{
		listRevisionsFunc: func(ctx context.Context, qID, compID string) ([]domain.QuotationRevision, error) {
			return []domain.QuotationRevision{
				{
					ID:                revisionID,
					RevisionNumber:    1,
					PublicationStatus: "draft",
					GenerationStatus:  "completed",
					ContentHTML:       &htmlContent,
				},
			}, nil
		},
	}

	svc := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
		WithQuotationRepository(mockQ).
		WithRevisionRepository(mockR)

	router := setupRevisionTestRouter(svc)

	// 1. Test ListRevisions -> metadata MUST be included in JSON
	listReq, _ := http.NewRequest(http.MethodGet, "/quotations/"+quotationID.String()+"/revisions", nil)
	listW := httptest.NewRecorder()
	router.ServeHTTP(listW, listReq)

	assert.Equal(t, http.StatusOK, listW.Code)
	listBody := listW.Body.String()
	assert.Contains(t, listBody, `"blueprint_eligible":true`)
	assert.Contains(t, listBody, `"blueprint_ineligible_reason":null`)
	assert.NotContains(t, listBody, "content_html")

	// 2. Test GetRevisionDetail -> metadata MUST be stripped from JSON to maintain contract
	detailReq, _ := http.NewRequest(http.MethodGet, "/quotations/"+quotationID.String()+"/revisions/"+revisionID.String(), nil)
	detailW := httptest.NewRecorder()
	router.ServeHTTP(detailW, detailReq)

	assert.Equal(t, http.StatusOK, detailW.Code)
	detailBody := detailW.Body.String()
	assert.NotContains(t, detailBody, "blueprint_eligible")
	assert.NotContains(t, detailBody, "blueprint_ineligible_reason")
	assert.NotContains(t, detailBody, "content_html")
}
