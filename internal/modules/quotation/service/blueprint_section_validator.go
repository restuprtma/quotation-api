package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	canonicalRefPattern = regexp.MustCompile(`^(journey|architecture|state|state_machine|workflow|wbs|open_questions|cross_references)(\.[a-z0-9_-]+)+$`)
	templateExprPattern = regexp.MustCompile(`(\$\{[^}]*\}|\{\{[^}]*\}\})`)
)

var forbiddenKeys = map[string]bool{
	"budget":        true,
	"maximumbudget": true,
	"ratecard":      true,
	"rates":         true,
	"paymentterms":  true,
	"legal":         true,
	"signature":     true,
	"apikey":        true,
	"credential":    true,
	"accesstoken":   true,
	"systemprompt":  true,
	"rawresponse":   true,
	"provider":      true,
	"model":         true,
}

var validArchNodeTypes = map[string]bool{
	"channel":     true,
	"application": true,
	"service":     true,
	"database":    true,
	"cache":       true,
	"storage":     true,
	"integration": true,
	"device":      true,
}

var validArchEdgeTypes = map[string]bool{
	"sync":      true,
	"async":     true,
	"data_flow": true,
	"event":     true,
}

var validWBSPriorities = map[string]bool{
	"high":   true,
	"medium": true,
	"low":    true,
}

// BlueprintSectionValidator enforces strict envelope decoding, security rules, and structural validity.
type BlueprintSectionValidator struct{}

// NewBlueprintSectionValidator constructs a new BlueprintSectionValidator.
func NewBlueprintSectionValidator() *BlueprintSectionValidator {
	return &BlueprintSectionValidator{}
}

type journeyEnvelope struct {
	Journey *BlueprintJourney `json:"journey"`
}

type architectureEnvelope struct {
	Architecture *BlueprintArchitecture `json:"architecture"`
}

type stateMachineEnvelope struct {
	StateMachine *BlueprintStateMachine `json:"state_machine"`
}

type workflowEnvelope struct {
	Workflows *[]BlueprintWorkflow `json:"workflows"`
}

type wbsEnvelope struct {
	WBS             *BlueprintWBS             `json:"wbs"`
	CrossReferences []BlueprintCrossReference `json:"cross_references"`
}

type decodedWBSSection struct {
	WBS             BlueprintWBS
	CrossReferences []BlueprintCrossReference
}

func normalizeKey(k string) string {
	k = strings.ToLower(k)
	k = strings.ReplaceAll(k, "-", "")
	k = strings.ReplaceAll(k, "_", "")
	k = strings.ReplaceAll(k, " ", "")
	return k
}

func containsForbiddenPattern(s string) bool {
	lower := strings.ToLower(s)
	if strings.Contains(lower, "<script") ||
		strings.Contains(lower, "onerror=") ||
		strings.Contains(lower, "javascript:") ||
		strings.Contains(lower, "data:text/html") ||
		strings.Contains(s, "```") ||
		templateExprPattern.MatchString(s) {
		return true
	}
	return false
}

func checkJSONDepthAndUTF8(payload []byte) bool {
	if !utf8.Valid(payload) {
		return false
	}
	depth := 0
	inString := false
	escaped := false
	for _, b := range payload {
		if inString {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				inString = false
			}
			continue
		}
		if b == '"' {
			inString = true
			continue
		}
		if b == '{' || b == '[' {
			depth++
			if depth > 32 {
				return false
			}
		} else if b == '}' || b == ']' {
			depth--
		}
	}
	return true
}

func validateBlueprintJSONSecurity(value any, path string, section string) []BlueprintValidationIssue {
	var issues []BlueprintValidationIssue

	switch v := value.(type) {
	case map[string]any:
		// Collect sorted keys for determinism
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, k := range keys {
			val := v[k]
			subPath := path + "/" + k
			if forbiddenKeys[normalizeKey(k)] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueForbiddenContent,
					Severity:    "BLOCKING_ERROR",
					Section:     section,
					Path:        subPath,
					Message:     "section payload contains forbidden content",
					Blocking:    true,
					Remediation: "Remove forbidden commercial terms, credentials, HTML/scripts, or template expressions.",
				})
			}
			issues = append(issues, validateBlueprintJSONSecurity(val, subPath, section)...)
		}
	case []any:
		for i, elem := range v {
			subPath := fmt.Sprintf("%s/%d", path, i)
			issues = append(issues, validateBlueprintJSONSecurity(elem, subPath, section)...)
		}
	case string:
		if containsForbiddenPattern(v) {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueForbiddenContent,
				Severity:    "BLOCKING_ERROR",
				Section:     section,
				Path:        path,
				Message:     "section payload contains forbidden content",
				Blocking:    true,
				Remediation: "Remove forbidden commercial terms, credentials, HTML/scripts, or template expressions.",
			})
		}
	}

	return issues
}

func strictDecodeBlueprintSection(sectionType domain.BlueprintSectionType, payload json.RawMessage) (any, string, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, "", fmt.Errorf("empty payload")
	}

	switch sectionType {
	case domain.BlueprintSectionJourney:
		var env journeyEnvelope
		dec := json.NewDecoder(bytes.NewReader(payload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&env); err != nil {
			return nil, extractUnknownFieldPath(err), err
		}
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, "", fmt.Errorf("trailing content after JSON payload")
		}
		if env.Journey == nil {
			return nil, "/journey", fmt.Errorf("missing journey root key")
		}
		return *env.Journey, "", nil

	case domain.BlueprintSectionArchitecture:
		var env architectureEnvelope
		dec := json.NewDecoder(bytes.NewReader(payload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&env); err != nil {
			return nil, extractUnknownFieldPath(err), err
		}
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, "", fmt.Errorf("trailing content after JSON payload")
		}
		if env.Architecture == nil {
			return nil, "/architecture", fmt.Errorf("missing architecture root key")
		}
		return *env.Architecture, "", nil

	case domain.BlueprintSectionStateMachine:
		var env stateMachineEnvelope
		dec := json.NewDecoder(bytes.NewReader(payload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&env); err != nil {
			return nil, extractUnknownFieldPath(err), err
		}
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, "", fmt.Errorf("trailing content after JSON payload")
		}
		if env.StateMachine == nil {
			return nil, "/state_machine", fmt.Errorf("missing state_machine root key")
		}
		return *env.StateMachine, "", nil

	case domain.BlueprintSectionWorkflow:
		var env workflowEnvelope
		dec := json.NewDecoder(bytes.NewReader(payload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&env); err != nil {
			return nil, extractUnknownFieldPath(err), err
		}
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, "", fmt.Errorf("trailing content after JSON payload")
		}
		if env.Workflows == nil {
			return nil, "/workflows", fmt.Errorf("missing workflows root key")
		}
		return *env.Workflows, "", nil

	case domain.BlueprintSectionWBS:
		var env wbsEnvelope
		dec := json.NewDecoder(bytes.NewReader(payload))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&env); err != nil {
			return nil, extractUnknownFieldPath(err), err
		}
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, "", fmt.Errorf("trailing content after JSON payload")
		}
		if env.WBS == nil {
			return nil, "/wbs", fmt.Errorf("missing wbs root key")
		}
		if env.CrossReferences == nil {
			env.CrossReferences = []BlueprintCrossReference{}
		}
		return decodedWBSSection{WBS: *env.WBS, CrossReferences: env.CrossReferences}, "", nil

	default:
		return nil, "", fmt.Errorf("unsupported or non-eligible blueprint section type: %s", sectionType)
	}
}

func extractUnknownFieldPath(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const prefix = `unknown field "`
	if idx := strings.Index(msg, prefix); idx != -1 {
		rest := msg[idx+len(prefix):]
		if end := strings.IndexByte(rest, '"'); end != -1 {
			fieldName := rest[:end]
			return "/" + fieldName
		}
	}
	return ""
}

func sortBlueprintValidationIssues(issues []BlueprintValidationIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Section != issues[j].Section {
			return issues[i].Section < issues[j].Section
		}
		if issues[i].Path != issues[j].Path {
			return issues[i].Path < issues[j].Path
		}
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].TargetID < issues[j].TargetID
	})
}

func normalizedJSON(value any) json.RawMessage {
	b, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

func isMalformedLayout(layout BlueprintLayout) bool {
	return math.IsNaN(layout.X) || math.IsInf(layout.X, 0) ||
		math.IsNaN(layout.Y) || math.IsInf(layout.Y, 0) ||
		layout.X < -100000 || layout.X > 100000 ||
		layout.Y < -100000 || layout.Y > 100000
}

func validateJourneySection(j BlueprintJourney) []BlueprintValidationIssue {
	var issues []BlueprintValidationIssue
	sec := "journey"
	phaseIDs := make(map[string]bool)
	actorIDs := make(map[string]bool)
	activityIDs := make(map[string]bool)

	for i, phase := range j.Phases {
		pPath := fmt.Sprintf("/journey/phases/%d", i)
		if strings.TrimSpace(phase.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        pPath + "/id",
				TargetID:    "",
				Message:     "Phase ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'phase-'.",
			})
		} else if phaseIDs[phase.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        pPath + "/id",
				TargetID:    phase.ID,
				Message:     "Duplicate phase ID detected.",
				Blocking:    true,
				Remediation: "Ensure all phase IDs are unique.",
			})
		} else {
			phaseIDs[phase.ID] = true
		}

		if strings.TrimSpace(phase.Label) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        pPath + "/label",
				TargetID:    phase.ID,
				Message:     "Phase label is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable label.",
			})
		}
	}

	for i, actor := range j.Actors {
		aPath := fmt.Sprintf("/journey/actors/%d", i)
		if strings.TrimSpace(actor.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        aPath + "/id",
				TargetID:    "",
				Message:     "Actor ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'actor-'.",
			})
		} else if actorIDs[actor.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        aPath + "/id",
				TargetID:    actor.ID,
				Message:     "Duplicate actor ID detected.",
				Blocking:    true,
				Remediation: "Ensure all actor IDs are unique.",
			})
		} else {
			actorIDs[actor.ID] = true
		}

		if strings.TrimSpace(actor.Label) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        aPath + "/label",
				TargetID:    actor.ID,
				Message:     "Actor label is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable label.",
			})
		}
	}

	for i, act := range j.Activities {
		actPath := fmt.Sprintf("/journey/activities/%d", i)
		if strings.TrimSpace(act.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        actPath + "/id",
				TargetID:    "",
				Message:     "Activity ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'act-'.",
			})
		} else if activityIDs[act.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        actPath + "/id",
				TargetID:    act.ID,
				Message:     "Duplicate activity ID detected.",
				Blocking:    true,
				Remediation: "Ensure all activity IDs are unique.",
			})
		} else {
			activityIDs[act.ID] = true
		}

		if strings.TrimSpace(act.Label) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        actPath + "/label",
				TargetID:    act.ID,
				Message:     "Activity label is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable label.",
			})
		}

		if strings.TrimSpace(act.PhaseID) == "" || !phaseIDs[act.PhaseID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        actPath + "/phase_id",
				TargetID:    act.ID,
				Message:     "Journey activity references unregistered phase ID.",
				Blocking:    true,
				Remediation: "Ensure phase_id points to a registered phase in journey.phases.",
			})
		}

		if strings.TrimSpace(act.ActorID) == "" || !actorIDs[act.ActorID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        actPath + "/actor_id",
				TargetID:    act.ID,
				Message:     "Journey activity references unregistered actor ID.",
				Blocking:    true,
				Remediation: "Ensure actor_id points to a registered actor in journey.actors.",
			})
		}

		for rIdx, ref := range act.References {
			if !canonicalRefPattern.MatchString(ref) {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueInvalidCanonicalRef,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        fmt.Sprintf("%s/references/%d", actPath, rIdx),
					TargetID:    act.ID,
					Message:     "Malformed canonical reference URI.",
					Blocking:    true,
					Remediation: "Format reference using canonical URI syntax.",
				})
			}
		}
	}

	return issues
}

func validateArchitectureSection(a BlueprintArchitecture) []BlueprintValidationIssue {
	var issues []BlueprintValidationIssue
	sec := "architecture"
	groupIDs := make(map[string]bool)
	nodeIDs := make(map[string]bool)
	edgeIDs := make(map[string]bool)

	for i, grp := range a.Groups {
		gPath := fmt.Sprintf("/architecture/groups/%d", i)
		if strings.TrimSpace(grp.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        gPath + "/id",
				TargetID:    "",
				Message:     "Architecture group ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'grp-'.",
			})
		} else if groupIDs[grp.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        gPath + "/id",
				TargetID:    grp.ID,
				Message:     "Duplicate architecture group ID detected.",
				Blocking:    true,
				Remediation: "Ensure all group IDs are unique.",
			})
		} else {
			groupIDs[grp.ID] = true
		}

		if strings.TrimSpace(grp.Label) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        gPath + "/label",
				TargetID:    grp.ID,
				Message:     "Architecture group label is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable label.",
			})
		}
	}

	for i, node := range a.Nodes {
		nPath := fmt.Sprintf("/architecture/nodes/%d", i)
		if strings.TrimSpace(node.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        nPath + "/id",
				TargetID:    "",
				Message:     "Architecture node ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'node-'.",
			})
		} else if nodeIDs[node.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        nPath + "/id",
				TargetID:    node.ID,
				Message:     "Duplicate architecture node ID detected.",
				Blocking:    true,
				Remediation: "Ensure all node IDs are unique.",
			})
		} else {
			nodeIDs[node.ID] = true
		}

		if strings.TrimSpace(node.Label) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        nPath + "/label",
				TargetID:    node.ID,
				Message:     "Architecture node label is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable label.",
			})
		}

		if strings.TrimSpace(node.GroupID) == "" || !groupIDs[node.GroupID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        nPath + "/group_id",
				TargetID:    node.ID,
				Message:     "Architecture node references unregistered group ID.",
				Blocking:    true,
				Remediation: "Ensure group_id matches a registered group in architecture.groups.",
			})
		}

		if !validArchNodeTypes[node.Type] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueUnsupportedVersion,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        nPath + "/type",
				TargetID:    node.ID,
				Message:     "Unsupported architecture node type.",
				Blocking:    true,
				Remediation: "Use one of the supported component types: channel, application, service, database, cache, storage, integration, device.",
			})
		}

		if isMalformedLayout(node.Layout) {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMalformedLayout,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        nPath + "/layout",
				TargetID:    node.ID,
				Message:     "Architecture node layout coordinates out of bounds or malformed.",
				Blocking:    true,
				Remediation: "Provide finite real numbers between -100000 and 100000 for x and y.",
			})
		}
	}

	for i, edge := range a.Edges {
		ePath := fmt.Sprintf("/architecture/edges/%d", i)
		if strings.TrimSpace(edge.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        ePath + "/id",
				TargetID:    "",
				Message:     "Architecture edge ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'edge-'.",
			})
		} else if edgeIDs[edge.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        ePath + "/id",
				TargetID:    edge.ID,
				Message:     "Duplicate architecture edge ID detected.",
				Blocking:    true,
				Remediation: "Ensure all edge IDs are unique.",
			})
		} else {
			edgeIDs[edge.ID] = true
		}

		if !validArchEdgeTypes[edge.Type] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueUnsupportedVersion,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        ePath + "/type",
				TargetID:    edge.ID,
				Message:     "Unsupported architecture edge type.",
				Blocking:    true,
				Remediation: "Use one of the supported communication paradigms: sync, async, data_flow, event.",
			})
		}

		if strings.TrimSpace(edge.Source) == "" || !nodeIDs[edge.Source] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        ePath + "/source",
				TargetID:    edge.ID,
				Message:     "Architecture edge references unregistered source node.",
				Blocking:    true,
				Remediation: "Ensure edge source matches a registered architecture node ID.",
			})
		}

		if strings.TrimSpace(edge.Target) == "" || !nodeIDs[edge.Target] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDanglingReference,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        ePath + "/target",
				TargetID:    edge.ID,
				Message:     "Architecture edge references unregistered target node.",
				Blocking:    true,
				Remediation: "Ensure edge target matches a registered architecture node ID.",
			})
		}
	}

	return issues
}

func validateStateMachineSection(sm BlueprintStateMachine) []BlueprintValidationIssue {
	var issues []BlueprintValidationIssue
	sec := "state_machine"
	stateIDs := make(map[string]bool)
	transIDs := make(map[string]bool)

	for i, st := range sm.States {
		sPath := fmt.Sprintf("/state_machine/states/%d", i)
		if strings.TrimSpace(st.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        sPath + "/id",
				TargetID:    "",
				Message:     "State ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'state-'.",
			})
		} else if stateIDs[st.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        sPath + "/id",
				TargetID:    st.ID,
				Message:     "Duplicate state ID detected.",
				Blocking:    true,
				Remediation: "Ensure all state IDs are unique.",
			})
		} else {
			stateIDs[st.ID] = true
		}

		if strings.TrimSpace(st.Label) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        sPath + "/label",
				TargetID:    st.ID,
				Message:     "State label is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable label.",
			})
		}

		if isMalformedLayout(st.Layout) {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMalformedLayout,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        sPath + "/layout",
				TargetID:    st.ID,
				Message:     "State layout coordinates out of bounds or malformed.",
				Blocking:    true,
				Remediation: "Provide finite real numbers between -100000 and 100000 for x and y.",
			})
		}
	}

	if strings.TrimSpace(sm.InitialState) == "" || !stateIDs[sm.InitialState] {
		issues = append(issues, BlueprintValidationIssue{
			Code:        BlueprintIssueStateInvalidInitial,
			Severity:    "BLOCKING_ERROR",
			Section:     sec,
			Path:        "/state_machine/initial_state",
			TargetID:    sm.InitialState,
			Message:     "Initial state is not defined in state_machine.states.",
			Blocking:    true,
			Remediation: "Set initial_state to one of the registered state IDs.",
		})
	}

	for i, tr := range sm.Transitions {
		tPath := fmt.Sprintf("/state_machine/transitions/%d", i)
		if strings.TrimSpace(tr.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        tPath + "/id",
				TargetID:    "",
				Message:     "Transition ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'trans-'.",
			})
		} else if transIDs[tr.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        tPath + "/id",
				TargetID:    tr.ID,
				Message:     "Duplicate transition ID detected.",
				Blocking:    true,
				Remediation: "Ensure all transition IDs are unique.",
			})
		} else {
			transIDs[tr.ID] = true
		}

		if strings.TrimSpace(tr.Source) == "" || !stateIDs[tr.Source] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueStateDanglingTransition,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        tPath + "/source",
				TargetID:    tr.ID,
				Message:     "Transition references unregistered source state.",
				Blocking:    true,
				Remediation: "Ensure transition source matches a registered state ID.",
			})
		}

		if strings.TrimSpace(tr.Target) == "" || !stateIDs[tr.Target] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueStateDanglingTransition,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        tPath + "/target",
				TargetID:    tr.ID,
				Message:     "Transition references unregistered target state.",
				Blocking:    true,
				Remediation: "Ensure transition target matches a registered state ID.",
			})
		}

		if tr.Source == tr.Target && strings.TrimSpace(tr.Event) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueStateDanglingTransition,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        tPath + "/event",
				TargetID:    tr.ID,
				Message:     "Self-transition requires a non-blank event trigger.",
				Blocking:    true,
				Remediation: "Specify a non-empty event trigger for self-transitions.",
			})
		}
	}

	return issues
}

func validateWorkflowSection(workflows []BlueprintWorkflow) []BlueprintValidationIssue {
	var issues []BlueprintValidationIssue
	sec := "workflow"
	wfIDs := make(map[string]bool)

	for wIdx, wf := range workflows {
		wfPath := fmt.Sprintf("/workflows/%d", wIdx)
		if strings.TrimSpace(wf.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        wfPath + "/id",
				TargetID:    "",
				Message:     "Workflow ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'wf-'.",
			})
		} else if wfIDs[wf.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        wfPath + "/id",
				TargetID:    wf.ID,
				Message:     "Duplicate workflow ID detected.",
				Blocking:    true,
				Remediation: "Ensure all workflow IDs are unique.",
			})
		} else {
			wfIDs[wf.ID] = true
		}

		if strings.TrimSpace(wf.Name) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        wfPath + "/name",
				TargetID:    wf.ID,
				Message:     "Workflow name is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable name.",
			})
		}

		laneIDs := make(map[string]bool)
		for lIdx, lane := range wf.Lanes {
			lPath := fmt.Sprintf("%s/lanes/%d", wfPath, lIdx)
			if strings.TrimSpace(lane.ID) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        lPath + "/id",
					TargetID:    "",
					Message:     "Swimlane ID is required.",
					Blocking:    true,
					Remediation: "Provide a unique kebab-case ID prefixed with 'lane-'.",
				})
			} else if laneIDs[lane.ID] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDuplicateID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        lPath + "/id",
					TargetID:    lane.ID,
					Message:     "Duplicate swimlane ID detected in workflow.",
					Blocking:    true,
					Remediation: "Ensure all swimlane IDs within the workflow are unique.",
				})
			} else {
				laneIDs[lane.ID] = true
			}

			if strings.TrimSpace(lane.Label) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        lPath + "/label",
					TargetID:    lane.ID,
					Message:     "Swimlane label is required.",
					Blocking:    true,
					Remediation: "Provide a non-empty human-readable label.",
				})
			}
		}

		nodeIDs := make(map[string]bool)
		incidentCount := make(map[string]int)
		outgoingTargets := make(map[string]map[string]bool)

		for nIdx, node := range wf.Nodes {
			nPath := fmt.Sprintf("%s/nodes/%d", wfPath, nIdx)
			if strings.TrimSpace(node.ID) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        nPath + "/id",
					TargetID:    "",
					Message:     "Workflow node ID is required.",
					Blocking:    true,
					Remediation: "Provide a unique kebab-case ID prefixed with 'wnode-'.",
				})
			} else if nodeIDs[node.ID] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDuplicateID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        nPath + "/id",
					TargetID:    node.ID,
					Message:     "Duplicate workflow node ID detected.",
					Blocking:    true,
					Remediation: "Ensure all node IDs within the workflow are unique.",
				})
			} else {
				nodeIDs[node.ID] = true
			}

			if strings.TrimSpace(node.Label) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        nPath + "/label",
					TargetID:    node.ID,
					Message:     "Workflow node label is required.",
					Blocking:    true,
					Remediation: "Provide a non-empty human-readable label.",
				})
			}

			if strings.TrimSpace(node.LaneID) == "" || !laneIDs[node.LaneID] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDanglingReference,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        nPath + "/lane_id",
					TargetID:    node.ID,
					Message:     "Workflow node references unregistered swimlane ID.",
					Blocking:    true,
					Remediation: "Assign the workflow node to a registered swimlane ID.",
				})
			}

			if isMalformedLayout(node.Layout) {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMalformedLayout,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        nPath + "/layout",
					TargetID:    node.ID,
					Message:     "Workflow node layout coordinates out of bounds or malformed.",
					Blocking:    true,
					Remediation: "Provide finite real numbers between -100000 and 100000 for x and y.",
				})
			}
		}

		edgeIDs := make(map[string]bool)
		for eIdx, edge := range wf.Edges {
			ePath := fmt.Sprintf("%s/edges/%d", wfPath, eIdx)
			if strings.TrimSpace(edge.ID) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        ePath + "/id",
					TargetID:    "",
					Message:     "Workflow edge ID is required.",
					Blocking:    true,
					Remediation: "Provide a unique kebab-case ID prefixed with 'wedge-'.",
				})
			} else if edgeIDs[edge.ID] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDuplicateID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        ePath + "/id",
					TargetID:    edge.ID,
					Message:     "Duplicate workflow edge ID detected.",
					Blocking:    true,
					Remediation: "Ensure all edge IDs within the workflow are unique.",
				})
			} else {
				edgeIDs[edge.ID] = true
			}

			sourceValid := strings.TrimSpace(edge.Source) != "" && nodeIDs[edge.Source]
			targetValid := strings.TrimSpace(edge.Target) != "" && nodeIDs[edge.Target]

			if !sourceValid {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDanglingReference,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        ePath + "/source",
					TargetID:    edge.ID,
					Message:     "Workflow edge references unregistered source node.",
					Blocking:    true,
					Remediation: "Ensure edge source points to a registered workflow node.",
				})
			} else {
				incidentCount[edge.Source]++
				if outgoingTargets[edge.Source] == nil {
					outgoingTargets[edge.Source] = make(map[string]bool)
				}
				if targetValid {
					outgoingTargets[edge.Source][edge.Target] = true
				}
			}

			if !targetValid {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDanglingReference,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        ePath + "/target",
					TargetID:    edge.ID,
					Message:     "Workflow edge references unregistered target node.",
					Blocking:    true,
					Remediation: "Ensure edge target points to a registered workflow node.",
				})
			} else {
				incidentCount[edge.Target]++
			}
		}

		if len(wf.Nodes) > 1 {
			for nIdx, node := range wf.Nodes {
				if incidentCount[node.ID] == 0 {
					issues = append(issues, BlueprintValidationIssue{
						Code:        BlueprintIssueWorkflowOrphanNode,
						Severity:    "BLOCKING_ERROR",
						Section:     sec,
						Path:        fmt.Sprintf("%s/nodes/%d", wfPath, nIdx),
						TargetID:    node.ID,
						Message:     "Workflow node has no incident edges.",
						Blocking:    true,
						Remediation: "Connect the node to the workflow via at least one incoming or outgoing edge.",
					})
				}
			}
		}

		for nIdx, node := range wf.Nodes {
			if node.Type == "decision" {
				count := len(outgoingTargets[node.ID])
				if count < 2 {
					issues = append(issues, BlueprintValidationIssue{
						Code:        BlueprintIssueWorkflowUnderbranched,
						Severity:    "BLOCKING_ERROR",
						Section:     sec,
						Path:        fmt.Sprintf("%s/nodes/%d", wfPath, nIdx),
						TargetID:    node.ID,
						Message:     "Decision node has fewer than two distinct outgoing targets.",
						Blocking:    true,
						Remediation: "Add at least 2 distinct outgoing edges representing decision branches (e.g. 'Yes' / 'No').",
					})
				}
			}
		}
	}

	return issues
}

func detectWBSCycle(modules []BlueprintWBSModule) (bool, []string) {
	adj := make(map[string][]string)
	modMap := make(map[string]bool)
	for _, m := range modules {
		if strings.TrimSpace(m.ID) != "" {
			modMap[m.ID] = true
			deps := make([]string, len(m.Dependencies))
			copy(deps, m.Dependencies)
			sort.Strings(deps)
			adj[m.ID] = deps
		}
	}

	sortedModuleIDs := make([]string, 0, len(modMap))
	for id := range modMap {
		sortedModuleIDs = append(sortedModuleIDs, id)
	}
	sort.Strings(sortedModuleIDs)

	// 0: unvisited, 1: visiting (in stack), 2: visited
	color := make(map[string]int)
	var cyclePath []string

	var dfs func(u string, path []string) bool
	dfs = func(u string, path []string) bool {
		color[u] = 1
		path = append(path, u)
		for _, v := range adj[u] {
			if color[v] == 1 {
				// Found back edge, extract exact cycle
				cycleStart := -1
				for idx, node := range path {
					if node == v {
						cycleStart = idx
						break
					}
				}
				if cycleStart >= 0 {
					cyclePath = append([]string{}, path[cycleStart:]...)
					cyclePath = append(cyclePath, v)
				} else {
					cyclePath = []string{u, v}
				}
				return true
			}
			if color[v] == 0 {
				if dfs(v, path) {
					return true
				}
			}
		}
		color[u] = 2
		return false
	}

	for _, mID := range sortedModuleIDs {
		if color[mID] == 0 {
			if dfs(mID, nil) {
				return true, cyclePath
			}
		}
	}

	return false, nil
}

func validateWBSSection(wbs BlueprintWBS, crossRefs []BlueprintCrossReference) []BlueprintValidationIssue {
	var issues []BlueprintValidationIssue
	sec := "wbs"
	moduleIDs := make(map[string]bool)
	featureIDs := make(map[string]bool)

	for mIdx, mod := range wbs.Modules {
		mPath := fmt.Sprintf("/wbs/modules/%d", mIdx)
		if strings.TrimSpace(mod.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        mPath + "/id",
				TargetID:    "",
				Message:     "WBS module ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'mod-'.",
			})
		} else if moduleIDs[mod.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        mPath + "/id",
				TargetID:    mod.ID,
				Message:     "Duplicate WBS module ID detected.",
				Blocking:    true,
				Remediation: "Ensure all module IDs are unique.",
			})
		} else {
			moduleIDs[mod.ID] = true
		}

		if strings.TrimSpace(mod.Name) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        mPath + "/name",
				TargetID:    mod.ID,
				Message:     "WBS module name is required.",
				Blocking:    true,
				Remediation: "Provide a non-empty human-readable name.",
			})
		}

		if !validWBSPriorities[mod.Priority] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueUnsupportedVersion,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        mPath + "/priority",
				TargetID:    mod.ID,
				Message:     "Priority must be high, medium, or low.",
				Blocking:    true,
				Remediation: "Set priority to one of: high, medium, low.",
			})
		}

		for rIdx, ref := range mod.References {
			if !canonicalRefPattern.MatchString(ref) {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueInvalidCanonicalRef,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        fmt.Sprintf("%s/references/%d", mPath, rIdx),
					TargetID:    mod.ID,
					Message:     "Malformed canonical reference URI.",
					Blocking:    true,
					Remediation: "Format reference using canonical URI syntax.",
				})
			}
		}

		for fIdx, feat := range mod.Features {
			fPath := fmt.Sprintf("%s/features/%d", mPath, fIdx)
			if strings.TrimSpace(feat.ID) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        fPath + "/id",
					TargetID:    "",
					Message:     "WBS feature ID is required.",
					Blocking:    true,
					Remediation: "Provide a unique kebab-case ID prefixed with 'feat-'.",
				})
			} else if featureIDs[feat.ID] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDuplicateID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        fPath + "/id",
					TargetID:    feat.ID,
					Message:     "Duplicate WBS feature ID detected.",
					Blocking:    true,
					Remediation: "Ensure all feature IDs are unique.",
				})
			} else {
				featureIDs[feat.ID] = true
			}

			if strings.TrimSpace(feat.Name) == "" {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueMissingID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        fPath + "/name",
					TargetID:    feat.ID,
					Message:     "WBS feature name is required.",
					Blocking:    true,
					Remediation: "Provide a non-empty human-readable name.",
				})
			}

			if len(feat.AcceptanceCriteria) == 0 {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueUnsupportedVersion,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        fPath + "/acceptance_criteria",
					TargetID:    feat.ID,
					Message:     "Feature acceptance criteria must not be empty.",
					Blocking:    true,
					Remediation: "Provide at least one testable acceptance criterion.",
				})
			}
		}
	}

	// Validate Dependencies
	for mIdx, mod := range wbs.Modules {
		mPath := fmt.Sprintf("/wbs/modules/%d", mIdx)
		depSeen := make(map[string]bool)
		for dIdx, dep := range mod.Dependencies {
			dPath := fmt.Sprintf("%s/dependencies/%d", mPath, dIdx)
			if dep == mod.ID {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueWBSInvalidDependency,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        dPath,
					TargetID:    mod.ID,
					Message:     "WBS module has self-dependency on itself.",
					Blocking:    true,
					Remediation: "Remove self-dependency from dependencies array.",
				})
			} else if depSeen[dep] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueDuplicateID,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        dPath,
					TargetID:    dep,
					Message:     "Duplicate dependency detected in WBS module.",
					Blocking:    true,
					Remediation: "Remove duplicate dependency ID.",
				})
			} else if !moduleIDs[dep] {
				issues = append(issues, BlueprintValidationIssue{
					Code:        BlueprintIssueWBSInvalidDependency,
					Severity:    "BLOCKING_ERROR",
					Section:     sec,
					Path:        dPath,
					TargetID:    mod.ID,
					Message:     "WBS module depends on unknown or unregistered module.",
					Blocking:    true,
					Remediation: "Ensure dependency points to a valid registered WBS module ID.",
				})
			}
			depSeen[dep] = true
		}
	}

	// Validate Dependency Cycles
	if hasCycle, cyclePath := detectWBSCycle(wbs.Modules); hasCycle {
		issues = append(issues, BlueprintValidationIssue{
			Code:        BlueprintIssueWBSDependencyCycle,
			Severity:    "BLOCKING_ERROR",
			Section:     sec,
			Path:        "/wbs/modules",
			TargetID:    "",
			Message:     fmt.Sprintf("Circular dependency cycle detected among WBS modules: %s.", strings.Join(cyclePath, " -> ")),
			Blocking:    true,
			Remediation: "Break dependency cycles by restructuring module dependencies into a Directed Acyclic Graph (DAG).",
		})
	}

	// Validate Cross References in WBS payload
	xrefIDs := make(map[string]bool)
	for xIdx, xref := range crossRefs {
		xPath := fmt.Sprintf("/cross_references/%d", xIdx)
		if strings.TrimSpace(xref.ID) == "" {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueMissingID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        xPath + "/id",
				TargetID:    "",
				Message:     "Cross reference ID is required.",
				Blocking:    true,
				Remediation: "Provide a unique kebab-case ID prefixed with 'xref-'.",
			})
		} else if xrefIDs[xref.ID] {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueDuplicateID,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        xPath + "/id",
				TargetID:    xref.ID,
				Message:     "Duplicate cross reference ID detected.",
				Blocking:    true,
				Remediation: "Ensure all cross reference IDs are unique.",
			})
		} else {
			xrefIDs[xref.ID] = true
		}

		if !canonicalRefPattern.MatchString(xref.SourceRef) {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueInvalidCanonicalRef,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        xPath + "/source_ref",
				TargetID:    xref.ID,
				Message:     "Malformed canonical reference in source_ref.",
				Blocking:    true,
				Remediation: "Format source_ref to match canonical URI grammar.",
			})
		}

		if !canonicalRefPattern.MatchString(xref.TargetRef) {
			issues = append(issues, BlueprintValidationIssue{
				Code:        BlueprintIssueInvalidCanonicalRef,
				Severity:    "BLOCKING_ERROR",
				Section:     sec,
				Path:        xPath + "/target_ref",
				TargetID:    xref.ID,
				Message:     "Malformed canonical reference in target_ref.",
				Blocking:    true,
				Remediation: "Format target_ref to match canonical URI grammar.",
			})
		}
	}

	return issues
}

func wrapSectionForNormalization(sectionType domain.BlueprintSectionType, decoded any) any {
	switch sectionType {
	case domain.BlueprintSectionJourney:
		return struct {
			Journey BlueprintJourney `json:"journey"`
		}{Journey: decoded.(BlueprintJourney)}
	case domain.BlueprintSectionArchitecture:
		return struct {
			Architecture BlueprintArchitecture `json:"architecture"`
		}{Architecture: decoded.(BlueprintArchitecture)}
	case domain.BlueprintSectionStateMachine:
		return struct {
			StateMachine BlueprintStateMachine `json:"state_machine"`
		}{StateMachine: decoded.(BlueprintStateMachine)}
	case domain.BlueprintSectionWorkflow:
		return struct {
			Workflows []BlueprintWorkflow `json:"workflows"`
		}{Workflows: decoded.([]BlueprintWorkflow)}
	case domain.BlueprintSectionWBS:
		wbsData := decoded.(decodedWBSSection)
		return struct {
			WBS             BlueprintWBS              `json:"wbs"`
			CrossReferences []BlueprintCrossReference `json:"cross_references"`
		}{
			WBS:             wbsData.WBS,
			CrossReferences: wbsData.CrossReferences,
		}
	default:
		return decoded
	}
}

// Validate executes strict envelope decoding, deep security scanning, and section structural validation.
func (v *BlueprintSectionValidator) Validate(
	sectionType domain.BlueprintSectionType,
	payload json.RawMessage,
) BlueprintValidationResult {
	secName := string(sectionType)
	if secName == "" {
		secName = "general"
	}

	// 1. Check max nesting depth and valid UTF-8
	if !checkJSONDepthAndUTF8(payload) {
		issue := BlueprintValidationIssue{
			Code:        BlueprintIssueUnsupportedVersion,
			Severity:    "BLOCKING_ERROR",
			Section:     secName,
			Path:        "",
			Message:     "malformed or unsupported section payload",
			Blocking:    true,
			Remediation: "Provide valid UTF-8 JSON within accepted nesting depth limits.",
		}
		return BlueprintValidationResult{
			Valid:             false,
			SafeOutcome:       BlueprintValidationInvalidSchema,
			Issues:            []BlueprintValidationIssue{issue},
			NormalizedContent: nil,
		}
	}

	// 2. Security walk on raw JSON tree
	var rawTree any
	var securityIssues []BlueprintValidationIssue
	if err := json.Unmarshal(payload, &rawTree); err == nil && rawTree != nil {
		securityIssues = validateBlueprintJSONSecurity(rawAny(rawTree), "", secName)
	}

	// 3. Strict envelope decoding
	decoded, errPath, decodeErr := strictDecodeBlueprintSection(sectionType, payload)
	if decodeErr != nil {
		issue := BlueprintValidationIssue{
			Code:        BlueprintIssueUnsupportedVersion,
			Severity:    "BLOCKING_ERROR",
			Section:     secName,
			Path:        errPath,
			Message:     "malformed or unsupported section envelope",
			Blocking:    true,
			Remediation: "Provide valid JSON matching the section schema contract without unknown fields.",
		}
		allIssues := append(securityIssues, issue)
		sortBlueprintValidationIssues(allIssues)

		safeOutcome := BlueprintValidationInvalidSchema
		for _, iss := range allIssues {
			if iss.Code == BlueprintIssueForbiddenContent {
				safeOutcome = BlueprintValidationSecurityFailure
				break
			}
		}

		return BlueprintValidationResult{
			Valid:             false,
			SafeOutcome:       safeOutcome,
			Issues:            allIssues,
			NormalizedContent: nil,
		}
	}

	// 4. Section structural validation
	var structIssues []BlueprintValidationIssue
	switch sectionType {
	case domain.BlueprintSectionJourney:
		structIssues = validateJourneySection(decoded.(BlueprintJourney))
	case domain.BlueprintSectionArchitecture:
		structIssues = validateArchitectureSection(decoded.(BlueprintArchitecture))
	case domain.BlueprintSectionStateMachine:
		structIssues = validateStateMachineSection(decoded.(BlueprintStateMachine))
	case domain.BlueprintSectionWorkflow:
		structIssues = validateWorkflowSection(decoded.([]BlueprintWorkflow))
	case domain.BlueprintSectionWBS:
		wbsData := decoded.(decodedWBSSection)
		structIssues = validateWBSSection(wbsData.WBS, wbsData.CrossReferences)
	}

	allIssues := append(securityIssues, structIssues...)
	sortBlueprintValidationIssues(allIssues)

	if len(allIssues) > 0 {
		safeOutcome := BlueprintValidationInvalidSchema
		for _, iss := range allIssues {
			if iss.Code == BlueprintIssueForbiddenContent {
				safeOutcome = BlueprintValidationSecurityFailure
				break
			}
		}
		return BlueprintValidationResult{
			Valid:             false,
			SafeOutcome:       safeOutcome,
			Issues:            allIssues,
			NormalizedContent: nil,
		}
	}

	wrapped := wrapSectionForNormalization(sectionType, decoded)
	return BlueprintValidationResult{
		Valid:             true,
		SafeOutcome:       "",
		Issues:            []BlueprintValidationIssue{},
		NormalizedContent: normalizedJSON(wrapped),
	}
}

func rawAny(v any) any {
	return v
}
