//go:build integration

package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestGetActiveBlueprintSkillByCommand(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	repo := repository.NewSkillRepository(pool)

	cleanupCompany := func(t *testing.T, companyID uuid.UUID) {
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), "DELETE FROM ai_skills WHERE company_id = $1", companyID)
		})
	}

	t.Run("active /blueprint returned", func(t *testing.T) {
		companyID := uuid.New()
		cleanupCompany(t, companyID)

		skillID := uuid.New()
		userID := uuid.New()
		expectedSkill := &domain.Skill{
			ID:                 skillID,
			CompanyID:          companyID,
			Name:               "Blueprint Generator",
			Command:            "/blueprint",
			ContentInstruction: "Instruction for blueprint generation",
			IsActive:           true,
			CreatedBy:          &userID,
		}

		err := repo.Create(ctx, pool, expectedSkill)
		if err != nil {
			t.Fatalf("failed to create active blueprint skill: %v", err)
		}

		skill, err := repo.GetActiveSkillByCommand(ctx, companyID.String(), "/blueprint")
		if err != nil {
			t.Fatalf("expected active blueprint skill, got error: %v", err)
		}
		if skill == nil {
			t.Fatal("expected non-nil skill")
		}
		if skill.ID != expectedSkill.ID {
			t.Errorf("expected ID %s, got %s", expectedSkill.ID, skill.ID)
		}
		if skill.CompanyID != expectedSkill.CompanyID {
			t.Errorf("expected CompanyID %s, got %s", expectedSkill.CompanyID, skill.CompanyID)
		}
		if skill.Name != expectedSkill.Name {
			t.Errorf("expected Name %q, got %q", expectedSkill.Name, skill.Name)
		}
		if skill.Command != expectedSkill.Command {
			t.Errorf("expected Command %q, got %q", expectedSkill.Command, skill.Command)
		}
		if skill.ContentInstruction != expectedSkill.ContentInstruction {
			t.Errorf("expected ContentInstruction %q, got %q", expectedSkill.ContentInstruction, skill.ContentInstruction)
		}
		if !skill.IsActive {
			t.Errorf("expected IsActive to be true, got %v", skill.IsActive)
		}
		if skill.DeletedAt != nil {
			t.Errorf("expected DeletedAt to be nil, got %v", skill.DeletedAt)
		}
	})

	t.Run("/quotation ignored", func(t *testing.T) {
		companyID := uuid.New()
		cleanupCompany(t, companyID)

		skill := &domain.Skill{
			ID:                 uuid.New(),
			CompanyID:          companyID,
			Name:               "Quotation Skill",
			Command:            "/quotation",
			ContentInstruction: "Instruction for quotation",
			IsActive:           true,
		}
		if err := repo.Create(ctx, pool, skill); err != nil {
			t.Fatalf("failed to create quotation skill: %v", err)
		}

		got, err := repo.GetActiveSkillByCommand(ctx, companyID.String(), "/blueprint")
		if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Fatalf("expected ErrSkillNotFound, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil skill, got %+v", got)
		}
	})

	t.Run("missing", func(t *testing.T) {
		companyID := uuid.New()
		cleanupCompany(t, companyID)

		// non-existent command for existing company
		got, err := repo.GetActiveSkillByCommand(ctx, companyID.String(), "/nonexistent")
		if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Fatalf("expected ErrSkillNotFound for nonexistent command, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil skill, got %+v", got)
		}

		// non-existent company
		got, err = repo.GetActiveSkillByCommand(ctx, uuid.New().String(), "/blueprint")
		if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Fatalf("expected ErrSkillNotFound for nonexistent company, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil skill, got %+v", got)
		}
	})

	t.Run("foreign company", func(t *testing.T) {
		companyA := uuid.New()
		companyB := uuid.New()
		cleanupCompany(t, companyA)
		cleanupCompany(t, companyB)

		skill := &domain.Skill{
			ID:                 uuid.New(),
			CompanyID:          companyA,
			Name:               "Blueprint Generator A",
			Command:            "/blueprint",
			ContentInstruction: "Instruction for blueprint generation A",
			IsActive:           true,
		}
		if err := repo.Create(ctx, pool, skill); err != nil {
			t.Fatalf("failed to create blueprint skill for company A: %v", err)
		}

		got, err := repo.GetActiveSkillByCommand(ctx, companyB.String(), "/blueprint")
		if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Fatalf("expected ErrSkillNotFound for foreign company, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil skill, got %+v", got)
		}
	})

	t.Run("inactive", func(t *testing.T) {
		companyID := uuid.New()
		cleanupCompany(t, companyID)

		skill := &domain.Skill{
			ID:                 uuid.New(),
			CompanyID:          companyID,
			Name:               "Inactive Blueprint",
			Command:            "/blueprint",
			ContentInstruction: "Instruction for blueprint generation",
			IsActive:           false,
		}
		if err := repo.Create(ctx, pool, skill); err != nil {
			t.Fatalf("failed to create inactive skill: %v", err)
		}

		got, err := repo.GetActiveSkillByCommand(ctx, companyID.String(), "/blueprint")
		if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Fatalf("expected ErrSkillNotFound for inactive skill, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil skill, got %+v", got)
		}
	})

	t.Run("soft-deleted", func(t *testing.T) {
		companyID := uuid.New()
		cleanupCompany(t, companyID)

		skillID := uuid.New()
		deletedTime := time.Now().UTC()
		query := `
			INSERT INTO ai_skills (
				id, company_id, name, command, content_instruction, is_active, created_at, updated_at, deleted_at
			) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW(), $7)
		`
		_, err := pool.Exec(ctx, query, skillID, companyID, "Deleted Blueprint", "/blueprint", "Instruction", true, deletedTime)
		if err != nil {
			t.Fatalf("failed to insert soft-deleted skill: %v", err)
		}

		got, err := repo.GetActiveSkillByCommand(ctx, companyID.String(), "/blueprint")
		if !errors.Is(err, repository.ErrSkillNotFound) {
			t.Fatalf("expected ErrSkillNotFound for soft-deleted skill, got %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil skill, got %+v", got)
		}
	})
}
