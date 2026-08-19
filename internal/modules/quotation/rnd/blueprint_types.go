package rnd

import "time"

// Schema version constant.
const (
	BlueprintSchemaVersion = "1.0"
)

// Machine outcome codes for the blueprint pipeline.
const (
	OutcomeSuccess              = "success"
	OutcomeInvalidInput         = "invalid_input"
	OutcomeInvalidSchema        = "invalid_schema"
	OutcomeCrossValidationError = "cross_validation_error"
	OutcomeSecurityFailure      = "security_failure"
	OutcomeRetry                = "retry"
)

// Severity levels for validation issues.
const (
	SeverityBlockingError = "BLOCKING_ERROR"
	SeverityWarning       = "WARNING"
)

// Blocking error codes from cross-validation-rules.md.
const (
	ErrDuplicateID                   = "ERR_DUPLICATE_ID"
	ErrMissingID                     = "ERR_MISSING_ID"
	ErrInvalidCanonicalRef           = "ERR_INVALID_CANONICAL_REF"
	ErrDanglingReference             = "ERR_DANGLING_REFERENCE"
	ErrJourneyOrphanActivity         = "ERR_JOURNEY_ORPHAN_ACTIVITY"
	ErrStateInvalidInitial           = "ERR_STATE_INVALID_INITIAL"
	ErrStateDanglingTransition       = "ERR_STATE_DANGLING_TRANSITION"
	ErrWorkflowOrphanNode            = "ERR_WORKFLOW_ORPHAN_NODE"
	ErrWorkflowDanglingEdge          = "ERR_WORKFLOW_DANGLING_EDGE"
	ErrWorkflowDecisionUnderbranched = "ERR_WORKFLOW_DECISION_UNDERBRANCHED"
	ErrWBSInvalidDependency          = "ERR_WBS_INVALID_DEPENDENCY"
	ErrWBSDependencyCycle            = "ERR_WBS_DEPENDENCY_CYCLE"
	ErrMalformedLayout               = "ERR_MALFORMED_LAYOUT"
	ErrUnsupportedSchemaVersion      = "ERR_UNSUPPORTED_SCHEMA_VERSION"
)

// Semantic warning codes from cross-validation-rules.md.
const (
	WarnArchNodeUnreferencedInWBS = "WARN_ARCH_NODE_UNREFERENCED_IN_WBS"
	WarnJourneyActivityUnlinked   = "WARN_JOURNEY_ACTIVITY_UNLINKED"
	WarnTerminologyMismatch       = "WARN_TERMINOLOGY_MISMATCH"
	WarnOpenQuestionUnresolved    = "WARN_OPEN_QUESTION_UNRESOLVED"
	WarnEmptySection              = "WARN_EMPTY_SECTION"
)

// SectionStatus represents the lifecycle status of an individual section.
type SectionStatus string

const (
	StatusPending    SectionStatus = "pending"
	StatusGenerating SectionStatus = "generating"
	StatusCompleted  SectionStatus = "completed"
	StatusFailed     SectionStatus = "failed"
	StatusManualEdit SectionStatus = "manual_edit"
	StatusValidated  SectionStatus = "validated"
)

// PublicationStatus represents the publication state of a blueprint revision.
type PublicationStatus string

const (
	PublicationStatusDraft      PublicationStatus = "draft"
	PublicationStatusPublished  PublicationStatus = "published"
	PublicationStatusSuperseded PublicationStatus = "superseded"
)

// CompletenessStatus represents whether all 5 core sections are completed/validated.
type CompletenessStatus string

const (
	CompletenessStatusPartial  CompletenessStatus = "partial"
	CompletenessStatusComplete CompletenessStatus = "complete"
)

// BlueprintStage represents a stage in the 7-stage generation pipeline.
type BlueprintStage string

const (
	StageContextAnalysis  BlueprintStage = "context_analysis"
	StageJourney          BlueprintStage = "journey"
	StageArchitecture     BlueprintStage = "architecture"
	StageStateMachine     BlueprintStage = "state_machine"
	StageWorkflow         BlueprintStage = "workflow"
	StageWBS              BlueprintStage = "wbs"
	StageCrossValidation  BlueprintStage = "cross_validation"
)

// BlueprintSourceContext captures the high-level context extracted from the source quotation revision.
type BlueprintSourceContext struct {
	Subject      string   `json:"subject"`
	SolutionType string   `json:"solution_type"`
	Summary      string   `json:"summary"`
	Scope        []string `json:"scope"`
	Timeline     string   `json:"timeline"`
}

// BlueprintInput matches docs/roadmap/rnd/blueprint-skill/input-contract.json.
type BlueprintInput struct {
	SourceQuotationRevisionID string                 `json:"source_quotation_revision_id"`
	SourceContext             BlueprintSourceContext `json:"source_context"`
	BusinessGoals             []string               `json:"business_goals"`
	Actors                    []string               `json:"actors"`
	Channels                  []string               `json:"channels"`
	BusinessPhases            []string               `json:"business_phases"`
	BusinessRules             []string               `json:"business_rules"`
	KnownStates               []string               `json:"known_states"`
	Integrations              []string               `json:"integrations"`
	TechnologyPreferences     []string               `json:"technology_preferences"`
	OperationalConstraints    []string               `json:"operational_constraints"`
	AdditionalNotes           string                 `json:"additional_notes"`
}

// LayoutCoordinate represents 2D layout coordinates for diagram placement.
type LayoutCoordinate struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ArchLayout is an alias/type for 2D layout coordinates.
type ArchLayout = LayoutCoordinate

// JourneyPhase represents a chronological phase in a user journey.
type JourneyPhase struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// JourneyActor represents an actor participating in a user journey.
type JourneyActor struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// JourneyActivity represents a user touchpoint activity.
type JourneyActivity struct {
	ID          string   `json:"id"`
	PhaseID     string   `json:"phase_id"`
	ActorID     string   `json:"actor_id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	Touchpoints []string `json:"touchpoints"`
	References  []string `json:"references"`
}

// JourneySection contains phases, actors, and activities.
type JourneySection struct {
	Phases     []JourneyPhase    `json:"phases"`
	Actors     []JourneyActor    `json:"actors"`
	Activities []JourneyActivity `json:"activities"`
}

// ArchGroup represents an architecture grouping tier.
type ArchGroup struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// ArchNode represents a node in the system architecture diagram.
type ArchNode struct {
	ID          string     `json:"id"`
	GroupID     string     `json:"group_id"`
	Type        string     `json:"type"` // channel, application, service, database, cache, storage, integration, device
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Layout      ArchLayout `json:"layout"`
}

// ArchEdge represents a connection between architecture nodes.
type ArchEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
	Type   string `json:"type"` // sync, async, data_flow, event
}

// ArchitectureSection contains groups, nodes, and edges.
type ArchitectureSection struct {
	Groups []ArchGroup `json:"groups"`
	Nodes  []ArchNode  `json:"nodes"`
	Edges  []ArchEdge  `json:"edges"`
}

// SMState represents a state in the state machine diagram.
type SMState struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Type   string     `json:"type"` // normal, terminal, exception
	Layout ArchLayout `json:"layout"`
}

// SMTransition represents a state transition.
type SMTransition struct {
	ID      string   `json:"id"`
	Source  string   `json:"source"`
	Target  string   `json:"target"`
	Event   string   `json:"event"`
	Actor   string   `json:"actor"`
	Guard   string   `json:"guard"`
	Actions []string `json:"actions"`
}

// StateMachineSection contains entity lifecycle state machine definitions.
type StateMachineSection struct {
	Entity       string         `json:"entity"`
	InitialState string         `json:"initial_state"`
	States       []SMState      `json:"states"`
	Transitions  []SMTransition `json:"transitions"`
}

// WorkflowLane represents a swimlane in a workflow diagram.
type WorkflowLane struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Order int    `json:"order"`
}

// WorkflowNode represents a step node in a workflow diagram.
type WorkflowNode struct {
	ID          string     `json:"id"`
	LaneID      string     `json:"lane_id"`
	Type        string     `json:"type"` // start, action, system_action, decision, wait, end
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Layout      ArchLayout `json:"layout"`
}

// WorkflowEdge represents a transition between workflow nodes.
type WorkflowEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// Workflow represents a business process workflow with swimlanes.
type Workflow struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Lanes       []WorkflowLane `json:"lanes"`
	Nodes       []WorkflowNode `json:"nodes"`
	Edges       []WorkflowEdge `json:"edges"`
}

// WorkflowSection represents a collection of workflows.
type WorkflowSection = []Workflow

// WBSFeature represents a granular feature under a WBS module.
type WBSFeature struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

// WBSModule represents a top-level module in the work breakdown structure.
type WBSModule struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Priority     string       `json:"priority"` // high, medium, low
	Dependencies []string     `json:"dependencies"`
	References   []string     `json:"references"`
	Features     []WBSFeature `json:"features"`
}

// WBSSection contains WBS modules.
type WBSSection struct {
	Modules []WBSModule `json:"modules"`
}

// OpenQuestion represents an unresolved inquiry requiring clarification.
type OpenQuestion struct {
	ID            string `json:"id"`
	Question      string `json:"question"`
	TargetSection string `json:"target_section"` // journey, architecture, state_machine, workflow, wbs, general
	Impact        string `json:"impact"`
	Status        string `json:"status"` // open, answered, deferred
}

// CrossReference represents a traceability link across blueprint sections.
type CrossReference struct {
	ID           string `json:"id"`
	SourceRef    string `json:"source_ref"`
	TargetRef    string `json:"target_ref"`
	RelationType string `json:"relation_type"` // implements, transitions_on, interacts_with, deploys_to, depends_on
	Description  string `json:"description"`
}

// BlueprintBundle represents the complete 5-section Blueprint bundle matching bundle-schema.json.
type BlueprintBundle struct {
	SchemaVersion   string              `json:"schema_version"`
	Title           string              `json:"title"`
	Summary         string              `json:"summary"`
	Journey         JourneySection      `json:"journey"`
	Architecture    ArchitectureSection `json:"architecture"`
	StateMachine    StateMachineSection `json:"state_machine"`
	Workflows       []Workflow          `json:"workflows"`
	WBS             WBSSection          `json:"wbs"`
	Assumptions     []string            `json:"assumptions"`
	OpenQuestions   []OpenQuestion      `json:"open_questions"`
	CrossReferences []CrossReference    `json:"cross_references"`
}

// ValidationIssue represents a single validation check finding (error or warning).
type ValidationIssue struct {
	Code                 string                 `json:"code"`
	Severity             string                 `json:"severity"`
	Section              string                 `json:"section"`
	Field                string                 `json:"field,omitempty"`
	Path                 string                 `json:"path,omitempty"`
	TargetID             string                 `json:"target_id,omitempty"`
	Message              string                 `json:"message"`
	Blocking             bool                   `json:"blocking"`
	ConfirmationRequired bool                   `json:"confirmation_required,omitempty"`
	Remediation          string                 `json:"remediation,omitempty"`
	Context              map[string]interface{} `json:"context,omitempty"`
}

// BlueprintStageResult records the execution outcome of a single pipeline stage.
type BlueprintStageResult struct {
	Stage      BlueprintStage    `json:"stage"`
	Status     SectionStatus     `json:"status"`
	StartedAt  time.Time         `json:"started_at"`
	DurationMS int64             `json:"duration_ms"`
	RetryCount int               `json:"retry_count"`
	Issues     []ValidationIssue `json:"issues,omitempty"`
	Error      string            `json:"error,omitempty"`
	Payload    interface{}       `json:"payload,omitempty"`
}

// BlueprintReportSummary aggregates stage metrics and overall status.
type BlueprintReportSummary struct {
	TotalStages        int  `json:"total_stages"`
	CompletedStages    int  `json:"completed_stages"`
	FailedStages       int  `json:"failed_stages"`
	BlockingErrors     int  `json:"blocking_errors"`
	WarningsCount      int  `json:"warnings_count"`
	Passed             bool `json:"passed"`
	CanPublishPartial  bool `json:"can_publish_partial"`
	CanPublishComplete bool `json:"can_publish_complete"`
}

// BlueprintBatchReport represents the full execution report of a blueprint pipeline run.
type BlueprintBatchReport struct {
	SchemaVersion  string                 `json:"schema_version"`
	Fixture        string                 `json:"fixture"`
	Provider       string                 `json:"provider"`
	Model          string                 `json:"model"`
	PromptVersion  string                 `json:"prompt_version"`
	Status         string                 `json:"status"`
	Stages         []BlueprintStageResult `json:"stages"`
	Issues         []ValidationIssue      `json:"issues,omitempty"`
	Summary        BlueprintReportSummary `json:"summary"`
	Bundle         *BlueprintBundle       `json:"bundle,omitempty"`
	StartedAt      time.Time              `json:"started_at,omitempty"`
	CompletedAt    time.Time              `json:"completed_at,omitempty"`
	TotalLatencyMS int64                  `json:"total_latency_ms,omitempty"`
}
