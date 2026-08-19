package rnd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Helper to create a valid fixture JSON for tests.
func createTestBlueprintInputFixture(t *testing.T, dir string, filename string) string {
	t.Helper()
	input := BlueprintInput{
		SourceQuotationRevisionID: "rev-2026-001",
		SourceContext: BlueprintSourceContext{
			Subject:      "Core Banking Migration",
			SolutionType: "custom_software",
			Summary:      "High-scale transactional banking migration to cloud microservices.",
			Scope:        []string{"Account Management", "Fund Transfer", "Reporting"},
			Timeline:     "6 months",
		},
		BusinessGoals:          []string{"Improve throughput to 10k TPS", "99.99% availability"},
		Actors:                 []string{"Customer", "Branch Officer", "System Admin"},
		Channels:               []string{"Mobile App", "Web Portal", "ATM API"},
		BusinessPhases:         []string{"Onboarding", "Transaction", "Settlement", "Reconciliation"},
		BusinessRules:          []string{"All transactions above 100M IDR require branch officer approval."},
		KnownStates:            []string{"DRAFT", "PENDING_APPROVAL", "PROCESSING", "COMPLETED", "REJECTED"},
		Integrations:           []string{"BI-FAST", "Payment Gateway", "Core Ledger"},
		TechnologyPreferences:  []string{"Go", "PostgreSQL", "Kafka", "Docker"},
		OperationalConstraints: []string{"24/7 uptime", "Latency < 200ms"},
		AdditionalNotes:        "Strict regulatory compliance required.",
	}

	data, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test input: %v", err)
	}

	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("failed to write test fixture: %v", err)
	}

	return path
}

// customMockProvider allows per-stage response customization for failure and schema testing.
type customMockProvider struct {
	name            string
	failAtStage     BlueprintStage
	failErr         error
	customResponses map[BlueprintStage]string
}

func (p *customMockProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "custom-mock"
}

func (p *customMockProvider) Generate(ctx context.Context, request ProviderRequest) (ProviderResponse, error) {
	if err := ctx.Err(); err != nil {
		return ProviderResponse{}, err
	}
	stage := BlueprintStage(request.Stage)
	if p.failAtStage != "" && stage == p.failAtStage {
		if p.failErr != nil {
			return ProviderResponse{}, p.failErr
		}
		return ProviderResponse{}, errors.New("simulated stage generation failure")
	}

	if raw, ok := p.customResponses[stage]; ok {
		return ProviderResponse{Raw: raw, Latency: 15 * time.Millisecond}, nil
	}

	return ProviderResponse{Raw: "", Latency: 10 * time.Millisecond}, nil
}

func TestBlueprintRunner_ConfigValidation(t *testing.T) {
	tempDir := t.TempDir()
	mockProv := &customMockProvider{name: "mock"}

	tests := []struct {
		name    string
		cfg     BlueprintRunnerConfig
		wantErr string
	}{
		{
			name: "missing artifact root",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: "",
				OutputDir:    filepath.Join(tempDir, "out"),
				RawDir:       filepath.Join(tempDir, "raw"),
				Provider:     mockProv,
				Model:        "gemini-2.5-pro",
				Runs:         1,
			},
			wantErr: "artifact root is required",
		},
		{
			name: "missing output dir",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: ".",
				OutputDir:    "",
				RawDir:       filepath.Join(tempDir, "raw"),
				Provider:     mockProv,
				Model:        "gemini-2.5-pro",
				Runs:         1,
			},
			wantErr: "output directory is required",
		},
		{
			name: "missing raw dir",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: ".",
				OutputDir:    filepath.Join(tempDir, "out"),
				RawDir:       "",
				Provider:     mockProv,
				Model:        "gemini-2.5-pro",
				Runs:         1,
			},
			wantErr: "raw directory is required",
		},
		{
			name: "nil provider",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: ".",
				OutputDir:    filepath.Join(tempDir, "out"),
				RawDir:       filepath.Join(tempDir, "raw"),
				Provider:     nil,
				Model:        "gemini-2.5-pro",
				Runs:         1,
			},
			wantErr: "provider is required",
		},
		{
			name: "missing model",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: ".",
				OutputDir:    filepath.Join(tempDir, "out"),
				RawDir:       filepath.Join(tempDir, "raw"),
				Provider:     mockProv,
				Model:        "",
				Runs:         1,
			},
			wantErr: "model is required",
		},
		{
			name: "invalid runs count negative",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: ".",
				OutputDir:    filepath.Join(tempDir, "out"),
				RawDir:       filepath.Join(tempDir, "raw"),
				Provider:     mockProv,
				Model:        "gemini-2.5-pro",
				Runs:         -1,
			},
			wantErr: "runs must be between 1 and 10",
		},
		{
			name: "invalid runs count too large",
			cfg: BlueprintRunnerConfig{
				ArtifactRoot: ".",
				OutputDir:    filepath.Join(tempDir, "out"),
				RawDir:       filepath.Join(tempDir, "raw"),
				Provider:     mockProv,
				Model:        "gemini-2.5-pro",
				Runs:         11,
			},
			wantErr: "runs must be between 1 and 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner, err := NewBlueprintRunner(tt.cfg)
			if err == nil || runner != nil {
				t.Fatalf("expected error containing %q, got runner=%v, err=%v", tt.wantErr, runner, err)
			}
			if !errors.Is(err, errors.New(tt.wantErr)) && err.Error() != tt.wantErr {
				// Also allow substring match
				if len(tt.wantErr) > 0 && !containsString(err.Error(), tt.wantErr) {
					t.Fatalf("expected error message %q, got %q", tt.wantErr, err.Error())
				}
			}
		})
	}
}

func TestBlueprintRunner_FullRun_Success(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "output")
	rawDir := filepath.Join(tempDir, "raw")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "01-banking-migration.json")

	prov := &customMockProvider{name: "mock"}
	cfg := BlueprintRunnerConfig{
		ArtifactRoot: ".",
		OutputDir:    outDir,
		RawDir:       rawDir,
		Provider:     prov,
		Model:        "mock-model-v1",
		Runs:         1,
	}

	runner, err := NewBlueprintRunner(cfg)
	if err != nil {
		t.Fatalf("failed to instantiate runner: %v", err)
	}

	ctx := context.Background()
	report, err := runner.Run(ctx, fixturePath)
	if err != nil {
		t.Fatalf("unexpected error running pipeline: %v", err)
	}

	if report == nil {
		t.Fatal("expected non-nil report")
	}

	// 1. Check report status & metadata
	if report.Status != "pass" {
		t.Errorf("expected report status 'pass', got %q", report.Status)
	}
	if report.Provider != "mock" {
		t.Errorf("expected provider 'mock', got %q", report.Provider)
	}
	if report.Model != "mock-model-v1" {
		t.Errorf("expected model 'mock-model-v1', got %q", report.Model)
	}
	if report.SchemaVersion != BlueprintSchemaVersion {
		t.Errorf("expected schema version %q, got %q", BlueprintSchemaVersion, report.SchemaVersion)
	}

	// 2. Check 7 stages
	if len(report.Stages) != 7 {
		t.Fatalf("expected 7 stages, got %d", len(report.Stages))
	}

	expectedStages := []BlueprintStage{
		StageContextAnalysis,
		StageJourney,
		StageArchitecture,
		StageStateMachine,
		StageWorkflow,
		StageWBS,
		StageCrossValidation,
	}

	for i, expectedStage := range expectedStages {
		st := report.Stages[i]
		if st.Stage != expectedStage {
			t.Errorf("stage %d: expected %q, got %q", i, expectedStage, st.Stage)
		}
		if st.Status != StatusCompleted {
			t.Errorf("stage %d (%s): expected status 'completed', got %q (error: %s)", i, expectedStage, st.Status, st.Error)
		}
		if st.DurationMS < 0 {
			t.Errorf("stage %d: negative duration %d", i, st.DurationMS)
		}
	}

	// 3. Check summary metrics
	if !report.Summary.Passed {
		t.Errorf("expected summary.Passed to be true")
	}
	if report.Summary.TotalStages != 7 {
		t.Errorf("expected total stages 7, got %d", report.Summary.TotalStages)
	}
	if report.Summary.CompletedStages != 7 {
		t.Errorf("expected completed stages 7, got %d", report.Summary.CompletedStages)
	}
	if report.Summary.FailedStages != 0 {
		t.Errorf("expected failed stages 0, got %d", report.Summary.FailedStages)
	}
	if report.Summary.BlockingErrors != 0 {
		t.Errorf("expected 0 blocking errors, got %d", report.Summary.BlockingErrors)
	}
	if !report.Summary.CanPublishComplete {
		t.Errorf("expected CanPublishComplete to be true")
	}
	if !report.Summary.CanPublishPartial {
		t.Errorf("expected CanPublishPartial to be true")
	}

	// 4. Check bundle content
	if report.Bundle == nil {
		t.Fatal("expected non-nil bundle")
	}
	if report.Bundle.Title == "" {
		t.Errorf("expected non-empty bundle title")
	}
	if len(report.Bundle.Journey.Phases) == 0 {
		t.Errorf("expected journey phases in bundle")
	}
	if len(report.Bundle.Architecture.Nodes) == 0 {
		t.Errorf("expected architecture nodes in bundle")
	}
	if len(report.Bundle.StateMachine.States) == 0 {
		t.Errorf("expected state machine states in bundle")
	}
	if len(report.Bundle.Workflows) == 0 {
		t.Errorf("expected workflows in bundle")
	}
	if len(report.Bundle.WBS.Modules) == 0 {
		t.Errorf("expected WBS modules in bundle")
	}

	// 5. Check report file persistence in OutputDir
	reportFiles, err := filepath.Glob(filepath.Join(outDir, "*.json"))
	if err != nil || len(reportFiles) == 0 {
		t.Fatalf("expected report json file to be created in %s, found %v (err: %v)", outDir, reportFiles, err)
	}

	// Read and verify persisted report JSON
	data, err := os.ReadFile(reportFiles[0])
	if err != nil {
		t.Fatalf("failed to read persisted report: %v", err)
	}
	var persistedReport BlueprintBatchReport
	if err := json.Unmarshal(data, &persistedReport); err != nil {
		t.Fatalf("persisted report is not valid JSON: %v", err)
	}
	if persistedReport.Status != "pass" {
		t.Errorf("persisted report status expected 'pass', got %q", persistedReport.Status)
	}
}

func TestBlueprintRunner_StageFailure_Isolation(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "output")
	rawDir := filepath.Join(tempDir, "raw")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "02-failure-simulation.json")

	// Simulate failure at Stage 4 (state_machine)
	prov := &customMockProvider{
		name:        "mock",
		failAtStage: StageStateMachine,
		failErr:     errors.New("LLM timeout generating state machine"),
	}

	cfg := BlueprintRunnerConfig{
		ArtifactRoot: ".",
		OutputDir:    outDir,
		RawDir:       rawDir,
		Provider:     prov,
		Model:        "mock-model",
		Runs:         1,
	}

	runner, err := NewBlueprintRunner(cfg)
	if err != nil {
		t.Fatalf("failed to instantiate runner: %v", err)
	}

	ctx := context.Background()
	report, err := runner.Run(ctx, fixturePath)
	// Runner should return report with failure information and failure status
	if report == nil {
		t.Fatal("expected non-nil report even on stage failure")
	}

	// 1. Overall Status should be 'fail'
	if report.Status != "fail" {
		t.Errorf("expected aggregate status 'fail', got %q", report.Status)
	}
	if report.Summary.Passed {
		t.Errorf("expected summary.Passed to be false")
	}

	// 2. Stages 1..3 (context_analysis, journey, architecture) MUST be 'completed'
	// and their data preserved
	if report.Stages[0].Status != StatusCompleted {
		t.Errorf("stage 0 (context_analysis): expected completed, got %s", report.Stages[0].Status)
	}
	if report.Stages[1].Status != StatusCompleted {
		t.Errorf("stage 1 (journey): expected completed, got %s", report.Stages[1].Status)
	}
	if report.Stages[2].Status != StatusCompleted {
		t.Errorf("stage 2 (architecture): expected completed, got %s", report.Stages[2].Status)
	}

	// Verify bundle retained data from completed stages
	if report.Bundle == nil {
		t.Fatal("expected bundle to be retained")
	}
	if len(report.Bundle.Journey.Phases) == 0 {
		t.Errorf("expected journey data to be preserved in bundle")
	}
	if len(report.Bundle.Architecture.Nodes) == 0 {
		t.Errorf("expected architecture data to be preserved in bundle")
	}

	// 3. Stage 4 (state_machine) MUST be 'failed' with error recorded
	if report.Stages[3].Status != StatusFailed {
		t.Errorf("stage 3 (state_machine): expected failed, got %s", report.Stages[3].Status)
	}
	if report.Stages[3].Error == "" {
		t.Errorf("stage 3 (state_machine): expected non-empty error message")
	}

	// 4. Subsequent stages 5..7 (workflow, wbs, cross_validation) MUST be 'pending'
	if report.Stages[4].Status != StatusPending {
		t.Errorf("stage 4 (workflow): expected pending, got %s", report.Stages[4].Status)
	}
	if report.Stages[5].Status != StatusPending {
		t.Errorf("stage 5 (wbs): expected pending, got %s", report.Stages[5].Status)
	}
	if report.Stages[6].Status != StatusPending {
		t.Errorf("stage 6 (cross_validation): expected pending, got %s", report.Stages[6].Status)
	}

	// 5. Summary metrics
	if report.Summary.CompletedStages != 3 {
		t.Errorf("expected 3 completed stages, got %d", report.Summary.CompletedStages)
	}
	if report.Summary.FailedStages != 1 {
		t.Errorf("expected 1 failed stage, got %d", report.Summary.FailedStages)
	}
	if report.Summary.CanPublishComplete {
		t.Errorf("expected CanPublishComplete to be false")
	}
	if !report.Summary.CanPublishPartial {
		t.Errorf("expected CanPublishPartial to be true (completed stages are valid)")
	}

	// 6. Report file must still be written to OutputDir
	reportFiles, err := filepath.Glob(filepath.Join(outDir, "*.json"))
	if err != nil || len(reportFiles) == 0 {
		t.Fatalf("expected report json file in %s", outDir)
	}
}

func TestBlueprintRunner_SchemaValidationFailure_Isolation(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "output")
	rawDir := filepath.Join(tempDir, "raw")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "03-schema-violation.json")

	// Custom architecture response that violates schema: dangling edge targeting non-existent node
	malformedArch := ArchitectureSection{
		Groups: []ArchGroup{
			{ID: "grp-1", Label: "Frontend", Order: 1},
		},
		Nodes: []ArchNode{
			{ID: "node-web", GroupID: "grp-1", Type: "channel", Label: "Web", Layout: LayoutCoordinate{X: 0, Y: 0}},
		},
		Edges: []ArchEdge{
			{ID: "edge-1", Source: "node-web", Target: "node-nonexistent", Label: "HTTP", Type: "sync"},
		},
	}
	rawArchJSON, _ := json.Marshal(malformedArch)

	prov := &customMockProvider{
		name: "mock",
		customResponses: map[BlueprintStage]string{
			StageArchitecture: string(rawArchJSON),
		},
	}

	cfg := BlueprintRunnerConfig{
		ArtifactRoot: ".",
		OutputDir:    outDir,
		RawDir:       rawDir,
		Provider:     prov,
		Model:        "mock-model",
		Runs:         1,
	}

	runner, err := NewBlueprintRunner(cfg)
	if err != nil {
		t.Fatalf("failed to instantiate runner: %v", err)
	}

	ctx := context.Background()
	report, err := runner.Run(ctx, fixturePath)
	if report == nil {
		t.Fatal("expected report")
	}

	// Architecture stage should fail due to ERR_DANGLING_REFERENCE
	if report.Stages[2].Status != StatusFailed {
		t.Errorf("stage 2 (architecture) expected 'failed', got %s", report.Stages[2].Status)
	}
	if len(report.Stages[2].Issues) == 0 {
		t.Errorf("stage 2 expected validation issues")
	} else {
		foundDangling := false
		for _, iss := range report.Stages[2].Issues {
			if iss.Code == ErrDanglingReference {
				foundDangling = true
				break
			}
		}
		if !foundDangling {
			t.Errorf("expected ErrDanglingReference in issues, got %v", report.Stages[2].Issues)
		}
	}

	// Stages 1..2 were completed, subsequent stages pending
	if report.Stages[0].Status != StatusCompleted {
		t.Errorf("stage 0 expected completed, got %s", report.Stages[0].Status)
	}
	if report.Stages[1].Status != StatusCompleted {
		t.Errorf("stage 1 expected completed, got %s", report.Stages[1].Status)
	}
	for i := 3; i < 7; i++ {
		if report.Stages[i].Status != StatusPending {
			t.Errorf("stage %d expected pending, got %s", i, report.Stages[i].Status)
		}
	}
}

func TestBlueprintRunner_InvalidInput(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "output")
	rawDir := filepath.Join(tempDir, "raw")

	// 1. Non-existent fixture file
	cfg := BlueprintRunnerConfig{
		ArtifactRoot: ".",
		OutputDir:    outDir,
		RawDir:       rawDir,
		Provider:     &customMockProvider{name: "mock"},
		Model:        "mock-model",
		Runs:         1,
	}

	runner, err := NewBlueprintRunner(cfg)
	if err != nil {
		t.Fatalf("failed to instantiate runner: %v", err)
	}

	ctx := context.Background()
	report, err := runner.Run(ctx, filepath.Join(tempDir, "non-existent.json"))
	if err == nil {
		t.Error("expected error for non-existent fixture")
	}
	if report != nil && report.Status != "fail" {
		t.Errorf("expected report status 'fail' if report returned, got %s", report.Status)
	}

	// 2. Malformed JSON fixture file
	malformedPath := filepath.Join(tempDir, "malformed.json")
	_ = os.WriteFile(malformedPath, []byte("NOT_JSON"), 0o600)

	report, err = runner.Run(ctx, malformedPath)
	if err == nil {
		t.Error("expected error for malformed json fixture")
	}
	if report != nil && report.Status != "fail" {
		t.Errorf("expected report status 'fail' for malformed json, got %s", report.Status)
	}
}

func TestBlueprintRunner_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "output")
	rawDir := filepath.Join(tempDir, "raw")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "04-cancelled.json")

	cfg := BlueprintRunnerConfig{
		ArtifactRoot: ".",
		OutputDir:    outDir,
		RawDir:       rawDir,
		Provider:     &customMockProvider{name: "mock"},
		Model:        "mock-model",
		Runs:         1,
	}

	runner, err := NewBlueprintRunner(cfg)
	if err != nil {
		t.Fatalf("failed to instantiate runner: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	report, err := runner.Run(ctx, fixturePath)
	if err == nil {
		t.Errorf("expected context cancellation error, got nil")
	}
	if report != nil && report.Status != "fail" {
		t.Errorf("expected report status 'fail', got %s", report.Status)
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || filepath.Base(s) == substr || stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
