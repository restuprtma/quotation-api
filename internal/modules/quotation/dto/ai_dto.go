package dto

type AIGenerateRequest struct {
	ProjectType        string                 `json:"project_type" binding:"required,oneof=product_based project_based"`
	ClientName         string                 `json:"client_name" binding:"required"`
	ClientIndustry     string                 `json:"client_industry" binding:"required"`
	Subject            string                 `json:"subject" binding:"required"`
	ClientRequirements []string               `json:"client_requirements" binding:"required,min=1"`
	PricingContext     map[string]interface{} `json:"pricing_context"` // Read-only context for the AI
	UserNotes          string                 `json:"user_notes"`
	Locale             string                 `json:"locale"`
}

type AIGenerateResponse struct {
	SchemaVersion          string                   `json:"schema_version"`
	Narrative              AINarrative              `json:"narrative"`
	ProductOrProjectDetail AIProductOrProjectDetail `json:"product_or_project_detail"`
	Blueprint              AIBlueprint              `json:"blueprint"`
	PricingLabels          []string                 `json:"pricing_labels"`
}

type AINarrative struct {
	Introduction string   `json:"introduction"`
	Scope        []string `json:"scope"`
}

type AIProductOrProjectDetail struct {
	Mode    string           `json:"mode"`              // "product" or "project"
	Modules []AIModuleDetail `json:"modules,omitempty"` // For product

	// For project
	Goals        []string                 `json:"goals,omitempty"`
	Deliverables []string                 `json:"deliverables,omitempty"`
	Milestones   []map[string]interface{} `json:"milestones,omitempty"`
	Team         []map[string]interface{} `json:"team,omitempty"`
}

type AIModuleDetail struct {
	Name     string   `json:"name"`
	Features []string `json:"features"`
}

type AIBlueprint struct {
	Title       string                   `json:"title"`
	Summary     string                   `json:"summary"`
	Actors      []map[string]interface{} `json:"actors"`
	Diagrams    []AIDiagram              `json:"diagrams"`
	Rules       []string                 `json:"rules"`
	Assumptions []string                 `json:"assumptions"`
	EdgeCases   []string                 `json:"edge_cases"`
	Outputs     []string                 `json:"outputs"`
}

type AIDiagram struct {
	Type        string                   `json:"type"` // journey, architecture, state_machine, workflow, timeline, comparison
	Title       string                   `json:"title"`
	Description string                   `json:"description,omitempty"`
	Columns     []map[string]interface{} `json:"columns,omitempty"`     // For journey
	Nodes       []map[string]interface{} `json:"nodes,omitempty"`       // For architecture
	Edges       []map[string]interface{} `json:"edges,omitempty"`       // For architecture
	States      []map[string]interface{} `json:"states,omitempty"`      // For state_machine
	Transitions []map[string]interface{} `json:"transitions,omitempty"` // For state_machine
	Steps       []map[string]interface{} `json:"steps,omitempty"`       // For workflow
	Events      []map[string]interface{} `json:"events,omitempty"`      // For timeline
	Comparison  []map[string]interface{} `json:"comparison,omitempty"`  // For comparison
}
