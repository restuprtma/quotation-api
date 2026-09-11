package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// ResolveCloudInfrastructureProfile resolves tax copy, conditional visibility,
// and the compact, ordered document section plan for a cloud quotation.
func ResolveCloudInfrastructureProfile(in domain.CloudInfrastructureInput) (domain.ResolvedCloudInfrastructureProfile, error) {
	in = in.Normalize()
	if err := in.Validate(); err != nil {
		return domain.ResolvedCloudInfrastructureProfile{}, err
	}

	profile := domain.ResolvedCloudInfrastructureProfile{
		TaxTreatment: in.TaxTreatment,
		ShowSetup:    len(in.SetupItems) > 0,
		ShowScope:    len(in.ScopeRows) > 0,
	}
	switch in.TaxTreatment {
	case domain.CloudTaxExclusive:
		profile.TaxClauseText = "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia."
	case domain.CloudTaxInclusive:
		profile.TaxClauseText = "Total investasi telah termasuk PPN sesuai nilai yang disepakati dalam quotation ini."
	case domain.CloudTaxNotStated:
		profile.TaxClauseText = ""
	default:
		return domain.ResolvedCloudInfrastructureProfile{}, fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}

	sections := []domain.DocumentSection{{Number: 1, Code: "cloud_cost", Title: "Biaya Venturo Cloud"}}
	if profile.ShowScope {
		sections = append(sections, domain.DocumentSection{Number: len(sections) + 1, Code: "scope_timeline", Title: "Lingkup Pekerjaan & Timeline"})
	}
	sections = append(sections, domain.DocumentSection{Number: len(sections) + 1, Code: "payment", Title: "Pembayaran"})
	sections = append(sections, domain.DocumentSection{Number: len(sections) + 1, Code: "limitations", Title: "Batasan dan Pengecualian"})
	profile.Sections = sections
	return profile, nil
}
