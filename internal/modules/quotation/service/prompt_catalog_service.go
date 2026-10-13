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
	normalizedRequested := normalizeRoleAlias(requestedKey)
	seniorityKey := catalogKey(seniority)
	if requestedKey == "programmer" || normalizedRequested == "programmer" {
		return resolution
	}
	for i := range items {
		item := items[i]
		itemRoleKey := catalogKey(item.Role)
		itemDisplayKey := catalogKey(item.DisplayName)
		if itemRoleKey == "programmer" || itemDisplayKey == "programmer" {
			continue
		}
		roleMatches := itemRoleKey == requestedKey ||
			itemDisplayKey == requestedKey ||
			normalizeRoleAlias(itemRoleKey) == normalizedRequested ||
			normalizeRoleAlias(itemDisplayKey) == normalizedRequested

		if !roleMatches {
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

func normalizeRoleAlias(key string) string {
	switch key {
	case "frontend developer", "front end developer", "fe developer", "fe programmer":
		return "frontend programmer"
	case "backend developer", "back end developer", "be developer", "be programmer":
		return "backend programmer"
	case "ui ux", "uiux", "ui ux designer", "ui designer", "ux designer", "ui/ux", "ui/ux designer":
		return "ui ux designer"
	case "fullstack programmer", "full stack programmer":
		return "fullstack developer"
	case "qa", "qa engineer", "tester", "quality assurance":
		return "functional tester"
	default:
		return key
	}
}

// DisambiguateRateCandidates resolves multiple candidates for a requested role
// by matching seniority and term type based on duration/period.
func DisambiguateRateCandidates(candidates []domain.RateCardItem, requestedSeniority string, periodQuarters int32, durationMonths *domain.Fact[int]) *domain.RateCardItem {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) == 1 {
		return &candidates[0]
	}

	// 1. Filter by seniority if specified or multiple seniorities exist in candidates
	targetSeniority := catalogKey(requestedSeniority)
	if targetSeniority == "" {
		targetSeniority = "intermediate"
	}
	var seniorityMatches []domain.RateCardItem
	for _, item := range candidates {
		if catalogKey(item.Seniority) == targetSeniority {
			seniorityMatches = append(seniorityMatches, item)
		}
	}
	pool := seniorityMatches
	if len(pool) == 0 {
		pool = candidates
	}
	if len(pool) == 1 {
		return &pool[0]
	}

	// 2. Disambiguate by term_type
	targetTerm := "long_term"
	if periodQuarters > 0 && periodQuarters < 12 {
		targetTerm = "short_term"
	} else if durationMonths != nil && durationMonths.Value > 0 && durationMonths.Value < 3 {
		targetTerm = "short_term"
	}

	var termMatches []domain.RateCardItem
	for _, item := range pool {
		if strings.EqualFold(item.TermType, targetTerm) {
			termMatches = append(termMatches, item)
		}
	}
	if len(termMatches) >= 1 {
		return &termMatches[0]
	}
	return &pool[0]
}

func catalogKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("_", " ", "-", " ", "/", " ").Replace(value)
	return strings.Join(strings.Fields(value), " ")
}
