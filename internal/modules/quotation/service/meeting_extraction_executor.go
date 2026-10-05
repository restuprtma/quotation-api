package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

// ErrMeetingExtractionExecutor is intentionally safe to persist as a worker
// failure.  It never includes revision JSON, notes, credentials, prompts, or
// provider response data.
var ErrMeetingExtractionExecutor = errors.New("meeting extraction executor failed")

// MeetingExtractionCatalogSnapshot is obtained once for a claimed attempt.
// Items and Hash must describe the same immutable snapshot; neither may be
// substituted from live catalog data after a phase has started.
type MeetingExtractionCatalogSnapshot struct {
	Items      []domain.RateCardItem
	Hash       string
	RateCardID string
}

// MeetingExtractionCatalogSnapshotLoader is deliberately narrow so the
// composition root can load the revision's persisted rate-card snapshot
// without giving phase code access to mutable catalog repositories.
type MeetingExtractionCatalogSnapshotLoader interface {
	LoadExtractionCatalogSnapshot(context.Context, *domain.QuotationRevision) (MeetingExtractionCatalogSnapshot, error)
}

// MeetingExtractionExecutorOptions contains production dependencies only.
// Provider resolution remains exclusively in ProviderSource (normally
// DBCredentialProviderSource), preserving the credential factory/cipher path.
type MeetingExtractionExecutorOptions struct {
	Revisions *repository.RevisionRepository
	Providers ProviderSource
	Catalog   MeetingExtractionCatalogSnapshotLoader
	Store     MeetingExtractionOrchestratorStore
	P5        MeetingExtractionP5Func

	PipelineVersion string
	PromptVersion   string
	SchemaVersion   string
}

// MeetingExtractionExecutor composes one orchestration attempt.  It retains
// notes only in the local MeetingNoteDocument until Run returns.
type MeetingExtractionExecutor struct {
	revisions *repository.RevisionRepository
	providers ProviderSource
	catalog   MeetingExtractionCatalogSnapshotLoader
	store     MeetingExtractionOrchestratorStore
	p5        MeetingExtractionP5Func

	pipelineVersion string
	promptVersion   string
	schemaVersion   string
}

// NewMeetingExtractionExecutor fails fast instead of leaving an enabled
// pipeline with a runtime fallback.  P5 is required: a nil/no-op finalizer is
// not a valid production composition.
func NewMeetingExtractionExecutor(options MeetingExtractionExecutorOptions) (*MeetingExtractionExecutor, error) {
	if options.Revisions == nil || options.Providers == nil || options.Catalog == nil || options.Store == nil || options.P5 == nil {
		return nil, fmt.Errorf("%w: incomplete dependencies", ErrMeetingExtractionExecutor)
	}
	if strings.TrimSpace(options.PipelineVersion) == "" || strings.TrimSpace(options.PromptVersion) == "" || strings.TrimSpace(options.SchemaVersion) == "" {
		return nil, fmt.Errorf("%w: version is required", ErrMeetingExtractionExecutor)
	}
	return &MeetingExtractionExecutor{
		revisions: options.Revisions, providers: options.Providers, catalog: options.Catalog, store: options.Store, p5: options.P5,
		pipelineVersion: options.PipelineVersion, promptVersion: options.PromptVersion, schemaVersion: options.SchemaVersion,
	}, nil
}

// Execute implements the worker executor contract without importing worker.
// All user text remains in the local document; returned errors are allowlisted
// sentinels or static composition failures.
func (e *MeetingExtractionExecutor) Execute(ctx context.Context, run *domain.ExtractionRun) error {
	if e == nil || run == nil || run.ID == uuid.Nil || run.CompanyID == uuid.Nil || run.RevisionID == uuid.Nil || run.Fence < 0 {
		return fmt.Errorf("%w: invalid run", ErrMeetingExtractionExecutor)
	}
	revision, err := e.revisions.GetByID(ctx, run.RevisionID.String())
	if err != nil || revision == nil || revision.ID != run.RevisionID || revision.CompanyID != run.CompanyID || revision.QuotationID != run.QuotationID {
		return fmt.Errorf("%w: revision unavailable", ErrMeetingExtractionExecutor)
	}
	notes, err := extractionNotesFromGenerationInput(revision.GenerationInput)
	if err != nil {
		return fmt.Errorf("%w: revision notes unavailable", ErrMeetingExtractionExecutor)
	}
	document, err := NewMeetingNoteDocument(notes, MeetingNoteDocumentConfig{})
	if err != nil {
		return fmt.Errorf("%w: revision notes invalid", ErrMeetingExtractionExecutor)
	}
	if !strings.EqualFold(document.Hash(), run.NoteHash) {
		// This is a terminal, non-retryable lifecycle outcome.  Record only
		// the allowlisted code; neither the persisted failure nor the returned
		// error may carry any portion of the meeting notes.
		if markErr := e.store.MarkFailed(ctx, MeetingExtractionFailure{
			RunID: run.ID, Fence: run.Fence, ErrorCode: ErrExtractionNotesChanged, Retryable: false,
		}); markErr != nil {
			return markErr
		}
		return ErrMeetingExtractionNotesChanged
	}

	// The snapshot is deliberately resolved before creating phases and copied
	// before it crosses the executor boundary, making it immutable per attempt.
	catalog, err := e.catalog.LoadExtractionCatalogSnapshot(ctx, revision)
	if err != nil || len(catalog.Hash) != 64 {
		return fmt.Errorf("%w: catalog snapshot unavailable", ErrMeetingExtractionExecutor)
	}
	catalog.Items = append([]domain.RateCardItem(nil), catalog.Items...)
	contextSnapshot, err := e.revisions.GetExtractionP5Context(ctx, run.RevisionID.String(), run.CompanyID.String())
	if err != nil {
		return fmt.Errorf("%w: p5 context unavailable", ErrMeetingExtractionExecutor)
	}

	provider, providerName, model, err := e.providers.ProviderForCompany(ctx, run.CompanyID)
	if err != nil {
		return fmt.Errorf("%w: provider unavailable", ErrMeetingExtractionExecutor)
	}
	extractionProvider, ok := provider.(ExtractionProvider)
	if !ok || extractionProvider == nil || strings.TrimSpace(providerName) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("%w: extraction provider unavailable", ErrMeetingExtractionExecutor)
	}

	orchestrator, err := NewMeetingExtractionOrchestrator(
		extractionProvider,
		e.store,
		NewDefaultMeetingExtractionPhaseSet(catalog.Items),
		e.p5,
		MeetingExtractionOrchestratorConfig{},
	)
	if err != nil {
		return fmt.Errorf("%w: orchestrator unavailable", ErrMeetingExtractionExecutor)
	}
	_, err = orchestrator.Run(ctx, MeetingExtractionOrchestratorInput{
		RunID: run.ID, Fence: run.Fence, Document: document, ExpectedNoteHash: run.NoteHash,
		PipelineVersion: e.pipelineVersion, PromptVersion: e.promptVersion, SchemaVersion: e.schemaVersion,
		CatalogSnapshotHash: catalog.Hash, Provider: providerName, Model: model,
		CompanyID: run.CompanyID, QuotationID: run.QuotationID, RevisionID: run.RevisionID,
		SolutionType: contextSnapshot.SolutionType, Metadata: QuotationMetadata{QuotationNumber: contextSnapshot.QuotationNumber, Subject: contextSnapshot.Subject, ClientName: contextSnapshot.ClientName, ClientCompanyName: contextSnapshot.ClientCompanyName}, QuotationDate: contextSnapshot.QuotationDate, LegalHTML: legalHTMLFromSnapshot(contextSnapshot.LegalTemplateSnapshot), RateCardID: catalog.RateCardID, Catalog: catalog.Items,
	})
	return err
}

func extractionNotesFromGenerationInput(input json.RawMessage) (string, error) {
	var payload struct {
		MeetingNotes string `json:"meeting_notes"`
	}
	if !json.Valid(input) || json.Unmarshal(input, &payload) != nil || strings.TrimSpace(payload.MeetingNotes) == "" {
		return "", ErrMeetingExtractionExecutor
	}
	return payload.MeetingNotes, nil
}

func legalHTMLFromSnapshot(raw []byte) string {
	var snapshot struct {
		ContentHTML string `json:"content_html"`
	}
	if json.Unmarshal(raw, &snapshot) == nil && strings.TrimSpace(snapshot.ContentHTML) != "" {
		return snapshot.ContentHTML
	}
	return ""
}
