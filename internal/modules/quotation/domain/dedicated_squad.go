package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SquadModel string

const (
	ModelSOWLarge          SquadModel = "1A"
	ModelSOWSmall          SquadModel = "1B"
	ModelStaffAugmentation SquadModel = "1C"
	ModelSprint            SquadModel = "1D"
)

type TimelineMode string

const (
	TimelineRelative TimelineMode = "relative"
	TimelineCalendar TimelineMode = "calendar"
	TimelineSprint   TimelineMode = "sprint"
)

type SquadLineInput struct {
	Role              string `json:"role"`
	Seniority         string `json:"seniority"`
	HeadcountQuarters int32  `json:"headcount_quarters"`
	PeriodQuarters    int32  `json:"period_quarters"`
	WorkdayCount      int32  `json:"workday_count,omitempty"`
}

type TimelineInput struct {
	Mode          TimelineMode `json:"mode"`
	StartDate     *time.Time   `json:"start_date,omitempty"`
	SprintEndDate *time.Time   `json:"sprint_end_date,omitempty"`
}

// UnmarshalJSON accepts both the browser's calendar-date form (YYYY-MM-DD)
// and RFC3339 timestamps. Marshal remains the standard time.Time encoding so
// persisted snapshots are unambiguous and reproducible.
func (in *TimelineInput) UnmarshalJSON(data []byte) error {
	var wire struct {
		Mode          TimelineMode    `json:"mode"`
		StartDate     json.RawMessage `json:"start_date"`
		SprintEndDate json.RawMessage `json:"sprint_end_date"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	start, err := decodeOptionalDate(wire.StartDate)
	if err != nil {
		return fmt.Errorf("start_date: %w", err)
	}
	end, err := decodeOptionalDate(wire.SprintEndDate)
	if err != nil {
		return fmt.Errorf("sprint_end_date: %w", err)
	}
	in.Mode, in.StartDate, in.SprintEndDate = wire.Mode, start, end
	return nil
}

type DedicatedSquadInput struct {
	Model              SquadModel       `json:"model"`
	Requirement        string           `json:"requirement"`
	DevelopmentSquad   []SquadLineInput `json:"development_squad"`
	MaintenanceEnabled bool             `json:"maintenance_enabled"`
	MaintenanceSquad   []SquadLineInput `json:"maintenance_squad,omitempty"`
	KickoffDiscountBPS int32            `json:"kickoff_discount_bps,omitempty"`
	PaymentCount       int32            `json:"payment_count"`
	PaymentAnchorDate  *time.Time       `json:"payment_anchor_date"`
	Timeline           TimelineInput    `json:"timeline"`
}

func (in *DedicatedSquadInput) UnmarshalJSON(data []byte) error {
	type inputAlias DedicatedSquadInput
	var wire struct {
		*inputAlias
		PaymentAnchorDate json.RawMessage `json:"payment_anchor_date"`
	}
	wire.inputAlias = (*inputAlias)(in)
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	anchor, err := decodeOptionalDate(wire.PaymentAnchorDate)
	if err != nil {
		return fmt.Errorf("payment_anchor_date: %w", err)
	}
	in.PaymentAnchorDate = anchor
	return nil
}

// Calculation types are snapshots of server-authoritative commercial values.
type SquadLineCalculation struct {
	SquadLineInput
	TermType      string `json:"term_type"`
	AmountPerUnit int64  `json:"amount_per_unit"`
	LineTotal     int64  `json:"line_total"`
}

type PreparationCalculation struct {
	Kind     string `json:"kind"`
	Gross    int64  `json:"gross"`
	Discount int64  `json:"discount"`
	Net      int64  `json:"net"`
}

type PaymentRow struct {
	WorkPeriod  string    `json:"work_period"`
	Terms       string    `json:"terms"`
	InvoiceDate time.Time `json:"invoice_date"`
	DueDate     time.Time `json:"due_date"`
	Nominal     int64     `json:"nominal"`
}

type DedicatedSquadCalculation struct {
	Model               SquadModel             `json:"model"`
	Development         []SquadLineCalculation `json:"development"`
	DevelopmentSubtotal int64                  `json:"development_subtotal"`
	Maintenance         []SquadLineCalculation `json:"maintenance,omitempty"`
	MaintenanceSubtotal int64                  `json:"maintenance_subtotal"`
	Preparation         PreparationCalculation `json:"preparation"`
	Total               int64                  `json:"total"`
	Discount            int64                  `json:"discount"`
	TotalInvestment     int64                  `json:"total_investment"`
	Payments            []PaymentRow           `json:"payments"`
}

type DocumentSection struct {
	Number int    `json:"number"`
	Code   string `json:"code"`
	Title  string `json:"title"`
}

type TimelineRow struct {
	Description  string   `json:"description"`
	Timeline     string   `json:"timeline"`
	Deliverables []string `json:"deliverables,omitempty"`
}

type Module struct {
	Name        string   `json:"name"`
	SubFeatures []string `json:"sub_features"`
}

const (
	MaxDedicatedRequirementRunes = 20_000
	MaxHeadcountQuarters         = 400 // 100 people
	MaxPeriodQuarters            = 96  // 24 months
	MaxSprintWorkdays            = 366
	MaxPaymentCount              = 60
)

func (in DedicatedSquadInput) Validate() error {
	if !in.Model.valid() {
		return fmt.Errorf("model: unsupported dedicated squad model %q", in.Model)
	}
	if len(in.DevelopmentSquad) == 0 {
		return fmt.Errorf("development_squad: must contain at least one line")
	}
	requirement := strings.TrimSpace(in.Requirement)
	if requirement == "" {
		return fmt.Errorf("requirement: is required")
	}
	if len([]rune(requirement)) > MaxDedicatedRequirementRunes {
		return fmt.Errorf("requirement: must be at most %d characters", MaxDedicatedRequirementRunes)
	}
	if in.PaymentAnchorDate == nil {
		return fmt.Errorf("payment_anchor_date: is required")
	}
	if in.PaymentCount <= 0 {
		return fmt.Errorf("payment_count: must be greater than zero")
	}
	if in.PaymentCount > MaxPaymentCount {
		return fmt.Errorf("payment_count: must be at most %d", MaxPaymentCount)
	}
	for i, line := range in.DevelopmentSquad {
		if err := validateSquadLine(line, in.Model == ModelSprint); err != nil {
			return fmt.Errorf("development_squad[%d].%w", i, err)
		}
	}
	for i, line := range in.MaintenanceSquad {
		if err := validateSquadLine(line, false); err != nil {
			return fmt.Errorf("maintenance_squad[%d].%w", i, err)
		}
	}

	if in.Model != ModelSOWLarge && (in.MaintenanceEnabled || len(in.MaintenanceSquad) > 0) {
		return fmt.Errorf("maintenance_enabled: only supported for model 1A")
	}
	if in.MaintenanceEnabled && len(in.MaintenanceSquad) == 0 {
		return fmt.Errorf("maintenance_squad: must contain at least one line when maintenance_enabled is true")
	}
	if !in.MaintenanceEnabled && len(in.MaintenanceSquad) > 0 {
		return fmt.Errorf("maintenance_squad: must be empty when maintenance_enabled is false")
	}

	switch in.Model {
	case ModelSOWLarge, ModelSOWSmall:
		if in.KickoffDiscountBPS != 0 {
			return fmt.Errorf("kickoff_discount_bps: only supported for model 1C")
		}
		if in.Timeline.Mode != TimelineRelative && in.Timeline.Mode != TimelineCalendar {
			return fmt.Errorf("timeline.mode: must be relative or calendar for model %s", in.Model)
		}
		if in.Timeline.Mode == TimelineCalendar && in.Timeline.StartDate == nil {
			return fmt.Errorf("timeline.start_date: is required for calendar timeline")
		}
		if in.Timeline.Mode == TimelineRelative && in.Timeline.StartDate != nil {
			return fmt.Errorf("timeline.start_date: must be empty for relative timeline")
		}
		if in.Timeline.SprintEndDate != nil {
			return fmt.Errorf("timeline.sprint_end_date: only supported for model 1D")
		}
	case ModelStaffAugmentation:
		if in.KickoffDiscountBPS != 5000 && in.KickoffDiscountBPS != 10000 {
			return fmt.Errorf("kickoff_discount_bps: must be 5000 or 10000 for model 1C")
		}
		if in.Timeline.Mode != "" || in.Timeline.StartDate != nil || in.Timeline.SprintEndDate != nil {
			return fmt.Errorf("timeline.mode: timeline is not supported for model 1C")
		}
	case ModelSprint:
		if in.KickoffDiscountBPS != 0 {
			return fmt.Errorf("kickoff_discount_bps: only supported for model 1C")
		}
		if in.Timeline.Mode != TimelineSprint {
			return fmt.Errorf("timeline.mode: must be sprint for model 1D")
		}
		if in.Timeline.StartDate == nil {
			return fmt.Errorf("timeline.start_date: is required for model 1D")
		}
		if in.Timeline.SprintEndDate == nil {
			return fmt.Errorf("timeline.sprint_end_date: is required for model 1D")
		}
		if in.Timeline.SprintEndDate.Before(*in.Timeline.StartDate) {
			return fmt.Errorf("timeline.sprint_end_date: must not be before timeline.start_date")
		}
		want := inclusiveWeekdays(*in.Timeline.StartDate, *in.Timeline.SprintEndDate)
		for i, line := range in.DevelopmentSquad {
			if line.WorkdayCount != want {
				return fmt.Errorf("development_squad[%d].workday_count: must equal %d weekdays in sprint range", i, want)
			}
		}
	}
	return nil
}

func (m SquadModel) valid() bool {
	return m == ModelSOWLarge || m == ModelSOWSmall || m == ModelStaffAugmentation || m == ModelSprint
}

func validateSquadLine(line SquadLineInput, sprint bool) error {
	if strings.TrimSpace(line.Role) == "" {
		return fmt.Errorf("role: is required")
	}
	if strings.TrimSpace(line.Seniority) == "" {
		return fmt.Errorf("seniority: is required")
	}
	if line.HeadcountQuarters <= 0 {
		return fmt.Errorf("headcount_quarters: must be greater than zero")
	}
	if line.HeadcountQuarters > MaxHeadcountQuarters {
		return fmt.Errorf("headcount_quarters: must be at most %d", MaxHeadcountQuarters)
	}
	if sprint {
		if line.WorkdayCount <= 0 {
			return fmt.Errorf("workday_count: must be greater than zero for model 1D")
		}
		if line.WorkdayCount > MaxSprintWorkdays {
			return fmt.Errorf("workday_count: must be at most %d", MaxSprintWorkdays)
		}
		if line.PeriodQuarters != 0 {
			return fmt.Errorf("period_quarters: must be zero for model 1D")
		}
		return nil
	}
	if line.PeriodQuarters <= 0 {
		return fmt.Errorf("period_quarters: must be greater than zero")
	}
	if line.PeriodQuarters > MaxPeriodQuarters {
		return fmt.Errorf("period_quarters: must be at most %d", MaxPeriodQuarters)
	}
	if line.WorkdayCount != 0 {
		return fmt.Errorf("workday_count: only supported for model 1D")
	}
	return nil
}

func inclusiveWeekdays(start, end time.Time) int32 {
	var count int32
	for day := dateOnly(start); !day.After(dateOnly(end)); day = day.AddDate(0, 0, 1) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			count++
		}
	}
	return count
}

func dateOnly(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, value.Location())
}

func decodeOptionalDate(raw json.RawMessage) (*time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("must be a date string")
	}
	if value == "" {
		return nil, nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("must use YYYY-MM-DD or RFC3339")
}
