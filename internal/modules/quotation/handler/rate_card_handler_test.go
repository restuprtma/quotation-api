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
	jwtpkg "venturo-skeleton-go/pkg/jwt"
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

func TestRateCardItemHandlersRouteRegistration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	svc := &service.Service{}
	h := handler.NewHandler(svc)

	group := router.Group("/quotation/v1/rate-cards")
	{
		group.POST("/:id/set-default", h.SetDefaultRateCard)
		group.POST("/:id/items", h.CreateRateCardItem)
		group.PUT("/:id/items/:item_id", h.UpdateRateCardItem)
		group.DELETE("/:id/items/:item_id", h.DeleteRateCardItem)
	}

	assert.NotNil(t, router)

	t.Run("POST /quotation/v1/rate-cards/:id/set-default without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/rate-cards/some-id/set-default", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("POST /quotation/v1/rate-cards/:id/items without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/rate-cards/some-id/items", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("PUT /quotation/v1/rate-cards/:id/items/:item_id without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/quotation/v1/rate-cards/some-id/items/item-id", bytes.NewBufferString("{}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("DELETE /quotation/v1/rate-cards/:id/items/:item_id without company context returns 403", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/quotation/v1/rate-cards/some-id/items/item-id", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestRateCardItemHandlers_PayloadValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("user", &jwtpkg.Claims{
			CompanyID: "00000000-0000-0000-0000-000000000001",
		})
		c.Next()
	})

	svc := &service.Service{}
	h := handler.NewHandler(svc)

	group := router.Group("/quotation/v1/rate-cards")
	{
		group.POST("/:id/items", h.CreateRateCardItem)
		group.PUT("/:id/items/:item_id", h.UpdateRateCardItem)
	}

	t.Run("POST /quotation/v1/rate-cards/:id/items with missing required fields returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/quotation/v1/rate-cards/some-id/items", bytes.NewBufferString(`{"role": ""}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("PUT /quotation/v1/rate-cards/:id/items/:item_id with invalid json returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/quotation/v1/rate-cards/some-id/items/item-id", bytes.NewBufferString("not-a-valid-json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

