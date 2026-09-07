package service

import (
	"fmt"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const SubscriptionLegalCatalogVersion = domain.SubscriptionLegalCatalogVersion

// ResolveSubscriptionSaaSProfile resolves business rules, legal catalog clauses,
// tax treatment copy, and conditional sections for a SaaS quotation.
func ResolveSubscriptionSaaSProfile(input domain.SubscriptionSaaSInput) (domain.ResolvedSubscriptionSaaSProfile, error) {
	if err := input.Validate(); err != nil {
		return domain.ResolvedSubscriptionSaaSProfile{}, err
	}

	profile := domain.ResolvedSubscriptionSaaSProfile{
		SectionLayout:       input.SectionLayout,
		BillingCycle:        input.BillingCycle,
		TaxTreatment:        input.TaxTreatment,
		LegalCatalogVersion: SubscriptionLegalCatalogVersion,
		HasFeatures:         len(input.FeatureGroups) > 0,
	}

	switch input.TaxTreatment {
	case domain.SubscriptionTaxExclusive:
		profile.TaxClauseText = "Harga belum termasuk PPN."
	case domain.SubscriptionTaxInclusive:
		profile.TaxClauseText = "Harga sudah termasuk PPN."
	case domain.SubscriptionTaxNotStated:
		profile.TaxClauseText = ""
	}

	if input.RightsClauseCode == domain.RightsClauseVenturoSoftwareOwnershipV1 {
		profile.RightsClauseCode = input.RightsClauseCode
		profile.RightsClauseText = "Aplikasi dan source code merupakan milik Venturo."
	}

	if input.AutoRenewal {
		profile.AutoRenewalText = fmt.Sprintf("Masa berlangganan awal adalah %d bulan dan diperpanjang otomatis untuk periode yang sama kecuali disepakati lain secara tertulis.", input.InitialTermMonths)
	}

	if input.IsPartnership && input.IncomeSharing != nil {
		sharing := input.IncomeSharing
		text := fmt.Sprintf(
			"Pendapatan dari %s dibagi sebesar %s%% untuk Venturo dan %s%% untuk Mitra. Pembayaran diselesaikan paling lambat %d hari kerja.",
			sharing.EligibleRevenueDescription,
			formatBPSPercent(sharing.VenturoShareBPS),
			formatBPSPercent(sharing.PartnerShareBPS),
			sharing.SettlementBusinessDays,
		)
		if sharing.ProhibitUnilateralDiscount {
			text += " Diskon tidak dapat diberikan secara sepihak."
		}
		profile.IncomeSharingText = text
	}

	return profile, nil
}

// BuildSubscriptionSectionPlan constructs the ordered list of document sections
// based on resolved profile layout and conditional clauses.
func BuildSubscriptionSectionPlan(profile domain.ResolvedSubscriptionSaaSProfile) []domain.DocumentSection {
	var sections []domain.DocumentSection

	switch profile.SectionLayout {
	case domain.SubscriptionLayoutSubscriptionDevelopment:
		sections = append(sections, domain.DocumentSection{
			Code:  string(domain.SubscriptionLayoutSubscriptionDevelopment),
			Title: "Biaya Subscription dan Development Aplikasi",
		})
	case domain.SubscriptionLayoutDevelopmentMaintenance:
		sections = append(sections, domain.DocumentSection{
			Code:  string(domain.SubscriptionLayoutDevelopmentMaintenance),
			Title: "Biaya Development & Maintenance",
		})
	default:
		sections = append(sections, domain.DocumentSection{
			Code:  "commercial",
			Title: "Biaya Subscription",
		})
	}

	if profile.SectionLayout == domain.SubscriptionLayoutDevelopmentMaintenance || profile.HasFeatures {
		sections = append(sections, domain.DocumentSection{
			Code:  "features",
			Title: "List Features",
		})
	}

	sections = append(sections, domain.DocumentSection{
		Code:  "payment",
		Title: "Pembayaran",
	})

	if profile.HasRights() {
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
	return sections
}

func formatBPSPercent(bps int32) string {
	if bps%100 == 0 {
		return fmt.Sprintf("%d", bps/100)
	}
	if bps%10 == 0 {
		return fmt.Sprintf("%.1f", float64(bps)/100)
	}
	return fmt.Sprintf("%.2f", float64(bps)/100)
}
