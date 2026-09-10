package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func newSanitizerUnderTest() service.HTMLSanitizer {
	return service.NewHTMLSanitizer()
}

func TestSanitize_AdversarialPayloads(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		forbidden   []string
		mustContain []string
	}{
		{
			name:        "script tag stripped with content",
			input:       `<p>Hello</p><script>alert('xss')</script>`,
			forbidden:   []string{"<script", "alert"},
			mustContain: []string{"<p>Hello</p>"},
		},
		{
			name:        "uppercase script tag stripped",
			input:       `<SCRIPT src="http://evil.com/x.js"></SCRIPT><p>ok</p>`,
			forbidden:   []string{"<script", "evil.com"},
			mustContain: []string{"<p>ok</p>"},
		},
		{
			name:        "iframe stripped",
			input:       `<iframe src="https://evil.com/frame"></iframe><p>after</p>`,
			forbidden:   []string{"<iframe", "evil.com"},
			mustContain: []string{"<p>after</p>"},
		},
		{
			name:        "anchor with javascript url stripped",
			input:       `<a href="javascript:alert(1)">klik disini</a>`,
			forbidden:   []string{"<a ", "href", "javascript:"},
			mustContain: []string{"klik disini"},
		},
		{
			name:        "anchor mixed-case javascript url stripped",
			input:       `<a href="JaVaScRiPt:alert(1)">teks</a>`,
			forbidden:   []string{"href", "avascript:"},
			mustContain: []string{"teks"},
		},
		{
			name:        "anchor entity-encoded javascript url stripped",
			input:       `<a href="&#106;avascript:alert(1)">teks</a>`,
			forbidden:   []string{"href", "avascript:"},
			mustContain: []string{"teks"},
		},
		{
			name:        "anchor data url stripped",
			input:       `<a href="data:text/html,<script>alert(1)</script>">teks</a>`,
			forbidden:   []string{"href", "data:text/html"},
			mustContain: []string{"teks"},
		},
		{
			name:      "img onerror stripped",
			input:     `<img src="x" onerror="alert(1)">`,
			forbidden: []string{"<img", "onerror", "alert"},
		},
		{
			name:        "event handler attribute stripped",
			input:       `<div onmouseover="alert(1)">hover text</div>`,
			forbidden:   []string{"onmouseover", "alert"},
			mustContain: []string{"hover text"},
		},
		{
			name:        "form tag stripped but inner content preserved",
			input:       `<form action="https://evil.com/steal"><p>isi form</p></form>`,
			forbidden:   []string{"<form", "action", "evil.com"},
			mustContain: []string{"<p>isi form</p>"},
		},
		{
			name:        "style tag stripped with css content",
			input:       `<style>body{display:none}</style><p>tetap tampil</p>`,
			forbidden:   []string{"<style", "display:none"},
			mustContain: []string{"<p>tetap tampil</p>"},
		},
		{
			name:        "style attribute stripped",
			input:       `<span style="position:fixed;top:0">x</span>`,
			forbidden:   []string{"style="},
			mustContain: []string{">x<"},
		},
		{
			name:        "svg onload stripped",
			input:       `<svg onload="alert(1)"></svg><p>aman</p>`,
			forbidden:   []string{"<svg", "onload", "alert"},
			mustContain: []string{"<p>aman</p>"},
		},
		{
			name:      "object embed stripped",
			input:     `<object data="evil.swf"></object><embed src="evil2.swf">`,
			forbidden: []string{"<object", "<embed", "evil"},
		},
		{
			name:      "base tag stripped",
			input:     `<base href="https://evil.com/">`,
			forbidden: []string{"<base", "evil.com"},
		},
		{
			name:      "meta refresh stripped",
			input:     `<meta http-equiv="refresh" content="0;url=https://evil.com/">`,
			forbidden: []string{"<meta", "evil.com", "http-equiv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSanitizerUnderTest()

			out, err := s.Sanitize(tt.input)

			require.NoError(t, err)
			for _, f := range tt.forbidden {
				assert.NotContains(t, out, f, "output tidak boleh mengandung %q.\ninput: %s\noutput: %s", f, tt.input, out)
			}
			for _, m := range tt.mustContain {
				assert.Contains(t, out, m)
			}
		})
	}
}

func TestSanitize_PreservesNarrativeTags(t *testing.T) {
	s := newSanitizerUnderTest()

	cases := map[string]string{
		"paragraph":  `<p>Paragraf biasa</p>`,
		"bold":       `<p>ini <strong>penting</strong></p>`,
		"italic":     `<p>ini <em>penekanan</em></p>`,
		"underline":  `<p>ini <u>garis bawah</u></p>`,
		"strike":     `<p>ini <s>coret</s></p>`,
		"blockquote": `<blockquote>Kutipan dokumen</blockquote>`,
		"section":    `<section><p>Bagian satu</p></section>`,
		"link":       `<a>link</a>`,
		"unordered":  `<ul><li>satu</li><li>dua</li></ul>`,
		"ordered":    `<ol><li>pertama</li><li>kedua</li></ol>`,
		"h1":         `<h1>Judul Utama</h1>`,
		"h2":         `<h2>Bagian</h2>`,
		"h3":         `<h3>Sub-bagian</h3>`,
		"span":       `<span>catatan kecil</span>`,
		"div":        `<div>blok div</div>`,
		"table":      `<table><thead><tr><th colspan="2">Kolom</th></tr></thead><tbody><tr><td rowspan="1">data</td><td>data2</td></tr></tbody></table>`,
	}

	for name, snippet := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := s.Sanitize(snippet)
			require.NoError(t, err)
			assert.Equal(t, snippet, out, "tag naratif dasar harus lolos utuh")
		})
	}
}

func TestSanitize_DedicatedTemplateSemanticMarkersOnly(t *testing.T) {
	s := newSanitizerUnderTest()
	input := `<article class="quotation-document" style="width:210mm"><section data-block-type="legal" data-section-number="4" data-evil="x"><table class="signature-table"><tr><th data-column-width="75">Description</th><th data-column-width="25">Timeline</th></tr></table><a href="https://evil.example">external</a></section></article>`
	out, err := s.Sanitize(input)
	require.NoError(t, err)
	assert.Contains(t, out, `class="quotation-document"`)
	assert.Contains(t, out, `data-section-number="4"`)
	assert.Contains(t, out, `data-column-width="75"`)
	assert.Contains(t, out, `class="signature-table"`)
	assert.NotContains(t, out, `style=`)
	assert.NotContains(t, out, `data-evil`)
	assert.NotContains(t, out, `href=`)
}

func TestSanitize_PreservesBreakTag(t *testing.T) {
	s := newSanitizerUnderTest()

	out, err := s.Sanitize(`<p>baris satu<br>baris dua</p>`)
	require.NoError(t, err)

	normalized := strings.ReplaceAll(out, "<br/>", "<br>")
	assert.Equal(t, `<p>baris satu<br>baris dua</p>`, normalized)
}

func TestSanitize_PreservesCriticalMarkers(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<div class="qtr-block qtr-legal" data-block="legal" data-block-type="legal" data-template-code="QUOTATION_TERMS"><p>Legal</p></div>` +
		`<table><tbody><tr data-role="programmer_senior"><td data-field="total_investment">Rp 60.000.000</td></tr></tbody></table>`

	out, err := s.Sanitize(input)
	require.NoError(t, err)

	assert.Contains(t, out, `data-block="legal"`)
	assert.Contains(t, out, `data-block-type="legal"`)
	assert.Contains(t, out, `data-template-code="QUOTATION_TERMS"`)
	assert.Contains(t, out, `data-field="total_investment"`)
	assert.Contains(t, out, `data-role="programmer_senior"`)
	assert.Contains(t, out, `class="qtr-block qtr-legal"`)
}

func TestSanitize_PreservesSubscriptionSaaSMarkers(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<article class="quotation-document">` +
		`<header data-block-type="metadata"><h1>PENAWARAN LAYANAN SOFTWARE SUBSCRIPTION / SAAS</h1><p><span data-field="quotation_number">Q-01</span></p></header>` +
		`<section data-block-type="opening"><p>Melalui penawaran ini...</p></section>` +
		`<section data-block-type="commercial" data-section-number="1"><h2>1. Biaya</h2>` +
		`<table><tbody>` +
		`<tr data-role="subscription_period_0"><td>Paket 1</td><td>Rp 1.000.000</td><td>12 Bulan</td><td data-field="subscription_line_total">Rp 12.000.000</td></tr>` +
		`<tr data-role="subscription_one_time_0"><td>Setup</td><td>-</td><td>-</td><td data-field="subscription_line_total">Rp 5.000.000</td></tr>` +
		`<tr><td colspan="3">TOTAL</td><td data-field="total_contract">Rp 17.000.000</td></tr>` +
		`</tbody></table></section>` +
		`<section data-block-type="features" data-section-number="2"><h2>2. List Features</h2><h3>Group A</h3><ul><li>Fitur 1</li></ul></section>` +
		`<section data-block-type="payment" data-section-number="3"><h2>3. Pembayaran</h2>` +
		`<table><tbody><tr><td>Q1</td><td>1 Jan</td><td>15 Jan</td><td data-field="payment_nominal">Rp 17.000.000</td></tr></tbody></table>` +
		`<p data-field="auto_renewal">Perpanjangan otomatis</p>` +
		`<p data-field="tax_clause">Harga belum termasuk PPN.</p>` +
		`</section>` +
		`<section data-block-type="rights" data-section-number="4"><h2>4. Hak Para Pihak</h2>` +
		`<p data-field="rights_clause">Milik Venturo</p>` +
		`<p data-field="income_sharing">Bagi hasil 50%</p>` +
		`</section>` +
		`<section data-block-type="legal" data-section-number="5"><h2>5. Batasan</h2><ol><li>Ketentuan</li></ol></section>` +
		`<section data-block-type="signature"><table class="signature-table"><tbody><tr><td>Hormat Kami</td></tr></tbody></table></section>` +
		`</article>`

	out, err := s.Sanitize(input)
	require.NoError(t, err)

	assert.Contains(t, out, `class="quotation-document"`)
	assert.Contains(t, out, `data-block-type="metadata"`)
	assert.Contains(t, out, `data-field="quotation_number"`)
	assert.Contains(t, out, `data-block-type="opening"`)
	assert.Contains(t, out, `data-block-type="commercial"`)
	assert.Contains(t, out, `data-section-number="1"`)
	assert.Contains(t, out, `data-role="subscription_period_0"`)
	assert.Contains(t, out, `data-role="subscription_one_time_0"`)
	assert.Contains(t, out, `data-field="subscription_line_total"`)
	assert.Contains(t, out, `data-field="total_contract"`)
	assert.Contains(t, out, `data-block-type="features"`)
	assert.Contains(t, out, `data-section-number="2"`)
	assert.Contains(t, out, `data-block-type="payment"`)
	assert.Contains(t, out, `data-section-number="3"`)
	assert.Contains(t, out, `data-field="payment_nominal"`)
	assert.Contains(t, out, `data-field="auto_renewal"`)
	assert.Contains(t, out, `data-field="tax_clause"`)
	assert.Contains(t, out, `data-block-type="rights"`)
	assert.Contains(t, out, `data-section-number="4"`)
	assert.Contains(t, out, `data-field="rights_clause"`)
	assert.Contains(t, out, `data-field="income_sharing"`)
	assert.Contains(t, out, `data-block-type="legal"`)
	assert.Contains(t, out, `data-section-number="5"`)
	assert.Contains(t, out, `data-block-type="signature"`)
	assert.Contains(t, out, `class="signature-table"`)
}

func TestSanitize_StripsScriptInSubscriptionSaaSDocument(t *testing.T) {
	s := newSanitizerUnderTest()
	input := `<section data-block-type="commercial"><tr data-role="subscription_period_0"><td data-field="subscription_line_total">Rp 10.000.000<script>alert(1)</script></td></tr></section>`
	out, err := s.Sanitize(input)
	require.NoError(t, err)
	assert.NotContains(t, out, "<script")
	assert.NotContains(t, out, "alert")
	assert.Contains(t, out, `data-field="subscription_line_total"`)
}

func TestSanitize_EmptyInput(t *testing.T) {
	s := newSanitizerUnderTest()

	out, err := s.Sanitize("")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestSanitize_PlainTextUntouched(t *testing.T) {
	s := newSanitizerUnderTest()

	out, err := s.Sanitize("teks polos tanpa tag")
	require.NoError(t, err)
	assert.Equal(t, "teks polos tanpa tag", out)
}

func TestSanitize_PreservesWhiteLabelMarkers(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<article class="quotation-document">` +
		`<header data-block-type="metadata"><h1>PENAWARAN LAYANAN WHITE LABEL PRODUCT</h1><p><span data-field="quotation_number">Q-01</span></p></header>` +
		`<section data-block-type="opening"><p>Melalui penawaran ini...</p></section>` +
		`<section data-block-type="commercial" data-section-number="1"><h2>1. Biaya White Label</h2>` +
		`<table><tbody>` +
		`<tr data-role="license_0"><td>POS Mini Market</td><td>Rp 35.000.000</td><td>1</td><td data-field="license_line_total">Rp 35.000.000</td></tr>` +
		`<tr><td colspan="3"><strong>TOTAL INVESTMENT</strong></td><td data-field="total_license"><strong>Rp 35.000.000</strong></td></tr>` +
		`</tbody></table></section>` +
		`<section data-block-type="commercial-maintenance-hosting" data-section-number="2"><h2>2. Biaya Maintenance &amp; Hosting</h2>` +
		`<table><tbody>` +
		`<tr data-role="maintenance_0"><td>Senior Web Programmer</td><td>Rp 16.170.000</td><td>0.5</td><td>3 Bulan</td><td data-field="maintenance_line_total">Rp 24.255.000</td></tr>` +
		`<tr data-role="hosting_0"><td>Shared Hosting Small</td><td>Rp 350.000</td><td>-</td><td>3 Bulan</td><td data-field="hosting_line_total">Rp 1.050.000</td></tr>` +
		`<tr><td colspan="4"><strong>TOTAL</strong></td><td data-field="maintenance_hosting_total"><strong>Rp 25.305.000</strong></td></tr>` +
		`</tbody></table></section>` +
		`<section data-block-type="scope" data-section-number="3"><h2>3. Lingkup Pekerjaan &amp; Timeline</h2>` +
		`<h3>Master Data</h3><ul><li>Data Produk</li></ul>` +
		`<section data-block-type="timeline"><table><tbody><tr><td>Setup</td><td>Bulan 1</td></tr></tbody></table></section>` +
		`</section>` +
		`<section data-block-type="payment" data-section-number="4"><h2>4. Pembayaran</h2>` +
		`<table><tbody><tr><td>Pembelian</td><td>Pelunasan</td><td>Kontrak</td><td>H+14</td><td data-field="payment_nominal">Rp 35.000.000</td></tr></tbody></table>` +
		`<p data-field="tax_clause">Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.</p>` +
		`</section>` +
		`<div data-block-type="rights"><p>Hak cipta source code dan re-branding disepakati.</p></div>` +
		`<section data-block-type="legal" data-section-number="5"><h2>5. Batasan dan Pengecualian</h2><ol><li>Batasan out-of-pocket</li></ol></section>` +
		`<section data-block-type="signature"><table class="signature-table"><tbody><tr><td>Hormat Kami</td></tr></tbody></table></section>` +
		`</article>`

	out, err := s.Sanitize(input)
	require.NoError(t, err)

	assert.Contains(t, out, `class="quotation-document"`)
	assert.Contains(t, out, `data-block-type="metadata"`)
	assert.Contains(t, out, `data-field="quotation_number"`)
	assert.Contains(t, out, `data-block-type="opening"`)
	assert.Contains(t, out, `data-block-type="commercial"`)
	assert.Contains(t, out, `data-section-number="1"`)
	assert.Contains(t, out, `data-role="license_0"`)
	assert.Contains(t, out, `data-field="license_line_total"`)
	assert.Contains(t, out, `data-block-type="commercial-maintenance-hosting"`)
	assert.Contains(t, out, `data-section-number="2"`)
	assert.Contains(t, out, `data-role="maintenance_0"`)
	assert.Contains(t, out, `data-role="hosting_0"`)
	assert.Contains(t, out, `data-field="maintenance_line_total"`)
	assert.Contains(t, out, `data-field="hosting_line_total"`)
	assert.Contains(t, out, `data-block-type="scope"`)
	assert.Contains(t, out, `data-section-number="3"`)
	assert.Contains(t, out, `data-block-type="timeline"`)
	assert.Contains(t, out, `data-block-type="payment"`)
	assert.Contains(t, out, `data-section-number="4"`)
	assert.Contains(t, out, `data-field="payment_nominal"`)
	assert.Contains(t, out, `data-field="tax_clause"`)
	assert.Contains(t, out, `data-block-type="rights"`)
	assert.Contains(t, out, `data-block-type="legal"`)
	assert.Contains(t, out, `data-section-number="5"`)
	assert.Contains(t, out, `data-block-type="signature"`)
	assert.Contains(t, out, `class="signature-table"`)
}

func TestSanitize_StripsUnsafeAttributesAndTagsInWhiteLabelDocument(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<article class="quotation-document" style="display:none" data-unknown="evil">` +
		`<section data-block-type="commercial" data-section-number="1" onclick="alert('click')">` +
		`<tr data-role="license_0" data-custom="bad"><td data-field="license_line_total">Rp 35.000.000<script>alert('xss')</script></td></tr>` +
		`</section>` +
		`<div data-block-type="rights" style="color:red"><p><a href="javascript:alert(1)">klik disini</a></p></div>` +
		`<section data-block-type="legal" data-section-number="5"><a href="https://evil.com">link</a></section>` +
		`</article>`

	out, err := s.Sanitize(input)
	require.NoError(t, err)

	assert.NotContains(t, out, "<script")
	assert.NotContains(t, out, "alert")
	assert.NotContains(t, out, "onclick")
	assert.NotContains(t, out, "href")
	assert.NotContains(t, out, "style=")
	assert.NotContains(t, out, "display:none")
	assert.NotContains(t, out, "color:red")
	assert.NotContains(t, out, "data-unknown")
	assert.NotContains(t, out, "data-custom")

	// Markers remain
	assert.Contains(t, out, `data-block-type="commercial"`)
	assert.Contains(t, out, `data-section-number="1"`)
	assert.Contains(t, out, `data-role="license_0"`)
	assert.Contains(t, out, `data-field="license_line_total"`)
	assert.Contains(t, out, `data-block-type="rights"`)
	assert.Contains(t, out, `data-block-type="legal"`)
	assert.Contains(t, out, `data-section-number="5"`)
}

func TestSanitize_PreservesCustomDevelopmentMarkers(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<article class="quotation-document">` +
		`<header data-block-type="metadata"><h1>PENAWARAN LAYANAN CUSTOM SOFTWARE DEVELOPMENT</h1><p><span data-field="quotation_number">QTN/01</span></p></header>` +
		`<section data-block-type="opening"><p>Melalui penawaran ini...</p></section>` +
		`<section data-quotation-section="commercial-development" data-quotation-section-number="1"><h2>1. Biaya Development</h2>` +
		`<table><tbody>` +
		`<tr data-role="development_line_0_0"><td>Backend</td><td>Rp 15.000.000</td><td>1</td><td>1</td><td data-quotation-field="development_line_0_0">Rp 15.000.000</td></tr>` +
		`<tr><td colspan="4">Subtotal</td><td data-quotation-field="development_subtotal">Rp 15.000.000</td></tr>` +
		`<tr><td colspan="4">Discount</td><td data-quotation-field="discount_amount">Rp 1.000.000</td></tr>` +
		`<tr><td colspan="4">Total</td><td data-quotation-field="total_investment">Rp 14.000.000</td></tr>` +
		`</tbody></table></section>` +
		`<section data-quotation-section="commercial-monthly" data-quotation-section-number="2"><h2>2. Biaya Monthly</h2>` +
		`<table><tbody>` +
		`<tr data-role="monthly_0"><td>Server</td><td>Spec</td><td>Rp 500.000</td><td>1</td><td data-quotation-field="monthly_line_0">Rp 500.000</td></tr>` +
		`<tr><td colspan="4">Total</td><td data-quotation-field="monthly_total">Rp 500.000</td></tr>` +
		`</tbody></table>` +
		`<ul data-quotation-component="monthly-notes"><li>Catatan server</li></ul>` +
		`</section>` +
		`<section data-quotation-section="scope" data-quotation-section-number="3"><h2>3. Lingkup Pekerjaan &amp; Timeline</h2>` +
		`<h3>Modul 1</h3><ul><li>Fitur A</li></ul>` +
		`<section data-quotation-section="timeline"><table><thead><tr><th data-column-width="75">Description</th><th data-column-width="25">Timeline</th></tr></thead><tbody><tr><td>Fase 1</td><td>1 Bulan</td></tr></tbody></table></section>` +
		`</section>` +
		`<section data-quotation-section="payment" data-quotation-section-number="4"><h2>4. Pembayaran</h2>` +
		`<table><tbody><tr data-role="payment_0" data-quotation-component="development"><td>Termin 1</td><td>DP</td><td>Kontrak</td><td>H+7</td><td data-quotation-field="payment_nominal_0">Rp 14.000.000</td></tr></tbody></table>` +
		`<p data-quotation-field="tax_clause">Seluruh rincian biaya belum termasuk PPN sesuai regulasi perpajakan yang berlaku di Indonesia.</p>` +
		`</section>` +
		`<section data-quotation-section="rights" data-quotation-section-number="5"><h2>5. Hak Para Pihak</h2><p>Hak cipta diserahkan kepada klien.</p></section>` +
		`<section data-quotation-section="limitations" data-quotation-section-number="6"><h2>6. Batasan dan Pengecualian</h2><ol><li>Klausul 1</li></ol></section>` +
		`<section data-quotation-section="legal"><p>Ketentuan legal.</p></section>` +
		`<section data-quotation-section="signature"><table class="signature-table"><tbody><tr><td>Hormat Kami</td></tr></tbody></table></section>` +
		`</article>`

	out, err := s.Sanitize(input)
	require.NoError(t, err)

	// Assert markers preserved
	assert.Contains(t, out, `class="quotation-document"`)
	assert.Contains(t, out, `data-quotation-section="commercial-development"`)
	assert.Contains(t, out, `data-quotation-section-number="1"`)
	assert.Contains(t, out, `data-quotation-field="development_line_0_0"`)
	assert.Contains(t, out, `data-quotation-field="development_subtotal"`)
	assert.Contains(t, out, `data-quotation-field="discount_amount"`)
	assert.Contains(t, out, `data-quotation-field="total_investment"`)
	assert.Contains(t, out, `data-quotation-section="commercial-monthly"`)
	assert.Contains(t, out, `data-quotation-section-number="2"`)
	assert.Contains(t, out, `data-quotation-field="monthly_line_0"`)
	assert.Contains(t, out, `data-quotation-field="monthly_total"`)
	assert.Contains(t, out, `data-quotation-component="monthly-notes"`)
	assert.Contains(t, out, `data-quotation-section="scope"`)
	assert.Contains(t, out, `data-quotation-section-number="3"`)
	assert.Contains(t, out, `data-quotation-section="timeline"`)
	assert.Contains(t, out, `data-column-width="75"`)
	assert.Contains(t, out, `data-column-width="25"`)
	assert.Contains(t, out, `data-quotation-section="payment"`)
	assert.Contains(t, out, `data-quotation-section-number="4"`)
	assert.Contains(t, out, `data-quotation-component="development"`)
	assert.Contains(t, out, `data-quotation-field="payment_nominal_0"`)
	assert.Contains(t, out, `data-quotation-field="tax_clause"`)
	assert.Contains(t, out, `data-quotation-section="rights"`)
	assert.Contains(t, out, `data-quotation-section-number="5"`)
	assert.Contains(t, out, `data-quotation-section="limitations"`)
	assert.Contains(t, out, `data-quotation-section-number="6"`)
	assert.Contains(t, out, `data-quotation-section="legal"`)
	assert.Contains(t, out, `data-quotation-section="signature"`)
	assert.Contains(t, out, `class="signature-table"`)
}

func TestSanitize_CustomDevelopmentIdempotent(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<article class="quotation-document">` +
		`<header data-block-type="metadata"><h1>PENAWARAN LAYANAN CUSTOM SOFTWARE DEVELOPMENT</h1></header>` +
		`<section data-quotation-section="commercial-development" data-quotation-section-number="1">` +
		`<table><tbody>` +
		`<tr data-role="development_line_0_0"><td data-quotation-field="development_line_0_0">Rp 15.000.000</td></tr>` +
		`<tr><td data-quotation-field="total_investment">Rp 15.000.000</td></tr>` +
		`</tbody></table>` +
		`</section>` +
		`<section data-quotation-section="payment" data-quotation-section-number="2">` +
		`<table><tbody><tr data-quotation-component="development"><td data-quotation-field="payment_nominal_0">Rp 15.000.000</td></tr></tbody></table>` +
		`</section>` +
		`</article>`

	firstPass, err := s.Sanitize(input)
	require.NoError(t, err)

	secondPass, err := s.Sanitize(firstPass)
	require.NoError(t, err)

	assert.Equal(t, firstPass, secondPass, "Sanitize must be idempotent: Sanitize(Sanitize(html)) == Sanitize(html)")
}

func TestSanitize_StripsUnsafeInCustomDevelopmentDocument(t *testing.T) {
	s := newSanitizerUnderTest()

	input := `<article class="quotation-document" style="display:none" data-malicious="evil">` +
		`<section data-quotation-section="commercial-development" data-quotation-section-number="1" onclick="alert('click')">` +
		`<tr data-role="development_line_0_0"><td data-quotation-field="development_line_0_0">Rp 15.000.000<script>alert('xss')</script></td></tr>` +
		`</section>` +
		`<section data-quotation-section="rights" data-quotation-section-number="2" style="color:red">` +
		`<p><a href="javascript:alert(1)">klik disini</a></p>` +
		`</section>` +
		`<section data-quotation-section="legal"><a href="https://evil.com">link</a></section>` +
		`<style>body { background: black; }</style>` +
		`</article>`

	out, err := s.Sanitize(input)
	require.NoError(t, err)

	assert.NotContains(t, out, "<script")
	assert.NotContains(t, out, "alert")
	assert.NotContains(t, out, "onclick")
	assert.NotContains(t, out, "href")
	assert.NotContains(t, out, "style=")
	assert.NotContains(t, out, "<style")
	assert.NotContains(t, out, "display:none")
	assert.NotContains(t, out, "color:red")
	assert.NotContains(t, out, "data-malicious")

	// Markers remain intact
	assert.Contains(t, out, `data-quotation-section="commercial-development"`)
	assert.Contains(t, out, `data-quotation-section-number="1"`)
	assert.Contains(t, out, `data-quotation-field="development_line_0_0"`)
	assert.Contains(t, out, `data-quotation-section="rights"`)
	assert.Contains(t, out, `data-quotation-section="legal"`)
}

func TestHTMLSanitizerPreservesKioskQueueMarkers(t *testing.T) {
	raw := `<article class="quotation-document kiosk-queue-document"><section data-block-type="commercial"><table data-role="hardware_cost_table"><tr data-role="hardware_0"><td data-field="hardware_total">Rp 1</td></tr></table></section><script>alert(1)</script></article>`
	out, err := newSanitizerUnderTest().Sanitize(raw)
	require.NoError(t, err)
	assert.Contains(t, out, `class="quotation-document kiosk-queue-document"`)
	assert.Contains(t, out, `data-role="hardware_cost_table"`)
	assert.Contains(t, out, `data-field="hardware_total"`)
	assert.NotContains(t, out, `<script`)
	assert.NotContains(t, out, `alert(1)`)
}

func TestHTMLSanitizerPreservesTalentOutsourcingMarkers(t *testing.T) {
	raw := `<article class="quotation-document talent-outsourcing-document" data-service="talent_outsourcing"><section data-block-type="commercial" data-section-code="development_cost"><table><tr data-row-index="0"><td data-field="cost_groups.0.items.0.line_total" data-value="40000000">Rp 40.000.000</td></tr></table></section></article><script>alert(1)</script>`
	out, err := newSanitizerUnderTest().Sanitize(raw)
	require.NoError(t, err)
	assert.Contains(t, out, `data-service="talent_outsourcing"`)
	assert.Contains(t, out, `data-section-code="development_cost"`)
	assert.Contains(t, out, `data-field="cost_groups.0.items.0.line_total"`)
	assert.Contains(t, out, `data-value="40000000"`)
	assert.NotContains(t, out, `<script`)
}
