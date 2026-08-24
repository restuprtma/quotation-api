package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var ErrShareLinkNotFound = errors.New("share link not found")

type ShareLinkRepository struct {
	db *pgxpool.Pool
}

func NewShareLinkRepository(db *pgxpool.Pool) *ShareLinkRepository {
	return &ShareLinkRepository{db: db}
}

func (r *ShareLinkRepository) Create(ctx context.Context, q Execer, link *domain.ShareLink) error {
	query := `
		INSERT INTO blueprint_share_links (
			id, company_id, blueprint_id, blueprint_revision_id, token_hash, label, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, revoked_at, last_accessed_at
	`
	return q.QueryRow(ctx, query,
		link.ID, link.CompanyID, link.BlueprintID, link.BlueprintRevisionID, link.TokenHash, link.Label, link.CreatedBy,
	).Scan(&link.CreatedAt, &link.RevokedAt, &link.LastAccessedAt)
}

func (r *ShareLinkRepository) Revoke(ctx context.Context, q Execer, id, companyID string) error {
	query := `
		UPDATE blueprint_share_links
		SET revoked_at = NOW()
		WHERE id = $1 AND company_id = $2 AND revoked_at IS NULL
	`
	tag, err := q.Exec(ctx, query, id, companyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrShareLinkNotFound
	}
	return nil
}

func (r *ShareLinkRepository) GetByTokenHash(ctx context.Context, tokenHash string) (*domain.ShareLink, error) {
	query := `
		SELECT id, company_id, blueprint_id, blueprint_revision_id, token_hash, label, created_by, created_at, revoked_at, last_accessed_at
		FROM blueprint_share_links
		WHERE token_hash = $1
	`
	link := &domain.ShareLink{}
	err := r.db.QueryRow(ctx, query, tokenHash).Scan(
		&link.ID, &link.CompanyID, &link.BlueprintID, &link.BlueprintRevisionID, &link.TokenHash, &link.Label, &link.CreatedBy, &link.CreatedAt, &link.RevokedAt, &link.LastAccessedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrShareLinkNotFound
	}
	if err == nil {
		// Update last_accessed_at async or directly here if needed
		r.db.Exec(ctx, `UPDATE blueprint_share_links SET last_accessed_at = NOW() WHERE id = $1`, link.ID)
	}
	return link, err
}

func (r *ShareLinkRepository) ListByBlueprint(ctx context.Context, blueprintID, companyID string) ([]domain.ShareLink, error) {
	query := `
		SELECT id, company_id, blueprint_id, blueprint_revision_id, token_hash, label, created_by, created_at, revoked_at, last_accessed_at
		FROM blueprint_share_links
		WHERE blueprint_id = $1 AND company_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, blueprintID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.ShareLink
	for rows.Next() {
		link := domain.ShareLink{}
		err := rows.Scan(
			&link.ID, &link.CompanyID, &link.BlueprintID, &link.BlueprintRevisionID, &link.TokenHash, &link.Label, &link.CreatedBy, &link.CreatedAt, &link.RevokedAt, &link.LastAccessedAt,
		)
		if err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, rows.Err()
}
