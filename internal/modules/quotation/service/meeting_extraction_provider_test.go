package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestExtractionProviderErrorsAreAllowlisted(t *testing.T) {
	for _, tc := range []struct {
		err   error
		code  string
		retry bool
	}{
		{&ProviderError{Code: ErrRateLimited}, ErrExtractionProviderRateLimited, true}, {&ProviderError{Code: ErrTimeout}, ErrExtractionProviderTimeout, true},
		{&ProviderError{Code: ErrProviderUnavailable}, ErrExtractionProviderUnavailable, true}, {&ProviderError{Code: ErrInvalidCredential}, ErrExtractionProviderUnavailable, false},
		{context.DeadlineExceeded, ErrExtractionProviderTimeout, true}, {errors.New("network"), ErrExtractionProviderUnavailable, true},
	} {
		t.Run(tc.code, func(t *testing.T) {
			got := ExtractionErrorFromProvider(tc.err)
			assert.Equal(t, tc.code, got.Code)
			assert.Equal(t, tc.retry, got.Retryable)
		})
	}
	assert.Equal(t, ErrExtractionProviderUnavailable, (&ExtractionProviderError{Code: ErrExtractionProviderUnavailable}).Error())
}

func TestGeminiExtractionStructuredOutputAndUsage(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"STOP","content":{"parts":[{"text":"{}"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":7,"thoughtsTokenCount":2}}`))
	}))
	defer server.Close()
	temp := float32(0)
	p := NewGeminiProvider(ProviderConfig{BaseURL: server.URL, DecryptedAPIKey: "test", Model: "gemini-test"})
	result, err := p.GenerateExtraction(context.Background(), ExtractionGenerateRequest{Prompt: "CANARY_PROMPT", JSONSchema: json.RawMessage(`{"type":"object"}`), MaxOutputTokens: 123, Temperature: &temp})
	assert.NoError(t, err)
	assert.Equal(t, "STOP", result.FinishReason)
	assert.Equal(t, int64(11), result.Usage.InputTokens)
	gc := body["generationConfig"].(map[string]any)
	assert.Equal(t, "application/json", gc["responseMimeType"])
	assert.Equal(t, float64(123), gc["maxOutputTokens"])
	assert.Equal(t, float64(0), gc["temperature"])
	assert.NotContains(t, result.Text, "CANARY_PROMPT")
}

func TestGeminiExtractionRetryAfterNeverLeaksBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("CANARY_RAW_BODY"))
	}))
	defer server.Close()
	p := NewGeminiProvider(ProviderConfig{BaseURL: server.URL, DecryptedAPIKey: "test"})
	_, err := p.GenerateExtraction(context.Background(), ExtractionGenerateRequest{Prompt: "CANARY_PROMPT"})
	var e *ExtractionProviderError
	assert.ErrorAs(t, err, &e)
	assert.Equal(t, ErrExtractionProviderRateLimited, e.Code)
	assert.False(t, e.RetryAfter.IsZero())
	assert.NotContains(t, err.Error(), "CANARY_RAW_BODY")
}
func TestExtractionProviderDeadlineAndRetryAfter(t *testing.T) {
	ctx, cancel := ExtractionCallContext(context.Background(), 0)
	defer cancel()
	d, _ := ctx.Deadline()
	assert.InDelta(t, 75, time.Until(d).Seconds(), 1)
	ctx, cancel = ExtractionCallContext(context.Background(), 5*time.Second)
	defer cancel()
	d, _ = ctx.Deadline()
	assert.InDelta(t, 60, time.Until(d).Seconds(), 1)
	parent, parentCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer parentCancel()
	child, childCancel := ExtractionCallContext(parent, 90*time.Second)
	defer childCancel()
	<-child.Done()
	assert.ErrorIs(t, child.Err(), context.DeadlineExceeded)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	when, ok := ParseRetryAfter("7", now)
	assert.True(t, ok)
	assert.Equal(t, now.Add(7*time.Second), when)
	_, ok = ParseRetryAfter("bad", now)
	assert.False(t, ok)
	date := now.Add(2 * time.Minute).Format(http.TimeFormat)
	when, ok = ParseRetryAfter(date, now)
	assert.True(t, ok)
	assert.Equal(t, now.Add(2*time.Minute), when)
	assert.Equal(t, 4*time.Second, ExtractionBackoff(2, time.Second, time.Minute, func(d time.Duration) time.Duration { return d }))
	assert.True(t, ExtractionRetryAllowed(2, 8, 0, 0, false))
	assert.False(t, ExtractionRetryAllowed(2, 8, 1, 0, false))
	assert.True(t, ExtractionRetryAllowed(2, 8, 1, 0, true))
	assert.False(t, ExtractionRetryAllowed(8, 8, 0, 0, true))
}
