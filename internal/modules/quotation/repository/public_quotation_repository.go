package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func (r *Repository) GetPublicQuotationByToken(ctx context.Context, token uuid.UUID) (*domain.Quotation, *domain.QuotationRevision, error) {
	query := `
		SELECT 
			q.id, q.company_id, q.client_id, q.quotation_number, q.subject, q.solution_type,
			q.commercial_scheme, q.status, q.current_revision, q.public_token,
			q.active_published_revision_id, q.created_by, q.created_at, q.updated_at, q.deleted_at,
			COALESCE(qco.name, legacy_client.name, '') AS client_name,
			qr.id, qr.company_id, qr.quotation_id, qr.revision_number, qr.idempotency_key,
			qr.publication_status, qr.generation_status, qr.generation_input, qr.budget_maximum,
			qr.allocation_snapshot, qr.rate_card_snapshot, qr.legal_template_snapshot,
			qr.prompt_state, qr.prompt_state_version, qr.prompt_section_hashes,
			qr.content_html, qr.content_version, qr.content_hash, qr.ai_provider, qr.ai_model,
			qr.skill_snapshot, qr.safety_envelope_version, qr.legal_content_modified,
			qr.legal_modified_by, qr.legal_modified_at, qr.commercial_content_modified,
			qr.commercial_modified_by, qr.commercial_modified_at, qr.generated_at,
			qr.published_at, qr.signer_name, qr.signer_email, qr.signature_data,
			qr.signed_content_hash, qr.signed_at, qr.signed_ip, qr.signed_user_agent,
			qr.created_by, qr.created_at, qr.updated_at
		FROM quotations q
		JOIN quotation_revisions qr 
		  ON qr.id = q.active_published_revision_id 
		 AND qr.quotation_id = q.id 
		 AND qr.publication_status = 'published'
		LEFT JOIN quotation_client_organizations qco 
		  ON qco.company_id = q.company_id 
		 AND qco.id = q.client_id 
		 AND qco.deleted_at IS NULL
		LEFT JOIN core.clients legacy_client 
		  ON legacy_client.id = q.client_id
		WHERE q.public_token = $1 AND q.deleted_at IS NULL
	`

	row := r.db.QueryRow(ctx, query, token)

	quot := &domain.Quotation{}
	rev := &domain.QuotationRevision{}

	err := row.Scan(
		&quot.ID, &quot.CompanyID, &quot.ClientID, &quot.QuotationNumber, &quot.Subject, &quot.SolutionType,
		&quot.CommercialScheme, &quot.Status, &quot.CurrentRevision, &quot.PublicToken,
		&quot.ActivePublishedRevisionID, &quot.CreatedBy, &quot.CreatedAt, &quot.UpdatedAt, &quot.DeletedAt,
		&quot.ClientName,
		&rev.ID, &rev.CompanyID, &rev.QuotationID, &rev.RevisionNumber, &rev.IdempotencyKey,
		&rev.PublicationStatus, &rev.GenerationStatus, &rev.GenerationInput, &rev.BudgetMaximum,
		&rev.AllocationSnapshot, &rev.RateCardSnapshot, &rev.LegalTemplateSnapshot,
		&rev.PromptState, &rev.PromptStateVersion, &rev.PromptSectionHashes,
		&rev.ContentHTML, &rev.ContentVersion, &rev.ContentHash, &rev.AiProvider, &rev.AiModel,
		&rev.SkillSnapshot, &rev.SafetyEnvelopeVersion, &rev.LegalContentModified,
		&rev.LegalModifiedBy, &rev.LegalModifiedAt, &rev.CommercialContentModified,
		&rev.CommercialModifiedBy, &rev.CommercialModifiedAt, &rev.GeneratedAt,
		&rev.PublishedAt, &rev.SignerName, &rev.SignerEmail, &rev.SignatureData,
		&rev.SignedContentHash, &rev.SignedAt, &rev.SignedIP, &rev.SignedUserAgent,
		&rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	return quot, rev, nil
}
