//go:build integration

package repository_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type blueprintFixture struct {
	CompanyID   uuid.UUID
	QuotationID uuid.UUID
	RevisionID  uuid.UUID
}

func seedBlueprintSource(t *testing.T, pool *pgxpool.Pool) blueprintFixture {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	companyID := uuid.New()
	quotationID := uuid.New()
	revisionID := uuid.New()
	quotationNumber := "BP-SRC-" + uuid.NewString()[:8]

	_, err = tx.Exec(ctx, `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject,
			solution_type, commercial_scheme, status, current_revision
		) VALUES (
			$1, $2, gen_random_uuid(), $3, 'Queue Management Blueprint Source',
			'Queue Management System', 'kiosk_queue_system', 'draft', 1
		)
	`, quotationID, companyID, quotationNumber)
	require.NoError(t, err)

	_, err = tx.Exec(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number,
			publication_status, generation_status, generation_input,
			content_html, content_version, created_by
		) VALUES (
			$1, $2, $3, 1,
			'draft', 'completed', '{"kiosk_queue_system":{"system_name":"Queue Test"}}'::jsonb,
			'<section data-section-code="scope"><h2>Scope</h2><p>Queue booking and calling</p></section>',
			1, $4
		)
	`, revisionID, companyID, quotationID, (*uuid.UUID)(nil))
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.NoError(t, err)

	return blueprintFixture{
		CompanyID:   companyID,
		QuotationID: quotationID,
		RevisionID:  revisionID,
	}
}

func validBlueprintCreateParams(f blueprintFixture) repository.CreateBlueprintGenerationParams {
	return repository.CreateBlueprintGenerationParams{
		CompanyID:              f.CompanyID,
		QuotationID:            f.QuotationID,
		SourceRevisionID:       f.RevisionID,
		Title:                  "Queue Management Blueprint",
		IdempotencyKey:         uuid.New(),
		IdempotencyPayloadHash: strings.Repeat("a", 64),
		SourceContextSnapshot:  json.RawMessage(`{"project_name":"Queue Management System","industry":"Service"}`),
		ContextContent:         json.RawMessage(`{"summary":"Queue Management Blueprint Context","scope":"High volume queuing"}`),
		SkillSnapshot:          json.RawMessage(`{"skill_id":"skill-default","name":"Blueprint Generator","version":"1.0"}`),
		CreatedBy:              nil,
	}
}

func cleanupBlueprintFixture(t *testing.T, pool *pgxpool.Pool, companyID uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _ = pool.Exec(ctx, `UPDATE blueprints SET active_published_revision_id = NULL WHERE company_id = $1`, companyID)
	_, _ = pool.Exec(ctx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM blueprint_section_runs WHERE company_id = $1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM blueprint_revisions WHERE company_id = $1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM blueprints WHERE company_id = $1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE company_id = $1`, companyID)
	_, _ = pool.Exec(ctx, `DELETE FROM quotations WHERE company_id = $1`, companyID)
}

func TestCreateBlueprintGenerationCreatesAggregate(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)

	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.False(t, res.IdempotentReplay)

	// Master Blueprint
	assert.NotEqual(t, uuid.Nil, res.Blueprint.ID)
	assert.Equal(t, f.CompanyID, res.Blueprint.CompanyID)
	assert.Equal(t, f.QuotationID, res.Blueprint.QuotationID)
	assert.Equal(t, params.Title, res.Blueprint.Title)
	assert.Equal(t, "draft", res.Blueprint.Status)
	assert.Equal(t, 1, res.Blueprint.CurrentRevision)
	assert.Nil(t, res.Blueprint.ActivePublishedRevisionID)
	assert.False(t, res.Blueprint.CreatedAt.IsZero())
	assert.False(t, res.Blueprint.UpdatedAt.IsZero())

	// Blueprint Revision
	assert.NotEqual(t, uuid.Nil, res.Revision.ID)
	assert.Equal(t, f.CompanyID, res.Revision.CompanyID)
	assert.Equal(t, res.Blueprint.ID, res.Revision.BlueprintID)
	assert.Equal(t, 1, res.Revision.RevisionNumber)
	require.NotNil(t, res.Revision.IdempotencyKey)
	assert.Equal(t, params.IdempotencyKey, *res.Revision.IdempotencyKey)
	require.NotNil(t, res.Revision.IdempotencyPayloadHash)
	assert.Equal(t, params.IdempotencyPayloadHash, *res.Revision.IdempotencyPayloadHash)
	require.NotNil(t, res.Revision.SourceQuotationRevisionID)
	assert.Equal(t, f.RevisionID, *res.Revision.SourceQuotationRevisionID)
	assert.Equal(t, "draft", res.Revision.PublicationStatus)
	assert.Equal(t, "partial", res.Revision.CompletenessStatus)
	assert.Equal(t, string(domain.BlueprintGenerationPending), res.Revision.GenerationStatus)
	assert.Equal(t, 1, res.Revision.ContentVersion)
	assert.JSONEq(t, "{}", string(res.Revision.ContentJSON))
	assert.JSONEq(t, "{}", string(res.Revision.ValidationResult))
	assert.JSONEq(t, "{}", string(res.Revision.CrossValidationResult))
	assert.JSONEq(t, string(params.SourceContextSnapshot), string(res.Revision.SourceContextSnapshot))
	assert.JSONEq(t, string(params.SkillSnapshot), string(res.Revision.SkillSnapshot))
	assert.False(t, res.Revision.CreatedAt.IsZero())
	assert.False(t, res.Revision.UpdatedAt.IsZero())

	// Section Runs
	require.Len(t, res.SectionRuns, 7)
	for i, expType := range domain.BlueprintSectionOrder {
		run := res.SectionRuns[i]
		assert.NotEqual(t, uuid.Nil, run.ID)
		assert.Equal(t, f.CompanyID, run.CompanyID)
		assert.Equal(t, res.Revision.ID, run.BlueprintRevisionID)
		assert.Equal(t, string(expType), run.SectionType)
		if i == 0 {
			assert.Equal(t, string(domain.BlueprintSectionValidated), run.Status)
			assert.JSONEq(t, string(params.ContextContent), string(run.ContentJSON))
		} else {
			assert.Equal(t, string(domain.BlueprintSectionPending), run.Status)
			assert.True(t, len(run.ContentJSON) == 0 || string(run.ContentJSON) == "null")
		}
		assert.JSONEq(t, "{}", string(run.ValidationResult))
		assert.False(t, run.UpdatedAt.IsZero())
	}
}

func TestCreateBlueprintGenerationRejectsForeignSource(t *testing.T) {
	pool := getTestPool(t)
	fA := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, fA.CompanyID)

	fB := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, fB.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)

	// Company A attempts to use Company B's QuotationID
	params1 := validBlueprintCreateParams(fA)
	params1.QuotationID = fB.QuotationID
	_, err := repo.CreateBlueprintGeneration(ctx, params1)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))

	// Company A attempts to use Company B's SourceRevisionID
	params2 := validBlueprintCreateParams(fA)
	params2.SourceRevisionID = fB.RevisionID
	_, err = repo.CreateBlueprintGeneration(ctx, params2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintSourceNotFound))
}

func TestCreateBlueprintGenerationRejectsSourceThatBecameIneligible(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(ctx context.Context, pool *pgxpool.Pool, f blueprintFixture)
	}{
		{
			name: "generation_status != completed",
			mutate: func(ctx context.Context, pool *pgxpool.Pool, f blueprintFixture) {
				_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET generation_status = 'pending' WHERE id = $1 AND company_id = $2`, f.RevisionID, f.CompanyID)
				require.NoError(t, err)
			},
		},
		{
			name: "blank content_html",
			mutate: func(ctx context.Context, pool *pgxpool.Pool, f blueprintFixture) {
				_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET content_html = '   ' WHERE id = $1 AND company_id = $2`, f.RevisionID, f.CompanyID)
				require.NoError(t, err)
			},
		},
		{
			name: "publication_status outside draft/published/superseded",
			mutate: func(ctx context.Context, pool *pgxpool.Pool, f blueprintFixture) {
				_, err := pool.Exec(ctx, `UPDATE quotation_revisions SET publication_status = 'archived' WHERE id = $1 AND company_id = $2`, f.RevisionID, f.CompanyID)
				require.NoError(t, err)
			},
		},
	}

	pool := getTestPool(t)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := seedBlueprintSource(t, pool)
			defer cleanupBlueprintFixture(t, pool, f.CompanyID)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			tc.mutate(ctx, pool, f)

			repo := repository.NewBlueprintGenerationRepository(pool)
			params := validBlueprintCreateParams(f)

			res, err := repo.CreateBlueprintGeneration(ctx, params)
			require.Error(t, err)
			assert.Nil(t, res)
			assert.True(t, errors.Is(err, repository.ErrBlueprintSourceIneligible))

			var bpCount, revCount, runCount int
			err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprints WHERE company_id = $1 AND quotation_id = $2`, f.CompanyID, f.QuotationID).Scan(&bpCount)
			require.NoError(t, err)
			assert.Equal(t, 0, bpCount)

			err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprint_revisions WHERE company_id = $1`, f.CompanyID).Scan(&revCount)
			require.NoError(t, err)
			assert.Equal(t, 0, revCount)

			err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprint_section_runs WHERE company_id = $1`, f.CompanyID).Scan(&runCount)
			require.NoError(t, err)
			assert.Equal(t, 0, runCount)
		})
	}
}

func TestCreateBlueprintGenerationSequentialReplay(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)

	res1, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, res1)
	assert.False(t, res1.IdempotentReplay)

	// Same key and payload hash -> idempotent replay
	res2, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.True(t, res2.IdempotentReplay)

	assert.Equal(t, res1.Blueprint.ID, res2.Blueprint.ID)
	assert.Equal(t, res1.Revision.ID, res2.Revision.ID)
	assert.Len(t, res2.SectionRuns, 7)
}

func TestCreateBlueprintGenerationPayloadMismatch(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params1 := validBlueprintCreateParams(f)

	res1, err := repo.CreateBlueprintGeneration(ctx, params1)
	require.NoError(t, err)
	require.NotNil(t, res1)

	// Same key, different hash
	params2 := validBlueprintCreateParams(f)
	params2.IdempotencyKey = params1.IdempotencyKey
	params2.IdempotencyPayloadHash = strings.Repeat("b", 64)

	_, err = repo.CreateBlueprintGeneration(ctx, params2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintIdempotencyPayloadMismatch))
}

func TestCreateBlueprintGenerationRejectsSecondBlueprint(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params1 := validBlueprintCreateParams(f)

	res1, err := repo.CreateBlueprintGeneration(ctx, params1)
	require.NoError(t, err)
	require.NotNil(t, res1)

	// Same quotation, different idempotency key
	params2 := validBlueprintCreateParams(f)
	params2.IdempotencyKey = uuid.New()
	params2.IdempotencyPayloadHash = strings.Repeat("c", 64)

	_, err = repo.CreateBlueprintGeneration(ctx, params2)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintAlreadyExists))
}

func TestCreateBlueprintGenerationConcurrentReplay(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)

	const concurrency = 8
	start := make(chan struct{})
	results := make([]*repository.CreateBlueprintGenerationResult, concurrency)
	errs := make([]error, concurrency)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		idx := i
		go func() {
			defer wg.Done()
			<-start
			res, err := repo.CreateBlueprintGeneration(ctx, params)
			results[idx] = res
			errs[idx] = err
		}()
	}
	close(start)
	wg.Wait()

	// Assert zero errors
	for i, err := range errs {
		require.NoError(t, err, "concurrency error at index %d", i)
	}

	// Assert exactly one unique Blueprint ID and Revision ID
	firstBlueprintID := results[0].Blueprint.ID
	firstRevisionID := results[0].Revision.ID
	replays := 0
	creations := 0

	for i, res := range results {
		require.NotNil(t, res, "result nil at index %d", i)
		assert.Equal(t, firstBlueprintID, res.Blueprint.ID)
		assert.Equal(t, firstRevisionID, res.Revision.ID)
		if res.IdempotentReplay {
			replays++
		} else {
			creations++
		}
	}

	assert.Equal(t, 1, creations, "expected exactly 1 initial creation")
	assert.Equal(t, concurrency-1, replays, "expected exactly %d idempotent replays", concurrency-1)

	// Verify database counts 1/1/7
	var bpCount int
	err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprints WHERE company_id = $1 AND quotation_id = $2`, f.CompanyID, f.QuotationID).Scan(&bpCount)
	require.NoError(t, err)
	assert.Equal(t, 1, bpCount)

	var revCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprint_revisions WHERE company_id = $1 AND blueprint_id = $2`, f.CompanyID, firstBlueprintID).Scan(&revCount)
	require.NoError(t, err)
	assert.Equal(t, 1, revCount)

	var runCount int
	err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM blueprint_section_runs WHERE company_id = $1 AND blueprint_revision_id = $2`, f.CompanyID, firstRevisionID).Scan(&runCount)
	require.NoError(t, err)
	assert.Equal(t, 7, runCount)
}

func TestGetBlueprintGenerationTenantIsolation(t *testing.T) {
	pool := getTestPool(t)
	fA := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, fA.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	paramsA := validBlueprintCreateParams(fA)

	resA, err := repo.CreateBlueprintGeneration(ctx, paramsA)
	require.NoError(t, err)
	require.NotNil(t, resA)

	// Owner company loads blueprint
	loaded, err := repo.GetBlueprintGeneration(ctx, resA.Blueprint.ID, fA.CompanyID)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, resA.Blueprint.ID, loaded.Blueprint.ID)
	assert.Equal(t, resA.Revision.ID, loaded.Revision.ID)
	assert.Len(t, loaded.SectionRuns, 7)
	assert.False(t, loaded.IdempotentReplay)

	// Foreign company loads blueprint -> not found
	foreignCompany := uuid.New()
	_, err = repo.GetBlueprintGeneration(ctx, resA.Blueprint.ID, foreignCompany)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintNotFound))
}
