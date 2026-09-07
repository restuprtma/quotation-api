package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validHubungInput() SubscriptionSaaSInput {
	schedule := make([]SubscriptionPaymentRowInput, 12)
	for i := int32(0); i < 12; i++ {
		schedule[i] = SubscriptionPaymentRowInput{
			WorkPeriod:       fmt.Sprintf("Bulan %d", i+1),
			StartMonthOffset: i,
			CoveredMonths:    1,
			InvoiceDateLabel: fmt.Sprintf("Tanggal 1 Bulan %d", i+1),
			DueDateLabel:     "H+14",
			Nominal:          1_000_000,
		}
	}
	return SubscriptionSaaSInput{
		ProductName:       "Hubung",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      SubscriptionBillingMonthly,
		TaxTreatment:      SubscriptionTaxNotStated,
		InitialTermMonths: 12,
		AutoRenewal:       true,
		PricePeriods: []SubscriptionPricePeriodInput{{
			Label:            "Subscription Bulanan Hubung",
			StartMonthOffset: 0,
			DurationMonths:   12,
			MonthlyRate:      1_000_000,
		}},
		PaymentSchedule:  schedule,
		RightsClauseCode: RightsClauseVenturoSoftwareOwnershipV1,
		IsPartnership:    false,
	}
}

func validPercetakanInput() SubscriptionSaaSInput {
	return SubscriptionSaaSInput{
		ProductName:       "Subscription AI Percetakan",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     SubscriptionLayoutSubscriptionDevelopment,
		BillingCycle:      SubscriptionBillingQuarterly,
		TaxTreatment:      SubscriptionTaxNotStated,
		InitialTermMonths: 12,
		PricePeriods: []SubscriptionPricePeriodInput{{
			Label:            "Subscription 12 bulan",
			StartMonthOffset: 0,
			DurationMonths:   12,
			MonthlyRate:      1_250_000,
		}},
		PaymentSchedule: []SubscriptionPaymentRowInput{
			{WorkPeriod: "1 Agustus 2026 - 31 Oktober 2026", StartMonthOffset: 0, CoveredMonths: 3, InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Nominal: 3_750_000},
			{WorkPeriod: "1 November 2026 - 31 Januari 2027", StartMonthOffset: 3, CoveredMonths: 3, InvoiceDateLabel: "1 Oktober 2026", DueDateLabel: "15 Oktober 2026", Nominal: 3_750_000},
			{WorkPeriod: "1 Februari 2027 - 30 April 2027", StartMonthOffset: 6, CoveredMonths: 3, InvoiceDateLabel: "1 Januari 2027", DueDateLabel: "15 Januari 2027", Nominal: 3_750_000},
			{WorkPeriod: "1 Mei 2027 - 31 Juli 2027", StartMonthOffset: 9, CoveredMonths: 3, InvoiceDateLabel: "1 April 2027", DueDateLabel: "15 April 2027", Nominal: 3_750_000},
		},
		IsPartnership: true,
		IncomeSharing: &IncomeSharingInput{
			VenturoShareBPS:            5_000,
			PartnerShareBPS:            5_000,
			EligibleRevenueDescription: "Penjualan yang berasal dari referensi mitra",
			SettlementBusinessDays:     30,
			ProhibitUnilateralDiscount: true,
		},
	}
}

func validAnnualDevelopmentMaintenanceInput() SubscriptionSaaSInput {
	return SubscriptionSaaSInput{
		ProductName:       "Annual SaaS",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     SubscriptionLayoutDevelopmentMaintenance,
		BillingCycle:      SubscriptionBillingYearly,
		TaxTreatment:      SubscriptionTaxExclusive,
		InitialTermMonths: 24,
		PricePeriods: []SubscriptionPricePeriodInput{
			{Label: "Tahun 1 - Development", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_000_000},
			{Label: "Tahun 2 - Maintenance", StartMonthOffset: 12, DurationMonths: 12, MonthlyRate: 1_500_000},
		},
		PaymentSchedule: []SubscriptionPaymentRowInput{
			{WorkPeriod: "Tahun 1", StartMonthOffset: 0, CoveredMonths: 12, InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Nominal: 12_000_000},
			{WorkPeriod: "Tahun 2", StartMonthOffset: 12, CoveredMonths: 12, InvoiceDateLabel: "Awal tahun kedua", DueDateLabel: "H+14", Nominal: 18_000_000},
		},
		FeatureGroups: []SubscriptionFeatureGroupInput{
			{Name: "Core Modules", Features: []string{"User Management", "Authentication", "Dashboard"}},
		},
		IsPartnership: false,
	}
}

func TestSubscriptionSaaSInput_ValidateMatrix(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SubscriptionSaaSInput)
		wantErr string
	}{
		{
			name:   "valid Hubung input",
			mutate: func(in *SubscriptionSaaSInput) {},
		},
		{
			name: "valid Percetakan 50:50 input",
			mutate: func(in *SubscriptionSaaSInput) {
				*in = validPercetakanInput()
			},
		},
		{
			name: "valid annual development maintenance input with features",
			mutate: func(in *SubscriptionSaaSInput) {
				*in = validAnnualDevelopmentMaintenanceInput()
			},
		},
		{
			name: "development_maintenance layout without features fails",
			mutate: func(in *SubscriptionSaaSInput) {
				*in = validAnnualDevelopmentMaintenanceInput()
				in.FeatureGroups = nil
			},
			wantErr: "feature_groups: is required for development_maintenance layout",
		},
		{
			name: "gap price period fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.InitialTermMonths = 12
				in.PricePeriods = []SubscriptionPricePeriodInput{
					{Label: "Period 1", StartMonthOffset: 0, DurationMonths: 6, MonthlyRate: 1_000_000},
					{Label: "Period 2", StartMonthOffset: 7, DurationMonths: 5, MonthlyRate: 1_000_000},
				}
			},
			wantErr: "price_periods[1].start_month_offset: must be contiguous",
		},
		{
			name: "price period does not start at 0 fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.PricePeriods[0].StartMonthOffset = 1
			},
			wantErr: "price_periods[0].start_month_offset: must be contiguous and start at 0",
		},
		{
			name: "price period total duration does not equal initial term months",
			mutate: func(in *SubscriptionSaaSInput) {
				in.PricePeriods[0].DurationMonths = 6
			},
			wantErr: "price_periods: total duration (6 months) must equal initial term (12 months)",
		},
		{
			name: "payment total covered months does not equal initial term",
			mutate: func(in *SubscriptionSaaSInput) {
				// Only 11 months covered instead of 12
				in.PaymentSchedule = in.PaymentSchedule[:11]
			},
			wantErr: "payment_schedule: total covered months (11) must equal initial term (12)",
		},
		{
			name: "payment schedule gap in offset fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.PaymentSchedule[1].StartMonthOffset = 2 // should be 1
			},
			wantErr: "payment_schedule[1].start_month_offset: must be contiguous and start at 0",
		},
		{
			name: "payment row covered months invalid for billing cycle",
			mutate: func(in *SubscriptionSaaSInput) {
				in.BillingCycle = SubscriptionBillingMonthly
				in.PaymentSchedule[0].CoveredMonths = 2
			},
			wantErr: "payment_schedule[0].covered_months: must be 1 for monthly billing cycle",
		},
		{
			name: "payment row covered months invalid for quarterly billing cycle",
			mutate: func(in *SubscriptionSaaSInput) {
				*in = validPercetakanInput()
				in.PaymentSchedule[0].CoveredMonths = 2 // first row of multi-row must be 3
			},
			wantErr: "payment_schedule[0].covered_months: must be 3 for quarterly billing cycle",
		},
		{
			name: "partnership without income sharing fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.IsPartnership = true
				in.IncomeSharing = nil
			},
			wantErr: "income_sharing: is required when is_partnership is true",
		},
		{
			name: "non-partnership with income sharing fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.IsPartnership = false
				in.IncomeSharing = &IncomeSharingInput{
					VenturoShareBPS:            5_000,
					PartnerShareBPS:            5_000,
					EligibleRevenueDescription: "Penjualan referensi",
					SettlementBusinessDays:     30,
				}
			},
			wantErr: "income_sharing: must be empty when is_partnership is false",
		},
		{
			name: "share not 10000 BPS fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.IsPartnership = true
				in.IncomeSharing = &IncomeSharingInput{
					VenturoShareBPS:            6_000,
					PartnerShareBPS:            3_000,
					EligibleRevenueDescription: "Penjualan referensi",
					SettlementBusinessDays:     30,
				}
			},
			wantErr: "income_sharing: shares must total 10000 basis points",
		},
		{
			name: "unsupported section layout fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.SectionLayout = "invalid_layout"
			},
			wantErr: "section_layout: unsupported value \"invalid_layout\"",
		},
		{
			name: "unsupported billing cycle fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.BillingCycle = "weekly"
			},
			wantErr: "billing_cycle: unsupported value \"weekly\"",
		},
		{
			name: "unsupported tax treatment fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.TaxTreatment = "vat_included"
			},
			wantErr: "tax_treatment: unsupported value \"vat_included\"",
		},
		{
			name: "initial term months 0 fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.InitialTermMonths = 0
			},
			wantErr: "initial_term_months: must be between 1 and 60",
		},
		{
			name: "initial term months above 60 fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.InitialTermMonths = 61
			},
			wantErr: "initial_term_months: must be between 1 and 60",
		},
		{
			name: "empty product name fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.ProductName = "   "
			},
			wantErr: "product_name: is required",
		},
		{
			name: "zero quotation date fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.QuotationDate = time.Time{}
			},
			wantErr: "quotation_date: is required",
		},
		{
			name: "unsupported rights clause code fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.RightsClauseCode = "custom_unsupported_rights"
			},
			wantErr: "rights_clause_code: unsupported value \"custom_unsupported_rights\"",
		},
		{
			name: "eligible revenue description exceeding 2000 runes fails",
			mutate: func(in *SubscriptionSaaSInput) {
				*in = validPercetakanInput()
				in.IncomeSharing.EligibleRevenueDescription = strings.Repeat("A", 2001)
			},
			wantErr: "income_sharing.eligible_revenue_description: must be between 1 and 2000 characters",
		},
		{
			name: "monthly rate zero fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.PricePeriods[0].MonthlyRate = 0
			},
			wantErr: "price_periods[0].monthly_rate: must be greater than zero",
		},
		{
			name: "monthly rate exceeds safe amount fails",
			mutate: func(in *SubscriptionSaaSInput) {
				in.PricePeriods[0].MonthlyRate = MaxSubscriptionSafeAmount + 1
			},
			wantErr: "price_periods[0].monthly_rate: exceeds maximum safe amount",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validHubungInput()
			tc.mutate(&input)
			err := input.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			}
		})
	}
}

func TestSubscriptionSaaSInput_UnmarshalJSONStrict(t *testing.T) {
	t.Run("valid Hubung payload unmarshals successfully", func(t *testing.T) {
		input := validHubungInput()
		b, err := json.Marshal(input)
		require.NoError(t, err)

		var decoded SubscriptionSaaSInput
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NoError(t, decoded.Validate())
		assert.Equal(t, input.ProductName, decoded.ProductName)
		assert.Equal(t, input.InitialTermMonths, decoded.InitialTermMonths)
		assert.Equal(t, input.QuotationDate.UTC(), decoded.QuotationDate.UTC())
	})

	t.Run("accepts calendar date format YYYY-MM-DD", func(t *testing.T) {
		raw := `{
			"product_name": "Hubung",
			"quotation_date": "2026-08-01",
			"section_layout": "subscription_development",
			"billing_cycle": "monthly",
			"tax_treatment": "not_stated",
			"initial_term_months": 1,
			"auto_renewal": false,
			"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 1, "monthly_rate": 1000000}],
			"payment_schedule": [{"work_period": "B1", "start_month_offset": 0, "covered_months": 1, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 1000000}],
			"is_partnership": false
		}`
		var decoded SubscriptionSaaSInput
		require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
		assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), decoded.QuotationDate.UTC())
		require.NoError(t, decoded.Validate())
	})

	t.Run("rejects top-level unknown field", func(t *testing.T) {
		raw := `{
			"product_name": "Hubung",
			"quotation_date": "2026-08-01",
			"section_layout": "subscription_development",
			"billing_cycle": "monthly",
			"tax_treatment": "not_stated",
			"initial_term_months": 1,
			"auto_renewal": false,
			"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 1, "monthly_rate": 1000000}],
			"payment_schedule": [{"work_period": "B1", "start_month_offset": 0, "covered_months": 1, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 1000000}],
			"is_partnership": false,
			"unexpected_field": "hack"
		}`
		var decoded SubscriptionSaaSInput
		require.Error(t, json.Unmarshal([]byte(raw), &decoded))
	})

	t.Run("rejects nested unknown field in price_periods", func(t *testing.T) {
		raw := `{
			"product_name": "Hubung",
			"quotation_date": "2026-08-01",
			"section_layout": "subscription_development",
			"billing_cycle": "monthly",
			"tax_treatment": "not_stated",
			"initial_term_months": 1,
			"auto_renewal": false,
			"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 1, "monthly_rate": 1000000, "illegal_period_flag": true}],
			"payment_schedule": [{"work_period": "B1", "start_month_offset": 0, "covered_months": 1, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 1000000}],
			"is_partnership": false
		}`
		var decoded SubscriptionSaaSInput
		require.Error(t, json.Unmarshal([]byte(raw), &decoded))
	})

	t.Run("rejects nested unknown field in payment_schedule", func(t *testing.T) {
		raw := `{
			"product_name": "Hubung",
			"quotation_date": "2026-08-01",
			"section_layout": "subscription_development",
			"billing_cycle": "monthly",
			"tax_treatment": "not_stated",
			"initial_term_months": 1,
			"auto_renewal": false,
			"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 1, "monthly_rate": 1000000}],
			"payment_schedule": [{"work_period": "B1", "start_month_offset": 0, "covered_months": 1, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 1000000, "unexpected_fee": 50000}],
			"is_partnership": false
		}`
		var decoded SubscriptionSaaSInput
		require.Error(t, json.Unmarshal([]byte(raw), &decoded))
	})

	t.Run("rejects nested unknown field in income_sharing", func(t *testing.T) {
		raw := `{
			"product_name": "AI Percetakan",
			"quotation_date": "2026-08-01",
			"section_layout": "subscription_development",
			"billing_cycle": "quarterly",
			"tax_treatment": "not_stated",
			"initial_term_months": 3,
			"auto_renewal": false,
			"price_periods": [{"label": "P1", "start_month_offset": 0, "duration_months": 3, "monthly_rate": 1000000}],
			"payment_schedule": [{"work_period": "B1", "start_month_offset": 0, "covered_months": 3, "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 3000000}],
			"is_partnership": true,
			"income_sharing": {
				"venturo_share_bps": 5000,
				"partner_share_bps": 5000,
				"eligible_revenue_description": "Rev",
				"settlement_business_days": 30,
				"prohibit_unilateral_discount": true,
				"unauthorized_clause": "override"
			}
		}`
		var decoded SubscriptionSaaSInput
		require.Error(t, json.Unmarshal([]byte(raw), &decoded))
	})
}
