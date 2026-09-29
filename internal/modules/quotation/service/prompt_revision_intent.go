package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

const (
	SchemaPromptRevisionV1          = "prompt_revision_v1"
	PromptRevisionIntentNarrative   = "narrative"
	PromptRevisionIntentCommercial  = "commercial"
	PromptRevisionIntentMaintenance = "maintenance"
	maxPromptRevisionInstruction    = 4_000
	maxPromptRevisionOutput         = 32 * 1024
)

var (
	ErrPromptRevisionInvalidIntent = errors.New("prompt revision intent is invalid")
	ErrPromptRevisionProvider      = errors.New("prompt revision provider failed")
)

// PromptRevisionIntent is the only model-owned structure accepted by the
// section-revision endpoint. It intentionally has no price or HTML fields.
type PromptRevisionIntent struct {
	SchemaVersion   string                     `json:"schema_version"`
	Intent          string                     `json:"intent"`
	TargetSection   string                     `json:"target_section"`
	ReplacementText string                     `json:"replacement_text,omitempty"`
	CommercialPatch []dto.PromptPatchOperation `json:"commercial_patch,omitempty"`
}

type PromptRevisionIntentService struct{ providers ProviderSource }

func NewPromptRevisionIntentService(providers ProviderSource) *PromptRevisionIntentService {
	return &PromptRevisionIntentService{providers: providers}
}

func (s *PromptRevisionIntentService) Classify(ctx context.Context, companyID uuid.UUID, state domain.PromptQuotationState, instruction, requestedSection, sectionText string) (PromptRevisionIntent, error) {
	if s == nil || s.providers == nil || companyID == uuid.Nil || !validPromptRevisionInstruction(instruction) || !validPromptRevisionRequestedSection(requestedSection) {
		return PromptRevisionIntent{}, ErrPromptRevisionInvalidIntent
	}
	prompt, err := buildPromptRevisionIntentPrompt(state, instruction, requestedSection, sectionText)
	if err != nil {
		return PromptRevisionIntent{}, ErrPromptRevisionInvalidIntent
	}
	provider, _, _, err := s.providers.ProviderForCompany(ctx, companyID)
	if err != nil || provider == nil {
		return PromptRevisionIntent{}, ErrPromptRevisionProvider
	}
	result, err := provider.Generate(ctx, GenerateRequest{Prompt: prompt})
	if err != nil {
		return PromptRevisionIntent{}, ErrPromptRevisionProvider
	}
	intent, err := decodePromptRevisionIntent(result.Text)
	if err != nil || !validatePromptRevisionIntent(intent, requestedSection) {
		return PromptRevisionIntent{}, ErrPromptRevisionInvalidIntent
	}
	return intent, nil
}

func validPromptRevisionInstruction(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && len([]rune(trimmed)) <= maxPromptRevisionInstruction
}

func validPromptRevisionRequestedSection(value string) bool {
	switch value {
	case "", "scope", "timeline", "narrative":
		return true
	default:
		return false
	}
}

func buildPromptRevisionIntentPrompt(state domain.PromptQuotationState, instruction, requestedSection, sectionText string) (string, error) {
	if !validPromptRevisionInstruction(instruction) || !validPromptRevisionRequestedSection(requestedSection) {
		return "", ErrPromptRevisionInvalidIntent
	}
	// State is structured context only. The current HTML is deliberately never
	// sent to the provider; only plain text from the BA-selected section is.
	state.ServiceInput = nil
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`Return exactly one JSON object, with no markdown or commentary.
Schema: {"schema_version":"prompt_revision_v1","intent":"narrative|commercial|maintenance","target_section":"scope|timeline|narrative|commercial","replacement_text":"plain text only","commercial_patch":[{"type":"set_duration_months|set_discount_bps|set_team|set_payment_pattern|maintenance_clarification"}]}.
Do not return HTML. Do not invent prices, totals, tariffs, quantities, tax, or rates. Narrative intent may target only the selected narrative section. Commercial intent must use a typed patch. Maintenance without an existing priced service must use maintenance_clarification.
STRUCTURED_STATE: %s
REQUESTED_SECTION: %s
CURRENT_SECTION_TEXT: %s
INSTRUCTION: %s`, string(stateJSON), requestedSection, strings.TrimSpace(sectionText), strings.TrimSpace(instruction)), nil
}

func decodePromptRevisionIntent(raw string) (PromptRevisionIntent, error) {
	if len(raw) == 0 || len(raw) > maxPromptRevisionOutput {
		return PromptRevisionIntent{}, ErrPromptRevisionInvalidIntent
	}
	decoder := json.NewDecoder(io.LimitReader(strings.NewReader(raw), maxPromptRevisionOutput+1))
	decoder.DisallowUnknownFields()
	var intent PromptRevisionIntent
	if err := decoder.Decode(&intent); err != nil {
		return PromptRevisionIntent{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return PromptRevisionIntent{}, ErrPromptRevisionInvalidIntent
	}
	return intent, nil
}

func validatePromptRevisionIntent(intent PromptRevisionIntent, requestedSection string) bool {
	if intent.SchemaVersion != SchemaPromptRevisionV1 {
		return false
	}
	switch intent.Intent {
	case PromptRevisionIntentNarrative:
		return isNarrativePromptSection(intent.TargetSection) && intent.TargetSection == requestedSection &&
			strings.TrimSpace(intent.ReplacementText) != "" && len([]rune(intent.ReplacementText)) <= maxPromptRevisionOutput &&
			!strings.ContainsAny(intent.ReplacementText, "<>") && len(intent.CommercialPatch) == 0
	case PromptRevisionIntentCommercial:
		return intent.TargetSection == "commercial" && intent.ReplacementText == "" && validCommercialRevisionPatch(intent.CommercialPatch)
	case PromptRevisionIntentMaintenance:
		return intent.TargetSection == "commercial" && intent.ReplacementText == "" && len(intent.CommercialPatch) == 1 && intent.CommercialPatch[0].Type == "maintenance_clarification"
	default:
		return false
	}
}

func isNarrativePromptSection(section string) bool {
	return section == "scope" || section == "timeline" || section == "narrative"
}

func validCommercialRevisionPatch(operations []dto.PromptPatchOperation) bool {
	if len(operations) == 0 || len(operations) > 4 {
		return false
	}
	for _, operation := range operations {
		switch operation.Type {
		case dto.PromptPatchSetDurationMonths:
			if operation.DurationMonths == nil || operation.DiscountBPS != nil || operation.PaymentPattern != nil || operation.Team != nil || operation.Field != nil {
				return false
			}
		case dto.PromptPatchSetDiscountBPS:
			if operation.DiscountBPS == nil || operation.DurationMonths != nil || operation.PaymentPattern != nil || operation.Team != nil || operation.Field != nil {
				return false
			}
		case dto.PromptPatchSetTeam:
			if operation.Team == nil || operation.DurationMonths != nil || operation.DiscountBPS != nil || operation.PaymentPattern != nil || operation.Field != nil {
				return false
			}
		case dto.PromptPatchSetPaymentPattern:
			if operation.PaymentPattern == nil || operation.DurationMonths != nil || operation.DiscountBPS != nil || operation.Team != nil || operation.Field != nil {
				return false
			}
		default:
			return false
		}
	}
	return true
}
