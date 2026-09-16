package service

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakeBlueprintSourceLoader struct {
	record      repository.BlueprintSourceRecord
	err         error
	callCount   int
	companyID   uuid.UUID
	quotationID uuid.UUID
	revisionID  uuid.UUID
	onCall      func(name string)
}

func (f *fakeBlueprintSourceLoader) LoadEligibleSource(_ context.Context, companyID, quotationID, revisionID uuid.UUID) (repository.BlueprintSourceRecord, error) {
	f.callCount++
	f.companyID = companyID
	f.quotationID = quotationID
	f.revisionID = revisionID
	if f.onCall != nil {
		f.onCall("sourceLoader")
	}
	return f.record, f.err
}

type fakeBlueprintSkillResolver struct {
	skill     *domain.Skill
	err       error
	callCount int
	companyID string
	command   string
	onCall    func(name string)
}

func (f *fakeBlueprintSkillResolver) GetActiveSkillByCommand(_ context.Context, companyID, command string) (*domain.Skill, error) {
	f.callCount++
	f.companyID = companyID
	f.command = command
	if f.onCall != nil {
		f.onCall("skillResolver")
	}
	return f.skill, f.err
}

type fakeBlueprintContextBuilder struct {
	result      NormalizedBlueprintContext
	err         error
	buildCalls  int
	receivedSrc BlueprintSourceProjection
	receivedReq dto.CreateBlueprintGenerationRequest
	onCall      func(name string)
}

func (f *fakeBlueprintContextBuilder) Build(source BlueprintSourceProjection, request dto.CreateBlueprintGenerationRequest) (NormalizedBlueprintContext, error) {
	f.buildCalls++
	f.receivedSrc = source
	f.receivedReq = request
	if f.onCall != nil {
		f.onCall("builder")
	}
	return f.result, f.err
}

type fakeBlueprintGenerationCreator struct {
	result      *repository.CreateBlueprintGenerationResult
	err         error
	createCalls int
	received    repository.CreateBlueprintGenerationParams
	onCall      func(name string)
}

func (f *fakeBlueprintGenerationCreator) CreateBlueprintGeneration(_ context.Context, params repository.CreateBlueprintGenerationParams) (*repository.CreateBlueprintGenerationResult, error) {
	f.createCalls++
	f.received = params
	if f.onCall != nil {
		f.onCall("creator")
	}
	return f.result, f.err
}

func validBlueprintGenerationServiceInput() (
	uuid.UUID,
	uuid.UUID,
	repository.BlueprintSourceRecord,
	*domain.Skill,
	dto.CreateBlueprintGenerationRequest,
) {
	companyID := uuid.New()
	createdBy := uuid.New()
	quotationID := uuid.New()
	revisionID := uuid.New()

	sourceRecord := repository.BlueprintSourceRecord{
		CompanyID:    companyID,
		QuotationID:  quotationID,
		RevisionID:   revisionID,
		Subject:      "Customer portal",
		SolutionType: "web",
		ContentHTML:  "<p>safe source content</p>",
	}

	skill := &domain.Skill{
		ID:                 uuid.New(),
		CompanyID:          companyID,
		Name:               "Blueprint Architect",
		Command:            BlueprintSkillCommand,
		ContentInstruction: "Draft comprehensive software blueprint",
		IsActive:           true,
		UpdatedAt:          time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC),
	}

	request := dto.CreateBlueprintGenerationRequest{
		IdempotencyKey:            uuid.New(),
		QuotationID:               quotationID,
		SourceQuotationRevisionID: revisionID,
		Title:                     "Customer portal blueprint",
		BusinessGoals:             []string{"Launch MVP"},
		Actors:                    []string{"Customer"},
		Channels:                  []string{"Web"},
		BusinessPhases:            []string{"Phase 1"},
	}

	return companyID, createdBy, sourceRecord, skill, request
}

func TestBlueprintGenerationServiceCreateLoadsSourceAndActiveBlueprintSkill(t *testing.T) {
	companyID, createdBy, sourceRecord, skill, request := validBlueprintGenerationServiceInput()
	normalized := NormalizedBlueprintContext{
		SourceContextSnapshot:  []byte(`{"subject":"Customer portal"}`),
		ContextContent:         []byte(`{"title":"Customer portal blueprint"}`),
		IdempotencyPayloadHash: strings.Repeat("a", 64),
	}

	var callOrder []string
	recordCall := func(name string) {
		callOrder = append(callOrder, name)
	}

	loader := &fakeBlueprintSourceLoader{record: sourceRecord, onCall: recordCall}
	resolver := &fakeBlueprintSkillResolver{skill: skill, onCall: recordCall}
	builder := &fakeBlueprintContextBuilder{result: normalized, onCall: recordCall}
	creator := &fakeBlueprintGenerationCreator{
		result: &repository.CreateBlueprintGenerationResult{
			Blueprint: domain.Blueprint{ID: uuid.New(), Title: request.Title},
			Revision:  domain.BlueprintRevision{ID: uuid.New()},
		},
		onCall: recordCall,
	}

	svc := NewBlueprintGenerationService(loader, resolver, builder, creator)

	result, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, creator.result, result)

	// Call order: sourceLoader -> skillResolver -> builder -> creator, each exactly once
	expectedOrder := []string{"sourceLoader", "skillResolver", "builder", "creator"}
	assert.Equal(t, expectedOrder, callOrder)
	assert.Equal(t, 1, loader.callCount)
	assert.Equal(t, 1, resolver.callCount)
	assert.Equal(t, 1, builder.buildCalls)
	assert.Equal(t, 1, creator.createCalls)

	// Verify loader arguments
	assert.Equal(t, companyID, loader.companyID)
	assert.Equal(t, request.QuotationID, loader.quotationID)
	assert.Equal(t, request.SourceQuotationRevisionID, loader.revisionID)

	// Verify resolver arguments
	assert.Equal(t, companyID.String(), resolver.companyID)
	assert.Equal(t, BlueprintSkillCommand, resolver.command)

	// Verify builder received mapped projection from loader
	expectedProj := BlueprintSourceProjection{
		CompanyID:    sourceRecord.CompanyID,
		QuotationID:  sourceRecord.QuotationID,
		RevisionID:   sourceRecord.RevisionID,
		Subject:      sourceRecord.Subject,
		SolutionType: sourceRecord.SolutionType,
		ContentHTML:  sourceRecord.ContentHTML,
	}
	assert.Equal(t, expectedProj, builder.receivedSrc)
	assert.True(t, reflect.DeepEqual(request, builder.receivedReq))

	// Verify creator arguments
	assert.Equal(t, companyID, creator.received.CompanyID)
	assert.Equal(t, request.QuotationID, creator.received.QuotationID)
	assert.Equal(t, request.SourceQuotationRevisionID, creator.received.SourceRevisionID)
	assert.Equal(t, request.Title, creator.received.Title)
	assert.Equal(t, request.IdempotencyKey, creator.received.IdempotencyKey)
	assert.Equal(t, normalized.IdempotencyPayloadHash, creator.received.IdempotencyPayloadHash)
	assert.Equal(t, string(normalized.SourceContextSnapshot), string(creator.received.SourceContextSnapshot))
	assert.Equal(t, string(normalized.ContextContent), string(creator.received.ContextContent))
	assert.NotEqual(t, "{}", string(creator.received.SkillSnapshot))
	assert.True(t, json.Valid(creator.received.SkillSnapshot))
	require.NotNil(t, creator.received.CreatedBy)
	assert.Equal(t, createdBy, *creator.received.CreatedBy)
}

func TestBlueprintGenerationServiceCreateRejectsMissingOrForeignSource(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{
			name: "missing source",
			err:  repository.ErrBlueprintSourceNotFound,
		},
		{
			name: "foreign source returns not found",
			err:  repository.ErrBlueprintSourceNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			companyID, createdBy, _, skill, request := validBlueprintGenerationServiceInput()
			loader := &fakeBlueprintSourceLoader{err: tc.err}
			resolver := &fakeBlueprintSkillResolver{skill: skill}
			builder := &fakeBlueprintContextBuilder{}
			creator := &fakeBlueprintGenerationCreator{}

			svc := NewBlueprintGenerationService(loader, resolver, builder, creator)

			_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
			require.Error(t, err)
			assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))

			assert.Equal(t, 1, loader.callCount)
			assert.Equal(t, 0, resolver.callCount, "resolver must not be called when source load fails")
			assert.Equal(t, 0, builder.buildCalls, "builder must not be called when source load fails")
			assert.Equal(t, 0, creator.createCalls, "creator must not be called when source load fails")
		})
	}
}

func TestBlueprintGenerationServiceCreateRejectsIneligibleSource(t *testing.T) {
	companyID, createdBy, _, skill, request := validBlueprintGenerationServiceInput()
	loader := &fakeBlueprintSourceLoader{err: repository.ErrBlueprintSourceIneligible}
	resolver := &fakeBlueprintSkillResolver{skill: skill}
	builder := &fakeBlueprintContextBuilder{}
	creator := &fakeBlueprintGenerationCreator{}

	svc := NewBlueprintGenerationService(loader, resolver, builder, creator)

	_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintSourceIneligible))

	assert.Equal(t, 1, loader.callCount)
	assert.Equal(t, 0, resolver.callCount, "resolver must not be called when source is ineligible")
	assert.Equal(t, 0, builder.buildCalls, "builder must not be called when source is ineligible")
	assert.Equal(t, 0, creator.createCalls, "creator must not be called when source is ineligible")
}

func TestBlueprintGenerationServiceCreateRejectsMissingOrInactiveSkill(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name     string
		resolver *fakeBlueprintSkillResolver
	}{
		{
			name: "skill not found",
			resolver: &fakeBlueprintSkillResolver{
				err: repository.ErrSkillNotFound,
			},
		},
		{
			name: "skill is inactive",
			resolver: &fakeBlueprintSkillResolver{
				skill: &domain.Skill{
					ID:                 uuid.New(),
					Name:               "Inactive Skill",
					Command:            BlueprintSkillCommand,
					ContentInstruction: "Instruction",
					IsActive:           false,
					UpdatedAt:          now,
				},
			},
		},
		{
			name: "skill is soft-deleted",
			resolver: &fakeBlueprintSkillResolver{
				skill: &domain.Skill{
					ID:                 uuid.New(),
					Name:               "Deleted Skill",
					Command:            BlueprintSkillCommand,
					ContentInstruction: "Instruction",
					IsActive:           true,
					DeletedAt:          &now,
					UpdatedAt:          now,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			companyID, createdBy, sourceRecord, _, request := validBlueprintGenerationServiceInput()
			loader := &fakeBlueprintSourceLoader{record: sourceRecord}
			builder := &fakeBlueprintContextBuilder{}
			creator := &fakeBlueprintGenerationCreator{}

			svc := NewBlueprintGenerationService(loader, tc.resolver, builder, creator)

			_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
			require.Error(t, err)
			assert.True(t, errors.Is(err, repository.ErrSkillNotFound))

			assert.Equal(t, 1, loader.callCount)
			assert.Equal(t, 1, tc.resolver.callCount)
			assert.Equal(t, 0, builder.buildCalls, "builder must not be called when skill resolution fails")
			assert.Equal(t, 0, creator.createCalls, "creator must not be called when skill resolution fails")
		})
	}
}

func TestBlueprintGenerationServiceCreatePersistsAllowlistedSkillSnapshot(t *testing.T) {
	companyID, createdBy, sourceRecord, skill, request := validBlueprintGenerationServiceInput()

	// Add extraneous fields on skill that must NOT leak into snapshot
	createdByID := uuid.New()
	skill.CreatedBy = &createdByID
	skill.CreatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	loader := &fakeBlueprintSourceLoader{record: sourceRecord}
	resolver := &fakeBlueprintSkillResolver{skill: skill}
	builder := &fakeBlueprintContextBuilder{
		result: NormalizedBlueprintContext{
			SourceContextSnapshot:  []byte(`{}`),
			ContextContent:         []byte(`{}`),
			IdempotencyPayloadHash: strings.Repeat("b", 64),
		},
	}
	creator := &fakeBlueprintGenerationCreator{
		result: &repository.CreateBlueprintGenerationResult{},
	}

	svc := NewBlueprintGenerationService(loader, resolver, builder, creator)

	_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
	require.NoError(t, err)

	snapshotRaw := creator.received.SkillSnapshot
	require.True(t, json.Valid(snapshotRaw), "SkillSnapshot must be valid JSON")

	var parsed map[string]any
	err = json.Unmarshal(snapshotRaw, &parsed)
	require.NoError(t, err)

	// Must contain exactly 5 allowlisted keys
	assert.Len(t, parsed, 5)
	expectedKeys := []string{"id", "name", "command", "content_instruction", "updated_at"}
	for _, key := range expectedKeys {
		_, ok := parsed[key]
		assert.Truef(t, ok, "key %q must be in SkillSnapshot", key)
	}

	assert.Equal(t, skill.ID.String(), parsed["id"])
	assert.Equal(t, skill.Name, parsed["name"])
	assert.Equal(t, BlueprintSkillCommand, parsed["command"])
	assert.Equal(t, skill.ContentInstruction, parsed["content_instruction"])
	assert.Equal(t, skill.UpdatedAt.UTC().Format(time.RFC3339Nano), parsed["updated_at"])

	// Check forbidden fields
	snapshotStr := strings.ToLower(string(snapshotRaw))
	forbiddenKeys := []string{
		"company_id", "created_by", "created_at", "deleted_at", "is_active",
		"api_key", "credential", "token", "provider", "model",
	}
	for _, forbidden := range forbiddenKeys {
		assert.Falsef(t, strings.Contains(snapshotStr, forbidden), "forbidden key %q leaked into snapshot: %s", forbidden, snapshotStr)
	}
}

func TestBlueprintGenerationServiceCreatePropagatesPayloadMismatch(t *testing.T) {
	companyID, createdBy, sourceRecord, skill, request := validBlueprintGenerationServiceInput()
	loader := &fakeBlueprintSourceLoader{record: sourceRecord}
	resolver := &fakeBlueprintSkillResolver{skill: skill}
	builder := &fakeBlueprintContextBuilder{
		result: NormalizedBlueprintContext{
			SourceContextSnapshot:  []byte(`{}`),
			ContextContent:         []byte(`{}`),
			IdempotencyPayloadHash: strings.Repeat("c", 64),
		},
	}
	creator := &fakeBlueprintGenerationCreator{err: repository.ErrBlueprintIdempotencyPayloadMismatch}

	svc := NewBlueprintGenerationService(loader, resolver, builder, creator)

	_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintIdempotencyPayloadMismatch))

	assert.Equal(t, 1, loader.callCount)
	assert.Equal(t, 1, resolver.callCount)
	assert.Equal(t, 1, builder.buildCalls)
	assert.Equal(t, 1, creator.createCalls)
}

func TestBlueprintGenerationServiceCreateDoesNotLeakSourceOrSkillFields(t *testing.T) {
	const secretHTML = "<p>ultra-secret-quotation-source-html-content</p>"
	const secretInstruction = "confidential-blueprint-skill-system-instruction"

	t.Run("source loader error does not leak source content", func(t *testing.T) {
		companyID, createdBy, _, skill, request := validBlueprintGenerationServiceInput()
		loader := &fakeBlueprintSourceLoader{err: errors.New("db failure with " + secretHTML)}
		resolver := &fakeBlueprintSkillResolver{skill: skill}
		builder := &fakeBlueprintContextBuilder{}
		creator := &fakeBlueprintGenerationCreator{}

		svc := NewBlueprintGenerationService(loader, resolver, builder, creator)
		_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
		require.Error(t, err)
		assert.False(t, strings.Contains(err.Error(), secretHTML), "error must not leak secret HTML")
	})

	t.Run("skill resolver error does not leak skill instruction", func(t *testing.T) {
		companyID, createdBy, sourceRecord, _, request := validBlueprintGenerationServiceInput()
		loader := &fakeBlueprintSourceLoader{record: sourceRecord}
		resolver := &fakeBlueprintSkillResolver{err: errors.New("resolver error with " + secretInstruction)}
		builder := &fakeBlueprintContextBuilder{}
		creator := &fakeBlueprintGenerationCreator{}

		svc := NewBlueprintGenerationService(loader, resolver, builder, creator)
		_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
		require.Error(t, err)
		assert.False(t, strings.Contains(err.Error(), secretInstruction), "error must not leak secret instruction")
	})

	t.Run("builder error does not leak source content", func(t *testing.T) {
		companyID, createdBy, sourceRecord, skill, request := validBlueprintGenerationServiceInput()
		loader := &fakeBlueprintSourceLoader{record: sourceRecord}
		resolver := &fakeBlueprintSkillResolver{skill: skill}
		builder := &fakeBlueprintContextBuilder{err: errors.New("normalization failed on " + secretHTML)}
		creator := &fakeBlueprintGenerationCreator{}

		svc := NewBlueprintGenerationService(loader, resolver, builder, creator)
		_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
		require.Error(t, err)
		assert.False(t, strings.Contains(err.Error(), secretHTML), "error must not leak secret HTML")
	})

	t.Run("creator error does not leak skill instruction", func(t *testing.T) {
		companyID, createdBy, sourceRecord, skill, request := validBlueprintGenerationServiceInput()
		loader := &fakeBlueprintSourceLoader{record: sourceRecord}
		resolver := &fakeBlueprintSkillResolver{skill: skill}
		builder := &fakeBlueprintContextBuilder{
			result: NormalizedBlueprintContext{
				SourceContextSnapshot:  []byte(`{}`),
				ContextContent:         []byte(`{}`),
				IdempotencyPayloadHash: strings.Repeat("d", 64),
			},
		}
		creator := &fakeBlueprintGenerationCreator{err: errors.New("creator insert error with " + secretInstruction)}

		svc := NewBlueprintGenerationService(loader, resolver, builder, creator)
		_, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
		require.Error(t, err)
		assert.False(t, strings.Contains(err.Error(), secretInstruction), "error must not leak secret instruction")
	})

	t.Run("public BlueprintStatusResponse JSON contains no sensitive fields", func(t *testing.T) {
		companyID, createdBy, sourceRecord, skill, request := validBlueprintGenerationServiceInput()
		loader := &fakeBlueprintSourceLoader{record: sourceRecord}
		resolver := &fakeBlueprintSkillResolver{skill: skill}
		builder := &fakeBlueprintContextBuilder{
			result: NormalizedBlueprintContext{
				SourceContextSnapshot:  []byte(`{"subject":"Customer portal"}`),
				ContextContent:         []byte(`{"title":"Customer portal blueprint"}`),
				IdempotencyPayloadHash: strings.Repeat("e", 64),
			},
		}

		now := time.Now().UTC()
		bpID := uuid.New()
		revID := uuid.New()
		creator := &fakeBlueprintGenerationCreator{
			result: &repository.CreateBlueprintGenerationResult{
				Blueprint: domain.Blueprint{
					ID:              bpID,
					CompanyID:       companyID,
					QuotationID:     request.QuotationID,
					Title:           request.Title,
					Status:          "draft",
					CurrentRevision: 1,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
				Revision: domain.BlueprintRevision{
					ID:                 revID,
					CompanyID:          companyID,
					BlueprintID:        bpID,
					RevisionNumber:     1,
					PublicationStatus:  "draft",
					CompletenessStatus: "partial",
					GenerationStatus:   "pending",
					ContentVersion:     1,
					CreatedAt:          now,
					UpdatedAt:          now,
				},
				SectionRuns: []domain.BlueprintSectionRun{
					{
						ID:                  uuid.New(),
						CompanyID:           companyID,
						BlueprintRevisionID: revID,
						SectionType:         "context",
						Status:              "validated",
						UpdatedAt:           now,
					},
				},
			},
		}

		svc := NewBlueprintGenerationService(loader, resolver, builder, creator)
		result, err := svc.CreateBlueprint(context.Background(), companyID, createdBy, request)
		require.NoError(t, err)

		// Derive public status response
		statusResp := dto.BlueprintStatusResponse{
			BlueprintID:        result.Blueprint.ID,
			RevisionID:         result.Revision.ID,
			Title:              result.Blueprint.Title,
			GenerationStatus:   result.Revision.GenerationStatus,
			PublicationStatus:  result.Revision.PublicationStatus,
			CompletenessStatus: result.Revision.CompletenessStatus,
			ContentVersion:     result.Revision.ContentVersion,
			CreatedAt:          result.Blueprint.CreatedAt,
			UpdatedAt:          result.Blueprint.UpdatedAt,
		}
		for _, run := range result.SectionRuns {
			statusResp.Sections = append(statusResp.Sections, dto.BlueprintSectionStatusResponse{
				Type:      run.SectionType,
				Status:    run.Status,
				Attempts:  run.AttemptCount,
				UpdatedAt: run.UpdatedAt,
			})
		}

		respBytes, err := json.Marshal(statusResp)
		require.NoError(t, err)
		respJSON := strings.ToLower(string(respBytes))

		forbiddenTokens := []string{
			"source_context_snapshot",
			"skill_snapshot",
			"content_instruction",
			"prompt",
			"credential",
			"token",
		}
		for _, tok := range forbiddenTokens {
			assert.Falsef(t, strings.Contains(respJSON, tok),
				"public status response must not contain %q in JSON: %s", tok, respJSON)
		}
	})
}
