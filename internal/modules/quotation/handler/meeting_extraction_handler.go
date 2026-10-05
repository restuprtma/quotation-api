package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

const defaultMeetingExtractionPipelineVersion = "meeting-extraction-v1"

var (
	ErrMeetingNotesChanged             = errors.New("meeting notes changed")
	ErrMeetingExtractionConflict       = errors.New("meeting extraction retry conflict")
	ErrMeetingExtractionLegacyFallback = errors.New("meeting extraction uses legacy path")
)

// MeetingExtractionAPI is the narrow HTTP boundary. Keeping the handler on
// this interface makes all concurrency, ETag, and redaction rules testable
// without a provider or worker in the request path.
type MeetingExtractionAPI interface {
	Start(context.Context, uuid.UUID, uuid.UUID, dto.StartMeetingExtractionRequest) (*dto.MeetingExtractionStartResponse, error)
	Status(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*dto.MeetingExtractionStatusResponse, error)
	Retry(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, dto.RetryMeetingExtractionRequest) (*dto.MeetingExtractionStartResponse, error)
}

type MeetingExtractionStartDecider interface {
	Decide(context.Context, uuid.UUID, dto.StartMeetingExtractionRequest) (bool, error)
}

type MeetingExtractionHandler struct {
	api MeetingExtractionAPI
}

func NewMeetingExtractionHandler(api MeetingExtractionAPI) *MeetingExtractionHandler {
	return &MeetingExtractionHandler{api: api}
}

func (h *MeetingExtractionHandler) Start(c *gin.Context) {
	quotationID, ok := parseQuotationID(c)
	if !ok {
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	var req dto.StartMeetingExtractionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request payload", false, nil)
		return
	}
	if req.ServiceScheme == "" {
		req.ServiceScheme = "auto"
	}
	if req.PipelineVersion == "" {
		req.PipelineVersion = defaultMeetingExtractionPipelineVersion
	}
	if !validMeetingExtractionStartRequest(req) {
		writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request payload", false, nil)
		return
	}
	if h == nil || h.api == nil {
		writeMeetingExtractionError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", false, nil)
		return
	}

	result, err := h.api.Start(c.Request.Context(), companyID, quotationID, req)
	if err != nil {
		h.writeAPIError(c, err)
		return
	}
	c.Header("Location", result.StatusURL)
	c.JSON(http.StatusAccepted, gin.H{"data": result})
}

func (h *MeetingExtractionHandler) Status(c *gin.Context) {
	quotationID, ok := parseQuotationID(c)
	if !ok {
		return
	}
	runID, err := uuid.Parse(c.Param("run_id"))
	if err != nil || runID == uuid.Nil {
		writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid run id", false, nil)
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	if h == nil || h.api == nil {
		writeMeetingExtractionError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", false, nil)
		return
	}

	status, err := h.api.Status(c.Request.Context(), companyID, quotationID, runID)
	if err != nil {
		h.writeAPIError(c, err)
		return
	}
	etag := meetingExtractionETag(status)
	c.Header("ETag", etag)
	if strings.TrimSpace(c.Request.Header.Get("If-None-Match")) == etag {
		c.AbortWithStatus(http.StatusNotModified)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": status})
}

func (h *MeetingExtractionHandler) Retry(c *gin.Context) {
	quotationID, ok := parseQuotationID(c)
	if !ok {
		return
	}
	runID, err := uuid.Parse(c.Param("run_id"))
	if err != nil || runID == uuid.Nil {
		writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid run id", false, nil)
		return
	}
	companyID, ok := companyUUID(c)
	if !ok {
		return
	}
	var req dto.RetryMeetingExtractionRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, context.Canceled) {
		// An empty body is the documented equivalent of an omitted note_hash.
		if c.Request.ContentLength != 0 {
			writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request payload", false, nil)
			return
		}
	}
	if req.NoteHash != "" && !isSHA256Hex(req.NoteHash) {
		writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid note hash", false, nil)
		return
	}
	if h == nil || h.api == nil {
		writeMeetingExtractionError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", false, nil)
		return
	}

	result, err := h.api.Retry(c.Request.Context(), companyID, quotationID, runID, req)
	if err != nil {
		h.writeAPIError(c, err)
		return
	}
	c.Header("Location", result.StatusURL)
	c.JSON(http.StatusAccepted, gin.H{"data": result})
}

func (h *MeetingExtractionHandler) writeAPIError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrMeetingExtractionLegacyFallback):
		writeMeetingExtractionError(c, http.StatusConflict, "LEGACY_PATH_REQUIRED", "Meeting extraction is handled by the legacy path", false, nil)
	case errors.Is(err, repository.ErrExtractionNotFound), errors.Is(err, repository.ErrRevisionNotFound):
		writeMeetingExtractionError(c, http.StatusNotFound, "NOT_FOUND", "Meeting extraction run not found", false, nil)
	case errors.Is(err, repository.ErrExtractionActiveRun), errors.Is(err, ErrMeetingNotesChanged):
		writeMeetingExtractionError(c, http.StatusConflict, "MEETING_NOTES_CHANGED", "Meeting notes changed; create a new extraction run", false, nil)
	case errors.Is(err, ErrMeetingExtractionConflict):
		writeMeetingExtractionError(c, http.StatusConflict, "EXTRACTION_RETRY_CONFLICT", "Meeting extraction cannot be retried in its current state", false, nil)
	case errors.Is(err, repository.ErrExtractionBudgetExhausted):
		writeMeetingExtractionError(c, http.StatusUnprocessableEntity, "AI_BUDGET_EXHAUSTED", "AI extraction budget exhausted", false, nil)
	default:
		writeMeetingExtractionError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", false, nil)
	}
}

func writeMeetingExtractionError(c *gin.Context, status int, code, message string, retryable bool, retryAfter *int64) {
	c.JSON(status, dto.MeetingExtractionErrorResponse{Code: code, Message: message, Retryable: retryable, RetryAfter: retryAfter})
}

func parseQuotationID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || id == uuid.Nil {
		writeMeetingExtractionError(c, http.StatusBadRequest, "INVALID_REQUEST", "Invalid quotation id", false, nil)
		return uuid.Nil, false
	}
	return id, true
}

func validMeetingExtractionStartRequest(req dto.StartMeetingExtractionRequest) bool {
	if req.PipelineVersion != defaultMeetingExtractionPipelineVersion {
		return false
	}
	if req.ServiceScheme != "auto" && !domain.IsCommercialScheme(req.ServiceScheme) {
		return false
	}
	return req.IdempotencyKey == "" || len(req.IdempotencyKey) <= 128
}

func meetingExtractionETag(status *dto.MeetingExtractionStatusResponse) string {
	payload, _ := json.Marshal(status)
	digest := sha256.Sum256(payload)
	return `"` + hex.EncodeToString(digest[:]) + `"`
}

func isSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// repositoryMeetingExtractionAPI adapts the durable M3 repositories to the
// HTTP boundary. It only returns safe DTO projections; worker execution stays
// outside this adapter.
type repositoryMeetingExtractionAPI struct {
	runs    *repository.MeetingExtractionRepository
	draft   *repository.GenerationRepository
	decider MeetingExtractionStartDecider
}

func NewRepositoryMeetingExtractionAPI(runs *repository.MeetingExtractionRepository, draft *repository.GenerationRepository, decider ...MeetingExtractionStartDecider) MeetingExtractionAPI {
	var policy MeetingExtractionStartDecider
	if len(decider) > 0 {
		policy = decider[0]
	}
	return &repositoryMeetingExtractionAPI{runs: runs, draft: draft, decider: policy}
}

func (a *repositoryMeetingExtractionAPI) Start(ctx context.Context, companyID, quotationID uuid.UUID, req dto.StartMeetingExtractionRequest) (*dto.MeetingExtractionStartResponse, error) {
	if a == nil || a.runs == nil || a.draft == nil {
		return nil, errors.New("meeting extraction api is not initialized")
	}
	if a.decider != nil {
		allowed, err := a.decider.Decide(ctx, companyID, req)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrMeetingExtractionLegacyFallback
		}
	}
	draft, err := a.draft.LoadDraftRevisionForRegeneration(ctx, companyID, quotationID)
	if err != nil {
		return nil, err
	}
	notes, err := meetingNotesFromGenerationInput(draft.GenerationInput)
	if err != nil {
		return nil, err
	}
	doc, err := service.NewMeetingNoteDocument(notes, service.MeetingNoteDocumentConfig{})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.MeetingNotes) != "" {
		supplied, err := service.NewMeetingNoteDocument(req.MeetingNotes, service.MeetingNoteDocumentConfig{})
		if err != nil {
			return nil, err
		}
		if supplied.Hash() != doc.Hash() {
			return nil, ErrMeetingNotesChanged
		}
	}
	created, err := a.runs.CreateOrReuseRun(ctx, repository.CreateExtractionRunParams{
		CompanyID: companyID, QuotationID: quotationID, RevisionID: draft.RevisionID,
		NoteHash: doc.Hash(), PipelineVersion: req.PipelineVersion,
	})
	if err != nil {
		return nil, err
	}
	return meetingExtractionStartResponse(created.Run), nil
}

func (a *repositoryMeetingExtractionAPI) Status(ctx context.Context, companyID, quotationID, runID uuid.UUID) (*dto.MeetingExtractionStatusResponse, error) {
	if a == nil || a.runs == nil {
		return nil, errors.New("meeting extraction api is not initialized")
	}
	run, phases, err := a.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.CompanyID != companyID || run.QuotationID != quotationID {
		return nil, repository.ErrExtractionNotFound
	}
	return meetingExtractionStatusResponse(run, phases), nil
}

func (a *repositoryMeetingExtractionAPI) Retry(ctx context.Context, companyID, quotationID, runID uuid.UUID, req dto.RetryMeetingExtractionRequest) (*dto.MeetingExtractionStartResponse, error) {
	if a == nil || a.runs == nil {
		return nil, errors.New("meeting extraction api is not initialized")
	}
	run, _, err := a.runs.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.CompanyID != companyID || run.QuotationID != quotationID {
		return nil, repository.ErrExtractionNotFound
	}
	if req.NoteHash != "" && !strings.EqualFold(req.NoteHash, run.NoteHash) {
		return nil, ErrMeetingNotesChanged
	}
	switch run.Status {
	case domain.ExtractionRunQueued, domain.ExtractionRunRunning:
		return nil, ErrMeetingExtractionConflict
	case domain.ExtractionRunWaitingRateLimit:
		return meetingExtractionStartResponse(run), nil
	case domain.ExtractionRunFailed:
		if err := a.runs.TransitionRun(ctx, run.ID, run.Fence, domain.ExtractionRunQueued); err != nil {
			return nil, err
		}
		run.Status = domain.ExtractionRunQueued
		return meetingExtractionStartResponse(run), nil
	default:
		return nil, ErrMeetingExtractionConflict
	}
}

func meetingExtractionStartResponse(run *domain.ExtractionRun) *dto.MeetingExtractionStartResponse {
	return &dto.MeetingExtractionStartResponse{
		RunID: run.ID.String(), QuotationID: run.QuotationID.String(), Status: string(run.Status),
		StatusURL: fmt.Sprintf("/quotation/v1/quotations/%s/meeting-extraction/%s", run.QuotationID, run.ID),
	}
}

func meetingExtractionStatusResponse(run *domain.ExtractionRun, phases []domain.ExtractionPhase) *dto.MeetingExtractionStatusResponse {
	views := make([]dto.MeetingExtractionPhaseView, 0, len(phases))
	completed := 0
	var current, failed *string
	for _, phase := range phases {
		status := string(phase.Status)
		if phase.Status == domain.ExtractionPhaseSucceeded || phase.Status == domain.ExtractionPhaseSkipped {
			completed++
		}
		if phase.Status == domain.ExtractionPhaseRunning && current == nil {
			value := phase.PhaseKey
			current = &value
		}
		if phase.Status == domain.ExtractionPhaseFailed && failed == nil {
			value := phase.PhaseKey
			failed = &value
		}
		views = append(views, dto.MeetingExtractionPhaseView{
			Key: phase.PhaseKey, Label: meetingExtractionPhaseLabel(phase.PhaseKey), Status: status,
			Attempt: 0, ErrorCode: nil, Retryable: phase.Status == domain.ExtractionPhaseFailed,
		})
	}
	if current == nil {
		for _, phase := range phases {
			if phase.Status == domain.ExtractionPhasePending {
				value := phase.PhaseKey
				current = &value
				break
			}
		}
	}
	return &dto.MeetingExtractionStatusResponse{
		RunID: run.ID.String(), QuotationID: run.QuotationID.String(), Status: string(run.Status),
		CurrentPhase: current, Progress: dto.MeetingExtractionProgress{Completed: completed, Total: len(phases)},
		Phases: views, FailedPhase: failed, Error: nil,
		Budget: dto.MeetingExtractionBudgetView{}, NeedsInput: []dto.MeetingExtractionNotice{}, Warnings: []dto.MeetingExtractionNotice{},
	}
}

func meetingExtractionPhaseLabel(key string) string {
	switch key {
	case domain.ExtractionPhasePlanMetadata:
		return "Membaca konteks"
	case domain.ExtractionPhaseScope:
		return "Menyusun scope"
	case domain.ExtractionPhaseStaffing:
		return "Memetakan tim"
	case domain.ExtractionPhaseService:
		return "Mengisi detail layanan"
	case domain.ExtractionPhaseFinalize:
		return "Menyelesaikan quotation"
	default:
		return "Memproses"
	}
}

func meetingNotesFromGenerationInput(input json.RawMessage) (string, error) {
	var payload struct {
		MeetingNotes string `json:"meeting_notes"`
	}
	if !json.Valid(input) || json.Unmarshal(input, &payload) != nil || strings.TrimSpace(payload.MeetingNotes) == "" {
		return "", errors.New("meeting notes are unavailable")
	}
	return payload.MeetingNotes, nil
}
