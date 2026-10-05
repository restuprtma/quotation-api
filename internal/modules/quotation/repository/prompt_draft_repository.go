package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PromptDraftClaim is the immutable input and catalog snapshot owned by one
// processing attempt. The claim is made before an AI provider is called.
type PromptDraftClaim struct {
	QuotationID      uuid.UUID
	RevisionID       uuid.UUID
	QuotationNumber  string
	GenerationInput  json.RawMessage
	RateCardSnapshot json.RawMessage
}

// ClaimPromptDraft transitions the current draft revision from pending (or a
// previously failed attempt) to in_progress under row locks. Completed and
// in-progress revisions are deliberately not mutated.
func (r *GenerationRepository) ClaimPromptDraft(ctx context.Context, companyID, quotationID uuid.UUID) (*PromptDraftClaim, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	claim := &PromptDraftClaim{}
	var status string
	err = tx.QueryRow(ctx, `
		SELECT q.id, r.id, q.quotation_number,
		       COALESCE(r.generation_input, '{}'::jsonb),
		       COALESCE(r.rate_card_snapshot, 'null'::jsonb),
		       r.generation_status
		FROM quotations q
		JOIN quotation_revisions r
		  ON r.quotation_id = q.id
		 AND r.revision_number = q.current_revision
		WHERE q.id = $1 AND q.company_id = $2 AND r.company_id = $2
		  AND q.deleted_at IS NULL AND r.publication_status = $3
		FOR UPDATE OF q, r
	`, quotationID, companyID, PublicationStatusDraft).Scan(
		&claim.QuotationID, &claim.RevisionID, &claim.QuotationNumber,
		&claim.GenerationInput, &claim.RateCardSnapshot, &status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRevisionNotFound
	}
	if err != nil {
		return nil, err
	}

	switch status {
	case GenerationStatusCompleted:
		return nil, ErrGenerationAlreadyCompleted
	case GenerationStatusInProgress:
		return nil, ErrGenerationInProgress
	case GenerationStatusPending, GenerationStatusFailed:
		// Claimable states.
	default:
		return nil, ErrRevisionConflict
	}

	result, err := tx.Exec(ctx, `
		UPDATE quotation_revisions
		SET generation_status = $1,
		    generation_error = NULL,
		    updated_at = NOW()
		WHERE id = $2 AND quotation_id = $3 AND company_id = $4
	`, GenerationStatusInProgress, claim.RevisionID, claim.QuotationID, companyID)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() != 1 {
		return nil, ErrRevisionNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return claim, nil
}

// FailPromptDraft records only a safe, client-facing error code and marks the
// quotation aggregate as failed in the same SQL statement.
func (r *GenerationRepository) FailPromptDraft(ctx context.Context, companyID, quotationID, revisionID uuid.UUID, safeCode string) error {
	code := safeGenerationFailureCode(safeCode)
	result, err := r.db.Exec(ctx, `
		WITH updated_rev AS (
			UPDATE quotation_revisions
			SET generation_status = $1,
			    generation_error = $2,
			    updated_at = NOW()
			WHERE id = $3 AND quotation_id = $4 AND company_id = $5
			RETURNING quotation_id
		)
		UPDATE quotations q
		SET status = $6, updated_at = NOW()
		FROM updated_rev r
		WHERE q.id = r.quotation_id AND q.company_id = $5
	`, GenerationStatusFailed, code, revisionID, quotationID, companyID, QuotationStatusFailed)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrRevisionNotFound
	}
	return nil
}

func safeGenerationFailureCode(code string) string {
	code = strings.TrimSpace(code)
	switch code {
	case "provider_timeout", "provider_rate_limited", "provider_invalid_credential",
		"settings_incomplete", "meeting_notes_no_business_signal", "unsafe_html",
		"generation_failed", "generation_in_progress":
		return code
	default:
		if strings.HasPrefix(code, "invalid_extraction.") {
			return code
		}
		return "generation_failed"
	}
}

// safeGenerationErrorCode makes legacy error text safe before it leaves the
// repository read model. New failure writes already go through the allowlist.
func safeGenerationErrorCode(code string) string {
	if strings.TrimSpace(code) == "" {
		return ""
	}
	return safeGenerationFailureCode(code)
}

// PromptDraftSnapshots is the complete set of persisted artefacts that must
// move together with prompt state. Nil JSON values intentionally remain NULL.
type PromptDraftSnapshots struct {
	AllocationSnapshot    json.RawMessage
	RateCardSnapshot      json.RawMessage
	LegalTemplateSnapshot json.RawMessage
	SkillSnapshot         json.RawMessage
	ResolvedProfile       json.RawMessage
	Calculation           json.RawMessage
	Module                json.RawMessage
	Timeline              json.RawMessage
	SectionHashes         json.RawMessage
	ContentHash           *string
}

type UpdatePromptDraftCASParams struct {
	CompanyID              uuid.UUID
	QuotationID            uuid.UUID
	RevisionID             uuid.UUID
	ExpectedContentVersion int
	ExpectedStateVersion   int64
	State                  json.RawMessage
	ContentHTML            string
	Snapshots              PromptDraftSnapshots
}

type PromptDraftSaveResult struct {
	RevisionID     uuid.UUID
	ContentVersion int
	StateVersion   int64
	UpdatedAt      time.Time
}

type CompletePromptDraftParams struct {
	CompanyID               uuid.UUID
	QuotationID             uuid.UUID
	RevisionID              uuid.UUID
	ContentHTML             string
	PromptState             json.RawMessage
	PromptSectionHashes     json.RawMessage
	RateCardSnapshot        json.RawMessage
	LegalTemplateSnapshot   json.RawMessage
	SkillSnapshot           json.RawMessage
	ResolvedProfileSnapshot json.RawMessage
	CalculationSnapshot     json.RawMessage
	ModuleSnapshot          json.RawMessage
	TimelineSnapshot        json.RawMessage
	ContentHash             *string
}

// CompletePromptDraft writes the prompt state and deterministic artefacts in
// the same transaction as the revision/quotation completion status. It is
// used after extraction and calculation so incomplete drafts are durable too.
func (r *GenerationRepository) CompletePromptDraft(ctx context.Context, params CompletePromptDraftParams) (*PromptDraftSaveResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	result := &PromptDraftSaveResult{RevisionID: params.RevisionID}
	err = tx.QueryRow(ctx, `
		WITH updated_rev AS (
			UPDATE quotation_revisions
			SET prompt_state = $1,
			    prompt_state_version = prompt_state_version + 1,
			    prompt_section_hashes = $2,
			    content_html = $3,
			    content_hash = $4,
			    rate_card_snapshot = $5,
			    legal_template_snapshot = $6,
			    skill_snapshot = $7,
			    resolved_profile_snapshot = $8,
			    calculation_snapshot = $9,
			    module_snapshot = $10,
			    timeline_snapshot = $11,
			    generation_status = $12,
			    generation_error = NULL,
			    generated_at = NOW(),
			    updated_at = NOW()
			WHERE id = $13 AND company_id = $14 AND quotation_id = $15
			  AND publication_status = $16
			RETURNING content_version, prompt_state_version, updated_at
		)
		UPDATE quotations q
		SET status = $17, updated_at = NOW()
		FROM updated_rev r
		WHERE q.id = $15 AND q.company_id = $14
		RETURNING r.content_version, r.prompt_state_version, r.updated_at
	`, params.PromptState, params.PromptSectionHashes, params.ContentHTML, params.ContentHash,
		params.RateCardSnapshot, params.LegalTemplateSnapshot, params.SkillSnapshot,
		params.ResolvedProfileSnapshot, params.CalculationSnapshot, params.ModuleSnapshot,
		params.TimelineSnapshot, GenerationStatusCompleted, params.RevisionID, params.CompanyID,
		params.QuotationID, PublicationStatusDraft, PublicationStatusDraft).Scan(&result.ContentVersion, &result.StateVersion, &result.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRevisionConflict
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

type PromptDraftRecord struct {
	CompanyID             uuid.UUID
	QuotationID           uuid.UUID
	RevisionID            uuid.UUID
	QuotationNumber       string
	GenerationStatus      string
	ContentHTML           string
	ContentVersion        int
	StateVersion          int64
	PromptState           json.RawMessage
	PromptSections        json.RawMessage
	Calculation           json.RawMessage
	AllocationSnapshot    json.RawMessage
	RateCardSnapshot      json.RawMessage
	LegalTemplateSnapshot json.RawMessage
	SkillSnapshot         json.RawMessage
	ResolvedProfile       json.RawMessage
	Module                json.RawMessage
	Timeline              json.RawMessage
}

func (r *GenerationRepository) LoadPromptDraft(ctx context.Context, companyID, quotationID uuid.UUID) (*PromptDraftRecord, error) {
	var record PromptDraftRecord
	err := r.db.QueryRow(ctx, `
		SELECT q.company_id, q.id, r.id, q.quotation_number, r.generation_status, r.content_html, r.content_version,
		       r.prompt_state_version, r.prompt_state, r.prompt_section_hashes,
		       COALESCE(r.calculation_snapshot, 'null'::jsonb),
		       COALESCE(r.allocation_snapshot, 'null'::jsonb), COALESCE(r.rate_card_snapshot, 'null'::jsonb),
		       COALESCE(r.legal_template_snapshot, 'null'::jsonb), COALESCE(r.skill_snapshot, 'null'),
		       COALESCE(r.resolved_profile_snapshot, 'null'::jsonb), COALESCE(r.module_snapshot, '[]'::jsonb),
		       COALESCE(r.timeline_snapshot, '[]'::jsonb)
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id AND r.revision_number = q.current_revision
		WHERE q.id = $1 AND q.company_id = $2 AND r.company_id = $2
		  AND q.deleted_at IS NULL AND r.publication_status = $3
	`, quotationID, companyID, PublicationStatusDraft).Scan(
		&record.CompanyID, &record.QuotationID, &record.RevisionID, &record.QuotationNumber, &record.GenerationStatus,
		&record.ContentHTML, &record.ContentVersion, &record.StateVersion,
		&record.PromptState, &record.PromptSections, &record.Calculation,
		&record.AllocationSnapshot, &record.RateCardSnapshot, &record.LegalTemplateSnapshot,
		&record.SkillSnapshot, &record.ResolvedProfile, &record.Module, &record.Timeline)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRevisionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// UpdatePromptDraftCAS serializes prompt-first writers on the current draft
// revision. It checks both optimistic-lock versions while holding row locks,
// then commits state, snapshots, HTML and hashes as one unit.
func (r *GenerationRepository) UpdatePromptDraftCAS(ctx context.Context, params UpdatePromptDraftCASParams) (*PromptDraftSaveResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var currentRevision int
	var contentVersion int
	var stateVersion int64
	var publicationStatus string
	err = tx.QueryRow(ctx, `
		SELECT q.current_revision, r.content_version, r.prompt_state_version, r.publication_status
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id
		WHERE q.id = $1 AND q.company_id = $2 AND q.deleted_at IS NULL
		  AND r.id = $3 AND r.company_id = $2
		  AND r.revision_number = q.current_revision
		FOR UPDATE OF q, r
	`, params.QuotationID, params.CompanyID, params.RevisionID).Scan(&currentRevision, &contentVersion, &stateVersion, &publicationStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRevisionConflict
	}
	if err != nil {
		return nil, err
	}
	if publicationStatus != PublicationStatusDraft || currentRevision < 1 ||
		contentVersion != params.ExpectedContentVersion || stateVersion != params.ExpectedStateVersion {
		return nil, ErrRevisionConflict
	}

	result := &PromptDraftSaveResult{RevisionID: params.RevisionID}
	err = tx.QueryRow(ctx, `
		UPDATE quotation_revisions
		SET prompt_state = $1,
		    prompt_state_version = prompt_state_version + 1,
		    prompt_section_hashes = $2,
		    content_html = $3,
		    content_hash = $4,
		    allocation_snapshot = $5,
		    rate_card_snapshot = $6,
		    legal_template_snapshot = $7,
		    skill_snapshot = $8,
		    resolved_profile_snapshot = $9,
		    calculation_snapshot = $10,
		    module_snapshot = $11,
		    timeline_snapshot = $12,
		    content_version = content_version + 1,
		    updated_at = NOW()
		WHERE id = $13 AND company_id = $14
		RETURNING content_version, prompt_state_version, updated_at
	`, params.State, params.Snapshots.SectionHashes, params.ContentHTML, params.Snapshots.ContentHash,
		params.Snapshots.AllocationSnapshot, params.Snapshots.RateCardSnapshot,
		params.Snapshots.LegalTemplateSnapshot, params.Snapshots.SkillSnapshot,
		params.Snapshots.ResolvedProfile, params.Snapshots.Calculation,
		params.Snapshots.Module, params.Snapshots.Timeline,
		params.RevisionID, params.CompanyID).Scan(&result.ContentVersion, &result.StateVersion, &result.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
