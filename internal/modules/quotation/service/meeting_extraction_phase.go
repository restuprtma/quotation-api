package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type PhaseKey string

const PhaseKeyPlanMetadata PhaseKey = "plan_metadata"

type FieldPath string

// Phase is deliberately small: orchestration decides graph order and whether a
// phase runs. A phase receives only in-memory canonical notes and safe,
// already-validated dependency results.
type Phase interface {
	Key() PhaseKey
	Owns() []FieldPath
	BuildPrompt(context.Context, PhaseInput) (ProviderRequest, error)
	Schema(PhaseInput) (json.RawMessage, error)
	Validate(context.Context, PhaseInput, json.RawMessage) (PhaseResult, error)
}

type PhaseInput struct {
	Document             *MeetingNoteDocument
	Dependencies         map[string]json.RawMessage
	PipelineVersion      string
	PromptVersion        string
	SchemaVersion        string
	CatalogSnapshotHash  string
	RegistryEntryVersion string
	ManualScheme         string
	UserHints            PlanUserHints
}

// ProviderRequest intentionally has no String method. Callers must pass it
// directly to the extraction wrapper and must never log it.
type ProviderRequest struct {
	Prompt          string
	JSONSchema      json.RawMessage
	MaxOutputTokens int
}

type PhaseResult interface {
	PhaseKey() PhaseKey
	SafeResultJSON() ([]byte, error)
	EvidenceReferenceIDs() []string
	WarningCodes() []string
}

// InputFingerprint covers every input that can affect a phase result. Values
// are length-delimited and dependencies are sorted, avoiding concatenation or
// map-order ambiguity.
func InputFingerprint(key PhaseKey, in PhaseInput) string {
	h := sha256.New()
	writeFingerprintValue(h, string(key))
	noteHash := ""
	if in.Document != nil {
		noteHash = in.Document.Hash()
	}
	writeFingerprintValue(h, noteHash)
	keys := make([]string, 0, len(in.Dependencies))
	for dependency := range in.Dependencies {
		keys = append(keys, dependency)
	}
	sort.Strings(keys)
	for _, dependency := range keys {
		writeFingerprintValue(h, dependency)
		digest := sha256.Sum256(canonicalFingerprintJSON(in.Dependencies[dependency]))
		writeFingerprintValue(h, hex.EncodeToString(digest[:]))
	}
	writeFingerprintValue(h, in.PipelineVersion)
	writeFingerprintValue(h, in.PromptVersion)
	writeFingerprintValue(h, in.SchemaVersion)
	writeFingerprintValue(h, in.CatalogSnapshotHash)
	writeFingerprintValue(h, in.RegistryEntryVersion)
	writeFingerprintValue(h, in.ManualScheme)
	// User-provided metadata wins over extracted metadata, so it is an input to
	// the result just as much as the note document is.  Hashing only the final
	// digest keeps the fingerprint fixed-width and avoids carrying the hint in
	// any persistence or diagnostic path.
	for _, hint := range []*string{in.UserHints.ClientCompany, in.UserHints.ProjectName, in.UserHints.PICName, in.UserHints.ClientName} {
		value := ""
		if hint != nil {
			value = *hint
		}
		digest := sha256.Sum256([]byte(value))
		writeFingerprintValue(h, hex.EncodeToString(digest[:]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func canonicalFingerprintJSON(raw []byte) []byte {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return raw
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return canonical
}

func writeFingerprintValue(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(string(rune(len([]rune(value))))))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

func normalizedManualScheme(value string) string { return strings.TrimSpace(value) }
