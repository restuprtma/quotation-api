package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

var (
	ErrInvalidAIBundle = errors.New("invalid AI bundle")
	ErrUnsafeAIBundle  = errors.New("unsafe AI bundle content detected")
)

var allowedDiagramTypes = map[string]bool{
	"journey":       true,
	"architecture":  true,
	"state_machine": true,
	"workflow":      true,
	"timeline":      true,
	"comparison":    true,
}

var unsafeMarkers = []string{
	"<script>", "</script>",
	"<style>", "</style>",
	"<iframe>", "</iframe>",
	"onclick=", "javascript:", "onload=", "onerror=",
}

// ValidateAIBundle runs all checks against the generated or incoming AI bundle
// to ensure it meets our structured schema and does not contain malicious injection.
func ValidateAIBundle(bundle *dto.AIGenerateResponse) error {
	if bundle.SchemaVersion == "" {
		return fmt.Errorf("%w: missing schema_version", ErrInvalidAIBundle)
	}

	if bundle.ProductOrProjectDetail.Mode != "product" && bundle.ProductOrProjectDetail.Mode != "project" {
		return fmt.Errorf("%w: invalid product_or_project_detail mode", ErrInvalidAIBundle)
	}

	// Validate Diagram types
	for _, diag := range bundle.Blueprint.Diagrams {
		if !allowedDiagramTypes[diag.Type] {
			return fmt.Errorf("%w: unsupported diagram type '%s'", ErrInvalidAIBundle, diag.Type)
		}
	}

	// Check lengths and sizes to prevent overly massive payloads (DoS protection)
	if len(bundle.Blueprint.Diagrams) > 10 {
		return fmt.Errorf("%w: too many diagrams (max 10)", ErrInvalidAIBundle)
	}
	if len(bundle.ProductOrProjectDetail.Modules) > 50 {
		return fmt.Errorf("%w: too many modules (max 50)", ErrInvalidAIBundle)
	}

	// Convert to JSON and check for unsafe substrings
	// We must disable HTML escaping so that <script> is not converted to \u003cscript\u003e
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(bundle); err != nil {
		return fmt.Errorf("%w: could not encode bundle", ErrInvalidAIBundle)
	}

	rawStr := strings.ToLower(buf.String())
	for _, marker := range unsafeMarkers {
		if strings.Contains(rawStr, marker) {
			return fmt.Errorf("%w: found '%s'", ErrUnsafeAIBundle, marker)
		}
	}

	// Example string length checks on big text fields
	if len(bundle.Narrative.Introduction) > 5000 {
		return fmt.Errorf("%w: narrative introduction too long (max 5000)", ErrInvalidAIBundle)
	}

	return nil
}
