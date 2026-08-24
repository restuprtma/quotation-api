package rnd

import (
	"fmt"
	"strings"
)

// BlueprintCrossValidator enforces semantic cross-reference integrity and non-blocking warnings across blueprint sections.
type BlueprintCrossValidator struct{}

// NewBlueprintCrossValidator creates a new instance of BlueprintCrossValidator.
func NewBlueprintCrossValidator() *BlueprintCrossValidator {
	return &BlueprintCrossValidator{}
}

func createCrossWarningIssue(code, section, path, targetID, message, remediation string, context map[string]interface{}) ValidationIssue {
	return ValidationIssue{
		Code:                 code,
		Severity:             SeverityWarning,
		Section:              section,
		Path:                 path,
		TargetID:             targetID,
		Message:              message,
		Blocking:             false,
		ConfirmationRequired: true,
		Remediation:          remediation,
		Context:              context,
	}
}

func createCrossBlockingIssue(code, section, path, targetID, message, remediation string, context map[string]interface{}) ValidationIssue {
	return ValidationIssue{
		Code:                 code,
		Severity:             SeverityBlockingError,
		Section:              section,
		Path:                 path,
		TargetID:             targetID,
		Message:              message,
		Blocking:             true,
		ConfirmationRequired: false,
		Remediation:          remediation,
		Context:              context,
	}
}

func lastSegment(s string) string {
	parts := strings.Split(s, ".")
	return parts[len(parts)-1]
}

func normalizeTerminology(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"actor-", "node-", "grp-", "lane-", "wf-", "phase-", "act-", "mod-", "feat-", "oq-", "xref-"} {
		s = strings.TrimPrefix(s, p)
	}
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// CrossValidate performs semantic cross-section referential and linting checks.
func (v *BlueprintCrossValidator) CrossValidate(bundle *BlueprintBundle, input *BlueprintInput) []ValidationIssue {
	var issues []ValidationIssue

	if bundle == nil {
		coreSections := []string{"journey", "architecture", "state_machine", "workflows", "wbs"}
		for _, sec := range coreSections {
			issues = append(issues, createCrossWarningIssue(
				WarnEmptySection,
				sec,
				"/"+sec,
				sec,
				fmt.Sprintf("Core section '%s' is empty.", sec),
				fmt.Sprintf("Generate or complete the '%s' section before final publication.", sec),
				map[string]interface{}{"section": sec},
			))
		}
		return issues
	}

	// 1. Check Empty Core Sections (WARN_EMPTY_SECTION)
	if len(bundle.Journey.Phases) == 0 && len(bundle.Journey.Actors) == 0 && len(bundle.Journey.Activities) == 0 {
		issues = append(issues, createCrossWarningIssue(
			WarnEmptySection,
			"journey",
			"/journey",
			"journey",
			"Core section 'journey' is empty.",
			"Generate or complete the 'journey' section before final publication.",
			map[string]interface{}{"section": "journey"},
		))
	}

	if len(bundle.Architecture.Groups) == 0 && len(bundle.Architecture.Nodes) == 0 && len(bundle.Architecture.Edges) == 0 {
		issues = append(issues, createCrossWarningIssue(
			WarnEmptySection,
			"architecture",
			"/architecture",
			"architecture",
			"Core section 'architecture' is empty.",
			"Generate or complete the 'architecture' section before final publication.",
			map[string]interface{}{"section": "architecture"},
		))
	}

	if len(bundle.StateMachine.States) == 0 && len(bundle.StateMachine.Transitions) == 0 && strings.TrimSpace(bundle.StateMachine.Entity) == "" {
		issues = append(issues, createCrossWarningIssue(
			WarnEmptySection,
			"state_machine",
			"/state_machine",
			"state_machine",
			"Core section 'state_machine' is empty.",
			"Generate or complete the 'state_machine' section before final publication.",
			map[string]interface{}{"section": "state_machine"},
		))
	}

	if len(bundle.Workflows) == 0 {
		issues = append(issues, createCrossWarningIssue(
			WarnEmptySection,
			"workflows",
			"/workflows",
			"workflows",
			"Core section 'workflows' is empty.",
			"Generate or complete the 'workflows' section before final publication.",
			map[string]interface{}{"section": "workflows"},
		))
	}

	if len(bundle.WBS.Modules) == 0 {
		issues = append(issues, createCrossWarningIssue(
			WarnEmptySection,
			"wbs",
			"/wbs",
			"wbs",
			"Core section 'wbs' is empty.",
			"Generate or complete the 'wbs' section before final publication.",
			map[string]interface{}{"section": "wbs"},
		))
	}

	// 2. Build Global Symbol Table and Raw ID Catalog
	symbolTable := make(map[string]bool)
	rawIDs := make(map[string]string) // rawID -> entity type

	// Journey symbols
	for _, p := range bundle.Journey.Phases {
		if strings.TrimSpace(p.ID) != "" {
			rawIDs[p.ID] = "journey.phase"
			symbolTable["journey.phase."+p.ID] = true
			symbolTable[p.ID] = true
		}
	}
	for _, a := range bundle.Journey.Actors {
		if strings.TrimSpace(a.ID) != "" {
			rawIDs[a.ID] = "journey.actor"
			symbolTable["journey.actor."+a.ID] = true
			symbolTable[a.ID] = true
		}
	}
	for _, act := range bundle.Journey.Activities {
		if strings.TrimSpace(act.ID) != "" {
			rawIDs[act.ID] = "journey.activity"
			symbolTable["journey.activity."+act.ID] = true
			symbolTable[act.ID] = true
		}
	}

	// Architecture symbols
	for _, g := range bundle.Architecture.Groups {
		if strings.TrimSpace(g.ID) != "" {
			rawIDs[g.ID] = "architecture.group"
			symbolTable["architecture.group."+g.ID] = true
			symbolTable[g.ID] = true
		}
	}
	for _, n := range bundle.Architecture.Nodes {
		if strings.TrimSpace(n.ID) != "" {
			rawIDs[n.ID] = "architecture.node"
			symbolTable["architecture.node."+n.ID] = true
			symbolTable[n.ID] = true
		}
	}
	for _, e := range bundle.Architecture.Edges {
		if strings.TrimSpace(e.ID) != "" {
			rawIDs[e.ID] = "architecture.edge"
			symbolTable["architecture.edge."+e.ID] = true
			symbolTable[e.ID] = true
		}
	}

	// State machine symbols
	entity := strings.TrimSpace(bundle.StateMachine.Entity)
	if entity != "" {
		rawIDs[entity] = "state.entity"
		symbolTable["state_machine.entity"] = true
		symbolTable["state.entity"] = true
	}
	for _, s := range bundle.StateMachine.States {
		if strings.TrimSpace(s.ID) != "" {
			rawIDs[s.ID] = "state"
			symbolTable["state."+s.ID] = true
			symbolTable["state_machine.state."+s.ID] = true
			symbolTable[s.ID] = true
			if entity != "" {
				symbolTable["state."+entity+"."+s.ID] = true
				symbolTable["state_machine."+entity+".state."+s.ID] = true
				symbolTable["state."+entity+".state."+s.ID] = true
			}
		}
	}
	for _, t := range bundle.StateMachine.Transitions {
		if strings.TrimSpace(t.ID) != "" {
			rawIDs[t.ID] = "state.transition"
			symbolTable["state.transition."+t.ID] = true
			symbolTable["state_machine.transition."+t.ID] = true
			symbolTable[t.ID] = true
			if entity != "" {
				symbolTable["state."+entity+".transition."+t.ID] = true
				symbolTable["state_machine."+entity+".transition."+t.ID] = true
			}
		}
	}

	// Workflow symbols
	for _, wf := range bundle.Workflows {
		if strings.TrimSpace(wf.ID) != "" {
			rawIDs[wf.ID] = "workflow"
			symbolTable["workflow."+wf.ID] = true
			symbolTable[wf.ID] = true
		}
		for _, l := range wf.Lanes {
			if strings.TrimSpace(l.ID) != "" {
				rawIDs[l.ID] = "workflow.lane"
				symbolTable["workflow."+wf.ID+".lane."+l.ID] = true
				symbolTable["workflow.lane."+l.ID] = true
				symbolTable[l.ID] = true
			}
		}
		for _, n := range wf.Nodes {
			if strings.TrimSpace(n.ID) != "" {
				rawIDs[n.ID] = "workflow.step"
				symbolTable["workflow."+wf.ID+".step."+n.ID] = true
				symbolTable["workflow."+wf.ID+".node."+n.ID] = true
				symbolTable["workflow.step."+n.ID] = true
				symbolTable["workflow."+n.ID] = true
				symbolTable[n.ID] = true
			}
		}
		for _, e := range wf.Edges {
			if strings.TrimSpace(e.ID) != "" {
				rawIDs[e.ID] = "workflow.edge"
				symbolTable["workflow."+wf.ID+".edge."+e.ID] = true
				symbolTable["workflow.edge."+e.ID] = true
				symbolTable[e.ID] = true
			}
		}
	}

	// WBS symbols
	for _, m := range bundle.WBS.Modules {
		if strings.TrimSpace(m.ID) != "" {
			rawIDs[m.ID] = "wbs.module"
			symbolTable["wbs.module."+m.ID] = true
			symbolTable[m.ID] = true
		}
		for _, f := range m.Features {
			if strings.TrimSpace(f.ID) != "" {
				rawIDs[f.ID] = "wbs.feature"
				symbolTable["wbs.module."+m.ID+".feature."+f.ID] = true
				symbolTable["wbs.feature."+f.ID] = true
				symbolTable[f.ID] = true
			}
		}
	}

	// Open Questions
	for _, q := range bundle.OpenQuestions {
		if strings.TrimSpace(q.ID) != "" {
			rawIDs[q.ID] = "open_questions"
			symbolTable["open_questions."+q.ID] = true
			symbolTable[q.ID] = true
		}
	}

	// Cross References
	for _, x := range bundle.CrossReferences {
		if strings.TrimSpace(x.ID) != "" {
			rawIDs[x.ID] = "cross_references"
			symbolTable["cross_references."+x.ID] = true
			symbolTable[x.ID] = true
		}
	}

	resolveRef := func(ref string) bool {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return false
		}
		if symbolTable[ref] {
			return true
		}
		if rawIDs[ref] != "" {
			return true
		}
		last := lastSegment(ref)
		return rawIDs[last] != ""
	}

	// 3. Check Dangling References (ERR_DANGLING_REFERENCE)
	// 3a. Cross references
	for i, xref := range bundle.CrossReferences {
		path := fmt.Sprintf("/cross_references/%d", i)
		if !resolveRef(xref.SourceRef) {
			issues = append(issues, createCrossBlockingIssue(
				ErrDanglingReference,
				"cross_references",
				path+"/source_ref",
				xref.ID,
				fmt.Sprintf("Cross reference '%s' source_ref '%s' does not resolve to any blueprint element.", xref.ID, xref.SourceRef),
				"Ensure source_ref points to an existing registered blueprint element.",
				map[string]interface{}{"xref_id": xref.ID, "source_ref": xref.SourceRef},
			))
		}
		if !resolveRef(xref.TargetRef) {
			issues = append(issues, createCrossBlockingIssue(
				ErrDanglingReference,
				"cross_references",
				path+"/target_ref",
				xref.ID,
				fmt.Sprintf("Cross reference '%s' target_ref '%s' does not resolve to any blueprint element.", xref.ID, xref.TargetRef),
				"Ensure target_ref points to an existing registered blueprint element.",
				map[string]interface{}{"xref_id": xref.ID, "target_ref": xref.TargetRef},
			))
		}
	}

	// 3b. Journey activity references
	for i, act := range bundle.Journey.Activities {
		path := fmt.Sprintf("/journey/activities/%d", i)
		for rIdx, ref := range act.References {
			if !resolveRef(ref) {
				issues = append(issues, createCrossBlockingIssue(
					ErrDanglingReference,
					"journey",
					fmt.Sprintf("%s/references/%d", path, rIdx),
					act.ID,
					fmt.Sprintf("Journey activity '%s' reference '%s' does not resolve to any blueprint element.", act.ID, ref),
					"Ensure reference points to an existing registered blueprint element.",
					map[string]interface{}{"activity_id": act.ID, "reference": ref},
				))
			}
		}
	}

	// 3c. WBS module references
	for i, mod := range bundle.WBS.Modules {
		path := fmt.Sprintf("/wbs/modules/%d", i)
		for rIdx, ref := range mod.References {
			if !resolveRef(ref) {
				issues = append(issues, createCrossBlockingIssue(
					ErrDanglingReference,
					"wbs",
					fmt.Sprintf("%s/references/%d", path, rIdx),
					mod.ID,
					fmt.Sprintf("WBS module '%s' reference '%s' does not resolve to any blueprint element.", mod.ID, ref),
					"Ensure reference points to an existing registered blueprint element.",
					map[string]interface{}{"module_id": mod.ID, "reference": ref},
				))
			}
		}
	}

	// 4. Check Unreferenced Architecture Nodes in WBS (WARN_ARCH_NODE_UNREFERENCED_IN_WBS)
	referencedArchNodes := make(map[string]bool)
	for _, mod := range bundle.WBS.Modules {
		for _, ref := range mod.References {
			referencedArchNodes[ref] = true
			referencedArchNodes[lastSegment(ref)] = true
		}
	}
	for _, xref := range bundle.CrossReferences {
		srcLast := lastSegment(xref.SourceRef)
		tgtLast := lastSegment(xref.TargetRef)
		srcType := rawIDs[srcLast]
		tgtType := rawIDs[tgtLast]

		srcIsWBS := strings.HasPrefix(xref.SourceRef, "wbs.") || strings.HasPrefix(srcType, "wbs")
		tgtIsWBS := strings.HasPrefix(xref.TargetRef, "wbs.") || strings.HasPrefix(tgtType, "wbs")
		srcIsArch := strings.HasPrefix(xref.SourceRef, "architecture.node.") || srcType == "architecture.node"
		tgtIsArch := strings.HasPrefix(xref.TargetRef, "architecture.node.") || tgtType == "architecture.node"

		if srcIsWBS && tgtIsArch {
			referencedArchNodes[xref.TargetRef] = true
			referencedArchNodes[tgtLast] = true
		}
		if tgtIsWBS && srcIsArch {
			referencedArchNodes[xref.SourceRef] = true
			referencedArchNodes[srcLast] = true
		}
	}

	for i, node := range bundle.Architecture.Nodes {
		nodeType := strings.ToLower(strings.TrimSpace(node.Type))
		if nodeType == "service" || nodeType == "application" || nodeType == "database" || nodeType == "integration" {
			if !referencedArchNodes[node.ID] && !referencedArchNodes["architecture.node."+node.ID] {
				issues = append(issues, createCrossWarningIssue(
					WarnArchNodeUnreferencedInWBS,
					"architecture",
					fmt.Sprintf("/architecture/nodes/%d", i),
					node.ID,
					fmt.Sprintf("Architecture node '%s' (type: '%s') is not referenced by any WBS module or feature.", node.ID, node.Type),
					fmt.Sprintf("Ensure this %s component is covered in a WBS module's technical scope or references, or confirm that it is an implicit shared resource.", node.Type),
					map[string]interface{}{"node_id": node.ID, "node_type": node.Type},
				))
			}
		}
	}

	// 5. Check Unlinked Journey Activities (WARN_JOURNEY_ACTIVITY_UNLINKED)
	isWorkflowOrStateTarget := func(ref string) bool {
		ref = strings.TrimSpace(ref)
		if strings.HasPrefix(ref, "workflow.") || strings.HasPrefix(ref, "state.") || strings.HasPrefix(ref, "state_machine.") {
			return true
		}
		last := lastSegment(ref)
		t := rawIDs[last]
		return t == "workflow.step" || t == "workflow" || t == "state" || t == "state.transition"
	}

	for i, act := range bundle.Journey.Activities {
		linked := false
		for _, ref := range act.References {
			if isWorkflowOrStateTarget(ref) {
				linked = true
				break
			}
		}

		if !linked {
			for _, xref := range bundle.CrossReferences {
				srcLast := lastSegment(xref.SourceRef)
				tgtLast := lastSegment(xref.TargetRef)
				if srcLast == act.ID && isWorkflowOrStateTarget(xref.TargetRef) {
					linked = true
					break
				}
				if tgtLast == act.ID && isWorkflowOrStateTarget(xref.SourceRef) {
					linked = true
					break
				}
			}
		}

		if !linked {
			issues = append(issues, createCrossWarningIssue(
				WarnJourneyActivityUnlinked,
				"journey",
				fmt.Sprintf("/journey/activities/%d", i),
				act.ID,
				fmt.Sprintf("Journey activity '%s' is not linked to any workflow step or state transition.", act.ID),
				"Link activity to a workflow step or state transition via references/cross_references, or confirm as an offline/manual touchpoint.",
				map[string]interface{}{"activity_id": act.ID, "label": act.Label},
			))
		}
	}

	// 6. Check Terminology Mismatch (WARN_TERMINOLOGY_MISMATCH)
	if input != nil {
		// 6a. Check Input Actors against Journey Actors & Workflow Swimlanes
		var actorCandidates []string
		for _, a := range bundle.Journey.Actors {
			actorCandidates = append(actorCandidates, a.Label, a.ID)
		}
		for _, wf := range bundle.Workflows {
			for _, l := range wf.Lanes {
				actorCandidates = append(actorCandidates, l.Label, l.ID)
			}
		}

		for idx, inputActor := range input.Actors {
			normInput := normalizeTerminology(inputActor)
			if normInput == "" {
				continue
			}
			matched := false
			for _, c := range actorCandidates {
				normC := normalizeTerminology(c)
				if normC == "" {
					continue
				}
				if normInput == normC || strings.Contains(normC, normInput) || strings.Contains(normInput, normC) {
					matched = true
					break
				}
			}

			if !matched {
				issues = append(issues, createCrossWarningIssue(
					WarnTerminologyMismatch,
					"journey",
					fmt.Sprintf("/input/actors/%d", idx),
					inputActor,
					fmt.Sprintf("Input actor '%s' is not represented in journey actors or workflow swimlanes.", inputActor),
					fmt.Sprintf("Align journey actor or swimlane names with input actor '%s', or confirm intentional terminology difference.", inputActor),
					map[string]interface{}{"actor": inputActor, "type": "actor"},
				))
			}
		}

		// 6b. Check Input Channels against Architecture Nodes/Groups & Journey Touchpoints
		var channelCandidates []string
		for _, n := range bundle.Architecture.Nodes {
			channelCandidates = append(channelCandidates, n.Label, n.ID)
		}
		for _, g := range bundle.Architecture.Groups {
			channelCandidates = append(channelCandidates, g.Label, g.ID)
		}
		for _, act := range bundle.Journey.Activities {
			channelCandidates = append(channelCandidates, act.Touchpoints...)
		}

		for idx, inputChannel := range input.Channels {
			normInput := normalizeTerminology(inputChannel)
			if normInput == "" {
				continue
			}
			matched := false
			for _, c := range channelCandidates {
				normC := normalizeTerminology(c)
				if normC == "" {
					continue
				}
				if normInput == normC || strings.Contains(normC, normInput) || strings.Contains(normInput, normC) {
					matched = true
					break
				}
			}

			if !matched {
				issues = append(issues, createCrossWarningIssue(
					WarnTerminologyMismatch,
					"architecture",
					fmt.Sprintf("/input/channels/%d", idx),
					inputChannel,
					fmt.Sprintf("Input channel '%s' is not represented in architecture channel nodes, groups, or journey touchpoints.", inputChannel),
					fmt.Sprintf("Add architecture channel node/group for '%s', or confirm intentional terminology difference.", inputChannel),
					map[string]interface{}{"channel": inputChannel, "type": "channel"},
				))
			}
		}
	}

	// 7. Check Unresolved Open Questions (WARN_OPEN_QUESTION_UNRESOLVED)
	for i, q := range bundle.OpenQuestions {
		if strings.ToLower(strings.TrimSpace(q.Status)) == "open" {
			issues = append(issues, createCrossWarningIssue(
				WarnOpenQuestionUnresolved,
				"open_questions",
				fmt.Sprintf("/open_questions/%d", i),
				q.ID,
				fmt.Sprintf("Open question '%s' is unresolved with status 'open'.", q.ID),
				"Resolve the inquiry with client stakeholders or mark status as 'answered' or 'deferred'.",
				map[string]interface{}{
					"question_id":    q.ID,
					"target_section": q.TargetSection,
					"impact":         q.Impact,
					"status":         q.Status,
				},
			))
		}
	}

	return issues
}
