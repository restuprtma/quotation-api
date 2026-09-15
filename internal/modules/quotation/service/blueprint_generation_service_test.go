package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type fakeBlueprintContextBuilder struct {
	result      NormalizedBlueprintContext
	err         error
	buildCalls  int
	receivedSrc BlueprintSourceProjection
	receivedReq dto.CreateBlueprintGenerationRequest
}

func (f *fakeBlueprintContextBuilder) Build(source BlueprintSourceProjection, request dto.CreateBlueprintGenerationRequest) (NormalizedBlueprintContext, error) {
	f.buildCalls++
	f.receivedSrc = source
	f.receivedReq = request
	return f.result, f.err
}

type fakeBlueprintGenerationCreator struct {
	result      *repository.CreateBlueprintGenerationResult
	err         error
	createCalls int
	received    repository.CreateBlueprintGenerationParams
}

func (f *fakeBlueprintGenerationCreator) CreateBlueprintGeneration(_ context.Context, params repository.CreateBlueprintGenerationParams) (*repository.CreateBlueprintGenerationResult, error) {
	f.createCalls++
	f.received = params
	return f.result, f.err
}

func validBlueprintGenerationServiceInput() (uuid.UUID, uuid.UUID, BlueprintSourceProjection, dto.CreateBlueprintGenerationRequest) {
	companyID := uuid.New()
	createdBy := uuid.New()
	source := BlueprintSourceProjection{
		CompanyID:    companyID,
		QuotationID:  uuid.New(),
		RevisionID:   uuid.New(),
		Subject:      "Customer portal",
		SolutionType: "web",
		ContentHTML:  "<p>safe source content</p>",
	}
	request := dto.CreateBlueprintGenerationRequest{
		IdempotencyKey:            uuid.New(),
		QuotationID:               source.QuotationID,
		SourceQuotationRevisionID: source.RevisionID,
		Title:                     "Customer portal blueprint",
	}
	return companyID, createdBy, source, request
}

func TestBlueprintGenerationServiceCreateCallsRepositoryOnce(t *testing.T) {
	companyID, createdBy, source, request := validBlueprintGenerationServiceInput()
	normalized := NormalizedBlueprintContext{
		SourceContextSnapshot:  []byte(`{"subject":"Customer portal"}`),
		ContextContent:         []byte(`{"title":"Customer portal blueprint"}`),
		IdempotencyPayloadHash: strings.Repeat("a", 64),
	}
	builder := &fakeBlueprintContextBuilder{result: normalized}
	creator := &fakeBlueprintGenerationCreator{result: &repository.CreateBlueprintGenerationResult{}}
	service := NewBlueprintGenerationService(builder, creator)

	result, err := service.CreateBlueprint(context.Background(), companyID, createdBy, source, request)

	if err != nil {
		t.Fatalf("CreateBlueprint() error = %v", err)
	}
	if result != creator.result {
		t.Fatal("CreateBlueprint() result did not come from creator")
	}
	if builder.buildCalls != 1 {
		t.Fatalf("builder calls = %d, want 1", builder.buildCalls)
	}
	if builder.receivedSrc.CompanyID != companyID || builder.receivedSrc.QuotationID != source.QuotationID || builder.receivedSrc.RevisionID != source.RevisionID {
		t.Fatalf("builder source IDs = %+v, want authenticated source IDs", builder.receivedSrc)
	}
	if !reflect.DeepEqual(builder.receivedReq, request) {
		t.Fatal("builder did not receive the request unchanged")
	}
	if creator.createCalls != 1 {
		t.Fatalf("creator calls = %d, want 1", creator.createCalls)
	}
	if creator.received.CompanyID != companyID || creator.received.QuotationID != request.QuotationID || creator.received.SourceRevisionID != request.SourceQuotationRevisionID {
		t.Fatalf("creator identity params = %+v, want request/authenticated IDs", creator.received)
	}
	if creator.received.Title != request.Title || creator.received.IdempotencyKey != request.IdempotencyKey {
		t.Fatalf("creator request params = %+v, want title and idempotency key", creator.received)
	}
	if string(creator.received.SourceContextSnapshot) != string(normalized.SourceContextSnapshot) || string(creator.received.ContextContent) != string(normalized.ContextContent) || creator.received.IdempotencyPayloadHash != normalized.IdempotencyPayloadHash {
		t.Fatalf("creator normalized params = %+v, want normalized context", creator.received)
	}
	if creator.received.CreatedBy == nil || *creator.received.CreatedBy != createdBy {
		t.Fatalf("creator CreatedBy = %v, want %s", creator.received.CreatedBy, createdBy)
	}
	if string(creator.received.SkillSnapshot) != "{}" {
		t.Fatalf("creator SkillSnapshot = %s, want {}", creator.received.SkillSnapshot)
	}
}

func TestBlueprintGenerationServiceCreatePropagatesPayloadMismatch(t *testing.T) {
	companyID, createdBy, source, request := validBlueprintGenerationServiceInput()
	builder := &fakeBlueprintContextBuilder{result: NormalizedBlueprintContext{}}
	creator := &fakeBlueprintGenerationCreator{err: repository.ErrBlueprintIdempotencyPayloadMismatch}
	service := NewBlueprintGenerationService(builder, creator)

	_, err := service.CreateBlueprint(context.Background(), companyID, createdBy, source, request)

	if !errors.Is(err, repository.ErrBlueprintIdempotencyPayloadMismatch) {
		t.Fatalf("CreateBlueprint() error = %v, want idempotency mismatch", err)
	}
	if creator.createCalls != 1 {
		t.Fatalf("creator calls = %d, want 1", creator.createCalls)
	}
}

func TestBlueprintGenerationServiceCreateRejectsTenantMismatch(t *testing.T) {
	companyID, createdBy, source, request := validBlueprintGenerationServiceInput()
	source.CompanyID = uuid.New()
	builder := &fakeBlueprintContextBuilder{}
	creator := &fakeBlueprintGenerationCreator{}
	service := NewBlueprintGenerationService(builder, creator)

	_, err := service.CreateBlueprint(context.Background(), companyID, createdBy, source, request)

	if !errors.Is(err, ErrBlueprintGenerationTenantMismatch) {
		t.Fatalf("CreateBlueprint() error = %v, want tenant mismatch", err)
	}
	if builder.buildCalls != 0 || creator.createCalls != 0 {
		t.Fatalf("calls builder=%d creator=%d, want both zero", builder.buildCalls, creator.createCalls)
	}
}

func TestBlueprintGenerationServiceCreateDoesNotExposeSourceContentInError(t *testing.T) {
	companyID, createdBy, source, request := validBlueprintGenerationServiceInput()
	const secretSourceContent = "private-source-content-must-not-leak"
	source.ContentHTML = secretSourceContent
	builder := &fakeBlueprintContextBuilder{err: errors.New(secretSourceContent)}
	creator := &fakeBlueprintGenerationCreator{}
	service := NewBlueprintGenerationService(builder, creator)

	_, err := service.CreateBlueprint(context.Background(), companyID, createdBy, source, request)

	if err == nil {
		t.Fatal("CreateBlueprint() error = nil, want safe builder failure")
	}
	if strings.Contains(err.Error(), secretSourceContent) {
		t.Fatalf("CreateBlueprint() error leaked source content: %v", err)
	}
	if !errors.Is(err, ErrBlueprintGenerationInvalidInput) {
		t.Fatalf("CreateBlueprint() error = %v, want invalid input", err)
	}
	if creator.createCalls != 0 {
		t.Fatalf("creator calls = %d, want 0", creator.createCalls)
	}
}
