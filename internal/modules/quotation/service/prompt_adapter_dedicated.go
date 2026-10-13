package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

// BuildDedicatedInput maps only catalog-backed, structural values into the
// existing calculator input. It never copies a price from prompt state.
func BuildDedicatedInput(state *domain.PromptQuotationState, catalog []domain.RateCardItem, anchor *time.Time) (domain.DedicatedSquadInput, []domain.DraftIssue) {
	issues := []domain.DraftIssue{}
	if state == nil {
		return domain.DedicatedSquadInput{}, []domain.DraftIssue{{Code: "state.missing", Field: "state", Message: "prompt state is required", Severity: domain.IssueBlocking}}
	}
	if err := state.Validate(); err != nil {
		issues = append(issues, domain.DraftIssue{Code: "state.invalid", Field: "state", Message: err.Error(), Severity: domain.IssueBlocking})
		return domain.DedicatedSquadInput{}, issues
	}
	issues = append(issues, state.ComputeIssues()...)
	if anchor == nil {
		issues = append(issues, domain.DraftIssue{Code: "payment_anchor.missing", Field: "payment_anchor_date", Message: "payment anchor date must be confirmed before calculation", Severity: domain.IssueBlocking})
	}

	input := domain.DedicatedSquadInput{
		Model:             domain.ModelSOWSmall,
		Requirement:       promptRequirement(state),
		PaymentAnchorDate: anchor,
		Timeline:          domain.TimelineInput{Mode: domain.TimelineRelative},
		QuotationProfile: &domain.QuotationProfileInput{
			PreparationMode:   domain.PreparationProfileNone,
			MaintenanceMode:   domain.MaintenanceProfileNone,
			ScopeTimelineMode: domain.ScopeTimelineRelative,
			PaymentMode:       domain.PaymentProfileRows,
		},
	}
	if state.DurationMonths != nil {
		input.PaymentCount = int32(state.DurationMonths.Value)
	}
	if state.PaymentPattern != nil {
		rows, err := paymentPatternRows(PaymentPattern(state.PaymentPattern.Value), state.DurationMonths)
		if err != nil {
			issues = append(issues, domain.DraftIssue{Code: "payment_pattern.unsupported", Field: "payment_pattern", Message: err.Error(), Severity: domain.IssueBlocking})
		} else {
			input.QuotationProfile.PaymentRows = rows
			input.PaymentCount = int32(len(rows))
		}
	}
	for i, line := range state.Team {
		item, ok := promptCatalogItem(line, catalog)
		if !ok {
			issues = append(issues, domain.DraftIssue{Code: "team.rate_unresolved", Field: fmt.Sprintf("team[%d].resolved_rate_item_id", i), Message: fmt.Sprintf("requested role %q has no matching catalog item", line.RequestedRole), Severity: domain.IssueBlocking})
			continue
		}
		input.DevelopmentSquad = append(input.DevelopmentSquad, domain.SquadLineInput{Role: item.Role, Seniority: item.Seniority, HeadcountQuarters: line.HeadcountQuarters, PeriodQuarters: line.PeriodQuarters})
	}
	if len(input.DevelopmentSquad) == 0 {
		issues = append(issues, domain.DraftIssue{Code: "team.empty_priceable", Field: "team", Message: "no resolved team line is available for calculation", Severity: domain.IssueBlocking})
	}
	return input, issues
}

// CalculatePromptDedicated is the calculator gate: unresolved state never
// reaches the calculator. The returned profile is the exact snapshot used for
// the calculation and can be persisted with the rate-card snapshot later.
func CalculatePromptDedicated(ctx context.Context, calculator *DedicatedSquadCalculator, companyID, rateCardID string, state *domain.PromptQuotationState, catalog []domain.RateCardItem, anchor *time.Time) (domain.DedicatedSquadInput, domain.DedicatedSquadCalculation, ResolvedQuotationProfile, []domain.DraftIssue, error) {
	input, issues := BuildDedicatedInput(state, catalog, anchor)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			return input, domain.DedicatedSquadCalculation{}, ResolvedQuotationProfile{}, issues, nil
		}
	}
	profile, err := ResolveDedicatedSquadProfile(input)
	if err != nil {
		return input, domain.DedicatedSquadCalculation{}, ResolvedQuotationProfile{}, issues, err
	}
	calc, err := calculator.CalculateWithProfile(ctx, companyID, rateCardID, input, profile)
	return input, calc, profile, issues, err
}

func promptRequirement(state *domain.PromptQuotationState) string {
	parts := make([]string, 0, len(state.Scope))
	for _, item := range state.Scope {
		if strings.TrimSpace(item.Value) != "" {
			parts = append(parts, "- "+strings.TrimSpace(item.Value))
		}
	}
	if len(parts) == 0 {
		return state.MeetingNotes
	}
	return strings.Join(parts, "\n")
}

func promptCatalogItem(line domain.PromptTeamLine, catalog []domain.RateCardItem) (domain.RateCardItem, bool) {
	if line.ResolvedRateItemID != nil {
		for _, item := range catalog {
			if item.ID.String() == strings.TrimSpace(*line.ResolvedRateItemID) {
				if catalogKey(item.Role) == "programmer" || catalogKey(item.DisplayName) == "programmer" {
					return domain.RateCardItem{}, false
				}
				return item, true
			}
		}
		return domain.RateCardItem{}, false
	}
	resolution := ResolveRole(line.RequestedRole, line.Seniority, catalog)
	if resolution.Status == RoleResolved {
		return valueOrEmpty(resolution.Item), true
	}
	if resolution.Status == RoleAmbiguous {
		if matched := DisambiguateRateCandidates(resolution.Candidates, line.Seniority, line.PeriodQuarters, nil); matched != nil {
			return *matched, true
		}
	}
	return domain.RateCardItem{}, false
}

func valueOrEmpty(item *domain.RateCardItem) domain.RateCardItem {
	if item == nil {
		return domain.RateCardItem{}
	}
	return *item
}
