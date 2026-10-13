package service_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestSignatureSettings_Validation(t *testing.T) {
	// 1. Invalid PNG data format
	err := service.ValidatePNGDataURL("not-a-png", false)
	assert.Error(t, err)

	// 2. Valid minimal PNG data URL
	validPNG := "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	err = service.ValidatePNGDataURL(validPNG, false)
	assert.NoError(t, err)

	// 3. Optional empty allowed if allowEmpty=true
	err = service.ValidatePNGDataURL("", true)
	assert.NoError(t, err)

	// 4. Empty string disallowed if allowEmpty=false
	err = service.ValidatePNGDataURL("", false)
	assert.Error(t, err)

	// 5. Wrong MIME type
	jpegDataURL := "data:image/jpeg;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	err = service.ValidatePNGDataURL(jpegDataURL, false)
	assert.Error(t, err)

	// 6. Corrupt base64
	err = service.ValidatePNGDataURL("data:image/png;base64,invalid-base64-content", false)
	assert.Error(t, err)

	// 7. Non-PNG binary data encoded as base64
	nonPNGBase64 := "data:image/png;base64,aGVsbG8gd29ybGQ="
	err = service.ValidatePNGDataURL(nonPNGBase64, false)
	assert.Error(t, err)
}
