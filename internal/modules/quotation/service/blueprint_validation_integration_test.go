package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type blueprintPersistenceSpy struct {
	calls int
}

func (s *blueprintPersistenceSpy) Persist(_ json.RawMessage) {
	s.calls++
}

func testSkillSnapshotForIntegration() json.RawMessage {
	snap := BlueprintSkillSnapshot{
		ID:                 uuid.New(),
		Name:               "Integration Blueprint Skill",
		Command:            BlueprintSkillCommand,
		ContentInstruction: "Generate software blueprint sections strictly according to schema contracts.",
		UpdatedAt:          time.Now().UTC(),
	}
	b, _ := json.Marshal(snap)
	return json.RawMessage(b)
}

func loadIntegrationTestBundle(t *testing.T) BlueprintBundle {
	t.Helper()
	fp := filepath.Join("testdata", "blueprint", "representative-simple-10.json")
	data, err := os.ReadFile(fp)
	require.NoError(t, err)

	var bundle BlueprintBundle
	err = json.Unmarshal(data, &bundle)
	require.NoError(t, err)
	return bundle
}

func TestBlueprintValidationBoundaryAcceptsValidFakeResponse(t *testing.T) {
	builder := NewBlueprintPromptBuilder()
	secValidator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	spy := &blueprintPersistenceSpy{}

	bundle := loadIntegrationTestBundle(t)
	ctxRaw := validTestContext()

	// 1. Build real prompt for WBS stage using valid dependencies
	journeyRaw, err := json.Marshal(map[string]any{"journey": bundle.Journey})
	require.NoError(t, err)
	archRaw, err := json.Marshal(map[string]any{"architecture": bundle.Architecture})
	require.NoError(t, err)
	smRaw, err := json.Marshal(map[string]any{"state_machine": bundle.StateMachine})
	require.NoError(t, err)
	wfRaw, err := json.Marshal(map[string]any{"workflows": bundle.Workflows})
	require.NoError(t, err)

	deps := map[domain.BlueprintSectionType]json.RawMessage{
		domain.BlueprintSectionContext:      ctxRaw,
		domain.BlueprintSectionJourney:      json.RawMessage(journeyRaw),
		domain.BlueprintSectionArchitecture: json.RawMessage(archRaw),
		domain.BlueprintSectionStateMachine: json.RawMessage(smRaw),
		domain.BlueprintSectionWorkflow:     json.RawMessage(wfRaw),
	}

	prompt, err := builder.Build(BlueprintPromptInput{
		Stage:         domain.BlueprintSectionWBS,
		SkillSnapshot: testSkillSnapshotForIntegration(),
		Context:       ctxRaw,
		Dependencies:  deps,
	})
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	// 2. Deterministic fake provider JSON response matching approved WBS section
	fakeWBSResponse, err := json.Marshal(map[string]any{
		"wbs":              bundle.WBS,
		"cross_references": bundle.CrossReferences,
	})
	require.NoError(t, err)

	// 3. Invoke real section validator
	secResult := secValidator.Validate(domain.BlueprintSectionWBS, fakeWBSResponse)
	require.True(t, secResult.Valid, "Section validator should accept valid WBS fake response")
	require.Empty(t, secResult.Issues)

	// 4. Assemble representative bundle and invoke real cross-validator
	crossResult := crossValidator.Validate(bundle)
	require.True(t, crossResult.Valid, "Cross-validator should accept valid bundle")
	require.Empty(t, crossResult.SafeOutcome)
	for _, iss := range crossResult.Issues {
		require.False(t, iss.Blocking, "Expected no blocking issues in valid bundle: %v", iss)
	}

	// 5. Invoke persistence spy only when both validators pass
	if secResult.Valid && crossResult.Valid {
		spy.Persist(secResult.NormalizedContent)
	}

	// Assert spy was called exactly once
	require.Equal(t, 1, spy.calls, "Persistence spy must be called exactly once for valid output")
}

func TestBlueprintValidationBoundaryRejectsDanglingReferenceBeforePersistence(t *testing.T) {
	builder := NewBlueprintPromptBuilder()
	secValidator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	spy := &blueprintPersistenceSpy{}

	danglingMarker := "MARKER_DANGLING_NONEXISTENT_NODE_REF"

	bundle := loadIntegrationTestBundle(t)
	ctxRaw := validTestContext()

	// 1. Build real prompt for WBS stage
	journeyRaw, err := json.Marshal(map[string]any{"journey": bundle.Journey})
	require.NoError(t, err)
	archRaw, err := json.Marshal(map[string]any{"architecture": bundle.Architecture})
	require.NoError(t, err)
	smRaw, err := json.Marshal(map[string]any{"state_machine": bundle.StateMachine})
	require.NoError(t, err)
	wfRaw, err := json.Marshal(map[string]any{"workflows": bundle.Workflows})
	require.NoError(t, err)

	deps := map[domain.BlueprintSectionType]json.RawMessage{
		domain.BlueprintSectionContext:      ctxRaw,
		domain.BlueprintSectionJourney:      json.RawMessage(journeyRaw),
		domain.BlueprintSectionArchitecture: json.RawMessage(archRaw),
		domain.BlueprintSectionStateMachine: json.RawMessage(smRaw),
		domain.BlueprintSectionWorkflow:     json.RawMessage(wfRaw),
	}

	prompt, err := builder.Build(BlueprintPromptInput{
		Stage:         domain.BlueprintSectionWBS,
		SkillSnapshot: testSkillSnapshotForIntegration(),
		Context:       ctxRaw,
		Dependencies:  deps,
	})
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	// 2. Fake provider response with dangling cross reference
	badBundle := bundle
	badBundle.CrossReferences = append(badBundle.CrossReferences, BlueprintCrossReference{
		ID:           "xref-dangling-1",
		SourceRef:    "wbs:module-1",
		TargetRef:    "architecture." + strings.ToLower(danglingMarker),
		RelationType: "implemented_by",
		Description:  "References a non-existent architectural node",
	})

	fakeWBSResponse, err := json.Marshal(map[string]any{
		"wbs":              badBundle.WBS,
		"cross_references": badBundle.CrossReferences,
	})
	require.NoError(t, err)

	// 3. Validate section (structure is valid, but contains cross references)
	secResult := secValidator.Validate(domain.BlueprintSectionWBS, fakeWBSResponse)

	// 4. Validate bundle with cross-validator
	crossResult := crossValidator.Validate(badBundle)

	// Cross-validator must fail because TargetRef points to non-existent architecture node
	require.False(t, crossResult.Valid)
	require.Equal(t, BlueprintValidationCrossValidationError, crossResult.SafeOutcome)
	require.NotEmpty(t, crossResult.Issues)

	hasDanglingIssue := false
	for _, iss := range crossResult.Issues {
		if iss.Code == BlueprintIssueDanglingReference {
			hasDanglingIssue = true
		}
		// Error and message must not expose the raw fixture marker
		require.False(t, strings.Contains(iss.Message, danglingMarker))
	}
	require.True(t, hasDanglingIssue, "Should report BlueprintIssueDanglingReference")

	// 5. Persistence gate: spy must never be called when validation fails
	if secResult.Valid && crossResult.Valid {
		spy.Persist(secResult.NormalizedContent)
	}

	require.Equal(t, 0, spy.calls, "Persistence spy must NOT be called when output has dangling references")
}

func TestBlueprintValidationBoundaryRejectsCommercialContentBeforePersistence(t *testing.T) {
	builder := NewBlueprintPromptBuilder()
	secValidator := NewBlueprintSectionValidator()
	crossValidator := NewProductionBlueprintCrossValidator()
	spy := &blueprintPersistenceSpy{}

	commercialSecretMarker := "CONFIDENTIAL_BUDGET_RATE_CARD_USD_150000"

	bundle := loadIntegrationTestBundle(t)
	ctxRaw := validTestContext()

	// 1. Build real prompt for Architecture stage
	journeyRaw, err := json.Marshal(map[string]any{"journey": bundle.Journey})
	require.NoError(t, err)

	deps := map[domain.BlueprintSectionType]json.RawMessage{
		domain.BlueprintSectionContext: ctxRaw,
		domain.BlueprintSectionJourney: json.RawMessage(journeyRaw),
	}

	prompt, err := builder.Build(BlueprintPromptInput{
		Stage:         domain.BlueprintSectionArchitecture,
		SkillSnapshot: testSkillSnapshotForIntegration(),
		Context:       ctxRaw,
		Dependencies:  deps,
	})
	require.NoError(t, err)
	require.NotEmpty(t, prompt)

	// 2. Fake provider response with forbidden commercial key and malicious content
	badArch := bundle.Architecture
	// Inject forbidden key via raw JSON
	fakeRaw := map[string]any{
		"architecture": badArch,
		"budget":       5000000,
		"rate_card":    commercialSecretMarker,
	}
	fakeResponse, err := json.Marshal(fakeRaw)
	require.NoError(t, err)

	// 3. Validate section with production section validator
	secResult := secValidator.Validate(domain.BlueprintSectionArchitecture, fakeResponse)

	// Section validator must reject commercial keys as security failure
	require.False(t, secResult.Valid)
	require.Equal(t, BlueprintValidationSecurityFailure, secResult.SafeOutcome)
	require.NotEmpty(t, secResult.Issues)

	hasForbiddenIssue := false
	for _, iss := range secResult.Issues {
		if iss.Code == BlueprintIssueForbiddenContent {
			hasForbiddenIssue = true
		}
		// Error message must use fixed safe text without leaking secret marker
		require.False(t, strings.Contains(iss.Message, commercialSecretMarker))
	}
	require.True(t, hasForbiddenIssue, "Should report BlueprintIssueForbiddenContent")

	// 4. Persistence gate: spy must never be called on invalid/commercial response
	if secResult.Valid {
		crossResult := crossValidator.Validate(bundle)
		if crossResult.Valid {
			spy.Persist(secResult.NormalizedContent)
		}
	}

	require.Equal(t, 0, spy.calls, "Persistence spy must NOT be called when output contains commercial/security defects")
}
