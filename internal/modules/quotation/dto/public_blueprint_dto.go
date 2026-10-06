package dto

import (
	"encoding/json"
	"time"
)

// ProgrammerBlueprintDTO defines the strict allowlisted response for public programmer sharing.
// All financial, client, prompt, model, and internal revision pointer fields are strictly excluded.
type ProgrammerBlueprintDTO struct {
	Title              string            `json:"title"`
	RevisionNumber     int               `json:"revision_number"`
	CompletenessStatus string            `json:"completeness_status"` // strictly "partial" or "complete"
	PublishedAt        time.Time         `json:"published_at"`
	Summary            string            `json:"summary"`
	Assumptions        []string          `json:"assumptions"`
	OpenQuestions      []string          `json:"open_questions"`
	Journey            json.RawMessage   `json:"journey,omitempty"`
	Architecture       json.RawMessage   `json:"architecture,omitempty"`
	StateMachine       json.RawMessage   `json:"state_machine,omitempty"`
	Workflow           json.RawMessage   `json:"workflow,omitempty"`
	WBS                json.RawMessage   `json:"wbs,omitempty"`
	CompletenessMatrix map[string]string `json:"completeness_matrix"`
	Warnings           []string          `json:"warnings"`
}
