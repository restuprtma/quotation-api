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

func TestGetBlueprintGenerationConsistentSnapshotRead(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)

	created, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)
	require.NotNil(t, created)

	// Snapshot read under RepeatableRead
	loaded, err := repo.GetBlueprintGeneration(ctx, created.Blueprint.ID, f.CompanyID)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Verify aggregate snapshot consistency
	assert.Equal(t, created.Blueprint.ID, loaded.Blueprint.ID)
	assert.Equal(t, f.CompanyID, loaded.Blueprint.CompanyID)
	assert.Equal(t, created.Revision.ID, loaded.Revision.ID)
	assert.Equal(t, 1, loaded.Revision.RevisionNumber)
	assert.Equal(t, string(domain.BlueprintGenerationPending), loaded.Revision.GenerationStatus)

	// Verify all 7 section runs are loaded in canonical order
	require.Len(t, loaded.SectionRuns, len(domain.BlueprintSectionOrder))
	for i, expectedSecType := range domain.BlueprintSectionOrder {
		run := loaded.SectionRuns[i]
		assert.Equal(t, string(expectedSecType), run.SectionType)
		assert.Equal(t, f.CompanyID, run.CompanyID)
		assert.Equal(t, loaded.Revision.ID, run.BlueprintRevisionID)
		if i == 0 {
			assert.Equal(t, string(domain.BlueprintSectionValidated), run.Status)
		} else {
			assert.Equal(t, string(domain.BlueprintSectionPending), run.Status)
		}
	}

	// Verify soft-deleted blueprint is not found
	_, err = pool.Exec(ctx, `UPDATE blueprints SET deleted_at = NOW() WHERE id = $1`, created.Blueprint.ID)
	require.NoError(t, err)

	_, err = repo.GetBlueprintGeneration(ctx, created.Blueprint.ID, f.CompanyID)
	assert.ErrorIs(t, err, repository.ErrBlueprintNotFound)
}

func TestClaimNextBlueprintSectionTenantIsolation(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Foreign company
	foreignCompany := uuid.New()
	claim, err := repo.ClaimNextBlueprintSection(ctx, foreignCompany, res.Blueprint.ID, now, staleAfter)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintNotFound))
	assert.Nil(t, claim)

	// Unknown Blueprint ID
	unknownBlueprint := uuid.New()
	claim, err = repo.ClaimNextBlueprintSection(ctx, f.CompanyID, unknownBlueprint, now, staleAfter)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintNotFound))
	assert.Nil(t, claim)
}

func TestClaimNextBlueprintSectionUsesCanonicalOrderAndSnapshots(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Claim 1: Should be Journey (context is already validated)
	claim1, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim1)

	assert.Equal(t, res.Blueprint.ID, claim1.BlueprintID)
	assert.Equal(t, res.Revision.ID, claim1.RevisionID)
	assert.Equal(t, f.CompanyID, claim1.CompanyID)
	assert.Equal(t, domain.BlueprintSectionJourney, claim1.SectionType)
	assert.Equal(t, 1, claim1.Attempt)
	assert.Equal(t, 1, claim1.ExpectedContentVersion)
	assert.Equal(t, params.Title, claim1.BlueprintTitle)
	assert.JSONEq(t, string(params.SourceContextSnapshot), string(claim1.SourceContextSnapshot))
	assert.JSONEq(t, string(params.SkillSnapshot), string(claim1.SkillSnapshot))
	assert.JSONEq(t, string(params.ContextContent), string(claim1.ContextContent))
	assert.JSONEq(t, "{}", string(claim1.RevisionContent))

	// Dependencies must contain ONLY context
	require.Len(t, claim1.Dependencies, 1)
	ctxDep, ok := claim1.Dependencies[domain.BlueprintSectionContext]
	require.True(t, ok)
	assert.JSONEq(t, string(params.ContextContent), string(ctxDep))

	// Verify revision generation_status became in_progress in DB
	var revGenStatus string
	err = pool.QueryRow(ctx, `SELECT generation_status FROM blueprint_revisions WHERE id = $1 AND company_id = $2`, res.Revision.ID, f.CompanyID).Scan(&revGenStatus)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BlueprintGenerationInProgress), revGenStatus)

	// Complete Journey via fixture update
	journeyContent := `{"user_journey":[{"step":1,"action":"Book ticket"}]}`
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'completed', content_json = $1::jsonb, updated_at = NOW()
		WHERE blueprint_revision_id = $2 AND section_type = 'journey' AND company_id = $3
	`, journeyContent, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	// Update revision content_version to 2 and content_json
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_revisions
		SET content_version = 2, content_json = '{"journey":{"user_journey":[{"step":1,"action":"Book ticket"}]}}'::jsonb
		WHERE id = $1 AND company_id = $2
	`, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	// Claim 2: Should be Architecture
	claim2, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim2)

	assert.Equal(t, domain.BlueprintSectionArchitecture, claim2.SectionType)
	assert.Equal(t, 1, claim2.Attempt)
	assert.Equal(t, 2, claim2.ExpectedContentVersion)

	// Dependencies must contain context AND journey
	require.Len(t, claim2.Dependencies, 2)
	assert.JSONEq(t, string(params.ContextContent), string(claim2.Dependencies[domain.BlueprintSectionContext]))
	assert.JSONEq(t, journeyContent, string(claim2.Dependencies[domain.BlueprintSectionJourney]))
}

func TestClaimNextBlueprintSectionConcurrentSingleWinner(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	const concurrency = 8
	start := make(chan struct{})
	claims := make([]*repository.BlueprintSectionClaim, concurrency)
	errs := make([]error, concurrency)

	now := time.Now()
	staleAfter := 5 * time.Minute

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		idx := i
		go func() {
			defer wg.Done()
			<-start
			c, e := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
			claims[idx] = c
			errs[idx] = e
		}()
	}
	close(start)
	wg.Wait()

	successes := 0
	busies := 0
	for i := 0; i < concurrency; i++ {
		if errs[i] == nil {
			successes++
			require.NotNil(t, claims[i])
			assert.Equal(t, domain.BlueprintSectionJourney, claims[i].SectionType)
			assert.Equal(t, 1, claims[i].Attempt)
		} else {
			assert.True(t, errors.Is(errs[i], repository.ErrBlueprintClaimBusy) || errors.Is(errs[i], repository.ErrBlueprintNoEligibleSection), "unexpected error: %v", errs[i])
			busies++
		}
	}

	assert.Equal(t, 1, successes, "expected exactly 1 winner")
	assert.Equal(t, concurrency-1, busies, "expected %d busy/no-eligible losers", concurrency-1)

	// Verify database attempt_count is exactly 1
	var dbAttempt int
	var dbStatus string
	err = pool.QueryRow(ctx, `
		SELECT attempt_count, status
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&dbAttempt, &dbStatus)
	require.NoError(t, err)
	assert.Equal(t, 1, dbAttempt)
	assert.Equal(t, "generating", dbStatus)

	// Exactly 1 generating row across all runs
	var generatingCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND status = 'generating' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&generatingCount)
	require.NoError(t, err)
	assert.Equal(t, 1, generatingCount)
}

func TestClaimNextBlueprintSectionDoesNotIncrementLosingAttempt(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// First claim succeeds
	claim1, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim1)
	assert.Equal(t, 1, claim1.Attempt)

	// Second claim attempts while first is generating -> returns ErrBlueprintClaimBusy
	claim2, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintClaimBusy))
	assert.Nil(t, claim2)

	// Verify attempt_count in DB did NOT increment, stays 1
	var dbAttempt int
	err = pool.QueryRow(ctx, `
		SELECT attempt_count
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&dbAttempt)
	require.NoError(t, err)
	assert.Equal(t, 1, dbAttempt)
}

func TestClaimNextBlueprintSectionRecoversStaleGenerating(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Simulate Journey was claimed 6 minutes ago (stale)
	staleStartedAt := now.Add(-6 * time.Minute)
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'generating', attempt_count = 1, started_at = $1, updated_at = $1
		WHERE blueprint_revision_id = $2 AND section_type = 'journey' AND company_id = $3
	`, staleStartedAt, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	// ClaimNextBlueprintSection should detect stale lease, mark failed/interrupted, and return ErrBlueprintRecoveryRequired
	claim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintRecoveryRequired))
	assert.Nil(t, claim)

	// Verify DB: Journey status is failed, safe_error_code is interrupted, attempt stays 1
	var status string
	var safeCode *string
	var attemptCount int
	err = pool.QueryRow(ctx, `
		SELECT status, safe_error_code, attempt_count
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&status, &safeCode, &attemptCount)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BlueprintSectionFailed), status)
	require.NotNil(t, safeCode)
	assert.Equal(t, "interrupted", *safeCode)
	assert.Equal(t, 1, attemptCount)

	// Verify revision generation_status became partial_failed
	var revGenStatus string
	var contentVer int
	err = pool.QueryRow(ctx, `
		SELECT generation_status, content_version
		FROM blueprint_revisions
		WHERE id = $1 AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&revGenStatus, &contentVer)
	require.NoError(t, err)
	assert.Equal(t, string(domain.BlueprintGenerationPartialFailed), revGenStatus)
	assert.Equal(t, 1, contentVer) // version unchanged
}

func TestClaimNextBlueprintSectionRejectsNonStaleGenerating(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Simulate Journey claimed 4 minutes ago (non-stale: 4m < 5m)
	activeStartedAt := now.Add(-4 * time.Minute)
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'generating', attempt_count = 1, started_at = $1, updated_at = $1
		WHERE blueprint_revision_id = $2 AND section_type = 'journey' AND company_id = $3
	`, activeStartedAt, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	// ClaimNext should return ErrBlueprintClaimBusy
	claim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintClaimBusy))
	assert.Nil(t, claim)

	// Verify DB state unchanged: still generating
	var status string
	err = pool.QueryRow(ctx, `
		SELECT status
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "generating", status)
}

func TestClaimFailedBlueprintSectionEnforcesRetryCodeAndAttemptLimit(t *testing.T) {
	pool := getTestPool(t)
	now := time.Now()
	staleAfter := 5 * time.Minute

	allowedCodes := []string{
		"provider_timeout",
		"provider_rate_limited",
		"provider_unavailable",
		"provider_invalid_credential",
		"provider_configuration",
		"invalid_schema",
		"interrupted",
	}

	for _, code := range allowedCodes {
		t.Run("allowed_"+code, func(t *testing.T) {
			f := seedBlueprintSource(t, pool)
			defer cleanupBlueprintFixture(t, pool, f.CompanyID)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			repo := repository.NewBlueprintGenerationRepository(pool)
			res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
			require.NoError(t, err)

			// Put Journey into failed with the allowed code at attempt 1
			_, err = pool.Exec(ctx, `
				UPDATE blueprint_section_runs
				SET status = 'failed', safe_error_code = $1, attempt_count = 1, updated_at = NOW()
				WHERE blueprint_revision_id = $2 AND section_type = 'journey' AND company_id = $3
			`, code, res.Revision.ID, f.CompanyID)
			require.NoError(t, err)

			claim, err := repo.ClaimFailedBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionJourney, now, staleAfter)
			require.NoError(t, err)
			require.NotNil(t, claim)
			assert.Equal(t, domain.BlueprintSectionJourney, claim.SectionType)
			assert.Equal(t, 2, claim.Attempt) // attempt incremented to 2
		})
	}

	disallowedCodes := []string{
		"security_failure",
		"cross_validation_error",
		"unknown",
		"persistence_failure",
	}

	for _, code := range disallowedCodes {
		t.Run("disallowed_"+code, func(t *testing.T) {
			f := seedBlueprintSource(t, pool)
			defer cleanupBlueprintFixture(t, pool, f.CompanyID)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			repo := repository.NewBlueprintGenerationRepository(pool)
			res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
			require.NoError(t, err)

			_, err = pool.Exec(ctx, `
				UPDATE blueprint_section_runs
				SET status = 'failed', safe_error_code = $1, attempt_count = 1, updated_at = NOW()
				WHERE blueprint_revision_id = $2 AND section_type = 'journey' AND company_id = $3
			`, code, res.Revision.ID, f.CompanyID)
			require.NoError(t, err)

			claim, err := repo.ClaimFailedBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionJourney, now, staleAfter)
			require.Error(t, err)
			assert.True(t, errors.Is(err, repository.ErrBlueprintSectionNotRetryable))
			assert.Nil(t, claim)
		})
	}

	t.Run("rejects_attempt_3", func(t *testing.T) {
		f := seedBlueprintSource(t, pool)
		defer cleanupBlueprintFixture(t, pool, f.CompanyID)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		repo := repository.NewBlueprintGenerationRepository(pool)
		res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
		require.NoError(t, err)

		_, err = pool.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'failed', safe_error_code = 'provider_timeout', attempt_count = 3, updated_at = NOW()
			WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
		`, res.Revision.ID, f.CompanyID)
		require.NoError(t, err)

		claim, err := repo.ClaimFailedBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionJourney, now, staleAfter)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintAttemptLimit))
		assert.Nil(t, claim)
	})

	t.Run("rejects_context_section", func(t *testing.T) {
		f := seedBlueprintSource(t, pool)
		defer cleanupBlueprintFixture(t, pool, f.CompanyID)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		repo := repository.NewBlueprintGenerationRepository(pool)
		res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
		require.NoError(t, err)

		claim, err := repo.ClaimFailedBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionContext, now, staleAfter)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSectionNotRetryable))
		assert.Nil(t, claim)
	})

	t.Run("rejects_cross_validation_section", func(t *testing.T) {
		f := seedBlueprintSource(t, pool)
		defer cleanupBlueprintFixture(t, pool, f.CompanyID)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		repo := repository.NewBlueprintGenerationRepository(pool)
		res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
		require.NoError(t, err)

		claim, err := repo.ClaimFailedBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionCrossValidation, now, staleAfter)
		require.Error(t, err)
		assert.True(t, errors.Is(err, repository.ErrBlueprintSectionNotRetryable))
		assert.Nil(t, claim)
	})
}

func TestCompleteBlueprintSectionRequiresActiveAttempt(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	params := validBlueprintCreateParams(f)
	res, err := repo.CreateBlueprintGeneration(ctx, params)
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Claim Journey
	claim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim)

	journeyContent := json.RawMessage(`{"user_journey":[{"step":1,"action":"Login"}]}`)
	validationResult := json.RawMessage(`{"valid":true}`)
	revisionContent := json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]}}`)

	completeParams := repository.CompleteBlueprintSectionParams{
		Claim:                 *claim,
		SectionContent:        journeyContent,
		SectionValidation:     validationResult,
		RevisionContent:       revisionContent,
		CrossValidationResult: nil,
		ProviderName:          "mock-provider",
		Model:                 "mock-model",
		CompletedAt:           now,
		Final:                 false,
	}

	result, err := repo.CompleteBlueprintSection(ctx, completeParams)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, res.Revision.ID, result.RevisionID)
	assert.Equal(t, domain.BlueprintSectionJourney, result.SectionType)
	assert.Equal(t, domain.BlueprintSectionCompleted, result.SectionStatus)
	assert.Equal(t, domain.BlueprintGenerationInProgress, result.GenerationStatus)
	assert.Equal(t, 2, result.ContentVersion)
	assert.Equal(t, 1, result.Attempt)

	// Verify DB state
	var dbStatus string
	var dbAttempt int
	err = pool.QueryRow(ctx, `
		SELECT status, attempt_count
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&dbStatus, &dbAttempt)
	require.NoError(t, err)
	assert.Equal(t, "completed", dbStatus)
	assert.Equal(t, 1, dbAttempt)

	var revVersion int
	var revProvider, revModel *string
	err = pool.QueryRow(ctx, `
		SELECT content_version, ai_provider, ai_model
		FROM blueprint_revisions
		WHERE id = $1 AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&revVersion, &revProvider, &revModel)
	require.NoError(t, err)
	assert.Equal(t, 2, revVersion)
	require.NotNil(t, revProvider)
	assert.Equal(t, "mock-provider", *revProvider)
	require.NotNil(t, revModel)
	assert.Equal(t, "mock-model", *revModel)

	// Replaying completion with already completed claim -> ErrBlueprintClaimConflict
	replayResult, err := repo.CompleteBlueprintSection(ctx, completeParams)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintClaimConflict))
	assert.Nil(t, replayResult)
}

func TestCompleteBlueprintSectionRejectsStaleCompletion(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	claim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim)

	// Simulate stale recovery occurred during execution
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'failed', safe_error_code = 'interrupted', updated_at = NOW()
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	completeParams := repository.CompleteBlueprintSectionParams{
		Claim:             *claim,
		SectionContent:    json.RawMessage(`{"user_journey":[{"step":1,"action":"Login"}]}`),
		SectionValidation: json.RawMessage(`{"valid":true}`),
		RevisionContent:   json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]}}`),
		CompletedAt:       now,
	}

	result, err := repo.CompleteBlueprintSection(ctx, completeParams)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintClaimConflict))
	assert.Nil(t, result)

	// Verify DB: Section run remains failed (not overwritten to completed), version stays 1
	var dbStatus string
	err = pool.QueryRow(ctx, `
		SELECT status
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&dbStatus)
	require.NoError(t, err)
	assert.Equal(t, "failed", dbStatus)

	var contentVer int
	err = pool.QueryRow(ctx, `
		SELECT content_version
		FROM blueprint_revisions
		WHERE id = $1 AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&contentVer)
	require.NoError(t, err)
	assert.Equal(t, 1, contentVer)
}

func TestCompleteBlueprintSectionRejectsStaleRetryCompletion(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Claim 1 (attempt 1)
	claim1, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim1)
	assert.Equal(t, 1, claim1.Attempt)

	// Simulate attempt 1 failed with retryable code
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'failed', safe_error_code = 'provider_timeout', updated_at = NOW()
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	// Claim 2 (retry, attempt 2)
	claim2, err := repo.ClaimFailedBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionJourney, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim2)
	assert.Equal(t, 2, claim2.Attempt)

	// Late completion from attempt 1 arrives
	staleParams := repository.CompleteBlueprintSectionParams{
		Claim:             *claim1, // attempt 1
		SectionContent:    json.RawMessage(`{"user_journey":[{"step":1,"action":"Login"}]}`),
		SectionValidation: json.RawMessage(`{"valid":true}`),
		RevisionContent:   json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]}}`),
		CompletedAt:       now,
	}

	result, err := repo.CompleteBlueprintSection(ctx, staleParams)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintClaimConflict))
	assert.Nil(t, result)

	// Verify DB: Journey stays generating with attempt 2
	var dbStatus string
	var dbAttempt int
	err = pool.QueryRow(ctx, `
		SELECT status, attempt_count
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&dbStatus, &dbAttempt)
	require.NoError(t, err)
	assert.Equal(t, "generating", dbStatus)
	assert.Equal(t, 2, dbAttempt)
}

func TestCompleteBlueprintSectionRejectsContentVersionConflict(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Claim Journey with ExpectedContentVersion = 1
	claim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	require.NotNil(t, claim)
	assert.Equal(t, 1, claim.ExpectedContentVersion)

	// Concurrent change increments revision content_version to 2
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_revisions
		SET content_version = 2, updated_at = NOW()
		WHERE id = $1 AND company_id = $2
	`, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	completeParams := repository.CompleteBlueprintSectionParams{
		Claim:             *claim,
		SectionContent:    json.RawMessage(`{"user_journey":[{"step":1,"action":"Login"}]}`),
		SectionValidation: json.RawMessage(`{"valid":true}`),
		RevisionContent:   json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]}}`),
		CompletedAt:       now,
	}

	result, err := repo.CompleteBlueprintSection(ctx, completeParams)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintContentConflict))
	assert.Nil(t, result)

	// Verify section run did not complete
	var dbStatus string
	err = pool.QueryRow(ctx, `
		SELECT status
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&dbStatus)
	require.NoError(t, err)
	assert.Equal(t, "generating", dbStatus)
}

func TestFailBlueprintSectionIsolatesCompletedUpstream(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Claim and complete Journey
	jClaim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)

	journeyContent := json.RawMessage(`{"user_journey":[{"step":1,"action":"Login"}]}`)
	journeyVal := json.RawMessage(`{"valid":true}`)
	revWithJourney := json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]}}`)

	jRes, err := repo.CompleteBlueprintSection(ctx, repository.CompleteBlueprintSectionParams{
		Claim:             *jClaim,
		SectionContent:    journeyContent,
		SectionValidation: journeyVal,
		RevisionContent:   revWithJourney,
		CompletedAt:       now,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, jRes.ContentVersion)

	// Claim Architecture (attempt 1, ExpectedContentVersion = 2)
	archClaim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	assert.Equal(t, domain.BlueprintSectionArchitecture, archClaim.SectionType)
	assert.Equal(t, 2, archClaim.ExpectedContentVersion)

	// Fail Architecture
	failParams := repository.FailBlueprintSectionParams{
		Claim:         *archClaim,
		SafeErrorCode: "provider_timeout",
		FailedAt:      now,
		Terminal:      false,
	}

	failRes, err := repo.FailBlueprintSection(ctx, failParams)
	require.NoError(t, err)
	require.NotNil(t, failRes)
	assert.Equal(t, domain.BlueprintSectionArchitecture, failRes.SectionType)
	assert.Equal(t, domain.BlueprintSectionFailed, failRes.SectionStatus)
	assert.Equal(t, domain.BlueprintGenerationPartialFailed, failRes.GenerationStatus)
	assert.Equal(t, 2, failRes.ContentVersion) // Version unchanged!
	assert.Equal(t, 1, failRes.Attempt)

	// Verify Journey section run is intact and byte-equivalent
	var jRunContent json.RawMessage
	var jRunStatus string
	err = pool.QueryRow(ctx, `
		SELECT content_json, status
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'journey' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&jRunContent, &jRunStatus)
	require.NoError(t, err)
	assert.Equal(t, "completed", jRunStatus)
	assert.JSONEq(t, string(journeyContent), string(jRunContent))

	// Verify revision content and version are intact
	var revContent json.RawMessage
	var revVer int
	var revStatus string
	err = pool.QueryRow(ctx, `
		SELECT content_json, content_version, generation_status
		FROM blueprint_revisions
		WHERE id = $1 AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&revContent, &revVer, &revStatus)
	require.NoError(t, err)
	assert.Equal(t, 2, revVer)
	assert.Equal(t, string(domain.BlueprintGenerationPartialFailed), revStatus)
	assert.JSONEq(t, string(revWithJourney), string(revContent))

	// Verify Architecture section run has safe_error_code
	var archSafeCode *string
	var archStatus string
	err = pool.QueryRow(ctx, `
		SELECT safe_error_code, status
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'architecture' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&archSafeCode, &archStatus)
	require.NoError(t, err)
	assert.Equal(t, "failed", archStatus)
	require.NotNil(t, archSafeCode)
	assert.Equal(t, "provider_timeout", *archSafeCode)
}

func TestCompleteBlueprintSectionManuallyRejectsConcurrentGenerating(t *testing.T) {
	pool := getTestPool(t)
	f := seedBlueprintSource(t, pool)
	defer cleanupBlueprintFixture(t, pool, f.CompanyID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := repository.NewBlueprintGenerationRepository(pool)
	res, err := repo.CreateBlueprintGeneration(ctx, validBlueprintCreateParams(f))
	require.NoError(t, err)

	now := time.Now()
	staleAfter := 5 * time.Minute

	// Complete Journey
	jClaim, err := repo.ClaimNextBlueprintSection(ctx, f.CompanyID, res.Blueprint.ID, now, staleAfter)
	require.NoError(t, err)
	_, err = repo.CompleteBlueprintSection(ctx, repository.CompleteBlueprintSectionParams{
		Claim:             *jClaim,
		SectionContent:    json.RawMessage(`{"user_journey":[{"step":1,"action":"Login"}]}`),
		SectionValidation: json.RawMessage(`{"valid":true}`),
		RevisionContent:   json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]}}`),
		CompletedAt:       now,
	})
	require.NoError(t, err)

	// Load snapshot for Architecture (which is pending, ExpectedContentVersion = 2)
	snapshot, err := repo.LoadBlueprintManualSnapshot(ctx, f.CompanyID, res.Blueprint.ID, domain.BlueprintSectionArchitecture)
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, domain.BlueprintSectionArchitecture, snapshot.SectionType)
	assert.Equal(t, domain.BlueprintSectionPending, snapshot.SectionStatus)
	assert.Equal(t, 2, snapshot.ExpectedContentVersion)

	// Simulate concurrent executor claims Architecture -> status becomes generating
	_, err = pool.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'generating', attempt_count = 1, started_at = NOW(), updated_at = NOW()
		WHERE blueprint_revision_id = $1 AND section_type = 'architecture' AND company_id = $2
	`, res.Revision.ID, f.CompanyID)
	require.NoError(t, err)

	// Manual completion should be rejected with ErrBlueprintManualConflict
	manResult, err := repo.CompleteBlueprintSectionManually(ctx, repository.CompleteBlueprintManualParams{
		Snapshot:            *snapshot,
		SectionContent:      json.RawMessage(`{"components":[{"name":"ManualComp"}]}`),
		SectionValidation:   json.RawMessage(`{"valid":true}`),
		RevisionContent:     json.RawMessage(`{"journey":{"user_journey":[{"step":1,"action":"Login"}]},"architecture":{"components":[{"name":"ManualComp"}]}}`),
		InvalidatedSections: []domain.BlueprintSectionType{domain.BlueprintSectionStateMachine, domain.BlueprintSectionWorkflow, domain.BlueprintSectionWBS, domain.BlueprintSectionCrossValidation},
		CompletedAt:         now,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrBlueprintManualConflict))
	assert.Nil(t, manResult)

	// Architecture remains generating
	var archStatus string
	err = pool.QueryRow(ctx, `
		SELECT status FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND section_type = 'architecture' AND company_id = $2
	`, res.Revision.ID, f.CompanyID).Scan(&archStatus)
	require.NoError(t, err)
	assert.Equal(t, "generating", archStatus)
}
