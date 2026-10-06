package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

type MockPublicBlueprintRecordLoader struct {
	mock.Mock
}

func (m *MockPublicBlueprintRecordLoader) LoadSharedRevisionData(
	ctx context.Context, blueprintID, revisionID uuid.UUID,
) (*domain.Blueprint, *domain.BlueprintRevision, []domain.BlueprintSectionRun, error) {
	args := m.Called(ctx, blueprintID, revisionID)
	if args.Get(0) == nil {
		return nil, nil, nil, args.Error(3)
	}
	return args.Get(0).(*domain.Blueprint), args.Get(1).(*domain.BlueprintRevision), args.Get(2).([]domain.BlueprintSectionRun), args.Error(3)
}

func TestPublicBlueprintService_StrictRedaction(t *testing.T) {
	shareRepo := new(MockShareLinkRepo)
	loader := new(MockPublicBlueprintRecordLoader)
	svc := service.NewPublicBlueprintService(shareRepo, loader)

	rawToken := "valid-programmer-token-43-chars-url-safe-abc"
	tokenHash := service.HashShareToken(rawToken)

	blueprintID := uuid.New()
	revisionID := uuid.New()
	companyID := uuid.New()
	now := time.Now().UTC()

	shareRepo.On("GetActiveByTokenHash", mock.Anything, tokenHash).Return(&domain.ShareLink{
		ID:                  uuid.New(),
		CompanyID:           companyID,
		BlueprintID:         blueprintID,
		BlueprintRevisionID: revisionID,
		TokenHash:           tokenHash,
		Label:               "Programmer Link",
		CreatedAt:           now,
	}, nil)

	shareRepo.On("TouchLastAccessed", mock.Anything, mock.Anything).Return(nil)

	bundleJSON := `{
		"schema_version": "blueprint-bundle-v1",
		"title": "Hospital Queue Architecture",
		"summary": "Core system specifications for hospital ticketing.",
		"journey": {"journeys": [{"name": "Patient Registration"}]},
		"architecture": {"components": [{"name": "QueueEngine"}]},
		"state_machine": {"states": ["WAITING", "CALLED"]},
		"workflows": [{"name": "Patient Call Flow"}],
		"wbs": {"modules": [{"name": "Core Module"}]},
		"assumptions": ["Stable LAN connectivity"],
		"open_questions": [{"question": "Database replication strategy"}]
	}`

	bp := &domain.Blueprint{
		ID:          blueprintID,
		CompanyID:   companyID,
		QuotationID: uuid.New(),
		Title:       "Hospital Queue Architecture",
	}

	supersededStatus := "superseded"
	rev := &domain.BlueprintRevision{
		ID:                    revisionID,
		CompanyID:             companyID,
		BlueprintID:           blueprintID,
		RevisionNumber:        2,
		PublicationStatus:     supersededStatus, // Internal status is superseded, but programmer must see partial or complete!
		CompletenessStatus:    "partial",
		ContentJSON:           json.RawMessage(bundleJSON),
		PublishedAt:           &now,
		SourceContextSnapshot: json.RawMessage(`{"sensitive_quotation_budget": 500000000, "client_name": "RS Harapan"}`),
	}

	runs := []domain.BlueprintSectionRun{
		{SectionType: "journey", Status: "validated"},
		{SectionType: "architecture", Status: "validated"},
		{SectionType: "state_machine", Status: "pending"},
		{SectionType: "workflow", Status: "validated"},
		{SectionType: "wbs", Status: "validated"},
	}

	loader.On("LoadSharedRevisionData", mock.Anything, blueprintID, revisionID).Return(bp, rev, runs, nil)

	res, err := svc.GetPublicBlueprint(context.Background(), rawToken)
	require.NoError(t, err)
	assert.NotNil(t, res)

	// 1. Allowlist values asserted
	assert.Equal(t, "Hospital Queue Architecture", res.Title)
	assert.Equal(t, 2, res.RevisionNumber)
	assert.Equal(t, "partial", res.CompletenessStatus)
	assert.NotEqual(t, "superseded", res.CompletenessStatus, "must not leak superseded status")
	assert.Equal(t, now, res.PublishedAt)
	assert.Equal(t, "Core system specifications for hospital ticketing.", res.Summary)
	assert.Contains(t, res.Assumptions, "Stable LAN connectivity")

	// 2. Strict serialization test against data leakage
	serialized, err := json.Marshal(res)
	require.NoError(t, err)
	jsonStr := string(serialized)

	forbiddenSubstrings := []string{
		companyID.String(),
		bp.QuotationID.String(),
		"500000000",
		"RS Harapan",
		"superseded",
		"token_hash",
		"source_context_snapshot",
		"ai_provider",
		"ai_model",
	}

	for _, forbidden := range forbiddenSubstrings {
		assert.NotContains(t, jsonStr, forbidden, "serialized ProgrammerBlueprintDTO must not contain forbidden substring: %s", forbidden)
	}
}

func TestPublicBlueprintService_NotFoundOrRevoked(t *testing.T) {
	shareRepo := new(MockShareLinkRepo)
	loader := new(MockPublicBlueprintRecordLoader)
	svc := service.NewPublicBlueprintService(shareRepo, loader)

	// Unknown token returns ErrPublicBlueprintNotFound
	rawToken := "non-existent-token"
	shareRepo.On("GetActiveByTokenHash", mock.Anything, service.HashShareToken(rawToken)).Return(nil, service.ErrPublicBlueprintNotFound)

	res, err := svc.GetPublicBlueprint(context.Background(), rawToken)
	assert.Nil(t, res)
	assert.ErrorIs(t, err, service.ErrPublicBlueprintNotFound)
}
