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
