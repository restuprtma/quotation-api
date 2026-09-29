package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

type promptRevisionPatcherStub struct{ err error }

func (s promptRevisionPatcherStub) ApplyCommercialPatch(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, dto.PromptPatchRequest) (*dto.PromptPatchResponse, error) {
	return nil, s.err
}

type promptSectionReviserStub struct{ err error }

func (s promptSectionReviserStub) Revise(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, dto.ReviseSectionRequest) (*dto.PromptPatchResponse, error) {
	return nil, s.err
}

func TestPromptRevisionHandler_MapsConflictWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPromptRevisionHandler(promptRevisionPatcherStub{err: promptHandlerError{code: "revision_conflict"}})
	r := gin.New()
	r.PATCH("/quotations/:id/prompt-state", func(c *gin.Context) { setPromptTestClaims(c); h.PatchPromptState(c) })
	req := httptest.NewRequest(http.MethodPatch, "/quotations/"+uuid.New().String()+"/prompt-state", strings.NewReader(`{"revision_id":"`+uuid.New().String()+`","content_version":1,"state_version":1,"operations":[{"type":"set_payment_pattern","payment_pattern":"50:50"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
	require.NotContains(t, rec.Body.String(), "database")
}

func TestPromptRevisionHandler_MapsProviderFailureWithoutLeakingDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPromptRevisionHandler(promptRevisionPatcherStub{}, promptSectionReviserStub{err: promptHandlerError{code: "revision_provider_failed"}})
	r := gin.New()
	r.POST("/quotations/:id/revise-section", func(c *gin.Context) { setPromptTestClaims(c); h.ReviseSection(c) })
	req := httptest.NewRequest(http.MethodPost, "/quotations/"+uuid.New().String()+"/revise-section", strings.NewReader(`{"revision_id":"`+uuid.New().String()+`","content_version":1,"state_version":1,"instruction":"Ringkas scope","target_section":"scope"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.NotContains(t, rec.Body.String(), "database")
}
