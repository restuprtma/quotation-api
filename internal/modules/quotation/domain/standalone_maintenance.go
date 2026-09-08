package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxStandaloneMaintenanceMoney = int64(9_000_000_000_000_000)

	MaxStandaloneMaintenanceRetainerItems    = 50
	MaxStandaloneMaintenanceHourlyCostItems  = 50
	MaxStandaloneMaintenanceDefectGroups     = 30
	MaxStandaloneMaintenanceDefectItems      = 100
	MaxStandaloneMaintenanceHourlyTasks      = 100
	MaxStandaloneMaintenancePaymentRows      = 60
	MaxStandaloneMaintenanceRightsStatements = 20

	MaxStandaloneMaintenancePeriodMonths   = 60
	MaxStandaloneMaintenanceHoursHalfUnits = 2000
	MaxStandaloneMaintenanceHeadcount      = 100

	MaxStandaloneMaintenanceShortTextRunes = 200
	MaxStandaloneMaintenanceLongTextRunes  = 4000
)

type StandaloneMaintenanceMode string

const (
	StandaloneMaintenanceRetainer   StandaloneMaintenanceMode = "retainer"
	StandaloneMaintenanceHourlyTask StandaloneMaintenanceMode = "hourly_task"
)

func (m StandaloneMaintenanceMode) valid() bool {
	return m == StandaloneMaintenanceRetainer || m == StandaloneMaintenanceHourlyTask
}

type StandaloneMaintenancePriority string

const (
	StandalonePriorityP0 StandaloneMaintenancePriority = "P0"
	StandalonePriorityP1 StandaloneMaintenancePriority = "P1"
	StandalonePriorityP2 StandaloneMaintenancePriority = "P2"
)

func (p StandaloneMaintenancePriority) valid() bool {
	return p == StandalonePriorityP0 || p == StandalonePriorityP1 || p == StandalonePriorityP2
}

type StandaloneMaintenanceTaxTreatment string

const (
	StandaloneTaxNotStated StandaloneMaintenanceTaxTreatment = "not_stated"
	StandaloneTaxExclusive StandaloneMaintenanceTaxTreatment = "exclusive_ppn"
	StandaloneTaxInclusive StandaloneMaintenanceTaxTreatment = "inclusive_ppn"
)

func (t StandaloneMaintenanceTaxTreatment) valid() bool {
	return t == StandaloneTaxNotStated || t == StandaloneTaxExclusive || t == StandaloneTaxInclusive
}

type MaintenanceRetainerItemInput struct {
	Name         string `json:"name"`
	MonthlyRate  int64  `json:"monthly_rate"`
	FTEQuarters  int32  `json:"fte_quarters"`
	PeriodMonths int32  `json:"period_months"`
}

type MaintenanceHourlyCostItemInput struct {
	Name               string `json:"name"`
	DisplayMonthlyRate int64  `json:"display_monthly_rate"`
	Headcount          int32  `json:"headcount"`
	HoursHalfUnits     int32  `json:"hours_half_units"`
	QuotedLineTotal    int64  `json:"quoted_line_total"`
}

type MaintenanceDefectItemInput struct {
	ItemNumber  string                        `json:"item_number"`
	Priority    StandaloneMaintenancePriority `json:"priority"`
	Function    string                        `json:"function"`
	ScreenFlow  string                        `json:"screen_flow"`
	Existing    string                        `json:"existing"`
	Improvement string                        `json:"improvement"`
	Reference   string                        `json:"reference,omitempty"`
}

type MaintenanceDefectGroupInput struct {
	Title string                       `json:"title"`
	Items []MaintenanceDefectItemInput `json:"items"`
}

type MaintenanceHourlyTaskInput struct {
	Description    string `json:"description"`
	Platform       string `json:"platform"`
	HoursHalfUnits int32  `json:"hours_half_units"`
}

type MaintenancePaymentRowInput struct {
	WorkPeriod       string `json:"work_period"`
	Terms            string `json:"terms"`
	InvoiceDateLabel string `json:"invoice_date_label"`
	DueDateLabel     string `json:"due_date_label"`
	Nominal          int64  `json:"nominal"`
}

type StandaloneMaintenanceInput struct {
	SystemName       string                            `json:"system_name"`
	QuotationDate    time.Time                         `json:"quotation_date"`
	Mode             StandaloneMaintenanceMode         `json:"mode"`
	TaxTreatment     StandaloneMaintenanceTaxTreatment `json:"tax_treatment"`
	RetainerItems    []MaintenanceRetainerItemInput    `json:"retainer_items,omitempty"`
	HourlyCostItems  []MaintenanceHourlyCostItemInput  `json:"hourly_cost_items,omitempty"`
	DefectGroups     []MaintenanceDefectGroupInput     `json:"defect_groups,omitempty"`
	HourlyTasks      []MaintenanceHourlyTaskInput      `json:"hourly_tasks,omitempty"`
	DiscountAmount   int64                             `json:"discount_amount"`
	ShowDiscountRow  bool                              `json:"show_discount_row"`
	PaymentSchedule  []MaintenancePaymentRowInput      `json:"payment_schedule"`
	RightsStatements []string                          `json:"rights_statements,omitempty"`
}

type MaintenanceRetainerLineCalculation struct {
	MaintenanceRetainerItemInput
	LineTotal int64 `json:"line_total"`
}

type MaintenanceHourlyLineCalculation struct {
	MaintenanceHourlyCostItemInput
	LineTotal int64 `json:"line_total"`
}

type StandaloneMaintenanceCalculation struct {
	Mode            StandaloneMaintenanceMode            `json:"mode"`
	Retainer        []MaintenanceRetainerLineCalculation `json:"retainer,omitempty"`
	Hourly          []MaintenanceHourlyLineCalculation   `json:"hourly,omitempty"`
	Subtotal        int64                                `json:"subtotal"`
	DiscountAmount  int64                                `json:"discount_amount"`
	TotalInvestment int64                                `json:"total_investment"`
	PaymentTotal    int64                                `json:"payment_total"`
}

type StandaloneMaintenanceScopeSnapshot struct {
	DefectGroups []MaintenanceDefectGroupInput `json:"defect_groups,omitempty"`
	HourlyTasks  []MaintenanceHourlyTaskInput  `json:"hourly_tasks,omitempty"`
}

type ResolvedStandaloneMaintenanceProfile struct {
	Mode            StandaloneMaintenanceMode         `json:"mode"`
	TaxTreatment    StandaloneMaintenanceTaxTreatment `json:"tax_treatment"`
	TaxClauseText   string                            `json:"tax_clause_text,omitempty"`
	ShowDiscountRow bool                              `json:"show_discount_row"`
	ShowRights      bool                              `json:"show_rights"`
	Sections        []DocumentSection                 `json:"sections"`
}

func (in *StandaloneMaintenanceInput) UnmarshalJSON(data []byte) error {
	type inputAlias StandaloneMaintenanceInput
	var wire struct {
		*inputAlias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.inputAlias = (*inputAlias)(&StandaloneMaintenanceInput{})
	if err := decodeStrictJSON(data, &wire); err != nil {
		return err
	}
	qDate, err := decodeOptionalDate(wire.QuotationDate)
	if err != nil {
		return fmt.Errorf("quotation_date: %w", err)
	}
	if qDate == nil || qDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	wire.inputAlias.QuotationDate = *qDate
	*in = StandaloneMaintenanceInput(*wire.inputAlias)
	return nil
}

func (in StandaloneMaintenanceInput) Validate() error {
	if !in.Mode.valid() {
		return fmt.Errorf("mode: unsupported value %q", in.Mode)
	}
	if in.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	systemName := strings.TrimSpace(in.SystemName)
	if systemName == "" {
		return fmt.Errorf("system_name: is required")
	}
	if len([]rune(systemName)) > MaxStandaloneMaintenanceShortTextRunes {
		return fmt.Errorf("system_name: must be at most %d characters", MaxStandaloneMaintenanceShortTextRunes)
	}
	if !in.TaxTreatment.valid() {
		return fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}

	var subtotal int64
	switch in.Mode {
	case StandaloneMaintenanceRetainer:
		if len(in.HourlyCostItems) > 0 {
			return fmt.Errorf("hourly_cost_items: must be empty for retainer mode")
		}
		if len(in.HourlyTasks) > 0 {
			return fmt.Errorf("hourly_tasks: must be empty for retainer mode")
		}
		if len(in.RetainerItems) == 0 {
			return fmt.Errorf("retainer_items: must contain at least one item")
		}
		if len(in.RetainerItems) > MaxStandaloneMaintenanceRetainerItems {
			return fmt.Errorf("retainer_items: must contain at most %d items", MaxStandaloneMaintenanceRetainerItems)
		}
		var err error
		subtotal, err = validateRetainerItems(in.RetainerItems)
		if err != nil {
			return err
		}

		if len(in.DefectGroups) == 0 {
			return fmt.Errorf("defect_groups: must contain at least one group")
		}
		if len(in.DefectGroups) > MaxStandaloneMaintenanceDefectGroups {
			return fmt.Errorf("defect_groups: must contain at most %d groups", MaxStandaloneMaintenanceDefectGroups)
		}
		if err := validateDefectGroups(in.DefectGroups); err != nil {
			return err
		}

	case StandaloneMaintenanceHourlyTask:
		if len(in.RetainerItems) > 0 {
			return fmt.Errorf("retainer_items: must be empty for hourly_task mode")
		}
		if len(in.DefectGroups) > 0 {
			return fmt.Errorf("defect_groups: must be empty for hourly_task mode")
		}
		if len(in.HourlyCostItems) == 0 {
			return fmt.Errorf("hourly_cost_items: must contain at least one item")
		}
		if len(in.HourlyCostItems) > MaxStandaloneMaintenanceHourlyCostItems {
			return fmt.Errorf("hourly_cost_items: must contain at most %d items", MaxStandaloneMaintenanceHourlyCostItems)
		}
		var err error
		subtotal, err = validateHourlyCostItems(in.HourlyCostItems)
		if err != nil {
			return err
		}

		if len(in.HourlyTasks) == 0 {
			return fmt.Errorf("hourly_tasks: must contain at least one task")
		}
		if len(in.HourlyTasks) > MaxStandaloneMaintenanceHourlyTasks {
			return fmt.Errorf("hourly_tasks: must contain at most %d tasks", MaxStandaloneMaintenanceHourlyTasks)
		}
		if err := validateHourlyTasks(in.HourlyTasks); err != nil {
			return err
		}
	}

	if in.DiscountAmount < 0 {
		return fmt.Errorf("discount_amount: must be greater than or equal to zero")
	}
	if in.DiscountAmount > MaxStandaloneMaintenanceMoney {
		return fmt.Errorf("discount_amount: exceeds maximum safe amount")
	}
	if in.DiscountAmount > 0 && !in.ShowDiscountRow {
		return fmt.Errorf("discount_amount: cannot be positive when show_discount_row is false")
	}
	if in.DiscountAmount > subtotal {
		return fmt.Errorf("discount_amount: cannot exceed subtotal")
	}

	if len(in.PaymentSchedule) == 0 {
		return fmt.Errorf("payment_schedule: must contain at least one row")
	}
	if len(in.PaymentSchedule) > MaxStandaloneMaintenancePaymentRows {
		return fmt.Errorf("payment_schedule: must contain at most %d rows", MaxStandaloneMaintenancePaymentRows)
	}
	var paymentSum int64
	for i, row := range in.PaymentSchedule {
		wp := strings.TrimSpace(row.WorkPeriod)
		if wp == "" {
			return fmt.Errorf("payment_schedule[%d].work_period: is required", i)
		}
		if len([]rune(wp)) > MaxStandaloneMaintenanceShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].work_period: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}

		terms := strings.TrimSpace(row.Terms)
		if terms == "" {
			return fmt.Errorf("payment_schedule[%d].terms: is required", i)
		}
		if len([]rune(terms)) > MaxStandaloneMaintenanceShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].terms: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}

		inv := strings.TrimSpace(row.InvoiceDateLabel)
		if inv == "" {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: is required", i)
		}
		if len([]rune(inv)) > MaxStandaloneMaintenanceShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}

		due := strings.TrimSpace(row.DueDateLabel)
		if due == "" {
			return fmt.Errorf("payment_schedule[%d].due_date_label: is required", i)
		}
		if len([]rune(due)) > MaxStandaloneMaintenanceShortTextRunes {
			return fmt.Errorf("payment_schedule[%d].due_date_label: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}

		if row.Nominal <= 0 {
			return fmt.Errorf("payment_schedule[%d].nominal: must be greater than zero", i)
		}
		if row.Nominal > MaxStandaloneMaintenanceMoney {
			return fmt.Errorf("payment_schedule[%d].nominal: exceeds maximum safe amount", i)
		}
		if row.Nominal > MaxStandaloneMaintenanceMoney-paymentSum {
			return fmt.Errorf("payment_schedule: total exceeds maximum safe amount")
		}
		paymentSum += row.Nominal
	}

	if len(in.RightsStatements) > MaxStandaloneMaintenanceRightsStatements {
		return fmt.Errorf("rights_statements: must contain at most %d statements", MaxStandaloneMaintenanceRightsStatements)
	}
	for i, stmt := range in.RightsStatements {
		trimmed := strings.TrimSpace(stmt)
		if trimmed == "" {
			return fmt.Errorf("rights_statements[%d]: cannot be blank", i)
		}
		if len([]rune(trimmed)) > MaxStandaloneMaintenanceLongTextRunes {
			return fmt.Errorf("rights_statements[%d]: must be at most %d characters", i, MaxStandaloneMaintenanceLongTextRunes)
		}
	}

	return nil
}

func validateRetainerItems(items []MaintenanceRetainerItemInput) (int64, error) {
	var subtotal int64
	for i, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return 0, fmt.Errorf("retainer_items[%d].name: is required", i)
		}
		if len([]rune(name)) > MaxStandaloneMaintenanceShortTextRunes {
			return 0, fmt.Errorf("retainer_items[%d].name: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}
		if item.MonthlyRate <= 0 {
			return 0, fmt.Errorf("retainer_items[%d].monthly_rate: must be greater than zero", i)
		}
		if item.MonthlyRate > MaxStandaloneMaintenanceMoney {
			return 0, fmt.Errorf("retainer_items[%d].monthly_rate: exceeds maximum safe amount", i)
		}
		if item.FTEQuarters < 1 || item.FTEQuarters > 4 {
			return 0, fmt.Errorf("retainer_items[%d].fte_quarters: must be between 1 and 4", i)
		}
		if item.PeriodMonths < 1 || item.PeriodMonths > MaxStandaloneMaintenancePeriodMonths {
			return 0, fmt.Errorf("retainer_items[%d].period_months: must be between 1 and %d", i, MaxStandaloneMaintenancePeriodMonths)
		}

		if item.MonthlyRate > MaxStandaloneMaintenanceMoney/int64(item.FTEQuarters) {
			return 0, fmt.Errorf("retainer_items[%d]: line calculation exceeds maximum safe amount", i)
		}
		inter := item.MonthlyRate * int64(item.FTEQuarters)
		if inter > MaxStandaloneMaintenanceMoney/int64(item.PeriodMonths) {
			return 0, fmt.Errorf("retainer_items[%d]: line calculation exceeds maximum safe amount", i)
		}
		numerator := inter * int64(item.PeriodMonths)
		if numerator%4 != 0 {
			return 0, fmt.Errorf("retainer_items[%d]: line calculation must be a whole rupiah amount (divisible by 4)", i)
		}
		lineTotal := numerator / 4
		if lineTotal > MaxStandaloneMaintenanceMoney-subtotal {
			return 0, fmt.Errorf("retainer_items: subtotal exceeds maximum safe amount")
		}
		subtotal += lineTotal
	}
	return subtotal, nil
}

func validateDefectGroups(groups []MaintenanceDefectGroupInput) error {
	seenItemNumbers := make(map[string]struct{})
	for gIdx, group := range groups {
		title := strings.TrimSpace(group.Title)
		if title == "" {
			return fmt.Errorf("defect_groups[%d].title: is required", gIdx)
		}
		if len([]rune(title)) > MaxStandaloneMaintenanceShortTextRunes {
			return fmt.Errorf("defect_groups[%d].title: must be at most %d characters", gIdx, MaxStandaloneMaintenanceShortTextRunes)
		}
		if len(group.Items) == 0 {
			return fmt.Errorf("defect_groups[%d].items: must contain at least one item", gIdx)
		}
		if len(group.Items) > MaxStandaloneMaintenanceDefectItems {
			return fmt.Errorf("defect_groups[%d].items: must contain at most %d items", gIdx, MaxStandaloneMaintenanceDefectItems)
		}
		for iIdx, item := range group.Items {
			itemNumber := strings.TrimSpace(item.ItemNumber)
			if itemNumber == "" {
				return fmt.Errorf("defect_groups[%d].items[%d].item_number: is required", gIdx, iIdx)
			}
			if len([]rune(itemNumber)) > MaxStandaloneMaintenanceShortTextRunes {
				return fmt.Errorf("defect_groups[%d].items[%d].item_number: must be at most %d characters", gIdx, iIdx, MaxStandaloneMaintenanceShortTextRunes)
			}
			if _, exists := seenItemNumbers[itemNumber]; exists {
				return fmt.Errorf("defect_groups[%d].items[%d].item_number: duplicate item number %q", gIdx, iIdx, itemNumber)
			}
			seenItemNumbers[itemNumber] = struct{}{}

			if !item.Priority.valid() {
				return fmt.Errorf("defect_groups[%d].items[%d].priority: must be P0, P1, or P2", gIdx, iIdx)
			}

			fn := strings.TrimSpace(item.Function)
			if fn == "" {
				return fmt.Errorf("defect_groups[%d].items[%d].function: is required", gIdx, iIdx)
			}
			if len([]rune(fn)) > MaxStandaloneMaintenanceLongTextRunes {
				return fmt.Errorf("defect_groups[%d].items[%d].function: must be at most %d characters", gIdx, iIdx, MaxStandaloneMaintenanceLongTextRunes)
			}

			sf := strings.TrimSpace(item.ScreenFlow)
			if sf == "" {
				return fmt.Errorf("defect_groups[%d].items[%d].screen_flow: is required", gIdx, iIdx)
			}
			if len([]rune(sf)) > MaxStandaloneMaintenanceLongTextRunes {
				return fmt.Errorf("defect_groups[%d].items[%d].screen_flow: must be at most %d characters", gIdx, iIdx, MaxStandaloneMaintenanceLongTextRunes)
			}

			ex := strings.TrimSpace(item.Existing)
			if ex == "" {
				return fmt.Errorf("defect_groups[%d].items[%d].existing: is required", gIdx, iIdx)
			}
			if len([]rune(ex)) > MaxStandaloneMaintenanceLongTextRunes {
				return fmt.Errorf("defect_groups[%d].items[%d].existing: must be at most %d characters", gIdx, iIdx, MaxStandaloneMaintenanceLongTextRunes)
			}

			im := strings.TrimSpace(item.Improvement)
			if im == "" {
				return fmt.Errorf("defect_groups[%d].items[%d].improvement: is required", gIdx, iIdx)
			}
			if len([]rune(im)) > MaxStandaloneMaintenanceLongTextRunes {
				return fmt.Errorf("defect_groups[%d].items[%d].improvement: must be at most %d characters", gIdx, iIdx, MaxStandaloneMaintenanceLongTextRunes)
			}

			ref := strings.TrimSpace(item.Reference)
			if ref != "" && len([]rune(ref)) > MaxStandaloneMaintenanceShortTextRunes {
				return fmt.Errorf("defect_groups[%d].items[%d].reference: must be at most %d characters", gIdx, iIdx, MaxStandaloneMaintenanceShortTextRunes)
			}
		}
	}
	return nil
}

func validateHourlyCostItems(items []MaintenanceHourlyCostItemInput) (int64, error) {
	var subtotal int64
	for i, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return 0, fmt.Errorf("hourly_cost_items[%d].name: is required", i)
		}
		if len([]rune(name)) > MaxStandaloneMaintenanceShortTextRunes {
			return 0, fmt.Errorf("hourly_cost_items[%d].name: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}
		if item.DisplayMonthlyRate <= 0 {
			return 0, fmt.Errorf("hourly_cost_items[%d].display_monthly_rate: must be greater than zero", i)
		}
		if item.DisplayMonthlyRate > MaxStandaloneMaintenanceMoney {
			return 0, fmt.Errorf("hourly_cost_items[%d].display_monthly_rate: exceeds maximum safe amount", i)
		}
		if item.Headcount < 1 || item.Headcount > MaxStandaloneMaintenanceHeadcount {
			return 0, fmt.Errorf("hourly_cost_items[%d].headcount: must be between 1 and %d", i, MaxStandaloneMaintenanceHeadcount)
		}
		if item.HoursHalfUnits < 1 || item.HoursHalfUnits > MaxStandaloneMaintenanceHoursHalfUnits {
			return 0, fmt.Errorf("hourly_cost_items[%d].hours_half_units: must be between 1 and %d", i, MaxStandaloneMaintenanceHoursHalfUnits)
		}
		if item.QuotedLineTotal <= 0 {
			return 0, fmt.Errorf("hourly_cost_items[%d].quoted_line_total: must be greater than zero", i)
		}
		if item.QuotedLineTotal > MaxStandaloneMaintenanceMoney {
			return 0, fmt.Errorf("hourly_cost_items[%d].quoted_line_total: exceeds maximum safe amount", i)
		}
		if item.QuotedLineTotal > MaxStandaloneMaintenanceMoney-subtotal {
			return 0, fmt.Errorf("hourly_cost_items: subtotal exceeds maximum safe amount")
		}
		subtotal += item.QuotedLineTotal
	}
	return subtotal, nil
}

func validateHourlyTasks(tasks []MaintenanceHourlyTaskInput) error {
	for i, task := range tasks {
		desc := strings.TrimSpace(task.Description)
		if desc == "" {
			return fmt.Errorf("hourly_tasks[%d].description: is required", i)
		}
		if len([]rune(desc)) > MaxStandaloneMaintenanceLongTextRunes {
			return fmt.Errorf("hourly_tasks[%d].description: must be at most %d characters", i, MaxStandaloneMaintenanceLongTextRunes)
		}
		plat := strings.TrimSpace(task.Platform)
		if plat == "" {
			return fmt.Errorf("hourly_tasks[%d].platform: is required", i)
		}
		if len([]rune(plat)) > MaxStandaloneMaintenanceShortTextRunes {
			return fmt.Errorf("hourly_tasks[%d].platform: must be at most %d characters", i, MaxStandaloneMaintenanceShortTextRunes)
		}
		if task.HoursHalfUnits < 1 || task.HoursHalfUnits > MaxStandaloneMaintenanceHoursHalfUnits {
			return fmt.Errorf("hourly_tasks[%d].hours_half_units: must be between 1 and %d", i, MaxStandaloneMaintenanceHoursHalfUnits)
		}
	}
	return nil
}
