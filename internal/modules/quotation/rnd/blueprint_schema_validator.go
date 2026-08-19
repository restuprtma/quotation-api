package rnd

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

var (
	canonicalRefPattern = regexp.MustCompile(`^(journey|architecture|state|state_machine|workflow|wbs|open_questions|cross_references)(\.[a-z0-9_-]+)+$`)
)

// BlueprintSchemaValidator enforces structural validation and referential integrity across blueprint sections and bundles.
type BlueprintSchemaValidator struct {
	canonicalRefRegex *regexp.Regexp
}

// NewBlueprintSchemaValidator creates a new instance of BlueprintSchemaValidator.
func NewBlueprintSchemaValidator() *BlueprintSchemaValidator {
	return &BlueprintSchemaValidator{
		canonicalRefRegex: canonicalRefPattern,
	}
}

func createBlockingIssue(code, section, path, targetID, message, remediation string, context map[string]interface{}) ValidationIssue {
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

func isBlank(s string) bool {
	return strings.TrimSpace(s) == ""
}

func isMalformedLayout(layout LayoutCoordinate) bool {
	return math.IsNaN(layout.X) || math.IsInf(layout.X, 0) || math.IsNaN(layout.Y) || math.IsInf(layout.Y, 0)
}

// ValidateJourney validates the JourneySection for duplicate IDs, missing IDs, orphan activities, and malformed canonical references.
func (v *BlueprintSchemaValidator) ValidateJourney(s *JourneySection) []ValidationIssue {
	if s == nil {
		return nil
	}

	var issues []ValidationIssue
	phaseIDs := make(map[string]bool)
	actorIDs := make(map[string]bool)
	activityIDs := make(map[string]bool)

	// Validate Phases
	for i, phase := range s.Phases {
		path := fmt.Sprintf("/journey/phases/%d", i)
		if isBlank(phase.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"journey",
				path+"/id",
				"",
				fmt.Sprintf("Journey phase at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'phase-'.",
				map[string]interface{}{"index": i, "label": phase.Label},
			))
			continue
		}

		if phaseIDs[phase.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"journey",
				path+"/id",
				phase.ID,
				fmt.Sprintf("Duplicate journey phase ID '%s' detected.", phase.ID),
				"Ensure each journey phase ID is unique.",
				map[string]interface{}{"phase_id": phase.ID},
			))
		} else {
			phaseIDs[phase.ID] = true
		}
	}

	// Validate Actors
	for i, actor := range s.Actors {
		path := fmt.Sprintf("/journey/actors/%d", i)
		if isBlank(actor.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"journey",
				path+"/id",
				"",
				fmt.Sprintf("Journey actor at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'actor-'.",
				map[string]interface{}{"index": i, "label": actor.Label},
			))
			continue
		}

		if actorIDs[actor.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"journey",
				path+"/id",
				actor.ID,
				fmt.Sprintf("Duplicate journey actor ID '%s' detected.", actor.ID),
				"Ensure each journey actor ID is unique.",
				map[string]interface{}{"actor_id": actor.ID},
			))
		} else {
			actorIDs[actor.ID] = true
		}
	}

	// Validate Activities
	for i, act := range s.Activities {
		path := fmt.Sprintf("/journey/activities/%d", i)
		if isBlank(act.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"journey",
				path+"/id",
				"",
				fmt.Sprintf("Journey activity at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'act-'.",
				map[string]interface{}{"index": i, "label": act.Label},
			))
		} else if activityIDs[act.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"journey",
				path+"/id",
				act.ID,
				fmt.Sprintf("Duplicate journey activity ID '%s' detected.", act.ID),
				"Ensure each journey activity ID is unique.",
				map[string]interface{}{"activity_id": act.ID},
			))
		} else {
			activityIDs[act.ID] = true
		}

		if isBlank(act.PhaseID) || !phaseIDs[act.PhaseID] {
			issues = append(issues, createBlockingIssue(
				ErrJourneyOrphanActivity,
				"journey",
				path+"/phase_id",
				act.ID,
				fmt.Sprintf("Journey activity '%s' references unknown or unregistered phase_id '%s'.", act.ID, act.PhaseID),
				"Register the phase in journey.phases or update the activity's phase_id.",
				map[string]interface{}{"activity_id": act.ID, "phase_id": act.PhaseID},
			))
		}

		if isBlank(act.ActorID) || !actorIDs[act.ActorID] {
			issues = append(issues, createBlockingIssue(
				ErrJourneyOrphanActivity,
				"journey",
				path+"/actor_id",
				act.ID,
				fmt.Sprintf("Journey activity '%s' references unknown or unregistered actor_id '%s'.", act.ID, act.ActorID),
				"Register the actor in journey.actors or update the activity's actor_id.",
				map[string]interface{}{"activity_id": act.ID, "actor_id": act.ActorID},
			))
		}

		for rIdx, ref := range act.References {
			if !v.canonicalRefRegex.MatchString(ref) {
				issues = append(issues, createBlockingIssue(
					ErrInvalidCanonicalRef,
					"journey",
					fmt.Sprintf("%s/references/%d", path, rIdx),
					act.ID,
					fmt.Sprintf("Journey activity '%s' contains malformed canonical reference '%s'.", act.ID, ref),
					"Format the reference to match canonical URI grammar (e.g. 'architecture.node.<id>').",
					map[string]interface{}{"activity_id": act.ID, "reference": ref},
				))
			}
		}
	}

	return issues
}

// ValidateArchitecture validates the ArchitectureSection for duplicate IDs, missing IDs, dangling edges/groups, and malformed layouts.
func (v *BlueprintSchemaValidator) ValidateArchitecture(s *ArchitectureSection) []ValidationIssue {
	if s == nil {
		return nil
	}

	var issues []ValidationIssue
	groupIDs := make(map[string]bool)
	nodeIDs := make(map[string]bool)
	edgeIDs := make(map[string]bool)

	// Validate Groups
	for i, group := range s.Groups {
		path := fmt.Sprintf("/architecture/groups/%d", i)
		if isBlank(group.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"architecture",
				path+"/id",
				"",
				fmt.Sprintf("Architecture group at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'grp-'.",
				map[string]interface{}{"index": i, "label": group.Label},
			))
			continue
		}

		if groupIDs[group.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"architecture",
				path+"/id",
				group.ID,
				fmt.Sprintf("Duplicate architecture group ID '%s' detected.", group.ID),
				"Ensure each architecture group ID is unique.",
				map[string]interface{}{"group_id": group.ID},
			))
		} else {
			groupIDs[group.ID] = true
		}
	}

	// Validate Nodes
	for i, node := range s.Nodes {
		path := fmt.Sprintf("/architecture/nodes/%d", i)
		if isBlank(node.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"architecture",
				path+"/id",
				"",
				fmt.Sprintf("Architecture node at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'node-'.",
				map[string]interface{}{"index": i, "label": node.Label},
			))
		} else if nodeIDs[node.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"architecture",
				path+"/id",
				node.ID,
				fmt.Sprintf("Duplicate architecture node ID '%s' detected.", node.ID),
				"Ensure each architecture node ID is unique.",
				map[string]interface{}{"node_id": node.ID},
			))
		} else {
			nodeIDs[node.ID] = true
		}

		if isBlank(node.GroupID) || !groupIDs[node.GroupID] {
			issues = append(issues, createBlockingIssue(
				ErrDanglingReference,
				"architecture",
				path+"/group_id",
				node.ID,
				fmt.Sprintf("Architecture node '%s' references unknown or unregistered group_id '%s'.", node.ID, node.GroupID),
				"Register the group in architecture.groups or update the node's group_id.",
				map[string]interface{}{"node_id": node.ID, "group_id": node.GroupID},
			))
		}

		if isMalformedLayout(node.Layout) {
			issues = append(issues, createBlockingIssue(
				ErrMalformedLayout,
				"architecture",
				path+"/layout",
				node.ID,
				fmt.Sprintf("Architecture node '%s' contains NaN or Inf coordinates in layout: {x: %v, y: %v}.", node.ID, node.Layout.X, node.Layout.Y),
				"Provide finite real numbers for layout coordinates.",
				map[string]interface{}{"node_id": node.ID, "layout": node.Layout},
			))
		}
	}

	// Validate Edges
	for i, edge := range s.Edges {
		path := fmt.Sprintf("/architecture/edges/%d", i)
		if isBlank(edge.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"architecture",
				path+"/id",
				"",
				fmt.Sprintf("Architecture edge at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'edge-'.",
				map[string]interface{}{"index": i, "label": edge.Label},
			))
		} else if edgeIDs[edge.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"architecture",
				path+"/id",
				edge.ID,
				fmt.Sprintf("Duplicate architecture edge ID '%s' detected.", edge.ID),
				"Ensure each architecture edge ID is unique.",
				map[string]interface{}{"edge_id": edge.ID},
			))
		} else {
			edgeIDs[edge.ID] = true
		}

		if isBlank(edge.Source) || !nodeIDs[edge.Source] {
			issues = append(issues, createBlockingIssue(
				ErrDanglingReference,
				"architecture",
				path+"/source",
				edge.ID,
				fmt.Sprintf("Architecture edge '%s' references unknown source node '%s'.", edge.ID, edge.Source),
				"Ensure edge source points to a registered architecture node.",
				map[string]interface{}{"edge_id": edge.ID, "source": edge.Source},
			))
		}

		if isBlank(edge.Target) || !nodeIDs[edge.Target] {
			issues = append(issues, createBlockingIssue(
				ErrDanglingReference,
				"architecture",
				path+"/target",
				edge.ID,
				fmt.Sprintf("Architecture edge '%s' references unknown target node '%s'.", edge.ID, edge.Target),
				"Ensure edge target points to a registered architecture node.",
				map[string]interface{}{"edge_id": edge.ID, "target": edge.Target},
			))
		}
	}

	return issues
}

// ValidateStateMachine validates the StateMachineSection for duplicate IDs, missing IDs, invalid initial state, dangling transitions, and malformed layouts.
func (v *BlueprintSchemaValidator) ValidateStateMachine(s *StateMachineSection) []ValidationIssue {
	if s == nil {
		return nil
	}

	var issues []ValidationIssue
	stateIDs := make(map[string]bool)
	transIDs := make(map[string]bool)

	// Validate States
	for i, st := range s.States {
		path := fmt.Sprintf("/state_machine/states/%d", i)
		if isBlank(st.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"state_machine",
				path+"/id",
				"",
				fmt.Sprintf("State at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'state-'.",
				map[string]interface{}{"index": i, "label": st.Label},
			))
		} else if stateIDs[st.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"state_machine",
				path+"/id",
				st.ID,
				fmt.Sprintf("Duplicate state ID '%s' detected.", st.ID),
				"Ensure each state ID is unique.",
				map[string]interface{}{"state_id": st.ID},
			))
		} else {
			stateIDs[st.ID] = true
		}

		if isMalformedLayout(st.Layout) {
			issues = append(issues, createBlockingIssue(
				ErrMalformedLayout,
				"state_machine",
				path+"/layout",
				st.ID,
				fmt.Sprintf("State '%s' contains NaN or Inf coordinates in layout: {x: %v, y: %v}.", st.ID, st.Layout.X, st.Layout.Y),
				"Provide finite real numbers for layout coordinates.",
				map[string]interface{}{"state_id": st.ID, "layout": st.Layout},
			))
		}
	}

	// Validate Initial State
	if isBlank(s.InitialState) || !stateIDs[s.InitialState] {
		issues = append(issues, createBlockingIssue(
			ErrStateInvalidInitial,
			"state_machine",
			"/state_machine/initial_state",
			s.InitialState,
			fmt.Sprintf("Initial state '%s' is not defined in state_machine.states.", s.InitialState),
			"Set initial_state to one of the registered state IDs.",
			map[string]interface{}{"initial_state": s.InitialState},
		))
	}

	// Validate Transitions
	for i, tr := range s.Transitions {
		path := fmt.Sprintf("/state_machine/transitions/%d", i)
		if isBlank(tr.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"state_machine",
				path+"/id",
				"",
				fmt.Sprintf("Transition at index %d is missing a required 'id'.", i),
				"Provide a valid kebab-case ID prefixed with 'trans-'.",
				map[string]interface{}{"index": i, "event": tr.Event},
			))
		} else if transIDs[tr.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"state_machine",
				path+"/id",
				tr.ID,
				fmt.Sprintf("Duplicate transition ID '%s' detected.", tr.ID),
				"Ensure each transition ID is unique.",
				map[string]interface{}{"transition_id": tr.ID},
			))
		} else {
			transIDs[tr.ID] = true
		}

		if isBlank(tr.Source) || !stateIDs[tr.Source] {
			issues = append(issues, createBlockingIssue(
				ErrStateDanglingTransition,
				"state_machine",
				path+"/source",
				tr.ID,
				fmt.Sprintf("Transition '%s' references unknown source state '%s'.", tr.ID, tr.Source),
				"Ensure transition source matches a registered state ID.",
				map[string]interface{}{"transition_id": tr.ID, "source": tr.Source},
			))
		}

		if isBlank(tr.Target) || !stateIDs[tr.Target] {
			issues = append(issues, createBlockingIssue(
				ErrStateDanglingTransition,
				"state_machine",
				path+"/target",
				tr.ID,
				fmt.Sprintf("Transition '%s' references unknown target state '%s'.", tr.ID, tr.Target),
				"Ensure transition target matches a registered state ID.",
				map[string]interface{}{"transition_id": tr.ID, "target": tr.Target},
			))
		}
	}

	return issues
}

// ValidateWorkflows validates the WorkflowSection for duplicate IDs, missing IDs, orphan nodes, dangling edges, underbranched decisions, and malformed layouts.
func (v *BlueprintSchemaValidator) ValidateWorkflows(workflows []Workflow) []ValidationIssue {
	if len(workflows) == 0 {
		return nil
	}

	var issues []ValidationIssue
	wfIDs := make(map[string]bool)

	for wIdx, wf := range workflows {
		wfPath := fmt.Sprintf("/workflows/%d", wIdx)
		if isBlank(wf.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"workflows",
				wfPath+"/id",
				"",
				fmt.Sprintf("Workflow at index %d is missing a required 'id'.", wIdx),
				"Provide a valid kebab-case ID prefixed with 'wf-'.",
				map[string]interface{}{"index": wIdx, "name": wf.Name},
			))
		} else if wfIDs[wf.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"workflows",
				wfPath+"/id",
				wf.ID,
				fmt.Sprintf("Duplicate workflow ID '%s' detected.", wf.ID),
				"Ensure each workflow ID is unique.",
				map[string]interface{}{"workflow_id": wf.ID},
			))
		} else {
			wfIDs[wf.ID] = true
		}

		laneIDs := make(map[string]bool)
		nodeIDs := make(map[string]bool)
		edgeIDs := make(map[string]bool)
		outgoingEdges := make(map[string]int)

		// Validate Lanes
		for lIdx, lane := range wf.Lanes {
			path := fmt.Sprintf("%s/lanes/%d", wfPath, lIdx)
			if isBlank(lane.ID) {
				issues = append(issues, createBlockingIssue(
					ErrMissingID,
					"workflows",
					path+"/id",
					"",
					fmt.Sprintf("Swimlane at index %d in workflow '%s' is missing a required 'id'.", lIdx, wf.ID),
					"Provide a valid kebab-case ID prefixed with 'lane-'.",
					map[string]interface{}{"workflow_id": wf.ID, "index": lIdx},
				))
			} else if laneIDs[lane.ID] {
				issues = append(issues, createBlockingIssue(
					ErrDuplicateID,
					"workflows",
					path+"/id",
					lane.ID,
					fmt.Sprintf("Duplicate swimlane ID '%s' in workflow '%s'.", lane.ID, wf.ID),
					"Ensure swimlane IDs within the workflow are unique.",
					map[string]interface{}{"workflow_id": wf.ID, "lane_id": lane.ID},
				))
			} else {
				laneIDs[lane.ID] = true
			}
		}

		// Validate Nodes
		for nIdx, node := range wf.Nodes {
			path := fmt.Sprintf("%s/nodes/%d", wfPath, nIdx)
			if isBlank(node.ID) {
				issues = append(issues, createBlockingIssue(
					ErrMissingID,
					"workflows",
					path+"/id",
					"",
					fmt.Sprintf("Workflow node at index %d in workflow '%s' is missing a required 'id'.", nIdx, wf.ID),
					"Provide a valid kebab-case ID prefixed with 'wnode-'.",
					map[string]interface{}{"workflow_id": wf.ID, "index": nIdx},
				))
			} else if nodeIDs[node.ID] {
				issues = append(issues, createBlockingIssue(
					ErrDuplicateID,
					"workflows",
					path+"/id",
					node.ID,
					fmt.Sprintf("Duplicate workflow node ID '%s' in workflow '%s'.", node.ID, wf.ID),
					"Ensure node IDs within the workflow are unique.",
					map[string]interface{}{"workflow_id": wf.ID, "node_id": node.ID},
				))
			} else {
				nodeIDs[node.ID] = true
			}

			if isBlank(node.LaneID) || !laneIDs[node.LaneID] {
				issues = append(issues, createBlockingIssue(
					ErrWorkflowOrphanNode,
					"workflows",
					path+"/lane_id",
					node.ID,
					fmt.Sprintf("Workflow node '%s' in workflow '%s' references unknown swimlane '%s'.", node.ID, wf.ID, node.LaneID),
					"Assign the workflow node to a registered swimlane ID.",
					map[string]interface{}{"workflow_id": wf.ID, "node_id": node.ID, "lane_id": node.LaneID},
				))
			}

			if isMalformedLayout(node.Layout) {
				issues = append(issues, createBlockingIssue(
					ErrMalformedLayout,
					"workflows",
					path+"/layout",
					node.ID,
					fmt.Sprintf("Workflow node '%s' contains NaN or Inf coordinates in layout: {x: %v, y: %v}.", node.ID, node.Layout.X, node.Layout.Y),
					"Provide finite real numbers for layout coordinates.",
					map[string]interface{}{"workflow_id": wf.ID, "node_id": node.ID, "layout": node.Layout},
				))
			}
		}

		// Validate Edges
		for eIdx, edge := range wf.Edges {
			path := fmt.Sprintf("%s/edges/%d", wfPath, eIdx)
			if isBlank(edge.ID) {
				issues = append(issues, createBlockingIssue(
					ErrMissingID,
					"workflows",
					path+"/id",
					"",
					fmt.Sprintf("Workflow edge at index %d in workflow '%s' is missing a required 'id'.", eIdx, wf.ID),
					"Provide a valid kebab-case ID prefixed with 'wedge-'.",
					map[string]interface{}{"workflow_id": wf.ID, "index": eIdx},
				))
			} else if edgeIDs[edge.ID] {
				issues = append(issues, createBlockingIssue(
					ErrDuplicateID,
					"workflows",
					path+"/id",
					edge.ID,
					fmt.Sprintf("Duplicate workflow edge ID '%s' in workflow '%s'.", edge.ID, wf.ID),
					"Ensure edge IDs within the workflow are unique.",
					map[string]interface{}{"workflow_id": wf.ID, "edge_id": edge.ID},
				))
			} else {
				edgeIDs[edge.ID] = true
			}

			if isBlank(edge.Source) || !nodeIDs[edge.Source] {
				issues = append(issues, createBlockingIssue(
					ErrWorkflowDanglingEdge,
					"workflows",
					path+"/source",
					edge.ID,
					fmt.Sprintf("Workflow edge '%s' references unknown source node '%s'.", edge.ID, edge.Source),
					"Ensure edge source points to a registered workflow node.",
					map[string]interface{}{"workflow_id": wf.ID, "edge_id": edge.ID, "source": edge.Source},
				))
			} else {
				outgoingEdges[edge.Source]++
			}

			if isBlank(edge.Target) || !nodeIDs[edge.Target] {
				issues = append(issues, createBlockingIssue(
					ErrWorkflowDanglingEdge,
					"workflows",
					path+"/target",
					edge.ID,
					fmt.Sprintf("Workflow edge '%s' references unknown target node '%s'.", edge.ID, edge.Target),
					"Ensure edge target points to a registered workflow node.",
					map[string]interface{}{"workflow_id": wf.ID, "edge_id": edge.ID, "target": edge.Target},
				))
			}
		}

		// Validate Decision Node Branching
		for nIdx, node := range wf.Nodes {
			if node.Type == "decision" {
				count := outgoingEdges[node.ID]
				if count < 2 {
					issues = append(issues, createBlockingIssue(
						ErrWorkflowDecisionUnderbranched,
						"workflows",
						fmt.Sprintf("%s/nodes/%d", wfPath, nIdx),
						node.ID,
						fmt.Sprintf("Decision node '%s' in workflow '%s' has %d outgoing edge(s); minimum required is 2.", node.ID, wf.ID, count),
						"Add at least 2 distinct outgoing edges representing decision branches (e.g. 'Yes' / 'No').",
						map[string]interface{}{"workflow_id": wf.ID, "node_id": node.ID, "outgoing_edges_count": count},
					))
				}
			}
		}
	}

	return issues
}

func detectWBSCycle(modules []WBSModule) (bool, []string) {
	adj := make(map[string][]string)
	for _, m := range modules {
		if !isBlank(m.ID) {
			adj[m.ID] = m.Dependencies
		}
	}

	// 0: unvisited, 1: visiting (in stack), 2: visited
	color := make(map[string]int)
	var cyclePath []string

	var dfs func(u string, path []string) bool
	dfs = func(u string, path []string) bool {
		color[u] = 1
		for _, v := range adj[u] {
			if color[v] == 1 {
				cyclePath = append(path, u, v)
				return true
			}
			if color[v] == 0 {
				if dfs(v, append(path, u)) {
					return true
				}
			}
		}
		color[u] = 2
		return false
	}

	for _, m := range modules {
		if !isBlank(m.ID) && color[m.ID] == 0 {
			if dfs(m.ID, nil) {
				return true, cyclePath
			}
		}
	}
	return false, nil
}

// ValidateWBS validates the WBSSection for duplicate IDs, missing IDs, self/unknown dependencies, cycles, and malformed canonical references.
func (v *BlueprintSchemaValidator) ValidateWBS(s *WBSSection) []ValidationIssue {
	if s == nil {
		return nil
	}

	var issues []ValidationIssue
	moduleIDs := make(map[string]bool)
	featureIDs := make(map[string]bool)

	// Validate Modules and Features
	for mIdx, mod := range s.Modules {
		mPath := fmt.Sprintf("/wbs/modules/%d", mIdx)
		if isBlank(mod.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"wbs",
				mPath+"/id",
				"",
				fmt.Sprintf("WBS module at index %d is missing a required 'id'.", mIdx),
				"Provide a valid kebab-case ID prefixed with 'mod-'.",
				map[string]interface{}{"index": mIdx, "name": mod.Name},
			))
		} else if moduleIDs[mod.ID] {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				"wbs",
				mPath+"/id",
				mod.ID,
				fmt.Sprintf("Duplicate WBS module ID '%s' detected.", mod.ID),
				"Ensure each WBS module ID is unique.",
				map[string]interface{}{"module_id": mod.ID},
			))
		} else {
			moduleIDs[mod.ID] = true
		}

		for fIdx, feat := range mod.Features {
			fPath := fmt.Sprintf("%s/features/%d", mPath, fIdx)
			if isBlank(feat.ID) {
				issues = append(issues, createBlockingIssue(
					ErrMissingID,
					"wbs",
					fPath+"/id",
					"",
					fmt.Sprintf("WBS feature at index %d under module '%s' is missing a required 'id'.", fIdx, mod.ID),
					"Provide a valid kebab-case ID prefixed with 'feat-'.",
					map[string]interface{}{"module_id": mod.ID, "index": fIdx},
				))
			} else if featureIDs[feat.ID] {
				issues = append(issues, createBlockingIssue(
					ErrDuplicateID,
					"wbs",
					fPath+"/id",
					feat.ID,
					fmt.Sprintf("Duplicate WBS feature ID '%s' detected.", feat.ID),
					"Ensure each WBS feature ID is unique.",
					map[string]interface{}{"module_id": mod.ID, "feature_id": feat.ID},
				))
			} else {
				featureIDs[feat.ID] = true
			}
		}

		for rIdx, ref := range mod.References {
			if !v.canonicalRefRegex.MatchString(ref) {
				issues = append(issues, createBlockingIssue(
					ErrInvalidCanonicalRef,
					"wbs",
					fmt.Sprintf("%s/references/%d", mPath, rIdx),
					mod.ID,
					fmt.Sprintf("WBS module '%s' contains malformed canonical reference '%s'.", mod.ID, ref),
					"Format the reference to match canonical URI grammar (e.g. 'architecture.node.<id>').",
					map[string]interface{}{"module_id": mod.ID, "reference": ref},
				))
			}
		}
	}

	// Validate Dependencies
	for mIdx, mod := range s.Modules {
		mPath := fmt.Sprintf("/wbs/modules/%d", mIdx)
		for dIdx, dep := range mod.Dependencies {
			dPath := fmt.Sprintf("%s/dependencies/%d", mPath, dIdx)
			if dep == mod.ID {
				issues = append(issues, createBlockingIssue(
					ErrWBSInvalidDependency,
					"wbs",
					dPath,
					mod.ID,
					fmt.Sprintf("WBS module '%s' has self-dependency on itself.", mod.ID),
					"Remove self-dependency from dependencies array.",
					map[string]interface{}{"module_id": mod.ID, "dependency": dep},
				))
			} else if !moduleIDs[dep] {
				issues = append(issues, createBlockingIssue(
					ErrWBSInvalidDependency,
					"wbs",
					dPath,
					mod.ID,
					fmt.Sprintf("WBS module '%s' depends on unknown or unregistered module '%s'.", mod.ID, dep),
					"Ensure dependency points to a valid registered WBS module ID.",
					map[string]interface{}{"module_id": mod.ID, "dependency": dep},
				))
			}
		}
	}

	// Validate Dependency Cycles via DFS
	if hasCycle, cyclePath := detectWBSCycle(s.Modules); hasCycle {
		issues = append(issues, createBlockingIssue(
			ErrWBSDependencyCycle,
			"wbs",
			"/wbs/modules",
			"",
			fmt.Sprintf("Circular dependency cycle detected among WBS modules: %s.", strings.Join(cyclePath, " -> ")),
			"Break dependency cycles by restructuring module dependencies into a Directed Acyclic Graph (DAG).",
			map[string]interface{}{"cycle_path": cyclePath},
		))
	}

	return issues
}

func (v *BlueprintSchemaValidator) resolveSymbol(ref string, symbolTable map[string]bool, rawIDs map[string]bool) bool {
	if symbolTable[ref] {
		return true
	}
	parts := strings.Split(ref, ".")
	lastPart := parts[len(parts)-1]
	return rawIDs[lastPart]
}

// ValidateBundle performs comprehensive 5-section and cross-section validation on a complete BlueprintBundle.
func (v *BlueprintSchemaValidator) ValidateBundle(bundle *BlueprintBundle) []ValidationIssue {
	if bundle == nil {
		return []ValidationIssue{
			createBlockingIssue(
				ErrUnsupportedSchemaVersion,
				"general",
				"/schema_version",
				"",
				"Blueprint bundle payload is nil.",
				"Provide a valid BlueprintBundle object.",
				nil,
			),
		}
	}

	var issues []ValidationIssue

	// Phase 1: Syntactic Schema Validation
	if bundle.SchemaVersion != BlueprintSchemaVersion {
		issues = append(issues, createBlockingIssue(
			ErrUnsupportedSchemaVersion,
			"general",
			"/schema_version",
			bundle.SchemaVersion,
			fmt.Sprintf("Unsupported schema version '%s'. Must be '%s'.", bundle.SchemaVersion, BlueprintSchemaVersion),
			fmt.Sprintf("Set schema_version to '%s'.", BlueprintSchemaVersion),
			map[string]interface{}{"schema_version": bundle.SchemaVersion, "expected": BlueprintSchemaVersion},
		))
	}

	// Global duplicate ID tracker
	globalIDs := make(map[string]string) // ID -> section/path
	checkGlobalID := func(id string, section, path string) {
		if isBlank(id) {
			return
		}
		if origPath, exists := globalIDs[id]; exists {
			issues = append(issues, createBlockingIssue(
				ErrDuplicateID,
				section,
				path,
				id,
				fmt.Sprintf("Duplicate global ID '%s' detected. First declared at '%s'.", id, origPath),
				"Ensure identifier is globally unique across all blueprint sections.",
				map[string]interface{}{"id": id, "conflict_with": origPath},
			))
		} else {
			globalIDs[id] = path
		}
	}

	// Phase 2: Section Validations
	issues = append(issues, v.ValidateJourney(&bundle.Journey)...)
	issues = append(issues, v.ValidateArchitecture(&bundle.Architecture)...)
	issues = append(issues, v.ValidateStateMachine(&bundle.StateMachine)...)
	issues = append(issues, v.ValidateWorkflows(bundle.Workflows)...)
	issues = append(issues, v.ValidateWBS(&bundle.WBS)...)

	// Collect global symbols for Phase 3 Symbol Resolution
	symbolTable := make(map[string]bool)
	rawIDs := make(map[string]bool)

	// Journey symbols
	for i, p := range bundle.Journey.Phases {
		checkGlobalID(p.ID, "journey", fmt.Sprintf("/journey/phases/%d/id", i))
		if !isBlank(p.ID) {
			rawIDs[p.ID] = true
			symbolTable[fmt.Sprintf("journey.phase.%s", p.ID)] = true
		}
	}
	for i, a := range bundle.Journey.Actors {
		checkGlobalID(a.ID, "journey", fmt.Sprintf("/journey/actors/%d/id", i))
		if !isBlank(a.ID) {
			rawIDs[a.ID] = true
			symbolTable[fmt.Sprintf("journey.actor.%s", a.ID)] = true
		}
	}
	for i, act := range bundle.Journey.Activities {
		checkGlobalID(act.ID, "journey", fmt.Sprintf("/journey/activities/%d/id", i))
		if !isBlank(act.ID) {
			rawIDs[act.ID] = true
			symbolTable[fmt.Sprintf("journey.activity.%s", act.ID)] = true
		}
	}

	// Architecture symbols
	for i, g := range bundle.Architecture.Groups {
		checkGlobalID(g.ID, "architecture", fmt.Sprintf("/architecture/groups/%d/id", i))
		if !isBlank(g.ID) {
			rawIDs[g.ID] = true
			symbolTable[fmt.Sprintf("architecture.group.%s", g.ID)] = true
		}
	}
	for i, n := range bundle.Architecture.Nodes {
		checkGlobalID(n.ID, "architecture", fmt.Sprintf("/architecture/nodes/%d/id", i))
		if !isBlank(n.ID) {
			rawIDs[n.ID] = true
			symbolTable[fmt.Sprintf("architecture.node.%s", n.ID)] = true
		}
	}
	for i, e := range bundle.Architecture.Edges {
		checkGlobalID(e.ID, "architecture", fmt.Sprintf("/architecture/edges/%d/id", i))
		if !isBlank(e.ID) {
			rawIDs[e.ID] = true
			symbolTable[fmt.Sprintf("architecture.edge.%s", e.ID)] = true
		}
	}

	// State Machine symbols
	entity := bundle.StateMachine.Entity
	for i, s := range bundle.StateMachine.States {
		checkGlobalID(s.ID, "state_machine", fmt.Sprintf("/state_machine/states/%d/id", i))
		if !isBlank(s.ID) {
			rawIDs[s.ID] = true
			symbolTable[fmt.Sprintf("state.%s.%s", entity, s.ID)] = true
			symbolTable[fmt.Sprintf("state.%s", s.ID)] = true
			symbolTable[fmt.Sprintf("state_machine.state.%s", s.ID)] = true
		}
	}
	for i, t := range bundle.StateMachine.Transitions {
		checkGlobalID(t.ID, "state_machine", fmt.Sprintf("/state_machine/transitions/%d/id", i))
		if !isBlank(t.ID) {
			rawIDs[t.ID] = true
			symbolTable[fmt.Sprintf("state.transition.%s", t.ID)] = true
			symbolTable[fmt.Sprintf("state_machine.transition.%s", t.ID)] = true
		}
	}

	// Workflow symbols
	for wIdx, wf := range bundle.Workflows {
		checkGlobalID(wf.ID, "workflows", fmt.Sprintf("/workflows/%d/id", wIdx))
		if !isBlank(wf.ID) {
			rawIDs[wf.ID] = true
			symbolTable[fmt.Sprintf("workflow.%s", wf.ID)] = true
		}
		for lIdx, l := range wf.Lanes {
			checkGlobalID(l.ID, "workflows", fmt.Sprintf("/workflows/%d/lanes/%d/id", wIdx, lIdx))
			if !isBlank(l.ID) {
				rawIDs[l.ID] = true
				symbolTable[fmt.Sprintf("workflow.%s.lane.%s", wf.ID, l.ID)] = true
				symbolTable[fmt.Sprintf("workflow.lane.%s", l.ID)] = true
			}
		}
		for nIdx, n := range wf.Nodes {
			checkGlobalID(n.ID, "workflows", fmt.Sprintf("/workflows/%d/nodes/%d/id", wIdx, nIdx))
			if !isBlank(n.ID) {
				rawIDs[n.ID] = true
				symbolTable[fmt.Sprintf("workflow.%s.step.%s", wf.ID, n.ID)] = true
				symbolTable[fmt.Sprintf("workflow.step.%s", n.ID)] = true
				symbolTable[fmt.Sprintf("workflow.%s", n.ID)] = true
			}
		}
		for eIdx, e := range wf.Edges {
			checkGlobalID(e.ID, "workflows", fmt.Sprintf("/workflows/%d/edges/%d/id", wIdx, eIdx))
			if !isBlank(e.ID) {
				rawIDs[e.ID] = true
				symbolTable[fmt.Sprintf("workflow.%s.edge.%s", wf.ID, e.ID)] = true
				symbolTable[fmt.Sprintf("workflow.edge.%s", e.ID)] = true
			}
		}
	}

	// WBS symbols
	for mIdx, m := range bundle.WBS.Modules {
		checkGlobalID(m.ID, "wbs", fmt.Sprintf("/wbs/modules/%d/id", mIdx))
		if !isBlank(m.ID) {
			rawIDs[m.ID] = true
			symbolTable[fmt.Sprintf("wbs.module.%s", m.ID)] = true
		}
		for fIdx, f := range m.Features {
			checkGlobalID(f.ID, "wbs", fmt.Sprintf("/wbs/modules/%d/features/%d/id", mIdx, fIdx))
			if !isBlank(f.ID) {
				rawIDs[f.ID] = true
				symbolTable[fmt.Sprintf("wbs.module.%s.feature.%s", m.ID, f.ID)] = true
				symbolTable[fmt.Sprintf("wbs.feature.%s", f.ID)] = true
			}
		}
	}

	// Open Questions
	for qIdx, q := range bundle.OpenQuestions {
		path := fmt.Sprintf("/open_questions/%d", qIdx)
		if isBlank(q.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"open_questions",
				path+"/id",
				"",
				fmt.Sprintf("Open question at index %d is missing a required 'id'.", qIdx),
				"Provide a valid kebab-case ID prefixed with 'oq-'.",
				map[string]interface{}{"index": qIdx, "question": q.Question},
			))
		} else {
			checkGlobalID(q.ID, "open_questions", path+"/id")
			rawIDs[q.ID] = true
			symbolTable[fmt.Sprintf("open_questions.%s", q.ID)] = true
		}
	}

	// Cross References
	for xIdx, xref := range bundle.CrossReferences {
		path := fmt.Sprintf("/cross_references/%d", xIdx)
		if isBlank(xref.ID) {
			issues = append(issues, createBlockingIssue(
				ErrMissingID,
				"cross_references",
				path+"/id",
				"",
				fmt.Sprintf("Cross reference at index %d is missing a required 'id'.", xIdx),
				"Provide a valid kebab-case ID prefixed with 'xref-'.",
				map[string]interface{}{"index": xIdx},
			))
		} else {
			checkGlobalID(xref.ID, "cross_references", path+"/id")
			rawIDs[xref.ID] = true
			symbolTable[fmt.Sprintf("cross_references.%s", xref.ID)] = true
		}

		if !v.canonicalRefRegex.MatchString(xref.SourceRef) {
			issues = append(issues, createBlockingIssue(
				ErrInvalidCanonicalRef,
				"cross_references",
				path+"/source_ref",
				xref.ID,
				fmt.Sprintf("Cross reference '%s' contains malformed source_ref '%s'.", xref.ID, xref.SourceRef),
				"Format source_ref to match canonical URI grammar.",
				map[string]interface{}{"xref_id": xref.ID, "source_ref": xref.SourceRef},
			))
		} else if !v.resolveSymbol(xref.SourceRef, symbolTable, rawIDs) {
			issues = append(issues, createBlockingIssue(
				ErrDanglingReference,
				"cross_references",
				path+"/source_ref",
				xref.ID,
				fmt.Sprintf("Cross reference '%s' source_ref '%s' does not resolve to any blueprint element.", xref.ID, xref.SourceRef),
				"Ensure source_ref points to an existing registered blueprint element.",
				map[string]interface{}{"xref_id": xref.ID, "source_ref": xref.SourceRef},
			))
		}

		if !v.canonicalRefRegex.MatchString(xref.TargetRef) {
			issues = append(issues, createBlockingIssue(
				ErrInvalidCanonicalRef,
				"cross_references",
				path+"/target_ref",
				xref.ID,
				fmt.Sprintf("Cross reference '%s' contains malformed target_ref '%s'.", xref.ID, xref.TargetRef),
				"Format target_ref to match canonical URI grammar.",
				map[string]interface{}{"xref_id": xref.ID, "target_ref": xref.TargetRef},
			))
		} else if !v.resolveSymbol(xref.TargetRef, symbolTable, rawIDs) {
			issues = append(issues, createBlockingIssue(
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

	// Phase 3: Resolve all section references against Symbol Table
	for actIdx, act := range bundle.Journey.Activities {
		for rIdx, ref := range act.References {
			if v.canonicalRefRegex.MatchString(ref) && !v.resolveSymbol(ref, symbolTable, rawIDs) {
				issues = append(issues, createBlockingIssue(
					ErrDanglingReference,
					"journey",
					fmt.Sprintf("/journey/activities/%d/references/%d", actIdx, rIdx),
					act.ID,
					fmt.Sprintf("Journey activity '%s' reference '%s' does not resolve to any blueprint element.", act.ID, ref),
					"Ensure reference points to an existing registered blueprint element.",
					map[string]interface{}{"activity_id": act.ID, "reference": ref},
				))
			}
		}
	}

	for modIdx, mod := range bundle.WBS.Modules {
		for rIdx, ref := range mod.References {
			if v.canonicalRefRegex.MatchString(ref) && !v.resolveSymbol(ref, symbolTable, rawIDs) {
				issues = append(issues, createBlockingIssue(
					ErrDanglingReference,
					"wbs",
					fmt.Sprintf("/wbs/modules/%d/references/%d", modIdx, rIdx),
					mod.ID,
					fmt.Sprintf("WBS module '%s' reference '%s' does not resolve to any blueprint element.", mod.ID, ref),
					"Ensure reference points to an existing registered blueprint element.",
					map[string]interface{}{"module_id": mod.ID, "reference": ref},
				))
			}
		}
	}

	return issues
}
