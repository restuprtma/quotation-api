package service

import (
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// KioskQueueLimitationCatalog maps limitation codes to server-owned explicit catalog text.
var KioskQueueLimitationCatalog = map[domain.KioskQueueLimitationCode]string{
	domain.KioskLimitCivilElectrical:  "Pekerjaan fisik dan sipil/kelistrikan meliputi bobok tembok, penarikan daya/kabel, stop kontak, bracket, dan perbaikan area bukan tanggung jawab Venturo.",
	domain.KioskLimitClientHardware:   "Perangkat fisik, jaringan lokasi, dan pendukung disediakan klien; Venturo hanya memasang/mengonfigurasi perangkat lunak.",
	domain.KioskLimitExtraHardware:    "Pengadaan perangkat keras tambahan di luar rincian penawaran akan dikenakan biaya terpisah sesuai spesifikasi dan harga pasar saat pengadaan.",
	domain.KioskLimitHardwareWarranty: "Garansi perangkat keras mengikuti ketentuan dan masa garansi resmi dari distributor/manufaktur pabrikan terkait; Venturo tidak memberikan garansi penggantian unit di luar garansi pabrikan.",
	domain.KioskLimitOutOfPocket:      "Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.",
	domain.KioskLimitOvertime:         "Percepatan pekerjaan di luar jam kerja reguler (malam hari, Sabtu, Minggu, atau hari libur nasional) dikenakan biaya lembur tersendiri.",
	domain.KioskLimitThirdParty:       "Lisensi pihak ketiga, credit API AI (OpenAI/Anthropic), Payment Gateway, WhatsApp API / SMS Gateway, domain, dan server cloud di luar paket yang disepakati.",
	domain.KioskLimitScopeChange:      "Penambahan fitur baru, perubahan alur proses yang telah di-approve, atau penundaan approval dari klien yang mengakibatkan masa kontrak molor.",
}

// ResolveKioskQueueProfile resolves document profile, tax clause text, visibility flags,
// limitation copy, and document section plan for a kiosk queue quotation.
func ResolveKioskQueueProfile(input domain.KioskQueueSystemInput) (domain.ResolvedKioskQueueProfile, error) {
	normalized := input.Normalize()
	if err := normalized.Validate(); err != nil {
		return domain.ResolvedKioskQueueProfile{}, err
	}

	showHardware := normalized.HardwareProvision == domain.HardwareVenturoSupplied && len(normalized.HardwareItems) > 0
	showMaintenance := len(normalized.MaintenanceItems) > 0
	showRights := len(normalized.RightsStatements) > 0

	var taxClauseText string
	switch normalized.TaxTreatment {
	case domain.KioskTaxInclusivePPN11:
		taxClauseText = "Total investasi telah termasuk PPN 11% sesuai regulasi perpajakan yang berlaku di Indonesia."
	case domain.KioskTaxExclusivePPN:
		taxClauseText = "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia."
	}

	limitationTexts := make([]string, 0, len(normalized.LimitationCodes)+len(normalized.AdditionalLimitations))
	for _, code := range normalized.LimitationCodes {
		if text, ok := KioskQueueLimitationCatalog[code]; ok {
			limitationTexts = append(limitationTexts, text)
		}
	}
	limitationTexts = append(limitationTexts, normalized.AdditionalLimitations...)

	profile := domain.ResolvedKioskQueueProfile{
		Variant:           normalized.Variant,
		HardwareProvision: normalized.HardwareProvision,
		TaxTreatment:      normalized.TaxTreatment,
		TaxClauseText:     taxClauseText,
		ShowHardware:      showHardware,
		ShowMaintenance:   showMaintenance,
		ShowRights:        showRights,
		PaymentDueDays:    normalized.PaymentDueDays,
		LimitationTexts:   limitationTexts,
	}

	profile.Sections = BuildKioskQueueSectionPlan(profile)
	return profile, nil
}

// BuildKioskQueueSectionPlan constructs the ordered section plan with contiguous 1-based numbering.
func BuildKioskQueueSectionPlan(profile domain.ResolvedKioskQueueProfile) []domain.DocumentSection {
	var sections []domain.DocumentSection
	switch profile.Variant {
	case domain.KioskQueueGovernmentMPP:
		sections = []domain.DocumentSection{
			{Code: "scope", Title: "Lingkup Pekerjaan"},
			{Code: "experts", Title: "Kebutuhan Tenaga Ahli"},
			{Code: "costs", Title: "Rincian Biaya"},
			{Code: "payment", Title: "Tahapan Pembayaran"},
			{Code: "limitations", Title: "Batasan dan Pengecualian"},
		}
	case domain.KioskQueuePrivateFoodCourt:
		sections = append(sections, domain.DocumentSection{
			Code:  "development",
			Title: "Biaya Development",
		})
		if profile.ShowMaintenance {
			sections = append(sections, domain.DocumentSection{
				Code:  "maintenance",
				Title: "Biaya Maintenance",
			})
		}
		sections = append(sections,
			domain.DocumentSection{Code: "timeline", Title: "Lingkup Pekerjaan & Timeline"},
			domain.DocumentSection{Code: "payment", Title: "Pembayaran"},
			domain.DocumentSection{Code: "limitations", Title: "Batasan dan Pengecualian"},
		)
	default:
		return nil
	}

	for i := range sections {
		sections[i].Number = i + 1
	}
	return sections
}
