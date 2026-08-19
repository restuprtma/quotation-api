package rnd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportSecurityIsNonCompensating(t *testing.T) {
	validation := newHTMLValidation()
	validation.Security["S4"] = ValidationCheck{Pass: false, Detail: "leak"}
	runs := []RunReport{{Status: "pass", SecurityChecks: validation.Security, StructuralChecks: validation.Structural}}
	aggregate := aggregateRuns(runs)
	if aggregate.Status != "fail" || aggregate.SecurityPass || aggregate.StructuralFullPassCount != 0 {
		t.Fatalf("unexpected aggregate: %+v", aggregate)
	}
}

func TestReportExpectedBudgetFailureCountsAsPass(t *testing.T) {
	security, structural := newExpectedFailureChecks()
	aggregate := aggregateRuns([]RunReport{{Status: "budget_unfeasible_expected", SecurityChecks: security, StructuralChecks: structural}})
	if aggregate.Status != "pass" || aggregate.PassCount != 1 || aggregate.ExpectedBudgetUnfeasibleCount != 1 || aggregate.StructuralFullPassCount != 1 {
		t.Fatalf("unexpected aggregate: %+v", aggregate)
	}
}

func TestWriteReportAtomicAndRedactedShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "report.json")
	report := BatchReport{SchemaVersion: "quotation-rnd-report-v1", Fixture: "fixture", Provider: "mock", Model: "model", PromptVersion: "quotation-v1", Runs: []RunReport{{Run: 1, Status: "pass"}}}
	if err := WriteReport(path, report); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var decoded BatchReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Provider != "mock" || decoded.Runs[0].Status == "" {
		t.Fatalf("unexpected report: %+v", decoded)
	}
	for _, forbidden := range []string{"raw_path", "raw_response", "budget_maximum", "prompt_text", "api_key"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("report contains forbidden field %q", forbidden)
		}
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".report-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary files remain: %v err=%v", matches, err)
	}
}
