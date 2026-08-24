package rnd

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestBlueprintPipeline_ManualCompletion tests Task T6.4: Manual Completion Workflow.
func TestBlueprintPipeline_ManualCompletion(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "out")
	rawDir := filepath.Join(tempDir, "raw")
	artRoot := filepath.Join(tempDir, "art")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "manual-completion-input.json")

	schemaValidator := NewBlueprintSchemaValidator()

	// --------------------------------------------------------------------------
	// Test 1: BA Manual State Machine Completion Workflow
	// --------------------------------------------------------------------------
	t.Run("BA_ManualStateMachine_CompletionWorkflow", func(t *testing.T) {
		// 1. Initial run fails at Stage 4 (State Machine)
		failProv := &customMockProvider{
			name:        "mock-fail-sm",
			failAtStage: StageStateMachine,
			failErr:     errors.New("state machine generation failed"),
		}

		runner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     failProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create runner: %v", err)
		}

		report, err := runner.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected run error: %v", err)
		}
		if report.Status != "fail" {
			t.Fatalf("expected initial run to fail, got %s", report.Status)
		}
		if report.Stages[3].Status != StatusFailed {
			t.Fatalf("expected Stage 4 to be failed, got %s", report.Stages[3].Status)
		}

		// 2. BA transitions Stage 4 to manual_edit
		report.Stages[3].Status = StatusManualEdit

		// 3. BA attempts invalid manual input first (missing initial state & dangling transition)
		invalidManualSM := StateMachineSection{
			Entity:       "Ticket",
			InitialState: "state-nonexistent",
			States: []SMState{
				{ID: "state-idle", Label: "Idle", Type: "normal", Layout: LayoutCoordinate{X: 10, Y: 10}},
				{ID: "state-done", Label: "Done", Type: "terminal", Layout: LayoutCoordinate{X: 100, Y: 10}},
			},
			Transitions: []SMTransition{
				{ID: "trans-1", Source: "state-idle", Target: "state-missing", Event: "PROCESS"},
			},
		}

		invalidIssues := schemaValidator.ValidateStateMachine(&invalidManualSM)
		hasBlocking := false
		for _, iss := range invalidIssues {
			if iss.Blocking || iss.Severity == SeverityBlockingError {
				hasBlocking = true
			}
		}
		if !hasBlocking {
			t.Errorf("expected blocking validation errors for invalid manual state machine payload")
		}

		// Status remains manual_edit because validation failed
		if report.Stages[3].Status != StatusManualEdit {
			t.Errorf("status should remain manual_edit on validation failure")
		}

		// 4. BA fixes manual input with valid states and transitions
		validManualSM := StateMachineSection{
			Entity:       "Ticket",
			InitialState: "state-idle",
			States: []SMState{
				{ID: "state-idle", Label: "Idle", Type: "normal", Layout: LayoutCoordinate{X: 50, Y: 50}},
				{ID: "state-running", Label: "In Progress", Type: "normal", Layout: LayoutCoordinate{X: 200, Y: 50}},
				{ID: "state-done", Label: "Done", Type: "terminal", Layout: LayoutCoordinate{X: 350, Y: 50}},
			},
			Transitions: []SMTransition{
				{ID: "trans-1", Source: "state-idle", Target: "state-running", Event: "START", Actor: "actor-1", Actions: []string{"Begin Work"}},
				{ID: "trans-2", Source: "state-running", Target: "state-done", Event: "RESOLVE", Actor: "actor-1", Actions: []string{"Close Ticket"}},
			},
		}

		validIssues := schemaValidator.ValidateStateMachine(&validManualSM)
		if len(validIssues) > 0 {
			t.Fatalf("unexpected validation errors for valid manual SM: %v", validIssues)
		}

		// BA saves successfully: status transitions to validated
		report.Bundle.StateMachine = validManualSM
		report.Stages[3].Status = StatusValidated
		report.Stages[3].Issues = validIssues

		// 5. Resume pipeline for downstream stages (workflow, wbs, cross_validation)
		workRunner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     &customMockProvider{name: "mock-working"},
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create resume runner: %v", err)
		}

		finalReport, err := workRunner.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume after manual completion failed: %v", err)
		}

		// Verify pipeline completed with pass
		if finalReport.Status != "pass" {
			t.Errorf("expected final report status 'pass', got %q", finalReport.Status)
		}
		if !finalReport.Summary.Passed {
			t.Errorf("expected final report summary.passed == true")
		}
		if !finalReport.Summary.CanPublishComplete {
			t.Errorf("expected can_publish_complete == true")
		}
		if finalReport.Summary.BlockingErrors != 0 {
			t.Errorf("expected 0 blocking errors, got %d", finalReport.Summary.BlockingErrors)
		}

		// Verify Stage 4 retained its StatusValidated
		if finalReport.Stages[3].Status != StatusValidated {
			t.Errorf("expected Stage 4 to remain StatusValidated, got %s", finalReport.Stages[3].Status)
		}
		// Verify Downstream Stages 5..7 Completed
		if finalReport.Stages[4].Status != StatusCompleted {
			t.Errorf("expected Stage 5 to be completed, got %s", finalReport.Stages[4].Status)
		}
		if finalReport.Stages[5].Status != StatusCompleted {
			t.Errorf("expected Stage 6 to be completed, got %s", finalReport.Stages[5].Status)
		}
		if finalReport.Stages[6].Status != StatusCompleted {
			t.Errorf("expected Stage 7 to be completed, got %s", finalReport.Stages[6].Status)
		}
	})

	// --------------------------------------------------------------------------
	// Test 2: BA Manual WBS Completion Workflow
	// --------------------------------------------------------------------------
	t.Run("BA_ManualWBS_CompletionWorkflow", func(t *testing.T) {
		// Run pipeline that completes stages 1..5 but fails at Stage 6 (WBS)
		failWBSProv := &customMockProvider{
			name:        "mock-fail-wbs",
			failAtStage: StageWBS,
			failErr:     errors.New("wbs generation failed"),
		}

		runner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     failWBSProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create runner: %v", err)
		}

		report, err := runner.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected runner error: %v", err)
		}
		if report.Stages[5].Status != StatusFailed {
			t.Fatalf("expected Stage 6 to be failed, got %s", report.Stages[5].Status)
		}

		// BA inputs valid manual WBS section
		validManualWBS := WBSSection{
			Modules: []WBSModule{
				{
					ID:          "mod-portal",
					Name:        "Customer Portal Module",
					Description: "Self-service web interface",
					Priority:    "high",
					References:  []string{"architecture.node.node-web"},
					Features: []WBSFeature{
						{
							ID:                 "feat-auth",
							Name:               "Authentication Feature",
							Description:        "Secure login and MFA",
							AcceptanceCriteria: []string{"MFA via OTP", "JWT session tokens"},
						},
					},
				},
				{
					ID:           "mod-core-api",
					Name:         "Core API Backend",
					Description:  "Application transaction processing",
					Priority:     "high",
					Dependencies: []string{"mod-portal"},
					References:   []string{"architecture.node.node-api"},
					Features: []WBSFeature{
						{
							ID:                 "feat-tx-process",
							Name:               "Transaction Processing",
							Description:        "Core financial processing",
							AcceptanceCriteria: []string{"ACID consistency", "Idempotent endpoints"},
						},
					},
				},
			},
		}

		wbsIssues := schemaValidator.ValidateWBS(&validManualWBS)
		if len(wbsIssues) > 0 {
			t.Fatalf("unexpected validation errors for valid manual WBS: %v", wbsIssues)
		}

		// BA applies and validates manual WBS
		report.Bundle.WBS = validManualWBS
		report.Stages[5].Status = StatusValidated
		report.Stages[5].Issues = wbsIssues

		// Resume Stage 7 (CrossValidation)
		workRunner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     &customMockProvider{name: "mock-work"},
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create work runner: %v", err)
		}

		finalReport, err := workRunner.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}

		if finalReport.Status != "pass" {
			t.Errorf("expected final report status 'pass', got %q", finalReport.Status)
		}
		if finalReport.Stages[5].Status != StatusValidated {
			t.Errorf("expected Stage 6 to remain StatusValidated, got %s", finalReport.Stages[5].Status)
		}
		if finalReport.Stages[6].Status != StatusCompleted {
			t.Errorf("expected Stage 7 to be completed, got %s", finalReport.Stages[6].Status)
		}
	})
}
