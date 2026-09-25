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
	items      []dto.BlueprintListItemResponse
	total      int
	err        error
	lastParams repository.BlueprintListParams
	lastCompID uuid.UUID
}

func (f *fakeBlueprintListReader) ListBlueprints(_ context.Context, companyID uuid.UUID, params repository.BlueprintListParams) ([]dto.BlueprintListItemResponse, int, error) {
	f.lastCompID = companyID
	f.lastParams = params
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.items, f.total, nil
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

func TestBlueprintReadService_BlueprintList_EmptySliceWhenNoItems(t *testing.T) {
	companyID := uuid.New()
	genReader := &fakeBlueprintGenerationReader{}
	listReader := &fakeBlueprintListReader{items: nil, total: 0}
	svc := NewBlueprintReadService(genReader, listReader)

	ctx := context.Background()
	params := repository.BlueprintListParams{Page: 1, Limit: 20}
	resp, err := svc.List(ctx, companyID, params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Items)
	assert.Empty(t, resp.Items)
	assert.Equal(t, 0, resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 20, resp.Limit)
	assert.Equal(t, 0, resp.TotalPages)

	// Verify JSON serialization produces items: []
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[],"total":0,"page":1,"limit":20,"total_pages":0}`, string(b))
}

func TestBlueprintReadService_BlueprintList_PaginationCalculations(t *testing.T) {
	tests := []struct {
		name               string
		total              int
		limit              int
		page               int
		expectedTotalPages int
		expectedPage       int
		expectedLimit      int
	}{
		{
			name:               "total 0 yields total_pages 0",
			total:              0,
			limit:              10,
			page:               1,
			expectedTotalPages: 0,
			expectedPage:       1,
			expectedLimit:      10,
		},
		{
			name:               "total 1 with limit 10 yields total_pages 1",
			total:              1,
			limit:              10,
			page:               1,
			expectedTotalPages: 1,
			expectedPage:       1,
			expectedLimit:      10,
		},
		{
			name:               "exact multiple total 10 with limit 10 yields total_pages 1",
			total:              10,
			limit:              10,
			page:               1,
			expectedTotalPages: 1,
			expectedPage:       1,
			expectedLimit:      10,
		},
		{
			name:               "total 11 with limit 10 yields total_pages 2 (ceil)",
			total:              11,
			limit:              10,
			page:               2,
			expectedTotalPages: 2,
			expectedPage:       2,
			expectedLimit:      10,
		},
		{
			name:               "total 25 with limit 10 yields total_pages 3",
			total:              25,
			limit:              10,
			page:               1,
			expectedTotalPages: 3,
			expectedPage:       1,
			expectedLimit:      10,
		},
		{
			name:               "total 6 with limit 5 yields total_pages 2",
			total:              6,
			limit:              5,
			page:               1,
			expectedTotalPages: 2,
			expectedPage:       1,
			expectedLimit:      5,
		},
		{
			name:               "clamps default page < 1 to 1, limit < 1 to 20",
			total:              50,
			limit:              0,
			page:               0,
			expectedTotalPages: 3,
			expectedPage:       1,
			expectedLimit:      20,
		},
		{
			name:               "clamps limit > 100 to 100",
			total:              250,
			limit:              150,
			page:               1,
			expectedTotalPages: 3,
			expectedPage:       1,
			expectedLimit:      100,
		},
	}

	companyID := uuid.New()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			listReader := &fakeBlueprintListReader{
				items: make([]dto.BlueprintListItemResponse, 0),
				total: tc.total,
			}
			svc := NewBlueprintReadService(&fakeBlueprintGenerationReader{}, listReader)

			resp, err := svc.List(context.Background(), companyID, repository.BlueprintListParams{
				Page:  tc.page,
				Limit: tc.limit,
			})
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tc.total, resp.Total)
			assert.Equal(t, tc.expectedPage, resp.Page)
			assert.Equal(t, tc.expectedLimit, resp.Limit)
			assert.Equal(t, tc.expectedTotalPages, resp.TotalPages)
		})
	}
}

func TestBlueprintReadService_BlueprintList_ForwardsParamsAndHandlesNilCompany(t *testing.T) {
	companyID := uuid.New()
	qID := uuid.New()
	statuses := []string{"pending", "in_progress", "partial_failed", "failed"}

	listReader := &fakeBlueprintListReader{
		items: []dto.BlueprintListItemResponse{
			{BlueprintID: uuid.New(), QuotationID: qID, Title: "Test BP", GenerationStatus: "pending", UpdatedAt: time.Now()},
		},
		total: 1,
	}
	svc := NewBlueprintReadService(&fakeBlueprintGenerationReader{}, listReader)

	params := repository.BlueprintListParams{
		QuotationID:        &qID,
		GenerationStatuses: statuses,
		Page:               2,
		Limit:              5,
	}

	resp, err := svc.List(context.Background(), companyID, params)
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 1, resp.Total)
	assert.Equal(t, 2, resp.Page)
	assert.Equal(t, 5, resp.Limit)
	assert.Equal(t, 1, resp.TotalPages)
	require.Len(t, resp.Items, 1)

	// Verify params forwarded to reader
	assert.Equal(t, companyID, listReader.lastCompID)
	assert.Equal(t, &qID, listReader.lastParams.QuotationID)
	assert.Equal(t, statuses, listReader.lastParams.GenerationStatuses)
	assert.Equal(t, 2, listReader.lastParams.Page)
	assert.Equal(t, 5, listReader.lastParams.Limit)

	// Test nil company ID
	nilCompanyResp, err := svc.List(context.Background(), uuid.Nil, params)
	require.NoError(t, err)
	require.NotNil(t, nilCompanyResp)
	assert.Empty(t, nilCompanyResp.Items)
	assert.Equal(t, 0, nilCompanyResp.Total)
	assert.Equal(t, 0, nilCompanyResp.TotalPages)

	// Test error from reader
	listReaderErr := errors.New("db connection failure")
	errReader := &fakeBlueprintListReader{err: listReaderErr}
	errSvc := NewBlueprintReadService(&fakeBlueprintGenerationReader{}, errReader)
	_, err = errSvc.List(context.Background(), companyID, params)
	assert.ErrorIs(t, err, listReaderErr)
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
