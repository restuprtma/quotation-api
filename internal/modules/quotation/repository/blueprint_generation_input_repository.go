package repository

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrBlueprintSourceNotFound   = errors.New("blueprint source not found")
	ErrBlueprintSourceIneligible = errors.New("blueprint source is not eligible")
)

type BlueprintSourceRecord struct {
	CompanyID    uuid.UUID
	QuotationID  uuid.UUID
	RevisionID   uuid.UUID
	Subject      string
	SolutionType string
	ContentHTML  string
}

type BlueprintGenerationInputRepository struct {
	db *pgxpool.Pool
}

func NewBlueprintGenerationInputRepository(db *pgxpool.Pool) *BlueprintGenerationInputRepository {
	return &BlueprintGenerationInputRepository{db: db}
}

func (r *BlueprintGenerationInputRepository) LoadEligibleSource(
	ctx context.Context,
	companyID, quotationID, revisionID uuid.UUID,
) (BlueprintSourceRecord, error) {
	query := `
		SELECT
			q.company_id,
			q.id,
			qr.id,
			COALESCE(q.subject, ''),
			q.solution_type,
			qr.content_html,
			qr.publication_status,
			qr.generation_status
		FROM quotation_revisions qr
		JOIN quotations q ON q.id = qr.quotation_id AND q.company_id = qr.company_id
		WHERE qr.id = $1
		  AND qr.quotation_id = $2
		  AND qr.company_id = $3
		  AND q.company_id = $3
		  AND q.deleted_at IS NULL
	`

	var rec BlueprintSourceRecord
	var rawContentHTML *string
	var pubStatus string
	var genStatus string

	err := r.db.QueryRow(ctx, query, revisionID, quotationID, companyID).Scan(
		&rec.CompanyID,
		&rec.QuotationID,
		&rec.RevisionID,
		&rec.Subject,
		&rec.SolutionType,
		&rawContentHTML,
		&pubStatus,
		&genStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BlueprintSourceRecord{}, ErrBlueprintSourceNotFound
		}
		return BlueprintSourceRecord{}, err
	}

	// Publication status check: must be draft, published, or superseded
	switch pubStatus {
	case "draft", "published", "superseded":
	default:
		return BlueprintSourceRecord{}, ErrBlueprintSourceIneligible
	}

	// Generation status check: must be completed
	if genStatus != "completed" {
		return BlueprintSourceRecord{}, ErrBlueprintSourceIneligible
	}

	// Content guard check: non-nil and non-blank
	if rawContentHTML == nil || strings.TrimSpace(*rawContentHTML) == "" {
		return BlueprintSourceRecord{}, ErrBlueprintSourceIneligible
	}

	rec.Subject = strings.TrimSpace(rec.Subject)
	rec.SolutionType = strings.TrimSpace(rec.SolutionType)
	rec.ContentHTML = *rawContentHTML

	return rec, nil
}
