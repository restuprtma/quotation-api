package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type revisionBundle struct {
	SchemaVersion   string                     `json:"schema_version"`
	Title           string                     `json:"title"`
	Summary         string                     `json:"summary"`
	Journey         *BlueprintJourney          `json:"journey,omitempty"`
	Architecture    *BlueprintArchitecture     `json:"architecture,omitempty"`
	StateMachine    *BlueprintStateMachine     `json:"state_machine,omitempty"`
	Workflows       *[]BlueprintWorkflow       `json:"workflows,omitempty"`
	WBS             *BlueprintWBS              `json:"wbs,omitempty"`
	Assumptions     []string                   `json:"assumptions"`
	OpenQuestions   []BlueprintOpenQuestion    `json:"open_questions"`
	CrossReferences *[]BlueprintCrossReference `json:"cross_references,omitempty"`
}

// MergeBlueprintSection merges a normalized section into the revision bundle content.
// It accepts only the 5 provider sections: journey, architecture, state_machine, workflow, and wbs.
func MergeBlueprintSection(
	current json.RawMessage,
	title string,
	contextContent json.RawMessage,
	sectionType domain.BlueprintSectionType,
	normalizedSection json.RawMessage,
) (json.RawMessage, error) {
	switch sectionType {
	case domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS:
	default:
		return nil, fmt.Errorf("invalid section type for merge: %s", sectionType)
	}

	rb := revisionBundle{
		SchemaVersion: BlueprintSchemaVersion,
		Assumptions:   make([]string, 0),
		OpenQuestions: make([]BlueprintOpenQuestion, 0),
	}

	trimmedCurrent := bytes.TrimSpace(current)
	if len(trimmedCurrent) > 0 && string(trimmedCurrent) != "{}" && string(trimmedCurrent) != "null" {
		dec := json.NewDecoder(bytes.NewReader(trimmedCurrent))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rb); err != nil {
			return nil, fmt.Errorf("malformed current content: %w", err)
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, errors.New("trailing tokens in current content")
		}
		if rb.SchemaVersion != "" && rb.SchemaVersion != BlueprintSchemaVersion {
			return nil, fmt.Errorf("unsupported schema version: %s", rb.SchemaVersion)
		}
		if rb.Assumptions == nil {
			rb.Assumptions = make([]string, 0)
		}
		if rb.OpenQuestions == nil {
			rb.OpenQuestions = make([]BlueprintOpenQuestion, 0)
		}
	}

	if len(bytes.TrimSpace(contextContent)) > 0 && json.Valid(contextContent) {
		var ctxMeta struct {
			Title   string `json:"title"`
			Summary string `json:"summary"`
		}
		_ = json.Unmarshal(contextContent, &ctxMeta)
		if rb.Title == "" && ctxMeta.Title != "" {
			rb.Title = ctxMeta.Title
		}
		if rb.Summary == "" && ctxMeta.Summary != "" {
			rb.Summary = ctxMeta.Summary
		}
	}
	if rb.Title == "" {
		rb.Title = title
	}
	if rb.SchemaVersion == "" {
		rb.SchemaVersion = BlueprintSchemaVersion
	}

	secDec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(normalizedSection)))
	secDec.DisallowUnknownFields()

	switch sectionType {
	case domain.BlueprintSectionJourney:
		var env struct {
			Journey *BlueprintJourney `json:"journey"`
		}
		if err := secDec.Decode(&env); err != nil || env.Journey == nil {
			return nil, fmt.Errorf("invalid normalized journey: %w", err)
		}
		if _, err := secDec.Token(); err != io.EOF {
			return nil, errors.New("trailing tokens in normalized journey")
		}
		rb.Journey = env.Journey

	case domain.BlueprintSectionArchitecture:
		var env struct {
			Architecture *BlueprintArchitecture `json:"architecture"`
		}
		if err := secDec.Decode(&env); err != nil || env.Architecture == nil {
			return nil, fmt.Errorf("invalid normalized architecture: %w", err)
		}
		if _, err := secDec.Token(); err != io.EOF {
			return nil, errors.New("trailing tokens in normalized architecture")
		}
		rb.Architecture = env.Architecture

	case domain.BlueprintSectionStateMachine:
		var env struct {
			StateMachine *BlueprintStateMachine `json:"state_machine"`
		}
		if err := secDec.Decode(&env); err != nil || env.StateMachine == nil {
			return nil, fmt.Errorf("invalid normalized state machine: %w", err)
		}
		if _, err := secDec.Token(); err != io.EOF {
			return nil, errors.New("trailing tokens in normalized state machine")
		}
		rb.StateMachine = env.StateMachine

	case domain.BlueprintSectionWorkflow:
		var env struct {
			Workflows *[]BlueprintWorkflow `json:"workflows"`
		}
		if err := secDec.Decode(&env); err != nil || env.Workflows == nil {
			return nil, fmt.Errorf("invalid normalized workflows: %w", err)
		}
		if _, err := secDec.Token(); err != io.EOF {
			return nil, errors.New("trailing tokens in normalized workflows")
		}
		rb.Workflows = env.Workflows

	case domain.BlueprintSectionWBS:
		var env struct {
			WBS             *BlueprintWBS              `json:"wbs"`
			CrossReferences *[]BlueprintCrossReference `json:"cross_references"`
		}
		if err := secDec.Decode(&env); err != nil || env.WBS == nil {
			return nil, fmt.Errorf("invalid normalized wbs: %w", err)
		}
		if _, err := secDec.Token(); err != io.EOF {
			return nil, errors.New("trailing tokens in normalized wbs")
		}
		rb.WBS = env.WBS
		rb.CrossReferences = env.CrossReferences
	}

	marshaled, err := json.Marshal(rb)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged bundle: %w", err)
	}
	return json.RawMessage(marshaled), nil
}

// InvalidateBlueprintDownstream removes sections derived from the target section,
// returning the cleaned content and the exact canonical list of invalidated sections.
func InvalidateBlueprintDownstream(
	current json.RawMessage,
	sectionType domain.BlueprintSectionType,
) (json.RawMessage, []domain.BlueprintSectionType, error) {
	var invalidated []domain.BlueprintSectionType

	switch sectionType {
	case domain.BlueprintSectionJourney:
		invalidated = []domain.BlueprintSectionType{
			domain.BlueprintSectionArchitecture,
			domain.BlueprintSectionStateMachine,
			domain.BlueprintSectionWorkflow,
			domain.BlueprintSectionWBS,
			domain.BlueprintSectionCrossValidation,
		}
	case domain.BlueprintSectionArchitecture:
		invalidated = []domain.BlueprintSectionType{
			domain.BlueprintSectionStateMachine,
			domain.BlueprintSectionWorkflow,
			domain.BlueprintSectionWBS,
			domain.BlueprintSectionCrossValidation,
		}
	case domain.BlueprintSectionStateMachine:
		invalidated = []domain.BlueprintSectionType{
			domain.BlueprintSectionWorkflow,
			domain.BlueprintSectionWBS,
			domain.BlueprintSectionCrossValidation,
		}
	case domain.BlueprintSectionWorkflow:
		invalidated = []domain.BlueprintSectionType{
			domain.BlueprintSectionWBS,
			domain.BlueprintSectionCrossValidation,
		}
	case domain.BlueprintSectionWBS:
		invalidated = []domain.BlueprintSectionType{
			domain.BlueprintSectionCrossValidation,
		}
	default:
		return nil, nil, fmt.Errorf("invalid section type for downstream invalidation: %s", sectionType)
	}

	rb := revisionBundle{
		SchemaVersion: BlueprintSchemaVersion,
		Assumptions:   make([]string, 0),
		OpenQuestions: make([]BlueprintOpenQuestion, 0),
	}

	trimmedCurrent := bytes.TrimSpace(current)
	if len(trimmedCurrent) > 0 && string(trimmedCurrent) != "{}" && string(trimmedCurrent) != "null" {
		dec := json.NewDecoder(bytes.NewReader(trimmedCurrent))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&rb); err != nil {
			return nil, nil, fmt.Errorf("malformed current content: %w", err)
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil, nil, errors.New("trailing tokens in current content")
		}
		if rb.SchemaVersion != "" && rb.SchemaVersion != BlueprintSchemaVersion {
			return nil, nil, fmt.Errorf("unsupported schema version: %s", rb.SchemaVersion)
		}
		if rb.Assumptions == nil {
			rb.Assumptions = make([]string, 0)
		}
		if rb.OpenQuestions == nil {
			rb.OpenQuestions = make([]BlueprintOpenQuestion, 0)
		}
	}

	switch sectionType {
	case domain.BlueprintSectionJourney:
		rb.Architecture = nil
		rb.StateMachine = nil
		rb.Workflows = nil
		rb.WBS = nil
		rb.CrossReferences = nil
	case domain.BlueprintSectionArchitecture:
		rb.StateMachine = nil
		rb.Workflows = nil
		rb.WBS = nil
		rb.CrossReferences = nil
	case domain.BlueprintSectionStateMachine:
		rb.Workflows = nil
		rb.WBS = nil
		rb.CrossReferences = nil
	case domain.BlueprintSectionWorkflow:
		rb.WBS = nil
		rb.CrossReferences = nil
	case domain.BlueprintSectionWBS:
		// WBS and CrossReferences belong to WBS stage, which is preserved.
	}

	marshaled, err := json.Marshal(rb)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal invalidated bundle: %w", err)
	}
	return json.RawMessage(marshaled), invalidated, nil
}

// DecodeBlueprintBundle decodes and strictly validates a complete blueprint bundle.
func DecodeBlueprintBundle(content json.RawMessage) (BlueprintBundle, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return BlueprintBundle{}, errors.New("empty blueprint bundle content")
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()

	var bundle BlueprintBundle
	if err := dec.Decode(&bundle); err != nil {
		return BlueprintBundle{}, fmt.Errorf("malformed blueprint bundle: %w", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return BlueprintBundle{}, errors.New("trailing tokens in blueprint bundle")
	}
	if bundle.SchemaVersion != BlueprintSchemaVersion {
		return BlueprintBundle{}, fmt.Errorf("unsupported blueprint schema version: %q, expected %q", bundle.SchemaVersion, BlueprintSchemaVersion)
	}

	return bundle, nil
}
