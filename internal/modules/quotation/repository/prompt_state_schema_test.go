//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptQuotationStateSchema(t *testing.T) {
	pool := getTestPool(t)
	ctx := context.Background()
	for _, column := range []string{"prompt_state", "prompt_state_version", "prompt_section_hashes"} {
		dataType, _, nullable, err := columnDetails(ctx, pool, "quotation_revisions", column)
		require.NoError(t, err)
		if column == "prompt_state_version" {
			assert.Equal(t, "bigint", dataType)
			assert.Equal(t, "NO", nullable)
		} else {
			assert.Equal(t, "jsonb", dataType)
			assert.Equal(t, "YES", nullable)
		}
	}
}
