package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

var (
	ErrBlueprintAlreadyExists              = errors.New("blueprint already exists for quotation")
	ErrBlueprintIdempotencyPayloadMismatch = errors.New("blueprint idempotency payload mismatch")
	ErrBlueprintInvalidCreateParams        = errors.New("invalid blueprint create params")
)

var lowercaseHex64Regex = regexp.MustCompile(`^[0-9a-f]{64}$`)

type CreateBlueprintGenerationParams struct {
	CompanyID              uuid.UUID
	QuotationID            uuid.UUID
	SourceRevisionID       uuid.UUID
	Title                  string
	IdempotencyKey         uuid.UUID
	IdempotencyPayloadHash string
	SourceContextSnapshot  json.RawMessage
	ContextContent         json.RawMessage
	SkillSnapshot          json.RawMessage
	CreatedBy              *uuid.UUID
}

type CreateBlueprintGenerationResult struct {
	Blueprint        domain.Blueprint
	Revision         domain.BlueprintRevision
	SectionRuns      []domain.BlueprintSectionRun
	IdempotentReplay bool
}

type BlueprintGenerationRepository struct {
	db *pgxpool.Pool
}

func NewBlueprintGenerationRepository(db *pgxpool.Pool) *BlueprintGenerationRepository {
	return &BlueprintGenerationRepository{db: db}
}

func validateCreateBlueprintGenerationParams(params CreateBlueprintGenerationParams) error {
	if params.CompanyID == uuid.Nil {
		return fmt.Errorf("%w: company_id must not be zero", ErrBlueprintInvalidCreateParams)
	}
	if params.QuotationID == uuid.Nil {
		return fmt.Errorf("%w: quotation_id must not be zero", ErrBlueprintInvalidCreateParams)
	}
	if params.SourceRevisionID == uuid.Nil {
		return fmt.Errorf("%w: source_revision_id must not be zero", ErrBlueprintInvalidCreateParams)
	}
	if params.IdempotencyKey == uuid.Nil {
		return fmt.Errorf("%w: idempotency_key must not be zero", ErrBlueprintInvalidCreateParams)
	}
	if params.CreatedBy != nil && *params.CreatedBy == uuid.Nil {
		return fmt.Errorf("%w: created_by must not be zero uuid", ErrBlueprintInvalidCreateParams)
	}

	trimmedTitle := strings.TrimSpace(params.Title)
	if trimmedTitle == "" {
		return fmt.Errorf("%w: title must not be blank", ErrBlueprintInvalidCreateParams)
	}
	if utf8.RuneCountInString(params.Title) > 160 {
		return fmt.Errorf("%w: title must not exceed 160 runes", ErrBlueprintInvalidCreateParams)
	}

	if !lowercaseHex64Regex.MatchString(params.IdempotencyPayloadHash) {
		return fmt.Errorf("%w: idempotency_payload_hash must be 64 lowercase hex characters", ErrBlueprintInvalidCreateParams)
	}

	if len(bytes.TrimSpace(params.SourceContextSnapshot)) == 0 || !json.Valid(params.SourceContextSnapshot) {
		return fmt.Errorf("%w: source_context_snapshot must be non-empty valid JSON", ErrBlueprintInvalidCreateParams)
	}
	if len(bytes.TrimSpace(params.ContextContent)) == 0 || !json.Valid(params.ContextContent) {
		return fmt.Errorf("%w: context_content must be non-empty valid JSON", ErrBlueprintInvalidCreateParams)
	}
	if len(bytes.TrimSpace(params.SkillSnapshot)) == 0 || !json.Valid(params.SkillSnapshot) {
		return fmt.Errorf("%w: skill_snapshot must be non-empty valid JSON", ErrBlueprintInvalidCreateParams)
	}

	return nil
}

func (r *BlueprintGenerationRepository) CreateBlueprintGeneration(ctx context.Context, params CreateBlueprintGenerationParams) (*CreateBlueprintGenerationResult, error) {
	// 1. Validate params before transaction
	if err := validateCreateBlueprintGenerationParams(params); err != nil {
		return nil, err
	}

	// 2. Begin transaction; defer rollback
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 3. Verify source via quotation_revisions JOIN quotations with locked read (FOR SHARE OF qr, q)
	var pubStatus string
	var genStatus string
	var rawContentHTML *string
	err = tx.QueryRow(ctx, `
		SELECT qr.publication_status, qr.generation_status, qr.content_html
		FROM quotation_revisions qr
		JOIN quotations q ON q.id = qr.quotation_id AND q.company_id = qr.company_id
		WHERE qr.id = $1
		  AND qr.quotation_id = $2
		  AND qr.company_id = $3
		  AND q.company_id = $3
		  AND q.deleted_at IS NULL
		FOR SHARE OF qr, q
	`, params.SourceRevisionID, params.QuotationID, params.CompanyID).Scan(&pubStatus, &genStatus, &rawContentHTML)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintSourceNotFound
		}
		return nil, err
	}

	// Canonical source eligibility guards
	switch pubStatus {
	case "draft", "published", "superseded":
	default:
		return nil, ErrBlueprintSourceIneligible
	}

	if genStatus != "completed" {
		return nil, ErrBlueprintSourceIneligible
	}

	if rawContentHTML == nil || strings.TrimSpace(*rawContentHTML) == "" {
		return nil, ErrBlueprintSourceIneligible
	}

	// 4. Insert Blueprint using INSERT ... SELECT from tenant-owned quotation and ON CONFLICT (quotation_id) DO NOTHING
	bpID := uuid.New()
	var bpCreatedAt, bpUpdatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO blueprints (
			id, company_id, quotation_id, title, status, current_revision, created_by
		)
		SELECT $1, $2, q.id, $4, 'draft', 1, $5
		FROM quotations q
		WHERE q.id = $3 AND q.company_id = $2
		ON CONFLICT (quotation_id) DO NOTHING
		RETURNING created_at, updated_at
	`, bpID, params.CompanyID, params.QuotationID, params.Title, params.CreatedBy).Scan(&bpCreatedAt, &bpUpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 5. On no inserted Blueprint: rollback, lookup same tenant/key; equal hash is replay, different hash is mismatch, missing key is ErrBlueprintAlreadyExists
			_ = tx.Rollback(ctx)
			return r.resolveConflict(ctx, params.CompanyID, params.IdempotencyKey, params.IdempotencyPayloadHash)
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			_ = tx.Rollback(ctx)
			return r.resolveConflict(ctx, params.CompanyID, params.IdempotencyKey, params.IdempotencyPayloadHash)
		}
		return nil, err
	}

	// 6. Insert Draft Revision 1 with key/hash, source/context/skill snapshots, {} content and validation JSON, and content version 1
	revID := uuid.New()
	var revCreatedAt, revUpdatedAt time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO blueprint_revisions (
			id, company_id, blueprint_id, revision_number,
			idempotency_key, idempotency_payload_hash,
			source_quotation_revision_id, source_context_snapshot,
			publication_status, completeness_status, generation_status,
			content_json, content_version,
			validation_result, cross_validation_result,
			skill_snapshot, created_by
		) VALUES (
			$1, $2, $3, 1,
			$4, $5,
			$6, $7,
			'draft', 'partial', 'pending',
			'{}'::jsonb, 1,
			'{}'::jsonb, '{}'::jsonb,
			$8, $9
		)
		RETURNING created_at, updated_at
	`, revID, params.CompanyID, bpID, params.IdempotencyKey, params.IdempotencyPayloadHash,
		params.SourceRevisionID, params.SourceContextSnapshot, string(params.SkillSnapshot), params.CreatedBy,
	).Scan(&revCreatedAt, &revUpdatedAt)

	if err != nil {
		_ = tx.Rollback(ctx)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// 9. On PostgreSQL 23505 for uq_blueprint_revisions_idempotency_key, rollback and resolve by tenant/key/hash without returning raw database text
			return r.resolveConflict(ctx, params.CompanyID, params.IdempotencyKey, params.IdempotencyPayloadHash)
		}
		return nil, err
	}

	// 7. Insert seven runs in BlueprintSectionOrder; context is validated with ContextContent, others pending with SQL NULL content
	runs := make([]domain.BlueprintSectionRun, 0, len(domain.BlueprintSectionOrder))
	for _, st := range domain.BlueprintSectionOrder {
		runID := uuid.New()
		var status string
		var contentJSON any

		if st == domain.BlueprintSectionContext {
			status = string(domain.BlueprintSectionValidated)
			contentJSON = []byte(params.ContextContent)
		} else {
			status = string(domain.BlueprintSectionPending)
			contentJSON = nil
		}

		var runUpdatedAt time.Time
		err = tx.QueryRow(ctx, `
			INSERT INTO blueprint_section_runs (
				id, company_id, blueprint_revision_id, section_type, status, attempt_count, content_json, validation_result
			) VALUES (
				$1, $2, $3, $4, $5, 0, $6, '{}'::jsonb
			)
			RETURNING updated_at
		`, runID, params.CompanyID, revID, string(st), status, contentJSON).Scan(&runUpdatedAt)
		if err != nil {
			_ = tx.Rollback(ctx)
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return r.resolveConflict(ctx, params.CompanyID, params.IdempotencyKey, params.IdempotencyPayloadHash)
			}
			return nil, err
		}

		run := domain.BlueprintSectionRun{
			ID:                  runID,
			CompanyID:           params.CompanyID,
			BlueprintRevisionID: revID,
			SectionType:         string(st),
			Status:              status,
			AttemptCount:        0,
			ValidationResult:    json.RawMessage("{}"),
			UpdatedAt:           runUpdatedAt,
		}
		if st == domain.BlueprintSectionContext {
			run.ContentJSON = params.ContextContent
		}
		runs = append(runs, run)
	}

	// 8. Commit and return in-memory aggregate in canonical order
	if err := tx.Commit(ctx); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return r.resolveConflict(ctx, params.CompanyID, params.IdempotencyKey, params.IdempotencyPayloadHash)
		}
		return nil, err
	}

	bp := domain.Blueprint{
		ID:              bpID,
		CompanyID:       params.CompanyID,
		QuotationID:     params.QuotationID,
		Title:           params.Title,
		Status:          "draft",
		CurrentRevision: 1,
		CreatedBy:       params.CreatedBy,
		CreatedAt:       bpCreatedAt,
		UpdatedAt:       bpUpdatedAt,
	}

	hashCopy := params.IdempotencyPayloadHash
	keyCopy := params.IdempotencyKey
	sourceRevCopy := params.SourceRevisionID

	rev := domain.BlueprintRevision{
		ID:                        revID,
		CompanyID:                 params.CompanyID,
		BlueprintID:               bpID,
		RevisionNumber:            1,
		IdempotencyKey:            &keyCopy,
		IdempotencyPayloadHash:    &hashCopy,
		SourceQuotationRevisionID: &sourceRevCopy,
		SourceContextSnapshot:     params.SourceContextSnapshot,
		PublicationStatus:         "draft",
		CompletenessStatus:        "partial",
		GenerationStatus:          string(domain.BlueprintGenerationPending),
		ContentJSON:               json.RawMessage("{}"),
		ContentVersion:            1,
		ValidationResult:          json.RawMessage("{}"),
		CrossValidationResult:     json.RawMessage("{}"),
		SkillSnapshot:             params.SkillSnapshot,
		CreatedBy:                 params.CreatedBy,
		CreatedAt:                 revCreatedAt,
		UpdatedAt:                 revUpdatedAt,
	}

	return &CreateBlueprintGenerationResult{
		Blueprint:        bp,
		Revision:         rev,
		SectionRuns:      runs,
		IdempotentReplay: false,
	}, nil
}

func (r *BlueprintGenerationRepository) resolveConflict(ctx context.Context, companyID, idempotencyKey uuid.UUID, payloadHash string) (*CreateBlueprintGenerationResult, error) {
	res, err := r.FindBlueprintByIdempotencyKey(ctx, companyID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, ErrBlueprintAlreadyExists
	}

	existingHash := ""
	if res.Revision.IdempotencyPayloadHash != nil {
		existingHash = strings.TrimSpace(*res.Revision.IdempotencyPayloadHash)
	}

	if existingHash != payloadHash {
		return nil, ErrBlueprintIdempotencyPayloadMismatch
	}

	return res, nil
}

func (r *BlueprintGenerationRepository) FindBlueprintByIdempotencyKey(ctx context.Context, companyID, key uuid.UUID) (*CreateBlueprintGenerationResult, error) {
	query := `
		SELECT
			b.id, b.company_id, b.quotation_id, b.title, b.status, b.current_revision,
			b.active_published_revision_id, b.created_by, b.created_at, b.updated_at, b.deleted_at,
			r.id, r.company_id, r.blueprint_id, r.revision_number, r.idempotency_key, r.idempotency_payload_hash,
			r.source_quotation_revision_id, r.source_context_snapshot, r.publication_status, r.completeness_status,
			r.generation_status, r.content_json, r.content_version, r.validation_result, r.cross_validation_result,
			r.skill_snapshot, r.safety_envelope_version, r.ai_provider, r.ai_model, r.generated_at, r.published_at,
			r.created_by, r.created_at, r.updated_at
		FROM blueprint_revisions r
		JOIN blueprints b ON b.id = r.blueprint_id
		WHERE r.idempotency_key = $1
		  AND r.company_id = $2
		  AND b.company_id = $2
	`

	var bp domain.Blueprint
	var rev domain.BlueprintRevision

	err := r.db.QueryRow(ctx, query, key, companyID).Scan(
		&bp.ID, &bp.CompanyID, &bp.QuotationID, &bp.Title, &bp.Status, &bp.CurrentRevision,
		&bp.ActivePublishedRevisionID, &bp.CreatedBy, &bp.CreatedAt, &bp.UpdatedAt, &bp.DeletedAt,
		&rev.ID, &rev.CompanyID, &rev.BlueprintID, &rev.RevisionNumber, &rev.IdempotencyKey, &rev.IdempotencyPayloadHash,
		&rev.SourceQuotationRevisionID, &rev.SourceContextSnapshot, &rev.PublicationStatus, &rev.CompletenessStatus,
		&rev.GenerationStatus, &rev.ContentJSON, &rev.ContentVersion, &rev.ValidationResult, &rev.CrossValidationResult,
		&rev.SkillSnapshot, &rev.SafetyEnvelopeVersion, &rev.AiProvider, &rev.AiModel, &rev.GeneratedAt, &rev.PublishedAt,
		&rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if rev.IdempotencyPayloadHash != nil {
		trimmed := strings.TrimSpace(*rev.IdempotencyPayloadHash)
		rev.IdempotencyPayloadHash = &trimmed
	}

	runs, err := r.loadSectionRuns(ctx, rev.ID, companyID)
	if err != nil {
		return nil, err
	}

	return &CreateBlueprintGenerationResult{
		Blueprint:        bp,
		Revision:         rev,
		SectionRuns:      runs,
		IdempotentReplay: true,
	}, nil
}

func (r *BlueprintGenerationRepository) GetBlueprintGeneration(ctx context.Context, blueprintID, companyID uuid.UUID) (*CreateBlueprintGenerationResult, error) {
	query := `
		SELECT
			b.id, b.company_id, b.quotation_id, b.title, b.status, b.current_revision,
			b.active_published_revision_id, b.created_by, b.created_at, b.updated_at, b.deleted_at,
			r.id, r.company_id, r.blueprint_id, r.revision_number, r.idempotency_key, r.idempotency_payload_hash,
			r.source_quotation_revision_id, r.source_context_snapshot, r.publication_status, r.completeness_status,
			r.generation_status, r.content_json, r.content_version, r.validation_result, r.cross_validation_result,
			r.skill_snapshot, r.safety_envelope_version, r.ai_provider, r.ai_model, r.generated_at, r.published_at,
			r.created_by, r.created_at, r.updated_at
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		WHERE b.id = $1
		  AND b.company_id = $2
		  AND b.deleted_at IS NULL
		  AND (r.publication_status = 'draft' OR r.revision_number = b.current_revision)
		ORDER BY r.revision_number DESC
		LIMIT 1
	`

	var bp domain.Blueprint
	var rev domain.BlueprintRevision

	err := r.db.QueryRow(ctx, query, blueprintID, companyID).Scan(
		&bp.ID, &bp.CompanyID, &bp.QuotationID, &bp.Title, &bp.Status, &bp.CurrentRevision,
		&bp.ActivePublishedRevisionID, &bp.CreatedBy, &bp.CreatedAt, &bp.UpdatedAt, &bp.DeletedAt,
		&rev.ID, &rev.CompanyID, &rev.BlueprintID, &rev.RevisionNumber, &rev.IdempotencyKey, &rev.IdempotencyPayloadHash,
		&rev.SourceQuotationRevisionID, &rev.SourceContextSnapshot, &rev.PublicationStatus, &rev.CompletenessStatus,
		&rev.GenerationStatus, &rev.ContentJSON, &rev.ContentVersion, &rev.ValidationResult, &rev.CrossValidationResult,
		&rev.SkillSnapshot, &rev.SafetyEnvelopeVersion, &rev.AiProvider, &rev.AiModel, &rev.GeneratedAt, &rev.PublishedAt,
		&rev.CreatedBy, &rev.CreatedAt, &rev.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	if rev.IdempotencyPayloadHash != nil {
		trimmed := strings.TrimSpace(*rev.IdempotencyPayloadHash)
		rev.IdempotencyPayloadHash = &trimmed
	}

	runs, err := r.loadSectionRuns(ctx, rev.ID, companyID)
	if err != nil {
		return nil, err
	}

	return &CreateBlueprintGenerationResult{
		Blueprint:        bp,
		Revision:         rev,
		SectionRuns:      runs,
		IdempotentReplay: false,
	}, nil
}

func (r *BlueprintGenerationRepository) loadSectionRuns(ctx context.Context, revisionID, companyID uuid.UUID) ([]domain.BlueprintSectionRun, error) {
	query := `
		SELECT
			id, company_id, blueprint_revision_id, section_type, status, attempt_count,
			content_json, validation_result, safe_error_code, started_at, completed_at, updated_at
		FROM blueprint_section_runs
		WHERE blueprint_revision_id = $1 AND company_id = $2
		ORDER BY CASE section_type
			WHEN 'context' THEN 1 WHEN 'journey' THEN 2 WHEN 'architecture' THEN 3
			WHEN 'state_machine' THEN 4 WHEN 'workflow' THEN 5 WHEN 'wbs' THEN 6
			WHEN 'cross_validation' THEN 7 ELSE 99 END
	`

	rows, err := r.db.Query(ctx, query, revisionID, companyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []domain.BlueprintSectionRun
	for rows.Next() {
		var run domain.BlueprintSectionRun
		err := rows.Scan(
			&run.ID, &run.CompanyID, &run.BlueprintRevisionID, &run.SectionType, &run.Status, &run.AttemptCount,
			&run.ContentJSON, &run.ValidationResult, &run.SafeErrorCode, &run.StartedAt, &run.CompletedAt, &run.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return runs, nil
}
