package service_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestValidateSignaturePayload_Streamlined(t *testing.T) {
	validPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

	t.Run("valid payload passes", func(t *testing.T) {
		err := service.ValidateSignaturePayloadStreamlined(validPNG, true)
		assert.NoError(t, err)
	})

	t.Run("consent false returns ErrConsentRequired", func(t *testing.T) {
		err := service.ValidateSignaturePayloadStreamlined(validPNG, false)
		assert.ErrorIs(t, err, service.ErrConsentRequired)
	})

	t.Run("invalid png format returns ErrInvalidSignaturePNG", func(t *testing.T) {
		// Not a data URL
		err := service.ValidateSignaturePayloadStreamlined("not-a-png", true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)

		// JPEG instead of PNG
		err = service.ValidateSignaturePayloadStreamlined("data:image/jpeg;base64,1234", true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)

		// Corrupt base64
		err = service.ValidateSignaturePayloadStreamlined("data:image/png;base64,invalid-base64===", true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)

		// Valid base64 but non-PNG header
		nonPNGData := "data:image/png;base64,dGVzdA=="
		err = service.ValidateSignaturePayloadStreamlined(nonPNGData, true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)
	})

	t.Run("oversized payload exceeds 2MB returns ErrInvalidSignaturePNG", func(t *testing.T) {
		// 2MB + 1 byte
		oversized := "data:image/png;base64," + strings.Repeat("A", 2*1024*1024+1)
		err := service.ValidateSignaturePayloadStreamlined(oversized, true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)
	})
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
