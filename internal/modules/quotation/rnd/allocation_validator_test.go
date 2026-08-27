package rnd

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
)

func TestTermTypeBoundary(t *testing.T) {
	if got := termType(2); got != "short_term" {
		t.Fatalf("termType(2) = %q", got)
	}
	if got := termType(3); got != "long_term" {
		t.Fatalf("termType(3) = %q", got)
	}
}

func TestDecodeAllocationRejectsCommercialField(t *testing.T) {
	_, err := DecodeAllocation([]byte(`{
		"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_months":3,"scope_references":["Scope A"],"reason":"Build","rate":12000000}],
		"selected_optional_scope":[],"standard_activities":[]
	}`))
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected commercial field rejection, got %v", err)
	}
}

func TestDecodeAllocationRejectsTrailingJSON(t *testing.T) {
	_, err := DecodeAllocation([]byte(validAllocationJSON() + ` {"extra":true}`))
	if err == nil || !strings.Contains(err.Error(), "single JSON value") {
		t.Fatalf("expected trailing JSON rejection, got %v", err)
	}
}

func TestValidateAndEnrichRejectsInvalidProposal(t *testing.T) {
	input := validAllocationInput()
	cards := validAllocationCards()
	tests := []struct {
		name    string
		mutate  func(*AllocationProposal)
		wantErr string
	}{
		{name: "empty resources", mutate: func(p *AllocationProposal) { p.Resources = nil }, wantErr: "resources"},
		{name: "unknown role", mutate: func(p *AllocationProposal) { p.Resources[0].Role = "architect" }, wantErr: "role"},
		{name: "unknown seniority", mutate: func(p *AllocationProposal) { p.Resources[0].Seniority = "lead" }, wantErr: "seniority"},
		{name: "zero employee count", mutate: func(p *AllocationProposal) { p.Resources[0].EmployeeCount = 0 }, wantErr: "employee_count"},
		{name: "excess employee count", mutate: func(p *AllocationProposal) { p.Resources[0].EmployeeCount = 11 }, wantErr: "employee_count"},
		{name: "zero period", mutate: func(p *AllocationProposal) { p.Resources[0].PeriodMonths = 0 }, wantErr: "period_months"},
		{name: "contract period exceeded", mutate: func(p *AllocationProposal) { p.Resources[0].PeriodMonths = 25 }, wantErr: "period_months"},
		{name: "project period exceeded", mutate: func(p *AllocationProposal) { p.Resources[0].PeriodMonths = 4 }, wantErr: "project_duration_months"},
		{name: "empty scope references", mutate: func(p *AllocationProposal) { p.Resources[0].ScopeReferences = nil }, wantErr: "scope_references"},
		{name: "duplicate scope reference", mutate: func(p *AllocationProposal) { p.Resources[0].ScopeReferences = []string{"Scope A", "Scope A"} }, wantErr: "duplicate"},
		{name: "unknown scope reference", mutate: func(p *AllocationProposal) { p.Resources[0].ScopeReferences = []string{"Invented"} }, wantErr: "scope reference"},
		{name: "missing mandatory scope coverage", mutate: func(p *AllocationProposal) {
			p.SelectedOptionalScope = []string{"Scope B"}
			p.Resources[0].ScopeReferences = []string{"Scope B"}
		}, wantErr: "mandatory scope"},
		{name: "unknown optional selection", mutate: func(p *AllocationProposal) { p.SelectedOptionalScope = []string{"Invented"} }, wantErr: "selected_optional_scope"},
		{name: "duplicate optional selection", mutate: func(p *AllocationProposal) { p.SelectedOptionalScope = []string{"Scope B", "Scope B"} }, wantErr: "duplicate"},
		{name: "unknown standard activity", mutate: func(p *AllocationProposal) { p.StandardActivities = []string{"deployment"} }, wantErr: "standard_activities"},
		{name: "duplicate standard activity", mutate: func(p *AllocationProposal) { p.StandardActivities = []string{"documentation", "documentation"} }, wantErr: "duplicate"},
		{name: "empty reason", mutate: func(p *AllocationProposal) { p.Resources[0].Reason = " " }, wantErr: "reason"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposal := validAllocationProposal()
			tt.mutate(&proposal)
			_, err := ValidateAndEnrich(input, proposal, cards)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateAndEnrichRejectsRateLookupFailures(t *testing.T) {
	input := validAllocationInput()
	proposal := validAllocationProposal()
	tests := []struct {
		name    string
		cards   RateCardFixture
		wantErr string
	}{
		{
			name:    "missing exact rate",
			cards:   RateCardFixture{RateCards: []RateCard{{Code: input.RateCardCode, Currency: "IDR", Items: []RateItem{{Role: "programmer", Seniority: "senior", TermType: "short_term", AmountPerMonth: 12_000_000}}}}},
			wantErr: "missing rate",
		},
		{
			name: "duplicate exact rate",
			cards: RateCardFixture{RateCards: []RateCard{{Code: input.RateCardCode, Currency: "IDR", Items: []RateItem{
				{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: 12_000_000},
				{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: 13_000_000},
			}}}},
			wantErr: "duplicate rate",
		},
		{
			name: "cross market lookup forbidden",
			cards: RateCardFixture{RateCards: []RateCard{
				{Code: input.RateCardCode, Currency: "IDR", Items: []RateItem{{Role: "quality_assurance", Seniority: "associate", TermType: "long_term", AmountPerMonth: 6_000_000}}},
				{Code: "SYNTH-INTERNATIONAL-2026", Currency: "IDR", Items: []RateItem{{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: 18_000_000}}},
			}},
			wantErr: "missing rate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAndEnrich(input, proposal, tt.cards)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateAndEnrichRejectsArithmeticOverflow(t *testing.T) {
	tests := []struct {
		name     string
		input    GenerationInput
		proposal AllocationProposal
		cards    RateCardFixture
		wantErr  string
	}{
		{
			name:     "multiplication overflow",
			input:    allocationInput(math.MaxInt64),
			proposal: proposalFor([]AllocationResource{allocationResource("programmer", "senior", 2, 1, "Scope A")}),
			cards:    cardsFor(RateItem{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: math.MaxInt64}),
			wantErr:  "overflow",
		},
		{
			name:  "sum overflow",
			input: allocationInput(math.MaxInt64),
			proposal: proposalFor([]AllocationResource{
				allocationResource("programmer", "senior", 1, 1, "Scope A"),
				allocationResource("quality_assurance", "senior", 1, 1, "Scope A"),
			}),
			cards: cardsFor(
				RateItem{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: math.MaxInt64/2 + 1},
				RateItem{Role: "quality_assurance", Seniority: "senior", TermType: "long_term", AmountPerMonth: math.MaxInt64/2 + 1},
			),
			wantErr: "overflow",
		},
		{
			name:     "utilization multiplication overflow",
			input:    allocationInput(math.MaxInt64),
			proposal: proposalFor([]AllocationResource{allocationResource("programmer", "senior", 1, 1, "Scope A")}),
			cards:    cardsFor(RateItem{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: math.MaxInt64}),
			wantErr:  "utilization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAndEnrich(tt.input, tt.proposal, tt.cards)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateAndEnrichFixture06ExactBudget(t *testing.T) {
	fixture, cards := loadCommittedAllocationData(t, "06-budget-exact.json")
	proposal := proposalFor([]AllocationResource{{
		Role: "programmer", Seniority: "senior", EmployeeCount: 1, PeriodMonths: 3,
		ScopeReferences: append([]string(nil), fixture.GenerationInput.MandatoryScope...), Reason: "Implementasi scope wajib.",
	}})

	got, err := ValidateAndEnrich(fixture.GenerationInput, proposal, cards)
	if err != nil {
		t.Fatal(err)
	}
	if got.TermType != "long_term" || got.Currency != "IDR" || got.TotalInvestment != 36_000_000 || got.UtilizationPercent != 100 {
		t.Fatalf("unexpected enrichment: %+v", got)
	}
	if len(got.Resources) != 1 || got.Resources[0].AmountPerMonth != 12_000_000 || got.Resources[0].LineTotal != 36_000_000 {
		t.Fatalf("unexpected resource enrichment: %+v", got.Resources)
	}
}

func TestValidateAndEnrichFixture07UnderUtilized(t *testing.T) {
	fixture, cards := loadCommittedAllocationData(t, "07-budget-under-utilized.json")
	proposal := proposalFor([]AllocationResource{
		{Role: "programmer", Seniority: "intermediate", EmployeeCount: 1, PeriodMonths: 4, ScopeReferences: []string{fixture.GenerationInput.MandatoryScope[0], fixture.GenerationInput.MandatoryScope[1]}, Reason: "Implementasi."},
		{Role: "quality_assurance", Seniority: "intermediate", EmployeeCount: 1, PeriodMonths: 4, ScopeReferences: []string{fixture.GenerationInput.MandatoryScope[2]}, Reason: "Validasi."},
	})

	got, err := ValidateAndEnrich(fixture.GenerationInput, proposal, cards)
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalInvestment != 72_000_000 || got.UtilizationPercent != 72 {
		t.Fatalf("unexpected total/utilization: %+v", got)
	}
}

func TestValidateAndEnrichFixture08ReturnsBudgetExceeded(t *testing.T) {
	fixture, cards := loadCommittedAllocationData(t, "08-budget-too-small.json")
	proposal := proposalFor([]AllocationResource{{
		Role: "quality_assurance", Seniority: "associate", EmployeeCount: 1, PeriodMonths: 1,
		ScopeReferences: append([]string(nil), fixture.GenerationInput.MandatoryScope...), Reason: "Cheapest valid allocation.",
	}})

	_, err := ValidateAndEnrich(fixture.GenerationInput, proposal, cards)
	var budgetErr *BudgetExceededError
	if !errors.As(err, &budgetErr) {
		t.Fatalf("expected BudgetExceededError, got %v", err)
	}
	if budgetErr.Total != 7_200_000 || budgetErr.Budget != 5_000_000 || budgetErr.Excess != 2_200_000 {
		t.Fatalf("unexpected budget error: %+v", budgetErr)
	}
}

func validAllocationJSON() string {
	return `{"resources":[{"role":"programmer","seniority":"senior","employee_count":1,"period_months":3,"scope_references":["Scope A"],"reason":"Build"}],"selected_optional_scope":[],"standard_activities":[]}`
}

func validAllocationInput() GenerationInput {
	return GenerationInput{
		CommercialScheme: "dedicated_squad", RateCardCode: "SYNTH-NATIONAL-2026",
		ProjectDurationMonths: 3, BudgetMaximum: 36_000_000,
		MandatoryScope: []string{"Scope A"}, OptionalScope: []string{"Scope B"},
		AllowedStandardActivities: []string{"documentation"},
	}
}

func validAllocationProposal() AllocationProposal {
	return proposalFor([]AllocationResource{allocationResource("programmer", "senior", 1, 3, "Scope A")})
}

func validAllocationCards() RateCardFixture {
	return cardsFor(RateItem{Role: "programmer", Seniority: "senior", TermType: "long_term", AmountPerMonth: 12_000_000})
}

func allocationInput(budget int64) GenerationInput {
	input := validAllocationInput()
	input.ProjectDurationMonths = 3
	input.BudgetMaximum = budget
	return input
}

func allocationResource(role, seniority string, count, period int64, scopes ...string) AllocationResource {
	return AllocationResource{Role: role, Seniority: seniority, EmployeeCount: count, PeriodMonths: period, ScopeReferences: scopes, Reason: "Valid reason"}
}

func proposalFor(resources []AllocationResource) AllocationProposal {
	return AllocationProposal{Resources: resources, SelectedOptionalScope: []string{}, StandardActivities: []string{}}
}

func cardsFor(items ...RateItem) RateCardFixture {
	return RateCardFixture{RateCards: []RateCard{{Code: "SYNTH-NATIONAL-2026", Market: "national", Currency: "IDR", Items: items}}}
}

func loadCommittedAllocationData(t *testing.T, fixtureName string) (Fixture, RateCardFixture) {
	t.Helper()
	root := filepath.Join("testdata", "quotation-skill")
	fixture, err := loadFixture(filepath.Join(root, "fixtures", fixtureName))
	if err != nil {
		t.Fatal(err)
	}
	var cards RateCardFixture
	if err := decodeStrictFile(filepath.Join(root, "rate-card-fixture.json"), &cards); err != nil {
		t.Fatal(err)
	}
	return fixture, cards
}
