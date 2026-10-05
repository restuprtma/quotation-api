package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// P5FinalizationArtifacts is deliberately a repository DTO.  It allows the
// service finalizer to compose the scheme change and completed revision write
// in exactly one PostgreSQL transaction without creating a service/repository
// import cycle.
type P5FinalizationArtifacts struct {
	ContentHTML             string
	ResolvedProfileSnapshot json.RawMessage
	CalculationSnapshot     json.RawMessage
	ModuleSnapshot          json.RawMessage
	TimelineSnapshot        json.RawMessage
	ContentHash             *string
}

type P5FinalizeCommit struct {
	CompanyID    uuid.UUID
	QuotationID  uuid.UUID
	RevisionID   uuid.UUID
	Scheme       string
	SolutionType string
	PromptState  json.RawMessage
	Artifacts    P5FinalizationArtifacts
}
type P5FinalizeCommitResult struct{ AlreadyCompleted bool }

// IsCompleted is intentionally a fast idempotency read before rendering. The
// commit method repeats the check while holding row locks, so a concurrent
// worker cannot create a second committed output after this read.
func (r *GenerationRepository) IsCompleted(ctx context.Context, companyID, quotationID, revisionID uuid.UUID) (bool, error) {
	var status string
	err := r.db.QueryRow(ctx, `SELECT generation_status FROM quotation_revisions WHERE id=$1 AND quotation_id=$2 AND company_id=$3`, revisionID, quotationID, companyID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrRevisionNotFound
	}
	if err != nil {
		return false, err
	}
	return status == GenerationStatusCompleted, nil
}

// FinalizeP5 atomically replaces the temporary dedicated scheme with the
// selected valid scheme and commits all deterministic artefacts. A replay that
// races after a successful commit is a harmless AlreadyCompleted result.
func (r *GenerationRepository) FinalizeP5(ctx context.Context, params P5FinalizeCommit) (P5FinalizeCommitResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return P5FinalizeCommitResult{}, err
	}
	defer tx.Rollback(ctx)
	var status, publication string
	err = tx.QueryRow(ctx, `SELECT r.generation_status, r.publication_status FROM quotations q JOIN quotation_revisions r ON r.quotation_id=q.id WHERE q.id=$1 AND q.company_id=$2 AND r.id=$3 AND r.company_id=$2 AND r.revision_number=q.current_revision FOR UPDATE OF q, r`, params.QuotationID, params.CompanyID, params.RevisionID).Scan(&status, &publication)
	if errors.Is(err, pgx.ErrNoRows) {
		return P5FinalizeCommitResult{}, ErrRevisionNotFound
	}
	if err != nil {
		return P5FinalizeCommitResult{}, err
	}
	if status == GenerationStatusCompleted {
		return P5FinalizeCommitResult{AlreadyCompleted: true}, tx.Commit(ctx)
	}
	if publication != PublicationStatusDraft {
		return P5FinalizeCommitResult{}, ErrRevisionConflict
	}
	_, err = tx.Exec(ctx, `UPDATE quotations SET commercial_scheme=$1, solution_type=$2, status=$3, updated_at=NOW() WHERE id=$4 AND company_id=$5`, params.Scheme, params.SolutionType, PublicationStatusDraft, params.QuotationID, params.CompanyID)
	if err != nil {
		return P5FinalizeCommitResult{}, err
	}
	result, err := tx.Exec(ctx, `UPDATE quotation_revisions SET prompt_state=$1, prompt_state_version=prompt_state_version+1, content_html=$2, content_hash=$3, resolved_profile_snapshot=$4, calculation_snapshot=$5, module_snapshot=$6, timeline_snapshot=$7, generation_status=$8, generation_error=NULL, generated_at=NOW(), updated_at=NOW() WHERE id=$9 AND quotation_id=$10 AND company_id=$11 AND publication_status=$12`, params.PromptState, params.Artifacts.ContentHTML, params.Artifacts.ContentHash, params.Artifacts.ResolvedProfileSnapshot, params.Artifacts.CalculationSnapshot, params.Artifacts.ModuleSnapshot, params.Artifacts.TimelineSnapshot, GenerationStatusCompleted, params.RevisionID, params.QuotationID, params.CompanyID, PublicationStatusDraft)
	if err != nil {
		return P5FinalizeCommitResult{}, err
	}
	if result.RowsAffected() != 1 {
		return P5FinalizeCommitResult{}, ErrRevisionConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return P5FinalizeCommitResult{}, err
	}
	return P5FinalizeCommitResult{}, nil
}
