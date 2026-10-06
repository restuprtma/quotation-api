package service_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestValidateSignaturePayload(t *testing.T) {
	validPngBytes := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	validDataUrl := "data:image/png;base64," + base64.StdEncoding.EncodeToString(validPngBytes)

	t.Run("valid payload passes", func(t *testing.T) {
		err := service.ValidateSignaturePayload("Budi Utomo", "budi@example.com", validDataUrl, true)
		assert.NoError(t, err)
	})

	t.Run("consent false rejected", func(t *testing.T) {
		err := service.ValidateSignaturePayload("Budi Utomo", "budi@example.com", validDataUrl, false)
		assert.ErrorIs(t, err, service.ErrConsentRequired)
	})

	t.Run("empty name rejected", func(t *testing.T) {
		err := service.ValidateSignaturePayload("", "budi@example.com", validDataUrl, true)
		assert.ErrorIs(t, err, service.ErrInvalidSignerName)
	})

	t.Run("invalid email rejected", func(t *testing.T) {
		err := service.ValidateSignaturePayload("Budi", "invalid-email", validDataUrl, true)
		assert.ErrorIs(t, err, service.ErrInvalidSignerEmail)
	})

	t.Run("non png prefix rejected", func(t *testing.T) {
		err := service.ValidateSignaturePayload("Budi", "budi@example.com", "data:image/jpeg;base64,1234", true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)
	})

	t.Run("corrupt base64 rejected", func(t *testing.T) {
		err := service.ValidateSignaturePayload("Budi", "budi@example.com", "data:image/png;base64,not-valid-base64===", true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)
	})

	t.Run("empty decoded bytes rejected", func(t *testing.T) {
		err := service.ValidateSignaturePayload("Budi", "budi@example.com", "data:image/png;base64,", true)
		assert.ErrorIs(t, err, service.ErrInvalidSignaturePNG)
	})
}

func TestComputeContentHash(t *testing.T) {
	html := "<article class=\"quotation-document\"><h1>Penawaran</h1></article>"
	expectedSum := sha256.Sum256([]byte(html))
	expectedHash := hex.EncodeToString(expectedSum[:])

	actualHash := service.ComputeContentHash(html)
	assert.Equal(t, expectedHash, actualHash)
	assert.Len(t, actualHash, 64)
}
