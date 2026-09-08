package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxSubscriptionTermMonths           = 60
	MaxSubscriptionPricePeriods         = 10
	MaxSubscriptionOneTimeItems         = 20
	MaxSubscriptionPaymentRows          = 60
	MaxSubscriptionFeatureGroups        = 30
	MaxSubscriptionFeaturesPerGroup     = 50
	MaxSubscriptionSafeAmount           = 9_000_000_000_000_000
	MaxSubscriptionLabelRunes           = 200
	MaxSubscriptionEligibleRevenueRunes = 2_000
)

const (
	RightsClauseVenturoSoftwareOwnershipV1 = "venturo_software_ownership_v1"
	SubscriptionLegalCatalogVersion        = "subscription_saas_legal_v1"
)

type SubscriptionSectionLayout string

const (
	SubscriptionLayoutSubscriptionDevelopment SubscriptionSectionLayout = "subscription_development"
	SubscriptionLayoutDevelopmentMaintenance  SubscriptionSectionLayout = "development_maintenance"
)

func (l SubscriptionSectionLayout) valid() bool {
	return l == SubscriptionLayoutSubscriptionDevelopment || l == SubscriptionLayoutDevelopmentMaintenance
}

type SubscriptionBillingCycle string

const (
	SubscriptionBillingMonthly   SubscriptionBillingCycle = "monthly"
	SubscriptionBillingQuarterly SubscriptionBillingCycle = "quarterly"
	SubscriptionBillingYearly    SubscriptionBillingCycle = "yearly"
)

func (c SubscriptionBillingCycle) valid() bool {
	return c == SubscriptionBillingMonthly || c == SubscriptionBillingQuarterly || c == SubscriptionBillingYearly
}

type SubscriptionTaxTreatment string

const (
	SubscriptionTaxNotStated SubscriptionTaxTreatment = "not_stated"
	SubscriptionTaxExclusive SubscriptionTaxTreatment = "exclusive_ppn"
	SubscriptionTaxInclusive SubscriptionTaxTreatment = "inclusive_ppn"
)

func (t SubscriptionTaxTreatment) valid() bool {
	return t == SubscriptionTaxNotStated || t == SubscriptionTaxExclusive || t == SubscriptionTaxInclusive
}

type SubscriptionPricePeriodInput struct {
	Label            string `json:"label"`
	StartMonthOffset int32  `json:"start_month_offset"`
	DurationMonths   int32  `json:"duration_months"`
	MonthlyRate      int64  `json:"monthly_rate"`
}

type SubscriptionOneTimeItemInput struct {
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
}

type SubscriptionPaymentRowInput struct {
	WorkPeriod       string `json:"work_period"`
	StartMonthOffset int32  `json:"start_month_offset"`
	CoveredMonths    int32  `json:"covered_months"`
	InvoiceDateLabel string `json:"invoice_date_label"`
	DueDateLabel     string `json:"due_date_label"`
	Nominal          int64  `json:"nominal"`
}

type SubscriptionFeatureGroupInput struct {
	Name     string   `json:"name"`
	Features []string `json:"features"`
}

type IncomeSharingInput struct {
	VenturoShareBPS            int32  `json:"venturo_share_bps"`
	PartnerShareBPS            int32  `json:"partner_share_bps"`
	EligibleRevenueDescription string `json:"eligible_revenue_description"`
	SettlementBusinessDays     int32  `json:"settlement_business_days"`
	ProhibitUnilateralDiscount bool   `json:"prohibit_unilateral_discount"`
}

type SubscriptionSaaSInput struct {
	ProductName       string                          `json:"product_name"`
	QuotationDate     time.Time                       `json:"quotation_date"`
	SectionLayout     SubscriptionSectionLayout       `json:"section_layout"`
	BillingCycle      SubscriptionBillingCycle        `json:"billing_cycle"`
	TaxTreatment      SubscriptionTaxTreatment        `json:"tax_treatment"`
	InitialTermMonths int32                           `json:"initial_term_months"`
	AutoRenewal       bool                            `json:"auto_renewal"`
	PricePeriods      []SubscriptionPricePeriodInput  `json:"price_periods"`
	OneTimeItems      []SubscriptionOneTimeItemInput  `json:"one_time_items,omitempty"`
	PaymentSchedule   []SubscriptionPaymentRowInput   `json:"payment_schedule"`
	FeatureGroups     []SubscriptionFeatureGroupInput `json:"feature_groups,omitempty"`
	RightsClauseCode  string                          `json:"rights_clause_code,omitempty"`
	IsPartnership     bool                            `json:"is_partnership"`
	IncomeSharing     *IncomeSharingInput             `json:"income_sharing,omitempty"`
}

func (in *SubscriptionSaaSInput) UnmarshalJSON(data []byte) error {
	type inputAlias SubscriptionSaaSInput
	var wire struct {
		*inputAlias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.inputAlias = (*inputAlias)(&SubscriptionSaaSInput{})
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
	*in = SubscriptionSaaSInput(*wire.inputAlias)
	return nil
}

func (in SubscriptionSaaSInput) Validate() error {
	if strings.TrimSpace(in.ProductName) == "" {
		return fmt.Errorf("product_name: is required")
	}
	if len([]rune(strings.TrimSpace(in.ProductName))) > MaxSubscriptionLabelRunes {
		return fmt.Errorf("product_name: must be at most %d characters", MaxSubscriptionLabelRunes)
	}
	if in.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	if !in.SectionLayout.valid() {
		return fmt.Errorf("section_layout: unsupported value %q", in.SectionLayout)
	}
	if !in.BillingCycle.valid() {
		return fmt.Errorf("billing_cycle: unsupported value %q", in.BillingCycle)
	}
	if !in.TaxTreatment.valid() {
		return fmt.Errorf("tax_treatment: unsupported value %q", in.TaxTreatment)
	}
	if in.InitialTermMonths < 1 || in.InitialTermMonths > MaxSubscriptionTermMonths {
		return fmt.Errorf("initial_term_months: must be between 1 and %d", MaxSubscriptionTermMonths)
	}
	if err := validateSubscriptionPriceCoverage(in.PricePeriods, in.InitialTermMonths); err != nil {
		return err
	}
	if err := validateSubscriptionOneTimeItems(in.OneTimeItems); err != nil {
		return err
	}
	if err := validateSubscriptionPaymentCoverage(in.PaymentSchedule, in.InitialTermMonths, in.BillingCycle); err != nil {
		return err
	}
	if err := validateSubscriptionFeatureGroups(in.FeatureGroups); err != nil {
		return err
	}
	if in.SectionLayout == SubscriptionLayoutDevelopmentMaintenance && len(in.FeatureGroups) == 0 {
		return fmt.Errorf("feature_groups: is required for development_maintenance layout")
	}
	if in.IsPartnership {
		if in.IncomeSharing == nil {
			return fmt.Errorf("income_sharing: is required when is_partnership is true")
		}
		if err := validateIncomeSharing(*in.IncomeSharing); err != nil {
			return err
		}
	} else if in.IncomeSharing != nil {
		return fmt.Errorf("income_sharing: must be empty when is_partnership is false")
	}
	return validateSubscriptionRightsCode(in.RightsClauseCode)
}

func validateSubscriptionPriceCoverage(periods []SubscriptionPricePeriodInput, initialTermMonths int32) error {
	if len(periods) == 0 {
		return fmt.Errorf("price_periods: must contain at least one period")
	}
	if len(periods) > MaxSubscriptionPricePeriods {
		return fmt.Errorf("price_periods: must contain at most %d periods", MaxSubscriptionPricePeriods)
	}
	var currentOffset int64
	for i, period := range periods {
		if l := len([]rune(strings.TrimSpace(period.Label))); l == 0 || l > MaxSubscriptionLabelRunes {
			return fmt.Errorf("price_periods[%d].label: must be between 1 and %d characters", i, MaxSubscriptionLabelRunes)
		}
		if period.DurationMonths <= 0 {
			return fmt.Errorf("price_periods[%d].duration_months: must be greater than zero", i)
		}
		if period.DurationMonths > initialTermMonths {
			return fmt.Errorf("price_periods[%d].duration_months: cannot exceed initial term (%d months)", i, initialTermMonths)
		}
		if period.MonthlyRate <= 0 {
			return fmt.Errorf("price_periods[%d].monthly_rate: must be greater than zero", i)
		}
		if period.MonthlyRate > MaxSubscriptionSafeAmount {
			return fmt.Errorf("price_periods[%d].monthly_rate: exceeds maximum safe amount", i)
		}
		if int64(period.StartMonthOffset) != currentOffset {
			return fmt.Errorf("price_periods[%d].start_month_offset: must be contiguous and start at 0, expected %d got %d", i, currentOffset, period.StartMonthOffset)
		}
		currentOffset += int64(period.DurationMonths)
	}
	if currentOffset != int64(initialTermMonths) {
		return fmt.Errorf("price_periods: total duration (%d months) must equal initial term (%d months)", currentOffset, initialTermMonths)
	}
	return nil
}

func validateSubscriptionOneTimeItems(items []SubscriptionOneTimeItemInput) error {
	if len(items) > MaxSubscriptionOneTimeItems {
		return fmt.Errorf("one_time_items: must contain at most %d items", MaxSubscriptionOneTimeItems)
	}
	for i, item := range items {
		if l := len([]rune(strings.TrimSpace(item.Name))); l == 0 || l > MaxSubscriptionLabelRunes {
			return fmt.Errorf("one_time_items[%d].name: must be between 1 and %d characters", i, MaxSubscriptionLabelRunes)
		}
		if item.Amount <= 0 {
			return fmt.Errorf("one_time_items[%d].amount: must be greater than zero", i)
		}
		if item.Amount > MaxSubscriptionSafeAmount {
			return fmt.Errorf("one_time_items[%d].amount: exceeds maximum safe amount", i)
		}
	}
	return nil
}

func validateSubscriptionPaymentCoverage(schedule []SubscriptionPaymentRowInput, initialTermMonths int32, cycle SubscriptionBillingCycle) error {
	if len(schedule) == 0 {
		return fmt.Errorf("payment_schedule: must contain at least one payment row")
	}
	if len(schedule) > MaxSubscriptionPaymentRows {
		return fmt.Errorf("payment_schedule: must contain at most %d payment rows", MaxSubscriptionPaymentRows)
	}
	var currentOffset int32
	for i, row := range schedule {
		if l := len([]rune(strings.TrimSpace(row.WorkPeriod))); l == 0 || l > MaxSubscriptionLabelRunes {
			return fmt.Errorf("payment_schedule[%d].work_period: must be between 1 and %d characters", i, MaxSubscriptionLabelRunes)
		}
		if l := len([]rune(strings.TrimSpace(row.InvoiceDateLabel))); l == 0 || l > MaxSubscriptionLabelRunes {
			return fmt.Errorf("payment_schedule[%d].invoice_date_label: must be between 1 and %d characters", i, MaxSubscriptionLabelRunes)
		}
		if l := len([]rune(strings.TrimSpace(row.DueDateLabel))); l == 0 || l > MaxSubscriptionLabelRunes {
			return fmt.Errorf("payment_schedule[%d].due_date_label: must be between 1 and %d characters", i, MaxSubscriptionLabelRunes)
		}
		if row.Nominal <= 0 {
			return fmt.Errorf("payment_schedule[%d].nominal: must be greater than zero", i)
		}
		if row.Nominal > MaxSubscriptionSafeAmount {
			return fmt.Errorf("payment_schedule[%d].nominal: exceeds maximum safe amount", i)
		}
		if row.CoveredMonths <= 0 {
			return fmt.Errorf("payment_schedule[%d].covered_months: must be greater than zero", i)
		}
		if row.StartMonthOffset != currentOffset {
			return fmt.Errorf("payment_schedule[%d].start_month_offset: must be contiguous and start at 0, expected %d got %d", i, currentOffset, row.StartMonthOffset)
		}
		switch cycle {
		case SubscriptionBillingMonthly:
			if row.CoveredMonths != 1 {
				return fmt.Errorf("payment_schedule[%d].covered_months: must be 1 for monthly billing cycle", i)
			}
		case SubscriptionBillingQuarterly:
			if i < len(schedule)-1 {
				if row.CoveredMonths != 3 {
					return fmt.Errorf("payment_schedule[%d].covered_months: must be 3 for quarterly billing cycle", i)
				}
			} else {
				if row.CoveredMonths < 1 || row.CoveredMonths > 3 {
					return fmt.Errorf("payment_schedule[%d].covered_months: must be between 1 and 3 for quarterly billing cycle", i)
				}
			}
		case SubscriptionBillingYearly:
			if i < len(schedule)-1 {
				if row.CoveredMonths != 12 {
					return fmt.Errorf("payment_schedule[%d].covered_months: must be 12 for yearly billing cycle", i)
				}
			} else {
				if row.CoveredMonths < 1 || row.CoveredMonths > 12 {
					return fmt.Errorf("payment_schedule[%d].covered_months: must be between 1 and 12 for yearly billing cycle", i)
				}
			}
		}
		currentOffset += row.CoveredMonths
	}
	if currentOffset != initialTermMonths {
		return fmt.Errorf("payment_schedule: total covered months (%d) must equal initial term (%d)", currentOffset, initialTermMonths)
	}
	return nil
}

func validateSubscriptionFeatureGroups(groups []SubscriptionFeatureGroupInput) error {
	if len(groups) > MaxSubscriptionFeatureGroups {
		return fmt.Errorf("feature_groups: must contain at most %d groups", MaxSubscriptionFeatureGroups)
	}
	for i, g := range groups {
		if l := len([]rune(strings.TrimSpace(g.Name))); l == 0 || l > MaxSubscriptionLabelRunes {
			return fmt.Errorf("feature_groups[%d].name: must be between 1 and %d characters", i, MaxSubscriptionLabelRunes)
		}
		if len(g.Features) == 0 {
			return fmt.Errorf("feature_groups[%d].features: must contain at least one feature", i)
		}
		if len(g.Features) > MaxSubscriptionFeaturesPerGroup {
			return fmt.Errorf("feature_groups[%d].features: must contain at most %d features", i, MaxSubscriptionFeaturesPerGroup)
		}
		for j, f := range g.Features {
			if l := len([]rune(strings.TrimSpace(f))); l == 0 || l > MaxSubscriptionLabelRunes {
				return fmt.Errorf("feature_groups[%d].features[%d]: must be between 1 and %d characters", i, j, MaxSubscriptionLabelRunes)
			}
		}
	}
	return nil
}

func validateIncomeSharing(sharing IncomeSharingInput) error {
	if sharing.VenturoShareBPS < 0 || sharing.PartnerShareBPS < 0 {
		return fmt.Errorf("income_sharing: shares must be non-negative")
	}
	if sharing.VenturoShareBPS+sharing.PartnerShareBPS != 10_000 {
		return fmt.Errorf("income_sharing: shares must total 10000 basis points")
	}
	desc := strings.TrimSpace(sharing.EligibleRevenueDescription)
	if l := len([]rune(desc)); l == 0 || l > MaxSubscriptionEligibleRevenueRunes {
		return fmt.Errorf("income_sharing.eligible_revenue_description: must be between 1 and %d characters", MaxSubscriptionEligibleRevenueRunes)
	}
	if sharing.SettlementBusinessDays <= 0 {
		return fmt.Errorf("income_sharing.settlement_business_days: must be greater than zero")
	}
	if sharing.SettlementBusinessDays > 365 {
		return fmt.Errorf("income_sharing.settlement_business_days: cannot exceed 365 days")
	}
	return nil
}

func validateSubscriptionRightsCode(code string) error {
	if code != "" && code != RightsClauseVenturoSoftwareOwnershipV1 {
		return fmt.Errorf("rights_clause_code: unsupported value %q", code)
	}
	return nil
}

type ResolvedSubscriptionSaaSProfile struct {
	SectionLayout       SubscriptionSectionLayout `json:"section_layout"`
	BillingCycle        SubscriptionBillingCycle  `json:"billing_cycle"`
	TaxTreatment        SubscriptionTaxTreatment  `json:"tax_treatment"`
	TaxClauseText       string                    `json:"tax_clause_text,omitempty"`
	RightsClauseCode    string                    `json:"rights_clause_code,omitempty"`
	RightsClauseText    string                    `json:"rights_clause_text,omitempty"`
	IncomeSharingText   string                    `json:"income_sharing_text,omitempty"`
	AutoRenewalText     string                    `json:"auto_renewal_text,omitempty"`
	LegalCatalogVersion string                    `json:"legal_catalog_version"`
	HasFeatures         bool                      `json:"has_features,omitempty"`
}

func (p ResolvedSubscriptionSaaSProfile) HasRights() bool {
	return p.RightsClauseCode != "" || p.RightsClauseText != "" || p.IncomeSharingText != ""
}

type SubscriptionPricePeriodCalculation struct {
	SubscriptionPricePeriodInput
	Subtotal int64 `json:"subtotal"`
}

type SubscriptionSaaSCalculation struct {
	PricePeriods   []SubscriptionPricePeriodCalculation `json:"price_periods"`
	OneTimeItems   []SubscriptionOneTimeItemInput       `json:"one_time_items,omitempty"`
	RecurringTotal int64                                `json:"recurring_total"`
	OneTimeTotal   int64                                `json:"one_time_total"`
	TotalContract  int64                                `json:"total_contract"`
	Payments       []SubscriptionPaymentRowInput        `json:"payments"`
}
