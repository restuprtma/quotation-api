package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

type RevisionRepository struct {
	db *pgxpool.Pool
}

func NewRevisionRepository(db *pgxpool.Pool) *RevisionRepository {
	return &RevisionRepository{db: db}
}

func (r *RevisionRepository) CreateRevision(ctx context.Context, q Execer, rev *domain.QuotationRevision) error {
	query := `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35
		) RETURNING created_at, updated_at
	`
	err := q.QueryRow(ctx, query,
		rev.ID, rev.CompanyID, rev.QuotationID, rev.RevisionNumber, rev.IdempotencyKey, rev.PublicationStatus, rev.GenerationStatus,
		rev.GenerationInput, rev.BudgetMaximum, rev.AllocationSnapshot, rev.RateCardSnapshot, rev.LegalTemplateSnapshot,
		rev.ContentHTML, rev.ContentVersion, rev.ContentHash, rev.AiProvider, rev.AiModel, rev.SkillSnapshot, rev.SafetyEnvelopeVersion,
		rev.LegalContentModified, rev.LegalModifiedBy, rev.LegalModifiedAt, rev.CommercialContentModified,
		rev.CommercialModifiedBy, rev.CommercialModifiedAt, rev.GeneratedAt, rev.PublishedAt, rev.SignerName, rev.SignerEmail,
		rev.SignatureData, rev.SignedContentHash, rev.SignedAt, rev.SignedIP, rev.SignedUserAgent, rev.CreatedBy,
	).Scan(&rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create revision", logger.Err(err))
		return err
	}
	return nil
}

func (r *RevisionRepository) GetActiveRevision(ctx context.Context, quotationID, companyID string) (*domain.QuotationRevision, error) {
	query := `
		SELECT
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by, created_at, updated_at
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2
		ORDER BY revision_number DESC LIMIT 1
	`
	return scanRevisionRow(r.db.QueryRow(ctx, query, quotationID, companyID))
}

func (r *RevisionRepository) UpdateRevision(ctx context.Context, q Execer, rev *domain.QuotationRevision) (*domain.QuotationRevision, error) {
	query := `
		UPDATE quotation_revisions
		SET
			publication_status = $1, generation_status = $2, content_html = $3, content_version = $4, content_hash = $5,
			legal_content_modified = $6, legal_modified_by = $7, legal_modified_at = $8, commercial_content_modified = $9,
			commercial_modified_by = $10, commercial_modified_at = $11, generated_at = $12, published_at = $13, signer_name = $14,
			signer_email = $15, signature_data = $16, signed_content_hash = $17, signed_at = $18, signed_ip = $19, signed_user_agent = $20,
			updated_at = NOW()
		WHERE id = $21 AND company_id = $22
		RETURNING updated_at
	`
	err := q.QueryRow(ctx, query,
		rev.PublicationStatus, rev.GenerationStatus, rev.ContentHTML, rev.ContentVersion, rev.ContentHash,
		rev.LegalContentModified, rev.LegalModifiedBy, rev.LegalModifiedAt, rev.CommercialContentModified,
		rev.CommercialModifiedBy, rev.CommercialModifiedAt, rev.GeneratedAt, rev.PublishedAt, rev.SignerName,
		rev.SignerEmail, rev.SignatureData, rev.SignedContentHash, rev.SignedAt, rev.SignedIP, rev.SignedUserAgent,
		rev.ID, rev.CompanyID,
	).Scan(&rev.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		logger.Error("Failed to update revision", logger.Err(err))
		return nil, err
	}
	return rev, nil
}

func (r *RevisionRepository) ListRevisionsByQuotationID(ctx context.Context, quotationID, companyID string) ([]domain.QuotationRevision, error) {
	query := `
		SELECT
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by, created_at, updated_at
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2
		ORDER BY revision_number ASC
	`
	rows, err := r.db.Query(ctx, query, quotationID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.QuotationRevision
	for rows.Next() {
		rev, err := scanRevisionRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *rev)
	}
	return result, rows.Err()
}

func (r *RevisionRepository) GetSignedRevision(ctx context.Context, quotationID, companyID string) (*domain.QuotationRevision, error) {
	query := `
		SELECT
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by, created_at, updated_at
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2 AND signature_data IS NOT NULL
		ORDER BY revision_number DESC LIMIT 1
	`
	return scanRevisionRow(r.db.QueryRow(ctx, query, quotationID, companyID))
}

func scanRevisionRow(scanner rowScanner) (*domain.QuotationRevision, error) {
	rev := &domain.QuotationRevision{}
	err := scanner.Scan(
		&rev.ID, &rev.CompanyID, &rev.QuotationID, &rev.RevisionNumber, &rev.IdempotencyKey, &rev.PublicationStatus, &rev.GenerationStatus,
		&rev.GenerationInput, &rev.BudgetMaximum, &rev.AllocationSnapshot, &rev.RateCardSnapshot, &rev.LegalTemplateSnapshot,
		&rev.ContentHTML, &rev.ContentVersion, &rev.ContentHash, &rev.AiProvider, &rev.AiModel, &rev.SkillSnapshot, &rev.SafetyEnvelopeVersion,
		&rev.LegalContentModified, &rev.LegalModifiedBy, &rev.LegalModifiedAt, &rev.CommercialContentModified,
		&rev.CommercialModifiedBy, &rev.CommercialModifiedAt, &rev.GeneratedAt, &rev.PublishedAt, &rev.SignerName, &rev.SignerEmail,
		&rev.SignatureData, &rev.SignedContentHash, &rev.SignedAt, &rev.SignedIP, &rev.SignedUserAgent, &rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		return nil, err
	}
	return rev, nil
}
