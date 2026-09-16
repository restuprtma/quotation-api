package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	ErrBlueprintPromptInvalidInput = errors.New("invalid blueprint prompt input")
	ErrBlueprintPromptSecurity     = errors.New("blueprint prompt security violation")
)

const (
	maxBlueprintContextSize    = 256 * 1024      // 256 KiB
	maxBlueprintDependencySize = 512 * 1024      // 512 KiB
	maxBlueprintPromptSize     = 2 * 1024 * 1024 // 2 MiB
)

// BlueprintPromptInput specifies the stage, skill configuration, and context inputs needed
// to deterministically build an execution prompt.
type BlueprintPromptInput struct {
	Stage         domain.BlueprintSectionType
	SkillSnapshot json.RawMessage
	Context       json.RawMessage
	Dependencies  map[domain.BlueprintSectionType]json.RawMessage
}

// BlueprintPromptBuilder builds secure, fixed-order prompts from embedded safe envelopes,
// backend contracts, tenant skills, and validated dependencies.
type BlueprintPromptBuilder struct {
	validator *BlueprintSectionValidator
}

// NewBlueprintPromptBuilder constructs a new BlueprintPromptBuilder.
func NewBlueprintPromptBuilder() *BlueprintPromptBuilder {
	return &BlueprintPromptBuilder{
		validator: NewBlueprintSectionValidator(),
	}
}

type blueprintPromptStageContract struct {
	instructionAsset string
	dependencies     []domain.BlueprintSectionType
}

var blueprintPromptStages = map[domain.BlueprintSectionType]blueprintPromptStageContract{
	domain.BlueprintSectionJourney:      {"blueprint_journey_v1.txt", []domain.BlueprintSectionType{domain.BlueprintSectionContext}},
	domain.BlueprintSectionArchitecture: {"blueprint_architecture_v1.txt", []domain.BlueprintSectionType{domain.BlueprintSectionContext, domain.BlueprintSectionJourney}},
	domain.BlueprintSectionStateMachine: {"blueprint_state_machine_v1.txt", []domain.BlueprintSectionType{domain.BlueprintSectionContext}},
	domain.BlueprintSectionWorkflow:     {"blueprint_workflow_v1.txt", []domain.BlueprintSectionType{domain.BlueprintSectionContext, domain.BlueprintSectionJourney, domain.BlueprintSectionStateMachine}},
	domain.BlueprintSectionWBS:          {"blueprint_wbs_v1.txt", []domain.BlueprintSectionType{domain.BlueprintSectionContext, domain.BlueprintSectionJourney, domain.BlueprintSectionArchitecture, domain.BlueprintSectionStateMachine, domain.BlueprintSectionWorkflow}},
}

// Build constructs the complete, deterministic prompt for a blueprint stage.
func (b *BlueprintPromptBuilder) Build(input BlueprintPromptInput) (string, error) {
	// 1. Stage verification
	contract, exists := blueprintPromptStages[input.Stage]
	if !exists {
		return "", fmt.Errorf("%w: unsupported prompt stage", ErrBlueprintPromptInvalidInput)
	}

	// 2. Parse skill snapshot
	if len(bytes.TrimSpace(input.SkillSnapshot)) == 0 {
		return "", fmt.Errorf("%w: missing skill snapshot", ErrBlueprintPromptInvalidInput)
	}
	var snapshot BlueprintSkillSnapshot
	snapDec := json.NewDecoder(bytes.NewReader(input.SkillSnapshot))
	snapDec.DisallowUnknownFields()
	if err := snapDec.Decode(&snapshot); err != nil {
		return "", fmt.Errorf("%w: malformed skill snapshot", ErrBlueprintPromptInvalidInput)
	}
	var extraSnap any
	if err := snapDec.Decode(&extraSnap); err != io.EOF {
		return "", fmt.Errorf("%w: trailing content in skill snapshot", ErrBlueprintPromptInvalidInput)
	}
	if snapshot.Command != BlueprintSkillCommand {
		return "", fmt.Errorf("%w: skill snapshot command is not %s", ErrBlueprintPromptInvalidInput, BlueprintSkillCommand)
	}
	if strings.TrimSpace(snapshot.ContentInstruction) == "" {
		return "", fmt.Errorf("%w: empty skill content instruction", ErrBlueprintPromptInvalidInput)
	}

	// 3. Validate Context
	if len(bytes.TrimSpace(input.Context)) == 0 {
		if depCtx, ok := input.Dependencies[domain.BlueprintSectionContext]; ok && len(bytes.TrimSpace(depCtx)) > 0 {
			input.Context = depCtx
		} else {
			return "", fmt.Errorf("%w: missing context", ErrBlueprintPromptInvalidInput)
		}
	}
	if len(input.Context) > maxBlueprintContextSize {
		return "", fmt.Errorf("%w: context exceeds maximum size limit", ErrBlueprintPromptInvalidInput)
	}
	if !checkJSONDepthAndUTF8(input.Context) {
		return "", fmt.Errorf("%w: invalid context json encoding or depth", ErrBlueprintPromptInvalidInput)
	}

	var rawCtx any
	ctxDec := json.NewDecoder(bytes.NewReader(input.Context))
	if err := ctxDec.Decode(&rawCtx); err != nil {
		return "", fmt.Errorf("%w: malformed context json", ErrBlueprintPromptInvalidInput)
	}
	var trailingCtx any
	if err := ctxDec.Decode(&trailingCtx); err != io.EOF {
		return "", fmt.Errorf("%w: trailing content in context json", ErrBlueprintPromptInvalidInput)
	}
	ctxMap, isMap := rawCtx.(map[string]any)
	if !isMap || ctxMap == nil {
		return "", fmt.Errorf("%w: context must be a json object", ErrBlueprintPromptInvalidInput)
	}

	ctxSecIssues := validateBlueprintJSONSecurity(rawCtx, "", "context")
	if len(ctxSecIssues) > 0 {
		return "", fmt.Errorf("%w: context contains forbidden content or keys", ErrBlueprintPromptSecurity)
	}

	// 4. Validate exact dependencies
	if len(input.Dependencies) != len(contract.dependencies) {
		return "", fmt.Errorf("%w: invalid dependency count", ErrBlueprintPromptInvalidInput)
	}
	for _, depType := range contract.dependencies {
		depPayload, depExists := input.Dependencies[depType]
		if !depExists || len(bytes.TrimSpace(depPayload)) == 0 {
			return "", fmt.Errorf("%w: missing required dependency", ErrBlueprintPromptInvalidInput)
		}
	}
	for k := range input.Dependencies {
		found := false
		for _, depType := range contract.dependencies {
			if k == depType {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("%w: unexpected extra dependency", ErrBlueprintPromptInvalidInput)
		}
	}

	// 5. Validate dependencies and serialize in fixed stage order
	type depEntry struct {
		key domain.BlueprintSectionType
		raw json.RawMessage
	}
	orderedDeps := make([]depEntry, 0, len(contract.dependencies))

	for _, depType := range contract.dependencies {
		depPayload := input.Dependencies[depType]
		if len(depPayload) > maxBlueprintDependencySize {
			return "", fmt.Errorf("%w: dependency exceeds maximum size limit", ErrBlueprintPromptInvalidInput)
		}

		if depType == domain.BlueprintSectionContext {
			if !checkJSONDepthAndUTF8(depPayload) {
				return "", fmt.Errorf("%w: malformed context dependency", ErrBlueprintPromptInvalidInput)
			}
			var depRaw any
			dDec := json.NewDecoder(bytes.NewReader(depPayload))
			if err := dDec.Decode(&depRaw); err != nil {
				return "", fmt.Errorf("%w: malformed context dependency json", ErrBlueprintPromptInvalidInput)
			}
			var extraDep any
			if err := dDec.Decode(&extraDep); err != io.EOF {
				return "", fmt.Errorf("%w: trailing content in context dependency json", ErrBlueprintPromptInvalidInput)
			}
			depMap, ok := depRaw.(map[string]any)
			if !ok || depMap == nil {
				return "", fmt.Errorf("%w: context dependency must be a json object", ErrBlueprintPromptInvalidInput)
			}
			if issues := validateBlueprintJSONSecurity(depRaw, "", "context"); len(issues) > 0 {
				return "", fmt.Errorf("%w: context dependency contains forbidden content", ErrBlueprintPromptSecurity)
			}
			orderedDeps = append(orderedDeps, depEntry{key: depType, raw: normalizedJSON(depMap)})
		} else {
			valRes := b.validator.Validate(depType, depPayload)
			if !valRes.Valid {
				if valRes.SafeOutcome == BlueprintValidationSecurityFailure {
					return "", fmt.Errorf("%w: dependency contains forbidden content", ErrBlueprintPromptSecurity)
				}
				return "", fmt.Errorf("%w: dependency failed section validation", ErrBlueprintPromptInvalidInput)
			}
			orderedDeps = append(orderedDeps, depEntry{key: depType, raw: valRes.NormalizedContent})
		}
	}

	// 6. Load embedded assets
	safetyEnvelope := GetSafetyEnvelope("blueprint_safety_v1.txt")
	if strings.TrimSpace(safetyEnvelope) == "" {
		return "", fmt.Errorf("%w: missing embedded safety envelope", ErrBlueprintPromptInvalidInput)
	}

	backendSchema := GetSafetyEnvelope("blueprint_bundle_schema_v1.json")
	if strings.TrimSpace(backendSchema) == "" {
		return "", fmt.Errorf("%w: missing embedded backend schema", ErrBlueprintPromptInvalidInput)
	}

	stageInstruction := GetSafetyEnvelope(contract.instructionAsset)
	if strings.TrimSpace(stageInstruction) == "" {
		return "", fmt.Errorf("%w: missing embedded stage instruction", ErrBlueprintPromptInvalidInput)
	}

	// 7. Format validated dependencies JSON
	var depBuf bytes.Buffer
	depBuf.WriteString("{\n")
	for i, d := range orderedDeps {
		depBuf.WriteString(fmt.Sprintf("  %q: ", string(d.key)))
		depBuf.Write(d.raw)
		if i < len(orderedDeps)-1 {
			depBuf.WriteString(",")
		}
		depBuf.WriteString("\n")
	}
	depBuf.WriteString("}")

	// 8. Assemble fixed-order prompt blocks
	stageInstructionVersion := strings.TrimSuffix(contract.instructionAsset, ".txt")
	normalizedContext := string(normalizedJSON(ctxMap))

	var sb strings.Builder
	sb.WriteString(`<SAFETY_ENVELOPE version="blueprint_safety_v1">` + "\n")
	sb.WriteString(strings.TrimSpace(safetyEnvelope) + "\n")
	sb.WriteString("</SAFETY_ENVELOPE>\n\n")

	sb.WriteString(fmt.Sprintf(`<BACKEND_SCHEMA version="1.0" stage="%s">`+"\n", string(input.Stage)))
	sb.WriteString(strings.TrimSpace(backendSchema) + "\n")
	sb.WriteString("</BACKEND_SCHEMA>\n\n")

	sb.WriteString(fmt.Sprintf(`<STAGE_INSTRUCTION version="%s">`+"\n", stageInstructionVersion))
	sb.WriteString(strings.TrimSpace(stageInstruction) + "\n")
	sb.WriteString("</STAGE_INSTRUCTION>\n\n")

	sb.WriteString(`<TENANT_SKILL_INSTRUCTION trust="configured">` + "\n")
	sb.WriteString(strings.TrimSpace(snapshot.ContentInstruction) + "\n")
	sb.WriteString("</TENANT_SKILL_INSTRUCTION>\n\n")

	sb.WriteString(`<UNTRUSTED_CONTEXT encoding="json">` + "\n")
	sb.WriteString(normalizedContext + "\n")
	sb.WriteString("</UNTRUSTED_CONTEXT>\n\n")

	sb.WriteString(`<VALIDATED_DEPENDENCIES encoding="json">` + "\n")
	sb.WriteString(depBuf.String() + "\n")
	sb.WriteString("</VALIDATED_DEPENDENCIES>\n\n")

	sb.WriteString("<OUTPUT_RULES>\n")
	sb.WriteString(fmt.Sprintf("1. Output MUST strictly adhere to the provided JSON schema for stage: %s.\n", string(input.Stage)))
	sb.WriteString("2. The output MUST be a valid JSON object matching the section contract.\n")
	sb.WriteString("3. Do not include markdown code fences (```json ... ```) or conversational explanation outside the JSON.\n")
	sb.WriteString("4. Only use information explicitly provided in the request, context, or validated dependencies.\n")
	sb.WriteString("</OUTPUT_RULES>")

	assembledPrompt := sb.String()
	if len(assembledPrompt) > maxBlueprintPromptSize {
		return "", fmt.Errorf("%w: assembled prompt exceeds maximum size limit", ErrBlueprintPromptInvalidInput)
	}

	return assembledPrompt, nil
}
