package rnd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrLegalTemplateUnapproved = errors.New("legal template is not approved")

type Stage string

const (
	StageAllocation Stage = "allocation"
	StageHTML       Stage = "html"
)

type ProviderRequest struct {
	Stage       Stage
	Model       string
	System      string
	Instruction string
	Payload     any
}

type ProviderResponse struct {
	Raw     string
	Latency time.Duration
}

type Provider interface {
	Name() string
	Generate(ctx context.Context, request ProviderRequest) (ProviderResponse, error)
}

type ClientInput struct {
	CompanyName string `json:"company_name"`
	PICName     string `json:"pic_name"`
}

type QuotationInput struct {
	Number  string `json:"number"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

type PaymentTermItem struct {
	Label       string `json:"label"`
	Percentage  int64  `json:"percentage"`
	InvoiceDate string `json:"invoice_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
}

type PaymentTerms struct {
	Mode  string            `json:"mode"`
	Items []PaymentTermItem `json:"items"`
}

type GenerationInput struct {
	Client                    ClientInput    `json:"client"`
	Quotation                 QuotationInput `json:"quotation"`
	SolutionType              string         `json:"solution_type"`
	CommercialScheme          string         `json:"commercial_scheme"`
	RateCardCode              string         `json:"rate_card_code"`
	ProjectDurationMonths     int64          `json:"project_duration_months"`
	BudgetMaximum             int64          `json:"budget_maximum"`
	Summary                   string         `json:"summary"`
	MandatoryScope            []string       `json:"mandatory_scope"`
	OptionalScope             []string       `json:"optional_scope"`
	AllowedStandardActivities []string       `json:"allowed_standard_activities"`
	PaymentTerms              PaymentTerms   `json:"payment_terms"`
	AdditionalNotes           string         `json:"additional_notes"`
}

type AllocationOracleResource struct {
	Role           string `json:"role"`
	Seniority      string `json:"seniority"`
	EmployeeCount  int64  `json:"employee_count"`
	PeriodMonths   int64  `json:"period_months"`
	AmountPerMonth int64  `json:"amount_per_month"`
	LineTotal      int64  `json:"line_total"`
}

type AllocationOracle struct {
	Resources          []AllocationOracleResource `json:"resources"`
	TotalInvestment    int64                      `json:"total_investment"`
	UtilizationPercent int64                      `json:"utilization_percent"`
}

type InfeasibilityOracle struct {
	CheapestRole           string `json:"cheapest_role"`
	CheapestSeniority      string `json:"cheapest_seniority"`
	CheapestAmountPerMonth int64  `json:"cheapest_amount_per_month"`
	BudgetMaximum          int64  `json:"budget_maximum"`
	MinimumExcessAmount    int64  `json:"minimum_excess_amount"`
	MaximumIterations      int64  `json:"maximum_iterations"`
	HTMLExpected           bool   `json:"html_expected"`
}

type ExpectedFixture struct {
	Validation          string               `json:"validation"`
	TermType            string               `json:"term_type"`
	AllocationOutcome   string               `json:"allocation_outcome"`
	SecurityOutcome     string               `json:"security_outcome"`
	AllocationOracle    *AllocationOracle    `json:"allocation_oracle,omitempty"`
	InfeasibilityOracle *InfeasibilityOracle `json:"infeasibility_oracle,omitempty"`
	TableShape          string               `json:"table_shape,omitempty"`
	RequiredRoles       []string             `json:"required_roles,omitempty"`
	AdversarialOutcome  string               `json:"adversarial_outcome,omitempty"`
	Assertions          []string             `json:"assertions"`
}

type Fixture struct {
	FixtureID       string          `json:"fixture_id"`
	Description     string          `json:"description"`
	GenerationInput GenerationInput `json:"generation_input"`
	Expected        ExpectedFixture `json:"expected"`
}

type RateItem struct {
	Role           string `json:"role"`
	Seniority      string `json:"seniority"`
	TermType       string `json:"term_type"`
	AmountPerMonth int64  `json:"amount_per_month"`
}

type RateCard struct {
	Code     string     `json:"code"`
	Market   string     `json:"market"`
	Currency string     `json:"currency"`
	Items    []RateItem `json:"items"`
}

type RateCardFixture struct {
	SchemaVersion string     `json:"schema_version"`
	FixtureNotice string     `json:"fixture_notice"`
	RateCards     []RateCard `json:"rate_cards"`
	CurrencyRule  string     `json:"currency_rule"`
}

type AllocationResource struct {
	Role            string   `json:"role"`
	Seniority       string   `json:"seniority"`
	EmployeeCount   int64    `json:"employee_count"`
	PeriodMonths    int64    `json:"period_months"`
	ScopeReferences []string `json:"scope_references"`
	Reason          string   `json:"reason"`
}

type AllocationProposal struct {
	Resources             []AllocationResource `json:"resources"`
	SelectedOptionalScope []string             `json:"selected_optional_scope"`
	StandardActivities    []string             `json:"standard_activities"`
}

type EnrichedResource struct {
	Role            string
	Seniority       string
	TermType        string
	EmployeeCount   int64
	PeriodMonths    int64
	AmountPerMonth  int64
	LineTotal       int64
	ScopeReferences []string
	Reason          string
}

type EnrichedAllocation struct {
	Resources             []EnrichedResource
	SelectedOptionalScope []string
	StandardActivities    []string
	Currency              string
	TermType              string
	TotalInvestment       int64
	UtilizationPercent    int64
}

type ArtifactSet struct {
	RateCards         RateCardFixture
	SafetyEnvelope    string
	Prompt            string
	LegalStatus       string
	LegalTemplateCode string
	LegalChecksum     string
	LegalHTML         string
}

type RunnerConfig struct {
	ArtifactRoot string
	OutputDir    string
	RawDir       string
	Provider     Provider
	Model        string
	Runs         int
	Clock        func() time.Time
}

type Runner struct {
	config    RunnerConfig
	artifacts ArtifactSet
}

func ReportPath(outputDir, fixtureID string) (string, error) {
	safeFixture := safeFixtureID(fixtureID)
	if safeFixture == "" {
		return "", errors.New("fixture_id has no safe filename characters")
	}
	return filepath.Join(outputDir, safeFixture+"-report.json"), nil
}

func NewRunner(config RunnerConfig) (*Runner, error) {
	if strings.TrimSpace(config.ArtifactRoot) == "" {
		return nil, errors.New("artifact root is required")
	}
	if strings.TrimSpace(config.OutputDir) == "" {
		return nil, errors.New("output directory is required")
	}
	if strings.TrimSpace(config.RawDir) == "" {
		return nil, errors.New("raw directory is required")
	}
	if config.Provider == nil {
		return nil, errors.New("provider is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("model is required")
	}
	if config.Runs < 1 || config.Runs > 10 {
		return nil, errors.New("runs must be between 1 and 10")
	}

	artifacts, err := loadArtifacts(config.ArtifactRoot)
	if err != nil {
		return nil, err
	}
	if artifacts.LegalStatus != "approved" || artifacts.LegalChecksum == "" || strings.HasPrefix(artifacts.LegalChecksum, "pending") {
		return nil, ErrLegalTemplateUnapproved
	}
	if config.Clock == nil {
		config.Clock = time.Now
	}

	return &Runner{config: config, artifacts: artifacts}, nil
}

func (r *Runner) Run(ctx context.Context, fixturePath string) (*BatchReport, error) {
	fixture, err := loadFixtureWithArtifacts(fixturePath, r.artifacts)
	if err != nil {
		return nil, err
	}
	safeFixture := safeFixtureID(fixture.FixtureID)
	if safeFixture == "" {
		return nil, errors.New("fixture_id has no safe filename characters")
	}
	if err := os.MkdirAll(r.config.OutputDir, 0o750); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}
	if err := os.MkdirAll(r.config.RawDir, 0o700); err != nil {
		return nil, fmt.Errorf("create raw directory: %w", err)
	}
	if err := os.Chmod(r.config.RawDir, 0o700); err != nil {
		return nil, fmt.Errorf("protect raw directory: %w", err)
	}

	report := &BatchReport{
		SchemaVersion: "quotation-rnd-report-v1", Fixture: fixture.FixtureID,
		Provider: r.config.Provider.Name(), Model: r.config.Model, PromptVersion: "quotation-v1",
	}
	reportPath, err := ReportPath(r.config.OutputDir, fixture.FixtureID)
	if err != nil {
		return nil, err
	}
	for run := 1; run <= r.config.Runs; run++ {
		runReport, runErr := r.runOnce(ctx, fixture, safeFixture, run)
		report.Runs = append(report.Runs, runReport)
		report.Aggregate = aggregateRuns(report.Runs)
		if err := WriteReport(reportPath, *report); err != nil {
			return report, err
		}
		if runErr != nil {
			return report, runErr
		}
	}
	return report, nil
}

type allocationPromptInput struct {
	Client                    ClientInput    `json:"client"`
	Quotation                 QuotationInput `json:"quotation"`
	SolutionType              string         `json:"solution_type"`
	CommercialScheme          string         `json:"commercial_scheme"`
	RateCardCode              string         `json:"rate_card_code"`
	ProjectDurationMonths     int64          `json:"project_duration_months"`
	Summary                   string         `json:"summary"`
	MandatoryScope            []string       `json:"mandatory_scope"`
	OptionalScope             []string       `json:"optional_scope"`
	AllowedStandardActivities []string       `json:"allowed_standard_activities"`
	AdditionalNotes           string         `json:"additional_notes"`
}

type retryFeedback struct {
	AttemptNumber      int   `json:"attempt_number"`
	MaximumAttempts    int   `json:"maximum_attempts"`
	BudgetExcessAmount int64 `json:"budget_excess_amount"`
}

type allocationPayload struct {
	GenerationInput    allocationPromptInput `json:"generation_input"`
	TermType           string                `json:"term_type"`
	AllowedRoles       []string              `json:"allowed_roles"`
	AllowedSeniorities []string              `json:"allowed_seniorities"`
	RetryFeedback      *retryFeedback        `json:"retry_feedback,omitempty"`
}

type htmlPayload struct {
	Metadata  string              `json:"metadata"`
	Scope     string              `json:"scope"`
	Closing   string              `json:"closing"`
	Signature string              `json:"signature"`
	Blocks    DeterministicBlocks `json:"blocks"`
}

func (r *Runner) runOnce(ctx context.Context, fixture Fixture, safeFixture string, run int) (RunReport, error) {
	started := r.config.Clock()
	record := RunReport{Run: run, StartedAt: started, TermType: termType(fixture.GenerationInput.ProjectDurationMonths), Currency: "IDR", RoundTripResult: "not_run"}
	var feedback *retryFeedback
	var allocation EnrichedAllocation
	var totalLatency time.Duration
	for attempt := 1; attempt <= 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return failedRun(record, "CONTEXT_CANCELLED", totalLatency), err
		}
		response, err := r.config.Provider.Generate(ctx, ProviderRequest{
			Stage: StageAllocation, Model: r.config.Model, System: r.artifacts.SafetyEnvelope,
			Instruction: r.artifacts.Prompt, Payload: makeAllocationPayload(fixture.GenerationInput, feedback),
		})
		record.Iterations = attempt
		if err != nil {
			return failedRun(record, "ALLOCATION_PROVIDER_ERROR", totalLatency), err
		}
		totalLatency += response.Latency
		if err := r.writeRaw(safeFixture, run, attempt, "allocation", response.Raw); err != nil {
			return failedRun(record, "RAW_WRITE_ERROR", totalLatency), err
		}
		proposal, err := DecodeAllocation([]byte(response.Raw))
		if err != nil {
			return failedRun(record, "INVALID_ALLOCATION", totalLatency), err
		}
		allocation, err = ValidateAndEnrich(fixture.GenerationInput, proposal, r.artifacts.RateCards)
		if err == nil {
			break
		}
		var exceeded *BudgetExceededError
		if !errors.As(err, &exceeded) {
			return failedRun(record, "INVALID_ALLOCATION", totalLatency), err
		}
		if attempt == 3 {
			security, structural := newExpectedFailureChecks()
			record.SecurityChecks, record.StructuralChecks = security, structural
			if fixture.Expected.AllocationOutcome == "budget_unfeasible" && budgetFundamentallyUnfeasible(fixture.GenerationInput, r.artifacts.RateCards) {
				record.Status = "budget_unfeasible_expected"
				record.ReasonCode = "BUDGET_UNFEASIBLE_AFTER_MAX_ATTEMPTS"
				record.LatencyMS = totalLatency.Milliseconds()
				return record, nil
			}
			return failedRun(record, "UNEXPECTED_BUDGET_UNFEASIBLE", totalLatency), ErrBudgetUnfeasible
		}
		feedback = &retryFeedback{AttemptNumber: attempt + 1, MaximumAttempts: 3, BudgetExcessAmount: exceeded.Excess}
	}

	blocks := RenderDeterministicBlocks(fixture.GenerationInput, allocation, r.artifacts.LegalHTML)
	response, err := r.config.Provider.Generate(ctx, ProviderRequest{
		Stage: StageHTML, Model: r.config.Model, System: r.artifacts.SafetyEnvelope,
		Instruction: r.artifacts.Prompt, Payload: makeHTMLPayload(fixture.GenerationInput, allocation, blocks),
	})
	if err != nil {
		return failedRun(record, "HTML_PROVIDER_ERROR", totalLatency), err
	}
	totalLatency += response.Latency
	if err := r.writeRaw(safeFixture, run, 1, "html", response.Raw); err != nil {
		return failedRun(record, "RAW_WRITE_ERROR", totalLatency), err
	}
	sanitized, validation, validateErr := SanitizeAndValidate(response.Raw, HTMLExpectation{Input: fixture.GenerationInput, Allocation: allocation, Blocks: blocks, LegalHTML: r.artifacts.LegalHTML})
	record.SecurityChecks, record.StructuralChecks = validation.Security, validation.Structural
	record.TotalInvestment, record.UtilizationPercent = int64Pointer(allocation.TotalInvestment), int64Pointer(allocation.UtilizationPercent)
	record.LatencyMS = totalLatency.Milliseconds()
	if validateErr != nil {
		record.Status, record.ReasonCode = "fail", "HTML_VALIDATION_FAILED"
		return record, validateErr
	}
	filename := fmt.Sprintf("%s-run-%02d.html", safeFixture, run)
	if err := writeAtomic(filepath.Join(r.config.OutputDir, filename), []byte(sanitized), 0o640); err != nil {
		return failedRun(record, "SANITIZED_WRITE_ERROR", totalLatency), err
	}
	record.Status = "pass"
	record.SanitizedEvidencePath = filename
	return record, nil
}

func makeAllocationPayload(input GenerationInput, feedback *retryFeedback) allocationPayload {
	return allocationPayload{
		GenerationInput:    allocationPromptInput{Client: input.Client, Quotation: input.Quotation, SolutionType: input.SolutionType, CommercialScheme: input.CommercialScheme, RateCardCode: input.RateCardCode, ProjectDurationMonths: input.ProjectDurationMonths, Summary: input.Summary, MandatoryScope: input.MandatoryScope, OptionalScope: input.OptionalScope, AllowedStandardActivities: input.AllowedStandardActivities, AdditionalNotes: input.AdditionalNotes},
		TermType:           termType(input.ProjectDurationMonths),
		AllowedRoles:       []string{"business_analyst", "ui_ux_designer", "programmer", "quality_assurance", "devops"},
		AllowedSeniorities: []string{"associate", "intermediate", "senior"}, RetryFeedback: feedback,
	}
}

func makeHTMLPayload(input GenerationInput, allocation EnrichedAllocation, blocks DeterministicBlocks) htmlPayload {
	metadata := renderNode(element("section", attr("data-block-type", "metadata"), element("h1", text(input.Quotation.Subject)), element("p", text(input.Quotation.Number)), element("p", text(input.Quotation.Date)), element("p", text(input.Client.CompanyName)), element("p", text(input.Client.PICName)))) +
		renderNode(element("section", attr("data-block-type", "opening"), element("p", text("Kami menyampaikan penawaran untuk mendukung kebutuhan yang sesuai dan relevan dengan ruang lingkup proyek."))))
	items := element("ul")
	for _, item := range append(append(append([]string{}, input.MandatoryScope...), allocation.SelectedOptionalScope...), allocation.StandardActivities...) {
		items.AppendChild(element("li", text(item)))
	}
	scope := renderNode(element("section", attr("data-block-type", "scope"), element("h2", text("Scope of Work dan Timeline")), items))
	closing := renderNode(element("section", attr("data-block-type", "closing"), element("p", text("Demikian penawaran ini kami sampaikan untuk dapat dipertimbangkan dan ditindaklanjuti sesuai kebutuhan."))))
	signature := renderNode(element("section", attr("data-block-type", "signature"), element("p", text("Hormat kami"))))
	return htmlPayload{Metadata: metadata, Scope: scope, Closing: closing, Signature: signature, Blocks: blocks}
}

func (r *Runner) writeRaw(fixture string, run, attempt int, stage, raw string) error {
	name := fmt.Sprintf("%s-run-%02d-attempt-%02d-%s.raw", fixture, run, attempt, stage)
	path := filepath.Join(r.config.RawDir, name)
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func failedRun(record RunReport, reason string, latency time.Duration) RunReport {
	record.Status, record.ReasonCode, record.LatencyMS = "fail", reason, latency.Milliseconds()
	if record.SecurityChecks == nil {
		record.SecurityChecks = newHTMLValidation().Security
	}
	if record.StructuralChecks == nil {
		record.StructuralChecks = newHTMLValidation().Structural
	}
	return record
}

func aggregateRuns(runs []RunReport) AggregateReport {
	aggregate := AggregateReport{RunCount: len(runs), SecurityPass: true, Status: "pass"}
	for _, run := range runs {
		accepted := run.Status == "pass" || run.Status == "budget_unfeasible_expected"
		if accepted {
			aggregate.PassCount++
		} else {
			aggregate.FailCount++
			aggregate.Status = "fail"
		}
		if run.Status == "budget_unfeasible_expected" {
			aggregate.ExpectedBudgetUnfeasibleCount++
		}
		securityPass := allChecksPass(run.SecurityChecks, "S")
		if !securityPass {
			aggregate.SecurityPass, aggregate.Status = false, "fail"
		}
		if securityPass && allChecksPass(run.StructuralChecks, "T") {
			aggregate.StructuralFullPassCount++
		}
	}
	return aggregate
}

func budgetFundamentallyUnfeasible(input GenerationInput, cards RateCardFixture) bool {
	minimum := int64(0)
	term := termType(input.ProjectDurationMonths)
	for _, card := range cards.RateCards {
		if card.Code != input.RateCardCode {
			continue
		}
		for _, item := range card.Items {
			if item.TermType == term && (minimum == 0 || item.AmountPerMonth < minimum) {
				minimum = item.AmountPerMonth
			}
		}
	}
	return minimum > input.BudgetMaximum
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".output-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

var unsafeFixtureCharacters = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func safeFixtureID(value string) string {
	value = unsafeFixtureCharacters.ReplaceAllString(strings.TrimSpace(value), "-")
	value = strings.Trim(value, ".-")
	if len(value) > 100 {
		value = value[:100]
	}
	return value
}

func int64Pointer(value int64) *int64 { return &value }

func decodeStrictFile(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", filepath.Base(path), err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("decode %s: expected a single JSON value", filepath.Base(path))
		}
		return fmt.Errorf("decode %s: trailing data after JSON value: %w", filepath.Base(path), err)
	}
	return nil
}

func loadFixture(path string) (Fixture, error) {
	var fixture Fixture
	if err := decodeStrictFile(path, &fixture); err != nil {
		return Fixture{}, err
	}
	if strings.TrimSpace(fixture.FixtureID) == "" {
		return Fixture{}, errors.New("fixture_id is required")
	}
	if strings.TrimSpace(fixture.Description) == "" {
		return Fixture{}, errors.New("description is required")
	}
	if err := validateGenerationInput(fixture.GenerationInput); err != nil {
		return Fixture{}, fmt.Errorf("validate generation_input: %w", err)
	}
	return fixture, nil
}

func loadFixtureWithArtifacts(path string, artifacts ArtifactSet) (Fixture, error) {
	fixture, err := loadFixture(path)
	if err != nil {
		return Fixture{}, err
	}
	for _, card := range artifacts.RateCards.RateCards {
		if card.Code == fixture.GenerationInput.RateCardCode {
			return fixture, nil
		}
	}
	return Fixture{}, fmt.Errorf("rate_card_code %q does not resolve to a loaded rate card", fixture.GenerationInput.RateCardCode)
}

func loadArtifacts(root string) (ArtifactSet, error) {
	read := func(name string) (string, error) {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return "", fmt.Errorf("read required artifact %s: %w", name, err)
		}
		return string(content), nil
	}

	var artifacts ArtifactSet
	if err := decodeStrictFile(filepath.Join(root, "rate-card-fixture.json"), &artifacts.RateCards); err != nil {
		return ArtifactSet{}, err
	}
	if err := validateRateCards(artifacts.RateCards); err != nil {
		return ArtifactSet{}, fmt.Errorf("validate rate-card-fixture.json: %w", err)
	}

	var err error
	if artifacts.SafetyEnvelope, err = read("safety-envelope.md"); err != nil {
		return ArtifactSet{}, err
	}
	if artifacts.Prompt, err = read("prompt-v1.md"); err != nil {
		return ArtifactSet{}, err
	}
	legalMarkdown, err := read("legal-template-v1.md")
	if err != nil {
		return ArtifactSet{}, err
	}
	artifacts.LegalStatus, artifacts.LegalTemplateCode, artifacts.LegalChecksum, artifacts.LegalHTML, err = extractLegalArtifact(legalMarkdown)
	if err != nil {
		return ArtifactSet{}, fmt.Errorf("parse legal-template-v1.md: %w", err)
	}
	return artifacts, nil
}

func validateGenerationInput(input GenerationInput) error {
	if err := validateRequiredText("client.company_name", input.Client.CompanyName, 160); err != nil {
		return err
	}
	if err := validateRequiredText("client.pic_name", input.Client.PICName, 160); err != nil {
		return err
	}
	if err := validateRequiredText("quotation.number", input.Quotation.Number, 80); err != nil {
		return err
	}
	if err := validateDate("quotation.date", input.Quotation.Date); err != nil {
		return err
	}
	if err := validateRequiredText("quotation.subject", input.Quotation.Subject, 200); err != nil {
		return err
	}
	if !containsString([]string{"website", "mobile_application", "custom_software", "api_integration", "other"}, input.SolutionType) {
		return fmt.Errorf("solution_type %q is not supported", input.SolutionType)
	}
	if input.CommercialScheme != "dedicated_squad" {
		return fmt.Errorf("commercial_scheme must be dedicated_squad")
	}
	if input.RateCardCode != "SYNTH-NATIONAL-2026" && input.RateCardCode != "SYNTH-INTERNATIONAL-2026" {
		return fmt.Errorf("rate_card_code %q is invalid", input.RateCardCode)
	}
	if input.ProjectDurationMonths < 1 || input.ProjectDurationMonths > 24 {
		return errors.New("project_duration_months must be between 1 and 24")
	}
	if input.BudgetMaximum < 1 {
		return errors.New("budget_maximum must be positive integer IDR")
	}
	if err := validateRequiredText("summary", input.Summary, 1000); err != nil {
		return err
	}
	if err := validateStringList("mandatory_scope", input.MandatoryScope, 1, 50); err != nil {
		return err
	}
	if err := validateStringList("optional_scope", input.OptionalScope, 0, 50); err != nil {
		return err
	}
	if err := validateStringList("allowed_standard_activities", input.AllowedStandardActivities, 0, 50); err != nil {
		return err
	}
	if input.PaymentTerms.Mode != "milestone" && input.PaymentTerms.Mode != "monthly" {
		return errors.New("payment_terms.mode must be milestone or monthly")
	}
	if len(input.PaymentTerms.Items) > 24 {
		return errors.New("payment_terms.items must contain at most 24 items")
	}
	var percentageTotal int64
	for i, item := range input.PaymentTerms.Items {
		if err := validateRequiredText(fmt.Sprintf("payment_terms.items[%d].label", i), item.Label, 100); err != nil {
			return err
		}
		if item.Percentage < 1 || item.Percentage > 100 {
			return fmt.Errorf("payment_terms.items[%d].percentage must be between 1 and 100", i)
		}
		percentageTotal += item.Percentage
		if item.InvoiceDate != "" {
			if err := validateDate(fmt.Sprintf("payment_terms.items[%d].invoice_date", i), item.InvoiceDate); err != nil {
				return err
			}
		}
		if item.DueDate != "" {
			if err := validateDate(fmt.Sprintf("payment_terms.items[%d].due_date", i), item.DueDate); err != nil {
				return err
			}
		}
	}
	if len(input.PaymentTerms.Items) > 0 && percentageTotal != 100 {
		return fmt.Errorf("payment_terms percentages must total 100, got %d", percentageTotal)
	}
	if utf8.RuneCountInString(input.AdditionalNotes) > 2000 {
		return errors.New("additional_notes must contain at most 2000 characters")
	}
	return nil
}

func validateRateCards(cards RateCardFixture) error {
	if cards.SchemaVersion != "quotation-rate-card-fixture-v1" {
		return fmt.Errorf("unsupported schema_version %q", cards.SchemaVersion)
	}
	if len(cards.RateCards) == 0 {
		return errors.New("rate_cards must not be empty")
	}
	cardCodes := make(map[string]struct{}, len(cards.RateCards))
	compoundKeys := make(map[string]struct{})
	for cardIndex, card := range cards.RateCards {
		if strings.TrimSpace(card.Code) == "" {
			return fmt.Errorf("rate_cards[%d].code is required", cardIndex)
		}
		if _, exists := cardCodes[card.Code]; exists {
			return fmt.Errorf("duplicate rate card code %q", card.Code)
		}
		cardCodes[card.Code] = struct{}{}
		if card.Currency != "IDR" {
			return fmt.Errorf("rate card %q currency must be IDR", card.Code)
		}
		if len(card.Items) == 0 {
			return fmt.Errorf("rate card %q items must not be empty", card.Code)
		}
		for itemIndex, item := range card.Items {
			if !containsString([]string{"business_analyst", "ui_ux_designer", "programmer", "quality_assurance", "devops"}, item.Role) {
				return fmt.Errorf("rate card %q item %d has invalid role %q", card.Code, itemIndex, item.Role)
			}
			if !containsString([]string{"associate", "intermediate", "senior"}, item.Seniority) {
				return fmt.Errorf("rate card %q item %d has invalid seniority %q", card.Code, itemIndex, item.Seniority)
			}
			if item.TermType != "short_term" && item.TermType != "long_term" {
				return fmt.Errorf("rate card %q item %d has invalid term_type %q", card.Code, itemIndex, item.TermType)
			}
			if item.AmountPerMonth <= 0 {
				return fmt.Errorf("rate card %q item %d amount_per_month must be positive", card.Code, itemIndex)
			}
			key := strings.Join([]string{card.Code, item.Role, item.Seniority, item.TermType}, "|")
			if _, exists := compoundKeys[key]; exists {
				return fmt.Errorf("duplicate rate compound key %q", key)
			}
			compoundKeys[key] = struct{}{}
		}
	}
	return nil
}

func extractLegalArtifact(markdown string) (status, templateCode, checksum, html string, err error) {
	markdown = strings.ReplaceAll(markdown, "\r\n", "\n")
	lines := strings.Split(markdown, "\n")

	yamlStart := -1
	yamlEnd := -1
	for i, line := range lines {
		if line == "```yaml" {
			if yamlStart != -1 {
				return "", "", "", "", errors.New("duplicate YAML fence")
			}
			yamlStart = i
			continue
		}
		if yamlStart != -1 && yamlEnd == -1 && line == "```" {
			yamlEnd = i
		}
	}
	if yamlStart == -1 || yamlEnd == -1 || yamlEnd <= yamlStart+1 {
		return "", "", "", "", errors.New("missing or empty YAML fence")
	}

	values := map[string]string{}
	for _, line := range lines[yamlStart+1 : yamlEnd] {
		for _, key := range []string{"status", "template_code", "checksum"} {
			prefix := key + ":"
			if strings.HasPrefix(line, prefix) {
				if _, exists := values[key]; exists {
					return "", "", "", "", fmt.Errorf("duplicate legal metadata %s", key)
				}
				values[key] = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			}
		}
	}
	status = values["status"]
	templateCode = values["template_code"]
	checksum = values["checksum"]
	if status == "" {
		return "", "", "", "", errors.New("missing legal metadata status")
	}
	if templateCode != "quotation-legal-v1" {
		return "", "", "", "", fmt.Errorf("legal metadata template_code must be quotation-legal-v1, got %q", templateCode)
	}
	if checksum == "" {
		return "", "", "", "", errors.New("missing legal metadata checksum")
	}

	headingIndex := -1
	for i, line := range lines {
		if line == "## Deterministic HTML Block" {
			if headingIndex != -1 {
				return "", "", "", "", errors.New("duplicate Deterministic HTML Block heading")
			}
			headingIndex = i
		}
	}
	if headingIndex == -1 {
		return "", "", "", "", errors.New("missing Deterministic HTML Block heading")
	}
	htmlStart := -1
	htmlEnd := -1
	for i := headingIndex + 1; i < len(lines); i++ {
		if lines[i] == "```html" {
			if htmlStart != -1 {
				return "", "", "", "", errors.New("duplicate deterministic HTML fence")
			}
			htmlStart = i
			continue
		}
		if htmlStart != -1 && htmlEnd == -1 && lines[i] == "```" {
			htmlEnd = i
			break
		}
	}
	if htmlStart == -1 || htmlEnd == -1 || htmlEnd <= htmlStart+1 {
		return "", "", "", "", errors.New("missing or empty deterministic HTML fence")
	}
	for i := htmlEnd + 1; i < len(lines); i++ {
		if lines[i] == "```html" {
			return "", "", "", "", errors.New("duplicate deterministic HTML fence")
		}
	}

	html = strings.TrimSpace(strings.Join(lines[htmlStart+1:htmlEnd], "\n"))
	const legalMarker = `<section data-block-type="legal" data-template-code="quotation-legal-v1" data-modified="false">`
	if !strings.Contains(html, legalMarker) {
		return "", "", "", "", errors.New("deterministic HTML is missing exact legal section marker")
	}
	return status, templateCode, checksum, html, nil
}

func validateRequiredText(field, value string, maxLength int) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%s must contain at most %d characters", field, maxLength)
	}
	return nil
}

func validateDate(field, value string) error {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return fmt.Errorf("%s must be a valid YYYY-MM-DD date", field)
	}
	return nil
}

func validateStringList(field string, values []string, minimum, maximum int) error {
	if len(values) < minimum || len(values) > maximum {
		return fmt.Errorf("%s must contain between %d and %d items", field, minimum, maximum)
	}
	seen := make(map[string]struct{}, len(values))
	for i, value := range values {
		if err := validateRequiredText(fmt.Sprintf("%s[%d]", field, i), value, 200); err != nil {
			return err
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains duplicate value %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
