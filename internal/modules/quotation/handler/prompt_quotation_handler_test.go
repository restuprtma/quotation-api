package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/shared/response"
	jwtpkg "venturo-skeleton-go/pkg/jwt"
)

type promptCreatorStub struct {
	called  bool
	result  *dto.MeetingDraftResponse
	errCode string
	err     error
}

func (s *promptCreatorStub) CreateFromMeeting(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _ dto.CreateFromMeetingRequest) (*dto.MeetingDraftResponse, error) {
	s.called = true
	if s.err != nil {
		return nil, s.err
	}
	if s.errCode != "" {
		return nil, promptHandlerError{code: s.errCode}
	}
	if s.result != nil {
		return s.result, nil
	}
	return &dto.MeetingDraftResponse{Readiness: "incomplete"}, nil
}

type promptHandlerError struct {
	code   string
	secret string
}

func (e promptHandlerError) Error() string {
	if e.secret != "" {
		return e.secret
	}
	return e.code
}

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

	t.Run("safe error code preserved and does not leak raw output", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h := NewPromptQuotationHandler(&promptCreatorStub{
			err: promptHandlerError{
				code:   "invalid_extraction.unsupported_evidence",
				secret: "raw-model-output-secret",
			},
		})
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		// companyUUID reads the normal middleware claims; use a valid claim object.
		setPromptTestClaims(c)
		h.CreateFromMeeting(c)

		assert.Equal(t, http.StatusUnprocessableEntity, c.Writer.Status())
		assert.Contains(t, rec.Body.String(), "invalid_extraction.unsupported_evidence")
		assert.NotContains(t, rec.Body.String(), "raw-model-output-secret")

		var resp response.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, []string{"invalid_extraction.unsupported_evidence"}, resp.Errors["detail"])
	})

	t.Run("generic invalid_extraction code preserved", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h := NewPromptQuotationHandler(&promptCreatorStub{errCode: "invalid_extraction"})
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		setPromptTestClaims(c)
		h.CreateFromMeeting(c)

		assert.Equal(t, http.StatusUnprocessableEntity, c.Writer.Status())
		assert.Contains(t, rec.Body.String(), "invalid_extraction")

		var resp response.Response
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, []string{"invalid_extraction"}, resp.Errors["detail"])
	})
}

func TestPromptQuotationHandler_NoBusinessSignalReturns422(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	h := NewPromptQuotationHandler(&promptCreatorStub{errCode: "meeting_notes_no_business_signal"})
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"meeting_notes":"notes"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	setPromptTestClaims(c)

	h.CreateFromMeeting(c)

	assert.Equal(t, http.StatusUnprocessableEntity, c.Writer.Status())
	var resp response.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Nil(t, resp.Data)
	assert.Equal(t, "Unable to create quotation draft", resp.Message)
	assert.Equal(t, []string{"meeting_notes_no_business_signal"}, resp.Errors["detail"])
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
