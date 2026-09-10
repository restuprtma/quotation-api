package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

type talentBenchmarkExpected struct {
	TotalBeforeTax int64 `json:"total_before_tax"`
	DPPAmount      int64 `json:"dpp_amount"`
	TaxAmount      int64 `json:"tax_amount"`
	GrandTotal     int64 `json:"grand_total"`
	PaymentRows    int   `json:"payment_rows"`
}

type talentBenchmarkFixture struct {
	Source   map[string]string            `json:"source"`
	Request  dto.GenerateQuotationRequest `json:"request"`
	Expected talentBenchmarkExpected      `json:"expected"`
}

func loadTalentBenchmarkFixture(t *testing.T, file string) talentBenchmarkFixture {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(sourceFile), "testdata", "talent_outsourcing", file))
	require.NoError(t, err)
	var fixture talentBenchmarkFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.Equal(t, "spreadsheet.xlsx", fixture.Source["workbook"])
	require.NotEmpty(t, fixture.Source["sheet"])
	require.NotEmpty(t, fixture.Source["source_gid"])
	require.Equal(t, "2026-09-09", fixture.Source["verified_on"])
	return fixture
}

func TestTalentOutsourcingBenchmarks(t *testing.T) {
	tests := []struct {
		file, mode                  string
		wantTotal, wantDPP, wantTax int64
		wantPayments                int
		wantText                    string
	}{
		{"bpjs.json", "manpower_supply", 675_146_400, 557_553_333, 66_906_400, 5, "Biaya Langsung Non Personil"},
		{"sys.json", "managed_squad", 412_060_000, 0, 0, 13, "Hiring Fee"},
		{"mega-kreasi-tech.json", "managed_squad", 184_625_000, 0, 0, 4, "Hiring Fee"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			fixture := loadTalentBenchmarkFixture(t, tt.file)
			require.Equal(t, domain.CommercialSchemeTalentOutsourcing, fixture.Request.CommercialScheme)
			require.NotNil(t, fixture.Request.TalentOutsourcing)
			input := *fixture.Request.TalentOutsourcing
			require.Equal(t, tt.mode, string(input.EngagementMode))
			require.NoError(t, input.Validate())

			calc, err := CalculateTalentOutsourcing(input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantTotal, calc.GrandTotal)
			assert.Equal(t, tt.wantDPP, calc.DPPAmount)
			assert.Equal(t, tt.wantTax, calc.TaxAmount)
			assert.Equal(t, fixture.Expected.TotalBeforeTax, calc.TotalBeforeTax)
			assert.Equal(t, fixture.Expected.DPPAmount, calc.DPPAmount)
			assert.Equal(t, fixture.Expected.TaxAmount, calc.TaxAmount)
			assert.Equal(t, fixture.Expected.GrandTotal, calc.GrandTotal)
			assert.Equal(t, calc.GrandTotal, calc.PaymentTotal)
			assert.Len(t, input.PaymentSchedule, tt.wantPayments)
			assert.Equal(t, fixture.Expected.PaymentRows, len(input.PaymentSchedule))

			profile, err := ResolveTalentOutsourcingProfile(input)
			require.NoError(t, err)
			require.Len(t, profile.Sections, 4)
			for i, section := range profile.Sections {
				assert.Equal(t, i+1, section.Number)
			}
			if input.EngagementMode == domain.TalentModeManpowerSupply {
				assert.True(t, profile.ShowNonPersonnel)
				assert.False(t, profile.ShowPreparation)
			} else {
				assert.True(t, profile.ShowPreparation)
				assert.False(t, profile.ShowNonPersonnel)
				assert.NotEmpty(t, profile.TaxClauseText)
			}

			rawHTML, err := NewTalentOutsourcingRenderer().Render(TalentOutsourcingDocumentInput{Metadata: QuotationMetadata{QuotationNumber: input.DocumentNumber, Subject: fixture.Request.Subject}, Input: input, Calculation: calc, Profile: profile, LegalHTML: `<section data-block-type="legal"><p>Ketentuan benchmark</p></section>`})
			require.NoError(t, err)
			sanitized, err := NewHTMLSanitizer().Sanitize(rawHTML)
			require.NoError(t, err)
			require.NoError(t, ValidateTalentOutsourcingIntegrity(sanitized, TalentOutsourcingIntegrityExpectations{Input: input, Calculation: calc, Profile: profile}))
			assert.Contains(t, sanitized, `data-service="talent_outsourcing"`)
			assert.Contains(t, sanitized, formatTalentIDR(tt.wantTotal))
			assert.Contains(t, sanitized, tt.wantText)
			if tt.wantDPP == 0 {
				assert.NotContains(t, sanitized, `data-field="dpp_amount"`)
				assert.NotContains(t, sanitized, `data-field="tax_amount"`)
			}
			assert.False(t, strings.Contains(strings.ToLower(sanitized), "<script"))
		})
	}
}
