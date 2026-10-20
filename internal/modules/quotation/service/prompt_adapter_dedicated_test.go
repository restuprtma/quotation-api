package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestBuildDedicatedInputUsesCatalogAndQuarterUnits(t *testing.T) {
	id := uuid.New()
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "Arunika",
		ClientCompany:    &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		ClientName:       &domain.Fact[string]{Value: "Nadia", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme: &domain.Fact[string]{Value: "Dedicated", Source: domain.SourceUser, Confirmed: true},
		DurationMonths:   &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true},
		Team:             []domain.PromptTeamLine{{RequestedRole: "programmer", Seniority: "intermediate", ResolvedRateItemID: ptr(id.String()), HeadcountQuarters: 8, PeriodQuarters: 12, Source: domain.SourceUser, Confirmed: true}},
		PaymentPattern:   &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
	}
	anchor := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	input, issues := BuildDedicatedInput(state, []domain.RateCardItem{{ID: id, Role: "backend_programmer", Seniority: "intermediate", AmountPerMonth: 12_000_000}}, &anchor)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			t.Fatalf("unexpected blocking issue: %+v", issue)
		}
	}
	if len(input.DevelopmentSquad) != 1 || input.DevelopmentSquad[0].HeadcountQuarters != 8 || input.DevelopmentSquad[0].PeriodQuarters != 12 {
		t.Fatalf("input did not preserve quarter units: %+v", input.DevelopmentSquad)
	}
	if input.QuotationProfile == nil || len(input.QuotationProfile.PaymentRows) != 2 {
		t.Fatalf("payment profile not mapped: %+v", input.QuotationProfile)
	}
	if input.KickoffDiscountBPS != 0 {
		t.Fatalf("kickoff discount must remain zero")
	}
}

func TestCalculatePromptDedicatedArunikaUsesCalculatorTotal(t *testing.T) {
	id := uuid.New()
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "Arunika Inventory",
		ClientCompany:    &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		ClientName:       &domain.Fact[string]{Value: "Nadia", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme: &domain.Fact[string]{Value: "Dedicated", Source: domain.SourceUser, Confirmed: true},
		DurationMonths:   &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true},
		Team:             []domain.PromptTeamLine{{RequestedRole: "programmer", Seniority: "intermediate", ResolvedRateItemID: ptr(id.String()), HeadcountQuarters: 8, PeriodQuarters: 12, Source: domain.SourceUser, Confirmed: true}},
		PaymentPattern:   &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
	}
	anchor := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	item := domain.RateCardItem{ID: id, Role: "backend_programmer", Seniority: "intermediate", AmountPerMonth: 12_000_000}
	_, calc, _, issues, err := CalculatePromptDedicated(context.Background(), NewDedicatedSquadCalculator(promptRateLookup{item: item}), "company", "card", state, []domain.RateCardItem{item}, &anchor)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			t.Fatalf("unexpected blocking issue: %+v", issue)
		}
	}
	if calc.DevelopmentSubtotal != 72_000_000 || calc.TotalInvestment != 72_000_000 {
		t.Fatalf("calculator total = %+v, want 72000000", calc)
	}
	if len(calc.Payments) != 2 || calc.Payments[0].Nominal != 36_000_000 || calc.Payments[1].Nominal != 36_000_000 {
		t.Fatalf("payments = %+v", calc.Payments)
	}
}

func TestBuildDedicatedInputBlocksUnknownRoleAndAnchor(t *testing.T) {
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "needs team",
		ClientCompany:    &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		ClientName:       &domain.Fact[string]{Value: "N", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme: &domain.Fact[string]{Value: "Dedicated", Source: domain.SourceUser, Confirmed: true},
		DurationMonths:   &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true},
		Team:             []domain.PromptTeamLine{{RequestedRole: "unknown", Seniority: "intermediate", HeadcountQuarters: 8, PeriodQuarters: 12, Source: domain.SourceUser}},
		PaymentPattern:   &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
	}
	_, issues := BuildDedicatedInput(state, nil, nil)
	seenRate, seenAnchor := false, false
	for _, issue := range issues {
		seenRate = seenRate || issue.Code == "team.rate_unresolved"
		seenAnchor = seenAnchor || issue.Code == "payment_anchor.missing"
	}
	if !seenRate || !seenAnchor {
		t.Fatalf("expected unresolved-role and anchor issues, got %+v", issues)
	}
}

func TestBuildDedicatedInputBlocksMissingDuration(t *testing.T) {
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "needs duration",
		ClientCompany:    &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		ClientName:       &domain.Fact[string]{Value: "N", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme: &domain.Fact[string]{Value: "Dedicated", Source: domain.SourceUser, Confirmed: true},
		Team:             []domain.PromptTeamLine{{RequestedRole: "programmer", Seniority: "intermediate", HeadcountQuarters: 8, PeriodQuarters: 12, Source: domain.SourceUser}},
		PaymentPattern:   &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
	}
	_, issues := BuildDedicatedInput(state, nil, nil)
	for _, issue := range issues {
		if issue.Code == "duration_months.missing" {
			return
		}
	}
	t.Fatalf("expected missing duration issue, got %+v", issues)
}

func TestBuildDedicatedInputUsesDurationForEqualMonthlyPayments(t *testing.T) {
	id := uuid.New()
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "monthly project",
		ClientCompany:    &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		ClientName:       &domain.Fact[string]{Value: "N", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme: &domain.Fact[string]{Value: "Dedicated", Source: domain.SourceUser, Confirmed: true},
		DurationMonths:   &domain.Fact[int]{Value: 4, Source: domain.SourceUser, Confirmed: true},
		Team:             []domain.PromptTeamLine{{RequestedRole: "backend_programmer", Seniority: "intermediate", ResolvedRateItemID: ptr(id.String()), HeadcountQuarters: 8, PeriodQuarters: 16, Source: domain.SourceUser, Confirmed: true}},
		PaymentPattern:   &domain.Fact[string]{Value: "monthly", Source: domain.SourceUser, Confirmed: true},
	}
	anchor := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	input, issues := BuildDedicatedInput(state, []domain.RateCardItem{{ID: id, Role: "backend_programmer", Seniority: "intermediate", AmountPerMonth: 12_000_000}}, &anchor)
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			t.Fatalf("unexpected blocking issue: %+v", issue)
		}
	}
	if got := len(input.QuotationProfile.PaymentRows); got != 4 {
		t.Fatalf("equal monthly rows = %d, want 4", got)
	}
}

func TestBuildDedicatedInputBlocksGenericProgrammerCatalogItem(t *testing.T) {
	id := uuid.New()
	state := &domain.PromptQuotationState{SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "generic programmer", ClientName: &domain.Fact[string]{Value: "N", Source: domain.SourceUser, Confirmed: true}, CommercialScheme: &domain.Fact[string]{Value: "Dedicated", Source: domain.SourceUser, Confirmed: true}, DurationMonths: &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true}, Team: []domain.PromptTeamLine{{RequestedRole: "programmer", Seniority: "intermediate", ResolvedRateItemID: ptr(id.String()), HeadcountQuarters: 8, PeriodQuarters: 12, Source: domain.SourceUser, Confirmed: true}}, PaymentPattern: &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true}}
	anchor := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	_, issues := BuildDedicatedInput(state, []domain.RateCardItem{{ID: id, Role: "programmer", Seniority: "intermediate", AmountPerMonth: 12_000_000}}, &anchor)
	for _, issue := range issues {
		if issue.Code == "team.rate_unresolved" && issue.Severity == domain.IssueBlocking {
			return
		}
	}
	t.Fatalf("generic programmer catalog item was not blocked: %+v", issues)
}

func TestBuildDedicatedInput_DoesNotMergeMaintenanceDuration(t *testing.T) {
	id := uuid.New()
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "6 bulan dev + 3 bulan maintenance",
		ClientCompany:             &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme:          &domain.Fact[string]{Value: "dedicated_squad", Source: domain.SourceUser, Confirmed: true},
		DurationMonths:            &domain.Fact[int]{Value: 6, Source: domain.SourceUser, Confirmed: true},
		MaintenanceDurationMonths: &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true},
		Team: []domain.PromptTeamLine{
			{
				RequestedRole:      "backend_programmer",
				Seniority:          "intermediate",
				ResolvedRateItemID: ptr(id.String()),
				HeadcountQuarters:  4,
				PeriodQuarters:     36, // AI or user inadvertently passed 9 months (36 quarters)
				Source:             domain.SourceUser,
				Confirmed:          true,
			},
		},
		PaymentPattern: &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
	}
	anchor := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	catalog := []domain.RateCardItem{{ID: id, Role: "backend_programmer", Seniority: "intermediate", AmountPerMonth: 12_000_000}}

	input, _ := BuildDedicatedInput(state, catalog, &anchor)
	if len(input.DevelopmentSquad) != 1 {
		t.Fatalf("expected 1 squad line, got %d", len(input.DevelopmentSquad))
	}
	// PeriodQuarters must be clamped to 6 months * 4 = 24 quarters, not 36
	if got := input.DevelopmentSquad[0].PeriodQuarters; got != 24 {
		t.Fatalf("DevelopmentSquad PeriodQuarters = %d, want 24 (6 months pure dev)", got)
	}
}

func TestBuildDedicatedInput_EmitsExplicitMaintenanceIssue(t *testing.T) {
	id := uuid.New()
	state := &domain.PromptQuotationState{
		SchemaVersion: domain.SchemaPromptQuotationV1, MeetingNotes: "6 bulan dev + 3 bulan maintenance",
		ClientCompany:             &domain.Fact[string]{Value: "PT Arunika", Source: domain.SourceUser, Confirmed: true},
		CommercialScheme:          &domain.Fact[string]{Value: "dedicated_squad", Source: domain.SourceUser, Confirmed: true},
		DurationMonths:            &domain.Fact[int]{Value: 6, Source: domain.SourceUser, Confirmed: true},
		MaintenanceDurationMonths: &domain.Fact[int]{Value: 3, Source: domain.SourceUser, Confirmed: true},
		Team: []domain.PromptTeamLine{
			{
				RequestedRole:      "backend_programmer",
				Seniority:          "intermediate",
				ResolvedRateItemID: ptr(id.String()),
				HeadcountQuarters:  4,
				PeriodQuarters:     24,
				Source:             domain.SourceUser,
				Confirmed:          true,
			},
		},
		PaymentPattern: &domain.Fact[string]{Value: "50:50", Source: domain.SourceUser, Confirmed: true},
	}
	anchor := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	catalog := []domain.RateCardItem{{ID: id, Role: "backend_programmer", Seniority: "intermediate", AmountPerMonth: 12_000_000}}

	_, issues := BuildDedicatedInput(state, catalog, &anchor)
	foundMaintenanceIssue := false
	for _, issue := range issues {
		if issue.Code == "maintenance.detected_unconfigured" {
			foundMaintenanceIssue = true
			if issue.Severity != domain.IssueWarning {
				t.Errorf("expected IssueWarning severity, got %v", issue.Severity)
			}
			if issue.Field != "maintenance_duration_months" {
				t.Errorf("expected field maintenance_duration_months, got %q", issue.Field)
			}
		}
	}
	if !foundMaintenanceIssue {
		t.Fatalf("expected maintenance.detected_unconfigured issue, but got: %+v", issues)
	}
}

func ptr(value string) *string { return &value }

type promptRateLookup struct{ item domain.RateCardItem }

func (l promptRateLookup) ResolveItem(context.Context, string, string, string, string, string) (*domain.RateCardItem, error) {
	return &l.item, nil
}
