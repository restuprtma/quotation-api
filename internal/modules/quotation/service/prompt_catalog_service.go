package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
)

const DefaultPromptRateCardCode = "VENTURO-STD-2026"

var ErrPromptCatalogEmpty = errors.New("prompt catalog has no rate items")

// PromptCatalogRoles projects rate-card items into the safe role metadata the
// Prompt-first editor needs. Pricing and internal-cost fields stay server-only.
func PromptCatalogRoles(items []domain.RateCardItem) []dto.PromptCatalogRole {
	roles := make([]dto.PromptCatalogRole, len(items))
	for i, item := range items {
		roles[i] = dto.PromptCatalogRole{
			RateItemID:  item.ID,
			Role:        item.Role,
			DisplayName: item.DisplayName,
			Seniority:   item.Seniority,
			TermType:    item.TermType,
		}
	}
	return roles
}

// PromptCatalogRolesFromSnapshot returns only the editor-safe role metadata
// from a persisted Prompt-first default-card snapshot. Missing, malformed, or
// non-default snapshots intentionally produce no roles.
func PromptCatalogRolesFromSnapshot(raw json.RawMessage) []dto.PromptCatalogRole {
	_, items, err := promptRateCardSnapshot(raw)
	if err != nil {
		return nil
	}
	return PromptCatalogRoles(items)
}

type PromptCatalogRepository interface {
	GetActiveRateCardByCode(context.Context, string, string, time.Time) (*domain.RateCard, []domain.RateCardItem, error)
}

type PromptCatalogService struct {
	repo PromptCatalogRepository
}

func NewPromptCatalogService(repo PromptCatalogRepository) *PromptCatalogService {
	return &PromptCatalogService{repo: repo}
}

func (s *PromptCatalogService) ResolveByCode(ctx context.Context, companyID, code string, at time.Time) (*domain.RateCard, []domain.RateCardItem, error) {
	card, items, err := s.repo.GetActiveRateCardByCode(ctx, companyID, code, at)
	if err != nil {
		return nil, nil, err
	}
	if card == nil || len(items) == 0 {
		return nil, nil, ErrPromptCatalogEmpty
	}
	return card, items, nil
}

type RoleResolutionStatus string

const (
	RoleResolved   RoleResolutionStatus = "resolved"
	RoleUnresolved RoleResolutionStatus = "unresolved"
	RoleAmbiguous  RoleResolutionStatus = "ambiguous"
)

type RoleResolution struct {
	Status     RoleResolutionStatus
	Requested  string
	Seniority  string
	Item       *domain.RateCardItem
	Candidates []domain.RateCardItem
}

// ResolveRole only matches role identifiers or catalog display names that are
// present in the supplied card. It intentionally does not invent aliases or
// map a generic request such as "programmer" to a specific role.
func ResolveRole(requested, seniority string, items []domain.RateCardItem) RoleResolution {
	resolution := RoleResolution{
		Status:    RoleUnresolved,
		Requested: requested,
		Seniority: seniority,
	}
	requestedKey := catalogKey(requested)
	seniorityKey := catalogKey(seniority)
	if requestedKey == "programmer" {
		return resolution
	}
	for i := range items {
		item := items[i]
		if catalogKey(item.Role) == "programmer" || catalogKey(item.DisplayName) == "programmer" {
			continue
		}
		if catalogKey(item.Role) != requestedKey && catalogKey(item.DisplayName) != requestedKey {
			continue
		}
		if seniorityKey != "" && catalogKey(item.Seniority) != seniorityKey {
			continue
		}
		resolution.Candidates = append(resolution.Candidates, item)
	}
	if len(resolution.Candidates) == 1 {
		resolution.Status = RoleResolved
		resolution.Item = &resolution.Candidates[0]
	} else if len(resolution.Candidates) > 1 {
		resolution.Status = RoleAmbiguous
	}
	return resolution
}

func catalogKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("_", " ", "-", " ").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
