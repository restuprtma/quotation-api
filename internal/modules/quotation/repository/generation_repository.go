package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/pkg/logger"
)

// Generation lifecycle constants shared by Session 4 orchestration and
// Session 5 regeneration. Quotation rows are created as `queued`; the draft
// revision starts as `pending` until the pipeline persists a result.
const (
	QuotationStatusQueued     = "queued"
	QuotationStatusFailed     = "failed"
	GenerationStatusPending   = "pending"
	GenerationStatusCompleted = "completed"
	GenerationStatusFailed    = "failed"

	PublicationStatusDraft = "draft"

	pgUniqueViolationCode       = "23505"
	uqRevisionsIdempotencyIndex = "uq_revisions_idempotency_key"

	maxCreateDraftAttempts = 3
)

// GenerationRepository owns persistence for the AI-first quotation
// generation flow: atomic draft creation with idempotency keys and
// completion updates after the pipeline finishes.
type GenerationRepository struct {
	db *pgxpool.Pool
}

func NewGenerationRepository(db *pgxpool.Pool) *GenerationRepository {
	return &GenerationRepository{db: db}
}

// CreateDraftParams carries everything required to atomically create the
// quotation master plus its Draft Revision 1.
type CreateDraftParams struct {
	CompanyID             uuid.UUID
	ClientID              *uuid.UUID
	ClientName            string
	QuotationNumber       string
	Subject               string
	SolutionType          string
	CommercialScheme      string
	IdempotencyKey        string // must be a valid UUID (column type is uuid)
	GenerationInput       json.RawMessage
	BudgetMaximum         *int64
	RateCardSnapshot      json.RawMessage
	LegalTemplateSnapshot json.RawMessage
	SkillSnapshot         json.RawMessage
	SafetyEnvelopeVersion string
	CreatedBy             *uuid.UUID
}

// CreateDraftResult reports the committed aggregate. When IdempotentReplay is
// true the fields describe the aggregate created by an earlier request with
// the same (company_id, idempotency_key), not a new insert.
type CreateDraftResult struct {
	QuotationID      uuid.UUID
	RevisionID       uuid.UUID
	PublicToken      uuid.UUID
	IdempotentReplay bool
}

// UpdateGenerationResultParams finalises a pending revision after the
// generation pipeline succeeds.
type UpdateGenerationResultParams struct {
	ContentHTML           string
	AllocationSnapshot    json.RawMessage
	RateCardSnapshot      json.RawMessage
	LegalTemplateSnapshot json.RawMessage
	SkillSnapshot         json.RawMessage
	AiProvider            string
	AiModel               string
	ContentHash           *string
	SafetyEnvelopeVersion string
}

// CreateDraftTransaction inserts the quotation master (status queued, stable
// DB-generated public token) and its Draft Revision 1 (generation pending,
// publication draft) inside a single ACID transaction, committing identifiers
// before returning so callers may safely invoke external providers afterwards.
//
// Concurrent safety: a duplicate (company_id, idempotency_key) violates the
// partial unique index uq_revisions_idempotency_key at INSERT time. Instead of
// surfacing the raw constraint error, the transaction is rolled back and the
// previously committed aggregate is looked up and returned with
// IdempotentReplay=true, guaranteeing one quotation per key even when many
// requests race.
func (r *GenerationRepository) CreateDraftTransaction(ctx context.Context, params CreateDraftParams) (*CreateDraftResult, error) {
	keyUUID, err := uuid.Parse(params.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency key must be a valid UUID: %w", err)
	}
	if err := validateCreateDraftParams(params); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < maxCreateDraftAttempts; attempt++ {
		result, err := r.tryCreateDraft(ctx, params, keyUUID)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !isIdempotencyConflict(err) {
			logger.Error("Failed to create generation draft", logger.Err(err))
			return nil, err
		}

		// Another request committed first. Its row is durable by the time our
		// INSERT failed, so exactly one lookup resolves the winner.
		existing, lookupErr := r.findResultByKey(ctx, params.CompanyID, keyUUID)
		if lookupErr != nil {
			logger.Error("Failed to look up idempotent draft", logger.Err(lookupErr))
			return nil, lookupErr
		}
		if existing != nil {
			existing.IdempotentReplay = true
			return existing, nil
		}
		// Practically unreachable (a violation implies a committed winner);
		// retried defensively within maxCreateDraftAttempts.
	}
	return nil, fmt.Errorf("idempotency conflict unresolved after %d attempts: %w", maxCreateDraftAttempts, lastErr)
}

func (r *GenerationRepository) tryCreateDraft(ctx context.Context, params CreateDraftParams, keyUUID uuid.UUID) (*CreateDraftResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	var clientID uuid.UUID
	if params.ClientID != nil && *params.ClientID != uuid.Nil {
		clientID = *params.ClientID
	} else if params.ClientName != "" {
		repo := NewRepository(r.db)
		idStr, err := repo.FindClientByName(ctx, tx, params.ClientName)
		if err != nil {
			return nil, err
		}
		if idStr != nil {
			clientID = uuid.MustParse(*idStr)
		} else {
			newIDStr, err := repo.CreateClientByName(ctx, tx, params.ClientName, params.CreatedBy)
			if err != nil {
				return nil, err
			}
			clientID = uuid.MustParse(newIDStr)
		}
	}

	quotation := &domain.Quotation{
		ID:               uuid.New(),
		CompanyID:        params.CompanyID,
		ClientID:         clientID,
		QuotationNumber:  params.QuotationNumber,
		Subject:          &params.Subject,
		SolutionType:     params.SolutionType,
		CommercialScheme: params.CommercialScheme,
		Status:           QuotationStatusQueued,
		CurrentRevision:  1,
		CreatedBy:        params.CreatedBy,
	}
	if err = r.insertDraftQuotation(ctx, tx, quotation); err != nil {
		return nil, err
	}

	revisionID := uuid.New()
	err = tx.QueryRow(ctx, `
		INSERT INTO quotation_revisions (
			id, company_id, quotation_id, revision_number, idempotency_key,
			publication_status, generation_status, generation_input, budget_maximum,
			rate_card_snapshot, legal_template_snapshot, skill_snapshot,
			safety_envelope_version, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id
	`,
		revisionID, params.CompanyID, quotation.ID, 1, keyUUID,
		PublicationStatusDraft, GenerationStatusPending, params.GenerationInput, params.BudgetMaximum,
		params.RateCardSnapshot, params.LegalTemplateSnapshot, params.SkillSnapshot,
		params.SafetyEnvelopeVersion, params.CreatedBy,
	).Scan(&revisionID)
	if err != nil {
		return nil, err
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateDraftResult{
		QuotationID: quotation.ID,
		RevisionID:  revisionID,
		PublicToken: quotation.PublicToken,
	}, nil
}

func (r *GenerationRepository) insertDraftQuotation(ctx context.Context, q Execer, quot *domain.Quotation) error {
	query := `
		INSERT INTO quotations (
			id, company_id, client_id, quotation_number, subject, solution_type,
			commercial_scheme, status, current_revision, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, public_token
	`
	err := q.QueryRow(ctx, query,
		quot.ID, quot.CompanyID, quot.ClientID, quot.QuotationNumber, quot.Subject,
		quot.SolutionType, quot.CommercialScheme, quot.Status, quot.CurrentRevision, quot.CreatedBy,
	).Scan(&quot.ID, &quot.PublicToken)
	if err != nil {
		return err
	}
	return nil
}

func validateCreateDraftParams(params CreateDraftParams) error {
	switch {
	case params.CompanyID == uuid.Nil:
		return fmt.Errorf("company_id is required")
	case (params.ClientID == nil || *params.ClientID == uuid.Nil) && params.ClientName == "":
		return fmt.Errorf("client_id or client_name is required")
	case params.QuotationNumber == "":
		return fmt.Errorf("quotation_number is required")
	case params.SolutionType == "":
		return fmt.Errorf("solution_type is required")
	case params.CommercialScheme == "":
		return fmt.Errorf("commercial_scheme is required")
	case len(params.GenerationInput) == 0:
		return fmt.Errorf("generation_input is required")
	default:
		return nil
	}
}

func isIdempotencyConflict(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolationCode &&
			pgErr.ConstraintName == uqRevisionsIdempotencyIndex
	}
	return false
}

// findResultByKey loads the aggregate committed for (company_id, key) on a
// fresh connection, usable after the caller's conflicting transaction rolled back.
func (r *GenerationRepository) findResultByKey(ctx context.Context, companyID, keyUUID uuid.UUID) (*CreateDraftResult, error) {
	var result CreateDraftResult
	err := r.db.QueryRow(ctx, `
		SELECT r.quotation_id, r.id, q.public_token
		FROM quotation_revisions r
		JOIN quotations q ON q.id = r.quotation_id
		WHERE r.company_id = $1 AND r.idempotency_key = $2
	`, companyID, keyUUID).Scan(&result.QuotationID, &result.RevisionID, &result.PublicToken)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &result, nil
}

// FindByIdempotencyKey returns the revision bound to (company_id, key), or
// (nil, nil) when no aggregate exists yet — a miss is a normal outcome for
// idempotency probing, not an error. Keys that are not valid UUIDs can never
// exist in the uuid-typed column and therefore report a miss as well.
func (r *GenerationRepository) FindByIdempotencyKey(ctx context.Context, companyID uuid.UUID, idempotencyKey string) (*domain.QuotationRevision, error) {
	keyUUID, err := uuid.Parse(idempotencyKey)
	if err != nil {
		return nil, nil
	}

	query := `
		SELECT
			id, company_id, quotation_id, revision_number, idempotency_key, publication_status, generation_status,
			generation_input, budget_maximum, allocation_snapshot, rate_card_snapshot, legal_template_snapshot,
			content_html, content_version::text AS content_version, content_hash, ai_provider, ai_model,
			skill_snapshot, safety_envelope_version,
			legal_content_modified, legal_modified_by, legal_modified_at, commercial_content_modified,
			commercial_modified_by, commercial_modified_at, generated_at, published_at, signer_name, signer_email,
			signature_data, signed_content_hash, signed_at, signed_ip, signed_user_agent, created_by, created_at, updated_at
		FROM quotation_revisions
		WHERE company_id = $1 AND idempotency_key = $2
	`
	rev, err := scanRevisionRow(r.db.QueryRow(ctx, query, companyID, keyUUID))
	if err != nil {
		if errors.Is(err, ErrRevisionNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return rev, nil
}

// UpdateGenerationResult atomically persists the completed pipeline output:
// sanitized HTML, snapshots, provider/model identity, hash, generated_at and
// status completed. content_version stays untouched — incrementing it belongs
// to regeneration (Session 5). Safe to run inside a caller transaction or on
// the pool directly.
func (r *GenerationRepository) UpdateGenerationResult(ctx context.Context, q Execer, companyID, revisionID uuid.UUID, res UpdateGenerationResultParams) error {
	query := `
		WITH updated_rev AS (
			UPDATE quotation_revisions SET
				content_html = $1,
				allocation_snapshot = $2,
				rate_card_snapshot = $3,
				legal_template_snapshot = $4,
				skill_snapshot = $5,
				ai_provider = $6,
				ai_model = $7,
				content_hash = $8,
				generation_status = $9,
				generated_at = NOW(),
				updated_at = NOW(),
				safety_envelope_version = $12
			WHERE id = $10 AND company_id = $11
			RETURNING quotation_id, updated_at
		)
		UPDATE quotations q SET
			status = $13
		FROM updated_rev r
		WHERE q.id = r.quotation_id
		RETURNING r.updated_at
	`
	var updatedAt time.Time
	err := q.QueryRow(ctx, query,
		res.ContentHTML, res.AllocationSnapshot, res.RateCardSnapshot, res.LegalTemplateSnapshot,
		res.SkillSnapshot, res.AiProvider, res.AiModel, res.ContentHash,
		GenerationStatusCompleted, revisionID, companyID,
		res.SafetyEnvelopeVersion, PublicationStatusDraft,
	).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRevisionNotFound
		}
		logger.Error("Failed to update generation result", logger.Err(err))
		return err
	}
	return nil
}

// UpdateGenerationFailure atomically records a generation failure, storing
// the error message in the revision and marking the quotation as failed.
func (r *GenerationRepository) UpdateGenerationFailure(ctx context.Context, q Execer, companyID, revisionID uuid.UUID, failureReason string) error {
	query := `
		WITH updated_rev AS (
			UPDATE quotation_revisions SET
				generation_status = $1,
				generation_error = $2,
				updated_at = NOW()
			WHERE id = $3 AND company_id = $4
			RETURNING quotation_id
		)
		UPDATE quotations q SET
			status = $5
		FROM updated_rev r
		WHERE q.id = r.quotation_id
	`
	res, err := q.Exec(ctx, query, GenerationStatusFailed, failureReason, revisionID, companyID, QuotationStatusFailed)
	if err != nil {
		logger.Error("Failed to update generation failure", logger.Err(err))
		return err
	}
	if res.RowsAffected() == 0 {
		return ErrRevisionNotFound
	}
	return nil
}

// Pool exposes the underlying connection pool so callers can pass it as an
// Execer to UpdateGenerationResult outside of an explicit transaction.
// ErrRateCardNotFound / ErrSkillNotFound / ErrLegalTemplateNotFound are reused
// from their respective repository files.
func (r *GenerationRepository) Pool() *pgxpool.Pool {
	return r.db
}

// ---------------------------------------------------------------------------
// Session 5: regeneration support — draft loading, race-safe cost-control
// quota, atomic regenerated-result persistence and read-model state.
// ---------------------------------------------------------------------------

// ErrRevisionConflict reports a regenerate attempt against a revision that is
// no longer an active draft (published/superseded).
var ErrRevisionConflict = errors.New("revision_conflict")

// MaxRegeneratesPerHour bounds regeneration per draft inside the rolling one
// hour window enforced by ConsumeRegenerateQuota. MaxRegenerateWindowHours is
// bound into the SQL via make_interval so the two can never drift apart.
const (
	MaxRegeneratesPerHour    = 5
	MaxRegenerateWindowHours = 1
)

// DraftRevision is the minimal projection required to re-run the generation
// pipeline for an existing draft.
type DraftRevision struct {
	RevisionID        uuid.UUID
	QuotationID       uuid.UUID
	QuotationNumber   string
	PublicationStatus string
	GenerationStatus  string
	GenerationInput   json.RawMessage
}

// LoadDraftRevisionForRegeneration resolves the current revision of a
// quotation for regeneration. A missing aggregate or a quotation owned by
// another company is indistinguishable on purpose (both ErrRevisionNotFound)
// so the API layer can answer 404 without leaking resource existence.
// Revisions that are not active drafts yield ErrRevisionConflict.
func (r *GenerationRepository) LoadDraftRevisionForRegeneration(ctx context.Context, companyID, quotationID uuid.UUID) (*DraftRevision, error) {
	var d DraftRevision
	err := r.db.QueryRow(ctx, `
		SELECT r.id, r.quotation_id, q.quotation_number, r.publication_status, r.generation_status, r.generation_input
		FROM quotation_revisions r
		JOIN quotations q ON q.id = r.quotation_id
		WHERE r.quotation_id = $1 AND r.company_id = $2 AND r.revision_number = q.current_revision
		  AND q.deleted_at IS NULL
	`, quotationID, companyID).Scan(
		&d.RevisionID, &d.QuotationID, &d.QuotationNumber,
		&d.PublicationStatus, &d.GenerationStatus, &d.GenerationInput,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		logger.Error("Failed to load draft revision", logger.Err(err))
		return nil, err
	}
	if d.PublicationStatus != PublicationStatusDraft {
		return nil, ErrRevisionConflict
	}
	return &d, nil
}

// ConsumeRegenerateQuota enforces the cost-control budget with ONE gated
// atomic UPDATE — never a read-then-write round trip. Postgres row-level
// locking serialises concurrent callers, each evaluating the CASE/guard
// against the latest committed row version, so two simultaneous regenerates
// can neither double-count nor skip the counter.
//
// The window resets when last_regenerate_at is older than one hour; a call
// beyond the limit matches no row and returns pgx.ErrNoRows while leaving
// count and window untouched (a rejected attempt must not postpone expiry).
// Quota is consumed even if the AI call afterwards fails: the guard exists to
// bound provider spend.
func (r *GenerationRepository) ConsumeRegenerateQuota(ctx context.Context, q Execer, companyID, revisionID uuid.UUID) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		UPDATE quotation_revisions SET
			regenerate_count = CASE
				WHEN last_regenerate_at IS NULL OR last_regenerate_at <= NOW() - make_interval(hours => $5) THEN 1
				ELSE regenerate_count + 1 END,
			last_regenerate_at = NOW(),
			updated_at = NOW()
		WHERE id = $1 AND company_id = $2 AND publication_status = $3
		  AND (last_regenerate_at IS NULL
		       OR last_regenerate_at <= NOW() - make_interval(hours => $5)
		       OR regenerate_count < $4)
		RETURNING regenerate_count
	`, revisionID, companyID, PublicationStatusDraft, MaxRegeneratesPerHour, MaxRegenerateWindowHours).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		// Limit exhausted, or the revision stopped being an active draft in
		// the microseconds between the caller's pre-check and this UPDATE.
		// The service layer reports rate_limited here; the residual race is
		// re-checked by UpdateRegeneratedResult's draft guard below.
		return 0, pgx.ErrNoRows
	}
	if err != nil {
		logger.Error("Failed to consume regenerate quota", logger.Err(err))
		return 0, err
	}
	return count, nil
}

// UpdateRegeneratedResult atomically replaces the generated artefacts of a
// draft revision AND increments content_version in the same statement. It is
// only invoked after the full pipeline succeeded, so a failed regeneration
// leaves both the previous content_html and the previous content_version
// untouched. No new revision row is ever created.
//
// The publication_status guard closes the TOCTOU window spanning the AI
// pipeline: if the revision is published/superseded while regeneration was
// running, the UPDATE matches nothing and reports ErrRevisionConflict instead
// of overwriting published content.
func (r *GenerationRepository) UpdateRegeneratedResult(ctx context.Context, q Execer, companyID, revisionID uuid.UUID, res UpdateGenerationResultParams) error {
	query := `
		UPDATE quotation_revisions SET
			content_html = $1,
			allocation_snapshot = $2,
			rate_card_snapshot = $3,
			legal_template_snapshot = $4,
			skill_snapshot = $5,
			ai_provider = $6,
			ai_model = $7,
			content_hash = $8,
			generation_status = $9,
			generated_at = NOW(),
			updated_at = NOW(),
			safety_envelope_version = $12,
			content_version = quotation_revisions.content_version + 1
		WHERE id = $10 AND company_id = $11 AND publication_status = $13
		RETURNING updated_at
	`
	var updatedAt time.Time
	err := q.QueryRow(ctx, query,
		res.ContentHTML, res.AllocationSnapshot, res.RateCardSnapshot, res.LegalTemplateSnapshot,
		res.SkillSnapshot, res.AiProvider, res.AiModel, res.ContentHash,
		GenerationStatusCompleted, revisionID, companyID,
		res.SafetyEnvelopeVersion, PublicationStatusDraft,
	).Scan(&updatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrRevisionConflict
		}
		logger.Error("Failed to update regenerated result", logger.Err(err))
		return err
	}
	return nil
}

// DraftRevisionForSave is the projection of a draft revision needed to validate
// and execute a SaveDraft operation.
type DraftRevisionForSave struct {
	RevisionID                uuid.UUID
	QuotationID               uuid.UUID
	RevisionNumber            int
	PublicationStatus         string
	GenerationStatus          string
	ContentVersion            int
	BudgetMaximum             *int64
	AllocationSnapshot        json.RawMessage
	RateCardSnapshot          json.RawMessage
	LegalTemplateSnapshot     json.RawMessage
	GenerationInput           json.RawMessage
	LegalContentModified      bool
	CommercialContentModified bool
}

// UpdateDraftParams contains the payload and metadata for updating a draft revision.
type UpdateDraftParams struct {
	RevisionID                uuid.UUID
	ExpectedVersion           int
	ContentHTML               string
	ContentHash               *string
	LegalContentModified      bool
	LegalModifiedBy           *uuid.UUID
	LegalModifiedAt           *time.Time
	CommercialContentModified bool
	CommercialModifiedBy      *uuid.UUID
	CommercialModifiedAt      *time.Time
}

// SaveDraftResult reports the committed version and state of the saved draft.
type SaveDraftResult struct {
	RevisionID                uuid.UUID
	ContentVersion            int
	ContentHTML               string
	LegalContentModified      bool
	CommercialContentModified bool
	UpdatedAt                 time.Time
}

// LoadDraftRevisionForSave resolves the current draft revision for a quotation.
// Returns ErrRevisionNotFound if the aggregate does not exist, or ErrRevisionConflict
// if the current revision is not in 'draft' status.
func (r *GenerationRepository) LoadDraftRevisionForSave(ctx context.Context, companyID, quotationID uuid.UUID) (*DraftRevisionForSave, error) {
	var d DraftRevisionForSave
	err := r.db.QueryRow(ctx, `
		SELECT r.id, r.quotation_id, r.revision_number, r.publication_status, r.generation_status,
		       r.content_version, r.budget_maximum, r.allocation_snapshot, r.rate_card_snapshot, r.legal_template_snapshot,
		       r.generation_input,
		       r.legal_content_modified, r.commercial_content_modified
		FROM quotation_revisions r
		JOIN quotations q ON q.id = r.quotation_id
		WHERE r.quotation_id = $1 AND r.company_id = $2 AND r.revision_number = q.current_revision
		  AND q.deleted_at IS NULL
	`, quotationID, companyID).Scan(
		&d.RevisionID, &d.QuotationID, &d.RevisionNumber, &d.PublicationStatus, &d.GenerationStatus,
		&d.ContentVersion, &d.BudgetMaximum, &d.AllocationSnapshot, &d.RateCardSnapshot, &d.LegalTemplateSnapshot,
		&d.GenerationInput,
		&d.LegalContentModified, &d.CommercialContentModified,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		logger.Error("Failed to load draft revision for save", logger.Err(err))
		return nil, err
	}
	if d.PublicationStatus != PublicationStatusDraft {
		return nil, ErrRevisionConflict
	}
	return &d, nil
}

// UpdateDraft updates a draft revision's HTML content and flags with optimistic concurrency control.
// It matches by revision ID, quotation ID, company ID, publication_status = 'draft', and the exact expected content_version.
// If the row was modified or published in the meantime, 0 rows match and ErrRevisionConflict is returned.
func (r *GenerationRepository) UpdateDraft(ctx context.Context, q Execer, companyID, quotationID uuid.UUID, params UpdateDraftParams) (*SaveDraftResult, error) {
	query := `
		WITH current_quotation AS (
			SELECT current_revision
			FROM quotations
			WHERE id = $10 AND company_id = $11 AND deleted_at IS NULL
			FOR UPDATE
		)
		UPDATE quotation_revisions SET
			content_html = $1,
			content_hash = $2,
			legal_content_modified = $3,
			legal_modified_by = CASE WHEN $3 THEN COALESCE($4, legal_modified_by) ELSE legal_modified_by END,
			legal_modified_at = CASE WHEN $3 THEN COALESCE($5, NOW()) ELSE legal_modified_at END,
			commercial_content_modified = $6,
			commercial_modified_by = CASE WHEN $6 THEN COALESCE($7, commercial_modified_by) ELSE commercial_modified_by END,
			commercial_modified_at = CASE WHEN $6 THEN COALESCE($8, NOW()) ELSE commercial_modified_at END,
			content_version = quotation_revisions.content_version + 1,
			updated_at = NOW()
		WHERE id = $9 AND quotation_id = $10 AND company_id = $11
		  AND publication_status = $12
		  AND content_version = $13
		  AND revision_number = (SELECT current_revision FROM current_quotation)
		RETURNING content_version, updated_at
	`
	res := &SaveDraftResult{
		RevisionID:                params.RevisionID,
		ContentHTML:               params.ContentHTML,
		LegalContentModified:      params.LegalContentModified,
		CommercialContentModified: params.CommercialContentModified,
	}

	err := q.QueryRow(ctx, query,
		params.ContentHTML, params.ContentHash,
		params.LegalContentModified, params.LegalModifiedBy, params.LegalModifiedAt,
		params.CommercialContentModified, params.CommercialModifiedBy, params.CommercialModifiedAt,
		params.RevisionID, quotationID, companyID,
		PublicationStatusDraft, params.ExpectedVersion,
	).Scan(&res.ContentVersion, &res.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionConflict
		}
		logger.Error("Failed to update draft revision", logger.Err(err))
		return nil, err
	}
	return res, nil
}

// GenerationState is the safe read model behind GET /generation-status and
// the regenerate response. It intentionally excludes budget figures and
// internal snapshots.
type GenerationState struct {
	GenerationStatus string     `json:"generation_status"`
	ContentVersion   int        `json:"content_version"`
	ContentHTML      string     `json:"content_html"`
	GeneratedAt      *time.Time `json:"generated_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// GetGenerationState returns the generation view of a quotation's current
// revision, tenant-scoped. Missing or foreign aggregates report
// ErrRevisionNotFound so handlers can answer 404.
func (r *GenerationRepository) GetGenerationState(ctx context.Context, companyID, quotationID uuid.UUID) (*GenerationState, error) {
	var s GenerationState
	err := r.db.QueryRow(ctx, `
		SELECT r.generation_status, r.content_version, COALESCE(r.content_html, ''), r.generated_at, r.updated_at
		FROM quotation_revisions r
		JOIN quotations q ON q.id = r.quotation_id
		WHERE r.quotation_id = $1 AND r.company_id = $2 AND r.revision_number = q.current_revision
		  AND q.deleted_at IS NULL
	`, quotationID, companyID).Scan(&s.GenerationStatus, &s.ContentVersion, &s.ContentHTML, &s.GeneratedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		return nil, err
	}
	return &s, nil
}

// QuotationGenerationDetail is the safe read model behind GET /quotations/:id
// for AI-first drafts. Budget figures and internal snapshots are deliberately
// excluded from this API view, except for legal_template_snapshot which is
// safely exposed to support the frontend "Restore Official Template" action.
type QuotationGenerationDetail struct {
	QuotationID               uuid.UUID       `json:"quotation_id"`
	CompanyID                 uuid.UUID       `json:"company_id"`
	ClientID                  uuid.UUID       `json:"client_id"`
	QuotationNumber           string          `json:"quotation_number"`
	Subject                   string          `json:"subject"`
	Status                    string          `json:"status"`
	RevisionID                uuid.UUID       `json:"revision_id"`
	LegalTemplateSnapshot     json.RawMessage `json:"legal_template_snapshot,omitempty"`
	LegalContentModified      bool            `json:"legal_content_modified"`
	CommercialContentModified bool            `json:"commercial_content_modified"`
	CreatedAt                 time.Time       `json:"created_at"`
	UpdatedAt                 time.Time       `json:"updated_at"`

	GenerationState
}

// GetGenerationDetail joins the quotation master with the current revision's
// generation state, tenant-scoped. Missing or foreign aggregates report
// ErrRevisionNotFound so handlers can answer 404 without leaking existence.
func (r *GenerationRepository) GetGenerationDetail(ctx context.Context, companyID, quotationID uuid.UUID) (*QuotationGenerationDetail, error) {
	var d QuotationGenerationDetail
	err := r.db.QueryRow(ctx, `
		SELECT q.id, q.company_id, q.client_id, q.quotation_number,
		       COALESCE(q.subject, ''), q.status, q.created_at, q.updated_at,
		       r.id, r.generation_status, r.content_version, COALESCE(r.content_html, ''),
		       COALESCE(r.legal_template_snapshot, '{}'::jsonb),
		       r.legal_content_modified, r.commercial_content_modified,
		       r.generated_at, r.updated_at
		FROM quotations q
		JOIN quotation_revisions r ON r.quotation_id = q.id AND r.revision_number = q.current_revision
		WHERE q.id = $1 AND q.company_id = $2 AND q.deleted_at IS NULL
	`, quotationID, companyID).Scan(
		&d.QuotationID, &d.CompanyID, &d.ClientID, &d.QuotationNumber,
		&d.Subject, &d.Status, &d.CreatedAt, &d.UpdatedAt,
		&d.RevisionID, &d.GenerationStatus, &d.ContentVersion, &d.ContentHTML,
		&d.LegalTemplateSnapshot,
		&d.LegalContentModified, &d.CommercialContentModified,
		&d.GeneratedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrRevisionNotFound
		}
		return nil, err
	}
	return &d, nil
}

// GetClientName resolves the display name of a platform-global client.
// core.clients is not company-scoped (tenant authorization for client
// selection belongs to the API layer), so a miss is a normal outcome and is
// reported as ("", nil).
func (r *GenerationRepository) GetClientName(ctx context.Context, clientID uuid.UUID) (string, error) {
	var name string
	err := r.db.QueryRow(ctx, `
		SELECT name FROM core.clients
		WHERE id = $1 AND is_active = true AND deleted_at IS NULL
	`, clientID).Scan(&name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return name, nil
}

// GetSkillForGeneration loads the active, non-deleted skill scoped to the
// company. Inactive or missing skills are indistinguishable here: both are
// ErrSkillNotFound so callers surface settings_incomplete either way.
func (r *GenerationRepository) GetSkillForGeneration(ctx context.Context, companyID, skillID uuid.UUID) (*domain.Skill, error) {
	skill := &domain.Skill{}
	err := r.db.QueryRow(ctx, `
		SELECT id, company_id, name, command, content_instruction, is_active, created_by, created_at, updated_at, deleted_at
		FROM ai_skills
		WHERE id = $1 AND company_id = $2 AND is_active = true AND deleted_at IS NULL
	`, skillID, companyID).Scan(
		&skill.ID, &skill.CompanyID, &skill.Name, &skill.Command, &skill.ContentInstruction,
		&skill.IsActive, &skill.CreatedBy, &skill.CreatedAt, &skill.UpdatedAt, &skill.DeletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSkillNotFound
		}
		return nil, err
	}
	return skill, nil
}

// GetRateCardWithItems loads the active rate card header plus all of its
// items, both tenant-scoped. Items may legitimately be empty; callers decide
// whether that makes the card unusable.
func (r *GenerationRepository) GetRateCardWithItems(ctx context.Context, companyID, rateCardID uuid.UUID) (*domain.RateCard, []domain.RateCardItem, error) {
	card := &domain.RateCard{}
	err := r.db.QueryRow(ctx, `
		SELECT id, company_id, code, name, market, currency, effective_from, effective_until, is_active, created_by, created_at, updated_at
		FROM rate_cards
		WHERE id = $1 AND company_id = $2 AND is_active = true
	`, rateCardID, companyID).Scan(
		&card.ID, &card.CompanyID, &card.Code, &card.Name, &card.Market, &card.Currency,
		&card.EffectiveFrom, &card.EffectiveUntil, &card.IsActive, &card.CreatedBy, &card.CreatedAt, &card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrRateCardNotFound
		}
		return nil, nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT id, company_id, rate_card_id, role, seniority, term_type, amount_per_month, created_at, updated_at
		FROM rate_card_items
		WHERE rate_card_id = $1 AND company_id = $2
	`, rateCardID, companyID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	items := make([]domain.RateCardItem, 0)
	for rows.Next() {
		item := domain.RateCardItem{}
		if err := rows.Scan(
			&item.ID, &item.CompanyID, &item.RateCardID, &item.Role, &item.Seniority,
			&item.TermType, &item.AmountPerMonth, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return card, items, nil
}

// GetActiveLegalTemplateForCompany resolves the highest active version of the
// given template code for the company, mirroring LegalTemplateRepository
// semantics. The code is supplied by the service layer from a named constant.
func (r *GenerationRepository) GetActiveLegalTemplateForCompany(ctx context.Context, companyID uuid.UUID, code string) (*domain.LegalTemplate, error) {
	tmpl := &domain.LegalTemplate{}
	err := r.db.QueryRow(ctx, `
		SELECT id, company_id, code, version, content_html, is_active, approved_by, approved_at, created_at
		FROM legal_templates
		WHERE company_id = $1 AND code = $2 AND is_active = true
		ORDER BY version DESC LIMIT 1
	`, companyID, code).Scan(
		&tmpl.ID, &tmpl.CompanyID, &tmpl.Code, &tmpl.Version, &tmpl.ContentHTML,
		&tmpl.IsActive, &tmpl.ApprovedBy, &tmpl.ApprovedAt, &tmpl.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLegalTemplateNotFound
		}
		return nil, err
	}
	return tmpl, nil
}

// NextQuotationNumber derives the next sequential quotation number for the
// company using the shared QUO-<year>-<seq> convention.
func (r *GenerationRepository) NextQuotationNumber(ctx context.Context, companyID uuid.UUID) (string, error) {
	var count int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM quotations WHERE company_id = $1`, companyID).Scan(&count); err != nil {
		return "", err
	}
	return fmt.Sprintf("QUO-%d-%04d", time.Now().UTC().Year(), count+1), nil
}
