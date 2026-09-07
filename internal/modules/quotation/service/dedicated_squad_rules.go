package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const largeSOWThreshold int64 = 150_000_000

type PreparationKind string

const (
	PreparationHiring             PreparationKind = "hiring"
	PreparationEngineeringKickoff PreparationKind = "engineering_kickoff"
	PreparationNone               PreparationKind = "none"
)

type PaymentKind string

const (
	PaymentNetPreparationDP  PaymentKind = "net_preparation_dp"
	PaymentEqualInstallments PaymentKind = "equal_installments"
	PaymentMonthly           PaymentKind = "monthly"
	PaymentSingleSettlement  PaymentKind = "single_settlement"
	PaymentExplicitRows      PaymentKind = "explicit_rows"
)

type ProjectTier string

const (
	ProjectTier20  ProjectTier = "20"
	ProjectTier50  ProjectTier = "50"
	ProjectTier100 ProjectTier = "100"
)

type DedicatedSquadRules struct {
	Profile                   ResolvedQuotationProfile `json:"profile"`
	PreparationKind           PreparationKind          `json:"preparation_kind"`
	PaymentKind               PaymentKind              `json:"payment_kind"`
	ProjectTier               ProjectTier              `json:"project_tier,omitempty"`
	PreparationDiscountBPS    int32                    `json:"preparation_discount_bps"`
	DirectProjectDiscountBPS  int32                    `json:"direct_project_discount_bps"`
	IncludeTimeline           bool                     `json:"include_timeline"`
	IncludeMaintenance        bool                     `json:"include_maintenance"`
	IncludeMaintenanceSection bool                     `json:"include_maintenance_section"`
	MaintenanceEmbedded       bool                     `json:"maintenance_embedded"`
	IncludeK6                 bool                     `json:"include_k6"`
	IncludePentest            bool                     `json:"include_pentest"`
}

// EvaluateDedicatedSquadRules is the compatibility entry point. New flows
// should resolve once and call EvaluateDedicatedSquadRulesWithProfile.
func EvaluateDedicatedSquadRules(input domain.DedicatedSquadInput, developmentSubtotal int64) (DedicatedSquadRules, error) {
	profile, err := ResolveDedicatedSquadProfile(input)
	if err != nil {
		return DedicatedSquadRules{}, err
	}
	return EvaluateDedicatedSquadRulesWithProfile(input, profile, developmentSubtotal)
}

func EvaluateDedicatedSquadRulesWithProfile(input domain.DedicatedSquadInput, profile ResolvedQuotationProfile, developmentSubtotal int64) (DedicatedSquadRules, error) {
	if err := input.Validate(); err != nil {
		return DedicatedSquadRules{}, err
	}
	if developmentSubtotal < 0 {
		return DedicatedSquadRules{}, fmt.Errorf("development_subtotal: must not be negative")
	}

	rules := DedicatedSquadRules{
		Profile:                   profile,
		PreparationKind:           profile.PreparationKind,
		PreparationDiscountBPS:    profile.PreparationDiscountBPS,
		IncludeTimeline:           profile.TimelineMode != domain.ScopeTimelineNone,
		IncludeMaintenance:        profile.MaintenanceMode == domain.MaintenanceProfileSeparate || profile.MaintenanceMode == domain.MaintenanceProfileIncluded,
		IncludeMaintenanceSection: profile.MaintenanceMode == domain.MaintenanceProfileSeparate || (input.QuotationProfile == nil && input.MaintenanceEnabled),
		MaintenanceEmbedded:       profile.MaintenanceMode == domain.MaintenanceProfileIncluded,
		IncludeK6:                 profile.HasPhase(PhaseK6),
		IncludePentest:            profile.HasPhase(PhasePentest),
	}
	if profile.PaymentMode == domain.PaymentProfileRows {
		rules.PaymentKind = PaymentExplicitRows
	} else {
		rules.PaymentKind = presetPaymentKind(input.Model)
	}

	// Legacy 1A-1D input retains commercial gates and 1B's tier-based
	// security selection. Profile-present quotations are controlled solely by
	// resolved choices, never by their subtotal.
	if input.QuotationProfile == nil {
		switch input.Model {
		case domain.ModelSOWLarge:
			if developmentSubtotal < largeSOWThreshold {
				return DedicatedSquadRules{}, fmt.Errorf("development_subtotal: model 1A requires at least %d", largeSOWThreshold)
			}
		case domain.ModelSOWSmall:
			if developmentSubtotal >= largeSOWThreshold {
				return DedicatedSquadRules{}, fmt.Errorf("development_subtotal: model 1B requires less than %d", largeSOWThreshold)
			}
			rules.ProjectTier = projectTierFor(developmentSubtotal)
			if rules.ProjectTier == ProjectTier100 {
				rules.IncludeK6, rules.IncludePentest = true, true
				rules.Profile.IncludedPhases = []PhaseCode{PhaseRequirement, PhaseDevelopment, PhaseK6, PhasePentest, PhaseSIT, PhaseUAT, PhaseGoLive}
			}
		case domain.ModelStaffAugmentation:
			monthlyPayments := longestPeriodMonths(input.DevelopmentSquad)
			if input.PaymentCount != monthlyPayments {
				return DedicatedSquadRules{}, fmt.Errorf("payment_count: model 1C requires %d monthly payments", monthlyPayments)
			}
		case domain.ModelSprint:
			if input.PaymentCount != 1 {
				return DedicatedSquadRules{}, fmt.Errorf("payment_count: model 1D requires exactly one payment")
			}
		}
	}
	return rules, nil
}

func presetPaymentKind(model domain.SquadModel) PaymentKind {
	switch model {
	case domain.ModelSOWLarge:
		return PaymentNetPreparationDP
	case domain.ModelSOWSmall:
		return PaymentEqualInstallments
	case domain.ModelStaffAugmentation:
		return PaymentMonthly
	case domain.ModelSprint:
		return PaymentSingleSettlement
	default:
		return ""
	}
}

func BuildSectionPlan(rules DedicatedSquadRules) []domain.DocumentSection {
	sections := []domain.DocumentSection{{Code: "development", Title: "Biaya Development"}}
	if rules.IncludeMaintenanceSection || (rules.Profile.PreparationKind == "" && rules.IncludeMaintenance) {
		sections = append(sections, domain.DocumentSection{Code: "maintenance", Title: "Biaya Maintenance"})
	}
	if rules.IncludeTimeline {
		sections = append(sections, domain.DocumentSection{Code: "timeline", Title: "Lingkup Pekerjaan & Timeline"})
	}
	sections = append(sections,
		domain.DocumentSection{Code: "payment", Title: "Pembayaran"},
		domain.DocumentSection{Code: "limitations", Title: "Batasan dan Pengecualian"},
	)
	for i := range sections {
		sections[i].Number = i + 1
	}
	return sections
}

func projectTierFor(subtotal int64) ProjectTier {
	switch {
	case subtotal >= 100_000_000:
		return ProjectTier100
	case subtotal >= 50_000_000:
		return ProjectTier50
	default:
		return ProjectTier20
	}
}

func longestPeriodMonths(lines []domain.SquadLineInput) int32 {
	var longestQuarters int32
	for _, line := range lines {
		if line.PeriodQuarters > longestQuarters {
			longestQuarters = line.PeriodQuarters
		}
	}
	return (longestQuarters + 3) / 4
}
