package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

// BlueprintListParams encapsulates query parameters for listing blueprints.
type BlueprintListParams struct {
	QuotationID        *uuid.UUID
	GenerationStatuses []string
	Page, Limit        int
}

// BlueprintReadRepository handles projection queries and snapshot reads for blueprints.
type BlueprintReadRepository struct {
	db      *pgxpool.Pool
	genRepo *BlueprintGenerationRepository
}

// NewBlueprintReadRepository constructs a new BlueprintReadRepository.
func NewBlueprintReadRepository(db *pgxpool.Pool) *BlueprintReadRepository {
	return &BlueprintReadRepository{
		db:      db,
		genRepo: NewBlueprintGenerationRepository(db),
	}
}

// GetBlueprintGeneration delegates snapshot read of aggregate blueprint generation to BlueprintGenerationRepository.
func (r *BlueprintReadRepository) GetBlueprintGeneration(ctx context.Context, blueprintID, companyID uuid.UUID) (*CreateBlueprintGenerationResult, error) {
	return r.genRepo.GetBlueprintGeneration(ctx, blueprintID, companyID)
}

// ListBlueprints retrieves projected blueprint items for a tenant, optionally filtered by quotation ID and generation status with pagination.
func (r *BlueprintReadRepository) ListBlueprints(ctx context.Context, companyID uuid.UUID, params BlueprintListParams) ([]dto.BlueprintListItemResponse, int, error) {
	if companyID == uuid.Nil {
		return make([]dto.BlueprintListItemResponse, 0), 0, nil
	}

	fromSQL := `
		FROM blueprints b
		LEFT JOIN LATERAL (
			SELECT r.generation_status, r.updated_at
			FROM blueprint_revisions r
			WHERE r.blueprint_id = b.id
			  AND r.company_id = b.company_id
			  AND (r.publication_status = 'draft' OR r.revision_number = b.current_revision)
			ORDER BY CASE WHEN r.publication_status = 'draft' THEN 1 ELSE 2 END, r.revision_number DESC
			LIMIT 1
		) r ON true
	`

	args := []any{companyID}
	whereClauses := []string{"b.company_id = $1", "b.deleted_at IS NULL"}

	if params.QuotationID != nil {
		if *params.QuotationID == uuid.Nil {
			return make([]dto.BlueprintListItemResponse, 0), 0, nil
		}
		args = append(args, *params.QuotationID)
		whereClauses = append(whereClauses, fmt.Sprintf("b.quotation_id = $%d", len(args)))
	}

	if len(params.GenerationStatuses) > 0 {
		args = append(args, params.GenerationStatuses)
		whereClauses = append(whereClauses, fmt.Sprintf("COALESCE(r.generation_status, 'pending') = ANY($%d)", len(args)))
	}

	whereSQL := strings.Join(whereClauses, " AND ")

	countQuery := "SELECT COUNT(*) " + fromSQL + " WHERE " + whereSQL
	var total int
	err := r.db.QueryRow(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return make([]dto.BlueprintListItemResponse, 0), 0, nil
	}

	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit < 1 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}
	offset := (page - 1) * limit

	selectArgs := append(make([]any, 0, len(args)+2), args...)
	selectArgs = append(selectArgs, limit, offset)
	limitArgIdx := len(selectArgs) - 1
	offsetArgIdx := len(selectArgs)

	selectQuery := fmt.Sprintf(`
		SELECT
			b.id AS blueprint_id,
			b.quotation_id,
			b.title,
			COALESCE(r.generation_status, 'pending') AS generation_status,
			COALESCE(r.updated_at, b.updated_at) AS updated_at
		%s
		WHERE %s
		ORDER BY updated_at DESC, b.created_at DESC
		LIMIT $%d OFFSET $%d
	`, fromSQL, whereSQL, limitArgIdx, offsetArgIdx)

	rows, err := r.db.Query(ctx, selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]dto.BlueprintListItemResponse, 0)
	for rows.Next() {
		var item dto.BlueprintListItemResponse
		if err := rows.Scan(
			&item.BlueprintID,
			&item.QuotationID,
			&item.Title,
			&item.GenerationStatus,
			&item.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// BlueprintRevisionDetail aggregates a blueprint revision with its section runs and source quotation revision info.
type BlueprintRevisionDetail struct {
	Revision                 domain.BlueprintRevision
	SourceQuotationRevNumber *int
	SectionRuns              []domain.BlueprintSectionRun
}

// ListBlueprintRevisionHistory retrieves all revisions for a blueprint scoped by company ID, ordered by revision_number DESC, along with section runs and source quotation revision numbers.
func (r *BlueprintReadRepository) ListBlueprintRevisionHistory(ctx context.Context, blueprintID, companyID uuid.UUID) ([]BlueprintRevisionDetail, error) {
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintNotFound
	}
	if r.db == nil {
		return nil, errors.New("database pool is nil")
	}

	// Verify blueprint exists and belongs to company
	var bpID uuid.UUID
	err := r.db.QueryRow(ctx, "SELECT id FROM blueprints WHERE id = $1 AND company_id = $2 AND deleted_at IS NULL", blueprintID, companyID).Scan(&bpID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	revQuery := `
		SELECT
			r.id, r.company_id, r.blueprint_id, r.revision_number,
			r.idempotency_key, r.idempotency_payload_hash,
			r.source_quotation_revision_id, r.source_context_snapshot,
			r.publication_status, r.completeness_status, r.generation_status,
			r.content_json, r.content_version,
			r.validation_result, r.cross_validation_result,
			r.skill_snapshot, r.safety_envelope_version,
			r.ai_provider, r.ai_model,
			r.generated_at, r.published_at,
			r.created_by, r.created_at, r.updated_at,
			qr.revision_number AS source_quotation_rev_number
		FROM blueprint_revisions r
		LEFT JOIN quotation_revisions qr ON qr.id = r.source_quotation_revision_id AND qr.company_id = r.company_id
		WHERE r.blueprint_id = $1 AND r.company_id = $2
		ORDER BY r.revision_number DESC
	`

	rows, err := r.db.Query(ctx, revQuery, blueprintID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var details []BlueprintRevisionDetail
	var revIDs []uuid.UUID
	revMap := make(map[uuid.UUID]int)

	for rows.Next() {
		var d BlueprintRevisionDetail
		err := rows.Scan(
			&d.Revision.ID, &d.Revision.CompanyID, &d.Revision.BlueprintID, &d.Revision.RevisionNumber,
			&d.Revision.IdempotencyKey, &d.Revision.IdempotencyPayloadHash,
			&d.Revision.SourceQuotationRevisionID, &d.Revision.SourceContextSnapshot,
			&d.Revision.PublicationStatus, &d.Revision.CompletenessStatus, &d.Revision.GenerationStatus,
			&d.Revision.ContentJSON, &d.Revision.ContentVersion,
			&d.Revision.ValidationResult, &d.Revision.CrossValidationResult,
			&d.Revision.SkillSnapshot, &d.Revision.SafetyEnvelopeVersion,
			&d.Revision.AiProvider, &d.Revision.AiModel,
			&d.Revision.GeneratedAt, &d.Revision.PublishedAt,
			&d.Revision.CreatedBy, &d.Revision.CreatedAt, &d.Revision.UpdatedAt,
			&d.SourceQuotationRevNumber,
		)
		if err != nil {
			return nil, err
		}
		if d.Revision.IdempotencyPayloadHash != nil {
			trimmed := strings.TrimSpace(*d.Revision.IdempotencyPayloadHash)
			d.Revision.IdempotencyPayloadHash = &trimmed
		}
		revMap[d.Revision.ID] = len(details)
		revIDs = append(revIDs, d.Revision.ID)
		details = append(details, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(revIDs) > 0 {
		runsQuery := `
			SELECT
				id, company_id, blueprint_revision_id, section_type, status, attempt_count,
				content_json, validation_result, safe_error_code, started_at, completed_at, updated_at
			FROM blueprint_section_runs
			WHERE blueprint_revision_id = ANY($1) AND company_id = $2
			ORDER BY blueprint_revision_id, CASE section_type
				WHEN 'context' THEN 1 WHEN 'journey' THEN 2 WHEN 'architecture' THEN 3
				WHEN 'state_machine' THEN 4 WHEN 'workflow' THEN 5 WHEN 'wbs' THEN 6
				WHEN 'cross_validation' THEN 7 ELSE 99 END
		`
		runRows, err := r.db.Query(ctx, runsQuery, revIDs, companyID)
		if err != nil {
			return nil, err
		}
		defer runRows.Close()

		for runRows.Next() {
			var run domain.BlueprintSectionRun
			if err := runRows.Scan(
				&run.ID, &run.CompanyID, &run.BlueprintRevisionID, &run.SectionType, &run.Status, &run.AttemptCount,
				&run.ContentJSON, &run.ValidationResult, &run.SafeErrorCode, &run.StartedAt, &run.CompletedAt, &run.UpdatedAt,
			); err != nil {
				return nil, err
			}
			if idx, ok := revMap[run.BlueprintRevisionID]; ok {
				details[idx].SectionRuns = append(details[idx].SectionRuns, run)
			}
		}
		if err := runRows.Err(); err != nil {
			return nil, err
		}
	}

	return details, nil
}

// GetRevisionForSharing loads a specific blueprint revision scoped by company and blueprint ID for sharing.
func (r *BlueprintReadRepository) GetRevisionForSharing(ctx context.Context, companyID, blueprintID, revisionID uuid.UUID) (*domain.BlueprintRevision, error) {
	if companyID == uuid.Nil || blueprintID == uuid.Nil || revisionID == uuid.Nil {
		return nil, ErrBlueprintNotFound
	}

	query := `
		SELECT
			id, company_id, blueprint_id, revision_number,
			publication_status, completeness_status, generation_status,
			content_json, content_version,
			validation_result, cross_validation_result,
			published_at, created_at, updated_at
		FROM blueprint_revisions
		WHERE id = $1 AND blueprint_id = $2 AND company_id = $3
	`
	var rev domain.BlueprintRevision
	err := r.db.QueryRow(ctx, query, revisionID, blueprintID, companyID).Scan(
		&rev.ID, &rev.CompanyID, &rev.BlueprintID, &rev.RevisionNumber,
		&rev.PublicationStatus, &rev.CompletenessStatus, &rev.GenerationStatus,
		&rev.ContentJSON, &rev.ContentVersion,
		&rev.ValidationResult, &rev.CrossValidationResult,
		&rev.PublishedAt, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}
	return &rev, nil
}

// LoadSharedRevisionData loads the blueprint master, the specified revision, and its section runs for public sharing without company isolation filter (token lookup already guarantees authorization).
func (r *BlueprintReadRepository) LoadSharedRevisionData(
	ctx context.Context, blueprintID, revisionID uuid.UUID,
) (*domain.Blueprint, *domain.BlueprintRevision, []domain.BlueprintSectionRun, error) {
	if blueprintID == uuid.Nil || revisionID == uuid.Nil {
		return nil, nil, nil, ErrBlueprintNotFound
	}

	bpQuery := `SELECT id, company_id, quotation_id, title, created_at, updated_at FROM blueprints WHERE id = $1 AND deleted_at IS NULL`
	var bp domain.Blueprint
	err := r.db.QueryRow(ctx, bpQuery, blueprintID).Scan(
		&bp.ID, &bp.CompanyID, &bp.QuotationID, &bp.Title, &bp.CreatedAt, &bp.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil, ErrBlueprintNotFound
		}
		return nil, nil, nil, err
	}

	revQuery := `
		SELECT
			id, company_id, blueprint_id, revision_number,
			publication_status, completeness_status, generation_status,
			content_json, content_version,
			validation_result, cross_validation_result,
			published_at, created_at, updated_at
		FROM blueprint_revisions
		WHERE id = $1 AND blueprint_id = $2
	`
	var rev domain.BlueprintRevision
	err = r.db.QueryRow(ctx, revQuery, revisionID, blueprintID).Scan(
		&rev.ID, &rev.CompanyID, &rev.BlueprintID, &rev.RevisionNumber,
		&rev.PublicationStatus, &rev.CompletenessStatus, &rev.GenerationStatus,
		&rev.ContentJSON, &rev.ContentVersion,
		&rev.ValidationResult, &rev.CrossValidationResult,
		&rev.PublishedAt, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, nil, ErrBlueprintNotFound
		}
		return nil, nil, nil, err
	}

	runsQuery := `
		SELECT
			id, company_id, blueprint_revision_id, section_type, status, attempt_count,
			content_json, validation_result, safe_error_code, started_at, completed_at, updated_at
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1
		ORDER BY CASE section_type
			WHEN 'context' THEN 1 WHEN 'journey' THEN 2 WHEN 'architecture' THEN 3
			WHEN 'state_machine' THEN 4 WHEN 'workflow' THEN 5 WHEN 'wbs' THEN 6
			WHEN 'cross_validation' THEN 7 ELSE 99 END
	`
	rows, err := r.db.Query(ctx, runsQuery, revisionID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	var runs []domain.BlueprintSectionRun
	for rows.Next() {
		var run domain.BlueprintSectionRun
		err := rows.Scan(
			&run.ID, &run.CompanyID, &run.BlueprintRevisionID, &run.SectionType, &run.Status, &run.AttemptCount,
			&run.ContentJSON, &run.ValidationResult, &run.SafeErrorCode, &run.StartedAt, &run.CompletedAt, &run.UpdatedAt,
		)
		if err != nil {
			return nil, nil, nil, err
		}
		runs = append(runs, run)
	}

	return &bp, &rev, runs, rows.Err()
}
