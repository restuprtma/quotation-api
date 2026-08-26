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

	skillRepo := repository.NewSkillRepository(pool)
	repo := repository.NewRepository(pool)
	svc := service.NewService(repo, nil, skillRepo, nil, nil, nil, nil, nil)

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
		res, err := svc.ListSkills(ctx, companyB.String())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		skills := res.([]dto.SkillResponse)
		if len(skills) != 0 {
			t.Errorf("expected 0 skills for company B, got %d", len(skills))
		}

		resA, err := svc.ListSkills(ctx, companyA.String())
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		skillsA := resA.([]dto.SkillResponse)
		if len(skillsA) == 0 {
			t.Errorf("expected at least 1 skill for company A, got 0")
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
