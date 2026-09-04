package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

const (
	maxExtractedModules       = 30
	maxSubFeaturesPerModule   = 20
	maxModuleNameRunes        = 120
	maxSubFeatureRunes        = 240
	maxModuleExtractorOutput  = 256 * 1024
	maxModuleRequirementRunes = 20_000
)

// ModuleExtractor limits AI involvement to identifying modules and their
// concrete sub-features from user-supplied requirements.
type ModuleExtractor struct{}

type moduleExtractionResponse struct {
	Modules []domain.Module `json:"modules"`
}

func (ModuleExtractor) Extract(ctx context.Context, provider AIProvider, requirement string) ([]domain.Module, error) {
	if provider == nil {
		return nil, errors.New("module extractor: provider is required")
	}
	requirement = strings.TrimSpace(requirement)
	if requirement == "" {
		return nil, errors.New("module extractor: requirement is required")
	}
	if utf8.RuneCountInString(requirement) > maxModuleRequirementRunes {
		return nil, fmt.Errorf("module extractor: requirement exceeds %d characters", maxModuleRequirementRunes)
	}

	encodedRequirement, err := json.Marshal(requirement)
	if err != nil {
		return nil, fmt.Errorf("module extractor: encode requirement: %w", err)
	}
	prompt := fmt.Sprintf("%s\n\n--- BEGIN UNTRUSTED REQUIREMENT ---\n%s\n--- END UNTRUSTED REQUIREMENT ---", GetDedicatedSquadModulesPrompt(), encodedRequirement)
	result, err := provider.Generate(ctx, GenerateRequest{Prompt: prompt})
	if err != nil {
		return nil, err
	}

	var response moduleExtractionResponse
	decoder := json.NewDecoder(io.LimitReader(strings.NewReader(result.Text), maxModuleExtractorOutput+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("module extractor: decode response: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("module extractor: decode response: %w", err)
	}
	if len(result.Text) > maxModuleExtractorOutput {
		return nil, errors.New("module extractor: response is too large")
	}
	if err := normalizeAndValidateModules(response.Modules); err != nil {
		return nil, err
	}
	return response.Modules, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values are not allowed")
	}
	return err
}

func normalizeAndValidateModules(modules []domain.Module) error {
	if len(modules) == 0 {
		return errors.New("module extractor: at least one module is required")
	}
	if len(modules) > maxExtractedModules {
		return fmt.Errorf("module extractor: at most %d modules are allowed", maxExtractedModules)
	}

	moduleNames := make(map[string]struct{}, len(modules))
	for i := range modules {
		modules[i].Name = normalizeModuleText(modules[i].Name)
		if err := validateExtractedText("module name", modules[i].Name, maxModuleNameRunes); err != nil {
			return fmt.Errorf("module extractor: modules[%d]: %w", i, err)
		}
		nameKey := strings.ToLower(modules[i].Name)
		if _, exists := moduleNames[nameKey]; exists {
			return fmt.Errorf("module extractor: duplicate module %q", modules[i].Name)
		}
		moduleNames[nameKey] = struct{}{}

		if len(modules[i].SubFeatures) == 0 {
			return fmt.Errorf("module extractor: modules[%d]: at least one sub-feature is required", i)
		}
		if len(modules[i].SubFeatures) > maxSubFeaturesPerModule {
			return fmt.Errorf("module extractor: modules[%d]: at most %d sub-features are allowed", i, maxSubFeaturesPerModule)
		}
		featureNames := make(map[string]struct{}, len(modules[i].SubFeatures))
		for j := range modules[i].SubFeatures {
			modules[i].SubFeatures[j] = normalizeModuleText(modules[i].SubFeatures[j])
			feature := modules[i].SubFeatures[j]
			if err := validateExtractedText("sub-feature", feature, maxSubFeatureRunes); err != nil {
				return fmt.Errorf("module extractor: modules[%d].sub_features[%d]: %w", i, j, err)
			}
			featureKey := strings.ToLower(feature)
			if _, exists := featureNames[featureKey]; exists {
				return fmt.Errorf("module extractor: modules[%d]: duplicate sub-feature %q", i, feature)
			}
			featureNames[featureKey] = struct{}{}
		}
	}
	return nil
}

func normalizeModuleText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func validateExtractedText(kind, value string, maxRunes int) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", kind)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s exceeds %d characters", kind, maxRunes)
	}
	if containsHTML(value) {
		return fmt.Errorf("%s must not contain HTML", kind)
	}
	return nil
}

func containsHTML(value string) bool {
	tokenizer := html.NewTokenizer(bytes.NewBufferString(value))
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return false
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken, html.CommentToken, html.DoctypeToken:
			return true
		}
	}
}
