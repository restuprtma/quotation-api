package dto

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestCredentialResponse_JSONSerialization(t *testing.T) {
	resp := CredentialResponse{
		ID:        uuid.New(),
		CompanyID: uuid.New(),
		Provider:  "openai",
		KeyHint:   "sk-...abcd",
		IsActive:  true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var raw map[string]interface{}
	err = json.Unmarshal(b, &raw)
	require.NoError(t, err)

	_, hasEncryptedAPIKey := raw["encrypted_api_key"]
	assert.False(t, hasEncryptedAPIKey, "JSON should not contain encrypted_api_key")

	_, hasAPIKey := raw["api_key"]
	assert.False(t, hasAPIKey, "JSON should not contain api_key")

	assert.Equal(t, "openai", raw["provider"])
	assert.Equal(t, "sk-...abcd", raw["key_hint"])
}

func TestGenerateQuotationRequest_DedicatedSquadJSONSerialization(t *testing.T) {
	anchor := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	discount := int32(2500)
	req := GenerateQuotationRequest{
		DedicatedSquad: &domain.DedicatedSquadInput{
			Model:             domain.ModelSOWLarge,
			Requirement:       "Build quotation engine",
			DevelopmentSquad:  []domain.SquadLineInput{{Role: "backend_programmer", Seniority: "senior", HeadcountQuarters: 4, PeriodQuarters: 8}},
			PaymentCount:      2,
			PaymentAnchorDate: &anchor,
			Timeline:          domain.TimelineInput{Mode: domain.TimelineRelative},
			QuotationProfile: &domain.QuotationProfileInput{
				PreparationMode:        domain.PreparationProfileHiring,
				PreparationDiscountBPS: &discount,
				ScopeTimelineMode:      domain.ScopeTimelineRelative,
				IncludedPhases:         []string{domain.PhaseRequirement, domain.PhaseDevelopment},
				PaymentMode:            domain.PaymentProfileRows,
				PaymentRows: []domain.PaymentRowInput{{
					WorkPeriod: "Kick off", Terms: "DP", AllocationKind: domain.PaymentAllocationInvestmentBPS, AllocationBPS: 10000,
				}},
				DocumentOptions: domain.DocumentOptionsInput{LetterPrefix: "VEN", TaxTreatment: domain.TaxExclusivePPN, ClauseCodes: []string{"out_of_pocket"}},
			},
		},
	}

	b, err := json.Marshal(req)
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	require.Contains(t, raw, "dedicated_squad")

	var decoded GenerateQuotationRequest
	require.NoError(t, json.Unmarshal(b, &decoded))
	require.NotNil(t, decoded.DedicatedSquad)
	require.Equal(t, domain.ModelSOWLarge, decoded.DedicatedSquad.Model)
	require.Equal(t, anchor, *decoded.DedicatedSquad.PaymentAnchorDate)
	require.NotNil(t, decoded.DedicatedSquad.QuotationProfile)
	require.Equal(t, domain.PreparationProfileHiring, decoded.DedicatedSquad.QuotationProfile.PreparationMode)
	require.Equal(t, int32(2500), *decoded.DedicatedSquad.QuotationProfile.PreparationDiscountBPS)
	require.Equal(t, domain.PaymentAllocationInvestmentBPS, decoded.DedicatedSquad.QuotationProfile.PaymentRows[0].AllocationKind)
	require.NoError(t, decoded.DedicatedSquad.Validate())
}

func TestGenerateQuotationRequest_SubscriptionSaaSJSONSerialization(t *testing.T) {
	quotationDate := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	req := GenerateQuotationRequest{
		Subject:          "Penawaran AI Percetakan",
		SolutionType:     "SaaS",
		CommercialScheme: domain.CommercialSchemeSubscriptionSaaS,
		SubscriptionSaaS: &domain.SubscriptionSaaSInput{
			ProductName:       "Subscription AI Percetakan",
			QuotationDate:     quotationDate,
			SectionLayout:     domain.SubscriptionLayoutSubscriptionDevelopment,
			BillingCycle:      domain.SubscriptionBillingQuarterly,
			TaxTreatment:      domain.SubscriptionTaxNotStated,
			InitialTermMonths: 12,
			PricePeriods: []domain.SubscriptionPricePeriodInput{{
				Label:            "Subscription 12 bulan",
				StartMonthOffset: 0,
				DurationMonths:   12,
				MonthlyRate:      1_250_000,
			}},
			PaymentSchedule: []domain.SubscriptionPaymentRowInput{
				{WorkPeriod: "1 Agustus 2026 - 31 Oktober 2026", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Nominal: 3_750_000},
				{WorkPeriod: "1 November 2026 - 31 Januari 2027", StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: "1 Oktober 2026", DueDateLabel: "15 Oktober 2026", Nominal: 3_750_000},
				{WorkPeriod: "1 Februari 2027 - 30 April 2027", StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: "1 Januari 2027", DueDateLabel: "15 Januari 2027", Nominal: 3_750_000},
				{WorkPeriod: "1 Mei 2027 - 31 Juli 2027", StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: "1 April 2027", DueDateLabel: "15 April 2027", Nominal: 3_750_000},
			},
			IsPartnership: true,
			IncomeSharing: &domain.IncomeSharingInput{
				VenturoShareBPS:            5_000,
				PartnerShareBPS:            5_000,
				EligibleRevenueDescription: "Penjualan yang berasal dari referensi mitra",
				SettlementBusinessDays:     30,
				ProhibitUnilateralDiscount: true,
			},
		},
	}

	t.Run("round trip serialization", func(t *testing.T) {
		b, err := json.Marshal(req)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &raw))
		require.Contains(t, raw, "subscription_saas")

		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NotNil(t, decoded.SubscriptionSaaS)
		assert.Equal(t, "Subscription AI Percetakan", decoded.SubscriptionSaaS.ProductName)
		assert.Equal(t, quotationDate, decoded.SubscriptionSaaS.QuotationDate.UTC())
		assert.Equal(t, domain.SubscriptionLayoutSubscriptionDevelopment, decoded.SubscriptionSaaS.SectionLayout)
		assert.Equal(t, domain.SubscriptionBillingQuarterly, decoded.SubscriptionSaaS.BillingCycle)
		assert.Equal(t, int32(12), decoded.SubscriptionSaaS.InitialTermMonths)
		assert.True(t, decoded.SubscriptionSaaS.IsPartnership)
		require.NotNil(t, decoded.SubscriptionSaaS.IncomeSharing)
		assert.Equal(t, int32(5_000), decoded.SubscriptionSaaS.IncomeSharing.VenturoShareBPS)
		assert.Equal(t, int32(5_000), decoded.SubscriptionSaaS.IncomeSharing.PartnerShareBPS)
		require.NoError(t, decoded.SubscriptionSaaS.Validate())
	})

	t.Run("rejects request with unknown nested field inside subscription_saas", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "subscription_saas",
			"subscription_saas": {
				"product_name": "Subscription AI Percetakan",
				"quotation_date": "2026-08-01",
				"section_layout": "subscription_development",
				"billing_cycle": "quarterly",
				"tax_treatment": "not_stated",
				"initial_term_months": 12,
				"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 12, "monthly_rate": 1250000}],
				"payment_schedule": [{"work_period": "Q1", "start_month_offset": 0, "covered_months": 3, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 3750000}],
				"is_partnership": false,
				"unrecognized_nested_field": "disallowed"
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside income_sharing", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "subscription_saas",
			"subscription_saas": {
				"product_name": "Subscription AI Percetakan",
				"quotation_date": "2026-08-01",
				"section_layout": "subscription_development",
				"billing_cycle": "quarterly",
				"tax_treatment": "not_stated",
				"initial_term_months": 12,
				"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 12, "monthly_rate": 1250000}],
				"payment_schedule": [{"work_period": "Q1", "start_month_offset": 0, "covered_months": 3, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 3750000}],
				"is_partnership": true,
				"income_sharing": {
					"venturo_share_bps": 5000,
					"partner_share_bps": 5000,
					"eligible_revenue_description": "Rev",
					"settlement_business_days": 30,
					"prohibit_unilateral_discount": true,
					"unknown_sub_field": 999
				}
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}

func TestGenerateQuotationRequest_WhiteLabelJSONSerialization(t *testing.T) {
	quotationDate := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	req := GenerateQuotationRequest{
		Subject:          "Penawaran POS Mini Market",
		SolutionType:     "White Label",
		CommercialScheme: domain.CommercialSchemeWhiteLabel,
		WhiteLabel: &domain.WhiteLabelInput{
			ProductName:   "POS Mini Market",
			QuotationDate: quotationDate,
			TaxTreatment:  domain.WhiteLabelTaxExclusive,
			LicenseItems:  []domain.WhiteLabelLicenseItemInput{{Name: "POS Mini Market", UnitRate: 35_000_000, Quantity: 1}},
			MaintenanceItems: []domain.WhiteLabelMaintenanceItemInput{
				{Name: "Senior Web Programmer", MonthlyRate: 16_170_000, FTEQuarters: 2, PeriodMonths: 3},
			},
			HostingItems: []domain.WhiteLabelHostingItemInput{
				{Name: "Shared Hosting Small", MonthlyRate: 350_000, Quantity: 1, PeriodMonths: 3},
			},
			ModuleGroups: []domain.WhiteLabelModuleGroupInput{
				{Name: "Inventory", Features: []string{"Stok masuk", "Stok keluar"}},
			},
			TimelineRows: []domain.WhiteLabelTimelineRowInput{
				{Description: "Persiapan Lingkungan", Timeline: "Minggu 1"},
			},
			PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
				{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
				{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			},
			RightsStatements: []string{"Hak cipta tetap pada Venturo."},
			LimitationCodes:  []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
		},
	}

	t.Run("round trip serialization", func(t *testing.T) {
		b, err := json.Marshal(req)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &raw))
		require.Contains(t, raw, "white_label")

		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NotNil(t, decoded.WhiteLabel)
		assert.Equal(t, "POS Mini Market", decoded.WhiteLabel.ProductName)
		assert.Equal(t, quotationDate, decoded.WhiteLabel.QuotationDate.UTC())
		assert.Equal(t, domain.WhiteLabelTaxExclusive, decoded.WhiteLabel.TaxTreatment)
		assert.Len(t, decoded.WhiteLabel.LicenseItems, 1)
		assert.Len(t, decoded.WhiteLabel.MaintenanceItems, 1)
		assert.Len(t, decoded.WhiteLabel.HostingItems, 1)
		assert.Len(t, decoded.WhiteLabel.ModuleGroups, 1)
		assert.Len(t, decoded.WhiteLabel.TimelineRows, 1)
		assert.Len(t, decoded.WhiteLabel.PaymentSchedule, 2)
		assert.Len(t, decoded.WhiteLabel.RightsStatements, 1)
		assert.Equal(t, []string{"out_of_pocket", "overtime", "third_party", "scope_change"}, decoded.WhiteLabel.LimitationCodes)
		require.NoError(t, decoded.WhiteLabel.Validate())
	})

	t.Run("rejects request with unknown nested field inside white_label", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "white_label",
			"white_label": {
				"product_name": "POS Mini Market",
				"quotation_date": "2026-09-07",
				"tax_treatment": "exclusive_ppn",
				"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
				"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
				"limitation_codes": ["out_of_pocket"],
				"unrecognized_field": "disallowed"
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside license_items", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "white_label",
			"white_label": {
				"product_name": "POS Mini Market",
				"quotation_date": "2026-09-07",
				"tax_treatment": "exclusive_ppn",
				"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1, "illegal_field": true}],
				"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
				"limitation_codes": ["out_of_pocket"]
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}

func TestGenerateQuotationRequest_StandaloneMaintenanceJSONSerialization(t *testing.T) {
	quotationDate := time.Date(2026, time.July, 7, 0, 0, 0, 0, time.UTC)
	req := GenerateQuotationRequest{
		Subject:          "Penawaran Standalone Maintenance Nex Health",
		SolutionType:     "Maintenance",
		CommercialScheme: domain.CommercialSchemeStandaloneMaintenance,
		StandaloneMaintenance: &domain.StandaloneMaintenanceInput{
			SystemName:    "Aplikasi Nex Health",
			QuotationDate: quotationDate,
			Mode:          domain.StandaloneMaintenanceRetainer,
			TaxTreatment:  domain.StandaloneTaxExclusive,
			RetainerItems: []domain.MaintenanceRetainerItemInput{
				{Name: "Senior Web Programmer", MonthlyRate: 16_050_000, FTEQuarters: 4, PeriodMonths: 1},
				{Name: "Senior Quality Assurance", MonthlyRate: 14_070_000, FTEQuarters: 2, PeriodMonths: 1},
			},
			DefectGroups: []domain.MaintenanceDefectGroupInput{{
				Title: "1. Detail Pasien",
				Items: []domain.MaintenanceDefectItemInput{{
					ItemNumber:  "1.1",
					Priority:    domain.StandalonePriorityP0,
					Function:    "Melihat riwayat medis pasien di luar sesi konsultasi.",
					ScreenFlow:  "Pasien > Data Pasien dan Riwayat > Lihat Detail Pasien > Medical History.",
					Existing:    "Hanya mencantumkan catatan registrasi awal.",
					Improvement: "Mencantumkan seluruh riwayat SOAP (Dokter, Terapis, dan Perawat).",
					Reference:   "Ref 1.01",
				}},
			}},
			ShowDiscountRow: true,
			DiscountAmount:  0,
			PaymentSchedule: []domain.MaintenancePaymentRowInput{{
				WorkPeriod:       "13 Juli - 13 Agustus 2026",
				Terms:            "Maintenance",
				InvoiceDateLabel: "13 Agustus 2026",
				DueDateLabel:     "27 Agustus 2026",
				Nominal:          23_085_000,
			}},
			RightsStatements: []string{"Source code tetap menjadi hak milik klien."},
		},
	}

	t.Run("round trip serialization for retainer mode", func(t *testing.T) {
		b, err := json.Marshal(req)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &raw))
		require.Contains(t, raw, "standalone_maintenance")

		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NotNil(t, decoded.StandaloneMaintenance)
		assert.Equal(t, "Aplikasi Nex Health", decoded.StandaloneMaintenance.SystemName)
		assert.Equal(t, quotationDate, decoded.StandaloneMaintenance.QuotationDate.UTC())
		assert.Equal(t, domain.StandaloneMaintenanceRetainer, decoded.StandaloneMaintenance.Mode)
		assert.Equal(t, domain.StandaloneTaxExclusive, decoded.StandaloneMaintenance.TaxTreatment)
		assert.Len(t, decoded.StandaloneMaintenance.RetainerItems, 2)
		assert.Len(t, decoded.StandaloneMaintenance.DefectGroups, 1)
		assert.Len(t, decoded.StandaloneMaintenance.DefectGroups[0].Items, 1)
		assert.True(t, decoded.StandaloneMaintenance.ShowDiscountRow)
		assert.Equal(t, int64(0), decoded.StandaloneMaintenance.DiscountAmount)
		assert.Len(t, decoded.StandaloneMaintenance.PaymentSchedule, 1)
		assert.Len(t, decoded.StandaloneMaintenance.RightsStatements, 1)
		require.NoError(t, decoded.StandaloneMaintenance.Validate())
	})

	t.Run("round trip serialization for hourly mode", func(t *testing.T) {
		hourlyDate := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
		hourlyReq := GenerateQuotationRequest{
			Subject:          "Penawaran Standalone Support Liputan6",
			SolutionType:     "Support",
			CommercialScheme: domain.CommercialSchemeStandaloneMaintenance,
			StandaloneMaintenance: &domain.StandaloneMaintenanceInput{
				SystemName:    "Portal Berita Liputan6",
				QuotationDate: hourlyDate,
				Mode:          domain.StandaloneMaintenanceHourlyTask,
				TaxTreatment:  domain.StandaloneTaxNotStated,
				HourlyCostItems: []domain.MaintenanceHourlyCostItemInput{
					{Name: "Senior Mobile", DisplayMonthlyRate: 16_050_000, Headcount: 1, HoursHalfUnits: 16, QuotedLineTotal: 730_000},
				},
				HourlyTasks: []domain.MaintenanceHourlyTaskInput{
					{Description: "Perbaikan bug", Platform: "Mobile Android", HoursHalfUnits: 16},
				},
				ShowDiscountRow: false,
				DiscountAmount:  0,
				PaymentSchedule: []domain.MaintenancePaymentRowInput{{
					WorkPeriod:       "Support Juli 2026",
					Terms:            "Pelunasan",
					InvoiceDateLabel: "31 Juli 2026",
					DueDateLabel:     "14 Agustus 2026",
					Nominal:          730_000,
				}},
			},
		}

		b, err := json.Marshal(hourlyReq)
		require.NoError(t, err)

		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NotNil(t, decoded.StandaloneMaintenance)
		assert.Equal(t, "Portal Berita Liputan6", decoded.StandaloneMaintenance.SystemName)
		assert.Equal(t, domain.StandaloneMaintenanceHourlyTask, decoded.StandaloneMaintenance.Mode)
		assert.Len(t, decoded.StandaloneMaintenance.HourlyCostItems, 1)
		assert.Len(t, decoded.StandaloneMaintenance.HourlyTasks, 1)
		require.NoError(t, decoded.StandaloneMaintenance.Validate())
	})

	t.Run("rejects request with unknown nested field inside standalone_maintenance", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "standalone_maintenance",
			"standalone_maintenance": {
				"system_name": "Aplikasi Nex Health",
				"quotation_date": "2026-07-07",
				"mode": "retainer",
				"tax_treatment": "exclusive_ppn",
				"retainer_items": [{"name": "Senior Web Programmer", "monthly_rate": 16050000, "fte_quarters": 4, "period_months": 1}],
				"defect_groups": [{"title": "1. Detail", "items": [{"item_number": "1.1", "priority": "P0", "function": "F", "screen_flow": "S", "existing": "E", "improvement": "I"}]}],
				"payment_schedule": [{"work_period": "P1", "terms": "Maint", "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 16050000}],
				"unrecognized_field": "disallowed"
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside retainer_items", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "standalone_maintenance",
			"standalone_maintenance": {
				"system_name": "Aplikasi Nex Health",
				"quotation_date": "2026-07-07",
				"mode": "retainer",
				"tax_treatment": "exclusive_ppn",
				"retainer_items": [{"name": "Senior Web Programmer", "monthly_rate": 16050000, "fte_quarters": 4, "period_months": 1, "illegal_prop": 999}],
				"defect_groups": [{"title": "1. Detail", "items": [{"item_number": "1.1", "priority": "P0", "function": "F", "screen_flow": "S", "existing": "E", "improvement": "I"}]}],
				"payment_schedule": [{"work_period": "P1", "terms": "Maint", "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 16050000}]
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside defect_groups items", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "standalone_maintenance",
			"standalone_maintenance": {
				"system_name": "Aplikasi Nex Health",
				"quotation_date": "2026-07-07",
				"mode": "retainer",
				"tax_treatment": "exclusive_ppn",
				"retainer_items": [{"name": "Senior Web Programmer", "monthly_rate": 16050000, "fte_quarters": 4, "period_months": 1}],
				"defect_groups": [{"title": "1. Detail", "items": [{"item_number": "1.1", "priority": "P0", "function": "F", "screen_flow": "S", "existing": "E", "improvement": "I", "unknown_nested": true}]}],
				"payment_schedule": [{"work_period": "P1", "terms": "Maint", "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 16050000}]
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside hourly_tasks", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "standalone_maintenance",
			"standalone_maintenance": {
				"system_name": "Portal Berita Liputan6",
				"quotation_date": "2026-07-10",
				"mode": "hourly_task",
				"tax_treatment": "not_stated",
				"hourly_cost_items": [{"name": "Dev", "display_monthly_rate": 16050000, "headcount": 1, "hours_half_units": 16, "quoted_line_total": 730000}],
				"hourly_tasks": [{"description": "Fix bug", "platform": "Mobile", "hours_half_units": 16, "unknown_task_attr": "nope"}],
				"payment_schedule": [{"work_period": "P1", "terms": "Maint", "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 730000}]
			}
		}`

		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}

func TestStandaloneMaintenancePreviewResponse_JSONSerialization(t *testing.T) {
	resp := StandaloneMaintenancePreviewResponse{
		Calculation: domain.StandaloneMaintenanceCalculation{
			Mode: domain.StandaloneMaintenanceRetainer,
			Retainer: []domain.MaintenanceRetainerLineCalculation{{
				MaintenanceRetainerItemInput: domain.MaintenanceRetainerItemInput{
					Name: "Senior Web Programmer", MonthlyRate: 16_050_000, FTEQuarters: 4, PeriodMonths: 1,
				},
				LineTotal: 16_050_000,
			}},
			Subtotal:        16_050_000,
			DiscountAmount:  0,
			TotalInvestment: 16_050_000,
			PaymentTotal:    16_050_000,
		},
		ResolvedProfile: domain.ResolvedStandaloneMaintenanceProfile{
			Mode:            domain.StandaloneMaintenanceRetainer,
			TaxTreatment:    domain.StandaloneTaxExclusive,
			TaxClauseText:   "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.",
			ShowDiscountRow: true,
			ShowRights:      false,
			Sections: []domain.DocumentSection{
				{Number: 1, Code: "maintenance_cost", Title: "Biaya Maintenance"},
			},
		},
	}

	b, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded StandaloneMaintenancePreviewResponse
	require.NoError(t, json.Unmarshal(b, &decoded))
	assert.Equal(t, domain.StandaloneMaintenanceRetainer, decoded.Calculation.Mode)
	assert.Equal(t, int64(16_050_000), decoded.Calculation.TotalInvestment)
	assert.Equal(t, domain.StandaloneTaxExclusive, decoded.ResolvedProfile.TaxTreatment)
	assert.Len(t, decoded.ResolvedProfile.Sections, 1)
	assert.Equal(t, "Biaya Maintenance", decoded.ResolvedProfile.Sections[0].Title)
}

func TestGenerateQuotationRequest_CustomDevelopmentJSONSerialization(t *testing.T) {
	quotationDate := time.Date(2026, time.July, 15, 0, 0, 0, 0, time.UTC)
	rate := int64(15_000_000)
	quotedTotal := int64(15_000_000)

	req := GenerateQuotationRequest{
		Subject:          "Penawaran Sistem Kopi Studio24",
		SolutionType:     "Custom Development",
		CommercialScheme: domain.CommercialSchemeCustomDevelopment,
		CustomDevelopment: &domain.CustomDevelopmentInput{
			SystemName:           "Sistem Kopi Studio24",
			QuotationDate:        quotationDate,
			DevelopmentTableMode: domain.CustomDevelopmentMonthlyPeriod,
			TaxTreatment:         domain.CustomDevelopmentTaxExclusive,
			DevelopmentGroups: []domain.CustomDevelopmentCostGroupInput{
				{
					Title: "Fase 1: Backend",
					Items: []domain.CustomDevelopmentCostItemInput{
						{
							Name:            "Senior Developer",
							DisplayRate:     &rate,
							Headcount:       1,
							PeriodLabel:     "1",
							QuotedLineTotal: &quotedTotal,
						},
						{
							Name:            "QA Tester (Included)",
							DisplayRate:     nil,
							Headcount:       1,
							PeriodLabel:     "1",
							QuotedLineTotal: nil,
						},
					},
				},
			},
			DiscountLabel:   "DISCOUNT",
			DiscountAmount:  1_000_000,
			ShowDiscountRow: true,
			MonthlyItems: []domain.CustomDevelopmentMonthlyItemInput{
				{Name: "Cloud Server", UnitRate: 1_500_000, Quantity: 1},
			},
			MonthlyNotes: []string{"Server cloud bulanan."},
			ModuleGroups: []domain.CustomDevelopmentModuleGroupInput{
				{Name: "Transaksi", Features: []string{"Fitur checkout"}},
			},
			TimelineRows: []domain.CustomDevelopmentTimelineRowInput{
				{Description: "Fase 1", Timeline: "1 Bulan"},
			},
			PaymentSchedule: []domain.CustomDevelopmentPaymentRowInput{
				{WorkPeriod: "P1", Terms: "DP", InvoiceDateLabel: "Inv 1", DueDateLabel: "Due 1", Component: domain.CustomDevelopmentPaymentProject, Nominal: 14_000_000},
				{WorkPeriod: "B1", Terms: "Monthly", InvoiceDateLabel: "Inv M", DueDateLabel: "Due M", Component: domain.CustomDevelopmentPaymentMonthly, Nominal: 1_500_000},
			},
			RightsStatements:      []string{"Hak cipta diserahkan."},
			LimitationCodes:       []string{"out_of_pocket"},
			AdditionalLimitations: []string{"Tidak termasuk domain."},
		},
	}

	t.Run("round trip serialization with nested date and omitted null quoted total", func(t *testing.T) {
		b, err := json.Marshal(req)
		require.NoError(t, err)

		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &raw))
		require.Contains(t, raw, "commercial_scheme")
		assert.JSONEq(t, `"custom_development"`, string(raw["commercial_scheme"]))
		require.Contains(t, raw, "custom_development")

		// Verify that the included line with QuotedLineTotal = nil omits the quoted_line_total key
		var devMap map[string]interface{}
		require.NoError(t, json.Unmarshal(raw["custom_development"], &devMap))
		groups := devMap["development_groups"].([]interface{})
		group0 := groups[0].(map[string]interface{})
		items := group0["items"].([]interface{})
		item1 := items[1].(map[string]interface{})
		_, hasQuotedLineTotal := item1["quoted_line_total"]
		assert.False(t, hasQuotedLineTotal, "nil quoted_line_total should be omitted in serialized JSON")

		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NotNil(t, decoded.CustomDevelopment)
		assert.Equal(t, "custom_development", decoded.CommercialScheme)
		assert.Equal(t, "Sistem Kopi Studio24", decoded.CustomDevelopment.SystemName)
		assert.Equal(t, quotationDate, decoded.CustomDevelopment.QuotationDate.UTC())
		assert.Len(t, decoded.CustomDevelopment.DevelopmentGroups, 1)
		assert.Len(t, decoded.CustomDevelopment.DevelopmentGroups[0].Items, 2)
		assert.NotNil(t, decoded.CustomDevelopment.DevelopmentGroups[0].Items[0].QuotedLineTotal)
		assert.Nil(t, decoded.CustomDevelopment.DevelopmentGroups[0].Items[1].QuotedLineTotal)
		require.NoError(t, decoded.CustomDevelopment.Validate())
	})

	t.Run("unmarshals input with explicit quoted_line_total null as included line", func(t *testing.T) {
		payload := `{
			"commercial_scheme": "custom_development",
			"custom_development": {
				"system_name": "Sistem Kopi Studio24",
				"quotation_date": "2026-07-15",
				"development_table_mode": "monthly_period",
				"tax_treatment": "exclusive_ppn",
				"development_groups": [
					{
						"title": "Fase 1",
						"items": [
							{"name": "Dev", "headcount": 1, "period_label": "1", "quoted_line_total": 15000000},
							{"name": "QA Included", "headcount": 1, "period_label": "1", "quoted_line_total": null}
						]
					}
				],
				"discount_label": "DISCOUNT",
				"discount_amount": 0,
				"show_discount_row": false,
				"module_groups": [{"name": "M1", "features": ["F1"]}],
				"timeline_rows": [{"description": "T1", "timeline": "1 bln"}],
				"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Inv", "due_date_label": "Due", "component": "development", "nominal": 15000000}],
				"limitation_codes": ["out_of_pocket"]
			}
		}`
		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal([]byte(payload), &decoded))
		require.NotNil(t, decoded.CustomDevelopment)
		assert.Nil(t, decoded.CustomDevelopment.DevelopmentGroups[0].Items[1].QuotedLineTotal)
		require.NoError(t, decoded.CustomDevelopment.Validate())
	})

	t.Run("rejects request with unknown nested field inside custom_development", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "custom_development",
			"custom_development": {
				"system_name": "Sistem",
				"quotation_date": "2026-07-15",
				"development_table_mode": "monthly_period",
				"tax_treatment": "exclusive_ppn",
				"unrecognized_field": "disallowed"
			}
		}`
		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside development_groups", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "custom_development",
			"custom_development": {
				"system_name": "Sistem",
				"quotation_date": "2026-07-15",
				"development_table_mode": "monthly_period",
				"tax_treatment": "exclusive_ppn",
				"development_groups": [
					{
						"title": "Fase 1",
						"unknown_group_field": "disallowed",
						"items": [{"name": "Dev", "headcount": 1, "period_label": "1", "quoted_line_total": 15000000}]
					}
				],
				"discount_amount": 0,
				"show_discount_row": false,
				"module_groups": [{"name": "M1", "features": ["F1"]}],
				"timeline_rows": [{"description": "T1", "timeline": "1 bln"}],
				"payment_schedule": [{"work_period": "P1", "terms": "DP", "invoice_date_label": "I", "due_date_label": "D", "component": "development", "nominal": 15000000}],
				"limitation_codes": ["out_of_pocket"]
			}
		}`
		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside items", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "custom_development",
			"custom_development": {
				"system_name": "Sistem",
				"quotation_date": "2026-07-15",
				"development_table_mode": "monthly_period",
				"tax_treatment": "exclusive_ppn",
				"development_groups": [
					{
						"title": "Fase 1",
						"items": [{"name": "Dev", "headcount": 1, "period_label": "1", "quoted_line_total": 15000000, "illegal_item_prop": 999}]
					}
				],
				"discount_amount": 0,
				"show_discount_row": false,
				"module_groups": [{"name": "M1", "features": ["F1"]}],
				"timeline_rows": [{"description": "T1", "timeline": "1 bln"}],
				"payment_schedule": [{"work_period": "P1", "terms": "DP", "invoice_date_label": "I", "due_date_label": "D", "component": "development", "nominal": 15000000}],
				"limitation_codes": ["out_of_pocket"]
			}
		}`
		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}

func TestGenerateQuotationRequest_KioskQueueSystemJSONSerialization(t *testing.T) {
	quotationDate := time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)
	req := GenerateQuotationRequest{
		Subject:          "Pengadaan Antrian MPP",
		SolutionType:     "Sistem Antrian",
		CommercialScheme: domain.CommercialSchemeKioskQueueSystem,
		KioskQueueSystem: &domain.KioskQueueSystemInput{
			Variant:           domain.KioskQueueGovernmentMPP,
			DocumentNumber:    "LANDA/MPP-Antrian/0726/003",
			SystemName:        "Sistem Antrian Terpadu MPP",
			QuotationDate:     quotationDate,
			HardwareProvision: domain.HardwareVenturoSupplied,
			TaxTreatment:      domain.KioskTaxInclusivePPN11,
			GovernmentScopePhases: []domain.KioskQueueScopePhaseInput{
				{
					Title:        "Fase 1: Persiapan",
					Deliverables: []string{"Dokumen Analisis"},
				},
			},
			ExpertRequirements: []domain.KioskQueueExpertInput{
				{
					Role:                 "Project Manager",
					Headcount:            1,
					DurationLabel:        "3 Bulan",
					MinimumQualification: "S1",
				},
			},
			ServiceGroups: []domain.KioskQueueServiceGroupInput{
				{
					Title: "Jasa",
					Items: []domain.KioskQueueServiceItemInput{
						{
							Name:           "Backend Engineer",
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
					Name:      "Kiosk 21.5",
					UnitPrice: 25_000_000,
					Quantity:  1,
				},
			},
			GovernmentPaymentSchedule: []domain.KioskQueueGovernmentPaymentInput{
				{
					Stage:        "DP",
					BillingBasis: "SPMK",
					Nominal:      25_000_000,
				},
			},
			PaymentDueDays: 14,
			LimitationCodes: []domain.KioskQueueLimitationCode{
				domain.KioskLimitCivilElectrical,
			},
			AdditionalLimitations: []string{"Tidak termasuk instalasi sipil."},
			RightsStatements:      []string{"Hak cipta diserahkan."},
		},
	}

	t.Run("round trip serialization and field verification", func(t *testing.T) {
		b, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded GenerateQuotationRequest
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NotNil(t, decoded.KioskQueueSystem)
		assert.Equal(t, "government_mpp", string(decoded.KioskQueueSystem.Variant))
		assert.Equal(t, "LANDA/MPP-Antrian/0726/003", decoded.KioskQueueSystem.DocumentNumber)
		assert.Nil(t, decoded.CustomDevelopment)
		assert.Empty(t, decoded.RateCardID)
		assert.Equal(t, quotationDate, decoded.KioskQueueSystem.QuotationDate.UTC())
		require.NoError(t, decoded.KioskQueueSystem.Validate())
	})

	t.Run("rejects date other than YYYY-MM-DD (e.g. 30/07/2026)", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "kiosk_queue_system",
			"kiosk_queue_system": {
				"variant": "government_mpp",
				"document_number": "LANDA/MPP-Antrian/0726/003",
				"system_name": "Sistem Antrian",
				"quotation_date": "30-07-2026",
				"hardware_provision": "venturo_supplied",
				"tax_treatment": "inclusive_ppn_11"
			}
		}`
		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date")
	})

	t.Run("rejects request with unknown nested field inside kiosk_queue_system", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "kiosk_queue_system",
			"kiosk_queue_system": {
				"variant": "government_mpp",
				"document_number": "LANDA/MPP-Antrian/0726/003",
				"system_name": "Sistem Antrian",
				"quotation_date": "2026-07-30",
				"hardware_provision": "venturo_supplied",
				"tax_treatment": "inclusive_ppn_11",
				"unrecognized_field": "disallowed"
			}
		}`
		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects request with unknown nested field inside service_groups items", func(t *testing.T) {
		badPayload := `{
			"commercial_scheme": "kiosk_queue_system",
			"kiosk_queue_system": {
				"variant": "government_mpp",
				"document_number": "LANDA/MPP-Antrian/0726/003",
				"system_name": "Sistem Antrian",
				"quotation_date": "2026-07-30",
				"hardware_provision": "venturo_supplied",
				"tax_treatment": "inclusive_ppn_11",
				"service_groups": [
					{
						"title": "Jasa",
						"items": [
							{"name": "Dev", "monthly_rate": 20000000, "headcount": 1, "period_quarters": 4, "illegal_item_prop": 999}
						]
					}
				]
			}
		}`
		var decoded GenerateQuotationRequest
		err := json.Unmarshal([]byte(badPayload), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}
