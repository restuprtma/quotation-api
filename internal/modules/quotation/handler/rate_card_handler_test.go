package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestRateCardHandler_CompanyContextRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	repo := repository.NewRepository(nil)
	svc := service.NewService(repo, nil, nil, nil, nil, nil, nil, nil)
	h := handler.NewHandler(svc)

	router.POST("/quotation/v1/rate-cards", h.CreateRateCard)
	router.GET("/quotation/v1/rate-cards/:id/details", h.GetRateCardDetail)
	router.DELETE("/quotation/v1/rate-cards/:id", h.DeleteRateCard)

	t.Run("CreateRateCard without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/rate-cards", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("GetRateCardDetail without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/quotation/v1/rate-cards/some-id/details", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("DeleteRateCard without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/rate-cards/some-id", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}
