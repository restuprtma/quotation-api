package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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
