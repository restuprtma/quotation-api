package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadDedicatedBenchmarkFixture(t *testing.T, filename string) dedicatedSquadBenchmarkFixture {
	t.Helper()
	path := filepath.Join("testdata", "dedicated_squad", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading fixture %s", filename)
	var fixture dedicatedSquadBenchmarkFixture
	require.NoError(t, json.Unmarshal(data, &fixture), "parsing fixture %s", filename)
	return fixture
}

func renderDedicatedBenchmark(t *testing.T, fixture dedicatedSquadBenchmarkFixture) (string, ResolvedQuotationProfile) {
	t.Helper()
	profile, err := ResolveDedicatedSquadProfile(fixture.Input)
	require.NoError(t, err)

	renderer := NewDedicatedSquadHTMLRenderer()
	metadata := QuotationMetadata{
		QuotationNumber:   fixture.Metadata.QuotationNumber,
		Subject:           fixture.Metadata.Subject,
		ClientName:        fixture.Metadata.ClientName,
		ClientCompanyName: fixture.Metadata.ClientCompany,
		IssueDate:         fixture.Metadata.IssueDate,
	}
	html, err := renderer.Render(DedicatedSquadDocumentInput{
		Metadata:    metadata,
		Calculation: fixture.Expected.Calculation,
		Profile:     profile,
		Rules:       fixture.Expected.Rules,
		Timeline:    fixture.Expected.Timeline,
	})
	require.NoError(t, err)
	return html, profile
}

func TestDedicatedSquadDOMValidator_CanonicalFixturesPass(t *testing.T) {
	fixtures := []string{
		"sow-250jt.json",
		"sow-50jt.json",
		"kopi-studio.json",
		"cuit.json",
	}

	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			fx := loadDedicatedBenchmarkFixture(t, name)
			rawHTML, profile := renderDedicatedBenchmark(t, fx)

			sanitizer := NewHTMLSanitizer()
			sanitized, err := sanitizer.Sanitize(rawHTML)
			require.NoError(t, err)

			expected := DedicatedSquadIntegrityExpectations{
				Input:       fx.Input,
				Calculation: fx.Expected.Calculation,
				Profile:     profile,
				Rules:       fx.Expected.Rules,
			}

			err = ValidateDedicatedSquadIntegrity(sanitized, expected)
			require.NoError(t, err, "canonical sanitized HTML must pass integrity check")

			commMod, err := CheckDedicatedSquadCommercialModified(sanitized, expected)
			require.NoError(t, err)
			assert.False(t, commMod, "unmodified HTML must have commercialModified=false")

			legalMod, err := CheckDedicatedSquadLegalModified(sanitized, expected)
			require.NoError(t, err)
			assert.False(t, legalMod, "unmodified HTML must have legalModified=false")
		})
	}
}

func TestDedicatedSquadDOMValidator_CommercialModifications(t *testing.T) {
	fx := loadDedicatedBenchmarkFixture(t, "sow-50jt.json")
	rawHTML, profile := renderDedicatedBenchmark(t, fx)
	sanitized, err := NewHTMLSanitizer().Sanitize(rawHTML)
	require.NoError(t, err)

	expected := DedicatedSquadIntegrityExpectations{
		Input:       fx.Input,
		Calculation: fx.Expected.Calculation,
		Profile:     profile,
		Rules:       fx.Expected.Rules,
	}

	tests := []struct {
		name        string
		target      string
		replacement string
	}{
		{
			name:        "total investment changed",
			target:      `data-field="total_investment"><strong>Rp 66.290.000</strong>`,
			replacement: `data-field="total_investment"><strong>Rp 70.000.000</strong>`,
		},
		{
			name:        "line total changed",
			target:      `data-field="line_total">Rp 13.370.000</td>`,
			replacement: `data-field="line_total">Rp 15.000.000</td>`,
		},
		{
			name:        "payment nominal changed",
			target:      `data-field="payment_nominal">Rp 22.096.666</td>`,
			replacement: `data-field="payment_nominal">Rp 25.000.000</td>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modified := strings.Replace(sanitized, tc.target, tc.replacement, 1)
			require.NotEqual(t, sanitized, modified, "target substring must exist")

			// Structure is still valid
			err := ValidateDedicatedSquadStructureForDraft(modified, expected)
			require.NoError(t, err)

			commMod, err := CheckDedicatedSquadCommercialModified(modified, expected)
			require.NoError(t, err)
			assert.True(t, commMod, "commercial modification must be detected")

			legalMod, err := CheckDedicatedSquadLegalModified(modified, expected)
			require.NoError(t, err)
			assert.False(t, legalMod, "legal should remain unmodified")
		})
	}
}

func TestDedicatedSquadDOMValidator_LegalModifications(t *testing.T) {
	fx := loadDedicatedBenchmarkFixture(t, "sow-50jt.json")
	rawHTML, profile := renderDedicatedBenchmark(t, fx)
	sanitized, err := NewHTMLSanitizer().Sanitize(rawHTML)
	require.NoError(t, err)

	expected := DedicatedSquadIntegrityExpectations{
		Input:       fx.Input,
		Calculation: fx.Expected.Calculation,
		Profile:     profile,
		Rules:       fx.Expected.Rules,
	}

	tests := []struct {
		name        string
		target      string
		replacement string
	}{
		{
			name:        "exclusion clause edited",
			target:      "Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.",
			replacement: "Biaya akomodasi khusus ditanggung oleh klien jika pekerjaan on-site.",
		},
		{
			name:        "tax clause in payment edited",
			target:      "Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.",
			replacement: "Seluruh rincian biaya sudah termasuk PPN 11%.",
		},
		{
			name:        "invoice clause in payment edited",
			target:      "Tagihan dikirimkan sesuai tanggal invoice dan wajib dilunasi dalam jangka waktu H+14 (empat belas) hari kalender.",
			replacement: "Tagihan dikirimkan dan dilunasi H+30 hari.",
		},
		{
			name:        "binding clause edited",
			target:      "Dokumen penawaran disepakati sebagai bagian yang tidak terpisahkan dari Service Agreement Venturo.",
			replacement: "Dokumen penawaran tunduk pada perjanjian terpisah.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modified := strings.Replace(sanitized, tc.target, tc.replacement, 1)
			require.NotEqual(t, sanitized, modified, "target substring must exist")

			// Structure is still valid
			err := ValidateDedicatedSquadStructureForDraft(modified, expected)
			require.NoError(t, err)

			legalMod, err := CheckDedicatedSquadLegalModified(modified, expected)
			require.NoError(t, err)
			assert.True(t, legalMod, "legal modification must be detected")

			commMod, err := CheckDedicatedSquadCommercialModified(modified, expected)
			require.NoError(t, err)
			assert.False(t, commMod, "commercial should remain unmodified")
		})
	}
}

func TestDedicatedSquadDOMValidator_StructureRejections(t *testing.T) {
	fx := loadDedicatedBenchmarkFixture(t, "sow-50jt.json")
	rawHTML, profile := renderDedicatedBenchmark(t, fx)
	sanitized, err := NewHTMLSanitizer().Sanitize(rawHTML)
	require.NoError(t, err)

	expected := DedicatedSquadIntegrityExpectations{
		Input:       fx.Input,
		Calculation: fx.Expected.Calculation,
		Profile:     profile,
		Rules:       fx.Expected.Rules,
	}

	tests := []struct {
		name        string
		target      string
		replacement string
	}{
		{
			name:        "missing payment section",
			target:      `<section data-block-type="payment"`,
			replacement: `<section data-block-type="corrupted_payment"`,
		},
		{
			name:        "missing commercial section",
			target:      `<section data-block-type="commercial"`,
			replacement: `<section data-block-type="corrupted_comm"`,
		},
		{
			name:        "missing legal section",
			target:      `<section data-block-type="legal"`,
			replacement: `<section data-block-type="corrupted_legal"`,
		},
		{
			name:        "duplicate total_investment field",
			target:      `<td data-field="total_investment">`,
			replacement: `<td data-field="total_investment"></td><td data-field="total_investment">`,
		},
		{
			name:        "missing root article class",
			target:      `class="quotation-document"`,
			replacement: `class="plain-article"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			modified := strings.Replace(sanitized, tc.target, tc.replacement, 1)
			require.NotEqual(t, sanitized, modified, "target substring must exist")

			err := ValidateDedicatedSquadStructureForDraft(modified, expected)
			require.Error(t, err, "structural defect must be rejected")
			assert.ErrorIs(t, err, ErrUnsafeHTML)
		})
	}
}
