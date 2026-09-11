package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestCloudInfrastructureIntegrityCanonicalAndTampered(t *testing.T) {
	in := cloudRendererInput()
	calc, err := CalculateCloudInfrastructure(in)
	require.NoError(t, err)
	profile, err := ResolveCloudInfrastructureProfile(in)
	require.NoError(t, err)
	expected := CloudInfrastructureIntegrityExpectations{Input: in, Calculation: calc, Profile: profile, LegalHTML: `<p>Ketentuan legal</p>`}
	doc, err := NewCloudInfrastructureRenderer().Render(CloudInfrastructureDocumentInput{Metadata: QuotationMetadata{QuotationNumber: in.DocumentNumber, IssueDate: "24 Juni 2026"}, Input: in, Calculation: calc, Profile: profile, LegalHTML: expected.LegalHTML})
	require.NoError(t, err)
	sanitized, err := NewHTMLSanitizer().Sanitize(doc)
	require.NoError(t, err)
	require.NoError(t, ValidateCloudInfrastructureIntegrity(sanitized, expected))
	tampered := strings.Replace(sanitized, "Rp 1.800.000", "Rp 1.700.000", 1)
	err = ValidateCloudInfrastructureIntegrity(tampered, expected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "monthly_subtotal")
	tampered = strings.Replace(sanitized, `data-cloud-package-row="1"`, `data-cloud-package-row="9"`, 1)
	err = ValidateCloudInfrastructureIntegrity(tampered, expected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package row")
}

func TestCloudInfrastructureDraftInspectorClassifiesCommercialAndLegalFields(t *testing.T) {
	in := cloudRendererInput()
	in.SetupItems = []domain.CloudSetupItemInput{{Name: "Provisioning", Amount: 150000}}
	in.PaymentSchedule = append([]domain.CloudPaymentRowInput{{WorkPeriod: "Setup", Terms: "Di muka", InvoiceDateLabel: "24 Jun", DueDateLabel: "8 Jul", Component: domain.CloudPaymentSetup, Nominal: 150000}}, in.PaymentSchedule...)
	calc, err := CalculateCloudInfrastructure(in)
	require.NoError(t, err)
	profile, err := ResolveCloudInfrastructureProfile(in)
	require.NoError(t, err)
	legal := `<p>Ketentuan legal</p>`
	doc, err := NewCloudInfrastructureRenderer().Render(CloudInfrastructureDocumentInput{Input: in, Calculation: calc, Profile: profile, LegalHTML: legal})
	require.NoError(t, err)
	sanitized, err := NewHTMLSanitizer().Sanitize(doc)
	require.NoError(t, err)
	expected := CloudInfrastructureIntegrityExpectations{Input: in, Calculation: calc, Profile: profile, LegalHTML: legal}
	require.NoError(t, ValidateCloudInfrastructureStructureForDraft(sanitized, expected))

	for _, tc := range []struct {
		name       string
		old, new   string
		legal      bool
		commercial bool
	}{
		{"scope narrative", "Membuat script testing menggunakan tools K6", "Aktivitas yang dapat diedit", false, false},
		{"package", "Virtual Machine Small - Kube Master", "Cloud VPS Medium", false, true},
		{"subtotal", "Rp 1.800.000", "Rp 1.700.000", false, true},
		{"setup", "Rp 150.000", "Rp 175.000", false, true},
		{"payment term", "Pelunasan Bulanan", "Termin bulanan", false, true},
		{"tax", "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.", "sudah termasuk PPN", true, false},
		{"limitation", "Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.", "Biaya tambahan", true, false},
		{"legal template", "Ketentuan legal", "Ketentuan lain", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := strings.Replace(sanitized, tc.old, tc.new, 1)
			require.NotEqual(t, sanitized, changed)
			commercial, err := CheckCloudInfrastructureCommercialModified(changed, in, calc)
			require.NoError(t, err)
			legalModified, err := CheckCloudInfrastructureLegalModified(changed, in, profile, legal)
			require.NoError(t, err)
			assert.Equal(t, tc.commercial, commercial)
			assert.Equal(t, tc.legal, legalModified)
		})
	}

	broken := strings.Replace(sanitized, `data-section-code="payment"`, `data-section-code="removed"`, 1)
	require.Error(t, ValidateCloudInfrastructureStructureForDraft(broken, expected))
}
