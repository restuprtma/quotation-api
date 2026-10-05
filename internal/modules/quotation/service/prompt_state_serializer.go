package service

import (
	"encoding/json"
	"errors"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrPromptStateSerialization = errors.New("prompt state serialization failed")

// MarshalPromptQuotationStateForPersistence is the only serializer used for
// prompt-state writes and response projections. MeetingNotes is an execution
// input, never persisted state; evidence references remain available for BA
// traceability without retaining source text.
func MarshalPromptQuotationStateForPersistence(state *domain.PromptQuotationState) (json.RawMessage, error) {
	if state == nil {
		return nil, ErrPromptStateSerialization
	}
	clone := clonePromptStateWithoutNotes(*state)
	encoded, err := json.Marshal(clone)
	if err != nil {
		return nil, err
	}
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &wire); err != nil {
		return nil, err
	}
	delete(wire, "meeting_notes")
	return json.Marshal(wire)
}

func clonePromptStateWithoutNotes(state domain.PromptQuotationState) domain.PromptQuotationState {
	state.MeetingNotes = ""
	return state
}
