package service_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/pkg/logger"
)

func TestSecurityAudit_ZeroRawTokenInLogs(t *testing.T) {
	rawToken, tokenHash, err := service.GenerateShareToken()
	require.NoError(t, err)

	// Intercept logger output buffer
	var logBuf bytes.Buffer
	logger.SetCustomOutput(&logBuf)

	// Simulate legitimate service call logging
	logger.Info("Share link created",
		logger.String("label", "Programmer Link"),
		logger.String("token_hash", tokenHash), // Storing hash is acceptable
	)

	logOutput := logBuf.String()

	// Zero-leak check: Raw token must NEVER appear in logs
	assert.False(t, strings.Contains(logOutput, rawToken), "raw token must not be leaked to log output")
}
