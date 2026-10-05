package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

type startPolicyProviderSource struct{ provider AIProvider }

func (s startPolicyProviderSource) ProviderForCompany(context.Context, uuid.UUID) (AIProvider, string, string, error) {
	return s.provider, "test", "model", nil
}

type startPolicyLegacyProvider struct{}

func (startPolicyLegacyProvider) Generate(context.Context, GenerateRequest) (GenerateResult, error) {
	return GenerateResult{}, nil
}

type startPolicyExtractionProvider struct{ startPolicyLegacyProvider }

func (startPolicyExtractionProvider) ExtractionCapabilities() ProviderCapabilities {
	return ProviderCapabilities{JSONSchema: true}
}
func (startPolicyExtractionProvider) GenerateExtraction(context.Context, ExtractionGenerateRequest) (ExtractionGenerateResult, error) {
	return ExtractionGenerateResult{Text: `{}`}, nil
}

func TestMeetingExtractionStartDeciderGeminiAndNonExtraction(t *testing.T) {
	company := uuid.New()
	allowed, err := NewMeetingExtractionStartDecider(startPolicyProviderSource{provider: startPolicyExtractionProvider{}}, "pipeline", 0).Decide(context.Background(), company, dto.StartMeetingExtractionRequest{})
	require.NoError(t, err)
	require.True(t, allowed)
	decider := NewMeetingExtractionStartDecider(startPolicyProviderSource{provider: startPolicyLegacyProvider{}}, "pipeline", 0)
	allowed, err = decider.Decide(context.Background(), company, dto.StartMeetingExtractionRequest{})
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, uint64(1), decider.LegacyFallbacks())
}

func TestMeetingExtractionStartDeciderShadowDefaultZeroDoesNotSample(t *testing.T) {
	decider := NewMeetingExtractionStartDecider(startPolicyProviderSource{provider: startPolicyExtractionProvider{}}, "shadow", 0)
	allowed, err := decider.Decide(context.Background(), uuid.New(), dto.StartMeetingExtractionRequest{})
	require.NoError(t, err)
	require.False(t, allowed)
}

func TestMeetingExtractionStartDeciderRejectsUnmappedSchemeBeforeRun(t *testing.T) {
	decider := NewMeetingExtractionStartDecider(startPolicyProviderSource{provider: startPolicyExtractionProvider{}}, "pipeline", 0)
	allowed, err := decider.Decide(context.Background(), uuid.New(), dto.StartMeetingExtractionRequest{ServiceScheme: "subscription_saas"})
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, uint64(1), decider.LegacyFallbacks())
}
