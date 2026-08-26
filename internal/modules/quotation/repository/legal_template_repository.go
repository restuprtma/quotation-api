package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrLegalTemplateNotFound = errors.New("legal template not found")

type LegalTemplateRepository struct {
	db *pgxpool.Pool
}

func NewLegalTemplateRepository(db *pgxpool.Pool) *LegalTemplateRepository {
	return &LegalTemplateRepository{db: db}
}

func (r *LegalTemplateRepository) Create(ctx context.Context, q Execer, template *domain.LegalTemplate) error {
	query := `
		INSERT INTO legal_templates (
			id, company_id, code, version, content_html, is_active, approved_by, approved_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at
	`
	return q.QueryRow(ctx, query,
		template.ID, template.CompanyID, template.Code, template.Version, template.ContentHTML, template.IsActive, template.ApprovedBy, template.ApprovedAt,
	).Scan(&template.CreatedAt)
}

func (r *LegalTemplateRepository) Get(ctx context.Context, id, companyID string) (*domain.LegalTemplate, error) {
	query := `
		SELECT id, company_id, code, version, content_html, is_active, approved_by, approved_at, created_at
		FROM legal_templates
		WHERE id = $1 AND company_id = $2
	`
	tmpl := &domain.LegalTemplate{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&tmpl.ID, &tmpl.CompanyID, &tmpl.Code, &tmpl.Version, &tmpl.ContentHTML, &tmpl.IsActive, &tmpl.ApprovedBy, &tmpl.ApprovedAt, &tmpl.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLegalTemplateNotFound
	}
	return tmpl, err
}

func (r *LegalTemplateRepository) Update(ctx context.Context, q Execer, template *domain.LegalTemplate) error {
	query := `
		UPDATE legal_templates
		SET code = $1, version = $2, content_html = $3, is_active = $4, approved_by = $5, approved_at = $6
		WHERE id = $7 AND company_id = $8
	`
	tag, err := q.Exec(ctx, query, template.Code, template.Version, template.ContentHTML, template.IsActive, template.ApprovedBy, template.ApprovedAt, template.ID, template.CompanyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLegalTemplateNotFound
	}
	return nil
}

func (r *LegalTemplateRepository) Delete(ctx context.Context, q Execer, id, companyID string) error {
	query := `
		DELETE FROM legal_templates
		WHERE id = $1 AND company_id = $2
	`
	tag, err := q.Exec(ctx, query, id, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLegalTemplateNotFound
	}
	return nil
}

func (r *LegalTemplateRepository) GetActiveTemplate(ctx context.Context, companyID, code string) (*domain.LegalTemplate, error) {
	query := `
		SELECT id, company_id, code, version, content_html, is_active, approved_by, approved_at, created_at
		FROM legal_templates
		WHERE company_id = $1 AND code = $2 AND is_active = true
		ORDER BY version DESC LIMIT 1
	`
	tmpl := &domain.LegalTemplate{}
	err := r.db.QueryRow(ctx, query, companyID, code).Scan(
		&tmpl.ID, &tmpl.CompanyID, &tmpl.Code, &tmpl.Version, &tmpl.ContentHTML, &tmpl.IsActive, &tmpl.ApprovedBy, &tmpl.ApprovedAt, &tmpl.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLegalTemplateNotFound
	}
	return tmpl, err
}

func (r *LegalTemplateRepository) CreateNewVersion(ctx context.Context, q Execer, template *domain.LegalTemplate) error {
	// Essentially same as Create
	return r.Create(ctx, q, template)
}

func (r *LegalTemplateRepository) ListActiveTemplates(ctx context.Context, companyID string) ([]*domain.LegalTemplate, error) {
	query := `
		SELECT id, company_id, code, version, content_html, is_active, approved_by, approved_at, created_at, updated_at
		FROM legal_templates
		WHERE company_id = $1 AND is_active = true
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*domain.LegalTemplate
	for rows.Next() {
		lt := &domain.LegalTemplate{}
		err := rows.Scan(&lt.ID, &lt.CompanyID, &lt.Code, &lt.Version, &lt.ContentHTML, &lt.IsActive, &lt.ApprovedBy, &lt.ApprovedAt, &lt.CreatedAt, &lt.UpdatedAt)
		if err != nil {
			return nil, err
		}
		list = append(list, lt)
	}
	return list, rows.Err()
}
