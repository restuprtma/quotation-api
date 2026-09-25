package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestBlueprintReadRepository_BlueprintList_NilCompanyReturnsEmpty(t *testing.T) {
	repo := repository.NewBlueprintReadRepository(nil)
	items, total, err := repo.ListBlueprints(context.Background(), uuid.Nil, repository.BlueprintListParams{
		Page:  1,
		Limit: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}

func TestBlueprintReadRepository_BlueprintList_NilQuotationIDReturnsEmpty(t *testing.T) {
	repo := repository.NewBlueprintReadRepository(nil)
	nilUUID := uuid.Nil
	items, total, err := repo.ListBlueprints(context.Background(), uuid.New(), repository.BlueprintListParams{
		QuotationID: &nilUUID,
		Page:        1,
		Limit:       20,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, total)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}
