package service_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestSignatureHTMLInjector_SyncDatesAndInjectDirector(t *testing.T) {
	inputHTML := `<section data-block-type="metadata">
<div data-role="metadata-row" data-field="issue_date_header"><span data-role="metadata-label">Tanggal</span><span data-role="metadata-separator">:</span><span data-role="metadata-value">5 Oktober 2026</span></div>
</section>
<section data-block-type="signature">
<p>Malang, <span data-field="issue_date">5 Oktober 2026</span></p>
<table class="signature-table"><tbody><tr><td><p>Hormat Kami,</p><br><br><p><strong>Yulianto Frandi</strong></p><p>Direksi Venturo Pro</p></td><td><p>Disetujui Oleh,</p><br><br><p><strong>Bapak Reza</strong></p></td></tr></tbody></table>
</section>`

	publishTime := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	dirSig := "data:image/png;base64,sig123"
	stamp := "data:image/png;base64,stamp123"

	result := service.SyncDatesAndInjectDirectorSignature(inputHTML, publishTime, "Yulianto Frandi", "Direksi Venturo Pro", dirSig, stamp)

	// Both dates must be 13 Oktober 2026
	assert.Contains(t, result, `<span data-role="metadata-value">13 Oktober 2026</span>`)
	assert.Contains(t, result, `<span data-field="issue_date">13 Oktober 2026</span>`)

	// Must contain director signature and stamp images
	assert.Contains(t, result, `src="data:image/png;base64,sig123"`)
	assert.Contains(t, result, `src="data:image/png;base64,stamp123"`)
}

func TestSignatureHTMLInjector_EmptySignatureOrStamp_OmitsImageTags(t *testing.T) {
	inputHTML := `<section data-block-type="signature">
<p>Malang, <span data-field="issue_date">5 Oktober 2026</span></p>
<table class="signature-table"><tbody><tr><td><p>Hormat Kami,</p><br><br><p><strong>Yulianto Frandi</strong></p><p>Direksi Venturo Pro</p></td></tr></tbody></table>
</section>`

	publishTime := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)

	// Case 1: Both empty
	resBothEmpty := service.SyncDatesAndInjectDirectorSignature(inputHTML, publishTime, "Yulianto Frandi", "Direksi Venturo Pro", "", "")
	assert.NotContains(t, resBothEmpty, `<img`)
	assert.NotContains(t, resBothEmpty, `src=""`)
	assert.Contains(t, resBothEmpty, `class="signature-stamp-box"`)

	// Case 2: Only signature provided, stamp empty
	resSigOnly := service.SyncDatesAndInjectDirectorSignature(inputHTML, publishTime, "Yulianto Frandi", "Direksi Venturo Pro", "data:image/png;base64,sig123", "")
	assert.Contains(t, resSigOnly, `class="director-signature-img"`)
	assert.NotContains(t, resSigOnly, `class="company-stamp-img"`)
	assert.NotContains(t, resSigOnly, `src=""`)

	// Case 3: Only stamp provided, signature empty
	resStampOnly := service.SyncDatesAndInjectDirectorSignature(inputHTML, publishTime, "Yulianto Frandi", "Direksi Venturo Pro", "", "data:image/png;base64,stamp123")
	assert.NotContains(t, resStampOnly, `class="director-signature-img"`)
	assert.Contains(t, resStampOnly, `class="company-stamp-img"`)
	assert.NotContains(t, resStampOnly, `src=""`)
}

func TestSignatureHTMLInjector_InjectClientSignature(t *testing.T) {
	inputHTML := `<td><p>Disetujui Oleh,</p><div class="client-signature-box"></div><p><strong>Bapak Reza</strong></p></td>`
	clientSig := "data:image/png;base64,client456"

	result := service.InjectClientSignature(inputHTML, clientSig)
	assert.Contains(t, result, `src="data:image/png;base64,client456"`)

	// Empty client signature should not inject img
	resEmpty := service.InjectClientSignature(inputHTML, "")
	assert.NotContains(t, resEmpty, `<img`)
}

func TestFormatIndonesianDate(t *testing.T) {
	d := time.Date(2026, 10, 13, 15, 30, 0, 0, time.UTC)
	assert.Equal(t, "13 Oktober 2026", service.FormatIndonesianDate(d))

	dJan := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "1 Januari 2027", service.FormatIndonesianDate(dJan))

	// Verify timezone near midnight UTC correctly translates to next day in WIB
	utcNearMidnight := time.Date(2026, 10, 12, 20, 0, 0, 0, time.UTC) // 20:00 UTC = 03:00 WIB next day
	wib := time.FixedZone("WIB", 7*3600)
	wibTime := utcNearMidnight.In(wib)
	assert.Equal(t, "13 Oktober 2026", service.FormatIndonesianDate(wibTime))
}
