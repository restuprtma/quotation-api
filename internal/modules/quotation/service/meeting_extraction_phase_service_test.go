package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestServiceDetailsPhase_AllSchemesHaveStrictSafeAssetsAndValidate(t *testing.T) {
	for _, scheme := range domain.CommercialSchemes() {
		t.Run(scheme, func(t *testing.T) {
			phase := NewServiceDetailsPhase(scheme)
			schema, err := phase.Schema(PhaseInput{})
			require.NoError(t, err)
			require.Contains(t, string(schema), `"additionalProperties":false`)
			for _, forbidden := range []string{"rate", "amount", "total", "price", "nominal", "discount", "quote"} {
				require.NotContains(t, strings.ToLower(string(schema)), `"`+forbidden+`"`)
			}
			key := firstServiceDetailKey(scheme)
			candidate := fmt.Sprintf(`{"scheme":%q,"facts":[{"key":%q,"value":"confirmed","evidence":{"refs":["L0001"],"support":"explicit"}}]}`, scheme, key)
			result, err := phase.Validate(context.Background(), PhaseInput{}, []byte(candidate))
			require.NoError(t, err)
			safe, err := result.SafeResultJSON()
			require.NoError(t, err)
			require.NotContains(t, string(safe), "meeting_notes")
			mapper, ok := ServiceDetailsMapperFor(scheme)
			require.True(t, ok)
			mapped, err := mapper.Map(result.(ServiceDetailsResult))
			require.NoError(t, err)
			require.Contains(t, mapped, key)
		})
	}
}
func TestServiceDetailsPhaseRejectsOtherSchemeKeysAndGenericNeedsReview(t *testing.T) {
	phase := NewServiceDetailsPhase(domain.CommercialSchemeDedicatedSquad)
	_, err := phase.Validate(context.Background(), PhaseInput{}, []byte(`{"scheme":"dedicated_squad","facts":[{"key":"package_name","value":"x","evidence":{"refs":["L0001"],"support":"explicit"}}]}`))
	require.Error(t, err)
	fallback := NewGenericServiceDetailsFallback()
	result, err := fallback.Validate(context.Background(), PhaseInput{}, []byte(`{}`))
	require.NoError(t, err)
	require.True(t, result.(ServiceDetailsResult).NeedsReview)
}

func TestM6SchemasHaveNoForbiddenMoneyProperties(t *testing.T) {
	schemas := []json.RawMessage{}
	p2, err := NewScopePhase().Schema(PhaseInput{})
	require.NoError(t, err)
	schemas = append(schemas, p2)
	roleID := uuid.New()
	p3, err := NewStaffingPhase([]domain.RateCardItem{{ID: roleID, Seniority: "senior"}}).Schema(PhaseInput{})
	require.NoError(t, err)
	schemas = append(schemas, p3)
	for _, scheme := range domain.CommercialSchemes() {
		schema, err := NewServiceDetailsPhase(scheme).Schema(PhaseInput{})
		require.NoError(t, err)
		schemas = append(schemas, schema)
	}
	for _, schema := range schemas {
		var value any
		require.NoError(t, json.Unmarshal(schema, &value))
		assertNoForbiddenSchemaProperties(t, value)
	}
}

func assertNoForbiddenSchemaProperties(t *testing.T, value any) {
	t.Helper()
	forbidden := map[string]bool{"rate": true, "amount": true, "total": true, "price": true, "nominal": true, "discount": true}
	switch node := value.(type) {
	case map[string]any:
		if properties, ok := node["properties"].(map[string]any); ok {
			for key := range properties {
				require.Falsef(t, forbidden[strings.ToLower(key)], "forbidden schema property %q", key)
			}
		}
		for _, child := range node {
			assertNoForbiddenSchemaProperties(t, child)
		}
	case []any:
		for _, child := range node {
			assertNoForbiddenSchemaProperties(t, child)
		}
	}
}
func firstServiceDetailKey(scheme string) string {
	for key := range serviceDetailKeys[scheme] {
		return key
	}
	return ""
}
