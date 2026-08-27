//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestLoadDraftRevisionForSave_Success(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	draft, err := repo.LoadDraftRevisionForSave(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.Equal(t, fx.revisionID, draft.RevisionID)
	assert.Equal(t, fx.quotationID, draft.QuotationID)
	assert.Equal(t, 1, draft.RevisionNumber)
	assert.Equal(t, repository.PublicationStatusDraft, draft.PublicationStatus)
	assert.Equal(t, 1, draft.ContentVersion)
	assert.False(t, draft.LegalContentModified)
	assert.False(t, draft.CommercialContentModified)
}

func TestLoadDraftRevisionForSave_NotFound(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := repo.LoadDraftRevisionForSave(ctx, uuid.New(), fx.quotationID)
	assert.ErrorIs(t, err, repository.ErrRevisionNotFound)

	_, err = repo.LoadDraftRevisionForSave(ctx, fx.companyID, uuid.New())
	assert.ErrorIs(t, err, repository.ErrRevisionNotFound)
}

func TestLoadDraftRevisionForSave_NonDraftStatus(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET publication_status = 'published' WHERE id = $1`, fx.revisionID)
	require.NoError(t, err)

	_, err = repo.LoadDraftRevisionForSave(ctx, fx.companyID, fx.quotationID)
	assert.ErrorIs(t, err, repository.ErrRevisionConflict)
}

func TestUpdateDraft_Success_IncrementsVersion(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	userID := uuid.New()
	now := time.Now().UTC()
	hash := "test-hash-123"

	res, err := repo.UpdateDraft(ctx, repo.Pool(), fx.companyID, fx.quotationID, repository.UpdateDraftParams{
		RevisionID:                fx.revisionID,
		ExpectedVersion:           1,
		ContentHTML:               "<section data-block=\"legal\">Updated Legal</section>",
		ContentHash:               &hash,
		LegalContentModified:      true,
		LegalModifiedBy:           &userID,
		LegalModifiedAt:           &now,
		CommercialContentModified: false,
	})
	require.NoError(t, err)
	assert.Equal(t, fx.revisionID, res.RevisionID)
	assert.Equal(t, 2, res.ContentVersion)
	assert.Equal(t, "<section data-block=\"legal\">Updated Legal</section>", res.ContentHTML)
	assert.True(t, res.LegalContentModified)
	assert.False(t, res.CommercialContentModified)
	assert.False(t, res.UpdatedAt.IsZero())

	// Verify in DB directly
	var dbVersion int
	var dbHTML string
	var dbLegalMod, dbCommMod bool
	var dbLegalBy *uuid.UUID
	err = pool.QueryRow(ctx, `
		SELECT content_version, content_html, legal_content_modified, commercial_content_modified, legal_modified_by
		FROM quotation_revisions WHERE id = $1
	`, fx.revisionID).Scan(&dbVersion, &dbHTML, &dbLegalMod, &dbCommMod, &dbLegalBy)
	require.NoError(t, err)
	assert.Equal(t, 2, dbVersion)
	assert.Equal(t, "<section data-block=\"legal\">Updated Legal</section>", dbHTML)
	assert.True(t, dbLegalMod)
	assert.False(t, dbCommMod)
	require.NotNil(t, dbLegalBy)
	assert.Equal(t, userID, *dbLegalBy)
}

func TestUpdateDraft_StaleVersion_ReturnsErrRevisionConflict(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	// Attempt save with stale version 99 (current is 1)
	_, err := repo.UpdateDraft(ctx, repo.Pool(), fx.companyID, fx.quotationID, repository.UpdateDraftParams{
		RevisionID:      fx.revisionID,
		ExpectedVersion: 99,
		ContentHTML:     "<p>Stale</p>",
	})
	assert.ErrorIs(t, err, repository.ErrRevisionConflict)

	// Verify DB version is still 1 and content not modified
	var dbVersion int
	err = pool.QueryRow(ctx, `SELECT content_version FROM quotation_revisions WHERE id = $1`, fx.revisionID).Scan(&dbVersion)
	require.NoError(t, err)
	assert.Equal(t, 1, dbVersion)
}

func TestUpdateDraft_NonDraftStatus_ReturnsErrRevisionConflict(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET publication_status = 'superseded' WHERE id = $1`, fx.revisionID)
	require.NoError(t, err)

	_, err = repo.UpdateDraft(ctx, repo.Pool(), fx.companyID, fx.quotationID, repository.UpdateDraftParams{
		RevisionID:      fx.revisionID,
		ExpectedVersion: 1,
		ContentHTML:     "<p>Superseded</p>",
	})
	assert.ErrorIs(t, err, repository.ErrRevisionConflict)
}

func TestUpdateDraft_ConcurrentSaves_OnlyOneWins(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	const concurrency = 10
	var wg sync.WaitGroup
	wg.Add(concurrency)

	var successCount int64
	var conflictCount int64

	for i := 0; i < concurrency; i++ {
		workerID := i
		go func() {
			defer wg.Done()
			_, uerr := repo.UpdateDraft(ctx, repo.Pool(), fx.companyID, fx.quotationID, repository.UpdateDraftParams{
				RevisionID:      fx.revisionID,
				ExpectedVersion: 1,
				ContentHTML:     fmt.Sprintf("<p>Worker %d</p>", workerID),
			})
			if uerr == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(uerr, repository.ErrRevisionConflict) {
				atomic.AddInt64(&conflictCount, 1)
			}
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(1), successCount, "Exactly 1 concurrent save must succeed")
	assert.Equal(t, int64(concurrency-1), conflictCount, "All other concurrent saves must receive ErrRevisionConflict")

	// Verify DB version is exactly 2
	var finalVersion int
	err := pool.QueryRow(ctx, `SELECT content_version FROM quotation_revisions WHERE id = $1`, fx.revisionID).Scan(&finalVersion)
	require.NoError(t, err)
	assert.Equal(t, 2, finalVersion)
}
