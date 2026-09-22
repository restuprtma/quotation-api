package defaults

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

type RateCardFixture struct {
	SchemaVersion string `json:"schema_version"`
	FixtureNotice string `json:"fixture_notice"`
	RateCards     []struct {
		Code     string `json:"code"`
		Market   string `json:"market"`
		Currency string `json:"currency"`
		Items    []struct {
			Role           string `json:"role"`
			Seniority      string `json:"seniority"`
			TermType       string `json:"term_type"`
			AmountPerMonth int64  `json:"amount_per_month"`
		} `json:"items"`
	} `json:"rate_cards"`
}

func ParseRateCardFixture(filepath string) ([]domain.RateCard, []domain.RateCardItem, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, nil, err
	}

	var fixture RateCardFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, nil, err
	}

	var rateCards []domain.RateCard
	var rateCardItems []domain.RateCardItem

	for _, rcData := range fixture.RateCards {
		rc := domain.RateCard{
			Code:     rcData.Code,
			Name:     rcData.Code, // Using code as name
			Market:   rcData.Market,
			Currency: rcData.Currency,
			IsActive: true,
		}
		rateCards = append(rateCards, rc)

		for _, itemData := range rcData.Items {
			item := domain.RateCardItem{
				Role:           itemData.Role,
				Seniority:      itemData.Seniority,
				TermType:       itemData.TermType,
				AmountPerMonth: itemData.AmountPerMonth,
			}
			rateCardItems = append(rateCardItems, item)
		}
	}

	return rateCards, rateCardItems, nil
}

func ParseLegalTemplateMarkdown(filepath string) (*domain.LegalTemplate, error) {
	data, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	content := string(data)

	// Extract template_code from YAML
	codeRegex := regexp.MustCompile(`(?m)^template_code:\s*(.+)$`)
	codeMatches := codeRegex.FindStringSubmatch(content)
	if len(codeMatches) < 2 {
		return nil, errors.New("template_code not found in yaml metadata")
	}
	code := strings.TrimSpace(codeMatches[1])

	// Extract version from YAML
	versionRegex := regexp.MustCompile(`(?m)^version:\s*(\d+)$`)
	versionMatches := versionRegex.FindStringSubmatch(content)
	version := 1
	if len(versionMatches) >= 2 {
		v, err := strconv.Atoi(strings.TrimSpace(versionMatches[1]))
		if err == nil {
			version = v
		}
	}

	// Extract deterministic HTML block
	// Look for ```html ... ```
	htmlStart := strings.Index(content, "```html")
	if htmlStart == -1 {
		return nil, errors.New("html block not found")
	}
	htmlStart += len("```html")

	htmlEnd := strings.Index(content[htmlStart:], "```")
	if htmlEnd == -1 {
		return nil, errors.New("end of html block not found")
	}

	htmlContent := strings.TrimSpace(content[htmlStart : htmlStart+htmlEnd])

	lt := &domain.LegalTemplate{
		Code:        code,
		Version:     version,
		ContentHTML: htmlContent,
		IsActive:    true,
	}

	return lt, nil
}
