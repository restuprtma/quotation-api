package repository_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestBlueprintLifecycle_UnitContracts(t *testing.T) {
	// Assert sentinel error definitions and messages
	assert.Equal(t, "active draft blueprint revision not found", repository.ErrBlueprintDraftNotFound.Error())
	assert.Equal(t, "blueprint master not found", repository.ErrBlueprintMasterNotFound.Error())
	assert.Equal(t, "blueprint revision not found", repository.ErrBlueprintRevisionNotFound.Error())

	// Compile-time interface checks
	var _ repository.BlueprintLifecycleRepository = (*repository.BlueprintGenerationRepository)(nil)
	var _ repository.BlueprintDraftRepository = (*repository.BlueprintGenerationRepository)(nil)

	// Test constructor and alias
	repo := repository.NewBlueprintLifecycleRepository(nil)
	require.NotNil(t, repo)
	assert.Nil(t, repo.Pool())

	_, err := repo.Begin(context.Background())
	assert.Error(t, err)
}

func getBlueprintLifecycleTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("skipping blueprint lifecycle database test: TEST_DATABASE_URL not set")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping blueprint lifecycle database test: unable to connect: %v", err)
		return nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping blueprint lifecycle database test: database ping failed: %v", err)
		return nil
	}

	t.Cleanup(pool.Close)
	return pool
}

func TestBlueprintLifecycle_PublishTransaction(t *testing.T) {
	pool := getBlueprintLifecycleTestPool(t)
	if pool == nil {
		return
	}

	ctx := context.Background()
	repo := repository.NewBlueprintLifecycleRepository(pool)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	companyID := uuid.New()
	clientID := uuid.New()
	quotationID := uuid.New()
	qRevID := uuid.New()
	blueprintID := uuid.New()
	quotNumber := "QUO-" + uuid.New().String()[:8]

	// Seed client organization, quotation, quotation revision, and blueprint
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
		VALUES ($1, $2, 'Test BP Lifecycle Client', 'test bp lifecycle client')
	`, clientID, companyID)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
		VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'published', 1)
	`, quotationID, companyID, clientID, quotNumber)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (id, company_id, quotation_id, revision_number, publication_status, generation_status, budget_maximum)
		VALUES ($1, $2, $3, 1, 'published', 'completed', 100000)
	`, qRevID, companyID, quotationID)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO blueprints (id, company_id, quotation_id, title, status, current_revision)
		VALUES ($1, $2, $3, 'Lifecycle Test Blueprint', 'draft', 1)
	`, blueprintID, companyID, quotationID)
	require.NoError(t, err)

	t.Run("LockBlueprintMaster", func(t *testing.T) {
		err := repo.LockBlueprintMaster(ctx, tx, blueprintID.String())
		require.NoError(t, err)

		err = repo.LockBlueprintMaster(ctx, tx, uuid.New().String())
		assert.ErrorIs(t, err, repository.ErrBlueprintMasterNotFound)
	})

	t.Run("GetMaxRevisionNumber_Empty", func(t *testing.T) {
		maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, blueprintID.String())
		require.NoError(t, err)
		assert.Equal(t, 0, maxRev)
	})

	rev1ID := uuid.New()
	t.Run("CreateRevisionTx_And_GetDraftRevision", func(t *testing.T) {
		rev := &domain.BlueprintRevision{
			ID:                        rev1ID,
			CompanyID:                 companyID,
			BlueprintID:               blueprintID,
			RevisionNumber:            1,
			SourceQuotationRevisionID: &qRevID,
			PublicationStatus:         "draft",
			CompletenessStatus:        "partial",
			GenerationStatus:          "completed",
		}
		err := repo.CreateRevisionTx(ctx, tx, rev)
		require.NoError(t, err)
		assert.False(t, rev.CreatedAt.IsZero())

		draftRev, err := repo.GetDraftRevision(ctx, tx, blueprintID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, draftRev)
		assert.Equal(t, rev1ID, draftRev.ID)
		assert.Equal(t, 1, draftRev.RevisionNumber)
		assert.Equal(t, "draft", draftRev.PublicationStatus)
		assert.Equal(t, "partial", draftRev.CompletenessStatus)

		maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, blueprintID.String())
		require.NoError(t, err)
		assert.Equal(t, 1, maxRev)
	})

	t.Run("PublishDraftBlueprint", func(t *testing.T) {
		// Mark superseded when no previous published revision exists (succeeds with 0 rows)
		err := repo.MarkPublishedRevisionSuperseded(ctx, tx, blueprintID.String())
		require.NoError(t, err)

		// Publish rev1 as complete
		err = repo.PublishDraftBlueprint(ctx, tx, blueprintID.String(), rev1ID.String(), true)
		require.NoError(t, err)

		// Active published revision verification
		pubRev, err := repo.GetActivePublishedRevision(ctx, tx, blueprintID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, pubRev)
		assert.Equal(t, rev1ID, pubRev.ID)
		assert.Equal(t, "published", pubRev.PublicationStatus)
		assert.Equal(t, "complete", pubRev.CompletenessStatus)
		assert.NotNil(t, pubRev.PublishedAt)

		// Draft revision should no longer exist
		_, err = repo.GetDraftRevision(ctx, tx, blueprintID.String(), companyID.String())
		assert.ErrorIs(t, err, repository.ErrBlueprintDraftNotFound)

		// Check blueprint master inside transaction
		var activePubID *uuid.UUID
		var bpStatus string
		err = tx.QueryRow(ctx, "SELECT active_published_revision_id, status FROM blueprints WHERE id = $1", blueprintID).Scan(&activePubID, &bpStatus)
		require.NoError(t, err)
		require.NotNil(t, activePubID)
		assert.Equal(t, rev1ID, *activePubID)
		assert.Equal(t, "published", bpStatus)
	})

	t.Run("CreateSecondRevision_Supersede_And_Publish", func(t *testing.T) {
		rev2ID := uuid.New()
		rev2 := &domain.BlueprintRevision{
			ID:                        rev2ID,
			CompanyID:                 companyID,
			BlueprintID:               blueprintID,
			RevisionNumber:            2,
			SourceQuotationRevisionID: &qRevID,
			PublicationStatus:         "draft",
			CompletenessStatus:        "partial",
			GenerationStatus:          "completed",
		}
		err := repo.CreateRevisionTx(ctx, tx, rev2)
		require.NoError(t, err)

		maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, blueprintID.String())
		require.NoError(t, err)
		assert.Equal(t, 2, maxRev)

		err = repo.UpdateBlueprintCurrentRevision(ctx, tx, blueprintID.String(), 2)
		require.NoError(t, err)

		// Supersede previous published revision (rev1)
		err = repo.MarkPublishedRevisionSuperseded(ctx, tx, blueprintID.String())
		require.NoError(t, err)

		// Publish rev2 as partial
		err = repo.PublishDraftBlueprint(ctx, tx, blueprintID.String(), rev2ID.String(), false)
		require.NoError(t, err)

		// Active published revision is now rev2 with completeness = partial
		pubRev, err := repo.GetActivePublishedRevision(ctx, tx, blueprintID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, pubRev)
		assert.Equal(t, rev2ID, pubRev.ID)
		assert.Equal(t, 2, pubRev.RevisionNumber)
		assert.Equal(t, "published", pubRev.PublicationStatus)
		assert.Equal(t, "partial", pubRev.CompletenessStatus)

		// Rev1 should now be superseded
		var rev1Status string
		err = tx.QueryRow(ctx, "SELECT publication_status FROM blueprint_revisions WHERE id = $1", rev1ID).Scan(&rev1Status)
		require.NoError(t, err)
		assert.Equal(t, "superseded", rev1Status)

		// Master active_published_revision_id points to rev2
		var activePubID *uuid.UUID
		var currentRev int
		err = tx.QueryRow(ctx, "SELECT active_published_revision_id, current_revision FROM blueprints WHERE id = $1", blueprintID).Scan(&activePubID, &currentRev)
		require.NoError(t, err)
		require.NotNil(t, activePubID)
		assert.Equal(t, rev2ID, *activePubID)
		assert.Equal(t, 2, currentRev)
	})

	t.Run("PublishDraftBlueprint_NotFoundCases", func(t *testing.T) {
		// Non-existent revision
		err := repo.PublishDraftBlueprint(ctx, tx, blueprintID.String(), uuid.New().String(), true)
		assert.ErrorIs(t, err, repository.ErrBlueprintRevisionNotFound)

		// Non-existent blueprint
		err = repo.UpdateBlueprintCurrentRevision(ctx, tx, uuid.New().String(), 99)
		assert.ErrorIs(t, err, repository.ErrBlueprintMasterNotFound)

		// Non-existent active published revision for random blueprint
		_, err = repo.GetActivePublishedRevision(ctx, tx, uuid.New().String(), companyID.String())
		assert.ErrorIs(t, err, repository.ErrBlueprintRevisionNotFound)
	})
}

func TestBlueprintLifecycle_DeleteDraft(t *testing.T) {
	pool := getBlueprintLifecycleTestPool(t)
	if pool == nil {
		return
	}

	ctx := context.Background()
	repo := repository.NewBlueprintLifecycleRepository(pool)

	companyA := uuid.New()
	companyB := uuid.New()
	clientA := uuid.New()
	clientB := uuid.New()
	quotA := uuid.New()
	quotB := uuid.New()
	qRevA := uuid.New()
	qRevB := uuid.New()
	bpA := uuid.New()
	bpB := uuid.New()
	bpRevA := uuid.New()
	bpRevB := uuid.New()

	// Cleanup at end of test
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM blueprint_revisions WHERE blueprint_id IN ($1, $2)", bpA, bpB)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM blueprints WHERE id IN ($1, $2)", bpA, bpB)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM quotation_revisions WHERE quotation_id IN ($1, $2)", quotA, quotB)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM quotations WHERE id IN ($1, $2)", quotA, quotB)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM quotation_client_organizations WHERE id IN ($1, $2)", clientA, clientB)
	})

	// Seed Tenant A
	_, err := pool.Exec(ctx, `
		INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
		VALUES ($1, $2, 'Client A', 'client a')
	`, clientA, companyA)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
		VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'published', 1)
	`, quotA, companyA, clientA, "QUO-"+uuid.New().String()[:8])
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_revisions (id, company_id, quotation_id, revision_number, publication_status, generation_status, budget_maximum)
		VALUES ($1, $2, $3, 1, 'published', 'completed', 100000)
	`, qRevA, companyA, quotA)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO blueprints (id, company_id, quotation_id, title, status, current_revision)
		VALUES ($1, $2, $3, 'BP A', 'draft', 1)
	`, bpA, companyA, quotA)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO blueprint_revisions (id, company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot, publication_status)
		VALUES ($1, $2, $3, 1, $4, '{}'::jsonb, 'draft')
	`, bpRevA, companyA, bpA, qRevA)
	require.NoError(t, err)

	// Seed Tenant B
	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
		VALUES ($1, $2, 'Client B', 'client b')
	`, clientB, companyB)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
		VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'published', 1)
	`, quotB, companyB, clientB, "QUO-"+uuid.New().String()[:8])
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO quotation_revisions (id, company_id, quotation_id, revision_number, publication_status, generation_status, budget_maximum)
		VALUES ($1, $2, $3, 1, 'published', 'completed', 100000)
	`, qRevB, companyB, quotB)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		INSERT INTO blueprints (id, company_id, quotation_id, title, status, current_revision)
		VALUES ($1, $2, $3, 'BP B', 'published', 1)
	`, bpB, companyB, quotB)
	require.NoError(t, err)

	// BP B has a published revision (not draft)
	_, err = pool.Exec(ctx, `
		INSERT INTO blueprint_revisions (id, company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot, publication_status)
		VALUES ($1, $2, $3, 1, $4, '{}'::jsonb, 'published')
	`, bpRevB, companyB, bpB, qRevB)
	require.NoError(t, err)

	t.Run("TenantIsolation_CannotDeleteOtherTenantDraft", func(t *testing.T) {
		// Company B attempts to delete Company A's draft
		err := repo.DeleteDraft(ctx, companyB.String(), bpA.String())
		assert.ErrorIs(t, err, repository.ErrBlueprintDraftNotFound)

		// Verify Company A's draft still exists in DB
		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM blueprint_revisions WHERE id = $1", bpRevA).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "draft revision for tenant A must remain untouched")
	})

	t.Run("PublicationStatusIsolation_CannotDeletePublishedRevision", func(t *testing.T) {
		// Attempting to delete draft on BP B which only has a published revision
		err := repo.DeleteDraft(ctx, companyB.String(), bpB.String())
		assert.ErrorIs(t, err, repository.ErrBlueprintDraftNotFound)

		// Verify Company B's published revision is NOT deleted
		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM blueprint_revisions WHERE id = $1", bpRevB).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "published revision must not be deleted by DeleteDraft")
	})

	t.Run("SuccessfulDelete_TargetsOnlyDraft", func(t *testing.T) {
		// Company A deletes its own draft revision
		err := repo.DeleteDraft(ctx, companyA.String(), bpA.String())
		require.NoError(t, err)

		// Verify row is gone
		var count int
		err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM blueprint_revisions WHERE id = $1", bpRevA).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "draft revision must be deleted")

		// Subsequent call returns ErrBlueprintDraftNotFound
		err = repo.DeleteDraft(ctx, companyA.String(), bpA.String())
		assert.ErrorIs(t, err, repository.ErrBlueprintDraftNotFound)
	})
}
