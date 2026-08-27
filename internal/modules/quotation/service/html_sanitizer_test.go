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
		"link":       `<a href="https://example.com/spec">link</a>`,
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
