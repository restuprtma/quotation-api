package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	ErrShareLinkNotFound = errors.New("share link not found")
)

type ShareLinkWithRevision struct {
	domain.ShareLink
	RevisionNumber int `json:"revision_number" db:"revision_number"`
}

type ShareLinkRepository struct {
	db *pgxpool.Pool
}

func NewShareLinkRepository(db *pgxpool.Pool) *ShareLinkRepository {
	return &ShareLinkRepository{db: db}
}

// Create inserts a new blueprint share link record into database.
func (r *ShareLinkRepository) Create(ctx context.Context, link *domain.ShareLink) error {
	query := `
		INSERT INTO blueprint_share_links (
			id, company_id, blueprint_id, blueprint_revision_id, token_hash, label, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at
	`
	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now().UTC()
	}

	return r.db.QueryRow(ctx, query,
		link.ID, link.CompanyID, link.BlueprintID, link.BlueprintRevisionID, link.TokenHash, link.Label, link.CreatedBy, link.CreatedAt,
	).Scan(&link.CreatedAt)
}

// Revoke revokes an individual share link by setting revoked_at to NOW().
// Operation is idempotent: revoking an already revoked link returns nil without error.
func (r *ShareLinkRepository) Revoke(ctx context.Context, companyID, blueprintID, linkID uuid.UUID) error {
	query := `
		UPDATE blueprint_share_links
		SET revoked_at = NOW()
		WHERE id = $1 AND blueprint_id = $2 AND company_id = $3 AND revoked_at IS NULL
	`
	_, err := r.db.Exec(ctx, query, linkID, blueprintID, companyID)
	return err
}

// GetActiveByTokenHash finds an active, non-revoked share link by token hash.
func (r *ShareLinkRepository) GetActiveByTokenHash(ctx context.Context, tokenHash string) (*domain.ShareLink, error) {
	query := `
		SELECT id, company_id, blueprint_id, blueprint_revision_id, token_hash, label, created_by, created_at, revoked_at, last_accessed_at
		FROM blueprint_share_links
		WHERE token_hash = $1 AND revoked_at IS NULL
	`
	link := &domain.ShareLink{}
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(
		&link.ID, &link.CompanyID, &link.BlueprintID, &link.BlueprintRevisionID, &link.TokenHash, &link.Label, &link.CreatedBy, &link.CreatedAt, &link.RevokedAt, &link.LastAccessedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrShareLinkNotFound
		}
		return nil, err
	}
	return link, nil
}

// TouchLastAccessed updates last_accessed_at to current timestamp.
func (r *ShareLinkRepository) TouchLastAccessed(ctx context.Context, linkID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `UPDATE blueprint_share_links SET last_accessed_at = NOW() WHERE id = $1`, linkID)
	return err
}

// ListByBlueprint returns all share links (active and revoked) for a blueprint ordered by created_at DESC.
// Token hashes are omitted from higher level responses.
func (r *ShareLinkRepository) ListByBlueprint(ctx context.Context, companyID, blueprintID uuid.UUID) ([]ShareLinkWithRevision, error) {
	query := `
		SELECT 
			l.id, l.company_id, l.blueprint_id, l.blueprint_revision_id, l.token_hash, 
			l.label, l.created_by, l.created_at, l.revoked_at, l.last_accessed_at,
			COALESCE(r.revision_number, 0) AS revision_number
		FROM blueprint_share_links l
		LEFT JOIN blueprint_revisions r ON r.id = l.blueprint_revision_id
		WHERE l.blueprint_id = $1 AND l.company_id = $2
		ORDER BY l.created_at DESC
	`
	rows, err := r.db.Query(ctx, query, blueprintID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ShareLinkWithRevision
	for rows.Next() {
		item := ShareLinkWithRevision{}
		err := rows.Scan(
			&item.ID, &item.CompanyID, &item.BlueprintID, &item.BlueprintRevisionID, &item.TokenHash,
			&item.Label, &item.CreatedBy, &item.CreatedAt, &item.RevokedAt, &item.LastAccessedAt,
			&item.RevisionNumber,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
