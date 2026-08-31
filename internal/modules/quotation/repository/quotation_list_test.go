//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestListQuotations_BudgetJoin(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewRepository(pool)

	companyID := uuid.New()
	clientID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	qID := uuid.New()
	quot := &domain.Quotation{
		ID:               qID,
		CompanyID:        companyID,
		ClientID:         clientID,
		QuotationNumber:  "QUO-TEST-001",
		SolutionType:     "custom_software",
		CommercialScheme: "dedicated_squad",
		Status:           "draft",
		CurrentRevision:  2,
		PublicToken:      uuid.New(),
	}

	err := repo.CreateQuotation(ctx, pool, quot)
	require.NoError(t, err)

	// Revision 1: superseded revision with budget 100_000_000
	rev1ID := uuid.New()
	budget1 := int64(100_000_000)
	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number, publication_status,
			generation_status, generation_input, budget_maximum, content_html, content_version
		) VALUES ($1, $2, $3, 1, 'superseded', 'completed', '{}', $4, '<p>Rev 1</p>', 1)
	`, rev1ID, companyID, qID, budget1)
	require.NoError(t, err)

	// Revision 2: active current_revision with budget 250_000_000
	rev2ID := uuid.New()
	budget2 := int64(250_000_000)
	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number, publication_status,
			generation_status, generation_input, budget_maximum, content_html, content_version
		) VALUES ($1, $2, $3, 2, 'draft', 'completed', '{}', $4, '<p>Rev 2</p>', 1)
	`, rev2ID, companyID, qID, budget2)
	require.NoError(t, err)

	// Test ListQuotations: should return active revision budget (250_000_000)
	items, total, err := repo.ListQuotations(ctx, companyID.String(), "", "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Equal(t, qID, items[0].ID)
	require.NotNil(t, items[0].BudgetMaximum, "BudgetMaximum must not be nil")
	assert.Equal(t, budget2, *items[0].BudgetMaximum, "BudgetMaximum should match active revision (revision 2)")

	// Test GetQuotationByID: should return active revision budget (250_000_000)
	got, err := repo.GetQuotationByID(ctx, qID.String(), companyID.String())
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.BudgetMaximum, "GetQuotationByID BudgetMaximum must not be nil")
	assert.Equal(t, budget2, *got.BudgetMaximum, "GetQuotationByID BudgetMaximum should match active revision")
}

func TestListQuotations_BudgetJoin_NilWhenNoRevision(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewRepository(pool)

	companyID := uuid.New()
	clientID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	qID := uuid.New()
	quot := &domain.Quotation{
		ID:               qID,
		CompanyID:        companyID,
		ClientID:         clientID,
		QuotationNumber:  "QUO-TEST-NIL",
		SolutionType:     "custom_software",
		CommercialScheme: "dedicated_squad",
		Status:           "draft",
		CurrentRevision:  1,
		PublicToken:      uuid.New(),
	}

	err := repo.CreateQuotation(ctx, pool, quot)
	require.NoError(t, err)

	// ListQuotations when no revisions exist yet
	items, total, err := repo.ListQuotations(ctx, companyID.String(), "", "", 10, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, items, 1)
	assert.Nil(t, items[0].BudgetMaximum, "BudgetMaximum should be nil when no revision exists")

	// GetQuotationByID when no revisions exist yet
	got, err := repo.GetQuotationByID(ctx, qID.String(), companyID.String())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.BudgetMaximum, "GetQuotationByID BudgetMaximum should be nil when no revision exists")
}
