package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func ptrString(s string) *string { return &s }

func validPromptCloudInfrastructureVPSState() domain.PromptQuotationState {
	serviceInput := PromptCloudInfrastructureServiceInput{
		Kind:           PromptCloudInfrastructureServiceInputKind,
		DocumentNumber: domain.Fact[string]{Value: "CTH/Cloud-VPS/0926/001", Source: domain.SourceMeeting, Evidence: "dokumen no CTH/Cloud-VPS/0926/001"},
		ServiceName:    domain.Fact[string]{Value: "Venturo Cloud VPS", Source: domain.SourceMeeting, Evidence: "layanan Venturo Cloud VPS"},
		TaxTreatment:   domain.Fact[string]{Value: string(domain.CloudTaxExclusive), Source: domain.SourceMeeting, Evidence: "belum termasuk PPN"},
		PackageItems: []PromptCloudPackageItem{
			{
				Category:  domain.Fact[string]{Value: string(domain.CloudPackageVPS), Source: domain.SourceMeeting, Evidence: "VPS Cloud"},
				Name:      domain.Fact[string]{Value: "Cloud VPS Medium", Source: domain.SourceMeeting, Evidence: "Paket VPS Medium"},
				VCPU:      domain.Fact[int]{Value: 4, Source: domain.SourceMeeting, Evidence: "4 vCPU"},
				RAMGB:     domain.Fact[int]{Value: 8, Source: domain.SourceMeeting, Evidence: "8 GB RAM"},
				StorageGB: domain.Fact[int]{Value: 80, Source: domain.SourceMeeting, Evidence: "80 GB SSD"},
				UnitRate: domain.MoneyInput{
					Amount:     ptrInt64(1_500_000),
					Source:     domain.SourceCatalog,
					CatalogRef: ptrString("venturo-infra-2026.vps.medium"),
				},
			},
		},
		SetupItems: []PromptCloudSetupItem{
			{
				Name: domain.Fact[string]{Value: "Server Setup & Initial Hardening", Source: domain.SourceMeeting, Evidence: "setup dan hardening"},
				Amount: domain.MoneyInput{
					Amount: ptrInt64(500_000),
					Source: domain.SourceUser,
				},
			},
		},
		ScopeRows: []PromptCloudScopeRow{
			{
				Sections: []PromptCloudScopeSection{
					{
						Title: domain.Fact[string]{Value: "Provisioning & Hardening", Source: domain.SourceMeeting, Evidence: "lingkup provisioning"},
						Activities: []domain.Fact[string]{
							{Value: "Setup OS Linux & Firewall", Source: domain.SourceMeeting, Evidence: "setup firewall"},
							{Value: "Install Web Server & SSL", Source: domain.SourceMeeting, Evidence: "install SSL"},
						},
					},
				},
				Timeline: domain.Fact[string]{Value: "3 Hari Kerja", Source: domain.SourceMeeting, Evidence: "timeline 3 hari kerja"},
			},
		},
		PaymentSchedule: []PromptCloudPaymentRow{
			{
				WorkPeriod:       domain.Fact[string]{Value: "Setup", Source: domain.SourceMeeting, Evidence: "setup fee"},
				Terms:            domain.Fact[string]{Value: "Di muka", Source: domain.SourceMeeting, Evidence: "pembayaran di muka"},
				InvoiceDateLabel: domain.Fact[string]{Value: "24 Sep 2026", Source: domain.SourceMeeting, Evidence: "24 Sep 2026"},
				DueDateLabel:     domain.Fact[string]{Value: "8 Okt 2026", Source: domain.SourceMeeting, Evidence: "8 Okt 2026"},
				Component:        domain.CloudPaymentSetup,
				Nominal: domain.MoneyInput{
					Amount: ptrInt64(500_000),
					Source: domain.SourceUser,
				},
			},
			{
				WorkPeriod:       domain.Fact[string]{Value: "Bulan 1", Source: domain.SourceMeeting, Evidence: "bulan 1"},
				Terms:            domain.Fact[string]{Value: "Bulanan", Source: domain.SourceMeeting, Evidence: "termin bulanan"},
				InvoiceDateLabel: domain.Fact[string]{Value: "30 Sep 2026", Source: domain.SourceMeeting, Evidence: "30 Sep 2026"},
				DueDateLabel:     domain.Fact[string]{Value: "14 Okt 2026", Source: domain.SourceMeeting, Evidence: "14 Okt 2026"},
				Component:        domain.CloudPaymentMonthly,
				Nominal: domain.MoneyInput{
					Amount:     ptrInt64(1_500_000),
					Source:     domain.SourceCatalog,
					CatalogRef: ptrString("venturo-infra-2026.vps.medium"),
				},
			},
			{
				WorkPeriod:       domain.Fact[string]{Value: "Bulan 2", Source: domain.SourceMeeting, Evidence: "bulan 2"},
				Terms:            domain.Fact[string]{Value: "Bulanan", Source: domain.SourceMeeting, Evidence: "termin bulanan"},
				InvoiceDateLabel: domain.Fact[string]{Value: "31 Okt 2026", Source: domain.SourceMeeting, Evidence: "31 Okt 2026"},
				DueDateLabel:     domain.Fact[string]{Value: "14 Nov 2026", Source: domain.SourceMeeting, Evidence: "14 Nov 2026"},
				Component:        domain.CloudPaymentMonthly,
				Nominal: domain.MoneyInput{
					Amount:     ptrInt64(1_500_000),
					Source:     domain.SourceCatalog,
					CatalogRef: ptrString("venturo-infra-2026.vps.medium"),
				},
			},
		},
		PaymentDueDays: domain.Fact[int]{Value: 14, Source: domain.SourceMeeting, Evidence: "jatuh tempo 14 hari"},
		LimitationCodes: []domain.Fact[string]{
			{Value: string(domain.CloudLimitOutOfPocket), Source: domain.SourceMeeting, Evidence: "out of pocket"},
			{Value: string(domain.CloudLimitThirdParty), Source: domain.SourceMeeting, Evidence: "third party"},
		},
		AdditionalLimitations: []domain.Fact[string]{
			{Value: "Backup data di luar retensi standar memerlukan konfigurasi khusus.", Source: domain.SourceMeeting, Evidence: "backup data tambahan"},
		},
	}

	raw, _ := json.Marshal(serviceInput)
	return domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		Readiness:     domain.ReadinessPriceable,
		MeetingNotes:  "Kebutuhan Cloud Infrastructure VPS Medium untuk klien.",
		ClientCompany: &domain.Fact[string]{
			Value:    "PT Nusantara Tech",
			Source:   domain.SourceMeeting,
			Evidence: "PT Nusantara Tech",
		},
		ClientName: &domain.Fact[string]{
			Value:    "PT Nusantara Tech",
			Source:   domain.SourceMeeting,
			Evidence: "PT Nusantara Tech",
		},
		CommercialScheme: &domain.Fact[string]{
			Value:    domain.CommercialSchemeCloudInfrastructure,
			Source:   domain.SourceMeeting,
			Evidence: "layanan cloud infrastructure",
		},
		ServiceInput: raw,
	}
}

func validPromptCloudInfrastructureSharedHostingState() domain.PromptQuotationState {
	serviceInput := PromptCloudInfrastructureServiceInput{
		Kind:           PromptCloudInfrastructureServiceInputKind,
		DocumentNumber: domain.Fact[string]{Value: "CTH/Cloud-Shared/0926/002", Source: domain.SourceMeeting, Evidence: "dokumen no CTH/Cloud-Shared/0926/002"},
		ServiceName:    domain.Fact[string]{Value: "Venturo Shared Hosting Pro", Source: domain.SourceMeeting, Evidence: "layanan Shared Hosting Pro"},
		TaxTreatment:   domain.Fact[string]{Value: string(domain.CloudTaxInclusive), Source: domain.SourceMeeting, Evidence: "termasuk PPN"},
		PackageItems: []PromptCloudPackageItem{
			{
				Category:  domain.Fact[string]{Value: string(domain.CloudPackageSharedHosting), Source: domain.SourceMeeting, Evidence: "Shared Hosting"},
				Name:      domain.Fact[string]{Value: "Shared Hosting Business", Source: domain.SourceMeeting, Evidence: "Paket Business"},
				VCPU:      domain.Fact[int]{Value: 2, Source: domain.SourceMeeting, Evidence: "2 vCPU"},
				RAMGB:     domain.Fact[int]{Value: 2, Source: domain.SourceMeeting, Evidence: "2 GB RAM"},
				StorageGB: domain.Fact[int]{Value: 20, Source: domain.SourceMeeting, Evidence: "20 GB Storage"},
				UnitRate: domain.MoneyInput{
					Amount: ptrInt64(350_000),
					Source: domain.SourceUser,
				},
			},
		},
		PaymentSchedule: []PromptCloudPaymentRow{
			{
				WorkPeriod:       domain.Fact[string]{Value: "Bulan 1", Source: domain.SourceMeeting, Evidence: "Bulan 1"},
				Terms:            domain.Fact[string]{Value: "Bulanan", Source: domain.SourceMeeting, Evidence: "termin bulanan"},
				InvoiceDateLabel: domain.Fact[string]{Value: "30 Sep 2026", Source: domain.SourceMeeting, Evidence: "30 Sep 2026"},
				DueDateLabel:     domain.Fact[string]{Value: "14 Okt 2026", Source: domain.SourceMeeting, Evidence: "14 Okt 2026"},
				Component:        domain.CloudPaymentMonthly,
				Nominal: domain.MoneyInput{
					Amount: ptrInt64(350_000),
					Source: domain.SourceUser,
				},
			},
		},
		PaymentDueDays: domain.Fact[int]{Value: 14, Source: domain.SourceMeeting, Evidence: "14 hari"},
		LimitationCodes: []domain.Fact[string]{
			{Value: string(domain.CloudLimitOutOfPocket), Source: domain.SourceMeeting, Evidence: "out of pocket"},
		},
	}

	raw, _ := json.Marshal(serviceInput)
	return domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1,
		Readiness:     domain.ReadinessPriceable,
		MeetingNotes:  "Kebutuhan Cloud Shared Hosting untuk landing page klien.",
		ClientCompany: &domain.Fact[string]{
			Value:    "CV Mitra Abadi",
			Source:   domain.SourceMeeting,
			Evidence: "CV Mitra Abadi",
		},
		ClientName: &domain.Fact[string]{
			Value:    "CV Mitra Abadi",
			Source:   domain.SourceMeeting,
			Evidence: "CV Mitra Abadi",
		},
		CommercialScheme: &domain.Fact[string]{
			Value:    domain.CommercialSchemeCloudInfrastructure,
			Source:   domain.SourceMeeting,
			Evidence: "layanan cloud infrastructure",
		},
		ServiceInput: raw,
	}
}

func mustPromptCloudInfrastructureServiceInput(t *testing.T, raw json.RawMessage) PromptCloudInfrastructureServiceInput {
	t.Helper()
	in, err := DecodePromptCloudInfrastructureServiceInput(raw)
	require.NoError(t, err)
	return in
}

func mustPromptCloudInfrastructureJSON(t *testing.T, in PromptCloudInfrastructureServiceInput) json.RawMessage {
	t.Helper()
	raw, err := EncodePromptCloudInfrastructureServiceInput(in)
	require.NoError(t, err)
	return raw
}

func hasCloudInfrastructureIssue(issues []domain.DraftIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func blockingCloudInfrastructureIssues(issues []domain.DraftIssue) []domain.DraftIssue {
	out := make([]domain.DraftIssue, 0, len(issues))
	for _, i := range issues {
		if i.Severity == domain.IssueBlocking {
			out = append(out, i)
		}
	}
	return out
}

func TestCalculatePromptCloudInfrastructureVPSComplete(t *testing.T) {
	state := validPromptCloudInfrastructureVPSState()

	input, calculation, profile, issues, err := CalculatePromptCloudInfrastructure(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingCloudInfrastructureIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, "CTH/Cloud-VPS/0926/001", input.DocumentNumber)
	assert.Equal(t, "Venturo Cloud VPS", input.ServiceName)
	assert.Equal(t, domain.CloudTaxExclusive, input.TaxTreatment)
	require.Len(t, input.PackageItems, 1)
	assert.Equal(t, domain.CloudPackageVPS, input.PackageItems[0].Category)
	assert.Equal(t, "Cloud VPS Medium", input.PackageItems[0].Name)
	assert.Equal(t, int32(4), input.PackageItems[0].VCPU)
	assert.Equal(t, int32(8), input.PackageItems[0].RAMGB)
	assert.Equal(t, int32(80), input.PackageItems[0].StorageGB)
	assert.Equal(t, int64(1_500_000), input.PackageItems[0].UnitRate)

	require.Len(t, input.SetupItems, 1)
	assert.Equal(t, "Server Setup & Initial Hardening", input.SetupItems[0].Name)
	assert.Equal(t, int64(500_000), input.SetupItems[0].Amount)

	assert.Equal(t, int64(1_500_000), calculation.MonthlySubtotal)
	assert.Equal(t, int64(500_000), calculation.SetupSubtotal)
	assert.Equal(t, int64(500_000), calculation.SetupPaymentTotal)
	assert.Equal(t, int32(2), calculation.MonthlyPaymentRows)
	assert.Equal(t, int64(3_000_000), calculation.ScheduledRecurringTotal)
	assert.Equal(t, int64(3_500_000), calculation.PaymentTotal)

	assert.True(t, profile.ShowSetup)
	assert.True(t, profile.ShowScope)
	assert.Equal(t, domain.CloudTaxExclusive, profile.TaxTreatment)
	assert.NotEmpty(t, profile.TaxClauseText)
	assert.Len(t, profile.Sections, 4) // cloud_cost, scope_timeline, payment, limitations
}

func TestCalculatePromptCloudInfrastructureSharedHostingComplete(t *testing.T) {
	state := validPromptCloudInfrastructureSharedHostingState()

	input, calculation, profile, issues, err := CalculatePromptCloudInfrastructure(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))

	require.NoError(t, err)
	assert.Empty(t, blockingCloudInfrastructureIssues(issues), "unexpected issues: %+v", issues)
	assert.Equal(t, domain.CloudPackageSharedHosting, input.PackageItems[0].Category)
	assert.Equal(t, int64(350_000), calculation.MonthlySubtotal)
	assert.Equal(t, int64(0), calculation.SetupSubtotal)
	assert.Equal(t, int32(1), calculation.MonthlyPaymentRows)
	assert.Equal(t, int64(350_000), calculation.PaymentTotal)

	assert.False(t, profile.ShowSetup)
	assert.False(t, profile.ShowScope)
	assert.Len(t, profile.Sections, 3) // cloud_cost, payment, limitations
}

func TestCalculatePromptCloudInfrastructureMissingOrUntrustedPackageUnitRate(t *testing.T) {
	for name, mutate := range map[string]func(*PromptCloudInfrastructureServiceInput){
		"missing package unit rate": func(input *PromptCloudInfrastructureServiceInput) {
			input.PackageItems[0].UnitRate.Amount = nil
			input.PackageItems[0].UnitRate.Source = domain.SourceSuggestion
		},
		"meeting package unit rate is untrusted": func(input *PromptCloudInfrastructureServiceInput) {
			input.PackageItems[0].UnitRate.Source = domain.SourceMeeting
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptCloudInfrastructureVPSState()
			serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
			mutate(&serviceInput)
			state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

			_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

			require.NoError(t, err)
			assert.Zero(t, calculation.PaymentTotal)
			if strings.Contains(name, "missing") {
				assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.package_item.unit_rate.unavailable"), "%+v", issues)
			} else {
				assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.package_item.unit_rate.untrusted_source"), "%+v", issues)
			}
		})
	}
}

func TestCalculatePromptCloudInfrastructureSetupOptionalAndUntrustedAmount(t *testing.T) {
	t.Run("missing setup amount blocks calculation when setup item is present", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
		serviceInput.SetupItems[0].Amount.Amount = nil
		serviceInput.SetupItems[0].Amount.Source = domain.SourceSuggestion
		state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

		_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

		require.NoError(t, err)
		assert.Zero(t, calculation.PaymentTotal)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.setup_item.amount.unavailable"), "%+v", issues)
	})

	t.Run("untrusted setup amount blocks calculation", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
		serviceInput.SetupItems[0].Amount.Source = domain.SourceMeeting
		state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

		_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

		require.NoError(t, err)
		assert.Zero(t, calculation.PaymentTotal)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.setup_item.amount.untrusted_source"), "%+v", issues)
	})

	t.Run("omitted setup items calculate cleanly without setup fee", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
		serviceInput.SetupItems = nil
		// adjust payment schedule to match 0 setup
		serviceInput.PaymentSchedule = serviceInput.PaymentSchedule[1:] // remove setup payment row
		state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

		_, calculation, profile, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

		require.NoError(t, err)
		assert.Empty(t, blockingCloudInfrastructureIssues(issues), "%+v", issues)
		assert.Zero(t, calculation.SetupSubtotal)
		assert.False(t, profile.ShowSetup)
	})
}

func TestCalculatePromptCloudInfrastructurePackageItemNeverHasQuantity(t *testing.T) {
	// Invariant: One package item has unit rate per month and no quantity;
	// never add quantity field or multiply imaginary seats.
	state := validPromptCloudInfrastructureVPSState()
	input, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

	require.NoError(t, err)
	assert.Empty(t, blockingCloudInfrastructureIssues(issues))
	require.Len(t, calculation.Packages, 1)
	assert.Equal(t, input.PackageItems[0].UnitRate, calculation.Packages[0].LineTotal)
}

func TestCalculatePromptCloudInfrastructurePaymentReconciliation(t *testing.T) {
	t.Run("monthly payment row mismatch emits payment mismatch issue", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
		// Monthly subtotal is 1,500,000, set payment row nominal to 1,000,000
		serviceInput.PaymentSchedule[1].Nominal.Amount = ptrInt64(1_000_000)
		state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

		_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

		require.NoError(t, err)
		assert.Zero(t, calculation.PaymentTotal)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.payment.mismatch"), "%+v", issues)
	})

	t.Run("setup payment total mismatch emits payment mismatch issue", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
		// Setup subtotal is 500,000, set setup payment row to 400,000
		serviceInput.PaymentSchedule[0].Nominal.Amount = ptrInt64(400_000)
		state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

		_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

		require.NoError(t, err)
		assert.Zero(t, calculation.PaymentTotal)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.payment.mismatch"), "%+v", issues)
	})

	t.Run("missing monthly service row emits payment mismatch issue", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
		// Leave only the setup payment row
		serviceInput.PaymentSchedule = serviceInput.PaymentSchedule[:1]
		state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

		_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

		require.NoError(t, err)
		assert.Zero(t, calculation.PaymentTotal)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.payment.mismatch"), "%+v", issues)
	})
}

func TestCalculatePromptCloudInfrastructureInvalidInput(t *testing.T) {
	for name, mutate := range map[string]func(*PromptCloudInfrastructureServiceInput){
		"invalid category": func(input *PromptCloudInfrastructureServiceInput) {
			input.PackageItems[0].Category.Value = "bare_metal"
		},
		"vcpu less than 1": func(input *PromptCloudInfrastructureServiceInput) {
			input.PackageItems[0].VCPU.Value = 0
		},
		"ram less than 1": func(input *PromptCloudInfrastructureServiceInput) {
			input.PackageItems[0].RAMGB.Value = 0
		},
		"storage less than 1": func(input *PromptCloudInfrastructureServiceInput) {
			input.PackageItems[0].StorageGB.Value = 0
		},
		"document number empty": func(input *PromptCloudInfrastructureServiceInput) {
			input.DocumentNumber.Value = ""
		},
	} {
		t.Run(name, func(t *testing.T) {
			state := validPromptCloudInfrastructureVPSState()
			serviceInput := mustPromptCloudInfrastructureServiceInput(t, state.ServiceInput)
			mutate(&serviceInput)
			state.ServiceInput = mustPromptCloudInfrastructureJSON(t, serviceInput)

			_, calculation, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())

			require.NoError(t, err)
			assert.Zero(t, calculation.PaymentTotal)
			assert.NotEmpty(t, blockingCloudInfrastructureIssues(issues), "expected blocking issues for %s", name)
		})
	}
}

func TestCalculatePromptCloudInfrastructureStateIssues(t *testing.T) {
	t.Run("nil state returns missing state issue", func(t *testing.T) {
		_, _, _, issues, err := CalculatePromptCloudInfrastructure(nil, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.state.missing"))
	})

	t.Run("wrong scheme returns invalid scheme issue", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		state.CommercialScheme.Value = domain.CommercialSchemeSubscriptionSaaS
		_, _, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.scheme.invalid"))
	})

	t.Run("missing quotation date returns quotation date missing issue", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		_, _, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Time{})
		require.NoError(t, err)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.quotation_date.missing"))
	})

	t.Run("invalid service input JSON returns invalid service input issue", func(t *testing.T) {
		state := validPromptCloudInfrastructureVPSState()
		state.ServiceInput = []byte(`{"kind": "wrong_kind"}`)
		_, _, _, issues, err := CalculatePromptCloudInfrastructure(&state, time.Now().UTC())
		require.NoError(t, err)
		assert.True(t, hasCloudInfrastructureIssue(issues, "cloud_infrastructure.service_input.invalid"))
	})
}

func TestPromptCloudInfrastructureAdapter_Render(t *testing.T) {
	state := validPromptCloudInfrastructureVPSState()

	input, calculation, profile, issues, err := CalculatePromptCloudInfrastructure(&state, time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Empty(t, blockingCloudInfrastructureIssues(issues))

	adapter := NewPromptCloudInfrastructureAdapter(nil)
	assert.Equal(t, domain.CommercialSchemeCloudInfrastructure, adapter.Scheme())

	metadata := QuotationMetadata{
		QuotationNumber: "CTH/Cloud-VPS/0926/001",
		Subject:         "Venturo Cloud VPS",
		ClientName:      "PT Nusantara Tech",
		IssueDate:       "24 September 2026",
	}

	snapshot := PromptCloudInfrastructureSnapshot{
		Input:       input,
		Calculation: calculation,
		Profile:     profile,
	}

	htmlContent, err := adapter.Render(metadata, snapshot, "<p>Legal terms</p>")
	require.NoError(t, err)
	assert.Contains(t, htmlContent, "PENAWARAN LAYANAN CLOUD INFRASTRUCTURE")
	assert.Contains(t, htmlContent, "Cloud VPS Medium")
	assert.Contains(t, htmlContent, "Server Setup &amp; Initial Hardening")
	assert.Contains(t, htmlContent, "Subtotal Bulanan")
	assert.Contains(t, htmlContent, "Rp 1.500.000")
	assert.Contains(t, htmlContent, "Rp 500.000")
	assert.Contains(t, htmlContent, "Yulianto Frandi")
	assert.Contains(t, htmlContent, "PT Nusantara Tech")
}

func TestPromptServiceAdapterRegistry_CloudInfrastructure(t *testing.T) {
	registry := NewPromptServiceAdapterRegistry(nil, nil, nil, nil, nil)
	require.NotNil(t, registry)
	adapter := registry.CloudInfrastructure()
	require.NotNil(t, adapter)
	assert.Equal(t, domain.CommercialSchemeCloudInfrastructure, adapter.Scheme())
}

func TestCloudInfrastructureFixtureRoundtrip(t *testing.T) {
	fixturePath := filepath.Join("testdata", "prompt_first", "cloud-infrastructure-vps.json")
	raw, err := os.ReadFile(fixturePath)
	if os.IsNotExist(err) {
		t.Skip("fixture file not yet created")
		return
	}
	require.NoError(t, err)

	serviceInput, err := DecodePromptCloudInfrastructureServiceInput(raw)
	require.NoError(t, err)
	assert.Equal(t, PromptCloudInfrastructureServiceInputKind, serviceInput.Kind)
	assert.Equal(t, "CTH/Cloud-VPS/0926/001", serviceInput.DocumentNumber.Value)
	assert.Equal(t, "Venturo Cloud VPS", serviceInput.ServiceName.Value)
	assert.Len(t, serviceInput.PackageItems, 1)
	assert.Equal(t, int64(1_500_000), *serviceInput.PackageItems[0].UnitRate.Amount)
}
