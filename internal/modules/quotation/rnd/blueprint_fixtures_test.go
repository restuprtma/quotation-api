package rnd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	fixtureIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	forbiddenPhrases = []string{
		"api_key",
		"apikey",
		"bearer ",
		"secret_key",
		"private_key",
		"profit_margin",
		"profit margin",
		"commercial_rate",
		"commercial rate",
		"rate_card",
		"internal_budget",
		"budget_maximum",
	}
)

func getFixturesDirectory(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("testdata", "blueprint-skill", "fixtures")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("cannot resolve testdata/blueprint-skill/fixtures: %v", err)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		t.Fatalf("testdata/blueprint-skill/fixtures not found at %s — run from package directory", abs)
	}
	return abs
}

func TestBlueprintFixtures_AllTenPresentAndValidSchema(t *testing.T) {
	fixturesDir := getFixturesDirectory(t)
	expectedFixtures := []string{
		"01-public-service-queue.json",
		"02-ecommerce.json",
		"03-employee-portal.json",
		"04-inventory-procurement.json",
		"05-clinic-appointment.json",
		"06-learning-management.json",
		"07-support-ticketing.json",
		"08-field-service.json",
		"09-approval-workflow.json",
		"10-small-simple-system.json",
	}

	for _, fixtureFilename := range expectedFixtures {
		fixtureFilename := fixtureFilename
		t.Run(fixtureFilename, func(t *testing.T) {
			path := filepath.Join(fixturesDir, fixtureFilename)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read fixture file %q: %v", fixtureFilename, err)
			}

			// 1. Raw JSON string checks for commercial leaks or forbidden secrets
			lowerData := strings.ToLower(string(data))
			for _, phrase := range forbiddenPhrases {
				if strings.Contains(lowerData, phrase) {
					t.Errorf("fixture %q contains forbidden commercial/secret phrase %q", fixtureFilename, phrase)
				}
			}

			// 2. Parse into BlueprintInput struct
			var input BlueprintInput
			if err := json.Unmarshal(data, &input); err != nil {
				t.Fatalf("failed to unmarshal fixture %q into BlueprintInput: %v", fixtureFilename, err)
			}

			// 3. Validate SourceQuotationRevisionID
			if strings.TrimSpace(input.SourceQuotationRevisionID) == "" {
				t.Errorf("fixture %q: source_quotation_revision_id is empty", fixtureFilename)
			}
			if !fixtureIDPattern.MatchString(input.SourceQuotationRevisionID) {
				t.Errorf("fixture %q: source_quotation_revision_id %q does not match pattern ^[A-Za-z0-9_-]+$", fixtureFilename, input.SourceQuotationRevisionID)
			}

			// 4. Validate SourceContext
			if strings.TrimSpace(input.SourceContext.Subject) == "" {
				t.Errorf("fixture %q: source_context.subject is empty", fixtureFilename)
			}
			if strings.TrimSpace(input.SourceContext.SolutionType) == "" {
				t.Errorf("fixture %q: source_context.solution_type is empty", fixtureFilename)
			}
			if strings.TrimSpace(input.SourceContext.Summary) == "" {
				t.Errorf("fixture %q: source_context.summary is empty", fixtureFilename)
			}
			if len(input.SourceContext.Scope) == 0 {
				t.Errorf("fixture %q: source_context.scope must have at least 1 item", fixtureFilename)
			}
			for i, scopeItem := range input.SourceContext.Scope {
				if strings.TrimSpace(scopeItem) == "" {
					t.Errorf("fixture %q: source_context.scope[%d] is empty", fixtureFilename, i)
				}
			}
			if strings.TrimSpace(input.SourceContext.Timeline) == "" {
				t.Errorf("fixture %q: source_context.timeline is empty", fixtureFilename)
			}

			// 5. Validate BusinessGoals
			if len(input.BusinessGoals) == 0 {
				t.Errorf("fixture %q: business_goals must have at least 1 item", fixtureFilename)
			}
			for i, goal := range input.BusinessGoals {
				if strings.TrimSpace(goal) == "" {
					t.Errorf("fixture %q: business_goals[%d] is empty", fixtureFilename, i)
				}
			}

			// 6. Validate Actors
			if len(input.Actors) == 0 {
				t.Errorf("fixture %q: actors must have at least 1 item", fixtureFilename)
			}
			for i, actor := range input.Actors {
				if strings.TrimSpace(actor) == "" {
					t.Errorf("fixture %q: actors[%d] is empty", fixtureFilename, i)
				}
			}

			// 7. Validate Channels
			if len(input.Channels) == 0 {
				t.Errorf("fixture %q: channels must have at least 1 item", fixtureFilename)
			}
			for i, ch := range input.Channels {
				if strings.TrimSpace(ch) == "" {
					t.Errorf("fixture %q: channels[%d] is empty", fixtureFilename, i)
				}
			}

			// 8. Validate BusinessPhases
			if len(input.BusinessPhases) == 0 {
				t.Errorf("fixture %q: business_phases must have at least 1 item", fixtureFilename)
			}
			for i, ph := range input.BusinessPhases {
				if strings.TrimSpace(ph) == "" {
					t.Errorf("fixture %q: business_phases[%d] is empty", fixtureFilename, i)
				}
			}

			// 9. Validate String Arrays (business_rules, known_states, integrations, technology_preferences, operational_constraints)
			assertNonEmptyStringItems(t, fixtureFilename, "business_rules", input.BusinessRules)
			assertNonEmptyStringItems(t, fixtureFilename, "known_states", input.KnownStates)
			assertNonEmptyStringItems(t, fixtureFilename, "integrations", input.Integrations)
			assertNonEmptyStringItems(t, fixtureFilename, "technology_preferences", input.TechnologyPreferences)
			assertNonEmptyStringItems(t, fixtureFilename, "operational_constraints", input.OperationalConstraints)
		})
	}
}

func assertNonEmptyStringItems(t *testing.T, fixtureFilename, fieldName string, items []string) {
	t.Helper()
	for i, item := range items {
		if strings.TrimSpace(item) == "" {
			t.Errorf("fixture %q: %s[%d] must not be empty string", fixtureFilename, fieldName, i)
		}
	}
}

func TestBlueprintFixtures_MockRunnerExecution(t *testing.T) {
	fixturesDir := getFixturesDirectory(t)
	expectedFixtures := []string{
		"01-public-service-queue.json",
		"02-ecommerce.json",
		"03-employee-portal.json",
		"04-inventory-procurement.json",
		"05-clinic-appointment.json",
		"06-learning-management.json",
		"07-support-ticketing.json",
		"08-field-service.json",
		"09-approval-workflow.json",
		"10-small-simple-system.json",
	}

	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "out")
	rawDir := filepath.Join(tempDir, "raw")

	mockProv, err := NewProvider("mock", ProviderConfig{}, nil)
	if err != nil {
		t.Fatalf("failed to create mock provider: %v", err)
	}

	cfg := BlueprintRunnerConfig{
		ArtifactRoot: fixturesDir,
		OutputDir:    outDir,
		RawDir:       rawDir,
		Provider:     mockProv,
		Model:        "deterministic-mock-v1",
		Runs:         1,
		Clock:        time.Now,
	}

	runner, err := NewBlueprintRunner(cfg)
	if err != nil {
		t.Fatalf("failed to create BlueprintRunner: %v", err)
	}

	for _, fixtureFilename := range expectedFixtures {
		fixtureFilename := fixtureFilename
		t.Run("Runner_"+fixtureFilename, func(t *testing.T) {
			path := filepath.Join(fixturesDir, fixtureFilename)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			report, err := runner.Run(ctx, path)
			if err != nil {
				t.Fatalf("runner.Run failed on fixture %q: %v", fixtureFilename, err)
			}

			if report == nil {
				t.Fatalf("report is nil for fixture %q", fixtureFilename)
			}

			if report.Status != "pass" {
				t.Errorf("fixture %q expected report.Status == 'pass', got %q", fixtureFilename, report.Status)
			}

			if !report.Summary.Passed {
				t.Errorf("fixture %q expected report.Summary.Passed == true, got false", fixtureFilename)
			}

			if report.Summary.BlockingErrors != 0 {
				t.Errorf("fixture %q expected 0 blocking errors, got %d", fixtureFilename, report.Summary.BlockingErrors)
			}

			if report.Summary.CompletedStages != 7 {
				t.Errorf("fixture %q expected 7 completed stages, got %d", fixtureFilename, report.Summary.CompletedStages)
			}

			if !report.Summary.CanPublishComplete {
				t.Errorf("fixture %q expected CanPublishComplete == true, got false", fixtureFilename)
			}

			if report.Bundle == nil {
				t.Fatalf("fixture %q expected report.Bundle != nil", fixtureFilename)
			}

			// Validate Bundle has required sections populated
			if strings.TrimSpace(report.Bundle.Title) == "" {
				t.Errorf("fixture %q: bundle.Title is empty", fixtureFilename)
			}
			if len(report.Bundle.Journey.Phases) == 0 {
				t.Errorf("fixture %q: bundle.Journey.Phases is empty", fixtureFilename)
			}
			if len(report.Bundle.Architecture.Nodes) == 0 {
				t.Errorf("fixture %q: bundle.Architecture.Nodes is empty", fixtureFilename)
			}
			if len(report.Bundle.StateMachine.States) == 0 {
				t.Errorf("fixture %q: bundle.StateMachine.States is empty", fixtureFilename)
			}
			if len(report.Bundle.Workflows) == 0 {
				t.Errorf("fixture %q: bundle.Workflows is empty", fixtureFilename)
			}
			if len(report.Bundle.WBS.Modules) == 0 {
				t.Errorf("fixture %q: bundle.WBS.Modules is empty", fixtureFilename)
			}
		})
	}
}
