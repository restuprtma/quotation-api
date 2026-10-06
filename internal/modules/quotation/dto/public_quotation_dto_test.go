package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func TestPublicQuotationResponse_StrictRedaction(t *testing.T) {
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	subject := "Customer Portal"
	signer := "Budi Utomo"

	res := dto.PublicQuotationResponse{
		QuotationNumber: "VEN/SAMPLE/001",
		Subject:         &subject,
		Client: dto.PublicQuotationClient{
			CompanyName: "PT Contoh Aman",
		},
		RevisionNumber: 2,
		ContentHTML:    "<section><p>Clean HTML</p></section>",
		PublishedAt:    now,
		Signed:         true,
		SignerName:     &signer,
		SignedAt:       &now,
	}

	bytes, err := json.Marshal(res)
	require.NoError(t, err)

	var rawMap map[string]interface{}
	err = json.Unmarshal(bytes, &rawMap)
	require.NoError(t, err)

	// Verify allowed fields exist
	assert.Equal(t, "VEN/SAMPLE/001", rawMap["quotation_number"])
	assert.Equal(t, "Customer Portal", rawMap["subject"])
	assert.Equal(t, float64(2), rawMap["revision_number"])
	assert.Equal(t, "<section><p>Clean HTML</p></section>", rawMap["content_html"])
	assert.Equal(t, true, rawMap["signed"])
	assert.Equal(t, "Budi Utomo", rawMap["signer_name"])

	clientObj, ok := rawMap["client"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "PT Contoh Aman", clientObj["company_name"])

	// STRICT ZERO-LEAK VERIFICATION: Verify forbidden fields are absent
	forbiddenFields := []string{
		"company_id", "id", "client_id", "created_by", "budget_maximum",
		"generation_input", "allocation_snapshot", "rate_card_snapshot",
		"legal_template_snapshot", "prompt_state", "prompt_state_version",
		"prompt_section_hashes", "ai_provider", "ai_model", "skill_snapshot",
		"safety_envelope_version", "legal_content_modified", "commercial_content_modified",
		"signer_ip", "signed_ip", "signed_user_agent", "signed_content_hash",
		"signature_data", "content_version", "content_hash",
	}

	for _, field := range forbiddenFields {
		assert.NotContains(t, rawMap, field, "Public DTO MUST NOT contain forbidden internal field: %s", field)
	}
}
