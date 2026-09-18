package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakeBlueprintGenerationReader struct {
	result *repository.CreateBlueprintGenerationResult
	err    error
}

func (f *fakeBlueprintGenerationReader) GetBlueprintGeneration(_ context.Context, blueprintID, companyID uuid.UUID) (*repository.CreateBlueprintGenerationResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.result == nil {
		return nil, repository.ErrBlueprintNotFound
	}
	if f.result.Blueprint.CompanyID != companyID || f.result.Blueprint.ID != blueprintID {
		return nil, repository.ErrBlueprintNotFound
	}
	return f.result, nil
}

type fakeBlueprintListReader struct {
	items []dto.BlueprintListItemResponse
	err   error
}

func (f *fakeBlueprintListReader) ListBlueprints(_ context.Context, companyID uuid.UUID, quotationID *uuid.UUID) ([]dto.BlueprintListItemResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items, nil
}

func makeSampleBlueprintResult(companyID uuid.UUID) *repository.CreateBlueprintGenerationResult {
	bpID := uuid.New()
	quotationID := uuid.New()
	revID := uuid.New()
	now := time.Now().UTC()
	prov := "gpt-4o"

	return &repository.CreateBlueprintGenerationResult{
		Blueprint: domain.Blueprint{
			ID:              bpID,
			CompanyID:       companyID,
			QuotationID:     quotationID,
			Title:           "Enterprise Architecture Blueprint",
			Status:          "draft",
			CurrentRevision: 1,
			CreatedAt:       now,
			UpdatedAt:       now,
		},
		Revision: domain.BlueprintRevision{
			ID:                    revID,
			CompanyID:             companyID,
			BlueprintID:           bpID,
			RevisionNumber:        1,
			PublicationStatus:     "draft",
			CompletenessStatus:    "partial",
			GenerationStatus:      string(domain.BlueprintGenerationPending),
			ContentJSON:           json.RawMessage(`{"modules":["auth","billing"]}`),
			ContentVersion:        1,
			ValidationResult:      json.RawMessage(`{"valid":true}`),
			CrossValidationResult: json.RawMessage(`{"cross_valid":true}`),
			SourceContextSnapshot: json.RawMessage(`{"scope":"crm"}`),
			SkillSnapshot:         json.RawMessage(`{"skill":"arch"}`),
			AiProvider:            &prov,
			CreatedAt:             now,
			UpdatedAt:             now,
		},
		SectionRuns: []domain.BlueprintSectionRun{
			{
				ID:                  uuid.New(),
				CompanyID:           companyID,
				BlueprintRevisionID: revID,
				SectionType:         string(domain.BlueprintSectionContext),
				Status:              string(domain.BlueprintSectionValidated),
				AttemptCount:        0,
				ContentJSON:         json.RawMessage(`{"summary":"Context section"}`),
				ValidationResult:    json.RawMessage(`{}`),
				UpdatedAt:           now,
			},
		},
	}
}

func TestBlueprintReadService_CanonicalSectionsOrderAndDefaults(t *testing.T) {
	companyID := uuid.New()
	sample := makeSampleBlueprintResult(companyID)

	// Note: only Context run is present in SectionRuns
	genReader := &fakeBlueprintGenerationReader{result: sample}
	listReader := &fakeBlueprintListReader{}
	svc := NewBlueprintReadService(genReader, listReader)

	ctx := context.Background()
	status, err := svc.GetStatus(ctx, companyID, sample.Blueprint.ID)
	require.NoError(t, err)
	require.NotNil(t, status)

	assert.Equal(t, sample.Blueprint.ID, status.BlueprintID)
	assert.Equal(t, sample.Revision.ID, status.RevisionID)
	assert.Equal(t, sample.Blueprint.Title, status.Title)
	assert.Equal(t, 1, status.ContentVersion)

	// Check canonical 7 sections in order
	expectedOrder := []string{
		"context", "journey", "architecture", "state_machine", "workflow", "wbs", "cross_validation",
	}
	require.Len(t, status.Sections, 7)
	for i, expected := range expectedOrder {
		sec := status.Sections[i]
		assert.Equal(t, expected, sec.Type)
		if expected == "context" {
			assert.Equal(t, "validated", sec.Status)
			assert.Equal(t, 0, sec.Attempts)
		} else {
			assert.Equal(t, "pending", sec.Status)
			assert.Equal(t, 0, sec.Attempts)
		}
	}
}

func TestBlueprintReadService_CanonicalSectionsContextDefaultWhenMissingFromRuns(t *testing.T) {
	companyID := uuid.New()
	sample := makeSampleBlueprintResult(companyID)
	// Empty runs slice
	sample.SectionRuns = nil

	genReader := &fakeBlueprintGenerationReader{result: sample}
	listReader := &fakeBlueprintListReader{}
	svc := NewBlueprintReadService(genReader, listReader)

	ctx := context.Background()
	status, err := svc.GetStatus(ctx, companyID, sample.Blueprint.ID)
	require.NoError(t, err)
	require.NotNil(t, status)

	require.Len(t, status.Sections, 7)
	assert.Equal(t, "context", status.Sections[0].Type)
	assert.Equal(t, "validated", status.Sections[0].Status)
	assert.Equal(t, 0, status.Sections[0].Attempts)

	for i := 1; i < 7; i++ {
		assert.Equal(t, "pending", status.Sections[i].Status)
		assert.Equal(t, 0, status.Sections[i].Attempts)
	}
}

func TestBlueprintReadService_MissingOrForeignTenantReturnsErrNotFound(t *testing.T) {
	companyA := uuid.New()
	companyB := uuid.New()
	sample := makeSampleBlueprintResult(companyA)

	genReader := &fakeBlueprintGenerationReader{result: sample}
	listReader := &fakeBlueprintListReader{}
	svc := NewBlueprintReadService(genReader, listReader)
	ctx := context.Background()

	// Foreign company ID on GetStatus
	_, err := svc.GetStatus(ctx, companyB, sample.Blueprint.ID)
	assert.ErrorIs(t, err, repository.ErrBlueprintNotFound)

	// Missing blueprint ID on GetStatus
	_, err = svc.GetStatus(ctx, companyA, uuid.New())
	assert.ErrorIs(t, err, repository.ErrBlueprintNotFound)

	// Foreign company ID on GetDetail
	_, err = svc.GetDetail(ctx, companyB, sample.Blueprint.ID)
	assert.ErrorIs(t, err, repository.ErrBlueprintNotFound)

	// Missing blueprint ID on GetDetail
	_, err = svc.GetDetail(ctx, companyA, uuid.New())
	assert.ErrorIs(t, err, repository.ErrBlueprintNotFound)
}

func TestBlueprintReadService_ListReturnsEmptySliceWhenNoItems(t *testing.T) {
	companyID := uuid.New()
	genReader := &fakeBlueprintGenerationReader{}
	listReader := &fakeBlueprintListReader{items: nil} // nil slice from repo
	svc := NewBlueprintReadService(genReader, listReader)

	ctx := context.Background()
	resp, err := svc.List(ctx, companyID, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Items)
	assert.Empty(t, resp.Items)

	// Verify JSON serialization produces items: []
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[]}`, string(b))
}

func TestBlueprintReadService_RedactionOfSensitiveFields(t *testing.T) {
	companyID := uuid.New()
	sample := makeSampleBlueprintResult(companyID)

	// Inject sensitive canary values
	secretSkill := "SECRET_SKILL_KEY_VALUE_XYZ"
	secretSource := "SECRET_SOURCE_INTERNAL_PAYLOAD"
	secretProvider := "SECRET_PROVIDER_API_TOKEN"

	sample.Revision.SkillSnapshot = json.RawMessage(`{"token":"` + secretSkill + `"}`)
	sample.Revision.SourceContextSnapshot = json.RawMessage(`{"confidential":"` + secretSource + `"}`)
	sample.Revision.AiProvider = &secretProvider

	genReader := &fakeBlueprintGenerationReader{result: sample}
	listReader := &fakeBlueprintListReader{}
	svc := NewBlueprintReadService(genReader, listReader)

	ctx := context.Background()
	detail, err := svc.GetDetail(ctx, companyID, sample.Blueprint.ID)
	require.NoError(t, err)
	require.NotNil(t, detail)

	// Assert detail.Status has matching RevisionID and ContentVersion
	assert.Equal(t, sample.Revision.ID, detail.Status.RevisionID)
	assert.Equal(t, sample.Revision.ContentVersion, detail.Status.ContentVersion)
	assert.Equal(t, sample.Blueprint.QuotationID, detail.QuotationID)

	// Marshal to JSON and assert secrets are completely absent
	b, err := json.Marshal(detail)
	require.NoError(t, err)
	jsonStr := string(b)

	assert.NotContains(t, jsonStr, "SECRET_SKILL")
	assert.NotContains(t, jsonStr, "SECRET_SOURCE")
	assert.NotContains(t, jsonStr, "SECRET_PROVIDER")
	assert.NotContains(t, jsonStr, secretSkill)
	assert.NotContains(t, jsonStr, secretSource)
	assert.NotContains(t, jsonStr, secretProvider)
}

func TestBlueprintReadService_CorruptStoredJSONReturnsSafeInternalError(t *testing.T) {
	companyID := uuid.New()

	tests := []struct {
		name       string
		mutateFunc func(r *repository.CreateBlueprintGenerationResult)
	}{
		{
			name: "corrupt ContentJSON",
			mutateFunc: func(r *repository.CreateBlueprintGenerationResult) {
				r.Revision.ContentJSON = json.RawMessage(`{"modules": [unclosed`)
			},
		},
		{
			name: "corrupt ValidationResult",
			mutateFunc: func(r *repository.CreateBlueprintGenerationResult) {
				r.Revision.ValidationResult = json.RawMessage(`{not-json}`)
			},
		},
		{
			name: "corrupt CrossValidationResult",
			mutateFunc: func(r *repository.CreateBlueprintGenerationResult) {
				r.Revision.CrossValidationResult = json.RawMessage(`{invalid`)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sample := makeSampleBlueprintResult(companyID)
			tc.mutateFunc(sample)

			genReader := &fakeBlueprintGenerationReader{result: sample}
			listReader := &fakeBlueprintListReader{}
			svc := NewBlueprintReadService(genReader, listReader)

			ctx := context.Background()
			detail, err := svc.GetDetail(ctx, companyID, sample.Blueprint.ID)
			assert.Error(t, err)
			assert.Nil(t, detail)
			assert.True(t, errors.Is(err, ErrCorruptBlueprintContent), "expected ErrCorruptBlueprintContent, got %v", err)
		})
	}
}

func TestBlueprintReadService_EmptyJSONDefaultsToEmptyObject(t *testing.T) {
	companyID := uuid.New()
	sample := makeSampleBlueprintResult(companyID)
	sample.Revision.ContentJSON = nil
	sample.Revision.ValidationResult = nil
	sample.Revision.CrossValidationResult = []byte("")

	genReader := &fakeBlueprintGenerationReader{result: sample}
	listReader := &fakeBlueprintListReader{}
	svc := NewBlueprintReadService(genReader, listReader)

	ctx := context.Background()
	detail, err := svc.GetDetail(ctx, companyID, sample.Blueprint.ID)
	require.NoError(t, err)
	require.NotNil(t, detail)

	assert.JSONEq(t, "{}", string(detail.ContentJSON))
	assert.JSONEq(t, "{}", string(detail.ValidationResult))
	assert.JSONEq(t, "{}", string(detail.CrossValidationResult))
}
