package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestValidateSignaturePayload_Streamlined(t *testing.T) {
	validPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	// No longer errors even if name/email are empty, as long as consent is true and PNG is valid
	err := service.ValidateSignaturePayloadStreamlined(validPNG, true)
	assert.NoError(t, err)

	// Fails if consent is false
	err = service.ValidateSignaturePayloadStreamlined(validPNG, false)
	assert.ErrorIs(t, err, service.ErrConsentRequired)
}

func TestComputeContentHash_WithInjectedSignature(t *testing.T) {
	rawHTML := `<td><p>Disetujui Oleh,</p><div class="client-signature-box"></div><p><strong>Bpk. Client</strong></p></td>`
	validPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	injectedHTML := service.InjectClientSignature(rawHTML, validPNG)
	assert.Contains(t, injectedHTML, validPNG)

	hashBefore := service.ComputeContentHash(rawHTML)
	hashAfter := service.ComputeContentHash(injectedHTML)

	assert.NotEqual(t, hashBefore, hashAfter)
	assert.Len(t, hashAfter, 64)
}
