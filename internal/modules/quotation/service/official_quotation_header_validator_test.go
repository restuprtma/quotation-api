package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func parseHTMLForTest(t *testing.T, rawHTML string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(rawHTML))
	require.NoError(t, err)
	return root
}

func TestValidateOfficialHeaderFields_ValidCanonicalHeader(t *testing.T) {
	headerHTML := service.RenderOfficialQuotationHeader("PENAWARAN LAYANAN DEDICATED PROGRAMMER SQUAD", service.QuotationMetadata{
		QuotationNumber:   "VEN/0826/007",
		Subject:           "Dedicated Squad",
		IssueDate:         "23 September 2026",
		ClientName:        "John Doe",
		ClientCompanyName: "Acme Corp",
	})
	docHTML := `<article class="quotation-document">` + headerHTML + `</article>`
	root := parseHTMLForTest(t, docHTML)

	err := service.ValidateOfficialHeaderFields(root)
	require.NoError(t, err)
}

func TestValidateOfficialHeaderFields_NilRoot(t *testing.T) {
	err := service.ValidateOfficialHeaderFields(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
}

func TestValidateOfficialHeaderFields_MissingMetadataBlock(t *testing.T) {
	docHTML := `<article class="quotation-document"><section data-block-type="opening"><p>Opening</p></section></article>`
	root := parseHTMLForTest(t, docHTML)

	err := service.ValidateOfficialHeaderFields(root)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	assert.Contains(t, err.Error(), "metadata block is missing")
}

func TestValidateOfficialHeaderFields_DuplicateMetadataBlock(t *testing.T) {
	headerHTML := service.RenderOfficialQuotationHeader("TITLE", service.QuotationMetadata{
		QuotationNumber:   "Q-01",
		Subject:           "Sub",
		IssueDate:         "2026-01-01",
		ClientName:        "Client",
		ClientCompanyName: "Company",
	})
	docHTML := `<article class="quotation-document">` + headerHTML + headerHTML + `</article>`
	root := parseHTMLForTest(t, docHTML)

	err := service.ValidateOfficialHeaderFields(root)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrUnsafeHTML)
	assert.Contains(t, err.Error(), "duplicate marker")
}

func TestValidateOfficialHeaderFields_MissingField(t *testing.T) {
	tests := []struct {
		name         string
		missingField string
	}{
		{name: "missing quotation_number", missingField: "quotation_number"},
		{name: "missing subject", missingField: "subject"},
		{name: "missing issue_date_header", missingField: "issue_date_header"},
		{name: "missing client_pic", missingField: "client_pic"},
		{name: "missing client_company", missingField: "client_company"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headerHTML := service.RenderOfficialQuotationHeader("TITLE", service.QuotationMetadata{
				QuotationNumber:   "Q-01",
				Subject:           "Sub",
				IssueDate:         "2026-01-01",
				ClientName:        "Client",
				ClientCompanyName: "Company",
			})
			// Remove target field
			tampered := strings.Replace(headerHTML, `data-field="`+tt.missingField+`"`, `data-field="removed"`, 1)
			docHTML := `<article class="quotation-document">` + tampered + `</article>`
			root := parseHTMLForTest(t, docHTML)

			err := service.ValidateOfficialHeaderFields(root)
			require.Error(t, err)
			assert.ErrorIs(t, err, service.ErrUnsafeHTML)
			assert.Contains(t, err.Error(), tt.missingField)
		})
	}
}

func TestValidateOfficialHeaderFields_DuplicateField(t *testing.T) {
	tests := []struct {
		name           string
		duplicateField string
	}{
		{name: "duplicate quotation_number", duplicateField: "quotation_number"},
		{name: "duplicate subject", duplicateField: "subject"},
		{name: "duplicate issue_date_header", duplicateField: "issue_date_header"},
		{name: "duplicate client_pic", duplicateField: "client_pic"},
		{name: "duplicate client_company", duplicateField: "client_company"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headerHTML := service.RenderOfficialQuotationHeader("TITLE", service.QuotationMetadata{
				QuotationNumber:   "Q-01",
				Subject:           "Sub",
				IssueDate:         "2026-01-01",
				ClientName:        "Client",
				ClientCompanyName: "Company",
			})
			// Inject duplicate field inside metadata block
			dupNode := `<div data-role="metadata-row" data-field="` + tt.duplicateField + `"><span>dup</span></div>`
			tampered := strings.Replace(headerHTML, `</section>`, dupNode+`</section>`, 1)
			docHTML := `<article class="quotation-document">` + tampered + `</article>`
			root := parseHTMLForTest(t, docHTML)

			err := service.ValidateOfficialHeaderFields(root)
			require.Error(t, err)
			assert.ErrorIs(t, err, service.ErrUnsafeHTML)
			assert.Contains(t, err.Error(), "duplicate marker")
		})
	}
}
