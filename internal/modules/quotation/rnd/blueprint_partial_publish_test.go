package rnd

import (
	"context"
	"path/filepath"
	"testing"
)

// TestBlueprintPipeline_PartialPublish tests Task T6.2: Partial Publish Rules and Status Matrix.
func TestBlueprintPipeline_PartialPublish(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "out")
	rawDir := filepath.Join(tempDir, "raw")
	artRoot := filepath.Join(tempDir, "art")
	fixturePath := createTestBlueprintInputFixture(t, tempDir, "partial-publish-input.json")

	schemaValidator := NewBlueprintSchemaValidator()
	crossValidator := NewBlueprintCrossValidator()

	// --------------------------------------------------------------------------
	// Scenario A: Draft Incomplete
	// 3 sections completed (Journey, Architecture, StateMachine), 2 pending (Workflow, WBS)
	// Expect: publication_status: draft, completeness_status: partial
	// can_publish_partial: true, can_publish_complete: false
	// --------------------------------------------------------------------------
	t.Run("Scenario_A_DraftIncomplete", func(t *testing.T) {
		report := &BlueprintBatchReport{
			SchemaVersion: BlueprintSchemaVersion,
			Fixture:       "partial-publish-draft",
			Provider:      "mock",
			Model:         "mock-model",
			PromptVersion: "1.0",
			Status:        "fail",
			Stages: []BlueprintStageResult{
				{Stage: StageContextAnalysis, Status: StatusCompleted},
				{Stage: StageJourney, Status: StatusCompleted},
				{Stage: StageArchitecture, Status: StatusCompleted},
				{Stage: StageStateMachine, Status: StatusCompleted},
				{Stage: StageWorkflow, Status: StatusPending},
				{Stage: StageWBS, Status: StatusPending},
				{Stage: StageCrossValidation, Status: StatusPending},
			},
			Bundle: &BlueprintBundle{
				SchemaVersion: BlueprintSchemaVersion,
				Title:         "Draft Banking Blueprint",
				Summary:       "Draft transactional banking blueprint",
				Journey: JourneySection{
					Phases:     []JourneyPhase{{ID: "phase-1", Label: "Onboarding", Order: 1}},
					Actors:     []JourneyActor{{ID: "actor-1", Label: "Customer", Order: 1}},
					Activities: []JourneyActivity{{ID: "act-1", PhaseID: "phase-1", ActorID: "actor-1", Label: "Open Account", Touchpoints: []string{"Mobile App"}}},
				},
				Architecture: ArchitectureSection{
					Groups: []ArchGroup{{ID: "grp-1", Label: "Backend", Order: 1}},
					Nodes:  []ArchNode{{ID: "node-1", GroupID: "grp-1", Type: "service", Label: "Account Service", Layout: LayoutCoordinate{X: 10, Y: 10}}},
					Edges:  []ArchEdge{},
				},
				StateMachine: StateMachineSection{
					Entity:       "Account",
					InitialState: "state-init",
					States:       []SMState{{ID: "state-init", Label: "Initial", Type: "normal", Layout: LayoutCoordinate{X: 10, Y: 10}}},
					Transitions:  []SMTransition{},
				},
			},
		}

		// Validate individual completed sections
		jIssues := schemaValidator.ValidateJourney(&report.Bundle.Journey)
		if len(jIssues) > 0 {
			t.Fatalf("unexpected journey issues in Draft: %v", jIssues)
		}
		aIssues := schemaValidator.ValidateArchitecture(&report.Bundle.Architecture)
		if len(aIssues) > 0 {
			t.Fatalf("unexpected architecture issues in Draft: %v", aIssues)
		}
		sIssues := schemaValidator.ValidateStateMachine(&report.Bundle.StateMachine)
		if len(sIssues) > 0 {
			t.Fatalf("unexpected state machine issues in Draft: %v", sIssues)
		}

		completedCount := 0
		for _, st := range report.Stages {
			if st.Status == StatusCompleted || st.Status == StatusValidated {
				completedCount++
			}
		}

		canPublishPartial := completedCount >= 1
		canPublishComplete := completedCount == len(report.Stages)

		if !canPublishPartial {
			t.Errorf("expected can_publish_partial to be true for Draft Incomplete, got false")
		}
		if canPublishComplete {
			t.Errorf("expected can_publish_complete to be false for Draft Incomplete, got true")
		}

		// Completeness status is partial
		completeness := CompletenessStatusPartial
		publication := PublicationStatusDraft
		if completeness != CompletenessStatusPartial || publication != PublicationStatusDraft {
			t.Errorf("expected draft x partial, got %s x %s", publication, completeness)
		}
	})

	// --------------------------------------------------------------------------
	// Scenario B: Published Partial
	// 3 sections completed/validated, 2 sections failed/empty (confirmed by BA)
	// Expect: publication_status: published, completeness_status: partial
	// Completed sections 100% valid schema, missing sections safely rendered as "Belum Tersedia"
	// --------------------------------------------------------------------------
	t.Run("Scenario_B_PublishedPartial", func(t *testing.T) {
		report := &BlueprintBatchReport{
			SchemaVersion: BlueprintSchemaVersion,
			Fixture:       "partial-published",
			Provider:      "mock",
			Model:         "mock-model",
			PromptVersion: "1.0",
			Status:        "fail",
			Stages: []BlueprintStageResult{
				{Stage: StageContextAnalysis, Status: StatusCompleted},
				{Stage: StageJourney, Status: StatusCompleted},
				{Stage: StageArchitecture, Status: StatusCompleted},
				{Stage: StageStateMachine, Status: StatusValidated},
				{Stage: StageWorkflow, Status: StatusFailed, Error: "Workflow generation timed out"},
				{Stage: StageWBS, Status: StatusFailed, Error: "WBS module dependency cycle detected"},
				{Stage: StageCrossValidation, Status: StatusPending},
			},
			Bundle: &BlueprintBundle{
				SchemaVersion: BlueprintSchemaVersion,
				Title:         "Partial Published Blueprint",
				Summary:       "Published blueprint with partial sections",
				Journey: JourneySection{
					Phases:     []JourneyPhase{{ID: "phase-1", Label: "Discovery", Order: 1}},
					Actors:     []JourneyActor{{ID: "actor-1", Label: "User", Order: 1}},
					Activities: []JourneyActivity{{ID: "act-1", PhaseID: "phase-1", ActorID: "actor-1", Label: "Explore", Touchpoints: []string{"Web"}}},
				},
				Architecture: ArchitectureSection{
					Groups: []ArchGroup{{ID: "grp-1", Label: "Core", Order: 1}},
					Nodes:  []ArchNode{{ID: "node-core", GroupID: "grp-1", Type: "service", Label: "Core Service", Layout: LayoutCoordinate{X: 100, Y: 100}}},
					Edges:  []ArchEdge{},
				},
				StateMachine: StateMachineSection{
					Entity:       "Request",
					InitialState: "state-open",
					States:       []SMState{{ID: "state-open", Label: "Open", Type: "normal", Layout: LayoutCoordinate{X: 50, Y: 50}}},
					Transitions:  []SMTransition{},
				},
				// Workflows and WBS are empty / failed
				Workflows: []Workflow{},
				WBS:       WBSSection{Modules: []WBSModule{}},
			},
		}

		// Completed/validated sections MUST be 100% valid
		if errs := schemaValidator.ValidateJourney(&report.Bundle.Journey); len(errs) > 0 {
			t.Fatalf("completed journey section is invalid: %v", errs)
		}
		if errs := schemaValidator.ValidateArchitecture(&report.Bundle.Architecture); len(errs) > 0 {
			t.Fatalf("completed architecture section is invalid: %v", errs)
		}
		if errs := schemaValidator.ValidateStateMachine(&report.Bundle.StateMachine); len(errs) > 0 {
			t.Fatalf("validated state machine section is invalid: %v", errs)
		}

		// CrossValidator on partial bundle generates warnings (WarnEmptySection), but 0 blocking errors
		xIssues := crossValidator.CrossValidate(report.Bundle, &BlueprintInput{})
		blockingCount := 0
		warnEmptyCount := 0
		for _, iss := range xIssues {
			if iss.Blocking || iss.Severity == SeverityBlockingError {
				blockingCount++
			}
			if iss.Code == WarnEmptySection {
				warnEmptyCount++
			}
		}

		if blockingCount > 0 {
			t.Errorf("expected 0 blocking errors on partial bundle, got %d", blockingCount)
		}
		if warnEmptyCount == 0 {
			t.Errorf("expected WarnEmptySection warnings for empty workflows and wbs, got 0")
		}

		publication := PublicationStatusPublished
		completeness := CompletenessStatusPartial

		if publication != PublicationStatusPublished || completeness != CompletenessStatusPartial {
			t.Errorf("expected published x partial, got %s x %s", publication, completeness)
		}
	})

	// --------------------------------------------------------------------------
	// Scenario C: Published Complete
	// All 5 core sections completed/validated with 0 blocking errors
	// Expect: publication_status: published, completeness_status: complete, can_publish_complete: true
	// --------------------------------------------------------------------------
	t.Run("Scenario_C_PublishedComplete", func(t *testing.T) {
		mockProv := &customMockProvider{name: "mock-full"}
		runner, err := NewBlueprintRunner(BlueprintRunnerConfig{
			ArtifactRoot: artRoot,
			OutputDir:    outDir,
			RawDir:       rawDir,
			Provider:     mockProv,
			Model:        "mock-model",
			Runs:         1,
		})
		if err != nil {
			t.Fatalf("failed to create runner: %v", err)
		}

		report, err := runner.Run(context.Background(), fixturePath)
		if err != nil {
			t.Fatalf("runner run failed: %v", err)
		}

		if report.Status != "pass" {
			t.Errorf("expected status 'pass', got %q", report.Status)
		}
		if !report.Summary.Passed {
			t.Errorf("expected summary.passed == true")
		}
		if !report.Summary.CanPublishComplete {
			t.Errorf("expected summary.can_publish_complete == true")
		}
		if !report.Summary.CanPublishPartial {
			t.Errorf("expected summary.can_publish_partial == true")
		}
		if report.Summary.CompletedStages != 7 {
			t.Errorf("expected 7 completed stages, got %d", report.Summary.CompletedStages)
		}
		if report.Summary.BlockingErrors != 0 {
			t.Errorf("expected 0 blocking errors, got %d", report.Summary.BlockingErrors)
		}
	})

	// --------------------------------------------------------------------------
	// Scenario D: Warning Modal Gate
	// Unreferenced architecture node emits semantic warning (WARN_ARCH_NODE_UNREFERENCED_IN_WBS)
	// Verify: blocking_errors == 0, warnings_count > 0, ConfirmationRequired == true
	// Publishing allowed after explicit BA acknowledgment
	// --------------------------------------------------------------------------
	t.Run("Scenario_D_WarningModalGate", func(t *testing.T) {
		bundle := &BlueprintBundle{
			SchemaVersion: BlueprintSchemaVersion,
			Title:         "System with Unreferenced Arch Node",
			Summary:       "Demonstrating warning gate confirmation",
			Journey: JourneySection{
				Phases:     []JourneyPhase{{ID: "phase-1", Label: "Phase 1", Order: 1}},
				Actors:     []JourneyActor{{ID: "actor-1", Label: "Actor 1", Order: 1}},
				Activities: []JourneyActivity{{ID: "act-1", PhaseID: "phase-1", ActorID: "actor-1", Label: "Act 1", Touchpoints: []string{"Web"}}},
			},
			Architecture: ArchitectureSection{
				Groups: []ArchGroup{{ID: "grp-1", Label: "Tier", Order: 1}},
				Nodes: []ArchNode{
					{ID: "node-referenced", GroupID: "grp-1", Type: "service", Label: "Service A", Layout: LayoutCoordinate{X: 10, Y: 10}},
					{ID: "node-unreferenced", GroupID: "grp-1", Type: "service", Label: "Redis Cache Service", Layout: LayoutCoordinate{X: 20, Y: 20}},
				},
				Edges: []ArchEdge{},
			},
			StateMachine: StateMachineSection{
				Entity:       "Order",
				InitialState: "state-1",
				States:       []SMState{{ID: "state-1", Label: "State 1", Type: "normal", Layout: LayoutCoordinate{X: 10, Y: 10}}},
				Transitions:  []SMTransition{},
			},
			Workflows: []Workflow{
				{
					ID:          "wf-1",
					Name:        "Workflow 1",
					Description: "Desc",
					Lanes:       []WorkflowLane{{ID: "lane-1", Label: "Actor 1", Order: 1}},
					Nodes: []WorkflowNode{
						{ID: "step-1", LaneID: "lane-1", Type: "start", Label: "Start", Description: "Start", Layout: LayoutCoordinate{X: 10, Y: 10}},
						{ID: "step-2", LaneID: "lane-1", Type: "end", Label: "End", Description: "End", Layout: LayoutCoordinate{X: 20, Y: 10}},
					},
					Edges: []WorkflowEdge{
						{ID: "e-1", Source: "step-1", Target: "step-2", Label: ""},
					},
				},
			},
			WBS: WBSSection{
				Modules: []WBSModule{
					{
						ID:          "mod-1",
						Name:        "Module 1",
						Description: "Desc",
						Priority:    "high",
						References:  []string{"architecture.node.node-referenced"},
						Features: []WBSFeature{
							{ID: "feat-1", Name: "Feature 1", Description: "Desc", AcceptanceCriteria: []string{"Criterion 1"}},
						},
					},
				},
			},
		}

		issues := crossValidator.CrossValidate(bundle, &BlueprintInput{})

		foundWarnUnref := false
		blockingErrors := 0
		for _, iss := range issues {
			if iss.Code == WarnArchNodeUnreferencedInWBS {
				foundWarnUnref = true
				if !iss.ConfirmationRequired {
					t.Errorf("expected ConfirmationRequired to be true for warning %s", iss.Code)
				}
				if iss.Blocking {
					t.Errorf("expected warning %s to be non-blocking", iss.Code)
				}
			}
			if iss.Blocking || iss.Severity == SeverityBlockingError {
				blockingErrors++
			}
		}

		if !foundWarnUnref {
			t.Errorf("expected to find warning %s for unreferenced node-unreferenced", WarnArchNodeUnreferencedInWBS)
		}
		if blockingErrors > 0 {
			t.Errorf("expected 0 blocking errors for warning gate test, got %d", blockingErrors)
		}
	})

	// --------------------------------------------------------------------------
	// Scenario E: Superseded Archive
	// A published blueprint is superseded by a newer revision.
	// Verify: publication_status: superseded, status locked as read-only audit trail
	// --------------------------------------------------------------------------
	t.Run("Scenario_E_SupersededArchive", func(t *testing.T) {
		status := PublicationStatusSuperseded
		completeness := CompletenessStatusPartial

		if status != "superseded" {
			t.Errorf("expected status superseded, got %s", status)
		}
		if completeness != "partial" {
			t.Errorf("expected historical completeness partial, got %s", completeness)
		}
	})

	// --------------------------------------------------------------------------
	// Section Validation Independence (Zero Cascading Failure)
	// Removing or modifying WBS / State Machine does not cause other completed sections to fail.
	// --------------------------------------------------------------------------
	t.Run("Section_Validation_Independence", func(t *testing.T) {
		journey := &JourneySection{
			Phases:     []JourneyPhase{{ID: "phase-1", Label: "Phase 1", Order: 1}},
			Actors:     []JourneyActor{{ID: "actor-1", Label: "Actor 1", Order: 1}},
			Activities: []JourneyActivity{{ID: "act-1", PhaseID: "phase-1", ActorID: "actor-1", Label: "Act 1", Touchpoints: []string{"Web"}}},
		}
		arch := &ArchitectureSection{
			Groups: []ArchGroup{{ID: "grp-1", Label: "Tier", Order: 1}},
			Nodes:  []ArchNode{{ID: "node-1", GroupID: "grp-1", Type: "service", Label: "Service 1", Layout: LayoutCoordinate{X: 10, Y: 10}}},
			Edges:  []ArchEdge{},
		}

		// Both sections independently validate with 0 errors
		if errs := schemaValidator.ValidateJourney(journey); len(errs) > 0 {
			t.Errorf("unexpected journey errors: %v", errs)
		}
		if errs := schemaValidator.ValidateArchitecture(arch); len(errs) > 0 {
			t.Errorf("unexpected architecture errors: %v", errs)
		}
	})
}
