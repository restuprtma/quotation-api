package service

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Extraction error codes are intentionally the public allowlist from the
// pipeline contract.  No upstream wording or response content is retained.
const (
	ErrExtractionProviderTimeout     = "PROVIDER_TIMEOUT"
	ErrExtractionProviderRateLimited = "PROVIDER_RATE_LIMITED"
	ErrExtractionProviderUnavailable = "PROVIDER_UNAVAILABLE"
	ErrExtractionInvalidJSON         = "INVALID_JSON"
	ErrExtractionSchemaInvalid       = "SCHEMA_VALIDATION_FAILED"
	ErrExtractionEvidenceMismatch    = "EVIDENCE_MISMATCH"
	ErrExtractionBudgetExhausted     = "AI_BUDGET_EXHAUSTED"
	ErrExtractionNotesChanged        = "MEETING_NOTES_CHANGED"
	ErrExtractionInternal            = "INTERNAL_ERROR"
)

const DefaultExtractionCallTimeout = 75 * time.Second

type ThinkingMode string

const (
	ThinkingDefault ThinkingMode = "default"
	ThinkingOff     ThinkingMode = "off"
	ThinkingMinimal ThinkingMode = "minimal"
)

type ThinkingConfig struct {
	Mode   ThinkingMode
	Budget int
}
type ProviderCapabilities struct {
	Streaming               bool
	JSONSchema              bool
	UsageMetadata           bool
	FinishReason            bool
	ThinkingModes           []ThinkingMode
	SupportsTemperatureZero bool
}
type TokenUsage struct{ InputTokens, OutputTokens, ThinkingTokens int64 }
type ExtractionGenerateRequest struct {
	Prompt          string
	JSONSchema      json.RawMessage
	MaxOutputTokens int
	Temperature     *float32
	Thinking        ThinkingConfig
	Stream          bool
}
type ExtractionGenerateResult struct {
	Text         string
	FinishReason string
	Usage        TokenUsage
	TTFB         time.Duration
	IdleGap      time.Duration
}
type ExtractionProvider interface {
	ExtractionCapabilities() ProviderCapabilities
	GenerateExtraction(context.Context, ExtractionGenerateRequest) (ExtractionGenerateResult, error)
}

// ExtractionProviderError exposes only a stable code. Message is static and
// must never be constructed from provider response data.
type ExtractionProviderError struct {
	Code       string
	Retryable  bool
	RetryAfter time.Time
}

func (e *ExtractionProviderError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code
}

func ExtractionErrorFromProvider(err error) *ExtractionProviderError {
	var p *ProviderError
	if errors.As(err, &p) {
		switch p.Code {
		case ErrRateLimited:
			return &ExtractionProviderError{Code: ErrExtractionProviderRateLimited, Retryable: true}
		case ErrTimeout:
			return &ExtractionProviderError{Code: ErrExtractionProviderTimeout, Retryable: true}
		case ErrProviderUnavailable:
			return &ExtractionProviderError{Code: ErrExtractionProviderUnavailable, Retryable: true}
		default:
			return &ExtractionProviderError{Code: ErrExtractionProviderUnavailable}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &ExtractionProviderError{Code: ErrExtractionProviderTimeout, Retryable: true}
	}
	return &ExtractionProviderError{Code: ErrExtractionProviderUnavailable, Retryable: true}
}

func ExtractionCallContext(parent context.Context, configured time.Duration) (context.Context, context.CancelFunc) {
	if configured == 0 {
		configured = DefaultExtractionCallTimeout
	}
	if configured < 60*time.Second {
		configured = 60 * time.Second
	}
	if configured > 90*time.Second {
		configured = 90 * time.Second
	}
	return context.WithTimeout(parent, configured)
}

func ParseRetryAfter(value string, now time.Time) (time.Time, bool) {
	v := strings.TrimSpace(value)
	if v == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.Atoi(v); err == nil && seconds >= 0 {
		return now.Add(time.Duration(seconds) * time.Second), true
	}
	if when, err := http.ParseTime(v); err == nil && when.After(now) {
		return when, true
	}
	return time.Time{}, false
}
func ExtractionBackoff(attempt int, base, maximum time.Duration, jitter func(time.Duration) time.Duration) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if base <= 0 {
		base = time.Second
	}
	if maximum < base {
		maximum = base
	}
	d := base
	for i := 0; i < attempt && d < maximum; i++ {
		d *= 2
		if d > maximum {
			d = maximum
		}
	}
	if jitter != nil {
		d = jitter(d)
		if d < 0 {
			return 0
		}
		if d > maximum {
			return maximum
		}
	}
	return d
}
func DefaultExtractionJitter(d time.Duration) time.Duration {
	return d/2 + time.Duration(rand.Int64N(int64(d)+1))
}

// ExtractionRetryAllowed caps each retry kind at one and never permits a
// retry to bypass the persisted run budget.
func ExtractionRetryAllowed(callsUsed, callsMax, transientRetries, repairRetries int, repair bool) bool {
	if callsUsed >= callsMax || callsUsed >= 10 {
		return false
	}
	if repair {
		return repairRetries < 1
	}
	return transientRetries < 1
}
