package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nethtml "golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func assertCanonicalOfficialHeader(t *testing.T, document, title string) {
	t.Helper()
	root, err := nethtml.Parse(strings.NewReader("<div>" + document + "</div>"))
	require.NoError(t, err)
	metadata := findOneTestNode(t, root, "data-block-type", "metadata")
	h1 := findOneElementNode(t, metadata, "h1")
	assert.Equal(t, title, getNodeText(h1))
	assert.Equal(t, 1, countAttrNodes(metadata, "data-role", "quotation-brand"))
	assert.Equal(t, 1, countAttrNodes(metadata, "data-role", "metadata-layout"))
	for _, field := range []string{"quotation_number", "subject", "issue_date_header", "client_pic", "client_company"} {
		assert.Equal(t, 1, countAttrNodes(metadata, "data-field", field), field)
	}
}

func findOneTestNode(t *testing.T, root *nethtml.Node, attr, value string) *nethtml.Node {
	t.Helper()
	var match *nethtml.Node
	count := 0
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n == nil {
			return
		}
		for _, a := range n.Attr {
			if a.Key == attr && a.Val == value {
				count++
				match = n
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	require.Equal(t, 1, count, "expected exactly 1 node with %s=%q, found %d", attr, value, count)
	return match
}

func countAttrNodes(root *nethtml.Node, attr, value string) int {
	count := 0
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n == nil {
			return
		}
		for _, a := range n.Attr {
			if a.Key == attr && a.Val == value {
				count++
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return count
}

func findOneElementNode(t *testing.T, root *nethtml.Node, tag string) *nethtml.Node {
	t.Helper()
	var match *nethtml.Node
	count := 0
	var walk func(*nethtml.Node)
	walk = func(n *nethtml.Node) {
		if n == nil {
			return
		}
		if n.Type == nethtml.ElementNode && strings.EqualFold(n.Data, tag) {
			count++
			match = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	require.Equal(t, 1, count, "expected exactly 1 element <%s>, found %d", tag, count)
	return match
}

func getNodeText(n *nethtml.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(cur *nethtml.Node) {
		if cur == nil {
			return
		}
		if cur.Type == nethtml.TextNode {
			b.WriteString(cur.Data)
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

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
	assertCanonicalOfficialHeader(t, html, "PENAWARAN LAYANAN KIOSK & SISTEM ANTRIAN")
	assert.Contains(t, html, `class="quotation-document kiosk-queue-document"`)
	assert.Contains(t, html, "LANDA/MPP-Antrian/0726/003")
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
	assertCanonicalOfficialHeader(t, html, "PENAWARAN LAYANAN KIOSK & SISTEM ANTRIAN")
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
	assertCanonicalOfficialHeader(t, html, "PENAWARAN LAYANAN KIOSK & SISTEM ANTRIAN")
	assert.Contains(t, html, `data-section-number="1"><h2>1. Biaya Development`)
	assert.Contains(t, html, `data-section-number="2"><h2>2. Biaya Maintenance`)
	assert.Contains(t, html, `data-section-number="3"><h2>3. Lingkup Pekerjaan &amp; Timeline`)
	assert.Contains(t, html, `data-field="service_total">Rp 96.305.000`)
	assert.Contains(t, html, `data-field="maintenance_total">Rp 83.625.000`)
	assert.Contains(t, html, `TOTAL MAINTENANCE`)
	assert.NotContains(t, html, `TOTAL (6 Bulan)`)
	assert.NotContains(t, html, `data-role="hardware_cost_table"`)
}

func TestKioskQueueRendererPrivateWithoutMaintenance(t *testing.T) {
	html, err := service.NewKioskQueueHTMLRenderer().Render(privateWithoutMaintenanceDocumentInput())
	require.NoError(t, err)
	assertCanonicalOfficialHeader(t, html, "PENAWARAN LAYANAN KIOSK & SISTEM ANTRIAN")
	assert.Contains(t, html, `data-section-number="1"><h2>1. Biaya Development`)
	assert.Contains(t, html, `data-section-number="2"><h2>2. Lingkup Pekerjaan &amp; Timeline`)
	assert.Contains(t, html, `data-section-number="3"><h2>3. Pembayaran`)
	assert.Contains(t, html, `data-section-number="4"><h2>4. Batasan dan Pengecualian`)
	assert.NotContains(t, html, `data-section-number="5"`)
	assert.NotContains(t, html, `Biaya Maintenance`)
	assert.NotContains(t, html, `data-block-type="commercial-maintenance"`)
	assert.Contains(t, html, `data-field="service_total">Rp 96.305.000`)
}

func TestKioskQueueRendererSystemNameOptionalRow(t *testing.T) {
	t.Run("renders system_name row when SystemName differs from Subject", func(t *testing.T) {
		docIn := governmentDocumentInput()
		docIn.Metadata.Subject = "Pengembangan Sistem Antrian"
		docIn.Input.SystemName = "Antrian Terpadu MPP"

		html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
		require.NoError(t, err)
		assertCanonicalOfficialHeader(t, html, "PENAWARAN LAYANAN KIOSK & SISTEM ANTRIAN")
		assert.Contains(t, html, `data-field="system_name"`)
		assert.Contains(t, html, `<span data-role="metadata-label">Sistem</span>`)
		assert.Contains(t, html, `<span data-role="metadata-value">Antrian Terpadu MPP</span>`)
	})

	t.Run("omits system_name row when SystemName equals Subject", func(t *testing.T) {
		docIn := governmentDocumentInput()
		docIn.Metadata.Subject = "Sistem Antrian Terpadu MPP"
		docIn.Input.SystemName = "Sistem Antrian Terpadu MPP"

		html, err := service.NewKioskQueueHTMLRenderer().Render(docIn)
		require.NoError(t, err)
		assertCanonicalOfficialHeader(t, html, "PENAWARAN LAYANAN KIOSK & SISTEM ANTRIAN")
		assert.NotContains(t, html, `data-field="system_name"`)
	})
}
