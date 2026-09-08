package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ResolveWhiteLabelProfile validates input, sets tax clause copy,
// calculates conditional visibility flags, and builds the ordered section plan.
func ResolveWhiteLabelProfile(input domain.WhiteLabelInput) (domain.ResolvedWhiteLabelProfile, error) {
	if err := input.Validate(); err != nil {
		return domain.ResolvedWhiteLabelProfile{}, err
	}

	profile := domain.ResolvedWhiteLabelProfile{
		TaxTreatment:           input.TaxTreatment,
		ShowMaintenanceHosting: len(input.MaintenanceItems) > 0 || len(input.HostingItems) > 0,
		ShowScopeTimeline:      len(input.ModuleGroups) > 0 || len(input.TimelineRows) > 0,
		ShowRights:             len(input.RightsStatements) > 0,
	}

	switch input.TaxTreatment {
	case domain.WhiteLabelTaxNotStated:
		profile.TaxClauseText = ""
	case domain.WhiteLabelTaxExclusive:
		profile.TaxClauseText = "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia."
	case domain.WhiteLabelTaxInclusive:
		profile.TaxClauseText = "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini."
	default:
		return domain.ResolvedWhiteLabelProfile{}, fmt.Errorf("tax_treatment: unsupported value %q", input.TaxTreatment)
	}

	var sections []domain.DocumentSection
	sections = append(sections, domain.DocumentSection{
		Code:  "white_label_cost",
		Title: "Biaya White Label",
	})

	if profile.ShowMaintenanceHosting {
		sections = append(sections, domain.DocumentSection{
			Code:  "maintenance_hosting",
			Title: "Biaya Maintenance & Hosting",
		})
	}

	if profile.ShowScopeTimeline {
		sections = append(sections, domain.DocumentSection{
			Code:  "scope_timeline",
			Title: "Lingkup Pekerjaan & Timeline",
		})
	}

	sections = append(sections, domain.DocumentSection{
		Code:  "payment",
		Title: "Pembayaran",
	})

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
