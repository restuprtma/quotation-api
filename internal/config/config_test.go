package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMeetingExtractionMode(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  MeetingExtractionMode
	}{
		{name: "legacy", value: "legacy", want: MeetingExtractionModeLegacy},
		{name: "pipeline", value: "pipeline", want: MeetingExtractionModePipeline},
		{name: "shadow", value: "shadow", want: MeetingExtractionModeShadow},
		{name: "trims surrounding whitespace", value: " pipeline ", want: MeetingExtractionModePipeline},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseMeetingExtractionMode(tt.value)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseMeetingExtractionModeRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "LEGACY", "experimental", "legacy|pipeline", "shadow-calls"} {
		t.Run(valueOrEmpty(value), func(t *testing.T) {
			got, err := ParseMeetingExtractionMode(value)
			require.Error(t, err)
			require.Empty(t, got)
			require.Contains(t, err.Error(), meetingExtractionModeEnv)
		})
	}
}

func TestLoadMeetingExtractionModeDefaultsToLegacy(t *testing.T) {
	t.Setenv(meetingExtractionModeEnv, "")

	got, err := LoadMeetingExtractionMode()
	require.NoError(t, err)
	require.Equal(t, MeetingExtractionModeLegacy, got)
}

func TestLoadMeetingExtractionModeReadsEnvironment(t *testing.T) {
	for _, mode := range []string{"legacy", "pipeline", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(meetingExtractionModeEnv, mode)

			got, err := LoadMeetingExtractionMode()
			require.NoError(t, err)
			require.Equal(t, MeetingExtractionMode(mode), got)
		})
	}
}

func TestLoadMeetingExtractionModeRejectsInvalidEnvironment(t *testing.T) {
	t.Setenv(meetingExtractionModeEnv, "pipeline-v2")

	got, err := LoadMeetingExtractionMode()
	require.Error(t, err)
	require.Empty(t, got)
	require.Contains(t, err.Error(), "pipeline-v2")
}

func TestLoadWiresMeetingExtractionModeIntoConfig(t *testing.T) {
	t.Setenv(meetingExtractionModeEnv, string(MeetingExtractionModePipeline))
	t.Setenv("AI_CREDENTIAL_ENCRYPTION_KEY", strings.Repeat("k", 32))

	cfg := Load()
	require.Equal(t, MeetingExtractionModePipeline, cfg.MeetingExtraction.Mode)
}

func TestLoadFailsFastForInvalidMeetingExtractionMode(t *testing.T) {
	t.Setenv(meetingExtractionModeEnv, "invalid")

	defer func() {
		require.NotNil(t, recover())
	}()

	Load()
}

func valueOrEmpty(value string) string {
	if value == "" {
		return "empty"
	}
	return value
}
