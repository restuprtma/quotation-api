package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

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

// ListBlueprints retrieves projected blueprint items for a tenant, optionally filtered by quotation ID.
func (r *BlueprintReadRepository) ListBlueprints(ctx context.Context, companyID uuid.UUID, quotationID *uuid.UUID) ([]dto.BlueprintListItemResponse, error) {
	if companyID == uuid.Nil {
		return make([]dto.BlueprintListItemResponse, 0), nil
	}

	query := `
		SELECT
			b.id AS blueprint_id,
			b.quotation_id,
			b.title,
			COALESCE(r.generation_status, 'pending') AS generation_status,
			COALESCE(r.updated_at, b.updated_at) AS updated_at
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
		WHERE b.company_id = $1
		  AND b.deleted_at IS NULL
	`
	args := []any{companyID}

	if quotationID != nil {
		if *quotationID == uuid.Nil {
			return make([]dto.BlueprintListItemResponse, 0), nil
		}
		query += " AND b.quotation_id = $2"
		args = append(args, *quotationID)
	}

	query += " ORDER BY updated_at DESC, b.created_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}
