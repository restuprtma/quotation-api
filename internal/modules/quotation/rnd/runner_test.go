package rnd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const validFixtureJSON = `{
  "fixture_id":"synthetic",
  "description":"test fixture",
  "generation_input":{
    "client":{"company_name":"PT Test","pic_name":"PIC Test"},
    "quotation":{"number":"TEST/001","date":"2026-08-18","subject":"Test quotation"},
    "solution_type":"custom_software",
    "commercial_scheme":"dedicated_squad",
    "rate_card_code":"SYNTH-NATIONAL-2026",
    "project_duration_months":3,
    "budget_maximum":36000000,
    "summary":"A valid synthetic test input.",
    "mandatory_scope":["Scope A"],
    "optional_scope":["Scope B"],
    "allowed_standard_activities":["documentation"],
    "payment_terms":{"mode":"milestone","items":[{"label":"DP","percentage":50},{"label":"Final","percentage":50}]},
    "additional_notes":""
  },
  "expected":{"validation":"valid","term_type":"long_term","allocation_outcome":"feasible","security_outcome":"pass","assertions":[]}
}`

const validRateCardsJSON = `{
  "schema_version":"quotation-rate-card-fixture-v1",
  "fixture_notice":"SYNTHETIC TEST DATA ONLY.",
  "rate_cards":[{
    "code":"SYNTH-NATIONAL-2026",
    "market":"national",
    "currency":"IDR",
    "items":[{"role":"programmer","seniority":"senior","term_type":"long_term","amount_per_month":12000000}]
  }],
  "currency_rule":"Integer IDR only."
}`

const approvedLegalMarkdown = `# Test Legal

## Template Metadata

` + "```yaml" + `
template_code: quotation-legal-v1
status: approved
checksum: test-approved
` + "```" + `

## Deterministic HTML Block

` + "```html" + `
<section data-block-type="legal" data-template-code="quotation-legal-v1" data-modified="false">
  <h2>Ketentuan</h2>
  <p>Teks legal yang disetujui.</p>
</section>
` + "```" + `
`

func TestLoadFixtureRejectsUnknownField(t *testing.T) {
	path := writeTestFile(t, "fixture.json", strings.Replace(validFixtureJSON, `"commercial_scheme":"dedicated_squad",`, `"commercial_scheme":"dedicated_squad","unexpected":true,`, 1))
	_, err := loadFixture(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestLoadFixtureRejectsTrailingJSON(t *testing.T) {
	path := writeTestFile(t, "fixture.json", validFixtureJSON+` {"second":true}`)
	_, err := loadFixture(path)
	if err == nil || !strings.Contains(err.Error(), "single JSON value") {
		t.Fatalf("expected trailing JSON error, got %v", err)
	}
}

func TestLoadFixtureValidatesGenerationInput(t *testing.T) {
	tests := []struct {
		name    string
		old     string
		new     string
		wantErr string
	}{
		{name: "commercial scheme", old: `"commercial_scheme":"dedicated_squad"`, new: `"commercial_scheme":"fixed_scope"`, wantErr: "commercial_scheme"},
		{name: "invalid quotation date", old: `"date":"2026-08-18"`, new: `"date":"2026-02-30"`, wantErr: "quotation.date"},
		{name: "empty mandatory scope", old: `"mandatory_scope":["Scope A"]`, new: `"mandatory_scope":[]`, wantErr: "mandatory_scope"},
		{name: "payment total", old: `{"label":"Final","percentage":50}`, new: `{"label":"Final","percentage":40}`, wantErr: "total 100"},
		{name: "duplicate mandatory scope", old: `"mandatory_scope":["Scope A"]`, new: `"mandatory_scope":["Scope A","Scope A"]`, wantErr: "duplicate"},
		{name: "duplicate optional scope", old: `"optional_scope":["Scope B"]`, new: `"optional_scope":["Scope B","Scope B"]`, wantErr: "duplicate"},
		{name: "duplicate standard activity", old: `"allowed_standard_activities":["documentation"]`, new: `"allowed_standard_activities":["documentation","documentation"]`, wantErr: "duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestFile(t, "fixture.json", strings.Replace(validFixtureJSON, tt.old, tt.new, 1))
			_, err := loadFixture(path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadFixtureAcceptsCommittedFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "quotation-skill", "fixtures", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 10 {
		t.Fatalf("expected 10 committed fixtures, got %d", len(paths))
	}
	for _, path := range paths {
		if _, err := loadFixture(path); err != nil {
			t.Errorf("loadFixture(%s): %v", filepath.Base(path), err)
		}
	}
}

func TestLoadArtifactsValidatesRateCards(t *testing.T) {
	tests := []struct {
		name    string
		rates   string
		wantErr string
	}{
		{name: "duplicate compound key", rates: strings.Replace(validRateCardsJSON, `{"role":"programmer","seniority":"senior","term_type":"long_term","amount_per_month":12000000}`, `{"role":"programmer","seniority":"senior","term_type":"long_term","amount_per_month":12000000},{"role":"programmer","seniority":"senior","term_type":"long_term","amount_per_month":13000000}`, 1), wantErr: "duplicate rate"},
		{name: "non IDR", rates: strings.Replace(validRateCardsJSON, `"currency":"IDR"`, `"currency":"USD"`, 1), wantErr: "currency"},
		{name: "zero rate", rates: strings.Replace(validRateCardsJSON, `"amount_per_month":12000000`, `"amount_per_month":0`, 1), wantErr: "amount_per_month"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeArtifactRoot(t, approvedLegalMarkdown, tt.rates)
			_, err := loadArtifacts(root)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestLoadFixtureWithArtifactsRejectsUnknownRateCard(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	artifacts, err := loadArtifacts(root)
	if err != nil {
		t.Fatal(err)
	}
	path := writeTestFile(t, "fixture.json", strings.Replace(validFixtureJSON, "SYNTH-NATIONAL-2026", "SYNTH-INTERNATIONAL-2026", 1))
	_, err = loadFixtureWithArtifacts(path, artifacts)
	if err == nil || !strings.Contains(err.Error(), "rate_card_code") {
		t.Fatalf("expected unknown rate-card error, got %v", err)
	}
}

func TestLoadArtifactsRejectsMissingRequiredFile(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	if err := os.Remove(filepath.Join(root, "prompt-v1.md")); err != nil {
		t.Fatal(err)
	}
	_, err := loadArtifacts(root)
	if err == nil || !strings.Contains(err.Error(), "prompt-v1.md") {
		t.Fatalf("expected missing artifact error, got %v", err)
	}
}

func TestLoadArtifactsRejectsMalformedLegalArtifact(t *testing.T) {
	tests := []struct {
		name    string
		legal   string
		wantErr string
	}{
		{name: "duplicate yaml fence", legal: approvedLegalMarkdown + "\n```yaml\nstatus: approved\n```\n", wantErr: "YAML fence"},
		{name: "duplicate deterministic html fence", legal: approvedLegalMarkdown + "\n## Deterministic HTML Block\n\n```html\n<section></section>\n```\n", wantErr: "Deterministic HTML Block"},
		{name: "missing checksum", legal: strings.Replace(approvedLegalMarkdown, "checksum: test-approved\n", "", 1), wantErr: "checksum"},
		{name: "wrong template code", legal: strings.Replace(approvedLegalMarkdown, "quotation-legal-v1", "quotation-legal-v2", 1), wantErr: "template_code"},
		{name: "missing legal marker", legal: strings.Replace(approvedLegalMarkdown, `data-block-type="legal"`, `data-block-type="terms"`, 1), wantErr: "legal section marker"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := writeArtifactRoot(t, tt.legal, validRateCardsJSON)
			_, err := loadArtifacts(root)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestNewRunnerRejectsUnapprovedLegalTemplate(t *testing.T) {
	legal := strings.Replace(approvedLegalMarkdown, "status: approved", "status: candidate_pending_ba_approval", 1)
	root := writeArtifactRoot(t, legal, validRateCardsJSON)
	provider := &recordingProvider{}

	_, err := NewRunner(validRunnerConfig(root, provider))
	if !errors.Is(err, ErrLegalTemplateUnapproved) {
		t.Fatalf("expected ErrLegalTemplateUnapproved, got %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("provider called %d times before legal approval", provider.calls)
	}
}

func TestNewRunnerRejectsPendingChecksum(t *testing.T) {
	legal := strings.Replace(approvedLegalMarkdown, "checksum: test-approved", "checksum: pending_after_approval", 1)
	root := writeArtifactRoot(t, legal, validRateCardsJSON)
	_, err := NewRunner(validRunnerConfig(root, &recordingProvider{}))
	if !errors.Is(err, ErrLegalTemplateUnapproved) {
		t.Fatalf("expected ErrLegalTemplateUnapproved, got %v", err)
	}
}

func TestNewRunnerLoadsApprovedArtifacts(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	runner, err := NewRunner(validRunnerConfig(root, &recordingProvider{}))
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if runner.artifacts.LegalChecksum != "test-approved" || !strings.Contains(runner.artifacts.LegalHTML, "Teks legal") {
		t.Fatalf("approved legal artifact not loaded: %+v", runner.artifacts)
	}
}

func validRunnerConfig(root string, provider Provider) RunnerConfig {
	return RunnerConfig{
		ArtifactRoot: root,
		OutputDir:    filepath.Join(root, "output"),
		RawDir:       filepath.Join(root, "raw"),
		Provider:     provider,
		Model:        "test-model",
		Runs:         1,
	}
}

func writeArtifactRoot(t *testing.T, legal, rates string) string {
	t.Helper()
	root := t.TempDir()
	writeFileAt(t, filepath.Join(root, "rate-card-fixture.json"), rates)
	writeFileAt(t, filepath.Join(root, "safety-envelope.md"), "test safety envelope")
	writeFileAt(t, filepath.Join(root, "prompt-v1.md"), "test prompt")
	writeFileAt(t, filepath.Join(root, "legal-template-v1.md"), legal)
	return root
}

func writeTestFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeFileAt(t, path, content)
	return path
}

func writeFileAt(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type recordingProvider struct {
	calls int
}

func (p *recordingProvider) Name() string { return "recording" }

func (p *recordingProvider) Generate(_ context.Context, _ ProviderRequest) (ProviderResponse, error) {
	p.calls++
	return ProviderResponse{}, nil
}

type scriptedProvider struct {
	responses []ProviderResponse
	errors    []error
	requests  []ProviderRequest
}

func (p *scriptedProvider) Name() string { return "scripted" }
func (p *scriptedProvider) Generate(ctx context.Context, request ProviderRequest) (ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return ProviderResponse{}, err
	}
	p.requests = append(p.requests, request)
	index := len(p.requests) - 1
	if index < len(p.errors) && p.errors[index] != nil {
		return ProviderResponse{}, p.errors[index]
	}
	if index >= len(p.responses) {
		return ProviderResponse{}, errors.New("unexpected provider call")
	}
	return p.responses[index], nil
}

func TestRunnerSuccessfulRunPersistsProtectedEvidenceAndReport(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	fixture := writeTestFile(t, "fixture.json", validFixtureJSON)
	provider := &scriptedProvider{responses: []ProviderResponse{{Raw: feasibleAllocationJSON(), Latency: time.Millisecond}, {Raw: validRunnerHTML(), Latency: 2 * time.Millisecond}}}
	runner, err := NewRunner(validRunnerConfig(root, provider))
	if err != nil {
		t.Fatal(err)
	}
	report, err := runner.Run(context.Background(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 2 || provider.requests[0].Stage != StageAllocation || provider.requests[1].Stage != StageHTML {
		t.Fatalf("unexpected requests: %+v", provider.requests)
	}
	if report.Aggregate.Status != "pass" || report.Runs[0].Status != "pass" || report.Runs[0].SanitizedEvidencePath == "" {
		t.Fatalf("unexpected report: %+v", report)
	}
	rawFiles, _ := filepath.Glob(filepath.Join(runner.config.RawDir, "*.raw"))
	if len(rawFiles) != 2 {
		t.Fatalf("expected 2 raw files, got %v", rawFiles)
	}
	for _, path := range rawFiles {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("raw mode=%o", info.Mode().Perm())
		}
	}
	data, err := os.ReadFile(filepath.Join(runner.config.OutputDir, "synthetic-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{runner.config.RawDir, "budget_maximum", "secret"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("report leaked %q", forbidden)
		}
	}
}

func TestRunnerRetriesOverBudgetThenSucceeds(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	fixture := writeTestFile(t, "fixture.json", validFixtureJSON)
	over := strings.Replace(feasibleAllocationJSON(), `"employee_count":1`, `"employee_count":2`, 1)
	provider := &scriptedProvider{responses: []ProviderResponse{{Raw: over}, {Raw: over}, {Raw: feasibleAllocationJSON()}, {Raw: validRunnerHTML()}}}
	runner, _ := NewRunner(validRunnerConfig(root, provider))
	report, err := runner.Run(context.Background(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 4 || report.Runs[0].Iterations != 3 {
		t.Fatalf("calls=%d report=%+v", len(provider.requests), report.Runs[0])
	}
	payload, ok := provider.requests[2].Payload.(allocationPayload)
	if !ok || payload.RetryFeedback == nil || payload.RetryFeedback.BudgetExcessAmount != 36000000 {
		t.Fatalf("missing retry feedback: %#v", provider.requests[2].Payload)
	}
	encoded, _ := json.Marshal(payload)
	if strings.Contains(string(encoded), "rate_cards") || strings.Contains(string(encoded), "budget_maximum") {
		t.Fatalf("retry payload leaked forbidden data: %s", encoded)
	}
}

func TestRunnerBudgetUnfeasibleStopsBeforeHTML(t *testing.T) {
	rates := strings.Replace(validRateCardsJSON, `"role":"programmer","seniority":"senior","term_type":"long_term","amount_per_month":12000000`, `"role":"quality_assurance","seniority":"associate","term_type":"short_term","amount_per_month":7200000`, 1)
	root := writeArtifactRoot(t, approvedLegalMarkdown, rates)
	fixtureJSON := strings.Replace(validFixtureJSON, `"project_duration_months":3`, `"project_duration_months":1`, 1)
	fixtureJSON = strings.Replace(fixtureJSON, `"budget_maximum":36000000`, `"budget_maximum":5000000`, 1)
	fixtureJSON = strings.Replace(fixtureJSON, `"allocation_outcome":"feasible"`, `"allocation_outcome":"budget_unfeasible"`, 1)
	fixture := writeTestFile(t, "fixture.json", fixtureJSON)
	allocation := `{"resources":[{"role":"quality_assurance","seniority":"associate","employee_count":1,"period_months":1,"scope_references":["Scope A"],"reason":"Test"}],"selected_optional_scope":[],"standard_activities":[]}`
	provider := &scriptedProvider{responses: []ProviderResponse{{Raw: allocation}, {Raw: allocation}, {Raw: allocation}}}
	runner, _ := NewRunner(validRunnerConfig(root, provider))
	report, err := runner.Run(context.Background(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.requests) != 3 || report.Runs[0].Status != "budget_unfeasible_expected" || report.Aggregate.Status != "pass" {
		t.Fatalf("requests=%d report=%+v", len(provider.requests), report)
	}
	if matches, _ := filepath.Glob(filepath.Join(runner.config.OutputDir, "*.html")); len(matches) != 0 {
		t.Fatalf("unexpected HTML: %v", matches)
	}
}

func TestRunnerInvalidAllocationAndHTMLMutationFail(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	fixture := writeTestFile(t, "fixture.json", validFixtureJSON)
	t.Run("invalid allocation", func(t *testing.T) {
		provider := &scriptedProvider{responses: []ProviderResponse{{Raw: `not-json`}}}
		runner, _ := NewRunner(validRunnerConfig(root, provider))
		_, err := runner.Run(context.Background(), fixture)
		if err == nil || len(provider.requests) != 1 {
			t.Fatalf("requests=%d err=%v", len(provider.requests), err)
		}
	})
	t.Run("mutated legal", func(t *testing.T) {
		provider := &scriptedProvider{responses: []ProviderResponse{{Raw: feasibleAllocationJSON()}, {Raw: strings.Replace(validRunnerHTML(), "Teks legal yang disetujui.", "Legal changed.", 1)}}}
		runner, _ := NewRunner(validRunnerConfig(root, provider))
		report, err := runner.Run(context.Background(), fixture)
		if err == nil || report.Runs[0].Status != "fail" || report.Runs[0].SecurityChecks["S5"].Pass {
			t.Fatalf("report=%+v err=%v", report, err)
		}
		if matches, _ := filepath.Glob(filepath.Join(runner.config.OutputDir, "*.html")); len(matches) != 0 {
			t.Fatalf("unexpected HTML: %v", matches)
		}
	})
}

func TestRunnerMultipleRunsCancellationAndSafeFilename(t *testing.T) {
	root := writeArtifactRoot(t, approvedLegalMarkdown, validRateCardsJSON)
	fixtureJSON := strings.Replace(validFixtureJSON, `"fixture_id":"synthetic"`, `"fixture_id":"../../unsafe fixture"`, 1)
	fixture := writeTestFile(t, "fixture.json", fixtureJSON)
	provider := &scriptedProvider{responses: []ProviderResponse{{Raw: feasibleAllocationJSON()}, {Raw: validRunnerHTML()}, {Raw: feasibleAllocationJSON()}, {Raw: validRunnerHTML()}}}
	config := validRunnerConfig(root, provider)
	config.Runs = 2
	runner, _ := NewRunner(config)
	report, err := runner.Run(context.Background(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Runs) != 2 {
		t.Fatalf("runs=%d", len(report.Runs))
	}
	for _, name := range []string{"unsafe-fixture-run-01.html", "unsafe-fixture-run-02.html"} {
		if _, err := os.Stat(filepath.Join(config.OutputDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cancelProvider := &scriptedProvider{}
	cancelRunner, _ := NewRunner(validRunnerConfig(root, cancelProvider))
	_, err = cancelRunner.Run(ctx, fixture)
	if !errors.Is(err, context.Canceled) || len(cancelProvider.requests) != 0 {
		t.Fatalf("requests=%d err=%v", len(cancelProvider.requests), err)
	}
}

func feasibleAllocationJSON() string {
	return `{"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_months":3,"scope_references":["Scope A"],"reason":"Build"}],"selected_optional_scope":[],"standard_activities":[]}`
}

func validRunnerHTML() string {
	input := GenerationInput{Client: ClientInput{CompanyName: "PT Test", PICName: "PIC Test"}, Quotation: QuotationInput{Number: "TEST/001", Date: "2026-08-18", Subject: "Test quotation"}, MandatoryScope: []string{"Scope A"}, PaymentTerms: PaymentTerms{Items: []PaymentTermItem{{Label: "DP", Percentage: 50}, {Label: "Final", Percentage: 50}}}, BudgetMaximum: 36000000}
	allocation := EnrichedAllocation{Resources: []EnrichedResource{{Role: "programmer", Seniority: "senior", EmployeeCount: 1, PeriodMonths: 3, AmountPerMonth: 12000000, LineTotal: 36000000}}, Currency: "IDR", TermType: "long_term", TotalInvestment: 36000000, UtilizationPercent: 100}
	blocks := RenderDeterministicBlocks(input, allocation, strings.TrimSpace(strings.Split(strings.Split(approvedLegalMarkdown, "```html")[1], "```")[0]))
	payload := makeHTMLPayload(input, allocation, blocks)
	return payload.Metadata + blocks.CommercialHTML + payload.Scope + blocks.PaymentHTML + blocks.LegalHTML + payload.Closing + payload.Signature
}
