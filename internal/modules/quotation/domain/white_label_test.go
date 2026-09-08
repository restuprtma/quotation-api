package domain_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validWhiteLabelInput() domain.WhiteLabelInput {
	return domain.WhiteLabelInput{
		ProductName:   "POS Mini Market",
		QuotationDate: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
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
		PaymentSchedule: []domain.WhiteLabelPaymentRowInput{
			{WorkPeriod: "Pembelian White Label", Terms: "Pelunasan", InvoiceDateLabel: "Saat kontrak ditandatangani", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentLicense, Nominal: 35_000_000},
			{WorkPeriod: "Maintenance 1", Terms: "Bulanan", InvoiceDateLabel: "Bulan 1", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 2", Terms: "Bulanan", InvoiceDateLabel: "Bulan 2", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
			{WorkPeriod: "Maintenance 3", Terms: "Bulanan", InvoiceDateLabel: "Bulan 3", DueDateLabel: "H+14", Component: domain.WhiteLabelPaymentMaintenanceHosting, Nominal: 8_435_000},
		},
		LimitationCodes: []string{"out_of_pocket", "overtime", "third_party", "scope_change"},
	}
}

func TestWhiteLabelInput_Validate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(in *domain.WhiteLabelInput)
		expectErr bool
		errSubstr string
	}{
		{
			name:      "valid input succeeds",
			mutate:    nil,
			expectErr: false,
		},
		{
			name: "product name empty",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ProductName = ""
			},
			expectErr: true,
			errSubstr: "product_name",
		},
		{
			name: "product name whitespace only",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ProductName = "   \t\n "
			},
			expectErr: true,
			errSubstr: "product_name",
		},
		{
			name: "product name over max runes",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ProductName = strings.Repeat("a", int(domain.MaxWhiteLabelNameRunes)+1)
			},
			expectErr: true,
			errSubstr: "product_name",
		},
		{
			name: "quotation date zero",
			mutate: func(in *domain.WhiteLabelInput) {
				in.QuotationDate = time.Time{}
			},
			expectErr: true,
			errSubstr: "quotation_date",
		},
		{
			name: "enum pajak asing",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TaxTreatment = "vat_unknown"
			},
			expectErr: true,
			errSubstr: "tax_treatment",
		},
		{
			name: "valid tax not_stated",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TaxTreatment = domain.WhiteLabelTaxNotStated
			},
			expectErr: false,
		},
		{
			name: "valid tax inclusive",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TaxTreatment = domain.WhiteLabelTaxInclusive
			},
			expectErr: false,
		},
		{
			name: "license kosong",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems = nil
			},
			expectErr: true,
			errSubstr: "license_items",
		},
		{
			name: "license over max items",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems = make([]domain.WhiteLabelLicenseItemInput, domain.MaxWhiteLabelLicenseItems+1)
				for i := range in.LicenseItems {
					in.LicenseItems[i] = domain.WhiteLabelLicenseItemInput{Name: fmt.Sprintf("Item %d", i), UnitRate: 1000, Quantity: 1}
				}
			},
			expectErr: true,
			errSubstr: "license_items",
		},
		{
			name: "license item name empty",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].Name = ""
			},
			expectErr: true,
			errSubstr: "license_items[0].name",
		},
		{
			name: "license item name whitespace only",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].Name = "   "
			},
			expectErr: true,
			errSubstr: "license_items[0].name",
		},
		{
			name: "license item name over max runes",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].Name = strings.Repeat("b", int(domain.MaxWhiteLabelNameRunes)+1)
			},
			expectErr: true,
			errSubstr: "license_items[0].name",
		},
		{
			name: "license rate nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].UnitRate = 0
			},
			expectErr: true,
			errSubstr: "license_items[0].unit_rate",
		},
		{
			name: "license rate negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].UnitRate = -1
			},
			expectErr: true,
			errSubstr: "license_items[0].unit_rate",
		},
		{
			name: "license rate di atas max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].UnitRate = domain.MaxWhiteLabelMoney + 1
			},
			expectErr: true,
			errSubstr: "license_items[0].unit_rate",
		},
		{
			name: "license quantity nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].Quantity = 0
			},
			expectErr: true,
			errSubstr: "license_items[0].quantity",
		},
		{
			name: "license quantity negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LicenseItems[0].Quantity = -1
			},
			expectErr: true,
			errSubstr: "license_items[0].quantity",
		},
		{
			name: "maintenance over max items",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems = make([]domain.WhiteLabelMaintenanceItemInput, domain.MaxWhiteLabelComponentItems+1)
				for i := range in.MaintenanceItems {
					in.MaintenanceItems[i] = domain.WhiteLabelMaintenanceItemInput{Name: fmt.Sprintf("Maint %d", i), MonthlyRate: 4000, FTEQuarters: 1, PeriodMonths: 1}
				}
			},
			expectErr: true,
			errSubstr: "maintenance_items",
		},
		{
			name: "maintenance item name empty",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].Name = ""
			},
			expectErr: true,
			errSubstr: "maintenance_items[0].name",
		},
		{
			name: "maintenance rate nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].MonthlyRate = 0
			},
			expectErr: true,
			errSubstr: "maintenance_items[0].monthly_rate",
		},
		{
			name: "maintenance rate negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].MonthlyRate = -1
			},
			expectErr: true,
			errSubstr: "maintenance_items[0].monthly_rate",
		},
		{
			name: "maintenance rate di atas max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].MonthlyRate = domain.MaxWhiteLabelMoney + 1
			},
			expectErr: true,
			errSubstr: "maintenance_items[0].monthly_rate",
		},
		{
			name: "FTE bukan 1..4 (nol)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].FTEQuarters = 0
			},
			expectErr: true,
			errSubstr: "fte_quarters",
		},
		{
			name: "FTE bukan 1..4 (5)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].FTEQuarters = 5
			},
			expectErr: true,
			errSubstr: "fte_quarters",
		},
		{
			name: "period nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].PeriodMonths = 0
			},
			expectErr: true,
			errSubstr: "period_months",
		},
		{
			name: "period negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0].PeriodMonths = -1
			},
			expectErr: true,
			errSubstr: "period_months",
		},
		{
			name: "maintenance multiplication tidak habis dibagi 4",
			mutate: func(in *domain.WhiteLabelInput) {
				in.MaintenanceItems[0] = domain.WhiteLabelMaintenanceItemInput{
					Name:         "Odd dev",
					MonthlyRate:  10_000_001,
					FTEQuarters:  1,
					PeriodMonths: 1,
				}
			},
			expectErr: true,
			errSubstr: "divisible by 4",
		},
		{
			name: "hosting over max items",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems = make([]domain.WhiteLabelHostingItemInput, domain.MaxWhiteLabelComponentItems+1)
				for i := range in.HostingItems {
					in.HostingItems[i] = domain.WhiteLabelHostingItemInput{Name: fmt.Sprintf("Host %d", i), MonthlyRate: 1000, Quantity: 1, PeriodMonths: 1}
				}
			},
			expectErr: true,
			errSubstr: "hosting_items",
		},
		{
			name: "hosting item name empty",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].Name = ""
			},
			expectErr: true,
			errSubstr: "hosting_items[0].name",
		},
		{
			name: "hosting rate nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].MonthlyRate = 0
			},
			expectErr: true,
			errSubstr: "hosting_items[0].monthly_rate",
		},
		{
			name: "hosting rate negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].MonthlyRate = -1
			},
			expectErr: true,
			errSubstr: "hosting_items[0].monthly_rate",
		},
		{
			name: "hosting rate di atas max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].MonthlyRate = domain.MaxWhiteLabelMoney + 1
			},
			expectErr: true,
			errSubstr: "hosting_items[0].monthly_rate",
		},
		{
			name: "hosting quantity nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].Quantity = 0
			},
			expectErr: true,
			errSubstr: "hosting_items[0].quantity",
		},
		{
			name: "hosting quantity negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].Quantity = -1
			},
			expectErr: true,
			errSubstr: "hosting_items[0].quantity",
		},
		{
			name: "hosting period nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].PeriodMonths = 0
			},
			expectErr: true,
			errSubstr: "hosting_items[0].period_months",
		},
		{
			name: "hosting period negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.HostingItems[0].PeriodMonths = -1
			},
			expectErr: true,
			errSubstr: "hosting_items[0].period_months",
		},
		{
			name: "module groups over max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups = make([]domain.WhiteLabelModuleGroupInput, 31)
				for i := range in.ModuleGroups {
					in.ModuleGroups[i] = domain.WhiteLabelModuleGroupInput{Name: fmt.Sprintf("Module %d", i)}
				}
			},
			expectErr: true,
			errSubstr: "module_groups",
		},
		{
			name: "module group name empty",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups[0].Name = ""
			},
			expectErr: true,
			errSubstr: "module_groups[0].name",
		},
		{
			name: "duplicate module name",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups = append(in.ModuleGroups, domain.WhiteLabelModuleGroupInput{
					Name:     "inventory",
					Features: []string{"Feature A"},
				})
			},
			expectErr: true,
			errSubstr: "duplicate module name",
		},
		{
			name: "features empty slice is valid",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups[0].Features = []string{}
			},
			expectErr: false,
		},
		{
			name: "features per group over max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups[0].Features = make([]string, 101)
				for i := range in.ModuleGroups[0].Features {
					in.ModuleGroups[0].Features[i] = fmt.Sprintf("Feature %d", i)
				}
			},
			expectErr: true,
			errSubstr: "features",
		},
		{
			name: "feature string kosong saat item feature memang ada",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups[0].Features = []string{"Stok masuk", "   "}
			},
			expectErr: true,
			errSubstr: "feature",
		},
		{
			name: "feature string over max runes",
			mutate: func(in *domain.WhiteLabelInput) {
				in.ModuleGroups[0].Features = []string{strings.Repeat("f", int(domain.MaxWhiteLabelNameRunes)+1)}
			},
			expectErr: true,
			errSubstr: "feature",
		},
		{
			name: "timeline rows over max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TimelineRows = make([]domain.WhiteLabelTimelineRowInput, 101)
				for i := range in.TimelineRows {
					in.TimelineRows[i] = domain.WhiteLabelTimelineRowInput{Description: fmt.Sprintf("Task %d", i), Timeline: "1 minggu"}
				}
			},
			expectErr: true,
			errSubstr: "timeline_rows",
		},
		{
			name: "timeline hanya satu kolom (description empty)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TimelineRows = []domain.WhiteLabelTimelineRowInput{
					{Description: "", Timeline: "2 minggu"},
				}
			},
			expectErr: true,
			errSubstr: "description",
		},
		{
			name: "timeline hanya satu kolom (timeline empty)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TimelineRows = []domain.WhiteLabelTimelineRowInput{
					{Description: "Setup server", Timeline: ""},
				}
			},
			expectErr: true,
			errSubstr: "timeline",
		},
		{
			name: "valid timeline rows",
			mutate: func(in *domain.WhiteLabelInput) {
				in.TimelineRows = []domain.WhiteLabelTimelineRowInput{
					{Description: "Persiapan Lingkungan", Timeline: "Minggu 1"},
					{Description: "UAT & Serah Terima", Timeline: "Minggu 2"},
				}
			},
			expectErr: false,
		},
		{
			name: "payment schedule kosong",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule = nil
			},
			expectErr: true,
			errSubstr: "payment_schedule",
		},
		{
			name: "payment schedule over max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule = make([]domain.WhiteLabelPaymentRowInput, domain.MaxWhiteLabelPaymentRows+1)
				for i := range in.PaymentSchedule {
					in.PaymentSchedule[i] = domain.WhiteLabelPaymentRowInput{
						WorkPeriod:       "Period",
						Terms:            "Terms",
						InvoiceDateLabel: "Inv",
						DueDateLabel:     "Due",
						Component:        domain.WhiteLabelPaymentLicense,
						Nominal:          1000,
					}
				}
			},
			expectErr: true,
			errSubstr: "payment_schedule",
		},
		{
			name: "payment string kosong (work period)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].WorkPeriod = ""
			},
			expectErr: true,
			errSubstr: "work_period",
		},
		{
			name: "payment string kosong (terms)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].Terms = ""
			},
			expectErr: true,
			errSubstr: "terms",
		},
		{
			name: "payment string kosong (invoice date label)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].InvoiceDateLabel = ""
			},
			expectErr: true,
			errSubstr: "invoice_date_label",
		},
		{
			name: "payment string kosong (due date label)",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].DueDateLabel = ""
			},
			expectErr: true,
			errSubstr: "due_date_label",
		},
		{
			name: "payment component unknown",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].Component = "arbitrary"
			},
			expectErr: true,
			errSubstr: "component",
		},
		{
			name: "nominal nol",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].Nominal = 0
			},
			expectErr: true,
			errSubstr: "nominal",
		},
		{
			name: "nominal negatif",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].Nominal = -1
			},
			expectErr: true,
			errSubstr: "nominal",
		},
		{
			name: "nominal di atas max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.PaymentSchedule[0].Nominal = domain.MaxWhiteLabelMoney + 1
			},
			expectErr: true,
			errSubstr: "nominal",
		},
		{
			name: "rights over max items",
			mutate: func(in *domain.WhiteLabelInput) {
				in.RightsStatements = make([]string, 21)
				for i := range in.RightsStatements {
					in.RightsStatements[i] = fmt.Sprintf("Right %d", i)
				}
			},
			expectErr: true,
			errSubstr: "rights_statements",
		},
		{
			name: "rights string kosong",
			mutate: func(in *domain.WhiteLabelInput) {
				in.RightsStatements = []string{""}
			},
			expectErr: true,
			errSubstr: "rights_statements[0]",
		},
		{
			name: "rights string whitespace only",
			mutate: func(in *domain.WhiteLabelInput) {
				in.RightsStatements = []string{"   "}
			},
			expectErr: true,
			errSubstr: "rights_statements[0]",
		},
		{
			name: "rights string di atas 1000 rune",
			mutate: func(in *domain.WhiteLabelInput) {
				in.RightsStatements = []string{strings.Repeat("r", int(domain.MaxWhiteLabelTextRunes)+1)}
			},
			expectErr: true,
			errSubstr: "rights_statements[0]",
		},
		{
			name: "valid rights statements",
			mutate: func(in *domain.WhiteLabelInput) {
				in.RightsStatements = []string{
					"Hak cipta software jadi sepenuhnya milik Venturo.",
					"Klien mendapatkan hak pakai lisensi secara non-eksklusif.",
				}
			},
			expectErr: false,
		},
		{
			name: "limitation codes kosong",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LimitationCodes = nil
			},
			expectErr: true,
			errSubstr: "limitation_codes",
		},
		{
			name: "limitation codes over max",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LimitationCodes = []string{"out_of_pocket", "overtime", "third_party", "scope_change", "extra1", "extra2", "extra3", "extra4", "extra5", "extra6", "extra7"}
			},
			expectErr: true,
			errSubstr: "limitation_codes",
		},
		{
			name: "duplicate limitation",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LimitationCodes = []string{"out_of_pocket", "out_of_pocket"}
			},
			expectErr: true,
			errSubstr: "duplicate",
		},
		{
			name: "unknown limitation",
			mutate: func(in *domain.WhiteLabelInput) {
				in.LimitationCodes = []string{"out_of_pocket", "not_allowed_code"}
			},
			expectErr: true,
			errSubstr: "not_allowed_code",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validWhiteLabelInput()
			if tc.mutate != nil {
				tc.mutate(&input)
			}
			err := input.Validate()
			if tc.expectErr {
				require.Error(t, err)
				if tc.errSubstr != "" {
					assert.Contains(t, err.Error(), tc.errSubstr)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWhiteLabelInput_UnmarshalJSONStrict(t *testing.T) {
	t.Run("valid payload round trip unmarshals successfully", func(t *testing.T) {
		input := validWhiteLabelInput()
		b, err := json.Marshal(input)
		require.NoError(t, err)

		var decoded domain.WhiteLabelInput
		require.NoError(t, json.Unmarshal(b, &decoded))
		require.NoError(t, decoded.Validate())
		assert.Equal(t, input.ProductName, decoded.ProductName)
		assert.Equal(t, input.QuotationDate.UTC(), decoded.QuotationDate.UTC())
		assert.Equal(t, input.TaxTreatment, decoded.TaxTreatment)
		assert.Len(t, decoded.LicenseItems, 1)
		assert.Len(t, decoded.MaintenanceItems, 1)
		assert.Len(t, decoded.HostingItems, 1)
		assert.Len(t, decoded.ModuleGroups, 1)
		assert.Len(t, decoded.PaymentSchedule, 4)
		assert.Equal(t, input.LimitationCodes, decoded.LimitationCodes)
	})

	t.Run("accepts calendar date format YYYY-MM-DD", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		require.NoError(t, json.Unmarshal([]byte(raw), &decoded))
		assert.Equal(t, time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC), decoded.QuotationDate.UTC())
		require.NoError(t, decoded.Validate())
	})

	t.Run("rejects invalid date format", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "not-a-date",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		require.Error(t, json.Unmarshal([]byte(raw), &decoded))
	})

	t.Run("rejects missing quotation date", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date")
	})

	t.Run("rejects top-level unknown field", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"],
			"unexpected_field": "hacked"
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects nested unknown field in license_items", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1, "extra_license_field": true}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects nested unknown field in maintenance_items", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"maintenance_items": [{"name": "Dev", "monthly_rate": 16000000, "fte_quarters": 2, "period_months": 3, "extra_maint": "fail"}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects nested unknown field in hosting_items", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"hosting_items": [{"name": "Host", "monthly_rate": 350000, "quantity": 1, "period_months": 3, "extra_host": 123}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects nested unknown field in module_groups", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"module_groups": [{"name": "Inventory", "features": ["Stok"], "extra_group": "bad"}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects nested unknown field in timeline_rows", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"timeline_rows": [{"description": "Setup", "timeline": "1 minggu", "unknown_col": "err"}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects nested unknown field in payment_schedule", func(t *testing.T) {
		raw := `{
			"product_name": "POS Mini Market",
			"quotation_date": "2026-09-07",
			"tax_treatment": "exclusive_ppn",
			"license_items": [{"name": "POS Mini Market", "unit_rate": 35000000, "quantity": 1}],
			"payment_schedule": [{"work_period": "P1", "terms": "Pelunasan", "invoice_date_label": "Saat tanda tangan", "due_date_label": "H+14", "component": "license", "nominal": 35000000, "bad_fee": 100}],
			"limitation_codes": ["out_of_pocket"]
		}`
		var decoded domain.WhiteLabelInput
		err := json.Unmarshal([]byte(raw), &decoded)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})
}
