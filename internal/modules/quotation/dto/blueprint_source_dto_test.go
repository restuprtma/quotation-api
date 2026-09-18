package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/dto"
)

func strPtr(s string) *string {
	return &s
}

func TestBlueprintSourceEligibility_Eligible(t *testing.T) {
	validHTML := strPtr("<p>Valid proposal content</p>")
	validStatuses := []string{"draft", "published", "superseded"}

	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			meta := dto.EvaluateBlueprintSourceEligibility(status, "completed", validHTML)
			assert.True(t, meta.BlueprintEligible)
			assert.Nil(t, meta.BlueprintIneligibleReason)
		})
	}
}

func TestBlueprintSourceEligibility_IncompleteGeneration(t *testing.T) {
	validHTML := strPtr("<p>Valid content</p>")
	incompleteStatuses := []string{"pending", "in_progress", "failed", "generating", ""}

	for _, genStatus := range incompleteStatuses {
		t.Run("gen_status_"+genStatus, func(t *testing.T) {
			meta := dto.EvaluateBlueprintSourceEligibility("draft", genStatus, validHTML)
			assert.False(t, meta.BlueprintEligible)
			require.NotNil(t, meta.BlueprintIneligibleReason)
			assert.Equal(t, "revision generation is incomplete", *meta.BlueprintIneligibleReason)
		})
	}
}

func TestBlueprintSourceEligibility_UnsupportedPublicationStatus(t *testing.T) {
	validHTML := strPtr("<p>Valid content</p>")
	unsupportedStatuses := []string{"archived", "deleted", "unknown", "rejected", ""}

	for _, pubStatus := range unsupportedStatuses {
		t.Run("pub_status_"+pubStatus, func(t *testing.T) {
			meta := dto.EvaluateBlueprintSourceEligibility(pubStatus, "completed", validHTML)
			assert.False(t, meta.BlueprintEligible)
			require.NotNil(t, meta.BlueprintIneligibleReason)
			assert.Equal(t, "unsupported publication status", *meta.BlueprintIneligibleReason)
		})
	}
}

func TestBlueprintSourceEligibility_BlankContentHTML(t *testing.T) {
	testCases := []struct {
		name string
		html *string
	}{
		{name: "nil content", html: nil},
		{name: "empty content", html: strPtr("")},
		{name: "whitespace content spaces", html: strPtr("   ")},
		{name: "whitespace content tabs and newlines", html: strPtr("\t\n  \r\n")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			meta := dto.EvaluateBlueprintSourceEligibility("draft", "completed", tc.html)
			assert.False(t, meta.BlueprintEligible)
			require.NotNil(t, meta.BlueprintIneligibleReason)
			assert.Equal(t, "revision content is blank", *meta.BlueprintIneligibleReason)
		})
	}
}

func TestBlueprintSource_JSONSerialization_ListItem(t *testing.T) {
	id := uuid.New()
	meta := dto.BlueprintSourceMetadata{
		BlueprintEligible:         true,
		BlueprintIneligibleReason: nil,
	}

	item := dto.QuotationRevisionResponse{
		ID:                      id,
		VersionNumber:           1,
		Status:                  "draft",
		BlueprintSourceMetadata: &meta,
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"blueprint_eligible":true`)
	assert.Contains(t, jsonStr, `"blueprint_ineligible_reason":null`)
	assert.NotContains(t, jsonStr, "content_html")
}

func TestBlueprintSource_JSONSerialization_DetailItem(t *testing.T) {
	id := uuid.New()
	item := dto.QuotationRevisionResponse{
		ID:                      id,
		VersionNumber:           1,
		Status:                  "draft",
		BlueprintSourceMetadata: nil, // stripped for detail contract
	}

	data, err := json.Marshal(item)
	require.NoError(t, err)

	jsonStr := string(data)
	assert.NotContains(t, jsonStr, "blueprint_eligible")
	assert.NotContains(t, jsonStr, "blueprint_ineligible_reason")
	assert.NotContains(t, jsonStr, "content_html")
}
