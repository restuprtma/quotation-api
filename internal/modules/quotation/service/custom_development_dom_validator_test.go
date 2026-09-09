package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func validDocumentSetup(t *testing.T) (string, CustomDevelopmentIntegrityExpectations) {
	docIn := sampleDocumentInput()
	renderer := NewCustomDevelopmentRenderer()
	htmlOut, err := renderer.Render(docIn)
	require.NoError(t, err)

	expectations := CustomDevelopmentIntegrityExpectations{
		Input:       docIn.Input,
		Calculation: docIn.Calculation,
		Profile:     docIn.Profile,
		LegalHTML:   docIn.LegalHTML,
	}
	return htmlOut, expectations
}

func TestValidateCustomDevelopmentIntegrity_ValidDocumentPasses(t *testing.T) {
	docHTML, expectations := validDocumentSetup(t)
	err := ValidateCustomDevelopmentIntegrity(docHTML, expectations)
	require.NoError(t, err)
}

func TestValidateCustomDevelopmentIntegrity_MutationsRejected(t *testing.T) {
	docHTML, expectations := validDocumentSetup(t)

	tests := []struct {
		name    string
		mutate  func(string) string
		wantErr string
	}{
		{
			name: "hapus section commercial-development",
			mutate: func(h string) string {
				root, _ := html.Parse(strings.NewReader(h))
				nodes := collectNodes(root)
				sec, _ := findExactlyOne(nodes, "data-quotation-section", "commercial-development")
				if sec != nil && sec.Parent != nil {
					sec.Parent.RemoveChild(sec)
				}
				out, _ := innerHTML(root)
				return out
			},
			wantErr: "commercial-development",
		},
		{
			name: "hapus section scope",
			mutate: func(h string) string {
				root, _ := html.Parse(strings.NewReader(h))
				nodes := collectNodes(root)
				sec, _ := findExactlyOne(nodes, "data-quotation-section", "scope")
				if sec != nil && sec.Parent != nil {
					sec.Parent.RemoveChild(sec)
				}
				out, _ := innerHTML(root)
				return out
			},
			wantErr: "scope",
		},
		{
			name: "hapus section timeline",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-section="timeline"`, `data-quotation-section="removed-timeline"`, 1)
			},
			wantErr: "timeline",
		},
		{
			name: "hapus section payment",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-section="payment"`, `data-quotation-section="removed-payment"`, 1)
			},
			wantErr: "payment",
		},
		{
			name: "hapus section limitations",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-section="limitations"`, `data-quotation-section="removed-limitations"`, 1)
			},
			wantErr: "limitations",
		},
		{
			name: "hapus legal marker",
			mutate: func(h string) string {
				return strings.Replace(h, `<section data-quotation-section="legal">`, `<div>`, 1)
			},
			wantErr: "legal",
		},
		{
			name: "hapus section signature",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-section="signature"`, `data-quotation-section="removed-signature"`, 1)
			},
			wantErr: "signature",
		},
		{
			name: "duplikasi section number",
			mutate: func(h string) string {
				// Change section 2's number to 1
				return strings.Replace(h, `data-quotation-section-number="2"`, `data-quotation-section-number="1"`, 1)
			},
			wantErr: "duplicate section number",
		},
		{
			name: "ubah total investment",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-field="total_investment"><strong>Rp 18.000.000</strong>`, `data-quotation-field="total_investment"><strong>Rp 10.000.000</strong>`, 1)
			},
			wantErr: "commercial",
		},
		{
			name: "ubah development subtotal",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-field="development_subtotal"><strong>Rp 20.062.500</strong>`, `data-quotation-field="development_subtotal"><strong>Rp 15.000.000</strong>`, 1)
			},
			wantErr: "commercial",
		},
		{
			name: "ubah monthly amount",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-field="monthly_line_0">Rp 800.000</td>`, `data-quotation-field="monthly_line_0">Rp 500.000</td>`, 1)
			},
			wantErr: "commercial",
		},
		{
			name: "ubah monthly total",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-field="monthly_total"><strong>Rp 1.272.000</strong>`, `data-quotation-field="monthly_total"><strong>Rp 900.000</strong>`, 1)
			},
			wantErr: "commercial",
		},
		{
			name: "ubah payment nominal",
			mutate: func(h string) string {
				return strings.Replace(h, `data-quotation-field="payment_nominal_0">Rp 9.000.000</td>`, `data-quotation-field="payment_nominal_0">Rp 5.000.000</td>`, 1)
			},
			wantErr: "commercial",
		},
		{
			name: "hapus one module feature",
			mutate: func(h string) string {
				return strings.Replace(h, `<li>Pencatatan order dan invoice</li>`, ``, 1)
			},
			wantErr: "feature",
		},
		{
			name: "ubah rights statement",
			mutate: func(h string) string {
				return strings.Replace(h, `Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan.`, `Hak cipta tetap menjadi milik Venturo selamanya.`, 1)
			},
			wantErr: "rights",
		},
		{
			name: "ubah limitations klausul katalog",
			mutate: func(h string) string {
				return strings.Replace(h, `Biaya akomodasi, transportasi luar kota, dan konsumsi harian`, `Biaya transportasi ditanggung penuh oleh vendor`, 1)
			},
			wantErr: "limitations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mutated := tt.mutate(docHTML)
			err := ValidateCustomDevelopmentIntegrity(mutated, expectations)
			require.Error(t, err, "mutation %q should be rejected", tt.name)
			assert.ErrorContains(t, err, tt.wantErr)
			assert.ErrorIs(t, err, ErrUnsafeHTML)
		})
	}
}

func TestValidateCustomDevelopmentIntegrity_UnexpectedMonthlySection(t *testing.T) {
	// Setup document without monthly
	input := sampleCustomDevelopmentInput()
	input.MonthlyItems = nil
	input.MonthlyNotes = nil
	input.PaymentSchedule = []domain.CustomDevelopmentPaymentRowInput{
		{
			WorkPeriod:       "Termin 1",
			Terms:            "Pelunasan",
			InvoiceDateLabel: "Kontrak",
			DueDateLabel:     "H+7",
			Component:        domain.CustomDevelopmentPaymentProject,
			Nominal:          18_000_000,
		},
	}
	calc, err := CalculateCustomDevelopment(input)
	require.NoError(t, err)
	profile, err := ResolveCustomDevelopmentProfile(input)
	require.NoError(t, err)

	renderer := NewCustomDevelopmentRenderer()
	docHTML, err := renderer.Render(CustomDevelopmentDocumentInput{
		Metadata: QuotationMetadata{
			QuotationNumber: "QTN/001",
			Subject:         "Subject",
			ClientName:      "Client",
		},
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   `<p>Legal clauses.</p>`,
	})
	require.NoError(t, err)

	expectations := CustomDevelopmentIntegrityExpectations{
		Input:       input,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   `<p>Legal clauses.</p>`,
	}

	// Valid document without monthly passes
	require.NoError(t, ValidateCustomDevelopmentIntegrity(docHTML, expectations))

	// Inject unexpected commercial-monthly section
	injected := strings.Replace(docHTML, `</article>`, `<section data-quotation-section="commercial-monthly"><p>fake monthly</p></section></article>`, 1)
	err = ValidateCustomDevelopmentIntegrity(injected, expectations)
	require.Error(t, err)
	assert.ErrorContains(t, err, "commercial-monthly")
}

func TestCheckCustomDevelopmentCommercialModified(t *testing.T) {
	docHTML, expectations := validDocumentSetup(t)

	t.Run("canonical document is not modified", func(t *testing.T) {
		modified, err := CheckCustomDevelopmentCommercialModified(docHTML, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.False(t, modified)
	})

	t.Run("tampered development line total is detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `data-quotation-field="development_line_0_0">Rp 15.000.000`, `data-quotation-field="development_line_0_0">Rp 1.000.000`, 1)
		modified, err := CheckCustomDevelopmentCommercialModified(tampered, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("tampered development subtotal is detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `data-quotation-field="development_subtotal"><strong>Rp 20.062.500</strong>`, `data-quotation-field="development_subtotal"><strong>Rp 10.000.000</strong>`, 1)
		modified, err := CheckCustomDevelopmentCommercialModified(tampered, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("tampered discount amount is detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `data-quotation-field="discount_amount"><strong>Rp 2.062.500</strong>`, `data-quotation-field="discount_amount"><strong>Rp 0</strong>`, 1)
		modified, err := CheckCustomDevelopmentCommercialModified(tampered, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("tampered monthly line is detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `data-quotation-field="monthly_line_0">Rp 800.000`, `data-quotation-field="monthly_line_0">Rp 999.000`, 1)
		modified, err := CheckCustomDevelopmentCommercialModified(tampered, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("tampered monthly note is detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `Biaya API Gemini ditandai out-of-pocket.`, `Biaya API Gemini gratis.`, 1)
		modified, err := CheckCustomDevelopmentCommercialModified(tampered, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("tampered payment nominal is detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `data-quotation-field="payment_nominal_0">Rp 9.000.000`, `data-quotation-field="payment_nominal_0">Rp 1.000.000`, 1)
		modified, err := CheckCustomDevelopmentCommercialModified(tampered, expectations.Input, expectations.Calculation)
		require.NoError(t, err)
		assert.True(t, modified)
	})
}

func TestCheckCustomDevelopmentRightsModified(t *testing.T) {
	docHTML, expectations := validDocumentSetup(t)

	t.Run("canonical rights not modified", func(t *testing.T) {
		modified, err := CheckCustomDevelopmentRightsModified(docHTML, expectations.Input.RightsStatements)
		require.NoError(t, err)
		assert.False(t, modified)
	})

	t.Run("modified statement detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `Hak cipta dan source code sistem diserahkan penuh kepada klien setelah pelunasan.`, `Hak cipta milik Venturo.`, 1)
		modified, err := CheckCustomDevelopmentRightsModified(tampered, expectations.Input.RightsStatements)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("missing rights block when expected returns modified", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `data-quotation-section="rights"`, `data-quotation-section="deleted-rights"`, 1)
		modified, err := CheckCustomDevelopmentRightsModified(tampered, expectations.Input.RightsStatements)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("unexpected rights block when expected empty returns modified", func(t *testing.T) {
		modified, err := CheckCustomDevelopmentRightsModified(docHTML, nil)
		require.NoError(t, err)
		assert.True(t, modified)
	})
}

func TestCheckCustomDevelopmentLimitationsModified(t *testing.T) {
	docHTML, expectations := validDocumentSetup(t)

	t.Run("canonical limitations not modified", func(t *testing.T) {
		modified, err := CheckCustomDevelopmentLimitationsModified(docHTML, expectations.Input.LimitationCodes, expectations.Input.AdditionalLimitations)
		require.NoError(t, err)
		assert.False(t, modified)
	})

	t.Run("modified catalog clause detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `Biaya akomodasi, transportasi luar kota, dan konsumsi harian jika pekerjaan on-site diminta oleh klien.`, `Biaya transportasi gratis.`, 1)
		modified, err := CheckCustomDevelopmentLimitationsModified(tampered, expectations.Input.LimitationCodes, expectations.Input.AdditionalLimitations)
		require.NoError(t, err)
		assert.True(t, modified)
	})

	t.Run("modified additional limitation detected", func(t *testing.T) {
		tampered := strings.Replace(docHTML, `Koneksi internet dan perangkat kasir disediakan oleh klien.`, `Koneksi internet disediakan oleh Venturo.`, 1)
		modified, err := CheckCustomDevelopmentLimitationsModified(tampered, expectations.Input.LimitationCodes, expectations.Input.AdditionalLimitations)
		require.NoError(t, err)
		assert.True(t, modified)
	})
}
