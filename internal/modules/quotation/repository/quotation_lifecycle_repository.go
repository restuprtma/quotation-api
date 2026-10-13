package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

const (
	PublicationStatusPublished  = "published"
	PublicationStatusSuperseded = "superseded"
)

// LockQuotationMaster locks the quotation row using SELECT ... FOR UPDATE to serialize revision publishing.
func (r *Repository) LockQuotationMaster(ctx context.Context, tx pgx.Tx, quotationID string) error {
	q := r.getExecer(tx)
	var id string
	err := q.QueryRow(ctx, "SELECT id FROM quotations WHERE id = $1 FOR UPDATE", quotationID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// MarkPublishedRevisionSuperseded updates the current published revision for the quotation to 'superseded'.
func (r *Repository) MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, quotationID string) error {
	q := r.getExecer(tx)
	_, err := q.Exec(ctx, `
		UPDATE quotation_revisions 
		SET publication_status = 'superseded', updated_at = NOW() 
		WHERE quotation_id = $1 AND publication_status = 'published'
	`, quotationID)
	return err
}

// PublishDraftQuotation marks the draft revision as published and updates quotation master active published revision and status.
func (r *Repository) PublishDraftQuotation(ctx context.Context, tx pgx.Tx, quotationID, draftID string) error {
	q := r.getExecer(tx)

	res, err := q.Exec(ctx, `
		UPDATE quotation_revisions 
		SET publication_status = 'published', published_at = NOW(), updated_at = NOW() 
		WHERE id = $1 AND publication_status = 'draft'
	`, draftID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrRevisionNotFound
	}

	res, err = q.Exec(ctx, `
		UPDATE quotations 
		SET active_published_revision_id = $1, status = 'published', updated_at = NOW() 
		WHERE id = $2
	`, draftID, quotationID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// PublishDraftQuotationWithSnapshots marks the draft revision as published, updates content_html with synced dates and director signature, saves signature and stamp snapshots, and updates quotation master active published revision and status.
func (r *Repository) PublishDraftQuotationWithSnapshots(ctx context.Context, tx pgx.Tx, quotationID, draftID string, updatedHTML, dirSig, stamp string) error {
	q := r.getExecer(tx)

	res, err := q.Exec(ctx, `
		UPDATE quotation_revisions 
		SET publication_status = 'published', published_at = NOW(), updated_at = NOW(),
		    content_html = $2, director_signature_snapshot = $3, director_stamp_snapshot = $4
		WHERE id = $1 AND publication_status = 'draft'
	`, draftID, updatedHTML, dirSig, stamp)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrRevisionNotFound
	}

	res, err = q.Exec(ctx, `
		UPDATE quotations 
		SET active_published_revision_id = $1, status = 'published', updated_at = NOW() 
		WHERE id = $2
	`, draftID, quotationID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// GetMaxRevisionNumber returns the highest revision number for a quotation, defaulting to 0 if none exist.
func (r *Repository) GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, quotationID string) (int, error) {
	q := r.getExecer(tx)
	var maxRev int
	err := q.QueryRow(ctx, "SELECT COALESCE(MAX(revision_number), 0) FROM quotation_revisions WHERE quotation_id = $1", quotationID).Scan(&maxRev)
	if err != nil {
		return 0, err
	}
	return maxRev, nil
}

// GetDraftRevision retrieves the current draft revision for a quotation.
func (r *Repository) GetDraftRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error) {
	q := r.getExecer(tx)
	query := `
		SELECT
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			prompt_state, prompt_state_version, prompt_section_hashes,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by, created_at, updated_at
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2 AND publication_status = 'draft'
		ORDER BY revision_number DESC
		LIMIT 1
	`
	return scanRevisionRow(q.QueryRow(ctx, query, quotationID, companyID))
}

// GetActivePublishedRevision retrieves the current active published revision for a quotation.
func (r *Repository) GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error) {
	q := r.getExecer(tx)
	query := `
		SELECT
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			prompt_state, prompt_state_version, prompt_section_hashes,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by, created_at, updated_at
		FROM quotation_revisions
		WHERE quotation_id = $1 AND company_id = $2 AND publication_status = 'published'
		ORDER BY revision_number DESC
		LIMIT 1
	`
	return scanRevisionRow(q.QueryRow(ctx, query, quotationID, companyID))
}

// CreateRevisionTx inserts a new revision under transaction (or pool fallback).
func (r *Repository) CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.QuotationRevision) error {
	if rev.ID == uuid.Nil {
		rev.ID = uuid.New()
	}
	if rev.PublicationStatus == "" {
		rev.PublicationStatus = PublicationStatusDraft
	}
	if rev.GenerationStatus == "" {
		rev.GenerationStatus = GenerationStatusPending
	}
	if len(rev.GenerationInput) == 0 {
		rev.GenerationInput = []byte("{}")
	}
	if rev.ContentVersion == nil {
		v := "1"
		rev.ContentVersion = &v
	}
	if rev.ContentHTML == nil {
		h := ""
		rev.ContentHTML = &h
	}

	query := `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			prompt_state, prompt_state_version, prompt_section_hashes,
			content_html, content_version, content_hash, ai_provider, ai_model, skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37,
			$38
		) RETURNING created_at, updated_at
	`
	q := r.getExecer(tx)
	err := q.QueryRow(ctx, query,
		rev.ID, rev.CompanyID, rev.QuotationID, rev.RevisionNumber, rev.IdempotencyKey, rev.PublicationStatus, rev.GenerationStatus,
		rev.GenerationInput, rev.BudgetMaximum, rev.AllocationSnapshot, rev.RateCardSnapshot, rev.LegalTemplateSnapshot,
		rev.PromptState, rev.PromptStateVersion, rev.PromptSectionHashes,
		rev.ContentHTML, rev.ContentVersion, rev.ContentHash, rev.AiProvider, rev.AiModel, rev.SkillSnapshot, rev.SafetyEnvelopeVersion,
		rev.LegalContentModified, rev.LegalModifiedBy, rev.LegalModifiedAt, rev.CommercialContentModified,
		rev.CommercialModifiedBy, rev.CommercialModifiedAt, rev.GeneratedAt, rev.PublishedAt, rev.SignerName, rev.SignerEmail,
		rev.SignatureData, rev.SignedContentHash, rev.SignedAt, rev.SignedIP, rev.SignedUserAgent, rev.CreatedBy,
	).Scan(&rev.CreatedAt, &rev.UpdatedAt)
	if err != nil {
		logger.Error("Failed to create revision in tx", logger.Err(err))
		return err
	}
	return nil
}

// UpdateQuotationCurrentRevision updates the current revision pointer on the quotation master row.
func (r *Repository) UpdateQuotationCurrentRevision(ctx context.Context, tx pgx.Tx, quotationID string, currentRevision int) error {
	q := r.getExecer(tx)
	res, err := q.Exec(ctx, `
		UPDATE quotations 
		SET current_revision = $1, updated_at = NOW() 
		WHERE id = $2
	`, currentRevision, quotationID)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) getExecer(tx pgx.Tx) Execer {
	if tx != nil {
		return tx
	}
	return r.db
}
