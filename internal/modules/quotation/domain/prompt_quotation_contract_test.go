package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptQuotationStatePipelineFixturesRoundTripAllSchemes(t *testing.T) {
	schemes := []string{
		CommercialSchemeDedicatedSquad,
		CommercialSchemeSubscriptionSaaS,
		CommercialSchemeWhiteLabel,
		CommercialSchemeStandaloneMaintenance,
		CommercialSchemeCustomDevelopment,
		CommercialSchemeKioskQueueSystem,
		CommercialSchemeTalentOutsourcing,
		CommercialSchemeCloudInfrastructure,
	}
	for _, scheme := range schemes {
		for _, kind := range []string{"valid", "incomplete"} {
			t.Run(scheme+"/"+kind, func(t *testing.T) {
				body, err := os.ReadFile(filepath.Join("..", "service", "testdata", "meeting_extraction_pipeline", scheme+"_"+kind+".json"))
				require.NoError(t, err)
				var state PromptQuotationState
				require.NoError(t, json.Unmarshal(body, &state))
				require.NoError(t, state.Validate())
				require.NotNil(t, state.CommercialScheme)
				require.Equal(t, scheme, state.CommercialScheme.Value)
				roundTrip, err := json.Marshal(state)
				require.NoError(t, err)
				var restored PromptQuotationState
				require.NoError(t, json.Unmarshal(roundTrip, &restored))
				require.Equal(t, state, restored)
			})
		}
	}
}

func TestEvidenceReferenceTokenIsNonSecretAndCompatibleWithMeetingFactValidation(t *testing.T) {
	token := NewEvidenceReferenceToken("C003", "L0012-L0014", 18, 42, "abc123")
	require.Equal(t, "ev1:C003:L0012-L0014:o18-42:h=abc123", token)
	require.True(t, IsEvidenceReferenceToken(token))
	require.NoError(t, validateFactSource("scope[0]", SourceMeeting, token))
}
