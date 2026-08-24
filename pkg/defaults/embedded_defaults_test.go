package defaults_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"venturo-skeleton-go/pkg/defaults"
)

func getProjectRoot() string {
	cwd, _ := os.Getwd()
	// Navigate up from quotation-api/pkg/defaults to project root
	return filepath.Join(cwd, "..", "..", "..")
}

func TestEmbeddedDefaultsParser(t *testing.T) {
	projectRoot := getProjectRoot()
	rateCardFixturePath := filepath.Join(projectRoot, "docs", "roadmap", "rnd", "quotation-skill", "rate-card-fixture.json")
	legalTemplatePath := filepath.Join(projectRoot, "docs", "roadmap", "rnd", "quotation-skill", "legal-template-v1.md")

	t.Run("ParseRateCardFixture is pure and idempotent", func(t *testing.T) {
		rateCards, rateCardItems, err := defaults.ParseRateCardFixture(rateCardFixturePath)
		if err != nil {
			t.Fatalf("failed to parse rate card fixture: %v", err)
		}

		if len(rateCards) == 0 {
			t.Errorf("expected parsed rate cards, got none")
		}

		if len(rateCardItems) == 0 {
			t.Errorf("expected parsed rate card items, got none")
		}

		// Verify correctness
		foundNational := false
		foundInternational := false
		for _, rc := range rateCards {
			if rc.Code == "SYNTH-NATIONAL-2026" {
				foundNational = true
				if rc.Market != "national" || rc.Currency != "IDR" {
					t.Errorf("incorrect fields for SYNTH-NATIONAL-2026")
				}
			}
			if rc.Code == "SYNTH-INTERNATIONAL-2026" {
				foundInternational = true
			}
		}

		if !foundNational || !foundInternational {
			t.Errorf("failed to find expected rate cards")
		}
	})

	t.Run("ParseLegalTemplateMarkdown is pure and idempotent", func(t *testing.T) {
		legalTemplate, err := defaults.ParseLegalTemplateMarkdown(legalTemplatePath)
		if err != nil {
			t.Fatalf("failed to parse legal template: %v", err)
		}

		if legalTemplate == nil {
			t.Fatalf("expected legal template, got nil")
		}

		if legalTemplate.Code != "quotation-legal-v1" {
			t.Errorf("expected code quotation-legal-v1, got %s", legalTemplate.Code)
		}

		if legalTemplate.Version != 1 {
			t.Errorf("expected version 1, got %d", legalTemplate.Version)
		}

		if !strings.Contains(legalTemplate.ContentHTML, "<section data-block-type=\"legal\"") {
			t.Errorf("expected content HTML to contain section, got %s", legalTemplate.ContentHTML)
		}
	})
}
