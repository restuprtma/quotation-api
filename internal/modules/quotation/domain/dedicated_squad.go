package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// Quotation profile values deliberately describe only the shape of a
// quotation. Money, dates, rates, and legal copy remain server-authoritative.
type PreparationMode string

const (
	PreparationProfileDefault PreparationMode = ""
	PreparationProfileNone    PreparationMode = "none"
	PreparationProfileHiring  PreparationMode = "hiring"
	PreparationProfileKickoff PreparationMode = "engineering_kickoff"
)

type MaintenanceMode string

const (
	MaintenanceProfileDefault  MaintenanceMode = ""
	MaintenanceProfileNone     MaintenanceMode = "none"
	MaintenanceProfileSeparate MaintenanceMode = "separate_section"
	MaintenanceProfileIncluded MaintenanceMode = "within_development"
)

type ScopeTimelineMode string

const (
	ScopeTimelineDefault  ScopeTimelineMode = ""
	ScopeTimelineNone     ScopeTimelineMode = "none"
	ScopeTimelineRelative ScopeTimelineMode = "relative"
	ScopeTimelineCalendar ScopeTimelineMode = "calendar"
	ScopeTimelineSprint   ScopeTimelineMode = "sprint"
)

type PaymentMode string

const (
	PaymentProfileDefault PaymentMode = ""
	PaymentProfilePreset  PaymentMode = "preset"
	PaymentProfileRows    PaymentMode = "explicit_rows"
)

type PaymentAllocationKind string

const (
	PaymentAllocationInvestmentBPS  PaymentAllocationKind = "investment_bps"
	PaymentAllocationNetPreparation PaymentAllocationKind = "net_preparation"
	PaymentAllocationRemainingBPS   PaymentAllocationKind = "remaining_bps"
)

type TaxTreatment string

const (
	TaxExclusivePPN TaxTreatment = "exclusive_ppn"
	TaxInclusivePPN TaxTreatment = "inclusive_ppn"
)

const (
	PhaseRequirement = "requirement"
	PhaseDevelopment = "development"
	PhaseSIT         = "sit"
	PhaseUAT         = "uat"
	PhaseGoLive      = "go_live"
	PhaseMaintenance = "maintenance"
	PhaseK6          = "k6"
	PhasePentest     = "pentest"
)

type PaymentRowInput struct {
	WorkPeriod         string                `json:"work_period"`
	Terms              string                `json:"terms"`
	AllocationKind     PaymentAllocationKind `json:"allocation_kind"`
	AllocationBPS      int32                 `json:"allocation_bps,omitempty"`
	InvoiceMonthOffset int32                 `json:"invoice_month_offset,omitempty"`
	InvoiceAtSprintEnd bool                  `json:"invoice_at_sprint_end,omitempty"`
}

type DocumentOptionsInput struct {
	LetterPrefix string       `json:"letter_prefix,omitempty"`
	TaxTreatment TaxTreatment `json:"tax_treatment,omitempty"`
	ClauseCodes  []string     `json:"clause_codes,omitempty"`
}

type QuotationProfileInput struct {
	PreparationMode        PreparationMode      `json:"preparation_mode,omitempty"`
	PreparationDiscountBPS *int32               `json:"preparation_discount_bps,omitempty"`
	MaintenanceMode        MaintenanceMode      `json:"maintenance_mode,omitempty"`
	ScopeTimelineMode      ScopeTimelineMode    `json:"scope_timeline_mode,omitempty"`
	IncludedPhases         []string             `json:"included_phases,omitempty"`
	PaymentMode            PaymentMode          `json:"payment_mode,omitempty"`
	PaymentRows            []PaymentRowInput    `json:"payment_rows,omitempty"`
	DocumentOptions        DocumentOptionsInput `json:"document_options,omitempty"`
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
	if err := decodeStrictJSON(data, &wire); err != nil {
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
	Model              SquadModel             `json:"model"`
	Requirement        string                 `json:"requirement"`
	DevelopmentSquad   []SquadLineInput       `json:"development_squad"`
	MaintenanceEnabled bool                   `json:"maintenance_enabled"`
	MaintenanceSquad   []SquadLineInput       `json:"maintenance_squad,omitempty"`
	KickoffDiscountBPS int32                  `json:"kickoff_discount_bps,omitempty"`
	PaymentCount       int32                  `json:"payment_count"`
	PaymentAnchorDate  *time.Time             `json:"payment_anchor_date"`
	Timeline           TimelineInput          `json:"timeline"`
	QuotationProfile   *QuotationProfileInput `json:"quotation_profile,omitempty"`
}

func (in *DedicatedSquadInput) UnmarshalJSON(data []byte) error {
	type inputAlias DedicatedSquadInput
	var wire struct {
		*inputAlias
		PaymentAnchorDate json.RawMessage `json:"payment_anchor_date"`
	}
	wire.inputAlias = (*inputAlias)(&DedicatedSquadInput{})
	if err := decodeStrictJSON(data, &wire); err != nil {
		return err
	}
	anchor, err := decodeOptionalDate(wire.PaymentAnchorDate)
	if err != nil {
		return fmt.Errorf("payment_anchor_date: %w", err)
	}
	wire.inputAlias.PaymentAnchorDate = anchor
	*in = DedicatedSquadInput(*wire.inputAlias)
	return nil
}

func decodeStrictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("must contain a single JSON value")
		}
		return fmt.Errorf("trailing JSON value: %w", err)
	}
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
	// Discount remains the historical preparation discount. Prompt-first
	// commercial discounts are separate so old calculation snapshots retain
	// their established meaning when decoded or rendered.
	Discount            int64        `json:"discount"`
	TotalBeforeDiscount int64        `json:"total_before_discount,omitempty"`
	AdditionalDiscount  int64        `json:"additional_discount,omitempty"`
	TotalInvestment     int64        `json:"total_investment"`
	Payments            []PaymentRow `json:"payments"`
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

	if in.QuotationProfile != nil {
		return in.validateQuotationProfile()
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

func (in DedicatedSquadInput) validateQuotationProfile() error {
	profile := in.QuotationProfile
	if !profile.PreparationMode.valid() {
		return fmt.Errorf("quotation_profile.preparation_mode: unsupported value %q", profile.PreparationMode)
	}
	if profile.PreparationDiscountBPS != nil {
		if *profile.PreparationDiscountBPS < 0 || *profile.PreparationDiscountBPS > 10_000 {
			return fmt.Errorf("quotation_profile.preparation_discount_bps: must be between 0 and 10000")
		}
		if profile.PreparationMode == PreparationProfileNone {
			return fmt.Errorf("quotation_profile.preparation_discount_bps: preparation_mode must not be none")
		}
	}
	if !profile.MaintenanceMode.valid() {
		return fmt.Errorf("quotation_profile.maintenance_mode: unsupported value %q", profile.MaintenanceMode)
	}
	switch profile.MaintenanceMode {
	case MaintenanceProfileNone:
		if in.MaintenanceEnabled || len(in.MaintenanceSquad) > 0 {
			return fmt.Errorf("quotation_profile.maintenance_mode: none requires no maintenance squad")
		}
	case MaintenanceProfileSeparate, MaintenanceProfileIncluded:
		if len(in.MaintenanceSquad) == 0 {
			return fmt.Errorf("maintenance_squad: must contain at least one line for quotation_profile.maintenance_mode")
		}
	}

	if !profile.ScopeTimelineMode.valid() {
		return fmt.Errorf("quotation_profile.scope_timeline_mode: unsupported value %q", profile.ScopeTimelineMode)
	}
	if profile.ScopeTimelineMode == ScopeTimelineNone && len(profile.IncludedPhases) > 0 {
		return fmt.Errorf("quotation_profile.included_phases: must be empty when scope_timeline_mode is none")
	}
	if err := validateUniqueCatalog(profile.IncludedPhases, validPhaseCode, "quotation_profile.included_phases"); err != nil {
		return err
	}

	if !profile.PaymentMode.valid() {
		return fmt.Errorf("quotation_profile.payment_mode: unsupported value %q", profile.PaymentMode)
	}
	if profile.PaymentMode != PaymentProfileRows && len(profile.PaymentRows) > 0 {
		return fmt.Errorf("quotation_profile.payment_rows: only supported for payment_mode explicit_rows")
	}
	if profile.PaymentMode == PaymentProfileRows {
		if len(profile.PaymentRows) == 0 {
			return fmt.Errorf("quotation_profile.payment_rows: must contain at least one row for payment_mode explicit_rows")
		}
		if len(profile.PaymentRows) > MaxPaymentCount {
			return fmt.Errorf("quotation_profile.payment_rows: must contain at most %d rows", MaxPaymentCount)
		}
		if err := validatePaymentRows(profile.PaymentRows, profile.PreparationMode); err != nil {
			return err
		}
	}

	if err := validateDocumentOptions(profile.DocumentOptions); err != nil {
		return err
	}
	return validateProfileSprintDates(in)
}

func (mode PreparationMode) valid() bool {
	return mode == PreparationProfileDefault || mode == PreparationProfileNone || mode == PreparationProfileHiring || mode == PreparationProfileKickoff
}

func (mode MaintenanceMode) valid() bool {
	return mode == MaintenanceProfileDefault || mode == MaintenanceProfileNone || mode == MaintenanceProfileSeparate || mode == MaintenanceProfileIncluded
}

func (mode ScopeTimelineMode) valid() bool {
	return mode == ScopeTimelineDefault || mode == ScopeTimelineNone || mode == ScopeTimelineRelative || mode == ScopeTimelineCalendar || mode == ScopeTimelineSprint
}

func (mode PaymentMode) valid() bool {
	return mode == PaymentProfileDefault || mode == PaymentProfilePreset || mode == PaymentProfileRows
}

func validatePaymentRows(rows []PaymentRowInput, preparationMode PreparationMode) error {
	investmentRows := 0
	netPreparationRows := 0
	remainingRows := 0
	var investmentBPS, remainingBPS int64
	for i, row := range rows {
		path := fmt.Sprintf("quotation_profile.payment_rows[%d]", i)
		if l := len([]rune(strings.TrimSpace(row.WorkPeriod))); l == 0 || l > 120 {
			return fmt.Errorf("%s.work_period: must be between 1 and 120 characters", path)
		}
		if l := len([]rune(strings.TrimSpace(row.Terms))); l == 0 || l > 120 {
			return fmt.Errorf("%s.terms: must be between 1 and 120 characters", path)
		}
		if row.InvoiceMonthOffset < 0 || row.InvoiceMonthOffset > 59 {
			return fmt.Errorf("%s.invoice_month_offset: must be between 0 and 59", path)
		}
		switch row.AllocationKind {
		case PaymentAllocationInvestmentBPS:
			investmentRows++
			if row.AllocationBPS <= 0 || row.AllocationBPS > 10_000 {
				return fmt.Errorf("%s.allocation_bps: must be between 1 and 10000", path)
			}
			investmentBPS += int64(row.AllocationBPS)
		case PaymentAllocationNetPreparation:
			netPreparationRows++
			if row.AllocationBPS != 0 {
				return fmt.Errorf("%s.allocation_bps: must be empty for allocation_kind net_preparation", path)
			}
		case PaymentAllocationRemainingBPS:
			remainingRows++
			if row.AllocationBPS <= 0 || row.AllocationBPS > 10_000 {
				return fmt.Errorf("%s.allocation_bps: must be between 1 and 10000", path)
			}
			remainingBPS += int64(row.AllocationBPS)
		default:
			return fmt.Errorf("%s.allocation_kind: unsupported value %q", path, row.AllocationKind)
		}
	}
	if investmentRows == len(rows) && investmentBPS == 10_000 {
		return nil
	}
	if netPreparationRows == 1 && remainingRows == len(rows)-1 && remainingBPS == 10_000 {
		if preparationMode == PreparationProfileNone {
			return fmt.Errorf("quotation_profile.payment_rows: net_preparation requires a non-none preparation_mode")
		}
		return nil
	}
	return fmt.Errorf("quotation_profile.payment_rows: must be all investment_bps totaling 10000, or one net_preparation plus remaining_bps totaling 10000")
}

func validateDocumentOptions(options DocumentOptionsInput) error {
	if options.LetterPrefix != "" && options.LetterPrefix != "CTH" && options.LetterPrefix != "VEN" && options.LetterPrefix != "LANDA" && options.LetterPrefix != "VENTURO" {
		return fmt.Errorf("quotation_profile.document_options.letter_prefix: unsupported value %q", options.LetterPrefix)
	}
	if options.TaxTreatment != "" && options.TaxTreatment != TaxExclusivePPN && options.TaxTreatment != TaxInclusivePPN {
		return fmt.Errorf("quotation_profile.document_options.tax_treatment: unsupported value %q", options.TaxTreatment)
	}
	return validateUniqueCatalog(options.ClauseCodes, validClauseCode, "quotation_profile.document_options.clause_codes")
}

func validateUniqueCatalog(values []string, allowed func(string) bool, path string) error {
	seen := make(map[string]struct{}, len(values))
	for i, value := range values {
		if !allowed(value) {
			return fmt.Errorf("%s[%d]: unsupported value %q", path, i, value)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("%s[%d]: duplicate value %q", path, i, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validPhaseCode(value string) bool {
	switch value {
	case PhaseRequirement, PhaseDevelopment, PhaseSIT, PhaseUAT, PhaseGoLive, PhaseMaintenance, PhaseK6, PhasePentest:
		return true
	default:
		return false
	}
}

func validClauseCode(value string) bool {
	switch value {
	case "out_of_pocket", "overtime", "onsite", "third_party":
		return true
	default:
		return false
	}
}

func validateProfileSprintDates(in DedicatedSquadInput) error {
	if in.Model != ModelSprint {
		return nil
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
