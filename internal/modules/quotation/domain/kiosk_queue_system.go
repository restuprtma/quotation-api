package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	MaxKioskQueueShortTextRunes        = 200
	MaxKioskQueueLongTextRunes         = 4_000
	MaxKioskQueueDocumentNumberRunes   = 50
	MaxKioskQueueScopePhases           = 30
	MaxKioskQueueDeliverablesPerPhase  = 100
	MaxKioskQueueExpertRequirements    = 50
	MaxKioskQueueHeadcount             = 100
	MaxKioskQueuePeriodQuarters        = 240
	MaxKioskQueueServiceGroups         = 30
	MaxKioskQueueServiceItemsPerGroup  = 100
	MaxKioskQueueHardwareItems         = 100
	MaxKioskQueueHardwareQuantity      = 10_000
	MaxKioskQueueMaintenanceItems      = 50
	MaxKioskQueueTimelineRows          = 100
	MaxKioskQueueGovernmentPaymentRows = 60
	MaxKioskQueuePrivatePaymentRows    = 60
	MaxKioskQueuePaymentDueDays        = 365
	MaxKioskQueueAdditionalLimitations = 20
	MaxKioskQueueRightsStatements      = 20
)

type KioskQueueVariant string
type HardwareProvision string
type KioskQueueTaxTreatment string
type KioskQueueLimitationCode string

const (
	KioskQueueGovernmentMPP    KioskQueueVariant = "government_mpp"
	KioskQueuePrivateFoodCourt KioskQueueVariant = "private_food_court"

	HardwareVenturoSupplied HardwareProvision = "venturo_supplied"
	HardwareClientSupplied  HardwareProvision = "client_supplied"

	KioskTaxInclusivePPN11 KioskQueueTaxTreatment = "inclusive_ppn_11"
	KioskTaxExclusivePPN   KioskQueueTaxTreatment = "exclusive_ppn"
)

const (
	KioskLimitCivilElectrical  KioskQueueLimitationCode = "civil_electrical"
	KioskLimitClientHardware   KioskQueueLimitationCode = "client_hardware"
	KioskLimitExtraHardware    KioskQueueLimitationCode = "extra_hardware"
	KioskLimitHardwareWarranty KioskQueueLimitationCode = "hardware_warranty"
	KioskLimitOutOfPocket      KioskQueueLimitationCode = "out_of_pocket"
	KioskLimitOvertime         KioskQueueLimitationCode = "overtime"
	KioskLimitThirdParty       KioskQueueLimitationCode = "third_party"
	KioskLimitScopeChange      KioskQueueLimitationCode = "scope_change"

	KioskQueueMaxMoney int64 = 9_000_000_000_000_000
)

var governmentMPPNumberPattern = regexp.MustCompile(`^LANDA/MPP-Antrian/((0[1-9]|1[0-2])[0-9]{2})/[0-9]{3,6}$`)

var validKioskQueueLimitationCodes = map[KioskQueueLimitationCode]struct{}{
	KioskLimitCivilElectrical:  {},
	KioskLimitClientHardware:   {},
	KioskLimitExtraHardware:    {},
	KioskLimitHardwareWarranty: {},
	KioskLimitOutOfPocket:      {},
	KioskLimitOvertime:         {},
	KioskLimitThirdParty:       {},
	KioskLimitScopeChange:      {},
}

type KioskQueueScopePhaseInput struct {
	Title        string   `json:"title"`
	Deliverables []string `json:"deliverables"`
}

type KioskQueueExpertInput struct {
	Role                 string `json:"role"`
	Headcount            int32  `json:"headcount"`
	DurationLabel        string `json:"duration_label"`
	MinimumQualification string `json:"minimum_qualification"`
}

type KioskQueueServiceItemInput struct {
	Name           string `json:"name"`
	MonthlyRate    int64  `json:"monthly_rate"`
	Headcount      int32  `json:"headcount"`
	PeriodQuarters int32  `json:"period_quarters"`
}

type KioskQueueServiceGroupInput struct {
	Title string                       `json:"title"`
	Items []KioskQueueServiceItemInput `json:"items"`
}

type KioskQueueHardwareItemInput struct {
	Name      string `json:"name"`
	UnitPrice int64  `json:"unit_price"`
	Quantity  int32  `json:"quantity"`
}

type KioskQueueTimelineRowInput struct {
	Description string `json:"description"`
	Timeline    string `json:"timeline"`
}

type KioskQueueGovernmentPaymentInput struct {
	Stage        string `json:"stage"`
	BillingBasis string `json:"billing_basis"`
	Nominal      int64  `json:"nominal"`
}

type KioskQueuePrivatePaymentInput struct {
	WorkPeriod       string `json:"work_period"`
	Terms            string `json:"terms"`
	InvoiceDateLabel string `json:"invoice_date_label"`
	DueDateLabel     string `json:"due_date_label"`
	Nominal          int64  `json:"nominal"`
}

type KioskQueueSystemInput struct {
	Variant                   KioskQueueVariant                  `json:"variant"`
	DocumentNumber            string                             `json:"document_number"`
	SystemName                string                             `json:"system_name"`
	QuotationDate             time.Time                          `json:"quotation_date"`
	HardwareProvision         HardwareProvision                  `json:"hardware_provision"`
	TaxTreatment              KioskQueueTaxTreatment             `json:"tax_treatment"`
	GovernmentScopePhases     []KioskQueueScopePhaseInput        `json:"government_scope_phases,omitempty"`
	ExpertRequirements        []KioskQueueExpertInput            `json:"expert_requirements,omitempty"`
	ServiceGroups             []KioskQueueServiceGroupInput      `json:"service_groups"`
	ServiceDiscountAmount     int64                              `json:"service_discount_amount"`
	ShowServiceDiscountRow    bool                               `json:"show_service_discount_row"`
	HardwareItems             []KioskQueueHardwareItemInput      `json:"hardware_items,omitempty"`
	MaintenanceItems          []KioskQueueServiceItemInput       `json:"maintenance_items,omitempty"`
	PrivateTimelineRows       []KioskQueueTimelineRowInput       `json:"private_timeline_rows,omitempty"`
	GovernmentPaymentSchedule []KioskQueueGovernmentPaymentInput `json:"government_payment_schedule,omitempty"`
	PrivatePaymentSchedule    []KioskQueuePrivatePaymentInput    `json:"private_payment_schedule,omitempty"`
	PaymentDueDays            int32                              `json:"payment_due_days"`
	LimitationCodes           []KioskQueueLimitationCode         `json:"limitation_codes"`
	AdditionalLimitations     []string                           `json:"additional_limitations,omitempty"`
	RightsStatements          []string                           `json:"rights_statements,omitempty"`
}

type KioskQueueServiceLineCalculation struct {
	KioskQueueServiceItemInput
	LineTotal int64 `json:"line_total"`
}

type KioskQueueHardwareLineCalculation struct {
	KioskQueueHardwareItemInput
	LineTotal int64 `json:"line_total"`
}

type KioskQueueServiceGroupCalculation struct {
	Title string                             `json:"title"`
	Items []KioskQueueServiceLineCalculation `json:"items"`
}

type KioskQueueCalculation struct {
	Variant               KioskQueueVariant                   `json:"variant"`
	ServiceGroups         []KioskQueueServiceGroupCalculation `json:"service_groups"`
	ServiceSubtotal       int64                               `json:"service_subtotal"`
	ServiceDiscountAmount int64                               `json:"service_discount_amount"`
	ServiceTaxAmount      int64                               `json:"service_tax_amount"`
	ServiceTotal          int64                               `json:"service_total"`
	Hardware              []KioskQueueHardwareLineCalculation `json:"hardware,omitempty"`
	HardwareSubtotal      int64                               `json:"hardware_subtotal"`
	HardwareTaxAmount     int64                               `json:"hardware_tax_amount"`
	HardwareTotal         int64                               `json:"hardware_total"`
	Maintenance           []KioskQueueServiceLineCalculation  `json:"maintenance,omitempty"`
	MaintenanceTotal      int64                               `json:"maintenance_total"`
	PaymentTargetTotal    int64                               `json:"payment_target_total"`
	PaymentTotal          int64                               `json:"payment_total"`
}

func (in *KioskQueueSystemInput) UnmarshalJSON(data []byte) error {
	type inputAlias KioskQueueSystemInput
	var wire struct {
		*inputAlias
		QuotationDate json.RawMessage `json:"quotation_date"`
	}
	wire.inputAlias = (*inputAlias)(&KioskQueueSystemInput{})
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
	*in = KioskQueueSystemInput(*wire.inputAlias)
	return nil
}

func (in KioskQueueSystemInput) Normalize() KioskQueueSystemInput {
	out := in
	out.DocumentNumber = strings.TrimSpace(in.DocumentNumber)
	out.SystemName = strings.TrimSpace(in.SystemName)

	if in.GovernmentScopePhases != nil {
		out.GovernmentScopePhases = make([]KioskQueueScopePhaseInput, len(in.GovernmentScopePhases))
		for pIdx, p := range in.GovernmentScopePhases {
			phaseCopy := KioskQueueScopePhaseInput{
				Title: strings.TrimSpace(p.Title),
			}
			if p.Deliverables != nil {
				phaseCopy.Deliverables = make([]string, len(p.Deliverables))
				for dIdx, d := range p.Deliverables {
					phaseCopy.Deliverables[dIdx] = strings.TrimSpace(d)
				}
			}
			out.GovernmentScopePhases[pIdx] = phaseCopy
		}
	}

	if in.ExpertRequirements != nil {
		out.ExpertRequirements = make([]KioskQueueExpertInput, len(in.ExpertRequirements))
		for eIdx, e := range in.ExpertRequirements {
			out.ExpertRequirements[eIdx] = KioskQueueExpertInput{
				Role:                 strings.TrimSpace(e.Role),
				Headcount:            e.Headcount,
				DurationLabel:        strings.TrimSpace(e.DurationLabel),
				MinimumQualification: strings.TrimSpace(e.MinimumQualification),
			}
		}
	}

	if in.ServiceGroups != nil {
		out.ServiceGroups = make([]KioskQueueServiceGroupInput, len(in.ServiceGroups))
		for gIdx, g := range in.ServiceGroups {
			groupCopy := KioskQueueServiceGroupInput{
				Title: strings.TrimSpace(g.Title),
			}
			if g.Items != nil {
				groupCopy.Items = make([]KioskQueueServiceItemInput, len(g.Items))
				for iIdx, item := range g.Items {
					groupCopy.Items[iIdx] = KioskQueueServiceItemInput{
						Name:           strings.TrimSpace(item.Name),
						MonthlyRate:    item.MonthlyRate,
						Headcount:      item.Headcount,
						PeriodQuarters: item.PeriodQuarters,
					}
				}
			}
			out.ServiceGroups[gIdx] = groupCopy
		}
	}

	if in.HardwareItems != nil {
		out.HardwareItems = make([]KioskQueueHardwareItemInput, len(in.HardwareItems))
		for hIdx, h := range in.HardwareItems {
			out.HardwareItems[hIdx] = KioskQueueHardwareItemInput{
				Name:      strings.TrimSpace(h.Name),
				UnitPrice: h.UnitPrice,
				Quantity:  h.Quantity,
			}
		}
	}

	if in.MaintenanceItems != nil {
		out.MaintenanceItems = make([]KioskQueueServiceItemInput, len(in.MaintenanceItems))
		for mIdx, m := range in.MaintenanceItems {
			out.MaintenanceItems[mIdx] = KioskQueueServiceItemInput{
				Name:           strings.TrimSpace(m.Name),
				MonthlyRate:    m.MonthlyRate,
				Headcount:      m.Headcount,
				PeriodQuarters: m.PeriodQuarters,
			}
		}
	}

	if in.PrivateTimelineRows != nil {
		out.PrivateTimelineRows = make([]KioskQueueTimelineRowInput, len(in.PrivateTimelineRows))
		for rIdx, r := range in.PrivateTimelineRows {
			out.PrivateTimelineRows[rIdx] = KioskQueueTimelineRowInput{
				Description: strings.TrimSpace(r.Description),
				Timeline:    strings.TrimSpace(r.Timeline),
			}
		}
	}

	if in.GovernmentPaymentSchedule != nil {
		out.GovernmentPaymentSchedule = make([]KioskQueueGovernmentPaymentInput, len(in.GovernmentPaymentSchedule))
		for pIdx, p := range in.GovernmentPaymentSchedule {
			out.GovernmentPaymentSchedule[pIdx] = KioskQueueGovernmentPaymentInput{
				Stage:        strings.TrimSpace(p.Stage),
				BillingBasis: strings.TrimSpace(p.BillingBasis),
				Nominal:      p.Nominal,
			}
		}
	}

	if in.PrivatePaymentSchedule != nil {
		out.PrivatePaymentSchedule = make([]KioskQueuePrivatePaymentInput, len(in.PrivatePaymentSchedule))
		for pIdx, p := range in.PrivatePaymentSchedule {
			out.PrivatePaymentSchedule[pIdx] = KioskQueuePrivatePaymentInput{
				WorkPeriod:       strings.TrimSpace(p.WorkPeriod),
				Terms:            strings.TrimSpace(p.Terms),
				InvoiceDateLabel: strings.TrimSpace(p.InvoiceDateLabel),
				DueDateLabel:     strings.TrimSpace(p.DueDateLabel),
				Nominal:          p.Nominal,
			}
		}
	}

	if in.LimitationCodes != nil {
		out.LimitationCodes = make([]KioskQueueLimitationCode, len(in.LimitationCodes))
		copy(out.LimitationCodes, in.LimitationCodes)
	}

	if in.AdditionalLimitations != nil {
		out.AdditionalLimitations = make([]string, len(in.AdditionalLimitations))
		for aIdx, a := range in.AdditionalLimitations {
			out.AdditionalLimitations[aIdx] = strings.TrimSpace(a)
		}
	}

	if in.RightsStatements != nil {
		out.RightsStatements = make([]string, len(in.RightsStatements))
		for rIdx, r := range in.RightsStatements {
			out.RightsStatements[rIdx] = strings.TrimSpace(r)
		}
	}

	return out
}

func (in KioskQueueSystemInput) Validate() error {
	normalized := in.Normalize()

	if normalized.Variant != KioskQueueGovernmentMPP && normalized.Variant != KioskQueuePrivateFoodCourt {
		return fmt.Errorf("variant: unsupported value %q", in.Variant)
	}
	if err := validateKioskPlainText("system_name", normalized.SystemName, 1, MaxKioskQueueShortTextRunes); err != nil {
		return err
	}
	if normalized.QuotationDate.IsZero() {
		return fmt.Errorf("quotation_date: is required")
	}
	if err := validateKioskServiceGroups(normalized.ServiceGroups); err != nil {
		return err
	}
	if normalized.ServiceDiscountAmount < 0 {
		return fmt.Errorf("service_discount_amount: must be greater than or equal to zero")
	}
	if normalized.ServiceDiscountAmount > KioskQueueMaxMoney {
		return fmt.Errorf("service_discount_amount: exceeds maximum safe amount")
	}
	if normalized.ServiceDiscountAmount > 0 && !normalized.ShowServiceDiscountRow {
		return fmt.Errorf("show_service_discount_row: discount is positive but show_service_discount_row is false")
	}
	subtotal, err := kioskServiceSubtotal(normalized.ServiceGroups)
	if err != nil {
		return err
	}
	if normalized.ServiceDiscountAmount > subtotal {
		return fmt.Errorf("service_discount_amount: cannot exceed service subtotal")
	}

	if normalized.Variant == KioskQueueGovernmentMPP {
		return normalized.validateGovernment()
	}
	return normalized.validatePrivate()
}

func (in KioskQueueSystemInput) validateGovernment() error {
	if !governmentMPPNumberPattern.MatchString(in.DocumentNumber) {
		return fmt.Errorf("document_number: invalid government MPP format, must match LANDA/MPP-Antrian/MMYY/NNN")
	}
	if in.TaxTreatment != KioskTaxInclusivePPN11 {
		return fmt.Errorf("tax_treatment: government_mpp must use %q", KioskTaxInclusivePPN11)
	}
	if in.HardwareProvision != HardwareVenturoSupplied && in.HardwareProvision != HardwareClientSupplied {
		return fmt.Errorf("hardware_provision: unsupported value %q for government_mpp", in.HardwareProvision)
	}

	if len(in.GovernmentScopePhases) < 1 || len(in.GovernmentScopePhases) > MaxKioskQueueScopePhases {
		return fmt.Errorf("government_scope_phases: must contain between 1 and %d phases", MaxKioskQueueScopePhases)
	}
	for pIdx, phase := range in.GovernmentScopePhases {
		if err := validateKioskPlainText(fmt.Sprintf("government_scope_phases[%d].title", pIdx), phase.Title, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if len(phase.Deliverables) < 1 || len(phase.Deliverables) > MaxKioskQueueDeliverablesPerPhase {
			return fmt.Errorf("government_scope_phases[%d].deliverables: must contain between 1 and %d deliverables", pIdx, MaxKioskQueueDeliverablesPerPhase)
		}
		for dIdx, deliv := range phase.Deliverables {
			if err := validateKioskPlainText(fmt.Sprintf("government_scope_phases[%d].deliverables[%d]", pIdx, dIdx), deliv, 1, MaxKioskQueueLongTextRunes); err != nil {
				return err
			}
		}
	}

	if len(in.ExpertRequirements) < 1 || len(in.ExpertRequirements) > MaxKioskQueueExpertRequirements {
		return fmt.Errorf("expert_requirements: must contain between 1 and %d experts", MaxKioskQueueExpertRequirements)
	}
	for eIdx, exp := range in.ExpertRequirements {
		if err := validateKioskPlainText(fmt.Sprintf("expert_requirements[%d].role", eIdx), exp.Role, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if exp.Headcount < 1 || exp.Headcount > MaxKioskQueueHeadcount {
			return fmt.Errorf("expert_requirements[%d].headcount: must be between 1 and %d", eIdx, MaxKioskQueueHeadcount)
		}
		if err := validateKioskPlainText(fmt.Sprintf("expert_requirements[%d].duration_label", eIdx), exp.DurationLabel, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if err := validateKioskPlainText(fmt.Sprintf("expert_requirements[%d].minimum_qualification", eIdx), exp.MinimumQualification, 1, MaxKioskQueueLongTextRunes); err != nil {
			return err
		}
	}

	if in.HardwareProvision == HardwareVenturoSupplied {
		if len(in.HardwareItems) < 1 || len(in.HardwareItems) > MaxKioskQueueHardwareItems {
			return fmt.Errorf("hardware_items: must contain between 1 and %d items when hardware_provision is %q", MaxKioskQueueHardwareItems, HardwareVenturoSupplied)
		}
		var hardwareSubtotal int64
		for hIdx, item := range in.HardwareItems {
			if err := validateKioskPlainText(fmt.Sprintf("hardware_items[%d].name", hIdx), item.Name, 1, MaxKioskQueueShortTextRunes); err != nil {
				return err
			}
			if item.UnitPrice < 0 {
				return fmt.Errorf("hardware_items[%d].unit_price: must be greater than or equal to zero", hIdx)
			}
			if item.UnitPrice > KioskQueueMaxMoney {
				return fmt.Errorf("hardware_items[%d].unit_price: exceeds maximum safe amount", hIdx)
			}
			if item.Quantity < 1 || item.Quantity > MaxKioskQueueHardwareQuantity {
				return fmt.Errorf("hardware_items[%d].quantity: must be between 1 and %d", hIdx, MaxKioskQueueHardwareQuantity)
			}
			if item.Quantity > 0 && item.UnitPrice > KioskQueueMaxMoney/int64(item.Quantity) {
				return fmt.Errorf("hardware_items[%d]: line total exceeds maximum safe amount", hIdx)
			}
			lineTotal := item.UnitPrice * int64(item.Quantity)
			if lineTotal > KioskQueueMaxMoney-hardwareSubtotal {
				return fmt.Errorf("hardware_items: subtotal exceeds maximum safe amount")
			}
			hardwareSubtotal += lineTotal
		}
	} else {
		if len(in.HardwareItems) > 0 {
			return fmt.Errorf("hardware_items: must be empty when hardware_provision is %q", HardwareClientSupplied)
		}
	}

	if len(in.MaintenanceItems) > 0 {
		return fmt.Errorf("maintenance_items: must be empty for government_mpp")
	}
	if len(in.PrivateTimelineRows) > 0 {
		return fmt.Errorf("private_timeline_rows: must be empty for government_mpp")
	}
	if len(in.PrivatePaymentSchedule) > 0 {
		return fmt.Errorf("private_payment_schedule: must be empty for government_mpp")
	}

	if len(in.GovernmentPaymentSchedule) < 1 || len(in.GovernmentPaymentSchedule) > MaxKioskQueueGovernmentPaymentRows {
		return fmt.Errorf("government_payment_schedule: must contain between 1 and %d rows", MaxKioskQueueGovernmentPaymentRows)
	}
	var paymentTotal int64
	for pIdx, p := range in.GovernmentPaymentSchedule {
		if err := validateKioskPlainText(fmt.Sprintf("government_payment_schedule[%d].stage", pIdx), p.Stage, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if err := validateKioskPlainText(fmt.Sprintf("government_payment_schedule[%d].billing_basis", pIdx), p.BillingBasis, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if p.Nominal <= 0 {
			return fmt.Errorf("government_payment_schedule[%d].nominal: must be greater than zero", pIdx)
		}
		if p.Nominal > KioskQueueMaxMoney {
			return fmt.Errorf("government_payment_schedule[%d].nominal: exceeds maximum safe amount", pIdx)
		}
		if p.Nominal > KioskQueueMaxMoney-paymentTotal {
			return fmt.Errorf("government_payment_schedule: total exceeds maximum safe amount")
		}
		paymentTotal += p.Nominal
	}

	if in.PaymentDueDays < 1 || in.PaymentDueDays > MaxKioskQueuePaymentDueDays {
		return fmt.Errorf("payment_due_days: must be between 1 and %d", MaxKioskQueuePaymentDueDays)
	}

	requiredLimits := []KioskQueueLimitationCode{KioskLimitCivilElectrical}
	if in.HardwareProvision == HardwareClientSupplied {
		requiredLimits = append(requiredLimits, KioskLimitClientHardware)
	}
	if err := validateKioskLimitations(in.LimitationCodes, requiredLimits); err != nil {
		return err
	}

	if err := validateKioskAdditionalLimitations(in.AdditionalLimitations); err != nil {
		return err
	}
	return validateKioskRightsStatements(in.RightsStatements)
}

func (in KioskQueueSystemInput) validatePrivate() error {
	if err := validateKioskPlainText("document_number", in.DocumentNumber, 1, MaxKioskQueueDocumentNumberRunes); err != nil {
		return err
	}
	if in.TaxTreatment != KioskTaxExclusivePPN {
		return fmt.Errorf("tax_treatment: private_food_court must use %q", KioskTaxExclusivePPN)
	}
	if in.HardwareProvision != HardwareClientSupplied {
		return fmt.Errorf("hardware_provision: private_food_court must use %q", HardwareClientSupplied)
	}

	if len(in.GovernmentScopePhases) > 0 {
		return fmt.Errorf("government_scope_phases: must be empty for private_food_court")
	}
	if len(in.ExpertRequirements) > 0 {
		return fmt.Errorf("expert_requirements: must be empty for private_food_court")
	}
	if len(in.HardwareItems) > 0 {
		return fmt.Errorf("hardware_items: must be empty for private_food_court")
	}
	if len(in.GovernmentPaymentSchedule) > 0 {
		return fmt.Errorf("government_payment_schedule: must be empty for private_food_court")
	}

	if len(in.MaintenanceItems) > MaxKioskQueueMaintenanceItems {
		return fmt.Errorf("maintenance_items: must contain at most %d items", MaxKioskQueueMaintenanceItems)
	}
	var maintenanceSubtotal int64
	for mIdx, item := range in.MaintenanceItems {
		if err := validateKioskPlainText(fmt.Sprintf("maintenance_items[%d].name", mIdx), item.Name, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if item.MonthlyRate < 0 {
			return fmt.Errorf("maintenance_items[%d].monthly_rate: must be greater than or equal to zero", mIdx)
		}
		if item.MonthlyRate > KioskQueueMaxMoney {
			return fmt.Errorf("maintenance_items[%d].monthly_rate: exceeds maximum safe amount", mIdx)
		}
		if item.Headcount < 1 || item.Headcount > MaxKioskQueueHeadcount {
			return fmt.Errorf("maintenance_items[%d].headcount: must be between 1 and %d", mIdx, MaxKioskQueueHeadcount)
		}
		if item.PeriodQuarters < 1 || item.PeriodQuarters > MaxKioskQueuePeriodQuarters {
			return fmt.Errorf("maintenance_items[%d].period_quarters: must be between 1 and %d", mIdx, MaxKioskQueuePeriodQuarters)
		}
		multiplier := int64(item.Headcount) * int64(item.PeriodQuarters)
		if multiplier > 0 && item.MonthlyRate > (4*KioskQueueMaxMoney)/multiplier {
			return fmt.Errorf("maintenance_items[%d]: line total exceeds maximum safe amount", mIdx)
		}
		product := item.MonthlyRate * multiplier
		if product%4 != 0 {
			return fmt.Errorf("maintenance_items[%d]: line total must be a whole integer rupiah", mIdx)
		}
		lineTotal := product / 4
		if lineTotal > KioskQueueMaxMoney-maintenanceSubtotal {
			return fmt.Errorf("maintenance_items: subtotal exceeds maximum safe amount")
		}
		maintenanceSubtotal += lineTotal
	}

	if len(in.PrivateTimelineRows) < 1 || len(in.PrivateTimelineRows) > MaxKioskQueueTimelineRows {
		return fmt.Errorf("private_timeline_rows: must contain between 1 and %d rows", MaxKioskQueueTimelineRows)
	}
	for rIdx, row := range in.PrivateTimelineRows {
		if err := validateKioskPlainText(fmt.Sprintf("private_timeline_rows[%d].description", rIdx), row.Description, 1, MaxKioskQueueLongTextRunes); err != nil {
			return err
		}
		if err := validateKioskPlainText(fmt.Sprintf("private_timeline_rows[%d].timeline", rIdx), row.Timeline, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
	}

	if len(in.PrivatePaymentSchedule) < 1 || len(in.PrivatePaymentSchedule) > MaxKioskQueuePrivatePaymentRows {
		return fmt.Errorf("private_payment_schedule: must contain between 1 and %d rows", MaxKioskQueuePrivatePaymentRows)
	}
	var paymentTotal int64
	for pIdx, row := range in.PrivatePaymentSchedule {
		if err := validateKioskPlainText(fmt.Sprintf("private_payment_schedule[%d].work_period", pIdx), row.WorkPeriod, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if err := validateKioskPlainText(fmt.Sprintf("private_payment_schedule[%d].terms", pIdx), row.Terms, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if err := validateKioskPlainText(fmt.Sprintf("private_payment_schedule[%d].invoice_date_label", pIdx), row.InvoiceDateLabel, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if err := validateKioskPlainText(fmt.Sprintf("private_payment_schedule[%d].due_date_label", pIdx), row.DueDateLabel, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if row.Nominal <= 0 {
			return fmt.Errorf("private_payment_schedule[%d].nominal: must be greater than zero", pIdx)
		}
		if row.Nominal > KioskQueueMaxMoney {
			return fmt.Errorf("private_payment_schedule[%d].nominal: exceeds maximum safe amount", pIdx)
		}
		if row.Nominal > KioskQueueMaxMoney-paymentTotal {
			return fmt.Errorf("private_payment_schedule: total exceeds maximum safe amount")
		}
		paymentTotal += row.Nominal
	}

	if in.PaymentDueDays != 0 {
		return fmt.Errorf("payment_due_days: must be 0 for private_food_court")
	}

	requiredLimits := []KioskQueueLimitationCode{KioskLimitClientHardware}
	if err := validateKioskLimitations(in.LimitationCodes, requiredLimits); err != nil {
		return err
	}

	if err := validateKioskAdditionalLimitations(in.AdditionalLimitations); err != nil {
		return err
	}
	return validateKioskRightsStatements(in.RightsStatements)
}

func validateKioskServiceGroups(groups []KioskQueueServiceGroupInput) error {
	if len(groups) < 1 || len(groups) > MaxKioskQueueServiceGroups {
		return fmt.Errorf("service_groups: must contain between 1 and %d groups", MaxKioskQueueServiceGroups)
	}
	for gIdx, group := range groups {
		if err := validateKioskPlainText(fmt.Sprintf("service_groups[%d].title", gIdx), group.Title, 1, MaxKioskQueueShortTextRunes); err != nil {
			return err
		}
		if len(group.Items) < 1 || len(group.Items) > MaxKioskQueueServiceItemsPerGroup {
			return fmt.Errorf("service_groups[%d].items: must contain between 1 and %d items", gIdx, MaxKioskQueueServiceItemsPerGroup)
		}
		for iIdx, item := range group.Items {
			if err := validateKioskPlainText(fmt.Sprintf("service_groups[%d].items[%d].name", gIdx, iIdx), item.Name, 1, MaxKioskQueueShortTextRunes); err != nil {
				return err
			}
			if item.MonthlyRate < 0 {
				return fmt.Errorf("service_groups[%d].items[%d].monthly_rate: must be greater than or equal to zero", gIdx, iIdx)
			}
			if item.MonthlyRate > KioskQueueMaxMoney {
				return fmt.Errorf("service_groups[%d].items[%d].monthly_rate: exceeds maximum safe amount", gIdx, iIdx)
			}
			if item.Headcount < 1 || item.Headcount > MaxKioskQueueHeadcount {
				return fmt.Errorf("service_groups[%d].items[%d].headcount: must be between 1 and %d", gIdx, iIdx, MaxKioskQueueHeadcount)
			}
			if item.PeriodQuarters < 1 || item.PeriodQuarters > MaxKioskQueuePeriodQuarters {
				return fmt.Errorf("service_groups[%d].items[%d].period_quarters: must be between 1 and %d", gIdx, iIdx, MaxKioskQueuePeriodQuarters)
			}
			multiplier := int64(item.Headcount) * int64(item.PeriodQuarters)
			if multiplier > 0 && item.MonthlyRate > (4*KioskQueueMaxMoney)/multiplier {
				return fmt.Errorf("service_groups[%d].items[%d]: line total exceeds maximum safe amount", gIdx, iIdx)
			}
			product := item.MonthlyRate * multiplier
			if product%4 != 0 {
				return fmt.Errorf("service_groups[%d].items[%d]: line total must be a whole integer rupiah", gIdx, iIdx)
			}
			lineTotal := product / 4
			if lineTotal > KioskQueueMaxMoney {
				return fmt.Errorf("service_groups[%d].items[%d]: line total exceeds maximum safe amount", gIdx, iIdx)
			}
		}
	}
	return nil
}

func kioskServiceSubtotal(groups []KioskQueueServiceGroupInput) (int64, error) {
	var total int64
	for gIdx, group := range groups {
		for iIdx, item := range group.Items {
			multiplier := int64(item.Headcount) * int64(item.PeriodQuarters)
			if multiplier > 0 && item.MonthlyRate > (4*KioskQueueMaxMoney)/multiplier {
				return 0, fmt.Errorf("service_groups[%d].items[%d]: line total exceeds maximum safe amount", gIdx, iIdx)
			}
			lineTotal := (item.MonthlyRate * multiplier) / 4
			if lineTotal > KioskQueueMaxMoney-total {
				return 0, fmt.Errorf("service_groups: subtotal exceeds maximum safe amount")
			}
			total += lineTotal
		}
	}
	return total, nil
}

func validateKioskLimitations(codes []KioskQueueLimitationCode, required []KioskQueueLimitationCode) error {
	if len(codes) > len(validKioskQueueLimitationCodes) {
		return fmt.Errorf("limitation_codes: must contain at most %d codes", len(validKioskQueueLimitationCodes))
	}
	seen := make(map[KioskQueueLimitationCode]struct{}, len(codes))
	for i, code := range codes {
		if _, ok := validKioskQueueLimitationCodes[code]; !ok {
			return fmt.Errorf("limitation_codes[%d]: unsupported limitation %q", i, code)
		}
		if _, exists := seen[code]; exists {
			return fmt.Errorf("limitation_codes[%d]: duplicate limitation code %q", i, code)
		}
		seen[code] = struct{}{}
	}
	for _, req := range required {
		if _, exists := seen[req]; !exists {
			return fmt.Errorf("limitation_codes: missing required limitation %q", req)
		}
	}
	return nil
}

func validateKioskPlainText(path, value string, minRunes, maxRunes int) error {
	if strings.ContainsAny(value, "<>") {
		return fmt.Errorf("%s: cannot contain HTML characters (< or >)", path)
	}
	runes := len([]rune(value))
	if minRunes > 0 && runes < minRunes {
		return fmt.Errorf("%s: is required", path)
	}
	if maxRunes > 0 && runes > maxRunes {
		return fmt.Errorf("%s: must be at most %d characters", path, maxRunes)
	}
	return nil
}

func validateKioskAdditionalLimitations(limitations []string) error {
	if len(limitations) > MaxKioskQueueAdditionalLimitations {
		return fmt.Errorf("additional_limitations: must contain at most %d limitations", MaxKioskQueueAdditionalLimitations)
	}
	seen := make(map[string]struct{}, len(limitations))
	for i, stmt := range limitations {
		if err := validateKioskPlainText(fmt.Sprintf("additional_limitations[%d]", i), stmt, 1, MaxKioskQueueLongTextRunes); err != nil {
			return err
		}
		key := strings.ToLower(stmt)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("additional_limitations[%d]: duplicate additional limitation %q", i, stmt)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateKioskRightsStatements(statements []string) error {
	if len(statements) > MaxKioskQueueRightsStatements {
		return fmt.Errorf("rights_statements: must contain at most %d statements", MaxKioskQueueRightsStatements)
	}
	for i, stmt := range statements {
		if err := validateKioskPlainText(fmt.Sprintf("rights_statements[%d]", i), stmt, 1, MaxKioskQueueLongTextRunes); err != nil {
			return err
		}
	}
	return nil
}
