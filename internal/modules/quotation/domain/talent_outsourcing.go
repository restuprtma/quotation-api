package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxTalentMoney            = int64(9_000_000_000_000_000)
	MaxTalentScopeBlocks      = 50
	MaxTalentPaymentRows      = 60
	MaxTalentShortTextRunes   = 200
	MaxTalentLongTextRunes    = 4_000
	MaxTalentDescriptionRunes = 20_000
)

type TalentEngagementMode string

const (
	TalentModeManpowerSupply TalentEngagementMode = "manpower_supply"
	TalentModeManagedSquad   TalentEngagementMode = "managed_squad"
)

type TalentTaxTreatment string

const (
	TalentTaxBPJSDPP12 TalentTaxTreatment = "bpjs_dpp_ppn_12"
	TalentTaxExclusive TalentTaxTreatment = "exclusive_ppn"
	TalentTaxNotStated TalentTaxTreatment = "not_stated"
)

type TalentCostCategory string

const (
	TalentCategoryPersonnel    TalentCostCategory = "personnel"
	TalentCategoryNonPersonnel TalentCostCategory = "non_personnel"
)

type TalentPeriodUnit string

const (
	TalentPeriodMonth   TalentPeriodUnit = "month"
	TalentPeriodWorkday TalentPeriodUnit = "workday"
)

type TalentLimitationCode string

const (
	TalentLimitOutOfPocket TalentLimitationCode = "out_of_pocket"
	TalentLimitOvertime    TalentLimitationCode = "overtime"
	TalentLimitThirdParty  TalentLimitationCode = "third_party"
	TalentLimitScopeChange TalentLimitationCode = "scope_change"
)

type TalentCostItemInput struct {
	Name         string           `json:"name"`
	Quantity     int32            `json:"quantity"`
	QuantityUnit string           `json:"quantity_unit"`
	PeriodCount  int32            `json:"period_count"`
	PeriodUnit   TalentPeriodUnit `json:"period_unit"`
	UnitRate     int64            `json:"unit_rate"`
}
type TalentCostGroupInput struct {
	Category TalentCostCategory    `json:"category"`
	Title    string                `json:"title"`
	Items    []TalentCostItemInput `json:"items"`
}
type TalentPreparationItemInput struct {
	Name            string `json:"name"`
	Headcount       int32  `json:"headcount"`
	QuotedLineTotal int64  `json:"quoted_line_total"`
}
type TalentSquadItemInput struct {
	Role         string `json:"role"`
	MonthlyRate  int64  `json:"monthly_rate"`
	Headcount    int32  `json:"headcount"`
	PeriodMonths int32  `json:"period_months"`
}
type TalentScopeBlockInput struct {
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Responsibilities []string `json:"responsibilities,omitempty"`
	Qualifications   []string `json:"qualifications,omitempty"`
	Deliverables     []string `json:"deliverables,omitempty"`
	Timeline         string   `json:"timeline,omitempty"`
}
type TalentPaymentRowInput struct {
	WorkPeriod       string `json:"work_period"`
	Terms            string `json:"terms"`
	InvoiceDateLabel string `json:"invoice_date_label"`
	DueDateLabel     string `json:"due_date_label"`
	Nominal          int64  `json:"nominal"`
}
type TalentOutsourcingInput struct {
	EngagementMode             TalentEngagementMode         `json:"engagement_mode"`
	DocumentNumber             string                       `json:"document_number"`
	ServiceName                string                       `json:"service_name"`
	QuotationDate              time.Time                    `json:"quotation_date"`
	TaxTreatment               TalentTaxTreatment           `json:"tax_treatment"`
	CostGroups                 []TalentCostGroupInput       `json:"cost_groups,omitempty"`
	PreparationItems           []TalentPreparationItemInput `json:"preparation_items,omitempty"`
	PreparationDiscountAmount  int64                        `json:"preparation_discount_amount,omitempty"`
	ShowPreparationDiscountRow bool                         `json:"show_preparation_discount_row,omitempty"`
	SquadItems                 []TalentSquadItemInput       `json:"squad_items,omitempty"`
	ScopeBlocks                []TalentScopeBlockInput      `json:"scope_blocks"`
	PaymentSchedule            []TalentPaymentRowInput      `json:"payment_schedule"`
	PaymentDueDays             int32                        `json:"payment_due_days"`
	LimitationCodes            []TalentLimitationCode       `json:"limitation_codes"`
	AdditionalLimitations      []string                     `json:"additional_limitations,omitempty"`
	WarrantyStatements         []string                     `json:"warranty_statements,omitempty"`
	RightsStatements           []string                     `json:"rights_statements,omitempty"`
}

func (in *TalentOutsourcingInput) UnmarshalJSON(data []byte) error {
	type alias TalentOutsourcingInput
	var wire struct {
		*alias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.alias = (*alias)(&TalentOutsourcingInput{})
	if err := decodeStrictJSON(data, &wire); err != nil {
		return err
	}
	d, err := decodeOptionalDate(wire.QuotationDate)
	if err != nil || d == nil || d.IsZero() {
		return fmt.Errorf("quotation_date: is required and must use YYYY-MM-DD")
	}
	wire.alias.QuotationDate = *d
	*in = TalentOutsourcingInput(*wire.alias)
	return nil
}
func NormalizeTalentOutsourcingInput(in TalentOutsourcingInput) TalentOutsourcingInput {
	out := in
	out.DocumentNumber = strings.TrimSpace(in.DocumentNumber)
	out.ServiceName = strings.TrimSpace(in.ServiceName)
	out.CostGroups = append([]TalentCostGroupInput(nil), in.CostGroups...)
	for i := range out.CostGroups {
		out.CostGroups[i].Title = strings.TrimSpace(out.CostGroups[i].Title)
		out.CostGroups[i].Items = append([]TalentCostItemInput(nil), out.CostGroups[i].Items...)
		for j := range out.CostGroups[i].Items {
			out.CostGroups[i].Items[j].Name = strings.TrimSpace(out.CostGroups[i].Items[j].Name)
			out.CostGroups[i].Items[j].QuantityUnit = strings.TrimSpace(out.CostGroups[i].Items[j].QuantityUnit)
		}
	}
	out.PreparationItems = append([]TalentPreparationItemInput(nil), in.PreparationItems...)
	for i := range out.PreparationItems {
		out.PreparationItems[i].Name = strings.TrimSpace(out.PreparationItems[i].Name)
	}
	out.SquadItems = append([]TalentSquadItemInput(nil), in.SquadItems...)
	for i := range out.SquadItems {
		out.SquadItems[i].Role = strings.TrimSpace(out.SquadItems[i].Role)
	}
	out.ScopeBlocks = append([]TalentScopeBlockInput(nil), in.ScopeBlocks...)
	for i := range out.ScopeBlocks {
		b := &out.ScopeBlocks[i]
		b.Title = strings.TrimSpace(b.Title)
		b.Description = strings.TrimSpace(b.Description)
		b.Timeline = strings.TrimSpace(b.Timeline)
		b.Responsibilities = trimTalentStrings(b.Responsibilities)
		b.Qualifications = trimTalentStrings(b.Qualifications)
		b.Deliverables = trimTalentStrings(b.Deliverables)
	}
	out.PaymentSchedule = append([]TalentPaymentRowInput(nil), in.PaymentSchedule...)
	for i := range out.PaymentSchedule {
		p := &out.PaymentSchedule[i]
		p.WorkPeriod = strings.TrimSpace(p.WorkPeriod)
		p.Terms = strings.TrimSpace(p.Terms)
		p.InvoiceDateLabel = strings.TrimSpace(p.InvoiceDateLabel)
		p.DueDateLabel = strings.TrimSpace(p.DueDateLabel)
	}
	out.LimitationCodes = append([]TalentLimitationCode(nil), in.LimitationCodes...)
	out.AdditionalLimitations = trimTalentStrings(in.AdditionalLimitations)
	out.WarrantyStatements = trimTalentStrings(in.WarrantyStatements)
	out.RightsStatements = trimTalentStrings(in.RightsStatements)
	return out
}
func trimTalentStrings(v []string) []string {
	out := make([]string, 0, len(v))
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
func (in TalentOutsourcingInput) Validate() error {
	in = NormalizeTalentOutsourcingInput(in)
	if in.EngagementMode != TalentModeManpowerSupply && in.EngagementMode != TalentModeManagedSquad {
		return fmt.Errorf("engagement_mode: unsupported value %q", in.EngagementMode)
	}
	if err := talentText("document_number", in.DocumentNumber, 1, 50); err != nil {
		return err
	}
	if err := talentText("service_name", in.ServiceName, 1, MaxTalentShortTextRunes); err != nil {
		return err
	}
	if in.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	if in.TaxTreatment != TalentTaxBPJSDPP12 && in.TaxTreatment != TalentTaxExclusive && in.TaxTreatment != TalentTaxNotStated {
		return fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}
	if in.EngagementMode == TalentModeManpowerSupply {
		if len(in.CostGroups) == 0 {
			return fmt.Errorf("cost_groups: is required")
		}
		if len(in.SquadItems) > 0 || len(in.PreparationItems) > 0 || in.PreparationDiscountAmount != 0 {
			return fmt.Errorf("squad_items/preparation_items: forbidden for manpower_supply")
		}
		if err := validateTalentCostGroups(in.CostGroups); err != nil {
			return err
		}
	} else {
		if len(in.CostGroups) > 0 {
			return fmt.Errorf("cost_groups: forbidden for managed_squad")
		}
		if len(in.SquadItems) == 0 {
			return fmt.Errorf("squad_items: is required")
		}
		if err := validateTalentManaged(in); err != nil {
			return err
		}
	}
	if len(in.ScopeBlocks) < 1 || len(in.ScopeBlocks) > MaxTalentScopeBlocks {
		return fmt.Errorf("scope_blocks: must contain between 1 and %d blocks", MaxTalentScopeBlocks)
	}
	hasTimeline := false
	for i, b := range in.ScopeBlocks {
		if err := talentText(fmt.Sprintf("scope_blocks[%d].title", i), b.Title, 1, MaxTalentShortTextRunes); err != nil {
			return err
		}
		if b.Description == "" && len(b.Responsibilities) == 0 && len(b.Qualifications) == 0 && len(b.Deliverables) == 0 {
			return fmt.Errorf("scope_blocks[%d]: requires content", i)
		}
		if err := talentText(fmt.Sprintf("scope_blocks[%d].description", i), b.Description, 0, MaxTalentDescriptionRunes); err != nil {
			return err
		}
		if b.Timeline != "" {
			hasTimeline = true
			if err := talentText(fmt.Sprintf("scope_blocks[%d].timeline", i), b.Timeline, 1, MaxTalentShortTextRunes); err != nil {
				return err
			}
		}
		for _, list := range [][]string{b.Responsibilities, b.Qualifications, b.Deliverables} {
			for _, s := range list {
				if err := talentText("scope_blocks", s, 1, MaxTalentLongTextRunes); err != nil {
					return err
				}
			}
		}
	}
	if !hasTimeline {
		return fmt.Errorf("timeline: at least one scope block requires timeline")
	}
	if len(in.PaymentSchedule) < 1 || len(in.PaymentSchedule) > MaxTalentPaymentRows {
		return fmt.Errorf("payment_schedule: must contain between 1 and %d rows", MaxTalentPaymentRows)
	}
	for i, p := range in.PaymentSchedule {
		for n, s := range map[string]string{"work_period": p.WorkPeriod, "terms": p.Terms, "invoice_date_label": p.InvoiceDateLabel, "due_date_label": p.DueDateLabel} {
			if err := talentText(fmt.Sprintf("payment_schedule[%d].%s", i, n), s, 1, MaxTalentShortTextRunes); err != nil {
				return err
			}
		}
		if p.Nominal <= 0 || p.Nominal > MaxTalentMoney {
			return fmt.Errorf("payment_schedule[%d].nominal: invalid amount", i)
		}
	}
	if in.PaymentDueDays < 0 || in.PaymentDueDays > 365 {
		return fmt.Errorf("payment_due_days: must be between 0 and 365")
	}
	if err := validateTalentLimitations(in.LimitationCodes); err != nil {
		return err
	}
	for _, list := range [][]string{in.AdditionalLimitations, in.WarrantyStatements, in.RightsStatements} {
		for _, s := range list {
			if err := talentText("legal statement", s, 1, MaxTalentLongTextRunes); err != nil {
				return err
			}
		}
	}
	return nil
}
func talentText(path, s string, min, max int) error {
	if strings.Contains(s, "<") || strings.Contains(s, ">") || strings.Contains(strings.ToLower(s), "javascript:") {
		return fmt.Errorf("%s: HTML-like input is not allowed", path)
	}
	n := len([]rune(s))
	if n < min {
		return fmt.Errorf("%s: is required", path)
	}
	if max > 0 && n > max {
		return fmt.Errorf("%s: must be at most %d characters", path, max)
	}
	return nil
}
func validateTalentCostGroups(gs []TalentCostGroupInput) error {
	seen := map[TalentCostCategory]bool{}
	for i, g := range gs {
		if (g.Category != TalentCategoryPersonnel && g.Category != TalentCategoryNonPersonnel) || seen[g.Category] {
			return fmt.Errorf("cost_groups[%d].category: duplicate or unsupported", i)
		}
		seen[g.Category] = true
		if err := talentText(fmt.Sprintf("cost_groups[%d].title", i), g.Title, 1, MaxTalentShortTextRunes); err != nil {
			return err
		}
		if len(g.Items) == 0 {
			return fmt.Errorf("cost_groups[%d].items: is required", i)
		}
		for j, x := range g.Items {
			if err := talentText(fmt.Sprintf("cost_groups[%d].items[%d].name", i, j), x.Name, 1, MaxTalentShortTextRunes); err != nil {
				return err
			}
			if x.Quantity < 1 || x.PeriodCount < 1 || x.UnitRate < 1 || x.UnitRate > MaxTalentMoney {
				return fmt.Errorf("cost_groups[%d].items[%d]: invalid count or unit_rate", i, j)
			}
			if x.PeriodUnit != TalentPeriodMonth && x.PeriodUnit != TalentPeriodWorkday {
				return fmt.Errorf("cost_groups[%d].items[%d].period_unit: unsupported", i, j)
			}
			if err := talentText("quantity_unit", x.QuantityUnit, 1, MaxTalentShortTextRunes); err != nil {
				return err
			}
			if x.PeriodUnit == TalentPeriodWorkday && (x.UnitRate > MaxTalentMoney/int64(x.Quantity) || x.UnitRate*int64(x.Quantity)%20 != 0) {
				return fmt.Errorf("cost_groups[%d].items[%d]: calculation must produce whole rupiah", i, j)
			}
		}
	}
	return nil
}
func validateTalentManaged(in TalentOutsourcingInput) error {
	var prep int64
	for i, x := range in.PreparationItems {
		if err := talentText(fmt.Sprintf("preparation_items[%d].name", i), x.Name, 1, MaxTalentShortTextRunes); err != nil {
			return err
		}
		if x.Headcount < 1 || x.QuotedLineTotal <= 0 || x.QuotedLineTotal > MaxTalentMoney {
			return fmt.Errorf("preparation_items[%d]: invalid", i)
		}
		if prep > MaxTalentMoney-x.QuotedLineTotal {
			return fmt.Errorf("preparation_items: overflow")
		}
		prep += x.QuotedLineTotal
	}
	if in.PreparationDiscountAmount < 0 || in.PreparationDiscountAmount > prep {
		return fmt.Errorf("preparation_discount_amount: invalid")
	}
	if in.PreparationDiscountAmount > 0 && !in.ShowPreparationDiscountRow {
		return fmt.Errorf("show_preparation_discount_row: required for positive discount")
	}
	for i, x := range in.SquadItems {
		if err := talentText(fmt.Sprintf("squad_items[%d].role", i), x.Role, 1, MaxTalentShortTextRunes); err != nil {
			return err
		}
		if x.MonthlyRate <= 0 || x.MonthlyRate > MaxTalentMoney || x.Headcount < 1 || x.PeriodMonths < 1 {
			return fmt.Errorf("squad_items[%d]: invalid", i)
		}
	}
	return nil
}
func validateTalentLimitations(c []TalentLimitationCode) error {
	if len(c) == 0 || len(c) > 4 {
		return fmt.Errorf("limitation_codes: must contain between 1 and 4 codes")
	}
	seen := map[TalentLimitationCode]bool{}
	for i, x := range c {
		if x != TalentLimitOutOfPocket && x != TalentLimitOvertime && x != TalentLimitThirdParty && x != TalentLimitScopeChange || seen[x] {
			return fmt.Errorf("limitation_codes[%d]: duplicate or unsupported", i)
		}
		seen[x] = true
	}
	return nil
}

type TalentCostLineCalculation struct {
	TalentCostItemInput
	LineTotal int64 `json:"line_total"`
}
type TalentCostGroupCalculation struct {
	Category TalentCostCategory          `json:"category"`
	Title    string                      `json:"title"`
	Items    []TalentCostLineCalculation `json:"items"`
}
type TalentPreparationLineCalculation struct {
	TalentPreparationItemInput
	LineTotal int64 `json:"line_total"`
}
type TalentSquadLineCalculation struct {
	TalentSquadItemInput
	LineTotal int64 `json:"line_total"`
}
type TalentOutsourcingCalculation struct {
	EngagementMode            TalentEngagementMode               `json:"engagement_mode"`
	CostGroups                []TalentCostGroupCalculation       `json:"cost_groups,omitempty"`
	Preparation               []TalentPreparationLineCalculation `json:"preparation,omitempty"`
	Squad                     []TalentSquadLineCalculation       `json:"squad,omitempty"`
	PersonnelSubtotal         int64                              `json:"personnel_subtotal"`
	NonPersonnelSubtotal      int64                              `json:"non_personnel_subtotal"`
	PreparationSubtotal       int64                              `json:"preparation_subtotal"`
	PreparationDiscountAmount int64                              `json:"preparation_discount_amount"`
	SquadSubtotal             int64                              `json:"squad_subtotal"`
	TotalBeforeTax            int64                              `json:"total_before_tax"`
	DPPAmount                 int64                              `json:"dpp_amount"`
	TaxAmount                 int64                              `json:"tax_amount"`
	GrandTotal                int64                              `json:"grand_total"`
	PaymentTotal              int64                              `json:"payment_total"`
}
type TalentTimelineSnapshotItem struct {
	Title    string `json:"title"`
	Timeline string `json:"timeline"`
}
type ResolvedTalentOutsourcingProfile struct {
	EngagementMode          TalentEngagementMode `json:"engagement_mode"`
	TaxTreatment            TalentTaxTreatment   `json:"tax_treatment"`
	TaxClauseText           string               `json:"tax_clause_text"`
	ShowPreparation         bool                 `json:"show_preparation"`
	ShowPreparationDiscount bool                 `json:"show_preparation_discount"`
	ShowNonPersonnel        bool                 `json:"show_non_personnel"`
	ShowWarranty            bool                 `json:"show_warranty"`
	ShowRights              bool                 `json:"show_rights"`
	PaymentDueDays          int32                `json:"payment_due_days"`
	LimitationTexts         []string             `json:"limitation_texts"`
	Sections                []DocumentSection    `json:"sections"`
}
