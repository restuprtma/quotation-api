package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"venturo-skeleton-go/internal/middleware"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
	"venturo-skeleton-go/internal/shared/response"
)

// MaxBlueprintRequestSize limits request bodies to 1 MiB.
const MaxBlueprintRequestSize = 1 << 20

// BlueprintCreator encapsulates blueprint aggregate generation.
type BlueprintCreator interface {
	CreateBlueprint(ctx context.Context, companyID, createdBy uuid.UUID, req dto.CreateBlueprintGenerationRequest) (*repository.CreateBlueprintGenerationResult, error)
}

// BlueprintReader encapsulates read-only blueprint queries.
type BlueprintReader interface {
	GetStatus(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintStatusResponse, error)
	GetDetail(ctx context.Context, companyID, blueprintID uuid.UUID) (*dto.BlueprintDetailResponse, error)
	List(ctx context.Context, companyID uuid.UUID, params repository.BlueprintListParams) (*dto.BlueprintListResponse, error)
}

// BlueprintPipelineRunner encapsulates stage execution and manual edits for blueprints.
type BlueprintPipelineRunner interface {
	Continue(ctx context.Context, companyID, blueprintID uuid.UUID) (*service.BlueprintPipelineResult, error)
	RetrySection(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType) (*service.BlueprintPipelineResult, error)
	ManualCompleteSectionAtVersion(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error)
}

// BlueprintDraftSaver encapsulates draft saves for blueprint sections.
type BlueprintDraftSaver interface {
	SaveDraftSectionAtVersion(ctx context.Context, companyID, blueprintID uuid.UUID, sectionType domain.BlueprintSectionType, revisionID uuid.UUID, expectedVersion int, payload json.RawMessage) (*service.BlueprintPipelineResult, error)
}

// BlueprintGenerationHandler serves HTTP requests for the Blueprint pipeline.
type BlueprintGenerationHandler struct {
	creator    BlueprintCreator
	reader     BlueprintReader
	pipeline   BlueprintPipelineRunner
	draftSaver BlueprintDraftSaver
}

// NewBlueprintGenerationHandler constructs a new BlueprintGenerationHandler.
func NewBlueprintGenerationHandler(creator BlueprintCreator, reader BlueprintReader, pipeline BlueprintPipelineRunner, draftSaver BlueprintDraftSaver) *BlueprintGenerationHandler {
	return &BlueprintGenerationHandler{
		creator:    creator,
		reader:     reader,
		pipeline:   pipeline,
		draftSaver: draftSaver,
	}
}

// Generate initiates blueprint generation or replays existing draft idempotently.
func (h *BlueprintGenerationHandler) Generate(c *gin.Context) {
	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}
	actorID, ok := getActorID(c)
	if !ok {
		return
	}

	var req dto.CreateBlueprintGenerationRequest
	if !decodeStrictJSON(c, &req) {
		return
	}

	if err := binding.Validator.ValidateStruct(&req); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "blueprint_invalid_input")
		return
	}

	if req.IdempotencyKey == uuid.Nil || req.QuotationID == uuid.Nil || req.SourceQuotationRevisionID == uuid.Nil ||
		strings.TrimSpace(req.Title) == "" || utf8.RuneCountInString(req.Title) > 160 ||
		len(req.BusinessGoals) == 0 || len(req.Actors) == 0 || len(req.BusinessPhases) == 0 {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "blueprint_invalid_input")
		return
	}

	res, err := h.creator.CreateBlueprint(c.Request.Context(), companyID, actorID, req)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	status, err := h.reader.GetStatus(c.Request.Context(), companyID, res.Blueprint.ID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	if res.IdempotentReplay {
		response.Success(c, http.StatusOK, "Blueprint generation retrieved successfully", status)
	} else {
		response.Success(c, http.StatusCreated, "Blueprint generation started successfully", status)
	}
}

// GetStatus returns the current generation and section statuses for a blueprint.
func (h *BlueprintGenerationHandler) GetStatus(c *gin.Context) {
	bpID, err := uuid.Parse(c.Param("id"))
	if err != nil || bpID == uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint id", "blueprint_invalid_input")
		return
	}

	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	status, err := h.reader.GetStatus(c.Request.Context(), companyID, bpID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint status retrieved successfully", status)
}

// GetDetail returns full revision detail for a blueprint.
func (h *BlueprintGenerationHandler) GetDetail(c *gin.Context) {
	bpID, err := uuid.Parse(c.Param("id"))
	if err != nil || bpID == uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint id", "blueprint_invalid_input")
		return
	}

	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	detail, err := h.reader.GetDetail(c.Request.Context(), companyID, bpID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint detail retrieved successfully", detail)
}

// Continue advances execution of the next pending stage in the blueprint pipeline.
func (h *BlueprintGenerationHandler) Continue(c *gin.Context) {
	bpID, err := uuid.Parse(c.Param("id"))
	if err != nil || bpID == uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint id", "blueprint_invalid_input")
		return
	}

	if !checkStrictEmptyBody(c) {
		return
	}

	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	_, err = h.pipeline.Continue(c.Request.Context(), companyID, bpID)
	if err != nil && !errors.Is(err, service.ErrBlueprintPipelineCompleted) {
		MapBlueprintError(c, err)
		return
	}

	status, err := h.reader.GetStatus(c.Request.Context(), companyID, bpID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint pipeline continued successfully", status)
}

// RetrySection retries a previously failed provider section stage.
func (h *BlueprintGenerationHandler) RetrySection(c *gin.Context) {
	bpID, err := uuid.Parse(c.Param("id"))
	if err != nil || bpID == uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint id", "blueprint_invalid_input")
		return
	}

	section, ok := parseProviderSection(c.Param("section"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "Invalid section parameter", "blueprint_invalid_input")
		return
	}

	if !checkStrictEmptyBody(c) {
		return
	}

	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	_, err = h.pipeline.RetrySection(c.Request.Context(), companyID, bpID, section)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	status, err := h.reader.GetStatus(c.Request.Context(), companyID, bpID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint section retried successfully", status)
}

// ManualCompleteSection directly applies manual section JSON edits with optimistic concurrency checks.
func (h *BlueprintGenerationHandler) ManualCompleteSection(c *gin.Context) {
	bpID, err := uuid.Parse(c.Param("id"))
	if err != nil || bpID == uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint id", "blueprint_invalid_input")
		return
	}

	section, ok := parseProviderSection(c.Param("section"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "Invalid section parameter", "blueprint_invalid_input")
		return
	}

	var req dto.ManualCompleteBlueprintSectionRequest
	if !decodeStrictJSON(c, &req) {
		return
	}

	if req.RevisionID == uuid.Nil || req.ContentVersion < 1 {
		response.Error(c, http.StatusBadRequest, "Invalid manual completion parameters", "blueprint_invalid_input")
		return
	}

	trimmedContent := bytes.TrimSpace(req.Content)
	if len(trimmedContent) == 0 || string(trimmedContent) == "null" || !json.Valid(trimmedContent) {
		response.Error(c, http.StatusBadRequest, "Invalid manual content payload", "blueprint_invalid_input")
		return
	}

	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	_, err = h.pipeline.ManualCompleteSectionAtVersion(
		c.Request.Context(),
		companyID,
		bpID,
		section,
		req.RevisionID,
		req.ContentVersion,
		req.Content,
	)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	status, err := h.reader.GetStatus(c.Request.Context(), companyID, bpID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint section manually completed successfully", status)
}

// SaveDraftSection saves section draft JSON edits with optimistic concurrency checks and returns canonical detail response.
func (h *BlueprintGenerationHandler) SaveDraftSection(c *gin.Context) {
	bpID, err := uuid.Parse(c.Param("id"))
	if err != nil || bpID == uuid.Nil {
		response.Error(c, http.StatusBadRequest, "Invalid blueprint id", "blueprint_invalid_input")
		return
	}

	section, ok := parseProviderSection(c.Param("section"))
	if !ok {
		response.Error(c, http.StatusBadRequest, "Invalid section parameter", "blueprint_invalid_input")
		return
	}

	var req dto.SaveBlueprintDraftSectionRequest
	if !decodeStrictJSON(c, &req) {
		return
	}

	if req.RevisionID == uuid.Nil || req.ContentVersion < 1 {
		response.Error(c, http.StatusBadRequest, "Invalid draft parameters", "blueprint_invalid_input")
		return
	}

	trimmedContent := bytes.TrimSpace(req.Content)
	if len(trimmedContent) == 0 || string(trimmedContent) == "null" || !json.Valid(trimmedContent) {
		response.Error(c, http.StatusBadRequest, "Invalid draft content payload", "blueprint_invalid_input")
		return
	}

	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	if h.draftSaver == nil {
		response.Error(c, http.StatusInternalServerError, "Draft saver not initialized", "persistence_failure")
		return
	}

	_, err = h.draftSaver.SaveDraftSectionAtVersion(
		c.Request.Context(),
		companyID,
		bpID,
		section,
		req.RevisionID,
		req.ContentVersion,
		req.Content,
	)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	if h.reader == nil {
		response.Error(c, http.StatusInternalServerError, "Reader not initialized", "persistence_failure")
		return
	}

	detail, err := h.reader.GetDetail(c.Request.Context(), companyID, bpID)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprint draft saved successfully", detail)
}

// List returns tenant-filtered and paginated blueprint items.
func (h *BlueprintGenerationHandler) List(c *gin.Context) {
	companyID, ok := getCompanyID(c)
	if !ok {
		return
	}

	page := 1
	rawPage := strings.TrimSpace(c.Query("page"))
	if rawPage != "" {
		p, err := strconv.Atoi(rawPage)
		if err != nil || p < 1 {
			response.Error(c, http.StatusBadRequest, "Invalid page query parameter", "blueprint_invalid_input")
			return
		}
		page = p
	}

	limit := 20
	rawLimit := strings.TrimSpace(c.Query("limit"))
	if rawLimit != "" {
		l, err := strconv.Atoi(rawLimit)
		if err != nil || l < 1 || l > 100 {
			response.Error(c, http.StatusBadRequest, "Invalid limit query parameter", "blueprint_invalid_input")
			return
		}
		limit = l
	}

	var qFilter *uuid.UUID
	rawQID := strings.TrimSpace(c.Query("quotation_id"))
	if rawQID != "" {
		qid, err := uuid.Parse(rawQID)
		if err != nil || qid == uuid.Nil {
			response.Error(c, http.StatusBadRequest, "Invalid quotation id query parameter", "blueprint_invalid_input")
			return
		}
		qFilter = &qid
	}

	var statuses []string
	rawStatus := strings.TrimSpace(c.Query("generation_status"))
	if rawStatus != "" {
		seen := make(map[string]bool)
		for _, part := range strings.Split(rawStatus, ",") {
			status := strings.TrimSpace(part)
			if !isValidGenerationStatus(status) {
				response.Error(c, http.StatusBadRequest, "Invalid generation_status query parameter", "blueprint_invalid_input")
				return
			}
			if !seen[status] {
				seen[status] = true
				statuses = append(statuses, status)
			}
		}
	}

	params := repository.BlueprintListParams{
		QuotationID:        qFilter,
		GenerationStatuses: statuses,
		Page:               page,
		Limit:              limit,
	}

	list, err := h.reader.List(c.Request.Context(), companyID, params)
	if err != nil {
		MapBlueprintError(c, err)
		return
	}

	response.Success(c, http.StatusOK, "Blueprints retrieved successfully", list)
}

// --- Internal Utilities ---

func isValidGenerationStatus(status string) bool {
	switch domain.BlueprintGenerationStatus(status) {
	case domain.BlueprintGenerationPending,
		domain.BlueprintGenerationInProgress,
		domain.BlueprintGenerationPartialFailed,
		domain.BlueprintGenerationCompleted,
		domain.BlueprintGenerationFailed:
		return true
	default:
		return false
	}
}

func getCompanyID(c *gin.Context) (uuid.UUID, bool) {
	raw := middleware.GetCompanyID(c)
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		response.Error(c, http.StatusForbidden, "Company context is required", "")
		return uuid.Nil, false
	}
	return id, true
}

func getActorID(c *gin.Context) (uuid.UUID, bool) {
	raw, err := middleware.GetUserID(c)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized", "User context is required")
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		response.Error(c, http.StatusUnauthorized, "Unauthorized", "Invalid user id")
		return uuid.Nil, false
	}
	return id, true
}

func parseProviderSection(param string) (domain.BlueprintSectionType, bool) {
	switch domain.BlueprintSectionType(param) {
	case domain.BlueprintSectionJourney,
		domain.BlueprintSectionArchitecture,
		domain.BlueprintSectionStateMachine,
		domain.BlueprintSectionWorkflow,
		domain.BlueprintSectionWBS:
		return domain.BlueprintSectionType(param), true
	default:
		return "", false
	}
}

func decodeStrictJSON(c *gin.Context, target any) bool {
	if c.Request.Body == nil {
		response.Error(c, http.StatusBadRequest, "Request body is required", "blueprint_invalid_input")
		return false
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, MaxBlueprintRequestSize)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "blueprint_invalid_input")
		return false
	}

	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		response.Error(c, http.StatusBadRequest, "Multiple JSON tokens are not allowed", "blueprint_invalid_input")
		return false
	}

	return true
}

func checkStrictEmptyBody(c *gin.Context) bool {
	if c.Request.Body == nil {
		return true
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	buf, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "Invalid request payload", "blueprint_invalid_input")
		return false
	}

	trimmed := bytes.TrimSpace(buf)
	if len(trimmed) > 0 {
		response.Error(c, http.StatusBadRequest, "Non-empty body is not allowed", "blueprint_invalid_input")
		return false
	}

	return true
}
