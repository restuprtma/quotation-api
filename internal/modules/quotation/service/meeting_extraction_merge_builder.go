package service

import (
	"encoding/json"
	"fmt"
	"strings"
)

// buildMeetingExtractionMerge converts only typed, validated P1-P4 facts into
// the M7 merger. Provider JSON and source text are never accepted here.
func buildMeetingExtractionMerge(phases MeetingExtractionValidatedPhaseData) MergeExtractionResult {
	candidates := make([]MergeCandidate, 0, 16)
	add := func(path FieldPath, value any, phase PhaseKey, source MergeSource, refs []string) {
		if value == nil {
			return
		}
		candidates = append(candidates, MergeCandidate{
			Path: path, Value: value, Phase: phase, Source: source,
			EvidenceRefs: append([]string(nil), refs...), EvidenceToken: strings.Join(refs, ","), EvidenceValid: true,
		})
	}
	planFact := func(path FieldPath, fact *PlanMetadataFact) {
		if fact == nil {
			return
		}
		source := MergeSourceExtracted
		if fact.Source == PlanSchemeSourceUser {
			source = MergeSourceUser
		}
		add(path, fact.Value, PhaseKeyPlanMetadata, source, fact.Evidence.Refs)
	}
	planFact("client_name", phases.Plan.Metadata.ClientName)
	planFact("client_company", phases.Plan.Metadata.ClientCompany)
	planFact("pic_name", phases.Plan.Metadata.PICName)
	planFact("project_name", phases.Plan.Metadata.ProjectName)
	if phases.Plan.Scheme != "" {
		source := MergeSourceExtracted
		if phases.Plan.SchemeSource == PlanSchemeSourceUser {
			source = MergeSourceUser
		}
		add("commercial_scheme", phases.Plan.Scheme, PhaseKeyPlanMetadata, source, phases.Plan.EvidenceRefs)
	}
	if len(phases.Scope.Scope) > 0 {
		add("scope", phases.Scope.Scope, PhaseKeyScope, MergeSourceExtracted, phases.Scope.EvidenceRefs)
	}
	if phases.Staffing.DurationMonths != nil {
		add("duration_months", phases.Staffing.DurationMonths.Value, PhaseKeyStaffing, MergeSourceExtracted, phases.Staffing.DurationMonths.Evidence.Refs)
	}
	if len(phases.Staffing.Team) > 0 {
		add("team", phases.Staffing.Team, PhaseKeyStaffing, MergeSourceExtracted, phases.Staffing.EvidenceRefs)
	}
	for _, fact := range phases.Service.Facts {
		// P4 is descriptive. Only the safe payment pattern is mapped into the
		// prompt state; service-specific fields remain scheme adapters' input.
		if fact.Key == "payment_label" || fact.Key == "payment_pattern" {
			var text string
			if json.Unmarshal(fact.Value, &text) == nil {
				add("payment_pattern", text, PhaseKeyServiceDetails, MergeSourceExtracted, fact.Evidence.Refs)
			}
		}
	}
	merged, err := MergeExtraction(MergeExtractionInput{
		Phases:         []Phase{NewPlanMetadataPhase(""), NewScopePhase(), NewStaffingPhase(nil), NewServiceDetailsPhase(phases.Service.Scheme)},
		Candidates:     candidates,
		RepairAttempts: map[FieldPath]int{},
	})
	if err != nil {
		return MergeExtractionResult{NeedsInput: true, Warnings: []MergeWarning{{Code: fmt.Sprintf("%s", err)}}}
	}
	return merged
}
