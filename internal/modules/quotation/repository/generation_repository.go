package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
	GenerationStatusPending   = "pending"
	GenerationStatusCompleted = "completed"

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
	ClientID              uuid.UUID
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
	return nil, fmt.Errorf("create draft did not settle after %d attempts: %w", maxCreateDraftAttempts, lastErr)
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

	quotation := &domain.Quotation{
		ID:               uuid.New(),
		CompanyID:        params.CompanyID,
		ClientID:         params.ClientID,
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
			rate_card_snapshot, legal_template_snapshot, skill_snapshot, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id
	`,
		revisionID, params.CompanyID, quotation.ID, 1, keyUUID,
		PublicationStatusDraft, GenerationStatusPending, params.GenerationInput, params.BudgetMaximum,
		params.RateCardSnapshot, params.LegalTemplateSnapshot, params.SkillSnapshot, params.CreatedBy,
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
	case params.ClientID == uuid.Nil:
		return fmt.Errorf("client_id is required")
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
			updated_at = NOW()
		WHERE id = $10 AND company_id = $11
		RETURNING updated_at
	`
	var updatedAt interface{}
	err := q.QueryRow(ctx, query,
		res.ContentHTML, res.AllocationSnapshot, res.RateCardSnapshot, res.LegalTemplateSnapshot,
		res.SkillSnapshot, res.AiProvider, res.AiModel, res.ContentHash,
		GenerationStatusCompleted, revisionID, companyID,
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
