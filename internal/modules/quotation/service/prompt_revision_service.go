package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/google/uuid"

	"venturo-skeleton-go/internal/modules/quotation/domain"
	"venturo-skeleton-go/internal/modules/quotation/dto"
	"venturo-skeleton-go/internal/modules/quotation/repository"
)

var (
	ErrDiscountPolicyUnverified = errors.New("discount policy is unverified")
	ErrDiscountAboveMaximum     = errors.New("discount exceeds maximum")
	ErrDiscountBelowFloor       = errors.New("discount violates floor")
)

type PromptRevisionError struct {
	Code string
	err  error
}

func (e *PromptRevisionError) Error() string    { return e.Code }
func (e *PromptRevisionError) Unwrap() error    { return e.err }
func (e *PromptRevisionError) SafeCode() string { return e.Code }

type promptRevisionRepository interface {
	LoadPromptDraft(context.Context, uuid.UUID, uuid.UUID) (*repository.PromptDraftRecord, error)
	UpdatePromptDraftCAS(context.Context, repository.UpdatePromptDraftCASParams) (*repository.PromptDraftSaveResult, error)
}

// DiscountPolicy is intentionally injected. The business register has not
// verified discount basis/max/floor yet, so production uses the unverified
// policy while focused tests can supply a clearly bounded fixture policy.
type DiscountPolicy interface {
	ValidateAdditionalDiscount(totalBeforeDiscount int64, bps int32) error
}

type FixedDiscountPolicy struct {
	MaxBPS int32
	Floor  int64
}

func (p FixedDiscountPolicy) ValidateAdditionalDiscount(totalBeforeDiscount int64, bps int32) error {
	if bps < 0 || bps > p.MaxBPS {
		return ErrDiscountAboveMaximum
	}
	discount := splitAmount(totalBeforeDiscount, int64(bps))
	if totalBeforeDiscount-discount < p.Floor {
		return ErrDiscountBelowFloor
	}
	return nil
}

type unverifiedDiscountPolicy struct{}

func (unverifiedDiscountPolicy) ValidateAdditionalDiscount(int64, int32) error {
	return ErrDiscountPolicyUnverified
}

type PromptRevisionService struct {
	repo      promptRevisionRepository
	sanitizer HTMLSanitizer
	policy    DiscountPolicy
}

func NewPromptRevisionService(repo promptRevisionRepository, sanitizer HTMLSanitizer, policy DiscountPolicy) *PromptRevisionService {
	if sanitizer == nil {
		sanitizer = NewHTMLSanitizer()
	}
	if policy == nil {
		policy = unverifiedDiscountPolicy{}
	}
	return &PromptRevisionService{repo: repo, sanitizer: sanitizer, policy: policy}
}

// ApplyCommercialPatch recalculates one explicitly scoped quotation. The
// quotation ID is a route value rather than a patchable request property.
func (s *PromptRevisionService) ApplyCommercialPatch(ctx context.Context, companyID, quotationID uuid.UUID, userID *uuid.UUID, req dto.PromptPatchRequest) (*dto.PromptPatchResponse, error) {
	_ = userID
	if s == nil || s.repo == nil || companyID == uuid.Nil || quotationID == uuid.Nil || req.RevisionID == uuid.Nil || req.ContentVersion < 1 || req.StateVersion < 0 || len(req.Operations) == 0 {
		return nil, promptRevisionError("invalid_patch", errors.New("invalid patch envelope"))
	}
	record, err := s.repo.LoadPromptDraft(ctx, companyID, quotationID)
	if err != nil {
		return nil, promptRevisionLoadError(err)
	}
	return s.apply(ctx, companyID, record, req)
}

func (s *PromptRevisionService) apply(ctx context.Context, companyID uuid.UUID, record *repository.PromptDraftRecord, req dto.PromptPatchRequest) (*dto.PromptPatchResponse, error) {
	if record == nil || record.RevisionID != req.RevisionID || record.QuotationID == uuid.Nil {
		return nil, promptRevisionError("revision_not_found", repository.ErrRevisionNotFound)
	}
	if record.ContentVersion != req.ContentVersion || record.StateVersion != req.StateVersion {
		return nil, promptRevisionError("revision_conflict", repository.ErrRevisionConflict)
	}
	var currentState domain.PromptQuotationState
	if err := json.Unmarshal(record.PromptState, &currentState); err != nil || currentState.Validate() != nil {
		return nil, promptRevisionError("commercial_snapshot_unavailable", errors.New("stored prompt state is invalid"))
	}
	var priorCalculation domain.DedicatedSquadCalculation
	if err := json.Unmarshal(record.Calculation, &priorCalculation); err != nil || priorCalculation.TotalInvestment <= 0 {
		priorCalculation = domain.DedicatedSquadCalculation{}
	}

	state, err := clonePromptState(currentState)
	if err != nil {
		return nil, promptRevisionError("commercial_snapshot_unavailable", err)
	}
	discountChanged, err := applyPromptPatchOperations(&state, req.Operations)
	if err != nil {
		return nil, err
	}
	if discountChanged && priorCalculation.TotalInvestment <= 0 {
		return nil, promptRevisionError("initial_price_required", errors.New("an initial price is required before discount"))
	}
	state.Issues = state.ComputeIssues()
	state.Readiness = readinessForIssues(state.Issues)
	if err := state.Validate(); err != nil || state.Readiness == domain.ReadinessIncomplete {
		return nil, promptRevisionError("commercial_input_missing", errors.New("commercial state is not priceable"))
	}

	card, items, err := promptRateCardSnapshot(record.RateCardSnapshot)
	if err != nil {
		return nil, promptRevisionError("commercial_snapshot_unavailable", err)
	}
	anchor := promptPaymentAnchor(priorCalculation)
	input, issues := BuildDedicatedInput(&state, items, &anchor)
	if hasBlockingIssue(issues) {
		return nil, promptRevisionError("commercial_input_missing", errors.New("patched state cannot be calculated"))
	}
	profile, err := ResolveDedicatedSquadProfile(input)
	if err != nil {
		return nil, promptRevisionError("commercial_input_missing", err)
	}
	calculator := NewDedicatedSquadCalculator(snapshotDedicatedRateLookup{items: items})
	calculation, err := calculator.CalculateWithProfile(ctx, companyID.String(), card.ID.String(), input, profile)
	if err != nil {
		return nil, promptRevisionError("commercial_snapshot_unavailable", err)
	}
	calculation.TotalBeforeDiscount = calculation.TotalInvestment
	if state.AdditionalDiscountBPS != nil && *state.AdditionalDiscountBPS > 0 {
		if err := s.policy.ValidateAdditionalDiscount(calculation.TotalBeforeDiscount, *state.AdditionalDiscountBPS); err != nil {
			return nil, promptRevisionDiscountError(err)
		}
		calculation.AdditionalDiscount = splitAmount(calculation.TotalBeforeDiscount, int64(*state.AdditionalDiscountBPS))
		calculation.TotalInvestment, err = checkedAdd(calculation.TotalBeforeDiscount, -calculation.AdditionalDiscount)
		if err != nil {
			return nil, promptRevisionError("commercial_input_missing", err)
		}
	}
	if err := recalculatePromptPayments(&calculation, state.PaymentPattern); err != nil {
		return nil, promptRevisionError("payment_pattern_invalid", err)
	}
	readiness := EvaluatePromptReadiness(&state, &calculation, PromptPaymentReadiness{
		Rows:                   calculation.Payments,
		DiscountPolicyVerified: discountPolicyVerified(s.policy),
	})
	state.Issues = readiness.Issues
	state.Readiness = readiness.Readiness

	commercial, err := s.sanitizer.Sanitize(renderPromptCommercialSection(calculation))
	if err != nil || strings.TrimSpace(commercial) == "" {
		return nil, promptRevisionError("unsafe_html", errors.New("commercial fragment was rejected"))
	}
	payment, err := s.sanitizer.Sanitize(renderPromptPaymentSection(calculation))
	if err != nil || strings.TrimSpace(payment) == "" {
		return nil, promptRevisionError("unsafe_html", errors.New("payment fragment was rejected"))
	}
	merged, err := MergePromptSection(record.ContentHTML, "commercial", commercial)
	if err == nil {
		merged, err = MergePromptSection(merged, "payment", payment)
	}
	if err != nil {
		return nil, promptRevisionError("section_merge_invalid", err)
	}
	stateJSON, _ := MarshalPromptQuotationStateForPersistence(&state)
	calculationJSON, _ := json.Marshal(calculation)
	profileJSON, _ := json.Marshal(profile)
	sectionJSON, _ := json.Marshal(promptSectionHashes(merged))
	contentHash := sha256.Sum256([]byte(merged))
	contentHashString := hex.EncodeToString(contentHash[:])
	saved, err := s.repo.UpdatePromptDraftCAS(ctx, repository.UpdatePromptDraftCASParams{
		CompanyID: companyID, QuotationID: record.QuotationID, RevisionID: record.RevisionID,
		ExpectedContentVersion: req.ContentVersion, ExpectedStateVersion: req.StateVersion,
		State: stateJSON, ContentHTML: merged,
		Snapshots: repository.PromptDraftSnapshots{AllocationSnapshot: record.AllocationSnapshot, RateCardSnapshot: record.RateCardSnapshot, LegalTemplateSnapshot: record.LegalTemplateSnapshot, SkillSnapshot: record.SkillSnapshot, ResolvedProfile: profileJSON, Calculation: calculationJSON, Module: record.Module, Timeline: record.Timeline, SectionHashes: sectionJSON, ContentHash: &contentHashString},
	})
	if err != nil {
		if errors.Is(err, repository.ErrRevisionConflict) {
			return nil, promptRevisionError("revision_conflict", err)
		}
		return nil, promptRevisionError("revision_update_failed", err)
	}
	return &dto.PromptPatchResponse{QuotationID: record.QuotationID, RevisionID: record.RevisionID, ContentHTML: merged, ContentVersion: saved.ContentVersion, StateVersion: saved.StateVersion, PromptState: dto.NewPromptQuotationStateView(state), Issues: state.Issues, Readiness: state.Readiness, Calculation: calculation, CatalogRoles: PromptCatalogRoles(items)}, nil
}

func discountPolicyVerified(policy DiscountPolicy) bool {
	_, unverified := policy.(unverifiedDiscountPolicy)
	return !unverified
}

func clonePromptState(state domain.PromptQuotationState) (domain.PromptQuotationState, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return domain.PromptQuotationState{}, err
	}
	var copy domain.PromptQuotationState
	return copy, json.Unmarshal(data, &copy)
}

func applyPromptPatchOperations(state *domain.PromptQuotationState, operations []dto.PromptPatchOperation) (bool, error) {
	discountChanged := false
	for _, operation := range operations {
		switch operation.Type {
		case dto.PromptPatchSetDurationMonths:
			if operation.DurationMonths == nil || *operation.DurationMonths < 1 || *operation.DurationMonths > domain.MaxPromptPeriodQuarters/4 {
				return false, promptRevisionError("invalid_patch", errors.New("duration_months is invalid"))
			}
			state.DurationMonths = &domain.Fact[int]{Value: *operation.DurationMonths, Source: domain.SourceUser, Confirmed: true}
			for i := range state.Team {
				state.Team[i].PeriodQuarters = int32(*operation.DurationMonths * 4)
			}
		case dto.PromptPatchSetTeam:
			if operation.Team == nil || len(*operation.Team) == 0 || len(*operation.Team) > domain.MaxPromptTeamLines {
				return false, promptRevisionError("invalid_patch", errors.New("team is invalid"))
			}
			team := append([]domain.PromptTeamLine(nil), (*operation.Team)...)
			for i := range team {
				team[i].Source = domain.SourceUser
				team[i].Evidence = ""
				team[i].Confirmed = true
			}
			state.Team = team
		case dto.PromptPatchSetDiscountBPS:
			if operation.DiscountBPS == nil || *operation.DiscountBPS < 0 || *operation.DiscountBPS > 10_000 {
				return false, promptRevisionError("invalid_patch", errors.New("discount_bps is invalid"))
			}
			value := *operation.DiscountBPS
			state.AdditionalDiscountBPS = &value
			discountChanged = true
		case dto.PromptPatchSetPaymentPattern:
			if operation.PaymentPattern == nil || strings.TrimSpace(*operation.PaymentPattern) == "" {
				return false, promptRevisionError("invalid_patch", errors.New("payment_pattern is required"))
			}
			value := strings.TrimSpace(*operation.PaymentPattern)
			state.PaymentPattern = &domain.Fact[string]{Value: value, Source: domain.SourceUser, Confirmed: true}
		case dto.PromptPatchConfirmField:
			if operation.Field == nil {
				return false, promptRevisionError("invalid_patch", errors.New("field is required"))
			}
			switch strings.TrimSpace(*operation.Field) {
			case "duration_months":
				if state.DurationMonths != nil {
					state.DurationMonths.Confirmed = true
				} else {
					return false, promptRevisionError("invalid_patch", errors.New("duration is unavailable"))
				}
			case "payment_pattern":
				if state.PaymentPattern != nil {
					state.PaymentPattern.Confirmed = true
				} else {
					return false, promptRevisionError("invalid_patch", errors.New("payment pattern is unavailable"))
				}
			case "team":
				for i := range state.Team {
					state.Team[i].Confirmed = true
				}
			default:
				return false, promptRevisionError("invalid_patch", errors.New("field is not commercial"))
			}
		default:
			return false, promptRevisionError("invalid_patch", errors.New("unsupported patch operation"))
		}
	}
	return discountChanged, nil
}

type promptRateSnapshot struct {
	Card  domain.RateCard       `json:"card"`
	Items []domain.RateCardItem `json:"items"`
}

func promptRateCardSnapshot(raw json.RawMessage) (domain.RateCard, []domain.RateCardItem, error) {
	var snapshot promptRateSnapshot
	if len(raw) == 0 || json.Unmarshal(raw, &snapshot) != nil || snapshot.Card.ID == uuid.Nil || len(snapshot.Items) == 0 {
		return domain.RateCard{}, nil, errors.New("rate card snapshot is unavailable")
	}
	if snapshot.Card.Code != domain.DefaultPromptRateCardCode && !snapshot.Card.IsDefault {
		return domain.RateCard{}, nil, errors.New("rate card snapshot is not the default card")
	}
	return snapshot.Card, snapshot.Items, nil
}

type snapshotDedicatedRateLookup struct{ items []domain.RateCardItem }

func (lookup snapshotDedicatedRateLookup) ResolveItem(_ context.Context, _ string, _ string, role, seniority, term string) (*domain.RateCardItem, error) {
	for _, item := range lookup.items {
		if item.Role == role && item.Seniority == seniority && item.TermType == term {
			value := item
			return &value, nil
		}
	}
	return nil, fmt.Errorf("rate snapshot has no item for %s/%s/%s", role, seniority, term)
}

func promptPaymentAnchor(calc domain.DedicatedSquadCalculation) time.Time {
	if len(calc.Payments) > 0 && !calc.Payments[0].InvoiceDate.IsZero() {
		return calc.Payments[0].InvoiceDate.UTC()
	}
	return time.Now().UTC()
}

func recalculatePromptPayments(calculation *domain.DedicatedSquadCalculation, pattern *domain.Fact[string]) error {
	if calculation == nil || pattern == nil {
		return errors.New("payment pattern is unavailable")
	}
	value := strings.ToLower(strings.TrimSpace(pattern.Value))
	if value == "monthly" || value == "equal_monthly" || value == "equal monthly" {
		value = fmt.Sprintf("monthly:%d", len(calculation.Payments))
	}
	parts := SplitPayment(calculation.TotalInvestment, PaymentPattern(value))
	if len(parts) == 0 || len(parts) != len(calculation.Payments) {
		return errors.New("payment rows cannot be reconciled")
	}
	for i, part := range parts {
		calculation.Payments[i].Nominal = part.Amount
	}
	return nil
}

func renderPromptCommercialSection(calc domain.DedicatedSquadCalculation) string {
	var b strings.Builder
	b.WriteString(`<section data-quotation-section="commercial" data-section-version="1" data-managed="true">`)
	b.WriteString(`<h2>Nilai Investasi (Development Squad)</h2>`)
	unit, period := "Rate (Monthly)", "Period (Month)"
	daily := calc.Model == domain.ModelSprint
	if daily {
		unit, period = "Rate (Hari)", "Period (Hari)"
	}
	if len(calc.Development) > 0 {
		b.WriteString(`<table><thead><tr><th>Development Squad</th><th>` + unit + `</th><th>Employee</th><th>` + period + `</th><th>Total</th></tr></thead><tbody>`)
		renderDedicatedLines(&b, calc.Development, daily)
		if len(calc.Maintenance) > 0 {
			renderDedicatedLines(&b, calc.Maintenance, false)
		}
		if calc.Preparation.Gross > 0 {
			fmt.Fprintf(&b, `<tr data-field="preparation"><td>%s</td><td>-</td><td>-</td><td>-</td><td>Rp %s</td></tr>`, htmlEscape(preparationLabel(calc.Preparation.Kind)), formatNumber(calc.Preparation.Gross))
		}
		if calc.AdditionalDiscount > 0 {
			writeMoneyRow(&b, "total_before_discount", "TOTAL BEFORE ADDITIONAL DISCOUNT", calc.TotalBeforeDiscount)
			if calc.Discount > 0 {
				writeMoneyRow(&b, "preparation_discount", "PREPARATION DISCOUNT", calc.Discount)
			}
			writeMoneyRow(&b, "additional_discount", "ADDITIONAL DISCOUNT", calc.AdditionalDiscount)
			writeMoneyRow(&b, "total_investment", "TOTAL INVESTMENT", calc.TotalInvestment)
		} else {
			writeMoneyRow(&b, "total", "TOTAL", calc.Total)
			if calc.Discount > 0 {
				writeMoneyRow(&b, "discount", "DISCOUNT", calc.Discount)
			}
			writeMoneyRow(&b, "total_investment", "TOTAL INVESTMENT", calc.TotalInvestment)
		}
		b.WriteString(`</tbody></table>`)
	} else {
		b.WriteString(`<table><tbody>` +
			promptMoneyRow("total_before_discount", "TOTAL BEFORE ADDITIONAL DISCOUNT", calc.TotalBeforeDiscount) +
			promptMoneyRow("preparation_discount", "PREPARATION DISCOUNT", calc.Discount) +
			promptMoneyRow("additional_discount", "ADDITIONAL DISCOUNT", calc.AdditionalDiscount) +
			promptMoneyRow("total_investment", "TOTAL INVESTMENT", calc.TotalInvestment) + `</tbody></table>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}
func promptMoneyRow(field, label string, amount int64) string {
	return `<tr data-field="` + field + `"><td><strong>` + html.EscapeString(label) + `</strong></td><td>Rp ` + formatNumber(amount) + `</td></tr>`
}
func renderPromptPaymentSection(calc domain.DedicatedSquadCalculation) string {
	var b strings.Builder
	b.WriteString(`<section data-quotation-section="payment" data-section-version="1" data-managed="true"><table><thead><tr><th>Termin</th><th>Nominal</th></tr></thead><tbody>`)
	for i, payment := range calc.Payments {
		b.WriteString(`<tr data-field="payment_nominal"><td>Termin ` + fmt.Sprintf("%d", i+1) + `</td><td>Rp ` + formatNumber(payment.Nominal) + `</td></tr>`)
	}
	b.WriteString(`</tbody></table></section>`)
	return b.String()
}

func promptRevisionError(code string, err error) error {
	return &PromptRevisionError{Code: code, err: err}
}
func promptRevisionLoadError(err error) error {
	if errors.Is(err, repository.ErrRevisionNotFound) {
		return promptRevisionError("revision_not_found", err)
	}
	return promptRevisionError("revision_load_failed", err)
}
func promptRevisionDiscountError(err error) error {
	switch {
	case errors.Is(err, ErrDiscountPolicyUnverified):
		return promptRevisionError("discount_policy_unverified", err)
	case errors.Is(err, ErrDiscountAboveMaximum):
		return promptRevisionError("discount_above_max", err)
	case errors.Is(err, ErrDiscountBelowFloor):
		return promptRevisionError("discount_below_floor", err)
	default:
		return promptRevisionError("discount_rejected", err)
	}
}
func safePromptRevisionErrorCode(err error) string {
	var safe interface{ SafeCode() string }
	if errors.As(err, &safe) {
		return safe.SafeCode()
	}
	return "revision_update_failed"
}
