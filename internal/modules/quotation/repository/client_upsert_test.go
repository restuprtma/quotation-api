//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func cleanupClientOrgsByName(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, name := range names {
			pool.Exec(ctx, `DELETE FROM quotation_client_organizations WHERE name = $1`, name)
			pool.Exec(ctx, `DELETE FROM core.clients WHERE name = $1`, name)
		}
	})
}

func TestClientAutoUpsert(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	genRepo := repository.NewGenerationRepository(pool)
	orgRepo := repository.NewClientOrganizationRepository(pool)

	companyID := uuid.New()
	cleanupCompanies(t, pool, companyID)

	t.Run("Exact match reuse", func(t *testing.T) {
		cleanupClientOrgsByName(t, pool, "PT Venturo")

		createdByID := uuid.New()
		existingOrg := &domain.ClientOrganization{
			CompanyID:      companyID,
			ID:             uuid.New(),
			Name:           "PT Venturo",
			NormalizedName: "pt venturo",
			CreatedBy:      &createdByID,
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
		if err := orgRepo.Create(ctx, existingOrg); err != nil {
			t.Fatalf("failed to create initial organization: %v", err)
		}

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

		if actualClientID != existingOrg.ID {
			t.Errorf("expected client ID %v, got %v", existingOrg.ID, actualClientID)
		}
	})

	t.Run("Create new", func(t *testing.T) {
		newClientName := "New Client XYZ"
		cleanupClientOrgsByName(t, pool, newClientName)

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

		// Verify organization was created in quotation_client_organizations
		var dbName string
		err = pool.QueryRow(ctx, `SELECT name FROM quotation_client_organizations WHERE id = $1 AND company_id = $2`, actualClientID, companyID).Scan(&dbName)
		if err != nil {
			t.Fatalf("failed to query created organization: %v", err)
		}

		if dbName != newClientName {
			t.Errorf("expected client name %q, got %q", newClientName, dbName)
		}

		// Verify that core.clients was NOT inserted
		var coreCount int
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM core.clients WHERE name = $1`, newClientName).Scan(&coreCount)
		if err != nil {
			t.Fatalf("failed to query core.clients count: %v", err)
		}
		if coreCount != 0 {
			t.Errorf("expected 0 core.clients named %q, found %d", newClientName, coreCount)
		}
	})

	t.Run("Transaction rollback", func(t *testing.T) {
		rollbackClientName := "New Client Rollback"
		cleanupClientOrgsByName(t, pool, rollbackClientName)

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

		// Verify that the organization was NOT saved to the DB
		var count int
		err = pool.QueryRow(ctx, `SELECT COUNT(*) FROM quotation_client_organizations WHERE name = $1 AND company_id = $2`, rollbackClientName, companyID).Scan(&count)
		if err != nil {
			t.Fatalf("failed to query organization count: %v", err)
		}

		if count != 0 {
			t.Errorf("expected 0 organizations named %q due to rollback, found %d", rollbackClientName, count)
		}
	})
}
