package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"strings"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type PromptNarrativeRevisionService struct {
	repo      promptRevisionRepository
	sanitizer HTMLSanitizer
}

func NewPromptNarrativeRevisionService(repo promptRevisionRepository, sanitizer HTMLSanitizer) *PromptNarrativeRevisionService {
	if sanitizer == nil {
		sanitizer = NewHTMLSanitizer()
	}
	return &PromptNarrativeRevisionService{repo: repo, sanitizer: sanitizer}
}

func (s *PromptNarrativeRevisionService) Apply(ctx context.Context, companyID, quotationID uuid.UUID, userID *uuid.UUID, intent PromptRevisionIntent, req dto.ReviseSectionRequest) (*dto.PromptPatchResponse, error) {
	_ = userID
	if s == nil || s.repo == nil || companyID == uuid.Nil || quotationID == uuid.Nil || req.RevisionID == uuid.Nil || req.ContentVersion < 1 || req.StateVersion < 0 || !validPromptRevisionInstruction(req.Instruction) || !validatePromptRevisionIntent(intent, req.TargetSection) || intent.Intent != PromptRevisionIntentNarrative {
		return nil, promptRevisionError("invalid_revision", ErrPromptRevisionInvalidIntent)
	}
	record, err := s.repo.LoadPromptDraft(ctx, companyID, quotationID)
	if err != nil {
		return nil, promptRevisionLoadError(err)
	}
	if record == nil || record.RevisionID != req.RevisionID || record.ContentVersion != req.ContentVersion || record.StateVersion != req.StateVersion {
		return nil, promptRevisionError("revision_conflict", repository.ErrRevisionConflict)
	}
	var state domain.PromptQuotationState
	if err := json.Unmarshal(record.PromptState, &state); err != nil || state.Validate() != nil {
		return nil, promptRevisionError("revision_state_unavailable", errors.New("stored prompt state is invalid"))
	}
	replacement, err := s.sanitizer.Sanitize(renderPromptNarrativeSection(intent.TargetSection, intent.ReplacementText))
	if err != nil || strings.TrimSpace(replacement) == "" || strings.Contains(strings.ToLower(replacement), "<script") {
		return nil, promptRevisionError("unsafe_html", errors.New("narrative fragment was rejected"))
	}
	merged, err := MergePromptSection(record.ContentHTML, intent.TargetSection, replacement)
	if err != nil {
		return nil, promptRevisionError("section_merge_invalid", err)
	}
	stateJSON, _ := MarshalPromptQuotationStateForPersistence(&state)
	sectionJSON, _ := json.Marshal(promptSectionHashes(merged))
	hash := sha256.Sum256([]byte(merged))
	contentHash := hex.EncodeToString(hash[:])
	saved, err := s.repo.UpdatePromptDraftCAS(ctx, repository.UpdatePromptDraftCASParams{
		CompanyID: companyID, QuotationID: quotationID, RevisionID: record.RevisionID,
		ExpectedContentVersion: req.ContentVersion, ExpectedStateVersion: req.StateVersion,
		State: stateJSON, ContentHTML: merged,
		Snapshots: repository.PromptDraftSnapshots{AllocationSnapshot: record.AllocationSnapshot, RateCardSnapshot: record.RateCardSnapshot, LegalTemplateSnapshot: record.LegalTemplateSnapshot, SkillSnapshot: record.SkillSnapshot, ResolvedProfile: record.ResolvedProfile, Calculation: record.Calculation, Module: record.Module, Timeline: record.Timeline, SectionHashes: sectionJSON, ContentHash: &contentHash},
	})
	if err != nil {
		if errors.Is(err, repository.ErrRevisionConflict) {
			return nil, promptRevisionError("revision_conflict", err)
		}
		return nil, promptRevisionError("revision_update_failed", err)
	}
	var calculation domain.DedicatedSquadCalculation
	_ = json.Unmarshal(record.Calculation, &calculation)
	result := &dto.PromptPatchResponse{QuotationID: quotationID, RevisionID: record.RevisionID, ContentHTML: merged, ContentVersion: saved.ContentVersion, StateVersion: saved.StateVersion, PromptState: dto.NewPromptQuotationStateView(state), Issues: state.Issues, Readiness: state.Readiness, Calculation: calculation}
	if _, items, snapshotErr := promptRateCardSnapshot(record.RateCardSnapshot); snapshotErr == nil {
		result.CatalogRoles = PromptCatalogRoles(items)
	}
	return result, nil
}

func renderPromptNarrativeSection(section, replacement string) string {
	value := strings.TrimSpace(replacement)
	lines := strings.Split(value, "\n")
	list := len(lines) > 1
	for _, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "- ") && !strings.HasPrefix(strings.TrimSpace(line), "* ") {
			list = false
			break
		}
	}
	var body strings.Builder
	if list {
		body.WriteString("<ul>")
		for _, line := range lines {
			body.WriteString("<li>" + html.EscapeString(strings.TrimSpace(line)[2:]) + "</li>")
		}
		body.WriteString("</ul>")
	} else {
		for _, paragraph := range strings.Split(value, "\n\n") {
			paragraph = strings.TrimSpace(paragraph)
			if paragraph != "" {
				body.WriteString("<p>" + html.EscapeString(paragraph) + "</p>")
			}
		}
	}
	return `<section data-quotation-section="` + section + `" data-section-version="1">` + body.String() + `</section>`
}

type promptSectionRevisionClassifier interface {
	Classify(context.Context, uuid.UUID, domain.PromptQuotationState, string, string, string) (PromptRevisionIntent, error)
}

type promptSectionRevisionCommercial interface {
	ApplyCommercialPatch(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, dto.PromptPatchRequest) (*dto.PromptPatchResponse, error)
}

type PromptSectionRevisionService struct {
	repo       promptRevisionRepository
	classifier promptSectionRevisionClassifier
	narrative  *PromptNarrativeRevisionService
	commercial promptSectionRevisionCommercial
}

func NewPromptSectionRevisionService(repo promptRevisionRepository, classifier promptSectionRevisionClassifier, narrative *PromptNarrativeRevisionService, commercial promptSectionRevisionCommercial) *PromptSectionRevisionService {
	return &PromptSectionRevisionService{repo: repo, classifier: classifier, narrative: narrative, commercial: commercial}
}

func (s *PromptSectionRevisionService) Revise(ctx context.Context, companyID, quotationID uuid.UUID, userID *uuid.UUID, req dto.ReviseSectionRequest) (*dto.PromptPatchResponse, error) {
	if s == nil || s.repo == nil || s.classifier == nil || !validPromptRevisionInstruction(req.Instruction) || !validPromptRevisionRequestedSection(req.TargetSection) || req.RevisionID == uuid.Nil || req.ContentVersion < 1 || req.StateVersion < 0 {
		return nil, promptRevisionError("invalid_revision", ErrPromptRevisionInvalidIntent)
	}
	record, err := s.repo.LoadPromptDraft(ctx, companyID, quotationID)
	if err != nil {
		return nil, promptRevisionLoadError(err)
	}
	if record == nil || record.RevisionID != req.RevisionID || record.ContentVersion != req.ContentVersion || record.StateVersion != req.StateVersion {
		return nil, promptRevisionError("revision_conflict", repository.ErrRevisionConflict)
	}
	var state domain.PromptQuotationState
	if err := json.Unmarshal(record.PromptState, &state); err != nil || state.Validate() != nil {
		return nil, promptRevisionError("revision_state_unavailable", ErrPromptRevisionInvalidIntent)
	}
	sectionText := ""
	if req.TargetSection != "" {
		markup, err := promptSectionMarkup(record.ContentHTML, req.TargetSection)
		if err != nil {
			return nil, promptRevisionError("section_merge_invalid", err)
		}
		sectionText = strings.TrimSpace(stripPromptHTML(markup))
	}
	intent, err := s.classifier.Classify(ctx, companyID, state, req.Instruction, req.TargetSection, sectionText)
	if err != nil {
		if errors.Is(err, ErrPromptRevisionProvider) {
			return nil, promptRevisionError("revision_provider_failed", err)
		}
		return nil, promptRevisionError("invalid_revision", err)
	}
	switch intent.Intent {
	case PromptRevisionIntentNarrative:
		if s.narrative == nil {
			return nil, promptRevisionError("revision_update_failed", errors.New("narrative service unavailable"))
		}
		return s.narrative.Apply(ctx, companyID, quotationID, userID, intent, req)
	case PromptRevisionIntentCommercial:
		if s.commercial == nil {
			return nil, promptRevisionError("revision_update_failed", errors.New("commercial service unavailable"))
		}
		return s.commercial.ApplyCommercialPatch(ctx, companyID, quotationID, userID, dto.PromptPatchRequest{RevisionID: req.RevisionID, ContentVersion: req.ContentVersion, StateVersion: req.StateVersion, Operations: intent.CommercialPatch})
	case PromptRevisionIntentMaintenance:
		return nil, promptRevisionError("maintenance_clarification_required", errors.New("maintenance needs a confirmed priced service"))
	default:
		return nil, promptRevisionError("invalid_revision", ErrPromptRevisionInvalidIntent)
	}
}

func stripPromptHTML(markup string) string {
	inside := false
	var text strings.Builder
	for _, r := range markup {
		switch r {
		case '<':
			inside = true
		case '>':
			inside = false
		default:
			if !inside {
				text.WriteRune(r)
			}
		}
	}
	return html.UnescapeString(text.String())
}
