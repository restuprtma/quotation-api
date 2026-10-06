package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type mockPublicQuotationService struct {
	getPublicQuotationFunc  func(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error)
	signPublicQuotationFunc func(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error)
}

func (m *mockPublicQuotationService) GetPublicQuotation(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error) {
	if m.getPublicQuotationFunc != nil {
		return m.getPublicQuotationFunc(ctx, publicToken)
	}
	return nil, service.ErrPublicQuotationNotFound
}

func (m *mockPublicQuotationService) SignPublicQuotation(ctx context.Context, publicToken string, req dto.SignPublicQuotationRequest, ip, userAgent string) (*dto.PublicQuotationResponse, error) {
	if m.signPublicQuotationFunc != nil {
		return m.signPublicQuotationFunc(ctx, publicToken, req, ip, userAgent)
	}
	return nil, nil
}

func setupPublicTestRouter(svc handler.PublicQuotationService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewPublicQuotationHandler(svc)
	p := r.Group("/p/q")
	{
		p.GET("/:public_token", h.GetPublicQuotation)
		p.POST("/:public_token/sign", h.SignPublicQuotation)
	}
	return r
}

func TestPublicQuotationHandler_GetPublicQuotation(t *testing.T) {
	token := uuid.New().String()

	t.Run("invalid UUID token returns 404 with no detail leak", func(t *testing.T) {
		r := setupPublicTestRouter(&mockPublicQuotationService{})
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/p/q/invalid-not-uuid", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.NotContains(t, w.Body.String(), "uuid")
	})

	t.Run("non existent or draft-only token returns 404", func(t *testing.T) {
		mockSvc := &mockPublicQuotationService{
			getPublicQuotationFunc: func(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error) {
				return nil, service.ErrPublicQuotationNotFound
			},
		}
		r := setupPublicTestRouter(mockSvc)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/p/q/"+token, nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("valid published token returns 200 with strict DTO", func(t *testing.T) {
		now := time.Now().UTC()
		subj := "Enterprise Portal"
		mockSvc := &mockPublicQuotationService{
			getPublicQuotationFunc: func(ctx context.Context, publicToken string) (*dto.PublicQuotationResponse, error) {
				return &dto.PublicQuotationResponse{
					QuotationNumber: "Q-2026-001",
					Subject:         &subj,
					Client:          dto.PublicQuotationClient{CompanyName: "Client PT"},
					RevisionNumber:  1,
					ContentHTML:     "<p>Quotation Content</p>",
					PublishedAt:     now,
					Signed:          false,
				}, nil
			},
		}
		r := setupPublicTestRouter(mockSvc)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/p/q/"+token, nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp struct {
			Data dto.PublicQuotationResponse `json:"data"`
		}
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, "Q-2026-001", resp.Data.QuotationNumber)
		assert.False(t, resp.Data.Signed)
	})
}
