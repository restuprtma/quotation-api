package repository

import (
	"context"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

// CreatePublicArtifact inserts a new public artifact for a revision.
func (r *Repository) CreatePublicArtifact(ctx context.Context, q Execer, artifact *domain.QuotationPublicArtifact) error {
	query := `
		INSERT INTO quotation_public_artifacts (
			id, company_id, revision_id, artifact_type, public_token, published_at, revoked_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at
	`
	err := q.QueryRow(ctx, query,
		artifact.ID, artifact.CompanyID, artifact.RevisionID, artifact.ArtifactType,
		artifact.PublicToken, artifact.PublishedAt, artifact.RevokedAt,
	).Scan(&artifact.CreatedAt)
	if err != nil {
		logger.Error("Failed to create public artifact", logger.Err(err))
		return err
	}
	return nil
}

// GetPublicArtifactsByRevisionID retrieves all public artifacts for a specific revision.
func (r *Repository) GetPublicArtifactsByRevisionID(ctx context.Context, q Execer, revisionID string) ([]domain.QuotationPublicArtifact, error) {
	query := `
		SELECT id, company_id, revision_id, artifact_type, public_token, published_at, revoked_at, created_at
		FROM quotation_public_artifacts
		WHERE revision_id = $1
		ORDER BY created_at ASC
	`
	rows, err := q.Query(ctx, query, revisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.QuotationPublicArtifact
	for rows.Next() {
		var a domain.QuotationPublicArtifact
		if err := rows.Scan(
			&a.ID, &a.CompanyID, &a.RevisionID, &a.ArtifactType,
			&a.PublicToken, &a.PublishedAt, &a.RevokedAt, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// RevokeArtifactsByQuotationID revokes all public artifacts for all revisions
// of a given quotation by setting revoked_at to the current time.
func (r *Repository) RevokeArtifactsByQuotationID(ctx context.Context, q Execer, quotationID string) error {
	query := `
		UPDATE quotation_public_artifacts a
		SET revoked_at = NOW()
		FROM quotation_revisions r
		WHERE a.revision_id = r.id
		  AND r.quotation_id = $1
		  AND a.revoked_at IS NULL
	`
	_, err := q.Exec(ctx, query, quotationID)
	if err != nil {
		logger.Error("Failed to revoke artifacts by quotation ID", logger.Err(err))
		return err
	}
	return nil
}
