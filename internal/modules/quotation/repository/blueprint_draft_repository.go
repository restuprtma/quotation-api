package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// BlueprintDraftSnapshot contains the revision, run, and context state required to validate and save a draft section.
type BlueprintDraftSnapshot struct {
	BlueprintID            uuid.UUID
	RevisionID             uuid.UUID
	SectionRunID           uuid.UUID
	CompanyID              uuid.UUID
	SectionType            domain.BlueprintSectionType
	SectionStatus          domain.BlueprintSectionStatus
	ExpectedContentVersion int
	BlueprintTitle         string
	RevisionContent        json.RawMessage
	ContextContent         json.RawMessage
}

// SaveBlueprintDraftParams contains the parameters to persist a section draft atomically.
type SaveBlueprintDraftParams struct {
	Snapshot            BlueprintDraftSnapshot
	SectionContent      json.RawMessage
	SectionValidation   json.RawMessage
	RevisionContent     json.RawMessage
	InvalidatedSections []domain.BlueprintSectionType
	SavedAt             time.Time
}

// BlueprintDraftRepository defines the repository interface for draft operations.
type BlueprintDraftRepository interface {
	LoadBlueprintDraftSnapshot(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*BlueprintDraftSnapshot, error)
	SaveBlueprintDraft(ctx context.Context, params SaveBlueprintDraftParams) (*BlueprintSectionPersistenceResult, error)
}

// LoadBlueprintDraftSnapshot loads the draft revision, section run, and context content needed for draft save validation.
func (r *BlueprintGenerationRepository) LoadBlueprintDraftSnapshot(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*BlueprintDraftSnapshot, error) {
	query := `
		SELECT b.id, b.title, r.id, r.content_json, r.content_version,
		       sr.id, sr.status, cr.content_json
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		JOIN blueprint_section_runs sr ON sr.blueprint_revision_id = r.id AND sr.company_id = b.company_id AND sr.section_type = $3
		JOIN blueprint_section_runs cr ON cr.blueprint_revision_id = r.id AND cr.company_id = b.company_id AND cr.section_type = 'context'
		WHERE b.id = $1 AND b.company_id = $2 AND b.deleted_at IS NULL
		  AND r.company_id = $2 AND r.publication_status = 'draft'
		  AND r.revision_number = b.current_revision
	`
	var bpID uuid.UUID
	var bpTitle string
	var revID uuid.UUID
	var revContent json.RawMessage
	var contentVersion int
	var runID uuid.UUID
	var runStatus string
	var contextContent json.RawMessage

	err := r.db.QueryRow(ctx, query, blueprintID, companyID, string(sectionType)).Scan(
		&bpID, &bpTitle, &revID, &revContent, &contentVersion,
		&runID, &runStatus, &contextContent,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	return &BlueprintDraftSnapshot{
		BlueprintID:            bpID,
		RevisionID:             revID,
		SectionRunID:           runID,
		CompanyID:              companyID,
		SectionType:            sectionType,
		SectionStatus:          domain.BlueprintSectionStatus(runStatus),
		ExpectedContentVersion: contentVersion,
		BlueprintTitle:         bpTitle,
		RevisionContent:        revContent,
		ContextContent:         contextContent,
	}, nil
}

// SaveBlueprintDraft atomically saves a section draft, locks the revision and run FOR UPDATE, verifies content_version CAS,
// marks the run as manual_edit, resets invalidated downstream runs to pending, updates bundle content and increments version.
func (r *BlueprintGenerationRepository) SaveBlueprintDraft(ctx context.Context, params SaveBlueprintDraftParams) (*BlueprintSectionPersistenceResult, error) {
	switch params.Snapshot.SectionType {
	case domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS:
	default:
		return nil, ErrBlueprintManualConflict
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Lock current draft revision FOR UPDATE under tenant scope
	revQuery := `
		SELECT r.id, r.content_version
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		WHERE b.id = $1 AND b.company_id = $2 AND b.deleted_at IS NULL
		  AND r.company_id = $2 AND r.id = $3 AND r.publication_status = 'draft'
		  AND r.revision_number = b.current_revision
		FOR UPDATE OF r
	`
	var revID uuid.UUID
	var revVersion int
	err = tx.QueryRow(ctx, revQuery, params.Snapshot.BlueprintID, params.Snapshot.CompanyID, params.Snapshot.RevisionID).Scan(&revID, &revVersion)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	if revVersion != params.Snapshot.ExpectedContentVersion {
		return nil, ErrBlueprintContentConflict
	}

	// 2. Lock target section run FOR UPDATE
	runQuery := `
		SELECT status, attempt_count
		FROM blueprint_section_runs
		WHERE id = $1 AND company_id = $2 AND blueprint_revision_id = $3
		FOR UPDATE
	`
	var runStatus string
	var runAttempt int
	err = tx.QueryRow(ctx, runQuery, params.Snapshot.SectionRunID, params.Snapshot.CompanyID, revID).Scan(&runStatus, &runAttempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintClaimConflict
		}
		return nil, err
	}

	if runStatus == string(domain.BlueprintSectionGenerating) {
		return nil, ErrBlueprintClaimConflict
	}

	// 3. Update target run -> status 'manual_edit'
	_, err = tx.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'manual_edit', content_json = $1, validation_result = $2,
		    safe_error_code = NULL, updated_at = $3
		WHERE id = $4 AND company_id = $5 AND blueprint_revision_id = $6
	`, params.SectionContent, defaultJSON(params.SectionValidation, "{}"), params.SavedAt, params.Snapshot.SectionRunID, params.Snapshot.CompanyID, revID)
	if err != nil {
		return nil, err
	}

	// 4. Reset invalidated downstream runs to pending/null
	for _, invType := range params.InvalidatedSections {
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'pending', content_json = NULL, validation_result = '{}'::jsonb,
			    safe_error_code = NULL, started_at = NULL, completed_at = NULL, updated_at = $1
			WHERE company_id = $2 AND blueprint_revision_id = $3 AND section_type = $4
		`, params.SavedAt, params.Snapshot.CompanyID, revID, string(invType))
		if err != nil {
			return nil, err
		}
	}

	// 5. Update revision: increment content_version, store content_json, update validation_result, clear cross_validation_result
	newContentVersion := revVersion + 1
	_, err = tx.Exec(ctx, `
		UPDATE blueprint_revisions
		SET content_version = content_version + 1,
		    content_json = $1,
		    validation_result = jsonb_set(COALESCE(validation_result, '{}'::jsonb), ARRAY[$2], $3::jsonb, true),
		    cross_validation_result = '{}'::jsonb,
		    completeness_status = 'partial',
		    generation_status = 'in_progress',
		    updated_at = $4
		WHERE id = $5 AND company_id = $6
	`, params.RevisionContent, string(params.Snapshot.SectionType), defaultJSON(params.SectionValidation, "{}"), params.SavedAt, revID, params.Snapshot.CompanyID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BlueprintSectionPersistenceResult{
		RevisionID:       revID,
		SectionType:      params.Snapshot.SectionType,
		SectionStatus:    domain.BlueprintSectionManualEdit,
		GenerationStatus: domain.BlueprintGenerationInProgress,
		ContentVersion:   newContentVersion,
		Attempt:          runAttempt,
	}, nil
}
