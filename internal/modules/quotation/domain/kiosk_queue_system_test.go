package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validGovernmentMPPInput() domain.KioskQueueSystemInput {
	return domain.KioskQueueSystemInput{
		Variant:           domain.KioskQueueGovernmentMPP,
		DocumentNumber:    "LANDA/MPP-Antrian/0726/003",
		SystemName:        "Sistem Antrian Terpadu MPP",
		QuotationDate:     time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC),
		HardwareProvision: domain.HardwareVenturoSupplied,
		TaxTreatment:      domain.KioskTaxInclusivePPN11,
		GovernmentScopePhases: []domain.KioskQueueScopePhaseInput{
			{
				Title: "Fase 1: Persiapan & Desain",
				Deliverables: []string{
					"Dokumen Analisis Kebutuhan",
					"UI/UX Design Mockup",
				},
			},
		},
		ExpertRequirements: []domain.KioskQueueExpertInput{
			{
				Role:                 "Project Manager",
				Headcount:            1,
				DurationLabel:        "3 Bulan",
				MinimumQualification: "S1 Sistem Informasi, pengalaman 5 tahun",
			},
		},
		ServiceGroups: []domain.KioskQueueServiceGroupInput{
			{
				Title: "Jasa Pengembangan Perangkat Lunak",
				Items: []domain.KioskQueueServiceItemInput{
					{
						Name:           "Senior Backend Engineer",
						MonthlyRate:    20_000_000,
						Headcount:      1,
						PeriodQuarters: 4,
					},
				},
			},
		},
		ServiceDiscountAmount:  0,
		ShowServiceDiscountRow: false,
		HardwareItems: []domain.KioskQueueHardwareItemInput{
			{
				Name:      "Kiosk Touchscreen 21.5 inch",
				UnitPrice: 25_000_000,
				Quantity:  1,
			},
		},
		GovernmentPaymentSchedule: []domain.KioskQueueGovernmentPaymentInput{
			{
				Stage:        "Uang Muka (DP 50%)",
				BillingBasis: "Setelah penandatanganan SPMK",
				Nominal:      25_000_000,
			},
			{
				Stage:        "Pelunasan (50%)",
				BillingBasis: "Setelah BAST pekerjaan selesai",
				Nominal:      25_000_000,
			},
		},
		PaymentDueDays: 14,
		LimitationCodes: []domain.KioskQueueLimitationCode{
			domain.KioskLimitCivilElectrical,
			domain.KioskLimitOutOfPocket,
		},
		AdditionalLimitations: []string{
			"Pekerjaan sipil dan perkabelan tidak termasuk penawaran ini.",
		},
		RightsStatements: []string{
			"Hak cipta dan kode sumber menjadi milik pemberi kerja setelah pelunasan.",
		},
	}
}

func validPrivateFoodCourtInput() domain.KioskQueueSystemInput {
	return domain.KioskQueueSystemInput{
		Variant:           domain.KioskQueuePrivateFoodCourt,
		DocumentNumber:    "VEN/Kiosk-FoodCourt/0826/007",
		SystemName:        "Self-Ordering Kiosk Food Court",
		QuotationDate:     time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		HardwareProvision: domain.HardwareClientSupplied,
		TaxTreatment:      domain.KioskTaxExclusivePPN,
		ServiceGroups: []domain.KioskQueueServiceGroupInput{
			{
				Title: "Pengembangan Sistem Self-Ordering Kiosk",
				Items: []domain.KioskQueueServiceItemInput{
					{
						Name:           "Frontend Developer",
						MonthlyRate:    18_000_000,
						Headcount:      1,
						PeriodQuarters: 4,
					},
				},
			},
		},
		ServiceDiscountAmount:  0,
		ShowServiceDiscountRow: false,
		MaintenanceItems: []domain.KioskQueueServiceItemInput{
			{
				Name:           "SLA Pemeliharaan Kiosk",
				MonthlyRate:    5_000_000,
				Headcount:      1,
				PeriodQuarters: 12,
			},
		},
		PrivateTimelineRows: []domain.KioskQueueTimelineRowInput{
			{
				Description: "Fase Analisis dan Desain UI/UX",
				Timeline:    "Bulan 1",
			},
			{
				Description: "Fase Development & Testing",
				Timeline:    "Bulan 2-3",
			},
		},
		PrivatePaymentSchedule: []domain.KioskQueuePrivatePaymentInput{
			{
				WorkPeriod:       "Kick Off",
				Terms:            "DP 50%",
				InvoiceDateLabel: "01/08/2026",
				DueDateLabel:     "15/08/2026",
				Nominal:          9_000_000,
			},
			{
				WorkPeriod:       "Go Live",
				Terms:            "Pelunasan 50%",
				InvoiceDateLabel: "01/10/2026",
				DueDateLabel:     "15/10/2026",
				Nominal:          9_000_000,
			},
		},
		PaymentDueDays: 0,
		LimitationCodes: []domain.KioskQueueLimitationCode{
			domain.KioskLimitClientHardware,
			domain.KioskLimitThirdParty,
		},
		AdditionalLimitations: []string{
			"Perangkat keras disediakan sepenuhnya oleh pihak klien.",
		},
		RightsStatements: []string{
			"Hak penggunaan sistem berlaku sesuai lisensi yang disepakati.",
		},
	}
}

func TestKioskQueueSystemInputValidate(t *testing.T) {
	validGovernment := validGovernmentMPPInput()
	validPrivate := validPrivateFoodCourtInput()
	tests := []struct {
		name    string
		mutate  func(*domain.KioskQueueSystemInput)
		base    domain.KioskQueueSystemInput
		wantErr string
	}{
		{name: "government valid", base: validGovernment},
		{name: "private valid", base: validPrivate},
		{name: "unknown variant", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) { in.Variant = "other" }, wantErr: "variant"},
		{name: "invalid government number", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) { in.DocumentNumber = "QUO-2026-001" }, wantErr: "document_number"},
		{name: "government tax changed", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) { in.TaxTreatment = domain.KioskTaxExclusivePPN }, wantErr: "tax_treatment"},
		{name: "private venturo hardware", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) { in.HardwareProvision = domain.HardwareVenturoSupplied }, wantErr: "hardware_provision"},
		{name: "client hardware has items", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) { in.HardwareProvision = domain.HardwareClientSupplied }, wantErr: "hardware_items"},
		{name: "government missing civil exclusion", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.LimitationCodes = []domain.KioskQueueLimitationCode{domain.KioskLimitOutOfPocket}
		}, wantErr: "civil_electrical"},
		{name: "private carries government scope", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.GovernmentScopePhases = validGovernment.GovernmentScopePhases
		}, wantErr: "government_scope_phases"},
		{name: "positive hidden discount", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceDiscountAmount = 1
			in.ShowServiceDiscountRow = false
		}, wantErr: "show_service_discount_row"},

		// Additional matrix & cross-variant cases
		{name: "government with client hardware missing client_hardware limitation", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareProvision = domain.HardwareClientSupplied
			in.HardwareItems = nil
			in.LimitationCodes = []domain.KioskQueueLimitationCode{domain.KioskLimitCivilElectrical}
		}, wantErr: "client_hardware"},
		{name: "government with client hardware valid", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareProvision = domain.HardwareClientSupplied
			in.HardwareItems = nil
			in.LimitationCodes = []domain.KioskQueueLimitationCode{domain.KioskLimitCivilElectrical, domain.KioskLimitClientHardware}
		}},
		{name: "government carries maintenance items", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.MaintenanceItems = validPrivate.MaintenanceItems
		}, wantErr: "maintenance_items"},
		{name: "government carries private timeline rows", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PrivateTimelineRows = validPrivate.PrivateTimelineRows
		}, wantErr: "private_timeline_rows"},
		{name: "government carries private payment schedule", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PrivatePaymentSchedule = validPrivate.PrivatePaymentSchedule
		}, wantErr: "private_payment_schedule"},
		{name: "government missing expert requirements", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ExpertRequirements = nil
		}, wantErr: "expert_requirements"},
		{name: "government missing government scope phases", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.GovernmentScopePhases = nil
		}, wantErr: "government_scope_phases"},
		{name: "government payment due days zero", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PaymentDueDays = 0
		}, wantErr: "payment_due_days"},
		{name: "government payment due days exceeds 365", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PaymentDueDays = 366
		}, wantErr: "payment_due_days"},
		{name: "private carries expert requirements", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ExpertRequirements = validGovernment.ExpertRequirements
		}, wantErr: "expert_requirements"},
		{name: "private carries hardware items", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareItems = validGovernment.HardwareItems
		}, wantErr: "hardware_items"},
		{name: "private carries government payment schedule", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.GovernmentPaymentSchedule = validGovernment.GovernmentPaymentSchedule
		}, wantErr: "government_payment_schedule"},
		{name: "private non-zero payment due days", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PaymentDueDays = 14
		}, wantErr: "payment_due_days"},
		{name: "private missing client hardware limitation", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.LimitationCodes = []domain.KioskQueueLimitationCode{domain.KioskLimitThirdParty}
		}, wantErr: "client_hardware"},
		{name: "private empty timeline rows", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PrivateTimelineRows = nil
		}, wantErr: "private_timeline_rows"},
		{name: "private empty payment schedule", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PrivatePaymentSchedule = nil
		}, wantErr: "private_payment_schedule"},

		// Enums and limitations
		{name: "unknown hardware provision", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareProvision = "unknown_provision"
		}, wantErr: "hardware_provision"},
		{name: "unknown tax treatment", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.TaxTreatment = "tax_free"
		}, wantErr: "tax_treatment"},
		{name: "unknown limitation code", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.LimitationCodes = append(in.LimitationCodes, "unsupported_code")
		}, wantErr: "unsupported limitation"},
		{name: "duplicate limitation code", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.LimitationCodes = append(in.LimitationCodes, domain.KioskLimitCivilElectrical)
		}, wantErr: "duplicate limitation"},
		{name: "duplicate additional limitation", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.AdditionalLimitations = []string{"Batasan A", "  batasan a  "}
		}, wantErr: "duplicate additional limitation"},

		// HTML-like checks (< and >)
		{name: "HTML-like in system_name", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.SystemName = "Sistem <script>alert(1)</script>"
		}, wantErr: "system_name"},
		{name: "HTML-like in service item name", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].Name = "Dev <b>Senior</b>"
		}, wantErr: "service_groups"},
		{name: "HTML-like in phase deliverable", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.GovernmentScopePhases[0].Deliverables[0] = "<img src=x onerror=alert(1)>"
		}, wantErr: "government_scope_phases"},
		{name: "HTML-like in expert role", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ExpertRequirements[0].Role = "<PM>"
		}, wantErr: "expert_requirements"},
		{name: "HTML-like in timeline description", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PrivateTimelineRows[0].Description = "Phase <one>"
		}, wantErr: "private_timeline_rows"},
		{name: "HTML-like in additional limitations", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.AdditionalLimitations = []string{"<script>"}
		}, wantErr: "additional_limitations"},
		{name: "HTML-like in rights statements", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.RightsStatements = []string{"<b>Hak Cipta</b>"}
		}, wantErr: "rights_statements"},

		// Unsafe integers and limits
		{name: "unsafe integer monthly rate", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].MonthlyRate = domain.KioskQueueMaxMoney + 1
		}, wantErr: "exceeds maximum safe amount"},
		{name: "hardware line multiplication overflow near max money", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareItems[0].UnitPrice = domain.KioskQueueMaxMoney
			in.HardwareItems[0].Quantity = 10_000
		}, wantErr: "exceeds maximum safe amount"},
		{name: "service line multiplication overflow near max money", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].MonthlyRate = domain.KioskQueueMaxMoney
			in.ServiceGroups[0].Items[0].Headcount = 100
			in.ServiceGroups[0].Items[0].PeriodQuarters = 240
		}, wantErr: "exceeds maximum safe amount"},
		{name: "maintenance line multiplication overflow near max money", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.MaintenanceItems[0].MonthlyRate = domain.KioskQueueMaxMoney
			in.MaintenanceItems[0].Headcount = 100
			in.MaintenanceItems[0].PeriodQuarters = 240
		}, wantErr: "exceeds maximum safe amount"},
		{name: "service subtotal accumulation exceeds max safe amount", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items = []domain.KioskQueueServiceItemInput{
				{Name: "Item 1", MonthlyRate: 5_000_000_000_000_000, Headcount: 1, PeriodQuarters: 4},
				{Name: "Item 2", MonthlyRate: 5_000_000_000_000_000, Headcount: 1, PeriodQuarters: 4},
			}
		}, wantErr: "exceeds maximum safe amount"},
		{name: "hardware subtotal accumulation exceeds max safe amount", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareItems = []domain.KioskQueueHardwareItemInput{
				{Name: "HW 1", UnitPrice: 5_000_000_000_000_000, Quantity: 1},
				{Name: "HW 2", UnitPrice: 5_000_000_000_000_000, Quantity: 1},
			}
		}, wantErr: "exceeds maximum safe amount"},
		{name: "unsafe integer discount amount", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceDiscountAmount = domain.KioskQueueMaxMoney + 1
			in.ShowServiceDiscountRow = true
		}, wantErr: "exceeds maximum safe amount"},
		{name: "unsafe integer hardware unit price", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareItems[0].UnitPrice = domain.KioskQueueMaxMoney + 1
		}, wantErr: "exceeds maximum safe amount"},
		{name: "negative discount amount", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceDiscountAmount = -1000
		}, wantErr: "service_discount_amount"},
		{name: "discount exceeds service subtotal", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceDiscountAmount = 90_000_000
			in.ShowServiceDiscountRow = true
		}, wantErr: "service_discount_amount"},
		{name: "counts above limit service groups", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			groups := make([]domain.KioskQueueServiceGroupInput, 31)
			for i := range groups {
				groups[i] = in.ServiceGroups[0]
			}
			in.ServiceGroups = groups
		}, wantErr: "service_groups"},
		{name: "counts above limit items per group", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			items := make([]domain.KioskQueueServiceItemInput, 101)
			for i := range items {
				items[i] = in.ServiceGroups[0].Items[0]
			}
			in.ServiceGroups[0].Items = items
		}, wantErr: "items"},
		{name: "headcount zero", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].Headcount = 0
		}, wantErr: "headcount"},
		{name: "headcount above 100", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].Headcount = 101
		}, wantErr: "headcount"},
		{name: "period quarters zero", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].PeriodQuarters = 0
		}, wantErr: "period_quarters"},
		{name: "period quarters above 240", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].PeriodQuarters = 241
		}, wantErr: "period_quarters"},
		{name: "hardware quantity zero", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareItems[0].Quantity = 0
		}, wantErr: "quantity"},
		{name: "hardware quantity above 10000", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.HardwareItems[0].Quantity = 10001
		}, wantErr: "quantity"},

		// Blank strings (normalized to empty)
		{name: "blank system name with spaces", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.SystemName = "   "
		}, wantErr: "system_name"},
		{name: "blank nested group title", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Title = "   "
		}, wantErr: "title"},
		{name: "blank nested item name", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].Name = "   "
		}, wantErr: "name"},
		{name: "blank nested deliverable", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.GovernmentScopePhases[0].Deliverables[0] = "   "
		}, wantErr: "deliverables"},
		{name: "blank nested expert role", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ExpertRequirements[0].Role = "   "
		}, wantErr: "role"},
		{name: "blank nested timeline description", base: validPrivate, mutate: func(in *domain.KioskQueueSystemInput) {
			in.PrivateTimelineRows[0].Description = "   "
		}, wantErr: "description"},
		{name: "non-integer quarter calculation remainder", base: validGovernment, mutate: func(in *domain.KioskQueueSystemInput) {
			in.ServiceGroups[0].Items[0].MonthlyRate = 1
			in.ServiceGroups[0].Items[0].Headcount = 1
			in.ServiceGroups[0].Items[0].PeriodQuarters = 1 // (1*1*1) % 4 != 0
		}, wantErr: "whole integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.base.Normalize()
			if tt.mutate != nil {
				tt.mutate(&input)
			}
			err := input.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestKioskQueueSystemInput_Normalize(t *testing.T) {
	orig := validGovernmentMPPInput()
	orig.SystemName = "  Sistem Antrian Terpadu MPP  "
	orig.DocumentNumber = "  LANDA/MPP-Antrian/0726/003  "
	orig.ServiceGroups[0].Title = "  Jasa Pengembangan  "
	orig.ServiceGroups[0].Items[0].Name = "  Senior Backend Engineer  "
	orig.GovernmentScopePhases[0].Title = "  Fase 1  "
	orig.GovernmentScopePhases[0].Deliverables[0] = "  Deliverable 1  "
	orig.ExpertRequirements[0].Role = "  PM  "
	orig.ExpertRequirements[0].DurationLabel = "  3 Bulan  "
	orig.ExpertRequirements[0].MinimumQualification = "  Kualifikasi  "
	orig.HardwareItems[0].Name = "  Hardware 1  "
	orig.GovernmentPaymentSchedule[0].Stage = "  Tahap 1  "
	orig.GovernmentPaymentSchedule[0].BillingBasis = "  SPMK  "
	orig.AdditionalLimitations = []string{"  Batasan 1  "}
	orig.RightsStatements = []string{"  Hak 1  "}

	norm := orig.Normalize()

	assert.Equal(t, "Sistem Antrian Terpadu MPP", norm.SystemName)
	assert.Equal(t, "LANDA/MPP-Antrian/0726/003", norm.DocumentNumber)
	assert.Equal(t, "Jasa Pengembangan", norm.ServiceGroups[0].Title)
	assert.Equal(t, "Senior Backend Engineer", norm.ServiceGroups[0].Items[0].Name)
	assert.Equal(t, "Fase 1", norm.GovernmentScopePhases[0].Title)
	assert.Equal(t, "Deliverable 1", norm.GovernmentScopePhases[0].Deliverables[0])
	assert.Equal(t, "PM", norm.ExpertRequirements[0].Role)
	assert.Equal(t, "3 Bulan", norm.ExpertRequirements[0].DurationLabel)
	assert.Equal(t, "Kualifikasi", norm.ExpertRequirements[0].MinimumQualification)
	assert.Equal(t, "Hardware 1", norm.HardwareItems[0].Name)
	assert.Equal(t, "Tahap 1", norm.GovernmentPaymentSchedule[0].Stage)
	assert.Equal(t, "SPMK", norm.GovernmentPaymentSchedule[0].BillingBasis)
	assert.Equal(t, "Batasan 1", norm.AdditionalLimitations[0])
	assert.Equal(t, "Hak 1", norm.RightsStatements[0])

	// Verify deep copy: mutating norm does not change orig
	norm.ServiceGroups[0].Items[0].Name = "Modified"
	assert.Equal(t, "  Senior Backend Engineer  ", orig.ServiceGroups[0].Items[0].Name)

	norm.GovernmentScopePhases[0].Deliverables[0] = "Modified"
	assert.Equal(t, "  Deliverable 1  ", orig.GovernmentScopePhases[0].Deliverables[0])

	// Verify normalization does not silently drop invalid rows
	origWithEmpty := validGovernmentMPPInput()
	origWithEmpty.ServiceGroups[0].Items = append(origWithEmpty.ServiceGroups[0].Items, domain.KioskQueueServiceItemInput{
		Name: "   ",
	})
	normWithEmpty := origWithEmpty.Normalize()
	assert.Len(t, normWithEmpty.ServiceGroups[0].Items, 2)
	assert.Equal(t, "", normWithEmpty.ServiceGroups[0].Items[1].Name)
}

func TestKioskQueueSystemInput_UnmarshalJSON(t *testing.T) {
	t.Run("valid unmarshal with YYYY-MM-DD", func(t *testing.T) {
		raw := `{
			"variant": "government_mpp",
			"document_number": "LANDA/MPP-Antrian/0726/003",
			"system_name": "Sistem Antrian Terpadu MPP",
			"quotation_date": "2026-07-30",
			"hardware_provision": "venturo_supplied",
			"tax_treatment": "inclusive_ppn_11",
			"government_scope_phases": [
				{
					"title": "Fase 1",
					"deliverables": ["Dokumen"]
				}
			],
			"expert_requirements": [
				{
					"role": "PM",
					"headcount": 1,
					"duration_label": "3 Bulan",
					"minimum_qualification": "S1"
				}
			],
			"service_groups": [
				{
					"title": "Jasa",
					"items": [
						{
							"name": "Backend",
							"monthly_rate": 20000000,
							"headcount": 1,
							"period_quarters": 4
						}
					]
				}
			],
			"service_discount_amount": 0,
			"show_service_discount_row": false,
			"hardware_items": [
				{
					"name": "Kiosk",
					"unit_price": 25000000,
					"quantity": 1
				}
			],
			"government_payment_schedule": [
				{
					"stage": "DP",
					"billing_basis": "SPMK",
					"nominal": 25000000
				}
			],
			"payment_due_days": 14,
			"limitation_codes": ["civil_electrical"]
		}`
		var in domain.KioskQueueSystemInput
		err := json.Unmarshal([]byte(raw), &in)
		require.NoError(t, err)
		assert.Equal(t, domain.KioskQueueGovernmentMPP, in.Variant)
		assert.Equal(t, "LANDA/MPP-Antrian/0726/003", in.DocumentNumber)
		assert.Equal(t, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC), in.QuotationDate.UTC())
		require.NoError(t, in.Validate())
	})

	t.Run("rejects date not in YYYY-MM-DD or RFC3339", func(t *testing.T) {
		raw := `{
			"variant": "government_mpp",
			"document_number": "LANDA/MPP-Antrian/0726/003",
			"system_name": "Sistem Antrian Terpadu MPP",
			"quotation_date": "30-07-2026",
			"hardware_provision": "venturo_supplied",
			"tax_treatment": "inclusive_ppn_11"
		}`
		var in domain.KioskQueueSystemInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date")
	})

	t.Run("rejects missing or null quotation_date", func(t *testing.T) {
		raw := `{
			"variant": "government_mpp",
			"document_number": "LANDA/MPP-Antrian/0726/003",
			"system_name": "Sistem Antrian Terpadu MPP",
			"quotation_date": null,
			"hardware_provision": "venturo_supplied",
			"tax_treatment": "inclusive_ppn_11"
		}`
		var in domain.KioskQueueSystemInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date")
	})

	t.Run("rejects unknown top-level field", func(t *testing.T) {
		raw := `{
			"variant": "government_mpp",
			"document_number": "LANDA/MPP-Antrian/0726/003",
			"system_name": "Sistem Antrian Terpadu MPP",
			"quotation_date": "2026-07-30",
			"hardware_provision": "venturo_supplied",
			"tax_treatment": "inclusive_ppn_11",
			"unknown_property": "illegal"
		}`
		var in domain.KioskQueueSystemInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects unknown nested field in service_groups", func(t *testing.T) {
		raw := `{
			"variant": "government_mpp",
			"document_number": "LANDA/MPP-Antrian/0726/003",
			"system_name": "Sistem Antrian Terpadu MPP",
			"quotation_date": "2026-07-30",
			"hardware_provision": "venturo_supplied",
			"tax_treatment": "inclusive_ppn_11",
			"service_groups": [
				{
					"title": "Jasa",
					"extra_illegal_field": 123,
					"items": [
						{
							"name": "Backend",
							"monthly_rate": 20000000,
							"headcount": 1,
							"period_quarters": 4
						}
					]
				}
			]
		}`
		var in domain.KioskQueueSystemInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects unknown nested field in hardware_items", func(t *testing.T) {
		raw := `{
			"variant": "government_mpp",
			"document_number": "LANDA/MPP-Antrian/0726/003",
			"system_name": "Sistem Antrian Terpadu MPP",
			"quotation_date": "2026-07-30",
			"hardware_provision": "venturo_supplied",
			"tax_treatment": "inclusive_ppn_11",
			"hardware_items": [
				{
					"name": "Kiosk",
					"unit_price": 25000000,
					"quantity": 1,
					"unknown_hardware_field": true
				}
			]
		}`
		var in domain.KioskQueueSystemInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}
