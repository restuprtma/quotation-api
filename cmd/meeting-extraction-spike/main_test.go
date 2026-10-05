package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestSpikeReportRedactsCanaries(t *testing.T) {
	canaries := []string{
		"NOTE_CANARY_7d4bf", "PROMPT_CANARY_8a19e", "API_KEY_CANARY_1f2c",
		"Authorization: Bearer AUTH_CANARY_12ab", "RAW_PROVIDER_BODY_CANARY_91de",
	}
	report := NewReport("small-metadata", 20)
	report.Add(AttemptObservation{Duration: 100 * time.Millisecond, Status: statusSuccess, SchemaValid: true, FinishReason: "stop", InputTokens: 12, OutputTokens: 8})
	report.Add(AttemptObservation{Duration: 200 * time.Millisecond, Status: statusTimeout, ProviderDetail: strings.Join(canaries, " ")})
	encoded, err := report.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range canaries {
		if strings.Contains(string(encoded), canary) {
			t.Fatalf("report leaked canary %q", canary)
		}
	}
}

func TestEncodedReportNeverContainsSensitiveCanaries(t *testing.T) {
	encoded, err := encodeReport(SpikeSummary{
		Provider: "gemini",
		Model:    "gemini-3.6-flash",
	}, StreamSummary{})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"NOTE_CANARY",
		"PROMPT_CANARY",
		"API_KEY_CANARY",
		"Authorization",
		"RAW_BODY_CANARY",
	} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("encoded report leaked canary %q", forbidden)
		}
	}
}

func TestSpikeReportAggregatesPercentilesAndMissingUsage(t *testing.T) {
	report := NewReport("small-metadata", 3)
	report.Add(AttemptObservation{Duration: 10 * time.Millisecond, Status: statusSuccess, SchemaValid: true, FinishReason: "stop", InputTokens: 3, OutputTokens: 5})
	report.Add(AttemptObservation{Duration: 20 * time.Millisecond, Status: statusSuccess, SchemaValid: true, FinishReason: "stop"})
	report.Add(AttemptObservation{Duration: 30 * time.Millisecond, Status: statusGatewayTimeout, SchemaValid: false, InputTokens: 7, OutputTokens: 11})
	summary := report.Summary()
	if summary.P50MS != 20 || summary.P95MS != 30 {
		t.Fatalf("unexpected percentiles: %+v", summary)
	}
	if summary.UsageMissing != 1 || summary.InputTokens != 10 || summary.OutputTokens != 16 {
		t.Fatalf("unexpected token aggregate: %+v", summary)
	}
	if summary.Counts.Success != 2 || summary.Counts.Timeout504 != 1 || summary.SchemaValid != 2 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
}

func TestSpikeSummaryIncludesSafeProviderWindowAndThinkingTotals(t *testing.T) {
	windowStart := time.Date(2026, 9, 30, 9, 0, 0, 0, time.UTC)
	report := NewReport("small-metadata", 2, "gemini", "gemini-3.6-flash", windowStart)
	report.Add(AttemptObservation{Duration: time.Second, Status: statusSuccess, InputTokens: 3, OutputTokens: 5, ThinkingTokens: 7})
	report.Add(AttemptObservation{Duration: 2 * time.Second, Status: statusSuccess, InputTokens: 11, OutputTokens: 13, ThinkingTokens: 17})

	summary := report.Summary()
	if summary.Provider != "gemini" || summary.Model != "gemini-3.6-flash" || summary.ThinkingTokens != 24 {
		t.Fatalf("missing safe provider/token metadata: %+v", summary)
	}
	if summary.WindowStartedAtWIB != "2026-09-30T16:00:00+07:00" {
		t.Fatalf("unexpected WIB window start: %q", summary.WindowStartedAtWIB)
	}
}

func TestNonGeminiReportIsIncompleteAndInvalidForGeminiDecision(t *testing.T) {
	summary := NewReport("small-metadata", 20, "openai", "gpt-4", time.Now()).Summary()
	if got := Classify(summary, StreamSummary{}); got != classificationIncomplete {
		t.Fatalf("non-Gemini classification = %q, want %q", got, classificationIncomplete)
	}
	if summary.Measurement != measurementIncomplete || summary.DecisionValidity != decisionInvalidForGemini {
		t.Fatalf("non-Gemini report must be incomplete and Gemini-invalid: %+v", summary)
	}
}

func TestClassifyRequiresCapabilitiesAndRepresentativeQuota(t *testing.T) {
	green := SpikeSummary{RequestedRuns: 20, CompletedRuns: 20, P95MS: 19_999, SchemaValid: 19, Capabilities: CapabilityReport{StructuredOutput: true, Streaming: true}}
	if got := Classify(green, StreamSummary{TTFBMS: 14_999, MaxIdleGapMS: 60_000}); got != classificationGreen {
		t.Fatalf("expected green, got %q", got)
	}
	yellow := green
	yellow.Counts.Timeout504 = 1
	if got := Classify(yellow, StreamSummary{TTFBMS: 14_999, MaxIdleGapMS: 60_000}); got != classificationYellow {
		t.Fatalf("expected yellow, got %q", got)
	}
	unsupported := green
	unsupported.Capabilities.StructuredOutput = false
	if got := Classify(unsupported, StreamSummary{}); got != classificationRed {
		t.Fatalf("unsupported structured output must be red, got %q", got)
	}
	incomplete := green
	incomplete.CompletedRuns = 4
	if got := Classify(incomplete, StreamSummary{}); got != classificationIncomplete {
		t.Fatalf("incomplete quota must be incomplete, got %q", got)
	}
}

func TestProviderCapabilitiesFallbackSkipsUnsupportedThinkingAndStreaming(t *testing.T) {
	planned := PlannedExperiments(ProviderCapabilities{StructuredOutput: true})
	if planned.Stream || len(planned.ThinkingModes) != 1 || planned.ThinkingModes[0] != thinkingDefault {
		t.Fatalf("unexpected fallback plan: %+v", planned)
	}
}

type fakeExtractionProvider struct {
	caps     service.ProviderCapabilities
	generate func(service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error)
}

func (p fakeExtractionProvider) ExtractionCapabilities() service.ProviderCapabilities { return p.caps }
func (p fakeExtractionProvider) GenerateExtraction(_ context.Context, r service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error) {
	if p.generate == nil {
		return service.ExtractionGenerateResult{}, nil
	}
	return p.generate(r)
}
func (p fakeExtractionProvider) Generate(context.Context, service.GenerateRequest) (service.GenerateResult, error) {
	return service.GenerateResult{}, nil
}

func TestSpikeStructuredOutputRequiresMetadataSchema(t *testing.T) {
	client := &spikeClient{extraction: fakeExtractionProvider{generate: func(service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error) {
		return service.ExtractionGenerateResult{Text: `{"unexpected":true}`, FinishReason: "STOP"}, nil
	}}}
	attempt := client.call(context.Background(), "synthetic only", true, "", false)
	if attempt.Status != statusSuccess || attempt.SchemaValid {
		t.Fatalf("schema-invalid response must be a successful but invalid observation: %+v", attempt)
	}
}

func TestSpikeStructuredOutputAcceptsMinimalP1Metadata(t *testing.T) {
	client := &spikeClient{extraction: fakeExtractionProvider{generate: func(service.ExtractionGenerateRequest) (service.ExtractionGenerateResult, error) {
		return service.ExtractionGenerateResult{Text: `{"client_company":"Atlas Cooperative","project_name":null,"pic_name":null,"client_name":null}`, FinishReason: "STOP"}, nil
	}}}
	attempt := client.call(context.Background(), "synthetic only", true, "", false)
	if attempt.Status != statusSuccess || !attempt.SchemaValid {
		t.Fatalf("minimal P1 metadata must validate: %+v", attempt)
	}
}

func TestGeminiExtractionProbeOmitsRejectedThinkingMode(t *testing.T) {
	var thinkingRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if _, hasThinking := body["thinkingConfig"]; hasThinking {
			thinkingRequests++
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"OK\"}]},\"finishReason\":\"STOP\"}]}\n\n")
			return
		}
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"{\"client_company\":null,\"project_name\":null,\"pic_name\":null,\"client_name\":null}"}]},"finishReason":"STOP"}]}`)
	}))
	defer server.Close()

	provider := service.NewGeminiProvider(service.ProviderConfig{ProviderName: "gemini", DecryptedAPIKey: "test-key", Model: "gemini-3.6-flash", BaseURL: server.URL, Timeout: time.Second})
	client := &spikeClient{provider: "gemini", model: "gemini-3.6-flash", extraction: provider}
	caps := client.probeCapabilities(context.Background())
	if len(caps.ThinkingModes) != 1 || caps.ThinkingModes[0] != thinkingDefault || thinkingRequests != 1 {
		t.Fatalf("rejected thinking must be excluded: caps=%+v requests=%d", caps, thinkingRequests)
	}
	client.call(context.Background(), "measurement", true, "", false)
	if thinkingRequests != 1 {
		t.Fatal("rejected thinking mode was sent in measurement")
	}
}

func TestLoadClientUsesProductionProviderSourceAndPreservesGeminiIdentity(t *testing.T) {
	original := resolveSpikeProvider
	resolveSpikeProvider = func(_ context.Context, companyID string) (service.AIProvider, string, string, error) {
		if companyID != "00000000-0000-0000-0000-000000000001" {
			t.Fatalf("unexpected company id: %q", companyID)
		}
		return fakeExtractionProvider{}, "gemini", "gemini-3.6-flash", nil
	}
	t.Cleanup(func() { resolveSpikeProvider = original })

	client, err := loadClient(context.Background(), "00000000-0000-0000-0000-000000000001")
	if err != nil || client.provider != "gemini" || client.model != "gemini-3.6-flash" || client.extraction == nil {
		t.Fatalf("loadClient must preserve factory/provider identity: client=%+v err=%v", client, err)
	}
}

func TestSpikeClassificationTreatsFortyFiveSecondsAsRed(t *testing.T) {
	summary := SpikeSummary{
		RequestedRuns: 20,
		CompletedRuns: 20,
		P95MS:         45_000,
		SchemaValid:   20,
		Counts:        Counts{Timeout504: 1},
		Capabilities:  CapabilityReport{StructuredOutput: true, Streaming: true},
	}
	if got := Classify(summary, StreamSummary{TTFBMS: 1, MaxIdleGapMS: 1}); got != classificationRed {
		t.Fatalf("p95 >=45 seconds must be red, got %q", got)
	}
}

func TestSpikeUnavailableCredentialProducesSafeIncompleteReport(t *testing.T) {
	fixturePath := filepath.Join(t.TempDir(), "fixture.json")
	const notesCanary = "NOTE_CANARY_UNAVAILABLE_923ab"
	if err := os.WriteFile(fixturePath, []byte(`{"meeting_notes":"`+notesCanary+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	original := loadSpikeClient
	loadSpikeClient = func(context.Context, string) (*spikeClient, error) {
		return nil, errors.New("raw provider response must not leak")
	}
	t.Cleanup(func() { loadSpikeClient = original })

	var stdout, stderr bytes.Buffer
	exitCode := run(context.Background(), []string{"--mode", "small-metadata", "--fixture", fixturePath, "--company-id", "00000000-0000-0000-0000-000000000000"}, &stdout, &stderr)
	if exitCode != 1 || stderr.String() != "error=provider_configuration_unavailable\n" {
		t.Fatalf("unexpected safe failure: exit=%d stderr=%q", exitCode, stderr.String())
	}
	for _, forbidden := range []string{notesCanary, "raw provider response"} {
		if strings.Contains(stdout.String(), forbidden) || strings.Contains(stderr.String(), forbidden) {
			t.Fatalf("unavailable-credential output leaked %q", forbidden)
		}
	}
	if !strings.Contains(stdout.String(), `"measurement": "incomplete"`) || !strings.Contains(stdout.String(), `"classification": "INCOMPLETE"`) {
		t.Fatalf("expected incomplete report, got %s", stdout.String())
	}
}
