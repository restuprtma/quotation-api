package main

import (
	"encoding/json"
	"sort"
	"time"
)

type attemptStatus string

const (
	statusSuccess            attemptStatus = "success"
	statusGatewayTimeout     attemptStatus = "timeout_504"
	statusRateLimited        attemptStatus = "rate_limited_429"
	statusTimeout            attemptStatus = "timeout"
	classificationGreen                    = "Green"
	classificationYellow                   = "Yellow"
	classificationRed                      = "Red"
	classificationIncomplete               = "INCOMPLETE"
	measurementComplete                    = "complete"
	measurementIncomplete                  = "incomplete"
	decisionValidForGemini                 = "valid_for_gemini_architecture_decision"
	decisionInvalidForGemini               = "not_valid_for_gemini_architecture_decision"
	thinkingDefault                        = "default"
	thinkingOff                            = "off"
	thinkingMinimal                        = "minimal"
)

type AttemptObservation struct {
	Duration, TTFB, IdleGap   time.Duration
	Status                    attemptStatus
	SchemaValid               bool
	FinishReason              string
	InputTokens, OutputTokens int64
	ThinkingTokens            int64
	ProviderDetail            string
}

type ProviderCapabilities struct {
	StructuredOutput, Streaming bool
	ThinkingModes               []string
}
type CapabilityReport struct {
	StructuredOutput bool     `json:"structured_output"`
	Streaming        bool     `json:"streaming"`
	ThinkingModes    []string `json:"thinking_modes"`
}
type Counts struct {
	Success        int `json:"success"`
	Timeout504     int `json:"timeout_504"`
	RateLimited429 int `json:"rate_limited_429"`
	Timeout        int `json:"timeout"`
}
type SpikeSummary struct {
	Provider           string           `json:"provider"`
	Model              string           `json:"model"`
	WindowStartedAtWIB string           `json:"window_started_at_wib"`
	Mode               string           `json:"mode"`
	RequestedRuns      int              `json:"requested_runs"`
	CompletedRuns      int              `json:"completed_runs"`
	P50MS              int64            `json:"p50_ms"`
	P95MS              int64            `json:"p95_ms"`
	SchemaValid        int              `json:"schema_valid"`
	UsageMissing       int              `json:"usage_missing"`
	InputTokens        int64            `json:"input_tokens"`
	OutputTokens       int64            `json:"output_tokens"`
	ThinkingTokens     int64            `json:"thinking_tokens"`
	FinishReasons      map[string]int   `json:"finish_reasons"`
	Counts             Counts           `json:"counts"`
	Capabilities       CapabilityReport `json:"capabilities"`
	Measurement        string           `json:"measurement"`
	DecisionValidity   string           `json:"decision_validity"`
}
type StreamSummary struct {
	TTFBMS       int64 `json:"ttfb_ms"`
	TotalMS      int64 `json:"total_ms"`
	MaxIdleGapMS int64 `json:"max_idle_gap_ms"`
}
type Report struct {
	mode            string
	runs            int
	provider        string
	model           string
	windowStartedAt time.Time
	attempts        []AttemptObservation
	caps            ProviderCapabilities
}

// NewReport accepts optional provider identity and window-start metadata to keep
// existing callers source-compatible while the CLI provider route is migrated.
// A report without a Gemini identity is deliberately not decision-valid.
func NewReport(mode string, runs int, metadata ...interface{}) *Report {
	r := &Report{mode: mode, runs: runs}
	if len(metadata) == 3 {
		if provider, ok := metadata[0].(string); ok {
			r.provider = provider
		}
		if model, ok := metadata[1].(string); ok {
			r.model = model
		}
		if windowStartedAt, ok := metadata[2].(time.Time); ok {
			r.windowStartedAt = windowStartedAt
		}
	}
	return r
}
func (r *Report) Add(o AttemptObservation)               { r.attempts = append(r.attempts, o) }
func (r *Report) SetCapabilities(c ProviderCapabilities) { r.caps = c }
func (r *Report) Summary() SpikeSummary {
	s := SpikeSummary{Provider: r.provider, Model: r.model, Mode: r.mode, RequestedRuns: r.runs, CompletedRuns: len(r.attempts), FinishReasons: map[string]int{}, Capabilities: CapabilityReport{StructuredOutput: r.caps.StructuredOutput, Streaming: r.caps.Streaming, ThinkingModes: append([]string(nil), r.caps.ThinkingModes...)}, Measurement: measurementComplete, DecisionValidity: decisionValidForGemini}
	if !r.windowStartedAt.IsZero() {
		s.WindowStartedAtWIB = r.windowStartedAt.In(time.FixedZone("WIB", 7*60*60)).Format(time.RFC3339)
	}
	durations := make([]int64, 0, len(r.attempts))
	for _, a := range r.attempts {
		durations = append(durations, a.Duration.Milliseconds())
		if a.SchemaValid {
			s.SchemaValid++
		}
		if a.InputTokens == 0 && a.OutputTokens == 0 {
			s.UsageMissing++
		}
		s.InputTokens += a.InputTokens
		s.OutputTokens += a.OutputTokens
		s.ThinkingTokens += a.ThinkingTokens
		if a.FinishReason != "" {
			s.FinishReasons[a.FinishReason]++
		}
		switch a.Status {
		case statusSuccess:
			s.Counts.Success++
		case statusGatewayTimeout:
			s.Counts.Timeout504++
		case statusRateLimited:
			s.Counts.RateLimited429++
		case statusTimeout:
			s.Counts.Timeout++
		}
	}
	if len(r.attempts) < r.runs || r.provider != "gemini" {
		s.Measurement = measurementIncomplete
	}
	if r.provider != "gemini" {
		s.DecisionValidity = decisionInvalidForGemini
	}
	s.P50MS, s.P95MS = percentile(durations, 50), percentile(durations, 95)
	return s
}
func (r *Report) JSON() ([]byte, error) { return json.MarshalIndent(r.Summary(), "", "  ") }
func percentile(v []int64, p int) int64 {
	if len(v) == 0 {
		return 0
	}
	v = append([]int64(nil), v...)
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	i := (p*len(v)+99)/100 - 1
	if i < 0 {
		i = 0
	}
	if i >= len(v) {
		i = len(v) - 1
	}
	return v[i]
}

type ExperimentPlan struct {
	Stream        bool
	ThinkingModes []string
}

func PlannedExperiments(c ProviderCapabilities) ExperimentPlan {
	p := ExperimentPlan{Stream: c.Streaming, ThinkingModes: []string{thinkingDefault}}
	for _, m := range c.ThinkingModes {
		if m == thinkingOff || m == thinkingMinimal {
			p.ThinkingModes = append(p.ThinkingModes, m)
		}
	}
	return p
}
func Classify(s SpikeSummary, stream StreamSummary) string {
	if s.Measurement == measurementIncomplete || s.CompletedRuns < s.RequestedRuns || (s.Provider != "" && s.Provider != "gemini") {
		return classificationIncomplete
	}
	if s.CompletedRuns < s.RequestedRuns || s.RequestedRuns < 20 || !s.Capabilities.StructuredOutput || s.Counts.Timeout504 > 2 || s.P95MS >= 45_000 {
		return classificationRed
	}
	if s.Counts.Timeout504 == 0 && s.P95MS < 20_000 && s.SchemaValid >= 19 {
		return classificationGreen
	}
	if (s.Counts.Timeout504 == 1 || s.Counts.Timeout504 == 2 || (s.P95MS >= 20_000 && s.P95MS <= 45_000)) && s.Capabilities.Streaming && stream.TTFBMS < 15_000 && stream.MaxIdleGapMS <= 60_000 {
		return classificationYellow
	}
	return classificationRed
}
