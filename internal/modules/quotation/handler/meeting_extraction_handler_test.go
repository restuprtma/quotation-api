package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type meetingExtractionAPIStub struct {
	mu         sync.Mutex
	start      *dto.MeetingExtractionStartResponse
	status     *dto.MeetingExtractionStatusResponse
	startCalls int
	retryErr   error
}

func (s *meetingExtractionAPIStub) Start(context.Context, uuid.UUID, uuid.UUID, dto.StartMeetingExtractionRequest) (*dto.MeetingExtractionStartResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalls++
	return s.start, nil
}

func (s *meetingExtractionAPIStub) Status(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*dto.MeetingExtractionStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}

func (s *meetingExtractionAPIStub) Retry(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, dto.RetryMeetingExtractionRequest) (*dto.MeetingExtractionStartResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.retryErr != nil {
		return nil, s.retryErr
	}
	return s.start, nil
}

func meetingExtractionRequest(method, path, body string) *gin.Context {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Set("meeting_extraction_test_recorder", rec)
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 4 {
		c.Params = gin.Params{{Key: "id", Value: parts[3]}}
	}
	if len(parts) >= 6 && parts[4] == "meeting-extraction" {
		c.Params = append(c.Params, gin.Param{Key: "run_id", Value: parts[5]})
	}
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	setPromptTestClaims(c)
	return c
}

func meetingExtractionRecorder(c *gin.Context) *httptest.ResponseRecorder {
	return c.MustGet("meeting_extraction_test_recorder").(*httptest.ResponseRecorder)
}

func TestMeetingExtractionStartReturns202AndConcurrentPostsReuseRun(t *testing.T) {
	gin.SetMode(gin.TestMode)
	quotationID, runID := uuid.New(), uuid.New()
	stub := &meetingExtractionAPIStub{start: &dto.MeetingExtractionStartResponse{
		RunID: runID.String(), QuotationID: quotationID.String(), Status: "queued",
		StatusURL: "/quotation/v1/quotations/" + quotationID.String() + "/meeting-extraction/" + runID.String(),
	}}
	h := NewMeetingExtractionHandler(stub)

	const requests = 16
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, requests)
	var wg sync.WaitGroup
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			c := meetingExtractionRequest(http.MethodPost, "/quotation/v1/quotations/"+quotationID.String()+"/meeting-extraction", `{"service_scheme":"auto","pipeline_version":"meeting-extraction-v1"}`)
			h.Start(c)
			responses <- meetingExtractionRecorder(c)
		}()
	}
	close(start)
	wg.Wait()
	close(responses)

	for rec := range responses {
		assert.Equal(t, http.StatusAccepted, rec.Code)
		assert.Contains(t, rec.Body.String(), runID.String())
	}
	assert.Equal(t, requests, stub.startCalls)
}

func TestMeetingExtractionStatusETagAnd304(t *testing.T) {
	gin.SetMode(gin.TestMode)
	quotationID, runID := uuid.New(), uuid.New()
	phase := "staffing"
	stub := &meetingExtractionAPIStub{status: &dto.MeetingExtractionStatusResponse{
		RunID: runID.String(), QuotationID: quotationID.String(), Status: "running", CurrentPhase: &phase,
		Progress:   dto.MeetingExtractionProgress{Completed: 2, Total: 5},
		Phases:     []dto.MeetingExtractionPhaseView{{Key: "staffing", Label: "Memetakan tim", Status: "running", Attempt: 1}},
		Budget:     dto.MeetingExtractionBudgetView{CallsUsed: 3, CallsMax: 8},
		NeedsInput: []dto.MeetingExtractionNotice{}, Warnings: []dto.MeetingExtractionNotice{},
	}}
	h := NewMeetingExtractionHandler(stub)
	path := "/quotation/v1/quotations/" + quotationID.String() + "/meeting-extraction/" + runID.String()

	first := meetingExtractionRequest(http.MethodGet, path, "")
	h.Status(first)
	firstRec := meetingExtractionRecorder(first)
	require.Equal(t, http.StatusOK, firstRec.Code)
	etag := firstRec.Header().Get("ETag")
	require.NotEmpty(t, etag)

	second := meetingExtractionRequest(http.MethodGet, path, "")
	second.Request.Header.Set("If-None-Match", etag)
	require.Equal(t, etag, second.Request.Header.Get("If-None-Match"))
	h.Status(second)
	secondRec := meetingExtractionRecorder(second)
	assert.Equal(t, http.StatusNotModified, secondRec.Code)
	assert.Empty(t, secondRec.Body.String())
	assert.Equal(t, etag, secondRec.Header().Get("ETag"))
}

func TestMeetingExtractionRetryMapsChangedNotesAndBudgetExhaustion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	quotationID, runID := uuid.New(), uuid.New()
	path := "/quotation/v1/quotations/" + quotationID.String() + "/meeting-extraction/" + runID.String() + "/retry"

	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{name: "changed notes", err: ErrMeetingNotesChanged, status: http.StatusConflict, code: "MEETING_NOTES_CHANGED"},
		{name: "budget exhausted", err: repository.ErrExtractionBudgetExhausted, status: http.StatusUnprocessableEntity, code: "AI_BUDGET_EXHAUSTED"},
		{name: "active run", err: ErrMeetingExtractionConflict, status: http.StatusConflict, code: "EXTRACTION_RETRY_CONFLICT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &meetingExtractionAPIStub{retryErr: tc.err}
			h := NewMeetingExtractionHandler(stub)
			c := meetingExtractionRequest(http.MethodPost, path, `{}`)
			h.Retry(c)
			rec := meetingExtractionRecorder(c)
			assert.Equal(t, tc.status, rec.Code)
			var body dto.MeetingExtractionErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, tc.code, body.Code)
			assert.False(t, body.Retryable)
			assert.NotContains(t, rec.Body.String(), "provider")
		})
	}
}

func TestMeetingExtractionStatusSerializationDenylist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	phase := "scope"
	status := &dto.MeetingExtractionStatusResponse{
		RunID: "safe-run", QuotationID: "safe-quotation", Status: "running", CurrentPhase: &phase,
		Progress:   dto.MeetingExtractionProgress{Completed: 1, Total: 5},
		Phases:     []dto.MeetingExtractionPhaseView{{Key: "scope", Label: "Menyusun scope", Status: "running", Attempt: 1}},
		Budget:     dto.MeetingExtractionBudgetView{CallsUsed: 1, CallsMax: 8},
		NeedsInput: []dto.MeetingExtractionNotice{{Code: "FIELD_REQUIRED", Field: "scope", Message: "Input diperlukan"}},
		Warnings:   []dto.MeetingExtractionNotice{{Code: "SCHEME_REVIEW_REQUIRED", Message: "Perlu review"}},
	}
	raw, err := json.Marshal(status)
	require.NoError(t, err)
	serialized := strings.ToLower(string(raw))
	for _, forbidden := range []string{"meeting_notes", "result_json", "quote", "prompt", "raw_response", "provider", "token", "lease", "fence"} {
		assert.NotContains(t, serialized, forbidden)
	}
}

func TestMeetingExtractionStartRejectsInvalidContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewMeetingExtractionHandler(&meetingExtractionAPIStub{})
	c := meetingExtractionRequest(http.MethodPost, "/quotation/v1/quotations/"+uuid.New().String()+"/meeting-extraction", `{"service_scheme":"not-a-scheme","pipeline_version":"v2"}`)
	h.Start(c)
	assert.Equal(t, http.StatusBadRequest, meetingExtractionRecorder(c).Code)
	assert.False(t, errors.Is(errors.New("invalid"), ErrMeetingNotesChanged))
}
