package repository_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func findProjectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find project root containing go.mod")
		}
		dir = parent
	}
}

func TestMigrationFiles_LifecycleConstraints(t *testing.T) {
	root := findProjectRoot(t)

	migrationPairs := []struct {
		module string
		upFile string
		dnFile string
	}{
		{
			module: "quotation",
			upFile: filepath.Join(root, "internal", "database", "migrations", "quotation", "20261005130000_add_lifecycle_constraints.up.sql"),
			dnFile: filepath.Join(root, "internal", "database", "migrations", "quotation", "20261005130000_add_lifecycle_constraints.down.sql"),
		},
		{
			module: "core",
			upFile: filepath.Join(root, "internal", "database", "migrations", "core", "20261005130000_add_lifecycle_constraints.up.sql"),
			dnFile: filepath.Join(root, "internal", "database", "migrations", "core", "20261005130000_add_lifecycle_constraints.down.sql"),
		},
	}

	for _, tc := range migrationPairs {
		t.Run(tc.module+"_up_file", func(t *testing.T) {
			content, err := os.ReadFile(tc.upFile)
			require.NoError(t, err, "up migration file must exist and be readable: %s", tc.upFile)
			sql := string(content)

			// Validate quotation revisions single draft index
			assert.Contains(t, sql, "idx_quotation_revisions_single_draft")
			assert.Contains(t, sql, "quotation_revisions(quotation_id)")
			assert.Contains(t, sql, "WHERE publication_status = 'draft'")

			// Validate blueprint revisions single draft index
			assert.Contains(t, sql, "idx_blueprint_revisions_single_draft")
			assert.Contains(t, sql, "blueprint_revisions(blueprint_id)")
			assert.Contains(t, sql, "WHERE publication_status = 'draft'")
		})

		t.Run(tc.module+"_down_file", func(t *testing.T) {
			content, err := os.ReadFile(tc.dnFile)
			require.NoError(t, err, "down migration file must exist and be readable: %s", tc.dnFile)
			sql := string(content)

			assert.Contains(t, sql, "DROP INDEX IF EXISTS idx_quotation_revisions_single_draft;")
			assert.Contains(t, sql, "DROP INDEX IF EXISTS idx_blueprint_revisions_single_draft;")
		})
	}
}

func getLifecycleTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("skipping lifecycle database test: TEST_DATABASE_URL not set")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skipping lifecycle database test: unable to connect: %v", err)
		return nil
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping lifecycle database test: database ping failed: %v", err)
		return nil
	}

	t.Cleanup(pool.Close)
	return pool
}

func TestLifecycleConstraints_Database(t *testing.T) {
	pool := getLifecycleTestPool(t)
	if pool == nil {
		return
	}

	ctx := context.Background()

	// Ensure the partial unique indexes exist in the test database
	_, err := pool.Exec(ctx, `
		CREATE UNIQUE INDEX IF NOT EXISTS idx_quotation_revisions_single_draft 
		ON quotation_revisions(quotation_id) 
		WHERE publication_status = 'draft';

		CREATE UNIQUE INDEX IF NOT EXISTS idx_blueprint_revisions_single_draft 
		ON blueprint_revisions(blueprint_id) 
		WHERE publication_status = 'draft';
	`)
	require.NoError(t, err, "failed to ensure indexes exist")

	t.Run("VerifyIndexExistence", func(t *testing.T) {
		indexes := []struct {
			table string
			index string
		}{
			{"quotation_revisions", "idx_quotation_revisions_single_draft"},
			{"blueprint_revisions", "idx_blueprint_revisions_single_draft"},
		}

		for _, idx := range indexes {
			var exists bool
			err := pool.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1 FROM pg_indexes 
					WHERE schemaname = 'public' 
					  AND tablename = $1 
					  AND indexname = $2
				)
			`, idx.table, idx.index).Scan(&exists)
			require.NoError(t, err)
			assert.True(t, exists, "expected index %s on %s", idx.index, idx.table)
		}
	})

	t.Run("QuotationRevisions_SingleDraftConstraintEnforcement", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		companyID := "00000000-0000-0000-0000-000000000001"
		var quotID string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES ($1, gen_random_uuid(), 'QUO-LIFECYCLE-TEST', 'custom_software', 'Lifecycle Test')
			RETURNING id
		`, companyID).Scan(&quotID)
		require.NoError(t, err)

		// First draft revision should succeed
		var rev1ID string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, publication_status, budget_maximum)
			VALUES ($1, $2, 1, 'draft', 100000)
			RETURNING id
		`, companyID, quotID).Scan(&rev1ID)
		require.NoError(t, err, "first draft revision must succeed")

		// Second draft revision for same quotation must fail with unique constraint violation
		var rev2ID string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, publication_status, budget_maximum)
			VALUES ($1, $2, 2, 'draft', 200000)
			RETURNING id
		`, companyID, quotID).Scan(&rev2ID)
		require.Error(t, err, "second draft revision for same quotation must fail")

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			assert.Equal(t, "23505", pgErr.Code, "should be unique_violation error code")
		}
	})

	t.Run("BlueprintRevisions_SingleDraftConstraintEnforcement", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)

		companyID := "00000000-0000-0000-0000-000000000001"
		var quotID string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotations (company_id, client_id, quotation_number, solution_type, subject)
			VALUES ($1, gen_random_uuid(), 'QUO-BP-LIFECYCLE-TEST', 'custom_software', 'BP Lifecycle Test')
			RETURNING id
		`, companyID).Scan(&quotID)
		require.NoError(t, err)

		var qRevID string
		err = tx.QueryRow(ctx, `
			INSERT INTO quotation_revisions (company_id, quotation_id, revision_number, publication_status, budget_maximum)
			VALUES ($1, $2, 1, 'draft', 100000)
			RETURNING id
		`, companyID, quotID).Scan(&qRevID)
		require.NoError(t, err)

		var bpID string
		err = tx.QueryRow(ctx, `
			INSERT INTO blueprints (company_id, quotation_id, title)
			VALUES ($1, $2, 'BP Lifecycle Test')
			RETURNING id
		`, companyID, quotID).Scan(&bpID)
		require.NoError(t, err)

		// First blueprint draft revision should succeed
		var bpRev1ID string
		err = tx.QueryRow(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot, publication_status)
			VALUES ($1, $2, 1, $3, '{}'::jsonb, 'draft')
			RETURNING id
		`, companyID, bpID, qRevID).Scan(&bpRev1ID)
		require.NoError(t, err, "first blueprint draft revision must succeed")

		// Second blueprint draft revision for same blueprint must fail with unique constraint violation
		var bpRev2ID string
		err = tx.QueryRow(ctx, `
			INSERT INTO blueprint_revisions (company_id, blueprint_id, revision_number, source_quotation_revision_id, source_context_snapshot, publication_status)
			VALUES ($1, $2, 2, $3, '{}'::jsonb, 'draft')
			RETURNING id
		`, companyID, bpID, qRevID).Scan(&bpRev2ID)
		require.Error(t, err, "second blueprint draft revision for same blueprint must fail")

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			assert.Equal(t, "23505", pgErr.Code, "should be unique_violation error code")
		}
	})
}

func TestQuotationLifecycle_UnitContracts(t *testing.T) {
	assert.Equal(t, "draft", repository.PublicationStatusDraft)
	assert.Equal(t, "published", repository.PublicationStatusPublished)
	assert.Equal(t, "superseded", repository.PublicationStatusSuperseded)

	repo := repository.NewRepository(nil)
	assert.NotNil(t, repo)
	assert.Nil(t, repo.Pool())
}

func TestQuotationLifecycle_PublishTransaction(t *testing.T) {
	pool := getLifecycleTestPool(t)
	if pool == nil {
		return
	}

	ctx := context.Background()
	repo := repository.NewRepository(pool)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	companyID := uuid.New()
	clientID := uuid.New()
	quotationID := uuid.New()
	quotNumber := "QUO-" + uuid.New().String()[:8]

	// Seed client organization and quotation
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_client_organizations (id, company_id, name, normalized_name)
		VALUES ($1, $2, 'Test Lifecycle Client', 'test lifecycle client')
	`, clientID, companyID)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (id, company_id, client_id, quotation_number, solution_type, commercial_scheme, status, current_revision)
		VALUES ($1, $2, $3, $4, 'custom_software', 'dedicated_squad', 'draft', 1)
	`, quotationID, companyID, clientID, quotNumber)
	require.NoError(t, err)

	t.Run("LockQuotationMaster", func(t *testing.T) {
		err := repo.LockQuotationMaster(ctx, tx, quotationID.String())
		require.NoError(t, err)

		err = repo.LockQuotationMaster(ctx, tx, uuid.New().String())
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})

	t.Run("GetMaxRevisionNumber_Empty", func(t *testing.T) {
		maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, quotationID.String())
		require.NoError(t, err)
		assert.Equal(t, 0, maxRev)
	})

	rev1ID := uuid.New()
	t.Run("CreateRevisionTx_And_GetDraftRevision", func(t *testing.T) {
		rev := &domain.QuotationRevision{
			ID:                rev1ID,
			CompanyID:         companyID,
			QuotationID:       quotationID,
			RevisionNumber:    1,
			PublicationStatus: repository.PublicationStatusDraft,
			GenerationStatus:  repository.GenerationStatusCompleted,
		}
		err := repo.CreateRevisionTx(ctx, tx, rev)
		require.NoError(t, err)
		assert.False(t, rev.CreatedAt.IsZero())

		draftRev, err := repo.GetDraftRevision(ctx, tx, quotationID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, draftRev)
		assert.Equal(t, rev1ID, draftRev.ID)
		assert.Equal(t, 1, draftRev.RevisionNumber)
		assert.Equal(t, repository.PublicationStatusDraft, draftRev.PublicationStatus)

		maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, quotationID.String())
		require.NoError(t, err)
		assert.Equal(t, 1, maxRev)
	})

	t.Run("PublishDraftQuotation", func(t *testing.T) {
		// First publish when no previous published revision exists
		err := repo.MarkPublishedRevisionSuperseded(ctx, tx, quotationID.String())
		require.NoError(t, err) // should succeed even if 0 rows updated

		err = repo.PublishDraftQuotation(ctx, tx, quotationID.String(), rev1ID.String())
		require.NoError(t, err)

		// Verification: published revision exists
		pubRev, err := repo.GetActivePublishedRevision(ctx, tx, quotationID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, pubRev)
		assert.Equal(t, rev1ID, pubRev.ID)
		assert.Equal(t, repository.PublicationStatusPublished, pubRev.PublicationStatus)
		assert.NotNil(t, pubRev.PublishedAt)

		// Draft revision should no longer exist
		_, err = repo.GetDraftRevision(ctx, tx, quotationID.String(), companyID.String())
		assert.ErrorIs(t, err, repository.ErrRevisionNotFound)

		// Check quotation master inside transaction
		var activePubID *uuid.UUID
		var quotStatus string
		err = tx.QueryRow(ctx, "SELECT active_published_revision_id, status FROM quotations WHERE id = $1", quotationID).Scan(&activePubID, &quotStatus)
		require.NoError(t, err)
		require.NotNil(t, activePubID)
		assert.Equal(t, rev1ID, *activePubID)
		assert.Equal(t, "published", quotStatus)
	})

	t.Run("CreateSecondRevision_Supersede_And_Publish", func(t *testing.T) {
		rev2ID := uuid.New()
		rev2 := &domain.QuotationRevision{
			ID:                rev2ID,
			CompanyID:         companyID,
			QuotationID:       quotationID,
			RevisionNumber:    2,
			PublicationStatus: repository.PublicationStatusDraft,
			GenerationStatus:  repository.GenerationStatusCompleted,
		}
		err := repo.CreateRevisionTx(ctx, tx, rev2)
		require.NoError(t, err)

		maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, quotationID.String())
		require.NoError(t, err)
		assert.Equal(t, 2, maxRev)

		err = repo.UpdateQuotationCurrentRevision(ctx, tx, quotationID.String(), 2)
		require.NoError(t, err)

		// Before publishing rev2, supersede rev1
		err = repo.MarkPublishedRevisionSuperseded(ctx, tx, quotationID.String())
		require.NoError(t, err)

		// Now publish rev2
		err = repo.PublishDraftQuotation(ctx, tx, quotationID.String(), rev2ID.String())
		require.NoError(t, err)

		// Verification: active published revision is now rev2
		pubRev, err := repo.GetActivePublishedRevision(ctx, tx, quotationID.String(), companyID.String())
		require.NoError(t, err)
		require.NotNil(t, pubRev)
		assert.Equal(t, rev2ID, pubRev.ID)
		assert.Equal(t, 2, pubRev.RevisionNumber)
		assert.Equal(t, repository.PublicationStatusPublished, pubRev.PublicationStatus)

		// rev1 should now be superseded in database
		var rev1Status string
		err = tx.QueryRow(ctx, "SELECT publication_status FROM quotation_revisions WHERE id = $1", rev1ID).Scan(&rev1Status)
		require.NoError(t, err)
		assert.Equal(t, repository.PublicationStatusSuperseded, rev1Status)

		// Quotation master active_published_revision_id should point to rev2
		var activePubID *uuid.UUID
		var currentRev int
		err = tx.QueryRow(ctx, "SELECT active_published_revision_id, current_revision FROM quotations WHERE id = $1", quotationID).Scan(&activePubID, &currentRev)
		require.NoError(t, err)
		require.NotNil(t, activePubID)
		assert.Equal(t, rev2ID, *activePubID)
		assert.Equal(t, 2, currentRev)
	})

	t.Run("PublishDraftQuotation_NotFoundCases", func(t *testing.T) {
		// Non-existent revision
		err := repo.PublishDraftQuotation(ctx, tx, quotationID.String(), uuid.New().String())
		assert.ErrorIs(t, err, repository.ErrRevisionNotFound)

		// Non-existent quotation
		err = repo.UpdateQuotationCurrentRevision(ctx, tx, uuid.New().String(), 99)
		assert.ErrorIs(t, err, repository.ErrNotFound)
	})
}

