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

func TestSignatureHTMLInjector_InjectClientSignature(t *testing.T) {
	inputHTML := `<td><p>Disetujui Oleh,</p><div class="client-signature-box"></div><p><strong>Bapak Reza</strong></p></td>`
	clientSig := "data:image/png;base64,client456"

	result := service.InjectClientSignature(inputHTML, clientSig)
	assert.Contains(t, result, `src="data:image/png;base64,client456"`)
}

func TestFormatIndonesianDate(t *testing.T) {
	d := time.Date(2026, 10, 13, 15, 30, 0, 0, time.UTC)
	assert.Equal(t, "13 Oktober 2026", service.FormatIndonesianDate(d))

	dJan := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, "1 Januari 2027", service.FormatIndonesianDate(dJan))
}
