package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"venturo-skeleton-go/internal/config"
	"venturo-skeleton-go/internal/modules/quotation/rnd"
)

func TestParseOptions(t *testing.T) {
	defaults := optionDefaults{Provider: "mock", Model: "deterministic-rnd-v1", Artifacts: "artifacts"}
	tests := []struct {
		name string
		args []string
		want options
		fail string
	}{
		{name: "defaults quotation", args: []string{"--skill", "quotation", "--fixture", "fixture.json"}, want: options{Skill: "quotation", Fixture: "fixture.json", Provider: "mock", Model: "deterministic-rnd-v1", Artifacts: "artifacts", Runs: 2}},
		{name: "defaults blueprint", args: []string{"--skill", "blueprint", "--fixture", "fixture.json"}, want: options{Skill: "blueprint", Fixture: "fixture.json", Provider: "mock", Model: "deterministic-rnd-v1", Artifacts: "artifacts", Runs: 2}},
		{name: "custom", args: []string{"--skill", "quotation", "--fixture", "fixture.json", "--provider", "ollama", "--model", "custom", "--runs", "10", "--output", "out", "--artifacts", "root"}, want: options{Skill: "quotation", Fixture: "fixture.json", Provider: "ollama", Model: "custom", Output: "out", Artifacts: "root", Runs: 10}},
		{name: "missing skill", args: []string{"--fixture", "fixture.json"}, fail: "--skill"},
		{name: "missing fixture", args: []string{"--skill", "quotation"}, fail: "--fixture"},
		{name: "wrong skill", args: []string{"--skill", "invalid", "--fixture", "fixture.json"}, fail: "quotation or blueprint"},
		{name: "wrong provider", args: []string{"--skill", "quotation", "--fixture", "fixture.json", "--provider", "secret-provider"}, fail: "provider"},
		{name: "bad runs", args: []string{"--skill", "quotation", "--fixture", "fixture.json", "--runs", "11"}, fail: "runs"},
		{name: "unknown flag", args: []string{"--skill", "quotation", "--fixture", "fixture.json", "--unknown", "value"}, fail: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOptions(tt.args, defaults)
			if tt.fail == "" {
				if err != nil {
					t.Fatal(err)
				}
				if got != tt.want {
					t.Fatalf("got=%+v want=%+v", got, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.fail)) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestParseOptionsBlueprint(t *testing.T) {
	defaults := optionDefaults{Provider: "mock", Model: "deterministic-rnd-v1"}

	// Test default artifacts resolution for blueprint
	got, err := parseOptions([]string{"--skill", "blueprint", "--fixture", "fixture.json"}, defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Artifacts != defaultBlueprintArtifacts {
		t.Fatalf("got artifacts=%q want=%q", got.Artifacts, defaultBlueprintArtifacts)
	}
	if got.Skill != "blueprint" {
		t.Fatalf("got skill=%q want=blueprint", got.Skill)
	}

	// Test default artifacts resolution for quotation
	gotQuotation, err := parseOptions([]string{"--skill", "quotation", "--fixture", "fixture.json"}, defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuotation.Artifacts != defaultQuotationArtifacts {
		t.Fatalf("got artifacts=%q want=%q", gotQuotation.Artifacts, defaultQuotationArtifacts)
	}

	// Test custom artifacts with blueprint
	gotCustom, err := parseOptions([]string{"--skill", "blueprint", "--fixture", "fixture.json", "--artifacts", "custom/artifacts"}, defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotCustom.Artifacts != "custom/artifacts" {
		t.Fatalf("got artifacts=%q want=custom/artifacts", gotCustom.Artifacts)
	}
}

func TestParseOptionsDoesNotExposeEnvironmentSecret(t *testing.T) {
	defaults := optionDefaults{Provider: "mock", Model: "safe", Artifacts: "artifacts"}
	secret := "super-secret-gemini-key"
	if err := os.Setenv("GEMINI_API_KEY", secret); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Unsetenv("GEMINI_API_KEY") })
	_, err := parseOptions([]string{"--skill", "quotation", "--fixture", "fixture.json", "--runs", "not-a-number"}, defaults)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe parse error: %v", err)
	}
}

func TestRunLegalGateAndSafeOutput(t *testing.T) {
	oldConfig := configLoader
	oldProvider := providerFactory
	oldRunner := runnerFactory
	t.Cleanup(func() { configLoader, providerFactory, runnerFactory = oldConfig, oldProvider, oldRunner })
	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: time.Second}}
	}
	calls := 0
	providerFactory = func(name string, cfg rnd.ProviderConfig) (rnd.Provider, error) {
		calls++
		return &testProvider{}, nil
	}
	root := t.TempDir()
	writePendingArtifacts(t, root)
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{"fixture_id":"fixture","description":"test","generation_input":{},"expected":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{"--skill", "quotation", "--fixture", fixture, "--artifacts", root}, stdout, stderr)
	if code == 0 || calls != 1 {
		t.Fatalf("code=%d calls=%d stdout=%q stderr=%q", code, calls, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "legal template is not approved") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), "super-secret") {
		t.Fatal("secret leaked")
	}
}

func TestRunSuccessfulPathUsesSafeConsoleSummary(t *testing.T) {
	oldConfig := configLoader
	oldProvider := providerFactory
	oldRunner := runnerFactory
	t.Cleanup(func() { configLoader, providerFactory, runnerFactory = oldConfig, oldProvider, oldRunner })
	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: time.Second}}
	}
	providerFactory = func(name string, cfg rnd.ProviderConfig) (rnd.Provider, error) { return &testProvider{}, nil }
	runnerFactory = func(config rnd.RunnerConfig) (runnerAPI, error) {
		return &fakeRunner{output: config.OutputDir}, nil
	}
	root := t.TempDir()
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output")
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{"--skill", "quotation", "--fixture", fixture, "--artifacts", root, "--output", output, "--runs", "1"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=pass") || !strings.Contains(stdout.String(), "output=") || !strings.Contains(stdout.String(), "report=") {
		t.Fatalf("unsafe/incomplete stdout: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "budget") || strings.Contains(stdout.String(), "raw") || strings.Contains(stdout.String(), "prompt") {
		t.Fatalf("sensitive stdout: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(output, "fixture-report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunBlueprintSuccessfulPath(t *testing.T) {
	oldConfig := configLoader
	oldProvider := providerFactory
	oldBlueprintRunner := blueprintRunnerFactory
	t.Cleanup(func() {
		configLoader, providerFactory, blueprintRunnerFactory = oldConfig, oldProvider, oldBlueprintRunner
	})

	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: time.Second}}
	}
	providerFactory = func(name string, cfg rnd.ProviderConfig) (rnd.Provider, error) { return &testProvider{}, nil }
	blueprintRunnerFactory = func(config rnd.BlueprintRunnerConfig) (blueprintRunnerAPI, error) {
		return &fakeBlueprintRunner{output: config.OutputDir, status: "pass"}, nil
	}

	root := t.TempDir()
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output")
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{"--skill", "blueprint", "--fixture", fixture, "--artifacts", root, "--output", output, "--runs", "1"}, stdout, stderr)
	if code != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=pass") || !strings.Contains(stdout.String(), "output=") || !strings.Contains(stdout.String(), "report=") {
		t.Fatalf("unsafe/incomplete stdout: %q", stdout.String())
	}
	if strings.Contains(stdout.String(), "budget") || strings.Contains(stdout.String(), "raw") || strings.Contains(stdout.String(), "prompt") {
		t.Fatalf("sensitive stdout: %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(output, "fixture-report.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunBlueprintFailedPath(t *testing.T) {
	oldConfig := configLoader
	oldProvider := providerFactory
	oldBlueprintRunner := blueprintRunnerFactory
	t.Cleanup(func() {
		configLoader, providerFactory, blueprintRunnerFactory = oldConfig, oldProvider, oldBlueprintRunner
	})

	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: time.Second}}
	}
	providerFactory = func(name string, cfg rnd.ProviderConfig) (rnd.Provider, error) { return &testProvider{}, nil }
	blueprintRunnerFactory = func(config rnd.BlueprintRunnerConfig) (blueprintRunnerAPI, error) {
		return &fakeBlueprintRunner{output: config.OutputDir, status: "fail"}, nil
	}

	root := t.TempDir()
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output")
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{"--skill", "blueprint", "--fixture", fixture, "--artifacts", root, "--output", output, "--runs", "1"}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected code=1, got code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=fail") {
		t.Fatalf("expected status=fail in stdout, got: %q", stdout.String())
	}
}

func TestRunBlueprintRunnerInitFailed(t *testing.T) {
	oldConfig := configLoader
	oldProvider := providerFactory
	oldBlueprintRunner := blueprintRunnerFactory
	t.Cleanup(func() {
		configLoader, providerFactory, blueprintRunnerFactory = oldConfig, oldProvider, oldBlueprintRunner
	})

	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: time.Second}}
	}
	providerFactory = func(name string, cfg rnd.ProviderConfig) (rnd.Provider, error) { return &testProvider{}, nil }
	blueprintRunnerFactory = func(config rnd.BlueprintRunnerConfig) (blueprintRunnerAPI, error) {
		return nil, errors.New("init failure")
	}

	root := t.TempDir()
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output")
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{"--skill", "blueprint", "--fixture", fixture, "--artifacts", root, "--output", output}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected code=1, got code=%d", code)
	}
	if !strings.Contains(stderr.String(), "error=runner initialization failed") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunBlueprintExecutionError(t *testing.T) {
	oldConfig := configLoader
	oldProvider := providerFactory
	oldBlueprintRunner := blueprintRunnerFactory
	t.Cleanup(func() {
		configLoader, providerFactory, blueprintRunnerFactory = oldConfig, oldProvider, oldBlueprintRunner
	})

	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: time.Second}}
	}
	providerFactory = func(name string, cfg rnd.ProviderConfig) (rnd.Provider, error) { return &testProvider{}, nil }
	blueprintRunnerFactory = func(config rnd.BlueprintRunnerConfig) (blueprintRunnerAPI, error) {
		return &fakeBlueprintRunner{err: errors.New("exec error")}, nil
	}

	root := t.TempDir()
	fixture := filepath.Join(root, "fixture.json")
	if err := os.WriteFile(fixture, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "output")
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{"--skill", "blueprint", "--fixture", fixture, "--artifacts", root, "--output", output}, stdout, stderr)
	if code != 1 {
		t.Fatalf("expected code=1, got code=%d", code)
	}
	if !strings.Contains(stderr.String(), "error=blueprint R&D run failed") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunBlueprintEndToEndMock(t *testing.T) {
	oldConfig := configLoader
	t.Cleanup(func() { configLoader = oldConfig })

	configLoader = func() *config.Config {
		return &config.Config{AI: config.AIConfig{Provider: "mock", Timeout: 5 * time.Second}}
	}

	root := t.TempDir()
	fixturePath := filepath.Join(root, "sample-blueprint-fixture.json")
	fixtureContent := `{
		"source_quotation_revision_id": "rev-001",
		"source_context": {
			"subject": "Payment Gateway Integration",
			"solution_type": "api_integration",
			"summary": "Build high-availability payment gateway API.",
			"scope": ["API Design", "Integration"],
			"timeline": "3 months"
		},
		"business_goals": ["Automate payments", "Reduce latency"],
		"actors": ["Customer", "Merchant", "Bank Admin"],
		"channels": ["Web", "Mobile"],
		"business_phases": ["Initiation", "Processing", "Settlement"],
		"business_rules": ["Idempotency required"],
		"known_states": ["Pending", "Processed", "Failed"],
		"integrations": ["Bank Core API"],
		"technology_preferences": ["Go", "PostgreSQL"],
		"operational_constraints": ["99.99% availability"],
		"additional_notes": "None"
	}`
	if err := os.WriteFile(fixturePath, []byte(fixtureContent), 0o600); err != nil {
		t.Fatal(err)
	}

	outputDir := filepath.Join(t.TempDir(), "output")
	stdout, stderr := new(strings.Builder), new(strings.Builder)
	code := run(context.Background(), []string{
		"--skill", "blueprint",
		"--fixture", fixturePath,
		"--artifacts", root,
		"--output", outputDir,
		"--provider", "mock",
		"--runs", "1",
	}, stdout, stderr)

	if code != 0 {
		t.Fatalf("expected code=0, got %d. stdout: %q, stderr: %q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=pass") {
		t.Fatalf("expected status=pass, got stdout: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "output="+outputDir) {
		t.Fatalf("expected output dir in stdout, got: %q", stdout.String())
	}
	expectedReportFile := filepath.Join(outputDir, "sample-blueprint-fixture-report.json")
	if !strings.Contains(stdout.String(), "report="+expectedReportFile) {
		t.Fatalf("expected report path in stdout, got: %q", stdout.String())
	}
	if _, err := os.Stat(expectedReportFile); err != nil {
		t.Fatalf("report file was not created: %v", err)
	}
}

func TestPrepareOutputRejectsArtifactTree(t *testing.T) {
	root := t.TempDir()
	defaultOutput, err := prepareOutput("", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(defaultOutput) })
	if inside, err := pathWithin(root, defaultOutput); err != nil || inside {
		t.Fatalf("default output=%q inside=%v err=%v", defaultOutput, inside, err)
	}
	for _, output := range []string{root, filepath.Join(root, "generated")} {
		if _, err := prepareOutput(output, root); err == nil {
			t.Fatalf("expected rejection for %s", output)
		}
	}
	sibling := root + "-copy"
	t.Cleanup(func() { _ = os.RemoveAll(sibling) })
	if output, err := prepareOutput(sibling, root); err != nil || output == "" {
		t.Fatalf("sibling output=%q err=%v", output, err)
	}
}

func TestProviderSpecificModelDefaults(t *testing.T) {
	defaults := optionDefaults{Provider: "mock", Model: "deterministic-rnd-v1", MockModel: "deterministic-rnd-v1", OllamaModel: "llama3", GeminiModel: "gemini-test", AgentRouterModel: "gpt-test", Artifacts: "artifacts"}
	for provider, want := range map[string]string{"mock": "deterministic-rnd-v1", "ollama": "llama3", "gemini": "gemini-test", "agentrouter": "gpt-test"} {
		got, err := parseOptions([]string{"--skill", "quotation", "--fixture", "fixture.json", "--provider", provider}, defaults)
		if err != nil || got.Model != want {
			t.Fatalf("provider=%s model=%q err=%v", provider, got.Model, err)
		}
	}
}

type testProvider struct{}

func (p *testProvider) Name() string { return "mock" }
func (p *testProvider) Generate(context.Context, rnd.ProviderRequest) (rnd.ProviderResponse, error) {
	return rnd.ProviderResponse{}, nil
}

type fakeRunner struct{ output string }

func (r *fakeRunner) Run(context.Context, string) (*rnd.BatchReport, error) {
	report := rnd.BatchReport{Fixture: "fixture", Aggregate: rnd.AggregateReport{Status: "pass"}}
	path, _ := rnd.ReportPath(r.output, report.Fixture)
	if err := rnd.WriteReport(path, report); err != nil {
		return nil, err
	}
	return &report, nil
}

type fakeBlueprintRunner struct {
	output string
	status string
	err    error
}

func (r *fakeBlueprintRunner) Run(ctx context.Context, fixturePath string) (*rnd.BlueprintBatchReport, error) {
	if r.err != nil {
		return nil, r.err
	}
	status := r.status
	if status == "" {
		status = "pass"
	}
	safeFixture := "fixture"
	report := rnd.BlueprintBatchReport{
		Fixture: safeFixture,
		Status:  status,
	}
	reportPath := filepath.Join(r.output, fmt.Sprintf("%s-report.json", safeFixture))
	data, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile(reportPath, data, 0o640)
	return &report, nil
}

func writePendingArtifacts(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"rate-card-fixture.json": `{"schema_version":"quotation-rate-card-fixture-v1","fixture_notice":"synthetic","rate_cards":[{"code":"SYNTH-NATIONAL-2026","market":"national","currency":"IDR","items":[{"role":"programmer","seniority":"senior","term_type":"long_term","amount_per_month":12000000}]}],"currency_rule":"integer"}`,
		"safety-envelope.md":     "safe",
		"prompt-v1.md":           "prompt",
		"legal-template-v1.md":   "# Legal\n\n```yaml\ntemplate_code: quotation-legal-v1\nstatus: candidate_pending_ba_approval\nchecksum: pending_after_approval\n```\n\n## Deterministic HTML Block\n\n```html\n<section data-block-type=\"legal\" data-template-code=\"quotation-legal-v1\" data-modified=\"false\"><h2>Ketentuan</h2></section>\n```\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
