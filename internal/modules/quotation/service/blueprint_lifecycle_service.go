package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type BlueprintLifecycleRepo = repository.BlueprintLifecycleRepository

var (
	ErrBlueprintConfirmationRequired     = errors.New("confirmation required for partial blueprint publish")
	ErrBlueprintImmutable               = errors.New("published blueprint is immutable")
	ErrBlueprintActiveDraftAlreadyExists = errors.New("active draft revision already exists")
	ErrBlueprintNoPublishedRevision      = errors.New("no published revision found to revise")
	ErrBlueprintRevisionNotFound         = repository.ErrBlueprintRevisionNotFound
	ErrBlueprintDraftNotFound            = errors.New("draft blueprint revision not found")
)

func mapBlueprintRepoErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, repository.ErrBlueprintNotFound),
		errors.Is(err, repository.ErrBlueprintMasterNotFound):
		return ErrBlueprintNotFound
	case errors.Is(err, repository.ErrBlueprintDraftNotFound),
		errors.Is(err, ErrBlueprintDraftNotFound):
		return ErrBlueprintDraftNotFound
	case errors.Is(err, repository.ErrBlueprintRevisionNotFound),
		errors.Is(err, ErrBlueprintRevisionNotFound):
		return ErrBlueprintRevisionNotFound
	default:
		return err
	}
}

func isDraftComplete(draft *domain.BlueprintRevision, req dto.PublishBlueprintRequest) bool {
	if len(req.WarningCodes) > 0 {
		return false
	}
	if draft.CompletenessStatus != "complete" {
		return false
	}

	// Check cross validation result for issues/warnings if present
	trimmedCross := bytes.TrimSpace(draft.CrossValidationResult)
	if len(trimmedCross) > 0 && string(trimmedCross) != "{}" && string(trimmedCross) != "null" {
		var crossRes struct {
			Valid  bool                     `json:"valid"`
			Issues []map[string]interface{} `json:"issues"`
		}
		if err := json.Unmarshal(trimmedCross, &crossRes); err == nil {
			if !crossRes.Valid || len(crossRes.Issues) > 0 {
				return false
			}
		}
	}

	// Check single-section validation result if present
	trimmedVal := bytes.TrimSpace(draft.ValidationResult)
	if len(trimmedVal) > 0 && string(trimmedVal) != "{}" && string(trimmedVal) != "null" {
		var valMap map[string]struct {
			Valid  bool                     `json:"valid"`
			Issues []map[string]interface{} `json:"issues"`
		}
		if err := json.Unmarshal(trimmedVal, &valMap); err == nil {
			for _, v := range valMap {
				if !v.Valid || len(v.Issues) > 0 {
					return false
				}
			}
		}
	}

	// If ContentJSON is populated, check whether all 5 provider sections exist
	trimmed := bytes.TrimSpace(draft.ContentJSON)
	if len(trimmed) > 0 && string(trimmed) != "{}" && string(trimmed) != "null" {
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &rawMap); err == nil && len(rawMap) > 0 {
			if _, ok := rawMap["journey"]; !ok {
				return false
			}
			if _, ok := rawMap["architecture"]; !ok {
				return false
			}
			if _, ok := rawMap["state_machine"]; !ok {
				return false
			}
			_, hasWorkflows := rawMap["workflows"]
			_, hasWorkflow := rawMap["workflow"]
			if !hasWorkflows && !hasWorkflow {
				return false
			}
			if _, ok := rawMap["wbs"]; !ok {
				return false
			}
		}
	}

	return true
}

// PublishBlueprint transitions a draft revision to published, marks previous published revision as superseded,
// and points blueprint active_published_revision_id to the published revision under a transaction with row-level locking.
func (s *BlueprintDraftService) PublishBlueprint(ctx context.Context, companyID, blueprintID string, req dto.PublishBlueprintRequest) error {
	repo := s.getLifecycleRepo()
	if repo == nil {
		return errors.New("repository not configured")
	}
	if companyID == "" {
		return ErrBlueprintNotFound
	}
	_, err := uuid.Parse(companyID)
	if err != nil {
		return ErrBlueprintNotFound
	}
	_, err = uuid.Parse(blueprintID)
	if err != nil {
		return ErrBlueprintNotFound
	}
	reqRevUUID, err := uuid.Parse(req.RevisionID)
	if err != nil {
		return ErrBlueprintNotFound
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Lock master row
	if err := repo.LockBlueprintMaster(ctx, tx, blueprintID); err != nil {
		if errors.Is(err, repository.ErrBlueprintMasterNotFound) || errors.Is(err, repository.ErrBlueprintNotFound) {
			return ErrBlueprintNotFound
		}
		return err
	}

	// Load draft revision under tenant scope
	draft, err := repo.GetDraftRevision(ctx, tx, blueprintID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintDraftNotFound) ||
			errors.Is(err, ErrBlueprintDraftNotFound) ||
			errors.Is(err, repository.ErrBlueprintRevisionNotFound) ||
			errors.Is(err, repository.ErrBlueprintNotFound) {
			return ErrBlueprintNotFound
		}
		return err
	}
	if draft == nil || draft.ID != reqRevUUID {
		return ErrBlueprintNotFound
	}

	// Determine completeness
	isComplete := isDraftComplete(draft, req)
	if !isComplete && !req.ConfirmPartial {
		return ErrBlueprintConfirmationRequired
	}

	// Supersede previously published revision
	if err := repo.MarkPublishedRevisionSuperseded(ctx, tx, blueprintID); err != nil {
		return err
	}

	// Publish draft revision and update master pointer
	if err := repo.PublishDraftBlueprint(ctx, tx, blueprintID, draft.ID.String(), isComplete); err != nil {
		return mapBlueprintRepoErr(err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	return nil
}

// CreateBlueprintRevision creates a new draft revision cloned from the active published revision (or regenerated from source).
func (s *BlueprintDraftService) CreateBlueprintRevision(ctx context.Context, companyID, blueprintID string, req dto.CreateBlueprintRevisionRequest) (*dto.BlueprintRevisionResponse, error) {
	repo := s.getLifecycleRepo()
	if repo == nil {
		return nil, errors.New("repository not configured")
	}
	if companyID == "" {
		return nil, ErrBlueprintNotFound
	}
	companyUUID, err := uuid.Parse(companyID)
	if err != nil {
		return nil, ErrBlueprintNotFound
	}
	blueprintUUID, err := uuid.Parse(blueprintID)
	if err != nil {
		return nil, ErrBlueprintNotFound
	}

	mode := req.Mode
	if mode == "" {
		mode = "clone"
	}
	if mode != "clone" && mode != "regenerate_from_source" {
		return nil, errors.New("invalid revision mode: must be 'clone' or 'regenerate_from_source'")
	}

	tx, err := repo.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock master row
	if err := repo.LockBlueprintMaster(ctx, tx, blueprintID); err != nil {
		if errors.Is(err, repository.ErrBlueprintMasterNotFound) || errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	// Check if active draft revision already exists
	draft, err := repo.GetDraftRevision(ctx, tx, blueprintID, companyID)
	if err == nil && draft != nil {
		return nil, ErrBlueprintActiveDraftAlreadyExists
	}
	if err != nil && !errors.Is(err, repository.ErrBlueprintDraftNotFound) &&
		!errors.Is(err, ErrBlueprintDraftNotFound) &&
		!errors.Is(err, repository.ErrBlueprintRevisionNotFound) &&
		!errors.Is(err, repository.ErrBlueprintNotFound) {
		return nil, err
	}

	// Load active published revision
	pubRev, err := repo.GetActivePublishedRevision(ctx, tx, blueprintID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintRevisionNotFound) || errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintNoPublishedRevision
		}
		return nil, err
	}
	if pubRev == nil {
		return nil, ErrBlueprintNoPublishedRevision
	}

	// Get max revision number under lock
	maxRev, err := repo.GetMaxRevisionNumber(ctx, tx, blueprintID)
	if err != nil {
		return nil, err
	}
	newRevisionNumber := maxRev + 1

	// Handle Mode
	var newRev *domain.BlueprintRevision
	newID := uuid.New()

	switch mode {
	case "clone":
		completenessStatus := pubRev.CompletenessStatus
		if completenessStatus == "" {
			completenessStatus = "complete"
		}
		contentJSON := pubRev.ContentJSON
		if len(contentJSON) == 0 {
			contentJSON = json.RawMessage("{}")
		}
		sourceContext := pubRev.SourceContextSnapshot
		if len(sourceContext) == 0 {
			sourceContext = json.RawMessage("{}")
		}
		validationRes := pubRev.ValidationResult
		if len(validationRes) == 0 {
			validationRes = json.RawMessage("{}")
		}
		crossValRes := pubRev.CrossValidationResult
		if len(crossValRes) == 0 {
			crossValRes = json.RawMessage("{}")
		}

		newRev = &domain.BlueprintRevision{
			ID:                        newID,
			CompanyID:                 companyUUID,
			BlueprintID:               blueprintUUID,
			RevisionNumber:            newRevisionNumber,
			SourceQuotationRevisionID: pubRev.SourceQuotationRevisionID,
			SourceContextSnapshot:     sourceContext,
			PublicationStatus:         "draft",
			CompletenessStatus:        completenessStatus,
			GenerationStatus:          "completed",
			ContentJSON:               contentJSON,
			ContentVersion:            1,
			ValidationResult:          validationRes,
			CrossValidationResult:     crossValRes,
			SkillSnapshot:             pubRev.SkillSnapshot,
			SafetyEnvelopeVersion:     pubRev.SafetyEnvelopeVersion,
			AiProvider:                pubRev.AiProvider,
			AiModel:                   pubRev.AiModel,
			GeneratedAt:               pubRev.GeneratedAt,
			PublishedAt:               nil,
			CreatedBy:                 pubRev.CreatedBy,
		}

	case "regenerate_from_source":
		sourceQuotationRevID := pubRev.SourceQuotationRevisionID
		if req.SourceQuotationRevID != "" {
			parsedQRevID, parseErr := uuid.Parse(req.SourceQuotationRevID)
			if parseErr != nil {
				return nil, parseErr
			}
			sourceQuotationRevID = &parsedQRevID
		}

		sourceContext := pubRev.SourceContextSnapshot
		if len(sourceContext) == 0 {
			sourceContext = json.RawMessage("{}")
		}

		newRev = &domain.BlueprintRevision{
			ID:                        newID,
			CompanyID:                 companyUUID,
			BlueprintID:               blueprintUUID,
			RevisionNumber:            newRevisionNumber,
			SourceQuotationRevisionID: sourceQuotationRevID,
			SourceContextSnapshot:     sourceContext,
			PublicationStatus:         "draft",
			CompletenessStatus:        "partial",
			GenerationStatus:          "pending",
			ContentJSON:               json.RawMessage("{}"),
			ContentVersion:            1,
			ValidationResult:          json.RawMessage("{}"),
			CrossValidationResult:     json.RawMessage("{}"),
			SkillSnapshot:             pubRev.SkillSnapshot,
			SafetyEnvelopeVersion:     pubRev.SafetyEnvelopeVersion,
			AiProvider:                pubRev.AiProvider,
			AiModel:                   pubRev.AiModel,
			GeneratedAt:               nil,
			PublishedAt:               nil,
			CreatedBy:                 pubRev.CreatedBy,
		}
	}

	// Insert revision via CreateRevisionTx
	if err := repo.CreateRevisionTx(ctx, tx, newRev); err != nil {
		return nil, err
	}

	// Update blueprint master current revision via UpdateBlueprintCurrentRevision
	if err := repo.UpdateBlueprintCurrentRevision(ctx, tx, blueprintID, newRevisionNumber); err != nil {
		return nil, mapBlueprintRepoErr(err)
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Return mapped *dto.BlueprintRevisionResponse
	return &dto.BlueprintRevisionResponse{
		ID:            newRev.ID,
		BlueprintID:   newRev.BlueprintID,
		VersionNumber: newRev.RevisionNumber,
		Status:        newRev.PublicationStatus,
		CreatedAt:     newRev.CreatedAt,
		UpdatedAt:     newRev.UpdatedAt,
	}, nil
}

// CreateBlueprintRevisionSimple provides a simple parameter interface for CreateBlueprintRevision.
func (s *BlueprintDraftService) CreateBlueprintRevisionSimple(ctx context.Context, companyID, blueprintID, mode, sourceQuotationRevID string) (*dto.BlueprintRevisionResponse, error) {
	return s.CreateBlueprintRevision(ctx, companyID, blueprintID, dto.CreateBlueprintRevisionRequest{
		Mode:                 mode,
		SourceQuotationRevID: sourceQuotationRevID,
	})
}
