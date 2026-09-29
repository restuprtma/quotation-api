package service

import "venturo-skeleton-go/internal/modules/quotation/domain"

// PromptPaymentReadiness carries the server-calculated payment rows and the
// discount-policy gate. It has no client-provided monetary input: calculation
// remains the source of truth for the amount each row must reconcile to.
type PromptPaymentReadiness struct {
	Rows                   []domain.PaymentRow
	DiscountPolicyVerified bool
}

// ReadinessResult is the one displayable readiness decision for a prompt
// draft. It is deliberately pure so create, patch, and detail paths can use
// the same result without invoking a lifecycle or publication operation.
type ReadinessResult struct {
	Readiness string              `json:"readiness"`
	Issues    []domain.DraftIssue `json:"issues"`
}

// EvaluatePromptReadiness combines state completeness with the authoritative
// calculation and payment reconciliation. Missing price data is never treated
// as zero, and an unverified discount policy blocks finalization only when a
// non-zero additional discount was requested.
func EvaluatePromptReadiness(state *domain.PromptQuotationState, calculation *domain.DedicatedSquadCalculation, payment PromptPaymentReadiness) ReadinessResult {
	issues := make([]domain.DraftIssue, 0, 12)
	if state == nil {
		issues = append(issues, promptReadinessIssue("state.missing", "state", "prompt quotation state is required"))
		return readinessResult(issues)
	}
	if err := state.Validate(); err != nil {
		issues = append(issues, promptReadinessIssue("state.invalid", "state", err.Error()))
	} else {
		issues = appendUniquePromptIssues(issues, state.ComputeIssues()...)
		issues = appendUniquePromptIssues(issues, state.Issues...)
	}

	if calculation == nil || calculation.TotalInvestment <= 0 {
		issues = appendUniquePromptIssues(issues, promptReadinessIssue("price.unavailable", "calculation", "an authoritative initial price is not available"))
	} else {
		if len(payment.Rows) == 0 {
			issues = appendUniquePromptIssues(issues, promptReadinessIssue("payment.missing", "payments", "a payment schedule is required for the calculated total"))
		} else if paymentTotal(payment.Rows) != calculation.TotalInvestment {
			issues = appendUniquePromptIssues(issues, promptReadinessIssue("payment.total_mismatch", "payments", "payment schedule does not equal the calculated total"))
		}
	}
	if state.AdditionalDiscountBPS != nil && *state.AdditionalDiscountBPS > 0 && !payment.DiscountPolicyVerified {
		issues = appendUniquePromptIssues(issues, promptReadinessIssue("discount.policy_unverified", "additional_discount_bps", "discount policy is not verified"))
	}
	return readinessResult(issues)
}

func promptReadinessIssue(code, field, message string) domain.DraftIssue {
	return domain.DraftIssue{Code: code, Field: field, Message: message, Severity: domain.IssueBlocking}
}

func appendUniquePromptIssues(issues []domain.DraftIssue, additions ...domain.DraftIssue) []domain.DraftIssue {
	for _, addition := range additions {
		duplicate := false
		for _, issue := range issues {
			if issue.Code == addition.Code && issue.Field == addition.Field {
				duplicate = true
				break
			}
		}
		if !duplicate {
			issues = append(issues, addition)
		}
	}
	return issues
}

func paymentTotal(rows []domain.PaymentRow) int64 {
	var total int64
	for _, row := range rows {
		total += row.Nominal
	}
	return total
}

func readinessResult(issues []domain.DraftIssue) ReadinessResult {
	for _, issue := range issues {
		if issue.Severity == domain.IssueBlocking {
			return ReadinessResult{Readiness: domain.ReadinessIncomplete, Issues: issues}
		}
	}
	if len(issues) > 0 {
		return ReadinessResult{Readiness: domain.ReadinessPriceable, Issues: issues}
	}
	return ReadinessResult{Readiness: domain.ReadinessReadyForFinal, Issues: issues}
}
