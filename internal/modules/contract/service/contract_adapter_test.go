//go:build integration

package service_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	contract_repo "venturo-skeleton-go/internal/modules/contract/repository"
	"venturo-skeleton-go/internal/modules/contract/service"
	quotation_domain "venturo-skeleton-go/internal/modules/quotation/domain"
	quotation_repo "venturo-skeleton-go/internal/modules/quotation/repository"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "localhost"
		}
		port := os.Getenv("DB_PORT")
		if port == "" {
			port = "5432"
		}
		user := os.Getenv("DB_USER")
		if user == "" {
			user = "postgres"
		}
		pass := os.Getenv("DB_PASSWORD")
		if pass == "" {
			pass = "postgres"
		}
		name := os.Getenv("DB_NAME")
		if name == "" {
			name = "quotation_db"
		}
		ssl := os.Getenv("DB_SSLMODE")
		if ssl == "" {
			ssl = "disable"
		}
		dsn = fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, name, ssl)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("failed to connect to db: %v", err)
	}

	return pool
}

func TestContractAdapter(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	companyID := uuid.New()
	clientID := uuid.New()

	qRepo := quotation_repo.NewRepository(pool)
	rRepo := quotation_repo.NewRevisionRepository(pool)
	cRepo := contract_repo.NewContractRepository(pool)
	svc := service.NewContractService(cRepo, qRepo, rRepo)

	// Create quotation
	qID := uuid.New()
	quot := &quotation_domain.Quotation{
		ID:               qID,
		CompanyID:        companyID,
		ClientID:         clientID,
		QuotationNumber:  "QT-CONTRACT-001",
		SolutionType:     "custom",
		CommercialScheme: "dedicated_squad",
		Status:           "draft",
		CurrentRevision:  1,
		PublicToken:      uuid.New(),
	}

	err := qRepo.CreateQuotation(ctx, pool, quot)
	if err != nil {
		t.Fatalf("failed to create quotation: %v", err)
	}

	now := time.Now()
	contentVersion := "1"
	budgetMax := int64(1000)
	emptyHTML := ""

	t.Run("CreateContract fails when revision is not signed", func(t *testing.T) {
		revID := uuid.New()
		rev := &quotation_domain.QuotationRevision{
			ID:                    revID,
			CompanyID:             companyID,
			QuotationID:           qID,
			RevisionNumber:        1,
			PublicationStatus:     "published",
			GenerationStatus:      "completed",
			GenerationInput:       []byte(`{}`),
			BudgetMaximum:         &budgetMax,
			AllocationSnapshot:    []byte(`{}`),
			RateCardSnapshot:      []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`),
			ContentHTML:           &emptyHTML,
			ContentVersion:        &contentVersion,
			SkillSnapshot:         []byte(`{}`),
		}

		err := rRepo.CreateRevision(ctx, pool, rev)
		if err != nil {
			t.Fatalf("failed to create revision: %v", err)
		}

		_, err = svc.CreateContract(ctx, companyID.String(), qID.String(), uuid.New().String())
		if !errors.Is(err, service.ErrQuotationNotSigned) {
			t.Errorf("expected ErrQuotationNotSigned, got %v", err)
		}
	})

	t.Run("CreateContract succeeds with signed revision", func(t *testing.T) {
		qID2 := uuid.New()
		quot2 := &quotation_domain.Quotation{
			ID:               qID2,
			CompanyID:        companyID,
			ClientID:         clientID,
			QuotationNumber:  "QT-CONTRACT-002",
			SolutionType:     "custom",
			CommercialScheme: "dedicated_squad",
			Status:           "draft",
			CurrentRevision:  1,
			PublicToken:      uuid.New(),
		}
		err := qRepo.CreateQuotation(ctx, pool, quot2)
		if err != nil {
			t.Fatalf("failed to create quotation 2: %v", err)
		}

		revID := uuid.New()
		sigData := "signature_data_abc"
		signerName := "John Doe"
		rev := &quotation_domain.QuotationRevision{
			ID:                    revID,
			CompanyID:             companyID,
			QuotationID:           qID2,
			RevisionNumber:        1,
			PublicationStatus:     "published",
			GenerationStatus:      "completed",
			GenerationInput:       []byte(`{}`),
			BudgetMaximum:         &budgetMax,
			AllocationSnapshot:    []byte(`{}`),
			RateCardSnapshot:      []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`),
			ContentHTML:           &emptyHTML,
			ContentVersion:        &contentVersion,
			SkillSnapshot:         []byte(`{}`),
			SignedAt:              &now,
			SignatureData:         &sigData,
			SignerName:            &signerName,
		}

		err = rRepo.CreateRevision(ctx, pool, rev)
		if err != nil {
			t.Fatalf("failed to create signed revision: %v", err)
		}

		contract, err := svc.CreateContract(ctx, companyID.String(), qID2.String(), uuid.New().String())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if contract.RevisionID != revID.String() {
			t.Errorf("expected revision ID %s, got %s", revID.String(), contract.RevisionID)
		}

		// Test contract already exists
		_, err = svc.CreateContract(ctx, companyID.String(), qID2.String(), uuid.New().String())
		if !errors.Is(err, service.ErrContractExists) {
			t.Errorf("expected ErrContractExists, got %v", err)
		}
	})
}
