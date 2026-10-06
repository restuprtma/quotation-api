package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var (
	ErrPublicBlueprintNotFound = errors.New("blueprint not found or link revoked")
)

type PublicBlueprintDataLoader interface {
	LoadSharedRevisionData(ctx context.Context, blueprintID, revisionID uuid.UUID) (*domain.Blueprint, *domain.BlueprintRevision, []domain.BlueprintSectionRun, error)
}

type PublicBlueprintService struct {
	shareRepo ShareLinkDataRepository
	loader    PublicBlueprintDataLoader
}

func NewPublicBlueprintService(shareRepo ShareLinkDataRepository, loader PublicBlueprintDataLoader) *PublicBlueprintService {
	return &PublicBlueprintService{
		shareRepo: shareRepo,
		loader:    loader,
	}
}

func (s *PublicBlueprintService) GetPublicBlueprint(ctx context.Context, rawToken string) (*dto.ProgrammerBlueprintDTO, error) {
	if rawToken == "" {
		return nil, ErrPublicBlueprintNotFound
	}

	tokenHash := HashShareToken(rawToken)
	link, err := s.shareRepo.GetActiveByTokenHash(ctx, tokenHash)
	if err != nil || link == nil || link.IsRevoked() {
		return nil, ErrPublicBlueprintNotFound
	}

	// Asynchronous best-effort update of last_accessed_at
	go func(id uuid.UUID) {
		_ = s.shareRepo.TouchLastAccessed(context.Background(), id)
	}(link.ID)

	bp, rev, runs, err := s.loader.LoadSharedRevisionData(ctx, link.BlueprintID, link.BlueprintRevisionID)
	if err != nil || bp == nil || rev == nil {
		return nil, ErrPublicBlueprintNotFound
	}

	// Must be published or superseded, and have published_at timestamp
	if (rev.PublicationStatus != "published" && rev.PublicationStatus != "superseded") || rev.PublishedAt == nil {
		return nil, ErrPublicBlueprintNotFound
	}

	// Build safe completeness matrix
	matrix := make(map[string]string)
	for _, run := range runs {
		status := string(run.Status)
		if run.SectionType == "context" && status == "" {
			status = "validated"
		}
		matrix[string(run.SectionType)] = status
	}

	// Parse content_json strictly for allowlisted sections
	var rawBundle struct {
		Summary       string          `json:"summary"`
		Journey       json.RawMessage `json:"journey"`
		Architecture  json.RawMessage `json:"architecture"`
		StateMachine  json.RawMessage `json:"state_machine"`
		Workflow      json.RawMessage `json:"workflow"`
		Workflows     json.RawMessage `json:"workflows"`
		WBS           json.RawMessage `json:"wbs"`
		Assumptions   []string        `json:"assumptions"`
		OpenQuestions json.RawMessage `json:"open_questions"`
	}

	if len(rev.ContentJSON) > 0 && string(rev.ContentJSON) != "{}" && string(rev.ContentJSON) != "null" {
		_ = json.Unmarshal(rev.ContentJSON, &rawBundle)
	}

	workflowPayload := rawBundle.Workflows
	if len(workflowPayload) == 0 || string(workflowPayload) == "null" {
		workflowPayload = rawBundle.Workflow
	}

	// Parse open questions safely into string list
	openQuestionsList := make([]string, 0)
	if len(rawBundle.OpenQuestions) > 0 {
		var genericQuestions []interface{}
		if err := json.Unmarshal(rawBundle.OpenQuestions, &genericQuestions); err == nil {
			for _, q := range genericQuestions {
				if strQ, ok := q.(string); ok {
					openQuestionsList = append(openQuestionsList, strQ)
				} else if mapQ, ok := q.(map[string]interface{}); ok {
					if qText, ok := mapQ["question"].(string); ok {
						openQuestionsList = append(openQuestionsList, qText)
					}
				}
			}
		}
	}

	// Safe warnings extract
	safeWarnings := make([]string, 0)
	trimmedCross := bytes.TrimSpace(rev.CrossValidationResult)
	if len(trimmedCross) > 0 && string(trimmedCross) != "{}" && string(trimmedCross) != "null" {
		var crossRes struct {
			Issues []struct {
				Message string `json:"message"`
				Code    string `json:"code"`
			} `json:"issues"`
		}
		if err := json.Unmarshal(trimmedCross, &crossRes); err == nil {
			for _, issue := range crossRes.Issues {
				if issue.Message != "" {
					safeWarnings = append(safeWarnings, issue.Message)
				}
			}
		}
	}

	completeness := "complete"
	if rev.CompletenessStatus == "partial" {
		completeness = "partial"
	}

	publishedAtVal := time.Now().UTC()
	if rev.PublishedAt != nil {
		publishedAtVal = *rev.PublishedAt
	}

	return &dto.ProgrammerBlueprintDTO{
		Title:              bp.Title,
		RevisionNumber:     rev.RevisionNumber,
		CompletenessStatus: completeness,
		PublishedAt:        publishedAtVal,
		Summary:            rawBundle.Summary,
		Assumptions:        rawBundle.Assumptions,
		OpenQuestions:      openQuestionsList,
		Journey:            rawBundle.Journey,
		Architecture:       rawBundle.Architecture,
		StateMachine:       rawBundle.StateMachine,
		Workflow:           workflowPayload,
		WBS:                rawBundle.WBS,
		CompletenessMatrix: matrix,
		Warnings:           safeWarnings,
	}, nil
}
