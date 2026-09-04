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
)

type ProjectTier string

const (
	ProjectTier20  ProjectTier = "20"
	ProjectTier50  ProjectTier = "50"
	ProjectTier100 ProjectTier = "100"
)

type DedicatedSquadRules struct {
	PreparationKind          PreparationKind `json:"preparation_kind"`
	PaymentKind              PaymentKind     `json:"payment_kind"`
	ProjectTier              ProjectTier     `json:"project_tier,omitempty"`
	PreparationDiscountBPS   int32           `json:"preparation_discount_bps"`
	DirectProjectDiscountBPS int32           `json:"direct_project_discount_bps"`
	IncludeTimeline          bool            `json:"include_timeline"`
	IncludeMaintenance       bool            `json:"include_maintenance"`
	IncludeK6                bool            `json:"include_k6"`
	IncludePentest           bool            `json:"include_pentest"`
}

func EvaluateDedicatedSquadRules(input domain.DedicatedSquadInput, developmentSubtotal int64) (DedicatedSquadRules, error) {
	if err := input.Validate(); err != nil {
		return DedicatedSquadRules{}, err
	}
	if developmentSubtotal < 0 {
		return DedicatedSquadRules{}, fmt.Errorf("development_subtotal: must not be negative")
	}

	switch input.Model {
	case domain.ModelSOWLarge:
		if developmentSubtotal < largeSOWThreshold {
			return DedicatedSquadRules{}, fmt.Errorf("development_subtotal: model 1A requires at least %d", largeSOWThreshold)
		}
		return DedicatedSquadRules{
			PreparationKind:        PreparationHiring,
			PaymentKind:            PaymentNetPreparationDP,
			PreparationDiscountBPS: 5000,
			IncludeTimeline:        true,
			IncludeMaintenance:     input.MaintenanceEnabled,
			IncludeK6:              true,
			IncludePentest:         true,
		}, nil
	case domain.ModelSOWSmall:
		if developmentSubtotal >= largeSOWThreshold {
			return DedicatedSquadRules{}, fmt.Errorf("development_subtotal: model 1B requires less than %d", largeSOWThreshold)
		}
		tier := projectTierFor(developmentSubtotal)
		securityTests := tier == ProjectTier100
		return DedicatedSquadRules{
			PreparationKind: PreparationNone,
			PaymentKind:     PaymentEqualInstallments,
			ProjectTier:     tier,
			IncludeTimeline: true,
			IncludeK6:       securityTests,
			IncludePentest:  securityTests,
			// Direct project discounts are intentionally excluded from v1.
			DirectProjectDiscountBPS: 0,
		}, nil
	case domain.ModelStaffAugmentation:
		monthlyPayments := longestPeriodMonths(input.DevelopmentSquad)
		if input.PaymentCount != monthlyPayments {
			return DedicatedSquadRules{}, fmt.Errorf("payment_count: model 1C requires %d monthly payments", monthlyPayments)
		}
		return DedicatedSquadRules{
			PreparationKind:        PreparationEngineeringKickoff,
			PaymentKind:            PaymentMonthly,
			PreparationDiscountBPS: input.KickoffDiscountBPS,
		}, nil
	case domain.ModelSprint:
		if input.PaymentCount != 1 {
			return DedicatedSquadRules{}, fmt.Errorf("payment_count: model 1D requires exactly one payment")
		}
		return DedicatedSquadRules{
			PreparationKind: PreparationNone,
			PaymentKind:     PaymentSingleSettlement,
			IncludeTimeline: true,
		}, nil
	default:
		return DedicatedSquadRules{}, fmt.Errorf("model: unsupported dedicated squad model %q", input.Model)
	}
}

func BuildSectionPlan(rules DedicatedSquadRules) []domain.DocumentSection {
	sections := []domain.DocumentSection{{Code: "development", Title: "Biaya Development"}}
	if rules.IncludeMaintenance {
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
