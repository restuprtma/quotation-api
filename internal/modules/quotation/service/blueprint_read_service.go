package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	// ErrBlueprintNotFound indicates that the requested blueprint does not exist or belongs to another tenant.
	ErrBlueprintNotFound = repository.ErrBlueprintNotFound
	// ErrCorruptBlueprintContent indicates that stored blueprint JSON payload is malformed.
	ErrCorruptBlueprintContent = errors.New("corrupted blueprint content stored")
)

// BlueprintGenerationReader loads aggregate blueprint generation state.
type BlueprintGenerationReader interface {
	GetBlueprintGeneration(ctx context.Context, blueprintID, companyID uuid.UUID) (*repository.CreateBlueprintGenerationResult, error)
}

// BlueprintListReader loads projection items for tenant blueprint lists.
type BlueprintListReader interface {
	ListBlueprints(ctx context.Context, companyID uuid.UUID, params repository.BlueprintListParams) ([]dto.BlueprintListItemResponse, int, error)
}

// BlueprintRevisionHistoryReader loads revision history with details for a blueprint.
type BlueprintRevisionHistoryReader interface {
	ListBlueprintRevisionHistory(ctx context.Context, blueprintID, companyID uuid.UUID) ([]repository.BlueprintRevisionDetail, error)
}

// BlueprintReadService coordinates read-only operations for blueprints.
type BlueprintReadService struct {
	genReader  BlueprintGenerationReader
	listReader BlueprintListReader
	histReader BlueprintRevisionHistoryReader
}

// NewBlueprintReadService constructs a new BlueprintReadService.
func NewBlueprintReadService(genReader BlueprintGenerationReader, listReader BlueprintListReader, histReaders ...BlueprintRevisionHistoryReader) *BlueprintReadService {
	s := &BlueprintReadService{
		genReader:  genReader,
		listReader: listReader,
	}
	if len(histReaders) > 0 && histReaders[0] != nil {
		s.histReader = histReaders[0]
	} else if hr, ok := listReader.(BlueprintRevisionHistoryReader); ok {
		s.histReader = hr
	} else if hr, ok := genReader.(BlueprintRevisionHistoryReader); ok {
		s.histReader = hr
	}
	return s
}

// WithHistoryReader configures a BlueprintRevisionHistoryReader on the service.
func (s *BlueprintReadService) WithHistoryReader(histReader BlueprintRevisionHistoryReader) *BlueprintReadService {
	s.histReader = histReader
	return s
}

// GetStatus returns the public generation/publication status and canonical section statuses.
func (s *BlueprintReadService) GetStatus(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error) {
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintNotFound
	}

	result, err := s.genReader.GetBlueprintGeneration(ctx, blueprintID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}
	if result == nil || result.Blueprint.CompanyID != companyID || result.Blueprint.ID != blueprintID {
		return nil, ErrBlueprintNotFound
	}

	sections := mapSectionStatuses(result.SectionRuns, result.Revision.UpdatedAt)

	return &dto.BlueprintStatusResponse{
		BlueprintID:        result.Blueprint.ID,
		RevisionID:         result.Revision.ID,
		Title:              result.Blueprint.Title,
		GenerationStatus:   result.Revision.GenerationStatus,
		PublicationStatus:  result.Revision.PublicationStatus,
		CompletenessStatus: result.Revision.CompletenessStatus,
		ContentVersion:     result.Revision.ContentVersion,
		Sections:           sections,
		CreatedAt:          result.Blueprint.CreatedAt,
		UpdatedAt:          result.Revision.UpdatedAt,
	}, nil
}

// GetDetail returns full detail for a blueprint revision, masking sensitive configuration/credentials.
func (s *BlueprintReadService) GetDetail(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintDetailResponse, error) {
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintNotFound
	}

	result, err := s.genReader.GetBlueprintGeneration(ctx, blueprintID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}
	if result == nil || result.Blueprint.CompanyID != companyID || result.Blueprint.ID != blueprintID {
		return nil, ErrBlueprintNotFound
	}

	contentJSON, err := sanitizeRawJSON(result.Revision.ContentJSON)
	if err != nil {
		return nil, err
	}

	validationResult, err := sanitizeRawJSON(result.Revision.ValidationResult)
	if err != nil {
		return nil, err
	}

	crossValidationResult, err := sanitizeRawJSON(result.Revision.CrossValidationResult)
	if err != nil {
		return nil, err
	}

	sections := mapSectionStatuses(result.SectionRuns, result.Revision.UpdatedAt)

	status := dto.BlueprintStatusResponse{
		BlueprintID:        result.Blueprint.ID,
		RevisionID:         result.Revision.ID,
		Title:              result.Blueprint.Title,
		GenerationStatus:   result.Revision.GenerationStatus,
		PublicationStatus:  result.Revision.PublicationStatus,
		CompletenessStatus: result.Revision.CompletenessStatus,
		ContentVersion:     result.Revision.ContentVersion,
		Sections:           sections,
		CreatedAt:          result.Blueprint.CreatedAt,
		UpdatedAt:          result.Revision.UpdatedAt,
	}

	return &dto.BlueprintDetailResponse{
		Status:                    status,
		QuotationID:               result.Blueprint.QuotationID,
		SourceQuotationRevisionID: result.Revision.SourceQuotationRevisionID,
		ContentJSON:               contentJSON,
		ValidationResult:          validationResult,
		CrossValidationResult:     crossValidationResult,
	}, nil
}

// List returns tenant-filtered and paginated blueprint items.
func (s *BlueprintReadService) List(ctx context.Context, companyID uuid.UUID, params repository.BlueprintListParams) (*dto.BlueprintListResponse, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit < 1 {
		limit = 20
	} else if limit > 100 {
		limit = 100
	}

	if companyID == uuid.Nil {
		return &dto.BlueprintListResponse{
			Items:      make([]dto.BlueprintListItemResponse, 0),
			Total:      0,
			Page:       page,
			Limit:      limit,
			TotalPages: 0,
		}, nil
	}

	items, total, err := s.listReader.ListBlueprints(ctx, companyID, params)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = make([]dto.BlueprintListItemResponse, 0)
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}

	return &dto.BlueprintListResponse{
		Items:      items,
		Total:      total,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

func sanitizeRawJSON(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return json.RawMessage("{}"), nil
	}
	if !json.Valid(trimmed) {
		return nil, ErrCorruptBlueprintContent
	}
	return json.RawMessage(trimmed), nil
}

func mapSectionStatuses(runs []domain.BlueprintSectionRun, fallbackTime time.Time) []dto.BlueprintSectionStatusResponse {
	runsByType := make(map[string]domain.BlueprintSectionRun, len(runs))
	for _, run := range runs {
		runsByType[run.SectionType] = run
	}

	sections := make([]dto.BlueprintSectionStatusResponse, 0, len(domain.BlueprintSectionOrder))
	for _, secType := range domain.BlueprintSectionOrder {
		secTypeStr := string(secType)
		if run, ok := runsByType[secTypeStr]; ok {
			status := run.Status
			if secType == domain.BlueprintSectionContext && status == "" {
				status = string(domain.BlueprintSectionValidated)
			}
			sections = append(sections, dto.BlueprintSectionStatusResponse{
				Type:        secTypeStr,
				Status:      status,
				Attempts:    run.AttemptCount,
				ErrorCode:   run.SafeErrorCode,
				StartedAt:   run.StartedAt,
				CompletedAt: run.CompletedAt,
				UpdatedAt:   run.UpdatedAt,
			})
		} else {
			status := string(domain.BlueprintSectionPending)
			if secType == domain.BlueprintSectionContext {
				status = string(domain.BlueprintSectionValidated)
			}
			sections = append(sections, dto.BlueprintSectionStatusResponse{
				Type:      secTypeStr,
				Status:    status,
				Attempts:  0,
				UpdatedAt: fallbackTime,
			})
		}
	}
	return sections
}

// ListRevisions returns the revision history for a blueprint scoped by company ID, ordered by revision_number DESC,
// along with completeness status, source quotation revision info, and section completeness matrix.
func (s *BlueprintReadService) ListRevisions(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintRevisionHistoryItem, error) {
	if companyID == uuid.Nil || blueprintID == uuid.Nil {
		return nil, ErrBlueprintNotFound
	}

	histReader := s.histReader
	if histReader == nil {
		if hr, ok := s.listReader.(BlueprintRevisionHistoryReader); ok {
			histReader = hr
		} else if hr, ok := s.genReader.(BlueprintRevisionHistoryReader); ok {
			histReader = hr
		}
	}
	if histReader == nil {
		return nil, errors.New("blueprint revision history reader not configured")
	}

	records, err := histReader.ListBlueprintRevisionHistory(ctx, blueprintID, companyID)
	if err != nil {
		if errors.Is(err, repository.ErrBlueprintNotFound) {
			return nil, ErrBlueprintNotFound
		}
		return nil, err
	}

	items := make([]dto.BlueprintRevisionHistoryItem, 0, len(records))
	for _, rec := range records {
		matrix := buildCompletenessMatrix(rec.SectionRuns, rec.Revision.ValidationResult)
		sourceRevNum := rec.SourceQuotationRevNumber
		if sourceRevNum == nil && len(rec.Revision.SourceContextSnapshot) > 0 {
			var snap struct {
				RevisionNumber     *int `json:"revision_number"`
				QuotationRevNumber *int `json:"quotation_rev_number"`
			}
			if err := json.Unmarshal(rec.Revision.SourceContextSnapshot, &snap); err == nil {
				if snap.RevisionNumber != nil {
					sourceRevNum = snap.RevisionNumber
				} else if snap.QuotationRevNumber != nil {
					sourceRevNum = snap.QuotationRevNumber
				}
			}
		}

		items = append(items, dto.BlueprintRevisionHistoryItem{
			ID:                        rec.Revision.ID,
			RevisionNumber:            rec.Revision.RevisionNumber,
			PublicationStatus:         rec.Revision.PublicationStatus,
			CompletenessStatus:        rec.Revision.CompletenessStatus,
			SourceQuotationRevisionID: rec.Revision.SourceQuotationRevisionID,
			SourceQuotationRevNumber:  sourceRevNum,
			CompletenessMatrix:        matrix,
			CreatedAt:                 rec.Revision.CreatedAt,
			GeneratedAt:               rec.Revision.GeneratedAt,
			PublishedAt:               rec.Revision.PublishedAt,
			CreatedBy:                 rec.Revision.CreatedBy,
		})
	}

	return items, nil
}

// GetBlueprintHistory is an alias for ListRevisions.
func (s *BlueprintReadService) GetBlueprintHistory(ctx context.Context, companyID, blueprintID uuid.UUID) ([]dto.BlueprintRevisionHistoryItem, error) {
	return s.ListRevisions(ctx, companyID, blueprintID)
}

func buildCompletenessMatrix(runs []domain.BlueprintSectionRun, validationResult json.RawMessage) map[string]string {
	matrix := make(map[string]string)
	for _, run := range runs {
		status := run.Status
		if run.SectionType == "context" && status == "" {
			status = string(domain.BlueprintSectionValidated)
		}
		matrix[run.SectionType] = status
	}
	if len(matrix) == 0 && len(validationResult) > 0 {
		var vr map[string]interface{}
		if err := json.Unmarshal(validationResult, &vr); err == nil {
			for k, v := range vr {
				if strVal, ok := v.(string); ok {
					matrix[k] = strVal
				} else if mapVal, ok := v.(map[string]interface{}); ok {
					if status, ok := mapVal["status"].(string); ok {
						matrix[k] = status
					}
				}
			}
		}
	}
	return matrix
}
