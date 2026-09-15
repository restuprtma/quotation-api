//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestTenantIsolation(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	companyA := uuid.New()
	companyB := uuid.New()
	clientA := uuid.New()

	// Create required dummy clients for Company A
	pool.Exec(ctx, "INSERT INTO core.clients (id, company_id, name) VALUES ($1, $2, 'Client A') ON CONFLICT DO NOTHING", clientA, companyA)

	t.Run("Quotations", func(t *testing.T) {
		repo := repository.NewRepository(pool)

		qID := uuid.New()
		quot := &domain.Quotation{
			ID:               qID,
			CompanyID:        companyA,
			ClientID:         clientA,
			QuotationNumber:  "QT-001",
			SolutionType:     "custom",
			CommercialScheme: "dedicated_squad",
			Status:           "draft",
			CurrentRevision:  1,
			PublicToken:      uuid.New(),
		}

		err := repo.CreateQuotation(ctx, pool, quot)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		_, err = repo.GetQuotationByID(ctx, qID.String(), companyB.String())
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Blueprints", func(t *testing.T) {
		repoQ := repository.NewRepository(pool)
		repoB := repository.NewBlueprintRepository(pool)

		qID := uuid.New()
		quot := &domain.Quotation{
			ID:               qID,
			CompanyID:        companyA,
			ClientID:         clientA,
			QuotationNumber:  "QT-002",
			SolutionType:     "custom",
			CommercialScheme: "dedicated_squad",
			Status:           "draft",
			CurrentRevision:  1,
			PublicToken:      uuid.New(),
		}
		repoQ.CreateQuotation(ctx, pool, quot)

		bID := uuid.New()
		bp := &domain.Blueprint{
			ID:              bID,
			CompanyID:       companyA,
			QuotationID:     qID,
			Title:           "BP-001",
			Status:          "draft",
			CurrentRevision: 1,
		}

		err := repoB.CreateBlueprint(ctx, pool, bp)
		if err != nil {
			t.Fatalf("failed to insert blueprint: %v", err)
		}

		_, err = repoB.GetBlueprintByID(ctx, bID.String(), companyB.String())
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if !errors.Is(err, repository.ErrBlueprintNotFound) {
			t.Errorf("expected ErrBlueprintNotFound, got %v", err)
		}
	})

	t.Run("Skills", func(t *testing.T) {
		repo := repository.NewSkillRepository(pool)

		sID := uuid.New()
		skill := &domain.Skill{
			ID:                 sID,
			CompanyID:          companyA,
			Name:               "Skill A",
			Command:            "skill_a",
			ContentInstruction: "Do A",
			IsActive:           true,
		}

		err := repo.Create(ctx, pool, skill)
		if err != nil {
			t.Fatalf("failed to insert skill: %v", err)
		}

		_, err = repo.Get(ctx, sID.String(), companyB.String())
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Errorf("expected ErrSkillNotFound, got %v", err)
		}
	})

	t.Run("Credentials", func(t *testing.T) {
		repo := repository.NewCredentialRepository(pool)

		cID := uuid.New()
		cred := &domain.Credential{
			ID:              cID,
			CompanyID:       companyA,
			Provider:        "OpenAI",
			Model:           "gpt-4",
			EncryptedAPIKey: "enc_key",
			KeyHint:         "sk-...",
			IsActive:        true,
		}

		err := repo.Create(ctx, pool, cred)
		if err != nil {
			t.Fatalf("failed to insert credential: %v", err)
		}

		_, err = repo.Get(ctx, cID.String(), companyB.String())
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if !errors.Is(err, repository.ErrCredentialNotFound) {
			t.Errorf("expected ErrCredentialNotFound, got %v", err)
		}
	})

	t.Run("RateCards", func(t *testing.T) {
		repo := repository.NewRateCardRepository(pool)

		rID := uuid.New()
		rc := &domain.RateCard{
			ID:        rID,
			CompanyID: companyA,
			Code:      "RC-001",
			Name:      "Standard",
			Market:    "national",
			Currency:  "USD",
			IsActive:  true,
		}

		err := repo.Create(ctx, pool, rc)
		if err != nil {
			t.Fatalf("failed to insert rate card: %v", err)
		}

		_, err = repo.Get(ctx, rID.String(), companyB.String())
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if !errors.Is(err, repository.ErrRateCardNotFound) {
			t.Errorf("expected ErrRateCardNotFound, got %v", err)
		}
	})

	t.Run("LegalTemplates", func(t *testing.T) {
		repo := repository.NewLegalTemplateRepository(pool)

		lID := uuid.New()
		lt := &domain.LegalTemplate{
			ID:          lID,
			CompanyID:   companyA,
			Code:        "LT-001",
			Version:     1,
			ContentHTML: "<p>Terms</p>",
			IsActive:    true,
		}

		err := repo.Create(ctx, pool, lt)
		if err != nil {
			t.Fatalf("failed to insert legal template: %v", err)
		}

		_, err = repo.Get(ctx, lID.String(), companyB.String())
		if err == nil {
			t.Errorf("expected error, got nil")
		} else if !errors.Is(err, repository.ErrLegalTemplateNotFound) {
			t.Errorf("expected ErrLegalTemplateNotFound, got %v", err)
		}
	})

	t.Run("ShareLinks", func(t *testing.T) {
		repoQ := repository.NewRepository(pool)
		repoB := repository.NewBlueprintRepository(pool)
		repoS := repository.NewShareLinkRepository(pool)

		qID := uuid.New()
		quot := &domain.Quotation{
			ID:               qID,
			CompanyID:        companyA,
			ClientID:         clientA,
			QuotationNumber:  "QT-003",
			SolutionType:     "custom",
			CommercialScheme: "dedicated_squad",
			Status:           "draft",
			CurrentRevision:  1,
			PublicToken:      uuid.New(),
		}
		repoQ.CreateQuotation(ctx, pool, quot)

		bID := uuid.New()
		bp := &domain.Blueprint{
			ID:              bID,
			CompanyID:       companyA,
			QuotationID:     qID,
			Title:           "BP-002",
			Status:          "draft",
			CurrentRevision: 1,
		}
		repoB.CreateBlueprint(ctx, pool, bp)

		// Create a dummy quotation revision for source_quotation_revision_id
		repoR := repository.NewRevisionRepository(pool)
		qrID := uuid.New()
		budgetMax := int64(1000)
		repoR.CreateRevision(ctx, pool, &domain.QuotationRevision{
			ID:                    qrID,
			CompanyID:             companyA,
			QuotationID:           qID,
			RevisionNumber:        1,
			PublicationStatus:     "draft",
			GenerationStatus:      "completed",
			GenerationInput:       []byte(`{}`),
			BudgetMaximum:         &budgetMax,
			AllocationSnapshot:    []byte(`{}`),
			RateCardSnapshot:      []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`),
			ContentHTML:           func(s string) *string { return &s }(""),
			ContentVersion:        func(s string) *string { return &s }("1"),
			SkillSnapshot:         []byte(`{}`),
		})

		brID := uuid.New()
		bpRev := &domain.BlueprintRevision{
			ID:                        brID,
			CompanyID:                 companyA,
			BlueprintID:               bID,
			RevisionNumber:            1,
			SourceQuotationRevisionID: &qrID,
			PublicationStatus:         "draft",
			CompletenessStatus:        "complete",
			GenerationStatus:          "completed",
			ContentJSON:               []byte(`{}`),
			ContentVersion:            1,
			SourceContextSnapshot:     []byte(`{}`),
			ValidationResult:          []byte(`{}`),
			CrossValidationResult:     []byte(`{}`),
			SkillSnapshot:             []byte(`{}`),
		}
		err := repoB.CreateBlueprintRevision(ctx, pool, bpRev)
		if err != nil {
			t.Fatalf("failed to insert blueprint revision: %v", err)
		}

		slID := uuid.New()
		sl := &domain.ShareLink{
			ID:                  slID,
			CompanyID:           companyA,
			BlueprintID:         &bID,
			BlueprintRevisionID: &brID,
			TokenHash:           uuid.New().String(),
			Label:               "Link 1",
		}

		err = repoS.Create(ctx, pool, sl)
		if err != nil {
			t.Fatalf("failed to insert share link: %v", err)
		}

		_, err = repoS.ListByBlueprint(ctx, bID.String(), companyB.String())
		if err != nil {
			t.Fatalf("expected no error when listing for wrong company, got %v", err)
		}
	})
}

func TestSecondActiveDraftDitolak(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	companyA := uuid.New()
	clientA := uuid.New()
	pool.Exec(ctx, "INSERT INTO core.clients (id, company_id, name) VALUES ($1, $2, 'Client A') ON CONFLICT DO NOTHING", clientA, companyA)
	budgetMax := int64(1000)

	t.Run("Quotation Second Draft Rejected", func(t *testing.T) {
		repoQ := repository.NewRepository(pool)
		repoR := repository.NewRevisionRepository(pool)

		qID := uuid.New()
		quot := &domain.Quotation{
			ID:               qID,
			CompanyID:        companyA,
			ClientID:         clientA,
			QuotationNumber:  "QT-004",
			SolutionType:     "custom",
			CommercialScheme: "dedicated_squad",
			Status:           "draft",
			CurrentRevision:  1,
			PublicToken:      uuid.New(),
		}
		err := repoQ.CreateQuotation(ctx, pool, quot)
		if err != nil {
			t.Fatalf("failed to insert quotation: %v", err)
		}

		rev1 := &domain.QuotationRevision{
			ID:                    uuid.New(),
			CompanyID:             companyA,
			QuotationID:           qID,
			RevisionNumber:        1,
			PublicationStatus:     "draft",
			GenerationStatus:      "completed",
			GenerationInput:       []byte(`{}`),
			BudgetMaximum:         &budgetMax,
			AllocationSnapshot:    []byte(`{}`),
			RateCardSnapshot:      []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`),
			ContentHTML:           func(s string) *string { return &s }(""),
			ContentVersion:        func(s string) *string { return &s }("1"),
			SkillSnapshot:         []byte(`{}`),
		}
		err = repoR.CreateRevision(ctx, pool, rev1)
		if err != nil {
			t.Fatalf("failed to insert first quotation revision: %v", err)
		}

		rev2 := &domain.QuotationRevision{
			ID:                    uuid.New(),
			CompanyID:             companyA,
			QuotationID:           qID,
			RevisionNumber:        2,
			PublicationStatus:     "draft",
			GenerationStatus:      "completed",
			GenerationInput:       []byte(`{}`),
			BudgetMaximum:         &budgetMax,
			AllocationSnapshot:    []byte(`{}`),
			RateCardSnapshot:      []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`),
			ContentHTML:           func(s string) *string { return &s }(""),
			ContentVersion:        func(s string) *string { return &s }("1"),
			SkillSnapshot:         []byte(`{}`),
		}
		err = repoR.CreateRevision(ctx, pool, rev2)
		if err == nil {
			t.Fatalf("expected error when inserting second draft revision, got nil")
		}
	})

	t.Run("Blueprint Second Draft Rejected", func(t *testing.T) {
		repoQ := repository.NewRepository(pool)
		repoB := repository.NewBlueprintRepository(pool)
		repoR := repository.NewRevisionRepository(pool)

		qID := uuid.New()
		quot := &domain.Quotation{
			ID:               qID,
			CompanyID:        companyA,
			ClientID:         clientA,
			QuotationNumber:  "QT-005",
			SolutionType:     "custom",
			CommercialScheme: "dedicated_squad",
			Status:           "draft",
			CurrentRevision:  1,
			PublicToken:      uuid.New(),
		}
		repoQ.CreateQuotation(ctx, pool, quot)

		qrID := uuid.New()
		repoR.CreateRevision(ctx, pool, &domain.QuotationRevision{
			ID:                    qrID,
			CompanyID:             companyA,
			QuotationID:           qID,
			RevisionNumber:        1,
			PublicationStatus:     "draft",
			GenerationStatus:      "completed",
			GenerationInput:       []byte(`{}`),
			BudgetMaximum:         &budgetMax,
			AllocationSnapshot:    []byte(`{}`),
			RateCardSnapshot:      []byte(`{}`),
			LegalTemplateSnapshot: []byte(`{}`),
			ContentHTML:           func(s string) *string { return &s }(""),
			ContentVersion:        func(s string) *string { return &s }("1"),
			SkillSnapshot:         []byte(`{}`),
		})

		bID := uuid.New()
		bp := &domain.Blueprint{
			ID:              bID,
			CompanyID:       companyA,
			QuotationID:     qID,
			Title:           "BP-005",
			Status:          "draft",
			CurrentRevision: 1,
		}
		repoB.CreateBlueprint(ctx, pool, bp)

		bpRev1 := &domain.BlueprintRevision{
			ID:                        uuid.New(),
			CompanyID:                 companyA,
			BlueprintID:               bID,
			RevisionNumber:            1,
			SourceQuotationRevisionID: &qrID,
			PublicationStatus:         "draft",
			CompletenessStatus:        "complete",
			GenerationStatus:          "completed",
			ContentJSON:               []byte(`{}`),
			ContentVersion:            1,
			SourceContextSnapshot:     []byte(`{}`),
			ValidationResult:          []byte(`{}`),
			CrossValidationResult:     []byte(`{}`),
			SkillSnapshot:             []byte(`{}`),
		}
		err := repoB.CreateBlueprintRevision(ctx, pool, bpRev1)
		if err != nil {
			t.Fatalf("failed to insert first blueprint revision: %v", err)
		}

		bpRev2 := &domain.BlueprintRevision{
			ID:                        uuid.New(),
			CompanyID:                 companyA,
			BlueprintID:               bID,
			RevisionNumber:            2,
			SourceQuotationRevisionID: &qrID,
			PublicationStatus:         "draft",
			CompletenessStatus:        "complete",
			GenerationStatus:          "completed",
			ContentJSON:               []byte(`{}`),
			ContentVersion:            1,
			SourceContextSnapshot:     []byte(`{}`),
			ValidationResult:          []byte(`{}`),
			CrossValidationResult:     []byte(`{}`),
			SkillSnapshot:             []byte(`{}`),
		}
		err = repoB.CreateBlueprintRevision(ctx, pool, bpRev2)
		if err == nil {
			t.Fatalf("expected error when inserting second draft blueprint revision, got nil")
		}
	})
}
