//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

func TestSkillRepository_CRUD(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()

	companyID := uuid.New()
	skillID := uuid.New()
	userID := uuid.New()

	repo := repository.NewSkillRepository(pool)

	t.Run("Create", func(t *testing.T) {
		skill := &domain.Skill{
			ID:                 skillID,
			CompanyID:          companyID,
			Name:               "Test Skill",
			Command:            "test_cmd",
			ContentInstruction: "Instruction",
			IsActive:           true,
			CreatedBy:          &userID,
		}

		err := repo.Create(ctx, pool, skill)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}
	})

	t.Run("Get", func(t *testing.T) {
		skill, err := repo.Get(ctx, skillID.String(), companyID.String())
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if skill.Name != "Test Skill" {
			t.Errorf("expected Test Skill, got %s", skill.Name)
		}
	})

	t.Run("Update", func(t *testing.T) {
		skill, _ := repo.Get(ctx, skillID.String(), companyID.String())
		skill.Name = "Updated Skill"
		err := repo.Update(ctx, pool, skill)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		updated, _ := repo.Get(ctx, skillID.String(), companyID.String())
		if updated.Name != "Updated Skill" {
			t.Errorf("expected Updated Skill, got %s", updated.Name)
		}
	})

	t.Run("ListActiveSkills", func(t *testing.T) {
		skills, err := repo.ListActiveSkills(ctx, companyID.String())
		if err != nil {
			t.Fatalf("ListActiveSkills failed: %v", err)
		}
		if len(skills) != 1 {
			t.Errorf("expected 1 skill, got %d", len(skills))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := repo.Delete(ctx, pool, skillID.String(), companyID.String())
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = repo.Get(ctx, skillID.String(), companyID.String())
		if err == nil {
			t.Errorf("expected error after delete, got nil")
		}
	})
}
