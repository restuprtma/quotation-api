package repository_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"venturo-skeleton-go/internal/modules/quotation/domain"
)

func TestMigrationFiles_SignatureSettingsAndSnapshots(t *testing.T) {
	root := findProjectRoot(t)
	upFile := filepath.Join(root, "internal", "database", "migrations", "quotation", "20261013100000_signature_settings_and_snapshots.up.sql")
	dnFile := filepath.Join(root, "internal", "database", "migrations", "quotation", "20261013100000_signature_settings_and_snapshots.down.sql")

	contentUp, err := os.ReadFile(upFile)
	require.NoError(t, err, "up migration file must exist: %s", upFile)
	sqlUp := string(contentUp)

	assert.Contains(t, sqlUp, "CREATE TABLE IF NOT EXISTS quotation_signature_settings")
	assert.Contains(t, sqlUp, "director_signature_data")
	assert.Contains(t, sqlUp, "company_stamp_data")
	assert.Contains(t, sqlUp, "director_signature_snapshot")
	assert.Contains(t, sqlUp, "director_stamp_snapshot")

	contentDn, err := os.ReadFile(dnFile)
	require.NoError(t, err, "down migration file must exist: %s", dnFile)
	sqlDn := string(contentDn)
	assert.Contains(t, sqlDn, "DROP TABLE IF EXISTS quotation_signature_settings")

	// Domain assertion
	var _ domain.SignatureSettings
	var rev domain.QuotationRevision
	_ = rev.DirectorSignatureSnapshot
	_ = rev.DirectorStampSnapshot
}
