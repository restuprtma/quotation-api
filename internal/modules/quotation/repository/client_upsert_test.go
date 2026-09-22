//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func cleanupClientsByName(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, name := range names {
			pool.Exec(ctx, `DELETE FROM core.clients WHERE name = $1`, name)
		}
	})
}

func TestClientAutoUpsert(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	genRepo := repository.NewGenerationRepository(pool)
	clientRepo := repository.NewRepository(pool)

	companyID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	t.Run("Exact match reuse", func(t *testing.T) {
		cleanupClientsByName(t, pool, "PT Venturo")

		createdByID := uuid.New()
		existingClientIDStr, err := clientRepo.CreateClientByName(ctx, pool, "PT Venturo", &createdByID)
		if err != nil {
			t.Fatalf("failed to create initial client: %v", err)
		}
		existingClientID := uuid.MustParse(existingClientIDStr)

		params := validDraftParams(companyID)
		params.ClientID = nil
		params.ClientName = " Pt Venturo "
		params.IdempotencyKey = uuid.New().String()
		params.QuotationNumber = "QUO-REUSE-" + params.IdempotencyKey[:8]

		draft, err := genRepo.CreateDraftTransaction(ctx, params)
		if err != nil {
			t.Fatalf("draft creation failed: %v", err)
		}

		var actualClientID uuid.UUID
		err = pool.QueryRow(ctx, `SELECT client_id FROM quotations WHERE id = $1`, draft.QuotationID).Scan(&actualClientID)
		if err != nil {
			t.Fatalf("failed to query quotation client_id: %v", err)
		}

		if actualClientID != existingClientID {
			t.Errorf("expected client ID %v, got %v", existingClientID, actualClientID)
		}
	})

	t.Run("Create new", func(t *testing.T) {
		newClientName := "New Client XYZ"
		cleanupClientsByName(t, pool, newClientName)

		params := validDraftParams(companyID)
		params.ClientID = nil
		params.ClientName = newClientName
		params.IdempotencyKey = uuid.New().String()
		params.QuotationNumber = "QUO-NEW-" + params.IdempotencyKey[:8]

		draft, err := genRepo.CreateDraftTransaction(ctx, params)
		if err != nil {
			t.Fatalf("draft creation failed: %v", err)
		}

		var actualClientID uuid.UUID
		err = pool.QueryRow(ctx, `SELECT client_id FROM quotations WHERE id = $1`, draft.QuotationID).Scan(&actualClientID)
		if err != nil {
			t.Fatalf("failed to query quotation client_id: %v", err)
		}

		// Verify client was created in DB
		var dbName string
		err = pool.QueryRow(ctx, `SELECT name FROM core.clients WHERE id = $1`, actualClientID).Scan(&dbName)
		if err != nil {
			t.Fatalf("failed to query created client: %v", err)
		}

		if dbName != newClientName {
			t.Errorf("expected client name %q, got %q", newClientName, dbName)
		}
	})

	t.Run("Transaction rollback", func(t *testing.T) {
		rollbackClientName := "New Client Rollback"
		cleanupClientsByName(t, pool, rollbackClientName)

		params := validDraftParams(companyID)
		params.ClientID = nil
		params.ClientName = rollbackClientName
		params.IdempotencyKey = uuid.New().String()
		params.QuotationNumber = "QUO-ROLL-" + params.IdempotencyKey[:8]

		// Force failure by providing an invalid budget
		invalidBudget := int64(-1)
		params.BudgetMaximum = &invalidBudget

		_, err := genRepo.CreateDraftTransaction(ctx, params)
		if err == nil {
			t.Fatal("expected error when budget_maximum is negative, got nil")
		}

		// Verify that the client was NOT saved to the DB
		var count int
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM core.clients WHERE name = $1`, rollbackClientName).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query client count: %v", err)
		}

		if count != 0 {
			t.Errorf("expected 0 clients named %q due to rollback, found %d", rollbackClientName, count)
		}
	})
}
