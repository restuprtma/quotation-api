package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func governmentDocumentInput() service.KioskQueueDocumentInput {
	in := governmentHardwareFixtureInput()
	in.DocumentNumber = "LANDA/MPP-Antrian/0726/003"
	calc, err := service.CalculateKioskQueueSystem(in)
	if err != nil {
		panic(err)
	}
	profile, err := service.ResolveKioskQueueProfile(in)
	if err != nil {
		panic(err)
	}
	return service.KioskQueueDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber:   in.DocumentNumber,
			Subject:           "Penawaran Sistem Antrian Terpadu MPP",
			ClientName:        "Bambang Sutrisno",
			ClientCompanyName: "Dinas Penanaman Modal MPP",
			IssueDate:         "30 Juli 2026",
		},
		Input:       in,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>",
	}
}

func governmentServiceOnlyDocumentInput() service.KioskQueueDocumentInput {
	in := governmentServiceOnlyFixtureInput()
	calc, err := service.CalculateKioskQueueSystem(in)
	if err != nil {
		panic(err)
	}
	profile, err := service.ResolveKioskQueueProfile(in)
	if err != nil {
		panic(err)
	}
	return service.KioskQueueDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber:   in.DocumentNumber,
			Subject:           "Penawaran Sistem Antrian Terpadu MPP (Jasa Saja)",
			ClientName:        "Bambang Sutrisno",
			ClientCompanyName: "Dinas Penanaman Modal MPP",
			IssueDate:         "30 Juli 2026",
		},
		Input:       in,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>",
	}
}

func privateDocumentInput() service.KioskQueueDocumentInput {
	in := privateFoodCourtFixtureInput()
	calc, err := service.CalculateKioskQueueSystem(in)
	if err != nil {
		panic(err)
	}
	profile, err := service.ResolveKioskQueueProfile(in)
	if err != nil {
		panic(err)
	}
	return service.KioskQueueDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber:   in.DocumentNumber,
			Subject:           "Penawaran Self-Ordering Kiosk Food Court",
			ClientName:        "Hendrawan",
			ClientCompanyName: "PT Rasa Kuliner Nusantara",
			IssueDate:         "1 Agustus 2026",
		},
		Input:       in,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>",
	}
}

func privateWithoutMaintenanceDocumentInput() service.KioskQueueDocumentInput {
	in := privateWithoutMaintenanceInput()
	calc, err := service.CalculateKioskQueueSystem(in)
	if err != nil {
		panic(err)
	}
	profile, err := service.ResolveKioskQueueProfile(in)
	if err != nil {
		panic(err)
	}
	return service.KioskQueueDocumentInput{
		Metadata: service.QuotationMetadata{
			QuotationNumber:   in.DocumentNumber,
			Subject:           "Penawaran Self-Ordering Kiosk Food Court",
			ClientName:        "Hendrawan",
			ClientCompanyName: "PT Rasa Kuliner Nusantara",
			IssueDate:         "1 Agustus 2026",
		},
		Input:       in,
		Calculation: calc,
		Profile:     profile,
		LegalHTML:   "<p>Syarat dan ketentuan penawaran resmi berlaku 30 hari.</p>",
	}
}

func TestKioskQueueRendererGovernmentWithHardware(t *testing.T) {
	html, err := service.NewKioskQueueHTMLRenderer().Render(governmentDocumentInput())
	require.NoError(t, err)
	assert.Contains(t, html, `class="quotation-document kiosk-queue-document"`)
	assert.Contains(t, html, `data-field="quotation_number">LANDA/MPP-Antrian/0726/003`)
	assert.Contains(t, html, `data-block-type="scope"`)
	assert.Contains(t, html, `data-block-type="experts"`)
	assert.Contains(t, html, `data-block-type="commercial"`)
	assert.Contains(t, html, `data-role="service_cost_table"`)
	assert.Contains(t, html, `data-role="hardware_cost_table"`)
	assert.Contains(t, html, `data-field="service_total">Rp 48.385.000`)
	assert.Contains(t, html, `data-field="hardware_total">Rp 51.615.000`)
	assert.Contains(t, html, `data-field="payment_total">Rp 100.000.000`)
}

func TestKioskQueueRendererGovernmentServiceOnly(t *testing.T) {
	html, err := service.NewKioskQueueHTMLRenderer().Render(governmentServiceOnlyDocumentInput())
	require.NoError(t, err)
	assert.Contains(t, html, `class="quotation-document kiosk-queue-document"`)
	assert.Contains(t, html, `data-section-number="3"><h2>3. Rincian Biaya`)
	assert.NotContains(t, html, `data-role="hardware_cost_table"`)
	assert.Contains(t, html, `data-field="service_total">Rp 62.000.000`)
	assert.Contains(t, html, `data-field="payment_total">Rp 62.000.000`)
	assert.Contains(t, html, "H+14")
}

func TestKioskQueueRendererPrivateFoodCourt(t *testing.T) {
	html, err := service.NewKioskQueueHTMLRenderer().Render(privateDocumentInput())
	require.NoError(t, err)
	assert.Contains(t, html, `data-section-number="1"><h2>1. Biaya Development`)
	assert.Contains(t, html, `data-section-number="2"><h2>2. Biaya Maintenance`)
	assert.Contains(t, html, `data-section-number="3"><h2>3. Lingkup Pekerjaan &amp; Timeline`)
	assert.Contains(t, html, `data-field="service_total">Rp 96.305.000`)
	assert.Contains(t, html, `data-field="maintenance_total">Rp 83.625.000`)
	assert.NotContains(t, html, `TOTAL (6 Bulan)`)
	assert.NotContains(t, html, `data-role="hardware_cost_table"`)
}

func TestKioskQueueRendererPrivateWithoutMaintenance(t *testing.T) {
	html, err := service.NewKioskQueueHTMLRenderer().Render(privateWithoutMaintenanceDocumentInput())
	require.NoError(t, err)
	assert.Contains(t, html, `data-section-number="1"><h2>1. Biaya Development`)
	assert.Contains(t, html, `data-section-number="2"><h2>2. Lingkup Pekerjaan &amp; Timeline`)
	assert.Contains(t, html, `data-section-number="3"><h2>3. Pembayaran`)
	assert.Contains(t, html, `data-section-number="4"><h2>4. Batasan dan Pengecualian`)
	assert.NotContains(t, html, `data-section-number="5"`)
	assert.NotContains(t, html, `Biaya Maintenance`)
	assert.NotContains(t, html, `data-block-type="commercial-maintenance"`)
	assert.Contains(t, html, `data-field="service_total">Rp 96.305.000`)
}
