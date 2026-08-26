//go:build integration

package repository_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type regenFixture struct {
	companyID   uuid.UUID
	quotationID uuid.UUID
	revisionID  uuid.UUID
	number      string
}

func seedRegenDraft(t *testing.T, pool *pgxpool.Pool) regenFixture {
	t.Helper()
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	companyID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	params := validDraftParams(companyID)
	res, err := repo.CreateDraftTransaction(ctx, params)
	require.NoError(t, err)
	return regenFixture{
		companyID:   companyID,
		quotationID: res.QuotationID,
		revisionID:  res.RevisionID,
		number:      params.QuotationNumber,
	}
}

func quotaState(t *testing.T, pool *pgxpool.Pool, revisionID uuid.UUID) (count int, last *time.Time) {
	t.Helper()
	require.NoError(t, pool.QueryRow(context.Background(), `
		SELECT regenerate_count, last_regenerate_at
		FROM quotation_revisions WHERE id = $1
	`, revisionID).Scan(&count, &last))
	return count, last
}

func TestLoadDraftRevision_ReturnsDraftWithInputAndNumber(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	draft, err := repo.LoadDraftRevisionForRegeneration(ctx, fx.companyID, fx.quotationID)
	require.NoError(t, err)
	assert.Equal(t, fx.revisionID, draft.RevisionID)
	assert.Equal(t, fx.quotationID, draft.QuotationID)
	assert.Equal(t, "draft", draft.PublicationStatus)
	assert.NotEmpty(t, draft.GenerationInput)
	assert.Contains(t, string(draft.GenerationInput), "mandatory_scope")
}

func TestLoadDraftRevision_WrongCompany_IsNotFound(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := repo.LoadDraftRevisionForRegeneration(ctx, uuid.New(), fx.quotationID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound),
		"tenant lain harus melihat 404-equivalent, bukan conflict")
}

func TestLoadDraftRevision_UnknownQuotation_IsNotFound(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := repo.LoadDraftRevisionForRegeneration(ctx, fx.companyID, uuid.New())
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound))
}

func TestLoadDraftRevision_PublishedRevision_IsConflict(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := pool.Exec(ctx, `
		UPDATE quotation_revisions SET publication_status = 'published' WHERE id = $1
	`, fx.revisionID)
	require.NoError(t, err)

	_, err = repo.LoadDraftRevisionForRegeneration(ctx, fx.companyID, fx.quotationID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrRevisionConflict))
}

func TestConsumeQuota_IncrementsAtomically(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	for want := 1; want <= 3; want++ {
		got, err := repo.ConsumeRegenerateQuota(ctx, repo.Pool(), fx.companyID, fx.revisionID)
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}

	count, last := quotaState(t, pool, fx.revisionID)
	assert.Equal(t, 3, count)
	require.NotNil(t, last)
	assert.WithinDuration(t, time.Now(), *last, time.Minute)
}

func TestConsumeQuota_LimitReached_NoRowAndStateUntouched(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	stamp := time.Now().Add(-10 * time.Minute)
	_, err := pool.Exec(ctx, `
		UPDATE quotation_revisions SET regenerate_count = $2, last_regenerate_at = $1 WHERE id = $3
	`, stamp, repository.MaxRegeneratesPerHour, fx.revisionID)
	require.NoError(t, err)

	_, err = repo.ConsumeRegenerateQuota(ctx, repo.Pool(), fx.companyID, fx.revisionID)
	require.Error(t, err, "regenerate ke-6 dalam satu jam harus ditolak")
	assert.True(t, errors.Is(err, pgx.ErrNoRows))

	count, last := quotaState(t, pool, fx.revisionID)
	assert.Equal(t, repository.MaxRegeneratesPerHour, count,
		"penolakan tidak boleh mengubah counter")
	require.NotNil(t, last)
	assert.WithinDuration(t, stamp, *last, time.Second,
		"penolakan tidak boleh menggeser window (mencegah limit yang tak pernah reset)")
}

func TestConsumeQuota_WindowExpired_ResetsToOne(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	stale := time.Now().Add(-2 * time.Hour)
	_, err := pool.Exec(ctx, `
		UPDATE quotation_revisions SET regenerate_count = 9, last_regenerate_at = $1 WHERE id = $2
	`, stale, fx.revisionID)
	require.NoError(t, err)

	got, err := repo.ConsumeRegenerateQuota(ctx, repo.Pool(), fx.companyID, fx.revisionID)
	require.NoError(t, err)
	assert.Equal(t, 1, got, "window kedaluwarsa harus me-reset counter ke 1")
}

func TestUpdateRegeneratedResult_IncrementsVersionReplacesHTML(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	hash := "deadbeef"
	err := repo.UpdateRegeneratedResult(ctx, repo.Pool(), fx.companyID, fx.revisionID, repository.UpdateGenerationResultParams{
		ContentHTML:           "<p>regenerated</p>",
		AllocationSnapshot:    []byte(`{"total_investment":63000000}`),
		RateCardSnapshot:      []byte(`{"currency":"IDR"}`),
		LegalTemplateSnapshot: []byte(`{"code":"QUOTATION_TERMS"}`),
		SkillSnapshot:         []byte(`{"command":"quotation_s4"}`),
		AiProvider:            "gemini",
		AiModel:               "gemini-test",
		ContentHash:           &hash,
		SafetyEnvelopeVersion: "quotation_safety_v1",
	})
	require.NoError(t, err)

	var (
		html    string
		version int
		status  string
		prov    string
	)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT content_html, content_version, generation_status, COALESCE(ai_provider,'')
		FROM quotation_revisions WHERE id = $1
	`, fx.revisionID).Scan(&html, &version, &status, &prov))
	assert.Equal(t, "<p>regenerated</p>", html)
	assert.Equal(t, 2, version, "regenerate wajib increment content_version")
	assert.Equal(t, "completed", status)
	assert.Equal(t, "gemini", prov)
}

// TestUpdateRegeneratedResult_PublishedMidFlight_IsConflict menutup TOCTOU
// window: jika revision ter-publish selagi pipeline AI berjalan, persist
// final WAJIB gagal dengan conflict — bukan menimpa konten published.
func TestUpdateRegeneratedResult_PublishedMidFlight_IsConflict(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := pool.Exec(ctx,
		`UPDATE quotation_revisions SET publication_status = 'published' WHERE id = $1`,
		fx.revisionID)
	require.NoError(t, err)

	hash := "deadbeef"
	err = repo.UpdateRegeneratedResult(ctx, repo.Pool(), fx.companyID, fx.revisionID, repository.UpdateGenerationResultParams{
		ContentHTML:           "<p>overwrite ilegal</p>",
		AllocationSnapshot:    []byte(`{}`),
		RateCardSnapshot:      []byte(`{}`),
		LegalTemplateSnapshot: []byte(`{}`),
		SkillSnapshot:         []byte(`{}`),
		AiProvider:            "gemini",
		AiModel:               "gemini-test",
		ContentHash:           &hash,
		SafetyEnvelopeVersion: "quotation_safety_v1",
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrRevisionConflict))

	var html string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT content_html FROM quotation_revisions WHERE id = $1`, fx.revisionID).Scan(&html))
	assert.Equal(t, "", html, "konten published tidak boleh tertimpa hasil regenerate")
}

func TestGetGenerationState_SoftDeletedQuotation_IsNotFound(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	_, err := pool.Exec(ctx,
		`UPDATE quotations SET deleted_at = NOW() WHERE id = $1`, fx.quotationID)
	require.NoError(t, err)

	_, err = repo.GetGenerationState(ctx, fx.companyID, fx.quotationID)
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound),
		"quotation soft-deleted tidak boleh masih terbaca via generation-status")

	_, err = repo.GetGenerationDetail(ctx, fx.companyID, fx.quotationID)
	assert.True(t, errors.Is(err, repository.ErrRevisionNotFound))
}

// revisionCount menghitung jumlah baris revision milik satu quotation.
func revisionCount(t *testing.T, pool *pgxpool.Pool, quotationID uuid.UUID) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM quotation_revisions WHERE quotation_id = $1`,
		quotationID).Scan(&n))
	return n
}

func TestConsumeQuota_ConcurrentCalls_CounterExact(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	fx := seedRegenDraft(t, pool)

	const goroutines = 16
	var wg sync.WaitGroup
	var succeeded, limited atomic.Int64
	results := make([]error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := repo.ConsumeRegenerateQuota(ctx, repo.Pool(), fx.companyID, fx.revisionID)
			results[idx] = err
		}(i)
	}
	wg.Wait()

	for _, err := range results {
		switch {
		case err == nil:
			succeeded.Add(1)
		case errors.Is(err, pgx.ErrNoRows):
			limited.Add(1)
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}

	assert.Equal(t, int64(repository.MaxRegeneratesPerHour), succeeded.Load(),
		"tepat MaxRegeneratesPerHour call boleh lolos")
	assert.Equal(t, int64(goroutines-repository.MaxRegeneratesPerHour), limited.Load())
	count, _ := quotaState(t, pool, fx.revisionID)
	assert.Equal(t, repository.MaxRegeneratesPerHour, count,
		"counter akhir harus persis jumlah gate yang lolos (tidak skip, tidak double)")
	t.Logf("concurrent quota: %d ok / %d limited", succeeded.Load(), limited.Load())
}
