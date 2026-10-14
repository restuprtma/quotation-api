package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

type promptCatalogRepoFake struct {
	card         *domain.RateCard
	items        []domain.RateCardItem
	err          error
	defaultCard  *domain.RateCard
	defaultItems []domain.RateCardItem
	defaultErr   error
	seen         struct {
		company, code string
		at            time.Time
	}
}

func (f *promptCatalogRepoFake) GetActiveRateCardByCode(_ context.Context, companyID, code string, at time.Time) (*domain.RateCard, []domain.RateCardItem, error) {
	f.seen.company, f.seen.code, f.seen.at = companyID, code, at
	return f.card, f.items, f.err
}

func (f *promptCatalogRepoFake) GetDefaultRateCard(_ context.Context, companyID string) (*domain.RateCard, []domain.RateCardItem, error) {
	f.seen.company = companyID
	if f.defaultErr != nil {
		return nil, nil, f.defaultErr
	}
	if f.defaultCard != nil {
		return f.defaultCard, f.defaultItems, nil
	}
	return nil, nil, repository.ErrRateCardNotFound
}

func TestPromptCatalogResolveByCodeUsesExactCodeAndPointInTime(t *testing.T) {
	at := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	repo := &promptCatalogRepoFake{card: &domain.RateCard{Code: DefaultPromptRateCardCode}, items: []domain.RateCardItem{{Role: "backend_programmer"}}}
	svc := NewPromptCatalogService(repo)

	card, items, err := svc.ResolveByCode(context.Background(), "company-a", DefaultPromptRateCardCode, at)
	require.NoError(t, err)
	assert.Equal(t, DefaultPromptRateCardCode, repo.seen.code)
	assert.Equal(t, "company-a", repo.seen.company)
	assert.Equal(t, at, repo.seen.at)
	assert.Equal(t, card, repo.card)
	assert.Len(t, items, 1)
}

func TestPromptCatalogResolveByCodePropagatesTenantOrInactiveLookupFailure(t *testing.T) {
	want := errors.New("rate card not found")
	svc := NewPromptCatalogService(&promptCatalogRepoFake{err: want})
	_, _, err := svc.ResolveByCode(context.Background(), "other-tenant", DefaultPromptRateCardCode, time.Now())
	assert.ErrorIs(t, err, want)
}

func TestPromptCatalogResolveByCodeRejectsCardWithoutItems(t *testing.T) {
	svc := NewPromptCatalogService(&promptCatalogRepoFake{card: &domain.RateCard{Code: DefaultPromptRateCardCode}})
	_, _, err := svc.ResolveByCode(context.Background(), "company-a", DefaultPromptRateCardCode, time.Now())
	assert.ErrorIs(t, err, ErrPromptCatalogEmpty)
}

func TestPromptCatalogRolesProjectsOnlySafeRoleMetadata(t *testing.T) {
	item := domain.RateCardItem{
		ID:             uuid.New(),
		Role:           "backend_programmer",
		DisplayName:    "Backend Programmer",
		Seniority:      "senior",
		TermType:       "long_term",
		AmountPerMonth: 25_000_000,
		BaseSalary:     15_000_000,
		KickoffFee:     1_000_000,
	}

	roles := PromptCatalogRoles([]domain.RateCardItem{item})
	require.Equal(t, []dto.PromptCatalogRole{{
		RateItemID:  item.ID,
		Role:        item.Role,
		DisplayName: item.DisplayName,
		Seniority:   item.Seniority,
		TermType:    item.TermType,
	}}, roles)

	payload, err := json.Marshal(roles)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "amount_per_month")
	assert.NotContains(t, string(payload), "base_salary")
	assert.NotContains(t, string(payload), "kickoff_fee")
}

func TestPromptCatalogRolesFromSnapshotAcceptsOnlyDefaultPromptCard(t *testing.T) {
	item := domain.RateCardItem{ID: uuid.New(), Role: "frontend_programmer", DisplayName: "Frontend Programmer", Seniority: "intermediate", TermType: "long_term", AmountPerMonth: 10_000_000}
	raw, err := json.Marshal(promptRateSnapshot{Card: domain.RateCard{ID: uuid.New(), Code: domain.DefaultPromptRateCardCode}, Items: []domain.RateCardItem{item}})
	require.NoError(t, err)

	roles := PromptCatalogRolesFromSnapshot(raw)
	require.Equal(t, []dto.PromptCatalogRole{{RateItemID: item.ID, Role: item.Role, DisplayName: item.DisplayName, Seniority: item.Seniority, TermType: item.TermType}}, roles)

	notDefault, err := json.Marshal(promptRateSnapshot{Card: domain.RateCard{ID: uuid.New(), Code: "OTHER"}, Items: []domain.RateCardItem{item}})
	require.NoError(t, err)
	assert.Empty(t, PromptCatalogRolesFromSnapshot(notDefault))
	assert.Empty(t, PromptCatalogRolesFromSnapshot(json.RawMessage(`not-json`)))

	customDefault, err := json.Marshal(promptRateSnapshot{Card: domain.RateCard{ID: uuid.New(), Code: "CUSTOM-CARD", IsDefault: true}, Items: []domain.RateCardItem{item}})
	require.NoError(t, err)
	customRoles := PromptCatalogRolesFromSnapshot(customDefault)
	require.Equal(t, []dto.PromptCatalogRole{{RateItemID: item.ID, Role: item.Role, DisplayName: item.DisplayName, Seniority: item.Seniority, TermType: item.TermType}}, customRoles)
}

func TestResolveRoleMatchesOnlyCatalogItems(t *testing.T) {
	items := []domain.RateCardItem{
		{ID: uuid.New(), Role: "frontend_programmer", DisplayName: "Front End Programmer", Seniority: "intermediate"},
		{ID: uuid.New(), Role: "backend_programmer", DisplayName: "Back End Programmer", Seniority: "senior"},
	}

	resolved := ResolveRole("FRONTEND_PROGRAMMER", "intermediate", items)
	assert.Equal(t, RoleResolved, resolved.Status)
	assert.Equal(t, items[0].ID, resolved.Item.ID)

	unresolved := ResolveRole("frontend_programmer", "senior", items)
	assert.Equal(t, RoleUnresolved, unresolved.Status)
	assert.Empty(t, unresolved.Candidates)
}

func TestResolveRoleDoesNotInventGenericAlias(t *testing.T) {
	items := []domain.RateCardItem{{Role: "backend_programmer", DisplayName: "Back End Programmer", Seniority: "senior"}}
	resolution := ResolveRole("programmer", "senior", items)
	assert.Equal(t, RoleUnresolved, resolution.Status)
	assert.Nil(t, resolution.Item)
}

func TestResolveRoleReportsAmbiguousCatalogMatches(t *testing.T) {
	items := []domain.RateCardItem{
		{Role: "backend_programmer", DisplayName: "Back End Programmer", Seniority: "senior", TermType: "long_term"},
		{Role: "backend_programmer", DisplayName: "Back End Programmer", Seniority: "senior", TermType: "short_term"},
	}
	resolution := ResolveRole("backend programmer", "senior", items)
	assert.Equal(t, RoleAmbiguous, resolution.Status)
	assert.Len(t, resolution.Candidates, 2)
	assert.Nil(t, resolution.Item)
}

func TestResolveRoleSupportsCommonAliases(t *testing.T) {
	items := []domain.RateCardItem{
		{ID: uuid.New(), Role: "frontend_programmer", DisplayName: "Front End Programmer", Seniority: "intermediate"},
		{ID: uuid.New(), Role: "backend_programmer", DisplayName: "Back End Programmer", Seniority: "senior"},
		{ID: uuid.New(), Role: "ui_ux_designer", DisplayName: "UI/UX Designer", Seniority: "intermediate"},
	}

	assert.Equal(t, RoleResolved, ResolveRole("Frontend Developer", "intermediate", items).Status)
	assert.Equal(t, RoleResolved, ResolveRole("backend developer", "senior", items).Status)
	assert.Equal(t, RoleResolved, ResolveRole("UI/UX", "intermediate", items).Status)
}

func TestDisambiguateRateCandidatesByTermAndSeniority(t *testing.T) {
	longTermID := uuid.New()
	shortTermID := uuid.New()
	items := []domain.RateCardItem{
		{ID: longTermID, Role: "backend_programmer", Seniority: "senior", TermType: "long_term"},
		{ID: shortTermID, Role: "backend_programmer", Seniority: "senior", TermType: "short_term"},
	}

	// 12 quarters (3 months) -> long_term
	resolved := DisambiguateRateCandidates(items, "senior", 12, nil)
	require.NotNil(t, resolved)
	assert.Equal(t, longTermID, resolved.ID)

	// 8 quarters (2 months) -> short_term
	resolvedShort := DisambiguateRateCandidates(items, "senior", 8, nil)
	require.NotNil(t, resolvedShort)
	assert.Equal(t, shortTermID, resolvedShort.ID)

	// Unspecified period defaults to long_term
	resolvedDefault := DisambiguateRateCandidates(items, "senior", 0, nil)
	require.NotNil(t, resolvedDefault)
	assert.Equal(t, longTermID, resolvedDefault.ID)
}

func TestResolveDefaultRateCard_WithCustomDefault(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	customCard := &domain.RateCard{
		ID:        uuid.New(),
		Code:      "CUSTOM-STD-2026",
		Name:      "Custom Default Rate Card",
		IsDefault: true,
		IsActive:  true,
	}
	customItems := []domain.RateCardItem{
		{ID: uuid.New(), Role: "backend_programmer", Seniority: "senior", AmountPerMonth: 20_000_000},
	}
	repo := &promptCatalogRepoFake{
		defaultCard:  customCard,
		defaultItems: customItems,
		card:         &domain.RateCard{Code: DefaultPromptRateCardCode},
		items:        []domain.RateCardItem{{Role: "fallback_programmer"}},
	}
	svc := NewPromptCatalogService(repo)

	card, items, err := svc.ResolveDefaultCard(context.Background(), "company-a", at)
	require.NoError(t, err)
	assert.Equal(t, "CUSTOM-STD-2026", card.Code)
	assert.True(t, card.IsDefault)
	assert.Equal(t, customCard.ID, card.ID)
	assert.Len(t, items, 1)
	assert.Equal(t, "backend_programmer", items[0].Role)
}

func TestResolveDefaultRateCard_FallbackWhenNoDefaultSet(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	fallbackCard := &domain.RateCard{
		ID:       uuid.New(),
		Code:     DefaultPromptRateCardCode,
		Name:     "Venturo Standard 2026",
		IsActive: true,
	}
	fallbackItems := []domain.RateCardItem{
		{ID: uuid.New(), Role: "frontend_programmer", Seniority: "intermediate", AmountPerMonth: 15_000_000},
	}
	repo := &promptCatalogRepoFake{
		defaultErr: repository.ErrRateCardNotFound,
		card:       fallbackCard,
		items:      fallbackItems,
	}
	svc := NewPromptCatalogService(repo)

	card, items, err := svc.ResolveDefaultCard(context.Background(), "company-a", at)
	require.NoError(t, err)
	assert.Equal(t, DefaultPromptRateCardCode, card.Code)
	assert.Equal(t, DefaultPromptRateCardCode, repo.seen.code)
	assert.Equal(t, at, repo.seen.at)
	assert.Len(t, items, 1)
	assert.Equal(t, "frontend_programmer", items[0].Role)
}

func TestResolveDefaultRateCard_FallbackWhenDefaultHasNoItems(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	emptyDefault := &domain.RateCard{
		ID:        uuid.New(),
		Code:      "EMPTY-DEFAULT",
		IsDefault: true,
		IsActive:  true,
	}
	fallbackCard := &domain.RateCard{
		ID:       uuid.New(),
		Code:     DefaultPromptRateCardCode,
		Name:     "Venturo Standard 2026",
		IsActive: true,
	}
	fallbackItems := []domain.RateCardItem{
		{ID: uuid.New(), Role: "qa_engineer", Seniority: "senior", AmountPerMonth: 14_000_000},
	}
	repo := &promptCatalogRepoFake{
		defaultCard:  emptyDefault,
		defaultItems: []domain.RateCardItem{}, // empty items on default card
		card:         fallbackCard,
		items:        fallbackItems,
	}
	svc := NewPromptCatalogService(repo)

	card, items, err := svc.ResolveDefaultCard(context.Background(), "company-a", at)
	require.NoError(t, err)
	assert.Equal(t, DefaultPromptRateCardCode, card.Code)
	assert.Len(t, items, 1)
}

func TestResolveDefaultRateCard_EmptyCatalogWhenFallbackAlsoHasNoItems(t *testing.T) {
	at := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	repo := &promptCatalogRepoFake{
		defaultErr: repository.ErrRateCardNotFound,
		card:       &domain.RateCard{Code: DefaultPromptRateCardCode},
		items:      []domain.RateCardItem{}, // empty fallback
	}
	svc := NewPromptCatalogService(repo)

	_, _, err := svc.ResolveDefaultCard(context.Background(), "company-a", at)
	assert.ErrorIs(t, err, ErrPromptCatalogEmpty)
}

func TestResolveDefaultRateCard_PropagatesDatabaseError(t *testing.T) {
	dbErr := errors.New("db connection failure")
	repo := &promptCatalogRepoFake{
		defaultErr: dbErr,
	}
	svc := NewPromptCatalogService(repo)

	_, _, err := svc.ResolveDefaultCard(context.Background(), "company-a", time.Now())
	assert.ErrorIs(t, err, dbErr)
}

