//go:build integration

package repository_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

// cleanupCompanies removes every quotation aggregate created for the given
// companies so integration runs stay repeatable. Revisions are deleted first
// to respect fk_quotations_active_published_revision.
func cleanupCompanies(t *testing.T, pool *pgxpool.Pool, companyIDs ...uuid.UUID) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, id := range companyIDs {
			pool.Exec(ctx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, id)
			pool.Exec(ctx, `DELETE FROM quotation_revisions WHERE company_id = $1`, id)
			pool.Exec(ctx, `DELETE FROM quotations WHERE company_id = $1`, id)
		}
	})
}

func validDraftParams(companyID uuid.UUID) repository.CreateDraftParams {
	budget := int64(150_000_000)
	key := uuid.New()
	clientID := uuid.New()
	return repository.CreateDraftParams{
		CompanyID:             companyID,
		ClientID:              &clientID,
		QuotationNumber:       "QUO-GEN-" + key.String()[:8],
		Subject:               "Customer Portal",
		SolutionType:          "custom_software",
		CommercialScheme:      "dedicated_squad",
		IdempotencyKey:        key.String(),
		GenerationInput:       []byte(`{"summary":"portal","mandatory_scope":["auth"]}`),
		BudgetMaximum:         &budget,
		RateCardSnapshot:      []byte(`{"rate_card_id":"11111111-1111-1111-1111-111111111111"}`),
		LegalTemplateSnapshot: []byte(`{"code":"legal-v1","version":1}`),
	}
}

func TestGenerationPersistenceSchema(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	expectedColumns := map[string]string{
		"idempotency_key":         "uuid",
		"generation_input":        "jsonb",
		"budget_maximum":          "bigint",
		"allocation_snapshot":     "jsonb",
		"calculation_snapshot":    "jsonb",
		"module_snapshot":         "jsonb",
		"timeline_snapshot":       "jsonb",
		"rate_card_snapshot":      "jsonb",
		"legal_template_snapshot": "jsonb",
		"content_html":            "text",
		"content_version":         "integer",
		"content_hash":            "character varying",
		"publication_status":      "character varying",
		"generation_status":       "character varying",
		"skill_snapshot":          "text",
		"safety_envelope_version": "character varying",
		"ai_provider":             "character varying",
		"ai_model":                "character varying",
		"generated_at":            "timestamp with time zone",
	}

	for col, wantType := range expectedColumns {
		t.Run(fmt.Sprintf("column_%s", col), func(t *testing.T) {
			exists, dataType, err := columnExists(ctx, pool, "quotation_revisions", col)
			if err != nil {
				t.Fatalf("query error checking column %s: %v", col, err)
			}
			if !exists {
				t.Fatalf("expected column 'quotation_revisions.%s' to exist", col)
			}
			if dataType != wantType {
				t.Errorf("column 'quotation_revisions.%s' has type %q, expected %q", col, dataType, wantType)
			}
		})
	}

	t.Run("uq_revisions_idempotency_key", func(t *testing.T) {
		exists, err := indexExists(ctx, pool, "quotation_revisions", "uq_revisions_idempotency_key")
		if err != nil {
			t.Fatalf("error checking index: %v", err)
		}
		if !exists {
			t.Fatal("expected unique index 'uq_revisions_idempotency_key' on quotation_revisions")
		}
	})
}

func TestCompleteDedicatedGenerationRoundTripAndTenantIsolation(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)
	companyID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	draft, err := repo.CreateDraftTransaction(ctx, validDraftParams(companyID))
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	err = repo.CompleteDedicatedGeneration(ctx, tx, repository.CompleteDedicatedGenerationParams{
		CompanyID: companyID, RevisionID: draft.RevisionID, ContentHTML: "<section>dedicated</section>",
		CalculationSnapshot: []byte(`{"total_investment":178600000}`),
		ModuleSnapshot:      []byte(`[{"name":"Reservasi","sub_features":["Booking"]}]`),
		TimelineSnapshot:    []byte(`[{"description":"Development","timeline":"Bulan 2, 3"}]`),
		AiProvider:          "openai", AiModel: "test-model",
	})
	if err != nil {
		t.Fatalf("complete dedicated generation: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	var calculation, modules, timeline, html, status string
	err = pool.QueryRow(ctx, `
		SELECT calculation_snapshot::text, module_snapshot::text, timeline_snapshot::text,
		       content_html, generation_status
		FROM quotation_revisions WHERE id = $1 AND company_id = $2
	`, draft.RevisionID, companyID).Scan(&calculation, &modules, &timeline, &html, &status)
	if err != nil {
		t.Fatalf("read snapshots: %v", err)
	}
	if calculation == "" || modules == "" || timeline == "" {
		t.Fatal("all dedicated snapshots must be persisted")
	}
	if html != "<section>dedicated</section>" || status != repository.GenerationStatusCompleted {
		t.Fatalf("unexpected completion state: html=%q status=%q", html, status)
	}

	foreignTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin foreign: %v", err)
	}
	defer foreignTx.Rollback(ctx)
	err = repo.CompleteDedicatedGeneration(ctx, foreignTx, repository.CompleteDedicatedGenerationParams{
		CompanyID: uuid.New(), RevisionID: draft.RevisionID, ContentHTML: "foreign",
		CalculationSnapshot: []byte(`{}`), ModuleSnapshot: []byte(`[]`), TimelineSnapshot: []byte(`[]`),
	})
	if !errors.Is(err, repository.ErrRevisionNotFound) {
		t.Fatalf("foreign tenant completion error = %v, want ErrRevisionNotFound", err)
	}
}

func TestSequentialDoubleSubmitIdempotent(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)

	companyA := uuid.New()
	cleanupCompanies(t, pool, companyA)

	params := validDraftParams(companyA)

	first, err := repo.CreateDraftTransaction(ctx, params)
	if err != nil {
		t.Fatalf("first submit failed: %v", err)
	}
	if first.IdempotentReplay {
		t.Error("first submit must not be flagged as replay")
	}

	replay, err := repo.CreateDraftTransaction(ctx, params)
	if err != nil {
		t.Fatalf("second submit failed: %v", err)
	}
	if !replay.IdempotentReplay {
		t.Error("second submit with same key must be flagged as IdempotentReplay")
	}
	if replay.QuotationID != first.QuotationID {
		t.Errorf("expected same quotation ID, got %s vs %s", replay.QuotationID, first.QuotationID)
	}
	if replay.RevisionID != first.RevisionID {
		t.Errorf("expected same revision ID, got %s vs %s", replay.RevisionID, first.RevisionID)
	}

	var revisionCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM quotation_revisions WHERE company_id = $1 AND idempotency_key = $2`,
		companyA, params.IdempotencyKey,
	).Scan(&revisionCount)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if revisionCount != 1 {
		t.Errorf("expected exactly 1 revision for the key, got %d", revisionCount)
	}

	found, err := repo.FindByIdempotencyKey(ctx, companyA, params.IdempotencyKey)
	if err != nil {
		t.Fatalf("lookup failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected lookup to find the committed revision")
	}
	if found.ID != first.RevisionID {
		t.Errorf("lookup returned revision %s, expected %s", found.ID, first.RevisionID)
	}

	missing, err := repo.FindByIdempotencyKey(ctx, companyA, uuid.New().String())
	if err != nil {
		t.Fatalf("miss lookup must not error: %v", err)
	}
	if missing != nil {
		t.Error("unknown key must return nil result, not a revision")
	}
}

func TestConcurrentDoubleSubmitIdempotent(t *testing.T) {
	pool := getTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repo := repository.NewGenerationRepository(pool)

	companyC := uuid.New()
	cleanupCompanies(t, pool, companyC)

	const defaultGoroutines = 8
	goroutines := defaultGoroutines
	if n := os.Getenv("GEN_CONCURRENCY"); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil && parsed > 0 {
			goroutines = parsed
		}
	}
	params := validDraftParams(companyC)

	start := make(chan struct{})
	results := make(chan *repository.CreateDraftResult, goroutines)
	errs := make(chan error, goroutines)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release all workers at once to maximise contention
			res, err := repo.CreateDraftTransaction(ctx, params)
			if err != nil {
				errs <- err
				return
			}
			results <- res
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent submit returned raw error (unique constraint must be handled internally): %v", err)
	}

	distinctQuotations := make(map[uuid.UUID]struct{})
	var revisionID uuid.UUID
	sawReplay := false
	count := 0
	for res := range results {
		count++
		distinctQuotations[res.QuotationID] = struct{}{}
		if revisionID == (uuid.UUID{}) {
			revisionID = res.RevisionID
		} else if res.RevisionID != revisionID {
			t.Errorf("got different revision IDs across workers: %s vs %s", res.RevisionID, revisionID)
		}
		if res.IdempotentReplay {
			sawReplay = true
		}
	}
	if count != goroutines {
		t.Fatalf("expected %d successful results, got %d", goroutines, count)
	}
	if len(distinctQuotations) != 1 {
		t.Errorf("expected exactly 1 quotation ID across all concurrent submits, got %d", len(distinctQuotations))
	}
	if !sawReplay {
		t.Error("expected at least one worker to observe IdempotentReplay=true")
	}

	var revisionCount int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM quotation_revisions WHERE company_id = $1 AND idempotency_key = $2`,
		companyC, params.IdempotencyKey,
	).Scan(&revisionCount)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if revisionCount != 1 {
		t.Errorf("unique constraint should keep exactly 1 revision row, got %d", revisionCount)
	}
}

func TestCreateDraftRollbackAtomicity(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)

	companyR := uuid.New()
	clientR := uuid.New()
	cleanupCompanies(t, pool, companyR)

	params := validDraftParams(companyR)
	params.ClientID = &clientR
	invalidBudget := int64(-1)
	params.BudgetMaximum = &invalidBudget // violates chk_budget_maximum_non_negative

	if _, err := repo.CreateDraftTransaction(ctx, params); err == nil {
		t.Fatal("expected error when budget_maximum is negative, got nil")
	}

	var quotationCount int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM quotations WHERE company_id = $1 AND quotation_number = $2`,
		companyR, params.QuotationNumber,
	).Scan(&quotationCount)
	if err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if quotationCount != 0 {
		t.Errorf("quotation insert must roll back with the failed transaction, found %d orphan rows", quotationCount)
	}
}

func TestTenantIsolationIdempotencyKey(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)

	companyA := uuid.New()
	companyB := uuid.New()
	cleanupCompanies(t, pool, companyA, companyB)

	sharedKey := uuid.New().String()

	paramsA := validDraftParams(companyA)
	paramsA.IdempotencyKey = sharedKey
	resA, err := repo.CreateDraftTransaction(ctx, paramsA)
	if err != nil {
		t.Fatalf("company A submit failed: %v", err)
	}

	paramsB := validDraftParams(companyB)
	paramsB.IdempotencyKey = sharedKey // identical key, different tenant
	resB, err := repo.CreateDraftTransaction(ctx, paramsB)
	if err != nil {
		t.Fatalf("company B submit with the same key must succeed in its own tenant scope: %v", err)
	}

	if resB.IdempotentReplay {
		t.Error("company B must not receive a replay of company A's aggregate")
	}
	if resB.QuotationID == resA.QuotationID || resB.RevisionID == resA.RevisionID {
		t.Error("tenant isolation violated: same key produced overlapping aggregates across companies")
	}

	foundA, err := repo.FindByIdempotencyKey(ctx, companyA, sharedKey)
	if err != nil || foundA == nil || foundA.ID != resA.RevisionID {
		t.Errorf("company A lookup mismatch: rev=%v err=%v", foundA, err)
	}
	foundB, err := repo.FindByIdempotencyKey(ctx, companyB, sharedKey)
	if err != nil || foundB == nil || foundB.ID != resB.RevisionID {
		t.Errorf("company B lookup mismatch: rev=%v err=%v", foundB, err)
	}
}

func TestUpdateGenerationResultPersistsCompletion(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewGenerationRepository(pool)

	companyU := uuid.New()
	cleanupCompanies(t, pool, companyU)

	params := validDraftParams(companyU)
	draft, err := repo.CreateDraftTransaction(ctx, params)
	if err != nil {
		t.Fatalf("draft creation failed: %v", err)
	}

	contentHTML := "<div data-quotation-block=\"investment\"><p>Total</p></div>"
	hash := "sha256:abc123"
	err = repo.UpdateGenerationResult(ctx, pool, companyU, draft.RevisionID, repository.UpdateGenerationResultParams{
		ContentHTML:           contentHTML,
		AllocationSnapshot:    []byte(`{"total_investment":120000000,"resources":[]}`),
		RateCardSnapshot:      []byte(`{"rate_card_id":"22222222-2222-2222-2222-222222222222"}`),
		LegalTemplateSnapshot: []byte(`{"code":"legal-v1","version":1}`),
		SkillSnapshot:         []byte(`{"skill_id":"33333333-3333-3333-3333-333333333333"}`),
		AiProvider:            "gemini",
		AiModel:               "gemini-2.0-flash",
		ContentHash:           &hash,
	})
	if err != nil {
		t.Fatalf("update generation result failed: %v", err)
	}

	rev, err := repo.FindByIdempotencyKey(ctx, companyU, params.IdempotencyKey)
	if err != nil || rev == nil {
		t.Fatalf("revision not found after completion: %v", err)
	}
	if rev.GenerationStatus != repository.GenerationStatusCompleted {
		t.Errorf("generation_status = %q, want completed", rev.GenerationStatus)
	}
	if rev.ContentHTML == nil || *rev.ContentHTML != contentHTML {
		t.Errorf("content_html not persisted correctly: %v", rev.ContentHTML)
	}
	if rev.GeneratedAt == nil {
		t.Error("generated_at must be set on completion")
	}
	if rev.AiProvider == nil || *rev.AiProvider != "gemini" {
		t.Errorf("ai_provider not persisted: %v", rev.AiProvider)
	}
	if rev.ContentHash == nil || *rev.ContentHash != hash {
		t.Errorf("content_hash not persisted: %v", rev.ContentHash)
	}
	if len(rev.AllocationSnapshot) == 0 {
		t.Error("allocation_snapshot must be persisted")
	}

	err = repo.UpdateGenerationResult(ctx, pool, companyU, uuid.New(), repository.UpdateGenerationResultParams{
		ContentHTML: "<p>x</p>",
		AiProvider:  "gemini",
		AiModel:     "gemini-2.0-flash",
	})
	if err == nil {
		t.Fatal("expected ErrRevisionNotFound when updating unknown revision")
	}
}
