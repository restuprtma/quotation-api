package repository_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestMigrationFiles_PublicQuotationSignature(t *testing.T) {
	root := findProjectRoot(t)
	upFile := filepath.Join(root, "internal", "database", "migrations", "quotation", "20261006110000_public_quotation_signature.up.sql")
	dnFile := filepath.Join(root, "internal", "database", "migrations", "quotation", "20261006110000_public_quotation_signature.down.sql")

	contentUp, err := os.ReadFile(upFile)
	require.NoError(t, err, "up migration file must exist: %s", upFile)
	sqlUp := string(contentUp)

	assert.Contains(t, sqlUp, "idx_quotations_public_token_active")
	assert.Contains(t, sqlUp, "CREATE INDEX IF NOT EXISTS idx_quotations_public_token_active")
	assert.Contains(t, sqlUp, "ON quotations(public_token)")
	assert.Contains(t, sqlUp, "WHERE deleted_at IS NULL")

	contentDn, err := os.ReadFile(dnFile)
	require.NoError(t, err, "down migration file must exist: %s", dnFile)
	sqlDn := string(contentDn)
	assert.Contains(t, sqlDn, "DROP INDEX IF EXISTS idx_quotations_public_token_active")

	// Domain status constants assertion
	assert.Equal(t, "signed", domain.QuotationStatusSigned)
	assert.Equal(t, "published", domain.QuotationStatusPublished)
	assert.Equal(t, "draft", domain.QuotationStatusDraft)
}
