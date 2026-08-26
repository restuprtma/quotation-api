package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrSkillNotFound = errors.New("skill not found")

type SkillRepository struct {
	db *pgxpool.Pool
}

func NewSkillRepository(db *pgxpool.Pool) *SkillRepository {
	return &SkillRepository{db: db}
}

func (r *SkillRepository) Create(ctx context.Context, q Execer, skill *domain.Skill) error {
	query := `
		INSERT INTO ai_skills (
			id, company_id, name, command, content_instruction, is_active, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		skill.ID, skill.CompanyID, skill.Name, skill.Command, skill.ContentInstruction, skill.IsActive, skill.CreatedBy,
	).Scan(&skill.CreatedAt, &skill.UpdatedAt)
}

func (r *SkillRepository) Get(ctx context.Context, id, companyID string) (*domain.Skill, error) {
	query := `
		SELECT id, company_id, name, command, content_instruction, is_active, created_by, created_at, updated_at, deleted_at
		FROM ai_skills
		WHERE id = $1 AND company_id = $2 AND deleted_at IS NULL
	`
	skill := &domain.Skill{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&skill.ID, &skill.CompanyID, &skill.Name, &skill.Command, &skill.ContentInstruction, &skill.IsActive, &skill.CreatedBy, &skill.CreatedAt, &skill.UpdatedAt, &skill.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSkillNotFound
	}
	return skill, err
}

func (r *SkillRepository) Update(ctx context.Context, q Execer, skill *domain.Skill) error {
	query := `
		UPDATE ai_skills
		SET name = $1, command = $2, content_instruction = $3, is_active = $4, updated_at = NOW()
		WHERE id = $5 AND company_id = $6 AND deleted_at IS NULL
		RETURNING updated_at
	`
	err := q.QueryRow(ctx, query, skill.Name, skill.Command, skill.ContentInstruction, skill.IsActive, skill.ID, skill.CompanyID).Scan(&skill.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSkillNotFound
	}
	return err
}

func (r *SkillRepository) Delete(ctx context.Context, q Execer, id, companyID string) error {
	query := `
		UPDATE ai_skills
		SET deleted_at = NOW(), is_active = false
		WHERE id = $1 AND company_id = $2 AND deleted_at IS NULL
	`
	tag, err := q.Exec(ctx, query, id, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrSkillNotFound
	}
	return nil
}

func (r *SkillRepository) GetActiveSkillByCommand(ctx context.Context, companyID, command string) (*domain.Skill, error) {
	query := `
		SELECT id, company_id, name, command, content_instruction, is_active, created_by, created_at, updated_at, deleted_at
		FROM ai_skills
		WHERE company_id = $1 AND command = $2 AND is_active = true AND deleted_at IS NULL
	`
	skill := &domain.Skill{}
	err := r.db.QueryRow(ctx, query, companyID, command).Scan(
		&skill.ID, &skill.CompanyID, &skill.Name, &skill.Command, &skill.ContentInstruction, &skill.IsActive, &skill.CreatedBy, &skill.CreatedAt, &skill.UpdatedAt, &skill.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSkillNotFound
	}
	return skill, err
}

func (r *SkillRepository) ListActiveSkills(ctx context.Context, companyID string) ([]domain.Skill, error) {
	query := `
		SELECT id, company_id, name, command, content_instruction, is_active, created_by, created_at, updated_at, deleted_at
		FROM ai_skills
		WHERE company_id = $1 AND is_active = true AND deleted_at IS NULL
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	skills := make([]domain.Skill, 0)
	for rows.Next() {
		var s domain.Skill
		if err := rows.Scan(
			&s.ID, &s.CompanyID, &s.Name, &s.Command, &s.ContentInstruction,
			&s.IsActive, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt,
		); err != nil {
			return nil, err
		}
		skills = append(skills, s)
	}
	return skills, rows.Err()
}
