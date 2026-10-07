package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// BlueprintCrossValidator enforces semantic cross-reference integrity and deterministic coverage.
type BlueprintCrossValidator struct{}

// NewProductionBlueprintCrossValidator constructs a new production BlueprintCrossValidator.
func NewProductionBlueprintCrossValidator() *BlueprintCrossValidator {
	return &BlueprintCrossValidator{}
}

type blueprintSymbolIndex struct {
	canonical map[string]string
	aliases   map[string][]string
}

type blueprintReferenceResolution int

const (
	blueprintReferenceMissing blueprintReferenceResolution = iota
	blueprintReferenceResolved
	blueprintReferenceAmbiguous
)

func newBlueprintSymbolIndex() *blueprintSymbolIndex {
	return &blueprintSymbolIndex{
		canonical: make(map[string]string),
		aliases:   make(map[string][]string),
	}
}

func (idx *blueprintSymbolIndex) addCanonical(canon string) {
	canon = strings.TrimSpace(canon)
	if canon != "" {
		idx.canonical[canon] = canon
	}
}

func (idx *blueprintSymbolIndex) addAlias(alias, canon string) {
	alias = strings.TrimSpace(alias)
	canon = strings.TrimSpace(canon)
	if alias == "" || canon == "" || alias == canon {
		return
	}
	idx.aliases[alias] = append(idx.aliases[alias], canon)
}

func (idx *blueprintSymbolIndex) finalize() {
	for alias, owners := range idx.aliases {
		seen := make(map[string]bool)
		unique := make([]string, 0, len(owners))
		for _, o := range owners {
			if !seen[o] {
				seen[o] = true
				unique = append(unique, o)
			}
		}
		sort.Strings(unique)
		idx.aliases[alias] = unique
	}
}

func (idx *blueprintSymbolIndex) resolve(rawRef string) (blueprintReferenceResolution, string) {
	ref := strings.TrimSpace(rawRef)
	if ref == "" {
		return blueprintReferenceMissing, ""
	}
	if canon, ok := idx.canonical[ref]; ok {
		return blueprintReferenceResolved, canon
	}
	owners, ok := idx.aliases[ref]
	if !ok || len(owners) == 0 {
		return blueprintReferenceMissing, ""
	}
	if len(owners) == 1 {
		return blueprintReferenceResolved, owners[0]
	}
	return blueprintReferenceAmbiguous, ""
}

func buildBlueprintSymbolIndex(bundle BlueprintBundle) *blueprintSymbolIndex {
	idx := newBlueprintSymbolIndex()

	// Journey
	for _, p := range bundle.Journey.Phases {
		id := strings.TrimSpace(p.ID)
		if id != "" {
			canon := "journey.phase." + id
			idx.addCanonical(canon)
			idx.addAlias(id, canon)
		}
	}
	for _, a := range bundle.Journey.Actors {
		id := strings.TrimSpace(a.ID)
		if id != "" {
			canon := "journey.actor." + id
			idx.addCanonical(canon)
			idx.addAlias(id, canon)
		}
	}
	for _, act := range bundle.Journey.Activities {
		id := strings.TrimSpace(act.ID)
		if id != "" {
			canon := "journey.activity." + id
			idx.addCanonical(canon)
			idx.addAlias(id, canon)
		}
	}

	// Architecture
	for _, g := range bundle.Architecture.Groups {
		id := strings.TrimSpace(g.ID)
		if id != "" {
			canon := "architecture.group." + id
			idx.addCanonical(canon)
			idx.addAlias(id, canon)
		}
	}
	for _, n := range bundle.Architecture.Nodes {
		id := strings.TrimSpace(n.ID)
		if id != "" {
			canon := "architecture.node." + id
			idx.addCanonical(canon)
			idx.addAlias(id, canon)
		}
	}
	for _, e := range bundle.Architecture.Edges {
		id := strings.TrimSpace(e.ID)
		if id != "" {
			canon := "architecture.edge." + id
			idx.addCanonical(canon)
			idx.addAlias(id, canon)
		}
	}

	// State Machine
	entity := strings.TrimSpace(bundle.StateMachine.Entity)
	for _, s := range bundle.StateMachine.States {
		id := strings.TrimSpace(s.ID)
		if id != "" {
			var canon string
			if entity != "" {
				canon = "state." + entity + "." + id
				idx.addCanonical(canon)
				idx.addAlias("state."+id, canon)
				idx.addAlias("state_machine.state."+id, canon)
				idx.addAlias("state_machine."+entity+".state."+id, canon)
				idx.addAlias("state."+entity+".state."+id, canon)
				idx.addAlias(id, canon)
			} else {
				canon = "state." + id
				idx.addCanonical(canon)
				idx.addAlias("state_machine.state."+id, canon)
				idx.addAlias(id, canon)
			}
		}
	}
	for _, t := range bundle.StateMachine.Transitions {
		id := strings.TrimSpace(t.ID)
		if id != "" {
			var canon string
			if entity != "" {
				canon = "state." + entity + ".transition." + id
				idx.addCanonical(canon)
				idx.addAlias("state.transition."+id, canon)
				idx.addAlias("state_machine.transition."+id, canon)
				idx.addAlias("state_machine."+entity+".transition."+id, canon)
				idx.addAlias(id, canon)
			} else {
				canon = "state.transition." + id
				idx.addCanonical(canon)
				idx.addAlias("state_machine.transition."+id, canon)
				idx.addAlias(id, canon)
			}
		}
	}

	// Workflows
	for _, wf := range bundle.Workflows {
		wfID := strings.TrimSpace(wf.ID)
		if wfID != "" {
			wfCanon := "workflow." + wfID
			idx.addCanonical(wfCanon)
			idx.addAlias(wfID, wfCanon)

			for _, l := range wf.Lanes {
				laneID := strings.TrimSpace(l.ID)
				if laneID != "" {
					laneCanon := "workflow." + wfID + ".lane." + laneID
					idx.addCanonical(laneCanon)
					idx.addAlias("workflow.lane."+laneID, laneCanon)
					idx.addAlias(laneID, laneCanon)
				}
			}
			for _, node := range wf.Nodes {
				nodeID := strings.TrimSpace(node.ID)
				if nodeID != "" {
					nodeCanon := "workflow." + wfID + ".step." + nodeID
					idx.addCanonical(nodeCanon)
					idx.addAlias("workflow."+wfID+".node."+nodeID, nodeCanon)
					idx.addAlias("workflow.step."+nodeID, nodeCanon)
					idx.addAlias("workflow.node."+nodeID, nodeCanon)
					idx.addAlias("workflow."+nodeID, nodeCanon)
					idx.addAlias(nodeID, nodeCanon)
				}
			}
			for _, edge := range wf.Edges {
				edgeID := strings.TrimSpace(edge.ID)
				if edgeID != "" {
					edgeCanon := "workflow." + wfID + ".edge." + edgeID
					idx.addCanonical(edgeCanon)
					idx.addAlias("workflow.edge."+edgeID, edgeCanon)
					idx.addAlias(edgeID, edgeCanon)
				}
			}
		}
	}

	// WBS
	for _, mod := range bundle.WBS.Modules {
		modID := strings.TrimSpace(mod.ID)
		if modID != "" {
			modCanon := "wbs.module." + modID
			idx.addCanonical(modCanon)
			idx.addAlias(modID, modCanon)

			for _, feat := range mod.Features {
				featID := strings.TrimSpace(feat.ID)
				if featID != "" {
					featCanon := "wbs.module." + modID + ".feature." + featID
					idx.addCanonical(featCanon)
					idx.addAlias("wbs.feature."+featID, featCanon)
					idx.addAlias(featID, featCanon)
				}
			}
		}
	}

	// Open Questions
	for _, q := range bundle.OpenQuestions {
		qID := strings.TrimSpace(q.ID)
		if qID != "" {
			qCanon := "open_questions." + qID
			idx.addCanonical(qCanon)
			idx.addAlias(qID, qCanon)
		}
	}

	// Cross References
	for _, xref := range bundle.CrossReferences {
		xID := strings.TrimSpace(xref.ID)
		if xID != "" {
			xCanon := "cross_references." + xID
			idx.addCanonical(xCanon)
			idx.addAlias(xID, xCanon)
		}
	}

	idx.finalize()
	return idx
}

func roundBlueprintCoverage(numerator, denominator int) float64 {
	if denominator <= 0 || numerator <= 0 {
		return 0.0
	}
	ratio := float64(numerator) / float64(denominator)
	return math.Round(ratio*10000) / 10000
}

func isWorkflowOrStateTarget(canon string) bool {
	if strings.HasPrefix(canon, "workflow.") && (strings.Contains(canon, ".step.") || strings.Contains(canon, ".node.")) {
		return true
	}
	if strings.HasPrefix(canon, "state.") || strings.HasPrefix(canon, "state_machine.") {
		return true
	}
	return false
}

// Validate executes strict cross-section referential integrity checks, semantic warnings, and coverage analysis.
func (v *BlueprintCrossValidator) Validate(bundle BlueprintBundle) BlueprintCrossValidationResult {
	if bundle.SchemaVersion != BlueprintSchemaVersion {
		issues := []BlueprintValidationIssue{
			{
				Code:        BlueprintIssueUnsupportedVersion,
				Severity:    "BLOCKING_ERROR",
				Section:     "cross_validation",
				Path:        "/schema_version",
				Message:     "unsupported schema version",
				Blocking:    true,
				Remediation: "Set schema_version to '1.0'.",
			},
		}
		return BlueprintCrossValidationResult{
			Valid:       false,
			SafeOutcome: BlueprintValidationCrossValidationError,
			Issues:      issues,
			Coverage: BlueprintCoverage{
				JourneyToWorkflow: 0.0,
				ArchitectureToWBS: 0.0,
			},
		}
	}

	idx := buildBlueprintSymbolIndex(bundle)

	var blockingIssues []BlueprintValidationIssue

	// 1. Cross References
	for i, xref := range bundle.CrossReferences {
		path := fmt.Sprintf("/cross_references/%d", i)
		resSrc, _ := idx.resolve(xref.SourceRef)
		if resSrc == blueprintReferenceMissing {
			blockingIssues = append(blockingIssues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     "cross_references",
				Path:        path + "/source_ref",
				TargetID:    xref.ID,
				Message:     "dangling reference does not resolve to any blueprint element",
				Blocking:    true,
				Remediation: "Ensure referenced element exists or remove reference.",
			})
		} else if resSrc == blueprintReferenceAmbiguous {
			blockingIssues = append(blockingIssues, BlueprintValidationIssue{
				Code:        BlueprintIssueAmbiguousReference,
				Severity:    "BLOCKING_ERROR",
				Section:     "cross_references",
				Path:        path + "/source_ref",
				TargetID:    xref.ID,
				Message:     "ambiguous reference matches multiple elements",
				Blocking:    true,
				Remediation: "Use the fully qualified canonical reference URI.",
			})
		}

		resTgt, _ := idx.resolve(xref.TargetRef)
		if resTgt == blueprintReferenceMissing {
			blockingIssues = append(blockingIssues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     "cross_references",
				Path:        path + "/target_ref",
				TargetID:    xref.ID,
				Message:     "dangling reference does not resolve to any blueprint element",
				Blocking:    true,
				Remediation: "Ensure referenced element exists or remove reference.",
			})
		} else if resTgt == blueprintReferenceAmbiguous {
			blockingIssues = append(blockingIssues, BlueprintValidationIssue{
				Code:        BlueprintIssueAmbiguousReference,
				Severity:    "BLOCKING_ERROR",
				Section:     "cross_references",
				Path:        path + "/target_ref",
				TargetID:    xref.ID,
				Message:     "ambiguous reference matches multiple elements",
				Blocking:    true,
				Remediation: "Use the fully qualified canonical reference URI.",
			})
		}
	}

	// 2. Journey Activity References
	for i, act := range bundle.Journey.Activities {
		path := fmt.Sprintf("/journey/activities/%d", i)
		for rIdx, ref := range act.References {
			res, _ := idx.resolve(ref)
			refPath := fmt.Sprintf("%s/references/%d", path, rIdx)
			if res == blueprintReferenceMissing {
				blockingIssues = append(blockingIssues, BlueprintValidationIssue{
					Code:        BlueprintIssueDanglingReference,
					Severity:    "BLOCKING_ERROR",
					Section:     "journey",
					Path:        refPath,
					TargetID:    act.ID,
					Message:     "dangling reference does not resolve to any blueprint element",
					Blocking:    true,
					Remediation: "Ensure referenced element exists or remove reference.",
				})
			} else if res == blueprintReferenceAmbiguous {
				blockingIssues = append(blockingIssues, BlueprintValidationIssue{
					Code:        BlueprintIssueAmbiguousReference,
					Severity:    "BLOCKING_ERROR",
					Section:     "journey",
					Path:        refPath,
					TargetID:    act.ID,
					Message:     "ambiguous reference matches multiple elements",
					Blocking:    true,
					Remediation: "Use the fully qualified canonical reference URI.",
				})
			}
		}
	}

	// 3. WBS Module References
	for i, mod := range bundle.WBS.Modules {
		path := fmt.Sprintf("/wbs/modules/%d", i)
		for rIdx, ref := range mod.References {
			res, _ := idx.resolve(ref)
			refPath := fmt.Sprintf("%s/references/%d", path, rIdx)
			if res == blueprintReferenceMissing {
				blockingIssues = append(blockingIssues, BlueprintValidationIssue{
					Code:        BlueprintIssueDanglingReference,
					Severity:    "BLOCKING_ERROR",
					Section:     "wbs",
					Path:        refPath,
					TargetID:    mod.ID,
					Message:     "dangling reference does not resolve to any blueprint element",
					Blocking:    true,
					Remediation: "Ensure referenced element exists or remove reference.",
				})
			} else if res == blueprintReferenceAmbiguous {
				blockingIssues = append(blockingIssues, BlueprintValidationIssue{
					Code:        BlueprintIssueAmbiguousReference,
					Severity:    "BLOCKING_ERROR",
					Section:     "wbs",
					Path:        refPath,
					TargetID:    mod.ID,
					Message:     "ambiguous reference matches multiple elements",
					Blocking:    true,
					Remediation: "Use the fully qualified canonical reference URI.",
				})
			}
		}
	}

	// 4. Semantic Warnings and Coverage Analysis
	var warningIssues []BlueprintValidationIssue

	// 4a. Empty Core Sections in canonical order
	coreSectionChecks := []struct {
		section string
		path    string
		empty   bool
	}{
		{
			section: "journey",
			path:    "/journey",
			empty:   len(bundle.Journey.Phases) == 0 && len(bundle.Journey.Actors) == 0 && len(bundle.Journey.Activities) == 0,
		},
		{
			section: "architecture",
			path:    "/architecture",
			empty:   len(bundle.Architecture.Groups) == 0 && len(bundle.Architecture.Nodes) == 0 && len(bundle.Architecture.Edges) == 0,
		},
		{
			section: "state_machine",
			path:    "/state_machine",
			empty:   len(bundle.StateMachine.States) == 0 && len(bundle.StateMachine.Transitions) == 0 && strings.TrimSpace(bundle.StateMachine.Entity) == "",
		},
		{
			section: "workflow",
			path:    "/workflows",
			empty:   len(bundle.Workflows) == 0,
		},
		{
			section: "wbs",
			path:    "/wbs",
			empty:   len(bundle.WBS.Modules) == 0,
		},
	}

	for _, check := range coreSectionChecks {
		if check.empty {
			warningIssues = append(warningIssues, BlueprintValidationIssue{
				Code:        BlueprintWarningEmptySection,
				Severity:    "WARNING",
				Section:     check.section,
				Path:        check.path,
				TargetID:    check.section,
				Message:     "core section is empty",
				Blocking:    false,
				Remediation: "Provide content for the section before publication.",
			})
		}
	}

	// 4b. Architecture Node Unreferenced in WBS and Architecture Coverage
	referencedArchNodes := make(map[string]bool)

	for _, mod := range bundle.WBS.Modules {
		for _, ref := range mod.References {
			res, canon := idx.resolve(ref)
			if res == blueprintReferenceResolved && strings.HasPrefix(canon, "architecture.node.") {
				referencedArchNodes[canon] = true
			}
		}
	}

	for _, xref := range bundle.CrossReferences {
		resSrc, srcCanon := idx.resolve(xref.SourceRef)
		resTgt, tgtCanon := idx.resolve(xref.TargetRef)
		if resSrc == blueprintReferenceResolved && resTgt == blueprintReferenceResolved {
			srcIsWBS := strings.HasPrefix(srcCanon, "wbs.module.")
			tgtIsWBS := strings.HasPrefix(tgtCanon, "wbs.module.")
			srcIsArchNode := strings.HasPrefix(srcCanon, "architecture.node.")
			tgtIsArchNode := strings.HasPrefix(tgtCanon, "architecture.node.")

			if srcIsWBS && tgtIsArchNode {
				referencedArchNodes[tgtCanon] = true
			}
			if tgtIsWBS && srcIsArchNode {
				referencedArchNodes[srcCanon] = true
			}
		}
	}

	eligibleArchCount := 0
	referencedEligibleArchCount := 0

	for i, node := range bundle.Architecture.Nodes {
		tLower := strings.ToLower(strings.TrimSpace(node.Type))
		if tLower == "service" || tLower == "application" || tLower == "database" || tLower == "integration" {
			eligibleArchCount++
			nodeCanon := "architecture.node." + strings.TrimSpace(node.ID)
			if referencedArchNodes[nodeCanon] {
				referencedEligibleArchCount++
			} else {
				warningIssues = append(warningIssues, BlueprintValidationIssue{
					Code:        BlueprintWarningArchitectureUncovered,
					Severity:    "WARNING",
					Section:     "architecture",
					Path:        fmt.Sprintf("/architecture/nodes/%d", i),
					TargetID:    node.ID,
					Message:     "architecture node is not referenced in WBS",
					Blocking:    false,
					Remediation: "Reference the architecture node in a WBS module or cross reference.",
				})
			}
		}
	}

	// 4c. Journey Activities Unlinked and Journey Coverage
	totalActivities := len(bundle.Journey.Activities)
	linkedActivities := 0

	for i, act := range bundle.Journey.Activities {
		actCanon := "journey.activity." + strings.TrimSpace(act.ID)
		linked := false

		for _, ref := range act.References {
			res, canon := idx.resolve(ref)
			if res == blueprintReferenceResolved && isWorkflowOrStateTarget(canon) {
				linked = true
				break
			}
		}

		if !linked {
			for _, xref := range bundle.CrossReferences {
				resSrc, srcCanon := idx.resolve(xref.SourceRef)
				resTgt, tgtCanon := idx.resolve(xref.TargetRef)
				if resSrc == blueprintReferenceResolved && resTgt == blueprintReferenceResolved {
					if srcCanon == actCanon && isWorkflowOrStateTarget(tgtCanon) {
						linked = true
						break
					}
					if tgtCanon == actCanon && isWorkflowOrStateTarget(srcCanon) {
						linked = true
						break
					}
				}
			}
		}

		if linked {
			linkedActivities++
		} else {
			warningIssues = append(warningIssues, BlueprintValidationIssue{
				Code:        BlueprintWarningJourneyUnlinked,
				Severity:    "WARNING",
				Section:     "journey",
				Path:        fmt.Sprintf("/journey/activities/%d", i),
				TargetID:    act.ID,
				Message:     "journey activity is not linked to any workflow node or state transition",
				Blocking:    false,
				Remediation: "Link activity to a workflow step or state transition.",
			})
		}
	}

	coverage := BlueprintCoverage{
		JourneyToWorkflow: roundBlueprintCoverage(linkedActivities, totalActivities),
		ArchitectureToWBS: roundBlueprintCoverage(referencedEligibleArchCount, eligibleArchCount),
	}

	allIssues := append(blockingIssues, warningIssues...)
	sortBlueprintValidationIssues(allIssues)

	if allIssues == nil {
		allIssues = []BlueprintValidationIssue{}
	}

	valid := len(blockingIssues) == 0
	safeOutcome := ""
	if !valid {
		safeOutcome = BlueprintValidationCrossValidationError
	}

	return BlueprintCrossValidationResult{
		Valid:       valid,
		SafeOutcome: safeOutcome,
		Issues:      allIssues,
		Coverage:    coverage,
	}
}

type rawStateMachinePayload struct {
	States []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Transitions []string `json:"transitions"`
	} `json:"states"`
	Transitions []struct {
		ID   string `json:"id"`
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"transitions"`
}

// ValidateStateMachineTransitions validates raw state machine transitions and detects potential cyclic flows.
func (c *BlueprintCrossValidator) ValidateStateMachineTransitions(rawStateMachine []byte) ([]string, error) {
	var payload rawStateMachinePayload
	if err := json.Unmarshal(rawStateMachine, &payload); err != nil {
		return nil, fmt.Errorf("invalid state machine json: %w", err)
	}

	adj := make(map[string][]string)
	for _, s := range payload.States {
		if _, exists := adj[s.ID]; !exists {
			adj[s.ID] = nil
		}
		for _, t := range s.Transitions {
			adj[s.ID] = append(adj[s.ID], t)
		}
	}
	for _, t := range payload.Transitions {
		if t.From != "" && t.To != "" {
			adj[t.From] = append(adj[t.From], t.To)
		}
	}

	var warnings []string
	visited := make(map[string]int) // 0: unvisited, 1: visiting, 2: visited

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = 1
		currentPath := append(path, node)

		for _, neighbor := range adj[node] {
			if visited[neighbor] == 1 {
				warnings = append(warnings, fmt.Sprintf("cyclic transition flow detected: %s -> %s", strings.Join(currentPath, " -> "), neighbor))
			} else if visited[neighbor] == 0 {
				dfs(neighbor, currentPath)
			}
		}
		visited[node] = 2
	}

	stateIDs := make([]string, 0, len(adj))
	for id := range adj {
		stateIDs = append(stateIDs, id)
	}
	sort.Strings(stateIDs)

	for _, id := range stateIDs {
		if visited[id] == 0 {
			dfs(id, nil)
		}
	}

	return warnings, nil
}
