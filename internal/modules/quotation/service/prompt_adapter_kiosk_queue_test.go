package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validGovernmentHardwareKioskQueueState() domain.PromptQuotationState {
	input := PromptKioskQueueServiceInput{
		Kind:              PromptKioskQueueServiceInputKind,
		Variant:           factString(string(domain.KioskQueueGovernmentMPP), domain.SourceMeeting),
		DocumentNumber:    ptrFact("LANDA/MPP-Antrian/0726/001", domain.SourceMeeting),
		SystemName:        factString("Sistem Antrian Terpadu MPP", domain.SourceMeeting),
		HardwareProvision: factString(string(domain.HardwareVenturoSupplied), domain.SourceMeeting),
		TaxTreatment:      factString(string(domain.KioskTaxInclusivePPN11), domain.SourceMeeting),
		GovernmentScopePhases: []PromptKioskQueueScopePhase{
			{
				Title: factString("Fase 1: Persiapan & Desain", domain.SourceMeeting),
				Deliverables: []domain.Fact[string]{
					factString("Dokumen Analisis Kebutuhan", domain.SourceMeeting),
					factString("UI/UX Design Mockup", domain.SourceMeeting),
				},
			},
		},
		ExpertRequirements: []PromptKioskQueueExpertItem{
			{
				Role:                 factString("Project Manager", domain.SourceMeeting),
				Headcount:            factInt(1, domain.SourceMeeting),
				DurationLabel:        factString("3 Bulan", domain.SourceMeeting),
				MinimumQualification: factString("S1 Sistem Informasi, pengalaman 5 tahun", domain.SourceMeeting),
			},
		},
		ServiceGroups: []PromptKioskQueueServiceGroup{
			{
				Title: factString("Requirement, Design & Development", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("UI UX", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-UIUX"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(1, domain.SourceMeeting)},
					{Name: factString("Senior Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(16_170_000, domain.SourceCatalog, "RATE-SR-PROG"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(3, domain.SourceMeeting)},
				},
			},
			{
				Title: factString("SIT, UAT & Go-Live Beta Release", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("Senior Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(16_170_000, domain.SourceCatalog, "RATE-SR-PROG"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(2, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(2, domain.SourceMeeting)},
				},
			},
		},
		HardwareItems: []PromptKioskQueueHardwareItem{
			{Name: factString("Mesin Kiosk Touchscreen 21.5 inch", domain.SourceMeeting), UnitPrice: moneyInput(24_500_000, domain.SourceCatalog, "HW-KIOSK"), Quantity: factInt(1, domain.SourceMeeting)},
			{Name: factString("Printer Thermal & Audio Master", domain.SourceMeeting), UnitPrice: moneyInput(4_500_000, domain.SourceCatalog, "HW-PRINTER"), Quantity: factInt(1, domain.SourceMeeting)},
			{Name: factString("TV Display 43 Inch", domain.SourceMeeting), UnitPrice: moneyInput(3_500_000, domain.SourceCatalog, "HW-TV"), Quantity: factInt(3, domain.SourceMeeting)},
			{Name: factString("Bracket TV", domain.SourceMeeting), UnitPrice: moneyInput(300_000, domain.SourceCatalog, "HW-BRACKET"), Quantity: factInt(3, domain.SourceMeeting)},
			{Name: factString("HDMI Switch", domain.SourceMeeting), UnitPrice: moneyInput(100_000, domain.SourceCatalog, "HW-HDMI"), Quantity: factInt(1, domain.SourceMeeting)},
			{Name: factString("Mini PC Core i5", domain.SourceMeeting), UnitPrice: moneyInput(6_000_000, domain.SourceCatalog, "HW-MINIPC"), Quantity: factInt(1, domain.SourceMeeting)},
		},
		GovernmentPaymentSchedule: []PromptKioskQueueGovernmentPaymentRow{
			{
				Stage:        factString("Uang Muka 50%", domain.SourceMeeting),
				BillingBasis: factString("Kontrak ditandatangani & SPMK terbit", domain.SourceMeeting),
				Nominal:      moneyInput(51_615_000, domain.SourceUser, ""),
			},
			{
				Stage:        factString("Pelunasan 50%", domain.SourceMeeting),
				BillingBasis: factString("Pekerjaan selesai 100%\nBerita Acara Serah Terima (BAST)", domain.SourceMeeting),
				Nominal:      moneyInput(49_184_100, domain.SourceUser, ""),
			},
		},
		PaymentDueDays: ptrFactInt(14, domain.SourceMeeting),
		LimitationCodes: []domain.Fact[string]{
			factString(string(domain.KioskLimitCivilElectrical), domain.SourceMeeting),
		},
		AdditionalLimitations: []domain.Fact[string]{
			factString("Pekerjaan sipil dan perkabelan tidak termasuk penawaran ini.", domain.SourceMeeting),
		},
	}

	raw, err := EncodePromptKioskQueueServiceInput(input)
	if err != nil {
		panic(err)
	}

	return domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Pengadaan Kiosk Antrian MPP dengan hardware dan instalasi lengkap.",
		ClientName:       ptrFact("Dinas Penanaman Modal", domain.SourceUser),
		CommercialScheme: ptrFact(domain.CommercialSchemeKioskQueueSystem, domain.SourceUser),
		ProjectName:      ptrFact("Sistem Antrian Terpadu MPP", domain.SourceUser),
		ServiceInput:     raw,
	}
}

func validGovernmentServiceOnlyKioskQueueState() domain.PromptQuotationState {
	input := PromptKioskQueueServiceInput{
		Kind:              PromptKioskQueueServiceInputKind,
		Variant:           factString(string(domain.KioskQueueGovernmentMPP), domain.SourceMeeting),
		DocumentNumber:    ptrFact("LANDA/MPP-Antrian/0726/002", domain.SourceMeeting),
		SystemName:        factString("Sistem Antrian Terpadu MPP (Jasa Saja)", domain.SourceMeeting),
		HardwareProvision: factString(string(domain.HardwareClientSupplied), domain.SourceMeeting),
		TaxTreatment:      factString(string(domain.KioskTaxInclusivePPN11), domain.SourceMeeting),
		GovernmentScopePhases: []PromptKioskQueueScopePhase{
			{
				Title: factString("Fase 1: Persiapan & Desain", domain.SourceMeeting),
				Deliverables: []domain.Fact[string]{
					factString("Dokumen Analisis Kebutuhan", domain.SourceMeeting),
					factString("UI/UX Design Mockup", domain.SourceMeeting),
				},
			},
		},
		ExpertRequirements: []PromptKioskQueueExpertItem{
			{
				Role:                 factString("Project Manager", domain.SourceMeeting),
				Headcount:            factInt(1, domain.SourceMeeting),
				DurationLabel:        factString("3 Bulan", domain.SourceMeeting),
				MinimumQualification: factString("S1 Sistem Informasi, pengalaman 5 tahun", domain.SourceMeeting),
			},
		},
		ServiceGroups: []PromptKioskQueueServiceGroup{
			{
				Title: factString("Requirement, Design & Development", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("UI UX", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-UIUX"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(1, domain.SourceMeeting)},
					{Name: factString("Senior Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(16_170_000, domain.SourceCatalog, "RATE-SR-PROG"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Intermediate Web Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-MID-PROG"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(3, domain.SourceMeeting)},
				},
			},
			{
				Title: factString("SIT, UAT & Go-Live Beta Release", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("Senior Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(16_170_000, domain.SourceCatalog, "RATE-SR-PROG"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(2, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(2, domain.SourceMeeting)},
				},
			},
		},
		GovernmentPaymentSchedule: []PromptKioskQueueGovernmentPaymentRow{
			{
				Stage:        factString("Uang Muka 50%", domain.SourceMeeting),
				BillingBasis: factString("Kontrak ditandatangani & SPMK terbit", domain.SourceMeeting),
				Nominal:      moneyInput(32_012_400, domain.SourceUser, ""),
			},
			{
				Stage:        factString("Pelunasan 50%", domain.SourceMeeting),
				BillingBasis: factString("Pekerjaan selesai 100%\nBerita Acara Serah Terima (BAST)", domain.SourceMeeting),
				Nominal:      moneyInput(32_012_400, domain.SourceUser, ""),
			},
		},
		PaymentDueDays: ptrFactInt(14, domain.SourceMeeting),
		LimitationCodes: []domain.Fact[string]{
			factString(string(domain.KioskLimitCivilElectrical), domain.SourceMeeting),
			factString(string(domain.KioskLimitClientHardware), domain.SourceMeeting),
			factString(string(domain.KioskLimitHardwareWarranty), domain.SourceMeeting),
		},
	}

	raw, err := EncodePromptKioskQueueServiceInput(input)
	if err != nil {
		panic(err)
	}

	return domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Pengadaan Kiosk Antrian MPP jasa pengembangan saja.",
		ClientName:       ptrFact("Dinas Penanaman Modal", domain.SourceUser),
		CommercialScheme: ptrFact(domain.CommercialSchemeKioskQueueSystem, domain.SourceUser),
		ProjectName:      ptrFact("Sistem Antrian Terpadu MPP (Jasa Saja)", domain.SourceUser),
		ServiceInput:     raw,
	}
}

func validPrivateFoodCourtKioskQueueState() domain.PromptQuotationState {
	input := PromptKioskQueueServiceInput{
		Kind:              PromptKioskQueueServiceInputKind,
		Variant:           factString(string(domain.KioskQueuePrivateFoodCourt), domain.SourceMeeting),
		DocumentNumber:    ptrFact("VEN/Kiosk-FoodCourt/0826/007", domain.SourceMeeting),
		SystemName:        factString("Self-Ordering Kiosk Food Court", domain.SourceMeeting),
		HardwareProvision: factString(string(domain.HardwareClientSupplied), domain.SourceMeeting),
		TaxTreatment:      factString(string(domain.KioskTaxExclusivePPN), domain.SourceMeeting),
		ServiceGroups: []PromptKioskQueueServiceGroup{
			{
				Title: factString("Engineering Kick-off", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("Senior Web Developer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-WEB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Senior Mobile Developer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-MOB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("UI/UX Designer", domain.SourceMeeting), MonthlyRate: moneyInput(9_550_000, domain.SourceCatalog, "RATE-UIUX"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(9_550_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
				},
			},
			{
				Title: factString("Requirement & Design", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("System Analyst", domain.SourceMeeting), MonthlyRate: moneyInput(13_370_000, domain.SourceCatalog, "RATE-SA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
				},
			},
			{
				Title: factString("Development", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("Senior Web Developer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-WEB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(9_550_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Senior Mobile Developer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-MOB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Frontend Developer", domain.SourceMeeting), MonthlyRate: moneyInput(9_550_000, domain.SourceCatalog, "RATE-FE"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
				},
			},
			{
				Title: factString("SIT, UAT & Go-Live", domain.SourceMeeting),
				Items: []PromptKioskQueueServiceItem{
					{Name: factString("Senior Web Developer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-WEB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Senior Mobile Developer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-MOB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("UI/UX Designer", domain.SourceMeeting), MonthlyRate: moneyInput(9_550_000, domain.SourceCatalog, "RATE-UIUX"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(4, domain.SourceMeeting)},
					{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(16_170_000, domain.SourceCatalog, "RATE-SR-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(2, domain.SourceMeeting)},
				},
			},
		},
		MaintenanceItems: []PromptKioskQueueServiceItem{
			{Name: factString("Senior Web Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-WEB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(12, domain.SourceMeeting)},
			{Name: factString("Senior Mobile Programmer", domain.SourceMeeting), MonthlyRate: moneyInput(11_550_000, domain.SourceCatalog, "RATE-SR-MOB"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(12, domain.SourceMeeting)},
			{Name: factString("Quality Assurance", domain.SourceMeeting), MonthlyRate: moneyInput(9_550_000, domain.SourceCatalog, "RATE-QA"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(6, domain.SourceMeeting)},
		},
		PrivateTimelineRows: []PromptKioskQueueTimelineRow{
			{Description: factString("Kick Off & Design", domain.SourceMeeting), Timeline: factString("Bulan 1", domain.SourceMeeting)},
			{Description: factString("Development Core", domain.SourceMeeting), Timeline: factString("Bulan 2", domain.SourceMeeting)},
			{Description: factString("Integration & Testing", domain.SourceMeeting), Timeline: factString("Bulan 3", domain.SourceMeeting)},
		},
		PrivatePaymentSchedule: []PromptKioskQueuePrivatePaymentRow{
			{WorkPeriod: factString("Termin 1", domain.SourceMeeting), Terms: factString("DP Kick Off", domain.SourceMeeting), InvoiceDateLabel: factString("01/08/2026", domain.SourceMeeting), DueDateLabel: factString("15/08/2026", domain.SourceMeeting), Nominal: moneyInput(6_685_000, domain.SourceUser, "")},
			{WorkPeriod: factString("Termin 2", domain.SourceMeeting), Terms: factString("Desain Selesai", domain.SourceMeeting), InvoiceDateLabel: factString("01/09/2026", domain.SourceMeeting), DueDateLabel: factString("15/09/2026", domain.SourceMeeting), Nominal: moneyInput(6_685_000, domain.SourceUser, "")},
			{WorkPeriod: factString("Termin 3", domain.SourceMeeting), Terms: factString("Development", domain.SourceMeeting), InvoiceDateLabel: factString("01/10/2026", domain.SourceMeeting), DueDateLabel: factString("15/10/2026", domain.SourceMeeting), Nominal: moneyInput(42_200_000, domain.SourceUser, "")},
			{WorkPeriod: factString("Termin 4", domain.SourceMeeting), Terms: factString("Pelunasan UAT", domain.SourceMeeting), InvoiceDateLabel: factString("01/11/2026", domain.SourceMeeting), DueDateLabel: factString("15/11/2026", domain.SourceMeeting), Nominal: moneyInput(40_735_000, domain.SourceUser, "")},
		},
		LimitationCodes: []domain.Fact[string]{
			factString(string(domain.KioskLimitClientHardware), domain.SourceMeeting),
		},
	}

	raw, err := EncodePromptKioskQueueServiceInput(input)
	if err != nil {
		panic(err)
	}

	return domain.PromptQuotationState{
		SchemaVersion:    domain.SchemaPromptQuotationV1,
		MeetingNotes:     "Self-ordering kiosk food court dengan SLA maintenance terpisah.",
		ClientName:       ptrFact("Food Court Management", domain.SourceUser),
		CommercialScheme: ptrFact(domain.CommercialSchemeKioskQueueSystem, domain.SourceUser),
		ProjectName:      ptrFact("Self-Ordering Kiosk Food Court", domain.SourceUser),
		ServiceInput:     raw,
	}
}

func ptrFactInt(value int, source domain.ValueSource) *domain.Fact[int] {
	fact := factInt(value, source)
	return &fact
}

func blockingKioskQueueIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	blocking := make([]domain.DraftIssue, 0)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			blocking = append(blocking, issue)
		}
	}
	return blocking
}

func hasKioskQueueIssue(issues []domain.DraftIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func mustPromptKioskQueueServiceInput(t *testing.T, raw []byte) PromptKioskQueueServiceInput {
	t.Helper()
	input, err := DecodePromptKioskQueueServiceInput(raw)
	require.NoError(t, err)
	return input
}

func mustPromptKioskQueueJSON(t *testing.T, input PromptKioskQueueServiceInput) []byte {
	t.Helper()
	raw, err := EncodePromptKioskQueueServiceInput(input)
	require.NoError(t, err)
	return raw
}

func TestBuildPromptKioskQueueInput_GovernmentHardware(t *testing.T) {
	state := validGovernmentHardwareKioskQueueState()
	quotationDate := time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)

	input, issues := BuildPromptKioskQueueInput(&state, quotationDate)

	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.KioskQueueGovernmentMPP, input.Variant)
	assert.Equal(t, "LANDA/MPP-Antrian/0726/001", input.DocumentNumber)
	assert.Equal(t, "Sistem Antrian Terpadu MPP", input.SystemName)
	assert.Equal(t, domain.HardwareVenturoSupplied, input.HardwareProvision)
	assert.Equal(t, domain.KioskTaxInclusivePPN11, input.TaxTreatment)
	assert.Equal(t, int32(14), input.PaymentDueDays)
	require.Len(t, input.GovernmentScopePhases, 1)
	require.Len(t, input.ExpertRequirements, 1)
	require.Len(t, input.ServiceGroups, 2)
	require.Len(t, input.HardwareItems, 6)
	require.Len(t, input.GovernmentPaymentSchedule, 2)
	assert.Equal(t, int64(24_500_000), input.HardwareItems[0].UnitPrice)
	assert.Equal(t, int32(1), input.HardwareItems[0].Quantity)
}

func TestBuildPromptKioskQueueInput_GovernmentServiceOnly(t *testing.T) {
	state := validGovernmentServiceOnlyKioskQueueState()
	quotationDate := time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)

	input, issues := BuildPromptKioskQueueInput(&state, quotationDate)

	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.KioskQueueGovernmentMPP, input.Variant)
	assert.Equal(t, domain.HardwareClientSupplied, input.HardwareProvision)
	assert.Empty(t, input.HardwareItems)
	require.Len(t, input.ServiceGroups, 2)
}

func TestBuildPromptKioskQueueInput_PrivateFoodCourt(t *testing.T) {
	state := validPrivateFoodCourtKioskQueueState()
	quotationDate := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	input, issues := BuildPromptKioskQueueInput(&state, quotationDate)

	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.KioskQueuePrivateFoodCourt, input.Variant)
	assert.Equal(t, domain.HardwareClientSupplied, input.HardwareProvision)
	assert.Equal(t, domain.KioskTaxExclusivePPN, input.TaxTreatment)
	assert.Equal(t, int32(0), input.PaymentDueDays)
	require.Len(t, input.ServiceGroups, 4)
	require.Len(t, input.MaintenanceItems, 3)
	require.Len(t, input.PrivateTimelineRows, 3)
	require.Len(t, input.PrivatePaymentSchedule, 4)
	assert.Empty(t, input.HardwareItems)
}

func TestCalculatePromptKioskQueue_GovernmentHardware(t *testing.T) {
	state := validGovernmentHardwareKioskQueueState()
	quotationDate := time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)
	_, calc, profile, issues, err := CalculatePromptKioskQueue(&state, quotationDate)
	require.NoError(t, err)
	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.EqualValues(t, 44_310_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 4_874_100, calc.ServiceTaxAmount)
	assert.EqualValues(t, 49_184_100, calc.ServiceTotal)
	assert.EqualValues(t, 46_500_000, calc.HardwareSubtotal)
	assert.EqualValues(t, 5_115_000, calc.HardwareTaxAmount)
	assert.EqualValues(t, 51_615_000, calc.HardwareTotal)
	assert.EqualValues(t, 100_799_100, calc.PaymentTargetTotal)
	assert.EqualValues(t, 100_799_100, calc.PaymentTotal)
	assert.True(t, profile.ShowHardware)
	assert.False(t, profile.ShowMaintenance)
}

func TestCalculatePromptKioskQueue_GovernmentServiceOnly(t *testing.T) {
	state := validGovernmentServiceOnlyKioskQueueState()
	quotationDate := time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)

	_, calc, profile, issues, err := CalculatePromptKioskQueue(&state, quotationDate)
	require.NoError(t, err)
	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.EqualValues(t, 57_680_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 6_344_800, calc.ServiceTaxAmount)
	assert.EqualValues(t, 64_024_800, calc.ServiceTotal)
	assert.EqualValues(t, 64_024_800, calc.PaymentTargetTotal)
	assert.Zero(t, calc.HardwareTotal)
	assert.False(t, profile.ShowHardware)
}

func TestCalculatePromptKioskQueue_AdditionalDiscountBPS(t *testing.T) {
	state := validGovernmentHardwareKioskQueueState()
	discountBPS := int32(1000) // 10%
	state.AdditionalDiscountBPS = &discountBPS

	serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
	serviceInput.GovernmentPaymentSchedule = []PromptKioskQueueGovernmentPaymentRow{
		{Stage: factString("DP", domain.SourceMeeting), BillingBasis: factString("SPMK", domain.SourceMeeting), Nominal: moneyInput(51_615_000, domain.SourceUser, "")},
		{Stage: factString("Pelunasan", domain.SourceMeeting), BillingBasis: factString("BAST", domain.SourceMeeting), Nominal: moneyInput(44_753_100, domain.SourceUser, "")},
	}
	state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

	_, calc, _, issues, err := CalculatePromptKioskQueue(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, blockingKioskQueueIssues(issues))
	assert.EqualValues(t, 4_431_000, calc.ServiceDiscountAmount)
	assert.EqualValues(t, 44_753_100, calc.ServiceTotal)
	assert.EqualValues(t, 96_368_100, calc.PaymentTargetTotal)
}

func TestCalculatePromptKioskQueue_PrivateFoodCourt_MaintenanceExcludedFromPaymentTarget(t *testing.T) {
	state := validPrivateFoodCourtKioskQueueState()
	// Total service subtotal: 138,505,000. Set discount BPS to arrive at 96,305,000 if needed,
	// or adjust payment schedule to match subtotal.
	// In the fixture, Payment schedule sum is 96,305,000 (which was after 42,200,000 discount).
	// Let's test discount calculation with AdditionalDiscountBPS or payment match.
	quotationDate := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

	// If discount is not set and payment is 96,305,000 while subtotal is 138,505,000, payment mismatch is emitted.
	// When payment schedule matches service total:
	serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
	serviceInput.PrivatePaymentSchedule = []PromptKioskQueuePrivatePaymentRow{
		{WorkPeriod: factString("Termin 1", domain.SourceMeeting), Terms: factString("DP Kick Off", domain.SourceMeeting), InvoiceDateLabel: factString("01/08/2026", domain.SourceMeeting), DueDateLabel: factString("15/08/2026", domain.SourceMeeting), Nominal: moneyInput(69_252_500, domain.SourceUser, "")},
		{WorkPeriod: factString("Termin 2", domain.SourceMeeting), Terms: factString("Pelunasan", domain.SourceMeeting), InvoiceDateLabel: factString("01/09/2026", domain.SourceMeeting), DueDateLabel: factString("15/09/2026", domain.SourceMeeting), Nominal: moneyInput(69_252_500, domain.SourceUser, "")},
	}
	state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

	_, calc, profile, issues, err := CalculatePromptKioskQueue(&state, quotationDate)
	require.NoError(t, err)
	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.EqualValues(t, 138_505_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 138_505_000, calc.ServiceTotal)
	assert.EqualValues(t, 83_625_000, calc.MaintenanceTotal)
	// Payment target total equals ServiceTotal, excluding MaintenanceTotal!
	assert.EqualValues(t, 138_505_000, calc.PaymentTargetTotal)
	assert.EqualValues(t, 138_505_000, calc.PaymentTotal)
	assert.True(t, profile.ShowMaintenance)
}

func TestCalculatePromptKioskQueue_QuarterDurationCalculation(t *testing.T) {
	// Formula: monthly_rate * headcount * period_quarters / 4
	// If monthly_rate = 16_000_000, headcount = 1, period_quarters = 1 (1 quarter-month / 1 week) -> 4_000_000
	state := validGovernmentServiceOnlyKioskQueueState()
	serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
	serviceInput.ServiceGroups = []PromptKioskQueueServiceGroup{
		{
			Title: factString("Sprint 1", domain.SourceMeeting),
			Items: []PromptKioskQueueServiceItem{
				{Name: factString("Senior Dev", domain.SourceMeeting), MonthlyRate: moneyInput(16_000_000, domain.SourceCatalog, "RATE-1"), Headcount: factInt(1, domain.SourceMeeting), PeriodQuarters: factInt(1, domain.SourceMeeting)},
			},
		},
	}
	// PPN 11% on 4_000_000 is 440_000 -> ServiceTotal = 4_440_000
	serviceInput.GovernmentPaymentSchedule = []PromptKioskQueueGovernmentPaymentRow{
		{Stage: factString("Full", domain.SourceMeeting), BillingBasis: factString("Delivery", domain.SourceMeeting), Nominal: moneyInput(4_440_000, domain.SourceUser, "")},
	}
	state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

	_, calc, _, issues, err := CalculatePromptKioskQueue(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Empty(t, blockingKioskQueueIssues(issues), "unexpected issues: %+v", issues)
	assert.EqualValues(t, 4_000_000, calc.ServiceSubtotal)
	assert.EqualValues(t, 440_000, calc.ServiceTaxAmount)
	assert.EqualValues(t, 4_440_000, calc.ServiceTotal)
}

func TestCalculatePromptKioskQueue_MissingUnitPriceBlocksCalculation(t *testing.T) {
	t.Run("service item missing monthly rate", func(t *testing.T) {
		state := validGovernmentHardwareKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.ServiceGroups[0].Items[0].MonthlyRate.Amount = nil
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptKioskQueue(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.ServiceTotal)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.service_monthly_rate.unavailable"), "%+v", issues)
	})

	t.Run("hardware item missing unit price", func(t *testing.T) {
		state := validGovernmentHardwareKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.HardwareItems[0].UnitPrice.Amount = nil
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptKioskQueue(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.HardwareTotal)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.hardware_unit_price.unavailable"), "%+v", issues)
	})

	t.Run("maintenance item missing monthly rate", func(t *testing.T) {
		state := validPrivateFoodCourtKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.MaintenanceItems[0].MonthlyRate.Amount = nil
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptKioskQueue(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.MaintenanceTotal)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.maintenance_monthly_rate.unavailable"), "%+v", issues)
	})

	t.Run("untrusted source for price", func(t *testing.T) {
		state := validGovernmentHardwareKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.HardwareItems[0].UnitPrice.Source = domain.SourceMeeting
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, calc, _, issues, err := CalculatePromptKioskQueue(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.Zero(t, calc.HardwareTotal)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.hardware_unit_price.untrusted_source"), "%+v", issues)
	})
}

func TestCalculatePromptKioskQueue_ClientSuppliedHardwareNeverCreatesZeroPricedLine(t *testing.T) {
	state := validGovernmentServiceOnlyKioskQueueState()
	// Ensure client supplied hardware does not create any hardware items in domain input
	input, issues := BuildPromptKioskQueueInput(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	assert.Empty(t, blockingKioskQueueIssues(issues))
	assert.Equal(t, domain.HardwareClientSupplied, input.HardwareProvision)
	assert.Nil(t, input.HardwareItems)

	_, calc, profile, _, err := CalculatePromptKioskQueue(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.False(t, profile.ShowHardware)
	assert.Zero(t, calc.HardwareSubtotal)
	assert.Zero(t, calc.HardwareTotal)
	assert.Empty(t, calc.Hardware)
}

func TestCalculatePromptKioskQueue_PaymentMismatchAndNoStageDerivation(t *testing.T) {
	state := validGovernmentHardwareKioskQueueState()
	serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
	// Change one payment nominal so total doesn't match 100,000,000
	serviceInput.GovernmentPaymentSchedule[0].Nominal = moneyInput(50_000_000, domain.SourceUser, "")
	state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

	_, _, _, issues, err := CalculatePromptKioskQueue(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.payment.mismatch"), "%+v", issues)
}

func TestCalculatePromptKioskQueue_TaxPolicyGate(t *testing.T) {
	t.Run("government MPP rejects non-inclusive PPN", func(t *testing.T) {
		state := validGovernmentHardwareKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.TaxTreatment = factString(string(domain.KioskTaxExclusivePPN), domain.SourceMeeting)
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, _, _, issues, err := CalculatePromptKioskQueue(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.tax_treatment.invalid"), "%+v", issues)
	})

	t.Run("private food court rejects inclusive PPN", func(t *testing.T) {
		state := validPrivateFoodCourtKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.TaxTreatment = factString(string(domain.KioskTaxInclusivePPN11), domain.SourceMeeting)
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, _, _, issues, err := CalculatePromptKioskQueue(&state, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.tax_treatment.invalid"), "%+v", issues)
	})

	t.Run("unknown tax treatment creates blocking issue", func(t *testing.T) {
		state := validGovernmentHardwareKioskQueueState()
		serviceInput := mustPromptKioskQueueServiceInput(t, state.ServiceInput)
		serviceInput.TaxTreatment = factString("unknown_tax", domain.SourceMeeting)
		state.ServiceInput = mustPromptKioskQueueJSON(t, serviceInput)

		_, _, _, issues, err := CalculatePromptKioskQueue(&state, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
		require.NoError(t, err)
		assert.True(t, hasKioskQueueIssue(issues, "kiosk_queue.tax_treatment.invalid"), "%+v", issues)
	})
}

func TestPromptServiceAdapterRegistry_KioskQueue(t *testing.T) {
	registry := NewPromptServiceAdapterRegistry(nil, nil, nil, nil, nil)
	adapter := registry.KioskQueue()
	require.NotNil(t, adapter)
	assert.Equal(t, domain.CommercialSchemeKioskQueueSystem, adapter.Scheme())
}

func TestPromptKioskQueueAdapter_Render(t *testing.T) {
	state := validGovernmentHardwareKioskQueueState()
	quotationDate := time.Date(2026, time.July, 30, 0, 0, 0, 0, time.UTC)
	input, calc, profile, issues, err := CalculatePromptKioskQueue(&state, quotationDate)
	require.NoError(t, err)
	require.Empty(t, blockingKioskQueueIssues(issues))

	adapter := NewPromptKioskQueueAdapter(nil)
	html, err := adapter.Render(QuotationMetadata{
		QuotationNumber:   "LANDA/MPP-Antrian/0726/001",
		ClientName:        "Dinas Penanaman Modal",
		ClientCompanyName: "Pemerintah Kabupaten",
		Subject:           "Sistem Antrian Terpadu MPP",
		IssueDate:         "30 Juli 2026",
	}, PromptKioskQueueSnapshot{
		Input:       input,
		Calculation: calc,
		Profile:     profile,
	}, "<p>Syarat dan ketentuan umum berlaku.</p>")

	require.NoError(t, err)
	assert.Contains(t, html, "kiosk-queue-document")
	assert.Contains(t, html, "LANDA/MPP-Antrian/0726/001")
	assert.Contains(t, html, "Sistem Antrian Terpadu MPP")
	assert.Contains(t, html, "Mesin Kiosk Touchscreen 21.5 inch")
	assert.Contains(t, html, "Syarat dan ketentuan umum berlaku.")
}

func TestPromptKioskQueueServiceInputFixtures(t *testing.T) {
	for _, name := range []string{"kiosk-queue-mpp-normal.json", "kiosk-queue-mpp-incomplete.json"} {
		raw := mustReadPromptFixture(t, name)
		input, err := DecodePromptKioskQueueServiceInput(raw)
		require.NoError(t, err)
		if strings.Contains(name, "incomplete") {
			assert.Nil(t, input.ServiceGroups[0].Items[0].MonthlyRate.Amount)
			assert.Nil(t, input.HardwareItems[0].UnitPrice.Amount)
		} else {
			assert.NotNil(t, input.ServiceGroups[0].Items[0].MonthlyRate.Amount)
			assert.NotNil(t, input.HardwareItems[0].UnitPrice.Amount)
		}
	}
}
