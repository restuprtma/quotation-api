//go:build integration

package service_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func ptrString(s string) *string { return &s }
func ptrBool(b bool) *bool       { return &b }

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
		t.Fatalf("failed to connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("failed to ping test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func TestQuotationService_SkillCRUD(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	companyA := uuid.New()
	companyB := uuid.New()

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for _, id := range []uuid.UUID{companyA, companyB} {
			pool.Exec(cleanupCtx, `UPDATE quotations SET active_published_revision_id = NULL WHERE company_id = $1`, id)
			pool.Exec(cleanupCtx, `DELETE FROM quotation_revisions WHERE company_id = $1`, id)
			pool.Exec(cleanupCtx, `DELETE FROM quotations WHERE company_id = $1`, id)
			pool.Exec(cleanupCtx, `DELETE FROM rate_card_items WHERE company_id = $1`, id)
			pool.Exec(cleanupCtx, `DELETE FROM rate_cards WHERE company_id = $1`, id)
			pool.Exec(cleanupCtx, `DELETE FROM legal_templates WHERE company_id = $1`, id)
			pool.Exec(cleanupCtx, `DELETE FROM ai_skills WHERE company_id = $1`, id)
		}
	})

	skillRepo := repository.NewSkillRepository(pool)
	repo := repository.NewRepository(pool)
	// SettingsService nyata: ListSkills memanggil EnsureDefaultAIAssets yang
	// men-seed skill/rate card/legal default per company.
	settingsSvc := service.NewSettingsService(
		pool,
		skillRepo,
		repository.NewRateCardRepository(pool),
		repository.NewLegalTemplateRepository(pool),
		service.NewSkillService(skillRepo),
	)
	svc := service.NewService(repo, nil, skillRepo, nil, nil, settingsSvc, nil, nil)

	// Create a skill for Company A directly via repo to test the Service read/update methods
	skillIDA := uuid.New()
	err := skillRepo.Create(ctx, pool, &domain.Skill{
		ID:                 skillIDA,
		CompanyID:          companyA,
		Command:            "/test_command_a",
		Name:               "Test Command A",
		ContentInstruction: "Original Prompt",
		IsActive:           true,
	})
	if err != nil {
		t.Fatalf("failed to setup skill for company A: %v", err)
	}

	t.Run("ListSkills - Tenant Isolation", func(t *testing.T) {
		// Panggilan pertama men-trigger EnsureDefaultAIAssets untuk company B
		// (men-seed skill default /quotation dan /blueprint milik B sendiri).
		res, err := svc.ListSkills(ctx, companyB.String())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		skills := res.([]dto.SkillResponse)
		for _, s := range skills {
			if s.ID == skillIDA {
				t.Errorf("company B must not see company A skill %v", skillIDA)
			}
		}

		resA, err := svc.ListSkills(ctx, companyA.String())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		skillsA := resA.([]dto.SkillResponse)
		found := false
		for _, s := range skillsA {
			if s.ID == skillIDA {
				found = true
			}
		}
		if !found {
			t.Errorf("expected skill %v visible for company A, got %d skills without it", skillIDA, len(skillsA))
		}
	})

	t.Run("GetSkill - NotFound / Tenant Isolation", func(t *testing.T) {
		_, err := svc.GetSkill(ctx, companyB.String(), skillIDA.String())
		if err == nil {
			t.Errorf("expected error getting company A skill with company B id, got nil")
		}

		res, err := svc.GetSkill(ctx, companyA.String(), skillIDA.String())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		skill := res.(dto.SkillResponse)
		if skill.ID != skillIDA {
			t.Errorf("expected skill ID %v, got %v", skillIDA, skill.ID)
		}
	})

	t.Run("UpdateSkill", func(t *testing.T) {
		req := &dto.UpdateSkillRequest{
			SystemPrompt: ptrString("Updated Prompt"),
			IsActive:     ptrBool(false),
		}

		// Attempt to update with wrong company
		_, err := svc.UpdateSkill(ctx, companyB.String(), skillIDA.String(), req)
		if err == nil {
			t.Errorf("expected error updating company A skill with company B id, got nil")
		}

		// Valid update
		res, err := svc.UpdateSkill(ctx, companyA.String(), skillIDA.String(), req)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		updated := res.(dto.SkillResponse)
		if updated.SystemPrompt != "Updated Prompt" {
			t.Errorf("expected prompt 'Updated Prompt', got %s", updated.SystemPrompt)
		}
		if updated.IsActive != false {
			t.Errorf("expected IsActive false, got true")
		}
	})

	t.Run("ResetSkillDefault", func(t *testing.T) {
		// Attempt to reset with wrong company
		err := svc.ResetSkillDefault(ctx, companyB.String(), skillIDA.String())
		if err == nil {
			t.Errorf("expected error resetting company A skill with company B id, got nil")
		}

		// Valid reset - this might fail if /test_command_a doesn't have a default prompt in embedded files,
		// but since it doesn't, it should return 'default prompt not found'.
		err = svc.ResetSkillDefault(ctx, companyA.String(), skillIDA.String())
		if err == nil {
			t.Errorf("expected error for non-existent default prompt, got nil")
		} else if err.Error() != "default prompt not found" {
			t.Errorf("expected 'default prompt not found', got %v", err)
		}
	})
}
