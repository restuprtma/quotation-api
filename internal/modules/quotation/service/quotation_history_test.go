package service_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/repository"
	"venturo-skeleton-go/internal/modules/quotation/service"
)

func TestHistory_MasksRawPrompt(t *testing.T) {
	companyID := uuid.New().String()
	quotationID := uuid.New().String()
	revID := uuid.New()
	userID := uuid.New()
	now := time.Now().UTC()
	safetyEnv := "env-v2.1"
	aiProv := "openai"
	aiMod := "gpt-4o"

	secretPrompt := "CRITICAL_SECRET_PROMPT_DO_NOT_LEAK"
	secretSystem := "TOP_SECRET_SYSTEM_INSTRUCTION"
	secretKey := "sk-sensitive-ai-key"
	secretSkill := "SECRET_INTERNAL_SKILL_PROMPT"
	secretMargin := "CONFIDENTIAL_MARGIN_SECRET"
	secretSalary := "999999999"

	generationInput := json.RawMessage(`{
		"raw_prompt": "` + secretPrompt + `",
		"system_prompt": "` + secretSystem + `",
		"api_key": "` + secretKey + `"
	}`)
	skillSnapshot := json.RawMessage(`{
		"system_instruction": "` + secretSkill + `",
		"model": "gpt-4o"
	}`)
	rateCardSnapshot := json.RawMessage(`{
		"code": "RC-V1",
		"currency": "IDR",
		"items": [{
			"role": "lead",
			"base_salary": ` + secretSalary + `,
			"margin_secret": "` + secretMargin + `"
		}]
	}`)
	legalTemplateSnapshot := json.RawMessage(`{
		"code": "LT-2026",
		"version": 2,
		"clauses": ["Confidential clause"]
	}`)
	promptState := json.RawMessage(`{
		"schema_version": "v1.2.0",
		"notes": "client meeting notes"
	}`)

	mockQRepo := &mockQuotationRepo{
		getQuotationFunc: func(ctx context.Context, id, compID string) (*domain.Quotation, error) {
			if id == quotationID && compID == companyID {
				return &domain.Quotation{ID: uuid.MustParse(id), CompanyID: uuid.MustParse(compID)}, nil
			}
			return nil, repository.ErrNotFound
		},
	}

	mockRRepo := &mockRevisionRepo{
		listRevisionsFunc: func(ctx context.Context, qID, compID string) ([]domain.QuotationRevision, error) {
			if qID == quotationID && compID == companyID {
				return []domain.QuotationRevision{
					{
						ID:                        revID,
						CompanyID:                 uuid.MustParse(compID),
						QuotationID:               uuid.MustParse(qID),
						RevisionNumber:            1,
						PublicationStatus:         "published",
						GenerationStatus:          "completed",
						GenerationInput:           generationInput,
						SkillSnapshot:             skillSnapshot,
						RateCardSnapshot:          rateCardSnapshot,
						LegalTemplateSnapshot:     legalTemplateSnapshot,
						PromptState:               promptState,
						PromptStateVersion:        3,
						SafetyEnvelopeVersion:     &safetyEnv,
						AiProvider:                &aiProv,
						AiModel:                   &aiMod,
						LegalContentModified:      true,
						CommercialContentModified: false,
						CreatedAt:                 now,
						GeneratedAt:               &now,
						PublishedAt:               &now,
						SignedAt:                  &now,
						CreatedBy:                 &userID,
					},
				}, nil
			}
			return nil, nil
		},
	}

	svc := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
		WithQuotationRepository(mockQRepo).
		WithRevisionRepository(mockRRepo)

	// 1. Verify GetQuotationHistory
	history, err := svc.GetQuotationHistory(context.Background(), companyID, quotationID)
	require.NoError(t, err)
	require.Len(t, history, 1)

	item := history[0]
	assert.Equal(t, revID, item.ID)
	assert.Equal(t, 1, item.RevisionNumber)
	assert.Equal(t, "published", item.PublicationStatus)
	assert.True(t, item.IsSigned)
	assert.True(t, item.LegalContentModified)
	assert.False(t, item.CommercialContentModified)
	require.NotNil(t, item.RateCardVersion)
	assert.Equal(t, "RC-V1", *item.RateCardVersion)
	require.NotNil(t, item.LegalTemplateVersion)
	assert.Equal(t, "2", *item.LegalTemplateVersion)
	require.NotNil(t, item.PromptVersion)
	assert.Equal(t, "v1.2.0", *item.PromptVersion)
	require.NotNil(t, item.SafetyEnvelopeVersion)
	assert.Equal(t, "env-v2.1", *item.SafetyEnvelopeVersion)

	// Verify serialized history contains zero leaked secrets
	historyJSON, err := json.Marshal(history)
	require.NoError(t, err)
	historyStr := string(historyJSON)

	assert.NotContains(t, historyStr, secretPrompt)
	assert.NotContains(t, historyStr, secretSystem)
	assert.NotContains(t, historyStr, secretKey)
	assert.NotContains(t, historyStr, secretSkill)
	assert.NotContains(t, historyStr, secretMargin)
	assert.NotContains(t, historyStr, secretSalary)
	assert.NotContains(t, historyStr, "raw_prompt")
	assert.NotContains(t, historyStr, "system_prompt")
	assert.NotContains(t, historyStr, "api_key")
	assert.NotContains(t, historyStr, "margin_secret")

	// 2. Verify ListRevisions masks raw prompts
	revs, err := svc.ListRevisions(context.Background(), companyID, quotationID)
	require.NoError(t, err)
	require.Len(t, revs, 1)
	assert.Nil(t, revs[0].AIRawResponse)

	revsJSON, err := json.Marshal(revs)
	require.NoError(t, err)
	revsStr := string(revsJSON)

	assert.NotContains(t, revsStr, secretPrompt)
	assert.NotContains(t, revsStr, secretSystem)
	assert.NotContains(t, revsStr, secretKey)
	assert.NotContains(t, revsStr, secretSkill)
	assert.NotContains(t, revsStr, secretMargin)
	assert.NotContains(t, revsStr, secretSalary)
}

func TestHistory_TenantIsolation(t *testing.T) {
	authorizedCompanyID := uuid.New().String()
	unauthorizedCompanyID := uuid.New().String()
	quotationID := uuid.New().String()

	mockQRepo := &mockQuotationRepo{
		getQuotationFunc: func(ctx context.Context, id, compID string) (*domain.Quotation, error) {
			if id == quotationID && compID == authorizedCompanyID {
				return &domain.Quotation{ID: uuid.MustParse(id), CompanyID: uuid.MustParse(compID)}, nil
			}
			return nil, repository.ErrNotFound
		},
	}

	mockRRepo := &mockRevisionRepo{
		listRevisionsFunc: func(ctx context.Context, qID, compID string) ([]domain.QuotationRevision, error) {
			if qID == quotationID && compID == authorizedCompanyID {
				return []domain.QuotationRevision{
					{
						ID:             uuid.New(),
						RevisionNumber: 1,
					},
				}, nil
			}
			return nil, nil
		},
	}

	svc := service.NewService(nil, nil, nil, nil, nil, nil, nil, nil).
		WithQuotationRepository(mockQRepo).
		WithRevisionRepository(mockRRepo)

	ctx := context.Background()

	// 1. Missing company ID returns ErrNoCompanyContext
	_, err := svc.GetQuotationHistory(ctx, "", quotationID)
	assert.ErrorIs(t, err, service.ErrNoCompanyContext)

	// 2. Unauthorized company ID returns ErrNotFound
	_, err = svc.GetQuotationHistory(ctx, unauthorizedCompanyID, quotationID)
	assert.ErrorIs(t, err, service.ErrNotFound)

	// 3. Non-existent quotation ID returns ErrNotFound
	nonExistentQuotationID := uuid.New().String()
	_, err = svc.GetQuotationHistory(ctx, authorizedCompanyID, nonExistentQuotationID)
	assert.ErrorIs(t, err, service.ErrNotFound)

	// 4. Invalid UUID format returns ErrNotFound
	_, err = svc.GetQuotationHistory(ctx, "invalid-company", quotationID)
	assert.ErrorIs(t, err, service.ErrNotFound)

	_, err = svc.GetQuotationHistory(ctx, authorizedCompanyID, "invalid-quotation")
	assert.ErrorIs(t, err, service.ErrNotFound)
}
