//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestBlueprintReadRepository_ListBlueprintsTenantIsolationAndProjection(t *testing.T) {
	pool := getTestPool(t)

	// Seed tenant A with 2 blueprints
	fA1 := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, fA1.CompanyID)
	fA2 := seedBlueprintSource(t, pool)
	// Both under same company
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Update fA2's quotation to use fA1's companyID so tenant A owns two blueprints
	_, err := pool.Exec(ctx, `UPDATE quotations SET company_id = $1 WHERE id = $2`, fA1.CompanyID, fA2.QuotationID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE quotation_revisions SET company_id = $1 WHERE id = $2`, fA1.CompanyID, fA2.RevisionID)
	require.NoError(t, err)
	fA2.CompanyID = fA1.CompanyID

	// Seed tenant B with 1 blueprint
	fB := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, fB.CompanyID)

	genRepo := repository.NewBlueprintGenerationRepository(pool)
	readRepo := repository.NewBlueprintReadRepository(pool)

	paramsA1 := validBlueprintCreateParams(fA1)
	paramsA1.Title = "Tenant A Blueprint One"
	resA1, err := genRepo.CreateBlueprintGeneration(ctx, paramsA1)
	require.NoError(t, err)

	paramsA2 := validBlueprintCreateParams(fA2)
	paramsA2.Title = "Tenant A Blueprint Two"
	resA2, err := genRepo.CreateBlueprintGeneration(ctx, paramsA2)
	require.NoError(t, err)

	paramsB := validBlueprintCreateParams(fB)
	paramsB.Title = "Tenant B Blueprint"
	resB, err := genRepo.CreateBlueprintGeneration(ctx, paramsB)
	require.NoError(t, err)

	// 1. List for Tenant A -> returns exactly 2 items
	listA, err := readRepo.ListBlueprints(ctx, fA1.CompanyID, nil)
	require.NoError(t, err)
	require.Len(t, listA, 2)

	titlesA := []string{listA[0].Title, listA[1].Title}
	assert.Contains(t, titlesA, "Tenant A Blueprint One")
	assert.Contains(t, titlesA, "Tenant A Blueprint Two")
	assert.NotContains(t, titlesA, "Tenant B Blueprint")

	for _, item := range listA {
		assert.NotEqual(t, uuid.Nil, item.BlueprintID)
		assert.NotEqual(t, uuid.Nil, item.QuotationID)
		assert.NotEmpty(t, item.GenerationStatus)
		assert.False(t, item.UpdatedAt.IsZero())
	}

	// 2. List for Tenant B -> returns exactly 1 item
	listB, err := readRepo.ListBlueprints(ctx, fB.CompanyID, nil)
	require.NoError(t, err)
	require.Len(t, listB, 1)
	assert.Equal(t, resB.Blueprint.ID, listB[0].BlueprintID)
	assert.Equal(t, "Tenant B Blueprint", listB[0].Title)

	// 3. Filter by quotationID for Tenant A (matching quotation 1)
	filteredA1, err := readRepo.ListBlueprints(ctx, fA1.CompanyID, &fA1.QuotationID)
	require.NoError(t, err)
	require.Len(t, filteredA1, 1)
	assert.Equal(t, resA1.Blueprint.ID, filteredA1[0].BlueprintID)

	// 4. Filter by foreign quotationID (Tenant B's quotation queried by Tenant A) -> returns empty slice []
	foreignFilter, err := readRepo.ListBlueprints(ctx, fA1.CompanyID, &fB.QuotationID)
	require.NoError(t, err)
	assert.NotNil(t, foreignFilter)
	assert.Empty(t, foreignFilter)

	// 5. Filter by non-existent quotationID -> returns empty slice []
	randomQID := uuid.New()
	missingFilter, err := readRepo.ListBlueprints(ctx, fA1.CompanyID, &randomQID)
	require.NoError(t, err)
	assert.NotNil(t, missingFilter)
	assert.Empty(t, missingFilter)

	// 6. List for unknown company -> returns empty slice []
	randomCompany := uuid.New()
	emptyList, err := readRepo.ListBlueprints(ctx, randomCompany, nil)
	require.NoError(t, err)
	assert.NotNil(t, emptyList)
	assert.Empty(t, emptyList)

	// 7. Soft delete one blueprint -> excluded from list
	_, err = pool.Exec(ctx, `UPDATE blueprints SET deleted_at = NOW() WHERE id = $1`, resA2.Blueprint.ID)
	require.NoError(t, err)

	listAAfterDelete, err := readRepo.ListBlueprints(ctx, fA1.CompanyID, nil)
	require.NoError(t, err)
	require.Len(t, listAAfterDelete, 1)
	assert.Equal(t, resA1.Blueprint.ID, listAAfterDelete[0].BlueprintID)
}

func TestBlueprintReadRepository_GetBlueprintGenerationSnapshotDelegation(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	genRepo := repository.NewBlueprintGenerationRepository(pool)
	readRepo := repository.NewBlueprintReadRepository(pool)

	params := validBlueprintCreateParams(f)
	created, err := genRepo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	// Read via BlueprintReadRepository
	readRes, err := readRepo.GetBlueprintGeneration(ctx, created.Blueprint.ID, f.CompanyID)
	require.NoError(t, err)
	require.NotNil(t, readRes)

	assert.Equal(t, created.Blueprint.ID, readRes.Blueprint.ID)
	assert.Equal(t, created.Revision.ID, readRes.Revision.ID)
	assert.Equal(t, created.Blueprint.Title, readRes.Blueprint.Title)
	require.Len(t, readRes.SectionRuns, 7)

	// Foreign tenant returns ErrBlueprintNotFound
	foreignCompany := uuid.New()
	_, err = readRepo.GetBlueprintGeneration(ctx, created.Blueprint.ID, foreignCompany)
	assert.ErrorIs(t, err, repository.ErrBlueprintNotFound)
}
