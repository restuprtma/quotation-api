package logger

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestDiscordEventForAlert_OnlyForwardsAllowlistedSafeMetadata(t *testing.T) {
	const canary = "MEETING-EXTRACTION-CANARY-MUST-NOT-LEAVE-ZAP"

	event := discordEventForAlert(zapcore.Entry{
		Level:   zapcore.ErrorLevel,
		Message: canary,
		Time:    time.Unix(1, 0),
	}, []zapcore.Field{
		zap.String("phase_key", "p3_staffing"),
		zap.Int64("duration_ms", 123),
		zap.String("meeting_notes", canary),
		zap.String("Authorization", "Bearer "+canary),
		zap.Error(errors.New(canary)),
		zap.Any("provider_response", map[string]string{"body": canary}),
		zap.String("error_code", "PROVIDER_TIMEOUT"),
	})

	require.Equal(t, "application alert", event.Message)
	require.Equal(t, map[string]any{
		"duration_ms": int64(123),
	}, event.Fields)
	require.NotContains(t, strings.Join([]string{event.Message, stringifyFields(event.Fields)}, " "), canary)
}

func stringifyFields(fields map[string]any) string {
	var values []string
	for key, value := range fields {
		values = append(values, key+"="+strings.TrimSpace(toString(value)))
	}
	return strings.Join(values, " ")
}

func toString(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return "non-string"
}
