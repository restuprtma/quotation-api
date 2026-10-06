package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

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

func (r *Repository) LockAndSignQuotationTx(
	ctx context.Context, tx pgx.Tx, token uuid.UUID,
	signerName, signerEmail, signatureData, ip, userAgent string,
) (*domain.Quotation, *domain.QuotationRevision, error) {
	// 1. Lock quotation master row
	var qID uuid.UUID
	var activeRevID *uuid.UUID
	var qNumber string
	var subject *string
	var clientID uuid.UUID
	var companyID uuid.UUID

	queryLockQ := `
		SELECT id, company_id, client_id, quotation_number, subject, active_published_revision_id
		FROM quotations 
		WHERE public_token = $1 AND deleted_at IS NULL 
		FOR UPDATE
	`
	err := tx.QueryRow(ctx, queryLockQ, token).Scan(&qID, &companyID, &clientID, &qNumber, &subject, &activeRevID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	if activeRevID == nil {
		return nil, nil, ErrNotFound
	}

	// 2. Lock active published revision row
	var revID uuid.UUID
	var revNum int
	var contentHTML *string
	var publishedAt *time.Time
	var existingSignedAt *time.Time

	queryLockRev := `
		SELECT id, revision_number, content_html, published_at, signed_at
		FROM quotation_revisions
		WHERE id = $1 AND quotation_id = $2 AND publication_status = 'published'
		FOR UPDATE
	`
	err = tx.QueryRow(ctx, queryLockRev, *activeRevID, qID).Scan(&revID, &revNum, &contentHTML, &publishedAt, &existingSignedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNotFound
		}
		return nil, nil, err
	}

	// Double-check unsigned condition
	if existingSignedAt != nil {
		return nil, nil, ErrAlreadySigned
	}

	// 3. Compute exact SHA-256 hash of content_html
	htmlStr := ""
	if contentHTML != nil {
		htmlStr = *contentHTML
	}
	hashSum := sha256.Sum256([]byte(htmlStr))
	contentHash := hex.EncodeToString(hashSum[:])

	now := time.Now().UTC()

	// 4. Update revision with signature metadata
	updateRevQuery := `
		UPDATE quotation_revisions
		SET signer_name = $1,
		    signer_email = $2,
		    signature_data = $3,
		    signed_content_hash = $4,
		    signed_at = $5,
		    signed_ip = $6,
		    signed_user_agent = $7,
		    updated_at = $5
		WHERE id = $8 AND signed_at IS NULL
	`
	res, err := tx.Exec(ctx, updateRevQuery, signerName, signerEmail, signatureData, contentHash, now, ip, userAgent, revID)
	if err != nil {
		return nil, nil, err
	}
	if res.RowsAffected() == 0 {
		return nil, nil, ErrAlreadySigned
	}

	// 5. Update quotation master status to 'signed'
	updateQQuery := `
		UPDATE quotations
		SET status = 'signed', updated_at = $1
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, updateQQuery, now, qID); err != nil {
		return nil, nil, err
	}

	// 6. Fetch client display name
	var clientName string
	queryClient := `
		SELECT COALESCE(qco.name, legacy_client.name, '')
		FROM core.clients legacy_client
		LEFT JOIN quotation_client_organizations qco 
		  ON qco.company_id = $1 AND qco.id = $2 AND qco.deleted_at IS NULL
		WHERE legacy_client.id = $2
	`
	_ = tx.QueryRow(ctx, queryClient, companyID, clientID).Scan(&clientName)

	return &domain.Quotation{
			ID:              qID,
			QuotationNumber: qNumber,
			Subject:         subject,
			ClientName:      &clientName,
			Status:          "signed",
		}, &domain.QuotationRevision{
			ID:                revID,
			RevisionNumber:    revNum,
			ContentHTML:       contentHTML,
			PublishedAt:       publishedAt,
			SignerName:        &signerName,
			SignerEmail:       &signerEmail,
			SignatureData:     &signatureData,
			SignedContentHash: &contentHash,
			SignedAt:          &now,
		}, nil
}
