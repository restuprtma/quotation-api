package service_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

	assert.Contains(t, out, `data-block="metadata"`)
	assert.Contains(t, out, "QTN/2026/08/0001")
	assert.Contains(t, out, "PT Contoh Jaya")
	assert.Contains(t, out, "26 Agustus 2026")
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

	assert.Contains(t, out, `data-block="investment"`)
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

	assert.Contains(t, out, `data-block="payment_schedule"`)
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

	start := strings.Index(out, `data-block="legal">`)
	require.Greater(t, start, -1)
	rest := out[start+len(`data-block="legal">`):]
	end := strings.Index(rest, `</div><!-- /qtr-legal -->`)
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
