package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validStandaloneRetainerInput() domain.StandaloneMaintenanceInput {
	return domain.StandaloneMaintenanceInput{
		SystemName:    "Aplikasi Nex Health",
		QuotationDate: time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC),
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
		PaymentSchedule: []domain.MaintenancePaymentRowInput{{
			WorkPeriod:       "13 Juli - 13 Agustus 2026",
			Terms:            "Maintenance",
			InvoiceDateLabel: "13 Agustus 2026",
			DueDateLabel:     "27 Agustus 2026",
			Nominal:          23_085_000,
		}},
	}
}

func validStandaloneHourlyInput() domain.StandaloneMaintenanceInput {
	return domain.StandaloneMaintenanceInput{
		SystemName:    "Portal Berita Liputan6",
		QuotationDate: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC),
		Mode:          domain.StandaloneMaintenanceHourlyTask,
		TaxTreatment:  domain.StandaloneTaxNotStated,
		HourlyCostItems: []domain.MaintenanceHourlyCostItemInput{
			{Name: "Senior Mobile Programmer", DisplayMonthlyRate: 16_050_000, Headcount: 1, HoursHalfUnits: 16, QuotedLineTotal: 730_000},
			{Name: "Senior QA", DisplayMonthlyRate: 12_500_000, Headcount: 1, HoursHalfUnits: 4, QuotedLineTotal: 140_000},
		},
		HourlyTasks: []domain.MaintenanceHourlyTaskInput{
			{Description: "Perbaikan crash login", Platform: "Android", HoursHalfUnits: 16},
			{Description: "Regression testing", Platform: "QA Test", HoursHalfUnits: 4},
		},
		ShowDiscountRow: false,
		DiscountAmount:  0,
		PaymentSchedule: []domain.MaintenancePaymentRowInput{{
			WorkPeriod:       "Support Juli 2026",
			Terms:            "Pelunasan",
			InvoiceDateLabel: "31 Juli 2026",
			DueDateLabel:     "14 Agustus 2026",
			Nominal:          870_000,
		}},
		RightsStatements: []string{"Source code dan modul merupakan hak milik bersama."},
	}
}

func TestStandaloneMaintenanceInput_Validate_ValidCases(t *testing.T) {
	t.Run("valid retainer input passes", func(t *testing.T) {
		input := validStandaloneRetainerInput()
		require.NoError(t, input.Validate())
	})

	t.Run("valid hourly input passes", func(t *testing.T) {
		input := validStandaloneHourlyInput()
		require.NoError(t, input.Validate())
	})

	t.Run("empty and nil inactive arrays are valid", func(t *testing.T) {
		retainer := validStandaloneRetainerInput()
		retainer.HourlyCostItems = []domain.MaintenanceHourlyCostItemInput{}
		retainer.HourlyTasks = []domain.MaintenanceHourlyTaskInput{}
		retainer.RightsStatements = []string{}
		require.NoError(t, retainer.Validate())

		hourly := validStandaloneHourlyInput()
		hourly.RetainerItems = []domain.MaintenanceRetainerItemInput{}
		hourly.DefectGroups = []domain.MaintenanceDefectGroupInput{}
		hourly.RightsStatements = nil
		require.NoError(t, hourly.Validate())
	})

	t.Run("duplicate reference is valid", func(t *testing.T) {
		input := validStandaloneRetainerInput()
		input.DefectGroups = append(input.DefectGroups, domain.MaintenanceDefectGroupInput{
			Title: "2. Detail Lain",
			Items: []domain.MaintenanceDefectItemInput{{
				ItemNumber:  "2.1",
				Priority:    domain.StandalonePriorityP1,
				Function:    "Fungsi kedua",
				ScreenFlow:  "Alur kedua",
				Existing:    "Ada",
				Improvement: "Diperbaiki",
				Reference:   "Ref 1.01", // duplicate reference is explicitly valid
			}},
		})
		require.NoError(t, input.Validate())
	})

	t.Run("blank reference is valid", func(t *testing.T) {
		input := validStandaloneRetainerInput()
		input.DefectGroups[0].Items[0].Reference = ""
		require.NoError(t, input.Validate())
	})

	t.Run("zero discount with show_discount_row false is valid", func(t *testing.T) {
		input := validStandaloneRetainerInput()
		input.ShowDiscountRow = false
		input.DiscountAmount = 0
		require.NoError(t, input.Validate())
	})
}

func TestStandaloneMaintenanceInput_Validate_TableDrivenBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(in *domain.StandaloneMaintenanceInput)
		expectedErr string
	}{
		{
			name: "invalid mode",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.Mode = "invalid_mode"
			},
			expectedErr: `mode: unsupported value "invalid_mode"`,
		},
		{
			name: "zero quotation date",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.QuotationDate = time.Time{}
			},
			expectedErr: "quotation_date: is required",
		},
		{
			name: "blank system name",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.SystemName = "   "
			},
			expectedErr: "system_name: is required",
		},
		{
			name: "overlong system name",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.SystemName = strings.Repeat("A", 201)
			},
			expectedErr: "system_name: must be at most 200 characters",
		},
		{
			name: "invalid tax treatment",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.TaxTreatment = "vat_unknown"
			},
			expectedErr: `tax_treatment: unsupported value "vat_unknown"`,
		},
		{
			name: "retainer mode with hourly cost rows",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems = []domain.MaintenanceHourlyCostItemInput{{
					Name: "Developer", DisplayMonthlyRate: 10_000_000, Headcount: 1, HoursHalfUnits: 2, QuotedLineTotal: 100_000,
				}}
			},
			expectedErr: "hourly_cost_items: must be empty for retainer mode",
		},
		{
			name: "retainer mode with hourly tasks",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyTasks = []domain.MaintenanceHourlyTaskInput{{
					Description: "Desc", Platform: "Web", HoursHalfUnits: 2,
				}}
			},
			expectedErr: "hourly_tasks: must be empty for retainer mode",
		},
		{
			name: "retainer mode with empty retainer items",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems = nil
			},
			expectedErr: "retainer_items: must contain at least one item",
		},
		{
			name: "retainer mode with empty defect groups",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups = []domain.MaintenanceDefectGroupInput{}
			},
			expectedErr: "defect_groups: must contain at least one group",
		},
		{
			name: "retainer items count exceeds 50",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems = make([]domain.MaintenanceRetainerItemInput, 51)
				for i := range in.RetainerItems {
					in.RetainerItems[i] = domain.MaintenanceRetainerItemInput{
						Name: "Dev", MonthlyRate: 1_000_000, FTEQuarters: 4, PeriodMonths: 1,
					}
				}
			},
			expectedErr: "retainer_items: must contain at most 50 items",
		},
		{
			name: "defect groups count exceeds 30",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups = make([]domain.MaintenanceDefectGroupInput, 31)
				for i := range in.DefectGroups {
					in.DefectGroups[i] = domain.MaintenanceDefectGroupInput{
						Title: "Group",
						Items: []domain.MaintenanceDefectItemInput{{
							ItemNumber: string(rune('A' + i)), Priority: domain.StandalonePriorityP0,
							Function: "Func", ScreenFlow: "Flow", Existing: "Exist", Improvement: "Impr",
						}},
					}
				}
			},
			expectedErr: "defect_groups: must contain at most 30 groups",
		},
		{
			name: "defect items count exceeds 100",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				items := make([]domain.MaintenanceDefectItemInput, 101)
				for i := range items {
					items[i] = domain.MaintenanceDefectItemInput{
						ItemNumber: string(rune(1000 + i)), Priority: domain.StandalonePriorityP1,
						Function: "Func", ScreenFlow: "Flow", Existing: "Exist", Improvement: "Impr",
					}
				}
				in.DefectGroups[0].Items = items
			},
			expectedErr: "defect_groups[0].items: must contain at most 100 items",
		},
		{
			name: "retainer item invalid FTE quarter zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].FTEQuarters = 0
			},
			expectedErr: "retainer_items[0].fte_quarters: must be between 1 and 4",
		},
		{
			name: "retainer item invalid FTE quarter greater than 4",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].FTEQuarters = 5
			},
			expectedErr: "retainer_items[0].fte_quarters: must be between 1 and 4",
		},
		{
			name: "retainer item invalid period months zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].PeriodMonths = 0
			},
			expectedErr: "retainer_items[0].period_months: must be between 1 and 60",
		},
		{
			name: "retainer item invalid period months greater than 60",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].PeriodMonths = 61
			},
			expectedErr: "retainer_items[0].period_months: must be between 1 and 60",
		},
		{
			name: "retainer item line total not whole rupiah (not divisible by 4)",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].MonthlyRate = 1001
				in.RetainerItems[0].FTEQuarters = 1
				in.RetainerItems[0].PeriodMonths = 1
			},
			expectedErr: "retainer_items[0]: line calculation must be a whole rupiah amount (divisible by 4)",
		},
		{
			name: "retainer item monthly rate zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].MonthlyRate = 0
			},
			expectedErr: "retainer_items[0].monthly_rate: must be greater than zero",
		},
		{
			name: "retainer item monthly rate negative",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].MonthlyRate = -5000
			},
			expectedErr: "retainer_items[0].monthly_rate: must be greater than zero",
		},
		{
			name: "retainer item monthly rate exceeds max money",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].MonthlyRate = domain.MaxStandaloneMaintenanceMoney + 1
			},
			expectedErr: "retainer_items[0].monthly_rate: exceeds maximum safe amount",
		},
		{
			name: "retainer item line calculation overflow",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].MonthlyRate = domain.MaxStandaloneMaintenanceMoney / 2
				in.RetainerItems[0].FTEQuarters = 3
				in.RetainerItems[0].PeriodMonths = 1
			},
			expectedErr: "retainer_items[0]: line calculation exceeds maximum safe amount",
		},
		{
			name: "discount exceeds subtotal",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				// Subtotal is 23_085_000
				in.DiscountAmount = 23_085_001
			},
			expectedErr: "discount_amount: cannot exceed subtotal",
		},
		{
			name: "positive discount with show_discount_row false",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.ShowDiscountRow = false
				in.DiscountAmount = 1_000_000
			},
			expectedErr: "discount_amount: cannot be positive when show_discount_row is false",
		},
		{
			name: "blank retainer item name",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].Name = "  "
			},
			expectedErr: "retainer_items[0].name: is required",
		},
		{
			name: "overlong retainer item name",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems[0].Name = strings.Repeat("B", 201)
			},
			expectedErr: "retainer_items[0].name: must be at most 200 characters",
		},
		{
			name: "blank defect group title",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Title = "   "
			},
			expectedErr: "defect_groups[0].title: is required",
		},
		{
			name: "defect group empty items",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items = nil
			},
			expectedErr: "defect_groups[0].items: must contain at least one item",
		},
		{
			name: "blank defect item number",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].ItemNumber = "  "
			},
			expectedErr: "defect_groups[0].items[0].item_number: is required",
		},
		{
			name: "invalid priority",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Priority = "P3"
			},
			expectedErr: "defect_groups[0].items[0].priority: must be P0, P1, or P2",
		},
		{
			name: "blank priority",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Priority = ""
			},
			expectedErr: "defect_groups[0].items[0].priority: must be P0, P1, or P2",
		},
		{
			name: "blank defect item function",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Function = "   "
			},
			expectedErr: "defect_groups[0].items[0].function: is required",
		},
		{
			name: "overlong defect item function",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Function = strings.Repeat("C", 4001)
			},
			expectedErr: "defect_groups[0].items[0].function: must be at most 4000 characters",
		},
		{
			name: "blank defect item screen flow",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].ScreenFlow = ""
			},
			expectedErr: "defect_groups[0].items[0].screen_flow: is required",
		},
		{
			name: "blank defect item existing",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Existing = " "
			},
			expectedErr: "defect_groups[0].items[0].existing: is required",
		},
		{
			name: "blank defect item improvement",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Improvement = "\t"
			},
			expectedErr: "defect_groups[0].items[0].improvement: is required",
		},
		{
			name: "overlong defect item reference",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups[0].Items[0].Reference = strings.Repeat("R", 201)
			},
			expectedErr: "defect_groups[0].items[0].reference: must be at most 200 characters",
		},
		{
			name: "duplicate item number across groups",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups = append(in.DefectGroups, domain.MaintenanceDefectGroupInput{
					Title: "2. Second Group",
					Items: []domain.MaintenanceDefectItemInput{{
						ItemNumber:  " 1.1 ", // same as first group item after trim
						Priority:    domain.StandalonePriorityP2,
						Function:    "F",
						ScreenFlow:  "S",
						Existing:    "E",
						Improvement: "I",
					}},
				})
			},
			expectedErr: `defect_groups[1].items[0].item_number: duplicate item number "1.1"`,
		},
		{
			name: "empty payment schedule",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule = nil
			},
			expectedErr: "payment_schedule: must contain at least one row",
		},
		{
			name: "payment schedule count exceeds 60",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule = make([]domain.MaintenancePaymentRowInput, 61)
				for i := range in.PaymentSchedule {
					in.PaymentSchedule[i] = domain.MaintenancePaymentRowInput{
						WorkPeriod: "P", Terms: "T", InvoiceDateLabel: "I", DueDateLabel: "D", Nominal: 100,
					}
				}
			},
			expectedErr: "payment_schedule: must contain at most 60 rows",
		},
		{
			name: "payment row nominal zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule[0].Nominal = 0
			},
			expectedErr: "payment_schedule[0].nominal: must be greater than zero",
		},
		{
			name: "payment row blank work period",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule[0].WorkPeriod = ""
			},
			expectedErr: "payment_schedule[0].work_period: is required",
		},
		{
			name: "payment row blank terms",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule[0].Terms = ""
			},
			expectedErr: "payment_schedule[0].terms: is required",
		},
		{
			name: "payment row blank invoice date label",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule[0].InvoiceDateLabel = ""
			},
			expectedErr: "payment_schedule[0].invoice_date_label: is required",
		},
		{
			name: "payment row blank due date label",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.PaymentSchedule[0].DueDateLabel = ""
			},
			expectedErr: "payment_schedule[0].due_date_label: is required",
		},
		{
			name: "rights statements count exceeds 20",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RightsStatements = make([]string, 21)
				for i := range in.RightsStatements {
					in.RightsStatements[i] = "Statement"
				}
			},
			expectedErr: "rights_statements: must contain at most 20 statements",
		},
		{
			name: "rights statement blank",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RightsStatements = []string{"   "}
			},
			expectedErr: "rights_statements[0]: cannot be blank",
		},
		{
			name: "rights statement overlong",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RightsStatements = []string{strings.Repeat("X", 4001)}
			},
			expectedErr: "rights_statements[0]: must be at most 4000 characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validStandaloneRetainerInput()
			tt.mutate(&input)
			err := input.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestStandaloneMaintenanceInput_Validate_HourlyModeBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(in *domain.StandaloneMaintenanceInput)
		expectedErr string
	}{
		{
			name: "hourly mode with retainer rows",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.RetainerItems = []domain.MaintenanceRetainerItemInput{{
					Name: "Dev", MonthlyRate: 10_000_000, FTEQuarters: 4, PeriodMonths: 1,
				}}
			},
			expectedErr: "retainer_items: must be empty for hourly_task mode",
		},
		{
			name: "hourly mode with defect groups",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.DefectGroups = []domain.MaintenanceDefectGroupInput{{
					Title: "Defects",
					Items: []domain.MaintenanceDefectItemInput{{
						ItemNumber: "1", Priority: domain.StandalonePriorityP0, Function: "F", ScreenFlow: "S", Existing: "E", Improvement: "I",
					}},
				}}
			},
			expectedErr: "defect_groups: must be empty for hourly_task mode",
		},
		{
			name: "hourly mode with empty hourly cost items",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems = nil
			},
			expectedErr: "hourly_cost_items: must contain at least one item",
		},
		{
			name: "hourly mode with empty hourly tasks",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyTasks = nil
			},
			expectedErr: "hourly_tasks: must contain at least one task",
		},
		{
			name: "hourly cost items count exceeds 50",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems = make([]domain.MaintenanceHourlyCostItemInput, 51)
				for i := range in.HourlyCostItems {
					in.HourlyCostItems[i] = domain.MaintenanceHourlyCostItemInput{
						Name: "Dev", DisplayMonthlyRate: 10_000_000, Headcount: 1, HoursHalfUnits: 2, QuotedLineTotal: 100_000,
					}
				}
			},
			expectedErr: "hourly_cost_items: must contain at most 50 items",
		},
		{
			name: "hourly tasks count exceeds 100",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyTasks = make([]domain.MaintenanceHourlyTaskInput, 101)
				for i := range in.HourlyTasks {
					in.HourlyTasks[i] = domain.MaintenanceHourlyTaskInput{
						Description: "Task", Platform: "Web", HoursHalfUnits: 2,
					}
				}
			},
			expectedErr: "hourly_tasks: must contain at most 100 tasks",
		},
		{
			name: "hourly cost item blank name",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].Name = ""
			},
			expectedErr: "hourly_cost_items[0].name: is required",
		},
		{
			name: "hourly cost item display monthly rate zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].DisplayMonthlyRate = 0
			},
			expectedErr: "hourly_cost_items[0].display_monthly_rate: must be greater than zero",
		},
		{
			name: "hourly cost item headcount zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].Headcount = 0
			},
			expectedErr: "hourly_cost_items[0].headcount: must be between 1 and 100",
		},
		{
			name: "hourly cost item headcount exceeds 100",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].Headcount = 101
			},
			expectedErr: "hourly_cost_items[0].headcount: must be between 1 and 100",
		},
		{
			name: "hourly cost item hours half units zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].HoursHalfUnits = 0
			},
			expectedErr: "hourly_cost_items[0].hours_half_units: must be between 1 and 2000",
		},
		{
			name: "hourly cost item hours half units exceeds 2000",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].HoursHalfUnits = 2001
			},
			expectedErr: "hourly_cost_items[0].hours_half_units: must be between 1 and 2000",
		},
		{
			name: "hourly cost item quoted line total zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyCostItems[0].QuotedLineTotal = 0
			},
			expectedErr: "hourly_cost_items[0].quoted_line_total: must be greater than zero",
		},
		{
			name: "hourly task blank description",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyTasks[0].Description = "  "
			},
			expectedErr: "hourly_tasks[0].description: is required",
		},
		{
			name: "hourly task blank platform",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyTasks[0].Platform = ""
			},
			expectedErr: "hourly_tasks[0].platform: is required",
		},
		{
			name: "hourly task invalid hours half units zero",
			mutate: func(in *domain.StandaloneMaintenanceInput) {
				in.HourlyTasks[0].HoursHalfUnits = 0
			},
			expectedErr: "hourly_tasks[0].hours_half_units: must be between 1 and 2000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validStandaloneHourlyInput()
			tt.mutate(&input)
			err := input.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestStandaloneMaintenanceInput_UnmarshalJSON(t *testing.T) {
	t.Run("valid JSON with YYYY-MM-DD date", func(t *testing.T) {
		payload := `{
			"system_name": "Aplikasi Nex Health",
			"quotation_date": "2026-07-07",
			"mode": "retainer",
			"tax_treatment": "exclusive_ppn",
			"retainer_items": [
				{"name": "Senior Web Programmer", "monthly_rate": 16050000, "fte_quarters": 4, "period_months": 1}
			],
			"defect_groups": [{
				"title": "1. Detail",
				"items": [{
					"item_number": "1.1", "priority": "P0", "function": "Func",
					"screen_flow": "Flow", "existing": "Exist", "improvement": "Impr"
				}]
			}],
			"show_discount_row": true,
			"discount_amount": 0,
			"payment_schedule": [
				{"work_period": "P1", "terms": "Maint", "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 16050000}
			]
		}`

		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.NoError(t, err)
		assert.Equal(t, "Aplikasi Nex Health", input.SystemName)
		assert.Equal(t, time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC), input.QuotationDate.UTC())
		assert.Equal(t, domain.StandaloneMaintenanceRetainer, input.Mode)
		require.NoError(t, input.Validate())
	})

	t.Run("valid JSON with RFC3339 date", func(t *testing.T) {
		payload := `{
			"system_name": "Portal Berita Liputan6",
			"quotation_date": "2026-07-10T15:04:05Z",
			"mode": "hourly_task",
			"tax_treatment": "not_stated",
			"hourly_cost_items": [
				{"name": "Senior Mobile", "display_monthly_rate": 16050000, "headcount": 1, "hours_half_units": 16, "quoted_line_total": 730000}
			],
			"hourly_tasks": [
				{"description": "Fix bug", "platform": "Mobile", "hours_half_units": 16}
			],
			"payment_schedule": [
				{"work_period": "P1", "terms": "Maint", "invoice_date_label": "Inv", "due_date_label": "Due", "nominal": 730000}
			]
		}`

		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.NoError(t, err)
		assert.Equal(t, time.Date(2026, 7, 10, 15, 4, 5, 0, time.UTC), input.QuotationDate.UTC())
		require.NoError(t, input.Validate())
	})

	t.Run("rejects missing quotation date", func(t *testing.T) {
		payload := `{
			"system_name": "Aplikasi Nex Health",
			"mode": "retainer",
			"tax_treatment": "exclusive_ppn"
		}`
		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date: is required")
	})

	t.Run("rejects invalid date format", func(t *testing.T) {
		payload := `{
			"system_name": "Aplikasi Nex Health",
			"quotation_date": "07-07-2026",
			"mode": "retainer",
			"tax_treatment": "exclusive_ppn"
		}`
		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "quotation_date: must use YYYY-MM-DD or RFC3339")
	})

	t.Run("rejects unknown field at root of input", func(t *testing.T) {
		payload := `{
			"system_name": "Aplikasi Nex Health",
			"quotation_date": "2026-07-07",
			"mode": "retainer",
			"tax_treatment": "exclusive_ppn",
			"unknown_root_property": "disallowed"
		}`
		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects unknown field in nested retainer item", func(t *testing.T) {
		payload := `{
			"system_name": "Aplikasi Nex Health",
			"quotation_date": "2026-07-07",
			"mode": "retainer",
			"tax_treatment": "exclusive_ppn",
			"retainer_items": [
				{"name": "Senior Web Programmer", "monthly_rate": 16050000, "fte_quarters": 4, "period_months": 1, "extra_item_prop": 123}
			]
		}`
		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects unknown field in nested defect item", func(t *testing.T) {
		payload := `{
			"system_name": "Aplikasi Nex Health",
			"quotation_date": "2026-07-07",
			"mode": "retainer",
			"tax_treatment": "exclusive_ppn",
			"defect_groups": [{
				"title": "Group 1",
				"items": [{
					"item_number": "1.1", "priority": "P0", "function": "F",
					"screen_flow": "S", "existing": "E", "improvement": "I",
					"unsupported_prop": true
				}]
			}]
		}`
		var input domain.StandaloneMaintenanceInput
		err := json.Unmarshal([]byte(payload), &input)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("rejects trailing JSON value", func(t *testing.T) {
		payload := `{"system_name": "Aplikasi Nex Health", "quotation_date": "2026-07-07", "mode": "retainer", "tax_treatment": "exclusive_ppn"} {"trailing": 1}`
		var input domain.StandaloneMaintenanceInput
		err := input.UnmarshalJSON([]byte(payload))
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "trailing JSON value") || strings.Contains(err.Error(), "must contain a single JSON value"))
	})
}
