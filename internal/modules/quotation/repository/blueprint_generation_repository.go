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

	ErrBlueprintClaimBusy           = errors.New("blueprint claim busy")
	ErrBlueprintNoEligibleSection   = errors.New("blueprint has no eligible section")
	ErrBlueprintRecoveryRequired    = errors.New("blueprint recovery required")
	ErrBlueprintSectionNotRetryable = errors.New("blueprint section is not retryable")
	ErrBlueprintAttemptLimit        = errors.New("blueprint section attempt limit reached")
	ErrBlueprintClaimConflict       = errors.New("blueprint claim conflict")
	ErrBlueprintContentConflict     = errors.New("blueprint content version conflict")
	ErrBlueprintManualConflict      = errors.New("blueprint manual completion conflict")
)

const BlueprintMaxSectionAttempts = 3

type BlueprintSectionClaim struct {
	BlueprintID            uuid.UUID
	RevisionID             uuid.UUID
	SectionRunID           uuid.UUID
	CompanyID              uuid.UUID
	SectionType            domain.BlueprintSectionType
	Attempt                int
	ExpectedContentVersion int
	BlueprintTitle         string
	SourceContextSnapshot  json.RawMessage
	SkillSnapshot          json.RawMessage
	RevisionContent        json.RawMessage
	ContextContent         json.RawMessage
	Dependencies           map[domain.BlueprintSectionType]json.RawMessage
}

type CompleteBlueprintSectionParams struct {
	Claim                 BlueprintSectionClaim
	SectionContent        json.RawMessage
	SectionValidation     json.RawMessage
	RevisionContent       json.RawMessage
	CrossValidationResult json.RawMessage
	ProviderName          string
	Model                 string
	CompletedAt           time.Time
	Final                 bool
}

type FailBlueprintSectionParams struct {
	Claim                 BlueprintSectionClaim
	SafeErrorCode         string
	SectionContent        json.RawMessage
	SectionValidation     json.RawMessage
	CrossValidationResult json.RawMessage
	FailedAt              time.Time
	Terminal              bool
}

type BlueprintManualSnapshot struct {
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

type CompleteBlueprintManualParams struct {
	Snapshot            BlueprintManualSnapshot
	SectionContent      json.RawMessage
	SectionValidation   json.RawMessage
	RevisionContent     json.RawMessage
	InvalidatedSections []domain.BlueprintSectionType
	CompletedAt         time.Time
}

type BlueprintSectionPersistenceResult struct {
	RevisionID       uuid.UUID
	SectionType      domain.BlueprintSectionType
	SectionStatus    domain.BlueprintSectionStatus
	GenerationStatus domain.BlueprintGenerationStatus
	ContentVersion   int
	Attempt          int
}

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

	runs, err := r.loadSectionRuns(ctx, r.db, rev.ID, companyID)
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

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (r *BlueprintGenerationRepository) GetBlueprintGeneration(ctx context.Context, blueprintID, companyID uuid.UUID) (*CreateBlueprintGenerationResult, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

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

	err = tx.QueryRow(ctx, query, blueprintID, companyID).Scan(
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

	runs, err := r.loadSectionRuns(ctx, tx, rev.ID, companyID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &CreateBlueprintGenerationResult{
		Blueprint:        bp,
		Revision:         rev,
		SectionRuns:      runs,
		IdempotentReplay: false,
	}, nil
}

func (r *BlueprintGenerationRepository) loadSectionRuns(ctx context.Context, q querier, revisionID, companyID uuid.UUID) ([]domain.BlueprintSectionRun, error) {
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

	rows, err := q.Query(ctx, query, revisionID, companyID)
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

var retryAllowlist = map[string]bool{
	"provider_timeout":            true,
	"provider_rate_limited":       true,
	"provider_unavailable":        true,
	"provider_invalid_credential": true,
	"provider_configuration":      true,
	"invalid_schema":              true,
	"interrupted":                 true,
}

var blueprintStagePromptDependencies = map[domain.BlueprintSectionType][]domain.BlueprintSectionType{
	domain.BlueprintSectionContext:         {},
	domain.BlueprintSectionJourney:         {domain.BlueprintSectionContext},
	domain.BlueprintSectionArchitecture:    {domain.BlueprintSectionContext, domain.BlueprintSectionJourney},
	domain.BlueprintSectionStateMachine:    {domain.BlueprintSectionContext},
	domain.BlueprintSectionWorkflow:        {domain.BlueprintSectionContext, domain.BlueprintSectionJourney, domain.BlueprintSectionStateMachine},
	domain.BlueprintSectionWBS:             {domain.BlueprintSectionContext, domain.BlueprintSectionJourney, domain.BlueprintSectionArchitecture, domain.BlueprintSectionStateMachine, domain.BlueprintSectionWorkflow},
	domain.BlueprintSectionCrossValidation: {domain.BlueprintSectionContext, domain.BlueprintSectionJourney, domain.BlueprintSectionArchitecture, domain.BlueprintSectionStateMachine, domain.BlueprintSectionWorkflow, domain.BlueprintSectionWBS},
}

type lockedRevisionData struct {
	blueprintID           uuid.UUID
	blueprintTitle        string
	revisionID            uuid.UUID
	contentJSON           json.RawMessage
	contentVersion        int
	sourceContextSnapshot json.RawMessage
	skillSnapshot         json.RawMessage
	generationStatus      string
}

type lockedRunData struct {
	id               uuid.UUID
	sectionType      domain.BlueprintSectionType
	status           domain.BlueprintSectionStatus
	attemptCount     int
	contentJSON      json.RawMessage
	validationResult json.RawMessage
	safeErrorCode    *string
	startedAt        *time.Time
	completedAt      *time.Time
}

func findRunByType(runs []lockedRunData, st domain.BlueprintSectionType) *lockedRunData {
	for i := range runs {
		if runs[i].sectionType == st {
			return &runs[i]
		}
	}
	return nil
}

func isStableRun(run lockedRunData) bool {
	if run.status != domain.BlueprintSectionCompleted && run.status != domain.BlueprintSectionValidated {
		return false
	}
	if len(bytes.TrimSpace(run.contentJSON)) == 0 || string(run.contentJSON) == "null" {
		return false
	}
	return true
}

func (r *BlueprintGenerationRepository) lockDraftRevisionAndRuns(ctx context.Context, tx pgx.Tx, companyID, blueprintID uuid.UUID) (*lockedRevisionData, []lockedRunData, error) {
	revQuery := `
		SELECT b.id, b.title, r.id, r.content_json, r.content_version,
		       r.source_context_snapshot, r.skill_snapshot, r.generation_status
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		WHERE b.id = $1 AND b.company_id = $2 AND b.deleted_at IS NULL
		  AND r.company_id = $2 AND r.publication_status = 'draft'
		  AND r.revision_number = b.current_revision
		FOR UPDATE OF r
	`
	var rev lockedRevisionData
	err := tx.QueryRow(ctx, revQuery, blueprintID, companyID).Scan(
		&rev.blueprintID, &rev.blueprintTitle, &rev.revisionID, &rev.contentJSON, &rev.contentVersion,
		&rev.sourceContextSnapshot, &rev.skillSnapshot, &rev.generationStatus,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrBlueprintNotFound
		}
		return nil, nil, err
	}

	runsQuery := `
		SELECT id, section_type, status, attempt_count, content_json,
		       validation_result, safe_error_code, started_at, completed_at
		FROM blueprint_section_runs
		WHERE company_id = $1 AND blueprint_revision_id = $2
		ORDER BY CASE section_type
		  WHEN 'context' THEN 1 WHEN 'journey' THEN 2 WHEN 'architecture' THEN 3
		  WHEN 'state_machine' THEN 4 WHEN 'workflow' THEN 5 WHEN 'wbs' THEN 6
		  WHEN 'cross_validation' THEN 7 ELSE 99 END
		FOR UPDATE
	`
	rows, err := tx.Query(ctx, runsQuery, companyID, rev.revisionID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var runs []lockedRunData
	for rows.Next() {
		var run lockedRunData
		var secTypeStr, statusStr string
		err := rows.Scan(
			&run.id, &secTypeStr, &statusStr, &run.attemptCount, &run.contentJSON,
			&run.validationResult, &run.safeErrorCode, &run.startedAt, &run.completedAt,
		)
		if err != nil {
			return nil, nil, err
		}
		run.sectionType = domain.BlueprintSectionType(secTypeStr)
		run.status = domain.BlueprintSectionStatus(statusStr)
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	if len(runs) != len(domain.BlueprintSectionOrder) {
		return nil, nil, ErrBlueprintNoEligibleSection
	}

	for i, expectedType := range domain.BlueprintSectionOrder {
		if runs[i].sectionType != expectedType {
			return nil, nil, ErrBlueprintNoEligibleSection
		}
	}

	return &rev, runs, nil
}

func (r *BlueprintGenerationRepository) checkAndRecoverGenerating(ctx context.Context, tx pgx.Tx, companyID uuid.UUID, revID uuid.UUID, runs []lockedRunData, now time.Time, staleAfter time.Duration) (bool, error) {
	for _, run := range runs {
		if run.status == domain.BlueprintSectionGenerating {
			if run.startedAt != nil && !run.startedAt.IsZero() && (now.Sub(*run.startedAt) >= staleAfter || !run.startedAt.After(now.Add(-staleAfter))) {
				// Stale generating row detected -> recover it
				_, err := tx.Exec(ctx, `
					UPDATE blueprint_section_runs
					SET status = 'failed', safe_error_code = 'interrupted', updated_at = $1
					WHERE id = $2 AND company_id = $3 AND blueprint_revision_id = $4
				`, now, run.id, companyID, revID)
				if err != nil {
					return true, err
				}

				newRevStatus := domain.BlueprintGenerationPartialFailed
				if run.attemptCount >= BlueprintMaxSectionAttempts {
					newRevStatus = domain.BlueprintGenerationFailed
				}

				_, err = tx.Exec(ctx, `
					UPDATE blueprint_revisions
					SET generation_status = $1, updated_at = $2
					WHERE id = $3 AND company_id = $4
				`, string(newRevStatus), now, revID, companyID)
				if err != nil {
					return true, err
				}

				if err := tx.Commit(ctx); err != nil {
					return true, err
				}

				return true, ErrBlueprintRecoveryRequired
			}

			// Active (non-stale) generating row
			return true, ErrBlueprintClaimBusy
		}
	}
	return false, nil
}

func (r *BlueprintGenerationRepository) ClaimNextBlueprintSection(ctx context.Context, companyID, blueprintID uuid.UUID, now time.Time, staleAfter time.Duration) (*BlueprintSectionClaim, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rev, runs, err := r.lockDraftRevisionAndRuns(ctx, tx, companyID, blueprintID)
	if err != nil {
		return nil, err
	}

	handled, err := r.checkAndRecoverGenerating(ctx, tx, companyID, rev.revisionID, runs, now, staleAfter)
	if handled {
		return nil, err
	}

	// Context (position 0) must be stable
	if !isStableRun(runs[0]) {
		return nil, ErrBlueprintNoEligibleSection
	}

	var targetRun *lockedRunData
	for i := 1; i < len(runs); i++ {
		run := runs[i]
		if run.status == domain.BlueprintSectionCompleted || run.status == domain.BlueprintSectionValidated {
			continue
		}
		if run.status == domain.BlueprintSectionFailed {
			return nil, ErrBlueprintNoEligibleSection
		}
		if run.status == domain.BlueprintSectionPending {
			// Check all canonical predecessor runs are stable
			for j := 0; j < i; j++ {
				if !isStableRun(runs[j]) {
					return nil, ErrBlueprintNoEligibleSection
				}
			}
			targetRun = &runs[i]
			break
		}
		return nil, ErrBlueprintNoEligibleSection
	}

	if targetRun == nil {
		return nil, ErrBlueprintNoEligibleSection
	}

	var newAttempt int
	err = tx.QueryRow(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'generating', attempt_count = attempt_count + 1,
		    safe_error_code = NULL, started_at = $1, completed_at = NULL, updated_at = $1
		WHERE id = $2 AND company_id = $3 AND blueprint_revision_id = $4
		  AND status = $5 AND attempt_count = $6
		RETURNING attempt_count
	`, now, targetRun.id, companyID, rev.revisionID, string(targetRun.status), targetRun.attemptCount).Scan(&newAttempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintClaimConflict
		}
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE blueprint_revisions
		SET generation_status = 'in_progress', updated_at = $1
		WHERE id = $2 AND company_id = $3
	`, now, rev.revisionID, companyID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	deps := make(map[domain.BlueprintSectionType]json.RawMessage)
	for _, depType := range blueprintStagePromptDependencies[targetRun.sectionType] {
		depRun := findRunByType(runs, depType)
		if depRun != nil {
			deps[depType] = depRun.contentJSON
		}
	}

	contextRun := findRunByType(runs, domain.BlueprintSectionContext)
	var contextContent json.RawMessage
	if contextRun != nil {
		contextContent = contextRun.contentJSON
	}

	return &BlueprintSectionClaim{
		BlueprintID:            rev.blueprintID,
		RevisionID:             rev.revisionID,
		SectionRunID:           targetRun.id,
		CompanyID:              companyID,
		SectionType:            targetRun.sectionType,
		Attempt:                newAttempt,
		ExpectedContentVersion: rev.contentVersion,
		BlueprintTitle:         rev.blueprintTitle,
		SourceContextSnapshot:  rev.sourceContextSnapshot,
		SkillSnapshot:          rev.skillSnapshot,
		RevisionContent:        rev.contentJSON,
		ContextContent:         contextContent,
		Dependencies:           deps,
	}, nil
}

func (r *BlueprintGenerationRepository) ClaimFailedBlueprintSection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, now time.Time, staleAfter time.Duration) (*BlueprintSectionClaim, error) {
	switch sectionType {
	case domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS:
	default:
		return nil, ErrBlueprintSectionNotRetryable
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rev, runs, err := r.lockDraftRevisionAndRuns(ctx, tx, companyID, blueprintID)
	if err != nil {
		return nil, err
	}

	handled, err := r.checkAndRecoverGenerating(ctx, tx, companyID, rev.revisionID, runs, now, staleAfter)
	if handled {
		return nil, err
	}

	var targetIdx int = -1
	for i := range runs {
		if runs[i].sectionType == sectionType {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return nil, ErrBlueprintSectionNotRetryable
	}

	targetRun := &runs[targetIdx]
	if targetRun.status != domain.BlueprintSectionFailed {
		return nil, ErrBlueprintSectionNotRetryable
	}

	if targetRun.attemptCount >= BlueprintMaxSectionAttempts {
		return nil, ErrBlueprintAttemptLimit
	}

	if targetRun.safeErrorCode == nil || !retryAllowlist[*targetRun.safeErrorCode] {
		return nil, ErrBlueprintSectionNotRetryable
	}

	// Check that all canonical predecessor runs are stable
	for j := 0; j < targetIdx; j++ {
		if !isStableRun(runs[j]) {
			return nil, ErrBlueprintSectionNotRetryable
		}
	}

	var newAttempt int
	err = tx.QueryRow(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'generating', attempt_count = attempt_count + 1,
		    safe_error_code = NULL, started_at = $1, completed_at = NULL, updated_at = $1
		WHERE id = $2 AND company_id = $3 AND blueprint_revision_id = $4
		  AND status = 'failed' AND attempt_count = $5
		RETURNING attempt_count
	`, now, targetRun.id, companyID, rev.revisionID, targetRun.attemptCount).Scan(&newAttempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintClaimConflict
		}
		return nil, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE blueprint_revisions
		SET generation_status = 'in_progress', updated_at = $1
		WHERE id = $2 AND company_id = $3
	`, now, rev.revisionID, companyID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	deps := make(map[domain.BlueprintSectionType]json.RawMessage)
	for _, depType := range blueprintStagePromptDependencies[targetRun.sectionType] {
		depRun := findRunByType(runs, depType)
		if depRun != nil {
			deps[depType] = depRun.contentJSON
		}
	}

	contextRun := findRunByType(runs, domain.BlueprintSectionContext)
	var contextContent json.RawMessage
	if contextRun != nil {
		contextContent = contextRun.contentJSON
	}

	return &BlueprintSectionClaim{
		BlueprintID:            rev.blueprintID,
		RevisionID:             rev.revisionID,
		SectionRunID:           targetRun.id,
		CompanyID:              companyID,
		SectionType:            targetRun.sectionType,
		Attempt:                newAttempt,
		ExpectedContentVersion: rev.contentVersion,
		BlueprintTitle:         rev.blueprintTitle,
		SourceContextSnapshot:  rev.sourceContextSnapshot,
		SkillSnapshot:          rev.skillSnapshot,
		RevisionContent:        rev.contentJSON,
		ContextContent:         contextContent,
		Dependencies:           deps,
	}, nil
}

func nullIfBlank(s string) *string {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func defaultJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		return json.RawMessage(fallback)
	}
	return raw
}

func (r *BlueprintGenerationRepository) CompleteBlueprintSection(ctx context.Context, params CompleteBlueprintSectionParams) (*BlueprintSectionPersistenceResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Lock revision FOR UPDATE
	revQuery := `
		SELECT r.id, r.content_version, r.generation_status
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		WHERE b.id = $1 AND b.company_id = $2 AND b.deleted_at IS NULL
		  AND r.company_id = $2 AND r.id = $3
		FOR UPDATE OF r
	`
	var revID uuid.UUID
	var revVersion int
	var revGenStatus string
	err = tx.QueryRow(ctx, revQuery, params.Claim.BlueprintID, params.Claim.CompanyID, params.Claim.RevisionID).Scan(&revID, &revVersion, &revGenStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	// 2. Lock target run FOR UPDATE
	runQuery := `
		SELECT status, attempt_count
		FROM blueprint_section_runs
		WHERE id = $1 AND company_id = $2 AND blueprint_revision_id = $3
		FOR UPDATE
	`
	var runStatus string
	var runAttempt int
	err = tx.QueryRow(ctx, runQuery, params.Claim.SectionRunID, params.Claim.CompanyID, revID).Scan(&runStatus, &runAttempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintClaimConflict
		}
		return nil, err
	}

	if runStatus != string(domain.BlueprintSectionGenerating) || runAttempt != params.Claim.Attempt {
		return nil, ErrBlueprintClaimConflict
	}

	if revVersion != params.Claim.ExpectedContentVersion {
		return nil, ErrBlueprintContentConflict
	}

	var newContentVersion int
	var finalGenStatus domain.BlueprintGenerationStatus
	var finalSecStatus domain.BlueprintSectionStatus = domain.BlueprintSectionCompleted

	if params.Claim.SectionType == domain.BlueprintSectionCrossValidation {
		// Cross validation completion
		newContentVersion = revVersion // does NOT increment content_version
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'completed', content_json = $1, validation_result = $2,
			    safe_error_code = NULL, completed_at = $3, updated_at = $3
			WHERE id = $4 AND company_id = $5 AND blueprint_revision_id = $6
		`, defaultJSON(params.SectionContent, "{}"), defaultJSON(params.SectionValidation, "{}"), params.CompletedAt, params.Claim.SectionRunID, params.Claim.CompanyID, revID)
		if err != nil {
			return nil, err
		}

		if params.Final {
			finalGenStatus = domain.BlueprintGenerationCompleted
			_, err = tx.Exec(ctx, `
				UPDATE blueprint_revisions
				SET completeness_status = 'complete', generation_status = 'completed',
				    cross_validation_result = $1, generated_at = $2, updated_at = $2
				WHERE id = $3 AND company_id = $4
			`, defaultJSON(params.CrossValidationResult, "{}"), params.CompletedAt, revID, params.Claim.CompanyID)
		} else {
			finalGenStatus = domain.BlueprintGenerationInProgress
			_, err = tx.Exec(ctx, `
				UPDATE blueprint_revisions
				SET cross_validation_result = $1, updated_at = $2
				WHERE id = $3 AND company_id = $4
			`, defaultJSON(params.CrossValidationResult, "{}"), params.CompletedAt, revID, params.Claim.CompanyID)
		}
		if err != nil {
			return nil, err
		}
	} else {
		// Provider section completion
		newContentVersion = revVersion + 1
		finalGenStatus = domain.BlueprintGenerationInProgress

		_, err = tx.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'completed', content_json = $1, validation_result = $2,
			    safe_error_code = NULL, completed_at = $3, updated_at = $3
			WHERE id = $4 AND company_id = $5 AND blueprint_revision_id = $6
		`, params.SectionContent, defaultJSON(params.SectionValidation, "{}"), params.CompletedAt, params.Claim.SectionRunID, params.Claim.CompanyID, revID)
		if err != nil {
			return nil, err
		}

		_, err = tx.Exec(ctx, `
			UPDATE blueprint_revisions
			SET content_version = content_version + 1,
			    content_json = $1,
			    ai_provider = $2,
			    ai_model = $3,
			    validation_result = jsonb_set(COALESCE(validation_result, '{}'::jsonb), ARRAY[$4], $5::jsonb, true),
			    updated_at = $6
			WHERE id = $7 AND company_id = $8
		`, params.RevisionContent, nullIfBlank(params.ProviderName), nullIfBlank(params.Model), string(params.Claim.SectionType), defaultJSON(params.SectionValidation, "{}"), params.CompletedAt, revID, params.Claim.CompanyID)
		if err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BlueprintSectionPersistenceResult{
		RevisionID:       revID,
		SectionType:      params.Claim.SectionType,
		SectionStatus:    finalSecStatus,
		GenerationStatus: finalGenStatus,
		ContentVersion:   newContentVersion,
		Attempt:          params.Claim.Attempt,
	}, nil
}

func (r *BlueprintGenerationRepository) FailBlueprintSection(ctx context.Context, params FailBlueprintSectionParams) (*BlueprintSectionPersistenceResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// 1. Lock revision FOR UPDATE
	revQuery := `
		SELECT r.id, r.content_version, r.generation_status
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		WHERE b.id = $1 AND b.company_id = $2 AND b.deleted_at IS NULL
		  AND r.company_id = $2 AND r.id = $3
		FOR UPDATE OF r
	`
	var revID uuid.UUID
	var revVersion int
	var revGenStatus string
	err = tx.QueryRow(ctx, revQuery, params.Claim.BlueprintID, params.Claim.CompanyID, params.Claim.RevisionID).Scan(&revID, &revVersion, &revGenStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	// 2. Lock target run FOR UPDATE
	runQuery := `
		SELECT status, attempt_count
		FROM blueprint_section_runs
		WHERE id = $1 AND company_id = $2 AND blueprint_revision_id = $3
		FOR UPDATE
	`
	var runStatus string
	var runAttempt int
	err = tx.QueryRow(ctx, runQuery, params.Claim.SectionRunID, params.Claim.CompanyID, revID).Scan(&runStatus, &runAttempt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBlueprintClaimConflict
		}
		return nil, err
	}

	if runStatus != string(domain.BlueprintSectionGenerating) || runAttempt != params.Claim.Attempt {
		return nil, ErrBlueprintClaimConflict
	}

	if revVersion != params.Claim.ExpectedContentVersion {
		return nil, ErrBlueprintContentConflict
	}

	// 3. Update target run
	if len(bytes.TrimSpace(params.SectionContent)) > 0 && string(params.SectionContent) != "null" {
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'failed', safe_error_code = $1, content_json = $2,
			    validation_result = COALESCE($3, validation_result), updated_at = $4
			WHERE id = $5 AND company_id = $6 AND blueprint_revision_id = $7
		`, params.SafeErrorCode, params.SectionContent, defaultJSON(params.SectionValidation, "{}"), params.FailedAt, params.Claim.SectionRunID, params.Claim.CompanyID, revID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'failed', safe_error_code = $1,
			    validation_result = COALESCE($2, validation_result), updated_at = $3
			WHERE id = $4 AND company_id = $5 AND blueprint_revision_id = $6
		`, params.SafeErrorCode, defaultJSON(params.SectionValidation, "{}"), params.FailedAt, params.Claim.SectionRunID, params.Claim.CompanyID, revID)
	}
	if err != nil {
		return nil, err
	}

	// 4. Update revision generation_status
	newRevStatus := domain.BlueprintGenerationPartialFailed
	if params.Terminal || params.Claim.Attempt >= BlueprintMaxSectionAttempts {
		newRevStatus = domain.BlueprintGenerationFailed
	}

	if params.Claim.SectionType == domain.BlueprintSectionCrossValidation && len(bytes.TrimSpace(params.CrossValidationResult)) > 0 && string(params.CrossValidationResult) != "null" {
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_revisions
			SET generation_status = $1, cross_validation_result = $2, updated_at = $3
			WHERE id = $4 AND company_id = $5
		`, string(newRevStatus), params.CrossValidationResult, params.FailedAt, revID, params.Claim.CompanyID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_revisions
			SET generation_status = $1, updated_at = $2
			WHERE id = $3 AND company_id = $4
		`, string(newRevStatus), params.FailedAt, revID, params.Claim.CompanyID)
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BlueprintSectionPersistenceResult{
		RevisionID:       revID,
		SectionType:      params.Claim.SectionType,
		SectionStatus:    domain.BlueprintSectionFailed,
		GenerationStatus: newRevStatus,
		ContentVersion:   revVersion,
		Attempt:          params.Claim.Attempt,
	}, nil
}

func (r *BlueprintGenerationRepository) LoadBlueprintManualSnapshot(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*BlueprintManualSnapshot, error) {
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

	return &BlueprintManualSnapshot{
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

func (r *BlueprintGenerationRepository) CompleteBlueprintSectionManually(ctx context.Context, params CompleteBlueprintManualParams) (*BlueprintSectionPersistenceResult, error) {
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

	// 1. Lock revision FOR UPDATE
	revQuery := `
		SELECT r.id, r.content_version
		FROM blueprints b
		JOIN blueprint_revisions r ON r.blueprint_id = b.id AND r.company_id = b.company_id
		WHERE b.id = $1 AND b.company_id = $2 AND b.deleted_at IS NULL
		  AND r.company_id = $2 AND r.id = $3
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

	// 2. Lock target run FOR UPDATE
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
			return nil, ErrBlueprintManualConflict
		}
		return nil, err
	}

	if runStatus == string(domain.BlueprintSectionGenerating) {
		return nil, ErrBlueprintManualConflict
	}
	if runStatus != string(params.Snapshot.SectionStatus) {
		return nil, ErrBlueprintManualConflict
	}

	// 3. Update target run -> validated
	_, err = tx.Exec(ctx, `
		UPDATE blueprint_section_runs
		SET status = 'validated', content_json = $1, validation_result = $2,
		    safe_error_code = NULL, completed_at = $3, updated_at = $3
		WHERE id = $4 AND company_id = $5 AND blueprint_revision_id = $6
	`, params.SectionContent, defaultJSON(params.SectionValidation, "{}"), params.CompletedAt, params.Snapshot.SectionRunID, params.Snapshot.CompanyID, revID)
	if err != nil {
		return nil, err
	}

	// 4. Reset invalidated downstream runs to pending/null/empty
	for _, invType := range params.InvalidatedSections {
		_, err = tx.Exec(ctx, `
			UPDATE blueprint_section_runs
			SET status = 'pending', content_json = NULL, validation_result = '{}'::jsonb,
			    safe_error_code = NULL, started_at = NULL, completed_at = NULL, updated_at = $1
			WHERE company_id = $2 AND blueprint_revision_id = $3 AND section_type = $4
		`, params.CompletedAt, params.Snapshot.CompanyID, revID, string(invType))
		if err != nil {
			return nil, err
		}
	}

	// 5. Update revision: increment version, save content, update validation_result, clear cross_validation_result
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
	`, params.RevisionContent, string(params.Snapshot.SectionType), defaultJSON(params.SectionValidation, "{}"), params.CompletedAt, revID, params.Snapshot.CompanyID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &BlueprintSectionPersistenceResult{
		RevisionID:       revID,
		SectionType:      params.Snapshot.SectionType,
		SectionStatus:    domain.BlueprintSectionValidated,
		GenerationStatus: domain.BlueprintGenerationInProgress,
		ContentVersion:   newContentVersion,
		Attempt:          runAttempt,
	}, nil
}
