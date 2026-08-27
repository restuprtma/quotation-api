package rnd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func createValidCrossReferencedBundle() (BlueprintBundle, BlueprintInput) {
	bundle := BlueprintBundle{
		SchemaVersion: "1.0",
		Title:         "Bengkel Management System",
		Summary:       "Integrated workshop management system.",
		Journey: JourneySection{
			Phases: []JourneyPhase{
				{ID: "phase-booking", Label: "Booking Phase", Order: 1},
			},
			Actors: []JourneyActor{
				{ID: "actor-customer", Label: "Customer", Order: 1},
			},
			Activities: []JourneyActivity{
				{
					ID:          "act-create-booking",
					PhaseID:     "phase-booking",
					ActorID:     "actor-customer",
					Label:       "Create Booking",
					Description: "Customer reserves service slot",
					Touchpoints: []string{"Mobile App"},
					References: []string{
						"architecture.node.node-booking-api",
						"state.state-booked",
						"workflow.wf-booking.step.wnode-submit",
						"wbs.module.mod-booking",
					},
				},
			},
		},
		Architecture: ArchitectureSection{
			Groups: []ArchGroup{
				{ID: "grp-backend", Label: "Backend Tier", Order: 1},
			},
			Nodes: []ArchNode{
				{
					ID:          "node-booking-api",
					GroupID:     "grp-backend",
					Type:        "service",
					Label:       "Booking API",
					Description: "Handles bookings",
					Layout:      ArchLayout{X: 100, Y: 100},
				},
				{
					ID:          "node-db",
					GroupID:     "grp-backend",
					Type:        "database",
					Label:       "Main DB",
					Description: "PostgreSQL Database",
					Layout:      ArchLayout{X: 100, Y: 200},
				},
				{
					ID:          "node-mobile-app",
					GroupID:     "grp-backend",
					Type:        "channel",
					Label:       "Mobile App",
					Description: "Flutter mobile client",
					Layout:      ArchLayout{X: 50, Y: 50},
				},
			},
			Edges: []ArchEdge{
				{
					ID:     "edge-api-db",
					Source: "node-booking-api",
					Target: "node-db",
					Label:  "SQL Queries",
					Type:   "sync",
				},
			},
		},
		StateMachine: StateMachineSection{
			Entity:       "ServiceOrder",
			InitialState: "state-booked",
			States: []SMState{
				{ID: "state-booked", Label: "Booked", Type: "normal", Layout: ArchLayout{X: 50, Y: 50}},
				{ID: "state-completed", Label: "Completed", Type: "terminal", Layout: ArchLayout{X: 250, Y: 50}},
			},
			Transitions: []SMTransition{
				{
					ID:      "trans-finish",
					Source:  "state-booked",
					Target:  "state-completed",
					Event:   "FINISH_WORK",
					Actor:   "actor-mechanic",
					Guard:   "inspections_passed",
					Actions: []string{"generate_invoice"},
				},
			},
		},
		Workflows: []Workflow{
			{
				ID:          "wf-booking",
				Name:        "Booking Workflow",
				Description: "End-to-end booking flow",
				Lanes: []WorkflowLane{
					{ID: "lane-customer", Label: "Customer", Order: 1},
				},
				Nodes: []WorkflowNode{
					{
						ID:          "wnode-submit",
						LaneID:      "lane-customer",
						Type:        "action",
						Label:       "Submit Booking",
						Description: "User submits booking",
						Layout:      ArchLayout{X: 10, Y: 10},
					},
					{
						ID:          "wnode-check",
						LaneID:      "lane-customer",
						Type:        "decision",
						Label:       "Slot Available?",
						Description: "Check if slot is available",
						Layout:      ArchLayout{X: 60, Y: 10},
					},
					{
						ID:          "wnode-confirm",
						LaneID:      "lane-customer",
						Type:        "end",
						Label:       "Confirm",
						Description: "Booking confirmed",
						Layout:      ArchLayout{X: 110, Y: 10},
					},
					{
						ID:          "wnode-reject",
						LaneID:      "lane-customer",
						Type:        "end",
						Label:       "Reject",
						Description: "Booking rejected",
						Layout:      ArchLayout{X: 110, Y: 60},
					},
				},
				Edges: []WorkflowEdge{
					{ID: "wedge-1", Source: "wnode-submit", Target: "wnode-check", Label: "Proceed"},
					{ID: "wedge-2", Source: "wnode-check", Target: "wnode-confirm", Label: "Yes"},
					{ID: "wedge-3", Source: "wnode-check", Target: "wnode-reject", Label: "No"},
				},
			},
		},
		WBS: WBSSection{
			Modules: []WBSModule{
				{
					ID:           "mod-booking",
					Name:         "Booking Module",
					Description:  "Booking management",
					Priority:     "high",
					Dependencies: []string{},
					References: []string{
						"architecture.node.node-booking-api",
						"architecture.node.node-db",
					},
					Features: []WBSFeature{
						{
							ID:                 "feat-slot-picker",
							Name:               "Slot Picker",
							Description:        "Select available slot",
							AcceptanceCriteria: []string{"Slot reserved"},
						},
					},
				},
			},
		},
		Assumptions: []string{"Operating 6 days a week"},
		OpenQuestions: []OpenQuestion{
			{
				ID:            "oq-payment",
				Question:      "Down payment required?",
				TargetSection: "workflow",
				Impact:        "Affects payment flow",
				Status:        "answered",
			},
		},
		CrossReferences: []CrossReference{
			{
				ID:           "xref-1",
				SourceRef:    "journey.activity.act-create-booking",
				TargetRef:    "workflow.wf-booking.step.wnode-submit",
				RelationType: "implements",
				Description:  "Booking creation mapped to submission workflow step",
			},
			{
				ID:           "xref-2",
				SourceRef:    "architecture.node.node-booking-api",
				TargetRef:    "wbs.module.mod-booking",
				RelationType: "implements",
				Description:  "Booking API implemented by Booking WBS module",
			},
		},
	}

	input := BlueprintInput{
		SourceQuotationRevisionID: "rev-001",
		Actors:                    []string{"Customer"},
		Channels:                  []string{"Mobile App"},
		BusinessPhases:            []string{"Booking Phase"},
	}

	return bundle, input
}

func findIssuesByCode(issues []ValidationIssue, code string) []ValidationIssue {
	var result []ValidationIssue
	for _, iss := range issues {
		if iss.Code == code {
			result = append(result, iss)
		}
	}
	return result
}

func TestBlueprintCrossValidator_ValidBundle(t *testing.T) {
	bundle, input := createValidCrossReferencedBundle()
	validator := NewBlueprintCrossValidator()

	issues := validator.CrossValidate(&bundle, &input)
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues for fully valid cross-referenced bundle, got %d: %+v", len(issues), issues)
	}
}

func TestBlueprintCrossValidator_DanglingReference(t *testing.T) {
	validator := NewBlueprintCrossValidator()

	t.Run("dangling cross reference source", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.CrossReferences = append(bundle.CrossReferences, CrossReference{
			ID:           "xref-dangling-src",
			SourceRef:    "journey.activity.act-non-existent",
			TargetRef:    "workflow.wf-booking.step.wnode-submit",
			RelationType: "implements",
		})

		issues := validator.CrossValidate(&bundle, &input)
		dangling := findIssuesByCode(issues, ErrDanglingReference)
		if len(dangling) == 0 {
			t.Fatalf("expected ERR_DANGLING_REFERENCE for non-existent source ref, got none")
		}
		if dangling[0].Severity != SeverityBlockingError || !dangling[0].Blocking {
			t.Errorf("expected SeverityBlockingError and Blocking=true, got %s, %v", dangling[0].Severity, dangling[0].Blocking)
		}
	})

	t.Run("dangling cross reference target", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.CrossReferences = append(bundle.CrossReferences, CrossReference{
			ID:           "xref-dangling-tgt",
			SourceRef:    "journey.activity.act-create-booking",
			TargetRef:    "architecture.node.node-ghost",
			RelationType: "implements",
		})

		issues := validator.CrossValidate(&bundle, &input)
		dangling := findIssuesByCode(issues, ErrDanglingReference)
		if len(dangling) == 0 {
			t.Fatalf("expected ERR_DANGLING_REFERENCE for non-existent target ref, got none")
		}
	})

	t.Run("dangling journey reference", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Journey.Activities[0].References = append(bundle.Journey.Activities[0].References, "state.state-unknown")

		issues := validator.CrossValidate(&bundle, &input)
		dangling := findIssuesByCode(issues, ErrDanglingReference)
		if len(dangling) == 0 {
			t.Fatalf("expected ERR_DANGLING_REFERENCE for unresolvable journey reference, got none")
		}
	})

	t.Run("dangling wbs reference", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.WBS.Modules[0].References = append(bundle.WBS.Modules[0].References, "architecture.node.node-missing")

		issues := validator.CrossValidate(&bundle, &input)
		dangling := findIssuesByCode(issues, ErrDanglingReference)
		if len(dangling) == 0 {
			t.Fatalf("expected ERR_DANGLING_REFERENCE for unresolvable WBS reference, got none")
		}
	})
}

func TestBlueprintCrossValidator_UnreferencedArchNode(t *testing.T) {
	validator := NewBlueprintCrossValidator()

	t.Run("unreferenced service node emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		// Add service node not referenced in WBS
		bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, ArchNode{
			ID:          "node-payment-service",
			GroupID:     "grp-backend",
			Type:        "service",
			Label:       "Payment Service",
			Description: "Handles payments",
			Layout:      ArchLayout{X: 200, Y: 100},
		})

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnArchNodeUnreferencedInWBS)
		if len(warnings) == 0 {
			t.Fatalf("expected WARN_ARCH_NODE_UNREFERENCED_IN_WBS for unreferenced service node, got none")
		}
		if warnings[0].Severity != SeverityWarning || warnings[0].Blocking {
			t.Errorf("expected SeverityWarning and Blocking=false, got %s, %v", warnings[0].Severity, warnings[0].Blocking)
		}
		if !warnings[0].ConfirmationRequired {
			t.Errorf("expected ConfirmationRequired=true")
		}
		if warnings[0].TargetID != "node-payment-service" {
			t.Errorf("expected TargetID 'node-payment-service', got '%s'", warnings[0].TargetID)
		}
	})

	t.Run("unreferenced database node emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, ArchNode{
			ID:          "node-analytics-db",
			GroupID:     "grp-backend",
			Type:        "database",
			Label:       "Analytics DB",
			Description: "ClickHouse OLAP",
			Layout:      ArchLayout{X: 200, Y: 200},
		})

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnArchNodeUnreferencedInWBS)
		if len(warnings) == 0 {
			t.Fatalf("expected WARN_ARCH_NODE_UNREFERENCED_IN_WBS for unreferenced database node, got none")
		}
	})

	t.Run("channel or cache node unreferenced in WBS does not emit warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, ArchNode{
			ID:          "node-redis-cache",
			GroupID:     "grp-backend",
			Type:        "cache",
			Label:       "Redis Cache",
			Description: "In-memory cache",
			Layout:      ArchLayout{X: 150, Y: 150},
		})

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnArchNodeUnreferencedInWBS)
		for _, w := range warnings {
			if w.TargetID == "node-redis-cache" {
				t.Fatalf("did not expect WARN_ARCH_NODE_UNREFERENCED_IN_WBS for cache node type")
			}
		}
	})

	t.Run("arch node referenced via cross_references is satisfied", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Architecture.Nodes = append(bundle.Architecture.Nodes, ArchNode{
			ID:          "node-auth-service",
			GroupID:     "grp-backend",
			Type:        "service",
			Label:       "Auth Service",
			Description: "OAuth2 Provider",
			Layout:      ArchLayout{X: 200, Y: 150},
		})
		// Link via cross_references to WBS module
		bundle.CrossReferences = append(bundle.CrossReferences, CrossReference{
			ID:           "xref-auth",
			SourceRef:    "architecture.node.node-auth-service",
			TargetRef:    "wbs.module.mod-booking",
			RelationType: "implements",
		})

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnArchNodeUnreferencedInWBS)
		for _, w := range warnings {
			if w.TargetID == "node-auth-service" {
				t.Fatalf("expected node-auth-service to be satisfied via cross_references, but got warning")
			}
		}
	})
}

func TestBlueprintCrossValidator_UnlinkedJourneyActivity(t *testing.T) {
	validator := NewBlueprintCrossValidator()

	t.Run("unlinked activity emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		// Add activity without workflow/state references or cross_references
		bundle.Journey.Activities = append(bundle.Journey.Activities, JourneyActivity{
			ID:          "act-manual-inspection",
			PhaseID:     "phase-booking",
			ActorID:     "actor-customer",
			Label:       "Manual Visual Check",
			Description: "Physical touchpoint with vehicle",
			Touchpoints: []string{"Physical Workshop"},
			References:  []string{"architecture.node.node-booking-api"}, // not workflow or state
		})

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnJourneyActivityUnlinked)
		if len(warnings) == 0 {
			t.Fatalf("expected WARN_JOURNEY_ACTIVITY_UNLINKED for unlinked activity, got none")
		}
		if warnings[0].TargetID != "act-manual-inspection" {
			t.Errorf("expected TargetID 'act-manual-inspection', got '%s'", warnings[0].TargetID)
		}
		if warnings[0].Severity != SeverityWarning || warnings[0].Blocking {
			t.Errorf("expected SeverityWarning and Blocking=false, got %s, %v", warnings[0].Severity, warnings[0].Blocking)
		}
	})

	t.Run("activity linked via cross_references to workflow step is satisfied", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Journey.Activities = append(bundle.Journey.Activities, JourneyActivity{
			ID:          "act-verify-code",
			PhaseID:     "phase-booking",
			ActorID:     "actor-customer",
			Label:       "Verify Code",
			Description: "Enter OTP code",
			Touchpoints: []string{"Mobile App"},
			References:  []string{},
		})
		bundle.CrossReferences = append(bundle.CrossReferences, CrossReference{
			ID:           "xref-otp",
			SourceRef:    "journey.activity.act-verify-code",
			TargetRef:    "workflow.wf-booking.step.wnode-check",
			RelationType: "implements",
		})

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnJourneyActivityUnlinked)
		for _, w := range warnings {
			if w.TargetID == "act-verify-code" {
				t.Fatalf("expected act-verify-code to be satisfied via cross_references, but got warning")
			}
		}
	})
}

func TestBlueprintCrossValidator_TerminologyMismatch(t *testing.T) {
	validator := NewBlueprintCrossValidator()

	t.Run("missing input actor emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		input.Actors = append(input.Actors, "Fleet Manager")

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnTerminologyMismatch)
		if len(warnings) == 0 {
			t.Fatalf("expected WARN_TERMINOLOGY_MISMATCH for missing input actor, got none")
		}

		found := false
		for _, w := range warnings {
			if w.TargetID == "Fleet Manager" {
				found = true
				if w.Severity != SeverityWarning || w.Blocking {
					t.Errorf("expected SeverityWarning and Blocking=false, got %s, %v", w.Severity, w.Blocking)
				}
			}
		}
		if !found {
			t.Fatalf("expected warning for 'Fleet Manager', got %+v", warnings)
		}
	})

	t.Run("missing input channel emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		input.Channels = append(input.Channels, "Self-Service Kiosk")

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnTerminologyMismatch)
		if len(warnings) == 0 {
			t.Fatalf("expected WARN_TERMINOLOGY_MISMATCH for missing input channel, got none")
		}

		found := false
		for _, w := range warnings {
			if w.TargetID == "Self-Service Kiosk" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected warning for 'Self-Service Kiosk', got %+v", warnings)
		}
	})

	t.Run("case insensitive and normalized matching passes without warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		// "customer" lowercase matches "actor-customer" / "Customer"
		input.Actors = []string{"customer"}
		// "mobile-app" normalized matches "Mobile App"
		input.Channels = []string{"mobile-app"}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnTerminologyMismatch)
		if len(warnings) != 0 {
			t.Fatalf("expected 0 terminology mismatch warnings for normalized names, got %d: %+v", len(warnings), warnings)
		}
	})

	t.Run("nil input does not panic or emit terminology warning", func(t *testing.T) {
		bundle, _ := createValidCrossReferencedBundle()
		issues := validator.CrossValidate(&bundle, nil)
		warnings := findIssuesByCode(issues, WarnTerminologyMismatch)
		if len(warnings) != 0 {
			t.Fatalf("expected 0 terminology mismatch warnings when input is nil, got %d", len(warnings))
		}
	})
}

func TestBlueprintCrossValidator_UnresolvedOpenQuestion(t *testing.T) {
	validator := NewBlueprintCrossValidator()

	t.Run("open status emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.OpenQuestions = []OpenQuestion{
			{
				ID:            "oq-payment-gateway",
				Question:      "Which payment gateway provider will be used?",
				TargetSection: "architecture",
				Impact:        "Affects third-party integration scope",
				Status:        "open",
			},
		}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnOpenQuestionUnresolved)
		if len(warnings) == 0 {
			t.Fatalf("expected WARN_OPEN_QUESTION_UNRESOLVED for open question, got none")
		}
		if warnings[0].TargetID != "oq-payment-gateway" {
			t.Errorf("expected TargetID 'oq-payment-gateway', got '%s'", warnings[0].TargetID)
		}
		if warnings[0].Severity != SeverityWarning || warnings[0].Blocking {
			t.Errorf("expected SeverityWarning and Blocking=false, got %s, %v", warnings[0].Severity, warnings[0].Blocking)
		}
	})

	t.Run("answered or deferred question does not emit warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.OpenQuestions = []OpenQuestion{
			{
				ID:            "oq-1",
				Question:      "Question 1",
				TargetSection: "wbs",
				Status:        "answered",
			},
			{
				ID:            "oq-2",
				Question:      "Question 2",
				TargetSection: "journey",
				Status:        "deferred",
			},
		}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnOpenQuestionUnresolved)
		if len(warnings) != 0 {
			t.Fatalf("expected 0 open question warnings for answered/deferred questions, got %d", len(warnings))
		}
	})
}

func TestBlueprintCrossValidator_EmptySection(t *testing.T) {
	validator := NewBlueprintCrossValidator()

	t.Run("empty journey section emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Journey = JourneySection{}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnEmptySection)
		found := false
		for _, w := range warnings {
			if w.Section == "journey" {
				found = true
				if w.Severity != SeverityWarning || w.Blocking {
					t.Errorf("expected SeverityWarning and Blocking=false, got %s, %v", w.Severity, w.Blocking)
				}
			}
		}
		if !found {
			t.Fatalf("expected WARN_EMPTY_SECTION for empty journey section, got %+v", warnings)
		}
	})

	t.Run("empty architecture section emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Architecture = ArchitectureSection{}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnEmptySection)
		found := false
		for _, w := range warnings {
			if w.Section == "architecture" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected WARN_EMPTY_SECTION for empty architecture section, got %+v", warnings)
		}
	})

	t.Run("empty state machine section emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.StateMachine = StateMachineSection{}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnEmptySection)
		found := false
		for _, w := range warnings {
			if w.Section == "state_machine" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected WARN_EMPTY_SECTION for empty state_machine section, got %+v", warnings)
		}
	})

	t.Run("empty workflows section emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.Workflows = nil

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnEmptySection)
		found := false
		for _, w := range warnings {
			if w.Section == "workflows" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected WARN_EMPTY_SECTION for empty workflows section, got %+v", warnings)
		}
	})

	t.Run("empty wbs section emits warning", func(t *testing.T) {
		bundle, input := createValidCrossReferencedBundle()
		bundle.WBS = WBSSection{}

		issues := validator.CrossValidate(&bundle, &input)
		warnings := findIssuesByCode(issues, WarnEmptySection)
		found := false
		for _, w := range warnings {
			if w.Section == "wbs" {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected WARN_EMPTY_SECTION for empty wbs section, got %+v", warnings)
		}
	})

	t.Run("nil bundle emits empty section warnings and does not panic", func(t *testing.T) {
		issues := validator.CrossValidate(nil, nil)
		warnings := findIssuesByCode(issues, WarnEmptySection)
		if len(warnings) != 5 {
			t.Fatalf("expected 5 empty section warnings for nil bundle, got %d: %+v", len(warnings), warnings)
		}
	})
}

func getResultsDirectory(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("testdata", "blueprint-skill", "results")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("cannot resolve testdata/blueprint-skill/results: %v", err)
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		t.Fatalf("testdata/blueprint-skill/results not found at %s — run from package directory", abs)
	}
	return abs
}

func TestBlueprintCrossValidator_RealRepresentativeOutputs(t *testing.T) {
	resultsDir := getResultsDirectory(t)
	fixturesDir := getFixturesDirectory(t)
	schemaValidator := NewBlueprintSchemaValidator()
	crossValidator := NewBlueprintCrossValidator()

	testCases := []struct {
		name        string
		bundleFile  string
		fixtureFile string
	}{
		{
			name:        "RepresentativeComplex_01",
			bundleFile:  "representative-complex-01.json",
			fixtureFile: "01-public-service-queue.json",
		},
		{
			name:        "RepresentativeSimple_10",
			bundleFile:  "representative-simple-10.json",
			fixtureFile: "10-small-simple-system.json",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			bundlePath := filepath.Join(resultsDir, tc.bundleFile)
			bundleData, err := os.ReadFile(bundlePath)
			if err != nil {
				t.Fatalf("failed to read bundle file %s: %v", tc.bundleFile, err)
			}

			var bundle BlueprintBundle
			if err := json.Unmarshal(bundleData, &bundle); err != nil {
				t.Fatalf("failed to unmarshal bundle %s: %v", tc.bundleFile, err)
			}

			fixturePath := filepath.Join(fixturesDir, tc.fixtureFile)
			fixtureData, err := os.ReadFile(fixturePath)
			if err != nil {
				t.Fatalf("failed to read fixture file %s: %v", tc.fixtureFile, err)
			}

			var input BlueprintInput
			if err := json.Unmarshal(fixtureData, &input); err != nil {
				t.Fatalf("failed to unmarshal fixture %s: %v", tc.fixtureFile, err)
			}

			// 1. Schema Validation
			schemaIssues := schemaValidator.ValidateBundle(&bundle)
			for _, iss := range schemaIssues {
				if iss.Blocking || iss.Severity == SeverityBlockingError {
					t.Errorf("unexpected schema blocking error in %s: code=%s, path=%s, msg=%s", tc.bundleFile, iss.Code, iss.Path, iss.Message)
				}
			}

			// 2. Cross Validation
			crossIssues := crossValidator.CrossValidate(&bundle, &input)
			for _, iss := range crossIssues {
				if iss.Blocking || iss.Severity == SeverityBlockingError {
					t.Errorf("unexpected cross-validation blocking error in %s: code=%s, path=%s, msg=%s", tc.bundleFile, iss.Code, iss.Path, iss.Message)
				}
				if iss.Remediation == "" {
					t.Errorf("warning issue missing remediation guide in %s: code=%s, target=%s", tc.bundleFile, iss.Code, iss.TargetID)
				}
			}

			t.Logf("[%s] Schema Issues: %d, Cross-Validation Warnings: %d (Zero Blocking Errors)", tc.name, len(schemaIssues), len(crossIssues))
		})
	}
}
