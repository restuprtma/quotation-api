package service_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type mockRevisionRepo struct {
	listRevisionsFunc func(ctx context.Context, quotationID, companyID string) ([]domain.QuotationRevision, error)
}

func (m *mockRevisionRepo) ListRevisionsByQuotationID(ctx context.Context, quotationID, companyID string) ([]domain.QuotationRevision, error) {
	if m.listRevisionsFunc != nil {
		return m.listRevisionsFunc(ctx, quotationID, companyID)
	}
	return nil, nil
}

type mockQuotationRepo struct {
	getQuotationFunc func(ctx context.Context, id, companyID string) (*domain.Quotation, error)
}

func (m *mockQuotationRepo) GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
	if m.getQuotationFunc != nil {
		return m.getQuotationFunc(ctx, id, companyID)
	}
	return &domain.Quotation{ID: uuid.MustParse(id)}, nil
}

func strPtr(s string) *string {
	return &s
}

func TestService_ListRevisions_MissingCompanyID(t *testing.T) {
	s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := s.ListRevisions(context.Background(), "", uuid.New().String())
	assert.ErrorIs(t, err, service.ErrNoCompanyContext)
}

func TestService_ListRevisions_QuotationNotFound(t *testing.T) {
	mockQRepo := &mockQuotationRepo{
		getQuotationFunc: func(ctx context.Context, id, companyID string) (*domain.Quotation, error) {
			return nil, repository.ErrNotFound
		},
	}

	s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
		WithQuotationRepository(mockQRepo)

	_, err := s.ListRevisions(context.Background(), uuid.New().String(), uuid.New().String())
	assert.ErrorIs(t, err, service.ErrNotFound)
}

func TestService_ListRevisions_EligibilityAndSerialization(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()

	rev1ID := uuid.New()
	rev2ID := uuid.New()
	rev3ID := uuid.New()
	rev4ID := uuid.New()
	rev5ID := uuid.New()
	rev6ID := uuid.New()
	rev7ID := uuid.New()

	mockQRepo := &mockQuotationRepo{
		getQuotationFunc: func(ctx context.Context, id, compID string) (*domain.Quotation, error) {
			return &domain.Quotation{ID: uuid.MustParse(quotationID)}, nil
		},
	}

	mockRRepo := &mockRevisionRepo{
		listRevisionsFunc: func(ctx context.Context, qID, compID string) ([]domain.QuotationRevision, error) {
			return []domain.QuotationRevision{
				// 1. completed nonblank draft -> eligible
				{
					ID:                rev1ID,
					RevisionNumber:    1,
					PublicationStatus: "draft",
					GenerationStatus:  "completed",
					ContentHTML:       strPtr("<h1>Draft Proposal</h1>"),
				},
				// 2. completed nonblank published -> eligible
				{
					ID:                rev2ID,
					RevisionNumber:    2,
					PublicationStatus: "published",
					GenerationStatus:  "completed",
					ContentHTML:       strPtr("<p>Published Proposal</p>"),
				},
				// 3. completed nonblank superseded -> eligible
				{
					ID:                rev3ID,
					RevisionNumber:    3,
					PublicationStatus: "superseded",
					GenerationStatus:  "completed",
					ContentHTML:       strPtr("<p>Superseded Proposal</p>"),
				},
				// 4. incomplete generation -> ineligible ("revision generation is incomplete")
				{
					ID:                rev4ID,
					RevisionNumber:    4,
					PublicationStatus: "draft",
					GenerationStatus:  "generating",
					ContentHTML:       strPtr("<p>Work in progress</p>"),
				},
				// 5. unsupported publication status -> ineligible ("unsupported publication status")
				{
					ID:                rev5ID,
					RevisionNumber:    5,
					PublicationStatus: "archived",
					GenerationStatus:  "completed",
					ContentHTML:       strPtr("<p>Archived content</p>"),
				},
				// 6. blank nil content HTML -> ineligible ("revision content is blank")
				{
					ID:                rev6ID,
					RevisionNumber:    6,
					PublicationStatus: "draft",
					GenerationStatus:  "completed",
					ContentHTML:       nil,
				},
				// 7. blank whitespace content HTML -> ineligible ("revision content is blank")
				{
					ID:                rev7ID,
					RevisionNumber:    7,
					PublicationStatus: "published",
					GenerationStatus:  "completed",
					ContentHTML:       strPtr("   \n\t  "),
				},
			}, nil
		},
	}

	s := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
		WithQuotationRepository(mockQRepo).
		WithRevisionRepository(mockRRepo)

	res, err := s.ListRevisions(context.Background(), companyID, quotationID)
	require.NoError(t, err)
	require.Len(t, res, 7)

	// Rev 1
	assert.Equal(t, rev1ID, res[0].ID)
	assert.Equal(t, 1, res[0].VersionNumber)
	assert.Equal(t, "draft", res[0].Status)
	require.NotNil(t, res[0].BlueprintSourceMetadata)
	assert.True(t, res[0].BlueprintSourceMetadata.BlueprintEligible)
	assert.Nil(t, res[0].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Rev 2
	assert.Equal(t, rev2ID, res[1].ID)
	require.NotNil(t, res[1].BlueprintSourceMetadata)
	assert.True(t, res[1].BlueprintSourceMetadata.BlueprintEligible)
	assert.Nil(t, res[1].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Rev 3
	assert.Equal(t, rev3ID, res[2].ID)
	require.NotNil(t, res[2].BlueprintSourceMetadata)
	assert.True(t, res[2].BlueprintSourceMetadata.BlueprintEligible)
	assert.Nil(t, res[2].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Rev 4 (incomplete)
	assert.Equal(t, rev4ID, res[3].ID)
	require.NotNil(t, res[3].BlueprintSourceMetadata)
	assert.False(t, res[3].BlueprintSourceMetadata.BlueprintEligible)
	require.NotNil(t, res[3].BlueprintSourceMetadata.BlueprintIneligibleReason)
	assert.Equal(t, "revision generation is incomplete", *res[3].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Rev 5 (unsupported pub status)
	assert.Equal(t, rev5ID, res[4].ID)
	require.NotNil(t, res[4].BlueprintSourceMetadata)
	assert.False(t, res[4].BlueprintSourceMetadata.BlueprintEligible)
	require.NotNil(t, res[4].BlueprintSourceMetadata.BlueprintIneligibleReason)
	assert.Equal(t, "unsupported publication status", *res[4].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Rev 6 (nil content)
	assert.Equal(t, rev6ID, res[5].ID)
	require.NotNil(t, res[5].BlueprintSourceMetadata)
	assert.False(t, res[5].BlueprintSourceMetadata.BlueprintEligible)
	require.NotNil(t, res[5].BlueprintSourceMetadata.BlueprintIneligibleReason)
	assert.Equal(t, "revision content is blank", *res[5].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Rev 7 (whitespace content)
	assert.Equal(t, rev7ID, res[6].ID)
	require.NotNil(t, res[6].BlueprintSourceMetadata)
	assert.False(t, res[6].BlueprintSourceMetadata.BlueprintEligible)
	require.NotNil(t, res[6].BlueprintSourceMetadata.BlueprintIneligibleReason)
	assert.Equal(t, "revision content is blank", *res[6].BlueprintSourceMetadata.BlueprintIneligibleReason)

	// Inspect JSON serialization to confirm list item contract
	marshaled, err := json.Marshal(res)
	require.NoError(t, err)
	marshaledStr := string(marshaled)
	assert.Contains(t, marshaledStr, `"blueprint_eligible":true`)
	assert.Contains(t, marshaledStr, `"blueprint_ineligible_reason":null`)
	assert.Contains(t, marshaledStr, `"blueprint_ineligible_reason":"revision generation is incomplete"`)
	assert.NotContains(t, marshaledStr, "content_html")
}
