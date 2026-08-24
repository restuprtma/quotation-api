package rnd

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

// TestBlueprintPipeline_FailureIsolation tests Task T6.3: Pipeline Failure & Retry Isolation.
func TestBlueprintPipeline_FailureIsolation(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "out")
	rawDir := filepath.Join(tempDir, "raw")
	artRoot := filepath.Join(tempDir, "art")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "resilience-input.json")

	// --------------------------------------------------------------------------
	// Test 1: Injeksi Error pada Stage 4 (State Machine) & Retry Recovery
	// --------------------------------------------------------------------------
	t.Run("Stage4_StateMachine_FailureAndRetry", func(t *testing.T) {
		failProv := &customMockProvider{
			name:        "mock-fail-sm",
			failAtStage: StageStateMachine,
			failErr:     errors.New("LLM generation timeout for state machine"),
		}

		runnerFail, err := NewBlueprintRunner(BlueprintRunnerConfig{
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

		// Initial Run that fails at Stage 4
		report, err := runnerFail.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected runner execution crash: %v", err)
		}

		if report.Status != "fail" {
			t.Errorf("expected report status 'fail', got %q", report.Status)
		}

		// Verify zero-wipe isolation for Upstream Stages 1..3
		if report.Stages[0].Status != StatusCompleted {
			t.Errorf("expected Stage 1 (Context) to be completed, got %s", report.Stages[0].Status)
		}
		if report.Stages[1].Status != StatusCompleted {
			t.Errorf("expected Stage 2 (Journey) to be completed, got %s", report.Stages[1].Status)
		}
		if report.Stages[2].Status != StatusCompleted {
			t.Errorf("expected Stage 3 (Architecture) to be completed, got %s", report.Stages[2].Status)
		}

		// Verify upstream payloads are intact in the bundle
		if report.Bundle.Title == "" || report.Bundle.Summary == "" {
			t.Errorf("expected Context payload to remain intact")
		}
		if len(report.Bundle.Journey.Phases) == 0 || len(report.Bundle.Journey.Activities) == 0 {
			t.Errorf("expected Journey payload to remain intact")
		}
		if len(report.Bundle.Architecture.Nodes) == 0 {
			t.Errorf("expected Architecture payload to remain intact")
		}

		// Verify Stage 4 is failed with recorded error
		if report.Stages[3].Status != StatusFailed {
			t.Errorf("expected Stage 4 (StateMachine) to be failed, got %s", report.Stages[3].Status)
		}
		if report.Stages[3].Error == "" {
			t.Errorf("expected Stage 4 error message to be recorded")
		}

		// Verify Downstream Stages 5..7 remain Pending
		if report.Stages[4].Status != StatusPending {
			t.Errorf("expected Stage 5 (Workflow) to be pending, got %s", report.Stages[4].Status)
		}
		if report.Stages[5].Status != StatusPending {
			t.Errorf("expected Stage 6 (WBS) to be pending, got %s", report.Stages[5].Status)
		}
		if report.Stages[6].Status != StatusPending {
			t.Errorf("expected Stage 7 (CrossValidation) to be pending, got %s", report.Stages[6].Status)
		}

		// Trigger Retry using working provider via Resume
		workProv := &customMockProvider{name: "mock-working"}
		runnerResume, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     workProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create resume runner: %v", err)
		}

		resumedReport, err := runnerResume.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}

		if resumedReport.Status != "pass" {
			t.Errorf("expected resumed report status 'pass', got %q", resumedReport.Status)
		}
		if resumedReport.Summary.CompletedStages != 7 {
			t.Errorf("expected 7 completed stages after resume, got %d", resumedReport.Summary.CompletedStages)
		}
		if resumedReport.Stages[3].RetryCount < 1 {
			t.Errorf("expected retry count >= 1 for Stage 4, got %d", resumedReport.Stages[3].RetryCount)
		}
		if !resumedReport.Summary.CanPublishComplete {
			t.Errorf("expected can_publish_complete == true after full resume")
		}
	})

	// --------------------------------------------------------------------------
	// Test 2: Injeksi Schema Error pada Stage 3 (Architecture) & Retry Recovery
	// --------------------------------------------------------------------------
	t.Run("Stage3_Architecture_SchemaErrorAndRetry", func(t *testing.T) {
		// Injected architecture JSON with dangling edge target
		invalidArchJSON := `{
			"groups": [{"id": "grp-1", "label": "Tier 1", "order": 1}],
			"nodes": [{"id": "node-1", "group_id": "grp-1", "type": "service", "label": "Service 1", "layout": {"x": 10, "y": 10}}],
			"edges": [{"id": "edge-1", "source": "node-1", "target": "node-nonexistent", "label": "HTTP", "type": "sync"}]
		}`

		failArchProv := &customMockProvider{
			name: "mock-fail-arch",
			customResponses: map[BlueprintStage]string{
				StageArchitecture: invalidArchJSON,
			},
		}

		runner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     failArchProv,
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

		if report.Status != "fail" {
			t.Errorf("expected report status 'fail', got %q", report.Status)
		}

		// Stage 1 & 2 Completed
		if report.Stages[0].Status != StatusCompleted || report.Stages[1].Status != StatusCompleted {
			t.Errorf("expected Stages 1 & 2 to be completed")
		}
		// Stage 3 Failed with blocking error
		if report.Stages[2].Status != StatusFailed {
			t.Errorf("expected Stage 3 (Architecture) to be failed, got %s", report.Stages[2].Status)
		}
		if len(report.Stages[2].Issues) == 0 {
			t.Errorf("expected schema issues recorded for Stage 3")
		}

		// Stages 4..7 remain Pending
		for s := 3; s < 7; s++ {
			if report.Stages[s].Status != StatusPending {
				t.Errorf("expected Stage %d to be pending, got %s", s+1, report.Stages[s].Status)
			}
		}

		// Retry with working provider
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

		resumedReport, err := workRunner.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}

		if resumedReport.Status != "pass" {
			t.Errorf("expected resumed status 'pass', got %q", resumedReport.Status)
		}
	})

	// --------------------------------------------------------------------------
	// Test 3: Injeksi Cycle WBS pada Stage 6 & Retry Recovery
	// --------------------------------------------------------------------------
	t.Run("Stage6_WBS_CycleFailureAndRetry", func(t *testing.T) {
		cyclicWBSJSON := `{
			"modules": [
				{
					"id": "mod-a",
					"name": "Module A",
					"description": "Desc A",
					"priority": "high",
					"dependencies": ["mod-b"],
					"features": [{"id": "feat-a", "name": "Feat A", "description": "Desc", "acceptance_criteria": ["Crit A"]}]
				},
				{
					"id": "mod-b",
					"name": "Module B",
					"description": "Desc B",
					"priority": "high",
					"dependencies": ["mod-a"],
					"features": [{"id": "feat-b", "name": "Feat B", "description": "Desc", "acceptance_criteria": ["Crit B"]}]
				}
			]
		}`

		cyclicProv := &customMockProvider{
			name: "mock-fail-wbs-cycle",
			customResponses: map[BlueprintStage]string{
				StageWBS: cyclicWBSJSON,
			},
		}

		runner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     cyclicProv,
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

		if report.Status != "fail" {
			t.Errorf("expected report status 'fail', got %q", report.Status)
		}

		// Stages 1..5 Completed
		for s := 0; s < 5; s++ {
			if report.Stages[s].Status != StatusCompleted {
				t.Errorf("expected Stage %d to be completed, got %s", s+1, report.Stages[s].Status)
			}
		}

		// Stage 6 Failed with ERR_WBS_DEPENDENCY_CYCLE
		if report.Stages[5].Status != StatusFailed {
			t.Errorf("expected Stage 6 to be failed, got %s", report.Stages[5].Status)
		}
		foundCycleErr := false
		for _, iss := range report.Stages[5].Issues {
			if iss.Code == ErrWBSDependencyCycle {
				foundCycleErr = true
			}
		}
		if !foundCycleErr {
			t.Errorf("expected ErrWBSDependencyCycle issue on Stage 6")
		}

		// Stage 7 Pending
		if report.Stages[6].Status != StatusPending {
			t.Errorf("expected Stage 7 to be pending, got %s", report.Stages[6].Status)
		}

		// Retry with working provider
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

		resumedReport, err := workRunner.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}

		if resumedReport.Status != "pass" {
			t.Errorf("expected resumed status 'pass', got %q", resumedReport.Status)
		}
		if resumedReport.Summary.CompletedStages != 7 {
			t.Errorf("expected 7 completed stages, got %d", resumedReport.Summary.CompletedStages)
		}
	})

	// --------------------------------------------------------------------------
	// Test 4: Exhausted Retries Handling
	// --------------------------------------------------------------------------
	t.Run("ExhaustedRetries_SafeHalting", func(t *testing.T) {
		persistentFailProv := &customMockProvider{
			name:        "mock-persistent-fail",
			failAtStage: StageStateMachine,
			failErr:     errors.New("persistent network error"),
		}

		runner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     persistentFailProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create runner: %v", err)
		}

		report, err := runner.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected runner crash: %v", err)
		}

		// Simulate 3 retry attempts
		for attempt := 1; attempt <= 3; attempt++ {
			report, err = runner.Resume(context.Background(), fixturePath, report)
			if err != nil {
				t.Fatalf("unexpected crash on retry attempt %d: %v", attempt, err)
			}
			if report.Status != "fail" {
				t.Errorf("expected status 'fail' on attempt %d", attempt)
			}
		}

		if report.Stages[3].RetryCount < 3 {
			t.Errorf("expected retry count >= 3 on Stage 4, got %d", report.Stages[3].RetryCount)
		}
		// Stages 1..3 remain safe and intact
		if report.Stages[0].Status != StatusCompleted || report.Stages[1].Status != StatusCompleted || report.Stages[2].Status != StatusCompleted {
			t.Errorf("expected Stages 1..3 to remain completed after exhausted retries")
		}
	})

	// --------------------------------------------------------------------------
	// Test 5: Injeksi Error pada Stage 1 (Context Analysis) & Retry Recovery
	// --------------------------------------------------------------------------
	t.Run("Stage1_ContextAnalysis_FailureAndRetry", func(t *testing.T) {
		failProv := &customMockProvider{
			name:        "mock-fail-ctx",
			failAtStage: StageContextAnalysis,
			failErr:     errors.New("LLM timeout for context analysis"),
		}

		runnerFail, err := NewBlueprintRunner(BlueprintRunnerConfig{
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

		report, err := runnerFail.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected runner crash: %v", err)
		}

		if report.Status != "fail" {
			t.Errorf("expected report status 'fail', got %q", report.Status)
		}
		if report.Stages[0].Status != StatusFailed {
			t.Errorf("expected Stage 1 to be failed, got %s", report.Stages[0].Status)
		}
		// Downstream stages 2..7 pending
		for s := 1; s < 7; s++ {
			if report.Stages[s].Status != StatusPending {
				t.Errorf("expected Stage %d to be pending, got %s", s+1, report.Stages[s].Status)
			}
		}
		// CanPublishPartial should be false
		if report.Summary.CanPublishPartial {
			t.Errorf("expected CanPublishPartial == false when Stage 1 fails")
		}

		// Resume with working provider
		workProv := &customMockProvider{name: "mock-working"}
		runnerResume, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     workProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create resume runner: %v", err)
		}

		resumedReport, err := runnerResume.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}
		if resumedReport.Status != "pass" {
			t.Errorf("expected resumed status 'pass', got %q", resumedReport.Status)
		}
		if resumedReport.Summary.CompletedStages != 7 {
			t.Errorf("expected 7 completed stages, got %d", resumedReport.Summary.CompletedStages)
		}
	})

	// --------------------------------------------------------------------------
	// Test 6: Injeksi Error pada Stage 2 (Journey) & Retry Recovery
	// --------------------------------------------------------------------------
	t.Run("Stage2_Journey_FailureAndRetry", func(t *testing.T) {
		failProv := &customMockProvider{
			name:        "mock-fail-journey",
			failAtStage: StageJourney,
			failErr:     errors.New("LLM timeout for journey generation"),
		}

		runnerFail, err := NewBlueprintRunner(BlueprintRunnerConfig{
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

		report, err := runnerFail.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected runner crash: %v", err)
		}

		if report.Status != "fail" {
			t.Errorf("expected report status 'fail', got %q", report.Status)
		}
		// Stage 1 completed and intact
		if report.Stages[0].Status != StatusCompleted {
			t.Errorf("expected Stage 1 to remain completed, got %s", report.Stages[0].Status)
		}
		if report.Bundle.Title == "" || report.Bundle.Summary == "" {
			t.Errorf("expected Stage 1 Context in bundle to remain intact")
		}
		// Stage 2 failed
		if report.Stages[1].Status != StatusFailed {
			t.Errorf("expected Stage 2 to be failed, got %s", report.Stages[1].Status)
		}
		// Downstream stages 3..7 pending
		for s := 2; s < 7; s++ {
			if report.Stages[s].Status != StatusPending {
				t.Errorf("expected Stage %d to be pending, got %s", s+1, report.Stages[s].Status)
			}
		}

		// Resume with working provider
		workProv := &customMockProvider{name: "mock-working"}
		runnerResume, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     workProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create resume runner: %v", err)
		}

		resumedReport, err := runnerResume.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}
		if resumedReport.Status != "pass" {
			t.Errorf("expected resumed status 'pass', got %q", resumedReport.Status)
		}
		if resumedReport.Summary.CompletedStages != 7 {
			t.Errorf("expected 7 completed stages, got %d", resumedReport.Summary.CompletedStages)
		}
	})

	// --------------------------------------------------------------------------
	// Test 7: Injeksi Error pada Stage 5 (Workflow) & Retry Recovery
	// --------------------------------------------------------------------------
	t.Run("Stage5_Workflow_FailureAndRetry", func(t *testing.T) {
		failProv := &customMockProvider{
			name:        "mock-fail-workflow",
			failAtStage: StageWorkflow,
			failErr:     errors.New("LLM timeout for workflow stage"),
		}

		runnerFail, err := NewBlueprintRunner(BlueprintRunnerConfig{
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

		report, err := runnerFail.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("unexpected runner crash: %v", err)
		}

		if report.Status != "fail" {
			t.Errorf("expected report status 'fail', got %q", report.Status)
		}
		// Stages 1..4 completed and intact
		for s := 0; s < 4; s++ {
			if report.Stages[s].Status != StatusCompleted {
				t.Errorf("expected Stage %d to remain completed, got %s", s+1, report.Stages[s].Status)
			}
		}
		if len(report.Bundle.Architecture.Nodes) == 0 || len(report.Bundle.StateMachine.States) == 0 {
			t.Errorf("expected upstream architecture & state machine bundle data to remain intact")
		}
		// Stage 5 failed
		if report.Stages[4].Status != StatusFailed {
			t.Errorf("expected Stage 5 to be failed, got %s", report.Stages[4].Status)
		}
		// Stages 6..7 pending
		if report.Stages[5].Status != StatusPending || report.Stages[6].Status != StatusPending {
			t.Errorf("expected Stages 6 & 7 to be pending")
		}

		// Resume with working provider
		workProv := &customMockProvider{name: "mock-working"}
		runnerResume, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     workProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create resume runner: %v", err)
		}

		resumedReport, err := runnerResume.Resume(context.Background(), fixturePath, report)
		if err != nil {
			t.Fatalf("resume failed: %v", err)
		}
		if resumedReport.Status != "pass" {
			t.Errorf("expected resumed status 'pass', got %q", resumedReport.Status)
		}
		if resumedReport.Summary.CompletedStages != 7 {
			t.Errorf("expected 7 completed stages, got %d", resumedReport.Summary.CompletedStages)
		}
	})
}
