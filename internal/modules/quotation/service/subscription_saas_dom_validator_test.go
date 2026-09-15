package service

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func percetakanDocumentFixture(t *testing.T) (string, SubscriptionSaaSIntegrityExpectations) {
	t.Helper()
	input := validPercetakanTestInput()
	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	legalHTML := "<ol><li>Batasan resmi penawaran.</li></ol>"
	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/08/0002",
		Subject:           "Pengembangan Subscription AI Percetakan",
		ClientName:        "Bpk. Hendra",
		ClientCompanyName: "PT Percetakan Jaya",
		IssueDate:         "1 Agustus 2026",
	}

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         input,
		Calculation:   calc,
		Profile:       profile,
		FeatureGroups: input.FeatureGroups,
		LegalHTML:     legalHTML,
	})
	require.NoError(t, err)

	exp := SubscriptionSaaSIntegrityExpectations{
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   legalHTML,
	}
	return docHTML, exp
}

func annualDocumentFixture(t *testing.T) (string, SubscriptionSaaSIntegrityExpectations) {
	t.Helper()
	input := domain.SubscriptionSaaSInput{
		ProductName:       "Annual SaaS",
		QuotationDate:     time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		SectionLayout:     domain.SubscriptionLayoutDevelopmentMaintenance,
		BillingCycle:      domain.SubscriptionBillingYearly,
		TaxTreatment:      domain.SubscriptionTaxExclusive,
		InitialTermMonths: 24,
		AutoRenewal:       true,
		PricePeriods: []domain.SubscriptionPricePeriodInput{
			{Label: "Tahun 1 - Development", StartMonthOffset: 0, DurationMonths: 12, MonthlyRate: 1_000_000},
			{Label: "Tahun 2 - Maintenance", StartMonthOffset: 12, DurationMonths: 12, MonthlyRate: 1_500_000},
		},
		PaymentSchedule: []domain.SubscriptionPaymentRowInput{
			{WorkPeriod: "Tahun 1", StartMonthOffset: 0, CoveredMonths: 12, InvoiceDateLabel: "Saat kontrak", DueDateLabel: "H+14", Nominal: 12_000_000},
			{WorkPeriod: "Tahun 2", StartMonthOffset: 12, CoveredMonths: 12, InvoiceDateLabel: "Awal tahun kedua", DueDateLabel: "H+14", Nominal: 18_000_000},
		},
		FeatureGroups: []domain.SubscriptionFeatureGroupInput{
			{Name: "Core Modules", Features: []string{"User Authentication"}},
		},
		IsPartnership: false,
	}

	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	legalHTML := "<ol><li>Ketentuan hukum resmi tahunan.</li></ol>"
	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/08/0003",
		Subject:           "Annual Enterprise SaaS",
		ClientName:        "Ibu Ratna",
		ClientCompanyName: "PT Sukses Bersama",
		IssueDate:         "1 Agustus 2026",
	}

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         input,
		Calculation:   calc,
		Profile:       profile,
		FeatureGroups: input.FeatureGroups,
		LegalHTML:     legalHTML,
	})
	require.NoError(t, err)

	exp := SubscriptionSaaSIntegrityExpectations{
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   legalHTML,
	}
	return docHTML, exp
}

func TestValidateSubscriptionSaaSIntegrity_OriginalPasses(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)

	err := ValidateSubscriptionSaaSIntegrity(doc, exp)
	require.NoError(t, err)

	commMod, err := CheckSubscriptionSaaSCommercialModified(doc, exp)
	require.NoError(t, err)
	assert.False(t, commMod)

	legalMod, err := CheckSubscriptionSaaSLegalModified(doc, exp)
	require.NoError(t, err)
	assert.False(t, legalMod)
}

func TestValidateSubscriptionSaaSIntegrity_MonthlyRateModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, "Rp 1.250.000", "Rp 2.000.000", 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.False(t, legalMod)
}

func TestValidateSubscriptionSaaSIntegrity_DurationModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, "12 Bulan", "6 Bulan", 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_LineTotalModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	// Replace line total
	tampered := strings.Replace(doc, `<td data-field="subscription_line_total">Rp 15.000.000</td>`, `<td data-field="subscription_line_total">Rp 10.000.000</td>`, 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_GrandTotalModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, `<td data-field="total_contract"><strong>Rp 15.000.000</strong></td>`, `<td data-field="total_contract"><strong>Rp 99.000.000</strong></td>`, 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_PaymentNominalModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, `<td data-field="payment_nominal">Rp 3.750.000</td>`, `<td data-field="payment_nominal">Rp 5.000.000</td>`, 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_PaymentRowAdded(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	payIdx := strings.Index(doc, `data-block-type="payment"`)
	require.Greater(t, payIdx, -1)
	tbodyClose := strings.Index(doc[payIdx:], "</tbody>") + payIdx
	extraRow := `<tr><td>Ekstra Termin</td><td>1 Mei 2027</td><td>15 Mei 2027</td><td data-field="payment_nominal">Rp 1.000.000</td></tr>`
	tampered := doc[:tbodyClose] + extraRow + doc[tbodyClose:]

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_CommercialRowAdded(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	commIdx := strings.Index(doc, `data-block-type="commercial"`)
	require.Greater(t, commIdx, -1)
	tbodyClose := strings.Index(doc[commIdx:], "</tbody>") + commIdx
	extraRow := `<tr><td>Ekstra Service</td><td>Rp 1.000.000</td><td>1 Bulan</td><td data-field="subscription_line_total">Rp 1.000.000</td></tr>`
	tampered := doc[:tbodyClose] + extraRow + doc[tbodyClose:]

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_PaymentRowDeleted(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	start := strings.Index(doc, `<tr><td>Mei-Juli 2027`)
	require.Greater(t, start, -1)
	end := strings.Index(doc[start:], `</tr>`) + start + len(`</tr>`)
	tampered := doc[:start] + doc[end:]

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_RightsTextModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, "50% untuk Mitra", "40% untuk Mitra", 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.False(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_OwnershipClauseModified(t *testing.T) {
	input := validPercetakanTestInput()
	input.RightsClauseCode = domain.RightsClauseVenturoSoftwareOwnershipV1
	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	renderer := NewSubscriptionSaaSHTMLRenderer()
	doc, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:    QuotationMetadata{QuotationNumber: "Q-001", Subject: "SaaS", ClientName: "Client"},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<ol><li>Batasan.</li></ol>",
	})
	require.NoError(t, err)

	exp := SubscriptionSaaSIntegrityExpectations{
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<ol><li>Batasan.</li></ol>",
	}

	tampered := strings.Replace(doc, "milik Venturo", "milik Klien", 1)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod)

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}

func TestValidateSubscriptionSaaSIntegrity_LimitationsModified(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, "Batasan resmi penawaran.", "Batasan diubah secara sepihak.", 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.False(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_AutoRenewalModified(t *testing.T) {
	doc, exp := annualDocumentFixture(t)
	tampered := strings.Replace(doc, "diperpanjang otomatis", "tidak diperpanjang otomatis", 1)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod)

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}

func TestValidateSubscriptionSaaSIntegrity_TaxClauseModified(t *testing.T) {
	doc, exp := annualDocumentFixture(t)
	tampered := strings.Replace(doc, "Harga belum termasuk PPN.", "Harga sudah termasuk PPN.", 1)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod)

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}

func TestValidateSubscriptionSaaSIntegrity_MissingBlocks(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)

	// Missing commercial block
	tamperedComm := strings.Replace(doc, `data-block-type="commercial"`, `data-block-type="other"`, 1)
	err := ValidateSubscriptionSaaSIntegrity(tamperedComm, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
	commMod, _ := CheckSubscriptionSaaSCommercialModified(tamperedComm, exp)
	assert.True(t, commMod)

	// Missing payment block
	tamperedPay := strings.Replace(doc, `data-block-type="payment"`, `data-block-type="other"`, 1)
	err = ValidateSubscriptionSaaSIntegrity(tamperedPay, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
	commMod, _ = CheckSubscriptionSaaSCommercialModified(tamperedPay, exp)
	assert.True(t, commMod)

	// Missing legal block
	tamperedLegal := strings.Replace(doc, `data-block-type="legal"`, `data-block-type="other"`, 1)
	err = ValidateSubscriptionSaaSIntegrity(tamperedLegal, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
	legalMod, _ := CheckSubscriptionSaaSLegalModified(tamperedLegal, exp)
	assert.True(t, legalMod)
}

func TestValidateSubscriptionSaaSIntegrity_ForbiddenTermsDetected(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)
	tampered := strings.Replace(doc, "Melalui penawaran ini", "Melalui penawaran budget ini", 1)

	err := ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}

func TestValidateSubscriptionSaaSIntegrity_OneTimeItemModified(t *testing.T) {
	input := validPercetakanTestInput()
	input.OneTimeItems = []domain.SubscriptionOneTimeItemInput{
		{Name: "Biaya Setup", Amount: 2_500_000},
	}
	input.PaymentSchedule[0].Nominal = 6_250_000
	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	renderer := NewSubscriptionSaaSHTMLRenderer()
	doc, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:    QuotationMetadata{QuotationNumber: "Q-001", Subject: "SaaS", ClientName: "Client"},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal</p>",
	})
	require.NoError(t, err)

	exp := SubscriptionSaaSIntegrityExpectations{
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Legal</p>",
	}

	tampered := strings.Replace(doc, "Rp 2.500.000", "Rp 5.000.000", 1)
	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod)
}

func TestValidateSubscriptionSaaSIntegrity_DuplicateUnconfiguredMarkersDetected(t *testing.T) {
	doc, exp := annualDocumentFixture(t)
	require.False(t, exp.Profile.HasRights())
	require.Empty(t, exp.Profile.RightsClauseText)

	// Inject duplicate unconfigured markers
	duplicateMarkers := `<p data-field="rights_clause">Injected 1</p><p data-field="rights_clause">Injected 2</p>`
	tampered := strings.Replace(doc, "</article>", duplicateMarkers+"</article>", 1)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod, "duplicate unconfigured markers must be detected as legal modified")

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}

func TestValidateSubscriptionSaaSIntegrity_UnexpectedRightsBlockDetected(t *testing.T) {
	doc, exp := annualDocumentFixture(t)
	require.False(t, exp.Profile.HasRights())

	// Inject an entire rights block when not configured
	injectedBlock := `<section data-block-type="rights"><h2>Hak Para Pihak</h2><p>Injected rights</p></section>`
	tampered := strings.Replace(doc, `<section data-block-type="payment"`, injectedBlock+`<section data-block-type="payment"`, 1)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod, "unexpected rights block must be detected as legal modified")

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
	assert.Contains(t, err.Error(), "rights block is unexpected")
}

func TestValidateSubscriptionSaaSIntegrity_MonthlyRateExactMatch(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)

	// Tamper Rp 1.250.000 to Rp 11.250.000 (which contains 1.250.000 as substring)
	tampered := strings.Replace(doc, "Rp 1.250.000", "Rp 11.250.000", 1)

	commMod, err := CheckSubscriptionSaaSCommercialModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, commMod, "monthly rate substring match must not bypass detection")

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}

func TestCheckSubscriptionSaaS_SaveDraftAuditFlags(t *testing.T) {
	doc, exp := percetakanDocumentFixture(t)

	t.Run("NarrativeModified_BothFlagsFalse", func(t *testing.T) {
		edited := strings.Replace(doc, "Melalui penawaran ini", "Melalui penawaran resmi yang disesuaikan ini", 1)
		commMod, err := CheckSubscriptionSaaSCommercialModified(edited, exp)
		require.NoError(t, err)
		assert.False(t, commMod)

		legalMod, err := CheckSubscriptionSaaSLegalModified(edited, exp)
		require.NoError(t, err)
		assert.False(t, legalMod)
	})

	t.Run("MissingCommercial_CommercialFlagTrue", func(t *testing.T) {
		edited := strings.Replace(doc, `data-block-type="commercial"`, `data-block-type="other"`, 1)
		commMod, err := CheckSubscriptionSaaSCommercialModified(edited, exp)
		require.NoError(t, err)
		assert.True(t, commMod)

		legalMod, err := CheckSubscriptionSaaSLegalModified(edited, exp)
		require.NoError(t, err)
		assert.False(t, legalMod)
	})

	t.Run("MissingPayment_CommercialFlagTrue", func(t *testing.T) {
		edited := strings.Replace(doc, `data-block-type="payment"`, `data-block-type="other"`, 1)
		commMod, err := CheckSubscriptionSaaSCommercialModified(edited, exp)
		require.NoError(t, err)
		assert.True(t, commMod)

		legalMod, err := CheckSubscriptionSaaSLegalModified(edited, exp)
		require.NoError(t, err)
		assert.False(t, legalMod)
	})

	t.Run("MissingLegal_LegalFlagTrue", func(t *testing.T) {
		edited := strings.Replace(doc, `data-block-type="legal"`, `data-block-type="other"`, 1)
		commMod, err := CheckSubscriptionSaaSCommercialModified(edited, exp)
		require.NoError(t, err)
		assert.False(t, commMod)

		legalMod, err := CheckSubscriptionSaaSLegalModified(edited, exp)
		require.NoError(t, err)
		assert.True(t, legalMod)
	})

	t.Run("EmptyLegalHTMLExpectation_ChecksProfileOnly", func(t *testing.T) {
		expWithoutLegal := exp
		expWithoutLegal.LegalHTML = ""

		legalMod, err := CheckSubscriptionSaaSLegalModified(doc, expWithoutLegal)
		require.NoError(t, err)
		assert.False(t, legalMod)

		// Modify income sharing text in document
		tampered := strings.Replace(doc, "50% untuk Mitra", "30% untuk Mitra", 1)
		legalMod, err = CheckSubscriptionSaaSLegalModified(tampered, expWithoutLegal)
		require.NoError(t, err)
		assert.True(t, legalMod)
	})
}

func TestValidateSubscriptionSaaSIntegrity_LegalTemplateBeginningWithH2Passes(t *testing.T) {
	input := validPercetakanTestInput()
	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	legalHTML := "<h2>Ketentuan</h2><h3>Pernyataan Pembuka</h3><p>Isi ketentuan penawaran...</p>"
	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/08/0002",
		Subject:           "Pengembangan Subscription AI Percetakan",
		ClientName:        "Bpk. Hendra",
		ClientCompanyName: "PT Percetakan Jaya",
		IssueDate:         "1 Agustus 2026",
	}

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         input,
		Calculation:   calc,
		Profile:       profile,
		FeatureGroups: input.FeatureGroups,
		LegalHTML:     legalHTML,
	})
	require.NoError(t, err)

	exp := SubscriptionSaaSIntegrityExpectations{
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   legalHTML,
	}

	legalMod, err := CheckSubscriptionSaaSLegalModified(docHTML, exp)
	require.NoError(t, err)
	assert.False(t, legalMod, "legal template starting with h2 must not be treated as modified")

	err = ValidateSubscriptionSaaSIntegrity(docHTML, exp)
	require.NoError(t, err, "validation must pass when legal template begins with h2")
}

func TestValidateSubscriptionSaaSIntegrity_ModifiedNestedLegalH2Fails(t *testing.T) {
	input := validPercetakanTestInput()
	calc, err := CalculateSubscriptionSaaS(input)
	require.NoError(t, err)
	profile, err := ResolveSubscriptionSaaSProfile(input)
	require.NoError(t, err)

	legalHTML := "<h2>Ketentuan</h2><h3>Pernyataan Pembuka</h3><p>Isi ketentuan penawaran...</p>"
	metadata := QuotationMetadata{
		QuotationNumber:   "QTN/2026/08/0002",
		Subject:           "Pengembangan Subscription AI Percetakan",
		ClientName:        "Bpk. Hendra",
		ClientCompanyName: "PT Percetakan Jaya",
		IssueDate:         "1 Agustus 2026",
	}

	renderer := NewSubscriptionSaaSHTMLRenderer()
	docHTML, err := renderer.Render(SubscriptionSaaSDocumentInput{
		Metadata:      metadata,
		Input:         input,
		Calculation:   calc,
		Profile:       profile,
		FeatureGroups: input.FeatureGroups,
		LegalHTML:     legalHTML,
	})
	require.NoError(t, err)

	exp := SubscriptionSaaSIntegrityExpectations{
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   legalHTML,
	}

	tampered := strings.Replace(docHTML, "<h2>Ketentuan</h2>", "<h2>Ketentuan Baru</h2>", 1)

	legalMod, err := CheckSubscriptionSaaSLegalModified(tampered, exp)
	require.NoError(t, err)
	assert.True(t, legalMod, "tampering nested h2 inside legal block must be detected")

	err = ValidateSubscriptionSaaSIntegrity(tampered, exp)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsafeHTML)
}
