package service

import (
	"fmt"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ResolveCustomDevelopmentProfile validates the input, resolves table mode headers,
// sets tax clause copy, calculates conditional visibility flags, and builds the ordered section plan.
func ResolveCustomDevelopmentProfile(in domain.CustomDevelopmentInput) (domain.ResolvedCustomDevelopmentProfile, error) {
	if err := in.Validate(); err != nil {
		return domain.ResolvedCustomDevelopmentProfile{}, err
	}

	profile := domain.ResolvedCustomDevelopmentProfile{
		TableMode:       in.DevelopmentTableMode,
		ShowMonthly:     len(in.MonthlyItems) > 0,
		ShowDiscountRow: in.ShowDiscountRow,
		ShowRights:      len(in.RightsStatements) > 0,
	}

	switch in.DevelopmentTableMode {
	case domain.CustomDevelopmentMonthlyPeriod:
		profile.RateHeader = "Rate (Monthly)"
		profile.PeriodHeader = "Period (Month)"
		profile.ShowRateColumn = true
	case domain.CustomDevelopmentDailyPeriod:
		profile.RateHeader = "Rate (Monthly)"
		profile.PeriodHeader = "Period (Day)"
		profile.ShowRateColumn = true
	case domain.CustomDevelopmentFixedDays:
		profile.RateHeader = ""
		profile.PeriodHeader = "Period (Days)"
		profile.ShowRateColumn = false
	default:
		return domain.ResolvedCustomDevelopmentProfile{}, fmt.Errorf("development_table_mode: unsupported value %q", in.DevelopmentTableMode)
	}

	switch in.TaxTreatment {
	case domain.CustomDevelopmentTaxExclusive:
		profile.TaxClauseText = "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia."
	case domain.CustomDevelopmentTaxInclusive:
		profile.TaxClauseText = "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini."
	case domain.CustomDevelopmentTaxNotStated:
		profile.TaxClauseText = ""
	default:
		return domain.ResolvedCustomDevelopmentProfile{}, fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}

	var sections []domain.DocumentSection
	sections = append(sections, domain.DocumentSection{
		Code:  "development",
		Title: "Biaya Development",
	})

	if profile.ShowMonthly {
		sections = append(sections, domain.DocumentSection{
			Code:  "monthly",
			Title: "Biaya Monthly",
		})
	}

	sections = append(sections, domain.DocumentSection{
		Code:  "scope",
		Title: "Lingkup Pekerjaan & Timeline",
	})

	sections = append(sections, domain.DocumentSection{
		Code:  "payment",
		Title: "Pembayaran",
	})

	if profile.ShowRights {
		sections = append(sections, domain.DocumentSection{
			Code:  "rights",
			Title: "Hak Para Pihak",
		})
	}

	sections = append(sections, domain.DocumentSection{
		Code:  "limitations",
		Title: "Batasan dan Pengecualian",
	})

	for i := range sections {
		sections[i].Number = i + 1
	}
	profile.Sections = sections

	return profile, nil
}

// NormalizeCustomDevelopmentModules trims all string fields in module groups and features,
// preserving original ordering and casing, and always returns a non-nil slice.
func NormalizeCustomDevelopmentModules(groups []domain.CustomDevelopmentModuleGroupInput) []domain.CustomDevelopmentModuleGroupInput {
	if len(groups) == 0 {
		return []domain.CustomDevelopmentModuleGroupInput{}
	}

	normalized := make([]domain.CustomDevelopmentModuleGroupInput, 0, len(groups))
	for _, g := range groups {
		features := make([]string, 0, len(g.Features))
		for _, feat := range g.Features {
			features = append(features, strings.TrimSpace(feat))
		}
		normalized = append(normalized, domain.CustomDevelopmentModuleGroupInput{
			Name:     strings.TrimSpace(g.Name),
			Features: features,
		})
	}
	return normalized
}

// NormalizeCustomDevelopmentTimeline trims all string fields in timeline rows,
// preserving original ordering and casing, and always returns a non-nil slice.
func NormalizeCustomDevelopmentTimeline(rows []domain.CustomDevelopmentTimelineRowInput) []domain.CustomDevelopmentTimelineRowInput {
	if len(rows) == 0 {
		return []domain.CustomDevelopmentTimelineRowInput{}
	}

	normalized := make([]domain.CustomDevelopmentTimelineRowInput, 0, len(rows))
	for _, r := range rows {
		normalized = append(normalized, domain.CustomDevelopmentTimelineRowInput{
			Description: strings.TrimSpace(r.Description),
			Timeline:    strings.TrimSpace(r.Timeline),
		})
	}
	return normalized
}
