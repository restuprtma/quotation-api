package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrBlueprintNotFound = errors.New("blueprint not found")

type BlueprintRepository struct {
	db *pgxpool.Pool
}

func NewBlueprintRepository(db *pgxpool.Pool) *BlueprintRepository {
	return &BlueprintRepository{db: db}
}

func (r *BlueprintRepository) CreateBlueprint(ctx context.Context, q Execer, bp *domain.Blueprint) error {
	query := `
		INSERT INTO blueprints (
			id, company_id, quotation_id, title, status, current_revision, active_published_revision_id, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		bp.ID, bp.CompanyID, bp.QuotationID, bp.Title, bp.Status, bp.CurrentRevision, bp.ActivePublishedRevisionID, bp.CreatedBy,
	).Scan(&bp.CreatedAt, &bp.UpdatedAt)
}

func (r *BlueprintRepository) GetBlueprintByID(ctx context.Context, id, companyID string) (*domain.Blueprint, error) {
	query := `
		SELECT id, company_id, quotation_id, title, status, current_revision, active_published_revision_id, created_by, created_at, updated_at, deleted_at
		FROM blueprints
		WHERE id = $1 AND company_id = $2 AND deleted_at IS NULL
	`
	bp := &domain.Blueprint{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&bp.ID, &bp.CompanyID, &bp.QuotationID, &bp.Title, &bp.Status, &bp.CurrentRevision, &bp.ActivePublishedRevisionID, &bp.CreatedBy, &bp.CreatedAt, &bp.UpdatedAt, &bp.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBlueprintNotFound
	}
	return bp, err
}

func (r *BlueprintRepository) GetBlueprintByQuotationID(ctx context.Context, quotationID, companyID string) (*domain.Blueprint, error) {
	query := `
		SELECT id, company_id, quotation_id, title, status, current_revision, active_published_revision_id, created_by, created_at, updated_at, deleted_at
		FROM blueprints
		WHERE quotation_id = $1 AND company_id = $2 AND deleted_at IS NULL
	`
	bp := &domain.Blueprint{}
	err := r.db.QueryRow(ctx, query, quotationID, companyID).Scan(
		&bp.ID, &bp.CompanyID, &bp.QuotationID, &bp.Title, &bp.Status, &bp.CurrentRevision, &bp.ActivePublishedRevisionID, &bp.CreatedBy, &bp.CreatedAt, &bp.UpdatedAt, &bp.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBlueprintNotFound
	}
	return bp, err
}

func (r *BlueprintRepository) ListBlueprints(ctx context.Context, companyID string) ([]domain.Blueprint, error) {
	query := `
		SELECT id, company_id, quotation_id, title, status, current_revision, active_published_revision_id, created_by, created_at, updated_at, deleted_at
		FROM blueprints
		WHERE company_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Blueprint
	for rows.Next() {
		bp := domain.Blueprint{}
		err := rows.Scan(
			&bp.ID, &bp.CompanyID, &bp.QuotationID, &bp.Title, &bp.Status, &bp.CurrentRevision, &bp.ActivePublishedRevisionID, &bp.CreatedBy, &bp.CreatedAt, &bp.UpdatedAt, &bp.DeletedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, bp)
	}
	return result, rows.Err()
}

func (r *BlueprintRepository) CreateBlueprintRevision(ctx context.Context, q Execer, rev *domain.BlueprintRevision) error {
	query := `
		INSERT INTO blueprint_revisions (
			id, company_id, blueprint_id, revision_number, idempotency_key, source_quotation_revision_id, source_context_snapshot,
			publication_status, completeness_status, generation_status, content_json, content_version, validation_result, cross_validation_result,
			skill_snapshot, safety_envelope_version, ai_provider, ai_model, generated_at, published_at, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
		) RETURNING created_at, updated_at
	`
	return q.QueryRow(ctx, query,
		rev.ID, rev.CompanyID, rev.BlueprintID, rev.RevisionNumber, rev.IdempotencyKey, rev.SourceQuotationRevisionID, rev.SourceContextSnapshot,
		rev.PublicationStatus, rev.CompletenessStatus, rev.GenerationStatus, rev.ContentJSON, rev.ContentVersion, rev.ValidationResult, rev.CrossValidationResult,
		rev.SkillSnapshot, rev.SafetyEnvelopeVersion, rev.AiProvider, rev.AiModel, rev.GeneratedAt, rev.PublishedAt, rev.CreatedBy,
	).Scan(&rev.CreatedAt, &rev.UpdatedAt)
}

func (r *BlueprintRepository) GetBlueprintRevision(ctx context.Context, id, companyID string) (*domain.BlueprintRevision, error) {
	query := `
		SELECT
			id, company_id, blueprint_id, revision_number, idempotency_key, source_quotation_revision_id, source_context_snapshot,
			publication_status, completeness_status, generation_status, content_json, content_version, validation_result, cross_validation_result,
			skill_snapshot, safety_envelope_version, ai_provider, ai_model, generated_at, published_at, created_by, created_at, updated_at
		FROM blueprint_revisions
		WHERE id = $1 AND company_id = $2
	`
	rev := &domain.BlueprintRevision{}
	err := r.db.QueryRow(ctx, query, id, companyID).Scan(
		&rev.ID, &rev.CompanyID, &rev.BlueprintID, &rev.RevisionNumber, &rev.IdempotencyKey, &rev.SourceQuotationRevisionID, &rev.SourceContextSnapshot,
		&rev.PublicationStatus, &rev.CompletenessStatus, &rev.GenerationStatus, &rev.ContentJSON, &rev.ContentVersion, &rev.ValidationResult, &rev.CrossValidationResult,
		&rev.SkillSnapshot, &rev.SafetyEnvelopeVersion, &rev.AiProvider, &rev.AiModel, &rev.GeneratedAt, &rev.PublishedAt, &rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRevisionNotFound
	}
	return rev, err
}

func (r *BlueprintRepository) UpdateSectionRun(ctx context.Context, q Execer, run *domain.BlueprintSectionRun) error {
	query := `
		INSERT INTO blueprint_section_runs (
			id, company_id, blueprint_revision_id, section_type, status, attempt_count, content_json, validation_result, safe_error_code, started_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) ON CONFLICT (blueprint_revision_id, section_type) DO UPDATE SET
			status = EXCLUDED.status,
			attempt_count = EXCLUDED.attempt_count,
			content_json = EXCLUDED.content_json,
			validation_result = EXCLUDED.validation_result,
			safe_error_code = EXCLUDED.safe_error_code,
			started_at = EXCLUDED.started_at,
			completed_at = EXCLUDED.completed_at,
			updated_at = NOW()
		RETURNING updated_at
	`
	return q.QueryRow(ctx, query,
		run.ID, run.CompanyID, run.BlueprintRevisionID, run.SectionType, run.Status, run.AttemptCount, run.ContentJSON, run.ValidationResult, run.SafeErrorCode, run.StartedAt, run.CompletedAt,
	).Scan(&run.UpdatedAt)
}

func (r *BlueprintRepository) ListSectionRuns(ctx context.Context, revisionID, companyID string) ([]domain.BlueprintSectionRun, error) {
	query := `
		SELECT
			id, company_id, blueprint_revision_id, section_type, status, attempt_count, content_json, validation_result, safe_error_code, started_at, completed_at, updated_at
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND company_id = $2
	`
	rows, err := r.db.Query(ctx, query, revisionID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.BlueprintSectionRun
	for rows.Next() {
		run := domain.BlueprintSectionRun{}
		err := rows.Scan(
			&run.ID, &run.CompanyID, &run.BlueprintRevisionID, &run.SectionType, &run.Status, &run.AttemptCount, &run.ContentJSON, &run.ValidationResult, &run.SafeErrorCode, &run.StartedAt, &run.CompletedAt, &run.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, run)
	}
	return result, rows.Err()
}
