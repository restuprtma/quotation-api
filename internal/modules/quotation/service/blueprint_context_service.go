package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/net/html"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

// ErrBlueprintContextInvalidInput indicates that source projection or request parameters
// fail validation checks for blueprint context construction.
var ErrBlueprintContextInvalidInput = errors.New("invalid blueprint context input")

var adversarialInstructionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(?:all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)disregard\s+(?:all\s+)?previous\s+instructions`),
	regexp.MustCompile(`(?i)reveal\s+system\s+instructions`),
	regexp.MustCompile(`(?i)reveal\s+system\s+\x70\x72\x6f\x6d\x70\x74`),
	regexp.MustCompile(`(?i)bypass\s+(?:all\s+)?safety\s+filters`),
}

// BlueprintSourceProjection holds tenant-scoped quotation revision data needed
// for blueprint context normalization without exposing unauthorized commercial or compliance fields.
type BlueprintSourceProjection struct {
	CompanyID    uuid.UUID
	QuotationID  uuid.UUID
	RevisionID   uuid.UUID
	Subject      string
	SolutionType string
	ContentHTML  string
}

// NormalizedBlueprintContext contains sanitized JSON snapshots and the deterministic
// idempotency payload hash for blueprint generation.
type NormalizedBlueprintContext struct {
	SourceContextSnapshot  json.RawMessage
	ContextContent         json.RawMessage
	IdempotencyPayloadHash string
}

type sourceContextSnapshotPayload struct {
	Subject       string `json:"subject"`
	SolutionType  string `json:"solution_type"`
	Summary       string `json:"summary"`
	ExtractedText string `json:"extracted_text"`
}

type contextContentPayload struct {
	Title                  string   `json:"title"`
	SourceSubject          string   `json:"source_subject"`
	SolutionType           string   `json:"solution_type"`
	Summary                string   `json:"summary"`
	ExtractedText          string   `json:"extracted_text"`
	BusinessGoals          []string `json:"business_goals"`
	Actors                 []string `json:"actors"`
	Channels               []string `json:"channels"`
	BusinessPhases         []string `json:"business_phases"`
	BusinessRules          []string `json:"business_rules"`
	KnownStates            []string `json:"known_states"`
	Integrations           []string `json:"integrations"`
	TechnologyPreferences  []string `json:"technology_preferences"`
	OperationalConstraints []string `json:"operational_constraints"`
	AdditionalNotes        string   `json:"additional_notes"`
}

type canonicalPayloadHashSource struct {
	IdempotencyKey        string          `json:"idempotency_key"`
	CompanyID             string          `json:"company_id"`
	QuotationID           string          `json:"quotation_id"`
	SourceRevisionID      string          `json:"source_revision_id"`
	Title                 string          `json:"title"`
	SourceContextSnapshot json.RawMessage `json:"source_context_snapshot"`
	ContextContent        json.RawMessage `json:"context_content"`
}

// BlueprintSourceContextBuilder extracts and normalizes source context into immutable snapshots.
type BlueprintSourceContextBuilder struct {
	sanitizer HTMLSanitizer
}

// NewBlueprintSourceContextBuilder instantiates a context builder with strict HTML sanitization.
func NewBlueprintSourceContextBuilder() *BlueprintSourceContextBuilder {
	return &BlueprintSourceContextBuilder{
		sanitizer: NewHTMLSanitizer(),
	}
}

// Build normalizes source projection and request into canonical snapshot, content, and payload hash.
func (b *BlueprintSourceContextBuilder) Build(
	source BlueprintSourceProjection,
	request dto.CreateBlueprintGenerationRequest,
) (NormalizedBlueprintContext, error) {
	if source.CompanyID == uuid.Nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: company id is zero", ErrBlueprintContextInvalidInput)
	}
	if source.QuotationID == uuid.Nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: source quotation id is zero", ErrBlueprintContextInvalidInput)
	}
	if source.RevisionID == uuid.Nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: source revision id is zero", ErrBlueprintContextInvalidInput)
	}
	if request.IdempotencyKey == uuid.Nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: idempotency key is zero", ErrBlueprintContextInvalidInput)
	}
	if request.QuotationID == uuid.Nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: request quotation id is zero", ErrBlueprintContextInvalidInput)
	}
	if request.SourceQuotationRevisionID == uuid.Nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: request source quotation revision id is zero", ErrBlueprintContextInvalidInput)
	}

	if request.QuotationID != source.QuotationID {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: request quotation id does not match source", ErrBlueprintContextInvalidInput)
	}
	if request.SourceQuotationRevisionID != source.RevisionID {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: request source revision id does not match source", ErrBlueprintContextInvalidInput)
	}

	trimmedSubject := strings.TrimSpace(source.Subject)
	if trimmedSubject == "" {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: source subject is blank", ErrBlueprintContextInvalidInput)
	}

	trimmedSolutionType := strings.TrimSpace(source.SolutionType)
	if trimmedSolutionType == "" {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: source solution type is blank", ErrBlueprintContextInvalidInput)
	}

	trimmedHTML := strings.TrimSpace(source.ContentHTML)
	if trimmedHTML == "" {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: source content html is blank", ErrBlueprintContextInvalidInput)
	}

	trimmedTitle := strings.TrimSpace(request.Title)
	if trimmedTitle == "" {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: request title is blank", ErrBlueprintContextInvalidInput)
	}

	sanitizedHTML, err := b.sanitizer.Sanitize(trimmedHTML)
	if err != nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: html sanitization failed: %v", ErrBlueprintContextInvalidInput, err)
	}

	extractedText := extractDOMPlainText(sanitizedHTML)
	if strings.TrimSpace(extractedText) == "" {
		return NormalizedBlueprintContext{}, fmt.Errorf("%w: content html produces no usable plain text", ErrBlueprintContextInvalidInput)
	}

	neutralizedNotes := neutralizeAdversarialInstructions(strings.TrimSpace(request.AdditionalNotes))

	snapshotPayload := sourceContextSnapshotPayload{
		Subject:       trimmedSubject,
		SolutionType:  trimmedSolutionType,
		Summary:       extractedText,
		ExtractedText: extractedText,
	}
	snapshotBytes, err := json.Marshal(snapshotPayload)
	if err != nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("failed to marshal source context snapshot: %w", err)
	}

	contentPayload := contextContentPayload{
		Title:                  trimmedTitle,
		SourceSubject:          trimmedSubject,
		SolutionType:           trimmedSolutionType,
		Summary:                extractedText,
		ExtractedText:          extractedText,
		BusinessGoals:          cleanStringList(request.BusinessGoals),
		Actors:                 cleanStringList(request.Actors),
		Channels:               cleanStringList(request.Channels),
		BusinessPhases:         cleanStringList(request.BusinessPhases),
		BusinessRules:          cleanStringList(request.BusinessRules),
		KnownStates:            cleanStringList(request.KnownStates),
		Integrations:           cleanStringList(request.Integrations),
		TechnologyPreferences:  cleanStringList(request.TechnologyPreferences),
		OperationalConstraints: cleanStringList(request.OperationalConstraints),
		AdditionalNotes:        neutralizedNotes,
	}
	contentBytes, err := json.Marshal(contentPayload)
	if err != nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("failed to marshal context content: %w", err)
	}

	hashSource := canonicalPayloadHashSource{
		IdempotencyKey:        request.IdempotencyKey.String(),
		CompanyID:             source.CompanyID.String(),
		QuotationID:           source.QuotationID.String(),
		SourceRevisionID:      source.RevisionID.String(),
		Title:                 trimmedTitle,
		SourceContextSnapshot: snapshotBytes,
		ContextContent:        contentBytes,
	}
	hashSourceBytes, err := json.Marshal(hashSource)
	if err != nil {
		return NormalizedBlueprintContext{}, fmt.Errorf("failed to marshal canonical hash source: %w", err)
	}

	hashSum := sha256.Sum256(hashSourceBytes)
	idempotencyPayloadHash := hex.EncodeToString(hashSum[:])

	return NormalizedBlueprintContext{
		SourceContextSnapshot:  snapshotBytes,
		ContextContent:         contentBytes,
		IdempotencyPayloadHash: idempotencyPayloadHash,
	}, nil
}

func extractDOMPlainText(sanitizedHTML string) string {
	doc, err := html.Parse(strings.NewReader(sanitizedHTML))
	if err != nil {
		return ""
	}

	var sb strings.Builder
	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n == nil {
			return
		}
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript", "iframe", "object", "embed":
				return
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				if sb.Len() > 0 {
					sb.WriteString(" ")
				}
				sb.WriteString(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}
	traverse(doc)
	return strings.TrimSpace(sb.String())
}

func neutralizeAdversarialInstructions(input string) string {
	cleaned := input
	for _, p := range adversarialInstructionPatterns {
		cleaned = p.ReplaceAllString(cleaned, "")
	}
	return strings.Join(strings.Fields(cleaned), " ")
}

func cleanStringList(items []string) []string {
	if items == nil {
		return []string{}
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
