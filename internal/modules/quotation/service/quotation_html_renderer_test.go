package service_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func sampleEnrichedAllocation() dto.EnrichedAllocation {
	return dto.EnrichedAllocation{
		Resources: []dto.EnrichedResource{
			{
				Role:           "programmer",
				Seniority:      "senior",
				TermType:       "long_term",
				EmployeeCount:  1,
				PeriodMonths:   3,
				AmountPerMonth: 12_000_000,
				LineTotal:      36_000_000,
			},
			{
				Role:           "quality_assurance",
				Seniority:      "intermediate",
				TermType:       "long_term",
				EmployeeCount:  2,
				PeriodMonths:   3,
				AmountPerMonth: 8_000_000,
				LineTotal:      48_000_000,
			},
		},
		Currency:        "IDR",
		TermType:        "long_term",
		TotalInvestment: 84_000_000,
	}
}

func sampleCriticalInput() service.CriticalBlocksInput {
	return service.CriticalBlocksInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber: "QTN/2026/08/0001",
			Subject:         "Pengembangan Customer Portal",
			ClientName:      "PT Contoh Jaya",
			IssueDate:       "26 Agustus 2026",
		},
		Allocation: sampleEnrichedAllocation(),
		Payment: dto.PaymentTermsInput{
			Mode: "milestone",
			Items: []dto.PaymentTermItem{
				{MilestoneName: "DP", Percentage: 30, Description: "Down payment"},
				{MilestoneName: "Pelunasan", Percentage: 70, Description: "Setelah serah terima"},
			},
		},
		LegalHTML: `<p>Dengan hormat,</p><ul><li>Penawaran berlaku 30 hari.</li></ul>`,
	}
}

func TestRenderCriticalBlocks_Metadata(t *testing.T) {
	r := service.NewHTMLRenderer()

	out, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	assert.Contains(t, out, `data-block-type="metadata"`)
	assert.Contains(t, out, "QTN/2026/08/0001")
	assert.Contains(t, out, "PT Contoh Jaya")
	assert.Contains(t, out, "26 Agustus 2026")
}

func TestRenderCriticalBlocks_MetadataWithClientCompanyName(t *testing.T) {
	r := service.NewHTMLRenderer()
	in := sampleCriticalInput()
	in.Metadata.ClientName = "Bpk. Budi Santoso"
	in.Metadata.ClientCompanyName = "PT Maju Bersama Digital"

	out, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)

	assert.Contains(t, out, `data-block-type="metadata"`)
	assert.Contains(t, out, "Bpk. Budi Santoso")
	assert.Contains(t, out, "PT Maju Bersama Digital")
	assert.Contains(t, out, "<p><strong>Perusahaan:</strong> PT Maju Bersama Digital</p>")
}

func TestRenderCriticalBlocks_MetadataEscapesInjectedHTML(t *testing.T) {
	in := sampleCriticalInput()
	in.Metadata.Subject = `<script>alert('meta')</script>`
	in.Metadata.ClientName = `PT <b>Ampersand & Sons</b>`
	r := service.NewHTMLRenderer()

	out, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)

	assert.NotContains(t, out, "<script>")
	assert.NotContains(t, out, "<b>Ampersand")
	assert.Contains(t, out, "&lt;script&gt;")
}

func TestRenderCriticalBlocks_InvestmentTableMatchesEnrichedLines(t *testing.T) {
	r := service.NewHTMLRenderer()
	allocation := sampleCriticalInput().Allocation

	out, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	assert.Contains(t, out, `data-block-type="commercial"`)
	assert.Contains(t, out, `data-role="programmer_senior"`)
	assert.Contains(t, out, `data-role="quality_assurance_intermediate"`)

	for _, res := range allocation.Resources {
		assert.Contains(t, out, fmt.Sprintf(`data-role="%s_%s"`, res.Role, res.Seniority))
	}
	assert.Contains(t, out, "12.000.000")
	assert.Contains(t, out, "8.000.000")
	assert.Contains(t, out, "36.000.000")
	assert.Contains(t, out, "48.000.000")
	assert.Contains(t, out, ">2<")
}

func TestRenderCriticalBlocks_TotalInvestmentExact(t *testing.T) {
	r := service.NewHTMLRenderer()
	allocation := sampleCriticalInput().Allocation
	require.Equal(t, int64(84_000_000), allocation.TotalInvestment)

	out, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	expectedTotal := fmt.Sprintf(`data-field="total_investment"><strong>Rp %s</strong></td>`, "84.000.000")
	assert.Contains(t, out, expectedTotal)
	assert.Equal(t, 1, strings.Count(out, `data-field="total_investment"`))
}

func TestRenderCriticalBlocks_PaymentSchedule(t *testing.T) {
	r := service.NewHTMLRenderer()

	out, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	assert.Contains(t, out, `data-block-type="payment"`)
	assert.Contains(t, out, "DP")
	assert.Contains(t, out, "Pelunasan")
	assert.Contains(t, out, ">30%<")
	assert.Contains(t, out, ">70%<")
}

func TestRenderCriticalBlocks_PaymentPercentageSumMustBe100(t *testing.T) {
	in := sampleCriticalInput()
	in.Payment.Items = []dto.PaymentTermItem{
		{MilestoneName: "DP", Percentage: 30},
		{MilestoneName: "Pelunasan", Percentage: 60},
	}
	r := service.NewHTMLRenderer()

	_, err := r.RenderCriticalBlocks(in)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrInvalidPaymentTerms)
}

func TestRenderCriticalBlocks_PaymentEmptyItemsRejected(t *testing.T) {
	in := sampleCriticalInput()
	in.Payment.Items = nil
	r := service.NewHTMLRenderer()

	_, err := r.RenderCriticalBlocks(in)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrInvalidPaymentTerms)
}

func TestRenderCriticalBlocks_LegalVerbatim(t *testing.T) {
	r := service.NewHTMLRenderer()
	legal := `<p>Dengan hormat,</p><ul><li>Penawaran berlaku 30 hari.</li></ul>`

	out, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	start := strings.Index(out, `data-block-type="legal">`)
	require.Greater(t, start, -1)
	rest := out[start+len(`data-block-type="legal">`):]
	end := strings.Index(rest, `</section>`)
	require.Greater(t, end, -1)
	rendered := rest[:end]

	assert.Equal(t, legal, rendered)
}

func TestRenderCriticalBlocks_NoInternalBudgetData(t *testing.T) {
	in := sampleCriticalInput()
	in.Allocation.UtilizationPercent = 84
	r := service.NewHTMLRenderer()

	out, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)

	for _, leaked := range []string{"budget", "Budget", "BUDGET", "utilization", "Utilization", "sisa", "Sisa", "unused"} {
		assert.NotContains(t, out, leaked)
	}
	assert.NotContains(t, out, "84%")
}

func TestRenderCriticalBlocks_EmptyResourcesRejected(t *testing.T) {
	in := sampleCriticalInput()
	in.Allocation.Resources = nil
	r := service.NewHTMLRenderer()

	_, err := r.RenderCriticalBlocks(in)

	require.Error(t, err)
}

func assembleForTest(t *testing.T) (string, service.IntegrityExpectations) {
	t.Helper()
	r := service.NewHTMLRenderer()

	in := sampleCriticalInput()
	critical, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)

	full, err := r.Assemble(critical, `<section data-block-type="opening"><p>Terima kasih atas kepercayaan Bapak/Ibu.</p></section>`)
	require.NoError(t, err)

	lines := make([]service.ExpectedLine, 0, len(in.Allocation.Resources))
	for _, res := range in.Allocation.Resources {
		lines = append(lines, service.ExpectedLine{Role: res.Role, Seniority: res.Seniority, LineTotal: res.LineTotal})
	}
	exp := service.IntegrityExpectations{
		Currency:        in.Allocation.Currency,
		TotalInvestment: in.Allocation.TotalInvestment,
		Lines:           lines,
		LegalHTML:       in.LegalHTML,
		BudgetMaximum:   nil,
	}
	return full, exp
}

func TestAssemble_DocumentStructure(t *testing.T) {
	r := service.NewHTMLRenderer()
	critical, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	full, err := r.Assemble(critical, `<section data-block-type="opening"><p>Naratif pembuka.</p></section>`)
	require.NoError(t, err)

	assert.Contains(t, full, `data-block-type="metadata"`)
	assert.Contains(t, full, `data-block-type="opening"`)
	assert.Contains(t, full, `data-block-type="commercial"`)
	assert.Less(t, strings.Index(full, `data-block-type="metadata"`), strings.Index(full, `data-block-type="opening"`))
	assert.Less(t, strings.Index(full, `data-block-type="opening"`), strings.Index(full, `data-block-type="commercial"`))
	assert.Less(t, strings.Index(full, `data-block-type="commercial"`), strings.Index(full, `data-block-type="payment"`))
	assert.Less(t, strings.Index(full, `data-block-type="payment"`), strings.Index(full, `data-block-type="legal"`))
}

func TestAssemble_EmptyCriticalRejected(t *testing.T) {
	r := service.NewHTMLRenderer()

	_, err := r.Assemble("", `<p>n</p>`)
	require.Error(t, err)
}

func TestAssemble_EmptyNarrativeRejected(t *testing.T) {
	r := service.NewHTMLRenderer()
	critical, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	_, err = r.Assemble(critical, "   ")
	require.Error(t, err)
}

func TestAssemble_RejectsMarkerSpoofInNarrative(t *testing.T) {
	r := service.NewHTMLRenderer()
	critical, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	spoof := `<p>Harga kami:</p><td data-field="total_investment">Rp 1.000.000</td>`
	_, err = r.Assemble(critical, spoof)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_CleanDocumentPasses(t *testing.T) {
	doc, exp := assembleForTest(t)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(doc, exp)
	require.NoError(t, err)
}

func TestDOMValidator_DetectsModifiedTotalInvestment(t *testing.T) {
	doc, exp := assembleForTest(t)
	tampered := strings.Replace(doc, "84.000.000", "99.000.000", 1)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsRemovedTotalRow(t *testing.T) {
	doc, exp := assembleForTest(t)
	removedIdx := strings.Index(doc, `<tr><td colspan="5"><strong>TOTAL INVESTMENT</strong></td>`)
	endIdx := strings.Index(doc[removedIdx:], "</tr>") + removedIdx + len("</tr>")
	tampered := doc[:removedIdx] + doc[endIdx:]
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsModifiedLineTotal(t *testing.T) {
	doc, exp := assembleForTest(t)
	tampered := strings.Replace(doc, "36.000.000", "13.000.000", 1)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsMissingLegalBlock(t *testing.T) {
	doc, exp := assembleForTest(t)
	start := strings.Index(doc, `<section data-block-type="legal"`)
	end := strings.Index(doc[start:], "</section>") + start + len("</section>")
	tampered := doc[:start] + doc[end:]
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsSingleCharacterLegalModification(t *testing.T) {
	doc, exp := assembleForTest(t)
	tampered := strings.Replace(doc, "berlaku 30 hari", "berlaku 31 hari", 1)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsDuplicateTotalMarker(t *testing.T) {
	doc, exp := assembleForTest(t)
	injected := strings.Replace(doc,
		`<p>Terima kasih atas kepercayaan Bapak/Ibu.</p>`,
		`<p>Terima kasih.</p><div><span data-field="total_investment">Rp 1.000.000</span></div>`,
		1)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(injected, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsBudgetDigitsLeak(t *testing.T) {
	doc, exp := assembleForTest(t)
	exp.BudgetMaximum = ptrInt64(100_000_000)
	injected := strings.Replace(doc,
		`<p>Terima kasih atas kepercayaan Bapak/Ibu.</p>`,
		`<p>Batas anggaran Anda 100.000.000.</p>`,
		1)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(injected, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_BudgetEqualToTotalDoesNotFalsePositive(t *testing.T) {
	doc, exp := assembleForTest(t)
	exp.BudgetMaximum = ptrInt64(exp.TotalInvestment)
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(doc, exp)
	require.NoError(t, err)
}

func TestDOMValidator_LegalWithQuotesApostrophesAndAmpersandsPasses(t *testing.T) {
	r := service.NewHTMLRenderer()

	legal := `<p>Klien "PT Nusantara & Partners" bertanggung jawab penuh atas data client's area.</p>` +
		`<ul><li>Penawaran & dokumen terkait berlaku 30 hari.</li></ul>`
	in := sampleCriticalInput()
	in.LegalHTML = legal
	critical, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)

	full, err := r.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)

	sanitizer := service.NewHTMLSanitizer()
	sanitized, err := sanitizer.Sanitize(full)
	require.NoError(t, err)

	v := service.NewDOMValidator()
	err = v.ValidateIntegrity(sanitized, service.IntegrityExpectations{
		Currency:        in.Allocation.Currency,
		TotalInvestment: in.Allocation.TotalInvestment,
		LegalHTML:       legal,
	})

	require.NoError(t, err, "legal snapshot dengan karakter escapable harus lolos validasi tanpa false positive")
}

func TestDOMValidator_LegalModificationWithSpecialCharactersDetected(t *testing.T) {
	r := service.NewHTMLRenderer()

	legal := `<p>Batas tanggung jawab "vendor" & klien disepakati bersama.</p>`
	in := sampleCriticalInput()
	in.LegalHTML = legal
	critical, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)
	full, err := r.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)
	sanitizer := service.NewHTMLSanitizer()
	sanitized, err := sanitizer.Sanitize(full)
	require.NoError(t, err)

	tamperedLegal := `<p>Batas tanggung jawab "vendee" & klien disepakati bersama.</p>`
	v := service.NewDOMValidator()
	err = v.ValidateIntegrity(sanitized, service.IntegrityExpectations{
		Currency:        in.Allocation.Currency,
		TotalInvestment: in.Allocation.TotalInvestment,
		LegalHTML:       tamperedLegal,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func ptrInt64(v int64) *int64 { return &v }

func TestRenderCriticalBlocks_MixedCaseRoleProducesLowercaseMarker(t *testing.T) {
	in := sampleCriticalInput()
	in.Allocation.Resources[0].Role = "Programmer"
	in.Allocation.Resources[0].Seniority = "Senior"
	r := service.NewHTMLRenderer()

	out, err := r.RenderCriticalBlocks(in)

	require.NoError(t, err)
	assert.Contains(t, out, `data-role="programmer_senior"`)
	assert.NotContains(t, out, `data-role="Programmer`)
}

func TestRenderCriticalBlocks_RoleWithIllegalCharactersRejected(t *testing.T) {
	in := sampleCriticalInput()
	in.Allocation.Resources[0].Seniority = `junior"><script>`
	r := service.NewHTMLRenderer()

	_, err := r.RenderCriticalBlocks(in)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrInvalidAllocationInput)
}

func TestAssemble_RejectsWhitespaceSeparatedMarkerInNarrative(t *testing.T) {
	r := service.NewHTMLRenderer()
	critical, err := r.RenderCriticalBlocks(sampleCriticalInput())
	require.NoError(t, err)

	spoof := `<p>Harga:</p><span data-field = "total_investment">Rp 1.000.000</span>`
	_, err = r.Assemble(critical, spoof)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsRemovedMetadataBlock(t *testing.T) {
	doc, exp := assembleForTest(t)
	start := strings.Index(doc, `<section data-block-type="metadata"`)
	require.Greater(t, start, -1)
	end := strings.Index(doc[start:], `</section>`) + start + len(`</section>`)
	tampered := doc[:start] + doc[end:]
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsRemovedInvestmentBlock(t *testing.T) {
	doc, exp := assembleForTest(t)
	start := strings.Index(doc, `<section data-block-type="commercial"`)
	require.Greater(t, start, -1)
	end := strings.Index(doc[start:], `</section>`) + start + len(`</section>`)
	tampered := doc[:start] + doc[end:]
	v := service.NewDOMValidator()

	err := v.ValidateIntegrity(tampered, exp)

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestWiring_LowercasedMixedCaseRolesStayConsistent(t *testing.T) {
	allocator := service.NewRateAllocator()
	renderer := service.NewHTMLRenderer()
	sanitizer := service.NewHTMLSanitizer()
	validator := service.NewDOMValidator()

	rateCardItems := []domain.RateCardItem{
		{ID: uuid.New(), CompanyID: uuid.New(), RateCardID: uuid.New(), Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: 12_000_000},
	}
	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{{Role: "programmer", Seniority: "senior", EmployeeCount: 1, PeriodMonths: 3}},
	}

	enriched, err := allocator.Calculate(proposal, rateCardItems, 3, 100_000_000, "IDR")
	require.NoError(t, err)

	critical, err := renderer.RenderCriticalBlocks(service.CriticalBlocksInput{
		Metadata:   service.QuotationMetadata{QuotationNumber: "QTN/CASE/001", Subject: "S", ClientName: "C"},
		Allocation: enriched,
		Payment:    dto.PaymentTermsInput{Mode: "milestone", Items: []dto.PaymentTermItem{{MilestoneName: "Full", Percentage: 100}}},
		LegalHTML:  `<p>Legal.</p>`,
	})
	require.NoError(t, err)

	assembled, err := renderer.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)
	sanitized, err := sanitizer.Sanitize(assembled)
	require.NoError(t, err)

	err = validator.ValidateIntegrity(sanitized, service.IntegrityExpectations{
		Currency:        enriched.Currency,
		TotalInvestment: enriched.TotalInvestment,
		Lines:           []service.ExpectedLine{{Role: enriched.Resources[0].Role, Seniority: enriched.Resources[0].Seniority, LineTotal: enriched.Resources[0].LineTotal}},
		LegalHTML:       `<p>Legal.</p>`,
	})
	require.NoError(t, err)
}

func TestDOMValidator_DetectsModifiedPaymentPercentage(t *testing.T) {
	r := service.NewHTMLRenderer()
	in := sampleCriticalInput()
	critical, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)
	full, err := r.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)

	tampered := strings.Replace(full, ">70%<", ">95%<", 1)
	require.NotEqual(t, full, tampered)

	v := service.NewDOMValidator()
	err = v.ValidateIntegrity(tampered, service.IntegrityExpectations{
		Currency:        in.Allocation.Currency,
		TotalInvestment: in.Allocation.TotalInvestment,
		PaymentItems: []service.ExpectedPayment{
			{Percentage: 30},
			{Percentage: 70},
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_DetectsRemovedPaymentRow(t *testing.T) {
	r := service.NewHTMLRenderer()
	in := sampleCriticalInput()
	critical, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)
	full, err := r.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)

	start := strings.Index(full, `<tr><td>Pelunasan</td>`)
	require.Greater(t, start, -1)
	end := strings.Index(full[start:], "</tr>") + start + len("</tr>")
	tampered := full[:start] + full[end:]

	v := service.NewDOMValidator()
	err = v.ValidateIntegrity(tampered, service.IntegrityExpectations{
		Currency:        in.Allocation.Currency,
		TotalInvestment: in.Allocation.TotalInvestment,
		PaymentItems: []service.ExpectedPayment{
			{Percentage: 30},
			{Percentage: 70},
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_PaymentExpectationsSatisfiedPasses(t *testing.T) {
	r := service.NewHTMLRenderer()
	in := sampleCriticalInput()
	critical, err := r.RenderCriticalBlocks(in)
	require.NoError(t, err)
	full, err := r.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)

	v := service.NewDOMValidator()
	err = v.ValidateIntegrity(full, service.IntegrityExpectations{
		Currency:        in.Allocation.Currency,
		TotalInvestment: in.Allocation.TotalInvestment,
		PaymentItems: []service.ExpectedPayment{
			{Percentage: 30},
			{Percentage: 70},
		},
		LegalHTML: in.LegalHTML,
	})

	require.NoError(t, err)
}

func TestDOMValidator_CheckLegalModified_MatchesReturnsFalse(t *testing.T) {
	v := service.NewDOMValidator()
	legalSnippet := `<p>Syarat &amp; Ketentuan: Pembayaran 30 hari.</p>`
	doc := fmt.Sprintf(`<section data-block-type="legal">%s</section>`, legalSnippet)

	modified, err := v.CheckLegalModified(doc, legalSnippet)
	require.NoError(t, err)
	assert.False(t, modified)
}

func TestDOMValidator_CheckLegalModified_DifferentTextReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	expected := `<p>Syarat &amp; Ketentuan Asli</p>`
	doc := `<section data-block-type="legal"><p>Syarat &amp; Ketentuan Diubah oleh BA</p></section>`

	modified, err := v.CheckLegalModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckLegalModified_MissingSectionReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	expected := `<p>Syarat &amp; Ketentuan Asli</p>`
	doc := `<section data-block-type="commercial"><p>Hanya investment</p></section>`

	modified, err := v.CheckLegalModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckLegalModified_EmptyExpectedReturnsFalse(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial"><p>Hanya investment</p></section>`

	modified, err := v.CheckLegalModified(doc, "")
	require.NoError(t, err)
	assert.False(t, modified)
}

func TestDOMValidator_CheckCommercialModified_MatchesReturnsFalse(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial">
		<table>
			<tr data-role="lead_developer_senior"><td data-field="line_total">35.000.000</td></tr>
			<tr><td data-field="total_investment">Rp 35.000.000</td></tr>
		</table>
	</section>
	<section data-block-type="payment">
		<table><tr><td data-field="payment_percentage">100%</td></tr></table>
	</section>`

	expected := service.IntegrityExpectations{
		Currency:        "IDR",
		TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{
			{Role: "lead_developer", Seniority: "senior", LineTotal: 35000000},
		},
		PaymentItems: []service.ExpectedPayment{
			{Percentage: 100},
		},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.False(t, modified)
}

func TestDOMValidator_CheckCommercialModified_EmptyExpectationsReturnsFalse(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial"><p>Any text</p></section>`

	modified, err := v.CheckCommercialModified(doc, service.IntegrityExpectations{})
	require.NoError(t, err)
	assert.False(t, modified)
}

func TestDOMValidator_CheckCommercialModified_ChangedTotalReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial">
		<table>
			<tr data-role="lead_developer_senior"><td data-field="line_total">35.000.000</td></tr>
			<tr><td data-field="total_investment">Rp 40.000.000</td></tr>
		</table>
	</section>`

	expected := service.IntegrityExpectations{
		Currency:        "IDR",
		TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{
			{Role: "lead_developer", Seniority: "senior", LineTotal: 35000000},
		},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckCommercialModified_MissingRowReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial">
		<table>
			<tr><td data-field="total_investment">Rp 35.000.000</td></tr>
		</table>
	</section>`

	expected := service.IntegrityExpectations{
		Currency:        "IDR",
		TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{
			{Role: "lead_developer", Seniority: "senior", LineTotal: 35000000},
		},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckCommercialModified_PaymentPercentageMismatchReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial">
		<table>
			<tr data-role="lead_developer_senior"><td data-field="line_total">35.000.000</td></tr>
			<tr><td data-field="total_investment">Rp 35.000.000</td></tr>
		</table>
	</section>
	<section data-block-type="payment">
		<table><tr><td data-field="payment_percentage">50%</td></tr></table>
	</section>`

	expected := service.IntegrityExpectations{
		Currency:        "IDR",
		TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{
			{Role: "lead_developer", Seniority: "senior", LineTotal: 35000000},
		},
		PaymentItems: []service.ExpectedPayment{
			{Percentage: 100},
		},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckCommercialModified_ChangedRenderedResourceFieldReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial"><table><tbody>
		<tr data-role="lead_developer_senior"><td>Lead Developer</td><td>Senior</td><td>2</td><td>1</td><td>35.000.000</td><td data-field="line_total">35.000.000</td></tr>
		<tr><td data-field="total_investment">Rp 35.000.000</td></tr>
	</tbody></table></section>`
	expected := service.IntegrityExpectations{
		Currency: "IDR", TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{{
			Role: "lead_developer", Seniority: "senior", EmployeeCount: 1,
			PeriodMonths: 1, AmountPerMonth: 35000000, LineTotal: 35000000,
		}},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckCommercialModified_ExtraResourceRowReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial"><table><tbody>
		<tr data-role="lead_developer_senior"><td>Lead Developer</td><td>Senior</td><td>1</td><td>1</td><td>35.000.000</td><td data-field="line_total">35.000.000</td></tr>
		<tr data-role="extra_role_senior"><td>Extra Role</td><td>Senior</td><td>1</td><td>1</td><td>1</td><td data-field="line_total">1</td></tr>
		<tr><td data-field="total_investment">Rp 35.000.000</td></tr>
	</tbody></table></section>`
	expected := service.IntegrityExpectations{
		Currency: "IDR", TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{{
			Role: "lead_developer", Seniority: "senior", EmployeeCount: 1,
			PeriodMonths: 1, AmountPerMonth: 35000000, LineTotal: 35000000,
		}},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckCommercialModified_MarkersOutsideInvestmentReturnTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial"><table><tr data-role="decoy"><td>Changed</td></tr></table></section>
	<section><table>
		<tr data-role="lead_developer_senior"><td>Lead Developer</td><td>Senior</td><td>1</td><td>1</td><td>35.000.000</td><td data-field="line_total">35.000.000</td></tr>
		<tr><td data-field="total_investment">Rp 35.000.000</td></tr>
	</table></section>`
	expected := service.IntegrityExpectations{
		Currency: "IDR", TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{{
			Role: "lead_developer", Seniority: "senior", EmployeeCount: 1,
			PeriodMonths: 1, AmountPerMonth: 35000000, LineTotal: 35000000,
		}},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_CheckCommercialModified_UnmarkedResourceRowReturnsTrue(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<section data-block-type="commercial"><table><tbody>
		<tr data-role="lead_developer_senior"><td>Lead Developer</td><td>Senior</td><td>1</td><td>1</td><td>35.000.000</td><td data-field="line_total">35.000.000</td></tr>
		<tr><td>Unmarked Role</td><td>Senior</td><td>1</td><td>1</td><td>1</td><td>1</td></tr>
		<tr><td colspan="5">Total</td><td data-field="total_investment">Rp 35.000.000</td></tr>
	</tbody></table></section>`
	expected := service.IntegrityExpectations{
		Currency: "IDR", TotalInvestment: 35000000,
		Lines: []service.ExpectedLine{{
			Role: "lead_developer", Seniority: "senior", EmployeeCount: 1,
			PeriodMonths: 1, AmountPerMonth: 35000000, LineTotal: 35000000,
		}},
	}

	modified, err := v.CheckCommercialModified(doc, expected)
	require.NoError(t, err)
	assert.True(t, modified)
}

func TestDOMValidator_ValidateDocumentSafety_ForbiddenTermsRejects(t *testing.T) {
	v := service.NewDOMValidator()
	doc := `<p>Dokumen ini menyebutkan budget internal 50000000.</p>`

	err := v.ValidateDocumentSafety(doc, nil, 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_ValidateDocumentSafety_BudgetLeakRejects(t *testing.T) {
	v := service.NewDOMValidator()
	budget := int64(100000000)
	doc := `<p>Biaya proyek adalah 100.000.000 rupiah.</p>`

	err := v.ValidateDocumentSafety(doc, &budget, 50000000)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestDOMValidator_ValidateDocumentSafety_BudgetLeakVariantsReject(t *testing.T) {
	v := service.NewDOMValidator()
	budget := int64(100000000)
	for _, doc := range []string{
		`<p>Nilai batas adalah 100000000 rupiah.</p>`,
		`<p>Nilai batas adalah 100,000,000 rupiah.</p>`,
		`<p>Nilai batas adalah 100 000 000 rupiah.</p>`,
		`<p>Nilai batas adalah <strong>100</strong><span>.000.000</span> rupiah.</p>`,
	} {
		t.Run(doc, func(t *testing.T) {
			err := v.ValidateDocumentSafety(doc, &budget, 50000000)
			require.Error(t, err)
			assert.ErrorIs(t, err, service.ErrUnsafeHTML)
		})
	}
}

func TestDOMValidator_ValidateDocumentSafety_BareTextBudgetLeakRejects(t *testing.T) {
	v := service.NewDOMValidator()
	budget := int64(100000000)
	err := v.ValidateDocumentSafety(`Nilai maksimum 100.000.000 rupiah.`, &budget, 50000000)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}
