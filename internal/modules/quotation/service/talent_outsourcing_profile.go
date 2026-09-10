package service

import (
	"fmt"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var TalentOutsourcingLimitationCatalog = map[domain.TalentLimitationCode]string{domain.TalentLimitOutOfPocket: "Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.", domain.TalentLimitOvertime: "Pekerjaan di luar jam kerja reguler dikenakan biaya lembur tersendiri.", domain.TalentLimitThirdParty: "Lisensi pihak ketiga, domain, server cloud, dan layanan eksternal di luar paket yang disepakati.", domain.TalentLimitScopeChange: "Penambahan ruang lingkup atau perubahan yang telah disetujui memerlukan kesepakatan terpisah."}

func ResolveTalentOutsourcingProfile(raw domain.TalentOutsourcingInput) (domain.ResolvedTalentOutsourcingProfile, error) {
	in := domain.NormalizeTalentOutsourcingInput(raw)
	if err := in.Validate(); err != nil {
		return domain.ResolvedTalentOutsourcingProfile{}, err
	}
	p := domain.ResolvedTalentOutsourcingProfile{EngagementMode: in.EngagementMode, TaxTreatment: in.TaxTreatment, ShowPreparation: len(in.PreparationItems) > 0, ShowPreparationDiscount: in.ShowPreparationDiscountRow, ShowWarranty: len(in.WarrantyStatements) > 0, ShowRights: len(in.RightsStatements) > 0, PaymentDueDays: in.PaymentDueDays}
	for _, g := range in.CostGroups {
		if g.Category == domain.TalentCategoryNonPersonnel {
			p.ShowNonPersonnel = true
		}
	}
	switch in.TaxTreatment {
	case domain.TalentTaxBPJSDPP12:
		p.TaxClauseText = "Perhitungan pajak menggunakan DPP PPN 11/12 dari harga sebelum PPN dan PPN 12% dari DPP PPN."
	case domain.TalentTaxExclusive:
		p.TaxClauseText = "Seluruh rincian biaya belum termasuk PPN sesuai ketentuan perpajakan yang berlaku di Indonesia."
	case domain.TalentTaxNotStated:
	default:
		return p, fmt.Errorf("unsupported tax treatment %q", in.TaxTreatment)
	}
	for _, c := range in.LimitationCodes {
		p.LimitationTexts = append(p.LimitationTexts, TalentOutsourcingLimitationCatalog[c])
	}
	p.LimitationTexts = append(p.LimitationTexts, in.AdditionalLimitations...)
	p.Sections = []domain.DocumentSection{{Number: 1, Code: "development_cost", Title: "Biaya Development"}, {Number: 2, Code: "scope_timeline", Title: "Lingkup Pekerjaan & Timeline"}, {Number: 3, Code: "payment", Title: "Pembayaran"}, {Number: 4, Code: "limitations", Title: "Batasan dan Pengecualian"}}
	return p, nil
}
func NormalizeTalentScopeBlocks(in domain.TalentOutsourcingInput) []domain.TalentScopeBlockInput {
	return domain.NormalizeTalentOutsourcingInput(in).ScopeBlocks
}
func NormalizeTalentTimeline(in domain.TalentOutsourcingInput) []domain.TalentTimelineSnapshotItem {
	n := domain.NormalizeTalentOutsourcingInput(in)
	out := make([]domain.TalentTimelineSnapshotItem, len(n.ScopeBlocks))
	for i, b := range n.ScopeBlocks {
		out[i] = domain.TalentTimelineSnapshotItem{Title: b.Title, Timeline: b.Timeline}
	}
	return out
}
