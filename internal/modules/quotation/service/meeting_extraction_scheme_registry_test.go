package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestPipelineSchemeRegistryReadinessMatchesRegisteredMappers(t *testing.T) {
	require.NoError(t, ValidatePipelineSchemeRegistry())
	require.True(t, PipelineSchemeReady(domain.CommercialSchemeDedicatedSquad))
	for _, scheme := range domain.CommercialSchemes()[1:] {
		require.False(t, PipelineSchemeReady(scheme), "scheme without mapper must not be ready: %s", scheme)
	}
	require.False(t, PipelineSchemeReady(PlanSchemeUnknown))
	require.False(t, PipelineSchemeReady(PlanSchemeMultiService))
	require.False(t, PipelineSchemeReady("not-a-scheme"))
}
