package service

import (
	"errors"
	"math"
	"regexp"
)

// DetermineTermType returns "short_term" if months < 3, else "long_term"
func DetermineTermType(months int) string {
	if months < 3 {
		return "short_term"
	}
	return "long_term"
}

// CalculateSafeLineTotal multiplies rate, count, and months with overflow protection
func CalculateSafeLineTotal(rate int64, count int, months int) (int64, error) {
	if rate < 0 || count < 0 || months < 0 {
		return 0, errors.New("negative value not allowed in calculation")
	}
	if rate == 0 || count == 0 || months == 0 {
		return 0, nil
	}

	// Overflow check: rate * count
	if int64(count) > math.MaxInt64/rate {
		return 0, errors.New("integer overflow during rate * count calculation")
	}
	sub := rate * int64(count)

	// Overflow check: sub * months
	if int64(months) > math.MaxInt64/sub {
		return 0, errors.New("integer overflow during subtotal * months calculation")
	}

	return sub * int64(months), nil
}

var legalBlockRegex = regexp.MustCompile(`(?is)<(?:div|section)[^>]*\bdata-(?:section|block-type|quotation-section)=["']legal["'][^>]*>.*?</(?:div|section)>`)

// ReplaceOrAppendLegalBlock replaces any existing legal section with official template, or appends it.
func ReplaceOrAppendLegalBlock(contentHTML, officialLegalHTML string) string {
	if legalBlockRegex.MatchString(contentHTML) {
		return legalBlockRegex.ReplaceAllString(contentHTML, officialLegalHTML)
	}
	return contentHTML + officialLegalHTML
}

// EnforceOfficialLegalBlock overrides any AI-generated legal section with official template
func EnforceOfficialLegalBlock(contentHTML, officialLegalHTML string, sanitizer HTMLSanitizer) string {
	sanitizedOfficial, err := sanitizer.Sanitize(officialLegalHTML)
	if err != nil {
		sanitizedOfficial = officialLegalHTML
	}
	// If AI output contained a legal block, replace it with canonical official legal block
	return ReplaceOrAppendLegalBlock(contentHTML, sanitizedOfficial)
}
