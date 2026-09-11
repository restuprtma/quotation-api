package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudInfrastructureIntegrityCanonicalAndTampered(t *testing.T) {
	in := cloudRendererInput()
	calc, err := CalculateCloudInfrastructure(in)
	require.NoError(t, err)
	profile, err := ResolveCloudInfrastructureProfile(in)
	require.NoError(t, err)
	expected := CloudInfrastructureIntegrityExpectations{Input: in, Calculation: calc, Profile: profile, LegalHTML: `<p>Ketentuan legal</p>`}
	doc, err := NewCloudInfrastructureRenderer().Render(CloudInfrastructureDocumentInput{Metadata: QuotationMetadata{QuotationNumber: in.DocumentNumber, IssueDate: "24 Juni 2026"}, Input: in, Calculation: calc, Profile: profile, LegalHTML: expected.LegalHTML})
	require.NoError(t, err)
	sanitized, err := NewHTMLSanitizer().Sanitize(doc)
	require.NoError(t, err)
	require.NoError(t, ValidateCloudInfrastructureIntegrity(sanitized, expected))
	tampered := strings.Replace(sanitized, "Rp 1.800.000", "Rp 1.700.000", 1)
	err = ValidateCloudInfrastructureIntegrity(tampered, expected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "monthly_subtotal")
	tampered = strings.Replace(sanitized, `data-cloud-package-row="1"`, `data-cloud-package-row="9"`, 1)
	err = ValidateCloudInfrastructureIntegrity(tampered, expected)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "package row")
}
