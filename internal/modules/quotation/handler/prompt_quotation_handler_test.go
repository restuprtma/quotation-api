package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

type promptCreatorStub struct {
	called  bool
	result  *dto.MeetingDraftResponse
	errCode string
}

func (s *promptCreatorStub) CreateFromMeeting(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ dto.CreateFromMeetingRequest) (*dto.MeetingDraftResponse, error) {
	s.called = true
	if s.errCode != "" {
		return nil, promptHandlerError{s.errCode}
	}
	if s.result != nil {
		return s.result, nil
	}
	return &dto.MeetingDraftResponse{Readiness: "incomplete"}, nil
}

type promptHandlerError struct{ code string }

func (e promptHandlerError) Error() string    { return e.code }
func (e promptHandlerError) SafeCode() string { return e.code }

func TestPromptQuotationHandler_MissingMeetingNotesReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &promptCreatorStub{}
	h := NewPromptQuotationHandler(stub)
	r := gin.New()
	r.POST("/quotation/v1/quotations/from-meeting", func(c *gin.Context) {
		h.CreateFromMeeting(c)
	})

	req := httptest.NewRequest(http.MethodPost, "/quotation/v1/quotations/from-meeting", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, stub.called)
}

func TestPromptQuotationHandler_RequiresTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPromptQuotationHandler(&promptCreatorStub{})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.CreateFromMeeting(c)

	assert.Equal(t, http.StatusForbidden, c.Writer.Status())
}

func TestPromptQuotationHandler_InvalidExtractionReturns422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPromptQuotationHandler(&promptCreatorStub{errCode: "invalid_extraction"})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	// companyUUID reads the normal middleware claims; use a valid claim object.
	setPromptTestClaims(c)
	h.CreateFromMeeting(c)

	assert.Equal(t, http.StatusUnprocessableEntity, c.Writer.Status())
}

func TestPromptQuotationHandler_PriceableDraftReturns201(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPromptQuotationHandler(&promptCreatorStub{result: &dto.MeetingDraftResponse{Readiness: "priceable"}})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	setPromptTestClaims(c)
	h.CreateFromMeeting(c)

	assert.Equal(t, http.StatusCreated, c.Writer.Status())
}

func setPromptTestClaims(c *gin.Context) {
	// The handler only needs the claims' company and user IDs; use the same
	// helper shape as the production middleware rather than bypassing it.
	c.Set("user", &jwtpkg.Claims{CompanyID: uuid.New().String(), UserID: uuid.New().String()})
}

func TestPromptQuotationHandler_UnknownFieldsAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &promptCreatorStub{result: &dto.MeetingDraftResponse{Readiness: "incomplete"}}
	h := NewPromptQuotationHandler(stub)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes","unknown_field":"some_val","project_name":"Project Alpha"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	setPromptTestClaims(c)
	h.CreateFromMeeting(c)

	assert.Equal(t, http.StatusCreated, c.Writer.Status())
	assert.True(t, stub.called)
}

func TestPromptQuotationHandler_ProjectNameTooLongReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewPromptQuotationHandler(&promptCreatorStub{errCode: "invalid_request"})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	longName := strings.Repeat("a", 201)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes","project_name":"`+longName+`"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	setPromptTestClaims(c)
	h.CreateFromMeeting(c)

	assert.Equal(t, http.StatusBadRequest, c.Writer.Status())
}
