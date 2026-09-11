package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type CloudPackageCategory string
type CloudTaxTreatment string
type CloudPaymentComponent string
type CloudLimitationCode string

const (
	MaxCloudMoney                                   = int64(9_000_000_000_000_000)
	MaxCloudShortTextRunes                          = 200
	MaxCloudLongTextRunes                           = 4_000
	MaxCloudPackageItems                            = 100
	MaxCloudSetupItems                              = 30
	MaxCloudScopeRows                               = 30
	MaxCloudScopeSections                           = 10
	MaxCloudScopeActivities                         = 30
	MaxCloudPaymentRows                             = 120
	CloudPackageSharedHosting CloudPackageCategory  = "shared_hosting"
	CloudPackageVPS           CloudPackageCategory  = "vps_cloud"
	CloudTaxExclusive         CloudTaxTreatment     = "exclusive_ppn"
	CloudTaxInclusive         CloudTaxTreatment     = "inclusive_ppn"
	CloudTaxNotStated         CloudTaxTreatment     = "not_stated"
	CloudPaymentSetup         CloudPaymentComponent = "setup"
	CloudPaymentMonthly       CloudPaymentComponent = "monthly_service"
	CloudLimitOutOfPocket     CloudLimitationCode   = "out_of_pocket"
	CloudLimitOvertime        CloudLimitationCode   = "overtime"
	CloudLimitThirdParty      CloudLimitationCode   = "third_party"
	CloudLimitScopeChange     CloudLimitationCode   = "scope_change"
)

type CloudPackageItemInput struct {
	Category  CloudPackageCategory `json:"category"`
	Name      string               `json:"name"`
	VCPU      int32                `json:"vcpu"`
	RAMGB     int32                `json:"ram_gb"`
	StorageGB int32                `json:"storage_gb"`
	UnitRate  int64                `json:"unit_rate"`
}
type CloudSetupItemInput struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}
type CloudScopeSectionInput struct {
	Title      string   `json:"title"`
	Activities []string `json:"activities"`
}
type CloudScopeRowInput struct {
	Sections []CloudScopeSectionInput `json:"sections"`
	Timeline string                   `json:"timeline"`
}
type CloudPaymentRowInput struct {
	WorkPeriod       string                `json:"work_period"`
	Terms            string                `json:"terms"`
	InvoiceDateLabel string                `json:"invoice_date_label"`
	DueDateLabel     string                `json:"due_date_label"`
	Component        CloudPaymentComponent `json:"component"`
	Nominal          int64                 `json:"nominal"`
}
type CloudInfrastructureInput struct {
	DocumentNumber        string                  `json:"document_number"`
	ServiceName           string                  `json:"service_name"`
	QuotationDate         time.Time               `json:"quotation_date"`
	TaxTreatment          CloudTaxTreatment       `json:"tax_treatment"`
	PackageItems          []CloudPackageItemInput `json:"package_items"`
	SetupItems            []CloudSetupItemInput   `json:"setup_items,omitempty"`
	ScopeRows             []CloudScopeRowInput    `json:"scope_rows,omitempty"`
	PaymentSchedule       []CloudPaymentRowInput  `json:"payment_schedule"`
	PaymentDueDays        int32                   `json:"payment_due_days"`
	LimitationCodes       []CloudLimitationCode   `json:"limitation_codes"`
	AdditionalLimitations []string                `json:"additional_limitations,omitempty"`
}

func (in *CloudInfrastructureInput) UnmarshalJSON(data []byte) error {
	type alias CloudInfrastructureInput
	var wire struct {
		*alias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.alias = (*alias)(&CloudInfrastructureInput{})
	if err := decodeStrictJSON(data, &wire); err != nil {
		return err
	}
	d, err := decodeOptionalDate(wire.QuotationDate)
	if err != nil || d == nil || d.IsZero() {
		return fmt.Errorf("quotation_date: is required and must use YYYY-MM-DD")
	}
	wire.alias.QuotationDate = *d
	*in = CloudInfrastructureInput(*wire.alias)
	return nil
}

func (in CloudInfrastructureInput) Normalize() CloudInfrastructureInput {
	out := in
	out.DocumentNumber = strings.TrimSpace(in.DocumentNumber)
	out.ServiceName = strings.TrimSpace(in.ServiceName)
	out.PackageItems = append([]CloudPackageItemInput(nil), in.PackageItems...)
	for i := range out.PackageItems {
		out.PackageItems[i].Name = strings.TrimSpace(out.PackageItems[i].Name)
	}
	out.SetupItems = append([]CloudSetupItemInput(nil), in.SetupItems...)
	for i := range out.SetupItems {
		out.SetupItems[i].Name = strings.TrimSpace(out.SetupItems[i].Name)
	}
	out.ScopeRows = make([]CloudScopeRowInput, len(in.ScopeRows))
	for i, row := range in.ScopeRows {
		out.ScopeRows[i].Timeline = strings.TrimSpace(row.Timeline)
		out.ScopeRows[i].Sections = make([]CloudScopeSectionInput, len(row.Sections))
		for j, s := range row.Sections {
			out.ScopeRows[i].Sections[j].Title = strings.TrimSpace(s.Title)
			out.ScopeRows[i].Sections[j].Activities = make([]string, len(s.Activities))
			for k, a := range s.Activities {
				out.ScopeRows[i].Sections[j].Activities[k] = strings.TrimSpace(a)
			}
		}
	}
	out.PaymentSchedule = append([]CloudPaymentRowInput(nil), in.PaymentSchedule...)
	for i := range out.PaymentSchedule {
		p := &out.PaymentSchedule[i]
		p.WorkPeriod = strings.TrimSpace(p.WorkPeriod)
		p.Terms = strings.TrimSpace(p.Terms)
		p.InvoiceDateLabel = strings.TrimSpace(p.InvoiceDateLabel)
		p.DueDateLabel = strings.TrimSpace(p.DueDateLabel)
	}
	out.LimitationCodes = append([]CloudLimitationCode(nil), in.LimitationCodes...)
	out.AdditionalLimitations = make([]string, len(in.AdditionalLimitations))
	for i, v := range in.AdditionalLimitations {
		out.AdditionalLimitations[i] = strings.TrimSpace(v)
	}
	return out
}

func containsCloudHTMLLike(value string) bool {
	return strings.Contains(value, "<") || strings.Contains(value, ">")
}
func validateCloudPlainText(path, value string, minRunes, maxRunes int) error {
	if containsCloudHTMLLike(value) {
		return fmt.Errorf("%s: HTML-like input is not allowed", path)
	}
	n := len([]rune(value))
	if n < minRunes {
		return fmt.Errorf("%s: is required", path)
	}
	if n > maxRunes {
		return fmt.Errorf("%s: must be at most %d characters", path, maxRunes)
	}
	return nil
}
func validateCloudMoney(path string, value int64) error {
	if value < 1 || value > MaxCloudMoney {
		return fmt.Errorf("%s: must be between 1 and %d", path, MaxCloudMoney)
	}
	return nil
}

func (in CloudInfrastructureInput) Validate() error {
	in = in.Normalize()
	if err := validateCloudPlainText("document_number", in.DocumentNumber, 1, 50); err != nil {
		return err
	}
	if err := validateCloudPlainText("service_name", in.ServiceName, 1, MaxCloudShortTextRunes); err != nil {
		return err
	}
	if in.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	if in.TaxTreatment != CloudTaxExclusive && in.TaxTreatment != CloudTaxInclusive && in.TaxTreatment != CloudTaxNotStated {
		return fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}
	if len(in.PackageItems) < 1 || len(in.PackageItems) > MaxCloudPackageItems {
		return fmt.Errorf("package_items: must contain between 1 and %d items", MaxCloudPackageItems)
	}
	for i, x := range in.PackageItems {
		p := fmt.Sprintf("package_items[%d]", i)
		if x.Category != CloudPackageSharedHosting && x.Category != CloudPackageVPS {
			return fmt.Errorf("%s.category: unsupported value %q", p, x.Category)
		}
		if err := validateCloudPlainText(p+".name", x.Name, 1, MaxCloudShortTextRunes); err != nil {
			return err
		}
		for _, s := range []struct {
			n string
			v int32
		}{{"vcpu", x.VCPU}, {"ram_gb", x.RAMGB}, {"storage_gb", x.StorageGB}} {
			if s.v < 1 || s.v > 1000 {
				return fmt.Errorf("%s.%s: must be between 1 and 1000", p, s.n)
			}
		}
		if err := validateCloudMoney(p+".unit_rate", x.UnitRate); err != nil {
			return err
		}
	}
	if len(in.SetupItems) > MaxCloudSetupItems {
		return fmt.Errorf("setup_items: must contain at most %d items", MaxCloudSetupItems)
	}
	for i, x := range in.SetupItems {
		p := fmt.Sprintf("setup_items[%d]", i)
		if err := validateCloudPlainText(p+".name", x.Name, 1, MaxCloudShortTextRunes); err != nil {
			return err
		}
		if err := validateCloudMoney(p+".amount", x.Amount); err != nil {
			return err
		}
	}
	if len(in.ScopeRows) > MaxCloudScopeRows {
		return fmt.Errorf("scope_rows: must contain at most %d rows", MaxCloudScopeRows)
	}
	for i, row := range in.ScopeRows {
		p := fmt.Sprintf("scope_rows[%d]", i)
		if len(row.Sections) < 1 || len(row.Sections) > MaxCloudScopeSections {
			return fmt.Errorf("%s.sections: must contain between 1 and %d sections", p, MaxCloudScopeSections)
		}
		for j, s := range row.Sections {
			q := fmt.Sprintf("%s.sections[%d]", p, j)
			if err := validateCloudPlainText(q+".title", s.Title, 1, MaxCloudShortTextRunes); err != nil {
				return err
			}
			if len(s.Activities) < 1 || len(s.Activities) > MaxCloudScopeActivities {
				return fmt.Errorf("%s.activities: must contain between 1 and %d activities", q, MaxCloudScopeActivities)
			}
			for k, a := range s.Activities {
				if err := validateCloudPlainText(fmt.Sprintf("%s.activities[%d]", q, k), a, 1, MaxCloudLongTextRunes); err != nil {
					return err
				}
			}
		}
		if err := validateCloudPlainText(p+".timeline", row.Timeline, 1, MaxCloudShortTextRunes); err != nil {
			return err
		}
	}
	if len(in.PaymentSchedule) < 1 || len(in.PaymentSchedule) > MaxCloudPaymentRows {
		return fmt.Errorf("payment_schedule: must contain between 1 and %d rows", MaxCloudPaymentRows)
	}
	for i, x := range in.PaymentSchedule {
		p := fmt.Sprintf("payment_schedule[%d]", i)
		for _, v := range []struct{ n, s string }{{"work_period", x.WorkPeriod}, {"terms", x.Terms}, {"invoice_date_label", x.InvoiceDateLabel}, {"due_date_label", x.DueDateLabel}} {
			if err := validateCloudPlainText(p+"."+v.n, v.s, 1, MaxCloudShortTextRunes); err != nil {
				return err
			}
		}
		if x.Component != CloudPaymentSetup && x.Component != CloudPaymentMonthly {
			return fmt.Errorf("%s.component: unsupported value %q", p, x.Component)
		}
		if err := validateCloudMoney(p+".nominal", x.Nominal); err != nil {
			return err
		}
	}
	if in.PaymentDueDays < 1 || in.PaymentDueDays > 365 {
		return fmt.Errorf("payment_due_days: must be between 1 and 365")
	}
	if len(in.LimitationCodes) < 1 || len(in.LimitationCodes) > 4 {
		return fmt.Errorf("limitation_codes: must contain between 1 and 4 codes")
	}
	seen := map[CloudLimitationCode]bool{}
	for i, c := range in.LimitationCodes {
		if c != CloudLimitOutOfPocket && c != CloudLimitOvertime && c != CloudLimitThirdParty && c != CloudLimitScopeChange {
			return fmt.Errorf("limitation_codes[%d]: unsupported value %q", i, c)
		}
		if seen[c] {
			return fmt.Errorf("limitation_codes[%d]: duplicate value %q", i, c)
		}
		seen[c] = true
	}
	for i, x := range in.AdditionalLimitations {
		if err := validateCloudPlainText(fmt.Sprintf("additional_limitations[%d]", i), x, 1, MaxCloudLongTextRunes); err != nil {
			return err
		}
	}
	return nil
}

type CloudPackageLineCalculation struct {
	CloudPackageItemInput
	LineTotal int64 `json:"line_total"`
}
type CloudSetupLineCalculation struct {
	CloudSetupItemInput
	LineTotal int64 `json:"line_total"`
}
type CloudInfrastructureCalculation struct {
	Packages                []CloudPackageLineCalculation `json:"packages"`
	Setup                   []CloudSetupLineCalculation   `json:"setup,omitempty"`
	MonthlySubtotal         int64                         `json:"monthly_subtotal"`
	SetupSubtotal           int64                         `json:"setup_subtotal"`
	SetupPaymentTotal       int64                         `json:"setup_payment_total"`
	MonthlyPaymentRows      int32                         `json:"monthly_payment_rows"`
	ScheduledRecurringTotal int64                         `json:"scheduled_recurring_total"`
	PaymentTotal            int64                         `json:"payment_total"`
}
type ResolvedCloudInfrastructureProfile struct {
	TaxTreatment  CloudTaxTreatment `json:"tax_treatment"`
	TaxClauseText string            `json:"tax_clause_text,omitempty"`
	ShowSetup     bool              `json:"show_setup"`
	ShowScope     bool              `json:"show_scope"`
	Sections      []DocumentSection `json:"sections"`
}
