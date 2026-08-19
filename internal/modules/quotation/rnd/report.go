package rnd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RunReport struct {
	Run                   int                        `json:"run"`
	StartedAt             time.Time                  `json:"started_at"`
	LatencyMS             int64                      `json:"latency_ms"`
	Iterations            int                        `json:"iterations"`
	TermType              string                     `json:"term_type"`
	Currency              string                     `json:"currency"`
	TotalInvestment       *int64                     `json:"total_investment,omitempty"`
	UtilizationPercent    *int64                     `json:"utilization_percent,omitempty"`
	SecurityChecks        map[string]ValidationCheck `json:"security_checks"`
	StructuralChecks      map[string]ValidationCheck `json:"structural_checks"`
	RoundTripResult       string                     `json:"round_trip_result"`
	Status                string                     `json:"status"`
	ReasonCode            string                     `json:"reason_code,omitempty"`
	SanitizedEvidencePath string                     `json:"sanitized_evidence_path,omitempty"`
}

type AggregateReport struct {
	RunCount                      int    `json:"run_count"`
	PassCount                     int    `json:"pass_count"`
	FailCount                     int    `json:"fail_count"`
	ExpectedBudgetUnfeasibleCount int    `json:"expected_budget_unfeasible_count"`
	SecurityPass                  bool   `json:"security_pass"`
	StructuralFullPassCount       int    `json:"structural_full_pass_count"`
	Status                        string `json:"status"`
}

type BatchReport struct {
	SchemaVersion string          `json:"schema_version"`
	Fixture       string          `json:"fixture"`
	Provider      string          `json:"provider"`
	Model         string          `json:"model"`
	PromptVersion string          `json:"prompt_version"`
	Runs          []RunReport     `json:"runs"`
	Aggregate     AggregateReport `json:"aggregate"`
}

func allChecksPass(checks map[string]ValidationCheck, prefix string) bool {
	for i := 1; i <= 8; i++ {
		if !checks[fmt.Sprintf("%s%d", prefix, i)].Pass {
			return false
		}
	}
	return true
}

func newExpectedFailureChecks() (map[string]ValidationCheck, map[string]ValidationCheck) {
	security, structural := newHTMLValidation().Security, newHTMLValidation().Structural
	for key := range security {
		security[key] = ValidationCheck{Pass: true, Detail: "not_executed_budget_gate"}
	}
	for key := range structural {
		structural[key] = ValidationCheck{Pass: true, Detail: "expected_failure_equivalent"}
	}
	return security, structural
}

func WriteReport(path string, report BatchReport) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("report path is required")
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("create report directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".report-*.tmp")
	if err != nil {
		return fmt.Errorf("create report temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write report: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close report: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename report: %w", err)
	}
	return nil
}
