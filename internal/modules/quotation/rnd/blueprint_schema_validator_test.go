package rnd

import (
	"math"
	"testing"
)

// helper to create a fully valid BlueprintBundle
func createValidBundle() BlueprintBundle {
	return BlueprintBundle{
		SchemaVersion: "1.0",
		Title:         "Bengkel Management System",
		Summary:       "Integrated workshop management system.",
		Journey: JourneySection{
			Phases: []JourneyPhase{
				{ID: "phase-booking", Label: "Booking", Order: 1},
				{ID: "phase-service", Label: "Servicing", Order: 2},
			},
			Actors: []JourneyActor{
				{ID: "actor-customer", Label: "Customer", Order: 1},
				{ID: "actor-mechanic", Label: "Mechanic", Order: 2},
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
				{ID: "state-in-progress", Label: "In Progress", Type: "normal", Layout: ArchLayout{X: 150, Y: 50}},
				{ID: "state-completed", Label: "Completed", Type: "terminal", Layout: ArchLayout{X: 250, Y: 50}},
			},
			Transitions: []SMTransition{
				{
					ID:      "trans-start",
					Source:  "state-booked",
					Target:  "state-in-progress",
					Event:   "START_WORK",
					Actor:   "actor-mechanic",
					Guard:   "mechanic_assigned",
					Actions: []string{"log_start"},
				},
				{
					ID:      "trans-finish",
					Source:  "state-in-progress",
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
					ID:           "mod-auth",
					Name:         "Auth Module",
					Description:  "Authentication module",
					Priority:     "high",
					Dependencies: []string{},
					References:   []string{"architecture.node.node-booking-api"},
					Features: []WBSFeature{
						{
							ID:                 "feat-login",
							Name:               "User Login",
							Description:        "Login with phone number",
							AcceptanceCriteria: []string{"OTP delivered"},
						},
					},
				},
				{
					ID:           "mod-booking",
					Name:         "Booking Module",
					Description:  "Booking management",
					Priority:     "high",
					Dependencies: []string{"mod-auth"},
					References:   []string{"architecture.node.node-booking-api"},
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
				Status:        "open",
			},
		},
		CrossReferences: []CrossReference{
			{
				ID:           "xref-1",
				SourceRef:    "journey.activity.act-create-booking",
				TargetRef:    "workflow.wf-booking.step.wnode-submit",
				RelationType: "implements",
				Description:  "Booking activity implements submit step",
			},
		},
	}
}

func hasErrorCode(issues []ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestBlueprintSchemaValidator_ValidBundle(t *testing.T) {
	validator := NewBlueprintSchemaValidator()
	bundle := createValidBundle()

	issues := validator.ValidateBundle(&bundle)
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues for valid bundle, got %d: %+v", len(issues), issues)
	}
}

func TestBlueprintSchemaValidator_NilSections(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	if issues := validator.ValidateJourney(nil); len(issues) != 0 {
		t.Errorf("expected 0 issues for nil Journey, got %d", len(issues))
	}
	if issues := validator.ValidateArchitecture(nil); len(issues) != 0 {
		t.Errorf("expected 0 issues for nil Architecture, got %d", len(issues))
	}
	if issues := validator.ValidateStateMachine(nil); len(issues) != 0 {
		t.Errorf("expected 0 issues for nil StateMachine, got %d", len(issues))
	}
	if issues := validator.ValidateWorkflows(nil); len(issues) != 0 {
		t.Errorf("expected 0 issues for nil Workflows, got %d", len(issues))
	}
	if issues := validator.ValidateWBS(nil); len(issues) != 0 {
		t.Errorf("expected 0 issues for nil WBS, got %d", len(issues))
	}

	bundleNilIssues := validator.ValidateBundle(nil)
	if !hasErrorCode(bundleNilIssues, ErrUnsupportedSchemaVersion) {
		t.Errorf("expected ErrUnsupportedSchemaVersion for nil Bundle")
	}
}

func TestBlueprintSchemaValidator_ErrUnsupportedSchemaVersion(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// Invalid schema version
	bundle := createValidBundle()
	bundle.SchemaVersion = "2.0"
	issues := validator.ValidateBundle(&bundle)
	if !hasErrorCode(issues, ErrUnsupportedSchemaVersion) {
		t.Errorf("expected ErrUnsupportedSchemaVersion when version is 2.0")
	}

	// Empty schema version
	bundleEmpty := createValidBundle()
	bundleEmpty.SchemaVersion = ""
	issuesEmpty := validator.ValidateBundle(&bundleEmpty)
	if !hasErrorCode(issuesEmpty, ErrUnsupportedSchemaVersion) {
		t.Errorf("expected ErrUnsupportedSchemaVersion when version is empty")
	}
}

func TestBlueprintSchemaValidator_ErrMissingID(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// 1. Missing phase ID
	journey := JourneySection{
		Phases:     []JourneyPhase{{ID: "   ", Label: "Phase 1", Order: 1}},
		Actors:     []JourneyActor{{ID: "actor-1", Label: "Actor 1", Order: 1}},
		Activities: []JourneyActivity{},
	}
	issues := validator.ValidateJourney(&journey)
	if !hasErrorCode(issues, ErrMissingID) {
		t.Errorf("expected ErrMissingID for empty phase ID")
	}

	// 2. Missing architecture node ID
	arch := ArchitectureSection{
		Groups: []ArchGroup{{ID: "grp-1", Label: "Group 1", Order: 1}},
		Nodes: []ArchNode{
			{ID: "", GroupID: "grp-1", Type: "service", Label: "Service 1", Layout: ArchLayout{X: 0, Y: 0}},
		},
		Edges: []ArchEdge{},
	}
	issues = validator.ValidateArchitecture(&arch)
	if !hasErrorCode(issues, ErrMissingID) {
		t.Errorf("expected ErrMissingID for empty node ID")
	}

	// 3. Missing WBS module ID
	wbs := WBSSection{
		Modules: []WBSModule{
			{ID: "", Name: "Module 1", Dependencies: []string{}},
		},
	}
	issues = validator.ValidateWBS(&wbs)
	if !hasErrorCode(issues, ErrMissingID) {
		t.Errorf("expected ErrMissingID for empty WBS module ID")
	}

	// 4. Missing OpenQuestion ID in bundle
	bundle := createValidBundle()
	bundle.OpenQuestions = []OpenQuestion{
		{ID: "", Question: "Unidentified question", TargetSection: "general", Status: "open"},
	}
	bundleIssues := validator.ValidateBundle(&bundle)
	if !hasErrorCode(bundleIssues, ErrMissingID) {
		t.Errorf("expected ErrMissingID for empty OpenQuestion ID")
	}

	// 5. Missing CrossReference ID in bundle
	bundle2 := createValidBundle()
	bundle2.CrossReferences = []CrossReference{
		{ID: " ", SourceRef: "journey.phase.phase-booking", TargetRef: "architecture.node.node-booking-api", RelationType: "implements"},
	}
	bundleIssues2 := validator.ValidateBundle(&bundle2)
	if !hasErrorCode(bundleIssues2, ErrMissingID) {
		t.Errorf("expected ErrMissingID for empty CrossReference ID")
	}
}

func TestBlueprintSchemaValidator_ErrDuplicateID(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// Duplicate node IDs in architecture
	arch := ArchitectureSection{
		Groups: []ArchGroup{{ID: "grp-1", Label: "Group 1", Order: 1}},
		Nodes: []ArchNode{
			{ID: "node-dup", GroupID: "grp-1", Type: "service", Label: "Service 1", Layout: ArchLayout{X: 0, Y: 0}},
			{ID: "node-dup", GroupID: "grp-1", Type: "database", Label: "Service 2", Layout: ArchLayout{X: 10, Y: 10}},
		},
		Edges: []ArchEdge{},
	}
	issues := validator.ValidateArchitecture(&arch)
	if !hasErrorCode(issues, ErrDuplicateID) {
		t.Errorf("expected ErrDuplicateID for duplicate architecture node ID")
	}

	// Duplicate across bundle
	bundle := createValidBundle()
	bundle.Journey.Phases = append(bundle.Journey.Phases, JourneyPhase{
		ID:    "node-booking-api", // conflicts with architecture node ID
		Label: "Conflicting Phase",
		Order: 3,
	})
	bundleIssues := validator.ValidateBundle(&bundle)
	if !hasErrorCode(bundleIssues, ErrDuplicateID) {
		t.Errorf("expected ErrDuplicateID for global duplicate ID in bundle")
	}
}

func TestBlueprintSchemaValidator_ErrInvalidCanonicalRef(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// Malformed canonical reference in journey activity
	journey := JourneySection{
		Phases: []JourneyPhase{{ID: "phase-1", Label: "Phase 1", Order: 1}},
		Actors: []JourneyActor{{ID: "actor-1", Label: "Actor 1", Order: 1}},
		Activities: []JourneyActivity{
			{
				ID:          "act-1",
				PhaseID:     "phase-1",
				ActorID:     "actor-1",
				Label:       "Act 1",
				References:  []string{"invalid://architecture/node/1", "wbs-module-payment"},
				Touchpoints: []string{"App"},
			},
		},
	}
	issues := validator.ValidateJourney(&journey)
	if !hasErrorCode(issues, ErrInvalidCanonicalRef) {
		t.Errorf("expected ErrInvalidCanonicalRef for malformed canonical ref")
	}

	// In cross references
	bundle := createValidBundle()
	bundle.CrossReferences = append(bundle.CrossReferences, CrossReference{
		ID:           "xref-bad",
		SourceRef:    "not a valid canonical ref",
		TargetRef:    "workflow.wf-booking.step.wnode-submit",
		RelationType: "implements",
	})
	bundleIssues := validator.ValidateBundle(&bundle)
	if !hasErrorCode(bundleIssues, ErrInvalidCanonicalRef) {
		t.Errorf("expected ErrInvalidCanonicalRef for malformed CrossReference.SourceRef")
	}
}

func TestBlueprintSchemaValidator_ErrDanglingReference(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// 1. Architecture node referencing non-existent group
	arch := ArchitectureSection{
		Groups: []ArchGroup{{ID: "grp-1", Label: "Group 1", Order: 1}},
		Nodes: []ArchNode{
			{ID: "node-1", GroupID: "grp-nonexistent", Type: "service", Label: "Service 1", Layout: ArchLayout{X: 0, Y: 0}},
		},
		Edges: []ArchEdge{},
	}
	issues := validator.ValidateArchitecture(&arch)
	if !hasErrorCode(issues, ErrDanglingReference) {
		t.Errorf("expected ErrDanglingReference for invalid group_id in architecture node")
	}

	// 2. Architecture edge referencing non-existent node
	arch2 := ArchitectureSection{
		Groups: []ArchGroup{{ID: "grp-1", Label: "Group 1", Order: 1}},
		Nodes: []ArchNode{
			{ID: "node-1", GroupID: "grp-1", Type: "service", Label: "Service 1", Layout: ArchLayout{X: 0, Y: 0}},
		},
		Edges: []ArchEdge{
			{ID: "edge-1", Source: "node-1", Target: "node-nonexistent", Label: "Calls", Type: "sync"},
		},
	}
	issues = validator.ValidateArchitecture(&arch2)
	if !hasErrorCode(issues, ErrDanglingReference) {
		t.Errorf("expected ErrDanglingReference for invalid target in architecture edge")
	}

	// 3. Cross-section canonical reference pointing to missing symbol in bundle
	bundle := createValidBundle()
	bundle.Journey.Activities[0].References = append(
		bundle.Journey.Activities[0].References,
		"architecture.node.node-nonexistent",
	)
	bundleIssues := validator.ValidateBundle(&bundle)
	if !hasErrorCode(bundleIssues, ErrDanglingReference) {
		t.Errorf("expected ErrDanglingReference for unresolvable canonical reference in bundle")
	}
}

func TestBlueprintSchemaValidator_ErrJourneyOrphanActivity(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// Activity with unknown phase_id
	journeyPhaseOrphan := JourneySection{
		Phases: []JourneyPhase{{ID: "phase-1", Label: "Phase 1", Order: 1}},
		Actors: []JourneyActor{{ID: "actor-1", Label: "Actor 1", Order: 1}},
		Activities: []JourneyActivity{
			{
				ID:      "act-1",
				PhaseID: "phase-unknown",
				ActorID: "actor-1",
				Label:   "Act 1",
			},
		},
	}
	issues := validator.ValidateJourney(&journeyPhaseOrphan)
	if !hasErrorCode(issues, ErrJourneyOrphanActivity) {
		t.Errorf("expected ErrJourneyOrphanActivity for missing phase_id")
	}

	// Activity with unknown actor_id
	journeyActorOrphan := JourneySection{
		Phases: []JourneyPhase{{ID: "phase-1", Label: "Phase 1", Order: 1}},
		Actors: []JourneyActor{{ID: "actor-1", Label: "Actor 1", Order: 1}},
		Activities: []JourneyActivity{
			{
				ID:      "act-2",
				PhaseID: "phase-1",
				ActorID: "actor-unknown",
				Label:   "Act 2",
			},
		},
	}
	issues = validator.ValidateJourney(&journeyActorOrphan)
	if !hasErrorCode(issues, ErrJourneyOrphanActivity) {
		t.Errorf("expected ErrJourneyOrphanActivity for missing actor_id")
	}
}

func TestBlueprintSchemaValidator_ErrStateInvalidInitial(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	sm := StateMachineSection{
		Entity:       "Order",
		InitialState: "state-unknown",
		States: []SMState{
			{ID: "state-draft", Label: "Draft", Type: "normal", Layout: ArchLayout{X: 0, Y: 0}},
		},
		Transitions: []SMTransition{},
	}
	issues := validator.ValidateStateMachine(&sm)
	if !hasErrorCode(issues, ErrStateInvalidInitial) {
		t.Errorf("expected ErrStateInvalidInitial when initial_state is not in states")
	}
}

func TestBlueprintSchemaValidator_ErrStateDanglingTransition(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// Source missing
	smSource := StateMachineSection{
		Entity:       "Order",
		InitialState: "state-draft",
		States: []SMState{
			{ID: "state-draft", Label: "Draft", Type: "normal", Layout: ArchLayout{X: 0, Y: 0}},
			{ID: "state-done", Label: "Done", Type: "terminal", Layout: ArchLayout{X: 10, Y: 10}},
		},
		Transitions: []SMTransition{
			{
				ID:     "trans-1",
				Source: "state-ghost",
				Target: "state-done",
				Event:  "FINISH",
			},
		},
	}
	issues := validator.ValidateStateMachine(&smSource)
	if !hasErrorCode(issues, ErrStateDanglingTransition) {
		t.Errorf("expected ErrStateDanglingTransition for invalid transition source")
	}

	// Target missing
	smTarget := StateMachineSection{
		Entity:       "Order",
		InitialState: "state-draft",
		States: []SMState{
			{ID: "state-draft", Label: "Draft", Type: "normal", Layout: ArchLayout{X: 0, Y: 0}},
		},
		Transitions: []SMTransition{
			{
				ID:     "trans-2",
				Source: "state-draft",
				Target: "state-ghost",
				Event:  "FINISH",
			},
		},
	}
	issues = validator.ValidateStateMachine(&smTarget)
	if !hasErrorCode(issues, ErrStateDanglingTransition) {
		t.Errorf("expected ErrStateDanglingTransition for invalid transition target")
	}
}

func TestBlueprintSchemaValidator_ErrWorkflowOrphanNode(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	workflows := []Workflow{
		{
			ID:   "wf-1",
			Name: "Workflow 1",
			Lanes: []WorkflowLane{
				{ID: "lane-1", Label: "Lane 1", Order: 1},
			},
			Nodes: []WorkflowNode{
				{
					ID:     "wnode-1",
					LaneID: "lane-ghost",
					Type:   "action",
					Label:  "Action 1",
					Layout: ArchLayout{X: 0, Y: 0},
				},
			},
			Edges: []WorkflowEdge{},
		},
	}

	issues := validator.ValidateWorkflows(workflows)
	if !hasErrorCode(issues, ErrWorkflowOrphanNode) {
		t.Errorf("expected ErrWorkflowOrphanNode for invalid lane_id")
	}
}

func TestBlueprintSchemaValidator_ErrWorkflowDanglingEdge(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	workflows := []Workflow{
		{
			ID:   "wf-1",
			Name: "Workflow 1",
			Lanes: []WorkflowLane{
				{ID: "lane-1", Label: "Lane 1", Order: 1},
			},
			Nodes: []WorkflowNode{
				{ID: "wnode-1", LaneID: "lane-1", Type: "action", Label: "Action 1", Layout: ArchLayout{X: 0, Y: 0}},
			},
			Edges: []WorkflowEdge{
				{ID: "wedge-1", Source: "wnode-1", Target: "wnode-ghost", Label: "Next"},
			},
		},
	}

	issues := validator.ValidateWorkflows(workflows)
	if !hasErrorCode(issues, ErrWorkflowDanglingEdge) {
		t.Errorf("expected ErrWorkflowDanglingEdge for edge target not in nodes")
	}
}

func TestBlueprintSchemaValidator_ErrWorkflowDecisionUnderbranched(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	workflows := []Workflow{
		{
			ID:   "wf-1",
			Name: "Workflow 1",
			Lanes: []WorkflowLane{
				{ID: "lane-1", Label: "Lane 1", Order: 1},
			},
			Nodes: []WorkflowNode{
				{ID: "wnode-dec", LaneID: "lane-1", Type: "decision", Label: "Is Valid?", Layout: ArchLayout{X: 0, Y: 0}},
				{ID: "wnode-next", LaneID: "lane-1", Type: "end", Label: "Done", Layout: ArchLayout{X: 10, Y: 0}},
			},
			Edges: []WorkflowEdge{
				{ID: "wedge-1", Source: "wnode-dec", Target: "wnode-next", Label: "Yes"},
			},
		},
	}

	issues := validator.ValidateWorkflows(workflows)
	if !hasErrorCode(issues, ErrWorkflowDecisionUnderbranched) {
		t.Errorf("expected ErrWorkflowDecisionUnderbranched when decision has < 2 outgoing edges")
	}
}

func TestBlueprintSchemaValidator_ErrWBSInvalidDependency(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// 1. Self-dependency
	wbsSelf := WBSSection{
		Modules: []WBSModule{
			{
				ID:           "mod-self",
				Name:         "Self Mod",
				Dependencies: []string{"mod-self"},
			},
		},
	}
	issues := validator.ValidateWBS(&wbsSelf)
	if !hasErrorCode(issues, ErrWBSInvalidDependency) {
		t.Errorf("expected ErrWBSInvalidDependency for self-dependency")
	}

	// 2. Non-existent dependency
	wbsMissing := WBSSection{
		Modules: []WBSModule{
			{
				ID:           "mod-1",
				Name:         "Mod 1",
				Dependencies: []string{"mod-unknown"},
			},
		},
	}
	issues = validator.ValidateWBS(&wbsMissing)
	if !hasErrorCode(issues, ErrWBSInvalidDependency) {
		t.Errorf("expected ErrWBSInvalidDependency for non-existent dependency")
	}
}

func TestBlueprintSchemaValidator_ErrWBSDependencyCycle(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// 2-node cycle: A -> B -> A
	wbs2NodeCycle := WBSSection{
		Modules: []WBSModule{
			{ID: "mod-a", Name: "Module A", Dependencies: []string{"mod-b"}},
			{ID: "mod-b", Name: "Module B", Dependencies: []string{"mod-a"}},
		},
	}
	issues := validator.ValidateWBS(&wbs2NodeCycle)
	if !hasErrorCode(issues, ErrWBSDependencyCycle) {
		t.Errorf("expected ErrWBSDependencyCycle for 2-node cycle")
	}

	// 3-node cycle: A -> B -> C -> A
	wbs3NodeCycle := WBSSection{
		Modules: []WBSModule{
			{ID: "mod-a", Name: "Module A", Dependencies: []string{"mod-b"}},
			{ID: "mod-b", Name: "Module B", Dependencies: []string{"mod-c"}},
			{ID: "mod-c", Name: "Module C", Dependencies: []string{"mod-a"}},
		},
	}
	issues = validator.ValidateWBS(&wbs3NodeCycle)
	if !hasErrorCode(issues, ErrWBSDependencyCycle) {
		t.Errorf("expected ErrWBSDependencyCycle for 3-node cycle")
	}

	// Valid DAG diamond (no cycle): A -> B, A -> C, B -> D, C -> D, D -> none
	wbsDAG := WBSSection{
		Modules: []WBSModule{
			{ID: "mod-d", Name: "Module D", Dependencies: []string{}},
			{ID: "mod-b", Name: "Module B", Dependencies: []string{"mod-d"}},
			{ID: "mod-c", Name: "Module C", Dependencies: []string{"mod-d"}},
			{ID: "mod-a", Name: "Module A", Dependencies: []string{"mod-b", "mod-c"}},
		},
	}
	issuesDAG := validator.ValidateWBS(&wbsDAG)
	if hasErrorCode(issuesDAG, ErrWBSDependencyCycle) {
		t.Errorf("unexpected ErrWBSDependencyCycle for valid DAG diamond")
	}
}

func TestBlueprintSchemaValidator_ErrMalformedLayout(t *testing.T) {
	validator := NewBlueprintSchemaValidator()

	// Architecture node with NaN layout
	arch := ArchitectureSection{
		Groups: []ArchGroup{{ID: "grp-1", Label: "Group 1", Order: 1}},
		Nodes: []ArchNode{
			{
				ID:      "node-1",
				GroupID: "grp-1",
				Type:    "service",
				Label:   "Service 1",
				Layout:  ArchLayout{X: math.NaN(), Y: 100},
			},
		},
		Edges: []ArchEdge{},
	}
	issues := validator.ValidateArchitecture(&arch)
	if !hasErrorCode(issues, ErrMalformedLayout) {
		t.Errorf("expected ErrMalformedLayout for NaN coordinate in ArchNode")
	}

	// State machine state with +Inf layout
	sm := StateMachineSection{
		Entity:       "Order",
		InitialState: "state-1",
		States: []SMState{
			{
				ID:     "state-1",
				Label:  "State 1",
				Type:   "normal",
				Layout: ArchLayout{X: 100, Y: math.Inf(1)},
			},
		},
		Transitions: []SMTransition{},
	}
	issues = validator.ValidateStateMachine(&sm)
	if !hasErrorCode(issues, ErrMalformedLayout) {
		t.Errorf("expected ErrMalformedLayout for +Inf coordinate in SMState")
	}

	// Workflow node with -Inf layout
	wf := []Workflow{
		{
			ID:   "wf-1",
			Name: "Workflow 1",
			Lanes: []WorkflowLane{
				{ID: "lane-1", Label: "Lane 1", Order: 1},
			},
			Nodes: []WorkflowNode{
				{
					ID:     "wnode-1",
					LaneID: "lane-1",
					Type:   "action",
					Label:  "Action 1",
					Layout: ArchLayout{X: math.Inf(-1), Y: 0},
				},
			},
			Edges: []WorkflowEdge{},
		},
	}
	issues = validator.ValidateWorkflows(wf)
	if !hasErrorCode(issues, ErrMalformedLayout) {
		t.Errorf("expected ErrMalformedLayout for -Inf coordinate in WorkflowNode")
	}
}
