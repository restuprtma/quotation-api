package service

import "encoding/json"

// BlueprintSchemaVersion defines the accepted schema version for blueprint bundles.
const BlueprintSchemaVersion = "1.0"

// Safe outcome codes for blueprint validation.
const (
	BlueprintValidationInvalidSchema        = "invalid_schema"
	BlueprintValidationCrossValidationError = "cross_validation_error"
	BlueprintValidationSecurityFailure      = "security_failure"
)

// Issue codes for structural and semantic validation errors.
const (
	BlueprintIssueDuplicateID             = "ERR_DUPLICATE_ID"
	BlueprintIssueMissingID               = "ERR_MISSING_ID"
	BlueprintIssueInvalidCanonicalRef     = "ERR_INVALID_CANONICAL_REF"
	BlueprintIssueAmbiguousReference      = "ERR_AMBIGUOUS_REFERENCE"
	BlueprintIssueDanglingReference       = "ERR_DANGLING_REFERENCE"
	BlueprintIssueStateInvalidInitial     = "ERR_STATE_INVALID_INITIAL"
	BlueprintIssueStateDanglingTransition = "ERR_STATE_DANGLING_TRANSITION"
	BlueprintIssueWorkflowOrphanNode      = "ERR_WORKFLOW_ORPHAN_NODE"
	BlueprintIssueWorkflowUnderbranched   = "ERR_WORKFLOW_DECISION_UNDERBRANCHED"
	BlueprintIssueWBSInvalidDependency    = "ERR_WBS_INVALID_DEPENDENCY"
	BlueprintIssueWBSDependencyCycle      = "ERR_WBS_DEPENDENCY_CYCLE"
	BlueprintIssueMalformedLayout         = "ERR_MALFORMED_LAYOUT"
	BlueprintIssueUnsupportedVersion      = "ERR_UNSUPPORTED_SCHEMA_VERSION"
	BlueprintIssueForbiddenContent        = "ERR_FORBIDDEN_CONTENT"
)

// Semantic warning codes for cross validation.
const (
	BlueprintWarningArchitectureUncovered = "WARN_ARCH_NODE_UNREFERENCED_IN_WBS"
	BlueprintWarningJourneyUnlinked       = "WARN_JOURNEY_ACTIVITY_UNLINKED"
	BlueprintWarningEmptySection          = "WARN_EMPTY_SECTION"
)

// BlueprintValidationIssue represents a single structural or cross-validation defect.
type BlueprintValidationIssue struct {
	Code        string `json:"code"`
	Severity    string `json:"severity"`
	Section     string `json:"section"`
	Path        string `json:"path"`
	TargetID    string `json:"target_id,omitempty"`
	Message     string `json:"message"`
	Blocking    bool   `json:"blocking"`
	Remediation string `json:"remediation,omitempty"`
}

// BlueprintValidationResult holds the output of single-section structural validation.
type BlueprintValidationResult struct {
	Valid             bool                       `json:"valid"`
	SafeOutcome       string                     `json:"safe_outcome,omitempty"`
	Issues            []BlueprintValidationIssue `json:"issues"`
	NormalizedContent json.RawMessage            `json:"-"`
}

// BlueprintCoverage calculates cross-sectional reference coverage ratios.
type BlueprintCoverage struct {
	JourneyToWorkflow float64 `json:"journey_to_workflow"`
	ArchitectureToWBS float64 `json:"architecture_to_wbs"`
}

// BlueprintCrossValidationResult holds the output of whole-bundle semantic cross-validation.
type BlueprintCrossValidationResult struct {
	Valid       bool                       `json:"valid"`
	SafeOutcome string                     `json:"safe_outcome,omitempty"`
	Issues      []BlueprintValidationIssue `json:"issues"`
	Coverage    BlueprintCoverage          `json:"coverage"`
}

// BlueprintLayout represents 2D coordinates for visual diagram nodes.
type BlueprintLayout struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// BlueprintJourneyPhase represents a chronological phase in a user journey.
type BlueprintJourneyPhase struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// BlueprintJourneyActor represents an actor participating in a user journey.
type BlueprintJourneyActor struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// BlueprintJourneyActivity represents a user touchpoint activity.
type BlueprintJourneyActivity struct {
	ID          string   `json:"id"`
	PhaseID     string   `json:"phase_id"`
	ActorID     string   `json:"actor_id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Touchpoints []string `json:"touchpoints"`
	References  []string `json:"references"`
}

// BlueprintJourney represents the complete User Journey section.
type BlueprintJourney struct {
	Phases     []BlueprintJourneyPhase    `json:"phases"`
	Actors     []BlueprintJourneyActor    `json:"actors"`
	Activities []BlueprintJourneyActivity `json:"activities"`
}

// BlueprintArchitectureGroup represents a logical or physical tier in the architecture.
type BlueprintArchitectureGroup struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// BlueprintArchitectureNode represents a component, service, or database node.
type BlueprintArchitectureNode struct {
	ID          string          `json:"id"`
	GroupID     string          `json:"group_id"`
	Type        string          `json:"type"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Layout      BlueprintLayout `json:"layout"`
}

// BlueprintArchitectureEdge represents a connection or data flow between nodes.
type BlueprintArchitectureEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
	Type   string `json:"type"`
}

// BlueprintArchitecture represents the complete Architecture section.
type BlueprintArchitecture struct {
	Groups []BlueprintArchitectureGroup `json:"groups"`
	Nodes  []BlueprintArchitectureNode  `json:"nodes"`
	Edges  []BlueprintArchitectureEdge  `json:"edges"`
}

// BlueprintState represents a state in an entity lifecycle.
type BlueprintState struct {
	ID     string          `json:"id"`
	Label  string          `json:"label"`
	Type   string          `json:"type"`
	Layout BlueprintLayout `json:"layout"`
}

// BlueprintTransition represents an allowed state transition.
type BlueprintTransition struct {
	ID      string   `json:"id"`
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Event   string   `json:"event"`
	Actor   string   `json:"actor"`
	Guard   string   `json:"guard"`
	Actions []string `json:"actions"`
}

// BlueprintStateMachine represents the complete State Machine section.
type BlueprintStateMachine struct {
	Entity       string                `json:"entity"`
	InitialState string                `json:"initial_state"`
	States       []BlueprintState      `json:"states"`
	Transitions  []BlueprintTransition `json:"transitions"`
}

// BlueprintWorkflowLane represents a swimlane in a business workflow.
type BlueprintWorkflowLane struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// BlueprintWorkflowNode represents a step node in a business workflow.
type BlueprintWorkflowNode struct {
	ID          string          `json:"id"`
	LaneID      string          `json:"lane_id"`
	Type        string          `json:"type"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
	Layout      BlueprintLayout `json:"layout"`
}

// BlueprintWorkflowEdge represents a directed transition in a workflow.
type BlueprintWorkflowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// BlueprintWorkflow represents a business process workflow.
type BlueprintWorkflow struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Lanes       []BlueprintWorkflowLane `json:"lanes"`
	Nodes       []BlueprintWorkflowNode `json:"nodes"`
	Edges       []BlueprintWorkflowEdge `json:"edges"`
}

// BlueprintWBSFeature represents a granular feature under a WBS module.
type BlueprintWBSFeature struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

// BlueprintWBSModule represents a top-level module in the WBS.
type BlueprintWBSModule struct {
	ID           string                `json:"id"`
	Name         string                `json:"name"`
	Description  string                `json:"description"`
	Priority     string                `json:"priority"`
	Dependencies []string              `json:"dependencies"`
	References   []string              `json:"references"`
	Features     []BlueprintWBSFeature `json:"features"`
}

// BlueprintWBS represents the complete Work Breakdown Structure section.
type BlueprintWBS struct {
	Modules []BlueprintWBSModule `json:"modules"`
}

// BlueprintOpenQuestion represents an unresolved design inquiry.
type BlueprintOpenQuestion struct {
	ID            string `json:"id"`
	Question      string `json:"question"`
	TargetSection string `json:"target_section"`
	Impact        string `json:"impact"`
	Status        string `json:"status"`
}

// BlueprintCrossReference represents a cross-sectional traceability link.
type BlueprintCrossReference struct {
	ID           string `json:"id"`
	SourceRef    string `json:"source_ref"`
	TargetRef    string `json:"target_ref"`
	RelationType string `json:"relation_type"`
	Description  string `json:"description"`
}

// BlueprintBundle represents the complete 5-section Blueprint bundle contract.
type BlueprintBundle struct {
	SchemaVersion   string                    `json:"schema_version"`
	Title           string                    `json:"title"`
	Summary         string                    `json:"summary"`
	Journey         BlueprintJourney          `json:"journey"`
	Architecture    BlueprintArchitecture     `json:"architecture"`
	StateMachine    BlueprintStateMachine     `json:"state_machine"`
	Workflows       []BlueprintWorkflow       `json:"workflows"`
	WBS             BlueprintWBS              `json:"wbs"`
	Assumptions     []string                  `json:"assumptions"`
	OpenQuestions   []BlueprintOpenQuestion   `json:"open_questions"`
	CrossReferences []BlueprintCrossReference `json:"cross_references"`
}
