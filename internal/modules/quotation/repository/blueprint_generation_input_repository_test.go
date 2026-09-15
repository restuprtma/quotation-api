//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type sourceFixture struct {
	CompanyA               uuid.UUID
	CompanyB               uuid.UUID
	QuotationA1            uuid.UUID
	QuotationA2            uuid.UUID
	QuotationB             uuid.UUID
	QuotationDeleted       uuid.UUID
	RevDraftID             uuid.UUID
	RevPublishedID         uuid.UUID
	RevSupersededID        uuid.UUID
	RevIneligibleStatusID  uuid.UUID
	RevIncompleteID        uuid.UUID
	RevNullContentID       uuid.UUID
	RevWhitespaceContentID uuid.UUID
	RevA2ID                uuid.UUID
	RevDeletedID           uuid.UUID
	RevBID                 uuid.UUID
	SubjectA1              string
	SolutionTypeA1         string
	DraftContentHTML       string
	PublishedContentHTML   string
	SupersededContentHTML  string
}

func cleanupSourceFixture(t *testing.T, pool *pgxpool.Pool, companyA, companyB uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, cid := range []uuid.UUID{companyA, companyB} {
		if cid == uuid.Nil {
			continue
		}
		_, _ = pool.Exec(ctx, `UPDATE blueprints SET active_published_revision_id = NULL WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM blueprint_section_runs WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM blueprint_revisions WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM blueprints WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE company_id = $1`, cid)
		_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE company_id = $1`, cid)
	}
}

func seedSourceFixture(t *testing.T, pool *pgxpool.Pool) sourceFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	f := sourceFixture{
		CompanyA:               uuid.New(),
		CompanyB:               uuid.New(),
		QuotationA1:            uuid.New(),
		QuotationA2:            uuid.New(),
		QuotationB:             uuid.New(),
		QuotationDeleted:       uuid.New(),
		RevDraftID:             uuid.New(),
		RevPublishedID:         uuid.New(),
		RevSupersededID:        uuid.New(),
		RevIneligibleStatusID:  uuid.New(),
		RevIncompleteID:        uuid.New(),
		RevNullContentID:       uuid.New(),
		RevWhitespaceContentID: uuid.New(),
		RevA2ID:                uuid.New(),
		RevDeletedID:           uuid.New(),
		RevBID:                 uuid.New(),
		SubjectA1:              "ERP System Implementation",
		SolutionTypeA1:         "Enterprise Resource Planning",
		DraftContentHTML:       "<section><h2>Draft Scope</h2><p>Draft revision content</p></section>",
		PublishedContentHTML:   "<section><h2>Published Scope</h2><p>Published revision content</p></section>",
		SupersededContentHTML:  "<section><h2>Superseded Scope</h2><p>Superseded revision content</p></section>",
	}

	// Ensure column allows NULL for test coverage of legacy/unmigrated or empty states
	_, _ = pool.Exec(ctx, `ALTER TABLE quotation_revisions ALTER COLUMN content_html DROP NOT NULL`)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	// Quotation A1
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES (
			$1, $2, gen_random_uuid(), $3, $4,
			$5, 'custom_development', 'draft', 1
		)
	`, f.QuotationA1, f.CompanyA, "QT-A1-"+uuid.NewString()[:8], f.SubjectA1, f.SolutionTypeA1)
	require.NoError(t, err)

	// Quotation A2 (another quotation in same company)
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES (
			$1, $2, gen_random_uuid(), $3, 'Quotation A2 Subject',
			'Mobile App', 'custom_development', 'draft', 1
		)
	`, f.QuotationA2, f.CompanyA, "QT-A2-"+uuid.NewString()[:8])
	require.NoError(t, err)

	// Quotation Deleted (soft-deleted quotation in Company A)
	deletedAt := time.Now().Add(-1 * time.Hour)
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision, deleted_at
		) VALUES (
			$1, $2, gen_random_uuid(), $3, 'Deleted Quotation Subject',
			'Web Portal', 'custom_development', 'draft', 1, $4
		)
	`, f.QuotationDeleted, f.CompanyA, "QT-DEL-"+uuid.NewString()[:8], deletedAt)
	require.NoError(t, err)

	// Quotation B (foreign company)
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES (
			$1, $2, gen_random_uuid(), $3, 'Quotation B Subject',
			'Cloud Infrastructure', 'custom_development', 'draft', 1
		)
	`, f.QuotationB, f.CompanyB, "QT-B-"+uuid.NewString()[:8])
	require.NoError(t, err)

	// Revisions for Quotation A1:
	// 1. Valid superseded
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'superseded', 'completed', '{}'::jsonb, $4, 1)
	`, f.RevSupersededID, f.CompanyA, f.QuotationA1, f.SupersededContentHTML)
	require.NoError(t, err)

	// 2. Valid published
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 2, 'published', 'completed', '{}'::jsonb, $4, 1)
	`, f.RevPublishedID, f.CompanyA, f.QuotationA1, f.PublishedContentHTML)
	require.NoError(t, err)

	// 3. Valid draft
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 3, 'draft', 'completed', '{}'::jsonb, $4, 1)
	`, f.RevDraftID, f.CompanyA, f.QuotationA1, f.DraftContentHTML)
	require.NoError(t, err)

	// Revisions on additional quotations to satisfy partial unique indexes (uq_revisions_one_draft_per_quotation):
	// Quotation for Ineligible publication status
	quotIneligible := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES ($1, $2, gen_random_uuid(), $3, 'Ineligible Status Quotation', 'Solution', 'custom_development', 'draft', 1)
	`, quotIneligible, f.CompanyA, "QT-INEL-"+uuid.NewString()[:8])
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'archived', 'completed', '{}'::jsonb, '<p>Archived</p>', 1)
	`, f.RevIneligibleStatusID, f.CompanyA, quotIneligible)
	require.NoError(t, err)

	// Quotation for Incomplete generation
	quotIncomplete := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES ($1, $2, gen_random_uuid(), $3, 'Incomplete Gen Quotation', 'Solution', 'custom_development', 'draft', 1)
	`, quotIncomplete, f.CompanyA, "QT-INC-"+uuid.NewString()[:8])
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'draft', 'pending', '{}'::jsonb, '<p>Pending</p>', 1)
	`, f.RevIncompleteID, f.CompanyA, quotIncomplete)
	require.NoError(t, err)

	// Quotation for Whitespace content
	quotWhitespace := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES ($1, $2, gen_random_uuid(), $3, 'Whitespace Content Quotation', 'Solution', 'custom_development', 'draft', 1)
	`, quotWhitespace, f.CompanyA, "QT-WS-"+uuid.NewString()[:8])
	require.NoError(t, err)

	whitespaceContent := "   \t\r\n   "
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'draft', 'completed', '{}'::jsonb, $4, 1)
	`, f.RevWhitespaceContentID, f.CompanyA, quotWhitespace, whitespaceContent)
	require.NoError(t, err)

	// Quotation for NULL content_html
	quotNull := uuid.New()
	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES ($1, $2, gen_random_uuid(), $3, 'Null Content Quotation', 'Solution', 'custom_development', 'draft', 1)
	`, quotNull, f.CompanyA, "QT-NULL-"+uuid.NewString()[:8])
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'draft', 'completed', '{}'::jsonb, NULL, 1)
	`, f.RevNullContentID, f.CompanyA, quotNull)
	require.NoError(t, err)

	// Quotation A2 revision
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'draft', 'completed', '{}'::jsonb, '<p>A2 content</p>', 1)
	`, f.RevA2ID, f.CompanyA, f.QuotationA2)
	require.NoError(t, err)

	// Quotation Deleted revision
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'draft', 'completed', '{}'::jsonb, '<p>Deleted content</p>', 1)
	`, f.RevDeletedID, f.CompanyA, f.QuotationDeleted)
	require.NoError(t, err)

	// Quotation B revision
	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version
		) VALUES ($1, $2, $3, 1, 'draft', 'completed', '{}'::jsonb, '<p>B content</p>', 1)
	`, f.RevBID, f.CompanyB, f.QuotationB)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	return f
}

func TestLoadEligibleBlueprintSource(t *testing.T) {
	pool := getTestPool(t)
	f := seedSourceFixture(t, pool)
	defer cleanupSourceFixture(t, pool, f.CompanyA, f.CompanyB)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationInputRepository(pool)

	t.Run("valid draft", func(t *testing.T) {
		record, err := repo.LoadEligibleSource(ctx, f.CompanyA, f.QuotationA1, f.RevDraftID)
		require.NoError(t, err)
		assert.Equal(t, f.CompanyA, record.CompanyID)
		assert.Equal(t, f.QuotationA1, record.QuotationID)
		assert.Equal(t, f.RevDraftID, record.RevisionID)
		assert.Equal(t, f.SubjectA1, record.Subject)
		assert.Equal(t, f.SolutionTypeA1, record.SolutionType)
		assert.Equal(t, f.DraftContentHTML, record.ContentHTML)
	})

	t.Run("valid published", func(t *testing.T) {
		record, err := repo.LoadEligibleSource(ctx, f.CompanyA, f.QuotationA1, f.RevPublishedID)
		require.NoError(t, err)
		assert.Equal(t, f.CompanyA, record.CompanyID)
		assert.Equal(t, f.QuotationA1, record.QuotationID)
		assert.Equal(t, f.RevPublishedID, record.RevisionID)
		assert.Equal(t, f.SubjectA1, record.Subject)
		assert.Equal(t, f.SolutionTypeA1, record.SolutionType)
		assert.Equal(t, f.PublishedContentHTML, record.ContentHTML)
	})

	t.Run("valid superseded", func(t *testing.T) {
		record, err := repo.LoadEligibleSource(ctx, f.CompanyA, f.QuotationA1, f.RevSupersededID)
		require.NoError(t, err)
		assert.Equal(t, f.CompanyA, record.CompanyID)
		assert.Equal(t, f.QuotationA1, record.QuotationID)
		assert.Equal(t, f.RevSupersededID, record.RevisionID)
		assert.Equal(t, f.SubjectA1, record.Subject)
		assert.Equal(t, f.SolutionTypeA1, record.SolutionType)
		assert.Equal(t, f.SupersededContentHTML, record.ContentHTML)
	})

	t.Run("missing revision", func(t *testing.T) {
		missingRevID := uuid.New()
		_, err := repo.LoadEligibleSource(ctx, f.CompanyA, f.QuotationA1, missingRevID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))
	})

	t.Run("foreign company", func(t *testing.T) {
		// Company B attempts to load Company A's quotation & revision
		_, err := repo.LoadEligibleSource(ctx, f.CompanyB, f.QuotationA1, f.RevDraftID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))

		// Random non-existent company
		_, err = repo.LoadEligibleSource(ctx, uuid.New(), f.QuotationA1, f.RevDraftID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))
	})

	t.Run("revision belongs to another quotation in same company", func(t *testing.T) {
		// RevA2 belongs to QuotationA2, not QuotationA1
		_, err := repo.LoadEligibleSource(ctx, f.CompanyA, f.QuotationA1, f.RevA2ID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))
	})

	t.Run("ineligible publication status", func(t *testing.T) {
		// Publication status is 'archived'
		var quotID uuid.UUID
		err := pool.QueryRow(ctx, `SELECT quotation_id FROM quotation_revisions WHERE id = $1`, f.RevIneligibleStatusID).Scan(&quotID)
		require.NoError(t, err)

		_, err = repo.LoadEligibleSource(ctx, f.CompanyA, quotID, f.RevIneligibleStatusID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceIneligible))
	})

	t.Run("generation not completed", func(t *testing.T) {
		// Generation status is 'pending'
		var quotID uuid.UUID
		err := pool.QueryRow(ctx, `SELECT quotation_id FROM quotation_revisions WHERE id = $1`, f.RevIncompleteID).Scan(&quotID)
		require.NoError(t, err)

		_, err = repo.LoadEligibleSource(ctx, f.CompanyA, quotID, f.RevIncompleteID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceIneligible))
	})

	t.Run("NULL content_html", func(t *testing.T) {
		var quotID uuid.UUID
		err := pool.QueryRow(ctx, `SELECT quotation_id FROM quotation_revisions WHERE id = $1`, f.RevNullContentID).Scan(&quotID)
		require.NoError(t, err)

		_, err = repo.LoadEligibleSource(ctx, f.CompanyA, quotID, f.RevNullContentID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceIneligible))
	})

	t.Run("whitespace-only content_html", func(t *testing.T) {
		var quotID uuid.UUID
		err := pool.QueryRow(ctx, `SELECT quotation_id FROM quotation_revisions WHERE id = $1`, f.RevWhitespaceContentID).Scan(&quotID)
		require.NoError(t, err)

		_, err = repo.LoadEligibleSource(ctx, f.CompanyA, quotID, f.RevWhitespaceContentID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceIneligible))
	})

	t.Run("soft-deleted quotation", func(t *testing.T) {
		_, err := repo.LoadEligibleSource(ctx, f.CompanyA, f.QuotationDeleted, f.RevDeletedID)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))
	})
}
