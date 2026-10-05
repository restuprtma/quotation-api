package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrConfirmationRequired     = errors.New("confirmation required for modified content")
	ErrQuotationImmutable       = errors.New("published quotation is immutable")
	ErrActiveDraftAlreadyExists = errors.New("active draft revision already exists")
	ErrNoPublishedRevision      = errors.New("no published revision found to revise")
	ErrContentVersionMismatch   = errors.New("content version mismatch")
	ErrDraftNotCompleted        = errors.New("draft generation must be completed before publishing")
	ErrEmptyContent             = errors.New("quotation content cannot be empty")
	ErrRevisionNotFound         = repository.ErrRevisionNotFound
)

type LifecycleRepository interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	LockQuotationMaster(ctx context.Context, tx pgx.Tx, quotationID string) error
	GetQuotationByID(ctx context.Context, id, companyID string) (*domain.Quotation, error)
	GetDraftRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error)
	MarkPublishedRevisionSuperseded(ctx context.Context, tx pgx.Tx, quotationID string) error
	PublishDraftQuotation(ctx context.Context, tx pgx.Tx, quotationID, draftID string) error
	GetActivePublishedRevision(ctx context.Context, tx pgx.Tx, quotationID, companyID string) (*domain.QuotationRevision, error)
	GetMaxRevisionNumber(ctx context.Context, tx pgx.Tx, quotationID string) (int, error)
	CreateRevisionTx(ctx context.Context, tx pgx.Tx, rev *domain.QuotationRevision) error
	UpdateQuotationCurrentRevision(ctx context.Context, tx pgx.Tx, quotationID string, currentRevision int) error
}

func (s *Service) getLifecycleRepo() LifecycleRepository {
	if s.lifecycleRepo != nil {
		return s.lifecycleRepo
	}
	if s.repo != nil {
		return s.repo
	}
	return nil
}

// PublishQuotation transitions a draft revision to published, marks previous published revision as superseded,
// and points quotation active_published_revision_id to the published revision under a transaction with row-level locking.
func (s *Service) PublishQuotation(ctx context.Context, companyID, quotationID string, req dto.PublishQuotationRequest) error {
	repo := s.getLifecycleRepo()
	if repo == nil {
		return errors.New("repository not configured")
	}
	if companyID == "" {
		return ErrNoCompanyContext
	}
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		return ErrNotFound
	}
	quotationUUID, err := uuid.Parse(quotationID)
	if err != nil {
		return ErrNotFound
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Step b: Lock master row
	if err := repo.LockQuotationMaster(ctx, tx, quotationID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}

	// Step c: Load quotation master (check companyID for tenant isolation)
	quot, err := repo.GetQuotationByID(ctx, quotationID, companyID)
	if err != nil {
		return mapRepoErr(err)
	}
	if quot == nil || quot.CompanyID != companyUUID || quot.ID != quotationUUID {
		return ErrNotFound
	}

	// Step d: Load draft revision
	draft, err := repo.GetDraftRevision(ctx, tx, quotationID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, repository.ErrRevisionNotFound) {
			return ErrNotFound
		}
		return err
	}
	if draft == nil {
		return ErrNotFound
	}

	// Step e: Validate draft
	reqRevUUID, err := uuid.Parse(req.RevisionID)
	if err != nil || draft.ID != reqRevUUID {
		return ErrNotFound
	}

	if draft.ContentVersion == nil {
		return ErrContentVersionMismatch
	}
	versionNum, convErr := strconv.Atoi(strings.TrimSpace(*draft.ContentVersion))
	if convErr != nil || versionNum != req.ContentVersion {
		return ErrContentVersionMismatch
	}

	if draft.GenerationStatus != "completed" {
		return ErrDraftNotCompleted
	}

	if draft.ContentHTML == nil || strings.TrimSpace(*draft.ContentHTML) == "" {
		return ErrEmptyContent
	}

	if draft.LegalContentModified && !req.ConfirmLegalModified {
		return ErrConfirmationRequired
	}
	if draft.CommercialContentModified && !req.ConfirmCommercialModified {
		return ErrConfirmationRequired
	}

	// Step f: Supersede old published revision
	if err := repo.MarkPublishedRevisionSuperseded(ctx, tx, quotationID); err != nil {
		return err
	}

	// Step g: Publish draft revision and update master active pointer
	if err := repo.PublishDraftQuotation(ctx, tx, quotationID, draft.ID.String()); err != nil {
		return mapRepoErr(err)
	}

	// Step h: Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// CreateQuotationRevision creates a new draft revision cloned from the active published revision.
func (s *Service) CreateQuotationRevision(ctx context.Context, companyID, quotationID string) (*dto.QuotationRevisionResponse, error) {
	repo := s.getLifecycleRepo()
	if repo == nil {
		return nil, errors.New("repository not configured")
	}
	if companyID == "" {
		return nil, ErrNoCompanyContext
	}
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, ErrNotFound
	}
	quotationUUID, err := uuid.Parse(quotationID)
	if err != nil {
		return nil, ErrNotFound
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Step b: Lock master row
	if err := repo.LockQuotationMaster(ctx, tx, quotationID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// Step c: Load quotation master (check companyID for tenant isolation)
	quot, err := repo.GetQuotationByID(ctx, quotationID, companyID)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	if quot == nil || quot.CompanyID != companyUUID || quot.ID != quotationUUID {
		return nil, ErrNotFound
	}

	// Step d: Check if active draft exists
	draft, err := repo.GetDraftRevision(ctx, tx, quotationID, companyID)
	if err == nil && draft != nil {
		return nil, ErrActiveDraftAlreadyExists
	}
	if err != nil && !errors.Is(err, repository.ErrRevisionNotFound) && !errors.Is(err, repository.ErrNotFound) {
		return nil, err
	}

	// Step e: Load active published revision
	pubRev, err := repo.GetActivePublishedRevision(ctx, tx, quotationID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrRevisionNotFound) || errors.Is(err, repository.ErrNotFound) {
			return nil, ErrNoPublishedRevision
		}
		return nil, err
	}
	if pubRev == nil {
		return nil, ErrNoPublishedRevision
	}

	// Step f: Get max revision number under lock
	maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, quotationID)
	if err != nil {
		return nil, err
	}
	newRevisionNumber := maxRev + 1

	// Step g: Clone published revision content & snapshots into new draft
	defaultVersion := "1"
	genInput := pubRev.GenerationInput
	if len(genInput) == 0 {
		genInput = json.RawMessage("{}")
	}

	newRev := &domain.QuotationRevision{
		ID:                    uuid.New(),
		CompanyID:             companyUUID,
		QuotationID:           quotationUUID,
		RevisionNumber:        newRevisionNumber,
		PublicationStatus:     repository.PublicationStatusDraft,
		GenerationStatus:      "completed",
		GenerationInput:       genInput,
		BudgetMaximum:         pubRev.BudgetMaximum,
		AllocationSnapshot:    pubRev.AllocationSnapshot,
		RateCardSnapshot:      pubRev.RateCardSnapshot,
		LegalTemplateSnapshot: pubRev.LegalTemplateSnapshot,
		ContentHTML:           pubRev.ContentHTML,
		ContentHash:           pubRev.ContentHash,
		AiProvider:            pubRev.AiProvider,
		AiModel:               pubRev.AiModel,
		SkillSnapshot:         pubRev.SkillSnapshot,
		SafetyEnvelopeVersion: pubRev.SafetyEnvelopeVersion,
		PromptState:           pubRev.PromptState,
		PromptStateVersion:    pubRev.PromptStateVersion,
		PromptSectionHashes:   pubRev.PromptSectionHashes,
		ContentVersion:        &defaultVersion,
		SignedAt:              nil,
		SignerName:            nil,
		SignerEmail:           nil,
		SignatureData:         nil,
		SignedContentHash:     nil,
		SignedIP:              nil,
		SignedUserAgent:       nil,
		PublishedAt:           nil,
		LegalContentModified:      false,
		LegalModifiedBy:           nil,
		LegalModifiedAt:           nil,
		CommercialContentModified: false,
		CommercialModifiedBy:      nil,
		CommercialModifiedAt:      nil,
		GeneratedAt:               pubRev.GeneratedAt,
		CreatedBy:                 pubRev.CreatedBy,
	}

	// Step h: Insert revision
	if err := repo.CreateRevisionTx(ctx, tx, newRev); err != nil {
		return nil, err
	}

	// Step i: Update quotation current revision
	if err := repo.UpdateQuotationCurrentRevision(ctx, tx, quotationID, newRevisionNumber); err != nil {
		return nil, mapRepoErr(err)
	}

	// Step j: Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Step k: Return mapped *dto.QuotationRevisionResponse and nil
	meta := dto.EvaluateBlueprintSourceEligibility(newRev.PublicationStatus, newRev.GenerationStatus, newRev.ContentHTML)
	return &dto.QuotationRevisionResponse{
		ID:                      newRev.ID,
		VersionNumber:           newRev.RevisionNumber,
		Status:                  newRev.PublicationStatus,
		SignatureImageURL:       newRev.SignatureData,
		SignedAt:                newRev.SignedAt,
		SignedBy:                newRev.SignerName,
		SignedIP:                newRev.SignedIP,
		PromptState:             newRev.PromptState,
		PromptStateVersion:      newRev.PromptStateVersion,
		PromptSectionHashes:     newRev.PromptSectionHashes,
		BlueprintSourceMetadata: &meta,
	}, nil
}
