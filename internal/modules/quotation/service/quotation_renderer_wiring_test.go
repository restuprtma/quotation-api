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

func TestWiring_RealRateAllocatorFeedsRenderer(t *testing.T) {
	allocator := service.NewRateAllocator()
	renderer := service.NewHTMLRenderer()
	sanitizer := service.NewHTMLSanitizer()
	validator := service.NewDOMValidator()

	rateCardItems := []domain.RateCardItem{
		{ID: uuid.New(), CompanyID: uuid.New(), RateCardID: uuid.New(), Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: 12_000_000},
		{ID: uuid.New(), CompanyID: uuid.New(), RateCardID: uuid.New(), Role: "quality_assurance", Seniority: "intermediate", TermType: "long_term", AmountPerMonth: 8_000_000},
	}

	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{
			{Role: "programmer", Seniority: "senior", EmployeeCount: 1, PeriodMonths: 3},
			{Role: "quality_assurance", Seniority: "intermediate", EmployeeCount: 2, PeriodMonths: 3},
		},
	}

	enriched, err := allocator.Calculate(proposal, rateCardItems, 3, 200_000_000, "IDR")
	require.NoError(t, err)

	legalHTML := `<p>Syarat dan ketentuan resmi perusahaan.</p>`
	critical, err := renderer.RenderCriticalBlocks(service.CriticalBlocksInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber: "QTN/WIRING/001",
			Subject:         "Wiring Verification",
			ClientName:      "PT Wiring",
			IssueDate:       "26 Agustus 2026",
		},
		Allocation: enriched,
		Payment: dto.PaymentTermsInput{
			Mode: "milestone",
			Items: []dto.PaymentTermItem{
				{MilestoneName: "DP", Percentage: 50},
				{MilestoneName: "Retensi", Percentage: 50},
			},
		},
		LegalHTML: legalHTML,
	})
	require.NoError(t, err)

	assembled, err := renderer.Assemble(critical, `<p>Proposal kerja sama dedicated squad.</p>`)
	require.NoError(t, err)

	sanitized, err := sanitizer.Sanitize(assembled)
	require.NoError(t, err)

	lines := make([]service.ExpectedLine, 0, len(enriched.Resources))
	for _, res := range enriched.Resources {
		lines = append(lines, service.ExpectedLine{Role: res.Role, Seniority: res.Seniority, LineTotal: res.LineTotal})
	}

	require.NoError(t, validator.ValidateIntegrity(sanitized, service.IntegrityExpectations{
		Currency:        enriched.Currency,
		TotalInvestment: enriched.TotalInvestment,
		Lines:           lines,
		LegalHTML:       legalHTML,
		BudgetMaximum:   nil,
	}))

	require.Len(t, enriched.Resources, 2)
	assert.Equal(t, int64(36_000_000), enriched.Resources[0].LineTotal)
	assert.Equal(t, int64(48_000_000), enriched.Resources[1].LineTotal)
	assert.Equal(t, int64(84_000_000), enriched.TotalInvestment)
	assert.Contains(t, sanitized, fmt.Sprintf(`data-field="total_investment"><strong>Rp %s</strong></td>`, "84.000.000"))
	for _, res := range enriched.Resources {
		assert.Contains(t, sanitized, fmt.Sprintf(`data-role="%s_%s"`, res.Role, res.Seniority))
	}

	assert.NotContains(t, sanitized, "200.000.000")
	assert.NotContains(t, strings.ToLower(sanitized), "budget")
}

func TestWiring_TamperingAfterSanitizeIsDetected(t *testing.T) {
	allocator := service.NewRateAllocator()
	renderer := service.NewHTMLRenderer()
	sanitizer := service.NewHTMLSanitizer()
	validator := service.NewDOMValidator()

	rateCardItems := []domain.RateCardItem{
		{ID: uuid.New(), CompanyID: uuid.New(), RateCardID: uuid.New(), Role: "programmer", Seniority: "junior", TermType: "short_term", AmountPerMonth: 9_000_000},
	}
	proposal := dto.AllocationProposal{
		Resources: []dto.AllocationResource{{Role: "programmer", Seniority: "junior", EmployeeCount: 1, PeriodMonths: 2}},
	}

	enriched, err := allocator.Calculate(proposal, rateCardItems, 2, 100_000_000, "IDR")
	require.NoError(t, err)
	require.Equal(t, "short_term", enriched.TermType)

	legalHTML := `<p>Legal resmi.</p>`
	critical, err := renderer.RenderCriticalBlocks(service.CriticalBlocksInput{
		Metadata:   service.QuotationMetadata{QuotationNumber: "QTN/TAMPER/001", Subject: "T", ClientName: "C"},
		Allocation: enriched,
		Payment:    dto.PaymentTermsInput{Mode: "milestone", Items: []dto.PaymentTermItem{{MilestoneName: "Full", Percentage: 100}}},
		LegalHTML:  legalHTML,
	})
	require.NoError(t, err)
	assembled, err := renderer.Assemble(critical, `<p>naratif</p>`)
	require.NoError(t, err)
	sanitized, err := sanitizer.Sanitize(assembled)
	require.NoError(t, err)

	tampered := strings.Replace(sanitized, "18.000.000", "50.000.000", 1)
	lines := []service.ExpectedLine{{Role: "programmer", Seniority: "junior", LineTotal: enriched.Resources[0].LineTotal}}

	err = validator.ValidateIntegrity(tampered, service.IntegrityExpectations{
		Currency:        enriched.Currency,
		TotalInvestment: enriched.TotalInvestment,
		Lines:           lines,
		LegalHTML:       legalHTML,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}
