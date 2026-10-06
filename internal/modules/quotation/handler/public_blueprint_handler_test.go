package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/handler"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type MockPublicBlueprintResolver struct {
	mock.Mock
}

func (m *MockPublicBlueprintResolver) GetPublicBlueprint(ctx context.Context, rawToken string) (*dto.ProgrammerBlueprintDTO, error) {
	args := m.Called(ctx, rawToken)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dto.ProgrammerBlueprintDTO), args.Error(1)
}

func TestPublicBlueprintHandler_GetPublicBlueprint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := new(MockPublicBlueprintResolver)
	h := handler.NewPublicBlueprintHandler(resolver)

	r := gin.New()
	r.GET("/p/blueprint/:token", h.GetPublicBlueprint)

	t.Run("valid token returns 200 with ProgrammerBlueprintDTO", func(t *testing.T) {
		token := "valid-token-123"
		resolver.On("GetPublicBlueprint", mock.Anything, token).Return(&dto.ProgrammerBlueprintDTO{
			Title:              "System Architecture",
			RevisionNumber:     2,
			CompletenessStatus: "complete",
			PublishedAt:        time.Now(),
			Summary:            "Architectural breakdown",
		}, nil).Once()

		req := httptest.NewRequest(http.MethodGet, "/p/blueprint/"+token, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "System Architecture")
	})

	t.Run("invalid token returns 404 generic message", func(t *testing.T) {
		token := "unknown-token"
		resolver.On("GetPublicBlueprint", mock.Anything, token).Return(nil, service.ErrPublicBlueprintNotFound).Once()

		req := httptest.NewRequest(http.MethodGet, "/p/blueprint/"+token, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), "Blueprint not found")
	})
}
