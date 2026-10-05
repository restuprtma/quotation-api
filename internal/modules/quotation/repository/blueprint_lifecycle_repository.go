package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

var (
	ErrBlueprintDraftNotFound    = errors.New("active draft blueprint revision not found")
	ErrBlueprintMasterNotFound   = errors.New("blueprint master not found")
	ErrBlueprintRevisionNotFound = errors.New("blueprint revision not found")
)

// BlueprintLifecycleRepository defines the lifecycle operations for blueprints and their revisions.
type BlueprintLifecycleRepository interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	LockBlueprintMaster(ctx context.Context, tx pgx.Tx, blueprintID string) error
	DeleteDraft(ctx context.Context, companyID, blueprintID string) error
	PublishDraftBlueprint(ctx context.Context, tx pgx.Tx, blueprintID, draftID string, isComplete bool) error
	MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, blueprintID string) error
	GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, blueprintID string) (int, error)
	GetDraftRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error)
	GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error)
	CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error
	UpdateBlueprintCurrentRevision(ctx context.Context, tx pgx.Tx, blueprintID string, currentRevision int) error
}

// NewBlueprintLifecycleRepository returns a BlueprintGenerationRepository as the lifecycle repository.
func NewBlueprintLifecycleRepository(pool *pgxpool.Pool) *BlueprintGenerationRepository {
	return NewBlueprintGenerationRepository(pool)
}

// Pool returns the underlying database pool.
func (r *BlueprintGenerationRepository) Pool() *pgxpool.Pool {
	return r.db
}

// Begin starts a new transaction on the underlying database pool.
func (r *BlueprintGenerationRepository) Begin(ctx context.Context) (pgx.Tx, error) {
	if r.db == nil {
		return nil, errors.New("db pool is nil")
	}
	return r.db.Begin(ctx)
}

func (r *BlueprintGenerationRepository) getExecer(tx pgx.Tx) Execer {
	if tx != nil {
		return tx
	}
	return r.db
}

// LockBlueprintMaster locks the blueprint row using SELECT id FROM blueprints WHERE id = $1 FOR UPDATE.
// Returns ErrBlueprintMasterNotFound if not found.
func (r *BlueprintGenerationRepository) LockBlueprintMaster(ctx context.Context, tx pgx.Tx, blueprintID string) error {
	q := r.getExecer(tx)
	var id string
	err := q.QueryRow(ctx, "SELECT id FROM blueprints WHERE id = $1 FOR UPDATE", blueprintID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBlueprintMasterNotFound
		}
		return err
	}
	return nil
}

// DeleteDraft deletes the draft revision for a blueprint scoped by company_id.
// Returns ErrBlueprintDraftNotFound if rows affected == 0.
func (r *BlueprintGenerationRepository) DeleteDraft(ctx context.Context, companyID, blueprintID string) error {
	res, err := r.db.Exec(ctx, `
		DELETE FROM blueprint_revisions 
		WHERE blueprint_id = $1 AND company_id = $2 AND publication_status = 'draft'
	`, blueprintID, companyID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrBlueprintDraftNotFound
	}
	return nil
}

// MarkPublishedRevisionSuperseded updates the current published revision for the blueprint to 'superseded'.
func (r *BlueprintGenerationRepository) MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, blueprintID string) error {
	q := r.getExecer(tx)
	_, err := q.Exec(ctx, `
		UPDATE blueprint_revisions 
		SET publication_status = 'superseded', updated_at = NOW() 
		WHERE blueprint_id = $1 AND publication_status = 'published'
	`, blueprintID)
	return err
}

// PublishDraftBlueprint transitions a draft blueprint revision to published and updates the blueprint master.
// Completeness status is "complete" if isComplete else "partial".
// Returns ErrBlueprintRevisionNotFound if the draft revision is not found.
// Returns ErrBlueprintMasterNotFound if the blueprint master is not found.
func (r *BlueprintGenerationRepository) PublishDraftBlueprint(ctx context.Context, tx pgx.Tx, blueprintID, draftID string, isComplete bool) error {
	completenessStatus := "partial"
	if isComplete {
		completenessStatus = "complete"
	}

	q := r.getExecer(tx)

	res, err := q.Exec(ctx, `
		UPDATE blueprint_revisions 
		SET publication_status = 'published', completeness_status = $1, published_at = NOW(), updated_at = NOW() 
		WHERE id = $2 AND publication_status = 'draft'
	`, completenessStatus, draftID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrBlueprintRevisionNotFound
	}

	res, err = q.Exec(ctx, `
		UPDATE blueprints 
		SET active_published_revision_id = $1, status = 'published', updated_at = NOW() 
		WHERE id = $2
	`, draftID, blueprintID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrBlueprintMasterNotFound
	}

	return nil
}

// GetMaxRevisionNumber returns the highest revision number for a blueprint, defaulting to 0 if none exist.
func (r *BlueprintGenerationRepository) GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, blueprintID string) (int, error) {
	q := r.getExecer(tx)
	var maxRev int
	err := q.QueryRow(ctx, "SELECT COALESCE(MAX(revision_number), 0) FROM blueprint_revisions WHERE blueprint_id = $1", blueprintID).Scan(&maxRev)
	if err != nil {
		return 0, err
	}
	return maxRev, nil
}

// GetDraftRevision retrieves the current draft revision for a blueprint.
func (r *BlueprintGenerationRepository) GetDraftRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error) {
	q := r.getExecer(tx)
	query := `
		SELECT
			id, company_id, blueprint_id, revision_number,
			idempotency_key, idempotency_payload_hash,
			source_quotation_revision_id, source_context_snapshot,
			publication_status, completeness_status, generation_status,
			content_json, content_version,
			validation_result, cross_validation_result,
			skill_snapshot, safety_envelope_version,
			ai_provider, ai_model,
			generated_at, published_at,
			created_by, created_at, updated_at
		FROM blueprint_revisions
		WHERE blueprint_id = $1 AND company_id = $2 AND publication_status = 'draft'
		ORDER BY revision_number DESC
		LIMIT 1
	`
	return scanBlueprintRevisionRow(q.QueryRow(ctx, query, blueprintID, companyID), ErrBlueprintDraftNotFound)
}

// GetActivePublishedRevision retrieves the current active published revision for a blueprint.
func (r *BlueprintGenerationRepository) GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, blueprintID, companyID string) (*domain.BlueprintRevision, error) {
	q := r.getExecer(tx)
	query := `
		SELECT
			id, company_id, blueprint_id, revision_number,
			idempotency_key, idempotency_payload_hash,
			source_quotation_revision_id, source_context_snapshot,
			publication_status, completeness_status, generation_status,
			content_json, content_version,
			validation_result, cross_validation_result,
			skill_snapshot, safety_envelope_version,
			ai_provider, ai_model,
			generated_at, published_at,
			created_by, created_at, updated_at
		FROM blueprint_revisions
		WHERE blueprint_id = $1 AND company_id = $2 AND publication_status = 'published'
		ORDER BY revision_number DESC
		LIMIT 1
	`
	return scanBlueprintRevisionRow(q.QueryRow(ctx, query, blueprintID, companyID), ErrBlueprintRevisionNotFound)
}

// CreateRevisionTx inserts a new blueprint revision under transaction (or pool fallback).
func (r *BlueprintGenerationRepository) CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.BlueprintRevision) error {
	if rev.ID == uuid.Nil {
		rev.ID = uuid.New()
	}
	if rev.PublicationStatus == "" {
		rev.PublicationStatus = "draft"
	}
	if rev.CompletenessStatus == "" {
		rev.CompletenessStatus = "partial"
	}
	if rev.GenerationStatus == "" {
		rev.GenerationStatus = "pending"
	}
	if len(rev.SourceContextSnapshot) == 0 {
		rev.SourceContextSnapshot = []byte("{}")
	}
	if len(rev.ContentJSON) == 0 {
		rev.ContentJSON = []byte("{}")
	}
	if rev.ContentVersion == 0 {
		rev.ContentVersion = 1
	}
	if len(rev.ValidationResult) == 0 {
		rev.ValidationResult = []byte("{}")
	}
	if len(rev.CrossValidationResult) == 0 {
		rev.CrossValidationResult = []byte("{}")
	}

	var skillSnapshot *string
	if len(rev.SkillSnapshot) > 0 {
		s := string(rev.SkillSnapshot)
		skillSnapshot = &s
	}

	query := `
		INSERT INTO blueprint_revisions (
			id, company_id, blueprint_id, revision_number,
			idempotency_key, idempotency_payload_hash,
			source_quotation_revision_id, source_context_snapshot,
			publication_status, completeness_status, generation_status,
			content_json, content_version,
			validation_result, cross_validation_result,
			skill_snapshot, safety_envelope_version,
			ai_provider, ai_model,
			generated_at, published_at, created_by
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7, $8,
			$9, $10, $11,
			$12, $13,
			$14, $15,
			$16, $17,
			$18, $19,
			$20, $21, $22
		) RETURNING created_at, updated_at
	`
	q := r.getExecer(tx)
	err := q.QueryRow(ctx, query,
		rev.ID, rev.CompanyID, rev.BlueprintID, rev.RevisionNumber,
		rev.IdempotencyKey, rev.IdempotencyPayloadHash,
		rev.SourceQuotationRevisionID, rev.SourceContextSnapshot,
		rev.PublicationStatus, rev.CompletenessStatus, rev.GenerationStatus,
		rev.ContentJSON, rev.ContentVersion,
		rev.ValidationResult, rev.CrossValidationResult,
		skillSnapshot, rev.SafetyEnvelopeVersion,
		rev.AiProvider, rev.AiModel,
		rev.GeneratedAt, rev.PublishedAt, rev.CreatedBy,
	).Scan(&rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create blueprint revision in tx", logger.Err(err))
		return err
	}
	return nil
}

// UpdateBlueprintCurrentRevision updates the current revision pointer on the blueprint master row.
func (r *BlueprintGenerationRepository) UpdateBlueprintCurrentRevision(ctx context.Context, tx pgx.Tx, blueprintID string, currentRevision int) error {
	q := r.getExecer(tx)
	res, err := q.Exec(ctx, `
		UPDATE blueprints 
		SET current_revision = $1, updated_at = NOW() 
		WHERE id = $2
	`, currentRevision, blueprintID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrBlueprintMasterNotFound
	}
	return nil
}

func scanBlueprintRevisionRow(row pgx.Row, notFoundErr error) (*domain.BlueprintRevision, error) {
	rev := &domain.BlueprintRevision{}
	err := row.Scan(
		&rev.ID, &rev.CompanyID, &rev.BlueprintID, &rev.RevisionNumber,
		&rev.IdempotencyKey, &rev.IdempotencyPayloadHash,
		&rev.SourceQuotationRevisionID, &rev.SourceContextSnapshot,
		&rev.PublicationStatus, &rev.CompletenessStatus, &rev.GenerationStatus,
		&rev.ContentJSON, &rev.ContentVersion,
		&rev.ValidationResult, &rev.CrossValidationResult,
		&rev.SkillSnapshot, &rev.SafetyEnvelopeVersion,
		&rev.AiProvider, &rev.AiModel,
		&rev.GeneratedAt, &rev.PublishedAt,
		&rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if notFoundErr != nil {
				return nil, notFoundErr
			}
			return nil, ErrBlueprintRevisionNotFound
		}
		return nil, err
	}
	if rev.IdempotencyPayloadHash != nil {
		trimmed := strings.TrimSpace(*rev.IdempotencyPayloadHash)
		rev.IdempotencyPayloadHash = &trimmed
	}
	return rev, nil
}
