// Non-production aggregate-only provider measurement command.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/database"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

const defaultFixture = "internal/modules/quotation/service/testdata/meeting_extraction_spike/synthetic_notes.json"

type options struct {
	Mode, Fixture, CompanyID, Output string
	Runs                             int
}
type fixture struct {
	MeetingNotes string `json:"meeting_notes"`
}
type spikeClient struct {
	provider   string
	model      string
	extraction service.ExtractionProvider
}

var loadSpikeClient = loadClient
var resolveSpikeProvider = resolveProviderForCompany

func main() { os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr)) }
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	o, err := parseOptions(args)
	if err != nil {
		fmt.Fprintln(stderr, "error=invalid_arguments")
		return 2
	}
	notes, err := loadFixture(o.Fixture)
	if err != nil {
		fmt.Fprintln(stderr, "error=invalid_fixture")
		return 2
	}
	c, err := loadSpikeClient(ctx, o.CompanyID)
	if err != nil {
		summary := NewReport(o.Mode, o.Runs).Summary()
		out, marshalErr := encodeReport(summary, StreamSummary{})
		if marshalErr != nil {
			return 1
		}
		if writeErr := writeReport(o.Output, out, stdout); writeErr != nil {
			fmt.Fprintln(stderr, "error=report_write_failed")
			return 1
		}
		fmt.Fprintln(stderr, "error=provider_configuration_unavailable")
		return 1
	}
	r, stream := execute(ctx, c, o, notes)
	summary := r.Summary()
	out, err := encodeReport(summary, stream)
	if err != nil {
		return 1
	}
	if err := writeReport(o.Output, out, stdout); err != nil {
		fmt.Fprintln(stderr, "error=report_write_failed")
		return 1
	}
	return 0
}

func encodeReport(summary SpikeSummary, stream StreamSummary) ([]byte, error) {
	return json.MarshalIndent(struct {
		SpikeSummary
		Stream         StreamSummary `json:"stream"`
		Classification string        `json:"classification"`
	}{summary, stream, Classify(summary, stream)}, "", "  ")
}

func writeReport(path string, out []byte, stdout io.Writer) error {
	if path != "" {
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return err
		}
	}
	_, _ = stdout.Write(append(out, '\n'))
	return nil
}
func parseOptions(args []string) (options, error) {
	o := options{Fixture: defaultFixture, Runs: 20}
	f := flag.NewFlagSet("meeting-extraction-spike", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Mode, "mode", "", "small-metadata|legacy-stream|thinking-ab")
	f.StringVar(&o.Fixture, "fixture", o.Fixture, "synthetic fixture")
	f.StringVar(&o.CompanyID, "company-id", "", "company UUID")
	f.StringVar(&o.Output, "output", "", "aggregate report")
	f.IntVar(&o.Runs, "runs", o.Runs, "run count")
	if e := f.Parse(args); e != nil || f.NArg() != 0 || uuid.Validate(o.CompanyID) != nil || o.Runs < 1 || (o.Mode != "small-metadata" && o.Mode != "legacy-stream" && o.Mode != "thinking-ab") {
		return options{}, errors.New("invalid")
	}
	return o, nil
}
func loadFixture(path string) (string, error) {
	b, e := os.ReadFile(filepath.Clean(path))
	if e != nil {
		return "", e
	}
	var f fixture
	if e = json.Unmarshal(b, &f); e != nil || strings.TrimSpace(f.MeetingNotes) == "" {
		return "", errors.New("invalid fixture")
	}
	return f.MeetingNotes, nil
}

// resolveProviderForCompany is the normal production credential route: the
// repository, request-scoped decrypt, and factory all remain inside the
// application's DBCredentialProviderSource.
func resolveProviderForCompany(ctx context.Context, companyID string) (service.AIProvider, string, string, error) {
	id, err := uuid.Parse(companyID)
	if err != nil {
		return nil, "", "", err
	}
	cfg := config.Load()
	db, err := database.New(cfg.Database.GetDSN())
	if err != nil {
		return nil, "", "", err
	}
	defer db.Close()
	source := service.NewDBCredentialProviderSource(repository.NewCredentialRepository(db.Pool), service.NewProviderFactory(), cfg.AI.CredentialEncryptionKey)
	return source.ProviderForCompany(ctx, id)
}

// loadClient exposes only provider identity and the extraction interface. It
// never receives, stores, or logs a decrypted credential.
func loadClient(ctx context.Context, companyID string) (*spikeClient, error) {
	provider, providerName, model, err := resolveSpikeProvider(ctx, companyID)
	if err != nil {
		return nil, err
	}
	extraction, ok := provider.(service.ExtractionProvider)
	if !ok || providerName == "" || model == "" {
		return nil, errors.New("extraction provider unavailable")
	}
	return &spikeClient{provider: providerName, model: model, extraction: extraction}, nil
}
func execute(ctx context.Context, c *spikeClient, o options, notes string) (*Report, StreamSummary) {
	caps := c.probeCapabilities(ctx)
	r := NewReport(o.Mode, o.Runs, c.provider, c.model, time.Now())
	r.SetCapabilities(caps)
	if o.Mode == "small-metadata" {
		for i := 0; i < o.Runs; i++ {
			r.Add(c.call(ctx, smallPrompt(notes), true, "", false))
		}
	} else if o.Mode == "legacy-stream" {
		if !caps.Streaming {
			return r, StreamSummary{}
		}
		a := c.call(ctx, legacyPrompt(notes), false, "", true)
		r.Add(a)
		return r, StreamSummary{TTFBMS: a.TTFB.Milliseconds(), TotalMS: a.Duration.Milliseconds(), MaxIdleGapMS: a.IdleGap.Milliseconds()}
	} else {
		for _, m := range PlannedExperiments(caps).ThinkingModes {
			for i := 0; i < o.Runs; i++ {
				r.Add(c.call(ctx, smallPrompt(notes), true, m, false))
			}
		}
	}
	return r, StreamSummary{}
}
func (c *spikeClient) probeCapabilities(ctx context.Context) ProviderCapabilities {
	j := c.call(ctx, "Extract only the four P1 metadata fields as JSON. Use null where unknown.", true, "", false)
	s := c.call(ctx, "Reply with OK.", false, "", true)
	capabilities := c.extraction.ExtractionCapabilities()
	modes := []string{thinkingDefault}
	for _, mode := range capabilities.ThinkingModes {
		if mode == service.ThinkingDefault {
			continue
		}
		candidate := string(mode)
		if c.call(ctx, "Reply with OK.", false, candidate, false).Status == statusSuccess {
			modes = append(modes, candidate)
		}
	}
	return ProviderCapabilities{StructuredOutput: capabilities.JSONSchema && j.Status == statusSuccess && j.SchemaValid, Streaming: capabilities.Streaming && s.Status == statusSuccess, ThinkingModes: modes}
}
func (c *spikeClient) call(ctx context.Context, prompt string, schema bool, thinking string, stream bool) AttemptObservation {
	start := time.Now()
	if c == nil || c.extraction == nil {
		return AttemptObservation{Duration: time.Since(start), Status: statusTimeout}
	}
	request := service.ExtractionGenerateRequest{Prompt: prompt, MaxOutputTokens: 512, Stream: stream}
	if schema {
		data, err := json.Marshal(metadataSchema())
		if err != nil {
			return AttemptObservation{Duration: time.Since(start), Status: statusTimeout}
		}
		request.JSONSchema = data
	}
	if thinking == thinkingMinimal {
		request.Thinking = service.ThinkingConfig{Mode: service.ThinkingMinimal, Budget: 128}
	} else if thinking == thinkingOff {
		request.Thinking = service.ThinkingConfig{Mode: service.ThinkingOff}
	}
	result, err := c.extraction.GenerateExtraction(ctx, request)
	if err != nil {
		return AttemptObservation{Duration: time.Since(start), Status: statusFromProviderError(err)}
	}
	valid := !schema
	if schema {
		valid = validMetadataSchema(result.Text)
	}
	return AttemptObservation{Duration: time.Since(start), TTFB: result.TTFB, IdleGap: result.IdleGap, Status: statusSuccess, SchemaValid: valid, FinishReason: safeFinish(result.FinishReason), InputTokens: result.Usage.InputTokens, OutputTokens: result.Usage.OutputTokens, ThinkingTokens: result.Usage.ThinkingTokens}
}

func validMetadataSchema(content string) bool {
	var value map[string]any
	if json.Unmarshal([]byte(content), &value) != nil || len(value) != 4 {
		return false
	}
	for _, field := range []string{"client_company", "project_name", "pic_name", "client_name"} {
		item, found := value[field]
		if !found || (item != nil && !isString(item)) {
			return false
		}
	}
	return true
}

func isString(value any) bool {
	_, ok := value.(string)
	return ok
}

func metadataSchema() map[string]any {
	properties := map[string]any{}
	for _, field := range []string{"client_company", "project_name", "pic_name", "client_name"} {
		properties[field] = map[string]any{"type": []string{"string", "null"}}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"client_company", "project_name", "pic_name", "client_name"},
		"properties":           properties,
	}
}
func statusFromProviderError(err error) attemptStatus {
	var extractionErr *service.ExtractionProviderError
	if errors.As(err, &extractionErr) {
		switch extractionErr.Code {
		case service.ErrExtractionProviderRateLimited:
			return statusRateLimited
		case service.ErrExtractionProviderTimeout:
			return statusGatewayTimeout
		}
	}
	return statusTimeout
}
func safeFinish(s string) string {
	switch s {
	case "stop", "length", "content_filter":
		return s
	default:
		return "other"
	}
}
func smallPrompt(notes string) string {
	return "Extract only P1 metadata: client_company, project_name, pic_name, client_name. Return null where unknown. Do not infer pricing. Notes:\n" + notes
}
func legacyPrompt(notes string) string {
	return "Return a JSON quotation extraction. Do not invent pricing. Meeting notes:\n" + notes
}
