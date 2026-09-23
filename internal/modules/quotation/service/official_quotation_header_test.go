package service

import (
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	nethtml "golang.org/x/net/html"
)

func TestRenderOfficialQuotationHeaderCanonicalTopology(t *testing.T) {
	got := renderOfficialQuotationHeader("PENAWARAN <LAYANAN>", QuotationMetadata{
		QuotationNumber:   "VEN/0826/007",
		Subject:           "Sistem Self-Ordering",
		IssueDate:         "Jumat, 7 Agustus 2026",
		ClientName:        "Muh. Khalid",
		ClientCompanyName: "PT Langgeng Sukses & Abadi",
	})

	root := parseTestFragment(t, got)
	metadata := findExactlyOneTestNode(t, root, "data-block-type", "metadata")
	assert.Equal(t, "section", metadata.Data)
	assert.Equal(t, 1, countTestNodes(metadata, "data-role", "quotation-brand"))
	assert.Equal(t, 1, countTestNodes(metadata, "data-role", "metadata-layout"))
	assert.Equal(t, 1, countTestNodes(metadata, "data-role", "metadata-facts"))
	assert.Equal(t, 1, countTestNodes(metadata, "data-role", "metadata-recipient"))

	brandNode := findExactlyOneTestNode(t, metadata, "data-role", "quotation-brand")
	imgNode := findExactlyOneElementTestNode(t, brandNode, "img")
	assert.True(t, hasAttribute(imgNode, "src", "/logo/venturo-logo.png"))
	assert.True(t, hasAttribute(imgNode, "alt", "Venturo — Expert Programmers"))

	h1Node := findExactlyOneElementTestNode(t, metadata, "h1")
	assert.Equal(t, "PENAWARAN <LAYANAN>", testNodeText(h1Node))

	recipientNode := findExactlyOneTestNode(t, metadata, "data-role", "metadata-recipient")
	recipientHeading := findExactlyOneTestNode(t, recipientNode, "data-role", "recipient-heading")
	assert.Equal(t, "Kepada", testNodeText(recipientHeading))

	for _, field := range []string{
		"quotation_number", "subject", "issue_date_header", "client_pic", "client_company",
	} {
		assert.Equal(t, 1, countTestNodes(metadata, "data-field", field), field)
	}

	assert.Contains(t, got, "PENAWARAN &lt;LAYANAN&gt;")
	assert.Contains(t, got, "PT Langgeng Sukses &amp; Abadi")
}

func TestRenderOfficialQuotationHeaderKeepsEmptyFieldNodes(t *testing.T) {
	got := renderOfficialQuotationHeader("TITLE", QuotationMetadata{})
	root := parseTestFragment(t, got)
	metadata := findExactlyOneTestNode(t, root, "data-block-type", "metadata")

	for _, field := range officialHeaderFields {
		assert.Contains(t, got, `data-field="`+field+`"`)
		node := findExactlyOneTestNode(t, metadata, "data-field", field)
		valueSpan := findExactlyOneTestNode(t, node, "data-role", "metadata-value")
		assert.Empty(t, testNodeText(valueSpan))
	}
}

func TestRenderOfficialQuotationHeaderOptionalRows(t *testing.T) {
	got := renderOfficialQuotationHeader("TITLE", QuotationMetadata{
		QuotationNumber: "Q-100",
		Subject:         "Kiosk Setup",
	}, officialHeaderRow{
		Field: "system_name",
		Label: "Sistem",
		Value: "Antrian Terpadu",
	})

	root := parseTestFragment(t, got)
	metadata := findExactlyOneTestNode(t, root, "data-block-type", "metadata")
	facts := findExactlyOneTestNode(t, metadata, "data-role", "metadata-facts")

	systemNode := findExactlyOneTestNode(t, facts, "data-field", "system_name")
	labelSpan := findExactlyOneTestNode(t, systemNode, "data-role", "metadata-label")
	assert.Equal(t, "Sistem", testNodeText(labelSpan))
	valSpan := findExactlyOneTestNode(t, systemNode, "data-role", "metadata-value")
	assert.Equal(t, "Antrian Terpadu", testNodeText(valSpan))
}

func assertCanonicalOfficialHeader(t *testing.T, document, title string) {
	t.Helper()
	root := parseTestFragment(t, document)
	metadata := findExactlyOneTestNode(t, root, "data-block-type", "metadata")
	assert.Equal(t, title, testNodeText(findExactlyOneElementTestNode(t, metadata, "h1")))
	assert.Equal(t, 1, countTestNodes(metadata, "data-role", "quotation-brand"))
	assert.Equal(t, 1, countTestNodes(metadata, "data-role", "metadata-layout"))
	for _, field := range officialHeaderFields {
		assert.Equal(t, 1, countTestNodes(metadata, "data-field", field), field)
	}
}

func assertOfficialHeader(t *testing.T, doc, title string) {
	t.Helper()
	assert.Equal(t, 1, strings.Count(doc, `data-block-type="metadata"`))
	unescapedTitle := html.UnescapeString(title)
	root := parseTestFragment(t, doc)
	metadata := findExactlyOneTestNode(t, root, "data-block-type", "metadata")
	h1 := findExactlyOneElementTestNode(t, metadata, "h1")
	assert.Equal(t, unescapedTitle, testNodeText(h1))
	for _, field := range []string{"quotation_number", "subject", "issue_date_header", "client_pic", "client_company"} {
		assert.Equal(t, 1, countTestNodes(metadata, "data-field", field), field)
	}
}

func parseTestFragment(t *testing.T, fragment string) *nethtml.Node {
	t.Helper()
	root, err := nethtml.Parse(strings.NewReader("<div>" + fragment + "</div>"))
	require.NoError(t, err)
	return root
}

func findExactlyOneTestNode(t *testing.T, root *nethtml.Node, attr, value string) *nethtml.Node {
	t.Helper()
	nodes := collectNodes(root)
	var match *nethtml.Node
	count := 0
	for _, n := range nodes {
		if hasAttribute(n, attr, value) {
			count++
			match = n
		}
	}
	require.Equal(t, 1, count, "expected exactly 1 node with %s=%q, found %d", attr, value, count)
	return match
}

func countTestNodes(root *nethtml.Node, attr, value string) int {
	nodes := collectNodes(root)
	count := 0
	for _, n := range nodes {
		if hasAttribute(n, attr, value) {
			count++
		}
	}
	return count
}

func findExactlyOneElementTestNode(t *testing.T, root *nethtml.Node, tag string) *nethtml.Node {
	t.Helper()
	nodes := collectNodes(root)
	var match *nethtml.Node
	count := 0
	for _, n := range nodes {
		if n.Type == nethtml.ElementNode && strings.EqualFold(n.Data, tag) {
			count++
			match = n
		}
	}
	require.Equal(t, 1, count, "expected exactly 1 element <%s>, found %d", tag, count)
	return match
}

func testNodeText(n *nethtml.Node) string {
	return nodeText(n)
}
