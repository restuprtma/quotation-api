//go:build integration

// Package testutil provides the single integration-DB and fixture boundary for
// meeting-extraction tests. It deliberately targets the M3 clone, never the
// developer database.
package testutil

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

const (
	coreLedgerVersion      int64 = 14
	quotationLedgerVersion int64 = 20260929010000
)

// Pool opens only the explicitly supplied TEST_DATABASE_URL. Integration tests
// must never silently connect to a developer database or synthesize a DSN.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("meeting extraction integration skipped: TEST_DATABASE_URL must explicitly point to quotation_m3_test")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skip("meeting extraction integration database unavailable: set TEST_DATABASE_URL or provision quotation_m3_test")
		return nil
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skip("meeting extraction integration database unavailable: set TEST_DATABASE_URL or provision quotation_m3_test")
		return nil
	}
	t.Cleanup(pool.Close)
	ensureLedgerAndExtractionSchema(t, pool)
	return pool
}

func ensureLedgerAndExtractionSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	coreVersion, ok := ledgerVersion(ctx, pool, "schema_migrations_core")
	if !ok || coreVersion != coreLedgerVersion {
		t.Fatal("meeting extraction integration refused: target core migration ledger must be version 14")
	}
	quotationVersion, ok := ledgerVersion(ctx, pool, "schema_migrations_quotation")
	if !ok {
		t.Fatal("meeting extraction integration refused: target quotation migration ledger is unavailable")
	}

	var extractionTableExists bool
	err := pool.QueryRow(ctx, `SELECT to_regclass('public.quotation_extraction_runs') IS NOT NULL`).Scan(&extractionTableExists)
	if err != nil {
		t.Fatal("meeting extraction integration refused: unable to inspect quotation_extraction_runs")
	}
	if !extractionTableExists {
		t.Fatal("meeting extraction integration refused: table quotation_extraction_runs does not exist")
	}
	if quotationVersion < quotationLedgerVersion {
		t.Fatalf("meeting extraction integration refused: quotation migration ledger %d is below required %d", quotationVersion, quotationLedgerVersion)
	}
}

func ledgerVersion(ctx context.Context, pool *pgxpool.Pool, table string) (int64, bool) {
	var version int64
	err := pool.QueryRow(ctx, "SELECT version FROM "+table+" LIMIT 1").Scan(&version)
	if err == nil {
		return version, true
	}
	if err == pgx.ErrNoRows {
		return 0, true
	}
	return 0, false
}

// ExtractionFixture creates its tenant chain in foreign-key order: user,
// core.clients, core.companies, quotation, then quotation revision.
// Canary values prove fixture inputs never enter extraction tables.
type ExtractionFixture struct {
	CompanyID   uuid.UUID
	QuotationID uuid.UUID
	RevisionID  uuid.UUID
	Params      repository.CreateExtractionRunParams
}

func NewExtractionFixture(t *testing.T, pool *pgxpool.Pool) ExtractionFixture {
	t.Helper()
	ctx := context.Background()
	companyID, ownerID, clientID := uuid.New(), uuid.New(), uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO core.users (id, email, username, password_hash) VALUES ($1, $2, $3, 'test')`, ownerID, "m8-"+ownerID.String()+"@test.invalid", "m8-"+ownerID.String())
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO core.clients (id, slug, name, owner_user_id) VALUES ($1, $2, 'M8 test client', $3)`, clientID, "m8-"+clientID.String()[:12], ownerID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO core.companies (id, name, owner_id, client_id) VALUES ($1, 'M8 test company', $2, $3)`, companyID, ownerID, clientID)
	require.NoError(t, err)

	budget := int64(150000000)
	created, err := repository.NewGenerationRepository(pool).CreateDraftTransaction(ctx, repository.CreateDraftParams{
		CompanyID: companyID, ClientName: "M8 test client", QuotationNumber: "M8-" + uuid.NewString()[:12], Subject: "M8 adapter", SolutionType: "custom_software", CommercialScheme: "dedicated_squad", IdempotencyKey: uuid.NewString(),
		GenerationInput: []byte(`{"meeting_notes":"CANARY_MEETING_NOTES","prompt":"CANARY_RAW_PROMPT","provider_response":"CANARY_RAW_PROVIDER_RESPONSE"}`), BudgetMaximum: &budget, RateCardSnapshot: []byte(`{}`), LegalTemplateSnapshot: []byte(`{}`), CreatedBy: &ownerID,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM quotations WHERE company_id = $1`, companyID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM core.companies WHERE id = $1`, companyID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM core.clients WHERE id = $1`, clientID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM core.users WHERE id = $1`, ownerID)
	})
	return ExtractionFixture{
		CompanyID: companyID, QuotationID: created.QuotationID, RevisionID: created.RevisionID,
		Params: repository.CreateExtractionRunParams{CompanyID: companyID, QuotationID: created.QuotationID, RevisionID: created.RevisionID, NoteHash: strings.Repeat("a", 64), PipelineVersion: "meeting-extraction-v1"},
	}
}
