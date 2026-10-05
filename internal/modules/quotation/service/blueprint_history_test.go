package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakeBlueprintHistoryReader struct {
	recordsFn func(ctx context.Context, blueprintID, companyID uuid.UUID) ([]repository.BlueprintRevisionDetail, error)
}

func (f *fakeBlueprintHistoryReader) ListBlueprintRevisionHistory(ctx context.Context, blueprintID, companyID uuid.UUID) ([]repository.BlueprintRevisionDetail, error) {
	if f.recordsFn != nil {
		return f.recordsFn(ctx, blueprintID, companyID)
	}
	return nil, nil
}

func intPtr(i int) *int {
	return &i
}

func TestBlueprintHistory_ReturnsCompletenessMatrix(t *testing.T) {
	companyID := uuid.New()
	blueprintID := uuid.New()
	rev1ID := uuid.New()
	rev2ID := uuid.New()
	sourceQRev1ID := uuid.New()
	sourceQRev2ID := uuid.New()
	userID := uuid.New()
	now := time.Now().UTC()
	pubAt := now.Add(-time.Hour)

	rev1Details := repository.BlueprintRevisionDetail{
		Revision: domain.BlueprintRevision{
			ID:                        rev1ID,
			CompanyID:                 companyID,
			BlueprintID:               blueprintID,
			RevisionNumber:            1,
			SourceQuotationRevisionID: &sourceQRev1ID,
			PublicationStatus:         "published",
			CompletenessStatus:        "complete",
			GenerationStatus:          "completed",
			CreatedAt:                 now.Add(-2 * time.Hour),
			GeneratedAt:               &pubAt,
			PublishedAt:               &pubAt,
			CreatedBy:                 &userID,
		},
		SourceQuotationRevNumber: intPtr(1),
		SectionRuns: []domain.BlueprintSectionRun{
			{SectionType: "context", Status: "validated"},
			{SectionType: "journey", Status: "completed"},
			{SectionType: "architecture", Status: "completed"},
			{SectionType: "state_machine", Status: "completed"},
			{SectionType: "workflow", Status: "completed"},
			{SectionType: "wbs", Status: "completed"},
			{SectionType: "cross_validation", Status: "validated"},
		},
	}

	rev2Details := repository.BlueprintRevisionDetail{
		Revision: domain.BlueprintRevision{
			ID:                        rev2ID,
			CompanyID:                 companyID,
			BlueprintID:               blueprintID,
			RevisionNumber:            2,
			SourceQuotationRevisionID: &sourceQRev2ID,
			PublicationStatus:         "draft",
			CompletenessStatus:        "partial",
			GenerationStatus:          "in_progress",
			CreatedAt:                 now,
			GeneratedAt:               &now,
			CreatedBy:                 &userID,
		},
		SourceQuotationRevNumber: intPtr(2),
		SectionRuns: []domain.BlueprintSectionRun{
			{SectionType: "context", Status: "validated"},
			{SectionType: "journey", Status: "completed"},
			{SectionType: "architecture", Status: "failed"},
			{SectionType: "state_machine", Status: "pending"},
		},
	}

	histReader := &fakeBlueprintHistoryReader{
		recordsFn: func(ctx context.Context, bID, cID uuid.UUID) ([]repository.BlueprintRevisionDetail, error) {
			if bID == blueprintID && cID == companyID {
				// Revisions ordered DESC by revision_number
				return []repository.BlueprintRevisionDetail{rev2Details, rev1Details}, nil
			}
			return nil, repository.ErrBlueprintNotFound
		},
	}

	svc := NewBlueprintReadService(nil, nil, histReader)

	// Test ListRevisions
	history, err := svc.ListRevisions(context.Background(), companyID, blueprintID)
	require.NoError(t, err)
	require.Len(t, history, 2)

	// Verify revision 2 (draft, partial)
	item2 := history[0]
	assert.Equal(t, rev2ID, item2.ID)
	assert.Equal(t, 2, item2.RevisionNumber)
	assert.Equal(t, "draft", item2.PublicationStatus)
	assert.Equal(t, "partial", item2.CompletenessStatus)
	require.NotNil(t, item2.SourceQuotationRevisionID)
	assert.Equal(t, sourceQRev2ID, *item2.SourceQuotationRevisionID)
	require.NotNil(t, item2.SourceQuotationRevNumber)
	assert.Equal(t, 2, *item2.SourceQuotationRevNumber)
	require.NotNil(t, item2.CompletenessMatrix)
	assert.Equal(t, "validated", item2.CompletenessMatrix["context"])
	assert.Equal(t, "completed", item2.CompletenessMatrix["journey"])
	assert.Equal(t, "failed", item2.CompletenessMatrix["architecture"])
	assert.Equal(t, "pending", item2.CompletenessMatrix["state_machine"])

	// Verify revision 1 (published, complete)
	item1 := history[1]
	assert.Equal(t, rev1ID, item1.ID)
	assert.Equal(t, 1, item1.RevisionNumber)
	assert.Equal(t, "published", item1.PublicationStatus)
	assert.Equal(t, "complete", item1.CompletenessStatus)
	require.NotNil(t, item1.SourceQuotationRevisionID)
	assert.Equal(t, sourceQRev1ID, *item1.SourceQuotationRevisionID)
	require.NotNil(t, item1.SourceQuotationRevNumber)
	assert.Equal(t, 1, *item1.SourceQuotationRevNumber)
	require.NotNil(t, item1.CompletenessMatrix)
	assert.Equal(t, "validated", item1.CompletenessMatrix["context"])
	assert.Equal(t, "completed", item1.CompletenessMatrix["journey"])
	assert.Equal(t, "completed", item1.CompletenessMatrix["architecture"])
	assert.Equal(t, "completed", item1.CompletenessMatrix["state_machine"])
	assert.Equal(t, "completed", item1.CompletenessMatrix["workflow"])
	assert.Equal(t, "completed", item1.CompletenessMatrix["wbs"])
	assert.Equal(t, "validated", item1.CompletenessMatrix["cross_validation"])

	// Verify GetBlueprintHistory alias returns identical results
	aliasHistory, err := svc.GetBlueprintHistory(context.Background(), companyID, blueprintID)
	require.NoError(t, err)
	assert.Equal(t, history, aliasHistory)
}

func TestBlueprintHistory_TenantIsolation(t *testing.T) {
	authorizedCompanyID := uuid.New()
	unauthorizedCompanyID := uuid.New()
	blueprintID := uuid.New()

	histReader := &fakeBlueprintHistoryReader{
		recordsFn: func(ctx context.Context, bID, cID uuid.UUID) ([]repository.BlueprintRevisionDetail, error) {
			if bID == blueprintID && cID == authorizedCompanyID {
				return []repository.BlueprintRevisionDetail{
					{
						Revision: domain.BlueprintRevision{
							ID:             uuid.New(),
							RevisionNumber: 1,
						},
					},
				}, nil
			}
			return nil, repository.ErrBlueprintNotFound
		},
	}

	svc := NewBlueprintReadService(nil, nil, histReader)
	ctx := context.Background()

	// 1. Nil company returns ErrBlueprintNotFound
	_, err := svc.ListRevisions(ctx, uuid.Nil, blueprintID)
	assert.ErrorIs(t, err, ErrBlueprintNotFound)

	// 2. Nil blueprint returns ErrBlueprintNotFound
	_, err = svc.ListRevisions(ctx, authorizedCompanyID, uuid.Nil)
	assert.ErrorIs(t, err, ErrBlueprintNotFound)

	// 3. Unauthorized company returns ErrBlueprintNotFound
	_, err = svc.ListRevisions(ctx, unauthorizedCompanyID, blueprintID)
	assert.ErrorIs(t, err, ErrBlueprintNotFound)

	// 4. Authorized company succeeds
	items, err := svc.ListRevisions(ctx, authorizedCompanyID, blueprintID)
	require.NoError(t, err)
	assert.Len(t, items, 1)
}
