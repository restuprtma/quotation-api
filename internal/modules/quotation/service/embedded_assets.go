package service

import (
	"embed"
	"io/fs"
)

//go:embed prompts
var promptsFS embed.FS

// GetSafetyEnvelope reads a prompt file from the embedded assets.
func GetSafetyEnvelope(name string) string {
	b, err := fs.ReadFile(promptsFS, "prompts/"+name)
	if err != nil {
		return ""
	}
	return string(b)
}

// GetDedicatedSquadModulesPrompt returns the immutable, versioned prompt used
// exclusively for module/sub-feature extraction.
func GetDedicatedSquadModulesPrompt() string {
	return GetSafetyEnvelope("dedicated_squad_modules_v1.txt")
}

// GetMeetingExtractionPrompt returns the embedded prompt used for meeting extraction.
func GetMeetingExtractionPrompt() string { return GetSafetyEnvelope("meeting_extraction_v1.txt") }

func GetMeetingExtractionSchema() string {
	return GetSafetyEnvelope("meeting_extraction_schema_v1.json")
}
