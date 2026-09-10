package domain

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ptrInt64(v int64) *int64 {
	return &v
}

func validCustomDevelopmentInput() CustomDevelopmentInput {
	return CustomDevelopmentInput{
		SystemName:           "Sistem Kopi Studio24",
		QuotationDate:        time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC),
		DevelopmentTableMode: CustomDevelopmentMonthlyPeriod,
		TaxTreatment:         CustomDevelopmentTaxExclusive,
		DevelopmentGroups: []CustomDevelopmentCostGroupInput{
			{
				Title: "Fase 1: Backend & Core Engine",
				Items: []CustomDevelopmentCostItemInput{
					{
						Name:            "Senior Backend Developer",
						DisplayRate:     ptrInt64(15_000_000),
						Headcount:       1,
						PeriodLabel:     "1",
						QuotedLineTotal: ptrInt64(15_000_000),
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
		MonthlyItems: []CustomDevelopmentMonthlyItemInput{
			{
				Name:          "Server Cloud Hosting",
				Specification: "2 vCPU, 4 GB RAM",
				UnitRate:      1_500_000,
				Quantity:      1,
			},
		},
		MonthlyNotes: []string{
			"Biaya server dibayarkan setiap awal bulan.",
		},
		ModuleGroups: []CustomDevelopmentModuleGroupInput{
			{
				Name: "Modul Transaksi",
				Features: []string{
					"Pencatatan order dan invoice",
					"Integrasi payment gateway",
				},
			},
		},
		TimelineRows: []CustomDevelopmentTimelineRowInput{
			{
				Description: "Fase 1: Setup dan Backend",
				Timeline:    "4 Minggu",
			},
		},
		PaymentSchedule: []CustomDevelopmentPaymentRowInput{
			{
				WorkPeriod:       "Kickoff Proyek",
				Terms:            "Down Payment 50%",
				InvoiceDateLabel: "Saat tanda tangan kontrak",
				DueDateLabel:     "H+7 setelah invoice",
				Component:        CustomDevelopmentPaymentProject,
				Nominal:          7_000_000,
			},
			{
				WorkPeriod:       "Serah Terima Sistem",
				Terms:            "Pelunasan 50%",
				InvoiceDateLabel: "Setelah UAT selesai",
				DueDateLabel:     "H+7 setelah invoice",
				Component:        CustomDevelopmentPaymentProject,
				Nominal:          7_000_000,
			},
			{
				WorkPeriod:       "Bulan 1",
				Terms:            "Tagihan Bulanan Hosting",
				InvoiceDateLabel: "Awal bulan layanan",
				DueDateLabel:     "H+7",
				Component:        CustomDevelopmentPaymentMonthly,
				Nominal:          1_500_000,
			},
		},
		RightsStatements: []string{
			"Source code menjadi hak milik klien setelah pelunasan.",
		},
		LimitationCodes: []string{
			"out_of_pocket",
			"overtime",
			"third_party",
			"scope_change",
		},
		AdditionalLimitations: []string{
			"Tidak termasuk lisensi font berbayar pihak ketiga.",
		},
	}
}

func TestCustomDevelopmentInputValidate(t *testing.T) {
	valid := validCustomDevelopmentInput()
	tests := []struct {
		name   string
		mutate func(*CustomDevelopmentInput)
		want   string
	}{
		{"unknown table mode", func(v *CustomDevelopmentInput) { v.DevelopmentTableMode = "hourly" }, "development_table_mode"},
		{"no priced line", func(v *CustomDevelopmentInput) { v.DevelopmentGroups[0].Items[0].QuotedLineTotal = nil }, "priced"},
		{"negative discount", func(v *CustomDevelopmentInput) { v.DiscountAmount = -1 }, "discount_amount"},
		{"hidden discount", func(v *CustomDevelopmentInput) { v.DiscountAmount = 1; v.ShowDiscountRow = false }, "show_discount_row"},
		{"monthly note without item", func(v *CustomDevelopmentInput) { v.MonthlyItems = nil; v.MonthlyNotes = []string{"Gemini token"} }, "monthly_notes"},
		{"duplicate limitation", func(v *CustomDevelopmentInput) { v.LimitationCodes = []string{"overtime", "overtime"} }, "limitation_codes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validCustomDevelopmentInput()
			_ = valid
			tt.mutate(&got)
			require.ErrorContains(t, got.Validate(), tt.want)
		})
	}
}

func TestCustomDevelopmentInput_Validate_ValidCases(t *testing.T) {
	t.Run("valid input passes", func(t *testing.T) {
		valid := validCustomDevelopmentInput()
		require.NoError(t, valid.Validate())
	})

	t.Run("valid input with nil quoted total on some items passes", func(t *testing.T) {
		valid := validCustomDevelopmentInput()
		// item 0 is priced, item 1 has nil QuotedLineTotal (included)
		require.NotNil(t, valid.DevelopmentGroups[0].Items[0].QuotedLineTotal)
		require.Nil(t, valid.DevelopmentGroups[0].Items[1].QuotedLineTotal)
		require.NoError(t, valid.Validate())
	})

	t.Run("valid input without monthly items and notes passes", func(t *testing.T) {
		valid := validCustomDevelopmentInput()
		valid.MonthlyItems = nil
		valid.MonthlyNotes = nil
		require.NoError(t, valid.Validate())
	})

	t.Run("valid input without rights and additional limitations passes", func(t *testing.T) {
		valid := validCustomDevelopmentInput()
		valid.RightsStatements = nil
		valid.AdditionalLimitations = nil
		require.NoError(t, valid.Validate())
	})

	t.Run("fixed_days table mode with nil display rates passes", func(t *testing.T) {
		valid := validCustomDevelopmentInput()
		valid.DevelopmentTableMode = CustomDevelopmentFixedDays
		valid.DevelopmentGroups[0].Items[0].DisplayRate = nil
		valid.DevelopmentGroups[0].Items[1].DisplayRate = nil
		require.NoError(t, valid.Validate())
	})

	t.Run("zero discount with show_discount_row false passes", func(t *testing.T) {
		valid := validCustomDevelopmentInput()
		valid.DiscountAmount = 0
		valid.ShowDiscountRow = false
		require.NoError(t, valid.Validate())
	})
}

func TestCustomDevelopmentInput_Validate_DetailedFieldErrors(t *testing.T) {
	t.Run("system_name validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.SystemName = ""
		require.ErrorContains(t, v.Validate(), "system_name")

		v = validCustomDevelopmentInput()
		v.SystemName = "   "
		require.ErrorContains(t, v.Validate(), "system_name")

		v = validCustomDevelopmentInput()
		v.SystemName = strings.Repeat("A", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "system_name")
	})

	t.Run("quotation_date validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.QuotationDate = time.Time{}
		require.ErrorContains(t, v.Validate(), "quotation_date")
	})

	t.Run("tax_treatment validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.TaxTreatment = "invalid_tax"
		require.ErrorContains(t, v.Validate(), "tax_treatment")
	})

	t.Run("development_table_mode fixed_days permits display_rate", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.DevelopmentTableMode = CustomDevelopmentFixedDays
		v.DevelopmentGroups[0].Items[0].DisplayRate = ptrInt64(1_000_000)
		require.NoError(t, v.Validate(), "fixed_days mode must permit display_rate")
	})

	t.Run("development_groups boundary validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.DevelopmentGroups = nil
		require.ErrorContains(t, v.Validate(), "development_groups")

		v = validCustomDevelopmentInput()
		groups := make([]CustomDevelopmentCostGroupInput, MaxCustomDevelopmentGroups+1)
		for i := range groups {
			groups[i] = CustomDevelopmentCostGroupInput{
				Title: "Group",
				Items: []CustomDevelopmentCostItemInput{
					{Name: "Item", Headcount: 1, PeriodLabel: "1", QuotedLineTotal: ptrInt64(1000)},
				},
			}
		}
		v.DevelopmentGroups = groups
		require.ErrorContains(t, v.Validate(), "development_groups")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Title = ""
		require.ErrorContains(t, v.Validate(), "title")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Title = strings.Repeat("T", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "title")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items = nil
		require.ErrorContains(t, v.Validate(), "items")

		v = validCustomDevelopmentInput()
		items := make([]CustomDevelopmentCostItemInput, MaxCustomDevelopmentItemsPerGroup+1)
		for i := range items {
			items[i] = CustomDevelopmentCostItemInput{Name: "Item", Headcount: 1, PeriodLabel: "1", QuotedLineTotal: ptrInt64(1000)}
		}
		v.DevelopmentGroups[0].Items = items
		require.ErrorContains(t, v.Validate(), "items")
	})

	t.Run("development cost item field validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].Name = ""
		require.ErrorContains(t, v.Validate(), "name")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].Name = strings.Repeat("N", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "name")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].Headcount = 0
		require.ErrorContains(t, v.Validate(), "headcount")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].Headcount = MaxCustomDevelopmentHeadcount + 1
		require.ErrorContains(t, v.Validate(), "headcount")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].PeriodLabel = ""
		require.ErrorContains(t, v.Validate(), "period_label")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].PeriodLabel = strings.Repeat("P", MaxCustomDevelopmentPeriodRunes+1)
		require.ErrorContains(t, v.Validate(), "period_label")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].DisplayRate = ptrInt64(0)
		require.ErrorContains(t, v.Validate(), "display_rate")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].DisplayRate = ptrInt64(MaxCustomDevelopmentMoney + 1)
		require.ErrorContains(t, v.Validate(), "display_rate")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].QuotedLineTotal = ptrInt64(0)
		require.ErrorContains(t, v.Validate(), "quoted_line_total")

		v = validCustomDevelopmentInput()
		v.DevelopmentGroups[0].Items[0].QuotedLineTotal = ptrInt64(MaxCustomDevelopmentMoney + 1)
		require.ErrorContains(t, v.Validate(), "quoted_line_total")
	})

	t.Run("discount validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.DiscountAmount = MaxCustomDevelopmentMoney + 1
		require.ErrorContains(t, v.Validate(), "discount_amount")

		v = validCustomDevelopmentInput()
		v.DiscountLabel = strings.Repeat("D", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "discount_label")

		v = validCustomDevelopmentInput()
		v.DiscountAmount = 20_000_000 // exceeds subtotal of 15_000_000
		require.ErrorContains(t, v.Validate(), "discount_amount")
	})

	t.Run("monthly items validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		items := make([]CustomDevelopmentMonthlyItemInput, MaxCustomDevelopmentMonthlyItems+1)
		for i := range items {
			items[i] = CustomDevelopmentMonthlyItemInput{Name: "Item", UnitRate: 1000, Quantity: 1}
		}
		v.MonthlyItems = items
		require.ErrorContains(t, v.Validate(), "monthly_items")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].Name = ""
		require.ErrorContains(t, v.Validate(), "name")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].Name = strings.Repeat("M", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "name")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].Specification = strings.Repeat("S", MaxCustomDevelopmentLongTextRunes+1)
		require.ErrorContains(t, v.Validate(), "specification")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].UnitRate = 0
		require.ErrorContains(t, v.Validate(), "unit_rate")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].UnitRate = MaxCustomDevelopmentMoney + 1
		require.ErrorContains(t, v.Validate(), "unit_rate")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].Quantity = 0
		require.ErrorContains(t, v.Validate(), "quantity")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].Quantity = MaxCustomDevelopmentMonthlyQuantity + 1
		require.ErrorContains(t, v.Validate(), "quantity")

		v = validCustomDevelopmentInput()
		v.MonthlyItems[0].UnitRate = MaxCustomDevelopmentMoney
		v.MonthlyItems[0].Quantity = 2
		require.ErrorContains(t, v.Validate(), "line calculation")
	})

	t.Run("monthly notes boundary and text validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		notes := make([]string, MaxCustomDevelopmentMonthlyNotes+1)
		for i := range notes {
			notes[i] = "Note"
		}
		v.MonthlyNotes = notes
		require.ErrorContains(t, v.Validate(), "monthly_notes")

		v = validCustomDevelopmentInput()
		v.MonthlyNotes = []string{""}
		require.ErrorContains(t, v.Validate(), "monthly_notes")

		v = validCustomDevelopmentInput()
		v.MonthlyNotes = []string{strings.Repeat("N", MaxCustomDevelopmentLongTextRunes+1)}
		require.ErrorContains(t, v.Validate(), "monthly_notes")
	})

	t.Run("module groups boundary and empty feature validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.ModuleGroups = nil
		require.ErrorContains(t, v.Validate(), "module_groups")

		v = validCustomDevelopmentInput()
		mg := make([]CustomDevelopmentModuleGroupInput, MaxCustomDevelopmentModuleGroups+1)
		for i := range mg {
			mg[i] = CustomDevelopmentModuleGroupInput{Name: "G", Features: []string{"F"}}
		}
		v.ModuleGroups = mg
		require.ErrorContains(t, v.Validate(), "module_groups")

		v = validCustomDevelopmentInput()
		v.ModuleGroups[0].Name = ""
		require.ErrorContains(t, v.Validate(), "name")

		v = validCustomDevelopmentInput()
		v.ModuleGroups[0].Name = strings.Repeat("G", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "name")

		v = validCustomDevelopmentInput()
		v.ModuleGroups = []CustomDevelopmentModuleGroupInput{
			{Name: "Modul A", Features: []string{"F1"}},
			{Name: "modul a", Features: []string{"F2"}},
		}
		require.ErrorContains(t, v.Validate(), "duplicate module name")

		v = validCustomDevelopmentInput()
		v.ModuleGroups[0].Features = nil
		require.ErrorContains(t, v.Validate(), "features")

		v = validCustomDevelopmentInput()
		feats := make([]string, MaxCustomDevelopmentFeaturesPerGroup+1)
		for i := range feats {
			feats[i] = "F"
		}
		v.ModuleGroups[0].Features = feats
		require.ErrorContains(t, v.Validate(), "features")

		v = validCustomDevelopmentInput()
		v.ModuleGroups[0].Features[0] = ""
		require.ErrorContains(t, v.Validate(), "feature string cannot be empty")

		v = validCustomDevelopmentInput()
		v.ModuleGroups[0].Features[0] = "   "
		require.ErrorContains(t, v.Validate(), "feature string cannot be empty")

		v = validCustomDevelopmentInput()
		v.ModuleGroups[0].Features[0] = strings.Repeat("F", MaxCustomDevelopmentLongTextRunes+1)
		require.ErrorContains(t, v.Validate(), "features")
	})

	t.Run("timeline rows boundary and text validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.TimelineRows = nil
		require.ErrorContains(t, v.Validate(), "timeline_rows")

		v = validCustomDevelopmentInput()
		tr := make([]CustomDevelopmentTimelineRowInput, MaxCustomDevelopmentTimelineRows+1)
		for i := range tr {
			tr[i] = CustomDevelopmentTimelineRowInput{Description: "D", Timeline: "T"}
		}
		v.TimelineRows = tr
		require.ErrorContains(t, v.Validate(), "timeline_rows")

		v = validCustomDevelopmentInput()
		v.TimelineRows[0].Description = ""
		require.ErrorContains(t, v.Validate(), "description")

		v = validCustomDevelopmentInput()
		v.TimelineRows[0].Description = strings.Repeat("D", MaxCustomDevelopmentLongTextRunes+1)
		require.ErrorContains(t, v.Validate(), "description")

		v = validCustomDevelopmentInput()
		v.TimelineRows[0].Timeline = ""
		require.ErrorContains(t, v.Validate(), "timeline")

		v = validCustomDevelopmentInput()
		v.TimelineRows[0].Timeline = strings.Repeat("T", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "timeline")
	})

	t.Run("payment schedule boundary and field validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.PaymentSchedule = nil
		require.ErrorContains(t, v.Validate(), "payment_schedule")

		v = validCustomDevelopmentInput()
		ps := make([]CustomDevelopmentPaymentRowInput, MaxCustomDevelopmentPaymentRows+1)
		for i := range ps {
			ps[i] = CustomDevelopmentPaymentRowInput{
				WorkPeriod: "P", Terms: "T", InvoiceDateLabel: "I", DueDateLabel: "D",
				Component: CustomDevelopmentPaymentProject, Nominal: 1000,
			}
		}
		v.PaymentSchedule = ps
		require.ErrorContains(t, v.Validate(), "payment_schedule")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].WorkPeriod = ""
		require.ErrorContains(t, v.Validate(), "work_period")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].WorkPeriod = strings.Repeat("W", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "work_period")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].Terms = ""
		require.ErrorContains(t, v.Validate(), "terms")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].Terms = strings.Repeat("T", MaxCustomDevelopmentShortTextRunes+1)
		require.ErrorContains(t, v.Validate(), "terms")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].InvoiceDateLabel = ""
		require.ErrorContains(t, v.Validate(), "invoice_date_label")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].DueDateLabel = ""
		require.ErrorContains(t, v.Validate(), "due_date_label")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].Component = "invalid_component"
		require.ErrorContains(t, v.Validate(), "component")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].Nominal = 0
		require.ErrorContains(t, v.Validate(), "nominal")

		v = validCustomDevelopmentInput()
		v.PaymentSchedule[0].Nominal = MaxCustomDevelopmentMoney + 1
		require.ErrorContains(t, v.Validate(), "nominal")
	})

	t.Run("rights statements validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		rs := make([]string, MaxCustomDevelopmentRightsStatements+1)
		for i := range rs {
			rs[i] = "Right"
		}
		v.RightsStatements = rs
		require.ErrorContains(t, v.Validate(), "rights_statements")

		v = validCustomDevelopmentInput()
		v.RightsStatements = []string{""}
		require.ErrorContains(t, v.Validate(), "rights_statements")

		v = validCustomDevelopmentInput()
		v.RightsStatements = []string{strings.Repeat("R", MaxCustomDevelopmentLongTextRunes+1)}
		require.ErrorContains(t, v.Validate(), "rights_statements")
	})

	t.Run("limitation codes validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		v.LimitationCodes = nil
		require.ErrorContains(t, v.Validate(), "limitation_codes")

		v = validCustomDevelopmentInput()
		v.LimitationCodes = []string{"out_of_pocket", "overtime", "third_party", "scope_change", "extra"}
		require.ErrorContains(t, v.Validate(), "limitation_codes")

		v = validCustomDevelopmentInput()
		v.LimitationCodes = []string{"unsupported_clause"}
		require.ErrorContains(t, v.Validate(), "unsupported limitation")
	})

	t.Run("additional limitations boundary and duplicate validation", func(t *testing.T) {
		v := validCustomDevelopmentInput()
		al := make([]string, MaxCustomDevelopmentAdditionalLimitations+1)
		for i := range al {
			al[i] = "Limitation"
		}
		v.AdditionalLimitations = al
		require.ErrorContains(t, v.Validate(), "additional_limitations")

		v = validCustomDevelopmentInput()
		v.AdditionalLimitations = []string{""}
		require.ErrorContains(t, v.Validate(), "additional_limitations")

		v = validCustomDevelopmentInput()
		v.AdditionalLimitations = []string{strings.Repeat("L", MaxCustomDevelopmentLongTextRunes+1)}
		require.ErrorContains(t, v.Validate(), "additional_limitations")

		v = validCustomDevelopmentInput()
		v.AdditionalLimitations = []string{
			"Tidak termasuk hosting server.",
			"  tidak termasuk hosting server.  ",
		}
		require.ErrorContains(t, v.Validate(), "duplicate additional limitation")
	})
}

func TestCustomDevelopmentInput_UnmarshalJSON(t *testing.T) {
	t.Run("valid unmarshal with YYYY-MM-DD", func(t *testing.T) {
		raw := `{
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
			"discount_amount": 1000000,
			"show_discount_row": true,
			"module_groups": [{"name": "M1", "features": ["F1"]}],
			"timeline_rows": [{"description": "T1", "timeline": "1 bln"}],
			"payment_schedule": [{"work_period": "P1", "terms": "DP", "invoice_date_label": "Inv", "due_date_label": "Due", "component": "development", "nominal": 14000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var in CustomDevelopmentInput
		err := json.Unmarshal([]byte(raw), &in)
		require.NoError(t, err)
		assert.Equal(t, "Sistem Kopi Studio24", in.SystemName)
		assert.Equal(t, time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC), in.QuotationDate.UTC())
		assert.Len(t, in.DevelopmentGroups, 1)
		assert.Len(t, in.DevelopmentGroups[0].Items, 2)
		assert.Nil(t, in.DevelopmentGroups[0].Items[1].QuotedLineTotal)
		require.NoError(t, in.Validate())
	})

	t.Run("rejects date not in YYYY-MM-DD or RFC3339", func(t *testing.T) {
		raw := `{
			"system_name": "Sistem",
			"quotation_date": "15-07-2026",
			"development_table_mode": "monthly_period",
			"tax_treatment": "exclusive_ppn"
		}`
		var in CustomDevelopmentInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date")
	})

	t.Run("rejects missing or null quotation_date", func(t *testing.T) {
		raw := `{
			"system_name": "Sistem",
			"quotation_date": null,
			"development_table_mode": "monthly_period",
			"tax_treatment": "exclusive_ppn"
		}`
		var in CustomDevelopmentInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date")
	})

	t.Run("rejects unknown top-level field", func(t *testing.T) {
		raw := `{
			"system_name": "Sistem",
			"quotation_date": "2026-07-15",
			"development_table_mode": "monthly_period",
			"tax_treatment": "exclusive_ppn",
			"unknown_field": "illegal"
		}`
		var in CustomDevelopmentInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects unknown nested field in development_groups items", func(t *testing.T) {
		raw := `{
			"system_name": "Sistem",
			"quotation_date": "2026-07-15",
			"development_table_mode": "monthly_period",
			"tax_treatment": "exclusive_ppn",
			"development_groups": [
				{
					"title": "Fase 1",
					"items": [
						{"name": "Dev", "headcount": 1, "period_label": "1", "quoted_line_total": 15000000, "extra_prop": 123}
					]
				}
			]
		}`
		var in CustomDevelopmentInput
		err := json.Unmarshal([]byte(raw), &in)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}

func TestCustomDevelopment_TableModeInvariance(t *testing.T) {
	// 1. Buat input valid dengan monthly_period dan display_rate terisi
	input := validCustomDevelopmentInput()
	require.NotEmpty(t, input.DevelopmentGroups)
	require.NotEmpty(t, input.DevelopmentGroups[0].Items)
	require.NotNil(t, input.DevelopmentGroups[0].Items[0].DisplayRate)
	initialRate := *input.DevelopmentGroups[0].Items[0].DisplayRate
	initialTotal := *input.DevelopmentGroups[0].Items[0].QuotedLineTotal
	require.NoError(t, input.Validate())

	// 2. Ubah hanya development_table_mode menjadi fixed_days
	input.DevelopmentTableMode = CustomDevelopmentFixedDays

	// 3. Pastikan rate dan total tetap sama
	require.NotNil(t, input.DevelopmentGroups[0].Items[0].DisplayRate)
	assert.Equal(t, initialRate, *input.DevelopmentGroups[0].Items[0].DisplayRate)
	require.NotNil(t, input.DevelopmentGroups[0].Items[0].QuotedLineTotal)
	assert.Equal(t, initialTotal, *input.DevelopmentGroups[0].Items[0].QuotedLineTotal)

	// 4. Pastikan validasi berhasil (backend tidak lagi menolak display_rate pada fixed_days)
	assert.NoError(t, input.Validate())
}
