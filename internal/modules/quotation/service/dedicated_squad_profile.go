package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// Development is a profile phase, but unlike the catalog-backed phases its
// deliverables are the extracted modules for the current quotation.
const PhaseDevelopment PhaseCode = "development"

// ResolvedQuotationProfile is the server-owned, complete set of conditional
// quotation choices. It intentionally contains no client-supplied money or
// legal copy.
type ResolvedQuotationProfile struct {
	PreparationKind        PreparationKind             `json:"preparation_kind"`
	PreparationDiscountBPS int32                       `json:"preparation_discount_bps"`
	MaintenanceMode        domain.MaintenanceMode      `json:"maintenance_mode"`
	TimelineMode           domain.ScopeTimelineMode    `json:"scope_timeline_mode"`
	IncludedPhases         []PhaseCode                 `json:"included_phases"`
	PaymentMode            domain.PaymentMode          `json:"payment_mode"`
	PaymentRows            []domain.PaymentRowInput    `json:"payment_rows,omitempty"`
	DocumentOptions        ResolvedDocumentOptions    `json:"document_options"`

	// UsesLegacyDefaults permits the timeline generator to retain the exact
	// established 1A-1D presentation while profile-present requests use the
	// selected phase order.
	UsesLegacyDefaults bool `json:"-"`
}

func (p ResolvedQuotationProfile) HasPhase(phase PhaseCode) bool {
	for _, candidate := range p.IncludedPhases {
		if candidate == phase {
			return true
		}
	}
	return false
}

// ResolveDedicatedSquadProfile overlays the optional, validated input choices
// onto the selected dedicated-squad preset. No rate-card or subtotal decision
// belongs here, which makes the result safe to snapshot and reuse.
func ResolveDedicatedSquadProfile(input domain.DedicatedSquadInput) (ResolvedQuotationProfile, error) {
	if err := input.Validate(); err != nil {
		return ResolvedQuotationProfile{}, err
	}
	resolved, err := dedicatedSquadPresetProfile(input)
	if err != nil {
		return ResolvedQuotationProfile{}, err
	}
	if input.QuotationProfile == nil {
		return resolved, nil
	}

	profile := input.QuotationProfile
	resolved.UsesLegacyDefaults = false
	if profile.PreparationMode != domain.PreparationProfileDefault {
		resolved.PreparationKind = preparationKindFor(profile.PreparationMode)
	}
	if profile.PreparationDiscountBPS != nil {
		resolved.PreparationDiscountBPS = *profile.PreparationDiscountBPS
	}
	if profile.MaintenanceMode != domain.MaintenanceProfileDefault {
		resolved.MaintenanceMode = profile.MaintenanceMode
	}
	if profile.ScopeTimelineMode != domain.ScopeTimelineDefault {
		resolved.TimelineMode = profile.ScopeTimelineMode
	}
	if profile.IncludedPhases != nil {
		resolved.IncludedPhases = phaseCodes(profile.IncludedPhases)
	}
	if profile.PaymentMode != domain.PaymentProfileDefault {
		resolved.PaymentMode = profile.PaymentMode
	}
	if profile.PaymentRows != nil {
		resolved.PaymentRows = append([]domain.PaymentRowInput(nil), profile.PaymentRows...)
	}
	options, err := ResolveDedicatedSquadDocumentOptions(profile.DocumentOptions)
	if err != nil {
		return ResolvedQuotationProfile{}, err
	}
	resolved.DocumentOptions = options

	if resolved.PreparationKind == PreparationNone {
		resolved.PreparationDiscountBPS = 0
	}
	if err := validateResolvedTimeline(input, resolved); err != nil {
		return ResolvedQuotationProfile{}, err
	}
	return resolved, nil
}

func dedicatedSquadPresetProfile(input domain.DedicatedSquadInput) (ResolvedQuotationProfile, error) {
	options, err := ResolveDedicatedSquadDocumentOptions(domain.DocumentOptionsInput{})
	if err != nil {
		return ResolvedQuotationProfile{}, err
	}
	profile := ResolvedQuotationProfile{PaymentMode: domain.PaymentProfilePreset, UsesLegacyDefaults: true, DocumentOptions: options}
	switch input.Model {
	case domain.ModelSOWLarge:
		profile.PreparationKind = PreparationHiring
		profile.PreparationDiscountBPS = 5000
		if input.MaintenanceEnabled {
			profile.MaintenanceMode = domain.MaintenanceProfileSeparate
		}
		profile.TimelineMode = domain.ScopeTimelineMode(input.Timeline.Mode)
		profile.IncludedPhases = []PhaseCode{PhaseRequirement, PhaseDevelopment, PhaseK6, PhasePentest, PhaseSIT, PhaseUAT, PhaseGoLive}
		if input.MaintenanceEnabled {
			profile.IncludedPhases = append(profile.IncludedPhases, PhaseMaintenance)
		}
	case domain.ModelSOWSmall:
		profile.PreparationKind = PreparationNone
		profile.TimelineMode = domain.ScopeTimelineMode(input.Timeline.Mode)
		// Security phases are added by the legacy 1B subtotal rule.
		profile.IncludedPhases = []PhaseCode{PhaseRequirement, PhaseDevelopment, PhaseSIT, PhaseUAT, PhaseGoLive}
	case domain.ModelStaffAugmentation:
		profile.PreparationKind = PreparationEngineeringKickoff
		profile.PreparationDiscountBPS = input.KickoffDiscountBPS
		profile.MaintenanceMode = domain.MaintenanceProfileNone
		profile.TimelineMode = domain.ScopeTimelineNone
	case domain.ModelSprint:
		profile.PreparationKind = PreparationNone
		profile.MaintenanceMode = domain.MaintenanceProfileNone
		profile.TimelineMode = domain.ScopeTimelineSprint
		profile.IncludedPhases = []PhaseCode{PhaseDevelopment}
	default:
		return ResolvedQuotationProfile{}, fmt.Errorf("model: unsupported dedicated squad model %q", input.Model)
	}
	return profile, nil
}

func preparationKindFor(mode domain.PreparationMode) PreparationKind {
	switch mode {
	case domain.PreparationProfileHiring:
		return PreparationHiring
	case domain.PreparationProfileKickoff:
		return PreparationEngineeringKickoff
	default:
		return PreparationNone
	}
}

func phaseCodes(values []string) []PhaseCode {
	result := make([]PhaseCode, len(values))
	for i, value := range values {
		result[i] = PhaseCode(value)
	}
	return result
}

func validateResolvedTimeline(input domain.DedicatedSquadInput, profile ResolvedQuotationProfile) error {
	switch profile.TimelineMode {
	case domain.ScopeTimelineNone:
		return nil
	case domain.ScopeTimelineRelative:
		if input.Timeline.Mode != domain.TimelineRelative {
			return fmt.Errorf("timeline.mode: must be relative for quotation_profile.scope_timeline_mode relative")
		}
	case domain.ScopeTimelineCalendar:
		if input.Timeline.Mode != domain.TimelineCalendar || input.Timeline.StartDate == nil {
			return fmt.Errorf("timeline.start_date: is required for quotation_profile.scope_timeline_mode calendar")
		}
	case domain.ScopeTimelineSprint:
		if input.Model != domain.ModelSprint || input.Timeline.Mode != domain.TimelineSprint {
			return fmt.Errorf("timeline.mode: sprint quotation profile is only supported for model 1D")
		}
		if input.Timeline.StartDate == nil || input.Timeline.SprintEndDate == nil {
			return fmt.Errorf("timeline: sprint dates are required for quotation_profile.scope_timeline_mode sprint")
		}
	default:
		return fmt.Errorf("quotation_profile.scope_timeline_mode: unsupported value %q", profile.TimelineMode)
	}
	return nil
}
